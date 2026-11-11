# 39 · Global Search with Permission Filtering

`🟢 easy` · *Topics:* one search over many kinds of records, Postgres FTS + trigram vs a dedicated engine, a derived index fed by events (idempotent, versioned upserts), reindex by alias swap, permission filtering before ranking, identifier / number queries, typo tolerance · *Builds on:* [10 Webhook Delivery](10-webhook-delivery.md) (outbox, at-least-once), [15 Job Scheduler](15-job-scheduler.md) (workers, `SKIP LOCKED`), [05 In-Memory Cache with TTL](05-kv-cache-ttl.md) (what you can afford to keep stale), [06 View & Like Counter](06-view-like-counter.md) (hot keys)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [38 AI Draft Queue](38-ai-draft-queue.md) · Next: [40 Media Reprocessing at Scale](40-media-reprocessing.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 7u.1 puts one search box over clients, requests, orders, equipment, projects and pages. It starts as `ILIKE` over `pg_trgm` indexes (migration 0038); Elasticsearch was floated and *deliberately not chosen*: "the endpoint stays and Elasticsearch can sit behind it" if search outgrows Postgres. Step 29.2 (migration 0057) builds `search_docs`: **one row per record**, typed columns for what is filtered (`kind`, `perm`, `public`, `archived`, `status`, `town`, `rank`…), a weighted `tsvector` (name and "Also called" words = A, body = B) with a GIN index, and a GiST trigram index on name + words only (ordered nearest-first: 0.3 ms a word at 10,000 rows; matching the body by trigram took 130 ms a word, so it never does). A trigger on each source table appends the record's key to `search_dirty` *in the writer's transaction* (plain `INSERT`, so writers never wait on each other; statement-level transition tables for bulk writes; row-level only when an indexed column really changed); a worker rebuilds ≤ 500 keys every 2 s with one `INSERT … SELECT … ON CONFLICT` per kind, plus a nightly full rebuild (each row records the builder's version). A change shows in ~3 s. The match has three arms: words through `tsv`; *identifiers* (`#1235`, `DEV-0083`, an email, a public id, phone digits); and trigram per longer word for typos. An exact identifier hit ranks first, the archived last, and only kinds whose `perm` the viewer holds are returned. Step 25.5 made order/request numbers searchable ("2051" or "#2051"); step 47 added file tags; step 31 added uploaded documents' extracted text (read out on the server by a worker, searched, but never put on a card or sent to the model). Measured at 10,000 rows: p95 5–8 ms, 20 of 20 invoices found first by number.
> **Here, at scale:** 5B documents across 100k tenants, 3,000 queries/s at peak, results strictly limited to what the asker may see, index fresh within seconds.

## 1. The task

> "One search box for the whole product. Type a name, a number, an email or a few words and get matching customers, orders, invoices, files and pages, ranked, instantly, and never anything I'm not allowed to open."

Search is a *derived copy* of your data. The design is about how the copy is fed, who may see which rows of it, and when to leave Postgres.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Is a search ever across tenants? | If never, tenant is a routing key and everything gets cheap. | Never: always one tenant. |
| How many kinds, what fields? | Document shape. | ~10 kinds (client, order, invoice, file, page…); title, aliases, body text, number, status, dates. |
| Permissions? | Where ACL lives. | Kind-level (a permission) plus some record-level (owner, team). |
| Freshness? | Indexing design. | New or changed record searchable in ≤ 5 s p99; ACL changes as fast. |
| Scale? | Engine choice. | 5B docs, 100k tenants (avg 50k, a few huge), 3k queries/s peak. |
| Languages? | Analyzers. | Mostly one; keep multi-language possible. |
| Typo tolerance, typeahead? | Extra query paths. | Typos yes; typeahead separate, prefix only. |
| IDs and numbers? | Users paste them. | Must find `#2051`, emails, phone numbers, public ids exactly and first. |
| Total hit counts, deep pages? | Cost and leaks. | No exact total; top 100 per query. |
| Deletes and erasure? | Stale hits. | Deleted records gone in seconds; erasure verified. |

## 3. Requirements

**Functional**
- `GET /search?q=` over all kinds the viewer may see; filter by kind; highlight; open the record.
- Exact identifier lookup (number, public id, email, phone), ranked first.
- Typo tolerance on names; synonyms/aliases ("also called" words).
- Index updates from every write path, including bulk imports and merges.
- Reindex everything (new analyzer, new field) with no downtime.

**Non-functional**
- p95 < 100 ms (seed: 5–8 ms at 10k rows); freshness ≤ 5 s.
- No cross-tenant or cross-permission leaks, including counts, highlights and suggestions.
- Search down must not take the product down.

**Out of scope**
- Semantic/vector search, personalised ranking, cross-tenant public search.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Documents | 5B; ~50k per tenant on average | 5B ÷ 100k (a few tenants hold 10M–50M: heavy tail) |
| Raw text | ~5 TB | 5B × ~1 KB (title, aliases, body excerpt, card) |
| Index on disk | ~5–7.5 TB; ~10–15 TB with 1 replica | assume index ≈ 1–1.5× text |
| Dedicated engine shards | ~160 primaries | ~6.25 TB ÷ ~40 GB (a common shard-size rule of thumb) |
| Sharded Postgres instead | 64 shards × ~78M docs | 5B ÷ 64; × ~2 KB ≈ 156 GB of rows per shard, 300+ GB with indexes |
| Queries | 3k/s peak; ~47/s per Postgres shard | one tenant → one shard; 3,000 ÷ 64 |
| Churn | 2 %/day ≈ 100M updates ≈ 1,160/s avg, ~3,500/s peak | assumed 2 % of docs change daily |
| Freshness budget | ≈ 3.5 s | 2 s poll + 0.5 s batch + 1 s index refresh |
| Full reindex | ≈ 28 h at 50k docs/s | 5 × 10⁹ ÷ 5 × 10⁴ = 10⁵ s |
| Seed, for scale | 10k docs ≈ 20 MB, p95 5–8 ms | measured |

What the numbers tell us: the read load is trivial *because every query is one tenant's*; the storage (5B docs) and the write stream (1–3.5k docs/s, forever) are what decide the architecture. A full reindex takes more than a day, so reindexing must be an online operation, and the queue that feeds the index must be a log, not a table you insert into and delete from 100M times a day.

## 5. Plan

1. Choose the store (stay on Postgres or add an engine) by the numbers that flip it.
2. Document shape: typed filters, weighted text, ACL tokens, version.
3. Get data in: events, idempotent versioned upserts, deletes, fan-out of renames.
4. Reindex without downtime.
5. Permission filtering before ranking.
6. Query understanding: classify, identifiers, typos, ranking.
7. Failure: the index is a copy; the primary decides.

## 6. High-level design

```
 writers ── tx: row + outbox event ──► relay ──► stream (partition = hash(tenant, doc))
                                                     │
                                       indexer workers (batch 500–1,000)
                                       upsert with external version = source version; deletes = tombstones
                                                     ▼
 browser ─► Search API ─► classify query ─► index (routed by tenant: 1 shard)       alias docs → docs_v7
              │   tenant_id + ACL tokens injected from the session (never from the request)
              │   top-K ids ──► verify against primary (batch PK lookup) ──► hits with cards
              └─ on index failure: 503 for search only; per-list filters still work
 reindex: snapshot primary by keyset ranges → docs_v8, catch up from stream offset, compare counts, flip alias
```

One query: the API takes tenant and permissions from the session, classifies the text (an identifier or words), asks one shard for the top ~40 by rank with the ACL filter *inside* the query, verifies those ids against the primary, and returns 20 hits. One write: the row and an outbox event commit together; the relay publishes; an indexer upserts the document if its version is newer than what is indexed.

## 7. API

```
GET /v1/search?q=&kinds=client,order&limit=20&cursor=
  → 200 { hits: [{ kind, id, public_id, title, card, highlights?, score }], next_cursor?, more: true }
          (no exact total; at most ~100 results reachable: search is not a list)
GET /v1/search/suggest?q=&limit=8       → prefix matches on titles and aliases, same ACL filter
POST /v1/admin/search/reindex           { tenant_id?, kind?, mapping_version }  → 202 { job_id }
GET  /v1/admin/search/lag               → { p50_ms, p99_ms, oldest_unindexed_age_s, drift_by_kind }
```

Event on the stream: `{ tenant_id, kind, id, op: "upsert"|"delete", version }`. The indexer re-reads the source row at `version` (or later), so a dropped or duplicated event only costs a redundant read.

## 8. Data model

One document per record. The DDL is one Postgres shard; the same fields map one-to-one onto an OpenSearch/Elasticsearch mapping (`keyword` for ids/numbers/ACL, `text` for title/aliases/body).

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;  CREATE EXTENSION IF NOT EXISTS btree_gin;  CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE search_docs (
  tenant_id   bigint   NOT NULL,
  kind        text     NOT NULL,
  id          bigint   NOT NULL,                  -- the row's id in its own table
  public_id   text     NOT NULL,                  -- c_…, o_…: what links use
  number      bigint,                             -- the human number: Order #2051
  acl         text[]   NOT NULL,                  -- any-of tokens that grant access: {'perm:leads.read','team:12','user:77'} or {'*'}
  archived    boolean  NOT NULL DEFAULT false,    -- searched, but ranked last
  title       text     NOT NULL,
  words       text     NOT NULL DEFAULT '',       -- aliases / "also called" / suggested words
  body        text     NOT NULL DEFAULT '',       -- searched, never returned: notes, message text, extracted file text
  rank        real     NOT NULL DEFAULT 0,        -- 0–1 tie-breaker, recomputed nightly
  card        jsonb    NOT NULL,                  -- the few fields shown; never notes, streets, phones, emails
  src_version bigint   NOT NULL,                  -- older events are ignored
  builder     smallint NOT NULL,                  -- a newer builder rebuilds every row
  tsv         tsvector GENERATED ALWAYS AS (      -- 'simple': one config cannot stem several languages (the seed used 'english')
                setweight(to_tsvector('simple', title || ' ' || words), 'A') ||
                setweight(to_tsvector('simple', body), 'B')) STORED,
  PRIMARY KEY (tenant_id, kind, id)
) PARTITION BY HASH (tenant_id);                  -- e.g. 16 partitions per shard server
CREATE INDEX search_docs_tsv  ON search_docs USING gin  (tenant_id, tsv);                                 -- tenant first
CREATE INDEX search_docs_trgm ON search_docs USING gist (tenant_id, (title || ' ' || words) gist_trgm_ops); -- names only, never body
CREATE INDEX search_docs_acl  ON search_docs USING gin  (tenant_id, acl);
CREATE UNIQUE INDEX search_docs_number ON search_docs (tenant_id, kind, number) WHERE number IS NOT NULL;

CREATE TABLE tenant_search_route (                -- which shard a tenant lives on; whales get their own
  tenant_id bigint PRIMARY KEY, shard smallint NOT NULL, dedicated boolean NOT NULL DEFAULT false
);
```

## 9. Deep dives

### 9.1 Postgres FTS or a dedicated engine?

| Option | Pros | Cons |
|---|---|---|
| **One Postgres** (the seed) | no second store; transactional triggers; trivial ops | ~100M docs is a ceiling for one node; one huge tenant slows everyone |
| **Postgres sharded by tenant** (64 × ~78M) | same tech, queries never fan out | you build the shard map, rebalancing, reindex tooling and replicas yourself |
| **OpenSearch/Elasticsearch, routed by tenant** | sharding, replicas, analyzers, facets, synonyms, relevance tuning built in | a second store to feed, monitor and upgrade; eventual consistency |
| Hosted search service | least ops | cost at 5B docs, data-residency, limits |

The numbers that flip it: **documents per node** (≈ 100M is where one Postgres stops being relaxed), **one whale tenant** (tens of millions of docs, which GIN posting lists make slow), **languages and analyzers**, **facets/aggregations**, **relevance work** (synonyms, boosts, A/B), and **who runs it**. The seed stays on Postgres at ~10⁴ docs and keeps the escape hatch: the endpoint is the contract, the engine sits behind it.

**Choice and why.** At 5B docs, a dedicated engine with **routing by tenant** (`routing = tenant_id`, so a query hits one shard and never scatter-gathers 160), whales on dedicated shards, and Postgres remaining the source of truth. Sharded Postgres is a respectable alternative, but at this size we would be rebuilding what the engine ships. Keep the seed's discipline: the index is a derived, rebuildable copy.

### 9.2 Getting data in

| Option | Pros | Cons |
|---|---|---|
| Dual-write from app code | simple | one write fails, the index drifts; every write path must remember |
| **Dirty-key queue fed by triggers, polled by a worker** (the seed) | no write path is missed (imports, merges, a hand-run SQL fix); same transaction | at 1,160 docs/s that is ~100M inserts and ~100M deletes a day on a queue table: bloat, vacuum pressure |
| **Outbox event → log/stream → indexers** | durable, replayable, partitioned for per-doc order | needs the relay and a stream |
| CDC from the WAL | catches *every* write, even without app cooperation | extra infrastructure; row-shaped events need re-reading to build a document |

**Choice and why.** Keep the seed's key insight (*no write path may be missed*, so mark in the writer's transaction) but move the transport to a log: the trigger or app code writes an outbox event, a relay publishes it, and indexers consume partitions keyed by `(tenant, doc)` so one doc's events stay ordered. Details:

- **Idempotent, versioned upserts.** Index with an *external version* equal to the source row's version: an older or duplicate event is a no-op. At-least-once delivery becomes exactly-once in effect (see [10](10-webhook-delivery.md)). Deletes are tombstones with a version, so a late "upsert" cannot resurrect a deleted record.
- **Events carry keys, not documents.** The indexer reads the current row(s) and builds the document, so an event is "a reason to look" (the seed's builder works the same way: it re-reads the live rows for each marked key).
- **Batch.** 500–1,000 docs per bulk request: 3,500 docs/s is ~4 bulk calls/s.
- **Fan-out of renames.** The seed marks a client's orders and invoices when the client is renamed, because their documents contain the name. A client with 1M orders means 1M reindexes. Either don't denormalise names into other kinds (store the client id and resolve at query time), or send fan-out events to a **low-priority lane with a rate cap**; never let it sit in front of ordinary updates.
- **Drift repair.** Triggers miss `TRUNCATE`; consumers have bugs. Run a nightly reconcile: per-(tenant, kind) counts and sampled version comparisons against the primary; enqueue repairs. This is the seed's nightly `search.rebuild`, made incremental.
- **Freshness and read-your-writes.** ~3.5 s end to end is fine for "found it", annoying for "I just created it": for the writer's own session, merge their last few writes from the primary into the first page, or return the new record's link directly from the create call.

### 9.3 Reindex without downtime

A new analyzer, a new field or a mapping bug means rebuilding 5B docs, which is ~28 h at 50k docs/s, so it must be online:

1. Create `docs_v8` with the new mapping. Point the **alias** `docs` at `docs_v7` (reads and writes go through the alias).
2. Record the stream offset; **dual-write** new events to v7 and v8 from that offset.
3. Bulk-load v8 from the primary by **keyset ranges per tenant** (resumable, parallel, rate-limited so production isn't starved), with the same external versions, so a loaded row never overwrites a newer event.
4. Compare counts per tenant/kind and sample documents; watch lag.
5. **Flip the alias** atomically to v8; keep v7 a day for rollback; drop it.

The seed's cheaper variant works at small scale: every row records the builder's `version`, a deploy that changes a row's shape makes the worker rebuild *every* row in place, and a nightly rebuild repairs drift.

### 9.4 Permission filtering: before ranking, not after

| Option | Pros | Cons |
|---|---|---|
| Post-filter: search, then check each hit against the source | trivial to reason about | a page of 20 may show 3; **counts and facets leak** existence; N checks per query; deep pages break |
| **Pre-filter inside the query: tenant + ACL tokens** | correct pagination and counts; one round trip | ACL must be indexed; changes need reindex |
| One index per user or per role | simple queries | combinatorial; ACL changes rebuild indexes |
| Pre-filter coarsely, then verify top-K against the primary | catches complex rules; defence in depth | one extra batch lookup (~1–5 ms for 20 PK reads) |

**Choice and why.** Pre-filter coarsely, verify the top-K. Mechanics:

- **Tenant** comes from the session and is injected by the Search API (never from request parameters); it is also the routing key.
- **ACL tokens, any-of.** Each doc lists the tokens that grant access (`perm:leads.read`, `team:12`, `user:77`, `*`); the viewer's token set is built from their session (their permissions, teams, own id) and the filter is `acl && viewer_tokens`. The seed's single `perm` column is the one-token version of exactly this ("only kinds whose perm the viewer holds").
- **Expand groups at query time, not into documents.** Adding someone to a team must not reindex every doc the team can see.
- **ACL changes are index writes** on a priority lane with an SLA of seconds; revoking a person's account is handled at the API (immediate), not through the index.
- **No exact totals, ever.** `more: true` and a capped `next_cursor`; counts and facets are computed inside the same ACL filter.
- **Cards, highlights and suggestions obey the same rule.** The card never holds restricted fields (the seed: "never notes, a street, a phone or an email"; uploaded text is searched but never returned, and a test plants a phrase to prove it); highlight only fields the viewer can read; suggestions are filtered like results.
- **Open-by-id re-authorizes against the primary.** The copy finds the record; the live row decides.
- Rules that are *not* any-of (AND conditions, time-boxed shares) don't fit tokens: pre-filter by the coarse token, then verify the top-K with the real check.

### 9.5 Understanding the query: identifiers, typos, ranking

```go
var (
    reNumber = regexp.MustCompile(`^#?(\d{3,12})$`)             // "2051", "#2051"
    rePublic = regexp.MustCompile(`^[a-z]{1,2}_[0-9A-Za-z]{12}$`) // "o_8fK2…": prefix + 12 base62
)
func classify(q string) Plan {
    q = norm.NFKC.String(strings.TrimSpace(q))
    switch {
    case reNumber.MatchString(q):  return Plan{Exact: ByNumber(digits(q))}   // unique (tenant, kind, number)
    case rePublic.MatchString(q):  return Plan{Exact: ByPublicID(q)}
    case strings.Contains(q, "@"): return Plan{Exact: ByEmail(strings.ToLower(q))}
    case len(digits(q)) >= 7 && mostlyDigits(q): return Plan{Exact: ByPhone(digits(q))}
    }
    return Plan{Text: q, Fuzzy: true}
}
```

- **Identifiers are lookups, not searches.** The seed made numbers searchable on purpose ("it is read out on the phone"): `2051` or `#2051` finds Order 2051. A btree lookup on `(tenant, kind, number)` beats any text query and is ranked first (a client first among identifier hits, since an email or phone names a person). Never let a text arm outrank an exact id.
- **Words.** AND the terms first; if too few hits, fall back to OR (the seed ORs, because its input is a whole question; humans typing a search box expect AND). Weight title and aliases (A) above body (B); break ties with the nightly `rank` (popularity/recency), put archived last, and boost the kind a question names ("owed" → invoices).
- **Aliases/synonyms** live in `words`: hand-typed plus model-suggested (step 29: "yellow digger", "mini ex" find the excavator). Suggested words are derived data with a drop (✕) list: the seed never re-suggests a dropped word.
- **Typos on short fields only.** Trigram/fuzzy matching runs on *names and aliases*, never on long bodies (130 ms a word at 10k rows in the seed), and only for terms the exact arm found nothing for. GiST trigram returns nearest-first, so a word stops at its best few instead of reading every row that shares a trigram.
- **Typeahead is a different path:** prefix on titles and aliases, tiny result cap, own rate limit; don't run the full three-arm query per keystroke.
- **Depth.** Keyset cursor on `(score, id)`, capped around 100 results. Users refine; exports are a different feature.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (50B docs, 30k queries/s):** the write path first (indexing throughput, 35k docs/s at peak, queue lag); then whale tenants (dedicated shards); then reindex time (10× longer, so incremental mapping changes only); the read load stays easy because it is tenant-routed.
- **Single points of failure:** the stream/relay (events buffer in the outbox table, nothing is lost), the index cluster (search 503s; the rest of the product runs, and per-list filters keep working as in the seed), the primary.
- **Lag:** alert on *oldest unindexed event age*, not on throughput; page at 30 s.
- **A stale hit:** a deleted record still in the index is dropped by the top-K verify; the user sees nothing wrong.
- **Poison event:** a record whose builder throws goes to a dead-letter table with its key; the batch continues.
- **Erasure requests:** a delete event on the priority lane, then a verify query by id and a reconcile pass.
- **Abuse:** per-user rate limit and a cap on query length and term count (a 500-term query is a DoS); `limit` capped at 20, depth at ~100.
- **Cost:** shard sizing and replica count are the bill; use one replica, tier cold tenants to cheaper nodes.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| dedicated engine routed by tenant | one big Postgres | scale, analyzers, tuning; one-shard queries | a second system to feed and run |
| outbox → log → indexers | polling a dirty table | durable, replayable, per-doc order | relay and stream to operate |
| external versions on every upsert | trusting event order | duplicates and reordering are harmless | every doc needs a monotonic source version |
| ACL tokens, pre-filter | post-filtering | correct pages and counts, no leaks | ACL changes are index writes |
| verify top-K on the primary | trusting the copy | stale or orphan hits never shown | one batch of PK reads per query |
| identifiers as exact lookups | text matching numbers | exact, first, cheap | a classifier to maintain |
| no exact total, depth ≤ 100 | "1,203 results" and page 40 | cheap queries, no count leaks | users must refine |

## 12. Check yourself

1. Why does tenant scoping make 3,000 queries/s across 5B documents easy?
2. Why is post-filtering by permission wrong, and what do you do instead?
3. An event arrives twice, and a delete arrives before an older upsert. What stops a wrong index state?
4. How do you change the mapping of a 5B-document index without downtime?
5. How does the search treat `#2051` differently from `kubota excavator`, and why are typos handled on a separate path?

<details><summary>Answers</summary>

1. Every query touches one tenant, so with routing it hits one shard; 3,000/s spread over 64 (or ~160) shards is ~47/s each, and no query scatter-gathers. Size and write rate are the hard parts, not read load. Whale tenants are moved to dedicated shards.
2. Post-filtering returns pages with holes (20 fetched, 3 visible), leaks existence through totals and facets, and costs N source checks per query. Pre-filter inside the query by tenant and ACL tokens, so counts and pages are right, then verify the top-K against the primary as defence in depth.
3. Upserts carry an external version (the source row's version), so a duplicate or older event is ignored; a delete is a versioned tombstone, so an older upsert cannot bring the record back. Reconcile jobs repair anything missed.
4. Build the new index beside the old one behind an alias: dual-write new events, bulk-load from the primary by keyset ranges with the same external versions, compare counts and samples, flip the alias atomically, keep the old index briefly for rollback.
5. `#2051` is classified as an identifier and answered by a btree lookup on `(tenant, kind, number)`, ranked first. The words go through the weighted full-text arm (AND, then OR). Typo matching (trigram/fuzzy) runs only on short name fields and only for terms with no exact hit, because fuzzy matching long bodies is slow and noisy.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (bulk upserts, `ON CONFLICT`), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (the outbox relay), [44 Background jobs & queues](../../44-background-jobs-queues.md) (indexer workers, dead letters), [43 Authorization, RBAC & multi-tenancy](../../43-authorization-rbac-multitenancy.md) (building the viewer's token set), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED`, keyset pagination, versioned writes), [52 Heaps & priority queues](../../52-heaps-priority-queues.md) (merging top-K lists), [55 Data pipelines](../../55-data-pipelines.md) (batching and backpressure), [36 Resilience](../../36-resilience-patterns.md), [38 Caching](../../38-caching-patterns.md) (cache the viewer's token set briefly).
- The indexer's inner loop: read a batch of events, collapse by `(tenant, kind, id)` keeping the highest version, load those rows, build documents with a pure `build(row) Doc`, send one bulk request, ack the offsets only after the bulk call succeeds; failed items go to the dead-letter table with their key.
- Packages: `pgx`, a Kafka/NATS client (`franz-go`, `nats.go`), the official Elasticsearch or OpenSearch Go client, `golang.org/x/text/unicode/norm`, `golang.org/x/sync/errgroup`, `regexp`.
- Test builders as pure functions with golden documents; test "the card never holds the text" by planting a phrase in `body` and asserting it appears in no response (the seed's trick).

## 14. Extensions

- **Typeahead service:** prefix index, per-tenant popularity, results within 50 ms.
- **Semantic/vector search** with ACL-aware retrieval (filter first, then rank) and a hybrid score with keyword search; feeds the assistant ([37](37-records-assistant.md)).
- **Facets and saved searches** per tenant; why facets make counts a leak surface.
- **Learning to rank / click feedback:** log clicks, tune boosts, A/B test rankers.
- **Multi-language analyzers** with a language field per document and per-tenant defaults.
- **Cross-tenant "public" search:** a second index with only public docs, and none of the ACL machinery.
- **Index tiering:** hot tenants on SSD, cold tenants on cheaper nodes, searched lazily.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Search API (Go, stateless) | 3,000 queries/s | CPU | 1 ms CPU a query (classify, build the query, decode hits, JSON) × 3,000 = 3 vCPU. N = ceil(3 ÷ (0.8 × 2 × ⅔ = 1.07)) = ceil(2.8) = 3. Each query also does one 20-row verify: 3,000 batch lookups/s ≈ 30 % of one node's ~10k reads/s, spread over the product's existing replicas | `c7g.large` (2 vCPU / 4 GiB) | 3 | CPU glue around two network calls: `c`; 50 % busy normally, 75 % with a zone down; the verify load lands on existing replicas, not new hardware |
| Index data nodes | 5B docs = 6.25 TB of primaries + 1 replica = 12.5 TB; 3,000 q/s; 3,500 docs/s | disk (NVMe) first; CPU about equal; memory | disk: 12.5 TB ÷ (937 GB × 0.75 ≈ 700 GB) = 17.9 → 18. CPU (x86 vCPUs): 3,000 × 10 ms + 3,500 × 2 ms = 30 + 7 = 37 vCPU ÷ (0.8 × 4 × ⅔ = 2.13) = 17.3 → 18. Heap 16 of 32 GiB; ~17 of 312 shard copies per node ≈ 700 GB | OpenSearch Service on `i4i.xlarge` (4 vCPU / 32 GiB / 937 GB NVMe); self-run on EC2 is half the price | 18, six per zone | `i`: the index must live on local NVMe (merges plus ~17 shards serving random reads; gp3's 3,000 IOPS baseline is thin), 8 GiB per vCPU leaves heap + page cache; not `r`: we need disk per node, not RAM. Zone loss: 12 survivors at 37 ÷ 48 = 77 %, shards run single-copy until the zone returns (12.5 TB does not fit 12 nodes to re-replicate) |
| Dedicated masters | ~312 shard copies of cluster state | memory (state), not load | quorum of 3 | `m7g.large`, managed | 3 | availability of the cluster, not throughput |
| Event stream | 3,500 events/s × ~200 B = 0.7 MB/s; ~20 GB a day | nothing (tens of MB/s per partition is the limit) | 3 days × 20 GB × 3 replicas = 180 GB | MSK `kafka.m7g.large` (managed ≈ 2 × `m7g.large`); the lesson's NATS on 3 small nodes is the cheaper self-run | 3 | three brokers for quorum and zones, at 1/50 of a partition's rate |
| Indexers + outbox relay | 3,500 docs/s | CPU | 1 ms (re-read, build card, bulk body) × 3,500 = 3.5 vCPU; N = ceil(3.5 ÷ 2.13) = 2 → 3 | `c7g.xlarge` (4 vCPU / 8 GiB) | 3 | CPU plus waits; ~4 bulk calls/s; no more consumers than partitions |

**Reindex is a capacity event, not just 28 h.** 50k docs/s is ~100 vCPU of indexing at 2 ms (more than the whole 72-vCPU cluster) plus a second 6.25 TB copy. Plan a burst of 18 more `i4i.xlarge` for ~2 days (9,000 × 48 ÷ 730 ≈ $590; ≈ 36 h at the spare ≈ 78 vCPU ÷ 2 ms ≈ 39k docs/s) and ~50 vCPU of Spot builders (4 × `c7g.4xlarge`, ≈ $20 for 28 h).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Search API: 3 × `c7g.large` (3 × $53) | $159 |
| Index: 18 × `i4i.xlarge`, managed ($250 × 2 = $500 each) | $9,000 |
| Masters: 3 × `m7g.large`, managed ($120 each) | $360 |
| MSK: 3 × `kafka.m7g.large` ($120 each) + 180 GB × $0.08 | $375 |
| Indexers: 3 × `c7g.xlarge` (3 × $105) | $315 |
| S3 snapshots: 6.25 TB × $0.023 | $144 |
| ALB: $16 + ~11 LCU (≈ 11 GB/h of results) | $80 |
| Data out: ~1,000 q/s average (assumed peak ÷ 3) × ~3 KB gzipped ≈ 7.9 TB × $0.09 | $710 |
| **Total** | **≈ $11,100** (≈ $7,600 with a 1-year commitment on the $10.2k of steady compute) |

- **Biggest line and why:** the index nodes, 81 %, because 5B documents are 12.5 TB that must sit on NVMe, not because of queries (3k/s is the easy part). The lever is bytes: a smaller card and a shorter body excerpt, tenant tiering to cheaper nodes, or running them yourself (half). Sharded Postgres is not cheaper: 64 × `db.r7g.2xlarge` Multi-AZ at $1,240 ≈ $79k.
- **What changes at 10×:** 50B docs = 125 TB with its replica ⇒ ~180 `i4i.xlarge` or, better for a stateful tier, 24 × `i4i.8xlarge` (7.4 TB NVMe each) ≈ $96k a month managed; 30k q/s ≈ 370 vCPU with indexing, still storage-bound. The write path runs out first (35k docs/s ⇒ 9 × `c7g.2xlarge` indexers; Search API 9 × `c7g.2xlarge`), then whale shards and reindex time (280 h at 50k docs/s, so incremental mapping changes only).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 5B documents (~1 KB each, ~5 TB of text, 6.25 TB of index, 12.5 TB with a replica) across 100k tenants, and every query stays inside one tenant (section 4).
- Reads are search, not lookup: weighted words, typo-tolerant names (trigram), exact identifiers by a btree on `(tenant, kind, number)`, and an any-of ACL filter applied before ranking; 3,000 queries/s at peak, then a 20-row verify against the primary.
- Writes are a stream: ~1,160 document updates/s on average, ~3,500/s at peak, forever, fresh within ~5 s; a full reindex is ~28 h, so it must be online (9.3).
- The index is a derived copy: the product's rows decide, and the index can always be rebuilt from them.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen as the system of record, ✔ as the index below ~100M docs, ✖ as the index at 5B | `tsvector` with weights plus GIN, `pg_trgm` GiST for typos, a `text[]` ACL with `&&` and a partial unique index on `number` are the whole seed (p95 5–8 ms at 10k docs). At 5B it means 64 shards × ~78M docs that you build, rebalance and reindex yourself, ≈ $79k a month against ≈ $11k for the whole OpenSearch design (section 15) |
| MySQL (RDS / Aurora) | ✔ as the system of record, ✖ as the index | the source rows and the outbox use only portable features. As an index it has basic `FULLTEXT` only (no weighted `tsvector`, no trigram index), a `FULLTEXT` index cannot lead with `tenant_id`, and the ACL overlap would be a JSON array with a multi-valued index; at 5B it has the same sharding bill as Postgres |
| OpenSearch Service | ✅ chosen as the derived index | `routing = tenant_id` sends a query to one shard, `keyword` ACL tokens filter before ranking, analyzers, synonyms and fuzzy matching replace the trigram arm, and an alias swap makes the 28 h reindex online (9.1, 9.3); never the source of truth |
| Kafka (MSK) | ✔ as the event log, not as a store of documents | 3,500 events/s × ~200 B is 0.7 MB/s; per-`(tenant, doc)` ordering and replay are what a dirty-key table taking ~100M inserts and deletes a day cannot give (9.2) |
| DynamoDB / Cassandra | ✖ poor fit | key access only: no full-text, no typo matching, no ranking; it could hold documents by key, but the primary already does |

**Pick:** PostgreSQL on RDS/Aurora stays the system of record. It is the product's own database; section 15 adds only the verify lookups on its existing replicas, and the outbox event commits in the same transaction as the row. OpenSearch Service (18 × `i4i.xlarge` plus 3 masters) is the derived index, fed through MSK. What decides it: tenant routing (one shard per query), an inverted index with analyzers and fuzzy matching at 5B documents, and the alias swap for an online reindex. Derived and rebuildable: the index and the stream; the record: the primary.

**If it were MySQL instead:** nothing material on the record side. `src_version` compare-and-set and the outbox are portable, and `INSERT … ON DUPLICATE KEY UPDATE` replaces `ON CONFLICT`. The index is OpenSearch either way, so the product's engine does not change this design. What changes is the small-scale path of the seed: without trigram and weighted full-text, a MySQL product reaches for OpenSearch at its first typo-tolerant search box instead of at ~100M documents.

**What would flip the choice:** down, under ~100M documents and no whale tenant, drop OpenSearch and keep the seed's single-Postgres `search_docs` (the cheat sheet puts Postgres full-text at tens of millions of documents). Up, at 10× (50B docs, ~125 TB with the replica) storage and indexing throughput run out, not the engine: fewer, larger `i4i.8xlarge` nodes (section 15); a hosted search service when nobody wants to run the cluster (9.1).

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 500B docs (~60 per person alive), 625 TB of primaries, 1.25 PB with a replica; 300k queries/s, 350k docs/s at peak (~700 vCPU of indexing on the data nodes); ~1,800 `i4i.xlarge` or ~225 `i4i.8xlarge`; ~15.6k primary shards | one cluster as a unit, not indexing (that is just more indexers): ~31k shard copies of cluster state on 3 masters; at 1,800 nodes one fails every ~3 h (5,000 nodes ≈ one an hour); a reindex is 5 × 10¹¹ ÷ 5 × 10⁴ = 10⁷ s = 116 days (it also needs a second copy, section 15). The verify is 300k batch lookups/s, ~30 replicas' worth of the product database | scale out: ~10 independent OpenSearch clusters of ~125 TB, each the 10× design of section 15 (24 × `i4i.8xlarge`), routed by `tenant_search_route` (it already holds `shard`); a whale gets its own cluster; reindex per cluster in parallel (5 × 10¹⁰ ÷ 5 × 10⁴ = 10⁶ s ≈ 12 days) and mostly as incremental mapping changes; ~84 indexers and ~72 API nodes on `c7g.2xlarge`; cold tenants to cheaper nodes. Give up: cross-cluster operations (a reindex of everything is ten roll-outs), a whale move is a reindex | ≈ $1.1M/month: index ~80 %, data out $71k |
| **1,000×** | 5T docs, 6.25 PB of primaries, 12.5 PB with a replica; 3M queries/s, 3.5M docs/s at peak; a whale tenant is 10–50B docs (10M–50M × 1,000), more than the whole baseline | tenant routing itself: one whale holds 12–62 TB ≈ 310–1,560 shards, so its queries scatter across hundreds of shards; one pipeline reindex is 10⁸ s ≈ 3.2 years; the index alone is ≈ $9M a month | change the shape: ~1,000 **cells**, each today's whole design (Postgres slice, outbox and stream, one index cluster, API) for ~5B docs behind a `tenant → cell` router; split a whale across 16–64 sub-shards by `hash(doc_id)` and scatter-gather with a top-K merge; for documents untouched for a year (assume 80 %) index only titles, aliases and identifiers, and make body search a slower "search the archive" path. Give up: whale-tenant latency and cost (a real fan-out), body search on old documents, cross-cell admin queries | ≈ $11M/month |
| **10,000×** | 50T docs (6,000 per person alive: a thought experiment), 62.5 PB of primaries, 125 PB with a replica; 30M queries/s, 35M docs/s at peak | money and physics: the index alone is ≈ $90M a month (125 PB on NVMe), plus 35M writes/s forever to keep a copy fresh for queries that mostly want an identifier or a name | change the problem: index less, approximately. Exact identifiers (number, public id, email, phone) never need an inverted index: a hash or btree lookup in the owning cell. Names, aliases and titles for everything (~0.3 KB a doc, assumed), full text only for a hot slice (recent 90 days, active tenants); a per-shard Bloom filter of terms lets a query skip shards that cannot match; the app syncs a person's recent records to the device and searches them there first. Give up: instant full text on everything, typo tolerance on cold data, exact counts of anything | ≈ $111M/month as is; roughly a quarter with the name-only tier |

**What stays the same at every scale:** the index is a derived, rebuildable copy (the primary decides; events carry keys and upserts carry the source version); a query is routed by tenant (one shard, one cell, or a bounded fan-out for a whale); the ACL filter runs before ranking and the top-K is verified against the primary; identifiers are exact lookups ranked first.

**Say it in an interview:** "At 100× indexing is just more indexers, but one cluster of 31k shard copies and a 116-day reindex is not operable, so I split into ~10 tenant-routed clusters of the 10× design. At 1,000× the whale tenants are bigger than my whole baseline, so I run ~1,000 cells, split whales across sub-shards with scatter-gather, and index only titles for old documents. At 10,000× the index bytes are the bill, so I stop indexing everything: identifiers as lookups, names for all, full text for a hot slice, the rest on demand."
