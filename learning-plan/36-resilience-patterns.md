# 36 — Resilience Patterns

> Part 9, Track B: [34 Event-Driven & Outbox](34-event-driven-outbox.md) → [35 Sagas](35-sagas-distributed-transactions.md) → **36 Resilience** → [37 CQRS & Event Sourcing](37-cqrs-event-sourcing.md).
> In a distributed system, dependencies *will* be slow, flaky, or down. Resilience patterns keep your service responsive and prevent one sick dependency from taking everything with it. Most are small, composable Go building blocks on top of `context` ([15](15-sync-context.md)).

## Goals
- Bound every outbound call with a **timeout** and propagate cancellation.
- **Retry** transient failures correctly — backoff, jitter, caps, and only for idempotent operations.
- Stop hammering a broken dependency with a **circuit breaker**, and isolate blast radius with **bulkheads**.
- Protect yourself with **rate limiting / load shedding** and degrade **gracefully** with fallbacks.

## Concepts

### Timeouts — the foundation
The default failure mode of a network call is *hang forever*. Every outbound call gets a **deadline** via `context` ([15](15-sync-context.md)), and deadlines propagate to downstreams (gRPC does this automatically, [27](27-grpc-microservices.md)):
```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()
resp, err := client.Do(ctx, req)   // returns context.DeadlineExceeded if it overruns
```
Give each hop a **timeout budget** smaller than its caller's remaining time, so an inner slow call surfaces as a clean deadline rather than a pile-up. A service with no client timeouts is one slow dependency away from exhausting its own goroutines and connections.

