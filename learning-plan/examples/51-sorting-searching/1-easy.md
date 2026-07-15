# Step 51 — Sorting & Searching · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

Uses the generic **`slices`** and **`cmp`** packages (Go 1.21+).

---

## 1. Sort a slice with slices.Sort

`🟢 easy` · *slices*

The modern default: `slices.Sort` sorts any slice of ordered values (ints, strings, floats…) in place, ascending. One call, no comparator, no boilerplate — this is what you reach for first in new code.

**Steps:**

1. `import "slices"`.
2. `slices.Sort(nums)` sorts in place — it mutates the slice.
3. Strings sort lexicographically the same way.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	// slices.Sort (Go 1.21+) sorts any slice of ordered values in place, ascending.
	nums := []int{5, 2, 8, 1, 9, 3}
	slices.Sort(nums)
	fmt.Println("ints:", nums)

	words := []string{"banana", "apple", "cherry"}
	slices.Sort(words) // strings sort lexicographically
	fmt.Println("strings:", words)
}
```

**Output:**

```
ints: [1 2 3 5 8 9]
strings: [apple banana cherry]
```

---

## 2. Sort in reverse (descending)

`🟢 easy` · *slices*

There's no `SortDescending`, and you don't need one. The simplest way is sort ascending then `slices.Reverse`. If you'd rather sort directly, pass a comparator that flips the operands — `cmp.Compare(b, a)` instead of `(a, b)`.

**Steps:**

1. `slices.Sort` then `slices.Reverse` for a quick descending order.
2. Or `slices.SortFunc` with `cmp.Compare(b, a)` to sort descending in one pass.
3. Prefer `cmp.Compare` over `b - a` — the subtraction can overflow.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

func main() {
	nums := []int{5, 2, 8, 1, 9, 3}
	// Easiest descending sort: sort ascending, then reverse in place.
	slices.Sort(nums)
	slices.Reverse(nums)
	fmt.Println("descending:", nums)

	// Or sort directly with a comparator that flips the operands.
	more := []int{5, 2, 8, 1}
	slices.SortFunc(more, func(a, b int) int { return cmp.Compare(b, a) })
	fmt.Println("via SortFunc:", more)
}
```

**Output:**

```
descending: [9 8 5 3 2 1]
via SortFunc: [8 5 2 1]
```

---

## 3. Sort structs by a field

`🟢 easy` · *Comparator*

To sort anything that isn't a plain ordered value, use `slices.SortFunc` with a comparator that returns a **negative / zero / positive** `int`. `cmp.Compare(a.field, b.field)` produces exactly that ordering for any comparable field.

**Steps:**

1. Write a comparator `func(a, b person) int`.
2. Return `cmp.Compare(a.age, b.age)` to order by age ascending.
3. Flip the operands (`b.age, a.age`) for descending.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type person struct {
	name string
	age  int
}

func main() {
	people := []person{
		{"Alice", 30},
		{"Bob", 25},
		{"Carol", 35},
	}
	// SortFunc takes a comparator returning <0, 0, >0. cmp.Compare does the right
	// thing for any ordered type — here we order by age ascending.
	slices.SortFunc(people, func(a, b person) int {
		return cmp.Compare(a.age, b.age)
	})
	for _, p := range people {
		fmt.Printf("%s (%d)\n", p.name, p.age)
	}
}
```

**Output:**

```
Bob (25)
Alice (30)
Carol (35)
```

---

## 4. Multi-key sort with cmp.Or

`🟢 easy` · *Comparator*

Real sorts often have tiebreakers: order by team, then by score, then by name. `cmp.Or` returns its **first non-zero argument**, so you list the comparisons in priority order and it falls through ties automatically — no nested `if`.

**Steps:**

1. List comparisons in priority order inside `cmp.Or(...)`.
2. Flip operands on any key you want descending (here `score`).
3. The first key that distinguishes two elements decides their order.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type player struct {
	team  string
	score int
	name  string
}

func main() {
	players := []player{
		{"red", 10, "Zoe"},
		{"blue", 10, "Amy"},
		{"red", 20, "Bob"},
		{"blue", 10, "Cy"},
	}
	// cmp.Or returns the first non-zero comparison: sort by team, then score
	// DESC, then name. It replaces a nest of if-else in the comparator.
	slices.SortFunc(players, func(a, b player) int {
		return cmp.Or(
			cmp.Compare(a.team, b.team),
			cmp.Compare(b.score, a.score), // higher score first
			cmp.Compare(a.name, b.name),
		)
	})
	for _, p := range players {
		fmt.Printf("%-4s %2d %s\n", p.team, p.score, p.name)
	}
}
```

**Output:**

```
blue 10 Amy
blue 10 Cy
red  20 Bob
red  10 Zoe
```

---

## 5. Stable sort

`🟢 easy` · *Stability*

