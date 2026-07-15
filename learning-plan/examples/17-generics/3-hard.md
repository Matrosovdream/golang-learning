# Step 17 — Generics · 🔴 Hard

Examples **29–40**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 29. A generic binary search tree

`🔴 hard` · *Containers*

A binary search tree parameterized by an `Ordered` element type. Note the recursive `insert` is a **free function**: methods can't declare the type parameter that recursion needs.

**Steps:**

1. `node[T]` and `Tree[T]` are generic types.
2. `insert[T Ordered](...)` recurses as a generic function.
3. In-order traversal yields sorted values.

```go
package main

import "fmt"

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

type node[T Ordered] struct {
	value       T
	left, right *node[T]
}

// Tree is a generic binary search tree.
type Tree[T Ordered] struct {
	root *node[T]
}

func (t *Tree[T]) Insert(v T) {
	t.root = insert(t.root, v)
}

// insert is a free function: methods cannot declare new type parameters,
// so recursion over node[T] lives in a generic function instead.
func insert[T Ordered](n *node[T], v T) *node[T] {
	if n == nil {
		return &node[T]{value: v}
	}
	if v < n.value {
		n.left = insert(n.left, v)
	} else if v > n.value {
		n.right = insert(n.right, v)
	}
	return n
}

// InOrder returns the values in sorted order.
func (t *Tree[T]) InOrder() []T {
	var out []T
	var walk func(*node[T])
	walk = func(n *node[T]) {
		if n == nil {
			return
		}
		walk(n.left)
		out = append(out, n.value)
		walk(n.right)
	}
	walk(t.root)
	return out
}

func main() {
	var t Tree[int]
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9, 2} {
		t.Insert(v)
	}
	fmt.Println(t.InOrder())

	var st Tree[string]
	for _, w := range []string{"banana", "apple", "cherry"} {
		st.Insert(w)
	}
	fmt.Println(st.InOrder())
}
```

**Output:**

```
[1 2 3 4 5 7 8 9]
[apple banana cherry]
```

---

## 30. A generic Result type

`🔴 hard` · *Containers*

`Result` wraps either a value or an error — a typed alternative to returning `(T, error)`.

**Steps:**

1. `Ok`/`Err` are the constructors.
2. `IsOk` checks success; `Unwrap` panics on error.
3. `divide` returns a `Result[int]`.

```go
package main

import (
	"errors"
	"fmt"
)

// Result holds either a value or an error: a typed alternative to (T, error).
type Result[T any] struct {
	value T
	err   error
}

func Ok[T any](v T) Result[T]      { return Result[T]{value: v} }
func Err[T any](e error) Result[T] { return Result[T]{err: e} }

func (r Result[T]) IsOk() bool { return r.err == nil }

func (r Result[T]) Unwrap() T {
	if r.err != nil {
		panic(r.err)
	}
	return r.value
}

func divide(a, b int) Result[int] {
	if b == 0 {
		return Err[int](errors.New("division by zero"))
	}
	return Ok(a / b)
}

func main() {
	r1 := divide(10, 2)
	r2 := divide(10, 0)
	fmt.Println(r1.IsOk(), r1.Unwrap())
	fmt.Println(r2.IsOk())
}
```

**Output:**

```
true 5
false
```

---

## 31. MapValues transform map values

`🔴 hard` · *Maps*

`MapValues` rebuilds a map with the same keys but transformed values. Three type parameters: `K`, the old value `V`, the new value `U`.

**Steps:**

1. Keys are copied; `f` transforms each value.
2. Sort keys for deterministic output.

```go
package main

import (
	"fmt"
	"sort"
)

// MapValues returns a new map with the same keys but values transformed by f.
func MapValues[K comparable, V, U any](m map[K]V, f func(V) U) map[K]U {
	out := make(map[K]U, len(m))
	for k, v := range m {
		out[k] = f(v)
	}
	return out
}

func main() {
	prices := map[string]int{"apple": 3, "banana": 2, "cherry": 5}
	withTax := MapValues(prices, func(p int) float64 { return float64(p) * 1.1 })

	keys := make([]string, 0, len(withTax))
	for k := range withTax {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s: %.2f\n", k, withTax[k])
	}
}
```

**Output:**

```
apple: 3.30
banana: 2.20
cherry: 5.50
```

---

## 32. Memoize cache a function

`🔴 hard` · *Higher-order*

`Memoize` wraps a one-argument function with a cache so each distinct input is computed once. The argument type `K` must be `comparable` to serve as a cache key.

**Steps:**

1. A closure captures the `map[K]V` cache.
2. Cache hits skip the wrapped function.
3. The call counter proves the second `square(4)` was cached.

```go
package main

import "fmt"

// Memoize wraps f so each distinct argument is computed only once.
func Memoize[K comparable, V any](f func(K) V) func(K) V {
	cache := make(map[K]V)
	return func(k K) V {
		if v, ok := cache[k]; ok {
			return v
		}
		v := f(k)
		cache[k] = v
		return v
	}
}

func main() {
	calls := 0
	square := Memoize(func(n int) int {
		calls++
		return n * n
	})
	fmt.Println(square(4))
	fmt.Println(square(4)) // cached: no recompute
	fmt.Println(square(5))
	fmt.Println("compute calls:", calls)
}
```

