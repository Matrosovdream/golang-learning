# Step 58 — Real-Time: WebSockets & SSE · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

A **live** WebSocket echo over TCP, control frames, and the application layer: **one writer per conn**, the **hub**, **backpressure**, **rooms**, and a **JSON protocol**.

---

## 9. A live WebSocket echo

`🟡 medium` · *websocket*

Everything from the easy tier, wired into a real connection: the server **hijacks** the TCP conn, writes the `101` handshake, reads one masked frame, and echoes it back unmasked; a raw client sends the upgrade request and a masked `"hello"`. This is a complete (if minimal) WebSocket round-trip in stdlib.

**Steps:**

1. `w.(http.Hijacker).Hijack()` takes over the connection after the handshake.
2. `readFrame` unmasks the client's text; `writeText` sends an unmasked reply.
3. The client reads the echoed frame.

```go
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func accept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// readFrame reads one text frame (masked or not) and returns its payload.
func readFrame(r *bufio.Reader) (string, error) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return "", err
	}
	masked := hdr[1]&0x80 != 0
	length := int(hdr[1] & 0x7f)
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return "", err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return "", err
	}
	if masked {
		for i := range payload {
			payload[i] ^= key[i%4]
		}
	}
	return string(payload), nil
}

func writeText(w *bufio.Writer, s string) {
	w.Write([]byte{0x81, byte(len(s))}) // server frames are unmasked
	w.WriteString(s)
	w.Flush()
}

// wsHandler hijacks the TCP connection, completes the handshake, then echoes one frame.
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, buf, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	buf.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n")
	buf.Flush()
	msg, err := readFrame(buf.Reader)
	if err != nil {
		return
	}
	writeText(buf.Writer, "echo: "+msg)
}

func runClient(addr string) (string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\n"+
		"Connection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n")
	br := bufio.NewReader(conn)
	for { // consume the 101 response headers up to the blank line
		line, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		if line == "\r\n" {
			break
		}
	}
	// send a masked "hello" text frame
	key := []byte{0x11, 0x22, 0x33, 0x44}
	payload := []byte("hello")
	frame := []byte{0x81, byte(0x80 | len(payload)), key[0], key[1], key[2], key[3]}
	for i, b := range payload {
		frame = append(frame, b^key[i%4])
	}
	conn.Write(frame)
	return readFrame(br)
}

func main() {
	srv := httptest.NewServer(http.HandlerFunc(wsHandler))
	defer srv.Close()
	msg, err := runClient(srv.Listener.Addr().String())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("client received:", msg)
}
```

**Output:**

```
client received: echo: hello
```

---

## 10. Frame read loop and close

`🟡 medium` · *framing*

A connection sends a stream of frames until it closes. Read them in a loop, dispatching on the opcode; the **close** frame (`0x8`) means "we're done" — reply/stop and tear down. Here `net.Pipe` gives two connected ends in-process (no network needed).

**Steps:**

1. `readFrame` returns the opcode and payload.
2. Loop until an error or a close frame.
3. Two text frames arrive, then the close.

```go
package main

import (
	"fmt"
	"io"
	"net"
)

func readFrame(r io.Reader) (opcode byte, payload string, err error) {
	hdr := make([]byte, 2)
	if _, err = io.ReadFull(r, hdr); err != nil {
		return
	}
	opcode = hdr[0] & 0x0f
	length := int(hdr[1] & 0x7f)
	buf := make([]byte, length)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	return opcode, string(buf), nil
}

func main() {
	c1, c2 := net.Pipe()
	go func() {
		c1.Write(append([]byte{0x81, 0x01}, 'a')) // text
		c1.Write(append([]byte{0x81, 0x01}, 'b')) // text
		c1.Write([]byte{0x88, 0x00})              // close (opcode 0x8)
		c1.Close()
	}()
	for {
		op, msg, err := readFrame(c2)
		if err != nil {
			break
		}
		if op == 0x8 {
			fmt.Println("recv: <close>")
			break
		}
		fmt.Printf("recv: %q (opcode %#x)\n", msg, op)
	}
}
```

**Output:**

```
recv: "a" (opcode 0x1)
recv: "b" (opcode 0x1)
recv: <close>
```

---

## 11. Ping/pong control frames

`🟡 medium` · *keepalive*

WebSocket has built-in liveness: the server sends a **Ping** (opcode `0x9`); the peer must reply with a **Pong** (`0xA`) echoing the same payload. Control frames set the `FIN` bit and carry a short optional payload. Missed pongs are how you detect a dead peer (example 25).

**Steps:**

1. `controlFrame(op, payload)` sets `FIN` + the control opcode.
2. Build a Ping with a payload.
3. The Pong reply carries the identical payload.

