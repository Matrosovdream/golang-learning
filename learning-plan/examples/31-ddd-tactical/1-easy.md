# 31 · Easy (1–5) — value objects & entities

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. A value object: Money

A **value object** is defined entirely by its attributes and is **immutable**: construct it validated,
and "change" it by returning a new one. Money uses integer minor units (cents), never a float.

```go
package main

import (
	"errors"
	"fmt"
)

type Money struct {
	cents    int64
	currency string
}

func NewMoney(cents int64, currency string) (Money, error) {
	if currency == "" {
		return Money{}, errors.New("money: currency required")
	}
	return Money{cents: cents, currency: currency}, nil
}

func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, fmt.Errorf("currency mismatch: %s vs %s", m.currency, o.currency)
	}
	return Money{cents: m.cents + o.cents, currency: m.currency}, nil // a NEW value
}

func (m Money) String() string {
	return fmt.Sprintf("%d.%02d %s", m.cents/100, m.cents%100, m.currency)
}

func main() {
	a, _ := NewMoney(1050, "USD")
	b, _ := NewMoney(295, "USD")
	sum, _ := a.Add(b)
	fmt.Println("a:", a, "b:", b, "sum:", sum)
	fmt.Println("a unchanged:", a) // Add returned a new value; a is untouched

	_, err := NewMoney(100, "")
	fmt.Println("no currency:", err)

	eur, _ := NewMoney(100, "EUR")
	_, err = a.Add(eur)
	fmt.Println("mismatch:", err)
}
```

**Output**
```
a: 10.50 USD b: 2.95 USD sum: 13.45 USD
a unchanged: 10.50 USD
no currency: money: currency required
mismatch: currency mismatch: USD vs EUR
```

---

## 2. Value objects compare by value

A struct of comparable fields is `==`-able and usable as a map key — no `Equals()` method needed.
That's why value objects are cheap to share.

```go
package main

import "fmt"

type Currency struct{ code string }

type Money struct {
	cents    int64
	currency Currency
}

func main() {
	a := Money{1000, Currency{"USD"}}
	b := Money{1000, Currency{"USD"}}
	c := Money{2000, Currency{"USD"}}

	fmt.Println("a == b:", a == b) // true — equal by value
	fmt.Println("a == c:", a == c) // false

	seen := map[Money]bool{a: true} // comparable → valid map key
	fmt.Println("b seen:", seen[b]) // true — b has the same value as a
}
```

**Output**
```
a == b: true
a == c: false
b seen: true
```

---

## 3. Entity identity

An **entity** has a stable identity that outlives its attributes: two entities are the same when
their IDs match, even if every other field differs.

```go
package main

import "fmt"

type Order struct {
	id     string
	status string
}

func (o Order) ID() string { return o.id }

func sameEntity(a, b Order) bool { return a.ID() == b.ID() }

func main() {
	o1 := Order{id: "ord-1", status: "draft"}
	o2 := Order{id: "ord-1", status: "placed"} // same id, different status
	o3 := Order{id: "ord-2", status: "draft"}

	fmt.Println("o1 & o2 same entity:", sameEntity(o1, o2)) // true — identity
	fmt.Println("o1 & o3 same entity:", sameEntity(o1, o3)) // false
	fmt.Println("o1 == o2 (by value): ", o1 == o2)          // false — fields differ
}
```

**Output**
```
o1 & o2 same entity: true
o1 & o3 same entity: false
o1 == o2 (by value):  false
```

---

## 4. A constructor enforces invariants

A constructor validates invariants so an invalid entity can never be built; unexported fields stop
callers bypassing the check.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct {
	id       string
	customer string
	status   string
}

func NewOrder(id, customer string) (*Order, error) {
	if id == "" {
		return nil, errors.New("order: id required")
	}
	if customer == "" {
		return nil, errors.New("order: customer required")
	}
	return &Order{id: id, customer: customer, status: "draft"}, nil
}

func main() {
	o, err := NewOrder("ord-1", "cust-7")
	fmt.Printf("ok: %+v err=%v\n", *o, err)

	_, err = NewOrder("ord-2", "")
	fmt.Println("no customer:", err)
}
```

**Output**
```
ok: {id:ord-1 customer:cust-7 status:draft} err=<nil>
no customer: order: customer required
```

---

## 5. Rich vs anemic: behaviour on the entity

Put behaviour **on** the entity (a rich model), not in a separate "service" that mutates dumb structs
(an anemic model). The method is where the invariant is guarded.

```go
package main

import (
	"errors"
	"fmt"
)

type LineItem struct {
	product string
	qty     int
}

type Order struct {
	status string
	items  []LineItem
}

var errNotDraft = errors.New("order: not in draft")

func (o *Order) AddItem(product string, qty int) error {
	if o.status != "draft" {
		return errNotDraft
	}
	if qty <= 0 {
		return fmt.Errorf("qty must be positive, got %d", qty)
	}
	o.items = append(o.items, LineItem{product, qty})
	return nil
}

func main() {
	o := &Order{status: "draft"}
	fmt.Println("add 2:           ", o.AddItem("book", 2))
	fmt.Println("add -1:          ", o.AddItem("pen", -1))
	o.status = "placed"
	fmt.Println("add after placed:", o.AddItem("mug", 1))
	fmt.Println("items:", o.items)
}
```

**Output**
```
add 2:            <nil>
add -1:           qty must be positive, got -1
add after placed: order: not in draft
items: [{book 2}]
```

---

Next tier → [Medium (6–10)](2-medium.md)
