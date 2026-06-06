# Step 12 — Errors & Error Handling · 🟡 Medium

Examples **6–16**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
