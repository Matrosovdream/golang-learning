# Step 13 — Goroutines · Progress

Type & run each example from [README.md](README.md); tick once your output matches.


### 🟢 easy
- [ ] 1. Start a goroutine and wait for it
- [ ] 2. Why main must wait
- [ ] 3. Many goroutines, results in an indexed slice
- [ ] 4. Loop-variable capture (Go 1.22+)
- [ ] 5. Anonymous goroutine with an argument

### 🟡 medium
- [ ] 6. Pass *sync.WaitGroup to a helper
- [ ] 7. Collect results over a channel, then sort
- [ ] 8. Atomic counter
- [ ] 9. Mutex-protected counter
- [ ] 10. sync.Once runs initialization exactly once
- [ ] 11. Concurrent map over a slice
- [ ] 12. Add before go, defer Done inside
- [ ] 13. Concurrency vs parallelism (GOMAXPROCS)
- [ ] 14. Fan-in: merge results from many goroutines
- [ ] 23. RWMutex: many readers, one writer
- [ ] 24. sync.Map for concurrent access
- [ ] 26. atomic.Value for a shared snapshot
- [ ] 28. Build a map concurrently under a Mutex
- [ ] 29. Lazy singleton with sync.Once
- [ ] 30. A two-stage pipeline
- [ ] 31. Producer / consumer with close
- [ ] 32. wg.Go (Go 1.25+)
- [ ] 34. Non-blocking receive with select/default
- [ ] 35. Return a value from a goroutine
- [ ] 36. defer LIFO inside a goroutine
- [ ] 39. Count completed tasks atomically

### 🔴 hard
- [ ] 15. Worker pool
- [ ] 16. Give a goroutine a guaranteed exit (avoid leaks)
- [ ] 17. Bounded concurrency with a semaphore channel
- [ ] 18. Collect errors from goroutines
- [ ] 19. Parallel partial sums
- [ ] 20. Per-goroutine result structs, sorted
- [ ] 21. Closing a channel broadcasts to all receivers
- [ ] 22. Race-free shared counter (test with -race)
- [ ] 25. Nested goroutines
- [ ] 27. Lock-free max with CompareAndSwap
- [ ] 33. Fan-out then fan-in
- [ ] 37. Worker pool with a results map
- [ ] 38. Parallel 'any match' with an atomic flag
- [ ] 40. A two-phase barrier
- [ ] 41. Cancel many workers by closing a channel
- [ ] 42. Share memory by communicating

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
