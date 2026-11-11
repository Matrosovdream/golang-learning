# 28 · Booking Calendar for Rentable Units

`🟢 easy` · *Topics:* no double booking as a database constraint (range exclusion), items vs physical units, pooled-unit assignment and fragmentation, holds with expiry, blocks and cancellations as bookings, month-view reads at scale, sharding by the conflict domain · *Builds on:* [13 Shopping Cart](13-shopping-cart.md) (reserve on add vs enforce at checkout), [05 In-Memory Cache with TTL](05-kv-cache-ttl.md) (lazy vs active expiry), [15 Job Scheduler](15-job-scheduler.md) (reaper loop)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [27 Deadlines & Escalation](27-deadlines-escalation.md) · Next: [29 Price & Availability Engine](29-price-availability-engine.md)

---

> 🌱 **Seed (honeydolist-site):** an equipment-rental business. A *machine* has one or more *units* (PLAN step 53, migration 0074); a booking belongs to one unit and the database refuses an overlap **per unit**: `EXCLUDE USING gist (unit_id WITH =, daterange(starts_on, ends_on, '[]') WITH &&) WHERE (status IN ('held','confirmed','out'))`. A booking written with no unit gets the first free active unit from a `BEFORE INSERT` trigger that first takes `pg_advisory_xact_lock(equipment_id)`. A *block* (internal use, maintenance; step 52, migration 0073) is just a booking with a `kind`. A cancelled order frees its dates (step 7v.4); optional holds on a request answer `409` "those dates are gone". Step 56 measured 200,000 machines, 280,000 units and 4.7 million bookings: per-machine reads took 0.6–2 ms, the month view across the whole fleet 238 ms until a gist index on the dates alone (0078) brought it to 70 ms.
> **Here, at scale:** a rental marketplace with 10M units owned by 100k owners, 500M bookings a year, 3k booking attempts/s at a holiday peak, and any owner's month calendar in under 100 ms.

## 1. The task

> "Design the booking calendar for a rental marketplace. Owners list items (a trailer, a camera, a
> cabin), each backed by one or more physical units. Customers pick dates. Owners can block dates for
> themselves. Two customers must never get the same unit on overlapping days."

The interesting part is not the calendar widget. It is *where the no-overlap rule lives*, what an
"item with 3 identical units" really means, and what a hold costs the people who are not holding.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What does a customer book: an item or a specific unit? | Decides who picks the unit and where the rule is enforced. | The **item**; the system picks a unit. Owners may pin a unit when they book by hand. |
| Granularity and the end day? | Same-day turnover, off-by-one bugs. | Whole days, **half-open** `[start, end)`: a unit returned on the 5th can go out on the 5th. (honeydolist uses inclusive `[]` days: a Tuesday return and a Tuesday pick-up clash.) |
| Do we hold dates while the customer pays? | Hold rows hide inventory from other people. | Yes, a 10-minute hold, then confirm. |
| Owner blocks (own use, maintenance)? | Same inventory, different actor. | Yes, including open-ended ("until I release it"). |
| Scale? | Store, partitioning, read path. | 10M units, 100k owners, 500M bookings/year, 3k attempts/s peak. |
| Consistency? | Writes must be exact; reads may lag. | Strong on writes. A search page may be a few seconds stale; the booking itself never is. |
| Time zones? | "Tonight" differs per place. | Dates are the unit's local calendar day; no overbooking, ever. |

## 3. Requirements

**Functional**
- Check availability of an item for a date range; list free units.
- Book (hold, then confirm), cancel, block, release a block.
- Owner calendar: a month across all of an owner's units; one unit's history.
- Bookings and blocks never overlap on a unit.

**Non-functional**
- Zero double bookings, even with two writers in the same millisecond and even for SQL run by hand.
- Booking p99 under 200 ms; availability reads p99 under 50 ms; owner month view under 100 ms.
- A hold that is never confirmed must free its dates without anyone noticing.

**Out of scope**
- Pricing and payment ([29](29-price-availability-engine.md), [30](30-invoicing.md)), seat-level ticketing (medium backlog 06), hourly slots (extension).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Confirmed bookings written | ~16/s average | 500M ÷ 31.5M s/year |
| Booking attempts | ~160/s average, **3k/s peak** | ~10 attempts per success (conflicts, abandoned holds); the peak is a long-weekend release, ~19× the average |
| Availability checks | **~90k/s peak** | each attempt follows ~30 page views and searches (3k × 30) |
| Bookings per year | ~200 GB | 500M × ~0.4 KB (columns plus six indexes) |
| Live rows (held/confirmed/out) | ~45M | 1.37M created per day × ~33 days (30-day lead + 3-day stay) |
| Hot set (live + finished within 90 days) | ~168M rows ≈ 67 GB | 1.37M/day × 123 days × 0.4 KB |
| Utilisation | ~41% | 500M × 3 days ÷ (10M units × 365 days) |
| One owner's month (5,000 units) | ~20k bookings | 5,000 × (30 days × 0.41 ÷ 3 days per booking ≈ 4.1) |
| Peak simultaneous holds | up to 2.7M | 3,000/s × 900 s, if every attempt held for 15 min (6% of live rows) |

