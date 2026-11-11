# 55 · Order Timeline with Photo Steps

`🟢 easy` · *Topics:* a user-written progress timeline is not the audit log, step model with source / visibility / version, offline-first capture with ids minted on the device, text first and photos trickling in (resumable direct-to-storage), EXIF time kept and GPS dropped, a revocable tracking link, keyset ordering with back-dated steps and system milestones · *Builds on:* [22 Audit Log & Revision History](22-audit-log-revisions.md) (the *other* history), [26 Configurable Status Workflow](26-status-workflow.md) (where system milestones come from), [11 Image Upload & Hosting](11-image-upload-hosting.md) (presigned uploads, the processing pipeline), [45 Live Notifications](45-live-notifications.md) (doorbell push)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [54 Programmatic Location Pages](54-programmatic-location-pages.md) · Next: [56 Related Content & Interlinking](56-related-content-interlinking.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 16, migration 0043. An order has a **Steps** tab: `order_steps` (title ≤ 120 chars, note ≤ 5,000, `happened_at` = now unless set back, never ahead) and `order_step_media` (up to 20 library photos per step, private, left in the library when a step is removed). It is built for a phone in a driveway: the camera is one tap (`capture="environment"`), each photo uploads as it is picked with its own progress, and Save waits for them. A permission of its own, `orders.steps`, lets a crew role log work without being able to edit price, client or status; a save is version-checked (stale = `409`); every add / change / remove is a History line in `audit_log`. The library re-encodes each photo to at most 2,400 px with a thumbnail and no EXIF, so the phone's GPS is gone. Step 35 (migration 0067) lets a deadline list the steps that meet it. **The customer sees nothing yet**: step 16's own decision 5 defers a "visible to the client" flag and a portal. Everything about customers, offline capture and scale below is the lesson's design, not the repo's.
> **Here, at scale:** a field-service platform with 20M jobs a year, 8 steps each, 40% of steps carrying about 3 phone photos taken on poor networks, and customers watching a tracking page.

## 1. The task

> "Design the progress timeline of a job for a field-service platform. Crews log what they did, with photos, from phones on site, often with one bar of signal. The office sees everything. The customer gets a link to watch the job's progress."

Two histories hide in this sentence, and mixing them is the classic mistake: what *people chose to say* about the job (the timeline), and what *the system recorded* about every change (the audit log, [22](22-audit-log-revisions.md)).

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Is the timeline the audit log? | Different readers, authorship, mutability. | No. The audit log is automatic, immutable, internal. The timeline is written by people, editable, and partly customer-facing. |
| Who writes steps? | Permissions. | Crew accounts holding `orders.steps` (logging work does not require editing the order). |
| Who reads them? | Privacy decides the design. | Staff: everything. Customer: only steps explicitly published, through a link, no login. |
| Offline? | Decides ids and queues. | Yes: a phone may be offline for hours; steps and photos are queued on the device. |
| Photo limits? | Bounds storage and processing. | ≤ 20 per step, ≤ 25 MB raw each, type by sniffed content ([11](11-image-upload-hosting.md) §9.2). No video in v1. |
| Edit or delete after the customer saw it? | Trust and surprise. | Edit allowed and marked "edited"; delete is soft. The old text lives in the audit log. |
| What time does a step carry? | Devices have wrong clocks. | The crew's claimed time, set back freely, never ahead of the server's now (plus small skew). |
| Tell the customer? | Notification volume. | One message per burst of newly published steps ([12](12-notification-service.md)), with the link. |
| Scale? | Store and processing choices. | 20M jobs/year, 8 steps each, 40% of steps with ~3 photos, 100k tracking pages open at the busiest minute. |

## 3. Requirements

**Functional**
- Add, edit, delete a step: title, note, time, up to 20 photos. Photos can arrive after the text.
- A timeline per order, newest first, paginated; staff see all, customers see published steps.
- Publish and retract steps to the customer; a tracking link per order that can be revoked.
- Key status milestones ("In progress", "Completed") appear in the timeline automatically.
- Each change is audited (22). A step can be linked to the deadline it meets ([27](27-deadlines-escalation.md)).

**Non-functional**
- Saving a step works on one bar of signal: the text must not wait for photos.
- Retrying any request or upload never duplicates a step or a photo.
- Photos are private by default; no GPS leaves the pipeline; a leaked link can be killed.
- Customer page p95 < 300 ms; a new published step reaches an open page in seconds.

**Out of scope**
- Video, chat with the customer, signatures, face blurring (see Extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Steps per year | 160M | 20M jobs × 8 |
| Step writes | ~18/s, peak ~55/s | 160M ÷ 9M working seconds (250 days × 10 h); peak 3× |
| Photos per year | 192M | 160M steps × 40% × 3 |
| Photo uploads | ~21/s, peak ~64/s | 192M ÷ 9M; peak 3× |
| Upload bandwidth at peak | 192 MB/s ≈ 1.5 Gbit/s raw (3 MB each); ~51 MB/s ≈ 0.4 Gbit/s if the app resizes to ~0.8 MB | 64 × 3 MB; 64 × 0.8 MB |
| Stored photos | ~121 TB/year | 192M × (0.6 MB re-encoded + 0.03 MB thumbnail), assumed sizes |
| Database rows | ~141 GB/year | steps 160M × ~0.7 KB = 112 GB; photo rows 192M × ~150 B = 29 GB |
| Tracking views | ~19/s average, ~190/s peak | 20M jobs × ~30 views ÷ 31.5M s; peak 10× |
| Thumbnails served | ~114/s, ~108 TB/year | 600M views × 6 thumbs ÷ 31.5M s; × 30 KB |
| Processing at peak | ~32 cores | 64 photos/s × ~0.5 CPU-s (assumed, as in [40](40-media-reprocessing.md)) |

What the numbers tell us: the **database is not the problem**: 55 step writes/s and ~190 reads/s are trivial. The load is **bytes from bad networks** (photos) and **trust** (private data behind a public link). So the design effort goes into the upload path and the link, not into sharding.

## 5. Plan

1. Separate the timeline from the audit log; fix the step model and who sees what.
2. Capture: ids from the device, text first, photos attached as they land.
3. The photo pipeline: quarantine, re-encode, EXIF, states.
4. The customer's tracking link, read path and push.
5. Ordering, pagination, and system milestones in the same list.

## 6. High-level design

```
 crew phone (queue in IndexedDB: step, then its photos; ids minted on the device)
   │ 1 POST /orders/{id}/steps {uid,…}   (a few hundred bytes: survives one bar of signal)
   │ 2 POST …/photos {uid,bytes,sha256}  → presigned multipart URLs
   │ 3 PUT chunks ─────────────────────► quarantine bucket
   ▼                                             │ object-created event
  LB ─► API ─► Postgres ◄────────────────┐      ▼
            │  order_steps, order_step_photos,   photo processor (11, 40): sniff, decode budget,
            │  order_timeline_state, audit_log   re-encode ≤ 2,400 px, thumbnail, read EXIF time/GPS,
            │                                    strip metadata ─► private library bucket ─► state=ready
            └─ outbox ─► live bus ─► "version 42" ─┐
                    └──► notify (12): "update on your job"
 customer: GET /t/{token} ─► edge cache (s-maxage 15 s) ─► Tracking API (replica) ─► JSON + signed thumbnail URLs
           GET /t/{token}/events  (SSE doorbell, 45)
```

**One write.** The phone saves the step locally, POSTs it with its `uid` (replays are no-ops), the API inserts the row, the audit entry and an outbox row in one transaction and bumps `order_timeline_state.version`. The office sees "Framing inspected · 3 photos on the way". Each photo then uploads in chunks; when the processor finishes, the photo flips to `ready` and the version bumps again.

**One read.** The customer's page fetches `/t/{token}`: hash the token, find the order, read the published, non-deleted steps (page 1), attach signed thumbnail URLs, cache by `(order, version)`. The SSE doorbell says "version 43" and the page refetches.

## 7. API

```
GET    /orders/{id}/timeline?cursor=&limit=25&audience=staff|customer   → 200 { items:[…], next_cursor, version }
POST   /orders/{id}/steps        { uid, title, body?, happened_at?, photos_expected }       → 201 | 200 (replay) step
PUT    /orders/{id}/steps/{sid}  If-Match: "<version>"  { title?, body?, happened_at? }      → 200 | 409 stale | 422
DELETE /orders/{id}/steps/{sid}                                                             → 204 (soft)
POST   /orders/{id}/steps/{sid}/photos  { uid, content_type, bytes, sha256 }                → 201 { upload: { parts:[url…] } } | 200 (replay)
POST   /orders/{id}/steps/{sid}/publish   |   /retract                                       → 200   (needs orders.steps.publish)
POST   /orders/{id}/tracking-link        → 201 { url }  (shown once)   DELETE → revoke
GET    /t/{token}                → 200 customer timeline (published steps only)   ETag: "<version>"
GET    /t/{token}/events         → text/event-stream  { "version": N }
```

Staff routes need `orders.steps` (as in the repo, not `orders.write`); the customer routes need only the token. Times are sent and stored in UTC; display is the business's time zone ([22](22-audit-log-revisions.md), exact time everywhere).

## 8. Data model

```sql
CREATE TABLE order_steps (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id         BIGINT      NOT NULL REFERENCES orders(id),
    uid              UUID        NOT NULL,                     -- minted on the device
    source           TEXT        NOT NULL DEFAULT 'crew' CHECK (source IN ('crew','system')),
    title            TEXT        NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    body             TEXT        CHECK (length(body) <= 5000),
    happened_at      TIMESTAMPTZ NOT NULL,                     -- never ahead of the server's now: checked in the service
    photos_expected  SMALLINT    NOT NULL DEFAULT 0 CHECK (photos_expected BETWEEN 0 AND 20),
    customer_visible BOOLEAN     NOT NULL DEFAULT false,
    published_at     TIMESTAMPTZ,
    version          INTEGER     NOT NULL DEFAULT 1,           -- If-Match target
    edited_at        TIMESTAMPTZ,
    deleted_at       TIMESTAMPTZ,
    audit_id         BIGINT,                                   -- system rows: the audit entry they mirror
    created_by       BIGINT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, uid),                                    -- idempotency
    CHECK (source = 'crew' OR audit_id IS NOT NULL)
);
CREATE INDEX order_steps_timeline_idx ON order_steps (order_id, happened_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX order_steps_public_idx   ON order_steps (order_id, happened_at DESC, id DESC)
    WHERE customer_visible AND deleted_at IS NULL;

CREATE TABLE order_step_photos (
    step_id    BIGINT   NOT NULL REFERENCES order_steps(id) ON DELETE CASCADE,
    uid        UUID     NOT NULL,                              -- minted on the device
    file_id    BIGINT,                                         -- library file, set when processing finishes
    state      TEXT     NOT NULL DEFAULT 'uploading' CHECK (state IN ('uploading','processing','ready','failed')),
    sort_order SMALLINT NOT NULL DEFAULT 0,
    taken_at   TIMESTAMPTZ,                                    -- EXIF time, read before metadata is stripped
    on_site    BOOLEAN,                                        -- coarse check vs the job address; coordinates never stored
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (step_id, uid)
);

CREATE TABLE order_timeline_state (                            -- not a column on orders (see 9.4)
    order_id BIGINT PRIMARY KEY REFERENCES orders(id),
    version  BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE order_tracking_links (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id   BIGINT      NOT NULL REFERENCES orders(id),
    token_hash BYTEA       NOT NULL UNIQUE,                    -- sha256 of a 128-bit random token; the token is never stored
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- from step 35 (migration 0067), kept as is: order_deadline_steps (deadline_id, step_id), both of the same order.
```

## 9. Deep dives

### 9.1 A step is not an audit entry

Both histories answer "what happened to this order, when, by whom", which is why they get merged by accident. They differ in everything else:

| | Audit log ([22](22-audit-log-revisions.md)) | Timeline (this task) |
|---|---|---|
| Author | the system, on every write | a person, deliberately |
| Reader | staff, security, support | staff and the customer |
| Mutable? | never | edit, back-date, retract, delete |
| Content | before/after of fields | a sentence and photos |
| Volume | every write (20k/s in 22) | ~8 per job |
| Retention | years | the life of the order |

| Option | Pros | Cons |
|---|---|---|
| One event log; the timeline is a filtered view | one pipeline | steps must be editable and carry photos; "edit" of an immutable log means compensating events; customer-safe subset tangled with security data |
| **Two tables: `order_steps` plus an audit entry for each change** | each fits its reader; what step 16 does (History lines) | two writes, in one transaction |
| Steps as a JSON array on the order row | one read | every step rewrites the row; two crew adding at once clobber each other; photo states rewrite it again; no per-step index |

**Choice and why.** Two tables, the audit entry written in the same transaction ([22](22-audit-log-revisions.md) §9.3). The timeline never *displays* audit rows. The only system content in it is the deliberate copy of a few milestones (9.5).

### 9.2 Who sees what, and what an edit or delete means after the customer has looked

The repo keeps everything internal and defers the customer. At scale the risk is a crew note like "wall is rotten, extra cost" reaching the customer unreviewed.

| Option | Pros | Cons |
|---|---|---|
| Everything visible by default | zero effort for crews | leaks internal remarks; the first incident ends the feature |
| **A per-step flag, default off; publishing is an action with its own permission** | safe default; the dispatcher can review; photos inherit | one extra tap; stale unpublished steps (nudge: "3 unpublished steps from today") |
| Two timelines (internal notes, customer updates) | no flag mistakes | duplicated model, UI and APIs; "move this note to the customer" becomes copy-paste |

**Choice and why.** The flag, `orders.steps.publish` for revealing, `orders.steps` for logging (the crew/dispatcher split mirrors the repo's `orders.steps` vs `orders.write`). After publication:
- **Edit** is allowed, bumps `version` (checked with `If-Match`; a stale edit is `409`, as in the repo), sets `edited_at`, and the customer page shows "edited". The old text stays in the audit log.
- **Delete** is soft (`deleted_at`): a hard delete would leave the customer's notification pointing at nothing, and support could not answer "what did we tell them?". Staff see a tombstone; customers see nothing.
- **Retract** (`customer_visible = false`) removes it from the customer page at the next version bump.

### 9.3 Photos from the field: text first, bytes trickle in

The repo uploads each photo as it is picked, with progress, and holds Save until all are in. On one bar of signal a 3 MB photo can take minutes and a drop restarts it; the crew's typed note is hostage to the slowest photo.

| Option | Pros | Cons |
|---|---|---|
| Upload photos first, then create the step with their ids (the repo) | a step never shows broken photos | Save blocked by the worst upload; nothing visible to the office; a killed app loses the text |
| **Create the step first (tiny), attach photos as they land** | the office sees the step at once; text survives; uploads are independent and resumable | placeholders and photo states; a step is "incomplete" for a while |
| One multipart request with everything | one retry unit | all-or-nothing on the worst link; the API holds large bodies |

**Choice and why.** Step first, photos after. The mechanics:
- **Ids minted on the device** (`step.uid`, `photo.uid`) with `UNIQUE (order_id, uid)` and `PRIMARY KEY (step_id, uid)`: every retry, double tap and re-sent queue item is a no-op ([16](16-public-form-intake.md) §9.1 for replay semantics).
- **A device queue** (IndexedDB) keeps the step, then each photo; the device keeps the original until the server says `ready`, and resizes to ~2,400 px before upload (§4: 1.5 → 0.4 Gbit/s at peak).
- **Direct, resumable upload** to a quarantine key (multipart parts, retried individually; [11](11-image-upload-hosting.md) §9.1), never through the API.
- **States** `uploading → processing → ready | failed`; the processor sniffs by content, applies the decode budget, re-encodes, makes the thumbnail ([11](11-image-upload-hosting.md) §9.2–9.3, [40](40-media-reprocessing.md)).

EXIF is read once, then stripped from what is stored:

| Option | Pros | Cons |
|---|---|---|
| Keep GPS in the file | proof of presence | the crew's other locations leak to anyone who can fetch the photo |
| Drop all metadata (the repo) | simplest, private | loses the capture time and presence evidence |
| **Read time and a coarse location check, store only `taken_at` and `on_site`, drop the file's metadata** | privacy and a useful signal | needs the job's geocoded address ([31](31-service-area-geocoding.md)); the check is advisory |

`taken_at` is a hint to validate or prefill a step's time (phone clocks lie), never an override of `happened_at`. Photos are private: thumbnails are served with short-lived signed URLs tied to the staff session or the tracking token, never as public paths.

### 9.4 The customer's tracking page

| Option | Pros | Cons |
|---|---|---|
| Read Postgres (a replica) on every view | simplest; ~190/s peak is easy | a link posted publicly can spike it |
| **Replica reads behind an edge cache keyed by token: `s-maxage=15, stale-while-revalidate=60`, ETag = timeline version** | origin stays quiet under a viral link; 304s are cheap | up to ~15 s of staleness, covered by the push |
| A static JSON file per order in object storage, rewritten on every change | zero database reads | 20M tiny objects, write amplification, purge complexity, signed photo URLs inside static files |

**Choice and why.** Replica plus edge cache. At ~190 views/s the cache is protection, not necessity, so do not build per-order static files.

The link is a **capability**, so it is not the order's `public_id` (an address, not a permission: ADR 0006). It is 128 random bits in the path, stored only as `sha256`, with an expiry (say 90 days after completion), `revoked_at`, `Referrer-Policy: no-referrer`, `X-Robots-Tag: noindex`, and a per-token rate limit ([17](17-abuse-protection.md)). A plain SHA-256 is right for a 128-bit random token; slow password hashes are for low-entropy secrets.

**Why the version lives in its own table:** a counter column on `orders` would move `orders.updated_at`, which is the order's *edit version*. Every open order form would then say "someone else changed this" after each step. The repo hit exactly this with assignments (migration 0045) and made an assignment leave `updated_at` alone. A separate `order_timeline_state` row avoids the question.

**Push** is a doorbell ([45](45-live-notifications.md) §9.1–9.3): the SSE message carries only `{version}`; the page refetches with `If-None-Match`. 100k open pages at peak is a few nodes' worth. Polling every 30–60 s with the ETag is the fallback. Customer messages go through the outbox and are **coalesced**: steps published within ~10 minutes produce one message ([12](12-notification-service.md)).

### 9.5 Ordering, pagination, and system milestones in the same list

Order by `happened_at DESC, id DESC`. A crew can back-date, so a new row may land in the middle of what is already loaded. That is fine: the client reloads page 1 when the version changes, and keyset cursors are values, not offsets ([23](23-paginated-lists.md) §9.1, §9.5). A typical job has 8 steps (one page); a long project with 10k steps pages 25 at a time using the partial indexes above.

Customers care about "In progress" and "Completed", which come from the status workflow ([26](26-status-workflow.md)), not from crew typing.

| Option | Pros | Cons |
|---|---|---|
| Merge two sources at read time (steps + status history) | no duplicated data | a merge-sorted cursor across two tables; audit rows are too noisy to filter for customers |
| **Copy customer-meaningful milestones into `order_steps` as `source = 'system'`, in the same transaction as the move** | one table, one index, trivial pagination | a small duplicate; the row must be immutable |
| Don't show system events | simplest | the customer sees crew chatter but not "your job is complete" |

**Choice and why.** Materialise. Keep the two kinds apart so they are never confused: `source` is a column; system rows have no edit, delete or photos; their `happened_at` is the server's clock; a label ("Status: In progress"); and `audit_id` points to the audit entry they mirror. A crew step back-dated before the status change sorts before it, which is simply true. A step can also list the deadline it meets (`order_deadline_steps`, [27](27-deadlines-escalation.md)); the add-step form may *suggest* marking that deadline done, never do it silently.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** photo processing and storage, not Postgres. 640 photos/s at peak × ~0.5 CPU-s ≈ 320 cores (32 today); storage grows ~1.2 PB/year. Autoscale the processors on queue age, move photos older than two years to cold storage ([24](24-data-lifecycle.md)). Rows reach ~1.4 TB/year: partition by `order_id` hash or archive closed orders.
- **Processor down:** steps and text keep working; photos show "processing"; page the owner when the oldest `uploading`/`processing` photo is older than 15 minutes ([50](50-alerting-engine.md)).
- **Storage or API down:** the device queue holds everything and replays; idempotent ids make the replay safe.
- **Bad device clock:** the repo refuses a time ahead with `422`; with an offline queue the user cannot fix it later, so clamp to the server's now and keep `taken_at` as the evidence.
- **Link leaked:** revoke and reissue; unpublished steps and staff notes were never reachable; photo URLs expire in minutes.
- **Single points of failure:** Postgres (the device queue buffers an outage), the object store (same), the live bus (pages fall back to ETag polling).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| timeline table + audit entries | one event log | editable, customer-safe, simple pagination | two writes per change |
| per-step visibility, default off | visible by default | no leaked internal notes | an extra publish tap |
| step first, photos after | photos first | text survives bad networks; early office view | photo states and placeholders |
| ids minted on the device | server-assigned ids | idempotent offline replay | a UUID column and unique constraints |
| copy milestones as system rows | read-time merge | one ordered list | a small, immutable duplicate |
| `taken_at` + `on_site`, metadata dropped | keep GPS | privacy with a useful signal | the check is only advisory |
| separate version table | counter on `orders` | order forms are not invalidated | one more small table |
| hashed capability link | order public id in the URL | revocable, expiring, read-only | a lookup per request |

## 12. Check yourself

1. How does the timeline differ from the audit log, and what goes wrong if you merge them?
2. A crew member with one bar of signal saves a step with 4 photos. Walk through what happens and what the office sees.
3. Why are the ids minted on the device, and which constraints make retries harmless?
4. A customer posts their tracking link on social media. What limits the damage?
5. A crew member back-dates a step before the "In progress" milestone. What does the timeline show, and how is paging affected?

<details><summary>Answers</summary>

1. The audit log is automatic, immutable, internal and high-volume; the timeline is human-written, editable, partly customer-facing and ~8 rows per job. Merging them makes the audit log editable (or the timeline immutable), leaks security data to customers, and forces compensating events for every edit.
2. The phone saves the step locally and POSTs the small step with its `uid`; the office sees it with "4 photos on the way". Photos upload as resumable multipart to quarantine, individually retried; when each is processed it flips to `ready` and the version bumps. A dropped connection loses nothing: the queue replays, and unique ids make replays no-ops.
3. Because the device may send the same thing several times before it gets an answer and has no server id yet. `UNIQUE (order_id, uid)` and `PRIMARY KEY (step_id, uid)` turn duplicates into returning the first row.
4. Only published steps are reachable; the token is 128 bits, stored hashed, revocable and expiring; the page is `noindex` and `no-referrer`; views are rate limited per token; the edge cache absorbs the traffic; photo URLs are signed and short-lived. Revoke and reissue ends it.
5. It sorts before the milestone (the row is honest: work logged for an earlier time). Keyset cursors are `(happened_at, id)` values, so already-loaded pages are unaffected, and the client reloads page 1 on the next version bump.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md), [59 Production DB patterns](../../59-production-database-patterns.md) (upserts, partial indexes), [41 API design](../../41-api-design-evolution.md) (idempotency, `ETag`/`If-Match`, `409`), [34 Outbox](../../34-event-driven-outbox.md), [44 Background jobs](../../44-background-jobs-queues.md) (the photo processor), [58 SSE](../../58-realtime-websockets-sse.md), [57 Web security](../../57-web-security.md) (capability URLs, referrer), [38 Caching](../../38-caching-patterns.md).
- Packages: `pgx`, `aws-sdk-go-v2/service/s3` (`s3.NewPresignClient`, multipart), `github.com/disintegration/imaging` or `golang.org/x/image`, `github.com/rwcarlsen/goexif/exif`, `crypto/rand`, `crypto/sha256`.

The idempotent insert (the `xmax = 0` trick tells an insert from a conflict; it is an implementation detail, so a plain `DO NOTHING` plus `SELECT` is the portable alternative):

```go
const insertStep = `
INSERT INTO order_steps (order_id, uid, title, body, happened_at, photos_expected, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (order_id, uid) DO UPDATE SET uid = EXCLUDED.uid   -- harmless write so RETURNING gives the existing row
RETURNING id, version, (xmax = 0) AS inserted`

func (r *Repo) AddStep(ctx context.Context, tx pgx.Tx, s NewStep) (id int64, version int, inserted bool, err error) {
	err = tx.QueryRow(ctx, insertStep, s.OrderID, s.UID, s.Title, s.Body, s.HappenedAt, s.PhotosExpected, s.By).
		Scan(&id, &version, &inserted)
	return // if inserted: write the audit entry, the outbox row and bump order_timeline_state in this same tx
}
```

The tracking token:

```go
func newTrackingToken() (token string, hash []byte, err error) {
	b := make([]byte, 16) // 128 bits
	if _, err = rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b) // 22 URL-safe characters, shown once
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil // store sum; look up by sha256(presented token)
}
```

## 14. Extensions

- **Video steps:** transcoding and long uploads ([40](40-media-reprocessing.md)); how much do customers really need?
- **Before / after pairs** and required photos per step type ("a delivery needs a photo of the drop-off").
- **Face and licence-plate blurring** before a photo may be published.
- **Step templates and quick buttons** ("Arrived", "Demo done") so crews type less; AI-drafted summaries from photos with human approval ([38](38-ai-draft-queue.md)).
- **Customer approval:** "approve this step" as a transition in the status workflow ([26](26-status-workflow.md)).
- **A PDF job report** assembled from published steps and photos, as a lazy idempotent job ([30](30-invoicing.md)).
- **Analytics:** time from "arrived" to "completed", on-site rate from `on_site`.
- **Per-tenant retention and legal holds** on photos ([24](24-data-lifecycle.md)).
