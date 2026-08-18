# Step 67 — Multi-User State in One Process · 🟡 Medium — examples **9–17**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # these are all concurrent — run go run -race . too
```

> ← Back to the [index](README.md) · Previous tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. The escaped pointer: a lock that protects nothing

`🟡 medium` · *Guarded state*

A getter that dutifully takes the lock and then returns a **pointer** into the guarded map has protected nothing — the caller reads and writes that struct long after the lock was released. Return **copies**, dereferencing inside the critical section, so the store keeps control of its own state.

**Steps:**

1. Build a `Store` with `mu sync.RWMutex` and `users map[string]*Profile`.
2. Write `GetPointer` — `RLock`, `defer RUnlock`, `return s.users[id]`. It looks correct and is not.
3. Write `Get` returning `(Profile, bool)` with `return *p, true` — the dereference **inside** the lock is what copies the struct.
4. Mutate `u1` through the escaped pointer and `u2` through a copy, then print both: the store was modified in the first case and is intact in the second.

```go
package main

import (
	"fmt"
	"sync"
)

type Profile struct {
	Name  string
	Email string
}

type Store struct {
	mu    sync.RWMutex
	users map[string]*Profile
}

// GetPointer looks safe — it takes the lock! — but it hands the caller a
// POINTER into the map. Once the lock is released the caller can read and
// write that Profile with no lock at all. The lock protected nothing.
func (s *Store) GetPointer(id string) *Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[id] // the pointer escapes the critical section
}

// Get returns a COPY. The caller can do whatever it likes with it and the
// store's state stays under the store's control.
func (s *Store) Get(id string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.users[id]
	if !ok {
		return Profile{}, false
	}
	return *p, true // dereference INSIDE the lock: this copies the struct
}

func (s *Store) Email(id string) string {
	p, _ := s.Get(id)
	return p.Email
}

func main() {
	s := &Store{users: map[string]*Profile{
		"u1": {Name: "alice", Email: "alice@example.com"},
		"u2": {Name: "bob", Email: "bob@example.com"},
	}}

	// u1: mutate through the escaped pointer.
	p := s.GetPointer("u1")
	p.Email = "changed@example.com"

	// u2: mutate a copy.
	c, _ := s.Get("u2")
	c.Email = "changed@example.com"

	fmt.Println("u1 (pointer escaped): ", s.Email("u1"), "<- the store was modified")
	fmt.Println("u2 (copy returned):   ", s.Email("u2"), "<- the store is intact")

	// Concurrently, GetPointer is worse than "surprising": two goroutines
	// touching *p with no lock is a textbook data race that -race reports.
}
```

**Output:**

```
u1 (pointer escaped):  changed@example.com <- the store was modified
u2 (copy returned):    bob@example.com <- the store is intact
```

> The same trap applies to any **slice, map, or channel** field — copying the struct copies the header, not the backing array. For those you must deep-copy inside the lock.

---

## 10. A per-user rate limiter

`🟡 medium` · *User-scoped state*

The canonical user-scoped structure: one bucket per user in a shared map. Two levels of sharing (the map across all users, each bucket across one user's concurrent requests) and one mutex covering both. The critical detail is that `Allow` does check-**and**-decrement in a single critical section.

**Steps:**

1. Define `Limiter` with `mu`, `tokens map[string]int`, and a `perUser` budget.
2. In `Allow`, fill the bucket on first sight, return `false` at zero, otherwise decrement and return `true` — all under one `Lock`. Splitting the read and the write would let two requests both see the last token.
3. Wire it into a handler that returns `429 Too Many Requests` when refused.
4. Fire 2 users × 5 concurrent requests with a budget of 3, and tally the status codes per user.

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

// Limiter keeps ONE bucket per user in a shared map. Two levels of state:
// the map (shared by all users) and each bucket (shared by one user's
// concurrent requests). The single mutex covers both.
type Limiter struct {
	mu      sync.Mutex
	tokens  map[string]int
	perUser int
}

func NewLimiter(perUser int) *Limiter {
	return &Limiter{tokens: make(map[string]int), perUser: perUser}
}

// Allow is check-and-decrement in ONE critical section. Splitting it into
// a read then a write would let two requests both see the last token.
func (l *Limiter) Allow(user string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, seen := l.tokens[user]; !seen {
		l.tokens[user] = l.perUser // first request: fill the bucket
	}
	if l.tokens[user] == 0 {
		return false
	}
	l.tokens[user]--
	return true
}

func main() {
	limiter := NewLimiter(3) // 3 requests per user

	h := func(w http.ResponseWriter, r *http.Request) {
		user := r.URL.Query().Get("user")
		if !limiter.Allow(user) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, "ok")
	}

	var mu sync.Mutex
	codes := map[string][]int{}

	var wg sync.WaitGroup
	for _, u := range []string{"alice", "bob"} {
		for i := 0; i < 5; i++ { // 5 requests each, budget is 3
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := httptest.NewRecorder()
				h(rec, httptest.NewRequest("GET", "/?user="+u, nil))
				mu.Lock()
				codes[u] = append(codes[u], rec.Code)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	for _, u := range []string{"alice", "bob"} {
		got := codes[u]
		sort.Ints(got)
		ok, limited := 0, 0
		for _, c := range got {
			if c == http.StatusOK {
				ok++
			} else {
				limited++
			}
		}
		fmt.Printf("%-6s allowed=%d limited=%d %s\n", u, ok, limited,
			strings.Trim(strings.Join(strings.Fields(fmt.Sprint(got)), ","), "[]"))
	}
}
```

