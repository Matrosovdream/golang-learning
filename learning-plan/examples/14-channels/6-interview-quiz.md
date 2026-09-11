# Step 14 — Channels · 🎯 Interview: predict the output & find the bug

Examples **78–93**: the "what does this print?" and "what's wrong here?" questions interviewers love, because each takes 30 seconds to show and exposes exactly how well you understand blocking, closing and leaks.

**How to use this part:** read the code, **commit to an answer out loud** (the exact output, or "deadlock", "panic", "leak"), and only then open **Answer**. It shows the real output, why it happens, and the fix. Guessing first is the whole exercise. Several programs deadlock or panic on purpose; their output is the real runtime message, trimmed with `...` where the goroutine dump gets long.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev: [🧩 more patterns](5-patterns.md) · Next part: [🛠️ interview tasks](7-interview-tasks.md)

---

## 78. A single send in main with no receiver

`🟢 easy` · *Predict the output*

Four lines. What does it print?

```go
package main

import "fmt"

func main() {
	ch := make(chan string)
	ch <- "hello"
	fmt.Println(<-ch)
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan send]:
main.main()
	/tmp/go-ex/main.go:7
exit status 2
```

Nothing is printed. `ch` is unbuffered, so `ch <- "hello"` waits until another goroutine is ready to receive. The only receive is in the **same** goroutine, one line later, and that goroutine is stuck on the line before it. It's the "one worker can't hand a box to himself" deadlock from the lesson's mental model. With no other goroutines, the runtime detects it and exits with status 2.

**Fixes:** give the value somewhere to wait, with `make(chan string, 1)`, or send from another goroutine: `go func() { ch <- "hello" }()`.

</details>

---

## 79. range over a channel nobody closes

`🟢 easy` · *Predict the output*

The producer sends three numbers. Does `done` get printed?

```go
package main

import "fmt"

func main() {
	ch := make(chan int)
	go func() {
		for i := 1; i <= 3; i++ {
			ch <- i
		}
	}()

	for v := range ch {
		fmt.Println(v)
	}
	fmt.Println("done")
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
1
2
3
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan receive]:
main.main()
	/tmp/go-ex/main.go:13
exit status 2
```

All three values arrive, then the program dies. `range` stops **only** when the channel is closed. After the third value the producer goroutine returns, and `main` waits for a fourth value that can never come. With no other goroutine left, that's a deadlock, and `done` never prints. In a server (with other goroutines alive), the same bug would be a silent leak instead (example 55).

**Fix:** the sender closes when it's finished. Put `defer close(ch)` as the first line of the goroutine.

</details>

---

## 80. Close, then range over a buffered channel

`🟢 easy` · *Predict the output*

Three values are buffered and then the channel is closed. What do the three `Println`s show? Common wrong answers: "range gets nothing, it's closed" and "panic".

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 5)
	for i := 1; i <= 3; i++ {
		ch <- i
	}
	close(ch)
	fmt.Println("len after close:", len(ch))

	var got []int
	for v := range ch {
		got = append(got, v)
	}
	fmt.Println("range got:", got)

	v, ok := <-ch
	fmt.Println("one more receive:", v, ok)
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
len after close: 3
range got: [1 2 3]
one more receive: 0 false
```

`close` doesn't throw away queued values. It only means "no **more** values". The buffer still holds 3 (so `len` is 3), `range` delivers all of them in order, and **only then** does it see the channel closed and stop. Any receive after that returns immediately with the zero value and `ok == false`. Only sending panics after a close; receiving never does.

</details>

---

## 81. wg.Wait() before reading an unbuffered results channel

`🟡 medium` · *Find the bug*

A textbook-looking "start workers, wait, close, collect". It's probably the most common channel bug in real interviews. What happens?

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	results := make(chan int)
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- i * i
		}()
	}

	wg.Wait()
	close(results)

	for r := range results {
		fmt.Println(r)
	}
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [sync.WaitGroup.Wait]:
sync.runtime_SemacquireWaitGroup(...)
	$GOROOT/src/runtime/sema.go:114
...
exit status 2
```

