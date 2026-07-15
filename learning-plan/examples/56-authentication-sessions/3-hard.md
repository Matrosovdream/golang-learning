# Step 56 — Authentication & Sessions · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

The real flows: **CSRF**, the **OAuth2 code flow + PKCE**, **OIDC** ID tokens, **TOTP** 2FA, **lockout**, **password reset**, and a full **auth service** — ending in an end-to-end httptest capstone.

---

## 18. CSRF: double-submit cookie

`🔴 hard` · *csrf*

Cookies ride along on cross-site requests, so a state-changing POST needs proof it came from *your* page. **Double-submit**: put a random token in a JS-readable cookie and require the client to echo it in a header. A cross-site attacker can't **read** the cookie (same-origin policy), so it can't set the matching header.

**Steps:**

1. The token lives in a cookie set at page load; JS copies it into `X-CSRF-Token`.
2. The server compares cookie vs header in constant time.
3. A forged request has no header (attacker can't read the cookie) → rejected.

```go
package main

import (
	"crypto/subtle"
	"fmt"
)

// Double-submit cookie: the server sends a random CSRF token in a (JS-readable)
// cookie; the client echoes it in a header. A cross-site attacker can't READ the
// cookie (same-origin policy), so it can't set the matching header. Constant-time compare.
func csrfOK(cookieToken, headerToken string) bool {
	if cookieToken == "" || headerToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}

func main() {
	token := "random-csrf-token" // set in a cookie at page load
	// Legit POST from our own page echoes the token in X-CSRF-Token.
	fmt.Println("legit POST: ", csrfOK(token, "random-csrf-token"))
	// Forged cross-site POST: the browser sends the cookie automatically, but the
	// attacker can't read it to set the header -> blank -> rejected.
	fmt.Println("forged POST:", csrfOK(token, ""))
}
```

**Output:**

```
legit POST:  true
forged POST: false
```

---

## 19. CSRF: synchronizer token

`🔴 hard` · *csrf*

The other CSRF pattern: the server keeps a per-session token (the **synchronizer** token) and embeds it in every form; each state-changing POST must present the matching one. Unlike double-submit, the source of truth is server-side — a cookie leak alone doesn't hand over the token.

**Steps:**

1. Store a CSRF token per session; render `tokenFor(session)` into the form.
2. On POST, `check(session, presented)` compares against the stored token (constant-time).
3. Wrong token — or the token from a different session — fails.

```go
package main

import (
	"crypto/subtle"
	"fmt"
)

// Synchronizer-token pattern: the server keeps a per-session CSRF token and embeds
// it in forms; each state-changing POST must present the matching token. Unlike
// double-submit, the source of truth is server-side, not a cookie.
type CSRF struct {
	perSession map[string]string // sessionID -> csrf token
}

func (c *CSRF) tokenFor(session string) string { return c.perSession[session] }
func (c *CSRF) check(session, presented string) bool {
	want := c.perSession[session]
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(presented)) == 1
}

func main() {
	c := &CSRF{perSession: map[string]string{"sess-1": "csrf-xyz"}}
	tok := c.tokenFor("sess-1") // rendered into the form
	fmt.Println("valid submit: ", c.check("sess-1", tok))
	fmt.Println("wrong token:  ", c.check("sess-1", "guess"))
	fmt.Println("wrong session:", c.check("sess-2", tok))
}
```

**Output:**

```
valid submit:  true
wrong token:   false
wrong session: false
```

---

## 20. OAuth2 authorization code flow

`🔴 hard` · *oauth2*

"Log in with Google/GitHub" is the OAuth2 **authorization-code** flow: redirect the user to the provider with a random `state`, receive a `code` on the callback, then exchange the code for a token server-to-server. The `state` parameter is the **CSRF defense for the redirect** — verify it *before* exchanging the code. Here `httptest` plays the provider.

**Steps:**

1. Send the user to `/authorize` with a random `state`; the provider redirects back with `code` + the same `state`.
2. On the callback, verify `state` matches what you sent.
3. Only then POST the `code` to `/token` and read the access token.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
)

