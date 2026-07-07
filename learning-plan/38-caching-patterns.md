# 38 — Caching Patterns

> Part 9, Track C (production cross-cutting): **38 Caching** → [39 Observability: Tracing](39-observability-tracing.md) → [40 Testing Architecture](40-testing-architecture.md) → [41 API Design & Evolution](41-api-design-evolution.md).
> Caching is the highest-leverage performance tool and one of the easiest to get subtly wrong. This lesson covers the strategies, the invalidation problem, and the concurrency traps — with Go implementations (`singleflight`, an in-memory TTL cache, Redis).

## Goals
- Choose a caching **strategy** (cache-aside, read/write-through, write-behind) deliberately.
- Handle **TTL, eviction, and invalidation** — the genuinely hard part.
- Prevent **cache stampede** with `singleflight` and jittered TTLs.
- Know **local vs distributed** caches and when to combine them.

## Concepts

### The cache is a disposable optimisation, never the source of truth
First principle: a cache can vanish at any moment (eviction, restart, Redis failover) and your system must still be **correct**, just slower. Everything below follows from that — never store data only in the cache, and never let a cache miss become a correctness bug. (Phil Karlton: "There are only two hard things in Computer Science: cache invalidation and naming things.")

### Cache-aside (lazy loading) — the default
The application manages the cache explicitly: check cache → on miss, load from the source and populate → return. Most caching you'll write:
```go
func (c *UserCache) Get(ctx context.Context, id string) (User, error) {
    if u, ok := c.store.Get(ctx, "user:"+id); ok {   // 1. try cache
        return u, nil
    }
    u, err := c.db.GetUser(ctx, id)                  // 2. miss → load source of truth
    if err != nil {
        return User{}, err
    }
    c.store.Set(ctx, "user:"+id, u, 5*time.Minute)   // 3. populate with a TTL
    return u, nil
}
```
Pros: simple, resilient (cache down → just slower), caches only what's actually read. Cons: first read per key is a miss; risk of stale data until TTL/invalidation; needs stampede protection (below).

### Read-through / write-through / write-behind
Who owns the read/write differs:
- **Read-through** — the cache *itself* loads on miss (you configure a loader; the app only ever talks to the cache). Cleaner call sites; needs a cache that supports loaders.
- **Write-through** — writes go **through the cache to the DB synchronously**; cache and DB updated together, so the cache is never stale for written keys. Cost: every write pays the cache write.
- **Write-behind (write-back)** — writes hit the cache and are **flushed to the DB asynchronously**. Fast writes, absorbs bursts — but a crash before flush **loses data**, so only for tolerable-loss data (counters, metrics), never orders or payments.

### TTL, eviction, and the invalidation problem
- **TTL** — bound staleness by expiring entries. **Always set a TTL**, even a long one, as a safety net against caches you forget to invalidate. Add **jitter** to TTLs so many keys don't expire at the same instant (stampede).
- **Eviction** — a bounded cache evicts under pressure, usually **LRU** (least-recently-used). Size it to your hot set.
- **Invalidation on write** — when the source changes, either **delete** the key (simplest — next read repopulates) or **update** it. Deletion is safer than update (fewer race windows). Beware the cache-aside race: reader loads old value, writer updates DB + deletes key, reader sets the *stale* value it already fetched → stale cache. Mitigations: short TTLs, delete-after-write ordering, versioned keys (`user:42:v7`), or write-through for hot keys.

### Cache stampede (thundering herd) — and `singleflight`
When a hot key expires, **every** concurrent request misses at once and hammers the DB with identical loads — sometimes enough to knock it over. `golang.org/x/sync/singleflight` collapses concurrent identical loads into **one** in-flight call; the rest wait and share the result:
```go
var g singleflight.Group

func (c *UserCache) Get(ctx context.Context, id string) (User, error) {
    if u, ok := c.store.Get(ctx, "user:"+id); ok {
        return u, nil
    }
    // Only ONE goroutine per key actually hits the DB; others wait for its result.
    v, err, _ := g.Do("user:"+id, func() (any, error) {
        u, err := c.db.GetUser(ctx, id)
        if err == nil {
            c.store.Set(ctx, "user:"+id, u, jitter(5*time.Minute))
        }
        return u, err
    })
    if err != nil {
        return User{}, err
    }
    return v.(User), nil
}
```
Other stampede defenses: **jittered TTL** (spread expiries), **early/background recompute** (refresh before expiry), and a **short lock** per key. (You met `singleflight` in [15](15-sync-context.md); this is its canonical use.)

### Local vs distributed — and two-tier (L1/L2)
- **Local (in-process)** — a Go map/LRU in the service. Nanosecond access, no network, but **per-instance** (each replica has its own, so invalidation is hard) and lost on restart. Great for tiny, hot, rarely-changing data (feature flags, config).
- **Distributed (Redis/Memcached)** — shared across all instances, survives restarts, one place to invalidate. Costs a network hop and is another dependency to run (and to make resilient — [36](36-resilience-patterns.md); a Redis outage must degrade to hitting the DB, not fail the request).
- **Two-tier (L1 local + L2 Redis)** — check local, then Redis, then DB; populate both. Cuts Redis load and latency for the hottest keys, at the cost of cross-instance staleness in L1 (use short L1 TTLs).

