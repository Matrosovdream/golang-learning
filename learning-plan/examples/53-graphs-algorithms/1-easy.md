# Step 53 — Graphs & Graph Algorithms · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

This tier covers **representations** and the two traversals (BFS/DFS) everything else builds on.

---

## 1. Adjacency list with a map

`🟢 easy` · *Representation*

The most common way to store a graph: a **map from each node to its list of neighbours**. It's O(V+E) memory and iterating a node's edges is O(degree). Works for any comparable node key. Since map iteration order is random, sort the nodes when you need stable output.

**Steps:**

1. `map[int][]int` maps each node to its out-neighbours.
2. Collect and sort the keys for deterministic printing.
3. Print each node and its neighbour list.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	// The most common graph representation: a map from each node to its list of
	// neighbours. This directed graph has edges 1->2, 1->3, 2->3, 3->4.
	graph := map[int][]int{
		1: {2, 3},
		2: {3},
		3: {4},
		4: {},
	}

	// Map iteration is random, so sort the nodes for stable output.
	nodes := make([]int, 0, len(graph))
	for n := range graph {
		nodes = append(nodes, n)
	}
	sort.Ints(nodes)
	for _, n := range nodes {
		fmt.Printf("%d -> %v\n", n, graph[n])
	}
}
```

**Output:**

```
1 -> [2 3]
2 -> [3]
3 -> [4]
4 -> []
```

---

## 2. Adjacency list as a slice

`🟢 easy` · *Representation*

When nodes are numbered `0..n-1`, a `[][]int` beats a map: index `i` holds node `i`'s neighbours. It's simpler, faster, and cache-friendlier — no hashing — and it iterates in node order for free. This is the representation the rest of the tier uses.

**Steps:**

1. Each slot of the outer slice is one node's neighbour list.
2. `range` gives both the node index and its neighbours.
3. Node 3 has no out-edges, so its list is empty.

```go
package main

import "fmt"

func main() {
	// When nodes are 0..n-1, a [][]int is simpler and faster than a map: index i
	// holds node i's neighbours. Edges: 0->1, 0->2, 1->3, 2->3.
	graph := [][]int{
		{1, 2}, // node 0
		{3},    // node 1
		{3},    // node 2
		{},     // node 3
	}
	for node, neighbors := range graph {
		fmt.Printf("%d -> %v\n", node, neighbors)
	}
}
```

**Output:**

```
0 -> [1 2]
1 -> [3]
2 -> [3]
3 -> []
```

---

## 3. Adjacency matrix

`🟢 easy` · *Representation*

The third representation: a 2-D boolean grid where `adj[i][j]` is `true` iff there's an edge `i→j`. Edge lookups are **O(1)**, but it costs **O(V²)** memory whether the graph is dense or sparse — so it only pays off for dense graphs.

**Steps:**

1. `[n][n]bool` — set `adj[i][j] = true` per edge.
2. Scan the grid to list edges.
3. `adj[0][1]` vs `adj[1][0]` shows the graph is directed.

```go
package main

import "fmt"

func main() {
	// An adjacency matrix: adj[i][j] == true means an edge i->j. It uses O(V^2)
	// memory but gives O(1) edge lookups, so it suits dense graphs. Edges: 0->1,
	// 1->2, 2->0.
	const n = 3
	var adj [n][n]bool
	adj[0][1] = true
	adj[1][2] = true
	adj[2][0] = true

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if adj[i][j] {
				fmt.Printf("edge %d -> %d\n", i, j)
			}
		}
	}
	fmt.Println("0->1?", adj[0][1], "| 1->0?", adj[1][0])
}
```

**Output:**

```
edge 0 -> 1
edge 1 -> 2
edge 2 -> 0
0->1? true | 1->0? false
```

---

## 4. Directed vs undirected edges

`🟢 easy` · *Edges*

The whole difference between the two graph kinds is **one line**. A directed edge adds `u→v`; an undirected edge adds it **both ways** (`u→v` and `v→u`). Everything downstream — traversal, components, cycles — treats them identically.

**Steps:**

1. `addDirected` appends `v` to `g[u]` only.
2. `addUndirected` also appends `u` to `g[v]`.
3. Node 1 ends up with both the reverse of the undirected edge and its own directed edge.

```go
package main

import "fmt"

// addDirected adds a one-way edge; addUndirected adds it both ways. That is the
// only structural difference between the two graph kinds.
func addDirected(g [][]int, u, v int) {
	g[u] = append(g[u], v)
}

func addUndirected(g [][]int, u, v int) {
	g[u] = append(g[u], v)
	g[v] = append(g[v], u)
}

func main() {
	g := make([][]int, 3)
	addUndirected(g, 0, 1) // edge both ways
	addDirected(g, 1, 2)   // one way only
	for node, nb := range g {
		fmt.Printf("%d -> %v\n", node, nb)
	}
}
```

**Output:**

```
0 -> [1]
1 -> [0 2]
2 -> []
```

---

## 5. Node degree

`🟢 easy` · *Edges*

A node's **out-degree** is how many edges leave it (the length of its neighbour list); its **in-degree** is how many edges point *to* it (how many lists contain it). In-degree drives Kahn's topological sort later ([14](2-medium.md#14-topological-sort-kahns-algorithm)).

**Steps:**

1. Out-degree is just `len(g[v])`.
2. In-degree scans every list counting appearances of `v`.
3. Node 2 is a sink (out 0) with three incoming edges.

```go
package main

