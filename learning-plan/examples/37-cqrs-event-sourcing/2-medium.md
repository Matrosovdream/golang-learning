# 37 · Medium (6–10) — event sourcing basics

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Apply: state is a fold over events

Event sourcing stores the changes, not the state. Current state is a left fold over the log; `Apply`
mutates state for one event — pure, no I/O.

```go
package main

import "fmt"

type Deposited struct{ Amount int64 }
type Withdrawn struct{ Amount int64 }

type Account struct{ balance int64 }

func (a *Account) Apply(e any) {
	switch e := e.(type) {
	case Deposited:
		a.balance += e.Amount
	case Withdrawn:
		a.balance -= e.Amount
	}
}

func main() {
	a := &Account{}
	for _, e := range []any{Deposited{100}, Withdrawn{30}, Deposited{50}} {
		a.Apply(e)
	}
	fmt.Println("balance =", a.balance) // 120
}
```

**Output**
```
balance = 120
```

---

## 7. A command decides events

A command **decides** which new events to emit — validating the invariant against current state
first. It returns events; it doesn't mutate directly.

```go
package main

import (
	"errors"
	"fmt"
)

type Withdrawn struct{ Amount int64 }

type Account struct{ balance int64 }

func (a *Account) Withdraw(amt int64) ([]any, error) {
	if amt > a.balance {
		return nil, errors.New("insufficient funds")
	}
	return []any{Withdrawn{Amount: amt}}, nil
}

func main() {
	a := &Account{balance: 100}
	events, err := a.Withdraw(30)
	fmt.Printf("events=%v err=%v\n", events, err)
	_, err = a.Withdraw(1000)
	fmt.Println("overdraw:", err)
}
```

**Output**
```
events=[{30}] err=<nil>
overdraw: insufficient funds
```

---

## 8. Replay to rebuild state

Rebuild = replay the log from zero. Each `Apply` also bumps the version (the number of events
applied).

```go
package main

import "fmt"

type Opened struct{ Owner string }
type Deposited struct{ Amount int64 }
type Withdrawn struct{ Amount int64 }

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

func Load(events []any) *Account {
	a := &Account{}
	for _, e := range events {
		a.Apply(e)
	}
	return a
}

func main() {
	log := []any{Opened{"alice"}, Deposited{100}, Withdrawn{30}}
	a := Load(log)
	fmt.Printf("owner=%s balance=%d version=%d\n", a.owner, a.balance, a.version)
}
```

**Output**
```
owner=alice balance=70 version=3
```

---

## 9. Optimistic concurrency

Append requires the expected version to match the current stream length, so a stale writer loses.

```go
package main

import (
	"errors"
	"fmt"
)

type EventStore struct{ streams map[string][]any }

func newStore() *EventStore { return &EventStore{streams: map[string][]any{}} }

var ErrConcurrency = errors.New("concurrency conflict: stale expected version")

func (s *EventStore) Append(id string, expectedVersion int, events ...any) error {
	if len(s.streams[id]) != expectedVersion {
		return ErrConcurrency
	}
	s.streams[id] = append(s.streams[id], events...)
	return nil
}

func main() {
	s := newStore()
	_ = s.Append("acc-1", 0, "Opened") // stream now length 1

	// two writers both loaded version 1:
	fmt.Println("writer1:", s.Append("acc-1", 1, "Deposited")) // ok → length 2
	fmt.Println("writer2:", s.Append("acc-1", 1, "Withdrawn")) // stale → conflict
	fmt.Println("stream length:", len(s.streams["acc-1"]))
}
```

**Output**
```
writer1: <nil>
writer2: concurrency conflict: stale expected version
stream length: 2
```

---

## 10. One store, many aggregates

The event store is keyed by aggregate id; each aggregate is rebuilt by replaying only its own stream.

```go
package main

import "fmt"

type Deposited struct{ Amount int64 }

type Account struct{ balance int64 }

func (a *Account) Apply(e any) {
	if d, ok := e.(Deposited); ok {
		a.balance += d.Amount
	}
}

func loadAccount(events []any) *Account {
	a := &Account{}
	for _, e := range events {
		a.Apply(e)
	}
	return a
}

func main() {
	store := map[string][]any{
		"acc-1": {Deposited{100}, Deposited{50}},
		"acc-2": {Deposited{200}},
	}
	for _, id := range []string{"acc-1", "acc-2"} {
		a := loadAccount(store[id])
		fmt.Printf("%s balance=%d\n", id, a.balance)
	}
}
```

**Output**
```
acc-1 balance=150
acc-2 balance=200
```

---

Next tier → [Hard (11–15)](3-hard.md)
