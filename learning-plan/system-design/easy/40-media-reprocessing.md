# 40 · Media Reprocessing at Scale

`🟢 easy` · *Topics:* originals as the source of truth, variants keyed by original + recipe, idempotent re-encode jobs, backfills as range tasks with checkpoints, priority lanes and throttling, video transcoding with leases and heartbeats, run tracking and rollback by pointer · *Builds on:* [11 Image Upload & Hosting](11-image-upload-hosting.md) (variants, immutable URLs), [15 Job Scheduler](15-job-scheduler.md) (leases, `SKIP LOCKED`), [03 Rate Limiter](03-rate-limiter.md) (throttling the backfill)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [39 Global Search with Permission Filtering](39-global-search.md) · Next: [41 Backup & Restore](41-backup-restore.md)

---

> 🌱 **Seed (honeydolist-site):** a library photo is kept as uploaded in a private bucket, and the worker derives WebP copies at 480 / 960 / 1,600 px and its own size (PLAN step 39.2, migration 0061). *Optimize all* and *Re-optimize all content* queue every file to be made again, always from the original, under new keys, with the old objects deleted an hour later through `file_garbage` (steps 40 and 43, migrations 0062–0063). Phone videos are re-made as H.264 MP4 by a separate `transcoder` container: 1 GB, 1 CPU, one video at a time, 20 min cap (step 41.2, migration 0064). Measured there: 1,600 → 1,200 px takes ~45 % off a photo; WebP is 15–20 % smaller than JPEG at the same setting.
> **Here, at scale:** 100M photos and 5M videos; the owner changes the quality rules and the whole library is redone without downtime at a fleet speed of 50k images a minute, while new uploads still get their sizes in seconds.

## 1. The task

> "Design the media pipeline behind a photo and video library. When the quality or size rules
> change, every existing photo and video must be made again from its original: no downtime, no
> broken links, and new uploads must not wait behind it."

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Fleet size, run length. | 100M photos (avg original 3 MB), 5M videos (avg 1 min, 100 MB); +200k photos/day, +50k videos/day. |
| Are originals always kept? | Decides what a "redo" can start from. | Yes for everything uploaded after launch; legacy files may have none. |
| How fast must a new upload get its sizes? | Sets the live lane's SLO. | Photos p99 < 10 s; videos < 10 min. |
| How often do the rules change? | Rare, but a bad run must be undoable fast. | A few times a year; rollback in minutes, not a second 33-hour run. |
| Do old links keep working during and after a run? | Cached pages name old URLs. | Yes, for as long as a page that names them can exist. |

## 3. Requirements

**Functional**
- A *recipe* (sizes, qualities, encoder version) is an immutable, versioned record; the current one applies to new uploads.
- *Start a run*: re-derive every asset under a target recipe, always from its original; *catch-up* mode skips a file that would not get smaller, *force* applies the recipe regardless.
- Pause, resume, stop; progress, failures and bytes saved per run; *rollback* to the previous recipe's files.

**Non-functional**
- New uploads are never delayed more than seconds by a backfill; a re-run, crash or retry produces the same objects.
- Public URLs are immutable; a decode bomb or corrupt file fails one asset, never a worker or a run.

**Out of scope:** the upload path ([11](11-image-upload-hosting.md)), live streaming, DRM, content analysis.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Originals (photos) | 300 TB | 100M × 3 MB |
| Derived set per photo | ~0.5 MB → ~50 TB | 4 WebP sizes + JPEG fallback + thumbnail (assumption); originals are ~6× the derived set |
| Fleet speed, full photo backfill | 50k/min ≈ 833/s → ≈ 33 h | 100M ÷ 50k per min = 2,000 min |
| CPU per photo | ~0.5 CPU-s | decode 12 MP, resize ×4, encode ×5, native encoders (assumption) |
| Cores for full speed | ≈ 417 busy cores, ≈ 13.9k CPU-hours in all | 833/s × 0.5 s; ≈ 52 eight-core nodes at 100 %, ~70 at 75 % |
| Bytes in / out | ≈ 2.5 GB/s in (20 Gbit/s), 0.4 GB/s out | 833 × 3 MB read, 833 × 0.5 MB written |
| Object requests | ≈ 5k PUT/s + 833 GET/s | 6 PUT + 1 GET per photo |
| Live uploads | ~2.3/s avg, ~10/s peak | 200k/day ÷ 86,400 |
| Videos: originals, full redo | 500 TB; ≈ 125k CPU-hours ≈ 5.2 days on 1,000 cores | 5M × 100 MB; ~90 CPU-s per 1-min 1080p video, 5M × 90 s = 450M CPU-s |
| Live video | ≈ 52 cores steady, ~160 at 3× peak | 50k/day × 90 CPU-s = 4.5M CPU-s/day |

