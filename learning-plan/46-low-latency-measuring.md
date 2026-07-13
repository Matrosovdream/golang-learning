# 46 — Low-Latency Go I: Measuring & Allocation Basics

> Part 11, Low-Latency track: **46 Measuring & Allocation** → [47 GC, Layout & Contention](47-low-latency-gc-contention.md) → [48 Lock-Free, Zero-Copy & Tail Latency](48-low-latency-lockfree-tail.md).
> Low latency in Go is mostly *mechanical sympathy* — writing code that works *with* the runtime (the allocator, the GC, the CPU cache) instead of against it. But the first rule is **measure, don't guess**. This lesson gives you the measurement tools and the single biggest lever a Go program has: **allocating less**.

## Goals
- Tell **latency** from **throughput**, and know why the **tail** (p99) is what users feel.
- Write **benchmarks** that measure the right thing — including allocations — and don't lie to you.
- Understand Go's **cost model**: where allocations come from, and how **escape analysis** decides stack vs heap.
- Cut allocations on hot paths with preallocation, `strings.Builder`, `[]byte`/`string` awareness, and a first look at `sync.Pool`.

## Concepts

### Latency vs throughput, and the tail
**Throughput** is how much work per second; **latency** is how long *one* request takes. They are not the same knob — batching raises throughput but can *raise* latency too. And latency is a *distribution*, not a number:
- **p50 (median)** — a typical request. **p99 / p99.9** — the *tail*. In a request that fans out to 10 services, a 1-in-100 slow call is hit on ~10% of requests, so your p99 downstream becomes your p50 experience. **The tail is the number that matters.**
- Report percentiles, never just the average — one 5-second GC pause hides completely in a mean over a million fast requests.
- Beware **coordinated omission**: a load generator that waits for each response *under-samples* exactly the slow periods, making the tail look far better than it is.

> Rule of thumb: optimize p99, and remember that **the biggest source of tail latency in a Go service is usually the garbage collector** — which is driven by how much you allocate. That's why this track starts with allocation.

### The cost model: what actually costs you
Roughly, in increasing pain:
1. **Stack allocation** — free (a pointer bump; reclaimed on return). The goal.
2. **Heap allocation** — costs the `malloc`, *and* future GC work to scan and free it. Death by a thousand cuts on a hot path.
3. **GC pressure** — every heap object is work the GC must mark. Allocate less → GC runs less often and scans less → shorter, rarer pauses. (Details in [47](47-low-latency-gc-contention.md).)

So the highest-leverage low-latency skill in Go is **not allocating**. Everything below is in service of that.

### Escape analysis: stack or heap?
The compiler decides at *compile time* whether a value can live on the stack (fast, freed on return) or must **escape** to the heap. See its decisions:
```bash
go build -gcflags='-m' .        # add -m -m for more detail
```
Common reasons a value escapes:
- **You return a pointer to a local** — it must outlive the frame, so it goes to the heap.
- **It's stored in an interface** — `fmt.Println(x)` boxes `x`; the boxed value usually escapes.
- **It's captured by a closure** that outlives the call, or its size isn't known at compile time (e.g. a slice made with a runtime length).
```go
func stackable() int      { x := 42; return x }        // x stays on the stack
func escapes() *int       { x := 42; return &x }        // &x escapes to the heap
```
You don't fight the compiler line-by-line in normal code — but on a proven-hot path, checking `-m` tells you *why* an allocation is happening.

### Inlining & devirtualization
Small functions get **inlined** — the body is copied into the caller. That removes the call overhead *and*, more importantly, lets escape analysis see through the call, so a value that would otherwise escape can stay on the stack. The same `-gcflags='-m'` reports it (`can inline`, `inlining call to`); `-m=2` adds the cost/threshold detail. When the compiler knows a value's concrete type at an interface call site, it also **devirtualizes** — replacing the dynamic dispatch with a direct (then often inlinable) call.
- Inlining is blocked by closures, `defer`/`recover`/`select`, `//go:noinline`, and bodies over ~80 AST nodes. Keep hot leaf functions small so they inline.
- To see the actual machine code, `go tool compile -S` and `go tool objdump` print the assembly (the `lensm` tool renders it side-by-side with source).

