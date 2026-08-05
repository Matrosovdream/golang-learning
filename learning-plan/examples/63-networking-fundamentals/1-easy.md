# Step 63 — Networking Fundamentals · 🟢 Easy

Examples **1–8**. Sockets, the accept loop, and the one thing that trips everyone up: **framing**.
Every example is a self-contained program that runs both the server and the client, so
`go run main.go` shows the whole exchange. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Your first TCP server

`🟢 easy` · *tcp server*

Before `net/http` there is a **socket**. `net.Listen` performs `socket()` + `bind()` + `listen()`: from that moment the kernel completes handshakes into an **accept queue**, and `Accept` pulls one out. Everything else in this library is a variation on these four lines.

**Steps:**

1. `net.Listen("tcp", …)` returns a `net.Listener`.
2. `Accept()` blocks until a client has completed the 3-way handshake.
3. The returned `net.Conn` is an `io.Reader`/`io.Writer` — `io.ReadAll` drains it until the peer closes.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	// net.Listen does socket() + bind() + listen(): from here the kernel accepts
	// handshakes into a queue, even before we call Accept.
	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0 = "kernel, pick a free one"
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	fmt.Println("network:", ln.Addr().Network())

	// A client in another goroutine, so this one program shows both sides.
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String()) // 3-way handshake
		if err != nil {
			log.Print(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "hello from the client")
		// Closing sends FIN -> the server's Read returns io.EOF.
	}()

	conn, err := ln.Accept() // blocks until a connection is in the accept queue
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	b, err := io.ReadAll(conn) // reads until EOF (the peer's FIN)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("server received %d bytes: %q\n", len(b), b)
}
```

**Output:**

```
network: tcp
server received 21 bytes: "hello from the client"
```

---

## 2. A TCP client with net.Dial

`🟢 easy` · *tcp client*

The other half. `net.Dial` runs the handshake and hands back the same `net.Conn` type the server got — the API is symmetric, which is why a proxy (example 25) is so short.

**Steps:**

1. `net.Dial("tcp", addr)` connects.
2. Write the request, read the reply — it is just a stream.
3. `bufio.Reader.ReadString('\n')` frames the response on a newline (example 7).

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
)

// server replies to one request, then closes.
func server(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	fmt.Printf("server read:  %q\n", line)
	fmt.Fprintf(conn, "%s\n", strings.ToUpper(strings.TrimSpace(line)))
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	go server(ln)

	// The client half: Dial gives you a net.Conn, which is just an
	// io.Reader + io.Writer + io.Closer with deadlines.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprint(conn, "ping\n") // request
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("client read:  %q\n", reply)
}
```

**Output:**

```
server read:  "ping\n"
client read:  "PING\n"
```

---

## 3. One goroutine per connection

`🟢 easy` · *concurrency*

The idiomatic Go server: the accept loop does nothing but accept, and each connection gets its own goroutine. A slow client can never block the loop. This is exactly what `http.Server.Serve` does internally (`go c.serve()`).

**Steps:**

1. `for { conn, err := ln.Accept(); go handle(conn) }`.
2. `handle` owns the connection for its whole life and `defer conn.Close()`s it.
3. Three clients connect at once; replies are sorted because concurrent arrival order is not deterministic.

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
)

// handle owns one connection for its whole life. One goroutine per connection is
// the idiomatic Go server: a parked goroutine costs a few KB, and the runtime's
// netpoller (epoll/kqueue) wakes it when bytes actually arrive.
func handle(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		fmt.Fprintf(conn, "echo: %s\n", sc.Text())
	}
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handle(conn) // <- the accept loop never blocks on a slow client
		}
	}()

	// Three clients at once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var replies []string
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			fmt.Fprintf(conn, "client-%d\n", n)
			line, _ := bufio.NewReader(conn).ReadString('\n')
			mu.Lock()
			replies = append(replies, line[:len(line)-1])
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	sort.Strings(replies) // concurrent -> arrival order is not deterministic
	for _, r := range replies {
		fmt.Println(r)
	}
}
```

**Output:**

```
echo: client-1
echo: client-2
echo: client-3
```

---

## 4. Port :0 — let the kernel choose

`🟢 easy` · *ports*

Binding port **0** asks the kernel for any free ephemeral port, and `ln.Addr()` reports what you actually got. This is the trick that makes network tests runnable in parallel with no port collisions — `httptest.NewServer` does it too.

**Steps:**

1. Listen on `127.0.0.1:0`.
2. Read the real address back from `ln.Addr()`; never hardcode it.
3. Note `127.0.0.1:0` is loopback-only — in a container you need `0.0.0.0` (lesson 65).

```go
package main

