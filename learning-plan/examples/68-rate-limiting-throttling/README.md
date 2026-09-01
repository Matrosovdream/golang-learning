# Step 68 — Rate Limiting & Throttling · Examples

A library of **26 runnable examples**, split into three files by difficulty. Every one is a complete
`package main` program written in **production shape** — injected clocks, real middleware, real
`http.Client` transports, real graceful shutdown — rather than a toy.

Companion to the lesson: [`68-rate-limiting-throttling.md`](../../68-rate-limiting-throttling.md).

**Run any example:**

```bash
mkdir -p /tmp/rl-ex && cd /tmp/rl-ex
go mod init scratch
go get golang.org/x/time@latest   # once: examples 3-8, 13-17, 23-26 use x/time/rate
# type the example into main.go, then:
go run .
```

Every example was `gofmt`-checked, `go vet`-ed, compiled and **run** before being added — the
**Output** under each one is real stdout. The concurrent ones were also run under `-race`.

**No external services required.** The distributed examples (20–22) run against a `Store`/`Script`
interface with an in-process fake, and the real Redis commands and Lua script are shown alongside.

| Tier | File | Examples | Focus |
|------|------|----------|-------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 | one limiter, one decision: `Allow` / `Wait` / `Reserve`, middleware, transports |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–19 | the algorithm family, per-key limiting + eviction, composing limits, the client side |
| 🔴 Hard | [3-hard.md](3-hard.md) | 20–26 | distributed enforcement, failure modes, operability, capstone service |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 Easy ([1-easy.md](1-easy.md))

- [1. Pace outbound calls with a ticker](1-easy.md#1-pace-outbound-calls-with-a-ticker)
- [2. Fixed window — and its 2× boundary burst](1-easy.md#2-fixed-window-and-its-2-boundary-burst)
- [3. Allow() — the inbound shape](1-easy.md#3-allow-the-inbound-shape)
- [4. Wait(ctx) — the outbound shape, and its error trap](1-easy.md#4-waitctx-the-outbound-shape-and-its-error-trap)
- [5. Reserve(): compute Retry-After, and always Cancel()](1-easy.md#5-reserve-compute-retry-after-and-always-cancel)
- [6. Rate vs burst: what NewLimiter(r, b) permits](1-easy.md#6-rate-vs-burst-what-newlimiterr-b-permits)
- [7. Middleware: 429, Retry-After and RateLimit-*](1-easy.md#7-middleware-429-retry-after-and-ratelimit-)
- [8. Pace an http.Client with a RoundTripper](1-easy.md#8-pace-an-httpclient-with-a-roundtripper)

### 🟡 Medium ([2-medium.md](2-medium.md))

- [9. Sliding window log — exact, and what it costs](2-medium.md#9-sliding-window-log-exact-and-what-it-costs)
- [10. Sliding window counter — the production compromise](2-medium.md#10-sliding-window-counter-the-production-compromise)
- [11. Leaky bucket — shaping, not just limiting](2-medium.md#11-leaky-bucket-shaping-not-just-limiting)
- [12. Token bucket by hand — lazy refill, no goroutine](2-medium.md#12-token-bucket-by-hand-lazy-refill-no-goroutine)
- [13. Per-key limiters: one budget per tenant](2-medium.md#13-per-key-limiters-one-budget-per-tenant)
- [14. Evicting idle keys — the leak every per-key limiter has](2-medium.md#14-evicting-idle-keys-the-leak-every-per-key-limiter-has)
- [15. Tiered limits: the budget comes from the plan](2-medium.md#15-tiered-limits-the-budget-comes-from-the-plan)
- [16. Several limits at once — and why Cancel() can't save you](2-medium.md#16-several-limits-at-once-and-why-cancel-cant-save-you)
- [17. Rate limit vs concurrency limit](2-medium.md#17-rate-limit-vs-concurrency-limit)
- [18. Load shedding on queue depth](2-medium.md#18-load-shedding-on-queue-depth)
- [19. Be a polite client: honour Retry-After](2-medium.md#19-be-a-polite-client-honour-retry-after)

### 🔴 Hard ([3-hard.md](3-hard.md))

- [20. Distributed fixed window with an atomic INCR](3-hard.md#20-distributed-fixed-window-with-an-atomic-incr)
- [21. Distributed token bucket in one atomic script](3-hard.md#21-distributed-token-bucket-in-one-atomic-script)
- [22. GCRA — one timestamp per key, exact Retry-After](3-hard.md#22-gcra-one-timestamp-per-key-exact-retry-after)
- [23. Fail open, fail closed, or fail local](3-hard.md#23-fail-open-fail-closed-or-fail-local)
- [24. A rate-limited, bounded, cancellable fetcher](3-hard.md#24-a-rate-limited-bounded-cancellable-fetcher)
- [25. Metrics: the three numbers that matter](3-hard.md#25-metrics-the-three-numbers-that-matter)
- [26. Capstone: a tenant-aware rate-limited service](3-hard.md#26-capstone-a-tenant-aware-rate-limited-service)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
