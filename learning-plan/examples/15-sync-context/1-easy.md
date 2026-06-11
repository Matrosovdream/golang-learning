# Step 15 — Sync, Context & Patterns · 🟢 Easy — examples **1–10**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # most examples here are concurrent — try go run -race . too
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. See a data race with -race

`🟢 easy` · *Race detector*

A plain `n++` on a shared int is a read-modify-write — load `n`, add 1, store `n` — and two goroutines running it with no lock interleave those three steps and silently lose increments. Go's race detector (`go run -race .`) catches exactly this class of bug at runtime.

**Steps:**

1. Create `main.go`: start two goroutines with `wg.Add(1)` / `defer wg.Done()`, each incrementing the shared `n` 100,000 times — no mutex, no atomic.
2. Run `go run .` a few times. The final count is almost never 200000 and changes every run — two goroutines both load the same value of `n`, both add 1, and both store, so two increments collapse into one.
3. Run `go run -race .`. The program still prints a count, but stderr shows a report like the snippet below — it names the racing line (`main.go:17`, the `n++`) from both goroutines, then ends with `Found 2 data race(s)` and `exit status 66`:

   ```
   WARNING: DATA RACE
   Read at 0x00c000012148 by goroutine 7:
     main.main.func1()
         main.go:17 +0x88

   Previous write at 0x00c000012148 by goroutine 8:
     main.main.func1()
         main.go:17 +0x98
   ```

4. Race reports fire only on code paths that actually run, so exercise your concurrency under `-race` (especially in tests) — a silent run proves nothing about paths you didn't hit.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var n int // shared by both goroutines — and NOT protected by any lock

	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100000; i++ {
				n++ // NOT atomic: load n, add 1, store n — three steps
			}
		}()
	}
	wg.Wait()

	// Both goroutines can load the same value, each add 1, then both
	// store — two increments collapse into one. Updates are lost.
	fmt.Println("final count:", n) // almost never 200000, varies per run
}
```

**Output:**

```
final count: 100502
```

*(your count will differ run to run — that is the data race; `go run -race .` flags it)*

---

## 2. Mutex 101: Lock blocks until Unlock

`🟢 easy` · *Mutex*

`mu.Lock()` does not fail or return an error when the mutex is taken — it simply **blocks** the calling goroutine until the holder calls `Unlock()`. This example makes that wait visible with a deterministic 4-line timeline.

**Steps:**

1. Goroutine A calls `mu.Lock()` immediately, prints `A: locked`, holds the lock for 150ms, prints `A: unlocking`, then calls `mu.Unlock()`.
2. Main sleeps 50ms first — so A is guaranteed to already hold the lock — then prints `B: waiting for the lock` and calls `mu.Lock()`, which blocks for the remaining ~100ms.
3. The moment A unlocks, B's `Lock()` returns and it prints `B: got the lock`; `wg.Wait()` lets A finish cleanly.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { // goroutine A
		defer wg.Done()
		mu.Lock() // A grabs the lock first (main sleeps 50ms below)
		fmt.Println("A: locked")
		time.Sleep(150 * time.Millisecond) // hold the lock while B waits
		fmt.Println("A: unlocking")
		mu.Unlock() // only now can B's Lock() return
	}()

	time.Sleep(50 * time.Millisecond) // 50ms << 150ms: A surely holds the lock
	fmt.Println("B: waiting for the lock")
	mu.Lock() // BLOCKS here until A calls Unlock — that is the whole point
	fmt.Println("B: got the lock")
	mu.Unlock()

	wg.Wait()
}
```

**Output:**

```
A: locked
B: waiting for the lock
A: unlocking
B: got the lock
```

---

## 3. defer mu.Unlock() survives every return path

`🟢 easy` · *Mutex*

A function with multiple `return` statements only needs one `mu.Lock()` + `defer mu.Unlock()` at the top: defer fires on every return path — and on panics — so no branch can leak the lock. This is also why you keep critical sections short: the lock is held until the function returns.

**Steps:**

