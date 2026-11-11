# 15 · Job Scheduler

`🟢 easy` · *Topics:* `FOR UPDATE SKIP LOCKED` dispatch, leases and crash recovery, at-least-once + idempotent handlers, cron with time zones and DST, catch-up policies, why exactly-once is a myth · *Builds on:* [08 Uptime Monitor](08-uptime-monitor.md) (due-queues), [10 Webhook Delivery](10-webhook-delivery.md) (retries), used by 02, 09, 12, 13, 14

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [14 Polls & Voting](14-polls-voting.md) · Next: [16 Public Form Intake](16-public-form-intake.md)

---

## 1. The task

> "Design a job scheduler. Services can say 'run this at time T' or 'run this every day at
> 09:00'. It must run jobs reliably, once, even when a scheduler node crashes."

Half the easy tasks have quietly depended on "a job that runs later". This is that job. The
word to interrogate is *once*.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Who executes the job: the scheduler, or does it just trigger something? | Running arbitrary code is a different product. | The scheduler **dispatches**: it enqueues a message or calls an HTTP endpoint. The owning service does the work. |
| Scale? | Rows and dispatch rate. | 10M scheduled jobs live; ~1k dispatches/s, bursts of 20k at round hours. |
| Delay range? | Seconds vs months changes storage. | 1 second to 1 year. |
| Recurring jobs? Time zones? | Cron + tz + DST are where bugs live. | Yes, cron expressions with a time zone per job. |
| "Once" — what exactly? | Exactly-once execution is impossible across a network; pick at-least-once or at-most-once. | At-least-once dispatch; handlers must be idempotent. |
| What if the scheduler was down when a job was due? | Catch-up policy. | Configurable: run once (default), skip, or run all missed. |
| Timeouts, retries, priorities? | Per-job policy. | Yes: per job type. |
| Precision? | Second vs millisecond. | Within ~1 s of the scheduled time. |
| Cancellation and rescheduling? | Mutations on pending jobs. | Yes. |

## 3. Requirements

**Functional**
- Schedule a one-off job at time T or a recurring job by cron expression (with tz).
- Dispatch due jobs to their target (queue message or HTTP call) within ~1 s.
- Retry failed dispatches; record each run; cancel or reschedule pending jobs.
- Recurring jobs compute their next occurrence after each run; missed runs follow a policy.

**Non-functional**
- No due job is ever lost or silently skipped, including across scheduler crashes.
- At-least-once dispatch with a unique run ID so targets can dedupe.
- Multiple scheduler nodes run concurrently without double-dispatching under normal operation.

