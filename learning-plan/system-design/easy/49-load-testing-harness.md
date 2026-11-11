# 49 · Load Testing Harness

`🟢 easy` · *Topics:* open vs closed workload models, coordinated omission, mergeable latency histograms, realistic data volume and scenario mix, a generator fleet that is not the bottleneck, server-side observation, regression gates · *Builds on:* [03 Rate Limiter](03-rate-limiter.md) (the limits a test runs into), [08 Uptime Monitor](08-uptime-monitor.md) (probing and verdicts), [15 Job Scheduler](15-job-scheduler.md) (nightly runs)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [48 Bulk Import with Preview & Undo](48-bulk-import.md) · Next: [50 Alerting Engine](50-alerting-engine.md)

---

> 🌱 **Seed (honeydolist-site):** load testing has two layers. **k6 scripts** (`infra/load/k6/`: `public.js` for visitors on a
> `ramping-arrival-rate`, `admin.js` for staff as `ramping-vus`, one virtual user per person, plus `forms.js` and `combined.js`)
> and a **Go driver, `cmd/loadtest`, on vegeta** with scenarios (browse, dashboard, leads, login, webhooks, media, writes, spike,
> soak, queue), each with a *budget*. The package comment states the model: requests "sent at a set rate, an open model, so requests
> arrive whether or not the last one answered and a stalled server shows in the percentiles instead of slowing the sender".
> `make load` (PLAN step 8b) runs the **production image under production's container limits** (`compose.load.yml`, `pg_stat_statements`
> on, statements over 50 ms logged with their plan, a pprof listener) on **generated volume** (20k clients, 10k quotes, 10k rental requests, 10k orders; step 56 measured
> 200k machines, 4.7M bookings, 800k orders). The report holds latencies per scenario, the ten costliest SQL statements, each
> container's peak memory against its limit, restarts and OOM kills, CPU and heap profiles. `loadtest compare` is the nightly gate: p95 above 1.5× the median of earlier nights *and* more than 10 ms is a regression. The driver and the generator both refuse a production target. Findings: photos served through Go (p95 11.7 s → 10 ms after the fix) and password hashing killing the API.
> **Here, at scale:** an internal platform running 500 tests a day for hundreds of teams, each up to 100k virtual users / 200k req/s from a generator fleet.

## 1. The task

> "Design a load testing platform. A team describes traffic against its service, the platform runs it from many
> machines, and returns a report it can trust: latencies, errors, what the server was doing, and pass or fail."

The capacity is the easy half. The hard half is that **a load test is a measuring instrument**, and the common
ones lie: they slow down when the server slows down, average things that cannot be averaged, and test an empty database.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Protocols? | Generator design, connection reuse. | HTTP/1.1 and HTTP/2 first; gRPC and WebSocket are extensions. |
| "N users" or "N requests per second"? | Closed vs open model. | Both exist; default to **open** (rate) because production traffic does not wait for your slowest request. |
| What is the target? | Safety and realism. | An isolated, production-shaped stack. Shared staging by approval. Production never. |
| What is "pass"? | Budgets and gates. | Per scenario: error rate, p95/p99, no restarts or OOM kills, no rate-limit refusals. |
| What data is behind the target? | Query plans change with volume. | Synthetic data at production volume and skew, restored from a snapshot before each run. |
| Who runs tests, how often? | Queueing and quotas. | Teams on demand, a PR smoke test, a nightly full run. |
| Scale? | Fleet size. | 500 runs/day, median 15 min, longest 4 h soak, largest 200k req/s and 100k VUs. |
| Retention? | Storage. | 10-second histograms 30 days; per-run summaries kept. |

## 3. Requirements

**Functional**
- Define a test: scenarios with weights and think time, load model (open rate or closed users), ramp/hold, budgets.
- Run it from N generators synchronised on a start time; abort on demand or on safety rules.
- Live view and a final report: percentiles per scenario and endpoint, errors by kind, achieved vs requested rate, server-side observations.
- Compare with earlier runs and gate a build on regressions.

**Non-functional**
- **Measurement honesty:** latency is measured from the intended send time; a saturated generator invalidates the run instead of flattering the server.
- Percentiles are correct across workers (merge histograms, never average percentiles).
- A test cannot be pointed at production or at a host its team does not own; it cannot be used as an attack tool.

