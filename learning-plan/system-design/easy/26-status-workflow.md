# 26 · Configurable Status Workflow

`🟢 easy` · *Topics:* statuses as data with system codes and stages, tenant-defined labels and transitions, one conditional `UPDATE` plus history in a single statement, optimistic versions and idempotent moves, in-transaction vs outbox side effects, orthogonal flags instead of status explosion, time-in-status reporting · *Builds on:* [09 Feature Flags](09-feature-flags.md) (config read on every request, cached by version), [10 Webhook Delivery](10-webhook-delivery.md) (outbox), [24 Data Lifecycle](24-data-lifecycle.md) (archive/remove as flags beside status)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [25 Contact Deduplication & Merge](25-contact-dedupe-merge.md) · Next: [27 Deadlines & Escalation](27-deadlines-escalation.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 11 moved request statuses into a table, `ref_request_statuses (slug, name, sort_order, stage open|won|lost, color, is_system, is_active)`: the owner renames, reorders and adds steps without a deploy; five seeded **system rows** are the codes the code acts on, protected by a trigger (the slug never changes; a system row's stage can't change and it is never retired or deleted); `requests.status` stores the *code itself* as a foreign key. Hooks read the code only for one exact step (`new` is the start) and read the **stage** for "any closed request" (entering stage `lost` releases a rental hold). Orders get the same treatment (`ref_order_statuses.is_closed`, `booking_status`). Step 14 audited ~45 hard-coded lists and generalised the rule. Step 64 made spam a verdict *beside* the pipeline (`is_spam`; status untouched), and step 16 kept the detailed story in a separate timeline of steps. The seed has no allowed-transitions table: any status can move to any other.
> **Here, at scale:** a ticketing/orders SaaS with 10k tenants, each with its own statuses and transition rules, 50M active records and 3k status changes a second at peak.

## 1. The task

> "Design the status workflow of a ticketing or orders product. Each customer defines their own
> statuses and which moves are allowed. Moving a record must be safe under concurrency, leave a
> history, and trigger the right side effects."

The trap is letting tenants rename *everything* while platform code still has to know what a
status *means* (is it closed? does the clock stop?).

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What can a tenant change? | Splits data from code. | Labels, order, colours, adding statuses, allowed transitions. Not the platform's stages. |
| Free movement or strict transitions? | Rules table or not. | Per workflow: *free* (any→any, as the seed) or *strict*. |
| Who may make a move? | Permissions. | Optional permission per transition. |
| One workflow per tenant? | Keys. | One per record type (tickets, orders). |
| Concurrent edits? | Conflict semantics. | The second mover gets a clear `409` with the current status. |
| What happens on a move? | Side effects design. | Notify, webhook, SLA timers, search index, a local invariant (release a hold). |
| History and reports? | Storage. | Every move kept 13 months hot; time-in-status per status. |
| Scale? | Sharding. | 10k tenants, 50M active records, 3k moves/s peak (avg 600/s). |

## 3. Requirements

**Functional**
- Tenants edit statuses (label, colour, order, stage) and, optionally, allowed transitions per workflow.
- Move a record from status A to B: validated against the workflow, permissions and guards.
- Every move writes a history row (who, from, to, when, note); replays are harmless.
- Bulk moves with per-record refusals; filters by status and by stage.
- Side effects after a move: notifications, webhooks, timers, index, rollups.

**Non-functional**
- A move is atomic and race-safe; an illegal move is impossible even with a stale cache.
- p99 under 50 ms; workflow edits visible to all servers within seconds.
- Platform behaviour never depends on a tenant-editable string.

**Out of scope**
- A visual workflow designer, approvals with multiple signers, cross-record workflows (sagas).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Workflow config | ~56 MB | 10k tenants × 2 workflows × (~8 statuses + ~20 transitions) × ~100 B: fits in every server's memory |
| Active records | 50M ≈ 50 GB | × ~1 KB |
| Moves | 600/s avg, 3k/s peak | peak = 5× |
| Records created | ~100/s | 600 moves/s ÷ ~6 moves per record (avg lifetime ≈ 6 days: 50M ÷ 8.6M a day) |
| History rows | ~19B a year ≈ 2.8 TB | 600 × 31.5M s; ~150 B a row with index |
| …per month | ~1.6B rows ≈ 240 GB | partition by month; keep 13 months hot |
| Row writes at peak | ~9k/s | 3k moves × 3 rows (record, history, outbox) |

What the numbers tell us: config is tiny and cacheable. The record table is small. The big thing
is **history** (terabytes a year: partition, roll up, archive) and **write volume**: 9k row
writes/s is at the edge of one primary, so the design must shard by `tenant_id` at about 10×.

## 5. Plan

1. Split vocabulary: platform **stages** in code, tenant **statuses** as rows pointing at a stage.
2. Model workflows, statuses, transitions, history; protect the rows the platform depends on.
3. Make the move one conditional statement: validation, version check, history, outbox.
4. Decide what happens in the transaction and what happens after (outbox).
5. Keep statuses coarse: flags beside the status, a timeline for detail; report from rollups.

## 6. High-level design

```
 client ──► API ──► workflow cache (per workflow_id, validated by workflows.version)
                      │
                      ▼   ONE statement per move
              Postgres: tickets (status, version) · status_history · outbox   [sharded by tenant_id at 10×]
                      │  (+ in the same transaction: local invariants, e.g. release a booking hold)
                      ▼
                  outbox relay ──► notifications (45) · webhooks (10) · SLA timers (27) · search · dwell rollups
 admin UI ──► PUT /workflows/{id} (base_version) ──► statuses + transitions rows ──► version++ ──► caches refetch
```

The cache serves the UI (which buttons to show) and early rejection. **The database statement
enforces the rules**, so a stale cache can never allow an illegal move.

## 7. API

```
GET  /workflows/{entity}                          → 200 { id, version, mode, statuses[], transitions[] }   ETag = version
PUT  /workflows/{id}  { base_version, statuses[], transitions[] } → 200 | 409 stale | 422 { reason: "system_status" | "status_in_use" }
POST /tickets/{id}/transition { to, expected_from, version, note? }   Idempotency-Key
        → 200 { status, history_id }
        | 409 { current_status }                        someone moved it first
        | 422 { reason: "transition_not_allowed" | "guard_failed:needs_assignee" }  | 403
POST /tickets/transition-bulk { ids[] | filter_token, to } → 200 { done[], refused[{ id, reason }] }
GET  /tickets?stage=open&status=waiting_vendor&cursor=…
GET  /tickets/{id}/history
GET  /reports/time-in-status?workflow=&from=&to=  → [{ status, p50_s, p90_s, exits }]
```

## 8. Data model

```sql
CREATE TABLE workflows (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL,
  entity_type text NOT NULL, mode text NOT NULL DEFAULT 'free' CHECK (mode IN ('free','strict')),
  version integer NOT NULL DEFAULT 1,                         -- bumped by every edit: the cache key
  UNIQUE (tenant_id, entity_type)
);

CREATE TABLE statuses (
  workflow_id bigint NOT NULL REFERENCES workflows(id),
  code        text   NOT NULL,            -- stable, never renamed: records store it
  label       text   NOT NULL,            -- tenants rename freely
  stage       text   NOT NULL CHECK (stage IN ('open','waiting','done','cancelled')),  -- what the PLATFORM acts on
  is_initial  boolean NOT NULL DEFAULT false, is_system boolean NOT NULL DEFAULT false,
  sort_order  integer NOT NULL, color text, is_active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (workflow_id, code)
);
CREATE UNIQUE INDEX one_initial_status ON statuses (workflow_id) WHERE is_initial;

CREATE TABLE status_transitions (        -- only consulted when workflows.mode = 'strict'
  workflow_id bigint NOT NULL, from_code text NOT NULL, to_code text NOT NULL,
  required_permission text,              -- NULL: anyone who may edit the record
  guard text,                            -- a NAMED guard implemented in code: 'needs_assignee', 'needs_resolution'
  PRIMARY KEY (workflow_id, from_code, to_code),
  FOREIGN KEY (workflow_id, from_code) REFERENCES statuses (workflow_id, code),
  FOREIGN KEY (workflow_id, to_code)   REFERENCES statuses (workflow_id, code)
);

CREATE TABLE tickets (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL, workflow_id bigint NOT NULL,
  status text NOT NULL, status_changed_at timestamptz NOT NULL DEFAULT now(),
  version integer NOT NULL DEFAULT 0, assignee_id bigint, is_spam boolean NOT NULL DEFAULT false,
  archived_at timestamptz, deleted_at timestamptz,        -- orthogonal to status (see 24)
  FOREIGN KEY (workflow_id, status) REFERENCES statuses (workflow_id, code)
);
CREATE INDEX tickets_stuck ON tickets (tenant_id, status, status_changed_at)
  WHERE archived_at IS NULL AND deleted_at IS NULL;       -- "Contacted for more than 3 days"

CREATE SEQUENCE status_history_id_seq;
CREATE TABLE status_history (
  id bigint NOT NULL DEFAULT nextval('status_history_id_seq'), ticket_id bigint NOT NULL, tenant_id bigint NOT NULL,
  from_code text NOT NULL, to_code text NOT NULL, changed_by bigint, changed_at timestamptz NOT NULL DEFAULT now(),
  request_id uuid, note text, PRIMARY KEY (id, changed_at)
) PARTITION BY RANGE (changed_at);                        -- monthly partitions
CREATE INDEX status_history_ticket ON status_history (ticket_id, changed_at DESC);
```

## 9. Deep dives

### 9.1 Statuses as data, stages as code

| Option | Pros | Cons |
|---|---|---|
| Enum / `CHECK` in code | simple, fast | every new status is a deploy; 10k tenants cannot have their own |
| Free-form table, code reads the names | tenants can do anything | a renamed `closed` silently breaks SLA, billing and reports |
| **Table with system rows and a `stage` column** | tenants rename and add; the platform reads `stage` | the vocabulary of stages must be designed well and grows slowly |

**Choice and why.** The seed's rule, generalised: **a value the code acts on is a *system* row**; its
`code` never changes, it is never retired or deleted, and a trigger enforces that. **Behaviour that
means "a kind of thing" reads a column (`stage`, `is_initial`), never a code.** SLA clocks run in
`open`, pause in `waiting`, stop in `done`/`cancelled`; "sending a quote moves a record from *any open
status sorted before it*" is a rule written in `stage` and `sort_order`, not in names. Records store the
**code** (composite FK to `(workflow_id, code)`), so SQL and Go read `WHERE status = 'new'` with no lookup;
a status in use cannot be deleted (the FK) and is **retired** (`is_active = false`: kept on old records,
hidden from pickers). Go keeps constants for the system codes and a test holds them equal to the seeded rows.

### 9.2 Transition rules: free, strict, or code

| Option | Pros | Cons |
|---|---|---|
| **Free** (any→any, the seed) | zero configuration; fine for a small team | no process control; mistakes are one click |
| **Strict, as data** (`status_transitions`) | tenants shape their own process; auditable; queryable ("who can reach `done`?") | a table to edit and cache; guards limited to named ones |
| State machine in code (State pattern) | expressive, testable | every tenant's process is a deploy |
| Tenant-supplied scripts | maximum flexibility | a security and support nightmare |

**Choice and why.** A per-workflow switch: `free` or `strict`. In strict mode transitions are rows,
each with an optional permission and a **named guard** the platform implements (`needs_assignee`,
`needs_resolution`). Tenants choose from the list; they never upload logic. Entering `done` can
require a resolution note without anyone writing code.

### 9.3 One move = one conditional statement

```sql
WITH moved AS (
  UPDATE tickets t
     SET status = $to, status_changed_at = now(), version = t.version + 1
   WHERE t.id = $id AND t.tenant_id = $tenant AND t.status = $from AND t.version = $version
     AND ( (SELECT w.mode FROM workflows w WHERE w.id = t.workflow_id) = 'free'
           OR EXISTS (SELECT 1 FROM status_transitions x
                      WHERE x.workflow_id = t.workflow_id AND x.from_code = $from AND x.to_code = $to) )
  RETURNING t.id, t.workflow_id
), hist AS (
  INSERT INTO status_history (ticket_id, tenant_id, from_code, to_code, changed_by, request_id, note)
  SELECT id, $tenant, $from, $to, $actor, $request_id, $note FROM moved RETURNING ticket_id
)
INSERT INTO outbox (topic, payload)
SELECT 'ticket.status_changed', jsonb_build_object('ticket_id', id, 'from', $from, 'to', $to) FROM moved;
```

(Parameters are shown by name; in Go they are `$1…$8`.) Zero rows moved means one of three things;
a follow-up `SELECT` tells which: **stale** (`status` or `version` changed →
`409` with the current status, the UI refreshes), **not allowed** (`422`), **gone** (`404`). Guards
and permissions are checked in Go against the row just read; the `version` predicate guarantees that
row has not changed since. The cache decides which buttons to *show*; this statement decides what is
*allowed*. **Idempotency:** the `version` predicate already makes a replay a no-op (zero rows); the
follow-up read sees `status = $to` and a newest history row carrying the same `request_id`, and answers
`200` with that `history_id`. (A unique key on `request_id` is impossible here: unique constraints on a
partitioned table must include the partition key.)

### 9.4 Side effects, and flags that are not statuses

| Effect | Where |
|---|---|
| A local invariant: release a booking hold when entering stage `cancelled`, re-hold when leaving it | **same transaction** (it must never disagree with the status) |
| Notify the assignee, webhook, SLA timer, search index, rollups | **outbox**, after commit ([10](10-webhook-delivery.md), [45](45-live-notifications.md), [27](27-deadlines-escalation.md)) |

| Modelling | Pros | Cons |
|---|---|---|
| Encode everything in the status (`quoted-spam`, `quoted-archived`, …) | one column | states × flags explode; "Not spam" cannot return to the old status |
| **Orthogonal columns** (`is_spam`, `archived_at`, `deleted_at`) beside a coarse status | status unchanged by verdicts; "Not spam" puts it back exactly | queries repeat predicates ([24](24-data-lifecycle.md)) |
| One state machine per dimension (payment status, fulfilment status) | each stays small | two columns to keep consistent |

**Choice and why.** The seed treats spam as a verdict **beside** the pipeline: the record keeps its
status, so un-marking restores it. Keep statuses **coarse** (a handful per workflow, cap ~30): they
route work and feed reports. The detailed story ("framing inspected", with photos) is a **timeline**
of free-form events, not more statuses.

### 9.5 Time in status at 19B rows

| Option | Pros | Cons |
|---|---|---|
| `lead(changed_at) OVER (PARTITION BY ticket_id …)` on history | exact, flexible | scans billions of rows |
| Keep `status_changed_at` on the record | "how long in this status *now*" and a stuck-record index | no past dwell |
| **Rollups from the outbox**: `dwell(tenant, day, workflow, status, exits, total_seconds, histogram)` | cheap p50/p90 for any range | a consumer; late events and corrections |

**Choice and why.** `status_changed_at` + the partial index for "stuck" queries (the seed measures
this at 0.02–0.06 ms), plus a rollup consumer for history; the dwell of a move is
`changed_at − previous changed_at`, computed once when the event is consumed, never re-scanned.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 90k row writes/s. Shard by `tenant_id` (every key already carries it);
  partition `status_history` monthly and drop or archive old partitions; run rollups off the outbox.
- **Hot tenant:** one tenant's automation moves 5M records. Per-tenant rate limit ([03](03-rate-limiter.md)),
  bulk moves in turns of 200 through the queue, fair scheduling between tenants.
- **Workflow edit while moves are in flight:** the edit bumps `version`; moves are validated by the
  database against the *current* rows, so an in-flight move either sees the old or the new rule, never a mix.
- **Cache stale or lost:** UI shows a button that then fails with `422`; correctness is unaffected.
- **Outbox relay down:** side effects are late, not lost; alert on the oldest unsent row.
- **Deleting a status in use:** refused (`status_in_use`); the tenant bulk-moves records first, or retires it.
- **Single point of failure:** the shard's primary; reads (lists, reports) go to replicas.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Stages in code, statuses as rows | tenant-defined everything | the platform never breaks on a rename | a deliberately small stage vocabulary |
| Status stores the code (composite FK) | a numeric status id | readable SQL, no lookup, measured indexes | a longer key; codes can never be renamed |
| Strict mode as data + named guards | free-form scripts | tenants shape the process safely | limited expressiveness |
| One conditional statement | read-check-write in app code | atomic, race-safe, retry-safe | SQL that must be read carefully |
| Flags beside a coarse status | status explosion | verdicts reversible, reports simple | repeated predicates |
| Rollups for dwell time | scanning history | cheap reports at 19B rows | a consumer and correction logic |

## 12. Check yourself

1. Why does platform code read `stage` instead of the status code or label? What is a *system* status?
2. What does zero rows from the move statement mean, and how do you tell the user?
3. A tenant wants to delete a status 40,000 records use. What happens?
4. Why is "spam" not a status? What does the seed do instead?
5. At 3k moves/s how many row writes is that, what limits you first, and how do you report time-in-status over 19B history rows?

<details><summary>Answers</summary>

1. Tenants rename and add statuses freely, so a label or a code like `closed` can mean anything.
   `stage` is the platform's closed vocabulary (open, waiting, done, cancelled) that SLA, billing and
   reports act on. A system status is a seeded row the code depends on (`new`, the initial one): fixed
   code and stage, never retired or deleted, enforced by a trigger.
2. The record was not in `$from` at that `version`, or the transition isn't allowed, or it is gone.
   A follow-up `SELECT` decides: stale → `409` with the current status; not allowed → `422`; gone → `404`.
3. The foreign key refuses the delete. The tenant either retires it (`is_active = false`: old records
   keep it, pickers hide it) or bulk-moves the 40,000 records to another status first, in turns of 200.
4. A verdict is independent of the pipeline: encoding it in status multiplies states and can't restore
   the previous status. The seed adds `is_spam`, leaves the status alone, so "Not spam" puts the record
   back exactly where it was.
5. 3k × 3 rows (record, history, outbox) = ~9k writes/s, at the edge of one primary: shard by tenant.
   History is ~19B rows a year, so partition monthly and compute dwell from rollups fed by the outbox,
   plus `status_changed_at` for "stuck now".

</details>

## 13. In Go

- Related lessons: [31 DDD tactical](../../31-ddd-tactical.md) (the aggregate that guards "move"), [30 Behavioral patterns](../../30-patterns-behavioral.md) (State), [59 Production DB patterns](../../59-production-database-patterns.md) (optimistic `version`, `ON CONFLICT`, partitioning), [34 Outbox](../../34-event-driven-outbox.md), [41 API design](../../41-api-design-evolution.md) (`409` vs `422`, idempotency keys), [12 Errors](../../12-errors.md) (typed errors).

```go
var (
	ErrStale      = errors.New("record changed")       // → 409
	ErrNotAllowed = errors.New("transition not allowed") // → 422
	ErrGone       = errors.New("record not found")     // → 404
)

func Move(ctx context.Context, tx pgx.Tx, m MoveCmd) error {
	// $1 to, $2 id, $3 tenant, $4 from, $5 version, $6 actor, $7 request id, $8 note
	tag, err := tx.Exec(ctx, moveSQL, m.To, m.ID, m.Tenant, m.From, m.Version, m.Actor, m.RequestID, m.Note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil // history + outbox rows were written by the same statement
	}
	var cur string
	switch err := tx.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1 AND tenant_id = $2`, m.ID, m.Tenant).Scan(&cur); {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrGone
	case err != nil:
		return err
	case cur != m.From:
		return fmt.Errorf("%w: now %q", ErrStale, cur)
	}
	return ErrNotAllowed
}
```

- Packages: `pgx`, `jackc/pgerrcode` (trigger `check_violation`), `golang.org/x/sync/singleflight` for the workflow cache refetch.

## 14. Extensions

- **Approvals:** a transition that needs N approvers (a sub-state with its own votes).
- **Automations:** "when entering X, assign to Y": a rules table evaluated off the outbox (never inline).
- **Workflow versions:** publish drafts, migrate in-flight records, keep old versions readable.
- **SLA by stage** with business-hours calendars ([27](27-deadlines-escalation.md)).
- **Cross-record workflows** (order → fulfilment → invoice) as a saga ([35 Sagas](../../35-sagas-distributed-transactions.md)).
- **Per-status field requirements** (a form per status) validated at the same statement.
