# Step 67 — Multi-User State in One Process · 🟢 Easy — examples **1–8**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # these are all concurrent — run go run -race . too
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. One process, many users: the shared global

`🟢 easy` · *Memory model*

In PHP each request is a fresh process, so a global resets to zero every time. In Go the process lives for weeks and **every request goroutine shares the same variable** — so 200 concurrent requests fight over one `int` and increments silently vanish. This is the single most important difference to internalize before writing a Go server.

**Steps:**

1. Declare a package-level `var visits int` and a handler that increments it — written as the two-step `n := visits; visits = n + 1` that `visits++` actually compiles to.
2. In `main`, launch 200 goroutines that each build a request with `httptest.NewRequest` + `httptest.NewRecorder` and call the handler directly — exactly what the mux does for a real request.
3. Print the final count and how many updates were lost. Run it several times: the number changes every run and is essentially never 200.
4. Run `go run -race .` — the detector names the read line and the write line from two different goroutines.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// In PHP every request is a fresh process: this variable would be 0 again
// on each hit. In Go the process is long-lived, so EVERY request goroutine
// shares this one variable — and 200 of them are about to fight over it.
var visits int

func handler(w http.ResponseWriter, r *http.Request) {
	// This is what visits++ actually compiles to: load, add, store.
	// Two goroutines can both load 41, both store 42 — one visit vanishes.
	n := visits
	visits = n + 1
	fmt.Fprintln(w, "ok")
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() { // one goroutine per request — exactly what net/http does
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			handler(rec, req)
		}()
	}
	wg.Wait()

	fmt.Println("visits:", visits, "of 200 requests")
	fmt.Println("lost updates:", 200-visits)
}
```

**Output:**

```
visits: 193 of 200 requests
lost updates: 7
```

*(your numbers will differ every run — that is the data race)*

---

## 2. The fix: a type that owns its lock

`🟢 easy` · *Guarded state*

The repair isn't "add a mutex somewhere" — it's to make the shared data and the lock that guards it **the same type**, with the lock unexported. Then no handler can access the counter without going through a method that locks, because there is no other way in.

**Steps:**

1. Define `Counter` with an unexported `mu sync.Mutex` and `n int`, plus pointer-receiver `Inc()` and `Value()` methods that both `Lock` + `defer Unlock`.
2. Write `makeHandler(visits *Counter) http.HandlerFunc` — the handler **closes over** the counter instead of reading a global. This is how you avoid package-level state entirely.
3. Fire the same 200 concurrent requests and confirm the total is exactly 200, every time.
4. Run under `-race`: clean.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Counter is the fix: the lock lives INSIDE the type, so no handler
// can forget to take it.
type Counter struct {
	mu sync.Mutex
	n  int
}

func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// The handler closes over the counter instead of using a global.
func makeHandler(visits *Counter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		visits.Inc()
		fmt.Fprintln(w, "ok")
	}
}

func main() {
	visits := &Counter{}
	h := makeHandler(visits)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		}()
	}
	wg.Wait()

	fmt.Println("visits:", visits.Value())
	fmt.Println("exactly 200:", visits.Value() == 200)
}
```

**Output:**

```
visits: 200
exactly 200: true
```

---

## 3. Request-scoped state is free

`🟢 easy` · *Scopes*

Every request runs on its own goroutine with its own stack, so **variables declared inside the handler are private to that request**. No other user can see or touch them, which means they need no lock, no atomic, nothing. Preferring locals over shared state is the cheapest concurrency strategy there is.

**Steps:**

1. Write a handler whose every variable is local: it reads `?user=` from the query, computes a total in a loop, and writes the answer to the recorder.
2. Fire 5 concurrent requests for 5 different users, each writing into its **own index** of a `results` slice — disjoint indices, so no lock is needed there either.
3. Sort and print: every request got its own correct answer, with zero synchronization anywhere in the program.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
)

// Every request runs on its own goroutine with its own stack, so locals
// declared inside the handler are private to that request. Nothing here
// is shared, so nothing here needs a lock.
func handler(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user") // request-local
	total := 0                        // request-local
	for i := 1; i <= 4; i++ {
		total += i // no other request can see or touch this
	}
	fmt.Fprintf(w, "%s=%d", user, total)
}

func main() {
	users := []string{"alice", "bob", "carol", "dave", "erin"}

	results := make([]string, len(users))
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/?user="+u, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)
			results[i] = rec.Body.String() // disjoint index: no lock needed
		}()
	}
	wg.Wait()

	sort.Strings(results)
	fmt.Println("each request got its own answer:")
	fmt.Println(" ", strings.Join(results, " "))
}
```

**Output:**

```
each request got its own answer:
  alice=10 bob=10 carol=10 dave=10 erin=10
