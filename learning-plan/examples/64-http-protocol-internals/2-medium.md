# Step 64 — The HTTP Protocol & `net/http` Internals · 🟡 Medium

Examples **9–17**. Server **internals** (accept loop, keep-alive, timeouts, streaming, hijack) and the client's **connection pool**.
Every example runs its own server *and* client in one program, so `go run main.go`
shows both sides. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. ConnState: the accept loop, observed

`🟡 medium` · *internals*

`http.Server` is the accept loop from [lesson 63](../63-networking-fundamentals/) with an HTTP parser attached: `ListenAndServe` → `net.Listen` → `Serve(ln)` → `for { ln.Accept(); go c.serve() }`. The **`ConnState`** hook lets you watch that lifecycle directly.

**Steps:**

1. Every connection passes through `StateNew` → `StateActive` → `StateIdle` (kept alive) → `StateClosed`.
2. Each request runs on its connection's own goroutine, so a slow handler blocks only its own client.
3. Five concurrent clients produce five `StateNew` transitions — one connection each in HTTP/1.1.

```go
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// http.Server is the accept loop from lesson 63 with an HTTP parser bolted on:
//
//	ListenAndServe -> net.Listen -> Serve(ln) -> for { ln.Accept(); go c.serve() }
//
// The ConnState hook lets you watch that lifecycle directly.
func main() {
	var mu sync.Mutex
	states := map[http.ConnState]int{}
	var inHandler int64

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&inHandler, 1)
			// Each request runs on the connection's own goroutine, so a slow
			// handler blocks only its own client.
			fmt.Fprintf(w, "goroutines while serving: %d\n", runtime.NumGoroutine())
			time.Sleep(20 * time.Millisecond)
		}),
		ConnState: func(c net.Conn, s http.ConnState) {
			mu.Lock()
			states[s]++
			mu.Unlock()
		},
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go srv.Serve(ln)

	before := runtime.NumGoroutine()

	// 5 clients at once, each with its own connection.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://" + ln.Addr().String())
			if err != nil {
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond) // let ConnState settle

	mu.Lock()
	defer mu.Unlock()
	fmt.Println("goroutines before:", before)
	fmt.Println("requests served:  ", atomic.LoadInt64(&inHandler))
	fmt.Println("StateNew  (accepted):", states[http.StateNew])
	fmt.Println("StateActive (reading a request):", states[http.StateActive])
	fmt.Println("StateIdle (kept alive, waiting): ", states[http.StateIdle])

	srv.Close()
}
```

**Output:**

```
goroutines before: 2
requests served:   5
StateNew  (accepted): 5
StateActive (reading a request): 5
StateIdle (kept alive, waiting):  5
```

---

## 10. Keep-alive: 10 requests, 1 connection

`🟡 medium` · *keep-alive*

HTTP/1.1 connections are **persistent by default**: many request/response pairs ride one TCP connection, amortising the handshake (and TLS handshake). Wrapping the listener to count `Accept` calls proves it.

**Steps:**

1. 10 sequential requests → **1** TCP connection.
2. After the server's `IdleTimeout` expires, the next request must redial.
3. `req.Close = true` (`Connection: close`) ends the connection after that response, so the request *after it* redials too.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// HTTP/1.1 connections are PERSISTENT by default: many request/response pairs
// ride one TCP connection, amortising the handshake (and the TLS handshake).
// Counting accepts on the listener proves it.
//
// The limit: responses must come back IN ORDER on that connection - head-of-line
// blocking, which is what HTTP/2 multiplexing fixes (examples 20-21).
type countingListener struct {
	net.Listener
	accepts int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		atomic.AddInt64(&l.accepts, 1)
	}
	return c, err
}

