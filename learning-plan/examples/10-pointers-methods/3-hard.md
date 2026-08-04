# Step 10 — Pointers & Methods · 🔴 Hard

Examples **17–61**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

## 25. Optional / nullable fields with *T

`🔴 hard` · *Optional values*

A `*T` field lets you tell "never set" (nil) apart from "set to the zero value" — the standard way to model optional config or nullable columns.

**Steps:**

1. effective() returns a default when Timeout is nil.
2. An explicit &0 is honored as a real value, not treated as "missing".

```go
package main

import "fmt"

type Config struct {
	Timeout *int // nil = "not set"; &0 = "explicitly zero"
}

func effective(c Config) int {
	if c.Timeout == nil {
		return 30 // default when the field was never set
	}
	return *c.Timeout // honor an explicit value, even 0
}

func main() {
	var unset Config
	zero := 0
	explicit := Config{Timeout: &zero}

	fmt.Println("unset -> default:", effective(unset))
	fmt.Println("explicit zero:   ", effective(explicit))
}
```

**Output:**

```
unset -> default: 30
explicit zero:    0
```

---

## 26. json.Unmarshal needs a pointer

`🔴 hard` · *Serialization*

Decoders write *into* your variable, so they need its address; a `*T` field also distinguishes an absent JSON key from a zero value.

**Steps:**

1. Pass &u so Unmarshal can populate it; a missing "age" leaves Age nil.
2. An explicit "age":0 makes Age a non-nil pointer to 0.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name string `json:"name"`
	Age  *int   `json:"age"` // pointer distinguishes "missing" from 0
}

func main() {
	var u User
	// Unmarshal needs the ADDRESS so it can write into u.
	_ = json.Unmarshal([]byte(`{"name":"Ada"}`), &u)
	fmt.Println("Name:", u.Name, "AgeMissing:", u.Age == nil)

	var u2 User
	_ = json.Unmarshal([]byte(`{"name":"Bo","age":0}`), &u2)
	fmt.Println("Name:", u2.Name, "Age:", *u2.Age)
}
```

**Output:**

```
Name: Ada AgeMissing: true
Name: Bo Age: 0
```

---

## 27. Typed nil in an interface is not nil

`🔴 hard` · *Interface nil trap*

Returning a nil `*T` as an `error` (or any interface) yields a non-nil interface, because the interface still carries the type — the classic Go trap.

**Steps:**

1. buggy() returns a nil *myError, but the error interface is non-nil.
2. fixed() returns the untyped nil literal, so the comparison is true.

```go
package main

import "fmt"

type myError struct{}

func (e *myError) Error() string { return "boom" }

// BUG: returns a non-nil error even on success — a nil *myError stored in an
// interface is NOT a nil interface (the interface still carries its type).
func buggy() error {
	var e *myError
	return e
}

// FIX: return the untyped nil literal explicitly.
func fixed() error {
	return nil
}

func main() {
	fmt.Println("buggy() == nil:", buggy() == nil)
	fmt.Println("fixed() == nil:", fixed() == nil)
}
```

**Output:**

```
buggy() == nil: false
fixed() == nil: true
```

---

## 28. Shallow copy shares pointer & slice fields

`🔴 hard` · *Copy semantics*

Copying a struct copies its fields shallowly: value fields are independent, but a slice/map/pointer field still shares the same underlying data.

**Steps:**

1. b := a copies the struct; Name becomes independent.
2. b.Members and a.Members share one backing array, so the write leaks across.

```go
package main

import "fmt"

type Team struct {
	Name    string
	Members []string // the slice header is copied, but the backing array is shared
}

func main() {
	a := Team{Name: "A", Members: []string{"x"}}
	b := a // struct copy...

	b.Name = "B"            // ...Name is an independent copy
	b.Members[0] = "hacked" // ...but this mutates a.Members too!

	fmt.Println("a:", a)
	fmt.Println("b:", b)
}
```

**Output:**

```
a: {A [hacked]}
b: {B [hacked]}
```

---

## 29. Defensive copy to break aliasing

`🔴 hard` · *Copy semantics*

To hand out a struct that can't be mutated through its shared slice/map/pointer fields, deep-copy those fields explicitly.

**Steps:**

1. Clone() allocates a fresh slice and copies the elements in.
2. Mutating the clone now leaves the original untouched.

```go
package main

import "fmt"

type Team struct {
	Name    string
	Members []string
}

// Clone returns an independent copy so callers can't mutate our internals.
func (t Team) Clone() Team {
	m := make([]string, len(t.Members))
	copy(m, t.Members)
	return Team{Name: t.Name, Members: m}
}

