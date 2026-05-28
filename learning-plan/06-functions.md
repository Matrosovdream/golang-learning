# 06 — Functions

## Goals
- Declare functions with single, multiple, and named return values.
- Use variadic functions and pass functions as values.
- Understand closures and how they capture variables.
- Understand `panic`/`recover` and why they're rare.

## Concepts
- **Basic function:**
  ```go
  func add(a int, b int) int {
      return a + b
  }
  func add(a, b int) int { } // shared type: write it once
  ```
  Parameter and return types come *after* the name (opposite of C/Java).
- **Multiple return values** — a defining Go feature, used pervasively for `(result, error)`:
  ```go
  func divide(a, b float64) (float64, error) {
      if b == 0 {
          return 0, errors.New("divide by zero")
      }
      return a / b, nil
  }
  q, err := divide(10, 2)
  ```
- **Named return values** — name the returns; a bare `return` returns their current values:
  ```go
  func split(sum int) (x, y int) {
      x = sum * 4 / 9
      y = sum - x
      return // returns x, y
  }
  ```
  Useful with `defer` (e.g., to modify the error on the way out), but don't overuse — they hurt readability in long functions.
- **The blank identifier `_`** — discard a return you don't need: `_, err := f()`. Required because unused variables are errors.
- **Variadic functions** — accept any number of trailing args; inside, the param is a slice:
  ```go
  func sum(nums ...int) int {
      total := 0
      for _, n := range nums { total += n }
      return total
  }
  sum(1, 2, 3)
  vals := []int{1, 2, 3}
  sum(vals...) // spread a slice with ...
  ```
- **Functions are first-class values** — store them in variables, pass them as arguments, return them:
  ```go
  func apply(nums []int, fn func(int) int) []int { /* ... */ }
  double := func(x int) int { return x * 2 } // function literal (anonymous)
  ```
- **Closures** — a function literal *closes over* variables from its enclosing scope, keeping them alive and shared:
  ```go
  func counter() func() int {
      count := 0
      return func() int {
          count++
          return count
      }
  }
  c := counter()
  c(); c() // 1, then 2 — state persists between calls
  ```
- **Recursion** — supported normally; Go has no tail-call optimization, so deep recursion can overflow the stack — prefer loops for large ranges.
- **`panic` and `recover`:**
  - **`panic`** stops normal flow and unwinds the stack, running deferred calls, then crashes the program. Use it for truly unrecoverable, programmer-error situations (not for normal errors).
  - **`recover`** (only meaningful inside a deferred function) stops a panic's unwinding and lets the program continue:
    ```go
    func safe() {
        defer func() {
            if r := recover(); r != nil {
                fmt.Println("recovered:", r)
            }
        }()
        panic("boom")
    }
    ```
  - These are **not** Go's error handling — `error` return values are. `panic`/`recover` are reserved for exceptional cases (e.g., a web server recovering from a handler panic so one bad request doesn't crash the whole process).
- **No function overloading, no default arguments.** Use distinct names or an options struct/variadic instead.

## Exercises
1. Write `divide(a, b float64) (float64, error)` returning an error on divide-by-zero. Call it and handle both returns.
2. Write a variadic `sum(nums ...int) int`, then call it both with literal args and by spreading a slice (`sum(vals...)`).
3. Write `counter()` that returns a closure incrementing a captured count; create two counters and prove they have independent state.
4. Write `apply(nums []int, fn func(int) int) []int` and pass it an anonymous `double` function.
5. Write `safeDivide` that uses `defer`+`recover` to turn a panic into a returned error (then discuss with Claude why returning an error directly is better here).
6. **Closure gotcha (pre-1.22 style):** create a slice of functions in a loop that each capture the loop variable; observe behavior. (Go 1.22 changed loop-var scoping — ask Claude what changed.)

## Best Practices & Pitfalls
- **Return `error` as the last value.** `(T, error)` is the universal convention; always check it (lesson 12).
- **Keep functions small and single-purpose.** Go favors many small functions over a few large ones.
- **Use named returns sparingly** — they shine for `defer`-based error wrapping but obscure flow in long functions.
- **Pitfall — ignoring errors with `_`:** only discard an error when you genuinely don't care (rare). Silently dropping errors is the #1 source of mysterious bugs.
- **Pitfall — `panic` for normal errors:** don't. A "user not found" is an `error`, not a `panic`. Reserve `panic` for impossible states / programmer mistakes.
- **Pitfall — loop variable capture:** before Go 1.22, all closures in a `for` loop shared one loop variable. Since 1.22 each iteration gets a fresh copy. Know which Go version you're on.

## Checklist
- [ ] I can write functions with multiple return values and handle `(T, error)`.
- [ ] I can write and call a variadic function and spread a slice into it.
- [ ] I can write a closure and explain what it captures.
- [ ] I understand `panic`/`recover` and why they're not normal error handling.
- [ ] I know to put `error` last and always check it.

## Resources
- A Tour of Go — Functions & closures: https://go.dev/tour/moretypes/24
- Effective Go — functions: https://go.dev/doc/effective_go#functions
- Blog — defer, panic, recover: https://go.dev/blog/defer-panic-and-recover
- Go 1.22 loop variable change: https://go.dev/blog/loopvar-preview
