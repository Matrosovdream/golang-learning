# Step 64 — The HTTP Protocol & `net/http` Internals · 🔴 Hard

Examples **18–26**. **TLS**, **HTTP/2** negotiation and multiplexing, proxies and forwarded headers, middleware, and a full capstone.
Every example runs its own server *and* client in one program, so `go run main.go`
shows both sides. All output below is real.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Previous tier: [🟡 medium](2-medium.md)

---

## 18. TLS end to end with a generated certificate

`🔴 hard` · *tls*

A full TLS setup with the certificate generated in-process — no `openssl`, no files. The handshake: **ClientHello** (carrying **SNI** = hostname and **ALPN** = protocols offered) → **ServerHello** + certificate chain → key agreement → encrypted data. The client verifies the chain to a trusted root **and** that the name it dialled appears in the cert's **SANs**.

**Steps:**

1. Generate an ECDSA key + self-signed cert with `DNSNames`/`IPAddresses` — **SANs decide validity**; `CommonName` alone has been ignored by clients for years.
2. Serve with `ServeTLS` and `MinVersion: tls.VersionTLS12` ([lesson 57](../57-web-security/)).
3. `r.TLS` reports the negotiated version, cipher suite, SNI and ALPN.
4. **The trap, live:** setting `TLSClientConfig` yourself switches off the Transport's automatic HTTP/2 — the ALPN line prints empty and the response is HTTP/1.1. `ForceAttemptHTTP2: true` restores it.
5. Without the CA in the client's pool: `x509: certificate signed by unknown authority` — the exact error every `scratch` container hits ([lesson 65](../65-docker-networking/)).

```go
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"
)

// TLS end to end, with a certificate generated in this program - no openssl, no
// files. The handshake in five steps:
//
//	ClientHello  (carries SNI = the hostname, and ALPN = protocols offered)
//	ServerHello  + certificate chain
//	key agreement -> Finished
//	encrypted application data
//
// The client verifies the chain to a trusted root AND that the name it dialled
// appears in the certificate's SANs.
func selfSigned() (tls.Certificate, *x509.CertPool) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Example Learning Co"}, CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		// SANs decide which names/IPs this cert is valid for. CommonName alone
		// has been ignored by clients for years.
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		log.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		log.Fatal(err)
	}
	pool := x509.NewCertPool() // our client's "trusted roots"
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func main() {
	cert, pool := selfSigned()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// r.TLS reports the negotiated connection state.
			fmt.Println("server sees TLS version:  ", tls.VersionName(r.TLS.Version))
			fmt.Println("server sees cipher suite: ", tls.CipherSuiteName(r.TLS.CipherSuite))
			fmt.Println("server sees SNI:          ", r.TLS.ServerName)
			fmt.Println("server sees ALPN:         ", r.TLS.NegotiatedProtocol)
			fmt.Fprintln(w, "hello over TLS")
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12, // never allow TLS 1.0/1.1 (lesson 57)
		},
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go srv.ServeTLS(ln, "", "") // certs come from TLSConfig
	defer srv.Close()

	url := fmt.Sprintf("https://localhost:%d/", ln.Addr().(*net.TCPAddr).Port)

	// NOTE the trap: setting TLSClientConfig yourself switches OFF the
	// Transport's automatic HTTP/2 support, so the client offers no ALPN
	// protocols at all - watch the "ALPN:" line print empty and the response
	// come back as HTTP/1.1.
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
	fmt.Println("client got:               ", resp.Status, "over", resp.Proto)

	// The one-word fix: ForceAttemptHTTP2 re-enables ALPN + h2 on a custom
	// Transport (example 19 covers negotiation properly).
	fmt.Println()
	h2client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
		},
		Timeout: 5 * time.Second,
	}
	resp2, err := h2client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	resp2.Body.Close()
	fmt.Println("with ForceAttemptHTTP2:   ", resp2.Status, "over", resp2.Proto)

	// Without the CA in the pool, verification fails - the error every scratch
	// container hits when it ships no CA bundle (lesson 65).
	_, err = (&http.Client{Timeout: 5 * time.Second}).Get(url)
	fmt.Println("\nwithout our CA in the trust store:")
	fmt.Println(" ", err)
}
```

**Output:**

