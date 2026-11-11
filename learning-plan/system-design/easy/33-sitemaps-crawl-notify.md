# 33 · Sitemaps & Crawl Notifications

`🟢 easy` · *Topics:* sitemap index and protocol limits, sharding and why "regenerate only dirty shards" fails the arithmetic, truthful `lastmod` from a content hash, change pings with a coalescing queue (IndexNow), robots.txt and noindex hygiene, what may be listed · *Builds on:* [10 Webhook Delivery](10-webhook-delivery.md) (outbound batches, backoff), [15 Job Scheduler](15-job-scheduler.md) (scheduled rebuilds), [32 Block CMS](32-block-cms.md) (where "published" comes from)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [32 Block-Based CMS](32-block-cms.md) · Next: [34 Crawler-Ready SPA Serving](34-crawler-ready-spa.md)

---

> 🌱 **Seed (honeydolist-site):** Step 67 built `/sitemap.xml` and a *generated* `robots.txt`. The sitemap lists the
> fixed pages minus the `noindex` set (one shared list, so a page can't be in one place and not the other) and every
> **live** record; each `<loc>` must answer 200 with a canonical equal to itself (a test walks them all). `lastmod` is
> the record's last edit, no `changefreq`/`priority` (Google ignores both), lead photos as `<image:image>`; it stays one
> file until 50,000 addresses, then an index; kept 5 min in memory and dropped on every save; `404` on any host that
> isn't the public one. `robots.txt` repeats `Disallow: /dashboard/` in **every** user-agent group (a crawler obeys only
> the most specific group), must *not* block `/api/`, `/assets/`, `/media/` (the renderer needs them) and leaves noindex
> pages crawlable (a crawler that can't fetch a page can't see its `noindex`). Step 73 added **IndexNow** (migration
> 0089): after any save a job is queued with a 30 s gather window, the worker *compares* the live list with
> `indexnow_urls` and submits what is new, changed or gone, once each, in batches of 10,000, retrying `429`/`5xx`.
> Step 73.5 fixed an untrue `lastmod`: a price lived in its own row and never touched the parent's timestamp. The site
> has a few hundred URLs. **Here, at scale:** a marketplace with 50M public URLs and 2M URL changes a day.

## 1. The task

> "Design the system that tells search engines what exists on our site and what just changed. 50 million public pages,
> about two million change every day. It must never list a page that isn't indexable, and a new page should be
> discoverable within minutes."

The mechanics are simple XML. The task is about *what is true* (which URLs, which `lastmod`), how to build 50M entries
without a sweat, and how to announce changes without becoming noise to the engines.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| One host or many? | Sitemaps and `robots.txt` are per host. | One big host; multi-tenant per-host sitemaps are an extension. |
| Which engines? | Notification options differ. | Google (sitemap + Search Console), Bing and others (sitemap + IndexNow). |
| How fresh? | Decides pings vs polling. | New pages discoverable in minutes; removals within hours. |
| What is "indexable"? | The one predicate everything shares. | Published, not `noindex`, not redirected, canonical to itself, not blocked. |
| Images, video, hreflang in the sitemap? | Size and builder complexity. | Lead image only; no video/hreflang yet. |
| How many changes/day, and how many are *real*? | Sizes the ping queue. | 2M edits ≈ 1.2M distinct URLs/day (assumption); many are not content changes. |
| Do all pages deserve a ping? | Pings can be abused into noise. | No: only changes to indexable content. |
| Scale of crawler traffic to the sitemap itself? | Origin load. | Tiny, but put it behind the CDN anyway. |

## 3. Requirements

**Functional**
- `/robots.txt`, `/sitemap.xml` (an index) and the sitemap files it names, exactly the indexable URLs.
- A truthful `<lastmod>` per URL: it moves **only** when indexable content changed.
- Announce adds, changes and removals (IndexNow for engines that accept it); dedupe edit bursts.
- Remove a URL from the sitemap the moment it stops being indexable.

