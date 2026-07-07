# Step 32 — Hexagonal (Ports & Adapters) · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[32-hexagonal-ports-adapters.md](../../32-hexagonal-ports-adapters.md): driven vs driving ports,
adapters, dependency inversion, in-memory fakes, the composition root, and testing the core with no
infrastructure.

## One-time setup

```bash
mkdir -p /tmp/hex-ex && cd /tmp/hex-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — the driving-adapter examples use `net/http` +
`net/http/httptest`, so nothing leaves the process.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — ports, adapters, the composition root
- [1. A driven port + an in-memory adapter](1-easy.md#1-a-driven-port--an-in-memory-adapter)
- [2. Swap the adapter without touching the core](1-easy.md#2-swap-the-adapter-without-touching-the-core)
- [3. Two adapters for one port](1-easy.md#3-two-adapters-for-one-port)
- [4. A driving port the core implements](1-easy.md#4-a-driving-port-the-core-implements)
- [5. The composition root](1-easy.md#5-the-composition-root)

### 🟡 [Medium](2-medium.md) — driving adapters & testing
- [6. HTTP driving adapter (httptest)](2-medium.md#6-http-driving-adapter-httptest)
- [7. A second driving adapter (CLI) for the same port](2-medium.md#7-a-second-driving-adapter-cli-for-the-same-port)
- [8. Test the core with an in-memory fake](2-medium.md#8-test-the-core-with-an-in-memory-fake)
- [9. Small, consumer-defined ports](2-medium.md#9-small-consumer-defined-ports)
- [10. Dependency inversion: arrows point inward](2-medium.md#10-dependency-inversion-arrows-point-inward)

### 🔴 [Hard](3-hard.md) — full hexagon, translation, capstone
- [11. A full mini hexagon](3-hard.md#11-a-full-mini-hexagon)
- [12. Swap a driven adapter for a decorated one](3-hard.md#12-swap-a-driven-adapter-for-a-decorated-one)
- [13. Error translation at the boundary](3-hard.md#13-error-translation-at-the-boundary)
- [14. A failable dependency + a fake adapter](3-hard.md#14-a-failable-dependency--a-fake-adapter)
- [15. Capstone: HTTP → use case → repo + events](3-hard.md#15-capstone-http--use-case--repo--events)
