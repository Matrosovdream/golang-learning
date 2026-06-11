# Step 15 — Sync, Context & Patterns · 🟡 Medium — examples **11–28**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # most examples here are concurrent — try go run -race . too
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 11. Mutex or channel? The same problem, twice

`🟡 medium` · *Design*

The same word-counting job solved both ways: a `sync.Mutex` protects one map that all 4 workers write, while a channel transfers each word to a single owner goroutine that counts with no lock at all. The rule: mutexes protect shared state, channels transfer ownership — pick whichever is simpler, and do not force channels everywhere.

**Steps:**

1. Write `fanOut(handle)`: one goroutine per entry in `chunks` (via `wg.Go`) calls `handle(w)` for every word, then `wg.Wait()` blocks until all 4 finish.
2. Version (a): all workers do `shared[w]++` directly, so every access is wrapped in `mu.Lock()` / `mu.Unlock()` — the map is shared state.
3. Version (b): workers only send on the `words` channel; one owner goroutine ranges over it and does `owned[w]++` lock-free. After `fanOut` returns, `close(words)` ends the owner's loop, and `<-done` guarantees counting finished before main reads the map.
4. `dump` sorts the `k=v` pairs before printing — both lines must come out identical.

```go
package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var chunks = []string{"go run go build", "go vet run test", "build vet go go", "fmt test go run"}

// fanOut runs one goroutine per chunk and calls handle(w) for every word.
func fanOut(handle func(string)) {
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Go(func() { // wg.Go = Add(1) + go + Done in one call (Go 1.25+)
			for _, w := range strings.Fields(chunk) {
				handle(w)
			}
		})
	}
	wg.Wait()
}

func dump(label string, m map[string]int) {
	var pairs []string
	for k, v := range m {
		pairs = append(pairs, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(pairs) // map iteration order is random: sort before printing
	fmt.Printf("%-8s %s\n", label, strings.Join(pairs, " "))
}

func main() {
	// (a) SHARED STATE: all 4 workers write one map -> guard it with a Mutex.
	shared, mu := map[string]int{}, sync.Mutex{}
	fanOut(func(w string) {
		mu.Lock()
		shared[w]++ // every access to shared happens inside the lock
		mu.Unlock()
	})
	dump("mutex:", shared)
	// (b) OWNERSHIP: workers only SEND; one owner goroutine counts, lock-free.
	owned, words, done := map[string]int{}, make(chan string), make(chan struct{})
	go func() { // the owner: sole writer of owned, so no Mutex needed
		for w := range words {
			owned[w]++
		}
		close(done)
	}()
	fanOut(func(w string) { words <- w })
	close(words) // all senders are finished -> ends the owner's range loop
	<-done       // owner done counting; only now is reading owned race-free
	dump("channel:", owned)
}
```

**Output:**

```
mutex:   build=2 fmt=1 go=6 run=3 test=2 vet=2
channel: build=2 fmt=1 go=6 run=3 test=2 vet=2
```

---

## 12. SafeCounter: 100 × 100 = 10000

`🟡 medium` · *Mutex*

The canonical mutex exercise: 100 goroutines each increment a shared counter 100 times, and the lock guarantees the final value is exactly 10000 — without it, `n++` (a read-modify-write) loses updates under contention.

**Steps:**

1. Define `SafeCounter` with a `sync.Mutex` and an `int`; give it pointer-receiver methods `Inc()` and `Value()` that `Lock` and `defer Unlock` around every access.
2. In `main`, start 100 goroutines via `wg.Add(1)`/`defer wg.Done()`, each calling `c.Inc()` 100 times in a loop.
3. After `wg.Wait()`, print `c.Value()` — then delete the Lock/Unlock in `Inc` and rerun with `go run -race .` to watch the detector catch the race.

```go
package main

import (
	"fmt"
	"sync"
)

// SafeCounter wraps a plain int with a mutex so any number of
// goroutines can increment it without losing updates.
type SafeCounter struct {
	mu sync.Mutex
	n  int
}

// Inc must use a pointer receiver: a value receiver would copy the
// struct — mutex included — and every caller would lock its own copy
// (that broken variant is example 4).
func (c *SafeCounter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock() // unlocks even if the body panics
	c.n++
}

// Value takes the same lock, so reads see a fully written n.
func (c *SafeCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func main() {
	var c SafeCounter // zero value is ready: unlocked mutex, n == 0
	var wg sync.WaitGroup

	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()

	// Delete Lock/Unlock in Inc and rerun with `go run -race .`:
	// the race detector reports the data race and the total drifts
	// below 10000. With the mutex it is exactly 100 × 100.
	fmt.Println("final:", c.Value())
}
```

**Output:**

```
final: 10000
```

---

## 13. Keep the critical section small

`🟡 medium` · *Mutex*

A mutex only serializes the code between `Lock` and `Unlock` — so the less you put there, the more your goroutines can overlap. Doing slow work while holding the lock turns a parallel program into a sequential one.

**Steps:**

