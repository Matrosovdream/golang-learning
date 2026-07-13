# 47 — Low-Latency Go II: GC Pressure, Memory Layout & Contention

> Part 11, Low-Latency track: [46 Measuring & Allocation](46-low-latency-measuring.md) → **47 GC, Layout & Contention** → [48 Lock-Free, Zero-Copy & Tail Latency](48-low-latency-lockfree-tail.md).
> Lesson 46 taught you to *allocate less*. This lesson is about the three forces that turn allocations into latency: the **garbage collector** (how it works and how to feed it less), **memory layout** (laying data out for the CPU cache), and **lock contention** (how goroutines stall on each other) — and the **profilers** that show you which one is hurting you.

## Goals
- Explain Go's GC well enough to reason about pauses: pacing, `GOGC`, `GOMEMLIMIT`, and why **pointers are the cost**.
- Cut GC work by reducing pointers, reusing objects with `sync.Pool` **correctly**, and laying out structs for the cache.
- Diagnose and fix **lock contention** with `RWMutex`, sharding, atomics, and cache-line padding for false sharing.
- Drive `pprof` (CPU, heap, mutex, block) and the execution tracer to find the real bottleneck before changing code.

## Concepts

### How Go's garbage collector actually works
Go's GC is a **concurrent, tri-colour mark-and-sweep** collector. It is **non-generational** and
**non-compacting** (objects don't move, so pointers stay valid and there's no copying phase). Crucially it
runs *concurrently with your program* — the "stop-the-world" (STW) pauses are tiny (sub-millisecond,
just to start and stop the mark phase); the marking itself happens while your goroutines run.