func main() {
	a := Team{Name: "A", Members: []string{"x"}}
	b := a.Clone()
	b.Members[0] = "changed"

	fmt.Println("a.Members:", a.Members) // untouched
	fmt.Println("b.Members:", b.Members)
}
```

**Output:**

```
a.Members: [x]
b.Members: [changed]
```

---

## 30. append reallocation invalidates element pointers

`🔴 hard` · *Aliasing*

A pointer into a slice element is only valid until an `append` grows the slice past its capacity and moves the backing array — then the old pointer is stale.

**Steps:**

1. p := &s[0] points into the current backing array.
2. append over capacity allocates a new array; p still sees the old one.

```go
package main

import "fmt"

func main() {
	s := make([]int, 1, 1) // len 1, cap 1 — no room to grow in place
	s[0] = 10
	p := &s[0] // pointer into the CURRENT backing array

	s = append(s, 20) // over capacity -> Go allocates a NEW backing array
	s[0] = 999        // writes into the new array

	fmt.Println("*p (old array): ", *p)
	fmt.Println("s[0] (new array):", s[0])
	fmt.Println("still aliased:  ", &s[0] == p)
}
```

**Output:**

```
*p (old array):  10
s[0] (new array): 999
still aliased:   false
```

---

## 31. Loop variable address (Go 1.22+)

`🔴 hard` · *Loops & closures*

Since Go 1.22 each loop iteration gets a fresh loop variable, so taking `&v` (or capturing v in a closure) inside a loop yields a distinct value each time.

**Steps:**

1. &v is stored on every iteration.
2. Under Go 1.22+ the three pointers address three different variables (1 2 3); pre-1.22 they all shared one (3 3 3).

```go
package main

import "fmt"

func main() {
	var ptrs []*int
	// Go 1.22+: each iteration gets a FRESH v, so &v differs every time.
	// (Under Go 1.21 and earlier this printed "3 3 3" — one shared variable.)
	for _, v := range []int{1, 2, 3} {
		ptrs = append(ptrs, &v)
	}
	for _, p := range ptrs {
		fmt.Print(*p, " ")
	}
	fmt.Println()
}
```

**Output:**

```
1 2 3 
```

---

## 32. Method values vs method expressions

`🔴 hard` · *Method values*

`c.Inc` is a *method value* with the receiver already bound; `(*Counter).Inc` is a *method expression* whose receiver is an explicit first argument.

**Steps:**

1. f := c.Inc captures c now; each f() mutates that same c.
2. g := (*Counter).Inc takes the receiver explicitly as g(c).

```go
package main

import "fmt"

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }

func main() {
	c := &Counter{}

	// method VALUE: the receiver c is bound now; calling f() is like c.Inc()
	f := c.Inc
	f()
	f()
	fmt.Println("after method value:", c.n)

	// method EXPRESSION: receiver becomes an explicit first argument
	g := (*Counter).Inc
	g(c)
	fmt.Println("after method expr: ", c.n)
}
```

**Output:**

```
after method value: 2
after method expr:  3
```

---

## 33. Embedding by value vs by pointer

`🔴 hard` · *Embedding*

Pointer-receiver methods of an embedded type are promoted to the outer struct; embedding by value works because the outer struct is addressable, and embedding by pointer shares the inner value.

**Steps:**

1. Car embeds Engine by value; c.Tune auto-addresses to (&c.Engine).Tune.
2. Truck embeds *Engine; the method mutates the pointed-to Engine directly.

```go
package main

import "fmt"

type Engine struct{ hp int }

func (e *Engine) Tune(by int) { e.hp += by } // pointer receiver

type Car struct {
	Engine // embedded by VALUE — Car is addressable, so *Engine methods promote
}

type Truck struct {
	*Engine // embedded by POINTER
}

func main() {
	c := Car{Engine{hp: 100}}
	c.Tune(50) // promoted + auto-addressed as (&c.Engine).Tune(50)
	fmt.Println("car hp:  ", c.hp)

	t := Truck{&Engine{hp: 200}}
	t.Tune(50)
	fmt.Println("truck hp:", t.hp)
}
```

**Output:**

```
car hp:   150
truck hp: 250
```

---

## 34. Self-referential tree with *Node

`🔴 hard` · *Data structures*

Recursive data structures are built from pointer fields; a nil-receiver method doubles as the "empty subtree" base case for insert and traversal.

**Steps:**

1. Insert on a nil *Node returns a new leaf, so the tree grows by reassignment.
2. InOrder recurses left, visits, then right, yielding sorted output.

```go
package main

import "fmt"

type Node struct {
	val         int
	left, right *Node
}

// Insert works even on a nil receiver, which becomes a new leaf.
func (n *Node) Insert(v int) *Node {
	if n == nil {
		return &Node{val: v}
	}
	if v < n.val {
		n.left = n.left.Insert(v)
	} else {
		n.right = n.right.Insert(v)
	}
	return n
}

