# 39 · Medium (6–10) — trees, propagation, correlation

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. The span tree

A trace is a **tree** of spans. Nested work becomes child spans; printing the tree shows where a
request spent its time.

```go
package main

import (
	"fmt"
	"strings"
)

type Span struct {
	name     string
	children []*Span
}

func (s *Span) child(name string) *Span {
	c := &Span{name: name}
	s.children = append(s.children, c)
	return c
}

func printTree(s *Span, depth int) {
	fmt.Printf("%s%s\n", strings.Repeat("  ", depth), s.name)
	for _, c := range s.children {
		printTree(c, depth+1)
	}
}

func main() {
	root := &Span{name: "POST /checkout"}
	root.child("rpc catalog.Get")
	orders := root.child("rpc orders.Create")
	orders.child("db INSERT orders")
	root.child("publish OrderPlaced")
	printTree(root, 0)
}
```

**Output**
```
POST /checkout
  rpc catalog.Get
  rpc orders.Create
    db INSERT orders
  publish OrderPlaced
```

---

## 7. Context propagation in-process

The trace context rides in `ctx`. A function that drops `ctx` breaks the trace; passing it threads the
current span into nested calls.

```go
package main

import (
	"context"
	"fmt"
)

type ctxKey struct{}

func withSpan(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ctxKey{}, name)
}

func current(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return "(none)"
}

func inner(ctx context.Context) {
	fmt.Println("inner sees current span:", current(ctx))
}

func main() {
	ctx := context.Background()
	fmt.Println("before:", current(ctx))
	ctx = withSpan(ctx, "Checkout")
	inner(ctx) // ctx carries the span into the nested call
}
```

**Output**
```
before: (none)
inner sees current span: Checkout
```

---

## 8. Propagation across a hop (traceparent)

Across a process boundary the trace context travels in headers: inject it on the way out, extract it
on the way in. (W3C `traceparent`, simplified.)

```go
package main

import (
	"fmt"
	"strings"
)

type SpanContext struct {
	traceID string
	spanID  string
}

func inject(sc SpanContext) map[string]string {
	return map[string]string{"traceparent": sc.traceID + "-" + sc.spanID}
}

func extract(headers map[string]string) SpanContext {
	parts := strings.SplitN(headers["traceparent"], "-", 2)
	if len(parts) != 2 {
		return SpanContext{}
	}
	return SpanContext{traceID: parts[0], spanID: parts[1]}
}

func main() {
	out := SpanContext{traceID: "abc123", spanID: "01"} // gateway side
	headers := inject(out)
	fmt.Println("wire header:", headers["traceparent"])

	in := extract(headers) // downstream service side
	fmt.Printf("extracted: trace=%s parent-span=%s (same trace → connected)\n", in.traceID, in.spanID)
}
```

**Output**
```
wire header: abc123-01
extracted: trace=abc123 parent-span=01 (same trace → connected)
```

---

## 9. Correlate logs with trace_id

Put the trace_id in every log line so you can jump from a log to its trace and grep one request across
all services — the superpower that connects the pillars.

```go
package main

import "fmt"

type Logger struct{ traceID string }

func (l Logger) Info(msg string) {
	fmt.Printf("level=info trace_id=%s msg=%q\n", l.traceID, msg)
}

func main() {
	log := Logger{traceID: "abc123"}
	log.Info("checkout started")
	log.Info("payment captured")
}
```

**Output**
```
level=info trace_id=abc123 msg="checkout started"
level=info trace_id=abc123 msg="payment captured"
```

---

## 10. The three pillars

Metrics say **something** is slow; a trace says **where**; logs say **why**. You need all three.

```go
package main

import "fmt"

func main() {
	pillars := []struct{ name, answers string }{
		{"metrics", "how much / how fast, in aggregate"},
		{"traces", "where did THIS request spend its time"},
		{"logs", "what exactly happened at this moment"},
	}
	for _, p := range pillars {
		fmt.Printf("%-8s → %s\n", p.name, p.answers)
	}
}
```

**Output**
```
metrics  → how much / how fast, in aggregate
traces   → where did THIS request spend its time
logs     → what exactly happened at this moment
```

---

Next tier → [Hard (11–15)](3-hard.md)
