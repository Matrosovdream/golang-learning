# 50 · Alerting Engine

`🟢 easy` · *Topics:* rule evaluation at scale, the alert lifecycle as a state machine (pending → firing → resolved), fingerprints and dedup, flapping control, grouping / routing / silences / inhibition, delivery guarantees, who watches the watcher · *Builds on:* [08 Uptime Monitor](08-uptime-monitor.md) (probe verdicts, dead man's switch), [12 Notification Service](12-notification-service.md) (delivery), [15 Job Scheduler](15-job-scheduler.md) (leases, timers), [10 Webhook Delivery](10-webhook-delivery.md) (outbox)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [49 Load Testing Harness](49-load-testing-harness.md) · Next: [51 Runtime Settings & Timed Kill Switches](51-runtime-settings.md)

---

> 🌱 **Seed (honeydolist-site):** two alerting layers. **Infrastructure alerts** (PLAN step 7l, ADR 0011): self-hosted Prometheus +
> Grafana + Loki, with *dashboards and alerts as files in the repo, never clicked together*
> (`infra/ops/grafana/provisioning/alerting/rules.yml`). Ten rules, each with a `for:` duration, evaluated every minute: site failing (5xx above
> 2% for 5 min), slow (API p95 above 500 ms for 10 min), database pool waits, **messages stuck** (oldest queued job older than 5 min, any dead job, or the worker down),
> webhook backlog growing, provider pull out of sync, disk above 80%, container restart loop, **no backup in 26 hours** (a `time() - last_success_timestamp` gauge), memory at 90% of its limit.
> Every rule has `noDataState: OK`. Routing groups by `(alertname, env)`: `group_wait` 30 s, `group_interval` 5 min, `repeat_interval` 4 h (24 h for staging), "fires
> once, repeats every 4 hours while it holds, and says when it has cleared". An **independent second path**: Settings → System reads the API's own checks "so it works even when the `ops` stack is down", and an outside uptime check covers the host itself. **Business-event alerts** (migrations 0039 and 0054, steps 7w, 25.6, 42): audit rows queue an `activity` job; the worker finds *who chose to hear it*
> (`user_alerts`: in-app, email off / instant / daily, all or mine; `role_alert_defaults`), groups repeats within a minute into one inbox row, dedupes per recipient with an outbox `dedupe_key`, and uses the owner's list only as a fallback when nobody covers the event.
> **Here, at scale:** a hosted alerting product: 5,000 tenants, 10M active series, 50k rules, 100k evaluations a minute.

## 1. The task

> "Design an alerting engine. Customers write rules over their metrics. When a rule's condition holds long enough, notify
> the right people once, remind them sensibly, tell them when it is over, and never page twenty times for one incident."

The condition check is easy. What makes an alerting system good is **what it does around** the check: waiting, deduplicating,
grouping, silencing, retrying, and noticing that it has itself stopped.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Where do the numbers come from? | Pull from a TSDB vs streaming evaluation. | A time-series store with a query language; the engine queries it on a schedule. |
| Rule shape? | Evaluation cost. | A query returning one sample per alerting series, a threshold, `for`, labels, annotations. |
| How fast must detection be? | Interval and ingest lag. | 30 s default evaluation, ~20 s ingest delay: detection within about 1-2 min plus `for`. |
| What if data is missing, or the query fails? | The dangerous case. | Per rule: no-data = OK / NoData alert / alerting; an *error* never resolves an alert. |
| Delivery channels, who is notified? | Routing model. | Email, chat, webhook, pager; static receivers or resolved from subscriptions at send time. |
| Silences, maintenance windows, inhibition? | Noise control. | Yes: silences by matchers and time window; inhibition of dependent alerts. |
| Scale? | Sharding, budgets. | 5,000 tenants, 10M series, 50k rules, 100k evals/min, incidents of 5k simultaneous alerts. |
| Tenancy? | Isolation of cost and config. | Per-tenant rules, routes, receivers, query budgets; one tenant cannot slow another. |

## 3. Requirements

**Functional**
- Rules with `for`, labels, annotations; per-rule interval; test a rule before saving it.
- Alert lifecycle with history: pending, firing, resolved; "resolved" notices only for alerts that fired.
- Routing tree, grouping, silences, inhibition, reminders; delivery with retries and a fallback receiver.
- Meta-monitoring: the engine proves it is alive, and says when it is not.

**Non-functional**
- Detection latency: minutes, not seconds; **no lost alert** (a fired alert is notified at least once); duplicates are tolerable, floods are not.
- An evaluator restart must not reset `for` timers or re-notify everything.
- Tenant isolation of query cost.

**Out of scope**
- The time-series database itself, on-call scheduling and escalation policy (extensions), dashboards.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Evaluations | ~1,670/s | 100k per minute ÷ 60; = 50k rules × 2 per minute (30 s default) |
| Ingest | ~333k samples/s | 10M active series ÷ 30 s scrape interval |
| Series per tenant | ~2,000 | 10M ÷ 5,000 |
| Rules per tenant | ~10 | 50k ÷ 5,000 |
| Query read load | ~1.7M samples/s | 1,670 evals/s × ~100 series × 10 samples (5 min at 30 s): *5× the ingest rate* |
| Evaluator capacity | ~800 evals/s each | assume 5 ms per query, 4 in parallel: 4 × 200/s |
| Evaluators needed | 3, ×2 for HA = 6 | 1,670 ÷ 800 = 2.1 |
| Alert instances tracked | ~200k | 50k rules × ~4 label sets |
| State memory | ~40 MB | 200k × ~200 B (fingerprint, labels, state, timestamps, last value) |
| Firing at steady state | ~2k (1%) | assumption |
| State transitions | ~50k/day (0.6/s), 10× in an incident | assumption; flapping inflates it |

What the numbers tell us: alert state is tiny (it fits in one process); **the expensive part is the queries**. 50k rules re-read
the recent past of the TSDB 1,670 times a second, so rule *cost* (series touched × window) needs a budget, and a "recording rule"
(precompute an expensive expression once) beats ten rules each recomputing it. Delivery volume is small until an incident: then
the problem is a *storm* (5,000 alert instances in a minute), not throughput.

## 5. Plan

1. Evaluate rules on a schedule, sharded across evaluators, with query budgets and a policy for missing data.
2. The alert lifecycle as a state machine with a stable identity (fingerprint).
3. Control noise: `for`, hysteresis, grouping, silences, inhibition.
4. Deliver reliably through an outbox, with retries, reminders and "resolved".
5. Watch the watcher: heartbeat, self-metrics, an independent path.

## 6. High-level design

```
 TSDB (10M series) ◄───── queries ───────── Evaluators (shards by hash(rule_id), leased)
                                               │  state machine per instance, in memory
                                               ▼  transitions only
                                     Postgres: alert_state, alert_events, alert_notifications (outbox)
                                               │
                           Notifier ◄──────────┘  group by route labels · wait/interval timers · silences · inhibition
                              │ at-least-once, dedupe key
                              ▼
          Notification service (12) ──► email / chat / webhook / pager        In-app inbox (45)
                                               ▲
 Watchdog rule (always firing) ──► external dead man's switch ─── silence for N min = page the platform team
```

Per evaluation: the evaluator runs the query at `now − ingest_delay`, turns each returned series into an *instance* identified
by its label fingerprint, steps each instance's state machine, and writes **only transitions** to Postgres (inside the same transaction
as the outbox row that announces them). The notifier turns transitions into *grouped* notifications.

