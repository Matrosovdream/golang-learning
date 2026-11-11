# 59 · Provider Adapters with Fake Drivers & Chaos Testing

`🟢 easy` · *Topics:* a narrow provider interface with optional capabilities, normalised errors (transient, rate-limited, auth, permanent), idempotency by look-before-create, a driver registry chosen per tenant, the test-double ladder (fake, stub, recorded fixtures, sandbox, canary), contract tests that keep a stub honest, deterministic fault injection, webhook chaos, environment guards that fail closed · *Builds on:* [12 Notification Service](12-notification-service.md) (adapters and failover: not repeated), [42 Credentials Vault](42-credentials-vault.md) (where each tenant's keys live), [51 Runtime Settings](51-runtime-settings.md) (a registry that reloads), [18 Inbound Webhook Receiver](18-inbound-webhook-receiver.md) (the receiving side the chaos exercises)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [58 Outbound Link Click Tracking](58-outbound-click-tracking.md) · Next: [60 Business-Day Time Zones & DST in Reports](60-business-day-time-zones.md)

---

> 🌱 **Seed (honeydolist-site):** every outside service sits behind a small interface of our own, and each has a stand-in. **Email and SMS:** `notify.Driver{Name, Channel, Send}` (ADR 0008); an error is *permanent* only when the driver wraps it in `Permanent(err)`, otherwise transient and retried on a jittered backoff; a `Guard` (`NOTIFY_REDIRECT_TO`, `NOTIFY_ALLOW_TO`) sits **above every driver** outside production; in development Mailpit runs with chaos on (`Recipient:451:100` makes every recipient a transient failure, `Recipient:550:100` a permanent one). **Invoicing** (PLAN step 7e): `invoicing.Provider` is a core of ten methods plus optional capabilities found by type assertion (`Sender`, `PaymentRecorder`, `WebhookReceiver`, `ChangeLister`…), creates look for our `Reference` first, a `fake` driver that production refuses, `wavestub` (a model of Wave's GraphQL with controls `/pay`, `/view`, `/delete` and signed webhooks in variants `replay | forged | stale | late | junk`), and `make wave-check` against Wave's real dev business (7e.3: one bug found and fixed). **Bulk email** (step 22): `bulkmail.Provider`, `mailgunstub` whose per-recipient *fate* follows the address (`bounce…@`, `defer…@`, `lost…@`…) plus knobs to lose, fail and replay webhooks, and `make mailgun-check`. Stand-ins disappear from Settings → Integrations unless `APP_ENV=development` (step 7s). One active integration per category, for one business.
> **Here, at scale:** a platform with 40 third-party integrations built by different teams, 100k tenants each choosing their own provider and account, and a requirement to prove failure handling without calling real providers.

## 1. The task

> "We integrate with dozens of outside services: email, SMS, invoicing, maps, analytics. Design the layer between our code and them so a provider can be swapped, so a tenant can use their own account, and so we can test what happens when the provider misbehaves, without ever touching the real thing."

The provider's *success* path is easy. The work is everything else: timeouts after the request landed, 429s, duplicate and out-of-order webhooks, a staging server that must never email a real customer.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| How many integrations, how many tenants? | Per-tenant config, cache sizes, test matrix. | 40 integrations in ~10 categories; 100k tenants with ~3 integrations each. |
| One account for all, or each tenant's own? | Credentials and failure isolation. | Each tenant brings its own account (keys in [42](42-credentials-vault.md)); some categories have a platform default. |
| Who writes adapters? | Consistency of behaviour. | Many teams: the interface and error taxonomy must be enforceable, not a convention. |
| Do providers offer sandboxes? | Decides what can be tested for real. | Some do, some do not; all are slow and rate-limited. |
| Webhooks too? | Half the failure modes live there. | Yes: signed, retried by the provider, sometimes late or duplicated. |
| Failover between providers? | Changes the error taxonomy. | For email and SMS only ([12](12-notification-service.md)); invoicing has one system of record. |
| Where do tests run? | CI speed, cost. | Every pull request (fake and stub, no network); nightly against sandboxes; a staging stack with continuous low-rate chaos. |
| What must never happen outside production? | Environment safety. | A message to a real customer; a real charge; a stand-in active in production. |

## 3. Requirements

**Functional**
- One interface per *category* (email, SMS, invoicing…), with optional capabilities a driver may or may not have.
- Every failure is normalised into one of four kinds so the retry, failover and alerting code is written once.
- Per-tenant driver choice, a priority order for failover, and a "test connection" check.
- For each driver: a fake, an HTTP stub that speaks the provider's wire format, and a contract suite that runs against all of them.
- Fault injection for every failure we care about, deterministic enough to put in CI.

**Non-functional**
- A new adapter cannot ship without passing the shared contract suite and the chaos matrix.
- The stub stays honest: drift from the real provider is detected within a day.
- Fail closed: if the environment is unknown, the guard is on and stand-ins are hidden.

**Out of scope**
- Writing any particular provider's driver; billing the tenant for provider usage; building the vault ([42](42-credentials-vault.md)).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Driver methods | ~480 | 40 integrations × ~12 operations (the seed's invoicing core alone is 10 methods + capabilities) |
| Contract cases | 24,000 per full matrix | 40 × 200 cases × 3 targets (fake, stub, sandbox) |
| Sandbox calls per nightly run | 8,000 | 40 × 200; spread over 2 h that is 1.1 calls/s in total, far below any sandbox limit worth using |
| Chaos scenarios | 1,920 | 40 integrations × 8 fault modes × 6 operations |
| Chaos run time | ~6.4 min serial, under 1 min on 8 cores | 1,920 × ~0.2 s with a fake clock (no real sleeping) |
| Account rows | 300k | 100k tenants × ~3 |
| Built-driver memory | ~300 MB if all resident | ~1 KB each (credentials + a pointer to a shared HTTP client) |
| Working set | ~15 MB | assume ~5% of tenants active in any 10 minutes: 5k × 3 drivers; an LRU of 20k per pod holds it |
| Retry amplification | +30% calls to a degraded provider | 5% failures × 6 attempts, as in [10](10-webhook-delivery.md); at 100k tenants a retry storm is itself an outage |

What the numbers tell us: running tests is cheap; **confidence** is the scarce thing. The suite is small enough to run on every change, the sandbox run is tiny, and the real risks are a stub that drifts from the provider and 40 adapters that each invent their own error handling.

## 5. Plan

1. The adapter interface and the error taxonomy (the contract every driver signs).
2. The test-double ladder: what each rung can and cannot catch, and the contract suite across them.
3. Chaos: the catalogue of faults and how to make them reproducible.
4. Environment safety: guards, hidden stand-ins, test modes.
5. The registry: choosing a driver per tenant, caching, failover, health.

## 6. High-level design

```
 worker / API ──► capability port        invoicing.Provider · notify.Driver · bulkmail.Provider …
                     │ resolve (tenant, category) → ordered accounts       registry + LRU of built drivers
                     ▼
              guard / policy            recipient allow-list or redirect · quotas · tenant test mode
                     ▼                  ▲ sits ABOVE every driver: none can bypass it
        ┌───────────── driver ─────────────┐
        │ real   Wave, Mailgun, Twilio …   │ production, staging (own accounts)
        │ stub   HTTP model of the API     │ CI, dev, chaos: controls /state /calls /pay …
        │ fake   in-memory, all capabilities│ unit tests, dev with no account (refused in prod)
        └───────────────────────────────────┘
 contract suite (the same cases) ─► fake ─► stub ─► sandbox (nightly) ─► contract_runs (drift alarm)
 webhooks ◄── stub: replay · forged · stale · late · junk · lose · fail-next-n
```

A call: the worker resolves the tenant's primary account, the guard may rewrite or refuse the recipient, the driver makes the HTTP call and turns *whatever happened* into either a result or one of four error kinds, and the worker's retry policy acts on the kind alone.

## 7. API

```
POST /admin/tenants/{t}/integrations/{category}/test   → 200 { ok, account, warning? } | 422 { kind: "auth" | "permanent" | "transient", message }
GET  /admin/drivers?category=email                     → drivers visible in THIS environment (stand-ins only when env = development)
PUT  /admin/tenants/{t}/integrations/{category}        { driver, priority, config }   secrets go to the vault (42), never here

-- the stub's control surface (the seed's wavestub) --
GET  /state    GET /calls    POST /pay/{invoice}    POST /view/{invoice}    POST /delete/{invoice}
POST /webhook/{event}/{invoice}?variant=replay|forged|stale|late|junk
-- ours, for the scaled-up version --
POST /chaos    { "mode": "http_5xx", "operation": "Create", "count": 3 }     faults by operation, with a seed
```

The first call is the seed's **Test connection** button (for invoicing, the driver's `Check()` says whether the credentials are good and which business they reach). Here it runs when someone saves credentials and then periodically, so a revoked key turns the account's `health` to `auth_failed` before a customer message fails.

