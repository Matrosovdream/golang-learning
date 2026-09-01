# Step 68 — Rate Limiting & Throttling · 🟢 Easy

Examples **1–8**. Each is a complete, runnable `package main` program in the shape you would
actually ship: injected clocks, real middleware, real `http.Client` transports. Read the concept
and steps, then **retype the code block** and run it.

**Run any example:**

```bash
mkdir -p /tmp/rl-ex && cd /tmp/rl-ex   # once: go mod init scratch && go get golang.org/x/time@latest
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Pace outbound calls with a ticker

`🟢 easy` · *Pacing*

The smallest limiter that does real work: one tick, one call. Good enough for pacing yourself against a third-party budget, and instructive about what it *cannot* do — no burst, no cancellation, no per-key.

**Steps:**

1. `time.NewTicker(every)` delivers on a channel at a fixed interval.
2. `Take()` blocks on `<-t.C`, so callers are paced without any counting.
3. `defer p.Stop()` — a ticker that is never stopped leaks its runtime timer.

```go
package main

import (
	"fmt"
	"time"
)

// Pacer is the smallest useful limiter: one tick, one call.
// Used to pace OUTBOUND work against a third-party API budget.
type Pacer struct{ t *time.Ticker }

func NewPacer(every time.Duration) *Pacer { return &Pacer{t: time.NewTicker(every)} }
func (p *Pacer) Take()                    { <-p.t.C } // blocks until the next tick
func (p *Pacer) Stop()                    { p.t.Stop() }

func callAPI(int) {}

func main() {
	p := NewPacer(50 * time.Millisecond) // 20 requests/sec
	defer p.Stop()

	start := time.Now()
	for i := 1; i <= 5; i++ {
		p.Take()
		callAPI(i)
		fmt.Printf("call %d at %v\n", i, time.Since(start).Round(10*time.Millisecond))
	}
}
```

**Output:**

```
call 1 at 50ms
call 2 at 100ms
call 3 at 150ms
call 4 at 200ms
call 5 at 250ms
```

---

## 2. Fixed window — and its 2× boundary burst

`🟢 easy` · *Algorithms*

The cheapest algorithm: one counter per key, reset on the calendar boundary. This example *measures* its famous flaw rather than asserting it — and injects the clock, so the test runs instantly instead of waiting a minute.

**Steps:**

1. `Truncate(window)` identifies which calendar window `now` falls in.
2. When the window changes, the counter resets to zero.
3. A fake clock jumps to 10:00:59.5 and then to 10:01:00.1 — two windows, 600ms apart.
4. 200 requests are admitted inside 600ms under a "100/min" limit.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// FixedWindow: `limit` events per calendar window. One counter per key.
// The clock is injected so tests never sleep.
type FixedWindow struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	count    int
	winStart time.Time
	now      func() time.Time
}

func NewFixedWindow(limit int, window time.Duration, now func() time.Time) *FixedWindow {
	return &FixedWindow{limit: limit, window: window, now: now}
}

func (f *FixedWindow) Allow() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.now().Truncate(f.window) // which calendar window are we in?
	if !w.Equal(f.winStart) {
		f.winStart, f.count = w, 0 // new window: counter resets
	}
	if f.count < f.limit {
		f.count++
		return true
	}
	return false
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func burst(f *FixedWindow, n int) int {
	allowed := 0
	for i := 0; i < n; i++ {
		if f.Allow() {
			allowed++
		}
	}
	return allowed
}

func main() {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: base}
	f := NewFixedWindow(100, time.Minute, clk.Now) // "100 per minute"

	clk.Advance(59500 * time.Millisecond) // 10:00:59.5 — end of window 1
	a := burst(f, 100)
	fmt.Printf("at %s: allowed %d\n", clk.t.Format("15:04:05.000"), a)

	clk.Advance(600 * time.Millisecond) // 10:01:00.1 — window 2 begins
	b := burst(f, 100)
	fmt.Printf("at %s: allowed %d\n", clk.t.Format("15:04:05.000"), b)

	fmt.Printf("\n%d requests allowed within 600ms under a 100/min limit\n", a+b)
	fmt.Println("-> fixed windows permit a 2x burst across the boundary")
}
```

