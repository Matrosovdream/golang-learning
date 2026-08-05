# Step 64 — The HTTP Protocol & `net/http` Internals · 🟢 Easy

Examples **1–8**. The **wire format** — what actually travels over the socket — and the server's response contract.
Every example runs its own server *and* client in one program, so `go run main.go`
shows both sides. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. HTTP/1.1 by hand over a raw socket

`🟢 easy` · *the wire*

HTTP/1.1 is text you can type. A request is a **request line**, **headers**, a blank line, then an optional body — every line ending in `CRLF`. Here the client side skips `net/http` entirely and speaks it over a raw TCP socket ([lesson 63](../63-networking-fundamentals/)), so nothing is hidden.

**Steps:**

1. `net.Dial` to the server, then write the request text yourself.
2. **`Host` is mandatory** in HTTP/1.1 — it is how one IP serves many sites (the plaintext cousin of TLS SNI).
3. `Connection: close` makes the server close after replying, so `io.ReadAll` sees the whole response.
4. The output shows `\r\n` explicitly: every header line really does end with CRLF.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// HTTP/1.1 is text you can type. A request is:
//
//	<METHOD> <target> HTTP/1.1 CRLF
//	<Header>: <value>   CRLF   (repeated)
//	CRLF                       <- blank line ends the headers
//	<optional body>
//
// Here we skip net/http on the client side entirely and speak it over a raw TCP
// socket (lesson 63), so there is no magic left to hide behind.
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "you asked for %s\n", r.URL.Path)
	}))
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Host is MANDATORY in HTTP/1.1 - it is how one IP serves many sites.
	// Connection: close tells the server not to keep the connection alive, so
	// reading to EOF gets us the whole response.
	req := "GET /hello HTTP/1.1\r\n" +
		"Host: example.local\r\n" +
		"User-Agent: hand-written/1.0\r\n" +
		"Connection: close\r\n" +
		"\r\n"

	fmt.Print("--- request bytes ---\n", req)
	if _, err := io.WriteString(conn, req); err != nil {
		log.Fatal(err)
	}

	resp, err := io.ReadAll(conn)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("--- response bytes ---")
	// Show CRLFs explicitly: every header line really does end with \r\n.
	fmt.Print(strings.ReplaceAll(string(resp), "\r\n", "\\r\\n\n"))
}
```

**Output:**

```
--- request bytes ---
GET /hello HTTP/1.1
Host: example.local
User-Agent: hand-written/1.0
Connection: close

--- response bytes ---
HTTP/1.1 200 OK\r\n
Date: Wed, 05 Aug 2026 15:28:52 GMT\r\n
Content-Length: 21\r\n
Content-Type: text/plain; charset=utf-8\r\n
Connection: close\r\n
\r\n
you asked for /hello
```

---

## 2. Dumping requests and responses

`🟢 easy` · *the wire*

`httputil.DumpRequest`/`DumpResponse` render the exact bytes `net/http` parsed or produced — the fastest way to answer "what did we actually send?" without reaching for `tcpdump`.

**Steps:**

1. Dump the request from inside the handler, the response from the client.
2. Notice what Go added for you: `Content-Length` (it buffered and measured the body), `Content-Type` (sniffed — example 7), `Date`, `Accept-Encoding: gzip`, `User-Agent`.
3. The parsed view (`resp.StatusCode`, `resp.Proto`) is the same information, structured.

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
)

// httputil.DumpRequest / DumpResponse show you the exact bytes net/http produced
// or parsed - the fastest way to answer "what did we actually send?".
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server sees the parsed request; dump it back to wire form.
		b, _ := httputil.DumpRequest(r, true)
		fmt.Printf("=== what the server received ===\n%s\n", b)

		w.Header().Set("X-Demo", "hello")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"ok":true}`)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/items?page=2", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// DumpResponse shows the status line + headers exactly as they arrived.
	b, _ := httputil.DumpResponse(resp, true)
	fmt.Printf("=== what the client received ===\n%s", b)

	// Note what net/http added for you: Content-Length (it buffered the small
	// body and measured it), Content-Type (sniffed from the first bytes), and
	// Date. You wrote none of those.
	fmt.Println("=== parsed by the client ===")
	fmt.Println("status code: ", resp.StatusCode)
	fmt.Println("proto:       ", resp.Proto)
	fmt.Println("content-type:", resp.Header.Get("Content-Type"))
}
```

**Output:**

```
=== what the server received ===
POST /items?page=2 HTTP/1.1
Host: 127.0.0.1:61874
Accept: application/json
Accept-Encoding: gzip
Content-Length: 0
User-Agent: Go-http-client/1.1


