# 10 — Pointers & Methods

## Goals
- Understand pointers in Go (and why they're safer than C pointers).
- Define methods on types with value and pointer receivers.
- Choose the right receiver type with confidence.
- Understand method sets and nil receivers.

## Concepts
- **Pointers hold a memory address.** `&x` takes the address of `x`; `*p` dereferences (reads/writes the value at `p`):
  ```go
  x := 10
  p := &x      // p is *int, points to x
  *p = 20      // x is now 20
  fmt.Println(*p, x) // 20 20
  ```
- **Why pointers?** Two reasons: (1) **mutation** — let a function change the caller's value; (2) **efficiency** — avoid copying large structs. Go's pointers are memory-safe: no pointer arithmetic, and the garbage collector tracks them.
- **Zero value of a pointer is `nil`.** Dereferencing a nil pointer panics. Check before dereferencing when a pointer can be nil.
- **`new(T)`** allocates a zeroed `T` and returns a `*T`. Rarely used directly; `&T{}` is more common and clearer.
- **No `->` operator.** Go auto-dereferences: if `p` is `*User`, `p.Name` works the same as `(*p).Name`.
- **Methods** — a function with a *receiver* attached to a type:
  ```go
  type Counter struct{ n int }

  func (c Counter) Value() int { return c.n }   // value receiver
  func (c *Counter) Inc()      { c.n++ }         // pointer receiver
  ```
  Call them like `c.Value()` / `c.Inc()`.
- **Value receiver `(c Counter)`** — the method gets a **copy**. Mutations don't affect the original. Good for read-only methods and small types.
- **Pointer receiver `(c *Counter)`** — the method gets a pointer to the original; mutations **persist**. Required when the method must modify the receiver, and preferred for large structs (avoids copying).
- **Go auto-takes the address.** Even though `Inc` needs `*Counter`, you can call `c.Inc()` on an addressable value `c` — Go inserts `&c` for you. (This doesn't work on non-addressable values like map elements.)
- **Method sets** (matters for interfaces, lesson 11):
  - The method set of `T` includes only **value-receiver** methods.
  - The method set of `*T` includes **both** value- and pointer-receiver methods.
  - Consequence: if a method has a pointer receiver, you generally need a **pointer** to satisfy an interface that requires it.
- **Receiver consistency rule:** if *any* method of a type needs a pointer receiver, give **all** its methods pointer receivers, for consistency and to keep the method set coherent.
- **Methods can be defined on any named type you own**, not just structs:
  ```go
  type Celsius float64
  func (c Celsius) String() string { return fmt.Sprintf("%.1f°C", c) }
  ```
  (You can't add methods to types from other packages — define a named type wrapping it.)
- **Nil receivers can be valid.** A pointer-receiver method can be called on a `nil` pointer and handle it gracefully (common in linked lists / trees) — but only if the method checks for nil before dereferencing fields.

## Exercises
1. Create `x := 10`, take `p := &x`, set `*p = 20`, and print both to see them stay in sync.
2. Write a function `zero(p *int)` that sets `*p = 0`; call it and confirm the caller's variable changed (contrast with a value-parameter version that doesn't).
3. Define `Counter` with a value-receiver `Value()` and a pointer-receiver `Inc()`. Call `Inc()` a few times and confirm `Value()` reflects the changes.
4. Change `Inc` to a *value* receiver and observe that increments no longer stick — explain why.
5. Define `type Celsius float64` with a `String() string` method and print a `Celsius` value (note `fmt` uses `String()` automatically — preview of `Stringer`).
6. Write a `*Node` linked-list type whose `Sum()` method handles a `nil` receiver as 0.

## Best Practices & Pitfalls
- **Use a pointer receiver when:** the method mutates the receiver, the struct is large, or the type has *any* pointer-receiver method (keep them consistent).
- **Use a value receiver when:** the type is small and the method is read-only (e.g., a 2-word struct, basic types). Value receivers are also safe for concurrent reads.
- **Pitfall — value receiver can't mutate.** A method on `(c Counter)` that does `c.n++` changes only the copy; the caller sees nothing. This is the #1 receiver bug.
- **Pitfall — mixing receiver types** on one type leads to confusing method sets and interface-satisfaction surprises. Be consistent.
- **Pitfall — calling a pointer method on a map element:** `m[k].Inc()` won't compile because map elements aren't addressable. Read it out, modify, write back.
- **Pitfall — dereferencing nil.** Guard pointer params that can be nil before using `*p` or `p.field`.

## Checklist
- [ ] I can use `&` and `*` and explain what each does.
- [ ] I know the two reasons to use a pointer (mutation, avoid copying).
- [ ] I can write value- and pointer-receiver methods and predict whether mutations persist.
- [ ] I can choose the right receiver type and keep them consistent.
- [ ] I understand method sets of `T` vs `*T` at a high level.
- [ ] I know dereferencing a nil pointer panics.

## Resources
- A Tour of Go — Methods & pointers: https://go.dev/tour/methods/1
- Effective Go — pointers vs values: https://go.dev/doc/effective_go#pointers_vs_values
- Code Review Comments — receiver type: https://go.dev/wiki/CodeReviewComments#receiver-type
- Spec — method sets: https://go.dev/ref/spec#Method_sets
