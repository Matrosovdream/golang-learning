# Step 34 — Event-Driven Architecture & the Outbox · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[34-event-driven-outbox.md](../../34-event-driven-outbox.md): commands vs events, at-least-once
delivery, idempotent consumers, the dual-write problem, and the transactional outbox.

## One-time setup

```bash
mkdir -p /tmp/eda-ex && cd /tmp/eda-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — the broker, DB, and outbox are small **in-memory
stand-ins** for Kafka/NATS + Postgres, so the delivery semantics (at-least-once, duplicates, the
outbox relay) are demonstrated with no infrastructure.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — bus, commands vs events
- [1. An in-memory event bus](1-easy.md#1-an-in-memory-event-bus)
- [2. Commands vs events](1-easy.md#2-commands-vs-events)
- [3. Fan-out to many subscribers](1-easy.md#3-fan-out-to-many-subscribers)
- [4. Typed events + dispatch](1-easy.md#4-typed-events--dispatch)
- [5. Notification vs state transfer](1-easy.md#5-notification-vs-state-transfer)

### 🟡 [Medium](2-medium.md) — delivery & idempotency
- [6. At-least-once delivery (redelivery)](2-medium.md#6-at-least-once-delivery-redelivery)
- [7. Idempotent consumer (dedup by id)](2-medium.md#7-idempotent-consumer-dedup-by-id)
- [8. Idempotency via a naturally-idempotent op](2-medium.md#8-idempotency-via-a-naturally-idempotent-op)
- [9. The dual-write problem](2-medium.md#9-the-dual-write-problem)
- [10. Ack order matters](2-medium.md#10-ack-order-matters)

### 🔴 [Hard](3-hard.md) — outbox, partitions, DLQ, capstone
- [11. The transactional outbox](3-hard.md#11-the-transactional-outbox)
- [12. The outbox relay is at-least-once](3-hard.md#12-the-outbox-relay-is-at-least-once)
- [13. Partition ordering](3-hard.md#13-partition-ordering)
- [14. Dead-letter queue](3-hard.md#14-dead-letter-queue)
- [15. Capstone: the full outbox flow](3-hard.md#15-capstone-the-full-outbox-flow)
