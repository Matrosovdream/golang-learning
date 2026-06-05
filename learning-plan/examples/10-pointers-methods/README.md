# Step 10 — Pointers & Methods · Examples

A library of **24 runnable examples**. Each is a complete `package main` program:
read the concept and steps, then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** is real stdout.

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them.

## Index


**Easy**

- [1. & and * basics](#1--and--basics)
- [2. Pointer zero value is nil](#2-pointer-zero-value-is-nil)
- [3. Two pointers can alias one variable](#3-two-pointers-can-alias-one-variable)
- [4. new(T) allocates a zeroed value](#4-newt-allocates-a-zeroed-value)
- [5. Pointer to a struct + auto-deref](#5-pointer-to-a-struct--auto-deref)

**Medium**

- [6. nil pointer dereference panics](#6-nil-pointer-dereference-panics)
- [7. Pass a pointer to mutate the caller's variable](#7-pass-a-pointer-to-mutate-the-callers-variable)
- [8. Swap two values via pointers](#8-swap-two-values-via-pointers)
- [9. Returning a pointer to a local (escape analysis)](#9-returning-a-pointer-to-a-local-escape-analysis)
- [10. Value vs pointer receiver (mutation)](#10-value-vs-pointer-receiver-mutation)
- [11. Pointer-receiver auto-addressing](#11-pointer-receiver-auto-addressing)
- [12. Pointer to an array element](#12-pointer-to-an-array-element)
- [13. Pointer to a slice element](#13-pointer-to-a-slice-element)
- [14. Modify a slice via index, not the range copy](#14-modify-a-slice-via-index-not-the-range-copy)
- [15. []*T vs []T](#15-t-vs-t)
- [16. map[K]*V to mutate stored values](#16-mapkv-to-mutate-stored-values)

**Hard**

- [17. Method sets: T vs *T](#17-method-sets-t-vs-t)
- [18. Interface satisfaction needs *T for pointer methods](#18-interface-satisfaction-needs-t-for-pointer-methods)
- [19. Map elements are not addressable](#19-map-elements-are-not-addressable)
- [20. Double pointers (**T)](#20-double-pointers-t)
- [21. Nil receiver methods](#21-nil-receiver-methods)
- [22. Comparing pointers](#22-comparing-pointers)
- [23. &T{} vs new(T)](#23-t-vs-newt)
- [24. Pointer receiver to grow a slice field](#24-pointer-receiver-to-grow-a-slice-field)

---

## 1. & and * basics

`🟢 easy` · *Pointer basics*

&x takes the address of x; *p dereferences a pointer to read or write the value it points to.

**Steps:**

1. p := &x stores x's address.
2. *p reads it; *p = ... writes back into x.

```go
package main

import "fmt"

func main() {
	x := 42
	p := &x                   // address of x
	fmt.Println("value:", *p) // dereference to read
	*p = 100                  // dereference to write
	fmt.Println("x is now:", x)
}
```

**Output:**

```
value: 42
x is now: 100
```

---

## 2. Pointer zero value is nil

`🟢 easy` · *Pointer basics*

An uninitialized pointer is nil; assign it an address before dereferencing.

**Steps:**

1. var p *int starts as nil.
2. After p = &x, *p is valid.

```go
package main

import "fmt"

func main() {
	var p *int // zero value is nil
	fmt.Println("p == nil:", p == nil)
	x := 5
	p = &x
	fmt.Println("now:", *p)
}
```

**Output:**

```
p == nil: true
now: 5
```

---

## 3. Two pointers can alias one variable

`🟢 easy` · *Pointer basics*

Several pointers can hold the same address; writing through one is visible through the others, and == compares addresses.

**Steps:**

1. p and q both point to x.
2. *p = 20 is seen via *q; p == q is true.

```go
package main

import "fmt"

func main() {
	x := 10
	p := &x
	q := &x // same address as p
	*p = 20
	fmt.Println(*q)     // 20
	fmt.Println(p == q) // true
}
```

**Output:**

```
20
true
```

---

## 4. new(T) allocates a zeroed value

`🟢 easy` · *Allocation*

new(T) allocates a zeroed T and returns a *T pointing to it.

**Steps:**

1. new(int) gives a *int whose value is 0.
2. Write through the pointer with *p.

```go
package main

import "fmt"

func main() {
	p := new(int) // *int -> 0
	fmt.Println(*p)
	*p = 7
	fmt.Println(*p)
}
```

**Output:**

```
0
7
```

---

## 5. Pointer to a struct + auto-deref

`🟢 easy` · *Pointers to structs*

On a *struct the dot operator auto-dereferences, so p.Field means (*p).Field.

**Steps:**

1. &Point{...} yields a *Point.
2. p.X = 10 writes through the pointer.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	p := &Point{X: 1, Y: 2}
	p.X = 10 // same as (*p).X = 10
	fmt.Println(*p)
}
```

**Output:**

```
{10 2}
```

---

## 6. nil pointer dereference panics

`🟡 medium` · *Nil & safety*

Dereferencing a nil pointer panics at runtime; guard with a nil check, and recover can catch it.

**Steps:**

1. Check p == nil before using *p.
2. Dereferencing nil panics; a deferred recover reports it.

```go
package main

import "fmt"

func main() {
	var p *int
	if p == nil {
		fmt.Println("p is nil, not dereferencing")
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered:", r)
		}
	}()
	fmt.Println(*p) // panics: nil dereference
}
```

**Output:**

```
p is nil, not dereferencing
recovered: runtime error: invalid memory address or nil pointer dereference
```

---

## 7. Pass a pointer to mutate the caller's variable

`🟡 medium` · *Pointers & functions*

Go passes arguments by value; to let a function change the caller's variable, pass a pointer.

**Steps:**

1. double takes *int and writes through it.
2. Call it with &x; x changes.

```go
package main

import "fmt"

func double(n *int) {
	*n *= 2
}

func main() {
	x := 21
	double(&x)
	fmt.Println(x) // 42
}
```

**Output:**

```
42
```

---

## 8. Swap two values via pointers

`🟡 medium` · *Pointers & functions*

Passing pointers lets a function swap the caller's variables in place.

**Steps:**

1. swap takes two *int.
2. *a, *b = *b, *a exchanges the pointed-to values.

```go
package main

import "fmt"

func swap(a, b *int) {
	*a, *b = *b, *a
}

func main() {
	x, y := 1, 2
	swap(&x, &y)
	fmt.Println(x, y) // 2 1
}
```

**Output:**

```
2 1
```

---

## 9. Returning a pointer to a local (escape analysis)

`🟡 medium` · *Escape analysis*

Unlike C, it's safe to return the address of a local in Go: the compiler moves it to the heap (it 'escapes').

**Steps:**

1. newCounter returns &n for a local n.
2. The value stays alive after the function returns.

```go
package main

import "fmt"

func newCounter() *int {
	n := 0
	return &n // safe: n escapes to the heap
}

func main() {
	p := newCounter()
	*p++
	*p++
	fmt.Println(*p) // 2
}
```

**Output:**

```
2
```

---

## 10. Value vs pointer receiver (mutation)

`🟡 medium` · *Receivers*

A value-receiver method gets a copy and can't mutate the original; a pointer-receiver method can.

**Steps:**

1. IncValue edits a copy — no lasting effect.
2. IncPtr uses *Counter and changes the real value.

```go
package main

import "fmt"

type Counter struct{ n int }

func (c Counter) IncValue() { c.n++ } // edits a copy
func (c *Counter) IncPtr()  { c.n++ } // edits the original

func main() {
	c := Counter{}
	c.IncValue()
	fmt.Println("after IncValue:", c.n) // 0
	c.IncPtr()
	fmt.Println("after IncPtr:", c.n) // 1
}
```

**Output:**

```
after IncValue: 0
after IncPtr: 1
```

---

## 11. Pointer-receiver auto-addressing

`🟡 medium` · *Receivers*

Calling a pointer-receiver method on an addressable value works because Go automatically takes its address.

**Steps:**

1. c is an addressable variable.
2. c.Inc() is shorthand for (&c).Inc().

```go
package main

import "fmt"

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }

func main() {
	c := Counter{} // addressable
	c.Inc()        // Go takes &c automatically
	c.Inc()
	fmt.Println(c.n) // 2
}
```

**Output:**

```
2
```

---

## 12. Pointer to an array element

`🟡 medium` · *Pointers into collections*

You can take the address of an array element and mutate the array through it.

**Steps:**

1. p := &arr[1] points into the array.
2. *p = 99 changes arr.

```go
package main

import "fmt"

func main() {
	arr := [3]int{1, 2, 3}
	p := &arr[1]
	*p = 99
	fmt.Println(arr) // [1 99 3]
}
```

**Output:**

```
[1 99 3]
```

---

## 13. Pointer to a slice element

`🟡 medium` · *Pointers into collections*

Slice elements are addressable, so a pointer into a slice mutates the backing array in place.

**Steps:**

1. p := &s[2] addresses the third element.
2. *p += 5 updates the slice.

```go
package main

import "fmt"

func main() {
	s := []int{10, 20, 30}
	p := &s[2]
	*p += 5
	fmt.Println(s) // [10 20 35]
}
```

**Output:**

```
[10 20 35]
```

---

## 14. Modify a slice via index, not the range copy

`🟡 medium` · *Pointers into collections*

range gives a copy of each element; assign through s[i] to actually change the slice.

**Steps:**

1. Editing the range value v does nothing.
2. s[i] *= 10 mutates the elements.

```go
package main

import "fmt"

func main() {
	s := []int{1, 2, 3}
	for _, v := range s {
		v *= 10 // copy — no effect
	}
	fmt.Println("after range:", s)
	for i := range s {
		s[i] *= 10 // real
	}
	fmt.Println("after index:", s)
}
```

**Output:**

```
after range: [1 2 3]
after index: [10 20 30]
```

---

## 15. []*T vs []T

`🟡 medium` · *Pointers into collections*

A slice of values copies on range; a slice of pointers shares the pointed-to data, so range can mutate it.

**Steps:**

1. Ranging []Box edits copies — originals unchanged.
2. Ranging []*Box mutates the shared structs.

```go
package main

import "fmt"

type Box struct{ V int }

func main() {
	vals := []Box{{1}, {2}}
	for _, b := range vals {
		b.V = 0 // copy — no effect
	}

	ptrs := []*Box{{1}, {2}}
	for _, b := range ptrs {
		b.V = 0 // mutates the pointed-to Box
	}

	fmt.Println("vals:", vals[0].V, vals[1].V) // 1 2
	fmt.Println("ptrs:", ptrs[0].V, ptrs[1].V) // 0 0
}
```

**Output:**

```
vals: 1 2
ptrs: 0 0
```

---

## 16. map[K]*V to mutate stored values

`🟡 medium` · *Pointers into collections*

Since map values aren't addressable, store pointers when you want to mutate entries in place.

**Steps:**

1. The map holds *Account values.
2. m["alice"].Balance += 50 edits the pointed-to struct directly.

```go
package main

import "fmt"

type Account struct{ Balance int }

func main() {
	m := map[string]*Account{"alice": {Balance: 100}}
	m["alice"].Balance += 50        // works because the value is a pointer
	fmt.Println(m["alice"].Balance) // 150
}
```

**Output:**

```
150
```

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

