# Step 17 — Generics · 🟡 Medium

Examples **11–28**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 11. Min and Max with an Ordered constraint

`🟡 medium` · *Constraints*

An `Ordered` constraint unions every type that supports `<` and `>`. With it, one `Max`/`Min` serves ints, floats, and strings alike.

**Steps:**

1. The union lists each ordered built-in, each with `~`.
2. `Max`/`Min` use `>`/`<`, allowed by the constraint.
3. Strings compare lexicographically.

```go
package main

import "fmt"

// Ordered lists every type that supports < and >. The ~ admits named types.
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 | ~string
}

func Max[T Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Min[T Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func main() {
	fmt.Println(Max(3, 9))
	fmt.Println(Min(3, 9))
	fmt.Println(Max("apple", "banana"))
}
```

**Output:**

```
9
3
banana
```

---

## 12. Reduce fold a slice

`🟡 medium` · *Algorithms*

`Reduce` (a.k.a. fold) collapses a slice into a single value. The accumulator type `U` is independent of the element type `T`.

**Steps:**

1. `init U` seeds the accumulator.
2. `f(acc, v)` combines the running value with each element.
3. Works for numeric sums and string concatenation alike.

```go
package main

import "fmt"

// Reduce folds a slice into a single accumulated value of type U.
func Reduce[T, U any](s []T, init U, f func(U, T) U) U {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

func main() {
	nums := []int{1, 2, 3, 4}
	sum := Reduce(nums, 0, func(acc, n int) int { return acc + n })
	concat := Reduce([]string{"a", "b", "c"}, "", func(acc, s string) string { return acc + s })
	fmt.Println(sum)
	fmt.Println(concat)
}
```

**Output:**

```
10
abc
```

---

## 13. Values from a map

`🟡 medium` · *Maps*

The mirror of `Keys`: collect a map's values. Order is unspecified, so sort for a deterministic result.

**Steps:**

1. `[K comparable, V any]` — values can be any type.
2. Sort the result before printing.

```go
package main

import (
	"fmt"
	"sort"
)

// Values returns a map's values in an arbitrary order.
func Values[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func main() {
	m := map[string]int{"a": 3, "b": 1, "c": 2}
	vals := Values(m)
	sort.Ints(vals)
	fmt.Println(vals)
}
```

**Output:**

```
[1 2 3]
```

---

## 14. IndexOf with comparable

`🟡 medium` · *Algorithms*

`IndexOf` returns the position of the first match, or `-1`. Comparing elements requires the `comparable` constraint.

**Steps:**

1. `[T comparable]` enables `v == target`.
2. Return `-1` as the not-found sentinel.

```go
package main

import "fmt"

// IndexOf returns the index of target, or -1 if it is not present.
func IndexOf[T comparable](s []T, target T) int {
	for i, v := range s {
		if v == target {
			return i
		}
	}
	return -1
}

func main() {
	fmt.Println(IndexOf([]string{"x", "y", "z"}, "y"))
	fmt.Println(IndexOf([]int{1, 2, 3}, 99))
}
```

**Output:**

```
1
-1
```

---

## 15. Equal compare two slices

`🟡 medium` · *Algorithms*

Comparing two slices element-by-element needs `comparable` elements. Length is checked first, then each pair.

**Steps:**

1. Different lengths short-circuit to `false`.
2. `a[i] != b[i]` requires `comparable`.

```go
package main

import "fmt"

// Equal reports whether two slices have the same elements in the same order.
func Equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func main() {
	fmt.Println(Equal([]int{1, 2, 3}, []int{1, 2, 3}))
	fmt.Println(Equal([]int{1, 2}, []int{1, 2, 3}))
	fmt.Println(Equal([]string{"a"}, []string{"b"}))
}
```

**Output:**

```
true
false
false
```

---

## 16. Reverse a slice in place

`🟡 medium` · *Algorithms*

