# Step 14 — Channels · Examples

A library of **40 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run (the concurrent ones also under `-race`) before being added — the **Output** under each one is real stdout.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–10 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 11–28 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 29–40 |

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

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
