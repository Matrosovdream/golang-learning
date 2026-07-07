# 32 — Hexagonal Architecture (Ports & Adapters)

> Part 9, Track A: [31 DDD](31-ddd-tactical.md) → **32 Hexagonal** → [33 DI & Wiring](33-dependency-injection.md).
> [25 — Clean Architecture](25-architecture.md) taught you *layers* (handler → service → repo) stacked top-to-bottom. Hexagonal is the same **dependency-inversion** idea drawn *symmetrically*: a core surrounded by interchangeable adapters. Learning it makes the "why" behind clean architecture click, and makes your core trivially testable.

## Goals
- Structure a service as a **core** (domain + application) surrounded by **adapters**, connected only through **ports** (interfaces).
- Distinguish **driving** (primary) from **driven** (secondary) adapters, and know which side owns each port.
- Invert dependencies so the **core depends on nothing** but interfaces it defines itself.
- Test the entire core with **in-memory adapters** and zero infrastructure.

## Concepts

### The shape: one core, ports on the edge, adapters plugged in
Picture a hexagon. **Inside** is your application core: the domain ([31](31-ddd-tactical.md)) plus the use cases that orchestrate it. The core's edge is pierced by **ports** — interfaces. **Outside**, each port is filled by an **adapter** — a concrete implementation talking to a real technology (HTTP, Postgres, Kafka, SMTP). The whole point:
> **Dependencies point inward.** The core defines the interfaces; adapters depend on the core; the core depends on no adapter. Swap any adapter without touching the core.
It's called *hexagonal* only because a hexagon has room to draw several ports — the number is meaningless. The other name, **Ports & Adapters**, is the accurate one.

### Two kinds of ports: driving vs driven
The critical distinction is **direction of the call**:

| | **Driving (primary)** | **Driven (secondary)** |
|---|---|---|
| Who calls whom | outside → **calls into** the core | core → **calls out** to outside |
| Port is | the core's **use-case interface** | a dependency the core **needs** |
| Adapter examples | HTTP handler, gRPC server, CLI, queue consumer | Postgres repo, mail sender, cache, message publisher |
| Who **defines** the interface | the **core** (its public API) | the **core** (what it requires) |
| Who **implements** it | the **core** | an **adapter** |

Both ports are **owned by the core** — that's the inversion. A driving adapter *depends on* and *calls* the driving port (implemented by the core); a driven adapter *implements* the driven port the core *declares*.

### Driven port + adapter (the core calls out)
The core declares exactly what it needs as a small interface (consumer-defined, [29](29-patterns-structural.md)), and the adapter satisfies it:
```go
// package core — a DRIVEN port. The core owns this interface.
type OrderRepository interface {
    Get(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, o *Order) error
}

// package adapters/postgres — a DRIVEN adapter implementing the port.
type OrderRepo struct{ db *sql.DB }
func (r OrderRepo) Get(ctx context.Context, id OrderID) (*Order, error) { /* SQL → domain */ }
func (r OrderRepo) Save(ctx context.Context, o *Order) error           { /* domain → SQL */ }
```
`core` imports **no** SQL. `adapters/postgres` imports `core`. The arrow points inward.

### The use case = the driving port (the core's API)
The core exposes its behaviour as an interface that driving adapters call:
```go
// package core — a DRIVING port: what the application can do.
type PlaceOrder interface {
    Place(ctx context.Context, cmd PlaceOrderCmd) (OrderID, error)
}

// the core IMPLEMENTS it, depending only on driven ports:
type OrderService struct {
    orders OrderRepository   // driven port (interface)
    events EventPublisher    // driven port (interface)
}
func (s OrderService) Place(ctx context.Context, cmd PlaceOrderCmd) (OrderID, error) {
    o, err := NewOrder(cmd.ID, cmd.Customer)      // domain
    if err != nil { return "", err }
    for _, it := range cmd.Items {
        if err := o.AddItem(it.Product, it.Qty, it.Price); err != nil { return "", err }
    }
    if err := o.Place(); err != nil { return "", err }
    if err := s.orders.Save(ctx, o); err != nil { return "", err } // out via driven port
    for _, e := range o.Events() { s.events.Publish(ctx, e) }
    return o.ID(), nil
}
```

### Driving adapter (the outside calls in)
An HTTP handler is a driving adapter: it translates a request into a call on the driving port and translates the result back. It depends on the port interface, not on `OrderService` concretely:
```go
// package adapters/http — a DRIVING adapter.
type OrderHandler struct{ uc core.PlaceOrder } // depends on the PORT, not the impl

func (h OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
    var body createOrderDTO
    _ = json.NewDecoder(r.Body).Decode(&body)
    id, err := h.uc.Place(r.Context(), body.toCommand())
    if err != nil {
        writeError(w, err)   // map domain errors → HTTP status
        return
    }
    writeJSON(w, http.StatusCreated, map[string]string{"id": string(id)})
}
```
The same driving port can be driven by a gRPC server, a CLI, or a Kafka consumer — three adapters, one core, no core changes.

### The composition root wires it (preview of [33](33-dependency-injection.md))
Nobody inside the core constructs an adapter. One place — `main` — builds the graph, injecting concrete adapters where interfaces are expected:
```go
func main() {
    db := mustOpenDB()
    repo := postgres.OrderRepo{DB: db}          // driven adapter
    bus  := kafka.Publisher{...}                // driven adapter
    svc  := core.OrderService{orders: repo, events: bus} // core, given its driven ports
    h    := httpadapter.OrderHandler{UC: svc}   // driving adapter, given the driving port
    http.ListenAndServe(":8080", routes(h))
}
```

