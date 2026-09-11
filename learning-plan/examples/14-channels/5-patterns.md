# Step 14 — Channels · 🧩 More patterns

Examples **58–77**: patterns beyond the basics. Backpressure, stop-and-wait lifecycles, request–reply, channel-built locks and pools, priority and latest-value handling, the *Concurrency in Go* toolkit (or-done, tee, bridge), futures, batching, debouncing, ordered streaming, the three closing rules, `reflect.Select`, and a supervisor. Everything here uses `done` channels rather than `context`, which arrives in lesson 15. Each is a complete `package main` program: read the concept and steps, then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # these are concurrent: try go run -race . too
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev: [🧪 edge cases](4-edge-cases.md) · Next part: [🎯 interview quiz](6-interview-quiz.md)

---

## 58. Backpressure you can watch: the producer stops at cap

`🟢 easy` · *Backpressure*

A bounded buffer is a brake. When the consumer falls behind, the buffer fills and the producer's next send **blocks**, so the producer runs exactly as fast as the consumer and memory stays bounded. Nobody has to write "slow down" logic; the channel does it.

**Steps:**

1. An endless producer records how many sends have completed.
2. With nobody receiving, it stops after exactly `cap` = 3 items.
3. Each receive makes room for exactly one more send.

```go
package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

func main() {
	queue := make(chan int, 3)
	var produced atomic.Int32

	go func() {
		for i := 1; ; i++ {
			queue <- i // blocks while the buffer is full
			produced.Store(int32(i))
		}
	}()

	time.Sleep(20 * time.Millisecond) // the consumer hasn't started yet
	fmt.Println("consumer idle:   producer stopped after", produced.Load(), "items (cap 3)")

	<-queue
	<-queue
	time.Sleep(20 * time.Millisecond)
	fmt.Println("consumer took 2: producer moved on to", produced.Load())
}
```

**Output:**

```
consumer idle:   producer stopped after 3 items (cap 3)
consumer took 2: producer moved on to 5
```

The producer sent items 4 and 5 into the two freed slots and is now blocked on item 6. That's the "full shelf" from the lesson's mental model. The opposite designs, an unbounded queue (example 74) or dropping when full (example 14), remove the brake, and each has a cost you have to choose deliberately.

---

## 59. Stop and wait: quit in, stopped out, Stop() safe twice

`🟡 medium` · *Lifecycle*

A background loop needs two signals, not one. `quit` goes **in** ("please exit"), and `stopped` comes **out** ("I really have exited, cleanup included"). Closing `quit` alone returns immediately while the loop may still be running, still holding resources or writing state. `Stop` should also be idempotent, since shutdown paths often call it twice.

**Steps:**

1. The loop does `defer close(p.stopped)`, so `stopped` closes on every exit path.
2. `Stop` closes `quit` inside a `sync.Once`, then blocks on `<-p.stopped`.
3. After `Stop` returns, `main` may read `p.polls` safely: the loop's last write happened before `stopped` was closed.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type Poller struct {
	quit     chan struct{} // closed by Stop: "please exit"
	stopped  chan struct{} // closed by the loop: "I have exited"
	stopOnce sync.Once
	polls    int // written only by the loop; read after stopped is closed
}

func StartPoller(every time.Duration) *Poller {
	p := &Poller{quit: make(chan struct{}), stopped: make(chan struct{})}
	go p.loop(every)
	return p
}

func (p *Poller) loop(every time.Duration) {
	defer close(p.stopped)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.polls++
		case <-p.quit:
			fmt.Println("loop: cleaning up")
			return
		}
	}
}

// Stop asks the loop to exit and waits until it has. Safe to call repeatedly.
func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.quit) })
	<-p.stopped
}

