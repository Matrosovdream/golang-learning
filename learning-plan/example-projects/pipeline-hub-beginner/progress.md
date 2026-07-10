# Pipeline Hub — beginner · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.
Focus: **concurrency** — the pipeline pattern: stages as goroutines, wired by channels,
`close` cascading downstream, `context` cancellation, plus WaitGroup/RWMutex/atomic.
Storage is **in-memory** by design (no Postgres) so the lesson stays on the primitives.

> ▶ **Resume here:** everything typed, `gofmt -l .` clean, `go build ./...` / `go vet ./...`
> clean, `docker compose config -q` valid, and a live `go run -race` smoke test passed
> (multi-line blob → 201 with a sensible top-N, `/analyze/{id}`, `/analyze`, `/stats`,
> `/healthz`, empty-text 400, unknown-id 404) with **no data races**.
>
> User's rebuild lives at **`projects/pipeline-hub/`** (repo root). This reference copy stays untouched.

### 🧱 Scaffold
- [x] Folder tree created
- [x] go.mod (`module pipelinehub`, stdlib only — no requires, no go.sum)

### 🟢 Core (inside-out)
- [x] internal/domain/analysis.go (Analysis/WordCount, repo interface, errors)
- [x] internal/config/config.go (worker count, max bytes, default top-n, min word len)
- [x] internal/repository/memory/analysis_repository.go (sync.RWMutex store)
- [x] internal/service/pipeline.go (★ source → tokenize (fan-out/fan-in) → count sink, close-cascade + context + atomic)
- [x] internal/handler/response.go
- [x] internal/handler/analysis_handler.go (controllers + DTOs + validation)
- [x] internal/router/router.go
- [x] cmd/api/main.go (goroutine + signal channel graceful shutdown)

### 🐳 Infra
- [x] Dockerfile (no ca-certificates — this app makes no outbound calls)
- [x] docker-compose.yml (single app service, no db)
- [x] .env
- [x] .dockerignore

### ▶ Run & verify
- [x] `gofmt -l .` prints nothing
- [x] `go build ./...` and `go vet ./...` succeed
- [x] `docker compose config -q` valid
- [x] POST /analyze with a multi-line blob → 201 with a sensible top-N (sorted by count)
- [x] GET /analyze/{id} returns the stored analysis; GET /analyze lists newest-first
- [x] GET /stats shows total_words (atomic) growing; empty text → 400; unknown id → 404
- [x] `go run -race ./cmd/api` under concurrent POSTs → no data race reported

---
*Project description: [README.md](README.md).*
