# 28 · Hard (13–17) — pool, prototype, advanced options

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 13. Object pool with sync.Pool

`sync.Pool` recycles short-lived allocations to cut GC pressure on hot paths. Pooled objects come out
**dirty** — always `Reset` before use — and must never be touched after `Put`.

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func render(name string) string {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset() // pooled objects are dirty
	defer bufPool.Put(buf)
	buf.WriteString("hello ")
	buf.WriteString(name)
	return buf.String()
}

func main() {
	fmt.Println(render("alice"))
	fmt.Println(render("bob"))
	fmt.Println(render("carol"))
}
```

**Output**
```
hello alice
hello bob
hello carol
```

> A `sync.Pool` may drop its contents at any GC, so it's a cache, **not** a fixed-size resource pool.
> For connections use a real pool (like `database/sql`'s).

---

## 14. Prototype: a correct deep Clone

When rebuilding is expensive but copying a template is cheap, give the type a `Clone()`. The catch:
plain assignment shares slices/maps/pointers, so you must deep-copy the reference-typed fields.

```go
package main

import "fmt"

type Graph struct {
	Nodes map[string][]string
}

func (g *Graph) Clone() *Graph {
	cp := &Graph{Nodes: make(map[string][]string, len(g.Nodes))}
	for k, edges := range g.Nodes {
		cp.Nodes[k] = append([]string(nil), edges...) // copy the slice too
	}
	return cp
}

func main() {
	g := &Graph{Nodes: map[string][]string{"a": {"b", "c"}}}
	gc := g.Clone()
	gc.Nodes["a"][0] = "Z" // mutate the clone
	gc.Nodes["d"] = []string{"e"}
	fmt.Println("original:", g.Nodes)
	fmt.Println("clone:   ", gc.Nodes)
}
```

**Output**
```
original: map[a:[b c]]
clone:    map[a:[Z c] d:[e]]
```

> Drop the inner `append([]string(nil), edges...)` (copy the map but share the slices) and mutating
> the clone would corrupt the original — the classic shallow-copy bug.

---

## 15. Self-referential option that returns an undo

Rob Pike's trick: each option **returns the option that undoes it**, so you can apply a change and
later restore the previous state — handy for scoped/temporary configuration.

```go
package main

import "fmt"

type Verbosity struct{ level int }

type Option func(*Verbosity) Option // returns the undo option

func Verbose(level int) Option {
	return func(v *Verbosity) Option {
		prev := v.level
		v.level = level
		return Verbose(prev) // applying this later restores prev
	}
}

func (v *Verbosity) Apply(opts ...Option) (undo Option) {
	for _, opt := range opts {
		undo = opt(v)
	}
	return undo
}

func main() {
	v := &Verbosity{level: 1}
	fmt.Println("start:     ", v.level)
	undo := v.Apply(Verbose(5))
	fmt.Println("after set: ", v.level)
	v.Apply(undo)
	fmt.Println("after undo:", v.level)
}
```

**Output**
```
start:      1
after set:  5
after undo: 1
```

---

## 16. Functional options on a generic type

Options work with generics too — the `Option` type carries the same type parameters as the target.
Call sites must spell the type arguments out, which is the price of generic options.

```go
package main

import (
	"fmt"
	"time"
)

type Cache[K comparable, V any] struct {
	ttl  time.Duration
	max  int
	data map[K]V
}

type Option[K comparable, V any] func(*Cache[K, V])

func WithTTL[K comparable, V any](d time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) { c.ttl = d }
}

func WithMax[K comparable, V any](n int) Option[K, V] {
	return func(c *Cache[K, V]) { c.max = n }
}

func NewCache[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{ttl: time.Minute, max: 100, data: make(map[K]V)}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func main() {
	c := NewCache[string, int](
		WithTTL[string, int](5*time.Second),
		WithMax[string, int](10),
	)
	fmt.Printf("ttl=%s max=%d\n", c.ttl, c.max)
}
```

**Output**
```
ttl=5s max=10
```

---

## 17. Capstone: a configurable HTTP client

Everything together — defaults, failable options, validation, and composition — to build two
independently configured clients from one constructor.

```go
package main

import (
	"errors"
	"fmt"
	"time"
)

type Client struct {
	baseURL string
	timeout time.Duration
	retries int
	headers map[string]string
}

type Option func(*Client) error

func WithTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return errors.New("timeout must be positive")
		}
		c.timeout = d
		return nil
	}
}

func WithRetries(n int) Option {
	return func(c *Client) error {
		if n < 0 {
			return errors.New("retries must be >= 0")
		}
		c.retries = n
		return nil
	}
}

func WithHeader(k, v string) Option {
	return func(c *Client) error { c.headers[k] = v; return nil }
}

func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("baseURL required")
	}
	c := &Client{
		baseURL: baseURL,
		timeout: 10 * time.Second,
		retries: 3,
		headers: map[string]string{"User-Agent": "ex/1.0"},
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Client) String() string {
	return fmt.Sprintf("%s timeout=%s retries=%d headers=%v",
		c.baseURL, c.timeout, c.retries, c.headers)
}

func main() {
	a, _ := NewClient("https://api.a.com")
	fmt.Println("A:", a)

	b, err := NewClient("https://api.b.com",
		WithTimeout(2*time.Second),
		WithRetries(5),
		WithHeader("Authorization", "Bearer xyz"),
	)
	fmt.Println("B:", b, "err:", err)

	_, err = NewClient("")
	fmt.Println("no url:", err)
}
```

**Output**
```
A: https://api.a.com timeout=10s retries=3 headers=map[User-Agent:ex/1.0]
B: https://api.b.com timeout=2s retries=5 headers=map[Authorization:Bearer xyz User-Agent:ex/1.0] err: <nil>
no url: baseURL required
```

---

Back to [index](README.md) · You've finished the Creational examples. Next lesson's examples:
[29 — Structural](../29-patterns-structural/README.md).
