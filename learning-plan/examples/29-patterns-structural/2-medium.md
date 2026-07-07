# 29 · Medium (7–12) — decorators, facade, proxy

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 7. Function decorator

Wrap a function to add behaviour while keeping the same signature, so the decorated and undecorated
versions are interchangeable.

```go
package main

import "fmt"

type Op func(int) int

func withLog(name string, op Op) Op {
	return func(n int) int {
		out := op(n)
		fmt.Printf("%s(%d) = %d\n", name, n, out)
		return out
	}
}

func main() {
	double := func(n int) int { return n * 2 }
	d := withLog("double", double)
	fmt.Println("result:", d(21))
}
```

**Output**
```
double(21) = 42
result: 42
```

---

## 8. Middleware chain over http.Handler

Middleware is a decorator over `http.Handler`. `Chain` wraps so the **first listed is outermost**
(first in, last out) — watch the enter/exit order.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

type Middleware func(http.Handler) http.Handler

func tag(name string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("enter", name)
			next.ServeHTTP(w, r)
			fmt.Println("exit ", name)
		})
	}
}

func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func main() {
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("handler")
		fmt.Fprint(w, "ok")
	})
	h := Chain(final, tag("A"), tag("B")) // A is outermost
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	fmt.Println("body:", rec.Body.String())
}
```

**Output**
```
enter A
enter B
handler
exit  B
exit  A
body: ok
```

---

## 9. Interface decorators that stack

Each wrapper satisfies `Store` and wraps a `Store`, so you can layer behaviour in any order —
here, uppercasing the result of a logging layer over the base.

```go
package main

import (
	"fmt"
	"strings"
)

type Store interface{ Get(key string) string }

type base struct{}

func (base) Get(key string) string { return "value-of-" + key }

type logging struct {
	next Store
	log  *[]string
}

func (l logging) Get(key string) string {
	*l.log = append(*l.log, "get "+key)
	return l.next.Get(key)
}

type upper struct{ next Store }

func (u upper) Get(key string) string { return strings.ToUpper(u.next.Get(key)) }

func main() {
	var log []string
	// upper wraps logging wraps base — one Store type throughout:
	var s Store = upper{next: logging{next: base{}, log: &log}}
	fmt.Println(s.Get("x"))
	fmt.Println("log:", log)
}
```

**Output**
```
VALUE-OF-X
log: [get x]
```

---

## 10. Facade over a subsystem

A facade exposes one simple method over several collaborators, so callers don't wire the subsystem.
(Your clean-architecture service layer is a facade over repositories and clients.)

```go
package main

import "fmt"

type users struct{ created []string }

func (u *users) create(email string) { u.created = append(u.created, email) }

type mailer struct{ sent []string }

func (m *mailer) welcome(email string) { m.sent = append(m.sent, email) }

type events struct{ published []string }

func (e *events) publish(name string) { e.published = append(e.published, name) }

type Signup struct {
	users  *users
	mailer *mailer
	events *events
}

func (s Signup) Register(email string) {
	s.users.create(email)
	s.mailer.welcome(email)
	s.events.publish("UserRegistered")
}

func main() {
	u, m, e := &users{}, &mailer{}, &events{}
	svc := Signup{users: u, mailer: m, events: e}
	svc.Register("a@x.com")
	fmt.Println("created:", u.created)
	fmt.Println("mailed: ", m.sent)
	fmt.Println("events: ", e.published)
}
```

**Output**
```
created: [a@x.com]
mailed:  [a@x.com]
events:  [UserRegistered]
```

---

## 11. Caching proxy

A proxy shares the real object's interface but interposes control — here, caching so the expensive
underlying call runs once per key.

```go
package main

import "fmt"

type Store interface{ Get(key string) string }

type slowStore struct{ calls int }

func (s *slowStore) Get(key string) string {
	s.calls++
	return "value-of-" + key
}

type cachingProxy struct {
	next  Store
	cache map[string]string
}

func newCachingProxy(next Store) *cachingProxy {
	return &cachingProxy{next: next, cache: map[string]string{}}
}

func (p *cachingProxy) Get(key string) string {
	if v, ok := p.cache[key]; ok {
		return v // served from cache; real store untouched
	}
	v := p.next.Get(key)
	p.cache[key] = v
	return v
}

func main() {
	real := &slowStore{}
	var s Store = newCachingProxy(real)
	fmt.Println(s.Get("x"))
	fmt.Println(s.Get("x")) // cache hit
	fmt.Println(s.Get("y"))
	fmt.Println("underlying calls:", real.calls) // 2, not 3
}
```

**Output**
```
value-of-x
value-of-x
value-of-y
underlying calls: 2
```

> Decorator vs proxy is *intent*: a decorator adds behaviour; a proxy controls access to the real
> thing. Mechanically they're the same wrap-and-delegate.

---

## 12. Protection proxy

Same interface, but the proxy guards access before delegating.

```go
package main

import (
	"errors"
	"fmt"
)

type Doc interface{ Read() (string, error) }

type secretDoc struct{ text string }

func (d secretDoc) Read() (string, error) { return d.text, nil }

type protectedDoc struct {
	next Doc
	role string
}

func (p protectedDoc) Read() (string, error) {
	if p.role != "admin" {
		return "", errors.New("access denied")
	}
	return p.next.Read()
}

func main() {
	doc := secretDoc{text: "launch codes"}
	admin := protectedDoc{next: doc, role: "admin"}
	guest := protectedDoc{next: doc, role: "guest"}
	fmt.Println(admin.Read())
	fmt.Println(guest.Read())
}
```

**Output**
```
launch codes <nil>
 access denied
```

---

Next tier → [Hard (13–17)](3-hard.md)
