# 53 · Web Push & the Installable App (PWA)

`🟢 easy` · *Topics:* the Web Push protocol at design level (subscription = endpoint + keys, VAPID, encrypted payload, push services in the middle), a per-device subscription table, sending 5M pushes in 10 minutes, stale-subscription cleanup, TTL / urgency / collapse keys, at-least-once without duplicate spam, push vs in-app vs email and the permission ask, service worker and install facts the backend must respect · *Builds on:* [45 Live In-App Notifications](45-live-notifications.md) (the in-app channel and routing; not repeated), [12 Notification Service](12-notification-service.md) (the pipeline push plugs into), [15 Job Scheduler](15-job-scheduler.md) (range tasks)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [52 Configurable Forms](52-configurable-forms.md) · Next: [54 Programmatic Location Pages](54-programmatic-location-pages.md)

---

> 🌱 **Seed (honeydolist-site):** **a backlog plan, not built** (`docs/backlog/mobile-pwa.md`, added 2026-09-23, "planned, not scheduled"): grep finds no `push_subscriptions`, VAPID, manifest or service worker in the code. What *does* exist is the live channel: an SSE stream (`GET /admin/events`, PLAN steps 7p.2d and 7v.3, `platform/live`) fed by a Postgres `NOTIFY` on one channel that each API `LISTEN`s to, carrying only *what changed* (a kind and ids, never names or amounts); and the notification pipeline of ADR 0008 (`Notify` writes a `notifications` row plus an `outbox` message in one transaction, the worker sends), with per-person alert choices (migrations 0039, 0054, 0065). The plan decides: a PWA, push as a third `Notify` channel, **one subscription row per device** (`endpoint` unique, `p256dh`, `auth`), a **host allow-list** so the worker can't be turned into an SSRF tool, a hand-written service worker that **caches nothing**, scope `/dashboard/` only, `404`/`410` deletes the row, VAPID keys held by the worker only, `tag`/`Topic` so five new leads show as one notification. It also records two traps found on paper: the manifest sits behind an auth gate that redirects, so install fails silently; and an installed iPhone app has its own cookie jar, so a fixed 14-day session signs staff out every fortnight.
> **Here, at scale:** a consumer-facing product with 20M subscribed devices, marketing campaigns to 5M devices in 10 minutes, transactional pushes all day, and 10–20% of subscriptions going stale every year.

## 1. The task

> "Add push notifications to our web app: users install it to their home screen, opt in, and get
> notified even when the app is closed. Marketing wants to push a campaign to five million devices
> in ten minutes, and nobody wants a notification that is about a stale order or arrives twice."

You do not talk to a phone. You talk to a **push service** run by the browser vendor, and it talks
to the device. Most of the design is what that indirection forces on you: opaque endpoints that
silently die, a payload you can't recall, and a permission prompt you only get to show once.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Web push only, or also native (APNs/FCM tokens)? | Different protocols and tables. | Web Push (browser/PWA) only; native tokens are an extension. |
| Who is notified: users, staff, customers? | Consent and volume. | Both staff (transactional) and consumers (opt-in marketing). |
| Transactional vs marketing? | Priorities, TTL, quiet hours. | Both; transactional must not queue behind a campaign. |
| One device or many per user? | Subscription model. | Many; a device can change hands (shared phone). |
| Deep link on tap, badge counts? | Payload and service-worker behaviour. | Yes: tap opens a URL; badge on supporting platforms. |
| What if the device is offline? | TTL and collapse. | Hold briefly (minutes to a day by type), then drop. |
| Delivery guarantee? | Duplicates. | At-least-once to the push service; show-once via collapse keys. |
| Scale? | Fleet size. | 20M devices, 40M routine pushes/day, 5M-device campaign in 10 min. |

## 3. Requirements

**Functional**
- A page can ask permission and register a device; the server stores the subscription against a user.
- Send a notification to a user (all their devices), a segment, or everyone.
- Tap opens the right URL; later pushes about the same thing replace the earlier one.
- Sign-out and deactivation stop pushes to that device at once.
- Preferences: per category on/off, quiet hours, a frequency cap.

