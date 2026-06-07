# Step 14 — Channels · 🟡 Medium

Examples **11–28**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 11. select waits on whichever is ready

`🟡 medium` · *select*

`select` blocks until one of its cases can proceed, then runs that one. Here the worker's value is ready well before the one-second timeout, so the value case wins.

**Steps:**

1. List several channel operations as `case`s.
2. The ready one runs; if several are ready, one is chosen at random.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	ch := make(chan string)
	go func() {
		ch <- "result"
	}()
	select { // wait on several operations; the ready one runs
	case v := <-ch:
		fmt.Println("got:", v)
	case <-time.After(time.Second):
		fmt.Println("timeout")
	}
}
```

**Output:**

```
got: result
```

---

## 12. Timeout with select and time.After

`🟡 medium` · *select*

`time.After(d)` returns a channel that fires after `d`. Put it in a `select` to bound how long you'll wait for a slow operation.

**Steps:**

1. The worker sleeps 50ms before sending.
2. The 10ms timeout fires first, so the timeout branch runs.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	ch := make(chan string)
	go func() {
		time.Sleep(50 * time.Millisecond) // slow worker
		ch <- "late"
	}()
	select {
	case v := <-ch:
		fmt.Println("got:", v)
	case <-time.After(10 * time.Millisecond): // fires first
		fmt.Println("timed out waiting")
	}
}
```

**Output:**

```
timed out waiting
```

---

## 13. Non-blocking receive with default

`🟡 medium` · *select*

A `select` with a `default` case never blocks: if no channel is ready, `default` runs immediately. This is the idiom for "check, but don't wait."

**Steps:**

1. Nothing is ever sent on `ch`.
2. Because no case is ready, `default` runs at once.

```go
package main

import "fmt"

func main() {
	ch := make(chan int) // nothing will ever be sent
	select {
	case v := <-ch:
		fmt.Println("received", v)
	default: // runs immediately when no case is ready
		fmt.Println("nothing ready")
	}
}
```

**Output:**

```
nothing ready
```

---

## 14. Non-blocking send with default

`🟡 medium` · *select*

The same trick works for sends: if the buffer is full, the send case isn't ready, so `default` lets you drop or handle the value instead of blocking.

**Steps:**

1. Fill the 1-slot buffer so a further send would block.
2. `select` falls through to `default` and drops the value.

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 1)
	ch <- 1 // fill the buffer
	select {
	case ch <- 2: // would block (full), so it is not chosen
		fmt.Println("sent 2")
	default:
		fmt.Println("buffer full, dropped 2")
	}
	fmt.Println("buffered:", <-ch)
}
```

**Output:**

```
buffer full, dropped 2
buffered: 1
```

---

## 15. Closing broadcasts to all receivers

`🟡 medium` · *Signaling*

A single `close` wakes *every* goroutine blocked receiving on that channel — the canonical fan-out "stop" signal. Each worker reports back so we can confirm all three woke.

**Steps:**

1. Three workers block on `<-stop`.
2. One `close(stop)` unblocks them all; collect and sort their IDs for stable output.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	stop := make(chan struct{})
	results := make(chan int, 3)
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-stop // every receiver unblocks when stop is closed
			results <- id
		}(i)
	}
	close(stop) // a single close wakes all three goroutines
	wg.Wait()
	close(results)

	var got []int
	for id := range results {
		got = append(got, id)
	}
	sort.Ints(got)
	fmt.Println("stopped:", got)
}
```

**Output:**

```
stopped: [1 2 3]
```

---

## 16. Fan-in: merge many channels

`🟡 medium` · *Fan-in*

Fan-in combines several input channels into one. A goroutine per input forwards values to a shared `out`, and a closer goroutine closes `out` after all inputs drain.

**Steps:**

1. `merge` launches one forwarder per input channel, tracked by a `WaitGroup`.
2. When all forwarders finish, `out` is closed so the caller's `range` ends.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

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

