# Step 13 — Goroutines · Examples

A library of **42 runnable examples**. Each is a complete `package main` program:
read the concept and steps, then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
go run -race .   # concurrency: also try the race detector
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, run, AND verified clean under the **race detector** (`-race`). The **Output** is real stdout (examples are written to be deterministic).

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them.

## Index


**Easy**

- [1. Start a goroutine and wait for it](#1-start-a-goroutine-and-wait-for-it)
- [2. Why main must wait](#2-why-main-must-wait)
- [3. Many goroutines, results in an indexed slice](#3-many-goroutines-results-in-an-indexed-slice)
- [4. Loop-variable capture (Go 1.22+)](#4-loop-variable-capture-go-122)
- [5. Anonymous goroutine with an argument](#5-anonymous-goroutine-with-an-argument)

**Medium**

- [6. Pass *sync.WaitGroup to a helper](#6-pass-syncwaitgroup-to-a-helper)
- [7. Collect results over a channel, then sort](#7-collect-results-over-a-channel-then-sort)
- [8. Atomic counter](#8-atomic-counter)
- [9. Mutex-protected counter](#9-mutex-protected-counter)
- [10. sync.Once runs initialization exactly once](#10-synconce-runs-initialization-exactly-once)
- [11. Concurrent map over a slice](#11-concurrent-map-over-a-slice)
- [12. Add before go, defer Done inside](#12-add-before-go-defer-done-inside)
- [13. Concurrency vs parallelism (GOMAXPROCS)](#13-concurrency-vs-parallelism-gomaxprocs)
- [14. Fan-in: merge results from many goroutines](#14-fan-in-merge-results-from-many-goroutines)
- [23. RWMutex: many readers, one writer](#23-rwmutex-many-readers-one-writer)
- [24. sync.Map for concurrent access](#24-syncmap-for-concurrent-access)
- [26. atomic.Value for a shared snapshot](#26-atomicvalue-for-a-shared-snapshot)
- [28. Build a map concurrently under a Mutex](#28-build-a-map-concurrently-under-a-mutex)
- [29. Lazy singleton with sync.Once](#29-lazy-singleton-with-synconce)
- [30. A two-stage pipeline](#30-a-two-stage-pipeline)
- [31. Producer / consumer with close](#31-producer--consumer-with-close)
- [32. wg.Go (Go 1.25+)](#32-wggo-go-125)
- [34. Non-blocking receive with select/default](#34-non-blocking-receive-with-selectdefault)
- [35. Return a value from a goroutine](#35-return-a-value-from-a-goroutine)
- [36. defer LIFO inside a goroutine](#36-defer-lifo-inside-a-goroutine)
- [39. Count completed tasks atomically](#39-count-completed-tasks-atomically)

**Hard**

- [15. Worker pool](#15-worker-pool)
- [16. Give a goroutine a guaranteed exit (avoid leaks)](#16-give-a-goroutine-a-guaranteed-exit-avoid-leaks)
- [17. Bounded concurrency with a semaphore channel](#17-bounded-concurrency-with-a-semaphore-channel)
- [18. Collect errors from goroutines](#18-collect-errors-from-goroutines)
- [19. Parallel partial sums](#19-parallel-partial-sums)
- [20. Per-goroutine result structs, sorted](#20-per-goroutine-result-structs-sorted)
- [21. Closing a channel broadcasts to all receivers](#21-closing-a-channel-broadcasts-to-all-receivers)
- [22. Race-free shared counter (test with -race)](#22-race-free-shared-counter-test-with--race)
- [25. Nested goroutines](#25-nested-goroutines)
- [27. Lock-free max with CompareAndSwap](#27-lock-free-max-with-compareandswap)
- [33. Fan-out then fan-in](#33-fan-out-then-fan-in)
- [37. Worker pool with a results map](#37-worker-pool-with-a-results-map)
- [38. Parallel 'any match' with an atomic flag](#38-parallel-any-match-with-an-atomic-flag)
- [40. A two-phase barrier](#40-a-two-phase-barrier)
- [41. Cancel many workers by closing a channel](#41-cancel-many-workers-by-closing-a-channel)
- [42. Share memory by communicating](#42-share-memory-by-communicating)

---

## 1. Start a goroutine and wait for it

`🟢 easy` · *Basics*

Prefix a call with go to run it concurrently; main must explicitly wait, here with a sync.WaitGroup, or it could exit first.

**Steps:**

1. wg.Add(1) before launching, defer wg.Done() inside.
2. wg.Wait() blocks until the goroutine finishes, so the order is deterministic.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("goroutine: working")
	}()
	wg.Wait() // block until Done is called
	fmt.Println("main: goroutine finished")
}
```

**Output:**

```
goroutine: working
main: goroutine finished
```

---

## 2. Why main must wait

`🟢 easy` · *Basics*

When main returns the program exits immediately, killing running goroutines — so `go fmt.Println(...)` alone is unreliable. Wait explicitly (never with time.Sleep).

**Steps:**

1. The commented bare `go` call might never run.
2. A WaitGroup guarantees the goroutine completes first.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	// Unreliable — main may exit before this runs:
	//   go fmt.Println("hi")
	// Fix: wait explicitly with a WaitGroup.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("hi (reliable because main waits)")
	}()
	wg.Wait()
}
```

**Output:**

```
hi (reliable because main waits)
```

---

## 3. Many goroutines, results in an indexed slice

`🟢 easy` · *WaitGroup*

When each goroutine writes to its OWN index of a slice, there's no data race and no need for locks — and the result is deterministic.

**Steps:**

1. Launch n goroutines; goroutine i writes results[i].
2. wg.Wait(), then print the filled slice.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const n = 5
	results := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = i * i // distinct index per goroutine: no race
		}()
	}
	wg.Wait()
	fmt.Println(results) // [0 1 4 9 16]
}
```

**Output:**

```
[0 1 4 9 16]
```

---

## 4. Loop-variable capture (Go 1.22+)

`🟢 easy` · *WaitGroup*

Since Go 1.22 each loop iteration has its own copy of the loop variable, so a goroutine capturing i sees its own value (older Go needed it passed as an argument).

**Steps:**

1. Each goroutine appends its own i to a shared slice (guarded by a mutex).
2. Sort the result because completion order is nondeterministic.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	var mu sync.Mutex
	var got []int
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			got = append(got, i) // each goroutine's own i (Go 1.22+)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Ints(got) // order varies, so sort for stable output
	fmt.Println(got)
}
```

**Output:**

```
[0 1 2]
```

---

## 5. Anonymous goroutine with an argument

`🟢 easy` · *Basics*

You can launch an anonymous function as a goroutine and pass it arguments at the call.

**Steps:**

1. go func(msg string){...}("...") starts immediately.
2. Passing the value as an argument captures it explicitly.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func(msg string) {
		defer wg.Done()
		fmt.Println(msg)
	}("hello from a goroutine")
	wg.Wait()
}
```

**Output:**

```
hello from a goroutine
```

---

## 6. Pass *sync.WaitGroup to a helper

`🟡 medium` · *WaitGroup*

A WaitGroup must not be copied; if a function uses one, take it by pointer (*sync.WaitGroup).

**Steps:**

1. worker receives wg as a pointer and calls Done.
2. Each worker writes its own output slot, so output is ordered.

```go
package main

import (
	"fmt"
	"sync"
)

func worker(id int, wg *sync.WaitGroup, out []string) {
	defer wg.Done()
	out[id] = fmt.Sprintf("worker %d done", id)
}

func main() {
	var wg sync.WaitGroup
	out := make([]string, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go worker(i, &wg, out)
	}
	wg.Wait()
	for _, s := range out {
		fmt.Println(s)
	}
}
```

**Output:**

```
worker 0 done
worker 1 done
worker 2 done
```

---

## 7. Collect results over a channel, then sort

`🟡 medium` · *Collecting results*

A buffered channel gathers results from goroutines; close it after Wait, drain it, and sort for deterministic output.

**Steps:**

1. Each goroutine sends its result on the channel.
2. After wg.Wait, close and range the channel, then sort.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	nums := []int{3, 1, 4, 1, 5}
	ch := make(chan int, len(nums))
	var wg sync.WaitGroup
	for _, n := range nums {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- n * n
		}()
	}
	wg.Wait()
	close(ch)

	var squares []int
	for s := range ch {
		squares = append(squares, s)
	}
	sort.Ints(squares)
	fmt.Println(squares)
}
```

**Output:**

```
[1 1 9 16 25]
```

---

## 8. Atomic counter

`🟡 medium` · *Shared state*

sync/atomic provides race-free operations on shared integers, so concurrent increments give the exact total.

**Steps:**

1. 100 goroutines each AddInt64(&counter, 1).
2. The result is always exactly 100 — no lock needed.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var counter int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&counter, 1)
		}()
	}
	wg.Wait()
	fmt.Println("counter:", atomic.LoadInt64(&counter)) // 100
}
```

**Output:**

```
counter: 100
```

---

## 9. Mutex-protected counter

`🟡 medium` · *Shared state*

A sync.Mutex serializes access to shared state: lock, mutate, unlock — preventing data races on a plain variable.

**Steps:**

1. Each goroutine locks before count++ and unlocks after.
2. The total is always 100.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var mu sync.Mutex
	count := 0
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			count++
			mu.Unlock()
		}()
	}
	wg.Wait()
	fmt.Println("count:", count) // 100
}
```

**Output:**

```
count: 100
```

---

## 10. sync.Once runs initialization exactly once

`🟡 medium` · *sync primitives*

sync.Once guarantees its function runs a single time no matter how many goroutines call Do.

**Steps:**

1. Five goroutines all call once.Do(...).
2. The init message prints exactly once.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var once sync.Once
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			once.Do(func() {
				fmt.Println("initialized (runs exactly once)")
			})
		}()
	}
	wg.Wait()
	fmt.Println("done")
}
```

**Output:**

```
initialized (runs exactly once)
done
```

---

## 11. Concurrent map over a slice

`🟡 medium` · *WaitGroup*

Transform a slice in parallel by giving each goroutine one index to update — a lock-free parallel map.

**Steps:**

1. Goroutine i squares data[i] in place.
2. Disjoint indices mean no synchronization is required.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	data := []int{1, 2, 3, 4, 5}
	var wg sync.WaitGroup
	for i := range data {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data[i] *= data[i]
		}()
	}
	wg.Wait()
	fmt.Println(data) // [1 4 9 16 25]
}
```

**Output:**

```
[1 4 9 16 25]
```

---

## 12. Add before go, defer Done inside

`🟡 medium` · *WaitGroup*

Call wg.Add before launching (calling it inside races with Wait), and defer wg.Done so it runs even on an early return or panic.

**Steps:**

1. Add(1) sits before the go statement.
2. defer wg.Done() is the first line of the goroutine.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	tasks := []string{"a", "b", "c"}
	done := make([]bool, len(tasks))
	for i := range tasks {
		wg.Add(1) // before `go`, never inside the goroutine
		go func() {
			defer wg.Done()
			done[i] = true
		}()
	}
	wg.Wait()
	fmt.Println("all done:", done) // [true true true]
}
```

**Output:**

```
all done: [true true true]
```

---

## 13. Concurrency vs parallelism (GOMAXPROCS)

`🟡 medium` · *Scheduler*

Concurrency is how you structure independent work; parallelism is how many pieces run at once. GOMAXPROCS caps simultaneous execution.

**Steps:**

1. runtime.GOMAXPROCS(2) limits parallel execution to 2.
2. GOMAXPROCS(0) reads the current setting back.

```go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	runtime.GOMAXPROCS(2) // cap parallelism at 2
	fmt.Println("GOMAXPROCS now:", runtime.GOMAXPROCS(0))
}
```

**Output:**

```
GOMAXPROCS now: 2
```

---

## 14. Fan-in: merge results from many goroutines

`🟡 medium` · *Collecting results*

Several producer goroutines send into one shared channel; collect everything after they finish, then sort.

**Steps:**

1. Each producer sends three values into the channel.
2. Wait, close, drain, and sort the merged results.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func producer(start int, ch chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < 3; i++ {
		ch <- start + i
	}
}

func main() {
	ch := make(chan int, 9)
	var wg sync.WaitGroup
	for _, start := range []int{0, 10, 20} {
		wg.Add(1)
		go producer(start, ch, &wg)
	}
	wg.Wait()
	close(ch)

	var all []int
	for v := range ch {
		all = append(all, v)
	}
	sort.Ints(all)
	fmt.Println(all)
}
```

**Output:**

```
[0 1 2 10 11 12 20 21 22]
```

---

## 15. Worker pool

`🔴 hard` · *Worker pool*

A fixed number of worker goroutines pull from a jobs channel until it closes, sending results back — bounding concurrency for large workloads.

**Steps:**

1. 3 workers range over the jobs channel.
2. Close jobs to stop them; Wait, close results, then collect and sort.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	jobs := make(chan int, 10)
	results := make(chan int, 10)

	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs { // pull until jobs is closed
				results <- j * 2
			}
		}()
	}

	for j := 1; j <= 5; j++ {
		jobs <- j
	}
	close(jobs) // workers finish their range loops

	wg.Wait()
	close(results)

	var out []int
	for r := range results {
		out = append(out, r)
	}
	sort.Ints(out)
	fmt.Println(out) // [2 4 6 8 10]
}
```

**Output:**

```
[2 4 6 8 10]
```

---

## 16. Give a goroutine a guaranteed exit (avoid leaks)

`🔴 hard` · *Lifecycle & leaks*

A goroutine blocked forever leaks; every goroutine needs a way to stop. Here a quit channel (closed by main) unblocks it.

**Steps:**

1. The goroutine blocks on <-quit, its guaranteed exit.
2. close(quit) releases it; wg.Wait confirms a clean shutdown.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-quit // blocks until closed — the guaranteed exit
		fmt.Println("worker received quit, exiting")
	}()

	fmt.Println("main: signaling quit")
	close(quit) // broadcasts to the receiver
	wg.Wait()
	fmt.Println("main: no leaked goroutine")
}
```

**Output:**

```
main: signaling quit
worker received quit, exiting
main: no leaked goroutine
```

---

## 17. Bounded concurrency with a semaphore channel

`🔴 hard` · *Bounded concurrency*

A buffered channel used as a semaphore limits how many goroutines run at once: acquire before work, release after.

**Steps:**

1. sem has capacity maxConcurrent; sending acquires, receiving releases.
2. Track peak in-flight count; it never exceeds the limit.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent)
	var inFlight, maxSeen int64
	var wg sync.WaitGroup

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{} // acquire (blocks when full)
			cur := atomic.AddInt64(&inFlight, 1)
			for {
				old := atomic.LoadInt64(&maxSeen)
				if cur <= old || atomic.CompareAndSwapInt64(&maxSeen, old, cur) {
					break
				}
			}
			atomic.AddInt64(&inFlight, -1)
			<-sem // release
		}()
	}
	wg.Wait()
	fmt.Println("never exceeded limit:", atomic.LoadInt64(&maxSeen) <= maxConcurrent)
}
```

**Output:**

```
never exceeded limit: true
```

---

## 18. Collect errors from goroutines

`🔴 hard` · *Errors*

Give each goroutine its own error slot, then combine with errors.Join (which drops nils) — a clean way to aggregate concurrent failures.

**Steps:**

1. Each goroutine writes errs[i].
2. errors.Join(errs...) merges only the non-nil ones.

```go
package main

import (
	"errors"
	"fmt"
	"sync"
)

func task(id int) error {
	if id%2 == 0 {
		return fmt.Errorf("task %d failed", id)
	}
	return nil
}

func main() {
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = task(i)
		}()
	}
	wg.Wait()
	fmt.Println(errors.Join(errs...)) // nils ignored
}
```

**Output:**

```
task 0 failed
task 2 failed
```

---

## 19. Parallel partial sums

`🔴 hard` · *Parallel work*

Split work into disjoint chunks, sum each in its own goroutine into a private slot, then combine — no locks needed.

**Steps:**

1. Each goroutine sums one slice chunk into partial[p].
2. Main adds the partials for the total.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	data := []int{1, 2, 3, 4, 5, 6, 7, 8}
	const parts = 4
	chunk := len(data) / parts
	partial := make([]int, parts)

	var wg sync.WaitGroup
	for p := 0; p < parts; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sum := 0
			for _, v := range data[p*chunk : (p+1)*chunk] {
				sum += v
			}
			partial[p] = sum
		}()
	}
	wg.Wait()

	total := 0
	for _, s := range partial {
		total += s
	}
	fmt.Println("partial:", partial, "total:", total)
}
```

**Output:**

```
partial: [3 7 11 15] total: 36
```

---

## 20. Per-goroutine result structs, sorted

`🔴 hard` · *Collecting results*

Send a small result struct per goroutine over a channel, then sort by a stable key for deterministic reporting.

**Steps:**

1. Each goroutine sends a Result{ID, Value}.
2. Sort by ID after collecting.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

type Result struct {
	ID    int
	Value int
}

func main() {
	ch := make(chan Result, 5)
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- Result{ID: i, Value: i * 10}
		}()
	}
	wg.Wait()
	close(ch)

	var results []Result
	for r := range ch {
		results = append(results, r)
	}
	sort.Slice(results, func(a, b int) bool { return results[a].ID < results[b].ID })
	for _, r := range results {
		fmt.Printf("id=%d value=%d\n", r.ID, r.Value)
	}
}
```

**Output:**

```
id=1 value=10
id=2 value=20
id=3 value=30
id=4 value=40
id=5 value=50
```

---

## 21. Closing a channel broadcasts to all receivers

`🔴 hard` · *Lifecycle & leaks*

A close on a channel unblocks every goroutine receiving from it at once — a simple one-to-many start/stop signal.

**Steps:**

1. All goroutines block on <-start.
2. close(start) releases them simultaneously.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	start := make(chan struct{})
	done := make([]bool, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // all wait here
			done[i] = true
		}()
	}
	close(start) // broadcast: unblocks all receivers
	wg.Wait()
	fmt.Println(done) // [true true true]
}
```

**Output:**

```
[true true true]
```

---

## 22. Race-free shared counter (test with -race)

`🔴 hard` · *Shared state*

Two goroutines incrementing a shared counter must synchronize; atomics make it exactly correct. Without them it's a data race — run `go run -race .` to catch it.

**Steps:**

1. Each goroutine does 1000 atomic increments.
2. The total is always 2000. (Plain count++ here would race.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	// Without atomics/locks this would be a DATA RACE.
	// Detect races during development with: go run -race .
	var counter int64
	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				atomic.AddInt64(&counter, 1)
			}
		}()
	}
	wg.Wait()
	fmt.Println("counter:", counter) // 2000
}
```

**Output:**

```
counter: 2000
```

---

## 23. RWMutex: many readers, one writer

`🟡 medium` · *Shared state*

sync.RWMutex lets multiple readers hold the lock simultaneously (RLock), while a writer needs exclusive Lock.

**Steps:**

1. RLock allows many readers to proceed at once.
2. All readers observe the same value, each into its own slot.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var mu sync.RWMutex
	value := 1
	readers := make([]int, 4)
	var wg sync.WaitGroup
	for i := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.RLock()
			readers[i] = value
			mu.RUnlock()
		}()
	}
	wg.Wait()
	fmt.Println(readers) // [1 1 1 1]
}
```

**Output:**

```
[1 1 1 1]
```

---

## 24. sync.Map for concurrent access

`🟡 medium` · *Shared state*

sync.Map is a concurrency-safe map you can use without an external lock; iterate it with Range.

**Steps:**

1. Each goroutine calls m.Store with its own key.
2. Range collects keys; sort them for stable output.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	var m sync.Map
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Store(i, i*i)
		}()
	}
	wg.Wait()

	var keys []int
	m.Range(func(k, v any) bool {
		keys = append(keys, k.(int))
		return true
	})
	sort.Ints(keys)
	for _, k := range keys {
		v, _ := m.Load(k)
		fmt.Printf("%d->%d ", k, v)
	}
	fmt.Println()
}
```

**Output:**

```
0->0 1->1 2->4 3->9 4->16 
```

---

## 25. Nested goroutines

`🔴 hard` · *WaitGroup*

Goroutines can launch their own goroutines, each with its own WaitGroup, forming a tree of concurrent work.

**Steps:**

1. Each outer goroutine spawns inner goroutines with a private WaitGroup.
2. Inner results are summed into the outer slot.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const outer, inner = 3, 4
	sums := make([]int, outer)
	var wg sync.WaitGroup
	for i := 0; i < outer; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parts := make([]int, inner)
			var inWg sync.WaitGroup
			for j := 0; j < inner; j++ {
				inWg.Add(1)
				go func() {
					defer inWg.Done()
					parts[j] = i*10 + j
				}()
			}
			inWg.Wait()
			s := 0
			for _, p := range parts {
				s += p
			}
			sums[i] = s
		}()
	}
	wg.Wait()
	fmt.Println(sums) // [6 46 86]
}
```

**Output:**

```
[6 46 86]
```

---

## 26. atomic.Value for a shared snapshot

`🟡 medium` · *Shared state*

atomic.Value stores and loads an arbitrary value atomically — handy for a config or snapshot read by many goroutines.

**Steps:**

1. Store a snapshot once before launching readers.
2. Readers Load concurrently with no lock.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var cfg atomic.Value
	cfg.Store("v1")

	var wg sync.WaitGroup
	got := make([]string, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = cfg.Load().(string)
		}()
	}
	wg.Wait()
	fmt.Println(got) // [v1 v1 v1]
}
```

**Output:**

```
[v1 v1 v1]
```

---

## 27. Lock-free max with CompareAndSwap

`🔴 hard` · *Shared state*

A CAS retry loop updates shared state without a mutex: read the old value, attempt to swap, retry if someone beat you.

**Steps:**

1. Each goroutine bumps a shared max only if its value is larger.
2. CompareAndSwap retries until the update sticks.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var max int64
	vals := []int64{3, 9, 2, 7, 5}
	var wg sync.WaitGroup
	for _, v := range vals {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				old := atomic.LoadInt64(&max)
				if v <= old || atomic.CompareAndSwapInt64(&max, old, v) {
					break
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("max:", atomic.LoadInt64(&max)) // 9
}
```

**Output:**

```
max: 9
```

---

## 28. Build a map concurrently under a Mutex

`🟡 medium` · *Shared state*

A built-in map is not safe for concurrent writes; guard every write with a mutex (or use sync.Map).

**Steps:**

1. A shared map is written only while holding the mutex.
2. Sort the keys to print deterministically.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	var mu sync.Mutex
	counts := map[int]int{}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			counts[i%3]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Printf("%d:%d ", k, counts[k])
	}
	fmt.Println()
}
```

**Output:**

```
0:2 1:2 2:2 
```

---

## 29. Lazy singleton with sync.Once

`🟡 medium` · *sync primitives*

sync.Once guarantees one-time initialization, the basis of a thread-safe lazy singleton.

**Steps:**

1. once.Do creates the instance exactly once.
2. Every goroutine receives the same pointer.

```go
package main

import (
	"fmt"
	"sync"
)

type DB struct{ id int }

var (
	instance *DB
	once     sync.Once
)

func getDB() *DB {
	once.Do(func() { instance = &DB{id: 1} })
	return instance
}

func main() {
	first := getDB()
	same := make([]bool, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			same[i] = getDB() == first
		}()
	}
	wg.Wait()
	fmt.Println(same) // [true true true true]
}
```

**Output:**

```
[true true true true]
```

---

## 30. A two-stage pipeline

`🟡 medium` · *Pipelines*

Stages connected by channels each run a goroutine; with one goroutine per stage, element order is preserved.

**Steps:**

1. gen and square each run one goroutine and close their output.
2. A single goroutine per stage keeps the order.

```go
package main

import "fmt"

func gen(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			out <- n
		}
	}()
	return out
}

func square(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			out <- n * n
		}
	}()
	return out
}

func main() {
	for v := range square(gen(1, 2, 3, 4, 5)) {
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
1 4 9 16 25 
```

---

## 31. Producer / consumer with close

`🟡 medium` · *Pipelines*

Closing a channel signals 'no more values'; the consumer ranges until it is closed.

**Steps:**

1. The producer sends values then closes the channel.
2. The consumer ranges until the channel is closed.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	ch := make(chan int)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ch)
		for i := 1; i <= 5; i++ {
			ch <- i
		}
	}()

	sum := 0
	for v := range ch {
		sum += v
	}
	wg.Wait()
	fmt.Println("sum:", sum) // 15
}
```

**Output:**

```
sum: 15
```

---

## 32. wg.Go (Go 1.25+)

`🟡 medium` · *WaitGroup*

The wg.Go method launches a goroutine and tracks it for you, replacing the manual Add/defer Done pair.

**Steps:**

1. Go 1.25+: wg.Go runs a func as a goroutine, handling Add and Done.
2. Each goroutine writes its own index.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	results := make([]int, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			results[i] = i + 1
		})
	}
	wg.Wait()
	fmt.Println(results) // [1 2 3 4]
}
```

**Output:**

```
[1 2 3 4]
```

---

## 33. Fan-out then fan-in

`🔴 hard` · *Patterns*

Spread work across many goroutines (fan-out), then merge their results through one channel (fan-in), closing it once all senders finish.

**Steps:**

1. Fan-out: one goroutine per item sends a result.
2. A closer goroutine waits then closes; main drains and sorts.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	work := []int{1, 2, 3, 4, 5, 6}
	out := make(chan int, len(work))

	var wg sync.WaitGroup
	for _, n := range work {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out <- n * n
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}
	sort.Ints(got)
	fmt.Println(got) // [1 4 9 16 25 36]
}
```