=== what the client received ===
HTTP/1.1 201 Created
Content-Length: 12
Content-Type: text/plain; charset=utf-8
Date: Wed, 05 Aug 2026 15:28:53 GMT
X-Demo: hello

{"ok":true}
=== parsed by the client ===
status code:  201
proto:        HTTP/1.1
content-type: text/plain; charset=utf-8
```

> Output note: `Date` headers and the ephemeral port change on every run.

---

## 3. Content-Length vs chunked

`🟢 easy` · *framing*

TCP has no message boundaries ([lesson 63](../63-networking-fundamentals/)), so HTTP must say where the body ends: **`Content-Length: N`** (read exactly N bytes) or **`Transfer-Encoding: chunked`** (framed chunks until a zero-length one). **Go chooses for you** — and the choice is visible on the wire.

**Steps:**

1. A small handler that returns immediately: Go buffers the body, measures it, sends `Content-Length`.
2. A handler that `Flush`es: Go cannot know the total, so it switches to **chunked**.
3. Both fetched over a raw socket so you see the real headers, not the parsed view.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// TCP has no message boundaries (lesson 63), so HTTP must say where the body
// ends. Two mechanisms:
//
//	Content-Length: N          -> read exactly N bytes
//	Transfer-Encoding: chunked -> read framed chunks until a zero-length one
//
// Go picks for you: a handler that returns quickly gets its small body buffered
// and measured (Content-Length); one that flushes or streams goes chunked.
func rawGet(addr, path string) string {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", path)
	b, _ := io.ReadAll(conn)
	return string(b)
}

func main() {
	mux := http.NewServeMux()

	// Small, returns immediately -> Go buffers it and sets Content-Length.
	mux.HandleFunc("GET /buffered", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	})

	// Flushes before returning -> Go cannot know the total length, so it
	// switches to chunked transfer encoding.
	mux.HandleFunc("GET /streamed", func(w http.ResponseWriter, r *http.Request) {
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "part%d ", i)
			w.(http.Flusher).Flush() // send what we have NOW
			time.Sleep(10 * time.Millisecond)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	for _, path := range []string{"/buffered", "/streamed"} {
		fmt.Printf("=== %s ===\n", path)
		raw := rawGet(addr, path)
		head, body, _ := strings.Cut(raw, "\r\n\r\n")
		for _, line := range strings.Split(head, "\r\n") {
			if line != "" {
				fmt.Println(" ", line)
			}
		}
		fmt.Printf("  body bytes: %q\n\n", body)
	}
}
```

**Output:**

```
=== /buffered ===
  HTTP/1.1 200 OK
  Date: Wed, 05 Aug 2026 15:28:53 GMT
  Content-Length: 5
  Content-Type: text/plain; charset=utf-8
  Connection: close
  body bytes: "hello"

=== /streamed ===
  HTTP/1.1 200 OK
  Date: Wed, 05 Aug 2026 15:28:53 GMT
  Content-Type: text/plain; charset=utf-8
  Connection: close
  Transfer-Encoding: chunked
  body bytes: "6\r\npart1 \r\n6\r\npart2 \r\n6\r\npart3 \r\n0\r\n\r\n"
```

---

## 4. Chunked encoding, decoded by hand

`🟢 easy` · *framing*

Chunked framing byte by byte: each chunk is `<length in hex>CRLF<payload>CRLF`, and the body ends with a zero-length chunk. It is exactly the length-prefix framing from [lesson 63, example 8](../63-networking-fundamentals/3-hard.md) with a hex header — the mechanism behind every streaming response.

**Steps:**

