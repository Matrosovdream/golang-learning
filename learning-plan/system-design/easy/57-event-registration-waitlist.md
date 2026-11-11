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