import (
	"fmt"
	"log"
	"net"
)

func main() {
	// Port 0 asks the kernel for any free ephemeral port. This is how you write
	// tests that can run in parallel without fighting over a hardcoded port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	// ln.Addr() reports what was ACTUALLY bound - ask it, never assume.
	addr := ln.Addr().(*net.TCPAddr)
	fmt.Println("asked for port:   0")
	fmt.Println("got a real port:  ", addr.Port > 0)
	fmt.Println("bound to loopback:", addr.IP.IsLoopback())

	// Anything that needs the address (a client, a test, a config line) reads it
	// from the listener rather than guessing.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Println("dialed ln.Addr():  ok")

	// Binding ":0" instead would listen on ALL interfaces; "127.0.0.1:0" is
	// loopback-only - unreachable from another machine (or another container).
}
```

**Output:**

```
asked for port:   0
got a real port:   true
bound to loopback: true
dialed ln.Addr():  ok
```

---

## 5. The 4-tuple: why one port serves many clients

`🟢 easy` · *4-tuple*

A TCP connection is identified by **(src IP, src port, dst IP, dst port)** — not by the port alone. The listening port stays the same for everyone; each client contributes a different ephemeral source port, so the tuples never collide.

**Steps:**

1. Compare `LocalAddr`/`RemoteAddr` on both ends — they mirror.
2. The server's local address is the listening address.
3. The client's local port was assigned automatically and differs from the server's.

```go
package main

import (
	"fmt"
	"log"
	"net"
)

// A TCP connection is identified by a 4-tuple:
//
//	(source IP, source port, destination IP, destination port)
//
// That is why one listening port can serve thousands of clients at once: every
// accepted connection differs in the client's ephemeral port.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	type tuple struct{ local, remote string }
	got := make(chan tuple, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		got <- tuple{conn.LocalAddr().String(), conn.RemoteAddr().String()}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	srv := <-got

	// The two ends see the same tuple, mirrored.
	fmt.Println("client local  == server remote:", client.LocalAddr().String() == srv.remote)
	fmt.Println("client remote == server local: ", client.RemoteAddr().String() == srv.local)

	// The server's local port is the well-known listening port; the client's
	// local port was assigned automatically from the ephemeral range.
	lp := ln.Addr().(*net.TCPAddr).Port
	cp := client.LocalAddr().(*net.TCPAddr).Port
	fmt.Println("server port is the listening port:", srv.local == ln.Addr().String())
	fmt.Println("client got a different port:      ", cp != lp)
}
```

**Output:**

```
client local  == server remote: true
client remote == server local:  true
server port is the listening port: true
client got a different port:       true
```

---

## 6. TCP has no message boundaries

`🟢 easy` · *framing*

**The single most common raw-TCP bug.** TCP guarantees order and delivery and guarantees *nothing* about where one `Write` ends and the next begins. Here three writes arrive as one read; the reverse (one write, three reads) is equally legal.

**Steps:**

1. The client writes `"AAA"`, `"BBB"`, `"CCC"` as three separate `Write` calls.
2. The server sleeps so all three land in the receive buffer, then does **one** `Read`.
3. It gets all 9 bytes at once — the message boundaries are gone.

```go
package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

