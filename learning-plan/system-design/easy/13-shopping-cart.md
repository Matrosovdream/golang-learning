# 13 · Shopping Cart

`🟢 easy` · *Topics:* guest vs authenticated state, merging on login, idempotent `PUT` semantics, price snapshot vs live price, why not to reserve inventory on add, cart → order as an immutable snapshot · *Builds on:* [05 Cache](05-kv-cache-ttl.md) (TTL state), [06 Counters](06-view-like-counter.md) (idempotent writes)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [12 Notification Service](12-notification-service.md) · Next: [14 Polls & Voting](14-polls-voting.md)

---

## 1. The task

> "Design the shopping cart for an e-commerce site. Users add and remove items, the cart follows
> them across devices, guests can shop before logging in, and checkout turns the cart into an order."

Low traffic by system-design standards, yet full of semantics interviewers love: what *is* a
cart when prices change, stock runs out, and a guest logs in with a different cart waiting.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Guest carts? What happens on login? | Merge rules are the trickiest part. | Yes; merge guest cart into the user's cart on login. |
| Cart lifetime? | Storage and TTL. | Guests 7 days; logged-in users 90 days of inactivity. |
| Max items per cart? | Bounds the document size. | 100 lines. |
| Check inventory on add, or at checkout? | Reserving on add leads to hoarding and stale reservations. | Validate at add (soft, informational), enforce at checkout. |
| Prices: locked when added, or current? | Changes the data model and UX. | Show current price; tell the user when it changed since they added it. Lock at order creation. |
| Scale? | Store choice. | 10M daily users, 50M live carts, ~5k cart writes/s at peak. |
| Consistency? | A user must see their own writes immediately, across devices. | Read-your-writes for the owner. |
| Analytics on carts ("abandoned cart" emails)? | Needs queryable storage. | Yes. |
| Promotions? Multi-currency? | Scope creep. | Out of scope; price is a single currency. |

## 3. Requirements

**Functional**
- Add, update quantity, remove items; view the cart with current prices and availability.
- Guest carts persist via a cookie; on login, merge into the account cart.
- Cart is shared across the user's devices.
- Checkout converts the cart into an order with prices frozen at that moment.
- Abandoned-cart detection for marketing (task 12).

**Non-functional**
- Writes visible immediately to the same user; p95 under 100 ms.
- No cart lost for logged-in users; guest carts may expire.
- Retrying any cart operation is safe.

**Out of scope**
- Promotions engine, taxes/shipping calculation, saved-for-later, wishlists.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Live carts | 50M | given |
| Cart size | ~2 KB | ~10 lines × ~150 B + header |
| Storage | ~100 GB | 50M × 2 KB; fits a modest Postgres or a Redis cluster |
| Writes | ~5k/s peak | adds, quantity changes, removals |
| Reads | ~50k/s peak | every page shows the cart badge; cache it |
| Price lookups | 50k/s × 10 lines = 500k product reads/s | must come from a cache, not the catalogue database |

What the numbers tell us: comfortably within one database for the carts themselves. The
hidden load is **enriching** a cart with current prices and stock on every read; that's a
product-cache problem, not a cart problem.

## 5. Plan

1. Storage: Redis, Postgres, or both, and what "cart" means for each.
2. Model: cart header + lines; what is stored vs what is computed at read time.
3. API with idempotent semantics.
4. Guest carts and the merge on login.
5. Prices and inventory: snapshot vs live, reserve or not.
6. Checkout: the cart → order transition.

## 6. High-level design

```
 browser (cookie: cart_id for guests / session for users)
    │
    ▼
 LB ──► Cart API ──► Postgres (carts, cart_items)              ← source of truth, 90-day TTL via updated_at
          │     └──► Redis (cart:{id} hash, cache-aside, 1 h)   ← hot reads, badge counts
          ├──► Product cache (price, stock flags) for enrichment on read
          └──► checkout: BEGIN; validate stock; INSERT orders + order_items (price snapshot); mark cart converted; COMMIT
 jobs: expire guest carts (7 d), abandoned-cart detector (updated_at < now − 24 h, status = active) → task 12
```

A cart write goes to Postgres and invalidates (or updates) the Redis copy. A read gets the cart
lines from Redis or Postgres, then decorates each line with current price and availability from
the product cache. Checkout is a single transaction that freezes prices into the order.

## 7. API