A minimal safe in-memory TTL cache (concurrent):
```go
type entry struct{ val any; exp time.Time }
type TTLCache struct {
    mu sync.RWMutex
    m  map[string]entry
}
func (c *TTLCache) Get(k string) (any, bool) {
    c.mu.RLock(); e, ok := c.m[k]; c.mu.RUnlock()
    if !ok || time.Now().After(e.exp) {   // treat expired as miss (lazy expiry)
        return nil, false
    }
    return e.val, true
}
func (c *TTLCache) Set(k string, v any, ttl time.Duration) {
    c.mu.Lock(); c.m[k] = entry{v, time.Now().Add(ttl)}; c.mu.Unlock()
}
```

### Negative caching & penetration
Caching **misses** (a short-TTL "not found") prevents a stream of requests for a non-existent key from hammering the DB every time (**cache penetration**). Keep the negative TTL short so a later-created record appears quickly, and guard against attackers probing random keys.

### HTTP caching is caching too
Don't forget the layer above your app: `ETag`/`If-None-Match` (conditional `304 Not Modified`) and `Cache-Control` let browsers, CDNs, and proxies cache responses so requests never reach you. That's the cheapest cache of all — covered on the API side in [41](41-api-design-evolution.md).

## Exercises
1. Implement cache-aside `Get` (check → miss → load → populate with TTL) over the in-memory `TTLCache` above and a fake DB; count DB hits and confirm the second read is a cache hit.
2. Add invalidation: on `Update`, delete the key; prove the next read reloads. Then construct the cache-aside stale-set race in a comment and pick a mitigation.
3. Cause a stampede: 100 goroutines request the same just-expired key; count DB loads (≈100). Wrap the loader in `singleflight.Group.Do` and confirm the DB is hit **once**.
4. Add jitter to your TTLs; expire 1000 keys and show their expiries spread over a window instead of firing together.
5. Implement write-through vs write-behind for a counter; kill the process before a write-behind flush and observe the lost increment — then state which data classes tolerate write-behind.
6. Build a two-tier cache (local `TTLCache` L1 + a fake Redis L2); trace a request through L1 miss → L2 hit and confirm L1 gets populated.
7. Add negative caching for "user not found" with a 10s TTL; show repeated lookups of a missing id hit the DB once, and that creating the user becomes visible after the TTL.

## Best Practices & Pitfalls
- **The cache is never the source of truth.** Design so a cold/failed cache means *slower*, never *wrong*. A Redis outage must fall back to the DB.
- **Always set a TTL, and jitter it.** A TTL is a safety net for invalidation you missed; jitter prevents synchronised expiry stampedes.
- **Prefer delete-on-write over update-on-write.** Fewer race windows; the next read repopulates cleanly.
- **Protect hot keys from stampede** with `singleflight` (or a lock / background refresh). One expiry shouldn't produce a load per concurrent request.
- **Pick the strategy on purpose.** Cache-aside by default; write-through when hot keys must never be stale; write-behind only for loss-tolerant data.
- **Pitfall — the stale-set race.** A slow reader can write back a value that a concurrent writer just invalidated. Short TTLs, versioned keys, or write-through for those keys.
- **Pitfall — unbounded local caches.** A map with no eviction is a memory leak. Bound size (LRU) and/or TTL-expire.
- **Pitfall — local caches and invalidation.** Per-instance caches can't be invalidated centrally; N replicas hold N stale copies. Use short TTLs for L1 or a distributed cache when correctness needs one source.
- **Pitfall — caching per-user/secret data by a shared key.** Key collisions leak one user's data to another. Include the user/tenant in the key and never cache secrets in shared caches.

## Checklist
- [ ] I can implement cache-aside with TTL and invalidate on write, and explain the stale-set race.
- [ ] I can name read-through / write-through / write-behind and choose one per data class.
- [ ] I can prevent a stampede with `singleflight` and jittered TTLs, and prove the DB is hit once.
- [ ] I can build a bounded, concurrent in-memory TTL cache and reason about eviction.
- [ ] I can compare local vs distributed vs two-tier and their invalidation/staleness trade-offs.
- [ ] I know why the cache must never be the source of truth and how a cache outage should degrade.

## Resources
- `golang.org/x/sync/singleflight`: https://pkg.go.dev/golang.org/x/sync/singleflight
- AWS Builders' Library, "Caching challenges and strategies": https://aws.amazon.com/builders-library/caching-challenges-and-strategies/
- Redis caching patterns & key eviction: https://redis.io/docs/latest/develop/reference/eviction/
- `go-redis` client (Redis in Go): https://redis.uptrace.dev/ · `hashicorp/golang-lru` (bounded LRU): https://github.com/hashicorp/golang-lru
- Prior art: `singleflight` example in [15 — Sync, Context & Patterns](15-sync-context.md).
- Next: [39 — Observability: Distributed Tracing](39-observability-tracing.md).
