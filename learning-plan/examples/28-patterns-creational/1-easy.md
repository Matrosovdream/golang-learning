# 28 · Easy (1–6) — zero value, constructors, options

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. The useful zero value (no constructor needed)

The best "creational pattern" is often designing a type whose zero value already works — then you
delete the constructor. `bytes.Buffer`, `strings.Builder`, and `sync.Mutex` all do this.

```go
package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
)

func main() {
	var buf bytes.Buffer // zero value, ready to use
	buf.WriteString("hello ")
	buf.WriteString("zero value")
	fmt.Println(buf.String())

	var b strings.Builder
	b.WriteString("no NewBuilder() needed")
	fmt.Println(b.String())

	var mu sync.Mutex // usable with no init
	mu.Lock()
	fmt.Println("locked & unlocked a zero-value mutex")
	mu.Unlock()
}
```

**Output**
```
hello zero value
no NewBuilder() needed
locked & unlocked a zero-value mutex
```

---

## 2. Constructor with validation → (T, error)

Go has no constructors; the convention is `NewT`. Validate invariants here and return `(*T, error)`
so an invalid value can never exist. Unexported fields stop callers corrupting it.

```go
package main

import (
	"errors"
	"fmt"
)

type Account struct {
	id      string
	balance int64
}

func NewAccount(id string, opening int64) (*Account, error) {
	if id == "" {
		return nil, errors.New("account: id required")
	}
	if opening < 0 {
		return nil, fmt.Errorf("account: negative opening balance %d", opening)
	}
	return &Account{id: id, balance: opening}, nil
}

func main() {
	a, err := NewAccount("acc-1", 100)
	fmt.Printf("ok: %+v err=%v\n", *a, err)

	_, err = NewAccount("", 100)
	fmt.Println("empty id:", err)

	_, err = NewAccount("acc-2", -5)
	fmt.Println("negative:", err)
}
```

**Output**
```
ok: {id:acc-1 balance:100} err=<nil>
empty id: account: id required
negative: account: negative opening balance -5
```

---

## 3. Constructor with sensible defaults

A constructor supplies defaults so callers get a working value with no configuration. Note it
returns a **concrete** `*Server` — *accept interfaces, return structs*.

```go
package main

import (
	"fmt"
	"time"
)

type Server struct {
	addr    string
	timeout time.Duration
	maxConn int
}

func NewServer(addr string) *Server {
	return &Server{addr: addr, timeout: 30 * time.Second, maxConn: 100}
}

func main() {
	s := NewServer("localhost:8080")
	fmt.Printf("addr=%s timeout=%s maxConn=%d\n", s.addr, s.timeout, s.maxConn)
}
```

**Output**
```
addr=localhost:8080 timeout=30s maxConn=100
```

---

## 4. Your first functional option

An `Option` is a function that mutates the target. The constructor applies defaults first, then each
option. Adding an option later never breaks existing callers.

```go
package main

import (
	"fmt"
	"time"
)

type Server struct {
	addr    string
	timeout time.Duration
}

type Option func(*Server)

func WithTimeout(d time.Duration) Option {
	return func(s *Server) { s.timeout = d }
}

func NewServer(addr string, opts ...Option) *Server {
	s := &Server{addr: addr, timeout: 30 * time.Second} // default
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func main() {
	a := NewServer("a")
	b := NewServer("b", WithTimeout(5*time.Second))
	fmt.Println("default:   ", a.timeout)
	fmt.Println("overridden:", b.timeout)
}
```

**Output**
```
default:    30s
overridden: 5s
```

---

## 5. Multiple options, applied in order

Options compose and apply left-to-right, so a later option overrides an earlier one.

```go
package main

import (
	"fmt"
	"time"
)

type Server struct {
	addr    string
	timeout time.Duration
	maxConn int
	tls     bool
}

type Option func(*Server)

func WithTimeout(d time.Duration) Option { return func(s *Server) { s.timeout = d } }
func WithMaxConn(n int) Option           { return func(s *Server) { s.maxConn = n } }
func WithTLS() Option                    { return func(s *Server) { s.tls = true } }

func NewServer(addr string, opts ...Option) *Server {
	s := &Server{addr: addr, timeout: 30 * time.Second, maxConn: 100}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func main() {
	s := NewServer("localhost", WithTimeout(2*time.Second), WithMaxConn(500), WithTLS())
	fmt.Printf("%s timeout=%s maxConn=%d tls=%v\n", s.addr, s.timeout, s.maxConn, s.tls)

	s2 := NewServer("x", WithMaxConn(10), WithMaxConn(20)) // last one wins
	fmt.Println("last wins:", s2.maxConn)
}
```

**Output**
```
localhost timeout=2s maxConn=500 tls=true
last wins: 20
```

---

## 6. An option that can fail

When an option must validate, use `Option func(*T) error` and stop at the first failure.

```go
package main

import (
	"fmt"
	"time"
)

type Server struct {
	timeout time.Duration
}

type Option func(*Server) error

func WithTimeout(d time.Duration) Option {
	return func(s *Server) error {
		if d <= 0 {
			return fmt.Errorf("timeout must be positive, got %s", d)
		}
		s.timeout = d
		return nil
	}
}

func NewServer(opts ...Option) (*Server, error) {
	s := &Server{timeout: 30 * time.Second}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func main() {
	s, err := NewServer(WithTimeout(5 * time.Second))
	fmt.Println("ok: ", s.timeout, err)

	_, err = NewServer(WithTimeout(-1))
	fmt.Println("bad:", err)
}
```

**Output**
```
ok:  5s <nil>
bad: timeout must be positive, got -1ns
```

---

Next tier → [Medium (7–12)](2-medium.md)