```

---

## 4. httptest: calling a handler with no server

`🟢 easy` · *Tooling*

`httptest.NewRequest` builds a `*http.Request` and `httptest.NewRecorder` is a fake `ResponseWriter` that captures the reply — **no network, no port, no server**. This is how you exercise handlers in tests and in every example that follows.

**Steps:**

1. Write a handler that rejects non-POST with `http.Error(w, …, http.StatusMethodNotAllowed)` and otherwise sets a header, writes `201`, and encodes JSON.
2. Call it with a recorder, then inspect `rec.Result()` for the status and headers and `rec.Body.String()` for the body.
3. Call it again with `GET` to exercise the rejected path — two lines per case, no server lifecycle to manage.
4. Remember the ordering rule: headers must be set **before** `WriteHeader`, and `WriteHeader` is called exactly once.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
)

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func main() {
	// httptest.NewRequest builds a *http.Request with no network involved.
	// httptest.NewRecorder is a fake ResponseWriter that captures the reply.
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/orders", nil))

	res := rec.Result()
	fmt.Println("status:", res.StatusCode)
	fmt.Println("content-type:", res.Header.Get("Content-Type"))
	fmt.Print("body: ", rec.Body.String())

	// The rejected path, same two lines.
	rec2 := httptest.NewRecorder()
	handler(rec2, httptest.NewRequest(http.MethodGet, "/orders", nil))
	fmt.Println("GET status:", rec2.Result().StatusCode)
}
```

**Output:**

```
status: 201
content-type: application/json
body: {"status":"created"}
GET status: 405
```

---

## 5. A real server, 100 real clients

`🟢 easy` · *Concurrency*

`httptest.NewServer` starts an actual `http.Server` on a random localhost port, so the requests are genuine TCP + HTTP and `net/http` really does spawn one goroutine per request. This is the proof that your handler runs concurrently with itself.

**Steps:**

1. Start `httptest.NewServer` with a handler that bumps an `atomic.Int64` and writes `ok`; `defer srv.Close()`.
2. Launch 100 goroutines that each `http.Get(srv.URL)`, then **drain and close the body** — skipping the drain prevents connection reuse and leaks.
3. After `wg.Wait()`, confirm all 100 arrived.
4. Note the counter is `atomic.Int64`, not `int`: the handler body is executing in 100 goroutines at once.

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

func main() {
	var served atomic.Int64 // real concurrency now: atomic, not a plain int

	// httptest.NewServer starts a REAL http.Server on a random localhost
	// port. From here on the requests are genuine TCP + HTTP.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	fmt.Println("server listening on a random port:", srv.URL != "")

	// 100 clients hit it at once. net/http runs each one in its own
	// goroutine, so the handler above is executing 100 times in parallel.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := http.Get(srv.URL)
			if err != nil {
				return
			}
			defer res.Body.Close()
			io.Copy(io.Discard, res.Body) // drain so the connection is reused
		}()
	}
	wg.Wait()

	fmt.Println("requests served:", served.Load())
	fmt.Println("all 100 arrived:", served.Load() == 100)
}
```

**Output:**

```
server listening on a random port: true
requests served: 100
all 100 arrived: true
```

---

## 6. Identity: credential → context → handler

`🟢 easy` · *Identity*

The universal shape of a multi-user server: middleware verifies the credential **once**, stashes the identity in the request context under an unexported key, and every handler downstream reads it from there — never re-parsing the token. One handler value serves all users; the identity rides in each request's context.

**Steps:**

1. Declare `type userKey struct{}` (unexported, zero bytes, collision-proof) and a `userFrom(ctx)` helper using the comma-ok assertion.
2. Write `authMiddleware`: read the header, reject with `401` and `return` if absent (the chain stops — `next` is never called), otherwise build the `User`.
3. Attach it with `context.WithValue` and call `next.ServeHTTP(w, r.WithContext(ctx))` — `WithContext` returns a **copy** of the request; you never mutate a `*http.Request` in place.
4. In the handler, read the identity back and handle `ok == false` defensively — a handler must not assume its middleware ran.

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
)

// userKey is unexported, so no other package can read or overwrite
// the identity we stash. Same typed-key idiom as lesson 15 example 10.
type userKey struct{}

type User struct {
	ID   string
	Role string
}

// userFrom is the ONLY way handlers get the identity — they never parse
// the token themselves.
func userFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// authMiddleware verifies the credential ONCE, then puts the identity
// in the request context for everything downstream.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return // the chain stops here: next is never called
		}

		user := User{ID: "u-" + token, Role: "member"} // pretend lookup

		// r.WithContext returns a COPY of the request with the new ctx.
		// You never mutate a *http.Request in place.
		ctx := context.WithValue(r.Context(), userKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func profile(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok { // defensive: the handler must not assume middleware ran
		http.Error(w, "no identity", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "id=%s role=%s", u.ID, u.Role)
}

func main() {
	h := authMiddleware(http.HandlerFunc(profile))

	// Two different users hit the SAME handler value. The handler is
	// shared; the identity is not — it rides in each request's context.
	for _, token := range []string{"alice", "bob"} {
		req := httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		fmt.Printf("%-6s -> %d %s\n", token, rec.Code, rec.Body.String())
	}

	// No credential: middleware rejects, profile never runs.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	fmt.Printf("%-6s -> %d %s", "(none)", rec.Code, rec.Body.String())
}
```

