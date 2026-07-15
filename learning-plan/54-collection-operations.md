# 54 — Collection Operations: Filter, Map, Reduce, Group & Sets

> Part of **Part 10 — Data Structures & Algorithms**, the practical **data-wrangling pair** with [55 — Data Pipelines](55-data-pipelines.md). Builds directly on [07 — Slices & Maps](07-slices-maps.md), [17 — Generics](17-generics.md), and [51 — Sorting & Searching](51-sorting-searching.md) (which owns *ordering* and *searching*). Thesis: **Go has no `Array.map`/`filter`/`reduce` — you write a `for` loop, reach for the `slices`/`maps` stdlib, or drop in a tiny generic helper. Learn the dozen shapes (filter, map, reduce, group-by, count-by, key-by, dedup, partition, chunk, flatten, set ops) and you can turn any `[]T` into the answer you need.**

## Goals
- Write the three core transformations — **Filter**, **Map**, **Reduce** — as plain loops *and* as reusable **generic helpers**, and know when Go's lack of built-ins is fine (a loop is clear) vs worth a helper.
- Use the **`slices`** and **`maps`** packages to replace hand-written loops: `Contains`/`IndexFunc`, `Max`/`MinFunc`, `Compact`, `Concat`, `Clone`, `Equal`, `Keys`/`Values`.
- Reach for **maps as the workhorse**: `GroupBy` → `map[K][]T`, `CountBy` → `map[K]int`, `KeyBy` → `map[K]T`, and `map[T]struct{}` as a **set** (union / intersection / difference).
- Shape data with **partition, chunk, flatten, zip, unique-by, min/max-by**, and compose them into a small **pipeline**.
- Understand Go 1.23 **iterators** (`iter.Seq`, range-over-func, `slices.Collect`/`Values`/`Sorted`, `maps.Keys`/`Values`) as the modern way to build **lazy** filter/map chains.

## Concepts

- **There is no built-in `filter`/`map`/`reduce`.** The idiomatic filter is a loop that appends to a new slice; the idiomatic map preallocates and assigns by index; the idiomatic reduce is a loop with an accumulator. This is not a limitation to route around — a three-line loop is often the clearest thing.
  ```go
  var kept []T                       // FILTER
  for _, v := range s { if keep(v) { kept = append(kept, v) } }

  out := make([]U, len(s))           // MAP (output type may differ)
  for i, v := range s { out[i] = f(v) }

  acc := init                        // REDUCE
  for _, v := range s { acc = combine(acc, v) }
  ```
