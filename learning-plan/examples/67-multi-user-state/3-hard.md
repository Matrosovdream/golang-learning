# Step 67 — Multi-User State in One Process · 🔴 Hard — examples **18–24**

Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .        # these are all concurrent — run go run -race . too
```

> ← Back to the [index](README.md) · Previous tier: [🟡 medium](2-medium.md)

---

## 18. The hub: a single owner and no mutex anywhere

`🔴 hard` · *Actor pattern*

The alternative to locking is **ownership**: one goroutine owns the client map, and everyone else asks it to do things by sending messages. Search this program for `sync` — there isn't one. This is the design behind every WebSocket hub in Go ([58](../../58-realtime-websockets-sse.md)), and it's lesson 15's "channels transfer ownership" scaled up to a real component.

**Steps:**

1. Define `Hub` with channels for `register`, `unregister`, `broadcast`, `query`, and `quit`.
2. Write `run()`: declare `clients := make(map[*Client]bool)` **inside the goroutine** — since only this goroutine touches it, no lock can ever be needed.
3. Broadcast with a non-blocking send so a slow client can't stall the hub; on unregister, `close(c.recv)` to tell that client no more messages are coming.
4. Implement request/response with `query chan chan []string`: the caller sends a **reply channel** and blocks on it, which is how you read owned state from outside.
5. In `main`, register two clients (unbuffered sends, so they return only once the hub has processed them), broadcast, unregister bob, and drain both channels — bob's `range` ends because the hub closed it.

```go
package main

import (
	"fmt"
	"sort"
)

// The hub is the actor pattern: ONE goroutine owns the clients map, so
// there is no mutex anywhere in this file. Everyone else asks the hub to
// do things by sending it a message.
type Client struct {
	name string
	recv chan string // buffered: the hub must never block on a slow client
}

type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan string
	query      chan chan []string // "send me the roster on this channel"
	quit       chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan string),
		query:      make(chan chan []string),
		quit:       make(chan struct{}),
	}
}

// run is the single owner. clients is a plain map with NO lock, because
// only this goroutine ever touches it.
func (h *Hub) run() {
	clients := make(map[*Client]bool)
	for {
		select {
		case c := <-h.register:
			clients[c] = true
			fmt.Println("hub:", c.name, "joined")

		case c := <-h.unregister:
			if clients[c] {
				delete(clients, c)
				close(c.recv) // tell the client no more messages are coming
				fmt.Println("hub:", c.name, "left")
			}

		case msg := <-h.broadcast:
			for c := range clients {
				select {
				case c.recv <- msg:
				default: // slow client: drop rather than stall the hub
				}
			}

		case reply := <-h.query:
			names := make([]string, 0, len(clients))
			for c := range clients {
				names = append(names, c.name)
			}
			sort.Strings(names)
			reply <- names

		case <-h.quit:
			for c := range clients {
				close(c.recv)
			}
			return
		}
	}
}

// Roster is a request/response over channels: send a reply channel,
// block until the owner answers.
func (h *Hub) Roster() []string {
	reply := make(chan []string)
	h.query <- reply
	return <-reply
}

