# Step 54 — Collection Operations · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

The three core shapes — **filter**, **map**, **reduce** — first as plain loops, then as generic helpers, plus the `slices` toolbox.

---

## 1. Filter a slice

`🟢 easy` · *filter*

Go has **no built-in `filter`**. The idiomatic version is a loop that appends what you want to keep into a new slice. If you don't need the original, you can filter **in place** by reusing the backing array via `s[:0]` — safe because you only ever write at an index at or behind the one you're reading.

**Steps:**

1. Build a new slice, appending elements that pass the test.
2. Or filter in place: `kept := nums[:0]`, then append the keepers back.
3. Both produce the same evens here.

```go
package main

import "fmt"

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8}
	// Go has no built-in Filter: build a NEW slice, appending what you keep.
	var evens []int
	for _, n := range nums {
		if n%2 == 0 {
			evens = append(evens, n)
		}
	}
	fmt.Println("evens:", evens)

	// Filter IN PLACE (no new allocation): reuse the backing array via nums[:0].
	// Safe because we only ever write at an index <= the one we're reading.
	kept := nums[:0]
	for _, n := range nums {
		if n%2 == 0 {
			kept = append(kept, n)
		}
	}
	fmt.Println("kept: ", kept)
}
```

**Output:**

```
evens: [2 4 6 8]
kept:  [2 4 6 8]
```

---

## 2. Map (transform) a slice

`🟢 easy` · *map*

"Map" here means transform every element into a new one. There's no built-in for it either — preallocate the output with `make` and assign by index (or `append` into a `cap`-sized slice). The output type can differ from the input type.

**Steps:**

1. `make([]int, len(nums))` then assign `doubled[i] = n * 2` — same length, index-for-index.
2. Transform `int → string` with `strconv.Itoa` into a `cap`-preallocated slice.
3. Preallocating avoids repeated `append` growth.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	nums := []int{1, 2, 3, 4}
	// Map = transform each element into a new slice. Preallocate the exact length.
	doubled := make([]int, len(nums))
	for i, n := range nums {
		doubled[i] = n * 2
	}
	fmt.Println("doubled:", doubled)

	// The output type can differ from the input type (int -> string).
	labels := make([]string, 0, len(nums))
	for _, n := range nums {
		labels = append(labels, "#"+strconv.Itoa(n))
	}
	fmt.Println("labels: ", labels)
}
```

**Output:**

```
doubled: [2 4 6 8]
labels:  [#1 #2 #3 #4]
```

---

## 3. Reduce: sum, product, max

`🟢 easy` · *reduce*

"Reduce" (fold) collapses a slice into a single value using an accumulator. The classic trio: sum (seed `0`), product (seed `1`), and max — which you must seed with the **first element**, not `0`, or an all-negative slice gives the wrong answer.

**Steps:**

1. Sum: accumulator starts at `0`, add each element.
2. Product: accumulator starts at `1`.
3. Max: seed with `nums[0]`, scan `nums[1:]`.

```go
package main

import "fmt"

func main() {
	nums := []int{3, 1, 4, 1, 5, 9, 2, 6}
	// Reduce = fold a slice into a single value with an accumulator.
	sum := 0
	for _, n := range nums {
		sum += n
	}
	product := 1
	for _, n := range nums {
		product *= n
	}
	// Seed max with the FIRST element, not 0 (0 would be wrong for all-negative input).
	max := nums[0]
	for _, n := range nums[1:] {
		if n > max {
			max = n
		}
	}
	fmt.Println("sum:", sum, "product:", product, "max:", max)
}
```

**Output:**

```
sum: 31 product: 6480 max: 9
```

---

## 4. A generic Filter

`🟢 easy` · *generics*

When the same filter loop repeats, extract a generic helper. `Filter[T any]` takes a slice and a `keep` predicate and returns a new slice. One type parameter is enough — input and output element types are the same.

**Steps:**

1. `func Filter[T any](s []T, keep func(T) bool) []T`.
2. Preallocate with `make([]T, 0, len(s))` and append the keepers.
3. Call it with different element types and predicates.

```go
package main

import "fmt"

// Filter returns a new slice with the elements for which keep returns true.
func Filter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6}
	evens := Filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println("evens:", evens)

	words := []string{"go", "rust", "c", "python"}
	short := Filter(words, func(w string) bool { return len(w) <= 2 })
	fmt.Println("short:", short)
}
```

**Output:**

```
evens: [2 4 6]
short: [go c]
```

---

## 5. A generic Map (two type params)

`🟢 easy` · *generics*

`Map` needs **two** type parameters, `[T, U any]`, because the result element type `U` can differ from the input type `T`. That's why the stdlib doesn't ship a generic `Map` for slices — but it's a handful of lines to write. Bonus: you can pass a matching stdlib function like `strings.ToUpper` directly.

**Steps:**

1. `func Map[T, U any](s []T, f func(T) U) []U`.
2. Preallocate `make([]U, len(s))` and assign by index.
3. Pass a `func(int) int` and, separately, `strings.ToUpper` (a `func(string) string`) by name.

```go
package main

