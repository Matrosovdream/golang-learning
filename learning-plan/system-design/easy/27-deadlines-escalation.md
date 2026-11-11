# 27 · Deadlines & Escalation

`🟢 easy` · *Topics:* finding what is due without a timer per item, a rolling `next_fire_at`, recomputing when a deadline moves or is met, idempotent reminders, escalation ladders and who is told (ladder × preferences × fallback), per-tenant time zones and DST, spreading the 09:00 herd, digest grouping · *Builds on:* [15 Job Scheduler](15-job-scheduler.md) (due-queue, `SKIP LOCKED`, leases, DST policy), [12 Notification Service](12-notification-service.md) (channels, preferences, digests), [10 Webhook Delivery](10-webhook-delivery.md) (outbox, dedupe keys), [26 Configurable Status Workflow](26-status-workflow.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [26 Configurable Status Workflow](26-status-workflow.md) · Next: [28 Booking Calendar for Rentable Units](28-booking-calendar.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 35 + migration 0067 give an order dated **deadlines** (`order_deadlines`: title, `due_on` date + optional time, status `open|done|missed|cancelled`, owner, `remind_days` = 3, and one timestamp per alert: `reminded_at`, `told_today_at`, `missed_at`, `escalated_at`, "each told once"), steps that meet a deadline (a pivot; a step can *complete* it in one save), and a worker task `deadlines.tick` every 5 minutes: due soon, due today, **missed the morning after**, still-missed in the daily email, escalation after N days (default 3, 0 = off) to the Superadmin and Admins **whatever they chose**, and a Monday week-ahead. Who is told comes from steps 25.6/7w/42: each person picks *in the dashboard / email at once / daily 7 AM email* per action, *only mine*, role defaults for new people, and the owner's list as the **fallback** only when no one on staff covers an action. The seed keeps one business time zone, starts the day's alerts at 6 AM so they land in the 7 AM digest, and dedupes outbox rows by key (`outbox_dedupe_any`).
> **Here, at scale:** 100M open deadlines across tenants in many time zones, ~30k reminders due per minute at the 09:00-local peak, and a rule that a missed deadline is never silent and never sent twice.

## 1. The task

> "Design deadline tracking for a work-management product. Items have dated deadlines; people are
> reminded before, on and after the date; ignored deadlines escalate to managers. It must be right
> across time zones and never miss or duplicate a reminder."

This is the job scheduler ([15](15-job-scheduler.md)) applied to a product feature: the new
questions are *who* is told, *when* "9 AM" is, and what happens when a deadline changes underneath
a reminder.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Date only, or date and time? | Defines "missed". | Date-only means "through the end of that local day"; a time is optional. |
| Whose time zone? | Every "9 AM" and "end of day". | The tenant's IANA zone; digests may use the person's own. |
| Which reminders? | The kinds of fire. | Soon (N days before), today, missed (next morning), escalated (after M days), a weekly look-ahead. |
| Who is told? | Recipient resolution. | A ladder (owner → manager → admins) filtered by each person's preferences, plus a fallback. |
| Can a person opt out? | Mandatory vs optional alerts. | Everything except escalation can be turned off. |
| Can deadlines change? | Recompute. | Yes: moved, completed by a step, cancelled, time zone changed. |
| Delivery guarantee? | Duplicates vs misses. | At-least-once fire, deduped to once in effect; up to ~1 min late is fine. |
| Scale? | Dispatcher design. | 100M open deadlines, 30k due/min at peak, 1.5 recipients each. |

## 3. Requirements

**Functional**
- Create, move, complete (directly or by attaching a step that completes it), cancel deadlines.
- Fire `soon`, `today`, `missed`, `escalate` once per deadline version; a missed deadline also shows in every daily digest while it stays missed.
- Resolve recipients per fire: ladder ∩ preferences, mandatory escalation, fallback owner.
- Channels per person per action: in-app, email at once, daily digest, off; "only mine".
- Per-tenant time zone and DST-correct dates; lists such as "overdue", "today", "this week".

**Non-functional**
- No reminder lost on a worker crash or restart; no duplicate on retry; none after the deadline is done.
- A deadline change takes effect within a minute.
- A person with 40 items due gets one digest, not 40 emails.

**Out of scope**
- Recurring deadlines, business-hours SLA clocks, calendar sync (see Extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Open deadlines | 100M ≈ 25 GB | ~250 B a row with indexes |
| New deadlines | ~3.3M a day ≈ 39 / s | 100M ÷ ~30-day lifetime |
| Fires | ~13M a day ≈ 154 / s ≈ 9.2k / min | ~4 fires per deadline (soon, today, missed, escalate) |
| Peak | 30k due / min ≈ 500 / s | the 09:00-local waves, already jittered over 15 min: ≈ 3× the average minute |
| Unjittered wave | ~450k in one minute | 30k × 15 |
| Claim batches at peak | 30 a minute | 30k ÷ 1,000 rows per claim |
| Recipients | ~45k a minute ≈ 750 / s | 1.5 per fire |
| After digest grouping | ~7.5k messages a minute | ~6 items per person: 45k ÷ 6 |
| Index on `next_fire_at` | ~100M entries, a few GB | one per open deadline; only its head is hot |

What the numbers tell us: the average load is tiny (154 fires/s); the shape is **spiky and
time-zoned**, so the design is about spreading the 09:00 wave, grouping messages per person, and
never doing per-deadline work that scales with the 100M rows. Fires are cheap indexed claims.

## 5. Plan

1. Model a deadline with an absolute instant, a zone, a version and exactly one *next fire*.
2. Dispatch: a due-index claimed in batches (the scheduler pattern), re-checking state at fire time.
3. Recompute on every change; make reminders idempotent per deadline version.
4. Resolve who is told: ladder × preferences × fallback; mandatory escalation.
5. Get time right: zones, end of day, DST; spread the herd; group per person.

## 6. High-level design

```
 API ──► deadlines (due_at UTC, tz, status, owner, due_version, next_kind, next_fire_at) ──► Postgres
   ▲        step attached with completes=true  ──► status 'done', next_fire_at = NULL (same transaction)
   │
 dispatchers (N workers, poll every 5–15 s)
   1 claim: due rows (SKIP LOCKED, 1,000 at a time), push next_fire_at forward as a lease
   2 per row, in one transaction: lock the row, re-check open + due_version + stale kind
   3 resolve recipients (ladder ∩ prefs, else fallback) ──► outbox rows (dedupe_key)
   4 compute the next kind and next_fire_at, commit
   │
 outbox relay ──► notification service (12): in-app (45) · email at once · daily digest (grouped per person)
```

A fire is a small transaction on one row. Nothing scans all deadlines: the claim reads the front
of one partial index.

## 7. API

```
POST /items/{id}/deadlines  { title, due_on, due_time?, owner_id?, remind_days?, step_ids[]? }  → 201
PUT  /deadlines/{id}        { …, version }               → 200 | 409 stale; bumps due_version if the instant changes
PUT  /deadlines/{id}/status { status: "done" | "open" | "cancelled" }      ("missed" is the dispatcher's alone)
PUT  /deadlines/{id}/steps  { step_ids[], completes? }   → a step meeting it can complete it in one save
GET  /deadlines?due=overdue|today|week&mine=1&cursor=…   → paged by (due_at, id)
GET/PUT /tenants/{id}/deadline-policy { tz, default_remind_days, escalate_after_days, escalate_to[], work_start }
GET/PUT /users/me/alert-prefs  [{ action, channel: "in_app"|"email"|"digest"|"off", only_mine }]
```

## 8. Data model

```sql
CREATE TABLE deadline_policies (
  tenant_id bigint PRIMARY KEY, tz text NOT NULL,                 -- IANA name: 'America/New_York', never a fixed offset
  default_remind_days integer NOT NULL DEFAULT 3, escalate_after_days integer NOT NULL DEFAULT 3,   -- 0 = off
  escalate_to text[] NOT NULL DEFAULT '{role:admin,role:superadmin}', work_start time NOT NULL DEFAULT '06:00'
);

CREATE TABLE deadlines (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL, item_id bigint NOT NULL,
  title text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
  due_on date NOT NULL, due_time time,                  -- date-only = through the end of that local day
  tz text NOT NULL,                                     -- copied from the policy; recomputed if it changes
  due_at timestamptz NOT NULL,                          -- the derived instant: end of local day, or due_time
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','missed','cancelled')),
  done_at timestamptz, owner_id bigint, remind_days integer NOT NULL DEFAULT 3 CHECK (remind_days BETWEEN 0 AND 60),
  due_version integer NOT NULL DEFAULT 1,               -- bumped when due_at changes: in-flight fires for old versions are dropped
  next_kind text CHECK (next_kind IN ('soon','today','missed','escalate')),
  next_fire_at timestamptz                              -- the ONE pending fire; NULL = nothing pending
);
CREATE INDEX deadlines_fire_idx  ON deadlines (next_fire_at) WHERE next_fire_at IS NOT NULL;      -- the dispatcher
CREATE INDEX deadlines_owner_idx ON deadlines (tenant_id, owner_id, due_at) WHERE status = 'open'; -- "my overdue"
CREATE INDEX deadlines_due_idx   ON deadlines (tenant_id, due_at, id)       WHERE status = 'open'; -- "this week"

CREATE TABLE deadline_steps (
  deadline_id bigint NOT NULL REFERENCES deadlines(id) ON DELETE CASCADE, step_id bigint NOT NULL,
  PRIMARY KEY (deadline_id, step_id)
);

CREATE TABLE alert_prefs (                                -- per person per action; role defaults seed it
  user_id bigint NOT NULL, action text NOT NULL, channel text NOT NULL CHECK (channel IN ('in_app','email','digest','off')),
  only_mine boolean NOT NULL DEFAULT true, set_by bigint, PRIMARY KEY (user_id, action)
);
-- outbox: dedupe_key text UNIQUE  (see 10): 'dl:{id}:{kind}:{due_version}:{user}' = once per version, kind and person
```

## 9. Deep dives

### 9.1 Finding what is due

| Option | Pros | Cons |
|---|---|---|
| A timer per item (in-process timers, a timing wheel) | precise | 100M timers to rebuild on every restart; hard to shard; lost on crash |
| A delayed queue (SQS delay, Redis sorted set) | built-in time | delay limits; moving or cancelling a pending message is awkward |
| A reminders table, 4 pre-created rows per deadline | simple to query | ~400M rows to create, move and delete on every edit |
| **A rolling `next_fire_at` on the deadline + a due-index claimed in batches** | one pending fire per row; cancel = `NULL`; no extra table; the scheduler pattern of [15](15-job-scheduler.md) | the table takes ~4 small updates per deadline in its life |

**Choice and why.** The rolling `next_fire_at`. The claim reads the head of a partial index and
pushes the time forward as a **lease**, so a dead worker's rows reappear in two minutes:

```sql
WITH due AS (
  SELECT id FROM deadlines WHERE next_fire_at <= now()
  ORDER BY next_fire_at LIMIT 1000 FOR UPDATE SKIP LOCKED
)
UPDATE deadlines d SET next_fire_at = now() + interval '2 minutes'
FROM due WHERE d.id = due.id
RETURNING d.id, d.tenant_id, d.next_kind, d.due_version, d.due_at, d.tz, d.owner_id;
```

The fire transaction then **locks the row again and re-checks** `status = 'open'` and `due_version`;
if the deadline was completed or moved meanwhile, it does nothing. At 154 fires/s on average and 500/s at
peak this is a few claims a second. **Catch-up after an outage** skips *stale informational* kinds
(a "due soon" after the date has passed) but never skips `missed` or `escalate`.

### 9.2 When a deadline changes under its reminders

| Change | What happens |
|---|---|
| Moved (`due_on`/`due_time`) | recompute `due_at`, `due_version++`, recompute `next_kind`/`next_fire_at`; an in-flight fire for the old version no-ops |
| Completed (directly, or by a step with `completes`) | `status = 'done'`, `done_at = now()`, `next_fire_at = NULL`, one transaction; after the date it reads "done late" |
| Cancelled / reopened | cancel clears the fire; reopen recomputes from now |
| Tenant changes time zone | batch job recomputes `due_at` for that tenant's open deadlines, bumping versions |
| Set to `missed` | by the dispatcher only, the morning after |

**Choice and why.** The version is the idempotency boundary: the dedupe key is
`deadline:kind:due_version:user`, so a retry of the same fire dedupes, while a moved deadline
legitimately fires again for its new date. Row locks serialise "complete" against "fire": the
"you missed it" email can never follow a completion that committed first. The seed's one-save
completion ("a step logged from the driveway closes the deadline") is just this transaction.

### 9.3 Who is told: ladder, preferences, fallback

| Option | Pros | Cons |
|---|---|---|
| Notify the owner only | trivial | an absent owner means a silent miss |
| Everyone with access | nobody misses | alert fatigue; people mute the product |
| **A ladder, filtered by each person's preferences, with a fallback** | right people at the right time; opt-outs respected | recipient resolution is a real query |

**Choice and why.** The ladder decides the *candidates*: `soon`/`today` → owner (else the item's
assignee); `missed` → owner and the manager role; `escalate` (after M days, default 3) →
`escalate_to` roles, **mandatory, ignoring opt-outs** (the seed's rule: the one alert that does not
ask). Preferences decide the *channel* per person: in-app, email now, daily digest, off, with "only
mine" (assigned to or made by me) and role defaults so a new manager hears the right things on day
one. The **fallback** is the seed's step 42: if *no* candidate has the action switched on, tell the
tenant's fallback list, so a deadline is never silent. Deactivated people drop out of the ladder.
Resolution is one join of candidates to `alert_prefs`; a missed deadline also repeats in the daily
digest each morning while it stays missed.

### 9.4 Time zones, end of day, DST

- Store the **IANA zone name** and the derived **UTC instant** (`due_at`). Then "missed" is
  `due_at < now()` for every tenant at once: the dispatcher needs no per-zone logic.
- Date-only means *through the end of that local day*: `due_at` = **the start of the next local
  day**, not "+24 h" (a day is 23 h or 25 h around DST).
- Fire times are *local* clock times (`work_start`, 06:00 so alerts land in the 07:00 digest, as in
  the seed): `time.Date(y, m, d, 6, 0, 0, 0, loc)`. In a DST gap the local time does not exist: run
  at the first valid instant after it; in a fold take the first occurrence ([15](15-job-scheduler.md) §9.4).
- `remind_days` counts **calendar days** here. Business days or working-hours SLA clocks need
  per-tenant holiday calendars: an extension, not a quiet default.
- Governments change DST rules; ship `tzdata` updates and recompute `due_at` of future open deadlines
  in affected zones.

```go
type Fire struct {
	Kind string
	At   time.Time
}

// plan returns the ordered fires for a deadline; dueOn carries the local calendar date, loc is the tenant zone.
func plan(dueOn time.Time, loc *time.Location, remindDays, escalateDays int) []Fire {
	y, m, d := dueOn.Date()
	at := func(off int) time.Time { return time.Date(y, m, d+off, 6, 0, 0, 0, loc) }
	var f []Fire
	if remindDays > 0 {
		f = append(f, Fire{"soon", at(-remindDays)})
	}
	f = append(f, Fire{"today", at(0)}, Fire{"missed", at(1)})
	if escalateDays > 0 {
		f = append(f, Fire{"escalate", at(escalateDays)})
	}
	return f
}
```

### 9.5 The 09:00 herd and duplicate messages

| Option | Pros | Cons |
|---|---|---|
| Fire everything at exactly `work_start` | simple | a ~450k spike in one minute per big zone |
| **Deterministic jitter**: `fire_at = base + (hash(id) mod 900) s` | spreads over 15 min: ~30k/min; stable across recomputes | fires up to 15 min after the nominal time |
| **Group per person** (digest, or one message per person per run) | ~45k recipient-items become ~7.5k messages | a short buffering window |
| Rate-limit and queue at the provider edge ([03](03-rate-limiter.md), [12](12-notification-service.md)) | protects providers | delays under load |

**Choice and why.** Jitter and grouping, with the notification service's queue as the buffer.
Duplicates come from three places and each has its own guard: two dispatchers claiming the same
row (`SKIP LOCKED` + lease), a crash after sending but before advancing (the outbox `dedupe_key`
makes the second insert a no-op, and the provider call carries an idempotency key), and a change
while a fire is in flight (`due_version`).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 300k fires/min at peak. Add dispatchers; shard the table by
  `tenant_id` hash and let each dispatcher claim its slice. The unjittered 09:00 spike is the real
  threat: widen the jitter window for the biggest zones.
- **Dispatcher down:** reminders are late, not lost; on recovery the backlog drains oldest-first with
  stale informational kinds skipped. Alert when `now() − min(next_fire_at)` exceeds 5 minutes.
- **Notification provider slow:** the outbox absorbs it; the digest grouping keeps volume low.
- **A giant tenant** (20M deadlines): per-tenant claim quotas so one tenant cannot starve the rest.
- **Clock skew:** use the database's `now()` for claims; never the worker's clock.
- **Single points of failure:** the primary (replicas serve the lists), the outbox relay (replicated workers).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Rolling `next_fire_at` | timers or a reminders table | one pending fire per row, trivial cancel | ~4 small updates per deadline |
| UTC `due_at` + IANA zone | local dates only | one global "missed" query, DST-safe | recompute when the zone or rules change |
| Dedupe key with `due_version` | at-least-once only | a move fires again, a retry does not | a unique index on the outbox |
| Ladder × preferences × fallback | notify everyone / owner only | right people, opt-outs, never silent | a recipient query per fire |
| Jitter + per-person grouping | exact-time fires | flat load, fewer messages | reminders up to 15 min off the nominal time |
| Mandatory escalation | fully optional alerts | a missed deadline always reaches a manager | some people get an alert they did not choose |

## 12. Check yourself

1. Why a rolling `next_fire_at` instead of a timer per deadline? What does the claim query look like?
2. A deadline is completed while a "missed" reminder is being fired. How is the wrong email prevented?
3. The deadline is moved to a later date after "due soon" fired. What happens next?
4. Who is told when an escalated deadline has an owner who muted everything? What if *nobody* has the alert on?
5. Why store `due_at` in UTC *and* an IANA zone, and how do you compute "end of day" on a DST day?

<details><summary>Answers</summary>

1. 100M timers cannot be rebuilt on restart or sharded easily. One pending fire per row with a
   partial index on `next_fire_at` is cheap to cancel (`NULL`) and reuses the scheduler pattern: claim
   `WHERE next_fire_at <= now() … LIMIT 1000 FOR UPDATE SKIP LOCKED` and push the time forward as a lease.
2. The fire transaction locks the row and re-checks `status = 'open'` (and `due_version`). Whichever of
   "complete" and "fire" gets the lock first wins; if completion committed first, the fire sees `done`
   and sends nothing.
3. `due_version` is bumped and the next fire is recomputed from now for the new date, so a fresh
   `soon` fires at the new time. The old dedupe key contains the old version, so the new fire is not
   suppressed, and any in-flight fire for the old version no-ops.
4. Escalation is mandatory: the `escalate_to` roles are told whatever they chose. If no candidate
   has the action on at all, the tenant's fallback owner list is told, so it is never silent.
5. The instant makes "missed" a plain `due_at < now()` query for all tenants; the zone name lets you
   recompute when tzdata changes. End of day = the **start of the next local day** built with
   `time.Date(y, m, d+1, 0, 0, 0, 0, loc)`, not "+24 h", because the day is 23 or 25 hours long on a DST change.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) (the ticker loop, graceful shutdown that finishes the batch), [44 Background jobs](../../44-background-jobs-queues.md), [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE SKIP LOCKED`, leases), [34 Outbox](../../34-event-driven-outbox.md) (dedupe keys), [36 Resilience](../../36-resilience-patterns.md) (backoff, jitter), [58 Real-time](../../58-realtime-websockets-sse.md) (the in-app channel).
- Dispatcher shape, as in [15](15-job-scheduler.md): `claim(ctx) ([]Row, error)` (the statement above), `fire(ctx, row)` in its own transaction with `errgroup` and a concurrency limit, `for { select { case <-ticker.C: … case <-ctx.Done(): return } }`.
- Test the clock, not the wall: inject a `Now func() time.Time` (the seed's worker uses an injected clock) and test the DST dates with `time.LoadLocation("America/New_York")`, the second Sunday of March and the first Sunday of November.
- Packages: `pgx`, `golang.org/x/sync/errgroup`, `time` (zone data via `time/tzdata` for static binaries), `hibiken/asynq` if the relay uses a queue.

## 14. Extensions

- **Recurring deadlines** (RRULE): generate the next occurrence when one completes; never pre-generate years.
- **Business-hours SLA clocks:** pause outside working hours and on holidays, with per-tenant calendars.
- **Dependencies:** deadline B starts when A is done; move A and everything after it shifts.
- **Client-facing deadlines:** a portal and SMS, with consent and quiet hours.
- **Calendar feeds:** an ICS feed per person; cache by `ETag`.
- **Weekly look-ahead** for managers (the seed's Monday email) as one more fire kind per person, not per deadline.
- **Health dashboard:** dispatcher lag, late-fire histogram, and a "silent deadlines" report (open, past due, nobody told).