1. Each `Flush()` in the handler produces one chunk on the wire.
2. The raw body shows `6\r\nhello \r\n8\r\nchunked \r\n…0\r\n\r\n`.
3. The decoder loop reads a hex size, then that many bytes, until size 0.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Chunked framing, byte by byte. Each chunk is:
//
//	<length in HEX> CRLF
//	<that many bytes> CRLF
//
// and the body ends with a zero-length chunk:  0 CRLF CRLF
//
// This is length-prefix framing (lesson 63, example 8) with a hex header - it is
// how a server streams a response whose total size it does not know yet.
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, part := range []string{"hello ", "chunked ", "world"} {
			fmt.Fprint(w, part)
			w.(http.Flusher).Flush() // each Flush = one chunk on the wire
		}
	}))
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")

	raw, err := io.ReadAll(conn)
	if err != nil {
		log.Fatal(err)
	}
	head, body, _ := strings.Cut(string(raw), "\r\n\r\n")

	fmt.Println("--- headers ---")
	for _, l := range strings.Split(head, "\r\n") {
		fmt.Println(" ", l)
	}

	fmt.Println("--- raw body (CRLF shown) ---")
	fmt.Println(strings.ReplaceAll(body, "\r\n", "\\r\\n"))

	fmt.Println("--- decoded ---")
	// "6\r\nhello \r\n8\r\nchunked \r\n5\r\nworld\r\n0\r\n\r\n"
	rest := body
	for {
		sizeLine, after, ok := strings.Cut(rest, "\r\n")
		if !ok {
			break
		}
		var n int
		if _, err := fmt.Sscanf(sizeLine, "%x", &n); err != nil {
			break
		}
		if n == 0 {
			fmt.Println("  chunk of 0 bytes -> end of body")
			break
		}
		fmt.Printf("  chunk of %d bytes: %q\n", n, after[:n])
		rest = after[n+2:] // skip the payload and its trailing CRLF
	}
	// net/http does all of this for you: resp.Body already yields the decoded
	// stream, and Transfer-Encoding never appears in resp.Header.
}
```

**Output:**

```
--- headers ---
  HTTP/1.1 200 OK
  Date: Wed, 05 Aug 2026 15:28:54 GMT
  Content-Type: text/plain; charset=utf-8
  Connection: close
  Transfer-Encoding: chunked
--- raw body (CRLF shown) ---
6\r\nhello \r\n8\r\nchunked \r\n5\r\nworld\r\n0\r\n\r\n
--- decoded ---
  chunk of 6 bytes: "hello "
  chunk of 8 bytes: "chunked "
  chunk of 5 bytes: "world"
  chunk of 0 bytes -> end of body
```

> `net/http` does all of this for you: `resp.Body` already yields the decoded stream, and `Transfer-Encoding` never appears in `resp.Header`.

---

## 5. Build the server explicitly — the five timeouts

`🟢 easy` · *server config*

`http.ListenAndServe(addr, h)` hides a **zero-value `http.Server`**, and a zero-value Server has **no timeouts at all**. One slow client then holds a connection, a goroutine and an fd indefinitely. Always construct the Server yourself.

**Steps:**

1. **`ReadHeaderTimeout`** — the cheapest big win (slow-loris defense, example 11).
2. **`ReadTimeout`** (headers + body), **`WriteTimeout`** (end of headers → end of response — the streaming trap, example 12).
3. **`IdleTimeout`** (how long a kept-alive connection may sit) and **`MaxHeaderBytes`**.
4. Pass your own `ServeMux`, never `DefaultServeMux` — any imported package can register routes on it (`net/http/pprof` does).

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// http.ListenAndServe(addr, h) is a two-line convenience with a zero-value
// http.Server behind it - and a zero-value Server has NO TIMEOUTS AT ALL. One
// slow or malicious client can then hold a connection (and a goroutine, and an
// fd) forever. Always construct the Server yourself.
func main() {
	mux := http.NewServeMux() // never DefaultServeMux: imported packages can add routes to it
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Handler: mux,

		// How long to wait for the REQUEST HEADERS. The cheapest big win:
		// without it, a client that dribbles headers ties up a connection
		// indefinitely (the slow-loris attack - example 11).
		ReadHeaderTimeout: 5 * time.Second,

		// Headers + body. Cap it, but remember large uploads need room.
		ReadTimeout: 30 * time.Second,

		// From the end of the request headers to the end of the response.
		// TRAP: this kills streaming/SSE/long downloads - see example 12.
		WriteTimeout: 30 * time.Second,

		// How long an idle keep-alive connection may sit before being closed.
		IdleTimeout: 120 * time.Second,

		// Bound the header size (default 1 MB).
		MaxHeaderBytes: 1 << 20,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go srv.Serve(ln) // ListenAndServe = net.Listen + Serve

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()

	fmt.Println("ReadHeaderTimeout:", srv.ReadHeaderTimeout)
	fmt.Println("ReadTimeout:      ", srv.ReadTimeout)
	fmt.Println("WriteTimeout:     ", srv.WriteTimeout)
	fmt.Println("IdleTimeout:      ", srv.IdleTimeout)
	fmt.Println("MaxHeaderBytes:   ", srv.MaxHeaderBytes)
	fmt.Println("request served:   ", resp.Status)

	// Graceful shutdown drains in-flight requests (lesson 62).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("shutdown:          clean")
}
```