**Non-functional**
- Obey the protocol: ≤ 50,000 URLs and ≤ 50 MB uncompressed per sitemap file; ≤ 50,000 sitemaps per index.
- Rebuilding everything must be cheap enough to do on a schedule; a build never serves a half-written file.
- A bad deploy must not put `Disallow: /` or a `5xx` on `robots.txt` (crawlers treat that as a site-wide signal).

**Out of scope**
- Crawl-budget tuning per path ([34](34-crawler-ready-spa.md) covers serving), ranking, Search Console automation.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| URLs | 50M | given |
| Files needed | ≥ 1,000 | 50M ÷ 50,000 per file |
| Bytes per entry | ~200 B | `<loc>` ~90 B + `<lastmod>` + tags; ~350 B with an image |
| One file | ~10 MB raw, ~1–2 MB gzipped | 50,000 × 200 B; XML compresses ~5–10× |
| All files | ~10 GB raw, ~1–2 GB gz | 1,000 × 10 MB |
| Index file | ~100 KB | 1,000 entries × ~100 B |
| Changes | ~23/s avg, ~115/s peak | 2M ÷ 86,400 ≈ 23; peak ×5 |
| Distinct changed URLs | ~1.2M/day | assumption: 60% after dedupe |
| IndexNow batches | ≥ 120/day | 1.2M ÷ 10,000 per request |
| Full rebuild cost | ~250 s on one worker, ~30 s on eight | 50M rows ÷ ~200k rows/s/worker (keyset scans of a narrow table) |
| Edits per shard per day | ~2,000 | 2M ÷ 1,000 shards, if edits are spread evenly |

What the numbers tell us: **volume is not the problem**: 23 changes/s is a trickle, and a full rebuild is half a
minute on a handful of workers. The risks are *correctness* (listing the wrong URLs, a lying `lastmod`) and
*politeness* (pinging noise). The "clever" optimisation, regenerating only the shards that changed, is worth checking
against the arithmetic before building (§9.2).

## 5. Plan

1. One predicate: "indexable", computed once, stored, used by the page, the sitemap and the pings.
2. A narrow projection table (`seo_urls`) so builds never scan content tables.
3. Shard layout and the index; the build job; atomic publication of files.
4. Truthful `lastmod`.
5. The change queue and pings.
6. `robots.txt` and the guard rails (verification, never list junk).

## 6. High-level design

```
 publish / edit / unpublish / redirect (events via outbox, task 10)
        │
        ▼
 seo projector ──► seo_urls (url_id, path, lastmod, content_hash, listed, shard)        ← one row per URL, narrow
        │                └─► crawl_queue (path PK, changed_at, kind)  UPSERT = coalesce 20 edits into 1 row
        │
 builder (scheduled, parallel: 1 worker per shard range)                 pinger (every ~30–60 s)
   stable shards by id range:  sitemaps/p-0000.xml.gz … p-0999.xml.gz      drain rows older than the debounce window
   fresh files: sitemaps/fresh-YYYYMMDD-NN.xml.gz (changed in last 48 h)   POST ≤10,000 URLs to IndexNow, backoff on 429/5xx
        │  write to object storage under a new key, then flip the index
        ▼
 /sitemap.xml (index, ETag, 304)  ◄── CDN ◄── crawlers        /robots.txt  (generated, tested, cached short)
        ▲
 sampled verifier: fetch 0.1% of listed URLs/day → must be 200, self-canonical, no noindex, not blocked
```

## 7. API

```
GET  /robots.txt                       → 200 text/plain (generated; never 5xx on a data error: serve last good)
GET  /sitemap.xml                      → 200 <sitemapindex> (≤ 50,000 <sitemap> entries), ETag, Last-Modified
GET  /sitemaps/p-0042.xml.gz           → 200 <urlset>, ≤ 50,000 <url>, ≤ 50 MB uncompressed; 304 on If-None-Match
GET  /{indexnow-key}.txt               → 200 the key (engines fetch it to verify we own the host)
POST https://api.indexnow.org/indexnow (outbound)
     { "host": "example.com", "key": "…", "keyLocation": "https://example.com/{key}.txt", "urlList": ["https://…", …≤10,000] }
GET  /admin/seo/status                 → { listed, shards: [{ shard, built_at, urls }], queue_depth, last_ping, verifier: { sampled, failed } }
```

