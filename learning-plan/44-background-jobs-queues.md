# 44 — Background Jobs & Task Queues

> Part 9, Track B (coordinating services): builds on [13 Goroutines](13-goroutines.md) → [14 Channels](14-channels.md) → [15 sync & context](15-sync-context.md) (in-process concurrency), and pairs with [34 Event-Driven & the Outbox](34-event-driven-outbox.md).
> Those lessons moved work off the request goroutine but kept it *inside the process* — a crash loses it. This lesson is about **durable, out-of-process work**: a Redis-backed queue where the API *enqueues* a job and returns, and a separate **worker binary** drains it — surviving restarts, retrying on failure, and scaling independently of the web tier.

## Goals
- Know **when a durable queue beats an in-process goroutine** — and when it's overkill.
- Split producer from consumer: the API is the **enqueuer**; a **separate worker binary** is the consumer.
- Use **asynq** basics: `Client.Enqueue`, `Server` + `ServeMux` + `HandlerFunc`, a **task type + JSON payload**.
- Internalize **at-least-once delivery ⇒ idempotent handlers**.
- Configure **retries, backoff, and a dead-letter (archived) queue**, and shut the worker down gracefully.

## Concepts

### In-process worker vs. durable queue
Lessons [15](15-sync-context.md)/[26](26-capstone.md) drained work through a goroutine reading a channel: `jobs := make(chan Job); go worker(jobs)`. It's fast and dependency-free — but the channel lives in the process's heap. Kill the process and every buffered job vanishes; there's no retry, and back-pressure is a full channel blocking the producer. A **durable queue** (asynq on Redis) moves the job list *out* of the process:

| | In-process channel worker | Durable queue (asynq/Redis) |
|---|---|---|
| Survives a restart/crash | ❌ jobs in the buffer are lost | ✅ jobs persist in Redis |
| Retries on failure | you hand-roll them | ✅ built-in, with backoff |
| Back-pressure | blocks the producer goroutine | queue absorbs the burst |
| Scale workers independently | ❌ tied to the API process | ✅ run N worker pods |
| Delivery guarantee | at-most-once (in memory) | **at-least-once** |

Reach for a channel when the work is cheap, in-memory, and OK to lose (a metrics flush). Reach for a queue when the work **must not be lost**, may **fail and need retry**, or should **scale separately** from the web tier — exactly the webhook ingestion below.

### Producer and consumer are two processes
The API's only job is to record the webhook and **hand it off**. The `WebhookService` enqueues a typed task and returns; it never touches the case tables:
```go
// internal/worker/tasks.go — the enqueuer the API holds.
const TypeWebhookProcess = "webhook:process" // task type: a routing key, "domain:action"

type WebhookProcessPayload struct {
    WebhookLogID int64           `json:"webhook_log_id"` // struct tags name the JSON fields
    TenantID     int64           `json:"tenant_id"`
    EventType    string          `json:"event_type"`
    EventGUID    string          `json:"event_guid"` // the natural key we dedupe on
    Data         json.RawMessage `json:"data"`
}

type Enqueuer struct{ client *asynq.Client }

func NewEnqueuer(redisAddr string) *Enqueuer {
    return &Enqueuer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

func (e *Enqueuer) EnqueueWebhook(ctx context.Context, p WebhookProcessPayload) error {
    b, err := json.Marshal(p) // serialize the struct to the task's opaque []byte body
    if err != nil {
        return err
    }
    _, err = e.client.EnqueueContext(ctx, asynq.NewTask(TypeWebhookProcess, b)) // task = type tag + bytes
    return err
}
```
The HTTP handler records the row, enqueues, and answers **202 Accepted** — *queued, not done*:
```go
// internal/handler/webhook_handler.go
if err := h.svc.Ingest(r.Context(), tenant, body.EventType, body.EventGUID, body.Data, raw); err != nil {
    writeServiceError(w, err)
    return
}
writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "event_guid": body.EventGUID})
```
202 is the honest status code for asynchronous work: *I've accepted this; I haven't finished it.* The caller gets a fast response; the real ingestion happens elsewhere.

