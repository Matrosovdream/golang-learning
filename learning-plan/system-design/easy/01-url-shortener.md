# 01 · URL Shortener

`🟢 easy` · *Topics:* read-heavy service, key generation, base62, cache-aside, HTTP redirects · *Builds on:* nothing, this is the baseline

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Next: [02 Pastebin](02-pastebin.md)

---

## 1. The task

> "Design a URL shortening service like bit.ly. A user gives us a long URL and gets back a short
> one. Anyone opening the short URL is sent to the long one."

That is all an interviewer will say. Everything else you have to ask.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| How many new URLs per day? How many redirects? | Decides whether one database is enough and how big the cache must be. | 100M new URLs/day; reads are 10× writes. |
| How short is "short"? | Fixes the key space and therefore the key-generation scheme. | As short as possible; 7 characters is fine. |
| Can users pick a custom alias? | Adds a uniqueness check and a conflict error. | Yes, optional. |
| Do links expire? Can they be deleted or edited? | Expiry needs a cleanup story; edits break caching. | Optional expiry, default never. No edit, no delete. |
| Same long URL twice: same short URL or a new one? | Dedup needs an index on the long URL (large) and changes semantics per user. | A new one each time. Simplest. |
| Do we need analytics? | Click tracking is a write on every read, which changes the hot path. | Click count only. Nothing else. |
| Availability or consistency? Latency target? | A redirect must be fast; a stale read for a second is acceptable. | Highly available; redirect in well under 100 ms. |
| Who can create links? | Open creation needs abuse protection. | Logged-in users via API key. |

## 3. Requirements

**Functional**
- Create a short URL for a long URL, optionally with a custom alias and an expiry.
- Opening a short URL redirects to the long URL.
- Count clicks per short URL.

**Non-functional**
- Redirects are fast (p99 under 100 ms server-side) and highly available.
- Short codes are unique. Never two long URLs behind one code.
- Reads vastly outnumber writes.

**Out of scope**
- Editing or deleting links, detailed analytics, user dashboards, link previews.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Writes | ~1,200/s | 100M/day ÷ 86,400 |
| Reads | ~12,000/s, peaks ~50,000/s | 10× writes; peak 4× |
| Records in 10 years | 365 billion | 100M × 365 × 10 |
| Storage | ~37 TB | 365B × ~100 B per row (code + URL + timestamps) |
| Key space needed | 62⁷ ≈ 3.5 trillion | ~10× the 10-year total, so 7 base62 chars are enough |
| Cache | ~10 GB | assume 20% of the ~100M distinct codes read per day serve 80% of reads: 20M × 500 B |

What the numbers tell us: writes are modest, one primary database handles 1,200/s. Reads are
the problem, so the hot path is cache → database and the cache must hold the hot set. Storage
reaches tens of terabytes, so at some point the table must be partitioned, but not on day one.

## 5. Plan

1. Settle the API: one create endpoint, one redirect endpoint.
2. Pick the key-generation scheme, since everything downstream depends on it.
3. Draw the simplest design: load balancer, stateless servers, one database.
4. Add a cache for the read path and decide the redirect status code.
5. Deep-dive the two real decisions: key generation and 301 vs 302.
6. Finish with scaling (sharding, replicas) and failure modes.

## 6. High-level design

```
            create (1.2k/s)                              redirect (12k/s, bursts 50k/s)
 client ──POST /api/v1/urls──► LB ──► API servers ──► ID generator ──► Postgres (primary)
                                        │   ▲                              │
                                        │   └──── cache miss ───────────── ┘  (read replicas later)
                                        ▼
 client ◄──302 Location────── LB ◄── API servers ◄──► Redis (code → long URL, hot set)
                                        │
                                        └──► click events ──► queue ──► counter worker ──► Postgres
```

**Write:** the server validates the URL, asks the ID generator for the next ID, encodes it in
base62, inserts `(code, long_url, …)`, returns the short URL. Nothing is cached on write; the
first read warms it.

**Read:** the server looks the code up in Redis. Hit: respond `302` with `Location`. Miss: read
the database, store in Redis with a TTL, respond. The click count is incremented
*asynchronously*: the redirect never waits for an analytics write.

## 7. API

