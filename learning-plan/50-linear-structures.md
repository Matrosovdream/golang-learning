# 50 — Linear Structures: Stacks, Queues, Deques & Linked Lists

> Part of **Part 10 — Data Structures**, alongside [42 — Trees](42-trees.md). Builds on [07 — Slices & Maps](07-slices-maps.md) (the slice is the workhorse here), [10 — Pointers & Methods](10-pointers-methods.md) (linked-list nodes), and [17 — Generics](17-generics.md) (`Stack[T]`, `Queue[T]`). The one-line thesis: **in Go a slice already *is* your stack, queue, and deque — reach for a linked list or `container/list` only when you have a specific reason.**

## Goals
- Use a **slice as a stack** (LIFO): push with `append`, pop by reslicing, peek the top.
- Use a **slice as a queue** (FIFO), understand the **reslicing memory pitfall**, and fix it with a head index or a **ring buffer**.
- Build a **deque** (double-ended queue) and know when the stdlib's `container/list` / `container/ring` earn their keep.
- Build **singly** and **doubly linked lists** with pointer structs; reverse one, detect a cycle, merge two.
- Reach for the right linear structure in real problems: balanced brackets, RPN, min-stack, monotonic stack/deque, **LRU cache**, and the **channel as Go's built-in concurrent queue**.

## Concepts

- **A slice is a stack.** Push is `append`; the top is the last element; pop is a reslice. No type needed for the simplest cases.
  ```go
  stack := []int{}
  stack = append(stack, 1, 2, 3) // push
  top := stack[len(stack)-1]     // peek -> 3
  stack = stack[:len(stack)-1]   // pop  -> stack is [1 2]
  ```
  LIFO falls out for free because `append` and "last index" both work at the tail — the cheap end of a slice.

