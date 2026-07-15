# Step 53 — Graphs & Graph Algorithms · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟢 easy](1-easy.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Unweighted shortest distances

`🟡 medium` · *BFS*

Because BFS reaches every node in order of distance, the **first** time it touches a node is via a shortest path. Track a distance array (`-1` = unvisited): a neighbour's distance is the current node's plus one. Nodes never reached stay `-1`.

**Steps:**

1. `dist[src] = 0`; everything else `-1`.
2. BFS: set `dist[nb] = dist[node] + 1` the first time you see `nb`.
3. Isolated node 5 stays `-1`.

```go
package main

import "fmt"

// bfsDistances returns the fewest edges from src to every node (or -1 if
// unreachable). BFS explores in level order, so the first time we reach a node is
// via a shortest path.
func bfsDistances(g [][]int, src int) []int {
	dist := make([]int, len(g))
	for i := range dist {
		dist[i] = -1
	}
	dist[src] = 0
	queue := []int{src}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, nb := range g[node] {
			if dist[nb] == -1 {
				dist[nb] = dist[node] + 1
				queue = append(queue, nb)
			}
		}
	}
	return dist
}

func main() {
	// 0-1, 0-2, 1-3, 3-4, node 5 isolated
	g := [][]int{
		{1, 2},
		{0, 3},
		{0},
		{1, 4},
		{3},
		{},
	}
	fmt.Println("distances from 0:", bfsDistances(g, 0))
}
```

**Output:**

```
distances from 0: [0 1 1 2 3 -1]
```

---

## 10. Reconstruct the shortest path

`🟡 medium` · *BFS*

Distances tell you *how far*; a **parent array** tells you the actual route. During BFS, record which node you came from to reach each neighbour. Then walk parents backward from the destination and reverse — that's the shortest path.

**Steps:**

1. `parent[nb] = node` when BFS first reaches `nb`.
2. Stop early once `dst` is dequeued.
3. Follow parents from `dst` back to `src`, then reverse.

```go
package main

import "fmt"

// shortestPath returns an actual shortest path from src to dst by recording each
// node's parent during BFS, then walking parents back from dst.
func shortestPath(g [][]int, src, dst int) []int {
	parent := make([]int, len(g))
	for i := range parent {
		parent[i] = -1
	}
	visited := make([]bool, len(g))
	visited[src] = true
	queue := []int{src}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node == dst {
			break
		}
		for _, nb := range g[node] {
			if !visited[nb] {
				visited[nb] = true
				parent[nb] = node
				queue = append(queue, nb)
			}
		}
	}
	if !visited[dst] {
		return nil // unreachable
	}
	// Walk parents back from dst, then reverse.
	var path []int
	for at := dst; at != -1; at = parent[at] {
		path = append(path, at)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func main() {
	g := [][]int{
		{1, 2},
		{0, 3},
		{0, 3},
		{1, 2, 4},
		{3},
	}
	fmt.Println("path 0 -> 4:", shortestPath(g, 0, 4))
}
```

**Output:**

```
path 0 -> 4: [0 1 3 4]
```

---

## 11. Connected components

`🟡 medium` · *DFS*

In an undirected graph, a **connected component** is a maximal group of mutually reachable nodes. Run a DFS from every not-yet-labelled node, stamping each with the current component id; bump the id after each DFS finishes. The final id count is the number of components.

**Steps:**

1. `comp[v] = -1` marks unlabelled.
2. For each unlabelled node, DFS-flood it with the current id, then increment the id.
3. Two clusters plus an isolated node → three components.

```go
package main

import "fmt"

// components labels each node with its connected-component id by running a DFS
// from every unvisited node, and returns the number of components. Works on
// undirected graphs.
func components(g [][]int) ([]int, int) {
	comp := make([]int, len(g))
	for i := range comp {
		comp[i] = -1
	}
	id := 0
	var dfs func(node int)
	dfs = func(node int) {
		comp[node] = id
		for _, nb := range g[node] {
			if comp[nb] == -1 {
				dfs(nb)
			}
		}
	}
	for v := range g {
		if comp[v] == -1 {
			dfs(v)
			id++
		}
	}
	return comp, id
}

func main() {
	// Two clusters: {0,1,2} and {3,4}, plus isolated {5}.
	g := [][]int{
		{1, 2},
		{0, 2},
		{0, 1},
		{4},
		{3},
		{},
	}
	comp, count := components(g)
	fmt.Println("component ids:", comp)
	fmt.Println("number of components:", count)
}
```

**Output:**

```
component ids: [0 0 0 1 1 2]
number of components: 3
```

---

## 12. Cycle detection: directed

`🟡 medium` · *Cycle*

A directed graph has a cycle iff DFS finds a **back edge** — an edge to a node still on the current recursion path. Track three colors: white (unvisited), **gray** (in progress, on the path), black (finished). Reaching a gray node means a cycle.

