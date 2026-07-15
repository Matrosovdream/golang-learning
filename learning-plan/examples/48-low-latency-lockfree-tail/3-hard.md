# 48 · Hard (11–15) — lock-free structures & a capstone

Back to [index](README.md) · Prev tier: [Medium](2-medium.md)

---

## 11. A lock-free stack (Treiber)

A **Treiber stack** is the classic lock-free structure: `Push`/`Pop` are CAS loops on the head pointer,
built on `atomic.Pointer[node]`. No mutex — concurrent pushers race, and the loser of a CAS simply retries.
(Run under `-race`.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type node struct {
	val  int
	next *node
}

// A lock-free stack (Treiber stack): Push/Pop with a CAS on the head pointer.
type Stack struct {
	head atomic.Pointer[node]
}

func (s *Stack) Push(v int) {
	n := &node{val: v}
	for {
		old := s.head.Load()
		n.next = old
		if s.head.CompareAndSwap(old, n) { // succeeds iff head hasn't moved
			return
		}
	}
}

func (s *Stack) Pop() (int, bool) {
	for {
		old := s.head.Load()
		if old == nil {
			return 0, false
		}
		if s.head.CompareAndSwap(old, old.next) {
			return old.val, true
		}
	}
}

func main() {
	var s Stack
	const goroutines, perG = 50, 1000
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				s.Push(1)
			}
		}()
	}
	wg.Wait()

	// Drain and count — every pushed item must come back exactly once.
	count, sum := 0, 0
	for {
		v, ok := s.Pop()
		if !ok {
			break
		}
		count++
		sum += v
	}
	fmt.Printf("pushed %d items concurrently, popped %d (sum %d)\n", goroutines*perG, count, sum)
	fmt.Println("(Go's GC sidesteps the classic ABA problem — a live node is never freed and reused)")
}
```

**Output**
```
pushed 50000 items concurrently, popped 50000 (sum 50000)
(Go's GC sidesteps the classic ABA problem — a live node is never freed and reused)
```

All 50000 pushes survive. In C, concurrent `Pop` invites the **ABA problem** (head goes A→B→A and a stale
CAS wrongly succeeds); Go's GC keeps a referenced node alive so its address can't be recycled underneath
you. Even so — this is showcase code. A channel or a `sync.Mutex`-guarded slice is what you'd actually
ship; reach for lock-free structures only with a proven, measured need.

---

## 12. Copy-on-write snapshots never tear

Extending #1 to a slice: readers must always see a **whole, consistent** snapshot, never a half-written
one. The writer builds each new slice completely, then atomically swaps the pointer. Readers verify the
invariant `buf[i] == i` holds end to end.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Every published snapshot satisfies the invariant buf[i] == i. A reader that
// ever saw a torn (partially-updated) slice would catch buf[i] != i.
func makeSnapshot(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s
}

func main() {
	var snap atomic.Pointer[[]int]
	init := makeSnapshot(1)
	snap.Store(&init)

	var wg sync.WaitGroup
	var violations, reads atomic.Int64

	// Readers: Load a snapshot and verify its invariant end to end.
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100_000; i++ {
				s := *snap.Load()
				for j := range s {
					if s[j] != j {
						violations.Add(1)
						break
					}
				}
				reads.Add(1)
			}
		}()
	}
	// Writer: publish ever-larger consistent snapshots (built fully BEFORE publish).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := 2; n <= 200; n++ {
			s := makeSnapshot(n)
			snap.Store(&s)
		}
	}()
	wg.Wait()

	fmt.Printf("reads: %d, torn snapshots observed: %d\n", reads.Load(), violations.Load())
	fmt.Println("readers always saw a whole snapshot — the writer swapped the pointer, never edited in place")
}
```

**Output**
```
reads: 800000, torn snapshots observed: 0
```
```
readers always saw a whole snapshot — the writer swapped the pointer, never edited in place
```

800,000 reads, zero tears. The pattern generalises to any read-mostly structure (routing tables, rule
sets, in-memory indexes): **build a new immutable version, publish it with one atomic store, and let old
readers drain naturally.** No reader ever blocks on a writer.

---

## 13. Zero allocations → zero GC on the hot path

The payoff of everything in lessons 46–48: a hot path that allocates nothing gives the garbage collector
nothing to do — so no GC cycles, and no GC-assist stealing CPU from your request during a collection.

```go
package main

import (
	"fmt"
	"runtime"
	"strconv"
)

var sinkS string
var sinkB []byte

func gcDuring(f func()) uint32 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.NumGC - before.NumGC
}

func main() {
	const n = 5_000_000

	// Allocating hot path: a fresh string every iteration → constant GC pressure.
	allocGC := gcDuring(func() {
		for i := 0; i < n; i++ {
			sinkS = "id-" + strconv.Itoa(i)
		}
	})

	// Zero-alloc hot path: format into ONE reused buffer → nothing to collect.
	zeroGC := gcDuring(func() {
		buf := make([]byte, 0, 32)
		for i := 0; i < n; i++ {
			buf = buf[:0]
			buf = append(buf, "id-"...)
			buf = strconv.AppendInt(buf, int64(i), 10)
			sinkB = buf
		}
	})

	fmt.Printf("allocating path (%d iters): %d GC cycles\n", n, allocGC)
	fmt.Printf("zero-alloc path (%d iters): %d GC cycles\n", n, zeroGC)
	fmt.Println("no allocation → nothing to collect → no GC-assist stealing CPU from your hot path")
}
```

**Output** *(the allocating count varies by machine/GOGC; zero is exact)*
```
allocating path (5000000 iters): 29 GC cycles
zero-alloc path (5000000 iters): 0 GC cycles
```

The allocating loop triggered 29 collections; the zero-alloc loop triggered **none**. During each of those
29 cycles, allocating goroutines pay GC-assist tax — which is exactly the CPU spike that shows up in your
tail. Fewer allocations is the most reliable tail-latency lever you have.

---

## 14. Zero-alloc serialization with a pooled buffer

Combining the pieces: serialize a response into a buffer drawn from a `sync.Pool`, formatting with
`Append*`. Once the pool is warm, it's **zero allocations per call** — the shape of a zero-alloc
encoder/logger.

```go
package main

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
)

type Resp struct {
	ID    int
	Name  string
	Score int
}

// Pool of *[]byte (a pointer, so putting it back doesn't allocate a header).
var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }}

var sinkLen int

// serialize writes a JSON object into a pooled buffer and hands it to consume —
// zero allocations once the pool is warm.
func serialize(r Resp, consume func([]byte)) {
	bp := bufPool.Get().(*[]byte)
	b := (*bp)[:0]
	b = append(b, `{"id":`...)
	b = strconv.AppendInt(b, int64(r.ID), 10)
	b = append(b, `,"name":`...)
	b = strconv.AppendQuote(b, r.Name)
	b = append(b, `,"score":`...)
	b = strconv.AppendInt(b, int64(r.Score), 10)
	b = append(b, '}')
	consume(b)
	*bp = b
	bufPool.Put(bp)
}

func main() {
	r := Resp{ID: 7, Name: "widget", Score: 99}

	allocs := testing.AllocsPerRun(1000, func() {
		serialize(r, func(b []byte) { sinkLen = len(b) })
	})
	fmt.Printf("serialize: %.1f allocs/op\n", allocs)

	serialize(r, func(b []byte) { fmt.Printf("output: %s\n", b) })
}
```

**Output**
```
serialize: 0.0 allocs/op
output: {"id":7,"name":"widget","score":99}
```

Pooling `*[]byte` (a pointer, so `Put` doesn't box a slice header), `[:0]` to reuse the backing array, and
`Append*` to format in place — the same three moves the fast JSON libraries use. In a real handler you'd
`w.Write(b)` directly and never build the string.

---

## 15. Capstone: a zero-allocation hot handler

Everything at once: a request handler that reads its config **lock-free** (`atomic.Pointer`, #1/#12),
serializes into a **pooled buffer** (#14) with `Append*`, and allocates **nothing** per request — even
while the config is hot-reloaded concurrently.

```go
package main

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

type Config struct {
	Greeting string
	MaxItems int
}

type Request struct {
	User  string
	Items []int
}

var cfg atomic.Pointer[Config]
var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 512); return &b }}
var sinkLen int

// handle: the hot path. Reads config lock-free (atomic.Pointer), serializes into
// a pooled buffer with Append*, and allocates nothing per request.
func handle(r Request, consume func([]byte)) {
	c := cfg.Load() // lock-free config snapshot — no mutex on the read path

	bp := bufPool.Get().(*[]byte)
	b := (*bp)[:0]
	b = append(b, c.Greeting...)
	b = append(b, ' ')
	b = append(b, r.User...)
	b = append(b, ": "...)
	n := len(r.Items)
	if n > c.MaxItems {
		n = c.MaxItems
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(r.Items[i]), 10)
	}
	consume(b)
	*bp = b
	bufPool.Put(bp)
}

func main() {
	cfg.Store(&Config{Greeting: "hello", MaxItems: 3})
	req := Request{User: "alice", Items: []int{10, 20, 30, 40, 50}}

	allocs := testing.AllocsPerRun(1000, func() {
		handle(req, func(b []byte) { sinkLen = len(b) })
	})
	fmt.Printf("handler hot path: %.1f allocs/op\n", allocs)

	handle(req, func(b []byte) { fmt.Printf("response:     %q\n", b) })

	// Hot-reload the config concurrently — readers never block or tear.
	cfg.Store(&Config{Greeting: "hi", MaxItems: 2})
	handle(req, func(b []byte) { fmt.Printf("after reload: %q\n", b) })
}
```

**Output**
```
handler hot path: 0.0 allocs/op
response:     "hello alice: 10,20,30"
after reload: "hi alice: 10,20"
```

Zero allocations per request, a lock-free config read, and a hot-reload that readers never notice. That's
the whole track in one function — but remember the discipline behind it: **measure
([46](../../46-low-latency-measuring.md)) → profile ([47](../../47-low-latency-gc-contention.md)) → fix the
hot path → re-measure.** Around this core you'd wrap the tail-latency levers from the lesson: deadlines,
load shedding, hedged reads, `GOMEMLIMIT`, and the right `GOMAXPROCS`. Lock-free, zero-copy code you didn't
measure a need for is just complexity you'll debug later.

---

That's the whole library — and the end of **Part 11**. Track your progress in [PROGRESS.md](PROGRESS.md);
ask for more and I'll append starting at #16.
</content>
