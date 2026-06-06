# Step 13 — Goroutines · Examples

A library of **42 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–26 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 27–42 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Start a goroutine and wait for it](1-easy.md#1-start-a-goroutine-and-wait-for-it)
- [2. Why main must wait](1-easy.md#2-why-main-must-wait)
- [3. Many goroutines, results in an indexed slice](1-easy.md#3-many-goroutines-results-in-an-indexed-slice)
- [4. Loop-variable capture (Go 1.22+)](1-easy.md#4-loop-variable-capture-go-122)
- [5. Anonymous goroutine with an argument](1-easy.md#5-anonymous-goroutine-with-an-argument)

### 🟡 [Medium](2-medium.md)

- [6. Pass *sync.WaitGroup to a helper](2-medium.md#6-pass-syncwaitgroup-to-a-helper)
- [7. Collect results over a channel, then sort](2-medium.md#7-collect-results-over-a-channel-then-sort)
- [8. Atomic counter](2-medium.md#8-atomic-counter)
- [9. Mutex-protected counter](2-medium.md#9-mutex-protected-counter)
- [10. sync.Once runs initialization exactly once](2-medium.md#10-synconce-runs-initialization-exactly-once)
- [11. Concurrent map over a slice](2-medium.md#11-concurrent-map-over-a-slice)
- [12. Add before go, defer Done inside](2-medium.md#12-add-before-go-defer-done-inside)
- [13. Concurrency vs parallelism (GOMAXPROCS)](2-medium.md#13-concurrency-vs-parallelism-gomaxprocs)
- [14. Fan-in: merge results from many goroutines](2-medium.md#14-fan-in-merge-results-from-many-goroutines)
- [15. RWMutex: many readers, one writer](2-medium.md#15-rwmutex-many-readers-one-writer)
- [16. sync.Map for concurrent access](2-medium.md#16-syncmap-for-concurrent-access)
- [17. atomic.Value for a shared snapshot](2-medium.md#17-atomicvalue-for-a-shared-snapshot)
- [18. Build a map concurrently under a Mutex](2-medium.md#18-build-a-map-concurrently-under-a-mutex)
- [19. Lazy singleton with sync.Once](2-medium.md#19-lazy-singleton-with-synconce)
- [20. A two-stage pipeline](2-medium.md#20-a-two-stage-pipeline)
- [21. Producer / consumer with close](2-medium.md#21-producer--consumer-with-close)
- [22. wg.Go (Go 1.25+)](2-medium.md#22-wggo-go-125)
- [23. Non-blocking receive with select/default](2-medium.md#23-non-blocking-receive-with-selectdefault)
- [24. Return a value from a goroutine](2-medium.md#24-return-a-value-from-a-goroutine)
- [25. defer LIFO inside a goroutine](2-medium.md#25-defer-lifo-inside-a-goroutine)
- [26. Count completed tasks atomically](2-medium.md#26-count-completed-tasks-atomically)

### 🔴 [Hard](3-hard.md)

- [27. Worker pool](3-hard.md#27-worker-pool)
- [28. Give a goroutine a guaranteed exit (avoid leaks)](3-hard.md#28-give-a-goroutine-a-guaranteed-exit-avoid-leaks)
- [29. Bounded concurrency with a semaphore channel](3-hard.md#29-bounded-concurrency-with-a-semaphore-channel)
- [30. Collect errors from goroutines](3-hard.md#30-collect-errors-from-goroutines)
- [31. Parallel partial sums](3-hard.md#31-parallel-partial-sums)
- [32. Per-goroutine result structs, sorted](3-hard.md#32-per-goroutine-result-structs-sorted)
- [33. Closing a channel broadcasts to all receivers](3-hard.md#33-closing-a-channel-broadcasts-to-all-receivers)
- [34. Race-free shared counter (test with -race)](3-hard.md#34-race-free-shared-counter-test-with--race)
- [35. Nested goroutines](3-hard.md#35-nested-goroutines)
- [36. Lock-free max with CompareAndSwap](3-hard.md#36-lock-free-max-with-compareandswap)
- [37. Fan-out then fan-in](3-hard.md#37-fan-out-then-fan-in)
- [38. Worker pool with a results map](3-hard.md#38-worker-pool-with-a-results-map)
- [39. Parallel 'any match' with an atomic flag](3-hard.md#39-parallel-any-match-with-an-atomic-flag)
- [40. A two-phase barrier](3-hard.md#40-a-two-phase-barrier)
- [41. Cancel many workers by closing a channel](3-hard.md#41-cancel-many-workers-by-closing-a-channel)
- [42. Share memory by communicating](3-hard.md#42-share-memory-by-communicating)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
