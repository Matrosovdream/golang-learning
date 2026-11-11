# 45 · Live In-App Notifications

`🟢 easy` · *Topics:* SSE vs WebSocket vs long-poll vs web push, connection cost per node, the stream as a doorbell (ids, not data), interest-based fan-out, per-user inbox + unread counter, `Last-Event-ID` replay and `resync`, grouping, subscription routing with role defaults, in-app → push → email escalation, reconnect storms · *Builds on:* [12 Notification Service](12-notification-service.md) (channels, idempotency), [10 Webhook Delivery](10-webhook-delivery.md) (outbox), [15 Job Scheduler](15-job-scheduler.md) (delayed escalation), [06 View & Like Counter](06-view-like-counter.md) (counters)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [44 Bulk Email Campaign Sender](44-email-campaign-sender.md) · Next: [46 Roles & Permissions (RBAC)](46-rbac.md)

---

> 🌱 **Seed (honeydolist-site):** the dashboard learns of changes through **one SSE stream**, `GET /api/v1/admin/events` (PLAN step 7p.2d; `platform/live`). A change is a Postgres `pg_notify` on a single channel, written **inside the transaction that made it true** (so it is announced if and only if it commits); every API process `LISTEN`s and hands each event to its open streams through a `Hub` with a 64-event buffer per stream. A slow stream, or a LISTEN reconnect, is answered with one `resync` event ("read everything on screen again"): nothing is replayed. A payload is **ids only**, never data (kept under 7,900 bytes; ids dropped beyond that), and the client re-reads through endpoints that check who is asking. Each event kind has a required permission, and per-person events carry `To`. The stream opens with `retry: 3000` and an `event: ready`, then a `: still here` comment as heartbeat (step 7v.3: a toast stack, five at most, six seconds each, no polling). Who is told what: `staff_inbox` (migration 0054, step 25.6; rows **grouped** per record + action + minute, with a partial unique index), `user_alerts` (0039, extended in 0054: `in_app`, `email = off | instant | daily`, `scope = all | mine`), `role_alert_defaults` (what a role starts with), `alert_digest` for the daily email, a Notifications tab on a person (0065, step 42), and the owner's list kept only as the **fallback** when nobody on staff covers an action. Email fan-out writes one outbox row per address with a dedupe key. The `mobile-pwa` backlog adds Web Push per device. Real size: a handful of staff.
> **Here, at scale:** 5M concurrent connected users, 200k live deliveries/s at peak, tens of millions of registered users, a notification center that survives a node dying.

## 1. The task

> "Design in-app notifications for a web product: a bell with an unread count, live toasts while the app is
> open, and a push or an email when it is not. Users choose what they hear about."

The easy half is a table and an endpoint. The interesting half is the **long-lived connection** (5M of them), the **fan-out** (who is on which server), the **missed event** (a laptop closed for ten minutes) and the **routing rules** (who should hear what, where, and when).

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Decides node count and the bus. | 100M registered, 20M daily, **5M concurrent** at peak; 200k deliveries/s at peak. |
| What is a "delivery"? | Mixing live ticks with stored items distorts storage. | A message to one user's open stream. About 1 in 10 is an inbox item (stored); the rest are ticks ("unread: 4", "record changed") that are never stored. |
| Direction? | One-way is far simpler than two-way. | Server → client only. Actions go through ordinary REST. |
| Guarantee? | Notifications must not silently vanish, but a duplicate toast is harmless. | At-least-once to the inbox; the live stream is best-effort with recovery. |
| Closed tab? | Decides push and email. | Web Push for installed/permitted devices, then email, by preference. |
| Preferences? | Drives the routing code. | Per event: in-app, push, email (off, instant, daily), scope (all, mine); role defaults; quiet hours. |
| Retention? | Storage. | 30 days of inbox items. |
| Broadcast to a whole organisation? | A 100k-user audience cannot be 100k rows. | Yes, via topics, not per-user rows. |
| Ordering? | Cross-event ordering is expensive. | Per user, by inbox id; no global order. |

## 3. Requirements

**Functional**
- Live toasts and a bell with the unread count while the app is open (any number of tabs).
- An inbox: list, mark read, mark all read; related events on the same record grouped.
- Preferences per event and channel, with role defaults and an admin editing someone else's.
- Push for closed browsers/installed apps; a delayed email if still unread; a daily digest.
- After reconnecting, the client catches up without a full reload.

**Non-functional**
- Live delivery p95 under 1 s from commit to screen for a connected user.
- A node dying costs its users one reconnect, not a thundering herd.
- Never tell a user about something they may not read (permission check at routing time).
- No unbounded memory per slow client.

**Out of scope**
- Chat and typing indicators (two-way, medium track), notification content authoring, native FCM/APNs (an extension).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Concurrent connections | 5M | given |
| Memory per connection | ~20 KB | Go goroutine + buffers + kernel socket state, TLS terminated at the LB (assumption; measure it) |
| Connections per node | 250k ⇒ ~5 GB | chosen for blast radius: 1M per node fits in RAM (20 GB) but a node loss then reconnects 1M clients |
| Nodes | 30 | 5M ÷ 250k = 20, ×1.5 so a lost availability zone (1 of 3) is survivable |
| Heartbeats | ~170k writes/s cluster-wide | 5M ÷ 30 s; ~5.7k/s per node; bandwidth negligible (tens of bytes each) |
| Live deliveries | 200k/s peak | assumption: 10k events/s × average 20 recipients |
| Egress | ~60 MB/s ≈ 480 Mbit/s at peak | 200k × ~300 B payload; ~16 Mbit/s per node |
| Inbox items | ~20k/s peak, ~5k/s average | 10% of deliveries are stored |
| Inbox storage | ~65 GB/day; ~1.9 TB for 30 days | 5k/s × 86,400 = 432M rows × ~150 B |
| Reconnect after a node loss | 250k clients | with jittered retry over 30 s ≈ 8.3k/s; with a fixed 3 s retry ≈ 83k/s |