func main() {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	ln := &countingListener{Listener: base}

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "ok")
		}),
		IdleTimeout:       100 * time.Millisecond, // how long a kept-alive conn may idle
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(ln)
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + base.Addr().String()

	for i := 0; i < 10; i++ {
		resp, err := client.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body) // drain, or the connection cannot be reused
		resp.Body.Close()
	}
	fmt.Println("10 requests ->", atomic.LoadInt64(&ln.accepts), "TCP connection(s)")

	// Let the server's IdleTimeout expire; the next request needs a new connection.
	time.Sleep(200 * time.Millisecond)
	resp, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	fmt.Println("after the idle timeout ->", atomic.LoadInt64(&ln.accepts), "connection(s) total")

	// A client can opt out per request with Connection: close. That request may
	// still reuse an idle connection, but the connection is closed afterwards -
	// so the NEXT request has to open a fresh one.
	req, _ := http.NewRequest("GET", url, nil)
	req.Close = true
	resp2, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	fmt.Println("with Connection: close ->", atomic.LoadInt64(&ln.accepts), "connection(s) total")

	resp3, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	fmt.Println("the request after it   ->", atomic.LoadInt64(&ln.accepts), "connection(s) total (it had to redial)")
}
```

**Output:**

```
10 requests -> 1 TCP connection(s)
after the idle timeout -> 2 connection(s) total
with Connection: close -> 2 connection(s) total
the request after it   -> 3 connection(s) total (it had to redial)
```

> The limit of keep-alive: responses come back **in order** on one connection — head-of-line blocking, which HTTP/2 fixes (examples 20–21).

---

## 11. ReadHeaderTimeout defeats the slow loris

`🟡 medium` · *hardening*

The **slow loris**: a client that opens a connection and dribbles headers forever without finishing the request. With no `ReadHeaderTimeout`, each one pins a goroutine and an fd indefinitely — a handful of clients can exhaust a server using almost no bandwidth.

**Steps:**

1. The attacker sends a request line and a header, then a junk header every 40ms, never the blank line.
2. **Protected** (`ReadHeaderTimeout: 200ms`): the server closes the connection on schedule.
3. **Unprotected** (`0` = no limit): the server holds it indefinitely — here *our own* read deadline is what gives up first.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// The SLOW LORIS: a client that opens a connection and dribbles headers forever,
// never finishing the request. With no ReadHeaderTimeout each such connection
// pins a goroutine and a file descriptor indefinitely - a handful of clients can
// exhaust the server without any bandwidth at all.
//
// ReadHeaderTimeout is the single cheapest hardening knob in net/http.
func attack(addr string, label string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Start a request and never finish the headers (no blank line).
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n")
	start := time.Now()
	go func() {
		for i := 0; ; i++ { // dribble a junk header every 40ms
			time.Sleep(40 * time.Millisecond)
			if _, err := fmt.Fprintf(conn, "X-Pad-%d: junk\r\n", i); err != nil {
				return
			}
		}
	}()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.ReadAll(conn) // returns when the SERVER gives up on us
	fmt.Printf("%-13s server closed the connection after ~%dms (we never sent a valid request)\n",
		label, time.Since(start).Milliseconds()/50*50)
}

func serve(readHeaderTimeout time.Duration) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go srv.Serve(ln)
	return ln.Addr().String()
}

func main() {
	// Protected: the server closes the connection when the headers take too long.
	attack(serve(200*time.Millisecond), "protected:")

	// Unprotected (0 = no limit): the connection would be held forever. We give
	// up after 1s to keep the example short - the SERVER never would.
	addr := serve(0)
	conn, _ := net.Dial("tcp", addr)
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n")
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err := io.ReadAll(conn)
	fmt.Printf("%-12s still holding the connection after 1s (our own read timed out: %v)\n",
		"unprotected:", err != nil)
}
```

**Output:**

```
protected:    server closed the connection after ~200ms (we never sent a valid request)
unprotected: still holding the connection after 1s (our own read timed out: true)
```

> This is the single cheapest hardening knob in `net/http`. Output note: the ~ms figure is rounded and varies slightly.

---

## 12. WriteTimeout breaks streaming — ResponseController fixes it

`🟡 medium` · *streaming*

`WriteTimeout` covers the **whole response**, from the end of the request headers to the last body byte. That is exactly wrong for streaming: SSE, long downloads and progress feeds get cut off mid-flight. Since Go 1.20 the fix is per-request: **`http.ResponseController`**.

**Steps:**

