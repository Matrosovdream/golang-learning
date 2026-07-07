# Step 27 — gRPC & Microservices · 🟢 Easy — examples **1–8**

Do the [one-time setup](README.md#one-time-setup-do-this-first) first (module + `greet.proto` + generated
stubs). Then for each example, put the code in `main.go` and `go run .`.

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Your first .proto and what protoc generates

`🟢 easy` · *proto / codegen*

You already wrote `greet.proto` and ran `protoc` in the setup. The point of this example is to **look at
what came out** — because everything else in the lesson is built on those two generated files.

**Steps:**

1. `protoc` produced two files next to your proto:
   - `greet.pb.go` — the **messages** (`HelloRequest`, `HelloReply`) as Go structs, with getters (`GetName()`), and the protobuf marshal/unmarshal machinery.
   - `greet_grpc.pb.go` — the **service**: a client interface, a server interface, and a registrar.
2. The three things you'll use constantly live in `greet_grpc.pb.go`. Find them:

   ```bash
   grep -nE "type GreeterClient|type GreeterServer|func NewGreeterClient|func RegisterGreeterServer" greetpb/greet_grpc.pb.go
   ```

3. Read what each one is:
   - **`GreeterClient`** — the interface you *call*. `SayHello(ctx, *HelloRequest) (*HelloReply, error)` — a method call that happens to go over the network.
   - **`GreeterServer`** — the interface you *implement*. Note the embedded `UnimplementedGreeterServer` you must include for forward compatibility.
   - **`NewGreeterClient(conn)`** — wraps a connection in a client.
   - **`RegisterGreeterServer(srv, impl)`** — plugs your implementation into a `*grpc.Server`.

**Output** (`grep …`):

```
33:type GreeterClient interface {
44:func NewGreeterClient(cc grpc.ClientConnInterface) GreeterClient {
108:type GreeterServer interface {
145:func RegisterGreeterServer(s grpc.ServiceRegistrar, srv GreeterServer) {
```

> **Field numbers are the contract.** `name = 1` — the `1` is the field's identity on the wire, not the
> name. You can rename `name` in the proto and old clients still work; you can **never** reuse the number `1`
> for a different field. Add new fields with new numbers; treat the proto as append-only.

---

## 2. Hello gRPC in one process (bufconn)

`🟢 easy` · *server + client*

Real gRPC is two processes. To *learn* it without juggling terminals, run the server and client in one
program connected by an in-memory pipe (`bufconn`). Same API as TCP — you just swap the dialer. This is
the skeleton every later example reuses.

**Steps:**

1. Implement `GreeterServer` — embed `UnimplementedGreeterServer` (forward-compat), then write `SayHello`.
2. Start a server on a `bufconn.Listener` in a goroutine.
3. Dial it with `grpc.NewClient` + a custom `ContextDialer` that returns the bufconn, wrap in `NewGreeterClient`, and call `SayHello`.

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

type server struct {
	greetpb.UnimplementedGreeterServer // embed for forward compatibility
}

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func main() {
	lis := bufconn.Listen(1 << 20) // 1MB in-memory pipe, stands in for a TCP listener
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	go srv.Serve(lis)

	// dial the in-memory listener; over TCP this is grpc.NewClient("host:port", ...)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials())) // no TLS for local dev
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := greetpb.NewGreeterClient(conn)
	reply, err := client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(reply.GetMessage())

	srv.GracefulStop()
}
```

**Output:**

```
2026/07/06 11:36:47 hello Stan
```

> `insecure.NewCredentials()` disables TLS — fine on a private Compose network or in-process, never on the
> public internet. `grpc.NewClient` is the current API; `grpc.Dial`/`DialContext` are deprecated.

---

## 3. The wire format: proto.Marshal vs JSON

`🟢 easy` · *protobuf*

gRPC is fast partly because protobuf is a compact **binary** format. See it directly: marshal a message and
compare its byte size to the equivalent JSON.

**Steps:**

1. Build a `HelloRequest{Name: "Stan", Count: 3}` and `proto.Marshal` it — you get raw bytes.
2. Marshal the same data as JSON and compare lengths.
3. Look at the hex: `0a` = field 1 (name), `04` = 4 bytes, then `Stan`; `10` = field 2 (count), `03` = 3. Field **numbers**, not names, are on the wire — that's why renaming a field is safe and renumbering is not.

```go
package main