Each worker blocks on `results <- i * i`, because nobody is receiving yet, so none of them reaches `wg.Done()`. `main` blocks in `wg.Wait()` (the `sync.WaitGroup.Wait` state in the dump), waiting for those `Done`s. Everyone waits on someone else: deadlock. The ordering "wait, then read" only works when the channel has room for every result.

**Fixes** (either one):

```go
// 1) read while waiting: close from a helper goroutine
go func() {
	wg.Wait()
	close(results)
}()
for r := range results { fmt.Println(r) }

// 2) make room for every result, then wait, close and read
results := make(chan int, 3)
```

Option 1 is the general pattern (example 26), because it works however many results there are.

</details>

---

## 82. Workers range over jobs, but nobody closes jobs

`🟡 medium` · *Find the bug*

Two workers, four jobs. What gets printed, and does the program end?

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	jobs := make(chan int)
	var processed atomic.Int32
	var wg sync.WaitGroup

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				processed.Add(1)
			}
		}()
	}

	for j := 1; j <= 4; j++ {
		jobs <- j
	}
	fmt.Println("sent all 4 jobs")

	wg.Wait()
	fmt.Println("processed:", processed.Load())
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
sent all 4 jobs
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [sync.WaitGroup.Wait]:
sync.runtime_SemacquireWaitGroup(...)
	$GOROOT/src/runtime/sema.go:114
...
exit status 2
```

All four jobs are delivered, so `sent all 4 jobs` prints. But each worker's `for range jobs` only ends when `jobs` is **closed**. Both workers park waiting for a fifth job, `main` parks in `wg.Wait()`, and nothing is left to run. `processed:` never prints.

**Fix:** `close(jobs)` right after the send loop. The sender (`main`) is the only goroutine that knows no more jobs are coming. This is the other half of example 81: 81 forgot to **drain** before waiting; this one forgot to **close** before waiting.

</details>

---

## 83. Every worker runs defer close(results)

`🟡 medium` · *Find the bug*

"Each worker closes the channel when it's done sending." It sounds responsible. The buffer is big enough, and `wg.Wait()` comes before the read. What happens?

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	results := make(chan int, 10)
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(results)                                 // "I'm done sending"
			time.Sleep(time.Duration(i) * 10 * time.Millisecond) // simulate work
			results <- i * 10
		}()
	}

	wg.Wait()
	for r := range results {
		fmt.Println(r)
	}
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
panic: send on closed channel
	panic: close of closed channel

goroutine 20 [running]:
main.main.func1()
	/tmp/go-ex/main.go:19
...
exit status 2
```

Worker 1 finishes first and closes `results`. Worker 2's send then hits a closed channel: `panic: send on closed channel`. While that goroutine unwinds, its own deferred `close(results)` runs and panics **again** (`close of closed channel`). That's the second, indented `panic:` line. A panic in any goroutine kills the whole program, so nothing is printed.

`close` means "no more values will **ever** be sent on this channel". With three senders, no single one of them can promise that.

**Fix:** exactly one closer, after all senders are finished:

```go
go func() {
	wg.Wait()
	close(results)
}()
for r := range results { fmt.Println(r) }
```

</details>

---

## 84. The receiver closes the channel: who panics?

`🟡 medium` · *Predict the output*

The consumer wants three values and then "tells the producer to stop" by closing the channel. Which goroutine panics, and does `main finished` print?

```go
package main

import (
	"fmt"
	"time"
)

func producer(out chan<- int) {
	for i := 1; ; i++ {
		out <- i
		time.Sleep(5 * time.Millisecond)
	}
}

func main() {
	ch := make(chan int)
	go producer(ch)

	for v := range ch {
		fmt.Println("got", v)
		if v == 3 {
			close(ch) // "I've had enough, stop sending"
			break
		}
	}

	time.Sleep(50 * time.Millisecond) // main carries on with other work
	fmt.Println("main finished")
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
got 1
got 2
got 3
panic: send on closed channel

goroutine 19 [running]:
main.producer(...)
	/tmp/go-ex/main.go:10
...
exit status 2
```

The **producer** panics, not the receiver who called `close`. Its next send after the close is `send on closed channel`. A panic in **any** goroutine crashes the whole process, so `main` never gets to print `main finished`, even though `main` itself did nothing "wrong" at the moment of the crash. That's why receiver-side closing is so dangerous: the code that fails is far from the code that caused it.

