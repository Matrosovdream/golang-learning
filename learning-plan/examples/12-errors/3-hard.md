# Step 12 — Errors & Error Handling · 🔴 Hard

Examples **17–28**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
