# 04 · Unique ID Generator

`🟢 easy` · *Topics:* UUID v4 vs v7, Snowflake bit layout, database sequences and ticket servers, range leasing, clock skew, index locality · *Builds on:* [01 URL Shortener](01-url-shortener.md) (needed an ID source)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [03 Rate Limiter](03-rate-limiter.md) · Next: [05 In-Memory Cache with TTL](05-kv-cache-ttl.md)

---

## 1. The task

> "Design a service that generates unique IDs for the objects in our system: orders, messages,
> events. There are many servers, and ideally they shouldn't have to coordinate on every request."

A small problem that reveals whether you understand clocks, coordination, and what databases
do with your keys.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Unique across the whole system, or per table? | Global uniqueness removes a class of bugs when IDs cross service boundaries. | Global. |
| Must IDs be numeric? Fit in 64 bits? | 128-bit UUIDs don't fit `BIGINT`; strings are bigger in indexes. | Prefer 64-bit integers. |
| Should IDs be roughly sortable by creation time? | "k-ordered" IDs give sequential index inserts and let you sort by ID. | Yes, time-ordered within about a second. |
| How many IDs per second? | A single generator caps out somewhere. | 10k/s today, design for 100k/s. |
| May IDs be predictable? | Sequential IDs leak volume ("order #1,000,001") and allow enumeration. | Not a secret, but don't expose business volume in public URLs (use a separate public code if needed). |
| Must a server generate IDs when the network is down? | Rules out a central ticket server as the only path. | Preferably yes. |
| Strictly monotonic? | Two servers can't be strictly ordered without coordination. | Monotonic *per generator* is enough. |

## 3. Requirements

**Functional**
- Produce globally unique IDs on request.
- IDs are 64-bit integers, increasing with time to within a small skew.

**Non-functional**
- 10k/s sustained, 100k/s possible; no network call on the request path.
- No two generators ever collide, including after restarts and clock adjustments.
- Simple to operate: no new stateful system if we can avoid it.

**Out of scope**
- Human-friendly codes (base62, task 01), per-tenant obfuscation, cryptographic unpredictability.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Required rate | 10k/s → 100k/s | given |
| Snowflake capacity per node | 4,096/ms = 4.1M/s | 12 sequence bits |
| Node count the layout supports | 1,024 | 10 machine bits |
| Lifetime of a 41-bit millisecond timestamp | ~69 years | 2⁴¹ ms |
| 64-bit integer in Postgres | 8 B + 8 B in each index | vs 16 B for a UUID, and 36 B if stored as text |

What the numbers tell us: one Snowflake-style node is already 40× over the target, so the
design is about **uniqueness guarantees and clock behaviour**, not throughput.

## 5. Plan

1. List the candidate schemes and kill the ones that violate a requirement.
2. Lay out the 64 bits and check the capacity math.
3. Solve the two operational problems: assigning machine IDs and handling clocks.
4. Decide how it's deployed: library or service.
5. Note what IDs do to your database indexes, since that is why we care about ordering.

## 6. High-level design

```
 Option A — library in every service (chosen):
   service process ──► idgen.Next()  = [ 41 bits time | 10 bits node | 12 bits seq ]
                           ▲ node id from: static config / pod ordinal / etcd lease at startup

 Option B — ID service:
   service ──gRPC──► idgen service (N replicas, each with its own node id) ──► ids
                     (needed only if clients can't be trusted to hold a unique node id)
```

Each process is assigned a node ID once at startup. From then on it generates IDs with no I/O:
read the clock, bump a sequence, pack the bits. Uniqueness comes from the node ID being unique
and the sequence resetting every millisecond.

## 7. API

In-process:

```go
gen, err := idgen.New(nodeID)   // nodeID 0..1023, must be unique among live generators
id := gen.Next()                // int64, k-ordered; safe for concurrent use
t := idgen.Time(id)             // decode the timestamp back out
```

As a service (only if needed):

```
POST /ids?count=100  → 200 { "ids": [ … ] }      batch to amortise the round trip
```

## 8. Data model

There is no data model for the IDs themselves. The only persistent state is **node ID
assignment**:

```sql
-- if using a database to lease node ids
CREATE TABLE idgen_nodes (
    node_id     SMALLINT PRIMARY KEY CHECK (node_id BETWEEN 0 AND 1023),
    owner       TEXT NOT NULL,              -- hostname / pod
    lease_until TIMESTAMPTZ NOT NULL
);
```

Or an etcd lease, or nothing at all when the node ID is derived from a stable identifier
(StatefulSet ordinal, a config file per host).

## 9. Deep dives

### 9.1 The candidates

| Scheme | Size | Time-ordered | Coordination | Pros | Cons |
|---|---|---|---|---|---|
| **DB `BIGSERIAL`** | 64 | yes, strictly | every insert goes to one primary | zero code; strictly monotonic | the primary is the bottleneck and the single point of failure; ID exists only after the insert; leaks volume |
| **Multi-master with step/offset** (server A: 1,3,5…; B: 2,4,6…) | 64 | roughly | none at request time | survives one DB | adding a server means changing every step; IDs from different servers interleave oddly |
| **Ticket server** (Flickr: a tiny MySQL doing `REPLACE INTO` on one row) | 64 | yes | one network call per ID (or per batch) | simple, battle-tested | SPOF unless two servers (odd/even); network on the request path |
| **Range leasing** (each node takes a range of 1,000 IDs from the DB, hands them out locally) | 64 | roughly: ranges interleave | one DB call per 1,000 IDs | DB load tiny; works offline until the range runs out | IDs from two nodes are not time-ordered relative to each other; gaps on restart |
| **UUID v4** | 128 | **no** | none | nothing to run, no clocks; generated anywhere, even in the browser | random keys scatter B-tree inserts across the whole index: poor cache locality and larger indexes; 16 bytes |
| **UUID v7** (RFC 9562, 2024) | 128 | yes (ms precision + random) | none | sortable *and* coordination-free; now the standard "just use this" | still 128-bit; the random part means no monotonicity within a millisecond unless the library adds a counter |
| **ULID / KSUID** | 128 | yes | none | lexicographically sortable strings | non-standard; same size cost as UUID |
| **Snowflake** (Twitter) | 64 | yes | node ID assigned once | fits `BIGINT`, k-ordered, millions/s/node, no I/O at request time | needs unique node IDs; sensitive to the clock going backwards; a custom epoch |

**Choice: Snowflake** for 64-bit, k-ordered IDs generated in-process. If the 64-bit requirement
were dropped, **UUID v7** is the pragmatic default: no node IDs to manage and still index-friendly.
`BIGSERIAL` remains the right answer for a single-database application that doesn't need IDs
before insert; don't over-engineer it.

### 9.2 Snowflake bit layout

```
 63   62 ............................ 22   21 ........ 12   11 ........ 0
 ┌───┬────────────────────────────────────┬───────────────┬───────────────┐
 │ 0 │  41 bits: ms since custom epoch    │ 10 bits: node │ 12 bits: seq  │
 └───┴────────────────────────────────────┴───────────────┴───────────────┘
   sign (always 0, keeps the number positive)   1,024 nodes   4,096 per ms per node
```

- `id = (ms << 22) | (node << 12) | seq`.
- The **custom epoch** (e.g. 2026-01-01) starts the 69-year clock when the system starts, not in 1970.
- Within one millisecond, `seq` increments. If it overflows 4,095, spin until the next millisecond.
- Ordering: IDs from one node are strictly increasing; across nodes they are ordered to within
  clock skew (a few ms with NTP). That is "k-ordered", and it is what index locality needs.

Different systems shift the split: Instagram uses 41 time / 13 shard / 10 sequence inside a
Postgres function per shard; Discord uses 42 / 5 worker / 5 process / 12 sequence. The idea is
the same; the split matches your fleet size and burst rate.

### 9.3 Assigning node IDs

Two live generators with the same node ID produce colliding IDs in the same millisecond. This is
the only way Snowflake fails, so it's the thing to get right.