```
POST /api/v1/urls
  body:   { "long_url": "https://…", "custom_alias": "launch2026"?, "expires_at": "2027-01-01T00:00:00Z"? }
  201:    { "short_url": "https://sho.rt/launch2026", "code": "launch2026", "expires_at": … }
  400:    invalid URL, alias too long/invalid characters
  409:    alias already taken
  429:    rate limited (see task 03)

GET /{code}
  302:    Location: <long_url>
  404:    unknown code
  410:    expired

GET /api/v1/urls/{code}
  200:    { "long_url": …, "created_at": …, "expires_at": …, "clicks": 1234 }
```

Why `/{code}` and not `/r/{code}`: the whole point is the shortest possible URL. Reserve a few
paths (`/api`, `/healthz`) so they can never collide with a code.

## 8. Data model

```sql
CREATE TABLE urls (
    code        VARCHAR(16) PRIMARY KEY,      -- base62 ID or custom alias
    long_url    TEXT        NOT NULL,
    user_id     BIGINT      NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,                  -- NULL = never
    clicks      BIGINT      NOT NULL DEFAULT 0
);
CREATE INDEX urls_user_id_idx ON urls (user_id);  -- "my links" later
```

The access pattern is a single lookup by `code`. That is a key-value shape, which is why this
service is equally at home in Postgres, DynamoDB, or Cassandra. We start with Postgres because
we know it; the schema would port unchanged.

## 9. Deep dives

### 9.1 Generating the short code

The problem: produce a unique 7-character code for ~1,200 URLs per second, across several
stateless servers, without two servers ever producing the same code.

| Option | How | Pros | Cons |
|---|---|---|---|
| **Hash the long URL** (MD5/SHA-256) and take the first 7 base62 chars | deterministic | no coordination; same URL → same code (free dedup) | collisions: 7 chars of a hash *will* collide; need check-and-retry with a salt; dedup conflicts with "new code each time" |
| **Database auto-increment → base62** | `INSERT` returns the ID, encode it | trivial, unique by construction, codes grow slowly from 1 char up | one sequence = one primary; codes are predictable (`abc` then `abd`), so links are enumerable |
| **Distributed ID generator → base62** (task 04) | Snowflake-style 64-bit IDs | no coordination at request time, scales horizontally | 64-bit IDs encode to 11 base62 chars, longer than wanted; needs a shuffle or a smaller ID |
| **Key Generation Service (KGS)** | pre-generate random unique codes into a table; servers take a batch of 1,000 at a time | codes are random (not enumerable), no collision check at request time, servers work offline from their batch | one more service to run; a server crash wastes its unused batch (fine, the space is huge) |

**Choice:** start with the **database sequence + base62** because it is the simplest correct
thing and codes stay short for years (62⁶ ≈ 57B before we need 7 chars). If enumerability is a
concern, mix in a step: `id × large odd constant mod 62⁷` is a bijection that scrambles the
order without a collision risk. Move to a KGS or task 04's generator when the single sequence
becomes the write bottleneck, which at 1,200/s it is not.

Base62 (`0-9a-zA-Z`) rather than base64: no `+`, `/`, `=`, which are unsafe or ugly in URLs.

### 9.2 301 or 302

| Code | Browser behaviour | Effect on us |
|---|---|---|
| **301 Moved Permanently** | caches the redirect; next click goes straight to the long URL and never touches our servers | fewer requests, lower cost; **click counts stop being correct**; the mapping can never change |
| **302 Found** (or 307) | asks us every time | every click is counted; expiry and future edits work; more load, which is exactly what the cache is for |

**Choice: 302.** Analytics is a requirement, so the browser must come back. If analytics were
out of scope and links were truly permanent, 301 would be the cheaper answer. Note 301/302 may
change a POST into a GET; 307/308 preserve the method. For links clicked in a browser (always a
GET) it does not matter; choose 302 for the widest compatibility.

### 9.3 Caching the redirect path

Cache-aside in Redis keyed by code, value the long URL plus `expires_at`, TTL about a day
(immutable data; a TTL only bounds memory). ~12k reads/s with a 90% hit rate leaves ~1.2k/s for
the database, the same order as the writes. Add read replicas if misses grow.

Cache what *does not* exist too: a `404` for a code that was never created costs a database
hit every time a bot probes random codes. Store a short-lived negative entry.