```
server sees TLS version:   TLS 1.3
server sees cipher suite:  TLS_AES_128_GCM_SHA256
server sees SNI:           localhost
server sees ALPN:          
client got:                200 OK over HTTP/1.1

server sees TLS version:   TLS 1.3
server sees cipher suite:  TLS_AES_128_GCM_SHA256
server sees SNI:           localhost
server sees ALPN:          h2
with ForceAttemptHTTP2:    200 OK over HTTP/2.0

without our CA in the trust store:
  Get "https://localhost:62005/": tls: failed to verify certificate: x509: certificate signed by unknown authority
2026/08/05 22:29:03 http: TLS handshake error from 127.0.0.1:62013: remote error: tls: bad certificate
```

---

## 19. ALPN: where HTTP/2 actually comes from

`🔴 hard` · *http/2*

HTTP/2 is not negotiated by an `Upgrade` header — it comes from **ALPN**, a TLS extension. The client offers `["h2", "http/1.1"]` in its ClientHello and the server picks. No extra round trip, no configuration.

**Steps:**

1. **Cleartext** → HTTP/1.1: no TLS means no ALPN, so a plain `http.Server` cannot negotiate h2 (cleartext h2 needs **h2c**, [lesson 66](../66-serving-go-apps/)).
2. **TLS without h2 enabled** → ALPN picks `http/1.1`.
3. **TLS with h2 enabled** → ALPN picks `h2`, and `r.Proto` is `HTTP/2.0`.
4. Your handler never changes: the same `http.Handler` serves 1.1, 2 and 3.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
)

// Where does HTTP/2 come from? ALPN - Application-Layer Protocol Negotiation, a
// TLS extension. The client offers ["h2", "http/1.1"] in its ClientHello and the
// server picks. No extra round trip, no Upgrade header, no configuration.
//
// Consequences:
//   - Over TLS, Go gives you HTTP/2 automatically on both sides.
//   - Over CLEARTEXT there is no ALPN, so a plain http.Server is HTTP/1.1 only;
//     cleartext h2 ("h2c") needs golang.org/x/net/http2/h2c (lesson 66).
//   - Your handler does not change: the same http.Handler serves 1.1, 2 and 3.
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "served over %s (major %d)\n", r.Proto, r.ProtoMajor)
}

func main() {
	// 1. Cleartext: HTTP/1.1, no negotiation possible.
	plain := httptest.NewServer(http.HandlerFunc(handler))
	defer plain.Close()
	show("cleartext http://", plain.Client(), plain.URL)

	// 2. TLS without HTTP/2 enabled: ALPN picks http/1.1.
	tlsOnly := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer tlsOnly.Close()
	show("TLS, h2 disabled ", tlsOnly.Client(), tlsOnly.URL)

	// 3. TLS with HTTP/2 enabled: ALPN picks h2.
	h2 := httptest.NewUnstartedServer(http.HandlerFunc(handler))
	h2.EnableHTTP2 = true
	h2.StartTLS()
	defer h2.Close()
	show("TLS, h2 enabled  ", h2.Client(), h2.URL)

	// The client side has its own switch: Transport.ForceAttemptHTTP2 (on by
	// default for the zero-value Transport, off if you set TLSClientConfig
	// yourself - a classic "why is my client still 1.1?" trap).
	fmt.Println("\nnote: a custom Transport with TLSClientConfig set disables h2")
	fmt.Println("      unless you also set ForceAttemptHTTP2: true")
}

func show(label string, c *http.Client, url string) {
	resp, err := c.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%s -> resp.Proto=%-8s body: %s", label, resp.Proto, body)
}
```

**Output:**

```
cleartext http:// -> resp.Proto=HTTP/1.1 body: served over HTTP/1.1 (major 1)
TLS, h2 disabled  -> resp.Proto=HTTP/1.1 body: served over HTTP/1.1 (major 1)
TLS, h2 enabled   -> resp.Proto=HTTP/2.0 body: served over HTTP/2.0 (major 2)

note: a custom Transport with TLSClientConfig set disables h2
      unless you also set ForceAttemptHTTP2: true
