# Go Learning Plan

A step-by-step path to learn Go from zero to building real backend services. Each step is a self-contained lesson with goals, concepts, exercises, best-practice notes, and resources. Claude is your tutor — ask questions as you go.

## How to use this plan

1. Work through steps in order (01 → 26).
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

## Progress

See [PROGRESS.md](PROGRESS.md) for the current step and notes from past lessons.
