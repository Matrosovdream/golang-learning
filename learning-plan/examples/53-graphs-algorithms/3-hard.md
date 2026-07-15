# Step 53 — Graphs & Graph Algorithms · 🔴 Hard

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

## 18. Union-Find (disjoint-set union)

`🔴 hard` · *Union-Find*

Union-Find answers "are these two in the same set?" in near-constant time. Each element points at a parent; `Find` follows to the root (the set's representative), and `Union` links two roots. Two optimisations make it almost O(1): **path compression** (flatten the tree during `Find`) and **union by rank** (attach the shorter tree under the taller).

**Steps:**

1. Every element starts as its own parent (its own set).
2. `Find` walks to the root, halving the path as it goes.
3. `Union` joins two roots, keeping the tree shallow via rank.

```go
package main

import "fmt"

// UnionFind (disjoint-set union) tracks a partition of {0..n-1} into sets, with
// near-O(1) Find/Union thanks to path compression and union by rank.
type UnionFind struct {
	parent []int
	rank   []int
}

func NewUnionFind(n int) *UnionFind {
	uf := &UnionFind{parent: make([]int, n), rank: make([]int, n)}
	for i := range uf.parent {
		uf.parent[i] = i // each element starts in its own set
	}
	return uf
}

// Find returns the set's representative, compressing the path on the way up.
func (uf *UnionFind) Find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]] // path halving
		x = uf.parent[x]
	}
	return x
}

// Union merges the sets of a and b; returns false if they were already joined.
func (uf *UnionFind) Union(a, b int) bool {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra == rb {
		return false
	}
	if uf.rank[ra] < uf.rank[rb] { // attach the shorter tree under the taller
		ra, rb = rb, ra
	}
	uf.parent[rb] = ra
	if uf.rank[ra] == uf.rank[rb] {
		uf.rank[ra]++
	}
	return true
}

func main() {
	uf := NewUnionFind(6)
	uf.Union(0, 1)
	uf.Union(2, 3)
	uf.Union(1, 3) // now {0,1,2,3} are one set
	fmt.Println("0 ~ 3:", uf.Find(0) == uf.Find(3))
	fmt.Println("0 ~ 4:", uf.Find(0) == uf.Find(4))
}
```

**Output:**

```
0 ~ 3: true
0 ~ 4: false
```

---

## 19. Components with Union-Find

`🔴 hard` · *Union-Find*

Union-Find gives connected components without any traversal: start with `n` components and drop the count by one every time a `Union` actually merges two different sets. This is the incremental alternative to the DFS of [11](2-medium.md#11-connected-components) — ideal when edges arrive over time.

**Steps:**

1. Initialise `count = n` (every node its own component).
2. For each edge, `Union` the endpoints; decrement `count` only on a real merge.
3. The final `count` is the number of components.

```go
package main

import "fmt"

type UnionFind struct {
	parent []int
	count  int
}

func NewUnionFind(n int) *UnionFind {
	uf := &UnionFind{parent: make([]int, n), count: n}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (uf *UnionFind) Find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

// Union merges two sets and, when they differ, drops the component count by one.
func (uf *UnionFind) Union(a, b int) {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra != rb {
		uf.parent[ra] = rb
		uf.count-- // two components became one
	}
}

func main() {
	// 5 nodes; edges connect {0,1,2} and {3,4}.
	uf := NewUnionFind(5)
	edges := [][2]int{{0, 1}, {1, 2}, {3, 4}}
	for _, e := range edges {
		uf.Union(e[0], e[1])
	}
	fmt.Println("components:", uf.count) // 2
}
```

**Output:**

```
components: 2
```

---

## 20. Cycle detection with Union-Find

`🔴 hard` · *Union-Find*

In an undirected graph, an edge whose two endpoints are **already in the same set** closes a cycle. Process edges in order and `Union` each; the first edge that fails to merge (endpoints already connected) is the redundant one. This is the union-find alternative to [13](2-medium.md#13-cycle-detection-undirected).

**Steps:**

1. `Union` returns `false` when the endpoints were already connected.
2. Add edges one by one; the first `false` is a cycle-closing edge.
3. Here `2-0` closes the triangle `0-1-2`.

```go
package main

import "fmt"

type UnionFind struct{ parent []int }

func NewUnionFind(n int) *UnionFind {
	uf := &UnionFind{parent: make([]int, n)}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (uf *UnionFind) Find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

func (uf *UnionFind) Union(a, b int) bool {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra == rb {
		return false // already connected: this edge closes a cycle
	}
	uf.parent[ra] = rb
	return true
}

// redundantEdge returns the first edge that connects two already-connected nodes
// — i.e. the edge that creates a cycle in an undirected graph.
func redundantEdge(n int, edges [][2]int) [2]int {
	uf := NewUnionFind(n)
	for _, e := range edges {
		if !uf.Union(e[0], e[1]) {
			return e
		}
	}
	return [2]int{-1, -1}
}

func main() {
	// 0-1, 1-2, 2-0 (this edge closes the triangle), 2-3
	edges := [][2]int{{0, 1}, {1, 2}, {2, 0}, {2, 3}}
	fmt.Println("redundant edge:", redundantEdge(4, edges)) // [2 0]
}
```

**Output:**

```
redundant edge: [2 0]
```

---

## 21. Dijkstra's shortest path

`🔴 hard` · *Weighted*

For **weighted** graphs, BFS no longer finds shortest paths — you need Dijkstra. A min-heap keyed on distance ([52](../52-heaps-priority-queues/)) always expands the **closest unsettled node**; relaxing its edges improves neighbours. This is **lazy** Dijkstra: push duplicates and skip stale pops rather than doing decrease-key. Non-negative weights only.

**Steps:**

1. `dist[src] = 0`; push `(src, 0)`.
2. Pop the closest node; skip it if its distance is stale.
3. Relax each edge, pushing neighbours whose distance improved.

```go
package main

import (
	"container/heap"
	"fmt"
)

type edge struct{ to, weight int }

type item struct{ node, dist int }

type pq []item

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].dist < p[j].dist }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x any)        { *p = append(*p, x.(item)) }
func (p *pq) Pop() any {
	o := *p
	n := len(o)
	it := o[n-1]
	*p = o[:n-1]
	return it
}

// dijkstra finds shortest distances from src using a min-heap keyed on distance
// (see lesson 52). Non-negative weights only.
func dijkstra(g [][]edge, src int) []int {
	const inf = 1 << 30
	dist := make([]int, len(g))
	for i := range dist {
		dist[i] = inf
	}
	dist[src] = 0
	h := &pq{{src, 0}}
	for h.Len() > 0 {
		cur := heap.Pop(h).(item)
		if cur.dist > dist[cur.node] {
			continue
		}
		for _, e := range g[cur.node] {
			if nd := cur.dist + e.weight; nd < dist[e.to] {
				dist[e.to] = nd
				heap.Push(h, item{e.to, nd})
			}
		}
	}
	return dist
}

func main() {
	// 0->1 (4), 0->2 (1), 2->1 (2), 1->3 (1), 2->3 (5)
	g := [][]edge{
		{{1, 4}, {2, 1}},
		{{3, 1}},
		{{1, 2}, {3, 5}},
		{},
	}
	fmt.Println("distances from 0:", dijkstra(g, 0)) // [0 3 1 4]
}
```

**Output:**

```
distances from 0: [0 3 1 4]
```

---

## 22. Kruskal's minimum spanning tree

`🔴 hard` · *MST*

A **minimum spanning tree** connects all nodes with the least total edge weight. **Kruskal** is beautifully simple: sort every edge by weight ([51](../51-sorting-searching/)), then add each edge whose endpoints are in **different components** — Union-Find ([18](#18-union-find-disjoint-set-union)) rejects the ones that would form a cycle. Greedy, and provably optimal.

**Steps:**

1. Sort edges ascending by weight.
2. For each edge, `Union` the endpoints; keep it only if they weren't already connected.
3. Stop implicitly once `n-1` edges are chosen — the tree's total weight is minimal.

```go
package main

import (
	"fmt"
	"slices"
)

type edge struct{ u, v, w int }

type UnionFind struct{ parent []int }

func NewUnionFind(n int) *UnionFind {
	uf := &UnionFind{parent: make([]int, n)}
	for i := range uf.parent {
		uf.parent[i] = i
	}
	return uf
}

func (uf *UnionFind) Find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

func (uf *UnionFind) Union(a, b int) bool {
	ra, rb := uf.Find(a), uf.Find(b)
	if ra == rb {
		return false
	}
	uf.parent[ra] = rb
	return true
}

// kruskal builds a minimum spanning tree: sort edges by weight, then add each
// edge whose endpoints are in different components (union-find rejects the ones
// that would form a cycle). Greedy, and provably minimal.
func kruskal(n int, edges []edge) (int, []edge) {
	slices.SortFunc(edges, func(a, b edge) int { return a.w - b.w })
	uf := NewUnionFind(n)
	total := 0
	var tree []edge
	for _, e := range edges {
		if uf.Union(e.u, e.v) { // endpoints not yet connected
			total += e.w
			tree = append(tree, e)
		}
	}
	return total, tree
}

func main() {
	edges := []edge{
		{0, 1, 4}, {0, 2, 1}, {1, 2, 2}, {1, 3, 5}, {2, 3, 8},
	}
	total, tree := kruskal(4, edges)
	fmt.Println("MST weight:", total)
	for _, e := range tree {
		fmt.Printf("  %d-%d (w%d)\n", e.u, e.v, e.w)
	}
}
```

**Output:**

```
MST weight: 8
  0-2 (w1)
  1-2 (w2)
  1-3 (w5)
```

---

## 23. Prim's minimum spanning tree

`🔴 hard` · *MST*

Prim reaches the same minimum tree by a different strategy: **grow** it from one node, always adding the cheapest edge that leaves the current tree. A min-heap ([52](../52-heaps-priority-queues/)) supplies that cheapest edge. Where Kruskal sorts all edges globally, Prim expands locally — the total weight comes out identical.

**Steps:**

1. Start at node 0; push its edges into a min-heap.
2. Pop the cheapest edge; if its endpoint is new, add it and push its edges.
3. Skip edges to nodes already in the tree; sum the weights taken.

```go
package main

import (
	"container/heap"
	"fmt"
)

type edge struct{ to, weight int }

type cand struct{ node, weight int }

type pq []cand

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].weight < p[j].weight }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x any)        { *p = append(*p, x.(cand)) }
func (p *pq) Pop() any {
	o := *p
	n := len(o)
	c := o[n-1]
	*p = o[:n-1]
	return c
}

// prim grows an MST from node 0: a min-heap yields the cheapest edge leaving the
// tree; add its endpoint if new, then push that node's edges. The total weight
// equals Kruskal's — same tree cost, different strategy.
func prim(g [][]edge) int {
	inTree := make([]bool, len(g))
	h := &pq{{0, 0}}
	total := 0
	for h.Len() > 0 {
		c := heap.Pop(h).(cand)
		if inTree[c.node] {
			continue
		}
		inTree[c.node] = true
		total += c.weight
		for _, e := range g[c.node] {
			if !inTree[e.to] {
				heap.Push(h, cand{e.to, e.weight})
			}
		}
	}
	return total
}

func main() {
	// Same graph as Kruskal, stored undirected.
	g := [][]edge{
		{{1, 4}, {2, 1}},
		{{0, 4}, {2, 2}, {3, 5}},
		{{0, 1}, {1, 2}, {3, 8}},
		{{1, 5}, {2, 8}},
	}
	fmt.Println("MST weight:", prim(g)) // 8
}
```

**Output:**

```
MST weight: 8
```

---

## 24. Multi-source BFS

`🔴 hard` · *Grid*

When several sources spread out at once — fire from many cells, distance to the nearest exit — seed the BFS queue with **all sources at distance 0** and expand normally. Because every source advances one ring per step in lockstep, each cell is reached first by its **closest** source. Here we compute each cell's distance to the nearest `0`.

**Steps:**

1. Enqueue every `0` cell up front (distance 0); mark others `-1`.
2. Standard BFS outward through 4-neighbours.
3. Each cell's recorded distance is to its nearest source.

```go
package main

import "fmt"

// nearestZero returns, for each cell, the distance to the nearest 0 cell.
// Multi-source BFS: seed the queue with ALL zeros at once (distance 0), then
// expand outward — every cell is reached first by its closest source.
func nearestZero(grid [][]int) [][]int {
	rows, cols := len(grid), len(grid[0])
	dist := make([][]int, rows)
	type cell struct{ r, c int }
	var queue []cell
	for r := 0; r < rows; r++ {
		dist[r] = make([]int, cols)
		for c := 0; c < cols; c++ {
			if grid[r][c] == 0 {
				queue = append(queue, cell{r, c}) // a source
			} else {
				dist[r][c] = -1 // unvisited
			}
		}
	}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range dirs {
			nr, nc := cur.r+d[0], cur.c+d[1]
			if nr >= 0 && nr < rows && nc >= 0 && nc < cols && dist[nr][nc] == -1 {
				dist[nr][nc] = dist[cur.r][cur.c] + 1
				queue = append(queue, cell{nr, nc})
			}
		}
	}
	return dist
}

func main() {
	grid := [][]int{
		{0, 0, 0},
		{0, 1, 0},
		{1, 1, 1},
	}
	for _, row := range nearestZero(grid) {
		fmt.Println(row)
	}
}
```

**Output:**

```
[0 0 0]
[0 1 0]
[1 2 1]
```

---

## 25. A generic Graph[T]

`🔴 hard` · *Generics*

Everything so far hardcoded `int` nodes. A **generic** `Graph[T comparable]` over an adjacency map lets you build graphs of strings, structs, whatever — with reusable traversal methods. This is the idiomatic way to package a graph once and use it everywhere ([17 — Generics](../../17-generics.md)).

**Steps:**

1. `Graph[T comparable]` wraps `map[T][]T`.
2. `AddEdge` records the edge and ensures both endpoints exist as nodes.
3. `BFS` is the same algorithm as [6](1-easy.md#6-bfs-traversal), now type-parameterised.

```go
package main

import (
	"fmt"
	"sort"
)

// Graph is a generic directed graph on comparable node keys, backed by an
// adjacency map. It offers reusable BFS without hardcoding int nodes.
type Graph[T comparable] struct {
	adj map[T][]T
}

func NewGraph[T comparable]() *Graph[T] {
	return &Graph[T]{adj: map[T][]T{}}
}

func (g *Graph[T]) AddEdge(u, v T) {
	g.adj[u] = append(g.adj[u], v)
	if _, ok := g.adj[v]; !ok {
		g.adj[v] = nil // ensure v exists as a node
	}
}

func (g *Graph[T]) BFS(src T) []T {
	visited := map[T]bool{src: true}
	queue := []T{src}
	var order []T
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, nb := range g.adj[n] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return order
}

func main() {
	g := NewGraph[string]()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "d")
	g.AddEdge("c", "d")
	fmt.Println("BFS from a:", g.BFS("a"))

	// Node count (sorted for stable output).
	nodes := make([]string, 0, len(g.adj))
	for n := range g.adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	fmt.Println("nodes:", nodes)
}
```

**Output:**

```
BFS from a: [a b c d]
nodes: [a b c d]
```

---

## 26. Word ladder

`🔴 hard` · *Implicit graph*

The capstone: a graph you never build. To find the shortest chain from `hit` to `cog` changing one letter at a time (each step a real dictionary word), the nodes are words and the edges are "differ by one letter" — **generated on the fly**. BFS over this *implicit* graph finds the shortest transformation, proving you don't need an explicit adjacency list to run a graph algorithm.

**Steps:**

1. BFS from `begin`, tracking the chain length.
2. Generate neighbours by trying every letter at every position; keep those in the dictionary and unseen.
3. The first time `end` is dequeued, its distance is the shortest ladder length.

```go
package main

import "fmt"

// ladderLength returns the number of words in the shortest transformation from
// begin to end, changing one letter at a time, where every intermediate word is
// in the dictionary. The graph is IMPLICIT: neighbours are words one letter away,
// generated on the fly. BFS gives the shortest chain.
func ladderLength(begin, end string, words []string) int {
	dict := map[string]bool{}
	for _, w := range words {
		dict[w] = true
	}
	if !dict[end] {
		return 0
	}
	type step struct {
		word string
		dist int
	}
	queue := []step{{begin, 1}}
	seen := map[string]bool{begin: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.word == end {
			return cur.dist
		}
		b := []byte(cur.word)
		for i := range b {
			orig := b[i]
			for c := byte('a'); c <= 'z'; c++ {
				b[i] = c
				next := string(b)
				if dict[next] && !seen[next] {
					seen[next] = true
					queue = append(queue, step{next, cur.dist + 1})
				}
			}
			b[i] = orig // restore
		}
	}
	return 0
}

func main() {
	words := []string{"hot", "dot", "dog", "lot", "log", "cog"}
	// hit -> hot -> dot -> dog -> cog  (5 words)
	fmt.Println("ladder length:", ladderLength("hit", "cog", words))
}
```

**Output:**

```
ladder length: 5
```

---

> Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
