# 22 · Audit Log & Revision History

`🟢 easy` · *Topics:* snapshot vs diff vs event sourcing, append-only partitioned tables, per-entity and per-actor queries, same-transaction vs async audit, restore semantics, redaction, tamper evidence · *Builds on:* [10 Webhook Delivery](10-webhook-delivery.md) (outbox), [20 Per-Tenant Document Numbering](20-document-numbering.md) (serialising a per-tenant sequence has a cost)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [21 Presence & Soft Edit Locks](21-presence-soft-locks.md) · Next: [23 Paginated Lists at a Billion Rows](23-paginated-lists.md)

---

> 🌱 **Seed (honeydolist-site):** migration 0013 (docs/db/tables.md §14) adds two polymorphic tables. `content_revisions`
> holds a full `jsonb` snapshot of a content row after every edit, with **no foreign key** because it "must survive
> the row being deleted, which is exactly when you need it most", so a bad edit is one click to undo. `audit_log`
> records who did what (`actor_id`, `action`, `entity_type`, `entity_id`, `before`, `after`, `ip_hash`), indexed by
> entity and by actor. Each audit row queues an `activity` outbox job in the same statement, and a test refuses code
> that writes the table around that hook. Step 51 shows exact business-zone time on every history and makes a
> user's activity line name the record, link to it and fold out *what changed* (field old → new, never password, hash,
> token, secret, key or code fields). ADR 0009 keeps secrets out of `settings` because saving settings snapshots into
> revisions and dumps travel off-site. Built for a few staff.
> **Here, at scale:** 20k audited writes/s at peak (~6k/s average, ~518M rows/day), two years online, seven years
> retained, the history of one record in milliseconds.

## 1. The task

> "Record every change anyone makes in our product: who, what, when, and what it looked like before and after.
> Show the history of any record, let admins see everything a user did, and let them roll back a bad edit."

Three different needs hide in that sentence: **accountability** (who did what), **history** (what did it
look like), and **recovery** (put it back). They want different storage shapes.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Compliance audit or product feature? | Compliance means "no change without a log line" and tamper evidence. | Both: guaranteed capture, and a visible history tab. |
| Restore which things? | Full snapshots are expensive; not everything needs undo. | Content (pages, templates, non-secret settings). Orders and invoices are not rolled back, they are corrected. |
| Retention? | Drives partitioning and cold storage. | 2 years online and searchable, 7 years archived. |
| Queries? | Decides indexes. | History of one entity (very common), everything one actor did (rare, time-bounded), exports. |
| May an entry contain PII or secrets? | Immutable log vs erasure rights. | Never secrets. PII only where needed, erasable. |
| Can entries be edited or deleted? | Defines "audit". | Never, except dropping whole partitions at retention end. |
| Must the log survive the entity's deletion? | The seed's reason for no foreign key. | Yes. |
| Scale? | Chooses storage and shards. | 20k/s peak, ~6k/s average, 32 shards by tenant. |

## 3. Requirements

**Functional**
- Every create, update, delete and sensitive read/login produces exactly one audit entry.
- History of an entity, newest first, with actor, time (UTC, shown in the business zone) and changed fields.
- Everything an actor did in a time range.
- Content revisions with one-click restore; restoring is itself a recorded change.
- Export a time range to a file.

**Non-functional**
- No business change commits without its audit entry (capture is atomic with the change).
- Entity history p99 under 50 ms at any age online; the audit insert adds under 1 ms to a write.
- Append-only; tamper-evident; secrets never stored.
- Retention enforced by dropping data in bulk, never row by row.

**Out of scope**
- A full SIEM, anomaly detection, read-access auditing at the row level.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Writes | 6k/s average, 20k/s peak | assumed; ~3.3× peak |
| Rows per day | ~518M | 6,000 × 86,400 |
| Per shard (32 shards) | ~190/s average, 625/s peak | 6,000 / 32 and 20,000 / 32 |
| Row size, diff-based | ~350 B | ~250 B changed fields and labels + ~100 B tuple overhead |
| Per day / year | ~181 GB / ~66 TB | 518.4M × 350 B; × 365 |
| Index overhead | ~+23% | two indexes at ~40 B each = 80 B / 350 B |
| Online, 2 years | ~162 TB, ~5 TB per shard | (66 × 2) + indexes ≈ 132 + 30 TB; / 32 |
| Archive, years 3–7 | ~55 TB | 5 × 66 TB = 330 TB raw, ~6× compression on JSON in Parquet + zstd (assumed) |
| If we stored full before+after (~4 KB) | ~2 TB/day | 518.4M × 4 KB: **11× bigger** |
| Reads | ~500/s history, a handful of exports | history tab opens; rare admin queries |

