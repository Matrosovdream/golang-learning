# Step 52 — Heaps & Priority Queues · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md)

---

## 18. Top-K frequent elements

`🔴 hard` · *Top-K*

The heap's signature win over sorting. To find the `k` most frequent values, keep a **min-heap of size k** on the counts: push each `(value, count)`, and whenever the heap exceeds `k`, pop the smallest count. What survives is the top `k` — **O(n log k)** time and **O(k)** memory, beating [51](../51-sorting-searching/)'s full sort when `k ≪ n`.

**Steps:**

1. Count occurrences into a map.
2. Push each `(val, count)`; if `len > k`, pop the smallest-count item.
3. Drain the heap for the survivors (sorted here only for stable display).

```go
package main

import (
	"container/heap"
	"fmt"
	"sort"
)

type pair struct {
	val, count int
}

// minCount is a MIN-heap on count: the smallest-count item sits at the root, so
// it's the cheapest to evict when the heap exceeds size k.
type minCount []pair

func (h minCount) Len() int           { return len(h) }
func (h minCount) Less(i, j int) bool { return h[i].count < h[j].count }
func (h minCount) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minCount) Push(x any)        { *h = append(*h, x.(pair)) }
func (h *minCount) Pop() any {
	old := *h
	n := len(old)
	p := old[n-1]
	*h = old[:n-1]
	return p
}

// topKFrequent keeps a size-k min-heap: push each (val,count); when the heap
// grows past k, pop the smallest count. What survives is the k largest — O(n log
// k), beating the full sort of lesson 51 when k << n.
func topKFrequent(nums []int, k int) []int {
	freq := map[int]int{}
	for _, v := range nums {
		freq[v]++
	}
	h := &minCount{}
	for v, c := range freq {
		heap.Push(h, pair{v, c})
		if h.Len() > k {
			heap.Pop(h)
		}
	}
	out := make([]int, 0, h.Len())
	for h.Len() > 0 {
		out = append(out, heap.Pop(h).(pair).val)
	}
	sort.Ints(out) // deterministic display
	return out
}

func main() {
	nums := []int{1, 1, 1, 2, 2, 3, 4, 4, 4, 4}
	fmt.Println("top 2:", topKFrequent(nums, 2)) // 1 (x3) and 4 (x4)
}
```

**Output:**

```
top 2: [1 4]
```

---

## 19. The k-th largest element

`🔴 hard` · *Top-K*

Same size-k idea, single answer. Keep a **min-heap of the k largest** seen so far: push each value, and when the heap exceeds `k`, pop the smallest. The root is then the smallest of the top k — i.e. the **k-th largest** overall. O(n log k), O(k) memory — perfect for a stream you can't fully store.

**Steps:**

1. Push each value; if `len > k`, pop the smallest.
2. The heap always holds the current top k.
3. The root (`h[0]`) is the k-th largest.

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

// kthLargest keeps a MIN-heap of the k largest seen so far. The root is the
// smallest of those k — i.e. the k-th largest overall. O(n log k), O(k) memory:
// ideal for a stream where you can't hold everything.
func kthLargest(nums []int, k int) int {
	h := &IntHeap{}
	for _, v := range nums {
		heap.Push(h, v)
		if h.Len() > k {
			heap.Pop(h) // drop the smallest; keep the top k
		}
	}
	return (*h)[0]
}

func main() {
	nums := []int{3, 2, 1, 5, 6, 4}
	fmt.Println("2nd largest:", kthLargest(nums, 2)) // 5
}
```

**Output:**

```
2nd largest: 5
```

---

## 20. Merge k sorted slices

`🔴 hard` · *Merge-K*

Extending the two-list merge from [50](../50-linear-structures/) to **k** lists. A min-heap holds one **cursor** per list — its current front value, which list, and which index. Pop the smallest, emit it, and push that list's next element. The heap only ever holds `k` items, so it's **O(N log k)** for `N` total elements.

**Steps:**

1. Seed the heap with the first element of each non-empty list.
2. Pop the min, append it, and push the next element from the same list.
3. Repeat until the heap empties — output is fully merged.

```go
package main

