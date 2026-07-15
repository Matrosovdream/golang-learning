# 47 · Hard (11–15) — benchmarks, profiling & a capstone

Back to [index](README.md) · Prev tier: [Medium](2-medium.md)

---

## 11. Array-of-structs vs struct-of-arrays

Summing one field of a million records: with an **array-of-structs** each iteration pulls a whole 48-byte
`Particle` into cache to read 8 bytes of it. With a **struct-of-arrays** the `X` values are contiguous, so
every cache line is 100% useful data. This is a `go test -bench` example.

`main.go`
```go
package main

func main() {} // this example is run with `go test -bench`
```

`bench_test.go`
```go
package main

import "testing"

const N = 1_000_000

// Array-of-structs: summing X drags every other field through cache too.
type Particle struct {
	X, Y, Z, VX, VY, VZ float64
}

var aos = make([]Particle, N)

// Struct-of-arrays: the X values are contiguous, so the loop streams pure Xs.
var soaX = make([]float64, N)

var sink float64

func BenchmarkSumAoS(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var s float64
		for j := range aos {
			s += aos[j].X
		}
		sink = s
	}
}

func BenchmarkSumSoA(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var s float64
		for j := range soaX {
			s += soaX[j]
		}
		sink = s
	}
}
```

Run: `go test -bench=. -benchmem`

**Output** *(illustrative — `ns/op` is machine-dependent; the ratio is the point)*
```
BenchmarkSumAoS-10    	    1714	    679519 ns/op	       0 B/op	       0 allocs/op
BenchmarkSumSoA-10    	    2096	    572122 ns/op	       0 B/op	       0 allocs/op
```

The SoA loop is faster with zero code cleverness — just a better memory layout for *this* access pattern.
The gap widens as the struct grows (more wasted bytes per fetch) and shrinks if you touch every field
anyway. Restructure to SoA only for hot loops that read one or two fields across many records.

---

## 12. Contention: mutex vs atomic vs sharded

The same increment, three ways, measured **under contention** with `b.RunParallel` (which spreads the loop
across all CPUs). This is where the abstract "serialising is slow" becomes a number.

`main.go`
```go
package main

func main() {} // run with `go test -bench`
```

`bench_test.go`
```go
package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

// RunParallel spreads the loop across GOMAXPROCS goroutines, so these measure
// how each counter behaves *under contention*.

func BenchmarkMutex(b *testing.B) {
	var mu sync.Mutex
	var n int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			n++
			mu.Unlock()
		}
	})
	_ = n
}

func BenchmarkAtomic(b *testing.B) {
	var n atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n.Add(1)
		}
	})
}

const shards = 64

type cell struct {
	n atomic.Int64
	_ [56]byte // one counter per cache line
}

func BenchmarkSharded(b *testing.B) {
	var cells [shards]cell
	var next atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		id := int(next.Add(1)-1) % shards // each goroutine claims one shard
		for pb.Next() {
			cells[id].n.Add(1)
		}
	})
}
```

Run: `go test -bench=.`

**Output** *(illustrative — absolute `ns/op` varies; the ratios are the lesson)*
```
BenchmarkMutex-10      	18084932	        66.48 ns/op
BenchmarkAtomic-10     	36307294	        34.64 ns/op
BenchmarkSharded-10    	1000000000	         0.3838 ns/op
```

Mutex → atomic ≈ 2× faster; atomic → sharded ≈ **90× faster**, because a sharded counter turns one
fought-over cache line into 64 uncontended ones. The ladder — mutex, atomic, shard — is the standard
escalation when a counter shows up hot in a profile.

---

## 13. False sharing: pad to a cache line

Even with *no shared variable*, eight counters packed tightly share a couple of cache lines, and the cores
ping-pong those lines between their caches. Padding each counter to its own 64-byte line removes the
contention entirely.

`main.go`
```go
package main

func main() {} // run with `go test -bench`
```

`bench_test.go`
```go
package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

const G = 8

// Unpadded: 8 counters packed tightly → several share a 64-byte cache line, so
// cores fight over the line even though each touches a *different* counter.
type unpadded struct{ n atomic.Int64 }

// Padded: each counter sits on its own cache line → no false sharing.
type padded struct {
	n atomic.Int64
	_ [56]byte
}

func drive(b *testing.B, add func(id int)) {
	each := b.N / G
	var wg sync.WaitGroup
	b.ResetTimer()
	for i := 0; i < G; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < each; j++ {
				add(id)
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkFalseSharing(b *testing.B) {
	var cs [G]unpadded
	drive(b, func(id int) { cs[id].n.Add(1) })
}

func BenchmarkPadded(b *testing.B) {
	var cs [G]padded
	drive(b, func(id int) { cs[id].n.Add(1) })
}
```

Run: `go test -bench=.`

**Output** *(illustrative; the ~40× gap is the point)*
```
BenchmarkFalseSharing-10    	38199832	        27.86 ns/op
BenchmarkPadded-10          	1000000000	         0.5932 ns/op
```

Same increments, ~47× slower purely because the counters shared cache lines. This is why the sharded
counter in #9/#12 pads each cell with `[56]byte`. It's a niche fix — but when a profile shows a hot atomic
on shared-ish data, it's *the* fix.

---

## 14. Reading a heap profile: `alloc_space` vs `inuse_space`