`Reverse` swaps elements in place, so it returns nothing. The element type is irrelevant — `any` suffices.

**Steps:**

1. Two indices walk inward from both ends.
2. Tuple assignment swaps without a temp variable.

```go
package main

import "fmt"

// Reverse flips a slice in place. Works for any element type.
func Reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func main() {
	xs := []int{1, 2, 3, 4, 5}
	Reverse(xs)
	fmt.Println(xs)

	ws := []string{"first", "second", "third"}
	Reverse(ws)
	fmt.Println(ws)
}
```

**Output:**

```
[5 4 3 2 1]
[third second first]
```

---

## 17. Unique remove duplicates

`🟡 medium` · *Algorithms*

`Unique` removes duplicates while keeping first-seen order. A `map[T]bool` set tracks what's been seen, so `T` must be `comparable`.

**Steps:**

1. `seen` is a set of already-emitted values.
2. Only first occurrences are appended.

```go
package main

import "fmt"

// Unique returns a new slice with duplicates removed, keeping first-seen order.
func Unique[T comparable](s []T) []T {
	seen := make(map[T]bool)
	var out []T
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func main() {
	fmt.Println(Unique([]int{1, 2, 2, 3, 3, 3, 1}))
	fmt.Println(Unique([]string{"a", "b", "a", "c", "b"}))
}
```

**Output:**

```
[1 2 3]
[a b c]
```

---

## 18. A generic Stack type

`🟡 medium` · *Containers*

A **generic type** declares its type parameter on the type itself. Methods reuse `T` but cannot add new type parameters of their own.

**Steps:**

1. `Stack[T any]` parameterizes the struct.
2. Methods are written `func (s *Stack[T]) ...`.
3. `Pop` returns `(T, bool)`, using `var zero T` when empty.

```go
package main

import "fmt"

// Stack is a generic LIFO container. The type parameter T is declared on the
// TYPE; methods reuse it but cannot introduce their own type parameters.
type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}

func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

func main() {
	var s Stack[int]
	s.Push(1)
	s.Push(2)
	s.Push(3)
	for {
		v, ok := s.Pop()
		if !ok {
			break
		}
		fmt.Println(v)
	}
}
```

**Output:**

```
3
2
1
```

---

## 19. A generic Queue type

`🟡 medium` · *Containers*

A FIFO queue, structured like the stack but dequeuing from the front. Same generic-type mechanics.

**Steps:**

1. `Enqueue` appends to the tail.
2. `Dequeue` removes from the head via `items[1:]`.

```go
package main

import "fmt"

// Queue is a generic FIFO container.
type Queue[T any] struct {
	items []T
}

func (q *Queue[T]) Enqueue(v T) {
	q.items = append(q.items, v)
}

func (q *Queue[T]) Dequeue() (T, bool) {
	var zero T
	if len(q.items) == 0 {
		return zero, false
	}
	v := q.items[0]
	q.items = q.items[1:]
	return v, true
}

func main() {
	var q Queue[string]
	q.Enqueue("a")
	q.Enqueue("b")
	q.Enqueue("c")
	for {
		v, ok := q.Dequeue()
		if !ok {
			break
		}
		fmt.Println(v)
	}
}
```

**Output:**

```
a
b
c
```

---

## 20. A generic Pair struct

`🟡 medium` · *Containers*

`Pair` parameterizes over **two** independent types. Generic types can take as many type parameters as they need.

**Steps:**

1. `Pair[A, B any]` has fields of types `A` and `B`.
2. `MakePair` infers both from its arguments.

```go
package main

import "fmt"

// Pair holds two values of independent types A and B.
type Pair[A, B any] struct {
	First  A
	Second B
}

func MakePair[A, B any](a A, b B) Pair[A, B] {
	return Pair[A, B]{First: a, Second: b}
}

func main() {
	p := MakePair("age", 30)
	fmt.Println(p.First, p.Second)
	fmt.Printf("%+v\n", p)
}
```

