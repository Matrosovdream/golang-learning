# Blog API — intermediate · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**, with a **middleware** package.
Data layer: **GORM** with associations (post *hasMany* comments, post *many2many* tags).

> ▶ **Resume here:** start at 🧱 Scaffold.

### 🧱 Scaffold
- [ ] Folder tree created (note the extra `internal/middleware`)
- [ ] go.mod (`module blogapi`, require gorm + gorm postgres driver)

### 🟢 Core (inside-out)
- [ ] internal/domain/blog.go
- [ ] internal/config/config.go
- [ ] internal/repository/postgres/post_repository.go
- [ ] internal/service/post_service.go
- [ ] internal/middleware/middleware.go
- [ ] internal/handler/response.go
- [ ] internal/handler/post_handler.go
- [ ] internal/router/router.go
- [ ] cmd/api/main.go

### 🐘 Infra
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] .env
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` succeeds
- [ ] `docker compose up --build` → AutoMigrate creates posts/tags/comments/post_tags
- [ ] POST /posts creates with a generated slug; duplicate title → `-2` suffix
- [ ] GET /posts paginates (`total`, `total_pages`) and filters by `q`/`tag`/`published`
- [ ] POST /posts/{id}/comments adds a comment; GET /posts/{id} preloads comments
- [ ] PUT /posts/{id} replaces tags; DELETE cascades comments
- [ ] Every response has an `X-Request-ID` header (middleware chain)

---
*Project description: [README.md](README.md).*
