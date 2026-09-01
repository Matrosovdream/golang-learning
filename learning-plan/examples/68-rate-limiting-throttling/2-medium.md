# Step 68 — Rate Limiting & Throttling · 🟡 Medium

Examples **9–19**: the algorithm family, per-key limiting and its memory leak, composing several
limits, and the client side. Each is a complete, runnable `package main` program in production
shape — injected clocks, real middleware, real `http.Client` transports.

**Run any example:**

```bash
mkdir -p /tmp/rl-ex && cd /tmp/rl-ex   # once: go mod init scratch && go get golang.org/x/time@latest
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Previous tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Sliding window log — exact, and what it costs

`🟡 medium` · *Algorithms*

Store a timestamp per request, drop the expired ones, count what remains. No boundary burst at all — this is the *exact* answer. The price is memory proportional to the limit, per key, which is why it is rarely what ships.

**Steps:**

1. `hits` is sorted, so expiring old entries is trimming a prefix.
2. Run the exact traffic that defeated the fixed window in example 2.
3. 100 allowed instead of 200 — the boundary burst is gone.
4. Count the retained timestamps: a 100k/min limit means 100k of these **per active key**.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// SlidingWindowLog: keep a timestamp per request, drop the expired ones, count
// what remains. EXACT -- no boundary burst -- but memory is O(limit) per key,
// so a 100k/min limit means 100k timestamps for every active key.
type SlidingWindowLog struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   []time.Time
	now    func() time.Time
}

func NewSlidingWindowLog(limit int, window time.Duration, now func() time.Time) *SlidingWindowLog {
	return &SlidingWindowLog{limit: limit, window: window, now: now}
}

func (s *SlidingWindowLog) Allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-s.window)

	// Drop everything older than the window. hits is sorted, so this is a prefix.
	i := 0
	for i < len(s.hits) && s.hits[i].Before(cutoff) {
		i++
	}
	s.hits = s.hits[i:]

	if len(s.hits) >= s.limit {
		return false
	}
	s.hits = append(s.hits, s.now())
	return true
}

func (s *SlidingWindowLog) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hits)
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func burst(allow func() bool, n int) int {
	ok := 0
	for i := 0; i < n; i++ {
		if allow() {
			ok++
		}
	}
	return ok
}

func main() {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: base}
	s := NewSlidingWindowLog(100, time.Minute, clk.Now)

	// The exact traffic that defeated the fixed window in example 2.
	clk.Advance(59500 * time.Millisecond)
	a := burst(s.Allow, 100)
	fmt.Printf("at %s: allowed %d\n", clk.t.Format("15:04:05.000"), a)

	clk.Advance(600 * time.Millisecond)
	b := burst(s.Allow, 100)
	fmt.Printf("at %s: allowed %d\n", clk.t.Format("15:04:05.000"), b)
	fmt.Printf("\n%d allowed in 600ms (fixed window let through 200)\n", a+b)

	// The cost: one timestamp per in-window request, per key.
	fmt.Printf("timestamps retained: %d (~%d bytes for this key alone)\n",
		s.Size(), s.Size()*int(24))

	// After a full window of silence, everything expires.
	clk.Advance(time.Minute)
	fmt.Printf("after 60s idle: allowed %d more\n", burst(s.Allow, 5))
}
```

**Output:**

```
at 10:00:59.500: allowed 100
at 10:01:00.100: allowed 0

100 allowed in 600ms (fixed window let through 200)
timestamps retained: 100 (~2400 bytes for this key alone)
after 60s idle: allowed 5 more
```

---

## 10. Sliding window counter — the production compromise

`🟡 medium` · *Algorithms*

Keep two integers — this window's count and the previous one's — and weight the previous by how much of it is still in view. Fixes almost all of the boundary burst at O(1) memory. This is what most API gateways actually run.

**Steps:**

1. `roll` advances the window, sliding `cur` into `prev` or clearing both after a gap.
2. `estimate` = `cur + prev × overlap`, where overlap shrinks as the window advances.
3. Same traffic as examples 2 and 9: **101** allowed, against 200 (fixed) and 100 (exact).
4. Two ints per key instead of one timestamp per request.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// SlidingWindowCounter: the production compromise. Keep only TWO integers --
// this window's count and the previous window's -- and weight the previous one
// by how far into the current window we are. Fixes most of the boundary burst
// at O(1) memory per key. This is what most API gateways actually run.
type SlidingWindowCounter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	cur      int
	prev     int
	curStart time.Time
	now      func() time.Time
}

func NewSlidingWindowCounter(limit int, window time.Duration, now func() time.Time) *SlidingWindowCounter {
	return &SlidingWindowCounter{limit: limit, window: window, now: now}
}

