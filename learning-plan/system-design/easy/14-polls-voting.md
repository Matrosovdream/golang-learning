# 14 · Polls & Voting

`🟢 easy` · *Topics:* one-vote-per-user enforced atomically, counters vs rows (exactness), caching results for a viral read load, anonymity in the schema, closing time · *Builds on:* [06 View & Like Counter](06-view-like-counter.md), [05 Cache](05-kv-cache-ttl.md), [03 Rate Limiter](03-rate-limiter.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [13 Shopping Cart](13-shopping-cart.md) · Next: [15 Job Scheduler](15-job-scheduler.md)

---

## 1. The task

> "Design polls, like on Twitter or in Slido: someone creates a poll with a few options, people
> vote once each, and everyone sees the results update in near real time until the poll closes."

The like counter from task 06 with three twists: a vote is a choice among options, it may be
changed, and results are read by thousands of people at once while a poll is viral.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Who can vote? | Anonymous voting can't enforce one vote per person. | Logged-in users only. |
| One vote per user? Can it be changed? | Dedup and update semantics. | One vote; changeable until the poll closes. |
| Results visible before voting? | Affects the read path and incentives. | Visible after you vote, or when the poll closes. |
| Anonymity? | Whether `user → option` is stored. | Voters' choices are private; the creator sees counts only. |
| Scale of a viral poll? | Hot-key design. | 1M votes in 10 minutes (~1.7k/s, bursts 10k/s), 100k viewers refreshing. |
| How fresh must results be? | Caching window. | Within ~2 s. |
| Poll lifetime and closing? | Hard stop semantics. | Creator sets a close time (1 h–7 d); votes after that are rejected. |
| Options count? Multi-select? | Schema. | 2–10 options; single-select now. |
| Audit / recount? | Exactness vs approximate counters. | Counts must be exact at close. |

## 3. Requirements

**Functional**
- Create a poll (question, 2–10 options, close time).
- Vote once per user; change the vote until close.
- Show per-option counts and percentages, live while open, frozen after close.
- Voters' individual choices are not exposed.

**Non-functional**
- Exact counts; a retried vote never counts twice.
- Results read at 50k/s during a viral poll with ≤ 2 s staleness.
- Votes accepted at 10k/s bursts on one poll without data loss.

**Out of scope**
- Ranked choice, multi-select, comments, moderation.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Vote writes, viral poll | 1.7k/s average over 10 min, 10k/s peak | given |
| Result reads, viral poll | 50k/s | 100k viewers polling every 2 s |
| Votes per day, platform-wide | ~50M | many small polls |
| `votes` rows | 50M/day × ~40 B = 2 GB/day | 1 year ≈ 700 GB; partition by poll creation month |
| Results payload | ~300 B | 10 options × (id, count) |
| Database if naive | 10k/s updates on ≤ 10 option rows | the hot-row problem from task 06, concentrated on a handful of rows |

What the numbers tell us: writes are spread across users (one row each, fine) but the **counts**
concentrate on ten rows; reads concentrate on one 300-byte document. Both are classic cache
shapes: counters in Redis, a result snapshot with a 1–2 s TTL.

## 5. Plan

1. Enforce one vote per user and keep counts exact: the write transaction.
2. Where counts live under a 10k/s burst on one poll.
3. Serve results at 50k/s.
4. Anonymity in the schema.
5. Closing, and freezing results.
6. Abuse and failure modes.

## 6. High-level design

```
 vote ──► API ──► rate limit (task 03) ──► Postgres TX:
                                             INSERT votes(poll_id, user_id, option_id)
                                               ON CONFLICT (poll_id, user_id) DO UPDATE SET option_id = EXCLUDED.option_id
                                               WHERE votes.option_id <> EXCLUDED.option_id
                                               RETURNING (old option_id)
                                             → if new: counts[opt] += 1 ; if changed: counts[old] −= 1, counts[new] += 1
                                           (counts as Redis HINCRBY, reconciled to poll_options.count periodically)
 results ──► API ──► Redis "results:{poll}" JSON, TTL 2 s ──miss──► HGETALL counts:{poll} → render → SET EX 2
 close ──► scheduler (task 15) at close_at: final reconcile count(*) GROUP BY option → poll_options.count, status=closed, cache results forever
```

The vote is one upsert keyed by `(poll_id, user_id)` so a user has exactly one row. The
transaction's outcome (new vote, changed vote, or no-op) tells us which counters to move. Counts
live in Redis for speed and are reconciled against the rows, which remain the truth.

## 7. API

```
POST /polls            { "question": "…", "options": ["Go", "Rust", "Zig"], "closes_at": "…" }  → 201 { id, … }
GET  /polls/{id}       → 200 { question, options: [{ id, text }], closes_at, status, my_vote?: option_id,
                                results?: { total, options: [{ id, count, pct }] } }   // results present only if voted or closed
PUT  /polls/{id}/vote  { "option_id": 2 }   → 200 { my_vote: 2, results }   idempotent; changes the vote if different
                                            → 409 poll closed · 404 unknown option
DELETE /polls/{id}/vote                     → 204 (retract, if allowed)
GET  /polls/{id}/results                     → 200 results   (cacheable; 403 if not yet entitled)
```

`PUT` for the vote: setting your choice twice is the same as once; changing it is the same
verb. Same lesson as task 13.

## 8. Data model

```sql
CREATE TABLE polls (
    id          BIGINT PRIMARY KEY,
    creator_id  BIGINT NOT NULL,
    question    TEXT NOT NULL,
    closes_at   TIMESTAMPTZ NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open',     -- open | closed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE poll_options (
    poll_id     BIGINT NOT NULL REFERENCES polls(id),
    id          SMALLINT NOT NULL,                -- 1..10 within the poll
    text        TEXT NOT NULL,
    count       BIGINT NOT NULL DEFAULT 0,        -- reconciled counter (truth = votes rows)
    PRIMARY KEY (poll_id, id)
);
CREATE TABLE votes (
    poll_id     BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    option_id   SMALLINT NOT NULL,
    voted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (poll_id, user_id)                -- one vote per user, enforced here
);
```

Redis: `counts:{poll}` hash `{option_id: count}`; `results:{poll}` rendered JSON with TTL 2 s;
`voters:{poll}` optional set for fast "have I voted" checks.

## 9. Deep dives

### 9.1 One vote per user, exactly

The primary key `(poll_id, user_id)` is the whole guarantee. The write is an **upsert**:

```sql
INSERT INTO votes (poll_id, user_id, option_id) VALUES ($1, $2, $3)
ON CONFLICT (poll_id, user_id) DO UPDATE
    SET option_id = EXCLUDED.option_id, voted_at = now()
    WHERE votes.option_id <> EXCLUDED.option_id
RETURNING (xmax = 0) AS inserted, option_id;
```

Three outcomes, each telling the counter logic what to do:

| Outcome | Rows affected | Counter moves |
|---|---|---|
| first vote | 1, `inserted = true` | `+1` on the new option |
| changed vote | 1, `inserted = false` | `−1` old, `+1` new (read the old value first, in the same transaction with `FOR UPDATE`, or return it via a CTE) |
| same vote again (retry) | 0 (the `WHERE` filtered it) | nothing |

Idempotency falls out of the data: a retried identical request is a no-op by construction, not
by remembering request IDs. Also check `closes_at > now()` in the same statement (join on
`polls`), so a vote can't slip in after closing.

### 9.2 Where the counts live

Task 06 showed `UPDATE poll_options SET count = count + 1` at 10k/s on ten rows is a lock queue.
Options:

| Approach | Exactness | Burst handling | Notes |
|---|---|---|---|
| counters in the same DB transaction | exact always | poor at 10k/s on ten rows | fine for ordinary polls (< 500 votes/s) |
| **rows in DB (truth) + Redis `HINCRBY` after commit + periodic reconcile** | exact at reconcile; live counts may drift by a few on failures | excellent | drift is repaired by `SELECT option_id, count(*) … GROUP BY` every N seconds for hot polls, and at close |
| Redis-only vote (Lua: `SADD voters NX` + `HINCRBY`) with async persistence | exact in Redis; lossy if Redis fails before persistence | excellent | unacceptable here: votes must not be lost |
| event stream + aggregation | exact and replayable | excellent | heavier than needed |

**Choice: rows as truth, Redis counters for live display, reconciliation for exactness.** The
transaction commits the row; then (best effort) the API applies the counter deltas with one
`HINCRBY` pipeline. If that fails, the next reconcile fixes it. Reconcile frequency scales with
heat: a viral poll every 5 s (one `GROUP BY` over an indexed range, cheap), a quiet poll only at
close. At close, the final reconcile writes `poll_options.count` and that becomes the frozen result.

For ordinary polls (the 99.9%), the counter update can simply be in the transaction; use the
Redis path when a poll's rate crosses a threshold. Both paths produce the same rows, so switching is safe.

### 9.3 Serving results to 100k viewers

Fifty thousand reads a second of the same 300-byte document is the easiest problem in the file:

- Render results JSON once, store `results:{poll}` with `EX 2`. All readers hit that key; on
  expiry one request (per API instance, with `singleflight`) rebuilds it from `counts:{poll}`.
- Clients poll every 2–5 s with jitter. Long-polling or SSE (lesson 58) reduces requests but
  needs connection fan-out; at 100k viewers plain polling through the cache is cheaper and simpler.
- After close, the results are immutable: cache for a day, set `Cache-Control` so the CDN can hold them.
- Entitlement ("results only after you vote"): check `votes` for `(poll, user)` (or the
  `voters:{poll}` set); the results document itself is the same for everyone, so it stays shared.

### 9.4 Anonymity

The `votes` table stores `(poll_id, user_id, option_id)`, so anyone with database access knows
how everyone voted. If voter choices must be private even from us:

| Approach | What it protects | Cost |
|---|---|---|
| don't expose it in any API; restrict DB access | against product leaks | nothing; the data exists |
| **split tables:** `voters(poll_id, user_id)` for dedup + `ballots(poll_id, option_id, random_id)` for counting, written in one transaction with no join key | the DB can't say who voted for what | changing a vote requires knowing the old ballot: store an **HMAC(user_id, poll secret)** as the ballot key instead of the user ID; the secret is deleted at close |
| encrypt `option_id` per voter | partial | key management, still linkable |

The split-table design is the honest answer to "voters are anonymous". Our requirement is
"private from other users and from the creator", so the simple table plus access control is
enough, and we note the stronger design as an extension.

### 9.5 Closing

- `closes_at` is enforced in the vote statement (`AND p.closes_at > now()`), so there is no race
  with a job flipping a status.
- A scheduled job (task 15) at `closes_at` runs the final reconcile, sets `status = closed`, and
  re-renders the results with a long TTL. If the job is late, votes were still rejected on time;
  only the "closed" badge and the frozen numbers lag.
- Editing options after votes exist would corrupt meaning; disallow edits once any vote exists.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 100k votes/s on one poll. `votes` inserts spread over users are
  fine for Postgres with partitioning; the Redis hash with ten fields takes 100k `HINCRBY`/s on one
  key, near a single node's limit. Apply task 06's local aggregation (per-instance delta map,
  flush every 200 ms) in front of Redis.
- **Redis down:** votes still commit (rows are truth); live counts are unavailable; serve the last
  rendered results from an in-process cache with a "results delayed" flag; reconcile when Redis returns.
- **Postgres down:** voting fails with `503`; results still served from cache. Correct priority.
- **Bots:** logged-in only plus rate limits per user and per IP (task 03); detect accounts created
  minutes ago voting in bulk; the creator can't see individual voters, but trust & safety can.
- **Clock skew at close:** use the database's `now()` in the statement, one clock for everyone.
- **Hot poll detection:** count votes per poll per minute in Redis to switch a poll to the
  "Redis counters + frequent reconcile" path automatically.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| upsert on `(poll, user)` | check-then-insert | atomic one-vote and idempotency | careful SQL |
| rows as truth + Redis counters + reconcile | counters in the transaction | 10k/s bursts on one poll | live numbers may be off by a few until reconcile |
| 2-second rendered results cache | per-request aggregation | 50k reads/s for free | ≤ 2 s staleness |
| polling with jitter | SSE fan-out | simplicity at 100k viewers | more HTTP requests |
| close enforced in SQL | status flipped by a job | no late votes | the job only does bookkeeping |
| simple table + access control | split anonymous ballots | simplicity | we *could* know how you voted |

## 12. Check yourself

1. How is "one vote per user" enforced, and why does a retried request not double count?
2. A user changes their vote from A to B. What must happen to the counters, and in what order relative to the row update?
3. Why cache the rendered results for 2 s instead of recomputing per request, and what replaces the cache after close?
4. Where do you enforce the closing time, and why not in a scheduled job that flips a flag?
5. What would you change in the schema to make voter choices unknowable even to the database admins?

<details><summary>Answers</summary>

1. By the primary key `(poll_id, user_id)` on `votes` and an upsert that only updates when the
   option differs. A retry with the same option matches the `WHERE` clause's "no change" case,
   affects zero rows, and triggers no counter movement.
2. `−1` on A and `+1` on B. The row update is the truth and happens first in the transaction (read
   the old option under the row lock, update it). The counter deltas follow (same transaction for
   normal polls; after commit via Redis for hot polls, with reconciliation covering any failure).
3. 100k viewers polling every 2 s is 50k identical reads; rendering once per 2 s turns that into a
   single aggregation per interval. After close the results are immutable, so the same document is
   cached for a long time and can be pushed to a CDN.
4. In the vote statement itself, by joining `polls` and requiring `closes_at > now()` with the
   database clock. A flag flipped by a job can be late or run on a skewed clock, letting votes in
   after the deadline; the job is only for the final reconcile and the "closed" badge.
5. Split `votes` into `voters(poll_id, user_id)` for dedup and `ballots(poll_id, option_id, key)`
   for counting, where `key = HMAC(poll_secret, user_id)` so a vote can still be changed; both rows
   written in one transaction; delete the poll secret at close so the link is gone forever.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (upsert with `ON CONFLICT … WHERE`, `RETURNING`, transactions), [59 Production DB patterns](../../59-production-database-patterns.md), [38 Caching](../../38-caching-patterns.md) (`singleflight` rebuilding `results:{poll}`), [58 Realtime](../../58-realtime-websockets-sse.md) (SSE as the alternative to polling), [15 Sync](../../15-sync-context.md) (local delta aggregation under a mutex), [68 Rate limiting](../../68-rate-limiting-throttling.md).
- Implementing the upsert's three outcomes is a good `pgx` exercise: use a CTE that selects the old option `FOR UPDATE`, then the upsert, returning both.
- Packages: `pgx`, `go-redis` (`HIncrBy` pipelines), `golang.org/x/sync/singleflight`, `crypto/hmac` for the anonymity extension.

## 14. Extensions

- **Multi-select and ranked-choice**: the row model becomes `(poll, user, rank, option)`; how do counts and results change?
- **Live results via SSE** with a fan-out layer: at what viewer count does it beat polling?
- **Anonymous ballots** with HMAC keys and secret deletion at close.
- **Geographic/demographic breakdowns** without exposing individuals (k-anonymity thresholds).
- **Fraud detection:** vote velocity per account age, IP clustering; where in the write path does a "shadow reject" go?
- **Embedding polls in other products** (task 10 webhooks on close with final results).