### Values over `interface{}`: generics don't box
An `interface{}`/`any` parameter **boxes** its argument onto the heap (the 0–255 small-int cache aside). **Generics** avoid this: a generic function is stamped out per *GC shape* (its size, alignment, and pointer-ness) and works on the value directly — the abstraction of `interface{}` without the allocation. The trade-off: a generic function can't be inlined and dispatches through a per-shape dictionary, so for a tiny operation on a single concrete type a plain concrete function can still win. As always: measure with `-benchmem`, don't assume.

### Benchmarking without fooling yourself
Go's `testing` package benchmarks. Put this in a `_test.go` file:
```go
func BenchmarkJoin(b *testing.B) {
    parts := []string{"a", "b", "c", "d"}
    b.ReportAllocs()          // always: show allocs/op and bytes/op
    b.ResetTimer()            // don't count setup above
    for i := 0; i < b.N; i++ {
        sink = strings.Join(parts, "-")   // assign to a package var (see below)
    }
}
var sink string               // prevents the compiler deleting the "useless" result
```
Run it:
```bash
go test -bench=. -benchmem -count=10 | tee old.txt
```
Then compare runs with **benchstat** (accounts for noise; gives you a mean ± variance and a p-value):
```bash
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```
Three ways benchmarks lie, and the fixes:
- **Dead-code elimination** — if you never use the result, the compiler may delete the whole loop body. Assign to an exported/package `sink`.
- **Constant folding / hoisting** — if the input is a constant, the compiler may compute it once. Vary the input or read it from a variable.
- **Setup counted in the timing** — `b.ResetTimer()` (or `b.StopTimer()`/`b.StartTimer()`) around setup.

### `testing.AllocsPerRun` — a deterministic allocation count
Benchmarks give you *time* (noisy, machine-dependent). For **allocations**, `testing.AllocsPerRun` gives you a **deterministic integer** you can assert on — great for examples and regression tests:
```go
n := testing.AllocsPerRun(100, func() {
    sink = fmt.Sprintf("%d", 12345)      // this allocates
})
fmt.Printf("allocs/op: %.0f\n", n)       // → 2 (same on every machine)
```
Most examples in this track use `AllocsPerRun` so the output is reproducible, not timing that varies by CPU.

### The everyday allocation wins
- **Preallocate slices and maps** when you know (or can estimate) the size. Growing a slice by `append` reallocates and copies repeatedly (~O(n) reallocations for n items):
  ```go
  out := make([]int, 0, len(in))   // one allocation, not log₂(n) of them
  m := make(map[string]int, len(in))
  ```
