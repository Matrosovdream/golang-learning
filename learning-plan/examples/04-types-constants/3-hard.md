# Step 04 — Variables, Types & Constants · 🔴 Hard

Examples **23–28**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 23. Named types vs type aliases

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

## 24. iota bit-shift size constants

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

## 25. Enum with iota + Stringer

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

## 26. iota bitmask flags

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

## 27. Converting between named types

`🔴 hard` · *Named types*

`Celsius` and `Fahrenheit` are distinct types that both wrap `float64`. The compiler won't let you mix them, so you write explicit conversion logic — exactly the type safety named types buy you.

**Steps:**

1. `type Celsius float64` and `type Fahrenheit float64` are two different types.
2. A `ToF` method does the math and returns the right type; assigning a `Celsius` to a `Fahrenheit` directly (commented) won't compile.

```go
package main

import "fmt"

// Two distinct named types that share the underlying float64.
type Celsius float64
type Fahrenheit float64

// Conversion functions make the relationship explicit and type-safe.
func (c Celsius) ToF() Fahrenheit {
	return Fahrenheit(c*9/5 + 32)
}

func main() {
	var boiling Celsius = 100
	f := boiling.ToF()

	fmt.Printf("%g°C = %g°F\n", float64(boiling), float64(f))
	fmt.Printf("%T and %T are different types\n", boiling, f)

	// The compiler forbids mixing them directly:
	// var wrong Fahrenheit = boiling // error: cannot use boiling (Celsius)
}
```

**Output:**

```
100°C = 212°F
main.Celsius and main.Fahrenheit are different types
```

---

## 28. Untyped constants have arbitrary precision

`🔴 hard` · *Constants*

An untyped constant has no fixed type and very high precision until it's *used*. The same constant can flow into a `float64` or an `int64` with no conversion, adapting to each context.

**Steps:**

1. `1 << 40` is far larger than an `int32` can hold, yet the untyped constant holds it fine.
2. Assign it to both a `float64` and an `int64`; constant arithmetic like `1.0/3.0` is exact at compile time.

```go
package main

import "fmt"

// Untyped constants have arbitrary precision and no fixed type until used.
// 1 << 40 is far bigger than an int32 can hold, yet it's fine here.
const big = 1 << 40

func main() {
	// The same untyped constant adapts to whichever type the context needs.
	var asFloat float64 = big
	var asInt64 int64 = big

	fmt.Println("as float64:", asFloat)
	fmt.Println("as int64: ", asInt64)

	// Untyped const arithmetic is exact at compile time:
	const third = 1.0 / 3.0
	fmt.Printf("%.10f\n", third)
}
```

**Output:**

```
as float64: 1.099511627776e+12
as int64:  1099511627776
0.3333333333
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
