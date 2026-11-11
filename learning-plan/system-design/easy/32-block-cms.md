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