```

---

## 20. Multiplexing: 20 concurrent requests, 1 connection

`🔴 hard` · *http/2*

HTTP/2's headline feature. One TCP connection carries many independent **streams** of binary frames, so 20 concurrent requests need **one** connection where HTTP/1.1 needed 20 (example 15C). Counting accepted TCP connections makes it visible.

**Steps:**

1. **Warm up first in both cases** — a burst issued before any connection exists makes the Transport dial several at once, which would hide the effect.
2. HTTP/1.1: 20 concurrent requests → 19 *new* connections.
3. HTTP/2: 20 concurrent requests → **0** new connections; all 20 streams rode the warm one.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// HTTP/2's headline feature: MULTIPLEXING. One TCP connection carries many
// independent STREAMS, each a sequence of binary frames (HEADERS, DATA, ...),
// so 20 concurrent requests need exactly one connection - where HTTP/1.1 needed
// 20 (example 15C).
//
// Counting accepted TCP connections makes it visible.
func countingServer(enableH2 bool) (*httptest.Server, *int64) {
	var accepts int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // hold each request open so they overlap
		fmt.Fprintln(w, "ok")
	}))
	srv.Listener = &countingListener{Listener: srv.Listener, n: &accepts}
	srv.EnableHTTP2 = enableH2
	srv.StartTLS()
	return srv, &accepts
}

type countingListener struct {
	net.Listener
	n *int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		atomic.AddInt64(l.n, 1)
	}
	return c, err
}

// warm makes one request so a connection exists (and h2 is negotiated) before
// the concurrent burst, and reports the protocol in use.
func warm(c *http.Client, url string) string {
	resp, err := c.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.Proto
}

func hammer(c *http.Client, url string, n int) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(url)
			if err != nil {
				log.Print(err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	wg.Wait()
}

func main() {
	const n = 20

	// Warm up first in BOTH cases: a burst issued before any connection exists
	// makes the Transport dial several at once, which would hide the effect.
	// After the warm-up the pool holds one usable connection.
	h1, a1 := countingServer(false)
	defer h1.Close()
	warm(h1.Client(), h1.URL)
	base1 := atomic.LoadInt64(a1)
	hammer(h1.Client(), h1.URL, n)
	new1 := atomic.LoadInt64(a1) - base1

	h2, a2 := countingServer(true)
	defer h2.Close()
	proto := warm(h2.Client(), h2.URL)
	base2 := atomic.LoadInt64(a2)
	hammer(h2.Client(), h2.URL, n)
	new2 := atomic.LoadInt64(a2) - base2

	fmt.Printf("%d concurrent requests over HTTP/1.1: %2d new TCP connections\n", n, new1)
	fmt.Printf("%d concurrent requests over %s:  %2d new TCP connections\n", n, proto, new2)
	fmt.Println()
	fmt.Println("HTTP/1.1: one request at a time per connection -> parallelism = connections")
	fmt.Println("HTTP/2:   many streams per connection          -> parallelism is free")
	fmt.Println()
	fmt.Println("Caveat: it is still ONE TCP connection, so a lost packet stalls every")
	fmt.Println("stream on it (transport-level head-of-line blocking). That is the")
	fmt.Println("problem QUIC/HTTP-3 solves by moving streams above UDP.")
}
```

**Output:**

```
20 concurrent requests over HTTP/1.1: 19 new TCP connections
20 concurrent requests over HTTP/2.0:   0 new TCP connections

HTTP/1.1: one request at a time per connection -> parallelism = connections
HTTP/2:   many streams per connection          -> parallelism is free

Caveat: it is still ONE TCP connection, so a lost packet stalls every
stream on it (transport-level head-of-line blocking). That is the
problem QUIC/HTTP-3 solves by moving streams above UDP.
```

> It is still one TCP connection, so a lost packet stalls every stream on it — **transport-level** head-of-line blocking, which is exactly what QUIC/HTTP-3 solves by moving streams above UDP.

---

## 21. Head-of-line blocking, measured

`🔴 hard` · *http/2*

On one HTTP/1.1 connection responses must return **in the order requested**: request 2 waits for response 1. Browsers hide this by opening ~6 connections per host, and Go's Transport does the same — pin it to a single connection and the serialisation is unmistakable.

**Steps:**

1. 5 requests × 100ms of work each, over **one** HTTP/1.1 connection: ~500ms, fully serialised.
2. The same 5 over HTTP/2: ~100ms, all streams in flight at once.
3. A clean ~5× speed-up, which is why HTTP/2 makes fine-grained APIs (many small calls) practical.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// HEAD-OF-LINE BLOCKING, measured.
//
// On one HTTP/1.1 connection, responses must come back in the order the requests
// were sent: request 2 waits for response 1 to finish. Browsers work around it
// by opening ~6 connections per host; Go's Transport does the same. Pin it to a
// single connection and the serialisation becomes obvious.
//
// HTTP/2 multiplexes streams on one connection, so the same work overlaps.
const (
	requests = 5
	workTime = 100 * time.Millisecond
)

