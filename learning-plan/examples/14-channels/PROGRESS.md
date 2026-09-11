# Step 14 — Channels · Progress

Type & run each example; tick once your output matches. Examples are split by tier and part:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md) · [🧪 edge cases](4-edge-cases.md) ·
[🧩 patterns](5-patterns.md) · [🎯 interview quiz](6-interview-quiz.md) · [🛠️ interview tasks](7-interview-tasks.md).

> ▶ **Resume here:** 🔴 **hard** tier — start with example **40. Capstone: bounded fetch with timeout & ordered map**. 1–39 ticked.


### 🟢 easy — [1-easy.md](1-easy.md)
- [x] 1. Unbuffered send and receive (rendezvous)
- [x] 2. Buffered channel holds values
- [x] 3. A channel of any type
- [x] 4. Producer closes, consumer ranges
- [x] 5. Comma-ok receive detects a closed channel
- [x] 6. Receiving from a closed channel returns zero
- [x] 7. Channel directions in signatures
- [x] 8. Signal completion with a done channel
- [x] 9. Sum values received from a channel
- [x] 10. len and cap of a buffered channel

### 🟡 medium — [2-medium.md](2-medium.md)
- [x] 11. select waits on whichever is ready
- [x] 12. Timeout with select and time.After
- [x] 13. Non-blocking receive with default
- [x] 14. Non-blocking send with default
- [x] 15. Closing broadcasts to all receivers
- [x] 16. Fan-in: merge many channels
- [x] 17. A two-stage pipeline
- [x] 18. Worker pool with jobs and results
- [x] 19. Buffered channel as a semaphore
- [x] 20. Quit channel in a select loop
- [x] 21. Return a result via a channel (future)
- [x] 22. Collect a fixed number of results
- [x] 23. Drain two producers (nil disables a case)
- [x] 24. Bound the whole wait with a timeout
- [x] 25. Do work on a ticker interval
- [x] 26. Range over a multi-producer channel
- [x] 27. First response wins
- [x] 28. Drain a buffer with select/default

### 🔴 hard — [3-hard.md](3-hard.md)
- [x] 29. Bounded parallelism with a semaphore
- [x] 30. Fan-out then fan-in
- [x] 31. Pipeline with cancellation
- [x] 32. or-channel: combine done signals
- [x] 33. Pub/sub broadcast to subscribers
- [x] 34. Rate limiting with a ticker
- [x] 35. Graceful shutdown: drain then stop
- [x] 36. Three-stage composable pipeline
- [x] 37. Disable a select case with a nil channel
- [x] 38. Bounded take from an infinite generator
- [x] 39. Worker pool with ordered results
- [ ] 40. Capstone: bounded fetch with timeout & ordered map

### 🧪 edge cases — [4-edge-cases.md](4-edge-cases.md)
- [ ] 41. Every operation on a nil, open and closed channel
- [ ] 42. Forgot make: range over a nil channel deadlocks
- [ ] 43. Sending copies the value, but a slice still shares memory
- [ ] 44. chan struct{} vs chan bool for signals
- [ ] 45. Channel types: directions, comparison and map keys
- [ ] 46. select with default won't stop a send on a closed channel
- [ ] 47. Closing a channel panics the senders already waiting on it
- [ ] 48. break inside select only exits the select
- [ ] 49. Every case expression is evaluated, even in cases that don't run
- [ ] 50. select picks at random: a closed done doesn't stop a ready send
- [ ] 51. Two ways a for-select burns 100% CPU
- [ ] 52. A buffered send isn't a delivery
- [ ] 53. len(ch) is stale as soon as you read it
- [ ] 54. A worker that sends into its own full queue deadlocks
- [ ] 55. The runtime only reports a deadlock when every goroutine is stuck
- [ ] 56. signal.Notify drops signals sent to an unbuffered channel
- [ ] 57. time.Timer Stop and Reset since Go 1.23