## 8. Data model

```sql
CREATE TABLE seo_urls (
    url_id       BIGINT PRIMARY KEY,                  -- = the page id; stable
    path         TEXT   NOT NULL UNIQUE,
    shard        INTEGER NOT NULL,                    -- url_id / 50000 (stable id-range shard)
    listed       BOOLEAN NOT NULL,                    -- THE indexable predicate, written by the projector only
    lastmod      TIMESTAMPTZ NOT NULL,                -- moves only when content_hash changes
    content_hash BYTEA   NOT NULL,                    -- hash of what a crawler can see: title, description, body text, structured data, lead image
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX seo_urls_shard_idx   ON seo_urls (shard, url_id) WHERE listed;       -- a shard's build is one ordered scan
CREATE INDEX seo_urls_changed_idx ON seo_urls (changed_at) WHERE listed;          -- the "fresh" files

CREATE TABLE seo_shards (
    shard       INTEGER PRIMARY KEY,
    object_key  TEXT NOT NULL,                        -- e.g. sitemaps/p-0042.xml.gz (the current build)
    url_count   INTEGER NOT NULL,
    etag        TEXT NOT NULL,
    built_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE crawl_queue (
    path        TEXT PRIMARY KEY,                     -- one row per URL: an edit burst is ONE row
    kind        TEXT NOT NULL CHECK (kind IN ('upsert', 'removed')),
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),   -- last change; the debounce is measured from here
    attempts    INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX crawl_queue_due_idx ON crawl_queue (next_attempt_at);
```

The projector keeps `seo_urls` current from publish/unpublish/redirect events; **nothing else decides `listed`**. The
sitemap builder, the pinger and the verifier all read it.

## 9. Deep dives

### 9.1 Index and shard layout

The protocol's limits are hard: **50,000 URLs and 50 MB (uncompressed) per sitemap file; an index may list up to
50,000 sitemaps.** At ~200 B an entry the URL count binds first (10 MB ≪ 50 MB), so 50M URLs need ≥ 1,000 files and
one small index.

| Layout | Pros | Cons |
|---|---|---|
| One huge file | simple | illegal beyond 50,000 |
| **Stable id-range shards** (`url_id / 50000`) | a URL stays in the same file forever; deterministic; easy parallel build | edits don't cluster: every shard has changes daily (see 9.2) |
| By category/section | readable, per-section Search Console reports | uneven sizes, categories move |
| By `lastmod` bucket | recent changes cluster | a URL changes shard whenever it's edited, so *two* files change per edit |
| **Stable shards + a rolling "fresh" set** | completeness from the stable files, freshness from the small fresh files | the same URL appears in two files (allowed; `lastmod` must agree) |

**Choice: stable id-range shards + rolling fresh files.** ~1.2M distinct changed URLs/day ÷ 50,000 ≈ 24 fresh files per
day of history; keep 2 days (~48). The index then has ~1,050 entries; each `<sitemap>` carries its file's own `lastmod`,
so a crawler re-fetches only files whose `lastmod` moved. A small set of files gets crawled often, the rest rarely.

### 9.2 Generation: the number that kills the clever idea

The tempting design: mark a shard *dirty* on edit and regenerate **only dirty shards**. Check it:

- 2M edits/day over 1,000 shards ≈ **2,000 edits per shard per day**. The chance a shard has *zero* edits in a day is
  effectively 0 (≈ e⁻²⁰⁰⁰), so **every shard is dirty every day**.
- Even rebuilding hourly: 2M ÷ 24 ≈ 83k edits/hour ≈ 83 per shard: all dirty. Every 5 minutes ≈ 7 per shard: 99.9% dirty.
  Only at ~1-minute intervals (1.4 edits per shard) do you see a quarter of shards clean, and by then you're building
  every minute for no reader's benefit.
