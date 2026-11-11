# 44 · Bulk Email Campaign Sender

`🟢 easy` · *Topics:* daily caps and hourly pace, send windows in the recipient's local time, a cursor per time-zone bucket instead of a plan per recipient, the recipient state machine, at-most-once sending with reconciliation, suppression and one-click unsubscribe, bounce/complaint feedback, domain warm-up, tenant fairness · *Builds on:* [12 Notification Service](12-notification-service.md) (transactional vs bulk, priority lanes), [15 Job Scheduler](15-job-scheduler.md) (`SKIP LOCKED`, due-queries), [10 Webhook Delivery](10-webhook-delivery.md) (retries), [03 Rate Limiter](03-rate-limiter.md) (quotas)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [43 Zero-Downtime Deploys & Schema Migrations](43-zero-downtime-deploys.md) · Next: [45 Live In-App Notifications](45-live-notifications.md)

---

> 🌱 **Seed (honeydolist-site):** *Campaigns → Emails* (PLAN step 22, migration 0048). The campaign is ours; the provider only carries it, behind a `bulkmail.Provider` interface that sends up to 1,000 recipients per call. A start takes a snapshot into `campaign_recipients` (one row per address, `UNIQUE (campaign_id, email)`), events are idempotent on `(provider, provider_event_id)`, `email_suppressions` holds who must not be mailed, and `outbox.lane = 'bulk'` keeps a campaign from queuing in front of a lead alert. Step 28 / migration 0055 added pace and hours: `pace_per_day` *or* `pace_per_hour` (a `CHECK` allows one, never both), a window (a US state → an IANA zone, `window_from`/`window_to` as wall-clock `time`, weekdays only), and `next_batch_at` / `next_why`. The worker queues **one batch at a time**, re-reads the rules before each, and counts the day's sends from `sent_at` since midnight in the window's zone (p95 1.6 ms at 16k copies). Delivery is deliberately **at-most-once**: a crash after the provider said OK leaves rows `sending`, and after 30 minutes `unknown` with a "Send to these again?" button. A complaint rate over 0.3% pauses a campaign. *"Each person's own hours"* was explicitly left out: one zone per campaign.
> **Here, at scale:** 100M emails/day for 5,000 tenants, each with its own daily cap and send window in the **recipient's** local time, new sending domains that must warm up, and one tenant's 10M-person campaign not starving the other 4,999.

## 1. The task

> "Design a bulk email campaign sender. A customer picks an audience and a message, says 'at most 50,000 a
> day, between 9 and 6 in each person's own time', and presses send. It must stay inside those rules, survive
> crashes without mailing anyone twice, stop mailing people who opt out, and show how far it has got."

This is not [12 Notification Service](12-notification-service.md): that one sends one message to one person *now*. This one sends one message to a million people *slowly, politely and on purpose*. The interesting part is not throughput; it is the rules.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Decides whether bookkeeping or sending is the bottleneck. | 100M emails/day, 5,000 tenants, heavy-tailed: the top 1% send about half. |
| Do we run the mail servers? | MTA operation (IP reputation, per-ISP throttling) is a different business. | No: we sit in front of an email provider (ESP) through an adapter, as the seed does. We own the plan, pacing, suppression and state. |
| Pace unit? | One number is simpler than two. | A tenant gives a daily cap *or* an hourly rate, never both (as in the seed). |
| Whose clock is the send window? | One zone per campaign is easy; per recipient is what people expect. | The recipient's, with a fallback zone. This is what the seed deferred. |
| Delivery guarantee? | Duplicates to a customer are worse than a missing copy to a prospect. | At-most-once per recipient, with automatic reconciliation of the uncertain ones. |
| Change pace while sending? | Real campaigns need "raise the cap at 2 PM". | Yes, effective from the next batch. |
| Opt-out? | Law and mailbox-provider rules. | One-click unsubscribe in every message (RFC 8058), honoured immediately. Bounces and complaints suppress automatically. |
| New senders? | Mailbox providers distrust a new domain that suddenly sends 1M a day. | A warm-up ladder per sending domain. |

## 3. Requirements

