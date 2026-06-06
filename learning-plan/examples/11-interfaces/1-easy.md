# Step 11 — Interfaces · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Minimal interface & implicit satisfaction

`🟢 easy`

A type satisfies a Go interface just by having the right method — there's no "implements" keyword. This implicit (structural) satisfaction is the foundation of every interface you'll write.

**Steps:**

1. Declare `Speaker` as a one-method interface: any type with `Speak() string` counts as a Speaker.
2. Define the concrete type `Dog` and give it a `Speak` method — nowhere do you write `Dog implements Speaker`; the method alone makes it true.
3. In `main`, assign `Dog{...}` to a `Speaker` variable. It only compiles because Dog's method set covers the interface.
4. Call `s.Speak()` through the interface variable; Go dispatches to `Dog.Speak` and prints the result.

```go
package main

import "fmt"

// Speaker is a one-method interface. Any type with a Speak() string
// method satisfies it.
type Speaker interface {
	Speak() string
}

// Dog is a concrete type. Notice: there is NO "implements Speaker"
// keyword anywhere. Defining the Speak method is ALL it takes —
// Go satisfies interfaces implicitly (structurally).
type Dog struct {
	Name string
}

func (d Dog) Speak() string {
	return d.Name + " says woof"
}

func main() {
	// Assign a Dog value to a Speaker variable. This compiles only
	// because Dog has the required method set.
	var s Speaker = Dog{Name: "Rex"}

	// Call through the interface; Go dispatches to Dog.Speak.
	fmt.Println("Speaker says:", s.Speak())
}
```

**Output:**

```
Speaker says: Rex says woof
```

---

## 2. Polymorphism: one function, many types

`🟢 easy`

An interface lets one function accept any type that implements it, so the same code drives many concrete types. This is the core payoff of interfaces: write to the behavior, not the type.

**Steps:**

1. Define `Animal` as an interface with one method `Speak() string`. Any type with that method satisfies it automatically (no `implements` keyword).
2. Give `Dog` and `Cat` each a `Speak()` value-receiver method. Now both are Animals, even though they share no struct fields.
3. Write `announce(a Animal)` that calls `a.Speak()`. It mentions neither Dog nor Cat — it only knows the interface, so it works for any current or future Animal.
4. Call `announce` with a `Dog` and a `Cat`: one function, two types. Then build a `[]Animal` holding a mix of both and loop over it, announcing each.

```go
package main

import "fmt"

// Animal is the contract: anything with a Speak() string is an Animal.
// The function below depends on this interface, not on concrete types.
type Animal interface {
	Speak() string
}

type Dog struct{ Name string }

func (d Dog) Speak() string { return d.Name + " says Woof" }

type Cat struct{ Name string }

func (c Cat) Speak() string { return c.Name + " says Meow" }

// announce accepts ANY Animal. One function, many types — that's polymorphism.
// It never mentions Dog or Cat; it only knows the interface.
func announce(a Animal) {
	fmt.Println(a.Speak())
}

func main() {
	// Same function, different concrete types passed in.
	announce(Dog{Name: "Rex"})
	announce(Cat{Name: "Whiskers"})

	// A slice of the interface type holds mixed concrete types together.
	zoo := []Animal{
		Dog{Name: "Buddy"},
		Cat{Name: "Felix"},
		Dog{Name: "Max"},
	}

	fmt.Println("--- zoo ---")
	for _, a := range zoo {
		announce(a)
	}
}
```

**Output:**

```
Rex says Woof
Whiskers says Meow
--- zoo ---
Buddy says Woof
Felix says Meow
Max says Woof
```

---

## 3. Interface value is a (type, value) pair

`🟢 easy`

An interface value isn't just the data — it stores BOTH the dynamic concrete type and the value. Reassigning the same interface variable swaps the whole pair, which is why %T can change at runtime.

**Steps:**

1. One variable `var v any` (the empty interface) is reassigned four times, each time to a different concrete type: int, string, a struct, and a named float.
2. `%T` prints the DYNAMIC type currently stored, `%v` prints the value. Watch %T change line by line — that proves the type travels with the value inside the interface.
3. Note struct values show as `main.Point` and `{3 4}`, and the named type `Celsius` keeps its own type identity even though it's a float64 underneath.
4. Setting `v = nil` empties BOTH halves of the pair: the type becomes `<nil>` and the value `<nil>`.

```go
package main

import "fmt"

// Point and Celsius are two unrelated concrete types.
type Point struct{ X, Y int }
type Celsius float64

func main() {
	// any (alias for interface{}) holds ANY concrete type.
	// An interface value is a PAIR: (dynamic type, dynamic value).
	var v any

	// Reassigning v swaps BOTH halves of the pair, not just the value.
	// %T prints the dynamic type; %v prints the dynamic value.
	v = 42
	fmt.Printf("type=%T value=%v\n", v, v)

	v = "hello"
	fmt.Printf("type=%T value=%v\n", v, v)

	v = Point{X: 3, Y: 4}
	fmt.Printf("type=%T value=%v\n", v, v)

	v = Celsius(36.6)
	fmt.Printf("type=%T value=%v\n", v, v)

	// A nil interface has NO type and NO value: the pair is (nil, nil).
	v = nil
	fmt.Printf("type=%T value=%v\n", v, v)
}
```