**Output:**

```
16
16
25
compute calls: 2
```

---

## 33. Compose two functions

`🔴 hard` · *Higher-order*

`Compose` chains two functions into one — `x -> g(f(x))` — threading three types `A -> B -> C` through the signatures.

**Steps:**

1. `f func(A) B`, `g func(B) C`.
2. The returned closure has type `func(A) C`.

```go
package main

import (
	"fmt"
	"strconv"
)

// Compose returns a function that applies g after f: x -> g(f(x)).
func Compose[A, B, C any](f func(A) B, g func(B) C) func(A) C {
	return func(a A) C {
		return g(f(a))
	}
}

func main() {
	doubleThenLabel := Compose(
		func(n int) int { return n * 2 },
		func(n int) string { return "=" + strconv.Itoa(n) },
	)
	fmt.Println(doubleThenLabel(21))
}
```

**Output:**

```
=42
```

---

## 34. Zip two slices into pairs

`🔴 hard` · *Algorithms*

`Zip` pairs elements of two slices into `[]Pair[A, B]`, stopping at the shorter length. Two element types flow through independently.

**Steps:**

1. `n` is the shorter of the two lengths.
2. Each index becomes one `Pair[A, B]`.

```go
package main

import "fmt"

type Pair[A, B any] struct {
	First  A
	Second B
}

// Zip pairs up elements of two slices, stopping at the shorter length.
func Zip[A, B any](as []A, bs []B) []Pair[A, B] {
	n := len(as)
	if len(bs) < n {
		n = len(bs)
	}
	out := make([]Pair[A, B], n)
	for i := 0; i < n; i++ {
		out[i] = Pair[A, B]{First: as[i], Second: bs[i]}
	}
	return out
}

func main() {
	names := []string{"Alice", "Bob", "Carol"}
	ages := []int{30, 25, 40, 99} // the extra 99 is ignored
	for _, p := range Zip(names, ages) {
		fmt.Printf("%s is %d\n", p.First, p.Second)
	}
}
```

**Output:**

```
Alice is 30
Bob is 25
Carol is 40
```

---

## 35. A generic linked list

`🔴 hard` · *Containers*

A singly linked list with O(1) `Prepend`. The node and the list are both generic types referencing `T`.

**Steps:**

1. `listNode[T]` links to the next node.
2. `Prepend` pushes onto the head.
3. `Slice` walks the chain into a `[]T`.

```go
package main

import "fmt"

type listNode[T any] struct {
	value T
	next  *listNode[T]
}

// List is a singly linked list. Prepend is O(1); Slice collects in order.
type List[T any] struct {
	head *listNode[T]
	size int
}

func (l *List[T]) Prepend(v T) {
	l.head = &listNode[T]{value: v, next: l.head}
	l.size++
}

func (l *List[T]) Slice() []T {
	out := make([]T, 0, l.size)
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.value)
	}
	return out
}

func main() {
	var l List[int]
	l.Prepend(3)
	l.Prepend(2)
	l.Prepend(1)
	fmt.Println(l.Slice(), "size:", l.size)
}
```

**Output:**

```
[1 2 3] size: 3
```

---

## 36. A generic typed event bus

`🔴 hard` · *Containers*

A typed publish/subscribe bus: handlers receive a strongly-typed event payload, with no `interface{}` or type assertions.

**Steps:**

1. `Bus[T]` holds `[]func(T)` handlers.
2. `Publish` fans the event out to every subscriber.
3. The payload type is fixed at `Bus[UserSignedUp]`.

```go
package main

import "fmt"

// Bus is a tiny typed pub/sub: every subscriber receives each published event.
type Bus[T any] struct {
	handlers []func(T)
}

func (b *Bus[T]) Subscribe(h func(T)) {
	b.handlers = append(b.handlers, h)
}

func (b *Bus[T]) Publish(event T) {
	for _, h := range b.handlers {
		h(event)
	}
}

type UserSignedUp struct {
	Name string
}

func main() {
	var bus Bus[UserSignedUp]
	bus.Subscribe(func(e UserSignedUp) { fmt.Println("email ->", e.Name) })
	bus.Subscribe(func(e UserSignedUp) { fmt.Println("audit ->", e.Name) })

	bus.Publish(UserSignedUp{Name: "Alice"})
	bus.Publish(UserSignedUp{Name: "Bob"})
}
```

**Output:**

```
email -> Alice
audit -> Alice
email -> Bob
audit -> Bob
```

---

## 37. A generic LRU cache

`🔴 hard` · *Containers*

A fixed-capacity cache that evicts the least-recently-used key. Both the key `K` (comparable) and value `V` are generic.

**Steps:**

1. `order` tracks recency; `touch` moves a key to the back.
2. `Put` evicts `order[0]` when full.
3. Accessing `"a"` saves it from eviction; `"b"` is dropped.

