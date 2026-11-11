# 20 · Per-Tenant Document Numbering

`🟢 easy` · *Topics:* key vs public id vs human number, sequence vs counter row vs allocation blocks, the hot-row ceiling, gap-tolerant vs gap-free (legal) series, what a number leaks, numbering a live table · *Builds on:* [04 Unique ID Generator](04-unique-id-generator.md) (ID sources, range leasing), [06 View & Like Counter](06-view-like-counter.md) (hot rows)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [19 Moderated Submissions (Reviews, Photos, Anonymous Feedback)](19-moderated-submissions.md) · Next: [21 Presence & Soft Edit Locks](21-presence-soft-locks.md)

---

> 🌱 **Seed (honeydolist-site):** every request and order has three identifiers (ADR 0006 and its 2026-10-09
> amendment): an internal `bigint` key that never leaves the database, a random `public_id` (`q_`/`r_`/`o_` + 12
> base62 chars) that addresses the record in URLs, and a human `number` (*Order #2051*, migration 0051, PLAN step
> 25.5) from a sequence of its own starting at 1001, shown to customers because it is read out on the phone.
> Existing rows were numbered in `created_at, id` order with the `updated_at` triggers off; a rolled-back insert
> leaves a gap; invoices carry the invoicing provider's numbers. The docs' next step, once several businesses share
> the app: a counter row per business, not a sequence each. One business, one sequence, tens of orders a day.
> **Here, at scale:** a multi-tenant SaaS with 1M tenants, ~3k new documents/s on average and 10k/s at peak, one
> tenant at 1k/s on a sale day; each tenant sees its own order numbers, and its invoices form a legally consecutive series.

## 1. The task

> "Our platform hosts a million shops. Each shop's orders and invoices need a human-friendly number that starts
> where the shop says, counts up inside that shop, and never repeats. Customers read it over the phone."

Ids are about machines; numbers are about people. The interesting part is that "never repeats" is easy,
"has no gaps" is expensive, and the two are different promises for different documents.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Per tenant or global? | A global sequence leaks the platform's volume to every tenant and cannot start at 1001 for each. | Per tenant, per document type ("series"). |
| Who sees it? | If customers see it, it is a product feature, not a key. | Customers, in emails and on the phone. |
| Gaps allowed? | The expensive question. | Orders and quotes: yes. Invoices: no, the series must be consecutive (voids are kept, never deleted). |
| Starting number and format? | Shops migrate from other systems. | Tenant sets the start (default 1001), may only raise it; optional prefix and year reset for invoices. |
| Must numbers follow creation order? | Decides blocks vs strict counters. | Orders: roughly. Invoices: strictly, number order = issue order. |
| How do we address a record in a URL or API? | The number must not be the address. | By `public_id`; the number is a search key. |
| Scale? | Counter design depends on the hottest tenant, not the average. | 1M tenants, 3k/s average, 10k/s peak, hottest tenant 1k/s. |

## 3. Requirements

**Functional**
- Assign each new order a per-tenant integer `number` at creation; never reused.
- Assign each invoice a number from a gap-free per-tenant series **when it is issued** (drafts have none).
- Find a document by number inside a tenant (`2051` or `#2051`); address it by `public_id`.
- Tenant can set the next number upward only, and the invoice prefix/year format.
- Void an invoice without freeing its number.

**Non-functional**
- Order creation p99 under 50 ms, independent of other tenants' load.
- A hot tenant must not slow anyone else; its own throughput target is 1k orders/s.
- No duplicate numbers within a (tenant, series), ever (a database constraint, not hope).
- Invoice series: no holes unless explicitly voided.

