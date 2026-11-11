# 11 · Image Upload & Hosting

`🟢 easy` · *Topics:* presigned direct-to-storage uploads, validating by content not extension, async processing pipelines, immutable content-addressed URLs on a CDN, dedup, signed URLs for private files · *Builds on:* [02 Pastebin](02-pastebin.md) (object storage), [10 Webhook Delivery](10-webhook-delivery.md) (async jobs)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [10 Webhook Delivery](10-webhook-delivery.md) · Next: [12 Notification Service](12-notification-service.md)

---

## 1. The task

> "Design an image hosting service like Imgur. Users upload images and get a link; the images
> load fast anywhere in the world; we show thumbnails in galleries."

Pastebin with bytes that are 100× bigger and a *processing* step. The upload path, not the
storage, is where a naive design falls over.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Max and typical size? Formats? | Memory per upload, processing cost. | Max 20 MB; typical 2 MB; JPEG/PNG/WebP/GIF. |
| Uploads per day? Views per day? | Ingest vs egress; CDN need. | 1M uploads/day; 100M views/day. |
| Variants needed? | Thumbnails, resized versions, format conversion (WebP). | Thumbnail (300 px), medium (1024 px), original. |
| Public or private? | Private needs access control on a CDN. | Both; public by default. |
| Can images be deleted or replaced? | Immutability enables permanent caching. | Deleted yes; replaced no (upload a new one). |
| Strip metadata? | EXIF contains GPS coordinates. | Yes, strip EXIF on all served variants. |
| Dedup identical uploads? | Storage savings vs complexity. | Nice to have. |
| Abuse scanning? | Illegal content, malware disguised as images. | Hook for an async scanner; out of scope otherwise. |
| Latency targets? | Upload feedback, first view. | Upload acknowledged immediately; thumbnails ready within ~5 s; views from CDN edge. |

## 3. Requirements

**Functional**
- Upload an image (up to 20 MB), get an ID and URLs for original, medium, thumbnail.
- Serve images fast globally; thumbnails for galleries.
- Private images viewable only via authorised, expiring links.
- Delete an image.

**Non-functional**
- Upload bytes never pass through our API servers.
- Served variants are stripped of EXIF; files are validated as real images.
- Variants appear within seconds; the UI handles "processing".
- Cache-friendly: once published, a URL's content never changes.

**Out of scope**
- Video, editing, albums, comments, NSFW classification.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Uploads | ~12/s, peaks ~60/s | 1M/day |
| Ingest | ~2 TB/day, ~730 TB/year | 1M × 2 MB |
| Views | ~1,160/s, peaks ~10k/s | 100M/day |
| Egress without CDN | 100M × ~200 KB (medium) = 20 TB/day ≈ 2 Gbit/s average | CDN is not optional |
| Processing | 1M images × ~200 ms CPU = ~55 CPU-hours/day ≈ 3 cores busy | cheap, but bursty |
| Metadata rows | 1M/day × ~300 B = 300 MB/day | trivial |

What the numbers tell us: storage grows by ~0.7 PB a year (object storage, fine); egress is the
dominant cost and must be served by a CDN; processing is small but must be **off the request
path** so bursts don't block uploads.

## 5. Plan

1. The upload path: get the bytes to object storage without touching our servers.
2. Validation and the asynchronous processing pipeline.
3. Serving: URLs, caching, CDN, variants.
4. Private images.
5. Dedup and deletion.
6. Failure modes and cost.

## 6. High-level design

```
 1. client ──POST /uploads {size, type}──► API ──► Postgres: images(id, status=pending)
    client ◄── { upload_url (presigned PUT, 15 min), image_id } ──┘

 2. client ──PUT bytes──────────────────────────────────────────► S3  uploads/{image_id}
                                                                   │ event notification (or client calls /complete)
 3.                                                                ▼
    queue ──► processing workers: download, sniff magic bytes, decode, strip EXIF,
              generate variants (thumb 300, medium 1024, original re-encoded),
              write images/{sha}/{variant}.{ext}, update row: status=ready, dims, sha
              failure → status=rejected, reason

 4. viewers ──GET cdn.example/i/{sha}/t.webp──► CDN ──miss──► S3 (origin)
    private: ──GET /i/{sha}/t.webp?Expires&Signature──► CDN verifies signed URL
```

