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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption; section 4 has no channel split, so assume 70 % email / 5 % SMS / 25 % push, ~40 KB per rendered email). Tiny hardware: nine small nodes for ~3 vCPU of work, chosen for zone spread and lane isolation.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API + provider-webhook ingest (Go, stateless) | 1,400/s campaign burst + ~115/s base ≈ 1,500 req/s; webhooks ~1,000 events/s (assume ~2 per send) | CPU, small | API 1,500 × 0.4 ms = 0.6 vCPU; webhooks 1,000 × 0.3 ms = 0.3; total 0.9. N = ceil(0.9 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | a JSON parse, one insert and one enqueue per request, so `c`; 15 % busy at peak. The zone rule sets the count |
| Workers, high lane (OTP, receipts) | ~115/s (assume everything non-bulk is transactional) | availability, not load | 115 × 1.5 ms (prefs, render, one TLS call, one update) = 0.17 vCPU. N = 1 → 3 | `c7g.large` | 3 | a separate deployment so a bulk backlog or a bad bulk rollout can never take OTP capacity (section 9.3); 3 % busy is the price of that guarantee |
| Workers, low lane (bulk) | up to 1,400/s in the worst mix (push is not provider-capped; email is capped at 500/s) | CPU, but the provider cap binds first | 1,400 × 1.5 ms = 2.1 vCPU. N = ceil(2.1 ÷ (0.8 × 4 × ⅔ = 2.13)) = 1 → 3. Memory ~1 GiB of 8 (templates, prefs, connection pools). Out: 500 × 40 KB ≈ 25 MB/s ≈ 200 Mbit/s in total | `c7g.xlarge` (4 vCPU / 8 GiB) | 3 | CPU is rendering and TLS, so `c`; adding nodes will not speed bulk email up: at 500/s a 5M burst takes 10,000 s ≈ 2.8 h whatever we run, the provider quota is the bound (section 10) |
| Postgres (`notifications`, `deliveries`, prefs, suppressions) | ≈ 3,900 commits/s at peak (1,500 API + 1,400 sends + 1,000 webhook updates); 10 GB/day ⇒ 1 TB at 90 days + ~0.4 TB indexes | memory, then write IOPS | hot set: provider events land within ~3 days (assumption) ⇒ 3 × (10 GB rows + 4 GB indexes) ≈ 42 GB ⇒ 64 GiB RAM. IOPS ≈ 2 per commit ⇒ 7.8k + ~2k lookups ⇒ `gp3` at 10,000 IOPS, 2 TB. CPU 3,900 × 0.5 ms = 2 vCPU of 8 | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ | 1 primary + synchronous standby | the `provider_msg_id` and idempotency-key lookups are random, so the recent days must sit in RAM: `r`, not `m`. Partition by day and archive after 90 days (section 4). Above 16k IOPS: batch webhook updates, then shard by `user_id` |
| Redis (daily caps, provider token buckets) | ~5M `(user, day)` counters × ~100 B = 0.5 GB; ~3k ops/s | none (tiny) | 0.5 GB × 2 for the fork = 1 GB; 3k ops/s ≪ one core | `cache.m7g.large` (2 vCPU / 8 GiB), ElastiCache | 1 primary + 1 replica | smallest non-burstable node, memory is 12 % used; `r` would buy nothing. Losing the caps resets counters, which is acceptable |

Queues: **SQS**, six queues (channel × lane), nothing to size: ≈ 1.2 requests per message (individual send, batched receive and delete) × 10M/day × 30.4 days × $0.40/M ≈ $150. If the queue were Redis (asynq), the 3.2M-message backlog (section 4) × ~0.6 KB ≈ 2 GB would join the Redis memory above.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.large` (3 × $53) | $160 |
| Workers: high 3 × `c7g.large` ($160), bulk 3 × `c7g.xlarge` ($320) | $480 |
| Postgres: `db.r7g.2xlarge` ≈ $620, Multi-AZ doubles | $1,240 |
| gp3 2 TB + 7k extra IOPS: $160 + $35 = $195, ×2 for the standby | $390 |
| Redis: 2 × `cache.m7g.large` (2 × ≈ $120) | $240 |
| SQS ≈ $150, ALB ≈ $30 | $180 |
| NAT: 3 × $33 + 8.7 TB × $0.045 | $490 |
| Data out: 8.7 TB (email 7M/day × 40 KB × 30.4 days dominates) × $0.09 | $780 |
| **Total** | **≈ $3,960** (≈ $3,220 with 1-year commitments, ≈ 35 % off the $2,120 of servers) |

Not in the AWS bill, and far bigger: provider fees (off the sheet, rough): email ≈ 213M a month × $0.10 per 1,000 ≈ $21k, SMS ≈ 15M × ≈ $0.008 ≈ $120k, push free. The AWS bill is under 3 % of the real cost of this service.

- **Biggest line and why:** the Postgres pair (41 % with its disks), because every notification is ~4 row writes and the last three days must stay in RAM; data out plus NAT together are 32 %, all of it rendered email bodies (a 60 % smaller body, or a provider's template API that takes only variables, would cut both). Lever: batch status and webhook updates to cut commits and IOPS.
- **What changes at 10×:** 14k/s burst, so API + webhooks ≈ 9 vCPU ⇒ N = ceil(9 ÷ 2.13) = 5 → 6 × `c7g.xlarge`; bulk workers 14k × 1.5 ms = 21 vCPU ⇒ N = ceil(21 ÷ 4.27) = 5 → 6 × `c7g.2xlarge`. What really breaks first is provider quota (section 10: 14k/s is 28× one 500/s email account), then Postgres at ~39k commits/s and ~78k IOPS: split webhook status writes from the main table, partition by day, then shard by `user_id`. Data out plus NAT ≈ $12k.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 10M `notifications` a day, ~1 KB each with the rendered subject and status: 10 GB/day, ~1 TB at 90 days plus ~0.4 TB of indexes; ~4 row writes per message and ~3,900 commits/s at the campaign peak (sections 4, 15).
- Every access is by key: the idempotency key, `(notification_id, channel)`, `(provider, provider_msg_id)` for webhooks (random lookups into the last ~3 days, a ~42 GB hot set), preferences by `(user_id, category, channel)`, suppression by `(channel, address)`. There are no joins on the hot path; a support question ("why did user 42 not get the receipt?") touches all four tables.
- Exactly one send per idempotency key (section 9.2) should be a unique constraint, not application logic.
- Daily caps and provider token buckets are tiny and expendable (~0.5 GB, ~3k ops/s).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the two unique keys give exactly-once at the database; the ~42 GB of random `provider_msg_id` lookups wants RAM (`db.r7g.2xlarge`, 64 GiB); `jsonb` for `data`. Cost: a `deliveries` row is updated ~3 times, so dead rows and autovacuum on a ~1 TB table; day partitions dropped at 90 days keep it bounded (section 15) |
| MySQL (RDS / Aurora) | ✔ works, nothing material | every feature used (unique keys, primary-key and secondary-index lookups, forward-only status updates, transactions) exists. InnoDB's clustered key `(user_id, category, channel)` reads all of a user's preferences as one range, and updates happen in place, so no vacuum |
| DynamoDB | ✔ works, a real alternative | every access is by key, items are ~1 KB (far under 400 KB), and random notification ids spread writes so the ~1,000 writes/s per-partition limit is not hit; the idempotency key becomes a conditional put. You give up ad-hoc queries across notifications, deliveries, prefs and suppressions, which is how support works |
| Redis (ElastiCache) | ✔ for caps and rate limits, ✖ as the record | `(user, day)` counters and provider token buckets, ~0.5 GB on `cache.m7g.large`; losing them resets the counters, which section 15 accepts |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.2xlarge`, 2 TB `gp3` at 10,000 IOPS, section 15) as the system of record, Redis on ElastiCache for the counters (lossy, derived) and SQS for the six channel-by-lane queues (a transport, not a store). What decides it: unique constraints for exactly-once, enough RAM for the hot lookups, and one place to answer the support question. Honestly, PostgreSQL and MySQL are interchangeable here; team skill, tooling and the platform default (this track: PostgreSQL) are the tie-breakers.

