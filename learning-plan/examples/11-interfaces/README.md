# Step 11 — Interfaces · Examples

A graded library of **25 runnable examples**, easy → hard. Each one is a complete
`package main` program: read the concept and steps, then **retype the code block**
into a scratch folder and run it. Don't copy-paste — typing it is the practice.

**How to run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # any throwaway folder
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added —
the **Output** block is the real captured stdout.

> Want more? Say *“add more interface examples”* and I'll append new ones starting at #26.

## Index

**Easy**

- [1. Minimal interface & implicit satisfaction](#1-minimal-interface--implicit-satisfaction)
- [2. Polymorphism: one function, many types](#2-polymorphism-one-function-many-types)
- [3. Interface value is a (type, value) pair](#3-interface-value-is-a-type-value-pair)
- [4. Stringer: fmt uses String() automatically](#4-stringer-fmt-uses-string-automatically)
- [5. The empty interface: any](#5-the-empty-interface-any)
- [6. Safe type assertion (comma-ok)](#6-safe-type-assertion-comma-ok)
- [7. Type switch basics](#7-type-switch-basics)
- [8. Slice of interfaces & a total](#8-slice-of-interfaces--a-total)

**Medium**

- [9. Method sets: pointer vs value receiver](#9-method-sets-pointer-vs-value-receiver)
- [10. Interface composition by embedding](#10-interface-composition-by-embedding)
- [11. The error interface](#11-the-error-interface)
- [12. sort.Slice with a closure](#12-sortslice-with-a-closure)
- [13. sort.Interface: Len/Less/Swap](#13-sortinterface-lenlessswap)
- [14. io.Writer: write the algorithm once](#14-iowriter-write-the-algorithm-once)
- [15. io.MultiWriter: fan-out](#15-iomultiwriter-fan-out)
- [16. Interface-to-interface assertion](#16-interface-to-interface-assertion)
- [17. Strategy via a map of interfaces](#17-strategy-via-a-map-of-interfaces)
- [18. Accept interfaces, return structs (mini DI)](#18-accept-interfaces-return-structs-mini-di)

**Hard**

- [19. The typed-nil interface trap](#19-the-typed-nil-interface-trap)
- [20. Interface equality & the uncomparable panic](#20-interface-equality--the-uncomparable-panic)
- [21. Optional interfaces (feature detection / upgrades)](#21-optional-interfaces-feature-detection--upgrades)
- [22. Decorator / middleware chain](#22-decorator--middleware-chain)
- [23. Recursive type switch over any (JSON-like walker)](#23-recursive-type-switch-over-any-json-like-walker)
- [24. Dependency injection with a test fake](#24-dependency-injection-with-a-test-fake)
- [25. Capstone: a tiny plugin system](#25-capstone-a-tiny-plugin-system)

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

## 9. Method sets: pointer vs value receiver

`🟡 medium`

A type's method set determines which interfaces it satisfies: methods with pointer receivers belong only to *T, so if an interface needs even one pointer-receiver method, only *T (not T) implements it.

**Steps:**

1. Mutator needs two methods: SetName(string) and Name() string. On User, SetName uses a *pointer* receiver (it must mutate), while Name uses a *value* receiver.
2. Method-set rule: value-receiver methods are in BOTH User and *User; pointer-receiver methods are in *User ONLY. So Mutator (which needs SetName) is satisfied by *User but not User.
3. That's why `var m Mutator = &User{...}` compiles: *User has SetName + Name. Assigning a plain `User{...}` would fail (see the commented line and its exact compiler error).
4. The %T print confirms the dynamic type stored in the interface is *main.User, the pointer.
5. Takeaway: when a method needs to mutate (or any method uses a pointer receiver), reach for &T to satisfy the interface.

```go
package main

import "fmt"

// Mutator: SetName mutates, Name reads.
type Mutator interface {
	SetName(string)
	Name() string
}

// User satisfies Mutator, but note the receiver kinds below.
type User struct {
	name string
}

// SetName has a POINTER receiver: it must mutate the underlying value,
// so it lives in the method set of *User only.
func (u *User) SetName(n string) { u.name = n }

// Name has a VALUE receiver: it lives in the method set of BOTH User and *User.
func (u User) Name() string { return u.name }

func main() {
	// OK: *User's method set includes SetName (pointer) AND Name (value),
	// so *User satisfies Mutator.
	var m Mutator = &User{name: "Ada"}
	fmt.Println("before:", m.Name())
	m.SetName("Grace")
	fmt.Println("after: ", m.Name())

	// FAILS to compile — uncommenting the next line yields:
	//   cannot use User{...} (value of struct type User) as Mutator value
	//   in variable declaration: User does not implement Mutator
	//   (method SetName has pointer receiver)
	// var bad Mutator = User{name: "Nope"}

	// Rule: if ANY method the interface needs has a pointer receiver,
	// only *T is in the method set, so you must use &T to satisfy it.
	fmt.Printf("dynamic type stored in m: %T\n", m)
}
```

**Output:**

```
before: Ada
after:  Grace
dynamic type stored in m: *main.User
```

---

## 10. Interface composition by embedding

`🟡 medium`

Larger interfaces are built by embedding smaller ones, and a value of the composed interface can be assigned to any of its embedded (subset) interfaces — exactly how io.ReadWriter relates to io.Reader and io.Writer.

**Steps:**

1. Read the three interface declarations top-down: Reader and Writer each have one method, then ReadWriter embeds both — its method set is their union, so a type needs Read AND Write to satisfy it.
2. Note that buffer (a *buffer, since the methods have pointer receivers) implements all three interfaces automatically, with no 'implements' keyword — satisfaction is structural in Go.
3. In main, a *buffer is held as a ReadWriter; we Write twice then Read to see the appended data.
4. The line `var r Reader = rw` is the key move: a ReadWriter IS-A Reader (superset to subset), so assignment is implicit and legal. Try reversing it (Reader -> ReadWriter) and it will NOT compile.

```go
package main

import "fmt"

// Two small, single-method interfaces. Keeping interfaces tiny is idiomatic Go:
// it lets callers ask for exactly the behavior they need.
type Reader interface {
	Read() string
}

type Writer interface {
	Write(s string)
}

// ReadWriter is built by EMBEDDING the two interfaces above. Its method set is
// the union of theirs: any type with Read and Write satisfies ReadWriter.
// (This mirrors io.ReadWriter = io.Reader + io.Writer.)
type ReadWriter interface {
	Reader
	Writer
}

// buffer is one concrete type. Because it has both methods, it satisfies
// Reader, Writer, and ReadWriter at once — no "implements" keyword needed.
type buffer struct {
	data string
}

func (b *buffer) Read() string { return b.data }

func (b *buffer) Write(s string) { b.data += s }

func main() {
	// A *buffer fits the composed interface.
	var rw ReadWriter = &buffer{}
	rw.Write("hello")
	rw.Write(" world")
	fmt.Println("via ReadWriter:", rw.Read())

	// Superset -> subset assignment: a ReadWriter IS-A Reader, so this is
	// allowed implicitly. The reverse (Reader -> ReadWriter) would NOT compile.
	var r Reader = rw
	fmt.Println("via Reader:    ", r.Read())
}
```

**Output:**

```
via ReadWriter: hello world
via Reader:     hello world
```

---

## 11. The error interface

`🟡 medium`

In Go, `error` is just an interface with one method, `Error() string`, so any type can become an error; this teaches the idiomatic (T, error) return pattern plus how to identify specific errors with sentinels (errors.Is) and custom types (errors.As).

**Steps:**

1. `error` is the built-in interface `interface{ Error() string }`. Give `*ValidationError` an `Error()` method and it satisfies `error` automatically — no declaration needed.
2. A sentinel like `ErrEmptyName = errors.New(...)` is one shared value you can identify later; `validate` returns it directly for the empty-name case.
3. Functions return `(T, error)`. The caller checks `if err != nil` first; a nil error means the result is valid. Printing `err` with `%v` calls its `Error()` method.
4. `errors.Is(err, ErrEmptyName)` tests against a sentinel value; `errors.As(err, &ve)` tests whether err is a particular concrete type and, if so, hands you the typed value (here `ve.Field`).

```go
package main

import (
	"errors"
	"fmt"
)

// ValidationError is a custom error type. To satisfy the built-in `error`
// interface, a type only needs ONE method: Error() string. That's it —
// `error` is just `interface{ Error() string }`.
type ValidationError struct {
	Field string
	Min   int
}

// Error() makes *ValidationError usable anywhere an `error` is expected.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %q must be at least %d", e.Field, e.Min)
}

// ErrEmptyName is a SENTINEL error: a single shared value created with
// errors.New. Callers compare against it to detect a specific condition.
var ErrEmptyName = errors.New("name is empty")

// validate returns (result, error) — the idiomatic Go pattern. A nil error
// means success; a non-nil error means the result should be ignored.
func validate(name string, age int) (string, error) {
	if name == "" {
		return "", ErrEmptyName // return the sentinel directly
	}
	if age < 18 {
		// Return a *ValidationError; it counts as an `error` automatically.
		return "", &ValidationError{Field: "age", Min: 18}
	}
	return name, nil
}

func main() {
	inputs := []struct {
		name string
		age  int
	}{
		{"Ada", 30},
		{"", 25},
		{"Bob", 12},
	}

	for _, in := range inputs {
		got, err := validate(in.name, in.age)
		if err != nil {
			// Printing an error just calls its Error() method.
			fmt.Printf("rejected: %v\n", err)

			// errors.Is compares against a sentinel (works through wrapping).
			if errors.Is(err, ErrEmptyName) {
				fmt.Println("  -> matched sentinel ErrEmptyName")
			}

			// errors.As checks if err is (or wraps) a specific concrete type.
			var ve *ValidationError
			if errors.As(err, &ve) {
				fmt.Printf("  -> it's a ValidationError on field %q\n", ve.Field)
			}
			continue
		}
		fmt.Printf("accepted: %s\n", got)
	}
}
```

**Output:**

```
accepted: Ada
rejected: name is empty
  -> matched sentinel ErrEmptyName
rejected: validation failed: "age" must be at least 18
  -> it's a ValidationError on field "age"
```

---

## 12. sort.Slice with a closure

`🟡 medium`

sort.Slice sorts a slice in place using a less-than closure, so the element type needs no interface implementation — the comparison logic lives right at the call site. It is the quick modern way to do multi-key sorting (by Age, then Name).

**Steps:**

1. Define Person{Name, Age} and build an unsorted slice; print it as the Before state.
2. Call sort.Slice(people, less): the closure takes indices i, j and returns true when people[i] should come before people[j].
3. Inside the closure, compare Age first; only when Age is equal fall back to comparing Name — that secondary key makes ties deterministic.
4. sort.Slice mutates the slice in place (no return value), so just print people again for the After state.
5. Note: the next example shows the explicit sort.Interface (Len/Less/Swap) version of this same sort.

```go
package main

import (
	"fmt"
	"sort"
)

// Person is the value we want to order. sort.Slice does not need the
// slice's element type to implement any interface — the comparison
// lives entirely in the closure we pass in.
type Person struct {
	Name string
	Age  int
}

func main() {
	people := []Person{
		{"Bob", 30},
		{"Alice", 30},
		{"Carol", 25},
		{"Dave", 25},
	}

	fmt.Println("Before:")
	for _, p := range people {
		fmt.Printf("  %-6s %d\n", p.Name, p.Age)
	}

	// sort.Slice sorts in place. The closure returns true when element i
	// must come before element j. We sort by Age first; on a tie we fall
	// back to Name so the result is fully deterministic.
	sort.Slice(people, func(i, j int) bool {
		if people[i].Age != people[j].Age {
			return people[i].Age < people[j].Age
		}
		return people[i].Name < people[j].Name
	})

	fmt.Println("After (Age, then Name):")
	for _, p := range people {
		fmt.Printf("  %-6s %d\n", p.Name, p.Age)
	}
}
```

**Output:**

```
Before:
  Bob    30
  Alice  30
  Carol  25
  Dave   25
After (Age, then Name):
  Carol  25
  Dave   25
  Alice  30
  Bob    30
```

---

## 13. sort.Interface: Len/Less/Swap

`🟡 medium`

Implementing sort.Interface (Len/Less/Swap) on a named slice type lets sort.Sort order any custom collection, and sort.Reverse wraps that same interface to flip the order without new code.

**Steps:**

1. Define a named slice type `type ByAge []Person` so you can attach methods to the slice itself, not to one element.
2. Implement the three methods sort.Interface requires: Len (count), Less(i,j) (the sort key — true means i comes first), and Swap (exchange in place).
3. Call `sort.Sort(ByAge(people))`: the conversion lets sort discover the methods and sorts ascending by Age.
4. Wrap with `sort.Reverse(ByAge(people))`: Reverse is itself a sort.Interface that flips Less, reusing your same three methods to sort descending.
5. Compare to example 12's sort.Slice: a closure is quicker for one-off sorts, while a named type with methods is reusable and works with Reverse/Stable.

```go
package main

import (
	"fmt"
	"sort"
)

// Person is the element we want to sort by Age.
type Person struct {
	Name string
	Age  int
}

// ByAge is a NAMED slice type. We attach sort.Interface to the slice itself,
// not to one element. That is the key contrast with sort.Slice (example 12):
// sort.Slice takes a closure, so no named type or methods are needed — handy
// for one-off sorts. sort.Interface (here) needs three methods, but the named
// type is reusable, self-documenting, and works with sort.Reverse/Stable too.
type ByAge []Person

// Len reports how many elements there are — sort needs the bounds.
func (a ByAge) Len() int { return len(a) }

// Less reports the ordering: is element i "before" element j?
// Return true for ascending age. This single method defines the sort key.
func (a ByAge) Less(i, j int) bool { return a[i].Age < a[j].Age }

// Swap exchanges two elements in place. sort rearranges via Swap only,
// so a value receiver is fine: the slice header is copied, but it still
// points at the same backing array we mutate.
func (a ByAge) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func main() {
	people := []Person{
		{"Carol", 31},
		{"Alice", 25},
		{"Bob", 42},
		{"Dave", 25},
	}

	// Convert to the named type so sort.Sort can find Len/Less/Swap.
	sort.Sort(ByAge(people))
	fmt.Println("Ascending by age:")
	for _, p := range people {
		fmt.Printf("  %-6s %d\n", p.Name, p.Age)
	}

	// sort.Reverse WRAPS any sort.Interface and flips Less, so we reuse
	// the exact same three methods to get a descending sort.
	sort.Sort(sort.Reverse(ByAge(people)))
	fmt.Println("Descending by age:")
	for _, p := range people {
		fmt.Printf("  %-6s %d\n", p.Name, p.Age)
	}
}
```

**Output:**

```
Ascending by age:
  Alice  25
  Dave   25
  Carol  31
  Bob    42
Descending by age:
  Bob    42
  Carol  31
  Alice  25
  Dave   25
```

---

## 14. io.Writer: write the algorithm once

`🟡 medium`

io.Writer is a one-method interface, so code that formats output (fmt.Fprintf, io.Copy) targets the interface instead of a concrete destination — you write the algorithm once and swap the sink (terminal, in-memory buffer, or your own custom writer) freely.

**Steps:**

1. countingWriter satisfies io.Writer by implementing the single method Write(p []byte) (int, error); it tallies bytes and newlines and reports it consumed all of p by returning len(p).
2. The greet helper takes an io.Writer and calls fmt.Fprintf exactly once; the SAME call drives os.Stdout, a *bytes.Buffer, and the custom counter just by passing a different argument.
3. io.Copy streams from any io.Reader (here a strings.Reader) into any io.Writer (the buffer) — another standard-library function written against the interfaces, not concrete types.
4. main() prints the buffer's captured text plus the counter's byte/line totals and io.Copy's result, so each sink's effect is visible side by side.

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// countingWriter is a custom io.Writer. The interface asks for ONE method:
//
//	Write(p []byte) (n int, err error)
//
// By implementing it we let any code that writes to an io.Writer write to us
// instead — fmt.Fprintf, io.Copy, log, etc. all target the interface, not a
// concrete type. Write the algorithm once; swap the destination freely.
type countingWriter struct {
	bytes int
	lines int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.bytes += len(p)
	c.lines += bytes.Count(p, []byte{'\n'})
	// We must return how many bytes we "consumed". We accepted them all.
	return len(p), nil
}

func main() {
	// Same Fprintf call, three different sinks — chosen by which io.Writer
	// we pass. The formatting logic does not change.
	greet := func(w io.Writer, name string) {
		fmt.Fprintf(w, "Hello, %s!\n", name)
	}

	fmt.Println("-- to os.Stdout --")
	greet(os.Stdout, "Stdout") // straight to the terminal

	var buf bytes.Buffer
	greet(&buf, "Buffer") // captured in memory instead
	fmt.Print("-- from *bytes.Buffer --\n", buf.String())

	counter := &countingWriter{}
	greet(counter, "Counter") // measured, not stored

	// io.Copy streams from any io.Reader to any io.Writer. Here the source is
	// a strings.Reader; the sink is our buffer. No call site change needed.
	src := strings.NewReader("piped through io.Copy\n")
	n, err := io.Copy(&buf, src)
	if err != nil {
		panic(err)
	}

	fmt.Println("-- counts from custom writer --")
	fmt.Printf("bytes=%d lines=%d\n", counter.bytes, counter.lines)
	fmt.Printf("io.Copy moved %d bytes; buffer now holds %d bytes\n", n, buf.Len())
}
```

**Output:**

```
-- to os.Stdout --
Hello, Stdout!
-- from *bytes.Buffer --
Hello, Buffer!
-- counts from custom writer --
bytes=16 lines=1
io.Copy moved 22 bytes; buffer now holds 37 bytes
```

---

## 15. io.MultiWriter: fan-out

`🟡 medium`

io.MultiWriter wraps several io.Writers into one, fanning every Write out to all of them; because each sink is just an interface value, unrelated concrete types (buffers, the terminal) compose without any special-casing.

**Steps:**

1. Two `*bytes.Buffer`s plus `os.Stdout` all satisfy `io.Writer`, so they're interchangeable as sinks.
2. `io.MultiWriter(&audit, &mirror, os.Stdout)` returns a single `io.Writer` that copies each write to all three (like Unix `tee`).
3. `fmt.Fprintf`/`fmt.Fprintln` and `io.Copy` all target any `io.Writer`, so writing once reaches every destination.
4. Print each buffer afterward; the equality check shows both captured byte-for-byte the same stream, proving the fan-out.

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	// Two in-memory sinks. *bytes.Buffer satisfies io.Writer.
	var audit, mirror bytes.Buffer

	// io.MultiWriter returns ONE io.Writer that fans every Write out to
	// all the writers it wraps — like the Unix "tee". Because each sink
	// is just an interface value, totally different concrete types
	// (buffers AND the terminal) compose with no special casing.
	fanout := io.MultiWriter(&audit, &mirror, os.Stdout)

	// One call, three destinations. fmt.Fprintf writes to any io.Writer.
	fmt.Fprintf(fanout, "live: %s\n", "deploy ok")
	fmt.Fprintln(fanout, "live: cache warmed")

	// io.Copy streams from any io.Reader to our fan-out writer the same way.
	src := strings.NewReader("live: stream chunk\n")
	if _, err := io.Copy(fanout, src); err != nil {
		fmt.Fprintln(os.Stderr, "copy:", err)
		return
	}

	// Prove both buffers captured an identical copy of everything above.
	fmt.Println("--- audit buffer ---")
	fmt.Print(audit.String())
	fmt.Println("--- mirror buffer ---")
	fmt.Print(mirror.String())
	fmt.Printf("--- buffers identical: %v ---\n", audit.String() == mirror.String())
}
```

**Output:**

```
live: deploy ok
live: cache warmed
live: stream chunk
--- audit buffer ---
live: deploy ok
live: cache warmed
live: stream chunk
--- mirror buffer ---
live: deploy ok
live: cache warmed
live: stream chunk
--- buffers identical: true ---
```

---

## 16. Interface-to-interface assertion

`🟡 medium`

A value held in one interface can be type-asserted to a DIFFERENT, richer interface to detect optional capabilities at runtime. This is how the standard library checks for things like fmt.Stringer without forcing every type to implement them.

**Steps:**

1. Define a base interface Animal{ Legs() } and a richer interface Named{ Name() }. Dog implements both; Snake implements only Animal.
2. describe() receives an Animal, so the compiler only guarantees Legs(). Name() is NOT statically available here.
3. The key line n, ok := a.(Named) asserts the BASE interface value against ANOTHER interface. Go checks the concrete type's method set at runtime.
4. When ok is true the value also satisfies Named, so n.Name() is safe to call; when false we fall back gracefully (no panic, because we used the comma-ok form).
5. Run with go run . — Dog reports its name, Snake is anonymous, proving capability detection across interfaces.

```go
package main

import "fmt"

// Animal is the BASE interface every animal satisfies.
type Animal interface {
	Legs() int
}

// Named is a RICHER capability: not every Animal can report a name.
// We'll detect it at runtime via an interface-to-interface assertion.
type Named interface {
	Name() string
}

// Dog satisfies BOTH Animal and Named.
type Dog struct{ name string }

func (d Dog) Legs() int    { return 4 }
func (d Dog) Name() string { return d.name }

// Snake satisfies ONLY Animal (no Name method).
type Snake struct{}

func (s Snake) Legs() int { return 0 }

// describe takes the BASE interface, then probes for the richer one.
func describe(a Animal) {
	// a.(Named) asks: does the concrete value behind this Animal
	// ALSO implement Named? ok tells us without panicking.
	if n, ok := a.(Named); ok {
		fmt.Printf("%d legs, named %q\n", a.Legs(), n.Name())
	} else {
		fmt.Printf("%d legs, anonymous\n", a.Legs())
	}
}

func main() {
	animals := []Animal{Dog{name: "Rex"}, Snake{}}
	for _, a := range animals {
		describe(a)
	}
}
```

**Output:**

```
4 legs, named "Rex"
0 legs, anonymous
```

---

## 17. Strategy via a map of interfaces

`🟡 medium`

A map[string]Op turns an interface into a runtime-selectable strategy registry: you dispatch behavior by string key, and adding a new operation never touches the dispatch code.

**Steps:**

1. Define `Op` with one method `Apply(a, b int) int`; each concrete type (`Add`, `Mul`, `Sub`) is a stateless `struct{}` whose method holds the behavior, satisfying `Op` implicitly.
2. Build `registry := map[string]Op{...}` — the interface value lets different concrete types live under one map type, keyed by a runtime string.
3. Dispatch a single op with `registry["mul"]` using the comma-ok form, then call `op.Apply(a, b)`; the caller never names the concrete type.
4. To list everything, collect keys into a slice and `sort.Strings` them first, because map iteration order is random — sorting makes the output deterministic.
5. Notice the payoff: a fourth op would be one new type plus one map entry; the dispatch loop and lookup code stay untouched (open for extension, closed for modification).

```go
package main

import (
	"fmt"
	"sort"
)

// Op is the strategy interface: any binary integer operation.
// Concrete types satisfy it implicitly — no "implements" keyword.
type Op interface {
	Apply(a, b int) int
}

// Each strategy is its own type. They carry no state here, so an
// empty struct is enough; the behavior lives entirely in the method.
type Add struct{}

func (Add) Apply(a, b int) int { return a + b }

type Mul struct{}

func (Mul) Apply(a, b int) int { return a * b }

type Sub struct{}

func (Sub) Apply(a, b int) int { return a - b }

func main() {
	// The registry maps a runtime string key to a strategy value.
	// Adding a new op = one map entry; the dispatcher below never changes.
	registry := map[string]Op{
		"add": Add{},
		"mul": Mul{},
		"sub": Sub{},
	}

	// Dispatch by key chosen at runtime — the caller need not know the type.
	a, b := 6, 4
	fmt.Println("Dispatch one op by key:")
	if op, ok := registry["mul"]; ok {
		fmt.Printf("  mul(%d, %d) = %d\n", a, b, op.Apply(a, b))
	}

	// Iterate the whole registry. Map order is random, so sort the keys
	// for deterministic output.
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("All registered ops:")
	for _, k := range keys {
		fmt.Printf("  %s(%d, %d) = %d\n", k, a, b, registry[k].Apply(a, b))
	}
}
```

**Output:**

```
Dispatch one op by key:
  mul(6, 4) = 24
All registered ops:
  add(6, 4) = 10
  mul(6, 4) = 24
  sub(6, 4) = 2
```

---

## 18. Accept interfaces, return structs (mini DI)

`🟡 medium`

The Go idiom "accept interfaces, return structs": a constructor takes a small interface (so any backing implementation can be injected) but returns the concrete type, giving callers full access while keeping the dependency swappable.

**Steps:**

1. Read the seam first: Store is a tiny interface with just Get(id) (string, bool) — it describes WHAT the Service needs, not how it's stored.
2. MapStore is one concrete implementation; its pointer receiver Get satisfies Store, so *MapStore counts as a Store anywhere.
3. NewService(s Store) *Service is the whole point: it ACCEPTS the interface (caller picks the implementation) and RETURNS the concrete *Service (caller keeps every method, no type assertions).
4. main wires the real store in with NewService(NewMapStore()) — that single line IS the dependency injection — then Greet runs without ever naming MapStore.
5. Because Service only knows the Store interface, a test could pass a fake store with canned data instead; the registry pattern in example 24 takes this swapping further.

```go
package main

import "fmt"

// Store is what the Service DEPENDS ON. We accept this small interface
// so any backing storage (real, fake, cached) can be plugged in.
type Store interface {
	Get(id int) (string, bool)
}

// MapStore is a concrete implementation backed by a map.
type MapStore struct {
	data map[int]string
}

func NewMapStore() *MapStore {
	return &MapStore{data: map[int]string{
		1: "Ada",
		2: "Linus",
	}}
}

func (m *MapStore) Get(id int) (string, bool) {
	name, ok := m.data[id]
	return name, ok
}

// Service depends on the Store interface but is itself a concrete struct.
// "Accept interfaces, return structs": the constructor takes a Store
// (so callers can swap implementations) yet returns *Service, so callers
// get the full, concrete type with all its methods and no guessing.
type Service struct {
	store Store
}

// NewService accepts the interface, returns the concrete *Service.
func NewService(s Store) *Service {
	return &Service{store: s}
}

// Greet uses the injected store without knowing its concrete type.
func (svc *Service) Greet(id int) string {
	name, ok := svc.store.Get(id)
	if !ok {
		return fmt.Sprintf("no user with id %d", id)
	}
	return "Hello, " + name + "!"
}

func main() {
	// Wire the real store into the service (dependency injection).
	svc := NewService(NewMapStore())

	for _, id := range []int{1, 2, 99} {
		fmt.Printf("id %d -> %s\n", id, svc.Greet(id))
	}
}
```

**Output:**

```
id 1 -> Hello, Ada!
id 2 -> Hello, Linus!
id 99 -> no user with id 99
```

---

## 19. The typed-nil interface trap

`🔴 hard`

An interface value is nil only when BOTH its type slot and value slot are nil; returning a typed nil pointer (e.g. *MyError) as error makes err != nil unexpectedly true, a common Go bug.

**Steps:**

1. An interface holds two slots: a dynamic TYPE and a VALUE. It equals nil only when both are nil. buggy() declares var e *MyError (value nil, but type *MyError) and returns it, so the interface becomes (type=*MyError, value=nil) — non-nil.
2. Run it: with ok=true the success path SHOULD give a nil error, yet buggy prints err == nil: false because the type slot is occupied. %T exposes the hidden *main.MyError even though the pointer inside is nil.
3. fixed() returns the untyped nil literal on success, leaving both slots empty: (type=nil, value=nil), so err == nil is true and %T shows <nil>.
4. The genuine-error case (ok=false) behaves the same for both functions — both return a real *MyError, so the trap only bites the success path.
5. Fix rule: never assign a typed nil pointer to an error return; return the bare nil literal (or check and convert), so the comparison and errors.Is/As work as expected.

```go
package main

import (
	"fmt"
	"strings"
)

// MyError is a custom error type. error is an interface, and any type with
// an Error() string method satisfies it.
type MyError struct {
	msg string
}

func (e *MyError) Error() string { return e.msg }

// buggy declares a *MyError, leaves it nil, then returns it as error.
// THE TRAP: the returned interface holds (type=*MyError, value=nil). An
// interface is nil ONLY when BOTH type AND value are nil. Here the type
// slot is non-nil, so the interface != nil even though the pointer is.
func buggy(ok bool) error {
	var e *MyError // nil pointer, but typed
	if !ok {
		e = &MyError{msg: "real failure"}
	}
	return e // returns interface (*MyError, nil) when ok — still non-nil!
}

// fixed returns the untyped nil literal on success, so the interface has an
// empty type slot too: (type=nil, value=nil) == true nil.
func fixed(ok bool) error {
	if ok {
		return nil // untyped nil -> truly nil interface
	}
	return &MyError{msg: "real failure"}
}

func report(label string, err error) {
	// %T prints the dynamic type stored in the interface; for a typed nil it
	// still shows *main.MyError, exposing the hidden type slot.
	fmt.Printf("%-14s err == nil: %-5t  dynamic type: %T\n", label, err == nil, err)
}

func main() {
	fmt.Println("Calling with ok=true (success path expected):")
	report("buggy:", buggy(true))
	report("fixed:", fixed(true))

	fmt.Println("\nCalling with ok=false (genuine error):")
	report("buggy:", buggy(false))
	report("fixed:", fixed(false))

	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("Rule: never return a typed nil pointer as error.")
	fmt.Println("Return the untyped nil literal so (type,value) is (nil,nil).")
}
```

**Output:**

```
Calling with ok=true (success path expected):
buggy:         err == nil: false  dynamic type: *main.MyError
fixed:         err == nil: true   dynamic type: <nil>

Calling with ok=false (genuine error):
buggy:         err == nil: false  dynamic type: *main.MyError
fixed:         err == nil: false  dynamic type: *main.MyError

----------------------------------------
Rule: never return a typed nil pointer as error.
Return the untyped nil literal so (type,value) is (nil,nil).
```

---

## 20. Interface equality & the uncomparable panic

`🔴 hard`

Comparing two interface values with == checks both the dynamic type and the dynamic value — but if the dynamic type is uncomparable (slice/map/func), the == panics at runtime, which you can intercept with recover().

**Steps:**

1. An interface value is a (dynamic type, dynamic value) pair. `a == b` on two interfaces is true ONLY when both the types match AND the values are equal — see Point{1,2} match while different values or different types (even int(2)) come back false.
2. Comparability is a property of the DYNAMIC type. Comparable types (ints, strings, structs whose fields are all comparable like Point) compare fine. Types containing a slice, map, or func (like Box{Tags []string}) are uncomparable.
3. Writing `a == b` always compiles when a/b are of interface type — the compiler can't know the runtime type. But if that runtime type turns out to be uncomparable, Go PANICS with 'comparing uncomparable type'.
4. safeEqual wraps the comparison: a deferred recover() catches the panic and turns it into a returned error, so two Boxes report the recovered message instead of crashing, while two Points return the real boolean answer.
5. Run `go run .` and read the labeled lines top to bottom: four legal comparisons, then the recovered panic, then a clean comparison through the same helper.

```go
package main

import "fmt"

// any (interface{}) can hold ANY dynamic type. Comparing two interface
// values with == compares (dynamic type, dynamic value). The catch:
// if the dynamic type is NOT comparable (slices, maps, funcs), the ==
// is legal to WRITE but PANICS at runtime.

// Point is comparable: all its fields are comparable, so two Points
// boxed into interfaces can be compared safely.
type Point struct {
	X, Y int
}

// Box holds a slice. Structs containing a slice/map/func are NOT
// comparable, so == on Boxes-in-interfaces panics at runtime.
type Box struct {
	Tags []string
}

// safeEqual reports whether a == b, but converts the runtime panic
// from comparing uncomparable dynamic types into a returned error.
func safeEqual(a, b any) (equal bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return a == b, nil // may panic if a/b hold an uncomparable type
}

func main() {
	// 1. Same dynamic type AND value -> equal.
	var i1 any = Point{1, 2}
	var i2 any = Point{1, 2}
	fmt.Println("Point{1,2} == Point{1,2}:", i1 == i2) // true

	// 2. Same type, different value -> not equal.
	var i3 any = Point{9, 9}
	fmt.Println("Point{1,2} == Point{9,9}:", i1 == i3) // false

	// 3. Different dynamic type, same "shape" -> not equal (no panic).
	var i4 any = int(2)
	fmt.Println("Point{1,2} == int(2):  ", i1 == i4) // false

	// 4. Untyped nil vs a boxed typed value -> not equal.
	var i5 any = nil
	fmt.Println("Point{1,2} == nil:     ", i1 == i5) // false

	// 5. Comparing uncomparable dynamic types panics; recover() catches it.
	var b1 any = Box{Tags: []string{"go"}}
	var b2 any = Box{Tags: []string{"go"}}
	eq, err := safeEqual(b1, b2)
	if err != nil {
		fmt.Println("Box == Box recovered:  ", err)
	} else {
		fmt.Println("Box == Box:            ", eq)
	}

	// Same comparable Points through safeEqual: no panic, real answer.
	eq, err = safeEqual(i1, i2)
	fmt.Println("safeEqual(Points):     ", eq, err)
}
```

**Output:**

```
Point{1,2} == Point{1,2}: true
Point{1,2} == Point{9,9}: false
Point{1,2} == int(2):   false
Point{1,2} == nil:      false
Box == Box recovered:   runtime error: comparing uncomparable type main.Box
safeEqual(Points):      true <nil>
```

---

## 21. Optional interfaces (feature detection / upgrades)

`🔴 hard`

A function can accept a small base interface yet opportunistically "upgrade" to a richer optional interface via a runtime type assertion. This is exactly how the standard library probes for http.Flusher or io.WriterTo: detect the extra capability if present, fall back if not.

**Steps:**

1. Two interfaces: Sink is the base contract (just Write); Flusher is the OPTIONAL richer one (adds Flush). A type can satisfy both.
2. process(s Sink, ...) only requires a Sink, but inside it does `if f, ok := s.(Flusher); ok` — a type assertion that asks 'does this concrete value ALSO implement Flusher?' at runtime.
3. plainSink implements only Write, so the assertion fails and process takes the fallback path. bufferedSink implements Write AND Flush, so the assertion succeeds and process uses the upgraded f.Flush().
4. The caller's static type stays Sink the whole time; capability detection is dynamic. This is the http.Flusher / io.WriterTo idiom — accept the minimal interface, light up extra behavior when the value supports it.

```go
package main

import (
	"fmt"
	"strings"
)

// Sink is the BASE capability every sink must have.
type Sink interface {
	Write(s string)
}

// Flusher is an OPTIONAL, richer capability. Not every Sink has it.
// This is the http.Flusher / io.WriterTo idiom: a small extra interface
// that callers can probe for at runtime via a type assertion.
type Flusher interface {
	Flush() string
}

// process needs only a Sink, but OPPORTUNISTICALLY upgrades: if the
// concrete value also satisfies Flusher, it uses the richer path.
// Otherwise it falls back to base behavior. The caller's static type
// stays Sink — feature detection happens dynamically.
func process(s Sink, lines []string) {
	for _, line := range lines {
		s.Write(line)
	}
	if f, ok := s.(Flusher); ok {
		fmt.Println("  [flusher path]   ->", f.Flush())
	} else {
		fmt.Println("  [fallback path]  -> no Flush; lines went straight through")
	}
}

// plainSink only implements Write. It is NOT a Flusher.
type plainSink struct{ count int }

func (p *plainSink) Write(s string) { p.count++ }

// bufferedSink implements Write AND Flush, so it satisfies both
// Sink and Flusher. process will detect the upgrade and use Flush.
type bufferedSink struct{ buf []string }

func (b *bufferedSink) Write(s string) { b.buf = append(b.buf, s) }
func (b *bufferedSink) Flush() string  { return strings.Join(b.buf, " | ") }

func main() {
	lines := []string{"alpha", "beta", "gamma"}

	plain := &plainSink{}
	fmt.Println("plainSink (Write only):")
	process(plain, lines)
	fmt.Printf("  wrote %d lines\n", plain.count)

	buffered := &bufferedSink{}
	fmt.Println("bufferedSink (Write + Flush):")
	process(buffered, lines)
}
```

**Output:**

```
plainSink (Write only):
  [fallback path]  -> no Flush; lines went straight through
  wrote 3 lines
bufferedSink (Write + Flush):
  [flusher path]   -> alpha | beta | gamma
```

---

## 22. Decorator / middleware chain

`🔴 hard`

A decorator both implements an interface and holds one, so wrappers can stack around a core to add logging, auth, etc. — this is Go's func(http.Handler) http.Handler middleware pattern.

**Steps:**

1. `Handler` has one method, `Handle(req) string`. The trick: each decorator implements Handler AND stores a `next Handler` field, so they nest like Russian dolls.
2. `CoreHandler` is the real work. `LoggingHandler` records the request, calls `next`, then records the response — pure pass-through that adds behavior around delegation.
3. `AuthHandler` shows decorators control delegation: on a bad token it returns 403 and never calls `next`, short-circuiting the chain before it reaches Core.
4. `main` composes `Logging(Auth(Core))`. Calling the outer Handler runs Logging first, which delegates inward; for the bad request Auth stops the inward flow but Logging still wraps the 403.
5. The log is a shared `*[]string` sink (not a clock) so output is deterministic; the two requests demonstrate the authorized and forbidden paths.

```go
package main

import "fmt"

// Handler is the single behavior every layer shares. Because decorators
// implement Handler AND hold a Handler, they can wrap each other freely.
// This is exactly the func(http.Handler) http.Handler middleware pattern.
type Handler interface {
	Handle(req string) string
}

// CoreHandler is the innermost "real work" — the thing the chain protects.
type CoreHandler struct{}

func (CoreHandler) Handle(req string) string {
	return "200 OK: served " + req
}

// LoggingHandler wraps any Handler, records the call, then delegates.
type LoggingHandler struct {
	next Handler
	log  *[]string // shared sink so output is deterministic, not a clock
}

func (h LoggingHandler) Handle(req string) string {
	*h.log = append(*h.log, "log: -> "+req)
	resp := h.next.Handle(req) // delegate inward
	*h.log = append(*h.log, "log: <- "+resp)
	return resp
}

// AuthHandler wraps any Handler and short-circuits unauthorized requests
// WITHOUT calling next — decorators control whether delegation happens.
type AuthHandler struct {
	next  Handler
	token string
}

func (h AuthHandler) Handle(req string) string {
	if req != h.token {
		return "403 Forbidden: bad token for " + req
	}
	return h.next.Handle(req)
}

func main() {
	var log []string

	// Compose Logging(Auth(Core)): outer layer runs first, then delegates in.
	chain := LoggingHandler{
		log: &log,
		next: AuthHandler{
			token: "secret",
			next:  CoreHandler{},
		},
	}

	fmt.Println("authorized:  ", chain.Handle("secret"))
	fmt.Println("unauthorized:", chain.Handle("hacker"))

	fmt.Println("--- captured log ---")
	for _, line := range log {
		fmt.Println(line)
	}
}
```

**Output:**

```
authorized:   200 OK: served secret
unauthorized: 403 Forbidden: bad token for hacker
--- captured log ---
log: -> secret
log: <- 200 OK: served secret
log: -> hacker
log: <- 403 Forbidden: bad token for hacker
```

---

## 23. Recursive type switch over any (JSON-like walker)

`🔴 hard`

A recursive type switch over `any` is how you process dynamically-typed trees like decoded JSON, where each node could be an object, array, or scalar. It teaches that `any` holds a concrete type you recover with `switch v.(type)`, and that recursion plus sorted map keys gives a clean, deterministic traversal.

**Steps:**

1. `walk(v any, indent int)` is the whole engine: the `switch x := v.(type)` peels off the concrete type hiding inside `any` and binds `x` to it in each case.
2. The two container cases (`map[string]any`, `[]any`) recurse — they call `walk` again on each child with a deeper indent, so arbitrarily nested data unwinds itself.
3. Map keys are collected and `sort.Strings`-ed before printing, because ranging a Go map is randomized; sorting makes the output identical on every run.
4. The scalar cases mirror `encoding/json`: every JSON number decodes to `float64`, plus `string`, `bool`, and `nil` for null; the `default` case is a safety net.
5. `main` hand-builds a `map[string]any` tree (same shape json.Unmarshal would produce) and calls `walk(doc, 0)` to print the labeled, indented tree.

```go
package main

import (
	"fmt"
	"sort"
	"strings"
)

// walk recurses over a value of type any — exactly the shape encoding/json
// produces when you decode into an any: objects become map[string]any, arrays
// become []any, numbers become float64, and the rest map to their Go types.
// The type switch is the idiomatic way to discriminate these dynamic types.
func walk(v any, indent int) {
	pad := strings.Repeat("  ", indent)
	switch x := v.(type) {
	case map[string]any:
		// Sort keys: ranging a map is randomized, so we sort for stable output.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("%sobject:\n", pad)
		for _, k := range keys {
			fmt.Printf("%s  %q ->\n", pad, k)
			walk(x[k], indent+2) // recurse: the value may itself be nested.
		}
	case []any:
		fmt.Printf("%sarray (len %d):\n", pad, len(x))
		for i, e := range x {
			fmt.Printf("%s  [%d] ->\n", pad, i)
			walk(e, indent+2)
		}
	case string:
		fmt.Printf("%sstring: %q\n", pad, x)
	case float64:
		// JSON has only one number type; it always decodes to float64.
		fmt.Printf("%snumber: %g\n", pad, x)
	case bool:
		fmt.Printf("%sbool: %t\n", pad, x)
	case nil:
		fmt.Printf("%snull\n", pad)
	default:
		// Defensive: any type we didn't anticipate still prints something.
		fmt.Printf("%sunknown (%T): %v\n", pad, x, x)
	}
}

func main() {
	// Hand-built tree shaped like decoded JSON, no encoding/json needed.
	doc := map[string]any{
		"name":   "ada",
		"age":    float64(36),
		"admin":  true,
		"spouse": nil,
		"tags":   []any{"math", "engine"},
		"addr": map[string]any{
			"city": "london",
			"zip":  float64(12345),
		},
	}

	fmt.Println("=== walking JSON-like tree ===")
	walk(doc, 0)
}
```

**Output:**

```
=== walking JSON-like tree ===
object:
  "addr" ->
    object:
      "city" ->
        string: "london"
      "zip" ->
        number: 12345
  "admin" ->
    bool: true
  "age" ->
    number: 36
  "name" ->
    string: "ada"
  "spouse" ->
    null
  "tags" ->
    array (len 2):
      [0] ->
        string: "math"
      [1] ->
        string: "engine"
```

---

## 24. Dependency injection with a test fake

`🔴 hard`

Depending on an interface instead of a concrete type lets you swap a real implementation for a recording test double, so you can assert on behavior without real side effects. This is the foundation of testable Go code.

**Steps:**

1. Notifier is the seam: alertAll takes a Notifier interface, so it has no idea whether it's talking to email or a fake.
2. EmailNotifier is the production type — it does a real side effect (printing here stands in for sending mail).
3. FakeNotifier is a test double with a pointer receiver: instead of sending, it appends each message to its sent slice.
4. main runs alertAll twice with the SAME logic — once with the real notifier, once with the fake — then inspects fake.sent.
5. Printing the recorded messages is exactly what a unit test would assert on, proving you can verify behavior with no real side effects.

```go
package main

import (
	"errors"
	"fmt"
	"strings"
)

// Notifier is the seam: alertAll depends on this interface, not on a
// concrete type. That decoupling is what lets us swap a real sender for
// a test double without touching the logic under test.
type Notifier interface {
	Notify(msg string) error
}

// EmailNotifier is the production implementation. It performs a real side
// effect (here, printing — pretend it dials an SMTP server).
type EmailNotifier struct {
	from string
}

func (e EmailNotifier) Notify(msg string) error {
	if strings.TrimSpace(msg) == "" {
		return errors.New("refusing to send empty message")
	}
	fmt.Printf("  [email from %s] %s\n", e.from, msg)
	return nil
}

// FakeNotifier is a test double. Instead of doing real work it RECORDS
// every message, so a test can later assert on what was sent.
type FakeNotifier struct {
	sent []string
}

func (f *FakeNotifier) Notify(msg string) error {
	f.sent = append(f.sent, msg) // capture, don't send
	return nil
}

// alertAll is the code under test. It knows nothing about email or fakes —
// only the interface. Same logic runs in production and in tests.
func alertAll(n Notifier, msgs []string) error {
	for _, m := range msgs {
		if err := n.Notify(m); err != nil {
			return fmt.Errorf("alert %q failed: %w", m, err)
		}
	}
	return nil
}

func main() {
	msgs := []string{"disk 90% full", "cert expires soon"}

	fmt.Println("Run 1: real notifier (side effects happen)")
	real := EmailNotifier{from: "ops@example.com"}
	if err := alertAll(real, msgs); err != nil {
		fmt.Println("  error:", err)
	}

	fmt.Println("Run 2: fake notifier (no side effects)")
	fake := &FakeNotifier{} // pointer: Notify mutates the slice
	if err := alertAll(fake, msgs); err != nil {
		fmt.Println("  error:", err)
	}

	// Now we can assert on behavior — exactly what a unit test does.
	fmt.Printf("Fake recorded %d message(s):\n", len(fake.sent))
	for i, m := range fake.sent {
		fmt.Printf("  %d: %s\n", i+1, m)
	}
}
```

**Output:**

```
Run 1: real notifier (side effects happen)
  [email from ops@example.com] disk 90% full
  [email from ops@example.com] cert expires soon
Run 2: fake notifier (no side effects)
Fake recorded 2 message(s):
  1: disk 90% full
  2: cert expires soon
```

---

## 25. Capstone: a tiny plugin system

`🔴 hard`

A small plugin architecture where one Plugin interface unifies many concrete types behind a registry and an ordered pipeline that short-circuits on the first error. It shows why interfaces matter: composition, polymorphism, and the error interface working together.

**Steps:**

1. Read the Plugin interface first: Name() string and Run(input) (string, error). Everything downstream depends only on this contract, never on concrete types.
2. Note three structs (UpperPlugin, ReversePlugin, NoEmptyPlugin) each satisfy that one interface with value receivers - that single shared interface is the polymorphism.
3. Registry is just a map[string]Plugin; Register stores a plugin under its own Name(), so you can later look plugins up by string.
4. Pipeline threads output->input across an ordered name list and returns immediately when any Run yields a non-nil error (or a name is missing), wrapping it with %w.
5. main registers all three, prints the registry with sorted keys for deterministic output, then runs one happy path and two failing paths to show the error gate.

```go
package main

import (
	"fmt"
	"sort"
	"strings"
)

// Plugin is the contract every plugin obeys: a name and a transform that may fail.
// Programming to this interface (not concrete types) is what enables the registry
// and pipeline below to treat all plugins uniformly — that's polymorphism.
type Plugin interface {
	Name() string
	Run(input string) (string, error)
}

// --- Concrete plugins: each is a different type satisfying the SAME interface. ---

type UpperPlugin struct{}

func (UpperPlugin) Name() string { return "upper" }
func (UpperPlugin) Run(s string) (string, error) {
	return strings.ToUpper(s), nil
}

type ReversePlugin struct{}

func (ReversePlugin) Name() string { return "reverse" }
func (ReversePlugin) Run(s string) (string, error) {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r), nil
}

// NoEmptyPlugin rejects empty strings, returning an error value. The pipeline
// stops the moment any plugin's error is non-nil — the error interface is the gate.
type NoEmptyPlugin struct{}

func (NoEmptyPlugin) Name() string { return "noempty" }
func (NoEmptyPlugin) Run(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("noempty: refusing empty input")
	}
	return s, nil
}

// Registry maps names to plugins so the program can look them up by string.
type Registry struct {
	plugins map[string]Plugin
}

func NewRegistry() *Registry { return &Registry{plugins: map[string]Plugin{}} }

func (r *Registry) Register(p Plugin) { r.plugins[p.Name()] = p }

// Pipeline runs input through an ordered list of plugins, threading the output
// of one into the next, and stops on the first error it meets.
func (r *Registry) Pipeline(input string, order []string) (string, error) {
	out := input
	for _, name := range order {
		p, ok := r.plugins[name]
		if !ok {
			return "", fmt.Errorf("pipeline: no plugin named %q", name)
		}
		next, err := p.Run(out)
		if err != nil {
			return "", fmt.Errorf("step %q failed: %w", name, err)
		}
		out = next
	}
	return out, nil
}

func main() {
	reg := NewRegistry()
	reg.Register(UpperPlugin{})
	reg.Register(ReversePlugin{})
	reg.Register(NoEmptyPlugin{})

	// Show the registry deterministically: sort keys before printing the map.
	names := make([]string, 0, len(reg.plugins))
	for n := range reg.plugins {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Println("registered:", strings.Join(names, ", "))

	// Happy path: noempty -> upper -> reverse.
	out, err := reg.Pipeline("hello", []string{"noempty", "upper", "reverse"})
	fmt.Printf("pipeline ok:  out=%q err=%v\n", out, err)

	// Failing path: noempty short-circuits on empty input, so upper never runs.
	out, err = reg.Pipeline("", []string{"noempty", "upper"})
	fmt.Printf("pipeline err: out=%q err=%v\n", out, err)

	// Unknown plugin name is also surfaced as an error.
	out, err = reg.Pipeline("hi", []string{"missing"})
	fmt.Printf("pipeline err: out=%q err=%v\n", out, err)
}
```

**Output:**

```
registered: noempty, reverse, upper
pipeline ok:  out="OLLEH" err=<nil>
pipeline err: out="" err=step "noempty" failed: noempty: refusing empty input
pipeline err: out="" err=pipeline: no plugin named "missing"
```

---