Two consequences dominate low-latency work:
- **Pointers are the unit of GC work.** Marking = walking the pointer graph. An object with *no pointers*
  (an `int`, a `[]byte`, a struct of only numbers) is never scanned. Fewer pointers → less marking → the
  collector finishes faster and steals fewer CPU cycles from you. During a cycle, every pointer write also
  goes through a **write barrier** (a small bookkeeping cost the compiler inserts on pointer assignments so
  the concurrent marker doesn't miss an edge) — one more reason pointer-light data is cheaper.
- **GC assist can add latency to *your* allocation.** If your goroutines allocate faster than the
  background collector can keep up, the runtime makes the *allocating* goroutine help mark (an "assist").
  A burst of allocation can therefore show up as latency *in the request doing the allocating*.

### Pacing: `GOGC` and `GOMEMLIMIT`
The GC decides *when* to run using a target:
- **`GOGC`** (default 100) — run the next GC when the heap has grown 100% (doubled) since the last
  live-heap size. Higher `GOGC` (e.g. 200, or `SetGCPercent`) = fewer GCs, more memory, less CPU spent
  collecting. Lower = more frequent GCs, less memory. It trades memory for CPU.
- **`GOMEMLIMIT`** (Go 1.19+, or `debug.SetMemoryLimit`) — a **soft memory cap**. The GC will run more
  aggressively as the heap approaches it, keeping you under the limit. This is the modern replacement for
  the old "ballast" hack. Set it a bit below your container's memory limit to avoid OOM kills while
  letting `GOGC` stay high for throughput.
```go
import "runtime/debug"
debug.SetGCPercent(200)                 // or env GOGC=200
debug.SetMemoryLimit(4 << 30)           // 4 GiB soft cap; or env GOMEMLIMIT=4GiB
```
Watch it work with `GODEBUG=gctrace=1 go run .` — one line per cycle showing pause times and heap sizes.

### Reduce pointers → reduce GC work
This is the highest-leverage GC lever after "allocate less":
- **Value slices over pointer slices.** `[]Item` is one allocation and *zero* pointers for the GC to
  chase; `[]*Item` is `n+1` allocations and `n` pointers scanned every cycle ([46](46-low-latency-measuring.md) #14).
- **Indices instead of pointers.** In a graph/tree stored in a big `[]Node`, reference children by
  **`int32` index** into the slice rather than `*Node`. The slice is one pointer-free allocation; the GC
  scans nothing inside it. (This is how high-performance parsers and ECS game engines store data.)
- **Avoid maps with pointer-heavy values** on hot data; a `map[K]int` index into a value slice beats
  `map[K]*V` for GC.
- **Keep byte data in `[]byte`**, not `[]*something`. A pointer-free object is invisible to the marker.

### Memory layout: struct field order & the cache
The CPU reads memory in **cache lines** (64 bytes). Data the CPU uses together should sit together. Two
concrete Go levers:
- **Field ordering & padding.** The compiler aligns fields, so field order changes struct size. Put
  larger fields first / group by size to avoid padding holes:
```go
type Bad  struct { a bool; b int64; c bool }   // 24 bytes: bool + 7 pad + int64 + bool + 7 pad
type Good struct { b int64; a bool; c bool }    // 16 bytes: int64 + 2 bools + 6 pad
```
  Check with `unsafe.Sizeof`, and let `go vet` / `fieldalignment` flag the waste:
```bash
go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
fieldalignment ./...
```
- **Struct-of-arrays (SoA) vs array-of-structs (AoS).** If you sum one field over a million records,
  `struct{Xs, Ys []float64}` streams just the `Xs` through cache; `[]struct{X, Y float64}` drags every
  `Y` along too, halving effective cache usage. SoA is a data-oriented-design staple for hot loops.

Don't reorder every struct in your codebase — do it for the hot, high-count ones a profile points at.

### `sync.Pool`, done right
`sync.Pool` recycles transient objects so a hot path allocates ~zero ([46](46-low-latency-measuring.md) #11).
The rules that keep it correct:
- **`Reset` on `Get`** (or before `Put`) — a pooled object carries stale state.
- **Never keep a reference after `Put`.** Once you hand it back, another goroutine may take it; mutating
  it is a data race.
- **The GC empties the pool** (up to twice per cycle). So it's for *transient* objects, not a long-lived
  cache, and won't hold memory forever.
- **Best for same-ish-sized objects.** Pooling wildly varying sizes wastes memory (you keep the big ones)
  — cap or bucket by size.
- **Measure.** For cheap, tiny objects the pool's own overhead can lose. Pool buffers and big structs,
  not every `int`.

### Lock contention — the other latency source
When many goroutines hit one mutex, they **serialise**: only one runs the critical section, the rest
block. That's invisible to a CPU profile (they're sleeping, not burning CPU) — you need the **mutex/block
profiles** (below). Fixes, roughly in order:
- **Shrink the critical section.** Do formatting, allocation, and I/O *outside* the lock; hold it only for
  the actual shared-state mutation.
- **`sync.RWMutex`** when reads vastly outnumber writes — many readers proceed in parallel.
- **Shard / stripe.** Split one hot mutex+map into N shards keyed by `hash(key) % N`; contention drops by
  ~N because different keys hit different locks.
- **Atomics** for simple counters/flags — `atomic.Int64`, `atomic.Pointer[T]` — no lock at all.
- **`sync.Map`** only for its niche: read-mostly, or disjoint key sets per goroutine. A plain
  `map`+`RWMutex` usually wins otherwise; don't reach for it by default.

### False sharing — when "no shared data" still contends
Two atomics/counters that live on the **same 64-byte cache line** ping-pong that line between cores even
though the goroutines touch *different* variables — "false sharing". Pad hot per-core data to its own
cache line:
```go
type counter struct {
    n   atomic.Int64
    _   [56]byte     // pad to a full 64-byte cache line (8 + 56)
}
var counters [numShards]counter   // each shard on its own line → no ping-pong
```
It's a niche fix (per-CPU counters, sharded structures), but a real one when a profile shows a hot atomic.

### Profiling — find the bottleneck, then fix it
Never guess. Go's profilers, and what each is *for*:
- **CPU profile** — where wall-clock CPU goes. `go test -cpuprofile`, or `net/http/pprof` in a live
  service. Read with `go tool pprof`, `top`, `list Func`, or a flame graph (`-http=:8080`).
- **Heap profile** — memory. Two views that answer different questions: **`alloc_space`** (total bytes
  ever allocated — *allocation rate*, i.e. GC pressure) vs **`inuse_space`** (live bytes now — *leaks/
  footprint*). For latency you usually chase `alloc_space`.
- **Mutex profile** (`runtime.SetMutexProfileFraction`) — where goroutines wait on locks.
- **Block profile** (`runtime.SetBlockProfileRate`) — where they block on channels/syscalls.
- **Execution tracer** (`runtime/trace` → `go tool trace`) — a timeline: GC pauses, scheduler latency,
  goroutine blocking, per-P activity. The tool for "why is my p99 spiky" when the profiles look fine.

Wire `net/http/pprof` into every service (import `_ "net/http/pprof"`, serve on an admin port); it's
near-free until you pull a profile.

## Exercises
1. Run any allocating program with `GODEBUG=gctrace=1` and read one line: pause time, heap-before/after,
   and how often it collects. Then set `GOGC=400` and watch the frequency drop.
2. Build a `[]Item` and a `[]*Item` of 1M elements; compare `inuse_space` and force a GC — reason about
   which one gives the collector more pointers to scan.
3. Store a tree as a `[]Node` with `int32` child indices instead of `*Node`. Confirm the whole tree is one
   allocation and (via `-gcflags=-m`) that nothing inside escapes per-node.
4. Reorder a struct's fields to remove padding; verify the size shrinks with `unsafe.Sizeof` and that
   `fieldalignment ./...` stops complaining.
5. Benchmark summing one field over 1M records as AoS (`[]struct{X,Y,Z float64}`) vs SoA
   (`struct{Xs,Ys,Zs []float64}`). Explain the difference by cache behaviour.
6. Take a `map`+`Mutex` counter hammered by 100 goroutines; measure the mutex profile, then shard it into
   16 and re-measure. Show contention dropping.
7. Enable `net/http/pprof`, drive load, and pull a CPU profile and an `alloc_space` heap profile. Use
   `go tool pprof -http` to view flame graphs and find the top allocator.

## Best Practices & Pitfalls
- **Fewer pointers = less GC.** Prefer value slices, indices over pointers, and pointer-free structs on
  hot, high-count data. The marker only chases pointers.
- **Tune with `GOMEMLIMIT` + `GOGC`, not ballast.** Set a soft memory limit below the container cap; raise
  `GOGC` for throughput if you have the headroom.
- **`sync.Pool`: reset on get, never retain after put, pool same-sized transient objects, and measure.**
- **Order struct fields to kill padding** on hot structs; let `fieldalignment` find them.
- **Shrink critical sections; shard hot locks; use atomics for counters.** Serialise as little as possible.
- **Profile before optimizing — with the *right* profile.** CPU for compute, `alloc_space` for GC pressure,
  mutex/block for contention, the tracer for latency spikes.
- **Pitfall — chasing `inuse_space` when your problem is allocation rate.** A low live heap can still churn
  GC if you allocate-and-discard constantly; look at `alloc_space`.
- **Pitfall — `sync.Map` as a default.** It's for read-mostly/disjoint-key workloads; a `map`+`RWMutex` is
  usually faster and simpler.
- **Pitfall — micro-optimizing layout everywhere.** Padding/SoA matter for the hot 1%; elsewhere they just
  hurt readability.
- **Pitfall — pooling tiny objects.** The pool bookkeeping can cost more than the allocation it saves.

## Checklist
- [ ] I can explain concurrent mark-sweep, why pointers are the GC's cost, and what GC assist is.
- [ ] I know what `GOGC` and `GOMEMLIMIT` trade off and can set them (env or `runtime/debug`).
- [ ] I reduce GC work with value slices, index-based references, and pointer-free structs.
- [ ] I can use `sync.Pool` correctly and state its three footguns.
- [ ] I can shrink/shard a hot lock, use atomics, and recognise false sharing.
- [ ] I can order struct fields to remove padding and reason about AoS vs SoA for the cache.
- [ ] I can pull and read CPU, heap (`alloc_space`/`inuse_space`), mutex, and block profiles, plus a trace.

## Resources
- Go blog, "Getting to Go: The Journey of Go's Garbage Collector": https://go.dev/blog/ismmkeynote
- "A Guide to the Go Garbage Collector" (GOGC, GOMEMLIMIT, the pacer): https://tip.golang.org/doc/gc-guide
- `runtime/pprof` & `net/http/pprof`: https://pkg.go.dev/net/http/pprof · Go blog "Profiling Go Programs": https://go.dev/blog/pprof
- `go tool trace` — the execution tracer: https://pkg.go.dev/runtime/trace
- `fieldalignment` linter: https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/fieldalignment
- Dave Cheney, "High Performance Go Workshop" (profiling, GC, memory): https://dave.cheney.net/high-performance-go-workshop/dotgo-paris.html
- Examples: [examples/47-low-latency-gc-contention](examples/47-low-latency-gc-contention/).
- Prior art in this plan: mutexes/atomics/`sync.Map` in [15 — Sync, Context & Patterns](15-sync-context.md); allocation basics in [46](46-low-latency-measuring.md).
- Next: [48 — Low-Latency Go III: Lock-Free, Zero-Copy & Tail Latency](48-low-latency-lockfree-tail.md).
</content>