func main() {
	hub := NewHub()
	go hub.run()

	alice := &Client{name: "alice", recv: make(chan string, 8)}
	bob := &Client{name: "bob", recv: make(chan string, 8)}
	hub.register <- alice // unbuffered: returns once the hub has registered
	hub.register <- bob

	fmt.Println("online:", hub.Roster())

	hub.broadcast <- "welcome!"
	hub.broadcast <- "standup in 5"

	hub.unregister <- bob
	hub.broadcast <- "bob missed this one"

	fmt.Println("online:", hub.Roster())

	close(hub.quit)

	// alice's channel is closed by the hub, so range terminates on its own.
	fmt.Print("alice received: ")
	for msg := range alice.recv {
		fmt.Printf("%q ", msg)
	}
	fmt.Println()

	n := 0
	for range bob.recv {
		n++
	}
	fmt.Println("bob received:", n, "(closed on unregister)")
}
```

**Output:**

```
hub: alice joined
hub: bob joined
online: [alice bob]
hub: bob left
online: [alice]
alice received: "welcome!" "standup in 5" "bob missed this one" 
bob received: 2 (closed on unregister)
```

> **Hub vs mutex:** the hub serializes *everything* through one goroutine, so it's the wrong choice for a hot read path (use `RWMutex` or `atomic.Pointer`). It's the right choice when state changes are events — join, leave, message — and you want them ordered and lock-free.

---

## 19. Graceful shutdown: finish what you started

`🔴 hard` · *Lifecycle*

`srv.Shutdown(ctx)` closes the listener immediately, then **blocks until every in-flight request has finished** — or until `ctx` expires. Without it, a deploy kills live requests mid-response: half-written JSON, uncommitted work, angry users.

**Steps:**

1. Build an `http.Server` over an explicit `net.Listen("tcp", "127.0.0.1:0")` (`:0` = any free port) and `go srv.Serve(ln)`.
2. Fire a request against a 200ms handler and let it get mid-flight (sleep 50ms).
3. Call `srv.Shutdown(ctx)` in a goroutine with a 2s budget, reporting its error.
4. Try a **new** request during shutdown — the listener is already closed, so it's refused.
5. Confirm the in-flight request still completed normally and `Shutdown` returned `nil`.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Graceful shutdown: stop accepting new connections, let in-flight
// requests finish, then exit. Without it, a deploy kills live requests.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // a request already in progress
		fmt.Fprint(w, "slow response completed")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0") // :0 = any free port
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	url := "http://" + ln.Addr().String()

	// Fire a request, then start shutting down while it is still running.
	type result struct {
		body string
		err  error
	}
	inflight := make(chan result, 1)
	go func() {
		res, err := http.Get(url + "/slow")
		if err != nil {
			inflight <- result{err: err}
			return
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		inflight <- result{body: string(b)}
	}()

	time.Sleep(50 * time.Millisecond) // the request is now mid-flight

	shutdownDone := make(chan error, 1)
	go func() {
		// Shutdown stops the listener immediately, then BLOCKS until every
		// active request has finished — or until ctx expires.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownDone <- srv.Shutdown(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // shutdown has begun

	// New connections are refused the moment Shutdown starts.
	if _, err := http.Get(url + "/slow"); err != nil {
		fmt.Println("new request during shutdown: refused")
	} else {
		fmt.Println("new request during shutdown: accepted (unexpected)")
	}

	r := <-inflight
	fmt.Println("in-flight request:", r.body)
	fmt.Println("shutdown error:", <-shutdownDone)
}
```

**Output:**

```
new request during shutdown: refused
in-flight request: slow response completed
shutdown error: <nil>
```

> In production: catch `SIGTERM` with `signal.NotifyContext`, call `Shutdown` with a budget shorter than your orchestrator's kill timeout, and **also** cancel your own background goroutines and `wg.Wait()` them — `Shutdown` only knows about HTTP requests.

---

## 20. Check-then-act: atomics do not save you

`🔴 hard` · *Concurrency bugs*

Ten seats, fifty users. The broken version uses **only atomic operations** and is still catastrophically wrong — it sells all 50. Atomics make each *step* indivisible; they do not make a *decision followed by an action* indivisible. Between `Load` and `Add`, every other goroutine can also read "seats available".

**Steps:**

1. Write `Broken` with an `atomic.Int64`: `if b.left.Load() > 0 { …; b.left.Add(-1) }`. Any real work between check and act widens the window; here a 1ms sleep stands in for it.
2. Write `Fixed` with a `sync.Mutex` held across **both** the check and the decrement.
3. Run a 50-goroutine stampede against each and print how many seats were sold.
4. Note that `-race` reports **nothing** for the broken version — it is not a data race, it is a logic race. No tool will find this for you.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	seats = 10
	users = 50
)

// BROKEN: every operation here is atomic, and it is STILL wrong.
// Atomic operations make each STEP indivisible; they do not make
// check-then-act indivisible. Between Load and Add, 40 other goroutines
// can also read "1 seat left".
type Broken struct{ left atomic.Int64 }

