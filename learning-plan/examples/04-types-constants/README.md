# Step 04 — Variables, Types & Constants · Examples

A library of **28 runnable examples**, split into three files by difficulty. Each is a complete
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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–9 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 10–22 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 23–28 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. The four ways to declare a variable](1-easy.md#1-the-four-ways-to-declare-a-variable)
- [2. Zero values of basic types](1-easy.md#2-zero-values-of-basic-types)
- [3. Basic types and %T](1-easy.md#3-basic-types-and-t)
- [4. Constants: single and grouped](1-easy.md#4-constants-single-and-grouped)
- [5. iota basics](1-easy.md#5-iota-basics)
- [6. Integer division and remainder](1-easy.md#6-integer-division-and-remainder)
- [7. Multiple assignment and the swap idiom](1-easy.md#7-multiple-assignment-and-the-swap-idiom)
- [8. The blank identifier](1-easy.md#8-the-blank-identifier)
- [9. bool: no truthy or falsy](1-easy.md#9-bool-no-truthy-or-falsy)

### 🟡 [Medium](2-medium.md)

- [10. Signed integer overflow wraps around](2-medium.md#10-signed-integer-overflow-wraps-around)
- [11. Unsigned integers wrap (underflow)](2-medium.md#11-unsigned-integers-wrap-underflow)
- [12. No implicit conversion; int/float truncates](2-medium.md#12-no-implicit-conversion-intfloat-truncates)
- [13. string, []byte, and []rune](2-medium.md#13-string-byte-and-rune)
- [14. Integer literal bases](2-medium.md#14-integer-literal-bases)
- [15. Untyped vs typed constants](2-medium.md#15-untyped-vs-typed-constants)
- [16. string(rune) vs strconv.Itoa](2-medium.md#16-stringrune-vs-strconvitoa)
- [17. Variable shadowing in a nested scope](2-medium.md#17-variable-shadowing-in-a-nested-scope)
- [18. Floating-point precision](2-medium.md#18-floating-point-precision)
- [19. Float infinity and NaN](2-medium.md#19-float-infinity-and-nan)
- [20. Rune arithmetic](2-medium.md#20-rune-arithmetic)
- [21. Typed constant overflow is a compile error](2-medium.md#21-typed-constant-overflow-is-a-compile-error)
- [22. The min and max builtins](2-medium.md#22-the-min-and-max-builtins)

### 🔴 [Hard](3-hard.md)

- [23. Named types vs type aliases](3-hard.md#23-named-types-vs-type-aliases)
- [24. iota bit-shift size constants](3-hard.md#24-iota-bit-shift-size-constants)
- [25. Enum with iota + Stringer](3-hard.md#25-enum-with-iota--stringer)
- [26. iota bitmask flags](3-hard.md#26-iota-bitmask-flags)
- [27. Converting between named types](3-hard.md#27-converting-between-named-types)
- [28. Untyped constants have arbitrary precision](3-hard.md#28-untyped-constants-have-arbitrary-precision)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
