# 30 · Invoicing: Money, Tax & PDF

`🟢 easy` · *Topics:* integer money and rounding policy, dated tax rates and snapshots, draft → issued immutability, credit notes instead of edits, async content-addressed PDFs, idempotent payment sync (webhook + reconcile), month-end billing runs · *Builds on:* [20 Per-Tenant Document Numbering](20-document-numbering.md) (gap-free numbers), [18 Inbound Webhook Receiver](18-inbound-webhook-receiver.md) (payment events), [15 Job Scheduler](15-job-scheduler.md) (billing runs, dunning), [29 Price & Availability Engine](29-price-availability-engine.md) (prices in minor units)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [29 Price & Availability Engine](29-price-availability-engine.md) · Next: [31 Service Area & Geocoding](31-service-area-geocoding.md)

---

> 🌱 **Seed (honeydolist-site):** invoices in a small business app (migrations 0024 taxes, 0025 invoices, 0044 invoice features; PLAN steps 7e, 7n, 17). *We own the invoice; a provider issues it*: our `invoices`/`invoice_items` hold what was billed, and once issued the provider's figures (number, tax, payments, `pdf_url`) are **mirrored** back, because "two systems rounding tax separately would disagree by a cent on the one invoice somebody checks". Our own sum is kept as `tax_expected_cents`. Rates are **dated rows** with `EXCLUDE USING gist (jurisdiction_id WITH =, daterange(effective_from, effective_to, '[)') WITH &&)`; an order holds a tax snapshot ("a rate change never re-prices the past"). A line's `amount_cents = round(quantity × unit price)` is computed once and stored. `overdue` is **not a status**: it is computed from `due_on`. Payments are unique on `(invoice_id, external_id)`; provider events land in an inbox unique on `(provider, account, event_id)`; reminders are once-per-step by a partial unique index. honeydolist does **not render PDFs**: the provider returns a URL. Refunds were skipped (step 17).
> **Here, at scale:** *we* are the system of record: 240M invoices a year, billed in a month-end run of ~5M invoices a day, PDFs on demand, every number auditable for years.

## 1. The task

> "Design the invoicing service for a billing platform. At month end it must issue millions of
> invoices with correct tax, let customers pay online, produce a PDF on demand, and never let an
> issued invoice change."

An accounting document is the least forgiving thing a web system makes: cents, dates and numbers are
checked by auditors, not users.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| System of record: us or a provider? | honeydolist mirrors a provider; at scale we own it. | We own the numbers and the PDF; accounting export is an integration. |
| Currencies? | Minor-unit exponents differ (JPY 0, most 2, some 3). | Several; one currency per invoice; exponent from a currency table. |
| Tax model? | Rates change on dates; rules by kind of line. | Dated rates per jurisdiction, taxability per line kind, a tax snapshot per invoice. |
| Can an issued invoice change? | Core invariant. | Never. Corrections are credit notes and a new invoice. |
| Numbering? | Often legally sequential per seller. | Gap-free per tenant, assigned at issue ([20](20-document-numbering.md)). |
| Volumes? | Batch vs interactive. | 240M/year, **5M/day** in a 4-day month-end window, 4-hour nightly run. |
| PDF: eager or on demand? | CPU and storage. | On demand, rendered once, stored forever. |
| Payments? | Webhooks arrive twice, late, or never. | Card/bank via a PSP; partial and over-payments possible. |
| Refunds, write-offs? | Scope. | Refunds as negative payments plus a credit note; write-off out of scope. |

## 3. Requirements

**Functional**
- Draft, edit, issue (freeze, number, tax snapshot), send, void (unpaid only), credit note.
- Record payments (online and manual), derive status: partial, paid; overdue is computed.
- Billing runs: create and issue invoices for a period, re-runnable safely.
- PDF and hosted view per invoice; reminders (dunning) on a schedule.

**Non-functional**
- Totals are exact integers and identical everywhere they are displayed.
- An issued invoice's amounts, lines, rate and number never change; history is reproducible for years.
- A retried request, webhook or billing run never double-issues or double-credits.

