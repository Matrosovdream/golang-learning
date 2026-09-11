# Step 14 — Channels · 🧪 Edge cases

Examples **41–57**: the corners of channel behavior. Every operation in every state, what `select` really evaluates and chooses, panics `default` can't prevent, busy loops, stale `len`, silent leaks, and timers since Go 1.23. Each is a complete `package main` program: read the concept and steps, then **retype the code block** into a scratch folder and run it.

Some examples **fail on purpose**: a deadlock, a panic, a compile error, a `go vet` warning or a `-race` report is exactly what they teach. Their output is the real message, trimmed with `...` where the goroutine dump gets long.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev: [🔴 hard](3-hard.md) · Next part: [🧩 more patterns](5-patterns.md)

---

## 41. Every operation on a nil, open and closed channel

`🟢 easy` · *Channel states*

A channel is always in one of three states: **nil** (declared, never `make`d), **open**, or **closed**. Every operation has a fixed outcome in each state. Most channel bugs are one cell of this table, so it's worth knowing by heart.

**Steps:**

1. `attempt` uses `recover` to turn a panic into a printable message.
2. Send and receive go through a `select` with `default`, so "would block" prints `blocks` instead of hanging the program.
3. `len`/`cap` are read before `close`, and `close` goes last because it changes the state.

```go
package main

import "fmt"

// attempt runs op and turns a panic into its message.
func attempt(op func() string) (res string) {
	defer func() {
		if r := recover(); r != nil {
			res = fmt.Sprint("panic: ", r)
		}
	}()
	return op()
}

func main() {
	var nilCh chan int          // declared, never made
	openCh := make(chan int, 1) // empty, room for one value
	closedCh := make(chan int, 1)
	close(closedCh)

	rows := []struct {
		state string
		ch    chan int
	}{{"nil", nilCh}, {"open", openCh}, {"closed", closedCh}}

	fmt.Printf("%-7s %-30s %-10s %-31s %s\n", "state", "send", "receive", "close", "len/cap")
	for _, r := range rows {
		ch := r.ch
		send := attempt(func() string {
			select { // default turns "would block" into something printable
			case ch <- 1:
				return "ok"
			default:
				return "blocks"
			}
		})
		recv := attempt(func() string {
			select {
			case v, ok := <-ch:
				return fmt.Sprint(v, " ", ok)
			default:
				return "blocks"
			}
		})
		lenCap := fmt.Sprintf("%d/%d", len(ch), cap(ch))
		cls := attempt(func() string {
			close(ch) // last: it changes the state
			return "ok"
		})
		fmt.Printf("%-7s %-30s %-10s %-31s %s\n", r.state, send, recv, cls, lenCap)
	}
}
```

**Output:**

```
state   send                           receive    close                           len/cap
nil     blocks                         blocks     panic: close of nil channel     0/0
open    ok                             1 true     ok                              0/1
closed  panic: send on closed channel  0 false    panic: close of closed channel  0/1
```

The same results as rules (plain operations, no `default`):

| | nil | open | closed |
|---|---|---|---|
| `ch <- v` | blocks **forever** | blocks until there's room or a receiver | **panic** |
| `<-ch` | blocks **forever** | blocks until a value arrives | buffered values first, then zero value + `ok == false` immediately |
| `close(ch)` | **panic** | ok | **panic** |
| `len` / `cap` | `0` / `0` | queued / buffer size | queued / buffer size |

Notice the closed row's send: a `default` case **does not** protect you from that panic (example 46).

---

## 42. Forgot make: range over a nil channel deadlocks

`🟢 easy` · *nil channel*

The zero value of a channel is `nil`, and a nil channel is never ready. The classic way to get one is a struct built without its constructor. The code compiles, `go vet` has no complaint, and the program just stops. Nothing panics; the only report is the runtime's deadlock message.

**Steps:**

1. `worker{}` leaves `jobs` as `nil`.
2. The sender blocks forever on `w.jobs <- i`, and `range` blocks forever on the receive.
3. Every goroutine is now asleep, so the runtime kills the program.

```go
package main

import "fmt"

type worker struct {
	jobs chan int // zero value of a channel field is nil
}

func main() {
	var w worker // forgot: w.jobs = make(chan int)

	go func() {
		for i := 1; i <= 3; i++ {
			w.jobs <- i // send on nil: blocks forever
		}
		close(w.jobs)
	}()

	for j := range w.jobs { // receive on nil: blocks forever
		fmt.Println("job", j)
	}
	fmt.Println("all jobs done")
}
```

