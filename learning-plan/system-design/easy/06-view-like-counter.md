# 06 · View & Like Counter

`🟢 easy` · *Topics:* write-heavy hot rows, write-behind, local batching, sharded counters, idempotent per-user actions, approximate counting (HyperLogLog) · *Builds on:* [05 In-Memory Cache](05-kv-cache-ttl.md) (`INCR`), [01 URL Shortener](01-url-shortener.md) (left click counting as "async")

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [05 In-Memory Cache with TTL](05-kv-cache-ttl.md) · Next: [07 Leaderboard](07-leaderboard.md)

---

## 1. The task

> "Design the view counter for videos and the like button for posts. Every view increments the
> count; the count is shown on every page. Some videos are extremely popular."

Two counters that look the same and are not: views are approximate and anonymous; likes are
exact and one per user. The task is to notice that and design each accordingly.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Must view counts be exact? | Exact counting at high rates is expensive; "about right" is cheap. | Approximate is fine; within ~1% and a few seconds stale. |
| Must like counts be exact? One like per user? Un-like? | Exactness plus dedup means a row per (user, post). | Exact. One per user. Un-like allowed. |
| Peak rate on one item? | The hot-row problem appears only on hot items. | A viral video: 10k views/s. Likes peak ~1k/s on one post. |
| Total rate? | Sizing. | 100k views/s and 5k likes/s system-wide. |
| How fresh must the displayed count be? | Allows batching. | Within a few seconds. |
| Unique viewers too, or raw views? | Unique needs set semantics. | Raw views now; unique viewers as a nice-to-have. |
| Anti-abuse? | Bots inflating counts. | Basic rate limiting per client (task 03); fraud detection out of scope. |
| Where is the count used? | If used for sorting ("most viewed"), it must live in the database too. | Shown on pages and used for sorting. |

## 3. Requirements

**Functional**
- Increment a video's view count on each view; show the count.
- Like / un-like a post exactly once per user; show the exact like count.
- Counts are queryable in the database for sorting.

**Non-functional**
- 100k increments/s system-wide; 10k/s on a single hot key without degrading.
- Displayed counts lag by at most a few seconds; likes are exact, views ~1%.
- A crash may lose at most a few seconds of view increments; must lose no likes.

**Out of scope**
- Per-viewer history, trending algorithms, fraud detection.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| View increments | 100k/s; one item at 10k/s | given |
| Naive database updates | 100k row updates/s | one `UPDATE … +1` per view: far beyond one Postgres |
| After batching per server (100 servers, flush every 1 s) | ≤ 100 updates/s **per distinct item** viewed in that second; total ~ (distinct items/s) | a hot item becomes 100 updates/s instead of 10k |
| Redis ops | 100k `INCR`/s | one node handles it; shard if needed |
| Like rows | 5k inserts/s; 1B rows/yr at ~50 B | fine for a partitioned table |
| Unique viewers with HyperLogLog | 12 KB per video regardless of audience | vs a set of 8 B user IDs: 80 MB for 10M viewers |

What the numbers tell us: counting is a **write** problem, the opposite of tasks 01 and 02.
The database cannot absorb one row update per event, and the hot item makes it worse: 10k
updates/s on *one row* serialise on that row's lock. The whole design is "aggregate before you
write".

## 5. Plan

1. Show why the naive design fails: contention on one row, not raw throughput.
2. Views: aggregate in memory, write deltas, accept bounded loss.
3. Likes: exactness through a row per (user, post) and an idempotent write.
4. Reads: where the displayed number comes from.
5. Hot keys inside Redis itself, and unique counting.

## 6. High-level design

```
 view event ──► API server ──► in-process aggregator: map[videoID]delta  (mutex, lesson 15)
                                       │ every 1 s (or 1,000 events)
                                       ▼
                               Redis: INCRBY views:{id} delta        ← serves reads
                                       │ every 10 s: flusher
                                       ▼
                               Postgres: UPDATE videos SET views = views + $delta  (batched, upsert)

 like ──► API server ──► Postgres TX:
                           INSERT INTO likes(user_id, post_id) ON CONFLICT DO NOTHING
                           if inserted: UPDATE posts SET likes = likes + 1          ← exact
                        ──► Redis: cache posts:{id}:likes (refreshed or incremented on success)

 reads ──► Redis (views, likes) ──miss──► Postgres
```

Views: each server sums events locally for a second, pushes one `INCRBY` per touched item to
Redis, and a flusher periodically moves Redis deltas into the database. Likes: one database
transaction that inserts the per-user row and bumps the counter only if the insert happened,
so retries and double-clicks cannot double-count.

## 7. API

```
POST /videos/{id}/view                  → 204    (fire-and-forget from the player; rate-limited)
GET  /videos/{id}                       → 200 { …, "views": 1203400 }   (approximate, cached)

PUT    /posts/{id}/like                 → 204    idempotent: liking twice is one like
DELETE /posts/{id}/like                 → 204    idempotent: un-liking twice is fine
GET    /posts/{id}                      → 200 { …, "likes": 8812, "liked_by_me": true }
```