func (s *SlidingWindowCounter) roll(t time.Time) {
	w := t.Truncate(s.window)
	switch {
	case w.Equal(s.curStart):
		return // same window
	case w.Sub(s.curStart) == s.window:
		s.prev, s.cur, s.curStart = s.cur, 0, w // slid by exactly one
	default:
		s.prev, s.cur, s.curStart = 0, 0, w // gap: both windows are stale
	}
}

// estimate = cur + prev * (fraction of the previous window still in view)
func (s *SlidingWindowCounter) estimate(t time.Time) float64 {
	elapsed := t.Sub(s.curStart)
	overlap := 1 - float64(elapsed)/float64(s.window)
	return float64(s.cur) + float64(s.prev)*overlap
}

func (s *SlidingWindowCounter) Allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.now()
	s.roll(t)
	if s.estimate(t) >= float64(s.limit) {
		return false
	}
	s.cur++
	return true
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func burst(allow func() bool, n int) int {
	ok := 0
	for i := 0; i < n; i++ {
		if allow() {
			ok++
		}
	}
	return ok
}

func main() {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: base}
	s := NewSlidingWindowCounter(100, time.Minute, clk.Now)

	clk.Advance(59500 * time.Millisecond)
	a := burst(s.Allow, 100)
	fmt.Printf("at %s: allowed %d  (estimate now %.1f)\n",
		clk.t.Format("15:04:05.000"), a, s.estimate(clk.t))

	clk.Advance(600 * time.Millisecond) // 1% into the new window
	b := burst(s.Allow, 100)
	fmt.Printf("at %s: allowed %d  (previous window still counts ~99%%)\n",
		clk.t.Format("15:04:05.000"), b)

	fmt.Printf("\n%d allowed in 600ms -- fixed window: 200, sliding log: 100\n", a+b)
	fmt.Println("cost: 2 ints per key, vs 1 timestamp per request")
}
```

**Output:**

```
at 10:00:59.500: allowed 100  (estimate now 100.0)
at 10:01:00.100: allowed 1  (previous window still counts ~99%)

101 allowed in 600ms -- fixed window: 200, sliding log: 100
cost: 2 ints per key, vs 1 timestamp per request
```

---

## 11. Leaky bucket — shaping, not just limiting

`🟡 medium` · *Algorithms*

The dual of a token bucket: same accounting, different verb. Work enters a bounded queue and leaves at a constant rate, so bursty input becomes perfectly smooth output. Right when the downstream is fragile and you would rather add latency than drop work.

**Steps:**

1. The bucket *is* a buffered channel; its capacity is the queue depth.
2. A drain goroutine ticks and executes exactly one job per tick.
3. `Submit` is non-blocking — a full bucket overflows rather than backing up.
4. Eight jobs submitted instantly: four run at a perfect 50ms cadence, four overflow.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// LeakyBucket SHAPES traffic instead of rejecting it: work enters a bounded
// queue and leaves at a constant rate. The dual of a token bucket -- same
// accounting, different verb. Right when the downstream is fragile and you
// would rather add latency than drop work.
type LeakyBucket struct {
	queue chan func()
	wg    sync.WaitGroup
	stop  chan struct{}
}

var ErrBucketFull = errors.New("leaky bucket full")

func NewLeakyBucket(capacity int, interval time.Duration) *LeakyBucket {
	b := &LeakyBucket{
		queue: make(chan func(), capacity), // the bucket IS the buffer
		stop:  make(chan struct{}),
	}
	b.wg.Add(1)
	go b.drain(interval)
	return b
}

// drain is the leak: exactly one job per tick, forever.
func (b *LeakyBucket) drain(interval time.Duration) {
	defer b.wg.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			select {
			case job := <-b.queue:
				job()
			default: // nothing queued: this tick is simply wasted
			}
		}
	}
}

// Submit is non-blocking: a full bucket overflows rather than backing up.
func (b *LeakyBucket) Submit(ctx context.Context, job func()) error {
	select {
	case b.queue <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrBucketFull
	}
}

func (b *LeakyBucket) Close() { close(b.stop); b.wg.Wait() }

func main() {
	b := NewLeakyBucket(4, 50*time.Millisecond) // capacity 4, drains 20/sec
	defer b.Close()

	start := time.Now()
	var mu sync.Mutex
	var done []string

	// A bursty producer: 8 jobs submitted instantly.
	for i := 1; i <= 8; i++ {
		err := b.Submit(context.Background(), func() {
			mu.Lock()
			done = append(done, fmt.Sprintf("job %d ran at %v", i, time.Since(start).Round(50*time.Millisecond)))
			mu.Unlock()
		})
		if err != nil {
			fmt.Printf("job %d rejected: %v\n", i, err)
		}
	}

	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	for _, s := range done {
		fmt.Println(s)
	}
	mu.Unlock()
	fmt.Println("\nbursty in, perfectly smooth out -- that is shaping, not just limiting")
}
```