What the numbers tell us: the backfill is a day-and-a-half batch job next to a trickle of 2–10
live uploads a second. The design problem is **fairness, idempotence and reversibility**, not raw
capacity. Originals dominate storage (cold-tier them). 5k PUT/s is above one prefix's write rate
(README cheat sheet: ~3,500/s), so keys must fan out over prefixes. Video is CPU-*days*, a different
beast: its own lane, its own fleet, and a rule for when a redo is not worth it.

## 5. Plan

1. What is kept forever, what is rebuildable, how derived keys are named.
2. Where per-asset state lives, and how the backfill finds work, checkpoints and resumes.
3. Fairness: live lane vs backfill lane, throttling, backpressure.
4. Video: long jobs, leases, heartbeats, sizing.
5. Runs: progress, canary, cost, rollback, garbage collection.

## 6. High-level design

```
 upload ──► API ──► object store: ORIGINALS (private, versioned, cold tier after 90 d)
              │ INSERT media_assets + outbox 'media.uploaded'
              ▼
   ┌──────────────── Postgres job tables (SKIP LOCKED claims) ────────────────┐
   │ LIVE lane:     one job per new asset              (priority, own slots)   │
   │ BACKFILL lane: range tasks [lo,hi) of 1,000 ids per run (rate-limited)    │
   └───────────────┬──────────────────────────────────────┬───────────────────┘
                   ▼                                      ▼
        photo workers (~70 × 8-core)            video workers (1 ffmpeg per core)
        GET original → derive → PUT variants     lease + heartbeat, tmpfs scratch
        key = v/<recipe>/<sha[:2]>/<sha>/<w>.webp   (same key = same bytes)
                   │ upsert media_variants(asset_id, recipe_id)  ← the pointer swap
                   ▼
   media_variants (narrow) ──► API/SSR build `sources` from it ──► CDN (immutable URLs)
   reprocess_runs / reprocess_ranges: progress, pause, stop, rollback
   retire sweeper: when recipe R1 is retired and its grace period is over, expire prefix v/R1/
```

**A live upload:** the API stores the original and a job; a worker derives the current recipe's files and
upserts `media_variants`; the next read returns the new `sources`. **A backfill:** the admin saves recipe R2
and starts a run; ranges are claimed, each asset is derived from its original under R2 and its pointer flips;
pages move to R2 URLs as their caches expire, and R1's files stay until the grace period ends.

## 7. API

```
PUT  /admin/media/recipes   {kind, params}  → 201 { recipe_id }    immutable; starts nothing
POST /admin/media/runs      { recipe_id, mode: "catch_up"|"force", sample_pct: 1..100 }
                                            → 202 { run_id, estimate: { assets, cpu_hours } }
GET  /admin/media/runs/{id}                 → 200 { status, total, done, skipped_no_original, skipped_same,
                                                    failed, bytes_before, bytes_after }
POST /admin/media/runs/{id}/pause | resume | stop   → 200
POST /admin/media/runs/{id}/rollback        → 202 { run_id }       pointer swap, no re-encoding
GET  /admin/media/runs/{id}/failures        → 200 [ { asset_id, error, attempts } ]
GET  /photos/{id}                           → 200 { sources: [ { url, width } ], … }    from media_variants
```

Saving a recipe and starting a run are separate calls: nothing runs from unsaved settings (as in the repo).

## 8. Data model

