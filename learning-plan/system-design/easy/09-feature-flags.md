# 09 · Feature Flags

`🟢 easy` · *Topics:* local evaluation, snapshot distribution (poll vs push), deterministic percentage rollout via hashing, kill switches, configuration as versioned data · *Builds on:* [05 Cache](05-kv-cache-ttl.md), [04 IDs](04-unique-id-generator.md) (hashing)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [08 Uptime Monitor](08-uptime-monitor.md) · Next: [10 Webhook Delivery](10-webhook-delivery.md)

---

## 1. The task

> "Design a feature flag service. Engineers turn features on and off, roll them out to 10% of
> users, or target specific accounts. Our applications evaluate flags on every request."

The service that must never be on the request path of the services that use it. Getting that
one idea right is most of the design.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| How many flags, how many services and instances? | Snapshot size and distribution fan-out. | ~1,000 flags; 50 services; ~2,000 instances. |
| Evaluations per second? Latency budget per evaluation? | Decides local vs remote evaluation. | Millions/s fleet-wide; must be microseconds. |
| How fast must a change propagate? | Polling vs push. | Normal changes within 30 s; a kill switch within seconds. |
| Targeting rules? | Rule model. | On/off, percentage of users, allow/deny lists by user or account, attribute match (country, plan). |
| Must a user stay in the same bucket? | Determinism of percentage rollout. | Yes: a user in the 10% stays in when it becomes 20%. |
| Flag types? | Booleans only or multivariate (strings, numbers, JSON)? | Boolean + string variants (A/B). |
| Environments? Audit? | Separate rules per env, who changed what. | dev/staging/prod; full audit log. |
| If the flag service is down? | Failure behaviour of every app. | Apps keep working with the last snapshot or defaults. Never block. |
| Client-side (browser/mobile) flags? | Security of rules and identities. | Later; server-side first. |

## 3. Requirements

**Functional**
- CRUD flags with rules per environment; versioned, audited.
- SDK: `enabled(flag, context)` / `variant(flag, context)` evaluated in the application.
- Percentage rollouts are deterministic per user; targeting by user, account, attributes.
- Changes propagate within 30 s; kill switch within a few seconds.

**Non-functional**
- Evaluation is a pure in-memory function, microseconds, no I/O.
- The flag service being unavailable has no effect on running applications.
- Snapshot distribution scales to thousands of instances cheaply.

