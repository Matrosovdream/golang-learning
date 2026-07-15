# 46 · Hard (11–15) — pooling, memstats & a capstone

Back to [index](README.md) · Prev tier: [Medium](2-medium.md)

---

## 11. Reuse buffers with `sync.Pool`

When a hot path needs a scratch buffer, a `sync.Pool` lets goroutines recycle them instead of allocating
a fresh one each call. `Get` a buffer, use it, `Reset`, `Put` it back. After warm-up the allocation count
drops to zero.

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// A pool of reusable buffers. New is called only when the pool is empty.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

var sink int

func withPool(data []string) {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset() // reuse the backing array; keep its capacity
	for _, s := range data {
		b.WriteString(s)
	}
	sink = b.Len()
	bufPool.Put(b) // hand it back for the next call
}

func withoutPool(data []string) {
	var b bytes.Buffer // fresh each call → grows from empty → allocates
	for _, s := range data {
		b.WriteString(s)
	}
	sink = b.Len()
}

func main() {
	data := []string{"low", "-", "latency", "-", "go"}
	fmt.Printf("fresh bytes.Buffer each call: %.1f allocs/op\n", testing.AllocsPerRun(100, func() { withoutPool(data) }))
	fmt.Printf("bytes.Buffer from sync.Pool:  %.1f allocs/op\n", testing.AllocsPerRun(100, func() { withPool(data) }))
}
```

**Output**
```
fresh bytes.Buffer each call: 1.0 allocs/op
bytes.Buffer from sync.Pool:  0.0 allocs/op
```

The rules (and the footguns — always `Put` back, never keep a reference after `Put`, and remember the GC
empties the pool) get the full treatment in [lesson 47](../../47-low-latency-gc-contention.md). Here it's
just the payoff: reuse → zero allocations on the hot path.

---

## 12. A zero-allocation log line

Combining #5's `Append*` idea with a reused buffer, you can format a whole structured line with **no**
allocations — the technique behind zero-alloc loggers.

```go
package main

import (
	"fmt"
	"strconv"
	"testing"
)

var sink []byte

// appendLine writes `level=<lvl> ms=<n> msg=<quoted>\n` into buf, no allocations.
func appendLine(buf []byte, level string, ms int64, msg string) []byte {
	buf = append(buf, "level="...)
	buf = append(buf, level...)
	buf = append(buf, " ms="...)
	buf = strconv.AppendInt(buf, ms, 10)
	buf = append(buf, " msg="...)
	buf = strconv.AppendQuote(buf, msg)
	buf = append(buf, '\n')
	return buf
}

func main() {
	buf := make([]byte, 0, 128)
	zero := func() {
		buf = buf[:0]
		buf = appendLine(buf, "INFO", 42, "request ok")
		sink = buf
	}
	fmt.Printf("append-based line: %.0f allocs/op\n", testing.AllocsPerRun(100, zero))

	viaFmt := func() {
		sink = []byte(fmt.Sprintf("level=%s ms=%d msg=%q\n", "INFO", 42, "request ok"))
	}
	fmt.Printf("fmt.Sprintf line:  %.0f allocs/op\n", testing.AllocsPerRun(100, viaFmt))

	fmt.Print(string(buf))
}
```

**Output**
```
append-based line: 0 allocs/op
fmt.Sprintf line:  2 allocs/op
level=INFO ms=42 msg="request ok"
```

Zero vs two, per line. At a million log lines a second, that's two million allocations of GC pressure you
just deleted.

---

## 13. Watch allocations drive the GC (`runtime.MemStats`)

Why does allocating matter? Because every heap object is work for the collector. `runtime.ReadMemStats`
lets you see it: allocate a million escaping pointers and read the deltas.

```go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	const n = 1_000_000

	var before, after runtime.MemStats
	runtime.GC() // clean slate so the deltas below are just our work
	runtime.ReadMemStats(&before)

	// Allocate a million small heap objects: each &v escapes to the heap.
	ptrs := make([]*int, n)
	for i := 0; i < n; i++ {
		v := i
		ptrs[i] = &v
	}

	runtime.ReadMemStats(&after)
	runtime.KeepAlive(ptrs)

	fmt.Printf("heap objects allocated (Mallocs): %d\n", after.Mallocs-before.Mallocs)
	fmt.Printf("bytes allocated (TotalAlloc):     %d MiB\n", (after.TotalAlloc-before.TotalAlloc)>>20)
	fmt.Printf("GC cycles triggered (NumGC):      %d  (varies with GOGC/heap size)\n", after.NumGC-before.NumGC)
}
```

**Output**
```
heap objects allocated (Mallocs): 1000007
bytes allocated (TotalAlloc):     15 MiB
GC cycles triggered (NumGC):      1  (varies with GOGC/heap size)
```

A million `&v`s become a million heap objects the GC must track. `runtime.KeepAlive` stops the compiler
from freeing `ptrs` before we read the stats. `GODEBUG=gctrace=1 go run .` prints a line per GC cycle —
try it and watch the collector work. (The full GC model is [lesson 47](../../47-low-latency-gc-contention.md).)

---

## 14. `[]T` vs `[]*T`: the pointer tax

A `[]T` stores its elements *inline* in one backing array — one allocation. A `[]*T` stores pointers, and
each pointed-to element is its **own** heap object — `n+1` allocations, plus `n` more things for the GC to
scan on every cycle.

```go
package main