func (b *Broken) Book() bool {
	if b.left.Load() > 0 { // CHECK
		time.Sleep(time.Millisecond) // any real work widens this window
		b.left.Add(-1)               // ACT — on information that is now stale
		return true
	}
	return false
}

// FIXED: check and act inside ONE critical section, so no other goroutine
// can observe or change left in between.
type Fixed struct {
	mu   sync.Mutex
	left int
}

func (f *Fixed) Book() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.left > 0 {
		time.Sleep(time.Millisecond)
		f.left--
		return true
	}
	return false
}

func stampede(book func() bool) int {
	var sold atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if book() {
				sold.Add(1)
			}
		}()
	}
	wg.Wait()
	return int(sold.Load())
}

func main() {
	b := &Broken{}
	b.left.Store(seats)
	soldBroken := stampede(b.Book)

	f := &Fixed{left: seats}
	soldFixed := stampede(f.Book)

	fmt.Printf("seats available: %d, users trying: %d\n\n", seats, users)
	fmt.Printf("atomic check-then-act: sold %d  oversold by %d\n", soldBroken, soldBroken-seats)
	fmt.Printf("mutex around both:     sold %d  oversold by %d\n", soldFixed, soldFixed-seats)
	fmt.Println("\ncorrect:", soldFixed == seats)
}
```

**Output:**

```
seats available: 10, users trying: 50

atomic check-then-act: sold 50  oversold by 40
mutex around both:     sold 10  oversold by 0

correct: true
```

> The lock-free fix is a **CAS retry loop** (lesson 13): load, compute, `CompareAndSwap`, retry if it failed. Across replicas, neither works — the database has to enforce it (`UPDATE … WHERE seats > 0`, or `SELECT … FOR UPDATE`).

---

## 21. Request IDs: making 200 concurrent users readable

`🔴 hard` · *Observability*

With hundreds of users interleaved, an unattributed log line is worthless. Middleware mints a request id and puts a **logger already carrying it** into the context, so deep code just calls `Log(ctx).Info(...)` and gets correct attribution without knowing request ids exist.

**Steps:**

1. Declare `loggerKey struct{}` and a `Log(ctx)` helper that falls back to `slog.Default()`.
2. In the middleware, mint `req-N` from an `atomic.Int64`, build `slog.Default().With("request_id", id, "path", …)`, and stash it.
3. Echo the id in the `X-Request-Id` response header so a user can quote it in a bug report.
4. Have `loadUser(ctx, id)` — deep, generic code — log through `Log(ctx)` and inherit the id automatically.
5. Configure `slog` with a `ReplaceAttr` that drops the timestamp so the output is deterministic.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
)

// Every log line from a request must be attributable to THAT request —
// otherwise 200 concurrent users produce one unreadable stream.
type loggerKey struct{}

var reqCounter atomic.Int64

// Log returns the request-scoped logger, or the default one if the
// middleware did not run.
func Log(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// requestID attaches a per-request id AND a logger already carrying it,
// so no downstream code has to remember to add it.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("req-%d", reqCounter.Add(1))

		logger := slog.Default().With("request_id", id, "path", r.URL.Path)
		ctx := context.WithValue(r.Context(), loggerKey{}, logger)

		w.Header().Set("X-Request-Id", id) // let the client quote it in a bug report
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Deep code just takes ctx — it never knows about request IDs.
func loadUser(ctx context.Context, id string) {
	Log(ctx).Info("loading user", "user_id", id)
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	Log(ctx).Info("handling request")
	loadUser(ctx, r.URL.Query().Get("user"))
	fmt.Fprint(w, "ok")
}

func main() {
	// Deterministic output: no timestamps, text format, stdout.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{} // drop the timestamp
			}
			return a
		},
	})))

	h := requestID(http.HandlerFunc(handler))

	// Two requests, run one after the other so the log order is stable.
	for _, user := range []string{"alice", "bob"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/profile?user="+user, nil))
		fmt.Println("-> response header X-Request-Id:", rec.Header().Get("X-Request-Id"))
	}
}
```

**Output:**

