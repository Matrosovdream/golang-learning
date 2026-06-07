# Step 14 — Channels · 🔴 Hard

Examples **29–40**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 29. Bounded parallelism with a semaphore

`🔴 hard` · *Semaphore*

Launch a goroutine per task but cap how many run at once with a semaphore channel. The `WaitGroup` waits for all; the buffered `results` channel collects every answer.

**Steps:**

1. `sem` (cap 3) bounds concurrency; acquire before work, release after.
2. `Wait`, close `results`, then drain and sort.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	tasks := []int{1, 2, 3, 4, 5, 6, 7, 8}
	const maxParallel = 3
	sem := make(chan struct{}, maxParallel) // bounds concurrency
	results := make(chan int, len(tasks))
	var wg sync.WaitGroup

	for _, t := range tasks {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release
			results <- n * n
		}(t)
	}
	wg.Wait()
	close(results)

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
[1 4 9 16 25 36 49 64]
```

---

## 30. Fan-out then fan-in

`🔴 hard` · *Fan-out/in*

Distribute one stream across N workers (fan-out), then merge their outputs back into one channel (fan-in). The shared input channel naturally load-balances work between workers.

**Steps:**

1. Three workers all `range` over the same `in` channel.
2. A closer goroutine closes `out` after the workers finish; collect and sort.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	in := make(chan int)
	go func() {
		defer close(in)
		for i := 1; i <= 9; i++ {
			in <- i
		}
	}()

	// Fan-out: 3 workers read from the same input channel.
	out := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range in {
				out <- n * n
			}
		}()
	}
	// Fan-in: close out once all workers have finished.
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
[1 4 9 16 25 36 49 64 81]
```

---

## 31. Pipeline with cancellation

`🔴 hard` · *Cancellation*

Real pipelines need a way to stop early without leaking goroutines. Pass a shared `done` channel into every stage; each stage `select`s its send against `<-done` so it can bail out.

**Steps:**

1. Both stages send via `select { case out <- v: case <-done: return }`.
2. `defer close(done)` in `main` stops the upstream goroutines once we take what we need.

```go
package main

import "fmt"

func gen(done <-chan struct{}, nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			select {
			case out <- n:
			case <-done: // stop early if the consumer is done
				return
			}
		}
	}()
	return out
}

func sq(done <-chan struct{}, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			select {
			case out <- n * n:
			case <-done:
				return
			}
		}
	}()
	return out
}

func main() {
	done := make(chan struct{})
	defer close(done) // closing on return unblocks/stops every stage

	nums := sq(done, gen(done, 2, 3, 4, 5))
	fmt.Println(<-nums) // take just the first two results...
	fmt.Println(<-nums) // ...then return; done stops the upstream goroutines
}
```

**Output:**

```
4
9
```

---

## 32. or-channel: combine done signals

`🔴 hard` · *Cancellation*

Sometimes "done" means "any of several signals fired." `or` returns one channel that closes as soon as the first input closes. `sync.Once` guarantees the close happens exactly once.

**Steps:**

1. One goroutine per input waits for its channel, then `once.Do(close)`.
2. The combined channel closes when the *fastest* input (20ms) does.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// or returns a channel that closes as soon as ANY input closes.
func or(channels ...<-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	var once sync.Once
	for _, c := range channels {
		go func(c <-chan struct{}) {
			<-c
			once.Do(func() { close(done) }) // close exactly once
		}(c)
	}
	return done
}

func after(d time.Duration) <-chan struct{} {
	c := make(chan struct{})
	go func() {
		time.Sleep(d)
		close(c)
	}()
	return c
}