func timeIt(c *http.Client, url string) time.Duration {
	// warm up so handshakes are not part of the measurement
	resp, err := c.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(url)
			if err != nil {
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	wg.Wait()
	return time.Since(start)
}

func main() {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(workTime) // each request takes the same time to produce
		fmt.Fprintln(w, "ok")
	})

	// HTTP/1.1, restricted to ONE connection: pure serialisation.
	h1 := httptest.NewServer(h)
	defer h1.Close()
	c1 := h1.Client()
	c1.Transport = &http.Transport{MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
	d1 := timeIt(c1, h1.URL)

	// HTTP/2: one connection, many streams.
	h2 := httptest.NewUnstartedServer(h)
	h2.EnableHTTP2 = true
	h2.StartTLS()
	defer h2.Close()
	d2 := timeIt(h2.Client(), h2.URL)

	round := func(d time.Duration) int64 { return d.Milliseconds() / 50 * 50 }

	fmt.Printf("%d requests, each taking %v:\n\n", requests, workTime)
	fmt.Printf("  HTTP/1.1, 1 connection: ~%3dms  (%d x %v, serialised)\n", round(d1), requests, workTime)
	fmt.Printf("  HTTP/2,   1 connection: ~%3dms  (all %d streams in flight at once)\n", round(d2), requests)
	fmt.Printf("\n  speed-up: %.1fx\n", float64(d1)/float64(d2))
	fmt.Println("\nThis is why HTTP/1.1 clients open several connections per host, and")
	fmt.Println("why HTTP/2 makes fine-grained APIs (many small calls) practical.")
}
```

**Output:**

```
5 requests, each taking 100ms:

  HTTP/1.1, 1 connection: ~500ms  (5 x 100ms, serialised)
  HTTP/2,   1 connection: ~100ms  (all 5 streams in flight at once)

  speed-up: 5.0x

This is why HTTP/1.1 clients open several connections per host, and
why HTTP/2 makes fine-grained APIs (many small calls) practical.
```

> Output note: timings are rounded to 50ms; the exact speed-up varies slightly per run.

---

## 22. TimeoutHandler: a real 503 instead of a dropped connection

`🔴 hard` · *timeouts*

A server-wide `WriteTimeout` is blunt — it kills streaming (example 12) and gives the client a **reset connection** rather than an HTTP response. `http.TimeoutHandler` is the per-route alternative: on overrun it sends a genuine **503** with a body you choose and cancels the request context.

**Steps:**

1. Wrap only the slow route; `/fast` is unaffected.
2. The handler **must cooperate** — it watches `r.Context().Done()` to stop its work. The goroutine is not killed for it.
3. The client receives `503 Service Unavailable` with your message.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"time"
)

// Server-wide WriteTimeout is blunt: it kills streaming (example 12) and gives
// the client a dropped connection rather than an HTTP response.
// http.TimeoutHandler is the per-route alternative: when the handler overruns it
// sends a real 503 with a body you choose, and cancels the request context so
// the handler can stop working.
//
// Its limits, worth knowing: it buffers the response (so it does not work for
// streaming endpoints), and the handler goroutine is NOT killed - it just finds
// its context cancelled. Long work must actually watch ctx.Done().
func slow(w http.ResponseWriter, r *http.Request) {
	select {
	case <-time.After(500 * time.Millisecond):
		fmt.Fprintln(w, "finished the slow work")
	case <-r.Context().Done():
		// The handler must cooperate: this is where you stop a DB query,
		// abandon an upstream call, etc.
		fmt.Println("handler: context cancelled ->", r.Context().Err())
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fast", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "immediate")
	})
	// Only this route gets the tight budget.
	mux.Handle("GET /slow", http.TimeoutHandler(
		http.HandlerFunc(slow), 150*time.Millisecond, "upstream too slow\n"))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{"/fast", "/slow"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			log.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("%-6s -> %s  body: %q\n", path, resp.Status, body)
	}

	time.Sleep(50 * time.Millisecond) // let the cancelled handler log

	fmt.Println("\nNote the client got a proper 503 with a message - not a reset")
	fmt.Println("connection, which is what a server-wide WriteTimeout would give it.")
}
```

**Output:**

```
/fast  -> 200 OK  body: "immediate\n"
handler: context cancelled -> context deadline exceeded
/slow  -> 503 Service Unavailable  body: "upstream too slow\n"

Note the client got a proper 503 with a message - not a reset
connection, which is what a server-wide WriteTimeout would give it.
```

