# Step 56 — Authentication & Sessions · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# examples 1 & 3 also need: go get golang.org/x/crypto/bcrypt golang.org/x/crypto/argon2
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

The primitives: **password hashing**, **secure randomness / constant-time compare**, and **cookies + sessions**.

---

## 1. Hash a password with bcrypt

`🟢 easy` · *passwords*

Never store a password — store a **slow, salted hash**. `bcrypt.GenerateFromPassword` salts and hashes with a tunable **cost** (embedded in the output, so a different hash comes out every call). `CompareHashAndPassword` re-derives the salt and compares in constant time.

**Steps:**

1. `go get golang.org/x/crypto/bcrypt`.
2. `GenerateFromPassword(pw, bcrypt.DefaultCost)`; read the cost back with `bcrypt.Cost`.
3. `CompareHashAndPassword` returns `nil` on a match.

```go
package main

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := []byte("s3cr3t-p@ss")
	// GenerateFromPassword salts + hashes; the salt is embedded in the output, so a
	// different hash comes out every call (that's correct — never store plaintext).
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	cost, _ := bcrypt.Cost(hash) // the work factor is embedded in the hash
	fmt.Println("cost:", cost)

	// Verify: CompareHashAndPassword re-derives the salt and is constant-time.
	fmt.Println("correct password:", bcrypt.CompareHashAndPassword(hash, password) == nil)
	fmt.Println("wrong password:  ", bcrypt.CompareHashAndPassword(hash, []byte("wrong")) == nil)
}
```

**Output:**

```
cost: 10
correct password: true
wrong password:   false
```

---

## 2. Why not plain SHA-256

`🟢 easy` · *passwords*

Two reasons a fast hash is wrong for passwords: it's **unsalted** (equal passwords produce equal hashes, leaking that two users share one) and **fast** (a GPU tries billions/sec). This example contrasts SHA-256 with the right tool for comparing secrets: `subtle.ConstantTimeCompare`.

**Steps:**

1. Hash the same password twice with SHA-256 — the digests are identical.
2. That's a downgrade attack surface; use bcrypt/argon2 for passwords.
3. For comparing tokens/MACs, use `subtle.ConstantTimeCompare` (no early exit → no timing leak).

```go
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
)

func main() {
	// Plain SHA-256 is WRONG for passwords: it's fast (billions/sec ⇒ brute-forceable)
	// and unsalted (equal passwords ⇒ equal hashes). Shown only to contrast with bcrypt.
	a := sha256.Sum256([]byte("password123"))
	b := sha256.Sum256([]byte("password123"))
	fmt.Println("unsalted: identical hashes:", a == b) // leaks that two users share a password

	// When comparing secrets (tokens, MACs) use a constant-time compare so an attacker
	// can't learn the answer byte-by-byte from timing. == on strings can short-circuit.
	x := []byte("expected-token")
	fmt.Println("subtle equal (match):", subtle.ConstantTimeCompare(x, []byte("expected-token")) == 1)
	fmt.Println("subtle equal (diff): ", subtle.ConstantTimeCompare(x, []byte("wrong")) == 1)
}
```

**Output:**

```
unsalted: identical hashes: true
subtle equal (match): true
subtle equal (diff):  false
```

---

## 3. Argon2id password hashing

`🟢 easy` · *passwords*

