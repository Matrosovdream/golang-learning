# Step 09 — Structs · 🟡 Medium

Examples **6–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 6. Structs are copied by value

`🟡 medium` · *Value semantics*

Assigning a struct copies all its fields; the copy is independent of the original.

**Steps:**

1. b := a duplicates the struct.
2. Editing b leaves a unchanged.

```go
package main

import "fmt"

type Box struct {
	Value int
}

func main() {
	a := Box{Value: 1}
	b := a // copy
	b.Value = 99
	fmt.Println("a:", a.Value) // 1
	fmt.Println("b:", b.Value) // 99
}
```

**Output:**

```
a: 1
b: 99
```

---

## 7. Pass by value vs by pointer

`🟡 medium` · *Value semantics*

A function receives a copy of a struct value; to mutate the caller's struct, pass a pointer.

**Steps:**

1. byValue edits a copy — no effect outside.
2. byPointer edits the original through *Box.

```go
package main

import "fmt"

type Box struct{ V int }

func byValue(b Box)    { b.V = 100 } // edits a copy
func byPointer(b *Box) { b.V = 100 } // edits the original

func main() {
	x := Box{V: 1}
	byValue(x)
	fmt.Println("after byValue:", x.V) // 1
	byPointer(&x)
	fmt.Println("after byPointer:", x.V) // 100
}
```

**Output:**

```
after byValue: 1
after byPointer: 100
```

---

## 8. The &T{} pointer literal

`🟡 medium` · *Pointers to structs*

&Type{...} builds a struct and returns a pointer to it in one step — the common way to create heap structs.

**Steps:**

1. &Server{...} yields a *Server.
2. %T confirms the pointer type.

```go
package main

import "fmt"

type Server struct {
	Host string
	Port int
}

func main() {
	s := &Server{Host: "localhost", Port: 8080}
	fmt.Printf("%s:%d\n", s.Host, s.Port)
	fmt.Printf("%T\n", s) // *main.Server
}
```

**Output:**

```
localhost:8080
*main.Server
```

---

## 9. new(T) for a struct

`🟡 medium` · *Pointers to structs*

new(T) allocates a zeroed T and returns a *T; for structs it's equivalent to &T{}.

**Steps:**

1. new(Point) gives a *Point with zero fields.
2. Set fields through the pointer.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	p := new(Point) // *Point, fields zeroed
	p.X = 10
	fmt.Println(*p)       // {10 0}
	fmt.Printf("%T\n", p) // *main.Point
}
```

**Output:**

```
{10 0}
*main.Point
```

---

## 10. Methods with a value receiver

`🟡 medium` · *Methods*

A method with a value receiver operates on a copy of the struct — fine for read-only computations.

**Steps:**

1. Area() reads r.W and r.H.
2. Call it with the dot operator like a field.

```go
package main

import "fmt"

type Rectangle struct {
	W, H int
}

func (r Rectangle) Area() int {
	return r.W * r.H
}

func main() {
	r := Rectangle{W: 3, H: 4}
	fmt.Println("area:", r.Area())
}
```

**Output:**

```
area: 12
```

---

## 11. Methods with a pointer receiver

`🟡 medium` · *Methods*

A pointer receiver lets a method mutate the struct it's called on; Go auto-takes the address for addressable values.

**Steps:**

1. Inc() uses *Counter to mutate n.
2. c.Inc() works even though c is a value (Go takes &c).

```go
package main

import "fmt"

type Counter struct {
	n int
}

func (c *Counter) Inc()      { c.n++ }
func (c Counter) Value() int { return c.n }

func main() {
	c := Counter{}
	c.Inc()
	c.Inc()
	fmt.Println(c.Value()) // 2
}
```

**Output:**

```
2
```

---

## 12. Nested structs

`🟡 medium` · *Nested*

A struct field can itself be a struct; reach inner fields by chaining the dot operator.

**Steps:**

1. User has an Addr field of type Address.
2. Access u.Addr.City; %+v prints the nesting.

```go
package main

import "fmt"

type Address struct {
	City string
	Zip  string
}

type User struct {
	Name string
	Addr Address
}

func main() {
	u := User{
		Name: "Sam",
		Addr: Address{City: "Oslo", Zip: "0001"},
	}
	fmt.Println(u.Addr.City)
	fmt.Printf("%+v\n", u)
}
```

**Output:**

```
Oslo
{Name:Sam Addr:{City:Oslo Zip:0001}}
```

---

## 13. Slice of structs: the range-copy gotcha

`🟡 medium` · *Collections of structs*

range gives a COPY of each struct element, so mutating the loop variable does nothing — mutate through the index instead.

**Steps:**

1. Editing the range variable it leaves the slice unchanged.
2. items[i].Price *= 2 mutates the real elements.

```go
package main

import "fmt"

type Item struct {
	Name  string
	Price int
}

func main() {
	items := []Item{{"pen", 1}, {"book", 5}}

	for _, it := range items {
		it.Price *= 2 // edits a copy — no effect
	}
	fmt.Println("after range copy:", items)

	for i := range items {
		items[i].Price *= 2 // edits the real element
	}
	fmt.Println("after index:", items)
}
```

**Output:**

```
after range copy: [{pen 1} {book 5}]
after index: [{pen 2} {book 10}]
```

---

## 14. Map values are not addressable

`🟡 medium` · *Collections of structs*

You can't assign to a field of a struct stored in a map; copy the value out, edit it, and put it back (or store pointers).

**Steps:**

1. m["pen"].Price = 2 is a compile error.
2. Read into a local, modify, then reassign the whole value.

```go
package main

import "fmt"

type Item struct {
	Price int
}

func main() {
	m := map[string]Item{"pen": {Price: 1}}
	// m["pen"].Price = 2 // compile error: cannot assign to struct field in map

	it := m["pen"]
	it.Price = 2
	m["pen"] = it
	fmt.Println(m["pen"].Price)
}
```

**Output:**

```
2
```

---

## 15. Anonymous structs

`🟡 medium` · *Anonymous*

You can declare and instantiate a struct type inline without naming it — handy for one-off data.

**Steps:**

1. The type is written right before its literal.
2. Useful for table-driven tests and quick groupings.

```go
package main

import "fmt"

func main() {
	point := struct {
		X, Y int
	}{X: 1, Y: 2}
	fmt.Printf("%+v\n", point)
}
```

**Output:**

```
{X:1 Y:2}
```

---

## 16. Struct comparability with ==

`🟡 medium` · *Comparability*

Two structs are == when all their fields are comparable and equal.

**Steps:**

1. Compare Points field-by-field with ==.
2. Equal fields give true; any difference gives false.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	a := Point{1, 2}
	b := Point{1, 2}
	c := Point{3, 4}
	fmt.Println(a == b) // true
	fmt.Println(a == c) // false
}
```

**Output:**

```
true
false
```

---

## 17. A slice/map field breaks comparability

`🟡 medium` · *Comparability*

If a struct contains a slice or map field, == won't compile — those types aren't comparable.

**Steps:**

1. The commented a == b line is a compile error.
2. Compare the relevant parts manually instead.

```go
package main

import "fmt"

type Bad struct {
	Tags []string // slice field => struct is not comparable
}

func main() {
	a := Bad{Tags: []string{"x"}}
	b := Bad{Tags: []string{"x"}}
	// fmt.Println(a == b) // compile error: struct containing []string cannot be compared
	fmt.Println("compare manually:", len(a.Tags) == len(b.Tags))
}
```

**Output:**

```
compare manually: true
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
