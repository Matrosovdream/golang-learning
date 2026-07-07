# 37 — CQRS & Event Sourcing (advanced)

> Part 9, Track B: [34 Event-Driven & Outbox](34-event-driven-outbox.md) → [35 Sagas](35-sagas-distributed-transactions.md) → [36 Resilience](36-resilience-patterns.md) → **37 CQRS & Event Sourcing**.
> The advanced end of the track — two **separate** ideas often mentioned together. **CQRS** splits reads from writes. **Event Sourcing** stores changes as an event log instead of current state. Both are powerful and both are easy to over-apply, so the "when *not* to" matters as much as the "how".

## Goals
- Separate the **write model** from the **read model** with CQRS, and know the lightweight version most teams actually need.
- Understand **Event Sourcing**: state as a fold over an append-only event log; rebuilds, snapshots, projections.
- See how CQRS, ES, events ([34](34-event-driven-outbox.md)), and DDD aggregates ([31](31-ddd-tactical.md)) fit together.
- Judge **when to use each — and when a boring CRUD table is the right answer.**

## Concepts

### CQRS: Command Query Responsibility Segregation
The one idea: **use different models for changing data and for reading it.** The **command side** takes commands, runs domain logic and invariants ([31](31-ddd-tactical.md)), and persists. The **query side** serves reads from a model shaped for querying — often denormalised, sometimes in a different store:
```go
// Command side — behaviour-rich, validates, returns (almost) nothing.
type PlaceOrder struct{ CustomerID, ... }
func (h OrderCommandHandler) Handle(ctx context.Context, cmd PlaceOrder) error { /* domain */ }

// Query side — dumb, fast, read-optimised DTOs. No domain model, no invariants.
type OrderSummary struct{ ID, Customer, Total, Status string }
func (q OrderQueries) ListForCustomer(ctx context.Context, id string) ([]OrderSummary, error)
```
Why bother: reads and writes have **different shapes and scale** — writes need one consistent aggregate and heavy validation; reads want wide denormalised rows, caching, maybe full-text or a search index. Splitting lets each side be optimised (and scaled) independently.

### CQRS-lite: you probably don't need two databases
The common, sane version: **one database, separate read paths.** Writes go through the domain model; reads bypass it entirely and hit **read-optimised views** — SQL views, materialised views, or hand-written query structs — instead of loading aggregates. No separate store, no eventual consistency, most of the benefit. Reach for two datastores (write DB + read DB kept in sync via events) only when a single store genuinely can't serve both loads.

### Read models are eventually consistent (when split)
Once the read side is a *separate* store updated from events, it lags the write side by milliseconds-to-seconds. The UI must tolerate "I just placed an order but the list doesn't show it yet." Design for it: return the new id from the command so the client can navigate directly, or read-your-writes from the command side for that one case. **Accepting eventual consistency is the price of a split read model** — if you can't, don't split the store.

### Event Sourcing: store the changes, not the state
Instead of storing *current state* and overwriting it, Event Sourcing stores the **full sequence of events** that produced it, append-only. Current state is a **left fold** over the events:
```go
type Account struct {
    id      string
    balance int64
    version int
}

// Apply mutates state for ONE event — the fold step. Pure, no I/O.
func (a *Account) Apply(e any) {
    switch e := e.(type) {
    case Opened:     a.id = e.ID
    case Deposited:  a.balance += e.Amount
    case Withdrawn:  a.balance -= e.Amount
    }
    a.version++
}

// Rebuild = replay the log from zero.
func Load(events []any) *Account {
    a := &Account{}
    for _, e := range events { a.Apply(e) }
    return a
}

// A command DECIDES which new events to emit (validates invariants first):
func (a *Account) Withdraw(amt int64) ([]any, error) {
    if amt > a.balance { return nil, ErrInsufficientFunds }
    return []any{Withdrawn{Amount: amt}}, nil     // append these to the log
}
```
The write flow: load aggregate by replaying its events → run the command to produce new events → **append** them to the event store (with an optimistic-concurrency check on `version`) → publish them ([34](34-event-driven-outbox.md), often via the outbox).

### Event store, snapshots, projections
- **Event store** — an append-only table/stream keyed by aggregate id, ordered by version. You never `UPDATE`/`DELETE`; you only append. That gives a perfect **audit log** and **temporal queries** ("what was the balance last Tuesday?" = fold events up to that time) for free.
- **Snapshots** — replaying thousands of events per load is slow, so periodically store a snapshot of state at version N; load = snapshot + events after N. A performance optimisation, not a source of truth.
- **Projections** — build read models by subscribing to the event stream and updating denormalised tables. This is where ES meets CQRS: the events feed any number of read models, and you can **rebuild a projection from scratch** by replaying the whole log (great for adding a new view or fixing a bug in one).

### Event versioning / upcasting
Events are immutable and stored **forever**, so schema change is unavoidable over years. You can't "migrate" the past; instead you **upcast** old event versions to the current shape on read (a small function per old version → new). Plan for this from day one — it's the tax of an append-only log. (Same additive-first discipline as [34](34-event-driven-outbox.md)/[27](27-grpc-microservices.md).)

