# Networking, HTTP Internals & Serving Cheatsheet

**Lessons:** [63 — Networking Fundamentals](../63-networking-fundamentals.md) · [64 — HTTP & net/http Internals](../64-http-protocol-internals.md) · [65 — Docker Networking](../65-docker-networking.md) · [66 — Every Way to Serve a Go App](../66-serving-go-apps.md)
**Examples:** [63](../examples/63-networking-fundamentals/) · [64](../examples/64-http-protocol-internals/)
**Covers:** TCP/IP, the `net` package, the HTTP wire format, TLS, the server machine, container networking, listeners
**Legend:** `[*]` = API the lessons have not covered yet

## THE FOUR LAYERS

```text
link                         Ethernet/wifi frames, MAC addresses
internet                     IP packets, addresses, routing, TTL
transport                    TCP (ordered, reliable, a stream)
                             UDP (messages, best effort)
application                  HTTP, gRPC, DNS, TLS-wrapped anything
an address is not a connection    a connection is a 4-TUPLE:
                             (src IP, src port, dst IP, dst port)
                             — which is how one server port serves 10,000 clients
```

## TCP

```text
3-way handshake              SYN -> SYN-ACK -> ACK (one round trip before any data)
byte stream, NO MESSAGE BOUNDARIES     the single most important fact.
                             Two Writes can arrive as one Read, or as three.
framing is YOUR job          delimiter (\n) or length-prefix (4 bytes, then the body)
close                        FIN -> ACK each way; half-open is legal (CloseWrite)
TIME_WAIT                    2 x MSL on the side that closed FIRST; it's correct
                             behaviour, not a leak
accept queue                 the kernel completes handshakes into a backlog; your
                             Accept() loop drains it. A slow loop = refused connections.
keepalive                    detects a peer that vanished without a FIN
Nagle / delayed ACK          small writes get coalesced; SetNoDelay(true) for latency
MTU ~1500                    bigger payloads fragment; PMTU discovery finds the limit
NAT                          rewrites the tuple; that's why inbound needs a mapping
```

## SOCKETS & THE GO RUNTIME

```text
a socket is a file descriptor       ulimit -n is the real ceiling on connections
netpoller                    the runtime multiplexes ALL sockets over epoll/kqueue
goroutine-per-connection     is cheap BECAUSE of that: a blocked goroutine costs
                             ~4KB of stack, not an OS thread
so: write blocking code      and let the runtime make it async
```

## THE net PACKAGE

```text
net.Listen("tcp", ":8080")   -> (Listener, error)
ln.Accept()                  -> (Conn, error); the loop
ln.Addr()                    with ":0", the OS-assigned port — the test idiom
net.Dial("tcp", "host:port") -> (Conn, error)
net.DialTimeout(...)         [*] bound the connect
&net.Dialer{Timeout, KeepAlive}.DialContext(ctx, ...)   the cancellable form
conn.Read(b) / conn.Write(b) Write can be PARTIAL — check n
conn.Close()
conn.SetDeadline / SetReadDeadline / SetWriteDeadline
                             THE ONLY cancellation a socket has. ctx does not
                             interrupt a blocked Read — a deadline does.
net.SplitHostPort / JoinHostPort      never split on ":" yourself (IPv6)
net.ListenPacket("udp", ...) UDP: messages KEEP their boundaries, may be lost/reordered
net.Listen("unix", "/tmp/x.sock")     Unix domain socket: no TCP stack, filesystem perms
net.Pipe()               [*] an in-memory Conn — perfect for tests
net.LookupHost / Resolver    DNS
GODEBUG=netdns=go|cgo|2  [*] pick and debug the resolver
```

## READING THE ERRORS

```text
connection refused           nothing is listening on that port (fast, definitive)
i/o timeout                  your deadline fired
connection reset by peer     the other side sent RST — it crashed or closed abruptly
EOF                          a clean close by the peer, mid-read
no such host                 DNS failure
address already in use       another process holds the port (or TIME_WAIT + no reuse)
broken pipe                  you wrote to a connection the peer already closed
context deadline exceeded    your ctx, not the socket
toolbox                      ss -tlnp / lsof -i / nc -zv / dig / tcpdump / curl -v
```

