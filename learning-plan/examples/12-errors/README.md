# Step 12 — Errors & Error Handling · Examples

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
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–16 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 17–28 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. error is just an interface](1-easy.md#1-error-is-just-an-interface)
- [2. errors.New for a simple error](1-easy.md#2-errorsnew-for-a-simple-error)
- [3. (result, error) + early return](1-easy.md#3-result-error--early-return)
- [4. nil error means success](1-easy.md#4-nil-error-means-success)
- [5. fmt.Errorf for a formatted error](1-easy.md#5-fmterrorf-for-a-formatted-error)

### 🟡 [Medium](2-medium.md)

- [6. Sentinel errors + errors.Is](2-medium.md#6-sentinel-errors--errorsis)
- [7. errors.New values are distinct](2-medium.md#7-errorsnew-values-are-distinct)
- [8. Wrapping with %w](2-medium.md#8-wrapping-with-w)
- [9. errors.Is sees through a wrap](2-medium.md#9-errorsis-sees-through-a-wrap)
- [10. errors.Unwrap](2-medium.md#10-errorsunwrap)
- [11. %w vs %v](2-medium.md#11-w-vs-v)
- [12. Multi-layer wrapping builds a trace](2-medium.md#12-multi-layer-wrapping-builds-a-trace)
- [13. Custom error types](2-medium.md#13-custom-error-types)
- [14. errors.As extracts a custom type](2-medium.md#14-errorsas-extracts-a-custom-type)
- [15. errors.As with a standard library error](2-medium.md#15-errorsas-with-a-standard-library-error)
- [16. Error message style and composition](2-medium.md#16-error-message-style-and-composition)

### 🔴 [Hard](3-hard.md)

- [17. errors.Join combines multiple errors](3-hard.md#17-errorsjoin-combines-multiple-errors)
- [18. errors.Is on a joined error](3-hard.md#18-errorsis-on-a-joined-error)
- [19. errors.As through a wrap chain](3-hard.md#19-errorsas-through-a-wrap-chain)
- [20. Sentinel 'not found' repository pattern](3-hard.md#20-sentinel-not-found-repository-pattern)
- [21. Custom error carrying structured data](3-hard.md#21-custom-error-carrying-structured-data)
- [22. == misses a wrapped cause](3-hard.md#22--misses-a-wrapped-cause)
- [23. Custom error with an Unwrap method](3-hard.md#23-custom-error-with-an-unwrap-method)
- [24. A custom Is method](3-hard.md#24-a-custom-is-method)
- [25. panic for an impossible state](3-hard.md#25-panic-for-an-impossible-state)
- [26. recover turns a panic into an error](3-hard.md#26-recover-turns-a-panic-into-an-error)
- [27. recover at a boundary (middleware-style)](3-hard.md#27-recover-at-a-boundary-middleware-style)
- [28. Don't ignore errors](3-hard.md#28-dont-ignore-errors)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