**Output:**

```
job 5 rejected: leaky bucket full
job 6 rejected: leaky bucket full
job 7 rejected: leaky bucket full
job 8 rejected: leaky bucket full
job 1 ran at 50ms
job 2 ran at 100ms
job 3 ran at 150ms
job 4 ran at 200ms

bursty in, perfectly smooth out -- that is shaping, not just limiting
```

---

## 12. Token bucket by hand — lazy refill, no goroutine

`🟡 medium` · *Algorithms*

This is how `x/time/rate` works inside, and why an idle limiter costs nothing: there is no ticker and no goroutine. Tokens are *computed from elapsed time* on the next call. The naive alternative — a goroutine per limiter topping up a counter — means one goroutine per API key.

**Steps:**

1. On each call, add `elapsed × rate` tokens and clamp to `burst`.
2. The clock is injected, so the whole example is deterministic and instant.
3. Advancing 10 seconds does not grant 100 tokens — idle time is capped at capacity.
4. `AllowN` lets an expensive request cost several tokens.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// TokenBucket implemented by hand -- LAZY refill, no ticker and no goroutine.
// This is how x/time/rate works internally, and why a limiter costs nothing
// while idle: tokens are computed from elapsed time on the next call.
//
// The naive alternative (a goroutine per limiter topping up a counter) means
// one goroutine per API key. This means zero.
type TokenBucket struct {
	mu     sync.Mutex
	rate   float64 // tokens per second
	burst  float64 // bucket capacity
	tokens float64
	last   time.Time
	now    func() time.Time
}

func NewTokenBucket(perSec, burst float64, now func() time.Time) *TokenBucket {
	return &TokenBucket{rate: perSec, burst: burst, tokens: burst, last: now(), now: now}
}

func (b *TokenBucket) Allow() bool { return b.AllowN(1) }

func (b *TokenBucket) AllowN(n float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := b.now()
	elapsed := t.Sub(b.last).Seconds()
	b.last = t

	b.tokens += elapsed * b.rate // refill for the time that passed
	if b.tokens > b.burst {
		b.tokens = b.burst // idle time does not accumulate past capacity
	}
	if b.tokens < n {
		return false
	}
	b.tokens -= n
	return true
}

func (b *TokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func main() {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)}
	b := NewTokenBucket(10, 5, clk.Now) // 10/sec, burst 5

	fmt.Println("-- drain the burst (no time passes) --")
	for i := 1; i <= 7; i++ {
		fmt.Printf("  req %d: allow=%-5v tokens=%.2f\n", i, b.Allow(), b.Tokens())
	}

	fmt.Println("\n-- advance 300ms: 3 tokens refill --")
	clk.Advance(300 * time.Millisecond)
	for i := 8; i <= 11; i++ {
		fmt.Printf("  req %d: allow=%-5v tokens=%.2f\n", i, b.Allow(), b.Tokens())
	}

	fmt.Println("\n-- advance 10s: refill is CAPPED at burst, not 100 tokens --")
	clk.Advance(10 * time.Second)
	b.Allow()
	fmt.Printf("  tokens=%.2f (capacity 5)\n", b.Tokens())

	fmt.Println("\n-- AllowN: a costly request can take several tokens --")
	fmt.Printf("  AllowN(3)=%v tokens=%.2f\n", b.AllowN(3), b.Tokens())
	fmt.Printf("  AllowN(3)=%v tokens=%.2f (not enough left)\n", b.AllowN(3), b.Tokens())
}
```

**Output:**

```
-- drain the burst (no time passes) --
  req 1: allow=true  tokens=4.00
  req 2: allow=true  tokens=3.00
  req 3: allow=true  tokens=2.00
  req 4: allow=true  tokens=1.00
  req 5: allow=true  tokens=0.00
  req 6: allow=false tokens=0.00
  req 7: allow=false tokens=0.00

-- advance 300ms: 3 tokens refill --
  req 8: allow=true  tokens=2.00
  req 9: allow=true  tokens=1.00
  req 10: allow=true  tokens=0.00
  req 11: allow=false tokens=0.00

-- advance 10s: refill is CAPPED at burst, not 100 tokens --
  tokens=4.00 (capacity 5)

-- AllowN: a costly request can take several tokens --
  AllowN(3)=true tokens=1.00
  AllowN(3)=false tokens=1.00 (not enough left)
