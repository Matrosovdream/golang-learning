# 34 · Medium (6–10) — delivery & idempotency

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. At-least-once delivery (redelivery)

The broker redelivers until the handler acks (returns nil). A transient failure just means "try
again" — so duplicates happen, and every consumer must expect them.

```go
package main

import "fmt"

type Broker struct{ maxAttempts int }

func (b Broker) Deliver(msg string, handle func(string) error) {
	for attempt := 1; attempt <= b.maxAttempts; attempt++ {
		err := handle(msg)
		if err == nil {
			fmt.Printf("attempt %d: ack\n", attempt)
			return
		}
		fmt.Printf("attempt %d: nack (%v) → redeliver\n", attempt, err)
	}
}

func main() {
	failures := 2
	handler := func(msg string) error {
		if failures > 0 {
			failures--
			return fmt.Errorf("transient")
		}
		return nil
	}
	Broker{maxAttempts: 5}.Deliver("OrderPlaced", handler)
}
```

**Output**
```
attempt 1: nack (transient) → redeliver
attempt 2: nack (transient) → redeliver
attempt 3: ack
```

---

## 7. Idempotent consumer (dedup by id)

Dedup by message id: processing the same message twice has the same effect as once. Essential under
at-least-once delivery.

```go
package main

import "fmt"

type Consumer struct {
	processed map[string]bool
	balance   int
}

func newConsumer() *Consumer { return &Consumer{processed: map[string]bool{}} }

func (c *Consumer) Handle(msgID string, amount int) {
	if c.processed[msgID] {
		fmt.Printf("msg %s already processed — skip\n", msgID)
		return
	}
	c.processed[msgID] = true
	c.balance += amount
	fmt.Printf("msg %s applied, balance=%d\n", msgID, c.balance)
}

func main() {
	c := newConsumer()
	c.Handle("m1", 100)
	c.Handle("m1", 100) // duplicate delivery — not applied twice
	c.Handle("m2", 50)
	fmt.Println("final balance:", c.balance)
}
```

**Output**
```
msg m1 applied, balance=100
msg m1 already processed — skip
msg m2 applied, balance=150
final balance: 150
```

---

## 8. Idempotency via a naturally-idempotent op

Another route: a naturally-idempotent operation. `SET status` applied twice is the same as once —
unlike `balance += amount`, which double-applies on redelivery.

```go
package main

import "fmt"

type Orders struct{ status map[string]string }

func (o Orders) MarkPaid(id string) { o.status[id] = "paid" }

func main() {
	orders := Orders{status: map[string]string{"ord-1": "pending"}}
	orders.MarkPaid("ord-1")
	orders.MarkPaid("ord-1") // redelivered — still just "paid"
	fmt.Println("ord-1 status:", orders.status["ord-1"])
}
```

**Output**
```
ord-1 status: paid
```

---

## 9. The dual-write problem

Saving to the DB and publishing to the broker are two separate operations. A crash between them
leaves the system inconsistent — the order exists but its event was never sent.

```go
package main

import "fmt"

type DB struct{ saved []string }
type Broker struct{ published []string }

func run(db *DB, broker *Broker, crashAfterSave bool) {
	db.saved = append(db.saved, "order") // step 1: committed
	if crashAfterSave {
		fmt.Println("CRASH after save, before publish!")
		return // event never sent → ghost state
	}
	broker.published = append(broker.published, "OrderPlaced")
}

func main() {
	db, broker := &DB{}, &Broker{}
	run(db, broker, true)
	fmt.Printf("db=%v broker=%v\n", db.saved, broker.published)
	fmt.Println("inconsistent: order exists but its event was never published")
}
```

**Output**
```
CRASH after save, before publish!
db=[order] broker=[]
inconsistent: order exists but its event was never published
```

---

## 10. Ack order matters

Ack-**after**-processing keeps messages safe across a crash (they get redelivered); ack-**first**
loses them.

```go
package main

import "fmt"

func ackFirst(process func() bool) {
	acked := true // acked before processing
	ok := process()
	fmt.Printf("ack-first: acked=%v processed=%v → crash mid-process => message LOST\n", acked, ok)
}

func ackAfter(process func() bool) {
	ok := process()
	acked := ok // ack only if processing succeeded
	fmt.Printf("ack-after: processed=%v acked=%v => crash mid-process => redelivered\n", ok, acked)
}

func main() {
	crashed := func() bool { return false } // processing did not complete
	ackFirst(crashed)
	ackAfter(crashed)
}
```

**Output**
```
ack-first: acked=true processed=false → crash mid-process => message LOST
ack-after: processed=false acked=false => crash mid-process => redelivered
```

---

Next tier → [Hard (11–15)](3-hard.md)
