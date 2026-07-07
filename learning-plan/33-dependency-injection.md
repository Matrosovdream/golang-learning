# 33 — Dependency Injection & Application Wiring

> Part 9, Track A: [31 DDD](31-ddd-tactical.md) → [32 Hexagonal](32-hexagonal-ports-adapters.md) → **33 DI & Wiring**.
> Ports & adapters only pay off if something *assembles* them. This lesson is about the **composition root** — the one place your object graph is built — and the three ways Go teams do it: **by hand**, with **google/wire** (compile-time codegen), and with **uber/fx** (runtime container).

## Goals
- Understand that DI in Go is just **passing dependencies as arguments** — no framework required.
- Build a **composition root** in `main` and keep construction out of everything else.
- Know **manual DI**, **google/wire**, and **uber/fx**, and choose deliberately between them.
- Eliminate global singletons and `init()`-time wiring so everything is swappable in tests.

## Concepts

### DI in Go is boring on purpose
"Dependency injection" sounds like a framework; in Go it's a habit: **a component receives its dependencies instead of creating them.** That's it. Constructor injection is the whole pattern:
```go
// ❌ hidden dependency: the service reaches out and builds its own DB.
func NewOrderService() *OrderService {
    db, _ := sql.Open("pgx", os.Getenv("DB_URL")) // untestable, global-ish
    return &OrderService{repo: postgres.OrderRepo{DB: db}}
}

// ✅ injected: dependencies come IN; the signature declares exactly what it needs.
func NewOrderService(repo OrderRepository, events EventPublisher) *OrderService {
    return &OrderService{repo: repo, events: events}
}
```
The injected version depends on **interfaces** ([32](32-hexagonal-ports-adapters.md)), so a test passes fakes and production passes real adapters. No reflection, no container, no magic — Go's type system is the DI framework.

### The composition root
There should be exactly **one** place that knows every concrete type and wires them together — the **composition root**, almost always `main` (or a `wire.go`). Everywhere else takes interfaces. This is the *only* place `sql.Open`, `kafka.NewProducer`, etc. appear:
```go
func main() {
    cfg := config.Load()

    db := mustOpen(cfg.DBURL)                 // leaf dependencies first
    defer db.Close()
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    repo := postgres.NewOrderRepo(db)         // driven adapters
    bus  := kafka.NewPublisher(cfg.Brokers)

    svc  := core.NewOrderService(repo, bus)   // core, given its ports
    h    := httpadapter.NewOrderHandler(svc)  // driving adapter

    srv := &http.Server{Addr: cfg.Addr, Handler: routes(h, logger)}
    // ... graceful shutdown (lesson 21)
    srv.ListenAndServe()
}
```
Build order is **leaves → root**: things with no dependencies first, then the things that depend on them. If you find yourself needing A before B and B before A, you have a cycle to break (usually with an interface or an event).

### Manual DI — the default, and often the best
The `main` above *is* manual DI. Its virtues are exactly what you want:
- **Compile-time checked** — a missing or wrong dependency is a build error, right there.
- **No reflection, no runtime surprises** — you can read the whole graph top to bottom.
- **Zero dependencies** — it's just function calls.

Its only cost is that for a very large graph (dozens of providers), the wiring block gets long and you hand-order it. Extract sub-graphs into small `NewX(...)` "provider" functions and manual DI scales surprisingly far. **Reach for a tool only when the hand-written root becomes a genuine maintenance burden.**

