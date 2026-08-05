# Step 63 — Networking Fundamentals · 🟡 Medium

Examples **9–17**. Connection **lifecycle** — closes, deadlines, shutdown, limits — plus **UDP**.
Every example is a self-contained program that runs both the server and the client, so
`go run main.go` shows the whole exchange. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. EOF means the peer closed

`🟡 medium` · *lifecycle*

When the other side closes, TCP sends a **FIN** and Go surfaces it as **`io.EOF`** on the next `Read`. EOF is not a failure — it is the normal end of a stream, and treating it as an error is how you end up logging noise on every healthy disconnect.

**Steps:**

1. Read in a loop; handle `n > 0` **before** checking the error (a read can return data *and* an error).
2. `errors.Is(err, io.EOF)` distinguishes a clean close from a real failure.
3. Compare with example 21, case 4: a *rude* close gives `ECONNRESET` instead.

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
)

// When the peer closes its side, TCP sends a FIN and Go surfaces it as io.EOF on
// the next Read. EOF is not a failure - it is the normal end of a stream.
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

		buf := make([]byte, 64)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				fmt.Printf("read %d bytes: %q\n", n, buf[:n])
			}
			if err != nil {
				fmt.Println("read error:      ", err)
				fmt.Println("is it io.EOF?    ", errors.Is(err, io.EOF))
				fmt.Println("=> peer closed cleanly, not a failure")
				return
			}
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(conn, "some data")
	conn.Close() // sends FIN
	<-done
}
```

**Output:**

```
read 9 bytes: "some data"
read error:       EOF
is it io.EOF?     true
=> peer closed cleanly, not a failure
```

---

## 10. Half-close with CloseWrite

`🟡 medium` · *lifecycle*

Each direction of a TCP connection closes independently — a connection can be **half-open**. `CloseWrite()` sends your FIN ("I am done sending") while you keep reading the reply. That is how "send the whole request, then read until EOF" protocols work without a length header.

**Steps:**

1. The client writes its request, then calls `(*net.TCPConn).CloseWrite()`.
2. The server's `io.ReadAll` now terminates — safe *only* because of the half-close.
3. The server still writes a reply on the same connection, and the client still reads it.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

// Each side of a TCP connection closes independently: a connection can be
// HALF-OPEN. CloseWrite() sends your FIN ("I'm done sending") while you keep
// reading the reply. That is how "send the whole request, then read until EOF"
// protocols work without a length header.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Safe only because the client half-closes: otherwise this blocks forever.
		req, err := io.ReadAll(conn)
		if err != nil {
			log.Print(err)
			return
		}
		fmt.Printf("server read whole request (%d bytes): %q\n", len(req), req)
		fmt.Fprint(conn, "response after EOF") // the connection still works this way
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprint(conn, "line one\nline two\n")

	tcp := conn.(*net.TCPConn)
	if err := tcp.CloseWrite(); err != nil { // FIN, read side stays open
		log.Fatal(err)
	}
	fmt.Println("client half-closed (CloseWrite)")

	resp, err := io.ReadAll(conn)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("client still read the reply:      %q\n", resp)
}
```

**Output:**

```
client half-closed (CloseWrite)
server read whole request (18 bytes): "line one\nline two\n"
client still read the reply:      "response after EOF"
```

---

## 11. Read deadlines and net.Error

`🟡 medium` · *deadlines*

There is no "cancel this Read". **Deadlines are the only built-in way** to time-limit a blocked network call. They are *absolute times*, not durations, and they stay in effect until changed — so set one before **every** read.

**Steps:**