```sql
CREATE TABLE media_recipes (
    id          integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text  NOT NULL CHECK (kind IN ('photo', 'video')),
    params      jsonb NOT NULL,   -- {"widths":[480,960,1600],"webp_q":60,"jpeg_q":70,"encoder":"libwebp 1.4"}
    params_hash text  NOT NULL,   -- first 16 hex of sha256(canonical params); goes into object keys
    UNIQUE (kind, params_hash)
);
CREATE TABLE media_assets (       -- the "files" table, only the columns that matter here
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            text NOT NULL CHECK (kind IN ('photo', 'video')),
    original_key    text,         -- private bucket; NULL = legacy file with nothing to redo from
    original_sha256 bytea,
    deleted_at      timestamptz
);
CREATE INDEX media_assets_sha_idx ON media_assets (original_sha256) WHERE original_sha256 IS NOT NULL;

CREATE TABLE media_variants (     -- narrow side table: a backfill rewrites this, never the wide asset row
    asset_id       bigint PRIMARY KEY REFERENCES media_assets(id),
    recipe_id      integer NOT NULL REFERENCES media_recipes(id),
    prev_recipe_id integer REFERENCES media_recipes(id),  -- previous set, kept for rollback until retired
    widths         integer[] NOT NULL,
    bytes          bigint NOT NULL,                       -- size of the derived set
    made_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_variants_recipe_idx ON media_variants (recipe_id, asset_id);

CREATE TABLE reprocess_runs (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipe_id  integer NOT NULL REFERENCES media_recipes(id),         -- the target
    mode       text NOT NULL CHECK (mode IN ('catch_up', 'force', 'rollback')),
    sample_pct smallint NOT NULL DEFAULT 100 CHECK (sample_pct BETWEEN 1 AND 100),
    status     text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'paused', 'stopped', 'done')),
    started_by bigint NOT NULL, started_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE reprocess_ranges (   -- 100M ids / 1,000 = 100,000 rows per run
    run_id      bigint NOT NULL REFERENCES reprocess_runs(id) ON DELETE CASCADE,
    lo bigint NOT NULL, hi bigint NOT NULL,                           -- asset ids [lo, hi)
    status      text NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'leased', 'done')),
    lease_until timestamptz,
    cursor      bigint NOT NULL,                                      -- next id to look at: the resume point
    done int NOT NULL DEFAULT 0, skipped_no_original int NOT NULL DEFAULT 0,
    skipped_same int NOT NULL DEFAULT 0, failed int NOT NULL DEFAULT 0,
    bytes_before bigint NOT NULL DEFAULT 0, bytes_after bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, lo)
);
CREATE INDEX reprocess_ranges_open_idx ON reprocess_ranges (run_id, lo) WHERE status <> 'done';
-- reprocess_failures (run_id, asset_id, attempts, error) holds only the assets that failed.
```

## 9. Deep dives

### 9.1 Originals forever, variants as a pure function

The original is the only thing that cannot be rebuilt. Everything else (sizes, thumbnails, the
MP4) is a function of `(original, recipe)`. Three ways to name the derived files:

| Option | Pros | Cons |
|---|---|---|
| **Overwrite the same key** | trivial; no garbage | a CDN or browser may hold the old bytes under that URL; a crash mid-PUT mixes versions; no rollback |
| **New random key per run** (what the repo does at small scale, then `file_garbage` deletes the old ones after an hour) | no stale caches | a re-run after a crash makes a *second* copy; garbage needs a row per object (600M rows at 6 objects × 100M); rollback means re-encoding |
| **Key = f(original sha256, recipe hash)**: `v/<recipe>/<sha[:2]>/<sha>/<w>.webp` | deterministic: a re-run finds the object already there and skips the encode; a crash is harmless; rollback is a pointer; whole retired recipes expire by prefix | the encoder version must be inside the recipe hash (a new libwebp changes bytes); needs an index to know whether another asset shares an original |

**Choice and why: deterministic keys.** The worker `HEAD`s the target keys first, so "run it twice" costs
one request. The upsert flips `media_variants` to the new recipe and keeps the old one in `prev_recipe_id`;
old URLs keep working until the recipe is *retired* (9.5). Two assets with the same original share keys (free
dedup, [11](11-image-upload-hosting.md) §9.6), so deleting an asset deletes derived objects only when no other
asset has the same `original_sha256` (`media_assets_sha_idx`). Two rules from the repo matter more at scale:
**derive only from the original** (each lossy pass stacks losses; an asset with `original_key IS NULL` is
skipped and counted), and give workers *read-only* access to the originals bucket, so a backfill bug cannot
delete the one irreplaceable thing.

