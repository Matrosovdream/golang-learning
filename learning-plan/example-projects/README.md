# Example Projects — Go backend web services

A hands-on track of **seven runnable web-service projects**, from a single-file
API up to a pair of microservice meshes. Every project is built on the same
**Go clean-architecture** skeleton, ships with **Docker Compose + Postgres**, and
includes a `README.md` (what it is, routes, how to run) and a `progress.md`
build checklist.

The track deliberately alternates data-access styles — **raw SQL** (pgx / sqlx)
vs **GORM** — and finishes with the **same shop in two architectures**
(synchronous gRPC vs asynchronous events) so you can feel the trade-offs.

---

## The shared shape

Every service follows the dependency rule, pointing **inward**:

```
handler / transport  ->  service  ->  domain  <-  repository
                                       (core)
```

- `domain` holds entities + interfaces and imports nothing outward.
- `repository` implements the domain's storage interface (swap it freely).
- `service` holds business rules; `handler`/transport is the only layer that
  knows about HTTP (or gRPC, or the message broker).
- `cmd/api/main.go` (or `services/*/main.go`) is the only place layers are wired.

```
<project>/
├── cmd/api/main.go        # or services/<name>/main.go in the microservice projects
├── internal/
│   ├── config/            # env-driven config
│   ├── domain/            # entities + interfaces (the core)
│   ├── repository/        # data access (Postgres)
│   ├── service/           # business logic
│   └── handler|server/    # HTTP / gRPC transport
├── migrations/ or AutoMigrate
├── Dockerfile + docker-compose.yml + .env
├── README.md + progress.md
└── go.mod / go.sum
```

## The seven projects

### 🟢 Beginner
1. **[url-shortener-beginner](url-shortener-beginner/README.md)** — raw SQL (pgx)
   Shorten URLs, redirect, count clicks. The clean-arch skeleton, stdlib routing,
   parameterized SQL, unique-constraint retry.
2. **[task-manager-beginner](task-manager-beginner/README.md)** — GORM
   Full CRUD with filtering, search and sorting. AutoMigrate, and keeping the ORM
   model separate from the domain entity.

### 🟡 Intermediate
3. **[blog-api-intermediate](blog-api-intermediate/README.md)** — GORM associations
   Posts with comments (hasMany) and tags (many2many), pagination, slug
   generation, and a middleware chain (request-id / logging / recover).
4. **[auth-service-intermediate](auth-service-intermediate/README.md)** — raw SQL (sqlx)
   Register / login / protected route. bcrypt hashing, JWT issue & verify, an auth
   middleware, and uniform login errors (no user enumeration).

### 🔴 Advanced
5. **[order-service-advanced](order-service-advanced/README.md)** — raw SQL + transactions
   Concurrency-safe checkout with `SELECT … FOR UPDATE` (no overselling) and an
   order state machine. The bridge to the microservice projects.

### 🟣 Microservices
6. **[shop-microservices-advanced](shop-microservices-advanced/README.md)** — **synchronous, gRPC**
   A REST gateway fronting `users` / `catalog` / `orders` services that call each
   other over gRPC, each with its own database. API-gateway pattern, service
   composition, gRPC-code → HTTP mapping, graceful `503` when a peer is down.
7. **[event-shop-microservices-advanced](event-shop-microservices-advanced/README.md)** — **asynchronous, RabbitMQ**
   The same shop, but services communicate through events on a topic exchange
   (a choreography saga). Eventual consistency: an order starts `pending` and
   converges to `confirmed` / `cancelled` as events flow.

> Projects **6 and 7 are the same domain in two architectures** — build both and
> compare. See the table at the bottom of project 7's README for sync vs async.

## Skills progression

| Concept                              | Introduced in |
|--------------------------------------|---------------|
| Clean architecture + routing         | 1             |
| Full REST CRUD + validation          | 2             |
| ORM associations + pagination        | 3             |
| Middleware chain                     | 3             |
| Auth (hashing, JWT, gating)          | 4             |
| Transactions + row locking + concurrency | 5         |
| State machines                       | 5             |
| gRPC + protobuf, API gateway         | 6             |
| Database-per-service                 | 6, 7          |
| Message broker, pub/sub, saga        | 7             |
| Eventual consistency                 | 7             |

## Running any project

```bash
cd <project>
docker compose up --build
# then hit the endpoints with curl (see that project's README)
docker compose down -v   # stop and drop the data volumes
```

## Prerequisites

- **Go 1.26+** and **Docker** (with Compose v2).
- Project 6 only: to *regenerate* the gRPC stubs you need `protoc`,
  `protoc-gen-go`, and `protoc-gen-go-grpc` — but the generated `*.pb.go` are
  committed, so `docker compose up` works without them.

Each project's `progress.md` is a checklist for building it yourself, layer by layer.
