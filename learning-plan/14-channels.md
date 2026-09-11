# 14 — Channels

## Goals
- Send and receive values between goroutines with channels.
- Understand buffered vs unbuffered channels and blocking behavior.
- Use `close`, `range`, and channel directions correctly.
- Coordinate multiple channels with `select`.
- Predict tricky channel code: nil and closed channels, what `select` evaluates and picks, deadlocks vs leaks, timers.
- Apply the advanced patterns (or-done, tee, bridge, batching, request–reply) and solve the classic channel interview tasks.

## Mental model: two rooms, a hole in the wall, and boxes

Keep this picture in your head for the whole lesson. A channel is a **hole in the wall between two
rooms**; goroutines are **workers**; values are **boxes**. Everything else follows from how much
shelf space sits in the hole.

### Unbuffered — `make(chan int)` — no shelf

```
      ROOM A                                     ROOM B
  ┌─────────────┐                           ┌─────────────┐
  │  sender     │        cap = 0            │  receiver   │
  │  [box] ─────┼──▶  ( no shelf )  ◀───────┼──── waits   │
  └─────────────┘      hand to hand         └─────────────┘

  The box is NEVER put down. It passes directly from one
  worker's hands to the other's -- so both must be at the
  hole at the same moment.
```

That is literally what the runtime does: with `cap == 0` there is nowhere to store the value, so it
is copied **straight from the sender's stack to the receiver's stack**. `ch <- 1` is not a deposit,
it is a handoff.

Consequences:

- A worker may **stand at the hole and wait**, holding the box, for as long as it takes. Waiting is
  normal, not an error.
- It is only broken when **nobody will ever come**.
- **One worker cannot serve both sides.** He cannot hand a box to himself.

That last point is the classic first deadlock:

```
  ONE ROOM ONLY
  ┌──────────────────────────────────────────────┐
  │  ch := make(chan int)                        │
  │  ch <- 1     ← worker waits at the hole      │
  │  <-ch        ← the only receiver is HIM,     │
  │                one step later -- a step he   │
  │                can never take while waiting  │
  └──────────────────────────────────────────────┘
        fatal error: all goroutines are asleep
```

### Buffered — `make(chan int, 3)` — a shelf with 3 slots

```
      ROOM A              SHELF (cap 3)                ROOM B
  ┌───────────┐      ┌──────┬──────┬──────┐      ┌────────────┐
  │  sender   │ ──▶  │ box1 │ box2 │      │ ──▶  │  receiver  │
  └───────────┘      └──────┴──────┴──────┘      └────────────┘
                       len = 2,  cap = 3
                        FIFO: box1 leaves first
```

The sender **drops the box and walks away**. He never learns whether it was picked up — that is the
guarantee you trade away for the convenience.

### When each worker has to wait

```
  SHELF FULL                             SHELF EMPTY
  ┌──────┬──────┬──────┐                 ┌──────┬──────┬──────┐
  │ box1 │ box2 │ box3 │                 │      │      │      │
  └──────┴──────┴──────┘                 └──────┴──────┴──────┘
   ▲                                                          ▲
   └── sender BLOCKS                       receiver BLOCKS ───┘
       (nowhere to put it)                     (nothing to take)
```

| shelf (`cap`) | sender blocks when | receiver blocks when |
|---|---|---|
| `0` — unbuffered | always, until a receiver arrives | always, until a sender arrives |
| `N` — buffered | the shelf is **full** | the shelf is **empty** |

A full shelf is **backpressure**: it is the only thing telling a fast producer to slow down. Over-size
the shelf and you have removed the brake, not fixed the problem.

### `close(ch)` — a sign on the hole

```
  ┌──────┬──────┬──────┐   ┌───────────────────┐
  │ box1 │ box2 │      │   │  NO MORE BOXES    │  ← close(ch)
  └──────┴──────┴──────┘   └───────────────────┘

  1. Boxes already on the shelf still come out, in order.
  2. After the last one, every receiver gets an EMPTY box
     (the zero value) plus ok == false -- immediately, forever.
  3. Putting a box in after the sign goes up  -> panic
  4. Hanging a second sign                    -> panic
```

