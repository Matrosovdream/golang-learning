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