## 7. API

```
PUT  /v1/tenants/{t}/rules/{name}  { expr, for, keep_firing_for, interval, no_data, exec_error, labels, annotations } → 200 { version }
POST /v1/tenants/{t}/rules:test    { expr, at? }                → 200 { series_matched, would_fire: [...], eval_ms }   bounded dry run
GET  /v1/tenants/{t}/alerts?state=pending,firing&label=env=prod → 200 page
POST /v1/tenants/{t}/silences      { matchers: [{name, op, value}], starts_at, ends_at, comment }  → 201
PUT  /v1/tenants/{t}/routes        { tree }                     → 200   validated: every path ends in a receiver of this tenant
POST /v1/tenants/{t}/receivers/{id}:test                         → 202   sends a labelled test notification
GET  /internal/watchdog                                          → 200 only if the last full evaluation cycle finished < 2 intervals ago
```

## 8. Data model

```sql
CREATE TABLE alert_rules (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL, name text NOT NULL,
  expr text NOT NULL,                                   -- returns one sample per alerting series
  for_seconds integer NOT NULL DEFAULT 0, keep_firing_seconds integer NOT NULL DEFAULT 0,
  interval_seconds integer NOT NULL DEFAULT 30 CHECK (interval_seconds BETWEEN 10 AND 3600),
  no_data text NOT NULL DEFAULT 'ok' CHECK (no_data IN ('ok','nodata','alerting')),
  exec_error text NOT NULL DEFAULT 'keep' CHECK (exec_error IN ('keep','alerting')),
  labels jsonb NOT NULL DEFAULT '{}', annotations jsonb NOT NULL DEFAULT '{}',
  version integer NOT NULL DEFAULT 1, enabled boolean NOT NULL DEFAULT true,
  UNIQUE (tenant_id, name)
);
CREATE TABLE alert_state (                              -- one row per alert instance; written on TRANSITIONS only
  rule_id bigint NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  fingerprint bigint NOT NULL,                          -- hash of the sorted label set: the instance's identity
  labels jsonb NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','firing','resolved')),
  active_at timestamptz NOT NULL,                       -- condition first true: the `for` timer survives a restart
  fired_at timestamptz, resolved_at timestamptz, last_value double precision,
  PRIMARY KEY (rule_id, fingerprint)
);
CREATE INDEX alert_state_live_idx ON alert_state (rule_id) WHERE state IN ('pending','firing');
CREATE TABLE silences (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL, matchers jsonb NOT NULL,
  starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL, created_by bigint NOT NULL, comment text NOT NULL,
  CHECK (ends_at > starts_at)
);
CREATE INDEX silences_active_idx ON silences (tenant_id, ends_at);
CREATE TABLE alert_notifications (                      -- the outbox (task 10): written in the transition's transaction
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL, group_key text NOT NULL,
  receiver_id bigint NOT NULL, kind text NOT NULL CHECK (kind IN ('firing','resolved','reminder')),
  payload jsonb NOT NULL, dedupe_key text NOT NULL UNIQUE,   -- group_key:version:kind: a retry cannot send twice
  status text NOT NULL DEFAULT 'pending', attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE eval_leases (shard integer PRIMARY KEY, owner text NOT NULL, lease_until timestamptz NOT NULL);
```

