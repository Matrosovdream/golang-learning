# Step 17 — Generics · 🟢 Easy

Examples **1–10**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. A generic function with a type parameter

`🟢 easy` · *Functions*

A generic function takes type parameters in square brackets after its name. Here `T` is constrained by `any`, so `First` works on a slice of any element type.

**Steps:**

1. `[T any]` declares one type parameter named `T`.
2. `s []T` and the return value both refer to it.
3. The compiler infers `T` from the argument at each call.

```go
package main

import "fmt"

// First returns the first element of any slice. T is a type parameter
// constrained by `any`, so this works for []int, []string, anything.
func First[T any](s []T) T {
	return s[0]
}

func main() {
	fmt.Println(First([]int{10, 20, 30}))
	fmt.Println(First([]string{"go", "rust", "zig"}))
}
```

**Output:**

```
10
go
```

---

## 2. The any constraint holds any type

`🟢 easy` · *Constraints*

`any` is an alias for `interface{}` and is the loosest constraint. With it you can store and pass values of `T`, but only do what is valid for *every* type.

**Steps:**

1. `Last` is declared once and called with three different element types.
2. No type arguments are written — each is inferred.

```go
package main

import "fmt"

// Last works for a slice of any element type.
func Last[T any](s []T) T {
	return s[len(s)-1]
}

func main() {
	fmt.Println(Last([]int{1, 2, 3}))
	fmt.Println(Last([]float64{1.5, 2.5}))
	fmt.Println(Last([]bool{true, false}))
}
```

**Output:**

```
3
2.5
false
```

---

## 3. Type inference vs explicit type arguments

`🟢 easy` · *Inference*

Usually the compiler infers type arguments from the values you pass. Write them explicitly as `Name[Type](...)` only when inference can't determine them.

**Steps:**

1. `Wrap(42)` infers `T = int`.
2. `Wrap[string]("hi")` states `T` explicitly.
3. `%T` prints the concrete element type.

```go
package main

import "fmt"

func Wrap[T any](v T) []T {
	return []T{v}
}

func main() {
	a := Wrap(42)           // T inferred as int from the argument
	b := Wrap[string]("hi") // T given explicitly
	fmt.Printf("%T %v\n", a, a)
	fmt.Printf("%T %v\n", b, b)
}
```

**Output:**

```
[]int [42]
[]string [hi]
```

---

## 4. The zero value of a type parameter

`🟢 easy` · *Zero values*

You can't write `0` or `nil` for a value of type parameter `T` — the right zero depends on `T`. `var zero T` produces it for whatever `T` turns out to be.

**Steps:**

1. `var zero T` is the idiom for a type-parameter zero value.
2. It yields `""`, `0`, `false`, or `nil` as appropriate.

```go
package main

import "fmt"

// Zero returns the zero value for any type T. You cannot write `return 0`
// or `return nil`; `var zero T` gives the right zero for whatever T is.
func Zero[T any]() T {
	var zero T
	return zero
}

func main() {
	fmt.Printf("%q\n", Zero[string]())
	fmt.Println(Zero[int]())
	fmt.Println(Zero[bool]())
	fmt.Println(Zero[[]int]() == nil)
}
```

**Output:**

```
""
0
false
true
```

---

## 5. The comparable constraint

`🟢 easy` · *Constraints*

`comparable` is a built-in constraint for types usable with `==` and `!=`. Use it whenever a generic function compares values or uses them as map keys.

**Steps:**

1. `[T comparable]` permits `v == target`.
2. Using `any` here would not compile.

```go
package main

import "fmt"

// comparable allows == and !=, needed to compare elements against target.
func Contains[T comparable](s []T, target T) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

func main() {
	fmt.Println(Contains([]int{1, 2, 3}, 2))
	fmt.Println(Contains([]string{"a", "b"}, "z"))
}
```

**Output:**

```
true
false
```

---

## 6. Map transform a slice to a new type

`🟢 easy` · *Algorithms*

`Map` is the canonical two-parameter generic: input element `T`, output element `U`. It preserves type safety across the transformation — no `any`, no casts.

**Steps:**

1. `f func(T) U` converts each element.
2. The result is a brand-new `[]U`.
3. `T` and `U` are both inferred from `nums` and `f`.

```go
package main

import (
	"fmt"
	"strconv"
)

// Map applies f to each element, producing a new slice of type []U.
// Two type parameters: input element T and output element U.
func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func main() {
	nums := []int{1, 2, 3}
	strs := Map(nums, func(n int) string { return strconv.Itoa(n * n) })
	fmt.Println(strs)
}
```

**Output:**

```
[1 4 9]
```

---

## 7. Filter keep matching elements

`🟢 easy` · *Algorithms*

`Filter` keeps only the elements for which the predicate returns true. The element type is preserved, so the result is still `[]T`.

**Steps:**

1. `keep func(T) bool` decides each element.
2. Append-to-nil builds the result slice.

```go
package main

import "fmt"

// Filter returns a new slice with only the elements for which keep is true.
func Filter[T any](s []T, keep func(T) bool) []T {
	var out []T
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6}
	even := Filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println(even)
}
```

**Output:**

```
[2 4 6]
```

---

## 8. A type union constraint Number

`🟢 easy` · *Constraints*

To use operators like `+`, the constraint must list the allowed types. A constraint interface can be a *union* of types written with `|`.

**Steps:**

1. `Number` unions `~int | ~int64 | ~float64`.
2. `+=` is allowed because every listed type supports it.
3. `var total T` starts the accumulator at zero.

```go
package main

import "fmt"

// Number is a constraint: T must be one of these types so that + is allowed.
type Number interface {
	~int | ~int64 | ~float64
}

func Sum[T Number](nums []T) T {
	var total T
	for _, n := range nums {
		total += n
	}
	return total
}

func main() {
	fmt.Println(Sum([]int{1, 2, 3, 4}))
	fmt.Println(Sum([]float64{1.5, 2.5, 3}))
}
```

**Output:**

```
10
7
```

---

## 9. Underlying types and the tilde

`🟢 easy` · *Constraints*

The `~` (tilde) means "any type whose **underlying** type is this". Without it, a named type like `type Celsius int` would not satisfy the constraint.

**Steps:**

1. `~int` admits `Celsius` (underlying type `int`).
2. Plain `int` literals satisfy it too.
3. Drop the `~` and `Double(c)` would fail to compile.

```go
package main

import "fmt"

// With ~int, any type whose UNDERLYING type is int satisfies the constraint.
type Integer interface {
	~int
}

type Celsius int // named type with underlying type int

func Double[T Integer](v T) T {
	return v * 2
}

func main() {
	var c Celsius = 21
	fmt.Println(Double(c)) // works because of ~int
	fmt.Println(Double(5)) // a plain int works too
}
```

**Output:**

```
42
10
```

---

## 10. Generic over a map with Keys

`🟢 easy` · *Maps*

Map keys are always comparable, so `K comparable` fits perfectly; the value type `V` can be anything. Map iteration order is random, so sort before printing.

**Steps:**

1. `[K comparable, V any]` mirrors a map's own type rules.
2. Collect keys, then `sort.Strings` for stable output.

```go
package main

import (
	"fmt"
	"sort"
)

// Keys returns a map's keys. K must be comparable (map keys always are);
// V can be anything.
func Keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func main() {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	keys := Keys(m)
	sort.Strings(keys) // map order is random; sort for stable output
	fmt.Println(keys)
}
```

**Output:**

```
[a b c]
```

---

*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