A heap profile answers two different questions with two sample types: **`alloc_space`** = every byte ever
allocated (your *allocation rate* / GC pressure), **`inuse_space`** = bytes live *right now* (footprint /
leaks). A function can dominate one and be invisible in the other.

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

var live [][]byte    // stays alive → shows up in inuse_space
var churnSink []byte // overwritten each loop → shows up only in alloc_space

// allocInUse keeps ~1 MiB permanently live.
func allocInUse() {
	for i := 0; i < 1000; i++ {
		live = append(live, make([]byte, 1024))
	}
}

// allocChurn allocates ~1 GiB total but keeps none of it — pure GC pressure.
func allocChurn() {
	for i := 0; i < 1_000_000; i++ {
		churnSink = make([]byte, 1024)
	}
}

func main() {
	allocInUse()
	allocChurn()
	runtime.GC()

	f, err := os.Create("heap.out")
	if err != nil {
		panic(err)
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		panic(err)
	}
	f.Close()
	runtime.KeepAlive(live)
	fmt.Println("wrote heap.out — inspect it with `go tool pprof`")
}
```

Run it, then read the two sample types:
```bash
go run .
go tool pprof -top -sample_index=alloc_space heap.out   # allocation rate
go tool pprof -top -sample_index=inuse_space heap.out   # live footprint
```

**`alloc_space`** — where allocation (GC pressure) comes from:
```
      flat  flat%   sum%        cum   cum%
 1017.49MB 99.80% 99.80%  1017.49MB 99.80%  main.allocChurn (inline)
         0     0% 99.80%  1017.99MB 99.85%  main.main
```

**`inuse_space`** — what's actually live now:
```
      flat  flat%   sum%        cum   cum%
 1538.05kB 75.01% 75.01%  1538.05kB 75.01%  runtime.mallocgc
  512.50kB 24.99%   100%   512.50kB 24.99%  main.allocInUse (inline)
```

`allocChurn` is **99.8%** of all allocation but **0%** of the live heap — it churns a gigabyte and keeps
nothing. `allocInUse` is the reverse. For *latency* you chase `alloc_space` (the churn feeding the GC); for
*leaks/footprint* you chase `inuse_space`. Wiring `net/http/pprof` into a service lets you pull both from a
running process. *(The numbers are sampled — the heap profiler records roughly 1 in every 512 KiB — so the
live figure shows ~half of the 1 MiB; the proportions are what matter.)*

---

## 15. Capstone: a profile-guided fix

An event aggregator, the way you'd first write it (`map[string]*Stat`, grown from empty) and the way a
profile would push you to write it (`map[string]Stat`, presized). Same answer; far fewer allocations and
far fewer pointers for the GC.

```go
package main

import (
	"fmt"
	"testing"
)

type Event struct {
	Key string
	Val int64
}

type Stat struct {
	Count, Sum int64
}

// v1: map of POINTERS grown from empty — every new key allocates a *Stat, and
// the map itself rehashes as it grows.
func aggV1(events []Event) map[string]*Stat {
	m := map[string]*Stat{}
	for _, e := range events {
		s := m[e.Key]
		if s == nil {
			s = &Stat{}
			m[e.Key] = s
		}
		s.Count++
		s.Sum += e.Val
	}
	return m
}

// v2: map of VALUES, presized to the key count — no per-key pointer allocation,
// no rehashing. Fewer allocations AND fewer pointers for the GC to scan.
func aggV2(events []Event, keys int) map[string]Stat {
	m := make(map[string]Stat, keys)
	for _, e := range events {
		s := m[e.Key]
		s.Count++
		s.Sum += e.Val
		m[e.Key] = s
	}
	return m
}

var sink1 map[string]*Stat
var sink2 map[string]Stat

func main() {
	const keys, n = 500, 100_000
	names := make([]string, keys)
	for i := range names {
		names[i] = fmt.Sprintf("key-%d", i)
	}
	events := make([]Event, n)
	for i := range events {
		events[i] = Event{Key: names[i%keys], Val: int64(i)}
	}

	fmt.Printf("v1 (map[string]*Stat, grown):    %.0f allocs/op\n",
		testing.AllocsPerRun(20, func() { sink1 = aggV1(events) }))
	fmt.Printf("v2 (map[string]Stat, presized):  %.0f allocs/op\n",
		testing.AllocsPerRun(20, func() { sink2 = aggV2(events, keys) }))

	// Same answer, both ways.
	fmt.Printf("v1[key-0] = %+v\n", *sink1["key-0"])
	fmt.Printf("v2[key-0] = %+v\n", sink2["key-0"])
}
```

**Output**
```
v1 (map[string]*Stat, grown):    517 allocs/op
v2 (map[string]Stat, presized):  4 allocs/op
v1[key-0] = {Count:200 Sum:9950000}
v2[key-0] = {Count:200 Sum:9950000}
```

517 → 4 allocations, and `v2`'s map holds no pointer values for the GC to chase. The workflow is the whole
lesson: profile (`alloc_space`, #14) → see the map-of-pointers churn → switch to a presized value map →
re-measure. If this aggregator were also *contended*, you'd shard it (#9) as the next step.

---

That's the whole library. Track your progress in [PROGRESS.md](PROGRESS.md); ask for more and I'll append
starting at #16. Next lesson: [48 — Lock-Free, Zero-Copy & Tail Latency](../../48-low-latency-lockfree-tail.md).
</content>
