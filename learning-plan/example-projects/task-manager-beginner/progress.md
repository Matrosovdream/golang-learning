# Task Manager — beginner · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**. Wiring lives only in `main`.
Data layer: **GORM** (ORM model kept separate from the domain entity).

> ▶ **Resume here:** start at 🧱 Scaffold.

### 🧱 Scaffold
- [ ] Folder tree created
- [ ] go.mod (`module taskmanager`, require gorm + gorm postgres driver)

### 🟢 Core (inside-out)
- [ ] internal/domain/task.go
- [ ] internal/config/config.go
- [ ] internal/repository/postgres/task_repository.go
- [ ] internal/service/task_service.go
- [ ] internal/handler/response.go
- [ ] internal/handler/task_handler.go
- [ ] internal/router/router.go
- [ ] cmd/api/main.go

### 🐘 Infra
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] .env
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` succeeds
- [ ] `docker compose up --build` → db healthy, AutoMigrate runs, app listening
- [ ] POST /tasks creates (201) with defaults applied
- [ ] GET /tasks?status=&priority=&q=&sort=&order= filters/searches/sorts
- [ ] PUT /tasks/{id} updates (200); DELETE /tasks/{id} → 204
- [ ] Invalid status/priority or missing title → 400; unknown id → 404

---
*Project description: [README.md](README.md).*
