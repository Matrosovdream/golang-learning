# Step 15 — Sync, Context & Patterns · 🔴 Hard — examples **29–40**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # most examples here are concurrent — try go run -race . too
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 29. A worker pool that exits cleanly on cancel

`🔴 hard` · *Patterns*

The standard shutdown order for a cancellable pool: collect every result you are owed, *then* `cancel()`, then `wg.Wait()` to prove all workers actually returned — the alternative is closing `jobs` and letting workers drain naturally, as in example 21's first run. Unbuffered channels make every send a handshake, so the whole run is deterministic.

**Steps:**

1. Write `worker` as an infinite loop over the example-21 `select`: `<-ctx.Done()` returns, `j := <-jobs` squares the job and sends it to `results`. Count each exit with `exits.Add(1)` in a `defer` placed *after* `defer wg.Done()`, so LIFO order counts the exit before `wg.Wait()` can unblock.
2. In `main`, start 3 workers sharing unbuffered `jobs` and `results` channels, plus a feeder goroutine that sends jobs 1–5.
3. Receive exactly 5 results, `sort.Ints` them (arrival order races between workers; sorted order doesn't), and only then call `cancel()` followed by `wg.Wait()`.
4. Print the sorted results and `exits.Load()` — it must read 3, because every worker took the `ctx.Done()` branch.

```go
package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

func worker(ctx context.Context, jobs <-chan int, results chan<- int, wg *sync.WaitGroup, exits *atomic.Int32) {
	defer wg.Done()    // LIFO: runs last, after the exit is counted
	defer exits.Add(1) // runs first, so wg.Wait() implies the count is final
	for {
		select { // the example-21 select: cancel wins over waiting for work
		case <-ctx.Done():
			return
		case j := <-jobs:
			results <- j * j
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan int)    // unbuffered: each send is a handshake with one worker
	results := make(chan int) // unbuffered: each result is handed straight to main

	var wg sync.WaitGroup
	var exits atomic.Int32
	for range 3 {
		wg.Add(1)
		go worker(ctx, jobs, results, &wg, &exits)
	}

	go func() { // feeder: 5 sends = 5 confirmed pickups, then it returns
		for j := 1; j <= 5; j++ {
			jobs <- j
		}
	}()

	got := make([]int, 0, 5)
	for range 5 {
		got = append(got, <-results) // collect ALL results before cancelling
	}
	sort.Ints(got) // arrival order races between workers; sorted is deterministic

	cancel()  // shutdown order: collect first, THEN cancel
	wg.Wait() // proves all 3 workers took the ctx.Done() branch and returned

	fmt.Println("results:", got)
	fmt.Printf("workers exited: %d/3\n", exits.Load())
}
```

**Output:**

```
results: [1 4 9 16 25]
workers exited: 3/3
```

---

## 30. Pipeline with context cancellation

`🔴 hard` · *Patterns*

This is the canonical go.dev/blog/pipelines shape: every stage wraps every send in `select { case out <- v: ; case <-ctx.Done(): return }`, so the send itself is the cancellation point and an abandoned stage can never block forever. Channels-lesson example 31 built this with a hand-rolled `done` channel — this is the same pipeline upgraded to `context`, which is what real Go code uses.

**Steps:**

1. Write `gen(ctx, wg)` emitting 1, 2, 3, ... forever, and `square(ctx, wg, in)` emitting `v*v`; in both, guard every send with a `select` on `ctx.Done()`.
2. In `main`, build the pipeline `square(ctx, &wg, gen(ctx, &wg))` with `wg.Add(2)` and receive exactly 3 values from `squares`.
3. Call `cancel()`, then `wg.Wait()` — both goroutines unblock from their pending sends and return; print `all stages exited: true`.

```go
package main

import (
	"context"
	"fmt"
	"sync"
)

// gen emits 1, 2, 3, ... forever. Every send selects on ctx.Done() too,
// so the send itself is the cancellation point.
func gen(ctx context.Context, wg *sync.WaitGroup) <-chan int {
	out := make(chan int)
	go func() {
		defer wg.Done()
		defer close(out)
		for n := 1; ; n++ {
			select {
			case out <- n:
			case <-ctx.Done(): // nobody is receiving anymore: unblock and exit
				return
			}
		}
	}()
	return out
}

// square reads from in and emits v*v, guarding its send the same way.
func square(ctx context.Context, wg *sync.WaitGroup, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer wg.Done()
		defer close(out)
		for v := range in {
			select {
			case out <- v * v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	squares := square(ctx, &wg, gen(ctx, &wg))
	for i := 0; i < 3; i++ { // take exactly 3 values, then walk away
		fmt.Println(<-squares)
	}
	cancel()  // tell both stages to stop instead of blocking on a send forever
	wg.Wait() // both goroutines have returned — no leaks
	fmt.Println("all stages exited:", true)
}
```

**Output:**

```
1
4
9
all stages exited: true
```

---

## 31. First error cancels the rest (mini errgroup)

`🔴 hard` · *Patterns*

When parallel tasks share a fate, you want the *first* error captured and everyone else canceled promptly — `sync.Once` picks the winner, `context.CancelFunc` spreads the news, and `sync.WaitGroup` waits for the stragglers. This is `golang.org/x/sync/errgroup` in miniature: retype it once to understand it, then use the real package in production code.

**Steps:**

1. Define `group` holding `ctx`, `cancel`, `wg`, `once`, `err`. In `Go`, wrap each task: if it returns an error, `g.once.Do(func() { g.err = err; g.cancel() })` — only the first error is recorded, and it cancels the siblings.
2. Implement `Wait` as `g.wg.Wait()`, then `g.cancel()` (cleanup), then `return g.err`.
3. Launch three tasks: A succeeds after 30ms, B fails after 80ms with `errors.New("B: boom")`, and C races a 500ms `time.After` against `ctx.Done()` — B's cancel wins at ~80ms, so C sets `cEarly` and returns `ctx.Err()`.
4. Print `g.Wait()` and `cEarly`: the result is `B: boom`, proving C's `context canceled` was silently dropped because `once` had already fired.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// group is golang.org/x/sync/errgroup in miniature: first error wins, rest get canceled.
type group struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	err    error
}

