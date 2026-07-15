# 46 · Medium (6–10) — escape analysis, boxing, benchmarks

Back to [index](README.md) · Prev tier: [Easy](1-easy.md) · Next tier: [Hard](3-hard.md)

---

## 6. Escape analysis with `-gcflags=-m`

The compiler decides *at compile time* whether a value lives on the stack (free, reclaimed on return) or
**escapes** to the heap (costs an allocation + future GC work). Returning a pointer to a local forces the
escape. Ask the compiler to show its work with `-gcflags='-m'`.

```go
package main

import "fmt"

type Point struct{ X, Y int }

//go:noinline
func byValue() Point { return Point{1, 2} } // stays on the caller's stack

//go:noinline
func byPointer() *Point { return &Point{3, 4} } // &Point must outlive the frame → heap

func main() {
	v := byValue()
	p := byPointer()
	fmt.Println(v, *p)
}
```

Run it normally, then inspect the escape decisions:

```bash
go run .
go build -gcflags='-m' -o /dev/null .
```

**Output** (`go run .`)
```
{1 2} {3 4}
```

**Compiler output** (`-gcflags='-m'`, the key lines)
```
./main.go:11:34: &Point{...} escapes to heap
./main.go:16:14: v escapes to heap
./main.go:16:17: *p escapes to heap
```

`byPointer` allocates (`&Point{...} escapes to heap`); `byValue` does not appear — it stays on the stack.
(The two `line 16` escapes are `v` and `*p` being boxed into `interface{}` for `fmt.Println` — the very
boxing you measure in #7.) `//go:noinline` keeps the two functions from being inlined so the report is
easy to read. On a hot path, `-m` is how you find *why* an allocation is happening.

---

## 7. Interface boxing allocates

Storing a concrete value in an `interface{}` (`any`) may copy it to the heap so the interface can hold a
pointer to it. The runtime caches small integers (0–255), so those are free; larger values allocate.

```go
package main

import (
	"fmt"
	"testing"
)

var sink any

//go:noinline
func box(x int) any { return x } // storing an int in an interface may allocate

func main() {
	small := 42   // 0..255 are cached by the runtime → no allocation
	big := 100000 // outside the cache → boxing allocates
	fmt.Printf("box(42):     %.0f allocs/op\n", testing.AllocsPerRun(100, func() { sink = box(small) }))
	fmt.Printf("box(100000): %.0f allocs/op\n", testing.AllocsPerRun(100, func() { sink = box(big) }))
}
```

**Output**
```
box(42):     0 allocs/op
box(100000): 1 allocs/op
```

This is why `fmt.Printf("%d", n)` and friends allocate: every `...any` argument gets boxed. On a hot path,
prefer typed calls (`strconv.AppendInt`, typed logger fields) over `interface{}`-based formatting.

---

## 8. `[]byte`→`string` conversion & the map-lookup elision

`string(b)` and `[]byte(s)` normally **copy** (strings are immutable; the byte slice isn't). But the
compiler *elides* the copy in a few blessed spots — most usefully a **map lookup key**: `m[string(b)]`
does not allocate. Materialise the string first and the copy comes back.

```go
package main

import (
	"fmt"
	"testing"
)

var sinkStr string

func main() {
	m := map[string]int{"apple": 1, "banana": 2, "cherry": 3}
	key := []byte("banana")
	var v int

	// Blessed elision: m[string(b)] reuses the bytes, no new string is allocated.
	fmt.Printf("m[string(b)] (elided):       %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() { v = m[string(key)] }))

	// Break it: materialise the string so it escapes → the conversion must copy.
	fmt.Printf("sink = string(b); m[sink]:   %.0f allocs/op\n",
		testing.AllocsPerRun(100, func() { sinkStr = string(key); v = m[sinkStr] }))

	_ = v
}
```

**Output**
```
m[string(b)] (elided):       0 allocs/op
sink = string(b); m[sink]:   1 allocs/op
```

The same elision applies to `switch string(b)` and `for range string(b)`. When parsing bytes, keep one
representation and use these sites instead of converting back and forth.

---

## 9. Presize a map with a size hint

Maps grow by rehashing into ever-larger bucket arrays. `make(map[K]V, n)` allocates enough buckets up
front, so filling it doesn't rehash. The win is smaller than for slices (maps still allocate bucket
arrays), but it's real.

```go
package main

import (
	"fmt"
	"testing"
)

var sinkMap map[int]int

func build(n, hint int) map[int]int {
	m := make(map[int]int, hint) // hint=0 grows/rehashes repeatedly; hint=n sizes once
	for i := 0; i < n; i++ {
		m[i] = i
	}
	return m
}

func main() {
	const n = 10000
	fmt.Printf("make(map, 0)  (growing):  %.0f allocs/op\n", testing.AllocsPerRun(5, func() { sinkMap = build(n, 0) }))
	fmt.Printf("make(map, n)  (presized): %.0f allocs/op\n", testing.AllocsPerRun(5, func() { sinkMap = build(n, n) }))
}
```

**Output**
```
make(map, 0)  (growing):  81 allocs/op
make(map, n)  (presized): 34 allocs/op
```

Roughly halved. (Exact counts depend on the map's internal growth schedule and Go version; the direction
is what matters.)

---

## 10. A real benchmark with `go test -bench`

`AllocsPerRun` gives allocations; for *time* you want the `testing` benchmark harness. This is the same
comparison as #4, but now measuring `ns/op` and `B/op` too. Put it in a **`_test.go`** file (a trivial
`main` keeps the package buildable):

`main.go`
```go
package main

func main() {}
```

`bench_test.go`
```go
package main

import (
	"strings"
	"testing"
)

// Input read from a variable so the compiler can't fold it to a constant.
var parts = strings.Split(strings.Repeat("abc,", 1000), ",")

// Package-level sink defeats dead-code elimination of the result.
var sink string

func BenchmarkConcatPlus(b *testing.B) {
	b.ReportAllocs() // report allocs/op and B/op alongside ns/op
	for i := 0; i < b.N; i++ {
		var s string
		for _, p := range parts {
			s += p
		}
		sink = s
	}
}

func BenchmarkBuilder(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var bld strings.Builder
		for _, p := range parts {
			bld.WriteString(p)
		}
		sink = bld.String()
	}
}
```

Run:
```bash
go test -bench=. -benchmem
```

**Output** *(illustrative — `ns/op` and `B/op` depend on your CPU; the ratio and `allocs/op` are the point)*
```
BenchmarkConcatPlus-10    	    9885	    120425 ns/op	 1602940 B/op	     999 allocs/op
BenchmarkBuilder-10       	  352180	      3582 ns/op	    8440 B/op	      11 allocs/op
```

The `Builder` is ~34× faster and allocates 999 → 11. Note the three habits that keep the numbers honest:
`b.ReportAllocs()`, a package-level `sink`, and reading the input from a variable (not a constant). To
compare two versions rigorously, run with `-count=10` and feed both to `benchstat`.

---

Next tier: [🔴 Hard (11–15)](3-hard.md) — pooling, memstats, and a capstone.
</content>
