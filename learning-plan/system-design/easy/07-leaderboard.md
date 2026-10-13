# 07 · Leaderboard

`🟢 easy` · *Topics:* sorted sets (skip list + hash), top-K and rank queries, why SQL rank is slow, periodic boards, tie-breaking, Redis as a primary store with a database behind it · *Builds on:* [05 In-Memory Cache](05-kv-cache-ttl.md), [06 Counters](06-view-like-counter.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [06 View & Like Counter](06-view-like-counter.md) · Next: [08 Uptime Monitor](08-uptime-monitor.md)

---

## 1. The task

> "Design a real-time leaderboard for a mobile game. Players earn points by winning matches.
> Show the top 10 and tell each player their own rank among millions."

The top 10 is easy in any database. "My rank among 25 million players, right now" is the part
that forces a specific data structure.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| How many players? How many active daily? | Memory for the board; write rate. | 25M monthly, 5M daily active. |
| How often does a player score? | Write rate. | ~10 matches/day per active player. |
| Must rank be exact and real-time? | Exact + real-time rules out batch jobs. | Exact, visible within a second. |
| Which boards? Global only, or per country, per friends, per week? | Each extra dimension is another sorted structure. | Global all-time + a monthly board that resets. Friends later. |
| Ties? | Ordering among equal scores must be defined. | Earlier to reach the score ranks higher. |
| Who reports scores? | Client-reported scores are trivially cheated. | The game server, after validating the match. |
| Read rate? | Top-10 is read on every lobby screen. | ~10k reads/s for top-10, ~5k/s for "my rank". |
| Durability? | If the board is lost, can it be rebuilt? | Scores must never be lost; the board can be rebuilt from them. |

## 3. Requirements

**Functional**
- Add points to a player's score (global and current-month boards).
- Return the top N players with scores.
- Return a player's rank and score, and the players immediately around them.
- Monthly board resets at month start; previous months are viewable.

**Non-functional**
- Updates visible within a second; rank queries in milliseconds.
- Scores are durable; the board can be rebuilt after a cache loss.
- Server-authoritative scoring.

**Out of scope**
- Anti-cheat heuristics, matchmaking, achievements, friends boards (extension).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Score updates | ~580/s average; bursts 5k/s in the evening | 5M × 10 / 86,400 |
| Reads | 15k/s | given |
| Sorted set memory, 25M members | ~2.5 GB | ~100 B per member (score 8 B, member ID, skip-list and hash overhead) |
| Monthly board | ~0.5 GB | 5M active members |
| SQL rank query | `COUNT(*) WHERE score > x` scans up to 25M index entries | milliseconds to seconds per query; too slow at 5k/s |

What the numbers tell us: writes are trivial. The data for one board fits comfortably in the
memory of one node. The decisive number is the cost of a **rank** query, which in SQL is a scan
and in a sorted set is a logarithmic walk.

## 5. Plan

1. Show where the relational approach stops working (rank, not top-K).
2. Introduce the sorted set and its complexities.
3. Decide the roles: Redis for ranking, the database for truth.
4. Periodic boards and tie-breaking.
5. Reads, caching, and what happens when Redis is lost.
6. Beyond one node: sharding a sorted set (sketch).

## 6. High-level design

```
 game server ──"match won: player 42 +25"──► Score service
                                               │  1. Postgres: INSERT match_results; UPDATE players SET score = score + 25
                                               │  2. Redis:    ZINCRBY lb:global 25 42
                                               │               ZINCRBY lb:2026-10 25 42
                                               ▼
 lobby ──GET /leaderboard/top?n=10──► Score service ──► Redis ZREVRANGE lb:global 0 9 WITHSCORES   (cached 1 s)
 profile ──GET /leaderboard/me──────► Score service ──► Redis ZREVRANK + ZSCORE + ZREVRANGE rank-2..rank+2
                                                            │
 rebuild on loss: SELECT id, score FROM players ──► ZADD in batches ──┘
```

A score update is written to the database first (truth), then applied to the sorted sets
(ranking). Reads never touch the database: top-N and rank are single Redis calls.

## 7. API

```
POST /scores                       (internal, from the game server; signed)
  { "player_id": 42, "points": 25, "match_id": "m-9f3a" }   → 202
  idempotent on match_id: replaying the same match adds nothing

GET /leaderboard/top?board=global&n=10
  200 { "board": "global", "entries": [ { "rank": 1, "player_id": 7, "score": 98210 }, … ] }

GET /leaderboard/me?board=2026-10
  200 { "rank": 183422, "score": 1240, "around": [ …rank-2..rank+2… ] }

GET /leaderboard/top?board=2026-09&n=100&offset=100   previous months, paginated by rank
```

## 8. Data model

Postgres, the source of truth:

```sql
CREATE TABLE players (
    id          BIGINT PRIMARY KEY,
    score       BIGINT NOT NULL DEFAULT 0,      -- all-time
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE TABLE match_results (
    match_id    TEXT PRIMARY KEY,               -- idempotency key
    player_id   BIGINT NOT NULL,
    points      INTEGER NOT NULL,
    played_at   TIMESTAMPTZ NOT NULL
);
CREATE TABLE monthly_scores (
    month       DATE   NOT NULL,                 -- 2026-10-01
    player_id   BIGINT NOT NULL,
    score       BIGINT NOT NULL,
    PRIMARY KEY (month, player_id)
);
```

Redis, the ranking structure:

```
ZINCRBY lb:global  25 "42"          score → member, kept sorted
ZINCRBY lb:2026-10 25 "42"
ZREVRANGE lb:global 0 9 WITHSCORES  → top 10               O(log N + 10)
ZREVRANK  lb:global "42"            → 0-based rank          O(log N)
ZSCORE    lb:global "42"            → score                 O(1)
ZREVRANGE lb:global r-2 r+2         → neighbours
```

## 9. Deep dives

### 9.1 Why not just SQL

`SELECT … ORDER BY score DESC LIMIT 10` with an index on `score` is instant: the database reads
the first 10 index entries. **Rank** is different: "how many players have a higher score" is
`SELECT count(*) FROM players WHERE score > $my_score`, which walks every index entry above
mine. For a mid-table player that's millions of entries per query. At 5k rank queries/s that is
the whole database budget spent on counting.

Window functions (`RANK() OVER (ORDER BY score DESC)`) compute all ranks at once, which is
perfect for a nightly snapshot, not for a per-request answer.

| Scale | SQL rank | Verdict |
|---|---|---|
| < 100k players | a few ms | fine, use SQL |
| millions of players, real-time | tens to hundreds of ms, scales with position | too slow and CPU-heavy at 5k/s |

### 9.2 The sorted set

Redis's `ZSET` is two structures over the same members: a **hash** (member → score, O(1)) and a
**skip list** ordered by score (O(log N) insert, delete, rank-by-score, and range-by-rank). The
skip list keeps a *span* count on each forward pointer, which is what makes "what is the rank of
this score" a logarithmic walk instead of a count. A balanced tree with subtree sizes gives the
same complexities; the skip list is simpler to implement and was Redis's choice.

| Operation | Complexity | Our use |
|---|---|---|
| `ZINCRBY` | O(log N) | score update |
| `ZREVRANGE 0 9` | O(log N + 10) | top 10 |
| `ZREVRANK` | O(log N) | my rank |
| `ZREVRANGE r-2 r+2` | O(log N + 5) | neighbours |
| `ZCARD` | O(1) | total players |

25M members at ~100 B each is ~2.5 GB: one node. For comparison, materialising ranks in a table
would require rewriting up to 25M rows on every score change.

### 9.3 Who is the source of truth

| Option | Pros | Cons |
|---|---|---|
| **Redis only** (with AOF persistence) | one write, simplest | a Redis failure or a bug loses scores; no joins, no history, no audits |
| **Database only** | durable, auditable | rank is slow (9.1) |
| **Database for truth, Redis for ranking** | durable *and* fast; the board is a derived structure that can be rebuilt | two writes per update; they can diverge briefly |

**Choice: database for truth, Redis as the derived index.** Write the database first (the
match result and the score), then the `ZINCRBY`. If the Redis write fails, a retry or the
periodic reconciliation fixes it; the match was recorded. If Redis is lost entirely, rebuild
with `SELECT id, score FROM players` and batched `ZADD` (25M members at ~500k/s of pipelined
`ZADD` is about a minute). Serve a "leaderboard updating" message meanwhile.

The `match_results.match_id` primary key makes score submission idempotent: the game server can
retry safely and a replayed match cannot award points twice. That protects both the database
and the sorted set, because the `ZINCRBY` happens only when the insert succeeded.

### 9.4 Monthly boards and resets

- One sorted set **per period**: `lb:2026-10`. A reset is just the next key name; nothing is
  deleted or copied at midnight.
- Writes go to the current month's key (computed from the server clock, UTC, documented).
- Old months stay readable; set an expiry (e.g. 13 months) or archive the top 1,000 to the database
  and drop the set.
- "All-time" and "this month" cost two `ZINCRBY` per update. Weekly or daily boards are more keys,
  same pattern. Aggregating across periods uses `ZUNIONSTORE` offline, not per request.

### 9.5 Ties

Redis orders equal scores lexicographically by member, which is deterministic but meaningless
("player 1000" beats "player 999"). Our rule is "earlier to reach the score wins". Encode it in
the score itself: scores are integers, so use a composite

```
zscore = points × 2^20 + (2^20 − 1 − seconds_since_epoch_bucket)
```

Higher points dominate; among equal points, an earlier timestamp yields a larger remainder and
ranks higher. Keep the total under 2⁵³ so the float64 score stays exact (points up to ~2³³ with
a 20-bit time component). Decode `points = zscore >> 20` on read. The alternative, a secondary
sort in the application, works for a top-10 page but not for computing a rank.

### 9.6 Reads and caching

- Top-10 at 10k/s: `ZREVRANGE` is cheap, but cache the JSON for one second in the service anyway;
  everyone sees the same page, and it removes 10k Redis calls per second for one.
- "My rank" cannot be shared-cached (per player), but it's two O(log N) calls. Fine.
- Pagination deep into the board (`offset=1,000,000`) is `ZREVRANGE 1000000 1000099`: still
  O(log N + 100) in a skip list, unlike SQL's `OFFSET` which scans.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 250M members ≈ 25 GB in one sorted set: too big for one node to
  handle comfortably (failover copies 25 GB). Shard by **score range** (node A holds scores
  0–999, node B 1,000–9,999, …): top-K comes from the top node; a rank is the sum of `ZCARD` of
  all higher nodes plus the local `ZREVRANK`. Members move between nodes as scores cross
  boundaries. Hash-sharding by player is simpler but makes top-K a scatter-gather and rank a sum
  of per-shard counts (`ZCOUNT`), still O(shards × log N). Medium track.
- **Redis down:** reads fail or serve the last cached top-10; writes still hit the database and
  are replayed to Redis later (keep an "unapplied" marker or just rebuild). The game continues.
- **Hot player:** a streamer with 100k rank lookups/s against the same member. Cache that one
  response for a second, like the top-10.
- **Score inflation / cheating:** only the game server may post scores, signed; idempotency on
  `match_id`; sanity bounds on points per match.
- **Clock at month boundary:** two servers disagreeing on the month for a few seconds would split
  writes across keys. Use the match's `played_at` from the database transaction to pick the key.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Redis sorted set | SQL `COUNT(*)` rank | O(log N) rank at any scale | a second system to keep in sync |
| DB first, then `ZINCRBY` | Redis as the only store | durability, audits, rebuildability | double write, brief divergence |
| key per period | resetting one key | zero-cost resets, history for free | more memory while periods overlap |
| composite score for ties | application-side sort | exact rank with the tie rule | 20 bits of score space, encode/decode |
| idempotency on `match_id` | trusting the game server's retries | no double awards | one more table |

## 12. Check yourself

1. Why is "my rank" slow in SQL when "top 10" is fast?
2. What two structures make up a sorted set, and which one answers `ZREVRANK`?
3. How much memory for 25M players, and what would you do at 250M?
4. The `ZINCRBY` fails after the database commit. What is the state, and how does it heal?
5. How do you implement "earlier to the score ranks higher" without a second sort?

<details><summary>Answers</summary>

1. Top 10 reads the first ten entries of the score index. Rank must *count* every entry above my
   score, which for a typical player is millions of index entries per query, and the cost grows with
   how far down the board you are.
2. A hash (member → score, O(1) lookup) and a skip list ordered by score whose forward pointers
   carry span counts. The skip list answers `ZREVRANK` in O(log N) by summing spans along the search path.
3. About 2.5 GB at ~100 B per member: one node. At 250M (~25 GB) shard by score range so top-K
   stays on one node and rank is a sum of higher nodes' cardinalities plus the local rank.
4. The match and score are durable in the database; the board is behind by 25 points for that
   player. A retry of the `ZINCRBY`, or a periodic reconciliation (compare `players.score` with
   `ZSCORE`, fix differences), heals it. The database being first means nothing is lost, only delayed.
5. Encode the tiebreaker into the score: `points × 2^20 + (2^20 − 1 − time_bucket)`, so equal
   points differ by a time component where earlier is larger. Decode points on read. Keep the
   total below 2⁵³ so the float64 score is exact.

</details>

## 13. In Go

- Related lessons: [52 Heaps & priority queues](../../52-heaps-priority-queues.md) (top-K from a stream with a min-heap), [42 Trees](../../42-trees.md) (what an order-statistic tree needs: subtree sizes), [38 Caching](../../38-caching-patterns.md) (the 1-second top-10 cache), [22 database/sql](../../22-database.md) (the idempotent insert).
- Exercise: implement a **skip list with spans** in Go and verify `Rank(member)` against a sorted slice on random data. It is the data structure behind the whole task.
- Packages: `go-redis` (`ZIncrBy`, `ZRevRangeWithScores`, `ZRevRank`, pipelines), `pgx`.

## 14. Extensions

- **Friends leaderboard:** fetch the friend IDs (small), pipeline `ZSCORE` for each, sort in the app; compare with `ZINTERSTORE` against a friends set.
- **Score-range sharding** across Redis nodes with the rank-by-summing-cardinalities algorithm; what happens when a player crosses a boundary?
- **Percentile** instead of rank ("top 3%"): `ZREVRANK / ZCARD`; cheap and often a better UX.
- **Time decay** for a "hot this week" board (combine with task 06's trending).
- **Seasons** with rewards: snapshot the top 1,000 to Postgres at period end, atomically, and handle late-arriving match results.
- **Anti-cheat signals:** points per minute per player, impossible deltas, device fingerprints.
