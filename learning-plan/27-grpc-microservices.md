# 27 — gRPC & Microservices (with logging & metrics)

## Goals
- Define service contracts in **Protocol Buffers** and generate Go stubs.
- Build gRPC **servers and clients**: unary and the three streaming modes.
- Split a system into services that call each other over gRPC, fronted by a REST **API gateway**.
- Add cross-cutting concerns with **interceptors**: structured **logging**, a propagated **request-id**, and **Prometheus metrics** — the "logging & metrics between services" part.
- Run the whole mesh with **Docker Compose**, scraped by **Prometheus** and visualised in **Grafana**.

## Concepts

### Why gRPC (vs REST/JSON) for service-to-service
- **Contract-first.** A `.proto` file *is* the API. Both sides generate typed code from it, so a field rename is a compile error, not a 3am incident.
- **Binary + HTTP/2.** Protobuf is a compact binary wire format; HTTP/2 gives one long-lived multiplexed connection (many concurrent calls, no head-of-line blocking) plus **streaming** in both directions.
- **Codes, not just 200/500.** gRPC has a rich status-code set (`NotFound`, `AlreadyExists`, `Unavailable`, `DeadlineExceeded`, …) that maps cleanly onto both errors *and* HTTP at the edge.
- **When to still use REST:** the *public* edge (browsers, third parties, curl). Common shape: **REST at the gateway, gRPC between internal services** — exactly what the projects do.

### Protocol Buffers — the contract
```proto
syntax = "proto3";
package greet.v1;
option go_package = "echo/proto/greet/v1;greetv1"; // importpath ; package alias

service GreeterService {
  rpc SayHello(HelloRequest) returns (HelloReply);          // unary
  rpc SayHelloStream(HelloRequest) returns (stream HelloReply); // server-streaming
}

message HelloRequest { string name = 1; }   // 1 = field number (the wire identity)
message HelloReply   { string message = 1; }
```
- **Field numbers are the contract**, not the names. Never reuse or renumber a field; add new ones. That is how you evolve an API without breaking old clients.
- **proto3 defaults:** every scalar has a zero value on the wire — an unset `int32` and `0` are indistinguishable unless you mark the field `optional` (which adds explicit presence).
- Generate the Go code:
  ```bash
  protoc --go_out=. --go_opt=paths=source_relative \
         --go-grpc_out=. --go-grpc_opt=paths=source_relative \
         proto/greet/v1/greet.proto
  ```
  This emits `greet.pb.go` (messages) and `greet_grpc.pb.go` (client interface + server interface + registrar). **Commit the generated files** so `docker build` doesn't need `protoc`.

### A server and a client
```go
// server: implement the generated ServerInterface, register, serve
type greeter struct{ greetv1.UnimplementedGreeterServiceServer } // forward-compat embed
func (greeter) SayHello(ctx context.Context, r *greetv1.HelloRequest) (*greetv1.HelloReply, error) {
    return &greetv1.HelloReply{Message: "hello " + r.GetName()}, nil
}
lis, _ := net.Listen("tcp", ":9000")
srv := grpc.NewServer()
greetv1.RegisterGreeterServiceServer(srv, greeter{})
srv.Serve(lis)
```
```go
// client: dial, wrap the conn in a generated client, call like a local method
conn, _ := grpc.NewClient("localhost:9000", grpc.WithTransportCredentials(insecure.NewCredentials()))
defer conn.Close()
client := greetv1.NewGreeterServiceClient(conn)
reply, err := client.SayHello(ctx, &greetv1.HelloRequest{Name: "Stan"})
```
- **Always pass a `context.Context`** with a deadline — `context.WithTimeout` — so a hung peer can't hang you. On timeout the call returns `codes.DeadlineExceeded`.
- Use `grpc.NewClient` (lazy, name-resolver aware). The old `grpc.Dial`/`DialContext` are deprecated.

