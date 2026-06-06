# Step 10 — Pointers & Methods · 🔴 Hard

Examples **17–24**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 17. Method sets: T vs *T

`🔴 hard` · *Method sets*

The method set of *T includes both value- and pointer-receiver methods; an addressable T can call either thanks to auto-addressing.

**Steps:**

1. t (a value) and p (a pointer) both call Val and Ptr methods.
2. Auto-addressing makes the value form work.

```go
package main

import "fmt"

type T struct{ v int }

func (t T) ValMethod() int  { return t.v }
func (t *T) PtrMethod() int { return t.v }

func main() {
	t := T{v: 5}
	fmt.Println(t.ValMethod(), t.PtrMethod()) // 5 5

	p := &T{v: 9}
	fmt.Println(p.ValMethod(), p.PtrMethod()) // 9 9
}
```

**Output:**

```
5 5
9 9
```

---

## 18. Interface satisfaction needs *T for pointer methods

`🔴 hard` · *Method sets*

If a method has a pointer receiver, only *T satisfies an interface requiring it — a plain T value does not.

**Steps:**

1. String() has a *Temp receiver.
2. var s Stringer = &Temp{...} compiles; the value form (commented) does not.

```go
package main

import "fmt"

type Stringer interface{ String() string }

type Temp struct{ c int }

func (t *Temp) String() string { return fmt.Sprintf("%dC", t.c) }

func main() {
	var s Stringer = &Temp{c: 20} // ok: *Temp implements Stringer
	// var bad Stringer = Temp{c: 20} // compile error: Temp does not implement Stringer
	fmt.Println(s.String())
}
```

**Output:**

```
20C
```

---

## 19. Map elements are not addressable

`🔴 hard` · *Addressability*

You can't call a pointer-receiver method directly on a map element because it isn't addressable; copy out, mutate, and put back (or store pointers).

**Steps:**

1. m["a"].Inc() won't compile.
2. Read the value, Inc it, then write it back.

```go
package main

import "fmt"

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }

func main() {
	m := map[string]Counter{"a": {}}
	// m["a"].Inc() // compile error: cannot call pointer method on m["a"] (not addressable)
	c := m["a"]
	c.Inc()
	m["a"] = c
	fmt.Println(m["a"].n) // 1
}
```

**Output:**

```
1
```

---

## 20. Double pointers (**T)

`🔴 hard` · *Double pointers*

A pointer can point to another pointer; dereference twice (**pp) to reach the underlying value.

**Steps:**

1. pp := &p makes a **int.
2. **pp = 99 writes through both levels.

```go
package main

import "fmt"

func main() {
	x := 1
	p := &x
	pp := &p // pointer to a pointer
	**pp = 99
	fmt.Println(x)    // 99
	fmt.Println(**pp) // 99
}
```

**Output:**

```
99
99
```

---

## 21. Nil receiver methods

`🔴 hard` · *Nil receivers*

A pointer-receiver method can be called on a nil pointer as long as it checks for nil — the basis of recursive structures like linked lists.

**Steps:**

1. Sum returns 0 when the receiver is nil (the base case).
2. It safely walks the list and even handles an empty (nil) list.

```go
package main

import "fmt"

type Node struct {
	Val  int
	Next *Node
}

func (n *Node) Sum() int {
	if n == nil {
		return 0 // nil receiver is fine here
	}
	return n.Val + n.Next.Sum()
}

func main() {
	list := &Node{1, &Node{2, &Node{3, nil}}}
	fmt.Println(list.Sum()) // 6

	var empty *Node
	fmt.Println(empty.Sum()) // 0
}
```

**Output:**

```
6
0
```

---

## 22. Comparing pointers

`🔴 hard` · *Pointer equality*

== on pointers compares addresses: two pointers are equal only if they point at the same variable.

**Steps:**

1. p1 and p2 both address a -> equal.
2. p3 addresses a different variable -> not equal.

```go
package main

import "fmt"

func main() {
	a := 1
	b := 1
	p1 := &a
	p2 := &a
	p3 := &b
	fmt.Println(p1 == p2) // true: same address
	fmt.Println(p1 == p3) // false: different variables
}
```

**Output:**

```
true
false
```

---

## 23. &T{} vs new(T)

`🔴 hard` · *Allocation*

&T{} and new(T) both allocate a zeroed T and return a *T — use whichever reads better.

**Steps:**

1. a := &Point{} and b := new(Point) are equivalent.
2. Their dereferenced values are equal and both are *Point.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	a := &Point{}               // pointer to zeroed Point
	b := new(Point)             // identical effect
	fmt.Println(*a == *b)       // true
	fmt.Printf("%T %T\n", a, b) // *main.Point *main.Point
}
```

**Output:**

```
true
*main.Point *main.Point
```

---

## 24. Pointer receiver to grow a slice field

`🔴 hard` · *Receivers*

Mutating methods that append to a slice field must use a pointer receiver, so the reassigned slice header sticks.

**Steps:**

1. Push uses *Stack and reassigns s.items via append.
2. A value receiver would lose the appended elements.

```go
package main

import "fmt"

type Stack struct {
	items []int
}

func (s *Stack) Push(v int) {
	s.items = append(s.items, v)
}

func main() {
	s := &Stack{}
	s.Push(1)
	s.Push(2)
	s.Push(3)
	fmt.Println(s.items) // [1 2 3]
}
```

**Output:**

```
[1 2 3]
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