## 9. Deep dives

### 9.1 Evaluating 100k rules a minute

| Option | Pros | Cons |
|---|---|---|
| **Scheduled queries against the TSDB** | any query the TSDB supports; simple; rules are data | read load of 1.7M samples/s; staleness = interval |
| Streaming evaluation as samples arrive | lowest latency; no read load | restricted to simple conditions; complex state, hard backfill |
| Scheduled queries on **recording rules** | expensive expressions computed once, shared by many alerts | extra moving part; a stale recording rule hides a problem |

**Choice and why.** Scheduled queries, with recording rules for expressions several alerts share. Mechanics:

- **Sharding.** `shard = hash(rule_id) mod S`; an evaluator holds a *lease* on each of its shards (`eval_leases`, the lease pattern of [15](15-job-scheduler.md)) and keeps its rules in memory. A dead evaluator's shards are taken over when the lease expires; the gap is at most one lease period. Duplicate notifications from an overlap are removed by the notification `dedupe_key`.
- **Spread, don't stampede.** Each rule's first evaluation is offset by `hash(rule_id) mod interval`, so 50k rules do not all query at second :00.
- **Evaluate in the past.** Query as of `now − 20 s` so late samples have landed; otherwise a slow ingest looks like a gap and flaps. The cost is 20 s of detection delay: say so.
- **Query budgets.** Per tenant: series touched, samples, 10 s timeout, a cap on active series (cardinality: a `user_id` label turns one series into a million). `rules:test` runs the query with the same limits and returns the cost before the rule is saved. A rule that blows its budget is skipped and counted (`missed_evaluations`), never allowed to slow others.
- **Missing data and errors are three different things.** *No data* (the query returned nothing), *an error* (the query failed), and *the condition false*. The seed sets `noDataState: OK` on all ten rules: right for ratios driven by traffic (no requests, no 5xx), but it means **a dead exporter silences its own alerts**. The cure is an explicit rule for absence (an `up == 0` or `absent()` rule), or the seed's **last-success timestamp** pattern: "no backup in 26 hours" is `time() - last_success_timestamp > 26h`, an alert on the *absence of a good event*, which works even if the thing that should report has died. **An evaluation error never resolves an alert and never fires one**: keep state, count the error, and alert on the engine if errors persist.

### 9.2 The lifecycle, and not flapping

```
 inactive ──cond true──► pending ──true for ≥ `for`──► firing ──cond false for ≥ `keep_firing_for`──► resolved ──► (inactive, row kept ~15 min)
              ▲              │ cond false: back to inactive, silently (never fired, so no "resolved" notice)
```

An instance's identity is its **fingerprint**, a hash of its sorted label set (including `alertname` and tenant). `active_at` is persisted at
the first transition to *pending*, so a restart reloads it from `alert_state` and the `for` timer is not reset (otherwise a restart at minute 4 of a 5-minute
`for` delays the page by 4 minutes, and every restart delays every alert).

```go
// step advances one instance by one evaluation and returns the transition, if any (types elided).
func step(in *Instance, cond bool, now time.Time, r Rule) (ev Event) {
	if cond {
		in.LastTrue = now
		switch in.State {
		case Inactive:
			in.State, in.ActiveAt, ev = Pending, now, EvPending
			fallthrough // with For == 0 it goes straight on to firing
		case Pending:
			if now.Sub(in.ActiveAt) >= r.For {
				in.State, ev = Firing, EvFiring
			}
		}
		return ev
	}
	switch in.State {
	case Pending:
		in.State = Inactive // never fired: silent
	case Firing:
		if now.Sub(in.LastTrue) >= r.KeepFiringFor {
			in.State, ev = Inactive, EvResolved
		}
	}
	return ev
}
```