func (g *group) Go(f func(context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(g.ctx); err != nil {
			// Only the FIRST error gets recorded; it also cancels the siblings.
			g.once.Do(func() { g.err = err; g.cancel() })
		}
	}()
}

func (g *group) Wait() error {
	g.wg.Wait()
	g.cancel() // release the context even on full success
	return g.err
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	g := &group{ctx: ctx, cancel: cancel}
	var cEarly bool // written before wg.Done, read after Wait: race-free
	// A succeeds quickly, B fails first, C would grind for 500ms but is cancelable.
	g.Go(func(ctx context.Context) error { time.Sleep(30 * time.Millisecond); return nil })
	g.Go(func(ctx context.Context) error { time.Sleep(80 * time.Millisecond); return errors.New("B: boom") })
	g.Go(func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done(): // fires at ~80ms when B's error cancels us
			cEarly = true
			return ctx.Err() // ignored by Go: once already fired for B
		}
	})
	fmt.Println("Wait() ->", g.Wait()) // "B: boom", not "context canceled"
	fmt.Println("C exited early:", cEarly)
}
```

**Output:**

```
Wait() -> B: boom
C exited early: true
```

---

## 32. sync.Cond: sleep until something changes

`🔴 hard` · *Cond*

`sync.Cond` lets goroutines park until a predicate over shared state becomes true: `Wait` atomically unlocks the mutex and sleeps, then re-locks when woken — and because wakeups can be spurious (or the state re-stolen by another goroutine before you get the lock back), you must re-check the predicate in a `for` loop, never an `if`. In practice channels and `close` cover most signaling needs (lesson 15's channels examples); reach for `Cond` specifically when the thing you wait on is "this guarded condition is now true".

**Steps:**

1. Create `cond := sync.NewCond(&mu)` and a `ready bool` predicate guarded by `mu`.
2. Start 3 waiter goroutines that do `cond.L.Lock(); for !ready { cond.Wait() }; cond.L.Unlock()`, then bump an `atomic.Int64` and `wg.Done()`.
3. In `main`, sleep 50ms so all three park inside `Wait`, set `ready = true` under the lock, then `cond.Broadcast()` to wake every waiter.
4. `wg.Wait()` and print the woken count: `woken: 3`.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	var mu sync.Mutex
	cond := sync.NewCond(&mu) // cond.L is &mu

	ready := false // the predicate: shared state guarded by mu
	var woken atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cond.L.Lock()
			// Wait MUST be called in a loop: Wait atomically unlocks mu
			// and parks the goroutine, then re-locks on wake — but a wake
			// is only a hint. Wakeups can be spurious, or another
			// goroutine may have grabbed the lock first and changed the
			// state back, so always re-check the predicate.
			for !ready {
				cond.Wait()
			}
			cond.L.Unlock()
			woken.Add(1)
		}()
	}

	time.Sleep(50 * time.Millisecond) // let all three park inside Wait

	cond.L.Lock()
	ready = true // change the predicate under the same lock...
	cond.L.Unlock()
	cond.Broadcast() // ...then wake every waiter (Signal wakes just one)

	wg.Wait()
	fmt.Println("woken:", woken.Load())
}
```