1. Write `runRound(holdDuringWork bool)` that starts 3 workers, each doing 60ms of "computation" (`time.Sleep(work)`) plus one `append` to a shared `results` slice, and returns the total elapsed time.
2. In the bad branch, call `mu.Lock()` *before* the sleep — every worker holds the mutex for its full 60ms, so the three runs back to back.
3. In the good branch, sleep first, then `mu.Lock()` only around `results = append(results, i)` — the slow parts overlap and only the tiny write is serialized.
4. In `main`, run both rounds and print booleans from the elapsed times: the bad round takes ≥ 170ms (3 × 60ms queued up), the good round stays under 120ms (~60ms).

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

const work = 60 * time.Millisecond // simulated computation per worker

// runRound starts 3 workers that each "compute" for 60ms and append one
// result to a shared slice. holdDuringWork picks where the Lock goes.
func runRound(holdDuringWork bool) time.Duration {
	var (
		mu      sync.Mutex
		results []int
		wg      sync.WaitGroup
	)
	start := time.Now()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if holdDuringWork {
				mu.Lock()
				time.Sleep(work)             // BAD: slow work inside the lock
				results = append(results, i) // others wait the full 60ms
				mu.Unlock()
			} else {
				time.Sleep(work) // GOOD: slow work outside the lock
				mu.Lock()
				results = append(results, i) // lock guards only the write
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return time.Since(start)
}

func main() {
	bad := runRound(true)   // workers queue up: 3 × 60ms ≥ 180ms
	good := runRound(false) // workers overlap: ~60ms total

	fmt.Println("lock held during work — serialized:", bad >= 170*time.Millisecond)
	fmt.Println("lock only for the write — parallel:", good < 120*time.Millisecond)
}
```

**Output:**

```
lock held during work — serialized: true
lock only for the write — parallel: true
```

---

## 14. TryLock: skip, don't wait (Go 1.18+)

`🟡 medium` · *Mutex*

`mu.TryLock()` (added in Go 1.18) attempts to take the mutex and returns `false` immediately if it's held, instead of blocking — the pattern for "skip this tick if the previous one is still running" loops. It's rare but real; the docs themselves warn that needing it is often a sign of a design problem, so reach for it deliberately.

**Steps:**

1. A holder goroutine calls `mu.Lock()`, signals via `close(locked)`, keeps the lock for 150ms, then unlocks.
2. Main waits on `<-locked`, sleeps 50ms (mid-hold), and calls `mu.TryLock()` — it returns `false` instantly, so print `busy — skipped this round`.
3. After `wg.Wait()` the mutex is free: the second `mu.TryLock()` returns `true` — print `idle — got it` and `Unlock` (a `true` return means you own it).

```go
// TryLock (Go 1.18+): try to take a mutex WITHOUT blocking.
// It returns true (you got it, you must Unlock) or false (someone
// else holds it) — it never waits.
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup

	locked := make(chan struct{}) // proves the holder owns mu before we try

	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock() // holder grabs the lock...
		close(locked)
		time.Sleep(150 * time.Millisecond) // ...and stays busy with it
		mu.Unlock()
	}()

	<-locked                          // holder definitely has the lock now
	time.Sleep(50 * time.Millisecond) // we arrive mid-hold (50ms << 150ms)

	// A plain Lock() here would stall ~100ms. TryLock answers instantly.
	if mu.TryLock() {
		fmt.Println("idle — got it")
		mu.Unlock()
	} else {
		fmt.Println("busy — skipped this round")
	}

	wg.Wait() // holder finished and released mu

	if mu.TryLock() { // free mutex → true; we now own it
		fmt.Println("idle — got it")
		mu.Unlock()
	} else {
		fmt.Println("busy — skipped this round")
	}
}
```

**Output:**

```
busy — skipped this round
idle — got it
```

---

## 15. A read-through cache with RWMutex

`🟡 medium` · *RWMutex*

`sync.RWMutex` lets many readers hold the lock simultaneously while writers wait for exclusive access — it pays off when reads vastly outnumber writes, as in a config or lookup cache. Until profiling proves contention, a plain `Mutex` is fine; reach for `RWMutex` deliberately, not by default.

**Steps:**

1. Define `Cache` with `mu sync.RWMutex` and `m map[string]string`; `Get` uses `RLock`/`RUnlock`, `Set` uses `Lock`/`Unlock`.
2. Seed `"config" -> "v1"`, then launch 4 goroutines that each `Get` and send the result into a buffered channel, synchronized with a `WaitGroup`.
3. After `wg.Wait()`, close the channel, drain it into a slice, and print the four reads.
4. Call `Set("config", "v2")` and show the final `Get` sees the overwrite.

```go
package main

import (
	"fmt"
	"strings"
	"sync"
)

// Cache lets any number of readers in at once; writers get exclusive access.
type Cache struct {
	mu sync.RWMutex
	m  map[string]string
}

// Get takes the read lock: concurrent Gets overlap instead of queueing.
func (c *Cache) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[key]
}

// Set takes the write lock: it waits for readers to drain, then blocks
// new ones until the map update is done — no torn reads possible.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = value
}

