# Step 14 — Channels · Examples

A library of **112 runnable examples** in seven files: the core split by difficulty (1–40), then edge cases,
more patterns, and two interview parts (41–112). Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run (the concurrent ones also under `-race`) before being added — the **Output** under each one is real stdout.
Some edge-case and quiz examples fail **on purpose** (a deadlock, a panic, a compile error in 45, a `go vet`
warning in 56, a `-race` report in 47); their output is the real message, trimmed. The two interview parts hide
answers and solutions in collapsible blocks, so try first.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–10 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 11–28 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 29–40 |
| 🧪 Edge cases | [4-edge-cases.md](4-edge-cases.md) | 41–57 |
| 🧩 More patterns | [5-patterns.md](5-patterns.md) | 58–77 |
| 🎯 Interview: predict & fix | [6-interview-quiz.md](6-interview-quiz.md) | 78–93 |
| 🛠️ Interview: implement it | [7-interview-tasks.md](7-interview-tasks.md) | 94–112 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Unbuffered send and receive (rendezvous)](1-easy.md#1-unbuffered-send-and-receive-rendezvous)
- [2. Buffered channel holds values](1-easy.md#2-buffered-channel-holds-values)
- [3. A channel of any type](1-easy.md#3-a-channel-of-any-type)
- [4. Producer closes, consumer ranges](1-easy.md#4-producer-closes-consumer-ranges)
- [5. Comma-ok receive detects a closed channel](1-easy.md#5-comma-ok-receive-detects-a-closed-channel)
- [6. Receiving from a closed channel returns zero](1-easy.md#6-receiving-from-a-closed-channel-returns-zero)
- [7. Channel directions in signatures](1-easy.md#7-channel-directions-in-signatures)
- [8. Signal completion with a done channel](1-easy.md#8-signal-completion-with-a-done-channel)
- [9. Sum values received from a channel](1-easy.md#9-sum-values-received-from-a-channel)
- [10. len and cap of a buffered channel](1-easy.md#10-len-and-cap-of-a-buffered-channel)

### 🟡 [Medium](2-medium.md)

- [11. select waits on whichever is ready](2-medium.md#11-select-waits-on-whichever-is-ready)
- [12. Timeout with select and time.After](2-medium.md#12-timeout-with-select-and-timeafter)
- [13. Non-blocking receive with default](2-medium.md#13-non-blocking-receive-with-default)
- [14. Non-blocking send with default](2-medium.md#14-non-blocking-send-with-default)
- [15. Closing broadcasts to all receivers](2-medium.md#15-closing-broadcasts-to-all-receivers)
- [16. Fan-in: merge many channels](2-medium.md#16-fan-in-merge-many-channels)
- [17. A two-stage pipeline](2-medium.md#17-a-two-stage-pipeline)
- [18. Worker pool with jobs and results](2-medium.md#18-worker-pool-with-jobs-and-results)
- [19. Buffered channel as a semaphore](2-medium.md#19-buffered-channel-as-a-semaphore)
- [20. Quit channel in a select loop](2-medium.md#20-quit-channel-in-a-select-loop)
- [21. Return a result via a channel (future)](2-medium.md#21-return-a-result-via-a-channel-future)
- [22. Collect a fixed number of results](2-medium.md#22-collect-a-fixed-number-of-results)
- [23. Drain two producers (nil disables a case)](2-medium.md#23-drain-two-producers-nil-disables-a-case)
- [24. Bound the whole wait with a timeout](2-medium.md#24-bound-the-whole-wait-with-a-timeout)
- [25. Do work on a ticker interval](2-medium.md#25-do-work-on-a-ticker-interval)
- [26. Range over a multi-producer channel](2-medium.md#26-range-over-a-multi-producer-channel)
- [27. First response wins](2-medium.md#27-first-response-wins)
- [28. Drain a buffer with select/default](2-medium.md#28-drain-a-buffer-with-selectdefault)

### 🔴 [Hard](3-hard.md)

- [29. Bounded parallelism with a semaphore](3-hard.md#29-bounded-parallelism-with-a-semaphore)
- [30. Fan-out then fan-in](3-hard.md#30-fan-out-then-fan-in)
- [31. Pipeline with cancellation](3-hard.md#31-pipeline-with-cancellation)
- [32. or-channel: combine done signals](3-hard.md#32-or-channel-combine-done-signals)
- [33. Pub/sub broadcast to subscribers](3-hard.md#33-pubsub-broadcast-to-subscribers)
- [34. Rate limiting with a ticker](3-hard.md#34-rate-limiting-with-a-ticker)
- [35. Graceful shutdown: drain then stop](3-hard.md#35-graceful-shutdown-drain-then-stop)
- [36. Three-stage composable pipeline](3-hard.md#36-three-stage-composable-pipeline)
- [37. Disable a select case with a nil channel](3-hard.md#37-disable-a-select-case-with-a-nil-channel)
- [38. Bounded take from an infinite generator](3-hard.md#38-bounded-take-from-an-infinite-generator)
- [39. Worker pool with ordered results](3-hard.md#39-worker-pool-with-ordered-results)
- [40. Capstone: bounded fetch with timeout & ordered map](3-hard.md#40-capstone-bounded-fetch-with-timeout--ordered-map)

### 🧪 [Edge cases](4-edge-cases.md)

- [41. Every operation on a nil, open and closed channel](4-edge-cases.md#41-every-operation-on-a-nil-open-and-closed-channel)
- [42. Forgot make: range over a nil channel deadlocks](4-edge-cases.md#42-forgot-make-range-over-a-nil-channel-deadlocks)
- [43. Sending copies the value, but a slice still shares memory](4-edge-cases.md#43-sending-copies-the-value-but-a-slice-still-shares-memory)
- [44. chan struct{} vs chan bool for signals](4-edge-cases.md#44-chan-struct-vs-chan-bool-for-signals)
- [45. Channel types: directions, comparison and map keys](4-edge-cases.md#45-channel-types-directions-comparison-and-map-keys)
- [46. select with default won't stop a send on a closed channel](4-edge-cases.md#46-select-with-default-wont-stop-a-send-on-a-closed-channel)
- [47. Closing a channel panics the senders already waiting on it](4-edge-cases.md#47-closing-a-channel-panics-the-senders-already-waiting-on-it)
- [48. break inside select only exits the select](4-edge-cases.md#48-break-inside-select-only-exits-the-select)
- [49. Every case expression is evaluated, even in cases that don't run](4-edge-cases.md#49-every-case-expression-is-evaluated-even-in-cases-that-dont-run)
- [50. select picks at random: a closed done doesn't stop a ready send](4-edge-cases.md#50-select-picks-at-random-a-closed-done-doesnt-stop-a-ready-send)
- [51. Two ways a for-select burns 100% CPU](4-edge-cases.md#51-two-ways-a-for-select-burns-100-cpu)
- [52. A buffered send isn't a delivery](4-edge-cases.md#52-a-buffered-send-isnt-a-delivery)
- [53. len(ch) is stale as soon as you read it](4-edge-cases.md#53-lench-is-stale-as-soon-as-you-read-it)
- [54. A worker that sends into its own full queue deadlocks](4-edge-cases.md#54-a-worker-that-sends-into-its-own-full-queue-deadlocks)
- [55. The runtime only reports a deadlock when every goroutine is stuck](4-edge-cases.md#55-the-runtime-only-reports-a-deadlock-when-every-goroutine-is-stuck)
- [56. signal.Notify drops signals sent to an unbuffered channel](4-edge-cases.md#56-signalnotify-drops-signals-sent-to-an-unbuffered-channel)
- [57. time.Timer Stop and Reset since Go 1.23](4-edge-cases.md#57-timetimer-stop-and-reset-since-go-123)

### 🧩 [More patterns](5-patterns.md)

- [58. Backpressure you can watch: the producer stops at cap](5-patterns.md#58-backpressure-you-can-watch-the-producer-stops-at-cap)
- [59. Stop and wait: quit in, stopped out, Stop() safe twice](5-patterns.md#59-stop-and-wait-quit-in-stopped-out-stop-safe-twice)
- [60. Request–reply: put a reply channel inside the request](5-patterns.md#60-requestreply-put-a-reply-channel-inside-the-request)
- [61. A mutex built from a channel: Lock, TryLock, LockTimeout](5-patterns.md#61-a-mutex-built-from-a-channel-lock-trylock-locktimeout)
- [62. A blocking resource pool: a buffered channel of connections](5-patterns.md#62-a-blocking-resource-pool-a-buffered-channel-of-connections)
- [63. Priority select: always empty high before touching low](5-patterns.md#63-priority-select-always-empty-high-before-touching-low)
- [64. Keep only the latest value: replace instead of blocking](5-patterns.md#64-keep-only-the-latest-value-replace-instead-of-blocking)
- [65. or-done: make any range stop when done closes](5-patterns.md#65-or-done-make-any-range-stop-when-done-closes)
- [66. tee: one stream in, two identical streams out](5-patterns.md#66-tee-one-stream-in-two-identical-streams-out)
- [67. Errors as values: result{val, err} in the stream](5-patterns.md#67-errors-as-values-resultval-err-in-the-stream)
- [68. Wait for all, or for the first success, over a slice of futures](5-patterns.md#68-wait-for-all-or-for-the-first-success-over-a-slice-of-futures)
- [69. Channel generator vs iter.Seq: when a channel is the wrong tool](5-patterns.md#69-channel-generator-vs-iterseq-when-a-channel-is-the-wrong-tool)
- [70. Batcher: flush when the batch is full or has waited long enough](5-patterns.md#70-batcher-flush-when-the-batch-is-full-or-has-waited-long-enough)
- [71. Debounce: fire only once the input goes quiet](5-patterns.md#71-debounce-fire-only-once-the-input-goes-quiet)
- [72. bridge: flatten a channel of channels](5-patterns.md#72-bridge-flatten-a-channel-of-channels)
- [73. Ordered results from a queue of per-job result channels](5-patterns.md#73-ordered-results-from-a-queue-of-per-job-result-channels)
- [74. An unbounded queue between two channels, and why you rarely want one](5-patterns.md#74-an-unbounded-queue-between-two-channels-and-why-you-rarely-want-one)
- [75. Closing with many senders: the three Go 101 cases](5-patterns.md#75-closing-with-many-senders-the-three-go-101-cases)
- [76. Fan-in over any number of channels with reflect.Select](5-patterns.md#76-fan-in-over-any-number-of-channels-with-reflectselect)
- [77. Supervisor: restart a worker that dies, up to a limit](5-patterns.md#77-supervisor-restart-a-worker-that-dies-up-to-a-limit)

### 🎯 [Interview: predict & fix](6-interview-quiz.md)

- [78. A single send in main with no receiver](6-interview-quiz.md#78-a-single-send-in-main-with-no-receiver)
- [79. range over a channel nobody closes](6-interview-quiz.md#79-range-over-a-channel-nobody-closes)
- [80. Close, then range over a buffered channel](6-interview-quiz.md#80-close-then-range-over-a-buffered-channel)
- [81. wg.Wait() before reading an unbuffered results channel](6-interview-quiz.md#81-wgwait-before-reading-an-unbuffered-results-channel)
- [82. Workers range over jobs, but nobody closes jobs](6-interview-quiz.md#82-workers-range-over-jobs-but-nobody-closes-jobs)
- [83. Every worker runs defer close(results)](6-interview-quiz.md#83-every-worker-runs-defer-closeresults)
- [84. The receiver closes the channel: who panics?](6-interview-quiz.md#84-the-receiver-closes-the-channel-who-panics)
- [85. Two goroutines, two channels, each waiting on the other](6-interview-quiz.md#85-two-goroutines-two-channels-each-waiting-on-the-other)
- [86. Unbuffered handoff: which print orders are guaranteed?](6-interview-quiz.md#86-unbuffered-handoff-which-print-orders-are-guaranteed)
- [87. The non-blocking send that always drops its message](6-interview-quiz.md#87-the-non-blocking-send-that-always-drops-its-message)
- [88. The timeout that leaks its worker goroutine](6-interview-quiz.md#88-the-timeout-that-leaks-its-worker-goroutine)
- [89. time.After inside a for-select: the timeout never fires](6-interview-quiz.md#89-timeafter-inside-a-for-select-the-timeout-never-fires)
- [90. recover() catches a closed-channel panic but not a deadlock](6-interview-quiz.md#90-recover-catches-a-closed-channel-panic-but-not-a-deadlock)
- [91. A pipeline stage returns early and the stages before it leak](6-interview-quiz.md#91-a-pipeline-stage-returns-early-and-the-stages-before-it-leak)
- [92. ticker.Stop() doesn't close ticker.C, so for range never ends](6-interview-quiz.md#92-tickerstop-doesnt-close-tickerc-so-for-range-never-ends)
- [93. Find all four bugs in one program, then fix them](6-interview-quiz.md#93-find-all-four-bugs-in-one-program-then-fix-them)

### 🛠️ [Interview: implement it](7-interview-tasks.md)

- [94. Ping-pong, N times](7-interview-tasks.md#94-ping-pong-n-times)
- [95. Two goroutines print 1..N in order (odd and even)](7-interview-tasks.md#95-two-goroutines-print-1n-in-order-odd-and-even)
- [96. N goroutines take turns in a ring: cat dog fish × 3](7-interview-tasks.md#96-n-goroutines-take-turns-in-a-ring-cat-dog-fish--3)
- [97. Print in Order (LeetCode 1114)](7-interview-tasks.md#97-print-in-order-leetcode-1114)
- [98. Print Zero Even Odd (LeetCode 1116)](7-interview-tasks.md#98-print-zero-even-odd-leetcode-1116)
- [99. Fizz Buzz Multithreaded (LeetCode 1195)](7-interview-tasks.md#99-fizz-buzz-multithreaded-leetcode-1195)
- [100. Building H2O (LeetCode 1117)](7-interview-tasks.md#100-building-h2o-leetcode-1117)
- [101. Concurrent prime sieve (a chain of filter goroutines)](7-interview-tasks.md#101-concurrent-prime-sieve-a-chain-of-filter-goroutines)
- [102. Equivalent Binary Trees (Tour of Go)](7-interview-tasks.md#102-equivalent-binary-trees-tour-of-go)
- [103. Merge two sorted channels into one sorted stream](7-interview-tasks.md#103-merge-two-sorted-channels-into-one-sorted-stream)
- [104. Sleeping Barber: the waiting room is a buffered channel](7-interview-tasks.md#104-sleeping-barber-the-waiting-room-is-a-buffered-channel)
- [105. A WaitGroup built only from channels](7-interview-tasks.md#105-a-waitgroup-built-only-from-channels)
- [106. Map-reduce word count](7-interview-tasks.md#106-map-reduce-word-count)
- [107. A bounded blocking queue whose Close never makes callers panic](7-interview-tasks.md#107-a-bounded-blocking-queue-whose-close-never-makes-callers-panic)
- [108. Scatter-gather: one required call plus optional calls](7-interview-tasks.md#108-scatter-gather-one-required-call-plus-optional-calls)
- [109. Web Crawler (Tour of Go): parallel, no duplicate visits, knows when it's done](7-interview-tasks.md#109-web-crawler-tour-of-go-parallel-no-duplicate-visits-knows-when-its-done)
- [110. Dining Philosophers (LeetCode 1226) without deadlock](7-interview-tasks.md#110-dining-philosophers-leetcode-1226-without-deadlock)
- [111. TTL cache with no mutex: one goroutine owns the data](7-interview-tasks.md#111-ttl-cache-with-no-mutex-one-goroutine-owns-the-data)
- [112. Capstone: ordered parallel pipeline that stops on the first error](7-interview-tasks.md#112-capstone-ordered-parallel-pipeline-that-stops-on-the-first-error)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