func main() {
	p := StartPoller(5 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	p.Stop()
	fmt.Println("main: Stop returned, polled at least 3 times:", p.polls >= 3)
	p.Stop() // no double-close panic, returns at once
	fmt.Println("main: second Stop returned too")
}
```

**Output:**

```
loop: cleaning up
main: Stop returned, polled at least 3 times: true
main: second Stop returned too
```

The same handshake appears in real code as `Close()` on clients and servers. With `context` (lesson 15) the `quit` side becomes `ctx.Done()`, but you still need the `stopped` side, or a `WaitGroup`, to know the goroutine has finished.

---

## 60. Request–reply: put a reply channel inside the request

`🟡 medium` · *Patterns*

A channel only carries values one way. To get an answer back, send a channel **inside** the request: "do this, and put the result here". Paired with a goroutine that owns some state, this gives you a small server with no mutex. Every request is handled one at a time by the owner.

**Steps:**

1. `request` carries its own `reply chan reply`.
2. `bank` is the only goroutine that touches `balances`. It handles one request at a time and answers on `req.reply`.
3. `call` creates a buffered reply channel of 1, so the bank never blocks on a caller that went away.

```go
package main

import (
	"errors"
	"fmt"
)

type reply struct {
	balance int
	err     error
}

func (r reply) String() string {
	if r.err != nil {
		return fmt.Sprintf("refused (%v), balance %d", r.err, r.balance)
	}
	return fmt.Sprintf("ok, balance %d", r.balance)
}

type request struct {
	account string
	amount  int        // positive: deposit, negative: withdrawal
	reply   chan reply // where the bank sends the answer
}

// bank owns the balances map: no other goroutine ever touches it.
func bank(requests <-chan request) {
	balances := map[string]int{}
	for req := range requests {
		next := balances[req.account] + req.amount
		if next < 0 {
			req.reply <- reply{balances[req.account], errors.New("insufficient funds")}
			continue
		}
		balances[req.account] = next
		req.reply <- reply{next, nil}
	}
}

func call(requests chan<- request, account string, amount int) reply {
	r := make(chan reply, 1) // buffered: the bank never waits on a slow caller
	requests <- request{account, amount, r}
	return <-r
}

func main() {
	requests := make(chan request)
	go bank(requests)
	defer close(requests)

	fmt.Println("alice +100:", call(requests, "alice", 100))
	fmt.Println("alice -30: ", call(requests, "alice", -30))
	fmt.Println("alice -500:", call(requests, "alice", -500))

	// Many concurrent callers, still no mutex: the bank serializes them.
	balances := make(chan int)
	for range 10 {
		go func() { balances <- call(requests, "bob", 10).balance }()
	}
	highest := 0
	for range 10 {
		highest = max(highest, <-balances)
	}
	fmt.Println("bob after 10 concurrent +10:", highest)
}
```

**Output:**

```
alice +100: ok, balance 100
alice -30:  ok, balance 70
alice -500: refused (insufficient funds), balance 70
bob after 10 concurrent +10: 100
```

Why buffer the reply channel: if a caller ever gives up waiting (a timeout), an unbuffered reply would leave the bank parked on the send forever, and the whole bank would be stuck (example 88 shows the leak). This is the basis of the "actor" and "hub" designs in lessons 58 and 67.

---

## 61. A mutex built from a channel: Lock, TryLock, LockTimeout

`🟡 medium` · *Interview classic*

A buffered channel with **one** slot is a lock: a send takes the lock (it blocks while the slot is full), and a receive releases it. It's a common interview warm-up because it shows you understand buffering. The channel version also has one real advantage over `sync.Mutex`: it works with `select`, so "lock, or give up after a timeout" is a two-case `select`.

**Steps:**

1. `Lock` sends a token and `Unlock` takes it back. `Unlock` on an empty slot panics, like `sync.Mutex`.
2. `TryLock` is a `select` with `default`; `LockTimeout` races the send against a timer.
3. 100 goroutines increment a counter under the lock (and `-race` stays quiet). Then the three lock flavours are tried while the lock is held.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// Mutex holds a single token slot: holding the lock = having put a token in.
type Mutex struct{ slot chan struct{} }

func NewMutex() *Mutex { return &Mutex{slot: make(chan struct{}, 1)} }

func (m *Mutex) Lock() { m.slot <- struct{}{} }

func (m *Mutex) Unlock() {
	select {
	case <-m.slot:
	default:
		panic("unlock of unlocked Mutex")
	}
}

func (m *Mutex) TryLock() bool {
	select {
	case m.slot <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *Mutex) LockTimeout(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case m.slot <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

func main() {
	m := NewMutex()
	counter := 0
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			counter++
			m.Unlock()
		}()
	}
	wg.Wait()
	fmt.Println("counter after 100 goroutines:", counter)

	m.Lock()
	fmt.Println("TryLock while held:           ", m.TryLock())
	fmt.Println("LockTimeout(20ms) while held: ", m.LockTimeout(20*time.Millisecond))
	go func() {
		time.Sleep(10 * time.Millisecond)
		m.Unlock()
	}()
	fmt.Println("LockTimeout(1s), freed at 10ms:", m.LockTimeout(time.Second))
	m.Unlock()

	defer func() { fmt.Println("recovered:", recover()) }()
	m.Unlock() // not held any more
}
```

**Output:**

```
counter after 100 goroutines: 100
TryLock while held:            false
LockTimeout(20ms) while held:  false
LockTimeout(1s), freed at 10ms: true
recovered: unlock of unlocked Mutex
```

In production, reach for `sync.Mutex` (faster; it has `TryLock` too). The channel lock earns its place when waiting must be cancellable. The same idea with `N` slots is the semaphore from examples 19 and 29.

---

## 62. A blocking resource pool: a buffered channel of connections

`🟡 medium` · *Patterns*

Fill a buffered channel with a fixed number of expensive objects (connections, workers, big buffers), and it becomes a pool. **Receiving** borrows an object, and it blocks while all of them are in use. **Sending** returns one. Unlike `sync.Pool` (lesson 28), which is a cache the GC may empty at any time, this pool is **bounded**, **blocking**, and never loses objects.

**Steps:**

1. `NewPool` pre-fills the channel with `size` connections.
2. `Get` waits for a free connection with a timeout. `Put` returns it.
3. Six workers share two connections, and the peak in use never exceeds 2. An exhausted pool makes `Get` give up instead of hanging.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Conn struct{ id int }

// Pool hands out a fixed set of connections; the channel is the free list.
type Pool struct{ free chan *Conn }

func NewPool(size int) *Pool {
	p := &Pool{free: make(chan *Conn, size)}
	for i := 1; i <= size; i++ {
		p.free <- &Conn{id: i}
	}
	return p
}

// Get waits for a free connection, but no longer than wait.
func (p *Pool) Get(wait time.Duration) (*Conn, bool) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case c := <-p.free:
		return c, true
	case <-timer.C:
		return nil, false
	}
}

func (p *Pool) Put(c *Conn) { p.free <- c }

func main() {
	pool := NewPool(2)
	var inUse, peak atomic.Int32
	var wg sync.WaitGroup

	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, ok := pool.Get(time.Second)
			if !ok {
				return
			}
			defer pool.Put(c)
			n := inUse.Add(1)
			for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
			}
			time.Sleep(10 * time.Millisecond) // use the connection
			inUse.Add(-1)
		}()
	}
	wg.Wait()
	fmt.Println("6 workers, pool of 2 -> peak connections in use:", peak.Load())

	a, _ := pool.Get(time.Second)
	b, _ := pool.Get(time.Second)
	_, ok := pool.Get(20 * time.Millisecond)
	fmt.Println("third Get on an exhausted pool succeeded:", ok)
	pool.Put(a)
	pool.Put(b)
	fmt.Println("free connections after returning both:", len(pool.free))
}
```

**Output:**

```
6 workers, pool of 2 -> peak connections in use: 2
third Get on an exhausted pool succeeded: false
free connections after returning both: 2
```

Checking `len(pool.free)` is fine here: nothing else is running, so it's a report, not a decision (see example 53). In real pools, also think about **broken** objects: a connection that fails health-checks should be closed and replaced, not `Put` back.

---

## 63. Priority select: always empty high before touching low

`🟡 medium` · *select*

`select` has no priorities (example 50), so one flat `select` over `high` and `low` mixes them at random. To prefer one channel, **nest two selects**. The first checks only `high`, with a `default`. The second, reached only when nothing urgent is ready, waits on both. It's a standard answer to "select is random; how do you prioritize?"

**Steps:**

1. Queue three `low` and three `HIGH` messages, interleaved.
2. Every pass tries `high` alone first. It falls through to the two-case `select` only when `high` is empty.
3. A closed channel is set to `nil`, which switches its cases off in both selects.

```go
package main

