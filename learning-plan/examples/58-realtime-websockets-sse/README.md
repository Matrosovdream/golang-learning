# Step 58 — Real-Time: WebSockets & SSE · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. **Stdlib-only.** SSE uses `net/http`; the WebSocket handshake + **frame encode/decode/masking** are **hand-rolled from `crypto/sha1`, `net`, and `bufio`** (RFC 6455) so you see the wire protocol — including a real echo over a live TCP connection (ex 9). The hub / rooms / backplane patterns are at the message layer (channels), so real code just swaps the transport (or a `coder/websocket` library) underneath.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Server-Sent Events (SSE) basics](1-easy.md#1-server-sent-events-sse-basics)
- [2. The SSE event format](1-easy.md#2-the-sse-event-format)
- [3. SSE and client disconnect](1-easy.md#3-sse-and-client-disconnect)
- [4. The WebSocket handshake key](1-easy.md#4-the-websocket-handshake-key)
- [5. The WebSocket upgrade response](1-easy.md#5-the-websocket-upgrade-response)
- [6. Encode a WebSocket frame](1-easy.md#6-encode-a-websocket-frame)
- [7. Frame masking](1-easy.md#7-frame-masking)
- [8. Decode a masked frame](1-easy.md#8-decode-a-masked-frame)

### 🟡 [Medium](2-medium.md)

- [9. A live WebSocket echo](2-medium.md#9-a-live-websocket-echo)
- [10. Frame read loop and close](2-medium.md#10-frame-read-loop-and-close)
- [11. Ping/pong control frames](2-medium.md#11-pingpong-control-frames)
- [12. One writer per connection](2-medium.md#12-one-writer-per-connection)
- [13. The hub pattern](2-medium.md#13-the-hub-pattern)
- [14. Backpressure and slow clients](2-medium.md#14-backpressure-and-slow-clients)
- [15. Rooms and channels](2-medium.md#15-rooms-and-channels)
- [16. A JSON message protocol](2-medium.md#16-a-json-message-protocol)
- [17. SSE vs WebSocket](2-medium.md#17-sse-vs-websocket)

### 🔴 [Hard](3-hard.md)

- [18. Check the Origin header](3-hard.md#18-check-the-origin-header)
- [19. An in-process pub/sub broker](3-hard.md#19-an-in-process-pubsub-broker)
- [20. Scaling with a backplane](3-hard.md#20-scaling-with-a-backplane)
- [21. Presence tracking](3-hard.md#21-presence-tracking)
- [22. Per-connection limits](3-hard.md#22-per-connection-limits)
- [23. Graceful shutdown](3-hard.md#23-graceful-shutdown)
- [24. SSE resume with Last-Event-ID](3-hard.md#24-sse-resume-with-last-event-id)
- [25. Heartbeat and idle timeout](3-hard.md#25-heartbeat-and-idle-timeout)
- [26. Capstone: a chat hub](3-hard.md#26-capstone-a-chat-hub)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
