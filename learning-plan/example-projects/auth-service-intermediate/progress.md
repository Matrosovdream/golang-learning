# Auth Service — intermediate · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**, with `auth` (JWT) and
`middleware` packages. Data layer: **raw SQL via sqlx**.

> ▶ **Resume here:** start at 🧱 Scaffold.

### 🧱 Scaffold
- [ ] Folder tree created (extra `internal/auth` and `internal/middleware`)
- [ ] go.mod (`module authservice`; require sqlx, pgx/v5, jwt/v5, x/crypto)

### 🟢 Core (inside-out)
- [ ] internal/domain/user.go
- [ ] internal/config/config.go
- [ ] internal/auth/token.go
- [ ] internal/repository/postgres/user_repository.go
- [ ] internal/service/auth_service.go
- [ ] internal/middleware/middleware.go
- [ ] internal/handler/response.go
- [ ] internal/handler/auth_handler.go
- [ ] internal/router/router.go
- [ ] cmd/api/main.go

### 🐘 Infra
- [ ] migrations/001_init.sql
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] .env (incl. JWT_SECRET, TOKEN_TTL)
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` succeeds
- [ ] `docker compose up --build` → users table created, app listening
- [ ] POST /register → 201; same email again → 409
- [ ] Weak password / bad email → 400
- [ ] POST /login → 200 token; wrong password or unknown email → 401 (same message)
- [ ] GET /me with no/invalid token → 401; with valid Bearer token → 200
- [ ] password_hash never appears in any response

---
*Project description: [README.md](README.md).*