func main() {
	cache := &Cache{m: make(map[string]string)}
	cache.Set("config", "v1") // seed before any reader starts

	results := make(chan string, 4) // buffered channel collects results race-free
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- cache.Get("config") // all 4 RLocks can be held at once
		}()
	}
	wg.Wait()
	close(results)

	var reads []string
	for v := range results {
		reads = append(reads, v) // all "v1": Set("v2") hasn't happened yet
	}
	fmt.Println("4 concurrent reads:", strings.Join(reads, " "))

	cache.Set("config", "v2") // exclusive write overwrites the entry
	fmt.Println("after Set:", cache.Get("config"))
}
```

**Output:**

```
4 concurrent reads: v1 v1 v1 v1
after Set: v2
```

---

## 16. atomic.Bool as a stop flag

`🟡 medium` · *Atomics*

A shared flag is the simplest cooperative stop: the worker polls it and exits on its own, while `context` (example 8) remains the richer tool for real cancellation. With a plain `bool` that polling is a data race (swap it in and run `go run -race .` to see the warning) — `atomic.Bool` fixes it with `Load`/`Store`.

**Steps:**

1. Declare `var stop atomic.Bool` shared between `main` and a worker that loops over a fixed slice of 8 `jobs`.
2. In the worker, call `stop.Load()` before blocking on the unbuffered `proceed` gate and again after waking — the flag can flip while you wait — then confirm each finished job on `ack`.
3. In `main`, let exactly 3 jobs through with `proceed <- struct{}{}` / `<-ack`, then `stop.Store(true)` and `close(proceed)` so the loop runs once more and sees the flag.
4. Receive the `result` from `done` and print the processed count and whether the worker saw the stop.

```go
package main

import (
	"fmt"
	"sync/atomic"
)

type result struct {
	processed int
	sawStop   bool
}

func main() {
	var stop atomic.Bool // make this a plain bool and -race reports a data race
	jobs := []int{1, 2, 3, 4, 5, 6, 7, 8}

	proceed := make(chan struct{}) // unbuffered gate: main paces the worker
	ack := make(chan struct{})     // worker confirms each finished job
	done := make(chan result)

	go func() {
		var r result
		for range jobs {
			if stop.Load() { // check before waiting for the next job...
				r.sawStop = true
				break
			}
			<-proceed        // block until main opens the gate
			if stop.Load() { // ...and again after waking: the flag can flip mid-wait
				r.sawStop = true
				break // cooperative stop: the worker chooses to exit
			}
			r.processed++
			fmt.Printf("worker: job %d done\n", r.processed)
			ack <- struct{}{}
		}
		done <- r
	}()

	for i := 0; i < 3; i++ {
		proceed <- struct{}{} // let one job through...
		<-ack                 // ...and wait until it is fully processed
	}
	stop.Store(true) // raise the flag
	close(proceed)   // open the gate for good so the loop can run and see it

	r := <-done
	fmt.Printf("processed: %d of %d\n", r.processed, len(jobs))
	fmt.Printf("stop seen: %v\n", r.sawStop)
}
```

**Output:**

```
worker: job 1 done
worker: job 2 done
worker: job 3 done
processed: 3 of 8
stop seen: true
```

---

## 17. atomic.Pointer[T]: lock-free snapshot swap (Go 1.19+)

`🟡 medium` · *Atomics*

Go 1.19's typed `atomic.Pointer[T]` lets readers `Load()` a whole struct pointer in one lock-free step — every reader sees a complete old or complete new snapshot, never a torn mix, which is the standard idiom for hot-reloaded config.

**Steps:**

1. Declare `var cfg atomic.Pointer[Config]` and seed it with `cfg.Store(&Config{TimeoutMS: 30, Debug: false})` — unlike `atomic.Value`, `Load()` returns `*Config` directly, no type assertion needed.
2. In `handle`, call `cfg.Load()` once and read `TimeoutMS` and `Debug` off that local pointer — the snapshot can't change underneath the reader.
3. Sequence a reader before and after the swap with a `sync.WaitGroup`, then `Store(&Config{TimeoutMS: 60, Debug: true})` to hot-reload in a single atomic step.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Config is an immutable snapshot: a writer never mutates a stored
// Config in place — it Stores a brand-new pointer instead.
type Config struct {
	TimeoutMS int
	Debug     bool
}

// Go 1.19+ typed atomic: Load returns *Config directly. The older
// atomic.Value returns any, forcing v.Load().(*Config) assertions;
// atomic.Pointer[Config] is checked at compile time instead.
var cfg atomic.Pointer[Config]

func handle(id int) {
	c := cfg.Load() // one atomic read = one complete snapshot
	// c can't change underneath us even if main swaps cfg right now.
	fmt.Printf("request %d: timeout=%dms debug=%v\n", id, c.TimeoutMS, c.Debug)
}

func main() {
	cfg.Store(&Config{TimeoutMS: 30, Debug: false})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); handle(1) }() // reader before the swap
	wg.Wait()                                  // sequence: reader 1 finishes first

	// Hot reload: swap the whole pointer in one atomic step. Readers
	// observe either the complete old Config {30,false} or the complete
	// new one {60,true} — never a torn mix like {60,false}.
	cfg.Store(&Config{TimeoutMS: 60, Debug: true})
	fmt.Println("main: stored new config")

	wg.Add(1)
	go func() { defer wg.Done(); handle(2) }() // reader after the swap
	wg.Wait()
}
```