**Output:**

```
at 10:00:59.500: allowed 100
at 10:01:00.100: allowed 100

200 requests allowed within 600ms under a 100/min limit
-> fixed windows permit a 2x burst across the boundary
```

---

## 3. Allow() — the inbound shape

`🟢 easy` · *x/time/rate*

`golang.org/x/time/rate` is the standard Go limiter and a token bucket. For inbound requests you want `Allow()`: decide immediately, reject with 429. Blocking an inbound request on a limiter converts a rate problem into a latency problem.

**Steps:**

1. `rate.Every(d)` converts a period into a rate; the second argument is **burst**.
2. `Allow()` takes a token if one is free and returns false otherwise — never blocks.
3. `Tokens()` exposes the current balance, useful for a `RateLimit-Remaining` header.
4. After idling, tokens refill at the configured rate, capped at burst.

```go
package main

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// The inbound shape: check, then reject. Never block an inbound request on a
// limiter -- that turns a rate problem into a latency problem.
type api struct{ lim *rate.Limiter }

func (a *api) handle() string {
	if !a.lim.Allow() {
		return "429 Too Many Requests"
	}
	return "200 OK"
}

func main() {
	a := &api{lim: rate.NewLimiter(rate.Every(100*time.Millisecond), 3)} // 10/s, burst 3

	fmt.Println("-- 6 requests back to back --")
	for i := 1; i <= 6; i++ {
		fmt.Printf("req %d: %-21s tokens left=%.2f\n", i, a.handle(), a.lim.Tokens())
	}

	fmt.Println("\n-- after 250ms idle: refilled ~2.5 tokens (capped at burst=3) --")
	time.Sleep(250 * time.Millisecond)
	for i := 7; i <= 9; i++ {
		fmt.Printf("req %d: %s\n", i, a.handle())
	}
}
```

**Output:**

```
-- 6 requests back to back --
req 1: 200 OK                tokens left=2.00
req 2: 200 OK                tokens left=1.00
req 3: 200 OK                tokens left=0.00
req 4: 429 Too Many Requests tokens left=0.00
req 5: 429 Too Many Requests tokens left=0.00
req 6: 429 Too Many Requests tokens left=0.00

-- after 250ms idle: refilled ~2.5 tokens (capped at burst=3) --
req 7: 200 OK
req 8: 200 OK
req 9: 429 Too Many Requests
```

---

## 4. Wait(ctx) — the outbound shape, and its error trap

`🟢 easy` · *x/time/rate*

For calls *you* make, blocking is correct: pace yourself rather than fail. `Wait(ctx)` parks the goroutine until a token frees up. The trap is its error contract — one path returns `ctx.Err()`, the other does not.

**Steps:**

1. `Wait(ctx)` blocks until a token is available or the context dies.
2. If the context is **already** dead, it returns `ctx.Err()` — `errors.Is` matches.
3. If a token *would* arrive after the deadline, it returns a plain `fmt.Errorf` — **`errors.Is` does not match**.
4. So never branch on `errors.Is(err, context.DeadlineExceeded)` alone after `Wait`; treat any non-nil error as "shed".

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// The outbound shape: block until the budget allows, but never forever.
type client struct{ lim *rate.Limiter }

func (c *client) fetch(ctx context.Context, path string) error {
	if err := c.lim.Wait(ctx); err != nil { // parks until a token or ctx dies
		return fmt.Errorf("fetch %s: %w", path, err)
	}
	return nil // the real HTTP call would go here
}

func classify(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.DeadlineExceeded):
		return "errors.Is -> DeadlineExceeded"
	case errors.Is(err, context.Canceled):
		return "errors.Is -> Canceled"
	default:
		return "UNMATCHED by errors.Is"
	}
}

