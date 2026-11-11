# 35 · Redirect Manager & 404 Tracker

`🟢 easy` · *Topics:* a hot-path rule lookup (Bloom filter, packed table, atomic snapshot swap), exact vs prefix rules, loop and chain handling on write, 301/302/307/308/410 and what browsers and CDNs remember, 404 tracking without a write per miss (Space-Saving), negative caching that scanners can't blow up, suggested redirects with a human in the loop · *Builds on:* [06 View & Like Counter](06-view-like-counter.md) (batching counters, hot rows), [05 Cache with TTL](05-kv-cache-ttl.md) (bounded caches), [03 Rate Limiter](03-rate-limiter.md) (scanner waves), [34 Crawler-Ready SPA Serving](34-crawler-ready-spa.md) (status codes decided before HTML)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [34 Crawler-Ready SPA Serving](34-crawler-ready-spa.md) · Next: [36 LLM Gateway](36-llm-gateway.md)

---

> 🌱 **Seed (honeydolist-site):** migration 0015 `redirects` (`from_path citext UNIQUE`, `to_path`, `status_code` in
> 301/302/308/410, `is_active`, `hits`) lets a missed redirect spotted in Search Console be fixed from the dashboard in
> seconds (`redirects.write`, 0016). The shell asks **last**, only for an address with nothing live behind it, so *a live
> page always wins its address*: exact path, active 301/302/308 rows, the chain followed to its end in one hop; a loop,
> more than five hops, an off-site target (`//host`, backslash), a switched-off row and a `410` row are a plain 404. One
> indexed read per lookup, uncached (the page cache keeps the result: 5 min, 404s 30 s). Writers: a live category's
> address change writes a 301 old → new, repoints rules that led to the old one (no chains) and frees the new address's
> own rule (moving back works); archiving a live page 301s to its list. A daily `redirectcheck` asks the old domain's
> addresses and flags anything but a 301/308. There is **no 404 tracker** (`hits` is never written) and the Cache card
> notes "a crawler's 404s pile up" in the page map. (Migration 0028 `route_misses` is unrelated: a cached "provider
> found no route", a small negative cache.)
> **Here, at scale:** 20M redirect rules, 100k lookups/s, and 5M distinct 404 paths a day from scanners and stale links.

## 1. The task

> "Design the redirect system for a large site: editors and migrations create redirect rules, every request is checked
> against them fast, and we want to know which broken URLs people and crawlers actually hit so we can fix the ones that
> matter."

A rule lookup is a hash probe; the design lives in keeping 20M rules cheap per node, in not creating loops or poisoned
browser caches, and in tracking misses without letting junk traffic become your biggest table.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Where do rules come from? | Volume and trust differ. | Editors, slug changes (system rules), migrations/imports (millions at once). |
| Match types? | Index vs trie vs regex. | Exact and prefix (`/old-blog/*` → `/blog/*`); no regex in v1. |
| Does a rule beat a live page? | Ordering and safety. | No: the live page always wins; creating a rule over a live URL is refused. |
| Same-site targets only? | Open-redirect risk. | Yes. External targets need an explicit allowlist (extension). |
| How fast is a lookup? | Per-request budget. | ≤ 50 µs added to a request; no DB round trip on the common path. |
| Freshness of a new rule? | Reload design. | Live on every node within ~5 s; instant rollback. |
| What is a 404 tracker *for*? | Decides what to count. | Fix broken inbound links that real users/crawlers hit; ignore scanner junk. |
| Scale? | Everything above. | 20M rules, 100k lookups/s, ~300 404s/s avg (26M/day), 5M distinct paths/day. |

## 3. Requirements

**Functional**
- CRUD for rules (exact and prefix), bulk import with a dry run, enable/disable, history.
- On request: apply the matching rule and answer `301/302/307/308` + `Location`, or `410`.
- Reject at write time: loops, chains beyond a cap, off-site targets, rules over live URLs.
- Track 404s: top missed paths, with referrer and bot/human split; suggest a target; one click to accept.

**Non-functional**
- Lookup adds microseconds, no DB on the common path; a node holds the rules in < 1 GB.
- A new/removed rule is live everywhere in seconds; a bad import is rolled back in one action.
- Junk (scanner) traffic cannot grow memory or the database without bound.
- A wrong permanent redirect must be recoverable even though browsers cache 301s.

**Out of scope**
- Regex rewrites, per-country rules, vanity/short links with click analytics ([01](01-url-shortener.md)).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Rules | 20M | given |
| Row size | ~150 B | `from` ~60 B + `to` ~70 B + fields |
| Table | ~3 GB heap + ~1.5 GB unique index | 20M × 150 B; index key ~60 B + overhead |
| As a Go `map[string]rule` | ~5 GB **per node** | ≈ 250 B/entry with string headers and map overhead (assumption) |
| Packed: sorted `(hash64, offset)` + mmapped strings | ~320 MB index + ~2.6 GB strings paged on demand | 16 B × 20M; 130 B × 20M |
| Bloom filter, 1% false positives | ~24 MB | 9.6 bits × 20M = 192 Mbit, 7 hashes |
| Lookups | 100k/s worst case (every request) | given; with "live wins" only misses ask, ~3% ≈ 3k/s (assumption) |
| Bloom false positives | ~1k/s reach the exact table | 1% of ~98k non-redirect lookups |
| 404 volume | ~300/s avg, ~26M/day | 300 × 86,400 = 25.9M; peak scan waves ×5 and more |
| Naive per-miss upsert | 1.5k writes/s peak, ~1 GB/day, ~365 GB/yr | 5M new rows/day × ~200 B |
| Space-Saving, K = 100,000 | ~15 MB | K × ~150 B; guarantees any path above N/K = 26M/100k ≈ 260 hits/day is tracked |

What the numbers tell us: the *rule set* is a memory-layout problem (5 GB map vs a few hundred MB packed); the *lookup
rate* is a "make the negative case almost free" problem (the Bloom probe costs ~100 ns, 100k/s ≈ 1% of a core); the
*404 stream* is a cardinality problem: count approximately and keep only what is actionable.

## 5. Plan

1. Rule model, normalisation, write-time validation (loops, chains, targets, live-URL conflicts).
2. The hot path: Bloom → exact table, immutable snapshot swapped atomically, delta reloads.
3. Status codes and what clients/CDNs cache.
4. 404 tracking and bounded negative caching.
5. Suggestions and the review queue.

## 6. High-level design

```
 request ─► CDN (caches redirect responses; purged by tag on rule change)
              ▼
 app middleware order:  1. live page?  (L1 / snapshot — cheap)         live ⇒ serve it (a live page wins)
                        2. redirect lookup:  Bloom(24 MB) ─no─► skip
                                                         └maybe► overlay(recent changes) ─► base table (packed, mmap)
                        3. tombstone (410)   4. shared 404 body ─► miss recorder (in-memory Space-Saving)
 writers: dashboard │ slug-change hook │ bulk import ─► validate ─► Postgres: redirects + redirect_changes (same tx)
 every node: poll redirect_changes (seq > last) every ~3 s ─► apply to overlay; rebuild base+Bloom every ~10 min
 miss recorder ─ every 60 s: merge top deltas ─► Redis ZSET miss:{day} ─ hourly ─► miss_daily (top 5,000) ─► suggestions
```

A request that matches nothing pays one Bloom probe; a match pays Bloom + one table probe. Misses are counted in memory
and merged in batches, never one write per hit ([06](06-view-like-counter.md)).

## 7. API

```
GET    /admin/redirects?q=&status=&source=&page_token=            → 200 keyset-paged list
POST   /admin/redirects          { "from": "/old", "to": "/new", "status": 301, "match": "exact", "note": "" }
                                 → 201 | 409 { "reason": "loop" | "live_page" | "exists" | "chain_too_long" } | 422 { "reason": "off_site" }
PATCH  /admin/redirects/{id}     { "is_active": false }                                 → 200
DELETE /admin/redirects/{id}                                                            → 204
POST   /admin/redirects:validate { "rows": [ … ] }  → 200 { "ok": n, "errors": [{ "row": 7, "reason": "loop" }] }   (dry run, task 48)
GET    /admin/404s?since=7d&min_hits=20&referrer=true&sort=hits → 200 [{ "path", "hits", "bot_hits", "top_referrers", "suggestion": { "to", "reason", "score" } }]
POST   /admin/404s/accept        { "paths": ["/x", "/y"] }   → 201 (creates rules, source = 'suggested')
public: any path → 301/308 (permanent) | 302/307 (temporary) + Location | 410 | 404
```

## 8. Data model

```sql
CREATE TABLE redirects (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    from_path   TEXT NOT NULL,       -- normalised: lower-case, no query string, no trailing slash, %-decoded once, < 2 KB
    match_kind  TEXT NOT NULL DEFAULT 'exact' CHECK (match_kind IN ('exact', 'prefix')),
    to_path     TEXT,                -- NULL only for 410
    status_code SMALLINT NOT NULL DEFAULT 301 CHECK (status_code IN (301, 302, 307, 308, 410)),
    is_active   BOOLEAN NOT NULL DEFAULT true,
    source      TEXT NOT NULL CHECK (source IN ('manual', 'system', 'import', 'suggested')),
    note        TEXT,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_hit_day DATE,               -- coarse, written by a daily batch; NOT per request
    UNIQUE (tenant_id, match_kind, from_path),
    CHECK ((status_code = 410) = (to_path IS NULL)),
    CHECK (to_path IS NULL OR (to_path LIKE '/%' AND to_path NOT LIKE '//%' AND to_path !~ '[\\\r\n]'))   -- same-site only
);

CREATE TABLE redirect_changes (      -- ordered log: nodes apply deltas instead of reloading 20M rows
    seq         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   BIGINT NOT NULL,
    redirect_id BIGINT NOT NULL,
    op          TEXT NOT NULL CHECK (op IN ('put', 'delete')),
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);                                   -- compacted after 7 days; a node further behind reloads fully

CREATE TABLE miss_daily (
    day            DATE   NOT NULL,
    path           TEXT   NOT NULL,
    hits           BIGINT NOT NULL,
    bot_hits       BIGINT NOT NULL DEFAULT 0,
    top_referrers  TEXT[] NOT NULL DEFAULT '{}',
    status         TEXT   NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'redirected', 'ignored')),
    PRIMARY KEY (day, path)
);
CREATE TABLE url_history (old_path TEXT PRIMARY KEY, page_id BIGINT NOT NULL, changed_at TIMESTAMPTZ NOT NULL DEFAULT now());   -- feeds suggestions
```

A rule and its `redirect_changes` row are written in **one transaction**; nodes can therefore never see a change without
its log entry, and a rolled-back import leaves no trace.

## 9. Deep dives

### 9.1 The hot-path lookup

| Option | Pros | Cons |
|---|---|---|
| A DB query per request | always fresh, no memory | 100k/s on Postgres; 1–3 ms each |
| `map[string]rule` per node | sub-µs probes | ~5 GB × every node; slow, spiky reload; GC scans a huge map |
| **Packed sorted array (hash + offset), strings mmapped** | ~320 MB hot index; the OS pages strings in on demand | a build step; read-only (rebuilt, not mutated) |
| **Bloom filter in front** | 24 MB; ~99% of non-redirects stop here | 1% false positives; can't delete (rebuild instead) |

**Choice: Bloom → overlay → packed base table, all in one immutable snapshot.** `hash64(from_path)` binary-searched in the
sorted index, then the stored path is compared (a hash alone would redirect on collisions). A published snapshot is
never mutated (readers hold no lock): nodes poll `redirect_changes WHERE seq > $last` every few seconds, copy the small
**overlay** map (recent puts/deletes, checked *before* the base; tiny, so copy-on-write is cheap), apply the changes to
the copy and swap in a new snapshot. Every ~10 minutes a background build makes a fresh base + Bloom (20M inserts ≈
seconds) and swaps it in the same way; the old one is dropped by the GC. Budget 2× memory while both exist, and build
streaming, not by loading everything first.

```go
type snapshot struct {
	bloom   *bloom.BloomFilter
	base    *packedTable          // sorted (hash64, offset) over mmapped strings
	overlay map[string]*rule      // changes since the base was built; nil rule = deleted
	seq     int64
}

var live atomic.Pointer[snapshot]

func lookup(path string) (*rule, bool) {
	s := live.Load()
	if r, ok := s.overlay[path]; ok { // newest truth first
		return r, r != nil
	}
	if !s.bloom.TestString(path) { // ~99% of non-redirects end here, ~100 ns
		return nil, false
	}
	return s.base.Get(path) // verifies the full path, not just the hash
}
```

**Ordering.** Honeydolist asks **last**: a live page always wins its address, so a bad rule can't hide content. The live
check is itself a cache hit, so "live first" costs little and only *misses* (stale links, moved pages, scanners) ask:
~3k/s, plus scanner waves the Bloom absorbs. Creating a rule over a live URL is refused (`409 live_page`); publishing a
page at a path that has a rule deactivates the rule and tells the editor.

**Exact vs prefix.** Exact rules live in the table. Prefix rules (`/old-blog/` → `/blog/`) go in a segment **trie** (or
radix tree) of at most a few hundred thousand entries; precedence is exact, then longest prefix. Avoid regex: it can't
be indexed or Bloom-filtered, and a hostile pattern is a ReDoS. Most "regex" needs are prefix plus a captured tail.

### 9.2 Loops, chains, statuses and caching

**On write**, one recursive query walks from the target and answers "would this create a loop, and how long is the
chain?". The `depth < 6` guard makes it terminate even if a loop already exists:

```sql
WITH RECURSIVE walk(path, depth) AS (
  SELECT $to::text, 1
  UNION ALL
  SELECT r.to_path, w.depth + 1
  FROM walk w JOIN redirects r
    ON r.tenant_id = $t AND r.match_kind = 'exact' AND r.is_active AND r.from_path = w.path
  WHERE w.depth < 6
)
SELECT coalesce(bool_or(path = $from), false) AS makes_loop, max(depth) AS chain_len FROM walk;
```

- **Collapse on write** (honeydolist's category move): when a page moves A → B and rules already lead to A, repoint
  them at B in the same transaction, so no chain forms; when the page later moves back to A, **delete A's own rule**
  (else a rule and a live page share an address and loops appear). **Flatten at serve** as a safety net (≤ 5 hops; a
  loop or longer chain answers 404 and raises an alert, never a browser redirect loop).
- **Targets:** a single leading `/`; reject `//host`, schemes, backslashes, control characters (open redirects, [57](../../57-web-security.md)).

| Status | Meaning | Caching reality |
|---|---|---|
| **301** | moved permanently | browsers and CDNs cache it aggressively; engines transfer signals; **you can't recall it from a user's browser** |
| **308** | like 301, **keeps the method** | same; right for `POST` APIs and forms |
| **302 / 307** | temporary (307 keeps the method) | not cached by default; signals stay on the old URL |
| **410** | gone on purpose | crawlers may drop it a little sooner than a 404 |

Consequences: (1) **roll out risky rules as 302**, or a 301 with `Cache-Control: max-age=300`, and promote to a
long-lived 301 once verified. (2) The CDN caches redirects, so a rule change **purges by tag** ([32](32-block-cms.md)).
(3) Honeydolist's table allows `410` but its shell answers a plain 404 for those rows; at scale serve the real `410`
([34](34-crawler-ready-spa.md) tombstones). (4) Fold `http→https`, `www→apex` and trailing-slash into **one** hop.

### 9.3 404 tracking without a write per miss

| Option | Pros | Cons |
|---|---|---|
| `INSERT … ON CONFLICT` per miss | exact | ~1.5k writes/s at peak, ~365 GB/yr of mostly junk; hot rows |
| **Offline over CDN/access logs** | no app code; a daily `GROUP BY` over 26M rows is trivial for a warehouse | not live; needs a log pipeline you may not have |
| **In-process Space-Saving + batch merge** | ~15 MB; near-real-time dashboard; heavy hitters guaranteed | approximate counts for the tail |

**Choice: Space-Saving per node, merged in batches.** Keep K = 100,000 counters; any path with more than N/K hits is
guaranteed to be in the summary (≈ 260 hits/day for one process seeing 26M). Every 60 s each node pushes its top deltas
(`ZINCRBY miss:{day}`, a Redis sorted set capped at ~200k) and an hourly job copies the top ~5,000 with referrers into
`miss_daily`. Postgres sees ~5k rows/hour instead of 5M/day. If you already ship access logs, the log query is the
audit truth and the in-process tracker is the live view. What to count:

- **Normalise first:** lower-case, strip the query string, collapse `//`, cap length (> 512 B is junk and dropped).
- **Classify scanners, don't count them per path:** `.php`, `/wp-admin`, `/.env`, `/.git/`, `../`, NUL bytes feed a single
  aggregate counter per class, and feed the WAF/rate limiter ([17](17-abuse-protection.md)).
- **What matters has a referrer or a verified crawler.** A 404 reached from a real link (an internal page, a partner
  site, a newsletter) or requested by Googlebot is fixable and affects users or indexing; a scanner has neither. Sort
  and filter the dashboard by that.

### 9.4 Negative caching that scanners can't blow up

Honeydolist caches each 404 page for 30 s so an about-to-be-published page isn't pinned as missing, but its page map keeps
those entries until the next invalidation ("a crawler's 404s pile up"). Its geo cache (0028) stores a *miss* as a row for
30 days so a page doesn't re-ask the provider forever; the same idea, bounded by 30 days. At scale the unbounded map
is an attack surface: 5M distinct junk paths × a cached page body is ~125 GB; even bare keys at ~100 B are ~500 MB.

| Option | Pros | Cons |
|---|---|---|
| Cache nothing | no memory growth | every junk request pays the full resolve |
| Cache each 404 body (unbounded) | repeat misses are free | memory = attacker's choice |
| Bounded LRU of miss *keys* | caps memory | unique junk always misses the LRU |
| **One shared 404 body + bounded miss LRU + a Bloom of live paths** | constant memory; unique junk is answered without the database | the live-path Bloom must be rebuilt as pages are created |

**Choice:** serve **one** 404 response (same bytes, one ETag) from memory, keep a bounded LRU of recent miss keys
(~1M entries ≈ 100 MB, 30 s TTL, so a newly published page appears quickly), and keep a Bloom filter of *live* paths
(50M × 9.6 bits ≈ 60 MB at 1%) so a never-seen junk path is rejected without touching the database. A Bloom "no" means
definitely not live, which is the only direction we need.

### 9.5 From top 404s to suggested redirects, with a human in the loop

The tracker produces candidates; **a person approves** (an automatic "send every 404 to the homepage" is a soft 404 and
poisons your own signals). Ranking: hits, then referrer present, then verified-crawler hits. Suggest a target by
evidence, best first:

1. **Slug history:** `url_history` says this path was a page that moved (confidence high).
2. **Similar live URL:** trigram or token similarity against live paths in the same section (`pg_trgm`), above a
   threshold; show the score.
3. Otherwise no suggestion, only the referrer list ("fix the link at the source").

The review UI is bulk: select 50, accept (rules `source = 'suggested'`, 302 until confirmed, then 301) or ignore
(suppress the path for 90 days). Audit who approved, and run the §9.2 loop check before commit.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (200M rules, 1M lookups/s): memory layout (200M × 16 B = 3.2 GB hot index per node, so the
  strings must be paged or the table sharded by path hash and routed), and the full-rebuild time. Move rules to an edge
  KV, or shard across a small rules tier.
- **Bad import** (2M rules with loops): dry-run validation, staged rollout (canary nodes apply first), one-click disable
  of an entire import batch (`source`/batch id), instant rollback by version.
- **Node divergence / reload:** expose the snapshot `seq` in `/healthz` and alert if a node lags > 60 s; 2× memory while
  two snapshots exist, so jitter rebuild times. **Database down:** nodes keep serving the last snapshot; edits pause.
- **Hot rule:** 10k/s to one redirect; counting hits on its row is a hot-row lock. Count in memory/Redis, fold into
  `last_hit_day` daily ([06](06-view-like-counter.md)); honeydolist left `hits` unwritten for the same reason.
- **Rule sprawl:** every slug change adds a system rule; compact chains, expire system rules with no hits for 12+
  months (needs the daily hit fold). **Scanner wave:** 50k/s of random paths stays at microseconds (Bloom + shared 404).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Bloom → overlay → packed base | in-memory map per node | ~100× less memory, near-free negatives | a build step; 1% false positives to verify |
| Live page wins; redirects asked after | redirects first | a bad rule can't hide content | one extra cheap check on misses |
| Collapse chains on write, flatten at serve | trust editors | no chains, no loops reach users | validation code and a recursive query |
| 302 first, promote to 301 | immediate 301 | recoverable mistakes | slower SEO signal transfer |
| Space-Saving + batch merge | row per miss | 15 MB and ~5k rows/hour | approximate tail counts |
| Shared 404 body + bounded miss LRU | cache each 404 | memory independent of attackers | a short delay before a new page replaces a cached miss |

## 12. Check yourself

1. Why does the live page win over a redirect rule, and how do you prevent shadowing?
2. How do 20M rules fit in memory, and what does the Bloom filter buy you?
3. How do you stop A → B → A on write, and what do you do with the chains A → B → C?
4. Why start with 302 (or a short-lived 301) for risky rules?
5. Why not upsert a row per 404, and what do you track instead?

<details><summary>Answers</summary>

1. A bad or stale rule must never hide real content, so the resolver serves a live page first and consults redirects
   only for addresses with nothing live. On write, creating a rule over a live path is refused (`409 live_page`) and
   publishing a page at a path with a rule deactivates the rule.
2. A Go map would be ~5 GB per node. A packed sorted `(hash, offset)` index (~320 MB) over mmapped strings keeps only
   hot data resident, and a ~24 MB Bloom filter (1% false positives) turns ~99% of non-redirect lookups into a ~100 ns
   "no", leaving ~1k/s to reach the table.
3. A recursive query walks from the new target (depth-capped); if it reaches the new rule's `from`, it's a loop → `409`.
   For chains, repoint existing rules to the final target in the same transaction (A → B → C becomes A → C), and flatten
   at serve time as a safety net with a 5-hop cap.
4. Browsers and CDNs cache 301s long-term and you can't recall them from users. A 302, or a 301 with
   `max-age=300`, is correctable; promote to a long-lived 301 once verified.
5. 300/s average (peaks several times that) and 5M new distinct rows a day (~1 GB/day, ~365 GB/yr) are mostly scanner
   junk. Keep an approximate heavy-hitter summary (Space-Saving, K = 100k, ~15 MB) per node, merge top deltas to Redis
   every minute and the top ~5,000 to Postgres hourly; classify scanners into aggregate counters; prioritise misses with a
   referrer or a verified crawler.

</details>

## 13. In Go

- Related lessons: [38 Caching](../../38-caching-patterns.md) (bounded LRU, negative caching), [15 Sync & context](../../15-sync-context.md) (`atomic.Pointer`, polling loops with `context`), [57 Web security](../../57-web-security.md) (open redirects, ReDoS), [68 Rate limiting](../../68-rate-limiting-throttling.md) (scanner waves), [59 Production DB patterns](../../59-production-database-patterns.md) (recursive CTEs, keyset paging, `ON CONFLICT`), [20 HTTP server](../../20-http-server.md) (`http.Redirect`, status codes), [46 Low-latency measuring](../../46-low-latency-measuring.md) (benchmark the lookup path).
- Packages: `github.com/bits-and-blooms/bloom/v3`, `golang.org/x/exp/mmap` or `syscall.Mmap`, `sync/atomic`, `net/url` (`path.Clean` and careful percent-decoding), `go-redis` (`ZINCRBY`), `hashicorp/golang-lru/v2`, `hash/maphash` or `xxhash` for the 64-bit key.
- Test the resolver with a table of nasty inputs: `//evil.com`, `/a/../b`, `%2e%2e`, mixed case, trailing slash, 5 KB paths, NUL bytes, `\\`.
- `http.Redirect` writes a body and a default status; set `Cache-Control` yourself and use `308` where the method must survive.

## 14. Extensions

- **External targets** behind an allowlist (partners, an old domain), with the open-redirect risk spelled out.
- **Bulk import** of millions of rules with preview and undo: [48 Bulk Import](48-bulk-import.md).
- **Edge deployment:** compile the snapshot to a CDN worker's KV; how do you keep one source of truth and propagate in seconds?
- **Redirect monitor:** crawl the top 10k rules daily and alert on chains, loops and targets that now 404 (honeydolist's `redirectcheck` does this for its old domain).
- **Per-country or per-device rules** and A/B redirects (302, never indexed): what do they do to caching and to crawlers?

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| App nodes carrying the redirect middleware (Go, stateless) | 100k lookups/s worst case, one per request (§4); ~3k/s live-misses ask, ~1k/s Bloom false positives reach the table | CPU for the app's own work; the rules need memory, not CPU | app work 100k × ~0.1 ms (cached page, as in 34) = 10 vCPU; rules add 100k × 0.2 µs (hash + Bloom) + 1k × 20 µs (table probe) + a 20M-entry rebuild ≈ 10 CPU-s per 10 min ⇒ ≈ 0.1 vCPU. N = ceil(10.1 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(2.4) = 3. Memory per node: index 0.32 GB + Bloom 24 MB, both ×2 while a rebuild swaps, + miss LRU 100 MB + live-path Bloom 60 MB + Space-Saving 15 MB ≈ 1 GB heap, ×1.5 for GC; app heap ~1 GiB; strings page cache up to 2 × 2.6 GB during a swap ⇒ ≈ 8 GiB worst case | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 3, one per zone | 42 % busy, 63 % with a zone down. The packed layout is what keeps this a `c` node: a Go `map` (5 GB × 2 during a swap + GC headroom ≈ 16 GiB) would not fit 16 GiB and would force `m7g.2xlarge` |
| Postgres (redirects, redirect_changes, miss_daily, url_history) | edits and imports (a few per second; a 2M-rule import in chunks); 3 nodes poll the change log (~1 qps); 4.5 GB of rules (§4) | availability (CPU, IOPS and memory are all small) | table 3 GB + unique index 1.5 GB (§4); `miss_daily` ≈ 120k rows/day × ~150 B ≈ 18 MB/day ≈ 6.6 GB/yr; each node's 10-minute rebuild streams ~2.6 GB, so 3 × 2.6 GB ÷ 600 s ≈ 13 MB/s; a 2M-rule import is ~4M row writes in batches ≈ 1 GB with WAL: minutes, not IOPS-bound | `db.m7g.large` (2 vCPU / 8 GiB), RDS Multi-AZ | 1 + synchronous standby | 4.5 GB fits in 8 GiB and IOPS are in the tens, so not `r` or `i`; Multi-AZ because rule edits and the 404 review stop when it is down (nodes keep serving their snapshot) |

Redis: ~250 `ZINCRBY`/s (3 nodes × ~5,000 deltas per 60 s push, assumption) on ≈ 200k members × ~100 B × 1.7 ≈ 35 MB is 0.25 % of a core. It should share the platform's Redis: stand-alone, the smallest pair is `cache.r7g.large` ≈ $310 for 35 MB. The CDN carries the only bytes this feature serves: ~1k redirects/s (assumed average) × 0.4 KB + 300 404s/s × ~2 KB (the shared body, gzipped) ≈ 1 MB/s ≈ 2.6 TB a month. S3, queue, GPU: none.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| App nodes: 3 × `c7g.2xlarge` (3 × $212); if the middleware rides on an existing tier the feature adds RAM, not nodes | $640 |
| Postgres: `db.m7g.large` ≈ $120 managed, Multi-AZ doubles | $240 |
| gp3: 2 volumes × 100 GB (primary, standby) | $20 |
| CDN egress: redirects and 404 bodies, ~2.6 TB × $0.085 | $220 |
| Redis: shares the platform's (stand-alone pair ≈ $310) | $0 |
| **Total** | **≈ $1,120** (≈ $810 with a 1-year plan on the $880 compute baseline) |

- **Biggest line and why:** the app nodes, 57 %, but they exist to serve pages, not redirects. The feature itself adds ~0.1 vCPU and 5–8 GiB of RAM per node; its own bill is the database and the CDN bytes, ≈ $480. This is honest tiny hardware: the whole design is a memory layout (Bloom + packed table + overlay) that turns 20M rules into a few hundred MB, and the money is not where the design effort is.
- **What changes at 10×:** 200M rules, 1M lookups/s. App work 1M × 0.1 ms = 100 vCPU + 1 for rules ⇒ N = ceil(101 ÷ 4.27) = 24 → 24 × `c7g.2xlarge`. The per-node rebuild breaks first: 24 nodes × 26 GB ÷ 600 s ≈ 1 GB/s of database reads. Build once centrally, ship the packed file through S3 and rebuild hourly, or move the rules to a small hash-routed tier (3 × `r7g.2xlarge`, 64 GiB). The hot index alone is 3.2 GB × 2 per node, so nodes that keep the rules would be `m7g.2xlarge` (32 GiB).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 20M rules × ~150 B ≈ 3 GB heap + 1.5 GB unique index (section 4). Writes are a few per second plus occasional multi-million-row imports. The database sees ~1 qps of change-log polling and each node's 10-minute rebuild (~2.6 GB streamed); the **100k lookups/s never reach it**, they are answered from the node's Bloom filter and packed table (9.1).
- A rule write needs real constraints in one small transaction: `UNIQUE (tenant_id, match_kind, from_path)`, same-site `CHECK`s on the target, a recursive walk to refuse loops and long chains (9.2), and the rule plus its `redirect_changes` row committed together (section 8).
- The 404 stream is ~300/s, 26M/day, 5M distinct paths/day, mostly scanner junk; only the top ~5,000 paths an hour (~120k rows/day) are meant to land in a table (9.3).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `WITH RECURSIVE` with a depth guard checks loops and chain length on write; the `UNIQUE` key and the `CHECK`s (including a regex `!~` on the target) keep bad rules out at the door; `text[]` holds `top_referrers`; `pg_trgm` ranks similar live paths for suggestions (9.5); the whole data set (~4.5 GB) sits in the 8 GiB of a `db.m7g.large` |
| MySQL (RDS / Aurora) | ✔ works, with small workarounds | recursive CTEs exist in 8.0, so the loop check ports (`bool_or` becomes `MAX(path = ?)`). The `from_path` key needs care: InnoDB caps an index key at 3,072 bytes, so a path of up to 2 KB wants `VARBINARY` or a hash column, and the default collation is case- and accent-insensitive (`/café` and `/cafe` would collide), so use a binary collation. No `pg_trgm`: suggestions use token overlap in Go over the section's live paths. `top_referrers` becomes a `JSON` array |
| Redis (ElastiCache) | ✅ for the live 404 counters (derived), ✖ as the rule store | `ZINCRBY miss:{day}` on a set capped at ~200k members is ~35 MB and ~250 ops/s (section 15). As the **rule** store it would put a network round trip on every request against the lesson's ≤ 50 µs budget; a shared, hash-routed rules tier is a 10× answer (section 15), not the base design |
| DynamoDB | ✔ works for rules, no gain | rules are fetched by key and 5 GB is tiny, but the hot path never reads the store, and the change log, a recursive loop check (up to 6 key reads) and the review queue gain nothing from it |
| Athena or ClickHouse over CDN/access logs | ✔ for exact 404 audit counts, ✖ for the live dashboard | an exact count per path over 5M new distinct paths a day is the ~365 GB-a-year table of section 4; a daily `GROUP BY` over 26M log rows belongs in a columnar store if logs are already shipped (9.3), not in the OLTP database |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.large`, section 15) as the single system of record for rules, the change log, `miss_daily` and `url_history`; **everything on the hot path is derived and rebuildable**: each node's snapshot (Bloom filter, packed table, overlay) is rebuilt from the rules table, and the Redis counters are a live view. The properties that decide it: atomic write of rule and log row, constraints that refuse bad rules at write time, and the fact that the database is off the request path, so no engine speed matters at 100k lookups/s.

**If it were MySQL instead:** the same tables with a binary-collation or `VARBINARY`/hash key for `from_path`, `JSON` for `top_referrers`, `AUTO_INCREMENT` for `seq`, `REGEXP` (or application code) for the target check, and suggestion ranking moved out of SQL. The clustered primary key makes each node's 10-minute rebuild scan sequential, a small win. Material cost: the trigram suggestions and a few key-design pitfalls; the design is otherwise unchanged.

**What would flip the choice:** at 10× (200M rules, 1M lookups/s) the per-node rebuild reads ≈ 1 GB/s from the database (section 15), so build the packed file once centrally and ship it through S3, or move rules to a small hash-routed tier or an edge key-value store; a wish for exact counts of every junk path moves the 404 stream to log analytics, never a row per miss in Postgres (1.5k writes/s, ~365 GB/yr).

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 2B rules; 10M lookups/s worst case (every request); ~100k/s Bloom false positives reach the table (§4's 1 %; §9.1's live-first order would make it ~3k/s); 30k 404s/s (2.6B a day), 500M distinct paths a day; packed index 32 GB + 260 GB of strings, Bloom 2.4 GB; ~237 app nodes | rules held on every node: a 32 GB hot index is 2× a 16 GiB node (4× during the swap), and every node rebuilding (237 × 260 GB ÷ 600 s) is ≈ 100 GB/s of database reads, 100× the ~1 GB/s a primary's disk gives | take the rules off the app nodes, as §15 and §16 do for 10×, pushed further: one central builder writes versioned packed files to S3; a hash-routed rules tier (8 shards of `r7g.2xlarge`, each with a replica; index in RAM, strings on disk) answers the ~100k/s Bloom "maybes" by `hash64(path)`; app nodes keep only the 2.4 GB Bloom and the overlay (3 s delta poll). Give up: the ≤ 50 µs budget on the 1 % "maybe" path (about 0.5 ms), a rules tier to run, an hourly instead of 10-minute base rebuild | ≈ $110k/month: linear from $1,120; the rules tier adds ≈ $5k, CDN egress ≈ $22k |
| **1,000×** | 20B rules (2.5 per person on Earth: mostly sprawl); 100M lookups/s worst case (100× a hyperscale peak); 300k 404s/s (26B a day), 5B distinct paths a day; index 320 GB, strings 2.6 TB, Bloom 24 GB, table 4.5 TB | one global rule set: a 24 GB Bloom per node (48 GB during a swap) no longer fits an app node; 4.5 TB is past "a few TB comfortable" for one primary; a full rebuild reads 4.5 TB ÷ 1 GB/s ≈ 75 minutes; ~1M/s of Bloom "maybes" | cells by tenant: rules are already `tenant_id`-scoped and every request carries a host, so route by host and give each of ~100 cells today's 10× design (200M rules, 1M lookups/s, its own ~45 GB `redirects` database, central build via S3, a small rules tier); a node's Bloom shrinks to 240 MB; 404 counters merge per cell (~2.5k Redis ops/s each). Give up: cross-tenant rules (shared platform redirects are copied into each cell), global 404 analytics (a stream to a warehouse), a host-to-cell router to keep correct | ≈ $1.1M/month, linear |
| **10,000×** | 200B rules (25 per person on Earth); 1B lookups/s worst case (one request per internet user every ~5 s, all at once: beyond the real world); 3M 404s/s (260B a day), 50B distinct paths a day; ~45 TB of table, 3.2 TB packed index, 26 TB of strings | the idea of one stored rule per old URL, probed on every request: ≈ $11M/month of AWS if linear to keep ~45 TB of mostly never-requested rows, and an exact count of 50B new junk paths a day | change the problem: the decision moves to the edge (CDN edge KV; redirects are cached there, so the lookup runs once per unique URL per edge, not per request); cold rules expire by `last_hit_day` (no hit in 12 months, archived to S3 for undo) under a per-tenant rule cap, with prefix rules for families; slug-move rules stop being stored per URL (a miss consults `url_history`); 404 counts stay approximate (Space-Saving plus a sketch, logs to a warehouse for audit). Give up: exact counts, redirects for URLs unhit for a year (restorable on demand), a cap editors can hit | ≈ $11M/month if linear: more than redirects are worth (edge KV prices are not in the sheet) |

**What stays the same at every scale:** the live page wins its address (rules are asked last, so a bad rule cannot hide content); a rule and its `redirect_changes` row commit together, and readers use an immutable snapshot swapped atomically; and one shared 404 body, with 404s counted approximately and never written one row per miss.

**Say it in an interview:** "At 100× every node holding the rules breaks: a 32 GB index per node and about 100 GB/s of rebuild reads, so a central builder writes packed files to S3, a hash-routed tier holds the index, and nodes keep only the Bloom filter. At 1,000× one global rule set is the problem, a 24 GB Bloom and a 4.5 TB table, so I cut it into ~100 tenant cells routed by host, each today's 10× design. At 10,000× I stop storing and probing a rule for every old URL: redirects are decided at the edge, cold rules expire, moves are derived from slug history on a miss, and 404 counts stay approximate."