import "fmt"

func main() {
	high := make(chan string, 10)
	low := make(chan string, 10)
	for i := 1; i <= 3; i++ { // interleaved on purpose
		low <- fmt.Sprint("low-", i)
		high <- fmt.Sprint("HIGH-", i)
	}
	close(high)
	close(low)

	for high != nil || low != nil {
		// 1) Is anything urgent ready right now? Take it first.
		select {
		case msg, ok := <-high:
			if !ok {
				high = nil
				continue
			}
			fmt.Println(msg)
			continue
		default:
		}
		// 2) Nothing urgent: wait on both.
		select {
		case msg, ok := <-high:
			if !ok {
				high = nil
				continue
			}
			fmt.Println(msg)
		case msg, ok := <-low:
			if !ok {
				low = nil
				continue
			}
			fmt.Println(msg)
		}
	}
}
```

**Output:**

```
HIGH-1
HIGH-2
HIGH-3
low-1
low-2
low-3
```

The priority applies only to messages that are **already waiting**. While the loop is blocked in the second `select`, a `high` and a `low` message arriving at the same moment are again a coin flip. That's usually fine. If you need strict ordering, the producers must put priorities into a single queue (a heap, lesson 52).

---

## 64. Keep only the latest value: replace instead of blocking

`🟡 medium` · *Patterns*

Sometimes only the **newest** value matters: a sensor reading, a progress percentage, "config changed", a UI state. Old unread values are useless, and the producer must never wait on a slow reader. A channel with capacity 1 plus a non-blocking "take out the stale value, put in the new one" gives you a **mailbox that keeps only the latest value**. Other names for this are conflation or a latest-value channel.

**Steps:**

1. `publish` tries to send. If the slot is full, it takes out the stale value and tries again. It never blocks.
2. Five publishes with no reader leave only `5` in the mailbox.
3. A slow display reading a fast sensor skips intermediate readings but always ends on the latest.

```go
package main

import (
	"fmt"
	"time"
)

// publish puts v into a 1-slot mailbox, throwing away an unread older value.
// It never blocks. Use it from a single publisher.
func publish(box chan int, v int) {
	for {
		select {
		case box <- v:
			return
		default: // full: drop the stale value (if still there) and retry
			select {
			case <-box:
			default:
			}
		}
	}
}

func main() {
	box := make(chan int, 1)
	for v := 1; v <= 5; v++ {
		publish(box, v) // no reader yet, and still no blocking
	}
	fmt.Println("reader finally looks and gets:", <-box)

	// A fast sensor and a slow display.
	temps := make(chan int, 1)
	shown := make(chan []int)
	go func() {
		var seen []int
		for t := range temps {
			seen = append(seen, t)
			time.Sleep(25 * time.Millisecond) // slow to redraw
		}
		shown <- seen
	}()
	for t := 20; t <= 30; t++ {
		publish(temps, t) // a reading every 5ms
		time.Sleep(5 * time.Millisecond)
	}
	close(temps)
	seen := <-shown
	fmt.Println("display skipped stale readings:", len(seen) < 11)
	fmt.Println("display ended on the latest reading:", seen[len(seen)-1])
}
```

**Output:**

```
reader finally looks and gets: 5
display skipped stale readings: true
display ended on the latest reading: 30
```

Compare the three ways to handle a full channel: **block** (backpressure, example 58), **drop the new value** (example 14), or **drop the old value** (this example). With several publishers, the take-then-retry loop can still throw away a value that's newer than the one being sent. If that matters, publish through one goroutine.

---

## 65. or-done: make any range stop when done closes

`🟡 medium` · *Cancellation*

`for v := range c` is the cleanest way to consume a channel, but it can't also watch `done`. If `c` belongs to someone else and may never close, you end up writing a nested `select` at every call site. **or-done** writes that `select` once. It wraps `c` in a channel that closes when *either* `c` closes *or* `done` does, so every consumer can go back to a plain `range`.

**Steps:**

1. `sensor` is an endless stream we don't control.
2. `orDone` selects on `done` both while receiving from `c` **and** while forwarding to `out`. Without the second `select`, it could block on the send forever.
3. `main` ranges, closes `done` at reading 3, and drains `readings` to show the forwarder shut down.

```go
package main

import "fmt"

// sensor stands for a channel you don't control: it never closes.
func sensor() <-chan int {
	c := make(chan int)
	go func() {
		for i := 1; ; i++ {
			c <- i
		}
	}()
	return c
}

