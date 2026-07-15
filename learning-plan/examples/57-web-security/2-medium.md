# Step 57 — Web Application Security · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

Hardening the HTTP surface: **security headers**, **CORS**, **SSRF / open-redirect** defenses, **validation**, **timeouts**, **TLS**, and safe failure.

---

## 9. Security headers middleware

`🟡 medium` · *headers*

A handful of response headers close whole vulnerability classes: **CSP** (the strongest XSS backstop), **HSTS** (force HTTPS), **`nosniff`** (no MIME sniffing), **`X-Frame-Options`** (anti-clickjacking), and a **Referrer-Policy**. Set them once in a middleware that wraps everything.

**Steps:**

1. In a middleware, set the five headers on `w.Header()` before calling `next`.
2. Wrap your whole mux with it.
3. Inspect the headers on the recorder.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func main() {
	h := secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for _, k := range []string{"Content-Security-Policy", "Strict-Transport-Security",
		"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		fmt.Printf("%-27s %s\n", k+":", rec.Header().Get(k))
	}
}
```

**Output:**

```
Content-Security-Policy:    default-src 'self'
Strict-Transport-Security:  max-age=63072000; includeSubDomains
X-Content-Type-Options:     nosniff
X-Frame-Options:            DENY
Referrer-Policy:            no-referrer
```

---

## 10. CORS done right

`🟡 medium` · *cors*

CORS is an **allowlist**. Reflect the request's `Origin` **only if it's on your list** (and add `Vary: Origin` so caches don't cross the streams). Never echo an arbitrary origin, and never combine `Access-Control-Allow-Origin: *` with credentials — the browser forbids it and it's dangerous.

**Steps:**

1. Look up `r.Header.Get("Origin")` in your allowlist.
2. Only on a match, set `Access-Control-Allow-Origin` (to that exact origin) + `Vary: Origin`.
3. An unlisted origin gets no CORS header → the browser blocks the cross-origin read.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func cors(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Reflect the origin ONLY if it's on the allowlist. Never echo an arbitrary
		// origin, and never combine "*" with credentials.
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	allow := map[string]bool{"https://app.example.com": true}
	h := cors(allow, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	check := func(origin string) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		fmt.Printf("%-28s -> ACAO=%q\n", origin, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	check("https://app.example.com")
	check("https://evil.example.com")
}
```

**Output:**

```
https://app.example.com      -> ACAO="https://app.example.com"
https://evil.example.com     -> ACAO=""
```

---

## 11. SSRF: validate outbound URLs

`🟡 medium` · *ssrf*

If your server fetches a **user-supplied URL**, an attacker can point it at internal services — the cloud metadata endpoint `169.254.169.254`, `localhost`, private ranges. Validate: require `http`/`https`, and reject IP-literal hosts in loopback/private/link-local ranges. This is necessary but **not sufficient** — a hostname can resolve to an internal IP (DNS rebinding), so also re-check the resolved address at dial time.

**Steps:**

1. Parse the URL; require an `http`/`https` scheme.
2. Reject `localhost` and IP literals that are loopback/private/link-local.
3. Note: also guard at dial time with `net.Dialer.Control` (DNS can resolve to an internal IP).

```go
package main

import (
	"fmt"
	"net"
	"net/url"
)

// validateURL is the URL-layer SSRF check: require http/https and reject IP-literal
// hosts in private/loopback/link-local ranges (e.g. the cloud metadata endpoint
// 169.254.169.254). NOT sufficient alone — a hostname can resolve to an internal IP
// (DNS rebinding), so also guard at dial time with a net.Dialer.Control that
// re-checks the resolved address.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "localhost" {
		return fmt.Errorf("host not allowed: localhost")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("host not allowed: %s", ip)
		}
	}
	return nil
}

func main() {
	for _, u := range []string{
		"https://api.example.com/data",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/admin",
		"http://localhost:6379/",
		"file:///etc/passwd",
	} {
		fmt.Printf("%-42s -> %v\n", u, validateURL(u))
	}
}
```

**Output:**

