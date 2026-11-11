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