**Output:**

```
age 30
{First:age Second:30}
```

---

## 21. A generic Set type

`🟡 medium` · *Containers*

A set is a map with `struct{}` values (zero bytes). `T comparable` is required because the elements become map keys.

**Steps:**

1. `map[T]struct{}` is the idiomatic set.
2. `Add` is idempotent; duplicates collapse.
3. `Has` uses the comma-ok form.

```go
package main

import "fmt"

// Set is a generic set backed by a map. Element type must be comparable.
type Set[T comparable] struct {
	m map[T]struct{}
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{m: make(map[T]struct{})}
}

func (s *Set[T]) Add(v T)      { s.m[v] = struct{}{} }
func (s *Set[T]) Has(v T) bool { _, ok := s.m[v]; return ok }
func (s *Set[T]) Len() int     { return len(s.m) }

func main() {
	s := NewSet[string]()
	s.Add("a")
	s.Add("b")
	s.Add("a") // duplicate ignored
	fmt.Println("len:", s.Len())
	fmt.Println("has a:", s.Has("a"))
	fmt.Println("has z:", s.Has("z"))
}
```

**Output:**

```
len: 2
has a: true
has z: false
```

---

## 22. GroupBy bucket by key

`🟡 medium` · *Algorithms*

`GroupBy` buckets elements by a key derived from each one. The key type `K` must be `comparable`; values keep their type `T`.

**Steps:**

1. `keyFn` derives the bucket key per element.
2. `map[K][]T` accumulates each bucket.
3. Sort the keys for stable printing.

```go
package main

import (
	"fmt"
	"sort"
)

// GroupBy buckets elements by the key returned by keyFn.
func GroupBy[K comparable, T any](s []T, keyFn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range s {
		k := keyFn(v)
		out[k] = append(out[k], v)
	}
	return out
}

func main() {
	words := []string{"apple", "avocado", "banana", "cherry", "blueberry"}
	groups := GroupBy(words, func(w string) byte { return w[0] })

	var initials []byte
	for k := range groups {
		initials = append(initials, k)
	}
	sort.Slice(initials, func(i, j int) bool { return initials[i] < initials[j] })

	for _, k := range initials {
		fmt.Printf("%c: %v\n", k, groups[k])
	}
}
```

**Output:**

```
a: [apple avocado]
b: [banana blueberry]
c: [cherry]
```

---

## 23. Frequency count occurrences

`🟡 medium` · *Algorithms*

`Frequency` tallies occurrences into a `map[T]int`. Because values are map keys, `T` must be `comparable`.

**Steps:**

1. `out[v]++` relies on the zero value `0`.
2. Sort keys before printing.

```go
package main

import (
	"fmt"
	"sort"
)

// Frequency counts how many times each value appears.
func Frequency[T comparable](s []T) map[T]int {
	out := make(map[T]int)
	for _, v := range s {
		out[v]++
	}
	return out
}

func main() {
	counts := Frequency([]string{"a", "b", "a", "c", "a", "b"})
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%d\n", k, counts[k])
	}
}
```

**Output:**

```
a=3
b=2
c=1
```

---

## 24. Clamp to a range

`🟡 medium` · *Constraints*

`Clamp` pins a value into `[lo, hi]`. It needs `<` and `>`, so the constraint is an ordered union, not `comparable`.

**Steps:**

1. Below `lo` returns `lo`; above `hi` returns `hi`.
2. Works for ints and floats with the same code.

```go
package main

import "fmt"

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

// Clamp constrains v to the range [lo, hi].
func Clamp[T Ordered](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	fmt.Println(Clamp(5, 0, 10))
	fmt.Println(Clamp(-3, 0, 10))
	fmt.Println(Clamp(42, 0, 10))
	fmt.Println(Clamp(2.5, 0.0, 1.0))
}
```

**Output:**

