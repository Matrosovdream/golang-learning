# Step 52 — Heaps & Priority Queues · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟢 easy](1-easy.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

This tier uses the standard library's `container/heap`.

---

## 9. container/heap: the IntHeap

`🟡 medium` · *container/heap*

Go's heap lives in `container/heap`. You don't get a ready-made type — you implement **`heap.Interface`**: `Len`, `Less`, `Swap` (that's `sort.Interface`) plus `Push(x any)` and `Pop() any`. `Less` with `<` makes it a **min-heap**. The `Push`/`Pop` methods just append to / trim the slice's **end** — they're low-level hooks, not what you call (example 10).

**Steps:**

1. Define a slice type and implement the five methods (`Push`/`Pop` need pointer receivers).
2. `heap.Init(h)` establishes the invariant on the starting data.
3. The min is always at index 0.

```go
package main

import (
	"container/heap"
	"fmt"
)

// IntHeap implements heap.Interface, which is sort.Interface (Len/Less/Swap)
// plus Push/Pop. Less with < makes it a MIN-heap. The Push/Pop methods operate
// on the slice's END — they are NOT what you call directly (see example 10).
type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func main() {
	h := &IntHeap{2, 1, 5}
	heap.Init(h) // establish the heap invariant on the initial data
	fmt.Println("min:", (*h)[0])
	heap.Push(h, 3)
	fmt.Println("min after push 3:", (*h)[0])
}
```

**Output:**

```
min: 1
min after push 3: 1
```

---

## 10. heap.Push and heap.Pop

`🟡 medium` · *container/heap*

The single most important thing about `container/heap`: **call the package functions, not the methods.** `heap.Push(h, x)` calls your `Push` to append, *then sifts up*. `heap.Pop(h)` swaps the root to the end, calls your `Pop` to trim, *then sifts down* and returns the min. Calling `h.Push`/`h.Pop` directly skips the sifting and **corrupts the heap**.

**Steps:**

1. Push six values with `heap.Push` — each is sifted into place.
2. Pop them all with `heap.Pop` — they emerge smallest-first.
3. Never call `h.Push`/`h.Pop` yourself.

```go
package main

import (
	"container/heap"
	"fmt"
)

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func main() {
	// Call the PACKAGE functions heap.Push/heap.Pop, never the methods directly.
	// heap.Push appends via your Push method, THEN sifts up. heap.Pop swaps the
	// root to the end, trims via your Pop method, THEN sifts down. The methods
	// only touch the end of the slice; the package functions keep the invariant.
	h := &IntHeap{}
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		heap.Push(h, v)
	}
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ") // comes out sorted (min first)
	}
	fmt.Println()
}
```

**Output:**

```
1 2 3 5 8 9 
```

---

## 11. heap.Init on existing data

`🟡 medium` · *container/heap*

When you already hold the data, `heap.Init` heapifies the whole slice in **O(n)** — cheaper than pushing one element at a time (O(n log n)). After `Init`, the slice satisfies the heap property and you can pop in sorted order.

**Steps:**

1. Fill the heap's slice directly, then call `heap.Init(h)`.
2. The internal array is now a valid heap (not sorted — just heap-ordered).
3. Popping drains it smallest-first.

```go
package main

import (
	"container/heap"
	"fmt"
)

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func main() {
	// When you already have the data, heap.Init heapifies it in O(n) — cheaper
	// than pushing one at a time.
	h := &IntHeap{9, 4, 7, 1, 8, 3}
	heap.Init(h)
	fmt.Println("heapified:", *h)
	fmt.Print("popped order: ")
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ")
	}
	fmt.Println()
}
```

**Output:**

```
heapified: [1 4 3 9 8 7]
popped order: 1 3 4 7 8 9 
```

---

## 12. A max-heap with container/heap

`🟡 medium` · *container/heap*

To make `container/heap` a **max-heap**, flip `Less` to `>`. Everything else is identical — the sifting logic just follows whatever ordering `Less` defines. Now the largest element sits at the root and pops come out descending.

**Steps:**

1. `Less(i, j)` returns `h[i] > h[j]`.
2. Push values with `heap.Push`.
3. Pop them → descending order.

```go
package main

import (
	"container/heap"
	"fmt"
)

// Flip Less to > and container/heap gives a MAX-heap: the largest is at the root.
type MaxHeap []int

func (h MaxHeap) Len() int           { return len(h) }
func (h MaxHeap) Less(i, j int) bool { return h[i] > h[j] } // > = max-heap
func (h MaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *MaxHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *MaxHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func main() {
	h := &MaxHeap{}
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		heap.Push(h, v)
	}
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ") // descending
	}
	fmt.Println()
}
```

**Output:**

```
9 8 5 3 2 1 
```

---

## 13. Heap sort

`🟡 medium` · *Heap sort*

A heap gives a sort almost for free: `heap.Init` the data (O(n)), then pop the min repeatedly (each O(log n)) → O(n log n) overall, in place on the heap's slice. In real code you'd call `slices.Sort` ([51](../51-sorting-searching/)); this shows *why* a heap sorts.

**Steps:**

1. Copy the input into the heap and `heap.Init`.
2. Pop everything into an output slice.
3. The result is sorted ascending.

```go
package main

import (
	"container/heap"
	"fmt"
)

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

// heapSort sorts by heapifying then popping the min repeatedly — O(n log n).
func heapSort(nums []int) []int {
	h := &IntHeap{}
	*h = append(*h, nums...)
	heap.Init(h)
	out := make([]int, 0, len(nums))
	for h.Len() > 0 {
		out = append(out, heap.Pop(h).(int))
	}
	return out
}

func main() {
	fmt.Println(heapSort([]int{5, 2, 8, 1, 9, 3, 7, 4}))
}
```

**Output:**

```
[1 2 3 4 5 7 8 9]
```

---

## 14. A priority queue

`🟡 medium` · *Priority queue*

A priority queue orders by an **explicit priority**, not the element's natural value. Store a struct and make `Less` compare the priority field. Here a lower number means "served first", so the pager (p1) beats the email (p5) no matter the insertion order.

**Steps:**

1. Heap of a `task` struct; `Less` compares `priority`.
2. Push tasks in any order.
3. Pop → lowest priority number first.

```go
package main

import (
	"container/heap"
	"fmt"
)

// A priority queue orders items by an explicit priority, not the value itself.
// Here a lower priority number = served first (a min-heap on priority).
type task struct {
	name     string
	priority int
}

type PQ []task

func (pq PQ) Len() int           { return len(pq) }
func (pq PQ) Less(i, j int) bool { return pq[i].priority < pq[j].priority }
func (pq PQ) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
func (pq *PQ) Push(x any)        { *pq = append(*pq, x.(task)) }
func (pq *PQ) Pop() any {
	old := *pq
	n := len(old)
	t := old[n-1]
	*pq = old[:n-1]
	return t
}

func main() {
	pq := &PQ{}
	heap.Push(pq, task{"email", 5})
	heap.Push(pq, task{"pager", 1})
	heap.Push(pq, task{"backup", 3})
	for pq.Len() > 0 {
		t := heap.Pop(pq).(task)
		fmt.Printf("serve %s (p%d)\n", t.name, t.priority)
	}
}
```

**Output:**

```
serve pager (p1)
serve backup (p3)
serve email (p5)
```

---

## 15. Update a priority with heap.Fix

`🟡 medium` · *heap.Fix*

Sometimes an item's priority changes while it's in the queue. Mutate it, then call **`heap.Fix(h, i)`** to re-sift just that element back into place — O(log n). You need its current index; production queues store an `index` field updated in `Swap`, but a small scan illustrates the idea.

**Steps:**

1. Heap of `*task` pointers so a change is visible to the heap.
2. Lower `backup`'s priority, then `heap.Fix` at its index.
3. It jumps to the front of the queue.

```go
package main

import (
	"container/heap"
	"fmt"
)

type task struct {
	name     string
	priority int
}

type PQ []*task

func (pq PQ) Len() int           { return len(pq) }
func (pq PQ) Less(i, j int) bool { return pq[i].priority < pq[j].priority }
func (pq PQ) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
func (pq *PQ) Push(x any)        { *pq = append(*pq, x.(*task)) }
func (pq *PQ) Pop() any {
	old := *pq
	n := len(old)
	t := old[n-1]
	*pq = old[:n-1]
	return t
}

func index(pq *PQ, t *task) int {
	for i, x := range *pq {
		if x == t {
			return i
		}
	}
	return -1
}

func main() {
	backup := &task{"backup", 3}
	pq := &PQ{{"email", 5}, backup, {"pager", 1}}
	heap.Init(pq)

	// Lower backup's priority in place, then heap.Fix to restore the invariant.
	// (Real code stores each item's index so this lookup is O(1), not a scan.)
	backup.priority = 0
	heap.Fix(pq, index(pq, backup))

	for pq.Len() > 0 {
		t := heap.Pop(pq).(*task)
		fmt.Printf("serve %s (p%d)\n", t.name, t.priority)
	}
}
```

**Output:**

```
serve backup (p0)
serve pager (p1)
serve email (p5)
```

---

## 16. Remove an item with heap.Remove

`🟡 medium` · *heap.Remove*

`heap.Remove(h, i)` deletes the element at index `i` and restores the heap in O(log n), returning the removed value. Useful for cancelling a queued task. (As with `Fix`, in real code you'd track each item's index rather than guess it.)

**Steps:**

1. Build a heap of six values.
2. `heap.Remove(h, 2)` deletes whatever is at index 2 and re-heapifies.
3. Popping the rest shows that value is gone.

```go
package main

import (
	"container/heap"
	"fmt"
)

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *IntHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

func main() {
	h := &IntHeap{}
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		heap.Push(h, v)
	}
	// heap.Remove(h, i) deletes the element at index i and restores the heap,
	// returning it. Here we remove whatever currently sits at index 2.
	removed := heap.Remove(h, 2)
	fmt.Println("removed:", removed)
	for h.Len() > 0 {
		fmt.Print(heap.Pop(h), " ")
	}
	fmt.Println()
}
```

**Output:**

```
removed: 2
1 3 5 8 9 
```

---

## 17. A generic heap

`🟡 medium` · *Generics*

`container/heap` predates generics, so it stores `any` and needs a new type per element. If you'd rather have type safety and skip the boilerplate, hand-roll a small generic heap: a slice plus a `less` function. One type, works for anything.

**Steps:**

1. `Heap[T]` holds `data []T` and `less func(a, b T) bool`.
2. `Push`/`Pop` sift using `less` — no per-type interface.
3. Pass `func(a, b int) bool { return a < b }` for a min-heap of ints.

```go
package main

import "fmt"

// The stdlib has no generic heap, so here's a small one: a slice plus a less
// function. Works for any type; no interface boilerplate per element type.
type Heap[T any] struct {
	data []T
	less func(a, b T) bool
}

func New[T any](less func(a, b T) bool) *Heap[T] {
	return &Heap[T]{less: less}
}

func (h *Heap[T]) Len() int { return len(h.data) }

func (h *Heap[T]) Push(v T) {
	h.data = append(h.data, v)
	for i := len(h.data) - 1; i > 0; {
		p := (i - 1) / 2
		if !h.less(h.data[i], h.data[p]) {
			break
		}
		h.data[i], h.data[p] = h.data[p], h.data[i]
		i = p
	}
}

func (h *Heap[T]) Pop() (T, bool) {
	var zero T
	n := len(h.data)
	if n == 0 {
		return zero, false
	}
	top := h.data[0]
	h.data[0] = h.data[n-1]
	h.data = h.data[:n-1]
	n--
	for i := 0; ; {
		l, r, best := 2*i+1, 2*i+2, i
		if l < n && h.less(h.data[l], h.data[best]) {
			best = l
		}
		if r < n && h.less(h.data[r], h.data[best]) {
			best = r
		}
		if best == i {
			break
		}
		h.data[i], h.data[best] = h.data[best], h.data[i]
		i = best
	}
	return top, true
}

func main() {
	// Min-heap of ints via a less function — no per-type boilerplate.
	h := New(func(a, b int) bool { return a < b })
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		h.Push(v)
	}
	for h.Len() > 0 {
		v, _ := h.Pop()
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
1 2 3 5 8 9 
```

---

> Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md) · Back to the [index](README.md)
