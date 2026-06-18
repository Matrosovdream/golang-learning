# Watch Hub — intermediate · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.
Focus: **intermediate concurrency** — background scheduler (goroutine per monitor),
context-tree cancellation, channel semaphore + token-bucket rate limiting,
retry/backoff, SSE pub/sub (actor pattern), errgroup, two-level locking, graceful drain.
Storage is **in-memory** by design; one external dep: **golang.org/x/sync/errgroup**.

> ▶ **Resume here:** everything typed; `go mod tidy`, `gofmt`, `go build ./...`,
> `go vet ./...` all clean; full live smoke test passed under `go run -race` with
> **no data races** — register/list/get+history, background up/error transitions,
> retries (`attempts:2`), immediate check, errgroup `/checks`, SSE events captured,
> delete→204, 400/404 cases, and graceful SIGTERM drain. Last step: ▶ Run & verify.
>
> Sibling of **[ping-hub-beginner](../ping-hub-beginner/)** (the synchronous version).
> User's rebuild lives at **`projects/watch-hub/`** (repo root). This reference copy stays untouched.

### 🧱 Scaffold
- [x] Folder tree created (note `service/` has 4 files: scheduler/checker/limiter/events)
- [x] go.mod (`module watchhub`, require golang.org/x/sync) + go.sum (`go mod tidy`)

### 🟢 Core (inside-out)
- [x] internal/domain/monitor.go (Monitor/MonitorView/CheckResult, Status, repo interface)
- [x] internal/config/config.go (concurrency knobs)
- [x] internal/repository/memory/monitor_store.go (★ two-level locking + history ring)
- [x] internal/service/limiter.go (token bucket: ticker + buffered channel)
- [x] internal/service/checker.go (★ semaphore + rate limit + retry/backoff + timeout)
- [x] internal/service/events.go (SSE pub/sub hub — actor pattern, lock-free)
- [x] internal/service/scheduler.go (★ goroutine-per-monitor lifecycle + cancellation + drain)
- [x] internal/handler/response.go
- [x] internal/handler/monitor_handler.go (CRUD + immediate check + errgroup batch)
- [x] internal/handler/events_handler.go (GET /events SSE + heartbeat)
- [x] internal/router/router.go
- [x] cmd/api/main.go (Shutdown → sched.Stop → limiter/hub close)

### 🐳 Infra
- [x] Dockerfile (ca-certificates; go mod download with the one dependency)
- [x] docker-compose.yml (single app service, no db)
- [x] .env
- [x] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` and `go build ./...` / `go vet ./...` succeed
- [ ] `docker compose up --build` → app listening; `/stats` shows goroutines climbing as monitors register
- [ ] POST /monitors → 201 `pending`; seconds later GET /monitors/{id} shows `up`/`error` + history
- [ ] A bad-host monitor shows `attempts:2` (retry/backoff) and `status:"error"`
- [ ] `curl -N /events` streams `event: status` frames as statuses change
- [ ] POST /checks (errgroup) returns up/down/errored tallies; POST /monitors/{id}/check runs now
- [ ] DELETE /monitors/{id} → 204 (goroutine cancelled); timeout>interval → 400; unknown id → 404
- [ ] `go run -race ./cmd/api` under concurrent load → no data race; SIGTERM drains cleanly

---
*Project description: [README.md](README.md).*
