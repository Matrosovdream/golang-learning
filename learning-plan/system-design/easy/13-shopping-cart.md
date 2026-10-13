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
