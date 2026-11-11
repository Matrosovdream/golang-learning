# 36 · LLM Gateway

`🟢 easy` · *Topics:* one chokepoint for every model call, layered kill switches, token budgets with reserve-then-settle, one log row per call, response and prompt caching, model routing, timeouts / circuit breaker / provider failover · *Builds on:* [03 Rate Limiter](03-rate-limiter.md) (limits), [06 View & Like Counter](06-view-like-counter.md) (hot counters), [09 Feature Flags](09-feature-flags.md) (switches), [12 Notification Service](12-notification-service.md) (provider adapters & failover)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [35 Redirect Manager & 404 Tracker](35-redirect-manager.md) · Next: [37 In-App Records Assistant](37-records-assistant.md)

---

> 🌱 **Seed (honeydolist-site):** every call to a model goes through one Go seam, `platform/llm` (a `Provider` with `Stream`, an Anthropic driver and a scripted `fake` for dev, PLAN step 29). One function, the meter's `add`, counts each call into `assistant_usage` (one row a day per surface) and, since step 33 (migration 0066), writes one `assistant_calls` row: provider, model, four token counts, cost in millionths of a dollar, `ms`, `first_ms`, `stop`, `error`, provider `request_id`; no text. Limits (a day and a month, per surface) are checked *before* a question starts; "blank = no limit, 0 = off". Step 32 added a Superadmin switch that hides the whole assistant by removing one permission at the single place permissions are read (no role edited) and also stops the background job that calls the model. Credentials are sealed under a server key (ADR 0009). Small-scale numbers: ~25,000 log rows a year, a 1M-token daily limit per surface.
> **Here, at scale:** a product with 30 AI features, 50M model calls a day (~580/s average), per-tenant spend caps, and a provider that has bad afternoons.

## 1. The task

> "Our product has thirty features that call an LLM: chat answers, summaries, tagging, drafts, search words. Today each team calls a provider directly. Design the one place every call must go through: it can switch features off, cap spend per customer, log every call, and keep working when a provider is down."

The model call is the most expensive and least reliable function call in the system: a second or two of latency, money per token, and an outage you do not control. Controls that live in thirty codebases do not exist.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Who calls it? | Interactive (a person waits) and batch (nobody waits) need different policies. | Internal services only: interactive, batch and background kinds. |
| Scale? | Decides the log store and the meter. | 50M calls/day, ~2,000/s peak, 100k tenants, 30 features. |
| Overhead budget? | The call itself takes 1–3 s. | Gateway adds ≤ 5 ms p99 (≈ 0.1 % of a 2 s call). |
| Streaming? | Changes retry rules and billing of partial answers. | Yes, for interactive. |
| More than one provider? | Failover is only possible if prompts are portable. | Two providers; a feature opts in with `fallback_ok`. |
| Budgets: hard or soft, what scope? | Money guard vs. product experience. | Per tenant × feature × day and month. Soft degrade at 90 %, hard stop at 100 %. |
| Switch scopes? | Incidents need big switches, support needs small ones. | Global, per feature, per tenant, per tenant × feature. |
| What may be logged? | Prompts hold customer data. | Metadata always; bodies sampled and redacted. |
| Bring-your-own provider keys? | Changes who pays and who is capped. | Out of scope (see 42). |

## 3. Requirements

**Functional**
- One API for all features: `complete` (streaming or not), with a model *class* (`small`, `large`, `vision`), never a vendor model id.
- Switches at four scopes; background and batch work stops too.
- Budgets per tenant and feature, a day and a month; a clear refusal that says which limit and when it resets.
- One log row per provider call, including refused and failed ones.
- Exact-match response cache; provider prompt caching used where it pays.
- Retries, a circuit breaker per provider and an optional fallback provider.
- A scripted fake provider for dev, CI and chaos tests.

**Non-functional**
- Gateway overhead ≤ 5 ms p99; a switch takes effect everywhere within ~5 s.
- A logging or metering failure never fails a user's request; money limits still fail *safe*.
- The log is the truth for cost; counters are a cache that can be rebuilt from it.

**Out of scope**
- Prompt authoring, evaluation harness (37), per-tenant BYO keys (42), fine-tuning.

## 4. Back-of-the-envelope

All prices are **illustrative assumptions**, not any vendor's: large model ~$3 per million input tokens and ~$15 per million output; small model ~$0.25 / ~$1.25.

