# 43 · Zero-Downtime Deploys & Schema Migrations

`🟢 easy` · *Topics:* two versions always overlap, expand/contract migrations, what locks what in Postgres, `lock_timeout` and retry, rolling vs blue/green vs canary, graceful shutdown and draining, rollback reality, compatibility of queue messages, caches and SPA assets across versions · *Builds on:* [15 Job Scheduler](15-job-scheduler.md) (leases: a job survives a worker restart), [09 Feature Flags](09-feature-flags.md) (deploy is not release), [10 Webhook Delivery](10-webhook-delivery.md) (versioned messages), [41 Backup & Restore](41-backup-restore.md) (restore points)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [42 Integration Credentials Vault](42-credentials-vault.md) · Next: [44 Bulk Email Campaign Sender](44-email-campaign-sender.md)

---

> 🌱 **Seed (honeydolist-site):** one image per commit, built once by `make package` and run by SHA on stage and production (`DEPLOYING-TO-PROD.md`). `make deploy TO=prod` runs `deploy.sh <sha>` on the server: pull, **dump the database** (the last seven dumps kept), then `compose up -d --wait`, where a one-off `migrate` container runs first (`depends_on: service_completed_successfully`) and Traefik sends traffic only once the api's own `healthcheck` subcommand passes (`start_interval: 1s`); `/readyz` reports the SHA. **Measured** (Traefik 3.7, a request every 0.2 s): **4.8 s of 404s** per deploy, down from ~12 s when the healthcheck probed every second while starting; the docs name the next step (start the new container beside the old, wait healthy, stop the old). Rollback is `make deploy-rollback`: the image only. *"The database never rolls back… every migration must work with the previous image too: add first, deploy, remove in a later release (expand → contract). `migrate down` is never run in production."* Migration comments hold the lessons: 0078 (*"on a table this size an index takes seconds and blocks writes while it builds"*), 0051 (a volatile default would number rows in disk order and rewrite the table; the backfill runs in one transaction with triggers off), 0082 (a `NOT NULL` column with a constant default *"rewrites no table"*). Queues absorb a deploy: a stopped worker fails nothing, messages wait, and the alert is *"oldest queued job older than 5 min"* (step 7l). Small-scale numbers: one api container, a few seconds of 404.
> **Here, at scale:** 200 app pods, 50 deploys a day, 20k req/s and a 2 TB table, with no visible downtime and no migration that blocks writes.

## 1. The task

> "Design how we ship code and schema changes to a 24/7 service: 200 pods, 50 deploys a day, a 2 TB
> Postgres table. Users must not notice a deploy, and a bad deploy must be stoppable and reversible."

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Deploy rate and who triggers it? | Sets how often versions overlap. | 50/day, any engineer via a pipeline; no deploy freezes. |
| Availability target? | Dropped requests count against it. | 99.99 %: ~173k failed requests/day of the 1.73B served. |
| Is a maintenance window acceptable? | If yes, life is easy; here it is not. | No. |
| What shares state with the app? | Each shared thing needs a compatibility rule. | Postgres, a queue (outbox), Redis cache, mobile and SPA clients. |
| Largest table and write rate? | Decides which DDL is dangerous. | 2 TB (~4B rows), steady writes. |
| Who can roll back, and how fast? | Defines the escape hatch. | On-call, in minutes; schema rollbacks are not promised. |
| One monolith binary or many services? | Many services multiply the compatibility matrix. | One API/worker binary, plus a few small services. |

## 3. Requirements

**Functional**
- A pipeline: build one immutable image per commit → migrate (expand only) → canary → rolling rollout → verify.
- Schema changes via *expand/contract*; large backfills run as throttled background jobs, outside migrations.
- Pause, abort and roll back a rollout; every deploy records its SHA, migration range and a restore point.

**Non-functional**
- No dropped requests from shutting a pod down; capacity never below 100 % during a rollout.
- No migration holds a lock that blocks normal reads or writes for more than ~3 s.
- Any code version that is live can talk to the schema, queue and cache as they are right now.

