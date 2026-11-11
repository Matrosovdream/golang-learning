# 18 · Inbound Webhook Receiver

`🟢 easy` · *Topics:* acknowledge only what is durable, a log between the endpoint and the database, an inbox with a unique event id (at-least-once delivery, exactly-once effect), "a message is a reason to look" and reconcile-by-pull, signature + replay window + secret rotation, ordering you must not trust, sizing the backlog for an outage · *Builds on:* [10 Webhook Delivery](10-webhook-delivery.md) (the *sending* side), [12 Notification Service](12-notification-service.md), [15 Job Scheduler](15-job-scheduler.md) (leases, at-least-once)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [17 Abuse & Spam Protection](17-abuse-protection.md) · Next: [19 Moderated Submissions](19-moderated-submissions.md)

---

> 🌱 **Seed (honeydolist-site):** ADR 0010 and PLAN Steps 7e.4 / 7e.6 (built 2026-09-24). A delivery from the invoicing provider (and later the bulk-email provider) is verified with CPU alone (no database), published to a **NATS JetStream** stream `INBOUND` (file storage, work-queue retention, refuse-new-when-full, `sync_interval: always`), and answered `200` only once NATS has it on disk; a bad signature is `401`, a queue that cannot take it `503`. The worker drains the stream into `provider_webhook_events` (migration 0025, `UNIQUE (provider, account, event_id)`) and acks **after** the commit; an unreadable body is stored as such and acked. An event never carries the truth: it brings a **pull** forward (`provider_sync_state`, migration 0031: watermark, one pull a minute, a 2-minute overlap, a nightly full pass). Load test: 50,000 deliveries at 50/s with Postgres stopped mid-run, all `200`, p99 6 ms, none lost.
> **Here, at scale:** a payments-style receiver taking 5,000 events/s on average and 50,000/s at peak from many providers, whose database is sometimes down.

## 1. The task

> "Providers call our URL when something happens: a payment cleared, an email bounced, an invoice
> changed. Design the receiver. It must answer fast, never lose an event it acknowledged, survive
> our database being down, and cope with duplicates, reordering and forged requests."

You do not control the sender. It retries on its own schedule, gives up after hours, and some
senders **never retry after a `4xx`**. That asymmetry shapes everything: *our* mistakes must never
look like a `4xx`.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Volume and burstiness? | Decides the log and the inbox. | 5,000/s average, 50,000/s peak (payroll day, a sale), about 1 KB each. |
| What does the sender do on failure? | Defines which refusals are final. | Retries for hours; no retry after `4xx` (Wave: six tries over about 14 h; Mailgun: 8 h). |
| How fast must we answer? | Providers time out. | Under 100 ms; never wait for the database. |
| Does an event carry the truth? | Decides whether order and loss matter. | No: it names an object; the truth is what the provider's API says now. |
| Order guaranteed? | Never assume it. | No. |
| Database down: accept or refuse? | The heart of the design. | Accept: the log holds events until it is back. |
| Many providers and accounts? | Secrets, pacing and state per pair. | 20 providers × 1,000 accounts. |
| Retention? | Storage. | Log: until processed. Inbox: 3 days of ids. Audit envelope: 90 days. |

## 3. Requirements

**Functional**
- Verify each delivery (signature, timestamp), then accept it durably and answer `200`.
- Process every accepted event exactly once *in effect*, whatever the delivery count.
- Recover state even if events were lost, late or out of order.
- Per-provider, per-account secrets that can be rotated without dropping deliveries.

**Non-functional**
- p99 under 100 ms with the database stopped.
- Zero loss of acknowledged events, including across a node crash.
- An outage of the database or the provider API degrades freshness, never correctness.

