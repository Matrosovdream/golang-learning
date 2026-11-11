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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Workflow API (Go, stateless) | 3k moves/s peak (600 avg); reads assumed 3× the moves = 9k/s (section 4 sizes only writes) | CPU | moves 3k × 0.5 ms = 1.5 vCPU; reads 9k × 0.3 ms = 2.7 vCPU; total 4.2. N = ceil(4.2 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.97) = 2 → 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | JSON, auth, a guard check and one SQL statement; the 56 MB workflow config is the only state, so `c`, not `m`. 35 % busy at peak, 52 % with a zone down |
| Postgres (tickets, status_history, outbox) | 3k moves/s = ~9k row writes/s (record + history + outbox); records 50 GB; history ≈ 3.1 TB (13 months × 240 GB) | memory, then IOPS | hot set = records 50 GB + their indexes ~15 GB + the recent-ticket tail of the history index ~12 GB (assumed) ≈ 77 GB ⇒ 128 GiB. IOPS ≈ 3 per move (WAL flush + record page + history-index page, assumed) = 9k + ~2k miss reads ≈ 11k ⇒ `gp3` 14,000 IOPS (cap 16,000) on 4 TB. CPU 3k × ~1.5 ms = 4.5 vCPU of 16 | `db.r7g.4xlarge` (16 vCPU / 128 GiB), RDS Multi-AZ (self-managed: `r7g.4xlarge`) | primary + sync standby; reads on 1 replica `db.r7g.2xlarge` | RAM keeps the whole hot set cached (`r7g.2xlarge` has 64 GiB, not enough); ~11k IOPS is within 70 % of gp3's 16k cap, beyond that `io2` or `i4i`. This is the lesson's "edge of one primary": 10× means shards |
| Outbox relay + dwell rollups (Go workers) | 3k outbox rows/s peak | CPU | relay 3k × 0.2 ms + rollups 3k × 0.2 ms = 1.2 vCPU. N = ceil(1.2 ÷ 1.07) = 2 → 3 | `c7g.large` (2 vCPU / 4 GiB) | 3 | small CPU work; a stopped relay makes effects late, not lost |
| Event broker (the lesson only says "outbox relay ─►"; assumed Kafka-compatible) | 3k events/s peak × ~300 B ≈ 0.9 MB/s (0.18 MB/s avg) | none: far below "tens of MB/s per partition" | 7-day retention = 0.18 MB/s × 604,800 s ≈ 110 GB × 3 replicas ≈ 330 GB | `kafka.m7g.large` (2 vCPU / 8 GiB), MSK | 3 brokers, one per zone | present for availability (replication 3), not load; `m`, since neither disk, network nor memory is stressed |

The 56 MB workflow config sits in each API node's memory. History older than 13 months is detached and archived to S3: ~2.8 TB a year ≈ $65 a month per year kept.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Workers: 3 × `c7g.large` | $160 |
| MSK: 3 × `kafka.m7g.large` (3 × ≈ $120) + ~330 GB storage ≈ $30 | $390 |
| Postgres primary: `db.r7g.4xlarge` ≈ $1,240, Multi-AZ doubles | $2,480 |
| Replica: `db.r7g.2xlarge` | $620 |
| `gp3` 4 TB + 11k extra IOPS ($320 + $55 = $375) × 3 copies (primary, standby, replica) | $1,130 |
| ALB ($16 + ~7 LCU) | $60 |
| Egress: ≈ 5.2 TB × $0.09 (600 moves/s × 0.3 KB + 1.8k reads/s × 1 KB, assumed) | $470 |
| **Total** | **≈ $5,600** (≈ $4,200 with a 1-year plan on compute) |

