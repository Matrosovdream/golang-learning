# Order Service — advanced

A mini e-commerce backend: manage products with stock, place orders that
**safely decrement inventory under concurrency**, and move orders through a
**state machine** (pending → paid → shipped, or → cancelled with restock).

It is the **fifth project** in the example-projects track and the last
single-service one. It returns to **raw SQL (pgx)** and adds the hard parts:
**explicit transactions** and **`SELECT … FOR UPDATE`** row locking.

---

## What you'll see

```bash
# add a product with one unit in stock
curl -s -X POST localhost:8080/products -H 'Content-Type: application/json' \
  -d '{"sku":"WIDGET","name":"Widget","price_cents":500,"stock":1}'

# fire two checkouts of that last unit at the same time:
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/orders \
  -H 'Content-Type: application/json' -d '{"items":[{"product_id":1,"quantity":1}]}' &
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/orders \
  -H 'Content-Type: application/json' -d '{"items":[{"product_id":1,"quantity":1}]}' &
wait
# -> exactly one 201 and one 409; stock ends at 0, never oversold

# the order lifecycle
curl -s -X POST localhost:8080/orders/2/pay      # pending -> paid
curl -s -X POST localhost:8080/orders/2/ship     # paid -> shipped
curl -s -X POST localhost:8080/orders/2/pay      # -> 409 invalid transition

# cancelling restocks the items
curl -s -X POST localhost:8080/orders/3/cancel   # -> cancelled, stock returned
```

## Routes

| Method | Path                        | Purpose                                  | Success |
|--------|-----------------------------|------------------------------------------|---------|
| POST   | `/products`                 | Create a product                         | 201     |
| GET    | `/products`                 | List products                            | 200     |
| GET    | `/products/{id}`            | Get a product                            | 200     |
| POST   | `/products/{id}/restock`    | Add stock (`{"quantity":N}`)             | 200     |
| POST   | `/orders`                   | Checkout (`{"items":[{product_id,quantity}]}`) | 201 |
| GET    | `/orders`                   | List orders (summaries)                  | 200     |
| GET    | `/orders/{id}`              | Get an order with its items              | 200     |
| POST   | `/orders/{id}/pay`          | pending → paid                           | 200     |
| POST   | `/orders/{id}/ship`         | paid → shipped                           | 200     |
| POST   | `/orders/{id}/cancel`       | pending/paid → cancelled (restocks)      | 200     |

## The order state machine

```
pending ──pay──► paid ──ship──► shipped   (terminal)
   │              │
   └──cancel──────┴──► cancelled           (terminal, restocks items)
```

Any other transition (e.g. paying a shipped order) returns **409** and changes
nothing. The rules live in `domain.CanTransition`; the repository enforces them
**inside the same transaction** that updates the row, after locking it.

## Tech stack

- **Go** standard-library HTTP (Go 1.22+ routing).
- **Raw SQL via pgx** with explicit `Begin`/`Commit`/`Rollback` transactions and
  `SELECT … FOR UPDATE` row locks.
- **Postgres 16** with `CHECK` constraints (`stock >= 0`, `quantity > 0`) as a
  database-level safety net; schema applied via initdb.
- Money stored as integer **cents** (no floating point).

## Architecture

```
            middleware (request-id -> logging -> recover)
                              |
handler  ->  service  ->  domain  <-  repository (pgx, transactions)
                           (core: entities + state machine + errors)
```

Two domains (products and orders) each have their own repository, service, and
handler — deliberately separated, which foreshadows the microservice split in
projects 6 and 7.

### How the no-oversell guarantee works

`OrderRepository.CreateOrder` runs one transaction:

1. Sort the lines by product id (stable lock order → no deadlocks between
   concurrent checkouts touching the same products).
2. For each line: `SELECT … FOR UPDATE` to **lock the product row**, check stock,
   then `UPDATE` to decrement.
3. Insert the order and its items; `COMMIT`.

A second checkout for the same product **blocks** on the locked row until the
first commits, then sees the updated stock — so two requests can never both sell
the last unit. Cancelling reverses it: the items are read, then each product is
restocked inside a transaction.

### Layout

```
order-service-advanced/
├── cmd/api/main.go                                  # wiring + pgx pool + shutdown
├── internal/
│   ├── config/config.go                             # env-driven config
│   ├── domain/
│   │   ├── product.go                                # Product entity, repo interface, errors
│   │   └── order.go                                  # Order/Item, OrderStatus + CanTransition, typed errors
│   ├── repository/postgres/
│   │   ├── product_repository.go                     # product CRUD + restock
│   │   └── order_repository.go                       # transactional checkout (FOR UPDATE) + status changes + restock
│   ├── service/
│   │   ├── product_service.go                        # product validation
│   │   └── order_service.go                          # checkout validation, transition naming
│   ├── middleware/middleware.go                     # RequestID, Logger, Recover, Chain
│   ├── handler/
│   │   ├── product_handler.go
│   │   ├── order_handler.go
│   │   └── response.go                               # JSON helpers + domain-error -> HTTP mapping
│   └── router/router.go
├── migrations/001_init.sql                          # products, orders, order_items (+ CHECK constraints)
├── Dockerfile
├── docker-compose.yml
├── .env
├── go.mod / go.sum
├── progress.md
└── README.md
```

## Run it

```bash
docker compose up --build
```

Postgres applies `migrations/001_init.sql` on first boot; the app retries until
the DB is ready and serves on `:8080`.

Tear down (and drop the data volume): `docker compose down -v`

### Run outside Docker

```bash
docker compose up -d db
go run ./cmd/api
```

## Concepts this project teaches

- Explicit SQL transactions with pgx: `Begin`, `Commit`, `defer Rollback`.
- Pessimistic locking with `SELECT … FOR UPDATE` to prevent overselling, and a
  stable lock order to avoid deadlocks.
- Modelling a **state machine** in the domain and enforcing it atomically.
- Snapshotting price onto order items so historical orders are immutable.
- `CHECK` constraints as a defence-in-depth backstop to application logic.
- A pgx gotcha: read a result set fully before issuing another query on the same
  transaction (see `restockOrder`).
- Typed errors carrying data (`InsufficientStockError`, `InvalidTransitionError`)
  mapped to precise HTTP status codes.
