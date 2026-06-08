# Shop Microservices — advanced (gRPC) · Progress

A monorepo (`module shop`) with one gateway + three gRPC services, each owning
its own Postgres. Build the contracts first, then each service, then wire it up.

> ▶ **Resume here:** start at 🧱 Scaffold & contracts.

### 🧱 Scaffold & contracts
- [ ] Folder tree + go.mod (`module shop`)
- [ ] proto/users/v1/users.proto
- [ ] proto/catalog/v1/catalog.proto
- [ ] proto/orders/v1/orders.proto
- [ ] Generate stubs (`protoc … --go_out --go-grpc_out`), verify `go build ./proto/...`

### 🧰 Shared packages
- [ ] pkg/db/db.go (connect-with-retry + Getenv)
- [ ] pkg/grpcserve/serve.go (gRPC server + graceful stop)

### 👤 users service
- [ ] internal/domain/user.go, repository, server (gRPC)
- [ ] main.go + Dockerfile

### 📦 catalog service
- [ ] internal/domain/product.go, repository (incl. Reserve tx + FOR UPDATE), server
- [ ] main.go + Dockerfile

### 🧾 orders service
- [ ] internal/domain/order.go, repository, server (calls users + catalog clients)
- [ ] main.go (dials users + catalog) + Dockerfile

### 🚪 gateway
- [ ] internal/client/clients.go (dial 3 services)
- [ ] internal/handler/handler.go (REST → gRPC, gRPC-code → HTTP mapping)
- [ ] main.go + Dockerfile

### 🐳 Compose & run
- [ ] docker-compose.yml (3 Postgres + 4 services), .env
- [ ] `go build ./...` and `go vet ./...` pass
- [ ] `docker compose up --build` → all 7 containers healthy
- [ ] POST /users, POST /products work through the gateway
- [ ] POST /orders fans out (orders → users + catalog); stock decremented; total correct
- [ ] Unknown user → 409, short stock → 409, unknown product → 404
- [ ] Kill the catalog container → checkout returns 503

---
*Project description: [README.md](README.md).*