func (n *Node) InOrder(visit func(int)) {
	if n == nil {
		return
	}
	n.left.InOrder(visit)
	visit(n.val)
	n.right.InOrder(visit)
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4} {
		root = root.Insert(v)
	}
	root.InOrder(func(v int) { fmt.Print(v, " ") })
	fmt.Println()
}
```

**Output:**

```
1 3 4 5 8 
```

---

## 35. atomic.Pointer[T] for lock-free swaps

`🔴 hard` · *Concurrency*

`atomic.Pointer[T]` publishes a whole struct atomically — readers always see a fully-constructed value, with no lock, ideal for hot-reloadable config.

**Steps:**

1. Several goroutines Store a fresh *Config concurrently.
2. After the WaitGroup, Load returns one valid config with no data race.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Config struct{ Rate int }

func main() {
	var cur atomic.Pointer[Config]
	cur.Store(&Config{Rate: 1})

	var wg sync.WaitGroup
	for i := 2; i <= 5; i++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			cur.Store(&Config{Rate: r}) // atomically publish a new *Config
		}(i)
	}
	wg.Wait()

	got := cur.Load().Rate
	fmt.Println("final rate in 2..5:", got >= 2 && got <= 5)
}
```

**Output:**

```
final rate in 2..5: true
```

> Run it with `go run -race .` to confirm there's no data race.

---

## 36. sync.Pool of *T to reuse allocations

`🔴 hard` · *Pooling*

`sync.Pool` recycles heap objects (always pointers) to cut allocation and GC pressure in hot paths; always reset a pooled value before reuse.

**Steps:**

1. Get returns a *bytes.Buffer (a fresh one via New, or a recycled one).
2. Reset it, use it, then Put it back with defer for the next caller.

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
)

// A pool recycles *bytes.Buffer values to cut allocations in hot paths.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) }, // returns *bytes.Buffer
}

func greet(name string) string {
	b := bufPool.Get().(*bytes.Buffer)
	defer bufPool.Put(b) // hand it back for the next caller
	b.Reset()            // a reused pointer may still hold old data
	b.WriteString("hi ")
	b.WriteString(name)
	return b.String()
}

func main() {
	fmt.Println(greet("Ada"))
	fmt.Println(greet("Bo"))
}
```

**Output:**

```
hi Ada
hi Bo
```

---

## 37. reflect settability requires a pointer

`🔴 hard` · *Reflection*

`reflect` can only *set* a value it can address, so you must reflect over a pointer and call `.Elem()`; reflecting a plain value gives an unsettable copy.

**Steps:**

1. reflect.ValueOf(x) wraps a copy — CanSet() is false.
2. reflect.ValueOf(&x).Elem() is addressable — CanSet() is true and SetInt writes through.

```go
package main

import (
	"fmt"
	"reflect"
)

func main() {
	x := 10

	// reflect.ValueOf(x) wraps a COPY — not settable.
	fmt.Println("copy settable:", reflect.ValueOf(x).CanSet())

	// Pass &x, then .Elem() to reach the addressable value behind the pointer.
	v := reflect.ValueOf(&x).Elem()
	fmt.Println("elem settable:", v.CanSet())
	v.SetInt(42)
	fmt.Println("x:", x)
}
```

**Output:**

```
copy settable: false
elem settable: true
x: 42
```

---

## 38. unsafe.Pointer and field offsets

`🔴 hard` · *Unsafe*

`unsafe.Pointer` bridges between pointer types and, with `unsafe.Offsetof`/`unsafe.Add`, reaches a struct field by raw byte offset — powerful, unportable, and rarely needed.

**Steps:**

1. Take the struct's base address and add Flags' byte offset.
2. Reinterpret that address as *uint32 and write through it.

```go
package main

import (
	"fmt"
	"unsafe"
)

type Header struct {
	Magic uint32
	Flags uint32
}

func main() {
	h := Header{Magic: 0xCAFE, Flags: 1}
	base := unsafe.Pointer(&h)

	// Reach Flags by adding its byte offset to the struct's base address.
	off := unsafe.Offsetof(h.Flags)
	flags := (*uint32)(unsafe.Add(base, off))
	*flags = 9 // write through the raw pointer

	fmt.Println("sizeof:", unsafe.Sizeof(h))
	fmt.Println("flags: ", h.Flags)
}
```

**Output:**

```
sizeof: 8
flags:  9
```

> Reserve `unsafe` for FFI, serialization, and micro-optimizations — the compiler makes no safety guarantees here.

---

## 39. runtime.SetFinalizer on a pointer

`🔴 hard` · *Finalizers*

A finalizer is a function the GC may call after a pointer becomes unreachable — a last-resort cleanup net, never a substitute for explicit Close/defer because timing isn't guaranteed.

**Steps:**

1. SetFinalizer registers a cleanup fn tied to the pointer f.
2. After f goes out of scope, runtime.GC() lets the finalizer run and send on the channel.

```go
package main

