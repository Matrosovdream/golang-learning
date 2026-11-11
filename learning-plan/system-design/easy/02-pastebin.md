# 02 · Pastebin

`🟢 easy` · *Topics:* metadata vs blob storage, object storage, expiry & cleanup, unguessable keys, serving text safely · *Builds on:* [01 URL Shortener](01-url-shortener.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [01 URL Shortener](01-url-shortener.md) · Next: [03 Rate Limiter](03-rate-limiter.md)

---

## 1. The task

> "Design Pastebin. Users paste a block of text and get a URL. Anyone with the URL can read the
> text. Pastes can expire."

Structurally this is task 01 with one difference that changes the storage design: the *value*
is no longer a 100-byte URL but up to megabytes of text.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| How big can a paste be? Average size? | Decides whether content lives in the database or in object storage. | Max 10 MB; average 10 KB. |
| How many pastes per day, how many reads? | Sizing. | 1M new pastes/day; reads 5× writes. |
| Expiry options? Default? | Needs a cleanup design. | Never, 10 min, 1 h, 1 day, 1 week, 1 month. Default never. |
| Public, unlisted, private? | Unlisted = unguessable URL, which rules out sequential IDs. | Public (listed) and unlisted. No accounts needed for unlisted. |
| Editable? Deletable? | Immutability enables aggressive caching. | Not editable. Deletable by the creator (via a secret token). |
| Syntax highlighting, titles, languages? | Metadata only; highlighting is a client concern. | Title + language tag stored; rendering is client-side. |
| Burn after reading? Password protection? | Both are interesting but change the read path. | Out of scope for now (extensions). |
| Latency, availability? | Same shape as 01. | Reads fast and highly available. |

## 3. Requirements

**Functional**
- Create a paste (text up to 10 MB, optional title, language, expiry) and get a URL.
- Read a paste, rendered and as raw text.
- Pastes expire and become unreachable; expired content is eventually deleted.
- Creator can delete a paste.

**Non-functional**
- Unlisted paste URLs are not guessable.
- Reads are fast; a paste is immutable so it can be cached hard.
- Durable: a paste that has not expired is never lost.

**Out of scope**
- Accounts, search, comments, editing, burn-after-read, passwords.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Writes | ~12/s | 1M/day ÷ 86,400 |
| Reads | ~60/s, peaks ~300/s | 5× writes |
| Content ingest | ~10 GB/day | 1M × 10 KB |
| Content in 10 years | ~36 TB (less with expiry) | 10 GB × 3,650 |
| Metadata in 10 years | ~1.5 TB | 3.65B rows × ~400 B |
| Peak bandwidth out | ~3 MB/s typical, 10 MB pastes can spike to 100s of MB/s | 300/s × 10 KB; a few large reads dominate |

What the numbers tell us: request rates are tiny. Any single database handles them. The design
is driven entirely by **size**: tens of terabytes of immutable blobs, with occasional 10 MB
reads that must not be loaded into a database connection or an application server's memory
twice.

## 5. Plan

1. API first; it is nearly identical to task 01.
2. Decide where the content lives (database vs object storage), the core decision.
3. Decide the key scheme: unlisted means random, not sequential.
4. Draw the design with metadata and blob paths separated.
5. Deep-dive expiry: how a paste stops being readable, and how its bytes actually get deleted.
6. Serving text safely and caching immutable content.

## 6. High-level design

```
 write                                                    read
 client ──POST /api/v1/pastes──► LB ──► API ──► Postgres (metadata: id, size, expires_at, …)
                                        │
                                        └──► Object storage (S3): key = pastes/<id>   (the bytes)

 client ──GET /{id}──► CDN ──miss──► LB ──► API ──► Postgres (metadata, expiry check)
                                              └──► S3 GET (stream through, or redirect to a signed URL)
                                       cache: Redis (metadata), CDN (content, immutable)

 sweeper (cron, task 15) ──► SELECT … WHERE expires_at < now() ──► DELETE S3 object, DELETE row
```

**Write:** validate size and content type, generate a random ID, upload the bytes to object
storage under `pastes/<id>`, then insert the metadata row. Order matters: blob first, row
second, so a row never points at a missing blob (an orphan blob is harmless and the sweeper can
find it).

**Read:** fetch metadata (cache-aside in Redis), check `expires_at`, then stream the bytes from
object storage to the client. For raw reads, consider redirecting to a short-lived presigned URL
so the bytes never pass through the API servers at all.

## 7. API

```
POST /api/v1/pastes
  body:   { "content": "…", "title"?: "…", "language"?: "go", "expires_in"?: "1h", "visibility": "unlisted"|"public" }
          (or multipart/octet-stream for large pastes, streamed)
  201:    { "id": "k8Qz3fWp", "url": "https://paste.io/k8Qz3fWp", "raw_url": ".../k8Qz3fWp/raw",
            "delete_token": "…", "expires_at": … }
  413:    too large

GET /{id}            200 rendered page · 404 unknown · 410 expired
GET /{id}/raw        200 text/plain; charset=utf-8 · same errors
GET /api/v1/pastes/{id}        metadata JSON
DELETE /api/v1/pastes/{id}     header: X-Delete-Token → 204
GET /api/v1/pastes?visibility=public&cursor=…   recent public pastes (cursor pagination, lesson 41)
```

## 8. Data model

```sql
CREATE TABLE pastes (
    id           VARCHAR(12) PRIMARY KEY,   -- random base62, 8 chars
    title        TEXT,
    language     VARCHAR(32),
    visibility   SMALLINT    NOT NULL,      -- 0 public, 1 unlisted
    size_bytes   INTEGER     NOT NULL,
    content_key  TEXT        NOT NULL,      -- S3 object key, e.g. pastes/k8Qz3fWp
    content_sha  BYTEA       NOT NULL,      -- integrity + future dedup
    delete_token_hash BYTEA  NOT NULL,      -- never store the token itself
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,               -- NULL = never
    views        BIGINT      NOT NULL DEFAULT 0
);
CREATE INDEX pastes_expires_at_idx ON pastes (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX pastes_public_recent_idx ON pastes (created_at DESC) WHERE visibility = 0;
```

The partial index on `expires_at` is what makes the sweeper cheap: it scans only rows that can
expire, in order, and stops at `now()`.

## 9. Deep dives

### 9.1 Where do the bytes live?

| Option | Pros | Cons |
|---|---|---|
| **Everything in Postgres** (`content TEXT`) | one system, transactional create, simplest code | 36 TB in one database; 10 MB rows bloat the heap and WAL, replication carries every byte, backups balloon, every read holds a connection while streaming |
| **Object storage for content, Postgres for metadata** | S3 is built for exactly this: cheap, durable, unlimited, serves large objects directly, CDN-friendly | two systems to keep consistent (blob first, then row); a read is two hops |
| **Hybrid:** inline in Postgres below ~64 KB, S3 above | small pastes (the vast majority) are one read; large ones don't hurt the database | two code paths; the threshold is one more thing to tune |

**Choice: object storage + metadata database.** Average size is 10 KB, so the hybrid would
put most pastes inline and is a reasonable optimisation later. The pure-database option fails
on operations (backups, replication, bloat) long before it fails on correctness.

### 9.2 Key generation: random, not sequential

Task 01 used a sequence. Here, an *unlisted* paste is "private by obscurity": whoever has the
URL can read it, so the URL must be unguessable. A sequential ID means anyone can walk
`/1`, `/2`, `/3` and read every paste ever made.

- Generate 8 random base62 characters from a cryptographic source (`crypto/rand`). 62⁸ ≈ 2.2 ×
  10¹⁴ keys; with 3.65B pastes in 10 years the chance of a random collision per insert is
  ~1.7 × 10⁻⁵. Low, but not zero.
- Rely on the primary-key constraint: on a unique violation, generate another key and retry.
  Never `SELECT` first.
- Public pastes could use shorter or sequential IDs, but one scheme is simpler. Random for all.

Random keys also spread writes evenly across any future partitioning of the table and across
S3 prefixes. Sequential keys would make a hot partition.

### 9.3 Expiry: unreadable now, deleted eventually

Two different problems with two different mechanisms:

1. **Stop serving it the moment it expires.** Check `expires_at` on every read, in the
   application, and return `410`. Never trust a cache for this: cache metadata with
   `TTL = min(1 day, expires_at − now)` so an entry never outlives the paste. For the CDN, set
   `Cache-Control: max-age` the same way.
2. **Delete the bytes eventually.** A background **sweeper** (task 15 has the scheduler) runs
   every minute: `SELECT id, content_key FROM pastes WHERE expires_at < now() LIMIT 1000`,
   deletes the S3 object, then the row. Idempotent: if it crashes halfway, the next run finds the
   same rows. Delete the blob *before* the row so a surviving row still points at something we
   can retry.

| Approach | Pros | Cons |
|---|---|---|
| Lazy only (delete on first read after expiry) | zero background work | never-read pastes never get deleted; storage leaks |
| Sweeper only | reliable deletion | between expiry and the next sweep the paste is still readable unless the read path checks too |
| **Both** | correct and cheap | two code paths, both simple |
| S3 lifecycle rules | S3 deletes objects for you | rules are per prefix/tag with day granularity, not per object at a minute; still useful as a safety net for orphans |

### 9.4 Serving text safely and cheaply

- Raw endpoint: `Content-Type: text/plain; charset=utf-8` and `X-Content-Type-Options: nosniff`.
  Without `nosniff` a browser may decide a paste containing `<script>` is HTML and execute it on
  our origin. That is a stored XSS.
- Rendered endpoint: HTML-escape everything. Highlighting happens in the browser.
- Immutable content → `Cache-Control: public, max-age=…, immutable` on the raw endpoint, bounded
  by expiry. The CDN then absorbs the viral paste.
- Large pastes: stream (`io.Copy`) from object storage to the response; never read 10 MB into a
  `[]byte` per request. Or redirect to a presigned S3 URL and let S3 serve the bytes.
- Compression: text compresses 3–10×. Store gzip'd in S3, serve `Content-Encoding: gzip` to
  clients that accept it, decompress for the rest. Cuts storage and egress cost by most of 36 TB.

### 9.5 Create: order of operations and limits

- Enforce the 10 MB limit **before** reading the body fully (`http.MaxBytesReader`), not after.
- Upload blob, then insert row. If the insert fails, the blob is an orphan: harmless, found later
  by a reconciliation job that lists S3 keys without rows (or by an S3 lifecycle rule on a
  `tmp/` prefix that the create path moves out of on success).
- The `delete_token` is random, returned once, stored hashed. Same reasoning as passwords.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** nothing in the request path (120 writes/s, 600 reads/s). Storage
  grows to 360 TB, which object storage absorbs; the metadata table at 15 TB would need
  partitioning by `id` hash.
- **Single points of failure:** the metadata database. Reads survive on cached metadata plus
  S3; writes fail. Same acceptable failure order as task 01.
- **Object storage slow or down:** reads fail even if metadata is fine. The CDN serves whatever it
  has cached. This is why the CDN is not optional for a popular paste.
- **Sweeper falls behind:** pastes remain unreadable (the read check covers correctness), only
  storage cost grows. Add workers; the `expires_at` index keeps each batch cheap.
- **Abuse:** rate-limit creation per IP (task 03), scan for malware/credential dumps asynchronously,
  take-down by delete token or admin.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| S3 for content | `TEXT` column in Postgres | cheap, durable, CDN-ready, database stays small | two writes to keep consistent; two hops on read |
| random 8-char IDs | sequence + base62 | unguessable unlisted pastes, even write distribution | tiny collision probability handled by retry; longer than needed for public pastes |
| read-path check + sweeper | sweeper alone | expired pastes are never served | two mechanisms |
| blob first, row second | row first | a row never points at nothing | orphan blobs need a reconciliation job |
| gzip at rest | raw bytes | 3–10× less storage and egress | CPU on write, and on read for non-gzip clients |

## 12. Check yourself

1. Why split metadata and content into two stores when the request rate is only 12 writes/s?
2. Why are sequential IDs acceptable in task 01 but not here?
3. A paste expires at 12:00:00. The sweeper runs at 12:00:59. What does a reader see at 12:00:30, and what makes that happen?
4. What is the attack if the raw endpoint lacks `X-Content-Type-Options: nosniff`?
5. Blob first or row first on create, and what happens on a crash in between for each order?

<details><summary>Answers</summary>

1. Because of size, not rate: 36 TB of immutable 10 KB–10 MB blobs inflates Postgres heap, WAL,
   replication and backups, and every read holds a connection while streaming. S3 is built for
   this; Postgres stays at ~1.5 TB of metadata.
2. Unlisted pastes are private only because their URL is unguessable. Sequential IDs let anyone
   enumerate `/1`, `/2`, … and read everything. Task 01 only redirects to URLs the creator chose to make public.
3. `410 Gone`. The read path compares `expires_at` with `now()` on every request, and the cache
   TTL for the metadata was capped at `expires_at − now`, so no stale copy outlives the paste.
   The sweeper only reclaims storage.
4. Stored XSS: a browser sniffs a paste starting with `<html>` or `<script>` as HTML and executes it
   on our origin, with our cookies.
5. Blob first. Crash after the blob upload leaves an orphan object nobody can reach; a
   reconciliation job deletes it. Row first would leave a reachable row whose content is missing,
   a visible error to users.

</details>

## 13. In Go

- Related lessons: [19 stdlib tour](../../19-stdlib-tour.md) (`io.Copy`, streaming), [21 REST API](../../21-rest-api.md) (`http.MaxBytesReader`, JSON vs multipart), [57 Web security](../../57-web-security.md) (`nosniff`, escaping), [41 API design](../../41-api-design-evolution.md) (cursor pagination for public pastes), [44 Background jobs](../../44-background-jobs-queues.md) (the sweeper).
- Random key: `crypto/rand` into a byte slice, map each byte modulo 62 (or use `rand.Int` to avoid modulo bias; the bias here is negligible but interviewers may ask).
- Packages: `aws-sdk-go-v2/service/s3` (`PutObject`, `GetObject`, presign), `compress/gzip`, `html/template` for rendering.

## 14. Extensions

- **Burn after reading:** the read must be an atomic "fetch and mark deleted". Two readers racing: who wins? (Hint: `UPDATE … SET burned = true WHERE id = $1 AND burned = false RETURNING …`.)
- **Password-protected pastes:** encrypt content with a key derived from the password (Argon2) so the server can't read it either.
- **Dedup by `content_sha`:** reference-count blobs; what does delete mean now?
- **Edit history / versions:** immutability breaks; what changes in caching?
- **Public paste search** (medium: inverted index, Elasticsearch, indexing pipeline).
- **Hybrid inline storage** for pastes under 64 KB; measure the read latency difference.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API (Go, stateless) | 300 reads/s peak + 12 writes/s; bodies up to 10 MB | CPU (tiny) | reads 300 × 0.5 ms = 0.15 vCPU; writes 12 × 2 ms (gzip + SHA-256 of ~10 KB; the S3 PUT is waiting, not CPU) = 0.02; total 0.17 vCPU. N = ceil(0.17 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | for availability, not load: one node could carry ~6× the peak. Stream with `io.Copy` (32 KiB buffers) or redirect large reads to a presigned S3 URL, so memory stays far below 4 GiB and the node never carries a 10 MB read; `c` since nothing needs RAM |
| Postgres (metadata) | 12 writes/s + ~300 lookups/s (mostly cached); year 1 = 365M rows × 400 B = 150 GB + ~35 % indexes ≈ 200 GB | memory (the random-key index); IOPS is not the bound | writes 12 × ~2 IOPS = 24; even 300 uncached reads/s ≈ 300–600 IOPS, under the 3,000 `gp3` includes. The random PK index is ~15 GB in year 1 (365M × ~40 B, assumption) and ~150 GB in year 10; CPU < 0.1 vCPU | `db.r7g.large` (2 vCPU / 16 GiB), RDS Multi-AZ, `gp3` 200 GB | 1 primary + synchronous standby | nothing is busy; the only thing that grows is the random-key index, and RAM keeps lookups on it off disk, so the smallest `r`. `db.m7g.large` would also work today; go up one size per ~2× of index |
| Redis (metadata, cache-aside) | ~60 reads/s avg, 300/s peak; hot set = pastes read in a day ≈ 5.2M × 400 B ≈ 2.1 GB | memory (small) | 2.1 GB × ~1.5 overhead = 3.1 GB; `maxmemory` ≤ 70 % ⇒ ≥ 4.5 GB of RAM; 300 ops/s is nothing for one core | `cache.m7g.large` (2 vCPU / 8 GiB), ElastiCache | 1 primary + 1 replica | optional at this rate: Postgres alone serves 300 reads/s, so the cache buys latency and shelter from CDN-miss spikes. Smallest node with enough RAM; an `r` would leave its memory unused |

S3 and CloudFront: nothing to size. Year 1 holds 3.65 TB raw (10 GB × 365) ≈ 1.2 TB gzip'd at 3× (low end of section 9.4's 3–10×); 12 PUT/s and 300 GET/s are far under ~3,500 PUT/s and ~5,500 GET/s per prefix (random keys spread them). Reads: 60/s × 10 KB = 0.6 MB/s ≈ 1.6 TB a month raw, ≈ 0.53 TB gzip'd to clients. The 100s-of-MB/s spikes from 10 MB pastes are a CDN or presigned-URL job, not an API-node job.

S3 request fees are not among the cheat sheet's anchors and are left out of the total: ~31.5M PUTs a month at ~$0.005 per 1,000 (from memory, verify) is ≈ $160, more than the storage line.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.large` ($78 × 2 managed = $156), Multi-AZ doubles | $310 |
| `gp3` 200 GB ($16), × 2 for the standby | $30 |
| Redis: 2 × `cache.m7g.large` (2 × $120) | $240 |
| S3: 1.2 TB × $0.023 | $30 |
| CloudFront egress: ~0.53 TB × $0.085 | $40 |
| ALB | $20 |
| **Total** | **≈ $830** (≈ $580 with a 1-year commitment on the ~$710 of compute lines; S3 request fees extra) |

- **Biggest line and why:** Postgres and its standby (~37 %; ~70 % together with Redis and the disk). The whole application is 0.17 vCPU, so the bill is the price of keeping two small stateful systems alive in two zones. The 36 TB that drives the design is cheap: year 1 costs ~$30 a month to store gzip'd, year 10 ≈ $280 (gzip'd) or $840 raw. Cheapest lever: drop Redis at this rate (−$240).
- **What changes at 10×:** 3,000 reads/s + 120 writes/s ⇒ 3,000 × 0.5 ms + 120 × 2 ms = 1.74 vCPU ⇒ N = ceil(1.74 ÷ 1.07) = 2 → 3, still 3 × `c7g.large`. The first thing to run out is the random-key index: ~146 GB in year 1 (3.65B × 40 B) ⇒ `db.r7g.4xlarge` (16 vCPU / 128 GiB) or partition by `hash(id)` as section 10 says. Storage ≈ 12 TB gzip'd a year ≈ $280 a month; egress ≈ 5 TB gzip'd ≈ $450.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Metadata only: ~12 writes/s and ~300 lookups/s at peak, mostly cached (section 4). 365M rows (~150 GB, ~200 GB with indexes) in year 1, ~1.5 TB after ten years at ~400 B a row.
- Three access paths: a point lookup by a random 8-char `id`; the sweeper's range `expires_at < now()`; and "recent public pastes" (`created_at DESC`, public only). The last two are exactly the partial indexes of section 8.
- The bytes (10 KB average, up to 10 MB, ~36 TB in ten years) are not database data at all (see 9.1): S3 holds them, one object per paste.
- No multi-row transaction: "blob first, row second" is an ordering in code (9.5), not a database guarantee.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for the metadata, ✖ for the bytes | `CREATE INDEX … WHERE expires_at IS NOT NULL` and `… WHERE visibility = 0` index only the rows each query can use, so the sweeper's scan and the public list stay small next to 365M+ rows; `UPDATE … RETURNING` is the atomic burn-after-reading of section 14. As a blob store it is the option 9.1 rejects (36 TB of heap, WAL and backups) |
| MySQL (RDS / Aurora) | ✔ works, with a workaround | no partial indexes: a plain index on `expires_at` also stores the never-expiring rows (the default is "never"), and `(visibility, created_at)` also stores every unlisted paste, so both indexes are bigger and every insert maintains them; the queries return the same rows. Burn-after-reading becomes `UPDATE … WHERE burned = 0` plus a rows-affected check (no `RETURNING`). The random key lands in a clustered primary key, which at 12 inserts/s is irrelevant |
| S3 | ✅ chosen as the system of record for the bytes | immutable objects, cheap and durable at 36 TB, CDN-friendly; 12 PUT/s and 300 GET/s are far under ~3,500 PUT/s and ~5,500 GET/s per prefix, and random keys spread the prefixes (section 15) |
| DynamoDB | ✔ works, earns nothing here | a ~400 B item fetched by key fits easily (the item limit is 400 KB), but 12 writes/s is nowhere near the load that justifies it, and the sweeper range and the public list would each be a secondary index designed up front |
| Redis (ElastiCache) | ✔ as a derived cache | metadata cached with `TTL = min(1 day, expires_at − now)` (9.3); optional at 300 reads/s, so section 15 treats it as latency insurance, not a requirement |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.large`, section 15) for the metadata plus S3 for the bytes: two systems of record with different jobs. Redis and CloudFront are derived and rebuildable from them. The deciding features are the two partial indexes and the `RETURNING` for burn-after-reading; at 12 writes/s speed does not separate the engines, so this is a size-and-neatness win more than a necessity.

**If it were MySQL instead:** the same table with `VARBINARY` for the hashes, plus full indexes on `expires_at` and on `(visibility, created_at)`. They grow with every row (up to ~3.65B in year 10) rather than only the expiring or public ones, and the sweeper and the public list return the same rows. Burn-after-reading is the rows-affected idiom above.

**What would flip the choice:** 10× load (section 15): the random-key index reaches ~146 GB, so first a bigger instance (`db.r7g.4xlarge`), then partitioning by `hash(id)` (section 10); DynamoDB only if writes exceeded one primary's ~10k/s (they are 12/s) or you wanted no database servers at all. Searching public pastes (section 14) means a derived OpenSearch index, never the record. The bytes never move into a database.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 1,200 writes/s, 30k reads/s peak; 100M pastes a day = 1 TB/day raw, 365 TB a year (~120 TB gzip'd); metadata 36.5B rows ≈ 15 TB a year; API ~17 vCPU = 18 `c7g.large` | the metadata index, not the request path: the random-key index is ~1.5 TB (36.5B × 40 B), 12× the 128 GiB of a `db.r7g.4xlarge`, and the 15 TB table is past "a few TB comfortably", so lookups go to disk and one `gp3` (16k IOPS) is the wall. 1,200 writes/s is only 12 % of a primary | as in section 10 and 16: shard metadata by `hash(id)` into ~16 Postgres (~1 TB table and ~90 GB index each, so the index fits RAM); Redis cluster of ~14 shards (210 GB hot set × 1.5 ÷ 22 GB); CloudFront in front of every read. Give up: "recent public pastes" no longer comes from one partial index: it needs a derived list (a Redis list or a merge of 16 shards) | ≈ $83k/month, plus ≈ $16k of S3 PUT fees (3.15B a month), more than storage (≈ $3k) |
| **1,000×** | 12k writes/s, 300k reads/s peak; 1B pastes a day = 10 TB/day raw, 3.65 PB a year (~1.2 PB gzip'd); 365B metadata rows ≈ 150 TB; API ~165 `c7g.large` | one S3 object per paste: 12k PUT/s ≈ 31.5B PUTs a month ≈ $158k in request fees, 5.7× the ≈ $28k of storage, and ~160 Postgres shards (+ standbys) would be a resharding incident each year | change the shape: ~16 cells (the first character of the id names the cell, no directory lookup), each the 100× design; inside a cell, pack pastes under 64 KB into ~64 MB append-only segment objects with `(segment, offset, length)` in the metadata, cutting PUTs ~1,000× to ~12/s; metadata in DynamoDB (TTL attribute does expiry) instead of 160 shards. Give up: per-paste S3 delete (deleting means compacting segments), the partial-index queries, and simplicity | ≈ $0.83M/month, plus ≈ $0.16M of PUT fees before packing |
| **10,000×** | 120k writes/s, 3M reads/s peak; 10B pastes a day (more than one per person) = 100 TB/day raw, 36.5 PB a year, 365 PB over ten years (~120 PB gzip'd) | the premise "keep every paste forever": ten years of gzip'd storage alone is ≈ $2.8M a month and growing every month, for data most people never read twice; egress (5.3 PB a month ≈ $450k) and ≈ $8.3M a month in total follow | change the problem: ration and shrink: default expiry of 30 days instead of "never", a 1 MB cap instead of 10 MB, a daily byte quota per account and IP, dedup by `content_sha` with a reference count (identical logs and snippets share one blob), cold pastes moved to a cheaper storage tier, reads served only through the CDN. Give up: "never expires" and 10 MB pastes, simple delete (a delete drops a reference), some write latency for hashing | ≈ $8.3M/month if nothing changes; the default expiry caps storage at 30 days of data |

**Assumed beyond section 4:** the sizes and CPU costs of section 15 (400 B of metadata per paste, 3× gzip, ~40 B per random index entry, 0.5 ms per read and 2 ms per write); S3 PUT fees at ≈ $0.005 per 1,000 from section 15, which flags that figure as from memory.

**What stays the same at every scale:** the split between metadata and bytes, with a random unguessable id as the only key and "blob first, row second"; and immutability, which lets the CDN absorb reads and lets the read-path expiry check stay correct at any size.

**Say it in an interview:** "At 100× the request rate is still modest, but the random-key index is 1.5 TB against 128 GiB of RAM, so I shard metadata by id hash and put a CDN in front; the bytes stay in S3. At 1,000× one object per paste costs more in PUT fees than in storage, so I split into cells named by the id's first character and pack small pastes into big segment objects. At 10,000× keeping everything forever is the problem, so I add default expiry, size and byte quotas and dedup by content hash."
