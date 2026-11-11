# 48 · Bulk Import with Preview & Undo

`🟢 easy` · *Topics:* staged import pipeline (upload → stage → dry-run preview → commit in chunks), idempotent upsert by external id + content hash, undo by run id, fairness and backpressure between tenants, hostile files (XML entities, CSV formula injection) · *Builds on:* [15 Job Scheduler](15-job-scheduler.md) (chunk jobs, leases), [11 Image Upload & Hosting](11-image-upload-hosting.md) (presigned upload), [03 Rate Limiter](03-rate-limiter.md) (per-tenant write budget)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [47 Privacy-Friendly Web Analytics](47-web-analytics.md) · Next: [49 Load Testing Harness](49-load-testing-harness.md)

---

> 🌱 **Seed (honeydolist-site):** the project imports twice. **PLAN step 7n / migration 0036** brings clients, orders and
> invoices in from the invoicing provider through a four-step wizard (what, which, preview, run): `import_runs`
> (`previewing → previewed → queued → running → done | failed | undone`, a `preview` JSON, `counts`) and `import_items`
> (entity, our id, their id, action `created | linked | skipped | failed`, reason). It runs in the worker, a second run
> takes only what is new, and **Undo** removes what the run created in dependency order (invoices, orders, clients, links),
> *sparing whatever a person changed since*. **PLAN step 82** imports the owner's SEO text from XML: Preview writes
> nothing and sorts every field into new / fill / overwrite / same; Apply saves each record through the editor's own use case,
> one transaction per record, an overwrite only when confirmed, and an empty field never clears text (file limit: 5 MB, 1,000
> records). Both handle tens to thousands of rows. ADR 0006's amendment: an undone import leaves a *gap* in customer-facing numbers.
> **Here, at scale:** a SaaS onboarding importer: 2,000 imports a day, up to 10M rows each, 500 in flight, per-row errors, hostile files.

## 1. The task

> "Design the bulk import for a SaaS product. A customer uploads a CSV or XML file of up to ten million
> records. Show them what will happen before anything changes, apply it reliably, report the rows that
> failed, and let them undo the whole import."