**Functional**
- Create a campaign from a template and an audience (groups, segments, exclusions); schedule, pause, resume, cancel.
- Pace: a daily cap or an hourly rate. Hours: a window (for example 09:00–18:00, weekdays only) evaluated in each recipient's local zone.
- Never mail a suppressed address; every message carries a one-click unsubscribe.
- Progress: counts per state, the pace, the next batch time, an estimated finish.
- Bounces, complaints and unsubscribes arrive from the provider and update state and suppressions.

**Non-functional**
- 100M/day sustained, about 10k/s design peak; a rule change takes effect within one batch.
- No recipient is mailed twice by our retries or crashes.
- Fairness: a large tenant cannot delay a small one by more than minutes.

**Out of scope**
- Template editor, A/B testing, send-time optimisation by engagement, running our own MTA, image hosting.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Average send rate | ~1,160/s | 100M ÷ 86,400 |
| Rate inside windows | ~2,300/s | windows crowd a US-centred base into about 12 UTC hours (9:00 ET to 18:00 PT): 100M ÷ 43,200 |
| Design peak | 10k/s | about 4× that: tenants without a cap send "as fast as allowed" at start |
| Provider calls | 10/s at peak | 10k ÷ 1,000 recipients per call. Trivial. |
| Recipient state writes | ~20k rows/s peak | two batched `UPDATE`s per recipient (claim, then mark sent); 1,000 rows per statement ⇒ ~20 statements/s |
| Events | ~140M/day, ~1.6k/s avg, ~6.5k/s peak | assume 1.4 events per message (delivered, opens, a few clicks); peak 4× |
| Recipient rows | 40 GB/day, 3.6 TB for 90 days | 100M × ~400 B with indexes |
| Event rows | 28 GB/day | 140M × ~200 B; 30 days raw ≈ 840 GB, then rollups |
| Suppression list | ~200M addresses, ~10 GB | assumption: ~50 B per entry with its index |
| Warm-up of a new domain | ~200/day → ~60k/day in ~4 weeks | illustrative ladder, ×2 every 3 days from day 7 |

What the numbers tell us: **sending is cheap** (10 API calls a second). The load is **bookkeeping**: hundreds of millions of tiny rows a day, so partition by day, batch every write, and drop whole partitions for retention. And the hard part is not volume but **rules**: caps, windows, warm-up, suppression and fairness, all evaluated before each batch.

## 5. Plan

1. Model: campaign, recipients (partitioned), one cursor row per time-zone bucket, batches, quotas, suppressions.
2. Pacing: a plan per recipient, or a cursor with a quota? (Numbers decide.)
3. Send windows in each recipient's local time; DST and weekdays.
4. Progress, pause/resume, crash safety: the recipient state machine and what "exactly once in effect" means for email.
5. Deliverability plumbing: suppression, unsubscribe, bounce/complaint feedback.
6. Warm-up and fairness across tenants.

## 6. High-level design

```
 tenant ─► Campaign API ─► Postgres: campaigns · campaign_buckets (cursors) · campaign_recipients (partitioned by day)
                │                    · batches · quota_usage · suppressions
                │ schedule → outbox 'campaign.start' @ time   (lane: bulk)
                ▼
          Planner (worker): snapshot audience → recipients (each with a tz bucket) − suppressed − excluded
                │           one campaign_buckets row per (campaign, tz)
                ▼
          Dispatcher (N workers, bulk lane):
                │  due-query on campaign_buckets WHERE next_batch_at <= now()  (SKIP LOCKED)
                │  re-read rules → open window? → reserve quota (tenant/day, sending domain/day)
                │  claim ≤ 1,000 'queued' recipients → write batch row → SendBatch(batch id as tag)
                ▼
          Provider adapter ──► ESP ──► inboxes
                ▲                  │ webhooks + periodic pull
                │                  ▼
                │            Inbound receiver ([18](18-inbound-webhook-receiver.md)) ─► events · recipient flags · suppressions
                └─ the cursor moves: next_batch_at from pace, window and remaining quota
```

One start: the planner turns 1M addresses into 1M recipient rows (each knowing its zone) and about 30 cursor rows; a dispatcher wakes a due cursor, checks the rules, reserves quota, claims up to 1,000 rows, sends one provider call and moves the cursor; events settle each row later.

