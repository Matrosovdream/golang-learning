# gRPC Observability — hard · Progress

Build top-down by layer; tick each box once it's typed and the package compiles.
The capstone gRPC project: **Postgres per service**, **gRPC health checks**, and a
full **Prometheus + Grafana** observability stack over the catalog+orders+gateway mesh.

> ▶ **Resume here:** reference copy complete — `go build ./...`, `go vet ./...`,
> `gofmt` clean; configs (dashboard JSON, prometheus.yml, grafana YAML, compose)
> validated; and a **real end-to-end smoke test with Postgres passed**: checkout
> fan-out + order/items stored, GET reads them back, one request-id in all three
> logs, `/metrics` shows counter+histogram+in-flight, and a **5-way concurrent
> oversell test floored stock at 0 (2×201, 3×409)** — proving `FOR UPDATE`. Last
> step for the rebuild: ▶ Run the full stack and open Prometheus + Grafana.

### 🧱 Scaffold
- [x] Folder tree (proto/, pkg/{obs,db,grpcserve}, services/*, deploy/{prometheus,grafana})
- [x] go.mod (`module grpcobs`)
- [x] catalog.proto + orders.proto; generate stubs

### 🔭 Shared packages
- [x] pkg/obs/obs.go — logger, request-id (server+client), logging, Metrics (counter+histogram+**in-flight gauge**), ServeMetrics
- [x] pkg/db/db.go — pgx pool connect-with-retry + Migrate
- [x] pkg/grpcserve/serve.go — gRPC server runner + graceful stop

### 🔴 catalog service (Postgres)
- [x] internal/domain/product.go
- [x] internal/repository/product_repository.go (pgx; Schema; Reserve = **SELECT … FOR UPDATE** tx)
- [x] internal/server/server.go (errors → NotFound / FailedPrecondition)
- [x] main.go (db connect+migrate, interceptors, **health service**, reflection, /metrics)

### 🔴 orders service (Postgres)
- [x] internal/domain/order.go
- [x] internal/repository/order_repository.go (pgx; orders + order_items in **one tx**)
- [x] internal/server/server.go (fan-out to catalog, propagate codes)
- [x] main.go (db, catalog client w/ request-id, health, /metrics)

### 🔴 gateway
- [x] internal/client/clients.go
- [x] internal/handler/handler.go (REST, request-id middleware, code→HTTP)
- [x] main.go

### 🐳 Infra + observability
- [x] Dockerfiles (catalog/orders/gateway)
- [x] docker-compose.yml (2 db + 3 services + prometheus + grafana; db healthchecks)
- [x] deploy/prometheus/prometheus.yml (scrape catalog + orders)
- [x] deploy/grafana/provisioning/{datasources,dashboards}/*.yml
- [x] deploy/grafana/dashboards/grpc.json (rate / p95 / error-ratio / in-flight)
- [x] .env / .dockerignore

### ▶ Run & verify
- [ ] `go build ./...` and `go vet ./...` succeed
- [ ] `docker compose up --build` → 7 containers healthy
- [ ] Create products; place an order → confirmed, correct total; GET /orders/{id} reads items
- [ ] `docker compose logs | grep <request-id>` → same id in gateway + orders + catalog
- [ ] http://localhost:9090 → `grpc_server_handled_total` returns series for both services
- [ ] http://localhost:3000 → "gRPC Services Overview" dashboard shows rate / p95 / errors / in-flight
- [ ] concurrent oversell test → some 201, rest 409; stock never below 0
- [ ] `docker compose down -v` cleans volumes

---
*Project description: [README.md](README.md).*