1. `SetReadDeadline(time.Now().Add(d))` before the read.
2. The error satisfies `net.Error` with `Timeout() == true`, and `errors.Is(err, os.ErrDeadlineExceeded)`.
3. A timeout is *retryable*: extend the deadline and read again — the connection is still usable.

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// There is no "cancel this Read". Deadlines are the only built-in way to stop a
// blocked network call. They are ABSOLUTE times, not durations, and they stay in
// effect until you change them - so set one before every read.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(300 * time.Millisecond) // a slow peer
		fmt.Fprint(conn, "late reply\n")
		time.Sleep(50 * time.Millisecond)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	buf := make([]byte, 64)

	// Too short: the peer has not written yet.
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = conn.Read(buf)
	fmt.Println("read error:        ", err)

	var nerr net.Error
	fmt.Println("is a net.Error:    ", errors.As(err, &nerr))
	fmt.Println("Timeout():         ", nerr.Timeout())
	fmt.Println("os.ErrDeadlineExceeded:", errors.Is(err, os.ErrDeadlineExceeded))

	// A timeout is retryable: extend the deadline and try again.
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("second read worked: %q\n", buf[:n])
}
```

**Output:**

```
read error:         read tcp 127.0.0.1:59294->127.0.0.1:59293: i/o timeout
is a net.Error:     true
Timeout():          true
os.ErrDeadlineExceeded: true
second read worked: "late reply\n"
```

> Output note: the addresses in the error text contain ephemeral ports, so they differ on every run.

---

## 12. An idle timeout that drops silent clients

`🟡 medium` · *deadlines*

A connection with no deadline can pin a goroutine **and** a file descriptor forever against a peer that never speaks again — the slow-loris resource leak. The fix is an **idle timeout**: refresh the deadline on every message, and drop the connection when it goes quiet.

**Steps:**

1. Call `SetReadDeadline` at the top of each loop iteration, not once before the loop.
2. When `Scan` fails, check `net.Error.Timeout()` to tell "idle" from "disconnected".
3. The dropped client sees `io.EOF` on its next read.

```go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const idleTimeout = 150 * time.Millisecond

// A connection with no deadline can hold a goroutine and a file descriptor
// forever against a peer that never speaks again (the slow-loris resource leak).
// An IDLE TIMEOUT is the fix: refresh the deadline on every message, and drop
// the connection when it goes quiet.
func handle(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for {
		conn.SetReadDeadline(time.Now().Add(idleTimeout)) // refresh per message
		if !sc.Scan() {
			var nerr net.Error
			if errors.As(sc.Err(), &nerr) && nerr.Timeout() {
				fmt.Println("server: idle timeout, closing connection")
			}
			return
		}
		fmt.Printf("server: got %q\n", sc.Text())
	}
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handle(conn)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprint(conn, "first\n")
	time.Sleep(50 * time.Millisecond) // still under the idle timeout
	fmt.Fprint(conn, "second\n")

	// ...then go quiet. The server drops us; our next Read sees EOF.
	buf := make([]byte, 16)
	_, err = conn.Read(buf)
	fmt.Println("client read after being dropped:", err, "(EOF:", errors.Is(err, io.EOF), ")")
}
```

**Output:**

```
server: got "first"
server: got "second"
server: idle timeout, closing connection
client read after being dropped: EOF (EOF: true )
```

---

## 13. Unblocking a stuck Read by closing

`🟡 medium` · *deadlines*

The second lever: `Close()` the connection from **another goroutine**, and the blocked `Read` returns immediately with `net.ErrClosed`. This is how you tear connections down on shutdown, or when a `context` cancelled somewhere else.

**Steps:**

1. One goroutine blocks in `Read` with no deadline.
2. Another calls `conn.Close()`.
3. `errors.Is(err, net.ErrClosed)` identifies it — "use of closed network connection".

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// The second way to unblock a stuck network call: Close() the connection from
// another goroutine. The blocked Read returns immediately with net.ErrClosed.
// This is how you tear down connections on shutdown, or on a context that was
// cancelled elsewhere.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(time.Second) // never sends anything useful
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}

	// Wire a context to the connection: cancel -> Close -> blocked Read returns.
	stop := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		fmt.Println("closer: closing the connection")
		conn.Close()
		close(stop)
	}()

	fmt.Println("reader: blocking in Read with no deadline...")
	_, err = conn.Read(make([]byte, 64))
	<-stop
	fmt.Println("reader: unblocked with error:", err)
	fmt.Println("errors.Is(err, net.ErrClosed):", errors.Is(err, net.ErrClosed))
}
```

