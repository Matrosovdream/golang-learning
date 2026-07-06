# gRPC Echo — beginner (your first two-service gRPC app)

The smallest possible microservice setup that still teaches the real thing: a
public **REST gateway** that forwards each request to a private **echo** service
over **gRPC**, with **structured logging** and a **request-id that ties the two
services' logs together**. No database — the focus is gRPC wiring, Docker
Compose, and inter-service logging.

It is the first of three gRPC projects. Its siblings add databases + a service
mesh (`grpc-orders-intermediate`) and full Prometheus + Grafana observability
(`grpc-observability-hard`).

---

## Architecture

```
                        x-request-id minted here
   client ── REST ────►  ┌───────────┐  ── gRPC ──►  ┌───────────┐
   curl :8080            │  gateway  │   (metadata   │   echo    │
                         │  (:8080)  │    carries    │  (:50051) │
                         └───────────┘    the id)    └───────────┘
        one request-id appears in BOTH services' JSON logs
```

- **gateway** — the only published container. Speaks REST outward, gRPC inward.
  Mints an `x-request-id` (or reuses an inbound one), and maps gRPC status codes
  to HTTP (echo down → `503`, timeout → `504`).
- **echo** — a private gRPC server. Echoes the message with the hostname that
  served it and a timestamp. Never exposed to the host.

Both install the same two interceptors from `pkg/obs`:
`RequestIDUnaryServer` (correlation id) → `LoggingUnaryServer` (one JSON line per
RPC). The gateway's gRPC **client** adds `RequestIDUnaryClient`, which forwards
the id on the wire.

## Gateway REST API

| Method | Path            | Does                                        |
|--------|-----------------|---------------------------------------------|
| GET    | `/echo/{msg}`   | forwards `msg` to `echo.Echo` over gRPC     |
| GET    | `/healthz`      | liveness (`ok`)                             |

## Run it

```bash
docker compose up --build
```

Two containers start; only the gateway is published, on `:8080`.

```bash
curl -s localhost:8080/echo/hello | jq
# {
#   "message": "hello",
#   "served_by": "a1b2c3d4e5f6",   # echo container's hostname
#   "received_at": "2026-07-06T04:15:10.12Z",
#   "request_id": "9f3a1c7b2e0d4a55"
# }

# bring your own correlation id and watch it flow through both services' logs:
curl -s -H 'X-Request-Id: trace-me-123' localhost:8080/echo/world | jq
```

Now look at the logs — the **same `request_id`** appears in the gateway line and
the echo line for one request:

```bash
docker compose logs gateway echo | grep trace-me-123
# gateway | {"level":"INFO","msg":"http_request","service":"gateway",...,"request_id":"trace-me-123"}
# echo    | {"level":"INFO","msg":"grpc_request","service":"echo",...,"request_id":"trace-me-123"}
```

See `503` mapping when the peer is down: `docker compose stop echo`, then curl
`/echo/hi` — the gateway returns `{"code":"Unavailable"}` with HTTP 503.

Tear down: `docker compose down`.

## Tech stack

- **Go** monorepo (single module `grpcecho`, two binaries under `services/`).
- **gRPC + Protocol Buffers** between gateway and echo; REST at the edge.
- **`log/slog`** JSON logging; a request-id propagated via gRPC **metadata**.
- **Docker Compose** runs both services on one network (no DB).

## Regenerating the gRPC stubs

The generated `*.pb.go` are committed, so `docker compose up` needs no `protoc`.
To regenerate after editing the `.proto`:

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/echo/v1/echo.proto
```

## Layout

```
grpc-echo-beginner/
├── go.mod / go.sum
├── proto/echo/v1/echo.proto + *.pb.go     # the contract + generated stubs
├── pkg/obs/obs.go                          # logger + request-id/logging interceptors
├── services/
│   ├── echo/     main.go + Dockerfile      # the gRPC server
│   └── gateway/  main.go + Dockerfile      # REST → gRPC, code→HTTP mapping
├── docker-compose.yml + .env + .dockerignore
└── README.md / progress.md
```

## Concepts this project teaches

- Defining a `.proto`, generating stubs, and implementing a gRPC **server** and **client**.
- The **API-gateway** pattern: REST at the edge, gRPC inside; mapping gRPC codes → HTTP.
- **Structured logging** with `slog`, one line per RPC via a server interceptor.
- A **request-id / correlation-id** propagated across services through gRPC **metadata**
  (the single most useful observability habit in a microservice system).
- **Service discovery via Compose DNS** (`echo:50051`) and graceful shutdown.

## Next

- `grpc-orders-intermediate` — a real 3-service mesh with Postgres per service and
  the request-id flowing across all of them.
- `grpc-observability-hard` — the same idea plus **Prometheus metrics** and a
  **Grafana** dashboard.
