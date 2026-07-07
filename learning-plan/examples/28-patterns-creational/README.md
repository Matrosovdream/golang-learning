# Step 28 — Creational Patterns (the Go way) · Examples

A library of **17 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[28-patterns-creational.md](../../28-patterns-creational.md): useful zero values, constructors,
functional options, builders, factories/registries, singleton, object pool, and prototype.

## One-time setup

```bash
mkdir -p /tmp/creational-ex && cd /tmp/creational-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under
each one is real stdout. Everything is standard-library only — no `go get` needed. (Example 16 uses
generics and 12 uses goroutines, so **Go 1.22+** is fine; you're on a newer toolchain.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–6 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 7–12 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 13–17 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — zero value, constructors, options
- [1. The useful zero value (no constructor needed)](1-easy.md#1-the-useful-zero-value-no-constructor-needed)
- [2. Constructor with validation → (T, error)](1-easy.md#2-constructor-with-validation--t-error)
- [3. Constructor with sensible defaults](1-easy.md#3-constructor-with-sensible-defaults)
- [4. Your first functional option](1-easy.md#4-your-first-functional-option)
- [5. Multiple options, applied in order](1-easy.md#5-multiple-options-applied-in-order)
- [6. An option that can fail](1-easy.md#6-an-option-that-can-fail)

### 🟡 [Medium](2-medium.md) — builders, factories, singleton
- [7. Fluent builder](2-medium.md#7-fluent-builder)
- [8. Builder that validates in Build()](2-medium.md#8-builder-that-validates-in-build)
- [9. Factory returning an interface](2-medium.md#9-factory-returning-an-interface)
- [10. Registry: a factory open for extension](2-medium.md#10-registry-a-factory-open-for-extension)
- [11. Abstract factory: a matched kit](2-medium.md#11-abstract-factory-a-matched-kit)
- [12. Singleton with sync.Once (concurrent)](2-medium.md#12-singleton-with-synconce-concurrent)

### 🔴 [Hard](3-hard.md) — pool, prototype, advanced options
- [13. Object pool with sync.Pool](3-hard.md#13-object-pool-with-syncpool)
- [14. Prototype: a correct deep Clone](3-hard.md#14-prototype-a-correct-deep-clone)
- [15. Self-referential option that returns an undo](3-hard.md#15-self-referential-option-that-returns-an-undo)
- [16. Functional options on a generic type](3-hard.md#16-functional-options-on-a-generic-type)
- [17. Capstone: a configurable HTTP client](3-hard.md#17-capstone-a-configurable-http-client)
