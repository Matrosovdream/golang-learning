# Step 06 — Functions · 🟢 Easy

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

## 1. Declaring and calling a function

`🟢 easy` · *Basics*

A function takes typed parameters and declares its return type after the parameter list.

**Steps:**

1. func add(a int, b int) int declares two int params and an int result.
2. Call it like add(2, 3).

```go
package main

import "fmt"

func add(a int, b int) int {
	return a + b
}

func main() {
	fmt.Println(add(2, 3))
}
```

**Output:**

```
5
```

---

## 2. Same-type parameter shorthand

`🟢 easy` · *Basics*

When consecutive parameters share a type, you write the type once after the last of them.

**Steps:**

1. func volume(l, w, h int) is shorthand for three ints.
2. Identical to writing l int, w int, h int.

```go
package main

import "fmt"

func volume(l, w, h int) int {
	return l * w * h
}

func main() {
	fmt.Println(volume(2, 3, 4))
}
```

**Output:**

```
24
```

---

## 3. Multiple return values

`🟢 easy` · *Returns*

Go functions can return more than one value; the caller receives them with multiple assignment.

**Steps:**

1. minmax returns two ints (smaller, larger).
2. Capture both with lo, hi := minmax(...).

```go
package main

import "fmt"

func minmax(a, b int) (int, int) {
	if a < b {
		return a, b
	}
	return b, a
}

func main() {
	lo, hi := minmax(8, 3)
	fmt.Println("lo:", lo, "hi:", hi)
}
```

**Output:**

```
lo: 3 hi: 8
```

---

## 4. The (value, error) idiom

`🟢 easy` · *Returns*

Go's universal failure convention: return the result plus an error as the last value; nil error means success.

**Steps:**

1. safeDiv returns (0, error) on divide-by-zero, else (quotient, nil).
2. Callers check the error before trusting the result.

```go
package main

import (
	"errors"
	"fmt"
)

func safeDiv(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a / b, nil
}

func main() {
	if q, err := safeDiv(10, 2); err == nil {
		fmt.Println("10/2 =", q)
	}
	if _, err := safeDiv(1, 0); err != nil {
		fmt.Println("error:", err)
	}
}
```

**Output:**

```
10/2 = 5
error: divide by zero
```

---

## 5. Named return values + naked return

`🟢 easy` · *Returns*

You can name the return values in the signature; a bare return then returns their current values. Use sparingly — it can hurt readability in long functions.

**Steps:**

1. (q, r int) declares the results up front.
2. A naked return sends back whatever q and r currently hold.

```go
package main

import "fmt"

func divmod(a, b int) (q, r int) {
	q = a / b
	r = a % b
	return // naked return: returns q, r
}

func main() {
	q, r := divmod(17, 5)
	fmt.Println("quotient:", q, "remainder:", r)
}
```

**Output:**

```
quotient: 3 remainder: 2
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
