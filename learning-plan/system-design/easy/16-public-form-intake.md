# 16 · Public Form Intake

`🟢 easy` · *Topics:* client-made idempotency keys and what a replay returns, the key renewed when the form changes, retrying only what is safe (bounded, jittered), load shedding with `Retry-After`, write + enqueue in one transaction instead of a broker in front, drafts and what never to store, attachments sent after the lead · *Builds on:* [10 Webhook Delivery](10-webhook-delivery.md) (outbox), [12 Notification Service](12-notification-service.md) (idempotency keys), [13 Shopping Cart](13-shopping-cart.md) (idempotent writes), [03 Rate Limiter](03-rate-limiter.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [15 Job Scheduler](15-job-scheduler.md) · Next: [17 Abuse & Spam Protection](17-abuse-protection.md)

---

> 🌱 **Seed (honeydolist-site):** the site's quote, contact, reserve and checkout forms (PLAN Step 7k, built 2026-09-24). The form sends an `Idempotency-Key` (a UUID); the server stores it as `submission_key uuid`, unique where set (today in `0010_requests.sql`; Step 11 rewrote the migrations), inserts with `ON CONFLICT (submission_key) WHERE submission_key IS NOT NULL DO NOTHING`, and a replay answers the first `201` and id, creating no row and no outbox message, and is not counted by the rate limit. The form retries twice (about 1 s and 3 s) only on a dropped connection or `502/503/504`, keeps its draft in `sessionStorage`, and counts its "filled too fast" trap from when the draft was first opened. The request row, an `outbox` message (migration 0014, ADR 0008) and a `pg_notify` commit together. Step 7k decided **no broker in front of the forms**. Photos (Step 83, migration 0105) go after the lead, one request each, authorised by the same key. The owner receives tens of submissions a day.
> **Here, at scale:** a lead-capture product taking 1M submissions a day from phones on bad networks, with 1,000/s spikes when a campaign lands.

## 1. The task

> "Design the endpoint behind a 'Request a quote' form on a site used by millions of people on
> phones. A submission must never be lost and never be duplicated, even when the connection drops
> after the server has already saved it."

Throughput is the easy part. The interesting part is what a client may safely do when it does
not know whether its request arrived: retry, resend, reopen, or give up.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale and shape? | Decides whether one Postgres is enough. | 1M submissions/day; 10× peak evenings; campaign spikes of 1,000/s for 5 minutes. |
| What happens after submit? | Side effects must not block or be lost. | Staff are notified (email/SMS), a CRM gets a webhook; all asynchronous. |
| Does the user need an answer *now*? | Rules out "accepted, validated later". | Yes: field errors (`422`), "too many" (`429`), or an id. |
| How bad is the network? | Decides the retry story. | Phones: about 1% of requests complete on the server but the answer never arrives. |
| Two genuinely different enquiries from one person? | A naive "dedupe by content" fails. | Both are kept. |
| Attachments? | Large bodies change the flow. | Up to 10 photos, 30% of leads carry about 3 at 2 MB each. |
| Configurable fields per form? | The server must hold every lead to the current rules. | Yes, visible/required per field, edited by a non-engineer. |
| Bots? | Traps must not eat real leads. | Yes; layered defence is task [17](17-abuse-protection.md). |
| How long must a key stay valid? | Storage and semantics. | As long as the lead exists; a replay next week is harmless. |

## 3. Requirements

**Functional**
- Submit a form (fields vary per form); get back an id, or per-field errors.
- A retried or double-tapped submission creates exactly one lead and returns the same id.
- A lead is announced (staff alert, CRM) exactly once in effect, even if the mail provider is down.
- Optional photos attach to the lead after it exists.
- A half-typed form survives a reload.

**Non-functional**
- p99 answer under 200 ms in normal operation; never wait on a third party.
- No lead lost once the server answered `201`; none lost to a mail or CRM outage.
- During overload, fail fast with an instruction to retry, not a 30-second hang.

**Out of scope**
- Bot and spam defence ([17](17-abuse-protection.md)), moderation ([19](19-moderated-submissions.md)), the CRM itself.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Average rate | ≈ 12/s | 1,000,000 ÷ 86,400 = 11.6 |
| Normal peak | ≈ 120/s | 10× average |
| Campaign spike | 1,000/s × 300 s = 300,000 submissions | email blast or TV ad |
| Lost responses | ≈ 10,000/day | 1% of 1M reach the server but the phone never hears; without a key each is a duplicate lead a salesperson phones twice |
| Retry load, normal | +2% | per-attempt failure p = 0.02: 1 + p + p² = 1.02 |
| Retry load, partial outage | ×1.75 (worst ×3) | p = 0.5: 1 + 0.5 + 0.25 = 1.75; every attempt failing: 3 attempts each |
| Lead rows | ≈ 1.5 KB each → 1.5 GB/day ≈ 550 GB/year | 1M × 1.5 KB × 365 = 547 GB |
| Key index | ≈ 40 B/entry → ≈ 15 GB/year | 365M × 40 B = 14.6 GB |
| Photo bytes | spike: 1,000 × 0.3 × 3 × 2 MB = 1.8 GB/s ≈ 14 Gbit/s | day: 1M × 0.3 × 3 × 2 MB = 1.8 TB ≈ 21 MB/s average |

What the numbers tell us: **1,000 two-insert transactions a second is within one Postgres**, so throughput does not drive the design. The number that does is *10,000 duplicates a day*, and the second is the photo bandwidth, which must not pass through the API.

## 5. Plan

1. The contract: what the client sends, what it can safely repeat.
2. The key: who makes it, when it changes, what a replay returns.
3. The order of checks in the handler.
4. The atomic write: row + key + outbox in one transaction.
5. Retries on the client, shedding on the server.
6. Drafts and honest messages.
7. Attachments, and fields as server-owned rules.

## 6. High-level design

```
 phone: form open, draft in sessionStorage, key = new UUID (renewed whenever a field changes)
   │ POST /v1/forms/{form}/submissions   Idempotency-Key, hp="", form_elapsed_ms
   ▼
 CDN/LB ──► API (stateless; sheds with 503 + Retry-After when saturated)
              1 traps tripped?  → fake 201 (bot learns nothing)            [17]
              2 key already stored? → answer the FIRST 201 + id, stop
              3 validate against the form's CURRENT rules (drop hidden, 422 missing required)
              4 limit: peek                                               [03]
              5 BEGIN; INSERT submission ON CONFLICT (key) DO NOTHING;
                       INSERT outbox row(s); COMMIT; record limit; 201 {id}
   ◄── 201 | 422 | 429/503 + Retry-After        retry only on status 0, 502, 503, 504 (same key)
                                   │
                                   ▼ worker: SKIP LOCKED + backoff        [10, 15]
                          staff email/SMS [12] · CRM webhook [10] · analytics
 photos: POST /v1/submissions/{id}/files  (same key = proof) → presigned PUT at scale [11]
```

**First write:** key `K`, no row has it, so the insert succeeds, the outbox row commits with it, `201 {id: s_ab12}`.
**Replay (response lost):** the phone retries with `K`; step 2 finds the row; the same `201 {id: s_ab12}`, nothing new, no limit consumed.
**Two in parallel:** both reach step 5; one insert waits on the other's uncommitted row, then does nothing and reads the winner's id.

## 7. API

```
GET  /v1/forms/{form}/rules      → 200 { version, fields: { phone: {visible, required}, … } }   ETag, cacheable
POST /v1/forms/{form}/submissions     Idempotency-Key: <uuid>
       { fields…, "hp": "", "form_elapsed_ms": 8423 }
       → 201 { "id": "s_Xk3…" }       first time and every replay (replays add Idempotent-Replayed: true)
       → 400  key present but not a UUID, or trap fields missing (a broken client is never silently "a bot")
       → 422  { errors: { phone: "Please give a number we can call." } }
       → 429  Retry-After             the limit counts accepted leads only
       → 503  Retry-After             shedding or down: safe to retry with the same key
POST /v1/submissions/{id}/files   Idempotency-Key: <same uuid>   → 201 | 404 (unknown id, wrong key, too old)
```

## 8. Data model

```sql
CREATE TABLE submissions (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id      text   NOT NULL UNIQUE DEFAULT new_public_id('s_'),  -- ADR 0006: random, never the integer key
    form_id        bigint NOT NULL,
    form_version   integer NOT NULL,               -- which rules it was validated against
    fields         jsonb  NOT NULL,
    submission_key uuid,                           -- the form's Idempotency-Key; NULL = header not sent
    body_hash      bytea,                          -- sha256 of the canonical fields: catches a key reused with other data
    status         text   NOT NULL DEFAULT 'new',
    created_at     timestamptz NOT NULL DEFAULT now()
);
-- NULLs are distinct in a unique index, so the predicate is not for correctness; it keeps the index small.
CREATE UNIQUE INDEX submissions_key ON submissions (submission_key) WHERE submission_key IS NOT NULL;
CREATE INDEX submissions_inbox ON submissions (form_id, created_at DESC, id DESC);

CREATE TABLE outbox (                              -- the shape of honeydolist's migration 0014
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic text NOT NULL, payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','done','failed')),
    attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(),
    dedupe_key text, last_error text, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outbox_pending_idx ON outbox (next_attempt_at) WHERE status IN ('pending','failed');
CREATE UNIQUE INDEX outbox_dedupe_waiting ON outbox (dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('pending','failed');
```

Form rules (which fields are shown and required, per form) live in a small versioned table or a `settings` row; the lead stores the `form_version` it was checked against.

## 9. Deep dives

### 9.1 The key: who makes it, when it changes, what a replay returns

| Option | Pros | Cons |
|---|---|---|
| Server-issued single-use token (`GET /token` first) | cannot be guessed or reused; server controls lifetime | an extra round trip on the very network we distrust; the token request can itself be lost; needs state |
| **Client-made UUID** | no extra trip; works from a restored draft; header optional | trusts the client (a buggy one can reuse a key; the damage stays inside its own submission) |
| Hash of the content as the key | zero client work | two genuine identical enquiries collapse into one; an edited-then-retried form looks new |
| No key; dedupe afterwards (same phone + email within 10 min) | no protocol change | heuristics: false merges, misses, and staff were already notified twice |

**Choice and why.** A client-made UUID, with three rules (Step 7k):

- The form makes a key when it opens, or restores it with the draft. Every automatic retry and every re-tap reuses it.
- **A new key whenever the visitor changes a field.** An edited form is a new submission; an untouched one is the same.
- The header is optional (no key behaves as before); a value that is not a UUID is `400`.

Server rules: a replay returns the first answer, same status, same id, with no new row, no outbox message, and no limit consumed. The order of checks matters: traps first (a bot gets its fake `201` whether or not it sent a key), then the key lookup (before validation and the limiter, so a replay can never be refused by a limit its original already passed), then validate, limit, write. Our hardening: store `body_hash`; same key with a different body is `422 key_reused`, because that means a client bug and silently returning the old id would hide it.

The race is solved by the database, not by code:

```go
func (r *Repo) Create(ctx context.Context, s Submission, key *uuid.UUID) (id string, replay bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return "", false, err }
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
	  INSERT INTO submissions (form_id, form_version, fields, submission_key, body_hash)
	  VALUES ($1,$2,$3,$4,$5)
	  ON CONFLICT (submission_key) WHERE submission_key IS NOT NULL DO NOTHING
	  RETURNING public_id`, s.FormID, s.Version, s.Fields, key, s.BodyHash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) { // a parallel attempt with this key committed first
		err = r.pool.QueryRow(ctx, `SELECT public_id FROM submissions WHERE submission_key = $1`, key).Scan(&id)
		return id, true, err
	}
	if err != nil { return "", false, err }
	if _, err = tx.Exec(ctx, `INSERT INTO outbox (topic, payload, dedupe_key) VALUES ('submission.created', $1, $2)`,
		s.Announce(id), "submission.created:"+id); err != nil { return "", false, err }
	return id, false, tx.Commit(ctx)
}
```

If a parallel transaction holds the key but has not committed, the second `INSERT … ON CONFLICT DO NOTHING` waits for it to finish. Why the key sits in the *same row, same transaction* and not in a Redis `SETNX` with a 24 h TTL: a crash between "claim the key" and "insert the row" leaves a key with no lead, and the retry is told "done". For effects outside your database (a payment) you need a key table with a stored response; here the effect is the row itself.

### 9.2 Retries that help, and not making an outage worse

| Option | Pros | Cons |
|---|---|---|
| No automatic retry; the human taps again | simplest; no storms | every dropped connection becomes the user's problem; without a key, the re-tap duplicates |
| Unbounded exponential backoff in the background | eventually succeeds | storms after an outage; spinner forever; a closed tab loses it |
| **Bounded: 2 more attempts (≈ 1 s, ≈ 3 s) on status 0, 502, 503, 504, then back to the human** | covers blips; amplification capped at 3×; fast honest feedback | an outage longer than ~4 s needs a human |
| Offline queue (service worker, background sync) | survives closing the tab | complexity; personal data parked on the device |

**Choice and why.** Bounded, same key, only on errors that mean "I don't know if you got it". Never on `4xx`: `422`, `400`, `429` will not change by asking again. Two honest messages, both true: *"Your connection dropped. Nothing is lost, your details are still here"* (status 0), and *"something went wrong on our side, please call"* (`5xx`); the old single message blamed the server for the phone's signal.

- **Jitter.** Delay = base × random(0.5…1.5). If a load balancer restarts and 30,000 clients fail in the same second, fixed delays make them return in the same second.
- **Amplification is bounded by construction:** three attempts at most, so an outage cannot do worse than ×3 (1,000/s becomes 3,000/s on a struggling service) and a partial one ×1.75. The human tap after the third failure is naturally spread out.
- **Server side: shed, don't queue.** A request refused in microseconds costs nothing; one that waits 30 s holds a goroutine and a database connection. When `Retry-After` is present, the client uses it (with jitter) instead of its own schedule.

```go
var inflight = make(chan struct{}, 512) // per-pod cap, sized from load tests

