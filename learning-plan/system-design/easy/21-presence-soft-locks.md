# 21 · Presence & Soft Edit Locks

`🟢 easy` · *Topics:* who-is-here and who-is-editing, heartbeats vs connection liveness, TTL state in Redis, advisory locks vs optimistic versions, takeover and idle expiry, what counts as a version bump · *Builds on:* [05 In-Memory Cache with TTL](05-kv-cache-ttl.md) (TTL state, lazy expiry), [13 Shopping Cart](13-shopping-cart.md) (optimistic `version`)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [20 Per-Tenant Document Numbering](20-document-numbering.md) · Next: [22 Audit Log & Revision History](22-audit-log-revisions.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 18 and migration 0045 show who has a request, order, client or invoice
> open (*here now*), who is editing it, and who manages it (*assigned to*). One `UNLOGGED` table `record_presence`,
> a row per browser tab (`session_key`), a heartbeat every 20 s, a row lapses after 60 s and a worker sweeps it every
> 30 s. A partial unique index `WHERE editing` allows one editor per record; 15 minutes without a keystroke
> (`typed_at`) makes the hold takeable, and the `records.take_over` permission takes it at once. Saves to a held
> record answer `409 record-held`. Assigning an order does **not** bump its version (the `updated_at` trigger has a
> `WHEN` clause), so open panels are not told "someone else changed this". Built for a handful of staff at a time.
> **Here, at scale:** a SaaS admin app with 2M concurrent sessions on 50M records: the heartbeats alone are ~100k/s,
> and one popular record can have thousands of viewers.

## 1. The task

> "In our admin app, show who else has this record open and who is editing it. Don't let two people overwrite each
> other's work, and let a manager take over when someone walks away."

Looks like UI polish. It is really a small distributed-state problem: ephemeral data that must expire by itself,
a lock that must never wedge, and a clear answer to "what is the *actual* guarantee against lost updates".

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Is presence history or just now? | History needs durable storage; "now" does not. | Just now: a restart may forget it. |
| Hard lock or advisory? | A hard lock needs the lock service on the write path. | Advisory UX lock, plus an optimistic version check as the real guarantee. |
| One editor per whole record, or per field? | Per-field is collaborative editing in disguise. | Whole record. |
| Same person in two tabs? | Decides the unit: user or tab. | One entry per tab. |
| How stale may presence be? | Sets TTL and heartbeat. | Gone within ~60 s of a closed laptop; closed tab is immediate. |
| Who may take over, and when? | Needs idle detection and a permission. | Managers at once; anyone who may edit after 15 min without a keystroke. |
| Lists too ("faces on rows")? | Turns single lookups into batch lookups. | Yes, 25 rows at a time. |
| Scale? | Heartbeats scale with sessions, not with edits. | 2M concurrent sessions, 50M records, ~10% editing. |

## 3. Requirements

**Functional**
- Show the people who have a record open, live, with who is editing and since when.
- At most one editor per record at a time; others see *"Mia is editing since 2:14"* and their Edit waits.
- Take over: a privileged user at once; anyone with edit rights after 15 minutes idle. The displaced editor keeps
  their typed text but cannot save.
- Faces on list rows for the 25 visible records.
- Presence disappears when a tab closes, a laptop sleeps or a node crashes.

**Non-functional**
- Presence is eventually consistent within ~2 s for joins and leaves.
- Presence being down must not stop anyone from saving (fail open).
- Never lose an update silently, whether presence works or not.

**Out of scope**
- Simultaneous editing with merge (hard backlog: collaborative editing), cursors, chat.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Sessions (tabs with a record open) | 2M | given |
| Editing sessions | ~200k | 10% |
| Heartbeats | 100k/s | 2M / 20 s |
| Heartbeat bytes over HTTP | ~50 MB/s | 100k × ~500 B with headers; a WebSocket ping is a few bytes |
| Joins + leaves | ~6.7k/s | 2 × 2M / 600 s (10-minute sessions) |
| Edit starts + ends | ~2.2k/s | 2 × 200k / 180 s (3-minute edits) |
| **State changes total** | **~9k/s** | 6.7k + 2.2k |
| Pushes to other viewers | ~9k/s | each change tells ~1 other person (about 2 viewers per active record) |
| Redis memory | ~0.6 GB | 2M × ~200 B entries + ~1.5M record hashes × ~100 B + 200k locks × ~150 B |
| Gateway nodes | 40 | 2M connections / 50k per node |

What the numbers tell us: **heartbeats are 10× the real state changes** (100k/s vs 9k/s), and they carry no
information. Everything that follows is about keeping heartbeats out of anything durable or shared. Memory is
trivial; the seed's design (a Postgres row updated every 20 s per tab) is 100k updates/s on one table, which
bloats and burns a node even when `UNLOGGED`.

## 5. Plan

1. Where presence lives (it is ephemeral, so not the main database).
2. How liveness is detected cheaply, and how changes fan out without storms.
3. The lock: what it guarantees and what it does not.
4. Takeover and expiry rules.
5. The version rule that makes "someone else changed this" honest.

## 6. High-level design

```
 browser tab ──WebSocket──► Gateway nodes (40) ──┬─► Redis cluster
   join / edit / typed / leave   │               │     pres:{type}:{id}   HASH  session → {user, editing, since, typed, exp}
   ping every 20 s (in memory)   │               │     lock:{type}:{id}   STRING {session, user, since, typed}  PX 60 s
                                 │               │     pub/sub: presence:{type}:{id}  (a node subscribes only while it has a viewer)
        push {here, editor} ◄────┘               │
                                                 │
 list screen ──GET /v1/presence?ids=…(≤100)──► API ──► pipelined HGETALL
 save        ──PUT /v1/orders/{id}  If-Match: v17 + edit token──► API ──► Postgres  (UPDATE … WHERE version = 17)
```

*Open a record:* the tab sends `join`; the gateway writes the session into the record's hash (`HSET`) and
publishes a change; other gateways holding viewers of that record push the new face list to their sockets.
*Start editing:* `SET lock NX PX 60000` (one atomic command): success means you are the editor, failure
returns the holder. *Save:* the API checks the caller holds the lock (best effort) and the optimistic version (always).
*Close tab:* the socket closes, the gateway deletes the session and releases the lock at once.

## 7. API

```
WS   /v1/live
  → {"t":"join","e":"order","id":"o_7Kq2"}      ← {"t":"presence","e":"order","id":"o_7Kq2","here":[{"u":"u_9","editing":true,"since":"…"}],"you":false}
  → {"t":"edit"}      ← {"t":"lock","ok":true,"token":"…"} | {"t":"lock","ok":false,"by":"u_9","since":"…"}
  → {"t":"typed"}     (coalesced into the next ping; resets the idle clock)
  → {"t":"leave"}
  ← {"t":"lock.lost","by":"u_3","reason":"taken-over"}
GET  /v1/presence?entity=order&ids=o_a,o_b,…        → 200 { "o_a": [{user, editing}], … }     (≤ 100 ids, one pipeline)
POST /v1/presence/{entity}/{id}/take-over           → 200 | 403 | 409 "not idle long enough"
PUT  /v1/orders/{id}   If-Match: "17"  X-Edit-Token: …
                                                     → 200 | 412 version changed | 409 { "type": "record-held", "by": …, "since": … }
```

## 8. Data model

The durable part is only the record's version and its assignee. Presence is Redis (below) and may be lost.

```sql
CREATE TABLE orders (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   bigint NOT NULL,
    public_id   text   NOT NULL UNIQUE,
    version     bigint NOT NULL DEFAULT 1,        -- bumped only by edits of the content
    assigned_to bigint,                           -- responsibility, not presence; changing it does not bump version
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Save: compare-and-set on the version the client loaded.
-- UPDATE orders SET …, version = version + 1, updated_at = now() WHERE id = $1 AND version = $2;   -- 0 rows → 412
```

```
pres:{entity}:{id}   HASH    field = session_key   value = {user, editing, since, typed, exp}     key TTL 120 s
lock:{entity}:{id}   STRING  {session, user, since, typed}                                        PX 60000
```

The seed's small-scale variant is the same idea in Postgres, good up to a few thousand tabs:

```sql
CREATE UNLOGGED TABLE record_presence (
    entity_type text NOT NULL, entity_id bigint NOT NULL, session_key text NOT NULL,
    user_id bigint NOT NULL, editing boolean NOT NULL DEFAULT false,
    since timestamptz NOT NULL DEFAULT now(), typed_at timestamptz NOT NULL DEFAULT now(),
    seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_type, entity_id, session_key)
);
CREATE UNIQUE INDEX record_presence_editor ON record_presence (entity_type, entity_id) WHERE editing;  -- the lock
```

## 9. Deep dives

### 9.1 Where presence lives

| Option | Pros | Cons |
|---|---|---|
| **Postgres `UNLOGGED` table** (the seed) | no new service; the unique partial index is a real lock; same transaction as the record | 100k heartbeat updates/s: dead tuples, vacuum, index churn; fine below ~5k tabs |
| **Redis, TTL on hashes and locks** | built for ephemeral state; `SET NX PX` is an atomic lease; pipelined batches; 0.6 GB | a second system; data lost on restart (acceptable); Lua for compare-and-extend |
| **In memory on each gateway + pub/sub** | fastest, no shared store | a record's viewers are on different nodes, so every query fans out to all 40; a node crash loses its share silently |
| A CRDT/awareness protocol (Yjs-style) | collaborative cursors for free | only worth it if you also do collaborative editing |

**Choice and why.** **Redis.** Presence is ephemeral by nature, and the property we need (state that disappears
by itself if nobody renews it) is exactly a TTL. Postgres stays the source of truth for records and versions. At
the seed's size, the `UNLOGGED` table is the better answer: one fewer moving part.

### 9.2 Heartbeat cost and fan-out

100k heartbeats/s over HTTP `PUT` is 50 MB/s of headers and ~100k requests/s through the load balancer to
carry no new information. Three levers:

| Lever | Effect |
|---|---|
| **A persistent connection is the heartbeat** | the WebSocket ping/pong is handled in the gateway's memory; a closed socket means an immediate leave; no HTTP, no auth, no DB per beat |
| **Gateway refreshes Redis in batches** | every 20 s per node, one pipeline of `HSET`/`PEXPIRE` for its sessions: 100k ops/s spread over a small cluster (Redis does ~100k simple ops/s per core) |
| **Piggyback `typed`** | keystrokes are noted in memory and sent with the next ping, never as a request per keystroke |

An even cheaper version is a *node lease*: sessions carry their gateway id, and one key `node:{id}` (TTL 10 s)
proves the node is alive (TTL 10 s, renewed every 3 s); if it expires, all its sessions are treated as gone. That
turns 100k refreshes/s into ~13 (40 nodes / 3 s). It costs a liveness check on every read (a 40-entry in-memory set), so start with batched refresh.

Fan-out: a change is published only on `presence:{entity}:{id}`, and a gateway subscribes to a channel only
while it has a viewer of that record. About 9k pushes/s overall is nothing. The danger is a **hot record**
(thousands of viewers): every join tells everyone, so n viewers produce n² messages over time. Coalesce (at most one push per record per second), and above ~50 viewers
send counts plus the first five faces ("+4,995 more").

### 9.3 Soft lock, optimistic version, or real collaboration

| Approach | Guarantee | Cost |
|---|---|---|
| **Optimistic version only** | no lost update: the second save gets `412` | the second editor discovers the conflict after typing; their work is at risk |
| **Advisory lock only** | prevents most conflicts up front | not safe: lock expires, Redis is down, a client ignores it |
| **Lock + version (our choice)** | the lock saves people's time; the version is the guarantee | two mechanisms to explain |
| Hard lock on the write path | strict | availability depends on the lock store; wedged locks block writes |
| Operational transform / CRDT | simultaneous editing | a different problem (hard backlog: collaborative editing) |

**Choice and why.** The lock is a **UX feature**, not a correctness mechanism. The compare-and-set on `version`
is the guarantee, so the system is correct even when presence is wrong. That decides the failure mode: if Redis
is unreachable the app **fails open**: no faces, no hold, and a conflicting save returns `412` with the other
version's author. The seed enforces the hold server-side (`409 record-held`); keep that check, but make it
best-effort, skipped when the lock store is unreachable.

### 9.4 The lock, takeover and stale expiry

A lock is a small lease: `{session, user, since, typed}` with `PX 60000`, renewed with each ping *only if the
caller still owns it*. That needs compare-and-extend, so it is a Lua script:

```lua
-- KEYS[1]=lock:{e}:{id}  ARGV: session, user, now_ms, ttl_ms, typed(0|1)
local cur = redis.call('GET', KEYS[1])
if not cur then
  redis.call('SET', KEYS[1], cjson.encode({s=ARGV[1], u=ARGV[2], since=tonumber(ARGV[3]), typed=tonumber(ARGV[3])}), 'PX', ARGV[4])
  return 1                                   -- acquired
end
local h = cjson.decode(cur)
if h.s ~= ARGV[1] then return cur end        -- someone else holds it: return the holder
if ARGV[5] == '1' then h.typed = tonumber(ARGV[3]) end
redis.call('SET', KEYS[1], cjson.encode(h), 'PX', ARGV[4])
return 1                                     -- renewed
```

Two clocks run: the **lease** (60 s without a ping and the lock vanishes, which covers closed laptops and crashed nodes)
and the **idle clock** (`typed`: a tab can ping forever while its owner is at lunch). Takeover rules:

| Caller | Allowed when |
|---|---|
| holder's own other tab | never silently; ask the user |
| anyone with edit rights | `now - typed ≥ 15 min` |
| users with `records.take_over` | at once |

Takeover is a second script with the same shape: check the rule, overwrite the key, publish `lock.lost` to the
old holder. The displaced client keeps its form with the typed text, disables Save, and says who took over and
when, so nothing typed is lost without the person seeing it. Use the **version** as the fencing token: the
old editor's late save still fails on `If-Match`, whatever the lock says. Always use the server's clock for `typed`
and `since`, never the browser's.

### 9.5 What counts as a change

The seed found a subtle point: assigning an order must **not** change its version, or every panel open on that
order is told "someone else changed this". Generalise to a rule: the version covers *content a person edits*.
It does not cover assignee, view or like counters, derived or denormalised fields, search-index timestamps, or a
background job touching the row.

Two implementations: an explicit `version` the application increments in the edit path (a counter, as in §8),
or a trigger whose `WHEN (OLD.assigned_to IS NOT DISTINCT FROM NEW.assigned_to)` skips the bump, as the seed does.
Prefer the **integer version**: the seed compares `updated_at`, and a `timestamptz` has microseconds while
JavaScript dates have milliseconds. A round trip through a browser can silently truncate it and make every save
a false conflict. If you must use a timestamp, send it as an opaque string (an ETag), never as a parsed date.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the refresh traffic (1M ops/s) and hot records. Move to the node lease, shard
  Redis by `{entity}:{id}` hash tag so a record's hash, lock and channel land on one shard, and cap fan-out.
- **Redis down:** presence and holds vanish; faces disappear; saves still work and conflicts return `412`.
  Show a quiet "live status unavailable" banner, not an error.
- **Gateway node crash:** its sessions linger for up to the TTL (60 s) then disappear; a lock it held expires on its
  own, so nobody is blocked for longer than a minute.
- **Network partition of one browser:** the socket drops; the gateway removes the session; the tab reconnects and
  re-joins, and the client re-requests the lock instead of assuming it still has it.
- **Thundering herd after a gateway deploy:** 50k sockets reconnect together. Jittered reconnect (0–10 s) and
  batched joins.
- **Clock skew:** only server clocks are used; `typed` and `since` come from the node, and idle is compared
  inside the Redis script with the passed `now`.
- **Abuse:** a client that joins thousands of records floods the hashes. Cap joins per socket (say 5) and per
  user; rate limit with [03](03-rate-limiter.md).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Redis with TTLs | Postgres `UNLOGGED` rows | no 100k/s write load on the database | a second system; presence lost on restart |
| Connection as heartbeat | HTTP `PUT` every 20 s | 10× less traffic, instant leave | stateful gateways, reconnect handling |
| Advisory lock + version check | hard locks | writes never depend on the lock service | a late conflict is possible (`412`) |
| Fail open | fail closed | availability | presence can lie during an outage |
| Whole-record lock | per-field locks | simple rules | two people cannot edit different fields |
| Idle takeover after 15 min + permission | locks until released | no wedged locks | a quiet writer can be displaced |
| Integer `version` | `updated_at` as version | no precision bugs, explicit bump rules | one more column, discipline in the edit path |

## 12. Check yourself

1. 2M sessions each heartbeat every 20 s and ~9k presence changes happen per second. What do these numbers tell you about where to spend effort?
2. What is the real protection against two people overwriting each other, and what happens when Redis is down?
3. Walk through a takeover: who may do it, when, and what does the displaced editor see?
4. Why must assigning an order not bump its version?
5. Why is `updated_at` risky as the version token sent through a browser?

<details><summary>Answers</summary>

1. Heartbeats are 100k/s against ~9k state changes/s, so liveness traffic dominates and carries no
   information. Make liveness cheap: use the open socket as the heartbeat, batch Redis refreshes (or use a node lease), and keep
   heartbeats out of the main database. The real state changes are easy.
2. The optimistic version check (`UPDATE … WHERE version = $2`, `412` on mismatch) is the guarantee; the lock only
   prevents wasted work. If Redis is down, the app fails open: no faces or holds, and a conflicting save is still
   refused with `412`.
3. Users with `records.take_over` can take over immediately; anyone with edit rights can once the holder has had no
   keystroke for 15 minutes (`typed`, server clock). The Lua script checks the rule, overwrites the lock and publishes
   `lock.lost`. The old editor's panel keeps the typed text with Save disabled and a notice of who took over; a late
   save would fail the version check anyway.
4. The version means "the content changed under you". An assignment changes responsibility, not content;
   bumping it would show "someone else changed this" to everyone with the record open and invite them to
   discard their own work needlessly.
5. A `timestamptz` has microsecond precision; browsers parse it to a JavaScript `Date` with millisecond precision,
   so sending it back can differ from the stored value and every save looks like a conflict. Use an integer
   version or pass the timestamp as an opaque string.

</details>

## 13. In Go

- Related lessons: [58 Realtime: WebSockets & SSE](../../58-realtime-websockets-sse.md) (the gateway, ping/pong, backpressure), [38 Caching patterns](../../38-caching-patterns.md) (TTL state, Redis keys), [15 Sync & context](../../15-sync-context.md) (per-connection goroutines, graceful shutdown), [67 Multi-user state](../../67-multi-user-state.md) (shared state between users), [59 Production DB patterns](../../59-production-database-patterns.md) (optimistic versions, `412`/`409` mapping).
- Packages: `github.com/coder/websocket` (or `gorilla/websocket`), `redis/go-redis/v9` (`Pipeline`, `Eval` / `Script`, `Subscribe`), `pgx`.
- Shape: one goroutine reads the socket, one writes from a buffered channel (drop or close a slow consumer), a `map[recordKey][]*conn` guarded by a mutex for local fan-out, and a ticker that batch-refreshes sessions into Redis.

## 14. Extensions

- **Node lease** instead of per-session TTL; measure the ops/s saved.
- **"Notify me when it is free":** a waiter queue per lock, pushed through [task 45](45-live-notifications.md).
- **Per-field or per-section locks** and cursors, then real collaborative editing (hard backlog: Google Docs-style).
- **Draft autosave** for the displaced editor, so a takeover never costs typed work.
- **Cross-region:** where does the lock live for a team spread over two continents?
- **Presence in lists at scale:** a cheap per-record "someone is here" count, kept by the gateways, for 1,000-row virtual lists where a batch lookup per scroll is too heavy.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| WebSocket gateways (Go, stateful) | 2M open sockets; heartbeats 100k/s (a ping every 20 s); 9k changes/s and 9k pushes/s | memory (sockets), not CPU | assume ~30 KiB per socket (two goroutines, buffers, TLS state): 50k × 30 KiB = 1.5 GB, × 1.4 GC headroom = 2.1 GB per node; CPU 100k ÷ 42 = 2.4k pings/s × 20 µs = 0.05 vCPU. Count by sockets, not CPU: 2M ÷ 50k = 40 → 42 | `m7g.large` (2 vCPU / 8 GiB) behind an NLB | 42, 14 per zone | a lost zone leaves 28 nodes at ≈ 71k sockets ≈ 3.0 GiB of 8, which fits; `c7g.large` (4 GiB) would be at 75 %; CPU is idle, so no bigger family. A deploy's reconnect herd (50k sockets over 10 s ≈ 5k handshakes/s ≈ 3.5 vCPU) spreads over the other 41 nodes |
| Redis (presence hashes, locks, pub/sub) | ≈ 100k refresh ops/s + 9k changes × 3 ops (`HSET`, `PUBLISH`, lock script) = 27k ⇒ 127k writes/s; ≈ 50k reads/s (assumed 2k list screens/s × 25 `HGETALL`, not in section 4); memory 0.6 GB | single-core CPU (ops/s); memory is tiny | one core ≈ 100k simple ops/s, plan ≤ 60 %: 127k ÷ 60k = 2.1 ⇒ 3 shards; each primary takes 42k ops/s, each replica applies 42k + serves 17k reads = 59k; memory 0.6 GB × 1.7 ≈ 1 GB in all, ≈ 0.35 GB per shard | `cache.m7g.large` (2 vCPU), ElastiCache cluster mode | 3 shards × (primary + replica) = 6 | not an `r` node: ~0.4 GB per shard wastes 8 GiB per vCPU, and ops/s is the bound; shard by `{entity}:{id}` so a record's hash, lock and channel share a node. The node lease (9.2) cuts refresh ops from 100k to ~13/s and the cluster to one shard |
| API (Go, stateless: saves, take-over, list lookups) | ≈ 1.1k saves/s (one per edit start, 2.2k ÷ 2) + ≈ 2k list lookups/s (assumed) | CPU | 1.1k × 1 ms + 2k × 0.3 ms = 1.7 vCPU. N = ceil(1.7 ÷ (0.8 × 4 × ⅔ = 2.13)) = 1 → 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | availability more than load; each lookup is one Redis pipeline, so the wait is Redis, not CPU |
| Postgres (orders and other records; the `version` compare-and-set) | this lesson adds ≈ 1.1k committed writes/s; 50M records × ~1 KB (assumed) ≈ 50 GB + 30 % indexes ≈ 65 GB | memory | hot set ≈ 10 % ≈ 7 GB, fits in 32 GiB; IOPS ≈ 1.1k × 2 = 2.2k, under gp3's 3,000 included; CPU ≈ 1.1k × 0.5 ms = 0.55 of 4 vCPU | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ | 1 primary + synchronous standby | the app's existing records DB: presence adds only the CAS writes, and the 100k heartbeats/s never reach it, which is the point of Redis; the app's own reads are another lesson's load |

Network: a pong is ≈ 100 B on the wire (frame + TLS + TCP/IP headers, assumed), so 100k/s ≈ 10 MB/s out, plus 9k pushes/s × ~400 B = 3.6 MB/s: ≈ 13.6 MB/s ≈ 36 TB a month to the internet. Per node that is 0.3 MB/s, so bandwidth is no bound, only a bill line. An ALB would bill 2M open sockets as ≈ 670 LCU (3,000 active connections per LCU, assumed) ≈ $3,900 a month, so the front is an NLB (a price not in the sheet; ≈ $100 assumed).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Gateways: 42 × `m7g.large` (42 × $60) | $2,520 |
| NLB (price not in the sheet; assumption) | $100 |
| API: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Redis: 6 × `cache.m7g.large` (6 × $120) | $720 |
| Postgres: `db.r7g.xlarge` ≈ $310, Multi-AZ doubles; 100 GB gp3, ×2 ($16) | $640 |
| Data out: 36 TB × $0.09 per GB | $3,200 |
| **Total** | **≈ $7,500** (≈ $6,000 with 1-year commitments on the $4,180 of compute) |

- **Biggest line and why:** data out (≈ 43 %), then the gateway fleet (≈ 34 %): the bill is bytes and sockets, not CPU. The keepalive pongs carry no information but are ≈ 26 TB ≈ $2,400 of the egress; a 30 s ping (90 s lease) cuts that by a third. The node lease does not, because the socket pings remain.
- **What changes at 10×:** 20M sockets ⇒ 400 → 402 × `m7g.large` (≈ $24k), and memory per socket is the lever (30 → 15 KiB halves the fleet); Redis refresh at 1M ops/s needs ≈ 21 shards ((1M + 270k) ÷ 60k) without the node lease and ≈ 5 with it (270k ÷ 60k = 4.5); egress ≈ 360 TB ≈ $32k; Postgres CAS at 11k/s ≈ 22k IOPS passes gp3's 16k cap (`io2`, or shard by tenant).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Ephemeral: 2M open sessions, 100k heartbeats/s against ≈ 9k real state changes/s; the whole presence state is ≈ 0.6 GB and may be forgotten on a restart (sections 2, 4).
- The operations are leases and TTLs, not queries: `SET NX PX` to take a lock, a compare-and-extend script on every ping, a hash per record with a 120 s TTL, a pub/sub channel per record (9.2, 9.4).
- The durable part is one integer: `orders.version`, a compare-and-set at ≈ 1.1k committed writes/s on 50M records ≈ 65 GB (section 15). It is the real guarantee; presence is allowed to be wrong while Redis is down (9.3).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen as the system of record, ✖ as the presence store | `UPDATE orders SET …, version = version + 1 WHERE id = $1 AND version = $2` is the guarantee, and the records already live here. As presence it works to ≈ 5k tabs (an `UNLOGGED` table plus the partial unique index `WHERE editing`, 9.1), but 100k heartbeat updates/s means dead tuples, vacuum and a sweeper, and there is no row TTL |
| MySQL (RDS / Aurora) | ✔ same for `version`, ✖ as the presence store | the compare-and-set is plain SQL (check rows affected). As presence it is no better than Postgres: no `UNLOGGED` table (every heartbeat is a logged InnoDB write), no partial unique index (a generated `editor` column that is `NULL` unless `editing`, under a unique index, gives the same one-editor rule), and the `MEMORY` engine locks whole tables and empties on restart |
| Redis (ElastiCache, cluster mode) | ✅ chosen for presence, locks and fan-out | a TTL makes state vanish if nobody renews it; `SET NX PX` is an atomic lease; Lua does compare-and-extend; pub/sub carries the pushes; 127k writes/s over 3 shards, 0.6 GB |
| MemoryDB | ✔ works, not needed | a durable Redis (Multi-AZ log) would let a lock survive failover, but presence is explicitly allowed to be lost and the version check covers a lost lock, so the extra cost buys nothing here |
| DynamoDB | ✔ for the lock alone, ✖ for heartbeats and fan-out | a conditional put (`attribute_not_exists(pk) OR expires_at < :now`) is a lease, and its TTL removes items lazily (typically within days), so every read must check `expires_at`; 100k heartbeats/s would be 100k billed writes/s, and there is no pub/sub for the pushes |

**Pick:** Redis on ElastiCache (`cache.m7g.large`, 3 shards × primary + replica, section 15) for presence, locks and pub/sub, sharded by the `{entity}:{id}` hash tag so a record's hash, lock and channel share a node; PostgreSQL on RDS Multi-AZ (`db.r7g.xlarge`) for the records and their integer `version`. What decides it is the split of jobs: TTL + atomic `SET NX PX` + Lua + pub/sub for state that must disappear by itself, a durable transactional compare-and-set for the one thing that must never be wrong. Postgres is the system of record; Redis is derived and disposable (a restart empties the faces and loses nothing that matters).

**If it were MySQL instead:** nothing material for the records database. The `version` column and its compare-and-set port as they are, and the integer version is the better choice anyway (9.5). One difference: the seed's trigger form `WHEN (OLD.assigned_to IS NOT DISTINCT FROM NEW.assigned_to)` has no equivalent, since MySQL triggers have no `WHEN` clause (put a `<=>` test in an `IF` inside the trigger body), another reason to bump `version` in the application's edit path. The small-scale `UNLOGGED` presence table would need the generated-column key above and would likely stop being "one fewer moving part" sooner, because each heartbeat is a logged write.

**What would flip the choice:** the refresh traffic: at 10× (1M ops/s) the cluster needs ≈ 21 shards, or ≈ 5 with the node lease (section 15), which is a Redis tuning step, not a new store. Below ≈ 5k concurrent tabs the Postgres `UNLOGGED` table is the better answer (9.1). If the lock ever had to be a hard guarantee on the write path, which 9.3 rejects, move it to MemoryDB or a conditional-write store with a fencing token. Postgres compare-and-set at 11k/s passes gp3's 16k IOPS cap and then shards by tenant (section 15).

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 200M concurrent sessions (20M editing); heartbeats 10M/s; 900k state changes/s and pushes/s; 5B records (≈ 6.5 TB); Redis 60 GB; 110k version-checked saves/s; gateways 200M ÷ 50k = 4,000 | the refresh traffic, as at 10× in section 10: 10M ops/s is 100 Redis cores flat out, ≈ 212 shards at 60 % ((10M + 2.7M) ÷ 60k) without the node lease. Next the records database: 110k compare-and-sets/s is 11× a primary's ~10k | the node lease of sections 9.2 and 16: sessions carry their gateway id and one key per node (renewed every 3 s) replaces per-session refreshes, 10M/s → ~1.3k/s (4,002 nodes ÷ 3 s), so Redis carries only the changes: 2.7M ÷ 60k = 45 shards by the `{entity}:{id}` hash tag; 4,002 `m7g.large` gateways (memory per socket, 30 → 15 KiB, halves that); records sharded by `tenant_id` into ~32 (≈ 3.4k saves/s each); above 50 viewers a record sends counts plus five faces. Give up: a crashed node's faces linger until its lease lapses (~10 s), and no transaction across tenants | ≈ $750k/month; egress ≈ $320k and gateways ≈ $240k are ~75 % of it |
| **1,000×** | 2B concurrent sessions (a quarter of everyone alive has a record open at this instant: a thought experiment); heartbeats 100M/s; 9M changes/s; ~40,000 gateways; Redis 600 GB and 27M writes/s ⇒ 450 shards; 1.1M saves/s | the fleet: 40,000 stateful gateways lose a node about every 7 minutes (5,000 nodes ≈ hourly), each loss dropping 50k sockets that reconnect at once, so the reconnect herd never stops; and 450 Redis shards under one cluster map is past what one cluster coordinates comfortably (assumed) | independent **cells** by `tenant_id`: ~100 cells, each the 10× design (20M sockets, ~402 gateways, ~5 Redis shards with the lease, its own Postgres shards and gateway hostname), and the login response names the tab's cell. Presence is tenant-scoped, so nothing crosses a cell; a deploy or a failure touches 1 % of tenants. Give up: presence across tenants (a person in two tenants holds two connections) and a global "who is online" | ≈ $7.5M/month (egress $3.2M, gateways $2.4M) |
| **10,000×** | 20B concurrent sessions (2.5 per person alive: a thought experiment); heartbeats 1B/s; 90M changes/s; 400,000 gateways; ≈ 600 TB of socket memory | money and physics: ≈ $75M a month; pings alone are 100 GB/s ≈ 260 PB a month (≈ $23M at first-tier price) carrying no information, and 600 TB of RAM holds sockets that mostly just watch | change the problem: passive viewers hold no socket: a cached "who is here" poll every ~60 s (jittered), only for records with an editor or two or more openers; counts above 50 from a sketch; the edit lock becomes `locked_by` and `lock_until` on the record row, renewed by the autosave the editor makes anyway, so there is no lock service, no per-editor ping, and `lock.lost` is learned on the next save (≤ 30 s). The version check stays. Give up: live faces for passive viewers (up to a minute stale), exact counts and instant takeover notice | ≈ $75M/month if nothing changes; far less once viewers hold no socket |

**What stays the same at every scale:** the optimistic `version` compare-and-set is the real guarantee and the lock is only UX, so the system is correct whenever presence is wrong; presence is ephemeral, derived, and fails open.

**Say it in an interview:** "At 100× the refresh traffic is 10M ops a second, so I use the node lease and shard Redis by record, and shard the records database by tenant. At 1,000× the problem is running 40,000 stateful gateways that fall over every few minutes, so I cut the world into ~100 tenant cells, each today's 10× design behind a login-time router. At 10,000× there are 2.5 sessions per person alive, so passive viewers stop holding sockets and the edit lock moves into the record row, renewed by autosave; the version check stays the guarantee."