**Out of scope**
- Browser-level (real rendering) tests, distributed tracing of the target, chaos injection.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Test minutes per day | 7,500 → ~5 runs at once on average, ~25 at peak | 500 × 15 min = 7,500 ÷ 1,440 min = 5.2; ×5 for the working-day peak |
| Biggest test: virtual users | 100k VUs | Little's law N = λ·(W+Z) = 200,000 × (0.1 s latency + 0.4 s think) |
| Biggest test: requests in flight | 20k | λ·W = 200,000 × 0.1 s |
| Generator workers for 200k req/s | 12 | assume one 8-vCPU worker sustains 25k simple req/s flat out; plan at 70% = 17.5k; 200k ÷ 17.5k = 11.4 |
| Response bandwidth | 3.2 Gbit/s total, ~270 Mbit/s per worker | 2 KB × 200k = 400 MB/s × 8 bits; ÷ 12 |
| Connections per worker | ~8.3k | 100k ÷ 12; default ephemeral range ≈ 28k ports per (source IP, target IP, port) |
| Raw samples, 15 min at 200k/s | 2.9 GB | 200k × 900 s × 16 B (timestamp, latency, status, endpoint) |
| Histograms, same test | ~14 MB | 50 endpoints × ~3 KB per 10 s window × 90 windows; about 200× smaller than raw |
| Stored per day | ~7 GB; ~200 GB for 30 days | 14 MB × 500 runs |
| Worker → coordinator traffic | ~180 KB/s | 12 workers × 50 endpoints × 3 KB every 10 s |

What the numbers tell us: capacity is a fleet-sizing exercise (a dozen workers, a few Gbit/s). **Metric volume is tiny
if you ship histograms, not samples.** All the difficulty is correctness: how latency is timed, how workers' results
combine, and whether the generator was itself keeping up. The seed's README says it plainly: laptop numbers compare a change with the last run; only production-shaped hardware says what the site can take.

## 5. Plan

1. Pick the workload model (open vs closed) and time requests from intended start.
2. Realistic scenarios and data: volume, skew, think time, identities.
3. The generator fleet: coordinator, workers, start barrier, health.
4. Metrics: histograms per window, merged centrally; observe the *server* from its own side.
5. The report, budgets, and the nightly regression gate.
6. Safety: targets, approvals, abort rules, cleanup.

## 6. High-level design

```
 UI / CI ─► Control API ─► Postgres (tests, targets, windows, baselines)
               │  ▲
               ▼  │ live + final results
          Coordinator ──── plan + start time T0 ───► Workers (ephemeral, same region as the target)
               ▲                                        │ open-model arrival loops, HTTP connection pools
               │  every 10 s: HDR histogram + counters  │
               └────────────────────────────────────────┘
                                                        ▼
        Target: isolated production-shaped stack  ◄── Observer: pg_stat_statements, container memory/restarts,
        (production image, production limits,             CPU + heap profiles, queue depth (from the target's side)
         seeded to production volume)
```

A run is: reserve the target (one run per target at a time) → restore the dataset snapshot → workers start at T0 →
ramp, hold, wind down → coordinator merges windows → observer data is attached → report and gate.

## 7. API

```
POST /v1/targets { name, base_url, kind: "isolated"|"shared_stage", max_rps }   → 201      (production cannot be registered)
POST /v1/tests { target_id, scenario_ref, model: "open", load: { rate: 20000, ramp: "2m", hold: "10m" },
                 budgets: { error_rate: 0.01, p95_ms: { "GET /api/items": 500 } } } → 201 { id }     over max_rps: needs approval
POST /v1/tests/{id}/start | /abort                                              → 202
GET  /v1/tests/{id}                      → 200 { status, valid, achieved_rate, live: { p50, p95, p99, errors } }
GET  /v1/tests/{id}/series?endpoint=…&metric=p99                                → 200 per-window series
GET  /v1/tests/{id}/report               → 200 JSON (and ?format=md): percentiles, budgets, server-side, verdict
GET  /v1/baselines?target=…&scenario=…                                          → 200 last N nights per endpoint
```

## 8. Data model

```sql
CREATE TABLE load_targets (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, team_id bigint NOT NULL, name text NOT NULL,
  base_url text NOT NULL, kind text NOT NULL CHECK (kind IN ('isolated','shared_stage')),
  max_rps integer NOT NULL, verified_at timestamptz          -- ownership proof: a token served from /.well-known/
);
CREATE TABLE load_tests (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, target_id bigint NOT NULL REFERENCES load_targets(id),
  scenario_ref text NOT NULL,                                  -- git sha of the scenario code + dataset version
  model text NOT NULL CHECK (model IN ('open','closed')), load jsonb NOT NULL, budgets jsonb NOT NULL,
  status text NOT NULL DEFAULT 'queued'
         CHECK (status IN ('queued','preparing','running','finishing','done','aborted','failed')),
  valid boolean, invalid_reason text,                          -- a saturated generator makes a run INVALID, not "slow"
  approved_by bigint, started_at timestamptz, finished_at timestamptz
);
CREATE UNIQUE INDEX load_one_active_per_target ON load_tests (target_id) WHERE status IN ('preparing','running','finishing');
CREATE TABLE load_windows (                                    -- merged across workers; hist = serialised HDR histogram
  test_id bigint NOT NULL REFERENCES load_tests(id) ON DELETE CASCADE, window_start timestamptz NOT NULL,
  scenario text NOT NULL, endpoint text NOT NULL, count bigint NOT NULL, errors bigint NOT NULL,
  status_counts jsonb NOT NULL, hist bytea NOT NULL, max_lag_us integer NOT NULL,
  PRIMARY KEY (test_id, window_start, scenario, endpoint)
);
CREATE TABLE load_statements (test_id bigint NOT NULL, query text NOT NULL, calls bigint, total_ms double precision,
                              mean_ms double precision, max_ms double precision, PRIMARY KEY (test_id, query));
CREATE TABLE load_baselines (target_id bigint NOT NULL, scenario text NOT NULL, endpoint text NOT NULL,
                             night date NOT NULL, p95_us bigint NOT NULL, PRIMARY KEY (target_id, scenario, endpoint, night));
```

