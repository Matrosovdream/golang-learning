# Go Learning Plan

A step-by-step path to learn Go from zero to building real backend services. Each step is a self-contained lesson with goals, concepts, exercises, best-practice notes, and resources. Claude is your tutor — ask questions as you go.

## How to use this plan

1. Work through steps in order (01 → 26); Parts 8–9 (27 → 41) are optional extensions you can take in any order.
2. For each step:
   - Read the lesson file (`NN-title.md`).
   - Write the exercises in `/go-project/NN-title/` (you'll scaffold the module as you go — see "Stack" below).
   - Run your code (`go run .`), format it (`gofmt`/`go fmt`), and check it (`go vet`).
   - Ask Claude to explain anything unclear or review your code.
3. When you finish a step, update [PROGRESS.md](PROGRESS.md) — that's where we track where you are.

Every lesson has a **Best Practices & Pitfalls** section. Read it even when you're in a hurry — idiomatic Go is learned by accumulating these small habits, not by reading one big style guide at the end.

## Stack we're targeting

- **Go 1.22+** (standard library first; the new `net/http` `ServeMux` routing assumes 1.22+)
- **`net/http`** for the web server — no framework until you understand the stdlib
- **`database/sql`** (with a Postgres driver such as `pgx`) for persistence
- **`log/slog`** for structured logging
- **`testing`** (table-driven) + `httptest` for tests
- **`golangci-lint`** for linting (introduced in Part 7)

We stay in the standard library on purpose: Go's stdlib is strong enough to build production services, and learning it first makes every framework easier later.

> **Note:** No Go module is scaffolded yet. When you reach Part 2 and want to run code, create one with `mkdir go-project && cd go-project && go mod init example.com/go-project`. Put each lesson's code in its own subfolder/package.

## Steps

### Part 1 — Foundations
- [01 — Introduction to Go](01-introduction.md) — what Go is, why use it, the philosophy & ecosystem
- [02 — Environment Setup](02-environment-setup.md) — install, the `go` toolchain, modules, editor
- [03 — First Program & Basic Syntax](03-first-program.md) — `package main`, `func main`, vars, constants, `gofmt`

### Part 2 — Language Core
- [04 — Variables, Types & Constants](04-types-constants.md) — numeric/string/bool, zero values, conversions, `iota`
- [05 — Control Flow](05-control-flow.md) — `if`, the single `for`, `switch`, `defer`
- [06 — Functions](06-functions.md) — multiple returns, variadic, closures, `defer`/`panic`/`recover`
- [07 — Arrays, Slices & Maps](07-slices-maps.md) — slice internals, `append`, `copy`, maps, sets
- [08 — Strings, Runes, Bytes & Formatting](08-strings.md) — UTF-8, `strings`, `strconv`, `fmt`

### Part 3 — Types, Methods & Interfaces
- [09 — Structs](09-structs.md) — fields, composition/embedding, tags, comparability
- [10 — Pointers & Methods](10-pointers-methods.md) — pointers, value vs pointer receivers, method sets
- [11 — Interfaces](11-interfaces.md) — implicit satisfaction, `any`, type switches, accept-interfaces
- [12 — Errors & Error Handling](12-errors.md) — `error`, wrapping with `%w`, `errors.Is`/`As`, custom errors

### Part 4 — Concurrency
- [13 — Goroutines](13-goroutines.md) — the `go` keyword, the scheduler, `WaitGroup`, leaks
- [14 — Channels](14-channels.md) — buffered/unbuffered, directions, `close`, `select`
- [15 — Sync, Context & Patterns](15-sync-context.md) — mutexes, `context.Context`, worker pools, `-race`

### Part 5 — Packages, Generics & Quality
- [16 — Packages & Modules](16-packages-modules.md) — package design, `internal/`, `go.mod`, versioning
- [17 — Generics](17-generics.md) — type parameters, constraints, `comparable`, when not to use them
- [18 — Testing & Benchmarking](18-testing.md) — table-driven tests, `httptest`, benchmarks, fuzzing, coverage
- [19 — Standard Library Tour for Backend](19-stdlib-tour.md) — `io`, `os`, `time`, `encoding/json`, `slog`, `flag`

### Part 6 — Building a Backend (REST API)
- [20 — HTTP Server Fundamentals](20-http-server.md) — `net/http`, 1.22+ routing, handlers, request lifecycle
- [21 — Building a JSON REST API](21-rest-api.md) — JSON encode/decode, validation, middleware, graceful shutdown
- [22 — Persistence with database/sql](22-database.md) — drivers, the pool, scanning, transactions, migrations
- [23 — Config, Logging & Observability](23-config-logging.md) — env config, `slog`, request logging, health checks

### Part 7 — Architecture & Best Practices
- [24 — Idiomatic Go & Effective Go](24-idiomatic-go.md) — naming, the zero value, error style, `vet`/lint
- [25 — Project Layout & Clean Architecture](25-architecture.md) — `cmd`/`internal`, layering, dependency injection
- [26 — Capstone Project: REST API Service](26-capstone.md) — a small backend that uses everything above

### Part 8 — Extensions (beyond the core plan)
- [27 — gRPC & Microservices](27-grpc-microservices.md) — protobuf contracts, gRPC servers/clients, streaming, interceptors, **request-id logging & Prometheus/Grafana metrics between services**. Examples: [examples/27-grpc-microservices](examples/27-grpc-microservices/); projects: `grpc-echo-beginner` → `grpc-orders-intermediate` → `grpc-observability-hard`.

**Design Patterns (the Go way)** — GoF patterns blended with idiomatic Go (functional options, embedding, first-class functions). Concurrency patterns stay in [15](15-sync-context.md); style in [24](24-idiomatic-go.md).
- [28 — Design Patterns I: Creational](28-patterns-creational.md) — useful zero value, constructors, **functional options**, builders, factory/registry, singleton (`sync.Once`) vs DI, object pool (`sync.Pool`), prototype/clone. · [examples](examples/28-patterns-creational/) (17)
- [29 — Design Patterns II: Structural](29-patterns-structural.md) — embedding vs inheritance, adapter (`HandlerFunc`), **decorator/middleware**, facade, proxy (caching/access), composite (trees), bridge, flyweight. · [examples](examples/29-patterns-structural/) (17)
- [30 — Design Patterns III: Behavioral](30-patterns-behavioral.md) — strategy & command as function values, the Template-Method **embedding trap**, observer/pub-sub, state machines, **range-over-func iterators** (`iter.Seq`, 1.23+), chain of responsibility, visitor via type switch. · [examples](examples/30-patterns-behavioral/) (16)

### Part 9 — Architecture & System Design (beyond the core plan)
Builds on [25](25-architecture.md) (clean-arch), [27](27-grpc-microservices.md) (microservices), and the design patterns above. Take in any order; docs first, graded examples added later.

*Track A — structuring one service (deepens [25](25-architecture.md)):*
- [31 — Domain-Driven Design (tactical)](31-ddd-tactical.md) — entities, value objects, aggregates & invariants, domain events, repositories, keeping the domain framework-free, anti-corruption layer. · [examples](examples/31-ddd-tactical/) (15)
- [32 — Hexagonal / Ports & Adapters](32-hexagonal-ports-adapters.md) — driving vs driven ports, dependency inversion, in-memory adapters for tests; how it relates to lesson 25's layered take. · [examples](examples/32-hexagonal-ports-adapters/) (15)
- [33 — Dependency Injection & Wiring](33-dependency-injection.md) — composition root, manual DI vs `google/wire` vs `uber/fx`, killing globals/service-locators. · [examples](examples/33-dependency-injection/) (15)

*Track B — coordinating services (deepens [27](27-grpc-microservices.md)):*
- [34 — Event-Driven Architecture & the Outbox](34-event-driven-outbox.md) — async events, at-least-once + idempotent consumers, the **transactional outbox** for the dual-write problem, event versioning. · [examples](examples/34-event-driven-outbox/) (15)
- [35 — Sagas & Distributed Transactions](35-sagas-distributed-transactions.md) — orchestration vs choreography, compensating actions, idempotency keys — the "no distributed transaction" trap solved. · [examples](examples/35-sagas-distributed-transactions/) (15)
- [36 — Resilience Patterns](36-resilience-patterns.md) — timeout, retry-with-jitter, **circuit breaker**, bulkhead, rate limit / load shedding, graceful degradation. · [examples](examples/36-resilience-patterns/) (15)
- [37 — CQRS & Event Sourcing](37-cqrs-event-sourcing.md) *(advanced)* — split read/write models, event log as source of truth, projections, snapshots — and when **not** to. · [examples](examples/37-cqrs-event-sourcing/) (15)

*Track C — production cross-cutting:*
- [38 — Caching Patterns](38-caching-patterns.md) — cache-aside / write-through / write-behind, TTL & invalidation, stampede protection with `singleflight`, local vs distributed. · [examples](examples/38-caching-patterns/) (15)
- [39 — Observability: Distributed Tracing](39-observability-tracing.md) — the three pillars, OpenTelemetry spans, context propagation across hops, correlating logs ↔ metrics ↔ traces, sampling. · [examples](examples/39-observability-tracing/) (15)
- [40 — Testing Architecture](40-testing-architecture.md) — the service test pyramid, fakes vs mocks, **testcontainers** for real infra, determinism, contract testing. · [examples](examples/40-testing-architecture/) (15)
- [41 — API Design & Evolution](41-api-design-evolution.md) — backward-compatible change & versioning, cursor pagination, idempotency keys, RFC 9457 `problem+json`, OpenAPI. · [examples](examples/41-api-design-evolution/) (15)

## Progress

See [PROGRESS.md](PROGRESS.md) for the current step and notes from past lessons.