```go
package main

import "fmt"

// LRU is a fixed-capacity cache that evicts the least-recently-used key.
type LRU[K comparable, V any] struct {
	cap   int
	data  map[K]V
	order []K // least-recent first, most-recent last
}

func NewLRU[K comparable, V any](capacity int) *LRU[K, V] {
	return &LRU[K, V]{cap: capacity, data: make(map[K]V)}
}

func (c *LRU[K, V]) touch(key K) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, key)
}

func (c *LRU[K, V]) Get(key K) (V, bool) {
	v, ok := c.data[key]
	if ok {
		c.touch(key)
	}
	return v, ok
}

func (c *LRU[K, V]) Put(key K, value V) {
	if _, ok := c.data[key]; !ok && len(c.data) >= c.cap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.data, oldest)
	}
	c.data[key] = value
	c.touch(key)
}

func main() {
	c := NewLRU[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")    // "a" is now most-recently-used
	c.Put("c", 3) // evicts "b", the least-recently-used

	_, hasB := c.Get("b")
	va, _ := c.Get("a")
	vc, _ := c.Get("c")
	fmt.Println("b present:", hasB)
	fmt.Println("a =", va)
	fmt.Println("c =", vc)
}
```

**Output:**

```
b present: false
a = 1
c = 3
```

---

## 38. Partition by a predicate

`🔴 hard` · *Algorithms*

`Partition` splits a slice into two by a predicate, returning both halves via named results. The element type is preserved.

**Steps:**

1. Matches go to `yes`, the rest to `no`.
2. Named returns `(yes, no []T)` document the split.

```go
package main

import "fmt"

// Partition splits s into the elements that satisfy pred and those that do not.
func Partition[T any](s []T, pred func(T) bool) (yes, no []T) {
	for _, v := range s {
		if pred(v) {
			yes = append(yes, v)
		} else {
			no = append(no, v)
		}
	}
	return yes, no
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8}
	evens, odds := Partition(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println("evens:", evens)
	fmt.Println("odds: ", odds)
}
```

**Output:**

```
evens: [2 4 6 8]
odds:  [1 3 5 7]
```

---

## 39. SortBy a generic key

`🔴 hard` · *Algorithms*

`SortBy` sorts in place using a generic key extractor. The element type `T` is unconstrained; the **key** type `K` must be `Ordered`.

**Steps:**

1. `keyFn` projects each element to an ordered key.
2. `sort.Slice` orders by `keyFn(s[i]) < keyFn(s[j])`.
3. Here people are sorted by age.

```go
package main

import (
	"fmt"
	"sort"
)

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

// SortBy sorts s in place by the ordered key returned by keyFn.
func SortBy[T any, K Ordered](s []T, keyFn func(T) K) {
	sort.Slice(s, func(i, j int) bool {
		return keyFn(s[i]) < keyFn(s[j])
	})
}

type Person struct {
	Name string
	Age  int
}

func main() {
	people := []Person{
		{"Carol", 40},
		{"Alice", 30},
		{"Bob", 25},
	}
	SortBy(people, func(p Person) int { return p.Age })
	for _, p := range people {
		fmt.Printf("%s %d\n", p.Name, p.Age)
	}
}
```

**Output:**

```
Bob 25
Alice 30
Carol 40
```

---

## 40. Capstone a generic repository and pipeline

`🔴 hard` · *Capstone*

A capstone tying it together: a generic `Repo` whose element type is constrained by a generic `Entity[ID]` interface, plus a `Map`/`Filter`/`Reduce` pipeline over the results.

**Steps:**

1. `Entity[ID]` is a constraint *with a method* (`GetID`).
2. `Repo[ID, E Entity[ID]]` uses one type parameter inside another's constraint.
3. `Product` satisfies `Entity[int]`, then flows through the pipeline.

```go
package main

import (
	"fmt"
	"sort"
)

// ---- Generic building blocks ----

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

func Filter[T any](s []T, keep func(T) bool) []T {
	var out []T
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

// ---- Generic repository keyed by an ordered ID ----

// Entity constrains E to types that expose an ID of type ID.
type Entity[ID Ordered] interface {
	GetID() ID
}

type Repo[ID Ordered, E Entity[ID]] struct {
	items map[ID]E
}

func NewRepo[ID Ordered, E Entity[ID]]() *Repo[ID, E] {
	return &Repo[ID, E]{items: make(map[ID]E)}
}

func (r *Repo[ID, E]) Save(e E) { r.items[e.GetID()] = e }

func (r *Repo[ID, E]) All() []E {
	out := make([]E, 0, len(r.items))
	for _, e := range r.items {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetID() < out[j].GetID() })
	return out
}

// ---- A domain type that satisfies Entity[int] ----

type Product struct {
	ID    int
	Name  string
	Price float64
}

func (p Product) GetID() int { return p.ID }

func main() {
	repo := NewRepo[int, Product]()
	repo.Save(Product{ID: 3, Name: "Keyboard", Price: 80})
	repo.Save(Product{ID: 1, Name: "Mouse", Price: 25})
	repo.Save(Product{ID: 2, Name: "Monitor", Price: 200})

	all := repo.All()

	// Pipeline: keep items priced >= 50, take their names; total all prices.
	expensive := Filter(all, func(p Product) bool { return p.Price >= 50 })
	names := Map(expensive, func(p Product) string { return p.Name })
	total := Reduce(all, 0.0, func(acc float64, p Product) float64 { return acc + p.Price })

	fmt.Println("all IDs in order:", Map(all, func(p Product) int { return p.ID }))
	fmt.Println("expensive:", names)
	fmt.Printf("total inventory value: %.2f\n", total)
}
```