**Output:**

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan receive (nil chan)]:
main.main()
	/tmp/go-ex/main.go:19
...
exit status 2
```

The goroutine states give it away: `chan receive (nil chan)` here, and `chan send (nil chan)` in the trimmed trace of the other goroutine. A normal missing receiver would just say `chan receive`.

**Fix:** never let a channel field keep its zero value. Build the struct with a constructor such as `func newWorker() *worker { return &worker{jobs: make(chan int)} }`. A deliberate nil channel is only useful for switching off a `select` case (examples 23 and 37).

---

## 43. Sending copies the value, but a slice still shares memory

`🟢 easy` · *Ownership*

A send copies the value, just like an assignment. For a struct of plain fields that's a real, independent copy. But a slice, map, pointer or channel is a small header that *points at* shared data. The copy goes through the channel, and the data it points at is still shared with the sender.

**Steps:**

1. Send `o`, then change `o.id` and `o.items[0]` on the sender's side.
2. The receiver still sees the old `id` but the **new** item, because only the slice header was copied.
3. The fix: send `slices.Clone(items)` so the receiver gets its own backing array.

```go
package main

import (
	"fmt"
	"slices"
)

type order struct {
	id    int
	items []string // a slice header: pointer + len + cap
}

func main() {
	ch := make(chan order, 2)

	o := order{id: 1, items: []string{"apple"}}
	ch <- o                     // the struct is copied into the channel...
	o.id = 99                   // ...so a plain field changed later stays private
	o.items[0] = "rotten apple" // ...but both copies point at one backing array

	got := <-ch
	fmt.Println("shared: id =", got.id, "items =", got.items)

	// Fix: send a deep-enough copy, then the receiver owns its own data.
	o2 := order{id: 2, items: []string{"pear"}}
	ch <- order{id: o2.id, items: slices.Clone(o2.items)}
	o2.items[0] = "rotten pear"

	got = <-ch
	fmt.Println("cloned: id =", got.id, "items =", got.items)
}
```

**Output:**

```
shared: id = 1 items = [rotten apple]
cloned: id = 2 items = [pear]
```

This demo runs in one goroutine, so it's deterministic. In real code the sender and receiver run at the same time, and "change it after sending" becomes a **data race** that `-race` reports. The rule: once you've sent something, it belongs to the receiver. Don't touch it, or send a copy.

---

## 44. chan struct{} vs chan bool for signals

`🟢 easy` · *Signaling*

When a channel only says "something happened", use `chan struct{}`. `struct{}{}` takes zero bytes, so a buffer of a million tokens needs no element storage. It also has no value that can mean the wrong thing. With `chan bool`, a closed channel reads as `false`, and the receiver can't tell "not ready" from "shut down".

**Steps:**

1. `unsafe.Sizeof` shows `struct{}{}` is 0 bytes and `bool` is 1.
2. Receiving from a closed `chan bool` returns `false`, which is easy to misread.
3. With `chan struct{}` the only message is that the receive returned.

```go
package main

import (
	"fmt"
	"unsafe"
)

func main() {
	fmt.Println("size of struct{}{}:", unsafe.Sizeof(struct{}{}))
	fmt.Println("size of true:      ", unsafe.Sizeof(true))

	// A bool signal channel has a value that can be misread.
	ready := make(chan bool, 1)
	close(ready) // the producer shut down
	isReady := <-ready
	fmt.Println("closed chan bool reads as ready =", isReady, "(closed, or 'not ready'?)")

	// A struct{} channel carries no value, so there is nothing to misread:
	// the only information is that the receive happened.
	done := make(chan struct{})
	close(done)
	<-done
	fmt.Println("closed chan struct{}: receive returned, that is the whole message")

	// A buffer of empty structs costs no element storage.
	tokens := make(chan struct{}, 1_000_000)
	fmt.Println("1,000,000-slot token channel, cap:", cap(tokens))
}
```

**Output:**

```
size of struct{}{}: 0
size of true:       1
closed chan bool reads as ready = false (closed, or 'not ready'?)
closed chan struct{}: receive returned, that is the whole message
1,000,000-slot token channel, cap: 1000000
```

Use `chan bool` (or any other type) only when the value itself is information, such as a pass/fail result. For pure signals such as `done`, `quit`, `ready` and semaphore tokens, use `chan struct{}` and signal with `close` or `struct{}{}`.

---

## 45. Channel types: directions, comparison and map keys

`🟢 easy` · *Channel types*

`chan T` converts to `chan<- T` or `<-chan T` implicitly. That's what happens every time you pass a channel to a function with a directional parameter. The reverse is a compile error, so a receive-only view can never be turned back into something you can send on or close. Channels are also comparable. `==` asks "is this the same channel?", which also makes them valid map keys.

**Steps:**

1. Narrow `a` to send-only and receive-only views. They still compare equal to `a`.
2. Compare against `nil`, and use channels as map keys.
3. The `broken` program tries the two forbidden moves: closing a receive-only channel and widening it back.

```go
package main