import (
	"fmt"
	"runtime"
)

type File struct{ name string }

func main() {
	done := make(chan string, 1)

	func() {
		f := &File{name: "data.txt"}
		// The finalizer runs at SOME point after f becomes unreachable —
		// never rely on it for correctness; use it only as a safety net.
		runtime.SetFinalizer(f, func(f *File) {
			done <- "cleaned up " + f.name
		})
	}() // f is unreachable after this call returns

	runtime.GC() // nudge the collector so the finalizer gets scheduled
	fmt.Println(<-done)
}
```

**Output:**

```
cleaned up data.txt
```

---

## 40. weak.Pointer to cache without leaking

`🔴 hard` · *Weak refs*

A `weak.Pointer[T]` (Go 1.24+) references a value without keeping it alive, so the GC can still reclaim it — the basis of memory-sensitive caches; `Value()` returns nil once it's collected.

**Steps:**

1. weak.Make(b) observes b without pinning it in memory.
2. After dropping the strong reference and forcing GC, Value() reports nil.

```go
package main

import (
	"fmt"
	"runtime"
	"weak"
)

type Big struct{ payload [1024]byte }

func main() {
	b := &Big{}
	wp := weak.Make(b) // a weak reference does NOT keep b alive

	fmt.Println("alive before GC:", wp.Value() != nil)

	b = nil      // drop the only strong reference
	runtime.GC() // b is now collectable; the weak pointer is cleared

	fmt.Println("alive after GC: ", wp.Value() != nil)
}
```

**Output:**

```
alive before GC: true
alive after GC:  false
```

---

## 41. Stringer with a pointer receiver

`🔴 hard` · *Method sets & fmt*

If `String()` has a **pointer** receiver, only `*T` implements `fmt.Stringer`. Printing a value uses the default format; printing its address uses `String()`.

**Steps:**

1. `String()` is defined on `*Temp`, so `Temp`'s method set excludes it.
2. `Println(t)` sees a plain `Temp` (default format); `Println(&t)` sees a `*Temp` (Stringer).

```go
package main

import "fmt"

type Temp struct{ c int }

func (t *Temp) String() string { return fmt.Sprintf("%d°C", t.c) }

func main() {
	t := Temp{c: 20}
	fmt.Println(t)  // value: Temp does NOT implement Stringer → default format
	fmt.Println(&t) // pointer: *Temp implements Stringer → uses String()
}
```

**Output:**

```
{20}
20°C
```

---

## 42. MarshalJSON with a pointer receiver

`🔴 hard` · *Method sets & json*

The same method-set rule bites `encoding/json`: a pointer-receiver `MarshalJSON` is only used when you marshal a pointer (or an addressable value).

**Steps:**

1. `MarshalJSON` is on `*Money`, so marshaling a `Money` value falls back to default struct encoding.
2. Marshaling `&m` uses the custom method.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Money struct{ Cents int }

func (m *Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"$%d.%02d"`, m.Cents/100, m.Cents%100)), nil
}

func main() {
	m := Money{Cents: 1250}
	b1, _ := json.Marshal(m)  // value: default marshaling (no MarshalJSON)
	b2, _ := json.Marshal(&m) // pointer: uses MarshalJSON
	fmt.Printf("value:   %s\n", b1)
	fmt.Printf("pointer: %s\n", b2)
}
```

**Output:**

```
value:   {"Cents":1250}
pointer: "$12.50"
```

---

## 43. In-place linked-list reversal

`🔴 hard` · *Pointer rewiring*

Reverse a singly-linked list by rewiring each node's `next` pointer as you walk, tracking `prev`.

**Steps:**

1. Save `head.next`, point `head.next` back at `prev`, then advance `prev` and `head`.
2. When `head` is nil, `prev` is the new head.

```go
package main

import "fmt"

type Node struct {
	val  int
	next *Node
}

func reverse(head *Node) *Node {
	var prev *Node
	for head != nil {
		next := head.next // save the rest
		head.next = prev  // rewire this node backward
		prev = head       // advance prev
		head = next       // advance head
	}
	return prev
}

func show(head *Node) {
	for n := head; n != nil; n = n.next {
		fmt.Print(n.val, " ")
	}
	fmt.Println()
}

func main() {
	head := &Node{1, &Node{2, &Node{3, nil}}}
	show(head)
	head = reverse(head)
	show(head)
}
```

**Output:**

```
1 2 3 
3 2 1 
```

---

## 44. Floyd's cycle detection

`🔴 hard` · *Two pointers*

A slow pointer (one step) and a fast pointer (two steps) meet if and only if the list has a cycle.

**Steps:**

1. Advance `slow` by 1 and `fast` by 2 each iteration.
2. If they ever point at the same node, there's a cycle; if `fast` reaches nil, there isn't.

```go
package main

