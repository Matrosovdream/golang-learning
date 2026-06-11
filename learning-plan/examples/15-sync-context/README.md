# Step 15 — Sync, Context & Patterns · Examples

A library of **40 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # most examples here are concurrent — try go run -race . too
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run — under `-race` too — before
being added; the **Output** under each one is real stdout. (Two deliberate exceptions: example 1
*is* a data race and example 4 *should* fail `go vet` — that's exactly what they teach.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–10 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 11–28 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 29–40 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. See a data race with -race](1-easy.md#1-see-a-data-race-with--race)
- [2. Mutex 101: Lock blocks until Unlock](1-easy.md#2-mutex-101-lock-blocks-until-unlock)
- [3. defer mu.Unlock() survives every return path](1-easy.md#3-defer-muunlock-survives-every-return-path)
- [4. Never copy a mutex (go vet copylocks)](1-easy.md#4-never-copy-a-mutex-go-vet-copylocks)
- [5. RWMutex: reads overlap, writes exclude](1-easy.md#5-rwmutex-reads-overlap-writes-exclude)
- [6. sync.OnceFunc and sync.OnceValue (Go 1.21+)](1-easy.md#6-synconcefunc-and-synconcevalue-go-121)
- [7. ctx goes first: Background and the call convention](1-easy.md#7-ctx-goes-first-background-and-the-call-convention)
- [8. context.WithCancel stops a goroutine](1-easy.md#8-contextwithcancel-stops-a-goroutine)
- [9. context.WithTimeout: Done fires on its own](1-easy.md#9-contextwithtimeout-done-fires-on-its-own)
- [10. context.WithValue: request-scoped metadata](1-easy.md#10-contextwithvalue-request-scoped-metadata)

### 🟡 [Medium](2-medium.md)

- [11. Mutex or channel? The same problem, twice](2-medium.md#11-mutex-or-channel-the-same-problem-twice)
- [12. SafeCounter: 100 × 100 = 10000](2-medium.md#12-safecounter-100--100--10000)
- [13. Keep the critical section small](2-medium.md#13-keep-the-critical-section-small)
- [14. TryLock: skip, don't wait (Go 1.18+)](2-medium.md#14-trylock-skip-dont-wait-go-118)
- [15. A read-through cache with RWMutex](2-medium.md#15-a-read-through-cache-with-rwmutex)
- [16. atomic.Bool as a stop flag](2-medium.md#16-atomicbool-as-a-stop-flag)
- [17. atomic.Pointer[T]: lock-free snapshot swap (Go 1.19+)](2-medium.md#17-atomicpointert-lock-free-snapshot-swap-go-119)
- [18. sync.Pool recycles allocations](2-medium.md#18-syncpool-recycles-allocations)
- [19. sync.Map: LoadOrStore, and when not to use it](2-medium.md#19-syncmap-loadorstore-and-when-not-to-use-it)
- [20. WaitGroup.Wait with a timeout](2-medium.md#20-waitgroupwait-with-a-timeout)
- [21. The cancellable worker loop](2-medium.md#21-the-cancellable-worker-loop)
- [22. Race the work against the deadline](2-medium.md#22-race-the-work-against-the-deadline)
- [23. WithDeadline, and reading it back](2-medium.md#23-withdeadline-and-reading-it-back)
- [24. Cancellation flows down the context tree](2-medium.md#24-cancellation-flows-down-the-context-tree)
- [25. WithCancelCause: keep the real reason (Go 1.20+)](2-medium.md#25-withcancelcause-keep-the-real-reason-go-120)
- [26. context.AfterFunc: hook the cancellation (Go 1.21+)](2-medium.md#26-contextafterfunc-hook-the-cancellation-go-121)
- [27. Timeouts bubble up a call chain](2-medium.md#27-timeouts-bubble-up-a-call-chain)
- [28. sleepCtx: an interruptible sleep](2-medium.md#28-sleepctx-an-interruptible-sleep)

### 🔴 [Hard](3-hard.md)

- [29. A worker pool that exits cleanly on cancel](3-hard.md#29-a-worker-pool-that-exits-cleanly-on-cancel)
- [30. Pipeline with context cancellation](3-hard.md#30-pipeline-with-context-cancellation)
- [31. First error cancels the rest (mini errgroup)](3-hard.md#31-first-error-cancels-the-rest-mini-errgroup)
- [32. sync.Cond: sleep until something changes](3-hard.md#32-synccond-sleep-until-something-changes)
- [33. Graceful shutdown: ctx + WaitGroup + timeout](3-hard.md#33-graceful-shutdown-ctx--waitgroup--timeout)
- [34. One parent budget, per-task timeouts](3-hard.md#34-one-parent-budget-per-task-timeouts)
- [35. Get-or-create once: the double-check idiom](3-hard.md#35-get-or-create-once-the-double-check-idiom)
- [36. Token bucket rate limiter with ctx stop](3-hard.md#36-token-bucket-rate-limiter-with-ctx-stop)
- [37. Heartbeats: prove the worker is alive](3-hard.md#37-heartbeats-prove-the-worker-is-alive)
- [38. Singleflight: collapse duplicate fetches](3-hard.md#38-singleflight-collapse-duplicate-fetches)
- [39. Acquire a semaphore — or give up via ctx](3-hard.md#39-acquire-a-semaphore--or-give-up-via-ctx)
- [40. Capstone: a tiny job scheduler](3-hard.md#40-capstone-a-tiny-job-scheduler)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
