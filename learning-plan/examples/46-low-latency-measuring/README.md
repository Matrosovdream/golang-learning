# Step 46 — Low-Latency Go I: Measuring & Allocation Basics · Examples

A library of **17 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[46-low-latency-measuring.md](../../46-low-latency-measuring.md): percentiles & the tail, benchmarking,
`testing.AllocsPerRun`, escape analysis, preallocation, `strings.Builder`, `[]byte`/`string` cost,
`strconv.Append*`, and a first look at `sync.Pool`.

## One-time setup

```bash
mkdir -p /tmp/ll1-ex && cd /tmp/ll1-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added. **Allocation counts come from
`testing.AllocsPerRun`, which is deterministic** — the `allocs/op` numbers below are the same on every
machine (Go 1.26 here; a different toolchain may shift a count by one as the runtime evolves). Two
examples are different on purpose: **#6** is read with `go build -gcflags='-m'` (compiler output, not
stdout), and **#10** is a `go test -bench` benchmark whose **ns/op and B/op are machine-dependent**
(marked *illustrative*). Standard-library only.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–17 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — measuring & the everyday wins
- [1. Percentiles and the tail](1-easy.md#1-percentiles-and-the-tail)
- [2. `testing.AllocsPerRun` — a deterministic count](1-easy.md#2-testingallocsperrun--a-deterministic-count)
- [3. Preallocate a slice](1-easy.md#3-preallocate-a-slice)
- [4. `strings.Builder` vs `+=`](1-easy.md#4-stringsbuilder-vs-)
- [5. `strconv.AppendInt` into a reused buffer → 0 allocs](1-easy.md#5-strconvappendint-into-a-reused-buffer--0-allocs)

### 🟡 [Medium](2-medium.md) — escape analysis, boxing, benchmarks
- [6. Escape analysis with `-gcflags=-m`](2-medium.md#6-escape-analysis-with--gcflags-m)
- [7. Interface boxing allocates](2-medium.md#7-interface-boxing-allocates)
- [8. `[]byte`→`string` conversion & the map-lookup elision](2-medium.md#8-bytestring-conversion--the-map-lookup-elision)
- [9. Presize a map with a size hint](2-medium.md#9-presize-a-map-with-a-size-hint)
- [10. A real benchmark with `go test -bench`](2-medium.md#10-a-real-benchmark-with-go-test--bench)

### 🔴 [Hard](3-hard.md) — pooling, memstats & a capstone
- [11. Reuse buffers with `sync.Pool`](3-hard.md#11-reuse-buffers-with-syncpool)
- [12. A zero-allocation log line](3-hard.md#12-a-zero-allocation-log-line)
- [13. Watch allocations drive the GC (`runtime.MemStats`)](3-hard.md#13-watch-allocations-drive-the-gc-runtimememstats)
- [14. `[]T` vs `[]*T`: the pointer tax](3-hard.md#14-t-vs-t-the-pointer-tax)
- [15. Capstone: a hot path driven to zero allocations](3-hard.md#15-capstone-a-hot-path-driven-to-zero-allocations)
- [16. Inlining & devirtualization](3-hard.md#16-inlining--devirtualization)
- [17. Generics keep values unboxed](3-hard.md#17-generics-keep-values-unboxed)
</content>