### google/wire — compile-time DI by code generation
[`google/wire`](https://github.com/google/wire) generates the wiring code you'd otherwise write by hand. You declare **providers** (constructors) and an **injector** signature; `wire` figures out the order and emits a plain Go function:
```go
//go:build wireinject

func InitOrderHandler(cfg Config) (*OrderHandler, error) {
    wire.Build(
        mustOpen,               // Config → *sql.DB
        postgres.NewOrderRepo,  // *sql.DB → OrderRepository
        kafka.NewPublisher,     // Config → EventPublisher
        core.NewOrderService,   // (OrderRepository, EventPublisher) → *OrderService
        NewOrderHandler,        // *OrderService → *OrderHandler
    )
    return nil, nil             // body is a placeholder; wire replaces it
}
```
Run `wire ./...` and it generates `wire_gen.go` containing the real, ordered constructor calls — **no reflection at runtime**, it's just codegen. Group related providers into a `wire.NewSet(...)`. Pros: compile-time safety like manual DI, but the ordering is automated and refactors are cheaper. Cons: a codegen step in your build, and errors are at generate-time rather than where you read the code.

### uber/fx — a runtime DI container with lifecycle
[`uber/fx`](https://github.com/uber-go/fx) builds the graph at **runtime** via reflection, and — its real value — manages **application lifecycle** (ordered startup/shutdown hooks):
```go
func main() {
    fx.New(
        fx.Provide(
            config.Load,
            mustOpen,               // constructors; fx resolves the graph by type
            postgres.NewOrderRepo,
            kafka.NewPublisher,
            core.NewOrderService,
            NewOrderHandler,
            newHTTPServer,
        ),
        fx.Invoke(func(*http.Server) {}), // force-construct the server
    ).Run()
}

func newHTTPServer(lc fx.Lifecycle, h *OrderHandler) *http.Server {
    srv := &http.Server{Addr: ":8080", Handler: routes(h)}
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error { go srv.ListenAndServe(); return nil },
        OnStop:  func(ctx context.Context) error { return srv.Shutdown(ctx) },
    })
    return srv
}
```
Pros: automatic wiring for large apps, first-class **OnStart/OnStop** ordering (DB connects before server starts; server drains before DB closes), and modules (`fx.Module`) for plugin-style composition. Cons: **runtime** resolution — a missing provider is a startup panic, not a compile error — plus reflection and a learning curve. It shines in big services with many long-lived resources and a real lifecycle.

### Choosing
| Use | When |
|---|---|
| **Manual DI** | Default. Small-to-medium services; you value reading the graph and compile-time safety. Most Go services never outgrow it. |
| **google/wire** | Large graph where hand-ordering is tedious, but you still want **compile-time** guarantees and no runtime reflection. |
| **uber/fx** | Large app with many managed resources and complex **lifecycle**/module composition; you accept runtime wiring for those features. |

Start manual. Migrate to a tool only when the wiring itself is the pain — not preemptively.

### Anti-patterns DI exists to kill
- **The service locator** — a global `registry.Get[Thing]()` that components call to fetch dependencies. It hides what a component needs (the signature lies) and reintroduces global state. DI's whole point is to make dependencies explicit in the signature.
- **Package-global singletons** — `var db *sql.DB` set in `init()`. Two tests now share state; you can't run them in parallel or with different fakes. Pass the dependency instead ([28](28-patterns-creational.md) covered `sync.Once` vs DI).
- **Constructors that read the environment** — `os.Getenv` inside `NewX` makes the type untestable and its config invisible. Read config once at the root and pass values down.

## Exercises
1. Refactor a `NewOrderService()` that calls `sql.Open` internally into `NewOrderService(repo, events)` taking interfaces; write a test that injects fakes with no database.
2. Build a manual composition root in `main` that wires config → db → repo → service → handler → server in leaf-to-root order, with `defer` cleanup and graceful shutdown.
3. Extract two sub-graphs into `newStorage(cfg)` and `newMessaging(cfg)` provider functions and show `main` shrink. Note how far manual DI now scales.
4. (If you install it) Convert that root to **google/wire**: declare providers, write an `InitApp` injector with `wire.Build`, generate `wire_gen.go`, and diff it against your hand-written version.
5. (If you install it) Convert the same app to **uber/fx** with `OnStart`/`OnStop` hooks so the DB connects before the server starts and drains before it closes. Trigger a missing-provider panic and note it's a *runtime* error.
6. Write one paragraph in your notes: for *your* current project size, which of the three you'd choose and why.

## Best Practices & Pitfalls
- **One composition root.** Exactly one place constructs concrete types; everything else receives interfaces. `sql.Open`/`kafka.New…` appear once, in `main`/`wire.go`.
- **Inject interfaces, return structs** ([28](28-patterns-creational.md)/[32](32-hexagonal-ports-adapters.md)). Constructors take the ports; they return the concrete type they build.
- **Build leaves first.** Order the root from zero-dependency values up to the top. A cycle means a missing interface or an event you should introduce.
- **Read config once, at the root.** Pass values down; never `os.Getenv` deep in the graph.
- **Prefer manual DI until it hurts.** It's compile-checked and dependency-free. Adopt wire/fx to solve a *specific* pain (tedious ordering; lifecycle), not as a default.
- **Pitfall — service locator / globals.** If a component fetches its dependencies from a global registry or package var, its signature no longer tells the truth and tests interfere. Make dependencies parameters.
- **Pitfall — fx's runtime failure mode.** Missing/duplicate providers surface as startup panics, not compile errors. Keep an `fx.Invoke` that constructs the top of the graph so failures fail fast at boot, and lean on integration tests.
- **Pitfall — over-wiring tiny apps.** A 300-line service doesn't need wire or fx. The tool should remove pain that actually exists.

## Checklist
- [ ] I can explain why constructor injection (passing deps in) beats a constructor that builds its own dependencies.
- [ ] I can write a manual composition root in leaf-to-root order with cleanup and graceful shutdown.
- [ ] I can extract provider functions to keep the root readable as it grows.
- [ ] I can describe how google/wire generates the wiring at compile time and what a `wire.Build`/`wire.NewSet` does.
- [ ] I can describe uber/fx's runtime graph + `OnStart`/`OnStop` lifecycle and its trade-off vs wire.
- [ ] I can pick manual/wire/fx for a given service size and justify it.
- [ ] I can spot and remove a service locator or a package-global singleton.

## Resources
- google/wire — guide & tutorial: https://github.com/google/wire/blob/main/docs/guide.md · https://github.com/google/wire/tree/main/_tutorial
- uber/fx — docs & getting started: https://uber-go.github.io/fx/
- Mark Bates, "Dependency Injection in Go" (manual DI first): https://blog.gobuffalo.io/
- Ties back to: [28 — Creational (sync.Once vs DI)](28-patterns-creational.md), [32 — Hexagonal](32-hexagonal-ports-adapters.md).
- Next (Track B): [34 — Event-Driven Architecture & the Outbox](34-event-driven-outbox.md).