**Output:**

```
request 1: timeout=30ms debug=false
main: stored new config
request 2: timeout=60ms debug=true
```

---

## 18. sync.Pool recycles allocations

`🟡 medium` · *Pool*

`sync.Pool` caches short-lived scratch objects (buffers, encoders) so hot paths can reuse them instead of allocating fresh ones — but the GC may empty the pool at any moment, so it must never hold state you cannot rebuild. A `newCount` counter inside `New` proves exactly when the pool allocates and when it recycles.

**Steps:**

1. Build a `sync.Pool` whose `New` func increments `newCount` and returns `new(bytes.Buffer)`; `New` only fires when the pool has nothing to hand out.
2. `pool.Get()` the first buffer (`newCount` becomes 1), write to it, then `buf.Reset()` **before** `pool.Put(buf)` so the next user never sees stale bytes.
3. `Get` again: `newCount` is still 1, so print `reused: true` — the counter, not pointer comparison, proves the buffer was recycled.
4. `Get` a third buffer while `buf2` is still checked out: the pool is empty again, so `New` fires and `newCount` becomes 2.

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
)

func main() {
	newCount := 0 // plain int is fine: this demo is single-goroutine

	pool := sync.Pool{
		// New runs only when the pool has no spare object to hand out.
		New: func() any {
			newCount++
			return new(bytes.Buffer)
		},
	}

	// First Get: pool is empty, so New fires -> newCount becomes 1.
	buf := pool.Get().(*bytes.Buffer)
	fmt.Println("after first Get, newCount =", newCount)

	buf.WriteString("hello, pool")
	fmt.Println("buffer holds:", buf.String())

	// ALWAYS Reset before Put: the next user must not see stale bytes.
	buf.Reset()
	pool.Put(buf)

	// Second Get: the pool hands back the recycled buffer, New does not run.
	buf2 := pool.Get().(*bytes.Buffer)
	fmt.Println("after second Get, newCount =", newCount)
	fmt.Println("reused:", newCount == 1) // counter proves no new allocation
	fmt.Println("recycled buffer is empty:", buf2.Len() == 0)

	// Third Get while buf2 is still checked out: pool is empty again,
	// so New must allocate a fresh buffer -> newCount becomes 2.
	buf3 := pool.Get().(*bytes.Buffer)
	fmt.Println("after third Get, newCount =", newCount)

	_ = buf3
	// Pool is scratch space only: the GC may drop pooled objects at any
	// time, so never park state in a Pool that you cannot rebuild.
}
```

**Output:**

```
after first Get, newCount = 1
buffer holds: hello, pool
after second Get, newCount = 1
reused: true
recycled buffer is empty: true
after third Get, newCount = 2
```

---

## 19. sync.Map: LoadOrStore, and when not to use it

`🟡 medium` · *sync.Map*

`sync.Map` is a concurrency-safe map whose `LoadOrStore` atomically inserts a key only if it is absent — perfect for "first goroutine wins" initialization. But its values are `any` (no type safety), and it only beats a `Mutex`+map for write-once/read-many or disjoint-key workloads — the package doc itself says to prefer a plain map with a Mutex by default.

**Steps:**

1. Launch 4 goroutines that all call `m.LoadOrStore("session", ...)`; exactly one stores, the rest get `loaded == true` and bump an `atomic.Int32`.
2. After `wg.Wait()`, print `stored fresh: 1, got existing: 3` from the counter.
3. `Store` two more keys, then `Range` to collect all keys — asserting each `any` key back to `string`.
4. `sort.Strings` the keys before printing, because `Range` order is unspecified.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

func main() {
	var m sync.Map // zero value is ready to use; no make() needed
	var gotExisting atomic.Int32
	var wg sync.WaitGroup

	// 4 goroutines race to claim the same key. LoadOrStore is atomic:
	// exactly one goroutine stores, the other three load the winner's value.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, loaded := m.LoadOrStore("session", fmt.Sprintf("token-%d", i))
			if loaded { // true = key already existed, our value was discarded
				gotExisting.Add(1)
			}
		}()
	}
	wg.Wait()
	fmt.Printf("stored fresh: %d, got existing: %d\n",
		4-gotExisting.Load(), gotExisting.Load())

	// Now use it as a plain concurrent map.
	m.Store("user", "alice")
	m.Store("theme", "dark")

	var keys []string
	m.Range(func(k, v any) bool { // k and v are any: no compile-time type safety
		keys = append(keys, k.(string)) // must assert back to the real type
		return true                     // false would stop the iteration
	})
	sort.Strings(keys) // Range order is unspecified — sort before printing
	fmt.Println("keys:", keys)
}
```

**Output:**

