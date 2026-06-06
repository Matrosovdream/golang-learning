# Step 05 — Control Flow · 🟡 Medium

Examples **7–14**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 7. continue skips an iteration

`🟡 medium` · *Loops*

continue jumps to the next iteration without running the rest of the loop body.

**Steps:**

1. Skip even numbers with continue.
2. Only odd values reach the Println.

```go
package main

import "fmt"

func main() {
	for i := 1; i <= 6; i++ {
		if i%2 == 0 {
			continue // skip evens
		}
		fmt.Println("odd:", i)
	}
}
```

**Output:**

```
odd: 1
odd: 3
odd: 5
```

---

## 8. range over a map (sort for order)

`🟡 medium` · *Loops*

Map iteration order is RANDOMIZED by Go on purpose, so collect and sort the keys when you need stable output.

**Steps:**

1. Collect keys into a slice and sort.Strings them.
2. Iterate the sorted keys to print deterministically.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	ages := map[string]int{"alice": 30, "bob": 25, "carol": 35}

	// Map order is random; sort keys for a stable result.
	keys := make([]string, 0, len(ages))
	for k := range ages {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s is %d\n", k, ages[k])
	}
}
```

**Output:**

```
alice is 30
bob is 25
carol is 35
```

---

## 9. range over a string decodes UTF-8

`🟡 medium` · *Loops*

Ranging a string yields the BYTE INDEX and the decoded rune; the index jumps by more than one across multibyte characters.

**Steps:**

1. é is two bytes, so the index goes 0,1,3,4,5 — skipping 2.
2. The value is a rune (code point), not a byte.

```go
package main

import "fmt"

func main() {
	for i, r := range "héllo" {
		fmt.Printf("index %d: %c (%d)\n", i, r, r)
	}
}
```

**Output:**

```
index 0: h (104)
index 1: é (233)
index 3: l (108)
index 4: l (108)
index 5: o (111)
```

---

## 10. range over an integer (Go 1.22+)

`🟡 medium` · *Loops*

Since Go 1.22 you can range over an int n to iterate 0..n-1 with no slice needed.

**Steps:**

1. for i := range 5 loops i from 0 to 4.
2. Handy for 'do something n times'.

```go
package main

import "fmt"

func main() {
	// Go 1.22+: range over an integer.
	for i := range 5 {
		fmt.Println("i =", i)
	}
}
```

**Output:**

```
i = 0
i = 1
i = 2
i = 3
i = 4
```

---

## 11. switch on a value

`🟡 medium` · *Switch*

An expression switch compares a value to each case; Go breaks automatically after a match (no break needed) and default catches the rest.

**Steps:**

1. Match n against 1, 2, 3, else default.
2. No fallthrough by default — exactly one branch runs.

```go
package main

import "fmt"

func describe(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	default:
		return "many"
	}
}

func main() {
	fmt.Println(describe(1), describe(2), describe(3), describe(9))
}
```

**Output:**

```
one two three many
```

---

## 12. Tagless switch (switch true)

`🟡 medium` · *Switch*

A switch with no value is switch true: each case is a boolean condition — a clean replacement for a long if/else-if chain.

**Steps:**

1. Omit the value after switch.
2. The first case whose condition is true wins.

```go
package main

import "fmt"

func classify(n int) string {
	switch { // same as: switch true
	case n < 0:
		return "negative"
	case n == 0:
		return "zero"
	default:
		return "positive"
	}
}

func main() {
	fmt.Println(classify(-5), classify(0), classify(9))
}
```

**Output:**

```
negative zero positive
```

---

## 13. Multiple values in one case

`🟡 medium` · *Switch*

A single case can list several comma-separated values that all map to the same branch.

**Steps:**

1. Group vowels in one case and whitespace in another.
2. Anything else hits default.

```go
package main

import "fmt"

func kind(r rune) string {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return "vowel"
	case ' ', '\t', '\n':
		return "space"
	default:
		return "consonant"
	}
}

func main() {
	fmt.Println(kind('e'), kind('z'), kind(' '))
}
```

**Output:**

```
vowel consonant space
```

---

## 14. switch with an init statement

`🟡 medium` · *Switch*

Like if, switch can run a short statement first; the variable is scoped to the switch.

**Steps:**

1. switch day := 3; day { ... } declares day then switches on it.
2. case 0, 6 would be the weekend; 3 falls to default.

```go
package main

import "fmt"

func main() {
	switch day := 3; day {
	case 0, 6:
		fmt.Println("weekend")
	default:
		fmt.Println("weekday", day)
	}
}
```

**Output:**

```
weekday 3
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