1. `update(key)` does `mu.Lock()` then `defer mu.Unlock()` before any branching, then either returns early with an error (unknown key) or increments `store[key]` and returns nil.
2. Call `update("missing")` to exercise the early-return path, then `update("hits")` for the normal path.
3. In `main`, take `mu.Lock()` a third time: if either path had leaked the lock, this would deadlock and crash — printing the final line is the proof both paths unlocked.

```go
package main

import (
	"fmt"
	"sync"
)

var (
	mu    sync.Mutex
	store = map[string]int{"hits": 1}
)

// update has two return paths. Lock + defer Unlock at the top covers
// BOTH of them — and would even cover a panic mid-function.
func update(key string) error {
	mu.Lock()
	defer mu.Unlock() // guaranteed to run on every return path

	if _, ok := store[key]; !ok {
		// Early return: without defer, this path would leak the lock.
		return fmt.Errorf("no such key %q", key)
	}
	store[key]++
	return nil // normal return: the same defer unlocks here too
}

func main() {
	fmt.Println("early-return path:", update("missing"))

	if err := update("hits"); err == nil {
		fmt.Println("normal path: ok, hits =", store["hits"])
	}

	// The proof: if either path above had leaked the mutex, this Lock
	// would block forever and the runtime would crash with
	// "all goroutines are asleep - deadlock!". Reaching the print means
	// both paths released it.
	mu.Lock()
	fmt.Println("third Lock acquired: no path leaked the mutex")
	mu.Unlock()
}
```

**Output:**

```
early-return path: no such key "missing"
normal path: ok, hits = 2
third Lock acquired: no path leaked the mutex
```

---

## 4. Never copy a mutex (go vet copylocks)

`🟢 easy` · *Mutex*

A `sync.Mutex` only works if every goroutine locks the *same* mutex — copy the struct that contains it and you lock a useless private copy. Never copy a struct containing a sync type; pass pointers, and let `go vet`'s copylocks check catch the mistake for you.

**Steps:**

1. Define `Counter` with a `mu sync.Mutex` and an `n int`, then give it two methods: `Inc()` with a value receiver (broken — locks and increments a copy) and `IncP()` with a pointer receiver (correct).
2. In `main`, call `c.Inc()` and print `c.n` — still `0`, because only the copy was incremented; then call `c.IncP()` and print `c.n` — now `1`.
3. Run `go vet ./...`: it must fail with `main.go:17:9: Inc passes lock by value: scratch.Counter contains sync.Mutex` (the `scratch` part is your module name). That diagnostic is the whole point — vet sees the mutex being copied through the value receiver.
4. Run `go run .` anyway: copying a mutex is legal Go, so the program compiles and runs — which is exactly why you need vet to catch it.

```go
package main

import (
	"fmt"
	"sync"
)

// Counter bundles a mutex with the value it guards.
type Counter struct {
	mu sync.Mutex
	n  int
}

// Inc has a VALUE receiver: c is a copy of the whole Counter,
// mutex included. It locks the copy and increments the copy —
// the original is never touched. go vet copylocks flags this.
func (c Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++ // the copy's n; thrown away when Inc returns
}

// IncP has a pointer receiver: the lock and the increment both
// hit the one real Counter. Always pass sync types by pointer.
func (c *Counter) IncP() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func main() {
	var c Counter

	c.Inc() // silently mutates a copy
	fmt.Println("after Inc  (value receiver):   n =", c.n)

	c.IncP() // mutates c itself
	fmt.Println("after IncP (pointer receiver): n =", c.n)
}
```

**Output:**

```
after Inc  (value receiver):   n = 0
after IncP (pointer receiver): n = 1
```

---

## 5. RWMutex: reads overlap, writes exclude

`🟢 easy` · *RWMutex*

`sync.RWMutex` lets any number of readers hold the lock at once via `RLock`, while `Lock` (the write lock) waits until every reader leaves. This is the lock to reach for when a value is read far more often than it is written.

**Steps:**

