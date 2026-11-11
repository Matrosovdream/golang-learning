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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API + dispatchers + outbox relay (Go, stateless) | 500 fires/s peak (154/s avg); 750 outbox rows/s; API ~1k req/s (assumed, not in section 4) | CPU, tiny | fires 500 × 1 ms (recipient join, outbox rows, `time.Date`, ~5 round trips) = 0.5 vCPU; relay 750 × 0.2 ms = 0.15; API 1k × 0.3 ms = 0.3; total ≈ 0.95 vCPU. N = ceil(0.95 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | a fire is a ~10 ms transaction that mostly waits on Postgres, so 500/s is only ~5 in flight; memory under 1 GiB, so `c`. 16 % busy at peak, 24 % with a zone down: three nodes for availability, not load |
| Postgres (deadlines, outbox, prefs) | 100M open deadlines ≈ 25 GB; indexes ~10 GB (assumed); outbox ≈ 20M rows/day × 3 days of dedupe ≈ 10 GB (assumed); ≈ 50 GB in all; ~1,750 row writes/s at peak (2 updates + 1.5 outbox rows per fire) | memory (the whole database fits in RAM), then CPU | ≈ 50 GB < 64 GiB, so every page is cached. `next_fire_at` is indexed, so each update is not HOT and rewrites 4 index entries: ≈ 4 IOPS per fire ⇒ 500 × 4 = 2k peak, ~600 avg, inside the 3,000 included; provision 6,000 for the 09:00 herd. CPU 500 × ~3 ms = 1.5 vCPU of 8 | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ, `gp3` 150 GB at 6,000 IOPS (self-managed: `r7g.2xlarge`) | primary + sync standby | `r` because the point is to keep ~50 GB cached, with 8 GiB per vCPU while only ~2 vCPU are used; `r7g.xlarge` (32 GiB) would cache just the hot head, so try it only after measuring. IOPS and CPU are not the limit. Multi-AZ because a lost primary means silent deadlines |

Message delivery is the real cost and is sized in lesson 12, not here (see the bill note). S3 and a broker are not involved: the outbox is a table.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API + dispatchers + relay: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.2xlarge` ≈ $620, Multi-AZ doubles | $1,240 |
| `gp3` 150 GB + 3k extra IOPS ($12 + $15 = $27) × 2 for the standby | $50 |
| ALB | $20 |
| **Total** | **≈ $1,470** (≈ $980 with a 1-year plan on compute) |

Not in the total: delivery. 13.3M fires/day × 1.5 recipients ÷ ~6 per digest ≈ 3.3M messages/day ≈ 100M a month. If all were email at an SES-like ≈ $0.10 per 1,000 (outside the cheat sheet, re-check) that is ≈ $10k a month, about seven times the servers.

- **Biggest line and why:** the database (~84 %, ~88 % with its volume). Compute for 500 fires/s is almost nothing, so the bill is one Multi-AZ memory node. The lever is a smaller node, `db.r7g.xlarge` Multi-AZ at $620, if the measured hot set is only the head of `next_fire_at` and the open rows of active tenants. Saves $620.
- **What changes at 10×:** 5k fires/s ⇒ 5 + 1.5 + 3 = 9.5 vCPU ⇒ N = ceil(9.5 ÷ 2.13) = 5 → 6 × `c7g.xlarge` (≈ $630). One database no longer fits: 5k × 3 ms = 15 vCPU and 5k × 4 = 20k IOPS (gp3 caps at 16k), so shard by tenant hash into 3 × `db.r7g.2xlarge` Multi-AZ (≈ $3.7k, ~170 GB each, only the hot head cached). Widen the jitter window (the unjittered wave would be 4.5M in one minute) and expect delivery to reach ≈ $100k if it is all email.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 100M open deadlines ≈ 25 GB; with indexes and the outbox the whole database is ~50 GB, small enough to sit in RAM (section 15).
- The core access is a claim: "the 1,000 oldest rows with `next_fire_at <= now()`", leased, then fired one row per transaction. 154 fires/s on average, 500/s at peak (section 4).
- Each fire writes ~2 deadline updates and ~1.5 outbox rows (~1,750 row writes/s at peak), and the outbox `dedupe_key` must be unique.
- UI reads are "my overdue" and "this week" by tenant and due date; recipients are one join of candidates to `alert_prefs`.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `FOR UPDATE SKIP LOCKED` inside a CTE with `UPDATE … FROM … RETURNING` claims and leases 1,000 rows in one statement (9.1); three partial indexes (`WHERE next_fire_at IS NOT NULL`, and `WHERE status = 'open'` twice) keep each index to the rows that matter; row locks serialise "complete" against "fire" (9.2); unique `dedupe_key` with `ON CONFLICT DO NOTHING`; `text[]` for `escalate_to` |
| MySQL (RDS / Aurora) | ✔ works, small workarounds | `SKIP LOCKED` has existed since 8.0, so the dispatcher is portable: `SELECT … FOR UPDATE SKIP LOCKED`, then `UPDATE … WHERE id IN (…)` in the same transaction (the `SELECT` already returns what `RETURNING` did); partial indexes become plain or composite ones; `text[]` becomes a JSON array or a child table |
| DynamoDB | ✖ poor fit | 25 GB is tiny, so size is not the issue; the access pattern is: "everything due" needs a GSI with a sharded partition key (all due rows share one time order, and a partition takes ~1,000 writes/s) polled several ways, and recipient resolution is a join |
| Redis (ElastiCache) | ✖ as the record, ✔ as a derived due-index | a sorted set by `next_fire_at` could serve the claim, but claim, lease and state change would then span two stores and the set must be kept equal to the table (see 9.1); it can always be rebuilt from the table |

**Pick:** PostgreSQL on RDS Multi-AZ, `db.r7g.2xlarge` with `gp3` at 6,000 IOPS and no replica (section 15). What decides it: the claim, lease and `RETURNING` in one `SKIP LOCKED` statement, one database for deadlines and outbox so a state change and its outbox row commit together, and a 50 GB database that is fully cached. Postgres is the only store; the due-index is a partial index inside the table, so there is nothing separate to rebuild.

**If it were MySQL instead:** the closest to interchangeable of these lessons. The claim becomes two statements in one transaction (~30 claims a minute at peak, so one extra round trip is nothing). Run the dispatcher at `READ COMMITTED` so the range scan of the due-index takes no gap locks. The three indexes grow to hold done and cancelled rows, which costs disk rather than RAM because only the `<= now()` head is hot. In its favour: section 15 notes that Postgres rewrites 4 index entries on every `next_fire_at` update (it is not HOT), while InnoDB touches only the indexes that contain a changed column and has no vacuum to keep up with. Team skill and tooling are the tie-breakers.

**What would flip the choice:** 10× is 5k fires/s and ~20k IOPS, past the 16k `gp3` cap, so section 15 shards by tenant hash into 3 nodes of the same engine. A latency target tighter than the ~1 minute the lesson accepts (sub-second fires at thousands per second) would move the due-index into Redis, with the table still the record. Nothing at 500 fires/s needs that.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 50k fires/s at the peak (1.5M a minute; ~15k/s average), 10B open deadlines ≈ 2.5 TB (more than one per person alive), ~175k row writes/s, 75k recipients/s, ~12.5k messages/s after digests; ~95 vCPU of API, dispatch and relay | the one primary: 175k row writes/s is ~17× its ~10k ceiling, and ~200k IOPS (4 per fire) is 12× a gp3 volume's 16k cap | shard by hash of `tenant_id` into ~30 shards (~1.7k fires/s each: section 15's 10× shape), one dispatcher group per shard claiming its own due-head; API and workers N = ceil(95 ÷ 4.27) = 23 → 24 `c7g.2xlarge`; claim quotas per tenant for a 20M-deadline tenant; jitter window widened to an hour for the biggest zones (the unjittered 09:00 wave is 45M in a minute). Give up: reminders up to an hour off the nominal time; the "silent deadlines" report becomes a rollup across shards | ≈ $147k servers; delivery ≈ $1M if all email (10B messages a month at ≈ $0.10 per 1,000, outside the cheat sheet, re-check) |
| **1,000×** | 500k fires/s at the peak (30M a minute), 100B open deadlines ≈ 25 TB (12 per person alive: a thought experiment), ~1.75M row writes/s, ~125k messages/s ≈ 3.3B a day | delivery, a third-party limit and a bill: 100B messages a month ≈ $10M if all email, ~7× the ≈ $1.5M of servers, and 125k sends/s is far past what one provider account allows by default | change the shape: replace one fire per deadline with **set-based sweeps**. One job per `(tenant, kind, local date)` at the tenant's `work_start` reads `deadlines_due_idx` by `due_on` and sends one digest per person, so date-only deadlines stop rewriting `next_fire_at` (4 index entries per fire) and a move or completion needs no recompute; the sweep key plus `due_version` keep the dedupe, and only timed deadlines keep `next_fire_at`. ~300 independent cells (the 100× shard shape) behind a cached `tenant → cell` router, each with its own outbox and sender pool; in-app by default, email for digests and escalation. Give up: exact-minute fires for date-only deadlines, a checkpoint per tenant-day, "email me at once" per deadline | ≈ $1.5M servers; delivery up to ≈ $10M, far less with digests by default |
| **10,000×** | 5M fires/s at the peak, 1T open deadlines (125 per person alive), 12.5M recipients/s, ~1.25M messages/s ≈ 33B a day | money and attention: ≈ $100M a month of delivery plus ≈ $15M of servers, and 33B messages a day is four for every person alive from this one feature: nobody reads them | change the problem, ration: one digest per person per day; mandatory escalations capped per person per day (say 3) with the rest folded into the digest; in-app and push by default, email opt-in; deadlines more than 90 days past due auto-close as `missed`; an open-deadline cap per tenant plan; managers see a "silent deadlines" aggregate instead of per-item mail. Give up: "never silent" is guaranteed only for the `escalate` tier, not for every deadline | ≈ $115M/month if nothing changed; far less after rationing |

**Arithmetic:** 500 × 100 = 50k fires/s; 1,750 × 100 = 175k row writes/s ÷ 10k ≈ 17; 50k × 4 index entries = 200k IOPS ÷ 16k ≈ 12; 0.95 vCPU × 100 = 95 ÷ 4.27 = 22.3 → 23 → 24; 50k ÷ 1.7k = 30 shards; 450k × 100 = 45M in one minute; 100M messages a month × N × $0.10 per 1,000 = $1M, $10M, $100M; 3.3M a day × 10,000 = 33B ÷ 8B people ≈ 4; $1,470 × N.

**What stays the same at every scale:** a deadline is an absolute UTC instant plus an IANA zone (so "missed" is one comparison), and every message dedupes on a key that contains `due_version`: at-least-once fires, once in effect. Escalation stays mandatory.

**Say it in an interview:** "At 100× one primary sees 175k row writes a second, 17× its ceiling, so I shard by tenant and give each shard its own dispatcher claiming its own due-head. At 1,000× the cost is delivery, not the database, so date-only deadlines move to per-tenant-day sweeps that send one digest per person, inside independent cells with their own sender pools. At 10,000× that is four messages a day for every person alive, so I ration: one digest a day, capped escalations, in-app by default, and only the escalation tier is guaranteed."
