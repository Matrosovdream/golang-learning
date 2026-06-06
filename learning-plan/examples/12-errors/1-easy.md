# Step 12 — Errors & Error Handling · 🟢 Easy

Examples **1–5**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