| Quantity | Estimate | How |
|---|---|---|
| Calls | ~580/s avg, ~2,000/s peak | 50M ÷ 86,400 = 579; peak ≈ 3.5× |
| Tokens | 100B in + 15B out per day | 50M × 2,000 in, × 300 out (assumed averages) |
| Cost, everything on the large model | ≈ $525k/day ≈ $16M/month | 100,000 M × $3 + 15,000 M × $15 = $300k + $225k |
| Cost, 80 % of calls routed to the small model | ≈ $140k/day ≈ $4.2M/month | 0.2 × $525k + 0.8 × ($25k + $18.75k) |
| …plus prompt caching (60 % of input is a stable prefix read at 0.1×) | ≈ $97k/day ≈ $2.9M/month | input cost × (1 − 0.6 × 0.9) = × 0.46 |
| Log rows | 50M/day × ~300 B = 15 GB/day, ~5.5 TB/year | metadata only, no bodies |
| Bodies, if all stored | ~500 GB/day | 50M × ~10 KB |
| Bodies, 1 % sample + 2 % errors/refusals | ~15 GB/day | 3 % of 500 GB |
| Meter operations | ~4,000 Redis ops/s at peak | reserve + settle per call |

What the numbers tell us: the same traffic costs $16M or $2.9M a month depending on routing and caching, so *cost control is the design*, not a feature. The meter is trivial in ops/s but has **hot keys** (one big tenant × one feature). The log is a TB-per-year table: partition it and age it out of Postgres. And 2 ms of gateway latency is noise next to 2 s of model.

## 5. Plan

1. Why a gateway, and library vs service.
2. The request path: auth → switches → budget → cache → route → provider → settle → log.
3. Switch semantics, including background work.
4. Budgets: estimate before, meter after.
5. The call log: what, how long, how written.
6. Timeouts, retries, breaker, failover.

## 6. High-level design

```
 30 features / batch workers / background jobs
              │   (service identity + tenant_id + feature + kind)
              ▼
 ┌────────────────────────── Gateway (stateless Go, N pods) ──────────────────────────┐
 │ 1 authn      who is this feature, which tenant                                      │
 │ 2 switches   in-memory snapshot (poll 2 s + pub/sub)   ── off → 503 feature_off      │
 │ 3 budget     Redis Lua: reserve estimate            ── over → 429 budget_exhausted   │
 │ 4 cache      exact-match (hash of class+prompt+params, scope-safe)  ── hit → 0 tokens│
 │ 5 route      model class → provider/model by policy, breaker state                   │
 │ 6 call       Provider adapter (anthropic / other / fake), stream back to caller      │
 │ 7 settle     real usage − reserved → counters;  8 emit call event (queue)            │
 └───────────────┬───────────────────────────────────────────────┬────────────────────┘
                 ▼                                               ▼
        Redis (budgets, reservations)              queue ► log writer (batch) ► llm_calls (day partitions)
                                                              └► rollup ► llm_usage_daily ► 80 % alerts
 control plane (admin API + UI) ► Postgres (switches, budgets, price list) ► snapshot to pods
```

One request: the feature posts a prompt; the gateway checks switches in memory (no I/O), reserves tokens in Redis, looks for a cached answer, picks a provider, streams text back while counting usage, settles the reservation, and emits one event. The log writer and rollups are asynchronous: they can lag without hurting a user.

## 7. API

```
POST /v1/llm/complete   { feature, kind, class: "small|large|vision", messages, tools?, schema?,
                          max_tokens, stream, cache: "off|exact", idempotency_key }
                        (tenant and caller come from the service credential, never the body)
  → 200 text/event-stream   delta…, usage, done {call_id}
  → 200 { text, stop, usage, call_id, cached }
  → 429 { code: "budget_exhausted", limit: "tenant_feature_day", resets_at }
  → 503 { code: "feature_off", scope: "feature" }          (no Retry-After: it is a decision, not a fault)
  → 502 { code: "provider_unavailable", call_id }

PUT /v1/admin/switches/{scope}/{key}   { state, reason, revert_at? }
PUT /v1/admin/budgets/{tenant}/{feature}   { period, limit_tokens | null }      (null = none, 0 = off)
GET /v1/admin/calls?tenant=&feature=&outcome=&from=&to=&sort=cost|slowest   → paged, stats strip
GET /v1/admin/calls/{call_id}          → one call, its attempts, its sampled body (if any)
```

## 8. Data model

