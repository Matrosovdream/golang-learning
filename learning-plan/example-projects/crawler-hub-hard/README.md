# Crawler Hub — hard

A **concurrent, bounded-parallelism web crawler**. POST a seed URL; the service
fetches it, extracts its links, and follows them — *many at a time* — across the
whole reachable site, deduping as it goes, then returns a summary: how many
**pages** it fetched, how many link **edges** it saw, the deepest level reached,
and how long the whole thing took.

This is the **hard capstone** of the concurrency track. Where
[ping-hub-beginner](../ping-hub-beginner) probes a *fixed* batch of URLs with a
worker pool, this one solves the harder problem: **work that discovers more
work.** Fetching one page reveals more pages to fetch, which reveal more still.
That breaks the beginner's fixed-pool shape and forces the real primitives —
a semaphore, a `WaitGroup` that tracks a *dynamic* set of goroutines, a
mutex-guarded dedup set, a rate limiter, and full context cancellation.

| Concept | Where it lives |
|---------|----------------|
| **counting semaphore** (`chan struct{}`) — bounds simultaneous fetches | [`crawler.go` › `crawl`](internal/service/crawler.go) — `sem <- struct{}{}` / `<-sem` |
| **`sync.WaitGroup`** dynamic termination — add-before-go / done-on-return / wait-then-close | [`crawler.go` › `Run` + `crawl`](internal/service/crawler.go) |
| **`sync.Mutex` check-and-set** — dedup + page budget in one `admit()` | [`crawler.go` › `admit`](internal/service/crawler.go) |
| **`time.Ticker` token bucket** — rate limiting | [`limiter.go`](internal/service/limiter.go) |
| **`context` cancel + timeout** — `WithTimeout`, `NewRequestWithContext`, `select { <-ctx.Done() }` | [`crawler.go`](internal/service/crawler.go) |
| **`sync/atomic`** counters — lock-free `Add`/`Load` | [`crawler.go`](internal/service/crawler.go) |
| **closer goroutine** — `wg.Wait(); close(results)` | [`crawler.go` › `Run`](internal/service/crawler.go) |
| **results channel + collector** — fan-in tally on one goroutine | [`crawler.go` › `Run`](internal/service/crawler.go) |
| **`sync.RWMutex`** in-memory store | [`repository/memory`](internal/repository/memory/crawl_repository.go) |

> **No database on purpose.** Every data-access project in the track ships
> Postgres; here the store is an in-memory map guarded by a `sync.RWMutex` — and
> *that* is the point. A crawl is CPU/network-bound concurrency, not persistence;
> adding a database would only distract from the primitives this project exists
> to teach. Restart the process and the crawl history is gone, by design.

---

## What you'll see

```bash
# crawl the built-in mini-site (default seed), 3 levels deep, 8 fetches at once
curl -s -X POST localhost:8080/crawls -H 'Content-Type: application/json' \
  -d '{"max_depth":3,"concurrency":8}'
```
```jsonc
{
  "id": 1,
  "seed": "http://localhost:8080/site/0",
  "pages": 23,               // distinct pages fetched (dedup already applied)
  "edges": 110,              // total links seen — MORE than pages, because of cycles
  "max_depth_reached": 3,
  "fetches": 23,             // atomic-counted; == pages on a clean run
  "duration_ms": 9,          // the whole crawl, running 8-wide — not the sum of fetches
  "created_at": "2026-07-10T20:18:07+07:00",
  "results": [
    {"url":"http://localhost:8080/site/0","depth":0,"http_status":200,"links_found":4,"latency_ms":3},
    {"url":"http://localhost:8080/site/7","depth":1,"http_status":200,"links_found":5,"latency_ms":0},
    {"url":"http://localhost:8080/site/3","depth":1,"http_status":200,"links_found":5,"latency_ms":0}
    // ...
  ]
}
```

Two numbers are direct evidence the engine is doing its job:

- **`edges` > `pages`.** 110 links point at only 23 distinct pages — the graph has
  **cycles** and back-links. A crawler without dedup would revisit those pages
  forever; ours fetched each exactly once (`fetches == pages`).
- **`duration_ms` ≈ the depth of the graph, not the number of pages.** 23 pages
  fetched 8-wide finish in single-digit milliseconds, because the fetches overlap.

## The built-in mini-site

