# 39 · Easy (1–5) — spans, ids, attributes

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. A span

A minimal in-memory tracer standing in for OpenTelemetry: it records spans so you can see them. A span
is one unit of work with a start and an end.

```go
package main

import "fmt"

type Span struct{ name string }

type Tracer struct{ spans []string }

func (t *Tracer) Start(name string) *Span { return &Span{name: name} }
func (t *Tracer) End(s *Span)             { t.spans = append(t.spans, s.name) }

func main() {
	tr := &Tracer{}
	s := tr.Start("HandleRequest")
	// ... do work ...
	tr.End(s)
	fmt.Println("recorded spans:", tr.spans)
}
```

**Output**
```
recorded spans: [HandleRequest]
```

---

## 2. Trace & span ids

Each span carries a trace_id (shared across the whole request) and its own span_id. IDs are
counter-based here so the demo is deterministic.

```go
package main

import "fmt"

type IDGen struct{ n int }

func (g *IDGen) next() int { g.n++; return g.n }

type Span struct {
	traceID int
	spanID  int
	name    string
}

func main() {
	ids := &IDGen{}
	traceID := ids.next() // one trace id for the whole request
	a := Span{traceID: traceID, spanID: ids.next(), name: "A"}
	b := Span{traceID: traceID, spanID: ids.next(), name: "B"}
	fmt.Printf("%s: trace=%d span=%d\n", a.name, a.traceID, a.spanID)
	fmt.Printf("%s: trace=%d span=%d\n", b.name, b.traceID, b.spanID)
}
```

**Output**
```
A: trace=1 span=2
B: trace=1 span=3
```

---

## 3. Parent/child via context

Parent/child spans are wired through context: a child inherits the parent's trace_id and records the
parent's span_id.

```go
package main

import (
	"context"
	"fmt"
)

type Span struct {
	traceID  int
	spanID   int
	parentID int
	name     string
}

type ctxKey struct{}

type IDGen struct{ n int }

func (g *IDGen) next() int { g.n++; return g.n }

var ids = &IDGen{}

func startSpan(ctx context.Context, name string) (context.Context, Span) {
	s := Span{name: name}
	if parent, ok := ctx.Value(ctxKey{}).(Span); ok {
		s.traceID = parent.traceID
		s.parentID = parent.spanID
	} else {
		s.traceID = ids.next() // root: a new trace
	}
	s.spanID = ids.next()
	return context.WithValue(ctx, ctxKey{}, s), s
}

func main() {
	ctx := context.Background()
	ctx, root := startSpan(ctx, "root")
	_, child := startSpan(ctx, "child")
	fmt.Printf("root:  trace=%d span=%d parent=%d\n", root.traceID, root.spanID, root.parentID)
	fmt.Printf("child: trace=%d span=%d parent=%d\n", child.traceID, child.spanID, child.parentID)
}
```

**Output**
```
root:  trace=1 span=2 parent=0
child: trace=1 span=3 parent=2
```

---

## 4. Span attributes

Spans carry attributes (key/value context). Keep them meaningful and low-cardinality (see example 14).

```go
package main

import "fmt"

type Span struct {
	name  string
	attrs map[string]string
}

func newSpan(name string) *Span { return &Span{name: name, attrs: map[string]string{}} }

func (s *Span) SetAttr(k, v string) *Span { s.attrs[k] = v; return s }

func main() {
	s := newSpan("Checkout")
	s.SetAttr("http.method", "POST").SetAttr("http.route", "/checkout")
	fmt.Println("span:", s.name)
	for _, k := range []string{"http.method", "http.route"} { // stable order
		fmt.Printf("  %s = %s\n", k, s.attrs[k])
	}
}
```

**Output**
```
span: Checkout
  http.method = POST
  http.route = /checkout
```

---

## 5. Span status & errors

A span has a status; on failure you record the error and mark it, so error spans stand out in the
trace.

```go
package main

import (
	"errors"
	"fmt"
)

type Span struct {
	name   string
	status string
	err    string
}

func (s *Span) RecordError(err error) {
	s.status = "error"
	s.err = err.Error()
}

func main() {
	ok := &Span{name: "A", status: "ok"}
	bad := &Span{name: "B", status: "ok"}
	bad.RecordError(errors.New("charge failed"))
	fmt.Printf("%s: status=%s\n", ok.name, ok.status)
	fmt.Printf("%s: status=%s err=%q\n", bad.name, bad.status, bad.err)
}
```

**Output**
```
A: status=ok
B: status=error err="charge failed"
```

---

Next tier → [Medium (6–10)](2-medium.md)
