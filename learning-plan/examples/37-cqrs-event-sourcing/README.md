# Step 37 — CQRS & Event Sourcing · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[37-cqrs-event-sourcing.md](../../37-cqrs-event-sourcing.md): CQRS and CQRS-lite, event-sourced
aggregates (Apply/decide/replay), optimistic concurrency, snapshots, projections, and upcasting.

## One-time setup

```bash
mkdir -p /tmp/cqrs-ex && cd /tmp/cqrs-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — the event store, streams, and read models are in-memory
slices/maps, so the fold-replay-project mechanics run with no infrastructure.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — CQRS
- [1. CQRS: separate write & read models](1-easy.md#1-cqrs-separate-write--read-models)
- [2. CQRS-lite: one store, a read view](1-easy.md#2-cqrs-lite-one-store-a-read-view)
- [3. A command handler](1-easy.md#3-a-command-handler)
- [4. The query side](1-easy.md#4-the-query-side)
- [5. Eventual consistency](1-easy.md#5-eventual-consistency)

### 🟡 [Medium](2-medium.md) — event sourcing basics
- [6. Apply: state is a fold over events](2-medium.md#6-apply-state-is-a-fold-over-events)
- [7. A command decides events](2-medium.md#7-a-command-decides-events)
- [8. Replay to rebuild state](2-medium.md#8-replay-to-rebuild-state)
- [9. Optimistic concurrency](2-medium.md#9-optimistic-concurrency)
- [10. One store, many aggregates](2-medium.md#10-one-store-many-aggregates)

### 🔴 [Hard](3-hard.md) — snapshots, projections, capstone
- [11. Snapshots](3-hard.md#11-snapshots)
- [12. A projection](3-hard.md#12-a-projection)
- [13. Rebuild a projection](3-hard.md#13-rebuild-a-projection)
- [14. Event versioning (upcasting)](3-hard.md#14-event-versioning-upcasting)
- [15. Capstone: event sourcing + CQRS](3-hard.md#15-capstone-event-sourcing--cqrs)