**Out of scope**
- Experiment analytics, approval workflows, client-side SDKs (extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Snapshot size | ~500 KB | 1,000 flags × ~500 B of rules (JSON); ~50 KB gzipped |
| Poll load | ~70 req/s | 2,000 instances ÷ 30 s; almost all `304 Not Modified` |
| Change rate | ~50/day | humans editing flags |
| Evaluations | millions/s | local; costs nothing to the service |
| Push connections (if SSE) | 2,000 long-lived | one small server handles it |

What the numbers tell us: the service itself is tiny. The *only* way to make it expensive is to
put it on the request path with a network call per evaluation. Everything flows from "evaluate
locally against a snapshot".

## 5. Plan

1. The architectural rule: evaluation is local; the service distributes rules.
2. The rule model and a deterministic evaluation function.
3. Distribution: polling with ETags, push for urgency, CDN for scale.
4. Sticky percentage rollout via hashing.
5. Storage, versioning, audit.
6. Failure modes: what the SDK does when it can't reach anything.

## 6. High-level design

```
 engineer ──► Flag UI/API ──► Postgres (flags, rules, versions, audit)
                                 │ on publish: compile env snapshot → version N (JSON)
                                 ▼
                           snapshot store (object storage / Redis) + CDN
                                 │                               ▲
                                 │ SSE "version N available"      │ GET /snapshot/prod  If-None-Match: "N-1"
                                 ▼                               │
 ┌─ application instance ─────────────────────────────────────────┴───────────────────┐
 │ SDK: atomic pointer to the current snapshot                                        │
 │      background: poll every 30 s (ETag) + SSE listener → swap pointer on change    │
 │      Enabled("new-checkout", ctx{user:42, country:"DE", plan:"pro"}) → bool  (µs) │
 │      fallback: last snapshot on disk → code defaults                               │
 └────────────────────────────────────────────────────────────────────────────────────┘
```

Publishing a change compiles the environment's flags into one immutable snapshot with a version.
SDKs hold the current snapshot in memory and swap it atomically when a newer version arrives
(by poll or push). Evaluation reads the snapshot and never blocks.

## 7. API

Management:

```
POST  /flags                  { "key": "new-checkout", "type": "boolean", "description": "…" }
PUT   /flags/{key}/envs/prod  { "enabled": true, "default": false,
                                "rules": [
                                  { "if": { "account_id": { "in": ["acme","globex"] } }, "serve": true },
                                  { "if": { "country": { "in": ["DE","AT"] } }, "serve": { "percentage": 10 } }
                                ] }                                         → 200 { "version": 418 }
GET   /flags/{key}/history                                                    → audit entries
POST  /flags/{key}/envs/prod/kill                                             → off for everyone, now
```

Distribution (what SDKs call):

```
GET /sdk/snapshot/prod         Authorization: SDK key
  200  ETag: "418"  Cache-Control: max-age=5    body: { "version": 418, "flags": { … } }
  304  Not Modified
GET /sdk/stream/prod           text/event-stream → "event: version\ndata: 419\n\n"   (then the SDK fetches)
```

SDK:

```go
flags.Enabled("new-checkout", flags.Context{UserID: "42", Attrs: map[string]string{"country": "DE", "plan": "pro"}})
flags.Variant("checkout-button-color", ctx) // "blue" | "green" | default
```

## 8. Data model

```sql
CREATE TABLE flags (
    key          TEXT PRIMARY KEY,
    type         TEXT NOT NULL,          -- boolean | string
    description  TEXT,
    archived     BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE flag_configs (              -- one row per (flag, env), current rules
    flag_key     TEXT REFERENCES flags(key),
    env          TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL,
    default_val  JSONB NOT NULL,
    rules        JSONB NOT NULL,         -- ordered list; first match wins
    version      BIGINT NOT NULL,        -- bumps on every change
    PRIMARY KEY (flag_key, env)
);
CREATE TABLE flag_audit (
    id           BIGSERIAL PRIMARY KEY,
    flag_key     TEXT, env TEXT,
    actor        TEXT NOT NULL,
    before       JSONB, after JSONB,
    at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE snapshots (                 -- immutable compiled artefacts
    env          TEXT NOT NULL,
    version      BIGINT NOT NULL,
    body         JSONB NOT NULL,         -- or an object-storage key
    created_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (env, version)
);
```

## 9. Deep dives

### 9.1 Evaluate locally, always

| Model | Per-evaluation cost | Behaviour when the service is down | Verdict |
|---|---|---|---|
| remote call per evaluation (`GET /evaluate?flag=…&user=…`) | a network round trip (~1 ms) × millions/s; the flag service must be as available as everything combined | every request in every service fails or slows | never |
| **local snapshot + pure function** | microseconds, no I/O | applications continue on the last snapshot | yes |

The service's job is to **distribute rules**, not to answer questions. The SDK is a small
rules engine: `evaluate(snapshot, flagKey, context) → value`. It must be deterministic (same
inputs, same output) and total (always returns something: the default on any error).

### 9.2 The rule model

A flag config is: `enabled` (master switch), an ordered list of `rules`, and a `default`.
Evaluation: if `!enabled` → `default`; else the **first rule whose conditions all match** decides;
no rule matched → `default`. A rule serves either a fixed value or a percentage split.

Conditions are attribute predicates: `in`, `not_in`, `eq`, `starts_with`, `semver_gte`, and so
on. Keep the set small and well-defined; every operator must be implemented identically in every
SDK language, which is why fewer is better.

The **kill switch** is `enabled = false` on the config: one field, evaluates to the default, no
rule evaluation, no chance of a buggy rule keeping the feature on.

### 9.3 Deterministic percentage rollout

"10% of users" must mean the *same* 10% on every instance, on every request, today and tomorrow,
and those users must remain inside when the rollout grows to 20%.

```
bucket = hash(flagKey + ":" + userID) mod 10000       // 0..9999, uniform
in     = bucket < percentage × 100                     // 10% → bucket < 1000
```

- **Deterministic:** no randomness at evaluation time, so no storage of assignments is needed.
- **Sticky under growth:** 20% is `bucket < 2000`, a superset of `< 1000`.
- **Per-flag salt:** including `flagKey` in the hash means a user who is in the first 10% of one
  flag isn't in the first 10% of every flag (otherwise "the 10% users" always get every experiment).
- **Hash choice:** a fast non-cryptographic hash with good distribution (MurmurHash3, xxHash,
  FNV-1a for simplicity). The same algorithm, byte for byte, in every SDK language, with a
  shared test vector file; an off-by-one in one SDK silently shifts assignments.
- **Bucketing key:** the user ID for user-level rollouts; the account ID when a whole company
  must see the same thing; a stable anonymous ID (cookie) for logged-out traffic.
- Multivariate: split the 10,000 buckets into ranges per variant (`0–4999` blue, `5000–9999` green).

### 9.4 Distribution: how a change reaches 2,000 instances

| Mechanism | Propagation | Cost | Notes |
|---|---|---|---|
| **Polling with ETag** every 30 s | ≤ 30 s | ~70 req/s, nearly all `304` | dumb, robust; works through any proxy |
| **SSE / long-poll push** of "new version" events | ~1 s | 2,000 idle connections | the SDK then fetches the snapshot; reconnect with backoff; falls back to polling |
| **Streaming the full ruleset** over WebSocket | ~1 s | more complex state sync | what hosted vendors do; not needed here |
| CDN in front of the snapshot endpoint | poll interval + CDN TTL (worst case) | offloads the origin | each version's body is immutable, but `/snapshot/prod` is a moving pointer: with `max-age=30` and a 30 s poll the worst case is ~60 s. Use a short CDN TTL (~5 s) or purge on publish; the origin is cheap |

**Choice: poll + push.** Polling is the baseline that always works; the SSE channel makes the
kill switch fast. The SDK treats a push as "poll now". Snapshots are compiled once per publish,
not per request, so the origin serves a static blob.

Version monotonicity: the SDK only ever swaps to a *higher* version, so a stale CDN node can't
roll an instance backwards.

### 9.5 Storage, versions, audit

- The management database holds the editable state and a full audit trail (who, when, before,
  after). Flags are configuration that changes production behaviour; treat edits like deploys.
- **Publish** compiles `flag_configs` for an environment into a snapshot with a monotonically
  increasing version and stores it immutably. Rollback is "publish version N−1's body as N+1".
- Environments are fully separate snapshots. Promotion copies rules dev → staging → prod explicitly.
- Stale flags are the long-term cost: track `last_evaluated_at` per flag (SDKs report usage
  counts periodically) and nag owners to delete flags that are 100% on.

### 9.6 Failure modes in the SDK

Order of fallbacks when fetching a snapshot fails:

1. Keep the current in-memory snapshot (nothing changes; log and emit a metric).
2. On startup with no network: load the last snapshot the SDK persisted to local disk.
3. No disk cache either: the **code defaults** passed at each call site (`Enabled(key, ctx, false)`).

The SDK must **never block application startup or a request** on the flag service, and must
initialise in the background with a short bounded wait at most. Ship a metric for snapshot age;
an instance running a 2-hour-old snapshot is the problem to alert on.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** nothing meaningful. 20,000 instances polling every 30 s is 700 req/s
  of `304`s; put the snapshot behind a CDN and the origin sees only revalidations.
- **Snapshot growth:** 10,000 flags with large allow-lists (10k user IDs each) make the snapshot
  tens of MB. Move large lists to **segments** fetched separately and cached, or bucket by hash
  instead of listing users.
- **Service down for hours:** apps keep serving the last snapshot; no new changes propagate.
  Acceptable by requirement; the management UI is the only thing unavailable.
- **A bad rule (e.g. targets everyone):** that's a deploy-class incident. Mitigation is process:
  staged environments, audit, kill switch, and rollback by republishing the previous version.
- **Consistency across services:** a request touching three services may see versions N and N+1
  for a few seconds. Usually fine. When it isn't (a flag that changes a wire format), evaluate
  once at the edge and pass the decision in a header/context.
- **Secrets in rules:** rules may contain customer IDs and strategy; server-side only. Client-side
  flags (browser) need a separate, filtered snapshot and per-user evaluation on the server.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| local evaluation from a snapshot | remote evaluate API | µs evaluations, zero coupling to the service's uptime | propagation delay; each SDK language reimplements the engine |
| hash bucketing | stored assignments | stateless, sticky, no per-user storage | changing the bucketing key re-randomises everyone |
| poll + SSE | poll only | seconds-fast kill switch | long-lived connections to manage |
| immutable versioned snapshots | reading rules live from the DB | cheap, cacheable, roll-back-able | a publish step |
| first-match ordered rules | weighted/scored rules | predictable, easy to reason about | ordering is a thing users must get right |

## 12. Check yourself

1. Why must evaluation never be a network call, even though the flag service is tiny?
2. User 42 is in the 10%. The rollout goes to 20%. Why is 42 still in, and what change *would* kick them out?
3. Why salt the hash with the flag key?
4. The flag service has been down for an hour. What do running apps see? What do newly started apps see?
5. Why compile snapshots instead of letting SDKs read rules straight from the database?

<details><summary>Answers</summary>

1. Evaluations happen millions of times a second across the fleet; a round trip each would make
   the flag service the busiest and most critical system in the company, and its outage would be
   everyone's outage. A local pure function over a cached snapshot costs microseconds and keeps
   working when the service is down.
2. Membership is `hash(flag:user) mod 10000 < percentage × 100`; 20% uses a larger threshold, a
   superset of the 10% set, so every bucket below 1,000 is still below 2,000. Changing the hash
   function, the salt, or the bucketing key (user ID → account ID) reshuffles everyone.
3. Without it the same users land in the lowest buckets for every flag, so "the 10%" always get
   every experiment and every risky rollout at once, and experiments contaminate each other.
4. Running apps keep evaluating the last snapshot they fetched; no changes propagate, nothing
   fails. Newly started apps load the last snapshot persisted on disk, or if there is none, fall
   back to code defaults. Alert on snapshot age.
5. A snapshot is an immutable artefact with a version: cheap to serve (static, CDN-cacheable,
   ETag-friendly), atomic (an SDK never sees half a change), and roll-back-able. Live reads would
   put the database on the fetch path and expose partially edited rule sets.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) example 17 (`atomic.Pointer[T]` snapshot swap) and [48 Low-latency III](../../48-low-latency-lockfree-tail.md) (copy-on-write), [58 Realtime: WebSockets & SSE](../../58-realtime-websockets-sse.md) (the push channel), [41 API design](../../41-api-design-evolution.md) (ETag/304), [11 Interfaces](../../11-interfaces.md) and [30 Behavioral patterns](../../30-patterns-behavioral.md) (condition operators as a small strategy table).
