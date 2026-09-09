# 14 — Channels

## Goals
- Send and receive values between goroutines with channels.
- Understand buffered vs unbuffered channels and blocking behavior.
- Use `close`, `range`, and channel directions correctly.
- Coordinate multiple channels with `select`.

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

## Exercises
1. Create an unbuffered channel; in a goroutine send a value, in `main` receive it. Note the send waits for the receive.
2. Make a buffered channel of size 2; send 2 values without a receiver (no block), then try a 3rd and observe it blocks.
3. Have a producer goroutine send 5 ints then `close` the channel; consume them in `main` with `for v := range ch`.
4. Use the comma-ok receive (`v, ok := <-ch`) to detect a closed channel explicitly.
5. Use channel directions: write `produce(out chan<- int)` and `consume(in <-chan int)` and wire them together.
6. Use `select` with a worker channel and a `time.After(500ms)` timeout; make the worker sometimes slow to trigger the timeout branch.
7. Implement a `done := make(chan struct{})` quit signal: launch a goroutine that loops in a `select` and exits when `done` is closed.

## Best Practices & Pitfalls
- **The sender closes, never the receiver.** Closing tells receivers "no more data"; a receiver closing would make senders panic.
- **Use channels to transfer ownership of data.** Once you send a value, don't keep mutating it — the receiver now owns it. This is how channels avoid data races.
- **Pitfall — deadlock.** If every goroutine is blocked waiting on a channel and none can proceed, the runtime panics with `all goroutines are asleep - deadlock!`. Usually means a missing receiver, an unclosed channel in a `range`, or a forgotten `wg`/send.
- **Pitfall — sending on a closed channel panics.** Design so the closing goroutine is the only sender, or use a separate `done` channel for shutdown.
- **Pitfall — leaking goroutines blocked on channels.** A goroutine stuck on `<-ch` that never receives leaks. Give it a `select` with a `done`/`ctx.Done()` escape (lesson 15).
- **Don't over-buffer to "fix" blocking.** Buffer sizes should reflect real producer/consumer slack, not paper over a coordination bug.
- **Prefer `range ch` over manual `ok` checks** when consuming until close — it's cleaner.

## Checklist
- [ ] I can create, send on, and receive from channels.
- [ ] I understand unbuffered (rendezvous) vs buffered blocking behavior.
- [ ] I know only the sender closes, and what receiving from a closed channel returns.
- [ ] I can `range` over a channel and use channel directions in signatures.
- [ ] I can use `select` with a timeout and a default case.
- [ ] I recognize the classic deadlock and closed-channel-panic situations.

## Resources
- A Tour of Go — Channels & select: https://go.dev/tour/concurrency/2
- Effective Go — channels: https://go.dev/doc/effective_go#channels
- Blog — Go concurrency patterns: https://go.dev/blog/pipelines
- Blog — Advanced concurrency patterns: https://go.dev/blog/io2013-talk-concurrency