**Out of scope**
- Global ordering across tenants, tax-law specifics per country, PDF rendering ([task 30](30-invoicing.md)).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| New documents | ~260M/day, 3k/s avg, 10k/s peak | 3,000 × 86,400 ≈ 259M; peak ≈ 3.3× |
| Shards | 32 Postgres shards by `tenant_id` | assumed; a counter lives in its tenant's shard, in the same database as the orders |
| Peak per shard | ~310 numbers/s | 10,000 / 32 |
| Counter rows | ~3M, ~300 MB | 1M tenants × ~3 series × ~100 B; fits in RAM |
| Hot tenant | 1,000/s on one counter row | 10% of platform peak |
| Row-lock ceiling, counter in the order's transaction | ~330/s per tenant | one writer at a time while the lock is held: 1 / 3 ms |
| Counter updates with blocks of 100 | 10/s for the hot tenant | 1,000 / 100 |
| `public_id`, 12 base62 chars | 62¹² ≈ 3.2×10²¹ ≈ 2⁷¹ | `log₂ 62 ≈ 5.95`, × 12 |
| Rows per year | ~95B | 259M × 365; all of them need a distinct `public_id` if uniqueness is global |

What the numbers tell us: the counter table is tiny, so storage is not the problem. **Contention on one
row is**: the lock is held until commit, so a tenant's rate is capped by its transaction time. The average tenant
(0.003 orders/s) never notices; the hottest one cannot use the naive design. Also, 95B random 71-bit ids
already makes collisions *expected* (§9.4).

## 5. Plan

1. Separate the three identifiers and say what each is for.
2. Pick the counter mechanism: sequence, counter row, or blocks, by the hottest tenant.
3. Treat gap-tolerant (orders) and gap-free (invoices) as two designs.
4. Decide what the number reveals and keep it out of addresses.
5. Show how to introduce numbering to a live table.

## 6. High-level design

```
 client ─► LB ─► Orders API (stateless, many)
                    │
        ┌───────────┴──────────────────────────────────────────┐
        │ ORDER (gap-tolerant)                                  │ INVOICE ISSUE (gap-free)
        │  BEGIN                                                │  BEGIN
        │  public_id = random()                                 │  UPDATE number_counters … +1 RETURNING  ← row lock
        │  n = alloc(tenant,'order')                            │  UPDATE invoices SET status='issued', number=n
        │      └ in-process block [lo..hi] (rare refill:        │  COMMIT                                 ← lock released
        │        UPDATE number_counters … + N, own tiny txn)    │
        │  INSERT order (…, number = n)                         │
        │  COMMIT                                               │
        └───────────────────────────────────────────────────────┘
          both inside the tenant's Postgres shard (number_counters, orders, invoices)
```

One order write: take a number from the tenant's in-memory block (a refill is one single-row `UPDATE`
in its own short transaction), insert the order with a random `public_id` and that number, commit. One invoice
issue: increment the counter *inside* the issuing transaction, so a rollback un-increments it.

## 7. API

```
POST /v1/orders                       → 201 { "id": "o_7Kq2XvB9mT3e", "number": 2051, ... }
GET  /v1/orders/{public_id}           → 200 order                       (the address)
GET  /v1/orders?number=2051           → 200 [order]                     (the search; tenant-scoped, authenticated)
POST /v1/invoices/{public_id}/issue   → 200 { "number": "INV-2026-000123", "series": "invoice:2026" } | 409 already issued
POST /v1/invoices/{public_id}/void    → 200   (number stays used; a void row is visible in the series)
PUT  /v1/numbering/{series}           { "next": 5000 } → 200 | 422 if lower than current
```

The tenant comes from the credential or hostname, never from the body. A number is looked up only within
that tenant, so `?number=` cannot probe other tenants.

## 8. Data model

