# 36 · Hard (11–15) — composition & capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Retry only idempotent ops

Only retry idempotent operations. A non-idempotent write needs an idempotency key before it's safe to
retry, or you risk double-applying it.

```go
package main

import (
	"errors"
	"fmt"
)

func retryIfSafe(idempotent bool, maxAttempts int, op func() error) error {
	attempts := maxAttempts
	if !idempotent {
		attempts = 1 // no retry — could double-apply
	}
	var err error
	for i := 1; i <= attempts; i++ {
		if err = op(); err == nil {
			return nil
		}
		fmt.Printf("  attempt %d failed\n", i)
	}
	return err
}

func main() {
	fails := func() error { return errors.New("boom") }
	fmt.Println("idempotent GET:")
	_ = retryIfSafe(true, 3, fails)
	fmt.Println("non-idempotent POST (no key):")
	_ = retryIfSafe(false, 3, fails)
}
```

**Output**
```
idempotent GET:
  attempt 1 failed
  attempt 2 failed
  attempt 3 failed
non-idempotent POST (no key):
  attempt 1 failed
```

---

## 12. Retry guarded by a breaker

The layers compose: a retry loop guarded by a circuit breaker. Once the breaker trips mid-retry, we
stop hammering and fail fast.

```go
package main

import (
	"errors"
	"fmt"
)

type Breaker struct {
	failures, threshold int
	open                bool
}

func (b *Breaker) allow() bool { return !b.open }

func (b *Breaker) record(ok bool) {
	if ok {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.open = true
	}
}

func callWithResilience(b *Breaker, maxAttempts int, op func() error) error {
	if !b.allow() {
		return errors.New("circuit open — fail fast")
	}
	var err error
	for i := 1; i <= maxAttempts; i++ {
		err = op()
		b.record(err == nil)
		if err == nil {
			return nil
		}
		fmt.Printf("attempt %d failed; breaker failures=%d open=%v\n", i, b.failures, b.open)
		if b.open {
			return errors.New("circuit tripped mid-retry")
		}
	}
	return err
}

func main() {
	b := &Breaker{threshold: 2}
	alwaysFail := func() error { return errors.New("down") }
	fmt.Println("result:", callWithResilience(b, 5, alwaysFail))
}
```

**Output**
```
attempt 1 failed; breaker failures=1 open=false
attempt 2 failed; breaker failures=2 open=true
result: circuit tripped mid-retry
```

---

## 13. Graceful degradation (fallback)

When the primary is down (breaker open), serve a fallback (stale cache) so the request still returns
something useful.

```go
package main

import "fmt"

func getRecommendations(breakerOpen bool, staleCache []string) []string {
	if breakerOpen {
		fmt.Println("primary down → serving stale cache")
		return staleCache
	}
	return []string{"fresh-1", "fresh-2"}
}

func main() {
	fmt.Println("healthy: ", getRecommendations(false, []string{"cached-1"}))
	fmt.Println("degraded:", getRecommendations(true, []string{"cached-1"}))
}
```

**Output**
```
healthy:  [fresh-1 fresh-2]
primary down → serving stale cache
degraded: [cached-1]
```

---

## 14. A retry budget

A retry budget caps retries as a fraction of total requests, so a widespread outage doesn't multiply
load. Here at most 20% of requests may be retries — so a retry is only allowed once enough budget has
accrued.

```go
package main

import "fmt"

type Budget struct {
	requests int
	retries  int
	maxRatio float64
}

func (b *Budget) allowRetry() bool {
	b.requests++
	if float64(b.retries+1) > b.maxRatio*float64(b.requests) {
		return false // over budget
	}
	b.retries++
	return true
}

func main() {
	b := &Budget{maxRatio: 0.2}
	for i := 1; i <= 6; i++ {
		fmt.Printf("request %d: retry allowed=%v (retries=%d/%d)\n", i, b.allowRetry(), b.retries, b.requests)
	}
}
```

**Output**
```
request 1: retry allowed=false (retries=0/1)
request 2: retry allowed=false (retries=0/2)
request 3: retry allowed=false (retries=0/3)
request 4: retry allowed=false (retries=0/4)
request 5: retry allowed=true (retries=1/5)
request 6: retry allowed=false (retries=1/6)
```

---

## 15. Capstone: the full resilience stack

The resilience stack around one flaky dependency, outermost first — rate-limit → bulkhead →
circuit-breaker → retry → fallback. Each line shows which layer produced the outcome.

```go
package main

import (
	"errors"
	"fmt"
)

type Stack struct {
	tokens     int  // rate limiter
	slots      int  // bulkhead capacity
	open       bool // breaker
	maxRetries int
}

func fallback(reason string) string { return "200 stale-cache (fallback: " + reason + ")" }

func (s *Stack) call(dep func() (string, error)) string {
	if s.tokens <= 0 { // 1. rate limit
		return "429 rate-limited"
	}
	s.tokens--
	if s.slots <= 0 { // 2. bulkhead
		return "503 bulkhead-full"
	}
	s.slots--
	defer func() { s.slots++ }()
	if s.open { // 3. circuit breaker
		return fallback("breaker-open")
	}
	var err error // 4. retry the dependency
	for i := 1; i <= s.maxRetries; i++ {
		var res string
		if res, err = dep(); err == nil {
			return "200 " + res
		}
	}
	return fallback(err.Error()) // 5. all retries failed → fallback
}

func main() {
	flaky := func() (string, error) { return "", errors.New("timeout") }
	s := &Stack{tokens: 2, slots: 1, maxRetries: 3}

	fmt.Println("call 1:", s.call(flaky)) // retries fail → fallback
	s.open = true
	fmt.Println("call 2:", s.call(flaky)) // breaker open → fallback fast
	s.tokens = 0
	fmt.Println("call 3:", s.call(flaky)) // rate-limited
}
```

**Output**
```
call 1: 200 stale-cache (fallback: timeout)
call 2: 200 stale-cache (fallback: breaker-open)
call 3: 429 rate-limited
```

---

Back to [index](README.md) · Next lesson's examples: [37 — CQRS & Event Sourcing](../37-cqrs-event-sourcing/README.md).