1. A 200ms `WriteTimeout` against a ~400ms stream: the client gets 3 of 5 ticks and an `unexpected EOF`.
2. `rc.SetWriteDeadline(time.Time{})` clears the deadline **for that one request**.
3. Same server, same timeout — now all 5 ticks arrive.

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// The WriteTimeout trap. WriteTimeout covers the whole response - from the end
// of the request headers to the last body byte. That is exactly wrong for
// streaming: SSE, long downloads and progress feeds all get cut off mid-flight.
//
// The fix (Go 1.20+): a generous server-wide WriteTimeout, plus
// http.ResponseController to extend or clear the deadline per request.
func stream(w http.ResponseWriter, r *http.Request, useController bool) {
	rc := http.NewResponseController(w)
	if useController {
		// Clear the write deadline for this ONE request (zero time = no deadline).
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			log.Print(err)
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for i := 1; i <= 5; i++ {
		if _, err := fmt.Fprintf(w, "data: tick %d\n\n", i); err != nil {
			return
		}
		if err := rc.Flush(); err != nil { // Flush via the controller, no type assertion
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func run(label string, useController bool) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		WriteTimeout:      200 * time.Millisecond, // deliberately shorter than the stream
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stream(w, r, useController)
		}),
	}
	go srv.Serve(ln)
	defer srv.Close()

	resp, err := http.Get("http://" + ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	ticks := 0
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if sc.Text() != "" {
			ticks++
		}
	}
	fmt.Printf("%-28s ticks received: %d/5  (read error: %v)\n", label, ticks, sc.Err())
}

func main() {
	// WriteTimeout=200ms vs a 400ms stream: the connection dies half way.
	run("WriteTimeout only:", false)
	// Same server, deadline cleared per request: the whole stream arrives.
	run("+ ResponseController:", true)
}
```

**Output:**

```
WriteTimeout only:           ticks received: 3/5  (read error: unexpected EOF)
+ ResponseController:        ticks received: 5/5  (read error: <nil>)
```

---

## 13. Flusher: sending bytes before the handler returns

`🟡 medium` · *streaming*

By default `net/http` **buffers** your response so it can measure it. `Flush()` pushes what you have written out immediately, which switches the response to chunked encoding (example 3) and is the entire mechanism behind SSE and progress feeds ([lesson 58](../58-realtime-websockets-sse/)).

**Steps:**

1. Set `Content-Type: text/event-stream`; each event is `data: …\n\n`.
2. `rc.Flush()` after each event — without it the client sees nothing until the handler returns.
3. The client reads events **as they arrive**; the timestamps prove nothing was buffered.

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// By default net/http BUFFERS your response - it wants to measure the body and
// set Content-Length. Flush() pushes what you have written out immediately,
// which switches the response to chunked encoding (example 3) and is the whole
// mechanism behind SSE, progress feeds and long downloads (lesson 58).
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE content type; each event is "data: ...\n\n".
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // ask nginx not to buffer us (lesson 65)

		rc := http.NewResponseController(w)
		for i := 1; i <= 4; i++ {
			fmt.Fprintf(w, "data: event %d\n\n", i)
			if err := rc.Flush(); err != nil { // without this the client sees nothing until the end
				return
			}
			time.Sleep(60 * time.Millisecond)
		}
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println("Content-Type:     ", resp.Header.Get("Content-Type"))
	fmt.Println("Transfer-Encoding:", resp.TransferEncoding, "(chunked: streaming, no Content-Length)")

	// Read events AS THEY ARRIVE, not after the response finishes.
	start := time.Now()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		fmt.Printf("t+%3dms  %s\n",
			time.Since(start).Milliseconds()/10*10, strings.TrimPrefix(line, "data: "))
	}
	if err := sc.Err(); err != nil {
		log.Fatal(err)
	}
	// The timestamps show them trickling in - proof nothing was buffered.
}
```

**Output:**

```
Content-Type:      text/event-stream
Transfer-Encoding: [chunked] (chunked: streaming, no Content-Length)
t+  0ms  event 1
t+ 60ms  event 2
t+120ms  event 3
t+180ms  event 4
```

> `X-Accel-Buffering: no` asks nginx not to re-buffer you ([lesson 65](../65-docker-networking/)). Output note: the t+ times vary by a few ms.

---

## 14. Hijacker: taking the raw connection

`🟡 medium` · *hijack*