## 8. Data model

```sql
CREATE TABLE drivers (
    name        TEXT PRIMARY KEY,                       -- 'wave', 'mailgun', 'fake-invoicing'
    category    TEXT    NOT NULL,
    is_stand_in BOOLEAN NOT NULL DEFAULT false         -- hidden and un-activatable unless env = development
);

CREATE TABLE integration_accounts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   BIGINT   NOT NULL,
    category    TEXT     NOT NULL,
    driver      TEXT     NOT NULL REFERENCES drivers(name),
    priority    SMALLINT NOT NULL DEFAULT 0,            -- 0 primary, 1 first failover, …
    is_active   BOOLEAN  NOT NULL DEFAULT true,
    config      JSONB    NOT NULL DEFAULT '{}',         -- non-secret settings: region, sender domain
    secret_ref  TEXT,                                   -- pointer into the vault (42)
    health      TEXT     NOT NULL DEFAULT 'unknown'
                CHECK (health IN ('unknown','ok','degraded','auth_failed')),
    health_at   TIMESTAMPTZ,
    version     INTEGER  NOT NULL DEFAULT 1,            -- bumped on any change: caches compare this
    UNIQUE (tenant_id, category, priority)
);

CREATE TABLE provider_calls (                           -- sampled; partitioned by day, kept ~14 days
    at          TIMESTAMPTZ NOT NULL,
    tenant_id   BIGINT   NOT NULL,
    driver      TEXT     NOT NULL,
    operation   TEXT     NOT NULL,
    outcome     TEXT     NOT NULL CHECK (outcome IN ('ok','transient','rate_limited','auth','permanent')),
    latency_ms  INTEGER  NOT NULL,
    attempt     SMALLINT NOT NULL DEFAULT 1
) PARTITION BY RANGE (at);

CREATE TABLE contract_runs (                            -- the drift detector
    driver        TEXT NOT NULL,
    target        TEXT NOT NULL CHECK (target IN ('fake','stub','sandbox')),
    suite_version TEXT NOT NULL,
    passed        INTEGER NOT NULL,
    failed        INTEGER NOT NULL,
    ran_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    details       JSONB,
    PRIMARY KEY (driver, target, ran_at)
);
```

