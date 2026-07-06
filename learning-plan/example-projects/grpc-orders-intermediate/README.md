# gRPC Orders — intermediate (a service mesh with logging & metrics)

A three-service system: a public **REST gateway** fronting two gRPC services,
**catalog** and **orders**, that call each other during checkout. It builds on
`grpc-echo-beginner` by adding **service composition** (orders → catalog),
**clean-architecture** layering, and a full observability story:

- a **request-id** minted at the gateway that flows through **all three** services'
  structured logs, so one `curl` is traceable end-to-end;
- a **Prometheus `/metrics`** endpoint on every service, fed by a metrics
  interceptor (counter + latency histogram, labelled by service/method/code).

Storage is **in-memory** on purpose, to keep the spotlight on gRPC + observability.
The hard project (`grpc-observability-hard`) adds Postgres-per-service and a real
Prometheus + Grafana stack that scrapes these very metrics.

---

## Architecture

```
                          request-id minted at the edge
   client ── REST ──►  ┌───────────┐
                       │  gateway  │  :8080 (public, REST → gRPC)
                       └─────┬─────┘
              ┌─────────────┴─────────────┐
              ▼ gRPC                      ▼ gRPC
        ┌───────────┐               ┌───────────┐
        │  orders   │ ── gRPC ────► │  catalog  │
        │  :9003    │  ReserveStock │  :9002    │
        └───────────┘               └───────────┘
   every service: JSON logs w/ the same request_id  +  /metrics (Prometheus)
```

### The checkout fan-out

`POST /orders` is where services compose. The orders service, for each line:

1. calls **catalog** `ReserveStock` over gRPC — which atomically decrements stock
   and returns the current name + price (a snapshot),
2. accumulates the total and stores the order.

Downstream gRPC codes propagate to the gateway and become HTTP:
unknown product → `404`, insufficient stock (`FailedPrecondition`) → `409`,
catalog down (`Unavailable`) → `503`.

## Gateway REST API

| Method | Path              | Routed to                              |
|--------|-------------------|----------------------------------------|
| POST   | `/products`       | catalog.CreateProduct                  |
| GET    | `/products`       | catalog.ListProducts                   |
| GET    | `/products/{id}`  | catalog.GetProduct                     |
| POST   | `/orders`         | orders.CreateOrder (→ catalog)         |
| GET    | `/orders/{id}`    | orders.GetOrder                        |

## Run it

```bash
docker compose up --build
```

Three services start; the gateway is on `:8080`, and each backend's `/metrics`
is published for a manual peek (`:2112` catalog, `:2113` orders).

```bash
# stock the catalog
curl -s -X POST localhost:8080/products -d '{"name":"Keyboard","price_cents":5000,"stock":5}'
curl -s -X POST localhost:8080/products -d '{"name":"Mouse","price_cents":2500,"stock":1}'

# checkout — gateway → orders → catalog, all sharing one request-id
curl -s -H 'X-Request-Id: order-trace-1' -X POST localhost:8080/orders \
  -d '{"items":[{"product_id":1,"quantity":2},{"product_id":2,"quantity":1}]}'
# -> {"id":1,"status":"confirmed","total_cents":12500,"items":[...]}

curl -s localhost:8080/products/1     # stock is now 3
```

### See the logging & metrics you asked for

One request-id across all three services:

```bash
docker compose logs | grep order-trace-1
# gateway  {"msg":"http_request","service":"gateway","path":"/orders","request_id":"order-trace-1"}
# orders   {"msg":"grpc_request","service":"orders","method":".../CreateOrder","request_id":"order-trace-1"}
# catalog  {"msg":"grpc_request","service":"catalog","method":".../ReserveStock","request_id":"order-trace-1"}
```

Prometheus metrics per service (counter + histogram, labelled by code):

```bash
curl -s localhost:2112/metrics | grep grpc_server_handled_total
# grpc_server_handled_total{code="OK",method=".../ReserveStock",service="catalog"} 2
# grpc_server_handled_total{code="FailedPrecondition",method=".../ReserveStock",service="catalog"} 1
```

Error mapping to prove the gateway's job:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/orders -d '{"items":[{"product_id":999,"quantity":1}]}'  # 404
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/orders -d '{"items":[{"product_id":2,"quantity":9}]}'    # 409
```

Tear down: `docker compose down`.

## Tech stack

- **Go** monorepo (single module `grpcorders`, three binaries under `services/`).
- **gRPC + Protocol Buffers** between services; REST at the gateway.
- **Clean architecture** per backend: `server` (gRPC) → `domain` ← `repository`.
- **`log/slog`** JSON logs + request-id via **metadata**; **Prometheus** metrics via interceptors.
- **In-memory** stores (no DB — deliberate; see the hard project for Postgres).

## Regenerating the gRPC stubs

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/catalog/v1/catalog.proto proto/orders/v1/orders.proto
```

## Layout

```
grpc-orders-intermediate/
├── go.mod / go.sum
├── proto/{catalog,orders}/v1/*.proto + *.pb.go     # contracts + stubs
├── pkg/
│   ├── obs/obs.go            # logger + request-id/logging/metrics interceptors + /metrics server
│   └── grpcserve/serve.go    # gRPC server runner w/ graceful stop
├── services/
│   ├── catalog/  (internal/{domain,repository,server} + main + Dockerfile)
│   ├── orders/   (… + a catalog gRPC client)
│   └── gateway/  (internal/{client,handler} + main + Dockerfile)
├── docker-compose.yml + .env + .dockerignore
└── README.md / progress.md
```

## Concepts this project teaches

- **Service composition**: orders orchestrating catalog over gRPC per request.
- **Clean architecture** applied to a gRPC service (transport/domain/repository).
- **Request-id propagation** across three services via gRPC metadata + client/server
  interceptors — the whole point of "logging between services".
- **Prometheus metrics** via an interceptor, exposed on a side `/metrics` port.
- **gRPC code → HTTP status** mapping at the gateway, incl. `409` (FailedPrecondition)
  and `503` (Unavailable).
- **Database-per-service** thinking: orders snapshots catalog's data, no shared tables.

## Known limitation (on purpose)

If an order has two items and the first `ReserveStock` succeeds but the second
fails, the first item's stock is already decremented with nothing to compensate
it — there's no distributed transaction across services. The fixes are a **saga**
with a compensating `ReleaseStock`, or **events** (eventual consistency). See
`event-shop-microservices-advanced` for the event-driven take.

## Next

`grpc-observability-hard` — the same mesh, but with **Postgres per service**, gRPC
**health checks**, and a real **Prometheus + Grafana** stack that scrapes these
`/metrics` and graphs request rate, error ratio, and p95 latency.