```sql
CREATE TABLE llm_calls (                       -- one row per provider call, partitioned by day
  created_at    timestamptz NOT NULL DEFAULT now(),
  id            bigint GENERATED ALWAYS AS IDENTITY,
  call_id       uuid        NOT NULL,            -- what a support ticket quotes
  tenant_id     bigint      NOT NULL,
  feature       text        NOT NULL,
  kind          text        NOT NULL CHECK (kind IN ('interactive','batch','background','test')),
  provider      text        NOT NULL,            -- 'fake' is visible, never logged as a real model
  model         text        NOT NULL,
  attempt       smallint    NOT NULL DEFAULT 1,
  outcome       text        NOT NULL CHECK (outcome IN
                  ('ok','cached','refused_switch','refused_budget','provider_error','timeout','cancelled')),
  stop          text,
  input_tokens  integer NOT NULL DEFAULT 0, output_tokens integer NOT NULL DEFAULT 0,
  cache_read_tokens integer NOT NULL DEFAULT 0, cache_write_tokens integer NOT NULL DEFAULT 0,
  cost_micros   bigint  NOT NULL DEFAULT 0,      -- millionths of a dollar; never a float
  price_version smallint NOT NULL,               -- cost at that day's prices stays explainable
  ms integer, first_ms integer, request_id text, error text CHECK (length(error) <= 500),
  prompt_sha256 bytea,                           -- dedupe/cache analytics without keeping text
  body_ref      text,                            -- object-store key, only for sampled/error bodies
  PRIMARY KEY (created_at, id)
) PARTITION BY RANGE (created_at);
CREATE INDEX ON llm_calls (tenant_id, created_at DESC);

CREATE TABLE llm_usage_daily (                 -- the meter's durable copy; rebuilt from llm_calls
  tenant_id bigint NOT NULL, feature text NOT NULL, day date NOT NULL,
  calls bigint NOT NULL DEFAULT 0, tokens bigint NOT NULL DEFAULT 0, cost_micros bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, feature, day)
);
CREATE TABLE llm_budgets (
  tenant_id bigint NOT NULL, feature text NOT NULL DEFAULT '*',
  period text NOT NULL CHECK (period IN ('day','month')),
  limit_tokens bigint CHECK (limit_tokens IS NULL OR limit_tokens >= 0),   -- NULL none, 0 off
  PRIMARY KEY (tenant_id, feature, period)
);
CREATE TABLE llm_switches (
  scope text NOT NULL CHECK (scope IN ('global','feature','tenant','tenant_feature')),
  key text NOT NULL, state text NOT NULL CHECK (state IN ('on','off')),
  reason text, set_by bigint, set_at timestamptz NOT NULL DEFAULT now(), revert_at timestamptz,
  PRIMARY KEY (scope, key)
);
CREATE TABLE llm_prices (model text NOT NULL, version smallint NOT NULL,
  in_micros_per_mtok bigint NOT NULL, out_micros_per_mtok bigint NOT NULL,
  cache_read_mult numeric(4,3) NOT NULL, cache_write_mult numeric(4,3) NOT NULL,
  PRIMARY KEY (model, version));
```

Create tomorrow's partition ahead of time; drop (or detach and archive) partitions past retention instead of `DELETE`.

## 9. Deep dives

### 9.1 One chokepoint: library, sidecar or service?

| Option | Pros | Cons |
|---|---|---|
| SDK per team, "please follow the guide" | zero infrastructure | thirty policies; keys everywhere; no global switch; a forgotten feature bypasses the cap |
| Shared library in a monorepo (the seed: `platform/llm`) | no extra hop; one codebase | Go only; caps and keys still live in each deployment; a global switch means a redeploy or a shared store anyway |
| Sidecar per pod | local hop, same logic | N copies of state (counters, breaker); upgrades are a fleet roll |
| **Central gateway service + thin client** | keys and caps in one place; one log; language-neutral; one breaker view of each provider | one more hop (~2 ms); must itself be highly available |

**Choice and why.** A central service. The seed's own note says when: a package is right while one app owns its keys and caps; "a service earns its place when apps in other languages need it, or when one place must hold the keys and caps of many projects." Thirty features is that day. Only the gateway holds provider credentials, so *there is no way around it*. Make it a stateless pod set behind the load balancer; the client library fails the request rather than falling back to a direct provider call.

### 9.2 Kill switches that actually stop things

Switches exist for incidents ("provider X is leaking latency"), money ("we are out of credit") and product ("hide the assistant for everyone but staff"). Decisions:

- **Layered, most specific wins on `off`:** global → feature → tenant → tenant × feature. `off` at any layer refuses; `on` never overrides an `off` above it.
- **Reads cost no I/O.** Each pod keeps the whole switch table in memory (it is small), refreshed by a 2 s poll of a version counter plus a pub/sub nudge. The seed reads its settings from memory for 5 s and a save replaces the copy on that node at once; same idea. Worst case a switch lands in ~5 s everywhere.
- **Hide, don't just refuse.** In the seed, "assistant off" removes `assistant.use` from every session whose owner lacks `system.write`, at the one place permissions are read, so the menu item, the page and every route follow, and **no role is edited**: turning it back on restores everything at once. At scale the gateway returns the same fact in a feature-flags response so the UI can grey the control instead of showing an error.
- **Privileged "try it".** The operator who flipped it off can still call it (a scoped test credential) to verify before turning it back on.
- **Background work must obey.** A batch worker checks before *claiming each item* and ends the run as `stopped` ("switched off", "limit reached"), not `failed`; nothing is lost and no alert fires. In the seed, step 32 exists precisely because a job nobody asked for (suggested words every 10 minutes) kept spending.
- **In-flight calls finish; streams are not killed** unless the incident says so (a global `off` with `drain: false`).
- **Timed switches:** `revert_at` turns it back on by itself, so a 2 a.m. "off for an hour" cannot become "off for a week".
- **If the control plane is down,** pods keep the last snapshot (fail static). A flip made during that outage is the one thing that cannot propagate, so keep the control plane simpler than anything it controls.

