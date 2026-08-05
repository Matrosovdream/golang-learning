# Step 63 — Networking Fundamentals · 🔴 Hard

Examples **18–26**. Other transports, **error triage**, reuse economics, socket options, the netpoller, and a proxy.
Every example is a self-contained program that runs both the server and the client, so
`go run main.go` shows the whole exchange. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Previous tier: [🟡 medium](2-medium.md)

---

## 18. Unix domain sockets

`🔴 hard` · *unix socket*

The same `net.Listener`/`net.Conn` API with no TCP/IP underneath: no handshake, no checksums, no port — just a **file path** guarded by filesystem permissions, and unreachable from the network entirely. The classic pairing is nginx in front, Go app on a socket (lesson 66).

**Steps:**

1. `net.Listen("unix", path)` — remove a **stale socket file** first, or `bind` fails with "address already in use".
2. `os.Chmod` the socket so only the proxy's user can connect — this replaces firewall rules.
3. `net.Dial("unix", path)`: no host, no port. Everything above still applies unchanged.

```go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
)

// A Unix domain socket is the same net.Listener / net.Conn API with no TCP/IP
// underneath: no handshake, no checksums, no port - just a file path, guarded by
// filesystem permissions. The usual pairing is "nginx in front, Go app on a
// socket", which cannot be reached from the network at all.
func main() {
	sockPath := filepath.Join(os.TempDir(), "ex63-demo.sock")

	// A crashed process leaves the socket file behind and bind() then fails with
	// "address already in use". Remove the stale file first.
	if err := os.Remove(sockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatal(err)
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close() // also unlinks the file (Go sets unlinkOnClose for us)

	// Only the proxy's user should be able to talk to us.
	if err := os.Chmod(sockPath, 0o660); err != nil {
		log.Fatal(err)
	}
	fi, err := os.Stat(sockPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("network:    ", ln.Addr().Network())
	fmt.Println("is a socket:", fi.Mode()&fs.ModeSocket != 0)
	fmt.Printf("permissions: %v\n", fi.Mode().Perm())

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		fmt.Fprintf(conn, "pong: %s", line)
	}()

	conn, err := net.Dial("unix", sockPath) // no host, no port
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "ping\n")
	reply, _ := bufio.NewReader(conn).ReadString('\n')
	fmt.Printf("round trip:  %q\n", reply)
}
```

**Output:**

```
network:     unix
is a socket: true
permissions: -rw-rw----
round trip:  "pong: ping\n"
```

---

## 19. Dialing with a context

`🔴 hard` · *context*

`net.Dial` cannot be cancelled. **`net.Dialer.DialContext`** can — and it is what belongs in a service, so a client that gave up (or a request whose deadline expired) does not leave you dialling into the void.

**Steps:**

1. An already-cancelled context fails before the dial starts: `errors.Is(err, context.Canceled)`.
2. An expired deadline behaves the same: `errors.Is(err, context.DeadlineExceeded)`.
3. `Dialer.Timeout` caps the dial itself (DNS + handshake), independently of later read/write deadlines.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// net.Dial has no way to be cancelled. net.Dialer.DialContext does - and it is
// what you want everywhere in a service, so a client that gives up (or a request
// whose deadline expired) does not leave you dialling into the void.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	var d net.Dialer

	// 1. A context that is already cancelled: the dial never even starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = d.DialContext(ctx, "tcp", ln.Addr().String())
	fmt.Println("cancelled ctx:      ", err)
	fmt.Println("  is context.Canceled:", errors.Is(err, context.Canceled))

	// 2. An expired deadline behaves the same way.
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel2()
	time.Sleep(time.Millisecond)
	_, err = d.DialContext(ctx2, "tcp", ln.Addr().String())
	fmt.Println("expired ctx:        ", err)
	fmt.Println("  is DeadlineExceeded:", errors.Is(err, context.DeadlineExceeded))

	// 3. A live context dials normally. Dialer.Timeout caps the dial itself
	//    (handshake + DNS), independent of any read/write deadlines later.
	d.Timeout = 2 * time.Second
	ctx3, cancel3 := context.WithTimeout(context.Background(), time.Second)
	defer cancel3()
	conn, err := d.DialContext(ctx3, "tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	fmt.Println("live ctx:            connected ok")
}
```

**Output:**

```
cancelled ctx:       dial tcp 127.0.0.1:59311: operation was canceled
  is context.Canceled: true
