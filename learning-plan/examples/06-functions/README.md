# Step 06 — Functions · Examples

A library of **20 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–13 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 14–20 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Declaring and calling a function](1-easy.md#1-declaring-and-calling-a-function)
- [2. Same-type parameter shorthand](1-easy.md#2-same-type-parameter-shorthand)
- [3. Multiple return values](1-easy.md#3-multiple-return-values)
- [4. The (value, error) idiom](1-easy.md#4-the-value-error-idiom)
- [5. Named return values + naked return](1-easy.md#5-named-return-values--naked-return)

### 🟡 [Medium](2-medium.md)

- [6. Variadic functions](2-medium.md#6-variadic-functions)
- [7. Spreading a slice into a variadic](2-medium.md#7-spreading-a-slice-into-a-variadic)
- [8. Functions are values](2-medium.md#8-functions-are-values)
- [9. Functions as arguments (higher-order)](2-medium.md#9-functions-as-arguments-higher-order)
- [10. Returning a function](2-medium.md#10-returning-a-function)
- [11. Closures capture state](2-medium.md#11-closures-capture-state)
- [12. Anonymous functions & IIFE](2-medium.md#12-anonymous-functions--iife)
- [13. Recursion](2-medium.md#13-recursion)

### 🔴 [Hard](3-hard.md)

- [14. Closures over the loop variable (Go 1.22+)](3-hard.md#14-closures-over-the-loop-variable-go-122)
- [15. defer + panic + recover → error](3-hard.md#15-defer--panic--recover--error)
- [16. Higher-order map and filter](3-hard.md#16-higher-order-map-and-filter)
- [17. Method values](3-hard.md#17-method-values)
- [18. Method expressions](3-hard.md#18-method-expressions)
- [19. Mutual recursion](3-hard.md#19-mutual-recursion)
- [20. Two closures sharing one variable](3-hard.md#20-two-closures-sharing-one-variable)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