**Out of scope**
- Tax *determination* by address (a tax engine), revenue recognition, collections, e-invoicing formats (extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Invoices | 240M/year | 20M a month, all in the month-end window |
| Run throughput | **~58/s** over 24 h, **~350/s** over 4 h | 5M/day ÷ 86,400; 5M ÷ 14,400 s |
| DB time per invoice | ~5 ms (1 header + ~10 lines) | assumption → 350/s × 5 ms ≈ 1.7 cores: the write path is small |
| Row volume | ~2.5 KB per invoice | ~0.5 KB header + 10 lines × ~0.2 KB |
| Storage | ~600 GB/year | 240M × 2.5 KB (plus indexes) |
| PDFs rendered eagerly | **~278 CPU-hours/run day**, ~300 GB/day | 5M × 0.2 s = 1M CPU-s; 5M × 60 KB (assumed) |
| PDFs on demand (10% opened, assumed) | ~28 CPU-hours, ~30 GB/day | one tenth of the above |
| Emails | 5M/day | one per invoice ([12](12-notification-service.md)) |
| Payment events | ~2M/day | assumes ~40% pay online in the first days; webhooks arrive at least once |

What the numbers tell us: the **database** is not the problem (a single primary handles 350 invoices/s).
The costs are **fan-out** (PDFs, emails) and **correctness**: rounding, immutability, idempotency.
Rendering lazily cuts 278 CPU-hours to about 28.

## 5. Plan

1. Money model: integers, rounding policy, dated tax rates, snapshots.
2. Lifecycle and immutability: draft → issued; corrections by credit note.
3. Payment sync: idempotent events, derived status, reconcile.
4. PDF as an idempotent, content-addressed async job.
5. The month-end billing run: re-runnable, resumable, fair.

## 6. High-level design

```
 billing run ─► batches per tenant ─► Invoice service ─► Postgres (invoices, lines, payments, credit_notes, tax_rates)
 customers/staff ─► API ───────────────┘      │ issue: BEGIN; allocate number (task 20); freeze; snapshot; COMMIT
                                              └──► outbox ─► relay ─► email queue (12) · pdf queue · accounting export
 PSP webhooks ─► receiver (18) ─► payment_events inbox ─► worker ─► invoice_payments (idempotent) ─► status
 nightly: reconcile with PSP settlements · overdue/dunning scan (15)
 pdf worker ─► render from frozen snapshot + template_version ─► object storage  pdf/{tenant}/{input-hash}.pdf
```

**Issue** is one transaction: lock the draft, compute and verify totals, take the next number, store
the tax and customer snapshots, flip to `issued`. Everything slow (PDF, email, export) happens after
commit through the outbox, so a slow renderer never delays or loses an issued invoice.

## 7. API

```
POST /invoices                      { customer_id, currency, lines[], tax_point_on? }   → 201 draft
PUT  /invoices/{id}/lines           (draft only)                                        → 200 | 409 "invoice is issued"
POST /invoices/{id}/issue           Idempotency-Key                                     → 200 { number, total, due_on } | 409 | 422
GET  /invoices/{id}                                                                     → 200
GET  /invoices/{id}/pdf             → 302 signed URL | 202 { retry_after_s } while rendering
POST /invoices/{id}/payments        { amount, method, paid_on, external_id }            → 201 | 200 if external_id seen
POST /invoices/{id}/credit-notes    { lines | amount, reason }                          → 201 (own number)
POST /invoices/{id}/void            (unpaid only)                                       → 200
POST /billing-runs                  { tenant_id, period }   idempotent per tenant+period → 202 { run_id }
GET  /billing-runs/{id}             → 200 { total, issued, failed, state }
```

## 8. Data model

```sql
CREATE TABLE tax_rates (                       -- dated, never edited (honeydolist 0024)
    jurisdiction_id BIGINT NOT NULL,
    rate_percent    NUMERIC(6,4) NOT NULL CHECK (rate_percent >= 0 AND rate_percent < 25),  -- 8.8750 is real
    valid           DATERANGE NOT NULL,        -- [from, to)
    EXCLUDE USING gist (jurisdiction_id WITH =, valid WITH &&)
);

CREATE TABLE invoices (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         BIGINT NOT NULL,
    customer_id       BIGINT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'draft'
                      CHECK (status IN ('draft','issued','sent','viewed','partial','paid','void')),  -- no 'overdue'
    number            TEXT,                    -- from the tenant's counter at issue; NULL while draft
    currency          CHAR(3) NOT NULL,
    issued_on DATE, due_on DATE, tax_point_on DATE,
    subtotal BIGINT NOT NULL DEFAULT 0, discount BIGINT NOT NULL DEFAULT 0,
    tax BIGINT NOT NULL DEFAULT 0,      total BIGINT NOT NULL DEFAULT 0,
    amount_paid       BIGINT NOT NULL DEFAULT 0,
    amount_due        BIGINT GENERATED ALWAYS AS (total - amount_paid) STORED,
    customer_snapshot JSONB,                   -- name, address, tax id as at issue
    tax_snapshot      JSONB,                   -- jurisdiction, rate per group, rounding rule, rule version
    template_version  TEXT, pdf_key TEXT,
    billing_run_key   TEXT,                    -- e.g. '2026-10:subscription:884213'
    CHECK (status = 'draft' OR (number IS NOT NULL AND issued_on IS NOT NULL))
);
CREATE UNIQUE INDEX invoices_number_uq ON invoices (tenant_id, number) WHERE number IS NOT NULL;
CREATE UNIQUE INDEX invoices_run_uq    ON invoices (tenant_id, billing_run_key) WHERE billing_run_key IS NOT NULL;
CREATE INDEX invoices_open_idx         ON invoices (due_on) WHERE status IN ('issued','sent','viewed','partial');

CREATE TABLE invoice_lines (
    invoice_id BIGINT NOT NULL REFERENCES invoices(id), line_no INTEGER NOT NULL,
    kind TEXT NOT NULL, description TEXT NOT NULL,
    quantity NUMERIC(12,3) NOT NULL CHECK (quantity > 0), unit_price BIGINT NOT NULL,
    amount BIGINT NOT NULL,                    -- round(quantity × unit_price), computed once in code
    tax_rate NUMERIC(6,4), tax_amount BIGINT,  -- frozen at issue
    PRIMARY KEY (invoice_id, line_no)
);
CREATE TABLE invoice_payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, invoice_id BIGINT NOT NULL REFERENCES invoices(id),
    amount BIGINT NOT NULL CHECK (amount <> 0),            -- negative = a refund
    paid_on DATE NOT NULL, method TEXT NOT NULL, source TEXT NOT NULL CHECK (source IN ('psp','manual')),
    external_id TEXT, UNIQUE (invoice_id, external_id)     -- the idempotency key of a payment
);
CREATE TABLE credit_notes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, invoice_id BIGINT NOT NULL REFERENCES invoices(id),
    number TEXT NOT NULL, amount BIGINT NOT NULL CHECK (amount > 0), tax BIGINT NOT NULL, reason TEXT NOT NULL
);
CREATE FUNCTION invoice_lines_frozen() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF (SELECT status FROM invoices WHERE id = OLD.invoice_id) <> 'draft' THEN
            RAISE EXCEPTION 'invoice % is issued: use a credit note', OLD.invoice_id;
        END IF;
        RETURN OLD;
    END IF;
    IF (SELECT status FROM invoices WHERE id = NEW.invoice_id) <> 'draft' THEN
        RAISE EXCEPTION 'invoice % is issued: use a credit note', NEW.invoice_id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER invoice_lines_frozen BEFORE INSERT OR UPDATE OR DELETE ON invoice_lines
    FOR EACH ROW EXECUTE FUNCTION invoice_lines_frozen();
```

## 9. Deep dives

### 9.1 Money: integers, rounding, rates, currencies

| Decision | Options | Choice |
|---|---|---|
| Representation | `float64` / `NUMERIC` everywhere / **integer minor units** + currency code | integers (`BIGINT`); `NUMERIC` only for quantities and rates (honeydolist: quantity `numeric(10,2)`, rate `numeric(6,4)`, amounts `bigint` cents) |
| Where rounding happens | at display / **once, when the number is computed, then stored** | line `amount` is stored, never recomputed for display (honeydolist: "so it rounds the same way everywhere it is shown") |
| Tax rounding | per line / per invoice per rate | a policy per jurisdiction, written into `tax_snapshot` |
| Currency exponent | assume 2 / **lookup table** | JPY has 0 decimals, some currencies 3: never hard-code `/100` |

Per-line and per-invoice rounding disagree. Five lines of `$1.00` at 8.875%: each line is
`100 × 0.08875 = 8.875¢ → 9¢` (half-up), so **5 × 9 = 45¢**. Taxed once on the `$5.00` subtotal:
`500 × 0.08875 = 44.375¢ → 44¢`. A cent apart, on a document an auditor will recompute. Neither is
"right": the jurisdiction (or your accountant) decides, and the invoice records which rule it used.
This is exactly why honeydolist mirrors the provider's tax and keeps its own sum (`tax_expected_cents`):
when two systems round differently, the *invoice* must say so.

**Rates are dated** rows with the same exclusion trick as [28](28-booking-calendar.md): a new rate
closes the old one, one rate per jurisdiction per day. Which date selects the rate is the **tax point**
(delivery or service date, not always the issue date), so store `tax_point_on`. At issue, copy rate and
rule into the snapshot: "a rate change never re-prices the past". A tax-exempt customer's certificate
(private file, expiry date) is part of the snapshot too.

### 9.2 Issued means immutable

| Option | Pros | Cons |
|---|---|---|
| Edit in place, keep an audit log | familiar | the document a customer holds no longer matches the database |
| **Freeze at issue; correct with credit note + new invoice** | the PDF, the number and the books always agree | corrections are two documents |
| Provider as truth (honeydolist), we mirror | their accounting is the record | two systems, sync lag, the rounding disagreement above |

**Choice and why: freeze.** States: `draft → issued → sent → viewed → partial/paid`, plus `void`.
`issued` requires a number and an issue date (a `CHECK`), lines are frozen by trigger (§8; the issue
transaction writes the line taxes first, then flips the status), and money
columns on the header are only touched by the issue transaction and the payment sync. **Void** is
allowed only while unpaid; after that, a **credit note** (its own number, referencing the invoice)
reverses part or all. **Overdue is computed**, not stored: `due_on < today AND status IN (…)`. It
changes with the clock, not with a write; the partial index `invoices_open_idx` serves the list.
Payments can *over*pay: honeydolist's `amount_paid <= total` check would reject the webhook, so we
accept the payment, flag the surplus as customer credit, and never drop a real payment on a constraint.

### 9.3 Payments: webhooks lie by omission and by repetition

A PSP delivers at least once, out of order, and sometimes not at all.

| Option | Pros | Cons |
|---|---|---|
| Trust each webhook's body | simple | duplicates, reordering, forged or stale amounts |
| **Inbox + idempotent apply + treat the event as a reason to look** | exactly-once *effect*; safe under replays | a re-read per event |
| Poll only | no inbound endpoint | slow, rate-limited, no real-time |

**Choice and why: the receiver from [18](18-inbound-webhook-receiver.md), then an idempotent apply.**
The event is stored in an inbox unique on `(psp, event_id)` (honeydolist: `provider_webhook_events`
unique on `(provider, account, event_id)`). The worker re-reads the payment from the PSP (honeydolist's
rule: "an event is never taken as the truth"), then in one transaction locks the invoice
(`SELECT … FOR UPDATE`), runs `INSERT INTO invoice_payments … ON CONFLICT (invoice_id, external_id) DO
NOTHING`, recomputes `amount_paid = SUM(amount)` and derives `partial`/`paid`. Refunds are negative
payment rows (honeydolist already reads one that way) and a `credit_note`. A **nightly reconcile**
diffs the PSP's settlement report against `invoice_payments`: missing, duplicate and mismatched
amounts become tickets. Reminders are once-per-step by a partial unique index
`(invoice_id, days_delta) WHERE days_delta IS NOT NULL` (honeydolist 0044): a re-run of the dunning
scan cannot email twice.

### 9.4 PDFs: idempotent, content-addressed, rendered once

honeydolist never renders: the provider gives `pdf_url`. When you own it:

| Option | Pros | Cons |
|---|---|---|
| Render at issue for every invoice | always ready | ~278 CPU-hours and ~300 GB per 5M-invoice day, most never opened |
| **Render on first request, store, never re-render** | ~10× less compute and storage (assumes 10% opened) | first view waits (`202` + retry) |
| Re-render on every request | no storage | a new template silently rewrites old invoices |

**Choice and why: lazy, stored, pinned.** The renderer takes only the *frozen* snapshot (lines, both
snapshots, `template_version`), so it is deterministic. The key is the hash of the **inputs**:
`pdf/{tenant}/{sha256(canonical JSON + template_version)}.pdf`. Hash inputs, not output bytes: PDF
generators embed timestamps and fonts, so output hashes drift. Two workers racing for the same missing PDF
write the same key; take `pg_try_advisory_lock(invoice_id)` (or a `singleflight`) so only one renders,
the other answers `202 Retry-After`. Never re-render an issued invoice with a newer template; bump the
version for *new* invoices only. Downloads go through short-lived signed URLs ([11](11-image-upload-hosting.md)).
Emails carry a link to the hosted invoice page (honeydolist has `view_url`), not an attachment.

### 9.5 The month-end run

20M invoices in four days, and every tenant wants theirs first.

- **Idempotent by construction:** each invoice has a `billing_run_key` unique per tenant
  (`'2026-10:subscription:884213'`); the run does `INSERT … ON CONFLICT DO NOTHING`, so a crashed run is
  simply started again. `POST /billing-runs` is unique per `(tenant, period)`.
- **Batches and checkpoints:** keyset-paginate the billable items ([23](23-paginated-lists.md)); each
  batch of ~500 is one transaction; progress counters (`total/issued/failed`) live on the run row.
- **Numbers in order:** issue allocates numbers per tenant from a counter row ([20](20-document-numbering.md)); parallelism
  is *across* tenants, one issuer per tenant keeps gap-free numbers cheap.
- **Fairness:** a per-tenant concurrency cap, so a tenant with 2M invoices cannot starve a tenant with 200.
- **Fan-out after commit:** outbox events feed the email and PDF queues with their own rate limits. Late
  emails are acceptable; an invoice that exists is already issued and visible in the portal.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** (1) email provider limits and PDF CPU, not the DB: scale workers, keep
  PDFs lazy. (2) One giant tenant's counter row: a single issuer lane at ~1k numbers/s is still 3.6M/hour.
  (3) The nightly reconcile on 20M+ payments: partition by date, reconcile per day.
- **A run crashes midway:** restart it; `ON CONFLICT DO NOTHING` on the run key skips what exists.
- **Webhook storm or outage:** the inbox absorbs bursts; a missed event is caught by the pull and the
  reconcile. A webhook for an unknown invoice is parked, not dropped.
- **PDF service down:** invoices still issue and pay; `GET /pdf` answers `202`/`503`, nothing is lost.
- **A rate was wrong:** past invoices are immutable; fix the rate going forward, issue credit notes and
  corrected invoices for the affected ones, driven by a query on `tax_snapshot`.
- **Clock/zone:** `issued_on`, `due_on` and `tax_point_on` are `DATE`s in the tenant's zone; overdue
  flips at local midnight.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| integer minor units + stored line amounts | decimals everywhere | exact, identical totals | a rounding policy to define and document |
| dated tax rates + invoice snapshot | recomputing from current rates | history never moves | a snapshot per invoice |
| freeze at issue, credit notes | editable invoices | document, number and books agree | corrections take two documents |
| inbox + idempotent apply + reconcile | trusting webhook bodies | exactly-once effect | re-reads, a nightly job |
| lazy, input-hashed PDFs | eager rendering | ~10× less CPU/storage | a first-view delay |
| `billing_run_key` unique | a run log | safe re-runs | key discipline in every producer |
| computed `overdue` | a stored status | never stale | a partial index and a query |

## 12. Check yourself

1. Five lines of $1.00 at 8.875%: what are the per-line and per-invoice tax, and why does it matter?
2. Why is an issued invoice immutable, and how do you correct a mistake?
3. A payment webhook arrives three times and one is out of order. What makes the result correct?
4. Why hash the PDF's *inputs* rather than its bytes, and why not re-render with the current template?
5. How do you make the month-end run safe to restart and fair across tenants?

<details><summary>Answers</summary>

1. Per line: 8.875¢ rounds to 9¢, five lines give 45¢. Per invoice: 500 × 0.08875 = 44.375¢ → 44¢. A
   cent apart; the jurisdiction decides, and the invoice stores which rule it used so the document can
   be recomputed by anyone.
2. The PDF, the number and the books must agree with what the customer holds. Freeze at issue
   (`CHECK` + trigger); correct with a credit note and, if needed, a new invoice.
3. The inbox is unique on `(psp, event_id)`, the apply is `INSERT … ON CONFLICT (invoice_id,
   external_id) DO NOTHING` under a row lock, `amount_paid` is recomputed as a sum and the status is
   derived, and the worker re-reads the payment from the PSP, so order and repetition do not matter.
4. PDF writers embed timestamps/fonts, so output bytes vary; the frozen inputs plus the template
   version define the document. Re-rendering with a newer template would silently change an issued
   invoice, so the version is pinned per invoice.
5. A unique `billing_run_key` per invoice with `ON CONFLICT DO NOTHING` makes re-runs harmless;
   batches with checkpoints make them resumable; one issuer lane per tenant and a per-tenant
   concurrency cap keep numbers gap-free and one big tenant from starving the rest.

</details>

## 13. In Go

- Related lessons: [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE`, `ON CONFLICT`, partial indexes, triggers), [34 Outbox](../../34-event-driven-outbox.md) (issue → email/PDF/export), [44 Background jobs](../../44-background-jobs-queues.md) (the PDF worker, rate-limited queues), [31 DDD](../../31-ddd-tactical.md) (an `Invoice` aggregate whose methods refuse changes after `Issue`), [41 API design](../../41-api-design-evolution.md) (`Idempotency-Key`, `409`), [35 Sagas](../../35-sagas-distributed-transactions.md) (payment and refund flows), [38 Caching](../../38-caching-patterns.md) (`singleflight` around renders).
- Money: `int64` minor units. Round once, half-up, in one function with a table-driven test (the 45¢ vs 44¢ case first):

```go
// taxMinor returns round-half-up(amountMinor × rate) for non-negative amounts.
// ratePPM is the rate in parts per million: 8.875% = 88_750. 100¢ → 9¢, 500¢ → 44¢.
func taxMinor(amountMinor, ratePPM int64) int64 {
	return (amountMinor*ratePPM + 500_000) / 1_000_000
}
```

- PDF: `html/template` rendered by a headless browser worker, or a pure-Go library for simple layouts. Keep it behind an interface so a template change is a version bump.
- Packages: `pgx`, `shopspring/decimal` (edges only), `golang.org/x/sync/singleflight`, an S3 client, `go-redis`.

## 14. Extensions

- **Subscriptions and proration:** upgrades mid-period, usage-based lines, a billing calendar per customer.
- **Dunning:** a reminder schedule with once-per-step guarantees; pausing service on non-payment.
- **Tax determination by address:** call a tax engine, cache by `(address, kind, date)`, snapshot the answer; reuse [31](31-service-area-geocoding.md) for the lookup.
- **E-invoicing formats** required in some countries: a second renderer from the same frozen snapshot.
- **Double-entry ledger export:** every invoice, payment and credit note as balanced entries (see the blockchain track's ledger project and the medium backlog's payment service).
- **Multi-entity sellers:** several legal entities per tenant, each with its own series and rates.
- **Statements:** a customer's open invoices and balance, sent through [12](12-notification-service.md).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Invoice API + billing-run workers + payment sync (Go, stateless) | run ~350 invoices/s over 4 h (58/s over 24 h); payments ~23/s avg, assumed 5× = 115/s at peak; portal reads ~300/s (assumed) | CPU, tiny | run 350 × 1 ms (totals, tax, snapshots) = 0.35 vCPU; payments 115 × 1.5 ms (verify, inbox, re-read PSP, apply) = 0.17; reads 300 × 0.3 ms = 0.09; total ≈ 0.6. N = ceil(0.6 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | the run is limited by 5 ms of Postgres per invoice and one counter lane per tenant, not by Go; memory is small, so `c` |
| PDF renderers (headless browser or a Go library) | ~10 % of 5M opened on a run day = 0.5M PDFs × 0.2 s = 28 CPU-hours (eager would be 278) | CPU (+ ~0.5 GB RAM per concurrent browser) | opens cluster in ~8 h: 0.5M ÷ 28,800 s = 17/s × 0.2 s = 3.5 vCPU, peak ~2× = 7. N = ceil(7 ÷ (0.8 × 8 × ⅔ = 4.27)) = 2 → 3 | `c7g.2xlarge` (8 vCPU / 16 GiB), on Spot | 3, scaled by queue depth | CPU-bound, so `c`; 8 concurrent renders × 0.5 GB = 4 GB of 16. Rendering is deterministic and content-addressed, so a Spot reclaim only repeats a job. Graviton if the renderer has an arm64 build, else x86. Eager rendering would need 1M CPU-s ÷ 14,400 s ≈ 70 vCPU for 4 h a day |
| Postgres (invoices, lines, payments, credit notes) | run ≈ 350 × ~14 rows ≈ 4.9k row writes/s in batches of ~500; ~600 GB/yr, 3 years kept ≈ 1.8 TB | memory, then CPU during the run | hot set = recent invoice headers (45 days ≈ 30M × 0.5 KB = 15 GB) + payments (90M × ~100 B = 9 GB) + their indexes ~12 GB ≈ 36 GB (assumed) ⇒ 64 GiB. CPU 350 × 5 ms = 1.75 vCPU of 8 (22 %). IOPS ≈ 4 index pages per invoice × 350 = 1.4k + payments ~0.3k ≈ 1.7k, inside the 3,000 included | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ, `gp3` 2 TB (self-managed: `r7g.2xlarge`) | primary + sync standby; no replica yet | RAM decides (headers and payments cached), CPU and IOPS stay small, so `r`; the lesson's "one primary handles 350/s" holds with room. Multi-AZ because this is the book of record |

S3 holds the PDFs (nothing to size): 10 % of 20M = 2M a month × 60 KB = 120 GB a month ≈ 1.4 TB after a year. Email (20M a month) goes through the notification service ([12](12-notification-service.md)); at an SES-like ≈ $0.10 per 1,000 (outside the cheat sheet, re-check) that is ≈ $2,000 a month, not in the total.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API / run / payments: 3 × `c7g.large` (3 × $53) | $160 |
| PDF renderers: 3 × `c7g.2xlarge` (3 × $212), ≈ $190 on Spot | $640 |
| Postgres: `db.r7g.2xlarge` ≈ $620, Multi-AZ doubles | $1,240 |
| `gp3` 2 TB ($160) × 2 for the standby (3,000 included IOPS is enough) | $320 |
| S3: ~1.4 TB × $0.023 (after year one, +$33 each year kept) | $35 |
| ALB + egress (PDF downloads ~120 GB, portal) | $50 |
| **Total** | **≈ $2,450** (≈ $1,950 with a 1-year plan on API and database; ≈ $1,500 with the renderers on Spot too) |

- **Biggest line and why:** the database tier (~64 %, volumes included), because the invoice is the system of record: two copies of 1.8 TB and a Multi-AZ memory node, for a load the CPU barely feels (1.75 vCPU). The real money is outside this bill: email (~$2k) and the payment provider's fee on every payment. The AWS lever is to detach partitions older than the audit window to S3 (`$0.023` against `$0.08` per GB-month, and out of the standby's copy).
- **What changes at 10×:** 3.5k invoices/s ⇒ 17.5 vCPU of database time and ~49k row writes/s, past one `r7g.2xlarge`, so shard by tenant (the lesson's issuer lanes already run per tenant) into 4 × `db.r7g.2xlarge` Multi-AZ (≈ $5k, ~55 % busy each). PDFs: 70 vCPU peak ⇒ N = ceil(70 ÷ 4.27) = 17 → 18 × `c7g.2xlarge` on Spot (≈ $1.1k at ~30 % of on-demand). Email becomes ≈ $20k.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- We are the system of record for money: 240M invoices a year, ~600 GB a year and ~1.8 TB over 3 years kept, each invoice a header plus ~10 lines written in one transaction (section 4).
- The run writes ~4.9k rows/s for 4 hours in batches of ~500; the hot set (recent headers, payments, indexes) is ~36 GB (section 15).
- Invariants the data must hold: numbers unique per tenant, `billing_run_key` and payment `external_id` unique, lines frozen after issue, no two tax rates for one jurisdiction on one day.
- Payments are applied under a row lock with a recomputed sum; auditors ask questions like "every invoice that used rate X" (section 10).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | transactions, `CHECK`s, unique keys and `FOR UPDATE` carry the invariants; `ON CONFLICT DO NOTHING` makes runs and payments idempotent; the exclusion constraint on `tax_rates` stops overlapping dated rates; `jsonb` snapshots, a stored generated `amount_due`, and the freeze trigger |
| MySQL (RDS / Aurora) | ✔ works, small workarounds | the invoice path is portable. Only the rate-overlap constraint has no equivalent, and `tax_rates` is tiny and written a few times a year (check under a lock, audit nightly). The `WHERE number IS NOT NULL` partial uniques become plain `UNIQUE` indexes, because a unique index accepts many `NULL`s. InnoDB clusters `invoice_lines (invoice_id, line_no)`, so an invoice's lines are contiguous |
| DynamoDB | ✖ poor fit as the record | issue and payment idempotency map to conditional writes (a header plus 10 lines is 11 items, under the 100 limit), but audits and reconciliation are ad-hoc queries and joins over years of rows: "invoices with rate X" would be a scan or an export |
| S3 | ✅ for the PDFs, ✖ for invoice data | `pdf/{tenant}/{input-hash}.pdf` is a key per object (9.4); the database keeps only `pdf_key` |
| Athena / Redshift over exports | ✔ only as a derived store for audit and finance reports | an append-only copy of closed invoices and payments, rebuildable from Postgres; not an OLTP store |

**Pick:** PostgreSQL on RDS Multi-AZ, `db.r7g.2xlarge` with `gp3` 2 TB and no replica yet (section 15), plus S3 for PDFs. The properties that decide it are the ACID transaction that issues a numbered, frozen invoice with its lines (9.2), the unique keys plus `ON CONFLICT DO NOTHING` that make runs, webhooks and payments safe to repeat (9.3, 9.5), and exact types (`BIGINT`, `NUMERIC`) with `CHECK`s. Postgres is the record; PDFs, email and accounting export are derived from frozen snapshots and can be regenerated.

**If it were MySQL instead:** nothing material for the invoice path, which is why either engine is a sound choice here. Rewrites: `INSERT … ON DUPLICATE KEY UPDATE` as a no-op for `ON CONFLICT DO NOTHING`; the freeze trigger in MySQL's trigger syntax with `SIGNAL`; `invoices_open_idx` as a generated column or a `(status, due_on)` index; snapshots as `JSON` with a functional index for the rate query; the counter read back inside the transaction after the row lock (no `RETURNING`). Mind isolation: Postgres' default `READ COMMITTED` makes the post-lock `SUM(amount)` see every committed payment, while under MySQL's `REPEATABLE READ` a plain read made before the lock can leave the sum on an older snapshot, so run it as a locking read or use `READ COMMITTED`.

**What would flip the choice:** 10× is 3.5k invoices/s, ~49k row writes/s, so section 15 shards by tenant into 4 nodes of the same engine (issuer lanes already run per tenant). Past a few TB per shard, closed partitions older than the audit window move to S3 and are queried with Athena rather than changing the record store. A requirement for multi-region active-active books, with no single primary per tenant, is the point to rethink the store.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 35k invoices/s in the 4-hour run (500M a day, 24B a year: three per person alive, a thought experiment), ~490k row writes/s, ~175 vCPU of database time, 60 TB a year; payments ~11.5k/s at peak; PDFs (lazy) ~700 vCPU at peak (eager would be ~7,000); email 2B a month | the primary: 490k row writes/s is ~49× its ~10k ceiling, and 175 vCPU of database time is ~22× the 8 vCPU of a `db.r7g.2xlarge` | shard by `tenant_id` into ~40 shards of today's `db.r7g.2xlarge` Multi-AZ (~55 % busy each): issuer lanes and gap-free numbers are already per tenant, and the webhook receiver routes each event to its tenant's shard through a cached directory. PDFs stay lazy: N = ceil(700 ÷ 4.27) = 165 `c7g.2xlarge` on Spot. Closed partitions older than the audit window go to S3. Give up: cross-tenant queries (finance reports run on Athena); one tenant is capped by one shard and one counter lane (~1k numbers/s ≈ 14M invoices in a 4-hour run) | ≈ $245k/month plus ≈ $200k email (≈ $0.10 per 1,000, outside the cheat sheet) |
| **1,000×** | 350k invoices/s in the run (5B a day, 240B a year: 30 per person alive), ~4.9M row writes/s, 600 TB a year; payments ~115k/s at peak; PDFs (lazy) ~7,000 vCPU; email 20B a month | third-party limits: the worker re-reads every payment event from the PSP (9.3), so 115k provider API calls/s at peak is far past any one provider account's rate limit (provider-specific); email alone is ≈ $2M a month, about the cost of all the servers | change the shape: ~400 independent **cells** (each the 100× shard shape, ~875 invoices/s) behind a cached `tenant → cell` router, each with its own provider accounts and email sender pool; payment re-reads go in batches through the provider's list endpoint, with the nightly reconcile as the backstop. The month-end run starts cell by cell in waves, and tax-rule or template changes roll out wave by wave (a wrong rate then touches 1 % of invoices, each needing a credit note). Give up: per-event real-time certainty becomes batched (minutes); a tenant lives in one cell; global finance reports come from the warehouse | ≈ $2.5M/month plus ≈ $2M email |
| **10,000×** | 3.5M invoices/s in the run (50B a day, 2.4T a year: 300 per person alive, a thought experiment), ~49M row writes/s, ~6 PB a year; payments ~1.15M/s at peak; PDFs (lazy) ~70,000 vCPU; email 200B a month | money and the calendar: ≈ $24.5M a month of servers plus ≈ $20M of email, all of it landing in a 4-day month-end window | change the problem: **rolling billing dates** (each customer on its own anniversary) instead of one month-end run cut the peak from 5M to ~0.67M invoices a day (7.5×); one statement email per customer per period, the hosted page is the invoice and a PDF is rendered only on an explicit download; lines of closed invoices compact into one Parquet object per `(tenant, period)` on S3, the database keeping header, totals, number and tax snapshot (~6 PB a year is ≈ $140k a month per year kept on S3, against ≈ $480k on one gp3 copy). Give up: invoice dates spread through the month (a product change), line-level SQL on closed invoices (Athena instead), per-invoice emails | ≈ $45M/month if nothing changed; far less after |

**Arithmetic:** 350 × 100 = 35k invoices/s; 4.9k × 100 = 490k row writes/s ÷ 10k = 49; 1.75 vCPU × 100 = 175 ÷ 8 ≈ 22, and 175 ÷ (8 × 0.55) = 40 shards; 7 vCPU × 100 = 700 ÷ 4.27 = 164.1 → 165; 20M a month × N × $0.10 per 1,000 = $200k, $2M, $20M; 350k ÷ 875 = 400 cells; 20M ÷ 30 days = 0.67M a day against 5M = 7.5×; 6 PB × $0.023 = $138k, × $0.08 = $480k; $2,450 × N.

**What stays the same at every scale:** integer minor units rounded once and stored, freeze at issue (snapshots, number, credit notes instead of edits), and the idempotency keys: `billing_run_key`, payment `external_id`, the input-hashed PDF key.

**Say it in an interview:** "At 100× one primary sees 490k row writes a second, 49× its ceiling, so I shard by tenant, which fits because issuer lanes and gap-free numbers are already per tenant. At 1,000× the payment provider's and the email provider's quotas bite before any database, so I run ~400 cells, each with its own provider accounts, and batch the payment re-reads. At 10,000× it is 300 invoices per person alive a year, so I change the product: rolling billing dates, one statement per customer, and closed lines compacted to S3."
