# Step 05 — Control Flow · Examples

A library of **20 runnable examples**. Each is a complete `package main` program:
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

- [1. if / else if / else](#1-if--else-if--else)
- [2. if with an init statement](#2-if-with-an-init-statement)
- [3. The three-part for loop](#3-the-three-part-for-loop)
- [4. for as a while loop](#4-for-as-a-while-loop)
- [5. for range over a slice](#5-for-range-over-a-slice)
- [6. Infinite loop with break](#6-infinite-loop-with-break)

**Medium**

- [7. continue skips an iteration](#7-continue-skips-an-iteration)
- [8. range over a map (sort for order)](#8-range-over-a-map-sort-for-order)
- [9. range over a string decodes UTF-8](#9-range-over-a-string-decodes-utf-8)
- [10. range over an integer (Go 1.22+)](#10-range-over-an-integer-go-122)
- [11. switch on a value](#11-switch-on-a-value)
- [12. Tagless switch (switch true)](#12-tagless-switch-switch-true)
- [13. Multiple values in one case](#13-multiple-values-in-one-case)
- [14. switch with an init statement](#14-switch-with-an-init-statement)

**Hard**

- [15. Type switch over any](#15-type-switch-over-any)
- [16. fallthrough](#16-fallthrough)
- [17. Labeled break / continue](#17-labeled-break--continue)
- [18. defer: LIFO order + argument timing](#18-defer-lifo-order--argument-timing)
- [19. defer + recover: catch a panic](#19-defer--recover-catch-a-panic)
- [20. goto (rarely used)](#20-goto-rarely-used)

---

## 1. if / else if / else

`🟢 easy` · *Conditionals*

Go's if needs no parentheses and braces are mandatory; the early-return idiom (return inside if) keeps the happy path flat.

**Steps:**

1. Each branch returns a label for the number's sign.
2. Returning early means no else is needed for the final case.

```go
package main

import "fmt"

func classify(n int) string {
	if n < 0 {
		return "negative"
	} else if n == 0 {
		return "zero"
	}
	return "positive"
}

func main() {
	fmt.Println(classify(-3))
	fmt.Println(classify(0))
	fmt.Println(classify(7))
}
```

**Output:**

```
negative
zero
positive
```

---

## 2. if with an init statement

`🟢 easy` · *Conditionals*

if can run a short statement before its condition; variables declared there are scoped to the if/else only.

**Steps:**

1. `if n, err := ...; err == nil` parses then checks in one line.
2. n and err do not exist after the if/else block.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	if n, err := strconv.Atoi("42"); err == nil {
		fmt.Println("parsed:", n)
	} else {
		fmt.Println("bad input:", err)
	}
	// n and err are out of scope here.
}
```

**Output:**

```
parsed: 42
```

---

## 3. The three-part for loop

`🟢 easy` · *Loops*

Go has only one loop keyword, for; the classic init; condition; post form is the C-style counting loop.

**Steps:**

1. Sum 1..5 with for i := 1; i <= 5; i++.
2. There is no while keyword — for covers it.

```go
package main

import "fmt"

func main() {
	sum := 0
	for i := 1; i <= 5; i++ {
		sum += i
	}
	fmt.Println("sum 1..5 =", sum)
}
```

**Output:**

```
sum 1..5 = 15
```

---

## 4. for as a while loop

`🟢 easy` · *Loops*

Drop the init and post and for becomes a while loop: just a condition.

**Steps:**

1. Double n until it reaches 100.
2. Only the condition remains between for and {.

```go
package main

import "fmt"

func main() {
	n := 1
	for n < 100 {
		n *= 2
	}
	fmt.Println("first power of two >= 100:", n)
}
```

**Output:**

```
first power of two >= 100: 128
```

---

## 5. for range over a slice

`🟢 easy` · *Loops*

range over a slice yields the index and a COPY of each element.

**Steps:**

1. range gives i (index) and f (value).
2. Use _ for the index if you only want values.

```go
package main

import "fmt"

func main() {
	fruits := []string{"apple", "banana", "cherry"}
	for i, f := range fruits {
		fmt.Println(i, f)
	}
}
```

**Output:**

```
0 apple
1 banana
2 cherry
```

---

## 6. Infinite loop with break

`🟢 easy` · *Loops*

A bare for {} loops forever; break exits it.

**Steps:**

1. Loop with no condition and break when i hits 3.
2. break leaves the loop immediately.

```go
package main

import "fmt"

func main() {
	i := 0
	for {
		if i == 3 {
			break
		}
		fmt.Println("tick", i)
		i++
	}
	fmt.Println("done")
}
```

**Output:**

```
tick 0
tick 1
tick 2
done
```

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

## 15. Type switch over any

`🔴 hard` · *Switch*

A type switch branches on the dynamic type inside an interface value; v := x.(type) binds v to the matched concrete type in each case.

**Steps:**

1. switch x := v.(type) inspects what concrete type v holds.
2. Each case gives x that case's type; default prints %T.

```go
package main

import "fmt"

func describe(v any) string {
	switch x := v.(type) {
	case int:
		return fmt.Sprintf("int %d", x)
	case string:
		return fmt.Sprintf("string %q", x)
	case bool:
		return fmt.Sprintf("bool %t", x)
	default:
		return fmt.Sprintf("other %T", x)
	}
}

func main() {
	for _, v := range []any{42, "hi", true, 3.14} {
		fmt.Println(describe(v))
	}
}
```

**Output:**

```
int 42
string "hi"
bool true
other float64
```

---

## 16. fallthrough

`🔴 hard` · *Switch*

Go switches do NOT fall through by default; the explicit fallthrough keyword forces the very next case body to run too.

**Steps:**

1. case 1 prints then falls through into case 2.
2. It only cascades one level — case 2 has no fallthrough, so it stops.

```go
package main

import "fmt"

func main() {
	switch n := 1; n {
	case 1:
		fmt.Println("one")
		fallthrough
	case 2:
		fmt.Println("two")
	case 3:
		fmt.Println("three")
	}
}
```

**Output:**

```
one
two
```

---

## 17. Labeled break / continue

`🔴 hard` · *Loops*

A label on a loop lets break/continue target an OUTER loop instead of the innermost one.

**Steps:**

1. Label the outer loop `outer:`.
2. break outer exits both loops at once when i+j == 3.

```go
package main

import "fmt"

func main() {
outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i+j == 3 {
				fmt.Println("breaking at", i, j)
				break outer // exits the OUTER loop
			}
			fmt.Println(i, j)
		}
	}
	fmt.Println("done")
}
```

**Output:**

```
0 0
0 1
0 2
1 0
1 1
breaking at 1 2
done
```

---

## 18. defer: LIFO order + argument timing

`🔴 hard` · *defer / recover*

Deferred calls run when the function returns, in last-in-first-out order, and their ARGUMENTS are evaluated at the defer statement, not at run time.

**Steps:**

1. Three defers print in reverse: 3, 2, 1.
2. i is captured when defer executes, so each holds its own value.

```go
package main

import "fmt"

func main() {
	fmt.Println("start")
	for i := 1; i <= 3; i++ {
		defer fmt.Println("deferred", i) // i captured now; runs in reverse
	}
	fmt.Println("end")
}
```

**Output:**

```
start
end
deferred 3
deferred 2
deferred 1
```

---

## 19. defer + recover: catch a panic

`🔴 hard` · *defer / recover*

recover, called inside a deferred function, stops a panic from crashing the program and lets you convert it into a normal error.

**Steps:**

1. safeDivide defers a func that calls recover().
2. Dividing by zero panics, is recovered, and becomes a returned error via the named return.

```go
package main

import "fmt"

func safeDivide(a, b int) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered: %v", r)
		}
	}()
	result = a / b // panics when b == 0
	return result, nil
}

func main() {
	fmt.Println(safeDivide(10, 2))
	fmt.Println(safeDivide(10, 0))
}
```

**Output:**

```
5 <nil>
0 recovered: runtime error: integer divide by zero
```

---

## 20. goto (rarely used)

`🔴 hard` · *goto*

goto jumps to a label in the same function; it's occasionally handy but usually a loop or function reads better.

**Steps:**

1. Label `loop:` and goto loop while i < 3.
2. Shown for completeness — prefer for loops in real code.

```go
package main

import "fmt"

func main() {
	i := 0
loop:
	if i < 3 {
		fmt.Println("i =", i)
		i++
		goto loop
	}
	fmt.Println("done")
}
```

**Output:**

```
i = 0
i = 1
i = 2
done
```

---