**Out of scope:** multi-region traffic shifting, mobile app releases, infrastructure provisioning.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Deploy cadence | one per ~29 min | 86,400 s ÷ 50 |
| Rollout length | ≈ 3–4 min | surge 25 % = 50 pods → 4 waves × (~15 s ready + ~30 s drain) (assumption) |
| Versions live at once | N and N−1, sometimes N−2 | a deploy during a rollback or another rollout |
| Load per pod | 100 req/s | 20,000 ÷ 200 |
| In flight per pod | ≈ 30 | Little's law: 100 req/s × 0.3 s p99 |
| Drops with no drain | 6,000 per deploy; 300k/day | 200 × 30; × 50 deploys |
| 99.99 % error budget | ≈ 173k requests/day | 1.73B × 0.0001 |
| DB connections during a rollout | 250 pods × 20 = 5,000 | 200 + 25 % surge; needs a pooler |
| Rows in the big table | ≈ 4×10⁹ | 2×10¹² B ÷ 500 B |
| Volatile-default `ADD COLUMN` | ≈ 2.8 h of exclusive lock | 2×10⁶ MB ÷ 200 MB/s = 10,000 s |
| Plain `CREATE INDEX` | ≈ 67 min of blocked writes | 2×10⁶ MB ÷ 500 MB/s = 4,000 s |
| Backfill of every row | ≈ 11 h | 4×10⁹ ÷ 10k per batch = 400k batches × ~100 ms (30 ms work + 70 ms pause) |

What the numbers tell us: **two versions are always running together**, so "stop, migrate, start" is impossible
and every change must be compatible with its neighbours. Skipping the drain alone would burn ~1.7× the daily error
budget. And a single careless `ALTER` is the difference between a 3 ms metadata change and a 3-hour outage.

## 5. Plan

1. Expand/contract: how a schema change spans several releases.
2. What each DDL statement locks, and the safe form of each.
3. Rollout strategy and graceful shutdown (requests, workers, long-lived connections).
4. Rollback: what can and cannot be undone, and restore points.
5. Compatibility across the DB, the queue, the cache, API clients and SPA assets.

## 6. High-level design

```
 commit ─► CI: tests + migration lint ─► build ONE image tagged <sha> ─► registry
                                                │
 release pipeline (one run per deploy, serialised):
   0  record deploy + pg_create_restore_point('deploy_<id>')
   1  migrate job, once:  lock_timeout 3 s, retry; EXPAND steps only          (never from 200 pods at boot)
   2  canary: 1 pod, then 5 %  ──► compare error rate and p99 with baseline for N min ──► abort or continue
   3  rolling: waves of 25 % surge, maxUnavailable 0, readiness gate, drain old pods
   4  workers: new consumers first; old workers stop claiming, finish the current job, exit
   later releases:  backfill job (throttled, resumable) → switch reads → CONTRACT migration (drop)

 LB ──► pods v N + v N−1 side by side ──► Postgres  (schema serves both)
                                      └─► queue / Redis / events  (messages and keys readable by both)
```

## 7. API

A small control plane over the pipeline (usually CI/CD tooling rather than a product API):

```
POST /deploys  { sha, strategy: "canary_rolling" }       → 202 { deploy_id }
GET  /deploys/{id}                                        → 200 { status, wave, canary_verdict, migration_from, migration_to, restore_point }
POST /deploys/{id}/pause | resume | abort                 → 200
POST /deploys/rollback  { to_sha }                        → 202    image only; schema is never rolled back
GET  /backfills                                           → 200 [ { name, cursor_id, max_id, state, rows_per_s } ]
POST /backfills/{name}/pause | resume                     → 200
PUT  /backfills/{name}  { batch_size, pause_ms }          → 200    the speed dial
GET  /readyz → 200 { version, schema: 86, draining: false }        GET /livez → 200
```

## 8. Data model

The control plane's own tables, then the shape of a safe change (expand → backfill → constrain → contract):

```sql
CREATE TABLE deploys (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, sha text NOT NULL,
    status text NOT NULL CHECK (status IN ('running', 'succeeded', 'rolled_back', 'failed')),
    migration_from integer, migration_to integer, restore_point text,
    canary_verdict text, started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
CREATE TABLE backfills (
    name text PRIMARY KEY, table_name text NOT NULL,
    cursor_id bigint NOT NULL DEFAULT 0, max_id bigint NOT NULL,       -- resume point and the end
    batch_size integer NOT NULL DEFAULT 10000, pause_ms integer NOT NULL DEFAULT 70,
    state text NOT NULL CHECK (state IN ('pending', 'running', 'paused', 'done')), updated_at timestamptz NOT NULL DEFAULT now()
);

-- release N — expand (metadata only, takes ACCESS EXCLUSIVE for milliseconds):
ALTER TABLE orders ADD COLUMN total_cents bigint;                       -- nullable, no default

-- the backfill job (not a migration), one batch; repeated until cursor_id >= max_id:
UPDATE orders SET total_cents = (total * 100)::bigint
 WHERE id > $1 AND id <= $1 + 10000 AND total_cents IS NULL;

-- release N+1 — constrain without a long lock:
ALTER TABLE orders ADD CONSTRAINT orders_total_cents_nn CHECK (total_cents IS NOT NULL) NOT VALID;
ALTER TABLE orders VALIDATE CONSTRAINT orders_total_cents_nn;          -- scans, but writes continue
ALTER TABLE orders ALTER COLUMN total_cents SET NOT NULL;              -- PG 12+: uses the validated CHECK, no scan
ALTER TABLE orders DROP CONSTRAINT orders_total_cents_nn;

-- an index, in its own migration file (goose: `-- +goose NO TRANSACTION`):
CREATE INDEX CONCURRENTLY orders_total_cents_idx ON orders (total_cents);

-- release N+3 — contract, alone in its release, after the rollback window:
ALTER TABLE orders DROP COLUMN total;
```