**Output:**

```
woken: 3
```

---

## 33. Graceful shutdown: ctx + WaitGroup + timeout

`🔴 hard` · *Patterns*

The canonical shutdown recipe: one `cancel()` broadcasts "stop" to every server goroutine, a `WaitGroup` drains them, and the example-20 `waitTimeout` idiom bounds that drain — so one stuck worker can never hang your shutdown forever.

**Steps:**

1. Launch three `server` goroutines; each loops a `select` between `ctx.Done()` (shutdown) and a 30 ms `time.Ticker` (work), and registers in a shared `sync.WaitGroup`.
2. In `main`, sleep 100 ms so they handle a few ticks, then call `cancel()` — every server sees `ctx.Done()`, runs its cleanup step (`cleanups.Add(1)`), and returns. In production the root ctx comes from `signal.NotifyContext(context.Background(), os.Interrupt)` instead.
3. Drain with `waitTimeout(&wg, 500*time.Millisecond)` rather than a bare `wg.Wait()`, then print whether all three stopped inside the budget and how many cleanups ran.

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// waitTimeout bounds wg.Wait() with a budget (the example-20 idiom).
func waitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false // a stuck worker must not hang the whole shutdown
	}
}

func server(ctx context.Context, wg *sync.WaitGroup, cleanups *atomic.Int64) {
	defer wg.Done()
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done(): // shutdown broadcast reaches every server at once
			cleanups.Add(1) // cleanup step: flush buffers, close connections...
			return
		case <-ticker.C: // simulated request handling, ~3 ticks per server
		}
	}
}

func main() {
	// In production the root ctx comes from the OS signal:
	// ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var cleanups atomic.Int64
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go server(ctx, &wg, &cleanups)
	}

	time.Sleep(100 * time.Millisecond) // let each server handle a few ticks
	cancel()                           // one call stops all three servers

	stopped := waitTimeout(&wg, 500*time.Millisecond) // bounded drain
	fmt.Println("all 3 stopped in time:", stopped)
	fmt.Println("cleanups ran:", cleanups.Load())
}
```

**Output:**

```
all 3 stopped in time: true
cleanups ran: 3
```

---

## 34. One parent budget, per-task timeouts

`🔴 hard` · *Context timeouts*

A real request handler has one overall budget but calls several dependencies, and you don't want one slow dependency to eat the entire budget — so each call derives *its own* shorter timeout from the parent. The child context expires at whichever comes first (its own slice or the parent's deadline), so a per-task failure leaves the rest of the request budget intact.

**Steps:**

1. In `main`, create the request budget: `parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)`.
2. Write `runTask(parent, need, perTask)` that derives `context.WithTimeout(parent, perTask)` and runs example 22's buffered-result `select`: a goroutine sleeps `need` then sends `"ok"` into a cap-1 channel, raced against `ctx.Done()`.
3. Launch t1 (needs 40ms) and t2 (needs 400ms) concurrently, each with its own 120ms slice; store each result in `results[i]` so the print order is deterministic.
4. After `wg.Wait()`, print the results in index order, then prove the budget survived t2's failure: `parent.Err() == nil`.

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// runTask gives one task its OWN slice of the request: the deadline is
// derived FROM parent, so it fires at min(perTask, what's left of the
// budget) — a slow dependency burns at most perTask, never the whole request.
func runTask(parent context.Context, need, perTask time.Duration) string {
	ctx, cancel := context.WithTimeout(parent, perTask) // child of parent, not Background
	defer cancel()

	res := make(chan string, 1) // example-22 pattern: cap 1, a late worker can't leak
	go func() {
		time.Sleep(need) // pretend to call a dependency
		res <- "ok"      // buffered: succeeds even if we already gave up
	}()
	select {
	case s := <-res:
		return s
	case <-ctx.Done():
		return ctx.Err().Error() // per-task "context deadline exceeded"
	}
}

func main() {
	// One request-wide budget shared by everything below.
	parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	needs := []time.Duration{40 * time.Millisecond, 400 * time.Millisecond}
	results := make([]string, len(needs)) // collect by index, print in order

	var wg sync.WaitGroup
	for i, need := range needs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runTask(parent, need, 120*time.Millisecond)
		}()
	}
	wg.Wait()

	for i, r := range results {
		fmt.Printf("t%d: %s\n", i+1, r)
	}
	// t2 blew its per-task slice, but the request budget is untouched.
	fmt.Println("parent still alive:", parent.Err() == nil)
}
```