### Retries — only when it's safe, always with backoff + jitter
Retrying converts a *transient* blip (a dropped connection, a `503`, a leader election) into success. But retry wrongly and you (a) duplicate non-idempotent side effects and (b) **synchronise a thundering herd** that DDoSes a recovering dependency. Rules:
- **Only retry idempotent operations** (GETs, or writes guarded by an idempotency key, [41](41-api-design-evolution.md)). Never blindly retry a bare "charge card".
- **Only retry retryable errors** — timeouts, connection failures, `503`/`429`, gRPC `Unavailable`. A `400`/`NotFound` won't get better; retrying wastes time.
- **Exponential backoff + jitter** — spread retries out and randomise so clients don't resynchronise:
```go
func retry(ctx context.Context, attempts int, op func() error) error {
    const base = 100 * time.Millisecond
    var err error
    for i := 0; i < attempts; i++ {
        if err = op(); err == nil || !retryable(err) {
            return err
        }
        // full jitter: sleep in [0, base*2^i)
        backoff := base * (1 << i)
        d := jitter(backoff)                 // rand in [0, backoff)
        select {
        case <-time.After(d):
        case <-ctx.Done():
            return ctx.Err()                 // respect cancellation while waiting
        }
    }
    return err
}
```
Cap the number of attempts *and* the total time (the caller's deadline already bounds it). Consider a **retry budget** (retries as a fraction of total requests) so a widespread outage doesn't multiply load.

### Circuit breaker — fail fast when a dependency is down
Retrying a dependency that's *hard down* just burns your resources and delays the inevitable error. A **circuit breaker** is a small state machine ([30](30-patterns-behavioral.md)) that trips after too many failures and then **fails fast** without calling, giving the dependency room to recover:
```
CLOSED  — calls pass through; count failures.
          too many failures → trip → OPEN
OPEN    — calls fail immediately (no call made); after a cooldown → HALF-OPEN
HALF-OPEN — allow a few trial calls; success → CLOSED, failure → OPEN
```
```go
type Breaker struct {
    mu           sync.Mutex
    state        State // closed/open/halfOpen
    failures     int
    threshold    int
    openUntil    time.Time
    cooldown     time.Duration
}

func (b *Breaker) Do(fn func() error) error {
    if !b.allow() {
        return ErrOpen                      // fail fast, dependency is presumed down
    }
    err := fn()
    b.record(err)
    return err
}
```
In production use a battle-tested one (`sony/gobreaker`) rather than hand-rolling all the edge cases. Trip on an **error rate over a window**, not a raw count, so a burst of traffic doesn't false-trip.

### Bulkhead — isolate resources so one failure can't sink the ship
Named after a ship's watertight compartments: partition resources so a flood in one can't drown the rest. Concretely, **cap concurrency per dependency** with its own semaphore/pool so a slow dependency can't consume every goroutine/connection your service has:
```go
// A bounded semaphore = a bulkhead around calls to one dependency (lesson 15).
sem := make(chan struct{}, 10)     // at most 10 concurrent calls to service X
func callX(ctx context.Context) error {
    select {
    case sem <- struct{}{}:
        defer func() { <-sem }()
    case <-ctx.Done():
        return ctx.Err()           // shed rather than queue unboundedly
    }
    return doCallX(ctx)
}
```
Now if X hangs, at most 10 goroutines are stuck; calls to Y and your health endpoint keep working. Separate connection pools per dependency are the same idea.

### Rate limiting & load shedding — protect *yourself*
Timeouts/retries/breakers protect you from *others*; rate limiting protects you from *overload*. Use a **token-bucket** limiter (`golang.org/x/time/rate`) to cap the work you accept, and **shed** excess fast with `429`/`503` instead of collapsing:
```go
lim := rate.NewLimiter(rate.Limit(500), 100) // 500 rps, burst 100
func handle(w http.ResponseWriter, r *http.Request) {
    if !lim.Allow() {
        http.Error(w, "slow down", http.StatusTooManyRequests) // 429 + Retry-After
        return
    }
    // ... serve
}
```
**Load shedding** is deliberate: past a concurrency/latency threshold, reject new work immediately so in-flight work still finishes. A fast `503` is far better than a slow death where every request times out. (You saw the token bucket and semaphore as concurrency tools in [15](15-sync-context.md); here they're reliability tools.)

### Graceful degradation & fallbacks
When a non-critical dependency is down, degrade instead of failing the whole request: serve a **stale cache** ([38](38-caching-patterns.md)), a default/empty result, or a reduced feature set. Pair with the circuit breaker: *open breaker → serve fallback.* Decide per feature what "degraded but useful" means (e.g. recommendations missing is fine; the checkout total is not).

### They compose — the standard stack
These layer together on one call, outermost to innermost:
```
rate-limit (accept?) → bulkhead (capacity?) → circuit-breaker (dependency up?)
    → timeout (bounded) → retry (transient?) → the actual call
                                         ↓ all failed
                                      fallback (degrade)
```
Add health/readiness probes ([23](23-config-logging.md)) so orchestrators stop routing to an unhealthy instance, and graceful shutdown ([21](21-rest-api.md)) so drains don't drop in-flight work.

## Exercises
1. Wrap an outbound HTTP call in `context.WithTimeout`; make the server sleep past the deadline and confirm you get `context.DeadlineExceeded`, not a hang.
2. Implement `retry(ctx, attempts, op)` with exponential backoff **and full jitter**, that stops on non-retryable errors and respects `ctx.Done()` while sleeping. Log the actual sleep each attempt and confirm jitter spreads them.
3. Classify errors: write `retryable(err)` that returns true for timeouts/`503`/`429`/gRPC `Unavailable` and false for `400`/`NotFound`. Verify a `404` isn't retried.
4. Build a circuit breaker with closed/open/half-open states that trips after N failures, fails fast while open, and probes in half-open. Drive it through all transitions with a fake failing dependency.
5. Add a **bulkhead**: a semaphore capping concurrency to one dependency at 10; show that when it hangs, calls to a second dependency and `/healthz` still succeed.
6. Add a `rate.Limiter` to a handler that returns `429` + `Retry-After` past 500 rps; load-test it and confirm it sheds instead of collapsing.
7. Combine timeout + retry + breaker + fallback around one call so that when the dependency is down, the request returns a stale-cache fallback quickly. Trace which layer produced the response.

## Best Practices & Pitfalls
- **Every network call has a timeout.** No exceptions. Propagate deadlines; give inner hops a smaller budget than their caller.
- **Retry only idempotent ops, only on retryable errors, always with backoff + jitter, always capped.** Unbounded or synchronised retries turn a blip into an outage.
- **Fail fast when a dependency is down.** A circuit breaker saves both sides; trip on error-rate over a window, not a single failure.
- **Bulkhead by dependency.** Separate concurrency limits / pools so one slow dependency can't exhaust your whole service.
- **Shed load deliberately.** A quick `429`/`503` beats a slow-motion collapse. Protect the work already in flight.
- **Degrade gracefully.** Decide, per feature, what a useful reduced response is; wire it to the breaker's open state.
- **Pitfall — retry storms / retry amplification.** Retries at every layer multiply (3×3×3 = 27 calls). Retry at *one* layer, budget it, and add jitter.
- **Pitfall — timeout longer than the caller's.** An inner timeout ≥ the outer one never fires usefully; the caller gives up first and the inner work becomes wasted load.
- **Pitfall — breaker with no half-open probe.** A breaker that opens forever needs manual intervention. Always allow trial calls to auto-recover.
- **Pitfall — retrying non-idempotent writes without an idempotency key.** That's how one "charge" becomes three.

## Checklist
- [ ] Every outbound call in my service has a context deadline, with a sane budget hierarchy.
- [ ] I can write correct retry with exponential backoff, jitter, a retryable-error predicate, and caps.
- [ ] I can implement (or correctly use) a circuit breaker and explain closed/open/half-open.
- [ ] I can bulkhead a dependency with a semaphore and show blast-radius containment.
- [ ] I can add rate limiting / load shedding that returns `429`/`503` under overload.
- [ ] I can wire a graceful-degradation fallback to a breaker's open state.
- [ ] I know why retries must be idempotent, jittered, capped, and confined to one layer.

## Resources
- Michael Nygard, *Release It!* — the canonical source for circuit breaker, bulkhead, timeouts.
- AWS Builders' Library, "Timeouts, retries, and backoff with jitter": https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/
- `golang.org/x/time/rate` (token-bucket limiter): https://pkg.go.dev/golang.org/x/time/rate
- `sony/gobreaker` (Go circuit breaker): https://github.com/sony/gobreaker
- Prior art in this plan: token bucket & semaphore in [15 — Sync, Context & Patterns](15-sync-context.md); retry/rate-limit in the `watch-hub` example project.
- Next: [37 — CQRS & Event Sourcing](37-cqrs-event-sourcing.md).
