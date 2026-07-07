# 35 · Hard (11–15) — locks, isolation, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Semantic lock

A semantic lock guards intermediate state: while a saga runs, the order is `PENDING` and other
operations must refuse it. Compensation clears the lock.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct{ status string } // "confirmed" | "PENDING" | "cancelled"

func (o *Order) beginSaga() { o.status = "PENDING" }
func (o *Order) cancel()    { o.status = "cancelled" } // compensation

func modify(o *Order) error {
	if o.status == "PENDING" {
		return errors.New("order is mid-saga (locked)")
	}
	return nil
}

func main() {
	o := &Order{status: "confirmed"}
	fmt.Println("modify confirmed:  ", modify(o))
	o.beginSaga()
	fmt.Println("modify during saga:", modify(o))
	o.cancel()
	fmt.Println("status after compensation:", o.status)
}
```

**Output**
```
modify confirmed:   <nil>
modify during saga: order is mid-saga (locked)
status after compensation: cancelled
```

---

## 12. Compensation needs captured data

A compensation must have the data it needs, **captured at forward time** — you can't re-derive the
charged amount after the fact.

```go
package main

import "fmt"

type SagaState struct {
	chargedAmount int // captured when the charge step ran
}

func chargeStep(s *SagaState, amount int) {
	s.chargedAmount = amount // capture for the compensation
	fmt.Println("charged", amount)
}

func refundCompensation(s *SagaState) {
	fmt.Println("refunding captured amount", s.chargedAmount)
}

func main() {
	s := &SagaState{}
	chargeStep(s, 42)
	// ... a later step fails ...
	refundCompensation(s) // uses the captured amount
}
```

**Output**
```
charged 42
refunding captured amount 42
```

---

## 13. The missing isolation

Sagas lack isolation: another transaction can see intermediate state. A naive read counts
reserved-but-unconfirmed stock as available; a semantic-lock view subtracts it.

```go
package main

import "fmt"

type Inventory struct {
	available int
	reserved  int
}

func main() {
	inv := &Inventory{available: 7, reserved: 3} // 3 reserved by an in-flight saga

	fmt.Println("dirty read — available:", inv.available, "(3 reserved by an in-flight saga)")

	sellable := inv.available - inv.reserved // mitigation: expose only what's safe
	fmt.Println("semantic-lock view — sellable:", sellable)
}
```

**Output**
```
dirty read — available: 7 (3 reserved by an in-flight saga)
semantic-lock view — sellable: 4
```

---

## 14. Orchestration vs choreography

Same 3-step saga, two coordination styles. Orchestration keeps the whole flow in one place you can
read; choreography spreads it across event handlers, so no single place shows the flow.

```go
package main

import "fmt"

func orchestration() {
	fmt.Println("[orchestrator] do reserve")
	fmt.Println("[orchestrator] do charge")
	fmt.Println("[orchestrator] do ship")
}

func choreography() {
	fmt.Println("[stock]    on OrderCreated -> reserve, emit StockReserved")
	fmt.Println("[payment]  on StockReserved -> charge, emit PaymentDone")
	fmt.Println("[shipping] on PaymentDone -> ship")
}

func main() {
	fmt.Println("== orchestration ==")
	orchestration()
	fmt.Println("== choreography ==")
	choreography()
}
```

**Output**
```
== orchestration ==
[orchestrator] do reserve
[orchestrator] do charge
[orchestrator] do ship
== choreography ==
[stock]    on OrderCreated -> reserve, emit StockReserved
[payment]  on StockReserved -> charge, emit PaymentDone
[shipping] on PaymentDone -> ship
```

---

## 15. Capstone: a full order saga

An orchestrated order saga (reserve stock → charge → ship). Shipping fails, so completed steps
compensate in reverse. Steps and compensations are idempotent via a `saga+step` key, and the world's
state is fully restored.

```go
package main

import (
	"errors"
	"fmt"
)

type World struct {
	stock   int
	balance int
	done    map[string]bool
}

func newWorld() *World { return &World{stock: 10, done: map[string]bool{}} }

func (w *World) once(key string, fn func()) {
	if w.done[key] {
		return
	}
	w.done[key] = true
	fn()
}

type Step struct {
	name       string
	do         func() error
	compensate func()
}

func RunSaga(steps []Step) string {
	for i, step := range steps {
		if err := step.do(); err != nil {
			fmt.Printf("step %s failed: %v\n", step.name, err)
			for j := i - 1; j >= 0; j-- {
				steps[j].compensate()
			}
			return "COMPENSATED"
		}
		fmt.Printf("ok %s\n", step.name)
	}
	return "COMPLETED"
}

func main() {
	w := newWorld()
	steps := []Step{
		{"reserve",
			func() error { w.once("s1:reserve", func() { w.stock -= 3 }); return nil },
			func() { w.once("s1:reserve:c", func() { w.stock += 3 }); fmt.Println("  compensate: release stock") }},
		{"charge",
			func() error { w.once("s1:charge", func() { w.balance += 50 }); return nil },
			func() { w.once("s1:charge:c", func() { w.balance -= 50 }); fmt.Println("  compensate: refund") }},
		{"ship",
			func() error { return errors.New("no couriers") },
			func() {}},
	}
	result := RunSaga(steps)
	fmt.Println("result:", result)
	fmt.Printf("final stock=%d balance=%d\n", w.stock, w.balance)
}
```

**Output**
```
ok reserve
ok charge
step ship failed: no couriers
  compensate: refund
  compensate: release stock
result: COMPENSATED
final stock=10 balance=0
```

---

Back to [index](README.md) · Next lesson's examples: [36 — Resilience Patterns](../36-resilience-patterns/README.md).
