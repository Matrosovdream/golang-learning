# Step 58 — Real-Time: WebSockets & SSE · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

The two transports from the wire up: **SSE** (HTTP that never ends) and the **WebSocket** handshake + **frame** format.

---

## 1. Server-Sent Events (SSE) basics

`🟢 easy` · *sse*

SSE is just a long-lived HTTP response with `Content-Type: text/event-stream`. You write `data: …` lines followed by a blank line, and **`Flush`** after each so the client gets it immediately. One-way (server→client), rides plain HTTP, and browsers reconnect automatically.

**Steps:**

1. Set the content type and grab `w.(http.Flusher)`.
2. Write `data: …\n\n` and `Flush()` per event.
3. A client reads the stream line by line.

```go
package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
)

func main() {
	// SSE (Server-Sent Events): keep the response open, write "data:" lines, and Flush
	// after each. The Content-Type must be text/event-stream.
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "data: event %d\n\n", i)
			flusher.Flush()
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println("Content-Type:", resp.Header.Get("Content-Type"))
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			fmt.Println("recv:", line)
		}
	}
}
```

**Output:**

```
Content-Type: text/event-stream
recv: data: event 1
recv: data: event 2
recv: data: event 3
```

---

## 2. The SSE event format

`🟢 easy` · *sse*

An SSE event is more than `data:`. It can carry an **`id:`** (used to resume after a reconnect), an **`event:`** name (a type the client dispatches on), one or more **`data:`** lines (multiline payloads split into several), and a **`retry:`** hint (reconnect delay in ms). A **blank line terminates** the event.

**Steps:**

1. Write `id:`, `event:`, `retry:` if present.
2. Split multiline data into multiple `data:` lines.
3. End with a blank line.

```go
package main

import (
	"bytes"
	"fmt"
	"strings"
)

// A full SSE event can carry an id (for resume), an event name (a type/channel), one
// or more data lines, and a retry hint (ms). A blank line terminates the event.
func writeEvent(buf *bytes.Buffer, id, event, data, retry string) {
	if id != "" {
		fmt.Fprintf(buf, "id: %s\n", id)
	}
	if event != "" {
		fmt.Fprintf(buf, "event: %s\n", event)
	}
	if retry != "" {
		fmt.Fprintf(buf, "retry: %s\n", retry)
	}
	for _, line := range strings.Split(data, "\n") { // multiline -> multiple data: lines
		fmt.Fprintf(buf, "data: %s\n", line)
	}
	buf.WriteString("\n") // blank line = end of event
}

func main() {
	var buf bytes.Buffer
	writeEvent(&buf, "42", "chat", "hello\nworld", "3000")
	fmt.Print(buf.String())
}
```

**Output:**

```
id: 42
event: chat
retry: 3000
data: hello
data: world

```

---

## 3. SSE and client disconnect

`🟢 easy` · *sse*

A streaming handler runs until the client goes away — so it **must** watch `r.Context().Done()` (cancelled when the connection drops) and return, or you leak a goroutine per disconnected client. Here an injected context stands in for the request's; cancelling it stops the loop.

**Steps:**

1. `select` over `ctx.Done()` and the events channel.
2. On cancel, return (don't leak).
3. Two events are delivered, then the "client" disconnects.

```go
package main

import (
	"context"
	"fmt"
)

// A real SSE handler loops until the client disconnects (r.Context() is cancelled) or
// a shutdown fires — otherwise the goroutine leaks. Simulated with an injected context.
func stream(ctx context.Context, events <-chan string, out func(string)) int {
	sent := 0
	for {
		select {
		case <-ctx.Done():
			return sent // client gone -> stop, don't leak
		case e, ok := <-events:
			if !ok {
				return sent
			}
			out(e)
			sent++
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan string) // unbuffered: each send waits for the stream to take it
	done := make(chan int, 1)
	go func() { done <- stream(ctx, events, func(s string) { fmt.Println("send: data:", s) }) }()

	events <- "a"
	events <- "b"
	cancel() // client disconnects
	fmt.Println("client disconnected; events sent:", <-done)
}
```

**Output:**

```
send: data: a
send: data: b
client disconnected; events sent: 2
```

---

## 4. The WebSocket handshake key

`🟢 easy` · *websocket*

The handshake proves the server actually speaks WebSocket (not some cache replaying an HTTP response). The server takes the client's random `Sec-WebSocket-Key`, appends a fixed GUID, SHA-1s it, and base64s the result into **`Sec-WebSocket-Accept`**. The inputs and output below are the RFC 6455 example vector.

**Steps:**

1. `accept = base64(sha1(key + wsGUID))`.
2. The GUID `258EAFA5-…` is a constant from the spec.
3. The RFC key yields `s3pPLMBiTxaQ9kYGzzhZRbK+xOo=`.

```go
package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
)

// The WebSocket handshake proves the server speaks the protocol: take the client's
// Sec-WebSocket-Key, append the fixed GUID, SHA-1 it, base64 the result into
// Sec-WebSocket-Accept. (Inputs/output are the RFC 6455 example vector.)
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func accept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func main() {
	key := "dGhlIHNhbXBsZSBub25jZQ==" // from the RFC example
	fmt.Println("Sec-WebSocket-Accept:", accept(key))
}
```

**Output:**

```
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

---

## 5. The WebSocket upgrade response

`🟢 easy` · *websocket*

The client sends an ordinary HTTP `GET` with `Upgrade: websocket`; the server validates it and replies **`101 Switching Protocols`** with the computed accept header. After the `101`, both sides stop speaking HTTP and switch to the WebSocket framing protocol on the same TCP connection.

**Steps:**

1. Parse the upgrade request; require `Upgrade: websocket`.
2. Return the `101` response with `Sec-WebSocket-Accept`.
3. The blank line ends the response; framing begins after it.

```go
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func accept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// handshakeResponse validates the upgrade request and returns the 101 response that
// switches the TCP connection over to the WebSocket protocol.
func handshakeResponse(r *http.Request) string {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return "HTTP/1.1 400 Bad Request\r\n\r\n"
	}
	return "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n"
}

