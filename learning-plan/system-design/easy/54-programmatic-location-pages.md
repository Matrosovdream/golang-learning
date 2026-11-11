# 54 · Programmatic Location Pages

`🟢 easy` · *Topics:* a candidate matrix versus the pages that deserve to exist, quality gates built from real data, "published ⇒ passed the gate" as a database invariant, assembling a page from template + local facts + user content, measuring distinctness on the variable part, tiers and where AI drafts are allowed, a page state machine tied to HTTP status and sitemap membership, rollout in waves with a stop-loss, cohort measurement · *Builds on:* [33 Sitemaps & Crawl Notifications](33-sitemaps-crawl-notify.md) (what may be listed), [34 Crawler-Ready SPA Serving](34-crawler-ready-spa.md) (honest status codes), [38 AI Draft Queue](38-ai-draft-queue.md) (drafts and review), [32 Block-Based CMS](32-block-cms.md) (page assembly), [35 Redirect Manager](35-redirect-manager.md) (retiring a URL)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [53 Web Push & the Installable App (PWA)](53-web-push-pwa.md) · Next: [55 Order Timeline with Photo Steps](55-order-timeline.md)

---

> 🌱 **Seed (honeydolist-site):** the growth idea is in `docs/seo/migration.md` (6 services × 10 towns = 60 candidate pages, plus equipment) with its warning: sixty near-identical pages are doorway spam, so *start with 3–4 written by hand and scale only once they rank*. The code enforces that in layers. `service_areas` (migration 0005) separates `is_offered` ("we do this work here", "plenty of true combinations will never deserve their own page") from `is_published` (the page exists), and a **CHECK** makes a published row need a non-empty hand-written intro (`service_areas_published_needs_intro`); `town_pages` (0103) and `rental_areas` (0101) carry the same CHECK. The **120-word minimum** is a *use-case* rule (`domain.MinTownIntroWords`, a `422`), not a constraint, and the editor warns when more than half of a town's sentences also appear in a sibling's intro (`similar_to`). A page is live only while it is published *and* the service is offered *and* the town is on the site; otherwise a **real 404** in the head and the API; an archived service answers `301`. The sitemap lists only live pairs (PLAN steps 70, 79, 81). **Nothing is live until the owner writes the words**: Step 79 even decided against an AI draft button ("the intro is the one thing that must be his"). Step 74's *Search health* flags thin pages (a body under 150 words and 20 other rules) but only reports.
> **Here, at scale:** a marketplace with 400 services × 30,000 towns = 12M candidate pages and 20M page views a day, where the question is no longer "can we serve it" but "which pages are allowed to exist, who writes them, and what do we do when they turn out thin".

## 1. The task

> "Our marketplace could have a page for every service in every town: 'plumber in Springfield',
> 'tree removal in Ashland'. Design the system that decides which of those pages exist, builds them
> from real data, keeps them true, and takes them down when they stop being useful. Search
> engines punish thin, templated pages, so do it carefully."

Serving a page is the easy part (a cached read). The design is a **pipeline with a conscience**:
generate candidates cheaply, publish only what has something real to say, watch the results, and
be able to retreat.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What makes a page "useful"? | The gate has to be measurable. | Real providers and recent jobs in that town, local facts, enough distinct content. |
| Who writes the words? | Cost vs quality. | A mix: the top pages edited by people, the rest assembled from data with sampled review. |
| Is AI-written text allowed? | Policy and risk. | As a *draft* only, behind a human gate or sampled review, never auto-published untouched in tier 1. |
| What does search-engine policy say? | The downside risk. | Mass-produced pages with little unique value risk demotion, possibly site-wide (treat as a risk model, not a precise rule). |
| What happens when a town loses its last provider? | Pages go stale. | The page demotes itself (noindex, then retire). |
| Rollout speed? | Blast radius. | In waves with stop-loss, not all at once. |
| Scale? | Sizes the matrix and the jobs. | 400 services × 30,000 towns; 20M page views/day. |
| Where does demand data come from? | Prioritisation. | On-site searches, search-engine impressions, our own leads. |

## 3. Requirements

**Functional**
- Maintain the candidate matrix (service × place) and, per candidate, its signals (providers, jobs, reviews, demand).
- Decide eligibility with versioned gates; assemble a draft; route through review; publish; demote; retire.
- Serve live pages with correct status, canonical, structured data, internal links and sitemap membership.
- Report per-cohort results; roll back a cohort in one step.

