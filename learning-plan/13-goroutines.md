# 13 — Goroutines

## Goals
- Launch concurrent work with the `go` keyword.
- Build a correct mental model of the Go scheduler.
- Wait for goroutines to finish with `sync.WaitGroup`.
- Recognize and avoid goroutine leaks.

## Concepts
- **A goroutine is a lightweight thread managed by the Go runtime.** Start one by prefixing any function call with `go`:
  ```go
  go doWork()           // runs concurrently; the caller continues immediately
  go func() { ... }()   // anonymous goroutine
  ```
- **Goroutines are cheap.** They start with a tiny (~2 KB) growable stack, so you can run thousands or millions — unlike OS threads. The runtime multiplexes many goroutines onto a few OS threads (the "M:N scheduler").
- **Concurrency vs parallelism** — *concurrency* is structuring a program as independently executing pieces; *parallelism* is running them at the same instant on multiple cores. Go gives you concurrency; the scheduler + `GOMAXPROCS` decide parallelism. (Rob Pike: "Concurrency is not parallelism.")
- **The scheduler (mental model)** — Go's runtime has its own scheduler that runs goroutines on a pool of OS threads. When a goroutine blocks (I/O, channel, syscall), the scheduler parks it and runs another on the same thread. You don't manage threads; you just launch goroutines.
- **`main` doesn't wait.** When `func main` returns, the program exits **immediately**, killing any still-running goroutines. So:
  ```go
  go fmt.Println("hi")
  // program may exit before this prints!
  ```
  You must explicitly wait — never with `time.Sleep` (a hack), but with a `WaitGroup` or channel.
- **`sync.WaitGroup`** — wait for a set of goroutines to complete:
  ```go
  var wg sync.WaitGroup
  for _, url := range urls {
      wg.Add(1)               // increment before launching
      go func() {
          defer wg.Done()     // decrement when finished
          fetch(url)
      }()
  }
  wg.Wait()                   // blocks until counter hits 0
  ```
  - `Add(n)` before starting, `Done()` (via `defer`) inside, `Wait()` to block.
- **Goroutine leaks** — a goroutine that blocks forever (on a channel that never receives, or work that never ends) is never collected; it leaks memory and resources. Every goroutine you start needs a guaranteed way to finish (covered fully with `context` in lesson 15).
- **Communicate by sharing channels, not memory** — Go's motto: *"Don't communicate by sharing memory; share memory by communicating."* Prefer passing data over channels (lesson 14) to coordinating goroutines, rather than shared variables guarded by locks (lesson 15) — though both have their place.
- **The race detector** — concurrent access to the same variable without synchronization is a **data race** (undefined behavior). Run `go run -race .` / `go test -race` to detect them at runtime (lesson 15).

## Exercises
1. Launch `go fmt.Println("hello")` in `main` with nothing else and observe that it often *doesn't* print (main exits first). Add a `WaitGroup` to fix it.
2. Launch 5 goroutines that each print their index; use a `WaitGroup` so `main` waits for all. Run several times and notice the order varies.
3. Use the **Go 1.22+** loop-variable semantics: capture the loop variable inside the goroutine and confirm each prints its own value. (On older Go you'd pass it as an argument — ask Claude why.)
4. Write a `fetchAll(urls []string)` that launches a goroutine per URL (simulate with `time.Sleep` + print) and waits for all with a `WaitGroup`.
5. Deliberately create a goroutine that blocks on receiving from a channel nobody sends to; discuss with Claude why this leaks and how `context` (lesson 15) would prevent it.
6. Add `-race` to a program with two goroutines incrementing a shared counter without a lock; read the race report.

## Best Practices & Pitfalls
- **Never use `time.Sleep` to synchronize.** It's flaky and slow. Use `WaitGroup` or channels.
- **Always `wg.Add` before `go`, and `defer wg.Done()` inside the goroutine.** Calling `Add` *inside* the goroutine races with `Wait`.
- **Know how every goroutine ends.** If you can't point to what unblocks it, you have a leak. Pair long-running goroutines with a `context` for cancellation (lesson 15).
- **Pitfall — passing a `WaitGroup` by value.** `WaitGroup` must not be copied; pass it as a pointer (`*sync.WaitGroup`) if you hand it to a function.
- **Pitfall — data races.** Two goroutines touching the same variable (one writing) without synchronization is undefined behavior — *always* test with `-race` during development.
- **Pitfall — unbounded goroutine spawning.** Launching a goroutine per item with no limit can exhaust memory; use a worker pool (lesson 15) for large/unbounded workloads.
- **Don't start a goroutine you can't stop.** Tie its lifetime to something (a channel close, a context, a `WaitGroup`).

## Checklist
- [ ] I can start a goroutine with `go` and explain why `main` might exit first.
- [ ] I can synchronize goroutines with `sync.WaitGroup` (Add/Done/Wait).
- [ ] I understand the difference between concurrency and parallelism.
- [ ] I know what a goroutine leak is and that every goroutine needs an exit.
- [ ] I know to test concurrent code with `-race`.

## Resources
- A Tour of Go — Goroutines: https://go.dev/tour/concurrency/1
- Effective Go — concurrency: https://go.dev/doc/effective_go#concurrency
- Talk — Concurrency is not parallelism: https://go.dev/blog/waza-talk
- Blog — Race detector: https://go.dev/blog/race-detector
