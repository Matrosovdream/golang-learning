# Step 04 — Variables, Types & Constants · 🟡 Medium

Examples **10–22**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 10. Signed integer overflow wraps around

`🟡 medium` · *Types*

Go integer overflow is defined: it wraps around (two's complement), it does not error. int8 maxes at 127.

**Steps:**

1. Set an int8 to its max (math.MaxInt8 = 127).
2. Incrementing wraps it to the minimum, -128.

```go
package main

import (
	"fmt"
	"math"
)

func main() {
	var x int8 = math.MaxInt8 // 127
	fmt.Println("before:", x)
	x++ // wraps around, no panic
	fmt.Println("after ++:", x)
}
```

**Output:**

```
before: 127
after ++: -128
```

---

## 11. Unsigned integers wrap (underflow)

`🟡 medium` · *Types*

Unsigned types are modular: subtracting below 0 wraps to the top, and adding past the max wraps to 0.

**Steps:**

1. uint8 0 minus 1 wraps to 255.
2. uint8 255 plus 1 wraps to 0.

```go
package main

import "fmt"

func main() {
	var u uint8 = 0
	u-- // 0 - 1 wraps to 255
	fmt.Println("0 - 1 as uint8 =", u)

	var max uint8 = 255
	max++ // 255 + 1 wraps to 0
	fmt.Println("255 + 1 as uint8 =", max)
}
```

**Output:**

```
0 - 1 as uint8 = 255
255 + 1 as uint8 = 0
```

---

## 12. No implicit conversion; int/float truncates

`🟡 medium` · *Conversions*

Go never converts numeric types automatically — you must write T(v). Converting a float to int truncates toward zero (it does not round).

**Steps:**

1. float64(i) converts explicitly; the commented line shows the implicit form fails to compile.
2. Converting a float VARIABLE to int truncates toward zero: 3.99 -> 3 and -3.99 -> -3 (note: int(3.99) on an untyped constant would be a compile error).

```go
package main

import "fmt"

func main() {
	i := 65
	var f float64 = float64(i) // explicit conversion required
	// var bad float64 = i      // compile error: cannot use i (int) as float64 value

	fmt.Println(f)

	// Converting a float VARIABLE to int truncates toward zero (no rounding).
	pos := 3.99
	neg := -3.99
	fmt.Println(int(pos)) // 3
	fmt.Println(int(neg)) // -3
}
```

**Output:**

```
65
3
-3
```

---

## 13. string, []byte, and []rune

`🟡 medium` · *Conversions*

len(string) counts bytes, but []rune counts characters. A multibyte rune like é is 2 bytes, so the two lengths differ.

**Steps:**

1. len("héllo") is 6 bytes; len([]rune(...)) is 5 runes.
2. Convert to []byte and []rune and back to a string.

```go
package main

import "fmt"

func main() {
	s := "héllo"
	fmt.Println("len bytes:", len(s))         // 6 (é is 2 bytes)
	fmt.Println("len runes:", len([]rune(s))) // 5

	b := []byte(s)
	r := []rune(s)
	fmt.Println("bytes:", b)
	fmt.Printf("rune[1]=%c\n", r[1])
	fmt.Println("rebuilt:", string(r))
}
```

**Output:**

```
len bytes: 6
len runes: 5
bytes: [104 195 169 108 108 111]
rune[1]=é
rebuilt: héllo
```

---

## 14. Integer literal bases

`🟡 medium` · *Literals*

The same integer can be written in decimal, hex (0x), octal (0o), or binary (0b), and underscores may group digits for readability.

**Steps:**

1. Write 255 four ways.
2. All four are equal.

```go
package main

import "fmt"

func main() {
	dec := 255
	hex := 0xFF
	oct := 0o377
	bin := 0b1111_1111 // underscores are ignored, just for humans

	fmt.Println(dec, hex, oct, bin)
	fmt.Println(dec == hex && hex == oct && oct == bin)
}
```

**Output:**

```
255 255 255 255
true
```

---

## 15. Untyped vs typed constants

`🟡 medium` · *Constants*

An untyped constant adapts to whatever type the context needs; a typed constant is locked to its type and needs an explicit conversion otherwise.

**Steps:**

1. The untyped const is assigned to both a float64 and an int with no conversion.
2. The commented line shows a typed int const cannot go straight into a float64.

```go
package main

import "fmt"

const untyped = 100   // untyped: flexible
const typed int = 100 // typed: int only

func main() {
	var f float64 = untyped // ok: untyped adapts to float64
	var i int = untyped     // ok: untyped adapts to int
	// var g float64 = typed // compile error: cannot use typed (int) as float64

	fmt.Println(f, i)
	fmt.Printf("%T %T\n", f, i)
}
```

**Output:**

```
100 100
float64 int
```

---

## 16. string(rune) vs strconv.Itoa

`🟡 medium` · *Conversions*

string(rune(65)) yields the CHARACTER "A" (Unicode code point 65); strconv.Itoa(65) yields the TEXT "65". Mixing these up is a classic beginner bug.

**Steps:**

1. string(rune(n)) interprets n as a code point.
2. strconv.Itoa(n) formats n as decimal digits.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	n := 65
	fmt.Println(string(rune(n))) // "A"  — code point 65
	fmt.Println(strconv.Itoa(n)) // "65" — decimal text
}
```

**Output:**

```
A
65
```

---

## 17. Variable shadowing in a nested scope

`🟡 medium` · *Variables*

`:=` inside an inner block declares a *new* variable that shadows an outer one of the same name. The outer variable is untouched — a classic source of bugs.

**Steps:**

1. The outer `n` is `10`.
2. Inside the `if`, `n := 99` creates a separate inner `n`; after the block, the outer `n` is still `10`.

```go
package main