1. Start 3 goroutines that each `RLock`, sleep 80ms, and `RUnlock`; time the whole batch with a `sync.WaitGroup` — if the reads overlapped, the total is well under 200ms.
2. Print the boolean `elapsed < 200*time.Millisecond` as `reads ran concurrently: true` — never the raw duration, which differs on every run.
3. Start one goroutine that holds `RLock` for 100ms and signals main over a `locked` channel once it has the lock.
4. After `<-locked`, call `mu.Lock()` in main and verify it blocked at least 80ms before returning — the writer had to wait for the reader.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var mu sync.RWMutex

	// Part 1: three readers grab the lock at the same time.
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.RLock()                        // RLock does not block other RLocks
			time.Sleep(80 * time.Millisecond) // hold the read lock
			mu.RUnlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	// Sequential reads would need 240ms; overlapping reads finish in ~80ms.
	fmt.Println("reads ran concurrently:", elapsed < 200*time.Millisecond)

	// Part 2: a writer must wait until every reader is gone.
	locked := make(chan struct{})
	go func() {
		mu.RLock()
		close(locked) // tell main the read lock is now held
		time.Sleep(100 * time.Millisecond)
		mu.RUnlock()
	}()
	<-locked // reader definitely holds RLock past this point
	start = time.Now()
	mu.Lock() // blocks here until the reader calls RUnlock
	waited := time.Since(start)
	mu.Unlock()
	fmt.Println("write waited for the reader:", waited >= 80*time.Millisecond)
}
```

**Output:**

```
reads ran concurrently: true
write waited for the reader: true
```

---

## 6. sync.OnceFunc and sync.OnceValue (Go 1.21+)

`🟢 easy` · *Once*

Go 1.21 added wrappers that replace the classic `var once sync.Once` + `once.Do(f)` dance: `sync.OnceFunc(f)` returns a function that runs `f` only on its first call, and `sync.OnceValue(load)` additionally caches `load`'s return value for every caller. Note the counters can be plain ints: the increments need no lock because Once guarantees the wrapped function runs exactly once, and all reads happen after the calls (and after `wg.Wait`) complete.

**Steps:**

1. Build `initDB := sync.OnceFunc(func() {...})` that increments `initCount` and prints; call `initDB()` three times sequentially — only the first call runs the body.
2. Build `cfg := sync.OnceValue(func() string {...})` that increments `loadCount` and returns a config string.
3. Launch 3 goroutines under a `sync.WaitGroup`; each stores `cfg()` into its slot of `results` — `load` runs once, all three get the same cached value.
4. After `wg.Wait()`, print `loadCount` and each goroutine's result in index order.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	// sync.OnceFunc (Go 1.21+) wraps f so only the FIRST call runs it;
	// later calls are no-ops. No sync.Once variable to declare.
	initCount := 0 // plain int is fine: we read it only after the calls
	initDB := sync.OnceFunc(func() {
		initCount++ // runs at most once, so no lock needed here
		fmt.Println("opening database connection")
	})

	initDB() // runs the function
	initDB() // no-op
	initDB() // no-op
	fmt.Printf("init ran: %d time(s)\n", initCount)

	// sync.OnceValue wraps a func() T: the first caller runs load and the
	// result is cached; every later caller gets that same value.
	loadCount := 0
	cfg := sync.OnceValue(func() string {
		loadCount++ // Once serializes this single run: no lock needed
		fmt.Println("loading config from disk")
		return "host=localhost port=5432"
	})

	var wg sync.WaitGroup
	results := make([]string, 3)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = cfg() // 3 goroutines race; load still runs once
		}()
	}
	wg.Wait() // after this, reading loadCount/results needs no lock

	fmt.Printf("load ran: %d time(s)\n", loadCount)
	for i, v := range results {
		fmt.Printf("goroutine %d got: %q\n", i, v)
	}
}
```

**Output:**

```
opening database connection
init ran: 1 time(s)
loading config from disk
load ran: 1 time(s)
goroutine 0 got: "host=localhost port=5432"
goroutine 1 got: "host=localhost port=5432"
goroutine 2 got: "host=localhost port=5432"
```

---

## 7. ctx goes first: Background and the call convention

`🟢 easy` · *Context basics*