**Output:**

```
[1 4 9 16 25 36]
```

---

## 34. Non-blocking receive with select/default

`🟡 medium` · *Channels & select*

A select with a default case never blocks: it runs default when no other case is ready.

**Steps:**

1. With nothing buffered, the default branch runs.
2. After a send, the receive branch succeeds. (select is covered fully in lesson 14.)

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 1)

	select {
	case v := <-ch:
		fmt.Println("got", v)
	default:
		fmt.Println("no value ready")
	}

	ch <- 42
	select {
	case v := <-ch:
		fmt.Println("got", v)
	default:
		fmt.Println("no value ready")
	}
}
```

**Output:**

```
no value ready
got 42
```

---

## 35. Return a value from a goroutine

`🟡 medium` · *Collecting results*

A goroutine can return a result by sending it on a (buffered) channel the caller receives from.

**Steps:**

1. The goroutine sends its result on a buffered channel.
2. The caller receives it with <-ch.

```go
package main

import "fmt"

func computeAsync(n int) <-chan int {
	result := make(chan int, 1) // buffered so the goroutine won't block
	go func() {
		result <- n * n
	}()
	return result
}

func main() {
	ch := computeAsync(7)
	fmt.Println("result:", <-ch) // 49
}
```

**Output:**

```
result: 49
```

---

## 36. defer LIFO inside a goroutine

`🟡 medium` · *Basics*

A goroutine's deferred calls run in last-in-first-out order when it returns, just like any function.

**Steps:**

1. Three defers run in reverse registration order (LIFO).
2. wg.Done is deferred first, so it runs last.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("cleanup 1")
		defer fmt.Println("cleanup 2")
		fmt.Println("working")
	}()
	wg.Wait()
	fmt.Println("main done")
}
```