```
stored fresh: 1, got existing: 3
keys: [session theme user]
```

---

## 20. WaitGroup.Wait with a timeout

`🟡 medium` · *WaitGroup*

`wg.Wait()` blocks forever with no timeout option, so the standard idiom wraps it: wait in a goroutine that closes a channel, then `select` between that channel and `time.After`. Crucially, a timeout means you *abandoned* the workers — they are still running; in real code pair this with context cancellation (example 33) so they actually stop.

**Steps:**

1. Write `waitTimeout(wg *sync.WaitGroup, d time.Duration) bool`: spawn `go func() { wg.Wait(); close(done) }()`, then `select` between `<-done` (return `true`) and `<-time.After(d)` (return `false`).
2. Demo A: launch 3 workers that each sleep 30ms and wait with a 300ms budget — prints `finished in time: true`.
3. Demo B: launch one worker that sleeps 600ms and wait with a 150ms budget — `waitTimeout` gives up and returns `false` while the worker keeps running.
4. Call `wgB.Wait()` for real at the end so the demo doesn't exit with a leaked goroutine.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// waitTimeout waits for wg, but gives up after d.
// Returns true if the group finished in time, false on timeout.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait() // blocks until the counter hits zero
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		// CAUTION: the workers are STILL RUNNING — we abandoned
		// them, we did not stop them. Pair this with context
		// cancellation (example 33) so they actually exit.
		return false
	}
}

func main() {
	// Demo A: 3 quick workers, generous budget — finishes in time.
	var wgA sync.WaitGroup
	for i := 0; i < 3; i++ {
		wgA.Add(1)
		go func() {
			defer wgA.Done()
			time.Sleep(30 * time.Millisecond)
		}()
	}
	fmt.Println("finished in time:", waitTimeout(&wgA, 300*time.Millisecond))

	// Demo B: one slow worker, tight budget — we give up first.
	var wgB sync.WaitGroup
	wgB.Add(1)
	go func() {
		defer wgB.Done()
		time.Sleep(600 * time.Millisecond)
	}()
	ok := waitTimeout(&wgB, 150*time.Millisecond)
	fmt.Printf("finished in time: %v (gave up)\n", ok)

	wgB.Wait() // in this demo, wait for real so the goroutine isn't leaked
	fmt.Println("slow worker eventually finished")
}
```

**Output:**

```
finished in time: true
finished in time: false (gave up)
slow worker eventually finished
```

---

## 21. The cancellable worker loop

`🟡 medium` · *Patterns*

A long-running worker must be able to stop for two different reasons: the producer closed the jobs channel (graceful drain) or the caller cancelled the context (abort). The `for { select { <-ctx.Done() / <-jobs } }` loop handles both — and the worker reports *which one* happened.

**Steps:**

1. Write `worker(ctx, label, jobs, done)`: an infinite `for`/`select` that returns `ctx.Err().Error()` when `<-ctx.Done()` fires, and `"jobs closed"` when `j, ok := <-jobs` gives `ok == false`; each processed job bumps counter `n`.
2. Run 1: start a worker on unbuffered `jobs1`, send jobs `1` and `2` (each send blocks until the worker takes it), then `close(jobs1)` and read the `report` from `done1`.
3. Run 2: start a fresh worker with its own `ctx2`, send one job, then call `cancel2()` — the next `select` can only see `ctx.Done()`, so the worker stops with `context canceled`.

```go
package main

import (
	"context"
	"fmt"
)

// report is what a worker sends back when its loop exits.
type report struct {
	processed int
	reason    string
}

// worker is THE cancellable loop: each turn races "am I cancelled?" vs "got a job?".
func worker(ctx context.Context, label string, jobs <-chan int, done chan<- report) {
	n := 0
	for {
		select {
		case <-ctx.Done(): // cancel() or deadline fired
			done <- report{n, ctx.Err().Error()}
			return
		case j, ok := <-jobs:
			if !ok { // channel closed: producer says "no more work"
				done <- report{n, "jobs closed"}
				return
			}
			fmt.Printf("%s: processed job %d\n", label, j)
			n++
		}
	}
}