**Non-functional**
- A page can only be live if it passed the current gate (an invariant, not a habit).
- Page-state changes propagate to sitemap, redirects and CDN within minutes.
- Re-evaluating 12M candidates must not need a table scan per request.

**Out of scope**
- Writing quality itself, link building, ranking prediction, per-user personalisation.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Candidates | 12M | 400 × 30,000 |
| Candidate rows | ~2.4 GB | 12M × ~200 B (state, hashes, counters) |
| Signals refresh | ~3.3k rows/s for a one-hour nightly pass | 12M ÷ 3,600 s; in practice only *dirty* pairs |
| Pass the gates | ~600k (5%) | assumption: most combinations have no provider, no jobs, no demand |
| Page views | ~230/s average, ~1.2k/s peak | 20M ÷ 86,400 ≈ 231; peak 5×. A CDN-cached read. |
| Sitemap | 12 files + index | 600k ÷ 50,000 = 12 (see [33](33-sitemaps-crawl-notify.md)) |
| Near-duplicate pairs, all candidates | 1.8 × 10¹¹ | 30,000²/2 = 4.5 × 10⁸ per service × 400 |
| …among the eligible only | ~4.5 × 10⁸ | 600k ÷ 400 = 1,500 per service; 1,500²/2 ≈ 1.1M × 400 |
| Daily stats | ~219M rows/year, ~9 GB | 600k pages × 365 × ~40 B |

What the numbers tell us: **the gate is the whole game**: it cuts 12M candidates to ~600k live
pages, which shrinks every later problem (the sitemap, the duplicate check, the review queue, the
stats table). Serving is trivial. And the duplicate check is small *after* the gate, which is the
reason to run the gate first.

## 5. Plan

1. Decide which candidates are eligible (signals and gates).
2. Assemble a page: template, local facts, user content, an editorial intro.
3. Measure distinctness on the variable part, not the boilerplate.
4. The lifecycle: state machine, HTTP status and sitemap membership per state.
5. Rollout in waves, with cohorts you can measure and retract.

## 6. High-level design

```
 services × places ──► candidate matrix (12M rows, state = candidate)
        ▲                      │
 signals: providers, jobs,     │ gate job (versioned; only dirty pairs): enough real data?
 reviews, demand ──────────────┤        no  → blocked (never rendered, stays a row)
 (outbox + nightly refresh)    ▼ yes
                         assemble draft: template blocks + local facts + reviews/projects (+ AI intro draft)
                               │
                  distinctness check (variable part only) ── too similar → blocked / needs human edit
                               ▼
                    tier 1: human edits ─┐        tier 2: sampled review ─┐
                               ▼         ▼                                  ▼
                              live (gate_passed_at set; wave N)
                               │  sitemap shard for the wave · CDN · JSON-LD · internal links from hubs
                               ▼
        daily stats per page/cohort ──► demote (noindex) ──► retire (301 to hub, or 410)
```

## 7. API

```
GET  /{service}/in/{place}                      → 200 page | 404 (not live) | 301 → hub (retired) | 410 (retired, no good target)
GET  /admin/location-pages?state=&service=&sort=demand&cursor=…      → 200 page of rows
POST /admin/location-pages/{service}/{place}/publish   → 200 | 422 { "gate": ["providers < 3", "unique_words < 120"] }
POST /admin/location-pages/{service}/{place}/retire    { "redirect_to": "/…" }   → 200
POST /admin/waves                               { "tier": 2, "limit": 5000, "filter": {…} } → 202 { wave_id }
POST /admin/waves/{id}/rollback                 → 202   (cohort → noindex, out of the sitemap)
GET  /admin/cohorts/{wave_id}/stats             → 200 { indexed_ratio, impressions_per_page, leads, … }
```

A page that is not live answers a **real 404** (honeydolist does the same, in the HTML head as well as the API), never a thin shell with a 200.

## 8. Data model