The seed defaults to `/site/0` — a **generated mini-site this same server hosts**
(see [`site_handler.go`](internal/handler/site_handler.go)). It exists so the
crawl is:

- **offline** — no network, no rate-limit bans, no flaky third parties;
- **reproducible** — the link graph is pure arithmetic (`neighbors(id)`), so the
  same crawl always visits the same pages;
- **cyclic** — pages link forward, sideways, *and* backward (a wrap-around
  back-link), so the graph has cycles. That's the whole reason it's here: cycles
  are exactly what make dedup non-optional. Point a naive crawler at it and it
  never terminates.

Each page is a few bytes of HTML with relative links the crawler must resolve:

```bash
$ curl -s localhost:8080/site/0
<html><body><h1>Page 0</h1><a href="/site/1">page 1</a> <a href="/site/3">..</a> <a href="/site/7">..</a> <a href="/site/29">..</a></body></html>
```

You can still point the crawler at the real internet by passing a `seed_url`
(the Docker image bundles CA certificates for HTTPS) — it just stays on that
seed's host so it can't wander off.

## Routes

| Method | Path            | Purpose                                       | Success |
|--------|-----------------|-----------------------------------------------|---------|
| POST   | `/crawls`       | Start a crawl; returns the job summary        | 201     |
| GET    | `/crawls`       | List past crawls (newest first)               | 200     |
| GET    | `/crawls/{id}`  | Get one crawl by id                           | 200     |
| GET    | `/stats`        | `{total_fetches, total_crawls}`               | 200     |
| GET    | `/site/{page}`  | A page of the built-in mini-site (HTML)       | 200     |
| GET    | `/healthz`      | Liveness probe                                | 200     |

### Request body (`POST /crawls`)

| Field          | Type     | Notes                                                                    |
|----------------|----------|--------------------------------------------------------------------------|
| `seed_url`     | `string` | optional; a valid `http(s)` URL. Omitted → the built-in site on this host |
| `max_depth`    | `int`    | optional; how many links deep to follow (default `3`, ≤ 10)              |
| `max_pages`    | `int`    | optional; the page budget — hard cap on distinct pages (default `100`, ≤ 1000) |
| `concurrency`  | `int`    | optional; simultaneous fetches (default & max = `MAX_CONCURRENCY`)       |
| `rate_per_sec` | `int`    | optional; token-bucket refill rate (default `50`, ≤ 1000)               |

An empty body (`-d '{}'` or none) crawls the built-in site with all defaults.
A malformed body or a non-`http(s)` `seed_url` returns `400`; an unknown crawl id
returns `404`. Every knob is **clamped** to a sane maximum so one request can't
launch an unbounded crawl.

## The concurrency engine

Everything lives in [internal/service/crawler.go](internal/service/crawler.go)
(with the rate limiter in [limiter.go](internal/service/limiter.go)) — the files
to read first. The shape is a **recursive fan-out with a semaphore and a
`WaitGroup`**:

```
                       ┌─────────────── admit() ───────────────┐
                       │  mutex: visited-set + page budget      │
                       │  (dedup — the ONE gate a URL passes)   │
                       └────────────────────────────────────────┘
                                       ▲  true?
                                       │
   seed ─admit─► go crawl(depth 0) ────┼────────────────────────────────┐
                     │                 │                                 │
                     ▼                 │ for each NEW link:              │
              [ acquire sem ]          │   wg.Add(1); go crawl(depth+1) ─┘   (spawns MORE work)
              [ limiter.Wait ]         │
              [ fetch (ctx) ]          │
              [ extract+resolve links ]
                     │
                     ▼
                 results ───► collector (this goroutine): tally pages/edges/depth
                     ▲
   closer goroutine: wg.Wait(); close(results)   ← ends the collector's range loop
```

A numbered walk through one crawl:

1. **Admit the seed.** Before launching anything, `admit(seed)` marks it visited
   and counts it against the budget. `wg.Add(1)` is called, then `go crawl(seed, 0)`.
2. **Bounded goroutines (semaphore).** Each `crawl` goroutine first does
   `sem <- struct{}{}` on a buffered `chan struct{}` of capacity `concurrency`.
   The send blocks once the buffer is full, so **at most `concurrency` fetches run
   at once** no matter how many goroutines exist. It releases with `<-sem` on
   return. A `select` on `ctx.Done()` means a cancelled crawl never parks here.