- Skew doesn't rescue it: say 80% of edits land on the newest 5% of pages (50 shards); the other 20% (400k edits)
  still spread ~420 per shard over the remaining 950. **Tail edits dirty everything.**

| Option | Pros | Cons |
|---|---|---|
| Build on every save | always current | 23 builds/s of a 10 MB file; pointless: crawlers don't look that often |
| On-demand with a cache (honeydolist: 5 min, dropped on save) | trivial at 300 URLs | at 50M a cold request streams 50k rows and gzips 10 MB under a crawler's nose; save-invalidation means constant cold hits |
| Dirty-shard rebuild | sounds efficient | saves nothing under these edit rates (above) |
| **Scheduled full rebuild in parallel + hourly fresh files** | simple, idempotent, ~30 s on 8 workers | recomputes unchanged shards (cheap) |

**Choice: scheduled, parallel, atomic.** Nightly (or every few hours) one job fans out 1,000 shard builds across workers
([44](../../44-background-jobs-queues.md)); each streams `seo_urls` by `(shard, url_id)` with keyset pagination, writes
to a *new* object key, then updates `seo_shards`; finally the index is rewritten pointing at the new keys, so crawlers
never see a half-built set. A separate hourly job writes `fresh-*` files from `changed_at`. **Freshness comes from
pings (9.4), not from rebuild frequency.** Measure first; optimise what the arithmetic says is expensive.

```go
const maxURLs, maxBytes = 50_000, 45 << 20 // stay under the 50 MB cap with headroom

func (s *shardWriter) Add(loc string, lastmod time.Time) (full bool) {
	line := fmt.Sprintf("<url><loc>%s</loc><lastmod>%s</lastmod></url>\n",
		html.EscapeString(loc), lastmod.UTC().Format("2006-01-02"))
	if s.n+1 > maxURLs || s.bytes+len(line) > maxBytes {
		return true // caller closes this file and starts the next
	}
	_, _ = s.gz.Write([]byte(line)) // a gzip.Writer over the object-store upload; check the error at Close
	s.n, s.bytes = s.n+1, s.bytes+len(line)
	return false
}
```

### 9.3 Truthful `lastmod`

