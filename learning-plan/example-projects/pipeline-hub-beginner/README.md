# Pipeline Hub — beginner

A **concurrent text-analytics pipeline**. POST a blob of text; it flows through
a chain of stages — each stage a goroutine, each connected to the next by a
channel — and comes back as the **top-N most frequent words**, plus line, word
and unique-word counts.

This is a **concurrency project** of the track. Where the sibling
[ping-hub-beginner](../ping-hub-beginner) teaches the **fan-out / fan-in**
worker pool, this one is laser-focused on the **pipeline pattern** from the Go
blog's *["Pipelines and cancellation"](https://go.dev/blog/pipelines)* — stages
wired by channels, `close` cascading downstream, and `context` tearing the whole
thing down. It maps to lessons **13 (goroutines)**, **14 (channels)** and
**15 (sync, context & patterns)** of the learning plan:

| Concept                       | Where it lives in this project                                                        |
|-------------------------------|---------------------------------------------------------------------------------------|
| **goroutines**                | the source stage, the tokenizer workers, the closer goroutine, the server goroutine   |
| **channels**                  | `lines` and `words` connect the stages; `close` + `for range` + `select`              |
| **directional channel types** | every stage returns `<-chan string` (receive-only) — the compiler enforces the role   |
| **close-cascade**             | each stage `close`s its output when its input drains; shutdown flows downstream        |
| **fan-out / fan-in**          | tokenize fans lines out to `N` workers, then fans their words back into one channel    |
| **sync**                      | `sync.WaitGroup` (await the tokenizer workers), `sync.RWMutex` (store), `atomic.Int64` |
| **context**                   | one `ctx` from the request builds the pipeline; cancel it and every stage tears down   |

> **No database on purpose.** Every other project in the track ships Postgres;
> here the store is an in-memory map guarded by a `sync.RWMutex` — *that* is the
> lesson. Adding Postgres would only distract from the concurrency primitives.

---

## What you'll see

```bash
# push a multi-line text blob through the pipeline
curl -s -X POST localhost:8080/analyze -H 'Content-Type: application/json' \
  -d '{"text":"the cat sat on the mat\nthe cat ran to the cat\nrain rain go away","top_n":5}'
```
```jsonc
{
  "id": 1,
  "lines": 3,          // number of lines the source stage emitted
  "words": 9,          // total surviving words the sink counted (stopwords dropped)
  "unique": 6,         // distinct words == size of the count map
  "duration_ms": 0,    // it's fast: no I/O, just channels and goroutines
  "created_at": "2026-07-10T22:26:13+07:00",
  "top": [             // sorted by count desc, ties broken alphabetically
    {"word":"cat","count":3},
    {"word":"rain","count":2},
    {"word":"away","count":1},
    {"word":"go","count":1},
    {"word":"mat","count":1}
  ]
}
```

Note the pipeline **strips punctuation, lowercases, drops a small stopword set**
(`the`, `on`, `to`, …) **and filters out words shorter than `min_len`** — so
`the` and `on` never reach the counter, and `cat` is tallied across all lines.

## Routes

| Method | Path            | Purpose                                   | Success |
|--------|-----------------|-------------------------------------------|---------|
| POST   | `/analyze`      | Push text through the pipeline            | 201     |
| GET    | `/analyze`      | List past analyses (newest first)         | 200     |
| GET    | `/analyze/{id}` | Get one analysis by id                    | 200     |
| GET    | `/stats`        | `{total_words, total_analyses}`           | 200     |
| GET    | `/healthz`      | Liveness probe                            | 200     |

### Request body (`POST /analyze`)

| Field     | Type     | Notes                                                           |
|-----------|----------|-----------------------------------------------------------------|
| `text`    | `string` | required, non-empty, ≤ `MAX_BYTES`                              |
| `top_n`   | `int`    | optional; how many top words to return (default `DEFAULT_TOP_N`, clamped ≤ 100) |
| `min_len` | `int`    | optional; minimum word length to keep (default `MIN_WORD_LEN`, clamped ≤ 20)    |

Empty text returns `400`; text over `MAX_BYTES` returns `400`; an unknown
analysis id returns `404`.

## The pipeline engine

All of it lives in
[internal/service/pipeline.go](internal/service/pipeline.go) — the file to read
first. Three stages, wired by channels:

```
 text                                                              top-N
  │                                                                  ▲
  ▼                                                                  │
┌────────┐  lines   ┌──────────────────────────┐  words   ┌───────────────┐
│ source │ ───────► │ tokenize  (N goroutines)  │ ───────► │  count (sink) │
│ 1 gor. │ <-chan   │ fan-out ─► workers ─► fan-in │ <-chan │  this gor.    │
└────────┘  string  └──────────────────────────┘  string  └───────────────┘
```

1. **source** — one goroutine splits the text into lines and sends each into the
   `lines` channel, then `close`s it. Every send is guarded by a
   `select { case out <- line: case <-ctx.Done(): return }`, so a cancelled
   request stops the producer immediately (its `defer close(out)` still runs).
2. **tokenize** — the **fan-out**: `WORKER_COUNT` goroutines all `for line :=
   range lines`, so lines are shared out among them. Each worker lowercases the
   line, strips punctuation, drops stopwords and short words, and sends every
   survivor into the `words` channel. A **closer goroutine** does
   `wg.Wait(); close(words)` — the **fan-in** — so `words` closes only after
   *every* worker has finished. (Closing a channel while a worker might still
   send would panic; "wait then close" is the safe idiom.)