`Hijack()` hands you the raw `net.Conn` and `net/http` stops managing it — no `ResponseWriter`, no keep-alive, no automatic close. You now speak whatever protocol you like on that socket. This is precisely how a **WebSocket upgrade** works ([lesson 58](../58-realtime-websockets-sse/)).

**Steps:**

1. `http.NewResponseController(w).Hijack()` (Go 1.20+; older code type-asserts `http.Hijacker`).
2. **You** must write the status line and headers — nothing has been sent yet.
3. After that it is not HTTP at all: plain lines over TCP, framed by `\n` ([lesson 63](../63-networking-fundamentals/)).

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Hijack() hands you the raw net.Conn (lesson 63) and net/http stops managing
// it: no more ResponseWriter, no keep-alive handling, no automatic close. You
// now speak whatever protocol you like on that socket.
//
// This is exactly how a WebSocket upgrade works (lesson 58): HTTP does the
// handshake, then the connection becomes a frame-based protocol.
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		conn, buf, err := rc.Hijack() // Go 1.20+; older code type-asserts http.Hijacker
		if err != nil {
			http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
			return
		}
		defer conn.Close() // OUR job now - the server will not do it

		// We must write the status line and headers OURSELVES: nothing has been
		// sent yet, and w is no longer usable.
		fmt.Fprint(buf, "HTTP/1.1 200 OK\r\n"+
			"Content-Type: text/plain\r\n"+
			"Connection: close\r\n"+
			"\r\n")
		fmt.Fprint(buf, "hijacked - now speaking a line protocol\n")
		buf.Flush()

		// Beyond this point it is not HTTP at all: plain lines over TCP.
		for {
			line, err := buf.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimSpace(line)
			if cmd == "BYE" {
				fmt.Fprint(buf, "BYE\n")
				buf.Flush()
				return
			}
			fmt.Fprintf(buf, "ECHO %s\n", cmd)
			buf.Flush()
		}
	}))
	defer srv.Close()

	// A raw TCP client: send an HTTP request, then stop speaking HTTP.
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")

	r := bufio.NewReader(conn)
	for i := 0; i < 5; i++ { // status line + 2 headers + blank line + first body line
		line, err := r.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %q\n", line)
	}

	for _, cmd := range []string{"hello", "world", "BYE"} {
		fmt.Fprintf(conn, "%s\n", cmd)
		reply, err := r.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-6s -> %s", cmd, reply)
	}

	// Caveat: srv.Shutdown() does NOT close hijacked connections - once you take
	// the conn, draining it on shutdown is your responsibility (lesson 66).
}
```

**Output:**

```
  "HTTP/1.1 200 OK\r\n"
  "Content-Type: text/plain\r\n"
  "Connection: close\r\n"
  "\r\n"
  "hijacked - now speaking a line protocol\n"