Before cancellation comes plumbing: a `context.Context` travels explicitly down the call chain as the first parameter of every function, so that a cancel or deadline created at the top can later reach the bottom. This example wires `ctx` through two layers with no cancellation at all and inspects what an empty root context looks like.

**Steps:**

1. In `main`, create the root context with `ctx := context.Background()` — empty, never cancelled, no deadline.
2. Pass it as the first argument to `fetchUser(ctx, 42)`, which forwards it to `queryDB(ctx, ...)`; by convention `ctx` is always first, always named `ctx`, and never stored in a struct field.
3. In `queryDB`, introspect it: `_, ok := ctx.Deadline()` reports whether a deadline is set, and `ctx.Err() != nil` reports whether it has been cancelled.
4. Use `context.TODO()` to mark call paths where the plumbing isn't finished yet — it behaves like `Background` but signals intent.

```go
package main

import (
	"context"
	"fmt"
)

// Convention: ctx is ALWAYS the first parameter and is always named ctx.
// You pass it down the call chain explicitly — never store it in a struct.
func fetchUser(ctx context.Context, id int) string {
	fmt.Printf("fetchUser: looking up user %d\n", id)
	return queryDB(ctx, fmt.Sprintf("SELECT name FROM users WHERE id = %d", id))
}

// Every layer that does I/O (or calls something that does) takes ctx too,
// so a cancellation started in main can reach all the way down here.
func queryDB(ctx context.Context, query string) string {
	fmt.Println("queryDB:", query)

	// A context can be inspected: does it carry a deadline?
	_, ok := ctx.Deadline()
	fmt.Println("queryDB: deadline set:", ok)

	// Has it been cancelled? Err() is nil while the context is still live.
	fmt.Println("queryDB: cancelled:", ctx.Err() != nil)

	return "Gopher"
}

func main() {
	// context.Background() is the root: empty, never cancelled, no deadline.
	// It is the standard starting point in main, init, and tests.
	ctx := context.Background()

	name := fetchUser(ctx, 42)
	fmt.Println("main: got user:", name)

	// If you haven't wired ctx through a call path yet, use context.TODO().
	// It behaves like Background but marks the spot as unfinished plumbing.
	todo := context.TODO()
	fmt.Println("main: TODO is also uncancelled:", todo.Err() == nil)
}
```

**Output:**

```
fetchUser: looking up user 42
queryDB: SELECT name FROM users WHERE id = 42
queryDB: deadline set: false
queryDB: cancelled: false
main: got user: Gopher
main: TODO is also uncancelled: true
```

---

## 8. context.WithCancel stops a goroutine

`🟢 easy` · *Context basics*

`context.WithCancel` gives you a cancellation signal you can broadcast to any number of goroutines: calling `cancel()` closes `ctx.Done()`, and every `select` watching that channel wakes up. This is Go's canonical way to tell a worker "stop now" — and `cancel` is idempotent, so it is safe to `defer` it even when you also call it explicitly.

**Steps:**

1. Write `worker(ctx, jobs, done)` that loops on `select`: on `<-ctx.Done()` it prints `ctx.Err()`, sends its job count on `done`, and returns; on `j := <-jobs` it processes the job.
2. In `main`, create `ctx, cancel := context.WithCancel(context.Background())` and `defer cancel()` as a safety net.
3. Send 3 jobs over an **unbuffered** `jobs` channel — each send is a handshake, so the worker has definitely received all three before the loop ends.
4. Call `cancel()`, then receive the worker's final count from `done` and print it.

```go
package main

import (
	"context"
	"fmt"
)

// worker processes jobs until its context is cancelled, then reports why.
func worker(ctx context.Context, jobs <-chan int, done chan<- int) {
	count := 0
	for {
		select {
		case <-ctx.Done(): // closed by cancel(); the canonical stop signal
			fmt.Println("worker: stopping, reason:", ctx.Err())
			done <- count // report how many jobs we finished
			return
		case j := <-jobs:
			count++
			fmt.Println("worker: processed job", j)
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // idempotent: calling cancel twice is safe

	jobs := make(chan int) // unbuffered: each send waits for the worker
	done := make(chan int)
	go worker(ctx, jobs, done)

	for j := 1; j <= 3; j++ {
		jobs <- j // handshake: returns only after the worker received j
	}

	cancel() // close ctx.Done(); the worker sees it on its next select

	processed := <-done // wait for the worker's exit report
	fmt.Println("main: worker processed", processed, "jobs")
}
```

