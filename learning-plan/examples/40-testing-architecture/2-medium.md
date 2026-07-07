# 40 · Medium (6–10) — technique

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Fake (outcome) vs mock (interaction)

Fakes assert **outcome**; mocks/spies assert **interaction**. A fake survives refactors; asserting
interaction is right only when the call pattern *is* the requirement.

```go
package main

import "fmt"

type counterFake struct{ n int }

func (c *counterFake) Inc() { c.n++ }

type callSpy struct{ calls int }

func (s *callSpy) Inc() { s.calls++ }

func bump(c interface{ Inc() }, times int) {
	for range times {
		c.Inc()
	}
}

func main() {
	fake := &counterFake{}
	bump(fake, 3)
	fmt.Println("fake asserts OUTCOME → final value:", fake.n)

	spy := &callSpy{}
	bump(spy, 3)
	fmt.Println("mock/spy asserts INTERACTION → call count:", spy.calls)
}
```

**Output**
```
fake asserts OUTCOME → final value: 3
mock/spy asserts INTERACTION → call count: 3
```

---

## 7. Table-driven tests

One struct slice of cases, one loop. This is how Go tests are written (shown here as a runnable
check).

```go
package main

import "fmt"

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func main() {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"positive", 5, 5},
		{"negative", -5, 5},
		{"zero", 0, 0},
	}
	for _, c := range cases {
		got := abs(c.in)
		status := "PASS"
		if got != c.want {
			status = "FAIL"
		}
		fmt.Printf("%-9s abs(%d)=%d want %d → %s\n", c.name, c.in, got, c.want, status)
	}
}
```

**Output**
```
positive  abs(5)=5 want 5 → PASS
negative  abs(-5)=5 want 5 → PASS
zero      abs(0)=0 want 0 → PASS
```

---

## 8. Injected clock

Inject a `Clock` so time-dependent code is deterministic — never call `time.Now()` deep in code under
test.

```go
package main

import "fmt"

type Clock interface{ Now() int64 }

type fixedClock struct{ t int64 }

func (c fixedClock) Now() int64 { return c.t }

type Token struct {
	value   string
	expires int64
}

func issue(clock Clock, ttl int64) Token {
	return Token{value: "tok", expires: clock.Now() + ttl}
}

func main() {
	tok := issue(fixedClock{t: 1000}, 60)
	fmt.Printf("token expires at %d (deterministic)\n", tok.expires)
}
```

**Output**
```
token expires at 1060 (deterministic)
```

---

## 9. httptest handler

`httptest` drives an HTTP handler in-process — no real socket, deterministic.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func main() {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	healthz(rec, req)
	fmt.Println("status:", rec.Code)
	fmt.Println("body:", rec.Body.String())
}
```

**Output**
```
status: 200
body: ok
```

---

## 10. Golden files

A golden-file test compares output against a checked-in expected file. Here we write the golden to a
temp dir, then compare (a `-update` flag would refresh it). Great for large, stable outputs.

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func render() string { return `{"id":"ord-1","total":3250}` }

func main() {
	dir, _ := os.MkdirTemp("", "golden")
	defer os.RemoveAll(dir)
	golden := filepath.Join(dir, "order.golden")

	_ = os.WriteFile(golden, []byte(render()), 0o644) // first run "updates" the golden

	want, _ := os.ReadFile(golden) // a later run compares
	if render() == string(want) {
		fmt.Println("golden match: PASS")
	} else {
		fmt.Println("golden match: FAIL")
	}
}
```

**Output**
```
golden match: PASS
```

---

Next tier → [Hard (11–15)](3-hard.md)
