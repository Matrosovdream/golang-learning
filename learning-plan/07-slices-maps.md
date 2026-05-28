# 07 — Arrays, Slices & Maps

## Goals
- Understand the difference between arrays and slices.
- Build a correct mental model of slice internals (pointer, length, capacity).
- Use `append`, `copy`, and `make` correctly, avoiding aliasing bugs.
- Use maps idiomatically, including the comma-ok idiom and sets.

## Concepts
- **Arrays** — fixed-size, value types: `var a [3]int`. The size is part of the type (`[3]int` ≠ `[4]int`). Arrays are **copied** on assignment and when passed to functions. You'll rarely use them directly — slices are what you want.
- **Slices** — the workhorse. A slice is a **view** into a backing array, described by three things:
  - **pointer** to the first element,
  - **length** (`len(s)`) — how many elements you can index,
  - **capacity** (`cap(s)`) — how many elements exist before a regrow is needed.
  ```go
  s := []int{1, 2, 3}          // literal
  s := make([]int, 3)          // len 3, cap 3, zeroed
  s := make([]int, 0, 10)      // len 0, cap 10 — preallocate
  ```
- **Slicing** — `s[low:high]` creates a new slice header pointing into the **same** backing array (no copy). `s[1:3]`, `s[:2]`, `s[2:]` all share memory with `s`.
- **`append`** — grows a slice:
  ```go
  s = append(s, 4)         // append one
  s = append(s, 5, 6, 7)   // append several
  s = append(s, other...)  // append another slice
  ```
  If there's spare capacity, `append` writes into the existing backing array; if not, it **allocates a new, larger array and copies** — which is why you must reassign `s = append(...)`.
- **The aliasing trap** — because slices share backing arrays, mutations can leak:
  ```go
  a := []int{1, 2, 3, 4}
  b := a[1:3]      // shares memory with a
  b[0] = 99        // also changes a[1]!
  ```
  And `append` can either mutate or detach the original depending on capacity — a classic source of subtle bugs.
- **`copy`** — explicitly copy elements between slices (returns number copied): `copy(dst, src)`. Use it to get an independent copy: `dup := make([]int, len(s)); copy(dup, s)`.
- **Nil vs empty slice** — a `nil` slice (`var s []int`) has len 0 and works with `append`/`range` perfectly. You rarely need to distinguish it from an empty `[]int{}`.
- **Maps** — unordered key→value hash tables:
  ```go
  m := map[string]int{"a": 1, "b": 2}
  m := make(map[string]int)
  m["c"] = 3
  delete(m, "a")
  n := len(m)
  ```
- **The comma-ok idiom** — reading a missing key returns the zero value, so you check existence with the two-value form:
  ```go
  v, ok := m["x"]   // ok is false if "x" absent; v is the zero value
  if ok { /* present */ }
  ```
- **Nil map gotcha** — reading from a nil map is fine (returns zero values), but **writing to a nil map panics**. Always `make` a map (or use a literal) before writing.
- **Map iteration order is random** — by design (lesson 05). Sort keys if you need order.
- **Sets** — Go has no built-in set; use `map[T]struct{}` (the empty struct uses zero memory) or `map[T]bool`:
  ```go
  seen := map[string]struct{}{}
  seen["x"] = struct{}{}
  _, exists := seen["x"]
  ```

## Exercises
1. Create a slice with `make([]int, 0, 5)`, append to it past capacity, and print `len`/`cap` after each append to watch capacity grow (it typically doubles).
2. Demonstrate the aliasing trap: slice `b := a[1:3]`, mutate `b[0]`, and show `a` changed too.
3. Use `copy` to make an independent duplicate of a slice and prove mutating the copy doesn't touch the original.
4. Remove an element from a slice by index using `append(s[:i], s[i+1:]...)` and explain why it works.
5. Build a `map[string]int` word-frequency counter from a slice of words using the comma-ok idiom.
6. Trigger the nil-map write panic on purpose, then fix it with `make`.
7. Implement a set with `map[string]struct{}`: add items, dedupe a slice, and test membership.

## Best Practices & Pitfalls
- **Always reassign the result of `append`:** `s = append(s, x)`. Forgetting this is a top beginner bug.
- **Preallocate when you know the size:** `make([]T, 0, n)` avoids repeated reallocations in hot loops.
- **Pitfall — sharing backing arrays:** if you return a sub-slice of internal data, the caller can mutate your internals. `copy` when you need isolation, especially across API boundaries.
- **Pitfall — `append` aliasing:** `b := append(a, x)` may or may not share `a`'s array depending on capacity, making behavior version-dependent and surprising. Use full-slice expressions `a[low:high:max]` to cap capacity when you need predictable detachment.
- **Pitfall — writing to a nil map panics.** Initialize maps before writing.
- **Pitfall — relying on map order.** It's randomized; sort keys for deterministic output (e.g., in tests).
- **Use `map[T]struct{}` for sets** — it signals "I only care about presence" and uses no value memory.

## Checklist
- [ ] I can explain slice pointer/length/capacity and what `append` does on regrow.
- [ ] I always reassign `s = append(s, ...)`.
- [ ] I can reproduce and avoid the slice aliasing trap with `copy`.
- [ ] I use the comma-ok idiom to check map membership.
- [ ] I know writing to a nil map panics.
- [ ] I can implement a set with a map.

## Resources
- A Tour of Go — Slices & maps: https://go.dev/tour/moretypes/7
- Blog — Go slices: usage and internals: https://go.dev/blog/slices-intro
- Blog — Arrays, slices: the mechanics of 'append': https://go.dev/blog/slices
- Effective Go — slices & maps: https://go.dev/doc/effective_go#slices