**Output:**

```
t1: ok
t2: context deadline exceeded
parent still alive: true
```

---

## 35. Get-or-create once: the double-check idiom

`🔴 hard` · *RWMutex*

A read-through cache wants the cheap `RLock` on the hot path, but `sync.RWMutex` cannot upgrade a read lock to a write lock — on a miss you must `RUnlock`, then `Lock`, and in that unlocked gap another goroutine may have built the entry, so you must check again before building or you do the work twice. (This dedupes completed work; to dedupe work still *in flight*, see example 38.)

**Steps:**

1. Write `Cache.Get` with a fast path — `RLock`, look up `c.data[key]`, `RUnlock` — and a slow path that takes the full `c.mu.Lock()` and re-checks the map before calling `build(key)`.
2. Count real builds in an `atomic.Int32` inside `build`, so the output proves how many times the value was constructed.
3. Park 8 goroutines on a `<-start` gate, release them all into `c.Get("k")` with `close(start)`, then `wg.Wait()` — all 8 miss the fast path together, queue on `Lock`, but only the first one builds; the other seven hit the re-check.
4. Print the value (now a pure fast-path hit) and `builds.Load()` — always `builds: 1`; without the re-check it could be anything up to 8.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

var builds atomic.Int32 // counts how many times we actually built the value

type Cache struct {
	mu   sync.RWMutex
	data map[string]string
}

func (c *Cache) Get(key string) string {
	c.mu.RLock() // fast path: shared lock, readers run in parallel
	if v, ok := c.data[key]; ok {
		c.mu.RUnlock()
		return v
	}
	c.mu.RUnlock() // RWMutex cannot upgrade RLock->Lock: release, then re-acquire

	c.mu.Lock() // slow path: exclusive — but others may have raced ahead
	defer c.mu.Unlock()
	if v, ok := c.data[key]; ok { // re-check! someone may have built it
		return v // while we waited in line for the write lock
	}
	v := build(key) // only the FIRST goroutine through Lock gets here
	c.data[key] = v
	return v
}

func build(key string) string {
	builds.Add(1)
	return "value-for-" + key
}

