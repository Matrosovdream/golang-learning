# 31 · Hard (11–15) — factories, ACL, concurrency, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Factory for a valid aggregate

A **factory** builds a valid aggregate when construction is non-trivial — it guarantees invariants
and can record the aggregate's "birth" event.

```go
package main

import (
	"errors"
	"fmt"
)

type OrderCreated struct{ OrderID string }

type Order struct {
	id       string
	customer string
	status   string
	events   []any
}

func NewOrder(id, customer string) (*Order, error) {
	if customer == "" {
		return nil, errors.New("order: customer required")
	}
	o := &Order{id: id, customer: customer, status: "draft"}
	o.events = append(o.events, OrderCreated{OrderID: id})
	return o, nil
}

func main() {
	o, err := NewOrder("ord-1", "cust-7")
	fmt.Printf("order: id=%s status=%s err=%v\n", o.id, o.status, err)
	fmt.Printf("initial events: %v\n", o.events)
}
```

**Output**
```
order: id=ord-1 status=draft err=<nil>
initial events: [{ord-1}]
```

---

## 12. Derived data stays consistent

The aggregate keeps derived data consistent: an Order's total is **computed** from its line-item
value objects, never stored and left to drift.

```go
package main

import "fmt"

type Money struct{ cents int64 }

func (m Money) String() string { return fmt.Sprintf("$%d.%02d", m.cents/100, m.cents%100) }

type LineItem struct {
	name  string
	price Money
	qty   int
}

type Order struct{ items []LineItem }

func (o *Order) Add(name string, price Money, qty int) {
	o.items = append(o.items, LineItem{name, price, qty})
}

func (o *Order) Total() Money {
	var sum int64
	for _, it := range o.items {
		sum += it.price.cents * int64(it.qty)
	}
	return Money{cents: sum}
}

func main() {
	o := &Order{}
	o.Add("book", Money{1500}, 2)
	o.Add("pen", Money{250}, 3)
	fmt.Println("total:", o.Total()) // 2*1500 + 3*250 = 3750
}
```

**Output**
```
total: $37.50
```

---

## 13. Anti-corruption layer

An **anti-corruption layer** translates a foreign/legacy model into your clean domain model at the
boundary, so the outside vocabulary never leaks inward.

```go
package main

import "fmt"

type LegacyUser struct { // vendor shape we don't control
	USR_ID   string
	FullName string
	Flags    int // bit 1 = active
}

type Customer struct { // our clean domain model
	ID     string
	Name   string
	Active bool
}

func fromLegacy(u LegacyUser) Customer {
	return Customer{
		ID:     u.USR_ID,
		Name:   u.FullName,
		Active: u.Flags&1 == 1,
	}
}

func main() {
	legacy := LegacyUser{USR_ID: "u-42", FullName: "Alice Smith", Flags: 1}
	c := fromLegacy(legacy)
	fmt.Printf("%+v\n", c)
}
```

**Output**
```
{ID:u-42 Name:Alice Smith Active:true}
```

---

## 14. Optimistic concurrency (version)

Aggregates carry a **version** for optimistic concurrency: a save succeeds only if the caller's
expected version still matches the stored one, so a stale writer loses.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct {
	id      string
	total   int64
	version int
}

type repo struct{ m map[string]*Order }

func newRepo() *repo { return &repo{m: map[string]*Order{}} }

var ErrConflict = errors.New("optimistic lock: version conflict")

func (r *repo) Save(o *Order, expected int) error {
	cur, ok := r.m[o.id]
	if ok && cur.version != expected {
		return ErrConflict
	}
	o.version = expected + 1
	stored := *o
	r.m[o.id] = &stored
	return nil
}

func main() {
	r := newRepo()
	_ = r.Save(&Order{id: "ord-1", total: 100}, 0) // stored at version 1

	// two writers both loaded version 1:
	w1 := &Order{id: "ord-1", total: 150}
	w2 := &Order{id: "ord-1", total: 200}
	fmt.Println("writer1:", r.Save(w1, 1)) // wins → version 2
	fmt.Println("writer2:", r.Save(w2, 1)) // stale → conflict

	fmt.Println("final total:", r.m["ord-1"].total, "version:", r.m["ord-1"].version)
}
```

**Output**
```
writer1: <nil>
writer2: optimistic lock: version conflict
final total: 150 version: 2
```

---

## 15. Capstone: a small order domain

Everything together — a value object (Money), an aggregate with invariants, a domain event, a
repository, and an application flow — all framework-free (stdlib only), the way a DDD domain layer
should look.

```go
package main

import (
	"errors"
	"fmt"
)

// --- value object ---
type Money struct{ cents int64 }

func (m Money) String() string { return fmt.Sprintf("$%d.%02d", m.cents/100, m.cents%100) }

// --- domain event ---
type OrderPlaced struct {
	OrderID string
	Total   Money
}

// --- aggregate ---
type item struct {
	name  string
	price Money
	qty   int
}

type Order struct {
	id       string
	customer string
	status   string
	items    []item
	events   []any
}

func NewOrder(id, customer string) (*Order, error) {
	if customer == "" {
		return nil, errors.New("order: customer required")
	}
	return &Order{id: id, customer: customer, status: "draft"}, nil
}

func (o *Order) AddItem(name string, price Money, qty int) error {
	if o.status != "draft" {
		return errors.New("order: not draft")
	}
	if qty <= 0 {
		return errors.New("order: qty must be positive")
	}
	o.items = append(o.items, item{name, price, qty})
	return nil
}

func (o *Order) Total() Money {
	var sum int64
	for _, it := range o.items {
		sum += it.price.cents * int64(it.qty)
	}
	return Money{sum}
}

func (o *Order) Place() error {
	if len(o.items) == 0 {
		return errors.New("order: cannot place empty order")
	}
	o.status = "placed"
	o.events = append(o.events, OrderPlaced{OrderID: o.id, Total: o.Total()})
	return nil
}

// --- repository (domain interface) + in-memory implementation ---
type OrderRepository interface{ Save(*Order) error }

type memRepo struct{ m map[string]*Order }

func (r *memRepo) Save(o *Order) error { r.m[o.id] = o; return nil }

// --- application flow ---
func main() {
	var repo OrderRepository = &memRepo{m: map[string]*Order{}}

	o, _ := NewOrder("ord-1", "cust-7")
	_ = o.AddItem("book", Money{1500}, 2)
	_ = o.AddItem("pen", Money{250}, 1)
	if err := o.Place(); err != nil {
		fmt.Println("place failed:", err)
		return
	}
	_ = repo.Save(o)

	for _, e := range o.events { // dispatch after persistence
		if ev, ok := e.(OrderPlaced); ok {
			fmt.Printf("event: OrderPlaced id=%s total=%s\n", ev.OrderID, ev.Total)
		}
	}
	fmt.Printf("saved: id=%s status=%s total=%s\n", o.id, o.status, o.Total())
}
```

**Output**
```
event: OrderPlaced id=ord-1 total=$32.50
saved: id=ord-1 status=placed total=$32.50
```

---

Back to [index](README.md) · Next lesson's examples: [32 — Hexagonal / Ports & Adapters](../32-hexagonal-ports-adapters/README.md).
