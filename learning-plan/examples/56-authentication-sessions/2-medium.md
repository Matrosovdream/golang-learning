# Step 56 — Authentication & Sessions · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

**JWTs from the bytes up** — build, verify, expire, and defend them — plus the auth middleware and the access/refresh token lifecycle. All stdlib crypto.

---

## 9. Build a JWT by hand

`🟡 medium` · *jwt*

A JWT isn't magic: it's `base64url(header) + "." + base64url(claims) + "." + base64url(HMAC-SHA256(...))`. Building one with `crypto/hmac` shows exactly what a library does — and makes the attacks (examples 10, 17) obvious.

**Steps:**

1. JSON-encode a header (`alg`/`typ`) and claims (`sub`/`exp`).
2. `signingInput = b64(header) + "." + b64(claims)`.
3. Append `b64(HMAC-SHA256(secret, signingInput))`.

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func sign(secret, msg []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(msg)
	return b64(h.Sum(nil))
}

func main() {
	secret := []byte("my-signing-key")
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"sub": "user-42", "exp": 2000000000})

	// A JWT is base64url(header) "." base64url(claims) "." base64url(HMAC(that)).
	signingInput := b64(header) + "." + b64(claims)
	token := signingInput + "." + sign(secret, []byte(signingInput))
	fmt.Println(token)
}
```

**Output:**

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjIwMDAwMDAwMDAsInN1YiI6InVzZXItNDIifQ.14xS-uFNMQwbGWiX2o0dR4K4osr-wqtqq4190Ri0Cyk
```

---

## 10. Verify a JWT

`🟡 medium` · *jwt*

Verification is the security-critical half: **recompute** the HMAC over `header.payload` and **constant-time compare** to the token's signature with `hmac.Equal`. If they match, the payload is authentic; if anyone tampered with it, the signature won't.

**Steps:**

1. Split into three parts; recompute `sign(secret, parts[0]+"."+parts[1])`.
2. `hmac.Equal` the recomputed signature against `parts[2]`.
3. Tampering with the payload breaks the match.

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
func sign(secret, msg []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(msg)
	return b64(h.Sum(nil))
}

func verify(token string, secret []byte) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	expected := sign(secret, []byte(parts[0]+"."+parts[1]))
	// Constant-time signature compare — this is the entire security of the token.
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, false
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	json.Unmarshal(raw, &claims)
	return claims, true
}

func main() {
	secret := []byte("my-signing-key")
	header := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := b64([]byte(`{"sub":"user-42"}`))
	token := header + "." + claims + "." + sign(secret, []byte(header+"."+claims))

	c, ok := verify(token, secret)
	fmt.Println("valid:", ok, "sub:", c["sub"])

	// Tamper with the payload -> signature no longer matches.
	tampered := header + "." + b64([]byte(`{"sub":"admin"}`)) + "." + strings.Split(token, ".")[2]
	_, ok = verify(tampered, secret)
	fmt.Println("tampered valid:", ok)
}
```

**Output:**

```
valid: true sub: user-42
tampered valid: false
```

---

## 11. Check token expiry claims

`🟡 medium` · *jwt*

A valid signature isn't enough — a token also has a lifetime. Check `nbf` (not-before) ≤ now < `exp` (expiry). Inject `now` instead of calling `time.Now()` so the logic is deterministic and testable.

**Steps:**

1. `now < nbf` → not yet valid.
2. `now >= exp` → expired.
3. Otherwise valid. (Real code passes `time.Now().Unix()`.)

```go
package main

import "fmt"

type Claims struct {
	Sub string
	Iat int64
	Nbf int64
	Exp int64
}

// validateTime checks nbf <= now < exp using an INJECTED now, so the test is
// deterministic (real code passes time.Now().Unix()).
func validateTime(c Claims, now int64) error {
	if now < c.Nbf {
		return fmt.Errorf("token not yet valid")
	}
	if now >= c.Exp {
		return fmt.Errorf("token expired")
	}
	return nil
}