## 7. API

```
POST /v1/campaigns                      { name, template_id, audiences[], sending_domain_id }  → 201 draft
PUT  /v1/campaigns/{id}/pace            If-Match: <version>
                                        { pace_per_day | pace_per_hour,
                                          window: { mode: "recipient_zone" | "fixed_zone", zone?, from, to, weekdays } }
                                                                                   → 200 | 409 (sent/cancelled) | 412 (stale) | 422
POST /v1/campaigns/{id}/schedule        { at }              Idempotency-Key       → 202   (never calls the provider inline)
POST /v1/campaigns/{id}/pause | resume | cancel                                    → 202
GET  /v1/campaigns/{id}/progress        → { states: {queued, sending, sent, delivered, bounced, unknown, cancelled, skipped},
                                            pace, next_batch_at, next_why: "pace"|"hours"|"day"|"warmup", finish_estimate }
POST /v1/campaigns/{id}/resend-unknown                                            → 202
POST /u/{token}                         RFC 8058 one-click, no login, idempotent → 200
GET  /u/{token}                         a confirmation page (scanners issue GETs; a GET must not unsubscribe)
POST /webhooks/email/{provider}         verified, accepted durably, answered 200   (see 18)
```

`next_why` is the seed's small idea worth copying: the screen can always say *why* nothing is going out (outside hours, day's number reached, warm-up cap).

## 8. Data model

```sql
CREATE TABLE campaigns (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id        BIGINT NOT NULL,
    sending_domain_id BIGINT NOT NULL,
    plan_day         DATE,            -- set when the snapshot is taken: claim queries name it, so one partition is read
    status           TEXT NOT NULL DEFAULT 'draft'
                     CHECK (status IN ('draft','scheduled','sending','paused','sent','cancelled')),
    pace_per_day     INTEGER CHECK (pace_per_day > 0),
    pace_per_hour    INTEGER CHECK (pace_per_hour > 0),
    window_mode      TEXT NOT NULL DEFAULT 'recipient_zone' CHECK (window_mode IN ('recipient_zone','fixed_zone')),
    window_zone      TEXT,            -- IANA name: the zone of a fixed window, and the fallback for unknown recipients
    window_from      TIME, window_to TIME,
    window_weekdays  BOOLEAN NOT NULL DEFAULT false,
    version          INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT one_pace CHECK (num_nonnulls(pace_per_hour, pace_per_day) <= 1),
    CONSTRAINT window_order CHECK (window_to > window_from)
);

-- One cursor per (campaign, time-zone bucket): the dispatcher's due-query runs over THIS table, not over recipients.
CREATE TABLE campaign_buckets (
    campaign_id   BIGINT NOT NULL REFERENCES campaigns(id),
    tz            TEXT   NOT NULL,                  -- IANA zone: 'America/Chicago'
    status        TEXT   NOT NULL DEFAULT 'sending' CHECK (status IN ('sending','paused','done')),
    next_batch_at TIMESTAMPTZ NOT NULL,
    next_why      TEXT CHECK (next_why IN ('pace','hours','day','warmup')),
    PRIMARY KEY (campaign_id, tz)
);
CREATE INDEX campaign_buckets_due ON campaign_buckets (next_batch_at) WHERE status = 'sending';

CREATE SEQUENCE recipient_id_seq;                    -- a plain sequence: portable across partitioned-table versions
CREATE TABLE campaign_recipients (
    plan_day    DATE   NOT NULL,                     -- day of the snapshot; the partition key (a function of campaign_id)
    campaign_id BIGINT NOT NULL,
    email       TEXT   NOT NULL,
    tz          TEXT   NOT NULL,
    id          BIGINT NOT NULL DEFAULT nextval('recipient_id_seq'),
    status      TEXT   NOT NULL DEFAULT 'queued'
                CHECK (status IN ('queued','sending','sent','delivered','bounced','failed','unknown','cancelled','skipped')),
    batch_id    BIGINT,
    unsub_token TEXT   NOT NULL,
    claimed_at  TIMESTAMPTZ, sent_at TIMESTAMPTZ,
    PRIMARY KEY (plan_day, campaign_id, email)       -- one copy per address per campaign
) PARTITION BY RANGE (plan_day);
CREATE INDEX recipients_claim ON campaign_recipients (campaign_id, tz, id) WHERE status = 'queued';

CREATE TABLE batches (id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, campaign_id BIGINT NOT NULL,
    tz TEXT NOT NULL, size INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'claimed'
    CHECK (state IN ('claimed','sent','reconciling')), claimed_at TIMESTAMPTZ NOT NULL DEFAULT now());

-- Quotas: one row per (scope, day). Reserve before claiming, refund what was not used.
CREATE TABLE quota_usage (scope TEXT NOT NULL, scope_id BIGINT NOT NULL, day DATE NOT NULL,
    used INTEGER NOT NULL DEFAULT 0, cap INTEGER NOT NULL, PRIMARY KEY (scope, scope_id, day));  -- scope: 'tenant' | 'domain'
```