func main() {
	// Run 1: the producer closes the channel -> graceful drain.
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	jobs1 := make(chan int) // unbuffered: each send waits for the worker
	done1 := make(chan report)
	go worker(ctx1, "run 1", jobs1, done1)
	jobs1 <- 1
	jobs1 <- 2
	close(jobs1)
	r1 := <-done1
	fmt.Printf("run 1: processed=%d reason=%s\n", r1.processed, r1.reason)

	// Run 2: the caller cancels the context -> worker stops mid-stream.
	ctx2, cancel2 := context.WithCancel(context.Background())
	jobs2 := make(chan int)
	done2 := make(chan report)
	go worker(ctx2, "run 2", jobs2, done2)
	jobs2 <- 1 // unbuffered send returns only after the worker took job 1
	cancel2()  // now ctx.Done() is the only case that can ever fire
	r2 := <-done2
	fmt.Printf("run 2: processed=%d reason=%s\n", r2.processed, r2.reason)
}
```

**Output:**

```
run 1: processed job 1
run 1: processed job 2
run 1: processed=2 reason=jobs closed
run 2: processed job 1
run 2: processed=1 reason=context canceled
```

---

## 22. Race the work against the deadline

`🟡 medium` · *Context timeouts*

The classic timeout pattern: start the work in a goroutine, then `select` between its result channel and `ctx.Done()` — whichever fires first decides the outcome. The result channel must be buffered (cap 1) so a worker that finishes *after* the caller gave up can still deliver its value and exit, instead of blocking on the send forever and leaking.

**Steps:**

1. Write `doWork(d)` that returns `<-chan int` made with `make(chan int, 1)`; a goroutine sleeps `d` then sends `42`.
2. In `race`, build `ctx` with `context.WithTimeout` and `select` between `r := <-res` and `<-ctx.Done()`.
3. Run A with work 30ms vs timeout 200ms (work wins), run B with work 400ms vs timeout 100ms (deadline wins, print `ctx.Err()`).

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// doWork simulates a job that takes d to finish and delivers its result
// on the returned channel. The channel is BUFFERED with capacity 1: if
// the caller has already given up and walked away, the late worker can
// still drop its value into the buffer and exit. With an unbuffered
// channel the send would block forever — a leaked goroutine.
func doWork(d time.Duration) <-chan int {
	res := make(chan int, 1) // cap 1: send never blocks, worker never leaks
	go func() {
		time.Sleep(d) // pretend to compute
		res <- 42     // succeeds even if nobody is listening anymore
	}()
	return res
}

// race runs doWork(work) but refuses to wait longer than timeout.
func race(label string, work, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res := doWork(work)
	select { // whichever fires first wins
	case r := <-res:
		fmt.Printf("%s result: %d\n", label, r)
	case <-ctx.Done():
		fmt.Printf("%s gave up: %v\n", label, ctx.Err())
	}
}

func main() {
	race("A:", 30*time.Millisecond, 200*time.Millisecond)  // work wins easily
	race("B:", 400*time.Millisecond, 100*time.Millisecond) // deadline wins
}
```

**Output:**

```
A: result: 42
B: gave up: context deadline exceeded
```

---

## 23. WithDeadline, and reading it back

`🟡 medium` · *Context timeouts*

`context.WithDeadline` cancels at an absolute wall-clock time — `WithTimeout` is just sugar that adds a duration to `time.Now()` and calls it. The deadline is also *readable*: `ctx.Deadline()` lets libraries inspect their remaining budget and size retries or batches before starting work.

**Steps:**

1. Create `ctx` with `context.WithDeadline(context.Background(), time.Now().Add(150*time.Millisecond))` and `defer cancel()`.
2. Read it back with `dl, ok := ctx.Deadline()` and print `ok`, then print whether `time.Until(dl) > 100*time.Millisecond` — the remaining budget.
3. Show the contrast: `context.Background().Deadline()` returns `ok == false`.
4. Block on `<-ctx.Done()` and print `ctx.Err()`, which is `context.DeadlineExceeded`.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	// WithDeadline takes an ABSOLUTE time. WithTimeout(parent, d) is just
	// sugar for WithDeadline(parent, time.Now().Add(d)) — same machinery.
	deadline := time.Now().Add(150 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel() // always release the timer, even if the deadline fires

	// Any code holding the ctx can read the deadline back. Libraries use
	// this to budget work (retries, batch sizes) BEFORE starting it.
	dl, ok := ctx.Deadline()
	fmt.Println("deadline set:", ok)

	// time.Until(dl) is how much budget remains. We just created the ctx,
	// so well over 100ms of the 150ms budget is left (3x+ margin vs 0).
	fmt.Println("more than 100ms left:", time.Until(dl) > 100*time.Millisecond)

	// A context with no deadline reports ok=false — callers must check it.
	_, ok = context.Background().Deadline()
	fmt.Println("background has deadline:", ok)

	// Block until the deadline passes; Done() closes and Err() explains why.
	<-ctx.Done()
	fmt.Println("err:", ctx.Err()) // context.DeadlineExceeded
}
```

**Output:**

```
deadline set: true
more than 100ms left: true
background has deadline: false
err: context deadline exceeded
```

---

## 24. Cancellation flows down the context tree

`🟡 medium` · *Context basics*

Contexts form a tree, and cancelling a node closes the `Done()` channel of that node and every descendant — but never the parent. This is exactly why HTTP handlers derive per-request contexts from one server root: cancelling a request kills only its subtree.

**Steps:**

1. Build a three-level chain: `parent` and `child` via `context.WithCancel`, then `grandchild` via `context.WithTimeout(child, time.Hour)`.
2. Call `cancelParent()` once, receive from all three `Done()` channels, and print each `Err()` — three times `context.Canceled`; the 1h timer never mattered.
3. Make a fresh `parent2`/`child2` pair and cancel only the child: `child2.Err()` is `context.Canceled` while `parent2.Err()` stays `nil`.
4. Print `parent2.Err() == nil` to prove cancellation never travels upward.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	// Build a three-level tree: parent -> child -> grandchild.
	parent, cancelParent := context.WithCancel(context.Background())
	child, cancelChild := context.WithCancel(parent)
	grandchild, cancelGrand := context.WithTimeout(child, time.Hour) // 1h timer
	defer cancelChild()
	defer cancelGrand()

	// One cancel at the ROOT tears down the entire subtree.
	cancelParent()

	<-parent.Done() // all three Done channels are already closed
	<-child.Done()
	<-grandchild.Done()
	fmt.Println("parent.Err():    ", parent.Err())
	fmt.Println("child.Err():     ", child.Err())
	// The 1h deadline never mattered: cancellation arrived first,
	// so Err() is context.Canceled, not DeadlineExceeded.
	fmt.Println("grandchild.Err():", grandchild.Err())

	// Fresh pair: cancellation flows DOWN only, never up.
	parent2, cancelParent2 := context.WithCancel(context.Background())
	child2, cancelChild2 := context.WithCancel(parent2)
	defer cancelParent2()

	cancelChild2() // cancel only the child
	<-child2.Done()
	fmt.Println("child2.Err(): ", child2.Err())
	fmt.Println("parent2.Err():", parent2.Err()) // still nil: parent keeps running
	fmt.Println("parent unaffected:", parent2.Err() == nil)
}
```

