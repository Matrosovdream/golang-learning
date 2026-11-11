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
