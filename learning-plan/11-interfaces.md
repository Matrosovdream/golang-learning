# 11 — Interfaces

## Goals
- Understand interfaces as behavior contracts, satisfied implicitly.
- Use the empty interface (`any`), type assertions, and type switches.
- Learn the idioms that make interfaces a powerful architectural tool.
- Build the mental model that drives Go's dependency injection (Part 7).

## Concepts
- **An interface is a set of method signatures** — a contract describing *behavior*, not data:
  ```go
  type Stringer interface {
      String() string
  }
  ```
- **Implicit satisfaction (structural typing)** — a type satisfies an interface **automatically** if it has the required methods. There is **no `implements` keyword**. If your type has a `String() string` method, it *is* a `Stringer`, even if you never mention `Stringer`. This is Go's most important design idea.
- **Why this matters:** the *consumer* defines the interface it needs; the *producer* doesn't need to know. This decouples packages and is the foundation of testing with fakes and dependency injection.
- **Small interfaces are idiomatic.** The standard library's most-used interfaces have one method:
  ```go
  type Writer interface { Write(p []byte) (n int, err error) } // io.Writer
  type Reader interface { Read(p []byte) (n int, err error) }   // io.Reader
  ```
  "The bigger the interface, the weaker the abstraction." Prefer one- or two-method interfaces.
- **Using interfaces for polymorphism:**
  ```go
  type Shape interface { Area() float64 }
  func TotalArea(shapes []Shape) float64 {
      var sum float64
      for _, s := range shapes { sum += s.Area() }
      return sum
  }
  ```
  Any type with an `Area() float64` method (Circle, Rectangle, …) can be passed — that's the polymorphism inheritance would give you in other languages.
- **An interface value is a (type, value) pair.** It stores both the concrete type and the concrete value. This is how type assertions and `%T` know what's inside.
- **The empty interface — `any`** — `interface{}` (aliased to `any` since Go 1.18) has no methods, so **every** type satisfies it. Use it only when you truly need to hold "anything" (e.g., `fmt.Println(...any)`). Prefer concrete types or generics when possible.
- **Type assertion** — recover the concrete type from an interface value:
  ```go
  s, ok := val.(string)  // ok = false if val isn't a string (safe form)
  s := val.(string)      // panics if wrong (use only when certain)
  ```
- **Type switch** — branch on the dynamic type:
  ```go
  switch v := val.(type) {
  case int:    // v is int
  case string: // v is string
  case nil:
  default:
  }
  ```
- **The `Stringer` interface** — `fmt` checks if a value has `String() string` and uses it automatically when printing. Implementing `Stringer` controls how your type appears in logs.
- **Nil interface gotcha** — an interface is nil only if **both** its type and value are nil. A non-nil interface holding a nil pointer is **not** equal to `nil`. This famously breaks naive error checks (see Pitfalls).
- **Architecture seed — "accept interfaces, return structs."** Functions should take the *narrowest interface* they need as a parameter (so callers can pass anything that fits, including test fakes) and return *concrete types* (so callers get full functionality). This is the backbone of clean Go design in Part 7.

## Exercises
1. Define a `Shape` interface with `Area() float64`. Implement it for `Circle` and `Rectangle`. Write `TotalArea([]Shape)` and pass a mix of both.
2. Implement `String() string` on a `Point` type and print it — watch `fmt` use it automatically.
3. Write a function `describe(v any)` using a type switch that prints different messages for `int`, `string`, `bool`, and a default.
4. Use the safe type-assertion form `s, ok := v.(string)` and handle both branches.
5. Recreate the nil-interface gotcha: a function returns an `error` interface holding a nil `*MyError` pointer; show that `err != nil` is unexpectedly true, and discuss the fix with Claude.
6. Define a tiny `Notifier` interface (`Notify(msg string) error`) and two implementations (email, log). Write code that depends only on `Notifier` — this previews dependency injection.

## Best Practices & Pitfalls
- **Define interfaces where they're used (the consumer), not where types are implemented.** Don't ship a big "interface package"; let each consumer declare the small contract it needs.
- **Keep interfaces small.** One or two methods is the sweet spot. Compose larger behavior from small interfaces (like `io.ReadWriter = Reader + Writer`).
- **"Accept interfaces, return concrete types."** Flexible inputs, useful outputs.
- **Pitfall — `any` everywhere.** Reaching for `any` throws away type safety. Use concrete types or generics (lesson 17) unless you genuinely need to hold arbitrary values.
- **Pitfall — the typed-nil interface bug:** returning a `nil` concrete pointer as an `error` makes `err != nil` true. Return a literal `nil` error, not a nil typed pointer.
- **Pitfall — unchecked type assertions** panic. Use the comma-ok form unless a panic is genuinely acceptable.
- **Implement `Stringer` (and `error`) for your domain types** to get clean logging and messages for free.

## Checklist
- [ ] I understand interfaces are satisfied implicitly (no `implements`).
- [ ] I can write a small interface and multiple implementations and use them polymorphically.
- [ ] I can use type assertions (safe form) and type switches.
- [ ] I know `any` is the empty interface and when (not) to use it.
- [ ] I can explain the typed-nil interface gotcha.
- [ ] I understand "accept interfaces, return structs" and consumer-defined interfaces.

## Resources
- A Tour of Go — Interfaces: https://go.dev/tour/methods/9
- Effective Go — interfaces: https://go.dev/doc/effective_go#interfaces
- Blog — Laws of Reflection (interface = type+value): https://go.dev/blog/laws-of-reflection
- Code Review Comments — interfaces: https://go.dev/wiki/CodeReviewComments#interfaces