What the numbers tell us: bandwidth and the bus are easy; **memory per connection and reconnect behaviour** decide the fleet; and the stored volume is large enough that retention must be a design item, not an afterthought.

## 5. Plan

1. Transport: how the browser keeps a channel open, and what one node can hold.
2. Fan-out: from "something happened" to the right node's open stream.
3. Missed events: ids, replay, resync, unread counters, grouping.
4. Routing: event → recipients → preferences → channels, with escalation.
5. Reconnect storms and deploys.

## 6. High-level design

```
 services ──tx: inbox rows + outbox 'notify' (same transaction)──► Postgres: inbox_items · inbox_state · prefs · watchers
                                                                       │ outbox relay (SKIP LOCKED), after commit
                                                                       ▼
                                                          Router: event → recipients (watchers ∪ role audience)
                                                                  → permission + prefs + quiet hours → channels
                       ┌────────────── in-app ─────────────┬──────── push ────────┬────── email / digest ──────┐
                       ▼                                   ▼                      ▼                            ▼
              NATS subject user.<id>              push worker (VAPID)      delayed 'escalate' job        mail service ([12])
                       │ interest-based routing
                       ▼
        Gateway nodes: SSE, ~250k connections each, user → connections map
                       │   id:/event:/data: frames, heartbeat every 25 s
        browsers ◄─────┘   (L7 LB, HTTP/2, idle timeout well above the heartbeat)
```

Walk through one event: a comment on a ticket is committed together with an inbox row for each watcher and an outbox row. The relay reads the outbox, the router resolves recipients and applies preferences, publishes `user.<id>` for those with `in_app`, and schedules an escalation job for the rest. A gateway holding that user's stream receives it and writes one SSE frame. A user who is offline sees it later in the inbox.

## 7. API

```
GET  /v1/stream                 Accept: text/event-stream     Last-Event-ID: <inbox id>
       event: ready     data: {"unread": 4}                   ← first frame; carries the counter, so most clients need no REST call
       event: notify    id: 90210   data: {"id":90210,"event":"ticket.comment","entity":"ticket","entity_id":"T-7","count":3}
       event: tick      data: {"kind":"record.changed","ids":["T-7"]}      (no id: never replayed)
       event: resync    data: {}                              ← "you may have missed something: re-read"
       : still here                                            ← heartbeat comment every 25 s
GET  /v1/inbox?after=<id>&limit=50         → { items[], next }
POST /v1/inbox/read      { up_to: id } | { ids: [..] }         → 200 { unread }
GET|PUT /v1/notification-prefs             { events: { "ticket.comment": { in_app, push, email: "off|instant|daily", scope: "all|mine" } },
                                             quiet_hours: { from, to, tz } }
POST /v1/push/subscriptions { endpoint, keys }                 → 201 (one row per device)
DELETE /v1/push/subscriptions/{id}                             → 204
POST /internal/events  { type, entity, actor, audience_hint }  → 202 (writes the outbox row; never delivers inline)
```

Payloads carry **a kind and ids**; the client reads the record through endpoints that check permission. That is the seed's rule ("what changed, never how") and it keeps secrets out of the bus.

## 8. Data model

```sql
CREATE SEQUENCE inbox_id_seq;
CREATE TABLE inbox_items (
    user_id     BIGINT NOT NULL,
    id          BIGINT NOT NULL DEFAULT nextval('inbox_id_seq'),
    event       TEXT   NOT NULL,                         -- 'ticket.comment'
    entity_type TEXT   NOT NULL, entity_id TEXT NOT NULL,
    title       TEXT   NOT NULL, detail TEXT, url TEXT,
    actor_id    BIGINT,
    count       INTEGER NOT NULL DEFAULT 1 CHECK (count > 0),
    bucket      TIMESTAMPTZ NOT NULL,                    -- the minute it happened in: the grouping key
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ,
    PRIMARY KEY (user_id, id)
) PARTITION BY HASH (user_id);                           -- 16 partitions: an inbox read touches exactly one
CREATE UNIQUE INDEX inbox_group_key ON inbox_items (user_id, entity_type, entity_id, event, bucket) WHERE read_at IS NULL;
CREATE INDEX inbox_unread ON inbox_items (user_id, id DESC) WHERE read_at IS NULL;

CREATE TABLE inbox_state (user_id BIGINT PRIMARY KEY, unread INTEGER NOT NULL DEFAULT 0, last_read_id BIGINT NOT NULL DEFAULT 0);

CREATE TABLE notification_prefs (
    user_id BIGINT NOT NULL, event TEXT NOT NULL,
    in_app BOOLEAN NOT NULL DEFAULT true, push BOOLEAN NOT NULL DEFAULT false,
    email TEXT NOT NULL DEFAULT 'off' CHECK (email IN ('off','instant','daily')),
    scope TEXT NOT NULL DEFAULT 'all' CHECK (scope IN ('all','mine')),
    set_by BIGINT, set_at TIMESTAMPTZ,                   -- an admin's change to someone else is attributable
    PRIMARY KEY (user_id, event)
);
CREATE TABLE role_defaults (role_id BIGINT NOT NULL, event TEXT NOT NULL, in_app BOOLEAN NOT NULL DEFAULT true,
    push BOOLEAN NOT NULL DEFAULT false, email TEXT NOT NULL DEFAULT 'off', scope TEXT NOT NULL DEFAULT 'all',
    PRIMARY KEY (role_id, event));
CREATE TABLE watchers (entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, user_id BIGINT NOT NULL,
    PRIMARY KEY (entity_type, entity_id, user_id));
CREATE TABLE push_devices (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, user_id BIGINT NOT NULL,
    endpoint TEXT NOT NULL UNIQUE, keys JSONB NOT NULL, last_ok_at TIMESTAMPTZ);
```