import "fmt"

type Node struct {
	val  int
	next *Node
}

func hasCycle(head *Node) bool {
	slow, fast := head, head
	for fast != nil && fast.next != nil {
		slow = slow.next
		fast = fast.next.next
		if slow == fast {
			return true
		}
	}
	return false
}

func main() {
	acyclic := &Node{1, &Node{2, &Node{3, nil}}}
	fmt.Println("acyclic has cycle:", hasCycle(acyclic))

	n3 := &Node{val: 3}
	n2 := &Node{val: 2, next: n3}
	n1 := &Node{val: 1, next: n2}
	n3.next = n2 // create a cycle: 3 → 2
	fmt.Println("cyclic has cycle: ", hasCycle(n1))
}
```

**Output:**

```
acyclic has cycle: false
cyclic has cycle:  true
```

---

## 45. container/list doubly-linked list

`🔴 hard` · *Stdlib lists*

`container/list` is the standard doubly-linked list; you traverse it through `*list.Element` pointers.

**Steps:**

1. `PushBack`/`PushFront` insert elements.
2. Walk from `Front()` following `Next()` until nil.

```go
package main

import (
	"container/list"
	"fmt"
)

func main() {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushFront(0) // 0, 1, 2

	for e := l.Front(); e != nil; e = e.Next() {
		fmt.Print(e.Value, " ")
	}
	fmt.Println()
	fmt.Println("len:", l.Len())
}
```

**Output:**

```
0 1 2 
len: 3
```

---

## 46. Mutate a slice element via &s[i]

`🔴 hard` · *Element pointers*

`&s[i]` gives a pointer to the element itself (slice elements are addressable), so you can mutate its fields in place — unlike the throwaway copy a `range` loop hands you.

**Steps:**

1. `p := &players[1]` points into the backing array.
2. Writing through `p` updates the stored element.

```go
package main

import "fmt"

type Player struct {
	name  string
	score int
}

func main() {
	players := []Player{{"a", 0}, {"b", 0}}

	p := &players[1] // pointer to the element itself
	p.score += 10
	p.name = "bob"

	fmt.Printf("%+v\n", players)
}
```

**Output:**

```
[{name:a score:0} {name:bob score:10}]
```

---

## 47. Address of a struct field

`🔴 hard` · *Addressability*

When a struct is addressable, so is each of its fields: `&c.field` is a `*T` you can hand to a mutating function.

**Steps:**

1. `c` is a local variable (addressable), so `&c.retries` is valid.
2. `bump` increments through the field pointer.

```go
package main

import "fmt"

type Config struct {
	retries int
	timeout int
}

func bump(n *int) { *n++ }

func main() {
	c := Config{retries: 3, timeout: 30}
	bump(&c.retries) // address of a single field
	fmt.Printf("%+v\n", c)
}
```

**Output:**

```
{retries:4 timeout:30}
```

---

## 48. errors.As needs a pointer

`🔴 hard` · *Error chains*

`errors.As` walks the wrapped-error chain and, on a match, **assigns** into the target — so you pass a pointer to your target variable (`&nf`, which is a `**NotFoundError`).

**Steps:**

1. Wrap a `*NotFoundError` with `%w`.
2. `errors.As(err, &nf)` finds it and sets `nf`.

```go
package main

import (
	"errors"
	"fmt"
)

type NotFoundError struct{ Key string }

func (e *NotFoundError) Error() string { return "not found: " + e.Key }

func main() {
	err := fmt.Errorf("lookup failed: %w", &NotFoundError{Key: "user:1"})

	var nf *NotFoundError
	if errors.As(err, &nf) { // &nf so As can assign into nf
		fmt.Println("matched, key =", nf.Key)
	}
}
```

**Output:**

```
matched, key = user:1
```

---

## 49. flag binds into pointers

`🔴 hard` · *Stdlib pointers*

The `flag` package writes parsed values through pointers: the `XxxVar` forms take a pointer you own, and the plain `Xxx` form returns a `*T`.

**Steps:**

1. `IntVar(&port, …)` binds into your variable; `Bool(…)` returns a `*bool`.
2. `Parse` fills them in (a `FlagSet` + explicit args keeps this deterministic).

```go
package main

import (
	"flag"
	"fmt"
)

func main() {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)

	var port int
	var host string
	fs.IntVar(&port, "port", 8080, "port") // binds into &port
	fs.StringVar(&host, "host", "localhost", "host")
	verbose := fs.Bool("verbose", false, "verbose") // returns a *bool

	_ = fs.Parse([]string{"-port", "9090", "-verbose"})
	fmt.Printf("host=%s port=%d verbose=%v\n", host, port, *verbose)
}
```

**Output:**

```
host=localhost port=9090 verbose=true
```

---

## 50. Mutating through an interface holding *T

`🔴 hard` · *Interfaces & pointers*

An interface value can hold a `*T`. Calling a pointer method through the interface mutates the underlying value — the interface carries the pointer, not a copy of the struct.

**Steps:**

1. Store a `*counter` in a `Counter` interface.
2. `i.Inc()` mutates the same struct `c` points to.

```go
package main

