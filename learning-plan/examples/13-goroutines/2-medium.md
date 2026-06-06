# Step 13 — Goroutines · 🟡 Medium

Examples **6–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

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

## 15. RWMutex: many readers, one writer

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

## 16. sync.Map for concurrent access

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

## 17. atomic.Value for a shared snapshot

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

## 18. Build a map concurrently under a Mutex

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

## 19. Lazy singleton with sync.Once

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

## 20. A two-stage pipeline

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

## 21. Producer / consumer with close

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

## 22. wg.Go (Go 1.25+)

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

## 23. Non-blocking receive with select/default

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

## 24. Return a value from a goroutine

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

## 25. defer LIFO inside a goroutine

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

## 26. Count completed tasks atomically

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

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