import (
	"fmt"
	"testing"
)

type Item struct {
	ID   int
	Name string
}

var sinkVals []Item
var sinkPtrs []*Item

func buildValues(n int) []Item {
	s := make([]Item, 0, n)
	for i := 0; i < n; i++ {
		s = append(s, Item{ID: i, Name: "x"}) // stored inline in the backing array
	}
	return s
}

func buildPointers(n int) []*Item {
	s := make([]*Item, 0, n)
	for i := 0; i < n; i++ {
		s = append(s, &Item{ID: i, Name: "x"}) // each &Item is its own heap object
	}
	return s
}

func main() {
	const n = 1000
	fmt.Printf("[]Item  (values):   %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sinkVals = buildValues(n) }))
	fmt.Printf("[]*Item (pointers): %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sinkPtrs = buildPointers(n) }))
}
```

**Output**
```
[]Item  (values):   1 allocs/op
[]*Item (pointers): 1001 allocs/op
```

1 vs 1001. Prefer value slices for collections of small structs — fewer allocations *and* less GC scan
work (pointers are what the collector chases). This "fewer pointers" idea is the heart of
[lesson 47](../../47-low-latency-gc-contention.md).

---

## 15. Capstone: a hot path driven to zero allocations

Take an allocation-heavy function — render a CSV row — and rewrite it to append into a caller-owned
buffer, proving the win with `AllocsPerRun`. This is the whole lesson in one example: measure, find the
allocations, reuse a buffer, re-measure.

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type Row struct {
	ID     int
	Name   string
	Amount int64 // cents
	Active bool
}

// v1: the obvious version — a temporary slice, Join, and a trailing concat.
func renderV1(r Row) string {
	fields := []string{
		strconv.Itoa(r.ID),
		r.Name,
		strconv.FormatInt(r.Amount, 10),
		strconv.FormatBool(r.Active),
	}
	return strings.Join(fields, ",") + "\n"
}

// v2: append straight into a caller-owned buffer — zero allocs when reused.
func renderV2(buf []byte, r Row) []byte {
	buf = strconv.AppendInt(buf, int64(r.ID), 10)
	buf = append(buf, ',')
	buf = append(buf, r.Name...)
	buf = append(buf, ',')
	buf = strconv.AppendInt(buf, r.Amount, 10)
	buf = append(buf, ',')
	buf = strconv.AppendBool(buf, r.Active)
	buf = append(buf, '\n')
	return buf
}

var sinkS string
var sinkB []byte

func main() {
	r := Row{ID: 7, Name: "widget", Amount: 1999, Active: true}

	fmt.Printf("v1 (slice + Join + concat): %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() { sinkS = renderV1(r) }))

	buf := make([]byte, 0, 64)
	fmt.Printf("v2 (append to reused buf):  %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() { buf = buf[:0]; sinkB = renderV2(buf, r) }))

	fmt.Printf("row: %q\n", string(renderV2(nil, r)))
}
```

**Output**
```
v1 (slice + Join + concat): 3 allocs/op
v2 (append to reused buf):  0 allocs/op
row: "7,widget,1999,true\n"
```

Same output, three allocations → zero. Note `v2` still allocates *once* if you pass a `nil` buffer (as the
last line does, to print the row) — the zero comes from **reusing** the buffer across calls. That's the
core low-latency move: pay for the buffer once, then amortise it over every row. Lessons
[47](../../47-low-latency-gc-contention.md) and [48](../../48-low-latency-lockfree-tail.md) build on this
to cut GC pressure and engineer the tail.

---

## 16. Inlining & devirtualization

The compiler **inlines** small functions — copying the body into the caller, which erases the call
overhead *and* lets escape analysis see through it (often removing an allocation). When a value's concrete
type is known at a call site, it also **devirtualizes** an interface call: no itable lookup, and the
method can then be inlined too. You can't see this at runtime, so read it from `-gcflags='-m'`.

```go
package main

import "fmt"

// Small leaf functions the compiler inlines: AST under ~80 nodes, no
// closures/defer/recover/select, no go:noinline.
func add(a, b int) int { return a + b }
func double(x int) int { return add(x, x) }

//go:noinline
func addNoInline(a, b int) int { return a + b } // pragma forbids inlining

type Stringer interface{ String() string }

type ID int

func (i ID) String() string { return "id" }

// describe takes an interface, but when the caller's concrete type is known the
// compiler devirtualizes the call (skips the itable lookup) and inlines it.
func describe(s Stringer) string { return s.String() }

func main() {
	var id ID = 7
	fmt.Println(double(21), addNoInline(20, 1), describe(id))
}
```

Run it, then read the inlining decisions:
```bash
go run .
go build -gcflags='-m' -o /dev/null .    # add -m=2 for the cost/threshold detail
```

**Output** (`go run .`)
```
42 21 id
```

**Compiler output** (`-gcflags='-m'`, the relevant lines)
```
./main.go:7:6: can inline add
./main.go:8:6: can inline double
./main.go:21:6: can inline describe
./main.go:8:36: inlining call to add
./main.go:25:20: inlining call to double
./main.go:25:54: devirtualizing s.String to ID
./main.go:25:54: inlining call to ID.String
```

`double`/`add`/`describe` all inline; the `describe(id)` call is **devirtualized** to the concrete `ID`
and its `String` inlined. `addNoInline` never gets a `can inline` line — the pragma blocks it (which is
why it's the one whose args escape to the heap for `fmt.Println`). Inlining is why tiny accessor methods
are free; the blockers are closures, `defer`/`recover`/`select`, and too-large bodies (~80 AST nodes).
Deeper still: `go tool compile -S` and `go tool objdump` show the actual assembly.

---

## 17. Generics keep values unboxed

Before generics, a "works on any number" function meant `interface{}` — which **boxes** every value onto
the heap. A generic function is instantiated per *GC shape* (size/alignment/pointer-ness) and operates on
the values directly, so it allocates nothing.

```go
package main

import (
	"fmt"
	"testing"
)

type Number interface{ ~int | ~int64 | ~float64 }

// Generic: instantiated per GC shape, it works on the int values directly —
// no boxing, so summing allocates nothing.
func Sum[T Number](xs []T) T {
	var total T
	for _, x := range xs {
		total += x
	}
	return total
}

var sinkT int
var sinkSlice []any

func main() {
	// Values > 255 so boxing isn't served from the runtime's small-int cache.
	ints := []int{1000, 2000, 3000, 4000, 5000}

	fmt.Printf("generic Sum[int]:        %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() { sinkT = Sum(ints) }))

	// The pre-generics way: hold the same numbers as []any. Each int must be
	// BOXED into an interface (a heap allocation) just to store it.
	fmt.Printf("box into []any then sum: %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() {
			anys := make([]any, len(ints))
			for i, v := range ints {
				anys[i] = v // boxing: escapes into anys → heap allocation
			}
			sinkSlice = anys
			s := 0
			for _, a := range anys {
				s += a.(int)
			}
			sinkT = s
		}))
}
```

**Output**
```
generic Sum[int]:        0 allocs/op
box into []any then sum: 6 allocs/op
```

Zero vs six (one per boxed int, plus the slice). Two caveats keep this honest: generics can't be *inlined*
(#16) and dispatch through a per-shape dictionary, so for a hot, tiny operation on one concrete type a
plain concrete function can still win — measure. But for "same algorithm over many value types," generics
give you the abstraction of `interface{}` **without** its allocation tax.

---

That's the whole library. Track your progress in [PROGRESS.md](PROGRESS.md); ask for more and I'll append
starting at #18.
</content>