func main() {
	c := &Cache{data: make(map[string]string)}
	start := make(chan struct{}) // gate: all 8 goroutines miss together
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c.Get("k")
		}()
	}
	close(start) // release everyone at once
	wg.Wait()
	fmt.Println("value:", c.Get("k"))     // now a pure RLock fast-path hit
	fmt.Println("builds:", builds.Load()) // double-check guarantees exactly 1
}
```

**Output:**

```
value: value-for-k
builds: 1
```

---

## 36. Token bucket rate limiter with ctx stop

`🔴 hard` · *Patterns*

A buffered channel of `struct{}` is a complete token bucket: the capacity is the burst, a ticker goroutine drips tokens back in, and a `select`+`default` on the send discards refills when the bucket is full. This is the pattern behind real API rate limiters, with `context` providing the shutdown path.

**Steps:**

1. Create `tokens := make(chan struct{}, 2)` and preload it with 2 tokens — that buffered capacity is the burst allowance.
2. Start `refill`: a 60ms `time.Ticker` whose tick does a non-blocking send into `tokens` (full bucket drops the token), and whose `ctx.Done()` case stops the ticker, does `close(done)`, and returns.
3. In `main`, serve 4 requests, each consuming one token. A `select` with `default` classifies each request: banked token = instant, empty bucket = block on `<-tokens` until the next refill. Requests 3 and 4 must each wait a tick, so `elapsed >= 110ms`.
4. Call `cancel()` and block on `<-done` — receiving proves the refiller goroutine exited cleanly.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// refill drips one token into the bucket per tick. If the bucket is
// full the send would block, so a select+default drops the token —
// that overflow drop is what caps the long-term rate.
func refill(ctx context.Context, tokens chan<- struct{}, done chan<- struct{}) {
	ticker := time.NewTicker(60 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			select {
			case tokens <- struct{}{}: // room in the bucket
			default: // bucket full: drop this refill
			}
		case <-ctx.Done():
			ticker.Stop()
			close(done) // signal a clean shutdown
			return
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	tokens := make(chan struct{}, 2) // capacity = burst size
	tokens <- struct{}{}             // preload the full burst...
	tokens <- struct{}{}             // ...so 2 requests pass instantly
	done := make(chan struct{})
	go refill(ctx, tokens, done)

	start := time.Now()
	instant := 0
	for req := 1; req <= 4; req++ {
		select {
		case <-tokens: // token already banked: serve instantly
			instant++
		default:
			<-tokens // bucket empty: block until the next refill
		}
	}
	elapsed := time.Since(start) // requests 3 and 4 each waited a tick

	fmt.Println("burst served instantly:", instant)
	fmt.Println("refills gated the rest:", elapsed >= 110*time.Millisecond)

	cancel() // tell the refiller to quit
	<-done   // blocks until refill's close(done): proof it exited
	fmt.Println("refiller stopped: true")
}
```

**Output:**

```
burst served instantly: 2
refills gated the rest: true
refiller stopped: true
```

---

## 37. Heartbeats: prove the worker is alive

`🔴 hard` · *Patterns*

A timeout alone can't tell *slow-but-alive* from *dead* — the heartbeat pattern (from Katherine Cox-Buday's *Concurrency in Go*) has the worker pulse on a channel while it works, so a real watchdog can reset its timer on each pulse and kill the worker only when the pulses lapse.

**Steps:**

1. Write `worker(ctx)` returning `(heartbeat <-chan struct{}, result <-chan int)`: a goroutine ticks every 40ms via `time.NewTicker` and delivers `42` on `result` after 200ms of "work" (`time.After`).
2. Send each pulse with a nested `select` + `default` — non-blocking, so a slow listener never stalls the worker.
3. In `main`, monitor with a `select` loop: count `heartbeat` pulses, print and return on `result`, and treat `<-ctx.Done()` as the worker-died failure case (a generous 2s timeout that never fires here).
4. Run `go run .` — by 200ms the 40ms ticker has pulsed at least 4 times, so `beats >= 3` prints `true`, then `result: 42`.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// worker pulses on heartbeat every 40ms while it "works" for 200ms.
func worker(ctx context.Context) (<-chan struct{}, <-chan int) {
	heartbeat := make(chan struct{})
	result := make(chan int)
	go func() {
		pulse := time.NewTicker(40 * time.Millisecond)
		defer pulse.Stop()
		workDone := time.After(200 * time.Millisecond) // the "work"
		for {
			select {
			case <-ctx.Done():
				return
			case <-pulse.C:
				select { // non-blocking send: a slow listener never stalls us
				case heartbeat <- struct{}{}:
				default: // nobody listening right now — drop the pulse
				}
			case <-workDone:
				result <- 42
				return
			}
		}
	}()
	return heartbeat, result
}