**Output:**

```
all IDs in order: [1 2 3]
expensive: [Monitor Keyboard]
total inventory value: 305.00
```

---

## 41. Composing constraints by embedding

`🔴 hard` · *Constraints*

A constraint can **embed** other constraints; its type set becomes their union. Here a numeric tower is built up: `Signed`/`Unsigned` → `Integer` → `Number`.

**Steps:**

1. `Integer` embeds `Signed | Unsigned` — no type literals of its own.
2. `Number` embeds `Integer | Float`.
3. `Sum` accepts any `Number` because `+` is defined across the whole set.

```go
package main

import "fmt"

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Integer embeds two constraints: its type set is their union.
type Integer interface {
	Signed | Unsigned
}
type Float interface {
	~float32 | ~float64
}

// Number is the union of every integer and float type.
type Number interface {
	Integer | Float
}

// Sum works for any Number because + is defined on the whole type set.
func Sum[T Number](xs []T) T {
	var total T
	for _, x := range xs {
		total += x
	}
	return total
}

func main() {
	fmt.Println(Sum([]int{1, 2, 3, 4}))
	fmt.Println(Sum([]uint8{10, 20, 30}))
	fmt.Printf("%.1f\n", Sum([]float64{1.5, 2.5, 3.0}))
}
```

**Output:**

```
10
60
7.0
```

---

## 42. A self-referential constraint

`🔴 hard` · *Constraints*

A type parameter may appear **inside its own constraint**. `Lesser[T]` requires a `Less(T) bool` method, so each type compares against its own kind — `MaxOf` then works on anything orderable by a method, not just built-in `<`.

**Steps:**

1. `Lesser[T]` mentions `T` in its method signature.
2. `MaxOf[T Lesser[T]]` ties the parameter to the constraint recursively.
3. `Version` satisfies it with a custom `Less`.

```go
package main

import "fmt"

// Lesser constrains T to types that can compare against their own type.
// Note T appears inside its own constraint — a self-referential type parameter.
type Lesser[T any] interface {
	Less(T) bool
}

// MaxOf returns the largest element using each type's own Less method.
func MaxOf[T Lesser[T]](xs []T) T {
	best := xs[0]
	for _, x := range xs[1:] {
		if best.Less(x) {
			best = x
		}
	}
	return best
}

type Version struct {
	Major, Minor int
}

func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	return v.Minor < o.Minor
}

func main() {
	vs := []Version{{1, 4}, {2, 0}, {1, 11}, {2, 0}}
	top := MaxOf(vs)
	fmt.Printf("latest: %d.%d\n", top.Major, top.Minor)
}
```

**Output:**

```
latest: 2.0
```

---

## 43. All, Any, and None predicates

`🔴 hard` · *Algorithms*

Three short predicate combinators over any slice. `None` is defined in terms of `Any`, showing how generic helpers compose.

**Steps:**

1. `Any` short-circuits on the first match.
2. `All` short-circuits on the first failure.
3. `None` is simply `!Any`.

```go
package main

import "fmt"

// Any reports whether at least one element satisfies pred.
func Any[T any](s []T, pred func(T) bool) bool {
	for _, v := range s {
		if pred(v) {
			return true
		}
	}
	return false
}

// All reports whether every element satisfies pred.
func All[T any](s []T, pred func(T) bool) bool {
	for _, v := range s {
		if !pred(v) {
			return false
		}
	}
	return true
}

// None is just "not Any": no element satisfies pred.
func None[T any](s []T, pred func(T) bool) bool {
	return !Any(s, pred)
}

func main() {
	nums := []int{2, 4, 6, 8}
	isEven := func(n int) bool { return n%2 == 0 }
	fmt.Println("any even:", Any(nums, isEven))
	fmt.Println("all even:", All(nums, isEven))
	fmt.Println("none odd:", None(nums, func(n int) bool { return n%2 == 1 }))
}
```

**Output:**

```
any even: true
all even: true
none odd: true
```

---

## 44. Take, Drop, and TakeWhile

`🔴 hard` · *Algorithms*

Slice-window helpers that preserve the element type. `Take`/`Drop` cut by count; `TakeWhile` cuts at the first element that fails a predicate.

**Steps:**

1. `Take`/`Drop` clamp `n` so they never panic.
2. `TakeWhile` returns the leading run that satisfies `pred`.

```go
package main

import "fmt"

// Take returns the first n elements (or all of them if n is too large).
func Take[T any](s []T, n int) []T {
	if n > len(s) {
		n = len(s)
	}
	return s[:n]
}

// Drop returns everything after the first n elements.
func Drop[T any](s []T, n int) []T {
	if n > len(s) {
		n = len(s)
	}
	return s[n:]
}

// TakeWhile returns the leading run of elements that satisfy pred.
func TakeWhile[T any](s []T, pred func(T) bool) []T {
	for i, v := range s {
		if !pred(v) {
			return s[:i]
		}
	}
	return s
}

func main() {
	nums := []int{1, 2, 3, 4, 1, 2}
	fmt.Println(Take(nums, 3))
	fmt.Println(Drop(nums, 4))
	fmt.Println(TakeWhile(nums, func(n int) bool { return n < 4 }))
}
```

