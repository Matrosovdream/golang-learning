# 05 · In-Memory Cache with TTL

`🟢 easy` · *Topics:* LRU with a map and a linked list, lazy vs active expiry, eviction policies, sharding a lock, memory bounds, what Redis actually is · *Builds on:* [01 URL Shortener](01-url-shortener.md) (used a cache as a box; now we open the box)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [04 Unique ID Generator](04-unique-id-generator.md) · Next: [06 View & Like Counter](06-view-like-counter.md)

---

## 1. The task

> "Design an in-memory cache service, a small Redis or Memcached. Clients store a key with a value
> and an optional time-to-live. Memory is limited: when it fills up, evict something."

Every previous task drew a box labelled "cache". This one is about what is inside the box, and
why the box behaves the way it does.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| One node or a cluster? | A cluster adds consistent hashing and replication; a node is a data-structures problem. | One node first; sharding by the client is an extension. |
| Does data have to survive a restart? | A cache may lose everything; a store may not. | No. It's a cache. |
| Value sizes? Total memory? | Memory accounting and the number of entries. | Values 100 B–100 KB, average 1 KB; 8 GB budget. |
| Operations? | `GET`/`SET`/`DELETE` only, or counters and compare-and-set too? | `GET`, `SET` (with TTL, optional `NX`), `DELETE`, `INCR`, `TTL`. |
| Eviction policy? | LRU is the default; LFU and TTL-first are alternatives. | LRU, configurable. |
| TTL precision? | Millisecond precision versus "within a second or so". | Within a second is fine. |
| Latency and throughput? | Sub-millisecond rules out heavy locking. | p99 under 1 ms; 100k ops/s per node. |
| Protocol? | HTTP is convenient; a binary/text protocol over TCP is faster. | Simple text protocol over TCP (RESP-like); HTTP optional. |
| Concurrency? | Thousands of client connections hitting the same keys. | Yes, highly concurrent. |

## 3. Requirements

**Functional**
- `SET key value [EX seconds] [NX]`, `GET key`, `DEL key`, `INCR key`, `TTL key`.
- Expired keys are never returned.
- When memory is full, evict least-recently-used keys until the new entry fits.

**Non-functional**
- O(1) `GET`/`SET`; p99 < 1 ms; ~100k ops/s on one node.
- Memory stays under the configured limit.
- Thread-safe under thousands of concurrent connections.

**Out of scope**
- Persistence, replication, clustering, pub/sub, transactions, scripting.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Entries | ~6M | 8 GB ÷ (1 KB value + ~300 B overhead for key, pointers, metadata) |
| Ops | 100k/s | given; ~10 µs per op budget on one core |
| Expiry sweep | 6M entries × scan = too much per second | motivates sampling or a heap, not full scans |
| Connections | 10k | each needs a goroutine or an event loop slot |

What the numbers tell us: millions of entries and a 10 µs budget per operation mean every
operation must be O(1) with a short critical section, and expiry must never be "scan everything".

## 5. Plan

1. The core data structure for an LRU with O(1) everything.
2. TTL: how a key dies without a scan.
3. Memory accounting and the eviction loop.
4. Concurrency: one lock, many locks, or no locks.
5. The protocol and the client-side patterns the cache enables.
6. Scaling to more than one node (sketch only).

## 6. High-level design

```
 clients ──TCP──► connection handler (one goroutine per conn, or an event loop)
                         │ parse command
                         ▼
                   ┌──────────────────────────────────────────────────────┐
                   │  store = N shards, pick shard = hash(key) % N         │
                   │  shard: mutex + map[key]*entry + doubly linked list   │
                   │         (list order = recency; head = most recent)    │
                   │  entry: key, value, expiresAt, size, list node        │
                   └──────────────────────────────────────────────────────┘
                         ▲
    background: expiry sampler (every 100 ms, sample 20 keys/shard, delete expired)
                memory watchdog (evict from list tail when over the limit)
```

`GET`: hash to a shard, lock it, look up the map, check `expiresAt` (if past: delete and
miss), move the node to the head of the list, unlock, return. `SET`: same, plus memory
accounting and, if over the limit, pop from the tail until under.

## 7. API

Text protocol (RESP-style; one request, one response per line):

```
SET user:42 "{…}" EX 300        → OK
SET lock:job7 1 EX 30 NX        → OK | (nil)         ← acquire a lock if absent
GET user:42                     → "{…}" | (nil)
DEL user:42                     → 1 | 0
INCR views:post:9               → 118                 ← atomic
TTL user:42                     → 287 | -1 (no ttl) | -2 (missing)
```

Optional HTTP facade for debugging: `GET /keys/{key}`, `PUT /keys/{key}?ttl=300`. Not for the
hot path: HTTP parsing costs more than the cache operation itself.

## 8. Data model

```go
type entry struct {
    key       string
    value     []byte
    expiresAt int64         // unix nanos; 0 = never
    size      int           // len(key)+len(value)+overhead, for accounting
    node      *list.Element // position in the recency list
}

type shard struct {
    mu    sync.Mutex
    items map[string]*entry
    lru   *list.List        // front = most recently used, back = eviction candidate
    bytes int64             // current accounted memory
}
```