```sql
CREATE TABLE location_pages (
    service_id        BIGINT NOT NULL,
    place_id          BIGINT NOT NULL,
    state             TEXT   NOT NULL DEFAULT 'candidate'
        CHECK (state IN ('candidate','blocked','draft','in_review','live','noindex','retired')),
    tier              SMALLINT,                              -- 1 = human-edited, 2 = data-assembled
    intro             TEXT,
    local_facts       JSONB  NOT NULL DEFAULT '{}',          -- providers, jobs, price band, drive time…
    unique_words      INT    NOT NULL DEFAULT 0,             -- words in the VARIABLE part
    simhash           BIGINT,                                -- of the variable part only
    wave_id           BIGINT,
    gate_version      SMALLINT,
    gate_passed_at    TIMESTAMPTZ,
    live_since        TIMESTAMPTZ,
    state_changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, place_id),
    -- the invariant: nothing is live without passing a gate and having words
    CONSTRAINT live_needs_gate CHECK (state <> 'live'
        OR (gate_passed_at IS NOT NULL AND btrim(coalesce(intro, '')) <> ''))
);
CREATE INDEX location_pages_state_idx ON location_pages (state, state_changed_at);
CREATE INDEX location_pages_live_idx  ON location_pages (service_id, place_id) WHERE state = 'live';

CREATE TABLE page_signals (
    service_id   BIGINT NOT NULL,
    place_id     BIGINT NOT NULL,
    providers_n  INT NOT NULL DEFAULT 0,                     -- active providers serving this place
    jobs_n       INT NOT NULL DEFAULT 0,                     -- completed jobs, last 12 months
    reviews_n    INT NOT NULL DEFAULT 0,
    demand_30d   INT NOT NULL DEFAULT 0,                     -- searches / impressions / lead intents
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, place_id)
);

CREATE TABLE page_stats_daily (                              -- partitioned by month in production
    day DATE NOT NULL, service_id BIGINT NOT NULL, place_id BIGINT NOT NULL,
    impressions INT NOT NULL DEFAULT 0, clicks INT NOT NULL DEFAULT 0,
    views INT NOT NULL DEFAULT 0,       leads  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (day, service_id, place_id)
);
```

`live_needs_gate` is the scaled-up version of honeydolist's `published_needs_intro`. The database
**cannot judge quality or similarity**; what it can do is refuse a `live` row that skipped the
pipeline (`gate_passed_at` is only ever written by the gate job). The rule is then enforced
where it is checkable, not trusted as a habit.

## 9. Deep dives

### 9.1 Which pages deserve to exist

Separate three questions that honeydolist's schema already separates: **do we do this there**
(`is_offered`), **is there anything to say** (the gate), **is the page live** (`state`).

Gates are *data*, versioned, and every one is something you can count:

| Gate | Example threshold | Why |
|---|---|---|
| Real supply | ≥ 3 active providers serving the place | a page for a service nobody offers there is a doorway |
| Local evidence | ≥ 2 completed jobs or reviews from that place in 12 months | the one thing a competitor's template can't copy |
| Local facts | ≥ 3 distinct facts (price band, drive time, permit note…) | gives the page content that varies |
| Demand | ≥ N searches or impressions in 30 days | don't build what nobody wants |
| Distinctness | ≥ 120 unique words in the variable part; similarity below a threshold (9.2) | honeydolist's 120-word floor, now measured not trusted |

| Option for where the gate lives | Pros | Cons |
|---|---|---|
| **A versioned job writes `gate_passed_at` + `gate_version`; the DB CHECK refuses `live` without it** | auditable; changing a threshold = new version, re-run on dirty pairs | a stale `gate_version` has to be re-evaluated deliberately |
| Gate only in the publish handler | simple | any other writer (a script, an import) bypasses it |
| Gate as a SQL `CHECK` on counts | impossible to bypass | counts live in other tables; a `CHECK` can't read them |

**Choice and why.** The versioned job plus the CHECK. Only the *dirty* set is re-evaluated: a
provider joins or leaves → the outbox marks `(service, place)` dirty → the gate runs for it.
**Hysteresis** stops flapping: promote at ≥ 3 providers, demote only below 2.

### 9.2 Assembling a page, and measuring distinctness

A page is assembled from blocks ([32](32-block-cms.md)): shared **template** (headings, FAQs, the
form), **local facts** (data), **user content** (real reviews and finished jobs in that place) and
an **editorial intro**. The trap is measuring similarity on the *whole* page: with 300 of 400 words
as shared template, every page looks 75% alike no matter how different the local part is.

- Hash and count only the **variable** blocks (`unique_words`, `simhash` over word 3-shingles of local facts + intro + local reviews).
- Compare within a **service** (that is where templated duplicates occur). honeydolist's check, "more than half of the sentences also appear in another town's intro", is the human-scale version.

