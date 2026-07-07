# Step 36 — Resilience Patterns · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[36-resilience-patterns.md](../../36-resilience-patterns.md): timeouts, retries with backoff & jitter,
circuit breakers, bulkheads, rate limiting, load shedding, and graceful degradation.

## One-time setup

```bash
mkdir -p /tmp/resil-ex && cd /tmp/resil-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only, and **deterministic**: backoff durations are printed (not
slept), and the jitter example seeds `math/rand` so its output is reproducible. (Example 6 uses
`for range 3`, so needs **Go 1.22+**.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — timeouts, retries, backoff
- [1. Timeout with context](1-easy.md#1-timeout-with-context)
- [2. Retryable vs non-retryable errors](1-easy.md#2-retryable-vs-non-retryable-errors)
- [3. A retry loop](1-easy.md#3-a-retry-loop)
- [4. Exponential backoff](1-easy.md#4-exponential-backoff)
- [5. Backoff with jitter](1-easy.md#5-backoff-with-jitter)

### 🟡 [Medium](2-medium.md) — breaker, bulkhead, rate limit
- [6. Circuit breaker states](2-medium.md#6-circuit-breaker-states)
- [7. The breaker fails fast when open](2-medium.md#7-the-breaker-fails-fast-when-open)
- [8. Bulkhead: cap concurrency](2-medium.md#8-bulkhead-cap-concurrency)
- [9. Token-bucket rate limiter](2-medium.md#9-token-bucket-rate-limiter)
- [10. Load shedding](2-medium.md#10-load-shedding)

### 🔴 [Hard](3-hard.md) — composition & capstone
- [11. Retry only idempotent ops](3-hard.md#11-retry-only-idempotent-ops)
- [12. Retry guarded by a breaker](3-hard.md#12-retry-guarded-by-a-breaker)
- [13. Graceful degradation (fallback)](3-hard.md#13-graceful-degradation-fallback)
- [14. A retry budget](3-hard.md#14-a-retry-budget)
- [15. Capstone: the full resilience stack](3-hard.md#15-capstone-the-full-resilience-stack)