**Steps:**

1. Color a node gray when DFS enters it, black when it returns.
2. An edge to a gray node → cycle.
3. The acyclic chain passes; the 0→1→2→0 loop is caught.

```go
package main

import "fmt"

// hasCycleDirected uses three colors: 0=unvisited, 1=in the current DFS path,
// 2=done. Reaching a node that's still "in progress" (gray) means a back edge —
// a cycle.
func hasCycleDirected(g [][]int) bool {
	const white, gray, black = 0, 1, 2
	color := make([]int, len(g))
	var dfs func(node int) bool
	dfs = func(node int) bool {
		color[node] = gray
		for _, nb := range g[node] {
			if color[nb] == gray {
				return true // back edge to a node on the current path
			}
			if color[nb] == white && dfs(nb) {
				return true
			}
		}
		color[node] = black
		return false
	}
	for v := range g {
		if color[v] == white && dfs(v) {
			return true
		}
	}
	return false
}

func main() {
	acyclic := [][]int{{1}, {2}, {}} // 0->1->2
	cyclic := [][]int{{1}, {2}, {0}} // 0->1->2->0
	fmt.Println("acyclic has cycle:", hasCycleDirected(acyclic))
	fmt.Println("cyclic has cycle:", hasCycleDirected(cyclic))
}
```

**Output:**

```
acyclic has cycle: false
cyclic has cycle: true
```

---

## 13. Cycle detection: undirected

`🟡 medium` · *Cycle*

Undirected graphs need a different test — every edge looks like a "back edge" to the node you just came from. So carry the **parent**: an edge to an already-visited node that **isn't** the parent means there's a second way in, i.e. a cycle.

**Steps:**

1. DFS carries the node you came from (`parent`).
2. Visited neighbour `≠` parent → cycle.
3. A tree has none; a triangle does.

```go
package main

import "fmt"

// hasCycleUndirected runs DFS carrying the parent. An edge to an already-visited
// node that ISN'T the parent means we found another way in — a cycle.
func hasCycleUndirected(g [][]int) bool {
	visited := make([]bool, len(g))
	var dfs func(node, parent int) bool
	dfs = func(node, parent int) bool {
		visited[node] = true
		for _, nb := range g[node] {
			if !visited[nb] {
				if dfs(nb, node) {
					return true
				}
			} else if nb != parent {
				return true // visited, not the parent -> cycle
			}
		}
		return false
	}
	for v := range g {
		if !visited[v] && dfs(v, -1) {
			return true
		}
	}
	return false
}

func main() {
	// Tree (no cycle): 0-1, 0-2
	tree := [][]int{{1, 2}, {0}, {0}}
	// Triangle (cycle): 0-1, 1-2, 2-0
	tri := [][]int{{1, 2}, {0, 2}, {0, 1}}
	fmt.Println("tree has cycle:", hasCycleUndirected(tree))
	fmt.Println("triangle has cycle:", hasCycleUndirected(tri))
}
```

**Output:**

```
tree has cycle: false
triangle has cycle: true
```

---

## 14. Topological sort: Kahn's algorithm

`🟡 medium` · *Topo sort*

A topological order lists a DAG's nodes so every edge points **forward** — the order to do tasks with prerequisites. **Kahn's** algorithm is BFS on **in-degree**: start from nodes with in-degree 0, and each time you emit a node, decrement its neighbours' in-degrees, queuing any that hit 0. If you can't emit every node, the graph had a **cycle**.

**Steps:**

1. Compute every node's in-degree.
2. Queue the in-degree-0 nodes; emit one, decrement its neighbours, queue new zeros.
3. Emitting all nodes ⇒ valid DAG; fewer ⇒ a cycle.

```go
package main

import "fmt"

// topoSortKahn orders a DAG so every edge points forward. Kahn's algorithm:
// repeatedly emit a node with in-degree 0 and decrement its neighbours'. If not
// all nodes are emitted, the graph had a cycle.
func topoSortKahn(g [][]int) ([]int, bool) {
	indeg := make([]int, len(g))
	for _, neighbors := range g {
		for _, v := range neighbors {
			indeg[v]++
		}
	}
	var queue []int
	for v := range g {
		if indeg[v] == 0 {
			queue = append(queue, v)
		}
	}
	var order []int
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, nb := range g[node] {
			indeg[nb]--
			if indeg[nb] == 0 {
				queue = append(queue, nb)
			}
		}
	}
	return order, len(order) == len(g) // false => cycle
}

func main() {
	// Prerequisites: 0->1, 0->2, 1->3, 2->3
	g := [][]int{
		{1, 2},
		{3},
		{3},
		{},
	}
	order, ok := topoSortKahn(g)
	fmt.Println("topo order:", order, "valid DAG:", ok)
}
```

**Output:**

```
topo order: [0 1 2 3] valid DAG: true
```