### Streaming (the HTTP/2 payoff)
| Kind | Signature shape | Use for |
|------|-----------------|---------|
| **Unary** | `rpc F(Req) returns (Res)` | ordinary request/response |
| **Server-streaming** | `returns (stream Res)` | one request, many replies (feeds, tailing) |
| **Client-streaming** | `(stream Req) returns (Res)` | upload / batch, one summary reply |
| **Bidirectional** | `(stream Req) returns (stream Res)` | chat, live sync |
Each side calls `stream.Send(...)` / `stream.Recv()`; `Recv` returns `io.EOF` when the other end is done.

### Errors & status codes
```go
return nil, status.Errorf(codes.NotFound, "user %d not found", id)
// caller side:
st, _ := status.FromError(err)   // st.Code(), st.Message()
if st.Code() == codes.NotFound { ... }
```
Map codes to HTTP at the gateway: `NotFound→404`, `AlreadyExists→409`, `InvalidArgument→400`, `Unavailable→503` (peer down). Never leak a raw Go error string across a service boundary — wrap it in a status with a code.

### Metadata — the request's "headers"
gRPC carries key/value **metadata** alongside each call (like HTTP headers). This is how a **request-id / correlation-id** rides from the gateway through every downstream hop:
```go
// client side: attach outgoing metadata
ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", reqID)
// server side: read incoming metadata
md, _ := metadata.FromIncomingContext(ctx)
reqID := first(md.Get("x-request-id"))
```

### Interceptors — middleware for gRPC (where logging & metrics live)
An **interceptor** wraps every RPC so you write cross-cutting logic once instead of in every handler. There are unary and stream variants, on both server and client:
```go
func LoggingUnary(logger *slog.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler) (any, error) {
        start := time.Now()
        resp, err := handler(ctx, req)               // <-- call the actual RPC
        logger.Info("grpc",
            "method", info.FullMethod,
            "code", status.Code(err).String(),
            "ms", time.Since(start).Milliseconds(),
            "request_id", requestIDFrom(ctx))
        return resp, err
    }
}
srv := grpc.NewServer(grpc.ChainUnaryInterceptor(RequestIDUnary(), LoggingUnary(log), MetricsUnary(reg)))
```
- **Logging interceptor** → one structured line per RPC (method, code, latency, request-id).
- **Request-id interceptor** → read `x-request-id` from incoming metadata (or mint one), stash it in `context`; a matching *client* interceptor re-attaches it to outgoing calls so the id survives every hop.
- **Metrics interceptor** → increment a Prometheus counter and observe a latency histogram, labelled by `method` and `code`.

### Observability: structured logs + metrics
- **Logging:** use `log/slog` with a JSON handler. Always include `request_id` so you can `grep` one user request across *all* services' logs. Log once per RPC in the interceptor, not scattered through handlers.
- **Metrics:** expose Prometheus metrics on a plain HTTP `/metrics` endpoint (separate port from gRPC). The three you almost always want:
  - `grpc_server_handled_total{method,code}` — a **counter** (rate & error ratio).
  - `grpc_server_handling_seconds{method}` — a **histogram** (latency percentiles).
  - `grpc_server_in_flight` — a **gauge** (concurrency).
- **Prometheus** scrapes each service's `/metrics`; **Grafana** graphs `rate(...)`, error ratio, and `histogram_quantile(0.95, ...)`. This is the "some other metric" you asked for.

### Microservices shape (applied clean architecture)
- **Database-per-service:** each service owns its store; no cross-service SQL joins or foreign keys. `orders` stores a *snapshot* of a product's name/price, not a FK into catalog.
- **API gateway:** the single public REST door; it fans out to internal gRPC services and maps codes → HTTP.
- **Service discovery via Compose DNS:** a service dials `users:9001` — Docker's network resolves the name. No hardcoded IPs.
- **Graceful shutdown:** on SIGTERM call `srv.GracefulStop()` so in-flight RPCs finish before the process exits.
- **The distributed-transaction trap:** if service A mutates its DB and then a call to B fails, you have no rollback across the boundary. Fixes: a **saga** with compensating actions, or move to **events** (see project `event-shop-microservices-advanced`).

