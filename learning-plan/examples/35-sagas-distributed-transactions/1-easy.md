# 35 · Easy (1–5) — steps & compensations

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. A linear saga (happy path)

A saga is a sequence of local transactions. On the happy path, each step succeeds and the saga
completes.

```go
package main

import "fmt"

type Step struct {
	name string
	do   func() error
}

func run(steps []Step) error {
	for _, s := range steps {
		if err := s.do(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		fmt.Println("did", s.name)
	}
	return nil
}

func main() {
	steps := []Step{
		{"create order", func() error { return nil }},
		{"reserve stock", func() error { return nil }},
		{"charge card", func() error { return nil }},
	}
	if err := run(steps); err != nil {
		fmt.Println("saga failed:", err)
		return
	}
	fmt.Println("saga completed")
}
```

**Output**
```
did create order
did reserve stock
did charge card
saga completed
```

---

## 2. A step and its compensation

Each step has a compensating action that semantically undoes it — a new, reversing operation
(charge → refund), not a database rollback.

```go
package main

import "fmt"

type Account struct{ balance int }

func main() {
	acc := &Account{balance: 100}

	charge := func(amt int) { acc.balance -= amt; fmt.Println("charged", amt, "→ balance", acc.balance) }
	refund := func(amt int) { acc.balance += amt; fmt.Println("refunded", amt, "→ balance", acc.balance) }

	charge(30)
	refund(30) // compensation
	fmt.Println("final balance:", acc.balance)
}
```

**Output**
```
charged 30 → balance 70
refunded 30 → balance 100
final balance: 100
```

---

## 3. Orchestration: compensate on failure

A coordinator runs each step; on failure it walks the compensations backward (most recent completed
step first).

```go
package main

import (
	"errors"
	"fmt"
)

type Step struct {
	name       string
	do         func() error
	compensate func()
}

func RunSaga(steps []Step) error {
	for i, step := range steps {
		if err := step.do(); err != nil {
			fmt.Printf("step %q failed: %v\n", step.name, err)
			for j := i - 1; j >= 0; j-- {
				fmt.Printf("compensating %q\n", steps[j].name)
				steps[j].compensate()
			}
			return err
		}
		fmt.Printf("did %q\n", step.name)
	}
	return nil
}

func main() {
	steps := []Step{
		{"order", func() error { return nil }, func() { fmt.Println("  → cancel order") }},
		{"stock", func() error { return nil }, func() { fmt.Println("  → release stock") }},
		{"payment", func() error { return errors.New("card declined") }, func() { fmt.Println("  → refund") }},
	}
	_ = RunSaga(steps)
}
```

**Output**
```
did "order"
did "stock"
step "payment" failed: card declined
compensating "stock"
  → release stock
compensating "order"
  → cancel order
```

---

## 4. Compensation is semantic undo

You can't roll back a committed local transaction in another service, so you issue a new reversing
one — in reverse order.

```go
package main

import "fmt"

type System struct {
	stock   int
	charged int
}

func main() {
	sys := &System{stock: 10}

	// forward:
	sys.stock -= 3
	fmt.Println("reserved 3, stock =", sys.stock)
	sys.charged += 50
	fmt.Println("charged $50, charged =", sys.charged)

	// compensations — new reversing transactions, in reverse order:
	sys.charged -= 50
	fmt.Println("refunded $50 (compensate charge), charged =", sys.charged)
	sys.stock += 3
	fmt.Println("released 3 (compensate reserve), stock =", sys.stock)
}
```

**Output**
```
reserved 3, stock = 7
charged $50, charged = 50
refunded $50 (compensate charge), charged = 0
released 3 (compensate reserve), stock = 10
```

---

## 5. Terminal state: completed vs compensated

A saga drives to a terminal state: **COMPLETED** (all forward) or **COMPENSATED** (all completed steps
undone).

```go
package main

import (
	"errors"
	"fmt"
)

type Step struct {
	do         func() error
	compensate func()
}

func run(steps []Step) string {
	for i, s := range steps {
		if err := s.do(); err != nil {
			for j := i - 1; j >= 0; j-- {
				steps[j].compensate()
			}
			return "COMPENSATED"
		}
	}
	return "COMPLETED"
}

func main() {
	ok := []Step{
		{func() error { return nil }, func() {}},
		{func() error { return nil }, func() {}},
	}
	bad := []Step{
		{func() error { return nil }, func() {}},
		{func() error { return errors.New("x") }, func() {}},
	}
	fmt.Println("happy path →", run(ok))
	fmt.Println("failure   →", run(bad))
}
```

**Output**
```
happy path → COMPLETED
failure   → COMPENSATED
```

---

Next tier → [Medium (6–10)](2-medium.md)
