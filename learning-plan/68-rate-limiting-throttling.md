# 68 — Rate Limiting & Throttling

> Deep-dive companion to [36 — Resilience Patterns](36-resilience-patterns.md), which introduces the token bucket as *one* of many resilience tools. This lesson is about the whole family: which algorithm to pick, how to key it per tenant, how to enforce it across replicas, and how to wire it into a real HTTP service.
> Read after [15 — Sync & Context](15-sync-context.md) (every limiter is mutex + clock + cancellation) and [20 — HTTP Server](20-http-server.md) / [21 — REST API](21-rest-api.md) (middleware is where limiters live). Pairs with [57 — Web Security](57-web-security.md) (limiting is a control against credential stuffing and scraping) and [67 — Multi-User State](67-multi-user-state.md) (per-user maps and their eviction problem).

## Goals
- Tell the **five algorithms** apart — fixed window, sliding window log, sliding window counter, token bucket, leaky bucket — and pick deliberately.
- Use **`golang.org/x/time/rate`** properly: `Allow` vs `Wait` vs `Reserve`, and what `NewLimiter(r, b)` actually permits.
- Build **per-key limiters** (per tenant, per IP, per API key) *without leaking memory*.
- Distinguish **rate limiting** (events per unit time) from **concurrency limiting** (in-flight at once) and **load shedding** (reject when unhealthy).
- Enforce limits **across replicas** with an atomic backend, and choose **fail-open vs fail-closed** on purpose.
- Return the **right HTTP response**: `429`, `Retry-After`, and the `RateLimit-*` headers.

## Concepts

### Limiting vs throttling vs shedding

Three different answers to "too much traffic", and mixing them up produces bad systems:

| | Question it answers | Typical action |
|---|---|---|
| **Rate limiting** | how many *events per unit time*? | reject over-limit requests (`429`) |
| **Throttling / shaping** | how fast may they *proceed*? | delay — queue and drain at a fixed rate |
| **Concurrency limiting** | how many *at once*? | block until a slot frees (a semaphore, [13](13-goroutines.md) #29) |
| **Load shedding** | am I *healthy enough* to accept this? | reject early based on queue depth or latency |

A limiter says "10 per second." A semaphore says "3 at a time." A slow dependency needs the second, not the first.

### The five algorithms

- **Fixed window** — count requests in the current clock window (`10:00:00–10:00:59`), reset at the boundary. One counter per key. Trivially cheap, and **allows a 2× burst across the boundary**: 100 requests at `10:00:59` and 100 more at `10:01:00` is 200 in one second under a "100/min" limit.
- **Sliding window log** — store a timestamp per request, drop those older than the window, count what's left. **Exact**, no boundary burst, but memory grows with the limit (100k/min = 100k timestamps *per key*).
- **Sliding window counter** — keep the current and previous window counts, and weight the previous one by how far into the current window you are. Fixes most of the boundary burst at two integers per key. The standard production compromise.
- **Token bucket** — a bucket of capacity `b` refilled at `r` tokens/sec; each request takes one. **Permits a burst of `b`** then settles to `r`. This is `x/time/rate`, and it's the default choice.
- **Leaky bucket** — a queue drained at a constant rate. Output is perfectly smooth (**shaping**, not just limiting) and requests *wait* rather than fail. Right for protecting a fragile downstream; wrong when latency matters more than smoothness.

Token bucket and leaky bucket are duals: same accounting, different verb — one **rejects** the overflow, the other **queues** it.

### `golang.org/x/time/rate`

```go
lim := rate.NewLimiter(rate.Every(100*time.Millisecond), 5) // 10/sec, burst 5
```

`rate.Every(d)` converts a period to a rate. The second argument is **burst**, and it is not optional detail: `NewLimiter(10, 1)` permits ten per second strictly paced; `NewLimiter(10, 100)` permits a hundred at once and *then* ten per second. Burst is how much accumulated idleness a caller may spend at once.

Three ways to consume a token, and the choice is a design decision:

| Method | Behaviour | Use for |
|---|---|---|
| `Allow()` | `true`/`false` immediately | inbound requests — reject with `429` |
| `Wait(ctx)` | blocks until a token or ctx dies | outbound calls — pace yourself |
| `Reserve()` | tells you the delay, lets you decide | when you need `Retry-After`, or to shed if the wait is too long |

`Reserve()` **holds** the token until you act; call `r.Cancel()` if you decide not to proceed, or you've silently consumed budget.

### Per-key limiting and the leak

Real limits are per tenant / per IP / per API key, which means a `map[string]*rate.Limiter` behind a mutex. That map **grows forever** unless you evict — one entry per IP that ever hit you is a memory leak with a user-supplied key, i.e. an attack. Every per-key limiter needs a `lastSeen` timestamp and a sweeper (see [67](67-multi-user-state.md), same problem, same fix).

### Distributed limiting

Per-process limiters multiply: three replicas each allowing 100/s is 300/s. Options, in increasing order of cost:

1. **Divide the budget** — `limit / replicas`. Free, but wrong under uneven load balancing and wrong the moment you scale.
2. **Shared atomic counter** (Redis `INCR` + `EXPIRE`) — fixed window across replicas. One round trip per request.
3. **Server-side script** (Redis Lua, or `CL.THROTTLE` from redis-cell) — token bucket or GCRA evaluated atomically. One round trip, exact semantics.

**GCRA** (Generic Cell Rate Algorithm) is the leaky-bucket-as-a-meter formulation: instead of a count it stores a single "theoretical arrival time", which makes it one value per key and naturally distributed. It's what redis-cell implements.

And decide the failure mode explicitly: when Redis is down, do you **fail open** (serve everything, risk overload) or **fail closed** (reject everything, guaranteed outage)? For abuse prevention, fail closed; for capacity protection of a healthy service, fail open with a local fallback limiter.

### The HTTP response

```
HTTP/1.1 429 Too Many Requests
Retry-After: 3
RateLimit-Limit: 100
RateLimit-Remaining: 0
RateLimit-Reset: 3
```

`Retry-After` (seconds, or an HTTP date) is the one clients actually honour. The `RateLimit-*` family is the IETF draft standard and worth emitting on **every** response, not just rejections, so well-behaved clients can self-pace. Never return `503` for a rate limit — that says *you* are broken.

## Exercises
1. Build a fixed-window limiter and *demonstrate* the boundary burst: 2× the limit inside one second, straddling the window edge.
2. Replace it with a sliding-window counter and show the same traffic pattern now gets rejected.
3. Use `rate.NewLimiter(rate.Every(200*time.Millisecond), 3)`; log `Allow()` and `Tokens()` for 10 rapid calls, then sleep 1s and repeat. Explain the burst.
4. Swap `Allow()` for `Wait(ctx)` with a 250ms context; show some calls succeed and others return `context.DeadlineExceeded`.
5. Use `Reserve()` to build a `Retry-After` header value, and `Cancel()` the reservation when you decide to shed instead.
6. Write a `perKeyLimiter` with a `map[string]*entry` + mutex + `lastSeen`, and a sweeper goroutine that evicts idle keys. Prove eviction happens.
7. Write HTTP middleware returning `429` + `Retry-After` + `RateLimit-*`, keyed by `X-API-Key` with a per-plan limit.
8. Combine a rate limiter with a semaphore ([13](13-goroutines.md) #29) and articulate what each one protects.

## Best Practices & Pitfalls
- **Pick burst deliberately.** `burst == 1` is a strict pacer and will feel broken to bursty clients; `burst == limit` lets a client spend a whole minute's budget in one millisecond. Most APIs want a small burst (5–20% of the rate).
- **Limit on the right key.** Limiting by IP punishes everyone behind a NAT or corporate proxy; limiting by API key or account is fairer. For unauthenticated endpoints you often need both.
- **Trust `X-Forwarded-For` only behind your own proxy**, and only the hop you control — otherwise the key is attacker-supplied and the limit is bypassable ([57](57-web-security.md)).
- **Pitfall — the unbounded key map.** The most common real bug in per-key limiting. Always evict.
- **Pitfall — limiting after the expensive work.** Middleware order matters: limit before auth DB lookups and body parsing, not after.
- **Pitfall — one global mutex.** A single lock in front of every request is a bottleneck at high RPS; shard the map ([13](13-goroutines.md) #38) or use per-key locks.
- **Pitfall — `Reserve()` without `Cancel()`.** Silently drains budget for requests you never served.
- **Pitfall — testing with `time.Sleep`.** Inject a clock instead, or your limiter tests are slow *and* flaky ([40](40-testing-architecture.md), [49](49-testing-kinds.md)).
- **Emit metrics** — allowed, limited, and wait-time, labelled by key class. A limiter you can't observe is one you can't tune.
- **Document the limit** in your API docs and headers. An undocumented limit is indistinguishable from an outage.

## Checklist
- [ ] I can name the five algorithms and the failure mode of each.
- [ ] I know what `NewLimiter(r, b)` permits in the first millisecond.
- [ ] I can choose between `Allow`, `Wait`, and `Reserve` and justify it.
- [ ] My per-key limiter evicts idle keys.
- [ ] I can say whether my limit is per-process or per-cluster, and whether that's intentional.
- [ ] My limiter fails open or closed by decision, not by accident.
- [ ] I return `429` with `Retry-After`, never `503`.
- [ ] I know when a semaphore is the right tool instead.

## Resources
- [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) — the standard Go limiter.
- [RFC 6585 §4](https://www.rfc-editor.org/rfc/rfc6585#section-4) — `429 Too Many Requests`.
- [IETF draft: RateLimit header fields](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/) — the `RateLimit-*` family.
- [redis-cell](https://github.com/brandur/redis-cell) — GCRA as a Redis module.
- [Stripe's rate limiters](https://stripe.com/blog/rate-limiters) — how a real API layers several limiter types.

---
*Examples: [`examples/68-rate-limiting-throttling/`](examples/68-rate-limiting-throttling/) · Progress: [PROGRESS.md](PROGRESS.md)*