## Exercises
1. Write a `greet.proto` with `GreeterService.SayHello` and generate the stubs with `protoc`. Inspect the two generated files — find the client interface, the server interface, and the `Register…Server` function.
2. Implement the server (embed `Unimplemented…Server`) and a client; call it end-to-end with a `context.WithTimeout`. Then stop the server and observe the client get `codes.Unavailable`.
3. Add a **server-streaming** RPC that streams N replies; loop with `stream.Recv()` until `io.EOF` on the client.
4. Return `status.Errorf(codes.NotFound, …)` from the server; on the client, use `status.FromError` and branch on `st.Code()`.
5. Write a **unary server interceptor** that logs `method`, `code`, and latency with `slog` (JSON handler). Chain it with `grpc.ChainUnaryInterceptor`.
6. Add a **request-id**: a server interceptor that reads `x-request-id` from metadata (mint one if absent) and puts it in `context`; a client interceptor that re-attaches it outward. Prove one id appears in two services' logs for a single request.
7. Add a **metrics interceptor** (a counter + a histogram labelled by method/code) and expose `/metrics`. Scrape it with a local Prometheus, then chart p95 latency and error rate in Grafana.
8. Build a two-service app behind a REST gateway; map each gRPC code to an HTTP status, including `503` when a downstream service is stopped.

## Best Practices & Pitfalls
- **Field numbers are forever.** Add fields, never renumber or reuse a number. Treat the `.proto` as an append-only contract.
- **Commit the generated `*.pb.go`.** CI and `docker build` shouldn't need `protoc`. Regenerate only when the `.proto` changes.
- **Every call gets a deadline.** A client without `context.WithTimeout` will wait forever on a wedged server. Deadlines propagate to the server automatically.
- **Cross-cutting concerns go in interceptors**, not handlers. Logging, auth, request-id, metrics, recovery — write once, chain everywhere. Order matters (request-id *before* logging so the log line has the id).
- **One structured log line per RPC** with a `request_id`, not `fmt.Println` scattered in business logic. Logs without a correlation id are nearly useless in a mesh.
- **Metrics on a separate HTTP port** from gRPC; label by `method` and `code` but **never by unbounded values** (user id, order id) — that explodes cardinality and kills Prometheus.
- **Pitfall — leaking errors:** returning a bare `err` gives the caller `codes.Unknown`. Always wrap in a `status` with a meaningful code.
- **Pitfall — the typed-nil / Unimplemented gap:** forgetting to embed `Unimplemented…Server` means adding an RPC later breaks the build for every server — embed it for forward compatibility.
- **Pitfall — no distributed transaction:** a successful call to B after A committed can't be rolled back if the next step fails. Design for it (saga / events), don't pretend the boundary is a DB transaction.
- **Pitfall — chatty fan-out:** N sequential gRPC calls per request add up. Batch where the contract allows, and set per-call deadlines so one slow peer doesn't sink the request.

## Checklist
- [ ] I can write a `.proto`, generate stubs, and explain what each generated file contains.
- [ ] I can implement a gRPC server and client, with deadlines, and handle `status` codes.
- [ ] I can write server- and client-streaming RPCs and read to `io.EOF`.
- [ ] I can write unary interceptors and chain them (request-id → logging → metrics).
- [ ] I propagate a request-id across services via metadata and see it in every service's logs.
- [ ] I expose Prometheus metrics, scrape them, and chart latency/error-rate in Grafana.
- [ ] I can front gRPC services with a REST gateway and map codes → HTTP (incl. `503`).
- [ ] I understand database-per-service and why there's no cross-service transaction.

## Resources
- gRPC Go quickstart & basics: https://grpc.io/docs/languages/go/quickstart/ · https://grpc.io/docs/languages/go/basics/
- Protocol Buffers (proto3) language guide: https://protobuf.dev/programming-guides/proto3/
- gRPC status codes: https://grpc.io/docs/guides/status-codes/
- `log/slog` (structured logging, Go 1.21+): https://pkg.go.dev/log/slog
- Prometheus Go client: https://github.com/prometheus/client_golang · Query basics: https://prometheus.io/docs/prometheus/latest/querying/basics/
- Companion example-projects: `grpc-echo-beginner`, `grpc-orders-intermediate`, `grpc-observability-hard`.