| Technique | Filters out | Cost |
|---|---|---|
| `for` (condition must hold) | one-sample blips | detection delayed by `for` |
| `keep_firing_for` | a metric that dips for one evaluation mid-incident | "resolved" delayed |
| Hysteresis: raise above 90%, clear below 80% | a value oscillating around one threshold | two numbers to explain |
| Notification-level `group_interval` and `repeat_interval` | repeated pages for one long incident | later updates |

### 9.3 Routing, grouping, silences, inhibition: 5,000 alerts, one page

A route tree matches on labels and ends at a receiver. Grouping collects alerts with the same `group_by` labels into **one notification**
with three timers; the seed's values are a good default:

| Timer | Seed value | Meaning |
|---|---|---|
| `group_wait` | 30 s | wait for the first siblings before the first notification |
| `group_interval` | 5 min | how long before notifying about *new* alerts joining an existing group |
| `repeat_interval` | 4 h (24 h for staging) | reminder while the group is unchanged; staging is deliberately quieter |

Worked storm: a shared dependency fails and **5,000 alert instances across 800 tenants** go pending within a minute. Without grouping: 5,000 notifications.
With `group_by: [tenant]` the notifier sends **800** (one per tenant, listing what is firing). And a platform-level alert "region down" **inhibits** the dependent
ones (*source matcher, target matcher, equal labels*: while `region_down{region=X}` fires, mute `*{region=X}`), so the platform on-call gets **1 page**.