// TCP is a BYTE STREAM. It guarantees order and delivery, and guarantees nothing
// about where one Write ends and the next begins. Three Writes can arrive as one
// Read - and one Write can arrive as three Reads.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Sleep so all three client writes land in the receive buffer first.
		time.Sleep(100 * time.Millisecond)

		buf := make([]byte, 1024)
		n, err := conn.Read(buf) // ONE Read...
		if err != nil {
			log.Print(err)
			return
		}
		fmt.Printf("client sent 3 messages of 3 bytes\n")
		fmt.Printf("server got 1 read of %d bytes: %q\n", n, buf[:n])
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(conn, "AAA") // ...three Writes
	fmt.Fprint(conn, "BBB")
	fmt.Fprint(conn, "CCC")
	<-done
	conn.Close()

	fmt.Println("=> the receiver must FRAME the stream itself (examples 7 and 8)")
}
```

**Output:**

```
client sent 3 messages of 3 bytes
server got 1 read of 9 bytes: "AAABBBCCC"
=> the receiver must FRAME the stream itself (examples 7 and 8)
```

> If you ever write `conn.Read(buf)` once and then parse `buf`, you have this bug. The next two examples are the two fixes.

---

## 7. Framing I — newline-delimited

`🟢 easy` · *framing*

Fix #1: agree on a **delimiter**. `bufio.Scanner` buffers the stream and yields whole lines however the bytes were chopped up. Simple, human-readable, and what Redis, SMTP and HTTP headers all do.

**Steps:**

1. Wrap the connection in a `bufio.Scanner`.
2. **Always call `sc.Buffer(...)`** to cap the line length — an unbounded line is a memory DoS.
3. Note the ragged writes on the client side: the scanner reassembles them correctly.

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

// Framing #1: a DELIMITER. Every message ends with '\n', so the reader knows
// where one stops. bufio.Scanner buffers the stream and hands you whole lines,
// no matter how the bytes were split across packets.
//
// Trade-off: the payload must never contain the delimiter (escape it, or use
// length-prefix framing instead - example 8).
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 0, 4096), 64*1024) // cap the line length!
		for sc.Scan() {
			fmt.Printf("message: %q\n", sc.Text())
		}
		if err := sc.Err(); err != nil {
			log.Print(err)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	// Deliberately ragged writes: a message split in half, two messages in one write.
	fmt.Fprint(conn, "hel")
	fmt.Fprint(conn, "lo\nwor")
	fmt.Fprint(conn, "ld\nthird\n")
	conn.Close() // FIN -> Scan returns false
	<-done
}
```

**Output:**

```
message: "hello"
message: "world"
message: "third"
```

> The catch: the payload must never contain the delimiter. If it can, escape it — or use length-prefix framing.

---

## 8. Framing II — length-prefixed

`🟢 easy` · *framing*

Fix #2: send the **length first**. `[4-byte length][payload]` handles binary data and embedded newlines, and lets the reader allocate exactly once. This is how gRPC, WebSocket frames (lesson 58) and most binary protocols work.

**Steps:**

1. `binary.BigEndian.PutUint32` writes the header; network byte order is big-endian.
2. **`io.ReadFull`** is the key call — it loops until the buffer is full, so a split packet is a non-issue.
3. **Bound the length** before allocating: the number came from the network.

```go
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

const maxFrame = 1 << 20 // always bound it: the length comes from the network

// writeFrame sends: [4-byte big-endian length][payload].
func writeFrame(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads exactly one message, however the stream was chopped up.
// io.ReadFull is the key: it loops until the buffer is full or the stream ends.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, fmt.Errorf("frame too large: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msg, err := readFrame(conn)
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Print(err)
				return
			}
			fmt.Printf("frame of %2d bytes: %q\n", len(msg), msg)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range []string{"hi", "a longer message", "with\nnewlines\ninside"} {
		if err := writeFrame(conn, []byte(m)); err != nil {
			log.Fatal(err)
		}
	}
	conn.Close()
	<-done
}
```

**Output:**

```
frame of  2 bytes: "hi"
frame of 16 bytes: "a longer message"
frame of 20 bytes: "with\nnewlines\ninside"
```

---
