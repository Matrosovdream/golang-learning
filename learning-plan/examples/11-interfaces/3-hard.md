# Step 11 — Interfaces · 🔴 Hard

Examples **19–25**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