**Output:**

```
alice  allowed=3 limited=2 200,200,200,429,429
bob    allowed=3 limited=2 200,200,200,429,429
```

> A real limiter refills over time (token bucket — [36](../../36-resilience-patterns.md)) and, across replicas, lives in Redis (example 22).

---

## 11. Presence: who is online right now

`🟡 medium` · *RWMutex*

Presence is the textbook `RWMutex` case: written once per heartbeat, read by every dashboard request. Note the flaw the last line points at — filtering stale users on read hides them but never deletes them, so the map grows forever. Example 12 fixes that.

**Steps:**

1. Define `Presence` with `mu sync.RWMutex`, `lastSeen map[string]time.Time`, and a `ttl`.
2. `Seen(user)` takes the write lock for a single map assignment; `Online()` takes the read lock so many dashboards can run at once.
3. `Online()` filters by `t.After(cutoff)` and **sorts** before returning — map iteration order is random.
4. Add a `Len()` that also takes `RLock`: even reading `len(map)` from another goroutine needs the lock.
5. Heartbeat 3 users, sleep past the TTL, heartbeat only alice, and print — plus the entry count, which is still 3.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Presence answers "who is online right now?" — read constantly by every
// dashboard request, written once per user heartbeat. Reads vastly
// outnumber writes, which is exactly what RWMutex is for.
type Presence struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time
	ttl      time.Duration
}

func NewPresence(ttl time.Duration) *Presence {
	return &Presence{lastSeen: make(map[string]time.Time), ttl: ttl}
}

// Seen is the write path: exclusive lock, one map assignment, done.
func (p *Presence) Seen(user string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSeen[user] = time.Now()
}

// Online is the read path: many dashboards can run this at once.
func (p *Presence) Online() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var out []string
	cutoff := time.Now().Add(-p.ttl)
	for user, t := range p.lastSeen {
		if t.After(cutoff) { // seen recently enough to count as online
			out = append(out, user)
		}
	}
	sort.Strings(out) // map order is random; sort before returning
	return out
}

// Len reports how many users the map holds — including stale ones.
// Even this trivial read needs the lock.
func (p *Presence) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.lastSeen)
}

