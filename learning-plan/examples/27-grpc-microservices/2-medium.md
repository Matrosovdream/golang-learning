# Step 27 — gRPC & Microservices · 🟡 Medium — examples **9–17**

Streaming in the other two directions, then **interceptors** — gRPC's middleware, where logging,
request-ids, and metrics live. Reuse the [scratch module](README.md#one-time-setup-do-this-first);
put each example in `main.go` and `go run .`.

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Client-streaming: one summary reply

`🟡 medium` · *streaming*

`rpc CollectNames(stream HelloRequest) returns (HelloReply)` — the **client** streams many messages, the
server reads them all and returns **one** summary. The server loops `Recv()` until `io.EOF`, then calls
`SendAndClose`; the client `Send`s then `CloseAndRecv`.

```go
package main

import (
	"context"
	"io"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) CollectNames(stream greetpb.Greeter_CollectNamesServer) error {
	var names []string
	for {
		req, err := stream.Recv()
		if err == io.EOF { // client is done sending
			return stream.SendAndClose(&greetpb.HelloReply{
				Message: "collected: " + joinComma(names),
			})
		}
		if err != nil {
			return err
		}
		names = append(names, req.GetName())
	}
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	stream, _ := client.CollectNames(context.Background())
	for _, n := range []string{"Ann", "Bob", "Cara"} {
		stream.Send(&greetpb.HelloRequest{Name: n})
	}
	reply, _ := stream.CloseAndRecv() // stop sending, wait for the one summary
	log.Println(reply.GetMessage())
}
```

**Output:**

```
2026/07/06 11:38:05 collected: Ann, Bob, Cara
```

---

## 10. Bidirectional streaming: Chat

`🟡 medium` · *streaming*

`rpc Chat(stream HelloRequest) returns (stream HelloReply)` — both sides stream at once over one HTTP/2
connection. Here the client sends on a goroutine while the main loop reads replies; `CloseSend` tells the
server "no more from me", which the server sees as `io.EOF`.

```go
package main

import (
	"context"
	"io"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) Chat(stream greetpb.Greeter_ChatServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil // client closed its half; end the stream
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&greetpb.HelloReply{Message: "echo " + req.GetName()}); err != nil {
			return err
		}
	}
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	stream, _ := client.Chat(context.Background())
	go func() { // send half
		for _, n := range []string{"x", "y", "z"} {
			stream.Send(&greetpb.HelloRequest{Name: n})
		}
		stream.CloseSend() // done sending -> server's Recv returns io.EOF
	}()
	for { // receive half
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		log.Println(msg.GetMessage())
	}
}
```

**Output:**

```
2026/07/06 11:38:05 echo x
2026/07/06 11:38:05 echo y
2026/07/06 11:38:05 echo z
```

---

## 11. Metadata: the request's headers

`🟡 medium` · *metadata*

Metadata is gRPC's key/value side-channel (like HTTP headers). The client attaches it with
`metadata.AppendToOutgoingContext`; the server reads it with `metadata.FromIncomingContext`. This is the
mechanism a **request-id** rides on across services.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	md, _ := metadata.FromIncomingContext(ctx) // read what the client attached
	caller := "unknown"
	if v := md.Get("x-caller"); len(v) > 0 {
		caller = v[0]
	}
	log.Printf("server: x-caller=%s", caller)
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	// attach metadata to the outgoing call
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-caller", "gateway")
	reply, _ := client.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})
	log.Println(reply.GetMessage())
}
```

**Output:**

```
2026/07/06 11:44:10 server: x-caller=gateway
2026/07/06 11:44:10 hello Stan
```

> Metadata keys are lower-cased by gRPC. Keys ending in `-bin` carry binary values; everything else is a
> string. Don't put huge blobs here — it's headers, not a payload.

---

## 12. A unary server interceptor that logs

`🟡 medium` · *interceptors*

An **interceptor** wraps every RPC so cross-cutting code lives in one place. A `UnaryServerInterceptor`
receives the request and a `handler` you must call — do work before/after it. Here: log method, resulting
**code**, and **latency** for every call, without touching any handler.

```go
package main

