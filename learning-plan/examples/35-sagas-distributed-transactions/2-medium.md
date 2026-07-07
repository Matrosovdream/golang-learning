# 35 · Medium (6–10) — idempotency & coordination

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Reverse-order compensation

A 3-step saga threading shared state. Shipping fails, so charge then reserve compensate in reverse —
the log shows the exact order.

```go
package main

import (
	"errors"
	"fmt"
)

type State struct{ log []string }

type Step struct {
	name       string
	do         func(*State) error
	compensate func(*State)
}

func RunSaga(s *State, steps []Step) error {
	for i, step := range steps {
		if err := step.do(s); err != nil {
			for j := i - 1; j >= 0; j-- {
				steps[j].compensate(s)
			}
			return fmt.Errorf("aborted at %s: %w", step.name, err)
		}
	}
	return nil
}

func main() {
	steps := []Step{
		{"reserve", func(s *State) error { s.log = append(s.log, "+reserve"); return nil }, func(s *State) { s.log = append(s.log, "-reserve") }},
		{"charge", func(s *State) error { s.log = append(s.log, "+charge"); return nil }, func(s *State) { s.log = append(s.log, "-charge") }},
		{"ship", func(s *State) error { return errors.New("no couriers") }, func(s *State) { s.log = append(s.log, "-ship") }},
	}
	st := &State{}
	err := RunSaga(st, steps)
	fmt.Println("err:", err)
	fmt.Println("log:", st.log)
}
```

**Output**
```
err: aborted at ship: no couriers
log: [+reserve +charge -charge -reserve]
```

---

## 7. Idempotent steps

Saga steps run over at-least-once messaging, so each must be idempotent. Key by `sagaID+step` and
dedup, so a retried step applies once.

```go
package main

import "fmt"

type Service struct {
	done  map[string]bool
	stock int
}

func newService() *Service { return &Service{done: map[string]bool{}, stock: 10} }

func (s *Service) Reserve(key string, qty int) {
	if s.done[key] {
		fmt.Println(key, "already done — skip")
		return
	}
	s.done[key] = true
	s.stock -= qty
	fmt.Println(key, "reserved", qty, "→ stock", s.stock)
}

func main() {
	s := newService()
	s.Reserve("saga1:reserve", 3)
	s.Reserve("saga1:reserve", 3) // retried step — applied once
	fmt.Println("stock:", s.stock)
}
```

**Output**
```
saga1:reserve reserved 3 → stock 7
saga1:reserve already done — skip
stock: 7
```

---

## 8. Idempotent compensations

Compensations are retried too, so they must be idempotent — refunding twice must refund once.

```go
package main

import "fmt"

type Payments struct {
	refunded map[string]bool
	balance  int
}

func newPayments() *Payments { return &Payments{refunded: map[string]bool{}} }

func (p *Payments) Refund(key string, amt int) {
	if p.refunded[key] {
		fmt.Println(key, "already refunded — skip")
		return
	}
	p.refunded[key] = true
	p.balance += amt
	fmt.Println(key, "refunded", amt, "→ balance", p.balance)
}

func main() {
	p := newPayments()
	p.Refund("saga1:charge", 50)
	p.Refund("saga1:charge", 50) // compensation retried — refunds once
	fmt.Println("balance:", p.balance)
}
```

**Output**
```
saga1:charge refunded 50 → balance 50
saga1:charge already refunded — skip
balance: 50
```

---

## 9. Persist & resume

The orchestrator persists how many steps completed, so a crash resumes from the checkpoint instead of
restarting the whole saga.

```go
package main

import "fmt"

type Saga struct {
	steps     []string
	completed int
}

func (s *Saga) runFrom(start, crashAt int) {
	for i := start; i < len(s.steps); i++ {
		if i == crashAt {
			fmt.Println("CRASH at step", s.steps[i])
			return
		}
		fmt.Println("did", s.steps[i])
		s.completed = i + 1 // checkpoint after each step
	}
}

func main() {
	s := &Saga{steps: []string{"reserve", "charge", "ship"}}
	s.runFrom(0, 2) // crash before "ship"
	fmt.Println("checkpoint: completed", s.completed, "steps")
	fmt.Println("--- restart ---")
	s.runFrom(s.completed, -1) // resume from the checkpoint
	fmt.Println("completed:", s.completed)
}
```

**Output**
```
did reserve
did charge
CRASH at step ship
checkpoint: completed 2 steps
--- restart ---
did ship
completed: 3
```

---

## 10. Choreography

No central coordinator: each service reacts to an event and emits the next; a failure event triggers
compensations.

```go
package main

import "fmt"

type Bus struct{ handlers map[string]func(string) }

func newBus() *Bus { return &Bus{handlers: map[string]func(string){}} }

func (b *Bus) on(event string, h func(string)) { b.handlers[event] = h }

func (b *Bus) emit(event, data string) {
	fmt.Println("event:", event)
	if h, ok := b.handlers[event]; ok {
		h(data)
	}
}

func main() {
	bus := newBus()
	bus.on("OrderCreated", func(d string) { bus.emit("StockReserved", d) })
	bus.on("StockReserved", func(d string) { bus.emit("PaymentFailed", d) }) // payment fails
	bus.on("PaymentFailed", func(d string) { bus.emit("StockReleased", d) }) // compensation
	bus.on("StockReleased", func(d string) { bus.emit("OrderCancelled", d) })
	bus.on("OrderCancelled", func(d string) { fmt.Println("saga compensated for", d) })
	bus.emit("OrderCreated", "ord-1")
}
```

**Output**
```
event: OrderCreated
event: StockReserved
event: PaymentFailed
event: StockReleased
event: OrderCancelled
saga compensated for ord-1
```

---

Next tier → [Hard (11–15)](3-hard.md)
