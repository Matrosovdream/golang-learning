# Step 12 — Errors & Error Handling · Examples

A library of **28 runnable examples**. Each is a complete `package main` program:
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

- [1. error is just an interface](#1-error-is-just-an-interface)
- [2. errors.New for a simple error](#2-errorsnew-for-a-simple-error)
- [3. (result, error) + early return](#3-result-error--early-return)
- [4. nil error means success](#4-nil-error-means-success)
- [5. fmt.Errorf for a formatted error](#5-fmterrorf-for-a-formatted-error)

**Medium**

- [6. Sentinel errors + errors.Is](#6-sentinel-errors--errorsis)
- [7. errors.New values are distinct](#7-errorsnew-values-are-distinct)
- [8. Wrapping with %w](#8-wrapping-with-w)
- [9. errors.Is sees through a wrap](#9-errorsis-sees-through-a-wrap)
- [10. errors.Unwrap](#10-errorsunwrap)
- [11. %w vs %v](#11-w-vs-v)
- [12. Multi-layer wrapping builds a trace](#12-multi-layer-wrapping-builds-a-trace)
- [13. Custom error types](#13-custom-error-types)
- [14. errors.As extracts a custom type](#14-errorsas-extracts-a-custom-type)
- [15. errors.As with a standard library error](#15-errorsas-with-a-standard-library-error)
- [16. Error message style and composition](#16-error-message-style-and-composition)

**Hard**

- [17. errors.Join combines multiple errors](#17-errorsjoin-combines-multiple-errors)
- [18. errors.Is on a joined error](#18-errorsis-on-a-joined-error)
- [19. errors.As through a wrap chain](#19-errorsas-through-a-wrap-chain)
- [20. Sentinel 'not found' repository pattern](#20-sentinel-not-found-repository-pattern)
- [21. Custom error carrying structured data](#21-custom-error-carrying-structured-data)
- [22. == misses a wrapped cause](#22--misses-a-wrapped-cause)
- [23. Custom error with an Unwrap method](#23-custom-error-with-an-unwrap-method)
- [24. A custom Is method](#24-a-custom-is-method)
- [25. panic for an impossible state](#25-panic-for-an-impossible-state)
- [26. recover turns a panic into an error](#26-recover-turns-a-panic-into-an-error)
- [27. recover at a boundary (middleware-style)](#27-recover-at-a-boundary-middleware-style)
- [28. Don't ignore errors](#28-dont-ignore-errors)

---

## 1. error is just an interface

`🟢 easy` · *Foundations*

Go's error is the one-method interface interface{ Error() string }; any type with that method is an error, and fmt prints it via Error().

**Steps:**

1. Give a struct an Error() string method.
2. Assign it to an error variable; printing calls Error().

```go
package main

import "fmt"

type MyErr struct{ Code int }

func (e MyErr) Error() string {
	return fmt.Sprintf("failed with code %d", e.Code)
}

func main() {
	var err error = MyErr{Code: 42}
	fmt.Println(err)         // calls Error()
	fmt.Println(err.Error()) // same string
}
```

**Output:**

```
failed with code 42
failed with code 42
```

---

## 2. errors.New for a simple error

`🟢 easy` · *Creating errors*

errors.New builds a basic error value from a fixed message; return it instead of nil to signal failure.

**Steps:**

1. Return errors.New(...) on the bad path.
2. Return nil on success.

```go
package main

import (
	"errors"
	"fmt"
)

func check(n int) error {
	if n < 0 {
		return errors.New("negative not allowed")
	}
	return nil
}

func main() {
	fmt.Println(check(5))  // <nil>
	fmt.Println(check(-1)) // negative not allowed
}
```

**Output:**

```
<nil>
negative not allowed
```

---

## 3. (result, error) + early return

`🟢 easy` · *Foundations*

The universal Go pattern: return (result, error), check err != nil immediately, and bail out early on failure.

**Steps:**

1. parse returns (0, err) when Atoi fails.
2. The caller checks err first and uses the result only on success.

```go
package main

import (
	"fmt"
	"strconv"
)

func parse(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err // early return
	}
	return n * 2, nil
}

func main() {
	for _, s := range []string{"21", "oops"} {
		n, err := parse(s)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Println("ok:", n)
	}
}
```

**Output:**

```
ok: 42
error: strconv.Atoi: parsing "oops": invalid syntax
```

---

## 4. nil error means success

`🟢 easy` · *Foundations*

A nil error is the success signal; callers test err == nil (or err != nil) to decide whether to trust the result.

**Steps:**

1. Return nil on the happy path.
2. err == nil is true on success, false on failure.

```go
package main

import "fmt"

func doWork(ok bool) error {
	if !ok {
		return fmt.Errorf("work failed")
	}
	return nil
}

func main() {
	err := doWork(true)
	fmt.Println("err == nil:", err == nil) // true
	err = doWork(false)
	fmt.Println("err == nil:", err == nil) // false
}
```

**Output:**

```
err == nil: true
err == nil: false
```

---

## 5. fmt.Errorf for a formatted error

`🟢 easy` · *Creating errors*

fmt.Errorf works like Sprintf but returns an error, letting you interpolate runtime values into the message.

**Steps:**

1. Use %d (and friends) to embed values.
2. Without %w it does not wrap — it's just formatted text.

```go
package main

import "fmt"

func validatePort(n int) error {
	if n < 1 || n > 65535 {
		return fmt.Errorf("invalid port %d: must be 1-65535", n)
	}
	return nil
}

func main() {
	fmt.Println(validatePort(8080))  // <nil>
	fmt.Println(validatePort(70000)) // invalid port 70000: must be 1-65535
}
```

**Output:**

```
<nil>
invalid port 70000: must be 1-65535
```

---

## 6. Sentinel errors + errors.Is

`🟡 medium` · *Sentinel errors*

A sentinel is a package-level error value for a known condition; callers branch on it with errors.Is.

**Steps:**

1. Declare var ErrNotFound = errors.New(...).
2. Return it; the caller matches with errors.Is(err, ErrNotFound).

```go
package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func lookup(id int) error {
	if id != 1 {
		return ErrNotFound
	}
	return nil
}

func main() {
	err := lookup(99)
	if errors.Is(err, ErrNotFound) {
		fmt.Println("handled: item does not exist")
	}
}
```

**Output:**

```
handled: item does not exist
```

---

## 7. errors.New values are distinct

`🟡 medium` · *Sentinel errors*

Each errors.New call returns a unique value, so two with the same text are not ==. That's why a sentinel is stored once in a variable and reused.

**Steps:**

1. Two errors.New("oops") are not equal.
2. Reusing one variable is what makes == / errors.Is work.

```go
package main

import (
	"errors"
	"fmt"
)

func main() {
	a := errors.New("oops")
	b := errors.New("oops")
	fmt.Println("a == b:", a == b) // false: different values
	fmt.Println("a == a:", a == a) // true
}
```

**Output:**

```
a == b: false
a == a: true
```

---

## 8. Wrapping with %w

`🟡 medium` · *Wrapping*

fmt.Errorf with the %w verb creates a new error that CONTAINS another, adding context while preserving the original.

**Steps:**

1. Wrap a cause with fmt.Errorf("...: %w", cause).
2. The message reads as context + cause.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func getUser(id int) error {
	return fmt.Errorf("getUser %d: %w", id, ErrNotFound)
}

func main() {
	err := getUser(7)
	fmt.Println(err) // getUser 7: not found
}
```

**Output:**

```
getUser 7: not found
```

---

## 9. errors.Is sees through a wrap

`🟡 medium` · *Wrapping*

errors.Is walks the wrap chain, so it still matches a sentinel even after it's been wrapped with %w.

**Steps:**

1. Wrap ErrNotFound with context.
2. errors.Is(err, ErrNotFound) is still true.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func getUser(id int) error {
	return fmt.Errorf("getUser %d: %w", id, ErrNotFound)
}

func main() {
	err := getUser(7)
	fmt.Println("message:", err)
	fmt.Println("is ErrNotFound?", errors.Is(err, ErrNotFound)) // true
}
```

**Output:**

```
message: getUser 7: not found
is ErrNotFound? true
```

---

## 10. errors.Unwrap

`🟡 medium` · *Wrapping*

errors.Unwrap returns the next error in the chain (or nil if there isn't one).

**Steps:**

1. Unwrap a wrapped error to get the cause.
2. Unwrapping a non-wrapping error returns nil.

```go
package main

import (
	"errors"
	"fmt"
)

func main() {
	base := errors.New("disk full")
	wrapped := fmt.Errorf("save failed: %w", base)
	fmt.Println(wrapped)                    // save failed: disk full
	fmt.Println(errors.Unwrap(wrapped))     // disk full
	fmt.Println(errors.Unwrap(base) == nil) // true: nothing under base
}
```

**Output:**

```
save failed: disk full
disk full
true
```

---

## 11. %w vs %v

`🟡 medium` · *Wrapping*

%w wraps (the cause stays inspectable); %v only formats the text (the chain is lost). errors.Is can find the cause only through %w.

**Steps:**

1. Build the same error with %w and with %v.
2. errors.Is succeeds for %w, fails for %v.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrBase = errors.New("base")

func main() {
	withW := fmt.Errorf("ctx: %w", ErrBase) // wraps
	withV := fmt.Errorf("ctx: %v", ErrBase) // just formats

	fmt.Println("wrapped with w  -> Is matches:", errors.Is(withW, ErrBase))  // true
	fmt.Println("formatted with v -> Is matches:", errors.Is(withV, ErrBase)) // false
}
```

**Output:**

```
wrapped with w  -> Is matches: true
formatted with v -> Is matches: false
```

---

## 12. Multi-layer wrapping builds a trace

`🟡 medium` · *Wrapping*

Wrapping with one short phrase at each layer produces a readable cause chain without stack-trace machinery.

**Steps:**

1. Each layer wraps the one below with context.
2. errors.Is still finds the root cause through all layers.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrConnRefused = errors.New("connection refused")

func queryDB() error { return ErrConnRefused }

func loadUser() error {
	if err := queryDB(); err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	return nil
}

func handler() error {
	if err := loadUser(); err != nil {
		return fmt.Errorf("loading user: %w", err)
	}
	return nil
}

func main() {
	err := handler()
	fmt.Println(err)                            // loading user: opening db: connection refused
	fmt.Println(errors.Is(err, ErrConnRefused)) // true
}
```

**Output:**

```
loading user: opening db: connection refused
true
```

---

## 13. Custom error types

`🟡 medium` · *Custom error types*

When callers need structured data, implement error on a struct so it carries fields alongside the message.

**Steps:**

1. ValidationError holds Field and Msg.
2. Its Error() composes them into a message.

```go
package main

import "fmt"

type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Msg)
}

func validate(age int) error {
	if age < 0 {
		return &ValidationError{Field: "age", Msg: "must be non-negative"}
	}
	return nil
}

func main() {
	err := validate(-1)
	fmt.Println(err) // age: must be non-negative
}
```

**Output:**

```
age: must be non-negative
```

---

## 14. errors.As extracts a custom type

`🟡 medium` · *Custom error types*

errors.As finds an error of a specific type in the chain and assigns it to your variable, so you can read its fields.

**Steps:**

1. Declare var ve *ValidationError.
2. errors.As(err, &ve) fills it; then read ve.Field.

```go
package main

import (
	"errors"
	"fmt"
)

type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

func validate(age int) error {
	if age < 0 {
		return &ValidationError{Field: "age", Msg: "must be non-negative"}
	}
	return nil
}

func main() {
	err := validate(-1)
	var ve *ValidationError
	if errors.As(err, &ve) {
		fmt.Println("field:", ve.Field)
		fmt.Println("msg:", ve.Msg)
	}
}
```

**Output:**

```
field: age
msg: must be non-negative
```

---

## 15. errors.As with a standard library error

`🟡 medium` · *Inspecting*

Standard library errors are inspectable too: os.Open returns a *fs.PathError you can extract with errors.As.

**Steps:**

1. Open a missing file to get a *fs.PathError.
2. errors.As recovers it; read its Op and Path.

```go
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

func main() {
	_, err := os.Open("/no/such/file/here")
	var perr *fs.PathError
	if errors.As(err, &perr) {
		fmt.Println("op:", perr.Op)     // open
		fmt.Println("path:", perr.Path) // /no/such/file/here
	}
}
```

**Output:**

```
op: open
path: /no/such/file/here
```

---

## 16. Error message style and composition

`🟡 medium` · *Idioms*

Idiomatic messages are lowercase with no trailing punctuation, so they read well when wrapped and composed.

**Steps:**

1. Keep each layer a short lowercase phrase.
2. Composed with %w it reads like a path: outer: inner.

```go
package main

import "fmt"

func main() {
	name := "config.json"
	inner := fmt.Errorf("permission denied")
	err := fmt.Errorf("open %s: %w", name, inner)
	fmt.Println(err) // open config.json: permission denied
}
```

**Output:**

```
open config.json: permission denied
```

---

## 17. errors.Join combines multiple errors

`🔴 hard` · *Aggregating*

errors.Join (Go 1.20+) bundles several errors into one; its message lists each on its own line.

**Steps:**

1. Join two independent errors.
2. Printing shows both, one per line.

```go
package main

import (
	"errors"
	"fmt"
)

func main() {
	err1 := errors.New("disk full")
	err2 := errors.New("network down")
	joined := errors.Join(err1, err2)
	fmt.Println(joined)
}
```

**Output:**

```
disk full
network down
```

---

## 18. errors.Is on a joined error

`🔴 hard` · *Aggregating*

errors.Is checks every branch of a joined error, so it matches any of the combined causes.

**Steps:**

1. Join ErrA and ErrB.
2. errors.Is finds both.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrA = errors.New("A")
var ErrB = errors.New("B")

func main() {
	joined := errors.Join(ErrA, ErrB)
	fmt.Println("has A?", errors.Is(joined, ErrA)) // true
	fmt.Println("has B?", errors.Is(joined, ErrB)) // true
}
```

**Output:**

```
has A? true
has B? true
```

---

## 19. errors.As through a wrap chain

`🔴 hard` · *Inspecting*

errors.As searches the whole chain, so it finds a custom type even when it's wrapped several layers deep.

**Steps:**

1. Wrap a *NotFoundError twice with context.
2. errors.As still extracts it and reads Key.

```go
package main

import (
	"errors"
	"fmt"
)

type NotFoundError struct{ Key string }

func (e *NotFoundError) Error() string { return "not found: " + e.Key }

func main() {
	base := &NotFoundError{Key: "user:7"}
	wrapped := fmt.Errorf("handler: %w", fmt.Errorf("service: %w", base))

	var nf *NotFoundError
	if errors.As(wrapped, &nf) {
		fmt.Println("found deep in chain, key:", nf.Key)
	}
	fmt.Println(wrapped)
}
```

**Output:**

```
found deep in chain, key: user:7
handler: service: not found: user:7
```

---

## 20. Sentinel 'not found' repository pattern

`🔴 hard` · *Sentinel errors*

Export a sentinel and wrap it with context at the call site; callers add context yet still branch with errors.Is.

**Steps:**

1. Get wraps ErrNotFound with the id.
2. The caller uses errors.Is to detect the 'not found' case.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

type Repo struct{ data map[int]string }

func (r Repo) Get(id int) (string, error) {
	v, ok := r.data[id]
	if !ok {
		return "", fmt.Errorf("get id %d: %w", id, ErrNotFound)
	}
	return v, nil
}

func main() {
	r := Repo{data: map[int]string{1: "alice"}}
	_, err := r.Get(99)
	if errors.Is(err, ErrNotFound) {
		fmt.Println("404:", err)
	}
	v, _ := r.Get(1)
	fmt.Println("found:", v)
}
```

**Output:**

```
404: get id 99: not found
found: alice
```

---

## 21. Custom error carrying structured data

`🔴 hard` · *Custom error types*

A custom error type can carry data callers act on — e.g. an HTTP status — recovered later with errors.As even after wrapping.

**Steps:**

1. StatusError holds a Code.
2. errors.As pulls it out so you know which status to return.

```go
package main

import (
	"errors"
	"fmt"
)

type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string { return fmt.Sprintf("status %d: %s", e.Code, e.Msg) }

func fetch() error {
	return fmt.Errorf("fetch: %w", &StatusError{Code: 404, Msg: "missing"})
}

func main() {
	err := fetch()
	var se *StatusError
	if errors.As(err, &se) {
		fmt.Println("HTTP code to return:", se.Code)
	}
}
```

**Output:**

```
HTTP code to return: 404
```

---

## 22. == misses a wrapped cause

`🔴 hard` · *Inspecting*

Comparing a wrapped error to a sentinel with == fails (it's a different value); always use errors.Is for wrapped errors.

**Steps:**

1. wrapped == ErrNotFound is false.
2. errors.Is(wrapped, ErrNotFound) is true.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func main() {
	wrapped := fmt.Errorf("ctx: %w", ErrNotFound)
	fmt.Println("== comparison:", wrapped == ErrNotFound)          // false!
	fmt.Println("errors.Is:    ", errors.Is(wrapped, ErrNotFound)) // true
}
```

**Output:**

```
== comparison: false
errors.Is:     true
```

---

## 23. Custom error with an Unwrap method

`🔴 hard` · *Wrapping*

Implement Unwrap() error on your type so errors.Is/As can traverse into the cause it holds.

**Steps:**

1. OpError stores an inner Err and exposes Unwrap.
2. errors.Is then finds the wrapped sentinel.

```go
package main

import (
	"errors"
	"fmt"
)

var ErrBase = errors.New("base failure")

type OpError struct {
	Op  string
	Err error
}

func (e *OpError) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *OpError) Unwrap() error { return e.Err } // lets Is/As traverse

func main() {
	err := &OpError{Op: "save", Err: ErrBase}
	fmt.Println(err)
	fmt.Println("is base?", errors.Is(err, ErrBase)) // true via Unwrap
}
```

**Output:**

```
save: base failure
is base? true
```

---

## 24. A custom Is method

`🔴 hard` · *Inspecting*

A type can define Is(target error) bool to control how errors.Is matches it — e.g. matching a whole category by Kind.

**Steps:**

1. KindError.Is matches any KindError with the same Kind.
2. errors.Is uses it, even through a wrap.

```go
package main

import (
	"errors"
	"fmt"
)

type KindError struct{ Kind string }

func (e *KindError) Error() string { return "kind: " + e.Kind }
func (e *KindError) Is(target error) bool {
	t, ok := target.(*KindError)
	return ok && t.Kind == e.Kind
}

func main() {
	err := fmt.Errorf("wrapped: %w", &KindError{Kind: "timeout"})
	fmt.Println(errors.Is(err, &KindError{Kind: "timeout"})) // true
	fmt.Println(errors.Is(err, &KindError{Kind: "auth"}))    // false
}
```

**Output:**

```
true
false
```

---

## 25. panic for an impossible state

`🔴 hard` · *panic vs error*

Use error for expected failures; use panic for programmer bugs / 'this can't happen' states.

**Steps:**

1. A default case that should be unreachable panics.
2. A deferred recover here only demonstrates the panic; real code would fix the bug.

```go
package main

import "fmt"

func mustColor(code int) string {
	switch code {
	case 0:
		return "red"
	case 1:
		return "green"
	default:
		panic(fmt.Sprintf("unreachable: invalid color code %d", code))
	}
}

func main() {
	fmt.Println(mustColor(1))
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered from bug:", r)
		}
	}()
	fmt.Println(mustColor(99))
}
```

**Output:**

```
green
recovered from bug: unreachable: invalid color code 99
```

---

## 26. recover turns a panic into an error

`🔴 hard` · *panic vs error*

A deferred function with recover can convert a panic into a returned error via a named return value.

**Steps:**

1. safeRun defers a recover that sets err.
2. A panicking function becomes a normal error; a clean run returns nil.

```go
package main

import "fmt"

func safeRun(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()
	fn()
	return nil
}

func main() {
	err := safeRun(func() { panic("boom") })
	fmt.Println(err)
	err = safeRun(func() { fmt.Println("ran fine") })
	fmt.Println("err:", err)
}
```

**Output:**

```
panic recovered: boom
ran fine
err: <nil>
```

---

## 27. recover at a boundary (middleware-style)

`🔴 hard` · *panic vs error*

recover belongs at process boundaries — e.g. an HTTP middleware that converts a handler panic into a 500 instead of crashing.

**Steps:**

1. withRecovery wraps a handler call.
2. A panicking handler yields status 500 instead of bringing the program down.

```go
package main

import "fmt"

func withRecovery(handler func() string) (status int, body string) {
	defer func() {
		if r := recover(); r != nil {
			status = 500
			body = fmt.Sprintf("internal error: %v", r)
		}
	}()
	return 200, handler()
}

func main() {
	s, b := withRecovery(func() string { return "ok" })
	fmt.Println(s, b)
	s, b = withRecovery(func() string { panic("handler blew up") })
	fmt.Println(s, b)
}
```

**Output:**

```
200 ok
500 internal error: handler blew up
```

---

## 28. Don't ignore errors

`🔴 hard` · *Idioms*

Discarding an error with _ hides failures and leaves you with a misleading zero value; handle, wrap-and-return, or log it.

**Steps:**

1. The ignored Atoi error leaves n as a silently-wrong 0.
2. Handling the error surfaces the real problem.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	// BAD: ignoring the error hides the failure.
	n, _ := strconv.Atoi("not-a-number")
	fmt.Println("ignored -> zero value:", n) // 0, silently wrong

	// GOOD: handle it.
	if n, err := strconv.Atoi("not-a-number"); err != nil {
		fmt.Println("handled:", err)
	} else {
		fmt.Println("value:", n)
	}
}
```

**Output:**

```
ignored -> zero value: 0
handled: strconv.Atoi: parsing "not-a-number": invalid syntax
```

---