```
level=INFO msg="handling request" request_id=req-1 path=/profile
level=INFO msg="loading user" request_id=req-1 path=/profile user_id=alice
-> response header X-Request-Id: req-1
level=INFO msg="handling request" request_id=req-2 path=/profile
level=INFO msg="loading user" request_id=req-2 path=/profile user_id=bob
-> response header X-Request-Id: req-2
```

> This is the one legitimate use of `context.WithValue`: cross-cutting metadata that every layer needs and no layer should have to pass explicitly. Across services the id travels in a header ([39](../../39-observability-tracing.md)).

---

## 22. What breaks when you run a second replica

`🔴 hard` · *Scale-out*

The hard boundary of everything in this lesson: **a mutex only coordinates goroutines inside one process.** Start a second replica and every piece of in-memory state silently diverges — sessions, rate limits, caches, presence, locks. The defense is to put the state behind an interface from day one, so swapping memory for Redis is a constructor change.

**Steps:**

1. Define a `SessionStore` interface with `Login` and `Online` — this is the seam.
2. Implement `MemoryStore` correctly, with a proper `RWMutex`. Nothing here is buggy; that's the point.
3. Give replicas A and B their **own** `MemoryStore`, log alice in on A, and read from B: she looks logged out.
4. Point both replicas at **one shared** store and repeat: consistent. In production that store is Redis or Postgres, and the interface means the server code doesn't change.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

// SessionStore is the seam. Program against the interface and the same
// server code works with in-memory state (1 replica) or a shared backend
// like Redis or Postgres (N replicas).
type SessionStore interface {
	Login(user string)
	Online() []string
}

// MemoryStore: fine for one process. Each replica gets its OWN map.
type MemoryStore struct {
	mu    sync.RWMutex
	users map[string]bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: make(map[string]bool)}
}

func (m *MemoryStore) Login(user string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user] = true
}

func (m *MemoryStore) Online() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.users))
	for u := range m.users {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// Server is one replica behind the load balancer.
type Server struct {
	name  string
	store SessionStore
}

func main() {
	fmt.Println("=== two replicas, each with its own memory ===")
	a := &Server{name: "replica-A", store: NewMemoryStore()}
	b := &Server{name: "replica-B", store: NewMemoryStore()}

	// The load balancer sends alice's login to A...
	a.store.Login("alice")
	// ...and her very next request to B.
	fmt.Println("A sees:", a.store.Online())
	fmt.Println("B sees:", b.store.Online(), "<- alice looks logged out")

	fmt.Println("\n=== two replicas, one shared store ===")
	shared := NewMemoryStore() // stand-in for Redis/Postgres
	a2 := &Server{name: "replica-A", store: shared}
	b2 := &Server{name: "replica-B", store: shared}

	a2.store.Login("alice")
	b2.store.Login("bob")
	fmt.Println("A sees:", a2.store.Online())
	fmt.Println("B sees:", b2.store.Online(), "<- consistent")

	// The lesson: a mutex only coordinates goroutines INSIDE one process.
	// The moment you run a second replica, in-memory state silently
	// diverges — sessions, rate limits, caches, locks, all of it.
	_ = a
	_ = b
}
```

**Output:**

```
=== two replicas, each with its own memory ===
A sees: [alice]
B sees: [] <- alice looks logged out

=== two replicas, one shared store ===
A sees: [alice bob]
B sees: [alice bob] <- consistent
```

> The checklist for every in-memory design: **sessions** → shared store or signed cookie; **rate limits** → Redis counters; **caches** → per-replica is fine (just N× the misses); **broadcast** → pub/sub backplane ([58](../../58-realtime-websockets-sse.md)); **locks** → the database.

---

## 23. Lost updates and optimistic locking

`🔴 hard` · *Cross-request races*

Two users open the same record, both edit, both save — and the second silently overwrites the first. **A mutex cannot help**: the reads happened in *different requests*, minutes apart. The fix is a version check on write, and telling the loser instead of losing their work.

**Steps:**

1. Build a `Repo` whose `Load` returns a copy and whose `SaveBlind` writes whatever it's handed — both perfectly locked, both complicit in the bug.
2. Have alice and bob both `Load` version 1, both edit, both `SaveBlind`: alice's edit vanishes.
3. Write `SaveChecked`, which compares the incoming `Version` to the stored one and returns `ErrConflict` if they differ, incrementing on success.
4. Replay the scenario: alice wins, bob is told to reload — which is an HTTP **409 Conflict**.

```go
package main

import (
	"errors"
	"fmt"
	"sync"
)

// Two users open the same record, both edit, both save. Without a version
// check the second save silently overwrites the first — a LOST UPDATE.
// A mutex does not help: both saves are individually well-locked.
var ErrConflict = errors.New("conflict: record changed since you loaded it")

type Doc struct {
	Title   string
	Version int
}

type Repo struct {
	mu  sync.Mutex
	doc Doc
}

func (r *Repo) Load() Doc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.doc // a copy
}

