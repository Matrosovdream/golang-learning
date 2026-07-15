# 46 · Easy (1–5) — measuring & the everyday wins

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Percentiles and the tail

Latency is a *distribution*, not a number. The **mean** hides the pauses users feel; the **tail** (p99)
is what a fan-out request actually experiences. Here 90 requests are fast, 9 are slow, 1 is a GC-pause
outlier — watch how p50, p99, and the max tell three different stories.

```go
package main

import (
	"fmt"
	"sort"
	"time"
)

// percentile returns the p-th percentile (0..100) of sorted durations using
// the nearest-rank method. sorted must be ascending.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(float64(len(sorted))*p/100 + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func main() {
	// 100 requests: 90 fast (1ms), 9 slow (50ms), 1 very slow GC pause (500ms).
	lat := make([]time.Duration, 0, 100)
	for i := 0; i < 90; i++ {
		lat = append(lat, 1*time.Millisecond)
	}
	for i := 0; i < 9; i++ {
		lat = append(lat, 50*time.Millisecond)
	}
	lat = append(lat, 500*time.Millisecond)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })

	var sum time.Duration
	for _, d := range lat {
		sum += d
	}
	mean := sum / time.Duration(len(lat))

	fmt.Printf("mean: %v\n", mean)
	fmt.Printf("p50:  %v\n", percentile(lat, 50))
	fmt.Printf("p99:  %v\n", percentile(lat, 99))
	fmt.Printf("max:  %v\n", percentile(lat, 100))
}
```

**Output**
```
mean: 10.4ms
p50:  1ms
p99:  50ms
max:  500ms
```

The mean (10.4ms) describes *no actual request*. Half finish in 1ms, but 1% take 50ms+ and the worst is
500ms. Report percentiles, and optimize the tail.

---

## 2. `testing.AllocsPerRun` — a deterministic count