What the numbers tell us: capture is a **write-heavy append** with a tiny read side. Size is the enemy:
the choice between diffs and snapshots is an 11× difference in disk, and retention must be a partition drop, since
deleting 518M rows a day would cost more than writing them.

## 5. Plan

1. Decide what an entry contains (diff, snapshot, event) and what restore needs.
2. Table design: append-only, partitioned, with the two indexes that serve the real queries.
3. Capture path: same transaction, async, or CDC.
4. Restore semantics, including deleted records.
5. Redaction, erasure and tamper evidence.

## 6. High-level design

```
 any service write path                                  shard-local Postgres
   BEGIN                                                 ┌────────────────────────────────────┐
   UPDATE orders …                                       │ audit_log          PARTITION BY month│
   audit.Record(tx, actor, action, entity, before, after)│ content_revisions  (content only)    │
   INSERT outbox('audit.created', id)                    │ outbox                               │
   COMMIT                                                └───────────────┬────────────────────┘
                                                                         │ shipper (batches, ordered by id)
                         ┌───────────────────────────────┬───────────────┴────────────┐
                         ▼                               ▼                            ▼
                  seal job (hash chain)        search index (optional)       cold archive: Parquet + zstd
                  per tenant, async                                           on object storage, Object Lock
 reads: history tab / actor view ──► API ──► the shard's audit_log (keyset pagination, task 23)
```

*A write:* the business statement and one audit insert commit together or not at all. *A read:* an entity's history is
one index range scan per monthly partition (24 for two years), newest first. *Retention:* a monthly cron detaches
the oldest partition, exports it to object storage, then drops it.

## 7. API

```
GET  /v1/audit?entity_type=order&entity_id=o_7Kq2&limit=50&cursor=…   → 200 { items: [...], next_cursor }
GET  /v1/audit?actor=u_9&from=2026-11-01&to=2026-11-08                → 200  (time range required, max 31 days)
GET  /v1/orders/{id}/revisions                                         → 200 [{ n, at, by, note }]
GET  /v1/pages/{id}/revisions/{n}                                      → 200 snapshot
POST /v1/pages/{id}/revisions/{n}/restore    If-Match: "17"            → 200 new revision | 412
POST /v1/audit/exports { "from": …, "to": …, "filters": … }            → 202 { job } → file in object storage
```

An item: `{ at, actor: {kind, id, name}, action, entity: {type, id, label}, changes: [{field, from, to}] }`.
`entity.label` is the title **stored at write time**, so a removed record still has a name and no link.

## 8. Data model

```sql
CREATE TABLE audit_log (
    id           bigint GENERATED ALWAYS AS IDENTITY,
    tenant_id    bigint      NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),       -- UTC; shown in the business zone by the UI
    actor_kind   text        NOT NULL,                     -- user | api_key | system
    actor_id     bigint,                                   -- NULL for system
    action       text        NOT NULL,                     -- order.update, user.login, settings.limits_changed
    entity_type  text        NOT NULL,
    entity_id    bigint      NOT NULL,
    entity_label text,                                     -- title at write time, survives deletion
    changes      jsonb,                                    -- {"price":[10,12]}: changed fields only, redacted
    request_id   text,                                     -- correlates with logs and traces
    ip_hash      text,
    PRIMARY KEY (tenant_id, created_at, id)                -- must include the partition key
) PARTITION BY RANGE (created_at);

CREATE TABLE audit_log_2026_11 PARTITION OF audit_log
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE INDEX audit_entity_idx ON audit_log (tenant_id, entity_type, entity_id, created_at DESC);
CREATE INDEX audit_actor_idx  ON audit_log (tenant_id, actor_id, created_at DESC) WHERE actor_id IS NOT NULL;

CREATE TABLE content_revisions (                           -- snapshots only for content that can be restored
    tenant_id    bigint      NOT NULL,
    entity_type  text        NOT NULL,
    entity_id    bigint      NOT NULL,                     -- no foreign key: must outlive the row
    revision     integer     NOT NULL,
    schema_ver   integer     NOT NULL,                     -- snapshot shape, for restores after migrations
    data         jsonb       NOT NULL,
    created_by   bigint,
    created_at   timestamptz NOT NULL DEFAULT now(),
    note         text,
    PRIMARY KEY (tenant_id, entity_type, entity_id, revision)
);
```