The unique key on the partitioned table includes the partition key, so `plan_day` is written once at snapshot and never changes. Events and suppressions are separate tables: `events` unique on `(provider, provider_event_id)`; `suppressions (tenant_id, email) PRIMARY KEY, reason, source, created_at` plus a platform-wide row for hard bounces.

## 9. Deep dives

### 9.1 Pacing: a plan per recipient, or a cursor with a quota

The rule is "at most N a day, spread over the window". Where does the spreading live?

| Option | Pros | Cons |
|---|---|---|
| **Materialised plan:** stamp every recipient with a `send_at` at snapshot time; the dispatcher is `WHERE send_at <= now()` | trivial dispatcher; exact per-recipient timing | any rule change (raise the cap, move the window) rewrites every unsent row: 1M rows ≈ 400 MB of WAL and a minute of load, per edit; pause and resume are bulk updates too |
| **Cursor + quota:** rules live on the campaign; one cursor row says when the next batch is due; each batch claims `min(1,000, quota left)` rows | a rule change is one row update; pause is a flag; the due-query touches hundreds of thousands of rows, not hundreds of millions | the pace is approximate (batch granularity); needs a quota counter |
| Token bucket per tenant at dispatch ([03](03-rate-limiter.md)) | fair, smooth | knows nothing about windows or daily totals; useful as an *extra* limiter, not the model |

**Choice and why: cursor + quota.** Rules are re-read before every batch, exactly like the seed's worker. With a daily pace the hourly rate is the cap spread over the window's hours (1,000 a day over 9 hours = 112 an hour, rounded up), a batch is `min(1,000, hourly rate)` and batches are spaced `size ÷ rate` apart. At tenant scale: a 1M/day cap over a 9-hour window is 111,111 an hour, so a batch of 1,000 every ~32 s. The seed counts the day's sends with `COUNT(*)` over `sent_at`; that is fine at 16k rows but a scan at 1M, so we **reserve from `quota_usage`** instead: `UPDATE quota_usage SET used = used + $n WHERE scope='tenant' AND scope_id=$t AND day=$d AND used + $n <= cap RETURNING cap - used`. Zero rows means "the day's number is gone": set `next_why = 'day'` and move the cursor to tomorrow's opening. A refund (`used = used - unused`) closes the gap if a claim returns fewer rows.

### 9.2 Send windows in each recipient's local time

The seed's window is one zone per campaign because "each person's own hours" needs a zone for every person. At scale we resolve one at snapshot time, in this order: the recipient's stored zone → zone from country or state in the address → zone from the signup IP's geolocation → the campaign's fallback zone. Store the IANA name on the recipient (`tz`). There are ~40 distinct zones in practice, so every campaign has **at most ~40 cursor rows**, and the dispatcher is a due-query over cursors:

```sql
SELECT campaign_id, tz FROM campaign_buckets
 WHERE status = 'sending' AND next_batch_at <= now()
 ORDER BY next_batch_at LIMIT 100 FOR UPDATE SKIP LOCKED;
```

Per cursor the next opening is computed with the zone database, never with a UTC offset (an offset is wrong twice a year). Daily caps count days in the *campaign's* zone so one tenant has one number; the window is wall-clock in each bucket's zone, so 9:00 stays 9:00 across a DST change. The function is small:

```go
type Window struct {
	Loc                    *time.Location
	FromH, FromM, ToH, ToM int
	Weekdays               bool
}

// Next returns t if the window is open at t, else the next opening.
func (w Window) Next(t time.Time) time.Time {
	at := func(d time.Time, h, m int) time.Time { // time.Date, not Add: a DST day is not 24 h
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, w.Loc)
	}
	lt := t.In(w.Loc)
	for i := 0; i < 8; i++ {
		d := lt.AddDate(0, 0, i)
		if w.Weekdays && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			continue
		}
		open, closeAt := at(d, w.FromH, w.FromM), at(d, w.ToH, w.ToM)
		switch {
		case t.Before(open):
			return open
		case t.Before(closeAt):
			return t
		}
	}
	return time.Time{} // not reachable for a sane window
}
```

| Option | Pros | Cons |
|---|---|---|
| One zone per campaign (the seed) | one cursor, simplest | someone gets mail at 3 AM; "9 to 6" is true for one city |
| **One cursor per zone bucket** | the rule people mean; bounded (~40 cursors); the same code path | the first hour of a US campaign sends only to Eastern, and finishing takes longer: a 1M campaign cap-limited to 50k/day takes 20 days either way |
| Per-recipient `send_at` | exact | the materialised-plan costs of 9.1 |

**Choice and why: a cursor per zone bucket.** A weekday-only rule is evaluated in the recipient's zone, so a Tokyo recipient's Monday can begin while New York is still on Sunday. Peak load stays flat because each bucket's cap share is spread over its own window.

### 9.3 Progress, pause/resume, and "exactly once in effect"

A recipient moves through one state machine and only the stated actor may move it:

| From → To | Who | Guard |
|---|---|---|
| `queued → sending` | dispatcher | claim query (`SKIP LOCKED`), sets `batch_id`, `claimed_at` |
| `sending → sent` | dispatcher | provider accepted the whole batch |
| `sending → queued` | reconciler | provider confirms it never got this batch |
| `sending → unknown` | reconciler | the provider cannot say, after 30 min |
| `sent → delivered \| bounced` | event applier | idempotent on `provider_event_id`; never moves backwards |
| `queued → cancelled` | cancel | one `UPDATE … WHERE status = 'queued'` |
| `queued → skipped` | dispatcher | suppressed since the snapshot |

You cannot get exactly-once delivery through a network call: the crash can fall *after* the provider accepted and *before* we wrote `sent`. Retrying then mails 1,000 people twice. The honest options:

| Option | Pros | Cons |
|---|---|---|
| At-least-once: retry the batch | nothing is lost | duplicates to real people; unacceptable for marketing mail |
| **At-most-once + reconcile:** never auto-retry a claimed batch; ask the provider what it saw | no duplicates; uncertain rows get resolved automatically | needs a way to ask the provider; a residue of `unknown` |
| Provider-side idempotency: send our batch id as an idempotency key or message tag | a retry is a no-op at the provider | only if the provider supports it; verify before relying on it |

**Choice and why: at-most-once + reconcile, using the batch id as a tag.** The dispatcher writes the `batches` row (`claimed`) *before* the call and sends `campaign_ref` and `batch_id` as tags, so every event comes back carrying them. After a crash the reconciler takes batches in `claimed` older than the lease, asks the provider's event log for that tag, and then either marks the rows `sent` (the provider has them) or returns them to `queued` (it never did). Only a batch the provider cannot answer for becomes `unknown`, which the seed shows as a "Send to these N again?" button. The seed has the button but no automatic reconcile; at 100M a day, manual resolution of every crash does not scale. **Pause** flips the cursors to `paused`; the batch in flight finishes. **Cancel** is one guarded update. **Resume** recomputes `next_batch_at` and never pushes the day past its cap, because the quota reserve is the only door.

### 9.4 Deliverability plumbing

Mailbox providers (the seed cites Gmail's and Yahoo's bulk-sender rules since 2024) expect SPF/DKIM/DMARC, one-click unsubscribe honoured fast, and complaints under 0.3%.