// merge fans in: it reads from every input channel into one output.
func merge(cs ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func(c <-chan int) {
			defer wg.Done()
			for v := range c {
				out <- v
			}
		}(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func main() {
	var got []int
	for v := range merge(gen(1, 2, 3), gen(4, 5, 6)) {
		got = append(got, v)
	}
	sort.Ints(got)
	fmt.Println(got)
}
```

**Output:**

```
[1 2 3 4 5 6]
```

---

## 17. A two-stage pipeline

`🟡 medium` · *Pipeline*

A pipeline chains stages, each a function that takes an input channel and returns an output channel. Values flow `gen → square` with each stage running concurrently.

**Steps:**

1. `gen` emits numbers and closes; `square` reads them, squares, and closes.
2. The `range` in `main` pulls the pipeline along.

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
	// Each stage reads from the previous one's channel.
	for v := range square(gen(2, 3, 4)) {
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
4 9 16 
```

---

## 18. Worker pool with jobs and results

`🟡 medium` · *Worker pool*

A fixed set of workers pull from a `jobs` channel and push to a `results` channel. Closing `jobs` ends the workers; a closer goroutine then closes `results`.

**Steps:**

1. Three workers `range` over `jobs` concurrently.
2. Close `jobs`, wait, close `results`; collect and sort.

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

	for w := 0; w < 3; w++ { // 3 workers share the jobs channel
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- j * 2
			}
		}()
	}

	for i := 1; i <= 5; i++ {
		jobs <- i
	}
	close(jobs) // stops the workers' range loops

	go func() {
		wg.Wait()
		close(results)
	}()

	var got []int
	for r := range results {
		got = append(got, r)
	}
	sort.Ints(got)
	fmt.Println(got)
}
```

**Output:**

```
[2 4 6 8 10]
```

---

## 19. Buffered channel as a semaphore

`🟡 medium` · *Semaphore*

A buffered channel of capacity N is a counting semaphore: sending acquires a slot (blocking when full), receiving releases one. This caps how many goroutines run a section at once.

**Steps:**

1. `sem` has capacity 2, so at most 2 goroutines hold a slot.
2. `sem <- struct{}{}` acquires; `defer func(){ <-sem }()` releases.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const limit = 2
	sem := make(chan struct{}, limit) // at most `limit` holders at once
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}        // acquire a slot (blocks if full)
			defer func() { <-sem }() // release on return
			mu.Lock()
			done++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	fmt.Println("tasks completed:", done)
}
```

**Output:**

```
tasks completed: 5
```

---

## 20. Quit channel in a select loop

`🟡 medium` · *Signaling*

A long-running loop selects between doing work and a `quit` signal. Closing `quit` makes that case ready, so the loop returns. This is the precursor to `context` cancellation.

**Steps:**

1. The producer sends three values, then `close(quit)`.
2. `main`'s loop sums work until the `quit` case fires.

```go
package main

import "fmt"

func main() {
	work := make(chan int)
	quit := make(chan struct{})

	go func() {
		for i := 0; i < 3; i++ {
			work <- i
		}
		close(quit) // signal "no more work"
	}()

	sum := 0
	for {
		select {
		case v := <-work:
			sum += v
		case <-quit:
			fmt.Println("sum:", sum)
			return
		}
	}
}
```

**Output:**

```
sum: 3
```

---

## 21. Return a result via a channel (future)

`🟡 medium` · *Patterns*

A function can kick off async work and hand back a channel to read the result later — a "future." A buffered channel of size 1 lets the goroutine finish even if no one is reading yet.

**Steps:**

1. `future` starts the computation and returns the result channel immediately.
2. `main` does other work, then receives the result when it needs it.

```go
package main

import "fmt"

// future starts the work now and hands back a channel to read later.
func future(x int) <-chan int {
	result := make(chan int, 1) // buffered so the goroutine never blocks
	go func() {
		result <- x * x
	}()
	return result
}

func main() {
	f := future(9) // computation begins immediately
	fmt.Println("doing other work...")
	fmt.Println("result:", <-f) // collect when we actually need it
}
```

**Output:**

```
doing other work...
result: 81
```

---

## 22. Collect a fixed number of results

`🟡 medium` · *Patterns*

When you know exactly how many results to expect, receive that many times instead of closing and ranging. Order of completion varies, so sort for stable output.

**Steps:**