func shed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case inflight <- struct{}{}:
			defer func() { <-inflight }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", strconv.Itoa(1+rand.IntN(4))) // spread the comeback
			http.Error(w, "busy, retry with the same key", http.StatusServiceUnavailable)
		}
	})
}
```

### 9.3 Write and enqueue atomically; why not a broker in front

| Option | Pros | Cons |
|---|---|---|
| Write the DB, then publish to a broker | simple | a crash between the two: lead saved but nobody told (or the reverse) |
| Broker in front; a consumer writes the DB | absorbs any spike | the endpoint can only say "received"; field errors, `429` and the new id all need the database at submit time; a post-accept failure is a lead the user believes is saved |
| **Outbox in the same transaction** | atomic; the endpoint answers with the database's facts; the worker retries with backoff | the table is a queue (churn, bloat); lag shows as notification delay |
| Outbox + relay into a broker | many consumers, replay, huge fan-out | one more system |

**Choice and why.** Step 7k's verdict, restated at 100,000× the volume: the form needs the database's answers at submit time, and writing to a broker and to Postgres cannot be atomic (the reason the outbox exists). A spike of 1,000/s is absorbed by the outbox: the worker lags by minutes while the form still answers in milliseconds. A broker becomes right when several separate services consume the same events or sustained load reaches thousands per second; even then the form does not change, a relay publishes outbox rows ([10](10-webhook-delivery.md), [18](18-inbound-webhook-receiver.md)).

In the transaction: the row, the key, and **one outbox row per recipient** with `dedupe_key = event id + address`, so a retried fan-out sends nobody twice (honeydolist's Step 7w does exactly this). The worker claims batches with `FOR UPDATE SKIP LOCKED` ([15](15-job-scheduler.md)); honeydolist wakes it with `LISTEN/NOTIFY` and a 5 s poll as fallback. At spike rates keep one caution: a transaction that sends `NOTIFY` takes a lock at commit, so notifying commits can queue behind one another; measure it, and drop to a 1 s poll (at most 1 s of extra notification delay) if it shows.

### 9.4 Drafts: what to keep, where, and what never to keep

| Option | Pros | Cons |
|---|---|---|
| Nothing | simplest | a reload or an accidental close on a phone loses a typed message |
| **`sessionStorage`** | survives reload; dies with the tab; no server involved | lost if the tab is closed; per-tab |
| `localStorage` | survives closing | a name and phone number outlive the session on a shared device |
| Server-side draft, saved as you type | cross-device | needs identity or a draft token; a write per pause; stores data nobody submitted |

**Choice and why.** `sessionStorage`, saved as typed, restored on open, cleared on success. Never saved: the honeypot field, files, anything secret. The **key is saved with the draft**, so reopen-and-send after a lost response replays instead of duplicating (and is renewed on the first edit).

The subtle one: **the "filled in under 2 s" trap must count from when the draft was first opened**, not from the reopening. Otherwise a restored draft sent at once looks like a bot, and is dropped behind a fake `201`, the worst failure there is: a lost lead nobody sees. Test it explicitly: a restored draft sent within a second must be accepted. Remember `form_elapsed_ms` comes from the client: a speed bump, not security.

### 9.5 Attachments after the lead, and fields as server-owned rules

| Option | Pros | Cons |
|---|---|---|
| Photos inside the lead request | one atomic request | a big mobile upload fails often, and one bad photo fails the lead, the thing that must not be lost |
| Presigned upload first, ids in the lead | direct to storage | orphan files when the form is never sent; anonymous writes before any lead exists |
| **Lead first, then each photo separately, authorised by the same key** | the lead cannot be lost to a photo; no new token | staff may be told before the photos arrive; closing the tab mid-upload loses the rest |

**Choice and why.** Step 83's design: at most 10 photos, counted under a row lock on the lead (twelve parallel uploads make ten), accepted only within an hour of the lead, the key is the proof (unknown id, wrong key or too old are one `404`, so no oracle; a bot that got a fake id gets `404`). At scale the *transport* changes, not the contract: 1.8 GB/s of photos cannot cross the API, so `POST …/files` returns a presigned `PUT` ([11](11-image-upload-hosting.md)) good for ten minutes, tied to the lead by the key.

**Fields are rules the server owns** (Steps 37–38): visible/required per field are data, the client reads them, but the server re-applies the *current* rules to every lead. A hidden field is dropped (a page left open for hours cannot sneak it in); a required one that is empty is a `422` on that field. Pages stay open, so accept version N and N−1, and record the `form_version` used.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (10M/day, spikes of 10k/s): commit rate on one primary, then the outbox churn (insert, claim, update, delete). Partition the outbox by day and drop old partitions; shard leads by `form_id` or tenant; the key index grows to ≈ 150 GB/year, so move keys to a narrow table pruned after 7 days (retries happen in minutes).
- **Single points of failure:** the Postgres primary (fail over; meanwhile `503 + Retry-After`). The bounded client retry buys about 4 s; a longer failover shows "connection dropped, your details are still here" and the human returns.
- **Postgres failover of 30 s at 1,000/s:** 30,000 submissions get `503`; each retries three times in about 4 s, then shows the honest message. With drafts kept and keys reused, nothing is lost or doubled, and the returning humans spread themselves out.
- **Mail/CRM down:** leads unaffected; the outbox waits and retries. Alert on the age of the oldest pending message (more than 5 minutes).
- **Limiter store down:** let the lead through (fail open, [03](03-rate-limiter.md)): a lost customer costs more than one extra email.
- **Hot form:** one viral form takes 80% of traffic; per-form concurrency caps so it cannot starve the rest.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Client-made key, renewed on edit | server tokens / content hash | no extra trip; edits are new submissions | trusts the client (contained) |
| Key in the lead's row and transaction | Redis key with TTL | atomic: no key without a lead | a 15 GB/year index |
| Bounded retry, then the human | unbounded backoff | ×3 ceiling, honest feedback | a long outage needs a tap |
| Shed with `503 + Retry-After` | queue and wait | cheap refusals, no pile-up | some users see "busy" |
| Outbox | broker in front | the database's answers at submit time | a queue table to maintain |
| `sessionStorage` draft | server-side drafts | no PII at rest anywhere | lost if the tab closes |
| Lead first, photos after | one big request | the lead is never lost to a photo | alerts may precede photos |

## 12. Check yourself

1. The server saved a lead but the phone never got the `201`. Walk through what happens next, with and without a key.
2. Why does the form make a *new* key when a field changes?
3. Why look up the key before validation and before the limiter?
4. Why is a Redis `SETNX` key with a 24 h TTL a worse home for the key than a column on the lead?
5. A restored draft, sent immediately, is silently discarded. Why, and how do you prevent it?

<details><summary>Answers</summary>

1. With no key, the user taps again, the server creates a second lead and the sales team calls twice: at 1% lost responses that is 10,000 duplicates a day. With a key, the retry (automatic or manual) carries the same `Idempotency-Key`; the lookup finds the stored lead and answers the same `201` and id, creating nothing and consuming no limit.
2. Same key means "the same submission". If the visitor corrects a typo and sends again, that *is* a different submission and must be stored; reusing the old key would return the old id and drop the correction.
3. A replay is not a new submission: validation rules or limits may have changed since, and a limit the original passed must not be able to refuse its own retry. So the order is traps, key lookup, validate, limit, write.
4. A crash between claiming the key and inserting the lead leaves a key with no lead; the retry is told "already done" and the lead is lost. With the key as a column in the same transaction, the key cannot exist without the lead.
5. The timing trap rejects forms completed in under 2 s (a human cannot), measured from when the form opened; a restored draft measured from the reopening has `elapsed ≈ 0` and looks like a bot, so the server returns a fake `201` and keeps nothing. Count from when the draft was first opened, and test that a restored draft sent within a second is accepted.

</details>

## 13. In Go

- Related lessons: [41 API design](../../41-api-design-evolution.md) (idempotency keys, `Retry-After`, problem details), [34 Event-driven & outbox](../../34-event-driven-outbox.md), [59 Production DB patterns](../../59-production-database-patterns.md) (`ON CONFLICT`, `SKIP LOCKED`, partial indexes), [36 Resilience](../../36-resilience-patterns.md) (backoff with jitter, retry budgets, load shedding), [20 HTTP server](../../20-http-server.md) (timeouts, middleware), [57 Web security](../../57-web-security.md).
- Parse the key with `uuid.Parse`; read the body once and hash it (`sha256.Sum256`) for `body_hash`; cap it with `http.MaxBytesReader`.
- Packages: `pgx/v5`, `github.com/google/uuid`, `golang.org/x/sync/semaphore` (an alternative to the channel in the shed middleware), `net/http`.

## 14. Extensions

- **Double opt-in:** confirm the email before the lead counts as "real"; where does the pending state live?
- **Step-up challenge** (proof of work, Turnstile) when the abuse score is high ([17](17-abuse-protection.md)).
- **Service-worker background sync:** an offline queue for tunnels and lifts; what about consent and stale keys?
- **Per-tenant form schemas** with versioned fields and a migration story for old leads.
- **Funnel analytics:** how many forms are opened, drafted, abandoned, sent? (events, not the lead table).
- **Exactly-once CRM sync:** the outbox gives at-least-once; make the CRM call idempotent with its own key ([10](10-webhook-delivery.md)).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Intake API (Go, stateless; the outbox worker runs in the same pods) | campaign spike 1,000 leads/s (× 1.75 with partial-outage retries) + 900 photo presigns/s (300 leads × 3) | CPU | leads 1,750/s × 1 ms (JSON, rules check, `sha256`, key lookup, 4-statement transaction) = 1.75 vCPU; presigns 900 × 0.5 ms = 0.45; total 2.2 vCPU. N = ceil(2.2 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.03) = 2 → 3. Worker: 900k outbox rows from the spike (3 per lead) at ~2,000/s × 0.5 ms ≈ 1 vCPU, after the spike | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS or EKS | 3, one per zone | 3 is the floor for three zones. `xlarge`, not `large`, because the worst case (3 attempts each: 3,000/s ⇒ 3.5 vCPU) must still fit with a zone down: 44 % of 8 vCPU, against 87 % of 4 on `large`. CPU work with tiny memory (512 in-flight cap × a few KB), so `c`. Outbound mail, SMS and CRM calls are waiting, not CPU |
| Postgres (`submissions`, `outbox`, photo rows) | spike: 1,000 lead transactions/s + 900 photo-row transactions/s; 1.5 GB/day ≈ 550 GB a year; key index ≈ 15 GB a year | write IOPS and CPU (the hot set is small) | CPU: leads 1,000 × 1 ms + photos 900 × 0.3 ms + replays 750 × 0.1 ms + outbox claim and ack ≈ 0.4 ⇒ ≈ 1.75 of 4 vCPU; IOPS: ≈ 3 per lead (row + 3 outbox rows + 4 indexes, assumption) = 3k, photos ≈ 1 each = 0.9k, outbox updates ≈ 3k ⇒ ≈ 7k ⇒ `gp3` 8,000 (cap 16,000); hot set: today's leads, today's outbox, recent keys ≈ 5 GB < 16 GiB; disk: 550 GB rows + ≈ 50 GB indexes ⇒ 1 TB | `db.m7g.xlarge` (4 vCPU / 16 GiB), RDS Multi-AZ | 1 primary + synchronous standby | Sized for a 5-minute spike (the average is 12 leads/s) and for "no lead lost after a `201`", hence the synchronous standby. `m`, not `r`: the hot set is a few GB. 44 % CPU at the spike leaves room for the ×3 retry case; a 30 s failover is handled as in section 10 (`503` + `Retry-After`, drafts and keys kept, nothing lost) |
| S3 (photos, direct presigned `PUT`) | spike 300 leads/s × 3 × 2 MB = 1.8 GB/s (14 Gbit/s), 900 `PUT`/s; day 1.8 TB (21 MB/s average) | request rate per prefix | 900 `PUT`/s is under ~3,500 per prefix | S3 Standard | nothing to size | Carrying 14 Gbit/s through the API would need network-family nodes and pay for every byte twice; the presigned `PUT` costs no compute. Spread keys over a few prefixes to stay under the per-prefix rate at 10× |

Retention is not in the lesson: assume photos are kept 90 days (each further month kept adds ≈ $1,260). S3 request fees (~27M `PUT` a month) are small and left out; thumbnails and re-encoding belong to task 11 and are not sized here. Replies are tiny (`201 {id}`), so ALB and egress are small.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.xlarge` (3 × $105; also runs the outbox worker) | $320 |
| Postgres: `db.m7g.xlarge` ≈ $240 managed, Multi-AZ doubles | $480 |
| gp3 1 TB + 5k extra IOPS: (1,000 × $0.08 + 5,000 × $0.005) × 2 = (80 + 25) × 2 | $210 |
| ALB (small requests; photos bypass it) | $20 |
| S3 photos, 90 days: 90 × 1.8 TB = 162 TB × $0.023 per GB | $3,700 |
| **Total** | **≈ $4,730** (≈ $4,450 with 1-year commitments on API and database) |