func main() {
	c := Claims{Sub: "u1", Iat: 1000, Nbf: 1000, Exp: 2000}
	fmt.Println("at 1500:", validateTime(c, 1500))
	fmt.Println("at 2500:", validateTime(c, 2500))
	fmt.Println("at 500: ", validateTime(c, 500))
}
```

**Output:**

```
at 1500: <nil>
at 2500: token expired
at 500:  token not yet valid
```

---

## 12. Auth middleware with context identity

`🟡 medium` · *middleware*

The middleware verifies the credential **once** and stashes the identity in the request context under an **unexported key** (same pattern as [step 43](../../43-authorization-rbac-multitenancy.md)). Handlers read the identity, never the raw token. Missing/invalid tokens get a 401 before the handler runs.

**Steps:**

1. Extract the `Bearer` token with `strings.CutPrefix`.
2. Verify it; on failure write 401 and return.
3. On success, `context.WithValue(r.Context(), userKey, user)` and call `next`.

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

type ctxKey int

const userKey ctxKey = 0

// verifyToken stands in for real JWT/session verification.
func verifyToken(tok string) (string, bool) {
	if tok == "valid-token" {
		return "user-42", true
	}
	return "", false
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		user, ok := verifyToken(tok)
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		// Put identity in the context; handlers read that, never the raw token.
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

func me(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "hello", r.Context().Value(userKey))
}

func main() {
	h := authMiddleware(http.HandlerFunc(me))
	do := func(authHeader string) {
		req := httptest.NewRequest("GET", "/me", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		fmt.Printf("status=%d body=%q\n", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	do("Bearer valid-token")
	do("Bearer nope")
	do("")
}
```

**Output:**

```
status=200 body="hello user-42"
status=401 body="invalid token"
status=401 body="missing bearer token"
```

---

## 13. Access and refresh tokens

`🟡 medium` · *lifecycle*

Short-lived **access** tokens limit the damage if one leaks; a long-lived **refresh** token lets the client get a new access token without re-login. The refresh endpoint checks the refresh token against server state and mints a fresh access token.

**Steps:**

1. `issue` returns an access token (~15 min) and a refresh token (~30 days).
2. The server tracks live refresh tokens (keyed to a user).
3. `refresh` swaps a valid refresh token for a new access token; an unknown one fails.

```go
package main

import "fmt"

type TokenPair struct {
	Access  string
	Refresh string
}

// issue creates a SHORT-lived access token and a LONG-lived refresh token.
func issue(userID string) TokenPair {
	return TokenPair{
		Access:  "access-for-" + userID,  // e.g. a JWT, exp ~15 min
		Refresh: "refresh-for-" + userID, // opaque, stored server-side, exp ~30 days
	}
}

// refresh swaps a valid refresh token for a NEW access token (no re-login).
func refresh(refreshTok string, live map[string]string) (string, bool) {
	userID, ok := live[refreshTok]
	if !ok {
		return "", false
	}
	return "access-for-" + userID, true
}

func main() {
	pair := issue("u1")
	fmt.Println("access: ", pair.Access)
	fmt.Println("refresh:", pair.Refresh)

	live := map[string]string{pair.Refresh: "u1"} // server tracks live refresh tokens
	newAccess, ok := refresh(pair.Refresh, live)
	fmt.Println("refreshed:", newAccess, ok)
	_, ok = refresh("stolen-guess", live)
	fmt.Println("bad refresh:", ok)
}
```

**Output:**

```
access:  access-for-u1
refresh: refresh-for-u1
refreshed: access-for-u1 true
bad refresh: false
```

---

## 14. Refresh rotation and reuse detection

`🟡 medium` · *lifecycle*

Best practice: **rotate** the refresh token on every use (issue a new one, invalidate the old). If an **already-rotated** token is presented again, that's the fingerprint of a stolen token — detect the **reuse** and revoke the whole family.

**Steps:**

1. `current` holds the only valid token per user; `used` remembers rotated-away tokens.
2. `rotate(old, new)` invalidates `old`, records it as used, issues `new`.
3. Presenting a `used` token → "REUSE DETECTED" → revoke the family.

```go
package main

import "fmt"

// A refresh-token "family": rotate on every use, and detect reuse of an old
// (already-rotated) token — which means it was stolen — then revoke the family.
type RefreshStore struct {
	current map[string]string // token -> userID (only the latest is valid)
	used    map[string]bool   // tokens already rotated away
}

func newStore() *RefreshStore {
	return &RefreshStore{current: map[string]string{}, used: map[string]bool{}}
}

func (s *RefreshStore) issue(user, token string) { s.current[token] = user }

func (s *RefreshStore) rotate(oldTok, newTok string) (string, string) {
	user, isCurrent := s.current[oldTok]
	if !isCurrent {
		if s.used[oldTok] {
			return "", "REUSE DETECTED - revoking family"
		}
		return "", "unknown token"
	}
	delete(s.current, oldTok)
	s.used[oldTok] = true
	s.current[newTok] = user
	return newTok, "ok"
}

func main() {
	s := newStore()
	s.issue("u1", "t1")

	next, msg := s.rotate("t1", "t2")
	fmt.Println("rotate t1->t2:", next, msg)

	// Attacker replays the old t1 (already rotated). Detected as reuse.
	_, msg = s.rotate("t1", "t3")
	fmt.Println("replay t1:    ", msg)
}
```

**Output:**

```
rotate t1->t2: t2 ok
replay t1:     REUSE DETECTED - revoking family
```

---

## 15. Revoke a stateless JWT (denylist)

