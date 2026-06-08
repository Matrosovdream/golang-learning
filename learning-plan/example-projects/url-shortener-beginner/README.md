# URL Shortener — beginner

A small URL-shortening web service: paste a long URL, get a short code back;
visiting the code redirects you to the original and counts the click.

It is the **first project** in the example-projects track and establishes the
Go clean-architecture skeleton that every later project reuses.

---

## What you'll see

```bash
# 1. shorten a URL
curl -s -X POST localhost:8080/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/effective_go"}'
# -> {"code":"X1y2Z3","short_url":"http://localhost:8080/X1y2Z3","long_url":"https://go.dev/doc/effective_go"}

# 2. open the short link (302 redirect, records a click)
curl -i localhost:8080/X1y2Z3

# 3. read the stats
curl -s localhost:8080/api/stats/X1y2Z3
# -> {"code":"X1y2Z3","long_url":"...","clicks":1,"created_at":"2026-06-08T..."}
```

## Routes

| Method | Path                  | Purpose                                  |
|--------|-----------------------|------------------------------------------|
| POST   | `/shorten`            | Create a short code for a long URL (201) |
| GET    | `/{code}`             | Redirect to the long URL (302) + count   |
| GET    | `/api/stats/{code}`   | Return code, long URL, click count (200) |

## Tech stack

- **Go** standard library only for HTTP (`net/http`, Go 1.22+ method+pattern routing) — no web framework.
- **pgx v5** with raw SQL (no ORM) for Postgres access.
- **Postgres 16** as the store; the schema in `migrations/` is applied automatically on first boot.
- **Docker Compose** brings up the app + database together.

## Architecture

Clean architecture — the dependency rule points **inward**:

```
handler  ->  service  ->  domain  <-  repository
                           (core)
```

- `domain` defines the `Link` entity and the `LinkRepository` **interface**, and
  imports nothing from the other layers (no `net/http`, no `pgx`).
- `repository/postgres` **implements** that interface with raw SQL. Swap it for an
  in-memory store and nothing else changes.
- `service` holds the business rules (URL validation, code generation, collision
  retry) and depends only on the interface.
- `handler` is the only layer that knows about HTTP.
- `cmd/api/main.go` is the **only** place that wires the layers together.

### Layout

```
url-shortener-beginner/
├── cmd/api/main.go                                  # entrypoint + dependency wiring + graceful shutdown
├── internal/
│   ├── config/config.go                             # env-driven config with defaults
│   ├── domain/link.go                               # entity + repository interface + domain errors
│   ├── repository/postgres/link_repository.go       # raw-SQL implementation (pgx pool)
│   ├── service/link_service.go                      # business logic
│   ├── handler/
│   │   ├── link_handler.go                           # HTTP controllers
│   │   └── response.go                               # JSON write helpers
│   └── router/router.go                             # routes + logging/recover middleware
├── migrations/001_init.sql                          # links table (auto-applied by Postgres)
├── Dockerfile                                        # multi-stage build, static binary
├── docker-compose.yml                                # app + postgres, wired by env + healthcheck
├── .env                                              # POSTGRES_* + APP_PORT + BASE_URL
├── go.mod / go.sum
├── progress.md                                        # build checklist
└── README.md
```

## Run it

```bash
docker compose up --build
```

Compose starts Postgres first (gated on a healthcheck), mounts `migrations/`
into the database's init folder so the `links` table is created on first boot,
then starts the app with `DATABASE_URL` pointing at the `db` service.

Tear down (and drop the database volume for a clean slate):

```bash
docker compose down -v
```

### Run the app outside Docker

If you'd rather run only Postgres in Docker and the app on your host:

```bash
docker compose up -d db
go run ./cmd/api          # reads defaults from internal/config/config.go
```

## Concepts this project teaches

- Structuring a Go service with clean-architecture boundaries and the dependency rule.
- HTTP routing, JSON request/response handling, and status codes with the stdlib.
- Raw SQL with pgx: connection pooling, parameterized queries, `RETURNING`,
  detecting a unique-constraint violation (`23505`) and `pgx.ErrNoRows`.
- Middleware (request logging + panic recovery) via handler wrapping.
- Running an app against Postgres with Docker Compose, including healthcheck-gated
  startup and DB-ready retry on connect.
- Graceful shutdown on `SIGINT`/`SIGTERM`.