The API hands out a short-lived permission to write one object; the browser uploads straight to
storage. A worker validates and produces variants into immutable, content-addressed paths that
the CDN caches forever.

## 7. API

```
POST /uploads
  { "filename": "cat.jpg", "content_type": "image/jpeg", "size": 2048000, "visibility": "public" }
  → 201 { "image_id": "img_01J9…", "upload_url": "https://s3…/uploads/img_01J9…?X-Amz-Signature=…",
          "expires_in": 900, "max_size": 20971520 }

PUT {upload_url}   (client → S3 directly; S3 enforces content-length-range and content-type from the presign)

POST /uploads/{image_id}/complete        → 202 { "status": "processing" }   (optional if S3 events are used)
GET  /images/{image_id}                  → 200 { "status": "ready", "width": 4000, "height": 3000,
                                                 "urls": { "thumb": "…/i/{sha}/t.webp", "medium": "…/m.webp", "original": "…/o.jpg" } }
DELETE /images/{image_id}                → 204
```

Served URL shape: `/i/{content_sha256_prefix}/{variant}.{ext}`, with
`Cache-Control: public, max-age=31536000, immutable`.

## 8. Data model

```sql
CREATE TABLE images (
    id            TEXT PRIMARY KEY,                -- img_…
    owner_id      BIGINT NOT NULL,
    status        TEXT NOT NULL,                   -- pending | processing | ready | rejected | deleted
    visibility    TEXT NOT NULL,                   -- public | private
    upload_key    TEXT NOT NULL,                   -- uploads/img_…
    blob_id       TEXT,                            -- FK → blobs.sha256 once processed
    width         INTEGER, height INTEGER,
    content_type  TEXT,
    reject_reason TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX images_owner_idx ON images (owner_id, created_at DESC);

CREATE TABLE blobs (                               -- content-addressed, shared across identical uploads
    sha256        TEXT PRIMARY KEY,
    size_bytes    BIGINT NOT NULL,
    variants      JSONB NOT NULL,                  -- { "t": "images/<sha>/t.webp", "m": …, "o": … }
    ref_count     INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Object storage layout: `uploads/{image_id}` (raw, lifecycle-deleted after 1 day), `images/{sha}/{t,m,o}.{ext}` (served, immutable).

## 9. Deep dives

### 9.1 Why presigned uploads

| Path | Pros | Cons |
|---|---|---|
| **Proxy through the API** (`POST /images` multipart) | one request; server sees bytes immediately | every upload holds an API server connection for seconds and streams 2–20 MB through it; at peaks the API tier is sized by upload bandwidth, not by API logic; a slow mobile client ties up a server |
| **Presigned URL, direct to object storage** | API issues a credential (~1 ms), storage handles the bytes; scales with S3, not with us; resumable/multipart uploads come free | two steps for the client; validation must happen *after* upload (we never saw the bytes); needs CORS on the bucket |

**Choice: presigned.** Constrain the presign: exact key, max size (`content-length-range`),
allowed content type, 15-minute expiry. The client can lie about the type; we verify in processing.

### 9.2 Validate by content, never by name

An `.jpg` extension or `Content-Type: image/jpeg` is a claim, not a fact. The worker reads the
first bytes (**magic numbers**: `FF D8 FF` JPEG, `89 50 4E 47` PNG, `52 49 46 46 … 57 45 42 50`
WebP, `47 49 46 38` GIF) and then fully **decodes** the image. Decoding both validates the file and
makes the output safe: a polyglot file (valid JPEG that is also valid HTML or JS) is destroyed by
re-encoding. Set limits before decoding: dimensions (a 1-pixel-wide, 100,000-pixel-tall PNG, or a
"decompression bomb", can allocate gigabytes), and a decode timeout.

Re-encoding also **strips EXIF** (GPS, device, timestamps) from every served variant; the stored
original keeps it only if the product needs it (default: strip there too).

### 9.3 The processing pipeline

- Trigger: an S3 event notification on `uploads/` (or the client's `/complete` call) enqueues
  `process(image_id)`. Both can fire; the worker is idempotent: if the row is already `ready`, exit.
- Steps: download → sniff → decode with limits → re-encode original (`o`) → resize to medium
  (`m`) and thumbnail (`t`), WebP for both → compute `sha256` of the decoded, normalised original →
  upload variants to `images/{sha}/…` → upsert `blobs`, set `images.blob_id`, `status = ready`.
- Failure: a decode error marks `rejected` with a reason the UI can show; a transient error
  (S3 timeout) retries with backoff (task 10's playbook); repeated failures dead-letter.
- Status: the client polls `GET /images/{id}` or subscribes via SSE. "Processing" usually lasts
  1–3 s.
- Resizing on the fly instead (`?w=300` at request time) saves storage and gives flexibility,
  but an uncached request costs 200 ms of CPU and an attacker can request 10,000 distinct sizes.
  Pre-generate the fixed set; allow on-the-fly only behind the CDN with a whitelist of sizes.

### 9.4 Serving: immutable URLs and the CDN

- URLs embed the **content hash**, so the content behind a URL can never change. That permits
  `max-age=31536000, immutable`: the CDN and browsers never revalidate. Egress to origin drops to the miss rate (a few percent).
- Variants are separate objects; the HTML picks `t` for galleries and `m` for pages. Serve WebP
  (30% smaller) with a JPEG fallback via `Accept` negotiation at the CDN, or just emit WebP everywhere.
- Set `Content-Type` correctly and `X-Content-Type-Options: nosniff`; serve from a **separate
  domain** (`cdn.example`, no cookies) so a malicious file can't run with the main site's origin.
- Hot image (front page): the CDN absorbs it entirely; origin sees one request per edge location.

### 9.5 Private images

The CDN caches public objects for anyone. Private objects need a check per viewer, but the CDN
cannot call our database. Standard answer: **signed URLs**. The API, after checking ownership,
returns `https://cdn…/i/{sha}/m.webp?Expires=1760349200&Signature=…` valid for, say, 1 hour.
The CDN verifies the signature with a key we configured and serves from cache. Trade-off: a
signed link can be shared until it expires; make the expiry short and re-sign on each page load.

