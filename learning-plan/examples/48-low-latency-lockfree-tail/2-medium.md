# 48 · Medium (6–10) — amortising: batch, coalesce, reuse

Back to [index](README.md) · Prev tier: [Easy](1-easy.md) · Next tier: [Hard](3-hard.md)

---

## 6. `net.Buffers`: one vectored write

To send a header and a body, you don't have to concatenate them into one slice first. `net.Buffers` holds
`[][]byte` and writes them all in a single **vectored** operation (`writev` on a real socket).

```go
package main

import (
	"bytes"
	"fmt"
	"net"
)

func main() {
	// Header lines and body as separate slices — no concatenation needed.
	bufs := net.Buffers{
		[]byte("HTTP/1.1 200 OK\r\n"),
		[]byte("Content-Length: 5\r\n"),
		[]byte("\r\n"),
		[]byte("hello"),
	}

	var out bytes.Buffer
	n, _ := bufs.WriteTo(&out)

	fmt.Printf("wrote %d bytes from %d slices in one vectored write:\n", n, 4)
	fmt.Print(out.String())
	fmt.Println("\n(against a real *net.TCPConn this is a single writev syscall — no join, no copy)")
}
```

**Output**
```
wrote 43 bytes from 4 slices in one vectored write:
HTTP/1.1 200 OK
Content-Length: 5

hello
(against a real *net.TCPConn this is a single writev syscall — no join, no copy)
```

Against a `*net.TCPConn`, `WriteTo` issues one `writev` syscall over the four slices — you skip both the
concatenation allocation *and* the extra syscalls. Handy for framed protocols (length prefix + payload).

---

## 7. Batch to amortise

Batching pays a per-call cost (a syscall, a round trip, a lock) **once** for many items. It trades a
little latency for a lot of throughput — bounded by flushing on a size threshold **and** a timer.

```go
package main

import "fmt"

// Batcher flushes when it reaches maxSize. In production you also flush on a
// timer, so the first item in a partial batch can't wait forever under low load.
type Batcher struct {
	maxSize int
	buf     []int
	flushes int
}

func (b *Batcher) Add(x int) {
	b.buf = append(b.buf, x)
	if len(b.buf) >= b.maxSize {
		b.flush()
	}
}

func (b *Batcher) flush() {
	if len(b.buf) == 0 {
		return
	}
	fmt.Printf("  flush %d items: %v\n", len(b.buf), b.buf)
	b.buf = b.buf[:0]
	b.flushes++
}

func main() {
	b := &Batcher{maxSize: 4}
	for i := 1; i <= 10; i++ {
		b.Add(i)
	}
	b.flush() // final partial batch (a timer would trigger this in production)
	fmt.Printf("10 items → %d network calls instead of 10\n", b.flushes)
}
```

**Output**
```
  flush 4 items: [1 2 3 4]
  flush 4 items: [5 6 7 8]
  flush 2 items: [9 10]
10 items → 3 network calls instead of 10
```

The **time bound is not optional**: a size-only flush can hold the first item forever when traffic is
slow. Always pair "flush at N items" with "flush after T milliseconds". Bigger batches = more throughput
but more worst-case latency; tune to your latency budget.

---

## 8. Coalesce duplicate work (mini singleflight)

When many goroutines ask for the *same* thing at once (a cache miss stampede), do the work **once** and
share the result. This is `singleflight` ([lesson 38](../../38-caching-patterns.md)); here's a minimal
hand-rolled version so you can see the mechanism.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type call struct {
	wg  sync.WaitGroup
	val int
}

// Group coalesces concurrent calls for the same key into ONE execution of fn —
// a minimal singleflight (real singleflight forgets the key once fn completes).
type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() int) int {
	g.mu.Lock()
	if g.m == nil {
		g.m = map[string]*call{}
	}
	if c, ok := g.m[key]; ok { // someone is already computing this key
		g.mu.Unlock()
		c.wg.Wait()
		return c.val
	}
	c := &call{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val = fn() // only the first caller runs fn; the rest wait for it
	c.wg.Done()
	return c.val
}

func main() {
	var g Group
	var loads atomic.Int64
	const n = 100

	var wg sync.WaitGroup
	results := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = g.Do("user:42", func() int {
				loads.Add(1) // the expensive load — should happen ONCE
				return 42
			})
		}(i)
	}
	wg.Wait()

	fmt.Printf("%d concurrent callers, underlying loads: %d\n", n, loads.Load())
	fmt.Printf("all got the same result: %v\n", results[0] == 42)
}
```

**Output**
```
100 concurrent callers, underlying loads: 1
all got the same result: true
```

100 callers, one expensive load. The real `golang.org/x/sync/singleflight` *forgets* the key once `fn`
returns (so a later miss re-runs), and offers `DoChan` / `Forget`; this teaching version keeps the entry,
which is why the count is a clean 1. Use it in front of caches and any deduplicable backend call.

---

## 9. A zero-allocation ring buffer

A ring buffer reuses one fixed array as a queue via head/tail indices, so a producer/consumer hot path
allocates **nothing** per item. (A single-producer/single-consumer ring can even be made lock-free with
two atomic indices.)

```go
package main