**Output:**

```
parent.Err():     context canceled
child.Err():      context canceled
grandchild.Err(): context canceled
child2.Err():  context canceled
parent2.Err(): <nil>
parent unaffected: true
```

---

## 25. WithCancelCause: keep the real reason (Go 1.20+)

`🟡 medium` · *Context basics*

When you cancel a context, every waiter only sees the generic `context.Canceled` from `ctx.Err()` — `context.WithCancelCause` (Go 1.20+) lets the canceller attach the real error, which anyone can recover with `context.Cause(ctx)`. `Err` answers "is it dead"; `Cause` answers "who killed it and why" — so log `Cause`.

**Steps:**

1. Call `context.WithCancelCause(context.Background())` — the returned `cancel` is a `CancelCauseFunc` that takes an error argument.
2. Cancel with `cancel(errors.New("user pressed stop"))`, then print both `ctx.Err()` (generic) and `context.Cause(ctx)` (the real reason).
3. Make a plain `context.WithTimeout` context, wait for `<-tctx.Done()`, and show that with no explicit cause, `context.Cause(tctx)` falls back to `tctx.Err()` — both are `context.DeadlineExceeded`, so `cause matches err: true`.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func main() {
	// Go 1.20+: WithCancelCause is like WithCancel, but cancel takes an error
	// explaining WHY. Err() stays generic; Cause() keeps the real reason.
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("user pressed stop")) // record the real reason
	<-ctx.Done()

	fmt.Println("err:  ", ctx.Err())          // generic: context canceled
	fmt.Println("cause:", context.Cause(ctx)) // specific: user pressed stop

	// A plain WithTimeout ctx has no separate cause: after expiry,
	// Cause falls back to Err — both are context.DeadlineExceeded.
	tctx, tcancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer tcancel()
	<-tctx.Done() // wait until the deadline fires

	fmt.Println("timeout err:  ", tctx.Err())
	fmt.Println("timeout cause:", context.Cause(tctx))
	fmt.Println("cause matches err:", context.Cause(tctx) == tctx.Err())
}
```

**Output:**

```
err:   context canceled
cause: user pressed stop
timeout err:   context deadline exceeded
timeout cause: context deadline exceeded
cause matches err: true
```

---

## 26. context.AfterFunc: hook the cancellation (Go 1.21+)

`🟡 medium` · *Context basics*

`context.AfterFunc(ctx, f)` (Go 1.21+) arranges for `f` to run in its own goroutine as soon as `ctx` is done — the hook for releasing resources tied to a request: close connections, abort uploads. The returned `stop` function deregisters the callback; it reports `true` if it got there before `f` was started.

**Steps:**

1. Demo A: register a cleanup that sends on `ranA`, call `cancelA()`, then receive — the send proves the callback fired.
2. Demo B: on a fresh `ctxB`, register again but immediately call `stop()` — it returns `true`, meaning the callback was deregistered in time.
3. Cancel `ctxB` anyway, then `select` on `ranB` against a 100ms `time.After` to prove nothing arrives: `cleanup after stop(): false`.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	// Demo A: AfterFunc runs the callback in its own goroutine once ctx is done.
	ctxA, cancelA := context.WithCancel(context.Background())
	ranA := make(chan bool, 1)
	context.AfterFunc(ctxA, func() { // Go 1.21+
		ranA <- true // real code: close conns, abort uploads, release locks
	})
	cancelA()                           // ctx done -> callback fires
	fmt.Println("cleanup ran:", <-ranA) // receiving proves it ran

	// Demo B: the returned stop() deregisters the callback.
	ctxB, cancelB := context.WithCancel(context.Background())
	ranB := make(chan bool, 1)
	stop := context.AfterFunc(ctxB, func() { ranB <- true })
	fmt.Println("stop() returned:", stop()) // true: removed before it could run
	cancelB()                               // now cancellation triggers nothing

	ran := false
	select {
	case ran = <-ranB: // would only fire if the callback had run
	case <-time.After(100 * time.Millisecond): // generous window; stays silent
	}
	fmt.Println("cleanup after stop():", ran)
}
```