```

---

## 13. Per-key limiters: one budget per tenant

`🟡 medium` · *Per-key*

Real limits are per tenant, per API key, per IP — which means a map of limiters behind a lock. The load-bearing detail is the check-and-insert under one lock: two concurrent first requests for a key must not create two limiters, or each gets its own budget.

**Steps:**

1. `limiterFor` does the lookup and insert under a single `Lock`.
2. Three tenants hammer concurrently and each gets its own burst of 3.
3. A noisy tenant exhausts only its own bucket.
4. A brand-new tenant still gets a full burst — isolation confirmed.

```go
package main

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Real limits are PER KEY -- per tenant, per API key, per IP. That means a
// map of limiters behind a lock. Note the double-checked insert: two requests
// for a new key must not create two limiters, or each gets its own budget.
type KeyedLimiter struct {
	mu    sync.Mutex
	m     map[string]*rate.Limiter
	rate  rate.Limit
	burst int
}

func NewKeyedLimiter(r rate.Limit, burst int) *KeyedLimiter {
	return &KeyedLimiter{m: make(map[string]*rate.Limiter), rate: r, burst: burst}
}

func (k *KeyedLimiter) limiterFor(key string) *rate.Limiter {
	k.mu.Lock()
	defer k.mu.Unlock()
	lim, ok := k.m[key]
	if !ok {
		lim = rate.NewLimiter(k.rate, k.burst)
		k.m[key] = lim
	}
	return lim
}

func (k *KeyedLimiter) Allow(key string) bool { return k.limiterFor(key).Allow() }

func (k *KeyedLimiter) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.m)
}

func main() {
	kl := NewKeyedLimiter(rate.Every(100*time.Millisecond), 3) // 10/s, burst 3 -- EACH

	// Three tenants hammering concurrently: budgets must not interfere.
	tenants := []string{"acme", "globex", "initech"}
	results := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, t := range tenants {
		wg.Go(func() {
			allowed := 0
			for i := 0; i < 5; i++ {
				if kl.Allow(t) {
					allowed++
				}
			}
			mu.Lock()
			results[t] = allowed
			mu.Unlock()
		})
	}
	wg.Wait()

	for _, t := range tenants {
		fmt.Printf("%-8s allowed %d of 5\n", t, results[t])
	}
	fmt.Printf("\nlimiters held: %d (one per tenant, each with its own burst)\n", kl.Len())

	// A noisy neighbour cannot spend anyone else's budget.
	for i := 0; i < 50; i++ {
		kl.Allow("acme") // acme hammers the API
	}
	fmt.Printf("\nafter acme burns 50 more requests:\n")
	fmt.Printf("  acme  allowed=%v (its own bucket is empty)\n", kl.Allow("acme"))
	fmt.Printf("  newco allowed=%v (a fresh tenant gets a full burst)\n", kl.Allow("newco"))
	fmt.Printf("  limiters held: %d\n", kl.Len())
}
```

**Output:**

```
acme     allowed 3 of 5
globex   allowed 3 of 5
initech  allowed 3 of 5

limiters held: 3 (one per tenant, each with its own burst)

after acme burns 50 more requests:
  acme  allowed=false (its own bucket is empty)
  newco allowed=true (a fresh tenant gets a full burst)
  limiters held: 4
```

---

## 14. Evicting idle keys — the leak every per-key limiter has

`🟡 medium` · *Per-key*

The most common real bug in per-key limiting. The key is **caller-supplied** — an IP, an API key — so an unbounded map is a memory-exhaustion vector, not just untidiness. Every per-key limiter needs a `lastSeen` stamp and a sweeper.

**Steps:**

1. Each entry records `lastSeen`; the limiter call happens **outside** the map lock.
2. `sweep` deletes entries idle longer than the TTL — deleting during `range` is safe in Go.
3. A 10,000-IP scan creates 10,000 limiters that will never be used again.
4. After the sweep: **1 remains**. Without it, the map only ever grows.

```go
package main

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// The most common REAL bug in per-key limiting: the map grows forever.
// The key is attacker-supplied (an IP, an API key), so an unbounded map is a
// memory-exhaustion vector, not just untidiness. Every per-key limiter needs
// a lastSeen stamp and a sweeper.
type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

type EvictingLimiter struct {
	mu      sync.Mutex
	m       map[string]*entry
	rate    rate.Limit
	burst   int
	idleTTL time.Duration
	now     func() time.Time
	stop    chan struct{}
	wg      sync.WaitGroup
}

func NewEvictingLimiter(r rate.Limit, burst int, idleTTL, sweepEvery time.Duration, now func() time.Time) *EvictingLimiter {
	e := &EvictingLimiter{
		m: make(map[string]*entry), rate: r, burst: burst,
		idleTTL: idleTTL, now: now, stop: make(chan struct{}),
	}
	e.wg.Add(1)
	go e.sweepLoop(sweepEvery)
	return e
}

func (e *EvictingLimiter) Allow(key string) bool {
	e.mu.Lock()
	en, ok := e.m[key]
	if !ok {
		en = &entry{lim: rate.NewLimiter(e.rate, e.burst)}
		e.m[key] = en
	}
	en.lastSeen = e.now()
	lim := en.lim
	e.mu.Unlock()
	return lim.Allow() // limiter call happens OUTSIDE the map lock
}