### The worker: Server + ServeMux + typed handlers
The consumer is its **own `main`** (`cmd/worker` is a second binary). It wires the same repositories, builds an `asynq.Server`, and routes each task **type** to a handler with a `ServeMux` — the same mux-and-dispatch shape as [21](21-rest-api.md)'s HTTP router, but for jobs:
```go
// internal/worker/server.go
func NewServer(redisAddr string, p *Processor) (*asynq.Server, *asynq.ServeMux) {
    srv := asynq.NewServer(
        asynq.RedisClientOpt{Addr: redisAddr},
        asynq.Config{Concurrency: 10}, // up to 10 tasks at once, each in its own goroutine
    )
    mux := asynq.NewServeMux()
    mux.HandleFunc(TypeWebhookProcess, p.HandleWebhookProcess) // task type -> a method value bound to p
    return srv, mux
}
```
The handler has the signature `func(context.Context, *asynq.Task) error`. It `json.Unmarshal`s the payload, does the work, and — critically — its **return value is the ack**: `nil` acks (job done), non-`nil` tells asynq to **retry**:
```go
// internal/worker/processor.go
func (p *Processor) HandleWebhookProcess(ctx context.Context, t *asynq.Task) error {
    var pl WebhookProcessPayload
    if err := json.Unmarshal(t.Payload(), &pl); err != nil {
        return fmt.Errorf("decode payload: %w", err) // bad payload -> retry (usually pointless; see below)
    }
    caseID, caseType, amount, currency, guid, err := p.ingest(ctx, pl)
    // ... handle err (idempotency below), then publish an event and return nil
}
```
`main` blocks on `srv.Run(mux)`, which installs its own signal handling and drains in-flight tasks on SIGTERM:
```go
// cmd/worker/main.go
if err := srv.Run(mux); err != nil { // Run blocks; returns non-nil only on a real failure
    log.Fatalf("worker: %v", err)
}
```

### At-least-once ⇒ make handlers idempotent
A durable queue promises **at-least-once**, not exactly-once. If the worker crashes *after* doing the DB insert but *before* acking, asynq redelivers on restart — the same task runs **twice**. So a handler must be safe to run again. The fix is to dedupe on a **natural key** — here the webhook's `EventGUID` — enforced by a unique index in Postgres. The repository's `Insert` returns a `ConflictError` on the duplicate; the handler treats that as **success, not failure**:
```go
caseID, caseType, amount, currency, guid, err := p.ingest(ctx, pl)
if err != nil {
    var conflict domain.ConflictError
    if errors.As(err, &conflict) { // already ingested -> this is a replay, not a failure
        _ = p.webhooks.MarkProcessed(ctx, pl.WebhookLogID, time.Now())
        return nil // ack: do NOT retry a duplicate
    }
    _ = p.webhooks.MarkFailed(ctx, pl.WebhookLogID, err.Error())
    return err // a real failure -> return non-nil so asynq retries
}
```
The rule: **`return nil` for "done or already done"; `return err` only for transient failures worth retrying.** Idempotency is what makes at-least-once safe — without a natural key + unique constraint, a retry double-applies.

### Retries, backoff, and the dead-letter queue
By default asynq retries a failing task **25 times** with exponential backoff, then **archives** it (the dead-letter queue) instead of dropping it — you can inspect and re-run archived tasks. Tune per task at enqueue time:
```go
e.client.EnqueueContext(ctx, asynq.NewTask(TypeWebhookProcess, b),
    asynq.MaxRetry(5),                // give up after 5 attempts, then archive
    asynq.Timeout(30*time.Second),    // ctx is cancelled past this -> counts as a failure
    asynq.Retention(24*time.Hour),    // keep the completed task's record for inspection
    asynq.Queue("webhooks"),          // route to a named queue (see priorities below)
)
```
Because every non-`nil` return is a retry, distinguish a **transient** error (DB briefly down — return it, let asynq retry) from a **permanent** one (payload that will never parse — log it and return `nil`, or it burns all 25 retries archiving garbage). Split traffic across named queues with weighted priority in `asynq.Config{Queues: map[string]int{"critical": 6, "default": 3, "low": 1}}` so a flood of low-priority jobs can't starve the important ones.

