# Step 68 — Rate Limiting & Throttling · 🔴 Hard

Examples **20–26**: enforcing a limit across replicas, deciding what happens when the shared store
dies, the outbound fetcher, operability, and a full service capstone. The distributed examples run
against a `Store`/`Script` interface with an in-process fake, so **every program runs with no Redis
server** — the real Redis commands and Lua are shown alongside.

**Run any example:**

```bash
mkdir -p /tmp/rl-ex && cd /tmp/rl-ex   # once: go mod init scratch && go get golang.org/x/time@latest
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Previous tier: [🟡 medium](2-medium.md)

---

## 20. Distributed fixed window with an atomic INCR

`🔴 hard` · *Distributed*

Per-process limiters **multiply**: three replicas each allowing 100/s is 300/s. A shared counter fixes that, and the Redis recipe is two commands — `INCR` the window key, `EXPIRE` it when it is new. The `Store` interface is the seam: swap the fake for go-redis and nothing else changes.

**Steps:**

1. The key embeds the window (`rl:acme:1767…`), so expiry is automatic.
2. TTL is **2× the window** so a key outlives its own window under clock skew.
3. `INCR` is atomic, which is the entire correctness argument — no read-then-write.
4. 18 requests across 3 replicas against a shared limit of 10: exactly 10 allowed.
5. Still a fixed window, so the 2× boundary burst from example 2 is still present.

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Per-process limiters MULTIPLY: three replicas each allowing 100/s is 300/s.
// A shared counter fixes that. This is the Redis fixed-window recipe:
//
//	INCR  ratelimit:{key}:{window}
//	EXPIRE ratelimit:{key}:{window} {ttl}   (only when the counter is new)
//
// Store is the seam. Swap the fake for go-redis and nothing else changes.
type Store interface {
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// fakeRedis: an in-process stand-in so the example runs with no server.
// Its INCR is atomic under a mutex, exactly as Redis's is on one thread.
type fakeRedis struct {
	mu   sync.Mutex
	vals map[string]int64
	exp  map[string]time.Time
	now  func() time.Time
	fail error // set to simulate an outage
}

func newFakeRedis(now func() time.Time) *fakeRedis {
	return &fakeRedis{vals: map[string]int64{}, exp: map[string]time.Time{}, now: now}
}

func (f *fakeRedis) IncrWithTTL(_ context.Context, key string, ttl time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return 0, f.fail
	}
	t := f.now()
	if e, ok := f.exp[key]; ok && t.After(e) { // lazily expire
		delete(f.vals, key)
		delete(f.exp, key)
	}
	f.vals[key]++
	if f.vals[key] == 1 {
		f.exp[key] = t.Add(ttl) // set the TTL only when the key is created
	}
	return f.vals[key], nil
}

func (f *fakeRedis) keys() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.vals)
}

type DistributedFixedWindow struct {
	store  Store
	limit  int64
	window time.Duration
	now    func() time.Time
}

func (d *DistributedFixedWindow) Allow(ctx context.Context, key string) (bool, error) {
	bucket := d.now().Truncate(d.window).Unix()
	k := fmt.Sprintf("rl:%s:%d", key, bucket)
	// TTL is 2x the window so a key outlives its own window under clock skew.
	n, err := d.store.IncrWithTTL(ctx, k, 2*d.window)
	if err != nil {
		return false, err
	}
	return n <= d.limit, nil
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func main() {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	store := newFakeRedis(clk.Now)

	// Three replicas sharing ONE budget of 10/minute for tenant "acme".
	replicas := make([]*DistributedFixedWindow, 3)
	for i := range replicas {
		replicas[i] = &DistributedFixedWindow{store: store, limit: 10, window: time.Minute, now: clk.Now}
	}

	ctx := context.Background()
	allowed, denied := 0, 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 18; i++ { // 6 requests hit each replica
		wg.Go(func() {
			ok, err := replicas[i%3].Allow(ctx, "acme")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				denied++
			case ok:
				allowed++
			default:
				denied++
			}
		})
	}
	wg.Wait()
	fmt.Printf("18 requests across 3 replicas, shared limit 10/min\n")
	fmt.Printf("  allowed: %d   denied: %d   (per-process limiters would allow 18)\n", allowed, denied)
	fmt.Printf("  keys in store: %d (one per key per window)\n", store.keys())

	clk.Advance(time.Minute) // next window: a fresh key, fresh budget
	ok, _ := replicas[0].Allow(ctx, "acme")
	fmt.Printf("\nnext minute: allowed=%v (new window key, old one expires by TTL)\n", ok)

	// The boundary burst from example 2 is still here -- distribution does not fix it.
	fmt.Println("\nnote: this is still a FIXED window, so it still permits a 2x boundary burst")
}
```

