# Step 33 — Dependency Injection & Wiring · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[33-dependency-injection.md](../../33-dependency-injection.md): constructor injection, the composition
root, provider functions, and the anti-patterns DI kills.

## One-time setup

```bash
mkdir -p /tmp/di-ex && cd /tmp/di-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only. The `google/wire` and `uber/fx` examples (8, 11) are
**hand-written stand-ins** that show what those tools do — `wire` generates the injector in ex. 8,
`fx` manages the lifecycle hooks in ex. 11 — so you can run them with no external deps.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — constructor injection & the root
- [1. Constructor injection](1-easy.md#1-constructor-injection)
- [2. A testable constructor](1-easy.md#2-a-testable-constructor)
- [3. Inject an interface, swap real/fake](1-easy.md#3-inject-an-interface-swap-realfake)
- [4. The composition root](1-easy.md#4-the-composition-root)
- [5. Several dependencies](1-easy.md#5-several-dependencies)

### 🟡 [Medium](2-medium.md) — providers, wire, anti-patterns
- [6. Provider functions](2-medium.md#6-provider-functions)
- [7. A provider set](2-medium.md#7-a-provider-set)
- [8. A hand-written injector (wire)](2-medium.md#8-a-hand-written-injector-wire)
- [9. Anti-pattern: the service locator](2-medium.md#9-anti-pattern-the-service-locator)
- [10. Anti-pattern: package globals](2-medium.md#10-anti-pattern-package-globals)

### 🔴 [Hard](3-hard.md) — lifecycle, segregation, capstone
- [11. Lifecycle hooks (fx-style)](3-hard.md#11-lifecycle-hooks-fx-style)
- [12. Read config once at the root](3-hard.md#12-read-config-once-at-the-root)
- [13. Interface segregation for testability](3-hard.md#13-interface-segregation-for-testability)
- [14. Decorator wiring at the root](3-hard.md#14-decorator-wiring-at-the-root)
- [15. Capstone: a full composition root](3-hard.md#15-capstone-a-full-composition-root)