Search engines use `lastmod` only if it proves reliable; a lying value teaches them to ignore yours. Honeydolist hit the
*under-reporting* version (a rate changed in its own table and never moved the machine's timestamp). At scale you meet
the *over-reporting* version: every view counter, every sidebar widget, every nightly "touch" moves `updated_at`.

| Option | Pros | Cons |
|---|---|---|
| `updated_at` of the row | free | moves on edits crawlers can't see; misses edits in child tables |
| "touch the parent" in code (honeydolist's fix) | right for known cases | every new child table must remember |
| **Hash of the indexable content** | immune to noise and to forgotten tables | must define the input once and keep it deterministic |

**Choice: `content_hash`.** The projector computes a hash over exactly what a crawler can observe (title, description,
canonical, body text, structured data, lead image URL). If the hash is unchanged, `lastmod` and the ping are skipped.
If it changed, `lastmod = now()` (the event's commit time, **not** the build time: a rebuild must never rewrite every
`lastmod` to "today"). Use `lastmod` dates at day granularity or a full W3C datetime consistently.

### 9.4 Change pings: coalesce, batch, back off

| Option | Pros | Cons |
|---|---|---|
| Ping per edit | simplest | 20 edits = 20 pings; engines throttle or ignore noisy hosts (treat as likely) |
| **Coalescing queue, debounce window** | one ping per URL per burst | adds ~30–60 s latency |
| Periodic *diff* of live list vs "already announced" (honeydolist) | stateless trigger, self-healing | at 50M a diff is a 50M-row anti-join |
| Event queue + nightly reconcile | cheap online, verified offline | two mechanisms |

**Choice: coalescing queue, plus a sampled reconcile.** The projector does
`INSERT INTO crawl_queue … ON CONFLICT (path) DO UPDATE SET changed_at = now(), kind = EXCLUDED.kind`, so a page edited
20 times is one row. The pinger wakes every ~30–60 s and takes rows whose `changed_at < now() - interval '30 seconds'`
(`SKIP LOCKED`, ≤ 10,000 per batch), POSTs to IndexNow, and deletes them on `200/202`. `429` and `5xx` retry with
exponential backoff and jitter; `400/403/422` are our bug (bad key, URL off-host): don't retry, alert
([10](10-webhook-delivery.md)). A `removed` ping goes out for deleted pages. Honest limits: **Google doesn't read
IndexNow**, and it retired its sitemap-ping endpoint in 2023 to my knowledge; for Google the sitemap's `lastmod`, a
good internal-link structure and Search Console are the mechanism. A nightly job diffs one *shard at a time* against
what we announced, to catch anything the queue missed. Gate pings by environment: honeydolist sends only from
production *and* an explicit switch, so a staging copy never announces itself.

### 9.5 What may be listed

The sitemap is a **promise**: every URL in it is canonical, 200, indexable. Break it and you waste crawl budget and
earn "submitted URL has crawl issue" reports. Rules, all derived from the one `listed` flag:

- **Never list:** redirected URLs (301/308), 404/410, `noindex` pages, pages blocked by `robots.txt`, non-canonical
  duplicates (`?sort=`, `?page=2` canonicalised to page 1), URLs on other hosts.
- **`robots.txt` must not hide what the page needs:** don't `Disallow` `/api/`, `/assets/`, `/media/` if the page is
  rendered by script, or the page indexes blank. Don't block a `noindex` page either: the crawler must fetch it to see
  the tag. Repeat shared `Disallow` lines in every UA group (the most specific group wins, groups don't merge).
- **Generated, tested, cached short:** build `robots.txt` in code and unit-test it as an RFC 9309 parser would. A `5xx`
  on it makes crawlers back off the whole site, so on a data error serve the *last good copy*, never an error.
- **Closed hosts:** staging answers `Disallow: /` and no sitemap (honeydolist returns 404 for the sitemap off the
  public host), and `X-Robots-Tag: noindex` on every response.
- **Verify continuously:** sample ~0.1% of listed URLs a day (50k ≈ 0.6 fetches/s) with a real GET: expect 200, a
  self-referencing canonical, no `noindex`. Alert if more than a small fraction fail; the number that matters is the
  failure *rate*, not any single URL.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (500M URLs, 20M changes/day): shard count (10,000 files; the index still fits under
  50,000 entries), build time (~5 min on 8 workers, fine), and the seo_urls projector write rate (230/s avg, 1.2k/s
  peak, fine). Beyond ~2.5B URLs the *index* limit binds; then use several indexes (each ≤ 50,000 sitemaps) listed in
  `robots.txt`, or split by host.
- **Single points of failure:** the builder (a missed night means yesterday's sitemap; pings still flow), the projector
  (events queue in the outbox), `robots.txt` generation (serve last good).
- **Projector falls behind:** sitemaps lag, still truthful; pinger drains late. Alert on queue age, not depth.
- **Mass unpublish (a tenant is banned):** 1M `removed` pings and 1M sitemap removals; throttle the pinger, since
  engines see 404/410 on recrawl anyway.
- **A rebuild writes a wrong `lastmod`** (all "today"): crawlers treat the whole site as fresh, then learn to distrust
  it. Test: building from an unchanged DB twice produces byte-identical files.
- **IndexNow endpoint down:** backoff, bounded queue; the sitemap alone still works.
- **Clock skew** between nodes producing `lastmod` in the future: clamp to now, alert.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Stable id-range shards + fresh files | category or `lastmod` shards | stable files, trivial parallel build, a small hot set | the same URL listed twice |
| Scheduled full rebuild | dirty-shard rebuild | simplicity (arithmetic shows dirty tracking saves nothing) | ~30 s of cheap recomputation |
| `content_hash` for `lastmod` | `updated_at` | a `lastmod` crawlers can trust | defining and versioning the hashed fields |
| Coalescing queue + sampled reconcile | ping per edit / full diff | one ping per burst, cheap online | 30–60 s ping delay; two mechanisms |
| One `listed` predicate | per-component filters | page, sitemap, pings can't disagree | a projector that must never lag silently |
| Generated, tested `robots.txt` | a static file | right domain, per-environment, regression-tested | code to maintain |

## 12. Check yourself

1. What are the sitemap protocol's limits and how many files do 50M URLs need?
2. Why doesn't "regenerate only the dirty shards" help at 2M edits a day over 1,000 shards?
3. Why use a content hash for `lastmod`, and why must a rebuild never rewrite it?
4. A page is edited 20 times in a minute. How many pings go out, and how?
5. Name four kinds of URL that must never be in a sitemap, and one thing `robots.txt` must not block.

<details><summary>Answers</summary>

1. ≤ 50,000 URLs and ≤ 50 MB uncompressed per file; an index lists up to 50,000 sitemaps. At ~200 B per entry the URL
   cap binds, so 50M ÷ 50,000 = at least 1,000 files plus one index.
2. 2M ÷ 1,000 ≈ 2,000 edits per shard per day: every shard is dirty daily. Even hourly (≈ 83 per shard) or every five
   minutes (≈ 7 per shard) nearly all are dirty, and tail edits dirty the rest even with heavy skew. Build on a
   schedule, in parallel (~30 s on 8 workers), and get freshness from pings.
3. `updated_at` moves on edits crawlers can't see and misses edits in child tables; a hash of the *observable* content
   moves exactly when the page does. A rebuild that rewrites `lastmod` to "today" tells crawlers everything changed,
   which they will learn to ignore. `lastmod` is the event's commit time, stored, not computed at build time.
4. One. `crawl_queue` has `path` as primary key and edits `UPSERT`, so 20 edits are one row. After a ~30 s quiet window
   the pinger sends it in a batch of up to 10,000 URLs; `429/5xx` back off and retry, `4xx` alert.
5. Redirected URLs, 404/410 pages, `noindex` pages, robots-blocked pages, non-canonical duplicates (any four). It must
   not block what a rendered page needs (`/api/`, `/assets/`, `/media/`), nor `noindex` pages (the crawler must fetch
   them to see the tag).

</details>

## 13. In Go

- Related lessons: [44 Background jobs](../../44-background-jobs-queues.md) (fan-out of shard builds), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (projector input), [59 Production DB patterns](../../59-production-database-patterns.md) (keyset scans, `ON CONFLICT`, `SKIP LOCKED`), [36 Resilience](../../36-resilience-patterns.md) (backoff with jitter), [20 HTTP server](../../20-http-server.md) and [64 HTTP internals](../../64-http-protocol-internals.md) (`ETag`/`If-None-Match`, `HEAD`, gzip), [15 Sync & context](../../15-sync-context.md) (worker pool, `errgroup`).
- Packages: `encoding/xml` or hand-written `bufio` + `compress/gzip` streaming (never build 10 MB in memory), `html.EscapeString`, `net/url` for canonical paths, `crypto/sha256`, `golang.org/x/sync/errgroup`, an S3 client with multipart upload.
- Test the builder as a pure function: same rows in, byte-identical file out; and a "the sitemap equals the public lists" test like honeydolist's, run against a seeded DB.

## 14. Extensions

- **Multi-tenant hosts:** one sitemap set and one `robots.txt` per host; where do 200k hosts' files live, and who builds them?
- **Image/video/news sitemaps** and `hreflang` alternates: the entry size grows 2–5×; does the URL cap or the byte cap bind?
- **Crawl-log analytics:** read CDN logs to learn which sitemap files crawlers fetch and which URLs they skip.
- **Search Console API:** automate "submitted vs indexed" reports and alert on drops per shard.
- **Priority hints:** put revenue-heavy URLs in files that always carry fresh `lastmod`.
- **Sitemap for removed content:** a short-lived list of 410s to speed de-indexing; compare with a plain `removed` ping.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| SEO workers (Go: projector, pinger, verifier, nightly shard builder) | ~115 change events/s; ~120 IndexNow batches/day; verifier 0.6 fetches/s; a nightly build of 50M rows | CPU (the nightly burst), otherwise idle | steady: projector 115 × ~1 ms (hash ~5 KB, two upserts) = 0.12 vCPU + pinger and verifier ≈ 0.05 ⇒ 0.2 vCPU; N = ceil(0.2 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 (one per zone). Build: 50M ÷ 200k rows/s per worker = 250 vCPU-s (§4), so 6 workers on the 6 vCPU ≈ 42 s | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | formatting and gzip are CPU; memory is tiny because shards stream (~50 MB per worker), so `c` not `m`; the burst is a minute a night, so a bigger or Spot fleet would save pennies |
| Postgres (seo_urls, seo_shards, crawl_queue) | 115 upserts/s into seo_urls + 115 into crawl_queue + 115 deletes at peak; 50M rows | memory (IOPS are tiny) | rows 50M × ~200 B = 10 GB + indexes (path ~5 GB, shard + changed + PK ~4 GB) ≈ 20 GB (assumption); writes 115 × (4 + 4 + 3 IOPS; an indexed `changed_at` defeats HOT updates) ≈ 1,300 IOPS < gp3's 3,000 included; the nightly scan reads 10 GB: ~80 s even cold at gp3's 125 MiB/s, less from RAM | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ | 1 + synchronous standby | ~1.2M changed URLs a day touch most of the 1.25M heap pages and the scan touches all, so RAM ≥ 20 GB keeps both off the disk; `m7g.xlarge` (16 GiB) would also run at ~1k IOPS, so `r` buys headroom, not necessity; no replica: the build is 6 streams for ~40 s a night |

S3: nothing to size. ~1,050 gzip files ≈ 1.6 GB per build (§4: 1–2 MB each), 7 builds kept ≈ 11 GB; the CDN serves them straight from S3, so crawlers never reach the workers. No Redis or Kafka: the queue is the `crawl_queue` table. Outbound: IndexNow ≈ 120 MB/day, verifier ≈ 50k GETs × ~100 KB ≈ 5 GB/day.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Workers: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.xlarge` ≈ $310 managed, Multi-AZ doubles | $620 |
| gp3: 2 volumes × 100 GB (primary, standby) | $20 |
| S3 (≈ 11 GB) + CDN egress for sitemap fetches (1.6 GB × 3 full fetches a day × 30 ≈ 144 GB × $0.085) | $10 |
| NAT: 1 gateway ($33) + ~150 GB of verifier traffic × $0.045 | $40 |
| ALB for `robots.txt`, status and the key file: $16 + ~1 LCU | $20 |
| **Total** | **≈ $870** (≈ $600 with a 1-year plan on the $780 compute baseline) |

- **Biggest line and why:** the database, 71 %, for a 20 GB table that one node's RAM can hold. This is honest tiny hardware: the workers run at 0.2 vCPU all day and the only real work is a 40-second burst. `seo_urls` is derived data (it can be rebuilt from the content tables), so the lever is single-AZ `db.r7g.xlarge` with backups, which halves the line to $310; I keep Multi-AZ because a 50M-row rebuild takes hours, and a stale projector means a lying sitemap.
- **What changes at 10×:** 500M URLs, 20M changes a day. `seo_urls` ≈ 200 GB no longer fits 32 GiB: either `db.r7g.4xlarge` (128 GiB, ≈ $2,480 Multi-AZ) or let the scan read gp3 (100 GB heap ÷ 1,000 MiB/s ≈ 100 s) and run builds on a replica. Writes ≈ 1.2k events/s × ~11 IOPS ≈ 13k IOPS, still `gp3` (cap 16,000, +$50 for the extra IOPS). The builder is 2,500 vCPU-s ⇒ 3 × `c7g.2xlarge` ≈ 104 s. The projector (1.2 vCPU) and the pinger (1,200 batches a day) still fit on the same nodes.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- `seo_urls` is one narrow row per URL: 50M rows × ~200 B ≈ 10 GB, ≈ 20 GB with indexes (section 15). One projector writes it (~115 upserts/s at peak); readers are **ordered range scans**: a shard build is 50,000 consecutive `url_id`s, 1,000 of them a night, plus the `changed_at` scan for the fresh files.
- `crawl_queue` is one row per changed path: an edit burst must collapse into one row by upsert, and a pinger drains up to 10,000 due rows at a time. Rows live for minutes.
- All of it is **derived**. The record is the content tables (the CMS of lesson 32); `seo_urls` can be re-projected from them, which takes hours for 50M rows, and the sitemap files in S3 (~1.6 GB per build) are derived again from `seo_urls`.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | an upsert on the primary key (`INSERT … ON CONFLICT (path) DO UPDATE`) is the coalescing step; `FOR UPDATE SKIP LOCKED` drains the queue; keyset scans by `(shard, url_id)` build a file in one ordered pass; it is the same engine as the CMS whose events feed the projector |
| MySQL (RDS / Aurora) | ✔ works, nearly interchangeable | `ON DUPLICATE KEY UPDATE` and `SKIP LOCKED` (8.0+) cover the queue. InnoDB's clustered key makes a shard (`url_id / 50000`) a contiguous primary-key range, so a build reads sequentially with no heap fetches, slightly better than an index scan plus heap. The `WHERE listed` partial indexes become plain indexes, which costs little if most of the 50M URLs are listed (an assumption). `path TEXT UNIQUE` must become a bounded `VARCHAR`/`VARBINARY` |
| DynamoDB | ✔ works, no gain | an item per `url_id` (~200 B) and a `PutItem` per path coalesce edits, and 115 writes/s is trivial. But the shard build becomes a `Query` on a GSI by shard (a second copy of 50M items), the fresh files need another GSI on `changed_at`, and the queue needs one on `next_attempt_at`; nothing here approaches a write rate that needs it |
| Redis (ElastiCache / MemoryDB) | ✔ for the queue only, ✖ for `seo_urls` | a sorted set of `changed_at` plus a hash would coalesce bursts in memory; a restart loses pending pings, which the nightly reconcile (9.4) repairs. Holding 20 GB of rows read once a night in RAM (≈ 34 GB with overhead) is the wrong memory use. The lesson chose no Redis: 115 upserts/s is not worth a component |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.xlarge` from section 15, no replica), with S3 for the generated files. The properties that decide it are the primary-key upsert that coalesces a burst into one row, `SKIP LOCKED` for the pinger, and ordered keyset scans for the builder; all three also exist in MySQL, so the honest tie-breaker is the CMS already being Postgres. System of record: the content tables (lesson 32). Derived and rebuildable: `seo_urls`, `seo_shards`, `crawl_queue` (the nightly reconcile covers a lost queue) and the S3 files. Multi-AZ is kept because a re-projection of 50M rows takes hours and a stale projector means a lying sitemap (section 15).

**If it were MySQL instead:** nothing material. `ON DUPLICATE KEY UPDATE` replaces `ON CONFLICT` (with `VALUES()` or a row alias for the new `kind`), `BYTEA` becomes `VARBINARY(32)`, `TIMESTAMPTZ` becomes a UTC `TIMESTAMP`, the two partial indexes lose their `WHERE`, and `path` becomes `VARCHAR(512)` with a binary collation (InnoDB cannot index an unbounded `TEXT` without a prefix length, and the default collation is case- and accent-insensitive, which a path is not). The shard build gets a little cheaper.

**What would flip the choice:** nothing in the write rate (115/s) leaves an RDS primary; at 10× (500M URLs ≈ 200 GB) it is a bigger node or builds on a replica (section 15), not another engine. Beyond a few billion URLs (the index limit also binds at ~2.5B, section 10) the nightly full scan of an OLTP table stops being the right shape: build from a columnar export in S3 (Parquet, read by a batch job or Athena) instead.
