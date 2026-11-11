# 32 · Block-Based CMS

`🟢 easy` · *Topics:* a page as an ordered list of typed JSON blocks, a block registry with versioned schemas, unknown-block handling, sanitised rich text, draft → scheduled → published as a pointer flip, revisions, CDN purge by tag, stale-while-revalidate, category trees · *Builds on:* [05 Cache with TTL](05-kv-cache-ttl.md) (what a page cache is), [10 Webhook Delivery](10-webhook-delivery.md) (outbox for the purge), [15 Job Scheduler](15-job-scheduler.md) (the scheduled flip)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [31 Service Area & Geocoding](31-service-area-geocoding.md) · Next: [33 Sitemaps & Crawl Notifications](33-sitemaps-crawl-notify.md)

---

> 🌱 **Seed (honeydolist-site):** ADR 0004 chose a hybrid model: typed tables for things with business meaning, and
> ordered `{type, props}` JSON blocks for marketing copy, validated by a Go registry ("one Go file plus one Vue
> component per block type"). ADR 0005 then *deferred* the blocks (`page_sections` was never built); what exists is a
> rich-text allowlist (Step 81: h2–h4, lists, links, tables, own-site pictures; script, style, iframes, handlers and
> `javascript:` dropped), `content_revisions` (migration 0013: a full snapshot per edit, no FK), one engine for Articles
> and News with a two-level category tree enforced by a trigger, and scheduled publishing (Step 76, migration 0099: a
> scheduled piece is a *draft* with `publish_at`, flipped live by a worker). Pages are cached in memory for 5 min and
> purged by every save. It serves tens of pages to one editor.
> **Here, at scale:** a multi-tenant site builder: 10M pages, 50k editors, 100k page views/s at peak, nearly all from a CDN.

## 1. The task

> "Design the content system behind a website builder. Editors build pages from blocks — a hero, an FAQ, a text section,
> a gallery — save drafts, schedule publication, and roll back mistakes. Visitors must see the live page fast and never
> see a half-saved one."

Reads are trivial once cached; the interesting part is what a page *is* (data, not HTML), what "live" means, and how a
publish reaches 300 CDN edges without anyone serving stale or broken content.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Is a page HTML, or data? | Decides security, rendering and editing. | Data: ordered typed blocks; renderers turn them into HTML. |
| Can editors paste raw HTML or script? | Stored XSS lives here. | No script ever. Rich text only through an allowlist. |
| Nested blocks (a column holding blocks)? | Doubles validation and rendering cost. | No, flat list; "section" is a block type. |
| Do edits go live on save? | Changes the whole publishing design. | No: save creates a draft; **publish** is a separate act. |
| Concurrent editors on one page? | Lost updates. | Detect and refuse (optimistic version), no merging. |
| Multi-tenant? | Keys, purges, noisy neighbours. | Yes, 200k tenants; every key and query carries `tenant_id`. |
| Scale? | Cache and storage design. | 10M pages, 50k editors, 100k views/s peak, 97% served by the CDN. |
| Freshness after publish? | Purge design. | Visible worldwide within ~5 s of publish. |

## 3. Requirements

**Functional**
- Create/edit a page as an ordered list of typed blocks; every block validated against its schema.
- Autosave a working draft; **Save version** makes a revision; **Publish** (now or at a time) makes one revision live.
- Revision history, diff, revert (a revert is a new revision, never a deletion).
- Categories in a tree; a page lives in one category; category pages list their pages.
- Public read by slug, with meta for SEO ([34](34-crawler-ready-spa.md)).

**Non-functional**
- Public read p99 under 50 ms from the edge; a half-written page is never visible.
- A publish is atomic (all blocks or none) and reaches every edge within ~5 s.
- No stored script: a compromised editor account must not become script in every visitor's browser.
- A bad block type or schema change must not 500 the whole page.

**Out of scope**
- Visual editor UX, real-time co-editing (hard track), translations, per-visitor personalisation.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Pages | 10M | given |
| One page document | ~20 KB | ~15 blocks × ~1 KB + one 5 KB rich-text block |
| Live content | ~200 GB | 10M × 20 KB |
| Saves | ~300k/day ≈ 3.5/s, peak ~17/s | 20% of 50k editors = 10k active × 30 saves; ×5 for peak |
| Revision bytes | ~6 GB/day ≈ 2.2 TB/yr raw | 300k × 20 KB × 365; JSON compresses 5–10×, so ~220–440 GB/yr |
| Publishes | ~100k/day ≈ 1.2/s | a third of saves publish |
| Origin requests | ~3k/s peak | 97% CDN hit ratio (an assumption: the long tail of 10M pages is colder than a typical site) |
| DB reads | ~300 renders/s | 90% hit in the origin's in-process cache; ×2 queries ≈ 600 qps on replicas |

What the numbers tell us: **writes are tiny** (17/s peak) and **reads are a CDN problem**; neither needs a clever
database. The hard parts are *correctness* (what is live), *invalidation* (a purge that cannot be lost), the *blast
radius* of shared content (change a header block, 1M pages go stale) and revision growth if autosave is careless.

## 5. Plan

1. Model: page, immutable revisions, pointers for draft/live; blocks as validated JSON (registry, rich text).
2. States, the publish flip, scheduled publishing.
3. Read path: origin render, caches, CDN, tag purge.
4. Category tree, then failure modes.

## 6. High-level design

```
 editor ──► Admin API ──► Postgres ── pages (pointers) ─ page_revisions (immutable JSONB) ─ categories
              │  validate blocks (registry) + sanitise rich text
              │  publish: ONE tx { flip pointer, outbox('page.published', tags) }
              ▼
          outbox relay ──► CDN purge by tag ────────────────┐
              │        └─► broadcast "invalidate" to origins │
          scheduler ──► flips due pages (task 15)            │
                                                              ▼
 visitor ──► CDN (s-maxage, stale-while-revalidate) ──► Render API ──► in-process LRU ──► read replica
                  ▲   `Cache-Tag: page:42 cat:7 tenant:9 block:header-v3`        (blocks JSON → HTML/JSON)
```

Write: autosave into the draft, then publish: one transaction flips the pointer and writes an outbox row; the relay
purges the CDN by tag and tells origins to drop their copies. Read: the CDN answers; on a miss the origin renders the
*published* revision (never the draft) and sets the tags.

## 7. API

```
PUT    /admin/pages/{id}/draft     { "base_version": 12, "blocks": [ … ] }  → 200 { "version": 13 } | 409 stale | 422 { "errors": [{ "path": "blocks[3].props.title", "msg": "required" }] }
POST   /admin/pages/{id}/revisions { "note": "new pricing" }                → 201 { "revision_id": 9021 }   (freeze the draft)
POST   /admin/pages/{id}/publish   { "revision_id": 9021, "at": null }      → 202   (at = RFC 3339 time → recorded as scheduled, flipped later)
POST   /admin/pages/{id}/unpublish                                          → 202
POST   /admin/pages/{id}/revert    { "revision_id": 8800 }                  → 201 new draft revision copied from 8800
GET    /p/{tenant}/{slug}   (public)  → 200 { "meta": {…}, "blocks": [ … ] }  ETag, Cache-Tag, Cache-Control: public, s-maxage=3600, stale-while-revalidate=60
```

A block on the wire: `{ "id": "b_7f3k", "type": "hero", "v": 2, "props": { "title": "…", "image_id": 981 } }`. Blocks
reference files by **id**, never URL, so changing the CDN host never touches content.

## 8. Data model

```sql
CREATE TABLE page_revisions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    page_id     BIGINT NOT NULL,                 -- no FK: history must outlive a hard delete (as honeydolist's content_revisions)
    blocks      JSONB  NOT NULL,                 -- the ordered block list, validated before insert
    content_hash BYTEA NOT NULL,                 -- sha256 of canonical JSON: identical saves reuse the row
    note        TEXT,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX page_revisions_page_idx ON page_revisions (tenant_id, page_id, id DESC);

CREATE TABLE pages (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id             BIGINT NOT NULL,
    slug                  CITEXT NOT NULL,
    category_id           BIGINT,
    state                 TEXT   NOT NULL DEFAULT 'draft'
                          CHECK (state IN ('draft', 'published', 'archived')),   -- is it LIVE?
    draft_revision_id     BIGINT REFERENCES page_revisions(id),
    published_revision_id BIGINT REFERENCES page_revisions(id),
    scheduled_revision_id BIGINT REFERENCES page_revisions(id),   -- pending publication: a NEW page or an update to a live one
    publish_at            TIMESTAMPTZ,
    first_published_at    TIMESTAMPTZ,             -- set once by trigger; after this the slug is indexed (task 35)
    version               INTEGER NOT NULL DEFAULT 0,   -- optimistic concurrency for the draft
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug),
    CHECK ((scheduled_revision_id IS NULL) = (publish_at IS NULL)),       -- scheduled ⇔ has a revision AND a time
    CHECK (state <> 'published' OR published_revision_id IS NOT NULL)
);
CREATE INDEX pages_due_idx ON pages (publish_at) WHERE scheduled_revision_id IS NOT NULL;
CREATE INDEX pages_category_idx ON pages (tenant_id, category_id) WHERE state = 'published';

CREATE TABLE categories (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    parent_id BIGINT REFERENCES categories(id),
    slug      CITEXT NOT NULL,
    path      TEXT   NOT NULL,                   -- materialised: '/guides/roofing/'  (maintained on move)
    depth     SMALLINT NOT NULL CHECK (depth BETWEEN 1 AND 4),
    UNIQUE (tenant_id, path)
);
CREATE INDEX categories_path_idx ON categories (tenant_id, path text_pattern_ops);  -- prefix scans: LIKE '/guides/%'
```

A page is "live" exactly when `state = 'published'`: every read path asks that one predicate and then loads
`published_revision_id`. Drafts and scheduled revisions are never reachable from a public query; there is nothing to
forget to filter.

## 9. Deep dives

### 9.1 Block schema and validation

What does a page look like in storage?

| Option | Pros | Cons |
|---|---|---|
| HTML strings with shortcodes | editors paste anything; one render step | `v-html` makes interactive components inert; the "fix" (a runtime template compiler) is `eval` for database content; stored XSS by design; no CSP without `unsafe-eval` |
| Arbitrary JSON, no schema | zero ceremony | a typo ships to production; renderers defend against everything; no admin form can be generated |
| **Typed JSON blocks + registry** | validated on write; admin form is generated from the schema; renderers are real components; no script anywhere | a small framework to maintain; each new block is a deploy |

**Choice: typed JSON blocks.** Each block type has a **registry entry**: a JSON Schema (the admin renders its edit form
from it) and a decoder. Blocks carry a **schema version** (`v`). Rules that keep the registry from becoming a monster:

- **Reject on write, tolerate on read.** An unknown `type` or a schema failure is a `422` at save time. A *reader* that
  meets an unknown type (a rollback deployed an older registry) **skips that block and logs it, but never fails the
  page**, and the data stays in the revision untouched.
- **Upcast on read, migrate lazily.** When `hero` goes v1 → v2, the code keeps an upcaster `v1→v2`; old revisions are
  never rewritten in place (they are history). A background job re-saves hot pages at the new version.
- **Limits:** ≤ 200 blocks, ≤ 64 KB per block, flat (no nesting). Honeydolist's own ADR warned that "small frameworks
  grow; resist making blocks nest".

```go
type BlockDef struct {
	Latest   int
	Upcast   map[int]func(json.RawMessage) (json.RawMessage, error) // version v → v+1
	Validate func(props json.RawMessage) error                      // checks the LATEST shape
}

func Normalize(reg map[string]BlockDef, b Block, forWrite bool) (Block, error) {
	def, ok := reg[b.Type]
	switch {
	case !ok && forWrite:
		return b, fmt.Errorf("unknown block type %q", b.Type) // never accept new junk
	case !ok:
		return b, ErrSkipBlock // a reader skips it and logs; the page still renders
	}
	for ; b.V < def.Latest; b.V++ {
		fn := def.Upcast[b.V]
		props, err := fn(b.Props)
		if err != nil {
			return b, err
		}
		b.Props = props
	}
	return b, def.Validate(b.Props)
}
```

### 9.2 Rich text without stored XSS

A text block needs bold, links and tables. Where do you make it safe?

| Option | Pros | Cons |
|---|---|---|
| Sanitise on **write** only | cheap reads | a policy change (new allowed tag, found bypass) leaves old rows unsafe |
| Sanitise on **read** only | always current policy | CPU on every cache miss; stored data stays dirty |
| **Both**, store the editor's source | defence in depth; re-sanitise on policy change | sanitiser runs twice on a miss (microseconds) |

**Choice: allowlist on write, re-check on render, store the source.** The allowlist (honeydolist's: h2–h4, p, lists,
bold/italic, quotes, links, tables with `colspan`/`rowspan`/`scope`, images only from the tenant's own media path) drops
`<script>`, `<style>`, `<iframe>`, `on*` handlers and `javascript:` URLs while keeping their *text*. Add `rel="noopener
nofollow ugc"` to links and force `https:`. Stamp each stored block with `policy_v`; when the policy bumps, a job
re-sanitises rows below the new version. Because **no executable content lives in the database**, a strict CSP
(no `unsafe-inline`, no `unsafe-eval`) is possible, which is the real payoff: a compromised editor account can deface
a page but cannot run script in visitors' sessions. A `raw_html` block, if sold as a feature, goes through the same
allowlist and is rendered inert. Lesson [57](../../57-web-security.md) covers `html/template` contexts.

### 9.3 States and publishing as a pointer flip

| Option | Pros | Cons |
|---|---|---|
| One row, `is_published` flag, edits live on save (honeydolist today) | simplest; one row | no draft of a live page; every typo is public; revert needs a restore |
| Two rows (live copy + draft copy) | clear separation | publish copies data; drift between copies; two ids |
| **Immutable revisions + pointers** | publish is one tiny UPDATE; instant revert (move the pointer); history for free | one extra join; revision rows accumulate |

**Choice: revisions and pointers.** A revision is **immutable**. The editor autosaves into the *draft* revision
(overwriting it while it is a draft; this is why autosave doesn't create 2.2 TB/yr), and **Save version** freezes it
(new row, deduplicated by `content_hash`). **Publish** is one transaction:

```sql
BEGIN;
UPDATE pages SET published_revision_id = $rev, state = 'published',
                 scheduled_revision_id = NULL, publish_at = NULL, version = version + 1,
                 first_published_at = COALESCE(first_published_at, now())
 WHERE id = $page AND tenant_id = $t AND version = $v;        -- 0 rows → 409, someone else acted
INSERT INTO outbox (topic, payload) VALUES ('page.published', $json_with_tags);
COMMIT;
```

- **Scheduled** = `scheduled_revision_id` + `publish_at`; the scheduler claims due rows with `FOR UPDATE SKIP LOCKED`
  via `pages_due_idx` ([15](15-job-scheduler.md)) and runs the *same* publish transaction. Scheduling never changes
  `state`, so a *live* page can have a pending update. (Honeydolist's CHECK says a published piece is never also
  scheduled; at scale "new pricing live at 09:00" is the common case.) Readers only look at `published_revision_id`.
- **The purge cannot be lost:** the outbox row commits with the flip ([10](10-webhook-delivery.md)); a relay crash
  means a late purge, never a missing one. **Revert** copies an old revision into a new draft; nothing is deleted.
- **After first publish the slug is fixed** (it's indexed); changing it writes a redirect ([35](35-redirect-manager.md)).
  Two editors: `PUT draft` carries `base_version`; a mismatch is `409`.

### 9.4 Render path and cache invalidation

| Option | Pros | Cons |
|---|---|---|
| TTL only (5 min) | no invalidation code | editors wait minutes; an urgent fix is invisible |
| Purge by **URL** | precise for one page | a shared block or a category page touches thousands of URLs you must enumerate |
| **Purge by tag** (surrogate key) | one call evicts every page carrying `block:header-v3` or `cat:7` | needs a CDN with tag purge; tag hygiene (Fastly calls it `Surrogate-Key`, Cloudflare `Cache-Tag`; availability depends on plan) |

**Choice:** `Cache-Control: public, s-maxage=3600, stale-while-revalidate=60` on HTML **plus tag purge on publish**, so
TTL is the safety net and the purge is the mechanism. Tags per response: `page:{id}`, `cat:{id}`, `tenant:{id}` and one
per *shared* block used (`block:{id}`). Origins keep an in-process LRU (as honeydolist's page map, 5 min TTL, 404s
30 s) and are told to drop entries by a broadcast (Postgres `LISTEN/NOTIFY` for a few nodes, Redis pub/sub or a stream
for hundreds). Details that decide whether it works:

- **Blast radius:** editing a shared header purges 1M pages; the next minute every one misses at once. Use **soft
  purge** (mark stale, serve stale while one request revalidates), request coalescing (`singleflight`, [38](../../38-caching-patterns.md))
  and a purge-rate cap, or you will DDoS your own origin (3k/s origin budget, §4).
- `stale-if-error`: if the origin or database is down, the CDN keeps serving the last good page. Never cache failures:
  a `5xx` is `no-store` (honeydolist answers `503` + `Retry-After`, never a cacheable 404). ETag on the rendered
  bytes makes revalidation a cheap `304`.

### 9.5 Category trees

| Option | Pros | Cons |
|---|---|---|
| **Adjacency list** (`parent_id`) | trivial writes and moves | subtree and breadcrumb need recursive queries |
| **Materialised path** (`'/a/b/'`) | subtree = prefix scan; breadcrumb = split | a move rewrites every descendant's path |
| Closure table | any ancestor/descendant query is a join | O(depth × subtree) rows to maintain |

**Choice: adjacency + materialised path, max depth 4.** Per-tenant trees are small (hundreds of nodes) and moves are
rare, so rewriting a subtree's `path` (a few hundred rows, one transaction) is cheap, while the hot reads ("pages in
this category and below", breadcrumbs) are a prefix scan or a split. Honeydolist enforces *exactly two levels* with a
trigger; here a depth `CHECK` plus a cycle trigger. A move purges `cat:{id}` and the descendants' tags, and writes
redirects for URLs that were ever live. Lesson [42](../../42-trees.md) has the tree vocabulary.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the *purge pipeline*, not the database. 1M-page purges, CDN purge-API rate limits (check
  your vendor's), and the revalidation stampede afterwards. Second: revision growth if autosave ever creates rows.
- **Single points of failure:** the outbox relay (run two, `SKIP LOCKED`); the purge broadcast (TTL is the backstop,
  so the worst case is "stale for up to s-maxage", not "stale forever").
- **Postgres down:** editing stops; visitors keep getting pages (CDN `stale-if-error`, origin LRU).
- **Bad deploy removes a block type:** readers skip those blocks and alert; no page 500s; roll back and they return.
- **Scheduled flood:** 200k pages set for 09:00 sharp: jitter the flips within a minute and rate-limit the purge. A
  noisy tenant gets per-tenant limits on saves, publishes and purges ([03](03-rate-limiter.md)).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Typed JSON blocks | HTML strings | safety, a CSP, generated admin forms, real components | pasting arbitrary markup; each block type is a deploy |
| Reject on write, skip on read | strict everywhere | one bad block never 500s a page | a silently skipped block needs monitoring |
| Revisions + pointers | edit-in-place flag | draft of a live page, instant revert, history | one more join, revision storage |
| Purge by tag + TTL safety net | TTL only, or URL purge | seconds-fresh and survivable if a purge is lost | tag discipline, a CDN with tag support |
| Adjacency + materialised path | closure table | cheap reads and simple writes | rewriting paths on a subtree move |

## 12. Check yourself

1. Why store a page as JSON blocks and not as HTML, and what does a strict CSP gain you?
2. A reader meets a block type its code doesn't know. What does it do, and why not fail the page?
3. Walk through publish: which statements are in the transaction, and why is the purge event in it?
4. An editor changes a shared header block used by 1M pages. What goes wrong and what do you do?
5. Why materialised path for categories rather than a closure table?

<details><summary>Answers</summary>

1. HTML strings are inert under `v-html` (components stop working) and the usual workaround, a runtime template
   compiler, executes database content as script. Typed blocks are validated data rendered by real components, so no
   executable content is stored, and a CSP without `unsafe-inline`/`unsafe-eval` becomes possible: a compromised
   editor account cannot become script in visitors' browsers.
2. It skips that block, logs it, and renders the rest; the data stays in the revision. Failing the page would let one
   rollback or bad deploy take down 10M pages; skipping loses only that block until the registry is restored.
3. `UPDATE pages SET published_revision_id … WHERE id AND version` (zero rows → `409`) and `INSERT INTO outbox`
   `page.published` with its tags, then commit. The event shares the transaction so the purge can't be lost: a relay
   crash makes it late, never missing. Otherwise a crash between flip and purge leaves a stale CDN.
4. A tag purge evicts 1M pages and the next requests all miss together, hitting the 3k/s origin budget. Use soft purge
   (stale-while-revalidate), `singleflight`, a cap on purge rate, and serve stale while revalidating.
5. Trees are small and moves are rare, so rewriting a subtree's paths is cheap, while reads (subtree, breadcrumb) are
   a prefix scan or a split. A closure table maintains O(depth × subtree) rows to answer queries we can already answer.

</details>

## 13. In Go

- Related lessons: [57 Web security](../../57-web-security.md) (`html/template`, allowlists, CSP), [38 Caching](../../38-caching-patterns.md) (cache-aside, `singleflight`, stampede), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED`, optimistic versions, partial indexes), [34 Event-driven & outbox](../../34-event-driven-outbox.md), [44 Background jobs](../../44-background-jobs-queues.md), [42 Trees](../../42-trees.md), [21 REST API](../../21-rest-api.md) (`422` with field paths, ETag).
- Packages: `encoding/json`, `github.com/santhosh-tekuri/jsonschema` or `go-playground/validator`, `github.com/microcosm-cc/bluemonday` (HTML allowlist), `golang.org/x/sync/singleflight`, `github.com/jackc/pgx/v5`, `crypto/sha256` for `content_hash` (hash a canonical encoding, not map-ordered JSON).
- Decode block props with `json.Decoder.DisallowUnknownFields()` per type, so a typo is a validation error, not silently ignored data.

## 14. Extensions

- **Nested blocks / layouts:** what does validation, diffing and rendering cost, and where do you draw the limit?
- **Per-locale and A/B variants:** a revision per locale; variant assignment at the edge ([09](09-feature-flags.md)); signed draft-preview URLs that are never cached.
- **Bulk import/export** of whole sites (honeydolist exports XML for every content kind): see [48 Bulk Import](48-bulk-import.md).
- **Search over blocks:** flatten text on publish into an index ([39 Global Search](39-global-search.md)).
- **AI drafts:** generated block text lands as a draft with an "unreviewed" flag, never auto-published ([38 AI Draft Queue](38-ai-draft-queue.md)).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Origin API (Go, stateless: render, admin, outbox relay, scheduler) | ~3k origin req/s (97 % CDN hit): ~300 renders/s + ~2,700 in-process hits/s; ~17 saves/s | memory (the in-process LRU), then CPU | hits 2,700 × 0.2 ms = 0.54 vCPU; renders 300 × 3 ms (JSONB decode, upcast, sanitise, encode) = 0.9; saves 17 × 10 ms (schema check, allowlist, sha256) = 0.17; relay + scheduler ≈ 0.1; total ≈ 1.7 vCPU. N = ceil(1.7 ÷ (0.8 × 2 × ⅔ = 1.07)) = ceil(1.6) = 2 → 3. Memory: LRU 2 GiB (~100k hot pages × 20 KB) + ~1 GiB working + 50 % GC headroom ≈ 4.5 GiB | `m7g.large` (2 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | 28 % busy, 43 % with a zone down, so CPU does not pick the family, the LRU does: `c7g.large` has 4 GiB and cannot hold it, `m` gives 4 GiB per vCPU |
| Postgres primary (pages, page_revisions, categories, outbox) | ~17 saves/s + 1.2 publishes/s; 200 GB live + up to 440 GB/yr of history | memory, then storage size (IOPS are tiny) | writes ≈ 18 × ~6 IOPS (heap + TOAST + index + WAL, assumption) ≈ 110 IOPS < gp3's 3,000 included; size ≈ (200 + 440 GB) × 1.15 indexes ≈ 740 GB ⇒ 1 TB `gp3`; hot on the primary: ~10k active drafts × 20 KB + indexes ≈ 3 GB | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ | 1 + synchronous standby | no `i` family or `io2`: the write rate is trivial and disk size, not IOPS, is what grows; `r` because if the replica dies the primary takes its 600 qps and must keep the ~25 GB hot set cached |
| Postgres read replica (published revisions for renders) | ~600 qps (300 renders/s × 2 queries) | memory | hot set: the tail that misses the LRU, 300/s × 3,600 s × 20 KB ≈ 21.6 GB an hour (upper bound, no page repeats) + ~3 GB indexes ≈ 25 GB, so RAM ≥ 25 GB with `shared_buffers` 25 % and the OS cache the rest; CPU 600 × ~1 ms = 0.6 vCPU of 8 | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS read replica | 1 (the primary is its fallback; the CDN serves stale if both fail) | 64 GiB keeps the hour's tail resident so misses stay rare; the CPU would fit an `m7g.large`, the RAM would not; a second replica only if renders triple |

The admin API and the outbox relay ride on the same three nodes (they are in the 1.7 vCPU). No Redis: three origin nodes learn about purges with `LISTEN/NOTIFY` (Redis pub/sub only at hundreds of nodes, §9.4). The CDN does the real work: 100k views/s peak × ~15 KB per page response (assumption) = 1.5 GB/s ≈ 12 Gbit/s; average ≈ peak ÷ 5 = 20k/s ⇒ 300 MB/s ≈ 780 TB a month. It has nothing to size, only egress to pay. As far as I know CloudFront invalidates by path, not by tag, so the tag purge of §9.4 means Fastly or Cloudflare in front (or tenant-versioned paths). Images and files are lesson 11.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Origin API: 3 × `m7g.large` (3 × $60) | $180 |
| Postgres primary: `db.r7g.xlarge` ≈ $310 managed, Multi-AZ doubles | $620 |
| Read replica: `db.r7g.2xlarge` ≈ $620 managed | $620 |
| gp3: 3 volumes × 1 TB (primary, standby, replica) × $0.08 | $240 |
| ALB: $16 + ~32 LCU (600 req/s average × 15 KB ≈ 32 GB/h) × $0.008 × 730 | $200 |
| CDN egress: ~780 TB × $0.085 (first-tier price; volume tiers are lower) | $66,300 |
| **Total** | **≈ $68,200** (≈ $67,700 with a 1-year plan on the ≈ $1,420 compute baseline) |

- **Biggest line and why:** CDN egress, 97 %. The whole origin and database is ≈ $1.9k because 97 % of the 100k views/s never reach it. The lever is bytes per page: 15 KB is the design's weight, so trimming block JSON, brotli and lazy-loading rarely used blocks cut the line directly; after that, volume pricing. The CDN also bills per request (rate not in the sheet; ~52B requests a month is the same order as the bytes), so treat $68k as a floor.
- **What changes at 10×:** 1M views/s peak (≈ 120 Gbit/s), origin 30k/s: 5.4 + 9 + 1.7 + 1 = 17 vCPU ⇒ on `m7g.2xlarge` N = ceil(17 ÷ 4.27) = 4 → 6 nodes. Renders 3,000/s ⇒ 6k qps ≈ 6 vCPU ⇒ 3 replicas (or one `db.r7g.4xlarge`, 128 GiB, as the hot set grows). 170 saves/s is still one primary. The first break is not hardware but the 1M-page purge and the revalidation stampede (§10); egress becomes ≈ $660k a month at the first-tier price, so a committed-volume CDN contract is the real project.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- A page is one ~20 KB ordered JSON document (~15 blocks), read and written **whole**; nothing in section 8 or 9 queries inside a block (no GIN index on `blocks`). 10M live pages ≈ 200 GB, plus up to ~440 GB a year of immutable revisions (section 4).
- Around the document sits relational structure: three nullable pointers to revisions, `UNIQUE (tenant_id, slug)`, `CHECK`s that tie `state`, `scheduled_revision_id` and `publish_at` together, a category tree (adjacency plus materialised path, depth ≤ 4), and a publish that flips a pointer **and** writes an outbox row in one transaction (9.3).
- The load is small: ~17 saves/s at peak, ~1.2 publishes/s, ~600 replica queries/s; the CDN answers 97 % of 100k views/s (section 4). Consistency matters on the write path (a stale `version` is a 409), not on reads.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `jsonb` keeps the block list in a column (large values are compressed out of line); the publish is one transaction (a `version`-conditional `UPDATE` plus the outbox insert); partial indexes keep `pages_due_idx` to the few scheduled rows and `pages_category_idx` to published ones; `FOR UPDATE SKIP LOCKED` claims due pages; `LISTEN/NOTIFY` tells the 3 origins to drop their copies; `CITEXT` for slugs |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | a `JSON` column holds the blocks just as well when read whole, and `SKIP LOCKED` exists. Workarounds: no partial indexes (`pages_due_idx` becomes a generated column that is `NULL` unless scheduled, plus a normal index; `pages_category_idx` gains `state`), no `LISTEN/NOTIFY` (a Redis channel or polling a change table for the origins), no `CITEXT` (the default collation is already case-insensitive, so `UNIQUE (tenant_id, slug)` holds) |
| MongoDB / DocumentDB | ✔ works, no gain here | one page = one document is natural, and the schema can churn. But only the blocks are document-shaped: the pointers, the unique slug, the due-page scan, the category prefix scan and the flip-plus-outbox commit are relational, and the cheat sheet's warning applies (joins and cross-document transactions get awkward). Schema change is already handled in Go by versions and upcasters (9.1) |
| DynamoDB | ✖ poor fit | a 20 KB page is far below the 400 KB item limit and reads by key are easy, but admin lists by category, "due pages" and revision history each need a GSI, and 17 saves/s gives no scale reason to give up ad-hoc queries |
| S3 | ✔ for cold revisions only | immutable revisions older than a year as gzip JSON by revision id, with a pointer left in the row; S3 is $0.023 per GB-month against gp3's $0.08. Not needed while history is ~1 TB (section 15) |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.xlarge` primary and one `db.r7g.2xlarge` replica, section 15). The whole write path is one short transaction whose guarantees (`CHECK`s, foreign keys, the unique slug, the version-conditional update, the outbox row in the same commit) live in the database, and `jsonb` makes the document a column instead of a second store. System of record: `pages` and `page_revisions`. Everything else is derived and rebuildable from `published_revision_id`: the origin LRU, the CDN copies and the rendered HTML.

**If it were MySQL instead:** the same schema with `JSON` for `blocks`, `BINARY(32)` for `content_hash`, `AUTO_INCREMENT` keys (a clustered, ever-growing primary key suits an append-only revision table), the generated-column index for due pages, a plain `(tenant_id, path)` index for prefix scans (`LIKE '/guides/%'` needs no `text_pattern_ops` there), and a pub/sub channel or a change-table poll replacing `LISTEN/NOTIFY`. That is one extra moving part (a channel to run) and nothing else material; run the scheduler's `SKIP LOCKED` scan at `READ COMMITTED` to avoid gap-lock waits on the `publish_at` range.

**What would flip the choice:** history reaching a few TB (about 5 years at ~440 GB a year, section 4) moves old revisions to S3 rather than to another engine; 10× saves (~170/s) is still one primary (section 15); MongoDB or DocumentDB becomes reasonable only if blocks nest, editors need field-level updates and queries inside documents, and the schema churns weekly, that is, when whole-document access truly dominates; real-time co-editing is the hard-track problem and would also change the store.
