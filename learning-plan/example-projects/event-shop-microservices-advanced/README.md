# Event Shop Microservices — advanced (asynchronous / RabbitMQ)

The same shop as `shop-microservices-advanced`, but the services no longer call
each other directly. They communicate **asynchronously through a message
broker** (RabbitMQ), reacting to **events** — a *choreography saga*. An order
is **eventually consistent**: it starts `pending` and converges to `confirmed`
or `cancelled` as events flow.

It is the **seventh and final** project in the example-projects track. Put it
next to project 6 to feel the difference between **synchronous** (gRPC request/
response) and **asynchronous** (event-driven) microservices.

---

## The event flow (choreography saga)

```
client ──POST /orders──► orders ──(order.placed)──► [ shop.events topic exchange ]
                                                         │
        ┌────────────────────────────────────────────────┼───────────────────────────┐
        ▼ order.placed                                     ▼ stock.reserved             ▼ payment.settled / stock.rejected
   ┌───────────┐   reserve stock                     ┌───────────┐  charge          ┌───────────────┐
   │ inventory │──(stock.reserved | stock.rejected)─►│ payments  │─(payment.settled)│ notifications │ (logs)
   └───────────┘                                     └───────────┘                  └───────────────┘
        │                                                 │
        └───────────────► orders consumes stock.reserved (fills items + total),
                          payment.settled (-> confirmed), stock.rejected (-> cancelled)
```

Happy path: `order.placed → stock.reserved → payment.settled → confirmed`.
Rejection: `order.placed → stock.rejected → cancelled`.

No service knows about any other — they only share the **event contracts** in
`pkg/events`. Communication is one-way: publish to a topic exchange, and whoever
bound a queue to that routing key reacts.

## Services

| Service        | HTTP            | DB             | Consumes                              | Publishes                       |
|----------------|-----------------|----------------|---------------------------------------|---------------------------------|
| orders         | `:8080`         | `orders-db`    | stock.reserved, stock.rejected, payment.settled | order.placed          |
| inventory      | `:8081` (admin) | `inventory-db` | order.placed                          | stock.reserved / stock.rejected |
| payments       | –               | `payments-db`  | stock.reserved                        | payment.settled                 |
| notifications  | –               | – (stateless)  | payment.settled, stock.rejected       | –                               |

## Public endpoints

```bash
# product admin (inventory)
POST /products      # {"name","price_cents","stock"}
GET  /products
GET  /products/{id}

# orders
POST /orders        # {"user_id", "items":[{"product_id","quantity"}]}  -> 202 Accepted (pending)
GET  /orders/{id}   # poll this — status moves pending -> confirmed | cancelled
```

`POST /orders` returns **202 Accepted**, not 201 — the order has been *recorded*,
but its fulfilment (stock, payment) happens later, asynchronously.

## What you'll see

```bash
# seed stock
curl -s -X POST localhost:8081/products -H 'Content-Type: application/json' -d '{"name":"Keyboard","price_cents":5000,"stock":5}'
curl -s -X POST localhost:8081/products -H 'Content-Type: application/json' -d '{"name":"Mouse","price_cents":2500,"stock":1}'

# place an order — returns 202 with status "pending", total 0, no items yet
curl -s -X POST localhost:8080/orders -H 'Content-Type: application/json' \
  -d '{"user_id":1,"items":[{"product_id":1,"quantity":2},{"product_id":2,"quantity":1}]}'

# poll the order; within moments it becomes "confirmed" with items + total filled in
curl -s localhost:8080/orders/1
# -> {"status":"confirmed","total_cents":12500,"items":[...]}

# an unfulfillable order ends up "cancelled" (stock.rejected)
curl -s -X POST localhost:8080/orders -H 'Content-Type: application/json' \
  -d '{"user_id":1,"items":[{"product_id":1,"quantity":999}]}'
```

Watch the broker live at the RabbitMQ management UI: http://localhost:15672 (guest/guest).

## Tech stack

- **Go** monorepo (single module `eventshop`), four binaries under `services/`.
- **RabbitMQ** (`github.com/rabbitmq/amqp091-go`) with a durable **topic exchange**
  `shop.events`; each service binds a durable queue to the routing keys it cares about.
- **pgx** raw SQL; each stateful service owns and migrates its own Postgres.
- **Docker Compose** runs RabbitMQ + 3 Postgres + 4 services.

## Run it

```bash
docker compose up --build
```

Eight containers come up (broker, three databases, four services). Only `orders`
(`:8080`) and `inventory` (`:8081`) expose HTTP.

Tear down (drops the data volumes): `docker compose down -v`

## Layout

```
event-shop-microservices-advanced/
├── go.mod / go.sum
├── pkg/
│   ├── db/db.go          # Postgres connect-with-retry
│   ├── broker/broker.go  # RabbitMQ connect / publish / consume (topic exchange)
│   └── events/events.go  # routing keys + event payload structs (the contracts)
├── services/
│   ├── orders/        # HTTP + publishes order.placed + consumes 3 events
│   ├── inventory/     # product admin HTTP + consumes order.placed, reserves stock
│   ├── payments/      # consumes stock.reserved, publishes payment.settled
│   └── notifications/ # consumes terminal events, logs (stateless)
├── docker-compose.yml
├── .env
└── README.md / progress.md
```

Each stateful service keeps the clean-architecture layering: `httpapi`/consumer
(transport) → `service` → `domain` ← `repository`, wired in `main`.

## Concepts this project teaches

- **Event-driven architecture** with a broker: topic exchange, durable queues,
  routing-key bindings, manual ack/nack, persistent messages.
- **Choreography saga**: a workflow driven by events with no central coordinator.
- **Eventual consistency**: `POST` returns `202` immediately; the order's final
  state is reached asynchronously (poll `GET /orders/{id}` to observe it).
- **Decoupling**: services depend only on shared event contracts, never on each
  other — add a new consumer (e.g. analytics) without touching the producers.
- Handling business rejections by **publishing an event** (`stock.rejected`)
  rather than returning an error to a caller.

## Sync vs async — compare with project 6

| | `shop-microservices-advanced` (gRPC) | `event-shop-…` (this) |
|---|---|---|
| Comms | synchronous request/response | asynchronous events |
| Coupling | caller knows callee's address | only shared event contracts |
| Failure of a peer | request fails now (`503`) | message waits in the queue |
| Order result | known when the request returns | eventually consistent (poll) |
| Backpressure / spikes | callee must keep up | queue absorbs the burst |

Same domain, two architectures — that contrast is the whole point of building both.
