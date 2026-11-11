# 34 · Crawler-Ready SPA Serving

`🟢 easy` · *Topics:* what the first response must contain for each kind of client, SSR vs prerender vs dynamic rendering vs head-only injection vs head-plus-body templates, bot classes and verification (UA + reverse DNS), a bot lane that cannot starve humans, tiered HTML caching, honest status codes from an SPA shell, one data model projected to HTML, JSON-LD and Markdown · *Builds on:* [03 Rate Limiter](03-rate-limiter.md) (lanes and limits), [05 Cache with TTL](05-kv-cache-ttl.md) (L1), [11 Image Upload & Hosting](11-image-upload-hosting.md) (CDN, immutable assets), [32 Block-Based CMS](32-block-cms.md) (the content being rendered)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [33 Sitemaps & Crawl Notifications](33-sitemaps-crawl-notify.md) · Next: [35 Redirect Manager & 404 Tracker](35-redirect-manager.md)

---

> 🌱 **Seed (honeydolist-site):** ADR 0001 proposed Nuxt SSR; ADR 0002 rejected it for a **client-rendered Vue SPA whose Go
> backend serves `index.html` with a per-route `<head>` injected from the database** (title, canonical, Open Graph,
> JSON-LD via `html/template` and `json.Marshal`, never string concatenation). A missing record is a real `404`, a
> database error is an uncached `503` with `Retry-After` (never a 404 that would teach crawlers a page is gone), pages
> are cached in memory by path (5 min, 404s 30 s) and purged on every admin save. Measured (Step 73, with each bot's
> user-agent string): every bot got the same 7,197-byte shell with `<div id="app"></div>`; AI fetchers are, by their
> published descriptions, plain fetchers that run no script, so they could quote a title and one sentence.
> **Step 73** then wrote the page's words as plain HTML inside `#app`, replaced on mount (no layout shift measured), from
> the same use cases as the API; added `ETag`/`Last-Modified` with `304`s; Step 85 serves a **Markdown copy of every page**
> built from the same `Body`, plus `llms.txt`/`llms-full.txt`; Steps 92–93 add `ImageObject` JSON-LD and images in
> `<noscript>`. A *number-parity test* proves every number in the API payload appears in the body and the reverse.
> **Here, at scale:** 20M pages, 30% of traffic is bots and link-preview scrapers, and the origin must survive crawl
> spikes of 20k requests/s on mostly-cold URLs.

## 1. The task

> "Our storefront is a single-page app with 20 million product and content pages. Search engines, link-preview
> scrapers and AI assistants must see real content, correct metadata and correct status codes, without changing what
> humans see, and without a crawl spike taking the site down."

An SPA's first response is an empty shell. Everything here is about what to put in that response, who gets it, how it
is cached, and how to keep one source of truth for three outputs.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Who reads the page without running script? | Decides how much goes in the HTML. | Link-preview scrapers (head only), AI/answer fetchers (body), Bing (partial JS), Google (renders, deferred). |
| Is content DB-backed and editable live? | Rules out build-time prerender for everything. | Yes, edits visible in seconds. |
| Is Nuxt/Next an option? | The big alternative. | Not now; revisit with explicit triggers (§9.1). |
| Page kinds? | The cost of a second renderer. | 12 kinds (home, list, detail, category, article…). |
| Do bots get the same HTML as humans? | Cloaking risk. | Always identical bytes. |
| Crawl spikes? | Capacity and shedding. | 20k req/s for hours, mostly cold long-tail URLs. |
| Freshness vs load? | TTLs and purge. | 5 min TTL, purge on publish within seconds. |
| Scale of humans? | The cache hot set. | ~21k req/s peak of HTML from humans; bots add ~9k/s normally. |
| Statuses that matter? | Soft-404 cost. | 200, 301/308, 404, 410, 503 all real. |

## 3. Requirements

**Functional**
- The first response carries: a correct `<head>` (title, description, canonical, Open Graph, robots meta, JSON-LD),
  the page's content as plain HTML inside `#app` (replaced on mount), and the bootstrap data for the app.
- Real status codes decided **before** any HTML is built.
- A Markdown copy of each page and `llms.txt` for agents, from the same data.
- Conditional requests (`ETag`, `If-None-Match`, `HEAD`) so crawler revisits are cheap.

**Non-functional**
- Humans' p99 stays flat while bots spike: bots are a lower-priority lane.
- Cold render ≤ 10 ms server-side; cache hit ≤ 2 ms.
- No cloaking, no hidden text, no content only bots see.
- Hostile data (`</script>`, `<img onerror>`) stays inert.