```
5
0
10
1
```

---

## 25. Chunk into batches

`🟡 medium` · *Algorithms*

`Chunk` splits a slice into fixed-size batches (the last may be shorter). It only stores and slices, so `any` is enough.

**Steps:**

1. Step the start index by `n` each time.
2. Clamp the end index to `len(s)`.
3. Sub-slices share the backing array.

```go
package main

import "fmt"

// Chunk splits a slice into batches of at most size n.
func Chunk[T any](s []T, n int) [][]T {
	var out [][]T
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}

func main() {
	fmt.Println(Chunk([]int{1, 2, 3, 4, 5, 6, 7}, 3))
	fmt.Println(Chunk([]string{"a", "b", "c"}, 2))
}
```

**Output:**

```
[[1 2 3] [4 5 6] [7]]
[[a b] [c]]
```

---

## 26. A generic Optional type

`🟡 medium` · *Containers*

`Optional` models presence/absence without `nil` — useful for value types where `nil` isn't available. It carries the value plus an `ok` flag.

**Steps:**

1. `Some`/`None` are the constructors.
2. `Get` returns `(T, bool)`; `OrElse` supplies a fallback.

```go
package main

import "fmt"

// Optional models a value that may be absent, without using nil.
type Optional[T any] struct {
	value T
	ok    bool
}

func Some[T any](v T) Optional[T] { return Optional[T]{value: v, ok: true} }
func None[T any]() Optional[T]    { return Optional[T]{} }

// Get returns the value and whether it is present.
func (o Optional[T]) Get() (T, bool) { return o.value, o.ok }

// OrElse returns the value if present, else the fallback.
func (o Optional[T]) OrElse(fallback T) T {
	if o.ok {
		return o.value
	}
	return fallback
}

func main() {
	a := Some(42)
	b := None[int]()
	fmt.Println(a.OrElse(-1))
	fmt.Println(b.OrElse(-1))
	v, ok := a.Get()
	fmt.Println(v, ok)
}
```

**Output:**

```
42
-1
42 true
```

---

## 27. Flatten a slice of slices

`🟡 medium` · *Algorithms*

`Flatten` concatenates a `[][]T` into a single `[]T` using append-spread. The element type is irrelevant.

**Steps:**

1. `append(out, inner...)` spreads each inner slice.
2. Order is preserved across the nested slices.

```go
package main

import "fmt"

// Flatten concatenates a slice of slices into one slice.
func Flatten[T any](s [][]T) []T {
	var out []T
	for _, inner := range s {
		out = append(out, inner...)
	}
	return out
}

func main() {
	fmt.Println(Flatten([][]int{{1, 2}, {3}, {4, 5, 6}}))
	fmt.Println(Flatten([][]string{{"a"}, {"b", "c"}}))
}
```

**Output:**

```
[1 2 3 4 5 6]
[a b c]
```

---

## 28. Associate build a map from a slice

`🟡 medium` · *Maps*

`Associate` turns a slice into a map via a key function and a value function — three type parameters working together.

**Steps:**

1. `keyFn` and `valFn` produce each entry.
2. `[T any, K comparable, V any]` — only the key must be comparable.

```go
package main

import (
	"fmt"
	"sort"
)

// Associate builds a map keyed by keyFn(v) with value valFn(v).
func Associate[T any, K comparable, V any](s []T, keyFn func(T) K, valFn func(T) V) map[K]V {
	out := make(map[K]V, len(s))
	for _, v := range s {
		out[keyFn(v)] = valFn(v)
	}
	return out
}

func main() {
	words := []string{"go", "rust", "zig"}
	lengths := Associate(words,
		func(w string) string { return w },
		func(w string) int { return len(w) },
	)
	keys := make([]string, 0, len(lengths))
	for k := range lengths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%d\n", k, lengths[k])
	}
}
```

**Output:**

```
go=2
rust=4
zig=3
```

---

*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
