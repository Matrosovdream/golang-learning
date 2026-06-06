# Step 09 — Structs · 🟢 Easy

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

## 1. Declaring a struct + keyed literal

`🟢 easy` · *Declaration & literals*

A struct groups named fields; a keyed literal (Field: value) is the clearest way to build one.

**Steps:**

1. Define type Person with Name and Age.
2. Build it with field names; %+v shows the names.

```go
package main

import "fmt"

type Person struct {
	Name string
	Age  int
}

func main() {
	p := Person{Name: "Alice", Age: 30}
	fmt.Println(p)
	fmt.Printf("%+v\n", p)
}
```

**Output:**

```
{Alice 30}
{Name:Alice Age:30}
```

---

## 2. Positional literal

`🟢 easy` · *Declaration & literals*

You can omit field names and give values in declaration order, but it's more fragile if the struct changes.

**Steps:**

1. Point{3, 4} fills X then Y.
2. All fields must be supplied, in order.

```go
package main

import "fmt"

type Point struct {
	X, Y int
}

func main() {
	p := Point{3, 4} // positional
	fmt.Println(p.X, p.Y)
}
```

**Output:**

```
3 4
```

---

## 3. Zero value of a struct

`🟢 easy` · *Declaration & literals*

A struct declared with var has every field set to its type's zero value — no constructor needed.

**Steps:**

1. var c Config zeroes all fields.
2. Strings are "", ints 0, bools false.

```go
package main

import "fmt"

type Config struct {
	Name    string
	Retries int
	Debug   bool
}

func main() {
	var c Config // all fields zero-valued
	fmt.Printf("%+v\n", c)
}
```

**Output:**

```
{Name: Retries:0 Debug:false}
```

---

## 4. Accessing and mutating fields

`🟢 easy` · *Fields*

Use the dot operator to read and assign fields on a struct value.

**Steps:**

1. Assign c.Count, then increment it.
2. Struct fields are ordinary lvalues.

```go
package main

import "fmt"

type Counter struct {
	Count int
}

func main() {
	c := Counter{}
	c.Count = 5
	c.Count++
	fmt.Println(c.Count)
}
```

**Output:**

```
6
```

---

## 5. Pointer to a struct + auto-deref

`🟢 easy` · *Pointers to structs*

With a *struct, the dot operator auto-dereferences: p.Field means (*p).Field.

**Steps:**

1. &Person{...} gives a *Person.
2. p.Name and (*p).Name are equivalent.

```go
package main

import "fmt"

type Person struct {
	Name string
}

func main() {
	p := &Person{Name: "Bob"}
	fmt.Println(p.Name) // auto-deref: same as (*p).Name
	p.Name = "Carol"
	fmt.Println((*p).Name)
}
```

**Output:**

```
Bob
Carol
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