`🟡 medium` · *revocation*

A signed JWT is valid until `exp` — there's nothing to "delete". To log a user out sooner, keep a small **denylist** of revoked token IDs (`jti`), checked on every request. Entries only need to live until each token's `exp`, then you prune them.

**Steps:**

1. Each token carries a unique `jti`.
2. On logout, add the `jti` to the denylist.
3. `allowed(jti)` is false for revoked tokens, true for the rest.

```go
package main

import "fmt"

// Stateless JWTs can't be "deleted" — they're valid until exp. To log out sooner,
// keep a short-lived denylist of revoked token ids (jti), checked on each request.
type Denylist map[string]bool

func (d Denylist) revoke(jti string)       { d[jti] = true }
func (d Denylist) allowed(jti string) bool { return !d[jti] }

func main() {
	deny := Denylist{}
	fmt.Println("before logout:", deny.allowed("jti-abc"))

	deny.revoke("jti-abc") // on logout
	fmt.Println("after logout: ", deny.allowed("jti-abc"))
	fmt.Println("other token:  ", deny.allowed("jti-xyz"))
	// The denylist only needs each entry until that token's exp — then prune it.
}
```

**Output:**

```
before logout: true
after logout:  false
other token:   true
```

---

## 16. Session vs JWT: the revocation trade-off

`🟡 medium` · *design*

The whole session-vs-JWT decision in one program. A **server-side session** is a lookup key — deleting the record logs the user out **now**. A **stateless JWT** is valid until `exp`; plain logout can't touch it. Pick sessions when instant revocation matters, JWTs when statelessness/scale does.

**Steps:**

1. `SessionAuth.logout` deletes the record → immediately invalid.
2. `JWTAuth.valid` is just `now < exp` → logout does nothing to it.
3. That gap is the trade-off (mitigate JWTs with short TTLs + refresh revocation).

```go
package main

import "fmt"

// Server-side session: the token is a lookup key; deleting the record logs out NOW.
type SessionAuth struct{ live map[string]string }

func (a SessionAuth) valid(tok string) bool { _, ok := a.live[tok]; return ok }
func (a SessionAuth) logout(tok string)     { delete(a.live, tok) }

// Stateless JWT: "valid" = signature ok AND now < exp. Plain logout can't revoke it
// early; it stays valid until it expires. That's the core trade-off.
type JWTAuth struct{ now, exp int64 }

func (a JWTAuth) valid() bool { return a.now < a.exp }

func main() {
	s := SessionAuth{live: map[string]string{"tok": "u1"}}
	fmt.Println("session valid:       ", s.valid("tok"))
	s.logout("tok")
	fmt.Println("session after logout:", s.valid("tok")) // instantly invalid

	j := JWTAuth{now: 1500, exp: 2000}
	fmt.Println("jwt valid:           ", j.valid()) // unaffected by logout until exp
}
```

**Output:**

```
session valid:        true
session after logout: false
jwt valid:            true
```

---

## 17. Defend against the alg-confusion attack

`🟡 medium` · *jwt security*

The classic JWT exploit: an attacker rewrites the header to `alg:"none"` (drop the signature) or swaps `RS256→HS256` (sign with the public key as an HMAC secret). The defense is one line — **pin the algorithm you expect** and reject anything else. Never let the token's own header decide how it's verified.

**Steps:**

1. Parse the header's `alg`.
2. If it isn't the expected `HS256`, reject immediately.
3. Only then verify the HMAC — so `alg:"none"` never reaches the check.

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

// verify PINS the expected algorithm. Attackers try alg:"none" (drop the signature)
// or RS256->HS256 confusion (sign with the public key as an HMAC secret). Never
// trust the token's own header to tell you how to verify it.
func verify(token string, secret []byte) (bool, string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false, "malformed"
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	json.Unmarshal(raw, &hdr)
	if hdr.Alg != "HS256" { // <- the defense
		return false, "unexpected alg: " + hdr.Alg
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(parts[0] + "." + parts[1]))
	return hmac.Equal([]byte(b64(h.Sum(nil))), []byte(parts[2])), "checked"
}

func main() {
	secret := []byte("k")
	hdr := b64([]byte(`{"alg":"HS256"}`))
	pl := b64([]byte(`{"sub":"u1"}`))
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(hdr + "." + pl))
	good := hdr + "." + pl + "." + b64(h.Sum(nil))

	ok, msg := verify(good, secret)
	fmt.Println("HS256 token:", ok, msg)

	none := b64([]byte(`{"alg":"none"}`)) + "." + pl + "." // no signature
	ok, msg = verify(none, secret)
	fmt.Println("none token: ", ok, msg)
}
```

**Output:**

```
HS256 token: true checked
none token:  false unexpected alg: none
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