import "fmt"

func main() {
	a := make(chan int)
	b := make(chan int)

	var sendOnly chan<- int = a // bidirectional -> send-only: implicit
	var recvOnly <-chan int = a // bidirectional -> receive-only: implicit

	fmt.Println("a == b:       ", a == b)        // two different channels
	fmt.Println("a == sendOnly:", a == sendOnly) // same channel, narrower type
	fmt.Println("a == recvOnly:", a == recvOnly)

	var none chan int
	fmt.Println("none == nil:  ", none == nil)

	// Comparable means channels can be map keys, e.g. per-subscriber state.
	names := map[chan int]string{a: "a", b: "b"}
	fmt.Println("names[a]:", names[a], "| entries:", len(names))
}
```

**Output:**

```
a == b:        false
a == sendOnly: true
a == recvOnly: true
none == nil:   true
names[a]: a | entries: 2
```

**The forbidden moves** (`broken/main.go`):

```go
package main

func consume(in <-chan int) {
	for range in {
	}
	close(in) // a receiver trying to close
}

func main() {
	ch := make(chan int)
	var recvOnly <-chan int = ch
	var back chan int = recvOnly // receive-only -> bidirectional
	_ = back
	consume(ch)
}
```

`go build` refuses both:

```
# scratch/e45/broken
./main.go:6:8: invalid operation: cannot close receive-only channel in (variable of type <-chan int)
./main.go:12:22: cannot use recvOnly (variable of type <-chan int) as chan int value in variable declaration
```

This is the compiler enforcing "only the sender closes". If a function only receives, give it `<-chan T`, and it can't close the channel even by accident.

---

## 46. select with default won't stop a send on a closed channel

`🟡 medium` · *Closed channel*

A `default` case runs only when **no** case can proceed. A send on a closed channel *can* proceed right away, straight into a panic. So a non-blocking "try send" is safe against a full buffer but not against a closed channel. There's no operation that asks "is this channel closed?" without risking that panic.

**Steps:**

1. `trySend` returns `true` when there's room and `false` when the buffer is full.
2. After `close(ch)` the send case is "ready", so `select` picks it and the runtime panics.
3. The deferred `recover` shows the message. `closed:` never prints.

```go
package main

import "fmt"

// trySend looks like a safe, non-blocking send.
func trySend(ch chan<- int, v int) bool {
	select {
	case ch <- v:
		return true
	default:
		return false // meant for "buffer full", not for "closed"
	}
}

func main() {
	ch := make(chan int, 1)
	fmt.Println("open, empty:", trySend(ch, 1))
	fmt.Println("open, full: ", trySend(ch, 2))

	close(ch)
	defer func() {
		fmt.Println("recovered:  ", recover())
	}()
	fmt.Println("closed:     ", trySend(ch, 3)) // never prints
}
```

**Output:**

```
open, empty: true
open, full:  false
recovered:   send on closed channel
```

**Fix:** don't make senders guess. The goroutine that closes a channel must be the one that sends on it, or must know every sender has already stopped. If a receiver needs producers to stop, it closes a separate `done` channel that the producers `select` on (examples 47 and 75).

---

## 47. Closing a channel panics the senders already waiting on it

`🟡 medium` · *Closed channel*

`close` doesn't only affect future sends. Every goroutine **already parked** in a send on that channel wakes up and panics. That's why a receiver must never close to mean "stop sending": producers that are mid-send when the close lands blow up.

**Steps:**

1. Three producers each try to send one job on an unbuffered channel.
2. The receiver takes one, waits briefly so the other two are parked, then closes.
3. Both parked producers panic with `send on closed channel`. Here `recover` catches it; in real code it would crash the process.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	panicked := 0
	var message any

	for id := 1; id <= 3; id++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					panicked++
					message = r
					mu.Unlock()
				}
			}()
			jobs <- id // parks until someone receives
		}()
	}

	<-jobs                            // the receiver takes one job...
	time.Sleep(20 * time.Millisecond) // (let the other two park on their send)
	close(jobs)                       // ...then "tells producers to stop" by closing
	wg.Wait()

	fmt.Println("received 1 job, then closed the channel")
	fmt.Println("producers that panicked:", panicked)
	fmt.Println("panic message:", message)
}
```

