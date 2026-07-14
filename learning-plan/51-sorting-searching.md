# 51 — Sorting, Searching & the Two-Pointer / Sliding-Window Toolkit

> Part of **Part 10 — Data Structures**, alongside [42 — Trees](42-trees.md) and [50 — Linear Structures](50-linear-structures.md). Builds on [07 — Slices & Maps](07-slices-maps.md), [17 — Generics](17-generics.md) (the modern `slices`/`cmp` are generic), and pairs with [42](42-trees.md)'s `slices.BinarySearch` note. Thesis: **reach for the `slices`/`sort` stdlib first — then learn the handful of pointer techniques (two-pointer, sliding window, binary-search-on-a-boundary) that turn O(n²) scans into O(n) or O(log n).**

## Goals
- Sort anything with the modern generic **`slices`** package (Go 1.21+) and the classic **`sort`** package, including **custom comparators** and **multi-key** ordering with `cmp.Or`.
- Know **stable vs unstable** sort and when the difference matters.
- Search a sorted slice with **`slices.BinarySearch`** / **`sort.Search`**, and use `sort.Search` for **boundary** queries (lower/upper bound).
- Apply the **two-pointer** pattern (pair-sum, in-place dedup, palindrome, three-way partition) and the **sliding-window** pattern (fixed and variable size).
- Use **maps as sets and frequency counters**, and combine counting + sorting for group-by and top-K problems.

## Concepts

- **`slices.Sort` is the default sort now.** For any slice of ordered values it's one call, in place, ascending. Descending = sort then `slices.Reverse`, or a comparator.
  ```go
  slices.Sort(nums)              // ascending
  slices.Reverse(nums)           // -> descending
  ```
- **Custom order = `slices.SortFunc` + `cmp.Compare`.** The comparator returns a negative / zero / positive `int`; `cmp.Compare(a, b)` produces exactly that for any ordered type and avoids the `a - b` **integer-overflow** trap. **Multi-key** sorts compose with `cmp.Or`, which returns the first non-zero result:
  ```go
  slices.SortFunc(rows, func(a, b Row) int {
      return cmp.Or(
          cmp.Compare(a.Team, b.Team),      // primary
          cmp.Compare(b.Score, a.Score),    // then score DESC (operands flipped)
          cmp.Compare(a.Name, b.Name),      // tiebreak
      )
  })
  ```
- **Stable vs unstable.** `slices.Sort`/`SortFunc` are **not** guaranteed to keep equal elements in their original order; `slices.SortStableFunc` (and `sort.Stable`) are. Use stable when you sort by one key and want a previous ordering preserved within ties.
- **Binary search needs a sorted slice.** `slices.BinarySearch` returns `(index, found)`; when not found, `index` is the **insertion point** that keeps the slice sorted — pair it with `slices.Insert`. `slices.BinarySearchFunc` searches by a key/comparator.
- **`sort.Search` is binary search on a boundary.** Given a predicate that is `false…false,true…true`, it returns the **first index where the predicate is true** — i.e. a lower bound. `lower = Search(n, x>=)`, `upper = Search(n, x>)`, and `upper-lower` is the count of `x`. This generalises to "binary search the answer".
- **Two pointers turn many O(n²) scans into O(n)** on sorted (or partitionable) data:
  - **Converging** (one at each end): pair-sum in a sorted slice, palindrome check, container problems.
  - **Fast/slow** (both from the left): in-place dedup, remove/partition, cycle detection ([50](50-linear-structures.md)).
  - **Three-way partition** (Dutch national flag): sort 0/1/2 in a single pass.
- **Sliding window** keeps a running summary of a contiguous range instead of recomputing it:
  - **Fixed size** — add the entering element, subtract the leaving one (max sum of `k`).
  - **Variable size** — grow the right edge; shrink the left while a condition holds (longest substring without repeats, shortest subarray with a large-enough sum). Each index enters and leaves at most once → O(n).
- **Maps are Go's set and frequency counter.** A **set** is `map[T]struct{}` (`struct{}` costs zero bytes); membership is `_, ok := m[v]`. A **frequency count** is `m[v]++` (a missing key reads as the zero value `0`). Counting + a sort is the backbone of group-by, most-frequent, and top-K.
- **Complexity to keep in mind:** comparison sort = **O(n log n)**; binary search = **O(log n)** but only on sorted data; two-pointer / sliding window = **O(n)**; quickselect (k-th element) = **O(n)** average; a map lookup = **O(1)** average and needs no ordering at all.