// orDone forwards values from c until c closes or done closes.
func orDone(done <-chan struct{}, c <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case v, ok := <-c:
				if !ok {
					return
				}
				select { // the forward must be cancellable too
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

func main() {
	done := make(chan struct{})
	readings := orDone(done, sensor())

	for v := range readings { // a plain range, yet it respects done
		fmt.Println("reading", v)
		if v == 3 {
			close(done)
			break
		}
	}

	for range readings { // drain: ends because orDone closes its output
	}
	fmt.Println("forwarder exited and closed its output")
}
```

**Output:**

```
reading 1
reading 2
reading 3
forwarder exited and closed its output
```

or-done can only stop **your** side. The `sensor` goroutine itself is still running, because only its owner can stop it. That's why APIs that return channels should accept a `done` (or a `context`, lesson 15). The pattern comes from *Concurrency in Go* (Cox-Buday), along with `tee` and `bridge` (examples 66 and 72).

---

## 66. tee: one stream in, two identical streams out

`🟡 medium` · *Patterns*

A channel gives each value to **one** receiver (example 33 had to send to each subscriber separately). **tee** splits a stream so two consumers each get every value, like the Unix `tee` command. The neat part is how it waits for both deliveries: a two-case `select`, where each case is switched off by setting its channel to `nil` once it has delivered.

**Steps:**

1. For each value, copy `out1` and `out2` into locals `o1` and `o2`.
2. Loop twice. Whichever output is ready gets `v`, and its local becomes `nil`, so the second pass can only choose the other one.
3. Consumer A sums the stream while consumer B logs it. Both see all four values, in order.

```go
package main

import (
	"fmt"
	"strings"
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

// tee copies every value from in to both outputs.
func tee(done <-chan struct{}, in <-chan int) (<-chan int, <-chan int) {
	out1 := make(chan int)
	out2 := make(chan int)
	go func() {
		defer close(out1)
		defer close(out2)
		for v := range in {
			o1, o2 := out1, out2 // local copies we can switch off
			for range 2 {        // deliver v to both, whichever is ready first
				select {
				case o1 <- v:
					o1 = nil // delivered: disable this case for v
				case o2 <- v:
					o2 = nil
				case <-done:
					return
				}
			}
		}
	}()
	return out1, out2
}

func main() {
	done := make(chan struct{})
	defer close(done)
	toSum, toLog := tee(done, gen(1, 2, 3, 4))

	var wg sync.WaitGroup
	sum := 0
	var log []string
	wg.Add(2)
	go func() {
		defer wg.Done()
		for v := range toSum {
			sum += v
		}
	}()
	go func() {
		defer wg.Done()
		for v := range toLog {
			log = append(log, fmt.Sprint(v))
		}
	}()
	wg.Wait()

	fmt.Println("consumer A, sum:", sum)
	fmt.Println("consumer B, log:", strings.Join(log, ","))
}
```

**Output:**

```
consumer A, sum: 10
consumer B, log: 1,2,3,4
```

The trade-off: `tee` sends one value at a time, so the **slower consumer sets the pace for both**. If one side is much slower, give that output a buffer, or accept dropping for that consumer (examples 14 and 64).

---

## 67. Errors as values: result{val, err} in the stream

`🟡 medium` · *Errors*

A goroutine can't `return` an error to whoever consumes its channel. The usual beginner fix is a second `errs` channel, which then has to be selected on, drained and closed in the right order, and it loses the link between each error and its position in the stream. Simpler and more idiomatic: send a `result` struct that carries **either** a value **or** an error, on the one channel.

**Steps:**

1. `parse` sends `result{n, err}` for every line, wrapping the error with the offending line.
2. `sum` stops at the first error and returns it. `defer close(done)` makes the parser goroutine exit instead of blocking forever on its next send.
3. `errors.Is` still sees the original `strconv.ErrSyntax` through the wrapping (lesson 12).

```go
package main

import (
	"errors"
	"fmt"
	"strconv"
)

type result struct {
	n   int
	err error
}

// parse sends each line's value or error down the same stream.
func parse(done <-chan struct{}, lines []string) <-chan result {
	out := make(chan result)
	go func() {
		defer close(out)
		for _, line := range lines {
			n, err := strconv.Atoi(line)
			if err != nil {
				err = fmt.Errorf("line %q: %w", line, err)
			}
			select {
			case out <- result{n, err}:
			case <-done:
				return
			}
		}
	}()
	return out
}

func sum(lines []string) (int, error) {
	done := make(chan struct{})
	defer close(done) // an early return stops the parser instead of leaking it

	total := 0
	for r := range parse(done, lines) {
		if r.err != nil {
			return total, r.err
		}
		total += r.n
	}
	return total, nil
}

func main() {
	fmt.Println(sum([]string{"1", "2", "3"}))

	total, err := sum([]string{"10", "x7", "20"})
	fmt.Println(total, err)
	fmt.Println("is a syntax error:", errors.Is(err, strconv.ErrSyntax))
}
```

**Output:**

```
6 <nil>
10 line "x7": strconv.Atoi: parsing "x7": invalid syntax
is a syntax error: true
```

The consumer decides the policy: stop at the first error (as here), skip and count failures, or collect them all. The producer just reports. With several parallel workers, "the first error cancels everyone" is exactly what `errgroup` provides (lesson 15).

---

## 68. Wait for all, or for the first success, over a slice of futures

`🟡 medium` · *Futures*

Example 21 made one future, a `<-chan T` that will hold one result. Real code starts several and then needs a combinator. **all** waits for every result, like JavaScript's `Promise.all`. **firstOK** takes the first *success* and fails only if every call fails, like `Promise.any`. Both are a few lines once each future is a 1-slot buffered channel.

**Steps:**

1. `start` returns a future. The buffer of 1 lets the call finish even if nobody ever reads it.
2. `all` reads the futures **in order**. That keeps the result order and doesn't make the total wait any longer than the slowest call.
3. `firstOK` fans the futures into one channel with room for all of them, returns the first success, and joins the errors if none succeeds.

```go
package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const ms = time.Millisecond

type result struct {
	name string
	val  int
	err  error
}

// start runs a fake call and returns a future: a channel that will hold one result.
func start(name string, delay time.Duration, val int, err error) <-chan result {
	f := make(chan result, 1) // buffered: the call can finish even if nobody reads
	go func() {
		time.Sleep(delay)
		f <- result{name, val, err}
	}()
	return f
}

// all waits for every future and keeps the order they were given in.
func all(futures []<-chan result) []result {
	out := make([]result, len(futures))
	for i, f := range futures {
		out[i] = <-f // reading in order is fine: each future keeps its value
	}
	return out
}

// firstOK returns the first success, or every error if all of them fail.
func firstOK(futures []<-chan result) (result, error) {
	merged := make(chan result, len(futures)) // room for all: no forwarder leaks
	for _, f := range futures {
		go func() { merged <- <-f }()
	}
	var errs []error
	for range futures {
		r := <-merged
		if r.err == nil {
			return r, nil
		}
		errs = append(errs, r.err)
	}
	return result{}, errors.Join(errs...)
}

func main() {
	for _, r := range all([]<-chan result{
		start("slow", 30*ms, 1, nil),
		start("fast", 10*ms, 2, nil),
		start("mid", 20*ms, 3, nil),
	}) {
		fmt.Println("all:    ", r.name, r.val)
	}

	r, err := firstOK([]<-chan result{
		start("fails fast", 5*ms, 0, errors.New("boom")),
		start("ok, slower", 25*ms, 42, nil),
		start("ok, slowest", 50*ms, 7, nil),
	})
	fmt.Println("firstOK:", r.name, r.val, err)

	_, err = firstOK([]<-chan result{
		start("a", 5*ms, 0, errors.New("a failed")),
		start("b", 30*ms, 0, errors.New("b failed")),
	})
	fmt.Println("firstOK, all failed:", strings.ReplaceAll(err.Error(), "\n", "; "))
}
```

**Output:**

```
all:     slow 1
all:     fast 2
all:     mid 3
firstOK: ok, slower 42 <nil>
firstOK, all failed: a failed; b failed
```

Notice "fails fast" doesn't win: `firstOK` skips errors and waits for "ok, slower". The losers (and "ok, slowest") keep running to completion, which is fine for cheap calls. For expensive ones, pass a `done` channel so the losers can stop early (example 27 is the simpler "first answer wins, errors included").

---

## 69. Channel generator vs iter.Seq: when a channel is the wrong tool

`🟡 medium` · *Design*

Before Go 1.23, "a function you can `range` over" usually meant returning a channel filled by a goroutine. That has two costs. Every value is a goroutine handoff, so it's slow. And a consumer that `break`s early leaves the producer blocked forever, so it leaks unless you add `done` (example 38). Since Go 1.23, `for v := range f` also works on **iterator functions** (`iter.Seq[T]`). They run in the caller's goroutine and stop when the loop breaks.

**Steps:**

1. The same counter is written both ways.
2. `break` after three values: the channel version leaks its goroutine, and the iterator stops cleanly.
3. Sum one million values each way: the iterator is far faster.

```go
package main

import (
	"fmt"
	"iter"
	"runtime"
	"time"
)

// countChan: a generator built on a channel needs its own goroutine.
func countChan(n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := range n {
			out <- i
		}
	}()
	return out
}

// countSeq: an iterator is a plain function call, no goroutine at all.
func countSeq(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return // the loop body did break: stop producing
			}
		}
	}
}