**Output:**

```
working
cleanup 2
cleanup 1
main done
```

---

## 37. Worker pool with a results map

`🔴 hard` · *Worker pool*

Workers can store results in a shared map keyed by their input, guarded by a mutex.

**Steps:**

1. Each worker stores its result in a shared map under a mutex.
2. Sort the job keys for stable output.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	jobs := []int{2, 3, 4}
	results := map[int]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := j * j
			mu.Lock()
			results[j] = r
			mu.Unlock()
		}()
	}
	wg.Wait()

	keys := make([]int, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Printf("%d->%d ", k, results[k])
	}
	fmt.Println()
}
```

**Output:**

```
2->4 3->9 4->16 
```

---

## 38. Parallel 'any match' with an atomic flag

`🔴 hard` · *Patterns*

Search concurrently and set a shared atomic.Bool when any goroutine finds the target.

**Steps:**

1. Each goroutine sets an atomic flag if it finds the target.
2. found.Load reports whether any goroutine matched.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	data := []int{4, 8, 15, 16, 23, 42}
	target := 16
	var found atomic.Bool
	var wg sync.WaitGroup
	for _, v := range data {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v == target {
				found.Store(true)
			}
		}()
	}
	wg.Wait()
	fmt.Println("found 16?", found.Load()) // true
}
```

