# Step 57 — Web Application Security · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

Object-level authorization, the defenses Go gives you **for free**, webhook verification, rate limiting, upload validation, and a hardened endpoint that combines them.

---

## 18. IDOR: object-level authorization

`🔴 hard` · *authz*

**IDOR** (insecure direct object reference / broken object-level authorization) is one of the most common real-world bugs: an authenticated user changes an `id` in the URL and reads someone else's data. Authenticated ≠ authorized — **scope every object fetch by the caller's ownership**, so a not-owned resource is indistinguishable from "not found".

**Steps:**

1. The insecure fetch looks up by `id` only — any user can read any document.
2. The secure fetch requires `d.OwnerID == callerID`, else returns "not found".
3. A mismatched owner should surface as 404, not 403 (don't reveal existence).

```go
package main

import "fmt"

type Doc struct {
	ID      int
	OwnerID int
	Title   string
}

var docs = map[int]Doc{
	1: {1, 100, "Alice's notes"},
	2: {2, 200, "Bob's notes"},
}

// getDocInsecure fetches by id only — any logged-in user can read ANY document by
// changing the id in the URL (IDOR / broken object-level authorization).
func getDocInsecure(id int) (Doc, bool) {
	d, ok := docs[id]
	return d, ok
}

// getDocSecure scopes the fetch by the caller's ownership — the authz check is part
// of the lookup, so a mismatched owner is indistinguishable from "not found".
func getDocSecure(id, callerID int) (Doc, bool) {
	d, ok := docs[id]
	if !ok || d.OwnerID != callerID {
		return Doc{}, false
	}
	return d, ok
}

func main() {
	// Alice (id 100) tries to read doc 2, which belongs to Bob (id 200).
	d, ok := getDocInsecure(2)
	fmt.Printf("insecure: ok=%v title=%q  <- leaked!\n", ok, d.Title)

	d, ok = getDocSecure(2, 100)
	fmt.Printf("secure:   ok=%v title=%q  <- 404\n", ok, d.Title)

	d, _ = getDocSecure(1, 100)
	fmt.Printf("own doc:  title=%q\n", d.Title)
}
```

**Output:**

```
insecure: ok=true title="Bob's notes"  <- leaked!
secure:   ok=false title=""  <- 404
own doc:  title="Alice's notes"
```

---

## 19. ReDoS and Go's RE2 engine

`🔴 hard` · *redos*

**ReDoS** (regular-expression denial of service) happens when a pattern like `^(a+)+$` triggers **catastrophic backtracking** — a short input hangs the engine for seconds. Go's `regexp` uses **RE2**, which runs in guaranteed **linear time** and never backtracks, so this whole class is off the table. Still cap input length and, for user-supplied patterns, add a size limit + timeout.

**Steps:**

1. Compile the classic evil pattern and match a crafted input — it returns instantly.
2. In PCRE/JS/Java this would hang; RE2 doesn't.
3. Defense-in-depth: bound input length regardless.

```go
package main

import (
	"fmt"
	"regexp"
)

func main() {
	// This pattern causes CATASTROPHIC BACKTRACKING in PCRE/JS/Java engines — a short
	// crafted input can hang them for seconds (ReDoS). Go's regexp uses RE2, which runs
	// in LINEAR time and never backtracks, so it stays fast.
	re := regexp.MustCompile(`^(a+)+$`)
	evil := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa!" // 29 a's then a non-match
	fmt.Println("matched:", re.MatchString(evil), "(returned instantly — no ReDoS)")

	// Still: cap input length, and for USER-SUPPLIED patterns compile with a size
	// limit and run under a timeout/goroutine you can abandon.
	fmt.Println("input length guard:", len(evil) <= 1000)
}
```

**Output:**

```
matched: false (returned instantly — no ReDoS)
input length guard: true
```

---

## 20. Stop MIME sniffing on downloads

`🔴 hard` · *headers*

If you serve a user-uploaded file without saying what it is, a browser may **sniff** the bytes and decide an "image" is actually HTML/JS — and execute it in your origin. Send **`X-Content-Type-Options: nosniff`**, an explicit `Content-Type`, and an **attachment** disposition so it downloads instead of rendering.

**Steps:**

1. Set `X-Content-Type-Options: nosniff`.
2. Set `Content-Type: application/octet-stream` (or the real, validated type).
3. Add `Content-Disposition: attachment` so it's downloaded, not rendered.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

// serveDownload sends nosniff + an attachment disposition so the browser won't
// MIME-sniff an uploaded file and run it as HTML/JS.
func serveDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="upload.bin"`)
	w.Write([]byte("<html><script>alert(1)</script></html>"))
}

func main() {
	rec := httptest.NewRecorder()
	serveDownload(rec, httptest.NewRequest("GET", "/files/1", nil))
	for _, k := range []string{"X-Content-Type-Options", "Content-Type", "Content-Disposition"} {
		fmt.Printf("%-24s %s\n", k+":", rec.Header().Get(k))
	}
}
```

**Output:**

```
X-Content-Type-Options:  nosniff
Content-Type:            application/octet-stream
Content-Disposition:     attachment; filename="upload.bin"
```

---

## 21. HTTP header injection is blocked

`🔴 hard` · *free defenses*

Header injection (a.k.a. response splitting) tries to smuggle a **CRLF** into a header value to inject a second header or split the response. Go's `net/http` **sanitizes header values on write**, so the injected `X-Injected` header never reaches the client — one whole class handled for you.

**Steps:**

1. A handler sets a header to a value containing `\r\nX-Injected: yes`.
2. Make a real request through an `httptest` server.
3. The client sees a sanitized `X-Echo` and **no** `X-Injected` header.

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

func main() {
	// An attacker-controlled value with CRLF tries to inject a second header
	// ("X-Injected"). Go's net/http SANITIZES header values on write, so the
	// injection never reaches the client.
	handler := func(w http.ResponseWriter, r *http.Request) {
		evil := "safe\r\nX-Injected: yes"
		w.Header().Set("X-Echo", evil)
		fmt.Fprintln(w, "ok")
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	fmt.Printf("X-Echo:     %q\n", resp.Header.Get("X-Echo"))
	fmt.Printf("X-Injected: %q (injection blocked)\n", resp.Header.Get("X-Injected"))
}
```

**Output:**

```
X-Echo:     "safe  X-Injected: yes"
X-Injected: "" (injection blocked)
```

---

## 22. Verify a webhook signature

`🔴 hard` · *integrity*

Inbound webhooks (Stripe, GitHub, …) are signed with **HMAC-SHA256** over the raw payload. Recompute the signature with your shared secret and compare with **`hmac.Equal`** (constant-time). A forged signature or a tampered payload won't match — so you only act on authentic events.

**Steps:**

1. `sign(secret, payload)` = hex HMAC-SHA256.
2. Verify with `hmac.Equal` (never `==`).
3. A wrong signature and a tampered payload both fail.

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Inbound webhooks (Stripe, GitHub, ...) are signed with HMAC-SHA256. Verify the
// signature with a constant-time compare before trusting the payload.
func sign(secret, payload []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func verifyWebhook(secret, payload []byte, signature string) bool {
	return hmac.Equal([]byte(sign(secret, payload)), []byte(signature))
}

func main() {
	secret := []byte("whsec_test")
	payload := []byte(`{"event":"payment.succeeded","id":"evt_1"}`)

	good := sign(secret, payload) // the provider sends this in a header
	fmt.Println("valid signature:  ", verifyWebhook(secret, payload, good))
	fmt.Println("forged signature: ", verifyWebhook(secret, payload, "deadbeef"))

	// A tampered payload no longer matches the signature.
	tampered := []byte(`{"event":"payment.succeeded","id":"evt_999"}`)
	fmt.Println("tampered payload: ", verifyWebhook(secret, tampered, good))
}
```

**Output:**

```
valid signature:   true
forged signature:  false
tampered payload:  false
```

---

## 23. Token-bucket rate limiting

`🔴 hard` · *abuse*

Rate limiting caps how fast a client can hit you — throttling brute force, scraping, and abuse. A **token bucket** allows a burst up to its capacity, then only as fast as tokens refill. (Deterministic here via manual refill; in production tokens drip on a `time.Ticker`, keyed per client/IP.)

**Steps:**

1. `take()` consumes a token if one's available, else denies.
2. Start with a burst of 3; the 4th and 5th requests are denied.
3. `refill(2)` adds two tokens back, allowing two more.

```go
package main

import "fmt"

// A fixed-capacity token bucket for per-client rate limiting. take() consumes a
// token if available; refill() adds tokens (in production, on a time.Ticker).
// Deterministic here via manual refill.
type Bucket struct {
	tokens, cap int
}

func (b *Bucket) take() bool {
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}
func (b *Bucket) refill(n int) { b.tokens = min(b.cap, b.tokens+n) }

func main() {
	b := &Bucket{tokens: 3, cap: 3} // burst of 3
	for i := 1; i <= 5; i++ {
		fmt.Printf("request %d: allowed=%v\n", i, b.take())
	}
	b.refill(2) // two tokens drip back
	fmt.Println("after refill(2):")
	for i := 1; i <= 3; i++ {
		fmt.Printf("request %d: allowed=%v\n", i, b.take())
	}
}
```

**Output:**

```
request 1: allowed=true
request 2: allowed=true
request 3: allowed=true
request 4: allowed=false
request 5: allowed=false
after refill(2):
request 1: allowed=true
request 2: allowed=true
request 3: allowed=false
```

---

## 24. Validate a file upload

`🔴 hard` · *uploads*

Never trust a client's filename or `Content-Type`. Cap the body size, read the first bytes, and determine the **real** type with `http.DetectContentType` (content sniffing done *by you*, deliberately). Reject anything not on your allowlist — a `.png` that's really HTML is rejected.

**Steps:**

1. `http.MaxBytesReader` caps the upload; `r.FormFile` reads the part.
2. Sniff the first 512 bytes with `http.DetectContentType`.
3. Accept only allowlisted types; a disguised HTML file is `415`.

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
)

var allowed = map[string]bool{"image/png": true, "image/jpeg": true}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // cap at 1 MiB
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "bad upload", http.StatusBadRequest)
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := file.Read(head)
	// Trust the CONTENT, not the client-provided filename/Content-Type.
	ctype := http.DetectContentType(head[:n])
	if !allowed[ctype] {
		http.Error(w, "type not allowed: "+ctype, http.StatusUnsupportedMediaType)
		return
	}
	fmt.Fprintf(w, "accepted %s", ctype)
}

