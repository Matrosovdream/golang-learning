# 03 — First Program & Basic Syntax

## Goals
- Write, run, and understand a minimal Go program line by line.
- Declare variables and constants the idiomatic way.
- Understand packages, imports, and exported vs unexported names.
- Internalize why `gofmt` runs on every save.

## Concepts
- **Anatomy of the smallest program:**
  ```go
  package main

  import "fmt"

  func main() {
      fmt.Println("Hello, Go!")
  }
  ```
  - **`package main`** — every `.go` file belongs to a package. The special package `main` with a `func main()` is what produces an executable. Library packages use any other name.
  - **`import "fmt"`** — pulls in the `fmt` (format) standard-library package. Unused imports are a **compile error** (not a warning) — Go refuses to build dead imports.
  - **`func main()`** — the entry point. Takes no args, returns nothing. The program exits when it returns.
  - **`fmt.Println`** — `Package.Function`. `Println` is **exported** (capitalized).
- **Exported vs unexported** — visibility is controlled by the **first letter's case**, not a keyword: **Capitalized = exported** (public, usable from other packages); **lowercase = unexported** (private to the package). `fmt.Println` is exported; a helper named `printLine` would be private.
- **Declaring variables:**
  - `var x int = 5` — full form with explicit type.
  - `var x = 5` — type inferred from the value.
  - `var x int` — declared with the **zero value** (here `0`).
  - `x := 5` — **short variable declaration**: declares + infers + assigns. **Only usable inside functions**, not at package level.
- **Zero values** — Go has no "uninitialized" variables. Every type has a zero value: `0` for numbers, `""` for strings, `false` for bool, `nil` for pointers/slices/maps/interfaces. This is a deliberate safety feature.
- **Unused variables are errors.** Like unused imports, a declared-but-unused local variable fails the build. (Package-level vars are exempt.)
- **Constants** — `const Pi = 3.14159`. Declared with `const`, evaluated at compile time, can't be reassigned. Constants can be **untyped**, which lets them adapt to context (`const big = 1 << 30` works as int or float where used).
- **`iota`** — a constant generator that auto-increments within a `const` block, starting at 0. Idiomatic for enums:
  ```go
  const (
      StatusActive   = iota // 0
      StatusInactive        // 1
      StatusBanned          // 2
  )
  ```
- **Multiple declaration & grouping:**
  ```go
  var (
      name string = "Ann"
      age  int    = 30
  )
  a, b := 1, 2          // multiple short decls
  a, b = b, a           // swap, no temp var needed
  ```
- **Comments** — `// line` and `/* block */`. Doc comments are just normal comments placed directly above a declaration (covered in Part 7).
- **`gofmt`** — the canonical formatter. It uses **tabs** for indentation and decides all spacing/alignment. You never format by hand. There is exactly one correct format.
- **Semicolons** — Go uses them internally but the lexer inserts them automatically, so you don't type them. This is why the opening brace `{` **must** be on the same line as `func`/`if`/`for` — moving it to the next line breaks the program.

## Exercises
1. In `go-project/03-first-program/`, create `main.go` with the Hello program above and run it with `go run .`.
2. Add variables using all four declaration forms (`var x int = 5`, `var y = 6`, `var z int`, `w := 7`) and print them. Print `z` to confirm its zero value is `0`.
3. Trigger the two famous compile errors on purpose: (a) import a package you don't use; (b) declare a variable you don't use. Read the exact error messages so you'll recognize them later.
4. Declare a `const` block of statuses using `iota` and print each value.
5. Write a string and an int side by side with `fmt.Println(name, age)`, then format them with `fmt.Printf("%s is %d\n", name, age)`. Note `%s`/`%d`/`\n`.
6. Deliberately mis-indent the file, save, run `go fmt ./...`, and watch it fix everything.

## Best Practices & Pitfalls
- **Prefer `:=` inside functions, `var` at package level.** `:=` is the idiomatic default for locals; reserve `var` for when you need the zero value or an explicit type.
- **Let the formatter run on save.** Don't waste energy on whitespace.
- **Name things by convention:** short names for short-lived locals (`i`, `r`, `buf`), descriptive names for package-level identifiers. Use `MixedCaps`/`mixedCaps`, **never** snake_case.
- **Pitfall — `:=` vs `=`:** `:=` *declares* new variables; `=` *assigns* to existing ones. Using `:=` again in the same scope re-declares and can shadow a variable unexpectedly. Only use `:=` when at least one variable on the left is new.
- **Pitfall — package-level `:=`:** doesn't compile. At package scope you must use `var`/`const`.
- **Don't capitalize a name unless you mean to export it.** Capitalization is your API surface.

## Checklist
- [ ] I can write and run the Hello program from memory.
- [ ] I can declare variables four ways and explain when to use each.
- [ ] I know what a zero value is and can predict it for common types.
- [ ] I understand exported (capital) vs unexported (lowercase) names.
- [ ] I can build an enum with `iota`.
- [ ] I know why unused imports/variables fail the build.

## Resources
- A Tour of Go — Basics: https://go.dev/tour/basics/1
- Effective Go — names & declarations: https://go.dev/doc/effective_go#names
- `fmt` package (formatting verbs): https://pkg.go.dev/fmt
- Constants & `iota`: https://go.dev/ref/spec#Iota