`argon2id` is the modern, memory-hard password KDF (OWASP's current first choice). `argon2.IDKey(pw, salt, time, memory, threads, keyLen)` derives a key; you store the salt + params + key and verify by re-deriving and constant-time comparing. The salt is normally **16 random bytes** — fixed here only so the output is stable.

**Steps:**

1. `go get golang.org/x/crypto/argon2`.
2. `argon2.IDKey(pw, salt, 1, 64*1024, 4, 32)` — time=1, memory=64MB, threads=4, len=32.
3. Verify by re-deriving with the same salt/params and `subtle.ConstantTimeCompare`.

```go
package main

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

func main() {
	password := []byte("s3cr3t-p@ss")
	// Argon2id is the modern memory-hard password KDF. Params: time=1, memory=64MB,
	// threads=4, keyLen=32. In real code the salt is 16 RANDOM bytes stored with the
	// hash; fixed here so the demo output is stable.
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey(password, salt, 1, 64*1024, 4, 32)
	fmt.Println("derived key (b64):", base64.RawStdEncoding.EncodeToString(key))

	// Verify by re-deriving with the same salt+params and constant-time comparing.
	check := argon2.IDKey([]byte("s3cr3t-p@ss"), salt, 1, 64*1024, 4, 32)
	fmt.Println("verify:", subtle.ConstantTimeCompare(key, check) == 1)
}
```

**Output:**

```
derived key (b64): QZZgkoTnlD1HlALW3VhvRuPBjppjuakDZ26Aao1szbY
verify: true
```

---

## 4. Generate a secure random token

`🟢 easy` · *randomness*

Session IDs, API keys, CSRF and reset tokens must be **unguessable** — use `crypto/rand`, never `math/rand` (which is predictable). Encode with `base64.RawURLEncoding` so the token is safe in cookies and URLs. Don't log the tokens themselves.

**Steps:**

1. Fill a byte slice with `crypto/rand.Read`.
2. Encode with `base64.RawURLEncoding` (URL-safe, no padding).
3. Two calls produce different tokens; 32 bytes → 43 chars.

```go
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	// Session IDs, API keys, CSRF tokens: use crypto/rand, NEVER math/rand.
	newToken := func(n int) string {
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil { // cryptographically secure bytes
			panic(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	t1 := newToken(32)
	t2 := newToken(32)
	// Don't print the tokens (secret + random) — show they differ and their size.
	fmt.Println("length (chars):   ", len(t1))
	fmt.Println("two tokens differ:", t1 != t2)
	fmt.Println("url-safe alphabet:", base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xfe, 0xfd}))
}
```

**Output:**

```
length (chars):    43
two tokens differ: true
url-safe alphabet: __79
```

---

## 5. Set and read a cookie

`🟢 easy` · *cookies*

Sessions ride in a cookie. `http.SetCookie(w, &http.Cookie{...})` writes the `Set-Cookie` header; `r.Cookie("name")` reads it back on the next request. Here `httptest` lets us drive both sides without a network.

**Steps:**

1. A handler calls `http.SetCookie` — inspect the `Set-Cookie` header on the recorder.
2. A second request carries the cookie (`req.AddCookie`).
3. `r.Cookie("session")` returns it (or an error if absent).

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func main() {
	// A handler that sets a session cookie.
	setter := func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
	}
	rec := httptest.NewRecorder()
	setter(rec, httptest.NewRequest("GET", "/login", nil))
	fmt.Println("Set-Cookie:", rec.Header().Get("Set-Cookie"))

	// A handler that reads it back with r.Cookie.
	reader := func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil {
			fmt.Fprintln(w, "no cookie")
			return
		}
		fmt.Fprintln(w, "got:", c.Value)
	}
	req := httptest.NewRequest("GET", "/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	rec2 := httptest.NewRecorder()
	reader(rec2, req)
	fmt.Print(rec2.Body.String())
}
```

**Output:**

```
Set-Cookie: session=abc123; Path=/
got: abc123
```

---

## 6. Harden cookie attributes

`🟢 easy` · *cookies*

A session cookie is a bearer credential — lock it down. `HttpOnly` hides it from JavaScript (an XSS bug can't steal it), `Secure` restricts it to HTTPS, `SameSite` blocks it on cross-site requests (a CSRF defense), and `MaxAge` bounds its life.

**Steps:**

1. Set `HttpOnly`, `Secure`, `SameSite`, `Path`, `MaxAge` on the `http.Cookie`.
2. `http.SetCookie` serializes all of them into one header.
3. Read the resulting `Set-Cookie` string.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func main() {
	// A session cookie hardened against theft and CSRF:
	//   HttpOnly -> JavaScript can't read it (an XSS bug can't steal it)
	//   Secure   -> sent only over HTTPS
	//   SameSite -> not sent on cross-site requests (a CSRF defense)
	//   MaxAge   -> lifetime in seconds
	c := &http.Cookie{
		Name:     "session",
		Value:    "abc123",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600,
	}
	rec := httptest.NewRecorder()
	http.SetCookie(rec, c)
	fmt.Println(rec.Header().Get("Set-Cookie"))
}
```

**Output:**

```
session=abc123; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Lax
```

---

## 7. A server-side session store

`🟢 easy` · *sessions*

The alternative to a stateless token: keep session data server-side, keyed by the cookie's ID. Its superpower is **instant revocation** — logout is just `delete`. A mutex-guarded map is the minimal store (Redis/DB in production).

**Steps:**

1. `Create(id, sess)` on login; `Get(id)` per request; `Delete(id)` on logout.
2. Guard the map with a `sync.RWMutex`.
3. After `Delete`, lookups fail → the user is logged out server-side.

```go
package main

import (
	"fmt"
	"sync"
)

type Session struct{ UserID int }

// A minimal server-side session store. The cookie holds only the id; the data
// lives here, so logout = delete (instantly revocable, unlike a stateless token).
type Store struct {
	mu sync.RWMutex
	m  map[string]Session
}

func NewStore() *Store { return &Store{m: map[string]Session{}} }

func (s *Store) Create(id string, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = sess
}
func (s *Store) Get(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.m[id]
	return sess, ok
}
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
}

func main() {
	store := NewStore()
	store.Create("sess-1", Session{UserID: 42}) // on login

	sess, ok := store.Get("sess-1")
	fmt.Println("after login:", sess.UserID, ok)

	store.Delete("sess-1") // on logout
	_, ok = store.Get("sess-1")
	fmt.Println("after logout, found:", ok)
}
```

**Output:**

```
after login: 42 true
after logout, found: false
```

---

## 8. Hash session IDs at rest

`🟢 easy` · *sessions*

A session ID is a bearer token: whoever holds it is logged in. So store its **hash**, not the raw value — if your session table leaks, the attacker gets hashes, not live sessions (same reasoning as password storage). Compare in constant time.

**Steps:**

1. Send the raw ID in the cookie; persist `sha256(id)`.
2. On each request, hash the presented ID and `subtle.ConstantTimeCompare` to the stored hash.
3. A forged/guessed ID doesn't match.

```go
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// Store only the HASH of a session id, like a password. If the session table leaks,
// the raw ids (which are bearer tokens) aren't exposed.
func hashID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

func main() {
	rawID := "the-secret-session-id" // sent to the client in the cookie
	stored := hashID(rawID)          // what the server persists

	// On each request, hash the presented id and constant-time compare to stored.
	presented := "the-secret-session-id"
	match := subtle.ConstantTimeCompare([]byte(hashID(presented)), []byte(stored)) == 1
	fmt.Println("valid session: ", match)

	forged := hashID("guessed-id")
	fmt.Println("forged matches:", subtle.ConstantTimeCompare([]byte(forged), []byte(stored)) == 1)
}
```

**Output:**

```
valid session:  true
forged matches: false
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
