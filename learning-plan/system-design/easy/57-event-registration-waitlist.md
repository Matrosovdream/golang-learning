# 57 · Event Registration with Capacity & Waitlist

`🟢 easy` · *Topics:* capacity as a counter row (a CHECK a pairwise constraint cannot give you), conditional `UPDATE` instead of count-then-insert, holds with expiry and a registration state machine, first-that-fits waitlist with offers, admission control for a 500k-attempt spike (gate + waiting room), cancellations, transfers and refunds that keep counters honest · *Builds on:* [28 Booking Calendar](28-booking-calendar.md) (holds, the DB refusing a double booking; this task is count-capacity, not range overlap), [14 Polls & Voting](14-polls-voting.md) (one per person, atomically), [13 Shopping Cart](13-shopping-cart.md) (reserve vs enforce), [17 Abuse Protection](17-abuse-protection.md) (bots)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [56 Related Content & Interlinking](56-related-content-interlinking.md) · Next: [58 Outbound Link Click Tracking](58-outbound-click-tracking.md)

---

> 🌱 **Seed (honeydolist-site):** **a plan, not built.** `docs/db/schema.md` sketches `events` (slug, name, `starts_at`/`ends_at` driving upcoming vs past, venue, `registration_url`), `event_sponsorship_tiers` and `event_registrations` (`team_name`, `captain_name`, `email`, `phone`, `players jsonb`, marked "phase 2"). No migration creates them: PLAN's *Deferred* table lists "Events: the golf tournament, sponsors, registrations (11 tables and a whole surface for one page a year)", and step 72.6 would at most add one static tournament page. `docs/ideas.md` items 1–3 (event reviews, attendee photos, anonymous feedback) lean on registrations for a "verified attendee" badge. **The plan has no capacity column and no waitlist**: a registration is a form carrying a team's details. What *is* built, and is the nearest analogue, is rental booking (migration 0011): "double-booking is prevented by the DATABASE, not by application logic", an exclusion constraint on a date range for held, confirmed and out bookings, optional holds on a request, `409` "those dates are gone", idempotent form submission (step 7k) and the outbox for notifications. Capacity as a *count* is a different kind of constraint from range overlap, and that difference is the lesson.
> **Here, at scale:** an events platform with 50k events a month, where one viral event takes 500k registration attempts in 10 minutes for 5,000 seats.

## 1. The task

> "Design event registration. Each event has a limited number of places. People register, may pay, may cancel; when an event is full people can join a waitlist and get a place if one frees up. A popular event must not oversell, and must survive the moment it opens."

On an ordinary day this is a CRUD form. The design is for one event for ten minutes.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Capacity per event or per ticket type? | Counter granularity. | Per ticket type (general, VIP). Each has its own capacity. |
| Seats per registration? | A team of 4 takes 4 places. | `qty` 1–10, a per-person maximum per ticket type (default 4). |
| Payment? | Holds, refunds, late webhooks. | Paid events: a seat is held 10 minutes while the person pays; free events confirm at once. |
| One registration per person? | Anti-hoarding, fairness. | One active registration per email per ticket type. |
| Waitlist? | Order and fairness rules. | Yes, capped at 4× capacity; first-that-fits; offers expire after 30 minutes. |
| Cancellations and transfers? | Counters must follow. | Cancel until a cutoff, with a refund policy; transfer to another person until 48 h before. |
| Are we under attack? | Bots decide the design at this scale. | Yes: assume 70% of attempts in a spike are automated. |
| Instant first-come-first-served, or fair? | Waiting room vs race. | Fairness matters more than microseconds: random order for the first seconds after opening. |
| Scale? | Everything above. | 50k events/month, ~200 registrations each; one event can take 500k attempts in 10 min for 5,000 seats. |

## 3. Requirements

**Functional**
- Register for a ticket type with `qty`; never exceed capacity; one active registration per person.
- Hold a seat for 10 minutes while paying; expire holds that are not paid.
- Join a waitlist when full; get an offer when seats free up; accept or decline it.
- Cancel (with refund per policy) and transfer; every status change is audited.
- Show remaining seats on the event page.

**Non-functional**
- Zero oversell, under any retry, race or crash.
- The registration database survives a 4k/s attempt peak without degrading for other events.
- Retrying any request is safe; a lost response does not cost the person their seat.