func main() {
	// Generous 2s deadline: the watchdog's failure case, never hit here.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	heartbeat, result := worker(ctx)
	beats := 0
	for {
		select {
		case <-heartbeat:
			beats++ // a real watchdog resets its kill-timer on every pulse
		case r := <-result:
			fmt.Println("heartbeats seen: at least 3:", beats >= 3)
			fmt.Println("result:", r)
			return
		case <-ctx.Done(): // worker went silent too long: declare it dead
			fmt.Println("worker died:", ctx.Err())
			return
		}
	}
}
```

**Output:**

```
heartbeats seen: at least 3: true
result: 42
```

---

## 38. Singleflight: collapse duplicate fetches

`🔴 hard` · *Patterns*

When 5 requests ask for the same uncached key at once, only one should hit the database — the rest should wait and share that result. This "singleflight" pattern is the standard defense against cache stampedes.

**Steps:**

1. `flight` keeps a `map[string]*call` of in-flight work under a `sync.Mutex`; each `call` has a `done` channel and a `val`.
2. `Do(key, fn)`: if an entry exists, unlock and wait on `<-c.done`, then return the shared `c.val`. Otherwise register a new `call`, run `fn` **once**, fill `val`, `close(done)` to publish, and delete the entry so a *later* call fetches fresh.
3. Five goroutines released by one `close(start)` gate all call `Do("user:1", fetch)`; `fetch` sleeps 80ms and bumps an `atomic.Int32`.
4. The counter proves `fetch` ran once, and all five callers got the same value. Unlike `sync.Once` it resets per key after completion — in real code use `golang.org/x/sync/singleflight`.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// call is one in-flight (or finished) fetch that callers can wait on.
type call struct {
	done chan struct{}
	val  string
}

// flight dedupes concurrent calls by key: the first caller runs fn,
// everyone else waits for that same result.
type flight struct {
	mu    sync.Mutex
	calls map[string]*call
}

func (f *flight) Do(key string, fn func() string) string {
	f.mu.Lock()
	if c, ok := f.calls[key]; ok { // someone is already fetching this key
		f.mu.Unlock()
		<-c.done // wait for their result
		return c.val
	}
	c := &call{done: make(chan struct{})}
	f.calls[key] = c
	f.mu.Unlock()

	c.val = fn()  // only the first caller pays the cost
	close(c.done) // publish: the close happens after the val write

	f.mu.Lock()
	delete(f.calls, key) // a LATER call for this key fetches fresh
	f.mu.Unlock()
	return c.val
}

func main() {
	var fetches atomic.Int32
	f := &flight{calls: make(map[string]*call)}

	fetch := func() string { // the expensive, duplicated work
		fetches.Add(1)
		time.Sleep(80 * time.Millisecond)
		return "user-1-data"
	}

	start := make(chan struct{})
	results := make(chan string)
	for i := 0; i < 5; i++ {
		go func() {
			<-start // release all 5 at once
			results <- f.Do("user:1", fetch)
		}()
	}
	close(start)

	same := true
	for i := 0; i < 5; i++ {
		if <-results != "user-1-data" {
			same = false
		}
	}
	fmt.Println("fetches:", fetches.Load())
	fmt.Println("all 5 got the same value:", same)
}
```

**Output:**

```
fetches: 1
all 5 got the same value: true
```

---

## 39. Acquire a semaphore — or give up via ctx

`🔴 hard` · *Patterns*

A buffered channel used as a semaphore (channels lesson, examples 19/29) bounds concurrency, but a plain `sem <- struct{}{}` blocks forever; wrapping it in a `select` against `ctx.Done()` gives bounded concurrency that still respects each request's deadline — `x/sync/semaphore.Weighted` is the production version of this exact pattern.

**Steps:**

1. Write `acquire(ctx, sem)`: a `select` that races the buffered send `sem <- struct{}{}` (slot won, return `nil`) against `<-ctx.Done()` (give up, return `ctx.Err()`).
2. Make `sem` with capacity 2; A and B acquire instantly, and each starts a goroutine that releases its slot (`<-sem`) after 200ms.
3. C calls `acquire` with a 60ms `context.WithTimeout` — slots free only at 200ms, so C's deadline fires first and it prints `context deadline exceeded`.
4. D calls `acquire` with an 800ms budget — at 200ms a holder releases, D's send succeeds, and it prints `acquired`.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// acquire takes one slot from sem — or gives up as soon as ctx is done.
// The select races the buffered send against cancellation.
func acquire(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}: // a slot was free (or became free in time)
		return nil
	case <-ctx.Done(): // deadline hit while still waiting
		return ctx.Err()
	}
}