### Benefits vs costs — read this before adopting
**Benefits:** complete audit trail; temporal queries and debugging ("replay to reproduce"); natural fit with event-driven systems; rebuildable read models; no lossy overwrites. **Costs:** real conceptual complexity; **eventual consistency** everywhere; **no ad-hoc queries** on the write side (you can't `SELECT ... WHERE balance > 100` on an event log — you need a projection); event **versioning/upcasting** forever; harder onboarding and tooling.

### When to use what
| Situation | Use |
|---|---|
| Ordinary CRUD, simple reads | **Plain table.** Not CQRS, not ES. |
| Reads and writes diverge in shape/scale | **CQRS-lite** (one DB, separate read views). |
| Read load dwarfs write, or needs a search index/replica | **CQRS with a separate read store** (accept eventual consistency). |
| Audit, temporal queries, complex collaborative domain, "how did we get here?" matters | **Event Sourcing** (usually + CQRS for reads). |
| "It sounds cool" | **Stop.** ES/CQRS on a CRUD app is the textbook over-engineering mistake. |

Start with the simplest thing that works; adopt CQRS-lite when read/write shapes actually diverge; adopt full ES only when the audit/temporal/collaboration requirements are real.

## Exercises
1. Take a CRUD `Order` and split it CQRS-lite: keep the domain write path, add a read-only `OrderQueries` that serves `OrderSummary` DTOs from a query (or SQL view) without loading the aggregate. Note what each side got to optimise.
2. Argue, in a comment, whether a given feature needs a *separate read datastore* or just separate read *views*. Pick the cheaper option that meets the requirement.
3. Implement an event-sourced `Account` with `Apply` (fold), a `Withdraw` command returning events (rejecting overdrafts), and `Load([]event)` replay. Prove `Load` reconstructs the balance from the log.
4. Add optimistic concurrency: include `version` on append and reject an append whose expected version is stale (two concurrent writers → one wins). 
5. Add a snapshot: after every 100 events, store state; change `Load` to start from the latest snapshot + subsequent events, and confirm identical state to a full replay.
6. Write a **projection** that subscribes to `Account` events and maintains a `balances` read table; then rebuild it from an empty table by replaying the log.
7. Upcast: introduce `Deposited{Amount, Currency}` as v2 of `Deposited{Amount}`; write the upcaster and show old v1 events still fold correctly.
8. In your notes, list two features from a real app and decide for each: plain CRUD, CQRS-lite, CQRS+read-store, or ES — and justify.

## Best Practices & Pitfalls
- **CQRS and ES are separable.** You can do CQRS without ES (very common) and ES without user-facing CQRS. Don't adopt both because they're mentioned together.
- **Default to CQRS-lite.** Separate read *views* on one database gives most of the value with none of the eventual-consistency tax. Split the store only when load demands it.
- **The write side has no ad-hoc queries in ES.** Every read shape needs a projection. If a feature needs flexible querying over write data, that's a strong signal ES is the wrong tool.
- **Design for event versioning from day one.** Events live forever; write upcasters, keep changes additive, never mutate stored events.
- **Snapshots are an optimisation, never the source of truth.** The log is authoritative; a corrupt snapshot must be rebuildable by replay.
- **Make projections rebuildable and idempotent.** You will replay them (new view, bug fix). Keyed upserts, not blind inserts.
- **Pitfall — over-engineering.** ES/CQRS on straightforward CRUD adds complexity, latency, and onboarding cost for zero benefit. This is the most common way these patterns go wrong.
- **Pitfall — unbounded aggregates in ES.** An aggregate with millions of events is slow even with snapshots. Keep aggregate lifetimes/streams bounded (close accounts, archive).
- **Pitfall — leaking eventual consistency to users who can't tolerate it.** If a workflow needs read-your-writes, serve that read from the command side or don't split the store.

## Checklist
- [ ] I can explain CQRS (separate read/write models) and implement the lightweight one-DB, separate-views version.
- [ ] I can reason about the eventual consistency a split read store introduces and design UX around it.
- [ ] I can implement an event-sourced aggregate: `Apply` fold, command→events, replay `Load`, optimistic version check.
- [ ] I can add snapshots and know they're an optimisation, not truth.
- [ ] I can build and rebuild an idempotent projection from the event log.
- [ ] I can version/upcast events and know why stored events are never mutated.
- [ ] I can decide, per feature, between CRUD / CQRS-lite / CQRS+read-store / ES — and default to the simplest.

## Resources
- Martin Fowler — CQRS: https://martinfowler.com/bliki/CQRS.html · Event Sourcing: https://martinfowler.com/eaaDev/EventSourcing.html
- Greg Young, "CQRS Documents" & talks (the origin): https://cqrs.files.wordpress.com/2010/11/cqrs_documents.pdf
- Microsoft, "CQRS pattern" & "Event Sourcing pattern": https://learn.microsoft.com/azure/architecture/patterns/cqrs
- EventStoreDB (a purpose-built event store): https://www.eventstore.com/
- Ties to: [31 — DDD (aggregates)](31-ddd-tactical.md), [34 — Event-Driven & Outbox](34-event-driven-outbox.md).
- Next (Track C): [38 — Caching Patterns](38-caching-patterns.md).