**Out of scope**
- Hydration strategy inside the SPA, A/B testing, personalisation, the sitemap ([33](33-sitemaps-crawl-notify.md)).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Pages | 20M | given |
| Rendered response | ~25 KB raw, ~5 KB gzip | head ~3 KB + crawler body ~10 KB + bootstrap JSON ~12 KB (assumption); honeydolist's *empty* shell is 7.2 KB |
| Whole corpus | 500 GB raw / 100 GB gz | 20M × 25 KB; 20M × 5 KB |
| HTML requests, peak | ~30k/s | ~21k humans + ~9k bots (30%) |
| Crawl spike | +20k/s, cold | given |
| Cold render | ~3 ms CPU + 3 indexed queries | Go template + use cases (assumption) |
| Unprotected spike | 60 cores, ~60k qps to the DB | 20k × 3 ms; 20k × 3 queries; one Postgres does ~10k+ indexed reads/s |
| Hot set for RAM | ~5 GB | top 200k pages × 25 KB (1 GB gzipped) |
| Snapshot reads in a spike | ≥ 4 S3 prefixes, use 16 | 20k ÷ 5,500 GET/s per prefix ≈ 3.6 |
| Re-render everything (template change) | ~16.7 CPU-hours ≈ 10 min on 100 cores | 20M × 3 ms = 60,000 CPU-s |

What the numbers tell us: a *cold* spike cannot reach the database (60k qps) or the renderer (60 cores) unprotected. If
95% is answered from **snapshots** (pre-rendered HTML in object storage) the residue is 1k renders/s ≈ 3 cores and 3k
qps, comfortable. Capacity work is about *not rendering twice* and *shedding bots first*, not about faster templates.

## 5. Plan

1. Decide what the first response contains for each kind of client; pick the rendering strategy.
2. One `Page` model built by the use cases the API already uses; three projections (HTML body, JSON-LD, Markdown).
3. Resolve the route and the status code before rendering.
4. Cache tiers and keys; snapshots written on publish.
5. Bot lanes, verification and shedding.
6. Failure behaviour: deploys, stale-if-error, cold start.

## 6. High-level design

```
 humans │ Googlebot/Bingbot │ AI fetchers │ link-preview scrapers
        ▼
 CDN / WAF ── cache (s-maxage + SWR) ── "verified bot" flag ── per-class rate limits
        │ miss
        ▼
 Shell service (Go) ─ lane: human │ bot  (separate concurrency pools; bots shed first)
   1. route manifest → resolver ⇒ kind + keys + STATUS (200/301/404/410) — before any HTML
   2. L1  in-process LRU: hot pages (5 min), one shared 404 body
   3. L2  snapshot store: pre-rendered html.gz per page, 16 key prefixes, written on publish
   4. L3  render: use cases → Page model → head + crawler body + bootstrap JSON (html/template)
        ▼
 Postgres replicas      publish ─► outbox ─► snapshotter (render, PUT, purge CDN by tag)
```

A response is `<head>` + `<div id="app">`*crawler body*`</div>` + `<script type="application/json" id="boot">` + the
bundle `<script>`. The SPA empties `#app` on mount, so humans never see the plain version for more than a moment.

## 7. API

```
GET  /{path}        → 200 HTML | 301/308 Location | 404 | 410 | 503 + Retry-After + Cache-Control: no-store
HEAD /{path}        → same status and headers, no body
GET  /{path}.md     → 200 text/markdown; Link: <https://…/path>; rel="canonical"   (same Page model as the HTML)
GET  /llms.txt      → 200 text/plain        GET /llms-full.txt → 200 (built once at a time, cached 5 min)
Response headers:   ETag: "<hash of the bytes sent>"   Last-Modified: <when these bytes were first made>
                    Cache-Control: public, max-age=0, s-maxage=300, stale-while-revalidate=60, stale-if-error=86400
                    Vary: Accept-Encoding   X-Robots-Tag: noindex (only where intended)
If-None-Match / If-Modified-Since → 304
```

## 8. Data model

```sql
CREATE TABLE page_snapshots (
    page_id      BIGINT PRIMARY KEY,
    tmpl_version INTEGER NOT NULL,                -- bump to re-render everything lazily
    content_hash BYTEA   NOT NULL,                -- of the Page model that was rendered
    etag         TEXT    NOT NULL,
    object_key   TEXT    NOT NULL,                -- 'html/<prefix 0-15>/<tenant>/<page_id>.html.gz'
    rendered_at  TIMESTAMPTZ NOT NULL,
    last_modified TIMESTAMPTZ NOT NULL            -- first time THESE bytes existed: survives re-renders that yield identical bytes
);
CREATE INDEX page_snapshots_stale_idx ON page_snapshots (tmpl_version, rendered_at);   -- backfill order

CREATE TABLE tombstones (                         -- why a deleted page is a 410/301, not a 404
    path        TEXT PRIMARY KEY,
    status      SMALLINT NOT NULL CHECK (status IN (301, 308, 410)),
    location    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 410) = (location IS NULL))
);

CREATE TABLE bot_ip_verdicts (                    -- or Redis with a 24 h TTL
    ip          INET PRIMARY KEY,
    bot         TEXT,                             -- 'googlebot' | 'bingbot' | NULL (claimed but unverified)
    verified_at TIMESTAMPTZ NOT NULL
);
```