Expiry is checked on read (`expires_at < now()` → 410) so a stale cache can never serve an
expired link for long: cache the record with a TTL no longer than the time to expiry.

### 9.4 Counting clicks without slowing redirects

A synchronous `UPDATE urls SET clicks = clicks + 1` on every redirect turns our read-heavy
system into a write-heavy one (12k updates/s) and adds a database round trip to every redirect.
Instead, publish a click event to a queue and let a worker aggregate counts per code every few
seconds, then write batched updates. Task 06 is this problem on its own.

### 9.5 Validation and abuse

- Validate the URL syntactically and restrict schemes to `http`/`https`.
- Length cap (2–8 KB). Reject before touching the database.
- Blocklist known phishing/malware domains; this service *is* an open redirector by design, so
  it will be abused.
- Rate-limit creation per API key (task 03).
- Custom aliases: `[a-zA-Z0-9_-]{4,16}`, case-sensitive, reserved words rejected, `409` on conflict
  via the primary-key constraint, not a `SELECT` first (two requests could race).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the cache miss rate at 120k reads/s would push ~12k/s to the
  database. Add read replicas and shard Redis by code. Then the database at 37 TB: partition the
  `urls` table by a hash of `code`. Because every read is by `code`, the shard is always known.
- **Single points of failure:** the ID sequence lives on the primary. Postgres failover covers
  it; a KGS or task 04 removes the dependency.
- **When Redis is down:** reads fall through to the database. It survives at 12k/s with
  replicas; at peak it may not. A per-server in-process cache for the top ~10k codes buys time.
- **When the database is down:** reads still work for cached codes (the business keeps
  running), writes fail with `503`. That is the right failure order for this service.
- **Hot code:** one viral link at 20k/s. Redis handles ~100k ops/s on one key; the in-process
  cache removes even that. Hot keys are a read problem here, which is the easy kind.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| sequence + base62 | hashing the URL | no collisions, short codes | free dedup; codes are predictable unless scrambled |
| 302 | 301 | correct click counts, expiry works | more traffic (absorbed by cache) |
| cache-aside Redis | no cache | ~10× less database read load | a stale window ≤ TTL for expired links |
| async click counting | synchronous update | redirects never wait on a write | counts lag by seconds |
| Postgres | KV store | familiarity, transactions, `409` via constraint | must shard manually at tens of TB |

## 12. Check yourself

1. Why 302 and not 301 here? Name a requirement change that would flip the answer.
2. What goes wrong with hash-based codes, and how do you fix it without changing the key length?
3. In order, what becomes the bottleneck as traffic grows: ID generation, the database, or the cache?
4. A user shortens the same long URL twice. What happens, and what would have to change to return the same code?
5. Why should a custom alias conflict be caught by the database constraint rather than a `SELECT` first?

<details><summary>Answers</summary>

1. A `301` lets browsers cache the redirect, so repeat clicks never reach us and click counts are
   wrong. Drop the analytics requirement (and expiry) and `301` becomes the cheaper choice.
2. Seven characters of a hash collide at scale. Detect the collision with the unique constraint on
   insert and retry with a salt (e.g. append a counter to the URL before hashing). Same length,
   extra round trip only on collision.
3. Cache misses hit the database first (reads at 12k/s vs writes at 1.2k/s). The single ID
   sequence is nowhere near its limit at 1.2k/s. Redis is last; it handles ~100k ops/s per node.
4. Two rows, two codes. To dedup you'd need an index on `(user_id, long_url)` (large, since URLs
   are up to kilobytes; index a hash of the URL instead) and a lookup before insert.
5. Two concurrent requests for the same alias both pass the `SELECT`, then one `INSERT` fails
   anyway. The constraint is the only check that can't race; the `SELECT` is redundant work.

</details>

## 13. In Go

- Related lessons: [20 HTTP server](../../20-http-server.md) and [21 REST API](../../21-rest-api.md) for the two endpoints, [22 database/sql](../../22-database.md) for the insert and the unique-violation error, [38 Caching](../../38-caching-patterns.md) for cache-aside and `singleflight` on misses, [68 Rate limiting](../../68-rate-limiting-throttling.md) for creation limits.
- The redirect is one line: `http.Redirect(w, r, longURL, http.StatusFound)`.
- Base62 encoding is a ten-line function: repeated `n % 62` into a byte slice, reversed. Write it from memory; it comes up in interviews.
- Packages: `net/http`, `net/url` (validation), `pgx`, `go-redis`, `golang.org/x/sync/singleflight`.

