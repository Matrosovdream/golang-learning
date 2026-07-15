# Step 48 — Low-Latency Go III: Lock-Free, Zero-Copy & Tail Latency · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[48-low-latency-lockfree-tail.md](../../48-low-latency-lockfree-tail.md): copy-on-write with
`atomic.Pointer`, CAS loops, `unsafe.String`, zero-copy I/O (`io.Copy`, `bufio`, `net.Buffers`),
batching, request coalescing, ring buffers, hedged requests, and a zero-allocation hot handler.

## One-time setup

```bash
mkdir -p /tmp/ll3-ex && cd /tmp/ll3-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the concurrency ones (#1, #2, #8,
#11, #12) were also checked under **`go run -race .`**. Output is **deterministic** — allocation counts
from `testing.AllocsPerRun`, concurrency *correctness* counts, and fixed latency samples all print the
exact values shown. Two lines vary by machine and say so: #4's MiB total and #13's GC-cycle count (the
*direction* is the lesson). Standard-library only — the singleflight in #8 is a minimal hand-rolled
version so there's no dependency. Needs **Go 1.20+** for `unsafe.String` (#3).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — atomics & zero-copy I/O
- [1. Copy-on-write config with `atomic.Pointer`](1-easy.md#1-copy-on-write-config-with-atomicpointer)
- [2. A CAS retry loop (lock-free float add)](1-easy.md#2-a-cas-retry-loop-lock-free-float-add)
- [3. `unsafe.String`: zero-copy `[]byte`→`string`](1-easy.md#3-unsafestring-zero-copy-bytestring)
- [4. Stream with `io.Copy`, don't buffer with `io.ReadAll`](1-easy.md#4-stream-with-iocopy-dont-buffer-with-ioreadall)
- [5. `bufio` batches the syscalls](1-easy.md#5-bufio-batches-the-syscalls)

### 🟡 [Medium](2-medium.md) — amortising: batch, coalesce, reuse
- [6. `net.Buffers`: one vectored write](2-medium.md#6-netbuffers-one-vectored-write)
- [7. Batch to amortise](2-medium.md#7-batch-to-amortise)
- [8. Coalesce duplicate work (mini singleflight)](2-medium.md#8-coalesce-duplicate-work-mini-singleflight)
- [9. A zero-allocation ring buffer](2-medium.md#9-a-zero-allocation-ring-buffer)
- [10. Hedged requests cut the tail](2-medium.md#10-hedged-requests-cut-the-tail)

### 🔴 [Hard](3-hard.md) — lock-free structures & a capstone
- [11. A lock-free stack (Treiber)](3-hard.md#11-a-lock-free-stack-treiber)
- [12. Copy-on-write snapshots never tear](3-hard.md#12-copy-on-write-snapshots-never-tear)
- [13. Zero allocations → zero GC on the hot path](3-hard.md#13-zero-allocations--zero-gc-on-the-hot-path)
- [14. Zero-alloc serialization with a pooled buffer](3-hard.md#14-zero-alloc-serialization-with-a-pooled-buffer)
- [15. Capstone: a zero-allocation hot handler](3-hard.md#15-capstone-a-zero-allocation-hot-handler)
</content>
