# Step 52 — Heaps & Priority Queues · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. The 🟢 easy tier builds a heap **from scratch**; 🟡 medium+ use the stdlib **`container/heap`**.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. A heap is an array](1-easy.md#1-a-heap-is-an-array)
- [2. The heap property](1-easy.md#2-the-heap-property)
- [3. Sift up (insert)](1-easy.md#3-sift-up-insert)
- [4. Sift down (extract)](1-easy.md#4-sift-down-extract)
- [5. A hand-rolled min-heap](1-easy.md#5-a-hand-rolled-min-heap)
- [6. Peek the minimum](1-easy.md#6-peek-the-minimum)
- [7. A max-heap](1-easy.md#7-a-max-heap)
- [8. Heapify in O(n)](1-easy.md#8-heapify-in-on)

### 🟡 [Medium](2-medium.md)

- [9. container/heap: the IntHeap](2-medium.md#9-containerheap-the-intheap)
- [10. heap.Push and heap.Pop](2-medium.md#10-heappush-and-heappop)
- [11. heap.Init on existing data](2-medium.md#11-heapinit-on-existing-data)
- [12. A max-heap with container/heap](2-medium.md#12-a-max-heap-with-containerheap)
- [13. Heap sort](2-medium.md#13-heap-sort)
- [14. A priority queue](2-medium.md#14-a-priority-queue)
- [15. Update a priority with heap.Fix](2-medium.md#15-update-a-priority-with-heapfix)
- [16. Remove an item with heap.Remove](2-medium.md#16-remove-an-item-with-heapremove)
- [17. A generic heap](2-medium.md#17-a-generic-heap)

### 🔴 [Hard](3-hard.md)

- [18. Top-K frequent elements](3-hard.md#18-top-k-frequent-elements)
- [19. The k-th largest element](3-hard.md#19-the-k-th-largest-element)
- [20. Merge k sorted slices](3-hard.md#20-merge-k-sorted-slices)
- [21. Running median with two heaps](3-hard.md#21-running-median-with-two-heaps)
- [22. Dijkstra's shortest path](3-hard.md#22-dijkstras-shortest-path)
- [23. Meeting rooms: minimum rooms](3-hard.md#23-meeting-rooms-minimum-rooms)
- [24. K closest points to origin](3-hard.md#24-k-closest-points-to-origin)
- [25. A generic priority queue](3-hard.md#25-a-generic-priority-queue)
- [26. Huffman coding](3-hard.md#26-huffman-coding)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