```
https://api.example.com/data               -> <nil>
http://169.254.169.254/latest/meta-data/   -> host not allowed: 169.254.169.254
http://10.0.0.5/admin                      -> host not allowed: 10.0.0.5
http://localhost:6379/                     -> host not allowed: localhost
file:///etc/passwd                         -> scheme "file" not allowed
```

---

## 12. Open redirect defense

`🟡 medium` · *redirect*

A `?next=` parameter that you redirect to blindly lets an attacker send `?next=https://evil.com` and bounce your users to a phishing page under your domain's trust. Only allow **local paths**: relative, starting with a single `/`, with no scheme and no host.

**Steps:**

1. Parse the target; reject if it's absolute or has a host.
2. Reject `//evil.com` (a protocol-relative URL that browsers treat as absolute).
3. Local paths pass; everything else falls back to `/`.

```go
package main

import (
	"fmt"
	"net/url"
	"strings"
)

// safeRedirect allows only LOCAL paths (no scheme, no host), so an attacker can't
// pass "?next=https://evil.com" to phish users off your domain (open redirect).
func safeRedirect(next string) (string, bool) {
	u, err := url.Parse(next)
	if err != nil {
		return "/", false
	}
	if u.IsAbs() || u.Host != "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/", false
	}
	return next, true
}

func main() {
	for _, n := range []string{"/dashboard", "https://evil.com", "//evil.com", "/settings?tab=1"} {
		dest, ok := safeRedirect(n)
		fmt.Printf("%-20q -> ok=%-5v dest=%s\n", n, ok, dest)
	}
}
```

**Output:**

```
"/dashboard"         -> ok=true  dest=/dashboard
"https://evil.com"   -> ok=false dest=/
"//evil.com"         -> ok=false dest=/
"/settings?tab=1"    -> ok=true  dest=/settings?tab=1
```

---

## 13. Allowlist input validation

`🟡 medium` · *validation*

Validate with an **allowlist**, not a blocklist: the value must match a strict pattern or be in a known set. Blocklisting "bad" characters always misses an encoding; allowlisting denies by default.

**Steps:**

1. A username must match `^[a-z0-9_]{3,16}$` — anything else is rejected.
2. A role must be one of a known set (`map[string]bool`).
3. Injection attempts and unknown roles fail by construction.

```go
package main

import (
	"fmt"
	"regexp"
)

var usernameRE = regexp.MustCompile(`^[a-z0-9_]{3,16}$`)

// Validation is an ALLOWLIST (deny by default): the value must match a strict
// pattern or be in a known set. Don't try to blocklist "bad" characters.
func validUsername(s string) bool { return usernameRE.MatchString(s) }

var roles = map[string]bool{"user": true, "admin": true, "auditor": true}

func validRole(s string) bool { return roles[s] }

func main() {
	for _, u := range []string{"alice_01", "ab", "Robert'); DROP TABLE--", "good_name"} {
		fmt.Printf("username %-26q -> %v\n", u, validUsername(u))
	}
	for _, r := range []string{"admin", "root"} {
		fmt.Printf("role %-8q -> %v\n", r, validRole(r))
	}
}
```

**Output:**

```
username "alice_01"                 -> true
username "ab"                       -> false
username "Robert'); DROP TABLE--"   -> false
username "good_name"                -> true
role "admin"  -> true
role "root"   -> false
```

---

## 14. Server timeouts

`🟡 medium` · *dos*

The **zero-value `http.Server` has no timeouts** — a slow client can open a connection and dribble bytes forever (Slowloris), tying up resources. Always set `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` and cap header size.

**Steps:**

1. Build an `http.Server` with explicit timeouts.
2. `ReadHeaderTimeout` bounds the slowest attack surface (header reads).
3. `MaxHeaderBytes` caps header memory.

```go
package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	// The zero-value http.Server has NO timeouts — a slow client can hold a connection
	// open forever (Slowloris). Always set them explicitly.
	srv := &http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 5 * time.Second,  // time to read request headers
		ReadTimeout:       10 * time.Second, // time to read the whole request
		WriteTimeout:      10 * time.Second, // time to write the response
		IdleTimeout:       60 * time.Second, // keep-alive idle limit
		MaxHeaderBytes:    1 << 20,          // 1 MiB header cap
	}
	fmt.Println("ReadHeaderTimeout:", srv.ReadHeaderTimeout)
	fmt.Println("ReadTimeout:      ", srv.ReadTimeout)
	fmt.Println("WriteTimeout:     ", srv.WriteTimeout)
	fmt.Println("IdleTimeout:      ", srv.IdleTimeout)
	fmt.Println("MaxHeaderBytes:   ", srv.MaxHeaderBytes)
}
```