> Its limits: `TimeoutHandler` **buffers** the response, so it does not work on streaming endpoints.

---

## 23. X-Forwarded-For: trust only your own hop

`🔴 hard` · *proxies*

Behind a reverse proxy, `r.RemoteAddr` is the **proxy's** address and the real client sits in `X-Forwarded-For` — a comma-separated list each hop appends to. **The rule:** only entries added by infrastructure *you* control are trustworthy, so count hops **from the right**. Never blindly take the first: that value is attacker-supplied.

**Steps:**

1. An honest request through one proxy: naive and correct parsing agree.
2. A forged request (`X-Forwarded-For: 1.2.3.4, 203.0.113.9`): the naive read swallows `1.2.3.4`; counting one trusted hop from the right still finds the real client.
3. `X-Forwarded-Proto` tells you the **external** scheme — use it for redirects and the `Secure` cookie flag, not `r.TLS`.

```go
package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Behind a reverse proxy, r.RemoteAddr is the PROXY's address. The real client
// is in X-Forwarded-For - a comma-separated list, appended to by each hop:
//
//	X-Forwarded-For: <client>, <proxy1>, <proxy2>
//
// THE RULE: only the entries added by infrastructure YOU control are
// trustworthy. Anything the original client sent is attacker-controlled - it can
// forge any prefix it likes. So you count hops from the RIGHT, skipping the
// proxies you trust, and take the next value. Never blindly take the first.
//
// This matters because that IP drives rate limits, audit logs, geo rules and IP
// allowlists (lesson 57).
func clientIP(r *http.Request, trustedProxies int) string {
	// The immediate peer: always trustworthy, because TCP delivered it.
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	if trustedProxies == 0 {
		return peer // no proxy in front: ignore the header entirely
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}
	parts := strings.Split(xff, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	// The last entry was added by our own edge proxy, the one before it by the
	// next hop out, and so on. Step back over the hops we trust.
	idx := len(parts) - trustedProxies
	if idx < 0 {
		idx = 0
	}
	return parts[idx]
}

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("  X-Forwarded-For:", r.Header.Get("X-Forwarded-For"))
		fmt.Println("  RemoteAddr is the peer (our proxy), not the client")
		fmt.Println("  naive  (first entry):", strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]), "<- SPOOFABLE")
		fmt.Println("  correct (1 trusted hop):", clientIP(r, 1))
		fmt.Println("  X-Forwarded-Proto:", r.Header.Get("X-Forwarded-Proto"), "(use this for redirects/Secure cookies)")
	}))
	defer srv.Close()

	// An honest request through one proxy: the proxy appended the real client.
	fmt.Println("honest request (client 203.0.113.9 via our proxy):")
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// An attacker forging a prefix to fake their IP for a rate limiter.
	fmt.Println("\nforged request (attacker prepends a fake IP):")
	req2, _ := http.NewRequest("GET", srv.URL, nil)
	req2.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")
	req2.Header.Set("X-Forwarded-Proto", "https")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()

	fmt.Println("\nThe naive read swallowed the forged 1.2.3.4; counting one trusted")
	fmt.Println("hop from the right found the real client in both cases.")
}
```

**Output:**

```
honest request (client 203.0.113.9 via our proxy):
  X-Forwarded-For: 203.0.113.9
  RemoteAddr is the peer (our proxy), not the client
  naive  (first entry): 203.0.113.9 <- SPOOFABLE
  correct (1 trusted hop): 203.0.113.9
  X-Forwarded-Proto: https (use this for redirects/Secure cookies)

forged request (attacker prepends a fake IP):
  X-Forwarded-For: 1.2.3.4, 203.0.113.9
  RemoteAddr is the peer (our proxy), not the client
  naive  (first entry): 1.2.3.4 <- SPOOFABLE
  correct (1 trusted hop): 203.0.113.9
  X-Forwarded-Proto: https (use this for redirects/Secure cookies)

The naive read swallowed the forged 1.2.3.4; counting one trusted
hop from the right found the real client in both cases.
```

> This IP drives rate limits, audit logs, geo rules and allowlists — getting it wrong is a real vulnerability ([lesson 57](../57-web-security/)).

---

## 24. A reverse proxy in ten lines

`🔴 hard` · *proxies*