**Non-functional**
- Campaign of 5M inside 10 minutes without delaying transactional pushes.
- Dead subscriptions removed automatically; no endless retries.
- The sending service holds the private key; no device can make it call arbitrary hosts.

**Out of scope**
- Rich media, native app stores, SMS fallback logic (see [12](12-notification-service.md)), audience building.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Subscribed devices | 20M | given |
| Subscription table | ~12 GB | 20M × ~600 B (endpoint ~400–500 B, two keys, label, timestamps, indexes) |
| Routine pushes | ~40M/day ≈ 460/s | assume 2 per device per day: 40M ÷ 86,400 |
| Campaign rate | ~8,300/s | 5,000,000 ÷ 600 s |
| In flight during a campaign | ~830 | 8,333/s × ~0.1 s per POST to a push service (assumed) |
| Bandwidth | ~12 MB/s ≈ 100 Mbit/s | 8,333 × ~1.5 KB (headers + encrypted payload under 1 KB) |
| Workers | 8 workers × ~200 goroutines | ~1,600 slots, 2× headroom over 830 |
| Stale subscriptions | 2–4M a year ≈ 5.5k–11k/day | 20M × 10–20% ÷ 365 |
| Wasted sends if never pruned | 250k–500k per campaign | 5M × 5–10% (half a year of decay) |
| Send log | ~4 GB/day | 40M × ~100 B; keep 7 days ≈ 28 GB |

What the numbers tell us: **8k sends/s is a small fleet**, not a hard problem. What is hard is the
*lifecycle* (a fifth of the fleet's addresses die every year, silently), *fairness* (a campaign
must not delay "your driver has arrived"), and the *platform rules* (consent, install, a service
worker that must not serve stale code).

## 5. Plan

1. How Web Push actually flows, and what a subscription is.
2. The subscription table and its lifecycle (register, move, sign out, die).
3. Sending: fan-out, rate, retries, TTL, urgency, collapse.
4. Push vs in-app vs email, and the permission ask.
5. Service worker and PWA install facts that reach the backend.

## 6. High-level design

```
 browser (service worker)                          your servers                              push services
 ───────────────────────                           ────────────                              ─────────────
 pushManager.subscribe(VAPID public key)
   └─ subscription {endpoint, p256dh, auth} ─────► POST /push/subscriptions ─► push_subscriptions
                                                        │
 event / campaign ─► Notify(audience, template) ─► outbox ─► fan-out job: device ranges
                                                        │
                                          send workers (HTTP/2 pools, one per push service)
                                              POST endpoint   Authorization: vapid t=…,k=…
                                                              Content-Encoding: aes128gcm
                                                              TTL / Urgency / Topic
                                                        │ 201 queued   404/410 gone   429/5xx retry
                                                        ▼                                          ▼
 service worker 'push' event ◄──────────────── (device online) ◄─────────────────── push service holds it
   └─ showNotification(); tap → clients.openWindow(url)
```

You never see delivery. A `201` means the push service accepted the message, not that a phone
buzzed. Everything downstream of that is the vendor's.

## 7. API

```
GET    /push/config                       → 200 { "public_key": "<VAPID public key>" }
POST   /push/subscriptions                { endpoint, keys: { p256dh, auth }, label? }  → 201 { id }   (upsert on endpoint)
GET    /push/subscriptions                → 200 [my devices]
DELETE /push/subscriptions/{id}           → 204                                            (sign-out calls this first)
POST   /push/subscriptions/{id}/test      → 202                                            (goes through the real pipeline)
POST   /internal/notifications            { audience, template, data, topic?, ttl, urgency }  → 202 { id }
```

The app calls `register` on **every open** if the browser's current subscription differs from
the server's: that self-heals lost subscriptions, `pushsubscriptionchange` on browsers that don't
fire it, and a rotated key.

## 8. Data model

