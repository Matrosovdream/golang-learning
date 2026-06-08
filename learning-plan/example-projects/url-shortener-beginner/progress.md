# URL Shortener — beginner · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.

> ▶ **Resume here:** start at 🧱 Scaffold.

### 🧱 Scaffold
- [ ] Folder tree created
- [ ] go.mod (`module urlshortener`)

### 🟢 Core (inside-out)
- [ ] internal/domain/link.go
- [ ] internal/config/config.go
- [ ] internal/repository/postgres/link_repository.go
- [ ] internal/service/link_service.go
- [ ] internal/handler/response.go
- [ ] internal/handler/link_handler.go
- [ ] internal/router/router.go
- [ ] cmd/api/main.go

### 🐘 Infra
- [ ] migrations/001_init.sql
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] .env
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` succeeds
- [ ] `docker compose up --build` → db healthy, app listening
- [ ] POST /shorten returns a code
- [ ] GET /{code} redirects (302)
- [ ] GET /api/stats/{code} shows clicks incrementing

---
*Project description: [README.md](README.md).*
