# 48 — Low-Latency Go III: Lock-Free, Zero-Copy & Tail Latency

> Part 11, Low-Latency track: [46 Measuring & Allocation](46-low-latency-measuring.md) → [47 GC, Layout & Contention](47-low-latency-gc-contention.md) → **48 Lock-Free, Zero-Copy & Tail Latency**.
> The final lesson pushes the hot path to its limits — **no locks**, **no copies**, **no allocations** — and then zooms out to the number that actually reaches users: the **tail**. These are advanced, measure-or-don't-bother techniques. Reach for them only where a profile ([47](47-low-latency-gc-contention.md)) proves you need them.

## Goals
- Replace locks with **atomics** where it's safe: copy-on-write with `atomic.Pointer[T]`, CAS retry loops — and know when *not* to.
- Move bytes with **zero copies**: `io.Copy`/`ReaderFrom`, `bufio`, `net.Buffers`, and the safe use of `unsafe.String`/`unsafe.Slice`.
- **Amortise**: batch work, coalesce duplicate requests, and reuse ring buffers on the hot path.
- Engineer the **tail**: tame GC pauses, hedge slow calls, shed load, and stop one slow request blocking others.

## Concepts

### Lock-free with atomics — copy-on-write and CAS
A mutex is *pessimistic*: everyone stops so one goroutine can touch shared state. When state is **read far
more than written**, an *optimistic* approach wins — readers never block at all.

**Copy-on-write config** with `atomic.Pointer[T]` (Go 1.19+): readers atomically `Load` the current
snapshot; a writer builds a *new* value and `Store`s it. Readers are wait-free and always see a consistent
snapshot:
```go
type Config struct{ Timeout time.Duration; Hosts []string }

var cfg atomic.Pointer[Config]           // holds *Config

func Current() *Config { return cfg.Load() }        // readers: zero locks, zero blocking
func Reload(c *Config) { cfg.Store(c) }             // writer: swap in a fresh snapshot
```
The rule: **never mutate the pointed-to value after publishing it.** Readers may be looking; treat the
snapshot as immutable and replace it wholesale.

**CAS retry loops** (`CompareAndSwap`) build lock-free updates for the rare case that a plain `Add`/`Store`
can't express — read the current value, compute the new one, and swap *only if* nothing changed
underneath; retry if it did:
```go
func addFloat(addr *atomic.Uint64, delta float64) {
    for {
        old := addr.Load()
        nw := math.Float64bits(math.Float64frombits(old) + delta)
        if addr.CompareAndSwap(old, nw) {  // succeeds iff no one else changed it
            return
        }
    }
}
```
**When *not* to go lock-free:** CAS loops are subtle (the ABA problem, livelock under heavy contention,
memory-ordering reasoning). A `sync.Mutex` is fast, obvious, and correct. Use lock-free only for genuinely
hot, read-mostly, simple state — and only with a benchmark proving it beats the mutex. Idiomatic Go
prefers "share memory by communicating" (channels) and plain mutexes; lock-free is the exception.

### Zero-copy I/O
Every `string(b)`/`[]byte(s)` and every intermediate buffer is a copy. Cut them:
- **`io.Copy(dst, src)`** streams through a small fixed buffer instead of reading everything into memory —
  and if `dst` implements `ReaderFrom` or `src` implements `WriterTo`, it hands off to that and may copy
  *zero* times in user space (e.g. `sendfile`). Prefer `io.Copy` over `ReadAll`-then-`Write`.
- **`bufio.Reader`/`Writer`** amortise syscalls: one big read/write instead of thousands of tiny ones.
- **`net.Buffers`** does a vectored write (`writev`) of many `[][]byte` in one syscall — send a header +
  body without concatenating them first.
- **Append, don't concatenate** ([46](46-low-latency-measuring.md)): build responses with
  `strconv.Append*` into a reused buffer.

