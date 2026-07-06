# gRPC Orders — intermediate · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
This project adds **service composition** (orders → catalog), **clean architecture**,
**request-id propagation across three services**, and **Prometheus /metrics** per service.
Storage is in-memory by design.

> ▶ **Resume here:** reference copy complete — `go build ./...`, `go vet ./...`,
> `gofmt` clean, and a full end-to-end smoke test passed (products, checkout
> fan-out total 12500, stock decremented, 404/409 mapping, one request-id in all
> three services' logs, `/metrics` counters). Last step for the rebuild: ▶ Run &
> verify under Docker Compose.

### 🧱 Scaffold
- [x] Folder tree (proto/, pkg/{obs,grpcserve}, services/{catalog,orders,gateway})
- [x] go.mod (`module grpcorders`)
- [x] proto/catalog/v1/catalog.proto (CreateProduct/GetProduct/ListProducts/ReserveStock)
- [x] proto/orders/v1/orders.proto (CreateOrder/GetOrder; snapshot items)
- [x] Generate stubs for both

### 🔭 Shared packages
- [x] pkg/obs/obs.go — logger, request-id server+client interceptors, logging interceptor
- [x] pkg/obs/obs.go — Metrics (counter + histogram + Go/process collectors), UnaryServerInterceptor, ServeMetrics
- [x] pkg/grpcserve/serve.go — gRPC server runner + graceful stop

### 🟡 catalog service
- [x] internal/domain/product.go (Product, ProductRepository, ErrNotFound/ErrInsufficientStock)
- [x] internal/repository/product_repository.go (in-memory, mutex, Reserve decrements)
- [x] internal/server/server.go (CRUD + ReserveStock; errors → codes.NotFound/FailedPrecondition)
- [x] main.go (wire repo + interceptors + /metrics + serve)

### 🟡 orders service
- [x] internal/domain/order.go (Order/OrderItem snapshots, OrderRepository)
- [x] internal/repository/order_repository.go (in-memory)
- [x] internal/server/server.go (CreateOrder fans out to catalog.ReserveStock, propagates codes)
- [x] main.go (catalog client w/ request-id interceptor + wire + /metrics + serve)

### 🟡 gateway
- [x] internal/client/clients.go (dial catalog + orders, request-id client interceptor)
- [x] internal/handler/handler.go (REST routes, request-id middleware, code→HTTP mapping)
- [x] main.go (wire + graceful shutdown)

### 🐳 Infra
- [x] Dockerfiles (catalog/orders/gateway, multi-stage, non-root)
- [x] docker-compose.yml (3 services, gateway :8080, metrics :2112/:2113 published)
- [x] .env / .dockerignore

### ▶ Run & verify
- [ ] `go build ./...` and `go vet ./...` succeed
- [ ] `docker compose up --build` → gateway on :8080
- [ ] POST two products; POST an order → confirmed, total_cents = 12500
- [ ] GET /products/1 → stock decremented to 3
- [ ] `docker compose logs | grep <request-id>` → same id in gateway + orders + catalog
- [ ] `curl :2112/metrics` → grpc_server_handled_total counters by service/method/code
- [ ] unknown product → 404; over-stock order → 409; `docker compose stop catalog` → order → 503

---
*Project description: [README.md](README.md).*
