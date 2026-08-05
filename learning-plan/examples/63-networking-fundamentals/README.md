# Step 63 — Networking Fundamentals · Examples

A library of **26 examples**, split into three files by difficulty.

Every example is **runnable Go** with a real **Output** — each program was `gofmt`ed, `go vet`ed and run before it was added here.
Each one starts its own server *and* its own client in a single `main`, listening on `127.0.0.1:0`, so you can `go run main.go` and watch both sides of the conversation with nothing else installed and no ports to reserve.

Stdlib only — `net`, `bufio`, `io`, `encoding/binary`, `syscall`. No HTTP anywhere: this is the layer *underneath* [lesson 64](../64-http-protocol-internals/).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — sockets, the accept loop & framing

- [1. Your first TCP server](1-easy.md#1-your-first-tcp-server)
- [2. A TCP client with net.Dial](1-easy.md#2-a-tcp-client-with-netdial)
- [3. One goroutine per connection](1-easy.md#3-one-goroutine-per-connection)
- [4. Port :0 — let the kernel choose](1-easy.md#4-port-0--let-the-kernel-choose)
- [5. The 4-tuple: why one port serves many clients](1-easy.md#5-the-4-tuple-why-one-port-serves-many-clients)
- [6. TCP has no message boundaries](1-easy.md#6-tcp-has-no-message-boundaries)
- [7. Framing I — newline-delimited](1-easy.md#7-framing-i--newline-delimited)
- [8. Framing II — length-prefixed](1-easy.md#8-framing-ii--length-prefixed)

### 🟡 [Medium](2-medium.md) — lifecycle, deadlines, shutdown & UDP

- [9. EOF means the peer closed](2-medium.md#9-eof-means-the-peer-closed)
- [10. Half-close with CloseWrite](2-medium.md#10-half-close-with-closewrite)
- [11. Read deadlines and net.Error](2-medium.md#11-read-deadlines-and-neterror)
- [12. An idle timeout that drops silent clients](2-medium.md#12-an-idle-timeout-that-drops-silent-clients)
- [13. Unblocking a stuck Read by closing](2-medium.md#13-unblocking-a-stuck-read-by-closing)
- [14. Graceful shutdown of a TCP server](2-medium.md#14-graceful-shutdown-of-a-tcp-server)
- [15. Capping concurrent connections](2-medium.md#15-capping-concurrent-connections)
- [16. UDP: datagrams keep their boundaries](2-medium.md#16-udp-datagrams-keep-their-boundaries)
- [17. The UDP truncation trap](2-medium.md#17-the-udp-truncation-trap)

### 🔴 [Hard](3-hard.md) — other transports, error triage & the netpoller

- [18. Unix domain sockets](3-hard.md#18-unix-domain-sockets)
- [19. Dialing with a context](3-hard.md#19-dialing-with-a-context)
- [20. DNS lookups you can cancel](3-hard.md#20-dns-lookups-you-can-cancel)
- [21. Reading network errors: refused / timeout / EOF / reset](3-hard.md#21-reading-network-errors-refused--timeout--eof--reset)
- [22. Connection reuse vs dial-per-request](3-hard.md#22-connection-reuse-vs-dial-per-request)
- [23. Socket options: Dialer, ListenConfig and Control](3-hard.md#23-socket-options-dialer-listenconfig-and-control)
- [24. 2000 connections, 2000 goroutines](3-hard.md#24-2000-connections-2000-goroutines)
- [25. A TCP proxy in two io.Copy calls](3-hard.md#25-a-tcp-proxy-in-two-iocopy-calls)
- [26. Capstone: a line-protocol key-value server](3-hard.md#26-capstone-a-line-protocol-key-value-server)

## A note on output stability

A few examples print addresses that contain **ephemeral ports** (`127.0.0.1:55100`), and example 24 prints goroutine and heap numbers. Those differ on every run and every machine — the surrounding assertions (`true`/`false`, counts, byte totals) are the stable part.

## Related

- Lesson: [63 — Networking Fundamentals](../../63-networking-fundamentals.md)
- Next layer up: [64 — The HTTP Protocol & `net/http` Internals](../../64-http-protocol-internals.md)
- In containers: [65 — Container & Docker Networking](../../65-docker-networking.md) · Serving: [66 — Every Way to Serve a Go App](../../66-serving-go-apps.md)