import "fmt"

func main() {
	n := 10
	fmt.Println("outer before:", n)

	if true {
		// := inside the block creates a NEW variable that shadows the outer n.
		n := 99
		fmt.Println("inner:", n)
	}

	// The outer n was never touched.
	fmt.Println("outer after:", n)
}
```

**Output:**

```
outer before: 10
inner: 99
outer after: 10
```

---

## 18. Floating-point precision

`🟡 medium` · *Types*

Floats are binary approximations, so `0.1 + 0.2` is not exactly `0.3`. Never compare floats with `==`; compare the difference against a small tolerance instead.

**Steps:**

1. Use variables so the addition runs at runtime in `float64` (as untyped constants, the sum would fold to exactly `0.3` at compile time and hide the bug).
2. `sum == 0.3` is `false`; `%.17f` reveals the trailing error; a tolerance check passes.

```go
package main

import "fmt"

func main() {
	// Use variables so the addition happens at RUNTIME in float64.
	// (As untyped constants, 0.1 + 0.2 would be folded to exactly 0.3.)
	a, b := 0.1, 0.2
	sum := a + b

	fmt.Println(sum)           // 0.30000000000000004
	fmt.Println(sum == 0.3)    // false!
	fmt.Printf("%.17f\n", sum) // see the hidden error

	// Compare with a small tolerance instead of ==.
	const eps = 1e-9
	diff := sum - 0.3
	if diff < 0 {
		diff = -diff
	}
	fmt.Println("close enough:", diff < eps)
}
```

**Output:**

```
0.30000000000000004
false
0.30000000000000004
close enough: true
```

---

## 19. Float infinity and NaN

`🟡 medium` · *Types*

Float division by zero does not panic — it yields `±Inf`. `0.0/0.0` (and other invalid ops) yield `NaN`, which is never equal to anything, not even itself.

**Steps:**

1. Dividing `1.0` by a zero *variable* gives `+Inf` (dividing by a constant zero won't compile).
2. `math.NaN()` is not equal to itself; use `math.IsNaN` to test for it.

```go
package main

import (
	"fmt"
	"math"
)

func main() {
	// Float division by zero does NOT panic — it yields infinity.
	// (The divisor must be a variable; dividing constants by 0 won't compile.)
	zero := 0.0
	fmt.Println("1/0:", 1.0/zero) // +Inf

	inf := math.Inf(1)
	nan := math.NaN()

	fmt.Println("inf+1:", inf+1) // still +Inf
	fmt.Println("nan:", nan)

	// NaN is never equal to anything, not even itself.
	fmt.Println("nan == nan:", nan == nan)
	fmt.Println("IsNaN:", math.IsNaN(nan))
}
```

**Output:**

```
1/0: +Inf
inf+1: +Inf
nan: NaN
nan == nan: false
IsNaN: true
```

---

## 20. Rune arithmetic

`🟡 medium` · *Types*

A `rune` is just an `int32`, so characters support arithmetic. This powers tricks like converting a digit character to its value or shifting case.

**Steps:**

1. `c + 1` moves `'A'` to `'B'`.
2. `'7' - '0'` gives the numeric `7`; subtracting `'a' - 'A'` (32) uppercases a letter.

```go
package main

import "fmt"

func main() {
	// A rune is just an int32, so you can do arithmetic on characters.
	var c rune = 'A'
	fmt.Printf("%c -> %c\n", c, c+1) // 'A' -> 'B'

	// '9' - '0' converts a digit character to its numeric value.
	digit := '7' - '0'
	fmt.Println("digit value:", digit)

	// Lowercase a letter: the gap between 'a' and 'A' is 32.
	upper := 'g' - ('a' - 'A')
	fmt.Printf("%c\n", upper)
}
```

**Output:**

```
A -> B
digit value: 7
G
```

---

## 21. Typed constant overflow is a compile error

`🟡 medium` · *Constants*

A typed constant that doesn't fit its type is rejected at *compile* time — a safety net. The same overflow through a runtime conversion is silent and just wraps.

**Steps:**

1. `const maxByte int8 = 127` is fine; `128` would be a compile error (commented).
2. `int8(n)` where `n` is the variable `128` compiles and silently wraps to `-128`.

```go
package main

import "fmt"

const maxByte int8 = 127 // fits exactly

func main() {
	fmt.Println("maxByte:", maxByte)

	// A typed constant that doesn't fit is caught at COMPILE time:
	// const tooBig int8 = 128 // error: constant 128 overflows int8

	// But the SAME overflow at runtime is silent — it just wraps:
	n := 128             // an int variable
	fmt.Println(int8(n)) // -128, no error
}
```

**Output:**

```
maxByte: 127
-128
```

---

## 22. The min and max builtins

`🟡 medium` · *Builtins*

Since Go 1.21, `min` and `max` are built in — no import, variadic, and work on any ordered type. The `math` package also exposes each integer type's limits as constants.

**Steps:**

1. `min`/`max` take two or more args and return the smallest/largest.
2. `math.MaxInt64`/`math.MinInt64` are the bounds of `int64`.

```go
package main

import (
	"fmt"
	"math"
)

func main() {
	// min and max are built-in since Go 1.21 — no import, any ordered type.
	fmt.Println(min(3, 8, 1))  // 1
	fmt.Println(max(3, 8, 1))  // 8
	fmt.Println(max(2.5, 2.6)) // works on floats too

	// The math package exposes the limits of each integer type as constants.
	fmt.Println("int max:", math.MaxInt64)
	fmt.Println("int min:", math.MinInt64)
}
```

**Output:**

```
1
8
2.6
int max: 9223372036854775807
int min: -9223372036854775808
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