## 9. Deep dives

### 9.1 Open vs closed, and coordinated omission

| Model | How it behaves | Pros | Cons |
|---|---|---|---|
| **Closed loop** (N virtual users, each waits for its reply, thinks, repeats) | offered load *falls* when the server slows | models a fixed pool of users or a connection-limited client | hides stalls: a frozen server stops the senders, so few slow samples are recorded |
| **Open loop** (arrivals at a rate, independent of replies) | offered load stays constant | models the public internet; a stall appears in the percentiles | needs a cap on in-flight requests; the generator must keep up |
| Open, Poisson gaps | arrivals are random (exponential gaps), mean rate fixed | realistic bursts; no lock-step spikes | slightly harder to reason about |
| Hybrid: arrival-rate executor with a pool of VUs (k6's `arrival-rate`) | open arrivals running scripted journeys | scripted flows at a fixed rate | the VU pool must be big enough, or arrivals are dropped |

The classic failure is **coordinated omission**: the generator times a request from when it *actually* sent it, and a
slow reply delays the next send, so the test quietly stops asking while the server is at its worst. Worked example: 1,000
req/s for 100 s, the server freezes for 10 s.

- *Open loop, timed from intended send:* 10,000 requests were due during the freeze and each waits until it ends (0-10 s, average 5 s). That is 10% of 100,000 requests: **p95 ≈ 5 s, p99 ≈ 9 s**.
- *Closed loop, 100 VUs (100 = 1,000/s × 0.1 s):* only the ~100 in-flight requests are slow, out of ~90,000 recorded: 0.1%. **p99 still looks like 100 ms.**

**Choice and why.** Open loop with Poisson arrivals, **latency measured from the intended start time**, a hard cap on
in-flight requests (hitting it is *recorded*, not hidden), and a `lag` metric per loop (how far behind schedule it ran). Closed loop stays available for "100 staff
using a dashboard" tests, as the seed's `admin.js` does.

```go
// openLoop sends at ~rate req/s with exponential gaps and times each request from when it SHOULD have started.
func openLoop(ctx context.Context, rate float64, send func(context.Context) error,
	rec func(lat time.Duration, err error), lagged *atomic.Int64) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	inflight := make(chan struct{}, 20_000) // safety valve: hitting it is recorded, never hidden
	next := time.Now()
	for ctx.Err() == nil {
		next = next.Add(time.Duration(rng.ExpFloat64() / rate * float64(time.Second)))
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		} else if -d > 5*time.Millisecond {
			lagged.Add(1) // behind schedule: the generator, not the server, is the bottleneck
		}
		intended := next // if we are late, the lateness counts as latency
		select {
		case inflight <- struct{}{}:
			go func() { defer func() { <-inflight }(); err := send(ctx); rec(time.Since(intended), err) }()
		default:
			rec(0, errDropped)
		}
	}
}
```

`time.Sleep` jitters by tens to hundreds of microseconds, so run one loop per few thousand req/s, each at `rate/n`. Store
latencies in an **HDR-style histogram** (fixed relative error, e.g. 3 significant digits, mergeable by adding bucket counts).
Percentiles come from the *merged* histogram; **averaging workers' p99s is meaningless** (9.3).

### 9.2 Realistic data, scenarios and identities

A fast test on a tiny database proves nothing. The seed's step 56 measured at 200k machines, 4.7M bookings and 800k
orders and found five reads that did not scale (an orders list at 1.0-2.3 s, a fleet list at 1.7-2.0 s, a client's rentals at
1.6 s); each looked fine on the 20k-row development data.

| Data source | Pros | Cons |
|---|---|---|
| Tiny fixtures | fast to build | misses every volume-dependent plan |
| Masked copy of production | the real shape | privacy risk; masking is hard to do right; stale |
| **Synthetic generator at production volume and skew, restored from a snapshot per run** | repeatable, no personal data, scalable | the skew must be modelled |

**Choice and why.** The generator (the seed's `generate`/`clean` and its ten-year histories) creates years of history, with *Zipf-like* access
(a few hot items, a long tail; uniformly random ids make caches look better or worse than reality) and a snapshot restored
before each run, so one run's writes do not change the next run's table sizes. Other rules:

- **Scenario mix by weight** from production analytics (e.g. 70% browse, 20% search, 8% checkout, 2% login), with **think time** per step. Without it, 100k VUs hammer one endpoint.
- **Identity per VU:** each staff VU is one signed-in person, each visitor a fresh cookie jar and cart. Shared credentials hit per-account rate limits first.
- **Client addresses:** limits keyed by address make one machine look like one visitor (the seed's README: a laptop is a single address). Its driver sets `X-Real-IP` itself because *the target is a test stack that trusts that header*; the alternative is raising the limit for the run and putting it back. Never by weakening production code paths.
- **Warm-up and steady state:** ramp, then measure a window after caches and pools fill; report ramp separately.
- **Tag test data** (`k6-…@gen.example.test`) so cleanup is one `DELETE … WHERE email LIKE`, as the seed's README does after the forms test.

### 9.3 The generator fleet

| Option | Pros | Cons |
|---|---|---|
| One big machine | simplest | NIC, ports and CPU cap it; a single failure kills the run |
| Fixed fleet of long-lived workers | warm, cheap to start | idle cost; noisy neighbours between teams |
| **Ephemeral workers per run (containers/VMs in the target's region)** | isolation, elastic, no idle cost | start-up time (~1 min); image and credentials plumbing |

The **coordinator** splits the rate (each worker gets `rate/N` and a slice of the VU pool), fixes a start time T0 a
minute ahead, and aligns 10-second windows to T0. Workers measure latency with the **monotonic clock** (never wall-clock differences) and use
wall time only to label windows, so NTP drift of a few ms moves a window edge, not a latency. Each worker pushes `(test, worker, window)`
histograms, **idempotent by that key**; the coordinator merges by adding buckets. A worker that goes silent is detected from missing windows: redistribute its
rate and mark the run *degraded*, or fail it; never silently under-load.

**Is the generator the bottleneck?** Per worker, report CPU (keep below ~70%), the arrival-loop `lag` (p99 above ~5 ms means it cannot keep schedule), the in-flight
cap hits, and connect/timeout error counts. Any of them over threshold sets `valid = false` with a reason. Mechanics that bite: file descriptors (`ulimit -n`), ephemeral ports
(28k per tuple: add target IPs or source IPs), cloud NAT port exhaustion, TLS handshakes (reuse connections and sessions), DNS (resolve once per worker and spread
across load balancer IPs), HTTP/2 multiplexing (changes the load shape: say which one the test uses). Run workers **in the target's region**: from a laptop on another continent every request also pays ~200-250 ms, as the seed warns.

### 9.4 The report, the server's side, and the regression gate

The driver's view is the *client's* view. The seed observes the *server* from its own side during each scenario:

- `pg_stat_statements` reset before, the **top ten statements by total time** after (calls, mean, max): this is how "a dropped index names its query".
- Each container's **peak memory against its limit**, restarts, and **OOM kills**, counted separately because "a peak the scrape cannot see, since it ends in the kill".
- A **CPU profile during** and a **heap profile after** the scenario.
- Reading rules, like the seed's table: *p95 rising during the hold with the achieved rate flat* means the server is slowing under steady load; *achieved below requested* means the generator or the target saturated; *429s* mean the limit stopped the test, not the server.

| Gate | Pros | Cons |
|---|---|---|
| Fixed absolute thresholds | simple, matches an SLO | drift goes unseen until the line is crossed; hardware changes break it |
| Compare with the last run | catches drift | one noisy run flags or hides a regression |
| **Compare with the median of the last ≥3 runs, ratio *and* absolute floor** | robust to noise; ignores tiny absolute changes | needs history; slow to adapt after an intended change |
| Statistical test (bootstrap CI, Mann-Whitney) | principled | harder to explain; needs raw or finer data |

**Choice and why.** The seed's rule: per (scenario, endpoint), tonight's p95 is a regression if it exceeds **1.5× the
median of earlier nights and the median by more than 10 ms**, and it says nothing until three nights exist. The ratio ignores scale, the
floor ignores 2 ms → 4 ms. Two pipelines: a **PR smoke** (short, strict on errors and budgets, loose on latency) and the
**nightly** (full length, strict on latency). A gate that flakes gets bypassed: track its false-positive rate and re-run a failing run once before blocking.

### 9.5 Safety: production, neighbours, abuse

- **Never production.** The seed's driver refuses a production target and its data generator refuses production too (protections such as a CDN's may also challenge or block the test). Production is simply not registrable as a target.
- **A target is registered and verified:** the owner proves control (a token served from `/.well-known/`, or a DNS TXT record), otherwise the platform is an attack tool. `max_rps` per target; above it needs an approval.
- **One run per target at a time** (the partial unique index): the seed's note that a full-gas run on a shared stage "slows production too" is the noisy-neighbour problem; shared targets get windows and a quota.
- **Abort rules:** stop on error rate above 50% for 30 s, or when the target's health check fails, or a human presses abort. A test that only damages teaches nothing.
- **Cleanup:** test data tagged and removed; test accounts deactivated at the end (the seed's `admin.js` creates load-test managers and deactivates them); secrets for test accounts live in a vault, not the scenario code.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the generator fleet's network and ports (2M req/s = 32 Gbit/s of 2 KB responses, 120 workers): shard across availability zones and give workers many source IPs. Next, the coordinator's merge if windows shrink to 1 s; merge in a tree (regional coordinators).
- **Worker dies:** missing windows detected within one interval; redistribute or mark degraded.
- **Coordinator dies:** workers keep to the plan (it is immutable) and buffer windows; a new coordinator resumes from stored windows (idempotent pushes).
- **Target collapses:** abort rules stop the run; the report keeps everything up to the abort, labelled.
- **Clock skew:** harmless for latencies (monotonic); a window edge may shift by milliseconds.
- **Observer cannot read the target** (Prometheus down): the report says so (the seed's `Notes`), and the run is flagged "client-side only".
- **Noisy results:** repeat runs, compare medians, record the environment (instance type, dataset version, image sha) so differences are explained.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Open loop, timed from intended start | closed loop with simple timing | stalls are visible; honest percentiles | in-flight caps and lag tracking; harder generators |
| Mergeable histograms | averaged percentiles / raw samples | correct percentiles, ~200× less data | fixed relative error; a bucket scheme to maintain |
| Production-shaped data + snapshot restore | small fixtures | volume-dependent bugs found; repeatable runs | storage, restore time, a generator to maintain |
| Ephemeral workers in the target's region | one big machine | elastic scale; clean network path | ~1 min start-up; more plumbing |
| Median-of-nights gate with ratio and floor | fixed thresholds | catches drift, tolerates noise | needs history |
| `valid = false` on generator saturation | reporting whatever came out | no flattering numbers | some runs are discarded |

## 12. Check yourself

1. A server freezes for 10 s during a 1,000 req/s test. What do p95 and p99 show under an open model timed from intended send, and under a 100-VU closed loop?
2. How many virtual users does a 200k req/s test need with 0.1 s latency and 0.4 s think time, and how many requests are in flight?
3. Twelve workers each report their p99. Why can you not average them, and what do you do instead?
4. How do you know the generator, not the server, is the bottleneck?
5. Tonight's p95 for an endpoint is 18 ms; the median of the last five nights is 11 ms. Regression or not, and why that rule?

<details><summary>Answers</summary>

1. Open: 10,000 requests (10% of 100,000) were due during the freeze and each waited until it ended, 0-10 s, so p95 ≈ 5 s and p99 ≈ 9 s. Closed with 100 VUs: only the ~100 in-flight requests are slow, about 0.1% of the ~90,000 recorded, so p99 stays near the normal 100 ms. The closed loop *omitted* the requests that should have been sent during the stall.
2. Little's law: VUs = λ × (W + Z) = 200,000 × 0.5 s = 100,000. In flight = λ × W = 200,000 × 0.1 s = 20,000.
3. A percentile of the combined data is not a function of the parts' percentiles: a worker with 1,000 requests and p99 100 ms and one with 9,000 requests and p99 1 s have no meaningful mean of 550 ms. Merge the workers' histograms (add bucket counts, they use the same bucket scheme) and read the percentile from the merged histogram.
4. Per worker: CPU above ~70%, the arrival loop's lag (its sends happen late against the schedule), the in-flight cap being hit, connect/timeout errors, and *achieved rate below requested* while the server's own metrics are idle. Any of these marks the run invalid with a reason.
5. 18 ms > 1.5 × 11 ms = 16.5 ms *and* 18 − 11 = 7 ms is below the 10 ms floor, so **not** a regression by the seed's rule: both conditions must hold. The floor stops tiny absolute changes from failing builds; the ratio makes the rule scale with the endpoint; the median of several nights stops one noisy night from setting the bar.

</details>

## 13. In Go

- Related lessons: [46 Low-latency measuring](../../46-low-latency-measuring.md) (timers, percentiles, measuring honestly), [39 Observability & tracing](../../39-observability-tracing.md) (the metrics the observer reads), [15 Sync & context](../../15-sync-context.md) (arrival loops, cancellation), [63 Networking fundamentals](../../63-networking-fundamentals.md) and [64 HTTP internals](../../64-http-protocol-internals.md) (ports, connection reuse, HTTP/2), [68 Rate limiting](../../68-rate-limiting-throttling.md) (the limits you hit), [62 Deployment & operations](../../62-deployment-operations.md) (production-shaped stacks).
- Tune `http.Transport` for a generator: `MaxIdleConnsPerHost` high, `DisableCompression` if you are not testing it, one transport per worker, `ForceAttemptHTTP2` explicitly chosen.
- Packages: `github.com/HdrHistogram/hdrhistogram-go` (mergeable histograms), `github.com/tsenart/vegeta/v12/lib` (an open-model attacker, the seed's driver), `pgx` (reading `pg_stat_statements`), `net/http/pprof`, `golang.org/x/sync/errgroup`. Outside Go: k6's `constant-arrival-rate` / `ramping-arrival-rate` executors are open model, and wrk2 corrects for coordinated omission.

## 14. Extensions

- **Real-user journeys with a browser** for the few flows where JavaScript matters; keep them at a few dozen VUs.
- **Soak and spike shapes** (the seed's scenarios): memory-leak detection over hours, and a step from 1× to 10× to test autoscaling and queues.
- **Trace-linked results:** attach exemplars (trace ids of the slowest requests) to each window so "p99 rose" links to a trace.
- **Capacity model:** fit throughput vs latency across rates to find the knee, and report "headroom at today's peak".
- **Chaos during load:** kill a replica, add latency to a dependency, and record the recovery curve.
- **Replay production traffic** (sampled, scrubbed) as a scenario instead of hand-written journeys.
- **Cost and quota accounting** per team; a fair queue when ten teams want the same shared stage.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). The interesting hardware here is the **generator fleet itself**; the target stack under test belongs to the team that owns it and is not in this bill.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Load generator workers (Go, open-loop arrival loops), one ephemeral fleet per run | largest run: 200k req/s, 100k VUs, 20k in flight, 3.2 Gbit/s of responses | CPU, then NIC and ephemeral ports | section 4's worker: 8 vCPU ÷ 25k req/s = 0.32 ms per request; 200k × 0.32 ms = 64 vCPU flat out. Size to 70 % (the lesson's own "generator saturated, run invalid" line; the 80 % × ⅔ zone rule is for serving users, a run is disposable): N = ceil(64 ÷ (0.7 × 8 = 5.6)) = ceil(11.4) = 12. Memory: 8.3k connections × ~10 KiB + 1.7k in-flight goroutines (20k ÷ 12) × 8 KiB ≈ 0.1 GiB. Network: 267 Mbit/s responses + ~67 Mbit/s requests (0.5 KB each, assumed) | `c7g.2xlarge` (8 vCPU / 16 GiB), started per run (ECS on Fargate in the same shape is the managed route: dearer per vCPU, not priced here) | 12 for the biggest run, 4 per zone; ~39 at the daily peak | CPU decides the count and memory uses 1 % of the 16 GiB, so `c` (2 GiB per vCPU), not `m` or `r`. Graviton: Go cross-compiles, a vCPU is a whole core (the 25k figure is a Graviton figure: re-measure on x86). Not `t`: its credits run out mid-run and a throttled generator silently becomes the bottleneck. Not `c7gn`: 12 nodes at ~0.33 Gbit/s are nowhere near a NIC limit. 8 vCPU keeps a lost worker to 1/12 of the rate |
| Control API + coordinator (Go; start barrier, histogram merge) | ~600 KB/s of histograms with ~25 runs live (180 KB/s for the largest run alone) | none (idle); availability | merging adds ~3 KB of bucket counts per worker per 10 s: assume 0.1 vCPU in total, N = ceil(0.1 ÷ 1.07) = 1, rounded up to 3 for zones | `c7g.large` (2 vCPU / 4 GiB) on ECS | 3, one per zone | a few small instances, for availability not load. Restartable by design (workers keep to the plan and buffer windows, pushes are idempotent), so no state lives on the box. The observer (target's `pg_stat_statements`, container memory, pprof) runs inside it |
| Postgres (tests, `load_windows`, baselines) | ~125 window rows/s (25 runs × 50 endpoints ÷ 10 s); 7 GB/day, ~200 GB at 30 days | memory (hot set), then storage | hot set = the last day of windows ≈ 7 GB + indexes ≈ 8 GB: fits in 16 GiB (`shared_buffers` 4 GiB + OS cache). IOPS ≈ 2 per commit (assumption); even if every row committed alone: 125 × 2 = 250, under gp3's 3,000 included. CPU ≈ 0.1 vCPU | `db.r7g.large` (2 vCPU / 16 GiB), RDS Multi-AZ, gp3 300 GB (200 GB data + index and vacuum headroom) | 1 primary + synchronous standby | the cheapest shape that keeps the hot set in RAM: `r.large` is 16 GiB for $78, `m.xlarge` 16 GiB for $120, and we need no CPU. Partition by day so 30-day retention is `DROP PARTITION`, not a huge `DELETE` |

**Fleet and mix** (assumption: the lesson gives only the largest run and the 15 min median). Per day: 400 small runs at 5k req/s (1 worker), 90 medium at 50k (50k × 0.32 ms = 16 vCPU ÷ 5.6 = 2.9, so 3 workers), 9 large at 200k (12), one 4 h soak at 20k (2); each billed 15 min plus ~2 min start-up and wind-down. Worker-minutes: 400 × 17 + 270 × 17 + 108 × 17 + 2 × 242 = 13,710 ≈ 228 worker-hours a day. Peak: 25 runs × 1.56 workers (780 ÷ 500) ≈ 39 workers ≈ 312 vCPU, so request a ~400 vCPU EC2 quota (room for a second large run).

**Traffic is the real cost of a generator.** Replies into EC2 are free, but a request and reply that cross a zone pay $0.01 per GB each way. At 2.5 KB per round trip a large run moves 500 MB/s × 900 s = 450 GB; ⅔ of it crosses a zone when workers and the target's load balancer both span 3 zones: 300 GB × $0.02 = $6, against $0.99 of workers. Workers belong in the target's VPC on private IPs: behind a NAT gateway the same run costs 450 GB × $0.045 = $20.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Workers: 228.5 h/day × 30.4 days = 6,950 h × `c7g.2xlarge` $0.29/h | $2,000 |
| Worker ↔ target traffic across zones: 19.4 TB/day × ⅔ × $0.02/GB | $7,900 |
| Control API + coordinator: 3 × `c7g.large` (3 × $53) | $160 |
| Postgres: `db.r7g.large` ≈ $156, Multi-AZ doubles | $310 |
| gp3 300 GB × 2 (standby) × $0.08 | $50 |
| Histograms to the coordinator: 210 GB × $0.09 (worst case, cross-region) | $20 |
| ALB for the API and UI | $20 |
| **Total** | **≈ $10,500** (≈ $10,300 with a 1-year commitment on the control plane; the fleet is bursty, so it goes on Spot, not a plan) |

- **Cost per run (workers + traffic):** small ≈ $0.23 ($0.08 + $0.15), medium ≈ $1.75 ($0.25 + $1.50), a 200k req/s run ≈ $7 ($0.99 + $6.00), the 4 h soak ≈ $12 ($2.34 + $9.60); average ≈ $0.65 over 500 runs ($325 a day).
- **Biggest line and why:** cross-zone traffic, ~75 % ($7,900 of $10,460): a generator's whole job is to move bytes. The lever is topology, not instance size: each worker uses the load-balancer address in its own zone (9.3 already says to spread over addresses: do it within the zone first), and if that line all disappears the total is ≈ $2,600. Spot on the 490 small and medium runs (83 % of worker-hours) then takes ≈ $1,000 more off at ~60 % off; keep the nightly gate and the soak on-demand, because a reclaimed worker makes a degraded run and a degraded run proves nothing.
- **What changes at 10×:** the largest run, 2M req/s (section 10): 2M × 0.32 ms = 640 vCPU ⇒ N = ceil(640 ÷ 5.6) = ceil(114.3) = 115 → 117 × `c7g.2xlarge` (936 vCPU, close to section 10's 120), 32 Gbit/s in total but still ~0.33 Gbit/s per worker. What runs out first is the account's vCPU quota (936 vCPU for one run: ask ahead) and ports, NAT and load-balancer addresses, not the instances; one such run costs 117 × $0.08 ≈ $10 of workers and 3 TB crossing a zone × $0.02 = $60 of traffic. Postgres follows run count, not rate (a histogram's size is endpoints × windows): at 10× the runs it takes 70 GB/day, ~2 TB at 30 days; move to `db.r7g.2xlarge` and archive windows older than a week to S3. Run count ×10 also multiplies the fleet to ≈ $20k and the traffic to ≈ $79k, which is when pinning workers to a zone stops being optional.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- A small relational control plane: targets, tests (`load` and `budgets` as `jsonb`), statements and baselines, ~500 new tests a day, plus one hard rule: at most one active run per target (section 8).
- The measurements are `load_windows`: one merged histogram (~3 KB `bytea`) per (test, 10 s window, scenario, endpoint), ~125 rows/s, ~7 GB/day, ~200 GB at 30 days (sections 4, 15). A run's series is at most 90 windows × 50 endpoints = 4,500 rows read by `test_id`; percentiles come from adding bucket counts, never from averaging (9.1, 9.3).
- Raw samples are what the histograms replace: 2.9 GB for one 15-minute run at 200k req/s, about 26 GB a day for the nine large runs alone (section 15's mix).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `load_one_active_per_target` is a partial unique index (`WHERE status IN ('preparing','running','finishing')`), so the database itself refuses a second active run on a target; day partitions make 30-day retention a `DROP PARTITION`; 125 rows/s is a few percent of the few thousand durable writes/s one primary takes, and the ~8 GB hot set fits 16 GiB of RAM |
| MySQL (RDS / Aurora) | ✔ works, with two workarounds | no partial index: use a generated column that is `target_id` while the status is active and `NULL` otherwise, with a unique index on it (NULLs do not collide). A partitioned InnoDB table cannot have foreign keys, so a day-partitioned `load_windows` loses `REFERENCES … ON DELETE CASCADE` and the retention job cleans up instead |
| ClickHouse / Timestream | ✖ not needed here, ✔ if raw samples were kept | 125 rows/s is far below what a time-series store is for, and keeping only scalar p95/p99 per window would lose the ability to re-merge windows into a run-level p99 (9.3), so the histogram stays a column. Raw samples (200k a second from one large run) are the workload that would justify it |
| S3 | ✔ for archives and artifacts, ✖ as the query store | windows older than a week (section 15's 10× plan), pprof profiles, report JSON/Markdown and, if ever kept, raw samples as compressed files per run; the live view and the baseline query need rows by key, not files |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.large`, gp3 300 GB, section 15) for everything the platform stores itself. Two things decide it: the partial unique index enforces the "one run per target" safety rule (9.5), and day partitions bound a table that grows 7 GB a day. The windows are the record of a finished run, since raw samples are never kept; `load_baselines` is derived from them and can be rebuilt while they are retained. S3 holds only archives and profiles.

**If it were MySQL instead:** the generated-column unique key replaces the partial index and the foreign key from `load_windows` goes, so orphan windows are removed by the retention job. `text` columns inside keys (`scenario`, `endpoint`, and `query` in `load_statements`) become bounded `VARCHAR`s, because MySQL cannot index a `TEXT` column in full; `jsonb` becomes `JSON`, `bytea` becomes `BLOB`, identity becomes `AUTO_INCREMENT`. InnoDB's clustered key `(test_id, window_start, …)` makes a run's series one range scan, a small plus that does not decide anything at 4,500 rows per run.

**What would flip the choice:** not size: at 10× the runs (section 15) it is ~70 GB/day and ~2 TB at 30 days, which means a bigger node (`db.r7g.2xlarge`) and windows older than a week archived to S3. The windows (not the control plane) would move to ClickHouse or Timestream if you decide to keep raw per-request samples, or to query months of windows across every team at roughly 100× today's 125 rows/s; the control plane stays in Postgres.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

Here the largest run and the number of runs both grow N× (as in the 10× rows of sections 10 and 15); the target stack belongs to the owning team and is not in the bill.

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | largest run 20M req/s, 10M VUs, 2M in flight, 320 Gbit/s of replies; 50,000 runs a day, ~2,500 live at peak. One run = 20M × 0.32 ms = 6,400 vCPU = 1,143 `c7g.2xlarge`; platform peak ≈ 3,900 workers (31,200 vCPU) | the account's vCPU quota, not the design: 9,144 vCPU for one run is 23× the ~400 vCPU section 15 asks for (78× at platform peak). Behind it, bytes: one such run moves 45 TB, 30 TB of it across zones ≈ $600 against $99 of workers | the same design scaled out: ~12 regional coordinators of ~100 workers merge histograms in a tree (section 10); workers spread over several accounts and regions; each worker aims at its own zone's load-balancer address (9.3), so almost nothing crosses a zone; Spot for small and medium runs. Give up: a zone-aligned load shape (no cross-zone realism) and more `degraded` runs from reclaimed Spot workers | ≈ $1.05M a month, cross-zone $790k of it (75 %); ≈ $260k once traffic stays in-zone |
| **1,000×** | largest run 200M req/s (~100× a hyperscale service's peak: a thought experiment), 100M VUs, 3.2 Tbit/s; 11,430 workers for it; 500k runs a day, ~39,000 workers at peak (312k vCPU) | failure and coordination, not capacity: with 11,430 workers one dies about every 26 min (5,000 nodes ≈ 1 h), so "a silent worker makes the run degraded" would degrade ~4 in 10 of the largest runs; a flat merge would be 571k histograms (1.7 GB) a window into one coordinator | independent **cells**: a coordinator, ~100 workers and the zone-local slice of the target, ~114 cells for the largest run; the top merges 114 cell summaries a window (17 MB); a lost worker is replaced inside its cell from a 10 % spare, a lost cell is dropped, and the run is valid if ≥ 99 % of cells report; per-team worker quotas and a fair queue. Give up: one simultaneous T0 (cells start within seconds of it) and per-worker lag detail at the top | ≈ $10.5M a month (cross-zone $7.9M); ≈ $2.6M in-zone |
| **10,000×** | 2B req/s (~100× the world's biggest event pipelines), 1B VUs (one internet user in five), 32 Tbit/s; 114k workers for one run; 5M runs a day | physics and money: one largest run costs ≈ $70k ($7 × 10,000), no target stack takes 2B req/s, and the platform bill is ≈ $105M a month | change the problem: stop generating the whole load. Run one target cell at full rate (the 10× run, 2M req/s), show that cells are independent, and extrapolate with a capacity model (the throughput-vs-latency knee, section 14); replay scaled-down traffic against the shared parts; cap a run at one cell. Give up: a single end-to-end "it held 2B req/s" proof: cell interactions are inferred, not measured | ≈ $105M a month if linear (≈ $26M in-zone); a one-cell cap cuts the largest run's cost ~1,000× |

**What stays the same at every scale:** open-loop timing from the intended send, mergeable histograms (add buckets, idempotent by `(test, worker, window)`) and `valid = false` for a saturated generator: the numbers stay honest whatever the topology.

**Say it in an interview:** "At 100× one run needs 1,143 workers and 9,000 vCPU, so the first thing to run out is the account's quota and the cross-zone bytes: I scale the fleet out, merge in a tree and pin each worker to its zone's load balancer. At 1,000× a worker dies every half hour, so I split a run into independent cells that tolerate loss. At 10,000× no target takes 2B req/s and one run costs $70k, so I stop generating the whole load: one cell at full rate plus a capacity model."