**Out of scope**
- The sending side ([10](10-webhook-delivery.md)), business handling of each event type, a UI for replays (§14).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Events per day | 432M | 5,000 × 86,400 |
| Peak ingest | 50,000/s = 50 MB/s ≈ 400 Mbit/s | 10× average, 1 KB each |
| Receiver cores | ≈ 5 at peak; run 4 pods × 4 cores | body read + HMAC + a publish wait; budget about 10k/s per core |
| Drain commits | 10/s average, 100/s at peak | batches of 500 rows |
| Inbox ids kept 3 days | 1.3B rows ≈ 130 GB | 432M × 3 = 1.3B × ≈ 100 B (id text, indexes) |
| Backlog in an outage | 10 min at peak ≈ 30 GB · 1 h at average ≈ 18 GB | 50,000 × 1 KB × 600 s · 5,000 × 1 KB × 3,600 s |
| Honeydolist's 1 GB cap | ≈ 1.05M events: full in 21 s at peak, 3.5 min at average | 2³⁰ ÷ 1,024 B; ÷ 50,000; ÷ 5,000 |
| Recovery after a 30 min DB outage | 9M backlog clears in 15 min if drain is 3× arrival | backlog 5,000 × 1,800 = 9M; 9M ÷ (15,000 − 5,000) = 900 s |
| Pull streams | 20 × 1,000 = 20,000; one pull a minute each ≈ 333 requests/s to providers | 20,000 ÷ 60 |