**Output:**

```
received 1 job, then closed the channel
producers that panicked: 2
panic message: send on closed channel
```

Run it with `go run -race .` and you also get `WARNING: DATA RACE`, with a stack pointing at `close(jobs)` and at `jobs <- id`. The race detector treats a `close` that isn't synchronized with a send as a data race. That's a second signal the design is wrong, and this example trips it on purpose.

In the room picture: closing the hole while workers are standing at it, box in hand, is an accident, not a polite "we're done". The receiver's correct move is to close a separate `stop` channel that producers `select` on alongside their send:

```go
select {
case jobs <- id:
case <-stop: // receiver lost interest: give up quietly
	return
}
```

---

## 48. break inside select only exits the select

`🟡 medium` · *select*

Inside a `select` (or a `switch`), a bare `break` ends only that statement. In a `for`-`select` loop the loop keeps going, and a closed channel is always ready, so the loop turns into a busy spin that keeps "breaking". Use a labeled `break`, or `return`.

**Steps:**

1. The buggy loop hits `break` on close, and the `for` goes around again. A cap of 5 polls stops it for the demo.
2. The fixed loop breaks out with `break loop`.

```go
package main

import "fmt"

func main() {
	ch := make(chan int)
	go func() {
		for i := 1; i <= 3; i++ {
			ch <- i
		}
		close(ch)
	}()

	polls := 0
	for polls < 5 { // safety cap: without it this loop never ends
		polls++
		select {
		case v, ok := <-ch:
			if !ok {
				fmt.Println("closed -> break")
				break // leaves the select, NOT the for loop
			}
			fmt.Println("got", v)
		}
	}
	fmt.Println("buggy loop polled", polls, "times")

	// Fix: label the loop and break the label (or return).
	ch2 := make(chan int)
	go func() { ch2 <- 1; close(ch2) }()
loop:
	for {
		select {
		case v, ok := <-ch2:
			if !ok {
				break loop
			}
			fmt.Println("got", v)
		}
	}
	fmt.Println("labeled break left the loop")
}
```

**Output:**

```
got 1
got 2
got 3
closed -> break
closed -> break
buggy loop polled 5 times
got 1
labeled break left the loop
```

`closed -> break` prints twice: every poll after the close finds the channel ready again. Without the cap, that line would print forever at 100% CPU (example 51). `continue` has no such issue because `select` isn't a loop, so it applies to the enclosing `for`.

---

## 49. Every case expression is evaluated, even in cases that don't run

`🟡 medium` · *select*

When execution reaches a `select`, the spec says every case's **channel expression** and every **value to send** are evaluated exactly once, in source order, *before* choosing a case. The losing cases' expressions still ran. Only the chosen case's body, and the assignment on its left-hand side, are conditional.

**Steps:**

1. Wrap each channel and send value in a function that prints when it's called.
2. Only the receive is ready, yet all four expressions print first, top to bottom.

```go
package main

import "fmt"

func value(name string) int {
	fmt.Println("  evaluated:", name)
	return 1
}

func channel(name string, ch chan int) chan int {
	fmt.Println("  evaluated:", name)
	return ch
}

func main() {
	ready := make(chan int, 1)
	ready <- 7
	noReceiver := make(chan int) // unbuffered, nobody receives: never ready
	var nilCh chan int

	fmt.Println("entering select")
	select {
	case v := <-channel("receive channel (ready)", ready):
		fmt.Println("chosen: receive", v)
	case noReceiver <- value("send value (no receiver)"):
		fmt.Println("chosen: send")
	case channel("send channel (nil)", nilCh) <- value("send value (nil)"):
		fmt.Println("chosen: nil send")
	}
}
```

**Output:**

```
entering select
  evaluated: receive channel (ready)
  evaluated: send value (no receiver)
  evaluated: send channel (nil)
  evaluated: send value (nil)
chosen: receive 7
```

Where this bites: `case out <- expensiveCompute():` runs the computation **every time** the loop enters the `select`, even on rounds where the timeout or `done` case wins. Compute the value before the `select` (or in its own goroutine), so the `select` only waits.

---

## 50. select picks at random: a closed done doesn't stop a ready send

`🟡 medium` · *select*