`httputil.ReverseProxy` is a complete HTTP proxy — connection reuse, streaming, header handling — that you configure with a **`Rewrite`** function. `Rewrite` (Go 1.20+) replaces the old `Director`, and the difference matters: it sees both requests and `SetXForwarded()` sets the forwarded headers **correctly**, dropping whatever the client forged.

**Steps:**

1. `pr.SetURL(target)` routes and rewrites; `pr.SetXForwarded()` stamps `X-Forwarded-For/Proto/Host` from the real peer.
2. `ErrorHandler` turns an upstream failure into a clean **502** instead of leaking details.
3. `FlushInterval: -1` flushes immediately — required for SSE through the proxy.
4. `ModifyResponse` lets you touch the response on the way back.

```go
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// A reverse proxy in about ten lines. httputil.ReverseProxy is a full HTTP proxy
// (connection reuse, streaming, header handling) that you configure with a
// Rewrite function.
//
// Rewrite (Go 1.20+) REPLACES the old Director field. It is not just a rename:
// Rewrite gets both the inbound and outbound request, and SetXForwarded() sets
// X-Forwarded-For/Proto/Host correctly - dropping anything the client forged
// (example 23), which Director-based code routinely got wrong.
func main() {
	// Two backends.
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "backend A saw path=%s xff=%q proto=%q\n",
			r.URL.Path, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Forwarded-Proto"))
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "backend B saw path=%s\n", r.URL.Path)
	}))
	defer b.Close()

	urlA, _ := url.Parse(a.URL)
	urlB, _ := url.Parse(b.URL)
	dead, _ := url.Parse("http://127.0.0.1:1") // nothing listening: for the error path

	pick := func(path string) *url.URL {
		switch {
		case strings.HasPrefix(path, "/b/"):
			return urlB
		case strings.HasPrefix(path, "/down/"):
			return dead
		default:
			return urlA
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(pick(pr.In.URL.Path)) // route + rewrite the target
			pr.SetXForwarded()              // set X-Forwarded-* from the REAL peer
			pr.Out.Header.Set("X-Proxy", "demo")
		},
		// Never leak an upstream failure as a 502 with a stack trace.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy: upstream %s failed", r.URL.Host)
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		},
		// Flush streaming responses promptly (-1 = flush immediately; needed for SSE).
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("X-Served-Via", "reverse-proxy")
			return nil
		},
	}

	front := httptest.NewServer(proxy)
	defer front.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	for _, path := range []string{"/hello", "/b/items", "/down/thing"} {
		req, _ := http.NewRequest("GET", front.URL+path, nil)
		req.Header.Set("X-Forwarded-For", "9.9.9.9") // a forged hop from the client
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("%-12s %s via=%q\n             %s", path, resp.Status,
			resp.Header.Get("X-Served-Via"), body)
	}

	fmt.Println("\nNote: SetXForwarded replaced the client's forged 9.9.9.9 with the")
	fmt.Println("real peer address. Use pr.Out.Header.Set(...) after it if you")
	fmt.Println("deliberately want to preserve a trusted upstream chain.")
}
```

**Output:**

```
/hello       200 OK via="reverse-proxy"
             backend A saw path=/hello xff="127.0.0.1" proto="http"
/b/items     200 OK via="reverse-proxy"
             backend B saw path=/b/items
2026/08/05 22:29:07 proxy: upstream 127.0.0.1:1 failed
/down/thing  502 Bad Gateway via=""
             upstream unavailable

Note: SetXForwarded replaced the client's forged 9.9.9.9 with the
real peer address. Use pr.Out.Header.Set(...) after it if you
deliberately want to preserve a trusted upstream chain.
```

> Watch the output: the client's forged `X-Forwarded-For: 9.9.9.9` was **replaced** with the real peer address.

---

## 25. Wrapping ResponseWriter without breaking Flush

`🔴 hard` · *middleware*

Every logging or metrics middleware needs the status code and byte count, and `ResponseWriter` exposes neither — so you wrap it. The catch: the *optional* capabilities (`Flusher`, `Hijacker`, `ReaderFrom`) live in **other interfaces**, and a naive wrapper hides them. Streaming then breaks silently.

**Steps:**

1. The wrapper records status and bytes by overriding `WriteHeader`/`Write`.
2. **`Unwrap() http.ResponseWriter` is the whole fix** — since Go 1.20, `ResponseController` walks the Unwrap chain to reach the real writer.
3. With `Unwrap`: chunked, 3 lines streamed. Without it: `Flush` fails with `feature not supported`, no `Transfer-Encoding`, response buffered.
4. The bug is invisible until someone adds SSE — which is why this pattern is worth internalising.

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
)