Benchmark *time* is noisy and machine-dependent. Benchmark *allocations* with `testing.AllocsPerRun`:
it runs your function `runs` times and returns the **average allocations per call** — a whole number you
can rely on being the same everywhere. (You can call it from a normal `main`; you don't need a test.)

```go
package main

import (
	"fmt"
	"strconv"
	"testing"
)

var sink string

func main() {
	n := 12345
	// AllocsPerRun returns a deterministic average — same on every machine.
	a := testing.AllocsPerRun(100, func() { sink = fmt.Sprintf("%d", n) })
	b := testing.AllocsPerRun(100, func() { sink = strconv.Itoa(n) })
	fmt.Printf("fmt.Sprintf(\"%%d\"): %.0f allocs/op\n", a)
	fmt.Printf("strconv.Itoa:      %.0f allocs/op\n", b)
}
```

**Output**
```
fmt.Sprintf("%d"): 1 allocs/op
strconv.Itoa:      1 allocs/op
```

Both allocate exactly **one** string — the result. (They tie on allocations, but `Itoa` is much cheaper
in CPU: no format-string parsing, no boxing the `int` into an `interface{}`.) To get to **zero**, you
have to stop allocating the result at all — see #5.

The `var sink` matters: assigning the result to a package variable stops the compiler from deleting the
call as dead code, which would report a misleading `0 allocs/op`.

---

## 3. Preallocate a slice

Appending to a `nil` slice regrows the backing array repeatedly (roughly doubling each time), and each
growth is an allocation + a copy. If you know the size, `make` it once.

```go
package main

import (
	"fmt"
	"testing"
)

var sinkSlice []int

func buildAppendNil(n int) []int {
	var s []int // nil slice: append reallocates as it grows
	for i := 0; i < n; i++ {
		s = append(s, i)
	}
	return s
}

func buildPrealloc(n int) []int {
	s := make([]int, 0, n) // one allocation, right-sized up front
	for i := 0; i < n; i++ {
		s = append(s, i)
	}
	return s
}

func main() {
	const n = 1024
	a := testing.AllocsPerRun(10, func() { sinkSlice = buildAppendNil(n) })
	b := testing.AllocsPerRun(10, func() { sinkSlice = buildPrealloc(n) })
	fmt.Printf("append to nil slice (n=%d): %.0f allocs/op\n", n, a)
	fmt.Printf("make([]int, 0, n):          %.0f allocs/op\n", b)
}
```

**Output**
```
append to nil slice (n=1024): 9 allocs/op
make([]int, 0, n):          1 allocs/op
```

Nine reallocations (as the slice doubles up to 1024) collapse to **one** when you preallocate. The same
applies to `make(map[K]V, n)`.

---

## 4. `strings.Builder` vs `+=`

`s += p` in a loop allocates a brand-new string every iteration and copies everything so far — O(n²)
copying and O(n) allocations. `strings.Builder` grows one backing buffer; `Grow` sizes it once so even
that is a single allocation.

```go
package main

import (
	"fmt"
	"strings"
	"testing"
)

var sink string

func concatPlus(parts []string) string {
	var s string
	for _, p := range parts {
		s += p // each += allocates a new string and copies everything so far
	}
	return s
}

func concatBuilder(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p)
	}
	return b.String()
}

func concatBuilderGrow(parts []string) string {
	var b strings.Builder
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	b.Grow(n) // size the backing buffer once
	for _, p := range parts {
		b.WriteString(p)
	}
	return b.String()
}

func main() {
	parts := make([]string, 100)
	for i := range parts {
		parts[i] = "abcdefgh"
	}
	fmt.Printf("s += p:         %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sink = concatPlus(parts) }))
	fmt.Printf("Builder:        %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sink = concatBuilder(parts) }))
	fmt.Printf("Builder + Grow: %.0f allocs/op\n", testing.AllocsPerRun(10, func() { sink = concatBuilderGrow(parts) }))
}
```

**Output**
```
s += p:         99 allocs/op
Builder:        8 allocs/op
Builder + Grow: 1 allocs/op
```

99 → 8 → 1. `Builder.String()` returns the buffer without copying it (it uses `unsafe` internally), so
`Grow` + writes is a single allocation for the whole string.

---

## 5. `strconv.AppendInt` into a reused buffer → 0 allocs

The `Append*` family writes into a `[]byte` you already own instead of returning a fresh string. Reset
the length with `buf[:0]` (keeping the capacity) and you can format on a hot path with **zero**
allocations.

```go
package main

import (
	"fmt"
	"strconv"
	"testing"
)

var sinkB []byte
var sinkS string

func main() {
	buf := make([]byte, 0, 64)
	nums := []int64{1, 22, 333, 4444, 55555}

	// AppendInt writes decimal digits into buf's existing capacity: zero allocations.
	format := func() {
		buf = buf[:0] // reset length, keep the backing array
		for _, n := range nums {
			buf = strconv.AppendInt(buf, n, 10)
			buf = append(buf, ' ')
		}
		sinkB = buf
	}
	fmt.Printf("AppendInt into reused buf: %.0f allocs/op\n", testing.AllocsPerRun(100, format))

	// Contrast: Sprintf per number allocates (result string + boxing the int).
	viaFmt := func() {
		var s string
		for _, n := range nums {
			s += fmt.Sprintf("%d ", n)
		}
		sinkS = s
	}
	fmt.Printf("fmt.Sprintf per number:    %.0f allocs/op\n", testing.AllocsPerRun(100, viaFmt))

	fmt.Printf("result: %q\n", string(buf))
}
```

**Output**
```
AppendInt into reused buf: 0 allocs/op
fmt.Sprintf per number:    12 allocs/op
result: "1 22 333 4444 55555 "
```

Zero vs twelve. This "append into a reused buffer" pattern is the backbone of zero-allocation
serialization (revisited in #12 and the capstone #15, and again in [lesson 48](../../48-low-latency-lockfree-tail.md)).

---

Next tier: [🟡 Medium (6–10)](2-medium.md) — escape analysis, boxing, and real benchmarks.
</content>