3. **Fetch under the rate limiter + context.** `limiter.Wait(ctx)` blocks for a
   token bucket refilled by a `time.Ticker`. Then `http.NewRequestWithContext`
   runs the GET under a per-request `context.WithTimeout` — a slow host can't pin
   a goroutine, and cancelling the parent cancels the request mid-flight.
4. **Extract + resolve links.** A `regexp` pulls `href="…"` values; each is
   resolved to an absolute URL with `base.ResolveReference` and kept only if it's
   `http(s)` **on the seed's host** (so the crawl stays put).
5. **Admit the children (dedup + budget).** For every link, `admit(link)` runs the
   *one* check-and-set under the mutex. It returns `true` only for a URL not seen
   before *and* only while the budget has room. A child goroutine is spawned
   **only** for admitted links — `wg.Add(1)` then `go crawl(link, depth+1)`.
6. **Termination (`WaitGroup`).** A separate closer goroutine does
   `wg.Wait(); close(results)`. When the last goroutine (including everything it
   spawned) returns, the count hits zero, `results` closes, and the collector's
   `for range` loop ends. The crawl is over — no sleeps, no polling, no guessing.
7. **Collector (fan-in).** The `Run` goroutine ranges over `results`, tallying
   pages, edges, and max depth. Only this goroutine touches the summary, so it
   needs no lock of its own.

### The termination problem

This is the crux, and it's *why* the beginner's shape doesn't transfer.

Ping-hub knows all its work up front: N URLs, a fixed pool, feed the channel,
`close` it, done. Here the work is **dynamic** — every fetch can enqueue more
fetches. Try the naive port and you hit a wall:

- **Fixed pool + a job queue channel deadlocks.** A worker pulling from a queue
  wants to *push* the links it just found back onto that same queue. With a bounded
  queue, all workers block trying to push while no one is left to pull → deadlock.
  With an unbounded queue you dodge the deadlock but now **nobody knows when to
  `close`** the queue — "queue is momentarily empty" is *not* "no more work is
  coming," because a busy worker is about to push more.

The fix is to stop thinking in "queue length" and track **in-flight work**
directly:

- a **`sync.WaitGroup`** counts goroutines that exist, incremented
  *before* each spawn (`wg.Add(1)` then `go`) and decremented on return
  (`defer wg.Done()`). The count is zero **iff** no work is running *and* none is
  queued to start — the exact termination condition.
- a **semaphore** (`chan struct{}`) decouples "how many goroutines exist" from
  "how many are hitting the network at once," so we can spawn freely without
  opening 10 000 sockets.

`wg.Add` **before** the `go` (never inside the new goroutine) is load-bearing: it
guarantees the closer can't observe a zero count in the window between "parent
found a link" and "child actually started." Get that ordering wrong and the crawl
either closes `results` early (lost pages) or hangs.

### Concurrency vs. parallelism (and the two knobs)

You asked goroutines to do this, but you never created an OS **thread**. Go runs
an **M:N scheduler**: millions of goroutines (each ~2 KB) multiplexed onto a few
OS threads. `GOMAXPROCS` (logged at startup) caps how many run Go code *in
parallel*. The crawler is **concurrent** regardless; it becomes **parallel** when
there's more than one CPU.

The two request knobs control *different* limits, and it's worth not conflating
them:

- **`concurrency`** is a *structural* bound — the semaphore size, i.e. how many
  fetches may be **in flight** simultaneously. Raise it and more sockets are open
  at once.
- **`rate_per_sec`** is a *temporal* bound — how many fetches may **start per
  second**, via the token bucket. Raise it and fetches are allowed to fire more
  often.

You can be highly concurrent but slow-rate (8 in flight, but only 2 new ones
allowed per second), or serial but fast (1 in flight, 500/sec). They compose.

### Why each primitive is here

- **`chan struct{}` semaphore** — `struct{}` is a zero-byte type, so the channel's
  buffer is pure counting, no payload. Capacity = the concurrency ceiling.
- **`sync.WaitGroup`** — the only correct way to detect "a *dynamic* set of
  goroutines has fully drained." A counter you can `Add` to mid-flight.
- **`sync.Mutex` in `admit`** — check-and-set (is-it-new *and* is-there-budget,
  then mark) must be **atomic as a pair**, or two goroutines both crawl the same
  new URL. One lock, one decision.
- **`time.Ticker` token bucket** — stdlib rate limiting: a goroutine drips tokens
  into a buffered channel; the buffer size is the burst allowance.
