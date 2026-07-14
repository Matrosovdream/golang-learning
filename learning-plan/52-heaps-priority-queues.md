# 52 — Heaps & Priority Queues

> Part of **Part 10 — Data Structures**, alongside [42 — Trees](42-trees.md), [50 — Linear Structures](50-linear-structures.md), and [51 — Sorting & Searching](51-sorting-searching.md). Builds on [42](42-trees.md) (a heap is a tree stored in a slice), [50](50-linear-structures.md) (it's the priority version of a queue), and [17 — Generics](17-generics.md). Thesis: **a heap is the data structure for "give me the smallest/largest, over and over" — and in Go that means `container/heap`: implement one small interface and you get an O(log n) priority queue that powers top-K, merge-K, schedulers, and Dijkstra.**

## Goals
- Understand a **binary heap**: a complete tree stored in a **slice**, with parent/child **index math** and the **heap property**.
- Write the two core operations — **sift up** (insert) and **sift down** (extract) — and build a heap from scratch, including O(n) **heapify**.
- Drive Go's **`container/heap`**: implement `heap.Interface`, and use `heap.Init/Push/Pop/Fix/Remove` — knowing why you call the **package functions**, not your own `Push`/`Pop` methods.
- Build a **priority queue** (min or max), update priorities with `heap.Fix`, and remove arbitrary items with `heap.Remove`.
- Apply heaps to the problems they own: **top-K** / k-th largest (O(n log k)), **merge k sorted** streams, **running median** (two heaps), **Dijkstra's** shortest path, scheduling, and **Huffman coding**.

## Concepts

- **A heap is an array, not a pointer tree.** A binary heap is a *complete* binary tree, so it packs perfectly into a slice with no pointers. For the node at index `i`:
  ```go
  parent := (i - 1) / 2
  left   := 2*i + 1
  right  := 2*i + 2
  ```
  This implicit layout is cache-friendly and allocation-free — one reason heaps are fast.
- **The heap property is local.** In a **min-heap**, every parent is `≤` both children (so the minimum is always at index 0); a **max-heap** flips that to `≥`. The property says nothing about left-vs-right or across subtrees — that's why a heap is *not* sorted, and why it's cheaper to maintain than a BST.
- **Two operations do everything.** **Sift up**: after appending at the end, swap the new node upward while it's smaller than its parent — O(log n) insert. **Sift down**: after moving the last element to the root, swap it downward with its smaller child until the property holds — O(log n) extract-min. Every heap method is one of these.
- **Heapify is O(n), not O(n log n).** Building a heap from an existing slice by sifting down every non-leaf node (from the last parent back to the root) is linear — cheaper than `n` individual inserts. That's what `heap.Init` does.
- **`container/heap` is interface-based.** You implement **`heap.Interface`** = `sort.Interface` (`Len`, `Less`, `Swap`) **plus** `Push(x any)` and `Pop() any`. `Less` decides min- vs max-heap. Your `Push`/`Pop` methods just append to / trim the **end** of the slice — they are the low-level hooks, **not** the operations you call.
- **Call the package functions, not the methods.** `heap.Push(h, x)` calls *your* `Push` to append, then **sifts up**. `heap.Pop(h)` swaps the root to the end, calls *your* `Pop` to trim, then **sifts down** the new root and returns the min. Calling `h.Push(x)` / `h.Pop()` directly bypasses the sifting and **corrupts the heap** — a classic bug.
- **`Fix` and `Remove` handle changes and deletions.** Change an item's priority in place, then `heap.Fix(h, i)` re-establishes the invariant in O(log n). `heap.Remove(h, i)` deletes the element at index `i`. To use them you need the element's **current index** — production priority queues store an `index` field and keep it updated inside `Swap`.
- **Priorities are separate from values.** A **priority queue** orders by an explicit priority (a struct field), not the element's natural order. Min-heap on priority = "lowest number served first"; flip `Less` for "highest first".
- **The size-k heap trick.** For **top-K largest**, keep a **min-heap of size k**: push everything, and whenever it exceeds `k`, pop the smallest. What survives is the k largest, in **O(n log k)** time and **O(k)** memory — the streaming-friendly win over sorting everything (which was [51](51-sorting-searching.md)'s approach).
- **Complexity:** peek min/max = **O(1)**; push / pop = **O(log n)**; build-heap (`Init`) = **O(n)**; heap sort = **O(n log n)**. A heap gives you the *extremes* cheaply but does **not** support fast search or ordered iteration — reach for a sorted slice or a tree ([42](42-trees.md)) when you need those.
- **No generic heap in the stdlib (yet).** `container/heap` predates generics and stores `any`, so you type-assert on `Pop`. For type safety you either wrap it or hand-roll a small generic heap (a slice + a `less` func) — both shown in the examples.

## Exercises
1. Write `parent`/`left`/`right` index helpers and an `isMinHeap` check; confirm a valid heap slice passes and a broken one fails.
2. Implement `siftUp` and `siftDown`, then a hand-rolled `MinHeap` with `Insert`/`ExtractMin`/`Peek`; extract everything and watch it come out sorted.
3. Turn it into a **max-heap** by flipping the comparisons; then write an O(n) `heapify` that builds a heap from an arbitrary slice bottom-up.
4. Implement the canonical `container/heap` `IntHeap` (min-heap). Use `heap.Init`, then `heap.Push`/`heap.Pop`, and articulate why you never call the methods directly.
5. Make a **max-heap** with `container/heap` (flip `Less`), and write **heap sort** (init + pop all).
6. Build a **priority-queue** of tasks ordered by priority; use `heap.Fix` to bump one task's priority and `heap.Remove` to cancel another.
7. Write a **generic** min-heap (`Heap[T]` with a `less` function) and use it for ints and for a struct type.
8. Top-K: return the k most frequent elements and the k-th largest with a **size-k heap** (O(n log k)); compare with the sort approach from [51](51-sorting-searching.md).
9. Merge **k sorted slices** with a heap of cursors (O(N log k)).
10. Stretch — pick two: **running median** with two heaps, **Dijkstra's** shortest path with a priority queue, **meeting rooms** (min rooms via a min-heap of end times), **k closest points**, or **Huffman coding** (build the prefix tree with a min-heap).

## Best Practices & Pitfalls
- **Reach for `container/heap` before hand-rolling.** It's tested, correct, and O(log n). Write the sift functions once to *understand* them; use the stdlib in real code.
- **Pitfall — calling `h.Push`/`h.Pop` instead of `heap.Push`/`heap.Pop`.** The methods only touch the slice's end and skip the sifting; the package functions maintain the invariant. Mixing them up silently corrupts the heap. Make `Push`/`Pop` **pointer receivers** (they resize the slice); `Len`/`Less`/`Swap` can be value receivers.
- **Pitfall — using a heap to find the k *smallest* with a min-heap of size k.** It's backwards: for the k largest use a **min-heap** (evict the smallest); for the k smallest use a **max-heap** (evict the largest). Getting this inverted is the most common heap bug.
- **`Fix`/`Remove` need the live index.** Scanning to find it is O(n) and defeats the point; store an `index` field on each item and update it in `Swap` so `Fix`/`Remove` stay O(log n). (This is the "indexed priority queue" pattern Dijkstra uses.)
- **Lazy deletion is often simpler than decrease-key.** In Dijkstra, pushing a node again and **skipping stale pops** (`if d > dist[u] { continue }`) is easier and usually as fast as maintaining `Fix`/index bookkeeping.
- **A heap is not sorted and not searchable.** Iterating the backing slice is *not* in priority order, and membership/lookup is O(n). Don't reach for a heap when you actually need ordered traversal or search.
- **Pitfall — non-deterministic output on ties.** Equal-priority items can pop in any order; add a tiebreaker to `Less` (e.g. insertion sequence or a secondary key) when you need reproducible results.

## Checklist
- [ ] I can do the parent/child index math and state the min- vs max-heap property.
- [ ] I can implement sift up / sift down and a hand-rolled heap, and explain why heapify is O(n).
- [ ] I can implement `heap.Interface` and know why I call `heap.Push`/`heap.Pop`, not the methods.
- [ ] I can build a priority queue and update it with `heap.Fix` / `heap.Remove`.
- [ ] I use a size-k **min-**heap for the k **largest** (and vice-versa) and know it's O(n log k).
- [ ] I can merge k sorted streams, maintain a running median with two heaps, and run Dijkstra with a PQ.
- [ ] I reach for `container/heap` in real code and know a heap gives extremes cheaply but not search or order.

## Resources
- `container/heap` (Interface, Init/Push/Pop/Fix/Remove + the PriorityQueue example): https://pkg.go.dev/container/heap
- Go source — `container/heap` example priority queue: https://cs.opensource.google/go/go/+/refs/tags/go1.22.0:src/container/heap/example_pq_test.go
- Wikipedia — Binary heap (index math, sift up/down, build-heap): https://en.wikipedia.org/wiki/Binary_heap
- Wikipedia — Dijkstra's algorithm: https://en.wikipedia.org/wiki/Dijkstra%27s_algorithm
- Examples: [examples/52-heaps-priority-queues](examples/52-heaps-priority-queues/).
- Related in this plan: the array-as-tree view in [42 — Trees](42-trees.md); top-K by sorting in [51 — Sorting & Searching](51-sorting-searching.md); the queue it generalises in [50 — Linear Structures](50-linear-structures.md).