func main() {
	sem := make(chan struct{}, 2) // capacity 2 = at most 2 holders at once

	// A and B grab both slots instantly; each holds its slot for 200ms.
	for _, name := range []string{"A", "B"} {
		if err := acquire(context.Background(), sem); err == nil {
			fmt.Println(name + ": acquired")
		}
		go func() {
			time.Sleep(200 * time.Millisecond)
			<-sem // release: free one slot
		}()
	}

	// C will only wait 60ms — slots free at 200ms (>=3x later), so C fails.
	ctxC, cancelC := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancelC()
	if err := acquire(ctxC, sem); err != nil {
		fmt.Println("C:", err) // ctx.Err() prints "context deadline exceeded"
	}

	// D budgets 800ms — plenty to outlive the 200ms holders, so D gets a slot.
	ctxD, cancelD := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancelD()
	if err := acquire(ctxD, sem); err == nil {
		fmt.Println("D: acquired")
	}
}
```

**Output:**

```
A: acquired
B: acquired
C: context deadline exceeded
D: acquired
```

---

## 40. Capstone: a tiny job scheduler

`🔴 hard` · *Capstone*

Worker pool + per-job timeout + safe shared results + graceful drain — combined into one 85-line scheduler. This is the shape of every real ingest service: workers pull from a closed channel, each unit of work gets its own deadline, and a mutex plus atomics keep the bookkeeping race-free.

**Steps:**

1. Reuse `sleepCtx` (example 28) so each job's "work" aborts the moment its context fires.
2. Write `worker` as the example-21 loop: `select` on `ctx.Done()` and `j, open := <-jobs`, returning when the channel is closed and drained — and wrap every job in its own `context.WithTimeout(parent, 90*time.Millisecond)`.
3. Record each outcome in the mutex-guarded `results` map and bump the `okN`/`timedOut` atomics; `defer exit.Add(1)` counts graceful worker exits.
4. Send 8 jobs with durations 30/30/150/30/150/30/30/30 ms, `close(jobs)`, `wg.Wait()`, then print the map sorted by job id — jobs 3 and 5 (150ms > 90ms budget) time out, the rest succeed.

```go
// Capstone: 3 ctx-aware workers drain a closed jobs channel; each job runs
// under its OWN 90ms deadline; a mutex guards results, atomics keep tallies.
package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type job struct{ id, ms int } // ms: how long the "work" takes

// sleepCtx (example 28): a sleep that aborts early if ctx fires first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	parent := context.Background()
	jobs := make(chan job)
	var (
		mu                  sync.Mutex
		results             = make(map[int]string) // shared: every access under mu
		okN, timedOut, exit atomic.Int32
		wg                  sync.WaitGroup
	)

	worker := func(ctx context.Context) {
		defer wg.Done()
		defer exit.Add(1)
		for { // the example-21 loop: ctx OR work, whichever is ready
			select {
			case <-ctx.Done(): // shutdown path (unused here: parent never fires)
				return
			case j, open := <-jobs:
				if !open {
					return // channel closed and drained: graceful exit
				}
				// Each job gets its OWN deadline, independent of its siblings.
				jctx, cancel := context.WithTimeout(parent, 90*time.Millisecond)
				status := "ok"
				if sleepCtx(jctx, time.Duration(j.ms)*time.Millisecond) != nil {
					status = "timeout" // deadline beat the work (150ms > 90ms)
					timedOut.Add(1)
				} else {
					okN.Add(1)
				}
				cancel() // release the deadline timer per job, not at worker exit
				mu.Lock()
				results[j.id] = status
				mu.Unlock()
			}
		}
	}

	wg.Add(3)
	for range 3 {
		go worker(parent)
	}
	for i, ms := range []int{30, 30, 150, 30, 150, 30, 30, 30} {
		jobs <- job{id: i + 1, ms: ms}
	}
	close(jobs) // no more work: workers finish in-flight jobs and return
	wg.Wait()   // all results are written before we read the map below

	ids := make([]int, 0, len(results))
	for id := range results {
		ids = append(ids, id)
	}
	slices.Sort(ids) // map order is random: sort for deterministic output
	for _, id := range ids {
		fmt.Printf("job %d: %s\n", id, results[id])
	}
	fmt.Printf("ok: %d, timed out: %d, workers exited: %d/3\n", okN.Load(), timedOut.Load(), exit.Load())
}
```

**Output:**

```
job 1: ok
job 2: ok
job 3: timeout
job 4: ok
job 5: timeout
job 6: ok
job 7: ok
job 8: ok
ok: 6, timed out: 2, workers exited: 3/3
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