func main() {
	c := &client{lim: rate.NewLimiter(rate.Every(100*time.Millisecond), 1)} // 10/s, no burst

	// Case A: deadline runs out while we are waiting for tokens.
	ctxA, cancelA := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelA()
	start := time.Now()
	for i := 1; i <= 4; i++ {
		err := c.fetch(ctxA, fmt.Sprintf("/orders/%d", i))
		fmt.Printf("call %d @%-6v %-30s %v\n",
			i, time.Since(start).Round(50*time.Millisecond), classify(err), err)
	}

	// Case B: the context is ALREADY dead before Wait is called.
	ctxB, cancelB := context.WithCancel(context.Background())
	cancelB()
	err := c.fetch(ctxB, "/orders/5")
	fmt.Printf("\npre-cancelled: %-30s %v\n", classify(err), err)
}
```

**Output:**

```
call 1 @0s     ok                             <nil>
call 2 @100ms  ok                             <nil>
call 3 @200ms  ok                             <nil>
call 4 @200ms  UNMATCHED by errors.Is         fetch /orders/4: rate: Wait(n=1) would exceed context deadline

pre-cancelled: errors.Is -> Canceled          fetch /orders/5: context canceled
```

---

## 5. Reserve(): compute Retry-After, and always Cancel()

`🟢 easy` · *x/time/rate*

`Reserve()` is the one that gives you a number to put in a header. It tells you how long until a token frees up, so you can absorb the delay or shed with an accurate `Retry-After`. It also **holds** the token — forgetting `Cancel()` is a silent budget leak, measured here.

**Steps:**

1. `r.Delay()` is the wait until this reservation's token is free.
2. Within budget: sleep the delay and serve. Over budget: `r.Cancel()` and return 429.
3. `int(math.Ceil(d.Seconds()))` is your `Retry-After` value in seconds.
4. Six requests arrive in the same instant — the case a sequential loop with sleeps cannot reproduce.
5. The correct gateway ends at −1.00 tokens (3 served); the buggy one at −4.00 (all 6 charged).

```go
package main

import (
	"fmt"
	"math"
	"time"

	"golang.org/x/time/rate"
)

// Reserve() tells you HOW LONG until a token frees up, so you can decide:
// absorb the delay, or shed now and tell the client when to return.
// An unused reservation MUST be cancelled or you drain budget silently.
type gateway struct {
	lim     *rate.Limiter
	maxWait time.Duration // how long we will stall a request before shedding
}

type verdict struct {
	status     int
	retryAfter int           // seconds, for the Retry-After header
	stall      time.Duration // how long this request will be held
}

func (g *gateway) admit() verdict {
	r := g.lim.Reserve()
	if !r.OK() { // impossible under any delay: n exceeds burst
		return verdict{status: 429, retryAfter: 1}
	}
	d := r.Delay()
	if d > g.maxWait {
		r.Cancel() // hand the token back
		return verdict{status: 429, retryAfter: int(math.Ceil(d.Seconds()))}
	}
	return verdict{status: 200, stall: d} // caller sleeps d, then serves
}

// The same gateway with the Cancel() call omitted -- the classic bug.
func (g *gateway) admitLeaky() verdict {
	r := g.lim.Reserve()
	if !r.OK() {
		return verdict{status: 429, retryAfter: 1}
	}
	d := r.Delay()
	if d > g.maxWait {
		// BUG: no r.Cancel() -- we shed the request but kept the token.
		return verdict{status: 429, retryAfter: int(math.Ceil(d.Seconds()))}
	}
	return verdict{status: 200, stall: d}
}

func storm(admit func() verdict, n int) {
	for i := 1; i <= n; i++ {
		v := admit()
		if v.status == 200 {
			fmt.Printf("  req %d -> 200 (stall %v then serve)\n", i, v.stall.Round(10*time.Millisecond))
		} else {
			fmt.Printf("  req %d -> 429 Retry-After: %ds\n", i, v.retryAfter)
		}
	}
}