import (
	"fmt"
	"testing"
)

// A fixed-capacity ring buffer: enqueue/dequeue reuse the same backing array,
// so a producer/consumer hot path allocates nothing per item.
type Ring struct {
	buf        []int
	head, tail int
	size       int
}

func NewRing(capacity int) *Ring { return &Ring{buf: make([]int, capacity)} }

func (r *Ring) Push(x int) bool {
	if r.size == len(r.buf) {
		return false // full
	}
	r.buf[r.tail] = x
	r.tail = (r.tail + 1) % len(r.buf)
	r.size++
	return true
}

func (r *Ring) Pop() (int, bool) {
	if r.size == 0 {
		return 0, false // empty
	}
	x := r.buf[r.head]
	r.head = (r.head + 1) % len(r.buf)
	r.size--
	return x, true
}

func main() {
	r := NewRing(8)

	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	var got []int
	for {
		x, ok := r.Pop()
		if !ok {
			break
		}
		got = append(got, x)
	}
	fmt.Printf("FIFO order: %v\n", got)

	// Once the ring exists, push+pop touch only the backing array: zero allocs.
	allocs := testing.AllocsPerRun(1000, func() {
		r.Push(1)
		r.Pop()
	})
	fmt.Printf("push+pop: %.0f allocs/op\n", allocs)
}
```

**Output**
```
FIFO order: [1 2 3 4 5]
push+pop: 0 allocs/op
```

Bounded and allocation-free — the two properties you want on a hot queue. The fixed capacity also gives
you natural back-pressure: a full ring returns `false`, so you shed or block instead of growing unbounded.

---

## 10. Hedged requests cut the tail

For an idempotent read to a replicated backend, if the first attempt hasn't answered by ~p95, fire a
**second** to another replica and take whichever returns first. A sliver of extra load erases the tail
caused by one slow replica. This models it with fixed latencies (no real waiting).

```go
package main

import (
	"fmt"
	"sort"
	"time"
)

func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(float64(len(sorted))*p/100+0.5) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func main() {
	// 100 requests. First-attempt latency: mostly 1ms, with a slow tail.
	// A hedge fires at 2ms to another replica that answers ~1ms later.
	const hedgeAfter = 2 * time.Millisecond
	const hedgeService = 1 * time.Millisecond

	first := make([]time.Duration, 100)
	for i := range first {
		first[i] = 1 * time.Millisecond
	}
	first[95], first[96], first[97], first[98], first[99] =
		8*time.Millisecond, 10*time.Millisecond, 12*time.Millisecond, 15*time.Millisecond, 20*time.Millisecond

	hedged := make([]time.Duration, len(first))
	for i, d := range first {
		alt := hedgeAfter + hedgeService // when the hedge would answer
		if d > hedgeAfter && alt < d {
			hedged[i] = alt // take the faster hedge
		} else {
			hedged[i] = d
		}
	}

	fs := append([]time.Duration(nil), first...)
	hs := append([]time.Duration(nil), hedged...)
	sort.Slice(fs, func(i, j int) bool { return fs[i] < fs[j] })
	sort.Slice(hs, func(i, j int) bool { return hs[i] < hs[j] })

	fmt.Printf("no hedge → p50 %v, p99 %v, max %v\n", percentile(fs, 50), percentile(fs, 99), percentile(fs, 100))
	fmt.Printf("hedged   → p50 %v, p99 %v, max %v\n", percentile(hs, 50), percentile(hs, 99), percentile(hs, 100))
	fmt.Println("one hedge at 2ms erases the long tail — for idempotent reads only")
}
```

**Output**
```
no hedge → p50 1ms, p99 15ms, max 20ms
hedged   → p50 1ms, p99 3ms, max 3ms
```

The median is unchanged, but p99 collapses from 15ms to 3ms. Two caveats: **hedge idempotent reads only**
(a hedged "charge card" charges twice), and **cancel the loser** with `context` so you don't do double
work for every slow request. This is the core idea of Dean & Barroso's "The Tail at Scale".

---

Next tier: [🔴 Hard (11–15)](3-hard.md) — lock-free structures and a capstone.
</content>