func main() {
	// A stand-in OAuth2 provider: /token swaps a valid authorization code for a token.
	codes := map[string]string{"auth-code-123": "user-42"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			r.ParseForm()
			user, ok := codes[r.Form.Get("code")]
			if !ok {
				http.Error(w, "invalid_grant", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-" + user})
		}
	}))
	defer provider.Close()

	// The client sent the user to /authorize with a random `state` (CSRF defense for
	// the redirect). The provider redirects back with code + the same state.
	state := "xyz-state"
	redirectBack := "/callback?code=auth-code-123&state=" + state

	// Callback: FIRST verify state matches what we sent, THEN exchange the code.
	cb, _ := url.Parse(redirectBack)
	q := cb.Query()
	if q.Get("state") != state {
		fmt.Println("state mismatch - abort (possible CSRF)")
		return
	}
	resp, err := http.PostForm(provider.URL+"/token", url.Values{"code": {q.Get("code")}})
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var tok map[string]string
	json.NewDecoder(resp.Body).Decode(&tok)
	fmt.Println("state verified, access_token:", tok["access_token"])
}
```

**Output:**

```
state verified, access_token: tok-user-42
```

---

## 21. PKCE for public clients

`🔴 hard` · *oauth2*

**PKCE** (RFC 7636) secures the code flow for clients that can't keep a secret — SPAs, mobile apps, CLIs. The client picks a random `verifier`, sends `code_challenge = base64url(sha256(verifier))` up front, and proves it with the raw `verifier` at token exchange. An intercepted auth code is then useless without the verifier.

**Steps:**

1. `challenge(verifier) = base64url(sha256(verifier))` with method `S256`.
2. Send the challenge with the authorization request.
3. At exchange, the server recomputes the challenge from the presented verifier and compares.

```go
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCE (RFC 7636) protects public clients (SPAs, mobile) that can't keep a secret.
// The client sends challenge = BASE64URL(SHA256(verifier)) up front, then proves it
// with the raw verifier at token exchange. An intercepted auth code is useless
// without the verifier.
func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func main() {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" // random, kept by the client
	c := challenge(verifier)
	fmt.Println("code_challenge:", c)
	fmt.Println("method: S256")

	// At token exchange the server recomputes the challenge from the presented verifier.
	fmt.Println("verifier matches:", challenge(verifier) == c)
	fmt.Println("wrong verifier:  ", challenge("attacker-guess") == c)
}
```

**Output:**

```
code_challenge: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
method: S256
verifier matches: true
wrong verifier:   false
```

---

## 22. Validate an OIDC ID token

`🔴 hard` · *oidc*

OIDC layers identity on top of OAuth2 via an **ID token** — a JWT describing the user. Validating it means: verify the signature, then check the standard claims — `iss` (who issued it), `aud` (it's for *your* client), and `exp`. Skipping `aud` lets a token minted for another app be replayed at yours.

**Steps:**

1. Verify the HMAC signature (real OIDC uses RS256 against the provider's JWKS).
2. Check `iss` == expected issuer and `aud` == your client id.
3. Check `now < exp`; extract `sub`/`email`.

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type IDToken struct {
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// validateID verifies the signature, then the standard OIDC claims: issuer,
// audience (your client id), and expiry. (Real OIDC verifies an RS256 signature
// against the provider's JWKS; HMAC is used here to stay self-contained.)
func validateID(token string, secret []byte, wantIss, wantAud string, now int64) (IDToken, error) {
	var t IDToken
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return t, fmt.Errorf("malformed")
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal([]byte(b64(h.Sum(nil))), []byte(parts[2])) {
		return t, fmt.Errorf("bad signature")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(raw, &t)
	switch {
	case t.Iss != wantIss:
		return t, fmt.Errorf("wrong issuer")
	case t.Aud != wantAud:
		return t, fmt.Errorf("wrong audience")
	case now >= t.Exp:
		return t, fmt.Errorf("expired")
	}
	return t, nil
}

func main() {
	secret := []byte("provider-key")
	hdr := b64([]byte(`{"alg":"HS256"}`))
	claims := b64([]byte(`{"iss":"https://issuer","aud":"my-client","sub":"u1","email":"a@x.com","exp":2000}`))
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(hdr + "." + claims))
	tok := hdr + "." + claims + "." + b64(h.Sum(nil))

	t, err := validateID(tok, secret, "https://issuer", "my-client", 1500)
	fmt.Println("valid:", err == nil, "sub:", t.Sub, "email:", t.Email)

	_, err = validateID(tok, secret, "https://issuer", "other-client", 1500)
	fmt.Println("wrong aud:", err)
}
```

