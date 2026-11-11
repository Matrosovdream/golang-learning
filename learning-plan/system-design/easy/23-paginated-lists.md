# 23 · Paginated Lists at a Billion Rows

`🟢 easy` · *Topics:* offset vs keyset pagination, cursor tokens and tiebreakers, row-value comparison, exact vs capped vs maintained counts, partial and composite indexes for filters + sort, deep-page and export limits, consistency while paging · *Builds on:* [01 URL Shortener](01-url-shortener.md) (read-heavy baseline), [06 View & Like Counter](06-view-like-counter.md) (counts that must not scan)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [22 Audit Log & Revision History](22-audit-log-revisions.md) · Next: [24 Data Lifecycle: Archive, Delete, Erase](24-data-lifecycle.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 7o gave every admin list `page` / `per_page` (25 by default, `400` above
> 100), `meta { total, page, per_page, total_pages }`, the page in the URL and a `Pager` under each list. It removed
> `LIMIT 200` caps that hid 19,800 of 20,000 clients and replaced "newest 200 of each table merged in Go" (which
> skipped requests) with one SQL query. It chose **offset** on purpose: "keyset only if a list ever outgrows that".
> Steps 11 and 56 then measured it at 200k requests and 800k orders. `count(*) OVER ()` made a list read and sort every
> row (orders list 1.0–2.3 s, then 40 ms after choosing ids from an index and counting separately); generic prepared
> plans took 17 ms and 94 ms where custom plans took 0.04 ms and 7.5 ms; one index turned a 16 ms scan into 0.07 ms.
> Partial indexes (`WHERE deleted_at IS NULL`, `WHERE is_spam`) keep live lists small. Worst measured p95 on the
> 20k-row generated data: 101.5 ms.
> **Here, at scale:** an inbox/admin list over 1B rows (32 shards, biggest tenant 50M rows), 50 filter combinations,
> 20k list requests/s.

## 1. The task

> "Design the list screen of an admin app: a table of records you can filter, sort, search and page through.
> There are a billion records. It must feel instant, for the first page and for the hundredth."

Everyone has written `LIMIT 25 OFFSET n`. The task is knowing which parts of that quietly stop scaling (the offset,
the count, the sort) and what each replacement gives up.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Is a list always scoped to a tenant? | Lets every index lead with `tenant_id`. | Yes. |
| Need "page 400 of 805" and jump-to-page? | Random access is what keyset gives up. | No: next/previous plus the first few pages. |
| Must the total be exact? | Exact counts cost a scan. | Not above 1,000: "1,000+" is fine. |
| Which filters and sorts? | Indexes can serve only a few. | ~8 filters (any combination), default sort newest first, 3 sortable columns. |
| Search in the same list? | Ranked search can't be keyset-paged easily. | Yes; capped at 1,000 results. |
| May rows change while I page? | Defines acceptable duplicates and skips. | Yes; no row shown twice, a skip of moved rows is tolerable. |
| Export the whole result? | Must not be a 40M-row page loop. | Async job, not an API page. |
| Scale? | Chooses everything below. | 1B rows, 32 shards by tenant, 20k list requests/s. |

## 3. Requirements

**Functional**
- List with filters, one sort, search and paging; show whether more exists and a total (exact below 1,000).
- Next/previous with a stable position; the position survives a reload (the URL holds it).
- Status-tab badges ("New 12") and a "Mine" filter.
- Export a filtered list to a file.

**Non-functional**
- p95 under 100 ms at any depth the UI allows; first page p99 under 50 ms.
- A list query never scans a whole tenant's table; unbounded work is refused (`400`/`422`), not attempted.
- No row appears twice in a page sequence; a heavy tenant must not slow others.

**Out of scope**
- Full-text relevance ranking (task 39), analytics grids, spreadsheets with arbitrary sort of every column.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Rows / per shard | 1B / ~31M | 1,000M / 32 |
| Storage | ~1 TB table + ~400 GB indexes | 1 KB per row; ~10 indexes at ~40 B per entry = 40 GB each |
| List requests | 20k/s peak, ~625/s per shard | 20,000 / 32; each is a page query plus a count |
| Response size | ~15 KB, ~300 MB/s at peak | 25 rows × ~600 B; × 20,000/s (gzip, and slim list rows) |
| Cost of skipping a row (offset), cached | ~0.1–1 µs | index-only entries are cheaper, heap fetches dearer (assumed) |
| Page 1,000 (offset 25,000) | 2.5–25 ms | 25,000 × 0.1–1 µs |
| Page 200,000 of a 50M tenant | 0.5–5 s per request | 5,000,000 × 0.1–1 µs |
| Exact `COUNT(*)` of that tenant | ~3 s | the seed: 12 ms for 200k rows = 60 ns/row; × 50M |
| Capped count (stop at 1,001) | ≤ 1 ms | reads at most 1,001 index entries |
| Keyset page | 0.05–0.5 ms at any depth | the seed's first pages: 0.02–0.45 ms |

What the numbers tell us: at the seed's size everything is fast, so offset and exact counts were the right
simple choice. At 1B rows **work grows with depth (offset) and with result size (count)**, so both must become
bounded. Keyset makes page cost constant; capped or maintained counts make the total cost constant.

## 5. Plan

1. Switch paging from "skip n rows" to "continue after this row" (keyset) and design the cursor.
2. Make totals bounded: capped, estimated or maintained.
3. Design indexes for the list's filters and sort, and decide what is not supported.
4. Refuse unbounded work: deep offsets, huge pages, exports through the list API.
5. Be explicit about what happens when rows change between pages.

## 6. High-level design

```
 browser ──GET /v1/requests?status=new&limit=25&cursor=…──► LB ──► API (stateless)
                                                                    │ 1. validate: limit ≤ 100, sort ∈ allowed set, cursor signature + filter hash
                                                                    │ 2. page query: keyset, LIMIT 26 (one extra row = has_more)
                                                                    │ 3. total: capped count (≤ 1,001)  or  maintained counter for tab badges
                                                                    ▼
                          read replica of the tenant's shard  (lists tolerate ~1–2 s of lag)
                          requests: partial composite indexes, deleted_at IS NULL
 export ──POST /v1/requests/exports──► job queue ──► worker: keyset loop of 5,000 rows on a replica ──► file in object storage
 badges ◄── counters (tenant, status) kept by events, refreshed by a worker (approximate, a few seconds old)
```

*One list request:* decode and check the cursor, run one indexed range scan that starts *after* the cursor row and
reads 26 rows, run a count that stops at 1,001, return rows, `next_cursor`, `has_more`, `total`. Cost is
independent of how deep the user is.

## 7. API

```
GET /v1/requests?status=new&type=rental&assigned=me&from=2026-01-01&sort=-created_at&limit=25&cursor=eyJh…
  → 200 { "items": [ … ],
          "page":  { "next_cursor": "eyJh…", "prev_cursor": "eyJi…", "has_more": true },
          "total": { "value": 1000, "exact": false } }          // "1,000+"
GET /v1/requests/counts?by=status                               → 200 { "new": 12, "contacted": 40, … , "as_of": "…" }
GET /v1/requests?page=3&per_page=25                             → 200 (compat: offset below 10,000, i.e. page ≤ 400)
GET /v1/requests?page=401                                       → 400 { "type": "offset-too-deep", "detail": "narrow the filters or export" }
GET /v1/requests?limit=101                                      → 400   (as the seed: per_page above 100 is a 400)
POST /v1/requests/exports { "filters": {…} }                    → 202 { "job": "…" }
```

The cursor is opaque base64: `{ sort values of the last row, id, filter-hash }`, HMAC-signed. A cursor used with different
filters or another tenant is rejected. `sort` is limited to an allow-list: each allowed sort has an index.

## 8. Data model

```sql
CREATE TABLE requests (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    bigint      NOT NULL,
    status       text        NOT NULL,
    request_type text        NOT NULL,
    assigned_to  bigint,
    is_spam      boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
    -- … name, email, search_text, etc.
);

-- The default screen: newest first. Partial: deleted rows never enter the index.
CREATE INDEX requests_list_idx ON requests (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
-- The status tabs: equality column first, then the sort order; INCLUDE lets badges read the index only.
CREATE INDEX requests_status_idx ON requests (tenant_id, status, created_at DESC, id DESC)
    INCLUDE (request_type) WHERE deleted_at IS NULL;
-- "Mine".
CREATE INDEX requests_mine_idx ON requests (tenant_id, assigned_to, created_at DESC, id DESC)
    WHERE assigned_to IS NOT NULL AND deleted_at IS NULL;
-- The Spam chip alone (the seed's migration 0082); other lists only apply a cheap `NOT is_spam` filter.
CREATE INDEX requests_spam_idx ON requests (tenant_id, created_at DESC, id DESC) WHERE is_spam AND deleted_at IS NULL;

CREATE TABLE list_counters (                     -- maintained by events; approximate, refreshed every few seconds
    tenant_id bigint NOT NULL, status text NOT NULL, n bigint NOT NULL, as_of timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, status)
);
```

The keyset page, with the cursor `(created_at, id) < ($3, $4)` (a row-value comparison the index serves directly),
and one extra row to learn `has_more`:

```sql
SELECT id, status, request_type, created_at FROM requests
 WHERE tenant_id = $1 AND deleted_at IS NULL AND status = $2
   AND (created_at, id) < ($3, $4)
 ORDER BY created_at DESC, id DESC LIMIT 26;
```

## 9. Deep dives

### 9.1 Offset vs keyset

| Option | Pros | Cons |
|---|---|---|
| **Offset** (`LIMIT 25 OFFSET n`, the seed) | page numbers and jump-to-page; trivial | cost grows with depth (page 200,000 ≈ 0.5–5 s); duplicates and skips when rows are inserted |
| **Keyset** (`WHERE (created_at, id) < cursor`) | constant cost at any depth; stable under inserts at the head | no jump-to-page; going back needs the reverse query or a cursor stack; sort columns need an index and a tiebreaker |
| Page-boundary table (materialise every 25th key) | jump-to-page and fast | a write path to maintain, stale under churn |
| **Hybrid: offset for the first N pages, keyset beyond** | familiar `Pager` for the common case | two code paths; offset capped below 10,000 |

**Choice and why.** **Keyset** for the API, with a **bounded offset compatibility mode** (page ≤ 400) because the seed's
`Pager` and page-in-the-URL are worth keeping. A keyset query needs three things: (1) **a total order**: add the
unique `id` as a tiebreak (`ORDER BY created_at DESC, id DESC`), or rows sharing a timestamp vanish across a page
boundary; (2) **a row-value comparison** `(created_at, id) < ($3, $4)`, which Postgres answers from the
`(created_at DESC, id DESC)` index (the expanded `created_at < $3 OR (created_at = $3 AND id < $4)` is often planned
worse); (3) **a cursor carrying the last row's sort values**, not a page number. Mixed directions
(`created_at DESC, id ASC`) break the row comparison and the index: keep all sort keys in one direction. Nullable sort
columns need `NULLS LAST` in index and comparison; easiest is to avoid sorting on them.

```go
type Cursor struct {
	At time.Time `json:"a"`
	ID int64     `json:"i"`
	F  string    `json:"f"` // hash of the filters this cursor belongs to
}

func (s *Store) Page(ctx context.Context, tenant int64, status string, cur *Cursor, limit int) ([]Row, *Cursor, error) {
	const q = `SELECT id, status, request_type, created_at FROM requests
	  WHERE tenant_id = $1 AND deleted_at IS NULL AND status = $2
	    AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4))
	  ORDER BY created_at DESC, id DESC LIMIT $5`
	var at *time.Time
	var id int64
	if cur != nil {
		at, id = &cur.At, cur.ID
	}
	rows, err := s.pool.Query(ctx, q, tenant, status, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Row])
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if len(items) > limit { // the extra row proves there is more
		items = items[:limit]
		last := items[limit-1]
		next = &Cursor{At: last.CreatedAt, ID: last.ID, F: filterHash(status)}
	}
	return items, next, nil
}
```

