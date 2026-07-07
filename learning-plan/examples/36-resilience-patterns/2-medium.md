# 36 · Medium (6–10) — breaker, bulkhead, rate limit

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Circuit breaker states

A circuit breaker is a small state machine: too many failures trip it **open**; after a cooldown it
goes **half-open**, and a trial success **closes** it again. (Uses `for range 3` — Go 1.22+.)

```go
package main

import "fmt"

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string { return [...]string{"closed", "open", "half-open"}[s] }

type Breaker struct {
	state     State
	failures  int
	threshold int
}

func (b *Breaker) onResult(ok bool) {
	switch b.state {
	case Closed:
		if !ok {
			b.failures++
			if b.failures >= b.threshold {
				b.state = Open
			}
		} else {
			b.failures = 0
		}
	case HalfOpen:
		if ok {
			b.state, b.failures = Closed, 0
		} else {
			b.state = Open
		}
	}
}

func main() {
	b := &Breaker{threshold: 3}
	for range 3 { // 3 failures trip it
		b.onResult(false)
		fmt.Println("after failure → state:", b.state)
	}
	b.state = HalfOpen // cooldown elapsed
	fmt.Println("cooldown → state:", b.state)
	b.onResult(true) // probe succeeds
	fmt.Println("probe success → state:", b.state)
}
```

**Output**
```
after failure → state: closed
after failure → state: closed
after failure → state: open
cooldown → state: half-open
probe success → state: closed
```

---

## 7. The breaker fails fast when open

When the breaker is open it fails fast — the real call is never made, giving the dependency room to
recover.

```go
package main

import (
	"errors"
	"fmt"
)

type Breaker struct {
	open  bool
	calls int
}

var ErrOpen = errors.New("circuit open")

func (b *Breaker) Do(fn func() error) error {
	if b.open {
		return ErrOpen // fail fast — fn is NOT called
	}
	b.calls++
	return fn()
}

func main() {
	b := &Breaker{}
	call := func() error { return nil }
	fmt.Println("closed:", b.Do(call))
	b.open = true
	fmt.Println("open:  ", b.Do(call))
	fmt.Println("actual calls made:", b.calls) // 1 — the open call never ran fn
}
```

**Output**
```
closed: <nil>
open:   circuit open
actual calls made: 1
```

---

## 8. Bulkhead: cap concurrency

A bulkhead caps concurrency to one dependency (a buffered channel of N slots), so a slow dependency
can't exhaust every goroutine. Extra callers are rejected.

```go
package main

import "fmt"

type Bulkhead struct{ slots chan struct{} }

func newBulkhead(n int) *Bulkhead { return &Bulkhead{slots: make(chan struct{}, n)} }

func (b *Bulkhead) TryAcquire() bool {
	select {
	case b.slots <- struct{}{}:
		return true
	default:
		return false // full → reject
	}
}

func (b *Bulkhead) Release() { <-b.slots }

func main() {
	bh := newBulkhead(2)
	fmt.Println("acquire 1:", bh.TryAcquire())
	fmt.Println("acquire 2:", bh.TryAcquire())
	fmt.Println("acquire 3:", bh.TryAcquire()) // full → rejected
	bh.Release()
	fmt.Println("after release, acquire:", bh.TryAcquire())
}
```

**Output**
```
acquire 1: true
acquire 2: true
acquire 3: false
after release, acquire: true
```

---

## 9. Token-bucket rate limiter

A token bucket starts with `burst` tokens; each `Allow` consumes one; a refill ticker (omitted here
for a deterministic demo) tops it back up.

```go
package main

import "fmt"

type Bucket struct{ tokens int }

func (b *Bucket) Allow() bool {
	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func main() {
	b := &Bucket{tokens: 3} // burst of 3
	for i := 1; i <= 5; i++ {
		fmt.Printf("request %d: allowed=%v (tokens left %d)\n", i, b.Allow(), b.tokens)
	}
}
```

**Output**
```
request 1: allowed=true (tokens left 2)
request 2: allowed=true (tokens left 1)
request 3: allowed=true (tokens left 0)
request 4: allowed=false (tokens left 0)
request 5: allowed=false (tokens left 0)
```

---

## 10. Load shedding

Past a concurrency threshold, reject new work immediately (a fast 503) so the work already in flight
still finishes.

```go
package main

import "fmt"

type Server struct {
	inFlight int
	maxLoad  int
}

func (s *Server) handle(req string) string {
	if s.inFlight >= s.maxLoad {
		return "503 shed: " + req // reject fast
	}
	s.inFlight++
	return "200 ok: " + req
}

func main() {
	s := &Server{maxLoad: 2}
	for _, r := range []string{"a", "b", "c"} {
		fmt.Println(s.handle(r))
	}
}
```

**Output**
```
200 ok: a
200 ok: b
503 shed: c
```

---

Next tier → [Hard (11–15)](3-hard.md)
