# Step 64 — The HTTP Protocol & `net/http` Internals · Examples

A library of **26 examples**, split into three files by difficulty.

Every example is **runnable Go** with a real **Output** — `gofmt`ed, `go vet`ed and run before being added. Each program starts its own server *and* client in a single `main` (via `httptest` or `net.Listen` on `127.0.0.1:0`), so you can `go run main.go` and see both ends of the exchange with nothing else installed.

**Stdlib only** — `net/http`, `net/http/httptest`, `net/http/httputil`, `crypto/tls`, `crypto/x509`. HTTP/2 over TLS is in the standard library, so even the h2 examples need no dependencies. (Cleartext h2c and HTTP/3 do need external packages — they live in [lesson 66](../66-serving-go-apps/).)

Several examples deliberately drop below `net/http` and speak HTTP over a raw TCP socket, which is the layer taught in [lesson 63](../63-networking-fundamentals/).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — the wire format & the response contract

- [1. HTTP/1.1 by hand over a raw socket](1-easy.md#1-http11-by-hand-over-a-raw-socket)
- [2. Dumping requests and responses](1-easy.md#2-dumping-requests-and-responses)
- [3. Content-Length vs chunked](1-easy.md#3-content-length-vs-chunked)
- [4. Chunked encoding, decoded by hand](1-easy.md#4-chunked-encoding-decoded-by-hand)
- [5. Build the server explicitly — the five timeouts](1-easy.md#5-build-the-server-explicitly--the-five-timeouts)
- [6. Headers must be set before the first Write](1-easy.md#6-headers-must-be-set-before-the-first-write)
- [7. Content-Type sniffing](1-easy.md#7-content-type-sniffing)
- [8. Everything the server knows about a request](1-easy.md#8-everything-the-server-knows-about-a-request)

### 🟡 [Medium](2-medium.md) — server internals & the client pool

- [9. ConnState: the accept loop, observed](2-medium.md#9-connstate-the-accept-loop-observed)
- [10. Keep-alive: 10 requests, 1 connection](2-medium.md#10-keep-alive-10-requests-1-connection)
- [11. ReadHeaderTimeout defeats the slow loris](2-medium.md#11-readheadertimeout-defeats-the-slow-loris)
- [12. WriteTimeout breaks streaming — ResponseController fixes it](2-medium.md#12-writetimeout-breaks-streaming--responsecontroller-fixes-it)
- [13. Flusher: sending bytes before the handler returns](2-medium.md#13-flusher-sending-bytes-before-the-handler-returns)
- [14. Hijacker: taking the raw connection](2-medium.md#14-hijacker-taking-the-raw-connection)
- [15. The Transport is a connection pool](2-medium.md#15-the-transport-is-a-connection-pool)
- [16. Drain the body, or lose the connection](2-medium.md#16-drain-the-body-or-lose-the-connection)
- [17. Client timeouts: blanket vs per-request context](2-medium.md#17-client-timeouts-blanket-vs-per-request-context)

### 🔴 [Hard](3-hard.md) — TLS, HTTP/2, proxies & middleware

- [18. TLS end to end with a generated certificate](3-hard.md#18-tls-end-to-end-with-a-generated-certificate)
- [19. ALPN: where HTTP/2 actually comes from](3-hard.md#19-alpn-where-http2-actually-comes-from)
- [20. Multiplexing: 20 concurrent requests, 1 connection](3-hard.md#20-multiplexing-20-concurrent-requests-1-connection)
- [21. Head-of-line blocking, measured](3-hard.md#21-head-of-line-blocking-measured)
- [22. TimeoutHandler: a real 503 instead of a dropped connection](3-hard.md#22-timeouthandler-a-real-503-instead-of-a-dropped-connection)
- [23. X-Forwarded-For: trust only your own hop](3-hard.md#23-x-forwarded-for-trust-only-your-own-hop)
- [24. A reverse proxy in ten lines](3-hard.md#24-a-reverse-proxy-in-ten-lines)
- [25. Wrapping ResponseWriter without breaking Flush](3-hard.md#25-wrapping-responsewriter-without-breaking-flush)
- [26. Capstone: a server built properly, and a client that talks to it right](3-hard.md#26-capstone-a-server-built-properly-and-a-client-that-talks-to-it-right)

## The five things worth memorising

1. **Set the five server timeouts** (ex. 5) — the zero-value `http.Server` has none.
2. **Headers are frozen after the first `Write`** (ex. 6) — a silent no-op otherwise.
3. **Share one `http.Client`, drain and close every body** (ex. 15, 16) — otherwise the pool never works.
4. **`WriteTimeout` breaks streaming**; use `ResponseController` per request (ex. 12).
5. **`Unwrap()` on any `ResponseWriter` wrapper** (ex. 25) — or middleware quietly kills `Flush`/`Hijack`.

## A note on output stability

Some outputs contain **ephemeral ports**, `Date` headers, log timestamps and rounded millisecond timings — those differ per run and per machine. The counts, statuses and byte totals are the stable part.

## Related

- Lesson: [64 — The HTTP Protocol & `net/http` Internals](../../64-http-protocol-internals.md)
- The layer below: [63 — Networking Fundamentals](../../63-networking-fundamentals.md) · [examples](../63-networking-fundamentals/)
- In containers: [65 — Container & Docker Networking](../../65-docker-networking.md) · Serving it: [66 — Every Way to Serve a Go App](../../66-serving-go-apps.md)
- Handler basics: [20](../../20-http-server.md)/[21](../../21-rest-api.md) · Hardening: [57](../../57-web-security.md) · Streaming: [58](../../58-realtime-websockets-sse.md)
