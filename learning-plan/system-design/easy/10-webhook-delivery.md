# 10 · Webhook Delivery

`🟢 easy` · *Topics:* transactional outbox, at-least-once delivery, exponential backoff with jitter, dead-letter queues, per-endpoint isolation and circuit breaking, HMAC signatures, SSRF · *Builds on:* [03 Rate Limiter](03-rate-limiter.md), [08 Uptime Monitor](08-uptime-monitor.md) (outbound HTTP at scale)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [09 Feature Flags](09-feature-flags.md) · Next: [11 Image Upload & Hosting](11-image-upload-hosting.md)

---

## 1. The task

> "Design our outbound webhook system. When something happens in our platform, like an order
> being paid, we POST an event to the URLs our customers registered. It has to be reliable."

"Reliable" against servers we don't control, that are slow, down, or buggy, and that must not be
able to hurt us or each other.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Event volume? Peaks? | Sizing queues and workers. | 10M events/day (~115/s), peaks 10×. Each event fans out to ~1.5 endpoints on average. |
| How many customer endpoints? | Isolation design. | 100k endpoints. |
| Delivery guarantee? | At-least-once is achievable; exactly-once is not. | At-least-once; customers must dedupe. |
| Ordering? | Per-customer ordering fights with retries. | Best effort; events carry timestamps and sequence numbers. |
| Retry policy and how long? | Backoff schedule, when to give up. | Exponential backoff with jitter, up to ~24 h, then dead-letter. |
| Timeout per attempt? | Slow endpoints tie up workers. | 10 s; 2xx within that is success. |
| Security? | Receivers must verify it was us; we must not be an attack vector. | HMAC-SHA256 signature with per-endpoint secret; HTTPS only. |
| Customer visibility? | Logs, manual retry, endpoint health. | Yes: delivery log per endpoint, replay, auto-disable after sustained failure. |
| Payload: full object or just an ID? | Staleness vs convenience. | Full object snapshot at event time plus IDs. |

## 3. Requirements

**Functional**
- Customers register endpoints (URL, secret, event types).
- Every matching event is delivered at least once; failures retry with backoff; exhausted deliveries go to a dead-letter state.
- Deliveries are signed; customers can view attempts and replay.
- Endpoints failing for a long time are disabled and the customer notified.

**Non-functional**
- No event is lost once the business transaction committed.
- One slow or dead endpoint does not delay other customers' deliveries.
- Delivery latency normally under a few seconds.

**Out of scope**
- Inbound webhooks, event schema evolution, per-customer SLAs.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Deliveries | ~175/s average, ~1,750/s peak | 115 events/s × 1.5 endpoints, peak 10× |
| In-flight at peak | ~1,750 × 0.5 s ≈ 900; worst case with 10 s timeouts ≈ 17k | rate × duration |
| Retries | if 5% fail and retry ~6 times: +50% load | retries are not free |
| Delivery rows | 15M/day, ~500 B each (with response snippet) → 7.5 GB/day | keep 30 days ≈ 225 GB |
| Workers | a few hundred goroutines per node, a few nodes | mostly waiting on the network |

What the numbers tell us: modest throughput, but the **work is I/O-bound against untrusted
servers**. The design is about isolation and bookkeeping, not raw speed.

## 5. Plan

1. Make event capture reliable: the outbox pattern, so an event is never lost between the business write and the queue.
2. Fan out events to endpoint subscriptions as individual delivery jobs.
3. Deliver with timeouts; retry with backoff; dead-letter; auto-disable.
4. Isolate endpoints from each other.
5. Security: signing (for them), SSRF protection (for us).
6. Visibility: delivery log, replay.

## 6. High-level design

```
 order service ── BEGIN; UPDATE orders…; INSERT INTO outbox(event) ; COMMIT ──► Postgres
                                                                                   │
                                                 outbox relay (poll / CDC) ────────┘
                                                        │ publish event
                                                        ▼
                                                   events topic
                                                        │
                                            fan-out worker: for each subscription matching
                                            (customer, event_type) → INSERT delivery(pending)
                                                        │
                                                   delivery queue  (partitioned by endpoint_id)
                                                        │
                            delivery workers ── POST with 10 s timeout, HMAC header ──► customer endpoint
                                 │   2xx → delivered
                                 │   else → attempts+1, next_attempt_at = backoff, requeue (delayed)
                                 │   attempts exhausted → dead_letter; N consecutive failures → disable endpoint, notify (task 12)
                                 ▼
                           deliveries table (status, attempts, last response)  ──► customer dashboard, replay
```