func main() {
	raw := "GET /chat HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	req, _ := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	fmt.Print(handshakeResponse(req))
}
```

**Output:**

```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=

```

---

## 6. Encode a WebSocket frame

`🟢 easy` · *framing*

After the handshake, data travels in **frames**. The header is tiny: byte 0 is the `FIN` bit plus a 4-bit **opcode** (`0x1` = text), byte 1 is the **mask bit** plus a 7-bit length. Server→client frames are **unmasked**, and small payloads (`<126`) put the length right in that byte.

**Steps:**

1. `0x81` = `FIN`(0x80) | text opcode(0x1).
2. `byte(len)` with the mask bit clear (server frame).
3. Append the raw payload.

```go
package main

import "fmt"

// encodeText builds a server->client text frame (RFC 6455): byte0 = FIN(0x80)|opcode(0x1);
// byte1 = mask-bit(0 for server)|length. Small payloads (<126) fit the length in one byte.
func encodeText(payload string) []byte {
	frame := []byte{0x81, byte(len(payload))} // FIN + text opcode, unmasked, len
	return append(frame, payload...)
}

func main() {
	frame := encodeText("Hi")
	fmt.Printf("frame bytes: % x\n", frame)
	fmt.Printf("FIN+opcode:  %08b\n", frame[0])
	fmt.Printf("mask+len:    %08b (len=%d)\n", frame[1], frame[1]&0x7f)
}
```

**Output:**

```
frame bytes: 81 02 48 69
FIN+opcode:  10000001
mask+len:    00000010 (len=2)
```

---

## 7. Frame masking

`🟢 easy` · *framing*

The spec **requires** client→server frames to be masked (it prevents certain proxy cache-poisoning attacks). Each payload byte is XORed with a 4-byte key, cycling by `index % 4`. Because XOR is its own inverse, the **same operation** unmasks — so one function does both.

**Steps:**

1. `out[i] = payload[i] ^ key[i%4]`.
2. Apply it once to mask.
3. Apply it again with the same key to recover the plaintext.

```go
package main

import "fmt"

// Client->server frames MUST be masked: each payload byte is XORed with a 4-byte
// masking key (cycling by index%4). Masking is symmetric — the same op unmasks.
func mask(payload, key []byte) []byte {
	out := make([]byte, len(payload))
	for i := range payload {
		out[i] = payload[i] ^ key[i%4]
	}
	return out
}

func main() {
	key := []byte{0x37, 0xfa, 0x21, 0x3d}
	masked := mask([]byte("Hello"), key)
	fmt.Printf("masked:   % x\n", masked)
	fmt.Printf("unmasked: %s\n", mask(masked, key)) // same op reverses it
}
```

**Output:**

```
masked:   7f 9f 4d 51 58
unmasked: Hello
```

---

## 8. Decode a masked frame

`🟢 easy` · *framing*

Putting it together on the read side: parse the opcode and length, confirm the mask bit is set, read the 4-byte key, then unmask the payload. This is what a server does for every inbound text frame.

**Steps:**

1. Byte 0 low nibble = opcode; byte 1 high bit = masked, low 7 bits = length.
2. Bytes 2–5 are the masking key; the rest is the masked payload.
3. XOR back to the text.

```go
package main

import "fmt"

// decodeClientFrame parses a masked client text frame: opcode/len, the 4-byte mask,
// then unmask the payload. (Handles a single small text frame for clarity.)
func decodeClientFrame(f []byte) (string, bool) {
	if len(f) < 2 {
		return "", false
	}
	opcode := f[0] & 0x0f
	masked := f[1]&0x80 != 0
	length := int(f[1] & 0x7f)
	if opcode != 0x1 || !masked || length >= 126 || len(f) < 6+length {
		return "", false
	}
	key := f[2:6]
	out := make([]byte, length)
	for i, b := range f[6 : 6+length] {
		out[i] = b ^ key[i%4]
	}
	return string(out), true
}

func main() {
	// A masked "Hi" frame from a client (0x82 = mask bit + length 2).
	key := []byte{0x01, 0x02, 0x03, 0x04}
	frame := []byte{0x81, 0x82, key[0], key[1], key[2], key[3], 'H' ^ 0x01, 'i' ^ 0x02}
	msg, ok := decodeClientFrame(frame)
	fmt.Printf("decoded: %q ok=%v\n", msg, ok)
}
```

**Output:**

```
decoded: "Hi" ok=true
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
