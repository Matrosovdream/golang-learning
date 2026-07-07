# Step 27 — gRPC & Microservices · 🔴 Hard — examples **18–24**

The production layer: stream interceptors, **structured logging**, **Prometheus metrics**, health checks,
reflection, retries — ending in a capstone that wires logging + metrics + request-id across two services.
Reuse the [scratch module](README.md#one-time-setup-do-this-first). Examples 20 & 24 need
`github.com/prometheus/client_golang` (in the setup).

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 18. A stream interceptor (wrapping ServerStream)

`🔴 hard` · *interceptors / streaming*

Unary interceptors get the request value; **stream** interceptors get a `grpc.ServerStream` you can wrap to
observe every `Send`/`Recv`. Embed `grpc.ServerStream` in your own type, override the method you care about,
and pass the wrapper down to the handler.

```go
package main

import (
	"context"
	"io"
	"log"
	"net"
	"time"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayManyHellos(r *greetpb.HelloRequest, stream greetpb.Greeter_SayManyHellosServer) error {
	for i := int32(0); i < r.GetCount(); i++ {
		stream.Send(&greetpb.HelloReply{Message: "hi"})
	}
	return nil
}

// wrap the stream to count messages sent
type countingStream struct {
	grpc.ServerStream // embed: inherit all the interface methods
	sent int
}

func (c *countingStream) SendMsg(m any) error {
	c.sent++
	return c.ServerStream.SendMsg(m) // delegate to the real stream
}

func streamLogger(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	cs := &countingStream{ServerStream: ss}
	err := handler(srv, cs) // hand the wrapper to the RPC
	log.Printf("stream method=%s sent=%d code=%s dur=%s", info.FullMethod, cs.sent, status.Code(err), time.Since(start))
	return err
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainStreamInterceptor(streamLogger))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()

	stream, _ := greetpb.NewGreeterClient(conn).SayManyHellos(context.Background(), &greetpb.HelloRequest{Count: 3})
	for {
		if _, err := stream.Recv(); err == io.EOF {
			break
		}
	}
	time.Sleep(10 * time.Millisecond) // let the server-side log flush
}
```

**Output:**

```
2026/07/06 11:44:24 stream method=/greet.v1.Greeter/SayManyHellos sent=3 code=OK dur=109.791µs
```

> `grpc.UnaryInterceptor` and `grpc.StreamInterceptor` are separate — a unary interceptor does **not** see
> streaming RPCs. In a real server you install both.

---

## 19. Structured logging with slog + request_id

`🔴 hard` · *observability*

`log.Printf` is unsearchable across a fleet. Use `log/slog` with a JSON handler and always include a
`request_id` field — then you can filter every service's logs for one request. Log **once per RPC** in the
interceptor, not scattered in handlers.

```go
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"time"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	if r.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name required")
	}
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func ridOf(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-request-id"); len(v) > 0 {
			return v[0]
		}
	}
	return "none"
}

func slogUnary(l *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		l.Info("grpc",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"ms", time.Since(start).Milliseconds(),
			"request_id", ridOf(ctx))
		return resp, err
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(slogUnary(logger)))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-request-id", "req-42")
	client.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})
	client.SayHello(ctx, &greetpb.HelloRequest{Name: ""})
}
```

**Output:**

```json
{"time":"2026-07-06T11:38:05.5+07:00","level":"INFO","msg":"grpc","method":"/greet.v1.Greeter/SayHello","code":"OK","ms":0,"request_id":"req-42"}
{"time":"2026-07-06T11:38:05.5+07:00","level":"INFO","msg":"grpc","method":"/greet.v1.Greeter/SayHello","code":"InvalidArgument","ms":0,"request_id":"req-42"}
```

---

## 20. Prometheus metrics interceptor + /metrics

`🔴 hard` · *observability* · needs `prometheus/client_golang`

The metrics you almost always want per RPC: a **counter** of handled calls by `method` + `code` (rate &
error ratio) and a **histogram** of latency by `method` (percentiles). Register them, increment in the
interceptor, and expose `/metrics` for Prometheus to scrape.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	greetpb "scratch/greetpb"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	if r.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name required")
	}
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func metricsUnary(reg *prometheus.Registry) grpc.UnaryServerInterceptor {
	handled := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total", Help: "RPCs handled, by method and code.",
	}, []string{"method", "code"})
	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "grpc_server_handling_seconds", Help: "RPC latency in seconds.", Buckets: prometheus.DefBuckets,
	}, []string{"method"})
	reg.MustRegister(handled, latency) // panics on a duplicate registration — good

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		latency.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		handled.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