// Every logging/metrics middleware needs the status code and byte count - and
// ResponseWriter exposes neither. So you wrap it. The catch: ResponseWriter is
// an interface whose OPTIONAL extras (Flusher, Hijacker, ReaderFrom) live in
// other interfaces. A naive wrapper hides them, and streaming silently breaks.
//
// The modern fix is one method: Unwrap() http.ResponseWriter. Since Go 1.20,
// http.ResponseController walks the Unwrap chain to find the real writer, so
// Flush/Hijack/deadlines keep working through any number of wrappers.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK // the implicit 200 of a bare Write
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap is the whole trick: it exposes the wrapped writer to ResponseController.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// brokenRecorder is the same thing WITHOUT Unwrap - included to show the failure.
type brokenRecorder struct {
	http.ResponseWriter
	status int
}

func logging(next http.Handler, unwrappable bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unwrappable {
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			// A real middleware would also log time.Since(start); omitted here
			// only to keep this example's output byte-for-byte reproducible.
			fmt.Printf("  [log] %s %s -> %d, %d bytes\n",
				r.Method, r.URL.Path, rec.status, rec.bytes)
		} else {
			rec := &brokenRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			fmt.Printf("  [log] %s %s -> %d (no byte count)\n", r.Method, r.URL.Path, rec.status)
		}
	})
}

func streamer(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(w, "chunk %d\n", i)
		if err := rc.Flush(); err != nil {
			fmt.Println("  handler: Flush failed ->", err)
			return
		}
	}
}

func run(label string, unwrappable bool) {
	srv := httptest.NewServer(logging(http.HandlerFunc(streamer), unwrappable))
	defer srv.Close()

	fmt.Println(label)
	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println("  Transfer-Encoding:", resp.TransferEncoding, "([chunked] = streamed, [] = buffered)")
	sc := bufio.NewScanner(resp.Body)
	n := 0
	for sc.Scan() {
		n++
	}
	fmt.Println("  lines received:   ", n)
}

func main() {
	run("with Unwrap() (correct):", true)
	fmt.Println()
	run("without Unwrap() (broken):", false)
	fmt.Println("\nWithout Unwrap, ResponseController cannot reach the real writer, so")
	fmt.Println("Flush fails with ErrNotSupported and the response is buffered instead")
	fmt.Println("of streamed - the bug is invisible until someone adds SSE.")
}
```

**Output:**

```
with Unwrap() (correct):
  [log] GET /stream -> 200, 24 bytes
  Transfer-Encoding: [chunked] ([chunked] = streamed, [] = buffered)
  lines received:    3

without Unwrap() (broken):
  handler: Flush failed -> feature not supported
  [log] GET /stream -> 0 (no byte count)
  Transfer-Encoding: [] ([chunked] = streamed, [] = buffered)
  lines received:    1

Without Unwrap, ResponseController cannot reach the real writer, so
Flush fails with ErrNotSupported and the response is buffered instead
of streamed - the bug is invisible until someone adds SSE.
```

---

## 26. Capstone: a server built properly, and a client that talks to it right

`🔴 hard` · *capstone*

Everything in the tier, assembled. Nothing here is new — it is the collected set of defaults a production Go service should start from.

**Steps:**

1. **Server:** explicit `http.Server` + all five timeouts (ex. 5), own mux, middleware that wraps `ResponseWriter` with `Unwrap` and recovers panics (ex. 25), `MaxBytesReader` on every body (ex. 8), per-route `TimeoutHandler` (ex. 22), a streaming SSE endpoint via `ResponseController` (ex. 12/13), graceful shutdown ([lesson 62](../62-deployment-operations/)).
2. **Client:** one shared `http.Client` with a per-request context (ex. 17), every body drained and closed (ex. 16).
3. Six requests — health, valid JSON, malformed JSON, a stream, a route that overruns its budget, and a 404 — produce a clean access log.
4. **All of it over a single TCP connection**, which is the point: correct client habits make the pool work.

```go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// CAPSTONE: a server built the way a production Go service should be, and a
// client that talks to it correctly. Everything here appeared earlier in the
// tier - this is the assembled version.
//
//	server: explicit http.Server + the five timeouts (ex. 5), own mux (never
//	        DefaultServeMux), middleware that wraps ResponseWriter with Unwrap
//	        (ex. 25), per-route TimeoutHandler (ex. 22), a streaming endpoint
//	        via ResponseController (ex. 12/13), capped request bodies (ex. 8),
//	        graceful shutdown (lesson 62).
//	client: ONE shared client with a per-request context (ex. 17), bodies
//	        drained and closed so connections are reused (ex. 16).
type wrappedWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *wrappedWriter) WriteHeader(c int) {
	if w.status == 0 {
		w.status = c
	}
	w.ResponseWriter.WriteHeader(c)
}

