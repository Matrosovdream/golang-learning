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
