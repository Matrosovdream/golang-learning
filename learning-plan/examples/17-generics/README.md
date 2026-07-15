# Step 17 — Generics · Examples

A library of **60 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–10 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 11–28 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 29–60 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. A generic function with a type parameter](1-easy.md#1-a-generic-function-with-a-type-parameter)
- [2. The any constraint holds any type](1-easy.md#2-the-any-constraint-holds-any-type)
- [3. Type inference vs explicit type arguments](1-easy.md#3-type-inference-vs-explicit-type-arguments)
- [4. The zero value of a type parameter](1-easy.md#4-the-zero-value-of-a-type-parameter)
- [5. The comparable constraint](1-easy.md#5-the-comparable-constraint)
- [6. Map transform a slice to a new type](1-easy.md#6-map-transform-a-slice-to-a-new-type)
- [7. Filter keep matching elements](1-easy.md#7-filter-keep-matching-elements)
- [8. A type union constraint Number](1-easy.md#8-a-type-union-constraint-number)
- [9. Underlying types and the tilde](1-easy.md#9-underlying-types-and-the-tilde)
- [10. Generic over a map with Keys](1-easy.md#10-generic-over-a-map-with-keys)

### 🟡 [Medium](2-medium.md)

- [11. Min and Max with an Ordered constraint](2-medium.md#11-min-and-max-with-an-ordered-constraint)
- [12. Reduce fold a slice](2-medium.md#12-reduce-fold-a-slice)
- [13. Values from a map](2-medium.md#13-values-from-a-map)
- [14. IndexOf with comparable](2-medium.md#14-indexof-with-comparable)
- [15. Equal compare two slices](2-medium.md#15-equal-compare-two-slices)
- [16. Reverse a slice in place](2-medium.md#16-reverse-a-slice-in-place)
- [17. Unique remove duplicates](2-medium.md#17-unique-remove-duplicates)
- [18. A generic Stack type](2-medium.md#18-a-generic-stack-type)
- [19. A generic Queue type](2-medium.md#19-a-generic-queue-type)
- [20. A generic Pair struct](2-medium.md#20-a-generic-pair-struct)
- [21. A generic Set type](2-medium.md#21-a-generic-set-type)
- [22. GroupBy bucket by key](2-medium.md#22-groupby-bucket-by-key)
- [23. Frequency count occurrences](2-medium.md#23-frequency-count-occurrences)
- [24. Clamp to a range](2-medium.md#24-clamp-to-a-range)
- [25. Chunk into batches](2-medium.md#25-chunk-into-batches)
- [26. A generic Optional type](2-medium.md#26-a-generic-optional-type)
- [27. Flatten a slice of slices](2-medium.md#27-flatten-a-slice-of-slices)
- [28. Associate build a map from a slice](2-medium.md#28-associate-build-a-map-from-a-slice)

### 🔴 [Hard](3-hard.md)

- [29. A generic binary search tree](3-hard.md#29-a-generic-binary-search-tree)
- [30. A generic Result type](3-hard.md#30-a-generic-result-type)
- [31. MapValues transform map values](3-hard.md#31-mapvalues-transform-map-values)
- [32. Memoize cache a function](3-hard.md#32-memoize-cache-a-function)
- [33. Compose two functions](3-hard.md#33-compose-two-functions)
- [34. Zip two slices into pairs](3-hard.md#34-zip-two-slices-into-pairs)
- [35. A generic linked list](3-hard.md#35-a-generic-linked-list)
- [36. A generic typed event bus](3-hard.md#36-a-generic-typed-event-bus)
- [37. A generic LRU cache](3-hard.md#37-a-generic-lru-cache)
- [38. Partition by a predicate](3-hard.md#38-partition-by-a-predicate)
- [39. SortBy a generic key](3-hard.md#39-sortby-a-generic-key)
- [40. Capstone a generic repository and pipeline](3-hard.md#40-capstone-a-generic-repository-and-pipeline)
- [41. Composing constraints by embedding](3-hard.md#41-composing-constraints-by-embedding)
- [42. A self-referential constraint](3-hard.md#42-a-self-referential-constraint)
- [43. All, Any, and None predicates](3-hard.md#43-all-any-and-none-predicates)
- [44. Take, Drop, and TakeWhile](3-hard.md#44-take-drop-and-takewhile)
- [45. FlatMap](3-hard.md#45-flatmap)
- [46. A generic binary min-heap](3-hard.md#46-a-generic-binary-min-heap)
- [47. Pointer helpers Ptr, Deref, and Coalesce](3-hard.md#47-pointer-helpers-ptr-deref-and-coalesce)
- [48. Generic functional options](3-hard.md#48-generic-functional-options)
- [49. Invert a map](3-hard.md#49-invert-a-map)
- [50. Capstone a generic insertion-ordered map](3-hard.md#50-capstone-a-generic-insertion-ordered-map)
- [51. MinBy and MaxBy with a key extractor](3-hard.md#51-minby-and-maxby-with-a-key-extractor)
- [52. CountBy tally with a key function](3-hard.md#52-countby-tally-with-a-key-function)
- [53. Scan running accumulation](3-hard.md#53-scan-running-accumulation)
- [54. Pipe chain functions of one type](3-hard.md#54-pipe-chain-functions-of-one-type)
- [55. Curry a two-argument function](3-hard.md#55-curry-a-two-argument-function)
- [56. Sort with a comparator function](3-hard.md#56-sort-with-a-comparator-function)
- [57. Binary search a sorted slice](3-hard.md#57-binary-search-a-sorted-slice)
- [58. Parallel Map with goroutines](3-hard.md#58-parallel-map-with-goroutines)
- [59. Lazy iterators with iter.Seq](3-hard.md#59-lazy-iterators-with-iterseq)
- [60. Capstone a lazy Filter→Map pipeline](3-hard.md#60-capstone-a-lazy-filtermap-pipeline)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