**Output:**

```
found 16? true
```

---

## 39. Count completed tasks atomically

`🟡 medium` · *Shared state*

An atomic.Int64 tallies how many goroutines finished, with no lock and no race.

**Steps:**

1. Each goroutine increments an atomic counter when done.
2. The final count equals the number of goroutines.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var completed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			completed.Add(1)
		}()
	}
	wg.Wait()
	fmt.Println("completed:", completed.Load()) // 10
}
```

**Output:**

```
completed: 10
```

---

## 40. A two-phase barrier

`🔴 hard` · *WaitGroup*

Waiting on one WaitGroup before starting the next batch acts as a barrier: every phase-1 goroutine finishes before any phase-2 goroutine starts.

**Steps:**

1. wg1.Wait is a barrier: all of phase 1 finishes first.
2. Phase 2 then safely reads what phase 1 wrote.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const n = 3
	phase1 := make([]bool, n)
	phase2 := make([]bool, n)

	var wg1 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			phase1[i] = true
		}()
	}
	wg1.Wait() // barrier

	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			phase2[i] = phase1[i]
		}()
	}
	wg2.Wait()
	fmt.Println(phase1, phase2)
}
```

**Output:**

```
[true true true] [true true true]
```

---

## 41. Cancel many workers by closing a channel