func main() {
	start := time.Now()
	<-or(
		after(100*time.Millisecond),
		after(20*time.Millisecond), // fastest — fires the combined channel
		after(200*time.Millisecond),
	)
	fmt.Println("fired early:", time.Since(start) < 100*time.Millisecond)
}
```

**Output:**

```
fired early: true
```

---

## 33. Pub/sub broadcast to subscribers

`🔴 hard` · *Patterns*

To send the *same* value to many consumers, give each its own channel. (A plain channel delivers each value to only one receiver.) The publisher writes once per subscriber.

**Steps:**

1. Create one channel per subscriber.
2. The publisher sends `42` to each; subscribers receive concurrently and record it.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	const subs = 3
	channels := make([]chan int, subs)
	for i := range channels {
		channels[i] = make(chan int, 1)
	}

	// Publisher sends the same value to every subscriber.
	go func() {
		for _, c := range channels {
			c <- 42
		}
	}()

	var got []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < subs; i++ {
		wg.Add(1)
		go func(c <-chan int) {
			defer wg.Done()
			v := <-c
			mu.Lock()
			got = append(got, v)
			mu.Unlock()
		}(channels[i])
	}
	wg.Wait()
	sort.Ints(got)
	fmt.Println("subscribers received:", got)
}
```

**Output:**

```
subscribers received: [42 42 42]
```

---

## 34. Rate limiting with a ticker

`🔴 hard` · *Rate limit*

A ticker paces work to a steady rate: receive from `limiter.C` before serving each request, so you serve at most one per tick. Five requests at one per 10ms take ≥ ~40ms.

**Steps:**

1. Queue all requests, then close the channel.
2. `<-limiter.C` gates each iteration of the `range`.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	requests := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		requests <- i
	}
	close(requests)

	limiter := time.NewTicker(10 * time.Millisecond)
	defer limiter.Stop()

	start := time.Now()
	for req := range requests {
		<-limiter.C // serve at most one request per tick
		fmt.Println("served request", req)
	}
	fmt.Println("rate-limited:", time.Since(start) >= 40*time.Millisecond)
}
```

**Output:**

```
served request 1
served request 2
served request 3
served request 4
served request 5
rate-limited: true
```

---

## 35. Graceful shutdown: drain then stop

`🔴 hard` · *Cancellation*

On a quit signal, a good worker doesn't drop in-flight work — it drains whatever is still buffered, *then* reports and exits. A nested `select`/`default` empties the channel.

**Steps:**

1. The outer `select` handles new jobs or the `quit` signal.
2. On `quit`, the inner loop drains remaining jobs (until `default`), reports, and returns.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	jobs := make(chan int, 5)
	quit := make(chan struct{})
	done := make(chan []int)

	go func() {
		var processed []int
		for {
			select {
			case j := <-jobs:
				processed = append(processed, j)
			case <-quit:
				// drain whatever is still buffered, then report
				for {
					select {
					case j := <-jobs:
						processed = append(processed, j)
					default:
						sort.Ints(processed)
						done <- processed
						return
					}
				}
			}
		}
	}()

	for i := 1; i <= 5; i++ {
		jobs <- i // all 5 fit in the buffer before we quit
	}
	close(quit)
	fmt.Println("processed:", <-done)
}
```

**Output:**

```
processed: [1 2 3 4 5]
```

---

## 36. Three-stage composable pipeline

`🔴 hard` · *Pipeline*

Because every stage has the same shape (`<-chan int` in, `<-chan int` out), stages compose by nesting. Here: generate → keep evens → double, each running in its own goroutine.

**Steps:**

1. Each stage closes its output with `defer close(out)`.
2. Nest the calls: `double(filterEven(gen(...)))`.

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

func filterEven(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			if n%2 == 0 {
				out <- n
			}
		}
	}()
	return out
}

func double(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for n := range in {
			out <- n * 2
		}
	}()
	return out
}