## 9. Deep dives

### 9.1 What an entry stores: snapshot, diff, or event

| Option | Pros | Cons |
|---|---|---|
| **Full snapshot** after each change (the seed's `content_revisions`) | restore is a copy; history of any version is one row | the whole row each time: ~2 KB × 518M/day ≈ 1 TB/day |
| **Before + after** (the seed's `audit_log`) | complete, easy to render a diff | the same size problem, doubled: ~4 KB × 518M/day ≈ 2 TB/day |
| **Diff** (changed fields only) | ~250 B; exactly what the "what changed" UI shows | restore needs replaying diffs or a snapshot |
| **Event sourcing** (events are the truth, state is derived) | audit for free, perfect history | a different architecture for the whole product ([lesson 37](../../37-cqrs-event-sourcing.md)); not an add-on |

**Choice and why.** **Diffs in the audit log** for the "who, what, when" tables that grow at 518M rows a day, and
**snapshots in a separate, much smaller table** only for entity types that have a restore button. Thin out old
snapshots: keep all for 30 days, then one a day, then one a week. Store `entity_label` at write time: the seed
resolves labels with one query across ~20 entity tables per page of activity, which is an N-way join across
shards at scale. Denormalising the label at write time makes the history a single-table read.

### 9.2 The table: append-only, monthly partitions, two indexes

The workload is "insert at the tail, read by entity, read by actor in a range, drop old data". So:

| Choice | Why |
|---|---|
| Partition by **month** | 24 partitions online: an entity-history query seeks into each (24 index lookups, ~0.1 ms each); daily partitions would be 730 seeks. Retention is `DETACH` + `DROP`, instant, no vacuum |
| Primary key `(tenant_id, created_at, id)` | a partitioned table's key must include `created_at`; inserts land at the end of each tenant's range |
| `audit_entity_idx (tenant, type, id, created_at DESC)` | the history tab: exactly the query, already ordered |
| `audit_actor_idx … WHERE actor_id IS NOT NULL` | "everything Mia did", only usable **with a time range**, so the API requires one |
| No other indexes, no updates | every extra index is 23% more write cost on 518M rows/day; no `UPDATE` means no dead tuples |
| Optional BRIN on `created_at` | tiny index for "all events between T1 and T2" exports |

Reading history is keyset pagination (`WHERE (created_at, id) < ($1, $2) ORDER BY created_at DESC, id DESC
LIMIT 50`, see [23](23-paginated-lists.md)), never offset. Return timestamps in UTC; the UI shows the business
zone (the seed uses one US zone for every history screen, with seconds on hover).

### 9.3 Same transaction, async, or CDC

| Option | Pros | Cons |
|---|---|---|
| **Insert the audit row in the business transaction** | atomic: no change without a log line, rollback removes both; trivially correct | +1 insert in the hot path (~0.2 ms); audit load lands on the primary |
| Async: publish after commit | no hot-path cost | **loses entries on a crash between commit and publish**: unacceptable for compliance |
| Async via **outbox** (task 10) | atomic capture, delivery later | two writes per change; still needs a consumer |
| **CDC** from the WAL (logical decoding) | catches everything, even manual SQL; no app code | no actor or request context unless each transaction sets `SET LOCAL app.actor = …` and a trigger copies it; schema coupling |

**Choice and why.** The seed's way: **write the audit row in the same transaction**, through one helper that is the
only way in, with a test or lint that fails the build for code writing `audit_log` around it. Capture is the
guarantee, so it must be atomic. The *downstream* work (search index, cold archive, hash sealing, alerts) goes on
an outbox row written in that same transaction, because those can lag. If audit inserts ever hurt p99, a
trigger plus CDC for bulk jobs is the escape hatch, but only with the actor passed as a session variable.

### 9.4 Restore and rollback

A restore is **a new edit**, never a rewrite of history: `POST …/restore` loads revision *n*, writes it as the
current content with `If-Match` on the current version (`412` if someone edited meanwhile), creates revision *n+1*
and an audit entry `page.restored {from: n}`. Rules worth stating:

- **A deleted record** is restorable because revisions have no foreign key and keep the id: restore re-creates it
  with the same public id, or a new one if the old id is taken.
- **Snapshots carry `schema_ver`**, so a restore after a migration upcasts the old shape instead of failing.
- **Restore content, not consequences.** Reverting a page does not revert emails sent or invoices issued because of
  it. Business records (orders, invoices) are never rolled back, they are corrected by a new record; the audit log
  explains why.
- **Field-level undo** replays one diff in reverse (only if the field still has the `after` value).

### 9.5 Redaction, erasure and tamper evidence

**Secrets.** The seed's deny-by-name rule (never show fields named password, hash, token, secret, key, code) is a
good last line, but at scale use an **allow-list per entity type** of auditable fields, and deny-list as a safety
net. ADR 0009's lesson goes further: if a value is a secret, it must not live in a table whose saves are
snapshotted or audited at all; the audit records *which keys* changed, never values. Redact at the single capture
helper, so no caller can forget.

```go
func Diff(auditable map[string]bool, before, after map[string]any) map[string][2]any {
	out := map[string][2]any{}
	for field := range auditable { // allow-list: unknown fields are never logged
		if !reflect.DeepEqual(before[field], after[field]) {
			out[field] = [2]any{before[field], after[field]}
		}
	}
	return out
}
```

**Erasure vs immutability.** Keep names and emails out of `changes` where an id will do; where PII must appear,
encrypt the payload with a per-subject key stored elsewhere ("crypto-shredding": deleting the key erases the PII
and leaves the immutable row intact).

**Tamper evidence.** Chain hashes per tenant: `row_hash = sha256(prev_hash ‖ canonical(row))`, so changing any old
row breaks every hash after it. Computing it inline would serialise every write of a tenant (task 20's counter-row
cost). Instead a **sealing job** reads new rows in id order per tenant and writes hashes in batches, and
periodically stores the chain head somewhere write-once (object storage with Object Lock). The chain is the same
idea as the [hash chain project](../../blockchain-projects/01-hashchain-beginner/README.md), applied to a log.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the index write cost and the primary's WAL volume. Drop the actor index to a
  separate async-built store (search or columnar), keep only the entity index in Postgres, add shards.
- **A hot tenant:** one tenant producing 20% of the audit volume overloads its shard; give it a dedicated shard
  (the tenant-to-shard map allows moves) and rate limit audited bulk operations ([03](03-rate-limiter.md)).
- **Bulk operations:** "update 1M rows" must write 1M entries or one entry with `scope: bulk` and a job id. Decide
  per action, and never skip silently.
- **Partition management:** a missing next-month partition makes inserts fail, i.e. business writes fail. Create
  partitions two months ahead and alert if fewer than one remain.
- **Audit store down or full:** since capture is in the business transaction, writes fail with it. That is the correct
  compliance behaviour; separate disks and capacity alerts keep it from happening.
- **Shipper lag:** the outbox grows; alert on the oldest unshipped row. The archive is behind, nothing is lost.
- **Restore from backup:** after restoring a shard, sealing hashes beyond the restored point must be recomputed.
- **Long-running transactions:** the audit row has `created_at = now()` at statement time, so rows can commit out of
  `created_at` order. Page by `(created_at, id)` knowing a late commit can appear slightly "in the past".

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Diffs in `audit_log`, snapshots only for content | snapshots everywhere | 11× less storage, matches the UI | restore needs the snapshot table |
| Audit row in the business transaction | async publish | no change without a log line | ~0.2 ms and primary load on every write |
| Monthly partitions, drop at retention | one table with `DELETE` | instant retention, 24 seeks per history | partition management |
| `entity_label` stored at write time | resolving titles by join | single-table history reads | labels can be stale after a rename (that is the point) |
| Allow-list of auditable fields | deny-list only | secrets cannot leak by omission | maintaining the list per entity |
| Async sealing job for the hash chain | inline chain | no per-tenant write serialisation | chain lags by seconds |
| Restore creates a new revision | rewinding history | history stays append-only and honest | restore does not undo side effects |

## 12. Check yourself

1. Why store diffs in the audit log but full snapshots in a separate revisions table?
2. Why must the audit row be written inside the business transaction, and what is the cost?
3. Why monthly partitions, and how does retention work?
4. How do you restore a record that was deleted, and why does `content_revisions` have no foreign key to it?
5. How do you make the log tamper-evident without serialising every write, and how do you keep secrets and PII out?

<details><summary>Answers</summary>

1. Audit entries grow at ~518M a day and need only "who changed what": ~350 B as diffs against ~4 KB
   with before + after, 11× less disk (and ~6× less than snapshots alone). A restore needs the full content, but only a few entity types (pages, templates)
   have one, so snapshots live in a small, thinned-out table.
2. If capture is separate (publish after commit), a crash in between loses the entry, which a compliance log
   cannot allow. Inside the transaction both commit or neither does. The cost is one extra insert (~0.2 ms) and
   audit write load on the primary; downstream consumers use an outbox row written in the same transaction.
3. An entity's history needs one index seek per partition: 24 monthly partitions is 24 seeks, daily would be ~730.
   Retention detaches and drops the oldest partition (after exporting it), which is instant and creates no dead tuples,
   unlike deleting hundreds of millions of rows.
4. Read the last revision (or the deleted-row audit entry), write it back as a new row with the same public id, and record
   a restore entry. No foreign key, so the revision survives the delete, which is exactly when it is needed.
5. A sealing job reads new rows in id order per tenant, computes `sha256(prev ‖ row)` in batches and anchors the
   head in write-once storage, so writers never wait on a chain. Redact at one capture helper using an allow-list
   of auditable fields, never store secrets in audited tables, and encrypt unavoidable PII with a per-subject key you can delete.

</details>

## 13. In Go

- Related lessons: [34 Event-driven & outbox](../../34-event-driven-outbox.md) (the downstream hook), [37 CQRS & event sourcing](../../37-cqrs-event-sourcing.md) (when events are the truth), [59 Production DB patterns](../../59-production-database-patterns.md) (partitions, transactions), [22 database/sql](../../22-database.md), [55 Data pipelines](../../55-data-pipelines.md) (the shipper and archive job), [57 Web security](../../57-web-security.md) (secrets handling), [39 Observability & tracing](../../39-observability-tracing.md) (`request_id`).
- Packages: `pgx` (`pgx.Tx` passed into `audit.Record`), `encoding/json`, `crypto/sha256`, `reflect` (or a typed diff generated per entity), `github.com/parquet-go/parquet-go` for the archive.
- Design the helper so a transaction is required: `func Record(ctx context.Context, tx pgx.Tx, e Entry) error` makes "audit outside a transaction" a compile-time impossibility.

## 14. Extensions

- **Legal hold:** a tenant flagged for investigation keeps its partitions past retention; how does the drop job respect it?
- **Customer-facing audit export** and streaming to a customer's SIEM via webhooks ([task 10](10-webhook-delivery.md)).
- **Search** over audit entries (OpenSearch fed by the shipper) for free-text and facet queries.
- **Read auditing:** log who opened sensitive records, sampled, with its own volume math.
- **Anchoring** the hash chain's head to a public ledger and verifying it independently.
- **Alerts on actions** ("a Superadmin changed rate limits"), driven by the same outbox rows ([task 50](50-alerting-engine.md)).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Audit shards (Postgres: `audit_log`, `content_revisions`, `outbox`; the business shards) | 20k inserts/s at peak = ≈ 625 per shard (≈ 190 average); history reads 500/s ≈ 16 per shard; 162 TB online ≈ 5 TB per shard | disk size, then memory | CPU 625 × 0.5 ms = 0.3 of 8 vCPU; IOPS ≈ 625 × 3 (two random index pages + the heap tail, assumed) = 1,900 + history reads 16/s × 24 partitions ≈ 400 ⇒ 2,300, so gp3 at 6,000 (shared with the business writes); insert-hot set = the current month's two indexes ≈ 39 GB at month end (172 GB heap + 23 % per shard-month), fits in 64 GiB | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ | 32 shards × (primary + standby) | `r`, not `m`: 32 GiB cannot cache a 20–40 GB insert-hot index; CPU is idle; below 16k IOPS gp3 is enough (`i4i` NVMe not needed). Monthly partitions keep the insert-hot set to one month of indexes instead of 24. By write rate alone ~7 nodes would do (20k ÷ ~3k/s); 32 is the lesson's tenant sharding |
| Shipper, hash sealer, archiver (Go) | ≈ 20k rows/s at peak read from 32 outboxes (≈ 7 MB/s) | CPU | assume 0.1 ms per row (batch read, canonical JSON, sha256, Parquet encode): 20k × 0.1 ms = 2 vCPU. N = ceil(2 ÷ (0.8 × 4 × ⅔ = 2.13)) = 1 → 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | CPU plus a few hundred MB for Parquet row groups, so `c`; sha256 over ≈ 400 B × 20k = 8 MB/s is trivial; seconds of lag are acceptable, so Spot would also do |
| History / actor API (Go, read-only) | ≈ 500 reads/s (section 4), a handful of exports | CPU, barely | 500 × 1 ms = 0.5 vCPU. N = ceil(0.5 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) | 3, one per zone | availability, not load; a read is 24 index seeks on one shard (≈ 2 ms cached, ≈ 25 ms if cold at ~1 ms each, assumed), inside the 50 ms p99, so the wait is disk latency, not CPU |

Cold archive: Parquet + zstd on S3 with Object Lock, ≈ 55 TB once years 3–7 are filled; nothing to size. Hash-chain heads are a few KB. The optional search index is not sized here.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Shards: 32 × `db.r7g.2xlarge`, Multi-AZ (32 × $1,240) | $39,700 |
| gp3: 162 TB × $0.08 = $12,960, + 3,000 extra IOPS × 32 volumes × $0.005 = $480, all ×2 for the standbys | $26,900 |
| Shipper / sealer / archiver: 3 × `c7g.xlarge` (3 × $105) | $320 |
| History API: 3 × `c7g.large` (3 × $53) | $160 |
| ALB | $30 |
| S3 archive: 55 TB × $0.023 (once full) | $1,270 |
| **Total** | **≈ $68,400** (≈ $54,300 with 1-year commitments on the $40,180 of compute) |

- **Biggest line and why:** the Multi-AZ shards, 97 % (nodes 58 %, disks 39 %). What forces it is "history at any age online in 50 ms": 162 TB on gp3 with a standby costs $0.16 per GB-month against $0.023 on S3 (7×). Diffs are what keep it at $27k of disk: before + after (11× the bytes) would be ≈ $300k. Keeping 90 days in Postgres and serving older history from Parquet is a requirement change, not a sizing one.
- **What changes at 10×:** 200k inserts/s ⇒ 6,250 per shard ≈ 19k IOPS, past gp3's 16k cap (`io2`, NVMe `i4i`, or ~3× the shards; moving the actor index to a columnar store halves the index IOPS); 1.6 PB online ≈ $260k a month on gp3 with standbys, so only ~90 days stay on Postgres; workers need 20 vCPU ⇒ N = ceil(20 ÷ 2.13) = 10 → 12 × `c7g.xlarge`.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Append-only and large: 6k rows/s average, 20k/s peak, ≈ 518M rows a day of ≈ 350 B diffs ≈ 181 GB a day ≈ 66 TB a year; 2 years online ≈ 162 TB with indexes, ≈ 5 TB on each of 32 shards (section 4).
- No updates and no row deletes: retention drops whole months. The read side is tiny (≈ 500/s): one entity's history newest first, one actor's rows in a time range, and exports.
- The audit row must commit in the same transaction as the business change, which lives in the same tenant shard (9.3).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora), sharded by tenant | ✅ chosen | monthly `PARTITION BY RANGE (created_at)` makes retention a `DETACH` + `DROP` with no vacuum; `(tenant_id, entity_type, entity_id, created_at DESC)` serves the history tab as one range scan per partition; the partial index `WHERE actor_id IS NOT NULL` skips system rows; `jsonb` holds the diff; and the insert rides in the business transaction (≈ 0.2 ms) |
| MySQL (RDS / Aurora), sharded by hand or with Vitess | ✔ works, with care | `RANGE` partitioning with `DROP PARTITION` (or `EXCHANGE PARTITION` to hand a month to the exporter) gives the same retention, and the rule that every unique key includes the partition column is the same. The differences are the clustered key, no partial index and no BRIN (below) |
| DynamoDB | ✔ as a history store fed by the outbox, ✖ for same-transaction capture | the shape fits: partition key `(tenant, entity_type, entity_id)`, sort key `created_at#id`, so the history tab is one `Query` (newest first, `LastEvaluatedKey` as the cursor) and the actor view a GSI on `(tenant, actor, day)`; ≈ 350 B rounds up to one 1 KB write unit, doubled by the GSI. It cannot commit with a business row that sits in Postgres, so capture becomes outbox + async, which 9.3 rejected for compliance; TTL expiry is free but lazy (days); a per-tenant hash chain needs an order a partition per entity does not give |
| Cassandra / Keyspaces | ✔ as a history store, same catch | the same model (partition `(tenant, entity_type, entity_id)`, clustering `created_at DESC`); time-window compaction with a TTL drops whole files by age, like a partition drop; a self-run cluster typically keeps 3 replicas (162 TB online ≈ 486 TB raw) against the 2 copies of an RDS Multi-AZ pair; Keyspaces has no compaction to tune |
| OpenSearch | ✔ as a derived index only | free-text and faceted search over entries (section 14), fed by the shipper and rebuildable from the log; never the record |
| S3 (Parquet + zstd, Object Lock) | ✅ chosen for years 3–7 | ≈ 55 TB of archive, queried offline (Athena), write-once anchors for the hash-chain heads |

**Pick:** PostgreSQL on RDS Multi-AZ, 32 shards of `db.r7g.2xlarge` (section 15), the same shards as the business data; S3 for the archive; OpenSearch optional and derived. Three things decide it: the audit row commits atomically with the change in the same shard (9.3), native monthly partitions with drop-based retention (9.2), and the tenant-to-shard layout and its operations already exist for the business data.

**Sharded Postgres, or a key-addressed store?** This is the append-only, key-addressed shape DynamoDB and Cassandra were built for, and 20k writes/s is routine for either, so the case for Postgres is not throughput: by write rate ≈ 7 nodes would do (section 15). It is atomic capture with a change that lives in a Postgres shard, plus shards, a map and on-call that already exist. The price is real: the Multi-AZ shards are 97 % of the ≈ $68k bill (nodes 58 %, disks 39 %, section 15), and the sheet has no DynamoDB or Cassandra prices, so price both before you commit. Under Vitess a MySQL fleet would sit in the same place: a `tenant_id` vindex and online resharding replace the hand-made map, at the cost of running Vitess yourself or buying it.

**If it were MySQL instead:**
- The layout ports, the primary key does not. `(tenant_id, created_at, id)` as InnoDB's clustered key puts every row mid-tree, a random page touch per insert across the shard's tenants. Use the append-only `(created_at, id)` as the key (it still contains the partition column) and keep `tenant_id` in the entity index: the table's tail is then sequential, and the two secondary indexes are the only random inserts (in Postgres the same key is a third random B-tree beside a sequential heap).
- No partial index: `audit_actor_idx` also indexes the system rows (NULL `actor_id`). No BRIN: exports range-scan the clustered key. `jsonb` becomes `JSON`, which is fine because `changes` is never indexed.
- Appends do not block each other (insert-intention locks are compatible), but keep the sealer's reads non-locking: a `FOR UPDATE` range scan under `REPEATABLE READ` takes next-key locks.

**What would flip the choice:** 10× (200k inserts/s, ≈ 19k IOPS a shard) passes gp3's 16k cap: `io2`, NVMe or more shards (section 15). Beyond that the move is by age, not by engine: about 90 days in Postgres, older history in Parquet or in a DynamoDB or Cassandra history store fed by the outbox row (capture stays atomic, only the reads move), and the actor index in a columnar or search store. A need for seconds-old analytics across tenants would point to ClickHouse.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 2M audited inserts/s peak (600k/s average ≈ 52B rows a day); 18 TB/day ≈ 6.6 PB a year, 16.2 PB online for two years; the hot tenant (20 %) at 400k/s | the 32 shards: each now takes 62,500 inserts/s ≈ 190k IOPS (12× gp3's 16k cap; section 4 counts two indexes but section 8 defines three, so real IOPS run higher) and holds 506 TB; the same layout by size would be 3,200 shards of 5 TB. The hot tenant alone is 40× a primary | move by age, as section 16 says, not by engine: ~90 days stay in Postgres (1.63 PB); older history goes to a Cassandra or DynamoDB history store fed by the outbox row (capture stays atomic, only reads move; the sheet has no price for it) and to Parquet. ~800 shards by tenant (≈ 2,500 inserts/s ≈ 7.5k IOPS and 2 TB each) behind a cached directory with online splits. The hot tenant spreads over ~40 shards by `entity_id` hash, so one entity's history stays on one shard, and its hash chain becomes one per (tenant, shard). Give up: the actor view of a spread tenant is a scatter-gather, and "any age online in 50 ms" now needs the history store | ≈ $6.8M/month if copied as is; ≈ $1.5M with 90 days online (800 × $1,240 nodes, ≈ $0.3M disks, ≈ $0.2M S3), plus the history store |
| **1,000×** | 20M inserts/s peak (6M/s average ≈ 518B rows a day); 181 TB/day ≈ 66 PB a year; 7 GB/s of diffs at peak; the hot tenant at 4M/s | running the fleet: 20M ÷ 2,500 = 8,000 shards = 16,000 nodes, one fails about every 19 minutes (5,000 nodes ≈ hourly), and because capture is inside the business transaction every shard failure is a business outage for its tenants; 8,000 partition jobs, sealers and archivers are a control plane nobody sized | independent **cells**: ~100 cells, each the 10× design of section 15 (200k inserts/s, ~80 shards), behind a thin `tenant → cell` router. Inside a cell the log leaves the business shard: the transaction writes one small outbox row (or the WAL feeds CDC with the actor in `SET LOCAL`, as in 9.3) instead of a row plus three index inserts, and a per-cell log (70 MB/s, a few partitions) feeds a key store for entity history, ClickHouse for actor and time queries, and the Parquet archive. Give up: history appears seconds after the write, three stores to run, and the per-tenant chain becomes Merkle roots per segment | ≈ $68M/month if copied as is; the log-based cell is cheaper per row but unpriced |
| **10,000×** | 200M inserts/s peak (60M/s average ≈ 5.2T rows a day: ~650 per person alive, a thought experiment and the top of what any pipeline moves); 1.8 PB/day ≈ 650 PB a year; seven years retained ≈ 4.6 EB raw | money: ≈ $684M a month if nothing changes, of which seven years of retention (≈ 770 PB after 6× compression) is ≈ $17.7M a month on S3 alone | change the problem: stop giving every low-value change its own row. Sensitive actions (permissions, money, logins, deletes, exports) keep one atomic entry each; routine edits fold into one entry per (actor, entity, editing session) with the net diff and a count; machine and bulk writes get one entry per job with a manifest in S3 (the `scope: bulk` of section 10); tamper evidence moves to hourly Merkle roots anchored in write-once storage. Give up: the per-save timeline and every intermediate state of routine edits | ≈ $684M/month if nothing changes; far less once routine edits are folded |

**What stays the same at every scale:** the audit entry commits atomically with the business change (one helper that requires a transaction, so no change without a log line), append-only, diffs not snapshots, and an allow-list so secrets cannot be logged.

**Say it in an interview:** "At 100× each of the 32 shards would take 62,500 inserts a second, so I shard by tenant (the same layout by size would be 3,200 shards), spread the hot tenant by entity, and keep only 90 days in Postgres with older history in a key store, as section 16 says. At 1,000× the problem is running 16,000 nodes where a failure is a business outage, so I make cells and take the log off the business shard: an outbox row in the transaction, the history in a log, a key store and a columnar store. At 10,000× that is hundreds of audit rows per person alive per day, so sensitive actions keep atomic entries and routine edits fold into one entry per session."