Snapshot bodies live in object storage; Postgres holds only the index. The *route manifest* is the site's `pages`/slug
tables plus code patterns, shared by the Go resolver and the Vue router (a test asserts every frontend route resolves
non-404 on the backend; honeydolist's `pages` table doubles as that manifest).

## 9. Deep dives

### 9.1 What does the first response contain, and who renders it?

| Option | Pros | Cons |
|---|---|---|
| **Full SSR** (Nuxt/Next on a Node pool) | one component tree renders for bots and humans; hydration | a Node fleet in the request path; JS render CPU ≫ a Go template; cold crawl spikes hit it hardest; SSR-safety rules (`window`) |
| **Build-time prerender (SSG)** | static files on a CDN, fastest | 20M pages = hours per build; an edit waits for a build; can't show a DB change live |
| **Dynamic rendering** (headless Chrome for bots only) | no app change | Chrome in production, hundreds of ms to seconds per page (thousands of cores at 20k/s); two audiences invite cloaking; Google calls it a workaround |
| **Head-only injection** (honeydolist ADR 0002) | ~80 lines; link previews correct; Google indexes via its render pass | AI fetchers and weaker renderers see no body; Google's render queue adds delay |
| **Head + crawler body from Go templates, replaced on mount** (Step 73) | no script needed by any bot; same use cases as the API; cheap | a **second renderer** beside Vue: drift you only see with JS off |

**Choice: head + crawler body, with a parity test.** The body is the *same content a person sees*, not different
content for bots, so it isn't cloaking. The drift risk is managed, not ignored: the number-parity test (every number in
the API payload appears in the body, and every number in the body comes from the payload) runs in CI for each page kind.
Be honest about the exit: honeydolist's own design notes say server HTML plus client components *is* SSR, and that
"if server-rendered body matters later, the honest fix is Nuxt, not a hand-rolled second renderer". Set **triggers**
that move you: more than ~25–30 block/page kinds, repeated drift incidents, or interactive content bots must see. Until
then 12 templates are cheaper than a Node fleet.

### 9.2 Bots: classify, verify, shed, never cloak

| Class | Examples | Reads | Policy |
|---|---|---|---|
| Search crawlers | Googlebot, bingbot | head + body (Google also renders JS) | verified → good rate, below humans in priority |
| Link-preview scrapers | Facebook, Slack, WhatsApp, X | `<head>` only; cache for days | always get the full head; absolute 1200×630 JPEG/PNG `og:image` |
| AI/answer fetchers | `OAI-SearchBot`, `ClaudeBot`, `PerplexityBot`, … | body, **no script** | allow by robots choice; same HTML |
| Unknown / spoofed | anything claiming to be Googlebot | everything | unverified claim = ordinary client limits |

A User-Agent is a *claim*. Verify the big crawlers with **reverse DNS then forward-confirm** (or the vendor's published
IP ranges), cache the verdict per IP for ~24 h, and time the DNS lookup out at ~200 ms (timeout = unverified). A CDN's
"verified bot" flag does the same by network address; honeydolist noticed a UA-string test from an ordinary address
proves nothing about whether the *real* bot gets in.

```go
func verifiedBot(ctx context.Context, ip netip.Addr, suffixes []string) bool {
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String()) // reverse: PTR
	if err != nil {
		return false
	}
	for _, name := range names {
		name = strings.TrimSuffix(name, ".")
		if !hasSuffixAny(name, suffixes) { // e.g. ".googlebot.com", ".google.com"
			continue
		}
		addrs, err := net.DefaultResolver.LookupHost(ctx, name) // forward-confirm
		for _, a := range addrs {
			if p, perr := netip.ParseAddr(a); err == nil && perr == nil && p.Unmap() == ip.Unmap() {
				return true
			}
		}
	}
	return false
}
```

| Spike strategy | Pros | Cons |
|---|---|---|
| Treat everyone equally | simple | a crawl spike degrades humans |
| `Crawl-delay` in robots.txt | free | Google ignores it |
| **Separate lane + shed with `503 + Retry-After`** | humans protected; major crawlers back off on 429/503 | bots may crawl slower for a while |
| Block bots | zero load | disappears from search |

**Choice: two concurrency pools** (a semaphore each; the bot pool is smaller and sheds first with `503` and
`Retry-After`) plus the snapshot tier, which makes most bot hits cheap. **The lane changes priority, never content.**