### Testing: the payoff
Because the core depends only on interfaces, you test it with **in-memory adapters** — no Docker, no network, milliseconds:
```go
type inMemOrders struct{ m map[OrderID]*Order }
func (r *inMemOrders) Get(_ context.Context, id OrderID) (*Order, error) { return r.m[id], nil }
func (r *inMemOrders) Save(_ context.Context, o *Order) error            { r.m[o.ID()] = o; return nil }

func TestPlaceOrder(t *testing.T) {
    svc := core.OrderService{orders: &inMemOrders{m: map[OrderID]*Order{}}, events: nopBus{}}
    id, err := svc.Place(ctx, validCmd)
    // assert on behaviour, not SQL
}
```
That in-memory fake is a legitimate **adapter** — the same port, a different technology (a map). Integration tests then swap in the real Postgres adapter via testcontainers ([40](40-testing-architecture.md)).

### Typical Go layout
```
internal/
  core/                 # domain + application; imports only stdlib
    order.go            # aggregate (lesson 31)
    ports.go            # OrderRepository, EventPublisher (driven) + PlaceOrder (driving)
    service.go          # OrderService implements PlaceOrder
  adapters/
    http/               # driving adapter
    postgres/           # driven adapter
    kafka/              # driven adapter
cmd/api/main.go         # composition root
```

### Hexagonal vs the layered clean architecture of lesson 25
They share the essential move — **dependency inversion via interfaces owned by the inside**. Differences of emphasis:
- **25 (layered)** draws a stack: handler → service → repository, top calls down. Great mental model, but it can tempt you to think "the repo is at the bottom."
- **32 (hexagonal)** draws a center with a symmetric edge: HTTP and Postgres are *both just adapters* on the outside, neither more "core" than the other. It resists the trap of treating the database as the foundation your app is built on — the **domain** is the foundation; the database is a detail you plug in.
Use whichever picture keeps your dependencies pointing inward. They produce nearly the same Go code.

## Exercises
1. Define a driven port `OrderRepository` in a `core` package and implement it twice: an `inMem` adapter (map) and a stub `postgres` adapter. Confirm `core` imports neither.
2. Define a driving port `PlaceOrder` and an `OrderService` that implements it using only driven ports. Prove the service compiles with the in-memory adapter and no database driver imported.
3. Write an HTTP driving adapter that depends on the `PlaceOrder` *interface*. Then write a CLI driving adapter for the same port — two adapters, zero core changes.
4. Unit-test the core end to end with in-memory adapters (no Docker). Then describe (in a comment) what an integration test would swap in.
5. Draw (in a comment or README) the dependency arrows for your packages and verify every arrow points **into** `core`. Find any arrow that points outward and fix it by introducing a port.
6. Take a function in the core that imports `database/sql` or `net/http` and refactor the concern behind a port so the core no longer imports it.

## Best Practices & Pitfalls
- **The core owns every port.** Both the interface it *offers* (driving) and the interfaces it *requires* (driven) are declared inside the core. Adapters depend on the core, never the reverse.
- **Keep ports small and consumer-shaped.** One or two methods per driven port. A fat `Database` interface with 30 methods is not a port; it's the whole implementation leaking through a keyhole.
- **Adapters translate; the core doesn't know they exist.** DTO↔domain and row↔domain mapping happens in the adapter. No `json` or `sql` tags on domain types (the [31](31-ddd-tactical.md) rule).
- **In-memory adapters are first-class.** Your test fakes are real adapters. If the core is hard to fake, a dependency isn't behind a port yet.
- **Pitfall — the database as the center.** If your design starts from tables and everything depends on the ORM model, you've inverted the inversion. Start from the domain; the DB is an outer adapter.
- **Pitfall — leaking adapter types into the core.** Returning a `*sql.Rows`, a `*http.Request`, or a vendor error from a port re-couples the core to the technology. Translate at the boundary.
- **Pitfall — one giant port.** Splitting only "handlers vs everything else" isn't hexagonal. Each distinct outbound concern (persistence, mail, events) is its own driven port so each can be swapped/faked independently.
- **Pitfall — over-architecting a tiny service.** For a 200-line CRUD tool, one package is fine. Reach for full ports & adapters when the domain has real behaviour and multiple I/O concerns.

## Checklist
- [ ] I can explain ports vs adapters and driving vs driven, and say who defines/implements each.
- [ ] I can write a driven port in the core and two adapters (in-memory + real) for it.
- [ ] I can write a driving port (use case) the core implements and drive it from two different adapters (HTTP + CLI).
- [ ] I can wire the whole graph in a composition root with no `New`-of-infra inside the core.
- [ ] I can unit-test the core with in-memory adapters and no infrastructure.
- [ ] I can articulate how hexagonal and lesson 25's layered clean architecture are the same inversion drawn differently.

## Resources
- Alistair Cockburn, the original Hexagonal Architecture article: https://alistair.cockburn.us/hexagonal-architecture/
- Netflix tech blog, "Ready for changes with Hexagonal Architecture": https://netflixtechblog.com/ready-for-changes-with-hexagonal-architecture-b315ec967749
- three-dots (Go), "Clean/Hexagonal Architecture in practice": https://threedots.tech/post/introducing-clean-architecture/
- Compare with: [25 — Project Layout & Clean Architecture](25-architecture.md).
- Next: [33 — Dependency Injection & Application Wiring](33-dependency-injection.md).