Also notice `producer` takes `chan<- int`. If `main` had received through a `<-chan int` variable, `close` wouldn't even compile (example 45).

**Fix:** the receiver closes a separate `stop` channel, and the producer selects on it:

```go
func producer(out chan<- int, stop <-chan struct{}) {
	for i := 1; ; i++ {
		select {
		case out <- i:
		case <-stop:
			return
		}
	}
}
```

</details>

---

## 85. Two goroutines, two channels, each waiting on the other

`🟡 medium` · *Find the bug*

Each side politely sends first and receives second. Which line prints first?

```go
package main

import "fmt"

func main() {
	pings := make(chan string)
	pongs := make(chan string)

	go func() {
		pongs <- "pong" // hand over my pong first...
		msg := <-pings  // ...then read main's ping
		fmt.Println("goroutine got", msg)
	}()

	pings <- "ping" // hand over my ping first...
	msg := <-pongs  // ...then read the goroutine's pong
	fmt.Println("main got", msg)
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan send]:
main.main()
	/tmp/go-ex/main.go:15
...
exit status 2
```

Neither line prints. `main` is parked on `pings <- "ping"`, waiting for the goroutine to receive. The goroutine is parked on `pongs <- "pong"`, waiting for `main` to receive. Each holds up the other: a **circular wait**, the same shape as two goroutines taking two mutexes in opposite order. The trimmed dump shows a second goroutine in `chan send` too.

**Fixes** (any one):

- Break the symmetry: one side receives first.
- Buffer one of the channels (`make(chan string, 1)`) so one send can finish without a receiver.
- When one goroutine really must send and receive "whichever is possible first", use `select` with both operations as cases:

```go
select {
case pings <- "ping":
case msg := <-pongs:
	fmt.Println("main got", msg)
}
```

</details>

---

## 86. Unbuffered handoff: which print orders are guaranteed?

`🟡 medium` · *Predict the output*

An unbuffered channel "synchronizes" the two goroutines. So in what order do lines **A**, **B** and **C** appear? Which orderings are **guaranteed**, and which aren't?

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
		fmt.Println("A: about to send")
		ch <- 1
		fmt.Println("C: send returned")
	}()

	<-ch
	fmt.Println("B: received")
	wg.Wait()
}
```

<details>
<summary><b>Answer</b></summary>

**Output** (one real run; B and C may appear in the other order on your machine):

```
A: about to send
C: send returned
B: received
```

Guaranteed:

- **A before B.** `main` can't receive until the send happens, and A prints before the send.
- **A before C.** They're sequential in the same goroutine.

**Not** guaranteed: **B vs C**. The handoff guarantees that both goroutines passed the send/receive **point**. After that they're two independent goroutines, and the scheduler may run either one first, or both at once on different cores. "Synchronized" means they **met** at the channel, not that they take turns afterwards.

The interview lesson: an unbuffered channel tells you *the receiver has the value*. If you also need "the receiver has finished handling it", you need a second signal: a reply (example 60), a `done` channel, or a `WaitGroup`.

</details>

---

## 87. The non-blocking send that always drops its message

`🟡 medium` · *Find the bug*

The author wanted logging that never slows the main path, so every event goes through a `select` with `default`. A logger goroutine is running. How many events get logged?

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	events := make(chan string) // unbuffered

	go func() { // the logger
		time.Sleep(10 * time.Millisecond) // startup: open the log file...
		for e := range events {
			fmt.Println("logged:", e)
		}
	}()

	for _, e := range []string{"start", "step", "stop"} {
		select { // "never block the hot path on logging"
		case events <- e:
		default:
			fmt.Println("dropped:", e)
		}
	}
	time.Sleep(30 * time.Millisecond)
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
dropped: start
dropped: step
dropped: stop
```

None. A non-blocking send on an **unbuffered** channel succeeds only if a receiver is **already parked** on that channel at that exact instant. The logger is still starting up, so all three events are dropped. Even with an instant startup, the logger would be busy printing the previous event while the next one arrives, so most events would still be dropped. `select`/`default` on an unbuffered channel means "send only if someone is standing at the hole right now".

