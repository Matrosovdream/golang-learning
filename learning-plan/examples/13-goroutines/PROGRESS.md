# Step 13 — Goroutines · Progress

Type & run each example; tick once your output matches. Examples are split by tier:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. Start a goroutine and wait for it**. None ticked yet.


### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. Start a goroutine and wait for it
- [ ] 2. Why main must wait
- [ ] 3. Many goroutines, results in an indexed slice
- [ ] 4. Loop-variable capture (Go 1.22+)
- [ ] 5. Anonymous goroutine with an argument

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 6. Pass *sync.WaitGroup to a helper
- [ ] 7. Collect results over a channel, then sort
- [ ] 8. Atomic counter
- [ ] 9. Mutex-protected counter
- [ ] 10. sync.Once runs initialization exactly once
- [ ] 11. Concurrent map over a slice
- [ ] 12. Add before go, defer Done inside
- [ ] 13. Concurrency vs parallelism (GOMAXPROCS)
- [ ] 14. Fan-in: merge results from many goroutines
- [ ] 15. RWMutex: many readers, one writer
- [ ] 16. sync.Map for concurrent access
- [ ] 17. atomic.Value for a shared snapshot
- [ ] 18. Build a map concurrently under a Mutex
- [ ] 19. Lazy singleton with sync.Once
- [ ] 20. A two-stage pipeline
- [ ] 21. Producer / consumer with close
- [ ] 22. wg.Go (Go 1.25+)
- [ ] 23. Non-blocking receive with select/default
- [ ] 24. Return a value from a goroutine
- [ ] 25. defer LIFO inside a goroutine
- [ ] 26. Count completed tasks atomically

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 27. Worker pool
- [ ] 28. Give a goroutine a guaranteed exit (avoid leaks)
- [ ] 29. Bounded concurrency with a semaphore channel
- [ ] 30. Collect errors from goroutines
- [ ] 31. Parallel partial sums
- [ ] 32. Per-goroutine result structs, sorted
- [ ] 33. Closing a channel broadcasts to all receivers
- [ ] 34. Race-free shared counter (test with -race)
- [ ] 35. Nested goroutines
- [ ] 36. Lock-free max with CompareAndSwap
- [ ] 37. Fan-out then fan-in
- [ ] 38. Worker pool with a results map
- [ ] 39. Parallel 'any match' with an atomic flag
- [ ] 40. A two-phase barrier
- [ ] 41. Cancel many workers by closing a channel
- [ ] 42. Share memory by communicating

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
