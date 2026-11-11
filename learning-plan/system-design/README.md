# System Design Track

A growing library of **system design tasks**, graded easy → medium → hard, one Markdown file per
task. Each file is a complete worked session: the task as an interviewer would state it, the
clarifying questions worth asking, a plan of attack, the solution (high-level design, API, data
model, deep dives), the trade-offs, check questions, and ideas for extending the task later.

> **Who does what:** Claude writes each task file and runs the session. You read the task, go
> through the questions, follow the plan, read the solution and the pros/cons, and ask whenever
> something is unclear. Nothing is built here; the deliverable is the *reasoning*.

---

## Contents

- [How a session works](#how-a-session-works)
- [Folder layout](#folder-layout)
- [The method: how we attack any task](#the-method-how-we-attack-any-task)
- [Toolbox: the building blocks](#toolbox-the-building-blocks)
- [Estimation cheat sheet](#estimation-cheat-sheet)
- [Task index](#task-index)
- [Adding a task](#adding-a-task)

---

## How a session works

Same rhythm as the lesson examples: one task at a time, `next` moves on.

```
 ┌──────────────────────────────────────────────────────────────────────────┐
 │ 1. 📄 Task        Claude states the task the way an interviewer would     │
 │ 2. ❓ Questions   we walk the clarifying questions; you add your own      │
 │ 3. 📐 Plan        the order of attack for this particular task             │
 │ 4. 🏗  Solve       high-level design → API → data model → deep dives       │
 │ 5. ⚖️  Pros/cons   every decision with its alternatives and trade-offs     │
 │ 6. ✅ Check       you answer the check questions in your own words        │
 │ 7. next           the task is ticked in PROGRESS.md, next task starts     │
 └──────────────────────────────────────────────────────────────────────────┘
```

You are never expected to produce the design unaided. Read, repeat it back, ask. The check
questions are where you find out whether it stuck.

## Folder layout

```
learning-plan/system-design/
├── README.md          # this file: method, toolbox, cheat sheet, index
├── PROGRESS.md        # tracker: ▶ Resume here + ticks
├── _template.md       # skeleton for a new task file
├── easy/              # 🟢  one service, one or two interesting decisions
│   ├── 01-url-shortener.md
│   └── ...
├── medium/            # 🟡  several components, real scaling pressure (backlog in medium/README.md)
└── hard/              # 🔴  distributed-systems problems (backlog in hard/README.md)
```

Every task file has the **same 14 sections** (see [_template.md](_template.md)), so you always
know where to look, and so a task can grow for years: a new deep dive is a new `9.x` heading,
a new idea goes under `14. Extensions`.

---

## The method: how we attack any task

A 45-minute interview, and the shape of every task file:

| Step | Time | What you do | What it produces |
|---|---|---|---|
| **1. Clarify** | 5 min | Ask about users, scale, features in/out, consistency vs availability, latency. | Functional + non-functional requirements, explicit *out of scope* list. |
| **2. Estimate** | 3 min | Requests per second, storage per year, bandwidth, cache size. Round aggressively. | Numbers that *drive* decisions ("1k writes/s: one Postgres is fine"). |
| **3. High-level design** | 10 min | Boxes and arrows, the API, the data model. Start with the simplest thing that works. | A diagram everyone agrees on before going deep. |
| **4. Deep dive** | 15 min | Pick the 2–3 components that are actually hard *for this task*. Alternatives, trade-offs, pick one and say why. | The part that shows you can think. |
| **5. Wrap up** | 5 min | Bottlenecks, single points of failure, what breaks at 10× scale, what you would do next. | A prioritised list, not a promise of perfection. |

Rules that hold across all tasks:

- **Numbers first.** The same feature is a Postgres table at 100 writes/s and a sharded queue at
  100k writes/s. Never choose a component before estimating.
- **Simplest thing first, then scale it.** Say "one server, one database" out loud, then add a
  cache, a queue, a shard *because of a number*, not because it looks impressive.
- **Every box must earn its place.** If you cannot say what breaks without it, remove it.
- **Trade-offs, not answers.** There is no best database. There is a best database *for these
  requirements*. Say what you give up.
- **Reads and writes are different problems.** Most systems are read-heavy; cache reads, queue writes.
- **Plan for failure.** Every arrow can time out. Every node can die. Say what happens.
- **Idempotency and at-least-once** show up in almost every task. Duplicates will happen; design so they are harmless.

---

## Toolbox: the building blocks

One line each. The task files explain the ones they use in depth.

| Block | What it is | Reach for it when |
|---|---|---|
| **DNS / CDN** | Name resolution; globally distributed cache for static or cacheable content. | Static files, images, public read-mostly API responses. |
| **Load balancer** | Spreads traffic across stateless servers. L4 (TCP) or L7 (HTTP, can route by path/header). | More than one app server. Always. |
| **API gateway** | A load balancer that also does auth, rate limiting, routing, TLS termination. | Many services behind one public entry point. |
| **Stateless app servers** | Your Go services. No session state in memory, so any server can answer any request. | Horizontal scaling: add servers behind the LB. |
| **Relational DB** (Postgres) | Tables, joins, transactions, strong consistency, indexes. The default. | Almost everything, until a number says otherwise. |
| **Key-value / wide-column store** (DynamoDB, Cassandra) | Partitioned by key, scales writes horizontally, limited queries, tunable consistency. | Simple access by key at very high write volume. |
| **Cache** (Redis, Memcached) | In-memory key-value, sub-millisecond, data can be lost. Redis adds data structures (sorted sets, hashes, counters). | Hot reads, counters, leaderboards, rate limits, sessions. |
| **Message queue** (RabbitMQ, SQS) | Buffer between producer and consumer; a message is consumed once then gone. | Decouple, absorb bursts, retry work later. |
| **Event log** (Kafka) | Append-only partitioned log; many consumers read the same events; replayable. | Fan-out to many consumers, event sourcing, streams. |
| **Object storage** (S3) | Cheap, durable, unlimited blobs addressed by key. Not a file system, not a database. | Files, images, backups, anything over a few hundred KB. |
| **Search index** (Elasticsearch) | Inverted index for full-text and faceted search. | "Search" as a feature. |
| **Scheduler / jobs** | Runs work later or on a schedule. | Cleanup, retries, digests, reports. |
| **Coordination service** (etcd, ZooKeeper) | Small strongly-consistent store for leader election, leases, config. | Exactly one node may do X. |
| **Observability** | Logs, metrics, traces, alerts. | Every design. Mention it in the wrap-up. |

Two ideas that are not boxes but appear everywhere:

- **Replication** copies data to more nodes: for reads (read replicas) and for availability (failover). Costs: lag, consistency.
- **Partitioning / sharding** splits data across nodes by a key. Costs: no cross-shard joins or transactions, hot shards, resharding.

---

## Estimation cheat sheet

### Time

| Period | Seconds | Use |
|---|---|---|
| 1 day | 86,400 ≈ **10⁵** | *daily volume ÷ 100,000 = per second* |
| 1 month | 2.6 × 10⁶ | |
| 1 year | 3.15 × 10⁷ | |

Daily → per second: **1M/day ≈ 12/s · 10M/day ≈ 115/s · 100M/day ≈ 1,160/s · 1B/day ≈ 11,600/s.**
Peak is usually **2–5× average**. Design for peak.

### Sizes

| Thing | Size |
|---|---|
| ASCII char | 1 B · UTF-8 text: 1–4 B per char |
| `int64`, timestamp | 8 B |
| UUID | 16 B binary, 36 chars as text |
| URL, email, name | ~100 B |
| A short text post | ~300 B |
| A row with 10 small columns + indexes | ~1 KB (rule of thumb) |
| Web image | 200 KB – 2 MB |
| 1 minute of 1080p video | ~100 MB |

Powers of two: 2¹⁰ ≈ 1 thousand · 2²⁰ ≈ 1 million · 2³⁰ ≈ 1 billion · 2³² ≈ 4.3 billion · 2⁴⁰ ≈ 1 trillion.
Base62 strings: 62⁶ ≈ 57 billion · 62⁷ ≈ 3.5 trillion · 62⁸ ≈ 218 trillion.

### Latency (orders of magnitude, not exact)

| Operation | About |
|---|---|
| Main memory reference | 100 ns |
| Mutex lock/unlock | 20 ns |
| SSD random read | 100 µs |
| Round trip inside a datacenter | 0.5 ms |
| Redis `GET` including network | 0.2–1 ms |
| Postgres indexed point query | 1–5 ms |
| HDD seek | 10 ms |
| Round trip across an ocean | 100–150 ms |
| Human notices | > 100 ms |

### Single-node rules of thumb

| Component | Roughly |
|---|---|
| Postgres, decent box | a few thousand writes/s, 10k+ indexed reads/s, a few TB comfortably |
| Redis | ~100k simple ops/s on one core; data must fit in RAM |
| One Go HTTP server | tens of thousands of trivial requests/s per core; usually the DB is the limit |
| Kafka partition | tens of MB/s |
| S3, one prefix | 3,500 writes/s, 5,500 reads/s (scales by adding prefixes) |

### Availability

| Target | Downtime per year |
|---|---|
| 99.9 % | 8.8 h |
| 99.99 % | 53 min |
| 99.999 % | 5.3 min |

Two components in series each at 99.9 %: about 99.8 %. Redundant in parallel: far better. Fewer
serial hops and redundancy at each hop is how availability is actually bought.

---

## Task index

### 🟢 Easy — [easy/](easy/)

One service, a handful of boxes, one or two decisions that are genuinely interesting. Each
introduces a building block or pattern that later tasks reuse.

| # | Task | What it teaches |
|---|---|---|
| 01 | [URL Shortener](easy/01-url-shortener.md) | the read-heavy baseline: key generation, base62, cache-aside, 301 vs 302 |
| 02 | [Pastebin](easy/02-pastebin.md) | metadata vs blob, object storage, expiry & cleanup, unguessable keys |
| 03 | [Rate Limiter](easy/03-rate-limiter.md) | token bucket vs windows, where the limiter lives, Redis + Lua, fail open |
| 04 | [Unique ID Generator](easy/04-unique-id-generator.md) | UUID v4/v7, Snowflake bit layout, ticket servers, clocks |
| 05 | [In-Memory Cache with TTL](easy/05-kv-cache-ttl.md) | LRU with map + list, lazy vs active expiry, sharding a lock |
| 06 | [View & Like Counter](easy/06-view-like-counter.md) | the hot-row problem, write-behind, local batching, sharded counters, HLL |
| 07 | [Leaderboard](easy/07-leaderboard.md) | sorted sets, top-K and rank, periodic boards, tie-breaking |
| 08 | [Uptime Monitor](easy/08-uptime-monitor.md) | scheduling at scale, worker pools, multi-region agreement, time-series storage |
| 09 | [Feature Flags](easy/09-feature-flags.md) | evaluate locally, snapshot distribution, sticky percentage rollout |
| 10 | [Webhook Delivery](easy/10-webhook-delivery.md) | outbox, at-least-once, backoff & dead letters, HMAC, SSRF |
| 11 | [Image Upload & Hosting](easy/11-image-upload-hosting.md) | presigned URLs, async processing, immutable URLs on a CDN |
| 12 | [Notification Service](easy/12-notification-service.md) | 202 + queue, priority lanes, provider adapters & failover, idempotency |
| 13 | [Shopping Cart](easy/13-shopping-cart.md) | guest vs user carts, merge, PUT semantics, price snapshots |
| 14 | [Polls & Voting](easy/14-polls-voting.md) | one vote per user atomically, counters vs rows, result caching |
| 15 | [Job Scheduler](easy/15-job-scheduler.md) | `SKIP LOCKED` dispatch, leases, at-least-once, cron & DST |

#### 16–60 · Seeded from honeydolist-site, designed for scale

Tasks 16–60 were mined from the `honeydolist-site` project (a small handyman-business site: Vue + Go + Postgres + NATS). Each file opens with a 🌱 **Seed** line saying what that project really does (with its PLAN step / migration / ADR) and a **Here, at scale** line saying what the same idea becomes at millions of users. Where the project is simpler than the lesson, or differs from it, the Seed line says so.

**Intake & abuse** — How a public endpoint stays correct and safe when the internet is hostile and mobile networks are bad.

| # | Task | What it teaches |
|---|---|---|
| 16 | [Public Form Intake](easy/16-public-form-intake.md) | client-made idempotency keys, replay semantics, jittered retries & load shedding, outbox vs broker, drafts |
| 17 | [Abuse & Spam Protection](easy/17-abuse-protection.md) | who counts as a person, trusted-proxy client IP, layered defence, fake 201 for bots, spam as a flag, live-tunable limits |
| 18 | [Inbound Webhook Receiver](easy/18-inbound-webhook-receiver.md) | durable accept then ack, inbox dedupe, reconcile-by-pull, signature + replay window, ordering |
| 19 | [Moderated Submissions](easy/19-moderated-submissions.md) | quarantine bucket, re-encode & decode budget, moderator queue & risk lanes, anonymous rate limiting, takedown |

**Records & integrity** — The plumbing under every admin app: numbers, presence, history, lists.

| # | Task | What it teaches |
|---|---|---|
| 20 | [Per-Tenant Document Numbering](easy/20-document-numbering.md) | key vs public id vs number, counter rows & allocation blocks, hot-row ceiling, gap-free invoices |
| 21 | [Presence & Soft Edit Locks](easy/21-presence-soft-locks.md) | Redis TTL presence, heartbeat cost math, advisory lock + version check, takeover & idle expiry |
| 22 | [Audit Log & Revision History](easy/22-audit-log-revisions.md) | diff vs snapshot, partitioned append-only log, same-transaction capture, restore, redaction, hash chain |
| 23 | [Paginated Lists at a Billion Rows](easy/23-paginated-lists.md) | keyset vs offset, capped & maintained counts, partial indexes for filter + sort, export limits |

**Lifecycle & workflow** — What happens to a record over its life: kept, merged, moved, due, deleted.

| # | Task | What it teaches |
|---|---|---|
| 24 | [Data Lifecycle: Archive, Delete, Erase](easy/24-data-lifecycle.md) | archive vs remove vs purge vs anonymise, batched purge & autovacuum, erasure ledger across stores |
| 25 | [Contact Deduplication & Merge](easy/25-contact-dedupe-merge.md) | unique email vs hint keys, E.164 phones, race-safe find-or-create, blocking vs O(n²), tombstone merge & undo |
| 26 | [Configurable Status Workflow](easy/26-status-workflow.md) | stages in code vs tenant statuses as rows, strict transitions & guards, conditional UPDATE + history + outbox |
| 27 | [Deadlines & Escalation](easy/27-deadlines-escalation.md) | rolling `next_fire_at`, `due_version` idempotency, escalation ladder × preferences, time zones & DST, 09:00 jitter |

**Booking & commerce** — Money-adjacent designs where correctness beats speed.

| # | Task | What it teaches |
|---|---|---|
| 28 | [Booking Calendar for Rentable Units](easy/28-booking-calendar.md) | range exclusion constraint, pooled units & fragmentation, holds with TTL, blocks, month-view reads |
| 29 | [Price & Availability Engine](easy/29-price-availability-engine.md) | dated prices, staged campaigns with a one-row switch, derived availability, stock ledger, hidden cost |
| 30 | [Invoicing: Money, Tax & PDF](easy/30-invoicing.md) | integer money & rounding, dated tax, freeze at issue, idempotent payments, lazy PDFs, billing runs |
| 31 | [Service Area & Geocoding](easy/31-service-area-geocoding.md) | geocoding cache & cost, postal lists vs polygons vs cells, haversine vs road, autocomplete, provider interface |

**Content & SEO** — Serving pages to people and to crawlers at 50M-URL scale.

| # | Task | What it teaches |
|---|---|---|
| 32 | [Block-Based CMS](easy/32-block-cms.md) | typed JSON blocks, versioned registry, sanitised rich text, publish as pointer flip, tag purge, category trees |
| 33 | [Sitemaps & Crawl Notifications](easy/33-sitemaps-crawl-notify.md) | sitemap index limits, shard rebuild math, `lastmod` from content hash, IndexNow coalescing, robots hygiene |
| 34 | [Crawler-Ready SPA Serving](easy/34-crawler-ready-spa.md) | head + crawler body vs SSR/prerender, bot verification, 3-tier HTML cache, honest status codes, one model → HTML/JSON-LD/Markdown |
| 35 | [Redirect Manager & 404 Tracker](easy/35-redirect-manager.md) | Bloom → packed table → atomic snapshot, loop & chain handling, 301 vs 302 caching, Space-Saving 404 tracking |

**AI & search** — Putting a model or an index behind a product without losing control of cost, safety or permissions.

| # | Task | What it teaches |
|---|---|---|
| 36 | [LLM Gateway](easy/36-llm-gateway.md) | one chokepoint, layered kill switches, reserve-then-settle token budgets, call log, caching & routing, failover |
| 37 | [In-App Records Assistant](easy/37-records-assistant.md) | tool-calling vs text-to-SQL, permissions in the tool, prompt injection from stored data, golden-question evals |
| 38 | [AI Draft Queue](easy/38-ai-draft-queue.md) | backfill as a job queue, drafts that never overwrite human text, approve with a version check, review capacity |
| 39 | [Global Search with Permission Filtering](easy/39-global-search.md) | Postgres FTS vs a dedicated engine, versioned idempotent indexing, alias-swap reindex, ACL pre-filter, id & typo queries |

**Platform & operations** — Keeping a running system safe: media, backups, secrets, deploys.

| # | Task | What it teaches |
|---|---|---|
| 40 | [Media Reprocessing at Scale](easy/40-media-reprocessing.md) | originals as truth, key = f(original, recipe), range-task backfills, priority lanes, video leases, pointer rollback |
| 41 | [Backup & Restore](easy/41-backup-restore.md) | RPO/RTO per failure class, logical vs physical + WAL (PITR), key custody, immutable off-site copies, restore drills |
| 42 | [Integration Credentials Vault](easy/42-credentials-vault.md) | root key → per-tenant KEK, AES-GCM with AAD, rotation without downtime, write-only API, secrets broker |
| 43 | [Zero-Downtime Deploys & Schema Migrations](easy/43-zero-downtime-deploys.md) | expand/contract, what locks what in Postgres, `lock_timeout` retry, canary + rolling, graceful drain, compat windows |

**Comms, access & analytics** — Telling people things, deciding who may do things, counting what happened.

| # | Task | What it teaches |
|---|---|---|
| 44 | [Bulk Email Campaign Sender](easy/44-email-campaign-sender.md) | daily caps & hourly pace, per-zone cursors, at-most-once + reconcile, suppression, warm-up, fairness |
| 45 | [Live In-App Notifications](easy/45-live-notifications.md) | SSE + web push, doorbell-not-data, per-user bus subjects, inbox replay & resync, routing & escalation |
| 46 | [Roles & Permissions (RBAC)](easy/46-rbac.md) | permissions not roles, escalation guards, versioned permission cache, bitsets, deny-by-default |
| 47 | [Privacy-Friendly Web Analytics](easy/47-web-analytics.md) | beacon ingest, salted daily-hash visitors, HLL, replay-safe counters, rollups, business-day DST |

**Tooling & settings** — The internal platform: moving data in, testing load, watching, configuring.

| # | Task | What it teaches |
|---|---|---|
| 48 | [Bulk Import with Preview & Undo](easy/48-bulk-import.md) | stage → preview → chunked commit, idempotent upsert, hash-guarded undo, fair scheduling, hostile files |
| 49 | [Load Testing Harness](easy/49-load-testing-harness.md) | open vs closed load, coordinated omission, mergeable histograms, generator fleet, regression gate |
| 50 | [Alerting Engine](easy/50-alerting-engine.md) | rule evaluation at scale, alert state machine, flapping, grouping & inhibition, outbox delivery, watchdog |
| 51 | [Runtime Settings & Timed Kill Switches](easy/51-runtime-settings.md) | settings registry, in-process snapshots (NOTIFY + feed poll), `off_until` switches, env > tenant > plan > default, audit |

**Forms, push & landing pages** — Product surfaces that tenants configure, devices that get pinged, pages that must earn their place.

| # | Task | What it teaches |
|---|---|---|
| 52 | [Configurable Forms](easy/52-configurable-forms.md) | tenant-owned fields, stable ids, one validator per side with golden vectors, JSONB + version, immutable versions, CDN schema |
| 53 | [Web Push & the Installable App (PWA)](easy/53-web-push-pwa.md) | push subscription per device, VAPID, 5M-in-10-min fan-out, `404/410` cleanup, collapse keys, consent, service-worker traps |
| 54 | [Programmatic Location Pages](easy/54-programmatic-location-pages.md) | candidate matrix, versioned quality gates + `live` CHECK, variable-part simhash, tiers, waves & stop-loss |

**Timelines, links & events** — Entity-level features that look small and hide a real decision each.

| # | Task | What it teaches |
|---|---|---|
| 55 | [Order Timeline with Photo Steps](easy/55-order-timeline.md) | timeline vs audit log, text-first photo capture, device-minted ids, revocable tracking link, system milestones |
| 56 | [Related Content & Interlinking](easy/56-related-content-interlinking.md) | curated > text > computed edges, materialised top-N, link rot, in-memory graph health, baked-in blocks |
| 57 | [Event Registration with Capacity & Waitlist](easy/57-event-registration-waitlist.md) | counter row + CHECK, holds & state machine, first-that-fits waitlist, waiting room + gate, refunds & drift |

**Tracking, adapters & time** — Cross-cutting plumbing: counting clicks you can bill for, third parties you can test, days that have 23 hours.

| # | Task | What it teaches |
|---|---|---|
| 58 | [Outbound Link Click Tracking](easy/58-outbound-click-tracking.md) | 302 via a first-party endpoint, signed click ids, ledger vs hot counter, versioned fraud verdicts, postbacks |
| 59 | [Provider Adapters with Fake Drivers & Chaos Testing](easy/59-provider-adapters-fakes.md) | narrow interface, four error kinds, fake/stub/sandbox ladder, fate-by-input chaos, fail-closed guard |
| 60 | [Business-Day Time Zones & DST in Reports](easy/60-business-day-time-zones.md) | five kinds of time, half-open day windows (23/25 h), 15-minute UTC grid, zone changes, DST golden tests |


#### Easy backlog

Empty: every idea found in honeydolist-site is now a task (16–60). Other parts of that project (or other projects) can be mined the same way: name one and it becomes `easy/NN-slug.md` with a 🌱 Seed line.

### 🟡 Medium — [medium/](medium/)

Backlog in [medium/README.md](medium/README.md). Not written yet.

### 🔴 Hard — [hard/](hard/)

Backlog in [hard/README.md](hard/README.md). Not written yet.

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want another task? Name it and it gets a file
> in the right folder, following the template.

---

## Adding a task

1. Copy [_template.md](_template.md) to `easy/NN-slug.md` (or medium/hard), next free number.
2. Fill all 14 sections. Keep the task statement short and vague, the way interviewers do.
3. Add a row to the index above and a tick line to [PROGRESS.md](PROGRESS.md).
4. Link it from the previous task's `Next:` line.

To **extend** an existing task: add a `9.x` deep dive, a question to `12. Check yourself`, or an
idea to `14. Extensions`. Don't rewrite what's there; append.
