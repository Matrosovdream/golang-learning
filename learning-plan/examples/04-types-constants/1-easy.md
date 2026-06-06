# Step 04 — Variables, Types & Constants · 🟢 Easy

Examples **1–9**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. The four ways to declare a variable

`🟢 easy` · *Variables*

Go gives you four declaration forms; all produce the same value, and an uninitialized var gets its type's zero value.

**Steps:**

1. `var a int` declares with the zero value (0). `var b int = 7` gives type and value. `var c = 7` infers the type. `d := 7` is the short form (only inside functions).
2. Run it: every variable here holds 7 except a, which is the int zero value 0.

```go
package main

import "fmt"

func main() {
	var a int     // zero value (0)
	var b int = 7 // explicit type + value
	var c = 7     // type inferred from value
	d := 7        // short form — only inside functions

	fmt.Println(a, b, c, d)
}
```

**Output:**

```
0 7 7 7
```

---

## 2. Zero values of basic types

`🟢 easy` · *Variables*

Every variable declared without an initializer gets a well-defined zero value — Go has no uninitialized memory.

**Steps:**

1. Declare one var of several basic types with no value.
2. Numbers are 0, bool is false, string is "" (empty), and a pointer is nil.

```go
package main

import "fmt"

func main() {
	var (
		i int
		f float64
		b bool
		s string
		p *int
	)
	fmt.Printf("int=%d float=%g bool=%t string=%q ptr=%v\n", i, f, b, s, p)
}
```

**Output:**

```
int=0 float=0 bool=false string="" ptr=<nil>
```

---

## 3. Basic types and %T

`🟢 easy` · *Types*

%T prints a value's type. byte is an alias for uint8 and rune is an alias for int32, which %T reveals.

**Steps:**

1. Declare an int, float64, byte, rune, and string.
2. Print each type with %T; note byte shows as uint8 and rune as int32.

```go
package main

import "fmt"

func main() {
	var i int = 42
	var f float64 = 3.14
	var b byte = 'A'
	var r rune = 'é'
	var s string = "hi"

	fmt.Printf("%T %T %T %T %T\n", i, f, b, r, s)
	fmt.Printf("byte=%d  rune=%d (%c)\n", b, r, r)
}
```

**Output:**

```
int float64 uint8 int32 string
byte=65  rune=233 (é)
```

---

## 4. Constants: single and grouped

`🟢 easy` · *Constants*

const defines compile-time values that cannot be reassigned; a const ( ... ) block declares several at once.

**Steps:**

1. Declare one const and a const block.
2. The commented line shows reassigning a const is a compile error.

```go
package main

import "fmt"

const Pi = 3.14159

const (
	A = 1
	B = 2
	C = 3
)

func main() {
	fmt.Println(Pi, A, B, C)
	// Pi = 4 // compile error: cannot assign to Pi (declared const)
}
```

**Output:**

```
3.14159 1 2 3
```

---

## 5. iota basics

`🟢 easy` · *iota*

iota is a counter that starts at 0 in each const block and increments by one per line — the idiomatic way to number constants.

**Steps:**

1. In a const block, set the first to iota; leave the rest blank to repeat the expression.
2. Red, Green, Blue become 0, 1, 2.

```go
package main

import "fmt"

const (
	Red   = iota // 0
	Green        // 1
	Blue         // 2
)

func main() {
	fmt.Println(Red, Green, Blue)
}
```

**Output:**

```
0 1 2
```

---

## 6. Integer division and remainder

`🟢 easy` · *Operators*

When both operands are integers, `/` truncates toward zero and `%` gives the remainder — there is no automatic fraction. Convert to a float first if you want one.

**Steps:**

1. `5 / 2` is `2`, not `2.5`; `5 % 2` is the leftover `1`.
2. `float64(a) / float64(b)` converts first, so you get `2.5`.

```go
package main

import "fmt"

func main() {
	a, b := 5, 2
	fmt.Println("5 / 2 =", a/b) // integer division truncates toward zero
	fmt.Println("5 % 2 =", a%b) // remainder (modulo)

	// To get a real fraction, convert to float first:
	fmt.Println("float:", float64(a)/float64(b))
}
```

**Output:**

```
5 / 2 = 2
5 % 2 = 1
float: 2.5
```

---

## 7. Multiple assignment and the swap idiom

`🟢 easy` · *Variables*

Go evaluates the entire right-hand side before assigning, so you can assign several variables on one line and swap two without a temporary.

**Steps:**

1. `x, y, z := 1, 2, 3` declares three at once.
2. `x, z = z, x` swaps them — no temp variable needed.

```go
package main

import "fmt"

func main() {
	// Declare and assign several variables at once.
	x, y, z := 1, 2, 3
	fmt.Println(x, y, z)

	// Parallel assignment: the right side is fully evaluated first,
	// so swapping needs no temporary variable.
	x, z = z, x
	fmt.Println("after swap:", x, y, z)
}
```

**Output:**

```
1 2 3
after swap: 3 2 1
```

---

## 8. The blank identifier

`🟢 easy` · *Variables*

`_` is the blank identifier: it lets you discard a value you don't want. Go treats an unused ordinary variable as a compile error, so `_` is how you ignore one of several return values.

**Steps:**

1. `minmax` returns two values; we want only the second.
2. `_, hi := minmax(8, 3)` throws away the first.

```go
package main

import "fmt"

// minmax returns both the smaller and larger of two ints.
func minmax(a, b int) (int, int) {
	if a < b {
		return a, b
	}
	return b, a
}

func main() {
	// The blank identifier _ discards a value you don't need.
	// Here we want only the larger result.
	_, hi := minmax(8, 3)
	fmt.Println("max is", hi)

	// Without _, an unused variable would be a COMPILE error:
	// lo, hi := minmax(8, 3) // error: lo declared and not used
}
```

**Output:**

```
max is 8
```

---

## 9. bool: no truthy or falsy

`🟢 easy` · *Types*

Unlike JS/Python, Go has no truthy/falsy: a condition must be an actual `bool`. Comparisons produce `bool` values you can store, and `&&`/`||` short-circuit.

**Steps:**

1. `if ready` works because `ready` is a `bool`; `if 1` would not compile.
2. `count == 0` is a `bool` you can assign to a variable and print.

```go
package main

import "fmt"

func main() {
	// Go has no "truthy/falsy": only a real bool works in a condition.
	// `if 1 { ... }` would be a compile error.
	ready := true
	count := 0

	if ready {
		fmt.Println("ready")
	}

	// Comparisons produce a bool value you can store and print.
	isEmpty := count == 0
	fmt.Println("isEmpty:", isEmpty)

	// && and || short-circuit: the right side runs only if needed.
	fmt.Println(ready && count == 0)
	fmt.Println(ready || count == 0)
}
```

**Output:**

```
ready
isEmpty: true
true
true
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