**Output:**

```
reader: blocking in Read with no deadline...
closer: closing the connection
reader: unblocked with error: read tcp 127.0.0.1:59298->127.0.0.1:59297: use of closed network connection
errors.Is(err, net.ErrClosed): true
```

> Output note: the error text contains ephemeral ports and varies per run.

---

## 14. Graceful shutdown of a TCP server

`🟡 medium` · *shutdown*

The same two-step shape `http.Server.Shutdown` uses: **close the listener** so no new connections are accepted, then **wait on a `WaitGroup`** so in-flight handlers finish. Nothing in flight is cut off.

**Steps:**

1. `ln.Close()` makes the blocked `Accept` return — check `errors.Is(err, net.ErrClosed)` and exit the loop cleanly.
2. `wg.Add(1)` per accepted connection, `wg.Done()` in the handler.
3. New dials are refused immediately, while the running handler still completes.

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Graceful shutdown of a raw TCP server, the same shape http.Server.Shutdown uses:
//  1. Close the listener  -> no NEW connections; Accept returns net.ErrClosed.
//  2. Wait on a WaitGroup -> in-flight handlers finish their work.
type server struct {
	ln net.Listener
	wg sync.WaitGroup
}

func (s *server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				fmt.Println("accept loop: listener closed, stopping")
				return
			}
			log.Print(err)
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			fmt.Println("handler: started (slow work)")
			time.Sleep(200 * time.Millisecond) // an in-flight request
			fmt.Fprint(conn, "done\n")
			fmt.Println("handler: finished")
		}()
	}
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	s := &server{ln: ln}
	go s.serve()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond) // let the handler start

	fmt.Println("shutdown: closing listener")
	ln.Close()

	// New connections are refused from here on.
	if _, err := net.Dial("tcp", ln.Addr().String()); err != nil {
		fmt.Println("shutdown: new dial rejected as expected")
	}

	s.wg.Wait() // drain
	fmt.Println("shutdown: all handlers drained, exiting")
}
```

**Output:**

```
handler: started (slow work)
shutdown: closing listener
accept loop: listener closed, stopping
shutdown: new dial rejected as expected
handler: finished
shutdown: all handlers drained, exiting
```

---

## 15. Capping concurrent connections

`🟡 medium` · *limits*

`go handle(conn)` with no ceiling turns a connection flood into goroutines, file descriptors and memory until the process dies. A buffered channel used as a **semaphore** caps how many are served at once — this is what `netutil.LimitListener` does at the listener level.

**Steps:**

1. `sem := make(chan struct{}, N)`; send to acquire before the goroutine, receive to release.
2. Acquiring **before** `go` applies backpressure to the accept loop itself.
3. Eight clients, limit two: the tracked high-water mark never exceeds the limit.

```go
package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const maxConcurrent = 2

// "go handle(conn)" with no ceiling means a connection flood becomes goroutines,
// file descriptors and memory until the process dies. A buffered channel used as
// a SEMAPHORE caps how many connections are served at once (this is what
// golang.org/x/net/netutil.LimitListener does at the listener level).
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	var cur, peak int64
	sem := make(chan struct{}, maxConcurrent)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			sem <- struct{}{} // acquire (blocks when full)
			go func() {
				defer func() { <-sem }() // release
				defer conn.Close()

				n := atomic.AddInt64(&cur, 1)
				for { // track the high-water mark
					p := atomic.LoadInt64(&peak)
					if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
						break
					}
				}
				time.Sleep(60 * time.Millisecond) // pretend work
				atomic.AddInt64(&cur, -1)
			}()
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				return
			}
			defer conn.Close()
			conn.Read(make([]byte, 1)) // wait until the handler closes us
		}()
	}
	wg.Wait()

	fmt.Println("clients:            8")
	fmt.Println("limit:             ", maxConcurrent)
	fmt.Println("peak concurrent:   ", atomic.LoadInt64(&peak))
	fmt.Println("stayed within limit:", atomic.LoadInt64(&peak) <= maxConcurrent)
}
```

**Output:**

```
clients:            8
limit:              2
peak concurrent:    2
stayed within limit: true
```

---

## 16. UDP: datagrams keep their boundaries

`🟡 medium` · *udp*

The other transport: no handshake, no connection, no ordering, no retransmission — but **one `WriteTo` is one datagram is one `ReadFrom`**. Compare with example 6, where three TCP writes collapsed into one read. DNS, QUIC/HTTP-3 and statsd all ride on this.

**Steps:**

1. `net.ListenPacket` — there is no `Listener` and no `Accept`, because there are no connections.
2. `ReadFrom` returns the payload **and the sender's address** (you need it to reply).
3. One reader goroutine feeding a worker pool is the shape here — not goroutine-per-connection.

```go
package main