func main() {
	p := NewPresence(100 * time.Millisecond)

	// Three users heartbeat concurrently.
	var wg sync.WaitGroup
	for _, u := range []string{"alice", "bob", "carol"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Seen(u)
		}()
	}
	wg.Wait()
	fmt.Println("online now:      ", p.Online())

	// Let the TTL lapse, then only alice heartbeats again.
	time.Sleep(150 * time.Millisecond)
	p.Seen("alice")
	fmt.Println("after 150ms idle:", p.Online())

	// Note: Online() only FILTERS stale users — it never deletes them.
	// The map grows forever without a sweeper (next example).
	fmt.Println("map still holds:", p.Len(), "entries")
}
```

**Output:**

```
online now:       [alice bob carol]
after 150ms idle: [alice]
map still holds: 3 entries
```

---

## 12. The sweeper: a background goroutine that shuts down cleanly

`🟡 medium` · *Background work*

Any long-lived map needs something to evict from it. A sweeper is a **server-scoped** goroutine — it belongs to the process, not to any request, so it takes the *server's* context, never `r.Context()`. Cancelling it is a request; `wg.Wait()` is the confirmation it actually stopped.

**Steps:**

1. Add `sweep()` that takes the write lock and `delete`s entries older than the TTL (deleting during `range` is safe in Go), returning how many it removed.
2. Write `StartSweeper(ctx, every, wg)`: `wg.Add(1)` **before** the `go`, then a `for { select { <-ctx.Done() / <-t.C } }` loop.
3. `defer t.Stop()` on the ticker — a `time.Ticker` leaks its runtime resources if never stopped.
4. In `main`, heartbeat alice repeatedly while bob goes quiet, watch the sweeper expire him, then `cancel()` **and** `wg.Wait()`.

```go
package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Presence struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time
	ttl      time.Duration
}

func NewPresence(ttl time.Duration) *Presence {
	return &Presence{lastSeen: make(map[string]time.Time), ttl: ttl}
}

func (p *Presence) Seen(user string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSeen[user] = time.Now()
}

func (p *Presence) Online() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.lastSeen))
	for u := range p.lastSeen {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// sweep deletes entries older than the TTL. Returns how many it removed.
func (p *Presence) sweep() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	cutoff := time.Now().Add(-p.ttl)
	n := 0
	for user, t := range p.lastSeen {
		if t.Before(cutoff) {
			delete(p.lastSeen, user) // deleting during range is safe in Go
			n++
		}
	}
	return n
}

// StartSweeper runs sweep on a ticker until ctx is cancelled. It is a
// background goroutine that belongs to the SERVER, not to any request —
// so it gets the server's context, never a request's.
func (p *Presence) StartSweeper(ctx context.Context, every time.Duration, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(every)
		defer t.Stop() // a ticker leaks its goroutine if you never Stop it
		for {
			select {
			case <-ctx.Done():
				fmt.Println("sweeper: stopping —", ctx.Err())
				return
			case <-t.C:
				if n := p.sweep(); n > 0 {
					fmt.Println("sweeper: expired", n, "user(s)")
				}
			}
		}
	}()
}