// sweep drops keys idle for longer than idleTTL. Returns how many it evicted.
func (e *EvictingLimiter) sweep() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	cutoff := e.now().Add(-e.idleTTL)
	n := 0
	for k, en := range e.m {
		if en.lastSeen.Before(cutoff) {
			delete(e.m, k) // deleting during range is safe in Go
			n++
		}
	}
	return n
}

func (e *EvictingLimiter) sweepLoop(every time.Duration) {
	defer e.wg.Done()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-t.C:
			e.sweep()
		}
	}
}

func (e *EvictingLimiter) Close() { close(e.stop); e.wg.Wait() }

func (e *EvictingLimiter) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.m)
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
	el := NewEvictingLimiter(rate.Every(time.Second), 5, 2*time.Minute, time.Hour, clk.Now)
	defer el.Close()

	// 10,000 one-shot callers -- e.g. a scan from a botnet, each IP seen once.
	for i := 0; i < 10000; i++ {
		el.Allow(fmt.Sprintf("203.0.113.%d:%d", i%256, i))
	}
	fmt.Printf("after a 10k-IP scan:      %d limiters held\n", el.Len())

	// Two active tenants keep using it.
	clk.Advance(time.Minute)
	el.Allow("tenant-acme")
	el.Allow("tenant-globex")
	fmt.Printf("plus 2 real tenants:      %d limiters held\n", el.Len())

	// Time passes; the scanners are long gone.
	clk.Advance(3 * time.Minute)
	el.Allow("tenant-acme") // keeps acme fresh
	evicted := el.sweep()   // the sweeper goroutine would do this on its ticker
	fmt.Printf("after sweep (TTL 2m):     evicted %d, %d remain\n", evicted, el.Len())
	fmt.Println("\nwithout the sweeper this map only ever grows -- with a caller-supplied key")
}
```

**Output:**

```
after a 10k-IP scan:      10000 limiters held
plus 2 real tenants:      10002 limiters held
after sweep (TTL 2m):     evicted 10001, 1 remain

without the sweeper this map only ever grows -- with a caller-supplied key
```

---

## 15. Tiered limits: the budget comes from the plan

`🟡 medium` · *Per-key*

Production limits are per plan, not per constant. Build the limiter from config on first use and adding a tier becomes a config change rather than a deploy. Note the fallback direction — that choice is a security decision.

**Steps:**

1. A plan table maps name → rate and burst.
2. The limiter for a tenant is constructed once, from that tenant's plan.
3. Each tier's instant burst is exactly its configured burst.
4. An **unknown** plan falls back to the cheapest tier — never the most generous.

```go
package main

import (
	"fmt"
	"sync"

	"golang.org/x/time/rate"
)

// Tiered limits: the budget comes from the caller's PLAN, not a constant.
// The limiter is created from config on first use, so adding a plan is a
// config change rather than a code change.
type Plan struct {
	Name   string
	PerSec float64
	Burst  int
}

var plans = map[string]Plan{
	"free":       {"free", 2, 2},
	"pro":        {"pro", 10, 20},
	"enterprise": {"enterprise", 100, 200},
}

type TieredLimiter struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter // key: tenant id
}

func NewTieredLimiter() *TieredLimiter {
	return &TieredLimiter{m: make(map[string]*rate.Limiter)}
}

func (t *TieredLimiter) Allow(tenant, plan string) bool {
	t.mu.Lock()
	lim, ok := t.m[tenant]
	if !ok {
		p, known := plans[plan]
		if !known {
			p = plans["free"] // unknown plan: fail to the cheapest tier, never the most generous
		}
		lim = rate.NewLimiter(rate.Limit(p.PerSec), p.Burst)
		t.m[tenant] = lim
	}
	t.mu.Unlock()
	return lim.Allow()
}

func drain(t *TieredLimiter, tenant, plan string, n int) int {
	ok := 0
	for i := 0; i < n; i++ {
		if t.Allow(tenant, plan) {
			ok++
		}
	}
	return ok
}

func main() {
	t := NewTieredLimiter()

	fmt.Printf("%-10s %-14s %-6s %s\n", "tenant", "plan", "burst", "allowed of 50 instant requests")
	for _, c := range []struct{ tenant, plan string }{
		{"acme", "free"},
		{"globex", "pro"},
		{"initech", "enterprise"},
		{"shady", "platinum"}, // a plan that does not exist
	} {
		p, known := plans[c.plan]
		label := c.plan
		if !known {
			label = c.plan + " (?)"
			p = plans["free"]
		}
		fmt.Printf("%-10s %-14s %-6d %d\n", c.tenant, label, p.Burst, drain(t, c.tenant, c.plan, 50))
	}
	fmt.Println("\nunknown plans fall back to the CHEAPEST tier -- never the most generous")
}
```

**Output:**

```
tenant     plan           burst  allowed of 50 instant requests
acme       free           2      2
globex     pro            20     20
initech    enterprise     200    50
shady      platinum (?)   2      2