The grouped insert is the seed's trick, one statement: `INSERT … ON CONFLICT (user_id, entity_type, entity_id, event, bucket) WHERE read_at IS NULL DO UPDATE SET count = inbox_items.count + 1, detail = EXCLUDED.detail, updated_at = now()`. Fifty comments in a minute on one ticket are one row and one bell entry. (Postgres needs the conflict target to match the partial index predicate exactly.)

## 9. Deep dives

### 9.1 Transport, and what one node can hold

| Option | Pros | Cons |
|---|---|---|
| **SSE** (`text/event-stream`) | plain HTTP; the browser reconnects by itself and sends `Last-Event-ID`; one-way is all we need; works with HTTP/2 | text only; HTTP/1.1 allows ~6 connections per origin, so use HTTP/2 or share one stream across tabs |
| WebSocket | two-way, binary | reconnect, ping/pong and resume are yours to write; overkill for one-way |
| Long-poll | works behind anything | every event costs a new request; at 5M users the re-poll rate dominates |
| Web Push | works with the browser closed | goes through the browser vendor's service; best-effort latency; one subscription per *device*; iOS only for an installed app (16.4+), per the seed's mobile backlog |

**Choice and why: SSE for open tabs, Web Push for closed ones, email for the rest; long-poll only as the fallback while the stream is down.** WebSockets arrive with a two-way feature such as chat ([lesson 58](../../58-realtime-websockets-sse.md)).

Operational facts that bite: Go's server `WriteTimeout` would cut a stream, so clear the write deadline per response (the seed does: `http.NewResponseController(w).SetWriteDeadline(time.Time{})`); send a heartbeat comment about every 25 s because load balancers and proxies commonly close idle connections near 60 s; turn off proxy response buffering; raise the file-descriptor limit well above the connection count. One tab = one connection, so a user with three tabs is three entries in a per-user map.

### 9.2 Fan-out: from `NOTIFY` to a bus

The seed's Hub is a broadcast: every API process hears every event and `Publish` loops over **every** open stream, which is perfect for dozens of dashboards and hopeless for 250k. Two changes make it scale: the hub becomes a **map keyed by user**, and the bus routes by interest instead of broadcasting.

```go
type Conn struct {
	out    chan Event  // bounded: 64
	resync atomic.Bool // set when the client fell behind
}

type Hub struct {
	mu    sync.RWMutex
	conns map[int64]map[*Conn]struct{} // user → their open streams
}

func (h *Hub) Deliver(userID int64, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.conns[userID] {
		select {
		case c.out <- ev:
		default: // a slow consumer must never block the publisher: drop its backlog, send one resync
			c.resync.Store(true)
		}
	}
}
```

| Option | Pros | Cons |
|---|---|---|
| **Postgres `LISTEN/NOTIFY`** (the seed) | zero new infrastructure; announced iff the transaction commits | one shared channel and queue; every listener gets everything; an 8,000-byte payload cap; commits that notify briefly serialise on a shared lock, so notify-heavy write paths stop scaling at modest rates |
| Redis pub/sub | simple; per-user channels | fire-and-forget; a single node's throughput is the ceiling; use sharded channels beyond that |
| **NATS core, one subject per user** (`user.<id>`) | interest-based: a node subscribes when a user connects, so a publish reaches only servers that hold that user; built for hundreds of thousands of small messages a second | 5M subscriptions across the cluster and reconnect churn to watch; at-most-once (we recover through the inbox) |
| Kafka | durable, replayable | consumer groups are per partition, not per user; the wrong shape for 5M live endpoints |

**Choice and why: outbox + NATS subjects per user.** Keep the seed's key property, "announced only if committed", by writing the inbox rows and an outbox row in the same transaction and letting a relay publish after commit ([10](10-webhook-delivery.md)). Duplicates are harmless: the client ignores an `id` it has already shown. **Broadcast audiences** (every member of a 100k-user organisation) are not 100k publishes or 100k rows: publish once to `org.<id>` (every connection also subscribes to its org subject), and store the event **once** with a per-user read cursor, materialising only what a user opens (fan-out on read). Per-user rows are for targeted items (mentions, assignments, alerts); topics are for announcements.

### 9.3 Missed events: ids, replay, resync, counters

The stream is a **doorbell, not the mail**. Truth lives in the inbox table and the record endpoints; the stream only says "look". That is why the seed can get away with `resync` and no replay, and why the same rule survives scale.