**Fix:** give the channel a buffer sized for real bursts, `make(chan string, 100)`, so `default` only triggers when the logger is **genuinely** 100 events behind. Consider counting the drops too, so you can see them in metrics. The same trap explains why `signal.Notify` needs a buffered channel (example 56).

</details>

---

## 88. The timeout that leaks its worker goroutine

`🟡 medium` · *Find the bug*

A textbook "call with a timeout". Twenty calls all time out, and `main` then waits long enough for every `slowQuery` to finish. How many goroutines are alive at the end?

```go
package main

import (
	"errors"
	"fmt"
	"runtime"
	"time"
)

func slowQuery() int {
	time.Sleep(50 * time.Millisecond)
	return 42
}

func queryWithTimeout(d time.Duration) (int, error) {
	result := make(chan int)
	go func() {
		result <- slowQuery()
	}()

	select {
	case v := <-result:
		return v, nil
	case <-time.After(d):
		return 0, errors.New("timeout")
	}
}

func main() {
	timeouts := 0
	for range 20 {
		if _, err := queryWithTimeout(10 * time.Millisecond); err != nil {
			timeouts++
		}
	}
	time.Sleep(100 * time.Millisecond) // every slowQuery has finished by now
	fmt.Println("timeouts:", timeouts)
	fmt.Println("goroutines alive:", runtime.NumGoroutine())
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
timeouts: 20
goroutines alive: 21
```

`main` plus **20 leaked goroutines**. When the timeout wins, `queryWithTimeout` returns and nobody will ever receive from `result`. Each worker finishes `slowQuery` 40ms later and parks forever on `result <- ...`, because the channel is unbuffered. No error, no deadlock report (`main` is alive), just goroutines and memory that are never freed, once per timed-out request. In a busy server this is a slow memory leak that only shows up in production.

**Fix:** one character. `result := make(chan int, 1)`. The buffer lets the late worker drop its value and exit; the channel and value are garbage collected afterwards. If `slowQuery` is expensive, also give it a way to stop early (a `done` channel, or `context` in lesson 15), so the abandoned work doesn't keep running.

</details>

---

## 89. time.After inside a for-select: the timeout never fires

`🟡 medium` · *Predict the output*

The intent: "handle events, but stop waiting after 50ms". The producer sends 15 events, 10ms apart, about 150ms in total. What prints?

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	events := make(chan int)
	go func() { // 15 events, 10ms apart (~150ms in total)
		for i := range 15 {
			events <- i
			time.Sleep(10 * time.Millisecond)
		}
	}()

	count := 0
	for count < 15 {
		select {
		case <-events:
			count++
		case <-time.After(50 * time.Millisecond): // "stop waiting after 50ms"
			fmt.Println("timed out after", count, "events")
			return
		}
	}
	fmt.Println("never timed out: handled all", count, "events")
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
never timed out: handled all 15 events
```

`time.After(50ms)` is evaluated **every time the loop enters the `select`** (example 49), so each pass gets a brand-new 50ms timer. Events arrive every 10ms, so no single timer ever gets to fire. The code implements an **idle timeout** ("give up if no event for 50ms"), not the intended **total deadline**. Under steady traffic it never times out.

**Fix** (`fix/main.go`): create the timer **once**, before the loop.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	events := make(chan int)
	go func() {
		for i := range 15 {
			events <- i
			time.Sleep(10 * time.Millisecond)
		}
	}()

	deadline := time.After(50 * time.Millisecond) // one timer for the whole loop
	count := 0
	for count < 15 {
		select {
		case <-events:
			count++
		case <-deadline:
			fmt.Println("timed out, events handled before the deadline < 15:", count < 15)
			return
		}
	}
	fmt.Println("handled all", count, "events")
}
```

**Output:**

```
timed out, events handled before the deadline < 15: true
```

Both behaviors are legitimate. An idle timeout is right for "the client went quiet", and a deadline is right for "this request has 50ms". The bug is getting one when you meant the other. Before Go 1.23 the per-iteration version also held every unexpired timer in memory until it fired; since 1.23 unreferenced timers are garbage collected, but the logic bug remains.

</details>

---

## 90. recover() catches a closed-channel panic but not a deadlock

`🟡 medium` · *Predict the output*

`safely` wraps each function in `defer`/`recover`. Three channel mistakes go through it. Which ones does it survive, and does `end of main` print?