## 9. Deep dives

### 9.1 Expand/contract: a schema change is several releases

| Option | Pros | Cons |
|---|---|---|
| **Maintenance window** (stop → migrate → start) | simplest; one version at a time | downtime every time; impossible at 50 deploys/day |
| **Expand/contract** | no downtime; each step is reversible until the contract | several releases per change; dual-write code to maintain |
| **Shadow table + logical replication + cutover** | can change anything, even partitioning | heavy machinery; reserve for rewrites that nothing else can do |

| Release | Schema | Code |
|---|---|---|
| N (expand) | add `total_cents` (nullable) | writes **both** columns; reads `total` |
| between | backfill job fills old rows | unchanged |
| N+1 | NOT NULL via validated CHECK | reads `total_cents`; still writes both |
| N+2 | nothing | writes only `total_cents` |
| N+3 (contract) | drop `total` | no reference to it |

**Choice and why: expand/contract for every change that old code could trip over.** The rule behind it is the one
the repo writes down: after go-live, every migration must work with the *previous image too*, because the previous
image is both the rollback target and, for four minutes, a live neighbour. A **rename** is never `RENAME COLUMN`
(old pods would break instantly): it is add, dual-write, backfill, switch reads, drop. A type change is the same. The
repo's pre-launch habit of *rewriting migrations in place and flushing stage* (step 11; *"a gap is never reused"*) is a
luxury of having no live data; the moment there is data, an applied migration is never edited again.

### 9.2 What locks what, and the safe form of each

