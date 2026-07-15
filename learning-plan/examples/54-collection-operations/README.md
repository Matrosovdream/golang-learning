# Step 54 — Collection Operations · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. Uses the generic **`slices`**/**`maps`**/**`cmp`** packages and Go 1.23 **iterators** (`iter`, range-over-func) — needs **Go 1.23+**.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Filter a slice](1-easy.md#1-filter-a-slice)
- [2. Map (transform) a slice](1-easy.md#2-map-transform-a-slice)
- [3. Reduce: sum, product, max](1-easy.md#3-reduce-sum-product-max)
- [4. A generic Filter](1-easy.md#4-a-generic-filter)
- [5. A generic Map (two type params)](1-easy.md#5-a-generic-map-two-type-params)
- [6. A generic Reduce](1-easy.md#6-a-generic-reduce)
- [7. The slices toolbox](1-easy.md#7-the-slices-toolbox)
- [8. Deduplicate a slice](1-easy.md#8-deduplicate-a-slice)

### 🟡 [Medium](2-medium.md)

- [9. GroupBy into buckets](2-medium.md#9-groupby-into-buckets)
- [10. CountBy: a frequency tally](2-medium.md#10-countby-a-frequency-tally)
- [11. KeyBy: build a lookup map](2-medium.md#11-keyby-build-a-lookup-map)
- [12. A generic Set type](2-medium.md#12-a-generic-set-type)
- [13. Set operations: union, intersection, difference](2-medium.md#13-set-operations-union-intersection-difference)
- [14. Partition into two slices](2-medium.md#14-partition-into-two-slices)
- [15. Chunk into batches](2-medium.md#15-chunk-into-batches)
- [16. Flatten a slice of slices](2-medium.md#16-flatten-a-slice-of-slices)
- [17. The maps toolbox](2-medium.md#17-the-maps-toolbox)

### 🔴 [Hard](3-hard.md)

- [18. A Filter → Map → Reduce pipeline](3-hard.md#18-a-filter--map--reduce-pipeline)
- [19. Group and aggregate into a sorted report](3-hard.md#19-group-and-aggregate-into-a-sorted-report)
- [20. UniqueBy: dedup structs by a key](3-hard.md#20-uniqueby-dedup-structs-by-a-key)
- [21. Max and min by a projection](3-hard.md#21-max-and-min-by-a-projection)
- [22. Zip two slices](3-hard.md#22-zip-two-slices)
- [23. Emit a map deterministically](3-hard.md#23-emit-a-map-deterministically)
- [24. Lazy filter/map with iterators](3-hard.md#24-lazy-filtermap-with-iterators)
- [25. Laziness: take from an infinite sequence](3-hard.md#25-laziness-take-from-an-infinite-sequence)
- [26. Capstone: an orders report](3-hard.md#26-capstone-an-orders-report)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
