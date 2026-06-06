# Step 11 — Interfaces · 🟡 Medium

Examples **9–18**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