import "fmt"

type Counter interface{ Inc() }

type counter struct{ n int }

func (c *counter) Inc() { c.n++ }

func main() {
	c := &counter{}
	var i Counter = c // the interface holds the *counter
	i.Inc()
	i.Inc()
	fmt.Println("n =", c.n) // mutations are visible on c
}
```

**Output:**

```
n = 2
```

---

## 51. Sorting a []*T

`🔴 hard` · *Pointer slices*

Sorting a `[]*T` reorders the pointers; the pointed-to structs are never copied and their identity is preserved.

**Steps:**

1. `slices.SortFunc` swaps the pointer elements.
2. The comparator dereferences to compare fields.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type Person struct {
	name string
	age  int
}

func main() {
	people := []*Person{{"carol", 30}, {"alice", 20}, {"bob", 25}}
	slices.SortFunc(people, func(a, b *Person) int { return cmp.Compare(a.age, b.age) })
	for _, p := range people {
		fmt.Printf("%s(%d) ", p.name, p.age)
	}
	fmt.Println()
}
```

**Output:**

```
alice(20) bob(25) carol(30) 
```

---

## 52. Don't copy a struct with a sync.Mutex

`🔴 hard` · *Copylocks*

A struct that embeds a `sync.Mutex` must never be copied — a copy gets its own lock, so mutual exclusion silently breaks. Work through a `*pointer`. (`go vet` reports *"passes lock by value"* for a value-copy version.)

**Steps:**

1. Give `Counter` a `sync.Mutex` and pointer-receiver methods.
2. Only ever hold a `*Counter` so all users share one lock.

```go
package main

import (
	"fmt"
	"sync"
)

type Counter struct {
	mu sync.Mutex
	n  int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func main() {
	c := &Counter{} // a *Counter — never copy the struct
	c.Inc()
	c.Inc()
	fmt.Println("n =", c.n)
}
```

**Output:**

```
n = 2
```

---

## 53. Lock-free stack with CompareAndSwap

`🔴 hard` · *Atomics*

A lock-free stack: `Push`/`Pop` retry a `CompareAndSwap` on the top `*node` until they win, so many goroutines mutate the structure without a mutex.

**Steps:**

1. Load the current top, prepare the new node, and CAS it into place; retry on contention.
2. 100 goroutines push 1..100; popping them all yields a deterministic sum (5050).

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type node struct {
	val  int
	next *node
}

type Stack struct{ top atomic.Pointer[node] }

func (s *Stack) Push(v int) {
	n := &node{val: v}
	for {
		old := s.top.Load()
		n.next = old
		if s.top.CompareAndSwap(old, n) {
			return
		}
	}
}

func (s *Stack) Pop() (int, bool) {
	for {
		old := s.top.Load()
		if old == nil {
			return 0, false
		}
		if s.top.CompareAndSwap(old, old.next) {
			return old.val, true
		}
	}
}

func main() {
	var s Stack
	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		wg.Add(1)
		go func(v int) { defer wg.Done(); s.Push(v) }(i)
	}
	wg.Wait()

	sum, count := 0, 0
	for {
		v, ok := s.Pop()
		if !ok {
			break
		}
		sum += v
		count++
	}
	fmt.Println("pushed & popped:", count, "sum:", sum) // 100, 5050
}
```

**Output:**

```
pushed & popped: 100 sum: 5050
```

> Run it with `go run -race .` — the CAS loop is data-race-free.

---

## 54. Generic linked stack Stack[T]

`🔴 hard` · *Generics & pointers*

Generics and pointers combine: a generic linked stack whose nodes point to the next `*node[T]`.

**Steps:**

1. `Push` prepends a new `*node[T]`.
2. `Pop` returns the top value (or the zero value of `T` when empty).

```go
package main

import "fmt"

type node[T any] struct {
	val  T
	next *node[T]
}

type Stack[T any] struct{ top *node[T] }

func (s *Stack[T]) Push(v T) { s.top = &node[T]{val: v, next: s.top} }

func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if s.top == nil {
		return zero, false
	}
	v := s.top.val
	s.top = s.top.next
	return v, true
}