expired ctx:         dial tcp 127.0.0.1:59311: i/o timeout
  is DeadlineExceeded: true
live ctx:            connected ok
```

> Output note: the error text embeds the ephemeral port, so it varies per run.

---

## 20. DNS lookups you can cancel

`🔴 hard` · *dns*

Every `Dial` with a hostname resolves first. Use **`net.Resolver`** with a context so lookups are cancellable — the package-level helpers (`net.LookupHost`) are not. Go has two resolvers: the **pure-Go** one (parses `/etc/resolv.conf`, works in a `scratch` container) and the **cgo** one (`getaddrinfo`, honours NSS). `CGO_ENABLED=0` always gets pure Go; inspect with `GODEBUG=netdns=2`.

**Steps:**

1. `r.LookupHost(ctx, name)` returns every A/AAAA address.
2. A failed lookup gives **`*net.DNSError`**: `IsNotFound` is permanent (do not retry), `IsTimeout`/`IsTemporary` are worth a retry (lesson 36).
3. `net.LookupPort` resolves service names from `/etc/services`.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"time"
)

// Every net.Dial with a hostname does a DNS lookup first. Use net.Resolver with
// a context so lookups are cancellable - the default package-level helpers
// (net.LookupHost) are not.
//
// Go has two resolvers: the pure-Go one (reads /etc/resolv.conf itself, works in
// a scratch container) and the cgo one (calls getaddrinfo, honours nsswitch).
// CGO_ENABLED=0 builds always get the pure-Go one. Inspect the choice with
// GODEBUG=netdns=2, force it with GODEBUG=netdns=go or netdns=cgo.
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := &net.Resolver{} // zero value = the default resolver

	addrs, err := r.LookupHost(ctx, "localhost")
	if err != nil {
		fmt.Println("lookup failed:", err)
		return
	}
	sort.Strings(addrs) // the order is not stable across machines
	fmt.Println("localhost ->", addrs)

	// A name that cannot exist (.invalid is reserved by RFC 2606).
	_, err = r.LookupHost(ctx, "no-such-name.invalid")
	fmt.Println("bad name  ->", err)

	// *net.DNSError tells you WHY, which decides whether a retry makes sense.
	var derr *net.DNSError
	if errors.As(err, &derr) {
		fmt.Println("  IsNotFound: ", derr.IsNotFound)  // permanent: do not retry
		fmt.Println("  IsTimeout:  ", derr.IsTimeout)   // transient: retry
		fmt.Println("  IsTemporary:", derr.IsTemporary) // transient: retry
	}

	// Service names resolve too, from /etc/services.
	port, err := net.LookupPort("tcp", "https")
	fmt.Println("tcp/https ->", port, err)
}
```

**Output:**

```
localhost -> [127.0.0.1 ::1]
bad name  -> lookup no-such-name.invalid: no such host
  IsNotFound:  true
  IsTimeout:   false
  IsTemporary: false
tcp/https -> 443 <nil>
```

> Output note: the address order for `localhost` (and whether `::1` appears) depends on the machine's resolver config.

---

## 21. Reading network errors: refused / timeout / EOF / reset

`🔴 hard` · *errors*

The four errors you will actually meet, produced on purpose in one program. Getting this reflex right saves hours — **the error already tells you which layer broke**.

**Steps:**

