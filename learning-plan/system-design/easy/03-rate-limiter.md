# 03 · Rate Limiter

`🟢 easy` · *Topics:* token bucket vs window counters, where the limiter lives, Redis + Lua atomicity, fail open vs fail closed, `429` semantics · *Builds on:* [01 URL Shortener](01-url-shortener.md) (uses it for creation limits)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [02 Pastebin](02-pastebin.md) · Next: [04 Unique ID Generator](04-unique-id-generator.md)

---

## 1. The task

> "Design a rate limiter for our public API. Each client may make at most N requests per time
> window. Requests over the limit are rejected."

Short to state, and every word hides a decision: what is a *client*, what is a *window*, what
does *rejected* look like, and where does the counting happen when there are twenty API servers.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What identifies a client? | API key is exact; IP is shared by NATs and spoofable behind proxies. | API key when present, else client IP. |
| One global limit or per endpoint? Per tier? | Rule model complexity. | Per (client, endpoint group), with a plan tier deciding the numbers. |
| Should bursts be allowed? | Token bucket allows a burst up to the bucket size; fixed windows allow 2× at the edge accidentally. | Yes, a controlled burst (e.g. 100/min with up to 20 at once). |
| How many API servers? Do they share state? | A per-server limiter multiplies the limit by the server count. | ~20 stateless servers; the limit must be global. |
| How exact must it be? | Exact needs a central store on every request; approximate allows local caching. | A few percent over is fine. Never massively over. |
| Latency budget for the check? | Decides in-memory vs network hop. | Under 2 ms added per request. |
| If the limiter's store is down? | Fail open (allow) or fail closed (reject). | Fail open, alert loudly. |
| Scale? | Sizing the store. | 10k requests/s, 100k distinct active clients. |
| What does the client need to know? | Headers for remaining quota and when to retry. | Yes: `429`, `Retry-After`, remaining/limit headers. |

## 3. Requirements

**Functional**
- Enforce a limit per client per endpoint group; allow a configured burst.
- Reject with `429 Too Many Requests` and tell the client when to retry.
- Rules are configurable without redeploying.

**Non-functional**
- Adds under 2 ms to a request; uses little memory per client.
- Accurate across all API servers (one global count, a few percent slack allowed).
- The limiter failing must not take the API down.

**Out of scope**
- Per-region limits, dynamic limits based on load (load shedding, lesson 48), billing/quotas.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Checks | 10k/s, peaks 50k/s | one check per request |
| Active clients | 100k | keys in the store |
| State per client | ~100 B | key name + 2 numbers (tokens, last refill) + Redis overhead |
| Store memory | ~10 MB | 100k × 100 B; trivial |
| Store ops | 10–50k/s | one atomic op per check; a single Redis handles ~100k/s |
| Added latency | ~0.5 ms | one Redis round trip inside the datacenter |

What the numbers tell us: a single Redis node can be the global counter at this scale, and
memory is a non-issue. The design is about **correctness** (atomic updates from 20 servers)
and **failure behaviour**, not capacity.

## 5. Plan

1. Choose the algorithm; the data per client follows from it.
2. Decide where the limiter runs: in the app, at the gateway, or as a service.
3. Make the update atomic across servers.
4. Define the client-facing contract (`429`, headers).
5. Decide the failure mode and how rules are configured.
6. Scaling and hot keys.

## 6. High-level design

```
 client ──► LB / API gateway ──► API server
                                   │  middleware: rate limit
                                   │    key = rl:{tier}:{client}:{endpoint-group}
                                   ▼
                                 Redis  (one Lua script: refill tokens, try to take one, return allowed + remaining + retry_after)
                                   │
                        allowed ───┴─── denied → 429 + Retry-After + X-RateLimit-*

 rules: config service / DB ──(poll every 30 s)──► in-memory rule table on every API server
 fallback: Redis unreachable → local in-process limiter at a conservative rate, metric + alert
```

A request arrives; the middleware resolves the client identity and the matching rule, runs one
atomic Redis script keyed by client and endpoint group, and either passes the request on or
answers `429`. The script does the whole read-modify-write on the Redis side, so twenty servers
can hit the same key without racing.

## 7. API

The limiter has no API of its own; it shapes every response.

```
200 OK
  X-RateLimit-Limit: 100          ← requests allowed per window
  X-RateLimit-Remaining: 57
  X-RateLimit-Reset: 1760345600   ← unix time when the bucket is full again (or seconds until)

429 Too Many Requests
  Retry-After: 12                  ← seconds (RFC 9110)
  Content-Type: application/problem+json
  { "type": "…/rate-limited", "title": "Too many requests", "retry_after": 12 }
```

The headers are a de-facto convention (GitHub, Stripe). The IETF `RateLimit` header draft
standardises them; use whichever your API already uses, consistently.

