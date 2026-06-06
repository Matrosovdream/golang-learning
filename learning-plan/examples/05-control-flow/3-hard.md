# Step 05 — Control Flow · 🔴 Hard

Examples **15–20**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