import (
	"encoding/json"
	"fmt"

	greetpb "scratch/greetpb"

	"google.golang.org/protobuf/proto"
)

func main() {
	msg := &greetpb.HelloRequest{Name: "Stan", Count: 3}

	wire, _ := proto.Marshal(msg) // binary protobuf
	js, _ := json.Marshal(map[string]any{"name": "Stan", "count": 3})

	fmt.Printf("protobuf: %d bytes  %x\n", len(wire), wire)
	fmt.Printf("json:     %d bytes  %s\n", len(js), js)

	// round-trip back into a struct
	var back greetpb.HelloRequest
	_ = proto.Unmarshal(wire, &back)
	fmt.Printf("decoded:  name=%q count=%d\n", back.GetName(), back.GetCount())
}
```

**Output:**

```
protobuf: 8 bytes  0a045374616e1003
json:     25 bytes  {"count":3,"name":"Stan"}
decoded:  name="Stan" count=3
```

---

## 4. Every call gets a deadline

`🟢 easy` · *context / deadlines*

A client with no deadline waits **forever** on a hung server. Always pass a `context.WithTimeout`. When it
fires, the call returns `codes.DeadlineExceeded` — and the deadline propagates to the server automatically.

**Steps:**

1. Make the server sleep 50ms before replying.
2. Call it with a 10ms deadline — the client gives up first.
3. Inspect the error's code with `status.Code(err)`.

```go
package main

import (
	"context"
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

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	time.Sleep(50 * time.Millisecond) // slow server
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})
	log.Printf("code = %s", status.Code(err)) // DeadlineExceeded
}
```

**Output:**

```
2026/07/06 11:38:05 code = DeadlineExceeded
```

---

## 5. Status codes, not error strings

`🟢 easy` · *errors*

Never return a bare Go error across a service boundary — the caller gets `codes.Unknown` and a mangled
string. Return a **`status`** with a meaningful **code**; the caller branches on the code.

**Steps:**

1. Server: reject an empty name with `status.Errorf(codes.InvalidArgument, …)`.
2. Client: turn the error into a status with `status.Convert(err)` and read `.Code()` / `.Message()`.
3. Note the happy path returns `codes.OK`.

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
	if r.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
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

	ok, _ := client.SayHello(context.Background(), &greetpb.HelloRequest{Name: "Stan"})
	log.Printf("ok:   %q", ok.GetMessage())

	_, err := client.SayHello(context.Background(), &greetpb.HelloRequest{Name: ""})
	st := status.Convert(err)
	log.Printf("fail: code=%s msg=%q", st.Code(), st.Message())
}
```

**Output:**

```
2026/07/06 11:36:47 ok:   "hello Stan"
2026/07/06 11:36:47 fail: code=InvalidArgument msg="name is required"
```

> At an API gateway you map these codes to HTTP: `InvalidArgument→400`, `NotFound→404`,
> `AlreadyExists→409`, `Unavailable→503`. You'll build that mapping in the projects.

---

## 6. A real TCP server (two terminals) and codes.Unavailable

`🟢 easy` · *real transport* · **two terminals**

bufconn is a teaching shortcut. Real services listen on a TCP port and are dialed by address — which also
means the peer can be **down**. When it is, the client gets `codes.Unavailable` (this is the `503` of gRPC).

**Steps:**

