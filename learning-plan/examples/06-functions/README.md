# Step 06 — Functions · Examples

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

- [1. Declaring and calling a function](#1-declaring-and-calling-a-function)
- [2. Same-type parameter shorthand](#2-same-type-parameter-shorthand)
- [3. Multiple return values](#3-multiple-return-values)
- [4. The (value, error) idiom](#4-the-value-error-idiom)
- [5. Named return values + naked return](#5-named-return-values--naked-return)

**Medium**

- [6. Variadic functions](#6-variadic-functions)
- [7. Spreading a slice into a variadic](#7-spreading-a-slice-into-a-variadic)
- [8. Functions are values](#8-functions-are-values)
- [9. Functions as arguments (higher-order)](#9-functions-as-arguments-higher-order)
- [10. Returning a function](#10-returning-a-function)
- [11. Closures capture state](#11-closures-capture-state)
- [12. Anonymous functions & IIFE](#12-anonymous-functions--iife)
- [13. Recursion](#13-recursion)

**Hard**

- [14. Closures over the loop variable (Go 1.22+)](#14-closures-over-the-loop-variable-go-122)
- [15. defer + panic + recover → error](#15-defer--panic--recover--error)
- [16. Higher-order map and filter](#16-higher-order-map-and-filter)
- [17. Method values](#17-method-values)
- [18. Method expressions](#18-method-expressions)
- [19. Mutual recursion](#19-mutual-recursion)
- [20. Two closures sharing one variable](#20-two-closures-sharing-one-variable)

---

## 1. Declaring and calling a function

`🟢 easy` · *Basics*

A function takes typed parameters and declares its return type after the parameter list.

**Steps:**

1. func add(a int, b int) int declares two int params and an int result.
2. Call it like add(2, 3).

```go
package main

import "fmt"

func add(a int, b int) int {
	return a + b
}

func main() {
	fmt.Println(add(2, 3))
}
```

**Output:**

```
5
```

---

## 2. Same-type parameter shorthand

`🟢 easy` · *Basics*

When consecutive parameters share a type, you write the type once after the last of them.

**Steps:**

1. func volume(l, w, h int) is shorthand for three ints.
2. Identical to writing l int, w int, h int.

```go
package main

import "fmt"

func volume(l, w, h int) int {
	return l * w * h
}

func main() {
	fmt.Println(volume(2, 3, 4))
}
```

**Output:**

```
24
```

---

## 3. Multiple return values

`🟢 easy` · *Returns*

Go functions can return more than one value; the caller receives them with multiple assignment.

**Steps:**

1. minmax returns two ints (smaller, larger).
2. Capture both with lo, hi := minmax(...).

```go
package main

import "fmt"

func minmax(a, b int) (int, int) {
	if a < b {
		return a, b
	}
	return b, a
}

func main() {
	lo, hi := minmax(8, 3)
	fmt.Println("lo:", lo, "hi:", hi)
}
```

**Output:**

```
lo: 3 hi: 8
```

---

## 4. The (value, error) idiom

`🟢 easy` · *Returns*

Go's universal failure convention: return the result plus an error as the last value; nil error means success.

**Steps:**

1. safeDiv returns (0, error) on divide-by-zero, else (quotient, nil).
2. Callers check the error before trusting the result.

```go
package main

import (
	"errors"
	"fmt"
)

func safeDiv(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a / b, nil
}

func main() {
	if q, err := safeDiv(10, 2); err == nil {
		fmt.Println("10/2 =", q)
	}
	if _, err := safeDiv(1, 0); err != nil {
		fmt.Println("error:", err)
	}
}
```

**Output:**

```
10/2 = 5
error: divide by zero
```

---

## 5. Named return values + naked return

`🟢 easy` · *Returns*

You can name the return values in the signature; a bare return then returns their current values. Use sparingly — it can hurt readability in long functions.

**Steps:**

1. (q, r int) declares the results up front.
2. A naked return sends back whatever q and r currently hold.

```go
package main

import "fmt"

func divmod(a, b int) (q, r int) {
	q = a / b
	r = a % b
	return // naked return: returns q, r
}

func main() {
	q, r := divmod(17, 5)
	fmt.Println("quotient:", q, "remainder:", r)
}
```

**Output:**

```
quotient: 3 remainder: 2
```

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

## 14. Closures over the loop variable (Go 1.22+)

`🔴 hard` · *Closures*

Since Go 1.22 each loop iteration gets a fresh copy of the loop variable, so closures created in the loop capture distinct values (prints 0 1 2, not 3 3 3 as in older Go).

**Steps:**

1. Build three closures inside a for loop.
2. Each captures its own i because 1.22 scopes i per iteration.

```go
package main

import "fmt"

func main() {
	funcs := []func(){}
	for i := 0; i < 3; i++ {
		funcs = append(funcs, func() { fmt.Print(i, " ") })
	}
	for _, f := range funcs {
		f()
	}
	fmt.Println()
}
```

**Output:**

```
0 1 2 
```

---

## 15. defer + panic + recover → error

`🔴 hard` · *panic / recover*

A deferred function calling recover() stops a panic and lets you convert it into a normal returned error via a named return value.

**Steps:**

1. mustPositive panics on a negative input.
2. The deferred recover turns the panic into err instead of crashing.

```go
package main

import "fmt"

func mustPositive(n int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("caught: %v", r)
		}
	}()
	if n < 0 {
		panic("negative number")
	}
	fmt.Println("ok:", n)
	return nil
}

func main() {
	fmt.Println(mustPositive(5))
	fmt.Println(mustPositive(-1))
}
```

**Output:**

```
ok: 5
<nil>
caught: negative number
```

---

## 16. Higher-order map and filter

`🔴 hard` · *First-class functions*

Passing functions lets you write reusable map (transform each) and filter (keep some) helpers.

**Steps:**

1. mapInts applies f to every element.
2. filterInts keeps elements where keep(x) is true.

```go
package main

import "fmt"

func mapInts(xs []int, f func(int) int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func filterInts(xs []int, keep func(int) bool) []int {
	var out []int
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func main() {
	xs := []int{1, 2, 3, 4, 5}
	doubled := mapInts(xs, func(n int) int { return n * 2 })
	evens := filterInts(xs, func(n int) bool { return n%2 == 0 })
	fmt.Println("doubled:", doubled)
	fmt.Println("evens:", evens)
}
```

**Output:**

```
doubled: [2 4 6 8 10]
evens: [2 4]
```

---

## 17. Method values

`🔴 hard` · *Methods as values*

g.Method (with a receiver value) is a 'method value': a function that has the receiver bound in, callable with no receiver argument.

**Steps:**

1. f := g.Greet captures g.
2. f() then needs no arguments.

```go
package main

import "fmt"

type Greeter struct {
	Name string
}

func (g Greeter) Greet() string {
	return "Hi, I'm " + g.Name
}

func main() {
	g := Greeter{Name: "Sam"}
	f := g.Greet // method value: receiver bound now
	fmt.Println(f())
}
```

**Output:**

```
Hi, I'm Sam
```

---

## 18. Method expressions

`🔴 hard` · *Methods as values*

Type.Method (with the type, not a value) is a 'method expression': a plain function whose FIRST argument is the receiver.

**Steps:**

1. Greeter.Greet is a func(Greeter) string.
2. You pass the receiver explicitly when calling it.

```go
package main

import "fmt"

type Greeter struct {
	Name string
}

func (g Greeter) Greet() string {
	return "Hi, I'm " + g.Name
}

func main() {
	f := Greeter.Greet // method expression: receiver is the first arg
	fmt.Println(f(Greeter{Name: "Lee"}))
}
```

**Output:**

```
Hi, I'm Lee
```

---

## 19. Mutual recursion

`🔴 hard` · *Recursion*

Two functions can call each other; each still needs a base case so the chain terminates.

**Steps:**

1. isEven(n) defers to isOdd(n-1) and vice versa.
2. Both bottom out at 0.

```go
package main

import "fmt"

func isEven(n int) bool {
	if n == 0 {
		return true
	}
	return isOdd(n - 1)
}

func isOdd(n int) bool {
	if n == 0 {
		return false
	}
	return isEven(n - 1)
}

func main() {
	fmt.Println("4 even?", isEven(4))
	fmt.Println("7 even?", isEven(7))
}
```

**Output:**

```
4 even? true
7 even? false
```

---

## 20. Two closures sharing one variable

`🔴 hard` · *Closures*

Multiple closures created in the same scope share the SAME captured variable, which is how you build little stateful objects from functions.

**Steps:**

1. makeAccount returns deposit and balance closures over one bal.
2. deposit mutates bal; balance reads the same bal.

```go
package main

import "fmt"

func makeAccount() (deposit func(int), balance func() int) {
	bal := 0
	deposit = func(amount int) { bal += amount }
	balance = func() int { return bal }
	return
}

func main() {
	deposit, balance := makeAccount()
	deposit(100)
	deposit(50)
	fmt.Println("balance:", balance()) // 150
}
```

**Output:**

```
balance: 150
```

---

