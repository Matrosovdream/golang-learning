# Order Service — advanced · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
Architecture: **handler → service → domain ← repository**, with a `middleware` package.
Data layer: **raw SQL (pgx)** with explicit transactions + `SELECT … FOR UPDATE`.

> ▶ **Resume here:** start at 🧱 Scaffold.

### 🧱 Scaffold
- [ ] Folder tree created
- [ ] go.mod (`module orderservice`, require pgx/v5)

### 🟢 Core (inside-out)
- [ ] internal/domain/product.go
- [ ] internal/domain/order.go  (OrderStatus + CanTransition state machine)
- [ ] internal/config/config.go
- [ ] internal/repository/postgres/product_repository.go
- [ ] internal/repository/postgres/order_repository.go  (transactions + FOR UPDATE)
- [ ] internal/service/product_service.go
- [ ] internal/service/order_service.go
- [ ] internal/middleware/middleware.go
- [ ] internal/handler/response.go
- [ ] internal/handler/product_handler.go
- [ ] internal/handler/order_handler.go
- [ ] internal/router/router.go
- [ ] cmd/api/main.go

### 🐘 Infra
- [ ] migrations/001_init.sql  (CHECK stock>=0, quantity>0)
- [ ] Dockerfile
- [ ] docker-compose.yml
- [ ] .env
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `go mod tidy` succeeds
- [ ] `docker compose up --build` → tables created, app listening
- [ ] POST /products creates; duplicate sku → 409
- [ ] Checkout decrements stock; order total is correct
- [ ] **Concurrency**: two checkouts of the last unit → exactly one 201, one 409
- [ ] pay → paid → ship → shipped; invalid transitions → 409
- [ ] Cancel restocks items; stock returns to its prior value
- [ ] Insufficient stock → 409; unknown product → 404

---
*Project description: [README.md](README.md).*
