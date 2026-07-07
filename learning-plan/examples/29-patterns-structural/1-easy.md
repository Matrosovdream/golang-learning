# 29 · Easy (1–6) — embedding & adapters

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Struct embedding: a promoted method

Embed a type and its fields and methods are *promoted* onto the outer type. This is composition
(has-a + delegation), not inheritance.

```go
package main

import "fmt"

type Logger struct{ prefix string }

func (l Logger) Log(msg string) { fmt.Println(l.prefix, msg) }

type Service struct {
	Logger // embedded
	name   string
}

func main() {
	s := Service{Logger: Logger{prefix: "[svc]"}, name: "orders"}
	s.Log("started") // promoted from the embedded Logger
	fmt.Println("service:", s.name, "prefix:", s.prefix)
}
```

**Output**
```
[svc] started
service: orders prefix: [svc]
```

---

## 2. Interface embedding (Reader + Writer)

A larger interface is the set-union of smaller ones — exactly how `io.ReadWriter` embeds `Reader`
and `Writer`. Keep interfaces small and compose.

```go
package main

import (
	"fmt"
	"strings"
)

type Reader interface{ Read() string }
type Writer interface{ Write(s string) }

type ReadWriter interface {
	Reader
	Writer
}

type buffer struct{ data string }

func (b *buffer) Read() string   { return b.data }
func (b *buffer) Write(s string) { b.data += s }

func main() {
	var rw ReadWriter = &buffer{}
	rw.Write("hello ")
	rw.Write("world")
	fmt.Println(rw.Read())
	fmt.Println(strings.ToUpper(rw.Read()))
}
```

**Output**
```
hello world
HELLO WORLD
```

---

## 3. Embed an interface to decorate one method

Embed an interface *value* and every method is promoted through it — override just the one you care
about, delegate the rest for free. This is the workhorse decorator idiom in Go.

```go
package main

import "fmt"

type Store interface {
	Get(key string) (string, error)
	Set(key, val string) error
}

type mapStore struct{ m map[string]string }

func (s *mapStore) Get(key string) (string, error) {
	v, ok := s.m[key]
	if !ok {
		return "", fmt.Errorf("not found: %s", key)
	}
	return v, nil
}
func (s *mapStore) Set(key, val string) error { s.m[key] = val; return nil }

type auditStore struct {
	Store // embedded interface = the wrapped store
	log   []string
}

func (a *auditStore) Set(key, val string) error {
	a.log = append(a.log, "set "+key)
	return a.Store.Set(key, val) // delegate to the wrapped store
}

// Get is promoted from the embedded Store unchanged — no code written.

func main() {
	base := &mapStore{m: map[string]string{}}
	a := &auditStore{Store: base}
	a.Set("x", "1")
	a.Set("y", "2")
	v, _ := a.Get("x") // promoted
	fmt.Println("x =", v)
	fmt.Println("audit:", a.log)
}
```

**Output**
```
x = 1
audit: [set x set y]
```

---

## 4. Adapter: a func satisfies an interface (HandlerFunc)

Give a function *type* the interface's method, and that method just calls itself. That's the entire
`http.HandlerFunc` trick — a function becomes an interface value.

```go
package main

import "fmt"

type Greeter interface{ Greet(name string) string }

type GreeterFunc func(string) string

func (f GreeterFunc) Greet(name string) string { return f(name) } // the whole adapter

func welcome(g Greeter, name string) { fmt.Println(g.Greet(name)) }

func main() {
	// Pass a bare function where a Greeter is expected, via the adapter:
	welcome(GreeterFunc(func(n string) string { return "hello " + n }), "alice")
	welcome(GreeterFunc(func(n string) string { return "hi " + n + "!" }), "bob")
}
```

**Output**
```
hello alice
hi bob!
```

---

## 5. Adapter: wrap a third-party struct

Wrap a vendor type so it satisfies *your* small interface — the rest of your code depends on `Cache`,
not on the vendor's API, so the vendor is swappable.

```go
package main

import "fmt"

type Cache interface {
	Get(key string) (string, bool)
}

type LegacyKV struct{ m map[string]string } // third-party, different API

func (kv *LegacyKV) Lookup(k string) (string, error) {
	v, ok := kv.m[k]
	if !ok {
		return "", fmt.Errorf("miss")
	}
	return v, nil
}

type kvAdapter struct{ kv *LegacyKV }

func (a kvAdapter) Get(key string) (string, bool) {
	v, err := a.kv.Lookup(key)
	return v, err == nil
}

func main() {
	legacy := &LegacyKV{m: map[string]string{"a": "1"}}
	var c Cache = kvAdapter{kv: legacy}
	v, ok := c.Get("a")
	fmt.Println("a:", v, ok)
	v, ok = c.Get("z")
	fmt.Println("z:", v, ok)
}
```

**Output**
```
a: 1 true
z:  false
```

---

## 6. Embedding + shadowing

If the outer type defines a method with the same name, it **shadows** the promoted one — but the
embedded version is still reachable through the embedded field. (Embedding delegates; it doesn't
override — the base never calls the outer's version. See lesson 30's Template-Method trap.)

```go
package main

import "fmt"

type Base struct{}

func (Base) Name() string { return "base" }

type Custom struct{ Base }

func (Custom) Name() string { return "custom" } // shadows the promoted Base.Name

func main() {
	c := Custom{}
	fmt.Println("outer:   ", c.Name())      // custom (shadows)
	fmt.Println("embedded:", c.Base.Name()) // base (still reachable)
}
```

**Output**
```
outer:    custom
embedded: base
```

---

Next tier → [Medium (7–12)](2-medium.md)