```go
func simhash(shingles []string) uint64 { // illustrative; use a stronger mixer than FNV in production
    var v [64]int
    for _, s := range shingles {
        h := fnv.New64a()
        h.Write([]byte(s))
        x := h.Sum64()
        for i := 0; i < 64; i++ {
            if x&(1<<uint(i)) != 0 { v[i]++ } else { v[i]-- }
        }
    }
    var out uint64
    for i := 0; i < 64; i++ {
        if v[i] > 0 { out |= 1 << uint(i) }
    }
    return out
}
func near(a, b uint64) bool { return bits.OnesCount64(a^b) <= 3 }
```

**Do the arithmetic before choosing a clever index.** All-pairs over all candidates is 1.8 × 10¹¹
popcounts, roughly minutes on one core (assume ~1 ns per XOR+popcount over an in-cache array),
seconds on a small fleet. After the gate it is 4.5 × 10⁸. So **no LSH is needed here**: a
per-service in-memory scan wins. It becomes necessary only if you must compare a new document against
*hundreds of millions* online; then split the 64 bits into 4 bands of 16 and index each band: two
hashes within Hamming distance 3 differ in at most 3 bits, so by pigeonhole **at least one band is
identical** and a band lookup finds every candidate neighbour.

**Where AI may write.** honeydolist's stance is "the owner's words, nothing live until he writes
them", right for 60 pages and impossible for 600k. Tiers:

| Tier | Pages | Words by | Gate before live |
|---|---|---|---|
| 1 | top ~10k by demand | a person edits (an AI draft may start it, [38](38-ai-draft-queue.md)) | full human review |
| 2 | the remaining ~590k eligible | assembled from real data; an AI-written intro only from the page's own facts | automated gates + **sampled** human review, with a stop if the sample fails |
| none | the other ~11.4M | never rendered | a row in the matrix, nothing more |

This departs from the seed's policy for tier 2, and says why: review capacity, not generation, is
the limit, the same conclusion as [38](38-ai-draft-queue.md).

### 9.3 Lifecycle, status codes and the sitemap

| State | HTTP | In sitemap | Notes |
|---|---|---|---|
| `candidate`, `blocked`, `draft`, `in_review` | **404** | no | never a thin 200 shell |
| `live` | 200, self-canonical, JSON-LD (`Service` with `areaServed`) | yes, in its **wave's sitemap file** | hubs link to it |
| `noindex` | 200 with `robots: noindex` | no | demoted but reachable for people (customers, existing links) |
| `retired` | **301** to the place or service hub, or **410** if nothing fits | no | removed from hub links; rule in [35](35-redirect-manager.md) |

- Membership of the sitemap is a **function of state**, not a separate list ([33](33-sitemaps-crawl-notify.md)); a state change dirties its shard.
- Honest status codes from an SPA shell are [34](34-crawler-ready-spa.md)'s job; the head must say 404 as well as the body.
- **Internal links** are part of the lifecycle: a place hub lists only *live* services in it, so retiring a page removes the link in the same change, and no live page links to a 404.
- Avoid flip-flopping: a page that goes live, noindex, live, noindex is an unstable signal. Demote only on a sustained condition (e.g. 60 days of no impressions *and* no leads) and re-promote only through the full gate.

### 9.4 Waves, cohorts, and a stop-loss

Search engines judge a site's pattern of pages. A scaled-content problem can affect more than the
new pages, so **roll out in waves you can retract**:

1. Wave 0: the hand-edited tier-1 handful (honeydolist's "3–4, written by hand").
2. Wave 1: ~1,000 tier-2 pages; wait for results (weeks, not hours; the interval is a judgement call).
3. Next waves: 5k, 25k, 100k, the rest, each *only* if the previous cohort cleared its bar.

Each wave has its **own sitemap file(s)**, so the cohort is observable as a unit (a search console
reports indexed vs submitted per sitemap file, as of my knowledge). The bar: indexed ratio
(indexed ÷ submitted), impressions per page, leads per 1,000 views, and a manual spot check. If a
wave fails: `POST /admin/waves/{id}/rollback` is one `UPDATE … SET state = 'noindex' WHERE wave_id = $1`
plus a sitemap regeneration (and a kill switch, [51](51-runtime-settings.md), for the whole page
family).

**Measuring at 600k pages.** Per-page search data may be incomplete (the search-console API returns
a limited number of rows a day per property; a warehouse export is the heavier route), so measure
**cohorts**: `(wave, tier, service)`. On-site views and leads are ours and exact: batch them into
`page_stats_daily` like [47](47-web-analytics.md)'s rollups (about 9 GB a year). Pull external
impressions with a watermark and backfill ([47](47-web-analytics.md) §9.4).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (120M candidates, 6M live): not serving. The gate job's **signal joins** (providers, jobs, reviews per pair): maintain `page_signals` incrementally from the outbox and re-gate only dirty pairs; then review capacity.
- **A bad template shipped to 300k pages:** the distinctness check measures the variable part; a template change is a *new version* applied by wave, never an in-place edit of live pages.
- **Provider data wrong** (a bulk import marks 5,000 towns "supplied"): gates read signals with a freshness limit and a ceiling on daily promotions per service; a spike pages a human.
- **Penalty-style demotion:** the symptom is indexed ratio and impressions dropping across *several* cohorts at once. Response: stop promoting, roll back the newest waves, keep tier 1.
- **Page views from an unprepared crawler:** 1.2k/s of cached reads is nothing; the risk is crawl rate on a fresh 600k URL set ([33](33-sitemaps-crawl-notify.md)), so waves also meter the crawl.
- **Single points of failure:** the gate job (its backlog just delays new pages), the stats pipeline (a gap blinds the stop-loss, so alert on freshness, [50](50-alerting-engine.md)).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Publish only gated pages | All 12M candidates | quality, smaller everything | most long-tail combinations stay un-pageable |
| Versioned gate + `live_needs_gate` CHECK | A handler-only check | the invariant can't be bypassed | an extra column and a re-gate step |
| Distinctness on the variable part | Whole-page similarity | a real signal, not template noise | blocks must be tagged variable/shared |
| Brute-force per-service simhash scan | LSH bands | simplicity, exact | revisit if comparing online against 10⁸+ |
| Tiers: human top, sampled-review bulk | Humans write everything | scale | a policy departure from "every page his words" |
| Waves with sitemap-per-wave | One big launch | measurable, reversible | slower to full coverage |
| 404 for not-live, 301/410 for retired | A thin 200 for everything | honest signals | hub/link bookkeeping |

## 12. Check yourself

1. Why does honeydolist keep `is_offered` and `is_published` separate, and what does the CHECK enforce?
2. Why is the 120-word rule in code and the non-empty intro in the CHECK, and how does `live_needs_gate` generalise it?
3. Why measure similarity on the variable part only?
4. Do the arithmetic: all-pairs near-duplicate checking for 12M candidates vs after the gate. Is LSH needed?
5. What status does each state return, and why must a not-live page be a real 404?

<details><summary>Answers</summary>

1. "We do the work there" and "a page exists" are different facts: many true combinations never
   deserve a page. The CHECK refuses a published row with an empty intro, so a templated page cannot
   go live by habit or by a stray script.
2. A word count is a policy that changes; a CHECK on a changing rule is awkward, and a count of
   words could be gamed anyway, so the repo checks it in the use case and uses the CHECK only as a
   floor. At scale the gate has inputs a CHECK can't read (providers, jobs, similarity), so a job
   writes `gate_passed_at` and the CHECK demands it for `live`: bypassing the pipeline is impossible.
3. Shared template text dominates a page; hashing everything makes all pages look alike. The
   variable blocks (local facts, intro, local reviews) are what makes a page distinct, so only they
   are hashed and counted.
4. All 12M: 400 × 30,000²/2 = 1.8 × 10¹¹ popcounts, minutes on a core and seconds on a fleet at
   ~1 ns each. After the gate: ~1,500 pages per service, ~4.5 × 10⁸ in total. No LSH: a per-service
   in-memory scan is enough. Bands (pigeonhole over 4 × 16 bits for distance ≤ 3) matter only when
   comparing online against hundreds of millions.
5. Not live: 404; live: 200; noindex: 200 with `noindex`; retired: 301 to a hub or 410. A thin 200
   shell is exactly the doorway page the system exists to avoid, and a 404 is an honest "nothing
   here yet".

</details>

## 13. In Go

- Related lessons: [55 Data pipelines](../../55-data-pipelines.md) (the gate and assemble stages), [44 Background jobs](../../44-background-jobs-queues.md) (dirty-set re-gating), [54 Collection operations](../../54-collection-operations.md) and [51 Sorting & searching](../../51-sorting-searching.md) (the in-memory scan), [59 Production DB patterns](../../59-production-database-patterns.md) (partial indexes, `CHECK`), [31 DDD](../../31-ddd-tactical.md) (the page aggregate with its state machine), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (signals marking pairs dirty).
- `math/bits.OnesCount64(a ^ b)` is the Hamming distance; load one service's hashes as a `[]uint64` and scan; benchmark it (`testing.B`) before believing the "~1 ns" assumption.
- Keep the gate as a pure function `Gate(version, signals, page) []Failure` so each rule is table-tested; the job only loads inputs and writes the result.
- Packages: `pgx/v5`, `hash/fnv` (sketch; use `xxhash` in production), `math/bits`, `golang.org/x/sync/errgroup`.

## 14. Extensions

- **Per-place hubs and breadcrumbs:** how internal linking should follow the state machine, and what hub page ranks for "services in {place}" ([56](56-related-content-interlinking.md)).
- **Freshness signals:** a page showing a provider count or price band must change when the data does; how often is "often enough" without churning `lastmod` ([33](33-sitemaps-crawl-notify.md))?
- **Pages with no local supply yet:** a lead-capture "notify me" page that is `noindex` by design.
- **Demand-first generation:** start from search-demand data and generate only candidates above a threshold; what that does to the matrix size.
- **Multilingual and multi-country:** `hreflang` between place pages and the gate per market.
- **Penalty drill:** rehearse the rollback of the newest wave and measure how long sitemap, CDN and redirects take to settle.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). Tiny hardware: the gate cuts 12M candidates to ~600k live pages, so serving, the duplicate check and the stats are all small. The interesting cost is the bytes the CDN sends.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Page servers (Go, server-rendered HTML + JSON-LD, behind the CDN) | ~1.2k views/s peak (230/s average); sized for a cold CDN, though ~10 % reach the origin if it hits 90 % (assumed) | CPU | 1.2k × 3 ms (render, assumption) = 3.6 vCPU. N = ceil(3.6 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.7) = 2 → 3. Memory: ~50 KB per in-flight render, well under 8 GiB | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS | 3, one per zone | even with no CDN at all the whole peak is 3.6 vCPU, so a few small instances for availability and a cold CDN; render is CPU, so `c`. A not-live page answers 404 without touching the database beyond one key read |
| Batch workers (gate, assemble, simhash scan, stats rollup, sitemap build) | nightly pass 3.3k signal rows/s for an hour; 4.5 × 10⁸ popcounts after the gate | CPU, in bursts | gate ≈ 0.1 ms per pair incl. its DB share (assumption) × 3.3k/s = 0.33 vCPU. Scan: 1.8 × 10¹¹ × ~1 ns = 180 s on one core for all candidates, 4.5 × 10⁸ × 1 ns = 0.45 s after the gate; all 12M hashes = 96 MB. N = 1 | `c7g.large` (2 vCPU / 4 GiB) | 2 (one spare so a lease can fail over) | no cluster and no LSH: the scan fits in seconds and megabytes. A backlog only delays new pages (section 10), so these could even run on Spot |
| Postgres (`location_pages`, `page_signals`, `page_stats_daily`) | 12M candidate rows 2.4 GB, signals ≈ 0.8 GB (12M × ~64 B), live pages 600k × ~3 KB = 1.8 GB, stats 9 GB a year | memory (hot set), then little | hot set ≈ 2.4 + 0.8 + 1.8 + ~1 index + ~1 recent stats = 7 GB: fits 16 GiB. IOPS: 3.3k signal updates/s in key order dirty ≈ 3.3k × 64 B ÷ 8 KB ≈ 26 pages/s; in random order up to 3.3k/s, so gp3 at 4,000 IOPS. CPU: 3.3k updates + 3.6k render reads/s (worst case, cold CDN) × 0.1 ms ≈ 0.7 of 2 vCPU | `db.r7g.large` (2 vCPU / 16 GiB), RDS Multi-AZ, gp3 100 GB | 1 primary + synchronous standby | the cheapest shape with 16 GiB for the hot set (`m7g.xlarge` has the same RAM and 2× the CPU we do not use: $120 vs $78); Multi-AZ because every CDN miss reads it |