When several cases are ready, `select` picks one **uniformly at random**. Source order means nothing. So if both `out <- v` and `<-done` are ready, a "stopped" worker still sends about half the time. That's by design, so no single case can starve the others. If stopping must win, check `done` first on its own.

**Steps:**

1. `done` is already closed and `out` has room, so both cases are ready in every round.
2. Count how often the send wins: roughly half of the 10,000 rounds.
3. A pre-check `select { case <-done: continue; default: }` makes "stop" win every time.

```go
package main

import "fmt"

func main() {
	const rounds = 10_000
	done := make(chan struct{})
	close(done) // "stop!" has already been signaled

	out := make(chan int, 1)
	sends := 0
	for range rounds {
		select {
		case out <- 1: // also ready: the buffer has room
			<-out // empty it for the next round
			sends++
		case <-done:
		}
	}
	pct := 100 * sends / rounds
	fmt.Println("both cases ready in every round:", rounds)
	fmt.Println("send still chosen 40-60% of the time:", pct >= 40 && pct <= 60)

	// Fix: when stopping must win, check done on its own first.
	sends = 0
	for range rounds {
		select {
		case <-done:
			continue
		default:
		}
		select {
		case out <- 1:
			<-out
			sends++
		case <-done:
		}
	}
	fmt.Println("with a done pre-check, sends:", sends)
}
```

**Output:**

```
both cases ready in every round: 10000
send still chosen 40-60% of the time: true
with a done pre-check, sends: 0
```

The pre-check is only *best effort*. If `done` closes between the pre-check and the second `select`, the random choice applies again. That's usually fine: the worker stops on its next round. When "not one more send after stop" is a hard requirement, the sender needs to own the decision (for example, a single goroutine that both sends and handles stopping).

---

## 51. Two ways a for-select burns 100% CPU

`🟡 medium` · *select*

A `for`-`select` loop is only cheap while the `select` **blocks**. Two common mistakes stop it from ever blocking. The loop still gives the right answer, but it burns a whole CPU core while it waits.

1. **A `default` case.** "Nothing ready" becomes "try again right away".
2. **A closed channel in a case.** It's ready on every pass and returns the zero value immediately (example 6).

**Steps:**

1. Run each loop while the real value is 50ms away, and count the useless iterations.
2. Fix: remove the `default` so the `select` blocks, and set a closed channel to `nil` so its case is switched off.

```go
package main

import (
	"fmt"
	"time"
)

// later sends v on ch after 50ms.
func later(ch chan<- int, v int) {
	time.Sleep(50 * time.Millisecond)
	ch <- v
}

func main() {
	// Spin 1: a default case means the select never waits.
	work := make(chan int)
	go later(work, 1)
	polls := 0
	for got := false; !got; {
		select {
		case <-work:
			got = true
		default:
			polls++ // "nothing yet, try again" - as fast as the CPU allows
		}
	}
	fmt.Println("default: polled more than 10,000 times in 50ms:", polls > 10_000)

	// Spin 2: a closed channel is ready on every pass.
	finished := make(chan int)
	close(finished)
	other := make(chan int)
	go later(other, 2)
	wakeups := 0
	for got := false; !got; {
		select {
		case <-finished: // closed: returns zero instantly, every time
			wakeups++
		case <-other:
			got = true
		}
	}
	fmt.Println("closed channel: woke the loop more than 10,000 times:", wakeups > 10_000)

	// Fixes: no default (just block), and nil out a channel once it closes.
	finished = make(chan int)
	close(finished)
	go later(other, 3)
	wakeups = 0
	for got := false; !got; {
		select {
		case _, ok := <-finished:
			wakeups++
			if !ok {
				finished = nil // a nil channel is never ready: case switched off
			}
		case <-other:
			got = true
		}
	}
	fmt.Println("with the nil fix, wakeups:", wakeups)
}
```

**Output:**

```
default: polled more than 10,000 times in 50ms: true
closed channel: woke the loop more than 10,000 times: true
with the nil fix, wakeups: 1
```

`default` is for a single "check without waiting" (examples 13 and 14), never for a loop that is waiting for something. If you must poll, add a `time.Ticker` case so the loop wakes up at a reasonable interval.

---

## 52. A buffered send isn't a delivery

`🟡 medium` · *Buffered*

When a send on a buffered channel returns, the value is **on the shelf**. Nobody has received it yet. If `main` returns at that point, the program exits, the consumer goroutine is killed and every queued value is gone. An unbuffered send is different: it returns only after a receiver has actually taken the value.

