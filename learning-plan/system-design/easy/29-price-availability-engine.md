# 29 · Price & Availability Engine

`🟢 easy` · *Topics:* effective-dated prices, price resolution and caching, staged bulk price changes with a one-row switch, derived availability (in stock / pre-order / back-order), stock ledger and conditional decrement, money in minor units, hiding cost and margin · *Builds on:* [13 Shopping Cart](13-shopping-cart.md) (it consumes this engine), [05 Cache](05-kv-cache-ttl.md) (hot set, TTL), [06 Counters](06-view-like-counter.md) (hot-row problem), [28 Booking Calendar](28-booking-calendar.md) (range exclusion constraints)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [28 Booking Calendar for Rentable Units](28-booking-calendar.md) · Next: [30 Invoicing: Money, Tax & PDF](30-invoicing.md)

---

> 🌱 **Seed (honeydolist-site):** a small shop inside a handyman site (PLAN step 10, migrations 0040, 0041, 0079). A product has *options*; an **offer** is one choice of each (the Bale Spear has 6 offers, Quick Move XL has 18), each with its own SKU, `regular_price_cents`, optional `sale_price_cents` with `sale_ends_on` (the regular price "returns by itself"), a buy price `cost_cents`, `stock_qty` (`NULL` = not counted) and `available_from`. The price charged is one function, `ChargedCents(regular, sale, saleEndsOn, today)`. Checkout runs `UPDATE … SET stock_qty = stock_qty - n WHERE stock_qty >= n` per line, every change is a row in `stock_moves`, and a cart holds nothing. The buy price never reaches the public API (`shop.costs`, a test asserts no public answer contains `cost`). A partner's product can send *Buy* to the partner's page (step 61).
> **Here, at scale:** 100M offers, 200k price reads/s at peak, and a Black Friday campaign that must switch 5M prices at exactly 00:00:00, and be rolled back in seconds if it is wrong.

## 1. The task

> "Design the price and availability service behind an online shop. Every product page, search result
> and cart line asks it 'what does this cost right now and can I buy it?'. Merchants schedule sales,
> and once a year they change millions of prices at midnight."

Cheap to state, full of traps: time, rounding, hot rows, and a number nobody may ever see.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| One price per offer, or per region/customer group? | Price lists multiply data. | One price per offer **per currency**; price lists are an extension. |
| Do sales have a *start* as well as an end? | honeydolist only has an end (a sale starts when staff press the button). | Both: sales are scheduled in advance. |
| How exact must the midnight switch be? | Decides the mechanism. | Within seconds, same for every shopper, no half-switched catalogue. |
| Can a sale be undone? | Mistakes at 5M scale are expensive. | Yes, in under a minute, without touching 5M rows. |
| What does "available" mean? | Stock, dates, suppliers. | In stock, low stock, pre-order (future date), back-order (supplier lead time), out of stock. |
| Reserve stock when added to a cart? | See [13](13-shopping-cart.md) §9.5. | No. Enforce at checkout. |
| Who sees the cost price and margin? | Leaks are commercial disasters. | Staff with a permission; never any public response. |
| Scale? | Cache and write design. | 100M offers, 200k price reads/s peak, 5M-price campaigns. |
| Partner-fulfilled products? | Their stock is theirs. | Price shown, *Buy* redirects to the partner; no cart, no stock promise. |

## 3. Requirements

**Functional**
- Resolve the price charged and availability of any offer, in bulk (a listing page asks for 24 at once).
- Schedule regular and sale prices with start and end; stage and activate campaigns of millions of prices.
- Take stock at checkout atomically; record every stock change; support pre-order and back-order.
- Hide cost and margin from everything except permissioned staff views.

**Non-functional**
- Price reads p99 under 20 ms from cache; every shopper sees the same price at the same instant.
- A displayed price is never silently changed at checkout (`409` with the new price instead).
- Last month's order totals and profit never change when prices change.

**Out of scope**
- Tax ([30](30-invoicing.md)), promo codes and stacking (extension), search ranking, recommendations.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Offers | 100M | given |
| Offers table | ~30 GB | 100M × ~300 B with indexes |
| Price reads | **200k/s peak** | ~8k page views/s × 24 offers on a listing page |
| Ordinary price changes | ~12/s | 1% of SKUs a day = 1M ÷ 86,400 |
| Campaign switch, row by row | **~8 min** | 5M updates at 10k/s = 500 s: shoppers see mixed prices the whole time |
| Price history | ~29 GB/year | 1M changes/day × 365 × ~80 B |
| Staged campaign | ~300 MB | 5M rows × ~60 B including the key |
| Hot-set cache | ~60 MB | top 1M offers × ~60 B, assumed ~90% of reads (fits in every app instance) |
| Whole price cache | ~6 GB | 100M × 60 B (fits one Redis; not needed in full) |
| Stock writes | ~100/s average, 5k/s peak | one hot launch SKU can see ~20k attempts/s (assumption) |

