# 33 · Easy (1–5) — constructor injection & the root

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Constructor injection

DI in Go is just: a component **receives** its dependencies instead of creating them. No framework.

```go
package main

import "fmt"

type Mailer interface{ Send(to string) string }

type smtp struct{}

func (smtp) Send(to string) string { return "sent to " + to }

type Service struct{ mailer Mailer }

func NewService(m Mailer) *Service { return &Service{mailer: m} }

func (s *Service) Welcome(user string) string { return s.mailer.Send(user) }

func main() {
	svc := NewService(smtp{}) // dependency comes IN
	fmt.Println(svc.Welcome("alice@x.com"))
}
```

**Output**
```
sent to alice@x.com
```

---

## 2. A testable constructor

A constructor that *takes* its dependency (rather than building one internally) is testable:
production injects a real clock; a test injects a fake for deterministic output.

```go
package main

import "fmt"

type Clock interface{ Now() int }

type Report struct{ clock Clock }

func NewReport(c Clock) *Report { return &Report{clock: c} }

func (r *Report) Line() string { return fmt.Sprintf("t=%d ready", r.clock.Now()) }

type fixedClock struct{ t int }

func (f fixedClock) Now() int { return f.t }

func main() {
	r := NewReport(fixedClock{t: 100}) // a test injects a fake clock
	fmt.Println(r.Line())
}
```

**Output**
```
t=100 ready
```

---

## 3. Inject an interface, swap real/fake

Inject an interface and the same code runs against a real or a fake implementation — the reason DI
aids testing.

```go
package main

import "fmt"

type Store interface{ Count() int }

type realStore struct{}

func (realStore) Count() int { return 42 }

type fakeStore struct{ n int }

func (f fakeStore) Count() int { return f.n }

func summary(s Store) string { return fmt.Sprintf("items: %d", s.Count()) }

func main() {
	fmt.Println("prod:", summary(realStore{}))
	fmt.Println("test:", summary(fakeStore{n: 3}))
}
```

**Output**
```
prod: items: 42
test: items: 3
```

---

## 4. The composition root

`main` builds the graph **leaf → root**: things with no dependencies first, then the things that
depend on them.

```go
package main

import "fmt"

type Repo interface{ Load() string }

type memRepo struct{}

func (memRepo) Load() string { return "data" }

type Service struct{ repo Repo }

func NewService(r Repo) *Service { return &Service{repo: r} }

func (s *Service) Do() string { return "processed " + s.repo.Load() }

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Handle() string { return "[200] " + h.svc.Do() }

func main() {
	repo := memRepo{}       // leaf
	svc := NewService(repo) // depends on repo
	h := NewHandler(svc)    // depends on svc
	fmt.Println(h.Handle())
}
```

**Output**
```
[200] processed data
```

---

## 5. Several dependencies

Several dependencies are just several parameters; the signature declares exactly what the service
needs.

```go
package main

import "fmt"

type Repo interface{ Get() string }
type Logger interface{ Log(string) }

type memRepo struct{}

func (memRepo) Get() string { return "row" }

type sliceLogger struct{ lines *[]string }

func (l sliceLogger) Log(s string) { *l.lines = append(*l.lines, s) }

type Service struct {
	repo Repo
	log  Logger
}

func NewService(r Repo, l Logger) *Service { return &Service{repo: r, log: l} }

func (s *Service) Run() string {
	s.log.Log("running")
	return s.repo.Get()
}

func main() {
	var lines []string
	svc := NewService(memRepo{}, sliceLogger{lines: &lines})
	fmt.Println("result:", svc.Run())
	fmt.Println("logs:", lines)
}
```

**Output**
```
result: row
logs: [running]
```

---

Next tier → [Medium (6–10)](2-medium.md)
