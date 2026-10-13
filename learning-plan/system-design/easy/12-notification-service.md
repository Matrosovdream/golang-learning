# 12 · Notification Service

`🟢 easy` · *Topics:* 202 + queue, priority lanes, provider adapters with failover, idempotency keys, user preferences and legal opt-out, templates, delivery tracking via provider webhooks · *Builds on:* [03 Rate Limiter](03-rate-limiter.md), [10 Webhook Delivery](10-webhook-delivery.md) (retries, dead letters)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [11 Image Upload & Hosting](11-image-upload-hosting.md) · Next: [13 Shopping Cart](13-shopping-cart.md)

---

## 1. The task

> "Design a notification service. Other teams' services call it to send email, SMS and push
> notifications to users. It handles templates, providers, retries and user preferences."

A platform service: its clients are other engineers, its dependencies are third parties with
rate limits and outages, and its failures are either silence or spam.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Channels? | Each has a provider ecosystem and rules. | Email, SMS, mobile push. In-app later. |
| Volume? Peaks? | Queue and provider sizing. | 10M/day; marketing campaigns can burst 5M in an hour. |
| Latency classes? | An OTP and a newsletter are not the same. | Transactional (OTP, receipts) within seconds; bulk within an hour. |
| Must we never send twice? | Client retries will happen. | Yes: idempotent API. |
| Preferences and opt-out? | Legal (CAN-SPAM, GDPR, TCPA for SMS) and product. | Per user, per category, per channel; unsubscribe links; quiet hours for push. |
| Multiple providers? | Failover and cost routing. | Yes: two email providers, two SMS, Apple/Google for push. |
| Templates, localisation? | Rendering responsibility. | Service renders templates with per-locale variants. |
| Delivery tracking? | Bounces, opens, failures from providers. | Yes: ingest provider webhooks; suppress hard bounces. |
| Per-user caps? | Protects users from floods. | Max N marketing messages per day per user. |

## 3. Requirements

**Functional**
- `send(notification)` with template, recipient, channel(s), data, priority, idempotency key.
- Render templates per locale; resolve recipient contact details and preferences.
- Deliver via provider adapters; retry transient failures; fail over between providers.
- Track delivery status from provider callbacks; honour unsubscribes and bounces.

**Non-functional**
- Transactional notifications: p95 under 5 s end-to-end; bulk never delays transactional.
- Exactly one send per idempotency key.
- Survive a provider outage without losing messages.