### 9.2 Counts

`COUNT(*)` is a scan: the seed measured 12 ms for a total of 200k rows (60 ns/row); linear, that is ~3 s for a
50M-row tenant, on **every** list request. And the seed's own `count(*) OVER ()` was worse: it forced the whole set
to be joined and sorted (orders list 1.0–2.3 s, a sort spilling to disk) and is the first thing to remove.

| Option | Pros | Cons |
|---|---|---|
| Exact `COUNT(*)` with the filters | always right | linear in rows: unusable above ~1M matches |
| Planner estimate (`EXPLAIN` rows) | free | off by 10× with correlated filters; not user-grade |
| **Capped count** (`SELECT count(*) FROM (SELECT 1 … LIMIT 1001) t`) | cost bounded (≤ 1,001 index entries); exact when small | "1,000+" above the cap |
| **Maintained counters** (per tenant × status) | O(1) reads for tab badges | updated by events (a hot row for a busy tenant: shard it, task 06); eventually consistent |

**Choice and why.** **Capped count for the list footer** ("1–25 of 1,000+"), and **maintained, slightly stale counters
for the status tabs**, refreshed by a worker every few seconds from the events (a recount per tenant on a schedule
heals drift). The seed's default screen reads "New" from an index alone, and that stays right: a covering partial index
makes a small-status count cheap. Page and count run in one round trip (`pgx.Batch`), but as two statements
with the same `WHERE`, never as a window function.

