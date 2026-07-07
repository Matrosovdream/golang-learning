# Step 31 — Domain-Driven Design (tactical) · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[31-ddd-tactical.md](../../31-ddd-tactical.md): value objects, entities, aggregates & invariants,
domain events, domain services, repositories, factories, and the anti-corruption layer.

## One-time setup

```bash
mkdir -p /tmp/ddd-ex && cd /tmp/ddd-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — no `go get`. (The whole point of DDD's tactical layer is
a **framework-free domain**, so these examples import nothing but the stdlib on purpose.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — value objects & entities
- [1. A value object: Money](1-easy.md#1-a-value-object-money)
- [2. Value objects compare by value](1-easy.md#2-value-objects-compare-by-value)
- [3. Entity identity](1-easy.md#3-entity-identity)
- [4. A constructor enforces invariants](1-easy.md#4-a-constructor-enforces-invariants)
- [5. Rich vs anemic: behaviour on the entity](1-easy.md#5-rich-vs-anemic-behaviour-on-the-entity)

### 🟡 [Medium](2-medium.md) — aggregates, events, repositories
- [6. Aggregate root guards an invariant](2-medium.md#6-aggregate-root-guards-an-invariant)
- [7. Domain events](2-medium.md#7-domain-events)
- [8. Reference other aggregates by ID](2-medium.md#8-reference-other-aggregates-by-id)
- [9. Repository interface + in-memory impl](2-medium.md#9-repository-interface--in-memory-impl)
- [10. Domain service across aggregates](2-medium.md#10-domain-service-across-aggregates)

### 🔴 [Hard](3-hard.md) — factories, ACL, concurrency, capstone
- [11. Factory for a valid aggregate](3-hard.md#11-factory-for-a-valid-aggregate)
- [12. Derived data stays consistent](3-hard.md#12-derived-data-stays-consistent)
- [13. Anti-corruption layer](3-hard.md#13-anti-corruption-layer)
- [14. Optimistic concurrency (version)](3-hard.md#14-optimistic-concurrency-version)
- [15. Capstone: a small order domain](3-hard.md#15-capstone-a-small-order-domain)
