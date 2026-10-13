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
  200  ETag: "418"  Cache-Control: max-age=30   body: { "version": 418, "flags": { … } }
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
| CDN in front of the snapshot endpoint | same as polling | offloads the origin | snapshots are immutable per version, so `max-age=30` with ETag revalidation is safe |

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