The seed has the single-business version: an `integrations` table with one active row per category (migration 0030). The tenant column, the priority and the health columns are the scale-up. `contract_runs` is new: it turns "the stub passed, the sandbox failed" into an alert instead of a surprise.

## 9. Deep dives

### 9.1 The adapter interface and the four kinds of failure

| Option | Pros | Cons |
|---|---|---|
| One fat interface for the category | simplest callers | every driver must implement everything: `ErrNotSupported` everywhere; a screen shows a button that then fails |
| **Narrow core + optional capability interfaces** (the seed) | a driver declares what it really has; callers ask `p.(Sender)`; the UI hides what is missing | callers must handle "not available" |
| One interface per provider | exact | nothing is swappable; every caller knows every provider |

**Choice and why: a narrow core plus optional capabilities.** The seed's invoicing core has ten methods (`Name`, `Check`, `EnsureCustomer`, `EnsureProduct`, `EnsureTax`, `Create`, `Update`, `Approve`, `Void`, `Get`); sending the email itself, recording a payment, receiving webhooks and listing changes are separate small interfaces, with compile-time proof of what a driver claims (`var _ invoicing.Sender = (*Provider)(nil)`).

**Normalised errors.** The provider's words never leave the driver. Four kinds, each with one policy:

| Kind | Examples | What the platform does |
|---|---|---|
| **Transient** | timeout, connection reset, 5xx | retry with jittered backoff, bounded (the seed: six attempts) |
| **Rate-limited** | 429, a `Retry-After` | retry after the hint; slow that provider's lane ([12](12-notification-service.md) §9.3) |
| **Auth** | key revoked, account suspended | **stop retrying**; mark the account `auth_failed`; tell the tenant; never fail over to someone else's account |
| **Permanent** | invalid address, rejected tax rate | dead-letter now; failover will not help |