```sql
CREATE TABLE push_subscriptions (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          BIGINT NOT NULL,
    endpoint         TEXT   NOT NULL,                       -- opaque URL at the push service
    endpoint_hash    BYTEA  NOT NULL UNIQUE,                -- sha256(endpoint): short, uniform unique key
    p256dh           TEXT   NOT NULL,                       -- device public key
    auth             TEXT   NOT NULL,                       -- device auth secret
    label            TEXT,                                  -- "iPhone · Safari"
    vapid_key_id     SMALLINT NOT NULL,                     -- which application server key it was made for
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at  TIMESTAMPTZ,
    failing_since    TIMESTAMPTZ                             -- first consecutive failure; sweep when old
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id);

CREATE TABLE push_sends (
    notification_id  BIGINT NOT NULL,
    subscription_id  BIGINT NOT NULL,
    status           TEXT   NOT NULL CHECK (status IN ('queued','accepted','gone','failed','skipped')),
    attempts         SMALLINT NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (notification_id, subscription_id)          -- the dedupe: one row per (message, device)
);
```

The log is partitioned by day and dropped after 7 days (partitioned, its primary key gains the day
column). On a retried fan-out task the primary key makes `INSERT … ON CONFLICT DO NOTHING` safe:
devices already `accepted` are skipped, rows still `queued` (the worker died mid-send) are re-sent.

## 9. Deep dives

### 9.1 The protocol shape, and what a subscription is

Design-level facts (the RFCs are 8030 for the protocol, 8291 for payload encryption, 8292 for
VAPID):