import (
	"container/heap"
	"fmt"
)

// Each heap item points to one input slice and the next index to read from it.
type cursor struct {
	val, list, idx int
}

type minCursor []cursor

func (h minCursor) Len() int           { return len(h) }
func (h minCursor) Less(i, j int) bool { return h[i].val < h[j].val }
func (h minCursor) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minCursor) Push(x any)        { *h = append(*h, x.(cursor)) }
func (h *minCursor) Pop() any {
	old := *h
	n := len(old)
	c := old[n-1]
	*h = old[:n-1]
	return c
}

// mergeK merges k sorted slices in O(N log k): the heap always holds the current
// front of each list, so popping the min and pushing that list's next element
// streams out the fully merged order.
func mergeK(lists [][]int) []int {
	h := &minCursor{}
	for li, list := range lists {
		if len(list) > 0 {
			heap.Push(h, cursor{list[0], li, 0})
		}
	}
	var out []int
	for h.Len() > 0 {
		c := heap.Pop(h).(cursor)
		out = append(out, c.val)
		if c.idx+1 < len(lists[c.list]) {
			next := lists[c.list][c.idx+1]
			heap.Push(h, cursor{next, c.list, c.idx + 1})
		}
	}
	return out
}

func main() {
	lists := [][]int{
		{1, 4, 7},
		{2, 5, 8},
		{3, 6, 9},
	}
	fmt.Println(mergeK(lists))
}
```

**Output:**

```
[1 2 3 4 5 6 7 8 9]
```

---

## 21. Running median with two heaps

`🔴 hard` · *Two heaps*

Track the median of a growing stream in O(log n) per element with **two heaps**: a **max-heap** for the smaller half and a **min-heap** for the larger half, kept balanced in size. The median is then O(1) — the max-heap's root, or the average of the two roots when the halves are equal.

**Steps:**

1. Route each value to the low (max-heap) or high (min-heap) side.
2. Rebalance so the two heaps differ in size by at most one.
3. The median is a root, or the average of the two roots.

```go
package main

import (
	"container/heap"
	"fmt"
)

type maxHeap []int // the low half (largest of it at the root)

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *maxHeap) Pop() any {
	o := *h
	n := len(o)
	v := o[n-1]
	*h = o[:n-1]
	return v
}

type minHeap []int // the high half (smallest of it at the root)

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *minHeap) Pop() any {
	o := *h
	n := len(o)
	v := o[n-1]
	*h = o[:n-1]
	return v
}

// MedianStream keeps the smaller half in a max-heap and the larger half in a
// min-heap, balanced in size. The median is then O(1): the max-heap root, or the
// average of the two roots.
type MedianStream struct {
	low  maxHeap
	high minHeap
}

func (m *MedianStream) Add(v int) {
	if m.low.Len() == 0 || v <= m.low[0] {
		heap.Push(&m.low, v)
	} else {
		heap.Push(&m.high, v)
	}
	// Rebalance so |low| == |high| or |low| == |high|+1.
	if m.low.Len() > m.high.Len()+1 {
		heap.Push(&m.high, heap.Pop(&m.low))
	} else if m.high.Len() > m.low.Len() {
		heap.Push(&m.low, heap.Pop(&m.high))
	}
}

func (m *MedianStream) Median() float64 {
	if m.low.Len() > m.high.Len() {
		return float64(m.low[0])
	}
	return float64(m.low[0]+m.high[0]) / 2
}

