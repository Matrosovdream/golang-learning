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
zscore = points × 2^20 + (2^20 − 1 − minutes_since_board_start)
```

Higher points dominate; among equal points, an earlier timestamp yields a larger remainder and
ranks higher. Keep the total under 2⁵³ so the float64 score stays exact (points up to ~2³³ with
a 20-bit time component). Decode `points = zscore >> 20` on read. The alternative, a secondary
sort in the application, works for a top-10 page but not for computing a rank.

Two details the formula hides:

- **The time part must fit in 20 bits.** Seconds since the board's start overflow it after 12 days
  (2²⁰ s), so use **minutes since the board's start**: 2²⁰ minutes ≈ 2 years, plenty for a monthly
  board. An all-time board needs more time bits (and so fewer points bits) or a coarser bucket.
  Players who reach the same score in the same minute still tie and fall back to member order.
- **Plain `ZINCRBY` no longer works.** Adding `points × 2²⁰` would keep the *old* time part, so the
  tiebreak would record when the player first scored, not when they reached this score (a player
  who went 10 → 100 at t=9 would beat one who hit 100 at t=5). Update with an atomic Lua script
  instead, the same pattern as task 03: `ZSCORE` → decode points → add → `ZADD` with the new
  composite. Sections 6 and 8 show plain `ZINCRBY` for the no-tiebreak case.

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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption; the 15k reads/s and the 5k/s burst are treated as peaks that coincide)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Score API (Go, stateless: HMAC check, Postgres transaction, Lua call, JSON) | 5k score writes/s + 10k top-10/s + 5k my-rank/s = 20k req/s | CPU | writes 5k × 0.5 ms = 2.5 vCPU; top-10 (1 s cached JSON) 10k × 0.05 ms = 0.5; rank 5k × 0.3 ms = 1.5; total 4.5 vCPU. N = ceil(4.5 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(2.1) = 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | the work is JSON, a signature check and waiting on Redis and Postgres; memory is ~1 GiB of 8, so `c` not `m`. 37 % busy normally, 56 % with one zone down |
| Redis (sorted sets `lb:global` + `lb:2026-10`) | 25M + 5M members = 2.5 + 0.5 = 3 GB (the 100 B per member already includes skip-list and hash overhead); ≈ 25k ops/s (10k Lua updates + 15k rank-path calls) | memory, then one core | 3 GB × 2 for the snapshot fork's copy-on-write under 5k writes/s (assumption) = 6 GB; `maxmemory` ≤ 70 % ⇒ ≥ 8.6 GiB RAM ⇒ next size is 16 GiB. Ops: 10k × ~30 µs (Lua) + 15k × ~10 µs ≈ 0.45 core | `r7g.large` (2 vCPU / 16 GiB), ElastiCache | 1 primary + 1 replica (other zone) | memory-bound, so the `r` family; commands run on one core, so only RAM matters, not vCPU. `m7g.large` (8 GiB) is too tight once fork headroom counts. The replica turns a node loss into a failover, not a ~1-minute rebuild (section 9.3) |
| Postgres (players, match_results, monthly_scores) | 5k commits/s at burst (580/s average), each = 1 insert + 2 updates; ≈ 200 GB | write IOPS and CPU, not memory | players 25M × ~100 B = 2.5 GB; monthly 5M × ~60 B = 0.3 GB; match_results 50M rows/day × ~130 B = 6.5 GB/day, 30 days hot ≈ 200 GB (retention is an assumption; older partitions go to S3). IOPS ≈ 2 per commit (assumption) ⇒ 10k + ~1k misses ⇒ `gp3` at 12,000 IOPS. CPU 5k × 0.6 ms = 3 vCPU of 8. Hot set ≈ 3 GB + today's index tail ≪ 32 GiB | `db.m7g.2xlarge` (8 vCPU / 32 GiB), RDS Multi-AZ, 250 GB `gp3` | 1 primary + synchronous standby | reads never reach it, so the hot set is small and `m` is enough; 8 vCPU and 12k IOPS are there for the evening burst. Use `r` if `match_id` is random and kept 7+ days: its index (~50 B × 50M × 7 ≈ 17 GB) is then touched at random and wants RAM; time-ordered ids avoid that |

Data out: 10k × ~1 KB (top-10 JSON + headers) + 5k × ~0.5 KB = 12.5 MB/s at peak ≈ 100 Mbit/s: no network bound on `c7g.xlarge`, but the bytes are a bill line (below). Nothing to size for the rebuild: `SELECT id, score FROM players` runs once on the standby.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Redis: 2 × `cache.r7g.large` (2 × ≈ $156) | $310 |
| Postgres: `db.m7g.2xlarge` ≈ $480, Multi-AZ doubles | $960 |
| gp3 250 GB + 9k extra IOPS: $20 + $45 = $65, ×2 for the standby | $130 |
| ALB: $16 + ≈ 15 LCU on average × $0.008 × 730 | $100 |
| Data out: average ⅓ of peak = 4.2 MB/s × 2.63M s ≈ 11 TB × $0.09 | $990 |
| **Total** | **≈ $2,810** (≈ $2,250 with 1-year commitments on the $1,590 of servers) |

- **Biggest line and why:** data out (35 %), narrowly ahead of the Postgres pair (34 %, 39 % with its disks): 15k small JSON responses a second to phones add up to ~11 TB, and the evening write burst needs a durable, Multi-AZ primary. The sorted set, the clever part of the design, is 11 % of the bill. Levers: gzip and a longer client poll interval for egress; writing ~10 results per commit would cut commits and IOPS about 10× and roughly halve the Postgres line.
- **What changes at 10×:** API 45 vCPU ⇒ N = ceil(45 ÷ 2.13) = 22 → 24 × `c7g.xlarge`. Redis holds 25 + 5 = 30 GB (section 10) and takes ~250k ops/s, far past one core: shard ~6 ways (score-range, as in section 10; ~5 GB and ~42k ops/s each, so `r7g.large` still fits), each with a replica. Postgres at 50k commits/s and ~100k IOPS is past one primary and one `gp3`: batch score writes, then shard by player id or move to `io2` / NVMe (`i4i`). Data out ≈ 110 TB ≈ $10k a month, which becomes the first thing to cut.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Two different jobs. The durable record is `match_results` (idempotent on `match_id`), `players.score` and `monthly_scores`: ~580 commits/s on average, 5k/s at the evening burst, each commit one insert and two updates, all by primary key (sections 4, 8, 15).
- The ranking is 25M members read at 15k/s: "top 10" and "my rank". Rank means "how many scores are higher", an order-statistic question, not a point read or a range scan (section 9.1).
- Scores must never be lost, but the board is derived: it is rebuilt from `players` in about a minute (section 9.3).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen as the system of record, ✖ as the rank path | the `match_id` primary key makes a replayed match insert nothing, and a transaction keeps `match_results` and `players.score` in step. But `count(*) WHERE score > x` walks every index entry above the player (up to 25M) and no index returns a rank directly, so 5k rank queries/s would spend the whole budget on counting (section 9.1) |
| MySQL (RDS / Aurora) | ✔ as the system of record, ✖ as the rank path | same verdict for the same reason: InnoDB has no rank index either. Everything the record needs (primary key, transactions, row locks) is portable |
| Redis (ElastiCache) | ✅ chosen as the derived ranking index | `ZREVRANK`, `ZREVRANGE` and `ZINCRBY` are O(log N) on a skip list with spans; both boards are ~3 GB, one `cache.r7g.large` with a replica; the board can be rebuilt, so durability is not required |
| MemoryDB | ✔ works if the board were the record | a durable sorted set would remove the rebuild, but the audit trail and the idempotent `match_id` still belong in a relational store, so you would pay for a second durable copy |
| DynamoDB | ✖ poor fit | the score table is a fine key-value shape, but it has no rank operation: "my rank" would be a count that reads every item above the score |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.2xlarge` from section 15) as the system of record, plus Redis on ElastiCache as the derived ranking index. Two properties decide it: a rank must be O(log N), which only the sorted set gives, and a score must be durable and idempotent on `match_id`, which a relational primary key gives. Postgres is the record; Redis is rebuilt from `SELECT id, score FROM players` and batched `ZADD` (section 9.3).