- **Build strings with `strings.Builder`**, not `+=` in a loop. `s += x` is O(n²) copies and n allocations; `Builder` grows one backing buffer (and you can `Grow(n)` it up front).
- **`[]byte` ↔ `string` conversions copy.** Each `string(b)` / `[]byte(s)` allocates and copies. The compiler *elides* the copy in a few blessed spots (e.g. `m[string(b)]` map lookups, `switch string(b)`, `for range string(b)`) — lean on those instead of converting.
- **Append the number, don't format it.** `strconv.AppendInt(buf, n, 10)` writes into an existing buffer with zero allocations; `fmt.Sprintf("%d", n)` allocates (and boxes `n` into an `interface{}`).
- **Reuse buffers with [`sync.Pool`](https://pkg.go.dev/sync#Pool)** on hot paths that need a scratch `[]byte`/`bytes.Buffer`. Get one, use it, `Reset`, `Put` it back. (Full treatment — and the footguns — in [47](47-low-latency-gc-contention.md).)

None of this means micro-optimize everywhere — that's how you get unreadable code. It means: **profile, find the hot path, then apply these there.** Clarity everywhere else.

## Exercises
1. Write a benchmark for `strings.Join` vs a manual `+=` loop vs `strings.Builder` over ~1000 parts. Add `b.ReportAllocs()` and compare `allocs/op` with `-benchmem`. Explain the ratio.
2. Add a `var sink` and show that *removing* it lets the compiler delete your benchmark's work (the time drops to ~0 ns/op). Put it back.
3. Use `go build -gcflags='-m'` on a function that returns `&localStruct{}` and one that returns `localStruct{}` by value. Confirm which one prints `escapes to heap`.
4. Use `testing.AllocsPerRun` to show that `fmt.Sprintf("%d", n)` and `strconv.Itoa(n)` each allocate the result string once, while `strconv.AppendInt(buf, n, 10)` into a reused buffer allocates **0**. (They tie on allocations, but `Itoa` is far cheaper in CPU — no format parsing, no boxing.)
5. Preallocate: benchmark building a `[]int` of 10000 with `append` on a nil slice vs `make([]int, 0, 10000)`. Compare `allocs/op`.
6. Show the compiler eliding a conversion: benchmark a `map[string]int` lookup written as `m[string(b)]` (b is `[]byte`) and confirm it does **not** allocate, then break it by first assigning `s := string(b)` and looking up `m[s]`.
7. Install `benchstat`, run one benchmark with `-count=10` before and after a change, and read the delta and p-value.
8. Read `go build -gcflags='-m'` on a small leaf function and a `//go:noinline` twin: confirm which gets `can inline` / `inlining call to`. Add an interface call whose concrete type is known and find the `devirtualizing` line.
9. With `testing.AllocsPerRun`, show a generic `Sum[T]([]T)` allocates **0** while holding the same numbers as `[]any` boxes each one. Explain why the generic version avoids the boxing.

## Best Practices & Pitfalls
- **Measure first.** Never optimize on a hunch — profile ([47](47-low-latency-gc-contention.md)) or benchmark, change one thing, re-measure with `benchstat`.
- **Optimize the tail (p99), not the mean.** Report percentiles. The mean hides the pauses users actually feel.
- **Allocating less is the master lever.** Fewer heap objects → less GC work → shorter, rarer pauses → a better tail.
- **Always `b.ReportAllocs()`** and read `allocs/op`; it's more stable and often more meaningful than `ns/op`.
- **Preallocate with a capacity** whenever the size is known or estimable; reach for `strings.Builder` over string `+=`.
- **Pitfall — benchmarks that lie.** Dead-code elimination, constant folding, and setup-in-the-timer all produce fantasy numbers. Use a `sink`, vary inputs, `ResetTimer`.
- **Pitfall — premature micro-optimization.** Don't uglify cold code to save nanoseconds nobody measures. Hot path only, proven by a profile.
- **Pitfall — treating `ns/op` as absolute truth.** It's relative and machine-dependent; use it to compare A vs B on the same box, not as a spec.
- **Pitfall — forgetting conversions copy.** A `string(b)` in a loop is a hidden allocation; use the compiler's blessed elision sites or keep one representation.

## Checklist
- [ ] I can explain latency vs throughput and why p99 (the tail) is what matters.
- [ ] I can write a benchmark with `ReportAllocs`/`ResetTimer` and a `sink` that resists dead-code elimination.
- [ ] I can read `go build -gcflags='-m'` and say why a value escapes to the heap.
- [ ] I use `testing.AllocsPerRun` to get a deterministic allocation count.
- [ ] I preallocate slices/maps, use `strings.Builder`, and know `[]byte`↔`string` conversions copy.
- [ ] I know `strconv.Append*` writes into a buffer with zero allocations.
- [ ] I can compare two implementations with `benchstat` and read the result.
- [ ] I can read `-gcflags='-m'` inlining/devirtualization decisions and name what blocks inlining.
- [ ] I know `interface{}` params box while generics (per GC shape) don't — and the generics trade-off.

## Resources
- Dave Cheney, "High Performance Go Workshop": https://dave.cheney.net/high-performance-go-workshop/dotgo-paris.html
- Go blog, "Profiling Go Programs": https://go.dev/blog/pprof
- `testing` package (benchmarks, `AllocsPerRun`): https://pkg.go.dev/testing
- `benchstat`: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
- "Escape analysis" — `go build -gcflags='-m'`; and Ardan Labs' escape-analysis mechanics: https://www.ardanlabs.com/blog/2017/05/language-mechanics-on-escape-analysis.html
- Inlining, devirtualization & generics allocation (interface boxing, GC shapes): https://g4s8.wtf/posts/go-low-latency-one/
- `lensm` — assembly ↔ source viewer (`go tool compile -S` / `objdump` made readable): https://github.com/loov/lensm
- Gil Tene, "How NOT to Measure Latency" (coordinated omission): https://www.youtube.com/watch?v=lJ8ydIuPFeU
- Examples: [examples/46-low-latency-measuring](examples/46-low-latency-measuring/).
- Prior art in this plan: benchmarks & fuzzing in [18 — Testing](18-testing.md); `sync.Pool` first appears in [15 — Sync, Context & Patterns](15-sync-context.md).
- Next: [47 — Low-Latency Go II: GC Pressure, Memory Layout & Contention](47-low-latency-gc-contention.md).
</content>
</invoke>