An *unknown* error is transient, with a ceiling. The seed has the same split in a smaller form: `Permanent(err)` is explicit, everything else is retried.

```go
type Kind int

const (Transient Kind = iota; RateLimited; Auth; Permanent)

type Error struct {
	Kind       Kind
	RetryAfter time.Duration
	Err        error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// the worker decides from the kind alone:
var pe *Error
if errors.As(err, &pe) && pe.Kind == Permanent { return deadLetter(job, err) }
```

**Idempotency.** The worker is at-least-once, so every driver call must be safe to repeat. Where the provider takes an idempotency key, pass ours through. Where it does not, **look before creating**, keyed on a reference we own: the seed's `Draft.Reference` ("a retried create looks for it first") and `EnsureCustomer` (look up by email before creating). The cost is one extra read per create; the alternative is a duplicate invoice after every timeout.

### 9.2 The test-double ladder, and keeping it honest

| Rung | What it is | Catches | Misses |
|---|---|---|---|
| **Fake** | an in-memory implementation of *our* interface (the seed's `fake`: a `Store`, `DEV-0001` numbering, every capability, `View` and `Pay` to play the client) | our application logic, every screen and worker path, with no account | the provider's wire format, auth, pagination, rate limits |
| **Stub** | an HTTP server that speaks the provider's API subset (`wavestub`, `mailgunstub`) so the **real driver** runs | serialisation, auth headers, pagination, error mapping, webhook signatures | anything our reading of the docs got wrong |
| Recorded fixtures | real responses frozen to files | realistic payload shapes | faults you did not record; they go stale silently |
| **Sandbox / test account** | the provider's own test environment | real behaviour | slow, rate-limited, shared state, sometimes absent, sometimes unlike production |
| Production canary | a daily read-only check or an internal test message | the final truth | cannot test destructive paths |

**Choice and why: fake for application tests, stub for driver and chaos tests, and a scheduled check against the real sandbox to keep the stub honest.** The seed describes this exactly: the stub is "a MODEL of Mailgun built from its documentation … the moment that understanding is wrong is `make mailgun-check` against a real account", and the Wave driver, "written from Wave's published schema and documentation, not against a live business", had one bug found and fixed when it was confirmed against Wave's dev business with `wave-check`.

**The contract suite is one table of cases, run against every target** (`-target fake|stub|sandbox`). When the sandbox fails a case the stub passed, the *stub* is wrong: first make the case fail on the stub (copy the real behaviour into it), then fix the driver. `contract_runs` records each result so the nightly job can alert on any `stub: pass, sandbox: fail` pair. The seed's `*check` binaries are the manual version: read-only by default, writes only with an explicit flag and the *test* business, non-zero exit if anything the driver relies on differs.

### 9.3 Chaos: a catalogue, and making it reproducible

| Fault | What it proves | Example |
|---|---|---|
| **Slow** (p50 ×10, long tail) | timeouts are set; threads are not pinned | latency injected per operation |
| **Hang** (no response at all) | the client has a deadline; the worker's lease is longer than it | the nastiest one: nothing to classify |
| **5xx**, **connection reset** | transient retry; a reset *after* the provider acted needs idempotency | reset mid-response |
| **429** with and without `Retry-After` | the lane slows, no retry storm | |
| **Duplicate**, **out-of-order**, **dropped**, **late** webhooks | inbox dedupe, reconcile-by-pull ([18](18-inbound-webhook-receiver.md)) | `replay`, `late`, and a lost one |
| **Forged**, **stale** signature, **junk** body | verification and replay window | the seed's `wavestub` variants |
| **Schema drift** (new enum value, null, extra field) | tolerant parsing, an alert instead of a crash | |
| **Partial success** (a batch where some recipients fail) | per-item results, not all-or-nothing | `bounce…@` inside one batch |

**Reproducible beats random.** Three ways, best first:

1. **Fate by input.** The seed's `mailgunstub.Fates` decides each copy's fate from the **recipient's local part**: `bounce…` fails permanently after 5 s and lands on the bounce list, `defer…` fails temporarily then delivers, `complain…`, `unsub…`, `click…`, `open…` produce those events, `lost…` produces nothing after *accepted* (a fate that never comes), anyone else is delivered. The test knows the outcome before sending, and a real-looking campaign can mix them.
2. **Named controls**: `LoseWebhook(event, email)`, `FailWebhooks(n)`, `Replay()`; or Mailpit's `Recipient:451:100` (every recipient answers 451, a transient SMTP code) and `Recipient:550:100` (permanent), switched on without stopping a container.
3. **Seeded randomness** for soak runs: pick faults from a PRNG seeded with a printed value, so a failing run can be replayed exactly.

```go
// Fate: what the stub does to a recipient. Pure, so the test knows the answer in advance.
func Fate(email string) []Step {
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	switch {
	case strings.HasPrefix(local, "bounce"):
		return []Step{{Event: "failed", Severity: "permanent", After: 5 * time.Second}}
	case strings.HasPrefix(local, "lost"):
		return nil // accepted, then silence: the pull must find it
	}
	return []Step{{Event: "delivered", After: 2 * time.Second}}
}
```

Where to run it: **CI** on every change, on a fake clock so "wait 30 seconds" costs nothing (1,920 scenarios in about a minute); **staging** with continuous low-rate chaos (say 1% of calls faulted) so dashboards and alerts are exercised; **production** only as announced game days, never as a default.

### 9.4 Environment safety: fail closed, above the drivers

The class of incident: staging, holding a copy of real client rows, emails a real customer. Defences, stacked:

- **A guard above every driver** (the seed): `RedirectTo` sends everything to one inbox, `AllowTo` lists the only addresses allowed (anything else is recorded as *Not sent: this environment only messages the addresses in NOTIFY_ALLOW_TO*). It sits in the single `Notify` entry point, so **no driver can bypass it**. A guard inside each driver would be 40 chances to forget.
- **Fail closed.** If the environment variable is missing or unknown, the guard is *on*. Switching it off is an explicit production setting.
- **Stand-ins cannot be active in production**: the registry refuses `fake` at start-up (the seed's `fake` "production refuses"), and Settings → Integrations hides stand-ins unless `APP_ENV=development` (step 7s). Hiding is cosmetic; the refusal is the guarantee.
- **Provider-side test modes** as a second wall: the seed's `mailguncheck -send` sends *in test mode* unless `-live` is passed, and to two addresses of one inbox so you can see both.
- **Secrets only where needed**: the seed gives the provider's credentials to the worker container, not the API ([42](42-credentials-vault.md)).
- **Data hygiene:** staging should not hold real contact data at all, but the guard exists because it often does.

### 9.5 The registry: one driver per tenant, cached, with failover

The seed's `Registry` is built from configuration, can be **live** (asked for the registry in force on every use, so Settings → Integrations changes apply without a restart), and each invoice remembers the provider that made it, so a record created by `fake` is never handed to `wave`. At 100k tenants that becomes:

| Option | Pros | Cons |
|---|---|---|
| Config in the environment (one account per deployment) | trivial | no tenant choice; redeploy to change |
| **Per-tenant rows + an LRU of built drivers, invalidated by `version`** | tenants self-serve; memory bounded (~15 MB working set); changes visible in seconds ([51](51-runtime-settings.md)) | a cache to invalidate |
| A routing service in front of all providers | one place for policy | a new critical hop |

**Choice and why: rows plus an LRU.** A built driver is cheap: credentials plus a pointer to a **shared** HTTP client per provider host (so connection pools are not per tenant). Failover follows `priority` and the error kind: fail over on *Transient* and *Rate-limited* only; never on *Permanent* (a bad address fails everywhere) and never on *Auth* (a tenant's broken account must not silently spill onto a shared one). A circuit breaker per (provider, region) stops 100k tenants hammering a provider that is down ([36 Resilience](../../36-resilience-patterns.md)). `Check()` runs on save and on a schedule, writing `health`. To roll out a new driver version, run it in **shadow** (call the new driver against the stub or sandbox with the same input, compare normalised results, serve the old one's answer) before moving 1% of tenants to it.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (400 integrations):** consistency, not compute. Without an enforced taxonomy and contract suite, 400 adapters have 400 retry policies. Gate merges on the suite; lint that no driver imports another driver's HTTP types.
- **Stub drift:** a provider changes an enum or a rate limit and the stub does not know. The nightly sandbox run and the `stub: pass, sandbox: fail` alert are the only defence; also subscribe to provider changelogs.
- **Retry storms:** a provider outage plus 100k tenants' retries (+30% per the table) can keep it down. Per-provider budgets, jittered backoff, a breaker, and honouring `Retry-After`.
- **Hanging calls:** the worst fault. Every client has a deadline; the job lease is longer than the deadline plus the retry budget; the hung call's outcome is "unknown", so the retry must be idempotent (§9.1).
- **Cache stampede on a registry change:** many pods rebuild a popular tenant's driver at once; coalesce with `singleflight`.
- **Credentials:** an `Auth` failure should alert the tenant once, not once per queued job.
- **Test environments diverging from production:** the guard and fail-closed rules are the control; run an environment self-check at start-up that prints the guard state and refuses to boot a "production" build with stand-ins registered.
- **Single points of failure:** the contract suite's CI is a gate, not a runtime dependency; at runtime the registry cache and the database are the only shared pieces, and the last good config is kept when the database is slow.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| narrow core + optional capabilities | one fat interface | honest drivers, UI that hides what is missing | callers handle "not available" |
| four normalised error kinds | provider-specific handling | one retry, failover and alert policy | a mapping to maintain per driver |
| fake + stub + scheduled sandbox check | sandbox only, or fakes only | fast CI *and* a drift detector | three artefacts per integration |
| fate by input, named controls | random chaos | tests that know their answer; replayable | a convention for test addresses |
| guard above all drivers, fail closed | a check inside each driver | no driver can bypass it | one rule for every channel |
| per-tenant rows + LRU | environment config | self-service, bounded memory | invalidation by `version` |

## 12. Check yourself

1. Why discover optional capabilities with a type assertion instead of a fat interface?
2. A `Create` call times out after the provider received it. What must the driver and the worker assume, and what makes the retry safe?
3. Name one bug a fake can find that a stub cannot, and one that a stub can find that a fake cannot.
4. What does the `bounce…@` convention buy you compared with random fault injection?
5. Where does the recipient guard live, and what does "fail closed" mean for it?

<details><summary>Answers</summary>

1. Each driver declares only what it truly supports (`Sender`, `PaymentRecorder`, …), the compiler can prove it, and callers or the UI simply skip what is missing. A fat interface forces `ErrNotSupported` stubs and buttons that fail when pressed.
2. The call may have succeeded: the outcome is unknown, so treat it as transient and retry, never as "failed, so nothing exists". Safety comes from idempotency: pass the provider's idempotency key if it has one, otherwise look before creating by a reference we own (the seed's `Draft.Reference`, `EnsureCustomer` by email).
3. A fake finds application-logic bugs (a screen or worker path wrongly handling a status) without any account; it cannot see wire-format bugs. A stub finds serialisation, auth-header, pagination and error-mapping bugs because the real driver runs against it; it cannot find places where our understanding of the provider's behaviour is wrong (only the sandbox can).
4. The test knows the outcome before sending (the address decides), so assertions are exact, a mixed batch can contain every fate at once, and a failure is reproducible without a seed. Random injection needs a printed seed to replay.
5. Above every driver, in the single entry point that all messages pass through, so no driver can skip it. Fail closed: if the environment is unknown or the setting is missing, the guard is active; turning it off is an explicit production decision.

</details>

## 13. In Go

- Related lessons: [11 Interfaces](../../11-interfaces.md) (implicit satisfaction, optional capabilities by type assertion, `var _ I = (*T)(nil)`), [12 Errors](../../12-errors.md) (`errors.As`, typed errors with `Unwrap`), [18 Testing](../../18-testing.md) and [40 Testing architecture](../../40-testing-architecture.md) (fakes over mocks, contract tests), [33 Dependency injection](../../33-dependency-injection.md) (the registry as composition root), [36 Resilience](../../36-resilience-patterns.md) (breaker, backoff), [44 Background jobs](../../44-background-jobs-queues.md) (retry policy by error kind), [20 HTTP server](../../20-http-server.md) (a stub is just a handler).
- Stubs with `net/http/httptest.NewServer`: the same handler runs in-process in tests and as a small command for development (the seed's `cmd/wavestub`, `cmd/mailgunstub`). Inject a clock (`func() time.Time`) so "after 30 s" is instant.
- The contract suite: a `[]struct{ name string; run func(t *testing.T, p Provider) }` table and `func TestContract(t *testing.T)` that picks the target from a flag or environment variable, so `go test -run Contract -target=stub` and `-target=sandbox` share every case.
- `golang.org/x/sync/singleflight` for coalescing driver builds; `github.com/sony/gobreaker` for the breaker; a shared `*http.Client` per provider host with explicit timeouts.
- Packages: `net/http`, `net/http/httptest`, `errors`, `context`, `golang.org/x/sync`, `github.com/jackc/pgx/v5`.

## 14. Extensions

- **Record and replay proxy** for providers without sandboxes: capture real traffic once, replay it deterministically, and flag when a recording is older than N months.
- **Provider changelog watcher:** diff published OpenAPI or GraphQL schemas nightly against what the stub models and open a ticket on a breaking change.
- **Per-provider budgets:** token buckets inside the adapter layer ([03](03-rate-limiter.md), the seed's `Throttle` for an SMS number's one-per-second), shared across tenants that share an account.
- **Consumer-driven contracts:** callers publish the calls they depend on; a provider-side change that breaks one fails CI.
- **Tenant-level chaos switch** as a runtime setting ([51](51-runtime-settings.md)): staging tenants with a fault profile, flipped without a deploy.
- **Production canaries:** a daily internal message and read-only call per provider, feeding the same alert path as customer traffic ([50](50-alerting-engine.md)).
- **Webhook signature rotation:** two valid secrets during a window, tested with the `forged` and `stale` variants.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every per-case and per-call figure is an assumption). Tiny in hardware terms, and plainly so: the adapter layer is a library that runs inside other services' pods, and its tests fit on one 8-vCPU runner for about a minute. The cost that belongs to this lesson is the test machinery, mostly an always-on staging copy, and the network bytes of real provider calls.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Contract + chaos suite in CI (every adapter PR; fake and stub only, no network) | 8,000 fake + 8,000 stub cases + 1,920 chaos scenarios per full matrix (§4); ~100 adapter PRs a day (assumed) | CPU | fake 8,000 × ~1 ms = 8 s; stub 8,000 × ~10 ms (in-process `httptest`) = 80 s; chaos 1,920 × 0.2 s (fake clock, §4) = 384 s; total ≈ 470 CPU-s ≈ 8 CPU-min ⇒ ~1 min on 8 vCPU, ~2 min with a cached build. 100 PRs × 2 min = 200 runner-min ≈ 3.3 h/day ≈ 100 h/month | `c7g.2xlarge` (8 vCPU / 16 GiB), ephemeral runners (CodeBuild or self-hosted) on Spot | 0 idle, 1–3 at a time | pure CPU, under 2 GiB of memory, no network (that is the point), so `c`. A Spot reclaim just restarts a 2-minute run |
| Nightly sandbox contract run | 8,000 calls in 2 h = 1.1 calls/s (§4) | the providers' latency and rate limits, not hardware | 1.1 calls/s × ~5 ms of CPU = 0.01 vCPU; the 2 h is waiting | `c7g.large` job, 2 h a night | 1 | the smallest size is already too big; the real budget is each provider's sandbox limit, so pace it to 1–2 calls/s per provider |
| Staging stack with continuous 1 % chaos (app + 40 stubs + Postgres) | ~50 synthetic calls/s (assumed), 1 % faulted (§9.3) | nothing: it exists to keep production's shape (and its alerts) exercised | 40 stubs × ~10 MB in one process = 0.4 GB; the app tier idles below 0.1 vCPU; data is disposable | 3 × `c7g.large` + `db.m7g.large` single-AZ, 50 GB `gp3` | 3 + 1 | three nodes so a zone-spread bug shows up in staging; `m` for a small database; no Multi-AZ because nothing in it is real. Always on, because the chaos must keep tripping dashboards |

Production runs nothing of its own. The registry, the LRU (20k drivers × ~1 KB = 20 MB per pod; 15 MB working set, 300 MB if every driver were resident, §4) and the shared HTTP clients live inside the workers of [12](12-notification-service.md) and [44](../../44-background-jobs-queues.md); the 300k account rows (≈ 100 MB at ~300 B) and `contract_runs` fit in the platform's existing Postgres.

The production cost per provider call is network, not CPU. CPU: ~0.3 ms per call (keep-alive, JSON) ≈ $0.003 per million calls. Bytes: ~5 KB through the NAT Gateway (request + response, assumed) = $0.23 per million at $0.045/GB, plus ~2 KB out = $0.18 at $0.09/GB, so ≈ $0.40 per million calls, 100× the CPU. The +30 % retry amplification of a degraded provider (§4) adds ≈ $0.12 per million. Call volume belongs to the host services, so this is not in the total.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| CI: ~100 h of `c7g.2xlarge` ($0.29/h) + nightly sandbox 60 h of `c7g.large` ($0.073/h) | $30 |
| Staging app: 3 × `c7g.large` (3 × $53) | $160 |
| Staging database: `db.m7g.large` ≈ $120 managed + 50 GB `gp3` ($4) | $120 |
| Production adapter layer (in-process; < 1 GB of rows in an existing database) | ≈ $0 |
| **Total** | **≈ $310** (≈ $210 with 1-year commitments on the $280 of always-on staging) |

- **Biggest line and why:** the staging stack (≈ 90 %), an always-on copy of production's shape so that 1 % chaos keeps proving the alerts work. Levers: two nodes instead of three, or off at night (but then the chaos stops tripping dashboards). The honest comparison is the bytes: one billion real provider calls cost ≈ $400 of NAT and egress, so the dollars are in the calls and the engineers, not in this layer's hardware.
- **What changes at 10×:** 400 integrations. The full matrix is 240,000 cases + 19,200 chaos scenarios ≈ 4,700 CPU-s ≈ 79 CPU-min ≈ 10 min on 8 vCPU: too slow for every PR. Run only the changed adapter's slice on a PR (≈ 12 CPU-s) and the full matrix nightly across 8 × `c7g.2xlarge` on Spot (≈ 75 s). The nightly sandbox run is 80,000 calls, 11 calls/s in total, so per-provider pacing matters more than hardware. 400 stubs ≈ 4 GB still fit one process. Production is unchanged.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:** barely any data, and this is a lesson about code, not storage.
- ~300k `integration_accounts` rows (≈ 100 MB), a few dozen `drivers`, a small `contract_runs`, and a sampled `provider_calls` kept ~14 days (section 15; its row count is not sized in section 4).
- Reads are by key, `(tenant_id, category)`, and a ~20k-driver LRU in each pod absorbs them (15 MB working set), so the database sees saves, cache misses and the `version` check.
- What it needs: `UNIQUE (tenant_id, category, priority)`, a foreign key from account to driver, a few `CHECK`s and a JSON column for non-secret config. Secrets are not here: `secret_ref` points into the vault (42).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen, because the platform already runs it | section 15 puts the account rows in the platform's existing Postgres and gives staging a `db.m7g.large`; the unique key, the foreign key, the `CHECK`s and `jsonb` for `config` all fit, and nothing here strains it |
| MySQL (RDS / Aurora) | ✔ works, nothing material | every feature used exists in MySQL 8 (`UNIQUE`, foreign keys, `CHECK` since 8.0.16, a `JSON` column, day partitions for `provider_calls`) |
| DynamoDB | ✔ works, not worth a second store | `(tenant_id, category)` is a natural key and 300k items is nothing, but the unique `priority` rule would need a conditional write per item and the `drivers` foreign key becomes application code, for no load benefit |

**Pick:** the relational database the platform already runs, PostgreSQL, so this lesson adds no new store. Make staging run the same engine and major version as production (the stack in section 15 does): a chaos environment on a different engine tests a different system. Tie-breakers, if there were no existing database: team skill, tooling, and features you may want later (`jsonb` with a GIN index would answer "which tenants use region X" without a scan).

**If it were MySQL instead:** nothing material. The same tables, a `JSON` column instead of `jsonb`, `AUTO_INCREMENT` instead of an identity column.

**What would flip the choice:** nothing here; at 10× (400 integrations) the account rows are ~3M and still fit one small instance. Only `provider_calls` could change the picture: if it stopped being a sample and became a full call log it would be an append-heavy ledger like [58](58-outbound-click-tracking.md) and belong in a log or analytics store, not with the accounts.