What the numbers tell us: the *write* path is tiny (16/s) and the hot set fits one Postgres box
(67 GB). The problems are (1) **correctness under races**, (2) **reads**, 90k/s of availability
checks, and (3) holds, which are cheap as rows but expensive as hidden inventory. A "whole
marketplace month view" would be 10M × 4.1 = 41M rows. It is never a query: calendars are always
scoped to an owner.

## 5. Plan

1. The rule: where "no overlap" lives. 2. The model: units, bookings, blocks, one status column.
3. Picking a unit from a pool. 4. Holds and expiry. 5. Blocks and cancellations.
6. Reads: availability search, the month view. 7. Partitioning: the conflict domain is one unit.

## 6. High-level design

```
 client ──► LB ──► Booking API ──► Postgres primary (owner shard)
                     │  │            units, bookings(hot)  ◄── EXCLUDE (unit_id =, stay &&)
                     │  │            bookings_archive (monthly partitions, no constraint)
                     │  └──► outbox ──► relay ──► availability index (Redis bitmaps / search)
                     │                       └──► notifications (task 12)
                     ├──► Redis: calendar cache  cal:{owner}:{yyyymm}:{ver}
                     └──► read replicas: month view, history
 reaper (every 30 s): held ∧ hold_expires_at < now()  →  expired      (task 15 pattern)
```

**A booking:** one transaction on the owner's shard: expire stale holds on the candidate units, choose
a free unit, `INSERT … status = 'held'`, commit. The constraint is the judge; the choice logic is a
courtesy. **Confirm** flips `held → confirmed` if the hold is alive. **Reads:** availability from the
derived index (seconds stale), the owner's month from a replica or the cache.

## 7. API

```
GET  /items/{id}/availability?from=2026-07-10&to=2026-07-13   → 200 { free_units: 2, days: [{ d, free }] }        read model, may lag seconds
POST /items/{id}/bookings   Idempotency-Key  { from, to, unit_id? }
                              → 201 { id, unit_id, status: "held", hold_expires_at }
                              → 409 problem+json "booking-conflict" { free_units: 0, next_free_from? }
POST /bookings/{id}/confirm                                   → 200 | 409 "hold-expired"
POST /bookings/{id}/cancel                                    → 200 (dates free at once)
POST /units/{id}/blocks     { from, to | open_ended, kind: "internal"|"maintenance", note }
                              → 201 | 409 "booking-conflict" naming what holds the dates
POST /blocks/{id}/release                                     → 200 (ends today, in the unit's zone)
GET  /owners/{id}/calendar?month=2026-07&unit=…               → 200 { bars: [{ unit, from, to, kind, status }] }
```

The booking `POST` carries an `Idempotency-Key` ([13](13-shopping-cart.md) §9.2): a retry returns the
same hold. Unit history is a paged list ([23](23-paginated-lists.md)).

## 8. Data model

```sql
CREATE EXTENSION IF NOT EXISTS btree_gist;          -- lets a GiST index/constraint mix  =  with  &&

CREATE TABLE units (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id  BIGINT  NOT NULL,
    item_id   BIGINT  NOT NULL,
    label     TEXT    NOT NULL,                      -- "Main", "Yellow (cracked)"
    is_active BOOLEAN NOT NULL DEFAULT true,         -- retired: out of rotation, history kept
    UNIQUE (id, item_id)
);

CREATE TABLE bookings (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id        BIGINT NOT NULL,
    item_id         BIGINT NOT NULL,
    unit_id         BIGINT NOT NULL,
    kind            TEXT   NOT NULL DEFAULT 'customer' CHECK (kind IN ('customer','internal','maintenance')),
    status          TEXT   NOT NULL CHECK (status IN ('held','confirmed','out','returned','cancelled','expired')),
    stay            DATERANGE NOT NULL,              -- half-open [start, end); open-ended = upper bound NULL
    customer_id     BIGINT,
    hold_expires_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (unit_id, item_id) REFERENCES units (id, item_id),
    CHECK (NOT isempty(stay)),
    CHECK (kind = 'customer' OR customer_id IS NULL),            -- a block has no customer
    CHECK (status <> 'held' OR hold_expires_at IS NOT NULL),
    CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (unit_id WITH =, stay WITH &&)
        WHERE (status IN ('held','confirmed','out'))             -- only these block
);
CREATE INDEX bookings_owner_stay_idx ON bookings USING gist (owner_id, stay);          -- the owner's month
CREATE INDEX bookings_item_stay_idx  ON bookings USING gist (item_id, stay)
    WHERE status IN ('held','confirmed','out');                                         -- "free units of this item"
CREATE INDEX bookings_holds_idx      ON bookings (hold_expires_at) WHERE status = 'held';  -- the reaper
```