- **Suppression is checked twice**: at snapshot (the seed does this) and again at claim, because a campaign now runs for days and people unsubscribe in between. A suppressed row becomes `skipped`. The check is an anti-join on an indexed primary key: ~10k lookups/s at peak is a fraction of one core, so no extra box. (A Bloom filter to skip lookups for non-members is an optimisation for 10× — §14.)
- **Unsubscribe**: each message carries `List-Unsubscribe: <https://…/u/TOKEN>` and `List-Unsubscribe-Post: List-Unsubscribe=One-Click` (RFC 8058). The token is per recipient and random (the seed stores 24 random bytes). `POST` suppresses at once and is idempotent; `GET` only shows a page, because link scanners fetch every URL.
- **Feedback loop**: bounces, complaints and unsubscribes arrive as signed webhooks ([18](18-inbound-webhook-receiver.md)): accepted durably, applied by an idempotent event table, and backed by a periodic pull so a missed webhook costs nothing (the provider may never retry a `delivered`, as the seed notes). A hard bounce or complaint inserts a suppression and pushes it to the provider's own list (`pushed_at IS NULL` is the work queue, as in the seed).
- **Auto-pause**: if `complaints ÷ delivered ≥ 0.3%` with at least 1,000 delivered (below that it is noise), pause the campaign and tell the owner. With 100M/day, a bad tenant must not be able to burn the platform's sending reputation: the same rule at *domain* scope pauses all of that domain's campaigns.
- **Per-receiving-ISP throttling** (gmail.com, outlook.com…) belongs to the provider when we buy sending, which is the choice here. If we ever run our own MTA, that is a separate task.

### 9.5 Warm-up and fairness: three quotas, one door

New sending domains must climb. An illustrative ladder: 200 a day for days 1–3, 500 for 4–6, 1,000 for 7–9, then doubling every three days, about 64k a day by day 27. It is data, not code: a `warmup_ladder(domain_id, day_index, cap)` row set read when the day's `quota_usage` row for `scope='domain'` is created. A claim must reserve from **both** the tenant's row and the domain's row; whichever is smaller wins, and `next_why = 'warmup'` explains it.

Fairness: the bulk lane has its own concurrency (as the seed: a campaign never holds up a lead alert), and inside the lane each tenant is limited to a small number of in-flight batches (say 4). Then the due-query's `ORDER BY next_batch_at` cannot be monopolised: a 10M campaign has at most 4 batches in flight while a 2k-person campaign's cursor is picked within one dispatch cycle (seconds).

| Option | Pros | Cons |
|---|---|---|
| One global FIFO | simplest | a 10M campaign delays everyone for hours |
| **Per-tenant in-flight cap + due-time order** | small tenants served in seconds; no extra infrastructure | one more counter; needs a tuned cap |

**Choice and why: per-tenant in-flight cap.** Run it as an `UPDATE tenants SET in_flight = in_flight + 1 WHERE in_flight < 4` guard around the claim, decrementing after the provider call (and on crash recovery, by the reaper).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the row write rate (200k recipient updates/s) and the event ingest (65k/s). Shard by `tenant_id` across several Postgres clusters, keep day partitions, move raw events to a columnar store and keep only per-campaign rollups hot. Provider calls (100/s) are still no problem.
- **Single points of failure:** planner and dispatcher are stateless workers; the cursor table is the only coordination, and a crash mid-batch is resolved by the reconciler (9.3). The ESP is a dependency: keep the adapter interface so failover to a second provider is a switch for *unsent* rows only.
- **Provider down:** classify errors; permanent (bad key, unverified domain) pauses the campaign with a reason; transient moves the cursor out with backoff and *refunds the quota*, or the day's number is spent on mail that never went (the seed errs towards sending fewer, never more).
- **Planner on a huge audience:** one `INSERT … SELECT` of 10M rows in one transaction is a bad idea; snapshot in chunks of 50k, with the campaign in `scheduled` until the last chunk commits.
- **Retention:** drop recipient partitions at 90 days and keep per-campaign aggregates and the suppression list for good: the promise to stop mailing must outlive the data.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Cursor + quota | a `send_at` per recipient | rule changes cost one row; the due-query is small | pace accurate only to a batch |
| One cursor per zone bucket | one zone per campaign | "9 to 6" means each person's 9 to 6 | up to ~40 cursors per campaign; slower finish |
| Quota counter row | `COUNT(*)` over `sent_at` | O(1) per batch at any campaign size | a counter to keep honest (reserve/refund) |
| At-most-once + reconcile | retry batches | no duplicates to real people | a residue of `unknown`; reliance on the provider's event log |
| Suppression anti-join, twice | check only at snapshot | opt-outs honoured in multi-day campaigns | one indexed lookup per recipient |
| Per-tenant in-flight cap | one global FIFO | small tenants served in seconds | a tuned constant |
| Buy sending (ESP) | run our own MTA | no IP reputation or ISP throttling work | provider limits and cost |