func main() {
	var s Stack[string]
	s.Push("a")
	s.Push("b")
	s.Push("c")
	for {
		v, ok := s.Pop()
		if !ok {
			break
		}
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
c b a 
```

---

## 55. **T as an out-parameter

`🔴 hard` · *Double pointers*

Rare in Go (prefer returning the value), but a function can assign the caller's pointer through a `**T` — a C-style "allocate and set" out-parameter.

**Steps:**

1. `acquire` takes `**Resource` and writes `*out = &Resource{…}`.
2. The caller passes `&r`, so its `r` now points at the new value.

```go
package main

import "fmt"

type Resource struct{ id int }

func acquire(out **Resource, id int) {
	*out = &Resource{id: id} // set the caller's pointer
}

func main() {
	var r *Resource // starts nil
	acquire(&r, 42) // pass the address of our pointer
	fmt.Printf("r = %+v\n", *r)
}
```

**Output:**

```
r = {id:42}
```

---

## 56. *[]T to replace the caller's slice

`🔴 hard` · *Slice headers*

Passing `*[]T` lets a function replace the caller's slice **header** (length/pointer), not just its elements — e.g. filtering in place.

**Steps:**

1. Build the result into `(*s)[:0]`, reusing the backing array.
2. Assign `*s = out` to publish the new, shorter slice.

```go
package main

import "fmt"

func keepEven(s *[]int) {
	out := (*s)[:0] // reuse the backing array
	for _, v := range *s {
		if v%2 == 0 {
			out = append(out, v)
		}
	}
	*s = out // publish the new (shorter) slice back to the caller
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6}
	keepEven(&nums)
	fmt.Println(nums)
}
```

**Output:**

```
[2 4 6]
```

---

## 57. Nil out []*T elements for the GC

`🔴 hard` · *Memory hygiene*

Nil out `[]*T` elements you're done with so the GC can reclaim them even while the slice header is still alive — common in pools and ring buffers.

**Steps:**

1. Set each `pool[i] = nil` to drop the reference.
2. The `Big` values become collectable; the slice length is unchanged.

```go
package main

import "fmt"

type Big struct{ data [1024]byte }

func main() {
	pool := []*Big{{}, {}, {}}
	for i := range pool {
		pool[i] = nil // release the reference; the Big is now collectable
	}

	live := 0
	for _, p := range pool {
		if p != nil {
			live++
		}
	}
	fmt.Println("len:", len(pool), "live:", live)
}
```

**Output:**

```
len: 3 live: 0
```

---

## 58. Pointers as identity map keys

`🔴 hard` · *Identity*

Pointers make good identity keys: two `*T` with equal contents are still distinct keys, because a pointer compares by address, not by value.

**Steps:**

1. Build two `*Session` with identical fields but different addresses.
2. Both are separate keys in the map.

```go
package main

import "fmt"

type Session struct{ user string }

func main() {
	a := &Session{user: "alice"}
	b := &Session{user: "alice"} // same contents, different address

	seen := map[*Session]int{}
	seen[a] = 1
	seen[b] = 2

	fmt.Println("distinct keys:", len(seen)) // 2 — keyed by identity
	fmt.Println("a's value:", seen[a])
	fmt.Println("a != b:", a != b)
}
```

**Output:**

```
distinct keys: 2
a's value: 1
a != b: true
```

---

## 59. Copy-on-write with atomic.Pointer

`🔴 hard` · *Atomics*

Copy-on-write: readers `Load` the current `*Config` lock-free; a writer publishes a brand-new `*Config` with `Store`. No reader ever observes a half-updated struct.

**Steps:**

1. Store an initial `*Config`; readers `Load().version`.
2. Hot-reload by `Store`-ing a completely new `*Config`.

```go
package main

import (
	"fmt"
	"sync/atomic"
)

type Config struct{ version int }

type Holder struct{ cfg atomic.Pointer[Config] }

func main() {
	var h Holder
	h.cfg.Store(&Config{version: 1})
	fmt.Println("readers see version:", h.cfg.Load().version)

	h.cfg.Store(&Config{version: 2}) // hot-reload: publish a new config atomically
	fmt.Println("after reload:       ", h.cfg.Load().version)
}
```

**Output:**

```
readers see version: 1
after reload:        2
```

---

## 60. Capstone: an intrusive LRU list

`🔴 hard` · *Doubly-linked list*

The guts of an LRU cache: an **intrusive** doubly-linked list (each node holds `prev`/`next *node`) plus a map index, giving O(1) move-to-front.

**Steps:**

1. `Touch` inserts a new node at the front, or unlinks an existing one and re-pushes it.
2. `unlink`/`pushFront` rewire the four neighbouring pointers (and head/tail).

```go
package main

import "fmt"

type node struct {
	key        string
	prev, next *node
}

type LRU struct {
	head, tail *node // most- and least-recently-used
	index      map[string]*node
}

func newLRU() *LRU { return &LRU{index: map[string]*node{}} }

func (l *LRU) unlink(n *node) {
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		l.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		l.tail = n.prev
	}
}

func (l *LRU) pushFront(n *node) {
	n.prev, n.next = nil, l.head
	if l.head != nil {
		l.head.prev = n
	}
	l.head = n
	if l.tail == nil {
		l.tail = n
	}
}

