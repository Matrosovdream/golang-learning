# Ping Hub — beginner

A **concurrent URL health-checker**. POST a list of URLs; the service probes
them all *at the same time* with a bounded pool of workers, then returns a
summary — how many are **up** / **down** / **errored**, plus each URL's HTTP
status and latency.

This is the **concurrency project** of the track. Where the other beginner
projects teach data access (raw SQL, GORM), this one is laser-focused on Go's
concurrency toolkit, mapped to lessons **13 (goroutines)**, **14 (channels)**
and **15 (sync, context & patterns)** of the learning plan:

| Concept       | Where it lives in this project                                              |
|---------------|----------------------------------------------------------------------------|
| **goroutines**| the worker pool, the jobs feeder, the results closer, the server goroutine |
| **channels**  | `jobs` (fan-out) and `results` (fan-in), `close` + `range`, `select`       |
| **sync**      | `sync.WaitGroup` (await workers), `sync.RWMutex` (store), `atomic.Int64`   |
| **context**   | per-URL `context.WithTimeout`, batch cancelled via the request's context   |
| **threads**   | goroutines are *not* OS threads — see [Goroutines vs threads](#goroutines-vs-threads) |

> **No database on purpose.** Every other project ships Postgres; here the store
> is an in-memory map guarded by a `sync.RWMutex` — *that* is the lesson. Adding
> Postgres would only distract from the concurrency primitives.

---

## What you'll see

```bash
# probe four URLs concurrently (per-URL timeout 2s)
curl -s -X POST localhost:8080/checks -H 'Content-Type: application/json' \
  -d '{"urls":[
        "https://example.com",
        "https://httpstat.us/200",
        "https://httpstat.us/404",
        "https://does-not-exist.invalid/"
      ],"timeout_ms":2000}'
```
```jsonc
{
  "id": 1,
  "requested": 4,
  "up": 2, "down": 1, "errored": 1,
  "duration_ms": 312,          // ~ the SLOWEST url, not the sum — that's concurrency
  "created_at": "2026-06-18T22:26:13+07:00",
  "results": [                 // completion order: FASTEST first, not request order
    {"url":"https://example.com","status":"up","http_status":200,"latency_ms":120},
    {"url":"https://httpstat.us/404","status":"down","http_status":404,"latency_ms":210},
    {"url":"https://httpstat.us/200","status":"up","http_status":200,"latency_ms":230},
    {"url":"https://does-not-exist.invalid/","status":"error","latency_ms":35,"error":"no such host"}
  ]
}
```

Two details worth pausing on — both are direct evidence of concurrency:

- **`duration_ms` ≈ the slowest URL**, not the sum of all of them. Five 200 ms
  requests with 5 workers finish in ~200 ms, not 1 s.
- **`results` come back in completion order** (fastest first), *not* the order
  you sent them. The fan-in collects whatever finishes next.

## Routes

| Method | Path           | Purpose                                   | Success |
|--------|----------------|-------------------------------------------|---------|
| POST   | `/checks`      | Probe a batch of URLs concurrently        | 201     |
| GET    | `/checks`      | List past jobs (newest first)             | 200     |
| GET    | `/checks/{id}` | Get one job by id                         | 200     |
| GET    | `/stats`       | `{total_checks, total_jobs}`              | 200     |
| GET    | `/healthz`     | Liveness probe                            | 200     |

### Request body (`POST /checks`)

| Field        | Type       | Notes                                                        |
|--------------|------------|--------------------------------------------------------------|
| `urls`       | `[]string` | required, 1..`MAX_URLS`; each must be a valid `http(s)` URL   |
| `timeout_ms` | `int`      | optional; per-URL timeout (default `DEFAULT_TIMEOUT_MS`, ≤30000) |

A status is `up` (HTTP < 400), `down` (4xx/5xx), or `error` (never got a
response: DNS failure, connection refused, or the per-URL timeout fired).

Bad input returns `400` (empty list, too many URLs, non-http(s) URL); an unknown
job id returns `404`.

## The concurrency engine

All of it lives in
[internal/service/checker.go](internal/service/checker.go) — the file to read
first. The shape is the classic **fan-out / fan-in** pipeline:

```
urls ──► [jobs chan] ──► N worker goroutines ──► [results chan] ──► collector
           (fan-out)          (probe each URL)        (fan-in)        (this goroutine)
```

1. **Fan-out** — start a *fixed* `WORKER_COUNT` of goroutines. Each one loops
   `for url := range jobs`, pulling work until the channel is closed. The fixed
   count is what **bounds** concurrency: 1000 URLs still only open
   `WORKER_COUNT` connections at a time.
2. **Feeder goroutine** — sends every URL into `jobs`, then `close(jobs)`. It
   runs in its own goroutine so the collector can read results *while* work is
   still being handed out (with unbuffered channels, doing both on one
   goroutine would deadlock). Its `select` also watches `ctx.Done()` so a
   cancelled batch stops handing out new work.
3. **Closer goroutine** — `wg.Wait()` blocks until every worker has returned,
   then `close(results)`. Closing is what lets the collector's `range` loop end.
4. **Fan-in** — the calling goroutine does `for res := range results`, tallying
   up/down/errored as each result arrives.

Each probe wraps the request in `context.WithTimeout(ctx, perURL)`, so a slow
host can't pin a worker forever, and cancelling the parent `ctx` cancels every
in-flight request too.

### Why each primitive is here

- **`sync.WaitGroup`** — the collector needs to know *when every worker is
  done* so it can close `results`. A `WaitGroup` counts that without a sleep or
  a guess.
- **`sync.RWMutex`** ([repository/memory](internal/repository/memory/check_repository.go))
  — many HTTP requests can read the store at once (`RLock`), but a save takes
  the exclusive `Lock`. Run with `-race` and it stays clean.
- **`atomic.Int64`** — `totalChecks` is incremented by every worker on every
  probe. A plain `int++` from many goroutines is a data race; `atomic.Add` is
  the lock-free fix for a single counter.
- **`context`** — per-URL deadlines + whole-batch cancellation tied to the HTTP
  request.

## Goroutines vs threads

You asked goroutines to do this work, but you never created an OS **thread**.
Go runs an **M:N scheduler**: many goroutines (millions are fine — each starts
at ~2 KB) are multiplexed onto a small number of OS threads. `GOMAXPROCS` (it
defaults to the number of CPUs; this app logs it at startup) caps how many
threads run Go code *in parallel*. So:

- Spawning a goroutine per URL would *work*, but a **worker pool** is the
  idiomatic way to keep resource use bounded — you rarely want 10 000
  simultaneous outbound connections.
- "Concurrency" (structure: many things in progress) is not the same as
  "parallelism" (many things literally running at once). This service is
  concurrent regardless of `GOMAXPROCS`; it becomes parallel when there's >1 CPU.

## Architecture

Same clean-architecture dependency rule as the rest of the track — pointing
**inward**:

```
handler  ->  service  ->  domain  <-  repository
                          (core)
```

### Layout

```
ping-hub-beginner/
├── cmd/api/main.go                                # wiring + graceful shutdown (goroutine + signal channel)
├── internal/
│   ├── config/config.go                           # env-driven config (worker count, timeout, max urls)
│   ├── domain/check.go                            # CheckJob/CheckResult, Status enum, repo interface, errors
│   ├── repository/memory/check_repository.go      # in-memory store guarded by sync.RWMutex
│   ├── service/checker.go                          # ★ the concurrency engine (pool/channels/WaitGroup/context/atomic)
│   ├── handler/
│   │   ├── check_handler.go                        # controllers + request/response DTOs + validation
│   │   └── response.go                             # JSON write helpers
│   └── router/router.go                           # routes + logging/recover middleware
├── Dockerfile
├── docker-compose.yml                             # single app service (no database)
├── .env
├── go.mod                                          # stdlib only — no external dependencies
├── progress.md
└── README.md
```

## Tech stack

- **Go** standard library only — `net/http`, `sync`, `sync/atomic`, `context`.
  No external modules, so `go.sum` doesn't exist and the build needs no network.
- Go 1.22+ method+pattern routing (`POST /checks`, `GET /checks/{id}`).
- **Docker Compose** runs a single container (no Postgres).

## Run it

```bash
docker compose up --build
# then, in another terminal:
curl -s -X POST localhost:8080/checks -H 'Content-Type: application/json' \
  -d '{"urls":["https://example.com","https://golang.org"]}'
```

Tear down: `docker compose down`

### Run outside Docker

```bash
go run ./cmd/api
# or, to watch the concurrency-safe store stay clean under load:
go run -race ./cmd/api
```

There are no dependencies to download — `go run` works immediately.

## Concepts this project teaches

- **Worker pool** (fan-out): a fixed number of goroutines reading one `jobs`
  channel to bound concurrency.
- **Fan-in**: collecting results from many goroutines over one channel, in
  completion order.
- **Channel lifecycle**: unbuffered send/receive, `close` to signal "no more",
  `for range` over a channel, and `select` with `ctx.Done()`.
- **`sync.WaitGroup`** to await a dynamic set of goroutines, and the
  "`wg.Wait()` then `close`" closer-goroutine idiom.
- **`sync.RWMutex`** to make an in-memory store safe for concurrent readers and
  exclusive writers (verified with `-race`).
- **`sync/atomic`** for a lock-free shared counter.
- **`context`** for per-operation timeouts and propagating cancellation from an
  HTTP request down to every in-flight goroutine.
- **Graceful shutdown**: run `ListenAndServe` on a goroutine, block on an
  `os/signal` channel, then `srv.Shutdown(ctx)`.
