# 37 · Hard (11–15) — snapshots, projections, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Snapshots

A snapshot is a performance optimization: store state at version N, then load = snapshot + events
after N. It must produce the same result as a full replay.

```go
package main

import "fmt"

type Deposited struct{ Amount int64 }

type Snapshot struct {
	balance int64
	version int
}

type Account struct {
	balance int64
	version int
}

func (a *Account) Apply(e any) {
	if d, ok := e.(Deposited); ok {
		a.balance += d.Amount
	}
	a.version++
}

func loadFromSnapshot(snap Snapshot, laterEvents []any) *Account {
	a := &Account{balance: snap.balance, version: snap.version}
	for _, e := range laterEvents {
		a.Apply(e)
	}
	return a
}

func fullReplay(events []any) *Account {
	a := &Account{}
	for _, e := range events {
		a.Apply(e)
	}
	return a
}

func main() {
	all := []any{Deposited{100}, Deposited{50}, Deposited{25}, Deposited{25}}
	snap := Snapshot{balance: 150, version: 2} // taken after the first 2 events

	fromSnap := loadFromSnapshot(snap, all[2:])
	full := fullReplay(all)
	fmt.Printf("from snapshot: balance=%d version=%d\n", fromSnap.balance, fromSnap.version)
	fmt.Printf("full replay:   balance=%d version=%d\n", full.balance, full.version)
	fmt.Println("identical:", fromSnap.balance == full.balance && fromSnap.version == full.version)
}
```

**Output**
```
from snapshot: balance=200 version=4
full replay:   balance=200 version=4
identical: true
```

---

## 12. A projection

A projection builds a read model by consuming events — here a per-account balance table. This is
where event sourcing meets CQRS.

```go
package main

import "fmt"

type Deposited struct {
	Account string
	Amount  int64
}
type Withdrawn struct {
	Account string
	Amount  int64
}

type Balances struct{ m map[string]int64 }

func newBalances() *Balances { return &Balances{m: map[string]int64{}} }

func (b *Balances) On(e any) {
	switch e := e.(type) {
	case Deposited:
		b.m[e.Account] += e.Amount
	case Withdrawn:
		b.m[e.Account] -= e.Amount
	}
}

func main() {
	events := []any{
		Deposited{"acc-1", 100},
		Deposited{"acc-2", 50},
		Withdrawn{"acc-1", 30},
	}
	proj := newBalances()
	for _, e := range events {
		proj.On(e)
	}
	fmt.Println("acc-1:", proj.m["acc-1"]) // 70
	fmt.Println("acc-2:", proj.m["acc-2"]) // 50
}
```

**Output**
```
acc-1: 70
acc-2: 50
```

---

## 13. Rebuild a projection

A projection must be rebuildable: drop the read model and replay the whole log (to add a new view or
fix a bug). Same log → same result.

```go
package main

import "fmt"

type Deposited struct{ Amount int64 }

func project(events []any) int64 {
	var total int64
	for _, e := range events {
		if d, ok := e.(Deposited); ok {
			total += d.Amount
		}
	}
	return total
}

func main() {
	log := []any{Deposited{100}, Deposited{50}, Deposited{25}}
	fmt.Println("built:  ", project(log))
	// later: the read schema changed → just rebuild from the same log
	fmt.Println("rebuilt:", project(log))
}
```

**Output**
```
built:   175
rebuilt: 175
```

---

## 14. Event versioning (upcasting)

Stored events live forever, so schema change is inevitable. **Upcast** old event versions to the
current shape on read (defaulting new fields).

```go
package main

import "fmt"

type DepositedV1 struct{ Amount int64 }
type DepositedV2 struct {
	Amount   int64
	Currency string // added in v2
}

func upcast(e any) DepositedV2 {
	switch e := e.(type) {
	case DepositedV1:
		return DepositedV2{Amount: e.Amount, Currency: "USD"} // default the new field
	case DepositedV2:
		return e
	}
	return DepositedV2{}
}

func main() {
	log := []any{DepositedV1{100}, DepositedV2{50, "EUR"}}
	for _, e := range log {
		fmt.Printf("%+v\n", upcast(e))
	}
}
```

**Output**
```
{Amount:100 Currency:USD}
{Amount:50 Currency:EUR}
```

---

## 15. Capstone: event sourcing + CQRS

An event-sourced `Account` (write side) with a CQRS read model. Commands decide events; the
append-only store is the source of truth; the write side is rebuilt by replay; the read model is a
projection of the same log.

```go
package main

import (
	"errors"
	"fmt"
)

// ===== events =====
type Opened struct{ Owner string }
type Deposited struct{ Amount int64 }
type Withdrawn struct{ Amount int64 }

// ===== write side: event-sourced aggregate =====
type Account struct {
	owner   string
	balance int64
	version int
}

func (a *Account) Apply(e any) {
	switch e := e.(type) {
	case Opened:
		a.owner = e.Owner
	case Deposited:
		a.balance += e.Amount
	case Withdrawn:
		a.balance -= e.Amount
	}
	a.version++
}

func (a *Account) Withdraw(amt int64) ([]any, error) {
	if amt > a.balance {
		return nil, errors.New("insufficient funds")
	}
	return []any{Withdrawn{amt}}, nil
}

// ===== event store (append-only) =====
type Store struct{ events []any }

func (s *Store) append(evts ...any) { s.events = append(s.events, evts...) }

func (s *Store) load() *Account {
	a := &Account{}
	for _, e := range s.events {
		a.Apply(e)
	}
	return a
}

// ===== read side: projection =====
type ReadModel struct{ balance int64 }

func (r *ReadModel) project(events []any) {
	a := &Account{}
	for _, e := range events {
		a.Apply(e)
	}
	r.balance = a.balance
}

func main() {
	store := &Store{}
	store.append(Opened{"alice"}, Deposited{100}) // commands → events

	acc := store.load()           // rebuild current state
	evts, err := acc.Withdraw(30) // decide from current state
	if err != nil {
		fmt.Println(err)
		return
	}
	store.append(evts...)

	final := store.load() // write side reconstructed by replay
	fmt.Printf("write side: owner=%s balance=%d version=%d\n", final.owner, final.balance, final.version)

	rm := &ReadModel{} // read side rebuilt from the same log
	rm.project(store.events)
	fmt.Println("read model balance:", rm.balance)
}
```

**Output**
```
write side: owner=alice balance=70 version=3
read model balance: 70
```

---

Back to [index](README.md) · That's **Track B** (coordinating services: 34 · 35 · 36 · 37) complete.
Next: [38 — Caching Patterns](../38-caching-patterns/README.md) (Track C).