import (
	"fmt"
	"strings"
)

// Map applies f to every element, returning a slice of the (possibly new) type U.
func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func main() {
	nums := []int{1, 2, 3}
	squares := Map(nums, func(n int) int { return n * n })
	fmt.Println("squares:", squares)

	words := []string{"go", "is", "fun"}
	upper := Map(words, strings.ToUpper) // pass a matching func by name
	fmt.Println("upper:  ", upper)
}
```

**Output:**

```
squares: [1 4 9]
upper:   [GO IS FUN]
```

---

## 6. A generic Reduce

`🟢 easy` · *generics*

`Reduce[T, U any]` folds a `[]T` into a single `U`, starting from `init`. The accumulator type `U` can be anything — including a `map`, which turns reduce into a frequency counter. This is the most general of the three: filter and map can both be written in terms of reduce.

**Steps:**

1. `func Reduce[T, U any](s []T, init U, f func(acc U, v T) U) U`.
2. Fold into an `int` for a sum.
3. Fold into a `map[string]int` for a count — the accumulator type differs from the element type.

```go
package main

import "fmt"

// Reduce folds s into a single accumulator, starting from init. The accumulator
// type U can differ from the element type T.
func Reduce[T, U any](s []T, init U, f func(acc U, v T) U) U {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

func main() {
	nums := []int{1, 2, 3, 4, 5}
	sum := Reduce(nums, 0, func(acc, n int) int { return acc + n })
	fmt.Println("sum:", sum)

	// Fold into a map instead of a number: a frequency count.
	words := []string{"a", "b", "a", "c", "b", "a"}
	counts := Reduce(words, map[string]int{}, func(acc map[string]int, w string) map[string]int {
		acc[w]++
		return acc
	})
	fmt.Println("counts:", counts) // fmt prints maps in sorted-key order
}
```

**Output:**

```
sum: 15
counts: map[a:3 b:2 c:1]
```

---

## 7. The slices toolbox

`🟢 easy` · *slices*

Before writing a loop, check whether the `slices` package already has it. `Contains`/`Index` answer membership and position on an unsorted slice; the `*Func` variants take a predicate; `Max`/`Min` scan for extremes; `Equal` compares element-by-element. Knowing these exist saves writing and testing the same three-liners.

**Steps:**

1. `Contains` / `IndexFunc` / `ContainsFunc` for membership and search.
2. `Max` / `Min` for extremes (need an ordered element type).
3. `Equal` compares two slices for equal length and elements.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	nums := []int{4, 2, 7, 1, 9}
	// The slices package ships the loops you'd otherwise hand-write:
	fmt.Println("Contains 7:      ", slices.Contains(nums, 7))
	fmt.Println("IndexFunc >5:    ", slices.IndexFunc(nums, func(n int) bool { return n > 5 }))
	fmt.Println("ContainsFunc even:", slices.ContainsFunc(nums, func(n int) bool { return n%2 == 0 }))
	fmt.Println("Max:", slices.Max(nums), "Min:", slices.Min(nums))
	fmt.Println("Equal:", slices.Equal([]int{1, 2, 3}, []int{1, 2, 3}))
}
```

**Output:**

```
Contains 7:       true
IndexFunc >5:     2
ContainsFunc even: true
Max: 9 Min: 1
Equal: true
```

---

## 8. Deduplicate a slice

`🟢 easy` · *dedup*

Two ways to remove duplicates. `slices.Compact` is fast but only drops **consecutive** equal elements — so you sort a clone first for a full dedup (losing the original order). To keep **first-seen order**, track seen values in a `map[T]struct{}` set.

**Steps:**

1. `slices.Clone` (so you don't mutate the caller), `Sort`, then `Compact` → sorted unique.
2. Order-preserving: a set + a loop that appends only the first sighting of each value.
3. Compare the two orderings in the output.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	nums := []int{3, 1, 2, 1, 3, 2, 1}
	// slices.Compact removes CONSECUTIVE duplicates — so sort a clone first for a
	// full dedup (Clone avoids mutating the caller's slice).
	sorted := slices.Clone(nums)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	fmt.Println("sorted+compact: ", sorted)

	// To dedup while PRESERVING first-seen order, track seen keys in a set.
	seen := map[int]struct{}{}
	var uniq []int
	for _, n := range nums {
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			uniq = append(uniq, n)
		}
	}
	fmt.Println("order-preserving:", uniq)
}
```

**Output:**

```
sorted+compact:  [1 2 3]
order-preserving: [3 1 2]
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
