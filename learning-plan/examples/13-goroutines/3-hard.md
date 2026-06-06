# Step 13 — Goroutines · 🔴 Hard

Examples **27–42**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 27. Worker pool

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

## 28. Give a goroutine a guaranteed exit (avoid leaks)

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

## 29. Bounded concurrency with a semaphore channel

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

## 30. Collect errors from goroutines

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

## 31. Parallel partial sums

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

## 32. Per-goroutine result structs, sorted

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

## 33. Closing a channel broadcasts to all receivers

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

## 34. Race-free shared counter (test with -race)

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

## 35. Nested goroutines

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

## 36. Lock-free max with CompareAndSwap

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

## 37. Fan-out then fan-in

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

## 38. Worker pool with a results map

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

## 39. Parallel 'any match' with an atomic flag

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
