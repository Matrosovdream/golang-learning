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