Admin API for rules:

```
GET  /admin/rate-limits                      list rules
PUT  /admin/rate-limits/{tier}/{endpoint}    { "limit": 100, "window": "1m", "burst": 20 }
```

## 8. Data model

Redis, per client and endpoint group, for a token bucket:

```
HSET rl:pro:key_8f3a:write  tokens 17.4  ts 1760345588.213
EXPIRE rl:pro:key_8f3a:write 120        ← idle clients vanish on their own
```

Rules (Postgres, or a YAML file in config management):

```sql
CREATE TABLE rate_limit_rules (
    tier            TEXT NOT NULL,       -- free, pro, enterprise
    endpoint_group  TEXT NOT NULL,       -- read, write, auth
    rate_per_sec    NUMERIC NOT NULL,    -- refill rate
    burst           INTEGER NOT NULL,    -- bucket capacity
    PRIMARY KEY (tier, endpoint_group)
);
```

## 9. Deep dives

### 9.1 Which algorithm

| Algorithm | How it works | State | Pros | Cons |
|---|---|---|---|---|
| **Fixed window counter** | `INCR rl:{client}:{minute}`; reject if > N | 1 int | trivial, cheap | **2× burst at the boundary:** 100 at 11:59:59 and 100 at 12:00:00 are both allowed |
| **Sliding window log** | store every request timestamp (sorted set); count those within the last window | 1 timestamp per request | exact | memory grows with the limit (1,000/min = 1,000 entries per client); ZSET ops are O(log n) |
| **Sliding window counter** | current window count + previous window count × overlap fraction | 2 ints | smooth, cheap, close to exact (Cloudflare reports ~0.003 % error) | an approximation; assumes even distribution in the previous window |
| **Token bucket** | bucket of capacity *burst* refills at *rate* tokens/s; a request takes one token or is rejected | 2 numbers | burst is an explicit parameter; the industry default (AWS, Stripe); maps to `x/time/rate` | needs a timestamp and float math; must be atomic |
| **Leaky bucket** | requests enter a queue drained at a fixed rate | a queue | perfectly smooth output | it *shapes* (delays) rather than *limits* (rejects); adds latency; queue memory |

**Choice: token bucket.** It expresses exactly what we agreed on, "100 per minute with bursts
up to 20", and the state is two numbers. Sliding window counter is the runner-up when you want
"N per window" semantics with no burst parameter.

Token bucket in words: `tokens = min(burst, tokens + (now − ts) × rate)`; if `tokens ≥ 1` then
`tokens −= 1`, allow; else deny with `retry_after = (1 − tokens) / rate`.

### 9.2 Where the limiter lives