func main() {
	p := NewPresence(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	p.StartSweeper(ctx, 30*time.Millisecond, &wg)

	p.Seen("alice")
	p.Seen("bob")
	fmt.Println("online:", p.Online())

	// Only alice keeps heartbeating; bob goes quiet and gets swept.
	for i := 0; i < 4; i++ {
		time.Sleep(50 * time.Millisecond)
		p.Seen("alice")
	}
	fmt.Println("online:", p.Online())

	cancel()  // tell the sweeper to stop
	wg.Wait() // and WAIT for it to actually be gone
	fmt.Println("shutdown complete")
}
```

**Output:**

```
online: [alice bob]
sweeper: expired 1 user(s)
online: [alice]
sweeper: stopping — context canceled
shutdown complete
```

---

## 13. Hot-reloaded config with `atomic.Pointer`

`🟡 medium` · *Atomics*

Config is read on every request by every user and written maybe once an hour. A mutex would make every request pay for a lock; `atomic.Pointer[T]` makes reads completely free while still guaranteeing each request sees a **complete** snapshot — never a half-updated struct.

**Steps:**

1. Declare `var cfg atomic.Pointer[Config]` and seed it with `cfg.Store(&Config{...})`.
2. In the handler, call `cfg.Load()` **once** into a local and read all fields off that local — two separate `Load()` calls could straddle a swap and mix old and new fields.
3. Hot reload by building a brand-new `Config` and `Store`ing it. Never mutate a stored one: readers are holding that pointer right now.
4. Run concurrent requests before and after the swap and print both.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

// Config is read on EVERY request by EVERY user and written maybe once an
// hour. A mutex would make every request pay for a lock; atomic.Pointer
// makes reads completely free.
type Config struct {
	Version   int
	Greeting  string
	MaxUpload int
}

var cfg atomic.Pointer[Config]

func handler(w http.ResponseWriter, r *http.Request) {
	c := cfg.Load() // ONE load = one consistent snapshot for this request
	fmt.Fprintf(w, "v%d %s (max %dMB)", c.Version, c.Greeting, c.MaxUpload)
}

func main() {
	cfg.Store(&Config{Version: 1, Greeting: "hello", MaxUpload: 10})

	call := func() string {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest("GET", "/", nil))
		return rec.Body.String()
	}

	// Traffic before the reload.
	var wg sync.WaitGroup
	before := make([]string, 3)
	for i := range before {
		wg.Add(1)
		go func() { defer wg.Done(); before[i] = call() }()
	}
	wg.Wait()
	fmt.Println("before reload:", before[0], before[1], before[2])

	// Hot reload: build a WHOLE new Config and swap the pointer. Never
	// mutate the stored one — readers are holding it right now.
	cfg.Store(&Config{Version: 2, Greeting: "hi there", MaxUpload: 50})
	fmt.Println("config reloaded")

	after := make([]string, 3)
	for i := range after {
		wg.Add(1)
		go func() { defer wg.Done(); after[i] = call() }()
	}
	wg.Wait()
	fmt.Println("after reload: ", after[0], after[1], after[2])
}
```

**Output:**

```
before reload: v1 hello (max 10MB) v1 hello (max 10MB) v1 hello (max 10MB)
config reloaded
after reload:  v2 hi there (max 50MB) v2 hi there (max 50MB) v2 hi there (max 50MB)
```

---

## 14. The `r.Context()` trap: background work that dies with the response

`🟡 medium` · *Request context*

**The most common context bug in Go servers.** `net/http` cancels `r.Context()` the moment the handler returns — so `go doWork(r.Context())` is killed the instant you write the response, silently, and usually only under load where you never notice. `context.WithoutCancel` (Go 1.21+) keeps the request's **values** while dropping its **cancellation**.

**Steps:**

1. Use a **real** `httptest.NewServer` — with a bare recorder the context is never cancelled and the bug does not reproduce.
2. In the handler start the same background job twice: once with `r.Context()` and once with `context.WithoutCancel(r.Context())`.
3. Respond `202 Accepted` and return, so the request context is cancelled while both jobs are still sleeping.
4. Read both outcomes: the first was killed, the second completed.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

// THE TRAP: net/http cancels r.Context() the moment the handler returns.
// Background work that borrowed that context dies with it — silently.
func main() {
	broken := make(chan string, 1)
	fixed := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WRONG: this goroutine outlives the request but borrows its context.
		go sendEmail(r.Context(), broken)

		// RIGHT: WithoutCancel (Go 1.21+) keeps the request's VALUES —
		// request id, trace span, logger — but drops its cancellation.
		go sendEmail(context.WithoutCancel(r.Context()), fixed)

		w.WriteHeader(http.StatusAccepted) // 202: "accepted, still working"
		fmt.Fprint(w, "queued")
	}))
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		panic(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	fmt.Printf("response: %d %s\n", res.StatusCode, body)

	// The handler has returned, so r.Context() is already cancelled.
	fmt.Println("with r.Context():        ", <-broken)
	fmt.Println("with WithoutCancel(ctx): ", <-fixed)
}