So closing is **not** cleanup and **not** required. It is how a receiver learns the stream ended —
which is exactly what `for v := range ch` and `v, ok := <-ch` are asking. A channel nobody needs to
learn that about can simply be left open and garbage collected.

Only the **sender** may hang the sign: a receiver cannot know whether a worker is mid-delivery. With
several senders, none of them can — you need one extra worker whose only job is *"wait for all
senders to finish, then hang the sign"*:

```go
go func() { wg.Wait(); close(ch) }()
```

### `var ch chan int` — no hole at all

A nil channel is a wall with **no hole in it**. Sends and receives block forever and can never be
served. That sounds purely like a bug, and usually is — but it is deliberately useful for switching
off a `select` case once a stream is finished (example 37).

### Queues of waiting workers at each side of the hole

A hole can have **lines of workers** waiting at it: senders on one side, receivers on the other. Each line is first-come, first-served.

```
        ROOM A                                          ROOM B
  ┌────────────────────┐                        ┌────────────────────┐
  │ senders waiting:   │      shelf (cap N)     │ receivers waiting: │
  │   w3   w2   w1 ────┼──▶ [ box | box | box ] ◀┼──── r1   r2        │
  └────────────────────┘                        └────────────────────┘
   a line forms here only                        a line forms here only
   while the shelf is FULL                       while the shelf is EMPTY
```

Two consequences, both used in the examples:

- **Both lines are never non-empty at the same time.** If a receiver is already waiting, an arriving sender hands the box straight to it. The box never touches the shelf, even on a buffered channel.
- **`close` clears both lines at once.** Waiting receivers leave with an empty box (`ok == false`). Waiting senders are still holding boxes, and they **panic** (example 47).

### `select`: one worker watching several holes

```
              ┌── hole A: jobs      can go now? ──┐
   worker ────┼── hole B: quit      can go now? ──┼──▶ pick ONE hole that can go (coin flip if several)
              └── hole C: timer     can go now? ──┘    none can go: wait at all of them at once
                                                        with `default`: don't wait at all
```

- A **nil** channel is a bricked-up hole: that case can never go, so `select` ignores it.
- A **closed** channel can always go: receives return empty boxes immediately, and sends **panic**. `default` doesn't help with either (examples 46 and 51).
- The coin flip is real. Case order means nothing, and priorities need a second `select` (examples 50 and 63).

## Concepts
- **A channel is a typed conduit** for sending values between goroutines:
  ```go
  ch := make(chan int)       // unbuffered
  ch <- 42                   // send (blocks until a receiver is ready)
  v := <-ch                  // receive (blocks until a value arrives)
  ```
- **Unbuffered channels are synchronization points.** A send blocks until another goroutine receives, and vice versa — they "rendezvous." This guarantees the handoff happened, making channels both communication *and* synchronization.
- **Buffered channels** — `make(chan int, 3)` holds up to 3 values without a receiver. Sends block only when the buffer is **full**; receives block only when it's **empty**. Use buffering to decouple producer/consumer speeds, not as a correctness crutch.
- **Closing a channel** — `close(ch)` signals "no more values":
  ```go
  v, ok := <-ch   // ok == false once the channel is closed and drained
  ```
  - **Only the sender should close**, and only once. Sending on a closed channel **panics**; closing a closed channel panics; closing a nil channel panics.
  - Receiving from a closed channel returns the zero value immediately (with `ok == false`).
- **`range` over a channel** — receive until it's closed:
  ```go
  for v := range ch {   // loops until ch is closed and drained
      use(v)
  }
  ```
- **Channel directions** — restrict a channel in function signatures for safety and clarity:
  ```go
  func produce(out chan<- int) { out <- 1 }   // send-only
  func consume(in <-chan int)  { <-in }        // receive-only
  ```
- **`select`** — wait on multiple channel operations; runs whichever is ready first (random choice if several are ready):
  ```go
  select {
  case v := <-ch1:
      use(v)
  case ch2 <- x:
      // sent
  case <-time.After(time.Second):
      // timeout
  default:
      // runs if nothing else is ready (non-blocking select)
  }
  ```