**Steps:**

1. Send 3 jobs into a buffered channel whose consumer starts late. All sends return immediately, and nothing is processed yet.
2. Fix: `close(jobs)`, then wait on `consumerDone`, so everything is processed before moving on.
3. Contrast: an unbuffered send blocks until its (late) receiver shows up.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	// Buffered: the sender drops the boxes on the shelf and walks away.
	jobs := make(chan string, 3)
	processed := make(chan string, 3)
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		time.Sleep(20 * time.Millisecond) // the consumer starts late
		for j := range jobs {
			processed <- j
		}
	}()

	for _, j := range []string{"a", "b", "c"} {
		jobs <- j // returns immediately: there is room on the shelf
	}
	fmt.Println("all 3 sends returned; still sitting in the buffer:", len(jobs))
	fmt.Println("processed so far:", len(processed), "(if main returned now, all 3 are lost)")

	// Fix: close to say "no more", then wait for the consumer to finish.
	close(jobs)
	<-consumerDone
	fmt.Println("after close + wait, processed:", len(processed))

	// Contrast: an unbuffered send returns only once a receiver has the value.
	handoff := make(chan string)
	go func() {
		time.Sleep(20 * time.Millisecond) // this receiver starts late too
		<-handoff
	}()
	start := time.Now()
	handoff <- "x"
	fmt.Println("unbuffered send waited for the late receiver:", time.Since(start) >= 15*time.Millisecond)
}
```

**Output:**

```
all 3 sends returned; still sitting in the buffer: 3
processed so far: 0 (if main returned now, all 3 are lost)
after close + wait, processed: 3
unbuffered send waited for the late receiver: true
```

"The send returned" means the value was handed over (unbuffered) or queued (buffered). It never means the work was *done*. For graceful shutdown, you wait for the consumer to finish, not for the sends to return.

---

## 53. len(ch) is stale as soon as you read it

`🟡 medium` · *Races*

`len(ch)` is a snapshot. Another goroutine can change the channel before your next line runs. "If `len(ch) > 0`, then receive" is a **check-then-act race**: two consumers both see one item, both commit to receiving, and one of them blocks forever. `-race` can't catch this, because nothing is written to shared memory. The logic is simply wrong.

**Steps:**

1. Two consumers both check `len(ch) > 0` while one item is queued. A barrier makes both check before either receives.
2. One gets the item. The other would block forever; a 50ms timeout stands in for "forever" so the demo finishes.
3. Fix: a single `select` with `default`. The receive is the check, and it's atomic.

```go
package main

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

func main() {
	ch := make(chan int, 1)
	ch <- 42 // exactly one item
	results := make(chan string, 2)

	// Buggy: check len, then receive.
	var checked sync.WaitGroup
	bothChecked := make(chan struct{})
	for range 2 {
		checked.Add(1)
		go func() {
			if len(ch) > 0 { // both consumers see 1 item...
				checked.Done()
				<-bothChecked // (force the unlucky interleaving for the demo)
				select {
				case v := <-ch: // ...but only one of them can take it
					results <- fmt.Sprint("got ", v)
				case <-time.After(50 * time.Millisecond): // stands in for "blocked forever"
					results <- "stuck: the item was already gone"
				}
			}
		}()
	}
	checked.Wait()
	close(bothChecked)
	fmt.Println("len-then-receive:", collect(results))

	// Fix: let the receive itself be the check.
	ch <- 7
	for range 2 {
		go func() {
			select {
			case v := <-ch:
				results <- fmt.Sprint("got ", v)
			default:
				results <- "nothing to take, moved on"
			}
		}()
	}
	fmt.Println("select/default:  ", collect(results))
}

func collect(results <-chan string) []string {
	got := []string{<-results, <-results}
	slices.Sort(got)
	return got
}
```

**Output:**

```
len-then-receive: [got 42 stuck: the item was already gone]
select/default:   [got 7 nothing to take, moved on]
```

`len` and `cap` are fine for metrics and logging ("queue depth: 17"). Never make a decision based on them. If you need "take one if there is one", that's a `select` with `default`. If you need "wait for one", that's a plain receive.

---

## 54. A worker that sends into its own full queue deadlocks

`🟡 medium` · *Deadlock*

Crawlers and tree or graph walkers often find new work while processing work. Pushing that new work into the **same channel the worker reads from** works while the buffer has room. It deadlocks the moment the buffer fills, because the only goroutine that could make room is the one stuck on the send. A bigger buffer only moves the failure to a bigger input.

**Steps:**

1. Seed the queue with one item. Each item with `n > 0` enqueues two children.
2. With a buffer of 2, the loop fills the queue and then blocks on its own send.
3. Fix (`fix/`): when the consumer is also the producer, use a plain slice for the backlog.

```go
package main