The constraint creates its own GiST index, which also answers "does anything overlap this range on
this unit". The `kind`/`customer_id` check mirrors honeydolist's `rental_bookings_block_is_free`: a
block never carries an order, a request or a customer.

## 9. Deep dives

### 9.1 Where "no overlap" lives

Two customers click Book for the same unit within one millisecond. Whichever layer decides, it has to
see both writes.

| Option | Pros | Cons |
|---|---|---|
| **Check, then insert** in app code | trivial | the classic race: both checks pass, both inserts succeed |
| `SELECT … FOR UPDATE` on the unit row, check, insert | correct; serialises per unit | every writer path must remember to do it; a manual SQL fix or a migration bypasses it |
| **Exclusion constraint** | declarative; the database refuses it for *every* writer; concurrent inserts are serialised by the index | the error must be mapped; a bad predicate silently weakens the rule |

**Choice and why: the exclusion constraint** (§8). `unit_id WITH =` plus `stay WITH &&` reads
"no two rows with the same unit and overlapping ranges", and `WHERE (status IN …)` says which rows
count: a cancelled or expired booking leaves the predicate, so its dates are free the instant the
status changes, with no delete. A violation raises SQLSTATE `23P01`, which the API turns into
`409 booking-conflict`. honeydolist tests exactly this: 12 writers racing for 5 units book exactly 5.

Two sharp edges. **Inclusive vs half-open:** `'[]'` makes a Tuesday return and a Tuesday pick-up
collide; the half-open default `[)` allows same-day turnover. Pick one on day one: changing it later
rewrites every range. **The predicate is the rule:** leave "held" out and holds stop protecting dates.

### 9.2 An item with N identical units: who gets which

A trailer item with units A and B. The customer asked for the *item*, so someone chooses the unit.

