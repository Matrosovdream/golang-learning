# Step 54 — Collection Operations · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

Composing the toolkit into real pipelines, plus Go 1.23 **iterators** (`iter.Seq`, range-over-func, `slices.Collect`) for **lazy** filter/map chains.

---

## 18. A Filter → Map → Reduce pipeline

`🔴 hard` · *pipeline*

The three core helpers compose into a pipeline that reads top-to-bottom: keep the paid orders, project their amounts, sum them. Each stage is independently testable, and the intent is obvious. (For hot paths you'd fuse this into one loop; for clarity, staged wins.)

**Steps:**

1. `Filter` orders where `paid` is true.
2. `Map` each surviving order to its `amount`.
3. `Reduce` the amounts to a total; format cents as dollars.

```go
package main

import "fmt"

type order struct {
	id     int
	paid   bool
	amount int // cents
}

func Filter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func Reduce[T, U any](s []T, init U, f func(U, T) U) U {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

func main() {
	orders := []order{
		{1, true, 1200},
		{2, false, 800},
		{3, true, 4300},
		{4, true, 1550},
		{5, false, 999},
	}
	// A three-stage pipeline: keep paid -> take amounts -> sum.
	paid := Filter(orders, func(o order) bool { return o.paid })
	amounts := Map(paid, func(o order) int { return o.amount })
	total := Reduce(amounts, 0, func(acc, a int) int { return acc + a })

	fmt.Printf("paid orders: %d\n", len(paid))
	fmt.Printf("revenue: $%.2f\n", float64(total)/100)
}
```

**Output:**

```
paid orders: 3
revenue: $70.50
```

---

## 19. Group and aggregate into a sorted report

`🔴 hard` · *group + aggregate*

The everyday reporting shape: group by a key while aggregating (here, sum amounts per customer straight into the map value), then move to a slice of rows to sort. `cmp.Or` gives a clean multi-key order — total descending, then name ascending as a tiebreak.

**Steps:**

1. `totals[s.customer] += s.amount` — group and sum in one step.
2. Copy the map into `[]row` so it can be sorted.
3. `slices.SortFunc` + `cmp.Or(cmp.Compare(b.total, a.total), cmp.Compare(a.customer, b.customer))`.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type sale struct {
	customer string
	amount   int
}

func main() {
	sales := []sale{
		{"acme", 100}, {"globex", 250}, {"acme", 300},
		{"initech", 75}, {"globex", 125}, {"acme", 50},
	}
	// Group by customer, summing straight into the map value.
	totals := map[string]int{}
	for _, s := range sales {
		totals[s.customer] += s.amount
	}

	// Move to a slice of rows so we can sort — by total DESC, then name.
	type row struct {
		customer string
		total    int
	}
	rows := make([]row, 0, len(totals))
	for c, t := range totals {
		rows = append(rows, row{c, t})
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(
			cmp.Compare(b.total, a.total), // total DESC
			cmp.Compare(a.customer, b.customer),
		)
	})
	for _, r := range rows {
		fmt.Printf("%-8s %d\n", r.customer, r.total)
	}
}
```

**Output:**

```
acme     450
globex   375
initech  75
```

---

## 20. UniqueBy: dedup structs by a key

`🔴 hard` · *dedup*

Deduping structs needs a key function — you can't compare whole structs when only one field defines identity. `UniqueBy[T, K]` keeps the **first** element seen for each key. Two type parameters: `T` for the element, `K comparable` for the key.

**Steps:**

1. `func UniqueBy[T any, K comparable](s []T, key func(T) K) []T`.
2. Track seen keys in a set; append only the first sighting.
3. Later rows for user 1 and user 2 are dropped.

```go
package main

import "fmt"

type event struct {
	userID int
	action string
}