import "fmt"

func main() {
	queue := make(chan int, 2) // a small buffer
	queue <- 3                 // seed: expand a tree 3 levels deep

	for n := range queue {
		fmt.Println("processing", n, "| queued:", len(queue))
		if n > 0 {
			queue <- n - 1 // each item discovers two children...
			queue <- n - 1 // ...but the only receiver is this very loop
		}
	}
	fmt.Println("done") // never reached
}
```

**Output:**

```
processing 3 | queued: 0
processing 2 | queued: 1
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan send]:
main.main()
	/tmp/go-ex/main.go:13
exit status 2
```

**Fix** (`fix/main.go`):

```go
package main

import "fmt"

func main() {
	backlog := []int{3} // the worker owns its backlog: appending never blocks
	processed := 0

	for len(backlog) > 0 {
		n := backlog[0]
		backlog = backlog[1:]
		processed++
		if n > 0 {
			backlog = append(backlog, n-1, n-1)
		}
	}
	fmt.Println("processed", processed, "items") // 1 + 2 + 4 + 8
}
```

**Output:**

```
processed 15 items
```

With **several** workers sharing the queue, a slice needs a mutex, and deciding when everything is finished gets harder. Example 109 (web crawler) solves that by counting work in progress, and example 74 shows an unbounded queue between two channels.

---

## 55. The runtime only reports a deadlock when every goroutine is stuck

`🟡 medium` · *Leaks*

`fatal error: all goroutines are asleep` sounds like a deadlock detector, but it fires only when **no** goroutine can ever run again. As long as anything else is alive, the stuck goroutines stay silent: `main`, an HTTP server, a timer, a test runner. They're a **leak**: memory and stack that are never freed. In a server that runs for weeks, this is the more common failure.

**Steps:**

1. `search` asks three replicas, takes the first answer, and returns. With an unbuffered channel the two losers park on their send forever.
2. After 100 searches, `runtime.NumGoroutine()` shows 200 leaked goroutines, and no error was reported.
3. Fix: a buffer with room for every sender, so each loser's send completes and its goroutine exits.

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

// search returns the first replica's answer. bufSize is the bug (0) or the fix.
func search(query string, bufSize int) string {
	replicas := []string{"r1", "r2", "r3"}
	results := make(chan string, bufSize)
	for _, r := range replicas {
		go func() {
			results <- r + ":" + query // with no buffer, the two losers park here forever
		}()
	}
	return <-results
}

func main() {
	before := runtime.NumGoroutine()
	for i := range 100 {
		search(fmt.Sprint("q", i), 0)
	}
	time.Sleep(50 * time.Millisecond) // let every goroutine reach its send
	fmt.Println("goroutines leaked by 100 searches:", runtime.NumGoroutine()-before)
	fmt.Println("no deadlock reported: main is still running")

	before = runtime.NumGoroutine()
	for i := range 100 {
		search(fmt.Sprint("q", i), 3) // room for every replica's answer
	}
	time.Sleep(50 * time.Millisecond)
	fmt.Println("goroutines leaked with a buffer of 3:", runtime.NumGoroutine()-before)
}
```

**Output:**

```
goroutines leaked by 100 searches: 200
no deadlock reported: main is still running
goroutines leaked with a buffer of 3: 0
```

A goroutine blocked on a channel is never garbage collected, even if nothing references that channel anymore. Ways to catch leaks: watch `runtime.NumGoroutine()` in metrics, read the goroutine profile (`/debug/pprof/goroutine?debug=1` shows many goroutines parked at the same line), or use `go.uber.org/goleak` in tests.

---

## 56. signal.Notify drops signals sent to an unbuffered channel

`🟡 medium` · *Library channels*

Some standard-library APIs send **into your channel** and refuse to block while doing it. `os/signal` is the famous one: if your channel isn't ready at that exact moment, the signal is **dropped**. With an unbuffered channel, "ready" means a goroutine is parked on the receive right now. During startup, or while `main` is busy, that's usually not the case, so your Ctrl+C handler silently misses the signal. `go vet` knows this pattern and warns.

**Steps:**