1. **refused** — the packet arrived and nothing was listening: wrong port, wrong interface (`127.0.0.1` vs `0.0.0.0`), service not up. `errors.Is(err, syscall.ECONNREFUSED)`.
2. **timeout** — packets vanished (firewall DROP, wrong host, MTU). The only one of the four worth retrying.
3. **EOF** — a clean FIN. Normal end of stream.
4. **reset** — a rude RST: a crash, an LB idle timeout, or `SetLinger(0)` as here. `errors.Is(err, syscall.ECONNRESET)`.

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"
)

// The four network errors you will actually meet, and what each one means.
// Getting this reflex right saves hours: the error already tells you which layer
// broke.
func main() {
	// 1. CONNECTION REFUSED - the packet arrived, nothing is listening there.
	//    Wrong port, wrong interface (127.0.0.1 vs 0.0.0.0), service not up yet.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := ln.Addr().String()
	ln.Close() // now nobody is listening on that port
	_, err := net.Dial("tcp", deadAddr)
	fmt.Println("1. refused:", err)
	fmt.Println("   ECONNREFUSED:", errors.Is(err, syscall.ECONNREFUSED))

	// 2. TIMEOUT - packets went into a black hole (firewall DROP, wrong host,
	//    MTU). Retryable, unlike the others.
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln2.Close()
	go ln2.Accept()
	c2, err := net.Dial("tcp", ln2.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer c2.Close()
	c2.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, err = c2.Read(make([]byte, 1))
	var nerr net.Error
	fmt.Println("2. timeout:", err)
	fmt.Println("   Timeout():", errors.As(err, &nerr) && nerr.Timeout(), "(retry this one)")

	// 3. EOF - the peer closed CLEANLY (FIN). Normal end of stream.
	ln3, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln3.Close()
	go func() {
		c, err := ln3.Accept()
		if err == nil {
			c.Close()
		}
	}()
	c3, _ := net.Dial("tcp", ln3.Addr().String())
	defer c3.Close()
	_, err = c3.Read(make([]byte, 1))
	fmt.Println("3. clean close:", err, "- io.EOF:", errors.Is(err, io.EOF))

	// 4. CONNECTION RESET - the peer closed RUDELY (RST): a crash, a proxy idle
	//    timeout, or an explicit SetLinger(0) like this one.
	ln4, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln4.Close()
	go func() {
		c, err := ln4.Accept()
		if err != nil {
			return
		}
		c.(*net.TCPConn).SetLinger(0) // close with RST instead of FIN
		c.Close()
	}()
	c4, _ := net.Dial("tcp", ln4.Addr().String())
	defer c4.Close()
	time.Sleep(50 * time.Millisecond)
	c4.Write([]byte("x")) // provokes the RST to surface
	_, err = c4.Read(make([]byte, 1))
	fmt.Println("4. reset:", err)
	fmt.Println("   ECONNRESET:", errors.Is(err, syscall.ECONNRESET))
}
```

**Output:**

```
1. refused: dial tcp 127.0.0.1:59315: connect: connection refused
   ECONNREFUSED: true
2. timeout: read tcp 127.0.0.1:59318->127.0.0.1:59317: i/o timeout
   Timeout(): true (retry this one)
3. clean close: EOF - io.EOF: true
4. reset: read tcp 127.0.0.1:59326->127.0.0.1:59325: read: connection reset by peer
   ECONNRESET: true
```

> A fifth, `no such host`, is DNS — see example 20. Output note: ports in the error text vary per run.

---

## 22. Connection reuse vs dial-per-request

`🔴 hard` · *reuse*

Every new connection costs a 3-way handshake — a full round trip, plus a TLS handshake for HTTPS — and leaves the closing side in **`TIME_WAIT`** for ~60s holding the tuple. Counting accepts on the server makes the difference impossible to miss.

**Steps:**

1. 100 requests, dial-per-request: **100 handshakes**.
2. 100 requests over one connection: **1 handshake**.
3. This is precisely why `http.Transport` pools connections — and why failing to drain and close a response body, which prevents reuse, is such an expensive bug (lesson 64).

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync/atomic"
)

const requests = 100

// Every new connection costs a 3-way handshake (a full round trip, plus a TLS
// handshake on top for HTTPS) and leaves the closing side in TIME_WAIT for ~60s,
// holding the port tuple. Reusing one connection avoids both.
//
// This is exactly why http.Transport keeps a connection pool - and why failing
// to drain and close a response body, which stops that reuse, is such an
// expensive bug (lesson 64).
func main() {
	var accepts int64

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&accepts, 1) // count handshakes
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					fmt.Fprintf(conn, "ok %s\n", sc.Text())
				}
			}()
		}
	}()

	// A) dial per request - a handshake every time.
	for i := 0; i < requests; i++ {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(conn, "%d\n", i)
		bufio.NewReader(conn).ReadString('\n')
		conn.Close()
	}
	perRequest := atomic.SwapInt64(&accepts, 0)

	// B) one connection, many requests - one handshake total.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	r := bufio.NewReader(conn)
	for i := 0; i < requests; i++ {
		fmt.Fprintf(conn, "%d\n", i)
		if _, err := r.ReadString('\n'); err != nil {
			log.Fatal(err)
		}
	}
	conn.Close()
	reused := atomic.LoadInt64(&accepts)

	fmt.Println("requests sent:              ", requests)
	fmt.Println("handshakes, dial-per-request:", perRequest)
	fmt.Println("handshakes, one connection:  ", reused)
	fmt.Println("=> reuse turned", perRequest, "handshakes into", reused)
}
```

**Output:**

```
requests sent:               100
handshakes, dial-per-request: 100
handshakes, one connection:   1
=> reuse turned 100 handshakes into 1
```

---

## 23. Socket options: Dialer, ListenConfig and Control

`🔴 hard` · *sockopts*

Common knobs live on the typed connection (`*net.TCPConn`) or on `Dialer`/`ListenConfig`; anything else is reachable through a **`Control`** function, which runs on the raw fd *after* `socket()` and *before* `bind()`/`connect()` — the only window where options like `SO_REUSEADDR`/`SO_REUSEPORT` can be set (lesson 66).

**Steps:**

1. `ListenConfig.Control` + `syscall.SetsockoptInt` for socket-level flags.
2. `Dialer.Timeout` (caps DNS + handshake) and `Dialer.KeepAlive` (probe idle connections).
3. On the conn: `SetNoDelay` (Nagle — Go disables it by default, favouring latency), `SetReadBuffer`/`SetWriteBuffer`, `SetKeepAlive`/`SetKeepAlivePeriod` for peers that vanish without a FIN.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

// Socket options, the Go way. Common knobs live on the typed connection
// (*net.TCPConn) or on the Dialer/ListenConfig; anything else is reachable
// through a Control function, which runs on the raw fd before bind/connect.
func main() {
	// ListenConfig.Control runs after socket() and before bind(): the only place
	// where options like SO_REUSEADDR / SO_REUSEPORT can be set (lesson 66).
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	fmt.Println("SO_REUSEADDR set via ListenConfig.Control: ok")

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(100 * time.Millisecond)
	}()

	// Dialer carries the client-side knobs.
	d := net.Dialer{
		Timeout:   2 * time.Second,  // caps DNS + handshake
		KeepAlive: 30 * time.Second, // probe idle connections (0 = default, -1 = off)
	}
	conn, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	tcp := conn.(*net.TCPConn)

	// Nagle batches small writes to save packets; Go DISABLES it by default
	// because interactive protocols care about latency more than packet count.
	if err := tcp.SetNoDelay(true); err != nil { // true = Nagle off (the default)
		log.Fatal(err)
	}
	// Socket buffers bound in-flight bytes; bigger helps high-latency/high-
	// bandwidth links, and costs kernel memory per connection.
	if err := tcp.SetReadBuffer(64 * 1024); err != nil {
		log.Fatal(err)
	}
	if err := tcp.SetWriteBuffer(64 * 1024); err != nil {
		log.Fatal(err)
	}
	// Keepalive detects peers that vanished without a FIN (NAT/firewall drops).
	if err := tcp.SetKeepAlive(true); err != nil {
		log.Fatal(err)
	}
	if err := tcp.SetKeepAlivePeriod(30 * time.Second); err != nil {
		log.Fatal(err)
	}

	fmt.Println("dialer timeout:            ", d.Timeout)
	fmt.Println("dialer keepalive:          ", d.KeepAlive)
	fmt.Println("NoDelay / buffers / keepalive on the conn: ok")
}
```

**Output:**

```
SO_REUSEADDR set via ListenConfig.Control: ok
dialer timeout:             2s
dialer keepalive:           30s
NoDelay / buffers / keepalive on the conn: ok
```

---

## 24. 2000 connections, 2000 goroutines

`🔴 hard` · *netpoller*

Why "one goroutine per connection" is not the mistake it would be in C. A goroutine blocked in `Read` costs a few KB of stack and **no thread**: the runtime parks it and registers the non-blocking fd with the **netpoller** (epoll on Linux, kqueue on BSD/macOS), which wakes it only when bytes actually arrive.

**Steps:**

1. Open 2000 connections; the server spawns one goroutine each.
2. `runtime.NumGoroutine()` shows ~2002 — while OS threads stay at `GOMAXPROCS`.
3. Heap cost is roughly 2 KB per parked connection.

```go
package main

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

