# Step 06 — Functions · 🟡 Medium

Examples **6–13**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 6. Variadic functions

`🟡 medium` · *Variadic*

A trailing ...T parameter accepts any number of arguments, received inside the function as a slice.

**Steps:**

1. sum(nums ...int) can be called with zero or many ints.
2. Inside, nums is a []int you can range over.

```go
package main

import "fmt"

func sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

func main() {
	fmt.Println(sum())
	fmt.Println(sum(1, 2, 3))
	fmt.Println(sum(10, 20, 30, 40))
}
```

**Output:**

```
0
6
100
```

---

## 7. Spreading a slice into a variadic

`🟡 medium` · *Variadic*

To pass an existing slice to a variadic function, spread it with slice... at the call site.

**Steps:**

1. xs is a []int.
2. sum(xs...) forwards its elements as the variadic args.

```go
package main

import "fmt"

func sum(nums ...int) int {
	t := 0
	for _, n := range nums {
		t += n
	}
	return t
}

func main() {
	xs := []int{1, 2, 3, 4}
	fmt.Println(sum(xs...)) // spread the slice
}
```

**Output:**

```
10
```

---

## 8. Functions are values

`🟡 medium` · *First-class functions*

Functions are first-class: you can assign them to variables and call through the variable.

**Steps:**

1. add and mul are values stored in variables.
2. op holds one then the other and is called the same way.

```go
package main

import "fmt"

func main() {
	add := func(a, b int) int { return a + b }
	mul := func(a, b int) int { return a * b }

	op := add
	fmt.Println(op(3, 4)) // 7
	op = mul
	fmt.Println(op(3, 4)) // 12
}
```

**Output:**

```
7
12
```

---

## 9. Functions as arguments (higher-order)

`🟡 medium` · *First-class functions*

A function can take another function as a parameter, letting the caller inject behavior.

**Steps:**

1. apply(x, f) calls f(x).
2. Pass different functions (double, square) to change the result.

```go
package main

import "fmt"

func apply(x int, f func(int) int) int {
	return f(x)
}

func main() {
	double := func(n int) int { return n * 2 }
	square := func(n int) int { return n * n }
	fmt.Println(apply(5, double)) // 10
	fmt.Println(apply(5, square)) // 25
}
```

**Output:**

```
10
25
```

---

## 10. Returning a function

`🟡 medium` · *First-class functions*

A function can return another function; the returned function remembers values from where it was created (a closure).

**Steps:**

1. adder(n) returns a func that adds n.
2. add10 is that returned function with n fixed at 10.

```go
package main

import "fmt"

func adder(n int) func(int) int {
	return func(x int) int {
		return x + n
	}
}

func main() {
	add10 := adder(10)
	fmt.Println(add10(5))   // 15
	fmt.Println(add10(100)) // 110
}
```

**Output:**

```
15
110
```

---

## 11. Closures capture state

`🟡 medium` · *Closures*

A closure keeps a live reference to variables from its enclosing scope, so it can carry private, mutable state across calls.

**Steps:**

1. counter() returns a func that increments a captured count.
2. Each counter has independent state.

```go
package main

import "fmt"

func counter() func() int {
	count := 0
	return func() int {
		count++
		return count
	}
}

func main() {
	next := counter()
	fmt.Println(next(), next(), next()) // 1 2 3

	other := counter()   // independent state
	fmt.Println(other()) // 1
}
```

**Output:**

```
1 2 3
1
```

---

## 12. Anonymous functions & IIFE

`🟡 medium` · *First-class functions*

You can define a function inline with no name and call it immediately (an immediately-invoked function expression).

**Steps:**

1. The func literal is defined and called in one expression.
2. Useful for small one-off computations.

```go
package main

import "fmt"

func main() {
	result := func(a, b int) int {
		return a * b
	}(6, 7) // call it right away
	fmt.Println(result)
}
```

**Output:**

```
42
```

---

## 13. Recursion

`🟡 medium` · *Recursion*

A function may call itself; every recursion needs a base case to stop.

**Steps:**

1. factorial(n) calls factorial(n-1) until n <= 1.
2. The base case returns 1 and unwinds the stack.

```go
package main

import "fmt"

func factorial(n int) int {
	if n <= 1 {
		return 1 // base case
	}
	return n * factorial(n-1)
}

func main() {
	fmt.Println(factorial(5)) // 120
}
```

**Output:**

```
120
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
