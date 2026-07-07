# 31 · Medium (6–10) — aggregates, events, repositories

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Aggregate root guards an invariant

The **aggregate root** enforces a business invariant at all times — here, you cannot place an empty
order. Outside code must go through the root's methods.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct {
	status string
	items  []string
}

var errEmpty = errors.New("order: cannot place an empty order")

func (o *Order) Add(item string) { o.items = append(o.items, item) }

func (o *Order) Place() error {
	if len(o.items) == 0 {
		return errEmpty // invariant enforced at the root
	}
	o.status = "placed"
	return nil
}

func main() {
	empty := &Order{status: "draft"}
	fmt.Println("place empty:     ", empty.Place())

	o := &Order{status: "draft"}
	o.Add("book")
	fmt.Println("place with items:", o.Place(), "status:", o.status)
}
```

**Output**
```
place empty:      order: cannot place an empty order
place with items: <nil> status: placed
```

---

## 7. Domain events

A **domain event** records that something happened (past tense). The aggregate records events as it
changes; the application layer dispatches them after persistence.

```go
package main

import "fmt"

type OrderPlaced struct{ OrderID string }

type Order struct {
	id     string
	events []any
}

func (o *Order) Place()        { o.record(OrderPlaced{OrderID: o.id}) }
func (o *Order) record(e any)  { o.events = append(o.events, e) }
func (o *Order) Events() []any { return o.events }
func (o *Order) ClearEvents()  { o.events = nil }

func main() {
	o := &Order{id: "ord-1"}
	o.Place()

	for _, e := range o.Events() { // dispatched AFTER a successful save
		switch ev := e.(type) {
		case OrderPlaced:
			fmt.Println("dispatch: order placed:", ev.OrderID)
		}
	}
	o.ClearEvents()
	fmt.Println("events after clear:", len(o.Events()))
}
```

**Output**
```
dispatch: order placed: ord-1
events after clear: 0
```

---

## 8. Reference other aggregates by ID

Hold an **ID**, not a pointer, to another aggregate. An `Order` keeps a `CustomerID`, not a
`*Customer` — so each aggregate loads and locks independently.

```go
package main

import "fmt"

type CustomerID string

type Customer struct {
	id   CustomerID
	name string
}

type Order struct {
	id       string
	customer CustomerID // an id, NOT a *Customer
}

func main() {
	cust := Customer{id: "cust-7", name: "Alice"}
	o := Order{id: "ord-1", customer: cust.id}

	fmt.Println("order", o.id, "belongs to", o.customer)
	// Loading the full Customer is a separate concern (a different repository):
	fmt.Println("customer name resolved separately:", cust.name)
}
```

**Output**
```
order ord-1 belongs to cust-7
customer name resolved separately: Alice
```

---

## 9. Repository interface + in-memory impl

A **repository** is a collection-like interface for one aggregate root, **defined in the domain** and
implemented in infrastructure. The domain half imports nothing external.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct {
	id    string
	total int64
}

type OrderRepository interface {
	Save(o *Order) error
	Get(id string) (*Order, error)
}

var ErrNotFound = errors.New("order not found")

// --- infrastructure: an in-memory implementation of the domain interface ---
type memRepo struct{ m map[string]*Order }

func newMemRepo() *memRepo { return &memRepo{m: map[string]*Order{}} }

func (r *memRepo) Save(o *Order) error { r.m[o.id] = o; return nil }
func (r *memRepo) Get(id string) (*Order, error) {
	o, ok := r.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	return o, nil
}

func main() {
	var repo OrderRepository = newMemRepo() // program to the interface
	_ = repo.Save(&Order{id: "ord-1", total: 4200})

	o, _ := repo.Get("ord-1")
	fmt.Printf("loaded: %+v\n", *o)

	_, err := repo.Get("ord-x")
	fmt.Println("missing:", err)
}
```

**Output**
```
loaded: {id:ord-1 total:4200}
missing: order not found
```

---

## 10. Domain service across aggregates

A **domain service** holds logic that spans multiple aggregates and belongs to no single entity —
here, a transfer touching two Accounts.

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

func (a *Account) Withdraw(amt int64) error {
	if amt > a.balance {
		return errors.New("insufficient funds")
	}
	a.balance -= amt
	return nil
}
func (a *Account) Deposit(amt int64) { a.balance += amt }

type TransferService struct{}

func (TransferService) Transfer(from, to *Account, amt int64) error {
	if err := from.Withdraw(amt); err != nil {
		return err
	}
	to.Deposit(amt)
	return nil
}

func main() {
	a := &Account{id: "A", balance: 100}
	b := &Account{id: "B", balance: 0}
	svc := TransferService{}

	fmt.Println("transfer 60:  ", svc.Transfer(a, b, 60))
	fmt.Printf("A=%d B=%d\n", a.balance, b.balance)

	fmt.Println("transfer 1000:", svc.Transfer(a, b, 1000))
	fmt.Printf("A=%d B=%d\n", a.balance, b.balance)
}
```

**Output**
```
transfer 60:   <nil>
A=40 B=60
transfer 1000: insufficient funds
A=40 B=60
```

---

Next tier → [Hard (11–15)](3-hard.md)