func (w *wrappedWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap keeps Flush/Hijack/deadlines reachable through the wrapper.
func (w *wrappedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type logEntry struct {
	method, path string
	status, size int
}

func middleware(next http.Handler, sink chan<- logEntry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { // never let a panic kill the connection silently
			if v := recover(); v != nil {
				log.Println("panic:", v)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		ww := &wrappedWriter{ResponseWriter: w}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // cap every request body
		next.ServeHTTP(ww, r)
		sink <- logEntry{r.Method, r.URL.Path, ww.status, ww.bytes}
	})
}

func routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"received": in})
	})

	// Streaming: clear the write deadline for this request, flush each event.
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		rc.SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 1; i <= 3; i++ {
			select {
			case <-r.Context().Done(): // client hung up
				return
			default:
			}
			fmt.Fprintf(w, "data: event %d\n\n", i)
			if err := rc.Flush(); err != nil {
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
	})

	// A slow route with its own budget, returning a real 503 rather than a reset.
	mux.Handle("GET /slow", http.TimeoutHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(time.Second):
				fmt.Fprintln(w, "done")
			case <-r.Context().Done():
			}
		}), 100*time.Millisecond, "too slow\n"))

	return mux
}

type countingListener struct {
	net.Listener
	n int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		atomic.AddInt64(&l.n, 1)
	}
	return c, err
}

func main() {
	logs := make(chan logEntry, 64)

	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	ln := &countingListener{Listener: base}

	srv := &http.Server{
		Handler:           middleware(routes(), logs),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	url := "http://" + base.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second} // ONE client, reused

	do := func(method, path, body string) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url+path, rdr)
		if err != nil {
			log.Fatal(err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%-6s %-9s -> error: %v\n", method, path, err)
			return
		}
		defer resp.Body.Close()

		if path == "/events" { // read the stream as it arrives
			sc := bufio.NewScanner(resp.Body)
			n := 0
			for sc.Scan() {
				if sc.Text() != "" {
					n++
				}
			}
			fmt.Printf("%-6s %-9s -> %s  %d events streamed\n", method, path, resp.Status, n)
			return
		}
		b, _ := io.ReadAll(resp.Body) // drain: keeps the connection poolable
		fmt.Printf("%-6s %-9s -> %s  %s\n", method, path, resp.Status, firstLine(b))
	}

	do("GET", "/healthz", "")
	do("POST", "/echo", `{"name":"ada","n":42}`)
	do("POST", "/echo", `{"bogus":`)
	do("GET", "/events", "")
	do("GET", "/slow", "")
	do("GET", "/missing", "")

	fmt.Println("\n--- access log ---")
	close(logs)
	for e := range logs {
		fmt.Printf("  %-4s %-9s %d %4dB\n", e.method, e.path, e.status, e.size)
	}

	fmt.Printf("\nTCP connections used for all of that: %d (one shared client, bodies drained)\n",
		atomic.LoadInt64(&ln.n))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("graceful shutdown: complete")
}

func firstLine(b []byte) string {
	for i, c := range b {
		if c == '\n' {
			return string(b[:i])
		}
	}
	return string(b)
}
```

**Output:**

```
GET    /healthz  -> 200 OK  ok
POST   /echo     -> 200 OK  {"received":{"n":42,"name":"ada"}}
POST   /echo     -> 400 Bad Request  bad json
GET    /events   -> 200 OK  3 events streamed
GET    /slow     -> 503 Service Unavailable  too slow
GET    /missing  -> 404 Not Found  404 page not found

--- access log ---
  GET  /healthz  200    2B
  POST /echo     200   35B
  POST /echo     400    9B
  GET  /events   200   45B
  GET  /slow     503    9B
  GET  /missing  404   19B

TCP connections used for all of that: 1 (one shared client, bodies drained)
graceful shutdown: complete
```

---
