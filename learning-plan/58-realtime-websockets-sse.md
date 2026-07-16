# 58 — Real-Time: WebSockets & Server-Sent Events

> Part of **Part 12 — Production Web App Concerns**. Builds on [14 — Channels](14-channels.md) and [15 — Sync & Context](15-sync-context.md) (the hub is a channel/select machine), [20](20-http-server.md)–[21 — HTTP](21-rest-api.md), the JSON-envelope idea from [55 — Data Pipelines](55-data-pipelines.md), and pairs with [57 — Web Security](57-web-security.md) (Origin checks, message limits) and [36 — Resilience](36-resilience-patterns.md) (backpressure). Grows up the SSE you saw in the `watch-hub` project. Thesis: **real-time is two layers — a transport (SSE for one-way, WebSocket for two-way) and an application (a single-goroutine hub that fans messages out to clients). Get the transport's framing and the hub's backpressure right, put a pub/sub backplane behind it, and you can scale to many instances.**

## Goals
- Stream one-way updates with **Server-Sent Events**: `text/event-stream`, `Flusher`, the `id`/`event`/`data`/`retry` fields, disconnect handling, and **`Last-Event-ID`** resume.
- Understand **WebSockets from the wire up**: the RFC 6455 handshake (`Sec-WebSocket-Accept`), the **frame format**, **client-side masking**, and control frames (**ping/pong/close**).
- Build the **hub**: a single goroutine that owns the client set and **broadcasts**, with **backpressure** (drop slow clients), **rooms**, and a **JSON message protocol**.
- Make it production-grade: **one writer per connection**, an **Origin** check, **per-connection limits**, **heartbeats/idle timeout**, **graceful shutdown**, and **scaling across instances with a pub/sub backplane**.
- Know **when to use which**: SSE vs WebSocket.

## Concepts

- **SSE is HTTP that never ends.** Set `Content-Type: text/event-stream`, then write `data: …\n\n` frames and **`Flush`** after each. It's **one-way** (server→client), rides plain HTTP (proxies/HTTP-2 friendly), and the browser **auto-reconnects**. An event may carry an `id:` (for resume), an `event:` name (a type), multiple `data:` lines, and a `retry:` hint; a **blank line ends the event**.
- **A handler must stop when the client leaves.** Loop on `select { case <-r.Context().Done(): return; case ev := <-events: … }`. Forgetting this leaks a goroutine per dropped connection.
- **SSE resumes with `Last-Event-ID`.** On reconnect the browser sends the last `id` it saw; the server replays everything after it. That's why you put an `id:` on each event.
- **The WebSocket handshake is a computed HTTP upgrade.** The client sends `Upgrade: websocket` + a random `Sec-WebSocket-Key`; the server replies **`101 Switching Protocols`** with `Sec-WebSocket-Accept = base64(sha1(key + "258EAFA5-…"))`. After that the TCP connection speaks the WebSocket framing protocol, not HTTP — in Go you `Hijack()` the connection to take it over.
- **A frame is a tiny binary header + payload.** Byte 0 = `FIN` bit + 4-bit **opcode** (`0x1` text, `0x2` binary, `0x8` close, `0x9` ping, `0xA` pong). Byte 1 = **mask bit** + 7-bit length (with 16/64-bit extensions for larger payloads). **Client→server frames MUST be masked**: each payload byte is XORed with a cycling 4-byte key (server→client frames are not). Control frames (close/ping/pong) are small and carry an optional payload.
- **A WebSocket connection needs exactly one writer.** Concurrent goroutines writing frames interleave bytes and corrupt the stream. The fix: a per-connection **send channel** drained by a single writer goroutine; everyone else just sends to the channel.
- **The hub is a single-goroutine actor.** One goroutine owns `map[*Client]bool` and a `select` over `register`/`unregister`/`broadcast` — so **no mutex** is needed. Broadcasting is a fan-out to each client's send channel.
- **Broadcast with backpressure.** Send to each client **non-blockingly** (`select { case c.send <- msg: default: drop/disconnect }`). Without this, one slow client blocks the whole hub. Cap each client's buffer and drop (or close) when it's full.
- **Structure and secure the messages.** Use a **JSON envelope** (`{type, payload}`) dispatched by `type`. **Check the `Origin` header** on the handshake — WebSockets bypass same-origin/CORS, so this is their CSRF defense. Enforce **message-size and rate limits** per connection. Send **ping** periodically and expect **pong**; too many missed pongs ⇒ close (idle detection). On shutdown, send a **close frame** (status `1001`) to each client.
- **Scale out with a backplane.** With many server instances, a client on instance A won't see a message published on instance B unless the instances share a bus. Each instance publishes to a **pub/sub backplane** (Redis Pub/Sub in production) and rebroadcasts locally what it receives. Program the hub against a small `Backplane` interface so the in-memory version and Redis are interchangeable.
- **SSE vs WebSocket.** SSE: one-way, text, plain HTTP, auto-reconnect, dead simple — feeds, notifications, progress, live dashboards. WebSocket: bidirectional, binary-capable, low latency — chat, presence, games, collaborative editing. Pick the simplest transport that fits.

