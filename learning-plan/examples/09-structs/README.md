# Step 09 — Structs · Examples

A library of **26 runnable examples**. Each is a complete `package main` program:
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

- [1. Declaring a struct + keyed literal](#1-declaring-a-struct--keyed-literal)
- [2. Positional literal](#2-positional-literal)
- [3. Zero value of a struct](#3-zero-value-of-a-struct)
- [4. Accessing and mutating fields](#4-accessing-and-mutating-fields)
- [5. Pointer to a struct + auto-deref](#5-pointer-to-a-struct--auto-deref)

**Medium**

- [6. Structs are copied by value](#6-structs-are-copied-by-value)
- [7. Pass by value vs by pointer](#7-pass-by-value-vs-by-pointer)
- [8. The &T{} pointer literal](#8-the-t-pointer-literal)
- [9. new(T) for a struct](#9-newt-for-a-struct)
- [10. Methods with a value receiver](#10-methods-with-a-value-receiver)
- [11. Methods with a pointer receiver](#11-methods-with-a-pointer-receiver)
- [12. Nested structs](#12-nested-structs)
- [13. Slice of structs: the range-copy gotcha](#13-slice-of-structs-the-range-copy-gotcha)
- [14. Map values are not addressable](#14-map-values-are-not-addressable)
- [15. Anonymous structs](#15-anonymous-structs)
- [16. Struct comparability with ==](#16-struct-comparability-with-)
- [17. A slice/map field breaks comparability](#17-a-slicemap-field-breaks-comparability)

**Hard**

- [18. Embedding: field promotion](#18-embedding-field-promotion)
- [19. Embedding: method promotion](#19-embedding-method-promotion)
- [20. Embedding: shadowing](#20-embedding-shadowing)
- [21. Embedding to satisfy an interface](#21-embedding-to-satisfy-an-interface)
- [22. JSON: Marshal with struct tags](#22-json-marshal-with-struct-tags)
- [23. JSON: Unmarshal into a struct](#23-json-unmarshal-into-a-struct)
- [24. JSON: omitempty and json:"-"](#24-json-omitempty-and-json-)
- [25. Constructor functions (NewX)](#25-constructor-functions-newx)
- [26. The empty struct struct{}](#26-the-empty-struct-struct)

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

## 18. Embedding: field promotion

`🔴 hard` · *Embedding*

An embedded (anonymous) field's fields are promoted, so you can access them directly on the outer struct.

**Steps:**

1. Dog embeds Animal.
2. d.Name reaches the promoted Animal.Name.

```go
package main

import "fmt"

type Animal struct {
	Name string
}

type Dog struct {
	Animal // embedded
	Breed  string
}

func main() {
	d := Dog{Animal: Animal{Name: "Rex"}, Breed: "Lab"}
	fmt.Println(d.Name, d.Breed) // Name promoted from Animal
}
```

**Output:**

```
Rex Lab
```

---

## 19. Embedding: method promotion

`🔴 hard` · *Embedding*

Methods of an embedded type are promoted too, so the outer struct can call them as if they were its own.

**Steps:**

1. Animal has Speak().
2. Dog embeds Animal and can call d.Speak() directly.

```go
package main

import "fmt"

type Animal struct {
	Name string
}

func (a Animal) Speak() string {
	return a.Name + " makes a sound"
}

type Dog struct {
	Animal
}

func main() {
	d := Dog{Animal{Name: "Rex"}}
	fmt.Println(d.Speak()) // promoted from Animal
}
```

**Output:**

```
Rex makes a sound
```

---

## 20. Embedding: shadowing

`🔴 hard` · *Embedding*

A field or method on the outer struct shadows the embedded one with the same name; the embedded one is still reachable via the type name.

**Steps:**

1. Derived.Name and Derived.Describe shadow Base's.
2. d.Base.Name still reaches the inner field.

```go
package main

import "fmt"

type Base struct {
	Name string
}

func (b Base) Describe() string { return "base:" + b.Name }

type Derived struct {
	Base
	Name string // shadows Base.Name
}

func (d Derived) Describe() string { return "derived:" + d.Name } // shadows Base.Describe

func main() {
	d := Derived{Base: Base{Name: "B"}, Name: "D"}
	fmt.Println(d.Name)       // D (outer)
	fmt.Println(d.Base.Name)  // B (inner, still reachable)
	fmt.Println(d.Describe()) // derived:D
}
```

**Output:**

```
D
B
derived:D
```

---

## 21. Embedding to satisfy an interface

`🔴 hard` · *Embedding*

Because embedded methods are promoted, embedding a type that already implements an interface makes the outer struct implement it too.

**Steps:**

1. Animal implements Speaker.
2. Dog embeds Animal, so a Dog is also a Speaker.

```go
package main

import "fmt"

type Speaker interface {
	Speak() string
}

type Animal struct{ Name string }

func (a Animal) Speak() string { return a.Name + " speaks" }

type Dog struct {
	Animal // Dog gets Speak() via embedding
}

func main() {
	var s Speaker = Dog{Animal{Name: "Rex"}}
	fmt.Println(s.Speak())
}
```

**Output:**

```
Rex speaks
```

---

## 22. JSON: Marshal with struct tags

`🔴 hard` · *JSON*

encoding/json maps struct fields to JSON; `json:"name"` tags rename keys. Only exported (capitalized) fields are marshaled.

**Steps:**

1. Tag each field with its JSON key.
2. json.Marshal returns the bytes; convert to string to print.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func main() {
	u := User{Name: "Alice", Email: "a@x.com", Age: 30}
	b, _ := json.Marshal(u)
	fmt.Println(string(b))
}
```

**Output:**

```
{"name":"Alice","email":"a@x.com","age":30}
```

---

## 23. JSON: Unmarshal into a struct

`🔴 hard` · *JSON*

json.Unmarshal fills a struct from JSON bytes; pass a pointer so it can write into your value.

**Steps:**

1. Provide a *User to Unmarshal.
2. Matching tags populate the fields; check the error.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func main() {
	data := `{"name":"Bob","age":25}`
	var u User
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%+v\n", u)
}
```

**Output:**

```
{Name:Bob Age:25}
```

---

## 24. JSON: omitempty and json:"-"

`🔴 hard` · *JSON*

The omitempty option drops zero-valued fields from the output; the tag json:"-" excludes a field entirely (e.g. secrets).

**Steps:**

1. Nickname is empty, so omitempty drops it.
2. Password has json:"-" and is never marshaled.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Account struct {
	User     string `json:"user"`
	Nickname string `json:"nickname,omitempty"`
	Password string `json:"-"`
}

func main() {
	a := Account{User: "alice", Password: "secret"}
	b, _ := json.Marshal(a)
	fmt.Println(string(b))
}
```

**Output:**

```
{"user":"alice"}
```

---

## 25. Constructor functions (NewX)

`🔴 hard` · *Constructors*

Go has no constructors; the idiom is a NewX function that validates inputs and returns (*T, error).

**Steps:**

1. NewAccount checks its arguments.
2. It returns a pointer on success, or an error.

```go
package main

import (
	"errors"
	"fmt"
)

type Account struct {
	Owner   string
	Balance int
}

func NewAccount(owner string, initial int) (*Account, error) {
	if owner == "" {
		return nil, errors.New("owner required")
	}
	if initial < 0 {
		return nil, errors.New("initial balance cannot be negative")
	}
	return &Account{Owner: owner, Balance: initial}, nil
}

func main() {
	a, err := NewAccount("Alice", 100)
	fmt.Println(a, err)
	_, err = NewAccount("", 0)
	fmt.Println("error:", err)
}
```

**Output:**

```
&{Alice 100} <nil>
error: owner required
```

---

## 26. The empty struct struct{}

`🔴 hard` · *Empty struct*

struct{} has zero size, making it the idiomatic value for sets (map[T]struct{}) and pure signaling.

**Steps:**

1. unsafe.Sizeof(struct{}{}) is 0.
2. Use it as a map value to build a memory-free set.

```go
package main

import (
	"fmt"
	"sort"
	"unsafe"
)

func main() {
	fmt.Println("size of struct{}:", unsafe.Sizeof(struct{}{}))

	set := map[string]struct{}{}
	for _, w := range []string{"a", "b", "a"} {
		set[w] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("unique:", keys)
}
```

**Output:**

```
size of struct{}: 0
unique: [a b]
```

---