import (
	"context"
	"log"
	"net"
	"time"

	greetpb "scratch/greetpb"

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

// the interceptor: same signature every time
func loggingUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req) // <-- run the actual RPC
	log.Printf("method=%s code=%s dur=%s", info.FullMethod, status.Code(err), time.Since(start))
	return resp, err
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(loggingUnary)) // install it
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: ""}) // errors
}
```

**Output:**

```
2026/07/06 11:45:01 method=/greet.v1.Greeter/SayHello code=OK dur=8.5µs
2026/07/06 11:45:01 method=/greet.v1.Greeter/SayHello code=InvalidArgument dur=3.1µs
```

---

## 13. Chaining interceptors

`🟡 medium` · *interceptors*

You usually want several: recover from panics, tag a request-id, log, record metrics. `grpc.ChainUnaryInterceptor`
runs them **outer-to-inner** — the first in the list wraps all the rest. Order matters: put request-id
**before** logging so the log line already has the id.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func tag(name string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		log.Printf("-> enter %s", name)
		resp, err := handler(ctx, req)
		log.Printf("<- leave %s", name)
		return resp, err
	}
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(tag("A-outer"), tag("B-inner")))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	greetpb.NewGreeterClient(conn).SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
}
```

**Output** (nesting — A wraps B wraps the handler):

```
2026/07/06 11:46:00 -> enter A-outer
2026/07/06 11:46:00 -> enter B-inner
2026/07/06 11:46:00 <- leave B-inner
2026/07/06 11:46:00 <- leave A-outer
```

---

## 14. A client interceptor that injects a request-id

`🟡 medium` · *interceptors*

Servers aren't the only place for middleware — a `UnaryClientInterceptor` wraps every **outgoing** call. Use
one to attach an `x-request-id` to metadata automatically, so no call site has to remember to.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	log.Printf("server saw x-request-id=%v", md.Get("x-request-id"))
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

// client interceptor: attach a request-id to every outgoing call
func injectRequestID(id string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", id)
		return invoker(ctx, method, req, reply, cc, opts...) // proceed with the call
	}
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(injectRequestID("req-777"))) // install client interceptor
	defer conn.Close()

	greetpb.NewGreeterClient(conn).SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
}
```

**Output:**

```
2026/07/06 11:47:12 server saw x-request-id=[req-777]
```

---

## 15. Propagate a request-id across a hop

`🟡 medium` · *observability* · ★ the key microservices pattern

This is how one user request stays traceable across services. Two servers, front → back:

- **server interceptor** reads `x-request-id` from incoming metadata (or mints one) and stashes it in `context`.
- **client interceptor** re-attaches whatever id is in `context` to the *next* outgoing call.

So the id set once at the edge appears in **every** service's logs for that request. Run it and see the same
`req-XYZ-42` at the frontend and the backend.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type ridKey struct{}

func ridFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ridKey{}).(string); ok {
		return v
	}
	return "none"
}

// server side: pull id out of metadata into context, and log it
func serverRID(name string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := "gen-fresh" // in real code: mint a UUID here
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get("x-request-id"); len(v) > 0 {
				id = v[0]
			}
		}
		ctx = context.WithValue(ctx, ridKey{}, id)
		log.Printf("%s: handling %s request_id=%s", name, info.FullMethod, id)
		return handler(ctx, req)
	}
}

// client side: re-attach the context's id to the outgoing call
func clientRID(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.AppendToOutgoingContext(ctx, "x-request-id", ridFrom(ctx)), method, req, reply, cc, opts...)
}

// backend service
type backend struct{ greetpb.UnimplementedGreeterServer }

func (backend) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "hi " + r.GetName()}, nil
}

// frontend service: each call fans out to the backend
type frontend struct {
	greetpb.UnimplementedGreeterServer
	backend greetpb.GreeterClient
}

func (f frontend) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return f.backend.SayHello(ctx, r) // ctx carries the id; clientRID puts it on the wire
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
	// backend server + a client to it that re-attaches the id
	blis := bufconn.Listen(1 << 20)
	bsrv := grpc.NewServer(grpc.ChainUnaryInterceptor(serverRID("backend")))
	greetpb.RegisterGreeterServer(bsrv, backend{})
	go bsrv.Serve(blis)
	toBackend := dial(blis, grpc.WithChainUnaryInterceptor(clientRID))

	// frontend server that calls the backend
	flis := bufconn.Listen(1 << 20)
	fsrv := grpc.NewServer(grpc.ChainUnaryInterceptor(serverRID("frontend")))
	greetpb.RegisterGreeterServer(fsrv, frontend{backend: toBackend})
	go fsrv.Serve(flis)
	toFrontend := dial(flis)

	// the edge sets the id once
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-request-id", "req-XYZ-42")
	reply, _ := toFrontend.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})
	log.Printf("reply: %s", reply.GetMessage())
}
```

