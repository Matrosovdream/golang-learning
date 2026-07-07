# 29 · Hard (13–17) — composite, bridge, flyweight

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 13. Composite: a file tree

Leaves (`File`) and containers (`Dir`) both satisfy one interface, so a client treats them uniformly;
a container just recurses over its children.

```go
package main

import "fmt"

type Node interface {
	Name() string
	Size() int64
}

type File struct {
	name string
	size int64
}

func (f File) Name() string { return f.name }
func (f File) Size() int64  { return f.size }

type Dir struct {
	name     string
	children []Node
}

func (d Dir) Name() string { return d.name }
func (d Dir) Size() int64 {
	var total int64
	for _, c := range d.children {
		total += c.Size() // uniform call — leaf or subtree alike
	}
	return total
}

func main() {
	tree := Dir{
		name: "root",
		children: []Node{
			File{"a.txt", 100},
			Dir{name: "sub", children: []Node{
				File{"b.txt", 200},
				File{"c.txt", 50},
			}},
		},
	}
	fmt.Printf("%s total size = %d\n", tree.Name(), tree.Size())
}
```

**Output**
```
root total size = 350
```

---

## 14. Bridge: swap the implementation

Split abstraction (`Notifier`) from implementation (`Sender`) via an interface, so each varies
independently — add a sender without touching `Notifier`, add a method without touching senders.

```go
package main

import "fmt"

type Sender interface{ Send(to, msg string) string }

type Notifier struct{ send Sender }

func (n Notifier) Alert(to string) string   { return n.send.Send(to, "ALERT") }
func (n Notifier) Welcome(to string) string { return n.send.Send(to, "welcome") }

type Email struct{}

func (Email) Send(to, msg string) string { return fmt.Sprintf("email→%s: %s", to, msg) }

type SMS struct{}

func (SMS) Send(to, msg string) string { return fmt.Sprintf("sms→%s: %s", to, msg) }

func main() {
	fmt.Println(Notifier{send: Email{}}.Alert("alice"))
	fmt.Println(Notifier{send: SMS{}}.Welcome("bob")) // swap impl, same Notifier
}
```

**Output**
```
email→alice: ALERT
sms→bob: welcome
```

---

## 15. Flyweight: string interning

Share one immutable copy of a repeated value instead of holding many identical copies. Because the
shared value is immutable, sharing is safe.

```go
package main

import "fmt"

type intern struct{ pool map[string]string }

func newIntern() *intern { return &intern{pool: map[string]string{}} }

func (in *intern) get(s string) string {
	if v, ok := in.pool[s]; ok {
		return v // reuse the existing copy
	}
	in.pool[s] = s
	return s
}

func main() {
	in := newIntern()
	words := []string{"go", "go", "rust", "go", "rust"}
	for _, w := range words {
		in.get(w)
	}
	fmt.Println("saw", len(words), "words,", len(in.pool), "distinct")
}
```

**Output**
```
saw 5 words, 2 distinct
```

---

## 16. Adapter to io.Writer

`prefixWriter` wraps an `io.Writer` and prefixes each write. Because it *is* an `io.Writer`,
`fmt.Fprintf` can target it directly — the composability of small interfaces.

```go
package main

import (
	"fmt"
	"io"
	"strings"
)

type prefixWriter struct {
	prefix string
	w      io.Writer
}

func (p prefixWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, p.prefix); err != nil {
		return 0, err
	}
	return p.w.Write(b)
}

func main() {
	var sb strings.Builder
	pw := prefixWriter{prefix: "> ", w: &sb}
	fmt.Fprintf(pw, "hello %s\n", "world")
	fmt.Fprintf(pw, "line two\n")
	fmt.Print(sb.String())
}
```

**Output**
```
> hello world
> line two
```

---

## 17. Capstone: layered store decorators

Stack decorators/proxies on one `Store` interface: logging wraps a caching proxy wraps the base.
Logging sees every call; caching stops repeats from ever reaching the base.

```go
package main

import (
	"fmt"
	"strings"
)

type Store interface{ Get(key string) string }

type base struct{ calls *int }

func (b base) Get(key string) string { *b.calls++; return "v:" + key }

type caching struct {
	next  Store
	cache map[string]string
}

func (c caching) Get(key string) string {
	if v, ok := c.cache[key]; ok {
		return v
	}
	v := c.next.Get(key)
	c.cache[key] = v
	return v
}

type logging struct {
	next Store
	log  *[]string
}

func (l logging) Get(key string) string {
	*l.log = append(*l.log, "get "+key)
	return l.next.Get(key)
}

func main() {
	calls := 0
	var log []string
	var s Store = logging{
		next: caching{next: base{calls: &calls}, cache: map[string]string{}},
		log:  &log,
	}
	fmt.Println(s.Get("x"))
	fmt.Println(s.Get("x")) // cache hit — base NOT called again
	fmt.Println(s.Get("y"))
	fmt.Println("base calls:", calls)            // 2
	fmt.Println("log:", strings.Join(log, ", ")) // all 3 logged
}
```

**Output**
```
v:x
v:x
v:y
base calls: 2
log: get x, get x, get y
```

---

Back to [index](README.md) · Next lesson's examples: [30 — Behavioral](../30-patterns-behavioral/README.md).