import "fmt"

// In a directed graph, out-degree is the length of a node's neighbour list, and
// in-degree is how many lists contain it.
func outDegree(g [][]int, v int) int { return len(g[v]) }

func inDegree(g [][]int, v int) int {
	count := 0
	for _, neighbors := range g {
		for _, n := range neighbors {
			if n == v {
				count++
			}
		}
	}
	return count
}

func main() {
	// 0->1, 0->2, 1->2, 3->2
	g := [][]int{
		{1, 2},
		{2},
		{},
		{2},
	}
	for v := range g {
		fmt.Printf("node %d: out=%d in=%d\n", v, outDegree(g, v), inDegree(g, v))
	}
}
```

**Output:**

```
node 0: out=2 in=0
node 1: out=1 in=1
node 2: out=0 in=3
node 3: out=1 in=0
```

---

## 6. BFS traversal

`🟢 easy` · *BFS*

Breadth-first search explores level by level using a **queue** ([50](../50-linear-structures/)). The one rule that trips people up: mark a node visited **when you enqueue it**, not when you dequeue it — otherwise it gets enqueued multiple times. BFS reaches nodes in order of distance, which is why it solves unweighted shortest paths.

**Steps:**

1. Start with the source in the queue, marked visited.
2. Dequeue a node, record it, enqueue its unvisited neighbours (marking them).
3. The visit order fans out level by level.

```go
package main

import "fmt"

// bfs visits nodes in breadth-first order from src: a queue holds the frontier,
// a visited set prevents revisiting. Neighbours are explored level by level.
func bfs(g [][]int, src int) []int {
	visited := make([]bool, len(g))
	queue := []int{src}
	visited[src] = true
	var order []int
	for len(queue) > 0 {
		node := queue[0] // dequeue the front
		queue = queue[1:]
		order = append(order, node)
		for _, nb := range g[node] {
			if !visited[nb] {
				visited[nb] = true // mark on enqueue, not dequeue
				queue = append(queue, nb)
			}
		}
	}
	return order
}

func main() {
	// 0-1, 0-2, 1-3, 2-3, 3-4 (undirected, sorted neighbours)
	g := [][]int{
		{1, 2},
		{0, 3},
		{0, 3},
		{1, 2, 4},
		{3},
	}
	fmt.Println("BFS from 0:", bfs(g, 0))
}
```

**Output:**

```
BFS from 0: [0 1 2 3 4]
```

---

## 7. DFS traversal

`🟢 easy` · *DFS*

Depth-first search goes as **deep** as it can before backtracking. The most natural form is recursion — the call stack *is* the stack. A `visited` set stops it looping on cycles. Compare the order with BFS: DFS dives down one branch fully before exploring the next.

**Steps:**

1. Mark the node visited and record it.
2. Recurse into each unvisited neighbour.
3. The order plunges deep (`0→1→3→2`) before surfacing.

```go
package main

import "fmt"

// dfs visits as deep as possible before backtracking. The recursion stack is the
// call stack; a visited set stops loops.
func dfs(g [][]int, node int, visited []bool, order *[]int) {
	visited[node] = true
	*order = append(*order, node)
	for _, nb := range g[node] {
		if !visited[nb] {
			dfs(g, nb, visited, order)
		}
	}
}

func main() {
	// 0-1, 0-2, 1-3, 2-3, 3-4 (undirected, sorted neighbours)
	g := [][]int{
		{1, 2},
		{0, 3},
		{0, 3},
		{1, 2, 4},
		{3},
	}
	visited := make([]bool, len(g))
	var order []int
	dfs(g, 0, visited, &order)
	fmt.Println("DFS from 0:", order)
}
```

**Output:**

```
DFS from 0: [0 1 3 2 4]
```

---

## 8. DFS with an explicit stack

`🟢 easy` · *DFS*

Recursion can overflow the goroutine stack on a huge or adversarial graph, so it's worth knowing the **iterative** form: a `[]int` used as a stack ([50](../50-linear-structures/)). Push neighbours in **reverse** so the first neighbour is popped first — that makes the order match recursive DFS.

**Steps:**

1. Push the source; loop while the stack is non-empty.
2. Pop, skip if already visited, else mark and record.
3. Push unvisited neighbours in reverse index order.

```go
package main

import "fmt"

// dfsIterative replaces recursion with an explicit stack. To match recursive DFS
// order, push neighbours in reverse so the first neighbour is popped first.
func dfsIterative(g [][]int, src int) []int {
	visited := make([]bool, len(g))
	stack := []int{src}
	var order []int
	for len(stack) > 0 {
		node := stack[len(stack)-1] // pop the top
		stack = stack[:len(stack)-1]
		if visited[node] {
			continue
		}
		visited[node] = true
		order = append(order, node)
		for i := len(g[node]) - 1; i >= 0; i-- { // reverse push
			if !visited[g[node][i]] {
				stack = append(stack, g[node][i])
			}
		}
	}
	return order
}

func main() {
	g := [][]int{
		{1, 2},
		{0, 3},
		{0, 3},
		{1, 2, 4},
		{3},
	}
	fmt.Println("iterative DFS from 0:", dfsIterative(g, 0))
}
```

**Output:**

```
iterative DFS from 0: [0 1 3 2 4]
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