| Location | Pros | Cons |
|---|---|---|
| **In-process, per server** (`x/time/rate`) | no network hop, no dependency, microseconds | each server has its own count: with 20 servers the real limit is 20 × N, and uneven balancing makes it unpredictable |
| **Centralised store** (Redis) called from middleware | one global count, accurate | +0.5 ms per request, a dependency that can fail, a hot spot |
| **API gateway / proxy** (Envoy, NGINX, Kong) | nothing in app code, applies to every service | the gateway still needs a shared store for a global limit; rule logic lives in config you don't unit-test |
| **Dedicated rate-limit service** (gRPC, e.g. Envoy's ratelimit) | one place for rules and the store, reused by all services | one more service; two hops instead of one |

**Choice: middleware + Redis**, with the in-process limiter as the fallback when Redis is
unreachable. At our scale the gateway and the dedicated service add operational weight without
removing the shared store they'd need anyway. If many services need limits, move the same
logic behind the gateway later; the Redis script does not change.

### 9.3 Atomicity: twenty servers, one key

A naive `GET` → compute → `SET` from two servers interleaves and both allow when one should
have denied. Options:

- `MULTI`/`EXEC` with `WATCH`: optimistic; retries under contention, exactly the hot-client case.
- **A Lua script** (`EVALSHA`): Redis runs it atomically, the read-modify-write happens server-side in one round trip, no retries. This is the standard answer.
- For the fixed-window counter alone, `INCR` + `EXPIRE` is enough (`INCR` is atomic), as long as `EXPIRE` is set when the counter is created (`SET NX EX` or the same script).

Use Redis's own clock (`TIME` inside the script) rather than each server's clock, so skew
between servers can't refill a bucket early.

### 9.4 Fail open or fail closed

Redis is down or slow. Two wrong answers and one trade-off:

- **Fail closed** (reject everything): the API is down because an auxiliary system is down. For a public API this is almost always the worse outcome.
- **Fail open** (allow everything): abusers get a free window, which for most APIs costs money, not correctness.
- **Degrade:** fall back to the in-process limiter at a conservative rate (e.g. `limit / server_count`), emit a metric, page someone. This caps the blast radius while keeping the API up.

Also give the Redis call a tight timeout (5–10 ms). A slow limiter is a slow API; a timed-out
check counts as "store unavailable" and takes the fallback path.

Security endpoints (login, password reset, OTP) are the exception: fail closed there, since the
limit *is* the control.

### 9.5 Identifying the client

- API key → exact and per account. Prefer it.
- Without a key: client IP. Behind a load balancer the IP is the LB's; read the real one from
  `X-Forwarded-For`, but **only the entry added by your own proxy**, since clients can set the
  header themselves. Many users share an IP (offices, carriers, NAT): keep IP limits generous and
  use them for abuse, not quotas.
- User ID after authentication is the most precise but is available only after auth runs; run
  a cheap IP limit before auth and the account limit after.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 500k checks/s exceeds one Redis node. Shard Redis by client key
  (every check touches one key, so sharding is trivial). A single enormous client is still one
  key at maybe 50k/s, within one node.
- **Reducing round trips:** servers can take tokens in small batches ("lease" 5 tokens locally,
  refill from Redis when they run out). Accuracy drops by at most the lease size per server; the
  Redis load drops 5×. This is how global limiters at very high scale work (hard track).
- **Redis failover:** a replica promoted after the primary dies has slightly stale buckets. A few
  extra requests get through once. Acceptable.
- **Rule changes:** servers poll rules every 30 s; a change applies within that window. Keep rules
  per tier, not per client, so the table stays small and fits in memory on every server.
- **Thundering reset:** fixed windows let every client's quota reset at the same instant and
  create a spike. Token buckets refill continuously and don't have this problem.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| token bucket | fixed window | smooth limiting, explicit burst | slightly more state and math |
| central Redis | per-server limiter | one true global count | a network hop and a dependency |
| Lua script | read-then-write | atomic without retries | a few lines of Lua to maintain |
| fail open + local fallback | fail closed | the API stays up when Redis dies | a brief window of over-admission |
| middleware in the app | gateway/sidecar | testable, deployable with the service | each service must include it |

## 12. Check yourself

1. Why is a fixed window "wrong", and by how much in the worst case?
2. You scale from 1 to 10 API servers with an in-process limiter. What is the effective limit, and why is it worse than just 10× N?
3. Why a Lua script rather than `GET` then `SET`?
4. Fail open or fail closed for (a) a public read API, (b) the login endpoint? Why?
5. Why use Redis's clock rather than the API server's?

<details><summary>Answers</summary>

1. The count resets at the window boundary, so a client can use a full quota at the end of one
   window and another full quota at the start of the next: up to 2× the limit within one window's
   length.
2. Roughly 10 × N, but not reliably: the load balancer doesn't spread one client evenly, so some
   servers hit their local limit while others have headroom, and the client sees intermittent
   `429`s well below 10 × N. Unpredictable *and* too generous.
3. Two servers can both read the same token count, both decide to allow, both write. The script
   runs the read-modify-write atomically on the Redis side in one round trip, so there is no window
   to race in and no retry loop.
4. (a) Fail open with a conservative local fallback: the limiter protects cost, and taking the API
   down costs more. (b) Fail closed: the limit *is* the security control against credential
   stuffing; allowing unlimited attempts defeats it.
5. Server clocks drift from each other; a server whose clock runs ahead would compute extra refill
   and let its requests through early. One clock, the store's, keeps every server's view consistent.

</details>

## 13. In Go

- Related lessons: [68 Rate limiting & throttling](../../68-rate-limiting-throttling.md) is this task in code; [21 REST API](../../21-rest-api.md) and [29 Structural patterns](../../29-patterns-structural.md) for middleware (decorator); [36 Resilience](../../36-resilience-patterns.md) for the timeout and fallback; [15 Sync & context](../../15-sync-context.md) for the in-process limiter's mutex.
- `golang.org/x/time/rate` is a token bucket (`rate.NewLimiter(r, burst)`, `Allow()`, `Wait(ctx)`), exactly the per-server fallback.
- Write the token-bucket refill math once by hand; it is ten lines and a common interview follow-up.
- Packages: `go-redis` (`ScriptLoad`/`EvalSha`), `x/time/rate`, `net/http`.

## 14. Extensions

- Sliding window counter in Redis with two keys and the overlap weighting; compare accuracy with the token bucket under a bursty load generator.
- Token leasing for very high throughput (local batches, bounded error).
- Per-tenant fairness: a shared downstream (a database) with per-tenant *weights* rather than hard limits.
- Shadow mode: compute the decision, log it, don't enforce; use it to tune new rules safely.
- Distinguish limits (reject) from load shedding (reject because *we* are overloaded, lesson 48).
- Multi-region: a client hitting two regions. Local limits per region with a global cap (hard track).