- **Timeouts with `time.After`** — returns a channel that fires after a duration; combine with `select` to bound how long you wait.
- **`done`/quit channels** — a common cancellation idiom (precursor to `context`): close a `done` channel to broadcast "stop" to many goroutines, since every receiver of a closed channel unblocks.
- **Nil channels block forever.** A send/receive on a nil channel never proceeds. This is occasionally used to *disable* a case in a `select` (set the channel to nil).

## Edge cases & deeper semantics

The concepts above are enough to write channel code. This section is what you need to **predict** it: the rules behind the edge-case, quiz and interview parts of the examples library.

### Every operation in every state

| | nil channel | open channel | closed channel |
|---|---|---|---|
| `ch <- v` | blocks **forever** | blocks until room or a receiver | **panic** |
| `<-ch` | blocks **forever** | blocks until a value | buffered values first, then zero value, `ok == false`, instantly |
| `close(ch)` | **panic** | ok | **panic** |
| `len(ch)` / `cap(ch)` | `0` / `0` | queued / buffer size | queued / buffer size |
| in a `select` case | never chosen | chosen when ready | always ready (receive: zero value; send: panic) |

- A `default` case prevents **blocking**, never **panics**: a "non-blocking" send on a closed channel still panics (example 46).
- A `for`-`select` spins at 100% CPU if it has a `default`, or a case on a closed channel. Set finished channels to `nil` (example 51).
- `len(ch)` is stale as soon as you read it. Use it for metrics, never for decisions (example 53).
- A send **copies** the value, but slices, maps and pointers inside it still share memory (example 43).
- Channel directions convert only from bidirectional to directional. `close` on a `<-chan T` is a compile error (example 45).

### `select` in depth

- **Evaluation:** on entry, every case's channel expression and every value to send are evaluated **once, in source order**, even for cases that don't run (example 49). `case out <- compute():` calls `compute()` on every pass.
- **Choice:** among ready cases, one is picked **uniformly at random**. A closed `done` does not beat a ready send (example 50). For priority, check the urgent channel alone first (example 63).
- **`break`** inside a `select` only leaves the `select`. Use a labeled `break` or `return` to leave the loop (example 48).
- **`select {}`** (no cases) blocks forever. So does a `select` whose cases are all `nil` and that has no `default`.

### Closing rules

Close is a **message** ("no more values will ever come"), not cleanup. An unclosed channel nobody references is garbage collected. Who may send that message:

| senders × receivers | who stops the flow | how |
|---|---|---|
| 1 × N | the sender | `close(data)` |
| N × 1 | the receiver | close a separate `stop` channel; `data` is never closed |
| M × N | anyone, through a moderator | ask via a 1-slot `toStop` channel; one goroutine closes `stop` |

Closing from the receiver side, or once per sender, panics some goroutine later: a sender that's already waiting, or one that sends afterwards (examples 47, 83, 84 and 75).

### Leaks vs deadlocks

- The runtime reports `all goroutines are asleep - deadlock!` only when **every** goroutine is blocked. If anything else is alive (`main`, a server, a timer), stuck goroutines are a silent **leak** that's never garbage collected (example 55).
- The deadlock report is a **fatal error**, not a panic. `recover` can't catch it and deferred functions don't run (example 90).
- The usual leak shapes: a result sent on an unbuffered channel after the receiver gave up (example 88), a pipeline stage whose consumer returned early (example 91), and `range` over a channel that's never closed (examples 79 and 92).
- Finding leaks: `runtime.NumGoroutine()` in metrics, the pprof goroutine profile (many goroutines parked on the same line), `go.uber.org/goleak` in tests.

### Timers and channels since Go 1.23

- Timer and ticker channels are **synchronous**. After `Stop` or `Reset`, no stale tick is delivered, so the old `if !t.Stop() { <-t.C }` drain is unnecessary, and it blocks forever if the tick was already received (example 57). This needs `go 1.23` or later in `go.mod`.
- Unreferenced timers and tickers are garbage collected even without `Stop`.
- `time.After` inside a `for`-`select` creates a new timer **every pass**. That's an idle timeout, not a deadline (example 89).
- `ticker.Stop()` does **not** close `ticker.C`, so `for range ticker.C` never ends (example 92).

