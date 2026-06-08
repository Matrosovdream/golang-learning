# Shop Microservices — advanced (synchronous / gRPC)

The same shop as `order-service-advanced`, but split into **independent
services that call each other live over gRPC**, each owning its **own
database**. A REST **API gateway** is the only public entrypoint.

It is the **sixth project** in the example-projects track and the first
multi-service one. Its twin, `event-shop-microservices-advanced` (project 7),
solves the same domain with **asynchronous** messaging — compare the two to feel
the difference between sync and async microservices.

---

## Architecture

```
                       ┌─────────────┐
   client ── REST ───► │   gateway   │  (public :8080, REST → gRPC)
                       └──────┬──────┘
            ┌─────────────────┼─────────────────┐
            ▼ gRPC            ▼ gRPC            ▼ gRPC
      ┌───────────┐     ┌───────────┐     ┌───────────┐
      │   users   │     │  catalog  │     │  orders   │
      │  :9001    │     │  :9002    │     │  :9003    │
      └─────┬─────┘     └─────┬─────┘     └─────┬─────┘
            │                 │                 │
      ┌─────▼─────┐     ┌─────▼─────┐     ┌─────▼─────┐
      │ users-db  │     │catalog-db │     │ orders-db │   (database per service)
      └───────────┘     └───────────┘     └───────────┘
                              ▲                 │
                              └──── gRPC ───────┘
                        orders calls users + catalog
                        during checkout (the "fan-out")
```

### The checkout fan-out

`POST /orders` is where the services compose. The `orders` service:

1. calls **users** `GetUser` to confirm the buyer exists,
2. calls **catalog** `ReserveStock` to atomically decrement stock and get the
   current prices (a transaction with `SELECT … FOR UPDATE` inside catalog),
3. stores the order (with **price snapshots**) in its **own** database.

Each downstream gRPC status code is mapped to an HTTP status at the gateway, so
an unknown user → `409`, short stock → `409`, unknown product → `404`, and a
**downstream service being down → `503`**.

## Services

| Service   | Transport | Port | Database     | Responsibility                              |
|-----------|-----------|------|--------------|---------------------------------------------|
| gateway   | REST      | 8080 | –            | Public API; translates REST ↔ gRPC          |
| users     | gRPC      | 9001 | `users-db`   | Accounts (`CreateUser`, `GetUser`)          |
| catalog   | gRPC      | 9002 | `catalog-db` | Products + stock (`…`, `ReserveStock`)      |
| orders    | gRPC      | 9003 | `orders-db`  | Places orders; calls users + catalog        |

## Gateway REST API

| Method | Path             | Routed to                              |
|--------|------------------|----------------------------------------|
| POST   | `/users`         | users.CreateUser                       |
| GET    | `/users/{id}`    | users.GetUser                          |
| POST   | `/products`      | catalog.CreateProduct                  |
| GET    | `/products`      | catalog.ListProducts                   |
| GET    | `/products/{id}` | catalog.GetProduct                     |
| POST   | `/orders`        | orders.CreateOrder (→ users + catalog) |
| GET    | `/orders/{id}`   | orders.GetOrder                        |

## Tech stack

- **Go** monorepo (single module `shop`, multiple binaries under `services/`).
- **gRPC + Protocol Buffers** for all internal communication; the gateway speaks REST.
- **pgx** raw SQL; each service owns and migrates its own Postgres on startup.
- **Docker Compose** runs 3 Postgres + 4 services on one network.

## Run it

```bash
docker compose up --build
```

This starts three databases and four services (seven containers). Only the
gateway is published, on `:8080`.

```bash
# create a user and two products
curl -s -X POST localhost:8080/users    -H 'Content-Type: application/json' -d '{"email":"stan@example.com","name":"Stan"}'
curl -s -X POST localhost:8080/products -H 'Content-Type: application/json' -d '{"name":"Keyboard","price_cents":5000,"stock":5}'
curl -s -X POST localhost:8080/products -H 'Content-Type: application/json' -d '{"name":"Mouse","price_cents":2500,"stock":1}'

# checkout — the gateway calls orders, which calls users + catalog over gRPC
curl -s -X POST localhost:8080/orders -H 'Content-Type: application/json' \
  -d '{"user_id":1,"items":[{"product_id":1,"quantity":2},{"product_id":2,"quantity":1}]}'
# -> {"id":1,"user_id":1,"status":"confirmed","total_cents":12500,"items":[...]}

# stock was decremented in the catalog service
curl -s localhost:8080/products/1   # stock 3
```

Tear down (drops all three data volumes): `docker compose down -v`

## Regenerating the gRPC stubs

The generated `*.pb.go` files are committed. To regenerate after editing a
`.proto` (needs `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` on your PATH):

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/users/v1/users.proto proto/catalog/v1/catalog.proto proto/orders/v1/orders.proto
```

## Layout

```
shop-microservices-advanced/
├── go.mod / go.sum                      # one module shared by every service
├── proto/<svc>/v1/*.proto + *.pb.go     # contracts + generated gRPC stubs
├── pkg/
│   ├── db/db.go                         # shared Postgres connect-with-retry
│   └── grpcserve/serve.go               # shared gRPC server runner (graceful stop)
├── services/
│   ├── gateway/   (REST → gRPC clients; internal/client, internal/handler)
│   ├── users/     (internal/{domain,repository,server} + main + Dockerfile)
│   ├── catalog/   (… + ReserveStock transaction)
│   └── orders/    (… + gRPC calls to users & catalog)
├── docker-compose.yml                   # 3 Postgres + 4 services
├── .env
└── README.md / progress.md
```

Each backend service keeps the clean-architecture layering you've used
throughout: `server` (gRPC transport) → `domain` ← `repository`, wired in `main`.

## Concepts this project teaches

- Splitting a domain into services with **database-per-service** (no cross-service
  foreign keys — `orders` stores product id + a name/price snapshot).
- **gRPC + protobuf**: defining contracts, generating stubs, implementing servers,
  and calling them from clients.
- The **API-gateway** pattern: one public REST surface fanning out to internal services.
- **Service composition**: `orders` orchestrating `users` + `catalog` per request.
- Mapping gRPC **status codes** to HTTP, including graceful **`503`** when a
  dependency is down (service discovery via Compose DNS).

## Known limitation (on purpose)

If `catalog.ReserveStock` succeeds but persisting the order then fails, stock has
been decremented with no order to show for it — there is no distributed
transaction across services. The classic fixes are a **saga** with a
compensating `ReleaseStock`, or moving to **events** (eventual consistency) —
which is exactly what project 7 explores.