- The browser's `pushManager.subscribe({ userVisibleOnly: true, applicationServerKey })` returns a **subscription**: an **endpoint** (a URL at the browser vendor's push service) and two keys, `p256dh` (the device's public key) and `auth` (a shared secret). You store all three.
- To send, you `POST` an **encrypted** body to the endpoint with a **VAPID** JWT in `Authorization`, signed with *your* private key and naming your public key. The push service checks the signature, stores the message, and delivers when the device is reachable. It cannot read the payload.
- **The subscription is bound to the VAPID public key it was created with.** Rotating the application server key invalidates *every* existing subscription. Treat the key like a root of trust: one key per environment, `vapid_key_id` on the row, rotate by dual-registering, never casually.
- A subscription is **per browser profile per device**, not per user. One user, many rows; a shared phone can change user: the register call **upserts on `endpoint_hash` and moves the row** to the signed-in user (honeydolist's plan does exactly this).

| Option for the table | Pros | Cons |
|---|---|---|
| **One row per device, unique on `endpoint_hash`** | a `410` kills one row; moves cleanly between users | N rows per user to fan out |
| One row per user with an array of endpoints | one read per user | a single dead endpoint rewrites the user row; no per-device status; harder to move |
| Store only a vendor token and re-create the endpoint on send | tiny | not how Web Push works: the endpoint is the address |

**Choice and why.** Per device, hash-keyed (the endpoint is long and vendor-controlled, and a
unique index on a 450-byte string is wasteful). The worker looks up endpoint and keys **at send
time**, so logs and the outbox carry a subscription id, never the endpoint or keys.

**The SSRF angle.** The worker POSTs to a URL a client supplied. Without a check, a hostile client
registers `http://169.254.169.254/…` and your sender fetches cloud metadata. honeydolist's plan
allow-lists the known push hosts (Apple, Google, Mozilla, Microsoft) and re-checks in the driver.
The cost of an allow-list is a code change when a new vendor appears; the alternative is the
resolve-and-validate dialer from [10](10-webhook-delivery.md). Use both: an allow-list of
*suffixes* plus the dialer that refuses private IPs, `https` only, no redirects.

### 9.2 Sending at scale

A campaign is one logical message and 5M sends. Model it like [15](15-job-scheduler.md)'s range
tasks and [40](40-media-reprocessing.md)'s backfills:

1. The audience resolves to a **snapshot** (a table of subscription id ranges); the campaign becomes tasks of ~5,000 devices each (≈ 1,000 tasks).
2. Workers claim tasks with `SKIP LOCKED`, insert `push_sends` rows (`ON CONFLICT DO NOTHING`), and send with bounded concurrency.
3. **Two lanes**: transactional and campaign, with the campaign lane capped (say 80% of send slots) so a 10-minute blast cannot delay "your table is ready". Same priority-lane idea as [12](12-notification-service.md).
4. **Per push-service limits.** Each vendor throttles you. Keep an HTTP/2 connection pool and a token bucket **per host** ([03](03-rate-limiter.md)); on `429` honour `Retry-After` and slow *that host's* lane only.

| Response | Meaning | Action |
|---|---|---|
| `201` | accepted by the push service | mark `accepted`, set `last_success_at` |
| `404`, `410` | subscription gone | **delete the subscription**; never retry |
| `400`, `413` | malformed or payload too large | permanent for this message; log |
| `401`, `403` | VAPID rejected (key mismatch) | page a human; this is *your* bug, not the device's |
| `429` | you are sending too fast | back off per `Retry-After` |
| `5xx`, timeout | vendor trouble | retry with jittered backoff, within the message's TTL |

**Duplicates.** Push has no idempotency key; a retry after a lost response can deliver twice. Two
defences: the `(notification_id, subscription_id)` primary key stops *your* double sends, and a
**collapse key** (`Topic` header, plus `tag` in `showNotification`) makes a duplicate *replace* the
earlier one on the device and at the push service instead of stacking.

**TTL, urgency, payload.** `TTL` is how long the push service may hold the message for an offline
device: "your driver has arrived" is 60 seconds (afterwards it is false), a promo is 24 hours.
`Urgency` (very-low to high) lets the vendor save battery for low-priority sends. Keep payloads
**small** (the practical limit is about 4 KB; aim for under 1 KB) and **send data, not a story**:
a title, a short body, a URL, and an id the app can re-read, the same "a notification says what
changed, never how" rule honeydolist's live stream follows.

```go
// Illustrative, with github.com/SherClockHolmes/webpush-go (the library the backlog plan names).
resp, err := webpush.SendNotificationWithContext(ctx, payload,
    &webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth}},
    &webpush.Options{
        Subscriber:      "mailto:ops@example.com",
        VAPIDPublicKey:  pub, VAPIDPrivateKey: priv,
        TTL:             60, Urgency: webpush.UrgencyHigh, Topic: "delivery-"+orderID,
    })
// resp.StatusCode → the table above; always resp.Body.Close()
```

### 9.3 Push, in-app or email, and the permission ask

| Channel | Reaches | Costs | Use for |
|---|---|---|---|
| **In-app** ([45](45-live-notifications.md)) | users with the app open | nothing; instant | live updates, unread badges |
| **Web push** | closed app, opted-in device | a consent step; payload limits; vendor trust | time-sensitive, personal ("leave now") |
| **Email** ([12](12-notification-service.md)) | everyone with an address | deliverability work; slow to read | receipts, digests, anything that must be kept |

The escalation ladder is the one from [45](45-live-notifications.md) §9.4: in-app first; if unread
after N minutes and push is on, push; if still unread and it matters, email. What this task adds is
**consent**, because it is the part that decides your reach:

- The browser's permission prompt can be shown, and if the user taps *Block* you can **never ask again** from the page. So ask with a **soft prompt first** ("Get a message when your order ships?" with *Yes / Not now*); only a *Yes* triggers the real browser prompt, and only after a user gesture, never on page load.
- Opt-in rates are low and vary a lot; plan reach from measured opt-ins, not from installs. (Any specific percentage you quote is an assumption.)
- A **preference centre** (per category, quiet hours, frequency cap) is what keeps people subscribed. Count unsubscribes: an annoying push costs you the device for good.
- Quiet hours and caps are evaluated in the **send worker at send time** against the user's time zone, so a delayed message never lands at 03:00.

### 9.4 Service worker and PWA facts that reach the backend

Push needs a **service worker** (the code that wakes to show the notification). Three things the
backend team must get right:

- **The manifest and the service worker file must be served without authentication.** honeydolist's plan found its own trap: its dashboard sits behind a gate that redirects signed-out requests to `/login`, and the browser fetches the manifest **without cookies**, so installing would fail silently. Serve `manifest.webmanifest`, `sw.js` and the icons outside the gate; they contain nothing private.
- **A stale service worker is the classic foot-gun.** One that precaches the app serves yesterday's bundle after a deploy. The plan's answer is a worker that handles `push`, `notificationclick` and `pushsubscriptionchange` and **caches nothing**, served with `Cache-Control: no-cache` so a new version is picked up on the next load, scoped to one path, and *not* installed on the public marketing site where it would cost page speed. If you do cache an offline shell, version it and delete old caches on `activate`.
- **Platform rules** (as of my knowledge, treat as time-sensitive): on iPhone/iPad, web push works only for a site **added to the Home Screen** (iOS/iPadOS 16.4 and later) and the permission request must come from a tap inside the installed app; Android Chrome and desktop browsers allow it from a normal tab. An installed iOS app keeps **its own cookie jar**, separate from Safari: a fixed 14-day session therefore signs people out every fortnight, which is why the plan proposes **sliding sessions** (renew after half the lifetime).

### 9.5 Lifecycle: no pushes to the wrong phone

| Event | What the server does |
|---|---|
| Sign-out on a device | the app unsubscribes (`pushManager.unsubscribe()` and `DELETE`) **before** the logout call |
| Another user signs in on the same device | upsert on `endpoint_hash` moves the row |
| Account deactivated / email changed | delete that user's subscriptions in the same transaction as the sessions |
| `404`/`410` from the push service | delete the row |
| Silent death (no `410`, device off for months) | `failing_since` plus a sweep: no success for 90 days → delete |
| App opened | compare browser subscription and key with the server; re-register on mismatch |

Recipients are resolved **at event time** from current permissions and consent: losing a role
stops the alerts with no cleanup, the property honeydolist's plan wants from "users with the
event's permission who have a device on".

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (50M devices, 80k sends/s): the push services throttle you before your fleet saturates, so per-host buckets and campaign windows matter more than worker count; then the `push_sends` write volume (partition by day, batch inserts) and the audience snapshot step.
- **A vendor's push service is down:** only that host's lane backs off; TTL expires old messages; transactional pushes to other vendors continue.
- **VAPID key mishap** (wrong key deployed): every send is `401/403`. Alert on the *ratio*, and make the dual-key registration the default so a key can be rolled back without orphaning devices.
- **Stale list before a campaign:** run the sweep and prune first; sending to a 10% dead list wastes 500k requests against your per-host rate budget (and a push service may treat a high error rate as abuse; that is theirs to decide).
- **Notification storm from one bug:** per-user caps in the send worker (e.g. 5 per hour for non-critical categories) and a global kill switch ([51](51-runtime-settings.md)).
- **Privacy:** the push service cannot read the encrypted payload, but it sees timing and your endpoint; keep sensitive details out of the payload anyway (a lock screen is public).
- **Single points of failure:** the audience snapshot job, the worker's private key (store it as a secret, [42](42-credentials-vault.md)), the subscription database.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Web Push / PWA | Native apps first | no store, one codebase, immediate updates | iOS install friction, weaker badge/background features |
| One row per device | One row per user | per-device failures and moves | N rows to fan out |
| `endpoint_hash` as unique key | Unique on the URL | small uniform index | a computed column on write |
| Two send lanes + per-host buckets | One FIFO | transactional never waits behind a campaign | tuning two queues |
| Collapse keys + `(message, device)` PK | Trusting retries | duplicates replace rather than stack | one extra row per send |
| Soft prompt before the browser prompt | Prompt on load | a refusal is not permanent | an extra tap, fewer total prompts |
| Service worker that caches nothing | Offline-first shell | no stale-bundle bugs | no offline mode |

## 12. Check yourself

1. What exactly is a push subscription, and why do you not talk to the device?
2. Why one row per device, unique on a hash of the endpoint?
3. List the response codes from a push service and what you do for `201`, `410`, `429`, `403`.
4. How do you stop a 5M-device campaign delaying a transactional push, and a retry showing twice?
5. Why does an authentication gate in front of the manifest break installing, and what does iOS change about push?

<details><summary>Answers</summary>

1. An endpoint URL at the browser vendor's push service plus two keys (`p256dh`, `auth`). You POST an
   encrypted message to the endpoint, signed with your VAPID key; the push service stores it and
   forwards when the device is reachable. Delivery to the device is entirely theirs.
2. A device is the unit that fails (`410`) and moves (shared phone); a user has many. The endpoint is
   long, so a `sha256` makes a compact unique key and the upsert moves the row to the new user.
3. `201` accepted: mark it, set `last_success_at`. `410`/`404`: delete the subscription. `429`: back
   off that host's lane per `Retry-After`. `403`/`401`: VAPID mismatch, page a human (never delete
   devices for it). `5xx`/timeout: retry with jittered backoff inside the TTL.
4. Separate lanes with the campaign lane capped (and per-host token buckets), so campaign slots are
   never all of them. For repeats: `(notification_id, subscription_id)` primary key prevents your own
   double sends, and a `Topic`/`tag` collapse key makes a duplicate replace the earlier notification.
5. Browsers fetch the manifest and service worker without cookies; a gate redirecting to a login
   page returns HTML instead, so install silently fails. Serve them outside the gate. On iOS, push
   works only for a site added to the Home Screen and the prompt must come from a tap in the
   installed app, which also has its own cookie jar (hence sliding sessions).

</details>

## 13. In Go

- Related lessons: [44 Background jobs](../../44-background-jobs-queues.md) (range tasks, retries), [36 Resilience](../../36-resilience-patterns.md) (backoff with jitter, per-host bulkheads), [58 Realtime: WebSockets & SSE](../../58-realtime-websockets-sse.md) (the in-app channel), [57 Web security](../../57-web-security.md) (SSRF, secrets), [68 Rate limiting & throttling](../../68-rate-limiting-throttling.md) (per-host buckets), [63 Networking fundamentals](../../63-networking-fundamentals.md) and [66 Serving Go apps](../../66-serving-go-apps.md) (HTTP/2 pools, serving static files with the right headers).
- One `http.Client` per push host with an `http2`-enabled `Transport` and a bounded `MaxConnsPerHost`; a `golang.org/x/time/rate.Limiter` per host; never share one limiter across vendors.
- Serve `sw.js` with `Cache-Control: no-cache` and `manifest.webmanifest` as `application/manifest+json`; test both **signed out**.
- Packages: `github.com/SherClockHolmes/webpush-go` (or ~150 lines on `crypto/ecdh` and `x/crypto/hkdf` for RFC 8291), `pgx/v5`, `golang.org/x/time/rate`, `golang.org/x/sync/errgroup`.

## 14. Extensions

- **Native tokens** (APNs/FCM) as a second driver and a `kind` column on the subscription; one `Notify`, two transports.
- **Per-user event preferences** as a table (category × channel × quiet hours), the thing honeydolist's plan defers until the second event type.
- **Badge counts** and silent updates: what the payload must carry, and the platform limits.
- **Rich campaigns:** A/B variants, send-time optimisation per user time zone ([27](27-deadlines-escalation.md) §9.5, the 09:00 herd and jitter), and measuring tap-through with a `?n=<id>` on the tap URL ([47](47-web-analytics.md)).
- **Sliding sessions** for the installed app: the security trade-off of renewing a session on use.
- **Offline-capable PWA:** a versioned shell cache and the update flow that avoids serving a stale app.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). Small hardware: 8k sends/s is a modest fleet, and the push services themselves (Google, Apple, Mozilla, Microsoft) do not bill per message, to my knowledge; what we pay for is a few instances, one database and the bytes out.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Send workers (Go: RFC 8291 encryption, VAPID signature, HTTP/2 POST per push host) | campaign 8,333 sends/s + routine 460/s ≈ 8,800/s; ~830 in flight; ~100 Mbit/s | CPU (the crypto) | ~1 ms CPU per send (assumption: ECDH + HKDF + AES-GCM + a fresh ES256 signature + TLS write) ⇒ 8.8 vCPU. N = ceil(8.8 ÷ (0.8 × 2 × ⅔ = 1.07)) = ceil(8.2) = 9. Memory: ~92 in-flight goroutines per node × 8 KiB, a few HTTP/2 pools: under 0.5 GiB. Network: 100 Mbit/s ÷ 9 ≈ 11 Mbit/s each | `c7g.large` (2 vCPU / 4 GiB) on ECS | 9, 3 per zone (49 % busy at the campaign peak, 73 % with a zone down) | per-send work is CPU, not waiting and not bandwidth, so `c`. Many small nodes keep the blast radius at 1/9 and match the lesson's "8 workers". Tasks are retriable (`SKIP LOCKED`, the `(message, device)` key dedupes), so 6 of the 9 can be Spot nodes scaled in for the campaign lane; 3 stay on-demand for transactional pushes |
| Subscription API (Go: register, delete, test, internal notify) | assume ~10 req/s: the app registers on open only when its subscription differs | none (idle); availability | ~0.01 vCPU ⇒ N = ceil(0.01 ÷ 1.07) = 1 → 3 | `c7g.large` on ECS | 3, one per zone | a few small instances, for availability not load; stateless; the sign-out path (`DELETE` before logout) must always answer |
| Postgres (`push_subscriptions` 12 GB, `push_sends` 28 GB over 7 days) | campaign: 8,333 × 2 row-writes (insert `queued`, update `accepted`) ≈ 16.7k row-writes/s ≈ 2.5 MB/s; routine ≈ 920/s | memory (hot set), then CPU | hot set = subscriptions 12 GB + indexes ~1.5 GB + recent log ~4 GB ≈ 18 GB: fits 32 GiB (`shared_buffers` 8 GiB + cache). CPU: 16.7k × ~0.1 ms (batched, 5,000 rows per statement) = 1.7 of 4 vCPU. IOPS: 2.5 MB/s of new pages, near-sequential because the key is (message, device) ≈ 300 page writes/s, far under gp3's 3,000 | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ, gp3 100 GB | 1 primary + synchronous standby | the whole hot set must sit in RAM and `m7g.xlarge` has 16 GiB, so `r`; CPU only peaks for 10 minutes. Multi-AZ because every send reads it and a lost subscription table means every user must be asked for permission again |

