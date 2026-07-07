# 32 · Hard (11–15) — full hexagon, translation, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. A full mini hexagon

One **driving** port the core implements, using two **driven** ports it declares. Adapters fill all
three at the composition root.

```go
package main

import (
	"errors"
	"fmt"
)

// ===== core: ports =====
type Order struct{ ID, Customer string }
type OrderRepo interface{ Save(Order) error }    // driven
type EventPublisher interface{ Publish(string) } // driven
type PlaceOrder interface {                      // driving
	Place(customer string) (string, error)
}

// ===== core: service implements the driving port using the driven ports =====
type service struct {
	repo   OrderRepo
	events EventPublisher
	seq    int
}

func (s *service) Place(customer string) (string, error) {
	if customer == "" {
		return "", errors.New("customer required")
	}
	s.seq++
	id := fmt.Sprintf("ord-%d", s.seq)
	if err := s.repo.Save(Order{ID: id, Customer: customer}); err != nil {
		return "", err
	}
	s.events.Publish("OrderPlaced:" + id)
	return id, nil
}

// ===== driven adapters =====
type memRepo struct{ saved []Order }

func (r *memRepo) Save(o Order) error { r.saved = append(r.saved, o); return nil }

type memBus struct{ events []string }

func (b *memBus) Publish(e string) { b.events = append(b.events, e) }

func main() {
	repo, bus := &memRepo{}, &memBus{}
	var uc PlaceOrder = &service{repo: repo, events: bus}
	id, _ := uc.Place("alice")
	fmt.Println("id:", id)
	fmt.Println("saved:", repo.saved)
	fmt.Println("events:", bus.events)
}
```

**Output**
```
id: ord-1
saved: [{ord-1 alice}]
events: [OrderPlaced:ord-1]
```

---

## 12. Swap a driven adapter for a decorated one

A logging wrapper that is itself a `Repo` slots in without the core noticing — an adapter over an
adapter (decorator from [lesson 29](../29-patterns-structural/README.md)).

```go
package main

import "fmt"

type Repo interface{ Save(id string) error }

type Service struct{ repo Repo }

func (s Service) Run(id string) { _ = s.repo.Save(id) }

type memRepo struct{ saved []string }

func (r *memRepo) Save(id string) error { r.saved = append(r.saved, id); return nil }

type loggingRepo struct {
	next Repo
	log  *[]string
}

func (r loggingRepo) Save(id string) error {
	*r.log = append(*r.log, "saving "+id)
	return r.next.Save(id)
}

func main() {
	base := &memRepo{}
	var log []string
	// swap the driven adapter for a decorated one — Service (the core) is unchanged:
	svc := Service{repo: loggingRepo{next: base, log: &log}}
	svc.Run("ord-1")
	fmt.Println("saved:", base.saved)
	fmt.Println("log:", log)
}
```

**Output**
```
saved: [ord-1]
log: [saving ord-1]
```

---

## 13. Error translation at the boundary

The adapter translates infrastructure errors into domain errors at the boundary, so the core never
imports or sees the infra error.

```go
package main

import (
	"errors"
	"fmt"
)

// The adapter TRANSLATES infrastructure errors into domain errors at the
// boundary, so the core never imports or sees the infra error.
var ErrUserNotFound = errors.New("user not found") // domain error

var errNoRows = errors.New("sql: no rows in result set") // infra error

type UserRepo interface {
	Get(id string) (string, error)
}

type sqlAdapter struct{ rows map[string]string }

func (a sqlAdapter) query(id string) (string, error) {
	name, ok := a.rows[id]
	if !ok {
		return "", errNoRows // the "DB" speaks its own error
	}
	return name, nil
}

func (a sqlAdapter) Get(id string) (string, error) {
	name, err := a.query(id)
	if errors.Is(err, errNoRows) {
		return "", ErrUserNotFound // translate at the boundary
	}
	return name, err
}

func lookup(r UserRepo, id string) {
	name, err := r.Get(id)
	if errors.Is(err, ErrUserNotFound) {
		fmt.Println(id, "-> not found (domain error)")
		return
	}
	fmt.Println(id, "->", name)
}

func main() {
	repo := sqlAdapter{rows: map[string]string{"1": "Alice"}}
	lookup(repo, "1")
	lookup(repo, "2")
}
```

**Output**
```
1 -> Alice
2 -> not found (domain error)
```

---

## 14. A failable dependency + a fake adapter

A driven port for an outbound dependency (a payment gateway). A fake adapter you can configure to
fail lets you test both the happy and sad paths of the core.

```go
package main

import (
	"errors"
	"fmt"
)

type PaymentGateway interface{ Charge(amount int64) error }

type checkout struct{ gw PaymentGateway }

func (c checkout) Buy(amount int64) string {
	if err := c.gw.Charge(amount); err != nil {
		return "declined: " + err.Error()
	}
	return "ok"
}

type fakeGateway struct{ fail bool }

func (g fakeGateway) Charge(amount int64) error {
	if g.fail {
		return errors.New("card declined")
	}
	return nil
}

func main() {
	fmt.Println("happy:", checkout{gw: fakeGateway{fail: false}}.Buy(1000))
	fmt.Println("sad:  ", checkout{gw: fakeGateway{fail: true}}.Buy(1000))
}
```

**Output**
```
happy: ok
sad:   declined: card declined
```

---

## 15. Capstone: HTTP → use case → repo + events

The whole hexagon end-to-end: an HTTP driving adapter calls the `PlaceOrder` use case, which uses
in-memory repo + event-bus driven adapters. Wired at the composition root and exercised with
`httptest`.

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// ===== core =====
type Order struct{ ID, Customer string }
type OrderRepo interface{ Save(Order) error }
type EventPublisher interface{ Publish(string) }
type PlaceOrder interface {
	Place(customer string) (string, error)
}

type service struct {
	repo   OrderRepo
	events EventPublisher
	seq    int
}

func (s *service) Place(customer string) (string, error) {
	if customer == "" {
		return "", errors.New("customer required")
	}
	s.seq++
	id := fmt.Sprintf("ord-%d", s.seq)
	_ = s.repo.Save(Order{ID: id, Customer: customer})
	s.events.Publish("OrderPlaced:" + id)
	return id, nil
}

// ===== driven adapters =====
type memRepo struct{ saved []Order }

func (r *memRepo) Save(o Order) error { r.saved = append(r.saved, o); return nil }

type memBus struct{ events []string }

func (b *memBus) Publish(e string) { b.events = append(b.events, e) }

// ===== driving adapter (HTTP) =====
type httpAdapter struct{ uc PlaceOrder }

func (h *httpAdapter) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Customer string `json:"customer"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id, err := h.uc.Place(body.Customer)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// ===== composition root =====
func main() {
	repo, bus := &memRepo{}, &memBus{}
	h := &httpAdapter{uc: &service{repo: repo, events: bus}}

	post := func(payload string) {
		req := httptest.NewRequest("POST", "/orders", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		h.create(rec, req)
		fmt.Printf("POST %s -> %d %s", payload, rec.Code, rec.Body.String())
	}
	post(`{"customer":"alice"}`)
	post(`{"customer":""}`)
	fmt.Println("repo saved:", repo.saved)
	fmt.Println("bus events:", bus.events)
}
```

**Output**
```
POST {"customer":"alice"} -> 201 {"id":"ord-1"}
POST {"customer":""} -> 400 {"error":"customer required"}
repo saved: [{ord-1 alice}]
bus events: [OrderPlaced:ord-1]
```

---

Back to [index](README.md) · Next lesson's examples: [33 — Dependency Injection & Wiring](../33-dependency-injection/README.md).