### 9.6 Dedup and deletion

- Dedup by `sha256` of the normalised original: two users uploading the same photo share one
  `blobs` row and one set of variants; `ref_count` tracks owners. Saves storage for memes and reposts
  (often 20–40% in practice).
- **Delete** = mark the `images` row `deleted`, decrement `ref_count`; when it hits zero, a job
  deletes the objects and purges the CDN paths. Because URLs are content-addressed and shared, a
  deleted public image remains cached at the edge until purged: purge explicitly.
- Raw uploads in `uploads/` are deleted by a lifecycle rule after 24 h regardless (processed or not).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** processing bursts (600 uploads/s × 200 ms = 120 cores at peak).
  Workers autoscale on queue depth; uploads are acknowledged regardless, so the user experience
  degrades to "thumbnail takes 20 s" rather than failing.
- **S3 event notifications are at-least-once and can be delayed:** the worker's idempotency and the
  client's `/complete` call cover both duplicates and delays.
- **Object storage down:** uploads fail (presign is issued but PUT fails); serving continues from the
  CDN for cached objects. Metadata API is unaffected.
- **Queue down:** uploads land in S3 and sit at `pending`; a reconciler job lists `uploads/` older
  than 1 minute with `status = pending` and enqueues them when the queue is back.
- **Cost:** egress dominates; CDN hit rate is the lever. Storage: tier originals to infrequent
  access after 30 days; variants stay hot.
- **Abuse:** per-user upload quotas (task 03), the async scanner hook, and a takedown path that
  purges the CDN.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| presigned direct upload | proxy through API | API tier independent of upload bandwidth; resumable uploads | two-step client flow; validation is post-hoc |
