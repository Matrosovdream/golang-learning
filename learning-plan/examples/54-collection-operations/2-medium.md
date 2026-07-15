# Step 54 — Collection Operations · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

Maps do the heavy lifting here: **group**, **count**, **key-by**, and **sets**, plus **partition / chunk / flatten** and the `maps` package.

---

## 9. GroupBy into buckets

`🟡 medium` · *group-by*

The most common map shape: bucket a slice into `map[K][]T` keyed by a projection. Appending to a missing key just works, because a missing key's value is a `nil` slice and `append(nil, x)` is valid. Remember to **sort the keys** before printing — map order is random.

**Steps:**

1. `byDept[e.dept] = append(byDept[e.dept], e)` — no need to check if the key exists.
2. Collect the keys into a slice and `slices.Sort` them.
3. Print each group in key order.

```go
package main

import (
	"fmt"
	"slices"
)

type employee struct {
	name string
	dept string
}

func main() {
	staff := []employee{
		{"Alice", "eng"},
		{"Bob", "sales"},
		{"Carol", "eng"},
		{"Dave", "sales"},
		{"Eve", "eng"},
	}
	// GroupBy: bucket elements into a map keyed by a projection. append to a nil
	// slice value just works — the missing key reads as nil.
	byDept := map[string][]employee{}
	for _, e := range staff {
		byDept[e.dept] = append(byDept[e.dept], e)
	}

	// Map order is random — collect and sort the keys before printing.
	depts := make([]string, 0, len(byDept))
	for d := range byDept {
		depts = append(depts, d)
	}
	slices.Sort(depts)
	for _, d := range depts {
		names := make([]string, 0, len(byDept[d]))
		for _, e := range byDept[d] {
			names = append(names, e.name)
		}
		fmt.Printf("%-6s %v\n", d, names)
	}
}
```

**Output:**

```
eng    [Alice Carol Eve]
sales  [Bob Dave]
```

---

## 10. CountBy: a frequency tally

`🟡 medium` · *count-by*

Counting is group-by's simpler sibling: `map[K]int` with `m[key]++`. A missing key reads as the zero value `0`, so the increment needs no guard. Combine with sorting to rank the results.

**Steps:**

1. `counts[v]++` tallies each value.
2. Collect keys and `slices.Sort` for stable output.
3. Print aligned columns with `%-7s %d`.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	votes := []string{"go", "rust", "go", "python", "go", "rust"}
	// CountBy: a frequency tally. A missing key reads as 0, so ++ just works.
	counts := map[string]int{}
	for _, v := range votes {
		counts[v]++
	}

	// Print in a stable order: sort the keys.
	langs := make([]string, 0, len(counts))
	for l := range counts {
		langs = append(langs, l)
	}
	slices.Sort(langs)
	for _, l := range langs {
		fmt.Printf("%-7s %d\n", l, counts[l])
	}
}
```

**Output:**

```
go      3
python  1
rust    2
```

---

## 11. KeyBy: build a lookup map

`🟡 medium` · *key-by*

When you'll look items up by an id repeatedly, build a `map[K]T` once instead of scanning the slice each time — O(1) per lookup instead of O(n). Use the comma-ok form to distinguish "found the zero value" from "not present".

**Steps:**

1. `byID[u.id] = u` for each element (one row per key; later rows overwrite).
2. Look up a present key directly.
3. Look up a missing key with `u, ok := byID[9]` — `ok` is `false`, `u` is the zero value.

```go
package main

import "fmt"

type user struct {
	id   int
	name string
}

func main() {
	users := []user{
		{1, "Alice"},
		{2, "Bob"},
		{3, "Carol"},
	}
	// KeyBy: build a lookup map from a slice — O(1) access by id instead of scanning.
	byID := make(map[int]user, len(users))
	for _, u := range users {
		byID[u.id] = u
	}
	fmt.Println("user 2:", byID[2].name)

	// A missing key returns the zero value plus ok=false (the comma-ok form).
	u, ok := byID[9]
	fmt.Printf("user 9: %+v found: %v\n", u, ok)
}
```

**Output:**

```
user 2: Bob
user 9: {id:0 name:} found: false
```

---

## 12. A generic Set type

`🟡 medium` · *set*

Go has no set type — the idiom is `map[T]struct{}`, where `struct{}` costs zero bytes. Wrap it in a named generic type with `Add`/`Has`/`Slice` methods for a reusable `Set[T]`. Constructing from a variadic list collapses duplicates for free.

**Steps:**

1. `type Set[T comparable] map[T]struct{}` with `Add` and `Has` methods.
2. `NewSet(3, 1, 2, 3, 1)` — duplicates collapse, so `len` is 3.
3. `Slice()` returns the elements in random order; sort for a stable print.

```go
package main

