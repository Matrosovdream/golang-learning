# Step 47 — Low-Latency Go II: GC, Layout & Contention · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype**. They reinforce
[47-low-latency-gc-contention.md](../../47-low-latency-gc-contention.md): struct padding & the cache,
pointer-free layout, `GOGC`/`GOMEMLIMIT`, `sync.Pool`, atomics/sharding/RWMutex, false sharing, and
reading a `pprof` heap profile.

## One-time setup

```bash
mkdir -p /tmp/ll2-ex && cd /tmp/ll2-ex
go mod init scratch
```

Most examples run with `go run .`. A few are different, and each says so at the top:

- **Deterministic** examples (sizes from `unsafe.Sizeof`, allocation counts from `testing.AllocsPerRun`,
  concurrency *correctness* counts) print the **exact** output shown — same on every machine.
- **#11–13** are `go test -bench` benchmarks: their `ns/op` is **machine-dependent** (marked
  *illustrative*) — only the **ratio** between the variants is the lesson.
- **#5, #6** print GC-cycle counts that **vary** by machine/`GOGC`; the *direction* is the point.
- **#14** writes a heap profile and reads it with `go tool pprof`.

Every example was compiled, `go vet`-ed, and run before being added. Standard-library only.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — memory layout & the GC knobs
- [1. Struct padding: field order changes size](1-easy.md#1-struct-padding-field-order-changes-size)
- [2. Find the padding holes with `Offsetof`](1-easy.md#2-find-the-padding-holes-with-offsetof)
- [3. Field order → cache-line packing](1-easy.md#3-field-order--cache-line-packing)
- [4. Index-based references beat pointers](1-easy.md#4-index-based-references-beat-pointers)
- [5. `GOGC`: pacing the collector](1-easy.md#5-gogc-pacing-the-collector)

### 🟡 [Medium](2-medium.md) — memory limits, pooling, contention
- [6. `GOMEMLIMIT`: a soft cap that triggers GC](2-medium.md#6-gomemlimit-a-soft-cap-that-triggers-gc)
- [7. `sync.Pool`: the reset footgun](2-medium.md#7-syncpool-the-reset-footgun)
- [8. Atomic vs mutex counter](2-medium.md#8-atomic-vs-mutex-counter)
- [9. Sharded (striped) counter](2-medium.md#9-sharded-striped-counter)
- [10. `RWMutex` for read-mostly data](2-medium.md#10-rwmutex-for-read-mostly-data)

### 🔴 [Hard](3-hard.md) — benchmarks, profiling & a capstone
- [11. Array-of-structs vs struct-of-arrays](3-hard.md#11-array-of-structs-vs-struct-of-arrays)
- [12. Contention: mutex vs atomic vs sharded](3-hard.md#12-contention-mutex-vs-atomic-vs-sharded)
- [13. False sharing: pad to a cache line](3-hard.md#13-false-sharing-pad-to-a-cache-line)
- [14. Reading a heap profile: `alloc_space` vs `inuse_space`](3-hard.md#14-reading-a-heap-profile-alloc_space-vs-inuse_space)
- [15. Capstone: a profile-guided fix](3-hard.md#15-capstone-a-profile-guided-fix)
</content>