**Output:**

```
cleanup ran: true
stop() returned: true
cleanup after stop(): false
```

---

## 27. Timeouts bubble up a call chain

`🟡 medium` · *Patterns*

A single `context.WithTimeout` at the top bounds an entire `handler -> service -> repo` call tree, because every layer shares the same `ctx` — and by wrapping with `%w` at each hop (just like step 12), the original `context.DeadlineExceeded` stays reachable via `errors.Is` no matter how many labels pile on.

**Steps:**

1. Write `repo(ctx)` that races a 300ms `time.After` against `<-ctx.Done()` in a `select`, returning `fmt.Errorf("repo: %w", ctx.Err())` when the context wins.
2. Stack `service(ctx)` and `handler(ctx)` on top; each one only adds its own prefix with `%w` (`"service: %w"`, `"handler: %w"`) and passes `ctx` straight through.
3. In `main`, create the context with `context.WithTimeout(context.Background(), 80*time.Millisecond)` — 80ms is well under repo's 300ms, so the deadline fires first.
4. Print the final error and `errors.Is(err, context.DeadlineExceeded)`; the chain reads top-to-bottom and `errors.Is` still finds the sentinel at the very end.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// repo is the bottom of the stack: it pretends a query takes 300ms,
// but races that work against ctx.Done so the caller's deadline wins.
func repo(ctx context.Context) error {
	select {
	case <-time.After(300 * time.Millisecond): // simulated slow query
		return nil
	case <-ctx.Done():
		return fmt.Errorf("repo: %w", ctx.Err()) // wrap, don't swallow
	}
}

// service adds its own label but keeps the chain intact with %w.
func service(ctx context.Context) error {
	if err := repo(ctx); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

// handler is the top layer: it owns the deadline for the whole tree.
func handler(ctx context.Context) error {
	if err := service(ctx); err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	return nil
}

func main() {
	// One timeout here bounds handler+service+repo, because the same
	// ctx is passed all the way down. 80ms < 300ms, so repo gives up.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err := handler(ctx)
	fmt.Println("error:", err)
	// errors.Is walks the %w chain back to the sentinel at the bottom.
	fmt.Println("is DeadlineExceeded:", errors.Is(err, context.DeadlineExceeded))
}
```

**Output:**

```
error: handler: service: repo: context deadline exceeded
is DeadlineExceeded: true
```

---

## 28. sleepCtx: an interruptible sleep

`🟡 medium` · *Patterns*

`time.Sleep` cannot be cancelled — a goroutine stuck in it ignores shutdown signals until the timer runs out. Every Go service ends up writing `sleepCtx`: a `select` over `time.After(d)` and `ctx.Done()` so the sleep ends the moment either fires.

**Steps:**

1. Write `sleepCtx(ctx, d)`: `select` on `<-time.After(d)` (return `nil`) vs `<-ctx.Done()` (return `ctx.Err()`).
2. Run A: sleep 40ms under a 300ms `context.WithTimeout` — the sleep finishes, error is `<nil>`.
3. Run B: sleep 600ms under a 100ms timeout — the context fires first, returning `context deadline exceeded` immediately.
4. Prove the early wake-up by checking `time.Since(start) < 300*time.Millisecond` instead of printing a raw duration.

```go
package main

import (
	"context"
	"fmt"
	"time"
)

// sleepCtx is the cancellable replacement for time.Sleep. It returns nil
// if the full duration elapsed, or ctx.Err() if the context fired first.
// Note: time.After's timer is not freed until it fires — fine for short
// sleeps like these; in long-lived loops prefer time.NewTimer + Stop.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d): // the sleep finished normally
		return nil
	case <-ctx.Done(): // cancelled or deadline exceeded — wake up early
		return ctx.Err()
	}
}

func main() {
	// Run A: 40ms sleep, 300ms budget — the sleep wins comfortably.
	ctxA, cancelA := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err := sleepCtx(ctxA, 40*time.Millisecond)
	cancelA()
	fmt.Printf("run A completed: %v\n", err)

	// Run B: 600ms sleep, 100ms budget — the context fires first and
	// sleepCtx returns immediately; we never wait the full 600ms.
	ctxB, cancelB := context.WithTimeout(context.Background(), 100*time.Millisecond)
	start := time.Now()
	err = sleepCtx(ctxB, 600*time.Millisecond)
	cancelB()
	fmt.Printf("run B interrupted: %v\n", err)
	// Prove we woke early without printing a raw (nondeterministic) duration.
	fmt.Println("woke before the full 600ms:", time.Since(start) < 300*time.Millisecond)
}
```

**Output:**

```
run A completed: <nil>
run B interrupted: context deadline exceeded
woke before the full 600ms: true
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
