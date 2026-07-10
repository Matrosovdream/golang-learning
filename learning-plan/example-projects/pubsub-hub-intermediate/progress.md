# PubSub Hub — intermediate · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain**. The broker IS the state, so it
lives in `service` (no repository package). Wiring lives only in `main`.
Focus: **broadcast fan-out** — goroutine-per-connection, buffered channels,
`select` non-blocking send (drop slow consumers), `select` + `ctx.Done()`,
`close`-to-signal, `sync.RWMutex`, `sync/atomic`, `http.Flusher` + SSE.
Storage is **in-memory / live only** by design (no Postgres, no history).

> ▶ **Resume here:** everything typed, `gofmt`/`go build`/`go vet` clean, and a
> live SSE smoke test passed under `go run -race` (subscribe streams frames,
> publish fans out, `/stats` deltas, no data race). Last step: ▶ Run & verify.

### 🧱 Scaffold
- [x] Folder tree created
- [x] go.mod (`module pubsubhub`, stdlib only — no requires, no go.sum)

### 🟢 Core (inside-out)
- [x] internal/domain/message.go (Message, TopicInfo, ValidationError)
- [x] internal/config/config.go (port, sub buffer, max topic len / body bytes)
- [x] internal/service/broker.go (★ RWMutex map + buffered chans + atomic counters + non-blocking send)
- [x] internal/handler/response.go
- [x] internal/handler/pubsub_handler.go (SSE subscribe + publish/topics/stats + validation)
- [x] internal/handler/demo.go (inline EventSource demo page)
- [x] internal/router/router.go
- [x] cmd/api/main.go (WriteTimeout: 0 for SSE; broker.Shutdown before srv.Shutdown)

### 🐳 Infra
- [x] Dockerfile (no ca-certificates — app makes no outbound calls)
- [x] docker-compose.yml (single app service, no db)
- [x] .env
- [x] .dockerignore

### ▶ Run & verify
- [x] `gofmt -l .` prints nothing
- [x] `go build ./...` and `go vet ./...` succeed
- [x] `docker compose config -q` passes
- [x] `curl -N /topics/demo/subscribe` streams the heartbeat then `data:` frames
- [x] `POST /topics/{topic}/publish` → 202; the subscriber receives the message (fan-out)
- [x] GET /topics lists {name, subscribers}; GET /stats shows delivered incrementing
- [x] empty/oversized topic or body → 400; GET /healthz → 200
- [x] `go run -race ./cmd/api` under concurrent publish/subscribe → no data race reported

---
*Project description: [README.md](README.md).*