`🔴 hard` · *Lifecycle & leaks*

Each worker selects between doing work and a shared done channel; closing done cancels them all at once.

**Steps:**

1. Each worker selects between jobs and a done channel.
2. Closing done makes every worker take the cancel branch.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	done := make(chan struct{})
	jobs := make(chan int)
	stopped := make([]bool, 3)
	var wg sync.WaitGroup

	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-jobs:
					// would process a job
				case <-done:
					stopped[w] = true // own slot: no race
					return
				}
			}
		}()
	}

	close(done) // every worker takes the cancel branch
	wg.Wait()
	fmt.Println(stopped) // [true true true]
}
```

**Output:**

```
[true true true]
```

---

## 42. Share memory by communicating

`🔴 hard` · *Patterns*

Go's motto in action: instead of locking shared data, pass ownership of a value through channels.

**Steps:**

1. The worker owns each Job while computing, then sends it back.
2. Order is preserved because a single worker handles the stream.

```go
package main

import "fmt"

type Job struct {
	ID     int
	Result int
}

func main() {
	in := make(chan Job)
	out := make(chan Job)

	go func() {
		for j := range in {
			j.Result = j.ID * j.ID
			out <- j
		}
		close(out)
	}()

	go func() {
		for id := 1; id <= 3; id++ {
			in <- Job{ID: id}
		}
		close(in)
	}()

	for j := range out {
		fmt.Printf("job %d -> %d\n", j.ID, j.Result)
	}
}
```

**Output:**

```
job 1 -> 1
job 2 -> 4
job 3 -> 9
```

---