```go
package main

import "fmt"

// Control frames keep a connection alive: the server sends a Ping (0x9); the peer must
// reply with a Pong (0xA) echoing the same payload. Missed pongs => the peer is gone.
const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

func controlFrame(opcode byte, payload string) []byte {
	return append([]byte{0x80 | opcode, byte(len(payload))}, payload...)
}

func main() {
	ping := controlFrame(opPing, "keepalive")
	fmt.Printf("ping frame: % x  (opcode %#x)\n", ping, ping[0]&0x0f)

	pong := controlFrame(opPong, "keepalive") // reply echoes the same payload
	fmt.Printf("pong frame: % x  (opcode %#x)\n", pong, pong[0]&0x0f)
}
```

**Output:**

```
ping frame: 89 09 6b 65 65 70 61 6c 69 76 65  (opcode 0x9)
pong frame: 8a 09 6b 65 65 70 61 6c 69 76 65  (opcode 0xa)
```

---

## 12. One writer per connection

`🟡 medium` · *concurrency*

The single biggest WebSocket footgun: **two goroutines writing to the same connection** interleave frame bytes and corrupt the stream. The fix is structural — a per-connection **send channel** drained by exactly one writer goroutine; every producer just sends to the channel.

**Steps:**

1. `Conn` has a `send chan string`; one goroutine ranges over it.
2. Many producers send to `c.send` concurrently — never to the socket directly.
3. All 5 arrive (order races, so sort for a stable print).

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

// A WebSocket connection must have a SINGLE writer — concurrent writes interleave
// frame bytes and corrupt the stream. The fix: a per-connection send channel drained
// by exactly one goroutine.
type Conn struct {
	send chan string
}

func main() {
	c := &Conn{send: make(chan string, 16)}
	var got []string
	done := make(chan struct{})

	go func() { // the ONE writer
		for msg := range c.send {
			got = append(got, msg)
		}
		close(done)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ { // many producers, all just send to the channel
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.send <- fmt.Sprintf("msg-%d", n)
		}(i)
	}
	wg.Wait()
	close(c.send)
	<-done

	sort.Strings(got) // producers race, so sort for a stable print
	fmt.Println("delivered:", got)
}
```

**Output:**

```
delivered: [msg-0 msg-1 msg-2 msg-3 msg-4]
```

---

## 13. The hub pattern

`🟡 medium` · *hub*

A real-time server needs to broadcast to many connections. The idiomatic Go design is a **hub**: one goroutine owns the client set and a `select` over `register`/`unregister`/`broadcast`. Because a single goroutine touches the map, there's **no mutex** — the channels serialize everything.

**Steps:**

1. The hub's `run` loop owns `clients` and selects over the three channels.
2. Register two clients, broadcast, unregister one, broadcast again.
3. Signal `quit` and wait for `stopped` so all mutations are flushed before reading.

```go
package main

import "fmt"

// The hub owns the set of clients and fans a message out to all of them. A single
// goroutine (select loop) owns the map, so NO mutex is needed.
type Client struct {
	id   int
	recv []string
}

type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan string
	quit       chan struct{}
	clients    map[*Client]bool
}

func newHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan string),
		quit:       make(chan struct{}),
		clients:    map[*Client]bool{},
	}
}

func (h *Hub) run(stopped chan struct{}) {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
		case c := <-h.unregister:
			delete(h.clients, c)
		case msg := <-h.broadcast:
			for c := range h.clients {
				c.recv = append(c.recv, msg)
			}
		case <-h.quit:
			close(stopped)
			return
		}
	}
}

func main() {
	h := newHub()
	stopped := make(chan struct{})
	go h.run(stopped)

	a, b := &Client{id: 1}, &Client{id: 2}
	h.register <- a
	h.register <- b
	h.broadcast <- "hello"
	h.unregister <- a
	h.broadcast <- "world" // only b is still registered

	close(h.quit)
	<-stopped // all hub mutations are now flushed

	fmt.Printf("client 1 got: %v\n", a.recv)
	fmt.Printf("client 2 got: %v\n", b.recv)
}
```

**Output:**

```
client 1 got: [hello]
client 2 got: [hello world]
```

---

## 14. Backpressure and slow clients

`🟡 medium` · *backpressure*

If the hub broadcasts with a **blocking** send, one slow client stalls everyone. The fix: send to each client's **buffered** channel **non-blockingly** (`select … default`), and drop (or disconnect) a client whose buffer is full. A fast client keeps up; a slow one loses messages instead of freezing the server.

**Steps:**

1. `select { case c.send <- msg: default: c.dropped++ }`.
2. The fast client (big buffer) receives everything.
3. The slow client (small buffer, never drained) drops the overflow.

```go
package main

import "fmt"

// Backpressure: broadcast with a NON-BLOCKING send to each client's buffered channel.
// If a client's buffer is full (a slow reader), drop the message (or disconnect them)
// instead of blocking the whole hub on one slow client.
type Client struct {
	id      int
	send    chan string
	dropped int
}

func broadcast(clients []*Client, msg string) {
	for _, c := range clients {
		select {
		case c.send <- msg:
		default:
			c.dropped++ // buffer full -> drop, don't block everyone
		}
	}
}