func main() {
	before := runtime.NumGoroutine()
	for v := range countChan(1_000) {
		if v == 2 {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	fmt.Println("channel generator: goroutines leaked by break:", runtime.NumGoroutine()-before)

	before = runtime.NumGoroutine()
	for v := range countSeq(1_000) {
		if v == 2 {
			break
		}
	}
	fmt.Println("iter.Seq:          goroutines leaked by break:", runtime.NumGoroutine()-before)

	start := time.Now()
	sumChan := 0
	for v := range countChan(1_000_000) {
		sumChan += v
	}
	chanTime := time.Since(start)

	start = time.Now()
	sumSeq := 0
	for v := range countSeq(1_000_000) {
		sumSeq += v
	}
	seqTime := time.Since(start)

	fmt.Println("same sum over 1,000,000 values:", sumChan == sumSeq)
	fmt.Println("iterator more than 10x faster:", seqTime*10 < chanTime)
}
```

**Output:**

```
channel generator: goroutines leaked by break: 1
iter.Seq:          goroutines leaked by break: 0
same sum over 1,000,000 values: true
iterator more than 10x faster: true
```

The rule: if nothing needs to happen **concurrently**, and you just want a sequence the caller pulls, write an iterator. Use a channel when the producer genuinely runs alongside the consumer: I/O, background work, fan-in from several goroutines, or anything that crosses goroutines. (`iter.Seq[int]` is a generic type; lesson 17 covers generics properly.)

---

## 70. Batcher: flush when the batch is full or has waited long enough

`🔴 hard` · *Patterns*

Batching trades a little latency for a lot of throughput: one database insert for 100 rows, one network write for 100 events. Size alone isn't enough, though. Under light traffic a half-full batch would wait forever. So a batch is flushed when it's **full**, when its **oldest item has waited `maxWait`**, or when the **input closes**. It's a popular interview question because it combines `select`, a timer, nil channels and slice ownership.

**Steps:**

1. The timer starts with the **first** value of a batch, not on every value.
2. `timeout` is `nil` while the buffer is empty, so the timer case is switched off.
3. After `out <- buf`, the slice belongs to the receiver, and the batcher starts a new one with `buf = nil`.
4. The input is a burst of 7, a pause, then 2 more, which shows all three flush reasons.

```go
package main

import (
	"fmt"
	"time"
)

type Batch struct {
	Items  []int
	Reason string
}

// batch groups values into slices of up to size. A partial batch is flushed
// once its first value has waited maxWait; the rest is flushed when in closes.
func batch(in <-chan int, size int, maxWait time.Duration) <-chan Batch {
	out := make(chan Batch)
	go func() {
		defer close(out)
		var buf []int
		timer := time.NewTimer(maxWait)
		timer.Stop()
		var timeout <-chan time.Time // nil while buf is empty: case switched off

		flush := func(reason string) {
			if len(buf) > 0 {
				out <- Batch{buf, reason} // ownership of buf moves to the receiver...
				buf = nil                 // ...so start a fresh slice
			}
			timer.Stop()
			timeout = nil
		}

		for {
			select {
			case v, ok := <-in:
				if !ok {
					flush("input closed")
					return
				}
				if len(buf) == 0 {
					timer.Reset(maxWait) // the clock starts with the first value
					timeout = timer.C
				}
				buf = append(buf, v)
				if len(buf) == size {
					flush("full")
				}
			case <-timeout:
				flush("max wait")
			}
		}
	}()
	return out
}

func main() {
	in := make(chan int)
	go func() {
		defer close(in)
		for i := 1; i <= 7; i++ {
			in <- i // a burst
		}
		time.Sleep(60 * time.Millisecond) // a quiet period
		in <- 8
		in <- 9
	}()

	for b := range batch(in, 3, 20*time.Millisecond) {
		fmt.Printf("%-14s %v\n", b.Reason+":", b.Items)
	}
}
```

**Output:**

```
full:          [1 2 3]
full:          [4 5 6]
max wait:      [7]
input closed:  [8 9]
```

Two details matter. Reusing one timer with `Stop`/`Reset` is safe without draining since Go 1.23 (example 57). And setting `buf = nil` instead of `buf = buf[:0]` is essential: reusing the backing array would overwrite a batch the receiver is still reading. That's the aliasing bug from example 43.

---

## 71. Debounce: fire only once the input goes quiet

`🔴 hard` · *Patterns*

A search box shouldn't send a request for every keystroke. **Debounce** waits until the input has been quiet for `d`, then acts on the **latest** value only. Every new value restarts the countdown. It's the timer counterpart of the batcher: one `select` over the input and a single reused timer.

**Steps:**

1. Each value is stored as `latest`, and `timer.Reset(d)` restarts the quiet period.
2. When the timer fires with something pending, emit it once.
3. On close, flush whatever is still pending. Here the user types "gor", pauses, then types "goroutine", so exactly two searches go out.

```go
package main

import (
	"fmt"
	"time"
)

// debounce emits the latest value from in only after in has been quiet for d.
// Whatever is still pending when in closes is emitted too.
func debounce(in <-chan string, d time.Duration) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		var latest string
		pending := false
		timer := time.NewTimer(d)
		timer.Stop()
		for {
			select {
			case v, ok := <-in:
				if !ok {
					if pending {
						out <- latest
					}
					return
				}
				latest, pending = v, true
				timer.Reset(d) // every new value restarts the quiet period
			case <-timer.C:
				if pending {
					out <- latest
					pending = false
				}
			}
		}
	}()
	return out
}

