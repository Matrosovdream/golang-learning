# 31 — Domain-Driven Design (Tactical Patterns in Go)

> Part 9 — **Architecture & System Design**, Track A (structuring one service): **31 DDD** → [32 Hexagonal](32-hexagonal-ports-adapters.md) → [33 DI & Wiring](33-dependency-injection.md).
> [25 — Clean Architecture](25-architecture.md) gave you the *layers* (cmd/internal, DI, layering). This lesson fills the **domain layer** with real modelling tools so it stops being an anemic bag of structs. We focus on the **tactical** patterns (the code); the strategic side (bounded contexts) gets a short intro.

## Goals
- Model a domain with **entities**, **value objects**, and **aggregates**, and enforce invariants so an invalid object can't exist.
- Raise and handle **domain events**; put cross-entity logic in **domain services**.
- Define **repositories** as domain concepts (interfaces in the domain, implementations in infra).
- Keep the domain package **import-free** of frameworks — the single most important DDD habit in Go.

## Concepts

### Strategic in one paragraph: ubiquitous language & bounded contexts
DDD's strategic half says: agree on a **ubiquitous language** (the domain's real words — `Order`, `Charge`, `Refund` — used identically in conversation and code), and split a big system into **bounded contexts**, each with its own model of those words (an `Order` in *Sales* ≠ an `Order` in *Shipping*). In Go a bounded context is usually **one service or one top-level package**. The rest of this lesson is the *tactical* toolkit you use *inside* one context.

### Entity — identity, not value
An **entity** has a stable **identity** that outlives its attributes; two entities are equal if their IDs match, even if every other field differs. Model it as a struct with **unexported fields** and methods that guard invariants — never let callers set fields directly:
```go
type Order struct {
    id      OrderID          // identity → equality is by this, not by fields
    status  Status
    items   []LineItem
    version int              // for optimistic locking (see lesson 22/41)
}

func (o *Order) ID() OrderID { return o.id }

// Behaviour lives ON the entity and protects the invariant:
func (o *Order) AddItem(p ProductID, qty int, price Money) error {
    if o.status != StatusDraft {
        return ErrOrderNotDraft            // can't modify a placed order
    }
    if qty <= 0 {
        return fmt.Errorf("qty must be positive, got %d", qty)
    }
    o.items = append(o.items, LineItem{Product: p, Qty: qty, Price: price})
    return nil
}
```
This is the opposite of the **anemic domain model** (structs with only getters/setters and all logic in a "service"). Put behaviour where the data is.

### Value object — equality by value, immutable
A **value object** has no identity; it's defined entirely by its attributes and should be **immutable** — construct it validated, and "change" it by returning a new one. `Money`, `Email`, `Address`, `DateRange` are value objects:
```go
type Money struct {
    amount   int64  // minor units (cents) — never float for money
    currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
    if currency == "" {
        return Money{}, errors.New("money: currency required")
    }
    return Money{amount: amount, currency: currency}, nil
}

// "Mutation" returns a NEW value — the original is untouched:
func (m Money) Add(other Money) (Money, error) {
    if m.currency != other.currency {
        return Money{}, fmt.Errorf("currency mismatch: %s vs %s", m.currency, other.currency)
    }
    return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}
```
Because a `Money` value is comparable with `==` and immutable, it's safe to pass around and share freely (this is the *flyweight* idea from [29](29-patterns-structural.md), for free).

### Aggregate & aggregate root — the consistency boundary
An **aggregate** is a cluster of entities/value objects treated as one unit for data changes. One entity is the **root**; the outside world may only hold a reference to the root and must go **through** it to touch anything inside. The aggregate's job is to keep a **business invariant true at all times** — the root's methods are where you enforce it:
```go
// Order is the aggregate root; LineItem lives INSIDE and is reached only via Order.
func (o *Order) Place() error {
    if len(o.items) == 0 {
        return ErrEmptyOrder           // invariant: no empty orders may be placed
    }
    o.status = StatusPlaced
    o.raise(OrderPlaced{OrderID: o.id, Total: o.Total()})
    return nil
}
```
Two rules that keep aggregates healthy:
- **Reference other aggregates by ID, not by pointer.** An `Order` holds a `CustomerID`, not a `*Customer`. This keeps aggregates small, load boundaries clear, and lets each be persisted/locked independently.
- **One transaction = one aggregate.** Persist one aggregate per DB transaction; coordinate changes *across* aggregates with domain events / sagas ([34](34-event-driven-outbox.md), [35](35-sagas-distributed-transactions.md)), not one giant transaction.

### Domain events — record that something happened
A **domain event** is an immutable fact named in the past tense (`OrderPlaced`, `PaymentCaptured`). The aggregate *records* events as it changes; the application layer *dispatches* them after the aggregate is safely persisted:
```go
type OrderPlaced struct {
    OrderID OrderID
    Total   Money
}

func (o *Order) raise(e any)      { o.events = append(o.events, e) }
func (o *Order) Events() []any    { return o.events }
func (o *Order) ClearEvents()     { o.events = nil }

// application layer, after repo.Save(order) succeeds:
for _, e := range order.Events() {
    bus.Publish(ctx, e)   // in-process handlers, or the outbox (lesson 34)
}
order.ClearEvents()
```
Events are how one aggregate tells the rest of the system it changed without directly calling it — the decoupling that later enables outbox/sagas/CQRS.

