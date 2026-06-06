# Step 10 — Pointers & Methods · 🟡 Medium

Examples **6–16**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