Importing is where bad data, double clicks, crashes and curiosity meet. The interesting parts are not
parsing: they are *never half-applied*, *safe to re-run*, and *undo that does not destroy later work*.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Formats and size? | Streaming vs in-memory; limits. | CSV and XML, up to 2 GB / 10M rows. API-based connectors are an extension. |
| Create only, or update existing records too? | Matching rules, overwrite risk. | Both, chosen per import: `create_only`, `fill` (empty fields only), `overwrite`. |
| How are existing records matched? | The idempotency key. | By `external_id` (the source system's id), within tenant and source. |
| Must there be a preview? | The staged design. | Yes: a dry run with counts and samples; nothing in the real tables changes until confirmed. |
| What happens to a bad row? | Per-row vs per-file failure. | A bad row is reported and skipped; a file that is not an import file at all is refused whole. |
| Undo: how far back, how complete? | Storage and semantics. | 30 days; removes what the import created and reverts what it updated, except what a person changed since. |
| Do imported rows fire events (emails, webhooks)? | Side effects cannot be undone. | No: imports run with notifications suppressed and nothing is published. |
| Scale? | Queues, fairness, DB write budget. | 2,000 imports a day, mean 100k rows, max 10M, 500 in flight, peak 5× average. |

## 3. Requirements

**Functional**
- Upload a file, map columns, get a preview (counts of create / update / same / skip / error, plus samples) before any real write.
- Commit the previewed import in the background with visible progress, resumable after a crash.
- Re-running the same file changes nothing the second time.
- Downloadable report of the rows that failed and why; fix-and-re-upload works.
- Undo a finished import while the data it touched is unchanged.

**Non-functional**
- An import never applies half of a *chunk*; a crash never duplicates or loses a row.
- One tenant's 10M-row import must not starve other tenants' small imports.
- Preview of a 100k-row file within ~30 s; progress feedback for larger ones.
- A hostile file (XML bomb, huge line, formula cells) cannot hurt the service or the customer.

**Out of scope**
- Continuous sync with a provider (webhook + pull + watermark: see [18](18-inbound-webhook-receiver.md)); merging duplicates ([25](25-contact-dedupe-merge.md)); column-mapping UX.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Rows per day | 200M | 2,000 imports × 100k mean |
| Average / peak write rate | ~2.3k / ~12k rows/s | 200M ÷ 86,400 = 2,315; × 5 for a Monday-morning onboarding wave = 11.6k |
| Largest file | 2 GB | 10M rows × ~200 B per CSV row |
| Raw uploads kept 30 days | ~1.2 TB | 2,000 × 20 MB = 40 GB/day × 30; object storage, cheap |
| Staging rows kept 7 days | ~0.84 TB | 200M × ~600 B (raw + parsed JSON + hash) = 120 GB/day × 7 |
| Per-row result log (30 days) | ~360 GB | only created/updated/failed rows, ~60% × 200M × ~100 B = 12 GB/day × 30 |
| Parse + stage speed | ~50k rows/s per worker | 10M rows ≈ 200 s; the median 20k-row file ≈ 0.4 s |
| A 10M-row commit at a fair 1k rows/s | ~2.8 h | 10,000,000 ÷ 1,000 = 10,000 s |

What the numbers tell us: parsing is cheap and parallel; **the commit phase is the shared, scarce
resource** (indexed writes to the real tables, WAL, replicas). 500 imports in flight does not mean 500
writing: most are uploading, parsing or waiting for a human to press *Commit*. Peak demand (11.6k rows/s)
can exceed what one write shard should give imports (~8k rows/s, an assumption to measure), so commit is
a *queue with a fair scheduler*, and the UI shows an ETA, not a promise.

## 5. Plan

1. The pipeline as states: upload → stage → preview → commit → done / undone.
2. Classifying a row (create / update / same) and writing it idempotently, in resumable chunks.
3. Undo: what is recorded, what is spared, what cannot be undone.
4. Fairness and backpressure across tenants; hostile input and the report.

## 6. High-level design

```
 browser ──presigned PUT──► object storage  raw/{tenant}/{sha256}.csv      (immutable, 30 d)
    │ POST /imports {file_key, mapping, mode}
    ▼
 Import API ──► import_runs (previewing)  ──► queue ──► Parse workers
                                                          │ stream file, validate, normalise, hash
                                                          ▼
                                         staging table (UNLOGGED, one per day)  +  row errors
                                                          │ set-based join on external_id
                                                          ▼
                                  import_runs (previewed): counts + samples      ◄── user reviews, resolves ambiguities
                                                          │ POST /commit  (Idempotency-Key)
                                                          ▼
                       fair scheduler ──► chunk jobs (5,000 rows, keyset on row_no) ──► target tables
                                                                                     └► import_items (created/updated/failed)
 undo: reverse chunks in dependency order, guarded by content hashes ───────────────────► done | failed | undone
```

Upload goes straight to object storage (the API never proxies 2 GB). Parsing *streams* the file into
a staging table; the preview is a SQL join of staging against the target; commit walks the staged rows
in chunks, each chunk one transaction that also advances the run's cursor.

## 7. API

```
POST /v1/imports/uploads                         → 201 { file_key, upload_url }        presigned PUT, size cap in the policy
POST /v1/imports { file_key, kind, mapping, mode } → 202 { id, status: "previewing" }
GET  /v1/imports/{id}                            → 200 { status, progress, preview: { create, update, same, skip, error, samples } }
GET  /v1/imports/{id}/rows?verdict=error&after=…  → 200 page (keyset), reason + line number
PUT  /v1/imports/{id}/decisions { row_no: "link:{id}" | "create" }   → 200   resolve "needs a look" matches
POST /v1/imports/{id}/commit { confirm_overwrites: "none" | "all" }   Idempotency-Key → 202
POST /v1/imports/{id}/undo                       Idempotency-Key → 202
GET  /v1/imports/{id}/report.csv                 → 200   failures and outcomes, formula-safe
```

`commit` and `undo` are the two dangerous verbs: both take an `Idempotency-Key` and both are no-ops
when the run is already in that state. `tenant_id` always comes from the session, never from the file.

## 8. Data model

```sql
CREATE TABLE import_runs (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id     text   NOT NULL UNIQUE,
  tenant_id     bigint NOT NULL,
  kind          text   NOT NULL,                       -- contacts | orders | pages …
  file_key      text   NOT NULL, file_sha256 bytea NOT NULL, file_bytes bigint NOT NULL,
  mapping       jsonb  NOT NULL,
  mode          text   NOT NULL CHECK (mode IN ('create_only','fill','overwrite')),
  status        text   NOT NULL DEFAULT 'previewing'
                CHECK (status IN ('previewing','previewed','queued','running','done','failed','undoing','undone','expired')),
  staging_table text,                                  -- import_rows_20261111, dropped after 7 days
  preview       jsonb, counts jsonb NOT NULL DEFAULT '{}',
  cursor_row    bigint NOT NULL DEFAULT 0,             -- last committed row_no, advanced inside each chunk's transaction
  error         text,
  created_by    bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  started_at    timestamptz, finished_at timestamptz, undone_at timestamptz
);
-- one active write per tenant: a second Commit while one runs is refused, not interleaved
CREATE UNIQUE INDEX import_runs_one_active ON import_runs (tenant_id) WHERE status IN ('queued','running','undoing');

CREATE UNLOGGED TABLE import_rows_20261111 (           -- staging: re-creatable from the file, so no WAL
  run_id bigint NOT NULL, row_no bigint NOT NULL,        -- line number: errors point at it
  external_id text, data jsonb NOT NULL, content_hash bytea NOT NULL,
  verdict text NOT NULL DEFAULT 'pending' CHECK (verdict IN ('pending','create','update','same','skip','error')),
  error text,
  PRIMARY KEY (run_id, row_no)
);

CREATE TABLE import_items (                            -- only rows that WROTE or FAILED; same/skip are counts
  run_id bigint NOT NULL REFERENCES import_runs(id) ON DELETE CASCADE,
  entity text NOT NULL, external_id text NOT NULL, entity_id bigint,
  action text NOT NULL CHECK (action IN ('created','updated','failed')),
  wrote_hash bytea,                                    -- what the import wrote: undo compares against it
  before jsonb,                                        -- previous values of an update
  reason text,
  PRIMARY KEY (run_id, entity, external_id),
  CHECK (action = 'failed' OR entity_id IS NOT NULL)
);

ALTER TABLE contacts ADD COLUMN external_id text, ADD COLUMN content_hash bytea,
                     ADD COLUMN source text NOT NULL DEFAULT 'app';      -- constant default: no table rewrite
CREATE UNIQUE INDEX contacts_external_idx ON contacts (tenant_id, source, external_id) WHERE external_id IS NOT NULL;
```

## 9. Deep dives

### 9.1 A staged pipeline: why stage, then preview, then commit

| Option | Pros | Cons |
|---|---|---|
| Parse and write row by row inside the request | trivial | times out at 100k rows; a crash leaves an unknown half; no preview |
| Parse and write in one background pass | no staging storage | no preview (the user finds out afterwards); errors found in row 9M after 8M are already written |
| **Stage → classify → preview → commit chunks** | errors found before any real write; preview is a cheap SQL join; commit can resume from a cursor | staging storage (~0.84 TB) and a second pass over the data |

**Choice and why.** Staging. Validation (types, lengths, required fields, known references) and
normalisation (trim, lowercase emails, E.164 phones) happen once, while streaming into staging; row
errors are stored on the staged row. The preview is then **set-based**: one `UPDATE … FROM` joins
staging to the target on `(tenant_id, source, external_id)` and sets each `verdict` (create / update /
same), and the counts are `GROUP BY verdict`. Nothing in the real tables is touched.

The preview is a *forecast, not a promise*: between preview and commit, people keep editing. So commit
re-decides every row at write time (9.2) and the final report shows where it differed from the forecast,
the same stance as the seed's "the save has the last word". Two more rules: **fail fast on systematic
errors** (if more than 20% of the first 10,000 rows are invalid the mapping is wrong; stop and say so
instead of staging 10M bad rows), and **expire** (a previewed run nobody commits is dropped after 7 days
along with its staging rows).

### 9.2 Idempotent upsert: external id plus content hash

Re-running a file, a crash in the middle of a chunk, and two people clicking *Commit* must all be harmless.
The key is the source system's `external_id`, unique per `(tenant, source)`; the second ingredient is a
`content_hash` of the *normalised* fields, so "same" is a cheap comparison, not a field-by-field diff.

```sql
-- one chunk, one transaction (staging table name comes from import_runs.staging_table)
WITH chunk AS (
  SELECT * FROM import_rows_20261111 WHERE run_id = $1 AND row_no > $2 AND row_no <= $3 AND verdict <> 'error'
), up AS (
  INSERT INTO contacts AS c (tenant_id, source, external_id, name, email, content_hash)
  SELECT $4, 'import', external_id, data->>'name', data->>'email', content_hash FROM chunk
  ON CONFLICT (tenant_id, source, external_id) WHERE external_id IS NOT NULL
  DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, content_hash = EXCLUDED.content_hash
  WHERE c.content_hash IS DISTINCT FROM EXCLUDED.content_hash          -- same content: no write, no WAL, no trigger
  RETURNING c.id, c.external_id, (xmax = 0) AS inserted                -- idiom: xmax = 0 means a fresh insert
)
INSERT INTO import_items (run_id, entity, external_id, entity_id, action, wrote_hash)
SELECT $1, 'contact', u.external_id, u.id, CASE WHEN u.inserted THEN 'created' ELSE 'updated' END,
       (SELECT content_hash FROM chunk k WHERE k.external_id = u.external_id) FROM up u;
UPDATE import_runs SET cursor_row = $3 WHERE id = $1;                   -- same transaction: progress and effect together
```

Mode changes the `DO UPDATE`: `create_only` is `DO NOTHING`; `fill` writes `COALESCE(NULLIF(c.name,''), EXCLUDED.name)`
per field; `overwrite` replaces. Whatever the mode, **an empty or missing cell never clears a stored value**
(the seed's rule, learned because an LLM writing `<answer/>` for "I don't know" must not wipe a page); clearing needs an explicit sentinel.

The hash must be canonical or "same" never matches. Length-prefix every field so `("ab","c")` differs from `("a","bc")`:

```go
func contentHash(c Contact) [32]byte {
	h := sha256.New()
	for _, f := range []string{c.Name, strings.ToLower(c.Email), c.Phone, c.Notes} { // fixed order, normalised
		fmt.Fprintf(h, "%d:%s|", len(f), f)
	}
	return [32]byte(h.Sum(nil)) // slice-to-array conversion, Go 1.20+
}
```

Matching by something other than an id is harder (the seed matches a provider customer by linked id, then
email, then phone digits): *one match links, several is "needs a look", none creates*. Ambiguous rows are
decided by the user during preview (`PUT …/decisions`) and the preview is re-run before commit. Duplicate
`external_id`s *inside the file* are detected in staging (last wins, or both flagged: choose, document, show).

### 9.3 Undo: record what you did, spare what changed

| Option | Pros | Cons |
|---|---|---|
| No undo; "restore from backup" | nothing to build | all-or-nothing, loses everyone's later work |
| **Run id + the hash written + before-image for updates** | exact, set-based "untouched since?" check; reverts updates too | before-images double storage for updates |
| Soft-revert flag on every row (`import_run_id`, hidden when undone) | instant, no deletes | every query must filter; updated rows have no previous values |

**Choice and why.** `import_items` records every *created* and *updated* row with the hash the import wrote
(`wrote_hash`) and, for updates, the previous values (`before`). Undo walks the run in **dependency order,
children first** (invoices, then orders, then clients; the seed's order) and, per row:

- *created*: delete (or archive, per [24](24-data-lifecycle.md)) **if `content_hash = wrote_hash`** and nothing references it;
- *updated*: restore `before` **if `content_hash = wrote_hash`**;
- otherwise **spare it and say why** ("changed since the import", "an order of theirs stays"). A spared child spares its parents.

The seed asks the audit log "edited since?" per row; at 10M rows that is 10M lookups. The hash makes it one
set-based `UPDATE … FROM import_items WHERE content_hash = wrote_hash`. Undo itself is a chunked, idempotent job with an
`undoing` state, runs inside the 30-day window, and cannot be undone again (import again instead).

**What undo cannot undo:** side effects (emails, webhooks, search-index documents, caches: hence *imports fire no
events*, and a re-index runs after commit and after undo); **sequence numbers** consumed by the rows (customer-facing
numbers have gaps, accepted and documented in the seed's ADR 0006, see [20](20-document-numbering.md)); and anything a
person did to the rows afterwards, which is exactly what is spared.

### 9.4 Fairness and backpressure

The scheduling unit is a **chunk**, not an import. Chunks are dispatched like [15](15-job-scheduler.md)'s due jobs
(`FOR UPDATE SKIP LOCKED`), but the pick is *fair*: next chunk from the tenant with the fewest chunks currently running, then oldest.

| Knob | Setting | Why |
|---|---|---|
| Rows per chunk | 5,000 (aim for a transaction under ~1 s) | long transactions hold locks, bloat tables and delay vacuum; tiny ones waste round trips |
| Chunks in flight per tenant | 1 default, 4 for large plans | a 10M-row import is 2,000 chunks; at 4 in parallel ≈ 2,000 ÷ 4 × ~1 s ≈ 8 min of wall time |
| Global write budget | ~8k rows/s per shard (measure it) | leaves headroom for the product's own traffic |
| Adaptive throttle | pause 2× when replica lag > 5 s or p99 write latency > 2× baseline; resume gradually | AIMD: the importer is the *first* thing to slow down |

Staging uses `COPY` (not row inserts) into an UNLOGGED table: 10× the rows/s of single inserts and no WAL for data
we can regenerate. **Queue visibility** matters as much as speed: show *"queued, 3 imports ahead"* and an ETA computed
from the measured rate; users who see nothing click *Commit* again.

### 9.5 Hostile and malformed input

| Threat | Defence |
|---|---|
| Gigabyte lines, million columns, NUL bytes | caps on file size, line length (1 MB), columns (200), field length; reject early, stream, never load the file |
| Zip/gzip bomb | cap *decompressed* bytes and the ratio |
| XML: external entities (XXE), entity-expansion "billion laughs", deep nesting | no DTD processing, no external fetches, depth cap. Go's `encoding/xml` does not fetch external entities or process DTDs; if you bind libxml2, disable both explicitly |
| **CSV formula injection on the way out** | the error report and exports contain attacker-controlled text: a cell starting `=`, `+`, `-`, `@`, tab or CR is executed by Excel. Prefix such cells with `'`, or tab-escape them, in *every* CSV we generate |
| Cross-tenant writes | `tenant_id` from the session only; mapping cannot name another tenant's reference ids (validate references inside the tenant) |
| Malformed rows | row error with line number and reason, not a failed file; a file that is not an import file at all (wrong header, not XML) is refused whole |

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the commit write budget (120k rows/s demanded). Shard imports by tenant to the tenant's own shard, give each shard its own scheduler, and stop treating staging as one database (a staging store per shard, or ClickHouse/Parquet-on-S3 for validation and preview).
- **Worker dies mid-chunk:** the transaction rolls back; the lease expires; another worker re-runs the same chunk, and `ON CONFLICT` plus the cursor in the same transaction make a repeat harmless.
- **Replica lag spikes:** the throttle slows chunks; if lag keeps growing, pause the whole queue (a global switch, see [51](51-runtime-settings.md)).
- **Two people press Commit:** the partial unique index refuses the second; with the same idempotency key they get the same `202`.
- **Single points of failure:** the scheduler (stateless, N copies, row locks), the staging store (re-creatable from the raw file, so lose it and re-parse), Postgres (as for everything else).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Stage, preview, then commit | one streaming pass | errors before writes; a real preview; resumable | staging storage; two passes |
| `external_id` + content hash | field-by-field diff | cheap "same", exact "unchanged since", idempotent re-runs | a canonical-hash discipline |
| Undo = delete/restore guarded by hash | restore from backup | partial, safe undo that spares later edits | before-image storage; undo only for 30 days |
| Chunked commits of 5,000 rows | one huge transaction | resumable, short locks, bounded WAL bursts | an import is not atomic as a whole |
| Fair chunk scheduler + per-tenant caps | FIFO by import | small tenants never wait behind a 10M import | more scheduler logic |

## 12. Check yourself

1. Why stage the file into a table before writing to the real tables?
2. A worker crashes after writing 3,000 of a chunk's 5,000 rows. What is on disk, and how does the retry stay correct?
3. How does the importer decide a row is "same", and why must the hash be canonical?
4. What does Undo do to a contact that a user edited after the import? And to its imported orders?
5. A tenant starts a 10M-row import; 90 s later another tenant starts a 500-row import. What stops the second from waiting hours?

<details><summary>Answers</summary>

1. To find errors and show a preview *before* any real write: validation, normalisation and classification are done once on staging, the preview is a set-based join, and a bad row 9M into the file does not leave 8M rows already applied. It also makes commit resumable from a cursor and keeps the parse step separate from the (scarce) write step.
2. Nothing: the chunk is one transaction, so a crash rolls back all 3,000 rows and the cursor stays where it was. The lease expires, another worker re-runs the chunk, and the upsert is idempotent (`ON CONFLICT` on the external id, `WHERE content_hash IS DISTINCT`), so even rows that had been written would not duplicate.
3. By comparing `content_hash` of the normalised incoming fields with the stored hash: equal means no write, no WAL, no trigger, no `updated_at` bump. If normalisation differed between runs (case, whitespace, field order, no length prefixes) equal data would hash differently and every re-run would rewrite everything.
4. It spares the contact ("changed since the import": its hash no longer equals `wrote_hash`) and keeps it, even though the run created it. Undo goes children first, so the contact's imported orders that nobody touched are still removed; an imported order someone *did* change is spared, and a spared child also keeps its parent contact alive. The report lists every spared row with its reason.
5. Chunks, not imports, are the scheduling unit, and the dispatcher picks the next chunk from the tenant with the fewest chunks in flight. The big import advances 1-4 chunks at a time; the 500-row import is one chunk and runs next. Per-tenant caps stop the big one from taking every worker slot.

</details>

## 13. In Go

- Related lessons: [44 Background jobs](../../44-background-jobs-queues.md) (chunk jobs, retries), [15 Sync & context](../../15-sync-context.md) (worker pools with `errgroup`, cancellation on shutdown), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED`, upserts, partial unique indexes), [55 Data pipelines](../../55-data-pipelines.md) (streaming stages), [57 Web security](../../57-web-security.md) (hostile input), [41 API design](../../41-api-design-evolution.md) (`Idempotency-Key`), [19 Stdlib tour](../../19-stdlib-tour.md) (`encoding/csv`, `encoding/xml`, `crypto/sha256`).
- Stream, never `ReadAll`: `csv.NewReader(bufio.NewReaderSize(r, 1<<20))` with `FieldsPerRecord = -1` and your own validation; treat a `*csv.ParseError` as *that row's* error and keep reading. `xml.NewDecoder` with a token loop and a depth counter for XML.
- Load staging with `pgx.CopyFrom`; run each chunk with `pgx.BeginFunc` so the cursor update is in the same transaction.
- Packages: `pgx/v5`, `golang.org/x/sync/errgroup`, `aws-sdk-go-v2` (presigned PUT), `golang.org/x/text/unicode/norm` (NFC).

## 14. Extensions

- **Connector imports** (pull from a provider's API with a watermark, then the same staging pipeline): the seed's `CustomerLister` / `InvoiceLister` capabilities, and [18](18-inbound-webhook-receiver.md) for keeping them in sync afterwards.
- **Column mapping with suggestions:** an LLM proposes the mapping from the header and five sample rows (through the gateway in [36](36-llm-gateway.md)); the user confirms.
- **Merge-on-import:** match by email/phone fuzzy rules and hand the ambiguous rows to the dedupe flow ([25](25-contact-dedupe-merge.md)).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API + fair scheduler + commit workers (Go, stateless) | 500 imports in flight, 2,000 a day; commit ≈ 2 chunk transactions/s at the ~8k rows/s budget (§4) | nothing: they wait on Postgres | dispatch 2–8 chunks/s × ~5 ms of Go CPU + status polls ≈ 0.1 vCPU | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | tiny, three for availability. Commit workers mostly wait on the database, so more of them would only add write contention |
| Parse and stage workers (Go) | 11.6k rows/s at the 5× peak; ~50k rows/s per worker; a 10M-row file ≈ 200 s (§4) | CPU | average demand 11.6k ÷ 50k = 0.23 vCPU; sized for bursts instead: 6 large files parsing at once at 1 vCPU each = 6 vCPU; N = ceil(6 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(2.8) = 3 | `c7g.xlarge` (4 vCPU / 8 GiB); a Spot fit (idempotent, staging is re-creatable) | 3, one per zone | CSV/XML parsing, hashing and normalising are CPU; the stream holds ~50 MB of buffers per worker, so `c`. Reading a 2 GB object and `COPY`ing ~6 GB into staging at ~30 MB/s is nowhere near the network limit |
| Postgres (target tables, `import_*`, UNLOGGED staging) | ~8k rows/s commit budget (demand 11.6k) + classification joins at up to 11.6k lookups/s; staging 0.84 TB (7 d) + `import_items` 360 GB (30 d) (§4) | IOPS, then memory | assume ~1.5 IOPS per row written (a chunk is one tenant's key range) ⇒ 8k × 1.5 = 12k, plus ~1 read IOPS per classified row ⇒ up to 11.6k: ≈ 24k at the Monday peak vs gp3's 16k cap. CPU 8k × 60 µs + 11.6k × 30 µs ≈ 0.8 vCPU of 16. Hot unique-key index pages of the importing tenants ≈ 60 GB (assumption). Disk 1.2 TB + product tables ⇒ 2,000 GB | `db.r7g.4xlarge` (16 vCPU / 128 GiB), RDS Multi-AZ, `gp3` 2,000 GB at 16,000 IOPS | 1 primary + synchronous standby | RAM for the index pages, IOPS as the real ceiling. Buying `io2` for 24k IOPS is 24k × $0.065 + 2 TB × $0.125 ≈ $1,810 per copy vs $225 on gp3, for a two-hour Monday peak; the AIMD throttle stretches ETAs instead, and sustained demand means sharding by tenant (§10) |

Raw uploads on S3 (30 days, 1.2 TB): nothing to size, and the presigned PUT keeps 2 GB files off our nodes. The honest picture: the parsing hardware is almost free; the shared write budget on Postgres is the product.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API, scheduler, commit workers: 3 × `c7g.large` (3 × $53) | $160 |
| Parse workers: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Postgres: `db.r7g.4xlarge` ($620 × 2 managed), Multi-AZ doubles | $2,480 |
| gp3 2,000 GB + 13k extra IOPS (= $225, ×2 for the standby) | $450 |
| S3 raw uploads: 1.2 TB × $0.023 | $30 |
| ALB (small API traffic) | $20 |
| **Total** | **≈ $3,460** (≈ $2,420 with a 1-year Savings Plan on compute), about $0.58 per million rows imported (6B a month) |

- **Biggest line and why:** Postgres with its disk (≈ 85 %), because commit is the scarce, shared resource (indexed writes, WAL, replicas) while parse and orchestration are cheap. The lever is the throttle and chunk scheduling, not a bigger node.
- **What changes at 10×:** 120k rows/s demanded against ~8k per shard ⇒ shard imports by tenant onto ~10 clusters of this shape (≈ 10 × $2,930 ≈ $29k); each shard stays within gp3's 16k IOPS, which is why sharding beats `io2`. Parsing is 120k × 20 µs ≈ 2.4 vCPU on average, still burst-sized: double to 6 × `c7g.xlarge`. Staging moves off Postgres to a store per shard or Parquet on S3, as §10 says.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The scarce, shared resource is commits into the product's own tables: 200M rows a day (~2.3k/s on average, ~11.6k/s at the Monday peak) against a budget of ~8k rows/s per write shard (an assumption to measure), in chunks of 5,000 rows, one transaction each (sections 4, 9.4).
- Each chunk must be idempotent and carry its own progress: upsert by `(tenant_id, source, external_id)`, skip a row whose `content_hash` is unchanged, record created versus updated per row, and advance `cursor_row` in the same transaction (9.2).
- Three data sets of different value: raw uploads (S3, immutable, ~1.2 TB), staging rows (0.84 TB for 7 days, 120 GB/day, re-creatable from the file) and the durable `import_runs` / `import_items` (~360 GB for 30 days) beside the target tables.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | one statement per chunk: `INSERT … ON CONFLICT (…) WHERE external_id IS NOT NULL DO UPDATE … WHERE c.content_hash IS DISTINCT FROM EXCLUDED.content_hash RETURNING …, (xmax = 0)` inside a data-modifying CTE that also feeds `import_items`; `UNLOGGED` staging loaded by `COPY` keeps 120 GB/day of re-creatable rows out of the WAL; the partial unique index `import_runs_one_active` allows one active write per tenant; `jsonb` staging rows and set-based `UPDATE … FROM` give the preview |
| MySQL (RDS / Aurora) | ✔ works, with several workarounds | no unlogged table (staging is redo-logged and binlogged), no `RETURNING` or `xmax`, no data-modifying CTE (the one-statement chunk becomes three or four), and one partial unique index becomes a generated-column index. The idempotency key itself is portable: a plain unique index allows many NULLs, so the `WHERE external_id IS NOT NULL` is not needed |
| S3 + Parquet / ClickHouse | ✖ now, ✔ for staging at 10× | the preview is a SQL join of staging against the live target table, which needs both in one database; at 120k rows/s of demand (section 10) staging moves to a store per shard or to Parquet, and only classified chunks reach the OLTP database |
| DynamoDB | ✖ poor fit | a chunk is 5,000 rows in one transaction with a cursor; transactions stop at 100 items, so chunk atomicity breaks into 50 transactions, and there is no set-based `GROUP BY verdict` or join to the target for the preview |

**Pick:** PostgreSQL on RDS Multi-AZ, `db.r7g.4xlarge` with `gp3` 2,000 GB at 16,000 IOPS (section 15), the same database as the target tables, because the commit has to be a transaction into them. Three things decide it: the one-statement idempotent chunk (conflict target on a partial unique index, `IS DISTINCT FROM`, `RETURNING`) with the cursor update in the same transaction; `UNLOGGED` staging plus `COPY`, where a crash empties staging by design and the S3 file re-creates it (9.1, section 10); and the partial unique index with `SKIP LOCKED` row locks for the one-active-write rule and the fair scheduler. System of record: the target tables and `import_runs` / `import_items` in Postgres, plus the raw files on S3. Staging is derived and disposable.

**If it were MySQL instead:** the pipeline survives, but this is the lesson that leans on Postgres most.
- Staging: `UNLOGGED` has no equivalent for a shared InnoDB table, so 120 GB a day of staging rows would be redo-logged, binlogged and replicated. Either accept that, or stage on a separate cheap database or S3 (the 10× design of section 10). `COPY` becomes `LOAD DATA` or multi-row inserts.
- Chunk: no data-modifying CTE, `RETURNING` or `xmax = 0`. Run `SELECT … FOR UPDATE` on the existing rows by key (it also gives the before-image for `import_items.before`, 9.3), then `INSERT … ON DUPLICATE KEY UPDATE` with per-column `IF(content_hash <=> new.content_hash, col, new.col)` (there is no `WHERE` on the update branch), then `INSERT INTO import_items` from the read, all with the cursor update: 3–4 statements instead of one.
- `import_runs_one_active` becomes a generated column (`tenant_id` while the status is queued, running or undoing, else NULL) with a unique index; chunks run at `READ COMMITTED`, since upserts on a unique key take next-key locks under the default `REPEATABLE READ` and parallel chunks of one tenant can lock-wait or deadlock.

**What would flip the choice:** the number that matters is the commit budget, ~8k rows/s per primary. Sustained demand above it is answered by sharding imports by tenant onto the tenant's own shard (~10 clusters at 10×, sections 10 and 15), not by another engine, and past ~24k IOPS a shard is cheaper than `io2`. Staging leaves Postgres (S3 and Parquet, or a store per shard) once the 0.84 TB and the preview joins become the load. A move of the target tables to a store without multi-row transactions would force the chunk-atomic commit to be redesigned.