What the numbers tell us: reads are the load (200k/s), writes are small (12/s) except for **one
event**, the midnight switch, which must not be an update storm. A 60 MB hot set means most reads never
leave the app server. The hard problems are the *switch*, the *hot stock row*, and *not leaking cost*.

## 5. Plan

1. Model: catalogue, effective-dated prices, costs apart, stock ledger.
2. Resolve "price now": the read path and its cache.
3. Bulk campaigns: staged, sealed, switched by one row, rolled back by one row.
4. Availability and stock: derived states, conditional decrement, the hot row.
5. Money and margin: integers, rounding, who may see cost.

## 6. High-level design

```
 shoppers ─► CDN ─► Catalog API ─► app-local hot cache (60 MB, 5 s) ─► Redis (price:{offer}) ─► Postgres read replicas
                          │                 ▲ overlay applied here at read time: active campaigns (tiny list, cached 5 s)
 staff ─► Admin API ──────┼─► offer_prices (dated rows, exclusion constraint)
                          ├─► price_campaigns + campaign_prices (staged, sealed, immutable)
                          └─► offer_costs (separate table, separate DB role)
 checkout ─► Order service ─► BEGIN; UPDATE offers SET stock_qty = stock_qty - n WHERE id = $1 AND stock_qty >= n;
                              INSERT stock_moves; INSERT order lines (price, cost snapshot); COMMIT
```

**A read:** the base price comes from the cache; the app then applies any *active campaign* overlay
using a tiny cached list. Nothing is invalidated when a campaign starts. **A checkout** re-prices
from this same path and compares with what the shopper saw.

## 7. API

```
GET  /products/{slug}?currency=USD     → 200 { offers: [{ sku, price: { charged, regular, sale_ends_at? }, availability: { state, qty_hint?, available_from?, ships_in_days? } }] }
POST /checkout   Idempotency-Key       → 201 | 409 { conflicts: [{ sku, reason: "price_changed"|"out_of_stock"|"not_for_sale", now: … }] }
GET  /products/{slug}/buy?offer=SKU    → 302 partner page (a counted click, never blocks)
-- admin (permissioned)
PUT  /admin/offers/{sku}/prices        { kind, amount, valid_from, valid_to }      → 200 | 409 "overlaps another price" | 422 "sale not below regular"
POST /admin/campaigns                  { name, valid_from, valid_to }              → 201 { id, status: "staging" }
POST /admin/campaigns/{id}/prices      NDJSON stream of { sku, amount }            → 202
POST /admin/campaigns/{id}/seal        → 200 { item_count, checksum } | 422 { below_cost: [...] }
POST /admin/campaigns/{id}/cancel      → 200   (the rollback)
POST /admin/offers/{sku}/stock-moves   { delta, reason, note }                     → 201
GET  /admin/offers/{sku}/margin        → 200 { cost, charged, profit, margin_pct } (needs shop.costs)
```

## 8. Data model

```sql
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE offers (                         -- catalogue only: no price columns
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id     BIGINT NOT NULL,
    sku            TEXT   NOT NULL UNIQUE,
    option_values  TEXT[] NOT NULL DEFAULT '{}',
    source_id      BIGINT,                    -- dealer / our own stock: lead time, discount
    external_url   TEXT,                      -- set: partner fulfils; Buy redirects
    stock_qty      INTEGER CHECK (stock_qty >= 0),   -- NULL = not counted (made to order)
    available_from DATE,                      -- NULL or past = now
    deleted_at     TIMESTAMPTZ
);

CREATE TABLE offer_prices (                   -- dated, never edited: a change closes one row and opens the next
    offer_id   BIGINT    NOT NULL REFERENCES offers(id),
    kind       TEXT      NOT NULL CHECK (kind IN ('regular','sale')),
    currency   CHAR(3)   NOT NULL DEFAULT 'USD',
    amount     BIGINT    NOT NULL CHECK (amount >= 0),         -- minor units (cents)
    valid      TSTZRANGE NOT NULL,                             -- [from, to)
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (offer_id WITH =, kind WITH =, currency WITH =, valid WITH &&)
);

CREATE TABLE offer_costs (                    -- what we pay: its own table, its own privileges
    offer_id BIGINT PRIMARY KEY REFERENCES offers(id),
    amount   BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL
);
REVOKE ALL ON offer_costs FROM catalog_reader;          -- the role the public read path connects as

CREATE TABLE price_campaigns (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL,
    valid      TSTZRANGE NOT NULL,
    status     TEXT NOT NULL DEFAULT 'staging' CHECK (status IN ('staging','sealed','cancelled')),
    item_count BIGINT, checksum TEXT                       -- fixed at seal time
);
CREATE TABLE campaign_prices (                -- immutable once the campaign is sealed
    campaign_id BIGINT NOT NULL REFERENCES price_campaigns(id),
    offer_id    BIGINT NOT NULL,
    amount      BIGINT NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (campaign_id, offer_id)
);

CREATE TABLE stock_moves (                    -- the ledger; stock_qty is its running total
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    offer_id BIGINT  NOT NULL,
    delta    INTEGER NOT NULL CHECK (delta <> 0),
    reason   TEXT    NOT NULL CHECK (reason IN ('checkout','cancel','manual')),
    order_id BIGINT, user_id BIGINT, note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX stock_moves_offer_idx ON stock_moves (offer_id, created_at DESC);
```

