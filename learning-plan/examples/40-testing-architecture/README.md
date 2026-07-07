# Step 40 — Testing Architecture · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[40-testing-architecture.md](../../40-testing-architecture.md): the test pyramid, fakes vs mocks/spies,
injected clocks, `httptest`, golden files, and contract checks.

## One-time setup

```bash
mkdir -p /tmp/test-ex && cd /tmp/test-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** is real stdout.
Standard-library only. These are **runnable `package main` demos** of testing *mechanics* (fakes,
spies, mocks, injected clocks, golden files) rather than `_test.go` files, so you can `go run` each
and watch the technique work. Examples 6 and 12 use `for range n` → **Go 1.22+**.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — the pyramid & test doubles
- [1. The test pyramid](1-easy.md#1-the-test-pyramid)
- [2. A hand-written fake](1-easy.md#2-a-hand-written-fake)
- [3. A stub](1-easy.md#3-a-stub)
- [4. A spy](1-easy.md#4-a-spy)
- [5. A mock](1-easy.md#5-a-mock)

### 🟡 [Medium](2-medium.md) — technique
- [6. Fake (outcome) vs mock (interaction)](2-medium.md#6-fake-outcome-vs-mock-interaction)
- [7. Table-driven tests](2-medium.md#7-table-driven-tests)
- [8. Injected clock](2-medium.md#8-injected-clock)
- [9. httptest handler](2-medium.md#9-httptest-handler)
- [10. Golden files](2-medium.md#10-golden-files)

### 🔴 [Hard](3-hard.md) — determinism, contracts, capstone
- [11. The test-double taxonomy](3-hard.md#11-the-test-double-taxonomy)
- [12. Determinism: no sleeps](3-hard.md#12-determinism-no-sleeps)
- [13. A contract check](3-hard.md#13-a-contract-check)
- [14. A reusable fake](3-hard.md#14-a-reusable-fake)
- [15. Capstone: unit-test a use case](3-hard.md#15-capstone-unit-test-a-use-case)
