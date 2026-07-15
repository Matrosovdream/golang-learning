# Step 52 — Heaps & Priority Queues · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

This tier builds a heap **from scratch** to understand it; the [🟡 medium](2-medium.md) tier switches to the stdlib `container/heap`.

---

## 1. A heap is an array

`🟢 easy` · *Index math*

A binary heap is a *complete* tree, so it packs perfectly into a plain slice — no pointers. The tree structure is implied by **index arithmetic**: the node at `i` has parent `(i-1)/2` and children `2i+1` / `2i+2`. This layout is why heaps are cache-friendly and allocation-free.

**Steps:**

1. Write `parent`, `left`, `right` as index helpers.
2. Lay a small min-heap out as a slice.
3. Navigate from a node to its parent and children by index.

```go
package main

import "fmt"

// A heap is stored in a plain slice — no pointers. For the node at index i the
// parent is (i-1)/2 and the children are 2i+1 and 2i+2. This "implicit" layout
// is why heaps are cache-friendly and allocation-free.
func parent(i int) int { return (i - 1) / 2 }
func left(i int) int   { return 2*i + 1 }
func right(i int) int  { return 2*i + 2 }

func main() {
	// A min-heap laid out as a slice:
	//          1
	//        /   \
	//       3     6
	//      / \   /
	//     5   9 8
	h := []int{1, 3, 6, 5, 9, 8}
	i := 1 // the node with value 3
	fmt.Printf("node %d (val %d)\n", i, h[i])
	fmt.Printf("  parent idx %d val %d\n", parent(i), h[parent(i)])
	fmt.Printf("  left   idx %d val %d\n", left(i), h[left(i)])
	fmt.Printf("  right  idx %d val %d\n", right(i), h[right(i)])
}
```

**Output:**

```
node 1 (val 3)
  parent idx 0 val 1
  left   idx 3 val 5
  right  idx 4 val 9
```

---

## 2. The heap property

`🟢 easy` · *Invariant*

What makes a slice a heap is one **local** rule: in a min-heap every parent is `≤` both of its children (so the minimum is at index 0). It says nothing about left-vs-right or across subtrees — that's why a heap isn't sorted, and why it's cheap to maintain. Checking it only needs each parent compared to its children.

**Steps:**

1. For each index `i`, look at children `2i+1` and `2i+2` (if they exist).
2. If any child is smaller than its parent, the property is violated.
3. A valid heap passes; a slice with a big parent fails.

```go
package main

import "fmt"

// isMinHeap reports whether the slice satisfies the min-heap property: every
// parent is <= both of its children. We only need to check each parent against
// its children (indices 2i+1 and 2i+2).
func isMinHeap(h []int) bool {
	for i := 0; i < len(h); i++ {
		l, r := 2*i+1, 2*i+2
		if l < len(h) && h[i] > h[l] {
			return false
		}
		if r < len(h) && h[i] > h[r] {
			return false
		}
	}
	return true
}

func main() {
	fmt.Println(isMinHeap([]int{1, 3, 6, 5, 9, 8})) // true
	fmt.Println(isMinHeap([]int{1, 3, 6, 5, 2, 8})) // false: 3 > child 2
}
```

**Output:**

```
true
false
```

---

## 3. Sift up (insert)

`🟢 easy` · *Sift up*

To insert, append the new value at the **end** of the slice, then let it **bubble up**: while it's smaller than its parent, swap them. It climbs to the right level in O(log n) — the tree's height. This is half of what makes a heap fast.

**Steps:**

1. Append the value; its index is `len-1`.
2. While it's smaller than its parent, swap and move up.
3. Watch each insertion re-establish the heap property.

```go
package main

import "fmt"

// siftUp restores the heap after appending a new value at the end: while the
// node is smaller than its parent, swap them — the value "bubbles up" to its
// correct level. This is how insertion keeps the heap O(log n).
func siftUp(h []int, i int) {
	for i > 0 {
		p := (i - 1) / 2
		if h[p] <= h[i] {
			break // parent already smaller: heap property holds
		}
		h[p], h[i] = h[i], h[p]
		i = p
	}
}

func insert(h []int, v int) []int {
	h = append(h, v)    // put it at the end...
	siftUp(h, len(h)-1) // ...then bubble it up
	return h
}

func main() {
	var h []int
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		h = insert(h, v)
		fmt.Println("after insert", v, "->", h)
	}
}
```