func main() {
	reg := prometheus.NewRegistry() // your own registry (not the global default)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(metricsUnary(reg)))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Ann"})
	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: ""}) // one error

	// this is what Prometheus scrapes; in a real service it's its own :2112/metrics HTTP server
	ts := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer ts.Close()
	res, _ := http.Get(ts.URL)
	body, _ := io.ReadAll(res.Body)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "grpc_server_handled_total") {
			fmt.Println(line)
		}
	}
}
```

**Output:**

```
grpc_server_handled_total{code="InvalidArgument",method="/greet.v1.Greeter/SayHello"} 1
grpc_server_handled_total{code="OK",method="/greet.v1.Greeter/SayHello"} 2
```

> **Never label by unbounded values** (user id, order id) — each distinct value is a new time series, and
> that "cardinality explosion" will take down Prometheus. Labels are for small, bounded sets: method, code,
> service. In Grafana you then chart `rate(grpc_server_handled_total[1m])` and
> `histogram_quantile(0.95, rate(grpc_server_handling_seconds_bucket[5m]))`.

---

## 21. Health checking (grpc_health_v1)

`🔴 hard` · *ops*

gRPC has a standard health service. Register `health.NewServer()`, set each service's status, and any client
(a load balancer, a Compose `healthcheck`, Kubernetes) can `Check` it — no custom endpoint needed.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, greetpb.UnimplementedGreeterServer{})

	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	// "" = overall server health; you can also set per-service names
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("greet.v1.Greeter", healthpb.HealthCheckResponse_SERVING)

	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()

	hc := healthpb.NewHealthClient(conn)
	resp, err := hc.Check(context.Background(), &healthpb.HealthCheckRequest{Service: "greet.v1.Greeter"})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("greet.v1.Greeter health = %s", resp.GetStatus())
}
```

**Output:**

```
2026/07/06 11:44:24 greet.v1.Greeter health = SERVING
```

> In Docker Compose you can gate `depends_on` on this with the `grpc_health_probe` binary as the container's
> `healthcheck`, so a service only starts once its dependencies report `SERVING`.

---

## 22. Server reflection (talk to it with grpcurl)

`🔴 hard` · *tooling*

Without a `.proto` on hand, how do you poke a running gRPC server? **Reflection** lets tools like `grpcurl`
discover its services and message shapes at runtime. One line registers it.

**Steps:**

1. Add reflection and run a real TCP server (reflection needs a client tool, so use a port, not bufconn):

```go
package main

import (
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	lis, _ := net.Listen("tcp", ":9000")
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, greetpb.UnimplementedGreeterServer{})

	reflection.Register(srv) // <-- one line; now grpcurl can introspect it

	log.Println("listening on :9000 with reflection")
	log.Fatal(srv.Serve(lis))
}
```

2. In another terminal (install `grpcurl` with `brew install grpcurl` or `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`):

```bash
grpcurl -plaintext localhost:9000 list                     # discover services
grpcurl -plaintext localhost:9000 describe greet.v1.Greeter
grpcurl -plaintext -d '{"name":"Stan"}' localhost:9000 greet.v1.Greeter/SayHello
```

**Output** (`list`):

```
greet.v1.Greeter
grpc.reflection.v1.ServerReflection
grpc.reflection.v1alpha.ServerReflection
```

> Turn reflection off (or gate it) in production if you don't want your API surface discoverable. It's
> invaluable in dev and staging.

---

## 23. Client retry with backoff on Unavailable

`🔴 hard` · *resilience*

A transient `Unavailable` (peer restarting, brief network blip) shouldn't fail the whole request. Retry it —
but only on retryable codes (never blindly retry a non-idempotent call), with **exponential backoff**, and
respect the context deadline.

```go
package main

import (
	"context"
	"log"
	"time"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func withRetry(ctx context.Context, attempts int, f func(context.Context) error) error {
	var err error
	backoff := 20 * time.Millisecond
	for i := 0; i < attempts; i++ {
		if err = f(ctx); status.Code(err) != codes.Unavailable {
			return err // OK, or a non-retryable error — stop
		}
		log.Printf("attempt %d: Unavailable, backing off %s", i+1, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff *= 2 // exponential
	}
	return err
}

func main() {
	// nothing is listening here, so every call is Unavailable
	conn, _ := grpc.NewClient("localhost:59999", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	err := withRetry(context.Background(), 3, func(ctx context.Context) error {
		c, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		_, e := client.SayHello(c, &greetpb.HelloRequest{Name: "Stan"})
		return e
	})
	log.Printf("gave up: code=%s", status.Code(err))
}
```

**Output:**

```
2026/07/06 11:44:54 attempt 1: Unavailable, backing off 20ms
2026/07/06 11:44:54 attempt 2: Unavailable, backing off 40ms
2026/07/06 11:44:54 attempt 3: Unavailable, backing off 80ms
2026/07/06 11:44:54 gave up: code=Unavailable
```