What the numbers tell us: the receive path is cheap CPU. The dangers are the *write path* (50,000 unique-indexed inserts a second is beyond one Postgres' comfortable few thousand writes/s) and the **backlog size** during an outage. ADR 0010 states the honest ceiling of the small design: `sync_interval: always` limits one NATS node to a few hundred messages a second, "the answer is a replicated cluster, not a looser sync".

## 5. Plan

1. The endpoint: what it checks, what it answers, and what it must never answer.
2. The durable accept: a log, and what "acknowledged" has to mean.
3. The drain and the inbox: dedupe, poison messages, ack after commit.
4. Reconciliation: events as hints, a pull as the guarantee.
5. Verification details: signature, window, secrets, floods.
6. Ordering, and sizing for the bad day.

## 6. High-level design

```
 provider ──POST──► LB ──► receiver pods (stateless, NO database on this path)
                            1 flood of bad signatures from this address? → 429          (peek)
                            2 read raw body ≤ 64 KiB
                            3 verify HMAC + timestamp window (+ replay token)  ── fail ──► 401
                            4 publish to the log, WAIT for its ack
                            5 200                                              ── log full/down ──► 503 + Retry-After
                            ▼
              durable log  subjects inbound.<kind>.<provider>.<account>
              (work-queue retention, refuse-new-when-full, replicated at scale)
                            ▼  pull consumers, explicit ack, redeliver forever
              drain workers: batch INSERT inbox ON CONFLICT DO NOTHING ── COMMIT ──► ack
                            ▼
              inbox row ──► marks sync_state.pull_requested_at  ──► puller: "what changed since watermark?"
                                         (nightly: full reconcile)        │ provider API, paged, paced
                                                                          ▼
                                            idempotent upsert into our tables (only if newer)
```

**Normal event:** verified, logged, `200` in a few milliseconds; the drain stores it and asks for a pull; the pull applies what changed.
**Database down:** steps 1–5 are unaffected; the drain does not ack and the log redelivers until the database returns.
**Duplicate:** the log's duplicate window or the inbox's unique key drops it; the effect is idempotent anyway.

## 7. API

```
POST /v1/webhooks/{kind}/{provider}       raw body; the provider's own signature header
     → 200 {}             on the log's ack
     → 401                signature wrong or timestamp outside the window (the sender treats it as final)
     → 404                unknown provider, or webhooks switched off for it
     → 413                body over 64 KiB
     → 429 Retry-After    too many bad signatures from this address
     → 503 Retry-After    the log cannot take it right now: the one refusal a genuine event can meet
GET  /v1/admin/webhooks/status           → { oldest_unprocessed_age_s, backlog, last_pull_at, last_pull_ok, failing_since }
```

## 8. Data model

```sql
-- the inbox: honeydolist migration 0025, narrowed for scale
CREATE TABLE provider_webhook_events (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider     text NOT NULL,
    account      text NOT NULL DEFAULT 'default',
    event_id     text NOT NULL,                 -- the sender's id; unreadable bodies get "unreadable:" + sha256(body)
    event_type   text NOT NULL,
    payload      jsonb NOT NULL,                -- at scale: an envelope + object id only; the log held the body
    received_at  timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    error        text,
    UNIQUE (provider, account, event_id)
);
CREATE INDEX provider_webhook_events_pending_idx ON provider_webhook_events (received_at) WHERE processed_at IS NULL;

-- one row per (provider, account): honeydolist migration 0031
CREATE TABLE provider_sync_state (
    provider text NOT NULL, account text NOT NULL DEFAULT 'default',
    watermark         timestamptz,              -- newest change already mirrored
    pull_requested_at timestamptz,              -- set by the first event of a burst; NULL when nothing is asked
    last_pull_at timestamptz, last_pull_ok boolean, last_pull_error text, failing_since timestamptz,
    last_full_at timestamptz, last_full_ok boolean,
    PRIMARY KEY (provider, account)
);
```

Unreadable-but-signed bodies are stored with `error` set and acked, so they are inspectable but never retried forever.

## 9. Deep dives

### 9.1 Durable accept, not database-first

| Option | Pros | Cons |
|---|---|---|
| `200` at once, process in memory | fastest | a crash loses acknowledged events; the sender will not resend |
| Write Postgres, then `200` (how 7e.4 first shipped) | one store, simple | accepting depends on the database: while it is down you answer `500` or hang and rely on the sender's retries, which some never make after a `4xx` |
| Own spool on local disk | no dependency | a queue to write and recover; the disk dies with the pod |
| **A durable log; `200` after its ack** | the database is off the path; refuse-new-when-full; replay; many consumers | one more stateful service; the *durability setting* is the whole point |

**Choice and why.** The rule set on review: *we do not rely on the sender's retries; an event is received once and nothing in it may be lost, whatever state our database is in.* Hence the log. Honeydolist's stream settings carry the thinking:

- **work-queue retention:** a message leaves when acked, so the stream stays nearly empty;
- **refuse new when full (`DiscardNew`):** answer `503`, never drop what was already accepted; a `503` is retried by senders, a `4xx` is not;
- **never give up on a message** (`MaxDeliver` unlimited): it waits for the database however long;
- **a short duplicate window** (1 hour, keyed by `provider:account:event_id`) drops quick redeliveries before the inbox sees them;
- if the connection to the log is down, **refuse at once** instead of holding the sender behind a timeout.

**What "acknowledged" must mean.** ADR 0010 rejected NATS at its defaults because it flushes to disk only every two minutes while acknowledging immediately (Jepsen's December 2025 analysis of NATS 2.12.1 found a simulated power failure lost about thirty seconds of acknowledged writes), and it rejected Kafka at that size because Kafka's durability comes from *replication*, so a single broker would need a flush per message to mean the same, and it is a 0.5–1 GB JVM. At 50,000 events a day one node with `sync_interval: always` has hundreds of times the headroom needed. At 50,000/s it does not: use a **replicated** stream (an ack means a quorum holds it) and an fsync policy that batches (group commit: one fsync covers every message that arrived while the last one ran). Test it the way the Jepsen analysis did: kill nodes, simulate power loss, and count what vanished. Size the byte cap for the longest outage you will tolerate (≈ 30 GB for 10 minutes at peak), not for the steady state.

### 9.2 The inbox: at-least-once delivery, exactly-once in effect

| Where duplicates are dropped | Pros | Cons |
|---|---|---|
| The log's duplicate window | free, instant | bounded window (an hour); a late duplicate slips through |
| **Inbox table, `UNIQUE (provider, account, event_id)`** | catches any later duplicate; auditable | at 1.3B ids (3 days) ≈ 130 GB: shard by `hash(provider, account)` or shorten |
| **The effect itself is idempotent** (upsert by object id, only if newer) | needs no inbox for correctness | every handler must be written that way |

**Choice and why.** All three, each for a different reason: the log's window for cheap early drops, the inbox for audit and for "show me what Wave sent at 14:02", and **idempotent effects as the real guarantee**. Then the inbox only needs a few days of ids (longer than the longest sender retry horizon of about 14 h), so it can be sharded and pruned (honeydolist prunes after 90 days; at 432M/day, 3).

The drain acks only after the commit, so a crash between commit and ack redelivers and the insert is a no-op:

```go
for {
	batch, err := cons.Fetch(500, jetstream.FetchMaxWait(time.Second))
	if err != nil { continue }
	var msgs []jetstream.Msg
	for m := range batch.Messages() { msgs = append(msgs, m) }
	if len(msgs) == 0 { continue }
	if err := store.InsertEvents(ctx, rows(msgs)); err != nil { // database down: do NOT ack
		time.Sleep(time.Second) // the log redelivers after AckWait (30 s in honeydolist)
		continue
	}
	for _, m := range msgs { _ = m.Ack() } // ack only after COMMIT
}
// InsertEvents: INSERT … SELECT * FROM unnest($1::text[], …) ON CONFLICT (provider, account, event_id) DO NOTHING
```

**Poison messages.** A correctly signed body that cannot be parsed would be redelivered forever. Honeydolist stores it as *unreadable* (with the reason) and acks it: "accepted is never lost, and never retried forever either". An event with no id gets `unreadable:` + the body's SHA-256, which dedupes only byte-identical redeliveries.

### 9.3 A message is a reason to look: reconcile by pull

| Option | Pros | Cons |
|---|---|---|
| Trust the event payload | cheapest | a lost, reordered or stale event leaves wrong state forever |
| Event → re-read that one object | fresh state | 5,000 events/s become 5,000 API calls/s against the provider's rate limit |
| **Event → bring a pull forward** | a burst is a few paged requests; also catches events that never arrived | needs a "changed since" query (or a cursor); otherwise fall back to re-reading open objects |
| Only a periodic pull | no endpoint to secure | freshness = the interval |

**Choice and why.** The webhook is the *accelerator*, the pull is the *guarantee* (ADR 0010, point 3). The pull asks the provider "what changed since the watermark", page by page, mirrors the changes, and moves the watermark to the newest change seen. Honeydolist's constants carry hard-won detail:

- one pull a minute when quiet (the provider allows about sixty requests a minute), never faster than every 30 s;
- **events within 2 s of the first are one pull**: the first event sets `pull_requested_at`, the puller waits that long, then runs once;
- **read again from 2 minutes behind the watermark**: a change can become visible late;
- at most 200 pages per run, the next run carries on; a failed pull is retried after a minute and `failing_since` starts the alarm;
- **a full pass nightly**, so even a bug in the watermark heals.

At scale: 20,000 (provider, account) streams at one pull a minute is 333 requests/s of provider API; schedule with jitter and respect each provider's own limit and pacing. A provider with no "changed since" API gets a periodic re-read of open objects instead. Because state is applied only if newer ([9.5](#95-ordering-dont-trust-arrival-order)), lost, duplicated and reordered events stop mattering.

### 9.4 Verification: signature, window, secrets, floods

| Timestamp window | Pros | Cons |
|---|---|---|
| Tight (the 5 minutes providers' guides suggest) | tiny replay window | a retry is signed with the **original** time, and a refusal is final: a late genuine retry is a lost event |
| **The sender's whole retry horizon + slack** | genuine late retries pass | a wider replay window, covered by dedupe and a replay-token cache |
| No timestamp check | simplest | replays accepted forever (harmless only if everything is idempotent) |

**Choice and why.** The ADR listed three ways *our* code could refuse a genuine delivery: a per-address limit, a five-minute window, an unreadable body. Honeydolist closed each: the window for Wave is **24 hours** (a retry carries the original timestamp), for Mailgun 9 hours (8 h of retries plus 1 h slack) with a 5-minute future tolerance and a cache of used tokens to reject replays; unreadable bodies are stored and acked; the address limit applies to *bad signatures* only (30 a minute), peeked before reading the body and recorded after a failure, so a forged flood costs one hash each and then nothing (see [17](17-abuse-protection.md)). Safe because a replay is a duplicate by event id and an event only asks for a look.

```go
func verify(secrets [][]byte, header string, body []byte, now time.Time, past time.Duration) error {
	ts, sigs := parseHeader(header) // "t=<unix>,v1=<hex>[,v1=<hex>]"
	if d := now.Sub(time.Unix(ts, 0)); d > past || d < -5*time.Minute {
		return ErrWindow
	}
	for _, secret := range secrets { // two while a rotation is in flight
		m := hmac.New(sha256.New, secret)
		fmt.Fprintf(m, "%d.", ts)
		m.Write(body) // the RAW bytes, never re-marshalled JSON
		want := m.Sum(nil)
		for _, s := range sigs {
			if got, err := hex.DecodeString(s); err == nil && hmac.Equal(got, want) { return nil }
		}
	}
	return ErrSignature
}
```

More rules: cap the body (`http.MaxBytesReader`, 64 KiB); compare in constant time; read the secret per delivery from the credential store ([42](42-credentials-vault.md)) so a rotation is one save; if the secret **cannot be read, answer `503` (try again), never `401`**: `401` is final for the sender. IP allowlists are a signal, not a gate; providers change ranges.

### 9.5 Ordering: don't trust arrival order

`invoice.paid` may arrive before `invoice.created`, or twice, or after a later `invoice.voided`. Options: a sender sequence number (rare), per-object ordering by partitioning the log on object id (helps, but still best-effort across retries), or **applying state only if it is newer**. Choose the last, with the pull as ground truth:

```sql
INSERT INTO invoices (provider, external_id, status, provider_updated_at, …)
VALUES ($1, $2, $3, $4, …)
ON CONFLICT (provider, external_id) DO UPDATE
   SET status = EXCLUDED.status, provider_updated_at = EXCLUDED.provider_updated_at
 WHERE invoices.provider_updated_at < EXCLUDED.provider_updated_at;   -- never move backwards
```

Compare the provider's own `updated_at` or version, never our arrival time.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (500k/s peak): the drain's database writes, then a hot account (one customer sending 40% of events). Shard the inbox by account hash, give big accounts their own subjects and consumers so they cannot starve others, and dedupe at the effect.
- **Single points of failure:** the log cluster (replicated, quorum acks) and the load balancer. If the log is unreachable, receivers answer `503`; senders retry and the pull recovers whatever never arrived.
- **Database down:** the log accumulates; alert when the oldest unprocessed message is more than 5 minutes old. The byte cap must hold the outage you are willing to survive (≈ 30 GB for 10 minutes at peak). After recovery, drain faster than arrival: with 3× headroom a 9M-event backlog clears in 15 minutes.
- **Provider API down:** pulls fail, `failing_since` alerts, webhooks keep being stored; nothing is lost, freshness drops.
- **Clock skew** on our side shows up as a rash of `401 outside the window`: alert on that rate.
- **Secret rotation mistakes:** accept old and new together during the overlap.
- **Replay flood with valid signatures:** the inbox/effect dedupe makes it free; add a per-account rate cap to bound the log's growth.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| `200` only after the log's ack | `200` after the database | the database may be down, nothing is lost | a stateful log to run and size |
| Replicated log + group commit at 50k/s | one node, fsync per message | throughput with durability | operational weight (the small design had none) |
| Events as hints, a pull as truth | trusting payloads | loss, reorder and forgery stop mattering | provider API budget and a watermark to maintain |
| Wide timestamp window + dedupe | tight window | genuine late retries accepted | a longer replay window (neutralised by idempotency) |
| `503` for our own trouble | `401`/`4xx` | senders retry | none worth naming |
| Idempotent effects + short inbox | a forever-global inbox | bounded storage | discipline in every handler |

## 12. Check yourself

1. Why answer `200` only after the log's acknowledgement, and what must the acknowledgement guarantee?
2. The log's byte cap is 1 GB and events are 1 KB. How long can the database be down at 50,000/s?
3. Why is an event "a reason to look" rather than the truth?
4. Why is a five-minute timestamp window dangerous even though the provider's guide recommends it?
5. A message is redelivered after the worker's database commit but before its ack. What stops a double effect?

<details><summary>Answers</summary>

1. If the receiver says `200` before the event is safe, a crash loses an event the sender will never resend. The ack must mean *persisted*: on disk with a flush that has really happened, or held by a quorum of replicas. Defaults that flush lazily make "acknowledged" a lie.
2. 2³⁰ ÷ 1,024 B ≈ 1.05M events; ÷ 50,000/s ≈ 21 seconds. Size the cap for the outage you will tolerate: 10 minutes at peak is 50,000 × 1 KB × 600 s ≈ 30 GB.
3. Events can be lost, duplicated, late, reordered or, if a secret leaks, forged. Applying one as state is fragile. Treating it as a hint, then pulling "what changed since the watermark", makes every one of those cases harmless, and the nightly full pass heals watermark bugs.
4. A retry is signed with the original timestamp, and many senders do not retry after a `4xx`: a late genuine retry would be refused and lost. Use the sender's whole retry horizon plus slack (24 h for Wave, 9 h for Mailgun) and rely on event-id dedupe and a replay cache for safety.
5. The inbox insert is `ON CONFLICT (provider, account, event_id) DO NOTHING`, and the effect is an upsert that only applies newer state, so the second delivery changes nothing. That is at-least-once delivery with exactly-once effect.

</details>

## 13. In Go

- Related lessons: [34 Event-driven & outbox](../../34-event-driven-outbox.md), [44 Background jobs & queues](../../44-background-jobs-queues.md) (acks, redelivery, poison messages), [36 Resilience](../../36-resilience-patterns.md), [59 Production DB patterns](../../59-production-database-patterns.md) (`ON CONFLICT`, partial indexes, batch inserts), [55 Data pipelines](../../55-data-pipelines.md) (batching and backpressure), [57 Web security](../../57-web-security.md), [19 Stdlib tour](../../19-stdlib-tour.md) (`crypto/hmac`).
- Read the raw body *before* any JSON decoding; the signature covers those exact bytes. Use `hmac.Equal`, never `==`.
- Drain as `Fetch` → one transaction → ack loop, with `errgroup` for several shards and a context for graceful shutdown that finishes the current batch.
- Packages: `github.com/nats-io/nats.go/jetstream` (or `segmentio/kafka-go`), `pgx/v5` (`unnest` batches or `CopyFrom` into a staging table), `crypto/hmac`, `golang.org/x/sync/errgroup`.

## 14. Extensions

- **Replay UI:** re-deliver event X to the handler; what makes that safe?
- **Receiving for your customers:** per-tenant receiver URLs and secrets (the mirror image of [10](10-webhook-delivery.md)).
- **Poison-message triage:** group unreadable events by provider and error, alert on a new class.
- **Hot-account isolation:** weighted fair consumption so one customer cannot starve the rest.
- **Provider-side queues** (SQS, Pub/Sub) instead of webhooks: pull-based ingestion, no endpoint to defend.
- **mTLS or provider IP allowlists** as an extra gate, and how to keep them current.