### 9.2 Where state lives, and how a backfill finds work

At 100M rows the question is not "how do I find the files to redo" but "what does touching them cost".

| Option | Pros | Cons |
|---|---|---|
| **Flag/version columns on the wide asset row + a partial index** (the repo: `variants IS NULL`, `optimize_requested_at`, `video_queued_at`) | one table; tiny code | 100M updates of a wide, hot row: new tuple versions, index churn, vacuum pressure, and triggers so "not an edit" does not bump the row's version |
| **A materialised items table per run** (100M rows of `(run, asset)`) | exact per-item status | 100M inserts to start a run, the same again to delete it |
| **Range tasks over a narrow side table** | 100k task rows per run; per-range checkpoint; parallel, resumable; asset rows untouched | per-item results are aggregated, not stored (failures are the exception) |

**Choice: range tasks + `media_variants`.** Starting a run inserts `ceil(max(id) / 1000)` ranges.
A worker claims one with a lease:

```sql
UPDATE reprocess_ranges r
   SET status = 'leased', lease_until = now() + interval '10 minutes'
 WHERE (r.run_id, r.lo) = (
        SELECT run_id, lo FROM reprocess_ranges
         WHERE run_id = $1 AND (status = 'todo' OR (status = 'leased' AND lease_until < now()))
         ORDER BY lo LIMIT 1
           FOR UPDATE SKIP LOCKED)
RETURNING lo, hi, cursor;
```

It walks `[cursor, hi)`: skip an asset already on the target recipe, count and skip one with no original,
otherwise derive and upsert; every 50 assets it writes `cursor` and the counters back and extends the lease.
Crash: the lease expires and another worker resumes at `cursor`; at worst 50 assets are looked at again and
skipped. Run totals are the *sum over ranges* (like the sharded counters of [06](06-view-like-counter.md)),
never one hot row updated per asset. A straggler sweep afterwards (`media_variants_recipe_idx`: rows still on
an old recipe) catches assets created or failed during the run.

### 9.3 Fairness: live uploads must not queue behind a 33-hour backfill

| Option | Pros | Cons |
|---|---|---|
| **One FIFO for everything** | simplest | a new upload waits behind up to 100M jobs |
| **Strict priority, one pool** | live always first | a burst of live jobs starves the backfill forever; a long video blocks slots |
| **Separate lanes + reserved capacity + a throttle + automatic backpressure** | live is isolated; the backfill speed is a dial | more knobs |

**Choice: lanes with reserved capacity.** Workers take from the live lane first. The backfill may use
at most ~80 % of slots and passes through a token bucket ([03](03-rate-limiter.md)) whose rate is a
runtime setting (50k/min is the maximum, not the default), so someone can turn it down at 3 pm and
up at midnight. **Backpressure turns the dial by itself:** live-lane age p99 > 10 s, object-store 5xx
above 1 %, or replica lag above a limit all pause new backfill claims (the lease on ranges in flight
keeps running). Fixed rules: keys fan out over 256 hash prefixes (`sha[:2]`), so ~5k PUT/s is ~20/s per prefix;
memory is sized by pixels, not file size (a 48 MP photo decodes to ~192 MB RGBA: cap the decoded pixel count
and size a worker at `concurrency × ~250 MB`); a poison file increments `failed`, is written to
`reprocess_failures` after 3 attempts with backoff, and the range moves on.

### 9.4 Video: long jobs need leases and heartbeats

A video takes minutes of CPU (the repo: "a whole CPU for a minute or two", 20 min at most). A photo-sized
lease (10 s) expires mid-job and a second worker starts the same video; a worst-case lease (30 min)
leaves a crashed job dead for half an hour. **Short lease + heartbeat:** lease 5 min, extended every 30 s
while ffmpeg runs; a failed extension kills ffmpeg (the job is no longer ours).

