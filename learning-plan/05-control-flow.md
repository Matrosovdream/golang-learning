# 05 — Control Flow

## Goals
- Use `if`/`else` idiomatically, including the init statement.
- Master the single `for` loop in all its forms (Go has no `while`).
- Use `switch`, including tagless and type switches.
- Understand `defer` and when it runs.

## Concepts
- **`if` / `else`:**
  ```go
  if x > 0 {
      // ...
  } else if x == 0 {
      // ...
  } else {
      // ...
  }
  ```
  - **No parentheses** around the condition; **braces are mandatory** (even for one line).
  - The condition must be a real `bool` — no implicit truthiness.
- **`if` with an init statement** — declare a variable scoped to the `if`/`else`:
  ```go
  if err := doThing(); err != nil {
      return err
  }
  // err is not visible here — it's scoped to the if
  ```
  This is *the* idiomatic error-checking pattern you'll see everywhere.
- **`for` — the only loop.** Three forms cover everything:
  ```go
  // 1. Classic three-part
  for i := 0; i < 10; i++ { }

  // 2. Condition-only (this is Go's "while")
  for x < 100 { x *= 2 }

  // 3. Infinite (break out manually)
  for { if done { break } }
  ```
- **`for ... range`** — iterate over slices, arrays, maps, strings, and channels:
  ```go
  for i, v := range items {        // index, value
  }
  for _, v := range items {        // ignore index with _
  }
  for k, v := range myMap {        // key, value (random order!)
  }
  for i := range 5 {               // Go 1.22+: 0,1,2,3,4
  }
  ```
- **`break` / `continue` / labels** — `break` exits the innermost loop/switch; `continue` skips to the next iteration. **Labels** let you break/continue an outer loop:
  ```go
  outer:
  for _, row := range grid {
      for _, cell := range row {
          if cell == target { break outer }
      }
  }
  ```
- **`switch`** — cleaner than chained `if`:
  ```go
  switch day {
  case "Sat", "Sun":      // multiple values per case
      fmt.Println("weekend")
  default:
      fmt.Println("weekday")
  }
  ```
  - **No automatic fallthrough** — each case breaks by default (opposite of C/Java). Use the explicit `fallthrough` keyword if you really want it.
  - **Tagless switch** — `switch { case x > 10: ...; case x > 5: ... }` is a clean replacement for long `if`/`else if` chains.
  - **`switch` with init** — `switch v := f(); v { ... }`.
- **Type switch** (preview of interfaces, lesson 11):
  ```go
  switch v := any.(type) {
  case int:    // v is an int here
  case string: // v is a string here
  default:
  }
  ```
- **`defer`** — schedules a function call to run when the surrounding function returns (whether normally or via panic). Deferred calls run in **LIFO** order:
  ```go
  f, _ := os.Open("file")
  defer f.Close()   // runs when the function returns — cleanup right next to acquisition
  ```
  - Arguments to a deferred call are **evaluated immediately**, but the call happens at return time. (Common gotcha.)
- **No `while`, no `do-while`, no ternary.** This is intentional. Use the `for` forms above and full `if`/`else`.

## Exercises
1. Write FizzBuzz (1–20) using a `for` loop and a `switch` (tagless, on `i%15`, `i%3`, `i%5`).
2. Rewrite a `while`-style countdown using condition-only `for`.
3. Use `for i := range 5` (Go 1.22+) and confirm it prints 0–4.
4. Write the idiomatic `if err := ...; err != nil { ... }` pattern around a function that returns an error.
5. Use a labeled `break` to exit a nested loop when you find a target in a 2D slice.
6. Open any file with `os.Open`, `defer f.Close()`, and add `defer fmt.Println("done")` plus another `defer` to prove they run in LIFO order.
7. **Defer gotcha:** in a loop, `defer fmt.Println(i)` for i in 0..2 — predict the output order before running.

## Best Practices & Pitfalls
- **Use the `if`-with-init pattern for errors.** It scopes the error tightly and reads cleanly: `if err := x(); err != nil { return err }`.
- **Return early; avoid deep nesting.** Handle the error/edge case and `return`, keeping the happy path un-indented. This "early return" style is core Go aesthetics.
- **`defer` for cleanup, right after acquisition.** Open then immediately `defer Close()` so you can't forget it.
- **Pitfall — `defer` inside a loop:** deferred calls don't run until the *function* returns, so deferring `Close()` for thousands of files in a loop leaks handles until the end. Move the body into its own function, or close explicitly inside the loop.
- **Pitfall — deferred argument evaluation:** `defer fmt.Println(x)` captures `x`'s value *now*; later changes to `x` won't show. To capture the final value, use a closure: `defer func() { fmt.Println(x) }()`.
- **Pitfall — map range order is random** and intentionally so. Never rely on iteration order; sort keys if you need determinism.
- **Prefer `switch` over long `if/else if` ladders** for readability.

## Checklist
- [ ] I can write all three `for` forms and `for range`.
- [ ] I use the `if err := ...; err != nil` pattern naturally.
- [ ] I know `switch` cases don't fall through by default.
- [ ] I can break/continue an outer loop with a label.
- [ ] I can explain when `defer` runs and when its args are evaluated.

## Resources
- A Tour of Go — Flow control: https://go.dev/tour/flowcontrol/1
- Effective Go — control structures: https://go.dev/doc/effective_go#control-structures
- Blog — `defer`, `panic`, `recover`: https://go.dev/blog/defer-panic-and-recover
- Spec — `for` statements: https://go.dev/ref/spec#For_statements