const conns = 2000

// Why "one goroutine per connection" is not the mistake it would be in C.
//
// A goroutine blocked in Read costs a few KB of stack and NO thread: the runtime
// parks it and registers the (non-blocking) fd with the netpoller - epoll on
// Linux, kqueue on BSD/macOS - which wakes it only when bytes actually arrive.
// So 2000 idle connections are 2000 parked goroutines on a handful of threads.
func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	var live int64

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&live, 1)
			go func() { // one goroutine per connection, blocked in Read
				defer conn.Close()
				conn.Read(make([]byte, 1))
			}()
		}
	}()

	before := runtime.NumGoroutine()
	clients := make([]net.Conn, 0, conns)
	for i := 0; i < conns; i++ {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			log.Fatalf("dial %d: %v (raise ulimit -n?)", i, err)
		}
		clients = append(clients, c)
	}
	for atomic.LoadInt64(&live) < conns { // wait for the server to catch up
		time.Sleep(time.Millisecond)
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Println("connections open:  ", atomic.LoadInt64(&live))
	fmt.Println("goroutines before: ", before)
	fmt.Println("goroutines now:    ", runtime.NumGoroutine())
	fmt.Println("OS threads:        ", runtime.GOMAXPROCS(0), "GOMAXPROCS (not one per conn!)")
	fmt.Printf("heap in use:        %d KB (~%d bytes per connection)\n",
		ms.HeapInuse/1024, int(ms.HeapInuse)/conns)

	for _, c := range clients {
		c.Close()
	}
	// Each connection is also an open FILE DESCRIPTOR: `ulimit -n` is the real
	// ceiling, and a leaked Close shows up as "too many open files".
}
```

**Output:**

```
connections open:   2000
goroutines before:  2
goroutines now:     2002
OS threads:         10 GOMAXPROCS (not one per conn!)
heap in use:        3848 KB (~1970 bytes per connection)
```

> Numbers vary by machine and Go version. The real ceiling is **file descriptors** (`ulimit -n`), not goroutines — and a leaked `Close` surfaces as "too many open files".

---

## 25. A TCP proxy in two io.Copy calls

`🔴 hard` · *proxy*

A protocol-agnostic proxy is two `io.Copy`s: everything the client says goes upstream, everything upstream says comes back. `io.Copy` loops until EOF reusing one buffer, so this handles any stream size without understanding the protocol at all.

**Steps:**

1. Dial upstream, then run both directions concurrently.
2. **Half-close each direction as it finishes** (`CloseWrite`) so the far end sees a clean EOF rather than a dropped connection.
3. `wg.Wait()` before closing both conns.

```go
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// A TCP proxy is two io.Copy calls: everything the client says goes upstream,
// everything upstream says goes back. io.Copy loops until EOF and reuses one
// buffer, so this handles any size of stream without knowing the protocol.
//
// The detail that matters: half-close each direction as it finishes (CloseWrite),
// so the far end sees a clean EOF instead of the whole connection dropping.
func proxy(client net.Conn, upstreamAddr string) {
	defer client.Close()

	upstream, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		log.Print(err)
		return
	}
	defer upstream.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // client -> upstream
		defer wg.Done()
		io.Copy(upstream, client)
		if c, ok := upstream.(*net.TCPConn); ok {
			c.CloseWrite()
		}
	}()
	go func() { // upstream -> client
		defer wg.Done()
		io.Copy(client, upstream)
		if c, ok := client.(*net.TCPConn); ok {
			c.CloseWrite()
		}
	}()
	wg.Wait()
}