### `unsafe.String` / `unsafe.Slice` — the zero-copy conversion (Go 1.20+)
`string(b)` copies so the immutable string can't be changed through the slice. When you can *guarantee*
the bytes won't be mutated for the string's lifetime, `unsafe.String(&b[0], len(b))` reinterprets the same
memory with **no copy**:
```go
func asString(b []byte) string {
    if len(b) == 0 { return "" }
    return unsafe.String(&b[0], len(b))   // NO copy — b must not change afterwards
}
```
This is a **sharp knife**. Break the "don't mutate the backing bytes" promise and you get memory
corruption that violates Go's guarantee that strings are immutable. Confine it to a tiny, audited hot-path
helper, guard with tests, and never expose the alias to code that might write to it. Most code should keep
using safe `string(b)`; the compiler already elides the copy at the blessed sites
([46](46-low-latency-measuring.md) #8).

### Amortise: batch, coalesce, reuse
- **Batching** trades a little latency for a lot of throughput: group N items and pay the per-call cost
  (a syscall, a round trip, a lock) once. A `Flush` on a timer or a size threshold bounds the added
  latency. (This is the throughput-vs-latency tension from [46](46-low-latency-measuring.md).)
- **Request coalescing / `singleflight`** ([38](38-caching-patterns.md)): when many goroutines ask for the
  same thing at once, do the work **once** and share the result — collapses a stampede into a single call.
- **Ring buffers** reuse a fixed array as a queue (a bounded `[]T` with head/tail indices), so a
  producer/consumer hot path allocates nothing per item. A single-producer/single-consumer (SPSC) ring can
  even be lock-free with two atomic indices.

### Tail-latency engineering
The tail (p99/p99.9, [46](46-low-latency-measuring.md)) is dominated by *occasional* events, so the fixes
are about the occasional, not the average:
- **GC pauses.** Go's pauses are sub-millisecond, but GC *assist* ([47](47-low-latency-gc-contention.md))
  steals CPU from allocating goroutines during a cycle. Fewer allocations → fewer/shorter cycles → a
  quieter tail. Set `GOMEMLIMIT` to avoid emergency collections; the old "ballast" trick is obsolete.
- **Hedged requests.** For a read-only, idempotent call to a replicated backend, if the first attempt
  hasn't answered by ~p95, fire a **second** to another replica and take whichever returns first. A tiny
  fraction of extra load erases the long tail caused by one slow replica. (Cancel the loser via `context`.)
- **Deadlines everywhere** ([36](36-resilience-patterns.md)). A request with no deadline can sit in the
  tail forever; a bounded one fails fast and frees resources.
- **Load shedding** ([36](36-resilience-patterns.md)). Past a concurrency/latency threshold, reject new
  work (`429`/`503`) so in-flight work still meets its deadline. A fast rejection beats a slow timeout.
- **Head-of-line blocking.** One slow item on a shared queue/connection stalls everything behind it.
  Bound queues, use separate queues/pools per class of work (a bulkhead, [36](36-resilience-patterns.md)),
  and prefer HTTP/2-style multiplexing over a single serialized pipe.
- **`GOMAXPROCS` & the scheduler.** In a container, set `GOMAXPROCS` to your CPU *quota* (e.g. via
  `automaxprocs`) — a mismatch causes scheduler thrash and latency spikes. Keep syscalls/cgo off the hot
  path; they can park a whole OS thread.
- **Warm up.** First requests pay for cold caches, JIT-free but cold branch predictors, un-grown pools,
  and lazy connection setup. Pre-warm pools and connection pools before taking traffic.

### Transport-level latency: HTTP/2, QUIC & HTTP/3
Head-of-line blocking also lives *below* your code, in the transport. **HTTP/2** multiplexes many streams
over one TCP connection — but TCP delivers one ordered byte stream, so a *single* lost packet stalls
**every** stream behind it (TCP-level HOL blocking). **QUIC** (the transport under **HTTP/3**) runs over
UDP and gives each stream its own loss recovery, so one dropped packet only pauses its own stream — a big
tail win on lossy/mobile networks. QUIC also folds in latency features you'd otherwise hand-build:
- **0-RTT resumption** — a returning client sends application data *in the first flight*, skipping a round
  trip. Same rule as hedging and retries: **0-RTT data must be idempotent** (it's replayable).
- **Connection migration** — sessions are keyed by a *connection ID*, not the IP/port tuple, so they
  survive NAT rebinding and network switches (Wi-Fi → cellular) without a reconnect.
- **TLS 1.3 built into the handshake** — encryption setup overlaps the transport handshake instead of
  stacking on top of it, cutting setup round trips.

In Go, `net/http` already negotiates **HTTP/2** for you (TLS + ALPN, no code change). **HTTP/3/QUIC** isn't
in the standard library — use `github.com/quic-go/quic-go` and its `http3` server/client (`quic.ListenAddr`
/ `DialAddrEarly` for 0-RTT). The trade-off today: QUIC's stack runs in userspace over UDP, so it costs
more CPU per packet than kernel TCP, and it's an external dependency — adopt it where the *network* (loss,
mobility, connection-setup latency) is your tail, not where CPU or GC is.

### The mindset
Every technique here costs *simplicity*. The order of operations never changes: **measure
([46](46-low-latency-measuring.md)) → profile ([47](47-low-latency-gc-contention.md)) → find the hot path →
apply the smallest fix that moves the number → re-measure.** Lock-free code you didn't need is just a bug
you haven't hit yet.

## Exercises
1. Build a copy-on-write config with `atomic.Pointer[Config]`: many reader goroutines `Load` while one
   writer `Store`s new snapshots. Run under `-race` and confirm readers never block and never tear.
2. Implement `addFloat` with a `CompareAndSwap` retry loop; drive it from many goroutines and check the sum
   is exact. Add a counter for retries and watch it rise with contention.
3. Copy a large file two ways — `io.ReadAll`+`Write` vs `io.Copy` — and compare peak memory and allocations.
   Explain why `io.Copy` stays flat.
4. Write a `[]byte`→`string` helper with `unsafe.String`; benchmark it against `string(b)` for 0 allocs,
   then write the test that proves mutating the backing array after conversion is a bug (so you never do it
   in real code).
5. Add batching to a writer: buffer items and `Flush` on size **or** a timer; measure throughput and the
   worst-case added latency as you vary the batch size.
6. Implement an SPSC ring buffer with a fixed `[]T` and two indices; show it enqueues/dequeues with **zero**
   allocations per item.
7. Simulate hedged requests: a backend whose latency is usually 1ms but 5ms at p99; fire a hedge at 2ms and
   show the p99 of `min(first, hedge)` collapse. Cancel the loser with `context`.
8. Stand up an HTTP/3 server with `quic-go`'s `http3` package plus a matching client; compare
   connection-setup round trips (and 0-RTT resumption via `DialAddrEarly`) against your HTTP/2
   (`net/http`) server, and reason about why one stream's packet loss no longer stalls the others.

## Best Practices & Pitfalls
- **Default to mutexes and channels; go lock-free only where a benchmark proves it and the state is
  simple & read-mostly.** `atomic.Pointer` copy-on-write is the safe, common case.
- **Never mutate a value after publishing it via an atomic pointer.** Replace the whole snapshot.
- **Stream with `io.Copy`/`bufio`/`net.Buffers`; append into reused buffers.** Don't read whole payloads
  into memory to copy them.
- **Treat `unsafe.String`/`unsafe.Slice` as a last resort:** one audited helper, guaranteed-immutable
  bytes, tests. When unsure, use the safe conversion.
- **Batch to amortise, `singleflight` to coalesce, ring buffers to reuse** — each trades a little latency
  or complexity for a lot of throughput.
- **Engineer the tail deliberately:** `GOMEMLIMIT`, hedged idempotent reads, deadlines, load shedding, no
  head-of-line blocking, correct `GOMAXPROCS`, warm-up.
- **Pitfall — lock-free cargo-culting.** A CAS loop that's slower and buggier than a mutex is a pure loss.
  The ABA problem and memory ordering are real; most teams shouldn't hand-roll this.
- **Pitfall — `unsafe` string aliasing a buffer you later reuse.** The string silently changes; it's the
  worst kind of heisenbug. Never alias a `sync.Pool`/append buffer you'll overwrite.
- **Pitfall — hedging non-idempotent calls.** A hedged "charge card" charges twice. Hedge reads only.
- **Pitfall — batching without a time bound.** A size-only flush can hold the first item forever under low
  load. Always add a timer.
- **Pitfall — optimizing the mean, ignoring the tail.** Users live in your p99. A change that improves the
  average but adds a rare stall is a regression where it counts.

## Checklist
- [ ] I can build copy-on-write state with `atomic.Pointer[T]` and explain why readers never block.
- [ ] I can write a correct `CompareAndSwap` retry loop and say when a mutex is the better choice.
- [ ] I stream with `io.Copy`/`ReaderFrom`, `bufio`, and `net.Buffers` instead of buffering whole payloads.
- [ ] I know exactly when `unsafe.String`/`unsafe.Slice` is safe, and the corruption bug when it isn't.
- [ ] I can batch (with a time bound), coalesce with `singleflight`, and reuse a ring buffer.
- [ ] I can name and apply the tail-latency levers: GC/`GOMEMLIMIT`, hedging, deadlines, shedding, HOL
      blocking, `GOMAXPROCS`, warm-up.
- [ ] I can explain TCP-level head-of-line blocking, how QUIC/HTTP-3 avoids it, and QUIC's 0-RTT and
      connection-migration wins (plus the idempotency caveat on 0-RTT).
- [ ] I apply all of this only where a profile justifies it — measure, fix the hot path, re-measure.

## Resources
- Jeff Dean & Luiz Barroso, "The Tail at Scale" (hedged requests, tail tolerance): https://research.google/pubs/pub40801/
- `sync/atomic` (generic `Pointer[T]`, CAS): https://pkg.go.dev/sync/atomic
- `unsafe.String` / `unsafe.Slice`: https://pkg.go.dev/unsafe#String
- `io.Copy` / `ReaderFrom` / `WriterTo`: https://pkg.go.dev/io · `net.Buffers` (writev): https://pkg.go.dev/net#Buffers
- `golang.org/x/sync/singleflight`: https://pkg.go.dev/golang.org/x/sync/singleflight
- `uber-go/automaxprocs` (right-size `GOMAXPROCS` in containers): https://github.com/uber-go/automaxprocs
- goperf.dev, "QUIC in Go" (HTTP/3, 0-RTT, per-stream loss recovery, connection migration): https://goperf.dev/02-networking/quic-in-go/
- `quic-go` — QUIC & HTTP/3 for Go (`quic.ListenAddr`, `DialAddrEarly`, `http3`): https://github.com/quic-go/quic-go
- Examples: [examples/48-low-latency-lockfree-tail](examples/48-low-latency-lockfree-tail/).
- Prior art in this plan: atomics/channels/context in [15](15-sync-context.md); resilience (timeouts, shedding, bulkheads) in [36](36-resilience-patterns.md); `singleflight` in [38 — Caching](38-caching-patterns.md); GC & contention in [47](47-low-latency-gc-contention.md).
- This closes Part 11. Circle back to [46](46-low-latency-measuring.md) — every technique here is only worth applying once you've *measured* that you need it.
</content>
