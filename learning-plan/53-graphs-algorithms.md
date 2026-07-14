# 53 — Graphs & Graph Algorithms

> The capstone of **Part 10 — Data Structures**, alongside [42 — Trees](42-trees.md), [50 — Linear Structures](50-linear-structures.md), [51 — Sorting & Searching](51-sorting-searching.md), and [52 — Heaps & Priority Queues](52-heaps-priority-queues.md). It **reuses the whole track**: BFS needs a queue and DFS a stack ([50](50-linear-structures.md)); Dijkstra and Prim need a heap ([52](52-heaps-priority-queues.md)); Kruskal needs a sort ([51](51-sorting-searching.md)); a tree ([42](42-trees.md)) is just an acyclic graph. Thesis: **most graph problems are a traversal (BFS or DFS) over an adjacency list, plus one of a small set of named algorithms — topological sort, union-find, Dijkstra, MST.**

## Goals
- Represent a graph three ways — **adjacency list** (`map` or `[][]int`), **adjacency matrix**, edge list — and pick the right one; handle **directed vs undirected**, weighted vs unweighted.
- Traverse with **BFS** (a queue) and **DFS** (recursion or an explicit stack), and use them for reachability, **shortest path in unweighted graphs**, and **connected components**.
- Detect **cycles** (directed via colors, undirected via DFS-parent or union-find) and produce a **topological order** (Kahn's BFS and DFS post-order).
- Implement **Union-Find** (disjoint-set union) with path compression + union by rank, and apply it to components, cycles, and **Kruskal's MST**.
- Run the weighted classics: **Dijkstra** (shortest path) and **Prim/Kruskal** (minimum spanning tree), plus grid/implicit graphs (islands, multi-source BFS, word ladder).

## Concepts

- **The adjacency list is the default representation.** Each node maps to its neighbours: `map[T][]T` for arbitrary keys, or `[][]int` when nodes are `0..n-1` (simpler, faster, cache-friendly). It's O(V+E) memory and iterating a node's edges is O(degree). An **adjacency matrix** (`[n][n]bool`) gives O(1) edge lookups but O(V²) memory — only for dense graphs.
- **Directed vs undirected is one line of code.** An undirected edge is just a directed edge added **both ways** (`g[u]=append(g[u],v); g[v]=append(g[v],u)`). Everything downstream (traversal, components) is the same.
- **BFS uses a queue and explores in levels.** Mark a node visited **when you enqueue it** (not when you dequeue), or you'll enqueue duplicates. Because BFS reaches nodes in increasing distance, the first time it touches a node is via a **shortest path** — that's why BFS solves unweighted shortest path, and record a `parent` to reconstruct the path.
- **DFS goes deep, then backtracks.** Natural as recursion (the call stack *is* the stack); convert to an **explicit stack** when recursion could blow the stack or you want to pause/resume ([50](50-linear-structures.md)). A `visited` set stops infinite loops on cyclic graphs.
- **Cycle detection differs by direction.** **Directed:** 3-color DFS — white (unseen), gray (on the current path), black (done); an edge to a **gray** node is a back edge = cycle. **Undirected:** DFS carrying the parent — an edge to a visited node that **isn't the parent** is a cycle (or use union-find: an edge joining two already-connected nodes closes a cycle).
- **A topological sort orders a DAG so every edge points forward.** Two ways: **Kahn's** (BFS) repeatedly emits an in-degree-0 node and decrements its neighbours — if it can't emit all nodes, there's a **cycle**; **DFS** appends each node in **post-order** and reverses. Used for build systems, task scheduling, course prerequisites.
- **Union-Find (disjoint-set union) answers "are these two connected?" in near-O(1).** Each element points at a parent; `Find` follows to the root (the set's representative), and `Union` links two roots. Two optimisations make it almost constant: **path compression** (flatten on the way up) and **union by rank/size** (attach the smaller tree under the larger). It's the engine of Kruskal's MST and dynamic-connectivity problems.
- **Weighted shortest path = Dijkstra.** A min-heap keyed on distance always expands the **closest unsettled node**; relax its edges to improve neighbours ([52](52-heaps-priority-queues.md)). Correct for **non-negative** weights. (Negative edges need Bellman-Ford.)
- **Minimum spanning tree = Kruskal or Prim.** **Kruskal:** sort all edges by weight, add each edge whose endpoints are in different components (union-find rejects cycle-forming edges). **Prim:** grow from one node, using a heap to pick the cheapest edge leaving the current tree. Both give a minimum-total-weight tree.
- **Many problems are graphs in disguise.** A **grid** is a graph where each cell links to its neighbours (islands = connected components, shortest path = BFS, **multi-source BFS** seeds all sources at once). A **word ladder** is an *implicit* graph — you generate neighbours (words one letter away) on the fly instead of storing edges.
- **Complexity:** BFS/DFS = **O(V+E)**; topological sort = O(V+E); union-find op = ~O(α(n)) ≈ O(1); Dijkstra with a heap = **O(E log V)**; Kruskal = O(E log E) (the sort); Prim with a heap = O(E log V).

## Exercises
1. Build a graph as `map[int][]int`, as `[][]int`, and as an adjacency matrix; add a directed and an undirected edge and print each node's neighbours.
2. Compute a node's out-degree and in-degree.
3. Write **BFS** and **DFS** (recursive) traversals from a source and compare their visit order; then write DFS with an **explicit stack** and match the recursive order.
4. Use BFS for **unweighted shortest distances** from a source, then reconstruct an actual shortest **path** with a parent array.
5. Count **connected components** of an undirected graph with DFS.
6. Detect a cycle in a **directed** graph (3-color DFS) and in an **undirected** graph (DFS + parent).
7. Produce a **topological order** two ways: Kahn's (in-degree BFS) and DFS post-order; make Kahn's report a cycle.
8. Check whether a graph is **bipartite** (2-coloring BFS).
9. Implement **Union-Find** with path compression + union by rank; use it to count components and to find the edge that creates a cycle.
10. Stretch — pick two: **Dijkstra** (weighted shortest path), **Kruskal's** MST, **Prim's** MST, **islands** on a grid (flood fill), **multi-source BFS**, or a **word ladder** (implicit-graph BFS).

## Best Practices & Pitfalls
- **Default to an adjacency list; reach for a matrix only when dense.** For most real graphs `[][]int` or `map[T][]T` is the right call — a matrix wastes O(V²) memory on sparse graphs.
- **Pitfall — marking visited on dequeue in BFS.** Mark a node the moment you **enqueue** it; marking on dequeue lets it be enqueued many times, blowing up the queue and the runtime.
- **Pitfall — forgetting `visited` on a cyclic graph.** DFS/BFS without a visited set loops forever on any cycle. On a tree you can skip it; on a general graph you can't.
- **Match the cycle-detection method to the direction.** The undirected "visited ≠ parent" test gives false positives on directed graphs; the directed gray-node test needs the recursion-stack color, not just "visited". Don't mix them up.
- **Sort neighbours (or use slice adjacency) when output must be deterministic.** Map iteration order is random, so BFS/DFS order — and any test or golden file over it — is unstable unless you impose an order.
- **Union-Find needs both optimisations to be fast.** Path compression *and* union by rank/size together give near-constant time; with neither, `Find` degrades to O(n) and union-find loses its point.
- **Dijkstra breaks on negative edges.** The greedy "closest node is settled" assumption fails; use Bellman-Ford for negative weights. Also prefer **lazy deletion** (push duplicates, skip stale pops) over decrease-key — it's simpler in Go ([52](52-heaps-priority-queues.md)).
- **Pitfall — recursion depth on huge graphs.** Deep recursive DFS can overflow the goroutine stack; switch to an explicit stack for very large or adversarial inputs.

## Checklist
- [ ] I can represent a graph as an adjacency list/matrix and add directed/undirected edges.
- [ ] I can write BFS and DFS (recursive and iterative) and know when to use each.
- [ ] I can find unweighted shortest paths (BFS + parent) and connected components.
- [ ] I can detect cycles in directed and undirected graphs and produce a topological order two ways.
- [ ] I can implement Union-Find with path compression + union by rank and apply it (components, Kruskal).
- [ ] I can run Dijkstra for weighted shortest paths and build an MST with Kruskal or Prim.
- [ ] I recognise grids and implicit graphs (islands, multi-source BFS, word ladder) as graph problems.

## Resources
- Go by Example — Maps (adjacency lists): https://gobyexample.com/maps
- `container/heap` (for Dijkstra/Prim): https://pkg.go.dev/container/heap
- Wikipedia — BFS: https://en.wikipedia.org/wiki/Breadth-first_search · DFS: https://en.wikipedia.org/wiki/Depth-first_search
- Wikipedia — Topological sorting: https://en.wikipedia.org/wiki/Topological_sorting · Disjoint-set data structure: https://en.wikipedia.org/wiki/Disjoint-set_data_structure
- Wikipedia — Dijkstra: https://en.wikipedia.org/wiki/Dijkstra%27s_algorithm · Minimum spanning tree: https://en.wikipedia.org/wiki/Minimum_spanning_tree
- Examples: [examples/53-graphs-algorithms](examples/53-graphs-algorithms/).
- Related in this plan: the queue/stack BFS & DFS use in [50 — Linear Structures](50-linear-structures.md); the heap behind Dijkstra/Prim in [52 — Heaps & Priority Queues](52-heaps-priority-queues.md); trees as acyclic graphs in [42 — Trees](42-trees.md).