func main() {
	// The origin: uppercases each line.
	origin, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer origin.Close()
	go func() {
		for {
			conn, err := origin.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					fmt.Fprintf(conn, "ORIGIN[%s]\n", sc.Text())
				}
			}()
		}
	}()

	// The proxy.
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer front.Close()
	go func() {
		for {
			conn, err := front.Accept()
			if err != nil {
				return
			}
			go proxy(conn, origin.Addr().String())
		}
	}()

	// A client that only ever talks to the proxy.
	conn, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	fmt.Fprint(conn, "hello\nthrough\nthe proxy\n")
	conn.(*net.TCPConn).CloseWrite()

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		fmt.Println("client got:", sc.Text())
	}
}
```

**Output:**

```
client got: ORIGIN[hello]
client got: ORIGIN[through]
client got: ORIGIN[the proxy]
```

> The HTTP-aware version of this is `httputil.ReverseProxy` (lesson 64); the deployment version is nginx/Caddy/Traefik (lesson 66).

---

## 26. Capstone: a line-protocol key-value server

`🔴 hard` · *capstone*

Everything in the lesson, assembled: a `SET`/`GET`/`DEL`/`QUIT` server over newline framing, with an accept loop, goroutine-per-connection, a concurrency cap, per-command idle deadlines, and graceful shutdown that drains in flight work.

**Steps:**

1. **Framing** (ex. 7): `bufio.Scanner` with a bounded buffer; `bufio.Writer` + explicit `Flush`.
2. **Concurrency** (ex. 3, 15): goroutine per connection behind a semaphore; the store is an `RWMutex`-guarded map.
3. **Deadlines** (ex. 11, 12): the read deadline is refreshed per command, so an idle client is dropped.
4. **Shutdown** (ex. 14): `ln.Close()` → `Accept` returns `net.ErrClosed` → `wg.Wait()` drains → later dials are refused.
5. Exercised with one scripted session plus 20 concurrent clients.

```go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// CAPSTONE: a line-protocol key-value server that puts every piece of the lesson
// together.
//
//	Protocol (framing = newline-delimited, example 7):
//	  SET key value  -> OK
//	  GET key        -> VALUE <v> | NOTFOUND
//	  DEL key        -> OK | NOTFOUND
//	  QUIT           -> BYE, connection closed
//
//	Wiring: accept loop + goroutine per connection (ex. 3), a semaphore to cap
//	concurrency (ex. 15), an idle timeout per connection (ex. 12), and graceful
//	shutdown that stops accepting and drains in-flight clients (ex. 14).
const (
	idleTimeout   = 2 * time.Second
	maxConcurrent = 16
)

