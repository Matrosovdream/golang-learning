# 19 · Moderated Submissions (Reviews, Photos, Anonymous Feedback)

`🟢 easy` · *Topics:* a quarantine bucket that no code path can serve, validate-by-content and re-encode every image (decompression bombs, EXIF/GPS), a moderation queue sized in human minutes, risk lanes and auto-approval, the "queue nobody checks" failure and its cures, anonymity without a flood (rate limits with no durable identifier), takedown and appeal · *Builds on:* [11 Image Upload & Hosting](11-image-upload-hosting.md) (presigned uploads, CDN), [14 Polls & Voting](14-polls-voting.md) (anonymity), [15 Job Scheduler](15-job-scheduler.md) (leases for the queue), [17 Abuse & Spam Protection](17-abuse-protection.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [18 Inbound Webhook Receiver](18-inbound-webhook-receiver.md) · Next: [20 Per-Tenant Document Numbering](20-document-numbering.md)

---

> 🌱 **Seed (honeydolist-site):** `docs/ideas.md` items 1–3 (event reviews, attendee photo uploads, anonymous feedback). These are **backlog ideas, not built**, but the schema already carries pieces of them: `files.access` has a `quarantine` value ("UNMODERATED public upload, a separate bucket never publicly readable", defaulting to `private` so a forgotten value fails closed; migration 0003); `private_feedback` (migration 0012) has *no* `ip_hash`, user agent, session or `created_by`, and its `context_id` is deliberately not a foreign key; `feedback.read` is its own permission, withheld from the default staff role (migration 0016); the imaging gate refuses over 50 megapixels or 20,000 px a side from the header before decoding, and spends a process-wide 160 MiB decode budget (a 1 MB 12 MP JPEG decodes to 18 MB, a 0.3 MB 34 MP 16-bit PNG to 270 MB); Step 83's public quote photos are sniffed, re-encoded as JPEG with metadata stripped, and stored in the private bucket. The ideas assume "a few dozen a year": pre-moderate everything.
> **Here, at scale:** a platform taking 5M photo and review submissions a day, with no accounts and a team of ten moderators.

## 1. The task

> "Let anyone share a photo or write a review without an account. Nothing unmoderated may ever
> be publicly reachable. A small team must keep up. Feedback to the operators can be anonymous,
> and someone must be able to get their face taken down."

At a few dozen a year, "the owner approves each one" works. At 5M a day the *human* is the
scarcest component, so the design is mostly about spending their minutes well.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Volume? | Decides how much a human can ever see. | 5M/day: 3M photos, 2M reviews; peak 5× average. |
| Accounts? | A login wall kills submissions. | None; an email confirmation link is the friction. |
| Moderate before or after publishing? | Legal and brand risk vs delay. | Before, but not by a human for everything: automation and trust tiers decide the clear cases. |
| How fast to publish? | Sets the SLA per lane. | Reviews median 1 h; photos within 24 h. |
| Anonymous feedback? | A different promise from a review. | A separate box to the operators; never published, no identifier stored. |
| Takedown? | Must exist before launch. | One-click unpublish, an email route, an appeal. |
| Video? | A different project. | Out of scope for v1 (transcoding, bandwidth, moderation cost). |
| Rights? | Legal exposure. | A ticked grant with a stored timestamp; the notice says "no photos of other people's children". |

## 3. Requirements

**Functional**
- Upload photos and submit reviews anonymously; confirm an email address to count.
- Nothing is public until approved; a published item can be unpublished at once.
- A moderation queue with priorities and SLAs; automation approves and rejects the clear cases.
- A feedback box to the operators with no identifying data kept.
- Takedown requests and appeals.

**Non-functional**
- Uploaded bytes are never served; only freshly re-encoded output is.
- Moderators never see the same item twice at once; decisions are audited.
- A queue nobody checks must be impossible to miss.

**Out of scope**
- Video, the review-display UI, legal specifics per country (take advice; illegal content has reporting duties).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Submissions | ≈ 58/s average, ≈ 290/s peak | 5,000,000 ÷ 86,400 = 57.9; ×5 |
| Photo upload bytes | 9 TB/day ≈ 104 MB/s ≈ 0.83 Gbit/s | 3M × 3 MB = 9 TB; ÷ 86,400. Through presigned URLs, not the API |
| Decode CPU | ≈ 35 photos/s × 0.15 s ≈ 5 cores average, ≈ 26 at peak | 3M ÷ 86,400 = 34.7/s; 0.15 s is an assumption |
| Public storage | ≈ 1.4 TB/day ≈ 520 TB/year | about 95% approved (see the lanes in 9.2) = 2.85M × 0.5 MB (re-encoded + variants) |
| Rejected originals kept 7 days for appeals | ≈ 3 TB | about 5% × 3M = 150,000 × 3 MB = 450 GB/day × 7 = 3.15 TB |
| One moderator | 4,320 items/day | 5 s per item × 6 productive hours = 21,600 s |
| Ten moderators | 43,200 items/day = **0.86% of volume** | 10 × 4,320 |
| Pre-moderating everything | ≈ 1,157 moderators | 5,000,000 ÷ 4,320 |
| Anonymous feedback | 20,000/day ≈ 0.23/s | assumption |

What the numbers tell us: bytes and CPU scale by adding machines; **people do not**. Ten moderators can look at under 1% of the traffic, so over 99% must be resolved by trust, automation and sampling, and the queue's order is the only lever left.

## 5. Plan

1. Where untrusted bytes go, and how they become safe bytes.
2. The pipeline: validate, re-encode, score, route.
3. The queue: lanes, SLAs, claiming, and what stops it rotting.
4. The anonymous box: promise, rate limit, schema.
5. Takedown, appeal, and the audit trail.

## 6. High-level design

```
 browser ── POST /uploads ─► presigned PUT ──► QUARANTINE bucket (never public; random keys; expire in 14 d)
    │ POST /reviews | /photos { text, upload_ids, rights_granted:true, email }  ──► 202 pending; confirmation email
    ▼
  API ─► submissions row (status = pending) ──outbox──► pipeline workers
                     1 sniff by content, check pixels and bytes ──► reject early
                     2 decode (memory-budgeted) → apply EXIF orientation → re-encode (no EXIF/GPS) → variants
                     3 hash-match known-bad, classifier(s) → risk score; derive trust tier from verified history
                     4 route:  trusted / low risk → auto-approve (sampled)   uncertain → human lane   high risk → auto-reject
                                                  │
                         moderation queue (lane, due_at) ── moderators claim with leases ── approve | reject + reason
                                                  ▼ approve: COPY re-encoded bytes to the PUBLIC bucket, publish row, invalidate caches
  batched digest + signed confirm links · auto-expire pending after N days · takedown: report → lane 0 → unpublish → CDN purge
  feedback box: POST /feedback → 204, no id, no cookie, no identifier stored; rate limit in memory only
```

## 7. API

```
POST /v1/uploads              { content_type, bytes } → 201 { upload_id, put_url (presigned, quarantine, 10 min) }
POST /v1/reviews/{target}     { rating, body, upload_ids?, rights_granted: true, email } → 202 { id }
POST /v1/feedback             { body, sentiment?, context?, contact? } → 204        (no id, no Set-Cookie)
GET  /v1/reviews/{target}?cursor=…        → approved items only
POST /v1/takedown             { item_id | url, reason, contact? } → 202
GET  /admin/moderation/queue?lane=1&limit=20     → claims 20 items with short-lived signed preview URLs
POST /admin/moderation/decisions  { item_id, decision: "approve"|"reject", reason }   Idempotency-Key
GET  /admin/moderation/links/{token}   → a confirmation page (GET never acts); POST on it approves or rejects
```

## 8. Data model

```sql
CREATE TABLE submissions (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id          text   NOT NULL UNIQUE DEFAULT new_public_id('u_'),
    kind               text   NOT NULL CHECK (kind IN ('review','photo')),
    target_id          bigint NOT NULL,
    body               text,
    rating             smallint CHECK (rating BETWEEN 1 AND 5),
    status             text   NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','approved','rejected','expired','taken_down')),
    lane               smallint NOT NULL DEFAULT 1,           -- 0 reports/high risk · 1 uncertain · 2 routine sample · 3 trusted sample
    due_at             timestamptz NOT NULL,                  -- SLA deadline: queue order, and auto-expire
    risk_score         smallint,
    email_hash         bytea,                                 -- HMAC(secret, normalised email); never the raw address here
    email_verified_at  timestamptz,
    verified_attendee  boolean NOT NULL DEFAULT false,
    rights_granted_at  timestamptz NOT NULL,
    claimed_by         bigint, lease_until timestamptz,       -- the moderator lease (task 15)
    decided_by         bigint, decided_at timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submissions_queue_idx ON submissions (lane, due_at) WHERE status = 'pending';

CREATE TABLE media_objects (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    submission_id bigint NOT NULL REFERENCES submissions(id),
    bucket text NOT NULL CHECK (bucket IN ('quarantine','public')) DEFAULT 'quarantine',  -- fail closed
    object_key text NOT NULL, sha256 bytea, width int, height int
);
CREATE TABLE moderation_decisions (                           -- append-only: who decided what, when, why
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, submission_id bigint NOT NULL,
    decision text NOT NULL, reason text, decided_by bigint, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE private_feedback (                               -- honeydolist migration 0012, the shape of a promise
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    context_type text CHECK (context_type IN ('site','event','job')),
    context_id   bigint,                                      -- deliberately NOT a foreign key
    body         text NOT NULL,
    sentiment    smallint CHECK (sentiment BETWEEN 1 AND 5),
    contact      text,                                        -- only if they choose to leave one
    status       text NOT NULL DEFAULT 'new' CHECK (status IN ('new','read','actioned','archived')),
    created_at   timestamptz NOT NULL                         -- store rounded to the hour (see 9.3)
);                                                            -- no ip, no user agent, no session, no created_by
```

## 9. Deep dives

### 9.1 Quarantine → scan → promote

| Where untrusted bytes go | Pros | Cons |
|---|---|---|
| Straight to the public bucket, moderate after | simplest | anyone hosts anything under your domain until a human gets to it |
| Through the API, which stores them | one place validates | 9 TB/day through app servers (104 MB/s) |
| **Presigned PUT into a quarantine bucket that is never public** | direct to storage; the *bucket policy* is the guarantee, so no bug in our code can serve it | validation happens after the upload, so the pipeline trusts nothing; orphans need a lifecycle rule |
| A separate account or network for quarantine | strongest blast-radius limit | operational cost |

**Choice and why.** Presigned PUT into quarantine, with the bucket's own policy saying "deny public read", plus a CI check that fetches a canary object anonymously and fails if it succeeds. Honeydolist's `access` column defaults to `private` and has a `quarantine` value, so a forgotten value fails closed, never open. Keys are random (see [02](02-pastebin.md)).

The pipeline's rules:
- **Re-encode every image; never serve the uploaded bytes.** One pass strips EXIF, neutralises polyglots (a JPEG that is also valid JavaScript) and normalises format and size. Read the EXIF *orientation* first and rotate the pixels, or photos turn sideways once the tag is gone. Strip GPS: holiday snaps carry coordinates.
- **Limits by content, not by name:** MIME from the sniffed bytes (`http.DetectContentType`), a byte cap, and a pixel cap read from the image *header* before any big allocation.
- **Memory by pixels:** decode cost depends on pixels, not file size. Spend a weighted semaphore for the memory *before* decoding; a photo that alone exceeds the budget is refused. This is both the decompression-bomb defence and the back-pressure when ten big photos arrive together.

```go
var budget = semaphore.NewWeighted(1 << 30) // decoded bytes in flight per pod; honeydolist uses 160 MiB on a 384 MB API

func reencode(ctx context.Context, src []byte) ([]byte, error) { // register decoders: image/jpeg, image/png, x/image/webp
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src)) // header only: cheap, before any big allocation
	if err != nil { return nil, ErrNotAnImage }
	if cfg.Width > 20_000 || cfg.Height > 20_000 || cfg.Width*cfg.Height > 50_000_000 { return nil, ErrTooLarge }
	cost := int64(cfg.Width) * int64(cfg.Height) * 4 // RGBA at least; 16-bit PNGs need more
	if cost > 1<<30 { return nil, ErrTooLarge }
	if err := budget.Acquire(ctx, cost); err != nil { return nil, err }
	defer budget.Release(cost)
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil { return nil, ErrNotAnImage }
	var out bytes.Buffer // apply the EXIF orientation to img here, before the tag is lost
	err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 82}) // brand-new bytes: no metadata, no appended payload
	return out.Bytes(), err
}
```

**Promote = copy, not move,** the re-encoded bytes into the public bucket under a content-addressed key (immutable URLs on a CDN, see [11](11-image-upload-hosting.md)), flip the row to `approved`, and invalidate caches: an idempotent step driven from the outbox. The original is deleted at the verdict, or after 7 days if rejected (appeals). Honeydolist's idea notes it: approved photos are copied into the same table as the site's own images, so the public gallery reads one place.

### 9.2 The queue, and how ten people cover five million items

| Policy | Pros | Cons |
|---|---|---|
| Pre-moderate everything by humans | safest | ≈ 1,157 moderators |
| Publish first, review reports | no delay | harmful content is live until someone reports it |
| **Hybrid by risk and trust** | fits the team; human minutes go where the uncertainty is | needs good scoring, sampling to catch drift, and an appeal path |
| Crowd flags only | cheap | slow, gameable |

**Choice and why.** A day of 5M items through lanes, with ten moderators able to do 43,200:

| Path | Share | Items/day | Decided by |
|---|---|---|---|
| Trusted submitters (verified email, history of approvals, no rejections) | 40% | 2,000,000 | auto-approve; humans sample 0.05% = 1,000 |
| Routine, low risk | 55% | 2,750,000 | auto-approve after checks; sample 0.1% = 2,750 |
| Uncertain (classifier band) | 0.7% | 35,000 | **humans, lane 1** |
| High risk | 4.3% | 215,000 | auto-reject (appealable) |
| Reports on published items | | ≈ 2,000 | **humans, lane 0, SLA 1 h** |

Human load ≈ 40,750 against 43,200 capacity: **94% utilised, no slack.** So the design needs a release valve: when the oldest item in a lane exceeds its SLA, *throttle intake* (the submit button says "temporarily unavailable") or tighten the auto-approve threshold, rather than let the queue grow without bound.

Mechanics: the queue is the `submissions` table and the partial index `(lane, due_at) WHERE status='pending'`. A moderator **claims** a batch with `FOR UPDATE SKIP LOCKED` and a lease (`lease_until`), so nobody sees the same item and an abandoned batch returns on its own ([15](15-job-scheduler.md)). Decisions are idempotent (`Idempotency-Key`, version check) and append-only.

**The failure nobody plans for: a queue nobody checks.** On a small site the owner is on a roof; the queue fills, nothing publishes, the feature looks broken and dies. Cures: a **batched digest** ("4 photos waiting", not one email per upload); **signed one-click links**, but a `GET` must only show a confirmation page and the action must be a `POST`, because mail scanners and link previews open links on their own; a visible queue count; **auto-expire** pending items after N days with a polite note (`expired`); SLA alerts on the oldest item per lane ([50](50-alerting-engine.md)).

Reviews add two rules from the ideas file: open a review only **after the event** it is about, and show **Verified attendee** when the reviewer's confirmed email matches a registration. Keep event reviews in their own table: a 5★ at a golf day must never average with a 5★ on a deck build. "Pre-moderation costs nothing at a few dozen a year" is exactly the assumption that stops holding.

### 9.3 Anonymity without a flood

| Rate-limit identity | Pros | Cons |
|---|---|---|
| IP stored per submission, like every other write | easy abuse investigation | a false promise on a page that invites candour; a leak or subpoena deanonymises |
| A cookie or session id | stops casual repeats | a linkable identifier; trivially cleared |
| **Short-lived in-memory counter keyed by a salted, coarse IP hash, never written to the database** | stops flooding without leaving a record | per-node memory; shared NATs collide; a restart forgets (acceptable) |
| Proof of work / challenge token | no identity at all | friction |
| No limit | simplest | a harassment vector |

**Choice and why** (the ideas file's posture): the counter lives in memory with a TTL of minutes. Make it more private than the idea strictly needs: **coarse** (IPv4 /24, IPv6 /48) so a hash identifies a neighbourhood, and a **salt generated at start-up and kept only in memory**, so even a memory dump after a restart cannot link a hash to yesterday's. At 1M active buckets × 50 B that is about 50 MB per node; with many nodes either accept N× the limit or use a shared in-memory store with persistence disabled and a TTL of minutes.

Everything else follows from the promise:
- **No `ip_hash`, user agent, session or `created_by`** on `private_feedback`; no id in the response (`204`); no `Set-Cookie`; no analytics event; the route is excluded from access logs, and the promise is worded precisely ("we don't record who sent this"), because the CDN and proxy still see addresses.
- **Round `created_at` to the hour:** an exact timestamp can be matched against any log that still has addresses.
- `context_id` is *not* a foreign key (it would tie the row to a record someone can cross-reference, and make it deletable by association); `contact` exists only if the sender chose to leave one.
- `feedback.read` is its own permission and not in the default staff grant: staff get the work, not the people.
- It *is* a harassment vector: one-click archive, a switch to turn it off, and copy that scopes it to "feedback about our work", since an employment complaint channel is a different legal context.

### 9.4 Takedown and appeal

| Approach | Pros | Cons |
|---|---|---|
| Delete the row and object | immediate | CDN edges keep serving until the TTL; no record for an appeal |
| **Soft unpublish + CDN purge, keep the bytes private for N days** | gone from the site at once; evidence kept; reversible | must purge every derived URL |
| Crypto-shredding (per-object keys) | removes it even from backups | complexity |

**Choice and why.** Unpublish and purge. A takedown route (a published address and `POST /v1/takedown`) and a one-click unpublish in the admin must exist **before launch**, not after the first complaint. Flow: report → lane 0 → decision within an hour → `status = taken_down` → purge. Tag every response and variant with a per-submission cache key (a surrogate key or cache tag, on CDNs that support purge-by-tag) so one call covers all sizes. Notify the submitter if they confirmed an email; allow an appeal that a *different* moderator reviews. Keep `rights_granted_at` and the decision log. Store the SHA-256 of taken-down images and refuse an identical re-upload (exact match; perceptual hashing is an extension). Repeat offenders: block the email hash.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (50M/day): the human queue, then decode CPU (≈ 260 cores at peak), then storage and CDN spend (≈ 5.2 PB a year). The queue breaks first because people are linear: at 10× the same ten moderators see 0.086%. Improve the classifier, tighten trust tiers, raise sampling only where drift appears.
- **Quarantine becomes public by mistake:** a misconfigured bucket is the worst failure. Account-level "block public access", the canary check, and keys that were random anyway.
- **Classifier down:** hold items (`pending`), do not auto-approve. A delayed publish is safe, an unreviewed one is not: fail closed here, the opposite of the rate limiter.
- **A moderator leaves mid-batch:** leases expire and the items return to the lane.
- **Email provider down:** digests wait; confirmation links are delayed; nothing is published without confirmation.
- **Review bombing:** per-target velocity limits; unverified reviews on a sudden spike go to lane 1 automatically.
- **Poisoned appeals/reports:** rate-limit the report button per reporter; weight by reporter history.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Quarantine bucket, policy-enforced | public bucket + moderate after | nothing unmoderated can be served | a promote step and orphan clean-up |
| Re-encode everything | serve originals | no metadata, polyglots or GPS leak | CPU, and a little quality |
| Risk lanes + trust tiers | human review of all | ten people cover 5M items | trust errors; sampling needed |
| In-memory salted coarse rate limit | stored IP hashes | real anonymity, still bounded floods | per-node accuracy, NAT collateral |
| Soft unpublish + purge | hard delete | instant, reversible, auditable | holding bytes for N days |
| `GET` confirms, `POST` acts | one-click `GET` approve | safe from link scanners | one extra click |

## 12. Check yourself

1. Why can ten moderators never "review the queue" at 5M items a day, and what does that force in the design?
2. Why re-encode, and why read the image header before decoding?
3. A signed "approve" link is emailed to the owner. What can go wrong with a `GET` link?
4. How do you rate-limit anonymous feedback without storing who sent it?
5. A published photo must come down now. List what has to happen.

<details><summary>Answers</summary>

1. One moderator does about 4,320 items a day (5 s each, 6 productive hours); ten do 43,200, which is 0.86% of 5M. Over 99% must be resolved by trust tiers, auto-approve with sampling, and auto-reject, and the queue's ordering, SLAs and an intake throttle become the design.
2. Re-encoding produces brand-new bytes: EXIF and GPS are gone and a polyglot file cannot survive; the orientation is applied first so photos are not rotated. The header gives dimensions cheaply, so a decompression bomb (a tiny file that decodes to hundreds of MB) is refused before any large allocation, and the pixel count sizes the memory reservation.
3. Mail security scanners and link previews fetch links automatically, so a `GET` that acts would approve or reject by itself. The link should open a confirmation page; the action is a `POST`, with a single-use, expiring, single-item token.
4. Keep a counter in memory, keyed by a coarse (/24 or /48) hash with a start-up salt held only in memory, with a TTL of minutes. Never write it to the database or logs; return `204` with no id or cookie; round stored timestamps to the hour.
5. Set `status = taken_down`, purge the CDN by the item's cache tag so every variant disappears, keep the bytes private for the appeal window, notify the submitter if possible, record the decision, and block re-upload of the same hash.

</details>

## 13. In Go

- Related lessons: [57 Web security](../../57-web-security.md) (upload handling, signed links, CSRF on one-click actions), [44 Background jobs & queues](../../44-background-jobs-queues.md) (pipeline workers), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED` claims, partial indexes), [43 Authorization & RBAC](../../43-authorization-rbac-multitenancy.md) (`feedback.read` as a permission), [15 Sync & context](../../15-sync-context.md) (the weighted semaphore, cancellation), [34 Outbox](../../34-event-driven-outbox.md) (promote as an idempotent step).
- Use `http.DetectContentType` on the first 512 bytes, `image.DecodeConfig` for the header, `golang.org/x/sync/semaphore` for the memory budget, `crypto/hmac` for the email hash and signed tokens (compare with `hmac.Equal`, include an expiry).
- Packages: `aws-sdk-go-v2/service/s3` (presign, copy), `golang.org/x/image/webp`, `pgx/v5`, `golang.org/x/sync/semaphore`.

## 14. Extensions

- **Perceptual hashing** for near-duplicate and re-upload detection; a shared industry hash list for illegal imagery (take legal advice on reporting duties).
- **ML classifier with human labels:** moderators' decisions retrain it; what drift metric tells you to re-sample?
- **Reviewer reputation:** trust tiers that move with outcomes, and decay.
- **Video:** transcoding, per-frame moderation, bandwidth (medium track: video upload & streaming).
- **Transparency report and notice-and-action flows** for regulated markets.
- **Crowd moderation:** weighted flags from trusted users as an extra lane-0 input.