hello  -> ECHO hello
world  -> ECHO world
BYE    -> BYE
```

> `srv.Shutdown()` does **not** close hijacked connections — tracking and draining them is your job ([lesson 66](../66-serving-go-apps/)).

---

## 15. The Transport is a connection pool

`🟡 medium` · *client pool*

`http.Client` is a thin policy layer (timeout, redirects, cookies); **`http.Transport`** is the engine, keeping idle connections keyed by `(scheme, host, port, proxy)`. This example separates two facts people conflate: **sequential** requests reuse a pooled connection, while **concurrent** requests need one each in HTTP/1.1.

**Steps:**

1. **A.** 50 sequential requests, one shared client → **1** connection.
2. **B.** 50 sequential requests, a new `Client`+`Transport` each time → **50** connections. Creating a client per request throws the pool away every time.
3. **C.** 10 concurrent requests → 10 connections (parallelism sets the floor), and a following sequential batch reuses them.
4. **`MaxIdleConnsPerHost` defaults to 2** — after a burst only 2 are kept idle per host, so a busy client to one backend keeps redialling. Raising it is the classic fix.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// http.Client is a thin policy layer (timeout, redirects, cookies).
// http.Transport is the engine: it keeps a POOL of idle connections keyed by
// (scheme, host, port, proxy) and reuses them.
//
// Two independent facts, which this example separates:
//
//	SEQUENTIAL requests reuse one pooled connection - unless you throw the pool
//	away by building a new Client/Transport each time.
//	CONCURRENT requests need one connection EACH: a connection carries one
//	request at a time in HTTP/1.1. Concurrency sets the floor (HTTP/2 removes
//	this - example 20).
type countingListener struct {
	net.Listener
	accepts int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		atomic.AddInt64(&l.accepts, 1)
	}
	return c, err
}

func newServer() (*countingListener, *http.Server, string) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	ln := &countingListener{Listener: base}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "ok")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(ln)
	return ln, srv, "http://" + base.Addr().String()
}

func get(c *http.Client, url string) {
	resp, err := c.Get(url)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body) // drain (example 16)
	resp.Body.Close()
}

func main() {
	const n = 50

	// A) sequential, ONE shared client: the pool is doing its job.
	ln, srv, url := newServer()
	shared := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < n; i++ {
		get(shared, url)
	}
	srv.Close()
	fmt.Printf("A. sequential, shared client:      %2d connections for %d requests\n",
		atomic.LoadInt64(&ln.accepts), n)

	// B) sequential, a NEW Client+Transport per request: no pool survives.
	ln2, srv2, url2 := newServer()
	for i := 0; i < n; i++ {
		get(&http.Client{Transport: &http.Transport{}}, url2)
	}
	srv2.Close()
	fmt.Printf("B. sequential, client per request: %2d connections for %d requests\n",
		atomic.LoadInt64(&ln2.accepts), n)

	// C) 10 CONCURRENT requests on the shared client: parallelism sets the floor.
	ln3, srv3, url3 := newServer()
	c3 := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); get(c3, url3) }()
	}
	wg.Wait()
	first := atomic.LoadInt64(&ln3.accepts)

	// ...and the SECOND round reuses whatever the pool kept.
	for i := 0; i < 10; i++ {
		get(c3, url3)
	}
	srv3.Close()
	fmt.Printf("C. 10 concurrent:                  %2d connections (one per in-flight request)\n", first)
	fmt.Printf("   + 10 more, sequential:          %2d connections total (reused from the pool)\n",
		atomic.LoadInt64(&ln3.accepts))

	// MaxIdleConnsPerHost defaults to 2: after a burst, only 2 connections are
	// KEPT idle per host and the rest are closed. A busy client to one backend
	// then redials constantly - raising this is the classic fix.
	fmt.Println("\nDefaultMaxIdleConnsPerHost:", http.DefaultMaxIdleConnsPerHost)
	tr := &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 100, IdleConnTimeout: 90 * time.Second}
	fmt.Println("tuned MaxIdleConnsPerHost: ", tr.MaxIdleConnsPerHost)
}
```

**Output:**

```
A. sequential, shared client:       1 connections for 50 requests
B. sequential, client per request: 50 connections for 50 requests
C. 10 concurrent:                  10 connections (one per in-flight request)
   + 10 more, sequential:          10 connections total (reused from the pool)

DefaultMaxIdleConnsPerHost: 2
tuned MaxIdleConnsPerHost:  100
```

---

## 16. Drain the body, or lose the connection

`🟡 medium` · *client pool*

**The client rule that bites everyone.** A connection returns to the pool only once its response body has been fully **read** *and* **closed**. `Close()` alone is not enough: with bytes still pending, the Transport cannot find where the next response starts, so it drops the connection.

**Steps:**

1. 20 requests, `Close()` only → **20** connections.
2. 20 requests, `io.Copy(io.Discard, resp.Body)` then `Close()` → **1** connection.
3. If you only need the status, send `HEAD` or a small `Range` request instead of downloading and discarding.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// THE CLIENT RULE THAT BITES EVERYONE: a connection can only be reused once its
// response body has been fully READ and CLOSED. Close alone is not enough - if
// bytes are still pending, the Transport cannot know where the next response
// begins on that stream, so it drops the connection and dials a new one.
//
//	defer resp.Body.Close()             // required, but not sufficient
//	io.Copy(io.Discard, resp.Body)      // ...drain it too
type countingListener struct {
	net.Listener
	accepts int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		atomic.AddInt64(&l.accepts, 1)
	}
	return c, err
}