- The SDK core is ~150 lines: `atomic.Pointer[Snapshot]`, a `Fetch(ctx) (*Snapshot, changed bool, err)` with `If-None-Match`, a poller goroutine with jittered ticker, an SSE reader that triggers a fetch, and `Evaluate` with `hash/fnv` bucketing. Table-test the bucketing with fixed vectors.
- Packages: `hash/fnv` or `github.com/cespare/xxhash/v2`, `net/http`, `encoding/json`, `sync/atomic`. Look at OpenFeature's Go SDK for the standard interface shape.

## 14. Extensions

- **Experiments:** record exposures (`user 42 saw variant blue`) to an analytics pipeline; sample to keep volume sane; tie to task 06's counters for results.
- **Segments:** named reusable audiences (`beta-testers`) referenced by many flags; fetched and cached separately.
- **Prerequisites:** flag B only evaluates if flag A is on; detect cycles at publish time.
- **Scheduled changes** (turn on at 09:00 Monday) via task 15.
- **Approval workflow** for prod changes, and diffing two versions in the UI.
- **Client-side SDK:** evaluate on the server per user and ship only resolved values to the browser.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). This is a tiny-hardware service: a few small instances for availability, not for load.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Flag API (Go, stateless): snapshot gateway + SSE + management/publish | 70 polls/s (2,000 ÷ 30 s, nearly all `304`); 2,000 SSE connections; publish stampede: a kill switch makes 2,000 instances refetch within ~2 s = 1k req/s of a 50 KB gzipped body | CPU, but ~0 | polls 70 × 0.1 ms = 0.007 vCPU; stampede 1k × 0.3 ms = 0.3 vCPU (the real peak); SSE keep-alives ≈ 0. Memory: 2,000 × ~20 KiB = 40 MiB + the 500 KB snapshot in RAM. N = ceil(0.3 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | the zone rule sets the count, not the load: ~0.3 vCPU of 6 is 5 % busy. `c` because it is CPU and bytes, never memory; the stampede is ≈ 50 MB/s ≈ 0.4 Gbit/s in total (≈ 0.13 per node), far under what a `large` can burst. The latest snapshot lives in each node's memory, so no Redis is needed |
| Postgres (flags, flag_configs, flag_audit, snapshots) | ~50 writes/day; ~1,000 flags × 500 B = 0.5 MB current; snapshots 50/day × 500 KB ≈ 25 MB/day ≈ 9 GB/year | none (availability only) | RAM and IOPS are irrelevant: the whole current state is 0.5 MB, and 50 writes a day is ~0 IOPS. 20 GB `gp3` | `db.m7g.large` (2 vCPU / 8 GiB), RDS Multi-AZ | 1 primary + standby | the smallest non-burstable class is 1,000× the load. Multi-AZ because the kill switch is an emergency tool and publishing needs the database; single-AZ would be half the line |
| Snapshot history (S3) | ≈ 18k immutable versions a year × ~50 KB gzipped ≈ 1 GB | nothing to size | | S3 Standard | | pennies: $0.023 × 1 GB |

Network: 50 changes/day × 2,000 instances × 50 KB = 5 GB/day (150 GB/month), plus 70 polls/s × ~0.5 KB of `304` headers ≈ 90 GB/month: ≈ 240 GB, about $20 even priced as internet egress (in-region it is cheaper).

Where the real cost would be: the rejected design. Evaluations are millions/s; assume 3M/s. Locally they cost ~1 µs each, ≈ 3 vCPU spread over 2,000 instances. A remote call at ~0.15 ms CPU each would need 3M × 0.15 ms = 450 vCPU, N = ceil(450 ÷ 4.27) = 106 → 108 × `c7g.2xlarge` ≈ $22,900 a month, before the network.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Flag API: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.m7g.large` ≈ $120, Multi-AZ doubles; 20 GB gp3 ≈ $3 | $240 |
| Internal ALB: $16 + ~1 LCU × $0.008 × 730 | $20 |
| Data transfer ≈ 240 GB × $0.09 (upper bound) | $20 |
| S3 history | ~$0 |
| **Total** | **≈ $440** (≈ $300 with 1-year commitments on the $400 of servers) |

- **Biggest line and why:** the management Postgres (55 %), a floor price: Multi-AZ for a database that takes 50 writes a day. The whole service costs about two `c7g.2xlarge`; the design's real saving is the ≈ $22,900 a month of remote-evaluation servers it never needs.
- **What changes at 10×:** 20,000 instances ⇒ 667 polls/s ≈ 0.07 vCPU and 20,000 SSE connections ≈ 0.4 GB spread over the same 3 × `c7g.large`. The one change is the publish stampede (20k × 50 KB = 1 GB in seconds ≈ 4 Gbit/s): serve snapshot bodies from S3 + CloudFront and let the origin answer revalidations (data ≈ 2.4 TB ≈ $200 a month). The first limit hit is snapshot size (section 10: segments), not hardware.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Tiny: ~1,000 flags × ~500 B of rules ≈ 0.5 MB of current state, ~50 writes a day, snapshots ~25 MB/day ≈ 9 GB a year, one audit row per change (sections 4, 15).
- Access is by key (`flag_key, env`) and whole-document: rules are written and read as one JSON value and never queried inside. The audit log is append-only.
- Evaluation never touches a database: SDKs hold a snapshot in memory (section 9.1), and the snapshot gateway serves a static versioned blob (~70 polls/s, nearly all `304`).
- Publishing must be atomic: the new rules, the audit row and the new snapshot version change together (section 9.5).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | one transaction covers `flag_configs`, `flag_audit` and `snapshots`; `jsonb` holds the rules, `before`/`after` and the snapshot body as whole documents; the `(env, version)` primary key rejects a concurrent duplicate publish. At 0.5 MB any instance is oversized (section 15) |
| MySQL (RDS / Aurora) | ✔ works, nothing material is lost | its `JSON` type stores the same documents, and no GIN index is needed because nothing queries inside the rules; the same transactions, primary keys and row locks |
| DynamoDB | ✔ works, not needed | items are tiny (far under 400 KB), access is by key, ~50 writes a day. It would remove the ≈ $240 a month Multi-AZ floor that is 55 % of the bill (section 15), at the price of an audit trail you can only query along paths you designed up front ("who changed any flag in March") |
| S3 | ✔ as a derived store for snapshot bodies | immutable versions, ~18k a year × ~50 KB gzipped ≈ 1 GB (section 15); `snapshots.body` may be an object key instead of a `JSONB` column. Redis is not needed: each API node keeps the latest snapshot in memory |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.large`, section 15) as the system of record for flags, rules, audit and snapshots; the in-memory snapshots in the API nodes and SDKs are derived copies. The deciding property is the transactional publish with its audit row, and `jsonb` documents; the honest reading is that 0.5 MB and 50 writes a day fit any relational database, so team skill and the platform's default (this track: PostgreSQL) are the real tie-breakers.

