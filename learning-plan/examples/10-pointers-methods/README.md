# Step 10 — Pointers & Methods · Examples

A library of **24 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–16 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 17–24 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. & and * basics](1-easy.md#1--and--basics)
- [2. Pointer zero value is nil](1-easy.md#2-pointer-zero-value-is-nil)
- [3. Two pointers can alias one variable](1-easy.md#3-two-pointers-can-alias-one-variable)
- [4. new(T) allocates a zeroed value](1-easy.md#4-newt-allocates-a-zeroed-value)
- [5. Pointer to a struct + auto-deref](1-easy.md#5-pointer-to-a-struct--auto-deref)

### 🟡 [Medium](2-medium.md)

- [6. nil pointer dereference panics](2-medium.md#6-nil-pointer-dereference-panics)
- [7. Pass a pointer to mutate the caller's variable](2-medium.md#7-pass-a-pointer-to-mutate-the-callers-variable)
- [8. Swap two values via pointers](2-medium.md#8-swap-two-values-via-pointers)
- [9. Returning a pointer to a local (escape analysis)](2-medium.md#9-returning-a-pointer-to-a-local-escape-analysis)
- [10. Value vs pointer receiver (mutation)](2-medium.md#10-value-vs-pointer-receiver-mutation)
- [11. Pointer-receiver auto-addressing](2-medium.md#11-pointer-receiver-auto-addressing)
- [12. Pointer to an array element](2-medium.md#12-pointer-to-an-array-element)
- [13. Pointer to a slice element](2-medium.md#13-pointer-to-a-slice-element)
- [14. Modify a slice via index, not the range copy](2-medium.md#14-modify-a-slice-via-index-not-the-range-copy)
- [15. []*T vs []T](2-medium.md#15-t-vs-t)
- [16. map[K]*V to mutate stored values](2-medium.md#16-mapkv-to-mutate-stored-values)

### 🔴 [Hard](3-hard.md)

- [17. Method sets: T vs *T](3-hard.md#17-method-sets-t-vs-t)
- [18. Interface satisfaction needs *T for pointer methods](3-hard.md#18-interface-satisfaction-needs-t-for-pointer-methods)
- [19. Map elements are not addressable](3-hard.md#19-map-elements-are-not-addressable)
- [20. Double pointers (**T)](3-hard.md#20-double-pointers-t)
- [21. Nil receiver methods](3-hard.md#21-nil-receiver-methods)
- [22. Comparing pointers](3-hard.md#22-comparing-pointers)
- [23. &T{} vs new(T)](3-hard.md#23-t-vs-newt)
- [24. Pointer receiver to grow a slice field](3-hard.md#24-pointer-receiver-to-grow-a-slice-field)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
