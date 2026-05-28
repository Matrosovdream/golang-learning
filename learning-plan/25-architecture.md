# 25 — Project Layout & Clean Architecture

## Goals
- Lay out a Go service repo the way the community expects.
- Apply layered/hexagonal architecture (handler → service → repository).
- Wire dependencies with constructor-based injection, no framework.
- Place interfaces at boundaries so layers are testable and swappable.

## Concepts
- **Standard project layout** — a widely-followed convention (not enforced by the tool):
  ```
  myservice/
    cmd/
      api/
        main.go          # entrypoint: parse config, build deps, start server
    internal/            # private packages — can't be imported by other modules
      http/              # transport layer: handlers, routing, middleware, DTOs
      service/           # business logic (use cases)
      store/             # persistence: repository implementations (database/sql)
      domain/            # core types (User, Task) + interfaces, no external deps
    pkg/                 # (optional) library code meant to be imported by others
    migrations/          # versioned SQL migrations
    go.mod
  ```
  - **`cmd/<name>/main.go`** — thin entrypoint per binary; only wiring lives here.
  - **`internal/`** — everything app-specific (lesson 16's import rule keeps it private).
  - **`pkg/`** — only for genuinely reusable libraries; many services skip it. Don't dump everything in `pkg/`.
  - Keep `main` small: load config → open DB → construct repository → construct service → construct handlers → start server.
- **Layered (clean/hexagonal) architecture** — separate concerns into layers with a strict dependency direction:
  ```
  HTTP handler  →  Service (business logic)  →  Repository (data access)
   (transport)        (use cases)                 (persistence)
  ```
  - **Handler/transport** — translates HTTP ↔ domain: decode request DTOs, validate, call a service, encode the response. Knows about `http`, not about SQL.
  - **Service** — the business logic / use cases. Knows about domain types and *interfaces* for what it needs (a repository). Knows nothing about HTTP or SQL.
  - **Repository** — data access. Implements a storage interface using `database/sql`. Knows about SQL, not about HTTP.
  - **Domain** — the core entities and the interfaces, depended on by everyone, depending on nothing.
- **The Dependency Rule** — dependencies point **inward**: outer layers (HTTP, SQL) depend on inner layers (domain), never the reverse. The domain has no knowledge of the database or the web. This keeps business logic pure and testable.
- **Dependency injection via constructors** — Go has **no DI framework needed**. You inject dependencies by passing them to `New...` constructors, and you depend on **interfaces**, not concrete types:
  ```go
  // domain: the interface the service needs (defined where it's consumed)
  type TaskRepository interface {
      Create(ctx context.Context, t Task) (Task, error)
      GetByID(ctx context.Context, id int) (Task, error)
  }

  // service depends on the interface, not on *sql.DB
  type TaskService struct{ repo TaskRepository }
  func NewTaskService(repo TaskRepository) *TaskService { return &TaskService{repo: repo} }

  // store implements the interface with database/sql
  type PostgresTaskRepo struct{ db *sql.DB }
  func NewPostgresTaskRepo(db *sql.DB) *PostgresTaskRepo { return &PostgresTaskRepo{db: db} }

  // main wires concrete into the interface slot
  repo := store.NewPostgresTaskRepo(db)
  svc  := service.NewTaskService(repo)
  h    := httpx.NewTaskHandler(svc)
  ```
- **Interfaces at the boundary, defined by the consumer** (lesson 11 applied) — the *service* declares the `TaskRepository` interface it needs; the *store* package satisfies it implicitly. This means: you can swap Postgres for an in-memory fake in tests with zero changes to the service, and packages don't import each other in a cycle.
- **No global state.** Don't use package-level `var db *sql.DB` or global loggers. Pass dependencies explicitly through constructors. Globals make testing and reasoning hard and create hidden coupling.
- **Testing falls out of the design** — because the service depends on an interface, unit-test it with an in-memory fake repository (no database). Test repositories against a real DB; test handlers with `httptest` (lesson 18). Clean layering = easy tests.
- **Where DTOs live** — request/response structs belong in the transport layer (lesson 21), separate from domain entities, so your API contract can evolve independently of your internal model.
- **Don't over-layer small projects.** For a tiny service, handler + store may be enough. Add the service layer when business logic grows beyond trivial pass-through. Architecture serves the code, not vice versa.

## Exercises
1. Restructure your Part 6 Task API into `cmd/api/main.go` + `internal/{http,service,store,domain}`. Move config loading and wiring into `main`.
2. Define a `TaskRepository` interface in the domain/service layer with `Create`/`GetByID`/`List`/`Delete`.
3. Implement two repositories satisfying it: an in-memory map-backed one and a `database/sql` Postgres one. Swap between them in `main` by changing one line.
4. Make `TaskService` depend only on the interface; ensure it imports neither `net/http` nor `database/sql`.
5. Unit-test `TaskService` with the in-memory fake repo (no DB). Then test the Postgres repo against a real/SQLite DB, and the handlers with `httptest`.
6. Verify the dependency direction: confirm `domain` imports nothing app-specific and that there are no import cycles (`go build ./...`).
7. Discuss with Claude where to *stop* layering for this project's size — when a service layer is overkill vs warranted.

## Best Practices & Pitfalls
- **Keep `main` thin and dumb.** It only wires concrete implementations into interfaces and starts the server. All logic lives in `internal/`.
- **Depend on interfaces you define at the consumer; inject concretes via constructors.** This is the whole game — it gives you testability, swappability, and no import cycles, with zero framework.
- **Dependencies point inward.** The domain must not import the database or HTTP packages. If you find `database/sql` imported in your service layer, the layering is broken.
- **Use `internal/` to stop accidental coupling** to your implementation details.
- **Avoid global state and `init()` wiring** — explicit constructors beat hidden globals every time.
- **Pitfall — interface in the wrong place.** Defining repository interfaces in the `store` package (the producer) instead of the `service` package (the consumer) recreates tight coupling. Define them where they're used.
- **Pitfall — over-engineering.** Don't build 5 layers and 12 interfaces for a CRUD app with three endpoints. Start simple (handler + store); introduce the service layer and more interfaces when complexity justifies them.
- **Pitfall — leaking DB/HTTP types across layers.** Don't pass `*sql.Rows` or `*http.Request` into the service layer; translate at the boundary into domain types/DTOs.
- **Pitfall — circular imports** are a design smell; break them by moving shared types to `domain` or inverting a dependency with an interface.

## Checklist
- [ ] I can lay out a service with `cmd/`, `internal/{http,service,store,domain}`.
- [ ] I keep `main` thin: config → deps → server.
- [ ] I apply handler → service → repository with dependencies pointing inward.
- [ ] I inject dependencies via constructors and depend on consumer-defined interfaces.
- [ ] I can swap a real repo for an in-memory fake to unit-test the service.
- [ ] I avoid global state and import cycles.
- [ ] I know when *not* to add a layer.

## Resources
- Standard Go Project Layout (popular convention): https://github.com/golang-standards/project-layout
- Organizing a Go module: https://go.dev/doc/modules/layout
- Article — How I write HTTP services in Go (Mat Ryer): https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
- Talk/idea — Clean/Hexagonal architecture in Go: https://go.dev/talks/ (see "Go best practices") · https://threedots.tech/post/introducing-clean-architecture/