## HTTP/1.1 ON THE WIRE

```text
GET /path HTTP/1.1\r\n       the request line
Host: example.com\r\n        mandatory in 1.1
Header: value\r\n
\r\n                         a blank line ends the headers
[body]
how the body's end is found  Content-Length: n     — exactly n bytes
                             Transfer-Encoding: chunked — size in hex, CRLF, data,
                               ..., then a 0-length chunk
                             neither, on a response: read until close
keep-alive                   the default in 1.1; the connection is reused
head-of-line blocking        one slow response blocks the next on the SAME connection
pipelining                   theoretically allowed, practically dead
```

## HTTP/2 & HTTP/3

```text
HTTP/2                       binary FRAMES over one TCP connection
streams                      many concurrent requests, multiplexed, interleaved
HPACK                        header compression with a shared dynamic table
flow control                 per stream and per connection
server push                  deprecated in practice
still has TCP HOL blocking   one lost packet stalls every stream
HTTP/3 over QUIC             UDP-based; streams are independent, so no TCP HOL
                             0-RTT resumption, connection migration across networks
h2c                      [*] HTTP/2 without TLS — for internal hops only
Go's server does h2          automatically over TLS; the client too
```

## TLS

```text
handshake                    ClientHello -> ServerHello + certificate -> key exchange
                             (1-RTT in TLS 1.3, 2 in 1.2)
certificate chain            leaf -> intermediate -> root; SERVE THE INTERMEDIATES
SNI                          the hostname in the ClientHello — how one IP serves
                             many certificates
ALPN                         negotiates the protocol IN the handshake: "h2" or
                             "http/1.1" — this is where HTTP/2 comes from
mTLS                         the client presents a certificate too; service identity
tls.Config{MinVersion: tls.VersionTLS12}
InsecureSkipVerify           a deliberate MITM hole; never in production
```

## THE net/http SERVER MACHINE

```text
ListenAndServe               net.Listen, then Serve(ln)
Serve(ln)                    for { c, _ := ln.Accept(); go c.serve() }
go c.serve()                 ONE GOROUTINE PER CONNECTION — the whole design
                             read request -> ServeMux -> your handler -> write
                             -> loop for the next keep-alive request
ServeMux                     pattern match -> Handler
ResponseWriter               buffered; the status is sent with the first flush
the five timeouts
  ReadHeaderTimeout          headers only — the Slowloris defence
  ReadTimeout                headers + body
  WriteTimeout               from the END of the request read to the end of the write
                             — it BREAKS SSE/streaming; set it to 0 for those routes
  IdleTimeout                keep-alive idle
  Handler-level              http.TimeoutHandler, or a ctx deadline
Flusher                      w.(http.Flusher).Flush() — push what's buffered
Hijacker                     take over the raw connection (WebSocket)
ResponseController       [*] Go 1.20+: SetWriteDeadline/Flush per response — the
                             modern replacement for the interface assertions
CloseNotifier                deprecated; use r.Context()
```

## THE net/http CLIENT

```text
http.Transport IS A CONNECTION POOL      one per client; reuse the client
MaxIdleConns / MaxIdleConnsPerHost       the default per-host is 2 — raise it for
                                          a service you call constantly
IdleConnTimeout / TLSHandshakeTimeout / ExpectContinueTimeout
DisableKeepAlives                        almost always wrong
THE DRAIN-AND-CLOSE RULE     io.Copy(io.Discard, resp.Body); resp.Body.Close()
                             — without draining, the connection is NOT reused
one client, reused           a new http.Client per request = a new pool = socket
                             exhaustion under load
retries                      only for idempotent requests; the body must be replayable
X-Forwarded-For / -Proto     trust them ONLY from your own proxy, which must overwrite
                             rather than append what the client sent
httputil.ReverseProxy        a production-grade proxy in a few lines; set Director
                             or Rewrite, and handle ErrorHandler
```