**CDN (CloudFront), S3 and the AI drafts:** nothing to size. The 12 sitemap files (600k URLs × ~100 B ≈ 60 MB) sit in S3. Egress is the bill: 20M views a day × ~100 KB (assumed average: ~15 KB of compressed HTML plus images, CSS and JS over first and repeat visits) = 2 TB/day ≈ 61 TB a month. The 590k tier-2 intros are ~0.5B tokens one-off (590k × ~900), spread over waves: a hosted model API priced per token (not in the sheet), not GPUs.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| CloudFront egress: 61 TB × $0.085/GB (first-tier price, an upper bound) | $5,200 |
| Page servers: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Batch workers: 2 × `c7g.large` (2 × $53) | $110 |
| Postgres: `db.r7g.large` ≈ $156, Multi-AZ doubles | $310 |
| gp3 100 GB × 2 (standby) × $0.08, plus 1,000 extra IOPS × 2 × $0.005 | $30 |
| ALB for origin traffic | $20 |
| **Total** | **≈ $6,000** (≈ $5,700 with a 1-year plan on the $740 of instances and RDS; egress and storage not discounted) |

- **Biggest line and why:** CloudFront egress, ~87 % ($5,200 of $5,990): the hardware is only ≈ $790 a month. The lever is page weight, not servers: each 10 KB of average page weight is ≈ $520 a month (20M × 10 KB = 200 GB/day × 30.4 × $0.085), so lazy images and long-cached, shared CSS and JS are worth more than any instance change. The AI intro tokens are a one-off third-party cost outside this table.
- **What changes at 10×:** 120M candidates, 6M live (section 10). The first thing to run out is the signal refresh: 120M ÷ 3,600 = 33k rows/s, which in random order is ~33k page writes/s, past gp3's 16,000 IOPS cap; keep key order (≈ 260 pages/s) and re-gate only dirty pairs from the outbox, as section 10 says. The hot set becomes ~70 GB (24 + 8 + 18 + ~10 + ~10), more than 64 GiB, so `db.r7g.4xlarge` (128 GiB, ≈ $620 × 2 managed × 2 Multi-AZ ≈ $2,480). The simhash scan grows with the square of pages per service: 4.5 × 10¹⁰ popcounts ≈ 45 s on one core, still no LSH, while all candidates would be 1.8 × 10¹³ ≈ 5 hours, which is why the gate runs first. Page servers stay at 3 behind a 90 % CDN hit rate; if views also grow 10×, egress becomes ≈ $52k and page weight becomes the project.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- A matrix of 12M candidate rows (~2.4 GB, key `(service_id, place_id)`), 12M `page_signals` rows (~0.8 GB) and ~600k live pages (~1.8 GB): a hot set of ~7 GB (sections 4, 15).
- The gate joins signals to pages and runs on the dirty set (a nightly pass is at most ~3.3k row updates/s), under one hard invariant: nothing is `live` without `gate_passed_at` and a non-empty intro (`live_needs_gate`, section 8).
- Reads are small: ~230 views/s (1.2k/s peak) behind a CDN, each a primary-key lookup, plus hubs and sitemaps that list live pages by state and wave. A wave rollback is one `UPDATE … WHERE wave_id = $1` (9.4).
- Stats: 600k pages × 365 days ≈ 219M rows a year (~9 GB), read as cohort aggregates by `(wave, tier, service)`.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the `CHECK` refuses a `live` row that skipped the gate, from every writer (a script, an import); the partial index `WHERE state = 'live'` covers 600k rows instead of 12M; the gate's join, the cohort `GROUP BY`s and the rollback are plain SQL in one transaction. Cost: each nightly update leaves a dead row version, so autovacuum must keep up with `page_signals` |
| MySQL (RDS / Aurora) | ✔ works, with one workaround | `CHECK` is enforced from 8.0.16 (`btrim` becomes `TRIM`); there is no partial index, so the live lookup is a full index over 12M rows, 20× the entries. InnoDB's clustered key `(service_id, place_id)` makes the nightly key-order pass sequential by construction, and there is no vacuum |
| DynamoDB | ✖ poor fit | the gate is a join and ad-hoc counting ("which candidates fail only the demand gate?"), and there is no table-level constraint: "live implies gated" would hold only where each writer adds a condition, which a script can skip |
| ClickHouse / Athena (stats only) | ✖ not yet | 219M rows a year is well below the billions where an analytics store pays; one cohort query reads 600k pages × 30 days = 18M rows |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.large`, gp3 100 GB, section 15), one database for the matrix, signals and stats. The decisive feature is the `CHECK` that turns "published implies passed the gate" into a database invariant (9.1), with the partial index keeping the live lookup small. Sitemap files (S3) and rendered pages (CDN) are derived from `state`; the simhash comparison runs in memory (96 MB of hashes, section 15), so nothing else needs a store.

**If it were MySQL instead:** mostly portable. `jsonb` becomes `JSON`, the partial index becomes a full `(state, service_id, place_id)` index on 12M rows, and the `CHECK` works on 8.0.16 and later (earlier versions parse and ignore it, so check the engine version first). The monthly partition of `page_stats_daily` is fine because its key already contains `day`.

**What would flip the choice:** at 10× (120M candidates, 6M live, hot set ~70 GB on a `db.r7g.4xlarge`, section 15) one primary still holds. The first part to leave is `page_stats_daily`, ~2.2B rows a year: move it to ClickHouse or Parquet in S3 with Athena. If the matrix itself outgrows one primary, shard by `service_id`: the key starts with it and the duplicate scan already works per service (9.2).

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

Here the limits are search-engine quality and people before hardware; places multiply, services stay at 400, so duplicate checking grows with the square of pages per service.

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 1.2B candidates, ~60M live (5 %); 2B views a day (23k/s average, 120k/s peak); 1,200 sitemap files; 4.5 × 10¹² eligible pairs; stats 22B rows a year (900 GB) | not serving (12k origin req/s ≈ 36 vCPU behind a 90 % CDN). The one Postgres: hot set ≈ 700 GB, ~5× the 128 GiB node, and a full re-gate is 333k signal writes/s, 33× a primary. And people: tier 1 stays ~10k human-edited pages, 0.017 % of 60M, so quality rests on gates and sampled review | shard `location_pages` and signals by `service_id` (as in section 16: the scan already works per service), re-gate dirty pairs only from the outbox, stats to Parquet or ClickHouse; keep the waves (1k, 5k, 25k, 100k, 600k, 6M, 60M: weeks each, one sitemap file per wave); the per-service simhash scan is ~75 core-minutes (~70 s on 64 cores), still no LSH. Give up: cross-service queries (cohorts move to the warehouse) and speed: full coverage takes months by design | ≈ $600k a month, egress $520k of it (87 %) |
| **1,000×** | 12B candidates (more than people alive: a thought experiment), ~600M live; 20B views a day (231k/s average, 1.2M/s peak); 12,000 sitemap files; 4.5 × 10¹⁴ eligible pairs | the quadratic duplicate check: 4.5 × 10¹⁴ popcounts ≈ 125 core-hours a pass (7.5 min on 1,000 cores, every night), and a new page compared against its 1.5M siblings online; 600M URLs is 1,000× the set that already needs waves | change the shape: LSH bands (4 × 16-bit band tables per service: a lookup instead of a scan, complete for distance ≤ 3 by pigeonhole, 9.2); pre-render live pages to S3 and rebuild on change, so 1.2M views/s never reach a page server or Postgres; independent per-service-group cells (own Postgres, gate, sitemap shards, wave controller) behind the `/{service}/…` path; cohort stats sampled in a warehouse. Give up: per-view freshness (minutes of rebuild lag), simplicity (band tables, candidate checks, cell routing) and cross-cell cohort comparisons | ≈ $6M a month, egress $5.2M: bytes, not servers |
| **10,000×** | 120B candidates (15 per person alive), ~6B live; 200B views a day (2.3M/s average, 12M/s peak: a hyperscale service's whole peak); AI intros for 6B pages ≈ 5T tokens (0.5B today) | the premise, not a component: 6B live pages from one site is not a set a search engine is likely to crawl and index in full (a risk judgement, not a measured limit), and egress alone is ≈ $52M a month (61 TB × 10,000 × $0.085) | change the problem: a fixed budget of indexable pages (say the top 50M by demand: a policy number), with gate thresholds set from the budget so that many pass; everything below is not a page but a section of its service hub and place hub, or on-site search results marked `noindex`; LLM intros only for tier 1; page weight capped (each 10 KB ≈ $5.2M a month). Give up: long-tail coverage (reached through hubs and search, not its own URL), per-page experiments, a uniform URL scheme | ≈ $60M a month if nothing changes (egress $52M): page weight is the lever |

**What stays the same at every scale:** the `live_needs_gate` invariant (live only if a versioned gate job wrote `gate_passed_at`), distinctness measured on the variable part only, honest status codes (a real 404 for not-live), and waves with a stop-loss.

**Say it in an interview:** "At 100× servers are not the problem: the CDN absorbs the views. The one Postgres is, with a 700 GB hot set, so I shard by service and re-gate only dirty pairs, and humans still write only the top 10k pages. At 1,000× the duplicate scan is quadratic, 125 core-hours a pass, so I use LSH bands, pre-rendered pages and cells per service group. At 10,000× there would be 6B pages, which no search engine will index in full, so I fix a budget of pages and turn the rest into hub sections and search."