func main() {
	m := &MedianStream{}
	for _, v := range []int{5, 15, 1, 3, 8, 7} {
		m.Add(v)
		fmt.Printf("added %2d, median %.1f\n", v, m.Median())
	}
}
```

**Output:**

```
added  5, median 5.0
added 15, median 10.0
added  1, median 5.0
added  3, median 4.0
added  8, median 5.0
added  7, median 6.0
```

---

## 22. Dijkstra's shortest path

`🔴 hard` · *Graph / PQ*

The definitive priority-queue algorithm. A min-heap keyed on distance always hands you the **closest unsettled node** next; relaxing its edges may improve neighbours, which get pushed. This is **lazy** Dijkstra: rather than a decrease-key, we push duplicates and **skip stale pops** — simpler, and correct for non-negative edge weights.

**Steps:**

1. Start with `dist[src] = 0`; push `(src, 0)`.
2. Pop the closest node; skip it if its distance is stale.
3. Relax each outgoing edge, pushing any node whose distance improved.

```go
package main

import (
	"container/heap"
	"fmt"
)

type edge struct {
	to, weight int
}

type state struct {
	node, dist int
}

// pq is a min-heap on dist. This is a "lazy" Dijkstra: we may push a node several
// times and simply skip stale pops, which avoids decrease-key bookkeeping.
type pq []state

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].dist < p[j].dist }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x any)        { *p = append(*p, x.(state)) }
func (p *pq) Pop() any {
	o := *p
	n := len(o)
	s := o[n-1]
	*p = o[:n-1]
	return s
}

// dijkstra returns the shortest distance from src to every node in a weighted
// graph (adjacency list). The priority queue always expands the closest
// unsettled node first — that greedy choice is what makes it correct for
// non-negative edge weights.
func dijkstra(graph [][]edge, src int) []int {
	const inf = 1 << 30
	dist := make([]int, len(graph))
	for i := range dist {
		dist[i] = inf
	}
	dist[src] = 0
	h := &pq{{src, 0}}
	for h.Len() > 0 {
		cur := heap.Pop(h).(state)
		if cur.dist > dist[cur.node] {
			continue // stale entry — a better path was already found
		}
		for _, e := range graph[cur.node] {
			if nd := cur.dist + e.weight; nd < dist[e.to] {
				dist[e.to] = nd
				heap.Push(h, state{e.to, nd})
			}
		}
	}
	return dist
}

func main() {
	// 0->1 (4), 0->2 (1), 2->1 (2), 1->3 (1), 2->3 (5)
	graph := [][]edge{
		{{1, 4}, {2, 1}},
		{{3, 1}},
		{{1, 2}, {3, 5}},
		{},
	}
	fmt.Println("distances from 0:", dijkstra(graph, 0))
}
```

**Output:**

```
distances from 0: [0 3 1 4]
```

---

## 23. Meeting rooms: minimum rooms

`🔴 hard` · *Scheduling*

How many rooms do you need so no two overlapping meetings share one? **Sort by start time**, then keep a **min-heap of end times** for rooms in use. For each meeting, if the earliest-ending room is free by its start, reuse it (pop); otherwise open a new room. The heap's peak size is the answer — a classic sort-plus-heap sweep.

**Steps:**

1. Sort meetings by start.
2. Before scheduling one, free the earliest room if it has ended (`heap root ≤ start`).
3. Push this meeting's end; the max heap size seen is the rooms needed.

```go
package main

import (
	"container/heap"
	"fmt"
	"slices"
)

type interval struct{ start, end int }

// endHeap is a min-heap of meeting end times.
type endHeap []int

func (h endHeap) Len() int           { return len(h) }
func (h endHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h endHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *endHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *endHeap) Pop() any {
	o := *h
	n := len(o)
	v := o[n-1]
	*h = o[:n-1]
	return v
}

// minRooms returns the fewest rooms so no two meetings share a room while
// overlapping. Sort by start; a min-heap holds the end times of rooms in use.
// For each meeting, if the earliest-ending room is free by its start, reuse it
// (pop); otherwise open a new room. The heap's peak size is the answer.
func minRooms(meetings []interval) int {
	slices.SortFunc(meetings, func(a, b interval) int { return a.start - b.start })
	h := &endHeap{}
	best := 0
	for _, m := range meetings {
		if h.Len() > 0 && (*h)[0] <= m.start {
			heap.Pop(h) // a room freed up
		}
		heap.Push(h, m.end)
		if h.Len() > best {
			best = h.Len()
		}
	}
	return best
}