| Method | Pros | Cons |
|---|---|---|
| Static config per host | no infrastructure | humans forget; autoscaling breaks it |
| Derive from a stable identity (StatefulSet pod ordinal, last bits of a private IP) | automatic | IP-derived IDs can collide across subnets; ordinals only exist in StatefulSets |
| **Lease from a coordination store** (etcd/ZooKeeper, or a DB table with `lease_until`) at startup, renew in the background | works with autoscaling; a dead node's ID is reclaimed after the lease expires | one dependency at startup only; must stop generating if the lease can't be renewed |

**Choice:** lease at startup; the request path still has no I/O. If the lease cannot be renewed,
the node keeps generating for a grace period, then refuses (fail closed): a duplicate ID is far
worse than a `503`.

### 9.4 Clocks

- **Clock moves backwards** (NTP step, VM migration): the same millisecond would be reused with a
  reset sequence and collide with IDs already issued. Options: refuse to generate until the clock
  catches up (if the jump is small, wait; if large, error and alert), or keep going with the
  *last* timestamp and let the sequence absorb it (works until 4,096 IDs pile up). Always track
  `lastTimestamp` and never go below it.
- **Leap seconds / slewing:** NTP slewing (speeding or slowing the clock) is fine; stepping is the
  danger. Configure NTP to slew, and use a monotonic reading for the "don't go backwards" check.
- **Sequence exhaustion:** 4,096 IDs in one millisecond on one node means waiting up to 1 ms.
  At 100k/s across 10 nodes it never happens.

### 9.5 Why ordering matters: your database index

Primary keys live in a B-tree. Inserting increasing keys appends to the rightmost leaf: the
page is hot in cache, pages fill completely, the index stays compact. Inserting random keys
(UUID v4) touches a random page each time: cache misses, half-empty pages after splits, and an
index that can be 2× larger and slower to write. On a table with 1B rows the difference is
measurable in both write throughput and cache hit rate. Sortable IDs (Snowflake, UUID v7) keep
the append pattern; that is their main point beyond "I can sort by ID".

One caveat: a single hot rightmost page can be a contention point in some databases under very
high insert rates; it is rarely the first bottleneck.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** nothing. 1M/s is 100k/s per node across 10 nodes, under 3% of a
  node's 4.1M/s ceiling.
- **Node-ID exhaustion:** 1,024 generators. Rebalance bits (fewer sequence bits, more node bits)
  before you get there; the layout is a deployment constant.
- **Coordination store down:** running nodes are unaffected; new nodes can't start. Acceptable for
  short outages; pre-assign IDs for critical services if not.
- **A duplicated node ID (misconfiguration):** silent duplicates. Add a startup check that the
  leased ID isn't held, and a reconciliation query for duplicate primary keys as a canary.
- **Exposing IDs:** a Snowflake ID reveals the creation time, which is sometimes a privacy issue.
  Use a separate opaque public identifier where that matters (task 01's codes).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Snowflake in-process | UUID v4 | 64-bit, k-ordered, index-friendly | node-ID management and clock handling |
| Snowflake | UUID v7 | half the size | the zero-coordination simplicity of v7 |
| lease node IDs | static config | autoscaling works | a startup dependency |
| fail closed on lease loss | keep generating | no duplicates ever | a possible `503` |
| custom epoch | Unix epoch | 69 years from *now* | IDs are not meaningful outside this system |

## 12. Check yourself

1. Why do random UUIDs hurt a database index, and which property of Snowflake/UUID v7 fixes it?
2. Two nodes end up with the same node ID. What exactly happens, and how do you prevent it?
3. The clock jumps back 2 seconds on one node. What does the generator do and why?
4. When is a plain `BIGSERIAL` the right answer?
5. Decode: how would you get the creation time back out of a Snowflake ID?

<details><summary>Answers</summary>

1. Random keys land on random B-tree pages: every insert is a likely cache miss and causes page
   splits that leave pages half full, so the index is larger and slower. Time-ordered IDs append to
   the rightmost page, which stays hot and fills completely.
2. In any millisecond both are active, they produce identical `(time, node, seq)` triples:
   duplicate primary keys, or worse, silently overwritten data in a KV store. Prevent it by leasing
   IDs from a coordination store (or deriving from a provably unique identity) and refusing to start
   without a lease.
3. It must not reuse timestamps it has already issued. It keeps `lastTimestamp`, notices
   `now < last`, and either waits for the clock to catch up (small jump) or returns an error and
   alerts (large jump). Generating with the old clock would collide with IDs from 2 seconds ago.
4. One application, one database, IDs needed only after insert, modest write rate, no plan to
   shard: `BIGSERIAL` is simpler than anything here and strictly monotonic. Add a public opaque
   code if enumeration is a concern.
5. `ms = id >> 22`, then `epoch + ms`. The node is `(id >> 12) & 0x3FF`, the sequence `id & 0xFFF`.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) (a mutex or CAS loop around `lastTimestamp` and `seq`), [47 Low-latency II](../../47-low-latency-gc-contention.md) (atomics vs mutex for a hot generator), [04 Types](../../04-types-constants.md) (bit shifting and masks on `int64`/`uint64`).
- Implementing `Next()` is a 30-line exercise: lock, read `time.Now().UnixMilli()`, compare with `last`, bump or reset `seq`, pack. Then write the table test for "same ms → seq increments" and "clock went back → error".
- Packages: `github.com/bwmarrin/snowflake`, `github.com/google/uuid` (`uuid.NewV7()` since v1.6), `github.com/oklog/ulid/v2`, `github.com/sony/sonyflake`.