`PUT`/`DELETE` rather than `POST /like`: the verb says the operation is idempotent, which is
exactly the property we need for retries.

## 8. Data model

```sql
-- likes: the source of truth for exact counts and per-user state
CREATE TABLE likes (
    user_id    BIGINT NOT NULL,
    post_id    BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)              -- post first: "who liked this post" is the hot query
);

-- denormalised counters for display and sorting
ALTER TABLE posts  ADD COLUMN likes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN views BIGINT NOT NULL DEFAULT 0;
CREATE INDEX videos_views_idx ON videos (views DESC);  -- "most viewed"
```

Redis:

```
views:{videoID}        → integer (INCRBY)
views:{videoID}:hll    → HyperLogLog of viewer IDs (PFADD / PFCOUNT), optional
posts:{postID}:likes   → integer, cache of the DB column
```

## 9. Deep dives

### 9.1 Why `UPDATE … SET views = views + 1` dies

Postgres can do thousands of updates per second *on different rows*. On the **same row** each
update takes the row lock, writes a new row version (MVCC), and the next update waits. 10k/s on
one row means a queue on one lock, dead tuples piling up faster than vacuum can clean them, and
WAL traffic for every single view. Throughput is not the problem; **contention and churn on one
hot row** are. Every solution below reduces the number of writes that reach that row.

### 9.2 Counting views

| Approach | How | Pros | Cons |
|---|---|---|---|
| **Redis `INCR` per event, periodic flush to DB** (write-behind) | server-side atomic increments; a job writes deltas to Postgres | Redis takes 100k/s easily; one key per item; reads are instant | lose up to one flush interval of counts if Redis dies (acceptable for views); Redis is on the hot path |
| **Local aggregation** in each API server, flush every second | a `map[id]int64` under a mutex; a ticker drains it with `INCRBY` or batched `UPDATE`s | 1,000× fewer writes; no extra hop per view | lose ≤ 1 s on a server crash; memory for the map (small) |
| **Sharded counters in the DB** | N rows per item (`counter_shards(item_id, shard, count)`), update a random shard; read = `SUM` | spreads the row lock N ways; stays in one durable system | N× rows, reads cost a `SUM`; still one DB write per view unless combined with batching |
| **Event stream** (Kafka) + stream processor | append events, aggregate per window, write results | exact, replayable, feeds analytics too | heavy for "a counter"; latency of a window |

**Choice: local aggregation → Redis → periodic flush to Postgres.** Two layers of batching:
the server collapses a second's worth of views to one `INCRBY` per item, Redis holds the live
number for reads, and a flusher moves deltas to the database every ~10 s for durability and
sorting. On a crash we lose at most a second per server plus up to 10 s of Redis deltas: for
views, agreed acceptable. If Redis is lost entirely, the database still has everything up to
the last flush.

The flush must write **deltas**, not absolute values: `GETDEL`/`GETSET` the Redis key to zero
atomically, then `UPDATE … SET views = views + $delta`. Writing absolute values from two flushers
would race.

### 9.3 Counting likes exactly, once per user

Exactness comes from a **row per (post, user)** with a primary key. The write is:

```sql
BEGIN;
INSERT INTO likes (post_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;
-- if the insert affected 1 row:
UPDATE posts SET likes = likes + 1 WHERE id = $1;
COMMIT;
```

- **Idempotent:** a retried request hits the conflict, inserts nothing, and skips the increment.
  The count can never drift from the rows.
- **Un-like:** `DELETE … RETURNING` and decrement only if a row was deleted. Same shape.
- **The hot row again:** 1k likes/s on one post is 1k updates/s on one `posts` row. That is near
  the edge for a single row but usually survivable; if not, keep the counter in Redis (`INCR` on
  successful insert) and **reconcile** periodically with `SELECT count(*) FROM likes WHERE post_id = $1`,
  or shard the counter as in 9.2. The rows remain the truth; the counter is a cache of them.
- **"Liked by me"** on a page of 50 posts: one query `WHERE user_id = $1 AND post_id = ANY($2)`,
  not 50 round trips.

### 9.4 Serving the number