**Output:**

```
ReadHeaderTimeout: 5s
ReadTimeout:       30s
WriteTimeout:      30s
IdleTimeout:       2m0s
MaxHeaderBytes:    1048576
request served:    200 OK
shutdown:          clean
```

---

## 6. Headers must be set before the first Write

`🟢 easy` · *responsewriter*

The `ResponseWriter` contract is ordered: header mutations → `WriteHeader(status)` → body. The first `Write` calls `WriteHeader(200)` implicitly, and from that moment **the header map is frozen**. Later `Set` calls are a **silent no-op** — no error, no panic, just a header that never arrives.

**Steps:**

1. `/wrong` writes the body first, then sets a header and a status — both ignored.
2. `/right` sets everything up front and gets a 418 with the header present.
3. Go logs `superfluous response.WriteHeader call` for the late status — the only hint you get.

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
)

// The ResponseWriter contract, in order:
//
//  1. w.Header().Set(...)   - all header mutations
//  2. w.WriteHeader(status) - status line + headers go on the wire
//  3. w.Write(body)         - body bytes
//
// After step 2 (which the first Write triggers implicitly with 200) the header
// map is FROZEN. Later Set calls are a silent no-op - no error, no panic, just a
// header that never arrives. This is a rite of passage; see it once, remember it.
func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /wrong", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "body first")             // implicit WriteHeader(200): headers locked
		w.Header().Set("X-Too-Late", "ignored") // silently discarded
		w.WriteHeader(http.StatusTeapot)        // also too late (logs "superfluous WriteHeader")
	})

	mux.HandleFunc("GET /right", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-In-Time", "present")
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "body last")
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{"/wrong", "/right"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-7s status=%d X-Too-Late=%q X-In-Time=%q\n",
			path, resp.StatusCode,
			resp.Header.Get("X-Too-Late"), resp.Header.Get("X-In-Time"))
		resp.Body.Close()
	}

	// Corollary for middleware: you cannot decide to add a header (or change the
	// status) after the handler has started writing. Buffer the response, or set
	// what you need up front.
}
```

**Output:**

```
2026/08/05 22:28:55 http: superfluous response.WriteHeader call from main.main.func1 (main.go:25)
/wrong  status=200 X-Too-Late="" X-In-Time=""
/right  status=418 X-Too-Late="" X-In-Time="present"
```

> Corollary for middleware: you cannot add a header or change the status once the handler has started writing. Output note: the log line carries a timestamp that varies.

---

## 7. Content-Type sniffing

`🟢 easy` · *responsewriter*

If you do not set `Content-Type`, `net/http` **sniffs** it from the first 512 bytes (`http.DetectContentType`). Convenient — and a hazard: user-uploaded content sniffed as `text/html` is a stored-XSS vector, which is why `X-Content-Type-Options: nosniff` exists ([lesson 57](../57-web-security/)).

**Steps:**

1. HTML sniffs correctly; **JSON does not** — there is no JSON signature, so it comes out as `text/plain`.
2. Set it explicitly (plus `nosniff`) and the guessing stops.
3. `DetectContentType` is exported, so you can ask it directly.

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
)

// If you do not set Content-Type, net/http SNIFFS it from the first 512 bytes of
// the body (http.DetectContentType, the WHATWG algorithm). Handy - and a hazard:
// user-uploaded content sniffed as text/html is a stored-XSS vector, which is
// why you send X-Content-Type-Options: nosniff (lesson 57).
func main() {
	mux := http.NewServeMux()
	bodies := map[string]string{
		"/json": `{"id":1,"name":"ada"}`,
		"/html": `<!doctype html><h1>hi</h1>`,
		"/text": "just some words",
	}
	for path, body := range bodies {
		b := body
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, b) // no Content-Type set on purpose
		})
	}
	// The fix: say what it is.
	mux.HandleFunc("GET /json-correct", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fmt.Fprint(w, bodies["/json"])
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{"/json", "/html", "/text", "/json-correct"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()
		fmt.Printf("%-14s -> %s\n", path, resp.Header.Get("Content-Type"))
	}

	// DetectContentType is exported, so you can ask it directly.
	fmt.Println()
	for _, s := range []string{`{"a":1}`, "<!doctype html>", "GIF89a", "plain"} {
		fmt.Printf("DetectContentType(%-16q) = %s\n", s, http.DetectContentType([]byte(s)))
	}
	// Note JSON sniffs as text/plain: there is no JSON signature. Always set it.
}
```