**Output:**

```
valid: true sub: u1 email: a@x.com
wrong aud: wrong audience
```

---

## 23. TOTP two-factor codes

`🔴 hard` · *2fa*

The 6-digit code in an authenticator app is **TOTP** (RFC 6238): `HOTP(secret, floor(unixtime/30))`. HOTP is an HMAC-SHA1 over the counter, reduced to 6 digits by **dynamic truncation** (RFC 4226). Using a fixed counter here makes the output deterministic — the value `287082` is the RFC's counter-1 test vector.

**Steps:**

1. HMAC-SHA1 the 8-byte big-endian counter with the shared secret.
2. Dynamic-truncate: the low nibble of the last byte picks a 4-byte offset; mask the top bit.
3. `code % 10^digits`; verify against the current counter (± a window for skew).

```go
package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
)

// TOTP (RFC 6238) = HOTP over a time-based counter — what authenticator apps show.
// Deterministic here via a FIXED counter instead of time.Now()/30.
func hotp(secret []byte, counter uint64, digits int) int {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	h := hmac.New(sha1.New, secret)
	h.Write(buf[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f // dynamic truncation (RFC 4226)
	code := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := 1
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return int(code) % mod
}

func main() {
	secret := []byte("12345678901234567890") // shared secret (normally base32-encoded)
	counter := uint64(1)                     // real TOTP: uint64(time.Now().Unix() / 30)
	code := hotp(secret, counter, 6)
	fmt.Printf("code: %06d\n", code)

	// Verification recomputes for the current counter (± a window for clock skew).
	fmt.Println("matches:      ", hotp(secret, 1, 6) == code)
	fmt.Println("wrong counter:", hotp(secret, 2, 6) == code)
}
```

**Output:**

```
code: 287082
matches:       true
wrong counter: false
```

---

## 24. Login rate limiting and lockout

`🔴 hard` · *abuse*

Password guessing is only viable if it's fast — so cap it. Track failed attempts per account and **lock** after a threshold; a correct password on a locked account is still refused until the lockout clears. (Production adds a time-based cooldown and often per-IP limits too.)

**Steps:**

1. `recordFail` increments a per-user counter; a success resets it.
2. `locked` is true once the counter hits the threshold.
3. Even the correct password is refused while locked.

```go
package main

import "fmt"

// Slow down brute force: count failed attempts per account and lock after a
// threshold. (A real impl adds a time-based cooldown; here we show the counter + gate.)
type Limiter struct {
	fails     map[string]int
	threshold int
}

func (l *Limiter) recordFail(user string) { l.fails[user]++ }
func (l *Limiter) reset(user string)      { delete(l.fails, user) }
func (l *Limiter) locked(user string) bool {
	return l.fails[user] >= l.threshold
}

func login(l *Limiter, user, pass, correct string) string {
	if l.locked(user) {
		return "locked"
	}
	if pass != correct {
		l.recordFail(user)
		return "wrong password"
	}
	l.reset(user)
	return "ok"
}

func main() {
	l := &Limiter{fails: map[string]int{}, threshold: 3}
	fmt.Println(login(l, "u1", "x", "secret"))
	fmt.Println(login(l, "u1", "y", "secret"))
	fmt.Println(login(l, "u1", "z", "secret"))
	fmt.Println(login(l, "u1", "secret", "secret")) // correct, but already locked
}
```

**Output:**

```
wrong password
wrong password
wrong password
locked
```

---

## 25. A secure password reset flow

`🔴 hard` · *reset*

A reset link is a temporary credential, so treat it like one: the token must be **random**, **single-use**, **expiring**, and stored **hashed** (only the emailed link has the raw value). On use, verify constant-time, check expiry, and mark it used so the link can't be replayed.

**Steps:**

1. Store `sha256(token)` + a user id + an expiry; email the raw token.
2. `consume` rejects used or expired tokens, then constant-time compares the hash.
3. Success marks it `Used` → a replay of the same link fails.