func upload(srv *httptest.Server, content []byte) (int, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "upload")
	fw.Write(content)
	mw.Close()
	resp, err := http.Post(srv.URL, mw.FormDataContentType(), &buf)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func main() {
	srv := httptest.NewServer(http.HandlerFunc(uploadHandler))
	defer srv.Close()

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...) // PNG magic bytes
	code, msg := upload(srv, png)
	fmt.Printf("png:  %d %s\n", code, msg)

	html := []byte("<html><script>alert(1)</script></html>")
	code, msg = upload(srv, html)
	fmt.Printf("html: %d %s\n", code, msg)
}
```

**Output:**

```
png:  200 accepted image/png
html: 415 type not allowed: text/html; charset=utf-8
```

---

## 25. Password strength policy

`🔴 hard` · *passwords*

Enforce a minimum strength: length plus character-class variety. (The strongest addition is a **breach-list check** — HaveIBeenPwned's k-anonymity API needs only the first 5 chars of the SHA-1 hash — but that's a network call, so it's described here, not run.)

**Steps:**

1. Require length ≥ 12.
2. Require lower + upper + digit + symbol.
3. Return the specific issues so the UI can explain them.

```go
package main

import (
	"fmt"
	"unicode"
)

// A minimum-strength policy: length + character-class variety. (Also check against a
// breach list — e.g. HaveIBeenPwned's k-anonymity API, which needs only the first 5
// chars of the SHA-1 hash — but that requires a network call, so it's omitted here.)
func passwordIssues(pw string) []string {
	var issues []string
	if len(pw) < 12 {
		issues = append(issues, "too short (min 12)")
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	if !hasLower || !hasUpper || !hasDigit || !hasSymbol {
		issues = append(issues, "need lower, upper, digit, and symbol")
	}
	return issues
}

func main() {
	for _, pw := range []string{"password", "Str0ng!Passphrase"} {
		if issues := passwordIssues(pw); len(issues) == 0 {
			fmt.Printf("%-20q OK\n", pw)
		} else {
			fmt.Printf("%-20q %v\n", pw, issues)
		}
	}
}
```

**Output:**

```
"password"           [too short (min 12) need lower, upper, digit, and symbol]
"Str0ng!Passphrase"  OK
```

---

## 26. Capstone: a hardened endpoint

`🔴 hard` · *capstone*

One endpoint that layers the defenses from this lesson: **security headers** + **body-size cap** (a `secure` middleware), **object-level authorization** (404 for not-owned), **strict JSON decoding** (unknown fields rejected), **allowlist validation**, and **generic error messages**. Driven end-to-end with `httptest`, showing each attack blocked and the valid request passing.

**Steps:**

1. `secure` wraps the handler with headers + `MaxBytesReader`.
2. The handler checks ownership first (404), then decodes strictly (400), then validates (422).
3. Four requests exercise: success, IDOR→404, bad input→422, overposting→400.

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
)

var nameRE = regexp.MustCompile(`^[a-zA-Z ]{1,40}$`)

type UpdateReq struct {
	Name string `json:"name"`
}

var owners = map[int]int{1: 100, 2: 200} // docID -> ownerID

// secure applies cross-cutting hardening: security headers + a request-body cap.
func secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		next(w, r)
	}
}

func updateDoc(w http.ResponseWriter, r *http.Request) {
	callerID := 100 // pretend the auth middleware set this
	var id int
	fmt.Sscanf(r.PathValue("id"), "%d", &id)

	// Object-level authz: unknown or not-owned -> 404 (don't reveal existence).
	if owners[id] != callerID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req UpdateReq
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !nameRE.MatchString(req.Name) {
		http.Error(w, "invalid name", http.StatusUnprocessableEntity)
		return
	}
	fmt.Fprintf(w, "updated doc %d name=%q", id, req.Name)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /docs/{id}", secure(updateDoc))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	do := func(id, body string) {
		req, _ := http.NewRequest("PATCH", srv.URL+"/docs/"+id, strings.NewReader(body))
		resp, _ := http.DefaultClient.Do(req)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("PATCH /docs/%s %-32s -> %d %s\n", id, body, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	do("1", `{"name":"My Notes"}`)           // ok (owned by 100)
	do("2", `{"name":"Hack"}`)               // 404 (owned by 200)
	do("1", `{"name":"Bad<script>"}`)        // 422 invalid
	do("1", `{"name":"ok","is_admin":true}`) // 400 unknown field
}
```

**Output:**

```
PATCH /docs/1 {"name":"My Notes"}              -> 200 updated doc 1 name="My Notes"
PATCH /docs/2 {"name":"Hack"}                  -> 404 not found
PATCH /docs/1 {"name":"Bad<script>"}           -> 422 invalid name
PATCH /docs/1 {"name":"ok","is_admin":true}    -> 400 bad request
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
