# 66 — Every Way to Serve a Go App: Listeners, Protocols & Runtimes

> Part of **Part 14 — Networking & Serving**, the closing lesson: [63 — TCP/IP & sockets](63-networking-fundamentals.md) → [64 — HTTP & `net/http` internals](64-http-protocol-internals.md) → [65 — Container & Docker networking](65-docker-networking.md) → **66 the catalog**. This is the **menu**: every way a Go program can accept traffic, and how to choose. Deploy mechanics (Compose/Kubernetes/PaaS/systemd manifests) live in [62](62-deployment-operations.md); this lesson is about what happens **inside the process**. Thesis: **there is exactly one primitive — get a `net.Listener`, hand it to a server. TLS, Unix sockets, socket activation, `SO_REUSEPORT`, h2c, HTTP/3, FastCGI, gRPC, serverless and test servers are all variations on "where does the listener come from, and who speaks what on it".**

## Goals
- Internalize the one primitive: **`net.Listen` → `srv.Serve(ln)`** — and see every other option as a substitution for the listener, the server, or the transport.
- Know the **TCP/TLS/Unix-socket/socket-activation/`SO_REUSEPORT`** listener recipes and when each is the right answer.
- Serve the **protocol variants**: HTTPS, **h2c**, **HTTP/3**, gRPC (and gRPC+HTTP on one port), WebSocket/SSE, raw TCP/UDP, FastCGI/CGI.
- Run the process in every **runtime shape**: bare binary, systemd, container, behind a reverse proxy, PaaS, **serverless**, in-process test server.
- Compose them safely: **multiple servers in one process** (public + admin/pprof), listener limits, graceful shutdown and zero-downtime restarts.

## Concepts

- **The one primitive.** `http.ListenAndServe(addr, h)` is just:
  ```go
  ln, err := net.Listen("tcp", addr)   // 63: socket + bind + listen
  srv := &http.Server{Handler: h}      // 64: accept loop + goroutine per conn
  err = srv.Serve(ln)
  ```
  Everything below swaps one of those three lines. **Always construct `&http.Server{}` yourself** so you get the five timeouts ([64](64-http-protocol-internals.md)) — `ListenAndServe`'s convenience costs you slow-loris protection.