1. **Terminal A** — save this as `server/main.go` (a subpackage so it doesn't clash with your example `main.go`) and run `go run ./server`. It listens on `:9000`.
2. **Terminal B** — save the client as `main.go` and `go run .`. It dials `localhost:9000` and prints the reply.
3. Now stop the server (Ctrl-C in Terminal A) and run the client again — it prints `code=Unavailable`.

`server/main.go`:

```go
package main

import (
	"context"
	"log"
	"net"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
)

type server struct{ greetpb.UnimplementedGreeterServer }

func (server) SayHello(ctx context.Context, r *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "hello " + r.GetName()}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatal(err)
	}
	srv := grpc.NewServer()
	greetpb.RegisterGreeterServer(srv, server{})
	log.Println("listening on :9000")
	log.Fatal(srv.Serve(lis))
}
```

`main.go` (client):

```go
package main

import (
	"context"
	"log"
	"time"

	greetpb "scratch/greetpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	conn, _ := grpc.NewClient("localhost:9000",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := greetpb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reply, err := client.SayHello(ctx, &greetpb.HelloRequest{Name: "Stan"})
	if err != nil {
		log.Printf("call failed: code=%s", status.Code(err)) // Unavailable if server is down
		return
	}
	log.Println(reply.GetMessage())
}
```

**Output** (server up, then server down):

```
# server running:
2026/07/06 11:40:12 hello Stan
# server stopped:
2026/07/06 11:40:20 call failed: code=Unavailable
```

> `grpc.NewClient` connects **lazily** — creating the client never fails just because the server is down; the
> first RPC is where you find out. That's why you branch on the call's error, not the dial.

---

## 7. proto3 field presence: zero value vs optional

`🟢 easy` · *proto3 semantics*

In proto3 a plain scalar has **no presence**: an unset `int32` and a real `0` look identical on the wire and
decode to the same Go zero value. If you need to tell "unset" from "zero", mark the field `optional` — that
generates a **pointer**, so `nil` means unset.

**Steps:**

1. Add an `optional` field to the proto and regenerate (temporary — you can revert after).

   ```proto
   message HelloRequest {
     string name = 1;
     int32 count = 2;
     optional int32 volume = 3;   // presence-tracked: generates *int32
   }
   ```

   ```bash
   protoc --go_out=. --go_opt=paths=source_relative \
          --go-grpc_out=. --go-grpc_opt=paths=source_relative greetpb/greet.proto
   ```

2. `count` (plain) is always an `int32`; `volume` (optional) is an `*int32`. Set neither and observe.

```go
package main

import (
	"fmt"

	greetpb "scratch/greetpb"
)

func main() {
	var r greetpb.HelloRequest // everything unset

	// plain scalar: unset is indistinguishable from 0
	fmt.Printf("count = %d  (unset looks like a real 0)\n", r.GetCount())

	// optional scalar: a nil pointer means "not set"
	fmt.Printf("volume set? %v\n", r.Volume != nil) // false
	fmt.Printf("volume get  %d  (getter returns zero when unset)\n", r.GetVolume())

	v := int32(11)
	r.Volume = &v
	fmt.Printf("volume set? %v  value=%d\n", r.Volume != nil, r.GetVolume())
}
```

**Output:**

```
count = 0  (unset looks like a real 0)
volume set? false
volume get  0  (getter returns zero when unset)
volume set? true  value=11
```

> Reach for `optional` when 0 / "" / false are legitimate values you must distinguish from "the client
> didn't send this" (patch requests, config overrides). Otherwise plain scalars are simpler.

---

## 8. Server-streaming: many replies, read to io.EOF

`🟢 easy` · *streaming*

`rpc SayManyHellos(HelloRequest) returns (stream HelloReply)` — one request, a **stream** of replies. The
server loops calling `stream.Send`; the client loops calling `Recv()` until it gets `io.EOF`.

**Steps:**

1. Server: implement `SayManyHellos(req, stream)` — `Send` one reply per `count`.
2. Client: call it, then `for { Recv() }` until `io.EOF` ends the loop.

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

// note: no ctx arg — the stream carries the context; return nil to end the stream
func (server) SayManyHellos(r *greetpb.HelloRequest, stream greetpb.Greeter_SayManyHellosServer) error {
	for i := int32(0); i < r.GetCount(); i++ {
		if err := stream.Send(&greetpb.HelloReply{Message: "hello " + r.GetName()}); err != nil {
			return err
		}
	}
	return nil // closing the stream sends io.EOF to the client
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

	stream, err := client.SayManyHellos(context.Background(), &greetpb.HelloRequest{Name: "Stan", Count: 3})
	if err != nil {
		log.Fatal(err)
	}
	for {
		msg, err := stream.Recv()
		if err == io.EOF { // the stream is done
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
2026/07/06 11:36:47 hello Stan
2026/07/06 11:36:47 hello Stan
2026/07/06 11:36:47 hello Stan
```

---

> ← [index](README.md) · Next tier: [🟡 medium](2-medium.md) →