- **Stored items** carry the inbox `id` as the SSE `id:` field. On reconnect the browser sends `Last-Event-ID`; the server replays `SELECT … FROM inbox_items WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT 200`, a single-partition index range. If more than 200 are missing, or the id is older than retention, send `resync` and let the client call `GET /inbox`.
- **Ticks** (record changed, unread count) have no id and are not replayed; `resync` covers them by telling the client to re-read what is on screen.
- **Unread** is a counter row (`inbox_state.unread`) updated in the same transaction as the insert or the mark-read, never `COUNT(*)` per page view at 20M daily users. The `ready` frame carries it, so a reconnecting client usually needs no REST call at all.
- **Grouping** (the seed's `bucket`) turns bursts into one row with a `count`, which also cuts storage and notification fatigue.
- **Retention:** 30 days. Hash partitions by user keep inbox reads to one partition, but deleting ~65 GB a day is vacuum work: do it in small batches off-peak. At 10× this belongs in a TTL store keyed by user.

| Option | Pros | Cons |
|---|---|---|
| No replay, `resync` only (seed) | trivial; nothing to keep | every long gap becomes a REST re-read |
| **Replay from the inbox by id, `resync` as the escape hatch** | short gaps cost one cheap query; the escape hatch bounds the work | an id order to maintain; replay window to size |
| A durable per-user stream (Kafka/JetStream) | exact resume | 5M consumers' state; far more machinery than the inbox already provides |

**Choice and why: inbox replay with `resync` as the escape hatch.** The inbox already exists for the bell, so replay is free.

### 9.4 Subscription routing and escalation

Resolution order for "does user U hear about event E on channel C?": the user's own row in `notification_prefs`, else the **role default** (what a role starts with), else the system default. The seed adds three rules worth keeping:

1. **Permission and `is_active` are checked at routing time** by the same query that finds subscribers (a join over `user_roles → role_permissions`), so a demoted or deactivated person falls silent by itself.
2. **A fallback** exists so a business-critical event never goes unheard: if no one on staff has an email for the action, use the owner's list. The seed's `Covered` check does exactly this.
3. **Email fan-out is one outbox row per address** with a deduplication key (event id + address), so a retried fan-out sends nobody twice ([12](12-notification-service.md)).

At scale the recipient query is the hot path: 10k events/s each resolving ~20 users. Cache the resolved *audience* per `(tenant, event, entity scope)` with a version stamp that changes on a role or preference edit, and look up prefs in batches (`WHERE user_id = ANY($1)`).

**Escalation** keeps people informed without spamming them:

| Step | When | Cancelled by |
|---|---|---|
| In-app (live + inbox) | always, if `in_app` | — |
| Push | immediately if the user has no open stream (no gateway holds a subscription for them) and `push` is on | the user being online; same `tag` collapses to one lock-screen entry ("3 new requests") |
| Email, instant | a delayed job at `now + 5 min`, checks `read_at IS NULL` when it runs ([15](15-job-scheduler.md)) | the item being read |
| Email, daily | rows collected into a digest table, sent by a morning job | — |
| Quiet hours | non-urgent items wait until the user's local morning (zone stored on the user) | urgent events skip it |

**Choice and why: ordered escalation with a check at execution time.** The check at send time ("is it still unread?") is what prevents the classic "I read it in the app and got an email anyway". Web Push is **per device**: a `410 Gone` removes only that device's row, as the seed's backlog notes.

### 9.5 Reconnect storms and deploys

Losing a node returns 250k clients to the load balancer. The seed's `retry: 3000` would bring them all back within about 3 s (83k connections/s, each with an auth check). Three defences:

- **Jittered retry**: send a randomised `retry:` (for example 3–30 s) in the last frame before a planned drain and use client-side exponential backoff with jitter on error.
- **A cheap handshake**: authenticate with a short-TTL cached session and answer `ready` with the unread counter, so reconnecting costs one cache read, not a database query.
- **Drain, don't kill**: a deploy removes the node from the LB, sends the jittered `retry`, waits for streams to close, restarts, and moves to the next node, 5% of the fleet at a time.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the bus subscription churn and the inbox write path (200k stored items/s). Shard the inbox by user across clusters, move to a TTL store, and cap per-user write rate (grouping and a per-user notification limit, see [03](03-rate-limiter.md)).
- **Slow client:** bounded buffer, drop and `resync` (never block, never grow).
- **Bus down:** the inbox is the truth; clients fall back to polling `GET /inbox?after=` every 30 s with jitter until the stream recovers, which is the seed's "poll only while the stream is down" rule.
- **Node loss:** 250k reconnects, jittered; the other 29 nodes absorb them (that is why the fleet is sized for a lost zone).
- **Router backlog:** the outbox grows but nothing is lost; alert on the oldest unrelayed row's age (as the seed does for queued mail: warn past five minutes).
- **Permission change mid-stream:** an open stream is authorised at connect time; on a role change, publish a `session.changed` tick so the gateway re-checks or closes the stream.
- **Privacy:** ids only on the bus; titles in the inbox row are rendered for the recipient who may read them.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| SSE (+ Web Push, email) | WebSockets | native reconnect and resume, plain HTTP | one-way only |
| Ids on the stream, data via REST | full payloads on the stream | permission checks in one place; tiny frames | an extra read after a notification |
| Per-user NATS subjects, outbox relay | `LISTEN/NOTIFY` broadcast | routing by interest; headroom | a bus to run; relay delay (ms) |
| Inbox replay + `resync` | durable per-user stream | replay with no new system | a bounded replay window |
| Counter row for unread | `COUNT(*)` per request | O(1) bell | a counter to keep consistent in-transaction |
| 250k connections per node | 1M per node | small blast radius | more nodes |
| Topics for broadcast | a row per recipient | one write for 100k users | fan-out on read; per-user read cursor |

## 12. Check yourself

1. Why does the seed's stream carry only a kind and ids, and what does that buy at 5M connections?
2. How many gateway nodes for 5M connections, and why not 5 nodes of 1M each?
3. A laptop reconnects after 10 minutes. How does it catch up, and when is `resync` sent instead?
4. Why is the seed's `Hub.Publish` unsuitable at 250k streams per node, and what replaces it?
5. A user reads a ticket notification in the app 2 minutes after it arrived. How do you make sure no email goes out 5 minutes later?

<details><summary>Answers</summary>

1. The stream is a doorbell: it says *something changed* and the client re-reads through endpoints that check permission, so authorisation lives in one place, no sensitive data crosses the bus, frames stay tiny (~300 B, ~60 MB/s at 200k deliveries/s) and a lost frame is repaired by a re-read. The cost is one REST call after a notification, which the `ready` frame's unread counter often avoids.
2. 5M ÷ 250k = 20 nodes; ×1.5 so a lost availability zone is survivable gives 30. Five nodes of 1M fit in RAM (~20 GB each) but one node loss reconnects 1M clients at once, and each node is a much larger blast radius; smaller nodes bound the reconnect storm.
3. The browser sends `Last-Event-ID`; the server replays inbox rows with `id` greater than that for the user (limit 200, one partition). If more are missing, the id is older than retention, or the connection lost its bus subscription meanwhile, it sends `resync` and the client re-reads the inbox and anything on screen. Ticks are never replayed.
4. It loops over every open stream for every event, O(connections) per event, which at 200k events/s per fleet is billions of iterations a second. Replace it with a map keyed by user (`map[userID]set of conns`) so a delivery is O(that user's tabs), fed by interest-based routing (NATS subject per user) instead of a broadcast.
5. Schedule the email as a delayed job that re-checks `read_at IS NULL` when it runs (and, for pushes, collapse by `tag`). Reading marks `read_at` in the same transaction as the unread counter, so the job finds it read and exits.

</details>

## 13. In Go

- Related lessons: [58 Realtime: WebSockets & SSE](../../58-realtime-websockets-sse.md) (streaming handlers, flushing, heartbeat), [15 Sync & context](../../15-sync-context.md) (per-connection goroutines, shutdown), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (the relay), [44 Background jobs](../../44-background-jobs-queues.md) (delayed escalation), [43 Authorization & RBAC](../../43-authorization-rbac-multitenancy.md) (permission checks at routing time), [38 Caching](../../38-caching-patterns.md) (audience cache), [63 Networking fundamentals](../../63-networking-fundamentals.md) (idle timeouts, file descriptors).
- Handler skeleton: `rc := http.NewResponseController(w)`; `rc.SetWriteDeadline(time.Time{})`; set `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`; `fmt.Fprintf(w, "id: %d\nevent: notify\ndata: %s\n\n", …)` then `rc.Flush()`; `select` on the connection's channel, a heartbeat ticker and `r.Context().Done()`.
- Measure before trusting the 20 KB/connection budget: open 100k idle streams in a test, read `runtime.MemStats` and RSS.
- Packages: `net/http`, `github.com/nats-io/nats.go`, `github.com/jackc/pgx/v5`, `github.com/SherClockHolmes/webpush-go` (VAPID Web Push).

## 14. Extensions

- **Presence ("online now", who is editing):** TTL keys and heartbeats; compare with a database table swept by a worker (the seed's `UNLOGGED` presence table).
- **Typing indicators and chat:** two-way, ordering and delivery receipts: the medium-track chat task.
- **Native mobile push (FCM/APNs):** a second driver beside Web Push, device tokens and invalidation feedback.
- **Per-user notification rate limit** and smart batching ("12 new comments") before the router, to protect users and the bus.
- **Multi-region gateways:** where is a user's connection, and how does a publish find it across regions?
- **Digest ranking:** which 5 of 200 unread items go in the morning email?

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Gateway nodes (Go, SSE) | 5M concurrent streams, 200k deliveries/s, 5M ÷ 30 s = 170k heartbeats/s (§4) | memory (connections), not CPU | 250k connections per node once a zone is lost × ~20 KB = 5 GB (§4); × 1.5 GC headroom = 7.5 GB + ~1 GiB OS ≈ 8.5 of 16 GiB. CPU then: (10k deliveries + 8.3k heartbeats)/s × 30 µs = 0.55 vCPU, plus a zone-loss reconnect surge 1.67M ÷ 30 s ÷ 20 nodes = 2.8k/s × 0.2 ms = 0.56 ⇒ 1.1 of 2 vCPU | `r7g.large` (2 vCPU / 16 GiB, $78) in an Auto Scaling group | 30 (10 per zone) = 5M ÷ 250k = 20, × 1.5 (§4) | ~8 GiB of RAM for ~1 vCPU of work is the `r` ratio; `c7g.2xlarge` has the same 16 GiB but 8 vCPU we would never use, at 2.7× the price ($212 vs $78); fewer, bigger nodes only make a node loss a bigger reconnect storm (9.5). 167k connections per node normally, 250k after a zone loss |
| Load balancer (TLS and HTTP/2 end here, §6) | 5M active connections, 60 MB/s, 8.3k new connections/s in a storm (§4) | the LB's per-connection billing and limits | LCUs per hour = the largest of: connections 5M ÷ 3,000 = 1,667; bytes 216 GB/h ÷ 1 GB = 216; new connections 8.3k ÷ 25 = 333. Connections decide: ≈ 1,667 LCU at the peak | ALB; an NLB passing TLS through bills per ~100k connections (far cheaper) but puts TLS on the gateways: more CPU and ~+25 KB per connection (not in the sheet, verify) | 1, spans 3 zones | the LB, not the servers, is where 5M connections cost money: 1,667 LCU × $0.008 × 730 ≈ $9.7k a month at the peak. Each client stream is one LB-to-node TCP connection, so 250k sockets per node need at least 4 LB source IPs per node (64k ports each); the ALB adds IPs as it scales, but confirm with AWS before 5M |
| Bus (NATS core, subject per user) | 200k publishes/s × ~300 B = 60 MB/s; 5M subscriptions (§4, 9.2) | memory (subscription table), then network | assume ~500 B per subscription × 5M = 2.5 GB on every server (each learns all interest) × 1.5 = 3.75 of 8 GiB. Traffic: 60 MB/s in × ~2.7 copies (in, route hop 2/3, out) ≈ 160 MB/s ≈ 1.3 Gbit/s cluster, 0.43 per node; CPU 200k × 2.7 × 3 µs ≈ 1.6 vCPU in all | `m7g.large` (2 vCPU / 8 GiB), self-run (no AWS-managed NATS; sharded ElastiCache pub/sub is the AWS-native stand-in, not priced) | 3, one per zone | the subscription table needs ~4 GiB, so `m` (8 GiB), not `c` (4 GiB) and not `r` (16 GiB unused); with a zone down 2 nodes carry 0.8 vCPU each of 2 (40 %) |
| Router, outbox relay, push and escalation workers (Go) | 10k events/s × ~20 recipients = 200k recipient resolutions/s (§4, 9.4); push assumed 2k/s | CPU | 200k × 15 µs (cached audience, prefs, publish) = 3 vCPU; 10k × 0.1 ms = 1; push 2k × 0.5 ms (TLS to the vendor) = 1 ⇒ 5 vCPU; N = ceil(5 ÷ 2.13) = ceil(2.35) = 3 | `c7g.xlarge` | 3, one per zone | stateless CPU work; the audience cache is a few MB in process, so `c` |
| Postgres (inbox, prefs, watchers, outbox) | 20k stored items/s peak, 5k average; 1.9 TB for 30 days, ~2.7 TB with 3 indexes (assumption) (§4) | IOPS (random index pages), then storage | ≈ 3 IOPS per stored item (3 indexes on random users, data ≫ RAM; assumption) ⇒ 20k × 3 = 60k IOPS; gp3 stops at 16k. io2 would be 60k × $0.065 + 3 TB × $0.125 ≈ $4,300 per copy, $8,550 Multi-AZ. CPU 20k × 0.1 ms ≈ 2 vCPU | `i4i.4xlarge` (16 vCPU / 128 GiB / 3.75 TB NVMe, ≈ $1,000), self-managed Postgres; managed: Aurora PostgreSQL or RDS on io2 | 1 primary + 2 replicas, one per zone | past ~16k IOPS the sheet says NVMe: 2.7 TB fits (72 %) and local IOPS are far above 60k, for $3,000 instead of $8,550+. NVMe dies with the instance, so the two streaming replicas are the durability, and they serve `GET /inbox` and replay |
| Redis (session and audience caches) | handshakes 8.3k/s in a storm, audience lookups; ~30k ops/s (assumption) | memory | ~3 GB of sessions and audiences (assumption) × 1.7 overhead = 5 GB; `maxmemory` ≤ 70 % ⇒ ≥ 7.3 GiB; 30k ops/s is under one core's ~100k | `cache.r7g.large` (2 vCPU / 16 GiB, ≈ $156) | 1 primary + 1 replica | memory per core: `r`; a miss costs a database read, so no bigger |

Egress is the other place connections cost money. Deliveries: 60 MB/s at peak, but the average is ~25 % of it (the lesson's 5k vs 20k stored items), so ~15 MB/s ≈ 38.9 TB a month. Heartbeats: 5M ÷ 30 s at ~100 B on the wire (SSE frame, HTTP/2, TLS, TCP/IP; assumption), with average concurrency half the peak: 2.5M ÷ 30 × 100 B = 8.3 MB/s ≈ 21.6 TB a month. The keep-alive costs half as much as the content, so its cadence is a cost lever.

**Rough monthly bill (≈, on-demand; average concurrency 2.5M, deliveries 25 % of peak)**

| Line | ≈ $/month |
|---|---|
| Gateways: 30 × `r7g.large` (30 × $78) | $2,340 |
| Bus: 3 × `m7g.large` (3 × $60) | $180 |
| Router and workers: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Postgres: 3 × `i4i.4xlarge` (3 × $1,000) | $3,000 |
| Redis: 2 × `cache.r7g.large` (2 × $156) | $310 |
| ALB: 2.5M ÷ 3,000 = 833 LCU × $0.008 × 730, + $16 | $4,880 |
| Egress, deliveries: 38.9 TB × $0.09 | $3,500 |
| Egress, heartbeats: 21.6 TB × $0.09 | $1,940 |
| Cross-zone bus hop: 38.9 TB × ⅔ × $0.02 (clients connect to their own zone's NATS node) | $520 |
| NAT + Web Push: 3 × $33 + 500 pushes/s × 1.5 KB ≈ 1.9 TB × ($0.09 + $0.045) | $360 |
| **Total** | **≈ $17,400** (lines sum to $17,350; ≈ $15,200 with a 1-year Savings Plan on the $6,150 of compute) |

- **Biggest line and why:** the network, not the servers. The ALB, egress and cross-zone traffic are ≈ 65 % of the bill and the 30 gateways only ≈ 13 %: 5M open connections are billed per connection and per byte, almost none of it CPU. Levers: heartbeat cadence (every +10 s saves ≈ $480 a month), and at much larger scale an NLB or own L4 tier instead of an ALB.
- **What changes at 10×:** 50M concurrent ⇒ 300 × `r7g.large` (50M ÷ 250k = 200, × 1.5; ≈ $23k); the ALB would be ≈ 8,300 LCU average, ≈ $49k a month, so move to an NLB with TLS on the gateways; 50M subscriptions is ~25 GB per NATS server, so shard users across NATS clusters; 200k stored items/s ≈ 600k IOPS, so shard the inbox over ~10 NVMe clusters or move it to a TTL store; egress grows ~10× (≈ $54k).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The stored part is the inbox: ~5k items/s on average and 20k/s at peak, 432M rows a day (~65 GB), so 30 days is ~13B rows and 1.9 TB (~2.7 TB with 3 indexes) (sections 4, 15). The hot reads are one user's range: replay `WHERE user_id = $1 AND id > $2 ORDER BY id LIMIT 200`, and the unread list.
- Writes must be atomic with their neighbours: inbox row, unread counter and outbox row in one transaction ("announced only if committed"), and a grouped insert that folds fifty comments in a minute into one row (section 8, 9.3).
- Small relational lookups sit on the routing path: prefs, role defaults, watchers and the permission join, batched with `WHERE user_id = ANY($1)` (9.4). The live fan-out (5M streams, 200k deliveries/s) is not the database's job: NATS carries it (9.2).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen (self-managed on NVMe in section 15; Aurora or RDS on `io2` are the managed alternatives) | the grouped insert is one statement because of the partial unique index `inbox_group_key` and `ON CONFLICT (…) WHERE read_at IS NULL DO UPDATE`; the partial `inbox_unread` index holds only unread rows; 16 hash partitions by `user_id` keep a replay inside one partition; `SKIP LOCKED` for the outbox relay |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | the grouping rule needs a generated-column unique index, and there is no partial index for `inbox_unread`, so that index covers all ~13B live rows. In its favour: the clustered key `(user_id, id)` stores a user's items together, so replay and the inbox list are one contiguous range. Missing `LISTEN/NOTIFY` costs nothing, because 9.2 already replaces it with outbox plus NATS |
| DynamoDB | ✔ for the inbox alone (the 10× move), ✖ for the rest | the inbox is pure key access: partition key `user_id`, sort key `id`, ~150 B items (limit 400 KB), and 20k writes/s over 100M users never nears 1,000 writes/s on a partition; TTL expires the 30 days without the ~5k deletes/s that 9.3 calls vacuum work. Costs: inbox row plus counter become a `TransactWriteItems`, grouping needs its own key design, and prefs, watchers and the permission join stay relational |
| Redis (ElastiCache) | ✔ as a derived cache (sessions, audiences), ✖ as the inbox | 1.9 TB × 1.5–2 for per-key overhead (cheat sheet) is 3–4 TB of RAM for a 30-day inbox; the unread counter is already atomic with the insert in the inbox database |

**Pick:** PostgreSQL, self-managed on `i4i.4xlarge` (16 vCPU / 128 GiB / 3.75 TB NVMe) with one primary and two streaming replicas in different zones, as in section 15. NVMe because the inbox is IOPS-bound: ~3 IOPS per stored item × 20k/s = 60k IOPS against gp3's 16k cap, and the replicas are both the durability and the readers for `GET /inbox` and replay. The managed alternatives (Aurora PostgreSQL, or RDS on `io2` at ≈ $8,550 a month Multi-AZ against ≈ $3,000 here) trade money for not running it yourself. The deciding features are the partial unique index with `ON CONFLICT … WHERE`, the partial unread index, per-user hash partitions and the same-transaction counter plus outbox. The inbox is the system of record; Redis caches are rebuildable; NATS is at-most-once and recovered through the inbox.

**If it were MySQL instead:**
- `inbox_group_key` becomes a generated column `open_bucket = IF(read_at IS NULL, bucket, NULL)` with a unique index on `(user_id, entity_type, entity_id, event, open_bucket)` (NULLs do not collide, so read rows drop out of the rule), and the grouped insert is `INSERT … ON DUPLICATE KEY UPDATE count = count + 1`.
- `inbox_unread` becomes an ordinary `(user_id, read_at, id)` index over all ~13B live rows, not just the unread ones: the largest single cost.
- `inbox_id_seq` becomes ids minted by the app (task 04), because InnoDB's `AUTO_INCREMENT` must lead an index and the key is `(user_id, id)`.
- Unchanged: outbox plus NATS, the NVMe-and-replicas shape, a `SKIP LOCKED` relay (8.0), run at `READ COMMITTED`. Gained: contiguous replay reads, and retention deletes are purged instead of vacuumed.

**What would flip the choice:** at 10× (200k stored items/s ≈ 600k IOPS, section 15) the inbox outgrows one NVMe primary: shard it by user across ~10 clusters or move it to a TTL store keyed by user (DynamoDB with TTL), keeping prefs, watchers and permissions in Postgres. At a tenth of the load (2k items/s ≈ 6k IOPS) plain RDS Multi-AZ on `gp3` is enough and the NVMe question disappears. If the team will not run self-managed Postgres, Aurora or `io2` is the price of that, at about three times the storage bill.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 500M concurrent (one internet user in ten, online at once), 20M deliveries/s (≈ 6 GB/s ≈ 48 Gbit/s), 17M heartbeats/s (§4's 30 s; §6 says 25 s, ≈ 20 % more), 2M stored items/s at peak, 190 TB of inbox (~270 TB with indexes); 3,000 gateways (500M ÷ 250k × 1.5) | the three things section 15 patched at 10× break again, 10× bigger: if each NATS server still learns all interest, 500M subscriptions × ~500 B = 250 GB per server, 2× the largest `r7g`; one NVMe primary would face 2M items/s ≈ 6M IOPS, 100× its ~60k; an ALB would bill 166,667 LCU (500M ÷ 3,000) ≈ $0.97M a month at peak | same design, sharded by `user_id` hash into ~100 user shards, each today's 5M-connection stack (a NATS cluster with 5M subscriptions ≈ 2.5 GB per server, an inbox cluster of 3 NVMe nodes at ~20k items/s, its own Redis), as in section 15's 10× moves; 3,000 `r7g.large` gateways that subscribe on the shard of the user they hold, behind an NLB with TLS on the gateways instead of the ALB. Give up: an org broadcast becomes one publish per shard and each gateway keeps ~100 bus connections; resharding work | ≈ $1.7M/month (linear: $17.4k × 100), ~60 % of it network (ALB $0.49M, egress $0.54M); the NLB removes most of the ALB line |
| **1,000×** | 5B concurrent (about every internet user online at once: a thought experiment), 200M deliveries/s (60 GB/s ≈ 480 Gbit/s), 170M heartbeats/s, 20M stored items/s, 1.9 PB of inbox; 30,000 gateways | coordination and blast radius: 30,000 gateways lose a node about every 10 min (the cheat sheet says hourly at 5,000), and one lost zone returns 1.67B clients: 5.6M TLS handshakes/s even jittered over 5 min, all into one session cache and one bus. One region also cannot be near everyone (100–150 ms across an ocean) | **cells by org**: ~50 cells of ~100M connections (600 gateways each) in the region nearest their orgs, each with its own NLB, bus shards, inbox shards, router and session cache. The login response names the cell's stream host (no lookup per connect); an org's `org.<id>` broadcast stays inside its cell; a notification for a watcher in another cell crosses on a durable inter-cell log (one ocean hop, 100–150 ms, inside the 1 s p95). A lost zone is 33M clients ÷ 300 s ≈ 111k/s into that cell only. Give up: one global bus and inbox, instant cross-cell delivery (hundreds of ms, at-least-once), handshake latency far from the org's cell | ≈ $17M/month (linear), ~60 % network |
| **10,000×** | 50B concurrent (more than six per person alive: a thought experiment), 2B deliveries/s (600 GB/s ≈ 4.8 Tbit/s), 1.7B heartbeats/s, 200M stored items/s (beyond the biggest pipelines' tens of millions a second), 19 PB of inbox; ~300,000 gateways | money and the socket itself: ≈ $174M a month, a gateway dies about every minute, and the keep-alives alone are ≈ 83 GB/s (25B average sockets ÷ 30 s × 100 B), ≈ $19M a month of egress | change the problem: a socket per user stops being the product. Only the focused tab or app holds a stream (background users get Web Push and a poll on focus; assume ≥ 3× fewer sockets); coalesce to at most one `tick` per user per second ("something changed", unread from a cached endpoint), so deliveries are bounded by users × 1/s, not events × recipients; stretch the heartbeat to the LB's idle limit (each +10 s saves ≈ $4.8M a month); gateways in many edge POPs, the router publishing once per POP that holds an interested user. Give up: sub-second toasts for background users, one frame per event, exact presence | ≈ $174M/month if nothing changes: the bill is the break |

**What stays the same at every scale:** the stream is a doorbell (kinds and ids, never data) and the inbox is the truth, with `Last-Event-ID` replay and `resync` as the escape hatch; the inbox row and the outbox row commit in one transaction ("announced only if committed"); ~250k sockets per node as the blast-radius cap; jittered reconnects.

**Say it in an interview:** "At 100× the things I patched at 10× break again: every NATS server would hold 250 GB of subscriptions and the inbox would need 6M IOPS, so I shard everything by user hash into ~100 copies of today's stack and put the gateways behind an NLB. At 1,000× it is every internet user online at once; 30,000 gateways and a lost zone's 1.67B reconnects are the problem, so I split into ~50 cells by org, each with its own bus and inbox. At 10,000× there are more sockets than people, so I stop giving everyone a socket: only the focused tab streams, ticks are coalesced to one a second, and fan-out happens at the edge."