| Option | Pros | Cons |
|---|---|---|
| **Assign at booking, first-fit** (honeydolist's trigger: first free active unit by `sort_order`) | simple; history says exactly which unit; no schema change for old writers | fragments the calendar over time |
| **Assign at booking, best-fit** (the free unit whose surrounding gaps are tightest) | keeps long gaps open for long requests | slightly more logic; still greedy |
| **Late binding**: count concurrent bookings per day against a capacity, assign the unit at pick-up | no fragmentation; units are fungible | you lose "who had *this* unit"; needs per-day counters; pick-up can still fail if a unit is broken |

Fragmentation, concretely (nights, half-open). A is busy `[Jun 1, Jun 5)`, B is busy `[Jun 6, Jun 10)`.
A request for `[Jun 4, Jun 8)` finds A busy on night 4 and B busy on nights 6–7: **no single unit is
free**, yet at most one unit is busy on any night, so a per-day *count* says "fits". honeydolist
records this honestly as "not done: reassigning units to fit a long request into a gap that no single
unit has". The cure is a bounded rebalance: move *one* future, unpinned booking to another unit and
retry. Moving B's `[Jun 6, Jun 10)` onto A (free from Jun 5) frees B completely. Do it inside the
same transaction, under the item's lock.

**Choice and why: assign at booking with best-fit, plus one rebalance attempt.** Owners care which
unit ("the yellow one is cracked") and maintenance history attaches to a unit. Late binding is right
for car-class pools where customers do not care; see §14. Choosing under contention:

- honeydolist takes `pg_advisory_xact_lock(equipment_id)`: writers of one item queue, fine at tens a day.
- At 3k attempts/s a hot item should not serialise. Select candidates with `… FOR UPDATE OF u SKIP
  LOCKED`: concurrent writers lock *different* free units. The constraint stays the safety net. Cost: a
  writer may see "no free unit" only because a rival holds the last one and may yet roll back.

Best-fit in one sentence: for each free unit, look at the nearest booking before and after the wanted
range and pick the unit whose two gaps add up to the least (a missing neighbour counts as wide open).
It is a few lines over the unit's sorted bookings, and a good table-driven test in Go.

### 9.3 Holds: TTL rows, and what they cost everyone else

Payment takes minutes. Without a hold, two people pay for the same dates and one gets a refund.

| Option | Pros | Cons |
|---|---|---|
| No hold; book on payment success | no hidden inventory | the loser pays, then is refunded: worst UX |
| honeydolist: optional hold on a request, **no TTL**, released when staff mark it lost or an order is made | fine for a human-handled quote at tens a day | an abandoned hold blocks dates for ever; useless at 3k/s |
| **Hold row with `hold_expires_at`**, lazy expiry plus a reaper | protected by the same constraint; expiry needs no extra system | an expired-but-unswept hold still *blocks*, so expiry must be handled in the booking path |
| Redis `SET NX PX` hold, DB row on confirm | native TTL | two systems; the hold is not covered by the constraint |

**Choice and why: hold rows in `bookings`.** An unswept expired hold would block the unit, so the
booking transaction first runs
`UPDATE bookings SET status='expired' WHERE unit_id = ANY($candidates) AND status='held' AND hold_expires_at < now() AND stay && $2`
(**lazy expiry**, as in [05](05-kv-cache-ttl.md)), and a reaper every 30 s sweeps the rest in batches
with `FOR UPDATE SKIP LOCKED`. Always compare against the *database* `now()`, never app clocks.
Confirm is one conditional update:
`UPDATE bookings SET status='confirmed', hold_expires_at=NULL WHERE id=$1 AND status='held' AND hold_expires_at > now()`.
Zero rows means `409 hold-expired`; if payment already succeeded, try to re-hold the same dates
atomically, else refund.

The real cost is social: at a peak up to 2.7M units sit "held" (§4), most abandoned, and 20 curious
clicks make a 20-unit item look sold out for 10 minutes. Mitigate with a short hold (extend once when
the payment page opens), a per-customer cap on holds, and "may free up" instead of "sold out".

### 9.4 Blocks, cancellations and open-ended blocks

honeydolist's lesson (steps 52, 7v.4): no second table and no customer-less order for "I use it
myself". A block is a booking with `kind` and no customer, so the constraint, the calendar and every
"is it free" check already treat it correctly; a customer-less *order* would leak into invoices and reports.

- **Cancel** is a status change; the row leaves the constraint's predicate and the dates are free at
  once. honeydolist found a gap here: a free booking made on the machine's page belonged to no order,
  so no route cancelled it, and its dates blocked the site until someone edited the database. Every
  booking source needs a cancel path.
- **A block that overlaps a customer booking** is refused with the clash named (`409`), never silently
  stacked. honeydolist considered an admin override and did not build it: release or cancel what is in
  the way first. Who compensates the customer is policy, out of scope.
- **Open-ended** ("until I release it"): honeydolist stores `2999-12-31` plus an `open_ended` flag so
  no reader changed. At scale prefer an unbounded range (`daterange(start, NULL)`: `&&` and the
  constraint handle infinity natively) and test `upper_inf(stay)` in readers. **Release** sets the end
  to *today in the unit's zone*.

### 9.5 Reads: availability search and the owner's month

Per-item reads are index lookups (honeydolist: 0.6–2 ms with 4.7M bookings). Two reads do not scale
that way:

| Read | Problem | Fix |
|---|---|---|
| **Search**: "which of 1M items in this city are free Jul 10–13" | can't probe `bookings` per item for thousands of items per query | a **derived availability index**: per item, a bitmap of days with ≥1 free unit (365 bits ≈ 46 B), updated from `booking.*` outbox events into Redis or the search engine. Stale by seconds; the constraint is the truth |
| **Owner month view** | honeydolist's whole-fleet month was 238 ms until a gist on dates alone (0078); at marketplace size a month across all owners is 41M rows | always scope by owner (`gist (owner_id, stay)`), serve from a replica, return compact bars, let the client virtualise the unit rows |

Cache the month view as `cal:{owner}:{yyyymm}:{ver}` where `ver` is a per-owner counter in Redis
(`INCR calver:{owner}` on every write). A write invalidates by bumping the version; stale keys expire
by TTL. This avoids tracking which cache keys a booking touches.

**Rule: reads may lie a little, writes may not.** A customer who sees a stale "available" and clicks
Book gets `409`, and the UI says "those dates just went". The constraint makes staleness harmless.

### 9.6 Partitioning and sharding: the conflict domain is one unit

Two units never conflict, so the unit (or its owner) is the natural shard key.

| Option | Pros | Cons |
|---|---|---|
| One primary + replicas | simplest; the hot set is ~67 GB | write headroom and blast radius are one box |
| **Shard by `owner_id`** | constraint, calendar and holds are single-shard; no cross-shard transactions | a giant owner is a hot shard (put them on their own); reshard needs a directory |
| Hash-partition `bookings` by `unit_id` | the exclusion constraint survives on partitioned tables from Postgres 17 *if every partition-key column is compared with `=`*; `unit_id WITH =` is | needs a recent Postgres; keep your PK and indexes partition-aware |

**Choice and why: one primary now, shardable keys.** Keep the *hot* table (live + 90 days) apart from
`bookings_archive`, partitioned by month with no constraint (safe: nobody can book the past); a nightly
job moves finished rows. When one box is not enough, shard by `owner_id`, which every query carries.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** (1) one viral item: 30k attempts/s on one item serialise on its
  units. Use `SKIP LOCKED` selection, put a waiting room or the limiter ([03](03-rate-limiter.md)) in front
  of the hot item, and show "high demand". (2) Search reads: the availability index moves from Redis
  to a sharded search tier. (3) Hold churn: bloat on `bookings` from expired rows; tune autovacuum and
  archive aggressively.
- **Single points of failure:** the shard primary (failover, synchronous replica if a confirmed
  booking must never be lost); the reaper is not critical, lazy expiry keeps correctness.
- **Redis/search down:** availability falls back to a rate-limited per-item query; booking is
  unaffected, since correctness never depended on them. **Postgres down:** no bookings; serve cached
  calendars read-only with a banner and accept no holds elsewhere.
- **Clocks and zones:** hold expiry uses the DB clock; "today" for release uses the unit's zone
  (honeydolist counts its business day in New York time).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| exclusion constraint on `(unit_id, stay)` | check-then-insert or app locks | the rule holds for every writer, including hand-run SQL | a constraint error to map; a wrong predicate is a silent bug |
| half-open `[start, end)` | inclusive `[]` | same-day turnover; no off-by-one on the end | a convention to document |
| assign a unit at booking, best-fit + one rebalance | late binding | "who had exactly this unit"; maintenance history per unit | greedy assignment can still fragment |
| holds as rows with TTL + lazy expiry | Redis holds or no holds | holds covered by the constraint; no extra system | hidden inventory; needs the reaper and a hold cap |
| derived availability index for search | probing `bookings` per item | search stays fast at 90k/s | seconds of staleness, outbox plumbing |
| one primary now, shard by owner later | sharding on day one | simplicity; the hot set is 67 GB | a planned migration if growth beats the plan |

## 12. Check yourself

1. Why is "check availability, then insert" wrong, and what exactly does `EXCLUDE USING gist (unit_id WITH =, stay WITH &&) WHERE (status IN (…))` enforce?
2. Units A and B: A busy `[Jun 1, Jun 5)`, B busy `[Jun 6, Jun 10)`. A new request is `[Jun 4, Jun 8)`. Why does a per-day count say it fits while no unit can take it, and what do you do?
3. An unswept expired hold sits on a unit. Why would that break bookings, and which two mechanisms fix it?
4. How do cancelling a booking, blocking a unit and releasing an open-ended block each interact with the constraint?
5. Why must the owner month view always be scoped by owner, and how do you cache it without tracking keys?

<details><summary>Answers</summary>

1. Two requests can both pass the check before either inserts. The constraint refuses any two rows
   with the same `unit_id` and overlapping `stay`, counting only held, confirmed and out rows; the
   index serialises concurrent inserts and the loser gets `23P01`, whoever wrote the row.
2. At most one unit is busy on any night, so a count never reaches 2, but A blocks night 4 and B
   blocks nights 6–7, so neither is free for the whole stay. Rebalance: move B's `[Jun 6, Jun 10)` to A
   (free from Jun 5), give the request to B, all in one transaction under the item's lock. Never move
   a pinned booking.
3. The constraint reads `status`, not the clock, so an expired but still-`held` row keeps blocking.
   Lazy expiry in the booking transaction guarantees correctness; the reaper only tidies.
4. Cancel sets `cancelled`, so the row leaves the predicate and the dates are free at once. A block is
   an insert that must pass the same constraint: a clash is a `409`. Release shrinks `stay` to end
   today (a block that has not started is cancelled instead); shrinking cannot violate the constraint.
5. A month across all owners is ~41M rows, never a valid query; scoped by `(owner_id, stay)` it is
   ~20k for a 5,000-unit owner. Cache under `cal:{owner}:{yyyymm}:{ver}` and `INCR` a per-owner
   version on every write: a write invalidates by changing the key, old versions expire by TTL.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (transactions, `SELECT … FOR UPDATE SKIP LOCKED`), [59 Production DB patterns](../../59-production-database-patterns.md) (advisory locks, partial indexes, concurrency tests), [41 API design](../../41-api-design-evolution.md) (`409` problem details, idempotency keys), [44 Background jobs](../../44-background-jobs-queues.md) and [15 Sync & context](../../15-sync-context.md) (the reaper loop with a ticker and `ctx`), [38 Caching](../../38-caching-patterns.md) (versioned keys), [12 Errors](../../12-errors.md) (`errors.As` on driver errors).
- Mapping the constraint error:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23P01" { // exclusion_violation
	return ErrDatesTaken // → 409 booking-conflict
}
```

- Test it as honeydolist does: N goroutines book an item with K units for the same dates; exactly K
  succeed. Use a real Postgres, not a mock.
- Packages: `pgx/v5` + `pgconn`, `go-redis`, `time` (use `civil`-style `YYYY-MM-DD` dates, not `time.Time`, for day ranges).

## 14. Extensions

- **Seat-level ticketing** (a seat is a unit, an event is the range): contention on thousands of seats at once; the medium backlog's ticket booking (06).
- **Hourly slots:** `tstzrange` instead of `daterange`; turnaround buffers by padding the stored range while showing the real one.
- **Bundles:** a trailer *and* its truck atomically; lock units in `unit_id` order to avoid deadlocks.
- **External calendars (iCal feeds):** import as blocks; the hard part is a booking that arrives *after* your own for the same dates.
- **Late binding for car-class pools** with per-day capacity counters and assignment at pick-up.
- **Service due:** honeydolist records return condition, meter hours and `service_every_days` per unit (migration 0075); turn "due" into an automatic maintenance block through the deadlines machinery ([27](27-deadlines-escalation.md)).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Booking + availability API (Go, stateless) | 90k availability checks/s + 3k booking attempts/s (160/s avg, so the peak is ~19×) | CPU | checks 90k × 0.15 ms (one pipelined Redis call + JSON) = 13.5 vCPU; attempts 3k × 0.5 ms (the DB time is waiting, not CPU) = 1.5; total 15 vCPU. N = ceil(15 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(7.03) = 8 → 9 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 9, three per zone | CPU only: ~10 KiB per connection × ~10k connections ≈ 100 MiB per node, so `c`. 42 % busy at peak, 62 % with a zone down. Many mid-size nodes keep the blast radius small. The average needs ~0.8 vCPU, so autoscale down to a 3-node floor outside long weekends |
| Postgres (bookings, units; GiST exclusion) | 3k attempts/s ⇒ ~3k hold inserts/s, and later ~3k expiries/s ≈ 6k writes/s (a conflict still writes a dead tuple); hot set 67 GB (168M rows); archive 200 GB/yr | memory, then IOPS | RAM ≥ 67 GB ⇒ 128 GiB. IOPS ≈ 6k writes × 2 (assumed) = 12k + ~1k miss reads ⇒ `gp3` 14,000 (cap 16,000) on 500 GB (hot 67 GB + ~1 year of archive 200 GB). CPU: 3k × ~1.5 ms (GiST probe, `SKIP LOCKED` pick, insert into 4 indexes) + expiries 3k × 0.5 ms ≈ 6 vCPU of 16 | `db.r7g.4xlarge` (16 vCPU / 128 GiB), RDS Multi-AZ (self-managed: `r7g.4xlarge`) | primary + sync standby; 1 replica `db.r7g.2xlarge` for month views and history | the hot set (67 GB) is bigger than `r7g.2xlarge`'s 64 GiB, so memory picks the node, and 16 vCPU come with it (38 % busy at the peak). Multi-AZ because a confirmed booking must not be lost. 12k IOPS is near gp3's cap: past that, `io2` or shards |
| Availability index + month-view cache (Redis) | ~90k checks/s × ~2 ops = 180k ops/s, + ~18k/s from the outbox updater | single-core ops/s, not memory | ~7M items (assumed 10M units ÷ 1.4) × (46 B bitmap + ~70 B key) ≈ 0.8 GB + month cache ≈ 3 GB (assumed) ≈ 4 GB × 1.7 ≈ 7 GB, so ~2.3 GB per shard. Ops: ~200k ÷ 100k per core = 2 cores ⇒ 3 shards at ~67 % | `cache.m7g.large` (2 vCPU / 8 GiB), ElastiCache cluster mode | 3 shards × (primary + replica) = 6 | the limit is one core per shard, memory is ~30 % of 8 GiB, so `m` rather than `r`; a lost cache degrades search only, since the constraint is the truth |
| Workers: reaper, outbox → index updater, nightly archive | ~6k events/s at peak; the reaper sweeps up to ~90k rows per 30 s | CPU, tiny | 6k × 0.15 ms ≈ 0.9 vCPU. N = ceil(0.9 ÷ 1.07) = 1 → 3 | `c7g.large` | 3 | small; the reaper is not critical (lazy expiry keeps correctness) |

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 9 × `c7g.xlarge` (9 × $105) | $950 |
| Workers: 3 × `c7g.large` | $160 |
| Redis: 6 × `cache.m7g.large` (6 × ≈ $120) | $720 |
| Postgres primary: `db.r7g.4xlarge` ≈ $1,240, Multi-AZ doubles | $2,480 |
| Replica: `db.r7g.2xlarge` | $620 |
| `gp3` 500 GB + 11k extra IOPS ($40 + $55 = $95) × 2 for the standby, + $95 for the replica | $290 |
| ALB ($16 + ~10 LCU on average) | $80 |
| Egress: ≈ 7.6 TB × $0.09 (4.8k checks/s average × ~0.6 KB, assumed) | $680 |
| **Total** | **≈ $6,000** (≈ $4,500 with a 1-year plan on the steady baseline: database, Redis, workers, 3 API nodes) |

- **Biggest line and why:** the Postgres primary (~41 %; ~57 % with replica and volumes), because the 67 GB hot set forces 128 GiB and a confirmed booking must survive a zone loss. Reads never reach it (Redis answers 90k checks/s), so the lever is write churn, not a smaller node: holds are half its writes (3k inserts + 3k expiries/s), and a shorter hold, a hold cap per customer and a waiting room for hot items cut CPU and IOPS together.
- **What changes at 10×:** 900k checks/s + 30k attempts/s ⇒ 135 + 15 = 150 vCPU ⇒ N = ceil(150 ÷ 4.27) = 36 → 36 × `c7g.2xlarge` (≈ $7.6k). 1.8M Redis ops/s is ~30 shards, which is the lesson's move to a sharded search tier (OpenSearch Service). 60k writes/s ≈ 120k IOPS is far past gp3 and one primary, so shard by `owner_id` into ~10 shards of today's shape (6k writes/s, 67 GB hot, ≈ 10 × $2,670 ≈ $27k).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The rule that matters is "no two live bookings of one unit overlap", and its conflict domain is a single unit (sections 8 and 9.6).
- Writes: 3k attempts/s at peak means ~3k hold inserts/s and later ~3k expiries/s, ~6k writes/s on a 67 GB hot set; ~200 GB a year is appended (sections 4 and 15).
- Reads that reach the database are per-unit overlap probes, "free units of this item", and one owner's month (~20k bookings for 5,000 units). The 90k availability checks/s are answered from a derived index.
- Holds and cancellations are status changes, so rows are updated often and the live set is only ~45M of 500M a year.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `EXCLUDE USING gist (unit_id WITH =, stay WITH &&) WHERE (status IN ('held','confirmed','out'))` makes the rule hold for every writer, and a cancelled or expired row leaves the predicate with no delete; `daterange` gives half-open stays and open-ended blocks (upper bound `NULL`); `gist (owner_id, stay)` serves the month; `FOR UPDATE OF u SKIP LOCKED` picks a unit under contention (9.2) |
| MySQL (RDS / Aurora) | ✔ works, with a heavy workaround | no range type, no exclusion constraint, no partial index. Either keep a night table with unique `(unit_id, night)` for blocking bookings only (~1.5B night rows a year at 3 nights per stay, and ~4 rows written per hold instead of 1), or lock the unit row, run an overlap query, then insert, which leaves the rule in code that every writer must remember (the con in 9.1) |
| DynamoDB | ✖ poor fit | a conditional write guards one item, not a range; you would model one item per unit-night and run a transaction over a stay's nights (limit 100 items, so long stays and open-ended blocks need caps or a horizon), and the owner's month needs a GSI |
| Redis (ElastiCache) | ✔ as a derived index and cache, ✖ as the record | the availability bitmaps and `cal:{owner}:{yyyymm}:{ver}` are rebuildable from `bookings`; a `SET NX PX` hold would sit outside the constraint (9.3) |
| OpenSearch | ✔ only as a derived search tier at 10× | section 10's move for "which items in this city are free"; never the source of truth |

**Pick:** PostgreSQL on RDS Multi-AZ, `db.r7g.4xlarge` with a `db.r7g.2xlarge` replica for month views and history (section 15). It is chosen for the exclusion constraint, which turns the hardest rule into a database guarantee (9.1); for range types and GiST indexes, which answer the calendar queries directly; and for `SKIP LOCKED` unit selection. `bookings` is the single system of record; the Redis bitmaps, month cache and any search tier are derived through the outbox and can be rebuilt from it.

**If it were MySQL instead:** the same units and bookings tables plus one of two additions. With the night table, a hold or expiry touches 4 rows, so the peak is ~24k row writes/s instead of ~6k (an estimate, on a node planned around 12k IOPS), and an open-ended block needs a far-future sentinel. With lock-then-check, take the unit row lock first so writers queue instead of deadlocking on gap locks under `REPEATABLE READ`, then check overlap with a B-tree on `(unit_id, status, start)`, cheap because one unit has only a few dozen bookings. InnoDB has no vacuum, which would ease the hold-churn bloat that section 10 warns about, but it does not pay for the extra rows.

**What would flip the choice:** the unit is the conflict domain, so ~10× (30k attempts/s, 60k writes/s, section 15) means sharding by `owner_id`, still on Postgres. Another engine is justified only if bookings must be accepted in several regions at once with no single primary per owner, which suggests per-unit ownership in a DynamoDB-style store, or if search outgrows Redis, which points to OpenSearch as a derived tier.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 300k attempts/s, 9M availability checks/s; 1B units, 50B bookings a year ≈ 20 TB, hot set ≈ 6.7 TB; ~600k writes/s (300k hold inserts + 300k expiries); ~20M Redis ops/s; ~1,500 vCPU of API | the Postgres primary: 600k writes/s is 60× its ~10k durable ceiling, and 6.7 TB of hot rows is ~50× the 128 GiB it caches; behind it the availability index needs ~200 Redis shards (20M ops/s ÷ 100k) | shard by `owner_id` into ~100 shards of today's shape (3k attempts/s, 67 GB hot each) behind a cached `owner → shard` directory: the exclusion constraint, holds and month view stay single-shard. The availability index becomes the sharded search tier of section 10; API N = ceil(1,500 ÷ 4.27) = 352 → 354 `c7g.2xlarge`. A viral item gets a waiting room: ~10 attempts per success (section 4) means a 20-unit item sells 20 times, so queue or lottery the rest and show "may free up". Give up: bundles across owners (no cross-shard transaction); a giant owner needs its own shard | ≈ $600k/month |
| **1,000×** | 3M attempts/s, 90M checks/s; 10B units (more than one per person alive: a thought experiment), 500B bookings a year ≈ 200 TB, ~6M writes/s | running the fleet and the planet: ~1,000 shards is ~3,000 Postgres nodes (a failure about every 1.7 h); the search tier is ~2,000 Redis shards (200M ops/s); and one region cannot serve the world inside the 200 ms booking budget (a cross-ocean round trip is ~150–250 ms, assumed) | **cells by geography**, because a unit is physical and never moves: each cell is today's whole baseline for the owners of one metro or region, with its own holds, reaper, outbox and availability index, behind a thin `owner → cell` router cached at the edge (a router outage stops new placements, not bookings). "Free near me" fans out to the few cells that cover the searched city instead of one global index. Give up: global "everything free this weekend" queries, bundles across cells (a trailer in one, its truck in another), one global ranking (results are merged per cell) | ≈ $6M/month |
| **10,000×** | 30M attempts/s, 900M checks/s; 100B units, 5T bookings a year (~600 per person alive: a thought experiment), ~60M writes/s | money and futility: ≈ $60M a month, and ~9 in 10 attempts fail (conflicts, abandoned holds: section 4's ~10 attempts per success) yet each still costs a hold insert and an expiry | change the problem, push work to the client and edge: the page fetches a per-city availability bitmap (46 B per item per year) from the CDN and filters locally, so 900M checks/s become a few cached fetches per session; the edge answers `409` to attempts the bitmap already shows as full, so doomed attempts never reach a cell; a hold row is written only when the payment page opens, short TTL extended once, capped per customer; hot items sell by lottery. Plan limits: booking horizon (say 12 months), open-ended blocks capped. Give up: exact live availability (the page is seconds old), unlimited horizons | ≈ $60M/month if nothing changed; far less after |

**Arithmetic:** 3k × 100 = 300k attempts/s → 300k inserts + 300k expiries = 600k writes/s ÷ 10k = 60; 67 GB × 100 = 6.7 TB ÷ 128 GiB ≈ 50; ~200k Redis ops/s × 100 = 20M ÷ 100k = 200 shards; 15 vCPU × 100 = 1,500 ÷ 4.27 = 351.6 → 352 → 354; 90k × 100 = 9M checks/s; 1,000 shards × 3 nodes = 3,000 ÷ 5,000 per hour ≈ one failure per 1.7 h; $6k × N.

**What stays the same at every scale:** the exclusion constraint on `(unit_id, stay)` with its status predicate, half-open ranges, and the unit as the conflict domain. Every step shards, cells or caches *around* that rule; none relaxes it.

**Say it in an interview:** "At 100× the primary sees 600k writes a second, 60× its ceiling, so I shard by owner id: the constraint, holds and month view stay inside one shard, and a viral item gets a waiting room. At 1,000× there are ~3,000 database nodes and one region cannot serve the world, so I build cells by geography behind a cached router, since units never move. At 10,000× nine in ten attempts fail, so the client filters availability from a CDN bitmap, the edge rejects doomed attempts, and a hold starts only at the payment page."