## CONTAINER NETWORKING

```text
network namespace            each container has its own interfaces, routes, and ports
veth + bridge + NAT          the default bridge network's plumbing
-p 8080:8080                 a DNAT rule on the host
EXPOSE                       documentation. It does nothing.
KILLER #1                    bind 0.0.0.0, NOT 127.0.0.1 — localhost inside the
                             container is unreachable from outside it
KILLER #2                    "localhost" inside a container means THAT container
container DNS 127.0.0.11     resolves service names on a user-defined network
                             (the default bridge has NO DNS — use compose networks)
address a peer               http://api:8080 — the SERVICE name and the CONTAINER
                             port, never the published host port
compose depends_on           start order, NOT readiness — add a healthcheck + condition
internal: true               a network with no outbound route
scaling                      multiple A records for one service name
host networking              no namespace, no NAT — Linux only
host.docker.internal         reach the host from a container (Docker Desktop)
scratch images               no CA certs (HTTPS fails), no tzdata, no /etc/hosts tools
PID 1                        your process gets no default signal handlers — handle
                             SIGTERM yourself; use exec-form ENTRYPOINT
debugging a toolless image   docker run --network container:<id> nicolaka/netshoot
Kubernetes mapping           pod = one netns shared by its containers; Service = a
                             virtual IP; cluster DNS = svc.namespace.svc.cluster.local
```

## EVERY WAY TO SERVE (the catalog)

```text
the one primitive            ln, _ := net.Listen(...); srv.Serve(ln)
                             — everything below is a different Listener
:0                           an OS-assigned port; ln.Addr() tells you which
TLS                          srv.ListenAndServeTLS / tls.NewListener / autocert
netutil.LimitListener(ln, n) [*] cap concurrent connections
Unix socket                  net.Listen("unix", path) — nginx in front, no TCP
systemd socket activation    the listener arrives as fd 3:
                             net.FileListener(os.NewFile(3, ""))
SO_REUSEPORT             [*] several processes on one port; kernel load balancing
zero-downtime restart        (1) SO_REUSEPORT, (2) pass the fd to a child,
                             (3) systemd socket activation, (4) a proxy in front
h2c                      [*] HTTP/2 cleartext, for internal service hops
HTTP/3                   [*] quic-go + an Alt-Svc header advertising it
raw TCP / UDP                your own protocol, straight on net.Conn/PacketConn
gRPC + REST on one port      switch on Content-Type + ProtoMajor, or use cmux
grpc bufconn                 an in-memory gRPC listener for tests
behind a reverse proxy       nginx/Caddy/Traefik terminate TLS; trust X-Forwarded-*
httputil.ReverseProxy        be the proxy yourself
FastCGI / CGI                net/http/fcgi and net/http/cgi — legacy hosting
PaaS / Lambda                read $PORT; no background goroutines after the response
several servers, one process public :8080 + admin :9090 (pprof, metrics, health)
                             — never expose pprof publicly
httptest / net.Pipe          no sockets at all, for tests
```

## TRAPS & MEMORIZE

```text
assuming Read returns a message   TCP has no boundaries; frame it yourself
ignoring the n from Write     partial writes are legal
no deadlines on a raw Conn    ctx will not save you; a Read blocks forever
one Accept loop doing work    accept, then `go handle(conn)` — always
binding 127.0.0.1 in Docker   the container is unreachable and nothing logs why
publishing != reachable       -p maps the HOST; containers talk on the container port
depends_on as readiness       the app starts before the database can accept
WriteTimeout on an SSE route  the server cuts the stream at the deadline
no ReadHeaderTimeout          Slowloris holds every connection open
a client per request          the pool is per-Transport; you just disabled it
not draining resp.Body        the connection is closed instead of reused
trusting X-Forwarded-For      spoofable unless your proxy overwrites it
missing intermediates         works in your browser, fails in curl and in Go
exposing pprof publicly       heap dumps and goroutine stacks to anyone who asks
```
