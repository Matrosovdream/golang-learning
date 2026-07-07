# Step 27 — gRPC & Microservices · Examples

A library of **24 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype into one scratch module** and run with `go run .`.
Most are self-contained: they spin up a gRPC **server and client in the same process** over an
in-memory `bufconn` pipe, so one `go run .` shows you a full round trip — no second terminal, no
Docker.

## One-time setup (do this first)

gRPC code is generated from a `.proto`, so unlike the other lessons you set up a tiny module once,
then reuse it for every example:

```bash
mkdir -p /tmp/grpc-ex/greetpb && cd /tmp/grpc-ex
go mod init scratch
```

Create `greetpb/greet.proto` with the contract every example shares:

```proto
syntax = "proto3";
package greet.v1;
option go_package = "scratch/greetpb;greetpb";

service Greeter {
  rpc SayHello(HelloRequest) returns (HelloReply);              // unary
  rpc SayManyHellos(HelloRequest) returns (stream HelloReply);  // server-streaming
  rpc CollectNames(stream HelloRequest) returns (HelloReply);   // client-streaming
  rpc Chat(stream HelloRequest) returns (stream HelloReply);    // bidirectional
}

message HelloRequest { string name = 1; int32 count = 2; }
message HelloReply   { string message = 1; }
```

Generate the stubs and pin the same deps these examples were verified with:

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       greetpb/greet.proto
go get google.golang.org/grpc@v1.81.1
go get github.com/prometheus/client_golang@v1.23.2   # only needed for examples 20 & 24
go mod tidy
```

You now have `greetpb/greet.pb.go` + `greetpb/greet_grpc.pb.go`. For each example, **put the code in
`main.go`** (replacing the last one) and run it:

```bash
go run .
```

> Need `protoc`? `brew install protobuf`, then the two plugins:
> `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` and
> `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` (ensure `$(go env GOPATH)/bin` is on your `PATH`).

Every example was compiled, `go vet`-ed, and run before being added; the **Output** under each one is
real stdout. A couple of examples use two terminals (a real TCP server) — they say so.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–24 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — proto & the basics
- [1. Your first .proto and what protoc generates](1-easy.md#1-your-first-proto-and-what-protoc-generates)
- [2. Hello gRPC in one process (bufconn)](1-easy.md#2-hello-grpc-in-one-process-bufconn)
- [3. The wire format: proto.Marshal vs JSON](1-easy.md#3-the-wire-format-protomarshal-vs-json)
- [4. Every call gets a deadline](1-easy.md#4-every-call-gets-a-deadline)
- [5. Status codes, not error strings](1-easy.md#5-status-codes-not-error-strings)
- [6. A real TCP server (two terminals) and codes.Unavailable](1-easy.md#6-a-real-tcp-server-two-terminals-and-codesunavailable)
- [7. proto3 field presence: zero value vs optional](1-easy.md#7-proto3-field-presence-zero-value-vs-optional)
- [8. Server-streaming: many replies, read to io.EOF](1-easy.md#8-server-streaming-many-replies-read-to-ioeof)

### 🟡 [Medium](2-medium.md) — streaming, metadata, interceptors
- [9. Client-streaming: one summary reply](2-medium.md#9-client-streaming-one-summary-reply)
- [10. Bidirectional streaming: Chat](2-medium.md#10-bidirectional-streaming-chat)
- [11. Metadata: the request's headers](2-medium.md#11-metadata-the-requests-headers)
- [12. A unary server interceptor that logs](2-medium.md#12-a-unary-server-interceptor-that-logs)
- [13. Chaining interceptors](2-medium.md#13-chaining-interceptors)
- [14. A client interceptor that injects a request-id](2-medium.md#14-a-client-interceptor-that-injects-a-request-id)
- [15. Propagate a request-id across a hop](2-medium.md#15-propagate-a-request-id-across-a-hop)
- [16. A recovery interceptor: panic → codes.Internal](2-medium.md#16-a-recovery-interceptor-panic--codesinternal)
- [17. Time every RPC (the seed of a metric)](2-medium.md#17-time-every-rpc-the-seed-of-a-metric)

### 🔴 [Hard](3-hard.md) — observability & production patterns
- [18. A stream interceptor (wrapping ServerStream)](3-hard.md#18-a-stream-interceptor-wrapping-serverstream)
- [19. Structured logging with slog + request_id](3-hard.md#19-structured-logging-with-slog--request_id)
- [20. Prometheus metrics interceptor + /metrics](3-hard.md#20-prometheus-metrics-interceptor--metrics)
- [21. Health checking (grpc_health_v1)](3-hard.md#21-health-checking-grpc_health_v1)
- [22. Server reflection (talk to it with grpcurl)](3-hard.md#22-server-reflection-talk-to-it-with-grpcurl)
- [23. Client retry with backoff on Unavailable](3-hard.md#23-client-retry-with-backoff-on-unavailable)
- [24. Capstone: two services, logging + metrics wired](3-hard.md#24-capstone-two-services-logging--metrics-wired)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