import (
	"fmt"
	"log"
	"net"
)

// UDP is the other transport: no handshake, no connection, no ordering, no
// retransmission - but MESSAGE BOUNDARIES ARE PRESERVED. One WriteTo is one
// datagram is one ReadFrom. Compare with example 6, where three TCP writes
// collapsed into one read.
//
// There is no per-connection goroutine here, because there are no connections:
// one goroutine reads the socket and hands work to a pool.
func main() {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0") // no Accept, no Listener
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1500) // size it for your largest datagram
		for i := 0; i < 3; i++ {
			n, addr, err := pc.ReadFrom(buf) // returns WHO sent it
			if err != nil {
				return
			}
			fmt.Printf("datagram %d: %2d bytes %-22q from %s\n",
				i+1, n, buf[:n], addr.Network())
		}
	}()

	client, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	for _, m := range []string{"AAA", "BBB", "a longer datagram"} {
		if _, err := client.Write([]byte(m)); err != nil { // one Write = one packet
			log.Fatal(err)
		}
	}
	<-done
	fmt.Println("3 writes -> 3 reads: boundaries preserved")
}
```

**Output:**

```
datagram 1:  3 bytes "AAA"                  from udp
datagram 2:  3 bytes "BBB"                  from udp
datagram 3: 17 bytes "a longer datagram"    from udp
3 writes -> 3 reads: boundaries preserved
```

---

## 17. The UDP truncation trap

`🟡 medium` · *udp*

If your buffer is smaller than the datagram, the excess is **silently discarded** — there is no "read the rest" like there is with TCP. No error, no short-read signal, just missing bytes.

**Steps:**

1. Send 100 bytes, read with a 10-byte buffer.
2. `n == 10`; the other 90 bytes are gone for good.
3. Size buffers for the largest message you accept — ~1500 bytes stays inside one Ethernet frame.

```go
package main

import (
	"fmt"
	"log"
	"net"
	"strings"
)

// The UDP trap: if your buffer is smaller than the datagram, the extra bytes are
// SILENTLY DISCARDED - there is no "read the rest" like there is with TCP.
// Always size the buffer for the largest message you accept (64 KB is the
// theoretical max; ~1500 bytes fits in one Ethernet frame without fragmenting).
func main() {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	payload := strings.Repeat("X", 100)

	done := make(chan struct{})
	go func() {
		defer close(done)
		small := make([]byte, 10) // too small on purpose
		n, _, err := pc.ReadFrom(small)
		if err != nil {
			log.Print(err)
			return
		}
		fmt.Println("sent:      ", len(payload), "bytes")
		fmt.Println("buffer:    ", len(small), "bytes")
		fmt.Println("read:      ", n, "bytes")
		fmt.Println("lost:      ", len(payload)-n, "bytes, unrecoverable")
	}()

	client, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte(payload)); err != nil {
		log.Fatal(err)
	}
	<-done
}
```

**Output:**

```
sent:       100 bytes
buffer:     10 bytes
read:       10 bytes
lost:       90 bytes, unrecoverable
```

---