**Output:**

```
after insert 5 -> [5]
after insert 3 -> [3 5]
after insert 8 -> [3 5 8]
after insert 1 -> [1 3 8 5]
after insert 9 -> [1 3 8 5 9]
after insert 2 -> [1 3 2 5 9 8]
```

---

## 4. Sift down (extract)

`🟢 easy` · *Sift down*

The other core operation. After the root is removed and replaced by the last element, that element **sinks down**: repeatedly swap it with its *smaller* child until neither child is smaller. This restores the heap in O(log n) and is the heart of extract-min.

**Steps:**

1. Compare the node with both children; find the smallest of the three.
2. If a child is smaller, swap and continue from there.
3. Stop when the node is `≤` both children.

```go
package main

import "fmt"

// siftDown restores the heap after the root is replaced: repeatedly swap the
// node with its SMALLER child until neither child is smaller. This is the core
// of extract-min. n bounds the live portion of the slice.
func siftDown(h []int, i, n int) {
	for {
		l, r, smallest := 2*i+1, 2*i+2, i
		if l < n && h[l] < h[smallest] {
			smallest = l
		}
		if r < n && h[r] < h[smallest] {
			smallest = r
		}
		if smallest == i {
			break // heap property restored
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

func main() {
	// Root (was 1) has been overwritten by 8; sift it down to restore the heap.
	h := []int{8, 3, 2, 5, 9}
	siftDown(h, 0, len(h))
	fmt.Println(h)
}
```

**Output:**

```
[2 3 8 5 9]
```

---

## 5. A hand-rolled min-heap

`🟢 easy` · *Min-heap*

Put sift-up and sift-down together and you have a working min-heap: `Insert` appends + sifts up; `ExtractMin` saves the root, moves the last element to the top, shrinks, and sifts down. Extract everything and it comes out **sorted** — the heap's signature behaviour.

**Steps:**

1. `Insert` = append then sift up.
2. `ExtractMin` = take root, move last to root, shrink, sift down.
3. Drain the heap and observe ascending output.

```go
package main

import "fmt"

// MinHeap is a min-heap built on a slice. Insert appends + sifts up; ExtractMin
// swaps the root with the last element, shrinks, and sifts the new root down.
type MinHeap struct {
	data []int
}

func (h *MinHeap) Len() int { return len(h.data) }

func (h *MinHeap) Insert(v int) {
	h.data = append(h.data, v)
	i := len(h.data) - 1
	for i > 0 {
		p := (i - 1) / 2
		if h.data[p] <= h.data[i] {
			break
		}
		h.data[p], h.data[i] = h.data[i], h.data[p]
		i = p
	}
}

func (h *MinHeap) ExtractMin() (int, bool) {
	n := len(h.data)
	if n == 0 {
		return 0, false
	}
	min := h.data[0]
	h.data[0] = h.data[n-1] // move last to root
	h.data = h.data[:n-1]
	n--
	i := 0
	for { // sift down
		l, r, s := 2*i+1, 2*i+2, i
		if l < n && h.data[l] < h.data[s] {
			s = l
		}
		if r < n && h.data[r] < h.data[s] {
			s = r
		}
		if s == i {
			break
		}
		h.data[i], h.data[s] = h.data[s], h.data[i]
		i = s
	}
	return min, true
}

func main() {
	h := &MinHeap{}
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		h.Insert(v)
	}
	// Extract in sorted order — that's what a heap gives you.
	for h.Len() > 0 {
		v, _ := h.ExtractMin()
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

## 6. Peek the minimum

`🟢 easy` · *Peek*

The whole point of a min-heap is O(1) access to the smallest element — it's always at index 0. `Peek` returns it without removing it, reporting `ok=false` on an empty heap so callers never index an empty slice.

**Steps:**

1. `Peek` returns `(h.data[0], true)`, or `(0, false)` when empty.
2. Peek before any insert → `false`.
3. After each insert, the min may change; peek to see the current smallest.

```go
package main

import "fmt"

type MinHeap struct{ data []int }

func (h *MinHeap) Len() int { return len(h.data) }

// Peek returns the minimum without removing it — it's always at index 0. O(1).
func (h *MinHeap) Peek() (int, bool) {
	if len(h.data) == 0 {
		return 0, false
	}
	return h.data[0], true
}

