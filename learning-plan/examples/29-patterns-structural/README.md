# Step 29 — Structural Patterns (the Go way) · Examples

A library of **17 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[29-patterns-structural.md](../../29-patterns-structural.md): embedding, adapters, decorators/
middleware, facade, proxy, composite, bridge, and flyweight.

## One-time setup

```bash
mkdir -p /tmp/structural-ex && cd /tmp/structural-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — no `go get`.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–6 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 7–12 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 13–17 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — embedding & adapters
- [1. Struct embedding: a promoted method](1-easy.md#1-struct-embedding-a-promoted-method)
- [2. Interface embedding (Reader + Writer)](1-easy.md#2-interface-embedding-reader--writer)
- [3. Embed an interface to decorate one method](1-easy.md#3-embed-an-interface-to-decorate-one-method)
- [4. Adapter: a func satisfies an interface (HandlerFunc)](1-easy.md#4-adapter-a-func-satisfies-an-interface-handlerfunc)
- [5. Adapter: wrap a third-party struct](1-easy.md#5-adapter-wrap-a-third-party-struct)
- [6. Embedding + shadowing](1-easy.md#6-embedding--shadowing)

### 🟡 [Medium](2-medium.md) — decorators, facade, proxy
- [7. Function decorator](2-medium.md#7-function-decorator)
- [8. Middleware chain over http.Handler](2-medium.md#8-middleware-chain-over-httphandler)
- [9. Interface decorators that stack](2-medium.md#9-interface-decorators-that-stack)
- [10. Facade over a subsystem](2-medium.md#10-facade-over-a-subsystem)
- [11. Caching proxy](2-medium.md#11-caching-proxy)
- [12. Protection proxy](2-medium.md#12-protection-proxy)

### 🔴 [Hard](3-hard.md) — composite, bridge, flyweight
- [13. Composite: a file tree](3-hard.md#13-composite-a-file-tree)
- [14. Bridge: swap the implementation](3-hard.md#14-bridge-swap-the-implementation)
- [15. Flyweight: string interning](3-hard.md#15-flyweight-string-interning)
- [16. Adapter to io.Writer](3-hard.md#16-adapter-to-iowriter)
- [17. Capstone: layered store decorators](3-hard.md#17-capstone-layered-store-decorators)
