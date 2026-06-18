# Watch Hub — intermediate

A **concurrent uptime monitor**. Register endpoints to *watch*; a background
scheduler re-checks each one on its own interval, concurrently, with bounded
parallelism, rate limiting, and retry-with-backoff. Query the latest status,
history and uptime — or subscribe to a live **Server-Sent Events** stream and
watch statuses flip in real time.

This is the **grown-up sibling of [ping-hub-beginner](../ping-hub-beginner/README.md)**.
Where Ping Hub probes a batch *once, synchronously, and returns*, Watch Hub runs
work **in the background, on a schedule, decoupled from the request** — the jump
that defines the intermediate concurrency level. It deepens lessons **13–15** of
the plan (goroutines / channels / sync, context & patterns).

| Concept added over the beginner project | Where it lives |
|---|---|
| **Background scheduler** (goroutine per monitor, ticker loop) | [service/scheduler.go](internal/service/scheduler.go) |
| **Per-monitor cancellation** (delete cancels one goroutine; shutdown cancels all) | [scheduler.go](internal/service/scheduler.go) |
| **Bounded global concurrency** (channel semaphore) | [service/checker.go](internal/service/checker.go) |
| **Token-bucket rate limiting** (ticker + buffered channel) | [service/limiter.go](internal/service/limiter.go) |
| **Retry with exponential backoff** (context-aware sleep) | [checker.go](internal/service/checker.go) |
| **Pub/sub broadcast over SSE** (actor pattern, no mutex) | [service/events.go](internal/service/events.go) · [handler/events_handler.go](internal/handler/events_handler.go) |
| **`errgroup` with `SetLimit`** (the batch endpoint) | [handler/monitor_handler.go](internal/handler/monitor_handler.go) |
| **Fine-grained locking** (a mutex per monitor + one for the map) | [repository/memory/monitor_store.go](internal/repository/memory/monitor_store.go) |
| **Graceful drain** (`WaitGroup` over all monitor goroutines) | [cmd/api/main.go](cmd/api/main.go) |

> **Still no database, on purpose** — like Ping Hub, the store is in-memory so the
> spotlight stays on the concurrency primitives. The new lesson in the store is
> **two-level locking**: one `RWMutex` guards the map, and each monitor carries
> its own `RWMutex` for its mutable state.

---

## What you'll see

```bash
# register a monitor — checked every 10s, 2s per-attempt timeout
curl -s -X POST localhost:8080/monitors -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","interval_seconds":10,"timeout_ms":2000}'
# -> {"id":1,"status":"pending",...}   (background loop starts immediately)

# a moment later it has been checked:
curl -s localhost:8080/monitors/1
# -> {"id":1,"status":"up","total_checks":2,"up_checks":2,"uptime_pct":100,
#     "history":[{"status":"up","http_status":200,"latency_ms":120,"attempts":1}, ...]}

# watch status changes live (leave it running in another terminal):
curl -N localhost:8080/events
# : connected
# event: status
# data: {"monitor_id":1,"url":"https://example.com","status":"up","latency_ms":120,"at":"..."}
```

A monitor on an unreachable host shows the retry in the result: `"attempts": 2`
(it tried, backed off, tried again) before settling on `status:"error"`.

## Routes

| Method | Path                      | Purpose                                       | Success |
|--------|---------------------------|-----------------------------------------------|---------|
| POST   | `/monitors`               | Register a monitor; starts a background loop   | 201     |
| GET    | `/monitors`               | List monitors with latest status + uptime      | 200     |
| GET    | `/monitors/{id}`          | One monitor incl. recent history               | 200     |
| DELETE | `/monitors/{id}`          | Stop & remove (cancels its goroutine)          | 204     |
| POST   | `/monitors/{id}/check`    | Trigger an immediate out-of-band check         | 200     |
| POST   | `/checks`                 | One-shot concurrent batch (errgroup), no state | 200     |
| GET    | `/events`                 | **SSE** stream of status-change events         | 200     |
| GET    | `/stats`                  | `{monitors,total_checks,in_flight,goroutines}` | 200     |
| GET    | `/healthz`                | Liveness probe                                 | 200     |

### `POST /monitors` body

| Field              | Type   | Notes                                                |
|--------------------|--------|------------------------------------------------------|
| `url`              | string | required, valid `http(s)` URL                        |
| `interval_seconds` | int    | 1..3600 (default 30) — how often to re-check         |
| `timeout_ms`       | int    | per-attempt timeout (default 3000); must be ≤ interval|

## The concurrency architecture

```
                         ┌───────────── Scheduler ─────────────┐
 POST /monitors  ──────► │  goroutine per monitor (ticker loop)│
                         │  ctx per monitor (cancel on delete) │
                         └───────────────┬─────────────────────┘
                                         │ Check(ctx, url, timeout)
                                         ▼
        ┌──────────────────────────── Checker ───────────────────────────┐
        │  semaphore (chan, cap = MAX_CONCURRENT)  →  bounds the WHOLE app │
        │  Limiter.Wait(ctx) (token bucket)        →  politeness           │
        │  retry × MAX_RETRIES with backoff, each attempt under a timeout  │
        └──────────────────────────────┬──────────────────────────────────┘
                                        │ on status change
                                        ▼
            EventHub (one goroutine owns the subscriber set) ──► SSE clients
```

### Why each primitive is here

- **goroutine per monitor** — each monitor is an independent timer loop
  (`for { select { <-ticker.C / <-ctx.Done() } }`). This is concurrency as
  *structure*: dozens of monitors, each minding its own schedule.
- **context tree** — every monitor's context descends from one root. Deleting a
  monitor calls *its* `cancel()`; shutting down cancels the *root*, which cancels
  all of them at once. Cancellation also reaches in-flight HTTP requests.