## Exercises
1. Sort `[]int` and `[]string` with `slices.Sort`; produce a descending order two ways (`Reverse`, and a `SortFunc` comparator).
2. Sort a `[]struct` by one field with `slices.SortFunc`/`cmp.Compare`, then by **two** fields with `cmp.Or` (second key descending).
3. Show the difference between `slices.SortFunc` and `slices.SortStableFunc` on data with equal keys.
4. Use `slices.BinarySearch` to find a value and to get the insertion point for one that's absent; insert it with `slices.Insert`.
5. Use `sort.Search` to compute the `[lower, upper)` index range of a repeated value in a sorted slice, and its count.
6. Implement `sort.Interface` (`Len`/`Less`/`Swap`) on a custom type and sort it with `sort.Sort`; then reverse with `sort.Reverse`.
7. Two-pointer: find a pair summing to a target in a **sorted** slice; remove duplicates from a sorted slice **in place**; check a palindrome ignoring case/punctuation.
8. Build a `Set[T comparable]` on `map[T]struct{}` (Add/Has/Len + intersection); build a word-frequency counter and print entries sorted by count.
9. Sliding window: max sum of a fixed window `k`; then the **variable-size** longest-substring-without-repeats and shortest-subarray-with-sum-≥-target.
10. Stretch — pick two: **two-sum** on unsorted input via a map, **group anagrams**, **Dutch national flag**, **quickselect** (k-th smallest), **merge sort** from scratch, **merge overlapping intervals**, or **top-K frequent** elements.

## Best Practices & Pitfalls
- **Default to `slices`/`sort`.** Hand-write a sort only to *understand* it (merge sort, quicksort); in real code the stdlib's introsort is faster and correct. Reach for `slices` (generic, 1.21+) in new code; you'll still read `sort.Slice`/`sort.Interface` everywhere in existing code.
- **Comparators return `int`, not `bool`.** `slices.SortFunc` wants `-1/0/+1` (use `cmp.Compare`); `sort.Slice`/`sort.Interface.Less` want a `bool` `a < b`. Mixing them up is a common slip.
- **Pitfall — `a - b` comparators overflow.** For `int` keys near the limits, `return a - b` can wrap and mis-sort. Use `cmp.Compare(a, b)`.
- **Pitfall — non-transitive / inconsistent `Less`.** A comparator must define a strict weak ordering (if `a<b` and `b<c` then `a<c`, and `a<b` implies `!(b<a)`). A `Less` that returns `<=` or contradicts itself makes the sort's output undefined and can even panic.
- **Binary search on unsorted data is silently wrong**, not an error. Guarantee the slice is sorted by the *same* key the search compares.
- **Pitfall — map iteration order is random.** Never print/return map results without sorting the keys first if the output must be stable (tests, golden files). This bites frequency/group-by code constantly.
- **Sliding window needs a monotonic shrink condition.** The left pointer must only ever move right; if shrinking can "undo", the window isn't valid and you need a different structure (a monotonic deque — [50](50-linear-structures.md) ex. 21).
- **Prefer a map to sorting when you don't need order.** Two-sum, dedup of unsorted data, and membership are O(n)/O(1) with a map — sorting first is O(n log n) you may not need.

## Checklist
- [ ] I can sort with `slices.Sort`/`SortFunc` and the classic `sort.Slice`/`sort.Interface`, and build multi-key orders with `cmp.Or`.
- [ ] I know stable vs unstable sort and when it matters.
- [ ] I can binary-search with `slices.BinarySearch`/`BinarySearchFunc` and use `sort.Search` for lower/upper bounds.
- [ ] I can apply converging and fast/slow two-pointer techniques (pair-sum, dedup, palindrome, Dutch flag).
- [ ] I can write fixed and variable-size sliding windows and explain why they're O(n).
- [ ] I use `map[T]struct{}` as a set and `m[v]++` as a counter, and I sort before emitting map results.
- [ ] I reach for the stdlib first and know the complexity of each tool.

## Resources
- `slices` package (Sort, SortFunc, BinarySearch, …): https://pkg.go.dev/slices
- `cmp` package (`Compare`, `Or`, `Ordered`): https://pkg.go.dev/cmp
- `sort` package (`Slice`, `Interface`, `Search`, `Stable`): https://pkg.go.dev/sort
- Go blog — "Sorting with slices and cmp": https://go.dev/blog/comparable
- Go by Example — Sorting & Sorting by Functions: https://gobyexample.com/sorting · https://gobyexample.com/sorting-by-functions
- Examples: [examples/51-sorting-searching](examples/51-sorting-searching/).
- Related in this plan: `slices.BinarySearch` as a tree alternative in [42 — Trees](42-trees.md); the monotonic-deque sliding window in [50 — Linear Structures](50-linear-structures.md); heaps for top-K in [52 — Heaps & Priority Queues](52-heaps-priority-queues.md).
