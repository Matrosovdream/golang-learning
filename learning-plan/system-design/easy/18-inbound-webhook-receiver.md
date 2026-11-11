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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Receiver pods (Go, stateless, no database on the path) | 50,000/s = 50 MB/s (400 Mbit/s) at ~1 KB; 5,000/s average | CPU | 50,000 × 0.1 ms (section 4's 10k/s per core) = 5 vCPU. N = ceil(5 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(2.3) = 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS or EKS | 3, one per zone | HTTP, TLS and the wait for the log's ack are the work (50,000 × 5 ms ≈ 250 requests in flight, a few MB); HMAC-SHA256 over 1 KB is microseconds on Graviton. `c`, not `m`: no memory to speak of. 42 % busy at peak, 63 % with a zone down. Section 4's "4 pods × 4 cores" becomes 3 so each zone holds one |
| Durable log (replicated stream, `200` only after the quorum ack) | 50 MB/s in at peak, written 3 times; backlog 30 GB (10 min at peak) | disk fsync, then network | per broker (every broker stores everything): 50 MB/s of disk writes; ≈ 50 MB/s in (17 from senders + 33 replication) and ≈ 40 MB/s out (33 replication + drain reads) ≈ 0.7 Gbit/s; backlog 30 GB (18 GB for 1 h at average) against 937 GB of NVMe; duplicate window: 1 h × 5,000/s = 18M ids × ~80 B ≈ 1.5 GB of RAM (≈ 14 GB at the peak rate) | `i4i.xlarge` (4 vCPU / 32 GiB / 937 GB NVMe), self-run NATS JetStream on EC2 (managed alternative: MSK, Kafka) | 3, one per zone | "Acknowledged" must mean durable, so every ack waits for a group-committed fsync: local NVMe has far more IOPS than gp3 and no network hop to EBS. The NVMe dies with the instance, which is fine because 3 nodes hold each message before the ack. 32 GiB fits the duplicate window at peak. Check the node's baseline bandwidth, not its "up to" figure |
| Drain and pull workers (Go) | drain 15,000 rows/s (3× arrival, section 4's recovery rate: 9M backlog in 900 s); pulls 20,000 ÷ 60 = 333 requests/s | CPU, tiny | drain 15,000 × 0.03 ms = 0.45 vCPU; pulls 333 × 0.5 ms = 0.17; total 0.6 vCPU. N = ceil(0.6 ÷ 1.07) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) | 3, one per zone | A fraction of a core of work; three for availability, not load. `Fetch(500)` batches keep the database at 30 commits/s |
| Postgres (inbox, `provider_sync_state`) | drain 15k rows/s (peaks above that wait in the log); inbox 3 days = 1.3B rows ≈ 130 GB | memory (the unique index), then IOPS | `UNIQUE (provider, account, event_id)` is probed at random: assume about half of the 130 GB ≈ 65 GB is hot, 64 GiB of RAM holds ≈ 70 % of it, so 15k × 30 % = 4.5k read IOPS + ≈ 3k dirty-page writes ≈ 7.5k ⇒ `gp3` 10,000; CPU 15k × 0.1 ms = 1.5 of 8 vCPU; disk 130 GB × ~3 for real row widths ⇒ 400 GB | `db.r7g.2xlarge` (8 vCPU / 64 GiB), RDS Multi-AZ | 1 primary + synchronous standby | `r`: the random index probes are the whole load, so the index must live in RAM; CPU stays under 20 %. 50k rows/s would be 5 vCPU and 15k+ read IOPS, past this node, so peaks queue in the log instead: a 10-minute peak leaves ≈ 21 GB and clears in ≈ 35 min. The log is what lets the database be small |

Network and the rest: replies are `200 {}` ≈ 0.3 KB, so 5,000/s ≈ 1.5 MB/s ≈ 3.9 TB a month out. Pulls: 333 requests/s to provider APIs, ~5 KB per reply (assumption) ≈ 4.4 TB a month, through NAT. Data in from the internet is free. If every delivery opened a new TLS connection, the ALB's new-connections dimension (25/s per LCU) would be ≈ 200 LCU ⇒ ≈ $1.2k a month instead of the $120 below.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Receivers: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Log: 3 × `i4i.xlarge` (3 × $250) | $750 |
| Workers: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.2xlarge` ≈ $620 managed, Multi-AZ doubles | $1,240 |
| gp3 400 GB + 7k extra IOPS: (400 × $0.08 + 7,000 × $0.005) × 2 = (32 + 35) × 2 | $130 |
| ALB: $16 + ≈ 18 LCU-hours average on bytes (5 MB/s ≈ 18 GB/h), keep-alive assumed | $120 |
| Egress: 3.9 TB of replies × $0.09 | $350 |
| Pull traffic: 3 NAT gateways × $33 + 4.4 TB × $0.045 | $300 |
| **Total** | **≈ $3,370** (≈ $2,500 with 1-year commitments on the four compute lines) |

- **Biggest line and why:** the inbox database (≈ 41 %: $1,240 + $130 of $3,370), which exists for dedupe and audit; the whole receive path (receivers, log, ALB) is $1,190, ≈ 35 %. The log is the part that earns its cost: it is why the database can be one 8-vCPU node instead of something that must absorb 50k writes a second.
- **What changes at 10×:** 500k/s peak = 500 MB/s (4 Gbit/s) ⇒ receivers 50 vCPU ⇒ N = ceil(50 ÷ 4.27) = 12 × `c7g.2xlarge`. The log writes 1.5 GB/s replicated: on 6 brokers ≈ 250 MB/s of disk and ≈ 2 Gbit/s each way per broker, which is sustained multi-Gbit, so `i4i.8xlarge` class (≈ $2,000 each, 7.5 TB NVMe) or a network family; 10 minutes of backlog is 300 GB. Postgres: 150k rows/s recovery and 13B inbox ids ≈ 1.3 TB ⇒ shard the inbox by account hash (section 10) into ~10 `db.r7g.2xlarge`, and give a hot account (40 % of events) its own subject, consumer and shard. Replies cost ≈ $3.5k a month.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The inbox is write-heavy: 432M events a day (≈ 5,000/s). The drain inserts 500-row batches at up to 15,000 rows/s; the 50,000/s peak is absorbed by the log, not the database (sections 4, 6 and 15).
- Every insert is a dedupe probe: `UNIQUE (provider, account, event_id)` over 3 days ≈ 1.3B rows ≈ 130 GB, probed at random, so the index has to sit in RAM. A row is inserted, marked processed, and deleted after 3 days.
- Effects are upserts "only if newer" into business tables. The provider's API is the final truth (9.3), and the log is the record of an accepted event until it is acked (9.1).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | one statement per batch, `INSERT … SELECT FROM unnest(…) ON CONFLICT (provider, account, event_id) DO NOTHING`; `ON CONFLICT … DO UPDATE … WHERE invoices.provider_updated_at < EXCLUDED.provider_updated_at` is "never move backwards" in one statement (9.5); the partial index `… WHERE processed_at IS NULL` stays tiny while the table holds 1.3B rows; `jsonb` for the envelope. Cost: inserting, updating and deleting 432M rows a day is the churn case for autovacuum |
| MySQL (RDS / Aurora) | ✔ works, same shape | `INSERT … ON DUPLICATE KEY UPDATE event_id = event_id` replaces `DO NOTHING` (`INSERT IGNORE` also hides unrelated errors); a 500-row multi-row `VALUES` replaces `unnest` (~4,000 placeholders, under the 65,535 limit); the "only if newer" upsert needs an `IF(…)` in every assignment, with the timestamp assigned last. The unique-index probes cost the same as in Postgres: random pages that must be in RAM. No partial index: `(processed_at, received_at)` covers all 1.3B rows and moves an entry on every `processed_at` update, so accept that or keep pending ids in a small side table. In its favour: rows update in place, so no dead rows from the `processed_at` flip |
| DynamoDB | ✔ for the inbox alone, ✖ for the effects | a conditional `PutItem` on `provider#account#event_id` is the dedupe in one call, a TTL attribute expires ids after 3 days with no delete job, and hashed keys spread 15k writes/s over partitions (~1,000 writes/s each). But the effect tables join business data and "show me what Wave sent at 14:02" needs a GSI by time, so it would sit beside Postgres, not replace it |
| NATS JetStream or Kafka (MSK) | ✅ chosen as the front log, ✖ as a query store | "acknowledged" means replicated and flushed, with work-queue retention (9.1); self-run JetStream on `i4i.xlarge`, MSK as the managed alternative; it holds events until they are acked and cannot answer ad-hoc questions |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.2xlarge`, section 15; `r` because the index probes must stay in RAM) for the inbox, sync state and effect tables. The log (3 × `i4i.xlarge`) is the record of an accepted event until it is acked; the inbox is a dedupe and audit copy (3 days, prunable) and the provider's API is ground truth. The batch insert with a unique key and the "only if newer" upsert as single statements decide it, and the log is why the database can be one node at 15k rows/s.

**If it were MySQL instead:** the same node class, still memory-bound by the same index, and the edits above, with no change to the design. In both engines a partition column must be part of every unique key, so pruning by dropping day partitions keeps the dedupe only if the column is something a redelivery repeats (the provider's event time), not `received_at`, which differs on every delivery.

**What would flip the choice:** ~15k sustained rows/s is the range of one primary (a 10× recovery is 150k): shard the inbox by `hash(provider, account)` into ~10 nodes (section 15), still relational. Or move the dedupe-only inbox to DynamoDB (conditional put plus TTL) and keep Postgres for effects and audit. Several services reading the same events, or a wish for managed operations, is the point to use MSK instead of self-run NATS.