**Output:**

```
[1 2 3]
[1 2]
[1 2 3]
```

---

## 45. FlatMap

`🔴 hard` · *Higher-order*

`FlatMap` maps each element to a *slice* and concatenates the results — a `Map` whose function returns many values per input. Two type parameters flow through independently.

**Steps:**

1. `f` returns `[]U` for each `T`.
2. `append(out, f(v)...)` flattens as it goes.

```go
package main

import (
	"fmt"
	"strings"
)

// FlatMap maps each element to a slice, then concatenates the results.
// Two type parameters flow through: input T and output element U.
func FlatMap[T, U any](s []T, f func(T) []U) []U {
	var out []U
	for _, v := range s {
		out = append(out, f(v)...)
	}
	return out
}

func main() {
	sentences := []string{"hello world", "go generics"}
	words := FlatMap(sentences, func(s string) []string {
		return strings.Fields(s)
	})
	fmt.Println(words)
	fmt.Println("count:", len(words))
}
```

**Output:**

```
[hello world go generics]
count: 4
```

---

## 46. A generic binary min-heap

`🔴 hard` · *Containers*

A hand-written binary heap (priority queue) over any `Ordered` element. `Push` bubbles up; `Pop` sifts the root down. Popping in a loop drains the heap in sorted order.

**Steps:**

1. The heap is a flat slice; child of `i` lives at `2i+1`/`2i+2`.
2. `Push` swaps a new leaf upward while it beats its parent.
3. `Pop` moves the last leaf to the root, then sifts it down.

```go
package main

import "fmt"

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

// Heap is a binary min-heap: Push/Pop keep the smallest element at the root.
type Heap[T Ordered] struct {
	data []T
}

// Push adds v, then bubbles it up until the heap order is restored.
func (h *Heap[T]) Push(v T) {
	h.data = append(h.data, v)
	i := len(h.data) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if h.data[parent] <= h.data[i] {
			break
		}
		h.data[parent], h.data[i] = h.data[i], h.data[parent]
		i = parent
	}
}

// Pop removes and returns the smallest element, then sifts the root down.
func (h *Heap[T]) Pop() T {
	root := h.data[0]
	last := len(h.data) - 1
	h.data[0] = h.data[last]
	h.data = h.data[:last]
	i, n := 0, len(h.data)
	for {
		l, r, smallest := 2*i+1, 2*i+2, i
		if l < n && h.data[l] < h.data[smallest] {
			smallest = l
		}
		if r < n && h.data[r] < h.data[smallest] {
			smallest = r
		}
		if smallest == i {
			break
		}
		h.data[i], h.data[smallest] = h.data[smallest], h.data[i]
		i = smallest
	}
	return root
}

func (h *Heap[T]) Len() int { return len(h.data) }

func main() {
	var h Heap[int]
	for _, v := range []int{5, 1, 8, 3, 9, 2} {
		h.Push(v)
	}
	for h.Len() > 0 {
		fmt.Print(h.Pop(), " ")
	}
	fmt.Println()
}
```

**Output:**

```
1 2 3 5 8 9 
```

---

## 47. Pointer helpers Ptr, Deref, and Coalesce

`🔴 hard` · *Helpers*

Working with optional fields (think JSON `*int`, `*string`) is verbose. These three generic helpers make it clean: wrap a value in a pointer, read with a default, or pick the first non-nil.

**Steps:**

1. `Ptr` returns the address of a copy — usable inline.
2. `Deref` substitutes a fallback when the pointer is nil.
3. `Coalesce` scans variadic pointers for the first set one.

```go
package main

import "fmt"

// Ptr returns a pointer to v — handy for optional struct or JSON fields.
func Ptr[T any](v T) *T { return &v }

// Deref returns *p, or fallback when p is nil.
func Deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// Coalesce returns the first non-nil pointer's value, else the zero value.
func Coalesce[T any](ps ...*T) T {
	for _, p := range ps {
		if p != nil {
			return *p
		}
	}
	var zero T
	return zero
}

type Config struct {
	Timeout *int
	Region  *string
}

func main() {
	cfg := Config{Timeout: Ptr(30)}
	fmt.Println("timeout:", Deref(cfg.Timeout, 10))
	fmt.Println("region: ", Deref(cfg.Region, "us-east"))
	fmt.Println("first set:", Coalesce(cfg.Region, Ptr("eu-west"), Ptr("ap-south")))
}
```

**Output:**

```
timeout: 30
region:  us-east
first set: eu-west
```

---

## 48. Generic functional options

`🔴 hard` · *Higher-order*

The functional-options pattern, made generic. `Option[T]` is `func(*T)`; `Build` applies a list of them to a fresh value. The same `Build` works for any configurable struct.

**Steps:**

