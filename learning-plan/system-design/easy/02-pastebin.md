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