| Statement | What happens at 2 TB | Safe form |
|---|---|---|
| `ADD COLUMN` nullable, or with a **constant** default | metadata only (PG 11+), milliseconds (the repo's 0082) | as is |
| `ADD COLUMN … DEFAULT nextval()` / random uuid (volatile) | rewrites the table, ~2.8 h exclusive | add nullable, backfill in batches, then `SET DEFAULT` for new rows (the repo's 0051 reasoning) |
| `CREATE INDEX` | blocks writes ~67 min | `CREATE INDEX CONCURRENTLY` (cannot run in a transaction; an interrupted build leaves an `INVALID` index to drop and redo) |
| `ADD FOREIGN KEY` / `CHECK` | scans under a lock that blocks writes | `… NOT VALID`, then `VALIDATE CONSTRAINT` (writes continue) |
| `SET NOT NULL` | full scan under lock | validated `CHECK (col IS NOT NULL)` first (PG 12+ skips the scan) |
| `DROP COLUMN`, `ALTER TYPE` | brief lock; old code that names the column breaks | only in the contract step; type changes by adding a column |

**The lock-queue trap.** Even a "fast" `ALTER` needs `ACCESS EXCLUSIVE`. If one long transaction holds a conflicting
lock, the `ALTER` waits, and *every later query queues behind the `ALTER`*: a 3 ms change becomes an outage in
seconds. So every migration runs with `SET lock_timeout = '3s'` and **retries with backoff**:

```go
func runMigration(ctx context.Context, db *sql.DB, stmt string) error {
	for attempt := 1; attempt <= 8; attempt++ {
		conn, err := db.Conn(ctx) // one session: SET applies to this connection only
		if err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, "SET lock_timeout = '3s'")
		if err == nil {
			_, err = conn.ExecContext(ctx, stmt)
		}
		conn.Close()
		var pgErr *pgconn.PgError
		if err == nil {
			return nil
		}
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" { // 55P03 = lock_not_available
			return err
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return errors.New("migration kept timing out on locks; try again off-peak")
}
```

Migrations run **exactly once, in their own job before the rollout** (the repo's `migrate` container with
`service_completed_successfully`), never from every pod at boot, and under a Postgres advisory lock so two runs
cannot overlap. A CI **migration linter** (for example squawk) rejects the unsafe forms in the table above before they merge.

### 9.3 Rolling, blue/green, canary, and shutting a pod down properly

| Option | Pros | Cons |
|---|---|---|
| **Recreate** (the repo today: one container, 4.8 s of 404s measured) | trivial | downtime on every deploy |
| **Rolling** (surge 25 %, `maxUnavailable` 0) | no extra fleet beyond the surge; automatic | two versions mix; rolling back is another rollout |
| **Blue/green** (a full second fleet, flip the LB) | instant switch and rollback | 2× capacity; the DB is still shared; sessions and sockets must drain |
| **Canary** (1 pod → 5 % → rolling) with automated analysis | small blast radius; catches regressions on real traffic | needs trustworthy metrics; one user may hit both versions |

**Choice: canary, then rolling**, blue/green for rare risky changes. A pod stops in a fixed order, and each step
exists for a reason:

```go
var draining atomic.Bool // /readyz returns 503 while true

<-ctx.Done()                  // SIGTERM (signal.NotifyContext)
draining.Store(true)          // 1. fail readiness: the LB stops choosing this pod
time.Sleep(8 * time.Second)   // 2. wait for the LB's view to propagate (it is not instant)
sctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = srv.Shutdown(sctx)        // 3. refuse new connections, finish the ~30 in-flight requests
// 4. exit; the pod's grace period must exceed 8 s + 30 s, or the kernel kills it mid-request
```

The repo's `SHUTDOWN_TIMEOUT` (15 s default) is step 3. **Workers** stop *claiming*, finish the job in hand within
the grace period, and exit; a job that outlives it is returned to the queue by its lease expiring
([15](15-job-scheduler.md)), which is why handlers are idempotent. **Long-lived connections** (SSE/WebSocket) get a
close with a *reconnect-after-random-delay* hint, so 200 pods restarting do not stampede. Watch the **queue's age**,
not the deploy: the repo's alert (*oldest queued job > 5 min*) tells you whether workers came back.

### 9.4 Rollback reality

| Thing | Can it go back? | How |
|---|---|---|
| Code image | yes, in minutes | roll forward to the previous SHA (the repo: `make deploy-rollback`) |
| Behaviour behind a flag | instantly | [09](09-feature-flags.md): deploy is not release |
| Expand migrations | no need to | extra columns and indexes are harmless to old code |
| **Contract migrations** (drops) | **no**: the data is gone | restore from a point in time ([41](41-backup-restore.md)) |
| Data written in a new format | old code must tolerate it | tolerant readers; the new format is additive |

The repo takes a `pg_dump` before every deploy and keeps seven. At 2 TB and 50 deploys a day that is a 2 TB dump every
29 minutes: impossible. The scaled-up equivalent is a **named restore point** (`pg_create_restore_point('deploy_<id>')`)
plus continuous WAL archiving, so "the database as of just before deploy 1042" is a PITR target. Hence the rule:
**a release containing a contract migration cannot be rolled back past it, so contracts ship alone, after a soak
(say 7 days).** Very often the right "rollback" is a fast *roll forward* (revert commit, same pipeline).

### 9.5 Compatibility windows across everything the versions share

| Shared thing | Rule while N and N−1 run together |
|---|---|
| Postgres schema | expand/contract: both versions work at every step |
| Queue / outbox messages | `topic` + `payload jsonb` (the repo's outbox): **add** fields only, consumers ignore unknown ones; a breaking change gets a new topic (`lead.created.v2`); deploy **consumers before producers**; keep the old consumer until the old messages have drained |
| Redis cache entries | put a shape version in the key (`page:v7:…`) or accept both shapes; the repo keeps the cache warm across deploys on purpose and offers `cache-flush` when a release changes what a read carries |
| HTTP API for clients | additive only; old mobile apps live for years; breaking changes at a new version |
| SPA assets | content-hashed file names and the **previous release's assets kept for days**: a tab open since yesterday runs old JavaScript against the new API, and a rollout serves new JavaScript from a pod that may still be on the old API |

Order of operations, always: **additive schema → code that works with both → contract.**

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (2,000 pods, 500 deploys/day):** a rollout is always in progress, so deploys must be safe to overlap or be serialised by a merge train; canary analysis must be automatic; the 5,000-connection peak becomes 50,000, so a pooler is mandatory.
- **A migration waits behind a long transaction:** `lock_timeout` fails it fast, the release stops before the rollout, nothing is blocked.
- **The backfill hurts the primary:** adaptive pause on replica lag, run off-peak, and watch autovacuum (every updated row leaves a dead version: up to ~2 TB of dead tuples if done carelessly).
- **Canary regresses:** auto-abort; the old pods are still serving. **A readiness flap:** keep `/readyz` *local* (do not put the DB in it, or a DB blip drains the whole fleet).
- **A pod is killed mid-job:** lease expiry returns the job; idempotent handlers make the retry harmless.
- **A DB failover during a deploy:** pools reconnect with backoff; the deploy pauses rather than continues blind.
- **Observability:** error rate and p99 by version, queue age, replica lag, lock waits, connection count, backfill rows/s.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Expand/contract over several releases | one migration with the code | no downtime, a safe previous version | speed per change; dual-write code |
| Backfill as a throttled job | a big `UPDATE` in the migration | no long locks, resumable, pauses on lag | hours of wall-clock time |
| Canary then rolling | recreate / all at once | small blast radius, no dropped requests | metrics and pipeline work |
| Fail readiness, wait, then `Shutdown` | killing pods on SIGTERM | ~0 dropped requests | slower rollouts (~45 s a wave) |
| Restore point + WAL | a dump per deploy | cheap, any-second rollback of data | needs the PITR machinery of [41](41-backup-restore.md) |
| Contracts ship alone after a soak | dropping in the same release | the image rollback stays honest | columns linger for a week |

## 12. Check yourself

1. Why can't a 200-pod service use "stop, migrate, start", and what invariant replaces it?
2. Give the steps to rename a column with no downtime.
3. Why can a fast `ALTER TABLE` cause an outage, and what two settings/habits prevent it?
4. A pod gets SIGTERM: in what order does it shut down, and why the sleep before `Shutdown`?
5. A release that dropped a column has a bug. What can you roll back, and what should you have done differently?

<details><summary>Answers</summary>

1. A rollout takes minutes, during which old and new pods serve traffic together, so the schema, queue and cache
   must work for both. The invariant: every live version must be compatible with the current schema and messages
   (and every change is additive until the old version is gone).
2. Add the new column (nullable); release code that writes both; backfill old rows in throttled batches; release
   code that reads the new column; release code that writes only the new one; finally drop the old column in its own
   release. Never `RENAME COLUMN` in place.
3. It needs `ACCESS EXCLUSIVE`; if a long transaction holds a conflicting lock it waits, and all later queries queue
   behind it. Use `lock_timeout` (fail fast) with retry and backoff, and keep transactions short; lint migrations in CI.
4. Fail readiness, sleep until the load balancer has stopped routing to it, then `Shutdown` with a timeout to finish
   in-flight requests, then exit, with a grace period longer than the sum. The sleep exists because routing changes
   propagate with a delay; without it, requests keep arriving at a pod that has already stopped accepting.
5. The image can go back, but the dropped column's data cannot: only a point-in-time restore brings it back. The
   contract migration should have shipped alone, after a soak period, with a named restore point recorded.

</details>

## 13. In Go

- Related lessons: [62 Deployment & operations](../../62-deployment-operations.md) (rollouts, health checks, rollback), [66 Serving Go apps](../../66-serving-go-apps.md) (graceful shutdown behind a proxy), [20 HTTP server](../../20-http-server.md) (`http.Server.Shutdown`), [15 Sync & context](../../15-sync-context.md) (`signal.NotifyContext`, cancelling workers), [59 Production DB patterns](../../59-production-database-patterns.md) (advisory locks, batch updates), [34 Outbox](../../34-event-driven-outbox.md) (message versions), [61 CI with GitHub Actions](../../61-ci-github-actions.md) (the migration lint step), [36 Resilience patterns](../../36-resilience-patterns.md) (retry with backoff).
- Packages: `github.com/pressly/goose/v3` (the repo's migration tool; `-- +goose NO TRANSACTION` for `CONCURRENTLY`), `github.com/jackc/pgx/v5` (`pgconn.PgError` codes), `net/http`, `os/signal`, `sync/atomic`.
- Test the contract: a CI job that runs the *previous* release's test suite against the *new* schema (expand steps must keep it green), and a shutdown test that fires requests while sending SIGTERM and asserts none fail.

## 14. Extensions

- **Shadow-table migrations:** logical replication into a new table shape and a cutover, for changes expand/contract cannot express (repartitioning).
- **Progressive delivery automation:** SLO-based canary analysis (error budget burn, latency percentiles) deciding promote or abort by itself.
- **Deploy-time load shedding:** pause low-priority work (batch, email campaigns) during the rollout window.
- **Multi-region:** roll one region at a time with a bake period between; global traffic shifting.
- **Preview environments** and **shadow traffic**: replay a slice of production against the new version before the canary.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| App fleet (Go API, stateless) | 20k req/s (1.73B a day, so also the average); 200 pods, 250 mid-rollout (§4) | CPU for the traffic, pod requests for the node count | 20k × 2 ms = 40 vCPU; N = ceil(40 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(9.4) = 10 → 12. But pods are placed by *requests*: 200 × 0.3 vCPU = 60 vCPU must still fit after a zone is lost: 60 ÷ (⅔ × ~7 allocatable) = 12.9 → 15. Surge: 250 × 0.3 = 75 ≤ 15 × 7 = 105, so the rollout needs no extra nodes | `c7g.2xlarge` (8 vCPU / 16 GiB) on EKS or ECS | 15, five per zone | CPU-bound (JSON, auth, 1–3 SQL calls); a pod is 0.3 vCPU / 0.6 GiB, exactly the `c` ratio of 2 GiB per vCPU, so `c`, not `m`. 33 % busy normally, 50 % with a zone down; the zone-loss headroom doubles as the rollout surge |
| Connection pooler (PgBouncer) | 5,000 client connections mid-rollout (250 pods × 20, §4); ~11k queries/s | CPU (one core per process), memory | direct: 5,000 × 5–10 MiB = 25–50 GiB of the database's RAM; pooled: ~150 server connections; 11k ÷ 3 = 3.7k queries/s per node ≈ 0.1 vCPU | `c7g.large` (2 vCPU / 4 GiB); RDS Proxy | 3, one per zone | the pooler is what lets 25 % more pods connect during a rollout without a 50 GiB memory bill on Postgres; trivial CPU, so the smallest `c` for availability |
| Postgres (the 4×10⁹-row, 2 TB table and its neighbours) | 2 TB table (§4), ~3 TB with indexes; assume 3k durable writes/s + 8k indexed reads/s (15 % writes, 40 % of requests reach the DB); backfill 10k rows per ~100 ms (§4) | memory, then IOPS | hot set ≈ 3 % of 3 TB ≈ 90 GB < 128 GiB. IOPS: 3k × 2 + ~1k misses = 7k, plus backfill 10 batches/s × ~640 pages (10k × 500 B ÷ 8 KiB) = 6.4k ⇒ ≈ 13.4k ⇒ `gp3` at its 16,000 cap. CPU ≈ 3–4 vCPU of 16 | `db.r7g.4xlarge` (16 vCPU / 128 GiB), RDS Multi-AZ | 1 primary + synchronous standby | RAM keeps reads off disk, so `r`; the backfill, not the traffic, is the IOPS peak and `pause_ms` is its throttle; past 16k IOPS go to `io2` or NVMe (`i4i`) |

CI builds (one image per commit, 50 a day) are bursty CPU: ~5 min on a `c7g.4xlarge` (assumption) = 4.2 h a day, a Spot job. A full-table backfill writes ~7 MB of WAL per batch ≈ 70 MB/s, ≈ 2.8 TB over its 11 h: watch standby lag and the WAL archive, not CPU. The image registry (ECR) needs nothing sized.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| App: 15 × `c7g.2xlarge` (15 × $212) | $3,180 |
| PgBouncer: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.4xlarge` ($620 × 2 managed), Multi-AZ doubles | $2,480 |
| gp3 3,000 GB + 13k extra IOPS (= $305, ×2 for the standby) | $610 |
| ALB: 20k req/s × ~3 KB = 216 GB/h = 216 LCU × $0.008 × 730, + $16 | $1,280 |
| NAT: 3 × $33 + image pulls (15 nodes × 100 MB × 50 deploys × 30 d = 2.25 TB × $0.045) | $200 |
| WAL archive for restore points: ~260 GB/day × 7 d ≈ 1.8 TB on S3 | $40 |
| CI builders: 4.2 h a day of a `c7g.4xlarge` ($420) | $70 |
| **Total** | **≈ $8,000** (lines sum to $8,020; ≈ $6,000 with a 1-year Savings Plan on the app, pooler and database) |

- **Biggest line and why:** the app fleet (≈ 40 %), then Postgres (≈ 38 %) and the ALB (≈ 16 %, billed by bytes). At 33 % CPU the nodes are bought so that 200 pods still fit when a zone is lost, not for traffic; the lever is the pod request: 0.2 instead of 0.3 vCPU gives 40 ÷ (⅔ × 7) = 8.6 → 9 nodes, ≈ $1,270 a month less.
- **What changes at 10×:** 200k req/s, 2,000 pods: 600 vCPU requested ÷ (⅔ × 7) = 129 → 129 × `c7g.2xlarge` (≈ $27k), with a rollout always in progress; the pooler must carry 50,000 client connections (~10 nodes); Postgres runs out first: 30k writes/s ≈ 60k IOPS and 80k reads/s are past one primary and past gp3's 16k, so NVMe (`i4i`) with read replicas, then sharding the big table; the ALB bill is ≈ $12.6k.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The thing being changed is one huge transactional table: ~4×10⁹ rows, 2 TB (~3 TB with indexes), ~3k durable writes/s and ~8k indexed reads/s, plus a backfill that updates 10k rows every ~100 ms for ~11 h (sections 4, 15). The control plane (`deploys`, `backfills`) is a few rows a day.
- So the question is not query speed but what each DDL statement locks and for how long: 9.2 prices a careless `ALTER` at 67 minutes to 2.8 hours, with two app versions live the whole time.
- 5,000 client connections during a rollout (250 pods × 20), and one thing that cannot be rolled back, a contract migration, which needs a restore point (9.4).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | every step of 9.2 has a form that holds a lock for milliseconds or lets writes continue: metadata-only `ADD COLUMN` with a constant default, `CREATE INDEX CONCURRENTLY`, `NOT VALID` then `VALIDATE CONSTRAINT`, and `SET NOT NULL` that reuses a validated CHECK (PG 12+, no scan). DDL is transactional, so a migration that fails halfway leaves nothing behind, and `lock_timeout` plus a retry on `55P03` defuses the lock-queue trap |
| MySQL (RDS / Aurora) | ✔ works, with a workaround | `INSTANT` add-column and online index builds (`ALGORITHM=INPLACE, LOCK=NONE`) cover the first two rows of 9.2. There is no `NOT VALID`/`VALIDATE` pair, so `NOT NULL` on 4×10⁹ rows is an online but full rebuild of the 2 TB table (hours: the lesson's 2.8 h rewrite is the right order of magnitude, and it needs room for a second copy), and anything that cannot be online goes through gh-ost or pt-online-schema-change. Every `ALTER` commits implicitly, so a migration file is not all-or-nothing. The lock-queue trap exists here too (metadata locks), bounded with `lock_wait_timeout` |
| DynamoDB | ✖ poor fit | no DDL, so no `ALTER` outage, but the work moves into code (tolerant readers, an item version, a scan-based backfill) and the table loses joins and ad-hoc queries; expand/contract (9.1) is still needed, as a discipline instead of a lock problem |
| MongoDB / DocumentDB | ✖ poor fit | the cheat sheet reaches for documents when the schema churns, but this is a fixed-shape table with constraints (`CHECK`, foreign keys), and a mixed fleet still needs dual writes and compatible readers (9.5): only the DDL step disappears |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.4xlarge`, section 15) behind a pooler (PgBouncer on 3 × `c7g.large`, or RDS Proxy). Three things decide it: the safe DDL forms above (each is a row of 9.2, and a CI linter can reject the unsafe ones); `lock_timeout` with an advisory lock, so the one-shot migration job can be retried without ever queueing the fleet behind it; and WAL archiving for point-in-time recovery, which is what makes a contract migration recoverable (9.4). The pooler is the price of a process per connection: 5,000 direct connections × 5–10 MiB would take 25–50 GiB of the database's RAM (section 15). `deploys` and `backfills` are tiny tables in the same database.

**If it were MySQL instead:** the design survives, and most rows of 9.2 get a MySQL form:
- add-column and index builds use `INSTANT` and `INPLACE, LOCK=NONE` (no `INVALID` index to clean up, but an online-DDL log that must be large enough to hold ~3k writes/s for hours);
- `NOT VALID` / `VALIDATE` has no counterpart: `NOT NULL`, a type change or any rewrite becomes an online rebuild or a gh-ost copy of 2 TB (hours, throttled on replica lag, extra disk);
- `lock_timeout` becomes `lock_wait_timeout` with the same retry; the advisory lock becomes `GET_LOCK`; migrations stop being all-or-nothing, so each step must be safe to re-run by hand;
- the backfill is the same PK-range batches and cheaper: InnoDB updates in place, so the ~2 TB dead-tuple risk of section 10 becomes watching undo length; a restore point becomes a recorded binlog position or timestamp; thousands of connections are cheaper, but still pool.

**What would flip the choice:** a change no safe form covers (repartitioning, a primary-key or type change on the 2 TB table) calls for a shadow table (9.1: logical replication on PostgreSQL, gh-ost on MySQL), which is a tool and not an engine switch. The table outgrowing one primary is the real trigger: at 10× (30k writes/s ≈ 60k IOPS, section 15) you shard it by key, and if you must shard anyway, MySQL with Vitess is the more proven path (cheat sheet). If you already run MySQL, do not switch for this reason: the same expand/contract rules plus gh-ost are enough.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 20,000 pods (25,000 mid-rollout), 2M req/s; **5,000 deploys a day** (one per 17 s); the big table 200 TB (4×10¹¹ rows) at 300k writes/s; 500k client connections mid-rollout; ~1,290 `c7g.2xlarge` | the one primary: 300k writes/s is 30× its ~10k ceiling, and a full backfill is 11 h × 100 = 46 days (a plain `CREATE INDEX` 4.6 days of blocked writes). Right behind it, the one serialised pipeline: a 3.5-min rollout caps it near 410 deploys a day, so 5,000 means ~12 rollouts overlapping | shard the table by `customer_id` over ~100 Postgres shards, as section 16 says at 10×: each is today's 2 TB table at ~3k writes/s, so every safe form of 9.2 still holds and the backfill runs on all shards at once in ~11 h. One migration runner applies each expand to shards in waves (`lock_timeout` 3 s, retry) and records a schema version per shard. ~100 per-service pipelines (today's 50 deploys a day each) with a merge train inside each; poolers per shard group (500k clients ÷ ~5k per PgBouncer node ≈ 100). Give up: atomic cross-shard DDL (shards are briefly on different versions), cross-shard joins and transactions | ≈ $0.8M/month (linear: $8,020 × 100) |
| **1,000×** | 200,000 pods (250,000 mid-rollout), 20M req/s; 50,000 deploys a day (one per 1.7 s: hyperscaler territory); a 2 PB table at 3M writes/s; 5M connections mid-rollout; ~12,900 nodes | one cluster and one rollout domain: 12,900 nodes is ~2.6× the ~5,000 a Kubernetes cluster is tested for, a node fails about every 23 min (the cheat sheet says hourly at 5,000), and four 25 % waves of 45 s put a bad build in front of 100 % of users in ~3 min, before a canary bake can say no | independent **cells**: ~100, each ≈ the 10× design (2,000 pods, ~129 nodes, a 20 TB table over ~10 shards, 200k req/s) with its own cluster, load balancer, pooler and queue, behind a thin `customer_id → cell` router. Roll out cell by cell (a canary cell with 1 % of traffic, then 2, 10, the rest) with a bake and an auto-halt on error budget: a bad build reaches 1 % of users, and a rollout takes ~1 h. The deployer and migration runner are control plane: cells keep serving without them. Give up: instant global deploy and rollback (a hotfix lane skips bakes), cross-cell queries, and the two-version rule (a 1-h rollout at one deploy per 29 min keeps N, N−1 and N−2 live: contract soaks get longer) | ≈ $8M/month (linear); per-cell fixed lines (ALB, NAT, pooler) are noise |
| **10,000×** | 2M pods, 200M req/s (more than any real service serves: a thought experiment); 500,000 deploys a day (~6 a second); a 20 PB table at 30M writes/s; ~129,000 nodes | the rate of change and the bill: a node dies every ~2 min (5,000 ÷ 129,000 of an hour), ≈ $80M a month, and at 6 deploys a second every cell is permanently mid-rollout, so there is no quiet moment in which one schema or one version is the only one | change the problem: ship *behaviour* as data: flags and config ([09](09-feature-flags.md)) become the main channel (seconds, per cell, reversible) and binaries ship less often; canary on a ~1 % sample of cells by error-budget burn instead of baking every cell; additive-only schema enforced by the linter, contracts run by a janitor that drops a column after N days of zero reads; bound the live table (time partitions, archive past ~90 days) so a DDL still touches TB, not PB; shrink the pod request (0.2 instead of 0.3 vCPU saves ≈ $12.7M a month). Give up: shipping code everywhere at once, a human reading every release (risk lanes instead), ad-hoc queries over old rows | ≈ $80M/month if nothing changes: the bill is the break |

**What stays the same at every scale:** one immutable image per commit; every change is expand → backfill → contract in separate releases, so any live version works against the current schema, queue and cache (N and N−1 at every scale); `lock_timeout` with a retry on `55P03`; the shutdown order (fail readiness, wait, `Shutdown`); and a contract that ships alone after a soak, with a restore point.

**Say it in an interview:** "At 100× one primary is 30× over its write ceiling and a backfill would take 46 days, so I shard the table by customer and run today's expand/contract on every shard, with one pipeline per service because 5,000 deploys a day cannot go through one. At 1,000× the cluster and the blast radius break (12,900 nodes, a global rollout in three minutes), so I split into ~100 cells and roll out cell by cell from a 1 % canary. At 10,000× nothing can be rolled out everywhere at once, so I ship behaviour as flags and config, canary on a sample, make the schema additive-only with a janitor for contracts, and keep the live table bounded."