**If it were MySQL instead:** nothing material. `JSONB` becomes `JSON`; `ON CONFLICT DO NOTHING` becomes `INSERT IGNORE` followed by a read of the original row to return it with `200` (Postgres needs that second read too, since `DO NOTHING` returns no row); `TEXT` keys (`idempotency_key`, `provider_msg_id`, `address`) become `VARCHAR(n)`.

**What would flip the choice:** at 10× (~39k commits/s, ~78k IOPS, section 15) one primary is past its ceiling. First split webhook status writes from the main table and batch them; then, because access is by key, move `deliveries` to DynamoDB or shard by `user_id`. Before that the provider quota breaks first (14k/s against a 500/s email account, section 10), which no database change fixes.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 1B notifications a day (11.5k/s average); campaign burst 140k/s (~150k req/s with the base), i.e. ~98k email/s, 7k SMS/s, 35k push/s; 100 TB of rows at 90 days; ~210 vCPU of bulk workers | in our own stack, Postgres: ~390k commits/s is 39× one primary's ~10k (~780k IOPS), and the ~4.2 TB hot set is 65× its 64 GiB of RAM. The provider quota, first since 10×, is now ~200 email accounts' worth at 500/s each | shard by `user_id` into ~100 Postgres shards, each exactly the section 15 database for 1 % of users (3.9k commits/s, ~42 GB hot set); an idempotent retry carries the same `user_id`, so the unique key stays shard-local; the shard number rides in the delivery id we hand to providers, so a webhook routes without a lookup. Many provider accounts and IP pools with weighted routing, per-account token buckets in a Redis cluster. Give up: ad-hoc cross-user queries (they go to a warehouse) | ≈ $400k/month AWS, linear (Postgres and disks ≈ $163k). Provider fees ≈ $14M (SMS ≈ $12M) are the real bill |
| **1,000×** | 10B a day (more than one per human: a thought experiment); burst 1.4M/s, i.e. ~980k email/s, 70k SMS/s, 350k push/s; ~4,000 nodes | the third parties: ~2,000 email accounts' worth of quota and ~700 SMS accounts, and ≈ $141M a month of provider fees (SMS alone ≈ $120M). Our side is ~4,000 nodes (2,000 Postgres, ~990 bulk workers, ~840 API, ~160 high-lane), so something fails about every hour | change the shape: ~10 **cells** by `hash(user_id)`, each the 100× design with its own queues, shards, sender domains and IP pools behind a `user_id → cell` router, and own the last hop: self-run MTAs for email (warm-up and reputation per cell), direct APNs/FCM HTTP/2 connections, SMS through carrier aggregators on volume contracts. Give up: deliverability work and abuse handling in-house, provider failover becomes IP-pool failover | ≈ $4M/month AWS + ≈ $141M provider fees: the SMS line alone is 30× the AWS bill |
| **10,000×** | 100B a day (about 12 per human, more than anyone reads); burst 14M/s; ~40,000 nodes if unchanged | attention and money, not capacity: ≈ $1.4B a month of provider fees (SMS ≈ $1.2B) for messages that people do not read; the product is the limit | change the problem: a per-user attention budget enforced centrally for every caller (e.g. 5 non-transactional messages a day: 5B users × 5 = 25B, 4× fewer than 100B before anything else); the `(user, day)` counter of section 9.5 becomes the one gate and over-budget sends are `suppressed`, not queued. Digests merge pending low-priority items into one message; a channel ladder (in-app inbox first, then push, then email; SMS only for OTP and security); stop sending to users who have not opened anything in 90 days. Give up: callers' assumption that every event becomes a message, marketing reach, some conversion | ≈ $40M/month AWS + ≈ $1.4B provider fees if nothing changes; at least 4× less from the budget alone, and SMS (85 % of the fees) shrinks further when reserved for OTP |

