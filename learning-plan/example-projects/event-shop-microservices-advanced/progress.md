# Event Shop Microservices — advanced (RabbitMQ) · Progress

A monorepo (`module eventshop`) of four event-driven services around a RabbitMQ
topic exchange. Build the shared contracts first, then each service.

> ▶ **Resume here:** start at 🧱 Scaffold & shared packages.

### 🧱 Scaffold & shared packages
- [ ] Folder tree + go.mod (`module eventshop`)
- [ ] pkg/events/events.go (routing keys + payload structs — the contracts)
- [ ] pkg/broker/broker.go (connect / publish / consume on a topic exchange)
- [ ] pkg/db/db.go (Postgres connect-with-retry)
- [ ] `go build ./pkg/...` passes

### 🧾 orders service
- [ ] internal/domain/order.go (status lifecycle), repository, service (publishes order.placed)
- [ ] internal/httpapi (POST /orders → 202, GET /orders/{id})
- [ ] main.go (consumes stock.reserved / stock.rejected / payment.settled) + Dockerfile

### 📦 inventory service
- [ ] internal/domain/product.go, repository (Reserve tx + FOR UPDATE), service
- [ ] internal/httpapi (product admin)
- [ ] main.go (consumes order.placed → publishes stock.reserved | stock.rejected) + Dockerfile

### 💳 payments service
- [ ] internal/domain/payment.go, repository, service
- [ ] main.go (consumes stock.reserved → publishes payment.settled) + Dockerfile

### 🔔 notifications service
- [ ] internal/service (logs), main.go (consumes payment.settled + stock.rejected) + Dockerfile

### 🐳 Compose & run
- [ ] docker-compose.yml (RabbitMQ + 3 Postgres + 4 services), .env
- [ ] `go build ./...` and `go vet ./...` pass
- [ ] `docker compose up --build` → 8 containers up
- [ ] Seed products via inventory; POST /orders returns 202 pending
- [ ] Poll GET /orders/{id} → becomes confirmed (event chain); total + items filled
- [ ] Over-quantity order → eventually cancelled (stock.rejected)
- [ ] Check payments DB row + notifications logs

---
*Project description: [README.md](README.md).*