- **Biggest line and why:** the database tier (~75 %: the primary alone ~44 %). The primary is sized by the 77 GB hot set and ~11k IOPS, and three 4 TB volumes cost $1,130 because 3.1 TB of the 3.2 TB is history. The cheap lever is the history window: keep 3 months hot, not 13, and the volumes drop to 1 TB (≈ $405), saving ≈ $650 net of S3.
- **What changes at 10×:** 30k moves/s ⇒ API 15 + 27 = 42 vCPU ⇒ N = ceil(42 ÷ 4.27) = 10 → 12 × `c7g.2xlarge`. 90k row writes/s ≈ 90k IOPS is past one primary and gp3's cap, so shard by `tenant_id` into 8 shards of today's shape (3.75k moves/s, ~11k IOPS, ~4 TB each ≈ 8 × $3,230 ≈ $26k). The broker stays small (9 MB/s).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The hot path is one move: a conditional write on one ticket row by primary key, plus a history row and an outbox row. At 3k moves/s that is ~9k row writes/s (section 4).
- History is the bulk: ~19B rows a year (~2.8 TB), append-only, partitioned by month; the live records are only 50 GB and the workflow config 56 MB.
- Rules the database itself must enforce: the status-and-version predicate, strict-mode transitions, "a status in use cannot be deleted" (composite foreign key), one initial status per workflow.
- Reads: lists by stage and status, "stuck for 3 days", time in status.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the whole move is one statement: `UPDATE … RETURNING` feeds the history `INSERT` and the outbox `INSERT` through data-modifying CTEs, so three writes commit together in one round trip (9.3); partial indexes (`one_initial_status`, and `tickets_stuck` that skips archived and deleted rows); declarative monthly partitions for `status_history` |
| MySQL (RDS / Aurora) | ✔ works, with a workaround | no `RETURNING`, and a CTE cannot hold an `UPDATE` or `INSERT`, so the move is a short transaction of separate statements; the `version` predicate still decides, so correctness is unchanged. Partial indexes need a generated column or a wider index. Range partitions exist, with the same rule that every unique key includes the partition key |
| DynamoDB | ✖ poor fit | the move itself maps well (a conditional write on status and version; `TransactWriteItems` over record, history and outbox is 3 items, limit 100), but "stage = open and status = waiting" lists, "stuck" queries and bulk moves by filter need GSIs keyed by tenant, so one big tenant meets the ~1,000 writes/s per partition limit |
| ClickHouse / Redshift / Athena | ✔ only as a derived store for history | 19B append-only rows aggregated by status and day is the shape it likes; the outbox rollups of 9.5 avoid needing it until ad-hoc dwell questions outgrow them; never the record |

**Pick:** PostgreSQL on RDS Multi-AZ, `db.r7g.4xlarge` with a `db.r7g.2xlarge` replica for lists and reports (section 15). It wins on the one-statement move with `RETURNING` and CTEs (9.3), on the composite foreign keys plus trigger-protected system rows that make illegal states unrepresentable (9.1), and on partial indexes and monthly partitions that carry ~3 TB of history. Records, history and outbox share one system of record, which is what makes the outbox transactional; the MSK broker, the dwell rollups, notifications and any search index are derived from the outbox. History older than 13 months leaves for S3 (section 15).

**If it were MySQL instead:** the move becomes `BEGIN`; conditional `UPDATE` (the strict-mode `EXISTS` subquery still works there); zero rows means the follow-up `SELECT` of the Go code; `INSERT` history; `INSERT` outbox; `COMMIT`. That is two more statements per move, ~6k more a second at the 3k/s peak, and SQL assembled in Go instead of one string. `one_initial_status` becomes a generated column that is `NULL` unless `is_initial`, with a unique index; `tickets_stuck` becomes a plain `(tenant_id, status, status_changed_at)` index that also holds archived rows. There is no vacuum to tune on the update-heavy `tickets` table. Workable, with more code on the hot path.

