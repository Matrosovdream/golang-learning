# 47 · Easy (1–5) — memory layout & the GC knobs

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Struct padding: field order changes size

The compiler aligns each field to its size, inserting **padding** to do so. That means the *order* of the
fields changes the struct's total size — the same three fields cost 24 bytes or 16 depending on layout.

```go
package main

import (
	"fmt"
	"unsafe"
)

// Same three fields, different order → different size, because the compiler
// inserts padding to keep each field aligned.
type Bad struct {
	a bool  // 1 byte, then 7 bytes of padding so b is 8-aligned
	b int64 // 8 bytes
	c bool  // 1 byte, then 7 bytes of padding to round the struct up
}

type Good struct {
	b int64 // 8 bytes
	a bool  // 1 byte
	c bool  // 1 byte, then 6 bytes of padding
}

func main() {
	bad := unsafe.Sizeof(Bad{})
	good := unsafe.Sizeof(Good{})
	fmt.Printf("Bad:   %d bytes\n", bad)
	fmt.Printf("Good:  %d bytes\n", good)
	fmt.Printf("saved: %d bytes/struct (%.0f%%)\n", bad-good, 100*float64(bad-good)/float64(bad))
}
```

**Output**
```
Bad:   24 bytes
Good:  16 bytes
saved: 8 bytes/struct (33%)
```

A third smaller. For a struct you keep in the millions, that's a third less memory *and* a third less to
drag through cache. Group fields largest-to-smallest to minimise padding.

---

## 2. Find the padding holes with `Offsetof`

`unsafe.Offsetof` shows exactly where each field lands, so you can *see* the padding holes rather than
guess at them.

```go
package main

import (
	"fmt"
	"unsafe"
)

type Bad struct {
	a bool
	b int64
	c bool
}

func main() {
	var s Bad
	// Offsetof reveals the padding holes: a is at 0, but b jumps to 8.
	fmt.Printf("Alignof(Bad): %d\n", unsafe.Alignof(s))
	fmt.Printf("Offsetof(a):  %d\n", unsafe.Offsetof(s.a))
	fmt.Printf("Offsetof(b):  %d   <- 7 bytes of padding after a\n", unsafe.Offsetof(s.b))
	fmt.Printf("Offsetof(c):  %d\n", unsafe.Offsetof(s.c))
	fmt.Printf("Sizeof(Bad):  %d   <- 7 bytes of padding after c\n", unsafe.Sizeof(s))
}
```

**Output**
```
Alignof(Bad): 8
Offsetof(a):  0
Offsetof(b):  8   <- 7 bytes of padding after a
Offsetof(c):  16
Sizeof(Bad):  24   <- 7 bytes of padding after c
```

`a` occupies byte 0, but `b` (an 8-byte `int64`) must start at an 8-aligned offset, so bytes 1–7 are
wasted. The `fieldalignment` linter (`go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest`)
flags these automatically.

---

## 3. Field order → cache-line packing

The CPU loads memory in 64-byte **cache lines**. How many records fit per line decides how much useful
data each memory fetch brings in — which is what a hot loop over millions of records lives or dies by.

```go
package main

import (
	"fmt"
	"unsafe"
)

// A record kept in the millions. Field order controls its size — and therefore
// how many fit in a 64-byte cache line, which is what a hot loop streams through.
type Bad struct {
	A bool
	B int64
	C bool
	D int64
	E bool
}

type Good struct {
	B int64
	D int64
	A bool
	C bool
	E bool
}

func main() {
	const line = 64
	bad := unsafe.Sizeof(Bad{})
	good := unsafe.Sizeof(Good{})
	fmt.Printf("Bad:  %2d bytes → %d fit per 64B cache line\n", bad, line/int(bad))
	fmt.Printf("Good: %2d bytes → %d fit per 64B cache line\n", good, line/int(good))
	fmt.Printf("At 1e6 records: %d MiB vs %d MiB\n", int(bad)*1_000_000>>20, int(good)*1_000_000>>20)
}
```