func (l *LRU) Touch(key string) {
	n, ok := l.index[key]
	if !ok {
		n = &node{key: key}
		l.index[key] = n
	} else {
		l.unlink(n)
	}
	l.pushFront(n)
}

func (l *LRU) order() []string {
	var out []string
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.key)
	}
	return out
}

func main() {
	l := newLRU()
	l.Touch("a")
	l.Touch("b")
	l.Touch("c")
	fmt.Println("after a,b,c:  ", l.order()) // most-recent first
	l.Touch("a")                             // move "a" back to the front
	fmt.Println("after touch a:", l.order())
}
```

**Output:**

```
after a,b,c:   [c b a]
after touch a: [a c b]
```

---

## 61. Partial update (PATCH) with pointer fields: absent vs zero

`🔴 hard` · *Partial updates*

Extending example 25 (`*T` for optional fields) into a real HTTP PATCH: a request struct with **pointer** fields lets `encoding/json` tell "the client omitted this key" (pointer stays nil) from "the client explicitly sent the zero value" (non-nil pointer at `false`/`""`). A plain `bool` collapses both into `false`, so you can't apply a partial update correctly.

**Steps:**

1. `Settings` holds plain fields (`Channel string`, `DailyDigest bool`) — the stored state you want to patch.
2. `patchReq` mirrors it with `*string` / `*bool` fields plus json tags; after Unmarshal, a nil field means "absent", a non-nil field means "present" (even if it points at `false`).
3. Unmarshal two bodies: one omits `daily_digest`, the other sets it to `false`. The omitted one leaves `p1.DailyDigest == nil`; the explicit-false one gives a non-nil `*bool` pointing at `false`.
4. `apply` copies `cur` and overlays only the non-nil fields, so an omitted field is preserved while an explicit value (even a zero one) is written.
5. The before/after lines prove it: `apply(body1)` keeps `DailyDigest:true` (omitted → preserved), while `apply(body2)` writes `DailyDigest:false` (explicitly sent → applied).

```go
package main

import (
	"encoding/json"
	"fmt"
)

// Settings is the stored state, with PLAIN fields. A plain bool cannot tell
// "the client omitted daily_digest" apart from "the client sent false".
type Settings struct {
	Channel     string
	DailyDigest bool
}

// patchReq models a PATCH body. Every field is a POINTER, so nil means
// "absent from the JSON" while a non-nil pointer means "explicitly provided" —
// even when the value provided is the zero value (empty string / false).
type patchReq struct {
	Channel     *string `json:"channel"`
	DailyDigest *bool   `json:"daily_digest"`
}

// apply overlays only the fields that were actually present (non-nil) onto a
// COPY of cur, leaving omitted fields untouched. This is the whole point of
// pointer PATCH DTOs: absent fields are preserved, present ones are applied.
func (p patchReq) apply(cur Settings) Settings {
	next := cur // copy; we never mutate the caller's value
	if p.Channel != nil {
		next.Channel = *p.Channel
	}
	if p.DailyDigest != nil {
		next.DailyDigest = *p.DailyDigest
	}
	return next
}

func main() {
	current := Settings{Channel: "email", DailyDigest: true}

	// Body 1 OMITS daily_digest entirely.
	body1 := []byte(`{"channel":"sms"}`)
	// Body 2 EXPLICITLY sets daily_digest to false.
	body2 := []byte(`{"channel":"sms","daily_digest":false}`)

	var p1, p2 patchReq
	_ = json.Unmarshal(body1, &p1)
	_ = json.Unmarshal(body2, &p2)

	// The pointer distinguishes ABSENT from ZERO: the omitted field stays nil,
	// while the explicit false gives a non-nil *bool pointing AT false.
	fmt.Printf("body1 DailyDigest ptr == nil: %-5v (absent)\n", p1.DailyDigest == nil)
	fmt.Printf("body2 DailyDigest ptr == nil: %-5v (present, value=%v)\n",
		p2.DailyDigest == nil, *p2.DailyDigest)

	fmt.Println()
	fmt.Printf("before:       %+v\n", current)
	fmt.Printf("apply(body1): %+v  <- DailyDigest preserved (was omitted)\n", p1.apply(current))
	fmt.Printf("apply(body2): %+v  <- DailyDigest applied (explicit false)\n", p2.apply(current))
}
```

**Output:**

```
body1 DailyDigest ptr == nil: true  (absent)
body2 DailyDigest ptr == nil: false (present, value=false)

before:       {Channel:email DailyDigest:true}
apply(body1): {Channel:sms DailyDigest:true}  <- DailyDigest preserved (was omitted)
apply(body2): {Channel:sms DailyDigest:false}  <- DailyDigest applied (explicit false)
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