// UniqueBy keeps the FIRST element seen for each distinct key.
func UniqueBy[T any, K comparable](s []T, key func(T) K) []T {
	seen := map[K]struct{}{}
	out := make([]T, 0, len(s))
	for _, v := range s {
		k := key(v)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

func main() {
	events := []event{
		{1, "login"},
		{2, "login"},
		{1, "click"}, // duplicate user 1 — dropped
		{3, "login"},
		{2, "logout"}, // duplicate user 2 — dropped
	}
	for _, e := range UniqueBy(events, func(e event) int { return e.userID }) {
		fmt.Printf("user %d: %s\n", e.userID, e.action)
	}
}
```

**Output:**

```
user 1: login
user 2: login
user 3: login
```

---

## 21. Max and min by a projection

`🔴 hard` · *slices*

`slices.Max`/`Min` need an *ordered element type*; for structs you want `MaxFunc`/`MinFunc`, which take a comparator and return the **element** that's largest/smallest by it. This is "argmax" — the priciest product, the newest record, the closest point.

**Steps:**

1. `slices.MaxFunc(products, cmp-by-price)` returns the whole priciest struct.
2. `slices.MinFunc` returns the cheapest.
3. Both **panic on an empty slice** — guard `len == 0` in real code.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type product struct {
	name  string
	price int // cents
}

func main() {
	products := []product{
		{"pen", 150},
		{"notebook", 450},
		{"eraser", 75},
		{"stapler", 900},
	}
	// MaxFunc/MinFunc return the ELEMENT that is largest/smallest by a comparator
	// (unlike slices.Max, which needs an ordered element type).
	priciest := slices.MaxFunc(products, func(a, b product) int {
		return cmp.Compare(a.price, b.price)
	})
	cheapest := slices.MinFunc(products, func(a, b product) int {
		return cmp.Compare(a.price, b.price)
	})
	fmt.Printf("priciest: %-8s $%.2f\n", priciest.name, float64(priciest.price)/100)
	fmt.Printf("cheapest: %-8s $%.2f\n", cheapest.name, float64(cheapest.price)/100)
}
```

**Output:**

```
priciest: stapler  $9.00
cheapest: eraser   $0.75
```

---

## 22. Zip two slices

`🔴 hard` · *zip*

Zip pairs up two slices element-by-element, stopping at the shorter one. It needs a generic `pair[A, B]` struct since Go has no tuple type. The common downstream move is folding the pairs into a `map[A]B`.

**Steps:**

1. `pair[A, B any]` struct + `Zip[A, B any](as []A, bs []B) []pair[A, B]`.
2. Iterate to `min(len(as), len(bs))`.
3. Fold the pairs into a `map[string]int` for lookup.

```go
package main

import "fmt"

type pair[A, B any] struct {
	First  A
	Second B
}

// Zip pairs elements up to the length of the shorter slice.
func Zip[A, B any](as []A, bs []B) []pair[A, B] {
	n := min(len(as), len(bs))
	out := make([]pair[A, B], n)
	for i := 0; i < n; i++ {
		out[i] = pair[A, B]{as[i], bs[i]}
	}
	return out
}

func main() {
	names := []string{"Alice", "Bob", "Carol"}
	ages := []int{30, 25, 35}
	for _, p := range Zip(names, ages) {
		fmt.Printf("%s is %d\n", p.First, p.Second)
	}

	// Zipping keys+values into a map is a common variant.
	m := make(map[string]int, len(names))
	for _, p := range Zip(names, ages) {
		m[p.First] = p.Second
	}
	fmt.Println("Bob:", m["Bob"])
}
```

**Output:**

```
Alice is 30
Bob is 25
Carol is 35
Bob: 25
```

---

## 23. Emit a map deterministically

`🔴 hard` · *ordering*

Map iteration order is random, so any output built from a map must impose an order. Two idioms: `slices.Sorted(maps.Keys(m))` for key order, and `slices.Collect(maps.Keys(m))` + `SortFunc` to sort by **value** (with a name tiebreak via `cmp.Or`). Both are collect-then-sort over an iterator.

**Steps:**

1. `slices.Sorted(maps.Keys(stock))` → keys ascending.
2. `slices.Collect(maps.Keys(stock))` then `SortFunc` by `stock[b]` vs `stock[a]` → value DESC.
3. `cmp.Or` adds the name tiebreak so equal quantities (banana, date = 4) are stable.

```go
package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
)

func main() {
	stock := map[string]int{"apple": 12, "banana": 4, "cherry": 30, "date": 4}

	// maps.Keys is an iterator; slices.Sorted collects+sorts it in one step.
	fmt.Println("by name:")
	for _, name := range slices.Sorted(maps.Keys(stock)) {
		fmt.Printf("  %-7s %d\n", name, stock[name])
	}

	// Sort by VALUE (qty) DESC, then name: collect keys, then SortFunc.
	fmt.Println("by qty desc:")
	names := slices.Collect(maps.Keys(stock))
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(stock[b], stock[a]), cmp.Compare(a, b))
	})
	for _, name := range names {
		fmt.Printf("  %-7s %d\n", name, stock[name])
	}
}
```

**Output:**

```
by name:
  apple   12
  banana  4
  cherry  30
  date    4
by qty desc:
  cherry  30
  apple   12
  banana  4
  date    4
```

---

## 24. Lazy filter/map with iterators

`🔴 hard` · *iterators*

An `iter.Seq[T]` is just `func(yield func(T) bool)` — Go 1.23's range-over-func drives it. You can wrap a sequence to build **lazy** `FilterSeq`/`MapSeq` that do no work until consumed. `slices.Values` makes a slice into a sequence; `slices.Collect` materializes one back.

**Steps:**

1. `FilterSeq`/`MapSeq` each return a new `iter.Seq` closure that ranges the upstream.
2. Compose: `slices.Values(nums)` → filter evens → map square. Nothing has run yet.
3. Consume with `for v := range squared`, then again with `slices.Collect`.

```go
package main

import (
	"fmt"
	"iter"
	"slices"
)

// FilterSeq wraps a sequence, yielding only elements that pass keep. It's LAZY:
// nothing runs until the returned Seq is ranged over.
func FilterSeq[T any](seq iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

// MapSeq transforms each yielded element.
func MapSeq[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8}
	// slices.Values turns a slice into an iter.Seq; compose filter then map lazily.
	evens := FilterSeq(slices.Values(nums), func(n int) bool { return n%2 == 0 })
	squared := MapSeq(evens, func(n int) int { return n * n })

	// Consume with range-over-func (Go 1.23+)...
	for v := range squared {
		fmt.Print(v, " ")
	}
	fmt.Println()

	// ...or materialize the sequence back to a slice with slices.Collect.
	fmt.Println(slices.Collect(squared))
}
```

**Output:**

```
4 16 36 64 
[4 16 36 64]
```

---

## 25. Laziness: take from an infinite sequence

`🔴 hard` · *iterators*

The payoff of laziness: a `Take(n)` that stops after `n` elements halts the **upstream** too — so you can pull a finite prefix from an *infinite* generator, and a producer only ever runs as many steps as the consumer asks for. When `Take`'s `yield` returns `false`, the range-over-func machinery propagates the stop back up the chain.

**Steps:**

1. `naturals()` yields `1, 2, 3, …` forever — safe only because it's lazy.
2. `Take(naturals(), 5)` pulls just the first five.
3. A `loud` producer counts its runs; `Take(loud, 3)` proves it ran exactly 3 times.

```go
package main

import (
	"fmt"
	"iter"
	"slices"
)

// Take yields at most n elements, then stops — which halts the upstream too.
func Take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		count := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			count++
			if count >= n {
				return
			}
		}
	}
}

func naturals() iter.Seq[int] { // an INFINITE sequence — fine, because it's lazy
	return func(yield func(int) bool) {
		for i := 1; ; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func main() {
	// Laziness lets us take a finite prefix of an infinite sequence.
	fmt.Println("first 5:", slices.Collect(Take(naturals(), 5)))

	// A producer that counts how many elements it actually generates proves the
	// pipeline only pulls what the consumer asks for.
	produced := 0
	loud := func(yield func(int) bool) {
		for i := 1; ; i++ {
			produced++
			if !yield(i * 10) {
				return
			}
		}
	}
	got := slices.Collect(Take(loud, 3))
	fmt.Println("got:", got, "| producer ran", produced, "times")
}
```

**Output:**

```
first 5: [1 2 3 4 5]
got: [10 20 30] | producer ran 3 times
```

---

## 26. Capstone: an orders report

`🔴 hard` · *capstone*

Everything at once: **filter** to paid orders, **group + aggregate** per customer into running summaries (storing `*Summary` so you mutate in place), **collect + sort** by total descending, and print a formatted table. This is the shape of a real "sales by customer" report endpoint.

**Steps:**

1. Filter `Status == "paid"`.
2. Fold into `map[string]*Summary`, bumping `Count` and `Total`.
3. Collect to `[]*Summary`, `SortFunc` by total DESC then name, print with `%` width verbs.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type Order struct {
	ID       int
	Customer string
	Status   string
	Amount   int // cents
}

type Summary struct {
	Customer string
	Count    int
	Total    int
}

func main() {
	orders := []Order{
		{1, "acme", "paid", 1200},
		{2, "globex", "paid", 4500},
		{3, "acme", "refunded", 800},
		{4, "acme", "paid", 3300},
		{5, "initech", "paid", 500},
		{6, "globex", "pending", 999},
		{7, "globex", "paid", 1500},
		{8, "acme", "paid", 700},
	}

	// 1. FILTER: only paid orders.
	paid := make([]Order, 0, len(orders))
	for _, o := range orders {
		if o.Status == "paid" {
			paid = append(paid, o)
		}
	}

	// 2. GROUP + AGGREGATE: fold into a map of running per-customer summaries.
	//    Storing *Summary lets us mutate the accumulator in place.
	agg := map[string]*Summary{}
	for _, o := range paid {
		s := agg[o.Customer]
		if s == nil {
			s = &Summary{Customer: o.Customer}
			agg[o.Customer] = s
		}
		s.Count++
		s.Total += o.Amount
	}

	// 3. COLLECT + SORT: by total DESC, then customer.
	rows := make([]*Summary, 0, len(agg))
	for _, s := range agg {
		rows = append(rows, s)
	}
	slices.SortFunc(rows, func(a, b *Summary) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(a.Customer, b.Customer))
	})

	// 4. REPORT.
	fmt.Printf("%-8s %6s %9s %8s\n", "CUSTOMER", "ORDERS", "TOTAL", "AVG")
	for _, s := range rows {
		total := float64(s.Total) / 100
		avg := total / float64(s.Count)
		fmt.Printf("%-8s %6d %9.2f %8.2f\n", s.Customer, s.Count, total, avg)
	}
}
```

**Output:**

```
CUSTOMER ORDERS     TOTAL      AVG
globex        2     60.00    30.00
acme          3     52.00    17.33
initech       1      5.00     5.00
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