**Out of scope**
- Executing user code, DAG workflows and dependencies, multi-tenant fairness (extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Live scheduled jobs | 10M rows × ~300 B = 3 GB | one table, indexed |
| Dispatches | 1k/s average; 20k burst at :00 | hourly crons pile up |
| Dispatcher poll | every 500 ms, batch 500 | 2 polls/s × N nodes; each pulls ≤ 500 |
| Burst drain | 20k ÷ (500 × 2/s × 4 nodes) = 5 s | acceptable; add nodes or batch size for faster |
| Run history | 86M runs/day × ~200 B ≈ 17 GB/day | keep 7 days detailed, aggregate after |

What the numbers tell us: a relational database handles this scale directly, if the dispatch
query is indexed and multiple nodes can take disjoint batches. The design is about **not
losing, not duplicating (much), and not drifting**: correctness, not throughput.

## 5. Plan

1. The model: a job, its next run, a lease.
2. Dispatch loop with `SKIP LOCKED`, and why it makes multiple nodes safe.
3. Leases: what happens when a node dies mid-dispatch.
4. "Once": at-least-once plus idempotency keys; why exactly-once can't be promised.
5. Recurring jobs: cron, time zones, DST, drift, catch-up.
6. Alternatives (Redis ZSET, brokers with delays, Temporal) and when to pick them.

## 6. High-level design

```
 client service ──POST /jobs {run_at | cron+tz, target, payload, policy}──► API ──► Postgres jobs (next_run_at indexed)

 dispatcher nodes (×N, identical, no leader):
   loop every 500 ms:
     BEGIN;
     SELECT … FROM jobs WHERE status='scheduled' AND next_run_at <= now()
       ORDER BY next_run_at LIMIT 500 FOR UPDATE SKIP LOCKED;
     UPDATE those SET status='leased', lease_owner=$node, lease_until=now()+30s;
     COMMIT;
     for each job: INSERT runs(run_id, job_id, scheduled_for) ; dispatch(target, payload, run_id)
                   success → one-off: status='done' | recurring: next_run_at = cron.Next(scheduled_for), status='scheduled'
                   failure → attempts+1, next_run_at = now()+backoff (or dead-letter), status='scheduled'
 reaper (any node): UPDATE jobs SET status='scheduled' WHERE status='leased' AND lease_until < now()   ← crashed node's jobs return

 targets: queue (asynq/SQS/Kafka) or HTTP webhook (task 10 rules) → the owning service runs the work, dedupes on run_id
```

Every node runs the same loop. The database hands each node a disjoint batch of due jobs by
locking rows and skipping the ones other nodes already hold. A lease with an expiry turns a
crashed node's jobs back into schedulable rows.

## 7. API

```
POST /jobs
  { "name": "send-digest", "target": { "type": "queue", "queue": "digests" } | { "type": "http", "url": "…" },
    "payload": { … },
    "run_at": "2026-10-14T09:00:00Z"            // one-off
    | "cron": "0 9 * * MON-FRI", "tz": "Europe/Berlin",   // recurring
    "policy": { "timeout_s": 30, "max_attempts": 5, "catch_up": "once" | "skip" | "all" } }
  → 201 { "id": "job_…", "next_run_at": "…" }

GET    /jobs/{id}                → status, next_run_at, last runs
DELETE /jobs/{id}                → 204 (cancel; a leased job finishes its current dispatch)
PATCH  /jobs/{id}                → reschedule / pause / resume
GET    /jobs/{id}/runs?cursor=…  → run history: scheduled_for, started_at, finished_at, outcome
```

Dispatch payload to the target includes `run_id` and `scheduled_for`, the idempotency handle.

## 8. Data model

```sql
CREATE TABLE jobs (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    target        JSONB NOT NULL,
    payload       JSONB NOT NULL,
    cron          TEXT,                            -- NULL for one-off
    tz            TEXT NOT NULL DEFAULT 'UTC',
    status        TEXT NOT NULL,                   -- scheduled | leased | paused | done | dead
    next_run_at   TIMESTAMPTZ,                     -- the occurrence being waited for
    lease_owner   TEXT,
    lease_until   TIMESTAMPTZ,
    attempts      INTEGER NOT NULL DEFAULT 0,      -- for the current occurrence
    policy        JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX jobs_due_idx ON jobs (next_run_at) WHERE status = 'scheduled';   -- the only hot index

CREATE TABLE runs (
    run_id        TEXT PRIMARY KEY,                -- job_id + scheduled_for, deterministic
    job_id        TEXT NOT NULL REFERENCES jobs(id),
    scheduled_for TIMESTAMPTZ NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    outcome       TEXT,                            -- dispatched | failed | dead
    error         TEXT,
    UNIQUE (job_id, scheduled_for)
);  -- partition by day; drop after 7 days
```

`run_id = job_id + ":" + scheduled_for` is deterministic: two dispatches of the same occurrence
(a retry, a crash replay) carry the same ID, which is what lets the target dedupe.

## 9. Deep dives

### 9.1 Dispatching with `SKIP LOCKED`

Several nodes poll the same table. Without care they all select the same due rows and
double-dispatch. `FOR UPDATE SKIP LOCKED` makes a `SELECT` lock the rows it returns and *skip*
rows another transaction has already locked, so concurrent pollers get disjoint batches with no
coordination service and no leader. It's the standard Postgres queue idiom (lesson 59).

| Design | Pros | Cons |
|---|---|---|
| single leader node polls | no concurrency issues | leader election needed; one node's throughput; failover gap |
| **N identical nodes + `SKIP LOCKED`** | linear scale-out, no leader, trivially HA | every node polls the same index; keep the batch small and the transaction short |
| partition jobs by hash across nodes | no shared polling | rebalancing when nodes join/leave (etcd leases); medium track |

The poll is cheap when the partial index exists: the planner walks `jobs_due_idx` from the
left until `next_run_at > now()`. Polling every 500 ms gives ≤ 0.5 s dispatch latency; add
`LISTEN/NOTIFY` on insert to wake pollers for near-immediate one-offs.

Use the **database's clock** (`now()`) in the query, not each node's, so skewed nodes don't
dispatch early or late.

### 9.2 Leases: surviving a dead dispatcher

A node takes a batch, marks it `leased` with `lease_until = now() + 30 s`, commits, and then
dispatches each job. If it crashes after the commit, those rows are `leased` forever, unless
someone reclaims them. The **reaper** query (run by every node each loop) returns expired leases
to `scheduled`. Consequences:

- Any job the dead node had *already* dispatched before crashing will be dispatched again by
  whoever picks it up. That's the at-least-once duplicate, carried with the same `run_id`.
- The lease duration bounds the delay after a crash (≤ 30 s). A node that's merely slow must
  **extend** its lease (heartbeat `lease_until`) or finish before it expires, otherwise a second
  node dispatches the same batch concurrently. Keep batches small enough to finish well within the lease.
- Mark each job's outcome individually right after its dispatch, not once per batch, so a crash
  loses at most one job's status.

### 9.3 "Once" is at-least-once plus idempotency

A dispatcher sends the message, then updates the row. If it dies between the two, the row says
"not done" and the job runs again. If it updates the row first and dies before sending, the job
never runs. There is no ordering of "send over the network" and "write to the database" that
makes both failures impossible, because the two systems don't share a transaction. The honest
options:

| Guarantee | How | Failure |
|---|---|---|
| **at-least-once** | dispatch, then mark done; retry on uncertainty | duplicates |
| at-most-once | mark done, then dispatch | lost runs |
| "exactly-once" | at-least-once + **idempotent consumer** keyed on `run_id` | none visible to the business, as long as every target dedupes |

**Choice: at-least-once, deterministic `run_id`, idempotent targets.** The target records
`run_id` in its own transaction with the work it does (`INSERT … ON CONFLICT DO NOTHING` on
`run_id`), so a second delivery is a no-op. Lesson 44's rule, "return nil to ack, err to retry,
dedupe on the natural key", is this. State it in the scheduler's documentation as a contract.

### 9.4 Recurring jobs: cron, time zones, DST, drift

- **Next occurrence from the scheduled time, not the run time:** `next = cron.Next(scheduled_for)`.
  Computing from "now" lets a 3-second-late run push every subsequent run 3 seconds later, forever (drift).
- **Time zones:** "09:00 Europe/Berlin" is 07:00 UTC in summer and 08:00 UTC in winter. Store the
  cron with its IANA zone and compute occurrences in that zone, then store `next_run_at` as UTC.
- **DST gaps and overlaps:** on the spring-forward night 02:30 does not exist; on fall-back it
  exists twice. A daily `30 2 * * *` job must have a defined behaviour: skip the gap (or run at
  03:00), and run once, not twice, in the overlap. Good cron libraries define this; test it with the
  two transition dates every year.
- **Catch-up policy** after downtime (the scheduler was off from 08:00 to 11:00 and a job was due
  hourly): `once` runs it one time now and resumes the schedule (the usual choice for "refresh the
  cache"), `skip` jumps to the next future occurrence (for "send the 09:00 reminder", which is
  pointless at 11:00), `all` runs the three missed occurrences in order (for "generate the hourly
  report", where every hour matters). It's per job because no default fits all three.
- **Overlap:** should the 10:00 run start if the 09:00 run of the same job is still going? For
  dispatch-only schedulers this is the target's concern; offer a `concurrency: forbid` flag that
  skips an occurrence while a run is open.

### 9.5 Alternatives, and when they win

| Alternative | Strengths | Weaknesses |
|---|---|---|
| **Redis sorted set** (score = `run_at`, Lua pop) as in task 08 | microsecond pops, 100k+/s | Redis durability is weaker; you re-implement leases/retries; two stores (Redis + DB for job definitions) |
| broker-native delayed messages (RabbitMQ delayed exchange, SQS delay ≤ 15 min, asynq `ProcessIn`) | nothing to build for short delays | caps on delay; no visibility or cancellation of pending jobs; cron still needs a scheduler process |
| Kafka | durable log | no delay/scheduling semantics at all; wrong tool |
| **Temporal / durable execution** | retries, timers and long-running workflows as code | a platform to run and learn; overkill for "run this at 9" |
| Kubernetes CronJob / system cron | zero code | one node, no history, no API, no catch-up policy, no jitter |

**Choice: Postgres-backed.** One durable store holds definitions, schedule, leases and history;
`SKIP LOCKED` gives multi-node dispatch; the query planner does the "due" check. Move the due
index to Redis only if the dispatch rate outgrows the database (task 08 scale), and reach for
Temporal when jobs become multi-step workflows.

### 9.6 Dispatching to targets

- **Queue target:** publish a message with `run_id` and the payload. The broker's durability
  makes the dispatch itself reliable; the consumer dedupes.
- **HTTP target:** POST with a timeout, signed (task 10's HMAC), success = `2xx`. Treat the
  target like any webhook endpoint: SSRF rules, no redirects, backoff, dead-letter after
  `max_attempts`.
- Retries of a *dispatch* are the scheduler's job; retries of the *work* belong to the target
  (lesson 44). Don't conflate them: a job that was dispatched but whose handler failed is "dispatched" to us.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 200k jobs due in the same second (everyone's hourly cron). The
  due index is hit by N nodes at once and the table receives 200k status updates in a few seconds.
  Mitigate with **jitter** on recurring jobs (`0 * * * *` becomes `0 * * * *` + per-job offset
  within a window, when the owner allows), bigger batches, more nodes, and partitioning the table
  by `hash(id)` so nodes poll different partitions.
- **A node is slow, not dead:** its lease expires mid-batch and another node dispatches the same
  jobs: duplicates. Heartbeat the lease and size batches to finish in a fraction of the lease.
- **Database down:** no dispatch at all. Jobs are not lost; when it returns, the catch-up policies
  decide what runs. This is the one component whose availability bounds the scheduler's; run it HA.
- **Target down:** dispatch fails and retries with backoff; after `max_attempts` the run is `dead`
  and alerts. A recurring job still schedules its next occurrence; a one-off waits in dead-letter.
- **Poison job** (payload that makes the target crash): per-job `max_attempts`, dead-letter,
  and never let one job's failure block the batch.
- **Clock jump on the DB** (NTP step): `now()` jumps, jobs fire early or late once. Rare; slew, don't step.
- **Observability:** lag = `now() − next_run_at` for the oldest `scheduled` job (should be ~0),
  leased count by node, dispatch error rate, dead-letter count. Lag is the alert.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Postgres + `SKIP LOCKED` | leader + in-memory timers | durability, HA with identical nodes, history in SQL | polling latency (~0.5 s), DB as the throughput ceiling |
| leases with a reaper | no leases / leader-only | crash recovery without coordination | duplicates after a crash |
| at-least-once + deterministic `run_id` | pretending exactly-once | achievable and honest | every target must dedupe |
| next from `scheduled_for` | next from `now()` | no drift | a very late recurring job may fire twice in quick succession unless `skip`/`once` policy says otherwise |
| per-job catch-up policy | one global rule | correct semantics per use case | one more thing for owners to decide |
| dispatch-only | executing code | tiny, safe scheduler | a target queue/endpoint per job type |

## 12. Check yourself

1. What does `FOR UPDATE SKIP LOCKED` buy you, and what would happen without it with three dispatcher nodes?
2. Why is exactly-once dispatch impossible here, and what do you do instead? Where does the dedup key come from?
3. A dispatcher node is killed after leasing 500 jobs and dispatching 200. Walk through recovery.
4. Why compute the next occurrence from the scheduled time rather than from the actual run time?
5. A daily job at 02:30 Europe/Berlin on the spring-forward night. What should happen, and why must it be defined explicitly?

<details><summary>Answers</summary>

1. Each poller locks the rows it selects and skips rows already locked by another transaction, so
   concurrent nodes receive disjoint batches without a leader or coordination service. Without it,
   all three select the same due rows and every job is dispatched three times (or they block on each
   other's locks).
2. The dispatch (network send) and the status update (database write) are in different systems and
   cannot be atomic together; whichever order you choose, a crash between them produces either a
   duplicate or a loss. Choose at-least-once and give every occurrence a deterministic
   `run_id = job_id + scheduled_for`, which the target records with its work so repeats are no-ops.
3. The 500 rows stay `leased` with a `lease_until` 30 s out. Within that window nothing happens;
   when it passes, the reaper flips them to `scheduled`, and other nodes pick them up. The 200 already
   dispatched are dispatched again with the same `run_id`; targets dedupe. The 300 never dispatched
   run late by up to the lease duration. Nothing is lost.
4. If a run starts 3 s late and the next is computed from the start time, every subsequent run is 3 s
   later than the previous schedule, accumulating forever. Computing from the scheduled occurrence
   keeps the series anchored to the cron grid.
5. 02:30 local does not exist that night (02:00 jumps to 03:00). The scheduler must pick a rule: skip
   that day, or run at the first valid instant (03:00 local). Either is fine; undefined behaviour means
   some library runs it twice (on fall-back) or never, and nobody notices until a report is missing.

</details>

## 13. In Go

- Related lessons: [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE SKIP LOCKED`, lease columns), [44 Background jobs](../../44-background-jobs-queues.md) (asynq's `Scheduler` and `ProcessAt`, the ack/retry contract), [15 Sync & context](../../15-sync-context.md) (dispatcher loop with `context` and a ticker, graceful shutdown that finishes the current batch), [35 Sagas](../../35-sagas-distributed-transactions.md) (idempotency keys), [36 Resilience](../../36-resilience-patterns.md) (backoff), [34 Outbox](../../34-event-driven-outbox.md).
- Cron parsing with time zones: `github.com/robfig/cron/v3` (`cron.ParseStandard`, `.Next(t)` in a `time.Location`) or `github.com/adhocore/gronx`. Test the DST transition dates explicitly with `time.LoadLocation("Europe/Berlin")`.
- Build the dispatcher as: `pollOnce(ctx) (n int, err error)` with the two-statement transaction, a `for { select { case <-ticker.C: … case <-ctx.Done(): return } }` loop, `errgroup` for per-job dispatch with bounded concurrency, and a `reap(ctx)` call each loop.
- Packages: `pgx`, `robfig/cron/v3`, `hibiken/asynq` (as a target), `golang.org/x/sync/errgroup`, `net/http`.

## 14. Extensions

- **Partitioned dispatch:** nodes own hash partitions via etcd leases and keep in-memory timing wheels; compare latency and complexity with polling (medium: distributed task queue).
- **Workflows / DAGs:** job B after job A succeeds; where does the dependency graph live, and how do retries propagate?
- **Multi-tenant fairness:** one tenant scheduling 1M jobs for 09:00 must not starve others; per-tenant rate limits (task 03) at dispatch.
- **`LISTEN/NOTIFY`** wake-ups for sub-second one-off jobs; measure the latency gain over polling.
- **Durable execution** (Temporal-style): timers, retries and long sleeps as code; when does it beat this design?
- **Observability dashboard:** scheduled-vs-actual lag histogram, dead-letter browser, manual re-run.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Dispatcher nodes (Go, identical, `SKIP LOCKED` loop; the job API runs in the same binary) | 1k dispatches/s average, 20k burst at :00; poll every 500 ms, batch 500 | drain rate set by the poll cadence, not CPU | CPU: 6,000/s × 0.4 ms (HMAC, JSON, one outbound call) = 2.4 vCPU, N = ceil(2.4 ÷ (0.8 × 2 × ⅔ = 1.07)) = 3. Drain: 2 polls × 500 = 1,000/s per node, so the lesson's 4 nodes empty 20k in 5 s; they must survive a zone loss ⇒ 6 nodes (4 left = 4,000/s = 5 s). Memory: ~1,200 calls in flight × ~10 KiB ≈ 12 MB | `c7g.large` (2 vCPU / 4 GiB) on ECS or EKS | 6, two per zone | Mostly waiting on targets with a little CPU, so the smallest `c`; memory is nowhere near 4 GiB. 20 % busy while draining at full cadence. The count buys availability and drain speed, not capacity |
| Postgres (`jobs`, `runs`) | burst drain 6,000 dispatches/s (≈ 1k average); 10M jobs ≈ 3 GB; runs 17 GB/day kept 7 days | CPU and write IOPS (the hot set is small) | CPU: 6,000 × 0.5 ms (lease update, `runs` insert, outcome update, two indexes) = 3 of 8 vCPU; IOPS: ≈ 2 per dispatch (heap + `jobs_due_idx` churn + WAL with grouped commits, assumption) = 12k + ~1k other ⇒ `gp3` 14,000 (cap 16,000); hot set: `jobs` ≈ 3 GB × 2 (update bloat) + the day's `runs` tail < 16 GB; disk: runs ≈ 17 GB + ≈ 12 GB of indexes = 29 GB/day × 7 ≈ 200 GB + jobs ≈ 10 GB ⇒ 300 GB | `db.m7g.2xlarge` (8 vCPU / 32 GiB), RDS Multi-AZ | 1 primary + synchronous standby | Section 10 calls the database the scheduler's ceiling and its one availability bound, so it alone gets a standby. `m`, not `r`: the hot set is a few GB, so CPU and IOPS decide (38 % CPU at the burst). Drop the day's `runs` partition instead of `DELETE`, or the IOPS double. Above ~16k IOPS gp3 is the first limit (the 10× case) |

Targets own their capacity: dispatch is publish-or-POST only. Assume half of the dispatches go to SQS and half to HTTP endpoints; 1k/s is 2.63 B a month (1,000 × 2,628,000 s). Job API traffic (`POST /jobs`, run history) is far below the dispatch rate (assume ≤ 500/s ≈ 0.5 vCPU) and needs no nodes of its own. Dispatchers sit in private subnets, so HTTP calls leave through NAT gateways.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Dispatchers: 6 × `c7g.large` (6 × $53) | $320 |
| Postgres: `db.m7g.2xlarge` ≈ $480 managed, Multi-AZ doubles | $960 |
| gp3 300 GB + 11k extra IOPS: (300 × $0.08 + 11,000 × $0.005) × 2 = (24 + 55) × 2 | $160 |
| SQS: 1.31 B sends in batches of 10 ⇒ 131M requests × $0.40 per million (sent one by one: ≈ $530) | $50 |
| HTTP targets: 1.31 B × ~1 KB ≈ 1.3 TB out × ($0.09 egress + $0.045 NAT) + 3 NAT gateways × $33 | $280 |
| ALB for the job API: $16 + a few LCUs | $30 |
| **Total** | **≈ $1,800** (≈ $1,350 with 1-year commitments on dispatchers and database) |

- **Biggest line and why:** the database and its disk (≈ 62 %: $960 + $160 of $1,800): it is the throughput ceiling and the only stateful part, while the six nodes that do the dispatching cost a third of it. The per-dispatch lines are the surprise: SQS and NAT-routed HTTP together ($330) cost as much as all the dispatchers ($320), and 2.63 B dispatches a month is why batching the SQS sends (10× cheaper) matters.
- **What changes at 10×:** 10k dispatches/s average and 200k jobs due in one second (section 10). Draining them in ~5 s needs 40k/s ⇒ ~42 dispatcher nodes (or batches of 2,500), and 40k/s is ≈ 20 vCPU and ≈ 80k IOPS of database work, past one primary and past gp3 ⇒ partition `jobs` by `hash(id)` over ~10 primaries (each at today's 4 to 6k/s) and jitter recurring jobs so the burst is ~3k/s over a minute instead. Run history grows to 170 GB/day: move it out of Postgres (batched writes to S3 or a columnar store). SQS and NAT/egress lines scale linearly to ≈ $3.3k a month.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- `jobs`: 10M rows ≈ 3 GB with one hot access path, "the next 500 due rows, never the same row on two nodes" (section 6). Every dispatch rewrites a row at least twice (lease, outcome): ~1k/s on average, ~6k/s while a :00 burst drains (section 15).
- `runs`: 86M rows a day ≈ 17 GB/day, kept 7 days, with a deterministic `run_id` and `UNIQUE (job_id, scheduled_for)` as the dedupe handle.
- Correctness over throughput (section 4): no due job lost across a crash, so durable commits and a standby. The database's availability bounds the scheduler's (section 10).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `FOR UPDATE SKIP LOCKED` hands each node a disjoint batch; the partial index `jobs_due_idx … WHERE status = 'scheduled'` holds only waiting rows, so the poll walks a short index; `JSONB` holds target, payload and policy; `LISTEN/NOTIFY` can wake pollers for one-offs; a retried occurrence re-inserting its `run_id` is `INSERT … ON CONFLICT DO NOTHING`. Cost: every status change leaves a dead row version and, because indexed columns change, is never heap-only, so autovacuum must keep up (section 15 budgets 2×) |
| MySQL (RDS / Aurora) | ✔ works, with changes | `SKIP LOCKED` exists since 8.0, so the dispatch SQL is the same. Differences: no partial index, so `(status, next_run_at)` also indexes `done` and `dead` rows; the default `REPEATABLE READ` takes next-key locks on the index range the due-scan walks, so inserts into it (new jobs, the reaper returning leased rows) can wait or deadlock: run the dispatcher at `READ COMMITTED`; the `runs` insert becomes `ON DUPLICATE KEY UPDATE run_id = run_id`, and InnoDB's duplicate-key check locks the existing entry, so concurrent inserts of one `run_id` can deadlock (retry on error 1213); no `LISTEN/NOTIFY`, so polling only, which is the design's baseline anyway. In its favour: rows update in place, so no vacuum for this churning table |
| Redis sorted set | ✔ as a later, derived due-index; ✖ as the record | score = `run_at` plus a Lua pop does 100k+/s, but leases, retries, history and cancellation must be rebuilt, and a Redis loss would forget jobs (9.5) |
| DynamoDB | ✖ poor fit | a conditional update can take a lease, but the due-scan needs a time-bucket key, and a :00 burst of 20k/s lands on one bucket against the ~1,000 writes/s per-partition limit; lag and run-history queries are ad hoc |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.2xlarge`, section 15): `SKIP LOCKED` plus the partial due-index plus transactional lease and outcome updates give multi-node dispatch with no leader, and one durable store holds definitions, schedule, leases and history (9.5). Redis is not part of the design yet.

**If it were MySQL instead:** the dispatch loop is unchanged apart from `READ COMMITTED` and a `(status, next_run_at)` index. For `runs`, partitioned by day (section 8): both engines need the partition column in every unique key, and InnoDB also rejects foreign keys on partitioned tables, so `REFERENCES jobs(id)` goes. A long text `run_id` as InnoDB's clustered key is copied into every secondary index, so give `runs` a small surrogate key and keep `run_id` as a unique key beside it.

**What would flip the choice:** ~16k IOPS or ~6k dispatches/s is where one primary on `gp3` ends (the 10× case in section 15): partition `jobs` by `hash(id)` over ~10 primaries, still Postgres. A Redis due-index in front only if the dispatch rate outgrows the database (task 08 scale); run history leaves the database (S3 or a columnar store) at 170 GB/day; Temporal once jobs become multi-step workflows.