## 14. Extensions

- Analytics: referrer, country, device, per-day series. Where does the event stream go and how is it aggregated?
- Expiring links cleanup job (see task 02's sweeper and task 15's scheduler).
- Dedup per user with a hashed-URL index.
- QR code generation and link previews (fetching the target page safely: see SSRF in task 10).
- Multi-region: read replicas per region, writes to one primary; what latency does a European click see?
- Replace the sequence with a KGS and compare operational complexity.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Redirect + create API (Go, stateless) | 50k redirects/s peak + 1.2k creates/s | CPU | redirects 50k × 0.15 ms = 7.5 vCPU; creates 1.2k × 0.5 ms = 0.6; total 8.1 vCPU. N = ceil(8.1 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(1.9) = 2 → 3 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 3, one per zone | a redirect is parse, one Redis GET, write headers: pure CPU. Memory is ~25 MiB of connections (50k/s × 50 ms ≈ 2.5k open × 10 KiB) plus a small in-process cache, so `c` not `m`. 34 % busy at peak, 51 % with one zone down |
| Redis (code → long URL, cache-aside) | ~55k ops/s (50k GET + ~5k fills on the 10 % misses); hot set 20M × 500 B = 10 GB | memory | 10 GB × ~1.5 overhead = 15 GB; `maxmemory` ≤ 70 % ⇒ ≥ 21 GB of RAM; 55k ops/s is about half of one core's ~100k | `r7g.xlarge` (4 vCPU / 32 GiB), ElastiCache | 1 primary + 1 replica (other zone) | memory-bound, so `r` (8 GiB per vCPU): `c` or `m` would buy 8–16 vCPUs just to get 32 GiB while Redis uses one core. A second shard only when ops or memory double |
| Postgres `urls` | ~5k miss reads/s (10 % of 50k) + 1.2k writes/s; year 1 = 36.5B rows × 100 B = 3.65 TB + ~35 % PK index (1.3 TB) ≈ 5 TB | IOPS, then memory | hot codes live in Redis, so RAM only needs the index's inner levels (~0.5 % of 1.3 TB ≈ 7 GB) plus recent rows. A miss ≈ 2 disk reads (leaf + row): 5k × 2 = 10k; writes 1.2k × 2 = 2.4k ⇒ ≈ 12.4k IOPS ⇒ `gp3` at 14,000 (cap 16,000). CPU ≈ 1.2 vCPU of 4 | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ, `gp3` 5 TB (alternative: DynamoDB, same key-value shape, section 8) | 1 primary + synchronous standby | random point reads over 5 TB with 32 GiB of RAM are IOPS-bound, not CPU-bound; `r` keeps the index's upper levels cached. `gp3` is enough below 16k IOPS. Year 10 (37 TB) means partitioning by `hash(code)`, as section 10 says |
| Click counter worker (+ queue) | 12k clicks/s avg, 50k/s peak, batched on the API nodes into ~3 messages/s | CPU (tiny) | sum a map, one batched UPDATE every few seconds: < 0.1 vCPU | `c7g.large` (2 vCPU / 4 GiB), SQS in between | 2, one per zone | availability, not load. Batching on the API nodes is a cost decision: one message per click would be 12k/s × 2.63M s ≈ 31.5B sends a month × $0.40 per million ≈ $12,600, more than the whole bill |

Network: a redirect answers with ~400 B (assumption), so 12k/s × 400 B = 4.8 MB/s ≈ 12.6 TB a month out. That is far below any node's limit but a real bill line. Redis sits in one zone, so ⅔ of the lookups cross zones (12k/s × ⅔ × ~260 B ≈ 5.5 TB a month).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.2xlarge` (3 × $212) | $640 |
| Redis: 2 × `cache.r7g.xlarge` (2 × $310) | $620 |
| Postgres: `db.r7g.xlarge` ($155 × 2 managed = $310), Multi-AZ doubles | $620 |
| `gp3` 5 TB + 11k extra IOPS ($400 + $55), × 2 for the standby | $910 |
| Counter workers: 2 × `c7g.large` (2 × $53) | $110 |
| ALB: $16 + ~50 LCU (assumed; new connections drive it) × $5.84 | $310 |
| Egress: 12.6 TB × $0.09 | $1,130 |
| Cross-zone Redis reads: ~5.5 TB × $0.02 (out + in) | $110 |
| **Total** | **≈ $4,450** (≈ $3,750 with a 1-year commitment on the ~$2,000 of compute lines) |

- **Biggest line and why:** egress (~25 %): every click gets a redirect over the internet, and `302` (not `301`, section 9.2) keeps every click coming back. The database with its disk is next (~34 %); the API servers are only ~14 %. Later the disk wins: year 10 is 37 TB (+ index ≈ 50 TB) ≈ 50,000 GB × $0.08 × 2 ≈ $8,000 a month for `gp3` alone.
- **What changes at 10×:** 500k redirects/s + 12k creates/s ⇒ 75 + 6 = 81 vCPU ⇒ N = ceil(81 ÷ 4.27) = 19 → 21 × `c7g.2xlarge`. Redis at ~550k ops/s and ~150 GB (100 GB × 1.5) needs cluster mode, ~8 shards of `r7g.xlarge` (≈ 69k ops/s and ≈ 19 GB each). Misses become 50k/s ⇒ ~125k IOPS (100k reads + 24k writes), far past one `gp3` volume, so shard `urls` by `hash(code)` into ~12 databases (the first real architecture change) or move to DynamoDB. Egress ≈ 126 TB ≈ $11k a month before volume tiers.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- One table and one access path: a point lookup by `code` (section 8). The database sees ~5k miss reads/s plus ~1.2k inserts/s (50k redirects/s at peak, 90 % answered by Redis; sections 4 and 15).
- 36.5B rows (~3.65 TB, ~5 TB with the index) in year 1 and ~37 TB after ten years at ~100 B a row. No joins, no range scans, no multi-row transactions.
- The only integrity rule is "one row per code", which the primary key enforces (a taken alias is a `409`, section 9.5). Rows never change except `clicks`, which a worker updates in batches (task 06).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the primary-key constraint gives the race-free `409` for custom aliases, a sequence feeds the base62 code (9.1), and one `db.r7g.xlarge` Multi-AZ carries 5k indexed reads/s and 1.2k writes/s (section 15). Nothing here needs a feature only Postgres has: it is the track default |
| MySQL (RDS / Aurora) | ✔ works, interchangeable | every feature used (primary-key uniqueness, one secondary index, single-row inserts) exists in InnoDB. The clustered primary key even suits a pure by-key table: a miss is one tree walk (~1 page read) where section 15 counts 2 (index leaf + heap row), so ~7.4k IOPS instead of ~12.4k at the same load. The price: a scrambled or custom-alias code is a random key, so page splits happen in the 5 TB table itself, not only in its 1.3 TB index |
| DynamoDB (or Cassandra / Keyspaces) | ✔ works, the strongest alternative | the table is already a key-value shape (section 8): a conditional put on "code absent" is the `409`, hashing spreads 5k reads/s and 1.2k writes/s far under the ~3,000 reads and ~1,000 writes per second per partition, and there are no shards to run at 37 TB. Cost: no ad-hoc queries, so "my links" (`user_id`) becomes a secondary index you design up front |
| Redis (ElastiCache) | ✖ as the record, ✔ as the derived cache | the 10 GB hot set is rebuildable from the table (9.3); 37 TB is not memory-sized and an evicting cache cannot be the only copy |

**Pick:** PostgreSQL on RDS Multi-AZ (the `db.r7g.xlarge` + `gp3` from section 15) as the system of record, with Redis as a derived cache in front. The deciding properties are modest and all portable: a primary-key constraint that settles alias races without a `SELECT` first, cheap indexed point reads, and a team that already runs it (section 8). `urls` is the record; every Redis entry can be rebuilt from it.

**If it were MySQL instead:** almost nothing. MySQL has no sequence object, so the code comes from an `AUTO_INCREMENT` column or the one-row counter of task 04's ticket server; `TIMESTAMPTZ` becomes a UTC `TIMESTAMP`; the duplicate-key error (1062) replaces Postgres's unique violation for the `409`. The batched `clicks` updates leave no dead rows to vacuum. Section 15's disk need shrinks by the read IOPS above.

**What would flip the choice:** the sharding trigger in section 15: ~10× load (≈ 125k IOPS, ~12 databases by `hash(code)`) or ~37-50 TB in year 10. At that point move the table to DynamoDB (same key-value shape, no shards to operate) or to Aurora, which grows storage by itself up to 128 TiB and offers up to 15 read replicas. Stay on one Postgres/MySQL primary while `gp3` at ≤ 16k IOPS covers the misses.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 5M redirects/s peak (1.2M/s average), 120k creates/s (10B links a day); ~365 TB of rows a year (~500 TB with the index); API 810 vCPU = 192 `c7g.2xlarge` | the database: 120k durable writes/s is 12× one primary's ~10k, and ~1.24M IOPS (500k misses + 120k writes, ~2 each) is 78× one `gp3`'s 16k. Redis is 5.5M ops/s = 55 cores' worth. Seven base62 characters (3.5T) last about a year at 10B links a day | as in section 16: `urls` in DynamoDB keyed by `code` (≥ 170 partitions: 500k reads/s ÷ 3,000) or ~120 Postgres shards by `hash(code)`; Redis cluster of ~100 shards plus an in-process cache for the top codes; 8-character codes (218T) from a KGS or task 04, since one sequence is gone too. Give up: "my links" by `user_id` needs its own secondary index; no cross-shard queries | ≈ $445k/month; egress (1.26 PB, ≈ $113k) is the biggest line |
| **1,000×** | 50M redirects/s peak, 1.2M creates/s (100B links a day, ~12 per person: a thought experiment); ~3.65 PB of rows a year; API 8,100 vCPU ≈ 1,900 nodes, ~1,000 Redis shards | running ~4,000 nodes: one fails about every hour (the sheet's 5,000 nodes ≈ hourly), resharding 100+ database shards is itself an incident, and one region's front door carries 50M × 400 B = 20 GB/s ≈ 160 Gbit/s | change the shape: ~100 independent cells, each the 100× design for 1/100 of codes; the first two characters of the code name the cell, so a redirect needs no directory lookup, and regions serve their nearest cells; viral links get a 5–10 s edge cache of the 302. Give up: global queries and global dedup (cells stream to one warehouse), exact live click counts, moving a code between cells | ≈ $4.5M/month |
| **10,000×** | 500M redirects/s peak (~1.6 Tbit/s of responses), 12M creates/s = 1T links a day (~125 per person on Earth); 365T links a year | money and the planet, not a component: ≈ $44.5M a month, egress alone 126 PB ≈ $11M, and 365T new codes a year overflow the 8-character space (218T) in about 7 months | change the problem: the edge serves redirects (CDN or edge KV caches the immutable `code → URL` for a short TTL, so the origin sees misses and sampled click beacons); creation gets per-account quotas and a default expiry (say 90 days unclicked) so codes are recycled, not minted forever. Give up: exact click counts (sampled), instant edits and deletes (bounded by the edge TTL), "never expires" as the default | ≈ $44.5M/month if nothing changes; far less once the edge answers most redirects |

**Assumed beyond section 4:** ~400 B per redirect response and ~500 B of cache per hot code (section 15); one Redis shard ≈ 100k ops/s at ~55 % busy; the 90 % cache hit rate holds as traffic grows.

**What stays the same at every scale:** a short code is one key in a key-value shape, so every rung only changes where that key lives (cell, shard, edge); `302` plus asynchronous click counting keeps redirects off the write path, and the primary-key constraint (or a conditional put) keeps codes unique.

**Say it in an interview:** "At 100× one Postgres is 12× over its write ceiling and 78× over a disk's IOPS, so I key a store by code (DynamoDB or ~120 shards), cluster Redis, and mint 8-character codes from a generator instead of one sequence. At 1,000× I would be running ~4,000 nodes that fail hourly, so I split into ~100 cells and let the first characters of the code name the cell. At 10,000× there are more links a year than 8 characters hold, so I push redirects to the edge, sample clicks, and cap and expire links."
