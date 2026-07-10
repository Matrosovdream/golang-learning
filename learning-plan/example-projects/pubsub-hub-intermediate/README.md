# PubSub Hub — intermediate

An **in-memory publish/subscribe broker over Server-Sent Events (SSE)**.
Subscribers open a long-lived HTTP stream on a topic; one `POST` to that topic
**fans the message out to every live subscriber at once**. A subscriber that
can't keep up is **dropped**, never allowed to block the publisher or the other
subscribers.

This is a **concurrency project** in the `-hub` family. Where its sibling
`ping-hub-beginner` teaches **fan-out / fan-in** with a bounded worker pool,
this one teaches the other half of the toolkit: **broadcast fan-out to many live
consumers with `select`-based non-blocking delivery and backpressure**, streamed
to clients as real-time SSE.

| Concept                                   | Where it lives in this project                                                       |
|-------------------------------------------|--------------------------------------------------------------------------------------|
| **goroutine-per-connection**              | the HTTP server runs each SSE handler on its own goroutine — the blocking loop is fine |
| **buffered channels** `make(chan T, n)`   | each subscriber's mailbox in [`broker.go`](internal/service/broker.go)               |
| **directional channel types** `<-chan`    | `Subscribe` returns a **receive-only** channel                                       |
| **non-blocking send** `select { … default }` | the ★ fan-out in `Publish` — drop a slow consumer instead of blocking                |
| **`select` + `ctx.Done()`**               | the SSE loop exits when the client disconnects                                       |
| **`close` + `v, ok := <-ch`**             | `Unsubscribe`/`Shutdown` close the mailbox; the reader sees `ok == false` and leaves |
| **`sync.RWMutex`**                        | many concurrent `Publish` (RLock) vs. exclusive `Subscribe`/`Unsubscribe` (Lock)     |
| **`sync/atomic`**                         | id generator + `published`/`delivered`/`dropped` counters, lock-free                  |
| **type assertion** `w.(http.Flusher)`     | pushing bytes mid-request for streaming                                              |

> **No database on purpose.** This is a **live broker**: it delivers only to
> subscribers connected *right now* and keeps **no history** — a message
> published to a topic with zero subscribers is simply gone. The entire state is
> a `map` guarded by a `sync.RWMutex`. Adding Postgres would turn this into a
> message queue and bury the concurrency lesson.

---

## What you'll see

