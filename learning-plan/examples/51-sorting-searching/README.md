# Step 51 — Sorting & Searching · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. Uses the generic **`slices`**/**`cmp`** packages (Go 1.21+).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Sort a slice with slices.Sort](1-easy.md#1-sort-a-slice-with-slicessort)
- [2. Sort in reverse (descending)](1-easy.md#2-sort-in-reverse-descending)
- [3. Sort structs by a field](1-easy.md#3-sort-structs-by-a-field)
- [4. Multi-key sort with cmp.Or](1-easy.md#4-multi-key-sort-with-cmpor)
- [5. Stable sort](1-easy.md#5-stable-sort)
- [6. Binary search a sorted slice](1-easy.md#6-binary-search-a-sorted-slice)
- [7. Handy slices helpers](1-easy.md#7-handy-slices-helpers)
- [8. The classic sort package](1-easy.md#8-the-classic-sort-package)

### 🟡 [Medium](2-medium.md)

- [9. Implement sort.Interface](2-medium.md#9-implement-sortinterface)
- [10. Boundary search with sort.Search](2-medium.md#10-boundary-search-with-sortsearch)
- [11. Binary-search structs by key](2-medium.md#11-binary-search-structs-by-key)
- [12. Two pointers: pair sum](2-medium.md#12-two-pointers-pair-sum)
- [13. Two pointers: remove duplicates](2-medium.md#13-two-pointers-remove-duplicates)
- [14. Two pointers: palindrome](2-medium.md#14-two-pointers-palindrome)
- [15. A set from a map](2-medium.md#15-a-set-from-a-map)
- [16. Frequency counting](2-medium.md#16-frequency-counting)
- [17. Fixed sliding window: max sum](2-medium.md#17-fixed-sliding-window-max-sum)

### 🔴 [Hard](3-hard.md)

- [18. Variable window: longest unique substring](3-hard.md#18-variable-window-longest-unique-substring)
- [19. Variable window: minimum subarray length](3-hard.md#19-variable-window-minimum-subarray-length)
- [20. Two-sum with a hash map](3-hard.md#20-two-sum-with-a-hash-map)
- [21. Group anagrams](3-hard.md#21-group-anagrams)
- [22. Dutch national flag](3-hard.md#22-dutch-national-flag)
- [23. Quickselect: the k-th smallest](3-hard.md#23-quickselect-the-k-th-smallest)
- [24. Merge sort from scratch](3-hard.md#24-merge-sort-from-scratch)
- [25. Merge overlapping intervals](3-hard.md#25-merge-overlapping-intervals)
- [26. Top-K frequent elements](3-hard.md#26-top-k-frequent-elements)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