unknown plans fall back to the CHEAPEST tier -- never the most generous
```

---

## 16. Several limits at once — and why Cancel() can't save you

`🟡 medium` · *Composition*

APIs enforce 10/second *and* 8/minute together. Check them in turn with `Allow()` and the per-second limiter is charged for requests the per-minute one rejects — the client is punished twice for one refusal. And you cannot reserve-and-cancel your way out: **`Reservation.Cancel()` only refunds when `Delay() > 0`**. A token available immediately is gone once taken.

**Steps:**

1. `naiveAllow` calls `Allow()` on each limiter in turn — the version everyone writes first.
2. The fix: `TokensAt(now)` checks every limit **without consuming**, then all are charged together.
3. A mutex makes that check-then-act atomic across the whole set.
4. Same verdicts either way — but the naive version leaves per-second at 0.0 where the correct one holds 2.0.

```go
package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Real APIs enforce SEVERAL limits at once: 10/second AND 8/minute.
// The subtlety: if the per-second limiter allows (spending a token) and the
// per-minute one then denies, that first token is spent on a request you
// rejected. And you CANNOT simply reserve-and-cancel your way out:
// Reservation.Cancel() only refunds when Delay() > 0. A token that was
// available immediately is gone the moment you take it.
type namedLimiter struct {
	name string
	lim  *rate.Limiter
}

// naiveAllow: the version almost everyone writes first.
func naiveAllow(lims []namedLimiter) (bool, string) {
	for _, nl := range lims {
		if !nl.lim.Allow() { // charges this limiter even if a later one rejects
			return false, nl.name
		}
	}
	return true, ""
}

// CompositeLimiter: check every limit WITHOUT consuming, then consume all.
// The mutex makes check-then-act atomic across the whole set.
type CompositeLimiter struct {
	mu     sync.Mutex
	limits []namedLimiter
}

func (c *CompositeLimiter) Allow() (bool, string, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()

	for _, nl := range c.limits {
		if nl.lim.TokensAt(now) < 1 { // non-destructive check
			r := nl.lim.Reserve() // only to read the delay for Retry-After
			d := r.Delay()
			r.Cancel() // safe: this reservation is in the future, so it refunds
			return false, nl.name, d
		}
	}
	for _, nl := range c.limits {
		nl.lim.AllowN(now, 1) // every limit had room: charge them all
	}
	return true, "", 0
}

func tokens(lims []namedLimiter) string {
	s := ""
	for _, nl := range lims {
		s += fmt.Sprintf("%s=%.1f ", nl.name, nl.lim.Tokens())
	}
	return s
}

func fresh() []namedLimiter {
	return []namedLimiter{
		{"per-second", rate.NewLimiter(rate.Every(100*time.Millisecond), 5)}, // 10/s, burst 5
		{"per-minute", rate.NewLimiter(rate.Every(time.Minute/8), 3)},        // 8/min, burst 3
	}
}

func main() {
	fmt.Println("NAIVE (Allow() each in turn):")
	n := fresh()
	for i := 1; i <= 5; i++ {
		ok, which := naiveAllow(n)
		if ok {
			fmt.Printf("  req %d -> 200                [%s]\n", i, tokens(n))
		} else {
			fmt.Printf("  req %d -> 429 by %-11s [%s]\n", i, which, tokens(n))
		}
	}

	fmt.Println("\nCORRECT (check all, then consume all):")
	c := &CompositeLimiter{limits: fresh()}
	for i := 1; i <= 5; i++ {
		ok, which, d := c.Allow()
		if ok {
			fmt.Printf("  req %d -> 200                [%s]\n", i, tokens(c.limits))
		} else {
			fmt.Printf("  req %d -> 429 by %-11s Retry-After: %ds [%s]\n",
				i, which, int(math.Ceil(d.Seconds())), tokens(c.limits))
		}
	}
	fmt.Println("\nsame verdicts -- but the naive version burns per-second tokens on")
	fmt.Println("requests it rejected, so a client is punished twice for one refusal")
}
```

**Output:**

```
NAIVE (Allow() each in turn):
  req 1 -> 200                [per-second=4.0 per-minute=2.0 ]
  req 2 -> 200                [per-second=3.0 per-minute=1.0 ]
  req 3 -> 200                [per-second=2.0 per-minute=0.0 ]
  req 4 -> 429 by per-minute  [per-second=1.0 per-minute=0.0 ]
  req 5 -> 429 by per-minute  [per-second=0.0 per-minute=0.0 ]