- **Silences** match by labels for a time window and are checked *at notification time*. They mute delivery, not detection: the alert is still evaluated, recorded and visible in the UI as silenced.
- **Who is notified is resolved at send time, not frozen in the rule.** The seed keeps a subscription per person (`user_alerts`) and per-role defaults; a person's own choice beats the role's, a *Covered?* check decides whether the owner's list is needed as a fallback, and permission is re-checked at send time so a demoted account falls silent by itself.
- **Urgency has levels**: page, instant email, in-app only, daily digest (the seed's *off / instant / daily*). Repeats within a minute fold into one inbox row with a count rather than many rows.
- **Tenant isolation:** a tenant's route tree may reference only its own receivers; trees are validated on save and compiled to a lookup that is cached per tenant.

### 9.4 Delivering notifications reliably

The transition and the notification are written in **one transaction** (`alert_state` update + `alert_notifications` insert, the outbox of [10](10-webhook-delivery.md)),
so a crash cannot lose a firing alert or announce one that never happened. The notifier claims rows with `SKIP LOCKED`, applies grouping timers, silences and inhibition, and hands
the message to the notification service ([12](12-notification-service.md)). The `dedupe_key` is `group_key:version:kind`, so a retried send or a duplicated evaluator cannot produce two.

| Concern | Rule |
|---|---|
| Ordering | `firing` before `resolved` for a group: a per-group sequence number; a late `firing` is dropped if `resolved` already went |
| Failure | retry with backoff and jitter; after N failures use the receiver's *fallback* (another channel); alert the *tenant* that its receiver is broken |
| Reminders | a scheduled job per open group at `repeat_interval` ([15](15-job-scheduler.md)), cancelled on resolve |
| Acknowledgement | optional: an acked group stops reminding (not resolving); unacked pages escalate after a delay (extension) |
| "Resolved" | sent only for alerts that fired; grouped like firings so it is not 800 more messages |

### 9.5 Who watches the watcher

If the evaluator stops, *no* rule fires and the dashboard is green. Layers, cheapest first:

1. **Watchdog:** a rule that is *always* firing, routed every few minutes to an **external** dead man's switch ([08](08-uptime-monitor.md), 9.6). Silence for N minutes pages the platform team. This catches the evaluator, the notifier, the network and the receiver together.
2. **Self-metrics with their own alerts:** `eval_lag_seconds`, `missed_evaluations_total`, TSDB ingest freshness, and the **age of the oldest undelivered notification**: the seed's *"messages are stuck: oldest queued job older than 5 minutes"* rule, applied to our own outbox.
3. **An independent path:** the seed's Settings → System page reads the API's own checks, so it works when the monitoring stack is down; an outside uptime check covers the host itself (ADR 0011: "the one thing a panel on the droplet cannot report"). For us: a second small checker in another region reading `/internal/watchdog`.
4. **Alert quality is part of reliability:** every rule has an owner and a runbook link; track fires per week and the share that needed action. Pages that need nothing teach people to ignore pages.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the query load (16.7k evals/s, ~17M samples/s read). Recording rules, longer default intervals for low-priority rules, per-tenant budgets enforced harder, and a read replica of the TSDB reserved for rules.
- **Cardinality explosion:** one tenant adds a high-cardinality label; its rules touch millions of series. A per-tenant active-series cap and rule cost limits keep it from taking the TSDB down.
- **Evaluator dies:** its leases expire (≤ 30 s); another evaluator reloads instances from `alert_state` and carries on; no `for` timer is lost.
- **TSDB down or slow:** queries error; states are kept (no mass resolve, no mass fire), `missed_evaluations` rises, the engine alert fires through the watchdog path, which does not depend on the TSDB.
- **Notification provider down:** the outbox grows; retries, fallback receiver, and an alert on the age of the oldest pending row.
- **Notification storm:** grouping and inhibition bound it; a per-tenant and per-receiver rate cap is the last defence.
- **Postgres down:** evaluation continues in memory for a while; transitions queue locally and flush on recovery (bounded; beyond the bound, drop *reminders*, never *firing*).
- **Single points of failure:** none in the evaluator tier (leases), the notifier (stateless, row locks), but the watchdog's external receiver must itself be independent.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Scheduled queries on a TSDB | streaming evaluation | any query, simple, rules as data | read load; detection delay of interval + ingest lag |
| State written on transitions only | state every evaluation | ~0.6 writes/s instead of ~1,670 | `last_value` is slightly stale in the database |
| Persisted `active_at` | in-memory `for` timers | restarts do not delay alerts | a small write on entering *pending* |
| Grouping + inhibition at the notifier | notify per alert | a 5,000-alert storm becomes 800 messages and 1 page | later or merged messages; routing complexity |
| Outbox in the transition's transaction | fire-and-forget send | no lost alerts, no phantom alerts | at-least-once: dedupe keys needed |
| Errors keep state | errors resolve or fire | no flapping when the TSDB hiccups | an alert may stay firing while the data source is unreachable |
| External watchdog | internal health checks only | catches total failure | one more service to run in another region |

## 12. Check yourself

1. A rule has `for: 5m`. Describe pending vs firing, and what happens if the evaluator restarts at minute 4.
2. What are the three outcomes of an evaluation besides "condition true", and what does the seed's `noDataState: OK` risk?
3. 5,000 alert instances fire in a minute. Which mechanisms turn that into 800 notifications, and which into a single page?
4. 100,000 evaluations a minute: how many per second, how many evaluators, and what dominates the cost?
5. How does anyone find out that the alerting engine itself has stopped?

<details><summary>Answers</summary>

1. The first true evaluation makes the instance *pending* and records `active_at`; it becomes *firing* only when the condition has held for 5 minutes, and only then is anyone notified. A condition that clears during pending returns to inactive silently. If `active_at` is persisted, the new evaluator reloads it and fires at the original minute 5; if timers lived only in memory, the 5 minutes would restart and the page would be 4 minutes late.
2. *No data* (the query returned nothing), *an error* (the query failed), and *the condition false*. `noDataState: OK` is right for traffic-driven ratios but means a dead exporter or a deleted metric silences its alerts forever: pair it with an absence rule (`up == 0`, `absent()`) or a last-success-timestamp rule such as "no backup in 26 hours". An error should keep the previous state, not resolve it.
3. `group_by: [tenant]` collapses the instances to one notification per tenant: 800. An *inhibition* rule (a platform-level "region down" alert as the source, the dependent alerts as targets, matching on the `region` label) mutes the dependents, leaving the single platform page.
4. 100,000 ÷ 60 ≈ 1,670 per second. At ~800 evaluations/s per evaluator (5 ms per query, 4 in parallel) that is 3 evaluators, 6 with HA. The cost is the queries against the TSDB (~1.7M samples/s read, five times the ingest), not the state machine.
5. A watchdog alert that always fires is routed to an external dead man's switch; silence pages the platform team. Add self-metrics (evaluation lag, missed evaluations, age of the oldest undelivered notification) and an independent checker or status page (the seed's Settings → System and an outside uptime check) that does not depend on the monitoring stack.

</details>

## 13. In Go

