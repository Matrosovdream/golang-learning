# Step 35 — Sagas & Distributed Transactions · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[35-sagas-distributed-transactions.md](../../35-sagas-distributed-transactions.md): local
transactions + compensations, orchestration vs choreography, idempotency keys, semantic locks, and
saga state/resume.

## One-time setup

```bash
mkdir -p /tmp/saga-ex && cd /tmp/saga-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — the services, broker, and saga state are in-memory
stand-ins, so the coordination and compensation logic runs with no infrastructure.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — steps & compensations
- [1. A linear saga (happy path)](1-easy.md#1-a-linear-saga-happy-path)
- [2. A step and its compensation](1-easy.md#2-a-step-and-its-compensation)
- [3. Orchestration: compensate on failure](1-easy.md#3-orchestration-compensate-on-failure)
- [4. Compensation is semantic undo](1-easy.md#4-compensation-is-semantic-undo)
- [5. Terminal state: completed vs compensated](1-easy.md#5-terminal-state-completed-vs-compensated)

### 🟡 [Medium](2-medium.md) — idempotency & coordination
- [6. Reverse-order compensation](2-medium.md#6-reverse-order-compensation)
- [7. Idempotent steps](2-medium.md#7-idempotent-steps)
- [8. Idempotent compensations](2-medium.md#8-idempotent-compensations)
- [9. Persist & resume](2-medium.md#9-persist--resume)
- [10. Choreography](2-medium.md#10-choreography)

### 🔴 [Hard](3-hard.md) — locks, isolation, capstone
- [11. Semantic lock](3-hard.md#11-semantic-lock)
- [12. Compensation needs captured data](3-hard.md#12-compensation-needs-captured-data)
- [13. The missing isolation](3-hard.md#13-the-missing-isolation)
- [14. Orchestration vs choreography](3-hard.md#14-orchestration-vs-choreography)
- [15. Capstone: a full order saga](3-hard.md#15-capstone-a-full-order-saga)