1. `Option[T]` is a function that mutates a `*T`.
2. `Build` starts from the zero value and folds the options in.
3. `WithHost`/`WithPort`/`WithTLS` are typed `Option[Server]` constructors.

```go
package main

import "fmt"

// Option mutates a value during construction.
type Option[T any] func(*T)

// Build starts from a zero T, applies every option, and returns the result.
func Build[T any](opts ...Option[T]) T {
	var t T
	for _, opt := range opts {
		opt(&t)
	}
	return t
}

type Server struct {
	Host string
	Port int
	TLS  bool
}

func WithHost(h string) Option[Server] { return func(s *Server) { s.Host = h } }
func WithPort(p int) Option[Server]    { return func(s *Server) { s.Port = p } }
func WithTLS() Option[Server]          { return func(s *Server) { s.TLS = true } }

func main() {
	s := Build(WithHost("localhost"), WithPort(8443), WithTLS())
	fmt.Printf("%+v\n", s)
}
```

**Output:**

```
{Host:localhost Port:8443 TLS:true}
```

---

## 49. Invert a map

`🔴 hard` · *Maps*

`Invert` swaps keys and values. Because the old values become the new keys, **both** `K` and `V` must be `comparable` — a constraint the compiler enforces for you.

**Steps:**

1. `[K, V comparable]` lets `V` serve as a map key.
2. Each `m[k] = v` becomes `out[v] = k`.

```go
package main

import (
	"fmt"
	"sort"
)

// Invert swaps keys and values. V must be comparable to become a key.
func Invert[K, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

func main() {
	codes := map[string]int{"US": 1, "FR": 33, "JP": 81}
	byCode := Invert(codes)

	nums := make([]int, 0, len(byCode))
	for n := range byCode {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		fmt.Printf("%d -> %s\n", n, byCode[n])
	}
}
```

**Output:**

```
1 -> US
33 -> FR
81 -> JP
```

---

## 50. Capstone a generic insertion-ordered map

`🔴 hard` · *Capstone*

A capstone combining a map, a slice, type parameters, and methods: an `OrderedMap` that iterates keys in the order they were first inserted — something Go's built-in `map` never guarantees.

**Steps:**

1. `keys []K` records insertion order; `values map[K]V` holds the data.
2. `Set` appends to `keys` only on first insert, so updates keep position.
3. `Each` walks `keys`, yielding a stable, ordered traversal.

```go
package main

import "fmt"

// OrderedMap is a map that remembers the insertion order of its keys.
type OrderedMap[K comparable, V any] struct {
	keys   []K
	values map[K]V
}

func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{values: make(map[K]V)}
}

// Set inserts or updates k; first insertion records its position.
func (m *OrderedMap[K, V]) Set(k K, v V) {
	if _, ok := m.values[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.values[k] = v
}

func (m *OrderedMap[K, V]) Get(k K) (V, bool) {
	v, ok := m.values[k]
	return v, ok
}

// Each iterates keys in the order they were first inserted.
func (m *OrderedMap[K, V]) Each(fn func(K, V)) {
	for _, k := range m.keys {
		fn(k, m.values[k])
	}
}

func main() {
	m := NewOrderedMap[string, int]()
	m.Set("gamma", 3)
	m.Set("alpha", 1)
	m.Set("beta", 2)
	m.Set("alpha", 10) // update keeps the original position

	m.Each(func(k string, v int) {
		fmt.Printf("%s=%d\n", k, v)
	})
}
```

**Output:**

```
gamma=3
alpha=10
beta=2
```

---

## 51. MinBy and MaxBy with a key extractor

`🔴 hard` · *Algorithms*

`MinBy`/`MaxBy` find the extreme element by a projected key — like `SortBy`, but they return a single element instead of sorting. The element type `T` is free; the **key** `K` must be `Ordered`.

**Steps:**

1. `key` projects each element to an ordered value.
2. Track the best element and its key together as you scan.
3. Caching `bestKey` avoids recomputing the projection each step.

```go
package main

import "fmt"

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

// MinBy returns the element whose key is smallest. The slice must be non-empty.
func MinBy[T any, K Ordered](s []T, key func(T) K) T {
	best := s[0]
	bestKey := key(best)
	for _, v := range s[1:] {
		if k := key(v); k < bestKey {
			best, bestKey = v, k
		}
	}
	return best
}

// MaxBy returns the element whose key is largest.
func MaxBy[T any, K Ordered](s []T, key func(T) K) T {
	best := s[0]
	bestKey := key(best)
	for _, v := range s[1:] {
		if k := key(v); k > bestKey {
			best, bestKey = v, k
		}
	}
	return best
}

type Product struct {
	Name  string
	Price float64
}

func main() {
	ps := []Product{
		{"Mouse", 25},
		{"Monitor", 200},
		{"Keyboard", 80},
	}
	fmt.Println("cheapest:", MinBy(ps, func(p Product) float64 { return p.Price }).Name)
	fmt.Println("priciest:", MaxBy(ps, func(p Product) float64 { return p.Price }).Name)
}
```

**Output:**

```
cheapest: Mouse
priciest: Monitor
```

---