An event is written in the same transaction as the business change. A relay moves it to the
queue. A fan-out step turns one event into one delivery per subscribed endpoint. Workers POST,
record the outcome, and either finish or schedule a retry.

## 7. API

Customer-facing:

```
POST /webhooks/endpoints   { "url": "https://shop.example/hooks", "events": ["order.paid","order.refunded"] }
  → 201 { "id": "ep_7f3", "secret": "whsec_…" }      (secret shown once)
GET  /webhooks/endpoints/{id}/deliveries?status=failed&cursor=…
POST /webhooks/deliveries/{id}/replay              → 202
POST /webhooks/endpoints/{id}/rotate-secret        → 200 (old secret valid for 24 h)
```

What we send:

```
POST https://shop.example/hooks
Content-Type: application/json
Webhook-Id: dlv_01J9…                 ← unique per delivery (same across retries of it)
Webhook-Timestamp: 1760345600
Webhook-Signature: v1=hex(HMAC-SHA256(secret, timestamp + "." + body))
User-Agent: ourplatform-webhooks/1.0

{ "id": "evt_01J9…", "type": "order.paid", "created_at": "…", "sequence": 8812,
  "data": { "order": { … snapshot … } } }
```

Success is any `2xx` within 10 s. Everything else, including a `3xx` (we don't follow redirects),
is a failure.

## 8. Data model

```sql
CREATE TABLE outbox (                       -- in the producing service's database (lesson 34)
    id          BIGSERIAL PRIMARY KEY,
    event_id    UUID NOT NULL,
    type        TEXT NOT NULL,
    payload     JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE TABLE endpoints (
    id              TEXT PRIMARY KEY,
    customer_id     BIGINT NOT NULL,
    url             TEXT NOT NULL,
    secret_enc      BYTEA NOT NULL,         -- encrypted at rest
    event_types     TEXT[] NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',   -- active | disabled
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deliveries (
    id              TEXT PRIMARY KEY,                 -- Webhook-Id
    event_id        UUID NOT NULL,
    endpoint_id     TEXT NOT NULL REFERENCES endpoints(id),
    status          TEXT NOT NULL,                    -- pending | delivered | failed | dead
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_status     INTEGER,
    last_error      TEXT,
    last_body_snip  TEXT,                             -- first 1 KB of the response, for debugging
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at    TIMESTAMPTZ,
    UNIQUE (event_id, endpoint_id)                    -- fan-out idempotency
);
CREATE INDEX deliveries_due_idx ON deliveries (next_attempt_at) WHERE status = 'pending';
CREATE INDEX deliveries_endpoint_idx ON deliveries (endpoint_id, created_at DESC);
```

## 9. Deep dives

### 9.1 Never losing the event: the outbox

The naive flow, "commit the order, then publish to the queue", has a gap: a crash between the
two loses the event; publishing first and then failing the commit emits a phantom. Writing the
event to an **outbox table in the same transaction** removes the gap: either both commit or
neither. A relay then publishes outbox rows (polling `WHERE published_at IS NULL`, or via
change-data-capture from the WAL) and marks them published. The relay may publish twice after a
crash, which is why everything downstream is idempotent. Lesson 34 covers this in code.

### 9.2 Fan-out and idempotency

One event, several subscriptions. The fan-out worker inserts one `deliveries` row per matching
endpoint with `UNIQUE (event_id, endpoint_id)`, so a replayed event creates no duplicates. The
delivery `id` is generated once and reused for every retry of that delivery, so the receiver can
dedupe on `Webhook-Id` even when we send the same delivery twice.

Document loudly: **receivers will see duplicates.** The right receiver code is "if I've seen this
`Webhook-Id`, return 200 and do nothing".

### 9.3 Retries, backoff, giving up

| Attempt | Delay | Cumulative |
|---|---|---|
| 1 | immediate | 0 |
| 2 | ~1 min | 1 min |
| 3 | ~5 min | 6 min |
| 4 | ~30 min | 36 min |
| 5 | ~2 h | 2.6 h |
| 6 | ~6 h | 8.6 h |
| 7 | ~15 h | ~24 h → dead-letter |

- **Exponential** so a dead endpoint costs us little; **jitter** (±20%) so thousands of deliveries
  to one recovering endpoint don't retry in lockstep.
- Classify responses: `5xx`, timeouts and connection errors retry; `4xx` (except `408`/`429`) is a
  client bug that will not fix itself, so fail fast and surface it in the dashboard. Honour
  `Retry-After` on `429`.
- **Dead letter:** the delivery stays queryable and replayable for 30 days; nothing is silently dropped.
- **Auto-disable:** an endpoint failing every attempt for, say, 3 days or 500 consecutive deliveries
  gets `status = disabled` and the customer gets an email (task 12). Deliveries to a disabled endpoint
  are recorded as `dead` immediately. Re-enabling can replay the backlog.

Scheduling the retry is a delayed job: `next_attempt_at` in the table with an index, picked up by
a dispatcher (task 15), or a delayed queue if the broker supports it.

### 9.4 Isolation: a slow endpoint must only hurt itself

With one shared queue and a pool of workers, an endpoint that takes 10 s per request can occupy
every worker and stall everyone. Options:

| Technique | Effect |
|---|---|
| **per-endpoint concurrency cap** (e.g. ≤ 8 in flight) | a slow endpoint holds at most 8 workers, not 800 |
| **circuit breaker per endpoint** (lesson 36) | after N failures, stop trying for a cooling period; attempts fail instantly instead of burning 10 s each |
| **partitioned queue by endpoint** | work for one endpoint queues behind itself, not behind others |
| **separate lanes** for retries vs first attempts | a retry storm doesn't delay fresh deliveries |
| hard per-attempt timeout | the worst any attempt can cost is 10 s |

Use all of them. They are cheap and they compose.

### 9.5 Security in both directions

**For the receiver** (prove it was us, prevent replay):
- `Webhook-Signature: v1=HMAC-SHA256(secret, timestamp.body)`; the receiver recomputes over the
  *raw* body and compares in constant time.
- The timestamp in the signed string lets the receiver reject old deliveries (> 5 min), defeating replays of captured requests.
- Secret rotation: sign with both old and new for 24 h (`v1=…,v1=…`).
- HTTPS only; we verify certificates. No `http://` endpoints, ever.

**For us** (an attacker registers an endpoint pointing at *our* internals: SSRF):
- Resolve the hostname and refuse private, loopback, link-local and metadata ranges
  (`10/8`, `172.16/12`, `192.168/16`, `127/8`, `169.254/16`, `::1`, `fd00::/8`).
- Re-check **after DNS resolution at connect time** (DNS rebinding: the name resolves to a public
  IP during validation and to `169.254.169.254` a second later). Use a custom dialer that validates the IP it is about to connect to.
- Don't follow redirects (a public URL redirecting to an internal one).
- Egress from a dedicated network segment with no route to internal services.
- Cap response body read (1 KB) and total time; never store or display raw responses without escaping.

### 9.6 Payload: thin or fat

| Payload | Pros | Cons |
|---|---|---|
| **thin** (`{ "type": "order.paid", "order_id": "…" }`) | small, never stale, no leaking of fields the endpoint shouldn't see; receiver fetches current state via API with proper auth | one extra API call per event; the receiver must have API credentials |
| **fat** (full object snapshot) | one hop; easy to consume; shows the object *as it was at event time* | staleness if processed late; bigger; must apply per-endpoint field permissions |

**Choice: fat with a version field**, the common convention (Stripe, GitHub), plus IDs so the
receiver can refetch when it needs the latest state. Document that the snapshot is point-in-time.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the `deliveries` table at 150M rows/day; partition by day and drop
  old partitions. Workers scale horizontally; queue partitions grow with them.
- **A giant customer:** 50% of all events to one endpoint. Per-endpoint caps throttle them; give
  them a dedicated lane with a higher cap rather than letting them share.
- **Queue down:** the outbox keeps accumulating; the relay catches up. Nothing is lost; latency grows.
- **Our worker crashes mid-POST:** the attempt may have reached the receiver. The delivery is still
  `pending` (we never recorded success), so it will be retried: a duplicate, which is exactly the
  at-least-once contract.
- **Endpoint returns 2xx but slow (9 s):** that's success; its per-endpoint cap limits the damage.
- **Mass outage of a popular host (a cloud region):** thousands of endpoints fail together; the
  breaker and backoff spread the recovery; make sure the retry lane can't starve first attempts.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| outbox + relay | publish after commit | no lost or phantom events | a relay process and duplicate publishes |
| at-least-once + `Webhook-Id` | attempting exactly-once | achievable, simple | receivers must dedupe |
| exponential backoff to ~24 h, then dead-letter | retry forever | bounded cost, visibility | a 25-hour outage loses live delivery (replay exists) |
| per-endpoint caps + breakers | one shared pool | isolation | more state per endpoint |
| HMAC with timestamp | no signature / basic auth | authenticity, replay protection | receivers must implement verification |
| fat payload | thin | one hop, point-in-time snapshot | staleness; field-level permissions |

## 12. Check yourself

1. Why is "commit, then publish" wrong, and what exactly does the outbox fix?
2. Why can't we promise exactly-once delivery? What do we promise instead, and what must the receiver do?
3. How does one endpoint taking 10 s per request hurt other customers in a naive design, and name three mitigations.
4. What is the SSRF risk here, and why is validating the URL at registration time not enough?
5. What does the receiver verify from the headers, and what attack does the timestamp prevent?

<details><summary>Answers</summary>

1. A crash between the commit and the publish loses the event; publishing first and then failing
   to commit announces something that never happened. Writing the event into an outbox table inside
   the same transaction makes "business change happened" and "event recorded" atomic; a relay
   publishes it afterwards, possibly more than once.
2. A POST can succeed at the receiver while our acknowledgment is lost (timeout, crash), so we
   cannot know whether to resend; resending risks a duplicate, not resending risks a loss. We promise
   at-least-once with a stable `Webhook-Id` per delivery; the receiver stores seen IDs and treats
   repeats as already processed.
3. Shared workers: each delivery to the slow endpoint holds a worker for 10 s, so a few hundred
   deliveries occupy the whole pool and everyone else waits. Mitigations: per-endpoint in-flight cap,
   per-endpoint circuit breaker, queue partitioned by endpoint, separate retry lane, hard timeouts.
4. A customer registers `http://169.254.169.254/…` or an internal service URL and we POST to it from
   inside our network, leaking metadata or triggering internal actions. Validation at registration
   can be bypassed by DNS rebinding (the name later resolves to a private IP) or by a redirect, so the
   IP must be re-checked at connect time, redirects disabled, and egress network-isolated.
5. It recomputes `HMAC-SHA256(secret, timestamp.rawBody)` and compares with `Webhook-Signature`
   in constant time, proving the body is unchanged and came from the secret's holder. Checking that
   the signed timestamp is recent prevents an attacker from replaying a captured, validly signed
   request later.

</details>

## 13. In Go

- Related lessons: [34 Event-driven & the outbox](../../34-event-driven-outbox.md) (the relay), [44 Background jobs](../../44-background-jobs-queues.md) (retries, backoff, dead-letter with asynq), [36 Resilience](../../36-resilience-patterns.md) (circuit breaker, timeouts, jitter), [57 Web security](../../57-web-security.md) (SSRF), [15 Sync & context](../../15-sync-context.md) (per-endpoint semaphores).
- Signing: `crypto/hmac` + `crypto/sha256`, `hmac.Equal` for constant-time compare. Dialer: `net.Dialer` with a `Control` func or a custom `DialContext` that resolves and checks the IP; `http.Client{CheckRedirect: func(...) error { return http.ErrUseLastResponse }}`.
- Packages: `net/http`, `crypto/hmac`, `hibiken/asynq`, `pgx`, `golang.org/x/sync/semaphore`.

## 14. Extensions

- **Replay UI:** replay by time range or event type for one endpoint; what rate limit protects the receiver?
- **Ordering per endpoint** as an option: a single in-flight delivery per endpoint; measure the throughput cost.
- **Event type versioning** (`order.paid.v2`) and per-endpoint version pinning (lesson 41).
- **mTLS** or OAuth client credentials instead of HMAC for enterprise customers.
- **Inbound webhooks** (the mirror image): verify, dedupe, respond fast, process async.
- **Delivery SLOs**: p95 time-to-first-attempt, success rate per endpoint, dashboards for customers.
