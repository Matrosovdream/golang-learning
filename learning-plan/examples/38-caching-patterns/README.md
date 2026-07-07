# Step 38 — Caching Patterns · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[38-caching-patterns.md](../../38-caching-patterns.md): cache-aside, TTL & invalidation, write-through
/ write-behind, LRU eviction, stampede protection with a mini `singleflight`, and two-tier caching.

## One-time setup

```bash
mkdir -p /tmp/cache-ex && cd /tmp/cache-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** is real stdout.
Standard-library only, and **deterministic**: the TTL cache uses an injectable clock, jitter seeds
`math/rand`, and the concurrency examples (9, 10) synchronize with a barrier so their counts are
stable (verified under `-race`). Examples 9–10 use `for range 100` → **Go 1.22+**.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — cache-aside, TTL, invalidation
- [1. Cache-aside (lazy loading)](1-easy.md#1-cache-aside-lazy-loading)
- [2. Hits vs misses](1-easy.md#2-hits-vs-misses)
- [3. Invalidate on write](1-easy.md#3-invalidate-on-write)
- [4. A TTL cache](1-easy.md#4-a-ttl-cache)
- [5. Negative caching](1-easy.md#5-negative-caching)

### 🟡 [Medium](2-medium.md) — write policies, LRU, stampede
- [6. Write-through](2-medium.md#6-write-through)
- [7. Write-behind](2-medium.md#7-write-behind)
- [8. LRU eviction](2-medium.md#8-lru-eviction)
- [9. Cache stampede](2-medium.md#9-cache-stampede)
- [10. singleflight collapses the herd](2-medium.md#10-singleflight-collapses-the-herd)

### 🔴 [Hard](3-hard.md) — two-tier, races, capstone
- [11. Two-tier cache (L1 + L2)](3-hard.md#11-two-tier-cache-l1--l2)
- [12. Jittered TTLs](3-hard.md#12-jittered-ttls)
- [13. The stale-set race](3-hard.md#13-the-stale-set-race)
- [14. Namespace keys by tenant](3-hard.md#14-namespace-keys-by-tenant)
- [15. Capstone: cache-aside + TTL + metrics](3-hard.md#15-capstone-cache-aside--ttl--metrics)