Memory is not measured; it is **accounted**: each entry's approximate size is added on `SET`
and subtracted on delete. Exact process memory includes allocator slack and fragmentation, so
set the limit at ~75% of the real budget.

## 9. Deep dives

### 9.1 LRU in O(1): a map plus a doubly linked list

The two operations that must be O(1): find by key, and "move this entry to most-recent /
remove the least-recent". A hash map gives the first. A doubly linked list gives the second:
unlink a node and push it to the front in constant time, pop the back in constant time. The
map's values point at list nodes so you never search the list.

| Structure | Lookup | Touch (move to front) | Evict oldest |
|---|---|---|---|
| map only | O(1) | can't track order | can't |
| slice/array of entries by time | O(n) | O(n) | O(1) |
| **map + doubly linked list** | O(1) | O(1) | O(1) |
| map + min-heap on last access | O(1) | O(log n) | O(log n) |

Go's `container/list` is this list. A hand-rolled intrusive list (prev/next pointers inside
`entry`) saves one allocation per entry and is what a production cache does.

### 9.2 Eviction policies

| Policy | Evicts | Good for | Weakness |
|---|---|---|---|
| **LRU** | not accessed for the longest time | temporal locality (most web caches) | a one-time scan of many keys (a backup, a crawler) flushes the hot set |
| **LFU** | accessed least often | stable popularity | a formerly hot key lingers; needs decay; more bookkeeping |
| **TinyLFU / W-TinyLFU** (Caffeine, ristretto) | admission filter: a new key must be more popular than the victim | highest hit rates in practice | complexity: frequency sketch + windows |
| FIFO | oldest inserted | simplicity | ignores reuse |
| Random | random key | nearly as good as LRU in some workloads, no bookkeeping | unpredictable |
| TTL-first (`volatile-ttl`) | the key closest to expiry | when TTLs encode value | only works if everything has a TTL |

**Choice: LRU**, since it's the expected default and the data structure is simple. Note that
Redis uses an *approximated* LRU (sample a few keys, evict the oldest of the sample) to avoid the
linked list's memory cost. Precision is not what makes LRU work; recency is.

### 9.3 Expiring keys without scanning

A naive "every second, walk all 6M entries and delete the expired ones" burns a core and holds
locks for tens of milliseconds. Three better options:

| Mechanism | How | Pros | Cons |
|---|---|---|---|
| **Lazy** | check `expiresAt` on access; delete if past | zero background cost | keys nobody reads again hold memory until evicted by LRU |
| **Active sampling** (Redis) | every 100 ms, sample 20 random keys that have a TTL; delete the expired ones; repeat immediately if more than 25% were expired | bounded CPU, no full scan, adaptive | probabilistic: a key can outlive its TTL by a little |
| **Min-heap / timing wheel by `expiresAt`** | pop expired entries from the top | precise, O(log n) per insert | the heap is one more structure to update on every `SET` with a TTL; a timing wheel is O(1) but coarse |

**Choice: lazy + active sampling.** Lazy guarantees correctness (an expired key is never
*returned*); sampling bounds memory. A heap is the right answer if exact expiry timing matters,
for example for scheduled jobs (task 15).

### 9.4 Concurrency: one lock is a bottleneck

With a single `sync.Mutex` around the whole store every operation serialises; at 10 µs each
that's a hard ceiling of ~100k ops/s and heavy contention from thousands of goroutines.

