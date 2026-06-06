# Step 06 — Functions · 🔴 Hard

Examples **14–20**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