## Exercises
1. Write an SSE handler that streams three `data:` events; consume it with an HTTP client.
2. Emit a full SSE event with `id`/`event`/`retry` and multiline `data`.
3. Make an SSE loop that stops on `ctx.Done()` (client disconnect) and returns how many it sent.
4. Compute `Sec-WebSocket-Accept` for the RFC key; then build the full `101` upgrade response.
5. Encode a server text frame; mask/unmask a payload; decode a masked client frame.
6. Stand up a **real** echo: `Hijack`, complete the handshake, read one masked frame, echo it — and connect with a raw client.
7. Read frames in a loop and handle the **close** opcode; build **ping/pong** control frames.
8. Show the corruption risk of multiple writers, then fix it with one writer + a send channel.
9. Build the hub (register/unregister/broadcast as a single-goroutine actor); add **backpressure** (drop a slow client).
10. Add **rooms** (scoped broadcast) and a **JSON envelope** dispatched by type.
11. Add an **Origin** allowlist, **per-connection** size/rate limits, a **heartbeat** (missed-pong close), and **graceful shutdown** (close frames).
12. Build an in-process **pub/sub broker**, then a **backplane** two instances share so a message on A reaches B.
13. Add **presence** (join/leave) and **`Last-Event-ID`** resume.
14. Capstone: a chat hub — rooms + presence + JSON messages + broadcast — driven with simulated clients.

## Best Practices & Pitfalls
- **One writer per connection, always.** Route all writes through a send channel; never let two goroutines write frames concurrently.
- **Stop on disconnect.** Tie every streaming loop to `r.Context()`/a done channel, or you leak a goroutine per dropped client.
- **Pitfall — a slow client stalls everyone.** Non-blocking broadcast + bounded per-client buffers; drop or disconnect laggards.
- **Pitfall — skipping the Origin check.** WebSockets ignore CORS; without an `Origin` allowlist any site can open a socket to your server with the user's cookies.
- **Pitfall — unbounded messages.** Cap frame/message size and rate per connection ([57](57-web-security.md)); a client can otherwise exhaust memory.
- **Pitfall — no heartbeat.** TCP can look "up" long after a peer vanishes; ping/pong with a missed-pong timeout is how you actually detect dead connections.
- **Pitfall — one instance only.** The moment you run two replicas, in-memory broadcast breaks; put a pub/sub backplane behind the hub from the start (behind an interface).
- **Prefer SSE when one-way suffices** — it's less code, survives proxies, and reconnects itself. Reach for WebSocket only when you truly need bidirectional/low-latency.
- **In real projects, use a maintained library** (`coder/websocket`, `gorilla/websocket`) rather than hand-rolled framing — but knowing the frame format is what lets you debug it.

## Checklist
- [ ] I can stream SSE (`text/event-stream` + `Flush`), emit the full event fields, stop on disconnect, and resume via `Last-Event-ID`.
- [ ] I can compute the handshake accept key, build the `101` response, and hijack the connection.
- [ ] I can encode/decode frames, mask/unmask, and handle close/ping/pong control frames.
- [ ] I route all writes through one writer goroutine per connection.
- [ ] I built a single-goroutine hub with non-blocking broadcast, backpressure, and rooms.
- [ ] I check `Origin`, enforce size/rate limits, heartbeat, and shut down gracefully.
- [ ] I can scale across instances with a pub/sub backplane behind an interface, and I know when to pick SSE vs WebSocket.

## Resources
- WHATWG SSE / `EventSource` spec: https://html.spec.whatwg.org/multipage/server-sent-events.html · MDN: https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
- WebSocket protocol (RFC 6455): https://datatracker.ietf.org/doc/html/rfc6455
- `net/http` `Flusher`/`Hijacker`: https://pkg.go.dev/net/http#Flusher · https://pkg.go.dev/net/http#Hijacker
- Libraries: `coder/websocket` https://pkg.go.dev/github.com/coder/websocket · `gorilla/websocket` https://pkg.go.dev/github.com/gorilla/websocket
- Examples: [examples/58-realtime-websockets-sse](examples/58-realtime-websockets-sse/).
- Related in this plan: channels/select in [14](14-channels.md); the hub actor pattern & context in [15](15-sync-context.md); Origin/limits in [57](57-web-security.md); JSON envelopes in [55](55-data-pipelines.md); backpressure/resilience in [36](36-resilience-patterns.md).