- Related lessons: [39 Observability & tracing](../../39-observability-tracing.md) (metrics, labels, cardinality), [15 Sync & context](../../15-sync-context.md) (ticker-driven evaluation loops, `errgroup`, graceful shutdown), [36 Resilience](../../36-resilience-patterns.md) (backoff, timeouts, fallback receivers), [34 Event-driven & outbox](../../34-event-driven-outbox.md), [44 Background jobs](../../44-background-jobs-queues.md) (reminders), [58 Realtime](../../58-realtime-websockets-sse.md) (live alert views), [68 Rate limiting](../../68-rate-limiting-throttling.md) (per-tenant caps).
- Evaluator loop: one goroutine per shard, `time.NewTicker(tick)`, a bounded `errgroup.SetLimit(4)` for the evaluations due in this tick, `context.WithTimeout` per query, and `ctx.Done()` to hand leases back on shutdown. Fingerprint with `hash/fnv` over sorted `name=value` pairs (or `prometheus/common/model`'s `Fingerprint`).
- Packages: `prometheus/client_golang` (self-metrics), `prometheus/common/model` (label sets), `pgx`, `golang.org/x/sync/errgroup`. To embed an existing query engine rather than write one, look at the PromQL engine (`prometheus/prometheus/promql`) or a TSDB's HTTP query API.

## 14. Extensions

- **SLO burn-rate alerts:** multi-window, multi-burn-rate rules instead of raw thresholds; far fewer pages for the same user pain.
- **On-call schedules and escalation:** rotations, overrides, "ack within 10 minutes or page the next level" as scheduled jobs ([15](15-job-scheduler.md)).
- **Backtesting:** "what would this rule have fired in the last 7 days?" before saving it.
- **Rules as code:** the seed's "files in the repo, never clicked together": review, versioned rollout, and a diff of what changes in firing.
- **Anomaly detection** rules (seasonal baselines) alongside static thresholds; how to keep them explainable.
- **Auto-mute suggestions** for rules that fire often and are never acted on; a monthly alert-quality report per tenant.
- **Live inbox and push** for in-app alerts ([45](45-live-notifications.md)), with the seed's one-minute folding of repeats.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). The engine itself is small; the memory-hungry part is the TSDB it queries.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Evaluators (Go; leased shards, state machines in memory) | ~1,670 evals/s (100k/min); state ≈ 40 MB | TSDB query latency (concurrency); CPU is low | CPU: 1,670 × 0.3 ms (decode ~100 series, step the state machines) = 0.5 vCPU ⇒ N = ceil(0.5 ÷ 1.07) = 1 → 3 for zones. The real limit is waiting: 5 ms per query, 4 in flight = 800 evals/s each, so 1,670 ÷ 800 = 2.1 → 3, ×2 for HA = 6 (section 4). Memory: rules + state ≈ 150 MB | `c7g.large` (2 vCPU / 4 GiB) on ECS | 6, 2 per zone | the process mostly waits on the network, so many small nodes; `c` because RAM use is ~4 %. The 6 is the lesson's concurrency choice, not a CPU need: with 16 queries in flight per process, 3 nodes would carry it (≈ $160 less) |
| Notifier (Go; group timers, silences, inhibition, `SKIP LOCKED` outbox) | ~0.6 transitions/s; storm: 5,000 instances in a minute ≈ 83/s, 800 notifications | none (idle) | ~1 ms of route and silence matching per transition (assumption): 83 × 1 ms = 0.08 vCPU ⇒ N = 1 → 3 | `c7g.large` on ECS | 3, one per zone | a few small instances, for availability not load. Stateless (the outbox row and `dedupe_key` hold the truth), so a dead node costs seconds. Provider fees per message are the other cost and are outside the sheet |
| Postgres (`alert_state`, events, outbox, silences, leases) | 83 transitions/s in a storm, each = state + outbox + event row; whole data set < 20 GB | none; memory only for headroom | rows: 200k × ~300 B ≈ 60 MB; history ≈ 50k/day × 300 B ≈ 15 MB/day (5.5 GB a year). IOPS ≈ 2 per commit (assumption): 83 × 2 = 166, plus leases and notifier updates, say ≤ 500 vs gp3's 3,000 included | `db.m7g.large` (2 vCPU / 8 GiB), RDS Multi-AZ, gp3 100 GB | 1 primary + synchronous standby | everything fits in RAM many times over, so no `r`; `m.large` is the smallest production shape (`t` is not for sustained load). It is on the critical path of "no lost alert", hence Multi-AZ |
| TSDB the engine queries (out of scope in section 3, but this is where the load lands) | 10M active series; 333k samples/s in; ~1.7M samples/s read | memory (active series), then disk | assume ~4 KiB per active series: 10M × 4 KiB ≈ 38 GiB per copy, 3 copies, one full copy per node ⇒ 38 ÷ 64 = 60 % of RAM (rest for compaction and queries). CPU: ingest 333k × 3 ≈ 1M appends/s ≈ 1 vCPU, reads 1,670 × ~1 ms ≈ 1.7 vCPU = ~3 of 24 vCPU. Disk: 333k × ~1.5 B ≈ 43 GB/day per copy, assume 15 days ⇒ 650 GB | `r7g.2xlarge` (8 vCPU / 64 GiB) + gp3 650 GB each (Amazon Managed Service for Prometheus is the managed route, priced per sample: not priced here) | 3, one per zone | memory-bound, so `r` (8 GiB per vCPU; CPU sits near 12 %): `c7g.2xlarge` has 16 GiB and 38 GiB would not fit. Disk is sequential (WAL, compaction), so gp3 is enough |

Each evaluator queries the TSDB node in its own zone, so the 1,670 results/s cross no zone line; if they did, 1,670 × 10 KB (assumed) = 16.7 MB/s ≈ 44 TB a month, ⅔ of it crossing × $0.02/GB ≈ $590. The watchdog's external checker is near-idle: a `t`-family instance in another region (the one place `t` fits) or a hosted dead man's switch: a few dollars, left out.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Evaluators: 6 × `c7g.large` (6 × $53) | $320 |
| Notifier: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.m7g.large` ≈ $120, Multi-AZ doubles | $240 |
| gp3 100 GB × 2 (standby) × $0.08 | $20 |
| TSDB: 3 × `r7g.2xlarge` (3 × $310) | $930 |
| TSDB disk: 3 × 650 GB × $0.08 | $160 |
| ALB for the rules, silences and alerts API | $20 |
| **Total** | **≈ $1,850** (≈ $1,270 with a 1-year plan on the $1,650 of instances; storage and ALB not discounted) |

- **Biggest line and why:** the TSDB, ~59 % ($1,090 of $1,850), because 10M series live in RAM three times over; the engine proper (evaluators, notifier, Postgres) is only ≈ $740 for 5,000 tenants. The levers are on the TSDB side: recording rules (so ten rules do not each re-read the same series), the per-tenant active-series cap, and longer intervals for low-priority rules; a smaller evaluator saves nothing.
- **What changes at 10×:** 16.7k evals/s is only 16.7k × 0.3 ms = 5 vCPU, but concurrency: 16.7k ÷ 800 = 20.9 → 21 → 24 evaluators, ×2 for HA = 48, so raise in-flight queries to 16 per process (3,200 evals/s each: 16.7k ÷ 3,200 = 5.2 → 6, ×2 = 12 `c7g.large`, ≈ $640). The TSDB runs out first: reads need ~17 vCPU (16.7k × 1 ms) and, if tenants grow too (100M series = 381 GiB per copy, ×3 = 1,144 GiB ÷ (128 × 0.6) = 14.9), about 15 × `r7g.4xlarge` (5 per zone, 15 × $620 ≈ $9,300): shard it. Postgres: a storm becomes 830 transitions/s ≈ 1,700 IOPS, still inside gp3's 3,000; raise provisioned IOPS before the next 10×.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Two very different stores. The engine's own data is small and relational: 50k `alert_rules`, ~200k `alert_state` rows (~60 MB), an outbox, silences and leases; state is written on transitions only, ~0.6/s and ~83/s in a storm, and the whole set stays under 20 GB (sections 4, 8, 15).
- The metrics it queries are 10M active series, ~333k samples/s in, and 1,670 queries a second reading ~1.7M samples/s: pick series by label matchers, then aggregate the last few minutes. The rules are stored as `expr text` in that store's query language (sections 4, 8).
- Delivery needs one atomic step: the state change and its outbox row in one transaction, with a unique `dedupe_key` (9.4).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for rules, alert state, outbox and silences; ✖ for the 10M-series samples | the state update and the `alert_notifications` row commit together, `dedupe_key UNIQUE` and `SKIP LOCKED` claims, labels and matchers as `jsonb`. For samples: 333k inserts/s is ~33× the ~10k durable writes/s of one primary; at an assumed ~40 B per row with its index (a rule of thumb) 28.8B samples a day is ~1.2 TB a day against ~43 GB in a TSDB; and 1,670 queries × ~100 series = ~167k indexed range reads/s, ~17× the ~10k+ of a decent node |
| MySQL (RDS / Aurora) | ✔ for the rules database (nothing material), ✖ for samples for the same reasons | everything in section 8 maps over (below); a row store pays the same per-sample overhead, and neither engine speaks the language the rules are written in |
| Prometheus-style TSDB (self-managed on EC2; Amazon Managed Service for Prometheus) | ✅ chosen for the metrics | ~1.5 B per compressed sample (section 15), a label index, and the query language `expr` is written in; one full copy per node, ~38 GiB of active-series data each, held in RAM |
| ClickHouse / Timestream | ✔ works if the rule language becomes SQL | cheaper per stored sample over long retention, but `expr` would be SQL and each of the 1,670 evaluations/s must stay near the lesson's 5 ms; Timestream is not sized in section 15 |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.large`, gp3 100 GB) for the engine's record, and a Prometheus-style TSDB (3 × `r7g.2xlarge`, gp3 650 GB each, section 15) for the samples it reads. A time-series store is not Postgres at 10M series for three measured reasons: the write rate (~33× one primary), the bytes per sample (~27×) and a query that is "find series by labels, then aggregate" rather than a key lookup. Postgres is the system of record for alert state, history and outbox; the TSDB is an upstream input the engine only reads (out of scope in section 3). The rules database needs no Postgres-only feature, so choosing it is a team-skill call.