Push services are outside the VPC, so the workers' traffic leaves through NAT gateways (workers in public subnets with public IPs would skip the NAT lines, at the price of exposed instances).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Send workers: 9 × `c7g.large` (9 × $53) | $480 |
| Subscription API: 3 × `c7g.large` | $160 |
| Postgres: `db.r7g.xlarge` ≈ $310, Multi-AZ doubles | $620 |
| gp3 100 GB × 2 (standby) × $0.08 | $20 |
| Data out to push services: (40M routine + one 5M campaign a day) × 1.5 KB = 67.5 GB/day × 30.4 = 2.05 TB × $0.09 | $190 |
| NAT gateways: 3 × $33 + 2.05 TB × $0.045 | $190 |
| ALB | $20 |
| **Total** | **≈ $1,680** (≈ $1,240 with a 1-year plan on the $1,260 of instances and RDS; Spot on the campaign nodes lowers it further) |

- **Biggest line and why:** the database, ~38 % ($640 of $1,680), and its load is small: it is the Multi-AZ premium for a 40 GB data set that every send touches. The honest lever is not a smaller node (`m` does not hold the hot set) but accepting that the whole system is about $1.7k a month. The workers could be 3 steady nodes plus 6 nodes started only for the campaign (6 × 0.5 h a day × $0.073/h ≈ $7 even on-demand): ≈ $170 instead of $480.
- **What changes at 10×:** section 10's 80k sends/s: 83k × 1 ms = 83 vCPU ⇒ N = ceil(83 ÷ 4.27) = ceil(19.5) = 20 → 21 × `c7g.2xlarge` (21 × $212 ≈ $4,450), 1 Gbit/s total, ~48 Mbit/s per node, so still CPU. But the push services throttle first, so per-host token buckets and campaign windows decide the real send time, not the node count. Postgres runs out first at the `push_sends` log: ~167k row-writes/s ≈ 25 MB/s ≈ 3,000 page writes/s (the gp3 limit) and ~17 vCPU: go to `db.r7g.4xlarge` with gp3 at ~10k IOPS, or write one row per 5,000-device task plus rows only for failures. Data out grows ~10× to 20 TB a month (≈ $1,850 of egress + ≈ $920 of NAT processing): then public-subnet workers become worth the exposure.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Device subscriptions: 20M rows × ~600 B ≈ 12 GB, unique on `endpoint_hash` (sha256), indexed by `user_id`. Access is by id at send time (~8.8k/s during a campaign), an upsert on every register, a delete on `404/410` (5.5k-11k a day), and "all devices of a user" (sections 4, 8).
- The send log `push_sends`: one row per (notification, device), day partitions kept 7 days ≈ 28 GB. A 5M-device campaign makes 8,333 `queued` inserts and 8,333 `accepted` updates a second, batched 5,000 rows per statement, so ~3 statements a second (section 15).
- Fan-out is range tasks over device ids claimed with `SKIP LOCKED` (9.2): no joins, no aggregates, no search.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `ON CONFLICT (endpoint_hash) DO UPDATE` moves a shared phone's device to the user who signs in, `ON CONFLICT DO NOTHING` on `(notification_id, subscription_id)` makes a retried fan-out safe, `SKIP LOCKED` claims tasks, and day partitions drop in 7 days; the ~18 GB hot set fits 32 GiB. Cost: every `queued → accepted` update leaves a dead row version, so autovacuum has to keep up with today's partition |
| MySQL (RDS / Aurora) | ✔ works, small changes | `ON DUPLICATE KEY UPDATE` for the move, `INSERT IGNORE` (or a no-op `ON DUPLICATE KEY UPDATE`) for the dedupe, `SKIP LOCKED` since 8.0, `BINARY(32)` for the hash. InnoDB updates in place, so no vacuum on the hot log, and the clustered key `(notification_id, subscription_id)` keeps a task's rows adjacent |
| DynamoDB | ✔ works, with key design | 12 GB + 28 GB is tiny and 16.7k writes/s is within reach, but a campaign keyed by `notification_id` puts 8,333 writes/s on one partition key against ~1,000/s: key it by `(notification_id, subscription_id % 64)`. "All devices of a user" needs a GSI on `user_id`, and the range tasks need an ordered key you design |
| Redis (ElastiCache) | ✖ as the record, ✔ for small rebuildable counters | a lost subscription table means asking 20M people for permission again (section 15), so it cannot be a cache; per-host token buckets and per-user caps shared by 9 workers are exactly the counters Redis is for |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.xlarge`, gp3 100 GB, section 15). The subscriptions are the system of record (they carry consent); `push_sends` is a 7-day working log that is dropped by partition, and the audience snapshot is derived. The 16.7k row-writes/s sounds beyond one primary's few thousand durable writes/s, but it is ~3 batched statements a second, ~1.7 of 4 vCPU and ~300 page writes/s (section 15). The two conflict clauses that decide the design exist in both engines, so this is nearly a coin flip.

**If it were MySQL instead:** nothing material. The register upsert returns its id by a `SELECT` on `endpoint_hash` instead of `RETURNING`, the day partitions still need the day in the primary key (the same rule as in Postgres), and the hot log needs no vacuum, a small advantage for a table that takes ~8k updates a second during a campaign.

**What would flip the choice:** not the 10× case: at 80k sends/s the push services throttle first, and writing one log row per 5,000-device task plus rows for failures (section 15) keeps the same engine, a bigger `db.r7g.4xlarge` at most. The subscription table changes store when it outgrows one primary, roughly 100M devices (~60 GB plus indexes against 128 GiB of RAM, a rule of thumb): then shard by `user_id` or move it to DynamoDB keyed by `endpoint_hash`.