### Domain service — logic that belongs to no single entity
When a rule spans multiple aggregates or needs external knowledge (a currency conversion, a "can this transfer happen" check across two accounts), it doesn't belong on either entity — put it in a **domain service** (a stateless struct/func in the domain, still framework-free):
```go
type TransferService struct{ rates ExchangeRates } // ExchangeRates is a domain interface

func (s TransferService) Transfer(from, to *Account, amt Money) error {
    if err := from.Withdraw(amt); err != nil { return err }
    converted := s.rates.Convert(amt, to.Currency())
    return to.Deposit(converted)
}
```

### Repository — a collection of aggregates, defined in the domain
A **repository** is an interface that looks like an in-memory collection of one aggregate root. **Define it in the domain**; implement it in infrastructure. One repository per aggregate root — not per table:
```go
// in package domain — no sql/gorm imports here:
type OrderRepository interface {
    Get(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, o *Order) error   // insert or update the whole aggregate
}
```
The Postgres/GORM implementation lives in `internal/adapters/postgres` and maps between the DB row and the domain aggregate (often a separate `orderRow` struct — never let ORM tags into the domain). This is the port you'll formalise in [32](32-hexagonal-ports-adapters.md).

### Factory — build a valid aggregate
When constructing an aggregate is non-trivial (multiple invariants, initial events), use a factory function so an invalid aggregate can never be born — this is the `NewT`/builder idea from [28](28-patterns-creational.md) applied to the domain:
```go
func NewOrder(id OrderID, customer CustomerID) (*Order, error) {
    if customer == "" {
        return nil, errors.New("order: customer required")
    }
    return &Order{id: id, customer: customer, status: StatusDraft}, nil
}
```

### The cardinal rule: keep the domain import-free
The domain package must not import `net/http`, `database/sql`, `gorm.io/...`, your logger, or any transport/persistence concern. It imports only the standard library (and maybe a uuid/decimal helper). If your `Order` has a `gorm:"column:..."` tag or a `json:"..."` tag driving your API, persistence and transport have leaked into the domain — split them out. When integrating with an external system whose model clashes with yours, translate at the edge with an **anti-corruption layer** (a small adapter that maps their model to yours) so their concepts never pollute your domain.

## Exercises
1. Model `Money` as an immutable value object (minor units + currency) with `NewMoney`, `Add`, and `Equals`; prove two equal `Money` values are `==` and that `Add` doesn't mutate the originals.
2. Build an `Order` aggregate root with unexported fields, `AddItem` (rejects qty ≤ 0 and non-draft orders), `Place` (rejects empty orders), and a `Total() Money`. Show you cannot construct or mutate an invalid `Order` from outside the package.
3. Make `Order.Place()` record an `OrderPlaced` domain event; expose `Events()`/`ClearEvents()`; in a fake application layer, "publish" them after a fake `Save`.
4. Reference the customer by `CustomerID` (a value), not `*Customer`. Write one sentence on why by-ID references keep aggregates independent.
5. Define an `OrderRepository` interface in the domain package and a trivial in-memory implementation in a separate package. Confirm the domain package imports nothing from infra.
6. Refactor an anemic `OrderService` (with `SetStatus`, `AddItemToOrder`) into behaviour on the `Order` entity, and note what invariant each moved method now protects.

## Best Practices & Pitfalls
- **Push behaviour onto entities/aggregates.** If your "service" has all the logic and your structs are just fields + getters, you've built an anemic model — the thing DDD exists to prevent.
- **Enforce invariants in constructors and methods, with unexported fields.** An aggregate should make illegal states unrepresentable, not merely discouraged.
- **Value objects are immutable and validated at creation.** Return new copies from operations; use minor-unit integers for money, never `float64`.
- **Reference other aggregates by ID; one aggregate per transaction.** Cross-aggregate consistency is *eventual*, via events/sagas — not one big transaction.
- **Keep the domain framework-free.** No `json`/`gorm`/`http` in domain types. Map to DTOs and DB rows at the boundary; use an anti-corruption layer for foreign models.
- **Pitfall — huge aggregates.** Pulling everything reachable into one aggregate makes it slow to load and a lock-contention hotspot. Draw the boundary at the true invariant.
- **Pitfall — leaking the ORM model as the domain model.** GORM/sqlx tags on your `Order` couple the domain to the schema; a migration then forces a domain change. Keep a separate persistence struct.
- **Pitfall — exposing setters.** A public `SetStatus` lets any caller skip the state machine. Expose intent (`Place`, `Cancel`), not raw mutation.

## Checklist
- [ ] I can model an entity (identity-based) vs a value object (immutable, value-based) and know when each applies.
- [ ] I can design an aggregate with a root that enforces an invariant, and explain the "reference by ID / one aggregate per transaction" rules.
- [ ] I can record domain events on an aggregate and dispatch them after persistence.
- [ ] I can place cross-aggregate logic in a domain service without breaking the framework-free rule.
- [ ] I can define a repository interface in the domain and implement it in infra with a mapping layer.
- [ ] I can spot and refactor an anemic domain model.

## Resources
- Eric Evans, *Domain-Driven Design*; Vaughn Vernon, *Implementing DDD* (the "red book") & *DDD Distilled*.
- Aggregate design (Vernon's essays): https://www.dddcommunity.org/library/vernon_2011/
- "Domain-Driven Design in Go" (three-dots / Watermill team): https://threedots.tech/post/ddd-lite-in-go-introduction/
- Effective Go on package design (why domain packages stay small & pure): https://go.dev/doc/effective_go#package-names
- Next: [32 — Hexagonal / Ports & Adapters](32-hexagonal-ports-adapters.md).
