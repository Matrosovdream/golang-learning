# 26 — Capstone Project: REST API Service

## Goals
- Build a complete, production-shaped backend service from scratch.
- Combine everything: types, errors, concurrency, HTTP, DB, config, logging, tests, architecture.
- Practice the full workflow: design → implement → test → run → harden.

## The project

Build a **Task Manager REST API** (or pick your own CRUD domain — bookmarks, notes, a small store). It should be small enough to finish but exercise every part of the course.

### Functional requirements
- **Resources:** `Task { id, title, description, status, created_at, updated_at }` and `User { id, email, ... }`.
- **Endpoints (REST, JSON):**
  - `POST   /tasks` — create (validate, 201 + created task)
  - `GET    /tasks` — list (with pagination `?limit=&offset=` and optional `?status=` filter)
  - `GET    /tasks/{id}` — get one (404 if missing)
  - `PUT    /tasks/{id}` — update (404 if missing, 400 on bad input)
  - `DELETE /tasks/{id}` — delete (204)
  - `GET    /healthz` & `GET /readyz` — liveness/readiness
- **Auth:** a simple `Authorization: Bearer <token>` middleware (static token from config, or a real login that issues a token if you want a stretch).
- **Persistence:** Postgres (or SQLite) via `database/sql`, with migrations.
- **Bonus concurrency:** a background worker (goroutine + channel + `context`) that does something real — e.g., processes a queue of "send notification" jobs, or periodically purges old completed tasks on a ticker. Shut it down cleanly on server shutdown.

### Non-functional requirements (this is where the course pays off)
- **Architecture:** layered — `cmd/api/main.go` + `internal/{http,service,store,domain}` (lesson 25). Dependencies injected via constructors; service depends on a repository **interface**.
- **Config:** loaded from env into a typed `Config`, fail-fast on missing required values (lesson 23).
- **Logging:** structured `slog` with per-request IDs and request-logging middleware (lesson 23).
- **Errors:** wrapped with `%w`, sentinel `ErrNotFound`, clean JSON error responses; no internal details leaked to clients (lesson 12, 21).
- **Middleware chain:** recovery (outermost) → request ID → logging → auth → router (lesson 21).
- **Resilience:** server timeouts, `MaxBytesReader` on bodies, context passed into all DB calls, graceful shutdown that drains in-flight requests *and* stops the background worker (lessons 15, 20, 21).
- **Tests:** table-driven unit tests for the service (with an in-memory fake repo), `httptest` tests for handlers, and at least one repository test against a real DB. Run with `-race` (lesson 18).
- **Quality:** `gofmt`, `go vet`, and `golangci-lint` all clean (lesson 24).

## Suggested build order
1. **Scaffold** the module and the `cmd/`/`internal/` layout. Define `domain` types and the `TaskRepository` interface.
2. **In-memory repo first** — implement the repository with a mutex-guarded map so you can build and test the whole stack with no DB.
3. **Service layer** — business logic + validation, depending only on the interface. Unit-test it with the fake repo.
4. **HTTP layer** — handlers, DTOs, routing (1.22 mux), JSON helpers, status codes. Test with `httptest`.
5. **Middleware** — recovery, request ID, logging, auth; wire the chain in `main`.
6. **Config + logging** — `Config.Load()`, `slog` setup, health/readiness endpoints.
7. **Swap in Postgres** — implement the SQL repository, add migrations, change one line in `main`. Add a repo-level test.
8. **Background worker** — add the goroutine/channel/ticker job tied to `context`; verify it stops on shutdown.
9. **Harden** — timeouts, body limits, graceful shutdown, error-response polish.
10. **Quality pass** — lint, vet, `-race` tests, fill coverage gaps, write doc comments.

## Definition of done
- [ ] All endpoints work end-to-end (verify with `curl`/HTTP client); correct status codes throughout.
- [ ] Layered architecture: `main` is thin; service has no `net/http`/`database/sql` imports; no import cycles.
- [ ] Config from env with fail-fast; secrets never logged.
- [ ] Structured logs with request IDs; recovery middleware keeps the server up on a handler panic.
- [ ] Errors wrapped and surfaced as clean JSON; `ErrNotFound` → 404.
- [ ] Postgres-backed with migrations; all queries parameterized and context-aware.
- [ ] Background worker runs and shuts down cleanly with the server.
- [ ] Graceful shutdown drains in-flight requests.
- [ ] `go test -race ./...`, `go vet ./...`, and `golangci-lint run` all pass.
- [ ] README explains how to configure, run, and test the service.

## Stretch goals
- Real auth: `POST /login` issues a JWT (or signed token); middleware verifies it; password hashing with `bcrypt`.
- Rate-limiting middleware (`golang.org/x/time/rate`).
- Pagination metadata + sorting; idempotent `PUT`.
- Dockerfile (multi-stage build → tiny static image) and `docker-compose` with Postgres.
- Prometheus `/metrics` + a Grafana dashboard (lesson 23).
- OpenAPI/Swagger spec for the API.
- CI workflow (GitHub Actions): build, vet, lint, `-race` tests on every push.

## Best Practices & Pitfalls (review)
- **Start with the in-memory repo** so you can build the full vertical slice before touching a database — fast feedback, easy tests.
- **Keep the dependency direction inward**; if the service layer reaches for `database/sql` or `net/http`, stop and fix the seam.
- **Don't over-engineer the capstone.** It should demonstrate the patterns cleanly, not become a framework. Cut scope (drop users/auth) before cutting quality (tests, shutdown, errors).
- **Tie every goroutine to a context** and prove shutdown is clean — the background worker is the most common place to leak.
- **Treat lint/vet/`-race` as part of "done,"** not an afterthought.

## Resources
- Article — How I write HTTP services in Go (Mat Ryer): https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
- Guide — Accessing relational databases: https://go.dev/doc/database/
- Standard project layout: https://github.com/golang-standards/project-layout
- Tutorials index: https://go.dev/doc/tutorial/
