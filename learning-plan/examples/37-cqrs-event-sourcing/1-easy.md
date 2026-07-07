# 37 · Easy (1–5) — CQRS

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. CQRS: separate write & read models

The write side takes commands (rich, validated); the read side serves queries (denormalized, fast).
Different models, possibly different stores.

```go
package main

import "fmt"

type WriteModel struct{ orders map[string]int64 }
type ReadModel struct{ summaries map[string]string }

func main() {
	w := &WriteModel{orders: map[string]int64{}}
	r := &ReadModel{summaries: map[string]string{}}

	w.orders["ord-1"] = 3250                     // command → write model
	r.summaries["ord-1"] = "Order ord-1: $32.50" // projected into the read model

	fmt.Println("write side:", w.orders["ord-1"])    // query the write store
	fmt.Println("read side: ", r.summaries["ord-1"]) // query the read store
}
```

**Output**
```
write side: 3250
read side:  Order ord-1: $32.50
```

---

## 2. CQRS-lite: one store, a read view

The common version: **one** database, separate read paths. Writes go through the domain; reads use a
read-optimized view — no second store.

```go
package main

import "fmt"

type Order struct {
	id       string
	customer string
	cents    int64
}

type OrderView struct {
	ID    string
	Total string
}

func toView(o Order) OrderView {
	return OrderView{ID: o.id, Total: fmt.Sprintf("$%d.%02d", o.cents/100, o.cents%100)}
}

func main() {
	o := Order{id: "ord-1", customer: "alice", cents: 3250}
	fmt.Printf("view: %+v\n", toView(o))
}
```

**Output**
```
view: {ID:ord-1 Total:$32.50}
```

---

## 3. A command handler

The command side runs domain logic and invariants, then persists.

```go
package main

import (
	"errors"
	"fmt"
)

type PlaceOrder struct {
	Customer string
	Total    int64
}

type Orders struct{ saved map[string]int64 }

func (o *Orders) Handle(cmd PlaceOrder) error {
	if cmd.Customer == "" {
		return errors.New("customer required")
	}
	if cmd.Total <= 0 {
		return errors.New("total must be positive")
	}
	id := fmt.Sprintf("ord-%d", len(o.saved)+1)
	o.saved[id] = cmd.Total
	fmt.Println("placed", id, "for", cmd.Customer)
	return nil
}

func main() {
	o := &Orders{saved: map[string]int64{}}
	fmt.Println("ok: ", o.Handle(PlaceOrder{Customer: "alice", Total: 500}))
	fmt.Println("bad:", o.Handle(PlaceOrder{Customer: "", Total: 500}))
}
```

**Output**
```
placed ord-1 for alice
ok:  <nil>
bad: customer required
```

---

## 4. The query side

The query side is dumb and fast: it returns read-optimized DTOs — no domain model, no invariants.

```go
package main

import "fmt"

type OrderSummary struct {
	ID       string
	Customer string
	Status   string
}

type Queries struct{ rows []OrderSummary }

func (q Queries) ListForCustomer(customer string) []OrderSummary {
	var out []OrderSummary
	for _, r := range q.rows {
		if r.Customer == customer {
			out = append(out, r)
		}
	}
	return out
}

func main() {
	q := Queries{rows: []OrderSummary{
		{"ord-1", "alice", "placed"},
		{"ord-2", "bob", "shipped"},
		{"ord-3", "alice", "delivered"},
	}}
	for _, s := range q.ListForCustomer("alice") {
		fmt.Printf("%s: %s\n", s.ID, s.Status)
	}
}
```

**Output**
```
ord-1: placed
ord-3: delivered
```

---

## 5. Eventual consistency

When the read model is a separate store updated from events, it **lags** the write model by a moment.
The UI must tolerate "I just wrote it but don't see it yet."

```go
package main

import "fmt"

type System struct {
	writeStore map[string]bool
	readStore  map[string]bool
}

func (s *System) write(id string) { s.writeStore[id] = true } // committed immediately

func (s *System) project() { // runs asynchronously, a moment later
	for id := range s.writeStore {
		s.readStore[id] = true
	}
}

func main() {
	sys := &System{writeStore: map[string]bool{}, readStore: map[string]bool{}}
	sys.write("ord-1")
	fmt.Println("right after write — in read model?", sys.readStore["ord-1"]) // false: lagging
	sys.project()
	fmt.Println("after projection catches up?     ", sys.readStore["ord-1"]) // true
}
```

**Output**
```
right after write — in read model? false
after projection catches up?      true
```

---

Next tier → [Medium (6–10)](2-medium.md)