**Out of scope**
- Seat maps and assigned seating (the seat-level version is the medium backlog's ticket booking), dynamic pricing, resale marketplaces.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Normal load | ~10M registrations/month ≈ 3.9/s | 50k events × 200; ÷ 2.592M s |
| Viral event | 833/s average, ~4,200/s in the first minute | 500k ÷ 600 s; assume a 5× opening burst |
| Winners | 5,000 of 500,000 = 1% | 99% must be refused cheaply |
| Hot-row budget | ~500–1,000 updates/s on one counter row | assumption: the row lock is held until the commit's WAL flush, ~1 ms |
| Naive load on the counter row | 4–8× over budget | 4,200 ÷ 500–1,000 |
| With admission control | ≈ 6,000 attempts reach the database: ~100/s, at most ~600/s if all arrive within 10 s | gate admits 1.2 × seats |
| Holds | 5,000 holds; ~1,500 abandoned (30%, assumed) | 10-minute TTL |
| Waitlist | cap 20,000 entries; ~1,500 offers | 4 × capacity |
| Waiting-room state | ~50 MB | 500k entries × ~100 B in a Redis sorted set |
| Admission rate | 10,000 into checkout at once, then ~55/s as sessions end | 2 × seats at start; 10,000 ÷ ~180 s (assumed 3-min checkout) |
| Storage | ~3 GB/month, ~36 GB/year | 10M × ~300 B |

What the numbers tell us: average load is nothing. The design question is **how the database avoids seeing the 99% who lose** (a single hot row cannot take 4k/s), while the 1% who win are never oversold.

## 5. Plan

1. Model capacity so the database itself refuses an oversell.
2. Holds and the registration state machine.
3. The waitlist: order, offers, fairness.
4. Admission control for the spike.
5. Cancel, transfer, refund, and keeping the counters honest.

## 6. High-level design

```
 browser ─► CDN ─► event page, cached 1–2 s ("312 left" / "sold out · join waitlist"); display only, never a decision
    │
    ├─ enter waiting room ─► queue (Redis sorted set; random order for the first seconds) ─► bot & limit checks (17)
    │                          │ admit ≤ 2 × seats-in-play, signed single-use token (15 min)
    ▼                          ▼
 POST /registrations + token ─► API ─► Redis gate: DECRBY seats_in_play (≈ 1.2 × seats; reconciled from Postgres every few s)
                                  └─► Postgres tx: INSERT registration (pending, hold 10 min); UPDATE ticket_type SET held += qty WHERE available ≥ qty
                                          │ 0 rows → sold out → offer the waitlist
                                          ▼
                              payment (PSP) ─► webhook (18) ─► confirm: held −= qty, sold += qty ─► outbox ─► email (12)
 reaper (15):  pending past hold_expires_at → expired, held −= qty → "seat freed"
 promoter (15): "seat freed" → next waiting entries that fit → offer (30 min) → notify → accept → pending registration with the seat held
```

**One registration.** The person passes the waiting room, gets a token, and POSTs with an `Idempotency-Key`. One transaction inserts a `pending` registration and increments `held` only if seats remain. Payment confirms it (or the hold expires and the seat returns). **One sold-out attempt:** the conditional update touches zero rows, the transaction rolls back, and the person is offered the waitlist.

## 7. API

```
GET  /events/{slug}                         → 200 event + ticket types { remaining | "sold_out", waitlist_open }   (cached 1–2 s)
POST /events/{slug}/queue                   → 200 { state: "waiting" | "admitted", position?, token? }     GET …/queue/status
POST /events/{slug}/registrations   Idempotency-Key, X-Admission: <token>  { ticket_type, qty, holder, details }
                                            → 201 { id, status: "pending", hold_expires_at } | 200 (replay)
                                            | 409 { reason: "sold_out" | "already_registered" | "limit" } | 403 (token missing/expired/used)
POST /registrations/{id}/confirm            (called by the PSP webhook handler)                              → 200
POST /registrations/{id}/cancel             → 200 { refund: "initiated" | "none" }
POST /registrations/{id}/transfer           { new_holder }                                                   → 200
POST /events/{slug}/waitlist                { ticket_type, qty, holder }     → 201 { position } | 409 { reason: "waitlist_full" }
POST /waitlist/{id}/accept | /decline       → 201 registration | 200
```

## 8. Data model

```sql
CREATE TABLE events (                        -- from the repo's plan, trimmed to what registration needs
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug                  TEXT NOT NULL UNIQUE,
    name                  TEXT NOT NULL,
    starts_at             TIMESTAMPTZ NOT NULL,
    registration_opens_at TIMESTAMPTZ NOT NULL,
    registration_closes_at TIMESTAMPTZ,
    is_published          BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE event_ticket_types (            -- the counter row: one per ticket type
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id       BIGINT   NOT NULL REFERENCES events(id),
    name           TEXT     NOT NULL,
    capacity       INTEGER  NOT NULL CHECK (capacity >= 0),
    held           INTEGER  NOT NULL DEFAULT 0 CHECK (held >= 0),    -- pending registrations and open offers
    sold           INTEGER  NOT NULL DEFAULT 0 CHECK (sold >= 0),    -- confirmed
    per_person_max SMALLINT NOT NULL DEFAULT 4,
    price_cents    BIGINT   NOT NULL DEFAULT 0,
    waitlist_cap   INTEGER  NOT NULL DEFAULT 0,
    waitlist_len   INTEGER  NOT NULL DEFAULT 0,
    waitlist_next  BIGINT   NOT NULL DEFAULT 0,                      -- source of FIFO order
    CHECK (held + sold <= capacity)                                  -- the last line of defence against an oversell
);

CREATE TABLE event_registrations (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id         BIGINT   NOT NULL REFERENCES events(id),
    ticket_type_id   BIGINT   NOT NULL REFERENCES event_ticket_types(id),
    holder_email     TEXT     NOT NULL,                 -- normalised lower-case: the person key
    holder_name      TEXT     NOT NULL,
    qty              SMALLINT NOT NULL CHECK (qty BETWEEN 1 AND 10),
    details          JSONB    NOT NULL DEFAULT '{}',    -- the repo's plan: team_name, captain, players
    status           TEXT     NOT NULL CHECK (status IN ('pending','confirmed','cancelled','expired')),
    hold_expires_at  TIMESTAMPTZ,
    idempotency_key  UUID     NOT NULL,
    from_waitlist_id BIGINT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at     TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,
    CHECK (status <> 'pending' OR hold_expires_at IS NOT NULL),
    UNIQUE (ticket_type_id, idempotency_key)
);
CREATE UNIQUE INDEX event_reg_one_active_per_person
    ON event_registrations (ticket_type_id, holder_email) WHERE status IN ('pending','confirmed');
CREATE INDEX event_reg_holds_idx ON event_registrations (hold_expires_at) WHERE status = 'pending';

CREATE TABLE event_waitlist (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_type_id   BIGINT   NOT NULL REFERENCES event_ticket_types(id),
    seq              BIGINT   NOT NULL,                 -- from waitlist_next: FIFO within the ticket type
    holder_email     TEXT     NOT NULL,
    qty              SMALLINT NOT NULL,
    status           TEXT     NOT NULL CHECK (status IN ('waiting','offered','accepted','declined','expired','left')),
    offer_no         SMALLINT NOT NULL DEFAULT 0,
    offer_expires_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticket_type_id, seq)
);
CREATE UNIQUE INDEX waitlist_one_per_person ON event_waitlist (ticket_type_id, holder_email) WHERE status IN ('waiting','offered');
CREATE INDEX waitlist_next_idx   ON event_waitlist (ticket_type_id, seq) WHERE status = 'waiting';
CREATE INDEX waitlist_offers_idx ON event_waitlist (offer_expires_at)    WHERE status = 'offered';
```

## 9. Deep dives

### 9.1 Capacity as a counter, not a count

[28](28-booking-calendar.md) lets the database refuse a double booking with an *exclusion constraint*, because overlap is a conflict between **two rows**. "At most 5,000 registrations" is a rule about **all rows together**, which no unique or exclusion constraint can express.

| Option | Pros | Cons |
|---|---|---|
| `SELECT count(*)`, then `INSERT` | trivial | race: two sessions both see 4,999 and both insert: oversell |
| `count(*)` under `SELECT … FOR UPDATE` on the event row | correct | lock held across an O(n) count and the insert: ~200–300 attempts/s per event; every attempt waits |
| **A counter row with a conditional `UPDATE`** | O(1), one statement, correct; the aggregate becomes a *row* rule a `CHECK` can enforce | one hot row per ticket type; counter and rows must change in the same transaction |
| Redis `DECRBY` as the only truth | microseconds | Redis can lose state; a decrement with no row leaks a seat; a row with no decrement oversells |

**Choice and why.** The counter row in Postgres is the truth: `UPDATE event_ticket_types SET held = held + $qty WHERE id = $1 AND capacity - held - sold >= $qty`, zero rows means sold out. The `CHECK (held + sold <= capacity)` is the backstop for a buggy code path (a failed `CHECK` aborts the transaction, so use the `WHERE` for control flow). The registration insert comes **first**, `ON CONFLICT (ticket_type_id, idempotency_key) DO NOTHING RETURNING id`: no row back means a replay, so return the first answer and touch nothing; a violation of the *per-person* index is a different error (`23505`) mapped to `409 already_registered`. A Redis gate in front of it (9.4) is allowed to be wrong only in the cautious direction.

### 9.2 Holds and the registration state machine

| Option | Pros | Cons |
|---|---|---|
| Take payment first, then insert | no holds | pay and then lose the race: refunds and angry people |
| **Hold the seat with a TTL, then confirm** (as the repo's rental holds, and [28](28-booking-calendar.md) §9.3) | the person pays knowing the seat is theirs | abandoned holds block seats for 10 minutes; needs a reaper |
| Hold only in Redis | fast | a restart forgets every hold |

```
 pending ──payment ok──► confirmed ──cancel──► cancelled
    │  └──hold expires──► expired                    (each move is one conditional UPDATE … WHERE status = $from,
    └────cancel─────────► cancelled                    with the counter change in the same transaction)
```

| Move | Counter effect |
|---|---|
| create pending | `held += qty` (only if available) |
| pending → confirmed | `held −= qty`, `sold += qty` |
| pending → expired / cancelled | `held −= qty`, then "seat freed" |
| confirmed → cancelled | `sold −= qty`, then "seat freed" |

The reaper claims batches (`WHERE status = 'pending' AND hold_expires_at < now() … FOR UPDATE SKIP LOCKED`, [15](15-job-scheduler.md)); availability is always computed from counters, so a hold that has expired but not yet been reaped is conservative, never an oversell. A **late payment** for an expired hold: try to re-take the seat with the same conditional update; if it is gone, refund automatically with an apology (9.5).

### 9.3 The waitlist

| Option | Pros | Cons |
|---|---|---|
| Strict FIFO | simplest to explain | a party of 4 at the front blocks a free single seat while it sits empty |
| **First-that-fits, keeping everyone's place** | seats do not stay empty | someone further back may be served first; explain it in the UI |
| Lottery from the waitlist at a deadline | resists bots and speed advantages | not instant; not suitable for every event |

**Choice and why.** First-that-fits with offers. A "seat freed" event wakes the promoter: for the freed `n` seats it takes the first waiting entries that fit (`ORDER BY seq`, `FOR UPDATE SKIP LOCKED`), moves each to `offered`, sets `offer_expires_at = now() + 30 minutes`, increments `held` for the offered seats, and notifies ([12](12-notification-service.md)). Accepting creates a `pending` registration (`from_waitlist_id`) whose hold is the offer's; declining or expiring returns the seats and the promoter cascades to the next entry. Every offer carries `offer_no`, so a retried promoter run cannot offer twice. Order comes from `waitlist_next` (an increment on the ticket type row, only on waitlist writes), and a person's position is `count(*) WHERE status = 'waiting' AND seq < $mine` over the partial index, fine up to tens of thousands. Cap the list (4× capacity) so a viral event does not store 500k hopeful rows.

### 9.4 Admission control: the database must not see the 99%

| Option | Pros | Cons |
|---|---|---|
| Let everyone hit the API and database | nothing to build | 4,200/s against a ~500–1,000/s hot-row budget; timeouts; client retries amplify the spike |
| **A cheap gate: remaining seats cached at the edge + a Redis counter** | refuses the losers in microseconds; the database sees ≈ 1.2 × seats | the gate must be conservative and reconciled |
| **A waiting room with signed admission tokens** | fairness (random order for the first seconds), smooth load, bot friction, an honest "you are #12,340" | an extra service; users dislike queues (but dislike failures more) |
| A lottery at the end of the window | no race, no hammering | not instant; some events cannot use it |

**Choice and why.** Layers. (1) The event page is cached 1–2 s and says "sold out, join the waitlist" without touching the database. (2) A **waiting room** opens at `registration_opens_at`: arrivals in the first seconds are ordered randomly (being fastest buys nothing), bots meet limits and challenges ([17](17-abuse-protection.md)), and the queue admits up to **2× the seats still in play** (10,000 at the start), refilling as holds convert or expire (~55/s). Admission issues an HMAC-signed, single-use token (event, expiry 15 min, a `jti` recorded with `SET NX`) that `POST /registrations` requires. (3) A Redis `DECRBY` on `seats_in_play` admits ≈ 1.2× the seats into the database; it is **reconciled from Postgres** (`capacity − held − sold`) every few seconds, and when Redis restarts it is rebuilt from Postgres while admissions pause. Position updates reach the browser over SSE or polling ([lesson 58, SSE](../../58-realtime-websockets-sse.md)).

### 9.5 Cancel, transfer, refund, and keeping the counters honest

- **Cancel:** one conditional update (`WHERE status = 'confirmed'`) plus `sold −= qty` in the same transaction, then "seat freed" for the promoter. The refund is a **separate, idempotent job** keyed by registration id ([30](30-invoicing.md) §9.3 for payment state); the seat is reusable immediately, the money follows the policy (full until the cutoff, then partial or none).
- **Transfer:** change the holder in place (same seat, same price, an audit entry), refused after the cutoff and for a person who already holds an active registration of that type (the per-person index decides). Cancel-and-rebook would lose the seat to the waitlist.
- **Races:** cancel vs promotion (the promoter reads `held` and `sold` inside its own transaction), cancel vs late payment, double cancel: all resolve because every move is `UPDATE … WHERE status = $from` and a zero-row result means "someone else moved it first": re-read and answer.
- **Counters drift** (a bug, a manual fix): a reconciliation job compares `held` and `sold` to `sum(qty)` by status for each ticket type and **alerts** on a mismatch ([50](50-alerting-engine.md)) before repairing it. The `CHECK` makes the dangerous direction (oversell) impossible.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (50k seats, ~100k attempts/s): the gate and the waiting room carry it, but a single counter row still tops out near 1,000 updates/s. Split capacity into K buckets (`ticket_type_buckets`), try a hashed bucket and fall through to the next; sold-out is when all buckets refuse. Or admit in waves.
- **Redis gate lost:** rebuild from Postgres, pause admission for seconds; no seat is lost because Postgres is the truth.
- **Queue lost:** people rejoin with new random positions; for mega events keep queue entries in a durable store.
- **PSP slow or down:** holds would expire mid-payment; extend the hold TTL when the PSP's status degrades, and rely on the late-payment path (9.2).
- **Postgres failover mid-hold:** transactions either committed (the hold exists) or not (replay with the same idempotency key finds or creates it).
- **Scalpers:** one active registration per person, verified accounts, transfer limits; the trade-off is hostile to legitimate gifts.
- **Single points of failure:** Postgres (the truth), Redis (the gate, rebuildable), the token signing key (rotate; accept the previous key briefly).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| a counter row with conditional `UPDATE` and a `CHECK` | `count(*)` under a lock | O(1), no oversell, backstop in the database | a hot row at extreme rates |
| holds with TTL and a reaper | pay first | the payer knows the seat is theirs | seats blocked for up to 10 minutes |
| first-that-fits waitlist with offers | strict FIFO | no empty seats while a big party waits | someone behind may be served first |
| a waiting room with random start | first-come-first-served races | fairness, smooth load, bot friction | an extra service and user patience |
| Redis gate reconciled from Postgres | Redis as the truth | microsecond refusals | a second system that must be rebuilt after a loss |
| refund as an async idempotent job | refund inside the cancel | the seat frees at once; retries are safe | a window where money is owed |
| transfer in place | cancel and rebook | keeps seat, price and history | rules around the cutoff and per-person limits |

## 12. Check yourself

1. Why can a unique or exclusion constraint not enforce "at most 5,000 registrations", and what does the counter row change?
2. The conditional `UPDATE` is already correct. Why add a Redis gate and a waiting room?
3. A person's `POST /registrations` response is lost and they retry from a second tab. What happens?
4. One seat frees up; the waitlist is 4, 2, 1 seats. What happens, and why can a retried promoter not double-offer?
5. A hold expires, the seat is re-sold, and then the "payment succeeded" webhook arrives. What does the system do?

<details><summary>Answers</summary>

1. Unique and exclusion constraints compare rows pairwise ("these two conflict"); "at most N" is an aggregate over all rows. A counter row turns the aggregate into a single row's value, so `UPDATE … WHERE capacity − held − sold >= qty` is atomic and `CHECK (held + sold <= capacity)` can enforce it.
2. Correctness is not the problem; throughput is. One counter row serialises at roughly 500–1,000 updates/s, while a viral event produces ~4,200 attempts/s of which 99% lose. The gate and the queue stop the losers before the database and add fairness and bot friction.
3. The retry carries the same `Idempotency-Key`, so `INSERT … ON CONFLICT (ticket_type_id, idempotency_key) DO NOTHING RETURNING id` returns no row; the server returns the first registration unchanged and touches no counter. A different key from the same person hits the per-person partial unique index and gets `409 already_registered`.
4. The promoter takes the first waiting entry that fits: the 1-seat entry is offered (30 min) while the 4 and the 2 keep their places. The offer carries an `offer_no` and `held` is incremented once in the same transaction, so a retried run finds the entry already `offered` and does nothing.
5. The webhook handler tries to confirm: `pending → confirmed` fails (the registration is `expired`); it then attempts to re-take the seat with the same conditional update. If seats remain, it confirms; if not, it issues an idempotent automatic refund and notifies the person with an apology.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) (transactions), [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE SKIP LOCKED`, partial unique indexes, conditional updates), [44 Background jobs](../../44-background-jobs-queues.md) (reaper and promoter), [41 API design](../../41-api-design-evolution.md) (idempotency keys, `409`), [68 Rate limiting](../../68-rate-limiting-throttling.md), [58 SSE](../../58-realtime-websockets-sse.md), [34 Outbox](../../34-event-driven-outbox.md).
- Packages: `pgx`, `go-redis` (sorted sets, `DECRBY`, `SET NX`, Lua), `crypto/hmac` + `crypto/sha256` for admission tokens.

The hold: insert first, then the conditional counter update; a zero-row update rolls the whole transaction back.

```go
func (r *Repo) Hold(ctx context.Context, in HoldInput) (Registration, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Registration{}, err
	}
	defer tx.Rollback(ctx) // no-op after Commit; on ErrSoldOut it also removes the inserted row

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO event_registrations
		  (event_id, ticket_type_id, holder_email, holder_name, qty, status, hold_expires_at, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, 'pending', now() + interval '10 minutes', $6)
		ON CONFLICT (ticket_type_id, idempotency_key) DO NOTHING
		RETURNING id`, in.EventID, in.TypeID, in.Email, in.Name, in.Qty, in.Key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.byKey(ctx, in.TypeID, in.Key) // a replay: the first answer, nothing changed
	}
	if err != nil {
		return Registration{}, mapUnique(err) // 23505 on the per-person index → ErrAlreadyRegistered
	}
	tag, err := tx.Exec(ctx, `
		UPDATE event_ticket_types SET held = held + $2
		WHERE id = $1 AND capacity - held - sold >= $2`, in.TypeID, in.Qty)
	if err != nil {
		return Registration{}, err
	}
	if tag.RowsAffected() == 0 {
		return Registration{}, ErrSoldOut
	}
	return Registration{ID: id, Status: "pending"}, tx.Commit(ctx)
}
```

## 14. Extensions

- **Sharded counters** for stadium-size events: K buckets, hashed first try, fall-through, and what "remaining" means across buckets.
- **A lottery mode** for the busiest events: collect entries for a window, draw, then notify winners with a payment deadline.
- **Seat maps and assigned seating:** the medium backlog's ticket booking (range-like conflicts per seat, [28](28-booking-calendar.md)).
- **Dynamic pricing and tiers** that open in waves ([29 Price & Availability Engine](29-price-availability-engine.md)).
- **Check-in:** signed QR codes, offline scanning, and a replay-safe "used" flag.
- **Reviews and verified attendees:** the seed's `docs/ideas.md` badge, derived from `confirmed` registrations and the event end time ([19](19-moderated-submissions.md)).
- **Sponsorship tiers** as inventory with their own counters (`event_sponsorship_tiers` in the repo's plan).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). Small in hardware terms: the normal load is 3.9 registrations/s. The capacity is for ten minutes on one viral event, and the opening time is known in advance, so it is scheduled, not kept running. What limits throughput is one counter row, and a bigger instance does not move that.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Waiting-room + registration API (Go, stateless) | viral event: 4,200 queue entries/s; 500k waiting (§4) polling every 10 s (assumed) = 50k polls/s; ≤ 600 registrations/s reach the database | CPU | entries 4.2k × 0.3 ms = 1.3 vCPU; polls 50k × 0.1 ms = 5.0; registrations 600 × 2 ms (token HMAC, `SET NX`, `DECRBY`, one transaction) = 1.2; total 7.5 vCPU. N = ceil(7.5 ÷ (0.8 × 2 × ⅔ = 1.07)) = ceil(7.03) = 8 → 9 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3 normally, 9 for each opening window (scheduled scale-out) | per-request CPU, trivial memory, so `c`; 42 % busy on 9 nodes, 62 % with a zone down. The reaper, promoter and outbox relay run as goroutines inside these nodes (`SKIP LOCKED` makes that safe), so they need no instances |
| Redis (waiting-room sorted set, gate counters, token `jti`) | `ZADD` 4.2k/s + `ZRANK` 50k/s + `DECRBY`/`SET NX` ~1.2k/s ≈ 55k ops/s; 500k entries × ~100 B = 50 MB (§4) | one core (ops/s), not memory | 55k × ~10 µs = 0.55 of the one core a node can use (the sheet: ~100k simple ops/s); memory 50 MB × 2 for the fork ≪ 8 GiB | `cache.m7g.large` (2 vCPU / 8 GiB), ElastiCache | 1 primary + 1 replica (other zone) | memory is nowhere near a limit, so not `r`; commands run on one core, so extra vCPU buys nothing: size by ops/s and stay under ~60 %. The replica turns a node loss into a failover instead of 500k people re-queuing (§10). Two viral events on one node would pass 100 %: cluster mode by event id |
| Postgres (ticket-type counter rows, registrations, waitlist) | ~100 attempts/s, ≤ 600/s if all arrive within 10 s (§4); ~5,000 confirmations over ~10 min; ~3 GB/month, 36 GB/year | the hot row's commit latency, then memory | 600 commits/s × ~2 IOPS = 1.2k vs 3,000 included in `gp3`; CPU 600 × ~1 ms = 0.6 of 2 vCPU. Hot set = this month's rows + indexes ≈ 3 GB × 1.7 ≈ 5 GB, plus connections ⇒ 16 GiB, not 8. The counter row serialises at ~500–1,000 updates/s (§4, ~1 ms commit); a Multi-AZ commit near 2 ms would halve that to ~500/s (assumption) | `db.r7g.large` (2 vCPU / 16 GiB), RDS Multi-AZ, 200 GB `gp3` | 1 primary + synchronous standby | neither CPU nor IOPS is the limit, the single row is; a larger instance does not raise it, which is why the gate admits only ~1.2 × seats (≈ 6,000 attempts). `r` rather than `m` only for RAM (5 GB hot set in 8 GiB is too tight). 36 GB/year means 200 GB lasts 3+ years |

Event pages (cached 1–2 s) and static assets sit on the CDN; their bytes are not in section 4 (≈ $850 per 10 TB at the first-tier price). The queue traffic itself is small: 50k polls/s × ~0.5 KB for 10 minutes ≈ 15 GB out ≈ $1.4 per viral event. Confirmation emails go through [12](12-notification-service.md); the payment provider is outside AWS.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API baseline: 3 × `c7g.large` (3 × $53) | $160 |
| Opening windows: 6 extra `c7g.large` × ~30 hours (assumed ~30 big openings) × $0.073/h | $10 |
| Redis: 2 × `cache.m7g.large` (2 × ≈ $120) | $240 |
| Postgres: `db.r7g.large` ≈ $156, Multi-AZ doubles | $310 |
| gp3 200 GB (× 2 for the standby) | $30 |
| ALB: $16 + ~170 LCU (4.2k new connections/s ÷ 25) × $0.008 × ~30 spike hours + baseline | $60 |
| **Total** | **≈ $810** (≈ $560 with 1-year commitments on the $710 of steady compute) |

- **Biggest line and why:** the database pair (≈ $340 with its disks, 42 %): seats are money and must survive a zone loss. The spike itself costs ≈ $10 of compute because it is scheduled. There is no useful lever: a smaller Postgres saves little, and the hot row does not care about instance size.
- **What changes at 10×:** 50k seats and ~100k attempts/s (§10). The room holds 5M entries (500 MB, still tiny) but the polls become 500k/s: CPU ≈ 50 + 30 + 12 = 92 vCPU ⇒ N = ceil(92 ÷ 4.27) = 22 → 24 × `c7g.2xlarge` for the window (≈ $7 an hour). Redis needs ~600k ops/s ⇒ ~9 shards at ≤ 70 % of a core, or one shared "now serving" number cached at the edge instead of per-user polls, which brings it back to ~2. The counter row gets ~6k attempts/s against a ~1k budget, so split it into ~8 buckets (§10); Postgres moves to `db.r7g.2xlarge` for the CPU.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Tiny on average, violent for ten minutes: ~10M registrations a month (≈ 3.9/s, ~36 GB a year), but one viral event brings ~4,200 attempts/s, of which 99% must be refused before they reach the database (section 4).
- The one hard rule is an aggregate, `held + sold <= capacity` per ticket type, enforced by a counter row and a conditional `UPDATE` that serialises at ~500–1,000 updates/s (sections 4, 9.1). Raw load is not the problem; one row's lock is.
- Conditional uniqueness: one active registration per person and ticket type, one waiting entry per person (partial unique indexes), and one registration per `idempotency_key`.
- Queue-like workers: the reaper (expired holds) and the promoter (waiting entries in `seq` order) claim rows with `FOR UPDATE SKIP LOCKED`.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the counter row is portable, but Postgres adds the pieces around it: partial unique indexes for "one active registration per person" (`WHERE status IN ('pending','confirmed')`) and one waiting entry per person; partial indexes for the reaper and promoter (`WHERE status = 'pending'`, `WHERE status = 'waiting'`); `INSERT … ON CONFLICT (ticket_type_id, idempotency_key) DO NOTHING RETURNING id` tells a replay from a new row in one statement; at the default `READ COMMITTED` a waiting `UPDATE` re-checks its `WHERE` against the winner's committed row |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | the conditional `UPDATE` and the `CHECK` work the same (`CHECK` is enforced from 8.0.16) and the row-lock ceiling is the same; what is missing is partial indexes and `RETURNING`, and the default `REPEATABLE READ` needs care for the workers (below) |
| DynamoDB | ✔ works, no gain | a conditional `UpdateItem` on a counter item and a `TransactWriteItems` (≤ 100 items) for registration + counter + a uniqueness marker item would work, but one counter item is one partition key with the same ~1,000 writes/s ceiling, so the spike is not solved; the reaper and promoter need extra indexes and TTL, and there are no ad-hoc queries |
| Redis (ElastiCache / MemoryDB) | ✔ as the derived gate and waiting room, ✖ as the record | `DECRBY` on `seats_in_play`, the waiting-room sorted set and the token `SET NX` refuse the 99% in microseconds; it can lose state, and a decrement with no row leaks a seat (9.1), so Postgres reconciles it every few seconds |

**Pick:** PostgreSQL on RDS Multi-AZ, the `db.r7g.large` of section 15, for events, ticket types, registrations and the waitlist: a counter row guarded by a `CHECK`, partial unique indexes that make "one active registration per person" a database rule, and `ON CONFLICT … RETURNING` for idempotent holds. A bigger instance would not raise the hot-row ceiling, which is why the Redis gate and the waiting room admit only ~1.2 × the seats. Postgres is the record; Redis (`cache.m7g.large` plus a replica) is derived and rebuilt from it when it restarts.

**If it were MySQL instead:** the same four tables, the same counter row and the same budget (InnoDB locks that one record, because a primary-key equality `UPDATE` takes a record lock and no gap lock, holds it to commit and re-reads the current row). Changes: each partial unique index becomes a generated column that is `NULL` unless the status is active, plus a unique index on it; the worker indexes become `(status, hold_expires_at)` and `(ticket_type_id, status, seq)`. `INSERT IGNORE` and `ON DUPLICATE KEY UPDATE` fire on either unique key, so a replay would look like `already_registered`: use a plain insert and read which key error 1062 names, then fetch the first row (no `RETURNING`). `SKIP LOCKED` exists since 8.0, but a range-scanning locking read at `REPEATABLE READ` takes next-key locks that can block inserts into the scanned range, so run the reaper and promoter at `READ COMMITTED`.

**What would flip the choice:** not the engine and not the load. At 10× (~100k attempts/s) the answer is ~8 counter buckets in the same database (section 10). A durable queue for mega events (people must not rejoin after a node loss) would move the waiting room to MemoryDB. Only thousands of viral events at once, with aggregate writes beyond one primary (~10k durable writes/s), would justify sharding by event.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | one viral event ×100: 50M attempts in 10 min for 500k seats (83k/s average, ~420k/s in the first minute; more seats than any venue holds: a multi-venue or streamed event); waiting room 5 GB, 5M polls/s, ~5.5M Redis ops/s; ~600k attempts reach the database (≈ 60k/s if admitted within 10 s); ~750 vCPU of API | two single things: the counter row (60k/s against its ~1k/s budget is 60× over, and 6× a primary's own ~10k commits/s) and the waiting room's one sorted set (5.5M ops/s against one core's ~100k is 55× over) | admit in waves metered to what the database can take (≤ 5k/s, so 600k attempts drain in ~2 minutes); split each ticket type into 8 counter buckets (try a hashed bucket, fall through, section 10) so each row sees ~600/s; shard the queue over ~80 Redis shards by session hash (~70k ops/s each) and replace per-user polls with one "now serving N" number cached at the edge; ~177 `c7g.2xlarge` for the window. Give up: an exact `remaining` across buckets (a party of 4 can be refused while single seats remain elsewhere), an exact queue position (a range instead) | ≈ $27k/month, not $81k: the spike is rented by the hour (~177 nodes × 30 h ≈ $1.5k, ALB ≈ $4k); Redis stays up (≈ $19k) with one `db.r7g.4xlarge` Multi-AZ ≈ $2.5k |
| **1,000×** | 500M attempts in 10 min for 5M seats (10 % of all internet users at once, and 50+ stadiums of seats: a thought experiment); 4.2M/s in the first minute; waiting room 50 GB, 50M polls/s, ~55M Redis ops/s; ~6M attempts reach the database | even with waves and 8 buckets, one Postgres primary registers ~10k commits/s: 6M attempts ÷ 10k = 10 minutes, as long as the whole window. The queue is ~550 Redis shards, and one region takes 4.2M requests/s at the front door | per-event **cells**: carve the 5M seats into ~100 sub-inventories of 50k, each section 15's 10× design (its own primary with 8 buckets, its own Redis, its own queue lane), behind a router that hashes each session to a cell (fair by construction) and spills to a cell with seats left when its own sells out; the router reads one cheap remaining counter per cell. Give up: one exact global `remaining` and party-size fit (a sweep re-sells stranded seats at the end), a global queue position | ≈ $0.4M/month if the 100 cells stay up (≈ $4k each: Redis ≈ $2.2k, Postgres ≈ $1.2k); rented per event, a spike costs a few hundred dollars |
| **10,000×** | 5B attempts in 10 min (about every internet user once) for 50M seats (no event has that many: a thought experiment); 42M requests/s at the front door, a 500 GB queue, 500M polls/s, ~60M attempts to the database | the race itself: 5B people chasing 50M seats means 99 % lose, and serving a live, fair race to all of them needs ~42M requests/s and ~500M polls/s. Nothing here is worth that | change the problem: no race, a lottery (extension). Entries are collected over a day (5B ÷ 86,400 ≈ 58k idempotent writes/s, ~700× below the spike), a batch job draws 50M of the 5B ids, and winners get a signed claim link with a payment deadline; only winners touch the counter (50M claims over a day ≈ 580/s, spread over buckets and cells). Give up: instant first-come-first-served and an answer in the first seconds; bots are handled by verified accounts and one entry per person | ≈ $8M/month if the live fleet stayed up; as a lottery ≈ $6k of queue calls per event (5B × 3 SQS calls × $0.40 per million) plus a batch draw |

**What stays the same at every scale:** seats are only ever granted by the conditional `UPDATE … WHERE capacity − held − sold >= qty` on a counter row in Postgres, backed by `CHECK (held + sold <= capacity)` (per bucket, per cell, or for winners only), with an `Idempotency-Key` and the per-person partial unique index, so no multiple can oversell.

**Say it in an interview:** "At 100× one counter row is 60× over its budget and the queue is one hot Redis key, so I admit in waves, split the counter into 8 buckets and shard the queue: the same design scaled out. At 1,000× one primary would need ten minutes to register 6M attempts, so I cut the event into ~100 cells of 50k seats behind a hash router. At 10,000× a live race for 5B people is the problem, so I replace it with a lottery: collect entries over a day, draw, and let only the winners touch the counter."
