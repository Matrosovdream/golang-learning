# Step 10 — Pointers & Methods · 🟢 Easy

Examples **1–5**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