- **Generic helpers pay off when the loop repeats.** With generics (1.18+) you can write `Filter[T]`, `Map[T, U]`, `Reduce[T, U]` once. `Map` needs **two type parameters** because the result type differs from the input. The stdlib deliberately ships *some* of these (`slices.IndexFunc`, `slices.Compact`) but **not** `Map`/`Filter`/`Reduce` — write your own small ones or use `golang.org/x/exp/slices`-style utilities.
- **Filter in place to avoid an allocation.** `s = s[:0]` reuses the backing array; appending back into it is safe because you only ever write at an index `≤` the one you're reading.
- **The `slices` package is a bag of loops you no longer write:** `Contains`/`Index`/`IndexFunc`/`ContainsFunc` (membership & position), `Max`/`Min` (ordered) and `MaxFunc`/`MinFunc` (return the *element* by a comparator), `Compact` (drop **consecutive** duplicates — sort first for a full dedup), `Concat` (flatten a fixed set of slices), `Clone`, `Equal`/`EqualFunc`, `Reverse`, `Insert`/`Delete`.
- **Maps are the workhorse of data shaping.** Four shapes cover most real work:
  - **GroupBy** → `map[K][]T`: `m[key(v)] = append(m[key(v)], v)` (append to a missing key's `nil` just works).
  - **CountBy** → `map[K]int`: `m[key(v)]++` (a missing key reads as `0`).
  - **KeyBy / index** → `map[K]T`: one row per key for O(1) lookup instead of a scan.
  - **Set** → `map[T]struct{}`: `struct{}` is zero bytes; membership is `_, ok := m[v]`.
- **Set algebra is three small loops.** **Union** = insert all of both into a set; **intersection** = keep `b`'s elements that are in `a`'s set; **difference** = keep `a`'s elements *not* in `b`'s set. All O(n+m).
- **Partition / chunk / flatten / zip** round out the toolkit: partition splits into (matching, non-matching) in one pass; chunk batches into fixed-size slices (`slices.Chunk` in 1.23 returns an iterator); flatten is `append(out, row...)` per row or `slices.Concat`; zip pairs two slices up to the shorter length.
- **Iterators (Go 1.23) make pipelines lazy.** An `iter.Seq[T]` is just `func(yield func(T) bool)`; `for v := range seq` drives it. `slices.Values`/`maps.Keys`/`maps.Values` produce them; `slices.Collect`/`slices.Sorted` consume them back to a slice. A lazy `Filter`/`Map` over `iter.Seq` only does work for the elements actually pulled — so you can even take a finite prefix of an **infinite** sequence.
  ```go
  evens := FilterSeq(slices.Values(nums), isEven)  // nothing runs yet
  out := slices.Collect(MapSeq(evens, square))      // pulls, filters, maps, collects
  ```
- **Map iteration order is random — always sort before you emit.** Every group-by / count-by result must have its keys collected and sorted before printing or returning, or your output (and your tests) will be non-deterministic.

## Exercises
1. Filter a slice two ways: into a **new** slice, and **in place** with `s[:0]`. Confirm both give the same result.
2. Map a `[]int` to `[]int` (double) and to `[]string` (`"#" + Itoa`). Write a generic `Map[T, U]` and re-do it.
3. Reduce a slice to a sum, a product, and a max — seeding max with `s[0]`, not `0`. Then write a generic `Reduce[T, U]` that folds into a `map[string]int` count.
4. Use `slices`: `Contains`, `IndexFunc`, `ContainsFunc`, `Max`/`Min`, `Equal`. Then dedup a slice with `Sort`+`Compact` and, separately, with an order-preserving set.
5. GroupBy (`map[K][]T`), CountBy (`map[K]int`), KeyBy (`map[K]T`) over a `[]struct`. Print each with **sorted keys**.
6. Build a generic `Set[T comparable]` (`Add`/`Has`/`Slice`), then `Union`/`Intersection`/`Difference` over two `[]int`.
7. Write `Partition`, `Chunk`, `Flatten` (+ `slices.Concat`), and `Zip` as generic helpers.
8. `MaxFunc`/`MinFunc` to find the priciest/cheapest struct; `UniqueBy` to dedup structs by a field keeping the first.
9. Emit a map deterministically: `slices.Sorted(maps.Keys(m))`, then sort keys by **value** with `SortFunc` + `cmp.Or`.
10. Iterators: write lazy `FilterSeq`/`MapSeq` over `iter.Seq`, consume with range-over-func and `slices.Collect`; add a `Take(n)` and prove it stops the upstream early.
11. Capstone: `[]Order` → filter paid → group by customer → aggregate (count/total/avg) → sort by total DESC → print a table.

## Best Practices & Pitfalls
- **A `for` loop is idiomatic — don't force a functional style.** Reach for a `Map`/`Filter` helper when the same shape repeats across a file; a one-off transform reads fine as a loop.
- **Preallocate when you know the size.** `make([]U, len(s))` for a map, `make([]T, 0, len(s))` for a filter — avoids repeated `append` growth.
- **Pitfall — `slices.Compact` only removes *adjacent* duplicates.** `[]int{3,1,3}` stays `[3,1,3]`. Sort first, or use a set.
- **Pitfall — map iteration order is random.** Collect keys into a slice and `slices.Sort` them before printing/returning. This bites group-by, count-by, and any map-to-output code constantly.
- **Pitfall — mutating while ranging.** The in-place filter (`s[:0]`) is safe only because writes never outrun reads; don't `append` to the *same* slice you're ranging in the general case.
- **Pitfall — `MaxFunc`/`MinFunc` (and `Max`/`Min`) panic on an empty slice.** Guard `len(s) == 0` first if the input can be empty.
- **Set values are `struct{}{}`, not `true`.** `map[T]bool` works but wastes a byte per entry and invites the `m[k]` (which is `false` for missing *and* for explicitly-false) ambiguity. Prefer `map[T]struct{}` + comma-ok.
- **Iterators are lazy — side effects fire on *consumption*.** Building a `FilterSeq`/`MapSeq` chain runs nothing; the work happens when you range over it or `Collect` it, and only for elements actually pulled.

## Checklist
- [ ] I can write filter / map / reduce as loops and as generic helpers, and I know why `Map` needs two type parameters.
- [ ] I reach for `slices`/`maps` (`Contains`, `IndexFunc`, `MaxFunc`, `Compact`, `Concat`, `Clone`, `Equal`, `Keys`) instead of re-writing loops.
- [ ] I use `map[K][]T` (group), `map[K]int` (count), `map[K]T` (key-by), and `map[T]struct{}` (set) fluently.
- [ ] I can implement union / intersection / difference and partition / chunk / flatten / zip / unique-by.
- [ ] I always sort map keys before emitting, and I guard `MaxFunc`/`MinFunc` against empty input.
- [ ] I can build a lazy `iter.Seq` filter/map pipeline and collect it with `slices.Collect`/`Sorted`.
- [ ] I can compose the toolkit into a filter → group → aggregate → sort report.

## Resources
- `slices` package: https://pkg.go.dev/slices
- `maps` package: https://pkg.go.dev/maps
- `iter` package (iterators): https://pkg.go.dev/iter
- `cmp` package (`Compare`, `Or`): https://pkg.go.dev/cmp
- Go blog — "Range Over Function Types" (1.23 iterators): https://go.dev/blog/range-functions
- Go wiki — "No, Go does not have a `map`/`filter`/`reduce`; here's why & how": https://go.dev/doc/effective_go
- Examples: [examples/54-collection-operations](examples/54-collection-operations/).
- Related in this plan: ordering & searching in [51 — Sorting & Searching](51-sorting-searching.md); applying this toolkit to real JSON/CSV/DB data in [55 — Data Pipelines](55-data-pipelines.md); generics in [17 — Generics](17-generics.md).