**If it were MySQL instead:** nothing material for the rules database. `jsonb` becomes `JSON` (matchers and labels are matched in the notifier's memory, never queried in SQL), `ON CONFLICT` becomes `ON DUPLICATE KEY UPDATE`, and the partial index `alert_state_live_idx` is dropped: it is an optimisation only (a rule has ~4 instances and the primary key `(rule_id, fingerprint)` already keeps them together; a restart reloads 60 MB). The outbox claim is a range scan with `SKIP LOCKED` (MySQL 8.0+), so run that session at `READ COMMITTED` to avoid gap-lock waits against inserts.

**What would flip the choice:** the rules database never, in this lesson: a storm at 10× is ~830 transitions/s ≈ 1,700 IOPS, inside gp3's 3,000 (section 15). The metrics store flips at 10× tenants: 100M series is ~381 GiB per copy and ~15 × `r7g.4xlarge` (section 15), the point to shard the TSDB or buy the managed service instead of running that fleet.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 167k evals/s (10M a minute), 5M rules, 500k tenants; 1B active series, 33M samples/s in, 170M samples/s read; 20M alert instances (4 GB of state); a storm moves 8,300 transitions/s | the TSDB's memory: 1B series × ~4 KiB = 3.7 TiB per copy (38 GiB today), ~150 `r7g.4xlarge` for three copies. Next in line: the storm's 8,300 commits/s is 83 % of one primary's ~10k writes/s and ~16.6k IOPS, over gp3's 16k cap | scale out by key: shard the TSDB by `tenant_id` (a rule's query hits one shard: ~50 shards × 3 copies), evaluators next to their tenant's shard (167k ÷ 3,200 = 53, ~106 with HA), Postgres sharded by `tenant_id` (8 shards, ~1k transitions/s each), recording rules and the per-tenant series cap. Give up: cross-tenant queries (the platform-wide "region down" inhibition moves to a small global tier) and easy resharding | ≈ $185k a month, ~60 % of it TSDB RAM |
| **1,000×** | 1.67M evals/s (100M a minute), 50M rules, 5M tenants; 10B series, 333M samples/s in, 1.7B samples/s read; 200M instances (40 GB); a storm moves 83k transitions/s | operating one fleet: 10B series ≈ 37 TiB per copy ≈ 500 `r7g.4xlarge`, so ~1,500 TSDB nodes plus ~1,000 evaluators: a node dies about every 2 h (5,000 nodes ≈ 1 h), and 83k transitions/s is the whole ceiling of those 8 Postgres shards (8 × ~10k). A reshard or a lease storm is now a global incident | independent **cells**: ~100 cells, each section 15's 10× design (100M series, 16.7k evals/s, ~15 `r7g.4xlarge`, 12 evaluators, its own Postgres), behind a `tenant_id → cell` router at ingest and API; a tenant never spans cells; a cell is the unit of failure, deploy and watchdog and keeps evaluating with the router down; a tiny global feed carries platform alerts to each cell's inhibition. Give up: cross-cell queries and one global dashboard (fan-out reads); moving a tenant is a migration | ≈ $1.85M a month, ~60 % TSDB RAM |
| **10,000×** | 16.7M evals/s (1B a minute), 500M rules, 50M tenants; 100B series (12 per person alive: a thought experiment), 3.3B samples/s in, 17B samples/s read; 2B instances (400 GB); a storm moves 830k transitions/s | money and people: 100B series ≈ 370 TiB per copy ≈ 5,000 `r7g.4xlarge`, 15,000 for three copies ≈ $9.3M of RAM; ~20M alerts firing at 1 % is more than anyone can read | change the problem: evaluate at the source: the collector evaluates simple thresholds and sends transitions, not samples (streaming evaluation, the alternative in 9.1); recording rules computed at ingest; a hard series cap per tenant (2,000 on average); sketches or sampling for high-cardinality series; 30 s evaluation for paid tiers, 5 min for free (10× fewer); page limits per tenant. Give up: arbitrary queries at the source (thresholds and rates only), detection latency for low tiers, exact aggregates over sampled series | ≈ $18.5M a month if nothing changes |

**What stays the same at every scale:** state written on transitions only with `active_at` persisted (a restart never resets a `for` timer), identity by label fingerprint, the outbox row in the transition's transaction with a unique `dedupe_key`, and the external watchdog.

**Say it in an interview:** "At 100× one TSDB copy is 3.7 TiB of series memory, about 150 nodes for three copies, so I shard the TSDB and Postgres by tenant and pin evaluators next to their shard. At 1,000× it is thousands of nodes and a storm that fills the Postgres shards, so I run ~100 independent cells behind a tenant router. At 10,000× there are 100B series and 17M evaluations a second, so I stop querying everything: simple rules run at the collector, series are capped, and free tiers evaluate less often."
