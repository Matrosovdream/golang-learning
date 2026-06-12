# Poll Hub — beginner microservices

Three small HTTP services around one idea — create polls, cast votes, read
results — sharing **one Postgres** and started by **one `docker-compose.yml`**.
This is your first microservice split, kept beginner-sized: plain JSON over
HTTP, no gRPC, no message broker.

```
            create / close polls                cast a vote
 client ──────────────┐                ┌────────────────────── client
                      ▼                ▼
               ┌──────────────┐   ┌──────────────┐   ┌───────────────┐
               │ poll-service │◀──│ vote-service │   │ stats-service │◀── client
               │    :8081     │   │    :8082     │   │     :8083     │  (results)
               └──────┬───────┘   └──────┬───────┘   └───────┬───────┘
                      │ GET /polls/{id}  │                   │
                      │ "does it exist?  │                   │
                      │  still open?"    │                   │
                      ▼                  ▼                   ▼
               ┌─────────────────────────────────────────────────────┐
               │            Postgres — polls · options · votes       │
               └─────────────────────────────────────────────────────┘
```

## What each service does

| Service | Port | Owns | Routes |
|---|---|---|---|
| **poll-service** | 8081 | `polls`, `options` | `POST /polls` · `GET /polls` · `GET /polls/{id}` · `POST /polls/{id}/close` · `DELETE /polls/{id}` |
| **vote-service** | 8082 | `votes` | `POST /votes` · `GET /polls/{id}/votes` |
| **stats-service** | 8083 | nothing (read-only) | `GET /polls/{id}/results` · `GET /top?limit=N` |

Every service also answers `GET /healthz` with `204`.

**The microservice moment:** before inserting a vote, vote-service does *not*
read the `polls` table — it calls poll-service over HTTP
(`GET /polls/{id}`) and checks the poll exists, is still `open`, and really
contains the chosen option. Each service writes only the tables it owns;
stats-service is allowed to *read* across them because reporting is its job.

## How this differs from the other projects

The earlier projects use the 5-layer clean-arch skeleton
(handler → service → domain ← repository). This one shows the other idiomatic
style — **flat, small services** — plus a few tricks that keep three services
as cheap as one:

- **Flat packages** — each service is `main.go` + `handlers.go` + `store.go`
  in one `package main`. No interfaces, no layers: at this size they would be
  ceremony, not structure.
- **One `go.mod` for all services** — shared helpers live in
  `internal/{env,httpx,postgres}` (~100 lines total), each binary is
  `go build ./services/<name>`.
- **One parameterized Dockerfile** — `ARG SERVICE` picks which binary to
  build; Compose passes `args: { SERVICE: poll }` etc. Three images, one file.
- **Go 1.22+ stdlib routing** — `mux.HandleFunc("POST /polls/{id}/close", …)`
  and `r.PathValue("id")`. No router dependency at all.
- **Schema as `db/init.sql`** — mounted into the Postgres container's
  `/docker-entrypoint-initdb.d/`, runs once on first start. No migration code.
- **Constraints do the business rules** — `UNIQUE (poll_id, voter)` makes
  double-voting impossible even under concurrent requests (the service just
  translates error code `23505` into a 409), and the composite foreign key
  `(option_id, poll_id) → options (id, poll_id)` makes a vote for another
  poll's option unrepresentable.

## Run it

```bash
docker compose up --build
```

Four containers start: `db` (Postgres 17, host port **5433**), `poll`, `vote`,
`stats`. The schema plus one starter poll are created on first start; the data
survives restarts in the `pollhub-data` volume. Reset everything with
`docker compose down -v`.

## Try it

```bash
# The starter poll is already there
curl localhost:8081/polls/1

# Create your own
curl -X POST localhost:8081/polls \
  -d '{"question":"Tabs or spaces?","options":["Tabs","Spaces","Mixed"]}'

# Vote (option ids come from the response above)
curl -X POST localhost:8082/votes -d '{"poll_id":1,"option_id":1,"voter":"stan"}'
curl -X POST localhost:8082/votes -d '{"poll_id":1,"option_id":2,"voter":"ada"}'

# Same voter again -> 409 (the UNIQUE constraint at work)
curl -X POST localhost:8082/votes -d '{"poll_id":1,"option_id":3,"voter":"stan"}'

# Option from another poll -> 400 (vote-service asked poll-service)
curl -X POST localhost:8082/votes -d '{"poll_id":1,"option_id":999,"voter":"bob"}'

# Results with counts and percentages
curl localhost:8083/polls/1/results

# Close the poll, then voting -> 409 "poll is closed"
curl -X POST localhost:8081/polls/1/close
curl -X POST localhost:8082/votes -d '{"poll_id":1,"option_id":1,"voter":"eve"}'

# Most-voted polls
curl "localhost:8083/top?limit=3"
```

## Run without Docker (optional)

Each service has localhost defaults, so with only the database in Docker:

```bash
docker compose up -d db        # Postgres on localhost:5433
go run ./services/poll         # :8081
go run ./services/vote         # :8082  (new terminal)
go run ./services/stats        # :8083  (new terminal)
```

## Project layout

```
poll-hub-microservices-beginner/
├── docker-compose.yml      # db + 3 services, one network
├── Dockerfile              # shared; ARG SERVICE picks the binary
├── db/init.sql             # schema + constraints + starter poll
├── go.mod / go.sum         # ONE module for everything
├── internal/
│   ├── env/env.go          # Get(key, fallback)
│   ├── httpx/httpx.go      # JSON/Error/Decode, logging, graceful Serve
│   └── postgres/postgres.go# pgx pool with connect-retry
└── services/
    ├── poll/               # main.go · handlers.go · store.go
    ├── vote/               # + pollclient.go (the HTTP call to poll-service)
    └── stats/              # read-only SQL aggregation
```

## Things worth noticing while typing it

1. **`pollclient.go`** is the heart of the lesson: a typed client around one
   `GET`, with a timeout, decoding into a *small* struct that ignores fields
   it doesn't need. A 404 becomes `errPollNotFound`; an unreachable
   poll-service becomes a **502** from vote-service — failures of a dependency
   are part of your API.
2. **`httpx.Serve`** is the whole graceful-shutdown story:
   `signal.NotifyContext` + `srv.Shutdown` with a deadline, shared by all
   three binaries.
3. **Check-then-insert isn't atomic.** Between vote-service's check and the
   `INSERT`, the poll could close or another request could vote — that's why
   the real rules live in the database constraints, and the HTTP check only
   produces friendlier errors. Race-proofing across services is an
   intermediate-track topic (transactions won't help across an HTTP call).
4. **`stats-service`** shows aggregation where it belongs: `COUNT` +
   `GROUP BY` + `LEFT JOIN` (so zero-vote options still appear) in SQL, only
   the percentage math in Go.