```go
package main

import "fmt"

func safely(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "-> recovered:", r)
		}
	}()
	f()
	fmt.Println(name, "-> ok")
}

func main() {
	safely("close twice", func() {
		ch := make(chan int)
		close(ch)
		close(ch)
	})
	safely("send after close", func() {
		ch := make(chan int, 1)
		close(ch)
		ch <- 1
	})
	safely("receive, no sender", func() {
		ch := make(chan int)
		<-ch
	})
	fmt.Println("end of main")
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
close twice -> recovered: close of closed channel
send after close -> recovered: send on closed channel
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan receive]:
main.main.func3()
	/tmp/go-ex/main.go:28
...
exit status 2
```

The first two are **panics**: runtime errors that unwind the stack, run deferred functions, and can be caught by `recover`. The third is not a panic. When every goroutine is blocked, the runtime reports a **fatal error** and exits the process immediately. No deferred functions run, `recover` never gets a chance, and `end of main` never prints.

Worth knowing for interviews: other fatal (unrecoverable) errors include `concurrent map writes`, running out of memory, and stack overflow. "Wrap it in `recover`" is never the fix for a deadlock or a race. The fix is the design.

</details>

---

## 91. A pipeline stage returns early and the stages before it leak

`🔴 hard` · *Find the bug*

A tidy two-stage pipeline, like example 17. `firstOver` stops as soon as it finds its answer. The result is correct, so what's wrong? How many goroutines are alive at the end?

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

func numbers(n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 1; i <= n; i++ {
			out <- i
		}
	}()
	return out
}

func square(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			out <- v * v
		}
	}()
	return out
}

// firstOver returns the first square larger than limit.
func firstOver(limit int) int {
	for sq := range square(numbers(1000)) {
		if sq > limit {
			return sq
		}
	}
	return -1
}

