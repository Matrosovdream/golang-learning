# Step 09 — Structs · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Declaring a struct + keyed literal](1-easy.md#1-declaring-a-struct--keyed-literal)
- [2. Positional literal](1-easy.md#2-positional-literal)
- [3. Zero value of a struct](1-easy.md#3-zero-value-of-a-struct)
- [4. Accessing and mutating fields](1-easy.md#4-accessing-and-mutating-fields)
- [5. Pointer to a struct + auto-deref](1-easy.md#5-pointer-to-a-struct--auto-deref)

### 🟡 [Medium](2-medium.md)

- [6. Structs are copied by value](2-medium.md#6-structs-are-copied-by-value)
- [7. Pass by value vs by pointer](2-medium.md#7-pass-by-value-vs-by-pointer)
- [8. The &T{} pointer literal](2-medium.md#8-the-t-pointer-literal)
- [9. new(T) for a struct](2-medium.md#9-newt-for-a-struct)
- [10. Methods with a value receiver](2-medium.md#10-methods-with-a-value-receiver)
- [11. Methods with a pointer receiver](2-medium.md#11-methods-with-a-pointer-receiver)
- [12. Nested structs](2-medium.md#12-nested-structs)
- [13. Slice of structs: the range-copy gotcha](2-medium.md#13-slice-of-structs-the-range-copy-gotcha)
- [14. Map values are not addressable](2-medium.md#14-map-values-are-not-addressable)
- [15. Anonymous structs](2-medium.md#15-anonymous-structs)
- [16. Struct comparability with ==](2-medium.md#16-struct-comparability-with-)
- [17. A slice/map field breaks comparability](2-medium.md#17-a-slicemap-field-breaks-comparability)

### 🔴 [Hard](3-hard.md)

- [18. Embedding: field promotion](3-hard.md#18-embedding-field-promotion)
- [19. Embedding: method promotion](3-hard.md#19-embedding-method-promotion)
- [20. Embedding: shadowing](3-hard.md#20-embedding-shadowing)
- [21. Embedding to satisfy an interface](3-hard.md#21-embedding-to-satisfy-an-interface)
- [22. JSON: Marshal with struct tags](3-hard.md#22-json-marshal-with-struct-tags)
- [23. JSON: Unmarshal into a struct](3-hard.md#23-json-unmarshal-into-a-struct)
- [24. JSON: omitempty and json:"-"](3-hard.md#24-json-omitempty-and-json-)
- [25. Constructor functions (NewX)](3-hard.md#25-constructor-functions-newx)
- [26. The empty struct struct{}](3-hard.md#26-the-empty-struct-struct)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
