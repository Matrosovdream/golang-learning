# 24 · Data Lifecycle: Archive, Delete, Erase

`🟢 easy` · *Topics:* archive vs soft delete vs hard delete vs anonymise, the Trash and a purge window, partial indexes that ignore dead rows, batched purges without table bloat, erasure across caches/search/object storage/backups/logs, keeping money records when a parent goes · *Builds on:* [02 Pastebin](02-pastebin.md) (expiry & cleanup), [10 Webhook Delivery](10-webhook-delivery.md) (outbox fan-out), [15 Job Scheduler](15-job-scheduler.md) (batched, resumable jobs)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [23 Paginated Lists at a Billion Rows](23-paginated-lists.md) · Next: [25 Contact Deduplication & Merge](25-contact-dedupe-merge.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 25.4 + migration 0053 give twelve kinds of record two words with one meaning each. **Archive** (`archived_at`) = out of the default list and counts, still searchable, history intact. **Remove** (`deleted_at`) = into a Trash for 30 days, restorable, then the worker's daily `records.purge` deletes it *where nothing points at it*. Money is never removed (invoices are archive-only; `RESTRICT` keys stand behind it). The lists' indexes became partial (`WHERE archived_at IS NULL AND deleted_at IS NULL`). Step 65 + migration 0083 do the same for people: `users.email` is unique among live rows only, and after the purge the history reads "a former user" (`ON DELETE SET NULL`). The plan also names *erasing a client on request* (7g) as a separate thing from Remove — but never builds it, which is the gap this lesson fills.
> **Here, at scale:** a SaaS with 500M records, 10M archive-eligible rows a month, and privacy-erasure requests (GDPR/CCPA) arriving every few minutes that must reach a database, replicas, a search index, caches, object storage, logs, analytics and backups.

## 1. The task

> "Design how our product handles end-of-life data. Users can archive and delete things and get
> them back; old data is cleaned up automatically; and when a person asks us to erase their data,
> we must really do it, everywhere, and be able to show that we did."

The word "delete" hides four different operations. The task is to give each its own meaning and
its own machinery, and to find every place a record is secretly copied.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What states do users see? | Decides columns, indexes, UI. | Active, Archived (hidden, searchable), Removed (Trash, 30 days), gone. |
| Can everything be removed? | Money and legal records must survive. | No: records with invoices/payments are archive-only; the rest can be removed. |
| Is "erase my data" the same as Remove? | Legal vs product concept. | No. Erasure is a privacy request with a deadline and proof; Remove is a convenience. |
| What must be kept after erasure? | Tax and accounting retention, opt-out lists. | Financial rows (anonymised) and a hashed suppression entry so unsubscribes stay honoured. |
| How long do backups live? | Backups still hold erased data. | 35 days; restores replay an erasure ledger. |
| Scale? | Purge strategy. | 500M records, 10M archive-eligible a month, 2M removes a month, ~140 erasure requests a day. |

## 3. Requirements

**Functional**
- Archive / unarchive any record; archived records leave default lists and counts but stay searchable and intact.
- Remove into a Trash, restore within 30 days, then purge automatically; "Delete now" for admins.
- Refuse removal when something that must be kept points at the record, and say why.
- Erasure requests: find every copy of a person's data, erase or anonymise it, record evidence.

**Non-functional**
- Default list queries must not slow down as dead rows pile up.
- Purges and retention jobs must not hurt p99 latency of normal traffic or stall replicas.
- Erasure completes within the legal window (about a month; we target 7 days) and is provable.
- Every state change is audited; restore is lossless.

**Out of scope**
- The legal analysis itself (not legal advice), consent management, data-subject access exports.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Live records | 500M | given |
| Main-table storage | ~0.5 TB | 500M × ~1 KB (row + indexes, rule of thumb) |
| With child tables and media rows | ~2 TB | assume children ≈ 3× the main rows' bytes |
| Archive-eligible rows | 10M / month ≈ 4 / s | 10M ÷ 2.6M s |
| …if run as one nightly batch | ~2,800 rows / s | 10M in a one-hour window |
| Removes into the Trash | 2M / month (0.4 % of rows) | assumption; Trash steady state ≈ 2M rows |
| Purge batches | 2,000 per 10M rows | batches of 5,000 rows |
| Erasure requests | ~140 / day | 100M people × 0.05 % a year = 50k ÷ 365 |
| Erasure work items | ~1,100 / day | 8 systems × 137; ≈ 0.013 / s |
| Autovacuum default trigger | 100M dead rows | `autovacuum_vacuum_scale_factor` 0.2 × 500M |

What the numbers tell us: nothing here is a throughput problem. Erasure is ~1 request per 10
minutes; the purge is ~4 rows/s on average. The hard parts are **semantics** (what each state
means), **completeness** (forgetting one of eight stores), and **not hurting the primary** with
10M-row deletes: the default autovacuum threshold alone lets 100M dead rows accumulate first.

## 5. Plan

1. Define the four end-of-life states and what each means to the app, the UI and reports.
2. Model them: nullable timestamps + who, partial indexes so default views ignore dead rows.
3. Guard removal: what may never be removed, and refusals per row.
4. Build the retention jobs: Trash purge, archive-to-cold, log/event expiry, without bloat.
5. Build the erasure pipeline (orchestrator, handlers, ledger, verification) and decide what happens to children when a parent disappears.

## 6. High-level design

```
 users ──► API ──► Postgres (live + archived + removed rows, same tables)
                    │   archived_at / deleted_at, partial indexes
                    │
                    ├─► audit_log (every state change, same transaction)
                    └─► outbox ──► relay ──► erasure orchestrator ──┬─► search index (delete doc)
                                                                     ├─► cache / CDN (purge keys, URLs)
                                                                     ├─► object storage (all versions)
                                                                     ├─► analytics warehouse (delete / unlink)
                                                                     ├─► email provider (suppression, not data)
                                                                     └─► third-party processors (their delete API)
 worker (off-peak):  trash.purge ──► batched DELETE ... SKIP LOCKED   (Postgres)
                     archive.cold ─► export Parquet to object storage, then delete
                     partitions.drop ─► DROP old monthly partitions of logs/events
 backups: expire after 35 days; restore ⇒ replay erasure_requests ledger
```

Removing: one UPDATE sets `deleted_at` and writes an audit row. Purging: the worker deletes rows
whose 30 days are up and that nothing points at. Erasing: a request row, then one idempotent outbox task per store, evidence in the ledger.

## 7. API

```
POST /records/{kind}/{id}/archive               → 204          (Undo = POST …/unarchive)
POST /records/{kind}/{id}/remove                → 200 { purge_after } | 409 { reason: "has_invoice" | "being_edited" | … }
POST /records/{kind}/{id}/restore               → 200 | 409 { reason: "email_taken" }
GET  /trash?kind=&cursor=                       → 200 { items[{ kind, id, label, removed_by, removed_at, purge_after }], next }
POST /records/bulk  { action, ids[] | filter_token }   → 200 { done[], refused[{ id, reason }] }   (max 2,000, applied in turns of 200)
POST /privacy/erasure-requests { subject: { email | user_id } }   → 202 { request_id }
GET  /privacy/erasure-requests/{id}             → 200 { status, deadline, steps[{ system, status, done_at }] }
```

## 8. Data model

```sql
-- on every removable kind: nullable, no default => metadata-only ADD COLUMN, no table rewrite
ALTER TABLE orders
  ADD COLUMN archived_at timestamptz,
  ADD COLUMN archived_by bigint REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN deleted_at  timestamptz,
  ADD COLUMN deleted_by  bigint REFERENCES users(id) ON DELETE SET NULL;

-- the default list reads only live rows; the planner uses this index only when the query's WHERE implies the predicate
CREATE INDEX orders_list_idx     ON orders (tenant_id, created_at DESC, id DESC)
  WHERE archived_at IS NULL AND deleted_at IS NULL;
CREATE INDEX orders_archived_idx ON orders (tenant_id, archived_at DESC) WHERE archived_at IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX orders_deleted_idx  ON orders (deleted_at) WHERE deleted_at IS NOT NULL;      -- the Trash and the purge

-- uniqueness only among live rows: a removed account frees its email, Restore must re-check
CREATE UNIQUE INDEX users_email_live_key ON users (lower(email)) WHERE deleted_at IS NULL;

-- a child that can block a purge needs an index on its FK column, or the "still referenced?" probe scans the table
CREATE INDEX invoices_order_idx ON invoices (order_id);

-- tombstone for records that must stay (anonymised, not deleted)
ALTER TABLE clients ADD COLUMN erased_at timestamptz;   -- name/email/phone/address nulled or replaced with 'Erased'

CREATE TABLE erasure_requests (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  subject_key bytea  NOT NULL,                         -- keyed hash of the identifier (HMAC), never the plain email
  received_at timestamptz NOT NULL DEFAULT now(),
  deadline    timestamptz NOT NULL,
  status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','partial','failed'))
);
CREATE TABLE erasure_steps (
  request_id bigint NOT NULL REFERENCES erasure_requests(id),
  system     text   NOT NULL,                          -- postgres | search | cache | cdn | objects | warehouse | email | processor:<name>
  status     text   NOT NULL DEFAULT 'pending',        -- pending | done | retained (legal) | failed
  evidence   jsonb,                                    -- counts, object versions removed, retained-because reason; no PII
  done_at    timestamptz,
  PRIMARY KEY (request_id, system)
);
```

## 9. Deep dives

### 9.1 Four end states, not one `is_deleted` flag

| State | Column | Default lists | Search | Restorable | Reports | Typical trigger |
|---|---|---|---|---|---|---|
| Active | none set | yes | yes | n/a | counted | normal |
| **Archived** | `archived_at` | hidden (an *Archived* filter shows them) | yes, marked | yes, instantly (Undo toast) | excluded from "open" counts, kept in history | "I'm done with this" |
| **Removed** (soft delete) | `deleted_at` | hidden everywhere but the Trash | no | yes, until `purge_after` | excluded | "I don't want this" |
| **Purged** (hard delete) | row gone | no | no | no | gone, children cascade or `SET NULL` | 30 days after removal, or "Delete now" |
| **Anonymised** (tombstone) | `erased_at` + nulled PII | yes, as "Erased" | no | no | counted (money stays correct) | erasure of someone whose rows must stay |

| Modelling option | Pros | Cons |
|---|---|---|
| One boolean `is_deleted` | trivial | cannot say archive vs remove, nor *when* (no purge clock), nor *who* |
| **Two timestamps + `_by`** (chosen) | orthogonal to the record's workflow status (see [26](26-status-workflow.md)); `deleted_at` is the purge clock; `_by` powers the audit line | every query must repeat two predicates |
| A `lifecycle` enum column | one predicate | cannot express "archived, then removed, restore goes back to archived" |
| Move rows to `*_deleted` tables | live tables stay small and clean | restore is a copy; foreign keys and unique checks across two tables; schema drift |

**Choice and why.** Two nullable timestamps. They are orthogonal to the status workflow, the
timestamp is the purge clock, and a partial index on the *live* rows keeps default lists as fast
as if dead rows did not exist. The cost is discipline: **every read says both predicates** (put
them in one repository helper; a missing predicate is a data leak, not a slow query), and
uniqueness becomes partial: `users_email_live_key` frees a removed user's email, so **Restore must
refuse when someone has taken it** and say who. The seed project does exactly this. Bulk actions follow the same shape: each row is its own decision ("you cannot remove yourself", "the last admin", "someone is editing it", see [21](21-presence-soft-locks.md)), applied in turns of 200 and answered with `{done, refused[{id, reason}]}`.

### 9.2 Retention jobs that don't hurt the primary

At 10M archive-eligible rows a month, how you delete matters more than when.

| Option | Pros | Cons |
|---|---|---|
| One big `DELETE … WHERE deleted_at < now() - '30 days'` | one line | long transaction, row locks, WAL burst, replica lag, 10M dead tuples at once, can run out of disk |
| **Batched delete**: 5,000 rows per statement, `SKIP LOCKED`, sleep between, resumable | bounded locks and WAL; stoppable; two workers cooperate | still creates dead tuples; needs vacuum tuning |
| **Partition drop**: monthly partitions, `DROP TABLE` the old one | instant, no bloat, no vacuum | only works when lifetime is a function of the partition key (logs, events, audit): not for the Trash, whose rows are removed at arbitrary times |
| Archive to cold storage (Parquet in object storage), then delete | cheap long-term storage, still queryable offline | a second system; restore from cold is a project |

**Choice and why.** Partition append-only data by month and drop (audit log, events, outbox
history). Use batched deletes for the Trash. Export old archived rows to cold storage before
deleting them.

```sql
WITH doomed AS (
  SELECT id FROM orders o
  WHERE o.deleted_at < now() - interval '30 days'
    AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.order_id = o.id)   -- what still names it is left out by the WHERE
  ORDER BY o.deleted_at
  LIMIT 5000
  FOR UPDATE SKIP LOCKED
)
DELETE FROM orders USING doomed WHERE orders.id = doomed.id RETURNING orders.id;
```

Run it in a loop with a pause (20 ms–1 s, proportional to replica lag), per kind, in the same
transaction as the audit row. 10M rows is 2,000 batches; at roughly 0.3 s of work plus 0.3 s of
pause per batch that is ~20 minutes, off-peak, stoppable at any batch boundary.
**Vacuum:** deletes leave dead tuples. The default autovacuum trigger is 20 % of the table (100M
rows at 500M); set `autovacuum_vacuum_scale_factor = 0.01` (5M) on the big tables, watch
`n_dead_tup`, and `REINDEX CONCURRENTLY` / `pg_repack` indexes that bloat anyway.

### 9.3 Erasure across systems, and proving it

A record is copied more places than the database. List them *before* writing code:

| Store | Does erasure reach it by itself? | What to do |
|---|---|---|
| Postgres primary, replicas | replicas follow the primary | delete or anonymise; one transaction |
| Search index | no | outbox event → delete the document by id |
| Cache (Redis), CDN | no; TTL eventually | delete keys by entity id; purge URLs |
| Object storage | no; **versioning keeps old versions** | delete every version of every key; check lifecycle rules |
| Logs, traces | no | keep PII out of logs; otherwise rely on a short retention (30 days) and say so |
| Analytics warehouse | no | store a pseudonymous id; erasure deletes the id↔person mapping, or delete rows |
| Email provider | no | keep a **hashed suppression entry** so an unsubscribe stays honoured |
| Third-party processors (invoicing, CRM) | no | call their delete API or record that the controller was told |
| **Backups** | no, and you cannot edit them | let them expire (35 days) and keep a ledger replayed after any restore |

| Approach | Pros | Cons |
|---|---|---|
| A manual runbook | no code | misses a store; no deadline tracking; not provable |
| **Orchestrator: ledger + one idempotent handler per store via the outbox** | retries, per-system status, evidence, deadline | handlers to maintain; new stores must register |
| Crypto-shredding: per-person data key, delete the key | erases backups and logs at once | key management at 100M subjects (a 32-byte key each ≈ 3.2 GB), no querying encrypted fields, complex |

**Choice and why.** The orchestrator now. It is a table, an outbox and eight small handlers, and
it turns "we think we erased it" into rows. **Proving it:** each step stores evidence without PII
(`{"objects_deleted": 14, "versions": 31}`); a **verification job** then searches every store for
the subject's identifiers and records zero hits; the request is `done` only when all steps are
`done` or `retained` (with the legal reason). Crypto-shredding is an extension for the stores you
cannot edit. Not legal advice; the legal windows are roughly a month (GDPR) and 45 days (CCPA).

### 9.4 When the parent goes: children, "who did it", and rows that must stay

| Foreign key / pattern | Behaviour | Use for |
|---|---|---|
| `ON DELETE CASCADE` | children vanish with the parent | **owned** children only (an order's steps) |
| `ON DELETE SET NULL` | the child stays, the link is lost | "who did it": `deleted_by`, `created_by` (audit lines read "a former user") |
| `ON DELETE RESTRICT` | the parent cannot be purged | money: orders with invoices, clients with orders |
| No FK, polymorphic subject (`entity_type`, `entity_id`) | the child stays, dangling on purpose | message and audit logs (a log should outlive its subject) |
| **Tombstone (anonymise the parent, keep the row)** | the id and the money stay, the person goes | erasing someone whose rows the law makes you keep |

**Choice and why.** Use the strongest constraint the business needs, row by row: `RESTRICT` behind
money, `SET NULL` for authorship, `CASCADE` only for owned children. An erasure request for a
person with paid invoices does **not** delete the invoices: it anonymises the client (`erased_at`,
name and contact fields replaced), keeps the invoice lines, and reports `retained: tax records, N
years`. Retention for a legal obligation is an accepted reason to keep data; being silent about
it is not.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the purge. 100M rows a month is 20,000 batches a month: partition
  by time where possible, shard the purge by tenant or hash, and move cold data out. Dead tuples
  and index bloat show up before CPU.
- **A query forgets a predicate:** removed rows leak into a list or a count. Mitigate with one
  repository helper, a lint/test that fails on queries against removable tables without it, and a
  Postgres view per table (`orders_live`) for reports.
- **Handler failure in erasure:** the step stays `pending` with attempts and backoff; an alert fires
  when a request is within 7 days of its deadline and not `done`.
- **Backup restore after an incident:** erased people reappear. The restore runbook replays the
  `erasure_requests` ledger (subject hashes) before the app goes live.
- **Single points of failure:** the ledger and the orchestrator (replicate; they are tiny). **Re-import of an erased person:** match against the hashed suppression list and refuse or flag.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Two timestamps (archive, remove) | one `is_deleted` flag | meaning, a purge clock, who-did-it | two predicates in every query |
| Partial indexes on live rows | full indexes | default lists unaffected by dead rows | queries must repeat the predicate to use them |
| Batched purge + partition drop | one big DELETE | bounded locks, WAL and lag | a loop, tuning, longer wall-clock |
| Orchestrated erasure with a ledger | a manual runbook | completeness, deadlines, evidence | handlers per store |
| Anonymise rows that must stay | delete them | accounting stays correct | the row exists (without the person) |

## 12. Check yourself

1. Give three differences between Archive, Remove and Erase.
2. Why must a default list query repeat `archived_at IS NULL AND deleted_at IS NULL`, and what happens if it doesn't?
3. Why not run one `DELETE` for 10M expired rows? What do you run instead, and what autovacuum setting matters?
4. A user asks to be erased. Name six places their data may live, and which one you cannot edit.
5. A client with three paid invoices asks to be erased. What happens to the invoices and to the client row?

<details><summary>Answers</summary>

1. *Archive*: hidden but intact, searchable, instant undo, no clock. *Remove*: hidden everywhere
   except the Trash, restorable for 30 days, then purged. *Erase*: a privacy request with a legal
   deadline; data leaves every store (or is anonymised), is not restorable, and needs evidence.
2. A partial index is used only when the query's `WHERE` implies the index predicate. Without both
   conditions the planner falls back to a bigger index or a scan, *and* the query returns dead rows,
   which is a leak (a removed record showing in a list, a count, an export), not just slowness.
3. One statement means a long transaction, a WAL burst, replica lag and 10M dead tuples at once.
   Run batches of ~5,000 with `SKIP LOCKED`, a pause between them, resumable. Autovacuum's default
   `scale_factor` of 0.2 waits for 20 % of 500M = 100M dead rows; lower it (0.01) on big tables.
4. Postgres (and replicas), search index, cache, CDN, object storage (all versions), logs,
   analytics, email provider, third-party processors, backups. You cannot edit backups: let them
   expire and replay an erasure ledger after any restore.
5. The invoices stay (a legal retention reason; `RESTRICT` protects them in the database). The
   client row is anonymised: `erased_at` set, name, email, phone, address replaced or nulled, id and
   totals kept. The erasure step records `retained: tax records` with the reason. It is not deleted.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md), [59 Production DB patterns](../../59-production-database-patterns.md) (`SKIP LOCKED`, partial indexes, safe migrations), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (the erasure fan-out), [44 Background jobs](../../44-background-jobs-queues.md) (the purge worker), [43 Authorization](../../43-authorization-rbac-multitenancy.md) (`records.remove` as a permission), [45 Polymorphic relations](../../45-polymorphic-relations.md) (the dangling subject).
- A purge loop that stops cleanly and backs off:

```go
func purge(ctx context.Context, db *pgxpool.Pool, sql string, batch int, pause time.Duration) (int64, error) {
	var total int64
	for {
		tag, err := db.Exec(ctx, sql, batch) // the batched DELETE above, LIMIT $1
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < int64(batch) {
			return total, nil // last, partial batch
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err() // resume tomorrow, nothing half-done
		case <-time.After(pause): // raise when replica lag grows
		}
	}
}
```

- Packages: `pgx`, `hibiken/asynq` or the outbox relay for erasure tasks, `crypto/hmac` for subject keys, `aws-sdk-go-v2/service/s3` (`ListObjectVersions` before you claim an object is gone).

## 14. Extensions

- **Crypto-shredding** for backups and the event log: a data key per person, deleted on erasure.
- **Legal hold:** a flag that blocks purge and erasure for records under investigation.
- **Tenant offboarding:** erase a whole tenant (millions of rows) with the same orchestrator, plus an export first.
- **Data inventory:** a registry of every store and column holding personal data, checked in CI so a new table cannot skip the erasure pipeline (compare the backup test in the seed project that fails if a table is in no group).