**Output:**

```
18 requests across 3 replicas, shared limit 10/min
  allowed: 10   denied: 8   (per-process limiters would allow 18)
  keys in store: 1 (one per key per window)

next minute: allowed=true (new window key, old one expires by TTL)

note: this is still a FIXED window, so it still permits a 2x boundary burst
```

---

## 21. Distributed token bucket in one atomic script

`🔴 hard` · *Distributed*

A token bucket needs read-modify-write, so it must execute **atomically on the server** — otherwise two replicas racing between GET and SET both see the same balance and both allow. In Redis that is one Lua script, included verbatim. Note the integer-milliseconds detail: it is a real bug fix, not a style choice.

**Steps:**

1. The Lua script does refill, check, decrement and `EXPIRE` in one round trip.
2. The fake's mutex plays the part of Lua's atomicity, so the example needs no server.
3. **Never pass float seconds since the epoch** — at 1.7e9 a `float64` cannot represent 0.6 exactly and the refill silently comes out short.
4. Take the clock from the **server** (`redis TIME`), never from each replica's wall clock.
5. `cost` lets an expensive endpoint spend more of the same budget.

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// A distributed TOKEN bucket. The whole read-modify-write must be atomic on
// the server, or two replicas racing between GET and SET both see the same
// balance and both allow -- the classic lost update.
//
// In Redis that means one Lua script (EVAL), shown verbatim below. Here the
// same logic runs under a mutex, so the example needs no server.
const tokenBucketLua = `
-- KEYS[1]=key  ARGV[1]=rate/sec  ARGV[2]=burst  ARGV[3]=now_ms  ARGV[4]=cost
-- now_ms is an INTEGER millisecond timestamp. Never pass float seconds since
-- the epoch: at 1.7e9 a float64 cannot represent 0.6 exactly, and the refill
-- silently comes out short.
local rate, burst, now, cost = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4])
local st     = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(st[1]) or burst
local ts     = tonumber(st[2]) or now
tokens = math.min(burst, tokens + ((now - ts) / 1000) * rate)   -- lazy refill
local allowed = 0
if tokens >= cost then tokens = tokens - cost; allowed = 1 end
redis.call('HMSET', KEYS[1], 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', KEYS[1], math.ceil(burst / rate) * 2)
return {allowed, tokens}
`

type Script interface {
	Eval(ctx context.Context, key string, rate, burst float64, nowMs int64, cost float64) (allowed bool, tokens float64, err error)
}

// fakeEval runs exactly the algorithm above, atomically.
type bucketState struct {
	tokens float64
	tsMs   int64 // integer time: elapsed stays exact
}

type fakeEval struct {
	mu    sync.Mutex
	state map[string]bucketState
}

func newFakeEval() *fakeEval {
	return &fakeEval{state: map[string]bucketState{}}
}

func (f *fakeEval) Eval(_ context.Context, key string, rate, burst float64, nowMs int64, cost float64) (bool, float64, error) {
	f.mu.Lock() // <- this lock IS the Lua script's atomicity
	defer f.mu.Unlock()

	st, ok := f.state[key]
	if !ok {
		st = bucketState{tokens: burst, tsMs: nowMs}
	}
	elapsed := float64(nowMs-st.tsMs) / 1000 // small int -> exact
	st.tokens = min(burst, st.tokens+elapsed*rate)
	st.tsMs = nowMs

	allowed := false
	if st.tokens >= cost {
		st.tokens -= cost
		allowed = true
	}
	f.state[key] = st
	return allowed, st.tokens, nil
}

type DistributedTokenBucket struct {
	script Script
	rate   float64 // tokens per second
	burst  float64
	now    func() time.Time
}