**Output:**

```
type=int value=42
type=string value=hello
type=main.Point value={3 4}
type=main.Celsius value=36.6
type=<nil> value=<nil>
```

---

## 4. Stringer: fmt uses String() automatically

`🟢 easy`

A type with a String() string method satisfies fmt.Stringer, so fmt reaches for that method automatically in Println, %v, and %s. This is the idiomatic way to give your types a readable display form.

**Steps:**

1. Point has a String() string method, so it satisfies the fmt.Stringer interface — fmt detects this and calls String() for you.
2. Watch Println, %v, and %s all print the same custom form (3, 4): each verb that wants a string asks Stringer first.
3. Plain has no String() method, so %v falls back to Go's default struct layout {3 4} — that contrast is the whole point.
4. Run with: go run .

```go
package main

import "fmt"

// Point implements fmt.Stringer: any type with a String() string method
// gets formatted by that method whenever fmt needs its string form.
type Point struct{ X, Y int }

// String controls how Point appears in Println, %v, and %s.
func (p Point) String() string {
	return fmt.Sprintf("(%d, %d)", p.X, p.Y)
}

// Plain has NO String method, so fmt falls back to default struct formatting.
type Plain struct{ X, Y int }

func main() {
	p := Point{X: 3, Y: 4}
	plain := Plain{X: 3, Y: 4}

	// All three reach for String() because Point satisfies fmt.Stringer.
	fmt.Println("Println:", p)
	fmt.Printf("%%v:      %v\n", p)
	fmt.Printf("%%s:      %s\n", p)

	// No String() here, so %v shows Go's default {field field} layout.
	fmt.Printf("Plain %%v: %v\n", plain)
}
```

**Output:**

```
Println: (3, 4)
%v:      (3, 4)
%s:      (3, 4)
Plain %v: {3 4}
```

---

## 5. The empty interface: any

`🟢 easy`

any (an alias for interface{}) is an interface with zero methods, so every Go type satisfies it — making it a universal container that accepts any value, at the cost of compile-time type safety.

**Steps:**

1. An interface lists required methods; any (= interface{}) lists NONE. Implementing zero methods is trivial, so every type — int, string, slice, struct — automatically satisfies any.
2. printAny(v any) can therefore be called with literally any value. Inside, %v prints the value and %T prints the dynamic (concrete) type stored in the interface at runtime.
3. In main we pass an int, string, bool, a []int slice, and a point struct — five unrelated types, one function, no errors. Note %T reports main.point because the struct lives in package main.
4. Use any only at real boundaries (fmt.Println, JSON decoding, generic-ish containers). Everywhere else prefer a concrete type or a real interface — any throws away the type checking that makes Go safe.

```go
package main

import "fmt"

// any is an alias for interface{}: an interface with ZERO methods.
// Because every type implements zero methods trivially, EVERY value
// satisfies any. That makes it a universal box — but you lose all
// static type info, so reach for it only at true boundaries
// (fmt.Println, JSON, generic containers), not as a lazy escape hatch.

type point struct {
	X, Y int
}

// printAny accepts anything. %v shows the value, %T the dynamic type
// stored inside the interface at runtime.
func printAny(v any) {
	fmt.Printf("value=%v\ttype=%T\n", v, v)
}

func main() {
	printAny(42)
	printAny("hello")
	printAny(true)
	printAny([]int{1, 2, 3})
	printAny(point{X: 3, Y: 4})
}
```

**Output:**

```
value=42	type=int
value=hello	type=string
value=true	type=bool
value=[1 2 3]	type=[]int
value={3 4}	type=main.point
```

---

## 6. Safe type assertion (comma-ok)

`🟢 easy`

A type assertion on an interface value can panic if the dynamic type doesn't match, but the two-result "comma-ok" form (s, ok := v.(string)) lets you test the type safely and branch on the result instead of crashing.

**Steps:**

1. Note `values` is a `[]any`, so each element's static type is `any` and its concrete (dynamic) type is only known at runtime.
2. In `describe`, `s, ok := v.(string)` asks 'is v really a string?' — `ok` is true with `s` holding the string, or false with `s` being the zero value "".
3. The `if/else` handles both branches: matching values print their length, non-strings report ok=false without ever panicking.
4. Read the comment block: the single-result form `v.(int)` would panic on a non-int, which is exactly what comma-ok protects you from.