### Under the hood: `hchan`

`make(chan T, n)` allocates a runtime `hchan`, roughly (simplified from `runtime/chan.go`):

```go
type hchan struct {
	buf      unsafe.Pointer // ring buffer with dataqsiz slots (none when unbuffered)
	dataqsiz uint           // capacity: cap(ch)
	qcount   uint           // values in the buffer: len(ch)
	sendx    uint           // next slot to write
	recvx    uint           // next slot to read
	recvq    waitq          // parked receivers (FIFO)
	sendq    waitq          // parked senders (FIFO), each holding a pointer to its value
	closed   uint32
	lock     mutex          // one lock guards all of the above
}
```

- **Send:** lock. If closed, panic. If a receiver is parked, copy the value **directly** to it and wake it; the buffer is skipped. Otherwise, if there's room, copy into `buf[sendx]`. Otherwise park in `sendq`.
- **Receive:** lock. If a sender is parked: on an unbuffered channel, copy straight from it; on a (full) buffered channel, take the head of the buffer and move the parked sender's value into the tail. Otherwise take `buf[recvx]` if non-empty. If it's closed and empty, return the zero value with `ok == false`. Otherwise park in `recvq`.
- **Close:** lock, set `closed`, wake **every** parked goroutine: receivers return the zero value, senders panic.
- **`select`:** shuffles the order in which it checks cases (that's the randomness), locks all involved channels in a fixed address order (so two `select`s can't deadlock on each other's locks), takes the first ready case, or enqueues itself on **every** channel and parks until one wakes it.
- So channels are **not lock-free**: every operation takes a short mutex. `len(ch)` reads `qcount` without it, which is why it's only a snapshot.

## Exercises
1. Create an unbuffered channel; in a goroutine send a value, in `main` receive it. Note the send waits for the receive.
2. Make a buffered channel of size 2; send 2 values without a receiver (no block), then try a 3rd and observe it blocks.
3. Have a producer goroutine send 5 ints then `close` the channel; consume them in `main` with `for v := range ch`.
4. Use the comma-ok receive (`v, ok := <-ch`) to detect a closed channel explicitly.
5. Use channel directions: write `produce(out chan<- int)` and `consume(in <-chan int)` and wire them together.
6. Use `select` with a worker channel and a `time.After(500ms)` timeout; make the worker sometimes slow to trigger the timeout branch.
7. Implement a `done := make(chan struct{})` quit signal: launch a goroutine that loops in a `select` and exits when `done` is closed.
8. Rebuild the state table yourself: for a nil, an open and a closed channel, try send, receive and close (use `recover`, and `select`/`default` for the blocking cases) and print what happens.
9. Write `queryWithTimeout` with an unbuffered result channel, prove the leak with `runtime.NumGoroutine()`, then fix it with one character.
10. Implement a mutex from a 1-slot channel with `Lock`, `Unlock`, `TryLock` and `LockTimeout`.
11. Write a batcher that flushes when a batch is full **or** its first item has waited 20ms, and flushes the rest when the input closes.
12. Print `1..N` in order from two goroutines (odd and even), and make it work for both odd and even `N`.
13. Take the worker pool from example 18, add a 50ms total budget and "stop at the first error", and prove no goroutine outlives the call.
14. Solve three tasks from the interview part of the examples library (examples 94–112) before opening their solutions.

## Best Practices & Pitfalls
- **The sender closes, never the receiver.** Closing tells receivers "no more data"; a receiver closing would make senders panic.
- **Use channels to transfer ownership of data.** Once you send a value, don't keep mutating it — the receiver now owns it. This is how channels avoid data races.
- **Pitfall — deadlock.** If every goroutine is blocked waiting on a channel and none can proceed, the runtime panics with `all goroutines are asleep - deadlock!`. Usually means a missing receiver, an unclosed channel in a `range`, or a forgotten `wg`/send.
- **Pitfall — sending on a closed channel panics.** Design so the closing goroutine is the only sender, or use a separate `done` channel for shutdown.
- **Pitfall — leaking goroutines blocked on channels.** A goroutine stuck on `<-ch` that never receives leaks. Give it a `select` with a `done`/`ctx.Done()` escape (lesson 15).
- **Don't over-buffer to "fix" blocking.** Buffer sizes should reflect real producer/consumer slack, not paper over a coordination bug.
- **Prefer `range ch` over manual `ok` checks** when consuming until close — it's cleaner.
- **Pitfall — `default` doesn't make a closed channel safe.** A "non-blocking" send on a closed channel still panics. Fix ownership instead of guessing.
- **Pitfall — deciding based on `len(ch)`.** It's stale immediately. Let the receive itself be the check (`select` with `default`).
- **Pitfall — `break` inside `select`** exits only the `select`. Use a labeled `break` or `return` to leave the loop.
- **Pitfall — `time.After` inside a `for`-`select`** is a per-pass idle timeout, not a deadline. Create the timer once, outside the loop.
- **Pitfall — `ticker.Stop()` doesn't close `ticker.C`.** A `for range ticker.C` never ends. Give the loop a `quit` case.
- **Buffer result and reply channels (`make(chan T, 1)`)** whenever the receiver might stop listening: timeouts, first-wins, early returns.
- **When code you don't control sends without blocking** (`signal.Notify`, some event hooks), the buffer is part of the contract. An unbuffered channel silently drops.
- **If a loop sets its channel variables to `nil`, pass the channels to goroutines as arguments.** Closures capture the variable, and a late sender would race with the loop or send on nil.
- **Channels for iteration are usually the wrong tool.** If nothing runs concurrently, write an `iter.Seq` instead: faster, and nothing leaks on `break`.

## Interview question bank

Theory questions that come up in Go interviews. Answer each one out loud before opening it; the example numbers point to runnable proof.

<details>
<summary><b>1. Unbuffered vs buffered: what does each one guarantee?</b></summary>

Unbuffered: a completed send means the receiver **has** the value, because the two goroutines met. Buffered: a completed send only means the value is **queued**. Nobody may ever receive it, for example if `main` exits first (example 52). Neither guarantees the receiver has *processed* it.
</details>

<details>
<summary><b>2. What do send, receive and close do on a nil, an open and a closed channel?</b></summary>

Nil: send and receive block forever, and close panics. Closed: send panics, receive drains the buffer and then returns the zero value with `ok == false` immediately, and close panics. Open: the normal blocking rules. See the table above and example 41.
</details>

<details>
<summary><b>3. How do you check whether a channel is closed?</b></summary>

Only by receiving: `v, ok := <-ch` (which consumes a value if there is one). There's no `isClosed`, and there couldn't be a useful one, since the answer could change right after you read it. Senders should never need to ask. Design ownership so the sender knows (examples 46 and 75).
</details>

<details>
<summary><b>4. Who closes a channel? What if there are several senders?</b></summary>

The sender, and only when it knows no more sends will happen. With several senders, a separate goroutine closes after `wg.Wait()`. If a receiver needs senders to stop, it closes a separate `stop` channel, and `data` is never closed. With many senders and receivers, a moderator closes `stop` once (examples 26, 75 and 83).
</details>

<details>
<summary><b>5. Must every channel be closed?</b></summary>

No. `close` is a message ("no more values"), not resource cleanup. Close only when a receiver needs that message (`range`, or `ok == false`). An unreferenced open channel is garbage collected. Goroutines **blocked** on it are not, though (example 55).
</details>

<details>
<summary><b>6. How does select choose when several cases are ready? How do you give one priority?</b></summary>

Uniformly at random; case order is irrelevant (example 50). For priority, first `select` on the urgent channel alone with a `default`, then `select` on everything (example 63). It's best effort, because values that arrive simultaneously while blocked still compete.
</details>

<details>
<summary><b>7. What does select evaluate before choosing?</b></summary>

Every case's channel expression and every send value, once, in source order, even for cases that won't run (example 49). Only the chosen case's receive assignment and body run afterwards.
</details>

<details>
<summary><b>8. What is a nil channel good for?</b></summary>

Switching off a `select` case: a nil channel is never ready. It's used to stop reading a closed input (examples 23 and 37), to arm a send only when there's something to send (example 74), and to turn off a timer or budget case (examples 70 and 108).
</details>

<details>
<summary><b>9. When is a `default` case a bug?</b></summary>

In a loop that is waiting for something, where it becomes a 100% CPU spin (example 51). On an unbuffered channel, where the send succeeds only if a receiver is parked at that exact instant, so it usually drops (example 87). And as supposed protection against a closed channel, which still panics (example 46).
</details>

<details>
<summary><b>10. How do you implement a timeout, and what's wrong with time.After in a loop?</b></summary>

`select` on the result and `time.After(d)`. Inside a `for`-`select`, `time.After` makes a fresh timer every pass, so it's an idle timeout, not a total deadline. Create the timer once before the loop (example 89). And buffer the result channel, or the worker leaks when the timeout wins (example 88).
</details>

<details>
<summary><b>11. What is a goroutine leak? Give channel-related causes and how to detect them.</b></summary>

A goroutine that can never finish, so its memory is never freed. Causes: a send nobody will receive (a timeout or early return), `range` over a channel nobody closes, a stage whose consumer stopped (examples 88, 91 and 92). Detection: `runtime.NumGoroutine()`, the pprof goroutine profile, and `goleak` in tests.
</details>

<details>
<summary><b>12. When does Go report "all goroutines are asleep - deadlock!"? Can you recover from it?</b></summary>

Only when **every** goroutine is blocked, with nothing else that could wake one. With other goroutines alive, the same bug is a silent leak (example 55). It's a fatal error, not a panic: `recover` can't catch it, and deferred functions don't run (example 90).
</details>

<details>
<summary><b>13. Channel or mutex: how do you choose?</b></summary>

Channels **transfer ownership** and **coordinate**: pipelines, handing off work, signaling, waiting with timeouts. Mutexes **protect shared state**: a cache, a counter, a map read by many. If the code reads like "lock, change a field, unlock", use a mutex. If it reads like "pass this along", use a channel. Lesson 15 goes deeper.
</details>

<details>
<summary><b>14. How is a channel implemented? Is it lock-free?</b></summary>

A runtime `hchan`: a ring buffer, send and receive indexes, FIFO queues of parked senders and receivers, a `closed` flag, and **one mutex**. So it's not lock-free. A send to a parked receiver copies the value directly and skips the buffer. See "Under the hood" above.
</details>

<details>
<summary><b>15. What is len(ch) good for?</b></summary>

Metrics and logging. It's a snapshot that may be wrong by the next line, so "if `len(ch) > 0`, receive" is a race (example 53). Use `select` with `default` for "take one if available". For an unbuffered or timer channel, `len` is always 0.
</details>

<details>
<summary><b>16. Why chan struct{}?</b></summary>

`struct{}{}` is zero bytes, so it costs nothing to send or buffer, and it has no value to misinterpret. It says "this channel is a signal", as in `done`, `quit` and semaphore tokens (example 44).
</details>

<details>
<summary><b>17. How do you limit concurrency with channels?</b></summary>

A buffered `chan struct{}` of capacity N as a semaphore: send to acquire, receive to release (examples 19 and 29). Or a fixed pool of N workers reading one jobs channel (example 18). Or a buffered channel of N reusable resources (example 62).
</details>

<details>
<summary><b>18. What is backpressure, and how does buffer size affect it?</b></summary>

A slow consumer slows the producer down, because sends block once the buffer is full (example 58). A buffer absorbs bursts, but a huge or unbounded one removes the brake and just delays running out of memory (example 74). The choice when full is: block, drop the new value, or drop the old value (examples 14 and 64).
</details>

<details>
<summary><b>19. How do you stop a pipeline early without leaking?</b></summary>

Pass a `done` channel to every stage, `select` every send (and blocking receive) against `<-done`, and `defer close(done)` where the consumer may return early (examples 31, 65 and 112). With `context`, it's `ctx.Done()` (lesson 15).
</details>

<details>
<summary><b>20. How do you broadcast one signal to many goroutines?</b></summary>

`close` a channel they all receive from. Every receiver wakes, including ones that start waiting later. Sending N values only works if you know N, and each value wakes exactly one receiver (examples 15 and 97).
</details>

<details>
<summary><b>21. What happens to goroutines blocked on channels, and to buffered values, when main returns?</b></summary>

The process exits: goroutines are killed mid-flight and buffered values are lost. Nothing is flushed. For graceful shutdown, close the input and **wait** for consumers to finish (example 52).
</details>

<details>
<summary><b>22. With several senders, in what order does a receiver get values?</b></summary>

Each channel is FIFO. Values come out in the order the sends completed, and parked goroutines are served first-come first-served. But how different senders' values interleave depends on scheduling, so don't rely on it. Tag values with an index if you need order (examples 39 and 73).
</details>

<details>
<summary><b>23. What changed for timers in Go 1.23?</b></summary>

Timer and ticker channels became synchronous: no stale tick after `Stop` or `Reset`, `len` and `cap` report 0, and the drain idiom is no longer needed. Unreferenced timers are garbage collected without `Stop`. Old `go.mod` versions keep the old behavior (example 57).
</details>

<details>
<summary><b>24. Why use chan&lt;- T and &lt;-chan T in signatures?</b></summary>

Documentation plus compiler enforcement. A receive-only parameter can't be sent on or **closed**, so "only the sender closes" is checked at compile time (examples 7 and 45).
</details>

<details>
<summary><b>25. Why do you often see make(chan T, 1) for a single result?</b></summary>

So the goroutine producing the result can always finish, even if the caller already gave up (timeout, first-response-wins, early return). With an unbuffered channel, that goroutine would leak (examples 21, 27, 60 and 88).
</details>

## Checklist
- [ ] I can create, send on, and receive from channels.
- [ ] I understand unbuffered (rendezvous) vs buffered blocking behavior.
- [ ] I know only the sender closes, and what receiving from a closed channel returns.
- [ ] I can `range` over a channel and use channel directions in signatures.
- [ ] I can use `select` with a timeout and a default case.
- [ ] I recognize the classic deadlock and closed-channel-panic situations.
- [ ] I can fill in the nil / open / closed × send / receive / close table from memory.
- [ ] I can explain what `select` evaluates, how it picks among ready cases, and how to prioritize one.
- [ ] I know who closes in the 1×N, N×1 and M×N cases, and why a receiver must not close.
- [ ] I can tell a deadlock from a leak, and find a leak with `runtime.NumGoroutine()`.
- [ ] I know the Go 1.23 timer behavior and the `time.After`-in-a-loop trap.
- [ ] I can describe roughly how `hchan` works: ring buffer, wait queues, one lock, direct handoff.
- [ ] I can solve the classic channel interview tasks: alternating printers, bounded queue, batcher, crawler, leak-free pipeline.

## Resources
- A Tour of Go — Channels & select: https://go.dev/tour/concurrency/2
- Effective Go — channels: https://go.dev/doc/effective_go#channels
- Blog — Go concurrency patterns: https://go.dev/blog/pipelines
- Blog — Advanced concurrency patterns: https://go.dev/blog/io2013-talk-concurrency
- Examples: [examples/14-channels](examples/14-channels/): 112 runnable examples in 7 parts (basics, edge cases, patterns, interview quiz, interview tasks).
- Go spec — Channel types & Select statements: https://go.dev/ref/spec#Channel_types · https://go.dev/ref/spec#Select_statements
- Go 101 — How to gracefully close channels: https://go101.org/article/channel-closing.html
- Dave Cheney — Channel axioms: https://dave.cheney.net/2014/03/19/channel-axioms
- Go 1.23 release notes — timer changes: https://go.dev/doc/go1.23#timer-changes
- Runtime source — `chan.go`: https://go.dev/src/runtime/chan.go
- Tour of Go exercises — Equivalent Binary Trees: https://go.dev/tour/concurrency/7 · Web Crawler: https://go.dev/tour/concurrency/10
- Go's concurrent prime sieve: https://go.dev/doc/play/sieve.go
- LeetCode concurrency problems (1114, 1116, 1117, 1195, 1226): https://leetcode.com/problemset/concurrency/
- Book — *Concurrency in Go* (Katherine Cox-Buday), ch. 4: or-done, tee, bridge, pipelines