### 🧩 more patterns — [5-patterns.md](5-patterns.md)
- [ ] 58. Backpressure you can watch: the producer stops at cap
- [ ] 59. Stop and wait: quit in, stopped out, Stop() safe twice
- [ ] 60. Request–reply: put a reply channel inside the request
- [ ] 61. A mutex built from a channel: Lock, TryLock, LockTimeout
- [ ] 62. A blocking resource pool: a buffered channel of connections
- [ ] 63. Priority select: always empty high before touching low
- [ ] 64. Keep only the latest value: replace instead of blocking
- [ ] 65. or-done: make any range stop when done closes
- [ ] 66. tee: one stream in, two identical streams out
- [ ] 67. Errors as values: result{val, err} in the stream
- [ ] 68. Wait for all, or for the first success, over a slice of futures
- [ ] 69. Channel generator vs iter.Seq: when a channel is the wrong tool
- [ ] 70. Batcher: flush when the batch is full or has waited long enough
- [ ] 71. Debounce: fire only once the input goes quiet
- [ ] 72. bridge: flatten a channel of channels
- [ ] 73. Ordered results from a queue of per-job result channels
- [ ] 74. An unbounded queue between two channels, and why you rarely want one
- [ ] 75. Closing with many senders: the three Go 101 cases
- [ ] 76. Fan-in over any number of channels with reflect.Select
- [ ] 77. Supervisor: restart a worker that dies, up to a limit

### 🎯 interview quiz — [6-interview-quiz.md](6-interview-quiz.md)
- [ ] 78. A single send in main with no receiver
- [ ] 79. range over a channel nobody closes
- [ ] 80. Close, then range over a buffered channel
- [ ] 81. wg.Wait() before reading an unbuffered results channel
- [ ] 82. Workers range over jobs, but nobody closes jobs
- [ ] 83. Every worker runs defer close(results)
- [ ] 84. The receiver closes the channel: who panics?
- [ ] 85. Two goroutines, two channels, each waiting on the other
- [ ] 86. Unbuffered handoff: which print orders are guaranteed?
- [ ] 87. The non-blocking send that always drops its message
- [ ] 88. The timeout that leaks its worker goroutine
- [ ] 89. time.After inside a for-select: the timeout never fires
- [ ] 90. recover() catches a closed-channel panic but not a deadlock
- [ ] 91. A pipeline stage returns early and the stages before it leak
- [ ] 92. ticker.Stop() doesn't close ticker.C, so for range never ends
- [ ] 93. Find all four bugs in one program, then fix them

### 🛠️ interview tasks — [7-interview-tasks.md](7-interview-tasks.md)
- [ ] 94. Ping-pong, N times
- [ ] 95. Two goroutines print 1..N in order (odd and even)
- [ ] 96. N goroutines take turns in a ring: cat dog fish × 3
- [ ] 97. Print in Order (LeetCode 1114)
- [ ] 98. Print Zero Even Odd (LeetCode 1116)
- [ ] 99. Fizz Buzz Multithreaded (LeetCode 1195)
- [ ] 100. Building H2O (LeetCode 1117)
- [ ] 101. Concurrent prime sieve (a chain of filter goroutines)
- [ ] 102. Equivalent Binary Trees (Tour of Go)
- [ ] 103. Merge two sorted channels into one sorted stream
- [ ] 104. Sleeping Barber: the waiting room is a buffered channel
- [ ] 105. A WaitGroup built only from channels
- [ ] 106. Map-reduce word count
- [ ] 107. A bounded blocking queue whose Close never makes callers panic
- [ ] 108. Scatter-gather: one required call plus optional calls
- [ ] 109. Web Crawler (Tour of Go): parallel, no duplicate visits, knows when it's done
- [ ] 110. Dining Philosophers (LeetCode 1226) without deadlock
- [ ] 111. TTL cache with no mutex: one goroutine owns the data
- [ ] 112. Capstone: ordered parallel pipeline that stops on the first error

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