### 9.3 Caching rendered HTML

| Option | Pros | Cons |
|---|---|---|
| CDN only | zero origin code | the long tail is never warm: 20k/s of cold URLs all reach origin |
| In-process map only (honeydolist) | trivial; microseconds | 500 GB doesn't fit; every node warms separately; no cross-node purge |
| **CDN → L1 LRU → L2 snapshots → render** | spike absorbed by snapshots; hot set in RAM | three layers to keep coherent |
| Pre-render and store *everything* | origin never renders | big backfill after each template change |

**Choice: three tiers.** The **snapshotter** renders on publish (event from the outbox, [10](10-webhook-delivery.md)),
writes `html.gz` to object storage under one of 16 key prefixes, and purges the CDN by tag ([32](32-block-cms.md)). L1 is a
bounded LRU of the hot ~200k pages (5 min TTL). A request miss falls to L2 (`GET` ~10–30 ms), and only an L2 miss
renders. Rules that decide whether it works:

- **Key hygiene.** The key is the normalised path plus an **allow-listed variant** (honeydolist keys a product by
  `?offer=` only, with a length cap and forbidden characters); every other query parameter is dropped, or `?utm=x`
  turns 20M pages into 20M × ∞ cache entries.
- **Never cache failures.** `503` is `no-store`. A `404` gets a short TTL (30 s: it may be published in a moment) and,
  at scale, **one shared 404 body** in memory rather than a cached entry per junk path. Honeydolist's own `Stats()`
  comment notes that crawler 404s "pile up between saves" in an unbounded map; cap it.
- **Validators.** `ETag` = hash of the bytes sent; `Last-Modified` = when those bytes were *first made* and kept across
  re-renders producing identical bytes, so a crawler's revisit is a `304` even after a cache drop (honeydolist does
  exactly this). A `304` saves bandwidth but still spends crawl budget; the real fix for budget is fewer, better URLs.
- **Stampede control.** `singleflight` per key at L3 ([38](../../38-caching-patterns.md)); serve stale while one request
  refreshes; `stale-if-error` so a dead database means stale pages, not an outage.
- **Template version bump:** don't re-render 20M at once (16.7 CPU-hours). Serve old snapshots (stale-while-revalidate)
  and backfill hot-first at a capped rate using the `page_snapshots_stale_idx` order.

### 9.4 Real status codes from an SPA shell