- **`context`** — per-fetch deadlines *and* whole-crawl cancellation, tied to the
  HTTP request so a disconnected client stops the crawl.
- **`sync/atomic`** — `totalFetches`/`totalCrawls` are bumped from every goroutine;
  `Add`/`Load` are the lock-free fix for a lone shared counter.
- **`sync.RWMutex`** ([store](internal/repository/memory/crawl_repository.go)) —
  many HTTP reads (`RLock`) with exclusive saves (`Lock`). Clean under `-race`.

## Architecture

Same clean-architecture dependency rule as the rest of the track — arrows point
**inward**:

```
handler  ->  service  ->  domain  <-  repository
                          (core)
```

### Layout

```
crawler-hub-hard/
├── cmd/api/main.go                                  # wiring + graceful shutdown (goroutine + signal channel)
├── internal/
│   ├── config/config.go                             # env-driven config (concurrency, depth, pages, rate, site size)
│   ├── domain/crawl.go                              # PageResult/CrawlJob, repo interface, errors
│   ├── repository/memory/crawl_repository.go        # in-memory store guarded by sync.RWMutex
│   ├── service/
│   │   ├── crawler.go                               # ★ the engine (semaphore/WaitGroup/mutex-admit/context/atomic)
│   │   └── limiter.go                               # ticker token-bucket rate limiter
│   ├── handler/
│   │   ├── crawl_handler.go                         # controllers + request/response DTOs + validation/clamping
│   │   ├── site_handler.go                          # the built-in deterministic mini-site (cyclic graph)
│   │   └── response.go                              # JSON write helpers
│   └── router/router.go                             # routes + logging/recover middleware
├── Dockerfile
├── docker-compose.yml                               # single app service (no database)
├── .env
├── go.mod                                            # stdlib only — no external dependencies
├── progress.md
└── README.md
```

## Tech stack

- **Go** standard library only — `net/http`, `net/url`, `regexp`, `sync`,
  `sync/atomic`, `context`, `time`. No external modules, so `go.sum` doesn't
  exist and the build needs no network.
  - Link extraction uses `regexp` on `href="…"`. A production crawler would parse
    HTML with `golang.org/x/net/html`; the regex keeps dependencies at **zero**,
    which is the point of the exercise.
- Go 1.22+ method+pattern routing (`POST /crawls`, `GET /crawls/{id}`, `GET /site/{page}`).
- **Docker Compose** runs a single container (no Postgres).

## Run it

```bash
docker compose up --build
# then, in another terminal:
curl -s -X POST localhost:8080/crawls -d '{"max_depth":3,"concurrency":8}'
```

Tear down: `docker compose down`

### Run outside Docker

```bash
go run ./cmd/api
# or — strongly recommended for a concurrency project — under the race detector:
go run -race ./cmd/api
```

There are no dependencies to download; `go run` works immediately. Fire several
crawls at once under `-race` and the store, the dedup set, and the counters all
stay clean.

## Concepts this project teaches

- **Bounded parallelism over dynamic work** — the pattern a fixed worker pool
  can't express: work that spawns more work.
- **Counting semaphore** with a buffered `chan struct{}` — bound in-flight
  fetches independently of how many goroutines exist.
- **`sync.WaitGroup` for a dynamic goroutine set** — add-before-launch,
  done-on-return, and the `wg.Wait(); close(ch)` closer-goroutine idiom that
  makes the crawl terminate exactly when it's done.
- **`sync.Mutex` check-and-set** — a single-lock `admit()` that does dedup and
  budget enforcement atomically, so cycles and back-links can't cause re-crawls.
- **Token-bucket rate limiting** from stdlib — a `time.Ticker` refilling a
  buffered channel; burst size = buffer capacity.
- **`context` cancellation + per-request timeouts** — `WithTimeout`,
  `NewRequestWithContext`, and `select { case <-ctx.Done() }` at every blocking
  point, tied to the HTTP request.
- **`sync/atomic`** for lock-free shared counters.
- **`sync.RWMutex`** for a concurrent-safe in-memory store (verified with `-race`).
- **URL handling** — `net/url` parsing and `ResolveReference` to turn relative
  links absolute, plus same-host scoping.
- **Graceful shutdown** — `ListenAndServe` on a goroutine, block on an
  `os/signal` channel, then `srv.Shutdown(ctx)`.