Reads come from Redis (`views:{id}`, `posts:{id}:likes`); on miss, from the database column, then
cached. A count that is a few seconds behind is explicitly fine. For sorting ("most viewed this
week") use the database column and the index; Redis would need a sorted set, which is task 07.

Display formatting ("1.2M") belongs in the client. Send the integer.

### 9.5 Hot keys inside Redis, and unique viewers

- A single Redis key at 10k `INCRBY`/s is fine (~100k ops/s per node). At 1M/s for one key, shard
  the key itself: `views:{id}:{rand % 16}` and `SUM` the 16 on read (or let the flusher sum them).
  Local aggregation already makes this unnecessary at our scale.
- **Unique viewers:** a set of user IDs grows with the audience (10M viewers → 80 MB per video).
  **HyperLogLog** (`PFADD`, `PFCOUNT`) estimates distinct count with ~0.8% error in 12 KB, regardless
  of audience size, and HLLs can be merged (`PFMERGE`) for "unique viewers this week". Approximate is
  the agreed requirement; this is the structure built for it.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 1M views/s is still ~one `INCRBY` per server per item per second
  after local batching; Redis sees (servers × distinct hot items) ops/s, not events. Shard Redis by
  item if it grows. The database sees deltas every 10 s regardless of traffic.
- **Server crash:** up to 1 s of its views lost. Make the flush interval a knob; measure what you lose.
- **Redis down:** views fall back to writing deltas straight to the database at a slower flush
  (every 10 s per server) or get dropped with a metric. Likes are unaffected: they never depended on Redis.
- **Database down:** likes fail (`503`); views keep accumulating in Redis and flush later. Correct failure order.
- **Drift:** the `posts.likes` column vs `count(*)` of rows; a nightly reconciliation job repairs
  and alerts. For views, `videos.views` vs Redis after a flush should match exactly by construction.
- **Abuse:** a bot hammering `POST /view`. Rate-limit per client (task 03) and count a view only
  after N seconds of playback (the player sends the event, not the page load).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| local aggregation + Redis + delayed flush | `UPDATE +1` per view | 1,000× fewer DB writes, no hot-row lock queue | up to ~1 s + flush interval of view loss on crash |
| row per (post, user) + conditional increment | counter only | exact likes, idempotent retries, "liked by me" | 1B rows/yr to store and partition |
| deltas on flush | absolute values | safe with multiple flushers | slightly more complex flush |
| HyperLogLog for uniques | a set per video | 12 KB fixed vs tens of MB | ~0.8% error |
| counts also in a DB column | Redis only | sorting, durability | a reconciliation job |

## 12. Check yourself

1. Why does one `UPDATE … +1` per view fail at 10k/s on a hot video when Postgres can do thousands of updates per second?
2. What do you lose with write-behind for views, and why is the same design unacceptable for likes?
3. Walk through the like transaction when the client retries after a timeout. Why is the count still right?
4. Why must the flusher write deltas and not the current Redis value?
5. When would you reach for HyperLogLog and what do you trade?

<details><summary>Answers</summary>

1. The thousands-per-second figure is across different rows. On one row every update waits for the
   previous one's row lock and creates a dead tuple; 10k/s serialise on a single lock while vacuum
   falls behind. Contention on one hot row, not total throughput, is the limit.
2. On a crash you lose the increments buffered since the last flush (about a second per server plus
   the Redis flush interval). Views are approximate by requirement, so that's fine. Likes are exact
   and per user: losing one means a user's like silently disappears, so likes must be written
   durably in the same transaction as the row.
3. First attempt: insert succeeds, counter +1, commit; the response is lost. Retry: the insert hits
   the primary key conflict and affects 0 rows, so the code skips the increment. The row exists once
   and the count moved once. Idempotency comes from the (post, user) key, not from the client.
4. Two flushers (or one flusher and a restart) that each write the absolute value would overwrite
   each other and double- or under-count. Atomically swapping the Redis key to zero and adding that
   delta to the column composes correctly no matter how many flushers run.
5. Counting *distinct* viewers when the audience is large: a set costs 8 B per viewer, an HLL costs
   12 KB flat and merges across windows. You trade exactness (~0.8% error) for constant memory; fine
   for display, wrong for billing.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) (mutex-guarded map + `time.Ticker` flush loop, graceful drain on shutdown), [47 Low-latency II](../../47-low-latency-gc-contention.md) (atomics and sharding for a hot counter), [22 database/sql](../../22-database.md) (transactions, `RowsAffected`, `ON CONFLICT`), [44 Background jobs](../../44-background-jobs-queues.md) (the flusher), [59 Production DB patterns](../../59-production-database-patterns.md).
- Build the aggregator: `map[int64]int64` + `sync.Mutex`, `Add(id)`, and `Flush() map[int64]int64` that swaps in a fresh map under the lock so the slow Redis write happens outside the critical section. Run it under `-race`.
- Packages: `go-redis` (`IncrBy`, `GetDel`, `PFAdd`/`PFCount`, pipelining), `pgx` (batch updates).

## 14. Extensions

- **Trending:** time-decayed scores (views in the last hour weighted by recency) feeding a sorted set (task 07).
- **Per-hour time series** of views for charts: bucketed keys `views:{id}:{hour}` with TTL, or a time-series table.
- **Exactly-once view counting** via Kafka + idempotent consumer: what does it cost compared with the batching design?
- **Sharded counters in Postgres** as a pure-database alternative; benchmark against a hot single row.
- **Fraud signals:** views per IP per video, velocity checks; where in the pipeline do they go?
- **Like notifications** ("Alice liked your post") via task 12 without touching the like transaction's latency.