- **channel semaphore** (`sem chan struct{}`) — caps how many checks run at once
  across the entire process, so 100 monitors firing together still only open
  `MAX_CONCURRENT` sockets.
- **token bucket** ([limiter.go](internal/service/limiter.go)) — a `time.Ticker`
  drips tokens into a buffered channel; `Wait` blocks for one. Smooths bursts.
- **retry/backoff** — only *transport* errors retry (a 404 is a real answer);
  the backoff sleep is `select`-ed against `ctx.Done()` so it cancels cleanly.
- **EventHub** ([events.go](internal/service/events.go)) — a single goroutine
  owns the subscribers map and everyone else talks to it over channels, so there
  is **no lock**: *share memory by communicating*. Slow clients get events
  dropped (non-blocking send) instead of stalling the hub.
- **two-level locking** ([monitor_store.go](internal/repository/memory/monitor_store.go))
  — the store mutex guards the map; each monitor has its own mutex, so recording
  a result for one monitor never blocks reads of another.
- **WaitGroup drain** — `Stop()` cancels the root context, then `wg.Wait()`
  blocks until every monitor goroutine has actually returned.

### Two pools, two styles

This project shows **two ways to bound a fan-out**, on purpose:

- The **scheduler/checker** uses a *hand-rolled* channel semaphore — the same
  build-it-yourself style as Ping Hub's worker pool, so you see the mechanics.
- The **`POST /checks` batch** uses
  [`golang.org/x/sync/errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup)
  with `g.SetLimit(n)` — the idiomatic library: a bounded pool, context
  propagation, and a single `Wait()` in a handful of lines.

> This is also the project's **one external dependency** — managing a module
> (`go.mod`/`go.sum`, `go mod download` in the Dockerfile) is itself an
> intermediate skill the beginner stdlib-only project skipped.

## Architecture

Same clean-architecture dependency rule as the rest of the track:

```
handler ─► service ─► domain ◄─ repository
                      (core)
```

### Layout

```
watch-hub-intermediate/
├── cmd/api/main.go                                  # wiring + graceful drain (Shutdown → sched.Stop → hub/limiter close)
├── internal/
│   ├── config/config.go                             # concurrency knobs (workers, rate, retries, history)
│   ├── domain/monitor.go                            # Monitor/MonitorView/CheckResult, Status, repo interface, errors
│   ├── repository/memory/monitor_store.go           # two-level-locked in-memory registry + history ring
│   ├── service/
│   │   ├── scheduler.go                             # ★ goroutine-per-monitor lifecycle + cancellation + drain
│   │   ├── checker.go                              # ★ semaphore + rate limit + retry/backoff + per-attempt timeout
│   │   ├── limiter.go                             # token-bucket rate limiter (ticker + buffered channel)
│   │   └── events.go                             # SSE pub/sub hub (actor pattern, lock-free)
│   ├── handler/
│   │   ├── monitor_handler.go                       # CRUD + immediate check + errgroup batch
│   │   ├── events_handler.go                        # GET /events SSE stream + heartbeat
│   │   └── response.go                              # JSON helpers
│   └── router/router.go                             # routes + logging/recover middleware
├── Dockerfile                                        # multi-stage; go mod download (one dep)
├── docker-compose.yml                               # single app service (no database)
├── .env
├── go.mod / go.sum                                  # require golang.org/x/sync
├── progress.md
└── README.md
```

## Configuration

| Env var                 | Default | Meaning                                    |
|-------------------------|---------|--------------------------------------------|
| `APP_PORT`              | 8080    | HTTP port                                  |
| `MAX_CONCURRENT_CHECKS` | 8       | process-wide cap on simultaneous probes    |
| `RATE_PER_SEC`          | 20      | token-bucket refill rate                   |
| `RATE_BURST`            | 10      | token-bucket burst size                    |
| `MAX_RETRIES`           | 2       | attempts per check before giving up        |
| `HISTORY_SIZE`          | 20      | results kept per monitor                   |
| `MAX_MONITORS`          | 100     | registration cap                           |
| `DEFAULT_TIMEOUT_MS`    | 3000    | per-attempt timeout when omitted           |

## Run it

```bash
docker compose up --build

# register a couple of monitors:
curl -s -X POST localhost:8080/monitors -d '{"url":"https://example.com","interval_seconds":5}'
curl -s -X POST localhost:8080/monitors -d '{"url":"https://httpstat.us/500","interval_seconds":5}'

# watch them live:
curl -N localhost:8080/events
```

Tear down: `docker compose down`

### Run outside Docker

```bash
go run ./cmd/api
# prove the shared state stays clean under concurrent load:
go run -race ./cmd/api
```

`GET /stats` reports `goroutines` — register a few monitors and watch it climb;
each monitor really is its own goroutine, multiplexed by the runtime onto a
handful of OS threads (`GOMAXPROCS`, logged at startup).

## Concepts this project teaches

- **Background workers decoupled from requests** — a scheduler of long-lived
  goroutines, each on its own `time.Ticker`.
- **A context tree for lifecycle** — root → per-task; cancel one or cancel all,
  with cancellation reaching in-flight I/O.
- **Bounding concurrency two ways** — a hand-rolled channel **semaphore** and
  **`errgroup.SetLimit`**.
- **Rate limiting** with a **token bucket** built from a ticker + channel.
- **Retry with exponential backoff**, cancellable mid-sleep via context.
- **Pub/sub fan-out** to many subscribers over **SSE**, using the **actor
  pattern** (one owner goroutine, no mutex) with slow-consumer drops.
- **Fine-grained locking** — per-entity mutexes vs one big lock.
- **`sync/atomic`** gauges/counters (in-flight, total checks).
- **Graceful drain** — `WaitGroup` over a dynamic set of goroutines on shutdown.