**Out of scope**
- Campaign authoring and segmentation, analytics dashboards, in-app inbox (extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Average | ~115/s | 10M/day |
| Campaign burst | ~1,400/s for an hour | 5M/h |
| Provider rate limits | e.g. 500/s email, 100/s SMS per account | the queue must absorb the difference: 1,400 in, 500 out → backlog drains in ~2 h |
| Rows | 10M/day × ~1 KB (with rendered subject, status, provider IDs) = 10 GB/day | keep 90 days ≈ 1 TB; archive after |
| Workers | tens of goroutines per channel per node | I/O-bound on provider APIs |

What the numbers tell us: the service is a **buffer between bursty producers and rate-limited
providers**, with a hard requirement that the slow bulk lane never blocks the fast transactional
lane. Durable queues and priority separation are the core.

## 5. Plan

1. The API: accept fast, process async, make it idempotent.
2. The pipeline from request to provider.
3. Priority lanes and provider rate limits.
4. Provider abstraction and failover.
5. Preferences, opt-out, suppression, caps.
6. Templates and tracking.

## 6. High-level design

```
 order service ──POST /notifications {idempotency_key, user, template, channels, data, priority}──► API
                                                                                                  │ dedupe on key
                                                                                                  │ INSERT notifications(status=accepted)
                                                                                                  ▼
                                                                         queues: email.high  email.low  sms.high  sms.low  push.high  push.low
                                                                                                  │
 workers per (channel, lane) ─────────────────────────────────────────────────────────────────────┘
   1. load user: contacts, locale, preferences, suppression list, daily caps
   2. render template(locale, data) → subject/body (HTML + text) / SMS text / push payload
   3. rate-limit per provider (task 03) → provider adapter.Send() → provider message id
   4. retry transient errors w/ backoff; on provider outage (breaker open) → fail over to provider B
   5. update notifications: status=sent, provider, provider_msg_id
                                                                     ▲
 provider webhooks (delivered / bounced / complained / opened) ──► ingest ──┘ status updates, suppression on hard bounce
```

The client gets `202` the moment the row is written. Everything slow (preference lookup,
rendering, provider calls) happens in workers consuming per-channel, per-priority queues.

## 7. API

```
POST /v1/notifications
  Idempotency-Key: ord_8812_receipt
  { "user_id": 42,
    "template": "order_receipt",
    "channels": ["email", "push"],          // or "preferred": true → pick by user preference
    "category": "transactional",            // transactional | account | marketing  (preferences apply per category)
    "priority": "high",                     // high | low
    "data": { "order_id": "8812", "total": "49.90 EUR" },
    "schedule_at"?: "2026-10-14T09:00:00+02:00" }
  → 202 { "id": "ntf_01J9…", "status": "accepted" }
  → 200 (same body) when the Idempotency-Key was seen before

GET /v1/notifications/{id}      → status per channel: accepted | rendered | sent | delivered | bounced | failed | suppressed
POST /v1/users/{id}/preferences { "marketing": { "email": false, "push": true }, "quiet_hours": "22:00-08:00" }
GET  /u/{token}                 unsubscribe landing (token = signed user+category)
POST /providers/{name}/events   provider webhook ingestion (verify signature)
```

## 8. Data model

```sql
CREATE TABLE notifications (
    id              TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    user_id         BIGINT NOT NULL,
    template        TEXT NOT NULL,
    category        TEXT NOT NULL,
    priority        TEXT NOT NULL,
    data            JSONB NOT NULL,
    schedule_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deliveries (                          -- one per (notification, channel)
    id              TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id),
    channel         TEXT NOT NULL,                 -- email | sms | push
    status          TEXT NOT NULL,                 -- queued | sent | delivered | bounced | failed | suppressed
    provider        TEXT,
    provider_msg_id TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    rendered_subject TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (notification_id, channel)
);
CREATE INDEX deliveries_provider_msg_idx ON deliveries (provider, provider_msg_id);  -- webhook lookups

CREATE TABLE user_channel_prefs (
    user_id   BIGINT NOT NULL, category TEXT NOT NULL, channel TEXT NOT NULL,
    enabled   BOOLEAN NOT NULL,
    PRIMARY KEY (user_id, category, channel)
);
CREATE TABLE suppressions (                        -- hard bounces, complaints, global unsubscribes
    channel   TEXT NOT NULL, address TEXT NOT NULL, reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel, address)
);
CREATE TABLE templates (
    key TEXT, locale TEXT, channel TEXT, version INTEGER,
    subject TEXT, body TEXT,                       -- Go text/template or MJML-compiled HTML
    PRIMARY KEY (key, locale, channel, version)
);
```

## 9. Deep dives

### 9.1 Accept fast, send later

Sending synchronously ("call SendGrid, then return 200") couples every caller's latency and
availability to the provider's. A `202 Accepted` after a durable write decouples them: the
caller's transaction is done in milliseconds, the provider can be slow or down, and the caller
can poll or receive a status webhook. The row is written **before** enqueueing (or the enqueue is
the outbox relay, lesson 34) so a crash never loses an accepted notification.

### 9.2 Idempotency

Clients retry on timeouts. Without protection a user gets two receipts or, worse, two OTPs. The
`Idempotency-Key` is unique in `notifications`; a duplicate insert returns the original row with
`200`. Choose keys that encode the *business event*: `ord_8812_receipt`, not a random UUID per
attempt (a random key per attempt is no idempotency at all). Keys are scoped per calling service
to avoid collisions between teams.

Inside the pipeline, workers are at-least-once consumers: a worker may crash after calling the
provider and before marking `sent`. Mitigate by passing the delivery ID as the provider's
idempotency/reference field where supported (most email APIs accept a custom message ID or
dedup key), and by checking status before sending on redelivery.

### 9.3 Priority lanes and provider rate limits

| Design | What happens during a 5M-message campaign |
|---|---|
| one queue per channel | OTPs wait behind 5M newsletters; logins fail for an hour |
| **separate queues per (channel, priority)** with **separate worker pools** | the high lane has its own workers and its own share of the provider quota; OTPs go out in seconds |
| single queue with priority ordering | works until the broker's priority support meets at-least-once redelivery; harder to reason about |

The provider's rate limit (say 500/s) is a shared resource. Reserve a slice for the high lane
(e.g. 200/s guaranteed) and let the low lane use the rest. Implement as two token buckets per
provider (task 03), or one bucket with the high lane allowed to pre-empt. The low lane's backlog
simply drains over hours; that is what bulk means.

### 9.4 Providers: one interface, many adapters, failover

```go
type Sender interface {
    Send(ctx context.Context, msg Message) (providerMsgID string, err error)
}
```

Adapters per provider (SendGrid, SES, Twilio, APNs, FCM) implement it; the worker depends only
on the interface (lessons 11, 29, 32). Around it:

- **Error classification:** transient (`429`, `5xx`, timeout) → retry with backoff; permanent
  (`400` bad address, invalid token) → fail the delivery, maybe suppress the address.
- **Circuit breaker per provider** (lesson 36): when it opens, route to the secondary provider
  instead of failing. Keep DNS/SPF/DKIM configured for both email providers in advance, or failover
  mail lands in spam.
- **Weighted routing** for cost: 80/20 across two SMS providers; per country routing (some
  providers are far cheaper or more reliable in some regions).
- Push specifics: APNs/FCM return per-token results; an "unregistered" token must be deleted from
  the user's devices, or you keep sending into the void.

### 9.5 Preferences, consent, suppression, caps

Resolution order in the worker, before rendering anything:

1. **Suppression list:** hard-bounced or complained addresses are never sent to again, regardless
   of category. Legal and deliverability (providers penalise bounce rates).
2. **Category + channel preference:** a user who turned off marketing email still gets
   transactional email (receipts, password resets); `transactional` cannot be turned off.
3. **Quiet hours** for push/SMS in the user's time zone: hold low-priority messages until morning
   (reschedule, task 15).
4. **Daily caps** per category per user (e.g. 3 marketing/day): a counter per `(user, day)` in
   Redis (task 06); over the cap → `suppressed`, not failed.

The unsubscribe link is a **signed token** (`user_id + category + expiry`, HMAC) so one click
works without login and can't be forged to unsubscribe others. Email headers
`List-Unsubscribe` and `List-Unsubscribe-Post` make Gmail/Apple show a native unsubscribe button,
which is now required for bulk senders.

### 9.6 Templates and tracking

- Templates are versioned per `(key, locale, channel)`; rendering uses the version current at
  send time, recorded in the delivery for debugging ("why did this email look like that?").
- Rendering is **server-side with auto-escaping** (`html/template`): data is untrusted
  (user-supplied names in subjects are a classic injection point). Fall back to the default locale.
- Email: always produce both HTML and plain text; inline CSS; keep under ~100 KB (Gmail clips).
- **Tracking:** providers POST delivery events (delivered, bounce, complaint, open). Verify their
  signature, look up by `(provider, provider_msg_id)`, update status. Hard bounce or complaint →
  insert into `suppressions`. Events arrive out of order and duplicated: apply as a state machine
  that only moves forward.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** provider quotas, not our code. Negotiate higher limits, add
  providers, or accept longer drain times for bulk. Our pipeline scales by adding workers per lane.
- **Provider A down for an hour:** breaker opens, traffic shifts to B; A's in-flight failures
  retry on B. If both are down, messages wait in the queue (durable); transactional latency SLO is
  breached and alerts fire.
- **Database down:** API returns `503` (can't record acceptance → caller retries later with the same
  key); workers pause. Nothing is lost.
- **Hot user:** a bug in a caller sends 10k notifications to one user. Daily caps and a per-user
  rate limit stop the flood; alert on spikes per template.
- **Duplicate webhook events** from providers: idempotent status transitions.
- **Rendering bug** (template references a missing field): fail *that* delivery with a clear error
  and alert; never send a half-rendered message. Validate templates against a schema at publish time.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| `202` + durable queue | synchronous send | caller decoupled from provider latency/outages | callers must poll or receive status |
| per-(channel, priority) queues and pools | one queue | transactional never waits behind bulk | more queues and workers to operate |
| idempotency key in the DB | trusting callers | exactly one send per business event | callers must choose good keys |
| `Sender` interface + breaker + failover | one provider | survive outages, route by cost | two providers to configure and keep warmed |
| suppression + prefs resolved in the worker | at the API | callers don't need to know preferences | work is spent on messages that end up suppressed |
| server-side templates | callers send final content | consistency, localisation, escaping | a template management surface |

## 12. Check yourself

1. Why `202` instead of sending synchronously and returning `200`?
2. Why must the OTP lane be a separate queue *and* a separate worker pool, not just a higher priority in one queue?
3. A caller retries after a timeout. How does the service avoid sending twice, and what makes a *bad* idempotency key?
4. Email provider A starts returning `503`. Walk through what happens to (a) messages in flight, (b) new messages.
5. A user unsubscribed from marketing. Which categories still reach them, and why is that legal and correct?

<details><summary>Answers</summary>

1. Synchronous sending couples every caller's latency and availability to third-party providers,
   which are slow (hundreds of ms) and sometimes down. Writing a durable row and returning `202` makes
   acceptance fast and reliable; the provider call happens in a worker that can retry and fail over.
2. Priority within one queue only decides *order*; the workers consuming it are still shared, so a
   5M-message backlog occupies all of them, and provider quota is consumed by whatever is at the
   head. Separate pools guarantee capacity, and a reserved slice of the provider rate limit
   guarantees throughput for the high lane.
3. The `Idempotency-Key` is unique in the table; the retry's insert conflicts and the service returns
   the original notification with `200`. A bad key is one that changes per attempt (a fresh UUID
   each retry): it is unique every time and therefore dedupes nothing.
4. (a) In-flight attempts fail with a transient error, are retried with backoff, and once the
   breaker for A opens they are routed to provider B. (b) New messages go to B directly while the
   breaker is open; a half-open probe periodically tests A and traffic shifts back when it recovers.
   Nothing is lost; latency may rise.
5. `transactional` and `account` (receipts, password resets, security alerts) still reach them;
   only `marketing` is suppressed. Consent law regulates promotional messages; messages the user
   needs to use the service they signed up for are expected, and blocking them would be a worse
   outcome for the user.

</details>

## 13. In Go

- Related lessons: [11 Interfaces](../../11-interfaces.md) and [29 Structural patterns](../../29-patterns-structural.md) (the `Sender` interface, adapters, decorators for breaker/rate limit), [44 Background jobs](../../44-background-jobs-queues.md) (asynq has priority queues: `Queues: {"high": 6, "low": 1}`), [36 Resilience](../../36-resilience-patterns.md), [41 API design](../../41-api-design-evolution.md) (idempotency keys), [34 Outbox](../../34-event-driven-outbox.md), [33 DI](../../33-dependency-injection.md) (wiring providers).
- Templates: `html/template` for email bodies (auto-escaping), `text/template` for SMS/subjects; embed defaults with `embed.FS`, load overrides from the DB.
- Packages: `hibiken/asynq`, `sony/gobreaker`, `golang.org/x/time/rate`, provider SDKs (`sendgrid-go`, `aws-sdk-go-v2/sesv2`, `twilio-go`, `sideshow/apns2`, `firebase.google.com/go/messaging`).

## 14. Extensions

- **In-app notification center:** a per-user feed with unread counts (task 06) and real-time delivery via SSE (lesson 58).
- **Digests / batching:** "5 new comments" instead of 5 emails; a windowing job that merges pending low-priority notifications per user.
- **Send-time optimisation:** schedule per user time zone (task 15) and quiet hours.
- **Campaign service** on top: segments, throttled fan-out of millions of notifications into the low lane.
- **Deliverability:** domain warm-up, DKIM/SPF/DMARC, bounce-rate monitoring per provider and per sending domain.
- **Device token lifecycle** for push: registration, rotation, pruning on `unregistered`.