## 52. CountBy tally with a key function

`🔴 hard` · *Maps*

`CountBy` buckets elements by a key function and counts each bucket — `Frequency` (#23) generalized to any projected key. `T` is free; the key `K` must be `comparable` to index the map.

**Steps:**

1. `key` maps each element to a bucket key.
2. `out[key(v)]++` relies on the zero value of a missing key being `0`.

```go
package main

import (
	"fmt"
	"sort"
)

// CountBy tallies how many elements fall into each key bucket.
func CountBy[T any, K comparable](s []T, key func(T) K) map[K]int {
	out := make(map[K]int)
	for _, v := range s {
		out[key(v)]++
	}
	return out
}

func main() {
	words := []string{"ant", "bear", "cat", "dog", "eagle", "fox"}
	byLen := CountBy(words, func(w string) int { return len(w) })

	lens := make([]int, 0, len(byLen))
	for l := range byLen {
		lens = append(lens, l)
	}
	sort.Ints(lens)
	for _, l := range lens {
		fmt.Printf("length %d: %d word(s)\n", l, byLen[l])
	}
}
```

**Output:**

```
length 3: 4 word(s)
length 4: 1 word(s)
length 5: 1 word(s)
```

---

## 53. Scan running accumulation

`🔴 hard` · *Higher-order*

`Scan` is `Reduce` that keeps every intermediate result — useful for running totals, prefix sums, or cumulative state. It returns one accumulator per input element.

**Steps:**

1. `init` seeds the accumulator like in `Reduce`.
2. After folding each element, store the current `acc`.
3. Two type parameters: input `T` and accumulator `U`.

```go
package main

import "fmt"

// Scan is like Reduce but keeps every intermediate accumulator.
// The result has one entry per input element.
func Scan[T, U any](s []T, init U, f func(U, T) U) []U {
	out := make([]U, len(s))
	acc := init
	for i, v := range s {
		acc = f(acc, v)
		out[i] = acc
	}
	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5}
	running := Scan(nums, 0, func(acc, n int) int { return acc + n })
	fmt.Println("running totals:", running)
}
```

**Output:**

```
running totals: [1 3 6 10 15]
```

---

## 54. Pipe chain functions of one type

`🔴 hard` · *Higher-order*

`Pipe` composes any number of `func(T) T` into a single function, applied left-to-right. Unlike `Compose` (#33), every stage shares one type, so it accepts a variadic list.

**Steps:**

1. `fns ...func(T) T` collects the stages.
2. The returned closure threads `x` through each in order.

```go
package main

import "fmt"

// Pipe chains functions of one type left-to-right: Pipe(f, g)(x) == g(f(x)).
func Pipe[T any](fns ...func(T) T) func(T) T {
	return func(x T) T {
		for _, f := range fns {
			x = f(x)
		}
		return x
	}
}

func main() {
	transform := Pipe(
		func(n int) int { return n + 1 },
		func(n int) int { return n * 2 },
		func(n int) int { return n - 3 },
	)
	fmt.Println(transform(5)) // ((5+1)*2)-3 = 9
}
```

**Output:**

```
9
```

---

## 55. Curry a two-argument function

`🔴 hard` · *Higher-order*

`Curry` turns `func(A, B) C` into `func(A) func(B) C`, enabling partial application: fix the first argument now, supply the second later. Three type parameters thread through the nested closures.

**Steps:**

1. The outer closure captures `a`.
2. The inner closure captures `b` and finally calls `f`.
3. `curried(10)` is a reusable, partially-applied function.

```go
package main

import "fmt"

// Curry turns a two-argument function into a chain of one-argument functions.
func Curry[A, B, C any](f func(A, B) C) func(A) func(B) C {
	return func(a A) func(B) C {
		return func(b B) C {
			return f(a, b)
		}
	}
}

func main() {
	add := func(a, b int) int { return a + b }
	curried := Curry(add)

	addTen := curried(10) // partially applied: B is still open
	fmt.Println(addTen(5))
	fmt.Println(addTen(32))
	fmt.Println(curried(2)(3))
}
```

**Output:**

```
15
42
5
```

---

## 56. Sort with a comparator function

`🔴 hard` · *Algorithms*

`SortFunc` sorts by a three-way comparator (`-1/0/+1`), the shape the standard library's `slices.SortFunc` and `cmp.Compare` use. A comparator composes tie-breakers cleanly — sort by age, then by name.

**Steps:**

1. `cmp(a, b) < 0` means `a` sorts before `b`.
2. `cmp.Compare` from the standard library returns the three-way result.
3. Chain comparators: fall through to the next key only on a tie.

```go
package main

import (
	"cmp"
	"fmt"
	"sort"
)

// SortFunc sorts s in place using a three-way comparator:
// cmp(a, b) < 0 means a sorts before b.
func SortFunc[T any](s []T, cmp func(a, b T) int) {
	sort.Slice(s, func(i, j int) bool {
		return cmp(s[i], s[j]) < 0
	})
}

type Person struct {
	Name string
	Age  int
}

func main() {
	people := []Person{
		{"Alice", 30},
		{"Bob", 30},
		{"Carol", 25},
	}
	// Sort by age ascending, then by name to break ties.
	SortFunc(people, func(a, b Person) int {
		if c := cmp.Compare(a.Age, b.Age); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	for _, p := range people {
		fmt.Printf("%s %d\n", p.Name, p.Age)
	}
}
```

**Output:**

```
Carol 25
Alice 30
Bob 30
```

---

## 57. Binary search a sorted slice

`🔴 hard` · *Algorithms*

A generic binary search over any `Ordered` slice. It returns the index and a found flag; when absent, the index is the insertion point that keeps the slice sorted.

**Steps:**

1. Maintain a half-open window `[lo, hi)`.
2. Compare the midpoint and shrink toward the target.
3. On exit, `lo` is exactly where the value belongs.

```go
package main

import "fmt"

type Ordered interface {
	~int | ~int64 | ~float64 | ~string
}

// BinarySearch returns the index of target in a sorted slice and whether it was
// found. When absent, the index is where target could be inserted to stay sorted.
func BinarySearch[T Ordered](s []T, target T) (int, bool) {
	lo, hi := 0, len(s)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case s[mid] == target:
			return mid, true
		case s[mid] < target:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return lo, false
}

func main() {
	nums := []int{2, 5, 8, 12, 16, 23, 38, 56, 72, 91}
	for _, t := range []int{23, 10} {
		if i, ok := BinarySearch(nums, t); ok {
			fmt.Printf("%d found at index %d\n", t, i)
		} else {
			fmt.Printf("%d not found; would insert at %d\n", t, i)
		}
	}
}
```

**Output:**

```
23 found at index 5
10 not found; would insert at 3
```

---

## 58. Parallel Map with goroutines

`🔴 hard` · *Concurrency*

A generic `Map` that runs `f` on each element concurrently, then waits. Each goroutine writes its **own index** in the output slice, so the result stays in input order with no mutex.

**Steps:**

1. Pre-size `out` so every goroutine has a reserved slot.
2. A `sync.WaitGroup` tracks completion.
3. Passing `i, v` as arguments captures them per-iteration safely.

```go
package main

import (
	"fmt"
	"sync"
)

// ParallelMap applies f to every element concurrently, preserving input order.
func ParallelMap[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	var wg sync.WaitGroup
	for i, v := range s {
		wg.Add(1)
		// Each goroutine writes its own index, so no lock is needed.
		go func(i int, v T) {
			defer wg.Done()
			out[i] = f(v)
		}(i, v)
	}
	wg.Wait()
	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5}
	squares := ParallelMap(nums, func(n int) int { return n * n })
	fmt.Println(squares)
}
```

**Output:**

```
[1 4 9 16 25]
```

---

## 59. Lazy iterators with iter.Seq

`🔴 hard` · *Iterators*

Go 1.23's `iter.Seq[T]` is just `func(yield func(T) bool)` — a *pull-free* iterator you consume with `for range`. Here an **infinite** `Count` is made finite by `TakeSeq`, all without building a slice.

**Steps:**

1. `Count` yields forever; returning `false` from `yield` stops it.
2. `TakeSeq` returns after `n` values, which signals the source to stop.
3. Ranging the composed iterator terminates despite the infinite source.

```go
package main

import (
	"fmt"
	"iter"
)

// Count returns an iterator that yields start, start+1, start+2, ... forever.
// iter.Seq[int] is just func(yield func(int) bool); nothing runs until ranged.
func Count(start int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := start; ; i++ {
			if !yield(i) {
				return // consumer stopped (e.g. a break or an outer limit)
			}
		}
	}
}

// TakeSeq yields at most n values from seq, then stops the source.
func TakeSeq[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		count := 0
		for v := range seq {
			if count >= n {
				return
			}
			if !yield(v) {
				return
			}
			count++
		}
	}
}

func main() {
	// Take 5 values from an infinite counter — laziness makes this terminate.
	for v := range TakeSeq(Count(10), 5) {
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
10 11 12 13 14 
```

---

## 60. Capstone a lazy Filter→Map pipeline

`🔴 hard` · *Capstone*

A capstone for the iterator model: generic `FilterSeq` and `MapSeq` transformers over `iter.Seq[T]`. They compose into a lazy pipeline that allocates **no** intermediate slices — each value flows straight through to the final `for range`.

**Steps:**

1. `seq` lifts a slice into an `iter.Seq[T]`.
2. `FilterSeq` forwards only values that pass; `MapSeq` transforms `T → U`.
3. Wiring them builds the pipeline; ranging it pulls one value through end-to-end at a time.

```go
package main

import (
	"fmt"
	"iter"
)

// seq turns a slice into an iter.Seq.
func seq[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

// FilterSeq yields only the values for which keep returns true.
func FilterSeq[T any](s iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

// MapSeq transforms each value as it flows through — no intermediate slice.
func MapSeq[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range s {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// Lazy pipeline: keep evens, then square them. Nothing runs until we range.
	evens := FilterSeq(seq(nums), func(n int) bool { return n%2 == 0 })
	squares := MapSeq(evens, func(n int) int { return n * n })

	for v := range squares {
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
4 16 36 64 100 
```

---

*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