func main() {
	keys := make(chan string)
	go func() {
		defer close(keys)
		for _, text := range []string{"g", "go", "gor"} {
			keys <- text
			time.Sleep(5 * time.Millisecond) // typing fast
		}
		time.Sleep(60 * time.Millisecond) // the user pauses
		for _, text := range []string{"gorou", "goroutine"} {
			keys <- text
			time.Sleep(5 * time.Millisecond)
		}
	}()

	for query := range debounce(keys, 25*time.Millisecond) {
		fmt.Println("search for:", query)
	}
}
```

**Output:**

```
search for: gor
search for: goroutine
```

Its sibling is **throttle**: act at most once per interval, however much input arrives. That's the ticker-based limiter from example 34, or `x/time/rate` from lesson 68. Debounce suits "the user stopped typing"; throttle suits "don't call this API more than 10 times a second".

---

## 72. bridge: flatten a channel of channels

`🔴 hard` · *Patterns*

Sometimes a producer hands out a **sequence of streams**: one channel per page of an API, per file, or per reconnect of a subscription. The consumer wants one flat stream. **bridge** takes `<-chan <-chan T`, drains each inner channel in turn, and forwards everything to a single output. Order is kept, and cancellation is handled in one place.

**Steps:**

1. `pages` yields 3 channels of 4 values each. Note the type syntax: `make(chan (<-chan int))` needs the parentheses.
2. `bridge` takes the next stream (or stops on `done` or when the outer channel closes), then forwards that stream's values.
3. The consumer sees a single stream of 1..12.

```go
package main

import "fmt"

// pages simulates a paginated API: every page arrives as its own channel.
func pages(n, perPage int) <-chan <-chan int {
	chans := make(chan (<-chan int))
	go func() {
		defer close(chans)
		for p := range n {
			page := make(chan int, perPage)
			for i := range perPage {
				page <- p*perPage + i + 1
			}
			close(page)
			chans <- page
		}
	}()
	return chans
}

