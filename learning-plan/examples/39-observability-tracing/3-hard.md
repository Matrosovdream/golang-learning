# 39 · Hard (11–15) — sampling, timing, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Head sampling

Decide at the start whether to keep a trace, from its trace id. It's deterministic — the same trace is
always sampled the same way, and a whole trace stays together.

```go
package main

import "fmt"

func sampled(traceID, ratioDenominator int) bool {
	return traceID%ratioDenominator == 0
}

func main() {
	// keep ~1 in 3 (trace id divisible by 3):
	for id := 1; id <= 6; id++ {
		fmt.Printf("trace %d: sampled=%v\n", id, sampled(id, 3))
	}
}
```

**Output**
```
trace 1: sampled=false
trace 2: sampled=false
trace 3: sampled=true
trace 4: sampled=false
trace 5: sampled=false
trace 6: sampled=true
```

---

## 12. Timing a span

Time a span with an injectable clock (a tick counter) so the demo is deterministic — no wall-clock.
Real spans record start/end timestamps.

```go
package main

import "fmt"

type Clock struct{ t int64 }

func (c *Clock) now() int64      { return c.t }
func (c *Clock) advance(d int64) { c.t += d }

type Span struct {
	name  string
	start int64
	end   int64
}

func main() {
	clk := &Clock{}
	s := Span{name: "db.query", start: clk.now()}
	clk.advance(72) // work takes 72 ticks
	s.end = clk.now()
	fmt.Printf("%s took %d ticks\n", s.name, s.end-s.start)
}
```

**Output**
```
db.query took 72 ticks
```

---

## 13. Span links

A span **link** connects related traces (e.g. a batch job to the messages that triggered it) when they
aren't in a strict parent/child relationship.

```go
package main

import "fmt"

type Span struct {
	traceID string
	links   []string // trace ids this span is linked to
}

func main() {
	batch := Span{traceID: "batch-1"}
	batch.links = []string{"msg-trace-A", "msg-trace-B", "msg-trace-C"}
	fmt.Println("batch trace:", batch.traceID)
	for _, l := range batch.links {
		fmt.Println("  linked to:", l)
	}
}
```

**Output**
```
batch trace: batch-1
  linked to: msg-trace-A
  linked to: msg-trace-B
  linked to: msg-trace-C
```

---

## 14. Low-cardinality attributes

Keep span attributes **low-cardinality**: use the route template, not the raw URL with an id.
High-cardinality attributes explode storage cost (and can leak PII).

```go
package main

import (
	"fmt"
	"strings"
)

func routeTemplate(path string) string {
	if strings.HasPrefix(path, "/orders/") {
		return "/orders/{id}"
	}
	return path
}

func main() {
	for _, p := range []string{"/orders/123", "/orders/456", "/orders/789"} {
		fmt.Printf("raw=%-14s attribute=%s\n", p, routeTemplate(p))
	}
	fmt.Println("→ 3 distinct URLs collapse to 1 low-cardinality attribute value")
}
```

**Output**
```
raw=/orders/123    attribute=/orders/{id}
raw=/orders/456    attribute=/orders/{id}
raw=/orders/789    attribute=/orders/{id}
→ 3 distinct URLs collapse to 1 low-cardinality attribute value
```

---

## 15. Capstone: a mini distributed trace

A gateway span, context propagated to a downstream service span, assembled into one trace tree, with
correlated logs — one trace_id across both services.

```go
package main

import (
	"context"
	"fmt"
	"strings"
)

type IDGen struct{ n int }

func (g *IDGen) next() int { g.n++; return g.n }

type Span struct {
	traceID  int
	spanID   int
	parentID int
	name     string
	service  string
}

var (
	ids   = &IDGen{}
	spans []Span
)

type ctxKey struct{}

func start(ctx context.Context, service, name string) (context.Context, Span) {
	s := Span{name: name, service: service}
	if p, ok := ctx.Value(ctxKey{}).(Span); ok {
		s.traceID = p.traceID
		s.parentID = p.spanID
	} else {
		s.traceID = ids.next()
	}
	s.spanID = ids.next()
	spans = append(spans, s)
	return context.WithValue(ctx, ctxKey{}, s), s
}

func logLine(s Span, msg string) {
	fmt.Printf("service=%s trace_id=%d msg=%q\n", s.service, s.traceID, msg)
}

func main() {
	// gateway receives a request → root span
	ctx, gw := start(context.Background(), "gateway", "POST /checkout")
	logLine(gw, "received request")

	// gateway calls the orders service; ctx propagates the trace across the hop
	_, ord := start(ctx, "orders", "CreateOrder")
	logLine(ord, "order created")

	fmt.Println(strings.Repeat("-", 30))
	for _, s := range spans {
		fmt.Printf("[trace %d] span %d parent %d %s/%s\n", s.traceID, s.spanID, s.parentID, s.service, s.name)
	}
}
```

**Output**
```
service=gateway trace_id=1 msg="received request"
service=orders trace_id=1 msg="order created"
------------------------------
[trace 1] span 2 parent 0 gateway/POST /checkout
[trace 1] span 3 parent 2 orders/CreateOrder
```

---

Back to [index](README.md) · Next lesson's examples: [40 — Testing Architecture](../40-testing-architecture/README.md).