func main() {
	// Three composable stages: generate → keep evens → double.
	for v := range double(filterEven(gen(1, 2, 3, 4, 5, 6))) {
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
4 8 12 
```

---

## 37. Disable a select case with a nil channel

`🔴 hard` · *select*

Merge two priority streams until both close. Each iteration rebinds a finished channel to `nil` — a `nil` channel is never ready, so `select` simply skips that case from then on.

**Steps:**

1. Local `hc`/`lc` mirror `high`/`low`, but become `nil` once their source closes.
2. The loop runs while either source is still open; we count everything received.

```go
package main

import "fmt"

func main() {
	high := make(chan int, 2)
	low := make(chan int, 2)
	high <- 1
	high <- 2
	low <- 10
	low <- 20
	close(high)
	close(low)

	var out []int
	highOpen, lowOpen := true, true
	for highOpen || lowOpen {
		// Rebind to nil to switch a finished case off — a nil
		// channel is never ready, so select ignores it.
		var hc, lc <-chan int = high, low
		if !highOpen {
			hc = nil
		}
		if !lowOpen {
			lc = nil
		}
		select {
		case v, ok := <-hc:
			if !ok {
				highOpen = false
				continue
			}
			out = append(out, v)
		case v, ok := <-lc:
			if !ok {
				lowOpen = false
				continue
			}
			out = append(out, v)
		}
	}
	fmt.Println("total received:", len(out))
}
```

**Output:**

```
total received: 4
```

---

## 38. Bounded take from an infinite generator

`🔴 hard` · *Cancellation*

A generator can loop forever, yielding numbers on demand. The consumer takes only what it needs, then closes `done` so the generator's `select` exits instead of leaking.

**Steps:**

1. `counter` sends `i` in a `select` against `<-done`.
2. After taking five values, `close(done)` stops the generator; `break` ends the loop.

```go
package main

import "fmt"

// counter is an "infinite" generator that stops when done is closed.
func counter(done <-chan struct{}) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 0; ; i++ {
			select {
			case out <- i:
			case <-done:
				return
			}
		}
	}()
	return out
}

func main() {
	done := make(chan struct{})
	nums := counter(done)

	var got []int
	for n := range nums {
		got = append(got, n)
		if n == 4 {
			close(done) // tell the generator to stop; no goroutine leak
			break
		}
	}
	fmt.Println(got)
}
```

**Output:**

```
[0 1 2 3 4]
```

---

## 39. Worker pool with ordered results

`🔴 hard` · *Worker pool*

Workers finish out of order, but you often need results in input order. Tag each job with its index, then place each result at `ordered[idx]` — no sorting needed.

**Steps:**

1. Jobs carry `{idx, val}`; workers compute and return the same `idx`.
2. The collector writes `ordered[r.idx] = r.val`, restoring input order.

```go
package main

import (
	"fmt"
	"sync"
)

type job struct {
	idx, val int
}

func main() {
	inputs := []int{5, 6, 7, 8}
	jobs := make(chan job)
	results := make(chan job, len(inputs))
	var wg sync.WaitGroup

	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- job{idx: j.idx, val: j.val * j.val}
			}
		}()
	}

	go func() {
		for i, n := range inputs {
			jobs <- job{idx: i, val: n}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]int, len(inputs))
	for r := range results {
		ordered[r.idx] = r.val // place each result at its original index
	}
	fmt.Println(ordered)
}
```

**Output:**

```
[25 36 49 64]
```

---

## 40. Capstone: bounded fetch with timeout & ordered map

`🔴 hard` · *Capstone*

Everything together: launch concurrent fetches, collect into a map keyed by id, and guard the whole gather with a single deadline. A labeled `break` exits the `for` (not just the `select`) if the deadline ever fires.

**Steps:**

1. Each `fetch` sends a `{id, val}` onto a buffered channel.
2. Collect until the map is full *or* `deadline` fires (`break collect`); print sorted by id.

```go
package main

import (
	"fmt"
	"sort"
	"time"
)

type res struct {
	id, val int
}

func fetch(id int, out chan<- res) {
	time.Sleep(time.Duration(id) * time.Millisecond) // simulate work
	out <- res{id: id, val: id * 10}
}

func main() {
	ids := []int{1, 2, 3, 4, 5}
	out := make(chan res, len(ids))
	for _, id := range ids {
		go fetch(id, out)
	}

	got := make(map[int]int)
	deadline := time.After(500 * time.Millisecond)
collect:
	for len(got) < len(ids) {
		select {
		case r := <-out:
			got[r.id] = r.val
		case <-deadline:
			fmt.Println("deadline hit before all results arrived")
			break collect // labeled break exits the for, not just select
		}
	}

	keys := make([]int, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Printf("id %d -> %d\n", k, got[k])
	}
}
```

**Output:**

```
id 1 -> 10
id 2 -> 20
id 3 -> 30
id 4 -> 40
id 5 -> 50
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