```go
package main

import "fmt"

// describe inspects an any value WITHOUT risking a panic.
// The comma-ok form returns (zeroValue, false) instead of crashing
// when the dynamic type doesn't match the asserted type.
func describe(v any) {
	// Safe: if v isn't a string, ok is false and s is "".
	if s, ok := v.(string); ok {
		fmt.Printf("%-8v -> it's a string of length %d\n", v, len(s))
	} else {
		fmt.Printf("%-8v -> not a string (ok=false, s=%q)\n", v, s)
	}

	// The single-return form below would PANIC if v's dynamic type
	// isn't int, e.g. v.(int) on a string aborts the program:
	//   panic: interface conversion: interface {} is string, not int
	// That's why comma-ok is the safe default for unknown types.
}

func main() {
	values := []any{"hello", 42, "go", true}
	fmt.Println("Inspecting each value:")
	for _, v := range values {
		describe(v)
	}
}
```

**Output:**

```
Inspecting each value:
hello    -> it's a string of length 5
42       -> not a string (ok=false, s="")
go       -> it's a string of length 2
true     -> not a string (ok=false, s="")
```

---

## 7. Type switch basics

`🟢 easy`

A type switch (switch x := v.(type)) branches on the concrete type inside an interface value, and inside each case x already has that case's type, so you can use it directly without a separate assertion.

**Steps:**

1. describe(v any) opens with switch x := v.(type); the binding x lets each case use the value with its case-specific type.
2. In the int case x is an int (so x*2 works); in the string case it's a string (len counts bytes); in the bool case it's a bool.
3. The default case keeps the original interface type, and %T prints the real dynamic type, %v its value.
4. main builds a mixed []any and prints describe for each element, so you see every arm fire.

```go
package main

import "fmt"

// describe inspects the dynamic type held in an interface value.
// Inside each case, x is automatically given that case's concrete type,
// so you can use it directly without a separate assertion.
func describe(v any) string {
	switch x := v.(type) {
	case int:
		// x is an int here: arithmetic just works.
		return fmt.Sprintf("int: %d (doubled %d)", x, x*2)
	case string:
		// x is a string here: len counts bytes.
		return fmt.Sprintf("string: %q (len %d)", x, len(x))
	case bool:
		// x is a bool here.
		return fmt.Sprintf("bool: %t", x)
	default:
		// x keeps the original interface type; %T reveals the real type.
		return fmt.Sprintf("other: %T = %v", x, x)
	}
}

func main() {
	mixed := []any{42, "hi", true, 3.14, []int{1, 2}}
	for _, v := range mixed {
		fmt.Println(describe(v))
	}
}
```

**Output:**

```
int: 42 (doubled 84)
string: "hi" (len 2)
bool: true
other: float64 = 3.14
other: []int = [1 2]
```

---

## 8. Slice of interfaces & a total

`🟢 easy`

A []Shape can hold different concrete types at once, and code that loops over it (summing Area) works on any of them — that's polymorphism over a heterogeneous slice.

**Steps:**

1. Define `Shape` with one method, `Area() float64`. Both `Circle` and `Rectangle` get an `Area` method, so they satisfy `Shape` implicitly — no declaration needed.
2. Build `shapes := []Shape{...}` mixing `Circle` and `Rectangle` values. The slice's element type is the interface, so dissimilar concrete types coexist.
3. `TotalArea` ranges over `[]Shape` and calls `s.Area()` without ever naming a concrete type — each value dispatches to its own method at runtime.
4. `main` prints each shape's area, then the total; `math.Pi` gives the circle areas (radius 1 -> 3.1416).

```go
package main

import (
	"fmt"
	"math"
)

// Shape is the common behavior. Any type with an Area() float64 method
// satisfies it implicitly — no "implements" keyword needed.
type Shape interface {
	Area() float64
}

type Circle struct {
	Radius float64
}

// Area on Circle makes *Circle and Circle both usable as a Shape value.
func (c Circle) Area() float64 {
	return math.Pi * c.Radius * c.Radius
}

type Rectangle struct {
	Width, Height float64
}

func (r Rectangle) Area() float64 {
	return r.Width * r.Height
}

// TotalArea works on ANY Shape — it never names a concrete type.
// That is polymorphism: one loop, many real types behind the interface.
func TotalArea(shapes []Shape) float64 {
	var sum float64
	for _, s := range shapes {
		sum += s.Area()
	}
	return sum
}

func main() {
	// A heterogeneous slice: Circles and Rectangles stored side by side
	// because each value carries a Shape's Area method.
	shapes := []Shape{
		Circle{Radius: 1},
		Rectangle{Width: 2, Height: 3},
		Circle{Radius: 2},
	}

	for i, s := range shapes {
		fmt.Printf("shape %d area = %.4f\n", i, s.Area())
	}
	fmt.Printf("TOTAL area = %.4f\n", TotalArea(shapes))
}
```

**Output:**

```
shape 0 area = 3.1416
shape 1 area = 6.0000
shape 2 area = 12.5664
TOTAL area = 21.7080
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
