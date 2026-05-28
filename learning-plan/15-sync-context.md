# 15 — Sync, Context & Patterns

## Goals
- Protect shared state with mutexes and atomics when channels aren't the right tool.
- Use `context.Context` to cancel work and enforce deadlines.
- Implement the core concurrency patterns: worker pool, fan-out/fan-in, pipeline.
- Make the race detector part of your workflow.

## Concepts
- **When to use locks vs channels** — channels are for *transferring ownership* and coordinating; **mutexes** are for *protecting shared state* that several goroutines read/write (a cache, a counter, a connection map). Use whichever makes the code simplest; don't force channels everywhere.
- **`sync.Mutex`** — mutual exclusion; only one goroutine holds the lock at a time:
  ```go
  type SafeCounter struct {
      mu sync.Mutex
      n  int
  }
  func (c *SafeCounter) Inc() {
      c.mu.Lock()
      defer c.mu.Unlock()
      c.n++
  }
  ```
- **`sync.RWMutex`** — multiple concurrent readers *or* one writer. Use `RLock`/`RUnlock` for reads when reads vastly outnumber writes.
- **`sync.Once`** — run an initializer exactly once, even under concurrency (common for lazy singletons / config loading): `once.Do(initFn)`.
- **`sync/atomic`** — lock-free atomic operations on integers/pointers (`atomic.Int64`, `atomic.AddInt64`, `CompareAndSwap`). Faster than a mutex for simple counters, but easy to misuse — reach for it only in hot paths.
- **`context.Context`** — the standard way to carry **cancellation**, **deadlines/timeouts**, and **request-scoped values** across API boundaries and goroutines. It's threaded as the **first parameter**, conventionally named `ctx`:
  ```go
  func doWork(ctx context.Context) error { ... }
  ```
  - **Create:** `context.Background()` (root, e.g., in `main`), `context.TODO()` (placeholder).
  - **Derive:** `ctx, cancel := context.WithCancel(parent)`, `context.WithTimeout(parent, 5*time.Second)`, `context.WithDeadline(...)`. **Always `defer cancel()`** to release resources.
  - **Observe:** `<-ctx.Done()` fires when cancelled/timed out; `ctx.Err()` says why (`context.Canceled` / `context.DeadlineExceeded`).
  - **Values:** `context.WithValue` carries request-scoped data (request ID, auth) — use sparingly, never for optional function params.
  - This is *the* mechanism for stopping goroutines and bounding request time in web servers (Part 6).
- **Cancellable goroutine pattern:**
  ```go
  func worker(ctx context.Context, jobs <-chan Job) {
      for {
          select {
          case <-ctx.Done():
              return                 // clean exit on cancel/timeout
          case j, ok := <-jobs:
              if !ok { return }
              process(j)
          }
      }
  }
  ```
- **Worker pool** — a fixed number of goroutines pulling from a jobs channel, sending to a results channel. Bounds concurrency so you don't spawn unbounded goroutines:
  ```go
  jobs := make(chan int, 100)
  results := make(chan int, 100)
  for w := 0; w < numWorkers; w++ {
      go worker(jobs, results)
  }
  ```
- **Fan-out / fan-in** — *fan-out*: multiple goroutines read from one channel to parallelize work; *fan-in*: merge several result channels into one. A `WaitGroup` typically closes the merged output when all sources finish.
- **Pipeline** — stages connected by channels, each stage a goroutine that reads from its input channel and writes to its output channel (`generate → square → filter → print`). Cancellation propagates via a shared `ctx` or `done` channel.
- **The race detector** — `go test -race ./...` and `go run -race .` instrument memory access to catch data races at runtime. Make it part of CI; races are often invisible without it.

## Exercises
1. Build a `SafeCounter` with a `sync.Mutex`; have 100 goroutines each `Inc()` 100 times; confirm the total is 10000 (and that removing the lock + `-race` reveals a race).
2. Convert the counter to use `atomic.Int64` and compare the code.
3. Use `sync.Once` to lazily initialize a value that several goroutines request concurrently; prove the initializer runs once.
4. Write `doWork(ctx)` that loops in a `select` and exits promptly when the context is cancelled; drive it with `context.WithTimeout(…, 200ms)` and `defer cancel()`.
5. Build a worker pool: feed 20 jobs into a channel, process them with 4 workers, collect results, and ensure all goroutines exit cleanly.
6. Build a 3-stage pipeline (generate ints → square them → sum) connected by channels.
7. Run any of the above with `-race` and confirm it's clean; then introduce a deliberate race and watch it get caught.

## Best Practices & Pitfalls
- **Pass `ctx` as the first argument; never store it in a struct.** Don't pass `nil` — use `context.Background()`/`context.TODO()`.
- **Always `defer cancel()`** for every `WithCancel`/`WithTimeout`/`WithDeadline`, even if the work finishes early — otherwise you leak the context's resources.
- **`defer mu.Unlock()` right after `Lock()`.** It guarantees unlock on every return path, including panics. Keep critical sections short.
- **Don't copy a `sync.Mutex`/`WaitGroup`/`Once`.** Embed them in a struct and pass the struct by pointer; copying them breaks them (`go vet` warns).
- **Bound your concurrency.** Use a worker pool instead of `go` per item for large/unbounded inputs to avoid memory blowups.
- **Pitfall — `context.WithValue` abuse:** it's for request-scoped metadata (request ID, trace), not for passing normal function arguments. Overusing it hides dependencies.
- **Pitfall — ignoring `ctx.Done()`** in long loops means cancellation/timeouts do nothing. Always `select` on `ctx.Done()` in goroutines that can run long.
- **Make `-race` non-negotiable in tests/CI.** A passing test without `-race` can still hide races.

## Checklist
- [ ] I can choose between a mutex and a channel for a given problem.
- [ ] I can protect shared state with `sync.Mutex`/`RWMutex` and `defer Unlock`.
- [ ] I can create, derive, and observe a `context.Context` (and always `defer cancel()`).
- [ ] I can write a goroutine that exits on `ctx.Done()`.
- [ ] I can build a worker pool and a pipeline.
- [ ] I run concurrent code with `-race`.

## Resources
- `sync` package: https://pkg.go.dev/sync
- `context` package: https://pkg.go.dev/context
- Blog — Go Concurrency Patterns: Context: https://go.dev/blog/context
- Blog — Pipelines and cancellation: https://go.dev/blog/pipelines
- Blog — The Go Memory Model (advanced): https://go.dev/ref/mem
