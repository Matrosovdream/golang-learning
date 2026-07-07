# 34 — Event-Driven Architecture & the Transactional Outbox

> Part 9, Track B (coordinating services): **34 Event-Driven & Outbox** → [35 Sagas](35-sagas-distributed-transactions.md) → [36 Resilience](36-resilience-patterns.md) → [37 CQRS & Event Sourcing](37-cqrs-event-sourcing.md).
> [27 — gRPC & Microservices](27-grpc-microservices.md) ended on "the distributed-transaction trap — move to events." This is that move: asynchronous messaging, the delivery guarantees you actually get, and the **transactional outbox** that fixes the dual-write problem.

## Goals
- Decouple services with **asynchronous events** instead of synchronous calls, and know when each fits.
- Design for the real guarantee brokers give you — **at-least-once** delivery — with **idempotent consumers**.
- Solve the **dual-write problem** with the **transactional outbox** pattern.
- Version event schemas so producers and consumers evolve independently.

## Concepts

### Commands vs events; sync vs async
- A **command** is an instruction to *do* something (`ChargeCard`) — one recipient, may fail, often synchronous.
- An **event** is a statement that something *happened* (`OrderPlaced`) — past tense, immutable, **zero or many** interested consumers, fire-and-forget.
Synchronous gRPC/HTTP ([27](27-grpc-microservices.md)) couples caller to callee in **time** (both must be up) and in **behaviour** (the caller waits and handles failure). **Events invert control**: the producer announces a fact and moves on; consumers subscribe on their own schedule. That decoupling is the entire value proposition — and its cost is **eventual consistency**.

Two event styles, worth naming:
- **Event notification** — a thin "it happened, go look" (`OrderPlaced{OrderID}`); consumer calls back for details. Low coupling, more chatter.
- **Event-carried state transfer** — the event carries the data consumers need (`OrderPlaced{OrderID, Items, Total}`); no callback, but the schema is now a shared contract.

### Brokers give you at-least-once, not exactly-once
Kafka, NATS JetStream, RabbitMQ, SQS — the practical default is **at-least-once**: a message is delivered one *or more* times, and ordering is only guaranteed within a partition/key. "Exactly-once" as an end-to-end illusion is achieved by **at-least-once delivery + idempotent processing**, not by the broker magically deduping for you. So design every consumer to tolerate:
- **Duplicates** — the same message twice (redelivery after a crash before ack).
- **Reordering** — across partitions/keys.
- **Redelivery on failure** — nack/timeout re-queues.

### Idempotent consumers
A consumer is **idempotent** when processing the same message twice has the same effect as once. Techniques:
```go
// Dedup by message id in the SAME transaction as the side effect:
func (c Consumer) Handle(ctx context.Context, msg Message) error {
    return c.db.Tx(ctx, func(tx Tx) error {
        // INSERT ... ON CONFLICT DO NOTHING → second delivery is a no-op
        inserted, err := tx.MarkProcessed(ctx, msg.ID)
        if err != nil { return err }
        if !inserted {
            return nil                 // already handled → ack, do nothing
        }
        return c.apply(ctx, tx, msg)   // the real side effect
    })
}
```
Other idempotency levers: **natural upserts** (`ON CONFLICT DO UPDATE` keyed by business id), operations that are naturally idempotent (`SET status='paid'` rather than `balance = balance - amount`), and **idempotency keys** carried on the message ([41](41-api-design-evolution.md)). **Ack only after** the side effect is committed — ack-then-process loses messages on a crash.

### The dual-write problem
The classic bug: a request must change the DB **and** publish an event. If you do them as two separate operations, a crash in between corrupts the system:
```go
// ❌ Two writes, not atomic.
tx.Save(order)      // committed
publish(OrderPlaced) // crash HERE → order exists, event never sent (or vice-versa)
```
There is no way to make a database transaction and a broker publish atomic across two systems. Retrying naively creates *ghost* events (published, DB rolled back) or *lost* events (committed, publish failed).