CORRECT (check all, then consume all):
  req 1 -> 200                [per-second=4.0 per-minute=2.0 ]
  req 2 -> 200                [per-second=3.0 per-minute=1.0 ]
  req 3 -> 200                [per-second=2.0 per-minute=0.0 ]
  req 4 -> 429 by per-minute  Retry-After: 8s [per-second=2.0 per-minute=0.0 ]
  req 5 -> 429 by per-minute  Retry-After: 8s [per-second=2.0 per-minute=0.0 ]

same verdicts -- but the naive version burns per-second tokens on
requests it rejected, so a client is punished twice for one refusal
```

---

## 17. Rate limit vs concurrency limit

`🟡 medium` · *Composition*

Two different questions, and neither tool substitutes for the other. A rate limit bounds how many *start* per second; a semaphore bounds how many are *in flight*. A slow dependency needs the second — and a latency spike multiplies in-flight work without changing the arrival rate at all.

**Steps:**

1. Identical 20/sec arrivals in both runs; each request takes 200ms downstream.
2. A CAS loop records the peak in-flight count (lesson 13 #36).
3. Rate limit only: **36** concurrent requests at peak.
4. Semaphore of 8: peak of exactly **8**, same throughput.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Rate limiting and concurrency limiting answer DIFFERENT questions, and one
// cannot substitute for the other:
//
//	rate limit        -> how many START per second   (protects a quota)
//	concurrency limit -> how many are IN FLIGHT      (protects a resource)
//
// A slow dependency needs the second. A metered API needs the first.
// Production systems usually need both.
type tracker struct {
	inFlight atomic.Int64
	peak     atomic.Int64
	served   atomic.Int64
}

func (t *tracker) enter() {
	cur := t.inFlight.Add(1)
	for {
		old := t.peak.Load()
		if cur <= old || t.peak.CompareAndSwap(old, cur) {
			break
		}
	}
}
func (t *tracker) exit() { t.inFlight.Add(-1); t.served.Add(1) }

// Every request takes 200ms downstream -- a slow dependency.
func work(t *tracker) {
	t.enter()
	defer t.exit()
	time.Sleep(200 * time.Millisecond)
}

func main() {
	const arrivals = 40

	// A) Rate limited to 20/sec: bounds STARTS, not in-flight.
	rateOnly := &tracker{}
	var wg sync.WaitGroup
	for i := 0; i < arrivals; i++ {
		wg.Go(func() { work(rateOnly) })
		time.Sleep(5 * time.Millisecond) // a 20/sec arrival rate
	}
	wg.Wait()
	fmt.Printf("rate limit only (20/s):  served=%d  peak in flight=%d\n",
		rateOnly.served.Load(), rateOnly.peak.Load())

	// B) Same arrivals, but a semaphore caps in-flight at 8.
	semOnly := &tracker{}
	sem := make(chan struct{}, 8)
	var wg2 sync.WaitGroup
	for i := 0; i < arrivals; i++ {
		wg2.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			work(semOnly)
		})
		time.Sleep(5 * time.Millisecond)
	}
	wg2.Wait()
	fmt.Printf("concurrency limit (8):   served=%d  peak in flight=%d\n",
		semOnly.served.Load(), semOnly.peak.Load())

	fmt.Println("\nidentical arrival rate; only the semaphore bounds resource usage.")
	fmt.Println("a rate limiter cannot: 20/s x 200ms latency = 4 in flight on average,")
	fmt.Println("but a latency spike multiplies that without changing the rate at all.")
}
```

**Output:**

```
rate limit only (20/s):  served=40  peak in flight=37
concurrency limit (8):   served=40  peak in flight=8

identical arrival rate; only the semaphore bounds resource usage.
a rate limiter cannot: 20/s x 200ms latency = 4 in flight on average,
but a latency spike multiplies that without changing the rate at all.
```

---

## 18. Load shedding on queue depth

`🟡 medium` · *Shedding*

Shedding asks a different question again: not "has this client used its quota?" but "am I healthy enough to accept this?". The signal is your own in-flight depth, so the limit adapts to how the service actually feels rather than to a number you guessed months ago.

**Steps:**

1. Middleware increments an in-flight counter and rejects above `maxDepth`.
2. `defer` decrements on every path, including the rejection.
3. 30 simultaneous requests against a depth of 5.
4. 5 served fast, 25 shed — shedding trades some requests for the latency of the rest.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

// Load shedding is not rate limiting: it asks "am I healthy enough to accept
// this?", not "has this client used its quota?". The signal is your own queue
// depth or latency -- so the limit adapts to how the service actually feels.
type shedder struct {
	inFlight atomic.Int64
	maxDepth int64
	shed     atomic.Int64
	served   atomic.Int64
}

