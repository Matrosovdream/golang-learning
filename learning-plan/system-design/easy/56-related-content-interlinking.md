# 56 · Related Content & Interlinking

`🟢 easy` · *Topics:* link edges from editors, body text and computed similarity, materialised top-N neighbours instead of per-request search, keeping links valid when pages are unpublished or renamed, orphan / dead-end / click-depth from an in-memory graph, serving the block inside cached HTML, internal links as crawl and SEO input · *Builds on:* [32 Block-Based CMS](32-block-cms.md) (the content model and cache purge), [39 Global Search](39-global-search.md) (the link picker's search), [34 Crawler-Ready SPA Serving](34-crawler-ready-spa.md) (links must exist in the crawler's HTML), [35 Redirect Manager](35-redirect-manager.md) (renamed addresses), [33 Sitemaps](33-sitemaps-crawl-notify.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [55 Order Timeline with Photo Steps](55-order-timeline.md) · Next: [57 Event Registration with Capacity & Waitlist](57-event-registration-waitlist.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 76 (migrations 0094–0099), phase 76.6 *Interlinking*: **owner-curated, never automatic keyword links**; the step's own "Not doing" list also rules out mass-generated articles ("scaled low-value pages are what search engines treat as spam"). `content_links` (0096) is one directional table across six kinds: a row means *the page of `from` offers `to` as further reading*, unique per pair, at most 30 per record, saved in the same `PUT` and transaction as the record. A link to a draft, archived or removed record is kept and flagged in the editor, never silently dropped, and every public read leaves non-live targets out. `content_refs` (0098) records what a body's *text* links to, **by address**, kept current by database triggers; the view `content_records` spells the one live rule (`live` · `draft` · `empty` · `archived` · `removed`). AFTER DELETE triggers forget a deleted record's links; a slug locks once a record has been live. Step 74's Search health flags *orphan* (nothing live links to it) and *dead end* (it leads nowhere). The public blocks show at most 12 of each kind. AI suggestions are explicitly not built. **Nothing is computed in the repo:** related content is exactly what an editor chose or wrote. Computed neighbours, the graph job and the cache design below are the lesson's scale-up.
> **Here, at scale:** a multi-tenant CMS with 50M pages where "related" must appear on every page view (100k views/s), nobody can curate it by hand, and links rot as pages are renamed and unpublished.

## 1. The task

> "Design 'related content' and internal linking for a content platform with tens of millions of pages. Every page shows a few related pages, editors can pin their own, links must not point at pages that are gone, and we want to find pages nothing links to."

It looks like a recommendation problem. It is mostly a **bookkeeping** problem: where edges come from, how they are stored and kept valid, and how a cheap read path serves them.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Who decides what is related? | Curated vs computed changes everything. | Editors can pin links; for the rest the system computes. Curated always wins. |
| Auto-insert links into body text? | SEO and trust. | No. Computed links appear only in a labelled "Related" block; bodies are never rewritten. |
| Directional or symmetric? | Storage and meaning. | Directional: "page A offers B". B need not offer A. |
| Which targets are allowed? | Dead links hurt users and SEO. | Only live, indexable pages of the same tenant (and language). |
| How many per page? | Bounds reads and crawl noise. | 6–8 shown, 12 stored, ≤ 30 curated. |
| What happens on unpublish / rename? | Link rot policy. | Edges survive and are filtered by state; renames create redirects ([35](35-redirect-manager.md)). |
| Freshness? | Batch vs streaming. | A changed page has its neighbours within minutes; the whole corpus is rebuilt weekly. |
| Personalised? | Cache-ability. | Not in the page; "for you" is a separate client call. |
| Scale? | Storage, compute. | 50M pages, 2M changes/day, 100k views/s, tenants isolated. |

## 3. Requirements

**Functional**
- Editors pin up to 30 related pages per page, in order, saved with the page.
- Every page gets computed related pages (top 12 stored) when the editor pinned fewer.
- "Linked from N places" for any page; a list of pages with broken or non-live links.
- Find orphans (no live inbound), dead ends (no live outbound), and click depth from the home page.

**Non-functional**
- Reading the block adds ~0 to a cached page view; a miss costs ~1 ms of database work.
- Unpublishing a page removes it from all blocks within a minute or two.
- No cross-tenant leakage: one tenant's drafts never influence another's neighbours.

**Out of scope**
- Personalised recommendations, external-link checking (see Extensions), ranking by popularity.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Computed edges | 600M rows | 50M pages × 12 |
| Curated edges | ~100M rows | 50M × ~2 average |
| `content_edges` size | ~28 GB, ~55 GB with the inbound index | 700M × ~40 B |
| Text refs | ~200M rows, ~25 GB with index | 50M × ~4 links, ~60 B each |
| Average inbound per page | 14 | 700M ÷ 50M |
| Embeddings | 154 GB float32, ~38 GB int8 | 50M × 768 dims × 4 B (or 1 B) |
| Corpus embedding cost | ~69 GPU-hours (assumption) | 50M pages × 500 tokens = 25B tokens ÷ 100k tokens/s |
| All-pairs similarity | 1.25 × 10¹⁵ pairs: impossible | 50M²/2 |
| Neighbour search for all | ~14 CPU-hours, ~13 min on 64 cores | 50M queries × ~1 ms (assumed ANN latency) |
| Incremental rebuild | ~23 queries/s, ~280 reverse updates/s | 2M changes/day ÷ 86,400; × 12 neighbours ÷ 86,400 |
| Read path | 100k views/s; at 5% cache miss 5k lookups/s | each a ranked 8-row read |
| Link rot | ~20k address/state changes a day × 14 inbound ≈ 280k edge touches, ~3/s | assume 1% of the 2M daily changes |
| Whole graph in memory | ~3.2 GB | 700M × 4 B ids + 50M × 8 B offsets (CSR) |

What the numbers tell us: all-pairs similarity is out, but **approximate nearest-neighbour search** over the corpus is an overnight batch. Edges fit one Postgres (~80 GB). The read path is trivial *because* neighbours are materialised. And the whole link graph fits in **3 GB of RAM**, so graph health needs no graph database.

## 5. Plan

1. Decide where edges come from and who wins when sources disagree.
2. Store edges ranked and materialised; build offline, update incrementally.
3. Keep links valid: states, renames, a broken-link detector, orphans and depth.
4. Serve the block inside cached HTML; keep titles fresh.
5. Respect the crawl and SEO side: small, relevant, visible in HTML.

## 6. High-level design

```
 editor save ─► CMS API ─ one tx ─► content_pages + content_edges(kind=curated) + content_text_refs (parsed from the body)
                                └─► outbox: page.changed {id, version, state, slug}
                  ┌──────────────────────────────┴────────────────────────────────┐
                  ▼                                                                ▼
        link builder (worker)                                          link-health job (worker, nightly + on events)
  embed page ─► ANN index (per tenant) ─► top-12                      inbound/outbound live counts, broken refs,
  taxonomy overlap ─► candidates ─► blend ─► content_edges            orphans, dead ends, click depth (in-memory CSR BFS)
  reverse update: offer the page to its neighbours' lists             ─► link_health, broken_links ─► dashboard / alerts
        └─ bump version ─► CDN tag purge (32) ─►  reader ─► CDN / page cache (34) ─► renderer: page + related block in the HTML
```

**One write.** The editor saves a page: the body is parsed into text refs, the pinned list is saved, and a `page.changed` event is queued, all in one transaction. The builder embeds the page if its text changed, asks the ANN index for neighbours, writes its 12 computed edges, offers the page to those neighbours' own lists, and purges the affected cache tags.

**One read.** A cache hit returns HTML that already contains the block. A miss renders the page and reads the 8 best live edges (`content_edges` joined to `content_pages`), filtering non-live targets and de-duplicating by target with curated first (the same pair may exist as both `curated` and `similar`).

## 7. API

```
GET  /pages/{id}/related?limit=8             → 200 [{ id, type, slug, title, kind }]       (mostly baked into the page)
PUT  /pages/{id}/related   { curated:[{type,slug}, …] }  ≤ 30, same save as the page          → 200 | 422 { rejected:[{slug, reason}] }
GET  /pages/{id}/inbound?cursor=&limit=50    → 200 "Linked from N places" [{ from, via: curated|text|similar }], next_cursor
GET  /admin/link-health?filter=orphan|dead_end|broken&cursor=                                   → 200 paged, worst first
POST /admin/link-health/recompute?scope=changed|all                                             → 202
```

A pinned link to a draft, archived or removed page is **accepted and flagged**, not dropped (the repo's rule: it may come back); a non-existent page, itself, a duplicate or a 31st link is `422`.

## 8. Data model

```sql
CREATE TABLE content_pages (            -- owned by the CMS (32); only what linking needs
    id         BIGINT PRIMARY KEY,
    tenant_id  BIGINT      NOT NULL,
    type       TEXT        NOT NULL,
    slug       TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    state      TEXT        NOT NULL CHECK (state IN ('live','draft','empty','archived','removed')),  -- the repo's content_records states
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, type, slug)
);

CREATE TABLE content_edges (            -- resolved by id: what a page offers as further reading
    from_id    BIGINT   NOT NULL,
    to_id      BIGINT   NOT NULL,
    kind       TEXT     NOT NULL CHECK (kind IN ('curated','taxonomy','similar')),
    rank       SMALLINT NOT NULL,       -- curated: the editor's order; computed: best first
    score      REAL,                    -- NULL for curated
    built_in   INTEGER,                 -- builder version that produced a computed edge
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (from_id, kind, to_id),
    CHECK (from_id <> to_id)
);
CREATE INDEX content_edges_out_idx     ON content_edges (from_id, kind, rank);   -- the block
CREATE INDEX content_edges_inbound_idx ON content_edges (to_id, kind);           -- "linked from", purge, rot

CREATE TABLE content_text_refs (        -- what a body links to, BY ADDRESS as written (the repo's content_refs)
    from_id BIGINT NOT NULL,
    to_type TEXT   NOT NULL,
    to_slug TEXT   NOT NULL,
    PRIMARY KEY (from_id, to_type, to_slug)
);
CREATE INDEX content_text_refs_to_idx ON content_text_refs (to_type, to_slug);

CREATE TABLE link_health (              -- one row per page, rewritten by the graph job
    page_id       BIGINT   PRIMARY KEY,
    inbound_live  INTEGER  NOT NULL,
    outbound_live INTEGER  NOT NULL,
    depth         SMALLINT,             -- clicks from the home page; NULL = unreachable
    computed_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX link_health_orphans_idx ON link_health (page_id) WHERE inbound_live = 0;

CREATE TABLE broken_links (
    from_id  BIGINT NOT NULL,
    to_type  TEXT   NOT NULL,
    to_slug  TEXT   NOT NULL,
    reason   TEXT   NOT NULL CHECK (reason IN ('missing','not_live','redirected')),
    found_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (from_id, to_type, to_slug)
);
```

Text refs stay **by address** (as in the repo) so a link written before its target exists still counts, and renames are an explicit, visible event rather than a silent id swap.

## 9. Deep dives

### 9.1 Where do edges come from?

| Source | Pros | Cons |
|---|---|---|
| Curated by editors (the repo's `content_links`) | precise, intent is clear, wins any argument | cannot cover 50M pages; goes stale |
| Links written in the body (the repo's `content_refs`) | free, accurate, authors meant them | only what was written; by address, so renames break them |
| Shared categories and tags | cheap SQL, explainable | coarse; a big category gives arbitrary picks |
| Embedding similarity (ANN) | covers every page, finds non-obvious matches | GPU and index cost, drifts with the model, occasional odd match, hard to explain |
| Behaviour (co-views) | strong quality signal | cold start, privacy, popularity feedback loop (extension) |

**Choice and why.** Layers with a fixed precedence: **curated first, in the editor's order**, then a blend of taxonomy overlap and embedding similarity (weights tuned offline, versioned in `built_in`). Guardrails on computed edges: both pages live and indexable, same tenant and language, a minimum score, at most 3 from one category (diversity), and **one slot in eight reserved for a page with few inbound links** (new or orphaned), so exploration feeds coverage. Computed links appear only in a labelled block. **Body text is never rewritten with automatic links**: that is the repo's rule, and it is also what search engines punish at scale.

### 9.2 Storage and build: materialise the top-N

| Option | Pros | Cons |
|---|---|---|
| Compute neighbours per request (ANN at read) | always fresh | ~1 ms × 100k views/s = ~100 cores and the vector index in the serving path; unstable results page to page |
| **Materialised, ranked edge rows built offline** | reads are an index range scan; stable; inbound queries come free from the same table | storage; a rebuild pipeline; staleness between builds |
| One JSON blob of related pages per page | one read | cannot answer "who links to X"; every neighbour rename rewrites many blobs |

**Choice and why.** Edge rows. Three build modes:
- **Full:** weekly, per tenant, into a *new* `built_in` version, then flip readers to it (as the media rebuilds in [40](40-media-reprocessing.md)). Delete the old version later. A model change is just a new version.
- **Incremental:** on `page.changed`, if the text hash changed, re-embed, write the page's own 12 edges and **offer the page to each neighbour's list** (a reverse update: it replaces that list's lowest score if better). That is ~280 updates/s, and without it a new page has no inbound links until the weekly pass.
- **Curated:** written synchronously with the page; no builder involved.

The ANN index is per tenant (or per tenant shard) so that neighbours never cross a tenant boundary.

### 9.3 Keeping links valid, and finding orphans

What happens to the edges pointing at a page that leaves `live`?

| Option | Pros | Cons |
|---|---|---|
| Delete the edges | tidy | an embargo, a scheduled unpublish, or an accident loses editors' curation |
| **Keep the edges, filter on `live` at read and build time** (the repo) | they come back with the page; no cleanup storm | every read must join state; truly deleted pages need a trigger or job to drop their edges |
| Rewrite links when a page is renamed or moved | nothing breaks | rewrites authors' text behind their backs |

**Choice and why.** Keep and filter. Hard deletes drop edges as the repo's AFTER DELETE triggers do. Renames create a redirect ([35](35-redirect-manager.md) §9.2) and **do not edit bodies**; the health job lists "links that go through a redirect" so editors fix them at leisure (the repo avoids the problem more bluntly by locking a slug once the record has been live).

**Broken-link detector.** Event-driven, not a full scan: when `page.changed` carries a state or slug change, look up inbound rows through the indexes (`content_edges (to_id)`, `content_text_refs (to_type, to_slug)`) and upsert `broken_links` with the reason. About 3 edge touches per second. A nightly sweep joins text refs to `content_pages` to catch drift.

**Orphans, dead ends, depth.** Load the live edges into **CSR arrays** (`off []uint64`, `out []uint32`: ~3.2 GB for 700M edges), count inbound and outbound per node, then BFS from the home page and hub pages. *Orphan* = no live inbound. *Dead end* = no live outbound. *Unreachable* (depth `NULL`) is stronger than orphan: a cluster of pages that link to each other but that nothing reachable links to. Hubs (very high inbound) matter for purge fan-out (9.4). The lesson on graphs is [53](../../53-graphs-algorithms.md); one machine is enough here.

### 9.4 Serving: the block lives inside cached HTML

| Option | Pros | Cons |
|---|---|---|
| Query edges on every view | simplest | 100k lookups/s, all for data that changes rarely |
| **Bake the block into the cached page HTML** ([34](34-crawler-ready-spa.md) §9.3), tag-purge on change | zero extra reads on a hit; crawlers see real links in the HTML | titles of neighbours are baked in, so a rename must purge every page that shows it |
| Fetch the block client-side from a separate cached JSON | independent TTL | crawlers that do not run JS see no links; layout shift |

**Choice and why.** Bake it in. Each cached page carries tags for the pages it displays; when page X is retitled or leaves `live`, purge `page:X` ([32](32-block-cms.md) §9.4). The inbound index gives the referrers, usually ~14. A hub with 2M referrers cannot be purged inline, so above a cap (say 1,000 tags) switch to **soft purge**: mark stale and re-render lazily (stale-while-revalidate), accepting minutes of an old title on a few pages. Personalised "for you" content is a separate client call after load, never part of the cached page.

### 9.5 The crawl and SEO side

Internal links are an input for crawlers (discovery, depth, relevance), so the constraints are about restraint:
- **Visible in the HTML** ([34](34-crawler-ready-spa.md)), real anchors with the target's title as text, not keyword-stuffed phrases.
- **Small blocks.** 6–8 related plus a bounded "mentioned in" list (the repo caps it at 12). There is no magic number, but a hundred-link footer dilutes everything.
- **Only indexable, live targets.** Never link to `noindex`, redirected, or non-live pages; the sitemap ([33](33-sitemaps-crawl-notify.md)) should list the same set the public lists show.
- **Depth.** Keep important pages within ~3 clicks of the home page or a hub; use the depth column to find the unreachable and the too-deep.
- **No `nofollow` on editorial internal links** (it only wastes the link); use it for untrusted user-generated links.
- **Scaled low-value pages stay a risk.** Computed linking cannot make thin pages worth indexing; the repo's "Not doing: mass-generated articles" applies at any size.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** not storage (~550 GB of edges at 500M pages: partition by hash of `from_id`), but the **embedding and ANN build** (1.5 TB float32 vectors: quantise, shard by tenant and language) and **purge fan-out for hubs**.
- **Builder down:** old edges stay valid (just stale); new pages get only curated and taxonomy edges until it recovers.
- **Bad model release:** the weekly build writes a new `built_in`; readers flip only after sampling quality (overlap with the previous version, spot checks), so rollback is a pointer flip.
- **Cache purge storm:** a mass unpublish of 1M pages produces 14M tag purges; batch them and rate-limit the CDN calls.
- **Tenant isolation:** an index or build job that mixes tenants leaks drafts as "similar to"; the job's input must be `live` pages of one tenant.
- **Single points of failure:** Postgres (reads fall back to cached HTML), the ANN index (rebuildable from vectors), the CDN purge API (soft TTLs bound the damage).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| curated > body > computed layers | pure automation | editor intent always wins; explainable | complexity of a precedence rule |
| materialised ranked edges | ANN at request time | stable, cheap reads, free inbound queries | storage and a rebuild pipeline |
| edges kept, filtered by state | cascade delete | embargoes and unpublishes are reversible | every read joins state |
| text refs by address, no body rewrites | refs by id, silent rewrites | authors' text untouched; renames visible | links via redirects until fixed |
| baked-in block with tag purge | client-side fetch | crawlers see links; zero extra reads | purge fan-out; soft purge for hubs |
| CSR graph in RAM | a graph database | minutes on one box, trivial ops | recompute rather than query live |
| one slot in eight for low-inbound pages | pure relevance | coverage, fewer orphans | slightly less relevant on some pages |

## 12. Check yourself

1. The repo never auto-inserts keyword links into body text. What does this lesson do for 50M pages instead, and what stays forbidden?
2. Why materialise neighbours offline rather than search per request? Use numbers.
3. A page is unpublished. What happens to the edges, the cached pages that show it, and the inbound counts?
4. How do you find orphans, dead ends and unreachable pages among 700M edges without a graph database?
5. A page's slug changes. Why not rewrite every body that links to it?

<details><summary>Answers</summary>

1. It computes a separate, labelled "Related" block from taxonomy overlap and embedding similarity, with curated links always first. It never rewrites body text with automatic links: that is the practice search engines punish at scale and the repo's explicit rule.
2. Per-request ANN is ~1 ms × 100k views/s ≈ 100 cores plus the vector index in the serving path, with results that change between views. Materialised rows make a read a ~1 ms index scan, only on cache misses (~5k/s), and they answer "linked from" for free.
3. Edges are kept and filtered by state (they return if the page does); inbound rows locate referrers via the index; cached pages showing it are purged by tag (soft purge above a cap); `broken_links` rows are written for text refs; live inbound counts drop on the next job run.
4. Load the live edges into CSR arrays (~3.2 GB), count in/out per node, BFS from home and hubs. Depth `NULL` means unreachable. It takes minutes on one machine.
5. It would edit authors' text silently, risk breaking their markup, and still miss external links. A redirect keeps everything working, and the health job lists links that go through redirects for editors to fix.

</details>

## 13. In Go

- Related lessons: [53 Graphs](../../53-graphs-algorithms.md) (BFS, adjacency), [55 Data pipelines](../../55-data-pipelines.md) (the batch build), [59 Production DB patterns](../../59-production-database-patterns.md) (upserts, partial indexes), [38 Caching](../../38-caching-patterns.md), [44 Background jobs](../../44-background-jobs-queues.md), [34 Outbox](../../34-event-driven-outbox.md), [42 Trees](../../42-trees.md) (category trees).
- Packages: `pgx`, `golang.org/x/net/html` and a Markdown parser (`goldmark`) to extract links from bodies, `pgvector` (HNSW) or a vector service for neighbours, `golang.org/x/sync/errgroup` for the batch.

BFS depth over CSR arrays (`-1` means unreachable):

```go
// out[off[v]:off[v+1]] are v's live neighbours; ids are dense uint32 (50M < 2^32).
func depths(off []uint64, out []uint32, roots []uint32) []int8 {
	depth := make([]int8, len(off)-1)
	for i := range depth {
		depth[i] = -1
	}
	cur := append([]uint32(nil), roots...)
	for _, r := range roots {
		depth[r] = 0
	}
	for d := int8(0); len(cur) > 0 && d < 127; d++ {
		var next []uint32
		for _, v := range cur {
			for _, w := range out[off[v]:off[v+1]] {
				if depth[w] == -1 {
					depth[w] = d + 1
					next = append(next, w)
				}
			}
		}
		cur = next
	}
	return depth
}
```

## 14. Extensions

- **Negative curation:** an editor blocks a computed edge (`content_edge_blocks`); the builder skips it forever.
- **The "Suggested links" panel** in the editor: titles of other pages that occur in the text, accepted one by one (never inserted automatically).
- **External link checking:** polite HEAD requests per host, backoff, and a "link is dead" workflow.
- **Behavioural signals:** co-views and click-through on related blocks, with the popularity feedback loop managed.
- **A/B tests** of builder versions measured by click-through and by pages-per-session.
- **Multilingual sites:** `hreflang` siblings as their own edge kind, never mixed into related.
- **PageRank-style authority** from the in-memory graph, to pick which pages deserve hub slots.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). The 95 % of views that hit the CDN need nothing here: the work is the 5 % miss path, one memory-hungry database, one in-memory vector index and a weekly batch. The real money is bytes out, below.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Renderer / reader (Go, stateless: page + related block on a cache miss) | 5k misses/s (5 % of 100k views/s, §4), each with one ranked 8-row read | CPU | 5k × ~2 ms (render, JSON, state filter, dedupe) = 10 vCPU. N = ceil(10 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(2.3) = 3 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 3, one per zone | render and serialise: CPU, with ~2 GiB of memory (an in-process title cache), so `c`. 42 % busy at peak, 63 % with a zone down |
| Postgres (edges, pages, text refs, link_health) | 5k ranked reads/s on misses; ~560 edge-row writes/s (23 pages/s × 12 own + ~280 reverse updates/s); a weekly 600M-row build; ~80 GB (§4: 55 GB edges + 25 GB refs) + ~13 GB of page rows (assumed) | memory, then IOPS | the read path touches edges 55 GB + pages ~13 GB ≈ 68 GB at random (misses are the long tail). Not in RAM, each miss is ~10 random reads: 5k × 10 = 50k IOPS, past `gp3`'s 16k cap. So RAM ≥ 68 GB plus a buffer split ⇒ 128 GiB. CPU: 5k × 0.3 ms = 1.5 vCPU + writes ≈ 3 of 16. Writes ~560 IOPS, the build throttled over 3 days = 2.3k rows/s: inside `gp3`'s 3,000 | `db.r7g.4xlarge` (16 vCPU / 128 GiB), RDS Multi-AZ, 300 GB `gp3` | 1 primary + synchronous standby | memory-bound, so `r`: fully cached, reads cost no IOPS and the box idles at ~20 % CPU. `r7g.2xlarge` (64 GiB) is cheaper but ~30 % of reads then miss: 5k × 30 % × 10 = 15k IOPS (≈ $60 of provisioned IOPS), at `gp3`'s ceiling with EBS-latency tails: not chosen. Add a read replica only if the weekly build disturbs reads |
| ANN index (HNSW, per tenant) | 50M × 768-dim vectors; ~23 neighbour queries/s incremental; 50M queries weekly | memory | int8 vectors 38.4 GB (§4) + graph (16 links × 2 × 4 B = 128 B per vector) 6.4 GB = 45 GB; × 1.3 headroom = 58 GB; a second copy while a rebuild flips ≈ 90 GB ⇒ 128 GiB. Incremental 23 × 1 ms = 0.02 vCPU. Weekly search 14 CPU-h (§4) ÷ 16 vCPU ≈ 52 min | `r7g.4xlarge` (16 vCPU / 128 GiB), self-run HNSW or `pgvector` (managed: OpenSearch k-NN, ≈ 2×) | 1 | memory-bound, so `r`. The 154 GB float32 originals live in S3 (≈ $4 a month) and the index is rebuildable; "builder down" is survivable (§10), so no standby |
| Embeddings (GPU) | 23 changed pages/s × 500 tokens = 11.5k tokens/s; 1B tokens/day; a full re-embed is 69 GPU-h (§4) | GPU | 1B ÷ 100k tokens/s = 10,000 s = 2.8 GPU-h a day; one GPU is ~12 % busy at peak | `g6.xlarge` (1 × L4), run ~3 h a day; a Spot burst for a model change | 1 (0 when idle) | 1B tokens a day is far above the few-million-a-day point where a hosted embeddings API wins (cheat sheet), so own the GPU. The only non-`c`/`m`/`r` box in the lesson |
| Link builder + link-health job (Go) | weekly: blend and write 600M edges; nightly: BFS over 700M edges | memory (graph job), then CPU (blend) | blend 600M × 20 µs = 3.3 CPU-h + taxonomy candidates 50M × 0.5 ms = 6.9 CPU-h ≈ 10 CPU-h ⇒ ~1.3 h on 8 vCPU. Graph: edge list 5.6 GB + CSR 3.2 GB + counts ≈ 10 GB at peak ⇒ 16 GiB | `m7g.xlarge` (4 vCPU / 16 GiB) | 2 | `m`: neither pure CPU nor a big resident set, and a `c` node (8 GiB) cannot build the graph. Incremental builds (23 × ~10 ms = 0.2 vCPU) ride along; one node would do, the second is for availability |

Data out is the real line. The block is ~8 links × ~125 B ≈ 1 KB per page view: 100k views/s × 1 KB = 100 MB/s = 0.8 Gbit/s. If that is sustained, it is ≈ 263 TB a month (an upper bound: volume tiers are cheaper, and a 3× peak-to-average ratio divides it by 3). The pages themselves, the ALB and the CDN's origin fetches belong to [32](32-block-cms.md) and [34](34-crawler-ready-spa.md); only the block's own bytes are counted here.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Renderers: 3 × `c7g.2xlarge` (3 × $212) | $640 |
| Postgres: `db.r7g.4xlarge` ≈ $1,240 managed, Multi-AZ doubles | $2,480 |
| gp3 300 GB (× 2 for the standby) | $50 |
| ANN node: `r7g.4xlarge` | $620 |
| Embeddings: `g6.xlarge` ~90 h × ≈ $0.8/h (a price the sheet does not list; check it) | $70 |
| Builders: 2 × `m7g.xlarge` (2 × $120) | $240 |
| Data out for the related block: 263 TB × $0.085 (sustained, first-tier price) | $22,400 |
| **Total** | **≈ $26,500** (≈ $25,100 with a 1-year plan on the $3,980 of steady compute; ≈ $11,600 if 100k views/s is a 3× peak) |

- **Biggest line and why:** the block's bytes (≈ 85 %): a feature that "adds ~0 to a cached page view" still ships ~1 KB to 100k views a second. Servers are only ≈ $4,100, of which the memory-sized Postgres is 60 %. Levers: brotli on the block (anchor lists shrink 2–3×), 6 links instead of 8, shorter titles, and a higher CDN hit rate.
- **What changes at 10×:** 50k misses/s ⇒ 100 vCPU ⇒ N = ceil(100 ÷ 4.27) = 24 × `c7g.2xlarge`. Edges reach ~550 GB (§10), more than one node's RAM, so shard by hash of `from_id` over ~6 × `db.r7g.4xlarge`. The ANN index becomes 384 GB int8 + graph ≈ 580 GB with headroom ⇒ ~6 × `r7g.4xlarge`, sharded by tenant. One re-embed is 690 GPU-h ⇒ fan out ~23 `g6.xlarge` on Spot for 30 h. Data out, ≈ $224k at the first-tier price, is the first thing to attack.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Relational and modest: ~700M edge rows (600M computed, ~100M curated) at ~55 GB with the inbound index, ~200M text refs at ~25 GB, plus ~13 GB of page rows ≈ 80 GB (sections 4 and 15).
- Two access paths on one table: a page's ranked outbound list (`from_id, kind, rank`; 5k reads/s on cache misses) and its inbound list (`to_id, kind`: "linked from", purge, link rot). Writes are ~560 edge rows/s plus a weekly 600M-row build.
- Two structures that are not rows: 50M × 768-dimension embeddings searched by nearest neighbour (154 GB as float32, ~38 GB as int8), and the link graph, 3.2 GB as CSR arrays in a Go process, walked by BFS for orphans, dead ends and depth.
- Mostly derived data: computed edges come from a weekly versioned build (`built_in`); only curated edges and text refs are authored.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for pages, edges, refs and health | key and index lookups plus upserts: the PK `(from_id, kind, to_id)` allows one row per pair and kind (a pair may be both `curated` and `similar`, 9.1), the two edge indexes serve the block and "linked from", `ON CONFLICT` upserts `broken_links`, and the partial index `link_health_orphans_idx WHERE inbound_live = 0` serves the orphan list; the ~80 GB fits the 128 GiB of the `db.r7g.4xlarge`, so a read costs no IOPS |
| MySQL (RDS / Aurora) | ✔ works for the edges, ✖ for the vectors | the edge tables need only primary-key and secondary-index lookups and upserts; InnoDB's clustered PK keeps a page's edges together and each secondary index already carries the other id, so both reads are covered by the index; the orphan index becomes a plain `(inbound_live, page_id)`; but the managed MySQL engines have no pgvector equivalent |
| pgvector (PostgreSQL extension) | ✔ works as the derived ANN index, if it benchmarks | HNSW over `vector` or `halfvec` columns keeps neighbour search in SQL and is one system fewer, but 50M × 768 is the size to measure (build time, recall, memory). Section 4's 38 GB is an int8 figure; pgvector's compact type is `halfvec` (2 bytes per dimension), so ≈ 77 GB plus the graph, and section 15's rebuild flip (two copies at once) would not fit one 128 GiB node |
| Dedicated vector store (OpenSearch k-NN, a vector database) | ✔ works as the derived ANN index | dedicated engines typically offer quantisation (the int8 sizing), sharding by tenant and filtered search; the price is a second system fed by the same `page.changed` events, rebuildable from the vectors in S3 and the pages |
| Graph database (Neptune) | ✖ poor fit | the link graph is 3.2 GB as CSR arrays and BFS over 700M edges takes minutes on one machine (9.3); one-hop questions ("linked from", the block) are index lookups, so a graph store would hold a second copy of 700M edges to answer a nightly batch |

**Pick:** PostgreSQL on RDS Multi-AZ, the `db.r7g.4xlarge` of section 15, for pages, edges, text refs, `link_health` and `broken_links`. Authored data (curated edges, text refs) is the record; computed edges, health and broken-link rows are derived from it and from the vectors, and are rebuilt by the weekly versioned build. The vector index is a separate rebuildable node (self-run HNSW or pgvector on an `r7g.4xlarge`, or OpenSearch k-NN; benchmark the two at 50M), and the graph is an in-memory CSR rebuilt nightly, so there is no graph database. What decides it: the whole relational set fits in RAM, the edge reads are two index paths, and everything heavy is a derived index that can be rebuilt.

**If it were MySQL instead:** the edge, text-ref and health tables port as they are, with `INSERT … ON DUPLICATE KEY UPDATE` for the upserts and the plain composite index for orphans; the 128 GiB sizing still holds (the InnoDB buffer pool takes the place of `shared_buffers` plus the OS cache). The one real change is the vectors: with no pgvector in managed MySQL the ANN index is a dedicated engine, which section 15 already lists as the alternative.

**What would flip the choice:** at 10× (500M pages) the edges reach ~550 GB: shard by hash of `from_id` over ~6 nodes (section 15), or move the edge table alone to DynamoDB, since after materialising the access is by key (a page's ranked list, plus an index on `to_id` for inbound), at the price of the state-filter join and the SQL sweeps. The vectors (~380 GB as int8) move to a dedicated engine sharded by tenant. A graph database only if editors need interactive multi-hop queries instead of a nightly batch.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 5B pages, 10M views/s (500k cache misses/s ≈ 1,000 vCPU of renderers), ~56k edge-row writes/s; 70B edges ≈ 9 TB of rows (≈ 6.8 TB hot), 3.8 TB of int8 vectors, a 320 GB CSR graph; the block alone is 10 GB/s ≈ 80 Gbit/s ≈ 26 PB a month | the one Postgres node: 6.8 TB hot against 128 GiB of RAM is ~50× over, so misses go to disk: 500k × ~10 random reads = 5M IOPS against `gp3`'s 16k cap. The 16 GiB graph job breaks too (320 GB of CSR) | shard edges by hash of `from_id` over ~60 `db.r7g.4xlarge` (section 15's 10× plan times ten) with a second copy keyed by `to_id`, so "linked from" and purge touch one shard; copy the target's `state` onto each edge row instead of joining pages (a state change updates its ~14 inbound edges). ANN index sharded by tenant and language over ~60 `r7g.4xlarge`; ~237 `c7g.2xlarge` renderers; the CSR job on one `x2gd.16xlarge` (1,024 GiB); a re-embed on ~100 Spot GPUs for 69 h. Give up: the edge-to-page join, one-shard inbound counts, easy resharding | ≈ $2.65M/month: data out ≈ $2.2M (85 %, first-tier price), the 60 + 60 big nodes ≈ $190k, renderers ≈ $50k |
| **1,000×** | 50B pages, 100M views/s (far above any real service's peak: a thought experiment), 5M misses/s ≈ 10,000 vCPU, ~560k edge-row writes/s; 700B edges ≈ 90 TB, 38 TB of int8 vectors; the block is 100 GB/s ≈ 263 PB a month | running it: ~600 edge shards + ~600 ANN nodes + ~2,350 renderers ≈ 3,500 nodes (a failure every ~1.5 h), purge fan-out for hubs, and one weekly build and one model version for the whole corpus: a bad release now hits every page at once | ~100 **cells**, each section 15's 10× design (500M pages: 6 edge shards, 6 ANN nodes, 24 renderers), behind a `tenant_id → cell` router (a tenant never spans cells; one over 500M pages is split by language). Each cell builds, embeds and purges on its own schedule and flips its `built_in` alone, so a bad build or a purge storm stays inside one cell. Give up: global jobs (one weekly build, one model everywhere), cross-tenant anything (there was none by design), manual splitting of a giant tenant | ≈ $26.5M/month (100 cells × ≈ $265k); data out ≈ $22M at the first-tier price, far less on a committed CDN contract |
| **10,000×** | 500B pages, 1B views/s (every internet user would load ~17,000 pages a day: a thought experiment), 50M misses/s; 7T edges ≈ 900 TB; the block is 1 TB/s ≈ 8 Tbit/s ≈ 2.6 EB a month; one full re-embed ≈ 690,000 GPU-h | bytes and money: ≈ $220M a month for the 1 KB block alone, and embedding and ranking 500B pages one by one (12 edges each, 6T rows) is not worth what a related link earns | change the problem: related becomes approximate and shared. Quantise vectors into ~1M clusters; a page stores a `cluster_id` and shows its cluster's top 8 (one list per cluster, cached once at the edge); only the ~1 % of pages with most of the views keep per-page neighbours and the tail gets taxonomy links only; embed only head and changed pages; ship ~4 links (brotli, ≈ 250 B). Give up: exact per-page neighbours on the tail, freshness on tail pages, 8 links | ≈ $265M/month if nothing changes (85 % bytes); 4 brotli'd links cut that line ≈ 4× |

**What stays the same at every scale:** curated links win and body text is never rewritten; neighbours are materialised and ranked offline, never searched per request; edges are kept and filtered by page state, so an unpublish is reversible.

**Say it in an interview:** "At 100× the one Postgres node holds ~50× more hot edges than it has RAM, so I shard edges by `from_id` with an inbound copy by `to_id` and shard the vector index by tenant: the same design scaled out. At 1,000× the risk is running ~3,500 nodes and one global weekly build, so I split into ~100 cells of the 10× design behind a tenant router. At 10,000× the 1 KB block alone costs ~$220M a month, so related becomes approximate: cluster-level lists, per-page neighbours only for the head, and four links."
