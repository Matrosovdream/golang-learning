# gRPC Observability — hard (the full stack: logs, metrics, dashboards)

The capstone of the gRPC track. The same catalog + orders + gateway mesh as
`grpc-orders-intermediate`, but production-shaped:

- **Database-per-service** — catalog and orders each own a **Postgres**; checkout
  reserves stock inside a `SELECT … FOR UPDATE` transaction (no overselling).
- **gRPC health checks** — every service registers the standard `grpc.health.v1`
  service.
- **Full observability** — structured logs with a **request-id** across every hop,
  **Prometheus** scraping each service's `/metrics`, and a **Grafana** dashboard
  (auto-provisioned) charting request rate, error ratio, p95 latency, and
  in-flight RPCs.

Seven containers: 2 databases, 3 app services, Prometheus, and Grafana.

---

## Architecture

```
   client ─REST─►  ┌───────────┐
                   │  gateway  │ :8080
                   └─────┬─────┘
          ┌─────────────┴─────────────┐
          ▼ gRPC                      ▼ gRPC
    ┌───────────┐   ReserveStock ┌───────────┐
    │  orders   │ ─────gRPC─────► │  catalog  │
    │  :9003    │                 │  :9002    │
    └─────┬─────┘                 └─────┬─────┘
          ▼                             ▼
     ┌─────────┐                   ┌──────────┐
     │orders-db│                   │catalog-db│   (database per service)
     └─────────┘                   └──────────┘

   every service ──/metrics──►  Prometheus :9090  ──►  Grafana :3000
   every service logs JSON with the SAME request_id per request
```

## The three things this project shows

### 1. Logging between services
A `request-id` is minted at the gateway and forwarded on every gRPC hop via
metadata (client + server interceptors in `pkg/obs`). One `curl` → one id in the
gateway's, orders', and catalog's logs:

```bash
docker compose logs | grep <your-request-id>
```

### 2. Metrics (Prometheus + Grafana)
A metrics interceptor records, per RPC: a **counter** (`grpc_server_handled_total`
by method+code), a **latency histogram** (`grpc_server_handling_seconds`), and an
**in-flight gauge** (`grpc_server_in_flight`). Prometheus scrapes both services;
Grafana graphs them on an auto-loaded dashboard.

### 3. Correctness under concurrency
`catalog.ReserveStock` decrements stock inside `SELECT … FOR UPDATE`, so N
concurrent checkouts of the last items can't oversell — the surplus get `409`.

## Run it

```bash
docker compose up --build
```

Open the UIs:
- **Gateway** REST — http://localhost:8080
- **Prometheus** — http://localhost:9090 (try the expression `grpc_server_handled_total`)
- **Grafana** — http://localhost:3000 → dashboard **"gRPC Services Overview"** (anonymous access is on)

Generate some traffic:

```bash
curl -s -X POST localhost:8080/products -d '{"name":"Keyboard","price_cents":5000,"stock":3}'
curl -s -X POST localhost:8080/products -d '{"name":"Mouse","price_cents":2500,"stock":10}'

# a normal order
curl -s -H 'X-Request-Id: trace-42' -X POST localhost:8080/orders \
  -d '{"items":[{"product_id":1,"quantity":1},{"product_id":2,"quantity":2}]}'

# hammer the last units to see FOR UPDATE stop overselling (some 201, rest 409)
for i in $(seq 1 5); do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST localhost:8080/orders \
    -d '{"items":[{"product_id":1,"quantity":1}]}' &
done; wait
curl -s localhost:8080/products/1   # stock floored at 0, never negative
```

Watch the Grafana dashboard move as you send traffic. Tear down (drops the data
volumes): `docker compose down -v`.

### Poke a service's health / API with grpcurl

Every service has reflection + the health service registered:

```bash
grpcurl -plaintext localhost:9002 grpc.health.v1.Health/Check   # requires publishing 9002; internal by default
```

(gRPC ports aren't published by default — they're internal to the Compose network,
like a real mesh. Add a `ports:` entry to a service if you want to probe it directly.)

## Tech stack

- **Go** monorepo (module `grpcobs`, three binaries under `services/`).
- **gRPC + protobuf** between services; **REST** at the gateway.
- **pgx** raw SQL, **database-per-service**, migrate-on-startup, `SELECT … FOR UPDATE`.
- **`log/slog`** JSON logs + request-id via metadata; **Prometheus** metrics via interceptors.
- **Prometheus + Grafana** (dashboard provisioned from `deploy/grafana`).
- **gRPC health** (`grpc.health.v1`) + server reflection.

## Regenerating the gRPC stubs

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/catalog/v1/catalog.proto proto/orders/v1/orders.proto
```

## Layout

```
grpc-observability-hard/
├── go.mod / go.sum
├── proto/{catalog,orders}/v1/*.proto + *.pb.go
├── pkg/
│   ├── obs/obs.go          # logger + request-id/logging/metrics interceptors (+ in-flight gauge) + /metrics
│   ├── db/db.go            # pgx connect-with-retry + migrate
│   └── grpcserve/serve.go  # gRPC server runner + graceful stop
├── services/
│   ├── catalog/  (internal/{domain,repository(pgx, FOR UPDATE),server} + main + health)
│   ├── orders/   (… + order+items in one tx + catalog client + health)
│   └── gateway/  (internal/{client,handler} + main)
├── deploy/
│   ├── prometheus/prometheus.yml
│   └── grafana/
│       ├── provisioning/{datasources,dashboards}/*.yml
│       └── dashboards/grpc.json          # 4-panel dashboard
├── docker-compose.yml + .env + .dockerignore
└── README.md / progress.md
```

## Concepts this project teaches

- **Database-per-service** with real Postgres; migrate-on-startup; `FOR UPDATE`
  to serialise stock reservation.
- **gRPC health checking** (`grpc.health.v1`) + server reflection.
- The **full observability triad**: correlated structured logs, Prometheus metrics
  (counter/histogram/gauge) via interceptors, and Grafana dashboards.
- **PromQL** for the panels: `rate(...)`, error ratio, `histogram_quantile(0.95, ...)`.
- Everything the earlier gRPC projects taught, wired into one production-shaped mesh.

## Known limitation (on purpose)

Same as the intermediate project: a multi-item order that reserves item 1 then
fails on item 2 has no cross-service rollback for item 1. The fix is a **saga**
(compensating `ReleaseStock`) or **events** — see `event-shop-microservices-advanced`.

## The three gRPC projects

| Project | Services | Storage | Observability |
|---------|----------|---------|---------------|
| `grpc-echo-beginner` | gateway + echo | none | slog + request-id |
| `grpc-orders-intermediate` | gateway + catalog + orders | in-memory | + Prometheus `/metrics` |
| `grpc-observability-hard` | + 2 Postgres, Prometheus, Grafana | Postgres/service | + dashboards, histograms, health |