**If it were MySQL instead:** nothing material. `JSONB` becomes `JSON`, `BIGSERIAL` becomes `AUTO_INCREMENT`, and the `TEXT` primary keys become `VARCHAR(n)` (InnoDB cannot index a bare `TEXT`; `key` is also a reserved word in MySQL, so quote it). The monotonic version bump needs a lock in either engine (a row lock on the environment, or `pg_advisory_lock` / `GET_LOCK`), with the `(env, version)` key as the backstop in both.

**What would flip the choice:** not load: 10× the instances changes snapshot delivery (S3 + CloudFront, section 15), not the database. What would flip it is operations: a platform with no relational estate that wants zero database servers could run DynamoDB + S3, with a conditional write on `version` giving the monotonic bump. If the audit trail starts being queried across flags and time, stay relational.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 200k instances: ~7k polls/s (nearly all `304`), 200k SSE connections (~4 GB), and a kill-switch stampede of 200k refetches in ~2 s ≈ 100k req/s and 10 GB (~40 Gbit/s); still ~1,000 flags and ~50 edits a day | not the snapshot path (S3 + CloudFront from section 15 absorbs the stampede) but the push tier: 200k × ~20 KiB = 4 GB; with one zone down each of 3 `c7g.large` holds 100k connections = 2 GB of 4 GiB, no garbage-collector headroom, and the zone loss is 100k simultaneous TLS reconnects (~200 vCPU-seconds against 4) | split the push tier from the API: SSE on ~6 `m7g.large` (~33k connections, ~0.7 GB each); snapshot bodies as immutable objects on S3 + CloudFront (`/snapshot/prod/418.json.gz`) with only a 1-5 s version pointer on the origin; SDKs reconnect with jittered backoff and spread normal-change refetches over ~2 s (a kill switch is not jittered). Give up: one more tier and CDN hop; normal changes arrive up to ~2 s later | ≈ $3.2k/month, not 100 × $440: Postgres ($240) and the API are floors; data out (~24 TB ≈ $2k) and the ALB's connection LCUs (≈ $0.4k) are what grow |
| **1,000×** | 2M instances: ~70k polls/s, 2M SSE connections (~40 GB), stampede of ~1M req/s and 100 GB (~400 Gbit/s); ~3B evaluations/s fleet-wide, still local (~3,000 vCPU of the applications' own) | one region and one push tier: 2M ÷ 33k = ~60 SSE nodes, and a zone loss reconnects ~667k clients at once (~1,300 vCPU-seconds of TLS at 2 ms each); every instance's push channel hangs off one origin | change the shape: a relay tier per region (a tree). Each region's ~10 relays hold the SSE connections of its instances, subscribe to the origin once, cache snapshot bodies and keep serving them if the origin or the control plane is down (static stability); SDKs find the nearest relay by DNS. Updates become deltas ("changes since version 418", ~1 KB instead of 50 KB). Give up: one more hop (+100-500 ms), regions can briefly hold different versions, a delta protocol with a full-fetch fallback | ≈ $26k/month: data out ≈ $20k, connection LCUs ≈ $4k, relays ≈ $2k; deltas cut the egress by ~60 % |
| **10,000×** | 20M clients (devices, not servers: as servers this is about a whole cloud's fleet): ~667k polls/s, 20M SSE connections (~400 GB), a kill-switch refetch of 20M × 50 KB = 1 TB in ~2 s ≈ 4 Tbit/s | shipping the ruleset to every device: a phone cannot hold a socket open for a flag, a stampede of 1 TB is a physical limit, and rules that name customers and strategy must not leave the server at all | change the problem: devices never receive rules. The edge evaluates per user and returns resolved values (~20 flags, ~1 KB), cached by (flag-set version, segment) at the CDN; devices fetch on app open and every ~15 min (jittered), and a kill switch is a silent push (APNs/FCM) that triggers one refetch, best effort, within minutes. Servers keep today's local evaluation. Give up: seconds-fast propagation to every device, rich per-user attributes in client rules, offline determinism | ≈ $200k/month in bytes alone if devices kept the full-snapshot protocol (changes ≈ $128k, polling ≈ $75k); ≈ $5k after (~22k req/s × ~1 KB) plus edge compute |

*Arithmetic:* 2,000 × 100 = 200k ÷ 30 s ≈ 7k polls/s; 200k × 50 KB = 10 GB; 200k ÷ 2 s = 100k req/s; 200k × 20 KiB = 4 GB ÷ 2 survivors = 2 GB each; 2M ÷ 33k = 60; 667k × 2 ms = 1,330 vCPU-s; 20M × 50 KB = 1 TB; data out 240 GB × 100 = 24 TB; × 1,000 = 240 TB ≈ $20k; × 10,000 = changes 1.5 PB ≈ $128k plus polls 667k × 0.5 KB ≈ 877 TB/month ≈ $75k.

**What stays the same at every scale:** the snapshot is an immutable, versioned artefact published atomically (rollback is republishing N−1 as N+1), SDKs only ever swap to a higher version, and an evaluation never waits on the network: an application with no connection runs on its last snapshot, then on code defaults.

**Say it in an interview:** "At 100× the stampede is 100k requests and 10 GB, which S3 plus a CDN absorbs, so what runs out is the push tier: 200k connections, which I move onto their own nodes while the origin serves only a tiny version pointer. At 1,000× it is 2M connections and a zone loss reconnecting 667k clients, so I put a relay tier in every region that caches bodies and keeps serving if the origin is down, and I send deltas instead of 50 KB. At 10,000× the clients are devices, so I stop shipping rules to them: the edge evaluates per user and returns about 1 KB of resolved values, and a kill switch is a silent push."