```go
func (w *Worker) transcode(ctx context.Context, t Task) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { // keep the lease while ffmpeg runs; give up if it is lost
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if ok, err := w.store.ExtendLease(ctx, t.ID, 5*time.Minute); err != nil || !ok {
					cancel() // CommandContext kills ffmpeg
					return
				}
			}
		}
	}()
	return exec.CommandContext(ctx, "ffmpeg", t.Args()...).Run()
}
```

| Option | Pros | Cons |
|---|---|---|
| In the general worker | no new service | ffmpeg's 1 GB and a whole CPU starve mail and invoices (the repo's reason for a separate container) |
| **A video fleet: one ffmpeg per core slot, 1 GB + tmpfs scratch each** | isolated, sized by CPU, scales by adding nodes | scheduling; ~70 MB static ffmpeg in the image |
| A managed transcoding service | no ffmpeg ops | per-minute cost on 5M videos; less control of the recipe |

**Choice: the fleet**, the repo's container multiplied: ~160 slots for the live peak, ~1,000 borrowed for a
full redo (5.2 days). Decide *per video*: skip when it is already an H.264 MP4 inside the bitrate budget (the
repo: 3 / 6 / 12 Mb/s at 720 / 1080 / above) or the result would not be smaller; *force* overrides. A PUT
appears whole or not at all, so a retaken job overwrites the same deterministic key and the pointer upsert
decides. The hard part is the source: iPhones record **HEVC, often HDR**, and converting that to 8-bit H.264
without tone mapping gives washed-out colour, so the recipe needs a tone-mapping chain (ffmpeg's
`zscale`/`tonemap`) and a test with real phone clips. One progressive MP4 (`+faststart`) per video now; an
HLS ladder multiplies CPU by the number of renditions (extension).

### 9.5 Runs: progress, canary, cost, rollback, garbage

