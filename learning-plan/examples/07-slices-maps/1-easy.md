# Step 07 — Arrays, Slices & Maps · 🟢 Easy

Examples **1–7**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Arrays have a fixed length

`🟢 easy` · *Arrays*

An array's length is part of its type ([3]int), it is zero-valued on declaration, and len gives its size.

**Steps:**

1. var a [3]int starts as three zeros.
2. Assign by index; len(a) is the fixed size.

```go
package main

import "fmt"

func main() {
	var a [3]int // fixed length 3, zero-valued
	a[0] = 10
	a[2] = 30
	fmt.Println(a, "len:", len(a))

	b := [3]string{"x", "y", "z"}
	fmt.Println(b)
}
```

**Output:**

```
[10 0 30] len: 3
[x y z]
```

---

## 2. Arrays are copied by value

`🟢 easy` · *Arrays*

Assigning or passing an array copies the whole thing — the copy is independent of the original.

**Steps:**

1. b := a duplicates the array.
2. Changing b leaves a untouched.

```go
package main

import "fmt"

func main() {
	a := [3]int{1, 2, 3}
	b := a // full copy
	b[0] = 99
	fmt.Println("a:", a) // unchanged
	fmt.Println("b:", b)
}
```

**Output:**

```
a: [1 2 3]
b: [99 2 3]
```

---

## 3. Slice literal and indexing

`🟢 easy` · *Slices*

A slice is a flexible view over elements; create one with a literal and read/write by index.

**Steps:**

1. []int{...} (no length) is a slice, not an array.
2. Index to read and to assign.

```go
package main

import "fmt"

func main() {
	s := []int{10, 20, 30}
	fmt.Println(s[0], s[2])
	s[1] = 99
	fmt.Println(s)
}
```

**Output:**

```
10 30
[10 99 30]
```

---

## 4. len vs cap

`🟢 easy` · *Slices*

A slice has a length (elements in use) and a capacity (room in its backing array); make([]T, len, cap) sets both.

**Steps:**

1. make([]int, 2, 5) gives len 2, cap 5.
2. Appending fills the spare capacity before the slice has to grow.

```go
package main

import "fmt"

func main() {
	s := make([]int, 2, 5) // len 2, cap 5
	fmt.Println("len:", len(s), "cap:", cap(s))
	s = append(s, 1, 2, 3)
	fmt.Println("after append -> len:", len(s), "cap:", cap(s))
}
```

**Output:**

```
len: 2 cap: 5
after append -> len: 5 cap: 5
```

---

## 5. append to a slice

`🟢 easy` · *Slices*

append adds elements and returns the (possibly new) slice; it even works on a nil slice.

**Steps:**

1. Start from a nil slice.
2. append one or several values at a time; reassign the result.

```go
package main

import "fmt"

func main() {
	var s []int // nil slice
	s = append(s, 1)
	s = append(s, 2, 3)
	fmt.Println(s, "len:", len(s))
}
```

**Output:**

```
[1 2 3] len: 3
```

---

## 6. Map literal and lookup

`🟢 easy` · *Maps*

A map associates keys with values; a missing key returns the value type's zero value.

**Steps:**

1. Create with a literal, then read/write by key.
2. Reading an absent key gives 0 (for int), not an error.

```go
package main

import "fmt"

func main() {
	ages := map[string]int{"alice": 30, "bob": 25}
	fmt.Println(ages["alice"])
	ages["carol"] = 35
	fmt.Println(ages["carol"])
	fmt.Println("missing:", ages["dave"]) // zero value 0
}
```

**Output:**

```
30
35
missing: 0
```

---

## 7. Map comma-ok lookup

`🟢 easy` · *Maps*

The two-value form v, ok := m[k] tells you whether the key was actually present, distinguishing 'missing' from 'zero value'.

**Steps:**

1. ok is true only if the key exists.
2. Use it to tell a stored 0 apart from an absent key.

```go
package main

import "fmt"

func main() {
	m := map[string]int{"x": 1}
	v, ok := m["x"]
	fmt.Println("x:", v, ok)
	v, ok = m["y"]
	fmt.Println("y:", v, ok) // 0 false
}
```

**Output:**

```
x: 1 true
y: 0 false
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