**Choice and why.** In-memory snapshots with polled versions, four scopes, `revert_at`, and workers that check per item. Cheap on the hot path, fast enough for humans, safe when the store blinks.

### 9.3 Cost control: reserve before, settle after

A check-then-call limit has a race: 50 concurrent calls each see "under limit" and each spends. The seed accepts this ("a limit can be passed by one question, about 10,000 tokens, and by one each when two run at once"). At 2,000 calls/s with big tenants, 50 concurrent calls × 3,000 tokens = 150k tokens: 15 % past a 1M-token cap.

| Option | Pros | Cons |
|---|---|---|
| Check meter, call, add actual (the seed) | trivial; one upsert | overshoot grows with concurrency; stale if the meter lags |
| **Reserve an estimate atomically, settle with actual** | overshoot bounded by estimate error | needs an estimate and reservation cleanup |
| Per-pod local budgets (cap ÷ N pods) | no shared store on the path | uneven; small tenants get 0 on most pods |
| Bill afterwards, alert only | no refusals | not a cap, a report |

**Choice and why.** Reserve then settle, in Redis, one Lua script so check and increment are atomic:

```lua
-- KEYS: used, reserved (same hash tag, e.g. {t42:summarise}) · ARGV: limit ("" = none), estimate
local used = tonumber(redis.call('GET', KEYS[1]) or '0')
local res  = tonumber(redis.call('GET', KEYS[2]) or '0')
if ARGV[1] ~= '' and used + res + tonumber(ARGV[2]) > tonumber(ARGV[1]) then return 0 end
redis.call('INCRBY', KEYS[2], ARGV[2]); redis.call('EXPIRE', KEYS[2], 90000)
return 1                                        -- limit 0 is always refused: "0 = off" for free
```

Estimate = prompt tokens (cheap counter) + `max_tokens`. After the call: `used += actual`, `reserved −= estimate`. A crashed pod leaves a reservation behind, so each reservation is also a hash entry with a TTL equal to the request deadline, and a reaper subtracts expired ones. Run the same script for the month key. At 90 % of any budget, route to the small model and send one alert per tenant per month (the seed: one 80 % alert, once a month per surface). Multi-call agent loops reserve per call and have a hard cap on calls per request.