1. Launch `n` goroutines, each sending one result.
2. Receive exactly `n` times, then stop.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	const n = 4
	results := make(chan int)
	for i := 1; i <= n; i++ {
		go func(x int) {
			results <- x * 10
		}(i)
	}
	got := make([]int, 0, n)
	for i := 0; i < n; i++ { // receive exactly n times, then stop
		got = append(got, <-results)
	}
	sort.Ints(got)
	fmt.Println(got)
}
```

**Output:**

```
[10 20 30 40]
```

---

## 23. Drain two producers (nil disables a case)

`🟡 medium` · *select*

To consume from multiple channels until all are closed, set a finished channel to `nil`. A `nil` channel is never ready, so its `select` case is effectively switched off.

**Steps:**

1. Each `case` uses comma-ok; on close, set that channel to `nil`.
2. The loop ends when both channels are `nil`.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	a := make(chan int)
	b := make(chan int)
	go func() { a <- 1; a <- 2; close(a) }()
	go func() { b <- 10; b <- 20; close(b) }()

	var got []int
	for a != nil || b != nil { // loop until both are drained
		select {
		case v, ok := <-a:
			if !ok {
				a = nil // a nil channel disables this case forever
				continue
			}
			got = append(got, v)
		case v, ok := <-b:
			if !ok {
				b = nil
				continue
			}
			got = append(got, v)
		}
	}
	sort.Ints(got)
	fmt.Println(got)
}
```

**Output:**

```
[1 2 10 20]
```

---

## 24. Bound the whole wait with a timeout

`🟡 medium` · *select*

Create the timeout channel *once* with `time.After` and select against it, so the entire operation has a single deadline regardless of how many receives you do.

**Steps:**

1. The worker replies after 20ms.
2. The 100ms deadline is generous, so the result arrives first.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	results := make(chan int)
	go func() {
		time.Sleep(20 * time.Millisecond)
		results <- 1
	}()

	timeout := time.After(100 * time.Millisecond) // bound the whole wait
	select {
	case r := <-results:
		fmt.Println("result:", r)
	case <-timeout:
		fmt.Println("operation timed out")
	}
}
```

**Output:**

```
result: 1
```

---

## 25. Do work on a ticker interval

`🟡 medium` · *Ticker*

`time.NewTicker` delivers a value on its `C` channel at a fixed interval. Always `Stop` it (here with `defer`) so its background goroutine is released.

**Steps:**

1. Create a 10ms ticker; `defer ticker.Stop()`.
2. Receive from `ticker.C` three times, counting ticks.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop() // always stop a ticker to release its resources

	ticks := 0
	for ticks < 3 {
		<-ticker.C // fires every 10ms
		ticks++
		fmt.Println("tick", ticks)
	}
}
```

**Output:**

```
tick 1
tick 2
tick 3
```

---

## 26. Range over a multi-producer channel

`🟡 medium` · *Fan-in*

When many goroutines write to one channel, you can't have each close it (closing twice panics). Instead, a dedicated goroutine waits for all producers, then closes once.

**Steps:**

1. Each producer sends one value; a `WaitGroup` tracks them.
2. A closer goroutine `Wait`s, then `close(out)` so `range` ends.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	out := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			out <- base
		}(i * 100)
	}
	// A separate goroutine closes out once every producer is done,
	// so the range below terminates.
	go func() {
		wg.Wait()
		close(out)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}
	sort.Ints(got)
	fmt.Println(got)
}
```

**Output:**

```
[0 100 200]
```

---

## 27. First response wins

`🟡 medium` · *Patterns*

Query several replicas and take the fastest answer. A buffered result channel (size 1) plus a `select`/`default` send means the slower replicas don't block forever when they finish.

**Steps:**

1. Each replica sleeps a different amount, then tries to send.
2. The first to finish fills the 1-slot buffer; the rest hit `default` and move on.

```go
package main

import (
	"fmt"
	"time"
)

func replica(id int, delay time.Duration, out chan<- int) {
	time.Sleep(delay)
	select {
	case out <- id:
	default: // someone already answered — don't block forever
	}
}

func main() {
	out := make(chan int, 1) // buffered so the winner never blocks
	go replica(1, 30*time.Millisecond, out)
	go replica(2, 10*time.Millisecond, out) // fastest
	go replica(3, 50*time.Millisecond, out)
	fmt.Println("fastest replica:", <-out)
}
```

**Output:**

```
fastest replica: 2
```

---

## 28. Drain a buffer with select/default

`🟡 medium` · *select*

A `select` loop with `default` reads everything currently buffered, then exits the moment the channel is empty — without blocking for more.

**Steps:**

1. Pre-fill the buffer with three values.
2. Receive in a loop; `default` fires when nothing is left and returns.

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3

	for {
		select {
		case v := <-ch:
			fmt.Println("drained", v)
		default: // nothing left to receive
			fmt.Println("channel empty")
			return
		}
	}
}
```

**Output:**

```
drained 1
drained 2
drained 3
channel empty
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