func main() {
	meetings := []interval{{0, 30}, {5, 10}, {15, 20}}
	fmt.Println("rooms needed:", minRooms(meetings)) // 2
}
```

**Output:**

```
rooms needed: 2
```

---

## 24. K closest points to origin

`🔴 hard` · *Top-K*

The mirror of example 19: for the k **closest** points you keep a **max-heap** of size k (evict the *farthest*), whereas for the k largest you kept a min-heap. Comparing **squared** distance avoids floating-point `sqrt` entirely. O(n log k).

**Steps:**

1. `Less` on `dist2()` with `>` makes a max-heap (farthest at the root).
2. Push each point; if `len > k`, pop the farthest.
3. The k survivors are the closest; sort them for display.

```go
package main

import (
	"container/heap"
	"fmt"
	"slices"
)

type point struct{ x, y int }

func (p point) dist2() int { return p.x*p.x + p.y*p.y }

// maxDist is a MAX-heap on squared distance: the farthest of the k kept points is
// at the root, so it's the one to evict when a closer point arrives.
type maxDist []point

func (h maxDist) Len() int           { return len(h) }
func (h maxDist) Less(i, j int) bool { return h[i].dist2() > h[j].dist2() }
func (h maxDist) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxDist) Push(x any)        { *h = append(*h, x.(point)) }
func (h *maxDist) Pop() any {
	o := *h
	n := len(o)
	v := o[n-1]
	*h = o[:n-1]
	return v
}

// kClosest returns the k points nearest the origin using a size-k max-heap: keep
// the k closest by evicting the farthest whenever the heap overflows. O(n log k).
func kClosest(points []point, k int) []point {
	h := &maxDist{}
	for _, p := range points {
		heap.Push(h, p)
		if h.Len() > k {
			heap.Pop(h) // drop the farthest
		}
	}
	out := []point(*h)
	slices.SortFunc(out, func(a, b point) int { return a.dist2() - b.dist2() })
	return out
}

func main() {
	points := []point{{1, 3}, {-2, 2}, {5, 8}, {0, 1}}
	for _, p := range kClosest(points, 2) {
		fmt.Printf("(%d,%d) d2=%d\n", p.x, p.y, p.dist2())
	}
}
```

**Output:**

```
(0,1) d2=1
(-2,2) d2=8
```

---

## 25. A generic priority queue

`🔴 hard` · *Generics*

A reusable, type-safe priority queue: values of any type carry an `int` priority, and `Pop` returns the lowest-priority value first. It's the same sift-up/sift-down heap, but generic — no per-type `container/heap` boilerplate, and no `any` type assertions at the call site.

**Steps:**

1. Store `pqItem[T]{value, priority}` in the heap; sift on `priority`.
2. `Push(value, priority)` and `Pop() (T, bool)` are fully typed.
3. Use it directly with a string value type.

```go
package main

import "fmt"

// PriorityQueue is a generic min-priority-queue: each value carries an int
// priority, and Pop returns the lowest-priority value first. Same sift-up/
// sift-down heap as before, but reusable for any value type.
type PriorityQueue[T any] struct {
	items []pqItem[T]
}

type pqItem[T any] struct {
	value    T
	priority int
}

func (pq *PriorityQueue[T]) Len() int { return len(pq.items) }

func (pq *PriorityQueue[T]) Push(value T, priority int) {
	pq.items = append(pq.items, pqItem[T]{value, priority})
	for i := len(pq.items) - 1; i > 0; {
		p := (i - 1) / 2
		if pq.items[p].priority <= pq.items[i].priority {
			break
		}
		pq.items[i], pq.items[p] = pq.items[p], pq.items[i]
		i = p
	}
}

