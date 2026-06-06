# Step 05 — Control Flow · Examples

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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–6 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 7–14 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 15–20 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. if / else if / else](1-easy.md#1-if--else-if--else)
- [2. if with an init statement](1-easy.md#2-if-with-an-init-statement)
- [3. The three-part for loop](1-easy.md#3-the-three-part-for-loop)
- [4. for as a while loop](1-easy.md#4-for-as-a-while-loop)
- [5. for range over a slice](1-easy.md#5-for-range-over-a-slice)
- [6. Infinite loop with break](1-easy.md#6-infinite-loop-with-break)

### 🟡 [Medium](2-medium.md)

- [7. continue skips an iteration](2-medium.md#7-continue-skips-an-iteration)
- [8. range over a map (sort for order)](2-medium.md#8-range-over-a-map-sort-for-order)
- [9. range over a string decodes UTF-8](2-medium.md#9-range-over-a-string-decodes-utf-8)
- [10. range over an integer (Go 1.22+)](2-medium.md#10-range-over-an-integer-go-122)
- [11. switch on a value](2-medium.md#11-switch-on-a-value)
- [12. Tagless switch (switch true)](2-medium.md#12-tagless-switch-switch-true)
- [13. Multiple values in one case](2-medium.md#13-multiple-values-in-one-case)
- [14. switch with an init statement](2-medium.md#14-switch-with-an-init-statement)

### 🔴 [Hard](3-hard.md)

- [15. Type switch over any](3-hard.md#15-type-switch-over-any)
- [16. fallthrough](3-hard.md#16-fallthrough)
- [17. Labeled break / continue](3-hard.md#17-labeled-break--continue)
- [18. defer: LIFO order + argument timing](3-hard.md#18-defer-lifo-order--argument-timing)
- [19. defer + recover: catch a panic](3-hard.md#19-defer--recover-catch-a-panic)
- [20. goto (rarely used)](3-hard.md#20-goto-rarely-used)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