| decode + re-encode everything | trusting the file | polyglot/EXIF/bomb safety | CPU per upload; original is not byte-identical |
| pre-generated variants | resize on request | predictable cost, no abuse surface | 3× storage for variants; fixed sizes |
| content-hash URLs, `immutable` | ID-based mutable URLs | permanent caching, dedup for free | deletes need explicit CDN purge |
| signed URLs for private | per-request auth at origin | CDN caching for private content | links are shareable until expiry |

## 12. Check yourself

1. Why should upload bytes not pass through the API servers, and what does the presign have to constrain?
2. A file named `cat.jpg` with `Content-Type: image/jpeg` arrives. What do you check, and why isn't that enough?
3. What makes `Cache-Control: immutable` safe here, and what does it cost on delete?
4. How do you serve a private image from a CDN that can't talk to your database?
5. The S3 notification for an upload arrives twice, and the client also calls `/complete`. What happens?

<details><summary>Answers</summary>

1. Each upload would hold an API connection and stream megabytes through it for seconds, so the API
   tier would be sized by bandwidth and slow clients instead of by API logic. The presign must pin the
   object key, maximum size, allowed content type and a short expiry, otherwise it is a blank cheque.
2. Both the name and the header are client claims. Sniff the magic bytes, then fully decode with
   dimension and time limits. Sniffing alone doesn't catch polyglot files or decompression bombs;
   decoding and re-encoding does, and strips EXIF as a side effect.
3. The URL contains the content hash, so the bytes behind it can never change; caches may keep it
   forever. On delete the bytes are still at every CDN edge until you issue an explicit purge for
   those paths, and shared blobs can only be purged when the reference count reaches zero.
4. Signed URLs: the API checks ownership, then returns a URL with an expiry and a signature the CDN
   can verify with a configured key. The CDN serves from cache without contacting us. The trade-off is
   that the link works for anyone until it expires, so keep expiry short.
5. Three `process(image_id)` jobs. The worker checks the row's status first: the first job does the
   work and sets `ready`; the others see `ready` (or `processing` with a lock) and exit. Idempotency
   in the worker makes the at-least-once triggers harmless.

</details>

## 13. In Go

- Related lessons: [19 stdlib tour](../../19-stdlib-tour.md) (`io`, streaming, `image` package), [44 Background jobs](../../44-background-jobs-queues.md) (the pipeline), [57 Web security](../../57-web-security.md) (`nosniff`, separate origin, SSRF when fetching by URL), [36 Resilience](../../36-resilience-patterns.md) (retries to S3), [55 Data pipelines](../../55-data-pipelines.md).
- Sniffing: `net/http.DetectContentType` on the first 512 bytes. Decoding: `image.DecodeConfig` first (cheap: dimensions only) to reject bombs, then `image.Decode`. Resizing: `golang.org/x/image/draw` (stdlib-adjacent) or `disintegration/imaging`; `h2non/bimg` (libvips) for speed.
- Presign: `aws-sdk-go-v2/service/s3` `PresignClient.PresignPutObject` with `ContentLength` and `ContentType` conditions.
- Packages: `image/jpeg`, `image/png`, `golang.org/x/image/webp` (decode only; encode via `chai2010/webp` or libvips), `crypto/sha256`.

## 14. Extensions

