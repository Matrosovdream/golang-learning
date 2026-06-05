# Step 04 — Variables, Types & Constants · Examples

A library of **16 runnable examples**. Each is a complete `package main` program:
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

- [1. The four ways to declare a variable](#1-the-four-ways-to-declare-a-variable)
- [2. Zero values of basic types](#2-zero-values-of-basic-types)
- [3. Basic types and %T](#3-basic-types-and-t)
- [4. Constants: single and grouped](#4-constants-single-and-grouped)
- [5. iota basics](#5-iota-basics)

**Medium**

- [6. Signed integer overflow wraps around](#6-signed-integer-overflow-wraps-around)
- [7. Unsigned integers wrap (underflow)](#7-unsigned-integers-wrap-underflow)
- [8. No implicit conversion; int/float truncates](#8-no-implicit-conversion-intfloat-truncates)
- [9. string, []byte, and []rune](#9-string-byte-and-rune)
- [10. Integer literal bases](#10-integer-literal-bases)
- [11. Untyped vs typed constants](#11-untyped-vs-typed-constants)
- [13. string(rune) vs strconv.Itoa](#13-stringrune-vs-strconvitoa)

**Hard**

- [12. Named types vs type aliases](#12-named-types-vs-type-aliases)
- [14. iota bit-shift size constants](#14-iota-bit-shift-size-constants)
- [15. Enum with iota + Stringer](#15-enum-with-iota--stringer)
- [16. iota bitmask flags](#16-iota-bitmask-flags)

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

## 6. Signed integer overflow wraps around

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

## 7. Unsigned integers wrap (underflow)

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

## 8. No implicit conversion; int/float truncates

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

## 9. string, []byte, and []rune

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

## 10. Integer literal bases

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

## 11. Untyped vs typed constants

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

## 12. Named types vs type aliases

`🔴 hard` · *Named types*

type Celsius float64 creates a NEW distinct type (needs conversion to mix with float64); type X = float64 is just an alias and is identical to float64.

**Steps:**

1. %T shows Celsius as main.Celsius but the alias as float64.
2. Assigning a Celsius to a float64 needs an explicit conversion.

```go
package main

import "fmt"

type Celsius float64   // new, distinct type
type MyAlias = float64 // alias: identical to float64

func main() {
	var c Celsius = 100
	var a MyAlias = 100

	fmt.Printf("%T\n", c) // main.Celsius
	fmt.Printf("%T\n", a) // float64

	// var x float64 = c       // compile error: cannot use c (Celsius) as float64
	var x float64 = float64(c) // explicit conversion required
	fmt.Println(x)
}
```

**Output:**

```
main.Celsius
float64
100
```

---

## 13. string(rune) vs strconv.Itoa

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

## 14. iota bit-shift size constants

`🔴 hard` · *iota*

Combining iota with a bit shift builds KB/MB/GB cleanly: the single expression repeats each line while iota advances.

**Steps:**

1. Skip iota 0 with _, then use 1 << (10 * iota).
2. KB, MB, GB become powers of 1024.

```go
package main

import "fmt"

const (
	_  = iota             // skip 0
	KB = 1 << (10 * iota) // 1 << 10
	MB                    // 1 << 20
	GB                    // 1 << 30
)

func main() {
	fmt.Println(KB, MB, GB)
}
```

**Output:**

```
1024 1048576 1073741824
```

---

## 15. Enum with iota + Stringer

`🔴 hard` · *iota*

The idiomatic Go enum: a named integer type, values via iota, and a String() method so fmt prints names instead of numbers.

**Steps:**

1. Define type Weekday int and number the days with iota.
2. String() maps the value to a name, so Println shows words.

```go
package main

import "fmt"

type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

func (d Weekday) String() string {
	names := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	return names[d]
}

func main() {
	fmt.Println(Sunday, Wednesday, Saturday)
}
```

**Output:**

```
Sunday Wednesday Saturday
```

---

## 16. iota bitmask flags

`🔴 hard` · *iota*

Shifting 1 by iota gives independent bit flags (1, 2, 4) you can combine with | and test with &.

**Steps:**

1. Read/Write/Execute become 1, 2, 4 via 1 << iota.
2. Combine with | and test membership with &.

```go
package main

import "fmt"

type Perm uint8

const (
	Read    Perm = 1 << iota // 1
	Write                    // 2
	Execute                  // 4
)

func main() {
	p := Read | Write
	fmt.Printf("perm = %03b\n", p)
	fmt.Println("can read:", p&Read != 0)
	fmt.Println("can exec:", p&Execute != 0)
}
```

**Output:**

```
perm = 011
can read: true
can exec: false
```

---

