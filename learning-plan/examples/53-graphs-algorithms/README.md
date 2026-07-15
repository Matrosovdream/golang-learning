# Step 53 — Graphs & Graph Algorithms · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. The capstone of the DSA track: it reuses queues/stacks ([50](../50-linear-structures/)), heaps ([52](../52-heaps-priority-queues/)), and sorting ([51](../51-sorting-searching/)).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Adjacency list with a map](1-easy.md#1-adjacency-list-with-a-map)
- [2. Adjacency list as a slice](1-easy.md#2-adjacency-list-as-a-slice)
- [3. Adjacency matrix](1-easy.md#3-adjacency-matrix)
- [4. Directed vs undirected edges](1-easy.md#4-directed-vs-undirected-edges)
- [5. Node degree](1-easy.md#5-node-degree)
- [6. BFS traversal](1-easy.md#6-bfs-traversal)
- [7. DFS traversal](1-easy.md#7-dfs-traversal)
- [8. DFS with an explicit stack](1-easy.md#8-dfs-with-an-explicit-stack)

### 🟡 [Medium](2-medium.md)

- [9. Unweighted shortest distances](2-medium.md#9-unweighted-shortest-distances)
- [10. Reconstruct the shortest path](2-medium.md#10-reconstruct-the-shortest-path)
- [11. Connected components](2-medium.md#11-connected-components)
- [12. Cycle detection: directed](2-medium.md#12-cycle-detection-directed)
- [13. Cycle detection: undirected](2-medium.md#13-cycle-detection-undirected)
- [14. Topological sort: Kahn's algorithm](2-medium.md#14-topological-sort-kahns-algorithm)
- [15. Topological sort: DFS post-order](2-medium.md#15-topological-sort-dfs-post-order)
- [16. Bipartite check](2-medium.md#16-bipartite-check)
- [17. Count islands on a grid](2-medium.md#17-count-islands-on-a-grid)

### 🔴 [Hard](3-hard.md)

- [18. Union-Find (disjoint-set union)](3-hard.md#18-union-find-disjoint-set-union)
- [19. Components with Union-Find](3-hard.md#19-components-with-union-find)
- [20. Cycle detection with Union-Find](3-hard.md#20-cycle-detection-with-union-find)
- [21. Dijkstra's shortest path](3-hard.md#21-dijkstras-shortest-path)
- [22. Kruskal's minimum spanning tree](3-hard.md#22-kruskals-minimum-spanning-tree)
- [23. Prim's minimum spanning tree](3-hard.md#23-prims-minimum-spanning-tree)
- [24. Multi-source BFS](3-hard.md#24-multi-source-bfs)
- [25. A generic Graph[T]](3-hard.md#25-a-generic-grapht)
- [26. Word ladder](3-hard.md#26-word-ladder)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