### Relation to the outbox
The queue is the **transport**, not a fix for the **dual-write** problem. In `Ingest`, we `webhooks.Create(...)` (a DB write) and then `EnqueueWebhook(...)` (a Redis write) as two separate steps — if the process dies between them, the row exists but no job was queued (or vice-versa). asynq can't make those two writes atomic. The **outbox pattern** does: write the job into the DB *in the same transaction* as the business row, and a relay drains the outbox into the queue. See [34](34-event-driven-outbox.md) — the queue and the outbox are complementary, not alternatives.

## Exercises
1. Build a two-binary asynq setup from scratch: an `Enqueuer` with a `TypeEmailSend` task + JSON payload, and a `cmd/worker` that `srv.Run(mux)`s a handler which logs the payload. Enqueue one and watch the worker process it.
2. Wire the enqueuer into an HTTP handler that returns **202 Accepted** with the task's id, and confirm the response comes back before the job finishes (add a `time.Sleep` in the handler).
3. Force a **duplicate delivery**: enqueue the *same* payload twice. Add an idempotency check keyed on a natural id (a unique index or an in-memory `seen` set) so the second run is a no-op that returns `nil`.
4. Make a handler `return fmt.Errorf("boom")` and watch asynq **retry with backoff**; then enqueue with `asynq.MaxRetry(3)` and confirm it stops after 3 and lands in the **archived** set.
5. Split a transient failure (return the error) from a permanent one (log and return `nil`) in one handler, and explain why the permanent branch must not retry.
6. Send SIGTERM to the worker mid-task and confirm `srv.Run` **drains the in-flight job** before exiting (graceful shutdown) rather than dropping it.

## Best Practices & Pitfalls
- **Make every handler idempotent.** At-least-once means "runs ≥ 1 times." Dedupe on a natural key backed by a unique constraint; treat the conflict as success (`return nil`).
- **Enqueue small, serializable payloads — ids, not objects.** Put a `WebhookLogID`/`TenantID` in the task and re-load from the DB in the worker. The payload is JSON bytes in Redis, not shared memory.
- **The worker is its own binary and its own deploy.** `cmd/worker` scales, restarts, and is monitored separately from the API. Don't run the consumer inside the web process.
- **Use separate queues and priorities.** Route by importance (`critical`/`default`/`low`) so a burst of cheap jobs can't starve latency-sensitive ones.
- **Return `nil` for permanent failures; `err` only for transient ones.** A payload that can never parse should be logged and acked, not retried 25 times.
- **Pitfall — enqueue-after-commit vs the dual-write race.** DB write then queue write are two operations; a crash between them loses one. The queue does **not** solve this — the [outbox](34-event-driven-outbox.md) does.
- **Pitfall — non-idempotent handler double-applies on retry.** "Charge the card" or "insert without a unique key" run twice after a crash-before-ack. Bugs here are silent until a retry actually happens in prod.
- **Pitfall — giant payloads / passing pointers.** You can't put a `*sql.DB` or a 5 MB blob in a task — it's serialized to Redis. Pass an id and a reference, and fetch on the worker side.

## Checklist
- [ ] I can explain when a durable queue beats an in-process channel worker (and when it's overkill).
- [ ] I can enqueue a typed task (`asynq.NewTask` + `json.Marshal` payload) and return **202** from the producer.
- [ ] I can build the consumer: `asynq.NewServer` + `ServeMux.HandleFunc(type, handler)` + `srv.Run(mux)` in a separate binary.
- [ ] I understand `return nil` = ack, `return err` = retry, and I make handlers idempotent on a natural key.
- [ ] I can tune `MaxRetry`/`Timeout`/`Retention`/queue priority and find archived (dead-letter) tasks.
- [ ] I can articulate that the queue is the transport and does **not** solve the dual-write problem.

## Resources
- asynq — simple, reliable Redis-backed task queue: https://github.com/hibiken/asynq (wiki: task retention, retries, priorities).
- asynq `asynqmon` web UI for inspecting queues, retries, and archived tasks: https://github.com/hibiken/asynqmon
- Ties back to: [15 — sync & context](15-sync-context.md) (in-process workers), [34 — Event-Driven & the Outbox](34-event-driven-outbox.md) (atomic enqueue).
- Next (Track B): [35 — Sagas & Distributed Transactions](35-sagas-distributed-transactions.md).