func (h *MinHeap) Insert(v int) {
	h.data = append(h.data, v)
	for i := len(h.data) - 1; i > 0; {
		p := (i - 1) / 2
		if h.data[p] <= h.data[i] {
			break
		}
		h.data[p], h.data[i] = h.data[i], h.data[p]
		i = p
	}
}

func main() {
	h := &MinHeap{}
	_, ok := h.Peek()
	fmt.Println("peek empty ok:", ok)

	for _, v := range []int{7, 4, 9, 2, 6} {
		h.Insert(v)
		m, _ := h.Peek()
		fmt.Printf("inserted %d, min is %d, size %d\n", v, m, h.Len())
	}
}
```

**Output:**

```
peek empty ok: false
inserted 7, min is 7, size 1
inserted 4, min is 4, size 2
inserted 9, min is 4, size 3
inserted 2, min is 2, size 4
inserted 6, min is 2, size 5
```

---

## 7. A max-heap

`🟢 easy` · *Max-heap*

A max-heap is the same code with the comparisons **flipped**: a parent must be `≥` its children, so the *maximum* sits at the root. Nothing else changes — sift up uses `>=`, sift down chases the *larger* child. Extract everything and it comes out descending.

**Steps:**

1. In `Push`, break when the parent is already `≥` the child.
2. In `Pop`, sift toward the *bigger* child.
3. Drain the heap → descending order.

```go
package main

import "fmt"

// A max-heap is a min-heap with the comparison flipped: a parent must be >= its
// children, so the maximum sits at the root. Only the comparisons change.
type MaxHeap struct{ data []int }

func (h *MaxHeap) Len() int { return len(h.data) }

func (h *MaxHeap) Push(v int) {
	h.data = append(h.data, v)
	for i := len(h.data) - 1; i > 0; {
		p := (i - 1) / 2
		if h.data[p] >= h.data[i] { // >= instead of <=
			break
		}
		h.data[p], h.data[i] = h.data[i], h.data[p]
		i = p
	}
}

func (h *MaxHeap) Pop() (int, bool) {
	n := len(h.data)
	if n == 0 {
		return 0, false
	}
	top := h.data[0]
	h.data[0] = h.data[n-1]
	h.data = h.data[:n-1]
	n--
	for i := 0; ; {
		l, r, big := 2*i+1, 2*i+2, i
		if l < n && h.data[l] > h.data[big] { // > instead of <
			big = l
		}
		if r < n && h.data[r] > h.data[big] {
			big = r
		}
		if big == i {
			break
		}
		h.data[i], h.data[big] = h.data[big], h.data[i]
		i = big
	}
	return top, true
}

func main() {
	h := &MaxHeap{}
	for _, v := range []int{5, 3, 8, 1, 9, 2} {
		h.Push(v)
	}
	for h.Len() > 0 {
		v, _ := h.Pop()
		fmt.Print(v, " ") // descending
	}
	fmt.Println()
}
```

**Output:**

```
9 8 5 3 2 1 
```

---

## 8. Heapify in O(n)

`🟢 easy` · *Build-heap*

When you already have a slice, you can build a heap **in place** faster than inserting one at a time. Sift down every non-leaf node, starting from the **last parent** and working back to the root — that's O(n), not O(n log n). This is exactly what `heap.Init` does under the hood.

**Steps:**

1. The last parent is at index `n/2 - 1`; leaves need no work.
2. Sift down each node from there back to index 0.
3. The minimum ends up at the root.

```go
package main

import "fmt"

// heapify turns an arbitrary slice into a min-heap in O(n) — faster than n
// inserts (O(n log n)). Sift down every non-leaf node, starting from the last
// parent and working back to the root.
func heapify(h []int) {
	n := len(h)
	for i := n/2 - 1; i >= 0; i-- { // last parent -> root
		siftDown(h, i, n)
	}
}

func siftDown(h []int, i, n int) {
	for {
		l, r, s := 2*i+1, 2*i+2, i
		if l < n && h[l] < h[s] {
			s = l
		}
		if r < n && h[r] < h[s] {
			s = r
		}
		if s == i {
			break
		}
		h[i], h[s] = h[s], h[i]
		i = s
	}
}

func main() {
	h := []int{9, 4, 7, 1, 8, 3, 6}
	heapify(h)
	fmt.Println("heap:", h)
	fmt.Println("min at root:", h[0])
}
```

**Output:**

```
heap: [1 4 3 9 8 7 6]
min at root: 1
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