3. **count** — the **sink**, running on the calling goroutine (no `go`). It
   `for word := range words` into a map, tallies a total, and bumps a
   process-wide `atomic.Int64`. When `words` closes, this loop ends and unblocks
   the whole call.

Then it flattens the map, sorts by count (ties alphabetical), and returns the
top-N.

### How `close` cascades — and how `ctx` tears it all down

The stages shut down like dominoes, **from the front**:

- `source` finishes its lines and `close`s `lines`.
- Each tokenize worker's `range lines` then ends; the last one to return trips
  `wg.Wait()`, and the closer `close`s `words`.
- The sink's `range words` ends, and `Analyze` returns.

So you never manually stop a downstream stage — **closing its input is the
signal to stop**, and it propagates down the chain on its own. `context` is the
other half: the source and tokenize sends `select` on `ctx.Done()`, so if the
HTTP client disconnects (`r.Context()` is cancelled), the producer stops feeding
and the workers stop mid-flight — the same cascade, triggered from the outside.

### Why each primitive is here

- **`sync.WaitGroup`** — the closer needs to know *when every tokenize worker is
  done* before it can `close(words)`. A `WaitGroup` counts that without a sleep
  or a guess.
- **`sync.RWMutex`** ([repository/memory](internal/repository/memory/analysis_repository.go))
  — many HTTP requests can read the store at once (`RLock`), but a save takes
  the exclusive `Lock`. Run with `-race` and it stays clean.
- **`atomic.Int64`** — `totalWords` is incremented by the sink on every single
  word, across every request. A plain `int++` from concurrent requests is a data
  race; `atomic.Add` is the lock-free fix for one shared counter.
- **`context`** — one cancellation signal, wired from the HTTP request, that
  every stage watches to tear the pipeline down early.

## Pipeline vs fan-out / fan-in

The sibling [ping-hub](../ping-hub-beginner) is a *single* fan-out/fan-in stage:
one pool of workers, one `jobs` channel in, one `results` channel out. This
project **chains stages** — `source → tokenize → count` — where each stage's
output channel is the next stage's input. Fan-out/fan-in still appears *inside*
the tokenize stage, but the headline pattern here is the **pipeline**: composable
stages connected by channels, each closing its output to signal the next that
the stream is done.

## Architecture

Same clean-architecture dependency rule as the rest of the track — pointing
**inward**:

```
handler  ->  service  ->  domain  <-  repository
                          (core)
```

### Layout

```
pipeline-hub-beginner/
├── cmd/api/main.go                                     # wiring + graceful shutdown (goroutine + signal channel)
├── internal/
│   ├── config/config.go                                # env-driven config (worker count, max bytes, top-n, min len)
│   ├── domain/analysis.go                              # Analysis/WordCount, repo interface, errors
│   ├── repository/memory/analysis_repository.go        # in-memory store guarded by sync.RWMutex
│   ├── service/pipeline.go                             # ★ the pipeline engine (stages/channels/WaitGroup/context/atomic)
│   ├── handler/
│   │   ├── analysis_handler.go                         # controllers + request/response DTOs + validation
│   │   └── response.go                                 # JSON write helpers
│   └── router/router.go                                # routes + logging/recover middleware
├── Dockerfile
├── docker-compose.yml                                  # single app service (no database)
├── .env
├── go.mod                                              # stdlib only — no external dependencies
├── progress.md
└── README.md
```

## Tech stack

- **Go** standard library only — `net/http`, `sync`, `sync/atomic`, `context`,
  `strings`, `unicode`, `sort`. No external modules, so `go.sum` doesn't exist
  and the build needs no network.
- Go 1.22+ method+pattern routing (`POST /analyze`, `GET /analyze/{id}`).
- **Docker Compose** runs a single container (no Postgres).

## Run it

```bash
docker compose up --build
# then, in another terminal:
curl -s -X POST localhost:8080/analyze -H 'Content-Type: application/json' \
  -d '{"text":"go go go gophers\nchannels select goroutines","top_n":3}'
```

Tear down: `docker compose down`

### Run outside Docker

```bash
go run ./cmd/api
# or, to prove the store + atomic counter stay clean under concurrent load:
go run -race ./cmd/api
```

There are no dependencies to download — `go run` works immediately.

## Concepts this project teaches

- **Pipeline pattern**: independent stages, each a goroutine, wired by channels,
  where one stage's output channel is the next stage's input.
- **Directional channel types** (`<-chan string`): stage signatures that let the
  compiler enforce a channel is receive-only for its consumer.
- **Channel lifecycle & the close-cascade**: `close` to signal "no more",
  `for range` over a channel to drain it, and closing an input as the signal for
  a downstream stage to stop — propagating shutdown down the chain.
- **Fan-out / fan-in** *within* a stage: many workers off one input channel, then
  the "`wg.Wait()` then `close`" closer-goroutine idiom to merge them.
- **`select` with `ctx.Done()`** on every send, so cancellation tears down the
  whole pipeline from the outside.
- **`sync.RWMutex`** for an in-memory store safe for concurrent readers and
  exclusive writers (verified with `-race`).
- **`sync/atomic`** for a lock-free shared counter incremented from the sink.
- **`context`** for propagating cancellation from an HTTP request into every
  stage goroutine.
- **Graceful shutdown**: run `ListenAndServe` on a goroutine, block on an
  `os/signal` channel, then `srv.Shutdown(ctx)`.