**If it were MySQL instead:** nothing material. The idempotent insert is `INSERT IGNORE` (or `ON DUPLICATE KEY UPDATE`) plus a rows-affected check instead of `ON CONFLICT DO NOTHING`, and InnoDB updates `players.score` in place, so there is no autovacuum to watch on a table updated ~5k times a second at the burst. One caution is the same in both: a random `match_id` splits pages (InnoDB's clustered table, Postgres's index; see task 04 and section 15), so use a time-ordered id.

**What would flip the choice:** below ~100k players SQL rank takes a few ms (section 9.1) and Redis is not worth a second system. At 10× (250M members, ~25 GB, section 10) the sorted set is sharded by score range. The database side hits ~50k commits/s (section 15), past one primary: batch match results first, then shard by player id or move `match_results` to DynamoDB, where a conditional put on `match_id` keeps the idempotency.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 500k score updates/s at the burst (58k/s average), 1.5M reads/s; 2.5B players (about half of all internet users: a thought experiment), ~300 GB of sorted sets, 2.5M Redis ops/s; ~450 vCPU of API | the one Postgres primary: 500k commits/s is 50× its ~10k durable-write ceiling (~1M IOPS against 16k on `gp3`). Right behind it, one Redis node: 300 GB and ~25 cores' worth of ops | shard `players`, `match_results` and `monthly_scores` by `player_id` across ~64 Postgres shards (7.8k commits/s each; batch ~10 results per commit, the lever of section 15). Redis by score range as in section 10: ~60 shards (~5 GB, ~42k ops/s each), boundaries cut at quantiles; rank = cached `ZCARD`s of higher shards + local `ZREVRANK`. ~213 `c7g.xlarge`. Give up: cross-shard queries, members that move shards when scores cross a boundary | ≈ $280k/month, linear; data out ($99k) is the biggest line |
| **1,000×** | 5M updates/s at the burst (580k/s average), 15M reads/s; 25B players (3× the world's population: a thought experiment), ~3 TB of sorted sets, 25M Redis ops/s; ~2,100 API nodes | running it: ~600 Redis shards + ~640 Postgres shards (each with a replica) + ~2,100 API nodes ≈ 4,600 nodes, so a node dies about every hour; re-cutting score ranges is itself an incident, and one global order means every rank sums counts over ~600 shards | independent **cells**: 100 cells by `hash(player_id)`, each the 10× design for 1 % of players (250M members, 6 Redis shards), behind a thin `player_id → cell` router; a cell fails alone. Global top 1,000 = merge of each cell's top 1,000 once a second; a global rank = in-cell rank × 100 (a cell is a uniform 1 % sample, error about 1/√rank). Give up: exact global rank below the top 1,000, cross-cell queries | ≈ $2.8M/month; data out ≈ $1M of it |
| **10,000×** | 50M updates/s at the burst, 150M reads/s; 250B players (30× the world's population: a thought experiment, read it as 10,000× the traffic); 125 GB/s ≈ 1 Tbit/s of replies; ~30 TB of sorted sets | the wire and the bill, not a component: ~1 KB replies at 150M/s are ~1 Tbit/s and ~110 PB a month of data out (≈ $10M); every phone asking the server for its rank is the cost | change the problem: the phone computes its own rank. Each cell publishes the top 10 and a cumulative score histogram (a few hundred log-spaced buckets, ~1 KB gzipped) as one shared object, cached by the CDN and on the device for 30-60 s; the client interpolates its percentile. Exact sorted sets stay only for the global top ~10k and small league or friends boards. Writes are coalesced per player per session (~10× fewer) into an append-only log. Give up: exact rank and "visible within a second" outside the top, ties inside a bucket | ≈ $28M/month if nothing changes (data out ≈ $10M); ~5-10× less once replies are shared and cacheable |

*Arithmetic:* 5k × 100 = 500k commits/s ÷ 10k = 50×; 2.5M ops/s ÷ 60 shards ≈ 42k; 300 GB ÷ 60 = 5 GB; 4.5 vCPU × 100 = 450 ÷ 2.13 = 211 → 213 nodes; at 1,000× 4,500 ÷ 2.13 → 2,112 API + 1,200 Redis + 1,280 Postgres nodes ≈ 4,600; 12.5 MB/s × 10,000 = 125 GB/s; 11 TB × 10,000 × $0.09 ≈ $10M.

**What stays the same at every scale:** the split between the record and the ranking: the database (later an append-only log) holds each score idempotently on `match_id`, and the sorted sets are derived and rebuildable; the composite `points × 2^20 + time` score keeps the tie rule exact wherever a rank is exact.

**Say it in an interview:** "At 100× one Postgres primary is 50× over its commit ceiling, so I shard results by player id and the sorted set by score range, about 60 shards, with rank as the sum of higher shards' sizes. At 1,000× the problem is operating ~4,600 nodes and one global order, so I split players into 100 hash cells, each today's 10× design, merge the top 1,000 globally and estimate a global rank as cell rank × 100. At 10,000× the wire and the bill are the limit, so the phone computes an approximate rank from a small cached histogram, and exact ranks exist only for the top and for small leagues."