### The Transactional Outbox — the fix
Write the event into an **outbox table in the same database transaction** as the state change. Now the "did it happen" and "should we tell anyone" facts commit together, atomically. A separate **relay** reads the outbox and publishes to the broker, marking rows sent:
```sql
CREATE TABLE outbox (
    id            uuid PRIMARY KEY,
    aggregate_id  uuid        NOT NULL,
    type          text        NOT NULL,   -- e.g. "OrderPlaced"
    payload       jsonb       NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz                        -- NULL until relayed
);
```
```go
// 1) Producer: state change + event in ONE transaction.
func (s Service) PlaceOrder(ctx context.Context, cmd Cmd) error {
    return s.db.Tx(ctx, func(tx Tx) error {
        if err := tx.SaveOrder(ctx, order); err != nil { return err }
        return tx.InsertOutbox(ctx, OrderPlaced{...}) // same tx → atomic
    })
}

// 2) Relay: poll unpublished rows, publish, mark sent. Runs as a goroutine/worker.
func (r Relay) tick(ctx context.Context) error {
    // FOR UPDATE SKIP LOCKED lets many relay instances share the table safely.
    rows, err := r.db.Query(ctx,
        `SELECT id, type, payload FROM outbox
         WHERE published_at IS NULL ORDER BY created_at
         FOR UPDATE SKIP LOCKED LIMIT 100`)
    if err != nil { return err }
    for _, row := range rows {
        if err := r.broker.Publish(ctx, row.Type, row.Payload); err != nil {
            return err                 // leave unpublished → retried next tick
        }
        _ = r.db.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, row.ID)
    }
    return nil
}
```
Note the relay itself is **at-least-once** (crash after publish, before the `UPDATE`, re-publishes) — which is exactly why consumers must be idempotent. The outbox turns "two systems, no atomicity" into "one atomic DB write + a retryable relay." (The alternative is **Change Data Capture** — Debezium tailing the DB's write-ahead log — same guarantee, no relay code, but more infrastructure.)

### Ordering, partitioning, and poison messages
- **Ordering** is per key. In Kafka, messages with the same key (e.g. `order_id`) go to one partition and stay ordered; across keys, no order. Choose a partition key that matches the ordering you need.
- **Consumer groups** parallelise: N consumers share the partitions, each partition to one consumer at a time.
- **Poison messages** — one that always fails — must not block the stream forever. After K retries, route it to a **dead-letter queue (DLQ)** and alert, then move on.
- **Retries** use backoff ([36](36-resilience-patterns.md)); avoid tight redelivery loops.

### Schema & versioning
The event is a **contract** the moment a second service reads it. Evolve it the same way you'd evolve a proto ([27](27-grpc-microservices.md)):
- **Additive only** — add optional fields; never remove/repurpose one. Old consumers ignore unknown fields.
- Include a **type** and a **version**/schema id on every message; consider a **schema registry** (Avro/Protobuf/JSON Schema) to enforce compatibility in CI.
- **Upcast** old event versions to the current shape on read if you must change structure.

## Exercises
1. Design an `OrderPlaced` event both as *notification* (`{OrderID}`) and *state transfer* (`{OrderID, Items, Total}`). Write one sentence on the coupling trade-off.
2. Implement an idempotent consumer: a `processed_messages(id)` table, insert-on-conflict-do-nothing in the same transaction as the side effect, and prove that replaying the same message twice changes state once.
3. Reproduce the dual-write bug in a comment/diagram: save + publish as two steps, and mark where a crash yields a ghost vs a lost event.
4. Build the outbox: an `outbox` table, a producer that saves an order **and** inserts the event in one transaction, and a relay loop using `FOR UPDATE SKIP LOCKED` that publishes then stamps `published_at`.
5. Kill the relay *after* the publish but *before* the `UPDATE`; restart it and observe the duplicate publish — then confirm your idempotent consumer absorbs it.
6. Choose a partition key for an order stream so all events of one order stay ordered, and explain what is (and isn't) ordered across orders.
7. Evolve the event: add an optional `Coupon` field and show an old consumer still works; then describe how you'd handle a breaking rename via versioning/upcasting.

## Best Practices & Pitfalls
- **Assume at-least-once; make every consumer idempotent.** Dedup by message id or use natural upserts. Never rely on the broker for exactly-once.
- **Ack after the effect commits.** Process → commit → ack. Ack-first loses messages on crash.
- **Never dual-write.** Any "save the DB and publish an event" path must go through the outbox (or CDC). Two independent writes will eventually diverge.
- **The relay is at-least-once too** — that's fine, because consumers are idempotent. Don't try to make the relay exactly-once; make consumers absorb duplicates.
- **Version events additively.** Treat the event schema as a public contract; add optional fields, never break existing ones. Put a type+version on every message.
- **Give every stream a DLQ and a retry cap.** A poison message must not wedge a partition. Dead-letter after K attempts and alert.
- **Pitfall — ordering assumptions.** Consumers that assume global order break under partitioning. Design for per-key order and out-of-order arrival across keys.
- **Pitfall — event-carried state that grows unbounded.** Fat events become a versioning liability and leak the producer's internals. Carry what consumers need, not your whole aggregate.
- **Pitfall — treating events as commands.** An event says *what happened*, not *what to do*. If a producer knows/needs a specific consumer to act, that's a command (or a saga step, [35](35-sagas-distributed-transactions.md)), not an event.

## Checklist
- [ ] I can distinguish commands from events and sync from async coupling, and choose appropriately.
- [ ] I can explain at-least-once delivery and design an idempotent consumer (dedup table / upsert / idempotent op).
- [ ] I can describe the dual-write problem and why DB+broker can't be atomic.
- [ ] I can implement the transactional outbox (same-tx insert + relay with `FOR UPDATE SKIP LOCKED`) and know it's at-least-once.
- [ ] I can reason about partition-key ordering, consumer groups, DLQs, and retries.
- [ ] I can version an event schema additively and describe upcasting for breaking changes.

## Resources
- Chris Richardson, microservices.io patterns — Transactional Outbox, Idempotent Consumer, Event-Driven: https://microservices.io/patterns/data/transactional-outbox.html
- Martin Fowler, "What do you mean by 'Event-Driven'?": https://martinfowler.com/articles/201701-event-driven.html
- Confluent, "Exactly-once semantics are possible" (why it's at-least-once + idempotence): https://www.confluent.io/blog/exactly-once-semantics-are-possible-heres-how-apache-kafka-does-it/
- Debezium (CDC alternative to a relay): https://debezium.io/documentation/reference/stable/
- Watermill (Go event-driven library): https://watermill.io/
- Next: [35 — Sagas & Distributed Transactions](35-sagas-distributed-transactions.md).
