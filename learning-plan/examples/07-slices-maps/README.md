# Step 07 — Arrays, Slices & Maps · Examples

A library of **22 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–7 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 8–16 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 17–22 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Arrays have a fixed length](1-easy.md#1-arrays-have-a-fixed-length)
- [2. Arrays are copied by value](1-easy.md#2-arrays-are-copied-by-value)
- [3. Slice literal and indexing](1-easy.md#3-slice-literal-and-indexing)
- [4. len vs cap](1-easy.md#4-len-vs-cap)
- [5. append to a slice](1-easy.md#5-append-to-a-slice)
- [6. Map literal and lookup](1-easy.md#6-map-literal-and-lookup)
- [7. Map comma-ok lookup](1-easy.md#7-map-comma-ok-lookup)

### 🟡 [Medium](2-medium.md)

- [8. Slicing a slice](2-medium.md#8-slicing-a-slice)
- [9. append growth (capacity doubling)](2-medium.md#9-append-growth-capacity-doubling)
- [10. Slice aliasing trap](2-medium.md#10-slice-aliasing-trap)
- [11. copy detaches a slice](2-medium.md#11-copy-detaches-a-slice)
- [12. nil vs empty slice](2-medium.md#12-nil-vs-empty-slice)
- [13. delete from a map](2-medium.md#13-delete-from-a-map)
- [14. Map iteration is unordered](2-medium.md#14-map-iteration-is-unordered)
- [15. Frequency counting with a map](2-medium.md#15-frequency-counting-with-a-map)
- [16. Arrays are comparable and usable as map keys](2-medium.md#16-arrays-are-comparable-and-usable-as-map-keys)

### 🔴 [Hard](3-hard.md)

- [17. nil map: read ok, write panics](3-hard.md#17-nil-map-read-ok-write-panics)
- [18. Three-index slice controls capacity](3-hard.md#18-three-index-slice-controls-capacity)
- [19. Remove an element by index](3-hard.md#19-remove-an-element-by-index)
- [20. 2D / jagged slices](3-hard.md#20-2d--jagged-slices)
- [21. A set via map[T]struct{}](3-hard.md#21-a-set-via-maptstruct)
- [22. Sorting slices](3-hard.md#22-sorting-slices)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
