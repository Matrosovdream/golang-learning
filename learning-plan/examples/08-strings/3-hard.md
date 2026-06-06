# Step 08 — Strings, Runes, Bytes & Formatting · 🔴 Hard

Examples **20–28**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 20. fmt: %v, %+v, %#v, %T

`🔴 hard` · *fmt verbs*

The core fmt verbs: %v (value), %+v (with field names), %#v (Go syntax), and %T (the type).

**Steps:**

1. Print a struct four ways.
2. Note how each verb adds more detail.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	p := Point{1, 2}
	fmt.Printf("%v\n", p)  // {1 2}
	fmt.Printf("%+v\n", p) // {X:1 Y:2}
	fmt.Printf("%#v\n", p) // main.Point{X:1, Y:2}
	fmt.Printf("%T\n", p)  // main.Point
}
```

**Output:**

```
{1 2}
{X:1 Y:2}
main.Point{X:1, Y:2}
main.Point
```

---

## 21. fmt: integer verbs

`🔴 hard` · *fmt verbs*

Integers can print in many bases and as characters: %d %b %o %x %X %c %U.

**Steps:**

1. Print 65 in decimal, binary, octal, hex.
2. %c is the character, %U the Unicode code point.

```go
package main

import "fmt"

func main() {
	n := 65
	fmt.Printf("dec=%d bin=%b oct=%o hex=%x HEX=%X\n", n, n, n, n, n)
	fmt.Printf("char=%c unicode=%U\n", n, n)
}
```

**Output:**

```
dec=65 bin=1000001 oct=101 hex=41 HEX=41
char=A unicode=U+0041
```

---

## 22. fmt: float verbs

`🔴 hard` · *fmt verbs*

Floats: %f (decimal), %.2f (fixed precision), %e (scientific), %g (compact).

**Steps:**

1. Same number, four formats.
2. %.2f rounds to two decimals.

```go
package main

import "fmt"

func main() {
	f := 1234.5678
	fmt.Printf("%f\n", f)   // 1234.567800
	fmt.Printf("%.2f\n", f) // 1234.57
	fmt.Printf("%e\n", f)   // 1.234568e+03
	fmt.Printf("%g\n", f)   // 1234.5678
}
```

**Output:**

```
1234.567800
1234.57
1.234568e+03
1234.5678
```

---

## 23. fmt: %s vs %q, and slices

`🔴 hard` · *fmt verbs*

%s prints a string raw; %q wraps it as a quoted Go literal showing escapes; %v works for composite values like slices.

**Steps:**

1. %s expands a tab; %q shows it as \t inside quotes.
2. %v prints a slice cleanly. (%p prints a pointer address, which varies per run.)

```go
package main

import "fmt"

func main() {
	s := "hi\tthere"
	fmt.Printf("%s\n", s) // raw (tab expands)
	fmt.Printf("%q\n", s) // quoted literal with escapes
	fmt.Printf("%v\n", []int{1, 2, 3})
}
```

**Output:**

```
hi	there
"hi\tthere"
[1 2 3]
```

---

## 24. fmt: width, precision, flags

`🔴 hard` · *fmt verbs*

Modifiers control layout: %-10s left-justifies, %10s right-justifies, %08d zero-pads, %+d shows the sign, %6.2f sets width+precision.

**Steps:**

1. Pipes show the field boundaries.
2. Combine width and precision like %6.2f.

```go
package main

import "fmt"

func main() {
	fmt.Printf("|%-10s|\n", "left")
	fmt.Printf("|%10s|\n", "right")
	fmt.Printf("|%08d|\n", 42)
	fmt.Printf("|%+d|\n", 42)
	fmt.Printf("|%6.2f|\n", 3.14159)
}
```

**Output:**

```
|left      |
|     right|
|00000042|
|+42|
|  3.14|
```

---

## 25. fmt: explicit argument indexes

`🔴 hard` · *fmt verbs*

%[n] selects which argument a verb consumes, letting you reuse or reorder arguments.

**Steps:**

1. %[1]d ... %[1]b reuses the same argument twice.
2. %[2]s %[1]s reorders the arguments.

```go
package main

import "fmt"

func main() {
	fmt.Printf("%[1]d in binary is %[1]b\n", 5)
	fmt.Printf("%[2]s %[1]s\n", "world", "hello")
}
```

**Output:**

```
5 in binary is 101
hello world
```

---

## 26. fmt: Sprintf and Fprintf

`🔴 hard` · *fmt verbs*

Sprintf returns a formatted string instead of printing; Fprintf writes formatted output to any io.Writer.

**Steps:**

1. Sprintf builds a string you can store.
2. Fprintf(os.Stdout, ...) writes to a chosen destination.

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	s := fmt.Sprintf("%s is %d", "age", 30)
	fmt.Println(s)
	fmt.Fprintf(os.Stdout, "written to stdout: %d\n", 7)
}
```

**Output:**

```
age is 30
written to stdout: 7
```

---

## 27. Counting runes with unicode/utf8

`🔴 hard` · *UTF-8*

The unicode/utf8 package works at the byte level: RuneCountInString counts characters and ValidString checks encoding.

**Steps:**

1. Compare len (bytes) with utf8.RuneCountInString (runes).
2. ValidString reports whether the bytes are valid UTF-8.

```go
package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	s := "héllo, 世界"
	fmt.Println("bytes:", len(s))
	fmt.Println("runes:", utf8.RuneCountInString(s))
	fmt.Println("valid UTF-8?", utf8.ValidString(s))
}
```

**Output:**

```
bytes: 14
runes: 9
valid UTF-8? true
```

---

## 28. strings.Map and NewReplacer

`🔴 hard` · *strings package*

Map rewrites each rune through a function; NewReplacer does many literal substring replacements in one efficient pass.

**Steps:**

1. Map shifts each lowercase letter by one (a Caesar-style transform).
2. NewReplacer swaps several substrings at once.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	rot := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return 'a' + (r-'a'+1)%26
		}
		return r
	}, "abz")
	fmt.Println(rot) // bca

	rep := strings.NewReplacer("cat", "dog", "fast", "slow")
	fmt.Println(rep.Replace("a fast cat"))
}
```

**Output:**

```
bca
a slow dog
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