A plain `slices.SortFunc` may reorder elements that compare **equal**. `slices.SortStableFunc` guarantees equal elements keep their **original relative order** — essential when you sort by one key and want a prior ordering preserved within ties.

**Steps:**

1. Sort these items by `rank` only.
2. Because it's stable, the rank-1 group stays in input order `a, c, e`.
3. Swap in `SortFunc` and the order within a rank becomes unspecified.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type item struct {
	name string
	rank int
}

func main() {
	items := []item{
		{"a", 1},
		{"b", 2},
		{"c", 1},
		{"d", 2},
		{"e", 1},
	}
	// SortStableFunc keeps equal elements in their ORIGINAL order. Sorting by
	// rank, the rank-1 group stays a, c, e and the rank-2 group stays b, d.
	slices.SortStableFunc(items, func(x, y item) int {
		return cmp.Compare(x.rank, y.rank)
	})
	for _, it := range items {
		fmt.Printf("%s:%d ", it.name, it.rank)
	}
	fmt.Println()
}
```

**Output:**

```
a:1 c:1 e:1 b:2 d:2 
```

---

## 6. Binary search a sorted slice

`🟢 easy` · *Search*

Binary search finds a value in O(log n) — but only on a **sorted** slice. `slices.BinarySearch` returns `(index, found)`. When the value is absent, `index` is where it *would* go to keep the slice sorted, so you can insert there with `slices.Insert`.

**Steps:**

1. Search a value that's present — get its index and `true`.
2. Search an absent value — `found` is `false` and the index is the insertion point.
3. `slices.Insert(nums, i, 6)` keeps the slice sorted.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	// BinarySearch needs a SORTED slice. It returns the index and whether the
	// target was found; if not found, the index is where it WOULD be inserted.
	nums := []int{1, 3, 5, 7, 9}

	i, found := slices.BinarySearch(nums, 7)
	fmt.Println("find 7:", i, found)

	i, found = slices.BinarySearch(nums, 6)
	fmt.Println("find 6:", i, found) // not found -> insertion point 3

	// Use the insertion point to keep a slice sorted on insert.
	nums = slices.Insert(nums, i, 6)
	fmt.Println("after insert:", nums)
}
```

**Output:**

```
find 7: 3 true
find 6: 3 false
after insert: [1 3 5 6 7 9]
```

---

## 7. Handy slices helpers

`🟢 easy` · *slices*

Beyond sorting, the `slices` package replaces a lot of hand-written loops: `Contains`, `Index`, `Max`, `Min`, `IsSorted`, and more. Knowing they exist saves you writing (and testing) the same three-line loops over and over.

**Steps:**

1. `Contains` / `Index` answer membership and position on an **unsorted** slice (linear scan).
2. `Max` / `Min` scan for extremes; `IsSorted` checks order.
3. After `Sort`, `IsSorted` returns `true`.

```go
package main

import (
	"fmt"
	"slices"
)

func main() {
	nums := []int{4, 2, 7, 1, 9, 2}
	fmt.Println("contains 7:", slices.Contains(nums, 7))
	fmt.Println("index of 7:", slices.Index(nums, 7))
	fmt.Println("max:", slices.Max(nums), "min:", slices.Min(nums))
	fmt.Println("is sorted:", slices.IsSorted(nums))

	slices.Sort(nums)
	fmt.Println("sorted:", nums, "now sorted?", slices.IsSorted(nums))
}
```

**Output:**

```
contains 7: true
index of 7: 2
max: 9 min: 1
is sorted: false
sorted: [1 2 2 4 7 9] now sorted? true
```

---

## 8. The classic sort package

`🟢 easy` · *sort*

Before Go 1.21 — and still throughout existing codebases — sorting goes through the `sort` package: `sort.Ints`/`sort.Strings`/`sort.Float64s` for basic slices, and `sort.Slice` with a **`less` function** (returning a `bool`, not an `int`) for everything custom. You'll read this everywhere, so recognise it.

**Steps:**

1. `sort.Ints(nums)` sorts an `[]int` in place.
2. `sort.Slice(s, less)` takes `func(i, j int) bool` comparing by **index**.
3. Note the `bool` here vs the `int` comparator of `slices.SortFunc`.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	// Before Go 1.21 (and still everywhere in existing code) you use the sort
	// package: sort.Ints/Strings/Float64s for basic slices...
	nums := []int{5, 2, 8, 1}
	sort.Ints(nums)
	fmt.Println("sort.Ints:", nums)

	// ...and sort.Slice with a less function for anything custom.
	words := []string{"bb", "a", "ccc"}
	sort.Slice(words, func(i, j int) bool {
		return len(words[i]) < len(words[j]) // by length
	})
	fmt.Println("by length:", words)
}
```

**Output:**

```
sort.Ints: [1 2 5 8]
by length: [a bb ccc]
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