## 14. Extensions

- Put a **shard key** into the ID (Instagram style) so the ID alone tells you which database shard holds the row.
- A **lock-free** generator with a single `atomic.Int64` holding `(timestamp, seq)` and a CAS loop (lesson 48); benchmark it against the mutex version.
- **Obfuscated public IDs**: a bijective mix (Feistel or multiply-by-odd-constant) so public URLs don't leak volume; compare with hashids.
- **Range leasing** implementation against Postgres (`UPDATE … RETURNING`), and what happens to ordering.
- Write the base62 wrapper and connect it back to task 01.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Snowflake generator (library, chosen) | 100k IDs/s peak (10k/s today); one node's ceiling is 4.1M/s | CPU, negligible | `Next()` = mutex + `time.Now()` + shifts; assume 0.2 µs ⇒ 100k × 0.2 µs = 0.02 vCPU | no instances: it runs inside the services that already exist | 0 extra | nothing to buy: no network call on the request path (section 3) and one node is ~40× over the 100k/s target. Choosing a library over a service is what makes the hardware line zero |
| Node-ID lease store | one write at start-up and one renewal per node every ~10 s (assumed): 50 nodes ≈ 5 writes/s, worst case 1,024 nodes ≈ 100/s | none | trivial row updates on `idgen_nodes` | a table in the existing Postgres/RDS (alternatives: DynamoDB conditional write; self-run etcd, 3 × `m7g.large` ≈ $180) | 0 new | a start-up and renewal dependency only (section 10), so a table in a database that already exists is enough; three etcd nodes for 5 writes/s would be the dearest part of this lesson |
| ID service (Option B only) | 100k IDs/s in batches of 100 = 1k calls/s | CPU, tiny | 1k × 0.2 ms (HTTP + JSON of 100 numbers) = 0.2 vCPU. N = ceil(0.2 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS | 3, one per zone, each with its own node ID | availability, not load: one node could serve ~5× this. `c`, since memory use is ~100 MiB of 4 GiB. Needed only if clients can't be trusted to hold a unique node ID |

Where the cost really sits: in the keys, not the generator. Every ID becomes a key in each index it appears in. At 10k IDs/s ≈ 315B IDs a year, storing 8 B (Snowflake) instead of 16 B (UUID) saves 8 B × 315B ≈ 2.5 TB per index per year, ≈ $200 a month of `gp3` per index at the end of year 1 (× 2 with a Multi-AZ standby). Random UUID v4 also splits pages and bloats the index (section 9.5).

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| ID generation: in-process library (~0.02 vCPU inside existing services) | $0 |
| Node-ID leases: one table in the existing database | $0 |
| ID service, Option B only: 3 × `c7g.large` (3 × $53) | $160 |
| **Total** | **≈ $0** for the chosen design (≈ $160 with Option B, ≈ $100 with a 1-year commitment) |

- **Biggest line and why:** there is none. This lesson needs no dedicated hardware; the spend that depends on it is key width in the databases that store the IDs (above). If you do run Option B, its three nodes are bought for availability: a single `c7g.large` would carry the load 5× over.
- **What changes at 10×:** 1M IDs/s is 1M × 0.2 µs = 0.2 vCPU in-process, spread over the services, or 10k calls/s × 0.2 ms = 2 vCPU for Option B ⇒ N = ceil(2 ÷ 1.07) = 2 → 3, still 3 × `c7g.large`. Nothing runs out in hardware. The first limit is the 1,024-node ID space (section 10), fixed by re-splitting the bits; the lease store still sees ≤ 100 writes/s.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- No database on the request path: IDs are generated in-process at 10k/s, up to 100k/s, against a 4.1M/s ceiling per node (section 4), so the generator needs no store for the IDs themselves.
- The only persistent state is node-ID assignment: `idgen_nodes`, at most 1,024 rows, written once at start-up and renewed about every 10 s per node, so ~5 writes/s for 50 nodes and ~100/s in the worst case (section 15).
- The real database effect is on the databases that store the IDs: an 8-byte time-ordered key appends to the right of a B-tree; a 16-byte random UUID lands on random pages and is repeated in every index (sections 4, 9.5).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for the lease table | the table goes into the database the services already run (no new instance, section 15); a lease claim is one conditional `UPDATE … WHERE lease_until < now() … RETURNING node_id`, with `FOR UPDATE SKIP LOCKED` so two nodes starting together take different rows. `BIGSERIAL` is also the right ID source when one database is the only place IDs are needed (9.1) |
| MySQL (RDS / Aurora) | ✔ works, interchangeable | the same table and conditional `UPDATE` (no `RETURNING`: read the row back by `owner`). Its Flickr-style ticket server, a one-row `REPLACE INTO`, is the option 9.1 rejects for putting a network call on every ID. As the store of the IDs the engine matters a little more: the row is the primary-key tree, so a random UUID v4 key scatters inserts across the whole table, and every secondary index carries a copy of the key |
| DynamoDB | ✔ works (section 15's alternative) | a conditional put (`node_id` absent, or `lease_until` in the past) is the lease; ~100 writes/s worst case is far under ~1,000 per partition. Not worth a second database just for 5 writes/s if a relational one exists |
| etcd / ZooKeeper | ✔ works, dear for the job | native leases with keep-alive and expiry are 9.3's first option, but three `m7g.large` nodes (≈ $180 a month, section 15) for ~5 writes/s is the priciest part of this lesson |
| Redis (ElastiCache) | ✖ poor fit as the lease authority | `SET NX EX` is a lease, but replication is asynchronous (task 03 accepts that for rate limits): a failover can forget a held lease and hand the same node ID to a second generator, i.e. duplicate IDs, which 9.3 calls far worse than a `503` |

**Pick:** no database generates the IDs: a Snowflake library in each service (zero extra instances, section 15), and the lease table in the existing PostgreSQL on RDS Multi-AZ. The lease table is the record of which node IDs are live; the IDs themselves are stored wherever they are used. The engine does not matter for a 1,024-row table: use whatever the services already run. What matters is the key you hand the other databases: 8 bytes and time-ordered.

**If it were MySQL instead:** nothing material for the lease table (claim with `UPDATE … ORDER BY node_id LIMIT 1`, then read back by `owner`). Snowflake `BIGINT` keys work unchanged. If you chose UUIDs, store them as `BINARY(16)` and prefer v7: a random v4 primary key hurts InnoDB at least as much as Postgres, since the table itself is ordered by it.

**What would flip the choice:** IDs that must be strictly increasing and gap-free (invoice numbers, task 20) need a database sequence or counter row, not Snowflake. With no relational database in the platform, lease through a DynamoDB conditional write, or derive the node ID from a StatefulSet ordinal and drop the store. More than 1,024 generators means re-splitting the bits (section 10), not another store.