An order line stores `unit_price`, `unit_cost` and `regular_price` **copied at checkout** (as in
[13](13-shopping-cart.md) §9.6, and honeydolist's `unit_cost_cents`), so history never moves.

## 9. Deep dives

### 9.1 Prices that change over time

honeydolist keeps `regular`, `sale` and `sale_ends_on` *on the offer row*. That makes the rule
"a sale is below the regular price" a one-line `CHECK`, but there is no start date, no history, and
"what did it cost on March 3rd?" is unanswerable.

| Option | Pros | Cons |
|---|---|---|
| Columns on the offer (honeydolist) | trivial; `CHECK (sale < regular)` | no scheduled start, no history, an update overwrites the past |
| **Dated rows with an exclusion constraint** | schedules, history and audits for free; one price per offer/kind/currency per instant is guaranteed | no cross-row `CHECK`; every reader resolves "valid now" |
| A cron that flips prices at the boundary | simple to describe | 5M row writes at midnight, missed runs, clock skew, no audit |

**Choice and why: dated rows.** The exclusion constraint is the same trick as [28](28-booking-calendar.md),
and honeydolist already uses it for sales-tax rates ("a new rate is a new row from a date, which closes
the one before"). Give up the cross-row `CHECK` and enforce "sale below regular, and above the floor"
in the write path (validate against the regular row valid over the sale's whole range), then audit
nightly with a query. Define boundaries in a *business time zone* and store `timestamptz`; honeydolist's
`sale_ends_on` is a **date** whose last day is included, a quiet source of "ended a day early" bugs.

### 9.2 The read path: resolve at read, cache the stable part

Every read needs "regular now, sale now, campaign now". 200k/s straight to Postgres is too much.

| Option | Pros | Cons |
|---|---|---|
| Resolve from `offer_prices` per request | always exact | 200k queries/s |
| **Cache base prices with `valid_until`; apply time-dependent overlays in memory** | the cached part changes rarely; midnight invalidates nothing | a little CPU per read; the overlay list must be tiny |
| Precompute "current price" with a flipper job | O(1) reads | the flipper must write at every boundary, the storm again |

**Choice and why: cache the stable part, resolve time at read time.** The cached value is the base
row set with its own `valid_until` (`min` of the rows' end times); the app compares `now()` to it, so
a sale expiring at 23:59:59 needs no purge. The 60 MB hot set lives in each instance (5 s TTL), the
rest in Redis, the rest in replicas. Always resolve in bulk (`WHERE offer_id = ANY($1)`), never one
query per card. The result per offer is
`charged = min(regular, active sale, active campaign)`, never above regular and never below the floor.
honeydolist makes this *one function* mirrored in the frontend with shared test cases; do the same, or
the product page and the cart will disagree by a cent and someone will notice.

### 9.3 Switching 5M prices at midnight (and taking it back)

| Option | Pros | Cons |
|---|---|---|
| `UPDATE` 5M offers at 00:00 | simple | ~8 min of mixed prices, WAL and replica lag, no rollback except another 5M updates |
| Flip a global "price epoch" | O(1) switch | every cache key changes: all 100M entries go cold at once |
| **Stage a campaign, seal it, activate by time** | the 5M rows are written days earlier, checked, and immutable; activation is *reading the clock*; rollback is one row | campaign precedence is a rule to document |

**Choice and why: campaigns.** Merchants upload to `campaign_prices` (status `staging`) days ahead; a
validator runs (below-cost list, below-floor, duplicates, row count) and `seal` freezes it with a count
and checksum. At `valid_from` nothing is written: readers see the sealed campaign in a cached list of
active campaigns (5 s TTL) and apply its overlay. Rows are immutable, so the overlay lookups are
cacheable forever by `(campaign_id, offer_id)`; **pre-warm the top 1M offers** ten minutes before the
start so the stampede never reaches Postgres. **Rollback:** `status = 'cancelled'`; within a few
seconds every instance ignores the overlay. honeydolist's *Put on sale* also shows the resulting margin
first and warns when a sale would sell below cost; run the same check at seal time.

### 9.4 Availability: derived, never a second switch

honeydolist removed `is_available`: "an empty shelf is a count of 0, and a second switch would only
fall out of step with it". Availability is a *function* of stock, date and supplier:

| Stock | `available_from` | Source lead time | State shown | Add to cart |
|---|---|---|---|---|
| not counted | empty/past | none | Available | yes |
| > low mark | empty/past | – | In stock | yes, up to count |
| 1–3 | empty/past | – | Only N left | yes, up to count |
| any | **future** | – | Available from {date} | yes, pre-order |
| 0 / not counted | empty/past | **has one** | Ships in ~N weeks | yes, back-order, no stock taken |
| 0 | empty/past | none | Out of stock | no |

**Stock is taken at checkout, in the order's own transaction:**
`UPDATE offers SET stock_qty = stock_qty - $n WHERE id = $1 AND stock_qty >= $n`; zero rows means
`409 out_of_stock` with how many are left, and nothing is written. Every change also inserts a
`stock_moves` row, so "why does it say 2?" always has an answer; staff change stock **only by writing a
move** ("Received 4", "Counted 6"), never by overwriting the number. Cancelling gives units back (a
`cancel` move). Offers with `stock_qty IS NULL` (not counted) skip the update. Display counts are cached for seconds; the conditional update is the truth.

**The hot row.** One launch SKU at 20k attempts/s all queue on one row lock (a few thousand updates/s
at best). Easy-level answer: let the losers fail fast with `409`, rate-limit that SKU ([03](03-rate-limiter.md)),
and show "Only N left" from a cache. Sharding the count into buckets ([06](06-view-like-counter.md))
makes "never below zero" hard; that is the medium flash-sale task.

**Partner-fulfilled offers** (`external_url` set): show the price, hide the stock line, and route *Buy*
through your own `/buy` endpoint that records the click and answers `302`. honeydolist counts a click
only once per visitor and product per 30 minutes and ignores staff and robots; the cart refuses such an
offer (`422`), and drops it if a cart held it when the link was switched on.

### 9.5 Money, rounding and the number nobody may see

- **Integers.** Minor units (`BIGINT`), a currency code beside every amount, never `float64` ([13](13-shopping-cart.md) §13).
  A percentage sale rounds **once**, at the price, half-up, and the rounded value is stored.
  `$1,245.00` against a regular `$1,369.50` is `124,500` vs `136,950` cents: a `124.50` saving, a
  *9%* badge (12,450 ÷ 136,950 = 9.09%).
- **Margin** is computed where shown, never stored: profit = charged − cost; margin = profit ÷ charged.
  With an (assumed) cost of `$900`: profit `$345`, margin `345 ÷ 1,245 = 27.7%`. Red below the
  low-margin mark (honeydolist's default is 15%).
- **Cost visibility is a structural fact, not a UI choice.** `offer_costs` is a separate table, the
  public read path connects as a role with no `SELECT` on it, public response types do not contain the
  field, and a contract test asserts no public response contains `cost`. A bug in a template cannot
  leak what the query cannot read.
- **Orders keep the cost they were sold at**, so profit for last month does not change when a dealer
  raises prices.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** (1) cache stampede when a hot sale starts or a node restarts: warm
  the hot set, coalesce misses with `singleflight`, add jitter to TTLs. (2) The hot stock row. (3)
  `campaign_prices` lookups for cold offers: batch them with the base price fetch.
- **Redis down:** reads fall to replicas with per-instance caches absorbing most of it; shed load
  (serve slightly stale base prices) rather than fail.
- **Postgres primary down:** reads continue from replicas/caches; checkout is unavailable, which it
  must be (we cannot take stock or freeze prices).
- **Replica lag during a bulk load:** campaign rows are written days before use, so lag cannot matter.
  Seal waits for replicas to confirm the count and checksum.
- **Wrong campaign:** cancel it. Seconds, not minutes. An in-flight checkout re-prices, sees the new
  price and answers `409 price_changed`; nobody is charged an unshown price.
- **Clock skew:** the instant is `now()` per request on the app, but compare to the database's idea at
  seal time and keep NTP healthy; two instances a second apart may disagree for a second, which is
  acceptable and visible only at the boundary.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| dated price rows + exclusion constraint | price columns on the offer | schedules, history, audits | the single-row `CHECK`; a resolver on every read |
| overlay resolved at read time | flipping prices at midnight | no update storm, nothing invalidated | a little CPU per read |
| staged, sealed, immutable campaigns | `UPDATE` of 5M rows | days of validation, one-row rollback | a precedence rule, an upload workflow |
| derived availability | an `is_available` flag | no state to fall out of step | a function to keep in sync with the frontend |
| conditional `UPDATE` + ledger | reserving on add | the last unit goes to exactly one buyer | `409`s at checkout |
| costs in a separate table and role | a hidden column | a leak is structurally impossible | an extra join for staff views |
| rejecting a changed price at checkout | charging the new price | no surprise charges | an extra click for the shopper |

## 12. Check yourself

1. Why store prices as dated rows with an exclusion constraint instead of columns on the offer, and what do you lose?
2. How do you switch 5M prices at exactly midnight without 5M updates, and how do you undo it?
3. Why is "is this in stock" a derived value, and what are the five states in the table?
4. Two shoppers buy the last unit at once. What happens, and what records it?
5. How do you make a cost-price leak structurally impossible?

<details><summary>Answers</summary>

1. Dated rows give scheduling, history and audits, and the constraint guarantees one price per
   offer, kind and currency at any instant. You lose the one-row `CHECK (sale < regular)`, so the
   write path must enforce it (and a nightly audit verifies), and every reader resolves "valid now".
2. Stage the prices days earlier into an immutable `campaign_prices` set, validate and seal it, and let
   readers apply it when `now()` is inside the campaign's range: no writes at midnight. Roll back by
   setting the campaign `cancelled`; instances ignore it within the cache TTL. Pre-warm hot offers to
   avoid a stampede.
3. A stored flag falls out of step with stock, dates and supplier lead time. The states are
   available (not counted), in stock/only N left, pre-order (future `available_from`), back-order
   (zero stock, supplier lead time), and out of stock.
4. Both run `UPDATE … SET stock_qty = stock_qty - n WHERE stock_qty >= n`; the row lock serialises
   them, the first matches a row, the second matches none and gets `409 out_of_stock`. The winner's
   transaction also inserted a `stock_moves` row (`checkout`), the ledger behind the count.
5. Keep cost in its own table that the public read path's database role cannot read, leave the field
   out of public response types, copy cost onto order lines at checkout, and add a contract test that
   fails if any public response contains `cost`.

</details>

## 13. In Go

- Related lessons: [59 Production DB patterns](../../59-production-database-patterns.md) (conditional updates, exclusion constraints, ledgers), [38 Caching](../../38-caching-patterns.md) (hot sets, `singleflight`, TTL jitter), [22 database/sql](../../22-database.md) (`= ANY($1)` bulk reads, `COPY` for campaign upload), [41 API design](../../41-api-design-evolution.md) (`409` with conflict lines), [55 Data pipelines](../../55-data-pipelines.md) (the NDJSON campaign loader), [57 Web security](../../57-web-security.md) (not leaking fields), [18 Testing](../../18-testing.md) (shared table-driven price cases).
- One pure function, tested with a table, used by the API and the checkout:

```go
// Charged returns the price in cents: the lowest of regular, sale and campaign, never above regular.
func Charged(regular int64, sale, campaign *int64) int64 {
	p := regular
	for _, c := range []*int64{sale, campaign} {
		if c != nil && *c < p {
			p = *c
		}
	}
	return p
}
```

- Packages: `pgx` (`CopyFrom` for campaign rows), `go-redis`, `golang.org/x/sync/singleflight`, `shopspring/decimal` only at the edges (percent maths), `int64` cents inside.

## 14. Extensions

- **Price lists** per region and customer group (B2B): add `price_list_id` to the constraint; fall back list → default.
- **Volume tiers and bundles:** price as a function of quantity; where does it live in the cart ([13](13-shopping-cart.md))?
- **Promo codes and stacking:** a rules engine over `Charged`; precedence, caps, exclusivity.
- **Price history rules:** some places require showing the lowest recent price beside a reduction; the dated rows already answer it.
- **Structured data:** honeydolist emits an `Offer` with `price`, availability (`InStock`, `OutOfStock`, `PreOrder` with its date) and `priceValidUntil` = the sale's last day; generate it from the same resolver.
- **Flash sales:** hot-SKU admission control with a queue (medium backlog 07).
- **Competitor-driven repricing** as a stream of price proposals that need approval before they are sealed.
