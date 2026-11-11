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