```
GET    /cart                              → 200 { id, items: [{ sku, qty, name, unit_price, price_changed_from?, available }], subtotal, updated_at }
PUT    /cart/items/{sku}   { "qty": 2 }    → 200 cart     idempotent: sets quantity (qty 0 = remove)
DELETE /cart/items/{sku}                   → 200 cart     idempotent
POST   /cart/items         { "sku": "…", "qty": 1 }  with Idempotency-Key  → 200 cart   (add-to-cart buttons: increments)
DELETE /cart                               → 204
POST   /cart/merge         (called by the login flow; body: guest cart id)  → 200 cart
POST   /checkout           Idempotency-Key  → 201 { order_id } | 409 { "conflicts": [{ sku, reason: "out_of_stock" | "price_changed", … }] }
```

The cart is identified by the session: guests by a `cart_id` cookie (random, HttpOnly, 7-day
expiry), users by their account. Never accept a cart ID from the body for a logged-in user.

## 8. Data model

```sql
CREATE TABLE carts (
    id          UUID PRIMARY KEY,
    user_id     BIGINT,                          -- NULL for guests
    status      TEXT NOT NULL DEFAULT 'active',  -- active | converted | expired | merged
    version     INTEGER NOT NULL DEFAULT 0,      -- optimistic concurrency for whole-cart ops
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX carts_one_active_per_user ON carts (user_id) WHERE status = 'active' AND user_id IS NOT NULL;
CREATE INDEX carts_abandoned_idx ON carts (updated_at) WHERE status = 'active';

CREATE TABLE cart_items (
    cart_id       UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    sku           TEXT NOT NULL,
    qty           INTEGER NOT NULL CHECK (qty > 0),
    price_at_add  NUMERIC(12,2) NOT NULL,        -- for "price changed" messaging only
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cart_id, sku)
);
```

The unique partial index enforces **one active cart per user** at the database level, so two
devices racing to create a cart can't make two.

Redis cache: `cart:{id}` → hash `{sku: qty}` plus `meta` field, TTL 1 h, rewritten on each change.

## 9. Deep dives

### 9.1 Where does the cart live

| Option | Pros | Cons |
|---|---|---|
| **Redis only** (hash per cart, TTL) | fastest; TTL is native; simplest code | a Redis incident loses every cart; no SQL for analytics ("abandoned carts with > €100"); persistence (AOF) helps but isn't a database |
| **Postgres only** | durable, queryable, transactional with checkout | 50k reads/s for badge counts and cart pages hit the database; TTL needs a job |
| **Postgres as truth + Redis cache-aside** | durable and fast; analytics in SQL; cache loss is a cache loss | two systems; write path updates both |
| Document store (DynamoDB) with TTL attribute | scales writes horizontally, TTL native | not needed at 5k writes/s; cross-cart queries are awkward |