More levers, in order of payoff (§4): **route by class** (classification and extraction do not need the large model); **provider prompt caching**: put the stable prefix (rules, tool schemas) first and per-request data (today's date, user text) last; the seed moved the date into the first user message for exactly this, and found one small model only caches prompts above a minimum length, so its 3,000-token site prompt was *not* cached. **Exact-match response cache** keyed by hash(class, params, prompt) with a scope: the seed caches only the *public* site answers for a day (key includes the question, the date and a hash of catalog, model and rules, so a rule edit gets a fresh answer) and never the dashboard's, which depend on the asker's permissions and change by the minute. Never share a cache entry across tenants unless the prompt has no tenant data. Count tokens in full even for cache reads (simple, errs safe) but compute *money* from the real price list.

### 9.4 The call log: what to keep, for how long, written how

One row per provider call, written at **one door**, so no path is missed (the seed's `record`, beside the meter's upsert). Retries are separate rows with `attempt`.

| What | Keep | Why |
|---|---|---|
| Metadata: tenant, feature, model, tokens, cost, ms, `first_ms`, `stop`, error, provider `request_id` | always, 365 days | cost truth, latency, what a support ticket asks for |
| Prompt/response bodies | 1 % sample + every error, refusal and 👎; object store; 30 days | debugging without a 500 GB/day bill |
| Tool result rows, retrieved documents | never | they are the customer's data (the seed logs "what was called, never the rows") |
| Conversation state | emptied a day after last use | the seed empties thread messages after a day but keeps the log a year |
| Caller identity | a keyed hash, show 4 characters | the seed shows `Visitor · 3f9a`, never an address |

**Redact before store:** emails, phone numbers, card-like digit runs, plus tenant-configured patterns. **Write path:** the gateway emits an event to a queue and moves on; a batch writer inserts thousands of rows per statement. "A failed insert is logged, never fails the question." The **log is the truth and the counters a cache**: a nightly job re-sums `llm_calls` into `llm_usage_daily` and compares with Redis (the seed has a Go test that a day's calls cost exactly what the meter says). **Retention is a promise:** the seed's privacy page says "kept a year", so changing `365` means changing that sentence too. At 15 GB/day keep ~14 days hot in Postgres, then roll up (`tenant, feature, day`) and move detail to columnar/object storage.

### 9.5 Timeouts, retries, breaker, failover

- **Timeouts:** connect 2 s; *first token* 10 s (interactive) or 60 s (batch); total by class. A hung stream burns a connection and a user's patience.
- **Retry only what is safe:** 429, 5xx, connect errors and timeouts *before the first token*. Never after text has streamed. Jittered exponential backoff (see [10](10-webhook-delivery.md)); honour `Retry-After`. A model call has no side effects, so retries are safe but they cost money: cap at 2 and meter every attempt.
- **A partial answer is billed.** A client that disconnects mid-stream was still charged: the seed's `Stream` returns the usage seen so far even with an error, "the meter must count it".
- **Circuit breaker per provider × model:** open after (say) 50 % errors over 30 s with ≥ 20 calls; half-open probes one call per 10 s. Open → skip to the fallback or fail fast with `502 provider_unavailable`.
- **Failover is a policy, not a reflex.** Prompts, tool schemas and structured-output behaviour differ between vendors, so only features marked `fallback_ok` (and evaluated against the fallback, see [37](37-records-assistant.md)) may fail over. Same adapter idea as [12](12-notification-service.md): one `Provider` interface, a registry, a fake.
- **Provider rate limits** (tokens per minute) are a shared resource: use priority lanes, so interactive traffic is never queued behind a batch backfill (the seed gives the drafts' outbox lane one or two worker slots for the same reason).
- **The fake provider** answers from a script and spends nothing; its rows are labelled `provider = fake`, so a dev answer can never look like Claude's (the seed once misread a cost estimate because they did).

**Choice and why.** Two attempts max, breaker per provider/model, fallback only for opted-in features, lanes by `kind`.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (500M calls/day):** the log (150 GB/day) before anything else: batch inserts, daily partitions, a columnar store for detail. Then hot meter keys: shard a hot (tenant, feature) counter across a few Redis keys (see [06](06-view-like-counter.md)) and sum on read. Then the provider's own rate limit, which becomes the real throughput ceiling.
- **Single points of failure:** Redis (budgets), the gateway pods, the control-plane store, each provider.
- **Redis down:** budgets cannot be checked. Interactive: fail open with a small per-pod token bucket for a few minutes, then closed; batch: fail closed (waiting is free). The seed's rule for an unreadable meter is "say no, the limits guard money"; here the product decides per kind.
- **Provider down:** breaker opens, fallback or `502`; batch jobs pause themselves; alerts fire on breaker state, not on individual errors.
- **Log queue down:** buffer on local disk for minutes; if that fills, drop bodies first, then metadata for `kind=test`; never block the call.
- **Abuse and prompt injection:** a user can make a feature burn tokens. Clamp `max_tokens`, cap calls per request, and add a per-user request-rate limit ([03](03-rate-limiter.md)) in front of the token budget.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| central gateway service | shared library | one place for keys, caps, switches, log | an extra hop and an HA component |
| in-memory switch snapshot | check the DB per call | no hot-path I/O; fast flips | ≤ 5 s propagation; fail-static during outages |
| reserve then settle | check then add | bounded overshoot under concurrency | estimates, reservations, a reaper |
| model *class* in the API | vendor model ids | routing and failover without client changes | clients cannot pick a specific model |
| one row per call, bodies sampled | log everything | affordable, privacy-friendly log | rarely, a body you wish you had |
| fallback only for opted-in features | automatic failover | no silent quality regressions | an outage can still take a feature down |

## 12. Check yourself

1. Why is a gateway better than a shared client library once you have thirty features and several languages?
2. Walk one request through the gateway and say which steps do I/O and which do not.
3. A tenant has a 1M-token daily cap and 50 concurrent calls arrive. What goes wrong with check-then-add, and how does reserve-then-settle fix it?
4. A background job is spending money nobody asked for. How does a switch stop it without losing work?
5. What do you store per call, what do you refuse to store, and what is the source of truth for cost?

<details><summary>Answers</summary>

1. Only the gateway holds provider keys, so there is no bypass; caps, switches, the log and the breaker live in one place and apply to every language; policy changes ship once. The price is one more hop (~2 ms against a 1–3 s model call) and having to run it highly available.
2. Authn (cached credential, no I/O), switches (in-memory snapshot, none), budget reserve (Redis, one round trip), cache lookup (Redis, one), provider call (the long one, streamed), settle (Redis), emit event (queue, async). The log and rollups are off the request path.
3. All 50 see "under limit" and all spend: with ~3,000 tokens each that is 150k, 15 % over. Reserving an estimate atomically (used + reserved + estimate ≤ limit) refuses the excess calls up front; settle replaces the estimate with the real usage, and a TTL/reaper frees reservations from crashed pods.
4. Workers check the switch before claiming each item and end the run as `stopped` with a reason, not `failed`; unclaimed items stay queued, so turning the switch back on resumes them. Add `revert_at` for timed switches.
5. Store: tenant, feature, model, tokens (four kinds), cost in micros with a price version, latency, first-token latency, stop reason, error, provider request id, a prompt hash; bodies only for a small sample and for errors/refusals. Never store tool-result rows or unredacted customer data. The `llm_calls` log is the truth; Redis counters are a cache reconciled against it nightly.

</details>

## 13. In Go

- Related lessons: [32 Hexagonal ports & adapters](../../32-hexagonal-ports-adapters.md) (`Provider` as a port; Anthropic, other and fake as adapters), [36 Resilience](../../36-resilience-patterns.md) (timeouts, backoff, breaker), [38 Caching](../../38-caching-patterns.md) (exact-match cache, singleflight), [58 Realtime: WebSockets & SSE](../../58-realtime-websockets-sse.md) (streaming tokens, `http.Flusher`), [68 Rate limiting](../../68-rate-limiting-throttling.md), [34 Outbox](../../34-event-driven-outbox.md) (call events), [15 Sync & context](../../15-sync-context.md) (cancel on client disconnect), [39 Observability](../../39-observability-tracing.md) (metrics: refusals by reason, breaker state).
- The seam, as in the seed (names simplified):

```go
type Provider interface {
    Name() string // "anthropic", "fake": what the log row stores
    // Stream sends one request and returns the whole turn. With an error the Turn still carries the
    // usage seen so far: a caller who left mid-answer has been billed.
    Stream(ctx context.Context, req Request, onText func(string)) (Turn, error)
}
```

- Packages: `net/http`, `github.com/redis/go-redis/v9` (`Eval` for the script), `pgx`, `github.com/sony/gobreaker`, `golang.org/x/sync/singleflight`, `golang.org/x/sync/errgroup`, the provider's official Go SDK.
- Test with the fake: scripted answers, injected latency and failures (429, mid-stream cut) to exercise retries, the breaker and partial billing without spending a cent.

## 14. Extensions

- **Bring your own key per tenant:** their budget, their provider; where is the credential sealed and who may see it? (task 42).
- **Batch endpoints** for non-urgent work, usually cheaper per token; a batch lane with its own budget (task 38).
- **Semantic cache** (embeddings) for near-duplicate prompts: hit rate vs the risk of serving a wrong answer.
- **Evaluation gate:** a golden-question suite that must pass before a model or prompt change reaches production (task 37), plus shadow traffic to compare models.
- **Prompt registry:** versioned prompts, each call logging `prompt_version`.
- **Data residency:** route EU tenants only to EU endpoints; the switch scopes already give the hook.
- **Showback:** a per-tenant cost report and a "you used 80 %" email, built from `llm_usage_daily`.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Gateway (Go, stateless: switches, budget, route, stream, settle, emit) | 2,000 calls/s peak (§4) ⇒ ~6,000 streams in flight (2,000 × ~3 s, Little's law) | CPU (TLS on both legs, JSON, SSE relay); network and memory are small | 2,000 × ~2 ms (TLS to the provider, decode an ~8 KB prompt, ~100 SSE events, token count, 3 Redis round trips) = 4 vCPU; log writer, rollup and reaper ride along ≈ 0.2; N = ceil(4.2 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.97) = 2 → 3. Memory: 6,000 × ~64 KiB (two sockets, buffers, a 10 KB body) ≈ 0.4 GiB + switch snapshot + heap ≈ 1 GiB of 8; network 2,000 × 9 KB = 18 MB/s ≈ 0.15 Gbit/s | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | a proxy that waits on a 2 s model call is mostly idle sockets, so it is CPU for the crypto and parsing, then nothing; 35 % busy, 53 % with a zone down; it is not an `n` (network) case at 0.15 Gbit/s |
| Redis (budgets, reservations, exact-match cache) | meter ~4,000 ops/s (§4; ~8,000 if the month key doubles it, §9.3) + ~2,000 cache GETs/s; meter ≈ 1M live keys, cache ≈ 2M entries × ~1.5 KB (assumption) | memory | meter 1M × ~150 B = 0.15 GB; cache 2M × 1.5 KB = 3 GB; total 3.15 GB × ~1.7 = 5.4 GB ⇒ ÷ 0.7 ≈ 7.7 GiB; ~10k ops/s (Lua ≈ 10 µs each) ≈ 10 % of one core; a hot (tenant, feature) key is serialised on one core, ~100k ops/s, far above one tenant's rate | `cache.r7g.large` (2 vCPU / 16 GiB), ElastiCache | 1 primary + 1 replica (other zone) | memory-bound, so `r`: `m7g.large` (8 GiB) is smaller than the 7.7 GiB it needs plus headroom; CPU idles; the replica matters more than the size, because Redis down is the fail-open/fail-closed event of §10 |
| Postgres (llm_calls partitions, llm_usage_daily, budgets, switches, prices) | ~2,000 row inserts/s at peak, in batches; 15 GB/day (§4: 50M × ~300 B); 14 days hot | storage size and sequential throughput; CPU and IOPS are tiny | 15 GB/day + ~3 GB/day indexes (assumption, ~20 %) = 18 GB/day × 14 ≈ 250 GB ⇒ 500 GB `gp3`; writes ≈ 1 MB/s of rows, ~2 MB/s with WAL, in batches of thousands ⇒ a few hundred IOPS vs 3,000 included; CPU 2,000 × ~30 µs ≈ 0.06 vCPU; the nightly re-sum of one day (15 GB) ≈ 1–2 min on 2 vCPU | `db.m7g.xlarge` (4 vCPU / 16 GiB), RDS Multi-AZ | 1 + synchronous standby | an append-only log is neither memory- nor IOPS-bound, so not `r` or `i`; `m` leaves room for the nightly scan and admin queries; Multi-AZ because the log is the truth for the spend; older partitions go to S3, not to a bigger disk |

Queue: SQS between the gateway and the log writer, 100 events per message (~30 KB): 500k messages/day. GPU: none, the models run at the providers; self-hosting ~1.3M tokens/s (115B tokens ÷ 86,400) is a different lesson and the sheet has no GPU prices. S3: detail older than 14 days as columnar files (5.5 TB/year raw ≈ 1.1 TB at an assumed 5× compression) plus ~450 GB of sampled bodies (15 GB/day × 30 days). The traffic to the providers is the infrastructure's real cost: 50M calls × ~8 KB prompt = 400 GB/day out ≈ 12 TB a month, all through NAT.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Gateway: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Redis: 2 × `cache.r7g.large` (2 × ≈ $156) | $310 |
| Postgres: `db.m7g.xlarge` ≈ $240 managed, Multi-AZ doubles | $480 |
| gp3: 2 volumes × 500 GB (primary, standby) | $80 |
| S3: ~1.1 TB columnar detail + ~450 GB sampled bodies × $0.023 | $40 |
| SQS: 1.5M requests/day × 30 × $0.40 per million | $20 |
| Internal ALB: $16 + ~19 LCU (580 calls/s × 9 KB ≈ 19 GB/h) × $0.008 × 730 | $130 |
| Data out to the providers: 12 TB × $0.09 | $1,080 |
| NAT: 3 gateways ($99) + ~13.8 TB processed × $0.045 | $720 |
| Model tokens, routed and prompt-cached (§4): $97k/day × 30 | $2,910,000 |
| **Total** | **≈ $2,913,200** (≈ $2,912,800 with a 1-year plan on the ≈ $1,110 compute baseline) |

- **Biggest line and why:** the model tokens, 99.9 %. Hardware is ≈ $3.2k a month, and even inside that, moving bytes to the providers (egress + NAT ≈ $1.8k) costs more than all the compute ($1.1k). This is honest tiny hardware: three small proxies, a small cache and one database, because the gateway only forwards. The lever is §4's: routing and prompt caching take the same traffic from ≈ $16M to $2.9M a month; no instance choice comes close.
- **What changes at 10×:** 20k calls/s, ~60,000 streams in flight, CPU 40 + 2 = 42 vCPU ⇒ on `c7g.4xlarge` N = ceil(42 ÷ 8.53) = 5 → 6 nodes (≈ $2,520). Redis ops ≈ 100k/s is one core's limit, so 3 shards of `cache.r7g.xlarge` (cache 54 GB ÷ 0.7 ≈ 77 GiB) with replicas, and hot (tenant, feature) counters split across keys (§10). Postgres at 150 GB/day keeps a 2–3 day window (≈ 360–540 GB) and sends detail to a columnar store. NAT breaks: ~60k connections to the same provider addresses pass its ~55k-per-destination limit, so use public subnets or several NAT addresses; egress ≈ $10.8k a month. Tokens ≈ $29M a month, and the providers' rate limits come first.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- **The call log** (`llm_calls`) is append-only, one ~300 B row per provider call: 50M a day, ~580/s average and ~2,000/s at peak, 15 GB a day, ~18B rows and ~5.5 TB a year (section 4). Rows are never updated. Reads are admin filters (tenant, feature, outcome, time range, sort by cost or slowest), a nightly re-sum of one day (15 GB), and a 365-day retention promise.
- **Control-plane config** (switches, budgets, prices): a few thousand rows, changed rarely, loaded into every pod's memory; it needs transactions, `CHECK`s and an audit trail, not speed.
- **The meter**: ~4,000 atomic reserve/settle operations/s (~8,000 with the month key) over ~1M live keys, with a hot key per (tenant, feature) and reservations that must expire after a crash (9.3). It is a counter cache that the log can rebuild.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for config and the 14-day hot log | daily range partitions (`PARTITION BY RANGE (created_at)`, key `(created_at, id)`) turn retention into a metadata `DROP`/detach instead of deleting ~700M rows; the log writer inserts thousands of rows per statement (~2 MB/s with WAL, a few hundred IOPS); 14 days ≈ 250 GB on `gp3`; `CHECK`s guard `kind` and `outcome` |
| MySQL (RDS / Aurora) | ✔ works, interchangeable for the hot window | `RANGE` partitioning with `DROP PARTITION` exists (the partition column must be in every unique key, as it already is here), and InnoDB's clustered `(created_at, id)` key stores rows in time order, so appends are sequential. `uuid` becomes `BINARY(16)`, `bytea` becomes `VARBINARY`, `timestamptz` a UTC `TIMESTAMP`; descending and `CHECK` support is there in 8.0.16+ |
| ClickHouse / Athena over Parquet in S3 | ✅ Athena over S3 columnar for days 15–365 (as section 15 sizes it); ✔ ClickHouse if that history must be interactive | ~18B rows a year of append-only metadata is the cheat sheet's "analytics over billions of rows"; Parquet at an assumed 5× compression is ~1.1 TB a year. Neither can hold the switches and budgets (no cheap single-row updates), so they are the second tier, not a replacement |
| OpenSearch | ✖ poor fit | the log is structured metadata filtered and aggregated, not searched as text: bodies are sampled to S3 and `error` is ≤ 500 chars. An inverted index over 5.5 TB a year costs heap and SSD for a query nobody runs, and it could only ever be a derived copy |
| Redis (ElastiCache / MemoryDB) | ✅ for the meter and the exact-match cache (derived), ✖ as the record | one Lua script makes check-and-reserve atomic and keys expire on their own; ~10k ops/s is ~10 % of one core (section 15). Postgres could hold the meter with a conditional `UPDATE` (`RETURNING` there, `ROW_COUNT()` on MySQL), but every call would wait on a synchronous Multi-AZ commit and a hot (tenant, feature) row would serialise on its lock. The log is the truth; the counters are re-summed from it nightly |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.xlarge`, 500 GB `gp3`, section 15) for the config tables and the 14-day hot window of `llm_calls`; Redis (`cache.r7g.large`, primary plus replica) for the meter and cache; S3 columnar files for days 15–365 and sampled bodies. Deciding properties: declarative daily partitions plus batch inserts for an append-only log, and ordinary relational constraints for the few tables that guard money. System of record: `llm_calls` (cost truth), `llm_budgets`, `llm_switches`, `llm_prices`. Derived and rebuildable: `llm_usage_daily` (re-summed nightly), the Redis counters and cache, and each pod's switch snapshot.

**If it were MySQL instead:** nothing material. The same partitioned log (`PARTITION BY RANGE (UNIX_TIMESTAMP(created_at))` on a `TIMESTAMP`, partition column in the primary key), `BINARY(16)` for `call_id`, `VARBINARY` for `prompt_sha256`, and an `id` that is either `AUTO_INCREMENT` with its own index (InnoDB wants that column first in some index, and it is second in the primary key) or generated by the application. Partition creation is manual in both engines, so the "create tomorrow's partition ahead of time" job is needed either way. The log's size and its shape, not the engine, are the real decisions here.

**What would flip the choice:** at 10× (150 GB/day, section 10) the log's *size* breaks first, not its write rate (20k rows/s is still only a few batched statements a second): keep a 2–3 day hot window and push detail to columnar storage sooner. If admins need interactive slicing over the whole year (~18B rows), put ClickHouse (or Redshift) behind the log writer; if one primary's ceiling of ~10k durable writes/s were ever the limit, the log moves to a stream (Kafka or Kinesis) feeding a columnar store, and Postgres keeps only the config.