func main() {
	fmt.Println("first square over 50:", firstOver(50))
	time.Sleep(20 * time.Millisecond)
	fmt.Println("goroutines alive after the call:", runtime.NumGoroutine())
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
first square over 50: 64
goroutines alive after the call: 3
```

`main` plus **two leaked stages**. `firstOver` returned at 64 and stopped receiving. `square` had already taken 9 from `numbers` and is parked forever on `out <- 81`. `numbers` is parked forever on `out <- 10`. Each call to `firstOver` leaks two more goroutines, and neither `-race` nor the deadlock detector reports anything.

The rule from the Go blog's *Pipelines* article: **stages must stop when the downstream stops listening**, not only when the upstream closes. A plain `for ... range in { out <- v }` can't do that.

**Fix:** a `done` channel threaded through every stage (example 31):

```go
func firstOver(limit int) int {
	done := make(chan struct{})
	defer close(done) // tells every stage to stop on any return path
	for sq := range square(done, numbers(done, 1000)) {
		if sq > limit {
			return sq
		}
	}
	return -1
}
// ...and inside each stage: select { case out <- v: case <-done: return }
```

</details>

---

## 92. ticker.Stop() doesn't close ticker.C, so for range never ends

`🔴 hard` · *Predict the output*

A goroutine counts ticks with `for range ticker.C`. After 30ms, `main` stops the ticker. Does the goroutine print how many ticks it handled?

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

func main() {
	ticker := time.NewTicker(5 * time.Millisecond)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		n := 0
		for range ticker.C { // "ends once the ticker is stopped"
			n++
		}
		fmt.Println("handled", n, "ticks")
	}()

	time.Sleep(30 * time.Millisecond)
	ticker.Stop()

	select {
	case <-finished:
		fmt.Println("tick goroutine finished")
	case <-time.After(100 * time.Millisecond):
		fmt.Println("tick goroutine still stuck in: for range ticker.C")
	}
	fmt.Println("goroutines alive:", runtime.NumGoroutine())
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
tick goroutine still stuck in: for range ticker.C
goroutines alive: 2
```

No. `Stop` **does not close** `ticker.C`. The documentation says so explicitly: closing it could make a concurrent reader see a bogus tick. After `Stop` no more ticks are sent, so `for range ticker.C` waits forever. The goroutine leaks (the second goroutine counted), and `handled N ticks` never prints. The same is true of `time.Timer`. You can't `close` these channels yourself either, because they're receive-only.

**Fix:** the loop needs its own way out. Also let the goroutine own its ticker:

```go
go func() {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n++
		case <-quit: // closed by whoever wants the loop to end
			return
		}
	}
}()
```

</details>

---

## 93. Find all four bugs in one program, then fix them

`🔴 hard` · *Find the bug*

A realistic interview snippet: a small worker pool with a time budget. It compiles, passes `go vet` and `-race`, and returns a plausible number. **Find four bugs** before opening the answer. Hint: run it in your head with 10 jobs at 20ms each and a 50ms budget.

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

// sumSquares squares nums using 3 workers. If the whole call takes longer
// than budget, it should return the partial sum.
func sumSquares(nums []int, budget time.Duration) int {
	jobs := make(chan int)
	results := make(chan int)

	for range 3 {
		go func() {
			for n := range jobs {
				time.Sleep(20 * time.Millisecond) // pretend work
				results <- n * n
			}
			close(results)
		}()
	}

	go func() {
		for _, n := range nums {
			jobs <- n
		}
	}()

	sum := 0
	for {
		select {
		case r := <-results:
			sum += r
		case <-time.After(budget):
			return sum
		}
	}
}

func main() {
	start := time.Now()
	sum := sumSquares([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 50*time.Millisecond)
	fmt.Println("sum:", sum)
	fmt.Println("stayed within the 50ms budget:", time.Since(start) <= 70*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	fmt.Println("goroutines alive after return:", runtime.NumGoroutine())
}
```

<details>
<summary><b>Answer</b></summary>

**Output:**

```
sum: 385
stayed within the 50ms budget: false
goroutines alive after return: 4
```

All 10 results came back, but the 50ms budget was ignored, and 3 goroutines outlived the call. The four bugs:

1. **`jobs` is never closed.** The feeder just returns, so every worker's `for n := range jobs` waits forever. The 3 extra goroutines are those workers.
2. **Every worker closes `results`.** It's hidden by bug 1, because no worker ever gets past its loop. **Fix bug 1 alone and bug 2 crashes the program** with `close of closed channel` (example 83). Fixing one bug often exposes the next.
3. **`time.After(budget)` inside the loop** restarts the 50ms on every result (example 89). Results arrive every ~20ms, so the "budget" only fires once results **stop** arriving, long after 50ms.
4. **Returning early strands everyone.** When the budget does fire, workers still blocked on the unbuffered `results <- n*n` and the feeder blocked on `jobs <- n` leak forever (examples 88 and 91).

**Fixed** (`fix/main.go`):

```go
package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func sumSquares(nums []int, budget time.Duration) int {
	jobs := make(chan int)
	results := make(chan int, len(nums)) // room for every result: workers never park on it
	done := make(chan struct{})
	defer close(done) // any return tells the feeder to stop

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				time.Sleep(20 * time.Millisecond)
				results <- n * n
			}
		}()
	}

	go func() {
		defer close(jobs) // the only sender closes jobs
		for _, n := range nums {
			select {
			case jobs <- n:
			case <-done:
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results) // exactly one closer, after every worker
	}()

	deadline := time.After(budget) // one timer for the whole call
	sum := 0
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return sum // all done within budget
			}
			sum += r
		case <-deadline:
			return sum
		}
	}
}

func main() {
	start := time.Now()
	sum := sumSquares([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 50*time.Millisecond)
	fmt.Println("partial sum (budget hit):", sum < 385)
	fmt.Println("stayed within the 50ms budget:", time.Since(start) <= 70*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	fmt.Println("goroutines alive after return:", runtime.NumGoroutine())
}
```

**Output:**

```
partial sum (budget hit): true
stayed within the 50ms budget: true
goroutines alive after return: 1
```

The fixes in one line each: the feeder owns `jobs` and closes it, and stops early on `done`; one helper closes `results` after `wg.Wait()`; one `deadline` timer for the whole call; `results` has room for every value, so late workers never park; `defer close(done)` covers every return path.

</details>

---

> ← Back to the [index](README.md) · Prev: [🧩 more patterns](5-patterns.md) · Next part: [🛠️ interview tasks](7-interview-tasks.md)