func (d *DistributedTokenBucket) Allow(ctx context.Context, key string, cost float64) (bool, float64, error) {
	// Replicas must agree on "now", so in production take the time from the
	// SERVER (redis TIME), never from each replica's wall clock.
	return d.script.Eval(ctx, key, d.rate, d.burst, d.now().UnixMilli(), cost)
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func main() {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	d := &DistributedTokenBucket{script: newFakeEval(), rate: 5, burst: 5, now: clk.Now}
	ctx := context.Background()

	fmt.Println("shared bucket: 5/sec, burst 5 -- three replicas, one budget")
	for i := 1; i <= 7; i++ {
		ok, tokens, _ := d.Allow(ctx, "acme", 1)
		fmt.Printf("  replica %d req %d: allowed=%-5v tokens=%.2f\n", (i%3)+1, i, ok, tokens)
	}

	clk.Advance(600 * time.Millisecond) // 3 tokens refill
	fmt.Println("\nafter 600ms (refills 3):")
	for i := 8; i <= 10; i++ {
		ok, tokens, _ := d.Allow(ctx, "acme", 1)
		fmt.Printf("  req %d: allowed=%-5v tokens=%.2f\n", i, ok, tokens)
	}

	// Weighted cost: an expensive endpoint spends more of the same budget.
	clk.Advance(time.Second)
	ok, tokens, _ := d.Allow(ctx, "acme", 4)
	fmt.Printf("\nexpensive call (cost 4): allowed=%v tokens=%.2f\n", ok, tokens)
	ok, tokens, _ = d.Allow(ctx, "acme", 4)
	fmt.Printf("again (cost 4):          allowed=%v tokens=%.2f\n", ok, tokens)
}
```

**Output:**

```
shared bucket: 5/sec, burst 5 -- three replicas, one budget
  replica 2 req 1: allowed=true  tokens=4.00
  replica 3 req 2: allowed=true  tokens=3.00
  replica 1 req 3: allowed=true  tokens=2.00
  replica 2 req 4: allowed=true  tokens=1.00
  replica 3 req 5: allowed=true  tokens=0.00
  replica 1 req 6: allowed=false tokens=0.00
  replica 2 req 7: allowed=false tokens=0.00

after 600ms (refills 3):
  req 8: allowed=true  tokens=2.00
  req 9: allowed=true  tokens=1.00
  req 10: allowed=true  tokens=0.00

expensive call (cost 4): allowed=true tokens=1.00
again (cost 4):          allowed=false tokens=1.00
```

---

## 22. GCRA — one timestamp per key, exact Retry-After

`🔴 hard` · *Distributed*

The Generic Cell Rate Algorithm, borrowed from ATM networking and the algorithm behind redis-cell. Instead of a counter or a token balance it stores **one value per key**: the theoretical arrival time of the next permitted request. One field means one compare-and-set, which is why it distributes so well — and the wait time falls out exactly, with no estimation.

**Steps:**

1. `T` = period / limit is the spacing; `tau` = burst × T is how far ahead a caller may run.
2. An idle key resets naturally when the clock passes its stored TAT — no sweeper needed for correctness.
3. Denials return the **exact** duration until the request would be permitted.
4. 6 instant requests: 3 pass (the burst), 3 denied with a precise 100ms retry.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// GCRA -- Generic Cell Rate Algorithm, borrowed from ATM networking and the
// algorithm behind redis-cell. Instead of a count or a token balance it stores
// ONE value per key: the Theoretical Arrival Time of the next permitted request.
//
// That single value makes it ideal for a distributed store (one field, one
// compare-and-set) and it gives you an exact Retry-After for free.
//
//	T   = emission interval = period / limit   (one request every T)
//	tau = burst tolerance   = burst * T        (how far ahead you may run)
type GCRA struct {
	mu    sync.Mutex
	T     time.Duration // spacing between requests
	tau   time.Duration // burst tolerance
	state map[string]time.Time
	now   func() time.Time
}

func NewGCRA(limit int, period time.Duration, burst int, now func() time.Time) *GCRA {
	T := period / time.Duration(limit)
	return &GCRA{T: T, tau: time.Duration(burst) * T, state: map[string]time.Time{}, now: now}
}

// Allow reports whether the request may proceed, and if not, exactly how long
// the caller must wait.
func (g *GCRA) Allow(key string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	tat, ok := g.state[key]
	if !ok || tat.Before(now) {
		tat = now // idle key: the clock has caught up
	}

	newTAT := tat.Add(g.T)
	allowAt := newTAT.Add(-g.tau) // may run this far ahead of schedule
	if now.Before(allowAt) {
		return false, allowAt.Sub(now) // exact Retry-After, no estimation
	}
	g.state[key] = newTAT
	return true, 0
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func main() {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	g := NewGCRA(10, time.Second, 3, clk.Now) // 10/sec, burst 3
	fmt.Printf("T=%v (one per %v), tau=%v (may run %v ahead)\n\n", g.T, g.T, g.tau, g.tau)

	fmt.Println("-- 6 requests in the same instant --")
	for i := 1; i <= 6; i++ {
		ok, retry := g.Allow("acme")
		if ok {
			fmt.Printf("  req %d: allowed\n", i)
		} else {
			fmt.Printf("  req %d: denied, retry in %v\n", i, retry)
		}
	}

	fmt.Println("\n-- advance 100ms: exactly one more request is earned --")
	clk.Advance(100 * time.Millisecond)
	for i := 7; i <= 8; i++ {
		ok, retry := g.Allow("acme")
		fmt.Printf("  req %d: allowed=%-5v retry=%v\n", i, ok, retry)
	}

	fmt.Println("\n-- idle 5s: the key resets, full burst returns --")
	clk.Advance(5 * time.Second)
	for i := 9; i <= 12; i++ {
		ok, _ := g.Allow("acme")
		fmt.Printf("  req %d: allowed=%v\n", i, ok)
	}
	fmt.Println("\none time.Time per key -- no counters, no timestamp lists, exact Retry-After")
}
```

**Output:**

```
T=100ms (one per 100ms), tau=300ms (may run 300ms ahead)

-- 6 requests in the same instant --
  req 1: allowed
  req 2: allowed
  req 3: allowed
  req 4: denied, retry in 100ms
  req 5: denied, retry in 100ms
  req 6: denied, retry in 100ms

-- advance 100ms: exactly one more request is earned --
  req 7: allowed=true  retry=0s
  req 8: allowed=false retry=100ms

-- idle 5s: the key resets, full burst returns --
  req 9: allowed=true
  req 10: allowed=true
  req 11: allowed=true
  req 12: allowed=false

one time.Time per key -- no counters, no timestamp lists, exact Retry-After
```

---

## 23. Fail open, fail closed, or fail local

`🔴 hard` · *Failure modes*

When the shared store is unreachable you must **already** have decided what happens. There is no safe default — only a decision. And the third option is usually the right one: fall back to a conservative local limiter instead of choosing between two bad extremes.

**Steps:**

1. **fail-open** serves everything: protects availability, risks the overload you were preventing.
2. **fail-closed** rejects everything: protects the resource, guarantees an outage.
3. **fail-local** keeps limiting imprecisely with a per-process limiter.
4. For abuse prevention choose fail-closed; for capacity protection choose fail-local — and alert on the degraded path either way.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// When the shared limiter store is unreachable, you must ALREADY have decided
// what happens. There is no safe default -- only a decision:
//
//	fail OPEN  -> serve everything. Protects availability, risks overload.
//	fail CLOSED-> reject everything. Protects the resource, guarantees an outage.
//
// The third option is usually the right one: fall back to a LOCAL limiter, so
// you keep limiting (imprecisely) instead of choosing between two bad extremes.
type Policy int

const (
	FailOpen Policy = iota
	FailClosed
	FailLocal
)

func (p Policy) String() string {
	return [...]string{"fail-open", "fail-closed", "fail-local"}[p]
}

var errBackendDown = errors.New("limiter backend unreachable")

type RemoteLimiter struct {
	down bool
}

func (r *RemoteLimiter) Allow(context.Context, string) (bool, error) {
	if r.down {
		return false, errBackendDown
	}
	return true, nil
}

type Gate struct {
	remote   *RemoteLimiter
	policy   Policy
	fallback *rate.Limiter // used only by FailLocal
}

func (g *Gate) Allow(ctx context.Context, key string) (bool, string) {
	ok, err := g.remote.Allow(ctx, key)
	if err == nil {
		return ok, "remote"
	}
	switch g.policy {
	case FailOpen:
		return true, "degraded: allowed without checking"
	case FailClosed:
		return false, "degraded: rejected"
	default:
		return g.fallback.Allow(), "degraded: local limiter"
	}
}

func main() {
	fmt.Println("the shared store is DOWN -- each policy reacts differently:")
	fmt.Println()
	for _, p := range []Policy{FailOpen, FailClosed, FailLocal} {
		g := &Gate{
			remote:   &RemoteLimiter{down: true}, // outage
			policy:   p,
			fallback: rate.NewLimiter(rate.Every(time.Second), 2), // conservative local budget
		}
		fmt.Printf("%-12s during an outage, 5 requests: ", p)
		for i := 0; i < 5; i++ {
			ok, _ := g.Allow(context.Background(), "acme")
			if ok {
				fmt.Print("A")
			} else {
				fmt.Print(".")
			}
		}
		_, why := g.Allow(context.Background(), "acme")
		fmt.Printf("   (%s)\n", why)
	}

	fmt.Println("\nA = allowed, . = rejected")
	fmt.Println("fail-open serves everything (overload risk); fail-closed serves nothing")
	fmt.Println("(guaranteed outage); fail-local keeps a conservative budget running.")
	fmt.Println("\nfor ABUSE prevention choose fail-closed; for capacity protection of a")
	fmt.Println("healthy service choose fail-local, and alert on the degraded path.")
}
```

**Output:**

```
the shared store is DOWN -- each policy reacts differently:

fail-open    during an outage, 5 requests: AAAAA   (degraded: allowed without checking)
fail-closed  during an outage, 5 requests: .....   (degraded: rejected)
fail-local   during an outage, 5 requests: AA...   (degraded: local limiter)

A = allowed, . = rejected
fail-open serves everything (overload risk); fail-closed serves nothing
(guaranteed outage); fail-local keeps a conservative budget running.

for ABUSE prevention choose fail-closed; for capacity protection of a
healthy service choose fail-local, and alert on the degraded path.
```

---

## 24. A rate-limited, bounded, cancellable fetcher

`🔴 hard` · *Outbound*

The real outbound job: pull N things from a metered API as fast as its budget allows and no faster, with bounded concurrency and full cancellation. Two independent controls doing two different jobs — the limiter respects **their** quota, the semaphore protects **your** resources.

**Steps:**

1. Spend budget **before** taking a concurrency slot — waiting is cheaper without holding a slot.
2. Both the limiter wait and the slot acquire honour `ctx`, so nothing leaks on cancel.
3. Results land in indexed slots, so input order survives concurrent completion.
4. A short deadline abandons 16 of 20 cleanly — no orphaned goroutines.

```go
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// The real outbound job: fetch N things from a metered API as fast as its
// budget allows, no faster, with bounded concurrency and full cancellation.
//
// Two independent controls, doing two different jobs (example 17):
//
//	limiter   -> respects THEIR quota      (requests per second)
//	semaphore -> protects OUR resources    (sockets, memory, in-flight work)
type Fetcher struct {
	lim      *rate.Limiter
	sem      chan struct{}
	inFlight atomic.Int64
	peak     atomic.Int64
	calls    atomic.Int64
}

func NewFetcher(perSec float64, burst, maxConcurrent int) *Fetcher {
	return &Fetcher{
		lim: rate.NewLimiter(rate.Limit(perSec), burst),
		sem: make(chan struct{}, maxConcurrent),
	}
}

func (f *Fetcher) one(ctx context.Context, id int) (string, error) {
	// 1. Spend budget first -- cheap to wait here, before taking a slot.
	if err := f.lim.Wait(ctx); err != nil {
		return "", fmt.Errorf("budget: %w", err)
	}
	// 2. Then take a concurrency slot.
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	cur := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for { // record the high-water mark
		old := f.peak.Load()
		if cur <= old || f.peak.CompareAndSwap(old, cur) {
			break
		}
	}
	f.calls.Add(1)

	select { // the "HTTP call"
	case <-time.After(80 * time.Millisecond):
		return fmt.Sprintf("item-%d", id), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// FetchAll returns results in INPUT order despite concurrent completion:
// indexed slots, no lock (lesson 13 #3).
func (f *Fetcher) FetchAll(ctx context.Context, ids []int) ([]string, []error) {
	out := make([]string, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() { out[i], errs[i] = f.one(ctx, id) })
	}
	wg.Wait()
	return out, errs
}

func main() {
	ids := make([]int, 20)
	for i := range ids {
		ids[i] = i + 1
	}

	f := NewFetcher(25, 5, 4) // their quota: 25/s burst 5 -- our limit: 4 in flight
	ctx := context.Background()

	start := time.Now()
	out, errs := f.FetchAll(ctx, ids)
	elapsed := time.Since(start)

	ok := 0
	for i := range out {
		if errs[i] == nil {
			ok++
		}
	}
	fmt.Printf("%d items in %v\n", ok, elapsed.Round(50*time.Millisecond))
	fmt.Printf("  effective rate: %.0f/sec over this short run -- ABOVE the 25/sec\n", float64(f.calls.Load())/elapsed.Seconds())
	fmt.Printf("                  budget because the burst of 5 was spent up front.\n")
	fmt.Printf("                  Averaged over a long run it converges to 25/sec.\n")
	fmt.Printf("  peak in flight: %d (limit 4)\n", f.peak.Load())
	fmt.Printf("  order preserved: %s ... %s\n", out[0], out[len(out)-1])

	// Cancel mid-flight: everything unwinds, nothing leaks.
	ctx2, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	f2 := NewFetcher(25, 5, 4)
	_, errs2 := f2.FetchAll(ctx2, ids)
	failed := 0
	for _, e := range errs2 {
		if e != nil {
			failed++
		}
	}
	fmt.Printf("\nwith a 150ms deadline: %d/%d abandoned cleanly\n", failed, len(ids))
}
```

**Output:**

```
20 items in 700ms
  effective rate: 29/sec over this short run -- ABOVE the 25/sec
                  budget because the burst of 5 was spent up front.
                  Averaged over a long run it converges to 25/sec.
  peak in flight: 4 (limit 4)
  order preserved: item-1 ... item-20

with a 150ms deadline: 16/20 abandoned cleanly
```

---

## 25. Metrics: the three numbers that matter

`🔴 hard` · *Operability*

A limiter you cannot observe is one you cannot tune. Counting rejections alone cannot distinguish "one abusive client, working as designed" from "our limit is too low and every customer is suffering" — only the per-key breakdown can.

**Steps:**

1. `allowed` / `limited` answers whether the limit binds at all.
2. **Per key class** answers who is hitting it — the number teams forget to collect.
3. Wait time is the latency the limiter itself adds.
4. Alert on reject-rate rising **across many keys**, or on growing wait time — not on raw rejections.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// A limiter you cannot observe is one you cannot tune. Three numbers matter,
// and the third is the one teams forget:
//
//	allowed / limited     -> is the limit binding at all?
//	limited BY KEY CLASS  -> is one tenant hitting it, or everyone?
//	wait time             -> how much latency the limiter is adding
//
// Counting "limited" alone cannot distinguish "one abusive client, working as
// designed" from "our limit is too low and every customer is suffering".
type Metrics struct {
	allowed atomic.Int64
	limited atomic.Int64
	waitNs  atomic.Int64

	mu        sync.Mutex
	limitedBy map[string]int64 // per key: who is actually hitting the wall
}

func NewMetrics() *Metrics { return &Metrics{limitedBy: map[string]int64{}} }

func (m *Metrics) recordAllowed(wait time.Duration) {
	m.allowed.Add(1)
	m.waitNs.Add(wait.Nanoseconds())
}

func (m *Metrics) recordLimited(key string) {
	m.limited.Add(1)
	m.mu.Lock()
	m.limitedBy[key]++
	m.mu.Unlock()
}

// Report is what a /metrics endpoint or a log line would expose.
func (m *Metrics) Report() {
	a, l := m.allowed.Load(), m.limited.Load()
	total := a + l
	rejectRate := 0.0
	if total > 0 {
		rejectRate = float64(l) / float64(total) * 100
	}
	avgWait := time.Duration(0)
	if a > 0 {
		avgWait = time.Duration(m.waitNs.Load() / a)
	}
	fmt.Printf("  requests:     %d\n", total)
	fmt.Printf("  allowed:      %d\n", a)
	fmt.Printf("  limited:      %d (%.1f%%)\n", l, rejectRate)
	fmt.Printf("  avg wait:     %v (latency the limiter added)\n", avgWait.Round(time.Microsecond))

	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Println("  limited by key:")
	for _, k := range []string{"acme", "globex", "initech"} {
		if n := m.limitedBy[k]; n > 0 {
			fmt.Printf("    %-8s %d\n", k, n)
		}
	}
}

type InstrumentedLimiter struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter
	mt *Metrics
}

func (il *InstrumentedLimiter) Allow(key string) bool {
	il.mu.Lock()
	lim, ok := il.m[key]
	if !ok {
		lim = rate.NewLimiter(rate.Every(100*time.Millisecond), 3)
		il.m[key] = lim
	}
	il.mu.Unlock()

	start := time.Now()
	r := lim.Reserve()
	if !r.OK() || r.Delay() > 0 {
		if r.OK() {
			r.Cancel() // delayed reservation -> refunds (see example 16)
		}
		il.mt.recordLimited(key)
		return false
	}
	il.mt.recordAllowed(time.Since(start))
	return true
}

func main() {
	mt := NewMetrics()
	il := &InstrumentedLimiter{m: map[string]*rate.Limiter{}, mt: mt}

	// One abusive tenant, two well-behaved ones.
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Go(func() { il.Allow("acme") }) // hammering
	}
	for i := 0; i < 2; i++ {
		wg.Go(func() { il.Allow("globex") })
		wg.Go(func() { il.Allow("initech") })
	}
	wg.Wait()

	fmt.Println("metrics snapshot:")
	mt.Report()
	fmt.Println("\nthe per-key breakdown is what tells you this is ONE abusive tenant")
	fmt.Println("and not a limit that is set too low for everybody.")
	fmt.Println("\nalert on: reject-rate rising ACROSS many keys, or avg wait growing.")
}
```

**Output:**

```
metrics snapshot:
  requests:     34
  allowed:      7
  limited:      27 (79.4%)
  avg wait:     4µs (latency the limiter added)
  limited by key:
    acme     27

the per-key breakdown is what tells you this is ONE abusive tenant
and not a limit that is set too low for everybody.

alert on: reject-rate rising ACROSS many keys, or avg wait growing.
```

---

## 26. Capstone: a tenant-aware rate-limited service

`🔴 hard` · *Capstone*

Everything wired together in one runnable service: tiered plans (15), per-key limiters (13) with idle eviction (14), `Reserve` for an accurate `Retry-After` (5), the 429 and `RateLimit-*` headers (7), metrics (25), and a graceful shutdown that drains in-flight requests and retires the sweeper (lesson 15).

**Steps:**

1. The API key selects the plan; an unknown key is **401 before any limiter work**.
2. Budget headers go on every response; rejections add `Retry-After`.
3. The sweeper goroutine has a guaranteed exit via a `stop` channel.
4. `srv.Shutdown(ctx)` drains in-flight requests, then `bg.Wait()` proves the sweeper exited.
5. free burst 2 rejects from the third request; pro at 50/s absorbs all seven.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// CAPSTONE: a tenant-aware rate-limited API service.
//
// Everything from this lesson wired together --
//   tiered plans (15) . per-key limiters (13) . idle eviction (14)
//   Reserve for Retry-After (5) . 429 + RateLimit headers (7) . metrics (25)
//   and graceful shutdown so in-flight requests finish (lesson 15).

type Plan struct {
	Name   string
	PerSec float64
	Burst  int
}

var plans = map[string]Plan{
	"free": {"free", 5, 2},
	"pro":  {"pro", 50, 5},
}

var tenants = map[string]string{ // api key -> plan
	"key-acme":   "free",
	"key-globex": "pro",
}

// ---------- limiter with eviction ----------

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

type TenantLimiter struct {
	mu      sync.Mutex
	m       map[string]*entry
	idleTTL time.Duration
}

func NewTenantLimiter(idleTTL time.Duration) *TenantLimiter {
	return &TenantLimiter{m: map[string]*entry{}, idleTTL: idleTTL}
}

func (t *TenantLimiter) get(key string, p Plan) *rate.Limiter {
	t.mu.Lock()
	defer t.mu.Unlock()
	en, ok := t.m[key]
	if !ok {
		en = &entry{lim: rate.NewLimiter(rate.Limit(p.PerSec), p.Burst)}
		t.m[key] = en
	}
	en.lastSeen = time.Now()
	return en.lim
}

func (t *TenantLimiter) sweep() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-t.idleTTL)
	n := 0
	for k, en := range t.m {
		if en.lastSeen.Before(cutoff) {
			delete(t.m, k)
			n++
		}
	}
	return n
}