- **Owning the listener buys you things.** `net.Listen("tcp", ":0")` picks a **free port** and `ln.Addr()` tells you which — the trick that makes tests parallel-safe. You can also **wrap** a listener before serving: `netutil.LimitListener(ln, 1000)` (cap concurrent connections), `tls.NewListener(ln, cfg)` (add TLS), a PROXY-protocol listener (recover the client IP behind an L4 LB), or your own `Accept` wrapper that logs/counts/blocks by IP.
- **TLS, three ways.** (1) `srv.ListenAndServeTLS(certFile, keyFile)` — simplest; HTTP/2 turns on automatically via ALPN. (2) `srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: ...}` + `srv.ServeTLS(ln)` — full control (cipher suites, mTLS via `ClientAuth`, `GetCertificate` for SNI-based multi-domain). (3) **Automatic certificates** with `golang.org/x/crypto/acme/autocert` (Let's Encrypt: `autocert.Manager{Prompt: autocert.AcceptTOS, HostPolicy: ..., Cache: autocert.DirCache("certs")}` — needs :80 for the HTTP-01 challenge or TLS-ALPN on :443). In production the common answer is **(0): don't** — terminate TLS at the ingress/LB and serve plain HTTP inside the trust boundary ([62](62-deployment-operations.md)).
- **Unix domain socket.** `ln, _ := net.Listen("unix", "/run/app.sock"); srv.Serve(ln)`. Zero TCP overhead, unreachable from the network, permissions via the filesystem — the classic pairing with a local nginx (`proxy_pass http://unix:/run/app.sock:`). Remember to **remove a stale socket file** before listening and `os.Remove` on shutdown, and to `os.Chmod`/`Chown` it for the proxy's user.
- **systemd socket activation.** systemd binds the port (even a **privileged** one, as root) and passes the listening fd to your unprivileged process as **fd 3**: `f := os.NewFile(3, "listener"); ln, _ := net.FileListener(f)` (or `github.com/coreos/go-systemd/activation`). Buys: no-root privileged ports, on-demand start, and **connections queued by the kernel across a restart** — a zero-downtime restart without any fd-passing code of your own.
- **`SO_REUSEPORT`: many listeners, one port.** With `net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error { ... setsockopt(SO_REUSEPORT) ... }}`, several *processes* can bind the same port and the kernel load-balances new connections across their accept queues. Uses: multi-process deployments (rare in Go — goroutines already use all cores), and **restart with zero dropped connections** (start the new binary, then stop the old). Linux/BSD only.
- **Graceful shutdown & zero-downtime restart.** In-process: `signal.NotifyContext` + `srv.Shutdown(ctx)` drains ([62](62-deployment-operations.md)) — flip readiness first so the LB stops routing. Across a binary upgrade, pick one: (a) let the **orchestrator** do a rolling update (the right answer for containers — no code), (b) **socket activation**, (c) **`SO_REUSEPORT`** overlap, or (d) **fd inheritance** (`cloudflare/tableflip`, `jpillora/overseer`) where the old process passes the listener to a child on SIGHUP. Note `Shutdown` does *not* close **hijacked** connections (WebSockets, [58](58-realtime-websockets-sse.md)) — track and close those yourself; `RegisterOnShutdown` is the hook.
- **Cleartext HTTP/2 (h2c).** Behind an LB that speaks plain HTTP internally, or for gRPC without TLS, wrap the handler: `h2c.NewHandler(mux, &http2.Server{})` from `golang.org/x/net/http2/h2c`. Without it, a plain `http.Server` on a non-TLS port is HTTP/1.1 only (no ALPN, no negotiation).
- **HTTP/3.** Not in the stdlib. `quic-go`'s `http3.Server{Addr: ":443", Handler: mux, TLSConfig: cfg}` listens on **UDP** — so you run **both** an h1/h2 TCP server and the h3 UDP server, advertising the latter with the **`Alt-Svc`** header (`http3.Server.SetQUICHeaders`). Deployment gotchas: publish/allow **UDP** ([65](65-docker-networking.md)), and many LBs don't forward it.
- **Raw TCP and UDP servers.** Not everything is HTTP ([63](63-networking-fundamentals.md)): a TCP protocol server is `Accept` → `go handle(conn)` → framed reads with deadlines; a UDP server is one `net.ListenPacket` + `ReadFrom` loop feeding a worker pool (no per-peer goroutine, since there's no connection state). This is how you'd write a metrics receiver, a game server, a line protocol, or a proxy.
- **gRPC — and gRPC + HTTP on one port.** `lis, _ := net.Listen("tcp", ":50051"); s := grpc.NewServer(); pb.RegisterXServer(s, impl); s.Serve(lis)` — same primitive ([27](27-grpc-microservices.md)). Sharing one port with REST: either a **handler switch** (`if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") { grpcServer.ServeHTTP(w, r) } else { mux.ServeHTTP(w, r) }`, wrapped in h2c for cleartext) or **`soheilhy/cmux`**, which sniffs the first bytes of each connection and routes it to a different `Serve`. For browsers you need **grpc-web** or **grpc-gateway** (a generated REST↔gRPC proxy). Tests use **`bufconn`** — an in-memory listener with no ports at all.
- **Behind a reverse proxy (the default production shape).** nginx / Caddy / Traefik / Envoy / an ALB / a Kubernetes Ingress terminates TLS, serves static files, buffers slow clients, rate-limits, and forwards to your Go app on `127.0.0.1:8080` or a Unix socket. Your side of the contract: bind **loopback or the container's `0.0.0.0`** ([65](65-docker-networking.md)), trust `X-Forwarded-*` **only from that hop** ([64](64-http-protocol-internals.md)), disable proxy buffering for SSE, and configure `Upgrade`/`Connection` passthrough for WebSockets. Go's own `httputil.ReverseProxy` puts the same pattern *inside* your process (BFF, gateway, canary split, dev proxy).
- **FastCGI & CGI.** `net/http/fcgi`'s `fcgi.Serve(ln, handler)` speaks FastCGI to nginx/Apache (shared hosting, legacy fleets); `net/http/cgi` runs your handler as a one-shot CGI process. Rarely the right choice today — but they're stdlib, and they prove the point: the same `http.Handler` serves any transport.
- **Serverless & PaaS.** **Cloud Run / Fly / Render / Heroku**: no code change at all — a container that listens on **`$PORT`** ([62](62-deployment-operations.md)). **AWS Lambda**: there is no listener; you register a handler (`lambda.Start`) and an adapter maps API Gateway/ALB events to `http.Handler` (`awslabs/aws-lambda-go-api-proxy`, or the **Lambda Web Adapter** which lets an unmodified HTTP server run as-is). Constraints to design for: **cold starts** (Go's are small — one reason it's a great fit), **no background goroutines after the response** (the runtime freezes the process), no local state between invocations, and per-request billing that punishes idle connection pools.
- **Multiple servers in one process.** Very common and worth doing deliberately: a **public** server on `:8080` (your API) and a **separate admin server** on `:9090` for `/metrics`, `/debug/pprof`, and `/readyz` — because `net/http/pprof` registers on `DefaultServeMux` and must **never** be internet-reachable ([57](57-web-security.md)). Run them with an `errgroup` and shut both down on signal. Same technique for "HTTP + gRPC + a raw TCP protocol in one binary".
- **In-process and test listeners.** `httptest.NewServer(handler)` (real socket on a random port), `httptest.NewUnstartedServer` + `StartTLS` (TLS with a client-trusting cert), `httptest.NewRecorder` (no socket at all), `net.Pipe` and gRPC's `bufconn` (in-memory `net.Conn`/`net.Listener`) — the fastest, most deterministic option when you don't need a real port ([18](18-testing.md)/[49](49-testing-kinds.md)). For local webhook development, a tunnel (`ngrok`, `cloudflared`) publishes your loopback listener on a public URL.
- **Choosing, in one table.**
  | Situation | Serve it with |
  |---|---|
  | Any HTTP API, containerized | `&http.Server{}` + timeouts on `0.0.0.0:$PORT`, TLS at the ingress |
  | Public internet, no LB | `ListenAndServeTLS` or `autocert`, + a redirect server on `:80` |
  | Local nginx/Caddy in front | Unix socket, or `127.0.0.1:8080` |
  | Privileged port without root | systemd socket activation (or `setcap cap_net_bind_service`) |
  | Zero-downtime restarts on a VM | socket activation / `SO_REUSEPORT` / `tableflip` |
  | Internal service-to-service RPC | gRPC (h2c inside the mesh) |
  | Browser real-time | WebSocket or SSE ([58](58-realtime-websockets-sse.md)) |
  | Non-HTTP protocol | raw TCP accept loop / UDP `ListenPacket` ([63](63-networking-fundamentals.md)) |
  | Event-driven, spiky, cheap-idle | Lambda + adapter; else Cloud Run |
  | Tests | `httptest` / `bufconn` |

## Exercises
1. Rewrite `http.ListenAndServe` as `net.Listen` + `&http.Server{}` + `Serve`, with all five timeouts set. Then bind `:0` and log the real port from `ln.Addr()`.
2. Wrap the listener with `netutil.LimitListener` and prove connection 101 waits; then add a `tls.NewListener` on top of the same listener.
3. Serve HTTPS three ways: `ListenAndServeTLS` with a self-signed cert, a custom `tls.Config` (`MinVersion`, `GetCertificate`), and `autocert` (dry-run against the Let's Encrypt staging endpoint).
4. Serve the same handler on a **Unix socket** and proxy to it from nginx (or from a Go `httputil.ReverseProxy` dialing `unix`); handle the stale-socket case.
5. Implement **systemd socket activation** with `net.FileListener(os.NewFile(3, ...))` and a `.socket` + `.service` unit pair; restart the service under load and show no connection was refused.
6. Enable **`SO_REUSEPORT`** via `net.ListenConfig.Control`, run two processes on one port, and confirm the kernel spreads connections.
7. Add **h2c** with `h2c.NewHandler` and verify with `curl --http2-prior-knowledge`; then add an **HTTP/3** listener with `quic-go/http3` + `Alt-Svc` and verify with `curl --http3`.
8. Run **gRPC and REST on one port** — first with the content-type handler switch under h2c, then with `cmux`. Add a `bufconn` test that needs no port.
9. Write a raw **TCP** line-protocol server and a **UDP** stats receiver in the same binary as your HTTP API, all started with an `errgroup` and shut down on SIGTERM.
10. Split the process into a **public API server** and an **admin server** (`/metrics`, `/debug/pprof`, `/readyz`) on a second port; verify pprof is not reachable on the public one.
11. Put your app behind nginx/Caddy: TLS termination, `X-Forwarded-*`, WebSocket upgrade passthrough, and buffering disabled for SSE. Then reproduce the same routing in-process with `httputil.ReverseProxy`.
12. Serve the same handler over **FastCGI** (`fcgi.Serve`) just to see it work; note what changed in your code (nothing).
13. Deploy the identical handler to **Cloud Run** (container on `$PORT`) and to **Lambda** (via an adapter); measure cold start and note which background work is no longer safe.
14. Capstone: **one binary, five surfaces** — HTTPS/h2 public API (graceful shutdown + connection limit), admin server on a second port, gRPC on a third, a UDP receiver, and a `-healthcheck` mode — wired with `errgroup`, config-driven addresses ([62](62-deployment-operations.md)), and a table in the README saying which listener is which and why.

## Best Practices & Pitfalls
- **Construct the `http.Server` explicitly, with timeouts, always.** `ListenAndServe`/`DefaultServeMux` are for demos ([64](64-http-protocol-internals.md)).
- **Make the listen address configuration**, not a constant. The same binary must bind `0.0.0.0:$PORT` in a container, `127.0.0.1:8080` behind a local proxy, and a Unix socket on a VM ([65](65-docker-networking.md)).
- **Never expose `/debug/pprof` or `/metrics` on the public listener.** Second port, separate mux, and never `DefaultServeMux`.
- **Pair every server with graceful shutdown**, and close hijacked/long-lived connections yourself — `Shutdown` won't.
- **Don't hand-roll TLS/renewal if an ingress can do it.** If you must, `autocert` + `MinVersion: TLS12` beats a manual cert dance.
- **Pitfall — one process, many `Serve` calls, no error propagation.** If the second listener fails to bind, you silently serve a crippled app; use `errgroup` and fail the process.
- **Pitfall — HTTP/3 without UDP.** The TCP port works, h3 silently never negotiates. Same class of bug as a compose file publishing only TCP ([65](65-docker-networking.md)).
- **Pitfall — background goroutines in serverless.** Work started after the response may never run; use a queue ([44](44-background-jobs-queues.md)).
- **Pitfall — reinventing the reverse proxy.** Static files, TLS, rate limiting, and buffering are solved; put a proxy in front and keep your app a handler.
- **Prefer boring:** a container listening on `$PORT` behind an ingress covers ~90% of services. Reach for socket activation, `SO_REUSEPORT`, or `cmux` only when you can name the requirement.

## Checklist
- [ ] I can rebuild `ListenAndServe` from `net.Listen` + `http.Server` and explain each piece.
- [ ] I can wrap a listener (limit, TLS) and serve on a Unix socket or an inherited fd.
- [ ] I can serve HTTPS (manual, custom `tls.Config`, and `autocert`) and know when to skip TLS in-process.
- [ ] I know how to get h2c, HTTP/3, gRPC, gRPC+REST on one port, WebSocket/SSE, FastCGI, and raw TCP/UDP.
- [ ] I run multiple servers in one process safely, with `errgroup` and separate muxes.
- [ ] I know four zero-downtime-restart strategies and which one my deployment already gives me.
- [ ] I can run the same handler on a VM, in a container, on a PaaS, in Lambda, and in tests without changing the handler.
- [ ] I can justify my choice from the decision table instead of copying a snippet.

## Resources
- Stdlib: `http.Server.Serve` https://pkg.go.dev/net/http#Server.Serve · `Server.Shutdown` https://pkg.go.dev/net/http#Server.Shutdown · `net.ListenConfig` https://pkg.go.dev/net#ListenConfig · `net.FileListener` https://pkg.go.dev/net#FileListener · `net/http/fcgi` https://pkg.go.dev/net/http/fcgi · `net/http/httptest` https://pkg.go.dev/net/http/httptest
- Extras: `netutil.LimitListener` https://pkg.go.dev/golang.org/x/net/netutil#LimitListener · `autocert` https://pkg.go.dev/golang.org/x/crypto/acme/autocert · `h2c` https://pkg.go.dev/golang.org/x/net/http2/h2c · `quic-go/http3` https://pkg.go.dev/github.com/quic-go/quic-go/http3 · `cmux` https://github.com/soheilhy/cmux · `tableflip` https://github.com/cloudflare/tableflip · `go-systemd/activation` https://pkg.go.dev/github.com/coreos/go-systemd/v22/activation
- Serverless: AWS Lambda Go https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html · Lambda Web Adapter https://github.com/awslabs/aws-lambda-web-adapter · Cloud Run container contract https://cloud.google.com/run/docs/container-contract
- Reverse proxies: nginx `proxy_pass` https://nginx.org/en/docs/http/ngx_http_proxy_module.html · Caddy https://caddyserver.com/docs/ · Traefik https://doc.traefik.io/traefik/
- Examples: [examples/66-serving-go-apps](examples/66-serving-go-apps/).
- Related in this plan: listeners & sockets in [63](63-networking-fundamentals.md); the server/client machinery in [64](64-http-protocol-internals.md); container plumbing in [65](65-docker-networking.md); deploy targets & graceful shutdown in [62](62-deployment-operations.md); gRPC in [27](27-grpc-microservices.md); WebSocket/SSE in [58](58-realtime-websockets-sse.md); test servers in [49](49-testing-kinds.md).