// SaveBlind is the lost-update bug: it writes whatever it is given.
func (r *Repo) SaveBlind(d Doc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doc = d
}

// SaveChecked is optimistic locking: the write only lands if nobody else
// wrote since we loaded. In SQL this is
// UPDATE docs SET ..., version = version + 1 WHERE id = ? AND version = ?
// and you check that RowsAffected == 1.
func (r *Repo) SaveChecked(d Doc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.Version != r.doc.Version {
		return ErrConflict
	}
	d.Version++
	r.doc = d
	return nil
}

func main() {
	fmt.Println("=== blind write: alice's edit disappears ===")
	r := &Repo{doc: Doc{Title: "draft", Version: 1}}

	alice := r.Load() // both load version 1
	bob := r.Load()

	alice.Title = "alice's title"
	r.SaveBlind(alice)

	bob.Title = "bob's title" // bob never saw alice's change
	r.SaveBlind(bob)

	fmt.Print("stored: ", r.Load().Title, " - alice's edit is gone\n\n")

	fmt.Println("=== optimistic locking: the loser is told ===")
	r2 := &Repo{doc: Doc{Title: "draft", Version: 1}}

	alice2 := r2.Load()
	bob2 := r2.Load()

	alice2.Title = "alice's title"
	fmt.Println("alice saves:", r2.SaveChecked(alice2))

	bob2.Title = "bob's title"
	err := r2.SaveChecked(bob2) // still holding version 1; store is at 2
	fmt.Println("bob saves:  ", err)

	fmt.Println("stored:", r2.Load().Title, "| version:", r2.Load().Version)
	fmt.Println("bob must reload and retry -> HTTP 409 Conflict")
}
```

**Output:**

```
=== blind write: alice's edit disappears ===
stored: bob's title - alice's edit is gone