import (
	"fmt"
	"slices"
)

// Set is the idiomatic Go set: a map with zero-size values.
type Set[T comparable] map[T]struct{}

func NewSet[T comparable](items ...T) Set[T] {
	s := make(Set[T], len(items))
	for _, v := range items {
		s.Add(v)
	}
	return s
}

func (s Set[T]) Add(v T)      { s[v] = struct{}{} }
func (s Set[T]) Has(v T) bool { _, ok := s[v]; return ok }

func (s Set[T]) Slice() []T {
	out := make([]T, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	return out
}

func main() {
	s := NewSet(3, 1, 2, 3, 1) // duplicates collapse
	fmt.Println("len:", len(s))
	fmt.Println("has 2:", s.Has(2), "has 9:", s.Has(9))

	vals := s.Slice()
	slices.Sort(vals) // Slice() order is random; sort for a stable print
	fmt.Println("sorted:", vals)
}
```

**Output:**

```
len: 3
has 2: true has 9: false
sorted: [1 2 3]
```

---

## 13. Set operations: union, intersection, difference

`🟡 medium` · *set*

The three set operations are each a small loop backed by a map. **Union** inserts everything from both; **intersection** keeps `b`'s elements that are in `a`'s set; **difference** keeps `a`'s elements *not* in `b`'s set. All run in O(n+m). (Sorting the results here is only to make the output deterministic.)

**Steps:**

1. `Union`: a set + a dedup as you insert both slices.
2. `Intersection`: build a set of `a`, keep `b`'s members of it (deduped).
3. `Difference`: build a set of `b`, keep `a`'s non-members.

```go
package main

import (
	"fmt"
	"slices"
)

// Set operations on slices, backed by a map. Results are sorted here only so the
// output is deterministic — the operations themselves don't guarantee order.
func Union[T comparable](a, b []T) []T {
	seen := map[T]struct{}{}
	var out []T
	add := func(vals []T) {
		for _, v := range vals {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	add(a)
	add(b)
	return out
}

func Intersection[T comparable](a, b []T) []T {
	inA := make(map[T]struct{}, len(a))
	for _, v := range a {
		inA[v] = struct{}{}
	}
	var out []T
	seen := map[T]struct{}{}
	for _, v := range b {
		if _, ok := inA[v]; ok {
			if _, dup := seen[v]; !dup {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
}

func Difference[T comparable](a, b []T) []T { // in a, not in b
	inB := make(map[T]struct{}, len(b))
	for _, v := range b {
		inB[v] = struct{}{}
	}
	var out []T
	for _, v := range a {
		if _, ok := inB[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}

func main() {
	a := []int{1, 2, 3, 4}
	b := []int{3, 4, 5, 6}
	u, i, d := Union(a, b), Intersection(a, b), Difference(a, b)
	slices.Sort(u)
	slices.Sort(i)
	slices.Sort(d)
	fmt.Println("union:        ", u)
	fmt.Println("intersection: ", i)
	fmt.Println("difference a-b:", d)
}
```

**Output:**

```
union:         [1 2 3 4 5 6]
intersection:  [3 4]
difference a-b: [1 2]
```

---

## 14. Partition into two slices

`🟡 medium` · *partition*

Partition splits a slice into two — the elements that match a predicate and those that don't — in a single pass. Handy when you need both halves (e.g. valid vs invalid records) rather than throwing one away like `Filter` does. Named return values keep it tidy.

**Steps:**

1. `func Partition[T any](s []T, pred func(T) bool) (yes, no []T)`.
2. One loop; append to `yes` or `no` per the predicate.
3. Naked `return` sends back both named slices.

```go
package main

import "fmt"

// Partition splits s into (yes, no) by a predicate — one pass, two slices.
func Partition[T any](s []T, pred func(T) bool) (yes, no []T) {
	for _, v := range s {
		if pred(v) {
			yes = append(yes, v)
		} else {
			no = append(no, v)
		}
	}
	return
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	evens, odds := Partition(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println("evens:", evens)
	fmt.Println("odds: ", odds)
}
```

**Output:**

```
evens: [2 4 6 8 10]
odds:  [1 3 5 7 9]
```

---

## 15. Chunk into batches

`🟡 medium` · *chunk*

Chunking splits a slice into consecutive sub-slices of a fixed size — the last one may be short. It's the shape you use to batch work (N rows per DB insert, N ids per API call). Go 1.23 ships `slices.Chunk`, but it returns a lazy iterator; the hand-written version returns a `[][]T` you can index.

**Steps:**

1. Step `i` by `n`; slice `s[i:end]` where `end = min(i+n, len(s))`.
2. `min` is a built-in (Go 1.21+).
3. The last batch here has a single element.

```go
package main

import "fmt"

// Chunk splits s into consecutive slices of at most size n (the last may be short).
// Go 1.23 also ships slices.Chunk, which returns a lazy iterator instead.
func Chunk[T any](s []T, n int) [][]T {
	if n <= 0 {
		return nil
	}
	var out [][]T
	for i := 0; i < len(s); i += n {
		end := min(i+n, len(s)) // built-in min, Go 1.21+
		out = append(out, s[i:end])
	}
	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7}
	for i, batch := range Chunk(nums, 3) {
		fmt.Printf("batch %d: %v\n", i, batch)
	}
}
```

**Output:**

```
batch 0: [1 2 3]
batch 1: [4 5 6]
batch 2: [7]
```

---

## 16. Flatten a slice of slices

`🟡 medium` · *flatten*

Flattening a `[][]T` into a `[]T` is one `append(out, row...)` per row — the `...` spreads each inner slice's elements. Empty inner slices just contribute nothing. For a fixed, known set of slices, `slices.Concat` does it in a single call.

**Steps:**

1. Loop the outer slice; `flat = append(flat, row...)`.
2. Empty inner slices (`{}`) add nothing.
3. `slices.Concat(a, b, c, ...)` is the built-in equivalent for a fixed argument list.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	matrix := [][]int{{1, 2}, {3, 4, 5}, {}, {6}}
	// Manual flatten: spread each inner slice into the output with append(..., row...).
	var flat []int
	for _, row := range matrix {
		flat = append(flat, row...)
	}
	fmt.Println("manual:", flat)

	// slices.Concat (Go 1.22+) joins a fixed set of slices in one call.
	flat2 := slices.Concat(matrix[0], matrix[1], matrix[2], matrix[3])
	fmt.Println("concat:", flat2)
}
```

**Output:**

```
manual: [1 2 3 4 5 6]
concat: [1 2 3 4 5 6]
```

---

## 17. The maps toolbox

`🟡 medium` · *maps*

The `maps` package mirrors `slices` for maps. `Clone` makes a shallow copy; `Equal` compares by keys and values; `Keys`/`Values` return **iterators** (Go 1.23+) that you collect or sort. `slices.Sorted(maps.Keys(m))` is the one-liner for "give me the keys in order" — the standard fix for random map iteration.

**Steps:**

1. `maps.Clone` copies; mutating the copy leaves the original alone.
2. `maps.Equal` reports the two now differ.
3. `slices.Sorted(maps.Keys(prices))` collects and sorts the keys.

```go
package main

import (
	"fmt"
	"maps"
	"slices"
)

func main() {
	prices := map[string]int{"apple": 3, "banana": 2, "cherry": 5}

	// maps.Clone makes a shallow copy — mutating the copy leaves the original alone.
	cp := maps.Clone(prices)
	cp["apple"] = 99
	fmt.Println("orig apple:", prices["apple"], "copy apple:", cp["apple"])

	// maps.Equal compares two maps by keys and values.
	fmt.Println("equal:", maps.Equal(prices, cp))

	// maps.Keys returns an ITERATOR (Go 1.23+); slices.Sorted collects+sorts it.
	keys := slices.Sorted(maps.Keys(prices))
	fmt.Println("keys: ", keys)
}
```

**Output:**

```
orig apple: 3 copy apple: 99
equal: false
keys:  [apple banana cherry]
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