// sendEmail is slow background work that must survive the response.
func sendEmail(ctx context.Context, out chan<- string) {
	select {
	case <-time.After(150 * time.Millisecond):
		out <- "email sent"
	case <-ctx.Done():
		out <- "KILLED before sending: " + ctx.Err().Error()
	}
}
```

**Output:**

```
response: 202 queued
with r.Context():         KILLED before sending: context canceled
with WithoutCancel(ctx):  email sent
```

> `WithoutCancel` is the *minimum* fix. Work that must not be lost belongs in a durable queue ([44](../../44-background-jobs-queues.md)) — a goroutine dies with the process, and deploys kill processes.

---

## 15. Idempotency: the double-clicked "Pay" button

`🟡 medium` · *Idempotency*

The user double-clicks, the browser retries, the mobile app resends on a flaky network — two requests, two goroutines, the same idempotency key, and the card must be charged **exactly once**. The fix is a check-and-insert in one critical section: checking first and locking later is check-then-act, and both goroutines would see "missing".

**Steps:**

1. Define `Payments` with `mu`, `results map[string]string` (key → charge id), and an `atomic.Int64` counting real charges.
2. In `Charge`, take the lock, look the key up **inside** it, and return the stored id on a replay.
3. Otherwise perform the side effect once, record it under the key, and return.
4. Fire 5 concurrent requests with the same `Idempotency-Key`: identical responses, one charge. A different key is a different payment.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
)

// The user double-clicks "Pay". Two requests, two goroutines, same
// idempotency key — and the card must be charged exactly once.
type Payments struct {
	mu      sync.Mutex
	results map[string]string // idempotency key -> charge id
	charges atomic.Int64      // how many times we actually hit the card
}

func NewPayments() *Payments {
	return &Payments{results: make(map[string]string)}
}

func (p *Payments) Charge(key string, amount int) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check INSIDE the lock. Checking first and locking later is the
	// classic check-then-act race: both goroutines would see "missing".
	if id, ok := p.results[key]; ok {
		return id // replay: hand back the original result
	}

	id := fmt.Sprintf("charge-%d", p.charges.Add(1)) // the real side effect
	p.results[key] = id
	return id
}

func main() {
	pay := NewPayments()

	h := func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		fmt.Fprint(w, pay.Charge(key, 4200))
	}

	// 5 concurrent clicks, one key.
	ids := make([]string, 5)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/pay", nil)
			req.Header.Set("Idempotency-Key", "key-abc")
			rec := httptest.NewRecorder()
			h(rec, req)
			ids[i] = rec.Body.String()
		}()
	}
	wg.Wait()

	same := true
	for _, id := range ids {
		if id != ids[0] {
			same = false
		}
	}
	fmt.Println("all 5 responses identical:", same, "->", ids[0])
	fmt.Println("times the card was charged:", pay.charges.Load())

	// A different key is a different payment.
	req := httptest.NewRequest("POST", "/pay", nil)
	req.Header.Set("Idempotency-Key", "key-xyz")
	rec := httptest.NewRecorder()
	h(rec, req)
	fmt.Println("new key ->", rec.Body.String(), "| total charges:", pay.charges.Load())
}
```

**Output:**

```
all 5 responses identical: true -> charge-1
times the card was charged: 1
new key -> charge-2 | total charges: 2
```

> Holding a mutex across a real payment API call would serialize your whole server. In production the map is a table with a **unique index** on the key, and the uniqueness violation is what tells you it's a replay ([41](../../41-api-design-evolution.md), [35](../../35-sagas-distributed-transactions.md)).

---

## 16. Lock striping: don't let alice block bob

`🟡 medium` · *Contention*

One global lock is correct but serializes your entire application — alice's slow write blocks bob, who has nothing to do with her data. **Lock striping** gives each user their own mutex: different users never contend, while two requests from the *same* user still serialize, which is exactly what you want.

**Steps:**

1. Write `Global.Update` holding one mutex across 80ms of work; three different users take ≥240ms.
2. Write `Striped` with `locks map[string]*sync.Mutex`; `lockFor(user)` holds the outer lock only long enough to look up (or create) a pointer.
3. `Update` locks **that user's** mutex for the slow work, then a separate small lock for the data write.
4. Time both, plus a same-user run to show it still serializes correctly.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

const work = 80 * time.Millisecond

// Global: ONE lock for every user. Alice's slow write blocks Bob, who has
// nothing to do with her data. Correct, but it serializes your whole app.
type Global struct {
	mu   sync.Mutex
	data map[string]int
}

func (g *Global) Update(user string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	time.Sleep(work) // slow work INSIDE the one global lock
	g.data[user]++
}

// Striped: one lock PER USER. Two users never contend; two requests from
// the same user still serialize, which is exactly what we want.
type Striped struct {
	mu    sync.Mutex             // guards the locks map only
	locks map[string]*sync.Mutex // per-user locks
	data  map[string]int
	dataM sync.Mutex
}

// lockFor hands back this user's mutex, creating it on first use. The
// outer lock is held only long enough to look up a pointer.
func (s *Striped) lockFor(user string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.locks[user]; !ok {
		s.locks[user] = &sync.Mutex{}
	}
	return s.locks[user]
}

func (s *Striped) Update(user string) {
	l := s.lockFor(user)
	l.Lock() // only requests for THIS user wait here
	defer l.Unlock()

	time.Sleep(work)
	s.dataM.Lock()
	s.data[user]++
	s.dataM.Unlock()
}

func run(update func(string), users []string) time.Duration {
	start := time.Now()
	var wg sync.WaitGroup
	for _, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			update(u)
		}()
	}
	wg.Wait()
	return time.Since(start)
}