- **Multipart/resumable uploads** for 20 MB over flaky mobile networks (S3 multipart or tus protocol).
- **Perceptual hashing** (pHash) to find near-duplicates and reposts, not just identical bytes.
- **On-the-fly transforms** behind the CDN with a signed whitelist of parameters; compare cost to pre-generation.
- **Video**: a transcoding pipeline with HLS output and adaptive bitrate (medium).
- **NSFW / malware scanning** as a pipeline stage that gates `ready`; what does the UI show meanwhile?
- **Multi-region replication** of the bucket for durability and faster origin misses.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). The hardware is tiny; the bytes are not: servers come to under 2 % of the bill.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Processing workers (Go + libvips: sniff, decode, 3 encodes, sha256) | 60 uploads/s (12/s average) × 200 ms CPU (section 4) | CPU, then network | 60 × 0.2 s = 12 vCPU (average 2.3, section 4's "≈ 3 cores"). N = ceil(12 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(2.8) = 3. Memory: assume a 100 MP pixel cap (section 9.2 gives none) ⇒ ≤ ~0.8 GB per decode × 8 parallel = 6.4 GB of 16 GiB. Network: 60 × (2 MB in + 2.2 MB out) = 252 MB/s ≈ 2 Gbit/s ÷ 3 = 0.67 Gbit/s per node | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS, on Spot behind autoscaling on queue depth | 3 at peak, one per zone; fewer between peaks | image work is pure CPU, so `c`; the pixel cap is what keeps memory at 6 GB, raise it and it needs `m`. 50 % busy at peak, 75 % with a zone down. Jobs are idempotent (section 9.3), so a 2-minute Spot reclaim only delays a thumbnail. Network is sustained for the minutes a peak lasts: check the instance's baseline, not its "up to" figure. A VPC gateway endpoint keeps S3 traffic off a NAT, which would add $0.045/GB × ~128 TB ≈ $5.8k a month |
| API (Go, stateless: presign, status polls, metadata) | 60 presigns/s × 1 ms (section 9.1) + assumed ~900/s of status polls and metadata reads × 0.2 ms | CPU, ~0 | 0.06 + 0.18 ≈ 0.25 vCPU. N = ceil(0.25 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) | 3, one per zone | tiny: the zone rule sets the count, not load (4 % busy); upload bytes never come here, which is the point of presigning |
| Postgres (`images`, `blobs`) | ~12 image rows/s (60 peak) × 3 writes ≈ 180 commits/s at peak; 300 MB/day ≈ 110 GB/year (+ ~20 GB index) | none (small), then memory | ≈ 2 IOPS per commit (assumption) ⇒ ~360 + ~300 misses ≈ 700, under `gp3`'s 3,000 included. Hot set = last week's rows ≈ 2 GB ≪ 8 GiB. CPU ≈ 0.25 vCPU of 2. 200 GB `gp3` covers year one | `db.m7g.large` (2 vCPU / 8 GiB), RDS Multi-AZ | 1 primary + standby | the smallest non-burstable node already has 4× the memory the hot rows need; no `r`, no extra IOPS. Old rows are read only for an owner's gallery |

Nothing to size: **S3** (peak ~60 PUT/s from clients + ~180/s from workers and ~500/s of CDN misses, all far under ~3,500 PUT/s and ~5,500 GET/s per prefix; `images/{sha}/` spreads keys), **CloudFront** (the egress in the bill below), and **SQS** (≈ 2M messages a day, because the S3 event and `/complete` both enqueue; × 3 requests × 30.4 days × $0.40/M ≈ $70). Stored per image ≈ 2 MB original + 0.2 MB medium + ~0.02 MB thumbnail ≈ 2.2 MB, so ≈ 2.2 TB/day ≈ 810 TB after one year (section 4's 730 TB counts originals only; dedup would trim it 20–40 %).

**Rough monthly bill (≈, on-demand; storage priced at month 12, ~810 TB held)**

| Line | ≈ $/month |
|---|---|
| CloudFront data out: 20 TB/day × 30.4 = 608 TB × $0.085 (first-tier price; volume tiers lower it) | $51,700 |
| S3 Standard: 810 TB × $0.023 (grows ≈ $1.5k each month) | $18,600 |
| Workers: 3 × `c7g.2xlarge` (3 × $212), less on Spot | $640 |
| API: 3 × `c7g.large` ($159) + ALB (≈ $30) | $190 |
| Postgres: `db.m7g.large` ≈ $120, Multi-AZ doubles; 200 GB gp3 × 2 = $32 | $270 |
| SQS | $70 |
| **Total** | **≈ $71,500** (≈ $71,100 with 1-year commitments: the $1,040 of servers hardly moves it) |

Not counted (off the sheet, from memory): CloudFront request fees on ~3B views a month and S3 request fees on ~120M PUTs, each a few $k at most.

- **Biggest line and why:** CloudFront data out (72 %), then S3 storage (26 %). Everything with an instance is 1.6 %. CloudFront bills the bytes it sends to viewers whether the CDN hit or missed, and S3 to CloudFront is free, so on AWS the CDN hit rate (section 10) lowers origin load, not this line. The levers are bytes per view (WebP, smaller `m` variant), a committed-use contract, and for storage tiering originals to infrequent access and dedup.
- **What changes at 10×:** 600 uploads/s ⇒ 120 vCPU ⇒ N = ceil(120 ÷ 4.27) = 29 → 30 × `c7g.2xlarge` (≈ $6.4k, a lot less on Spot), still 0.67 Gbit/s per node; Postgres at ~1,800 commits/s ⇒ `db.m7g.2xlarge` with ~4k IOPS. Neither is the problem: data out ≈ 6 PB a month (≈ $0.5M at first-tier price) and ≈ 8 PB stored after a year (≈ $186k a month at the anchor) make bytes, tiering and CDN contracts the whole business.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Two kinds of data. Bytes: ~2.2 MB stored per image (original, medium, thumbnail) ≈ 2.2 TB/day and ≈ 810 TB after a year, immutable once written and served by a CDN (sections 4, 15). Metadata: ~12 rows/s (60/s at peak), ~300 B each ≈ 110 GB a year, ~180 commits/s at peak.
- Bytes are fetched whole by key (`images/{sha}/{variant}`). Metadata is fetched by primary key plus one range (`owner_id, created_at DESC`), and dedup is "does this `sha256` exist, then bump `ref_count`".
- Only the metadata needs transactions (an `images` row and `blobs.ref_count`).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✔ as the system of record for metadata, ✖ for the bytes | `images` and `blobs` use primary-key lookups, the gallery index, a `ref_count` under a row lock and `variants JSONB` read whole. Putting the bytes in `bytea` would push ~2.2 TB/day through the WAL, backups and the standby, and 810 TB is more than two orders of magnitude above the "few TB" one primary is sized for (cheat sheet) |
| MySQL (RDS / Aurora) | ✔ as the system of record for metadata, ✖ for the bytes | the metadata uses only portable features, so the verdict matches Postgres; `BLOB` columns have the same problem as `bytea` at this size |
| DynamoDB | ✔ works for metadata, no reason to add it; ✖ for the bytes | ~300 B items and 60 writes/s are nowhere near its limits, the gallery would be an index and `ref_count` an atomic counter, but it replaces a ~$270 a month line (section 15) with a second data model; originals of up to 20 MB are far over its 400 KB item limit |
| S3 | ✅ chosen for the bytes | one immutable object per variant, the CloudFront origin (S3 to CloudFront transfer is free, section 15); well under the per-prefix 3,500 PUT / 5,500 GET per second; presigned PUTs keep the bytes off our servers (section 9.1); lifecycle rules expire `uploads/` after a day and tier originals |

**Pick:** S3 for the bytes (~810 TB after a year, the second line of section 15's bill) and PostgreSQL on RDS Multi-AZ (`db.m7g.large`, 200 GB `gp3`) for the metadata. Bytes are immutable objects behind a CDN; metadata is a small relational table whose one consistency need is `ref_count`. S3 holds the record of the image (the re-encoded original `o`; medium and thumbnail can be regenerated from it), Postgres holds ownership and status, and the CDN cache is derived.

**If it were MySQL instead:** nothing material. `JSONB` becomes `JSON` (read whole, never queried inside); the blob upsert `ON CONFLICT (sha256) DO UPDATE SET ref_count = ref_count + 1` becomes `ON DUPLICATE KEY UPDATE ref_count = ref_count + 1`; a decrement-and-check in one statement (`RETURNING`) becomes an `UPDATE` then a `SELECT` in the same transaction; `TEXT` keys become `VARCHAR(n)` or `CHAR(64)`. At 12 inserts/s the page splits of a random `sha256` primary key in InnoDB are invisible.

**What would flip the choice:** the bytes never move to a database. The metadata flips on a requirement, not a number: at 10× it is ~1,800 commits/s, still one `db.m7g.2xlarge` (section 15); uploads that must be accepted in several regions at once are where per-key ownership (DynamoDB) becomes attractive. The cost levers are S3 tiering and dedup (20-40 %), not another engine.