**Output:**

```
alice  -> 200 id=u-alice role=member
bob    -> 200 id=u-bob role=member
(none) -> 401 unauthorized
```

---

## 7. `r.Context()` is cancelled when the client leaves

`🟢 easy` · *Request context*

Every request carries a context that `net/http` **cancels when the client disconnects**. Watching `<-r.Context().Done()` is how a handler stops doing expensive work whose result nobody will ever receive — a browser tab closed, a mobile app backgrounded, an upstream timeout.

**Steps:**

1. Start a real `httptest.NewServer` whose handler `select`s between `time.After(2*time.Second)` (the "slow query") and `<-r.Context().Done()`.
2. Report the outcome on a **buffered** channel (cap 1) so the handler can never block on the send even though its client is long gone.
3. Build a client request with `http.NewRequestWithContext` and a 100ms timeout — far shorter than the 2s of work.
4. The client errors out; the handler wakes on `Done()` and reports `context canceled` instead of finishing.

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"
)

func main() {
	// The handler reports WHY it stopped, over a buffered channel so it
	// can never block even though its client has walked away.
	outcome := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.Context() is cancelled when the CLIENT disconnects. Watching it
		// is how you stop doing work nobody will receive.
		select {
		case <-time.After(2 * time.Second): // the "slow query"
			outcome <- "finished the work"
			fmt.Fprint(w, "done")
		case <-r.Context().Done():
			outcome <- "client left: " + r.Context().Err().Error()
			return // stop immediately; writing to w now is pointless
		}
	}))
	defer srv.Close()

	// The client gives up after 100ms — far sooner than the 2s of work.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, err := http.DefaultClient.Do(req)
	fmt.Println("client error:", err != nil)

	fmt.Println("handler:", <-outcome)
}
```

**Output:**

```
client error: true
handler: client left: context canceled
```

---

## 8. Per-user data in one shared map

`🟢 easy` · *User-scoped state*

User-scoped state has **two levels**: the map is shared by all users, and each entry is shared by one user's concurrent requests. One mutex covering the whole map handles both. Note `Snapshot` copies before returning — handing out the internal map would let the caller read it with no lock held.

**Steps:**

1. Define `Store` with `mu sync.Mutex` and `counts map[string]int`, plus a constructor (a nil map panics on write, so the zero value is not usable here).
2. `Hit(user)` increments and **reads the value back inside the same lock** — releasing and re-reading would give you someone else's number.
3. `Snapshot()` copies the map inside the lock and returns the copy.
4. Fire 3 users × 50 concurrent requests each, then print the sorted totals: exactly 50 apiece.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
)

// Store holds per-user data for EVERY user in one shared map.
// One mutex guards the whole map.
type Store struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewStore() *Store {
	return &Store{counts: make(map[string]int)} // nil map would panic on write
}

func (s *Store) Hit(user string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[user]++
	return s.counts[user] // read it back INSIDE the lock
}

func (s *Store) Snapshot() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Copy before handing it out: returning s.counts itself would let the
	// caller read the map with no lock held — a data race.
	out := make(map[string]int, len(s.counts))
	for k, v := range s.counts {
		out[k] = v
	}
	return out
}

func main() {
	store := NewStore()
	h := func(w http.ResponseWriter, r *http.Request) {
		user := r.URL.Query().Get("user")
		fmt.Fprint(w, store.Hit(user))
	}

	// 3 users, 50 concurrent requests each, all interleaved.
	var wg sync.WaitGroup
	for _, u := range []string{"alice", "bob", "carol"} {
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h(httptest.NewRecorder(), httptest.NewRequest("GET", "/?user="+u, nil))
			}()
		}
	}
	wg.Wait()

	snap := store.Snapshot()
	users := make([]string, 0, len(snap))
	for u := range snap {
		users = append(users, u)
	}
	sort.Strings(users) // map order is random
	for _, u := range users {
		fmt.Printf("%-6s %d\n", u, snap[u])
	}
}
```

**Output:**

```
alice  50
bob    50
carol  50
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