type store struct {
	mu sync.RWMutex
	m  map[string]string
}

func (s *store) set(k, v string) { s.mu.Lock(); s.m[k] = v; s.mu.Unlock() }
func (s *store) get(k string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[k]
	return v, ok
}
func (s *store) del(k string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[k]
	delete(s.m, k)
	return ok
}

type server struct {
	ln    net.Listener
	store *store
	sem   chan struct{}
	wg    sync.WaitGroup
}

func newServer(ln net.Listener) *server {
	return &server{
		ln:    ln,
		store: &store{m: make(map[string]string)},
		sem:   make(chan struct{}, maxConcurrent),
	}
}

func (s *server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return // graceful shutdown
			}
			log.Print("accept:", err)
			return
		}
		s.sem <- struct{}{}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }()
			s.handle(conn)
		}()
	}
}

func (s *server) handle(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), 64*1024) // bound the line length
	w := bufio.NewWriter(conn)
	defer w.Flush()

	for {
		conn.SetReadDeadline(time.Now().Add(idleTimeout)) // refresh per command
		if !sc.Scan() {
			return // EOF, idle timeout, or a closed connection
		}
		cmd, arg, _ := strings.Cut(strings.TrimSpace(sc.Text()), " ")

		switch strings.ToUpper(cmd) {
		case "SET":
			k, v, ok := strings.Cut(arg, " ")
			if !ok {
				fmt.Fprintln(w, "ERR usage: SET key value")
				break
			}
			s.store.set(k, v)
			fmt.Fprintln(w, "OK")
		case "GET":
			if v, ok := s.store.get(arg); ok {
				fmt.Fprintln(w, "VALUE", v)
			} else {
				fmt.Fprintln(w, "NOTFOUND")
			}
		case "DEL":
			if s.store.del(arg) {
				fmt.Fprintln(w, "OK")
			} else {
				fmt.Fprintln(w, "NOTFOUND")
			}
		case "QUIT":
			fmt.Fprintln(w, "BYE")
			w.Flush()
			return
		default:
			fmt.Fprintln(w, "ERR unknown command")
		}
		conn.SetWriteDeadline(time.Now().Add(idleTimeout))
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// shutdown stops accepting, then waits for in-flight connections to finish.
func (s *server) shutdown() {
	s.ln.Close()
	s.wg.Wait()
}

// client runs one session and returns the transcript.
func client(addr string, cmds []string) []string {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return []string{"dial: " + err.Error()}
	}
	defer conn.Close()

	var out []string
	r := bufio.NewReader(conn)
	for _, c := range cmds {
		if _, err := fmt.Fprintln(conn, c); err != nil {
			return out
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return out
		}
		out = append(out, fmt.Sprintf("%-18s -> %s", c, strings.TrimSpace(line)))
	}
	return out
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	srv := newServer(ln)
	go srv.serve()
	addr := ln.Addr().String()

	// One session that exercises the whole protocol.
	for _, line := range client(addr, []string{
		"SET greeting hello",
		"GET greeting",
		"GET missing",
		"DEL greeting",
		"DEL greeting",
		"BOGUS",
		"QUIT",
	}) {
		fmt.Println(line)
	}

	// 20 concurrent clients, each writing then reading back its own key.
	var wg sync.WaitGroup
	results := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k%02d", n)
			out := client(addr, []string{
				fmt.Sprintf("SET %s v%02d", key, n),
				fmt.Sprintf("GET %s", key),
			})
			results[n] = out[len(out)-1]
		}(i)
	}
	wg.Wait()
	sort.Strings(results)
	fmt.Println("\n20 concurrent clients, first and last:")
	fmt.Println(" ", results[0])
	fmt.Println(" ", results[len(results)-1])

	fmt.Println("\nshutting down...")
	srv.shutdown()
	fmt.Println("listener closed, all connections drained")

	if _, err := net.Dial("tcp", addr); err != nil {
		fmt.Println("post-shutdown dial refused: yes")
	}
}
```

**Output:**

```
SET greeting hello -> OK
GET greeting       -> VALUE hello
GET missing        -> NOTFOUND
DEL greeting       -> OK
DEL greeting       -> NOTFOUND
BOGUS              -> ERR unknown command
QUIT               -> BYE

20 concurrent clients, first and last:
  GET k00            -> VALUE v00
  GET k19            -> VALUE v19

shutting down...
listener closed, all connections drained
post-shutdown dial refused: yes
```

---