## 12. Check yourself

1. Why is a cursor per time-zone bucket better than stamping every recipient with a `send_at`?
2. A tenant raises their daily cap from 100k to 200k at 2 PM. What happens, and how is the day's number kept from being exceeded?
3. The dispatcher crashes after the provider accepted a batch but before the rows were marked `sent`. What do we do, and why not just retry?
4. What are the three quotas a claim must pass, and which one explains `next_why = 'warmup'`?
5. Why must `GET /u/{token}` not unsubscribe, and why check suppression again at claim time?

<details><summary>Answers</summary>

1. A rule change (new cap, new window, pause) is one row update per cursor instead of rewriting up to millions of unsent rows, and the due-query runs over a small table of cursors (hundreds of thousands at most) instead of hundreds of millions of recipient rows. Per-recipient local time is still honoured because each bucket has its own zone and window.
2. The rules are re-read before every batch, so the next batch is computed under the new cap and the waiting cursor is moved to "now" if the new rules allow. The day's number is kept by the `quota_usage` reserve: a claim does `used = used + n WHERE used + n <= cap`, and only that statement can authorise sending, so a resume, a retry or a changed rule can never push the day past its cap.
3. Mark the batch for reconciliation after the lease and ask the provider's event log for our batch tag. If it has the messages, mark the rows `sent`; if not, return them to `queued`. Only an unanswerable case becomes `unknown` for a human. Retrying blindly would mail up to 1,000 people twice, which for marketing mail is worse than a delayed or missing copy.
4. Tenant daily cap, sending-domain daily cap (the warm-up ladder), and the platform/provider rate (plus the per-tenant in-flight limit as fairness). The domain's warm-up cap is the one behind `warmup`.
5. Mail security scanners and link prefetchers request every URL in a message; if a GET unsubscribed, they would unsubscribe real customers. RFC 8058 one-click is a POST. Re-checking at claim time matters because multi-day campaigns run long after the snapshot; someone who opted out on day 1 must not receive the day-3 batch.

</details>

## 13. In Go

- Related lessons: [44 Background jobs](../../44-background-jobs-queues.md) (leases, retries, lanes), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (the `campaign.start` job), [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE SKIP LOCKED`, atomic counters, partitions), [68 Rate limiting](../../68-rate-limiting-throttling.md) (quota reserve), [36 Resilience](../../36-resilience-patterns.md) (backoff, classifying permanent errors), [15 Sync & context](../../15-sync-context.md) (a dispatcher loop with graceful shutdown).
- Keep the provider behind one small interface (`SendBatch(ctx, Batch) (Accepted, error)`; optional capabilities discovered by type assertion), with a fake driver that "sends" into a mail catcher; only config code imports a driver package.
- Time zones: `time.LoadLocation` and `time.Date(…, loc)`, never fixed offsets; unit-test both DST transition dates in `America/New_York` and `Europe/Berlin`, plus `America/Phoenix`.
- Packages: `pgx`, `golang.org/x/sync/errgroup`, an HTTP client for the provider, `crypto/rand` for unsubscribe tokens.

## 14. Extensions

- **Bloom filter / in-memory suppression** snapshot per dispatcher to skip lookups for the ~99% of addresses that are not suppressed; measure the false-positive handling.
- **Send-time optimisation:** per-recipient best hour from engagement history; where does that plan live without becoming a materialised plan?
- **Multi-provider failover and cost routing:** unsent rows move to a second ESP; what happens to in-flight uncertainty?
- **Own MTA and open tracking:** IP pools, per-ISP throttling, bot opens (mail privacy features open everything), and why most teams should not.