**Output:**

```
/json          -> text/plain; charset=utf-8
/html          -> text/html; charset=utf-8
/text          -> text/plain; charset=utf-8
/json-correct  -> application/json

DetectContentType("{\"a\":1}"     ) = text/plain; charset=utf-8
DetectContentType("<!doctype html>") = text/html; charset=utf-8
DetectContentType("GIF89a"        ) = image/gif
DetectContentType("plain"         ) = text/plain; charset=utf-8
```

---

## 8. Everything the server knows about a request

`🟢 easy` · *request*

Where each piece comes from: the **request line** gives Method/URL/Proto, **`Host` is a header promoted to its own field**, `RemoteAddr` is the immediate peer (a proxy's IP if you are behind one — example 23), and the body is a **one-shot stream**.

**Steps:**

1. `r.URL.Query()` parses lazily; `r.Header` is a `map[string][]string` with canonicalised keys.
2. `r.Header.Get("Host")` is **empty** — it moved to `r.Host`.
3. **Cap the body** with `http.MaxBytesReader` before reading it: the length is attacker-controlled ([lesson 57](../57-web-security/)).
4. Reading the body twice yields nothing the second time — buffer it if middleware and handler both need it.
5. `r.Context()` is cancelled when the client disconnects — pass it to every downstream call.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
)

// Everything the server knows about a request, and where it comes from.
// Note the split: the request LINE gives you Method/URL/Proto, the Host is a
// HEADER promoted to its own field, and the body is a one-shot STREAM.
func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Method:    ", r.Method)
		fmt.Println("URL.Path:  ", r.URL.Path)
		fmt.Println("RawQuery:  ", r.URL.RawQuery)
		fmt.Println("query q:   ", r.URL.Query().Get("q")) // parsed on demand
		fmt.Println("Proto:     ", r.Proto, "/ major", r.ProtoMajor)

		// Host is NOT in r.Header - HTTP/1.1 promotes it to a field.
		fmt.Println("Host:      ", r.Host != "", "(the header, promoted to a field)")
		fmt.Println("Header Host:", r.Header.Get("Host") == "", "(empty: it moved)")

		// RemoteAddr is the immediate peer - a proxy's IP if you are behind one
		// (example 23).
		fmt.Println("RemoteAddr is loopback:", strings.HasPrefix(r.RemoteAddr, "127.0.0.1"))

		var names []string
		for k := range r.Header {
			names = append(names, k)
		}
		sort.Strings(names)
		fmt.Println("headers:   ", names)

		// The body is a stream you may read ONCE. Always cap it before reading
		// (lesson 57) - the length is attacker-controlled.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		fmt.Printf("body:       %q\n", body)

		second, _ := io.ReadAll(r.Body)
		fmt.Printf("second read: %q (a stream is consumed once)\n", second)

		// The request context is cancelled when the client disconnects - pass it
		// to every downstream call (lesson 15).
		fmt.Println("ctx done?  ", r.Context().Err())

		fmt.Fprintln(w, "ok")
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/users/7?q=hello&page=2",
		strings.NewReader("payload bytes"))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Request-Id", "abc-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}
```

**Output:**

```
Method:     PUT
URL.Path:   /users/7
RawQuery:   q=hello&page=2
query q:    hello
Proto:      HTTP/1.1 / major 1
Host:       true (the header, promoted to a field)
Header Host: true (empty: it moved)
RemoteAddr is loopback: true
headers:    [Accept-Encoding Content-Length Content-Type User-Agent X-Request-Id]
body:       "payload bytes"
second read: "" (a stream is consumed once)
ctx done?   <nil>
```

---