// bridge flattens a channel of channels into one stream, keeping the order.
func bridge(done <-chan struct{}, chans <-chan <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			var stream <-chan int
			select {
			case s, ok := <-chans:
				if !ok {
					return // no more streams
				}
				stream = s
			case <-done:
				return
			}
			for v := range stream { // drain this stream before taking the next
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()
	return out
}

func main() {
	done := make(chan struct{})
	defer close(done)

	var got []int
	for v := range bridge(done, pages(3, 4)) {
		got = append(got, v)
	}
	fmt.Println("one flat stream:", got)
}
```

**Output:**

```
one flat stream: [1 2 3 4 5 6 7 8 9 10 11 12]
```

The inner `for v := range stream` still can't notice `done` if an inner stream stalls without closing. Production code wraps it with `orDone(done, stream)` from example 65. bridge doesn't merge streams concurrently; for that, see fan-in (example 16). It joins them **one after another**, like `cat`.

---

## 73. Ordered results from a queue of per-job result channels

`🔴 hard` · *Worker pool*

Example 39 restored the order by writing into `ordered[idx]`, but it has to collect **everything** before printing anything. To **stream** results in input order, use a *channel of channels*. For each job, create a 1-slot result channel and push it into a queue **before** starting the work. A single reader takes the result channels from the queue in order and waits on each one in turn. Jobs still run in parallel; only the output is held back until it's that job's turn.

**Steps:**

1. The dispatcher makes `res` for each input, enqueues it (which reserves the job's place), and starts the job.
2. The queue's buffer (`workers` = 3) limits how many jobs can run ahead of the reader. That bounds both concurrency and memory.
3. The reader does `out <- <-res` for each queued channel, in order.

```go
package main

import (
	"fmt"
	"slices"
	"time"
)

// orderedMap runs f on up to `workers` inputs at once and emits results in input order.
func orderedMap(inputs []int, workers int, f func(int) int) <-chan int {
	// One 1-slot result channel per job. The queue holds those channels in
	// input order, and its buffer bounds how far ahead of the reader we get.
	queue := make(chan chan int, workers)
	go func() {
		defer close(queue)
		for _, in := range inputs {
			res := make(chan int, 1)
			queue <- res // reserve this job's place in line
			go func() { res <- f(in) }()
		}
	}()

	out := make(chan int)
	go func() {
		defer close(out)
		for res := range queue {
			out <- <-res // wait for THIS job, whichever job finished first
		}
	}()
	return out
}

func main() {
	finished := make(chan int, 8)
	square := func(n int) int {
		time.Sleep(time.Duration(9-n) * 5 * time.Millisecond) // later inputs are faster
		finished <- n
		return n * n
	}

	var results []int
	for r := range orderedMap([]int{1, 2, 3, 4, 5, 6, 7, 8}, 3, square) {
		results = append(results, r)
	}
	close(finished)
	var order []int
	for n := range finished {
		order = append(order, n)
	}

	fmt.Println("results, in input order:", results)
	fmt.Println("jobs finished in a different order:", !slices.IsSorted(order))
}
```

**Output:**

```
results, in input order: [1 4 9 16 25 36 49 64]
jobs finished in a different order: true
```

The price is head-of-line blocking: a slow first job holds back fast results behind it, but never more than the buffer allows. It's the same trade-off as HTTP/1.1 pipelining. It's a good follow-up answer to "now keep the order, and don't buffer the whole input".

---

## 74. An unbounded queue between two channels, and why you rarely want one

`🔴 hard` · *Patterns*

Go has no "infinite buffer" channel, but you can build one: a goroutine sits between an `in` and an `out` channel and keeps a growing slice. It's a classic interview exercise because the core trick is the **nil-channel send**. The `out <- next` case must be switched off while there's nothing to send, and setting `sendTo` to `nil` does exactly that.

**Steps:**

1. Each pass: if the backlog is non-empty, arm the send case (`sendTo = out`). Otherwise `sendTo` stays `nil` and the case never fires.
2. Receiving appends to the backlog. Sending removes the head.
3. When `in` closes, stop receiving (`src = nil`) but keep delivering until the backlog is empty, then close `out`.

```go
package main

import "fmt"

// unbounded connects two channels through a growing slice: senders never wait
// for the reader, and the reader gets everything in order.
func unbounded() (chan<- int, <-chan int) {
	in := make(chan int)
	out := make(chan int)
	go func() {
		defer close(out)
		src := in // set to nil once in closes
		var backlog []int
		for src != nil || len(backlog) > 0 {
			var sendTo chan int // nil (case disabled) while there is nothing to send
			var next int
			if len(backlog) > 0 {
				sendTo, next = out, backlog[0]
			}
			select {
			case v, ok := <-src:
				if !ok {
					src = nil // keep delivering the backlog
					continue
				}
				backlog = append(backlog, v)
			case sendTo <- next:
				backlog = backlog[1:]
			}
		}
	}()
	return in, out
}

func main() {
	in, out := unbounded()
	for i := 1; i <= 1000; i++ {
		in <- i // nobody is reading out yet, and it never stalls
	}
	close(in)
	fmt.Println("1000 sends done with no reader")

	count, sum, first := 0, 0, 0
	for v := range out {
		if count == 0 {
			first = v
		}
		count++
		sum += v
	}
	fmt.Println("received:", count, "values, first:", first, "sum:", sum)
}
```

**Output:**

```
1000 sends done with no reader
received: 1000 values, first: 1 sum: 500500
```

**Why you rarely want it:** the unbounded queue removes backpressure (example 58). A producer that's faster than its consumer no longer slows down. It fills memory until the process is killed, sometimes hours later in production. Prefer a bounded channel and decide explicitly what happens when it's full: block, drop, or reject. Where it does fit: a consumer that must never block its producer, such as the event loop that feeds itself work (example 54), with a separate limit on the total work.

---

## 75. Closing with many senders: the three Go 101 cases

`🔴 hard` · *Closing*

"Who closes the channel?" has one principle: **never close a channel from the receiving side, and never close a channel that has several senders.** Go 101's article "How to Gracefully Close Channels" turns it into three concrete cases. This is one of the most-asked channel design questions in interviews:

| Senders × receivers | Who stops things | How |
|---|---|---|
| 1 × N | the sender | `close(data)`: it alone knows no more sends will happen |
| N × 1 | the receiver | close a separate `stop` channel. `data` is **never** closed. |
| M × N | anyone | send a request to a **moderator**, which closes `stop` exactly once |

**Steps:**

1. `oneSender`: the ordinary case.
2. `oneReceiver`: senders `select` on `data <- v` and `<-stop`. The receiver closes `stop` when it has had enough.
3. `manyToMany`: senders and receivers call `requestStop`, a non-blocking send into a 1-slot channel. The moderator goroutine receives the first request and closes `stop`.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// 1 sender, N receivers: the sender knows when it is done, so it closes.
func oneSender() {
	data := make(chan int)
	var wg sync.WaitGroup
	var got atomic.Int32
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range data {
				got.Add(1)
			}
		}()
	}
	for i := range 30 {
		data <- i
	}
	close(data) // safe: nobody else sends
	wg.Wait()
	fmt.Println("1 sender, 3 receivers: sender closed data; received", got.Load())
}

// N senders, 1 receiver: the receiver closes a separate stop channel.
// data itself is never closed, just garbage collected.
func oneReceiver() {
	data := make(chan int)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for s := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case data <- s*1000 + i:
				case <-stop:
					return
				}
			}
		}()
	}
	received := 0
	for range data {
		if received++; received == 100 {
			close(stop)
			break
		}
	}
	wg.Wait()
	fmt.Println("3 senders, 1 receiver: receiver took", received, "then closed stop; all senders exited")
}

type msg struct{ sender, seq int }

// M senders, N receivers: anyone may ask to stop; one moderator closes stop.
func manyToMany() {
	data := make(chan msg)
	stop := make(chan struct{})
	toStop := make(chan string, 1) // buffer 1: the first request never blocks
	var stoppedBy string
	go func() { // the moderator is the only goroutine that closes stop
		stoppedBy = <-toStop
		close(stop)
	}()
	requestStop := func(who string) {
		select {
		case toStop <- who:
		default: // someone already asked
		}
	}

	var wg sync.WaitGroup
	for s := range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				if i == 1_000_000 { // a sender may also decide to stop everyone
					requestStop("a sender")
					return
				}
				select {
				case data <- msg{s, i}:
				case <-stop:
					return
				}
			}
		}()
	}
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case m := <-data:
					if m.seq == 5 { // a receiver decides everyone should stop
						requestStop("a receiver")
						return
					}
				case <-stop:
					return
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("3 senders, 2 receivers: stop requested by %s; all 5 exited, data never closed\n", stoppedBy)
}

func main() {
	oneSender()
	oneReceiver()
	manyToMany()
}
```

**Output:**

```
1 sender, 3 receivers: sender closed data; received 30
3 senders, 1 receiver: receiver took 100 then closed stop; all senders exited
3 senders, 2 receivers: stop requested by a receiver; all 5 exited, data never closed
```

A channel that's never closed isn't a leak. Once no goroutine references it, it's garbage collected like any other value. `close` is a **message** ("no more values"), not cleanup, so skip it when nobody needs that message. `sync.Once` around `close(stop)` is a common alternative to the moderator goroutine.

---

## 76. Fan-in over any number of channels with reflect.Select

`🔴 hard` · *select*

A `select` statement has a fixed number of cases, written in the source. When the number of channels is only known at run time, the usual answer is one goroutine per input (fan-in, example 16). The other answer is `reflect.Select`: build a `[]reflect.SelectCase`, and it behaves like a `select` with that many cases, in a single goroutine.

**Steps:**

1. Build one `SelectRecv` case per channel.
2. `reflect.Select` returns the chosen index, the value, and `ok`.
3. On `!ok` (that channel closed), replace its `Chan` with the zero `reflect.Value`. The case is then ignored, just like setting a channel to `nil` in a normal `select`.

```go
package main

import (
	"fmt"
	"reflect"
	"slices"
)

func gen(prefix string, n int) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		for i := 1; i <= n; i++ {
			out <- fmt.Sprint(prefix, i)
		}
	}()
	return out
}

// mergeAll receives from any number of channels in ONE goroutine.
func mergeAll(chans []<-chan string) []string {
	cases := make([]reflect.SelectCase, len(chans))
	for i, ch := range chans {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}
	}

	var got []string
	for open := len(cases); open > 0; {
		i, v, ok := reflect.Select(cases) // like a select with len(cases) cases
		if !ok {
			cases[i].Chan = reflect.Value{} // zero Value: case ignored, like a nil channel
			open--
			continue
		}
		got = append(got, v.String())
	}
	return got
}

func main() {
	// The number of inputs is only known at run time.
	var chans []<-chan string
	for i, name := range []string{"a", "b", "c", "d"} {
		chans = append(chans, gen(name, i+1))
	}

	got := mergeAll(chans)
	slices.Sort(got)
	fmt.Println(len(chans), "channels merged,", len(got), "values:", got)
}
```

**Output:**

```
4 channels merged, 10 values: [a1 b1 b2 c1 c2 c3 d1 d2 d3 d4]
```

Reach for this rarely. Reflection allocates and is several times slower per operation than a real `select`, and the code is harder to read. One goroutine per input is simpler and fast enough for almost everything. `reflect.Select` earns its place when the set of channels changes often (subscribers joining and leaving), or when you'd otherwise start a very large number of forwarding goroutines. Its behavior matches `select`: it picks randomly among ready cases, and supports `SelectSend` and `SelectDefault` too.

---

## 77. Supervisor: restart a worker that dies, up to a limit

`🔴 hard` · *Lifecycle*

A long-running worker can die: a panic on a bad input, or an error it can't recover from. A **supervisor** watches for that and starts a replacement. The worker reports **how** it ended on an `exits` channel: `nil` for a clean finish, an error for a crash. The supervisor restarts it on errors and gives up after a limit, so a job that always crashes can't cause an endless restart loop.

**Steps:**

1. `worker` turns a panic into an error with `recover`, and always sends exactly one value on `exits`.
2. `supervise` waits on `exits`. On an error it starts worker `id+1` on the **same** `jobs` channel, so the new worker continues where the old one stopped.
3. Run it twice: with 2 restarts all work finishes; with 1 the limit is hit.

```go
package main

import (
	"errors"
	"fmt"
)

// worker processes jobs until it hits one it can't handle, and reports how it ended.
func worker(id int, jobs <-chan int, exits chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			exits <- fmt.Errorf("worker %d died: %v", id, r)
		}
	}()
	for j := range jobs {
		if j%4 == 0 {
			panic(fmt.Sprint("cannot handle job ", j))
		}
		fmt.Printf("  worker %d did job %d\n", id, j)
	}
	exits <- nil // clean exit: no more jobs
}

// supervise keeps one worker alive, restarting it at most maxRestarts times.
func supervise(jobs <-chan int, maxRestarts int) error {
	exits := make(chan error)
	id := 1
	go worker(id, jobs, exits)
	for restarts := 0; ; restarts++ {
		err := <-exits
		if err == nil {
			return nil
		}
		fmt.Println("  supervisor:", err)
		if restarts == maxRestarts {
			return errors.New("restart limit reached")
		}
		id++
		go worker(id, jobs, exits)
	}
}

func jobs(n int) <-chan int {
	ch := make(chan int, n)
	for i := 1; i <= n; i++ {
		ch <- i
	}
	close(ch)
	return ch
}

func main() {
	fmt.Println("maxRestarts = 2")
	fmt.Println("result:", supervise(jobs(10), 2))
	fmt.Println("maxRestarts = 1")
	fmt.Println("result:", supervise(jobs(10), 1))
}
```

**Output:**

```
maxRestarts = 2
  worker 1 did job 1
  worker 1 did job 2
  worker 1 did job 3
  supervisor: worker 1 died: cannot handle job 4
  worker 2 did job 5
  worker 2 did job 6
  worker 2 did job 7
  supervisor: worker 2 died: cannot handle job 8
  worker 3 did job 9
  worker 3 did job 10
result: <nil>
maxRestarts = 1
  worker 1 did job 1
  worker 1 did job 2
  worker 1 did job 3
  supervisor: worker 1 died: cannot handle job 4
  worker 2 did job 5
  worker 2 did job 6
  worker 2 did job 7
  supervisor: worker 2 died: cannot handle job 8
result: restart limit reached
```

Jobs 4 and 8 are **lost**: the worker died holding them. Real supervisors send the bad job to a "dead letter" queue before restarting, and usually add a backoff between restarts. This is the Erlang/OTP idea in miniature. Note that `recover` only works inside the goroutine that panicked, so every goroutine you start needs its own `defer`/`recover` if a panic must not kill the process.

---

> ← Back to the [index](README.md) · Prev: [🧪 edge cases](4-edge-cases.md) · Next part: [🎯 interview quiz](6-interview-quiz.md)