1. Register an unbuffered channel, signal ourselves (`SIGUSR1`) while `main` is sleeping, then try to receive.
2. Do the same with a buffer of 1: the signal waits in the buffer until `main` gets to it.
3. Run `go vet` on the program to see the warning. (macOS/Linux: `syscall.Kill` isn't available on Windows.)

```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// deliver signals our own process while main is busy (not receiving),
// then reports whether the signal ever reached the channel.
func deliver(sigs <-chan os.Signal) bool {
	syscall.Kill(os.Getpid(), syscall.SIGUSR1)
	time.Sleep(50 * time.Millisecond) // busy: nobody is receiving yet

	select {
	case <-sigs:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func main() {
	unbuffered := make(chan os.Signal)
	signal.Notify(unbuffered, syscall.SIGUSR1) // go vet flags this line
	fmt.Println("unbuffered channel got the signal:", deliver(unbuffered))
	signal.Stop(unbuffered)

	buffered := make(chan os.Signal, 1)
	signal.Notify(buffered, syscall.SIGUSR1)
	fmt.Println("buffered channel got the signal:  ", deliver(buffered))
}
```

**Output:**

```
unbuffered channel got the signal: false
buffered channel got the signal:   true
```

**`go vet .`** (this example fails vet on purpose):

```
main.go:27:2: misuse of unbuffered os.Signal channel as argument to signal.Notify
```

The general lesson: when code you don't control sends with `select`/`default` (signal delivery, some event buses and metrics hooks), the **buffer is part of the contract**. For shutdown handling, the idiom is `sigs := make(chan os.Signal, 1)`, or `signal.NotifyContext` (lesson 15).

---

## 57. time.Timer Stop and Reset since Go 1.23

`🔴 hard` · *Timers*

Before Go 1.23, a timer's channel had a 1-slot buffer. A tick that fired but was never received sat there, and after `Stop` or `Reset` you could read that **stale** value. So everyone learned the drain idiom `if !t.Stop() { <-t.C }`. Since Go 1.23 (when your `go.mod` says `go 1.23` or later), timer channels are **synchronous**. `Stop` and `Reset` guarantee that no stale tick is delivered afterwards, and `len`/`cap` report 0. Unreferenced timers are also garbage collected even without `Stop`.

**Steps:**

1. `len`/`cap` of `t.C` are 0.
2. Let the timer expire without receiving. `Stop` returns `true` and no stale tick arrives.
3. Let it expire again, then `Reset(40ms)` without draining. The receive waits for the new deadline.
4. After a tick **was** received, the old idiom's `<-t.C` would block forever. A timeout shows it.

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.NewTimer(10 * time.Millisecond)
	fmt.Println("len/cap of t.C:", len(t.C), cap(t.C)) // synchronous channel since Go 1.23

	time.Sleep(30 * time.Millisecond) // the timer expires, nobody receives
	fmt.Println("Stop after expiry, tick never received:", t.Stop())
	select {
	case <-t.C:
		fmt.Println("stale tick received after Stop")
	case <-time.After(30 * time.Millisecond):
		fmt.Println("no stale tick after Stop")
	}

	// Reset without draining: the next receive waits for the NEW deadline.
	t.Reset(10 * time.Millisecond)
	time.Sleep(30 * time.Millisecond) // expires again, still not received
	start := time.Now()
	t.Reset(40 * time.Millisecond)
	<-t.C
	fmt.Println("after Reset, waited for the new 40ms deadline:", time.Since(start) >= 35*time.Millisecond)

	// The old idiom "if !t.Stop() { <-t.C }" after the tick was already received:
	// Stop reports false, and there is nothing left to drain.
	if !t.Stop() {
		select {
		case <-t.C:
			fmt.Println("drained a value")
		case <-time.After(30 * time.Millisecond):
			fmt.Println("old drain idiom: a bare <-t.C would block forever")
		}
	}
}
```

**Output:**

```
len/cap of t.C: 0 0
Stop after expiry, tick never received: true
no stale tick after Stop
after Reset, waited for the new 40ms deadline: true
old drain idiom: a bare <-t.C would block forever
```

With a `go 1.22` (or older) line in `go.mod`, the runtime keeps the old asynchronous behavior (`GODEBUG=asynctimerchan=1`), so the second and third lines would differ. The practical rules today: call `Reset`/`Stop` without draining, and never write a bare `<-t.C` unless you know the tick hasn't been received yet. The same applies to `time.Ticker`.

---

> ← Back to the [index](README.md) · Prev: [🔴 hard](3-hard.md) · Next part: [🧩 more patterns](5-patterns.md)