- **Biggest line and why:** the photos (≈ 78 %), not the lead: a lead is 1.5 KB, three photos are 6 MB, and 30 % of leads carry them. The servers are ≈ $1,000 and are sized for a 5-minute spike while the average is 12 leads/s. The levers are retention and storage tiers, and resizing on upload (task 11), not instance size.
- **What changes at 10×:** 10k leads/s spikes ⇒ leads 10k × 1.75 × 1 ms = 17.5 + presigns 9k × 0.5 ms = 4.5 ⇒ 22 vCPU ⇒ N = ceil(22 ÷ 4.27) = 6 × `c7g.2xlarge`. Postgres: ≈ 19k commits/s ≈ 70k IOPS is past gp3 and one primary ⇒ shard leads by `form_id` or tenant (section 10) into ~6 nodes (≈ 12k IOPS each) and partition and drop `outbox` by day. S3: 18 GB/s (144 Gbit/s) and 9k `PUT`/s ⇒ spread over at least 3 prefixes; photo storage at 90 days is ≈ $37k a month, the line to engineer.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- 1M leads a day (≈ 12/s average, 1,000/s for a 5-minute campaign spike), each ≈ 1.5 KB with a `fields` document: ≈ 550 GB a year, plus a key index of ≈ 15 GB a year (section 4).
- Each submit is one small transaction: the lead row plus about three outbox rows, and a retry with the same key must find the first answer (sections 6 and 9.1). The database has to settle "did this key already win?" atomically.
- Reads are light: the staff inbox by `(form_id, created_at DESC)`, one lead by id, the outbox claimed with `SKIP LOCKED`. No joins that matter, and nothing may be lost once `201` has gone out.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | the unique index on `submission_key` plus `INSERT … ON CONFLICT … DO NOTHING RETURNING public_id` is the whole idempotency guarantee, in the same row and transaction as the lead and outbox rows; the outbox's partial unique index `(dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('pending','failed')` stops a retried fan-out queuing a recipient twice yet lets a finished message's key be reused; `jsonb` for `fields`; `pg_notify` commits with the row and wakes the worker |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | a unique index on a nullable `BINARY(16)` already allows many NULLs, so `submission_key` needs no predicate. `ON CONFLICT … RETURNING` becomes `INSERT … ON DUPLICATE KEY UPDATE id = id` plus the affected-row count (1 new, 0 replay), with `public_id` generated in Go instead of by a SQL default (no `RETURNING` to read it back). The outbox's partial unique index becomes a generated column `IF(status IN ('pending','failed'), dedupe_key, NULL)` with a unique index. No `LISTEN/NOTIFY`, so the worker polls (the 1 s fallback of 9.3). `JSON` is enough, since `fields` is stored and returned, not searched |
| DynamoDB | ✔ works, gains little here | a conditional `PutItem` on the key rejects a replay, `TransactWriteItems` writes lead and outbox items together, Streams can replace the outbox worker, and 1.5 KB items with random UUID keys are comfortable at 1,000/s. But the staff inbox by form and date, search by phone or email and reporting then need GSIs or a second store, and the average is 12 leads/s: nothing here outgrows one primary |
| Redis (ElastiCache) | ✖ as the home of the key | a `SETNX` key with a TTL can exist without a lead after a crash between the claim and the insert, so the retry is told "done" and the lead is lost (9.1); fine for the limiter ([17](17-abuse-protection.md)) |
| S3 | ✅ chosen for photos (not a database) | 1.8 TB a day by presigned `PUT` straight from the phone; the database keeps only the photo row and its key |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.xlarge`, section 15) for leads, keys and the outbox; S3 for photo bytes. Two properties decide it: the unique key lives in the same row and transaction as the lead, so a key cannot exist without a lead, and the partial unique index on the outbox gives per-recipient dedupe while a message is waiting.

**If it were MySQL instead:** four statement-level changes (the idempotent insert, `public_id` made in Go, a generated-column outbox key, a polling worker) and no change to the design: the guarantee, a unique index inside the transaction, is portable. The outbox churns (insert, claim, update, delete): Postgres leaves dead rows for autovacuum, InnoDB updates in place.

**What would flip the choice:** spikes of ~10k leads/s (the 10× case in section 10) are past one primary: shard leads by `form_id` or tenant, still relational. A broker (MSK) becomes right when several services consume the same events or thousands per second are sustained, and it is fed from the outbox, so the form does not change (9.3). DynamoDB only if writes stay above one primary and the inbox and reporting queries have moved to a derived store.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 100k leads/s in a spike (175k with retries; 30M leads in 5 min), 1,200/s average (100M a day, 55 TB of rows a year); 90k photo presigns/s, 180 GB/s; API 220 vCPU | the shard key: sharding by `form_id` (the 10× plan) puts a viral form's 80 % (section 10) = 80k leads/s on one shard, 47× its ~1,700 leads/s (12k IOPS ÷ ~7 per lead); 700k IOPS in all needs ~60 shards anyway | shard by `hash(submission_key)` over ~60 primaries: a replay carries the same key, so the unique index, the lead and its outbox rows stay one local transaction; 54 `c7g.2xlarge` (220 ÷ 4.27 = 52 → 54); photos over ~30 S3 prefixes (90k `PUT`/s ÷ 3,500); outboxes partitioned by day. Give up: "list a form's leads" in one query (the staff inbox becomes a derived read model); keyless requests cannot dedupe across shards (they never could) | ≈ $470k; photo storage ≈ $370k of it |
| **1,000×** | 1M leads/s in a spike (300M in 5 min), 12k/s average (1B a day); 900k presigns/s, 1.8 TB/s of photos; API 2,200 vCPU | the fleet and the sinks: ~580 primaries (7M IOPS ÷ 12k) with standbys plus 516 API nodes ≈ 1,700 machines; and the outbox: 3 rows per lead is 900M messages from one spike, for mail, SMS and CRM providers that cap calls per second | **cells**: ~100, each the 10× stack (~6 primaries, 6 API nodes) behind an edge router on `hash(key)`; per-form and per-tenant admission quotas with `429` or `503` + `Retry-After` (a waiting room) pace a campaign instead of absorbing it; fan-out becomes per-recipient digests (one alert per owner per interval, a bulk call to the CRM). Give up: an instant alert per lead, a single global inbox (derived), moving keys between cells | ≈ $4.7M; photo storage ≈ $3.7M of it |
| **10,000×** | 10M leads/s in a spike (3B in 5 min: a third of humanity), 116k/s average (10B a day); photos 18 PB a day, ≈ 144 Tbit/s in a spike | bytes and money: 90 days of photos is ≈ 1.6 EB ≈ $37M a month of storage alone, and no origin takes the spike | change the problem: the phone downsizes photos (~300 KB, ~7× fewer bytes) and uploads one only when staff open the lead; the edge checks the cached rules, derives the id from the key (a keyed hash) and answers `201` after a replicated log write in its cell, as lesson 18's receiver does, and the database drains the log later, deduping by key. Give up: a database answer at submit time (a rare post-accept failure is a lead the visitor believes saved), photos on demand only, staff see leads seconds later | ≈ $47M if nothing changes; photos are $37M of it |

**What stays the same at every scale:** the client-made `Idempotency-Key`, renewed when a field changes, with a replay returning the first `201`; the lead first and photos after, authorised by the same key; and bounded, jittered retries (the ×3 ceiling).

**Say it in an interview:** "At 100× sharding by form would put a viral form's 80k leads a second on one shard, so I shard by the idempotency key, which keeps a replay on its own shard. At 1,000× I run cells behind a router, pace campaigns with admission quotas and turn per-lead alerts into digests. At 10,000× the photos are the bill, so the phone shrinks them and sends them only when staff open the lead, and the edge answers after a replicated log write, like the webhook receiver."