func (t *TenantLimiter) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}

// ---------- metrics ----------

type Metrics struct{ allowed, limited, unauth atomic.Int64 }

// ---------- middleware ----------

func limitMiddleware(tl *TenantLimiter, mt *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		planName, known := tenants[key]
		if !known {
			mt.unauth.Add(1)
			http.Error(w, "unknown api key", http.StatusUnauthorized)
			return
		}
		p := plans[planName]
		lim := tl.get(key, p)

		// Advertise the budget on EVERY response, not only rejections.
		w.Header().Set("RateLimit-Limit", strconv.Itoa(int(p.PerSec)))
		w.Header().Set("RateLimit-Policy", fmt.Sprintf("%d;w=1;burst=%d", int(p.PerSec), p.Burst))

		res := lim.Reserve()
		if !res.OK() || res.Delay() > 0 {
			if res.OK() {
				res.Cancel() // delayed reservation: this DOES refund
			}
			retry := int(math.Ceil(res.Delay().Seconds()))
			if retry < 1 {
				retry = 1
			}
			mt.limited.Add(1)
			w.Header().Set("RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		mt.allowed.Add(1)
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(int(math.Max(0, math.Floor(lim.Tokens())))))
		next.ServeHTTP(w, r)
	})
}

func main() {
	tl := NewTenantLimiter(2 * time.Minute)
	mt := &Metrics{}

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // real work
		fmt.Fprintln(w, `{"orders":[]}`)
	})

	mux := http.NewServeMux()
	mux.Handle("/v1/orders", limitMiddleware(tl, mt, api))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: mux}

	// The sweeper: a background goroutine with a guaranteed exit (lesson 13 #28).
	stop := make(chan struct{})
	var bg sync.WaitGroup
	bg.Add(1)
	go func() {
		defer bg.Done()
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				tl.sweep()
			}
		}
	}()

	go srv.Serve(ln)
	base := "http://" + ln.Addr().String()

	call := func(key string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, base+"/v1/orders", nil)
		req.Header.Set("X-API-Key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, ""
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Retry-After")
	}

	for _, c := range []struct{ key, label string }{
		{"key-acme", "acme (free: 5/s burst 2)"},
		{"key-globex", "globex (pro: 50/s burst 5)"},
		{"key-nobody", "unknown key"},
	} {
		fmt.Printf("%-28s ", c.label)
		for i := 0; i < 7; i++ {
			code, retry := call(c.key)
			switch code {
			case 200:
				fmt.Print("200 ")
			case 429:
				fmt.Printf("429(%ss) ", retry)
			default:
				fmt.Printf("%d ", code)
			}
		}
		fmt.Println()
	}

	fmt.Printf("\nlimiters held: %d\n", tl.Len())
	fmt.Printf("metrics: allowed=%d limited=%d unauthorized=%d\n",
		mt.allowed.Load(), mt.limited.Load(), mt.unauth.Load())

	// Graceful shutdown: stop accepting, let in-flight requests finish,
	// then retire background goroutines and confirm they exited.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("shutdown error:", err)
	}
	close(stop)
	bg.Wait()
	fmt.Println("\nserver drained, sweeper exited -- clean shutdown")
}
```

**Output:**

```
acme (free: 5/s burst 2)     200 200 429(1s) 429(1s) 429(1s) 429(1s) 429(1s) 
globex (pro: 50/s burst 5)   200 200 200 200 200 200 200 
unknown key                  401 401 401 401 401 401 401 

limiters held: 2
metrics: allowed=9 limited=5 unauthorized=7

server drained, sweeper exited -- clean shutdown
```

---
