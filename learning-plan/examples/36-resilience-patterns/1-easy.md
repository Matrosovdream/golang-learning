# 36 · Easy (1–5) — timeouts, retries, backoff

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Timeout with context

Every outbound call gets a deadline via `context`, so a wedged peer can't hang you. Checking
`ctx.Err()` bails out early. (A past deadline makes this deterministic — no real waiting.)

```go
package main

import (
	"context"
	"fmt"
	"time"
)

func doWork(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err // already cancelled/expired → bail out
	}
	return nil // otherwise do the work
}

func main() {
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0)) // deadline in 1970
	defer cancel()
	fmt.Println("work returned:", doWork(ctx))
}
```

**Output**
```
work returned: context deadline exceeded
```

---

## 2. Retryable vs non-retryable errors

Only retry **retryable** errors. Timeouts and 503s are worth another try; a 4xx won't get better, so
retrying just wastes time.

```go
package main

import (
	"errors"
	"fmt"
)

var (
	errTimeout     = errors.New("timeout")
	errUnavailable = errors.New("503 service unavailable")
	errBadRequest  = errors.New("400 bad request")
	errNotFound    = errors.New("404 not found")
)

func retryable(err error) bool {
	switch {
	case errors.Is(err, errTimeout), errors.Is(err, errUnavailable):
		return true
	default:
		return false
	}
}

func main() {
	for _, err := range []error{errTimeout, errUnavailable, errBadRequest, errNotFound} {
		fmt.Printf("%-24s retryable=%v\n", err, retryable(err))
	}
}
```

**Output**
```
timeout                  retryable=true
503 service unavailable  retryable=true
400 bad request          retryable=false
404 not found            retryable=false
```

---

## 3. A retry loop

A retry loop turns a transient blip into success: try until it works or the attempt cap is hit.

```go
package main

import (
	"errors"
	"fmt"
)

func retry(maxAttempts int, op func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = op()
		if err == nil {
			fmt.Printf("attempt %d: success\n", attempt)
			return nil
		}
		fmt.Printf("attempt %d: %v\n", attempt, err)
	}
	return err
}

func main() {
	failsLeft := 2
	op := func() error {
		if failsLeft > 0 {
			failsLeft--
			return errors.New("transient")
		}
		return nil
	}
	retry(5, op)
}
```

**Output**
```
attempt 1: transient
attempt 2: transient
attempt 3: success
```

---

## 4. Exponential backoff

Exponential backoff spaces retries out: wait `base*2^i` between attempts, so a recovering dependency
isn't hammered. (Printed, not slept, for a deterministic demo.)

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	base := 100 * time.Millisecond
	for i := 0; i < 5; i++ {
		backoff := base * (1 << i)
		fmt.Printf("retry %d: wait %s\n", i+1, backoff)
	}
}
```

**Output**
```
retry 1: wait 100ms
retry 2: wait 200ms
retry 3: wait 400ms
retry 4: wait 800ms
retry 5: wait 1.6s
```

---

## 5. Backoff with jitter

**Full jitter**: wait a random duration in `[0, base*2^i)`, so many clients don't resynchronise into
a thundering herd. Seeded here for a reproducible demo; in production the seed is random.

```go
package main

import (
	"fmt"
	"math/rand"
	"time"
)

func main() {
	r := rand.New(rand.NewSource(1))
	base := 100 * time.Millisecond
	for i := 0; i < 5; i++ {
		maxWait := base * (1 << i)
		jittered := time.Duration(r.Int63n(int64(maxWait)))
		fmt.Printf("retry %d: max %s, jittered %s\n", i+1, maxWait, jittered)
	}
}
```

**Output**
```
retry 1: max 100ms, jittered 47.77941ms
retry 2: max 200ms, jittered 82.153551ms
retry 3: max 400ms, jittered 66.145821ms
retry 4: max 800ms, jittered 635.010051ms
retry 5: max 1.6s, jittered 287.113937ms
```

---

Next tier → [Medium (6–10)](2-medium.md)