```sql
CREATE TABLE number_counters (
    tenant_id  bigint NOT NULL,
    series     text   NOT NULL,           -- 'order', 'quote', 'invoice:2026'
    last_value bigint NOT NULL,           -- the last number handed out
    PRIMARY KEY (tenant_id, series)
) WITH (fillfactor = 50);                 -- room for HOT updates: this row changes constantly

CREATE TABLE orders (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,   -- internal, never exposed
    tenant_id  bigint NOT NULL,
    public_id  text   NOT NULL,                                   -- 'o_' + base62, the address
    number     bigint NOT NULL,                                   -- the human number
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, number),
    UNIQUE (tenant_id, public_id)
);

CREATE TABLE invoices (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id  bigint NOT NULL,
    public_id  text   NOT NULL,
    status     text   NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'void')),
    series     text,
    number     bigint,
    issued_at  timestamptz,
    CHECK ((status = 'draft') = (number IS NULL)),   -- a draft has no number; issued and void do
    UNIQUE (tenant_id, public_id)
);
CREATE UNIQUE INDEX invoices_number_key ON invoices (tenant_id, series, number) WHERE number IS NOT NULL;
```

The uniqueness constraints are the real guarantee; the counter is only the way to find a free value
cheaply. Gap-tolerant allocation (first number is the tenant's start, e.g. 1001; `$3` is the start):

```sql
INSERT INTO number_counters (tenant_id, series, last_value) VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, series) DO UPDATE SET last_value = number_counters.last_value + 1
RETURNING last_value;
```

## 9. Deep dives

### 9.1 Where the counter lives

| Option | Pros | Cons |
|---|---|---|
| One Postgres sequence per table (the seed's choice) | `nextval` is non-transactional and very fast; no lock contention | global: every tenant sees the platform's volume and cannot start at 1001; gaps on rollback |
| **A sequence per tenant** | native | 1M tenants × 3 series = 3M sequence relations: catalog bloat, a DDL statement on every signup, slow `pg_dump`. The docs rule it out too: a counter row per business, not a sequence each |
| **Counter row, incremented in the order's transaction** | consecutive in commit order; rollback un-increments; trivial | the row lock is held until commit: serialises a tenant's writers (§9.2) |
| **Counter row + in-process blocks** | refill is one update per N orders; no lock on the hot path | a crash wastes the rest of a block; numbers interleave across API instances |
| Redis `INCR` | fastest | not in the order's transaction: crash between `INCR` and commit leaves gaps, Redis loss can repeat numbers (the unique constraint catches it, but the write fails) |

**Choice and why.** A **counter table** for both series. For orders, use **blocks**, because the hottest
tenant decides the design. For invoices, use the **in-transaction counter**: the series volume is lower (a
whale issues dozens a second, not a thousand) and gap-free is the whole point. A Postgres sequence is still the
right answer for the internal key: it is not shown to anyone.

### 9.2 The hot tenant: row locks set a ceiling

`UPDATE number_counters …` locks the row until the *transaction* ends, not until the statement ends. Another
writer for the same (tenant, series) waits. So the ceiling for one tenant is about **1 / (time from the update to
commit)**: 3 ms gives ~330 numbers/s, 0.5 ms (update as the last statement, fast commit) gives ~2,000/s. The 1k/s
hot tenant sits right on that edge, and a slow downstream call inside the transaction would collapse it.

| Remedy | Effect | Cost |
|---|---|---|
| Number **last** in the transaction | shortens lock hold | helps, does not remove the ceiling |
| **Allocation blocks** (take N, hand out from memory) | counter updates drop N-fold: 1,000/s → 10/s with N = 100 | gaps after a crash or deploy; numbers not in creation order across instances |
| **Adaptive block size** | slow tenants get N = 1, no visible jumps; busy tenants get up to 1,000 | a little code and a rate estimate |
| Single-writer queue per tenant | strictly ordered and gap-free at high rate | a component per hot tenant (medium-level) |

The adaptive block is what makes blocks acceptable. A fixed N = 100 would make a shop with three orders a day see
`#1001, #1101, #1201` after each deploy. Size the block as `clamp(rate × 2 s, 1, 1000)`:

```go
type block struct {
	mu       sync.Mutex
	next, hi int64 // next number to hand out, last number of the block
}

func (a *Allocator) Next(ctx context.Context, tenant int64, series string) (int64, error) {
	b := a.block(tenant, series) // short map lock; creates an empty block
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.next == 0 || b.next > b.hi {
		n := a.sizeFor(tenant, series) // adaptive: 1 for slow tenants, up to 1000
		err := a.db.QueryRow(ctx, `UPDATE number_counters SET last_value = last_value + $3
			WHERE tenant_id = $1 AND series = $2 RETURNING last_value - $3 + 1, last_value`,
			tenant, series, n).Scan(&b.next, &b.hi)
		if err != nil {
			return 0, err
		}
	}
	b.next++
	return b.next - 1, nil
}
```

The refill is its own autocommit statement: it never joins the order's transaction or holds the counter lock
across other work. Keep `UNIQUE (tenant_id, number)` anyway: a bug here becomes a failed insert, not two orders
sharing a number.

### 9.3 Gap-tolerant vs gap-free

Gaps appear whenever a number is taken in one transaction and used in another, or the using transaction rolls
back: `nextval` is never rolled back, a block's unused tail is lost on restart, a double-click's `ON CONFLICT DO
NOTHING` insert burns a number. The seed accepts this for orders.

| Document | Gaps? | Why | Mechanism |
|---|---|---|---|
| Order, quote, ticket | fine | customers need *distinct*, not *consecutive* | blocks, number at creation |
| Invoice, credit note | **no**, in many jurisdictions | tax rules often expect an unbroken series per issuer, or an explanation for every hole (check the actual rule with an accountant, it varies) | in-transaction counter, number at **issue** |

Gap-free needs three things:

1. **The counter increment in the same transaction as the use**, so a rollback undoes it. A later issuer waits
   on the row lock, so commit order equals number order. Run it under `READ COMMITTED`; under `REPEATABLE READ`
   the waiter gets a serialization error instead.
2. **Number at issue, not at creation.** A draft is edited, abandoned, deleted. If drafts took numbers, every
   deleted draft would be a hole. The `CHECK ((status = 'draft') = (number IS NULL))` enforces it.
3. **Void, never delete.** A mistaken issued invoice becomes `void` and keeps its number; the correction is a
   credit note with its own number. A hole is then an incident, detectable by a nightly query for
   `max(number) - count(*) > 0` per series.

The cost is serialisation per series (one issuer at a time per tenant), so keep the transaction short: compute
totals and render the PDF *before*, in draft; the issue transaction only stamps the number. If one whale is too slow,
split into series per location or register (usually legal) rather than relaxing "consecutive". The seed's shortcut
is the cheapest way to be correct: the invoicing provider owns the series, and you need not be the system of record.

### 9.4 What the number leaks, and why the URL uses the public id

Two order numbers reveal the shop's volume: #2051 now and #2090 a week later means ~39 orders a week (the "German
tank problem"). The seed accepts that on purpose. A tenant who would rather not show it can start at 1001 instead of 1
or advance by random steps of 1–5 (gaps are legal for orders). A *global* sequence would leak the whole
platform's volume to everyone, another reason the counter is per tenant.

An enumerable number must never be an address: `GET /orders/2051`, then `2052`, is IDOR (insecure direct object
reference) waiting for one missing permission check. Addresses are random `public_id`s, and every lookup checks the tenant.

Random ids have their own scale issue. 12 base62 chars is ~2⁷¹, and at 95B rows a year the birthday bound
`n² / 2N` = (9.5×10¹⁰)² / (2 × 3.2×10²¹) ≈ **1.4 expected collisions a year** under *global* uniqueness. Options:
make the id unique *per tenant* (`UNIQUE (tenant_id, public_id)`, as in §8: a tenant has at most a few billion
rows, so the chance is negligible; it needs the tenant in the URL or hostname), lengthen to 16 chars (~2⁹⁵), or
generate in the application and **retry on a unique violation**. Do the retry in any case; it costs nothing.

### 9.5 Introducing numbers on a live table

The seed's migration 0051 is right for a small table: add a nullable column, number existing rows in `created_at, id`
order, `setval`, then default, `NOT NULL`, `UNIQUE`. Two traps it notes: `ADD COLUMN … DEFAULT nextval(…)` is a
*volatile* default, so Postgres rewrites the table and numbers rows in physical order, not creation order; and
numbering a row is not editing it, so `updated_at` (the optimistic-concurrency version) must not move, or every open
page says "someone else changed this". The seed switches the two triggers off inside the migration transaction.
**At scale that is unsafe**: `DISABLE TRIGGER` hits every session, so live writes would skip their version bump:

| Step | How |
|---|---|
| 1. Expand | `ADD COLUMN number bigint` (instant, nullable); change the version trigger to `WHEN (OLD.number IS NOT DISTINCT FROM NEW.number)`, the same trick the seed uses for `assigned_to` |
| 2. Dual write | deploy code that numbers every new order from the counter |
| 3. Backfill, tenant by tenant | per batch of ~10k rows, in `(created_at, id)` order, lock the tenant's counter row, assign `last_value + row_number()`, advance the counter; small tenants take milliseconds, whales run off-peak |
| 4. Constrain | `CREATE UNIQUE INDEX CONCURRENTLY`, then `ADD CONSTRAINT … UNIQUE USING INDEX`; `CHECK (number IS NOT NULL) NOT VALID`, then `VALIDATE CONSTRAINT`, then `SET NOT NULL` |

Live orders created *during* a tenant's backfill take numbers from the same counter, so they may be numbered
before older rows still waiting in the backfill. Acceptable for orders ("distinct, roughly ordered"); for a
legal series, pause that tenant's issuing for the backfill window.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the hot tenant's counter row (100k/s platform peak means whales at 10k/s).
  Bigger blocks, then a per-tenant sharded allocator that hands each API instance its own sub-range (task 04's
  range leasing), accepting that numbers are no longer in creation order.
- **Counter row bloat:** a row updated 1,000×/s creates dead tuples fast. `fillfactor = 50` allows HOT updates;
  keep autovacuum aggressive on this table; blocks cut the update rate anyway.
- **Failures:** the tenant's shard down means that tenant cannot create orders (no global allocator exists, by
  design). An API instance crash loses its unused block tail, or a number taken before commit: a visible, harmless gap.
- **Counter restored from a backup older than the data:** the next insert violates `UNIQUE (tenant_id, number)`.
  Recover with `UPDATE number_counters SET last_value = (SELECT max(number) …)`; run that check after any restore.
- **Moving a tenant between shards:** the counter row moves with it; freeze writes for the cutover, or the new
  counter can start behind the data. A script creating orders in a loop burns numbers: rate limit ([03](03-rate-limiter.md)).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Counter table per (tenant, series) | one global sequence | per-tenant numbers, custom start, no cross-tenant leak | we own concurrency; one more table |
| Counter row, not sequence per tenant | 3M sequences | no catalog bloat, no DDL on signup | row-lock contention to manage |
| Blocks with adaptive size for orders | in-transaction counter | hot tenants at 1k/s, no lock on the hot path | gaps after crashes; creation order only roughly kept |
| In-transaction counter for invoices | blocks | consecutive series, rollback-safe | serialised issuing per series |
| Number at issue, void instead of delete | number at draft | no holes from abandoned drafts | drafts need another identifier |
| `public_id` as the address, number as a search key | the number in URLs | no enumeration, no IDOR | two identifiers to explain |
| Per-tenant uniqueness for `public_id` | global uniqueness | no birthday collisions at 10¹¹ rows | the tenant must be in the URL |

## 12. Check yourself

1. Why is `ALTER TABLE … ADD COLUMN number bigint DEFAULT nextval('s')` a bad way to number a live table?
2. Why not create one Postgres sequence per tenant?
3. A tenant creates 1,000 orders/s and the counter update is in the order's transaction, which lasts 3 ms. What happens, and what do you change?
4. Which documents may have gaps and which may not, and what mechanism gives each its guarantee?
5. A customer says "my order is #2051". What can a competitor learn from that number, and why must `/orders/2051` not exist as an address?

<details><summary>Answers</summary>

1. `nextval` is a volatile default, so Postgres rewrites the whole table under a heavy lock and numbers rows in
   physical order, not creation order. Add the column nullable, backfill in `created_at, id` order in batches, then
   add default, `NOT NULL`, `UNIQUE` the non-blocking way.
2. Each sequence is a database relation: 3M of them (1M tenants × ~3 series) bloat the catalog, make every signup
   a DDL statement, and slow dumps. A counter row per (tenant, series) is a few hundred bytes and needs no DDL.
3. The row lock lasts until commit, so that tenant's writers queue: the ceiling is 1 / 3 ms ≈ 330/s, a third of
   its rate, with growing latency. Take numbers in blocks (N = 100 turns 1,000/s into 10 counter updates/s, each in
   its own short transaction), size blocks adaptively, number last. Accept gaps after crashes.
4. Orders and quotes may have gaps: they need to be distinct, so blocks and sequences are fine. Invoices and
   credit notes often may not: the counter increment happens in the issuing transaction (rollback un-increments), numbers
   are assigned at issue not creation, and mistakes are voided, never deleted.
5. From two numbers and their dates, the order rate, so the volume (a start at 1001 or random steps softens it; it
   is a product choice). As an address it invites walking through 2052, 2053, …, and one missing authorisation check
   leaks every order. Addresses are random `public_id`s; the number is only a search key within the authenticated tenant.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (transactions, `RETURNING`, upserts), [59 Production DB patterns](../../59-production-database-patterns.md) (row locks, `NOT VALID` constraints, online migrations), [43 Authorization, RBAC & multi-tenancy](../../43-authorization-rbac-multitenancy.md) (tenant scoping on every query), [15 Sync & context](../../15-sync-context.md) (the per-key mutex in the allocator), [12 Errors](../../12-errors.md) (detecting a unique violation and retrying).
- Packages: `pgx` / `pgxpool` (`pgconn.PgError` code `23505` for unique violations), `crypto/rand` for `public_id`, `math/big` or a base62 encoder.
- Test it: 200 goroutines calling `Next` for one tenant must return 200 distinct numbers; `go test -race`; a table test for the invoice issue path with a forced rollback must leave `last_value` unchanged.

## 14. Extensions

- **Series that reset yearly** (`INV-2026-000123`): the year is part of the series key, created lazily on the first issue of January.
- **Check digits** on numbers read over the phone (Luhn or Damm), so "2051" vs "2015" is caught.
- **Per-location registers** for invoices, each its own consecutive series.
- **Importing history** from another system: collisions with the counter, and the "raise only" rule.
- **Strict order without gaps at high rate:** a single-writer allocator per hot tenant (medium: distributed task queue).
- **Audit the series:** a nightly job proving invoice numbers have no holes, feeding [task 22](22-audit-log-revisions.md).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Orders / Invoices API (Go, stateless) | 10k creates/s at peak, 3k/s average | CPU | 10,000 × 0.5 ms = 5 vCPU (JSON, random `public_id`, block allocation in memory, one insert). N = ceil(5 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(2.3) = 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | CPU work only; the allocator's blocks and the tenant-to-shard map (1M × ~50 B = 50 MB) live in memory, so `c`, not `m`; 42 % busy at peak, 63 % with a zone down. The numbering itself is a rounding error here; order reads (not in section 4) would be the next load |
| Postgres shards (`orders`, `invoices`, `number_counters`) | 32 shards, ≈ 310 creates/s each at peak (10,000 ÷ 32); the whale's shard ≈ 1,300/s (1,000 + ≈ 280); ≈ 95B rows/year | memory, then disk size | per shard: IOPS ≈ 310 × 2 = 620 + ~1,000 miss reads ≈ 1,600, under gp3's 3,000 included (whale's shard: 1,300 × 2 + 1,000 ≈ 3,600 ⇒ 6,000); CPU 310 × 0.5 ms = 0.16 vCPU of 4 (whale 0.65); counters 3M rows ≈ 300 MB in all (≈ 9 MB per shard); this lesson's slice of a document ≈ 200 B (72 B row + three indexes ≈ 100 B + page overhead, assumed) ⇒ 95B × 200 B = 19 TB/year ≈ 600 GB per shard | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ | 32 shards × (primary + standby) | memory-bound: the `(tenant, public_id)` and `(tenant, number)` indexes are hit at random, so the recent part must stay cached (`r`: 8 GiB per vCPU); CPU is nearly idle. 10k writes/s is only ~3–4 nodes' worth of durable writes, so the 32 shards come from data size and tenant isolation, not from throughput |

Counter rows need no tuning: with blocks of ~100 the whale updates its row 10 times a second (section 4), far under the row-lock ceiling of ~330/s. The invoice path (in-transaction counter) is the same hardware; its limit is a lock, not a node.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.xlarge` (3 × $105) | $320 |
| ALB: $16 + ~20 LCU × $0.008 × 730 | $130 |
| Postgres: 32 × `db.r7g.xlarge` (≈ $310 each, Multi-AZ doubles to $620) | $19,800 |
| gp3 at the end of year one: 19 TB ($1,520) + 3,000 extra IOPS on the whale's shard ($15), ×2 for the standbys | $3,070 |
| **Total** | **≈ $23,300** (≈ $16,300 with 1-year commitments on the $20,120 of compute) |

- **Biggest line and why:** the database tier is 98 % of the bill (nodes 85 %, disks 13 %); the API that does the numbering is 1.4 %. Price is linear in size, so fewer, bigger shards would save operations, not money; the lever is bytes per document. A real order row (~1 KB, not this lesson's 200 B slice) would make the disk line ≈ 5× ($15k).
- **What changes at 10×:** 100k creates/s ⇒ 50 vCPU ⇒ N = ceil(50 ÷ 8.53) = 6 × `c7g.4xlarge`; per shard 3,100 writes/s ⇒ ≈ 6,200 IOPS (still inside gp3's 16k) but ≈ 1.6 of 4 vCPU before any reads, so shards move to `db.r7g.2xlarge` (or 64 shards); disks reach 190 TB a year (≈ $30k a month at year-end); and the whale hits 10k/s on one counter row, the first real architecture change (bigger blocks, per-instance sub-ranges).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- ≈ 260M new documents a day (3k/s average, 10k/s peak), ≈ 95B rows a year, on 32 shards by `tenant_id`: ≈ 310 creates/s per shard at peak, the whale's shard ≈ 1,300/s (sections 4, 15).
- Every access is by key inside one tenant: take a number, fetch by `(tenant_id, public_id)` or `(tenant_id, number)`, list a tenant's documents. Nothing reads across tenants.
- Three rules the store must enforce itself: unique `(tenant_id, number)` for orders, unique `(tenant_id, series, number)` for issued invoices, and a gap-free counter bumped in the issuing transaction so a rollback un-increments it (9.3). One counter row tops out at ≈ 330 updates/s while its lock is held (9.2).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora), sharded by hand | ✅ chosen | `INSERT … ON CONFLICT DO UPDATE … RETURNING last_value` takes a number in one statement; the row lock held to commit is what makes invoice numbers gap-free; unique keys plus the partial unique index and the `CHECK` make "no duplicate, ever" a database fact; `fillfactor = 50` gives HOT updates on the counter row; `CREATE UNIQUE INDEX CONCURRENTLY` and `NOT VALID` constraints make the live backfill safe (9.5) |
| MySQL (RDS / Aurora), sharded by hand or with Vitess | ✔ works, small edits | `ON DUPLICATE KEY UPDATE … LAST_INSERT_ID(…)` replaces `RETURNING`; unique keys and row locks behave the same (below) |
| DynamoDB | ✔ works for orders, ✖ for gap-free invoices | an `UpdateItem` with `ADD` is an atomic counter that returns the new value, and 10k writes/s needs no shard fleet; blocks still apply (one item ≈ 1,000 writes/s). But only the primary key is unique, so `(tenant, number)` and `(tenant, public_id)` need an item each in a `TransactWriteItems` (2× the write cost), and a transaction on a hot counter fails with a conflict instead of queueing behind a lock, so gap-free issuing becomes retry logic |
| Cassandra / Keyspaces | ✖ poor fit | counter columns can double-count when a write is retried; the safe alternative, a lightweight transaction (`IF last_value = x`, Paxos), costs several round trips per attempt and queues badly on one hot key |
| Redis | ✖ as the record | `INCR` sits outside the order's transaction: a crash leaves gaps and a Redis loss can repeat numbers (9.1) |

**Pick:** PostgreSQL on RDS Multi-AZ, 32 shards of `db.r7g.xlarge` behind a tenant-to-shard map (section 15). It wins on three things: constraints and row locks inside the shard, so numbering, uniqueness and the rollback rule are database guarantees (8, 9.3); `RETURNING` for one-statement allocation; and tenant locality, since a tenant's counter, orders and invoices share a shard and no transaction ever spans two.

**Sharded Postgres, or a key-addressed store?** Throughput alone does not need shards: 10k writes/s is about 3–4 nodes of durable writes (section 15). The 32 shards come from size (≈ 19 TB a year at the assumed 200 B per document, ≈ 600 GB a shard) and tenant isolation. This is a key-addressed workload, so DynamoDB would take the volume with no shards to run and is a credible choice for the order path; what it gives up is the database enforcing the invoice series. Sharding by tenant keeps that path on one node. Ways to shard: by hand as here, Citus (self-managed; not an RDS or Aurora feature) or Aurora Limitless (check availability), or MySQL under Vitess (a `tenant_id` vindex, online resharding, and sequence tables that hand out id blocks, the same idea as 9.2; you run it or buy it, RDS MySQL does not shard).

**If it were MySQL instead:** the same design, with these differences.
- Allocation: `INSERT … ON DUPLICATE KEY UPDATE last_value = LAST_INSERT_ID(last_value + 1)`, then read `LAST_INSERT_ID()` on the same connection. Create counter rows at tenant signup: two sessions inserting the same missing key is a classic InnoDB deadlock under `REPEATABLE READ`.
- The partial unique index on invoices is not needed: unique indexes in both engines allow many NULLs, so a plain `UNIQUE (tenant_id, series, number)` already ignores drafts (it only costs index space). `CHECK` is enforced from 8.0.16.
- Gap-free works under the default `REPEATABLE READ`: `UPDATE` reads the latest committed row and the second issuer just waits for the lock, so there is no serialization error to handle (Postgres needs `READ COMMITTED`, 9.3); an update by full key takes a record lock, not a gap lock. The counter row needs no `fillfactor` or vacuum tuning: InnoDB updates in place and old versions go to the undo log (a long-running transaction delays their purge).
- Clustered key: `orders.id` is append-only, which is what InnoDB wants; the unique keys are secondary indexes that carry the 8-byte `id`. Clustering by `(tenant_id, number)` would keep a tenant's orders together but moves every insert to its tenant's tail instead of the table's.

**What would flip the choice:** the first real change is not an engine: at 100k creates/s the whale hits ≈ 10k/s on one counter row (section 15), which calls for bigger blocks and per-instance sub-ranges (task 04). DynamoDB becomes the better fit if the gap-free series goes away (or is delegated to an invoicing provider) and documents become pure key-value at a rate where 64 shards are a burden. Multi-region writes would not flip it: global tables resolve conflicts by last writer wins, so two regions could issue the same number; a counter needs one home region per tenant.