Open **two terminals**. In the first, subscribe and leave it streaming
(`curl -N` disables buffering so frames arrive as they're sent):

```bash
curl -N localhost:8080/topics/demo/subscribe
```
```text
: subscribed to "demo"

data: {"id":2,"topic":"demo","body":"hello","created_at":"2026-07-10T12:00:01+07:00"}

data: {"id":3,"topic":"demo","body":"again","created_at":"2026-07-10T12:00:03+07:00"}
```

In the second terminal, publish a couple of messages — each returns `202` and
appears instantly in the first terminal:

```bash
curl -X POST localhost:8080/topics/demo/publish -d '{"message":"hello"}'
curl -X POST localhost:8080/topics/demo/publish -d '{"message":"again"}'
```
```jsonc
{"id":2,"topic":"demo","body":"hello","created_at":"2026-07-10T12:00:01+07:00"}
```

Start a **second** subscriber and publish again — the one message is delivered to
**both**. That's the fan-out. Watch the tallies climb:

```bash
curl -s localhost:8080/stats
# {"delivered":4,"dropped":0,"published":2,"subscribers":2,"topics":1}
```

Or just open <http://localhost:8080/> — a tiny inline demo page subscribes with
the browser's `EventSource` and has a form to publish, so you can watch the
broadcast live.

## Routes

| Method | Path                        | Purpose                                             | Success |
|--------|-----------------------------|-----------------------------------------------------|---------|
| GET    | `/topics/{topic}/subscribe` | Open an SSE stream of that topic's messages         | 200 (stream) |
| POST   | `/topics/{topic}/publish`   | Fan a message out to every current subscriber       | 202     |
| GET    | `/topics`                   | List live topics: `[{name, subscribers}]`           | 200     |
| GET    | `/stats`                    | `{published, delivered, dropped, topics, subscribers}` | 200  |
| GET    | `/healthz`                  | Liveness probe                                       | 200     |
| GET    | `/`                         | Self-contained browser demo page                    | 200     |

### Request body (`POST /topics/{topic}/publish`)

| Field     | Type     | Notes                                                   |
|-----------|----------|---------------------------------------------------------|
| `message` | `string` | required, 1..`MAX_BODY_BYTES` bytes                     |

An empty or oversized topic name (`> MAX_TOPIC_LEN`) or body returns `400`.

## The broadcast engine

All of it lives in [internal/service/broker.go](internal/service/broker.go) —
the file to read first. The shape is **one-to-many fan-out**: every publish
walks the topic's subscribers and drops the message into each mailbox.

```
                         ┌── sub #1 mailbox (buffered chan) ──► SSE goroutine ──► client
POST /publish ─► Publish ─┼── sub #2 mailbox (buffered chan) ──► SSE goroutine ──► client
                         └── sub #3 mailbox (FULL) ──✗ dropped   (slow client)
```

1. **Subscribe** allocates a unique id (`nextID.Add(1)`, atomic), makes a
   **buffered** channel (`make(chan domain.Message, bufSize)`) as that
   subscriber's private mailbox, registers it under `mu.Lock()`, and returns a
   **receive-only** `<-chan` so the caller can only read from it.
2. **The SSE handler** ([pubsub_handler.go](internal/handler/pubsub_handler.go))
   blocks in a `for { select { … } }` loop receiving from that channel and
   flushing each message to the client. The HTTP server already runs this on its
   own goroutine, so blocking parks just this one connection.
3. **Publish** builds the message, then under `mu.RLock()` snapshots the topic's
   subscribers and does a **non-blocking send** into each mailbox.
4. **Unsubscribe** (via `defer`, on client disconnect) `close`s the mailbox so
   the SSE loop's `v, ok := <-ch` returns `ok == false` and the handler exits.

### The non-blocking send / slow-consumer drop

This is the heart of the project:

```go
select {
case sub.ch <- msg:
    b.delivered.Add(1)
default:
    b.dropped.Add(1) // slow consumer: drop rather than block the publisher
}
```

A `select` with a `default` case **never blocks**. If the send can't proceed
*right now* — the subscriber's buffered mailbox is full because that client is
reading too slowly — Go takes `default` and the message is **dropped for that
subscriber only**. Without this, a single stalled client would fill its channel,
block the `sub.ch <- msg` send, and freeze the publisher *and every other
subscriber of the topic behind it*. The buffer (`SUB_BUFFER`, default 16)
absorbs brief bursts; past that, dropping is the deliberate **backpressure**
policy that keeps one slow reader from becoming everyone's problem.

> Delivery here is **at-most-once** and best-effort. If you need every message
> guaranteed, you want a durable queue — a different tool with different
> trade-offs. This broker optimises for a fast, never-stalling publisher.

### SSE, `http.Flusher`, and `WriteTimeout: 0`

Server-Sent Events is just a long-lived `text/event-stream` response where the
server writes `data: …\n\n` frames over time. Two things make it work in Go:

- **`flusher, ok := w.(http.Flusher)`** — a **type assertion**. The
  `http.ResponseWriter` you're handed also implements `http.Flusher`; asserting
  it (comma-ok, so no panic if it doesn't) lets us call `flusher.Flush()` to push
  each frame out *before* the handler returns.
- **`WriteTimeout: 0`** on the `http.Server` — no write deadline. An SSE stream
  is open indefinitely; any positive `WriteTimeout` would fire mid-stream and
  kill every subscription. Instead the handler ends cleanly on
  `r.Context().Done()` (client disconnect) or when `broker.Shutdown()` closes the
  mailbox.

## Architecture

Same clean-architecture dependency rule as the rest of the track — pointing
**inward**:

```
handler  ->  service  ->  domain
(SSE/HTTP)   (broker)     (core)
```

There's no repository package: the **broker IS the state**, so it lives in
`internal/service`.

### Layout

```
pubsub-hub-intermediate/
├── cmd/api/main.go                       # wiring + graceful shutdown (WriteTimeout: 0 for SSE)
├── internal/
│   ├── config/config.go                  # env-driven config (port, sub buffer, max sizes)
│   ├── domain/message.go                 # Message, TopicInfo, ValidationError
│   ├── service/broker.go                 # ★ the broadcast broker (RWMutex + buffered chans + atomic)
│   ├── handler/
│   │   ├── pubsub_handler.go             # SSE subscribe + publish/topics/stats + validation
│   │   ├── demo.go                       # tiny self-contained EventSource demo page
│   │   └── response.go                   # JSON write helpers
│   └── router/router.go                  # routes + logging/recover middleware
├── Dockerfile
├── docker-compose.yml                    # single app service (no database)
├── .env
├── go.mod                                # stdlib only — no external dependencies
├── progress.md
└── README.md
```

## Tech stack

- **Go** standard library only — `net/http`, `sync`, `sync/atomic`, `context`,
  `encoding/json`. No external modules, so `go.sum` doesn't exist and the build
  needs no network.
- Go 1.22+ method+pattern routing (`POST /topics/{topic}/publish`, `GET /{$}`).
- SSE is plain HTTP + `http.Flusher` — no framework.
- **Docker Compose** runs a single container (no Postgres).

## Run it

```bash
docker compose up --build
# then subscribe in one terminal:
curl -N localhost:8080/topics/demo/subscribe
# and publish in another:
curl -X POST localhost:8080/topics/demo/publish -d '{"message":"hello"}'
```

Tear down: `docker compose down`

### Run outside Docker

```bash
go run ./cmd/api
# or, to prove the shared map + counters stay clean under concurrent load:
go run -race ./cmd/api
```

There are no dependencies to download — `go run` works immediately.

## Concepts this project teaches

- **Broadcast fan-out**: one publish delivered to N live subscribers by walking a
  `map` of per-subscriber channels.
- **Buffered channels as mailboxes**: `make(chan T, n)` gives each subscriber
  slack to absorb bursts.
- **Non-blocking send**: `select` with a `default` case to drop a slow consumer
  instead of blocking the publisher — the backpressure lesson.
- **`select` with `ctx.Done()`**: ending a streaming handler when the client
  disconnects.
- **Channel lifecycle for signalling**: `close` a channel so a receiver's
  `v, ok := <-ch` reports "no more, we're done".
- **Directional channel types**: returning a receive-only `<-chan` to keep send
  and close on the broker's side, enforced by the compiler.
- **`sync.RWMutex`**: concurrent readers (`Publish`) vs. an exclusive writer
  (`Subscribe`/`Unsubscribe`), verified with `-race`.
- **`sync/atomic`**: lock-free id generation and shared counters.
- **Type assertion** `w.(http.Flusher)` and **SSE** streaming with `WriteTimeout: 0`.
- **Graceful shutdown** of long-lived connections: close every mailbox, then
  `srv.Shutdown(ctx)`.
