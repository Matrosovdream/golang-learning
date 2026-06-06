# Step 05 — Control Flow · 🟢 Easy

Examples **1–6**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