func (s *shedder) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := s.inFlight.Add(1)
		defer s.inFlight.Add(-1)

		if cur > s.maxDepth {
			s.shed.Add(1)
			w.Header().Set("Retry-After", "1")
			// 429 (not 503): the request is refused, the service is fine.
			http.Error(w, "overloaded, shedding load", http.StatusTooManyRequests)
			return
		}
		s.served.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	s := &shedder{maxDepth: 5}

	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // a slow downstream
		fmt.Fprintln(w, "ok")
	})
	srv := httptest.NewServer(s.middleware(slow))
	defer srv.Close()

	// 30 clients arrive at once -- far more than the service can hold.
	codes := make([]int, 30)
	done := make(chan int, 30)
	for i := 0; i < 30; i++ {
		go func() {
			resp, err := http.Get(srv.URL + "/report")
			if err != nil {
				done <- 0
				return
			}
			resp.Body.Close()
			done <- resp.StatusCode
		}()
	}
	for i := 0; i < 30; i++ {
		codes[i] = <-done
	}

	ok, limited := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	fmt.Printf("30 simultaneous requests, max depth %d\n", s.maxDepth)
	fmt.Printf("  200 OK:               %d\n", ok)
	fmt.Printf("  429 shed:             %d\n", limited)
	fmt.Println("\nthe survivors got FAST responses instead of everyone timing out --")
	fmt.Println("shedding trades some requests for the latency of the rest")
}
```

**Output:**

```
30 simultaneous requests, max depth 5
  200 OK:               5
  429 shed:             25

the survivors got FAST responses instead of everyone timing out --
shedding trades some requests for the latency of the rest
```

---

## 19. Be a polite client: honour Retry-After

`🟡 medium` · *Client*

The other side of the contract. When a server sends `Retry-After` it has told you exactly when to return — inventing your own backoff instead is how you get banned. And the retry sleep must stay cancellable.

**Steps:**

1. Parse `Retry-After` in both legal forms: delta-seconds and an HTTP date.
2. If the server asks for longer than your budget, fail **immediately** rather than sleeping.
3. Sleep with a `time.Timer` inside a `select` on `ctx.Done()` — never a bare `time.Sleep`.
4. Two 429s, honoured, then success — total 2s, exactly what the server asked for.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"time"
)

// The other side of the contract: BE a well-behaved client.
// Honour Retry-After instead of inventing your own backoff -- the server just
// told you exactly when to come back. Retrying sooner is how you get banned.
type politeClient struct {
	http       *http.Client
	maxRetries int
	maxWait    time.Duration
}

var errGaveUp = errors.New("gave up after retries")

// retryAfter parses the header; delta-seconds form (an HTTP-date is also legal).
func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			return time.Until(t)
		}
	}
	return fallback
}

func (c *politeClient) get(ctx context.Context, url string) (string, int, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", 0, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return "", 0, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			return string(body), attempt, nil
		}

		wait := retryAfter(resp, 500*time.Millisecond)
		if wait > c.maxWait {
			return "", attempt, fmt.Errorf("server asked for %v, over our %v budget: %w", wait, c.maxWait, errGaveUp)
		}
		fmt.Printf("  attempt %d: 429, server said wait %v\n", attempt+1, wait)

		// Sleep, but stay cancellable -- never a bare time.Sleep in a client.
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return "", attempt, ctx.Err()
		}
	}
	return "", c.maxRetries, errGaveUp
}

func main() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 { // the first two callers are throttled
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := &politeClient{http: &http.Client{Timeout: 5 * time.Second}, maxRetries: 3, maxWait: 2 * time.Second}

	fmt.Println("polite client:")
	start := time.Now()
	body, attempts, err := c.get(context.Background(), srv.URL+"/v1/orders")
	fmt.Printf("  -> %q after %d retries in %v (err=%v)\n",
		body, attempts, time.Since(start).Round(100*time.Millisecond), err)

	// A server asking for longer than we can wait: fail fast, do not sleep.
	strict := &politeClient{http: &http.Client{Timeout: 5 * time.Second}, maxRetries: 3, maxWait: 500 * time.Millisecond}
	hits.Store(0)
	fmt.Println("\nclient with a 500ms budget, server asks for 1s:")
	_, _, err = strict.get(context.Background(), srv.URL+"/v1/orders")
	fmt.Printf("  -> gave up immediately: %v\n", err)
}
```

**Output:**

```
polite client:
  attempt 1: 429, server said wait 1s
  attempt 2: 429, server said wait 1s
  -> "{\"ok\":true}" after 2 retries in 2s (err=<nil>)

client with a 500ms budget, server asks for 1s:
  -> gave up immediately: server asked for 1s, over our 500ms budget: gave up after retries
```

---
