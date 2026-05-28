# 09 — Structs

## Goals
- Define and instantiate structs the idiomatic way.
- Use composition (embedding) instead of inheritance.
- Understand struct tags and where they're used.
- Know when structs are comparable and how they're copied.

## Concepts
- **A struct is a typed collection of fields** — Go's primary way to model data (there are no classes):
  ```go
  type User struct {
      ID    int
      Name  string
      Email string
  }
  ```
- **Creating structs:**
  ```go
  u := User{ID: 1, Name: "Ann", Email: "a@x.com"} // keyed literal (preferred)
  u := User{1, "Ann", "a@x.com"}                  // positional (fragile — avoid)
  var u User                                       // zero value: all fields zeroed
  p := &User{Name: "Bo"}                           // pointer to a struct
  ```
- **Accessing fields** — `u.Name`. Go auto-dereferences pointers, so `p.Name` works on `*User` too (no `->` operator).
- **Structs are value types.** Assigning or passing a struct **copies all its fields**. To share/mutate, pass a pointer (`*User`). This matters for performance (large structs) and for methods (lesson 10).
- **Embedding (composition)** — embed a type to "promote" its fields and methods, building bigger types from smaller ones:
  ```go
  type Animal struct{ Name string }
  func (a Animal) Speak() string { return a.Name + " makes a sound" }

  type Dog struct {
      Animal      // embedded — no field name
      Breed string
  }
  d := Dog{Animal: Animal{Name: "Rex"}, Breed: "Lab"}
  d.Name      // promoted field — reads d.Animal.Name
  d.Speak()   // promoted method
  ```
  This is **composition, not inheritance**: `Dog` *has an* `Animal`, and its methods are promoted for convenience. There's no polymorphic override — interfaces (lesson 11) provide polymorphism.
- **Anonymous structs** — one-off structs without a named type, handy for table-driven tests and quick groupings:
  ```go
  point := struct{ X, Y int }{X: 1, Y: 2}
  ```
- **Struct tags** — string metadata attached to fields, read via reflection by libraries (JSON, SQL, validation). The backtick syntax:
  ```go
  type User struct {
      ID    int    `json:"id"`
      Name  string `json:"name"`
      Email string `json:"email,omitempty"`
  }
  ```
  Tags don't affect Go code directly; `encoding/json` reads `json:"..."` to decide field names in output (lesson 19/21).
- **Comparability** — structs are comparable with `==` if **all** their fields are comparable (numbers, strings, bools, pointers, arrays of comparable types). Structs containing slices, maps, or functions are **not** comparable and `==` won't compile. Comparable structs can be used as map keys.
- **Methods belong to types, not structs specifically** — but most methods are defined on structs. That's lesson 10.

## Exercises
1. Define a `User` struct and create instances with a keyed literal, a zero value, and a `&User{}` pointer. Print each with `%+v`.
2. Modify a field through a pointer (`p.Name = "X"`) and confirm the change; then pass a struct *by value* to a function that mutates a field and show the original is unchanged.
3. Build `Animal` + `Dog` with embedding; access a promoted field and call a promoted method through `Dog`.
4. Add `json` struct tags to `User` (with one `omitempty`) — you'll use these in Part 6.
5. Use an anonymous struct to hold a `{name, age}` pair inline.
6. Try comparing two `User` values with `==`; then add a `[]string` field and observe the compile error — explain why.

## Best Practices & Pitfalls
- **Always use keyed struct literals** (`User{Name: ...}`), not positional ones. Positional literals break silently when fields are reordered or added.
- **Decide value vs pointer deliberately.** Small, immutable-ish structs are fine to copy; large structs or ones you must mutate should be passed as pointers (more in lesson 10).
- **Prefer composition over deep embedding chains.** Embedding is for genuine "is-built-from" relationships and small mixins, not for faking class hierarchies.
- **Pitfall — embedding name clashes:** if `Dog` and its embedded `Animal` both have a `Name`, the outer one wins and the inner is reached via `d.Animal.Name`. Avoid accidental shadowing.
- **Pitfall — copying a struct that contains a mutex or slice:** copying a `sync.Mutex` breaks it; copying a struct with a slice shares the backing array. Use pointers for such types.
- **Use struct tags exactly as the library expects** — a typo in `json:"id"` silently falls back to the field name.

## Checklist
- [ ] I can define a struct and create it with a keyed literal, zero value, and pointer.
- [ ] I understand structs are copied by value and when to use a pointer.
- [ ] I can embed a type and use promoted fields/methods.
- [ ] I know what struct tags are and where libraries read them.
- [ ] I can explain when a struct is comparable / usable as a map key.

## Resources
- A Tour of Go — Structs: https://go.dev/tour/moretypes/2
- Effective Go — embedding: https://go.dev/doc/effective_go#embedding
- Spec — struct types: https://go.dev/ref/spec#Struct_types
- Blog — JSON and Go (struct tags in action): https://go.dev/blog/json