> gRPC can also do this for you declaratively via a **service config** retry policy
> (`grpc.WithDefaultServiceConfig(...)` with `methodConfig.retryPolicy`) — same idea, no hand-rolled loop.

---

## 24. Capstone: two services, logging + metrics wired

`🔴 hard` · *observability capstone*

Everything at once — a `frontend` that fans out to a `backend`, with three interceptors chained on each
(request-id → slog → Prometheus). One `curl`-equivalent produces: the **same `request_id` in both services'
JSON logs**, and **per-service Prometheus counters**. This is the miniature of `grpc-observability-hard`.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	greetpb "scratch/greetpb"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type ridKey struct{}

func ridFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ridKey{}).(string); ok {
		return v
	}
	return "none"
}

// interceptor 1: request-id in <- metadata, into context
func serverRID(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	id := "gen"
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-request-id"); len(v) > 0 {
			id = v[0]
		}
	}
	return handler(context.WithValue(ctx, ridKey{}, id), req)
}

// client interceptor: request-id -> outgoing metadata
func clientRID(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.AppendToOutgoingContext(ctx, "x-request-id", ridFrom(ctx)), method, req, reply, cc, opts...)
}

// interceptor 2: structured log per RPC
func slogUnary(l *slog.Logger, svc string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		l.Info("grpc", "service", svc, "method", info.FullMethod,
			"code", status.Code(err).String(), "ms", time.Since(start).Milliseconds(), "request_id", ridFrom(ctx))
		return resp, err
	}
}

// interceptor 3: Prometheus counter per RPC
func metricsUnary(reg *prometheus.Registry, svc string) grpc.UnaryServerInterceptor {
	handled := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total", Help: "RPCs by method and code.",
		ConstLabels: prometheus.Labels{"service": svc},
	}, []string{"method", "code"})
	reg.MustRegister(handled)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		handled.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

type backend struct{ greetpb.UnimplementedGreeterServer }

func (backend) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "hi " + r.GetName()}, nil
}

type frontend struct {
	greetpb.UnimplementedGreeterServer
	backend greetpb.GreeterClient
}

func (f frontend) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return f.backend.SayHello(ctx, r)
}

func dial(lis *bufconn.Listener, extra ...grpc.DialOption) greetpb.GreeterClient {
	opts := append([]grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, extra...)
	conn, _ := grpc.NewClient("passthrough:///bufnet", opts...)
	return greetpb.NewGreeterClient(conn)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	reg := prometheus.NewRegistry()

	// backend + a client to it that re-attaches the request-id
	blis := bufconn.Listen(1 << 20)
	bsrv := grpc.NewServer(grpc.ChainUnaryInterceptor(serverRID, slogUnary(logger, "backend"), metricsUnary(reg, "backend")))
	greetpb.RegisterGreeterServer(bsrv, backend{})
	go bsrv.Serve(blis)
	toBackend := dial(blis, grpc.WithChainUnaryInterceptor(clientRID))

	// frontend that fans out to the backend
	flis := bufconn.Listen(1 << 20)
	fsrv := grpc.NewServer(grpc.ChainUnaryInterceptor(serverRID, slogUnary(logger, "frontend"), metricsUnary(reg, "frontend")))
	greetpb.RegisterGreeterServer(fsrv, frontend{backend: toBackend})
	go fsrv.Serve(flis)
	toFrontend := dial(flis)

	// one request from the edge, with a request-id set once
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-request-id", "req-42")
	toFrontend.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})

	// scrape the shared registry
	ts := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer ts.Close()
	res, _ := http.Get(ts.URL)
	body, _ := io.ReadAll(res.Body)
	fmt.Println("--- /metrics ---")
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "grpc_server_handled_total") {
			fmt.Println(line)
		}
	}
}
```

**Output** — one request-id in both logs, one counter per service:

```json
{"time":"...","level":"INFO","msg":"grpc","service":"backend","method":"/greet.v1.Greeter/SayHello","code":"OK","ms":0,"request_id":"req-42"}
{"time":"...","level":"INFO","msg":"grpc","service":"frontend","method":"/greet.v1.Greeter/SayHello","code":"OK","ms":1,"request_id":"req-42"}
```
```
--- /metrics ---
grpc_server_handled_total{code="OK",method="/greet.v1.Greeter/SayHello",service="backend"} 1
grpc_server_handled_total{code="OK",method="/greet.v1.Greeter/SayHello",service="frontend"} 1
```

That's the whole idea of the example-projects: the gateway mints a request-id, it rides the metadata through
every hop so all services' logs share it, and every service exports Prometheus counters/histograms that
Prometheus scrapes and Grafana graphs. Go build `grpc-observability-hard` to see it wired with real Postgres,
Docker Compose, Prometheus, and Grafana.

---

> ← Prev tier: [🟡 medium](2-medium.md) · [index](README.md) · 🎉 that's the lesson — on to the projects.
