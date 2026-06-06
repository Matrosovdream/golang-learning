# Step 11 — Interfaces · Examples

A library of **25 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–18 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 19–25 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Minimal interface & implicit satisfaction](1-easy.md#1-minimal-interface--implicit-satisfaction)
- [2. Polymorphism: one function, many types](1-easy.md#2-polymorphism-one-function-many-types)
- [3. Interface value is a (type, value) pair](1-easy.md#3-interface-value-is-a-type-value-pair)
- [4. Stringer: fmt uses String() automatically](1-easy.md#4-stringer-fmt-uses-string-automatically)
- [5. The empty interface: any](1-easy.md#5-the-empty-interface-any)
- [6. Safe type assertion (comma-ok)](1-easy.md#6-safe-type-assertion-comma-ok)
- [7. Type switch basics](1-easy.md#7-type-switch-basics)
- [8. Slice of interfaces & a total](1-easy.md#8-slice-of-interfaces--a-total)

### 🟡 [Medium](2-medium.md)

- [9. Method sets: pointer vs value receiver](2-medium.md#9-method-sets-pointer-vs-value-receiver)
- [10. Interface composition by embedding](2-medium.md#10-interface-composition-by-embedding)
- [11. The error interface](2-medium.md#11-the-error-interface)
- [12. sort.Slice with a closure](2-medium.md#12-sortslice-with-a-closure)
- [13. sort.Interface: Len/Less/Swap](2-medium.md#13-sortinterface-lenlessswap)
- [14. io.Writer: write the algorithm once](2-medium.md#14-iowriter-write-the-algorithm-once)
- [15. io.MultiWriter: fan-out](2-medium.md#15-iomultiwriter-fan-out)
- [16. Interface-to-interface assertion](2-medium.md#16-interface-to-interface-assertion)
- [17. Strategy via a map of interfaces](2-medium.md#17-strategy-via-a-map-of-interfaces)
- [18. Accept interfaces, return structs (mini DI)](2-medium.md#18-accept-interfaces-return-structs-mini-di)

### 🔴 [Hard](3-hard.md)

- [19. The typed-nil interface trap](3-hard.md#19-the-typed-nil-interface-trap)
- [20. Interface equality & the uncomparable panic](3-hard.md#20-interface-equality--the-uncomparable-panic)
- [21. Optional interfaces (feature detection / upgrades)](3-hard.md#21-optional-interfaces-feature-detection--upgrades)
- [22. Decorator / middleware chain](3-hard.md#22-decorator--middleware-chain)
- [23. Recursive type switch over any (JSON-like walker)](3-hard.md#23-recursive-type-switch-over-any-json-like-walker)
- [24. Dependency injection with a test fake](3-hard.md#24-dependency-injection-with-a-test-fake)
- [25. Capstone: a tiny plugin system](3-hard.md#25-capstone-a-tiny-plugin-system)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