**What would flip the choice:** at 10× (30k moves/s, ~90k row writes/s) section 15 shards Postgres by `tenant_id` into 8. That is also where DynamoDB for tickets and history becomes a real option (the cheat sheet's threshold is tens of thousands of sustained writes/s), paid for with the list and report queries (OpenSearch or GSIs for lists, Athena for history). The cheaper step before it is moving closed months of history to S3 and Parquet.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 300k moves/s, 900k row writes/s; 5B active records ≈ 5 TB (about one per internet user), history ~1.9T rows ≈ 280 TB a year; 1M tenants, so 5.6 GB of workflow config; ~420 vCPU of API | the one primary: 900k row writes/s is ~90× its ~10k durable ceiling (and ~900k IOPS is ~56× a gp3 volume's 16k cap) | shard by `tenant_id` (every key already carries it) into ~80 shards of section 15's 10× shape (3.75k moves/s each) behind a cached `tenant → shard` directory; API N = ceil(420 ÷ 4.27) = 99 `c7g.2xlarge`, each caching only the workflows of its own tenants (5.6 GB no longer fits "in every node"); keep 3 months of `status_history` hot and detach older months to S3; per-tenant move quotas for the hot tenant. Give up: cross-tenant reports (warehouse only); a tenant bigger than a shard gets a shard of its own | ≈ $560k/month; the history volumes alone ≈ $110k |
| **1,000×** | 3M moves/s, 9M row writes/s; 50B active records ≈ 50 TB (six per person alive: a thought experiment), history ~19T rows ≈ 2.8 PB a year; 10M tenants, 56 GB of workflow config | running the fleet: ~800 shards is ~2,400 database nodes (primary, standby, replica), so one fails about every 2 h (5,000 nodes ≈ hourly); the `tenant → shard` map and the workflow config become a control plane nobody sized; history on three gp3 copies is ≈ $1.1M a month | independent **cells**, ~800 of them (the 100× shard shape, ~12k tenants and ~70 MB of config each), behind a thin cached `tenant → cell` router (a router outage freezes placement, not traffic). A different store for history: the outbox stream lands in S3 as Parquet (Athena or ClickHouse for audit and dwell) and a cell keeps 90 days, so the move writes only the record and its outbox row. Workflow edits, schema and code roll out cell by cell. Give up: history reads lag by seconds to minutes; a tenant never spans cells; cross-tenant analytics only via the warehouse | ≈ $5.6M/month, ≈ $1M less once history leaves the volumes |
| **10,000×** | 30M moves/s, 90M row writes/s (as much as the biggest event pipelines on earth move); 500B active records, history ~190T rows ≈ 28 PB a year | physics and money: ≈ $56M a month, and 30M moves/s is one move every 4 minutes for every person alive, around the clock, so it can only be machines (rules, integrations, bulk edits) | change the problem: record at the rate people act, not machines. Rule-driven and bulk moves apply as one set operation with one history row per batch (`tenant, rule, from, to, count, id range`); moves per second per tenant become a plan limit; dwell time comes from histograms per `(tenant, status, day)`, exact only for the last 30 days, not from a row per move; closed records past 90 days leave for Parquet. Give up: per-move history for automated moves, exact old percentiles | ≈ $56M/month if nothing changed; far less after batching |

**Arithmetic:** 3k × 100 = 300k moves/s × 3 rows = 900k row writes/s ÷ 10k = 90; 900k IOPS ÷ 16k ≈ 56; 4.2 vCPU × 100 = 420 ÷ 4.27 = 98.4 → 99; 300k ÷ 3.75k = 80 shards; 56 MB × 100 = 5.6 GB; 2.8 TB × 100 = 280 TB; 3M ÷ 3.75k = 800 cells × 3 nodes = 2,400 nodes ÷ 5,000 per hour ≈ one failure per 2 h; $1,130 × 1,000 ≈ $1.1M; 8B people ÷ 30M moves/s = one move per person per ~270 s; $5.6k × N.

**What stays the same at every scale:** stages in code and statuses as rows (the platform acts on `stage`, never on a tenant's label), and one conditional `UPDATE` with the `version` predicate plus an outbox row in the same transaction: a stale cache can never allow an illegal move.

**Say it in an interview:** "At 100× one primary sees 900k row writes a second, 90× its ceiling, so I shard by tenant id, which every key already carries, and each API node caches only its tenants' workflows. At 1,000× the problem is running ~2,400 database nodes and a 2.8 PB-a-year history, so I make cells behind a thin router and let history leave the primary as a stream into columnar storage. At 10,000× 30M moves a second can only be machines, so I batch and rate-limit them, and keep history as batch records and histograms."
