# Crawler Hub — hard · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.
Focus: **bounded parallelism over DYNAMIC work** — semaphore, WaitGroup termination,
mutex check-and-set dedup, ticker rate limiter, context cancellation, atomic counters.
Storage is **in-memory** by design (no Postgres) so the lesson stays on the primitives.

> ▶ **Resume here:** everything typed, `gofmt -l .` clean, `go build ./...` /
> `go vet ./...` clean, `docker compose config -q` clean, and a live smoke test passed
> under `go run -race` — a default-seed crawl returns pages>1 with edges>pages (cycles),
> terminates in milliseconds, and repeated + concurrent crawls report **no data race**
> and **no deadlock/hang**.
>
> User's rebuild lives at **`projects/crawler-hub/`** (repo root). This reference copy stays untouched.

### 🧱 Scaffold
- [x] Folder tree created
- [x] go.mod (`module crawlerhub`, stdlib only — no requires, no go.sum)

### 🟢 Core (inside-out)
- [x] internal/domain/crawl.go (PageResult/CrawlJob, repo interface, ErrNotFound + ValidationError)
- [x] internal/config/config.go (concurrency, depth, pages, rate, site size, fetch timeout)
- [x] internal/repository/memory/crawl_repository.go (sync.RWMutex store, newest-first List)
- [x] internal/service/limiter.go (time.Ticker token bucket)
- [x] internal/service/crawler.go (★ semaphore + WaitGroup + mutex admit + context + atomic)
- [x] internal/handler/response.go
- [x] internal/handler/site_handler.go (built-in deterministic cyclic mini-site)
- [x] internal/handler/crawl_handler.go (controllers + DTOs + validation + clamping)
- [x] internal/router/router.go
- [x] cmd/api/main.go (goroutine + signal channel graceful shutdown; logs GOMAXPROCS + MaxConcurrency)

### 🐳 Infra
- [x] Dockerfile (ca-certificates for outbound HTTPS; no go mod download of deps)
- [x] docker-compose.yml (single app service, no db)
- [x] .env
- [x] .dockerignore

### ▶ Run & verify
- [x] `gofmt -l .` prints nothing
- [x] `go build ./...` and `go vet ./...` succeed
- [x] `docker compose config -q` passes
- [x] `go run -race ./cmd/api` → app listening on :8080 (logs GOMAXPROCS + max_concurrency)
- [x] POST /crawls (default seed) → 201 with pages>1, edges>pages (cycles), max_depth_reached set
- [x] Crawl TERMINATES quickly (single-digit ms) — no hang
- [x] GET /site/0 serves the built-in mini-site HTML with relative links
- [x] GET /crawls/{id} returns the stored crawl; GET /crawls lists newest-first
- [x] GET /stats shows total_fetches (atomic) + total_crawls; unknown id → 404; bad seed_url → 400
- [x] max_pages budget respected (e.g. max_pages:5 → pages==5)
- [x] repeated + 6 concurrent crawls under `-race` → NO data race, NO deadlock/panic in log

---
*Project description: [README.md](README.md).*