**Output:**

```
worker: processed job 1
worker: processed job 2
worker: processed job 3
worker: stopping, reason: context canceled
main: worker processed 3 jobs
```

---

## 9. context.WithTimeout: Done fires on its own

`🟢 easy` · *Context timeouts*

A context made with `WithTimeout` cancels itself when the deadline passes — nobody has to call `cancel()` for `Done()` to close. You still ALWAYS `defer cancel()`, because it stops the internal timer and releases the context's resources immediately instead of leaving them for the garbage collector.

**Steps:**

1. Create `ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)` and `defer cancel()`.
2. Record `t0 := time.Now()`, then block on `<-ctx.Done()` — it unblocks by itself after the deadline.
3. Print `ctx.Err()` (it is `context.DeadlineExceeded`) and the boolean `elapsed >= 100*time.Millisecond`.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	// WithTimeout = WithDeadline(now + 100ms). The context cancels ITSELF
	// when the timer fires — no goroutine has to call cancel().
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	// ALWAYS defer cancel() anyway: it stops the timer and releases the
	// context's resources immediately instead of waiting for GC.
	defer cancel()

	t0 := time.Now()
	<-ctx.Done() // blocks until the 100ms deadline expires
	elapsed := time.Since(t0)

	fmt.Println("err:", ctx.Err()) // context.DeadlineExceeded
	fmt.Println("waited at least 100ms:", elapsed >= 100*time.Millisecond)
}
```

**Output:**

```
err: context deadline exceeded
waited at least 100ms: true
```

---

## 10. context.WithValue: request-scoped metadata

`🟢 easy` · *Context values*

`context.WithValue` carries request-scoped metadata (request IDs, auth tokens) down a call chain without threading extra parameters through every function. Use it ONLY for that kind of cross-cutting metadata — never for ordinary function parameters, because values hidden in a context are invisible dependencies.

**Steps:**

1. Declare the typed key `type reqIDKey struct{}` — an unexported empty struct: zero bytes, and no other package can construct it, so keys never collide.
2. In `main`, attach the ID once at the edge: `ctx := context.WithValue(context.Background(), reqIDKey{}, "req-42")`, then call `handle(ctx)`.
3. Deep in the chain, `logf` retrieves it with the comma-ok assertion `id, ok := ctx.Value(reqIDKey{}).(string)` and prefixes every log line with `[req-42]`.
4. Call `handle(context.Background())` too — the key is absent, `ok` is `false`, and `logf` prints `no request id` instead.

```go
package main

import (
	"context"
	"fmt"
)

// reqIDKey is the typed-key idiom: an unexported empty struct.
// It occupies zero bytes, and no other package can construct this
// type, so its values can never collide with someone else's keys.
type reqIDKey struct{}

// logf is called deep inside the call chain; it pulls the request ID
// out of the context with the comma-ok type assertion.
func logf(ctx context.Context, msg string) {
	if id, ok := ctx.Value(reqIDKey{}).(string); ok {
		fmt.Printf("[%s] %s\n", id, msg)
		return
	}
	fmt.Println("no request id:", msg) // missing key -> ok == false
}

// handle never touches the ID itself — the metadata rides along
// invisibly in ctx, which is exactly what WithValue is for.
func handle(ctx context.Context) {
	logf(ctx, "fetching user")
	logf(ctx, "writing response")
}

func main() {
	// Attach the request ID once, at the edge (e.g. HTTP middleware).
	ctx := context.WithValue(context.Background(), reqIDKey{}, "req-42")
	handle(ctx)

	// A plain Background context carries no value: assertion fails.
	handle(context.Background())
}
```

**Output:**

```
[req-42] fetching user
[req-42] writing response
no request id: fetching user
no request id: writing response
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