**Choice: Postgres + Redis cache.** The cart is money-adjacent (it turns into an order) and the
business wants to query it; durability and SQL win. Guest carts could live in Redis only
(they're allowed to expire), but one code path is simpler; a 7-day `expires_at` and a cleanup job
cover them.

### 9.2 Idempotent operations

Users double-click, mobile apps retry, and proxies replay. `POST /cart/items {sku, qty: 1}` as
"increment" retried twice adds two. Two fixes, use both:

- Make the primary mutation **`PUT /cart/items/{sku} {qty}`**: it *sets* the quantity, so
  repeating it is harmless. The UI's "+" button sends the new total quantity, not "+1".
- For the convenience `POST` increment, require an `Idempotency-Key` (lesson 41) stored for 24 h.

Cross-device conflicts (phone sets qty 2, laptop sets qty 3 at the same instant): last write wins
per line is acceptable for a cart. For whole-cart operations (merge, checkout) use the `version`
column: `UPDATE carts SET … WHERE id = $1 AND version = $2`; zero rows means someone else changed it
and the operation re-reads and retries.

### 9.3 Guest carts and the merge

Guest cart: a random `cart_id` in an `HttpOnly` cookie; the row has `user_id NULL` and a 7-day
expiry. On login the guest cookie is still present, so the login flow calls `merge(guest_id →
user)`:

| Rule | Behaviour |
|---|---|
| Same SKU in both | **sum** quantities (capped by a per-line max), or **max**: pick one and document it. Sum matches "I added it twice on purpose"; max avoids surprise duplicates. We choose max. |
| SKU only in guest cart | move it over. |
| Guest cart empty | nothing; keep the user cart. |
| User cart empty | adopt the guest lines. |
| Merge done | mark the guest cart `merged`, clear the cookie. |

Do it in one transaction with the user cart row locked (`SELECT … FOR UPDATE`), so two
concurrent logins (two tabs) can't double-merge. Make merge idempotent: a guest cart already
`merged` is a no-op. Never merge *into* the guest cart; the user cart is the one with history.

### 9.4 Prices: snapshot or live?

| Approach | Pros | Cons |
|---|---|---|
| **Live price at read**, lock at checkout | always correct; no stale discounts honoured | the subtotal can change between page views; needs messaging |
| Price locked at add | predictable for the user | legally and commercially awkward (a 2-week-old price); needs expiry rules |
| **Live + remember `price_at_add`** | correct *and* explainable ("was €20, now €18") | one extra column |

**Choice: live + `price_at_add`.** Prices and stock are looked up on read from the product cache
(a local or Redis cache of the catalogue with a short TTL); at 500k lookups/s this cache is
the real performance work. The cart service never stores a price it treats as authoritative,
except in the order.

### 9.5 Inventory: do not reserve on add

Reserving stock when an item is added means a cart can hold the last unit for 7 days while a
buyer who wants it now sees "out of stock". Carts are abandoned ~70% of the time. So:

- **On add:** check availability for feedback only ("only 2 left"); don't reserve.
- **At checkout:** validate and decrement stock atomically in the order transaction
  (`UPDATE inventory SET available = available − $qty WHERE sku = $1 AND available >= $qty`; zero
  rows → conflict). Return `409` with the exact lines that failed so the UI can fix the cart.
- **Short reservation during payment** (hold stock for ~10 minutes while the user pays) is the
  middle ground when payment is slow; it's a reservation with a TTL and a release job. Flash sales
  need more than that (medium track).

### 9.6 Checkout: cart → order

One transaction: lock the cart (`FOR UPDATE`, check `status = active` and `version`), re-read
current prices, compare with what the client displayed (the client sends the subtotal it saw;
mismatch → `409 price_changed`), decrement inventory per line, insert `orders` and
`order_items` with **frozen** `unit_price`, set the cart `converted`, commit. The order is an
**immutable snapshot**; the cart was a mutable working set. An `Idempotency-Key` on `POST
/checkout` guarantees a retried checkout returns the same order rather than creating two.

After commit: emit `order.created` (outbox, task 10) for payment, email (task 12), analytics.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** product enrichment at 5M lookups/s. A per-instance in-memory
  cache of hot SKUs (seconds TTL) in front of Redis, and batched lookups (one call for all lines).
- **Redis down:** reads fall through to Postgres (50k/s is heavy; replicas help); writes unaffected.
  Degrade the badge count (hide it) before degrading the cart page.
- **Postgres down:** carts can't change. Serve the cached cart read-only with a banner. Checkout
  is down, which it must be.
- **Catalogue down:** show the cart with the last known prices from the cache and disable checkout
  (we cannot freeze prices we can't verify).
- **Cart bloat / abuse:** 100-line cap, qty cap per line, rate limit cart writes per session (task 03).
- **Guest cookie theft:** the cart ID is a bearer token to a guest cart; keep it `HttpOnly`,
  `Secure`, `SameSite=Lax`, random (128 bits), and never let it reference a *user's* cart.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Postgres + Redis cache | Redis only | durability, SQL analytics, transactional checkout | two systems to update |
| `PUT` sets quantity | `POST` increments | idempotent by design | the UI must send absolute quantities |
| merge with max per SKU | sum | no surprise duplicates | a deliberate double-add merges to one |
| live price + `price_at_add` | locked price | correctness, explainable changes | subtotal can change between views |
| no reservation on add | reserve on add | stock stays available to buyers | checkout can fail with `409`; the UI must handle it |
| immutable order snapshot | order referencing cart | audit-safe history | duplication of line data |

## 12. Check yourself

1. Why not reserve inventory when an item is added to the cart?
2. What goes wrong with `POST /cart/items {sku, qty: 1}` as the only add operation, and what are the two fixes?
3. Describe the merge on login, including how you prevent two tabs from merging twice.
4. Why store `price_at_add` if prices are live?
5. Checkout: what exactly is in the transaction, and what makes a retried checkout safe?

<details><summary>Answers</summary>

1. Most carts are abandoned, so reserved stock would sit unavailable to real buyers for days;
   it also needs reservation expiry and release logic. Check availability on add for feedback only,
   and enforce atomically at checkout where the intent to buy is real.
2. It is an increment, so a retried or double-clicked request adds two. Make the primary operation
   `PUT /cart/items/{sku} {qty}` (set the quantity, idempotent), and require an `Idempotency-Key`
   for any increment-style `POST`.
3. Lock the user's active cart `FOR UPDATE`, load the guest cart, for each guest line insert or
   take the max quantity on conflict, mark the guest cart `merged`, clear the cookie, commit. The
   row lock serialises concurrent merges and the `merged` status makes the second one a no-op.
4. To explain changes: the cart shows "was €20, now €18" or warns about an increase. It is never
   used as the charged price, which is the live price frozen into the order at checkout.
5. Lock cart (status, version) → re-read live prices and compare with what the client displayed →
   decrement inventory per line with a conditional update → insert order and order lines with
   frozen prices → mark the cart converted → commit. An `Idempotency-Key` on the checkout request
   returns the existing order on retry; the cart's `converted` status is a second guard.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (transactions, upserts), [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE`, optimistic versions, partial unique indexes), [41 API design](../../41-api-design-evolution.md) (idempotency keys, `409` with problem details), [56 Authentication & sessions](../../56-authentication-sessions.md) (guest cookie), [38 Caching](../../38-caching-patterns.md) (cache-aside, product cache), [31 DDD](../../31-ddd-tactical.md) (Cart as an aggregate with invariants: max lines, qty > 0).
- Money: `NUMERIC` in the database, a decimal type or integer minor units (cents) in Go; never `float64` (same rule as the blockchain track).
- Packages: `pgx`, `go-redis`, `shopspring/decimal` or `int64` cents.

## 14. Extensions

- **Saved for later** and **wishlists**: same aggregate or a different one?
- **Promo codes:** a rules engine evaluated at read time; where do stacking and expiry live?
- **Short payment reservations** with TTL and a release job (task 15); then **flash sales** with a queue as admission control (medium).
- **Multi-currency / tax**: price books per region, tax at checkout; what changes in the snapshot?
- **Real-time stock warnings** on the cart page via SSE (lesson 58).
- **Cart in multiple regions:** where is the truth for a user travelling between EU and US datacenters?

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Cart API (Go, stateless) | 50k reads/s + 5k writes/s | CPU | reads 50k × 0.15 ms = 7.5 vCPU; writes 5k × 0.4 ms = 2 vCPU; total 9.5 vCPU. N = ceil(9.5 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(2.2) = 3 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS or EKS | 3, one per zone | JSON, one Redis call and an occasional SQL statement is pure CPU work; memory use is about 2 GiB of 16, so `c` (2 GiB per vCPU), not `m`. 40 % busy normally, 59 % with a zone down |
| Redis (hot carts, cache-aside, 1 h TTL) | 55k ops/s (50k reads + 5k rewrites); hot set = carts touched in the last hour | memory | assume 10 % of the 50M carts = 5M × 2 KB = 10 GB × ~1.7 overhead = 17 GB; `maxmemory` ≤ 70 % of RAM ⇒ ≥ 24 GB; 55k ops/s is under one core's ~100k | `cache.r7g.xlarge` (4 vCPU / 32 GiB), ElastiCache | 1 primary + 1 replica in another zone | Memory-bound, so the `r` family (8 GiB per vCPU; `m` would need 8 vCPU for 32 GiB, `c` 16). CPU sits at about half a core. Reads go to the primary: a replica read could miss the user's own write (section 3, read-your-writes). A second shard only when memory or ops double |
| Postgres (carts, cart_items) | 100 GB (≈ 150 GB with indexes, assumption); 5k committed writes/s | memory, then IOPS | hot set ≈ 10 GB rows + 5 GB indexes fits in 64 GiB; ≈ 2 IOPS per committed write (WAL flush + dirty pages, assumption) = 10k, + ~1k miss reads ⇒ `gp3` 200 GB at 12,000 IOPS (cap 16,000); CPU: 5k × 0.5 ms + 2.5k Redis misses × 0.2 ms ≈ 3 of 8 vCPU | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ | 1 primary + synchronous standby | RAM holds the hot set, so misses stay rare; `r7g.xlarge` (4 vCPU) would sit at 75 % CPU. gp3 is enough below 16k IOPS; above that move to `io2` or NVMe (`i4i`) with replicas. Reads never reach it, because Redis answers them (95 % hit, assumption) |

Product enrichment (500k lookups/s) runs in-process from a local cache of hot SKUs: ~1M SKUs × 500 B = 500 MB per node (assumption), about 0.1 vCPU at 0.2 µs a lookup, already inside the 0.15 ms per read. The catalogue service that feeds that cache is another system and is not sized here. Expiry and abandoned-cart jobs are batched scans of `carts_abandoned_idx`: a scheduled ECS task, no instance of their own.

Network: assume ~0.5 KB per response on average (badge counts ≈ 0.3 KB, full carts ≈ 2 KB), so 55k × 0.5 KB = 27.5 MB/s (220 Mbit/s) at peak, about 9 MB/s per API node. Nothing is network-bound; the cost is the transfer itself (below).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.2xlarge` (3 × $212) | $640 |
| Redis: 2 × `cache.r7g.xlarge` (2 × $310) | $620 |
| Postgres: `db.r7g.2xlarge` ≈ $620 managed, Multi-AZ doubles | $1,240 |
| gp3 200 GB + 9k extra IOPS: (200 × $0.08 + 9,000 × $0.005) × 2 for the standby | $120 |
| ALB: $16 + ≈ 30 LCU-hours average (8 MB/s ≈ 30 GB/h, 1 GB/h per LCU) × $0.008 × 730 | $190 |
| Internet egress: 27.5 MB/s peak, ~30 % average ≈ 8.25 MB/s ≈ 21.7 TB a month × $0.09 | $1,950 |
| Cross-zone calls: ⅔ of Redis (≈ 13 TB) and Postgres (≈ 3 TB) traffic, ≈ 16 TB × $0.02 | $310 |
| **Total** | **≈ $5,100** (≈ $4,200 with 1-year commitments on the three compute lines) |

- **Biggest line and why:** data transfer (≈ 45 %: egress $1,950 + cross-zone $310), not servers: 55k small responses a second add up to ≈ 22 TB a month. The database is next (≈ 27 % with its disk) because a cart becomes an order and must survive a zone loss. The lever is fewer and smaller responses (derive the badge from the last cart response instead of polling it, compress), not a smaller instance.
- **What changes at 10×:** 550k req/s ⇒ 75 + 20 = 95 vCPU ⇒ N = ceil(95 ÷ 4.27) = 23 → 24 × `c7g.2xlarge`. Enrichment at 5M lookups/s (section 10's first break) stays in-process, ≈ 1 vCPU, if the local cache is fed by a change feed instead of per-node refreshes. Redis at 550k ops/s is 5.5 cores ⇒ cluster mode, 6 shards (8 if carts also grow 10×: ≈ 240 GB of RAM). 50k writes/s ≈ 100k IOPS is past gp3 and past one primary ⇒ shard carts by user id, ~10 shards each at today's 5k writes/s: the first real architecture change. Egress scales to ≈ $19.5k a month.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Two small relational tables, `carts` and `cart_items` (primary key `(cart_id, sku)`): 50M live carts ≈ 100 GB, ~5k writes/s and ~50k reads/s at peak (section 4). Redis answers the reads, so the database sees the writes and the cache misses.
- One invariant must hold under races: one active cart per user (section 8). Merge and checkout are multi-row transactions with `FOR UPDATE` (9.3, 9.6), and checkout also decrements stock and inserts the order.
- The owner needs read-your-writes; marketing needs SQL over carts ("active and untouched for 24 h").

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the partial unique index `carts_one_active_per_user … WHERE status = 'active' AND user_id IS NOT NULL` makes a second active cart impossible, `carts_abandoned_idx` covers only active carts, `INSERT … ON CONFLICT` merges lines, and cart, stock and order commit in one transaction |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | no partial indexes: a generated column `IF(status = 'active', user_id, NULL)` with a unique index enforces the same rule (NULLs never collide), the abandoned-cart index has to cover every status, and `ON DUPLICATE KEY UPDATE` replaces `ON CONFLICT`. The real change is the `UUID` primary key: a random value as InnoDB's clustered key splits pages across 50M carts. In its favour: key `(cart_id, sku)` keeps a cart's ≤ 100 lines together on disk |
| DynamoDB | ✔ for the cart alone, ✖ for checkout | one item per cart (2 KB, far under 400 KB), native TTL for guest expiry, and 5k writes/s is easy; but "one active cart per user" needs a second lock item written in a transaction, abandoned-cart analytics need a GSI or an export, and a full 100-line checkout touches ≥ 102 items (100 stock rows, the order, the cart) against the 100-item transaction limit |
| Redis (ElastiCache / MemoryDB) | ✖ as the record, ✅ chosen as the cache | a Redis incident would lose every cart and there is no SQL for analytics (9.1); as the cache-aside copy of the hot ~5M carts it answers the 50k reads/s. MemoryDB would make it durable, but not queryable or transactional with the order |

**Pick:** PostgreSQL on RDS Multi-AZ (the `db.r7g.2xlarge` of section 15) as the system of record, because the partial unique index turns "one active cart per user" into a database guarantee and checkout is one ordinary transaction across cart, stock and order. ElastiCache Redis (`cache.r7g.xlarge`) holds derived, rebuildable cart copies.

**If it were MySQL instead:** the same two tables on the same instance shape, with three edits: a generated column plus unique index for the one-active-cart rule; a `BIGINT` auto-increment primary key, with the random 128-bit cart id (the guest cookie, section 10) as a unique `BINARY(16)` key so inserts append; and an abandoned-cart index over all 50M rows instead of the active ones only (still small next to 100 GB). The guarantees do not change, only three schema workarounds to document.

**What would flip the choice:** ~50k cart writes/s (the 10× case in section 15) outgrows one primary, and the answer is sharding carts by user id, not another engine. DynamoDB becomes attractive only if checkout stops needing one transaction with stock (a saga instead) and the abandoned-cart queries move to an export.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 500k writes/s, 5M reads/s; 5B carts (about one per internet user), 10 TB; API 950 vCPU | the one Postgres primary: 500k commits/s ≈ 1M IOPS, 50× its ~10k durable writes/s and 62× the 16k IOPS cap of `gp3`; 10 TB is also past "a few TB comfortable". Redis needs ~80 shards (1.7 TB hot set ÷ 22 GB usable) | as in section 16, shard carts by `user_id`: ~100 Postgres shards, each today's `db.r7g.2xlarge` at 5k writes/s and 100 GB, so the one-active-cart index stays local; ~80 Redis shards; 225 `c7g.2xlarge` (950 ÷ 4.27 = 223 → 225). Give up: checkout as one transaction with stock (stock cannot live in 100 shards: a conditional decrement, then the order, under the idempotency key) and cross-user queries (abandoned carts move to a stream) | ≈ $510k; egress (≈ $195k) is still the biggest line |
| **1,000×** | 5M writes/s, 50M reads/s; 50B carts (six per person alive: a thought experiment), 100 TB; API 9,500 vCPU | running one fleet: 1,000 primaries and standbys, ~800 Redis shards and ~2,230 API nodes ≈ 5,800 machines, past the cheat sheet's "a node dies about every hour" at 5,000; one shard map, one deploy, one resharding for all of it | independent **cells**: ~100, each today's 10× stack (~10 Postgres shards, 8 Redis shards, 24 API nodes, 50k writes/s) for a slice of users behind a thin `user_id → cell` router; a guest cart routes by its cookie hash and is copied into the user's cell at login. Give up: global cart queries and one analytics database (each cell streams to a warehouse); moving a user between cells is a copy job | ≈ $5.1M, linear |
| **10,000×** | 50M writes/s ≈ 4.3 trillion a day, 500M reads/s; 500B carts (sixty per person alive), 1 PB; ≈ 2.2 Tbit/s of responses | the bill and the bytes, not a component: 95,000 vCPU ≈ 22,000 API nodes and ≈ $19.5M of egress alone, to persist a cart on every click | change the problem: the device owns the working cart and syncs a per-line diff only when idle, on login, tab close and checkout (one write per session, 20–100× fewer; the last-write-wins per line of 9.2 already allows it); the badge comes from the last response, not a poll (section 15's own lever); guest carts that never sync are never stored. Give up: live cross-device carts mid-session, abandoned-cart emails for guests | ≈ $51M if nothing changes; the 20–100× cut in writes and reads is what lowers it |

**What stays the same at every scale:** `PUT` sets an absolute quantity (a retry or a replayed batched diff is harmless) and the cart → order boundary (the order is an immutable snapshot written under an idempotency key).

**Say it in an interview:** "At 100× the single Postgres primary is 50× past its write limit, so I shard carts by user id and cluster Redis. At 1,000× the pain is running about 5,800 machines as one fleet, where something dies every hour, so I split users into independent cells behind a router. At 10,000× the bill is the break, so I stop persisting every click: the device keeps the cart and syncs when idle, at login and at checkout."