A client-side router cannot change the HTTP status, so a naive SPA answers `200` for every path and shows "not found"
in the browser: a **soft 404**. Crawlers waste budget on infinite bogus URLs and dilute the real ones. The resolver
decides the status first, in a fixed order (honeydolist's):

| Situation | Status | Note |
|---|---|---|
| Live page | 200 | self-referencing canonical |
| Draft, unpublished, never written | 404 | also absent from API, sitemap, `llms.txt` |
| Deleted on purpose | 410, or 301 to its list | via `tombstones`; honeydolist sends removed/archived pages to their list with a 301 |
| Moved (slug change) | 301/308 | looked up **last**: a live page always wins its address ([35](35-redirect-manager.md)) |
| Render or database error | **503** + `Retry-After`, `no-store` | never 404 (would teach crawlers the page is gone) and never 200 with a default card |
| Staging / non-public host | `X-Robots-Tag: noindex` | and `Disallow: /` ([33](33-sitemaps-crawl-notify.md)) |

Two pitfalls: the route table in the backend is now authoritative for "what exists" and must never drift from the
Vue router (one shared manifest, plus a test); and **paging**: crawlers must be able to *follow* `<a href="?page=n">`
links, while the head's canonical stays on the list's own address (honeydolist: the shell ignores `?page=n`, so every
page n has page 1's head and the crawler body shows page 1).

### 9.5 One model, three projections

| Option | Pros | Cons |
|---|---|---|
| Hand-write HTML, JSON-LD and Markdown separately | simplest per output | three sources of truth; they disagree within a month |
| JSON-LD injected by client script | one place | AI fetchers don't run script; Google may read it, others won't |
| **One `Page` model → `RenderBody`, `JSONLD`, `Markdown`** | structured data and text can't disagree; each output testable | a model rich enough for all three |

**Choice: one model.** The use cases build a `Page` (the same ones the JSON API serves); three pure functions project
it: an `html/template` body, a single `@graph` of JSON-LD built with `json.Marshal` (a name containing `</script>` stays
inside its tag; `template.JS` stops the engine mangling it), and a Markdown renderer for `/page.md` and
`llms-full.txt`. Rules inherited from honeydolist: the markup tells the truth (an `Offer`'s price equals the price
shown; no rate means no offer); **omit empty values** rather than invent them (no hours until typed); never self-write
`AggregateRating`/`Review`; list photos as `ImageObject` nodes and, in the body, as `<img>` inside `<noscript>` (JS
clients don't download them twice); test hostile names in every field. Link-preview scrapers cache hard and you can't
purge their caches, so get `og:image` right the first time: absolute HTTPS, no redirects, 1200×630, small JPEG/PNG.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (200k req/s crawl spike): S3-style prefix limits (≥ 40 prefixes), then egress and the CDN
  bill; shedding bots earlier. The database stays safe if snapshots exist for the tail.
- **Cold start / deploy:** empty L1 plus a stampede. Warm the top-N list from snapshots before taking traffic; roll
  nodes gradually.
- **Old shell, deleted bundle:** a cached page references `app.abc123.js` that a deploy removed; Googlebot indexes it
  blank. Hashed immutable assets, and keep the last several deploys' files ([62](../../62-deployment-operations.md)).
- **Snapshot store down:** humans fall through to L3; bots get `503`. **Database down:** serve stale (`stale-if-error`),
  `503` only on true misses.
- **Wrong status cached:** a 404 cached for a page published a second later: 30 s TTL plus a publish purge bound it.
- **Bot verification slow:** DNS timeout ⇒ treat as unverified; never block a request on it for long.
- **Hostile content in a snapshot:** snapshots are rendered by the escaping template; re-render on any escaping fix
  (bump `tmpl_version`).
- **Single points of failure:** the snapshotter (events queue), the CDN purge path (TTL is the backstop).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Head + crawler body from Go templates | Nuxt/Next SSR | no Node fleet, cheap renders, same use cases as the API | a second renderer and a parity test |
| Same bytes for every client | bot-specific pages | no cloaking risk, one cache | nothing |
| Bot lane + `503/Retry-After` shedding | equal treatment | humans protected during spikes | slower crawling during a spike |
| Pre-rendered snapshots on publish | render on demand only | cold crawl spikes never reach the DB | storage (~100 GB gz) and a snapshotter |
| Resolver decides status first | SPA decides in the browser | no soft 404s | the route manifest must match the SPA |
| One `Page` model, three projections | three hand-written outputs | text, JSON-LD and Markdown can't disagree | a richer model |

## 12. Check yourself

1. Which clients read what, and why isn't head-only injection enough in 2026?
2. Why is serving the same HTML to bots and humans important, and what *can* differ?
3. A crawl spike of 20k req/s hits cold URLs. Show why unprotected it breaks, and what protects you.
4. A database error occurs while rendering a page. What status, what caching, and why not 404?
5. What does the number-parity test check, and what risk does it contain?

<details><summary>Answers</summary>

1. Link-preview scrapers read only `<head>`; Google renders script on a deferred pass; AI fetchers and weaker
   renderers, as far as their published descriptions say, run no script and see only the HTML. Head-only gives them a title and one sentence, nothing to quote. A
   plain-HTML body from the same data fixes that.
2. Different content for bots is cloaking and gets penalised. Only *priority and rate limits* differ (a separate lane,
   shedding with `503`); the bytes, the status and the cache entry are the same.
3. 20k × 3 ms is 60 cores and 20k × 3 queries is ~60k qps against a database good for ~10k+ indexed reads/s. With
   pre-rendered snapshots answering ~95%, only ~1k renders/s (≈ 3 cores, 3k qps) remain; a separate bot lane and
   `503 + Retry-After` shed the rest, and CDN + L1 absorb repeats.
4. `503` with `Retry-After`, `Cache-Control: no-store`. A 404 would tell crawlers the page is gone and drop it from
   the index; a 200 with a default card would create duplicate/soft-404 content.
5. Every number in the API payload appears in the HTML body and every number in the body comes from the payload (for
   each page kind), catching drift between the Go template and the Vue component. It contains the second-renderer
   risk; it can't prove the *wording* matches, only the facts.

</details>

## 13. In Go

- Related lessons: [57 Web security](../../57-web-security.md) (`html/template` contexts, `template.JS`, XSS), [64 HTTP internals](../../64-http-protocol-internals.md) (conditional requests, `HEAD`, gzip, `Vary`), [66 Serving Go apps](../../66-serving-go-apps.md) (the shell as a handler, reverse proxy in front), [38 Caching](../../38-caching-patterns.md) (`singleflight`, stale-while-revalidate), [68 Rate limiting](../../68-rate-limiting-throttling.md) and [36 Resilience](../../36-resilience-patterns.md) (lanes, load shedding, `Retry-After`), [20 HTTP server](../../20-http-server.md), [62 Deployment](../../62-deployment-operations.md) (immutable assets).
- Packages: `html/template`, `net/http` (`http.ServeContent` answers `If-None-Match`/`If-Modified-Since` with a `304` when you set `ETag` and the modtime), `net` and `net/netip` (PTR and forward lookups), `golang.org/x/sync/singleflight` and `semaphore`, `github.com/hashicorp/golang-lru/v2`, `compress/gzip`, an S3 client.
- Keep `RenderBody`, `JSONLD` and `Markdown` as pure functions `Page → []byte`; golden-file tests per page kind catch silent changes in a diff.

## 14. Extensions

- **Content negotiation:** `Accept: text/markdown` on the canonical URL, beside `/page.md`.
- **Edge rendering** of the `<head>` only (a worker reading the snapshot index), keeping HTML at the edge.
- **Per-bot analytics:** which bots, which URLs, which statuses; feed it back into sitemap priorities ([33](33-sitemaps-crawl-notify.md)).
- **The migration trigger:** write down the metrics (render CPU, page kinds, drift incidents) at which you move to Nuxt, and what you'd keep (resolvers port as plain functions).
- **Widget-heavy pages:** prerender with a headless browser for the 1% that can't be templated, cached as snapshots.
- **Hreflang and canonical clusters** for 20 languages: the head's size and the cache key both grow.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Shell service (Go: resolver, L1 LRU, snapshot reads, L3 render) | ~29k req/s at the worst moment: ~9k normal (assume the CDN absorbs ~70 % of the 30k/s, §4) + the 20k/s cold crawl spike | CPU | spike: 95 % from snapshots 19k × 0.3 ms (S3 GET over TLS, copy 5 KB) = 5.7 vCPU; 5 % renders 1k × 3 ms (§4) = 3.0; normal: 90 % L1 8.1k × 0.1 ms = 0.81, 10 % L2 0.9k × 0.3 ms = 0.27; total ≈ 9.8 vCPU. N = ceil(9.8 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(2.3) = 3 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 3, one per zone | the spike is TLS and copying, not templates; memory is L1 5 GB (200k pages × 25 KB, §4) + ~2 GiB heap, +50 % GC headroom ≈ 10 GiB of 16, so `c` is enough; network peaks at ~1.2 Gbit/s for all nodes; 41 % busy, 61 % with a zone down; bots get their own pool |
| Postgres read replicas (content for L3 renders, `page_snapshots`, `tombstones`) | ~3k qps (1k renders/s × 3 queries, §4); ~200 GB of source rows (assumption: 20M × ~10 KB) | IOPS, then memory | CPU 3k × ~0.5 ms = 1.5 vCPU, one replica alone 38 %; the spike reads a uniform tail, so with ~25 GiB of cache over 200 GB assume 80 % of reads miss RAM at ~1.5 page reads: 3k × 0.8 × 1.5 = 3.6k IOPS ⇒ `gp3` at 5,000 IOPS (+2,000 × $0.005) | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS read replicas | 2, one per zone | a uniform tail cannot be cached anyway, so buy IOPS (cheap) rather than RAM; `r` keeps the indexes and the hot head resident; the primary is the CMS database (lesson 32), not repeated here |

Snapshot store: S3, nothing to size: 100 GB gz (§4), 16 prefixes (≈ 88k GET/s ceiling against 19k needed). Reach it through an S3 gateway endpoint (free); through NAT the spike's ~20 TB a month of snapshot reads would cost ≈ $900. A full re-render is 20M × 3 ms = 60,000 CPU-s (§4), but the replicas can spare only ~1k renders/s, so it is a capped backfill: 20M ÷ 1k = 20,000 s ≈ 5.6 h on ~3 vCPU of the shell nodes' headroom, not "100 cores for 10 minutes" (that would be 60k qps). S3 request prices are not in the sheet; the ones below are from memory: ≈ $0.0004 per 1,000 GET, ≈ $0.005 per 1,000 PUT.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Shell: 3 × `c7g.2xlarge` (3 × $212) | $640 |
| 2 read replicas: `db.r7g.xlarge` (2 × $310 managed) | $620 |
| gp3: 2 × 300 GB × $0.08 + 2 × 2,000 extra IOPS × $0.005 | $70 |
| S3 storage: ~200 GB (snapshots plus a copy during a backfill) × $0.023 | $10 |
| S3 GETs: ~137M a day in a 2-hour spike + ~16M otherwise = 153M × 30 = 4.6B × $0.0004 per 1,000 | $1,800 |
| ALB: $16 + ~63 LCU (~3.5k req/s average × 5 KB ≈ 63 GB/h) × $0.008 × 730 | $380 |
| CDN egress: ~78 TB normal + ~22 TB in spikes = ~99 TB × $0.085 (first tier) | $8,400 |
| One full re-render a month: 20M PUTs × $0.005 per 1,000 (Spot compute is pennies) | $100 |
| **Total** | **≈ $12,000** (≈ $11,600 with a 1-year plan on the $1,260 compute baseline) |

- **Biggest line and why:** CDN egress, 70 %. The servers and databases together are about 10 %. The page is already 5 KB gzipped, so the levers are not size but count: answer revisits with `304` (~0.3 KB instead of 5 KB, about 15× fewer bytes for unchanged pages), shed bots first, and negotiate volume pricing. The S3 GET fee (15 %) shows that snapshots trade database load for request fees: a daily 2-hour spike is a $1.8k line, which a longer `s-maxage` on tail pages (more CDN hits) reduces.
- **What changes at 10×:** a 200k req/s spike ⇒ CPU 98 vCPU ⇒ N = ceil(98 ÷ 4.27) = 23 → 24 × `c7g.2xlarge` (≈ $5,100); S3 prefixes ≥ 40 (§10). Renders would be 10k/s = 30k qps, beyond two replicas, so the answer is more snapshot coverage (99.5 % brings renders back to 1k/s), not more replicas. Egress ≈ $85k and S3 GETs ≈ $18k a month: shedding bots earlier is the cheapest hardware decision in the design.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Three kinds of data. The **content** an L3 render reads: 20M pages × ~10 KB ≈ 200 GB of source rows (section 15), fetched by primary key, ~3k queries/s from ~1k renders/s × 3 queries. The **snapshot bodies**: 20M × ~5 KB gzipped = 100 GB, read by key, up to ~19k/s in a crawl spike. And three small control tables: `page_snapshots` (a 20M-row index of the bodies), `tombstones` (301/410 decisions) and `bot_ip_verdicts` (a 24 h cache).
- A crawl spike reads a **uniform cold tail**, which nothing caches, so the database is bound by IOPS (~3.6k, section 15), not CPU (1.5 vCPU of 4); the design's answer is that snapshots keep ~95 % of spike traffic away from it.
- Staleness is acceptable (5-minute TTL, `stale-if-error`); a wrong status is not, so the status decision reads authoritative rows (`tombstones`, the slug tables) before any HTML.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen (the CMS database and its read replicas) | the source rows already live there (lesson 32); two `db.r7g.xlarge` replicas carry the ~3k qps of by-key reads; `page_snapshots_stale_idx (tmpl_version, rendered_at)` orders the backfill; `CHECK ((status = 410) = (location IS NULL))` keeps tombstones valid; `INET` is a native type for `bot_ip_verdicts` |
| MySQL (RDS / Aurora) | ✔ works, interchangeable for these tables | InnoDB's clustered primary key returns a row by `page_id` in one tree walk, a slight edge for uniform-tail point reads (section 15 assumes ~1.5 page reads per query on a Postgres heap plus index); `INET` becomes `VARBINARY(16)` with `INET6_ATON`; `CHECK` is enforced from 8.0.16. The catch is that the source rows are lesson 32's, which use partial indexes and `LISTEN/NOTIFY`: choosing MySQL here means taking that lesson's workarounds too, or running two engines |
| S3 | ✅ chosen for snapshot bodies (derived) | a body is an immutable object fetched by key: 100 GB gz, 16 key prefixes ≈ 88k GET/s of ceiling against 19k needed (section 15); keeping 100 GB of blobs in the replicas would compete with the hot index pages for RAM |
| DynamoDB | ✔ works for the bodies and the small tables, no gain | a 5 KB body is far below the 400 KB limit and every read is by key, so it could replace both S3 and `page_snapshots` (~19k reads/s is spread over several partitions at ~3,000 reads/s each). It would trade S3's GET fees for read fees and add a second copy of the keying scheme; it is not an alternative for the content, which the CMS database owns |
| Redis (ElastiCache) | ✔ for `bot_ip_verdicts` and as an L2 if wanted, ✖ as the record | native 24 h TTL fits the verdicts (the table notes "or Redis"); the hottest ~5 GB already sits in each node's L1 LRU, so a shared L2 adds a hop for little |

**Pick:** the CMS PostgreSQL on RDS with two read replicas (`db.r7g.xlarge`, `gp3` at 5,000 IOPS, section 15) for content, `page_snapshots` and `tombstones`, plus S3 for snapshot bodies. The deciding properties are that by-key reads scale out on replicas and the cost of a uniform tail is bought as IOPS rather than RAM. System of record: the content rows and `tombstones` (authored decisions). Derived and rebuildable: `page_snapshots` and the S3 bodies (a re-render backfill, capped at ~1k renders/s), the CDN and L1 copies, and `bot_ip_verdicts`.

**If it were MySQL instead:** nothing material in this lesson's tables: `BYTEA` becomes `VARBINARY`, `INET` becomes `VARBINARY(16)`, the clustered key trims a page read per point lookup, and replicas work the same way. The real cost is inherited from lesson 32's data model (partial indexes, `LISTEN/NOTIFY`), not from anything added here.

**What would flip the choice:** the load, not the engine, moves first. At 10× (a 200k req/s spike) renders would be 10k/s ≈ 30k qps, beyond two replicas, and the answer is more snapshot coverage (99.5 % brings renders back to ~1k/s), not more replicas or another database (section 15). The S3 prefix ceiling arrives at ~40 prefixes (section 10). DynamoDB for the bodies and indexes becomes attractive only if the team wants no replicas or S3 prefixes to operate and the per-read bill is acceptable.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 3M HTML req/s peak (2.1M human + 0.9M bot), cold crawl spike +2M/s; 2B pages ≈ 10 TB of gzipped snapshots and 20 TB of source rows; ~980 vCPU of shell; ~10 PB a month through the CDN (§4 never says what the CDN absorbs; §15 assumed 70 %) | snapshot reads: the spike's ~1.9M GET/s ÷ 5,500 per prefix = 346 prefixes, 22× today's 16 (88k/s). Next, renders: 5 % of 2M = 100k/s ≈ 300k qps, 100× what two replicas carry | same design out: ~512 key prefixes (hash of `page_id`); snapshot coverage to 99.5 % so renders fall to 10k/s ≈ 30k qps ≈ 9 `db.r7g.xlarge` replicas (as §15, §16); ~231 `c7g.2xlarge` shells over 3 zones; a hard bot budget per class (`503 + Retry-After` above it) so capacity follows humans, not the worst crawl; CDN volume pricing. Give up: slower crawling during spikes, ≈ $180k a month of S3 request fees | ≈ $1.2M/month: CDN egress $840k (70 %), S3 GETs $180k, the rest ≈ $180k |
| **1,000×** | 30M HTML req/s peak, spike +20M/s cold (a crawl no search engine sends to one host: assumption); 20B pages ≈ 100 TB of snapshots; ~1.2 Tbit/s at the edge peak; ~99 PB a month ≈ 300 Gbit/s sustained | money, twice: CDN egress ≈ $8.4M a month and S3 GET fees ≈ $1.8M (4.6T requests, priced per request, not per byte), 85 % of ≈ $12M. And a uniform cold tail nothing caches: ~19M GET/s ≈ 3,500 prefixes | change the shape: ~20 regional cells, each owning a slice of pages (by tenant or `page_id` range) with its own shell fleet, replicas and snapshot tier behind CDN routing; the cold-tail snapshot tier moves from S3 to NVMe nodes (`i4i`: 100 TB gz ≈ 110 nodes per copy, ≈ $55k a month for two copies, more nodes for the read rate, against $1.8M of GET fees); two CDNs on committed contracts. Give up: S3's simplicity (a fleet to run and replicate), one global snapshot namespace | ≈ $12M/month linear; CDN $8.4M is the real line, the $1.8M of GET fees becomes ≈ $0.06M of NVMe |
| **10,000×** | 300M HTML req/s peak (210M human + 90M bot: one request per internet user every ~17 s, all at once, 100× a hyperscale peak), spike +200M/s; 200B pages; ~12 Tbit/s at the edge, ~990 PB a month | the bill and the world: egress ≈ $84M a month at first-tier price (70 % of ≈ $120M); 90M bot requests a second is not a crawl, it is a cost | change the problem: ration and shrink. Bots get a fixed share by class (unverified claims get ordinary client limits) and are steered to cheap bytes: sitemaps with truthful `lastmod`, `304`s (0.3 KB instead of 5 KB), the Markdown copy; tail pages untouched for a year get a 24 h `s-maxage` (the publish purge still works); only sitemap pages are crawlable (robots and `noindex` for the rest); humans' repeat views come from the browser (immutable bundles, `ETag`). Give up: crawl freshness for the tail, long-tail pages out of search, a flat bot policy | ≈ $120M/month if nothing changes: the bill is the break; `304`s alone are ~15× fewer bytes on revisits (§15) |

**What stays the same at every scale:** the resolver decides the status before any HTML is built (no soft 404s); every client gets the same bytes, and the bot lane changes priority, never content; and snapshots are rendered on publish and purged by tag, so a cold request is a read, not a render.

**Say it in an interview:** "At 100× the first break is snapshot reads: a 2M/s cold spike needs ~350 object-store prefixes, so I use 512, push snapshot coverage to 99.5 % so renders stay near 10k/s, and cap the bot lane. At 1,000× the CDN is $8.4M and S3 request fees $1.8M a month, so I cut regional cells, move the cold tail to an NVMe tier and run two CDNs under volume contracts. At 10,000× 90M crawler requests a second is a cost, not a crawl: bots get a fixed share and cheap bytes (`304`, Markdown), and tail pages get long TTLs."
