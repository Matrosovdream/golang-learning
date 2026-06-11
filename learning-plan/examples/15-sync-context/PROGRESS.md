# Step 15 — Sync, Context & Patterns · Progress

Type & run each example; tick once your output matches. Examples are split by tier:
[🟢 easy](1-easy.md) · [🟡 medium](2-medium.md) · [🔴 hard](3-hard.md).

> ▶ **Resume here:** 🟢 **easy** tier — start with example **1. See a data race with -race**. None ticked yet.


### 🟢 easy — [1-easy.md](1-easy.md)
- [ ] 1. See a data race with -race
- [ ] 2. Mutex 101: Lock blocks until Unlock
- [ ] 3. defer mu.Unlock() survives every return path
- [ ] 4. Never copy a mutex (go vet copylocks)
- [ ] 5. RWMutex: reads overlap, writes exclude
- [ ] 6. sync.OnceFunc and sync.OnceValue (Go 1.21+)
- [ ] 7. ctx goes first: Background and the call convention
- [ ] 8. context.WithCancel stops a goroutine
- [ ] 9. context.WithTimeout: Done fires on its own
- [ ] 10. context.WithValue: request-scoped metadata

### 🟡 medium — [2-medium.md](2-medium.md)
- [ ] 11. Mutex or channel? The same problem, twice
- [ ] 12. SafeCounter: 100 × 100 = 10000
- [ ] 13. Keep the critical section small
- [ ] 14. TryLock: skip, don't wait (Go 1.18+)
- [ ] 15. A read-through cache with RWMutex
- [ ] 16. atomic.Bool as a stop flag
- [ ] 17. atomic.Pointer[T]: lock-free snapshot swap (Go 1.19+)
- [ ] 18. sync.Pool recycles allocations
- [ ] 19. sync.Map: LoadOrStore, and when not to use it
- [ ] 20. WaitGroup.Wait with a timeout
- [ ] 21. The cancellable worker loop
- [ ] 22. Race the work against the deadline
- [ ] 23. WithDeadline, and reading it back
- [ ] 24. Cancellation flows down the context tree
- [ ] 25. WithCancelCause: keep the real reason (Go 1.20+)
- [ ] 26. context.AfterFunc: hook the cancellation (Go 1.21+)
- [ ] 27. Timeouts bubble up a call chain
- [ ] 28. sleepCtx: an interruptible sleep

### 🔴 hard — [3-hard.md](3-hard.md)
- [ ] 29. A worker pool that exits cleanly on cancel
- [ ] 30. Pipeline with context cancellation
- [ ] 31. First error cancels the rest (mini errgroup)
- [ ] 32. sync.Cond: sleep until something changes
- [ ] 33. Graceful shutdown: ctx + WaitGroup + timeout
- [ ] 34. One parent budget, per-task timeouts
- [ ] 35. Get-or-create once: the double-check idiom
- [ ] 36. Token bucket rate limiter with ctx stop
- [ ] 37. Heartbeats: prove the worker is alive
- [ ] 38. Singleflight: collapse duplicate fetches
- [ ] 39. Acquire a semaphore — or give up via ctx
- [ ] 40. Capstone: a tiny job scheduler

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