```go
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// A secure reset token is random, single-use, expiring, and stored HASHED (like a
// password). The raw token goes only in the emailed link; the DB keeps its hash.
type ResetToken struct {
	Hash   string
	UserID string
	Exp    int64
	Used   bool
}

func hashTok(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func consume(t *ResetToken, presentedRaw string, now int64) (string, error) {
	if t.Used {
		return "", fmt.Errorf("token already used")
	}
	if now >= t.Exp {
		return "", fmt.Errorf("token expired")
	}
	if subtle.ConstantTimeCompare([]byte(hashTok(presentedRaw)), []byte(t.Hash)) != 1 {
		return "", fmt.Errorf("invalid token")
	}
	t.Used = true // single-use: invalidate immediately
	return t.UserID, nil
}

func main() {
	raw := "emailed-reset-token"
	t := &ResetToken{Hash: hashTok(raw), UserID: "u1", Exp: 2000}

	user, err := consume(t, raw, 1500)
	fmt.Println("first use:", user, err)

	// Replaying the same link fails — it's single-use now.
	_, err = consume(t, raw, 1600)
	fmt.Println("replay:   ", err)
}
```

**Output:**

```
first use: u1 <nil>
replay:    token already used
```

---

## 26. Capstone: a full auth service

`🔴 hard` · *capstone*

Everything wired together: **register** (bcrypt hash), **login** (constant-time verify → session cookie), an **auth middleware** that reads the cookie, **/me**, and **logout** (revoke the session). Driven end-to-end with `httptest` + a `cookiejar` client, so the cookie flows exactly as a browser's would. Note the **uniform login error** (no user enumeration) and the instant server-side revocation on logout.

**Steps:**

1. `POST /register` bcrypt-hashes and stores the password.
2. `POST /login` verifies and sets an `HttpOnly` session cookie; `GET /me` is behind `auth`.
3. `POST /logout` deletes the session server-side → the next `/me` is 401.

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// A minimal auth service: register (hash), login (verify -> session cookie), an auth
// middleware reading the cookie, /me, and logout (revoke session).
type app struct {
	mu       sync.Mutex
	users    map[string][]byte // email -> bcrypt hash
	sessions map[string]string // session id -> email
	seq      int
}

func newApp() *app {
	return &app{users: map[string][]byte{}, sessions: map[string]string{}}
}

func (a *app) register(w http.ResponseWriter, r *http.Request) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(r.FormValue("password")), bcrypt.MinCost)
	a.mu.Lock()
	a.users[r.FormValue("email")] = hash
	a.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	a.mu.Lock()
	hash, ok := a.users[email]
	a.mu.Unlock()
	// Uniform error whether the email is unknown or the password is wrong (no enumeration).
	if !ok || bcrypt.CompareHashAndPassword(hash, []byte(r.FormValue("password"))) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	a.mu.Lock()
	a.seq++
	sid := fmt.Sprintf("sess-%d", a.seq) // real code: crypto/rand
	a.sessions[sid] = email
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "session", Value: sid, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		a.mu.Lock()
		email, ok := a.sessions[c.Value]
		a.mu.Unlock()
		if !ok {
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-User", email) // stash identity for the handler
		next(w, r)
	}
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "you are", r.Header.Get("X-User"))
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	a := newApp()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", a.register)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("GET /me", a.auth(a.me))
	mux.HandleFunc("POST /logout", a.logout)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	jar, _ := cookiejar.New(nil) // the client carries the session cookie automatically
	client := &http.Client{Jar: jar}
	form := url.Values{"email": {"a@x.com"}, "password": {"pw"}}

	post := func(path string) int {
		resp, _ := client.PostForm(srv.URL+path, form)
		resp.Body.Close()
		return resp.StatusCode
	}
	get := func(path string) (int, string) {
		resp, _ := client.Get(srv.URL + path)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, strings.TrimSpace(string(b))
	}

	fmt.Println("register:", post("/register"))
	fmt.Println("login:   ", post("/login"))
	code, body := get("/me")
	fmt.Printf("me:       %d %q\n", code, body)
	fmt.Println("logout:  ", post("/logout"))
	code, body = get("/me")
	fmt.Printf("me again: %d %q\n", code, body)
}
```

**Output:**

```
register: 201
login:    200
me:       200 "you are a@x.com"
logout:   204
me again: 401 "invalid session"
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