### 9.3 Filters + sort versus index design

Eight filters in any combination are hundreds of combinations; no index set covers them all. The seed's rules
generalise:

- **Index for the sort, lead with equality filters, finish with the sort keys:** `(tenant_id, status, created_at DESC, id
  DESC)`. The planner walks it in order and stops after 26 rows. A filter that is not in the index is applied to rows as
  they are read: cheap if the filter keeps most rows, slow if it rejects almost all (the scan runs a long way
  looking for 26 matches).
- **Partial indexes** for what the UI makes hot: the default screen, the "New" tab, "Mine", the Spam chip. `WHERE
  deleted_at IS NULL` keeps removed rows out of every list index; the seed's spam index is `WHERE is_spam AND deleted_at IS NULL`,
  tiny because few rows match.
- **A very selective filter wins** (an email, a phone, a public id): the planner uses its own index and sorts the few
  matches in memory. Keep those lookups as separate "find" endpoints.
- **Allow-list the sorts.** The seed notes that sorting by client name, status, total or distance "reads everything — they
  have no index to use". At 50M rows that is a scan per request, so each allowed sort gets an index or is refused.
- **Filtered and sorted columns live in one table** (the seed's rule 8). A filter on a joined table forces a join
  and a sort over the join; denormalise the column onto the list's table.
- **Several tables in one list** (the seed's old inbox): merge them into one table. "Newest 200 per table, merged
  in Go" skips rows (the seed's bug); a SQL merge of the top *n* per arm is correct but slow under a selective filter.
- **Late row lookup:** choose the 25 ids from an index first (index-only), then fetch those rows, the seed's
  `ListIDs` then `List` (40 ms at page 1,000 instead of 2 s).
- **Plan with the values:** generic prepared-statement plans cannot see `$n IS NULL OR …` and chose scans (17 ms and
  94 ms instead of 0.04 ms and 7.5 ms in the seed). Set `plan_cache_mode = force_custom_plan` for list queries. Pass
  search terms as parameters, never computed in SQL, and require ≥ 3 characters for trigram search.

### 9.4 Deep pages, big pages, exports

| Abuse | Limit |
|---|---|
| `limit=100000` | max 100 rows per page (`400`, as the seed) |
| `?page=200000` | offset below 10,000 (page ≤ 400): beyond, `400 offset-too-deep`, "narrow the filters or export" |
| Scripts walking the API | per-token rate limit ([03](03-rate-limiter.md)); `statement_timeout` of ~2 s on list queries |
| "Export everything" | `POST …/exports`: a worker keyset-loops (5,000 rows a batch, a fence `created_at <= $started_at` for a consistent cut, sleeping between batches) on a **replica** and writes a file to object storage; one export per tenant at a time |
| Heavy tenant | per-tenant concurrency cap on list queries so one tenant's 50M rows do not use a shard's connections |

### 9.5 Rows change while you page

| Change | Offset | Keyset on `(created_at, id)` |
|---|---|---|
| New row inserted at the head | every later page shifts: a row is shown twice | later pages unaffected; the new row appears on a refresh of page 1 |
| Row deleted | a row is skipped | unaffected |
| Sort key changes (sorting by `updated_at`) | rows jump, are duplicated or skipped | a row can jump above the cursor and be missed; for a column that only grows, none is shown twice |

**Choice and why.** The default sort is **immutable** (`created_at`, `id`) because that makes paging stable. If users
must sort by a mutable column, accept that a moved row may be missed and say so ("new activity may reorder"). For
"refresh", keep the cursor and poll for rows newer than the head instead of re-paging.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** counts and sorts for whale tenants. Make maintained counters the only source for
  badges, add per-tenant concurrency limits, and for arbitrary filter + sort + search over 50M+ rows use a search
  engine fed by the same events ([task 39](39-global-search.md)) rather than more indexes.
- **Index count and write cost:** each index is ~40 GB and one more write per row change (four list indexes plus the key
  is ~5 per insert, fewer for partial ones). Add one only for a screen the product commits to; drop unused ones (`pg_stat_user_indexes`).
- **Replica lag:** a user edits a record, returns to the list and does not see it. Read from the primary for ~5 s
  after that user's write, or patch the row into the client's list.
- **Planner surprises:** a statistics change flips a plan to a scan. Alert on list p99 and keep `EXPLAIN` tests per
  list (the seed has `perf_plans_test.go`).
- **Stale or tampered cursors:** signed and filter-hashed; a foreign or expired one is a `400`, never a wrong page.
- **Thundering refresh:** a dashboard polling every few seconds multiplies load; cache the default first page 2–5 s per
  (tenant, filters) and coalesce identical in-flight requests (singleflight).
- **Index builds:** `CREATE INDEX` blocks writes at this size; use `CONCURRENTLY` (the seed's warning in 0078).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Keyset cursors | offset paging | constant cost at any depth, stable paging | jump-to-page; cursor handling in the client |
| Capped counts + maintained counters | exact totals | bounded cost, O(1) badges | "1,000+" and slightly stale badges |
| Sort allow-list with an index each | sort by any column | no table scans per request | product flexibility |
| Partial composite indexes | one index per filter | the hot screens read a handful of entries | write cost per index; planner dependence |
| Offset compat mode capped at page 400 | unlimited offsets | the familiar `Pager` for common cases | two paths to test |
| Async exports from a replica | large pages via the API | predictable shard load | latency to get the file |
| Immutable default sort | sort by last update | no skips or duplicates | users cannot sort by "recently touched" by default |

## 12. Check yourself

1. Why does page 200,000 with `OFFSET` cost 0.5–5 s on a 50M-row tenant while a keyset page costs milliseconds?
2. Why must the cursor and `ORDER BY` include `id`, and what does `(created_at, id) < ($3, $4)` do?
3. What is wrong with `count(*) OVER ()` and with exact totals, and what do you use for "of N" and for tab badges?
4. You have 8 filters and users combine them freely. How do you handle indexes, and what do you refuse to support?
5. A customer asks to download 40M filtered rows. What do you build?

<details><summary>Answers</summary>

1. Offset must walk and discard every skipped row: 5M rows × ~0.1–1 µs = 0.5–5 s, growing with depth and run on every request.
   Keyset starts the index range scan *at* the cursor's key, reads 26 rows and stops, so the cost is the same at any depth.
2. `created_at` alone is not unique; rows sharing a timestamp across a page boundary would be skipped or repeated.
   `id` makes the order total. The row-value comparison means "after this exact position in the `(created_at, id)`
   order", which Postgres answers from the matching composite index as a single range start.
3. `count(*) OVER ()` forces the whole filtered set to be read (and sorted) just to attach a total to 25 rows; exact
   `COUNT(*)` is linear in matches (~3 s for 50M). Use a capped count for the footer ("1,000+") and
   maintained, slightly stale counters (per tenant × status) for badges; run page and count as separate statements.
4. Index for the sort with equality filters leading (`tenant, status, created_at DESC, id DESC`), partial
   indexes for the hot screens (`deleted_at IS NULL`, "New", "Mine"), no index per combination. Refuse sorts without an index,
   offsets of 10,000 or more, pages above 100 rows, and unbounded search results; point heavy users to search or export.
5. An async export job: a worker keyset-loops 5,000 rows per batch on a replica with a `created_at` fence for
   consistency, throttles itself, streams to a file in object storage and notifies the user; one export per tenant at a time.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md), [59 Production DB patterns](../../59-production-database-patterns.md) (keyset queries, partial indexes, `CREATE INDEX CONCURRENTLY`, `EXPLAIN`), [41 API design & evolution](../../41-api-design-evolution.md) (cursor pagination, problem details for `400`), [21 REST API](../../21-rest-api.md), [38 Caching patterns](../../38-caching-patterns.md) (short-lived first-page cache, singleflight), [44 Background jobs](../../44-background-jobs-queues.md) (exports).
- Packages: `pgx` (`pgx.Batch` for page + count in one round trip, `CollectRows`, and `RuntimeParams["plan_cache_mode"] = "force_custom_plan"` on the pool config), `crypto/hmac` for cursors, `golang.org/x/sync/singleflight`, `encoding/base64`.
- Test with data: 1M-row generated fixtures per list, assertions on `EXPLAIN (FORMAT JSON)` (no `Seq Scan`, no `Sort` node on the default screens), and a property test that paging through a shuffled dataset returns each row exactly once.

## 14. Extensions

- **Jump-to-page** with a materialised page-boundary table, and what it costs under churn.
- **Faceted counts** ("rental 31, quote 12") per filter without a scan: a search engine or incremental aggregates.
- **Saved filters and views**, shared between users, stored as filter objects the API validates.
- **Virtualised infinite scroll** in the UI over keyset pages, with prefetch of the next cursor.
- **Search results paging** (similarity-ranked): keyset on `(score, id)` with a frozen scoring snapshot ([task 39](39-global-search.md)).
- **A columnar store** (ClickHouse) for analytics-style lists where users sort and filter on anything.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| List API (Go, stateless) | 20k list requests/s at peak, each a page query + a capped count; ≈ 15 KB responses | CPU (JSON + gzip) | assume 0.7 ms per request (cursor check, scan 26 rows, JSON ≈ 0.1 ms, gzip ≈ 0.4 ms): 20k × 0.7 ms = 14 vCPU. N = ceil(14 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(3.3) = 4 → 6 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 6, two per zone | CPU-bound, and gzip is the biggest slice, worth it because it cuts egress ~5×; memory ≈ 3 GiB (a 2–5 s first-page cache plus connections) of 16, so `c`; 29 % busy at peak, 44 % with a zone down; 6 nodes × 10 connections per shard = 60 per replica, so no pooler |
| Postgres shards (`requests` + four list indexes), lists read from replicas | 32 shards, ≈ 625 list requests/s each at peak; ≈ 31M rows and ≈ 44 GB per shard (1 TB + 400 GB ÷ 32) | memory, then CPU | DB CPU per request ≤ 1.5 ms (keyset page ≤ 0.5 ms + capped count ≤ 1 ms): 625 × 1.5 ms = 0.94 of 4 vCPU; the 12.5 GB of indexes and the newest rows stay cached in 32 GiB; deep pages miss: ≈ 625 × 3 pages × 15 % ≈ 280 IOPS, far under gp3's 3,000 | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS | 32 shards × (primary + read replica in another zone) = 64 nodes | memory-bound, so `r` not `m` (16 GiB would not hold the indexes); the data is small, and `r7g.2xlarge` would hold a whole shard in RAM at double the price, which is not needed; the replica is also the failover target. The primaries' write rate is the app's and is not in section 4 |

Export workers and the counter refresher: 3 × `c7g.large` for availability. A 40M-row export is 8,000 batches of 5,000 rows with pauses, under 30 minutes at ~0.2 s a batch, and a ≈ 24 GB file in S3 (40M × 600 B): nothing to size there. `list_counters` is tiny.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 6 × `c7g.2xlarge` (6 × $212) | $1,270 |
| Workers: 3 × `c7g.large` (3 × $53) | $160 |
| ALB: $16 + ~54 LCU × $0.008 × 730 (≈ 54 GB/h gzipped at the average) | $330 |
| Postgres: 64 × `db.r7g.xlarge` (64 × $310) | $19,800 |
| gp3: 64 × 100 GB × $0.08 | $510 |
| Data out: ≈ 39.4 TB × $0.09 (assumed average 5k req/s, gzipped ≈ 5×) | $3,550 |
| **Total** | **≈ $25,600** (≈ $18,200 with 1-year commitments on the $21,230 of compute) |

- **Biggest line and why:** the 64 database nodes, ≈ 77 %. By this lesson's own loads the list side is small (20k × 1.5 ms = 30 vCPU of DB work over 1.4 TB), so a handful of `r7g.4xlarge` replicas would carry it; the 32 × 2 comes from section 4's tenant sharding, so check whether lists need that before buying it. Next is egress: gzip saves ≈ $14k (75 MB/s raw ≈ 197 TB ≈ $17.7k a month); the API tier is only ≈ 5 %.
- **What changes at 10×:** 200k req/s ⇒ 140 vCPU ⇒ N = ceil(140 ÷ 8.53) = 17 → 18 × `c7g.4xlarge` (≈ $7.6k); each replica takes 6,250 req/s × 1.5 ms ≈ 9.4 vCPU, and 10× rows (440 GB per shard) no longer fit RAM, so `db.r7g.4xlarge` (16 vCPU / 128 GiB), 64 × $1,240 ≈ $79k; egress ≈ 394 TB ≈ $35k. First cache the first page 2–5 s with singleflight and move whale tenants' free-form filters to a search engine (task 39), as section 10 says.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 1B mutable rows (status, assignee and soft delete change) on 32 shards: ≈ 31M rows and ≈ 44 GB a shard (≈ 1 KB a row plus its indexes), and one tenant with 50M rows (section 4).
- 20k list requests/s at peak (≈ 625 a shard), each a keyset page of 26 rows plus a capped count of at most 1,001 index entries, on read replicas that may lag 1–2 s (sections 6, 15).
- Lists are reached through filters, not by key: ≈ 8 filters in any combination, 3 allowed sorts, hot screens served by partial composite indexes (8, 9.3).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora), sharded by tenant | ✅ chosen | partial composite indexes keep dead rows out (`WHERE deleted_at IS NULL`, and `WHERE is_spam AND deleted_at IS NULL` for the tiny Spam chip); `INCLUDE (request_type)` lets the badges read the index only; the row-value cursor `(created_at, id) < ($3, $4)` is one range start; `force_custom_plan` and `EXPLAIN` tests fix the plan trap the seed measured |
| MySQL (RDS / Aurora), sharded by hand or with Vitess | ✔ works, with workarounds | the secondary index carries the primary key, so the `id` tiebreak is free and keyset paging is natural; but no partial indexes, no `INCLUDE`, and the row-value cursor needs checking (below) |
| DynamoDB / Cassandra | ✖ poor fit | every list needs its own key: a GSI per (filter, sort) pair, and eight filters in any combination cannot be a set of GSIs (the default quota is 20 per table); a `FilterExpression` runs after the read and you pay for what was read, so a selective filter reads the tenant; there is no cheap count (`Select=COUNT` still reads the items). Keyset is native (`LastEvaluatedKey`): the one part that fits |
| OpenSearch | ✔ as a derived index for whale tenants | free-form filter + sort + search over 50M+ rows (section 10, task 39), paged with `search_after`; fed by the events, never the record |
| Redis (ElastiCache) | ✔ as a cache | the first page cached 2–5 s per (tenant, filters) behind singleflight, and optionally the badge counters; rebuildable, not the record |

**Pick:** PostgreSQL on RDS, 32 shards of `db.r7g.xlarge`, each a primary plus a read replica in another zone, with lists served from the replica (section 15). Three things decide it: partial composite indexes that make the hot screens read a handful of entries, the row-value keyset comparison, and an index-only (`INCLUDE`) path for badges and the capped count (9.1–9.3). OpenSearch and Redis are derived; Postgres is the only system of record.

**Sharded relational, or something else?** For the list workload alone, sharding is not what the numbers ask for: 1.4 TB in all and 20k × 1.5 ms = 30 vCPU of database work is the size of one Aurora cluster (storage grows to 128 TiB, up to 15 low-lag readers) with a few `r7g.4xlarge` readers, which is what section 15 says too ("check whether lists need that before buying it"). The 32 × 2 nodes come from the platform's tenant sharding in section 4, and it costs nothing in query shape because every list is `tenant_id`-scoped, so no query ever fans out across shards. Shard when one primary cannot take the writes or the data, by the key you query by; replicas fix a read-bound list. Under Vitess a MySQL fleet would use the same `tenant_id` key, with online resharding, if you run it. DynamoDB or Cassandra would only fit if the screens shrank to "newest first per tenant and tab", one GSI each.

**If it were MySQL instead:**
- No partial indexes: each list index covers all ≈ 31M rows a shard (≈ 1.25 GB each at the lesson's ≈ 40 B an entry) instead of only live ones, the Spam index stops being tiny, and all four list indexes are written on every insert (≈ 5 index writes, no longer "fewer for partial ones"). To seek past dead rows, lead with a generated `is_live` column that is `1` for live rows and `NULL` otherwise: `(tenant_id, is_live, status, created_at, id)`.
- No `INCLUDE`: put `request_type` last in the key to make the index covering. `DESC` indexes exist from 8.0, and keeping every sort key in one direction keeps backward scans cheap anyway.
- The row-value cursor is accepted syntax, but verify with `EXPLAIN` that it becomes an index range; the expanded form `created_at < ? OR (created_at = ? AND id < ?)` is the safe one on MySQL. List reads are non-locking, so gap locks do not touch the read path.
- The identity key is append-only and clustered, so "choose 25 ids from an index, then fetch the rows" stays the same two-step (a deferred join).

**What would flip the choice:** whale tenants with arbitrary filter + sort + search over 50M+ rows (section 10) go to OpenSearch fed by the same events, not to more indexes or another primary database. An analytics-style list where users sort and filter on any column points to ClickHouse (14). At 10× (200k req/s, 440 GB a shard that no longer fits RAM) the step is `db.r7g.4xlarge` replicas and a first-page cache, as section 15 says.