func main() {
	g := &gateway{
		lim:     rate.NewLimiter(rate.Every(200*time.Millisecond), 2), // 5/s, burst 2
		maxWait: 250 * time.Millisecond,
	}

	// Six requests arriving in the same instant -- the case a sequential
	// loop with sleeps can never reproduce.
	fmt.Println("correct (cancels unused reservations):")
	storm(g.admit, 6)
	fmt.Printf("  tokens left: %+.2f  <- only the 3 SERVED requests were charged\n", g.lim.Tokens())

	buggy := &gateway{
		lim:     rate.NewLimiter(rate.Every(200*time.Millisecond), 2),
		maxWait: 250 * time.Millisecond,
	}
	fmt.Println("\nbuggy (forgets Cancel):")
	storm(buggy.admitLeaky, 6)
	fmt.Printf("  tokens left: %+.2f  <- all 6 charged; the 3 shed ones stole budget\n", buggy.lim.Tokens())
}
```

**Output:**

```
correct (cancels unused reservations):
  req 1 -> 200 (stall 0s then serve)
  req 2 -> 200 (stall 0s then serve)
  req 3 -> 200 (stall 200ms then serve)
  req 4 -> 429 Retry-After: 1s
  req 5 -> 429 Retry-After: 1s
  req 6 -> 429 Retry-After: 1s
  tokens left: -1.00  <- only the 3 SERVED requests were charged

buggy (forgets Cancel):
  req 1 -> 200 (stall 0s then serve)
  req 2 -> 200 (stall 0s then serve)
  req 3 -> 200 (stall 200ms then serve)
  req 4 -> 429 Retry-After: 1s
  req 5 -> 429 Retry-After: 1s
  req 6 -> 429 Retry-After: 1s
  tokens left: -4.00  <- all 6 charged; the 3 shed ones stole budget
```

---

## 6. Rate vs burst: what NewLimiter(r, b) permits

`🟢 easy` · *x/time/rate*

Burst is not a tuning detail — it decides what a client experiences in the first millisecond. All three limiters here allow exactly 10/sec in steady state; they differ enormously in what they permit *right now*.

**Steps:**

1. Drain `Allow()` in a tight loop to measure the instantaneous burst.
2. Then count what the same limiter admits over a 500ms window.
3. `burst=1` is a strict pacer; `burst=20` lets a client spend 2 seconds of budget at once.
4. Most APIs want a small burst — roughly 5–20% of the rate.

```go
package main

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// NewLimiter(r, b): r is the STEADY rate, b is how much accumulated idleness
// a caller may spend at once. Picking b is a product decision, not a detail.
func immediateBurst(r rate.Limit, b int) int {
	lim := rate.NewLimiter(r, b)
	n := 0
	for lim.Allow() { // drain everything available right now
		n++
		if n > 1000 {
			break
		}
	}
	return n
}

func overWindow(r rate.Limit, b int, d time.Duration) int {
	lim := rate.NewLimiter(r, b)
	deadline := time.Now().Add(d)
	n := 0
	for time.Now().Before(deadline) {
		if lim.Allow() {
			n++
		}
		time.Sleep(time.Millisecond)
	}
	return n
}

func main() {
	fmt.Printf("%-28s %-16s %s\n", "limiter", "instant burst", "allowed in 500ms")
	for _, c := range []struct {
		r rate.Limit
		b int
	}{
		{10, 1},  // strict pacer
		{10, 5},  // small burst -- the usual choice
		{10, 20}, // burst > rate: a client can spend 2s of budget at once
	} {
		label := fmt.Sprintf("NewLimiter(%v, %d)", c.r, c.b)
		fmt.Printf("%-28s %-16d %d\n", label, immediateBurst(c.r, c.b), overWindow(c.r, c.b, 500*time.Millisecond))
	}
	fmt.Println("\nsteady rate is identical (10/s); only the opening burst differs")
}
```

**Output:**

```
limiter                      instant burst    allowed in 500ms
NewLimiter(10, 1)            1                5
NewLimiter(10, 5)            5                9
NewLimiter(10, 20)           20               24