func run(label string, drain bool) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	ln := &countingListener{Listener: base}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, strings.Repeat("x", 4096)) // big enough not to fit in one read
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(ln)
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + base.Addr().String()

	for i := 0; i < 20; i++ {
		resp, err := client.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		if drain {
			io.Copy(io.Discard, resp.Body) // read to EOF -> connection goes back to the pool
		}
		resp.Body.Close()
	}
	time.Sleep(20 * time.Millisecond)
	fmt.Printf("%-34s %2d connections for 20 requests\n", label, atomic.LoadInt64(&ln.accepts))
}

func main() {
	run("Close() only (body unread):", false)
	run("drained + Close():", true)

	// Body.Close() on an unread body also has to read-and-discard or kill the
	// connection - which is why "just close it" quietly costs you the pool.
	// If you only need the status, send HEAD, or use a small Range request.
}
```

**Output:**

```
Close() only (body unread):        20 connections for 20 requests
drained + Close():                  1 connections for 20 requests
```

---

## 17. Client timeouts: blanket vs per-request context

`🟡 medium` · *client timeouts*

**`http.DefaultClient` has no timeout** — a server that accepts your connection and goes quiet hangs that goroutine forever. Two levers answer different questions: `Client.Timeout` caps the whole request; a per-request **context** does that *and* propagates cancellation downstream.

**Steps:**

1. `Client.Timeout` → `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`; `os.IsTimeout(err)` is true.
2. `http.NewRequestWithContext` → the same cancellation, but the **handler sees it** (`r.Context().Err()`) and can stop working — watch the server's line in the output.
3. Finer knobs live on the Transport: `TLSHandshakeTimeout`, **`ResponseHeaderTimeout`** (time to first byte — the right one when the body is a long stream), `ExpectContinueTimeout`.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

// http.DefaultClient (and http.Get / http.Post) has NO TIMEOUT. A server that
// accepts your connection and then goes quiet will hang that goroutine forever.
// Two levers, and they answer different questions:
//
//	Client.Timeout      - a blanket cap for the WHOLE request, incl. reading the body
//	NewRequestWithContext - a per-request deadline that also PROPAGATES downstream
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second): // a slow backend
			fmt.Fprintln(w, "finally")
		case <-r.Context().Done(): // the client went away: stop working
			fmt.Println("server: client disconnected ->", r.Context().Err())
		}
	}))
	defer srv.Close()

	fmt.Println("DefaultClient.Timeout:", http.DefaultClient.Timeout, "(zero = wait forever!)")

	// 1. Client.Timeout - simple, blanket.
	c := &http.Client{Timeout: 150 * time.Millisecond}
	_, err := c.Get(srv.URL)
	fmt.Println("\nClient.Timeout:      ", err)
	fmt.Println("  is a timeout:      ", os.IsTimeout(err))

	// 2. Per-request context - the one to prefer: it flows into the handler
	//    (see the server print above) and on into the next hop (lessons 15, 39).
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err = http.DefaultClient.Do(req)
	fmt.Println("\ncontext deadline:    ", err)
	fmt.Println("  DeadlineExceeded:  ", errors.Is(err, context.DeadlineExceeded))

	time.Sleep(50 * time.Millisecond) // let the server's message print

	// Finer-grained knobs live on the Transport: DialContext timeout,
	// TLSHandshakeTimeout, ResponseHeaderTimeout, ExpectContinueTimeout.
	tr := &http.Transport{
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 2 * time.Second, // time to FIRST byte, excludes body streaming
		ExpectContinueTimeout: time.Second,
	}
	fmt.Println("\nResponseHeaderTimeout:", tr.ResponseHeaderTimeout,
		"(use this, not Client.Timeout, when the body is a long stream)")
}
```

**Output:**

```
DefaultClient.Timeout: 0s (zero = wait forever!)
server: client disconnected -> context canceled

Client.Timeout:       Get "http://127.0.0.1:62002": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  is a timeout:       true
server: client disconnected -> context canceled

context deadline:     Get "http://127.0.0.1:62002": context deadline exceeded
  DeadlineExceeded:   true

ResponseHeaderTimeout: 2s (use this, not Client.Timeout, when the body is a long stream)
```

---