*Arithmetic:* 140k = 5M/h ÷ 3,600 × 100; 98k/7k/35k = 70/5/25 % of 140k; 3,900 × 100 = 390k commits/s ÷ 10k = 39×; 42 GB × 100 = 4.2 TB ÷ 64 GiB = 65×; 98k ÷ 500 = 196; 1,000×: 980k ÷ 500 = 1,960, 70k ÷ 100 = 700; 1.4M × 1.5 ms = 2,100 vCPU ÷ 2.13 = 987; 900 vCPU ÷ 1.07 = 843; fees $141k × 1,000 = $141M, $3,960 × 10,000 = $39.6M.

**What stays the same at every scale:** `202` after a durable write, the `Idempotency-Key` as a unique constraint (exactly one send per business event), separate queues and worker pools per (channel, priority) so transactional never waits behind bulk, and preferences, suppression and caps resolved in the worker before anything is rendered.

**Say it in an interview:** "At 100× the first thing in my stack to break is Postgres: 390k commits/s is 39× one primary, so I shard by user id into about 100 shards, each today's database, and spread sending over many provider accounts. At 1,000× the providers are the limit, about 2,000 email accounts' worth of quota and $141M a month in fees, so I split into cells and own the last hop: our own MTAs, direct APNs and FCM, SMS aggregators. At 10,000× it is 12 messages per person per day, so I enforce a per-user attention budget, send digests and keep SMS for OTP."
