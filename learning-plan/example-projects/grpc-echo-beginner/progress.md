# gRPC Echo — beginner · Progress

Build in order; tick each box once it's typed and the package compiles.
This is the first gRPC project: **two services, one gRPC hop, structured logging
with a propagated request-id.** No database — the focus is the gRPC wiring +
Docker Compose + inter-service logging.

> ▶ **Resume here:** reference copy complete — `go build ./...`, `go vet ./...`,
> `gofmt` all clean, and a local end-to-end smoke test passed (curl → gateway →
> echo, one request-id in both logs, 503 when echo is stopped). Last step for the
> rebuild: ▶ Run & verify under Docker Compose.

### 🧱 Scaffold
- [x] Folder tree (`proto/`, `pkg/obs`, `services/{echo,gateway}`)
- [x] go.mod (`module grpcecho`)
- [x] proto/echo/v1/echo.proto (EchoService.Echo; message/served_by/received_at)
- [x] Generate stubs → echo.pb.go + echo_grpc.pb.go

### 🔭 Observability plumbing
- [x] pkg/obs/obs.go — slog JSON logger, NewID (crypto/rand), context helpers
- [x] RequestIDUnaryServer (read/mint id from metadata → context)
- [x] RequestIDUnaryClient (re-attach id to outgoing calls)
- [x] LoggingUnaryServer (one JSON line/RPC: method, code, ms, request_id)

### 🟢 Services
- [x] services/echo/main.go (implement EchoServiceServer, chain interceptors, reflection, graceful stop)
- [x] services/gateway/main.go (HTTP `/echo/{msg}`, mint request-id at the edge, gRPC client, code→HTTP mapping, graceful shutdown)

### 🐳 Infra
- [x] services/echo/Dockerfile (multi-stage, non-root)
- [x] services/gateway/Dockerfile
- [x] docker-compose.yml (echo private, gateway published on :8080)
- [x] .env / .dockerignore

### ▶ Run & verify
- [ ] `go build ./...` and `go vet ./...` succeed
- [ ] `docker compose up --build` → gateway on :8080, echo private
- [ ] `curl /echo/hello` → JSON with message/served_by/received_at/request_id
- [ ] `curl -H 'X-Request-Id: trace-me-123' /echo/world` → same id in gateway AND echo logs
- [ ] `docker compose stop echo` then curl `/echo/hi` → HTTP 503, `{"code":"Unavailable"}`
- [ ] `docker compose down` cleans up

---
*Project description: [README.md](README.md).*