| Approach | Pros | Cons |
|---|---|---|
| one global mutex | trivial, exact LRU | contention; one core |
| `sync.RWMutex` | parallel reads | `GET` in an LRU is a *write* (it moves the node), so reads need the write lock anyway |
| **sharding**: N shards, each with its own lock, map and list | contention ÷ N; multiple cores | LRU becomes per-shard (approximately global, good enough); memory limit is per shard |
| lock-free structures | no blocking | complex; Go has no production-grade lock-free LRU in the stdlib |
| **single-threaded event loop** (Redis's answer) | no locks at all, every command atomic by construction | one core for commands; network I/O is threaded separately; fine because the network, not the CPU, is usually the limit |

**Choice: sharding** (e.g. 256 shards). It keeps the code a plain mutex-and-map and spreads the
load across cores. The interview insight to say out loud: Redis chose single-threaded on
purpose because it makes `INCR`, `SET NX` and every other command atomic for free, which is
why it's safe to build locks and counters on it.

### 9.5 What the cache enables on the client side

- **Cache-aside** (task 01): read cache, miss → read DB → `SET … EX`. Always a TTL, so a bug
  can't pin stale data forever.
- **Stampede protection:** when a hot key expires, a thousand requests miss at once and all hit
  the database. `singleflight` on the client collapses them to one load (lesson 38); or
  "probabilistic early refresh" re-fetches slightly before expiry.
- **Locks and leases:** `SET lock NX EX 30` is a lock with a safety timeout. Release with a
  compare-and-delete (only if the value is still *my* token), otherwise you delete someone else's lock.
- **Counters:** `INCR` is atomic on the server, so twenty API servers can count into one key (task 06).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 1M ops/s needs more than one node. Clients shard by key with
  **consistent hashing** (a ring, virtual nodes) so adding a node moves only ~1/N of the keys
  instead of invalidating everything. That is the medium-track "distributed cache" task.
- **Memory limit vs real RSS:** fragmentation and allocator overhead mean accounted bytes
  underestimate real usage. Watch RSS; leave headroom.
- **Hot key:** a single key at 500k reads/s pins one shard (or one node). Replicate the key
  client-side (`key#1..#8`) or add a tiny in-process cache in the clients.
- **Big values:** a 100 MB value blocks a shard while copying. Cap value size (Memcached: 1 MB default).
- **Node dies:** everything in it is gone; clients see misses and the database takes the load. Warm
  slowly, or run replicas. This is why a cache must never be the only copy.
- **GC pauses:** millions of pointers in maps are GC scan work (lesson 47). Store values in big
  byte arenas or use off-heap tricks (what `bigcache`/`freecache` do) if pauses show up in p99.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| map + doubly linked list | heap by last access | O(1) touch and evict | 2 pointers per entry |
| LRU | LFU / TinyLFU | simplicity, predictable | hit rate under scan-heavy workloads |
| lazy + sampled expiry | heap of expirations | bounded CPU, simple | TTL precision within ~100 ms |
| 256 lock shards | one global lock | multi-core throughput | exact global LRU ordering |
| accounted memory | measuring RSS | cheap per-op | must leave headroom for allocator slack |

## 12. Check yourself

1. Why does an LRU cache need a linked list, and why does a `GET` take the write lock?
2. What is wrong with a goroutine that scans all keys every second to delete expired ones, and what does Redis do instead?
3. How does sharding change what "least recently used" means?
4. Why is `SET … NX EX` enough for a lock, and what is the bug in `DEL lock` to release it?
5. Your cache node restarts and is empty. What happens to the database, and what are two mitigations?

<details><summary>Answers</summary>

1. To evict the least-recently-used entry in O(1) you need an ordered structure where "move to
   front" and "remove from back" are constant time; a doubly linked list gives that, with the map
   pointing at list nodes. A `GET` moves the node to the front, which mutates the list, so it is a
   write under the lock.
2. It walks millions of entries while holding locks: a CPU spike and latency stall every second,
   regardless of how many keys are actually expired. Redis samples 20 random keys with TTLs every
   100 ms, deletes the expired ones, and repeats only while the sample's expiry ratio is high; plus
   lazy deletion on access guarantees an expired key is never returned.
3. Each shard tracks recency only among its own keys and evicts from its own tail. Globally, an
   entry in a cold shard may survive while a more recently used entry in a hot shard is evicted. It is
   an approximation of LRU, which is fine because recency, not exact ordering, drives the hit rate.
4. `NX` makes it atomic "set only if absent", and `EX` bounds the lock if the holder dies. A bare
   `DEL` releases whoever's lock is there: if your lock expired and another client acquired it, you
   delete *their* lock. Release must be "delete only if the value equals my token" (a Lua script).
5. Every read misses and goes to the database: a thundering herd that can take the database down
   (a cold-cache outage). Mitigate with gradual warm-up (rate-limit misses / `singleflight` per key),
   replicas so one node's loss costs 1/N of the hit rate, or persistence to reload from a snapshot.

</details>

## 13. In Go

- Related lessons: [38 Caching patterns](../../38-caching-patterns.md) (cache-aside, TTL with an injectable clock, LRU, mini `singleflight`), [15 Sync & context](../../15-sync-context.md) (mutex, `RWMutex`, the read-through cache example), [47 Low-latency II](../../47-low-latency-gc-contention.md) (sharding a lock, pointer-heavy maps as GC work), [42 Trees](../../42-trees.md) and [52 Heaps](../../52-heaps-priority-queues.md) for the heap alternative.
- Build it in steps: `map` + `container/list` LRU (unbounded) → add `expiresAt` + lazy check → add byte accounting + evict loop → wrap in 256 shards → add the sampler goroutine with a `time.Ticker` and `context` for shutdown.
- Packages: `container/list`, `hash/fnv` or `hash/maphash` for shard selection, `golang.org/x/sync/singleflight`; production options: `hashicorp/golang-lru/v2`, `dgraph-io/ristretto` (TinyLFU), `allegro/bigcache` (GC-friendly).

## 14. Extensions

- Implement **W-TinyLFU** admission and measure hit rate vs LRU on a Zipfian trace.
- **Consistent hashing** client with virtual nodes; add and remove nodes and count moved keys (medium: distributed cache).
- **Persistence:** periodic snapshot (RDB-style) vs append-only log (AOF); what does "cache" mean once it persists?
- **GC-friendly storage:** byte arena + offsets instead of `map[string]*entry`; compare pause times with `GODEBUG=gctrace=1`.
- **Pipelining** in the protocol: many commands per round trip; measure throughput at 1 vs 100 per batch.
- **Hot-key detection:** sample access counts, log the top-10 keys per minute.