func main() {
	users := []string{"alice", "bob", "carol"} // three DIFFERENT users

	g := &Global{data: map[string]int{}}
	gd := run(g.Update, users)

	s := &Striped{locks: map[string]*sync.Mutex{}, data: map[string]int{}}
	sd := run(s.Update, users)

	// Global: 3 × 80ms queued up. Striped: all three overlap at ~80ms.
	fmt.Println("one global lock — serialized: ", gd >= 230*time.Millisecond)
	fmt.Println("lock per user   — parallel:   ", sd < 150*time.Millisecond)

	// Same user twice still serializes, which is the point of the lock.
	sd2 := run(s.Update, []string{"alice", "alice"})
	fmt.Println("same user twice — serialized: ", sd2 >= 150*time.Millisecond)
}
```

**Output:**

```
one global lock — serialized:  true
lock per user   — parallel:    true
same user twice — serialized:  true
```

> ⚠️ Striping is an **optimization with a cost**: the `locks` map grows forever unless you evict, and multi-user operations (transfer money A→B) now need two locks — which means **lock ordering** or you deadlock. Start with one mutex; stripe when a profile proves contention.

---

## 17. Fan-out: alice posts, everyone sees it

`🟡 medium` · *Interaction*

User-to-user interaction in its simplest form: each subscriber owns a **buffered** channel, and the publisher sends **non-blockingly**. That `default:` branch is what stops one slow reader from freezing the publisher — and therefore every other user. Note the dropped counter must be atomic: `RLock` admits many concurrent publishers.

**Steps:**

1. Define `Broker` with `subs map[string]chan string` under an `RWMutex`, plus `dropped atomic.Int64`.
2. `Subscribe` takes the write lock and returns a receive-only buffered channel.
3. `Publish` takes the read lock and fans out with `select { case ch <- msg: default: … }` — full buffer means drop, never block.
4. Give one subscriber a buffer of 1 and never read it; publish 3 messages and watch it lose 2 while the healthy subscribers get all 3.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Broker is user-to-user interaction: alice posts, everyone subscribed
// gets it. Each subscriber owns a BUFFERED channel so one slow reader
// cannot stall the publisher.
type Broker struct {
	mu   sync.RWMutex
	subs map[string]chan string
	// RLock lets MANY goroutines run Publish at once, so this counter
	// must be atomic — a plain int++ here would be a data race even
	// though we are "holding a lock".
	dropped atomic.Int64
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[string]chan string)}
}

func (b *Broker) Subscribe(user string, buffer int) <-chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, buffer)
	b.subs[user] = ch
	return ch
}

// Publish fans out to every subscriber. The non-blocking send is the
// critical part: a subscriber whose buffer is full gets its message
// DROPPED rather than freezing the publisher and everybody else.
func (b *Broker) Publish(msg string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for user, ch := range b.subs {
		select {
		case ch <- msg:
		default:
			b.dropped.Add(1)
			fmt.Printf("  (dropped for %s: buffer full)\n", user)
		}
	}
}

func main() {
	b := NewBroker()

	alice := b.Subscribe("alice", 4)
	bob := b.Subscribe("bob", 4)
	slow := b.Subscribe("slow", 1) // tiny buffer, never reads

	b.Publish("alice posted a photo")
	b.Publish("bob commented")
	b.Publish("carol joined") // "slow" is full by now

	// Give the prints above a moment, then drain the healthy subscribers.
	time.Sleep(10 * time.Millisecond)

	drain := func(name string, ch <-chan string) {
		var got []string
		for {
			select {
			case m := <-ch:
				got = append(got, m)
				continue
			default:
			}
			break
		}
		sort.Strings(got)
		fmt.Printf("%-5s received %d: %v\n", name, len(got), got)
	}

	drain("alice", alice)
	drain("bob", bob)
	drain("slow", slow)
	fmt.Println("total dropped:", b.dropped.Load())
}
```

**Output:**

```
  (dropped for slow: buffer full)
  (dropped for slow: buffer full)
alice received 3: [alice posted a photo bob commented carol joined]
bob   received 3: [alice posted a photo bob commented carol joined]
slow  received 1: [alice posted a photo]
total dropped: 2
```

---

> ← Back to the [index](README.md) · Previous tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