func (pq *PriorityQueue[T]) Pop() (T, bool) {
	var zero T
	n := len(pq.items)
	if n == 0 {
		return zero, false
	}
	top := pq.items[0].value
	pq.items[0] = pq.items[n-1]
	pq.items = pq.items[:n-1]
	n--
	for i := 0; ; {
		l, r, best := 2*i+1, 2*i+2, i
		if l < n && pq.items[l].priority < pq.items[best].priority {
			best = l
		}
		if r < n && pq.items[r].priority < pq.items[best].priority {
			best = r
		}
		if best == i {
			break
		}
		pq.items[i], pq.items[best] = pq.items[best], pq.items[i]
		i = best
	}
	return top, true
}

func main() {
	var pq PriorityQueue[string]
	pq.Push("laundry", 5)
	pq.Push("fire!", 1)
	pq.Push("dishes", 3)
	for pq.Len() > 0 {
		v, _ := pq.Pop()
		fmt.Println("do:", v)
	}
}
```

**Output:**

```
do: fire!
do: dishes
do: laundry
```

---

## 26. Huffman coding

`🔴 hard` · *Greedy*

The capstone: a heap drives the classic **greedy** compression algorithm, and the result is a **tree** ([42](../42-trees/)). Repeatedly pop the two least-frequent nodes from a **min-heap** and merge them under a new parent, until one tree remains. Frequent symbols end up near the root, so they get the **shortest codes** — here `a` (freq 5) gets a 1-bit code while the rare `c`/`d` get 3 bits.

**Steps:**

1. Seed a min-heap (by frequency, tie-broken by symbol for determinism) with a leaf per symbol.
2. Pop two, merge under a parent whose frequency is their sum; push it back.
3. When one node remains it's the root; walk left=`0`/right=`1` to read each symbol's code.

```go
package main

import (
	"container/heap"
	"fmt"
	"sort"
)

type hnode struct {
	sym         byte
	freq        int
	left, right *hnode
}

// forest is a MIN-heap on frequency (tie-broken by symbol for determinism), so
// the two least-frequent subtrees are always at the front to be merged.
type forest []*hnode

func (f forest) Len() int { return len(f) }
func (f forest) Less(i, j int) bool {
	if f[i].freq != f[j].freq {
		return f[i].freq < f[j].freq
	}
	return f[i].sym < f[j].sym // deterministic tie-break
}
func (f forest) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f *forest) Push(x any)   { *f = append(*f, x.(*hnode)) }
func (f *forest) Pop() any {
	o := *f
	n := len(o)
	v := o[n-1]
	*f = o[:n-1]
	return v
}

// huffman builds a prefix-code tree: repeatedly pop the two lowest-frequency
// nodes and merge them under a new parent, until one tree remains. Frequent
// symbols end up near the root (short codes) — the classic greedy heap algorithm.
func huffman(freqs map[byte]int) *hnode {
	f := &forest{}
	for sym, fr := range freqs {
		heap.Push(f, &hnode{sym: sym, freq: fr})
	}
	for f.Len() > 1 {
		a := heap.Pop(f).(*hnode)
		b := heap.Pop(f).(*hnode)
		s := a.sym // parent's sym = min of children, keeping ties deterministic
		if b.sym < s {
			s = b.sym
		}
		heap.Push(f, &hnode{sym: s, freq: a.freq + b.freq, left: a, right: b})
	}
	return heap.Pop(f).(*hnode)
}

func codes(n *hnode, prefix string, out map[byte]string) {
	if n.left == nil && n.right == nil {
		out[n.sym] = prefix
		return
	}
	codes(n.left, prefix+"0", out)
	codes(n.right, prefix+"1", out)
}

func main() {
	freqs := map[byte]int{'a': 5, 'b': 2, 'c': 1, 'd': 1}
	root := huffman(freqs)
	out := map[byte]string{}
	codes(root, "", out)

	syms := make([]byte, 0, len(out))
	for s := range out {
		syms = append(syms, s)
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i] < syms[j] })
	for _, s := range syms {
		fmt.Printf("%c freq=%d code=%s\n", s, freqs[s], out[s])
	}
}
```

**Output:**

```
a freq=5 code=1
b freq=2 code=00
c freq=1 code=010
d freq=1 code=011
```

---

> Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