**Output:**

```
ReadHeaderTimeout: 5s
ReadTimeout:       10s
WriteTimeout:      10s
IdleTimeout:       1m0s
MaxHeaderBytes:    1048576
```

---

## 15. Harden TLS

`🟡 medium` · *tls*

Set a **minimum TLS version** so clients can't negotiate down to a broken protocol. Go already selects safe cipher suites; for TLS 1.2 you may pin an AEAD-only list. TLS 1.3 cipher suites aren't configurable (they're all strong).

**Steps:**

1. `tls.Config{MinVersion: tls.VersionTLS12}` (prefer 1.3 where you can).
2. Optionally pin TLS-1.2 AEAD cipher suites.
3. Attach the config to your `http.Server.TLSConfig`.

```go
package main

import (
	"crypto/tls"
	"fmt"
)

func main() {
	// Harden TLS: require at least 1.2 (prefer 1.3). Go picks safe ciphers by default;
	// for TLS 1.2 you may pin an AEAD-only list. TLS 1.3 cipher suites aren't configurable.
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}
	fmt.Println("min version is TLS 1.2:", cfg.MinVersion == tls.VersionTLS12)
	fmt.Println("pinned cipher suites:  ", len(cfg.CipherSuites))
	for _, cs := range cfg.CipherSuites {
		fmt.Println("  -", tls.CipherSuiteName(cs))
	}
}
```

**Output:**

```
min version is TLS 1.2: true
pinned cipher suites:   2
  - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
  - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
```

---

## 16. Don't leak internal errors

`🟡 medium` · *info leak*

An internal error often contains a DSN, an internal IP, or a SQL fragment. Never send `err.Error()` to the client — **log the detail server-side** and return a **generic** message + the right status. The attacker gets nothing; you keep the diagnostics.

**Steps:**

1. On failure, log the real error (server-side only).
2. `http.Error(w, "internal server error", 500)` — generic body.
3. Confirm the client response doesn't contain the internal detail.

```go
package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

var errInternal = errors.New("pq: connection refused to 10.0.3.4:5432") // internal detail

func handler(w http.ResponseWriter, r *http.Request) {
	err := errInternal // pretend a DB call failed
	if err != nil {
		// Log the DETAIL server-side (log.Printf("query failed: %v", err)), but return
		// a GENERIC message + 500 — never leak internals (stack traces, DSNs, SQL).
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "ok")
}

func main() {
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest("GET", "/", nil))
	fmt.Println("status:", rec.Code)
	fmt.Printf("client sees: %q\n", rec.Body.String())
	fmt.Println("leaks internal detail:", strings.Contains(rec.Body.String(), "10.0.3.4"))
}
```

**Output:**

```
status: 500
client sees: "internal server error\n"
leaks internal detail: false
```

---

## 17. Redact secrets in logs

`🟡 medium` · *secrets*

Logs get shipped, indexed, and shared — so secrets must never reach them. Implement **`slog.LogValuer`** on your secret types to log a redacted placeholder. Even if someone logs the whole struct, the token prints as `REDACTED`.

**Steps:**

1. Give the secret type a `LogValue()` returning `slog.StringValue("REDACTED")`.
2. Log a value of that type as a normal attribute.
3. (The example strips the timestamp so the output is stable.)

```go
package main

import (
	"log/slog"
	"os"
)

type Token string

// LogValue makes slog print a redacted form — the real token never reaches the logs.
func (t Token) LogValue() slog.Value { return slog.StringValue("REDACTED") }

func main() {
	// Strip the timestamp so the output is stable.
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	log := slog.New(h)

	secret := Token("super-secret-token")
	log.Info("login", "user", "alice", "token", secret)
}
```

**Output:**

```
level=INFO msg=login user=alice token=REDACTED
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