- **A slice is also a queue, but the naive dequeue leaks.** Enqueue at the tail (`append`); dequeue from the head (`q = q[1:]`). The trap: `q[1:]` only moves the slice *header* forward — the backing array keeps growing and the popped elements are **never freed** (they're still reachable through the array). A long-lived queue driven this way grows without bound.
  ```go
  q = append(q, x) // enqueue (tail)
  front := q[0]    // peek
  q = q[1:]        // dequeue — BUT the backing array never shrinks
  ```
  Two fixes: **(a)** track a `head` index and periodically **compact** (`copy` the live tail to the front, reset `head`); **(b)** use a fixed-capacity **ring buffer**. Both keep memory bounded.

- **A ring buffer** is a fixed slice plus `head`, `tail`, and `count`; indices wrap with `% cap`. It's O(1) enqueue/dequeue with **zero per-operation allocation** and bounded memory — the structure behind bounded work queues and audio/network buffers.
  ```go
  i := (r.head + k) % len(r.buf) // wrap-around indexing
  ```

- **A deque** (double-ended queue) supports push/pop at *both* ends. A slice handles the tail cheaply; the head is O(n) if you insert there, so for heavy front-and-back work use a ring buffer or `container/list`.

- **A linked list is a pointer struct — the same shape as a tree node, minus a child.** `nil` is the empty list and every base case, exactly like [42 — Trees](42-trees.md).
  ```go
  type node[T any] struct {
      val  T
      next *node[T]
  }
  ```
  You **prepend** in O(1) (new head points at the old head). Reversal, cycle detection, and merging are all pointer-rewiring exercises — the classic interview set, and the reason to understand pointers cold.

- **The stdlib gives you two ready-made linked structures.** `container/list` is a **doubly linked list** (O(1) insert/remove at any element you hold) — the backbone of an **LRU cache**. `container/ring` is a **circular** doubly linked list of fixed size. Both are pre-generics (`any` values), so you type-assert on the way out.

- **The Go-native queue is the channel.** A **buffered channel** is a concurrent, blocking FIFO with backpressure built in — `ch <- x` enqueues, `<-ch` dequeues, and the runtime handles the locking. For cross-goroutine hand-off you almost never build a mutex-guarded queue; you use a channel ([14 — Channels](14-channels.md)).

- **Complexity you should know by heart:** slice stack push/pop = **amortized O(1)**; slice-queue dequeue via `q[1:]` = O(1) time but **unbounded memory**; ring buffer = O(1) time + O(1) memory; linked-list prepend / `container/list` remove = O(1); indexing a linked list = O(n) (no random access — that's the slice's job).

## Exercises
1. Use a `[]int` as a stack: push `1,2,3`, peek the top, pop twice, and print the stack after each step.
2. Write a generic `Stack[T]` with `Push`, `Pop() (T, bool)`, `Peek`, `Len`, `IsEmpty`; use it to reverse a slice.
3. Use a `[]string` as a FIFO queue; then reproduce the reslicing memory issue in words and rewrite it with a `head` index that compacts when `head` grows large.
4. Implement a fixed-capacity **ring buffer** queue (`Enqueue`/`Dequeue`/`Len`/`Full`) using `head`, `tail`, `count`, and `% cap` wrapping.
5. Implement a slice-backed **deque** with `PushFront`/`PushBack`/`PopFront`/`PopBack`.
6. Write `balanced(s string) bool` using a stack to match `()[]{}`.
7. Evaluate a **reverse-Polish** (postfix) expression like `["3","4","+","2","*"]` with a stack.
8. Build a singly linked list (`prepend`, `append`, `length`, `find`); then **reverse** it iteratively by rewiring `next` pointers.
9. Use `container/list` as a queue/deque, and `container/ring` to rotate a fixed circular buffer.
10. Stretch — pick two: a **min-stack** (O(1) `Min`), a **queue from two stacks**, a **monotonic stack** (next-greater-element), **sliding-window maximum** with a monotonic deque, an **LRU cache** (`container/list` + `map`), **Floyd's cycle detection**, or **merge two sorted lists**.

## Best Practices & Pitfalls
- **Default to a slice.** For a stack, queue, or deque, a `[]T` is simpler, faster, and more cache-friendly than any linked structure. Write a linked list to *learn* pointers; use one in production only when you need O(1) splice/remove at a held position (→ `container/list`) or a genuine LIFO/FIFO abstraction.
- **Pitfall — the growing slice-queue.** `q = q[1:]` never frees the head; a long-running dequeue loop leaks the whole backing array. Use a head index + compaction, a ring buffer, or a channel. (Same family as the [07](07-slices-maps.md) "slice keeps its backing array alive" gotcha.)
- **Pop must report emptiness.** Return `(T, bool)` (or check `len` first). `stack[len(stack)-1]` on an empty slice **panics** — an out-of-range index, not a `nil` you can test.
- **Zero-value-ready over constructors.** A `Stack[T]` whose zero value (`var s Stack[int]`) is an empty, usable stack is the idiomatic Go design — no `NewStack` required (→ [24 — Idiomatic Go](24-idiomatic-go.md), [28 — Creational Patterns](28-patterns-creational.md)).
- **Nil the popped pointer** in a pointer-holding stack/queue (`buf[i] = nil` after removing) so the GC can collect the element; otherwise the slot pins it.
- **`container/list`/`container/ring` hold `any`.** You type-assert on the way out (`e.Value.(T)`), losing compile-time type safety — one reason a hand-rolled generic list or a plain slice is often nicer today.
- **Don't hand-roll a concurrent queue first.** For goroutine-to-goroutine work, a **buffered channel** is the queue — it already has the lock and the backpressure. Reach for a mutex-guarded structure only when channels genuinely don't fit ([15 — Sync & Context](15-sync-context.md)).
- **Pitfall — shadowing builtins.** `len`, `cap`, `min`, `max` are predeclared; name fields/methods `Len()`, not `len`.

## Checklist
- [ ] I can use a slice as a stack (push/peek/pop) and explain why the tail is the cheap end.
- [ ] I can write a generic `Stack[T]`/`Queue[T]` whose zero value is ready to use, with a `(T, bool)` pop.
- [ ] I can explain the slice-queue memory leak and fix it with a head index or a ring buffer.
- [ ] I can implement a ring-buffer queue with `% cap` wrapping and a deque with both ends.
- [ ] I can build a singly linked list and reverse it, detect a cycle (Floyd), and merge two sorted lists.
- [ ] I can use `container/list` for an LRU cache and know when a slice or a channel is the better choice.

## Resources
- `container/list` (doubly linked list): https://pkg.go.dev/container/list
- `container/ring` (circular list): https://pkg.go.dev/container/ring
- Go Slices — usage & internals (the reslicing/backing-array model): https://go.dev/blog/slices-intro
- Effective Go — slices as stacks/queues idioms: https://go.dev/doc/effective_go
- Go by Example — Channels (Go's built-in queue): https://gobyexample.com/channels
- Examples: [examples/50-linear-structures](examples/50-linear-structures/).
- Related in this plan: pointer structs & `nil`-base recursion in [42 — Trees](42-trees.md); the slice model in [07 — Slices & Maps](07-slices-maps.md); channels as queues in [14 — Channels](14-channels.md).