**Output**
```
Bad:  40 bytes → 1 fit per 64B cache line
Good: 24 bytes → 2 fit per 64B cache line
```
```
At 1e6 records: 38 MiB vs 22 MiB
```

Reordering doubles the records per cache line and cuts 16 MiB off a million-record slice. Do this for the
hot, high-count structs a profile points at — not every struct in the codebase.

---

## 4. Index-based references beat pointers

A tree of `*Node` is `n` separate heap objects and `2n` pointers the GC must scan every cycle. Store the
whole tree in one `[]Node` and reference children by `int32` **index**: one allocation, and the backing
array holds *no pointers*, so the collector skips it entirely.

```go
package main

import (
	"fmt"
	"testing"
)

// Pointer version: each node is its own heap object, and every child pointer is
// something the GC must scan on each cycle.
type PNode struct {
	Val         int
	Left, Right *PNode
}

func buildPtr(depth int) *PNode {
	if depth == 0 {
		return nil
	}
	return &PNode{Val: depth, Left: buildPtr(depth - 1), Right: buildPtr(depth - 1)}
}

// Index version: all nodes live in ONE []INode; children are int32 indices
// (-1 = nil). The backing array holds no pointers, so the GC scans nothing inside.
type INode struct {
	Val         int
	Left, Right int32
}

func buildIdx(depth int) []INode {
	nodes := make([]INode, 0, (1<<depth)-1)
	var build func(d int) int32
	build = func(d int) int32 {
		if d == 0 {
			return -1
		}
		i := int32(len(nodes))
		nodes = append(nodes, INode{Val: d})
		l := build(d - 1)
		r := build(d - 1)
		nodes[i].Left, nodes[i].Right = l, r
		return i
	}
	build(depth)
	return nodes
}

var sinkP *PNode
var sinkI []INode

func main() {
	const depth = 10 // 2^10 - 1 = 1023 nodes
	fmt.Printf("*PNode tree (pointers): %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sinkP = buildPtr(depth) }))
	fmt.Printf("[]INode tree (indices): %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sinkI = buildIdx(depth) }))
}
```

**Output**
```
*PNode tree (pointers): 1023 allocs/op
[]INode tree (indices): 1 allocs/op
```

1023 → 1. Beyond the allocation win, the index version is invisible to the garbage collector's mark phase
(no pointers to chase), which is the real low-latency payoff. This is how high-performance parsers and
game engines store node-heavy data.

---

## 5. `GOGC`: pacing the collector

`GOGC` sets *when* the GC runs: at the default 100, it collects once the heap has grown 100% since the last
cycle. Higher = fewer collections, more memory used. It's a CPU-for-memory dial.

```go
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var sink []byte

func churn(rounds int) {
	for i := 0; i < rounds; i++ {
		sink = make([]byte, 4096) // allocate and discard → pure GC pressure
	}
}

func gcCycles(percent, rounds int) uint32 {
	debug.SetGCPercent(percent)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	churn(rounds)
	runtime.ReadMemStats(&after)
	return after.NumGC - before.NumGC
}

func main() {
	const rounds = 200_000
	low := gcCycles(50, rounds)    // small heap-growth target → collect often
	high := gcCycles(1600, rounds) // large target → collect rarely
	fmt.Printf("GOGC=50   → %d GC cycles\n", low)
	fmt.Printf("GOGC=1600 → %d GC cycles\n", high)
	fmt.Println("(exact counts vary; the point is a low GOGC collects far more often — CPU for memory)")
}
```

**Output** *(counts vary by machine; the direction is the lesson)*
```
GOGC=50   → 541 GC cycles
GOGC=1600 → 12 GC cycles
```

Same allocation work, ~45× more collections at low `GOGC`. In production you rarely set `GOGC` directly —
you set `GOMEMLIMIT` (next tier, #6) and let the pacer choose. Watch any program's cycles live with
`GODEBUG=gctrace=1 go run .`.

---

Next tier: [🟡 Medium (6–10)](2-medium.md) — memory limits, pooling, and contention.
</content>
