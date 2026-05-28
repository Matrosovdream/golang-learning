# 04 — Variables, Types & Constants

## Goals
- Know Go's basic types and their zero values.
- Understand why Go requires explicit type conversions (no implicit coercion).
- Use constants and `iota` to model fixed sets of values.
- Predict the type the compiler infers in common situations.

## Concepts
- **The basic types:**
  - **Integers:** `int`, `int8`, `int16`, `int32`, `int64` and unsigned `uint`, `uint8`…`uint64`. Plain `int` is the default — it's 64-bit on modern machines. Use sized types only when you have a reason (binary formats, memory).
  - **Floating point:** `float32`, `float64`. `float64` is the default for decimals.
  - **`bool`** — `true`/`false`. No truthy/falsy: only actual booleans work in `if`.
  - **`string`** — immutable, UTF-8 encoded bytes (deep dive in lesson 08).
  - **Aliases:** `byte` = `uint8` (a raw byte), `rune` = `int32` (a Unicode code point).
  - **`complex64`/`complex128`** — exist, rarely used; ignore for now.
- **Zero values** (no `nil`/`undefined` surprises): `int`→`0`, `float64`→`0`, `bool`→`false`, `string`→`""`. A `var x int` is immediately usable as `0`.
- **No implicit conversions.** Go will **not** auto-convert between types, even `int` and `int64`. You must convert explicitly with `T(v)`:
  ```go
  var i int = 10
  var f float64 = float64(i)   // required
  var u uint = uint(f)         // truncates toward zero
  ```
  This prevents a class of subtle bugs (and surprises people coming from JS/Python).
- **Type inference** — `:=` and `var x = ...` infer the type from the right side. An integer literal infers to `int`; a decimal literal infers to `float64`.
- **Untyped constants** — constants without an explicit type are *untyped* and have very high precision. They adapt to the context where they're used:
  ```go
  const k = 1 << 40        // untyped, fine even though it overflows int32
  var a float64 = k        // becomes float64 here
  var b int64 = k          // becomes int64 here
  ```
  This is why `const Pi = 3.14159` can be used in both `float32` and `float64` math.
- **Typed constants** — `const MaxUsers int = 100` is locked to `int` and follows the no-implicit-conversion rule.
- **`iota` enums** — auto-incrementing constant generator within a `const` block; resets to 0 in each block:
  ```go
  type Weekday int
  const (
      Sunday Weekday = iota // 0
      Monday                // 1
      Tuesday               // 2
  )
  // Skip a value with _ , or scale with expressions:
  const (
      _  = iota             // skip 0
      KB = 1 << (10 * iota) // 1<<10
      MB                    // 1<<20
      GB                    // 1<<30
  )
  ```
- **Named types** — `type Celsius float64` creates a *distinct* type (not just an alias). You can't mix `Celsius` and `float64` without conversion — useful for type safety (e.g., not confusing `Celsius` with `Fahrenheit`).
- **Type alias vs named type** — `type Byte = uint8` (with `=`) is a true alias (identical type); `type MyInt int` (no `=`) is a new distinct type. Aliases are rare; named types are common.

## Exercises
1. Declare one variable of each basic type without initializing, then print all of them to observe the zero values.
2. Try to add an `int` and a `float64` directly — read the compile error — then fix it with an explicit conversion.
3. Convert a `float64` like `3.99` to `int` and confirm it truncates (not rounds) to `3`.
4. Build a `Weekday` enum with `iota` and a `String()`-free version first; print the numeric values.
5. Build the `KB/MB/GB` size constants with `iota` and bit shifts, then print `GB`.
6. Define `type Celsius float64` and `type Fahrenheit float64`, write a conversion function between them, and confirm the compiler stops you from assigning one to the other directly.

## Best Practices & Pitfalls
- **Default to `int` and `float64`.** Don't reach for `int32`/`int64` unless an external format or memory constraint demands it.
- **Use named types for domain values** (`UserID`, `Cents`, `Celsius`) — it makes APIs self-documenting and prevents mixing up values that happen to share an underlying type.
- **Pitfall — integer division:** `5 / 2` is `2`, not `2.5`, because both operands are ints. Convert first: `float64(5) / 2`.
- **Pitfall — overflow is silent.** `int8(127) + 1` wraps to `-128` with no error. Use a big enough type.
- **Pitfall — conversion truncates, doesn't round.** `int(2.99)` is `2`. Use `math.Round` if you need rounding.
- **Don't over-use `iota` cleverness.** A readable explicit list beats a too-clever expression chain.

## Checklist
- [ ] I can list the common basic types and their zero values.
- [ ] I know Go requires explicit conversions and can write `T(v)`.
- [ ] I understand untyped vs typed constants.
- [ ] I can build an enum with `iota`, including skipping/scaling values.
- [ ] I understand why integer division and overflow behave as they do.

## Resources
- A Tour of Go — Basic types: https://go.dev/tour/basics/11
- Effective Go — constants: https://go.dev/doc/effective_go#constants
- Blog — Constants (untyped): https://go.dev/blog/constants
- Spec — Numeric types: https://go.dev/ref/spec#Numeric_types