=== optimistic locking: the loser is told ===
alice saves: <nil>
bob saves:   conflict: record changed since you loaded it
stored: alice's title | version: 2
bob must reload and retry -> HTTP 409 Conflict
```

> **Optimistic** (a version column, retry on conflict) suits low contention and works across replicas. **Pessimistic** (`SELECT … FOR UPDATE`) holds a DB lock for the whole transaction — correct under high contention, but it serializes and can deadlock. Expose the version to clients as an `ETag` + `If-Match` ([41](../../41-api-design-evolution.md)).

---

## 24. Capstone: a small multi-user service

`🔴 hard` · *Capstone*

Everything at once: identity middleware, per-user rate limiting, presence, fan-out, a real server under concurrent load from three users, and graceful shutdown. Note `Users` keeps the budget **and** the last-seen timestamp under one lock — related state belongs together, not split across a lock per field.

**Steps:**

1. `Users.Touch` records activity and consumes a token in **one** critical section (example 20's rule).
2. `Feed.Publish` fans out with non-blocking sends; `dropped` is atomic because `RLock` admits concurrent publishers (example 17).
3. Chain the middleware `auth(users.rateLimit(mux))` — order matters, you can't rate-limit per user before you know the user.
4. Route with Go 1.22+ method patterns (`"POST /post"`).
5. Drive 3 users × 6 requests concurrently against a budget of 4, tally the codes, check the unauthenticated path never reaches the limiter, then `srv.Shutdown(ctx)`.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---------- identity ----------

type userKey struct{}

func userFrom(ctx context.Context) string {
	u, _ := ctx.Value(userKey{}).(string)
	return u
}

// ---------- per-user state ----------

// Users holds everything we track per user behind ONE lock: the request
// budget and the last-seen timestamp. Keeping related state together
// under one lock is simpler — and safer — than a lock per field.
type Users struct {
	mu    sync.Mutex
	state map[string]*userState
	quota int
}

type userState struct {
	remaining int
	lastSeen  time.Time
}

func NewUsers(quota int) *Users {
	return &Users{state: make(map[string]*userState), quota: quota}
}

// Touch records activity and consumes one token, in a single critical
// section — check-then-act must not be split (example 20).
func (u *Users) Touch(name string) (allowed bool) {
	u.mu.Lock()
	defer u.mu.Unlock()

	s, ok := u.state[name]
	if !ok {
		s = &userState{remaining: u.quota}
		u.state[name] = s
	}
	s.lastSeen = time.Now()

	if s.remaining == 0 {
		return false
	}
	s.remaining--
	return true
}

func (u *Users) Online(within time.Duration) []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	cutoff := time.Now().Add(-within)
	out := make([]string, 0, len(u.state))
	for name, s := range u.state {
		if s.lastSeen.After(cutoff) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// ---------- the feed (user-to-user interaction) ----------

type Feed struct {
	mu      sync.RWMutex
	subs    map[string]chan string
	dropped atomic.Int64 // RLock allows concurrent Publish: must be atomic
}

func NewFeed() *Feed { return &Feed{subs: make(map[string]chan string)} }

func (f *Feed) Subscribe(name string) <-chan string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan string, 32)
	f.subs[name] = ch
	return ch
}

func (f *Feed) Publish(msg string) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, ch := range f.subs {
		select {
		case ch <- msg: // non-blocking: one slow reader can't stall the rest
		default:
			f.dropped.Add(1)
		}
	}
}

// ---------- middleware ----------

func auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("X-User")
		if name == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, name)))
	})
}

func (u *Users) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !u.Touch(userFrom(r.Context())) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	users := NewUsers(4) // 4 requests per user
	feed := NewFeed()
	var served atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("POST /post", func(w http.ResponseWriter, r *http.Request) {
		served.Add(1)
		feed.Publish(userFrom(r.Context()) + " posted")
		w.WriteHeader(http.StatusCreated)
	})

	// Middleware order matters: identity must exist before we can rate
	// limit per user.
	handler := auth(users.rateLimit(mux))

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	url := "http://" + ln.Addr().String()

	feed.Subscribe("dashboard")

	// 3 users × 6 requests, all concurrent. Budget is 4 each.
	names := []string{"alice", "bob", "carol"}
	var mu sync.Mutex
	codes := map[string]map[int]int{}

	var wg sync.WaitGroup
	for _, name := range names {
		codes[name] = map[int]int{}
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest("POST", url+"/post", nil)
				req.Header.Set("X-User", name)
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					return
				}
				io.Copy(io.Discard, res.Body)
				res.Body.Close()

				mu.Lock()
				codes[name][res.StatusCode]++
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	for _, name := range names {
		fmt.Printf("%-6s created=%d rate-limited=%d\n",
			name, codes[name][http.StatusCreated], codes[name][http.StatusTooManyRequests])
	}

	// Unauthenticated request never reaches the rate limiter.
	res, _ := http.Post(url+"/post", "", nil)
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	fmt.Println("no credential ->", res.StatusCode)

	fmt.Println("online:", users.Online(time.Second))
	fmt.Println("requests served:", served.Load())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fmt.Println("graceful shutdown:", srv.Shutdown(ctx))
}
```

**Output:**

```
alice  created=4 rate-limited=2
bob    created=4 rate-limited=2
carol  created=4 rate-limited=2
no credential -> 401
online: [alice bob carol]
requests served: 12
graceful shutdown: <nil>
```

> Run this one with `go run -race .` and let it sink in: three users, 18 concurrent requests, shared maps everywhere, and no races — because every piece of shared state has exactly one owner and one lock.

---

> ← Back to the [index](README.md) · Previous tier: [🟡 medium](2-medium.md)
