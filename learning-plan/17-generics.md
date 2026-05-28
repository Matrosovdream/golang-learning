# 17 — Generics

## Goals
- Write functions and types parameterized by type (Go 1.18+).
- Use type constraints, including `comparable` and the `constraints` package.
- Understand type inference for generic calls.
- Judge when generics help and when they hurt.

## Concepts
- **Why generics?** Before 1.18, writing a `Map`/`Filter`/`Min` that worked for many types meant either duplicating code per type or using `any` (losing type safety). Generics let you write it once, type-safely.
- **Type parameters** — declared in square brackets after the function/type name:
  ```go
  func Map[T, U any](s []T, f func(T) U) []U {
      out := make([]U, len(s))
      for i, v := range s {
          out[i] = f(v)
      }
      return out
  }
  doubled := Map([]int{1, 2, 3}, func(n int) int { return n * 2 })
  ```
  `T` and `U` are type parameters; `any` is their **constraint** (here, "any type").
- **Constraints** — an interface used as a constraint specifies what operations the type must support. `any` allows everything (but then you can only do things valid for all types — store, pass, compare-by-`==` only if `comparable`).
- **`comparable`** — a built-in constraint for types usable with `==`/`!=` (needed for map keys, equality). Example:
  ```go
  func Contains[T comparable](s []T, target T) bool {
      for _, v := range s {
          if v == target { return true }
      }
      return false
  }
  ```
- **Constraints with operators** — to use `<`, `+`, etc., the constraint must list the allowed underlying types. The `golang.org/x/exp/constraints` package (and your own unions) provide these:
  ```go
  type Number interface {
      ~int | ~int64 | ~float64   // ~ means "any type whose underlying type is this"
  }
  func Sum[T Number](nums []T) T {
      var total T
      for _, n := range nums { total += n }
      return total
  }
  ```
  - The `~` (tilde) means "and any named type with this underlying type" (so `type Celsius float64` still qualifies).
  - `|` is a **union** of allowed types.
- **Generic types** — structs/containers parameterized by type:
  ```go
  type Stack[T any] struct{ items []T }
  func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }
  func (s *Stack[T]) Pop() (T, bool) {
      var zero T
      if len(s.items) == 0 { return zero, false }
      v := s.items[len(s.items)-1]
      s.items = s.items[:len(s.items)-1]
      return v, true
  }
  ```
  Note `var zero T` to get the zero value of a type parameter.
- **Type inference** — you usually don't write the type arguments; the compiler infers them from the function arguments (`Map([]int{...}, f)` infers `T=int`). Supply them explicitly only when inference can't (`Stack[int]{}`).
- **Methods can't add type parameters.** Only the *type* declares them (`Stack[T]`); methods use the type's parameters but can't introduce new ones.

## Exercises
1. Write a generic `Map[T, U any]([]T, func(T) U) []U` and use it to turn `[]int` into `[]string`.
2. Write `Filter[T any]([]T, func(T) bool) []T` and use it to keep even numbers.
3. Write `Contains[T comparable]([]T, T) bool` and test it on `[]string` and `[]int`.
4. Define a `Number` constraint with a type union and write `Sum[T Number]([]T) T`; call it with `[]int` and `[]float64`.
5. Implement a generic `Stack[T any]` with `Push`/`Pop` (return `(T, bool)` using `var zero T`); use it with `int` and with a struct type.
6. Try calling a generic function with explicit type args (`Map[int, string](...)`) vs relying on inference; note when each is needed.
7. Discuss with Claude one case where generics are clearly worth it (containers, `Map`/`Filter`) and one where a plain interface or concrete code is better.

## Best Practices & Pitfalls
- **Reach for generics for *containers* and *algorithms over many types* (collections, `Map`/`Filter`/`Min`), not as a default.** Most Go code doesn't need them.
- **Prefer interfaces when you need behavior, generics when you need to preserve a concrete type across a transformation.** If a plain interface (`io.Writer`) expresses the contract, use it.
- **Keep constraints as loose as the code allows** — `any` if you only store/pass, `comparable` if you compare, a type union only if you use operators.
- **Use `~` in type-union constraints** so named types (e.g., `type ID int`) still satisfy them — forgetting it excludes them surprisingly.
- **Pitfall — over-generic code.** Generic signatures with several type params and complex constraints can be harder to read than two concrete functions. "A little copying is better than a little dependency" — and sometimes better than a lot of generics.
- **Pitfall — expecting method type parameters.** They don't exist; put the parameters on the type.
- **Pitfall — zero values:** you can't write `nil`/`0` for a `T`; use `var zero T`.

## Checklist
- [ ] I can write a generic function with one or more type parameters.
- [ ] I can use `any`, `comparable`, and a type-union constraint (with `~`).
- [ ] I can define and use a generic type like `Stack[T]`.
- [ ] I understand type inference and when explicit type args are needed.
- [ ] I can articulate when generics help vs when to use interfaces/concrete code.

## Resources
- A Tour of Go — Generics: https://go.dev/tour/generics/1
- Blog — An Introduction to Generics: https://go.dev/blog/intro-generics
- Blog — When to use generics: https://go.dev/blog/when-generics
- `constraints` package: https://pkg.go.dev/golang.org/x/exp/constraints