steady rate is identical (10/s); only the opening burst differs
```

---

## 7. Middleware: 429, Retry-After and RateLimit-*

`🟢 easy` · *HTTP*

The middleware every Go service ends up writing, with a real server and real client in one `go run`. Note the headers go on **every** response, not just rejections, so well-behaved clients can self-pace before you have to reject them.

**Steps:**

1. Mount the limiter **before** auth lookups and body parsing so rejects cost nothing.
2. `Reserve()` + `Delay() > 0` means "no token right now" — cancel and reject.
3. `Retry-After` is the header clients actually honour; `RateLimit-*` is the IETF draft family.
4. Return **429**, never 503 — 503 says *you* are broken.

```go
package main

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// rateLimit is the middleware every Go service ends up writing.
// Order matters: mount it BEFORE auth lookups and body parsing, so rejected
// requests cost nothing.
func rateLimit(lim *rate.Limiter, perSec int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Emit the budget on EVERY response so good clients can self-pace.
		w.Header().Set("RateLimit-Limit", strconv.Itoa(perSec))

		res := lim.Reserve()
		if !res.OK() || res.Delay() > 0 {
			if res.OK() {
				res.Cancel() // we are not going to wait -- give the token back
			}
			retry := int(math.Ceil(res.Delay().Seconds()))
			if retry < 1 {
				retry = 1
			}
			w.Header().Set("RateLimit-Remaining", "0")
			w.Header().Set("RateLimit-Reset", strconv.Itoa(retry))
			w.Header().Set("Retry-After", strconv.Itoa(retry)) // the one clients honour
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		remaining := int(math.Max(0, math.Floor(lim.Tokens())))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
		next.ServeHTTP(w, r)
	})
}

func main() {
	const perSec = 5
	lim := rate.NewLimiter(rate.Every(time.Second/perSec), 2) // 5/s, burst 2

	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"ok":true}`)
	})
	srv := httptest.NewServer(rateLimit(lim, perSec, app))
	defer srv.Close()

	for i := 1; i <= 5; i++ {
		resp, err := http.Get(srv.URL + "/orders")
		if err != nil {
			fmt.Println("client error:", err)
			return
		}
		resp.Body.Close()
		fmt.Printf("req %d -> %-3d  RateLimit-Limit=%s Remaining=%s Retry-After=%q\n",
			i, resp.StatusCode,
			resp.Header.Get("RateLimit-Limit"),
			resp.Header.Get("RateLimit-Remaining"),
			resp.Header.Get("Retry-After"))
	}
}
```

**Output:**

```
req 1 -> 200  RateLimit-Limit=5 Remaining=1 Retry-After=""
req 2 -> 200  RateLimit-Limit=5 Remaining=0 Retry-After=""
req 3 -> 429  RateLimit-Limit=5 Remaining=0 Retry-After="1"
req 4 -> 429  RateLimit-Limit=5 Remaining=0 Retry-After="1"
req 5 -> 429  RateLimit-Limit=5 Remaining=0 Retry-After="1"
```

---

## 8. Pace an http.Client with a RoundTripper

`🟢 easy` · *HTTP*

The cleanest way to enforce an outbound budget: put the limiter *below* the API surface, in a `http.RoundTripper`. Every caller of that client is paced automatically — including library code you do not own and cannot modify.

**Steps:**

1. `RoundTrip` calls `lim.Wait(req.Context())` before delegating to the base transport.
2. Using the request's own context means a cancelled request stops waiting.
3. The loop fires 5 requests as fast as it can; the transport spaces them.
4. Arrival timestamps are recorded server-side — the proof is at the receiving end.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// pacedTransport wraps any RoundTripper with an outbound budget.
// Every http.Client using it is paced -- including code you do not own,
// because the limiter lives below the API surface.
type pacedTransport struct {
	base http.RoundTripper
	lim  *rate.Limiter
}

func (t *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Wait honours the request's context, so a cancelled request stops waiting.
	if err := t.lim.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func main() {
	var mu sync.Mutex
	var arrivals []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrivals = append(arrivals, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: &pacedTransport{
			base: http.DefaultTransport,
			lim:  rate.NewLimiter(rate.Every(100*time.Millisecond), 1), // 10/s, strict
		},
		Timeout: 5 * time.Second,
	}

	// Fire 5 requests as fast as the loop can: the transport paces them.
	for i := 0; i < 5; i++ {
		resp, err := client.Get(srv.URL + "/v1/things")
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		resp.Body.Close()
	}

	fmt.Println("gap between arrivals AT THE SERVER:")
	for i := 1; i < len(arrivals); i++ {
		fmt.Printf("  %d -> %d: %v\n", i, i+1, arrivals[i].Sub(arrivals[i-1]).Round(10*time.Millisecond))
	}
}
```

**Output:**

```
gap between arrivals AT THE SERVER:
  1 -> 2: 100ms
  2 -> 3: 100ms
  3 -> 4: 100ms
  4 -> 5: 100ms
```

---