---

## 15. Topological sort: DFS post-order

`🟡 medium` · *Topo sort*

The other route to a topological order: DFS and append each node in **post-order** (after all its descendants), then **reverse**. Because a node is only finished once everything it points to is finished, reversing the finish order puts every node before its dependents.

**Steps:**

1. DFS; append a node *after* recursing into its neighbours.
2. Reverse the resulting post-order.
3. The result is a valid forward ordering of the DAG.

```go
package main

import "fmt"

// topoSortDFS produces a topological order by DFS post-order: a node is appended
// only after all its descendants, so reversing the post-order gives the topo
// order. Assumes a DAG.
func topoSortDFS(g [][]int) []int {
	visited := make([]bool, len(g))
	var order []int
	var dfs func(node int)
	dfs = func(node int) {
		visited[node] = true
		for _, nb := range g[node] {
			if !visited[nb] {
				dfs(nb)
			}
		}
		order = append(order, node) // post-order
	}
	for v := range g {
		if !visited[v] {
			dfs(v)
		}
	}
	// Reverse the post-order.
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

func main() {
	// 2->3, 3->1, 4->0, 4->1, 5->2, 5->0
	g := [][]int{
		{},
		{},
		{3},
		{1},
		{0, 1},
		{2, 0},
	}
	fmt.Println("topo order:", topoSortDFS(g))
}
```

**Output:**

```
topo order: [5 4 2 3 1 0]
```

---

## 16. Bipartite check

`🟡 medium` · *Coloring*

A graph is **bipartite** if you can 2-color it so no edge joins two same-colored nodes — equivalently, it has no odd-length cycle. BFS from each uncolored node, giving every neighbour the **opposite** color; a neighbour that already has the *same* color is a conflict.

**Steps:**

1. Colors are `+1`/`-1`; `0` means uncolored.
2. BFS assigns `-color[node]` to each neighbour.
3. Same color on both ends of an edge ⇒ not bipartite (odd cycle).

```go
package main

import "fmt"

// isBipartite tries to 2-color the graph so no edge joins same-colored nodes.
// BFS assigns the opposite color to each neighbour; a conflict means it's not
// bipartite (it has an odd cycle).
func isBipartite(g [][]int) bool {
	color := make([]int, len(g)) // 0 = uncolored, 1/-1 = the two colors
	for start := range g {
		if color[start] != 0 {
			continue
		}
		color[start] = 1
		queue := []int{start}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			for _, nb := range g[node] {
				if color[nb] == 0 {
					color[nb] = -color[node]
					queue = append(queue, nb)
				} else if color[nb] == color[node] {
					return false // same color on both ends of an edge
				}
			}
		}
	}
	return true
}

func main() {
	// Square 0-1-2-3-0 (even cycle) is bipartite.
	square := [][]int{{1, 3}, {0, 2}, {1, 3}, {0, 2}}
	// Triangle 0-1-2-0 (odd cycle) is not.
	triangle := [][]int{{1, 2}, {0, 2}, {0, 1}}
	fmt.Println("square bipartite:", isBipartite(square))
	fmt.Println("triangle bipartite:", isBipartite(triangle))
}
```

**Output:**

```
square bipartite: true
triangle bipartite: false
```

---

## 17. Count islands on a grid

`🟡 medium` · *Grid*

A grid is a graph in disguise: each `'1'` cell is a node connected to its four neighbours. Counting **islands** (connected groups of land) is just counting connected components. For each unvisited land cell, DFS-**flood-fill** it, sinking the whole island to `'0'` so it isn't counted twice.

**Steps:**

1. Scan every cell; on an unvisited `'1'`, bump the count and flood it.
2. `sink` recurses into the 4 neighbours, marking land as `'0'`.
3. Bounds/water checks are the recursion's base cases.

```go
package main

import "fmt"

// numIslands counts connected groups of '1' cells in a grid (4-directional). Each
// unvisited land cell starts a DFS flood fill that sinks the whole island.
func numIslands(grid [][]byte) int {
	rows, cols := len(grid), len(grid[0])
	var sink func(r, c int)
	sink = func(r, c int) {
		if r < 0 || r >= rows || c < 0 || c >= cols || grid[r][c] != '1' {
			return
		}
		grid[r][c] = '0' // mark visited by sinking it
		sink(r+1, c)
		sink(r-1, c)
		sink(r, c+1)
		sink(r, c-1)
	}
	count := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if grid[r][c] == '1' {
				count++
				sink(r, c)
			}
		}
	}
	return count
}

func main() {
	grid := [][]byte{
		[]byte("11000"),
		[]byte("11000"),
		[]byte("00100"),
		[]byte("00011"),
	}
	fmt.Println("islands:", numIslands(grid)) // 3
}
```

**Output:**

```
islands: 3
```

---

> Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md) · Back to the [index](README.md)