| Question | Option | Choice |
|---|---|---|
| Per-item results? | a row per asset per run | **counters per range + `reprocess_failures` only for failures**: 100M result rows is a second library |
| Proving the recipe is good | run everything, look at the result | **canary first**: `sample_pct = 1` (assets where `id % 100 = 0`), compare bytes and a similarity score (SSIM) on the sample, then run the rest |
| Undo | re-encode the old recipe (33 more hours, old bytes' quality gone if the old files were deleted) | **pointer rollback**: a `mode = rollback` run swaps `recipe_id` ↔ `prev_recipe_id` per range, no encoding, minutes of DB work |
| Delete old files | per-object `DELETE` rows (the repo's `file_garbage`) | **retire a recipe**: after a grace period of days (longer than any cached page; it is also the rollback window) expire the whole prefix `v/<recipe>/` with a lifecycle rule |

The grace period costs space: R1 beside R2 for 7 days is up to ~50 TB more (+14 % of ~350 TB) for a week,
the price of a one-click undo. The admin sees `done / total` summed over ranges, *bytes saved*
(`bytes_before − bytes_after`), and `estimate.cpu_hours` before pressing start.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (1B photos):** the backfill takes 14 days, so rules can change faster than a
  run finishes. Runs must be supersedable: starting R3 cancels R2's open ranges and targets R3. Then PUT/GET
  rates (more prefixes) and the 100M-row upsert stream into `media_variants` (batch it, keep it narrow).
- **A swap warms the CDN gradually:** a changed asset has a new URL, so its first view is a miss to origin.
  A 33-hour swap is a natural ramp; a pointer rollback flipping everything in minutes is not, so ramp it by range.
- **Single points of failure:** the job tables' Postgres (pausing a backfill is harmless, the live lane
  stopping is not: it also reads an outbox and survives a restart) and the originals bucket.
- **Worker dies mid-range or mid-video:** the lease expires, another worker resumes at `cursor` or restarts the
  transcode; deterministic keys make both safe. **Object store throttles (503):** backoff with jitter; the
  backpressure signal pauses claims.
- **Bad recipe shipped to 100 %:** pointer rollback, then fix and canary again. **Original missing or corrupt:**
  counted and listed, current variants kept; never fall back to re-encoding a derived file.
- **Watch:** live-lane age p99, backfill rate vs its setting, failures by error class, bytes saved, CDN
  origin-miss rate during a swap, worker memory high-water mark.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Originals forever, read-only to workers | keeping only the "optimised" file | any recipe can be redone or undone; no generation loss | 300 TB (cold tier) |
| Key = f(original, recipe) | new random key per run | idempotent re-runs, rollback by pointer, prefix expiry | recipe hash must include the encoder version |
| Narrow `media_variants` side table | flags on the asset row | cheap rewrites, no "not an edit" triggers | one more table to join |
| Range tasks per run | per-item queue rows | 100k rows, checkpoints, parallel claims | per-item results only for failures |
| Lanes + reserved capacity + backpressure | one FIFO | live uploads stay fast; speed is a dial | more knobs to tune |
| Pointer rollback with a grace window | re-encode to undo | undo in minutes | ~14 % extra storage for a week |

## 12. Check yourself

1. Why must every redo start from the original and never from the previous variant?
2. Why is the variant key a function of (original hash, recipe hash) instead of a random key per run, and what must be inside the recipe hash?
3. Why range tasks over a side table instead of `UPDATE files … WHERE variants_version < current`?
4. How do you keep a 33-hour backfill from delaying new uploads?
5. A transcode worker dies after 12 minutes of a 15-minute job; a week later the new recipe turns out too blurry. What happens in each case?

<details><summary>Answers</summary>

1. Every lossy pass loses detail, so deriving from a derived file stacks losses that can never be undone;
   the original is the only lossless source. An asset with no original is skipped and counted.
2. A deterministic key makes the job idempotent: a retry finds the object there and skips the encode, a crash
   cannot leave a mixed state, old URLs stay valid because nothing is overwritten, rollback is a pointer swap.
   The recipe hash must contain everything that changes the bytes: sizes, qualities and the **encoder version**.
3. The `UPDATE` rewrites 100M wide rows (new tuple versions, index churn, vacuum, triggers to avoid bumping
   the edit version) and has no natural checkpoint. Range tasks over a narrow table are 100k rows, each
   resumable from its `cursor`, claimable in parallel, with the asset rows untouched.
4. Separate lanes with the live lane first, a cap on the backfill's share of slots, a token-bucket rate that is a
   runtime setting, and backpressure that pauses backfill claims when live-lane age, store errors or replica lag cross a limit.
5. The lease stops being extended, expires within five minutes, and another worker restarts the transcode from
   the original; the output key is deterministic, so the half-finished attempt is just overwritten. For the blurry
   recipe, a rollback run swaps `recipe_id` with `prev_recipe_id` per asset, with no encoding, as long as the grace
   period has not retired the old prefix.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) (`errgroup` with a limit for worker concurrency, `CommandContext`, cancelling on a lost lease), [44 Background jobs](../../44-background-jobs-queues.md) (lanes, retries, dead letters), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED`, leases, upserts with a guard), [68 Rate limiting & throttling](../../68-rate-limiting-throttling.md) (a token bucket as the backfill dial), [55 Data pipelines](../../55-data-pipelines.md) (batch stages, checkpoints), [36 Resilience patterns](../../36-resilience-patterns.md) (backoff with jitter).
- Image work: `golang.org/x/image`, `image/jpeg`, a WebP encoder (the repo uses `github.com/gen2brain/webp`, libwebp compiled to WebAssembly: pure Go, no cgo, but several times slower than native, which is why this task's 0.5 CPU-s per photo assumes native). Decode with `image.DecodeConfig` first to read the pixel count and refuse bombs before allocating.
- Storage: `github.com/minio/minio-go/v7` (any S3-compatible store); `StatObject` for the "already there?" check; `pgx` for the claim query.
- ffmpeg is a child process: `exec.CommandContext`, stderr captured and truncated into `reprocess_failures.error`, scratch on a tmpfs sized into the container's memory limit.

## 14. Extensions

- **HLS ladder:** 360p/720p/1080p renditions, segment storage, a manifest per video; CPU ×2–3 and a CDN story (medium: video upload & streaming).
- **Content-aware recipes:** per-asset quality search (lowest quality above a similarity target) instead of one global number.
- **Cold-tier originals:** lifecycle to an archive class after 90 days; a redo then reads from slow storage, so schedule it and batch the retrievals.
- **Multi-region:** originals in one region, derived files replicated or regenerated per region from a regional cache.