func main() {
	fast := &Client{id: 1, send: make(chan string, 8)}
	slow := &Client{id: 2, send: make(chan string, 2)} // small buffer, never drained

	for i := 0; i < 5; i++ {
		broadcast([]*Client{fast, slow}, fmt.Sprintf("m%d", i))
	}
	fmt.Printf("fast: buffered=%d dropped=%d\n", len(fast.send), fast.dropped)
	fmt.Printf("slow: buffered=%d dropped=%d\n", len(slow.send), slow.dropped)
}
```

**Output:**

```
fast: buffered=5 dropped=0
slow: buffered=2 dropped=3
```

---

## 15. Rooms and channels

`🟡 medium` · *rooms*

Most apps don't broadcast to *everyone* — they scope messages to a **room** (a chat channel, a document, a game lobby). Model it as `map[room]set-of-clients`; a broadcast reaches only that room's members.

**Steps:**

1. `join(room, client)` adds to the room's set.
2. `members(room)` returns the room's clients (sorted for a stable print).
3. Client 3 is in both rooms; the `go` room has three members.

```go
package main

import (
	"fmt"
	"sort"
)

// Rooms/channels: clients subscribe to named rooms; a broadcast reaches only the
// members of that room. A map of room -> set of client ids.
type Rooms struct {
	m map[string]map[int]bool
}

func newRooms() *Rooms { return &Rooms{m: map[string]map[int]bool{}} }

func (r *Rooms) join(room string, client int) {
	if r.m[room] == nil {
		r.m[room] = map[int]bool{}
	}
	r.m[room][client] = true
}

func (r *Rooms) members(room string) []int {
	var out []int
	for c := range r.m[room] {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

func main() {
	r := newRooms()
	r.join("go", 1)
	r.join("go", 2)
	r.join("rust", 3)
	r.join("go", 3)

	fmt.Println("room go:  ", r.members("go"))
	fmt.Println("room rust:", r.members("rust"))
}
```

**Output:**

```
room go:   [1 2 3]
room rust: [3]
```

---

## 16. A JSON message protocol

`🟡 medium` · *protocol*

Raw text frames get unmanageable fast. Give your messages structure: a **JSON envelope** with a `type` discriminator and a `payload` you decode per type — the tagged-union pattern from [step 55](../55-data-pipelines/). The read loop unmarshals the envelope, then dispatches on `type`.

**Steps:**

1. `Envelope{Type, Payload json.RawMessage}`.
2. Unmarshal the envelope, switch on `Type`.
3. Decode the raw payload into the concrete message struct.

```go
package main

import (
	"encoding/json"
	"fmt"
)

// A message protocol over WS: a JSON envelope with a "type" discriminator and a raw
// payload decoded per type (the tagged-union pattern from step 55).
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ChatMsg struct {
	Room string `json:"room"`
	Text string `json:"text"`
}

func main() {
	raw := `{"type":"chat","payload":{"room":"go","text":"hi"}}`
	var env Envelope
	json.Unmarshal([]byte(raw), &env)

	switch env.Type {
	case "chat":
		var m ChatMsg
		json.Unmarshal(env.Payload, &m)
		fmt.Printf("chat in #%s: %s\n", m.Room, m.Text)
	default:
		fmt.Println("unknown type:", env.Type)
	}
}
```

**Output:**

```
chat in #go: hi
```

---

## 17. SSE vs WebSocket

`🟡 medium` · *design*

Both give you real-time, but they're not interchangeable. **SSE** is one-way, text, over plain HTTP, and auto-reconnects — ideal for feeds, notifications, and progress. **WebSocket** is bidirectional, binary-capable, and lower-latency — needed for chat, presence, games, and collaboration. Pick the simplest one that fits.

**Steps:**

1. Compare the two on direction, transport, reconnect, and binary support.
2. SSE wins on simplicity; WebSocket wins on interactivity.
3. Rule of thumb below.

```go
package main

import "fmt"

// Choosing a real-time transport.
type transport struct {
	name          string
	direction     string
	overHTTP      bool
	autoReconnect bool
	binary        bool
}

func main() {
	sse := transport{"SSE", "server->client", true, true, false}
	ws := transport{"WebSocket", "bidirectional", false, false, true}
	for _, t := range []transport{sse, ws} {
		fmt.Printf("%-10s dir=%-14s plainHTTP=%-5v auto-reconnect=%-5v binary=%v\n",
			t.name, t.direction, t.overHTTP, t.autoReconnect, t.binary)
	}
	fmt.Println("rule: one-way feeds/notifications -> SSE; two-way chat/games -> WebSocket")
}
```

**Output:**

```
SSE        dir=server->client plainHTTP=true  auto-reconnect=true  binary=false
WebSocket  dir=bidirectional  plainHTTP=false auto-reconnect=false binary=true
rule: one-way feeds/notifications -> SSE; two-way chat/games -> WebSocket
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
