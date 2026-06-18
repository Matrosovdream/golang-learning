# Ping Hub — beginner · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.
Focus: **concurrency** — goroutines, channels, sync (WaitGroup/RWMutex/atomic), context.
Storage is **in-memory** by design (no Postgres) so the lesson stays on the primitives.

> ▶ **Resume here:** everything typed, `go build ./...` / `go vet ./...` clean, and a
> live smoke test passed (batch with up/down/error, `/stats`, 400/404 cases) — incl.
> `go run -race` under concurrent load with **no data races**. Last step: ▶ Run & verify.
>
> User's rebuild lives at **`projects/ping-hub/`** (repo root). This reference copy stays untouched.

### 🧱 Scaffold
- [x] Folder tree created
- [x] go.mod (`module pinghub`, stdlib only — no requires, no go.sum)

### 🟢 Core (inside-out)
- [x] internal/domain/check.go (CheckJob/CheckResult, Status enum, repo interface, errors)
- [x] internal/config/config.go (worker count, default timeout, max urls)
- [x] internal/repository/memory/check_repository.go (sync.RWMutex store)
- [x] internal/service/checker.go (★ worker pool + channels + WaitGroup + context + atomic)
- [x] internal/handler/response.go
- [x] internal/handler/check_handler.go (controllers + DTOs + validation)
- [x] internal/router/router.go
- [x] cmd/api/main.go (goroutine + signal channel graceful shutdown)

### 🐳 Infra
- [x] Dockerfile (ca-certificates for outbound HTTPS; no go mod download of deps)
- [x] docker-compose.yml (single app service, no db)
- [x] .env
- [x] .dockerignore

### ▶ Run & verify
- [ ] `go build ./...` and `go vet ./...` succeed
- [ ] `docker compose up --build` → app listening on :8080
- [ ] POST /checks with a mix of URLs → 201 with up/down/errored tallies
- [ ] `duration_ms` ≈ the slowest URL (concurrent, not summed); results in completion order
- [ ] GET /checks/{id} returns the stored job; GET /checks lists newest-first
- [ ] GET /stats shows total_checks (atomic) growing; empty/too-many/bad-scheme urls → 400; unknown id → 404
- [ ] `go run -race ./cmd/api` under concurrent POSTs → no data race reported

---
*Project description: [README.md](README.md).*