**Output** — one id, both services:

```
2026/07/06 11:42:02 frontend: handling /greet.v1.Greeter/SayHello request_id=req-XYZ-42
2026/07/06 11:42:02 backend: handling /greet.v1.Greeter/SayHello request_id=req-XYZ-42
2026/07/06 11:42:02 reply: hi Stan
```

> This is exactly what `grpc-orders-intermediate` does: the gateway mints a request-id, and it shows up in
> the gateway's, the orders service's, and the catalog service's logs for a single `curl`.

---

## 16. A recovery interceptor: panic → codes.Internal

`🟡 medium` · *interceptors / resilience*

A panic in one handler must not crash the whole server. A recovery interceptor `recover()`s, turns the panic
into a clean `codes.Internal` for the client, and logs it — using a **named return** so the deferred func can
set `err`.

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	if r.GetName() == "boom" {
		panic("something blew up")
	}
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func recovery(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in %s: %v", info.FullMethod, r)
			err = status.Errorf(codes.Internal, "internal error") // don't leak internals to the client
		}
	}()
	return handler(ctx, req)
}

func main() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(recovery))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	_, err := client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "boom"})
	log.Printf("client got code=%s (server still alive)", status.Code(err))

	ok, _ := client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	log.Printf("next call fine: %s", ok.GetMessage())
}
```

**Output:**

```
2026/07/06 11:48:30 PANIC in /greet.v1.Greeter/SayHello: something blew up
2026/07/06 11:48:30 client got code=Internal (server still alive)
2026/07/06 11:48:30 next call fine: hello Stan
```

---

## 17. Time every RPC (the seed of a metric)

`🟡 medium` · *observability*

Before wiring Prometheus, see the shape of a metrics interceptor: it just times each call and accumulates
counts by method and code. Swap the `map` for Prometheus collectors (example 20) and you have real metrics.

```go
package main

import (
	"context"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	greetpb "scratch/greetpb"

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
	time.Sleep(time.Millisecond)
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

type metrics struct {
	mu    sync.Mutex
	count map[string]int // key: "method code"
}

func (m *metrics) unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	key := info.FullMethod + " " + status.Code(err).String()
	m.mu.Lock()
	m.count[key]++
	m.mu.Unlock()
	return resp, err
}

func main() {
	m := &metrics{count: map[string]int{}}

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(m.unary))
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	for i := 0; i < 3; i++ {
		client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	}
	client.SayHello(context.Background(), &greetpb.HelloRequest{Name: ""}) // one error

	keys := make([]string, 0, len(m.count))
	for k := range m.count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Printf("%-45s %d", k, m.count[k])
	}
}
```

**Output:**

```
2026/07/06 11:49:44 /greet.v1.Greeter/SayHello InvalidArgument 1
2026/07/06 11:49:44 /greet.v1.Greeter/SayHello OK             3
```

---

> ← Prev tier: [🟢 easy](1-easy.md) · [index](README.md) · Next tier: [🔴 hard](3-hard.md) →
