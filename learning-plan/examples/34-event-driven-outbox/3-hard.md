# 34 · Hard (11–15) — outbox, partitions, DLQ, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. The transactional outbox

Write the state change and the event in **one transaction**, so they commit together atomically —
closing the dual-write gap from example 9.

```go
package main

import "fmt"

type Tx struct {
	orders []string
	outbox []string
}

type DB struct {
	orders []string
	outbox []string
}

func (db *DB) Transaction(fn func(*Tx) error) error {
	tx := &Tx{}
	if err := fn(tx); err != nil {
		return err // rollback: nothing applied
	}
	db.orders = append(db.orders, tx.orders...) // commit both together
	db.outbox = append(db.outbox, tx.outbox...)
	return nil
}

func main() {
	db := &DB{}
	_ = db.Transaction(func(tx *Tx) error {
		tx.orders = append(tx.orders, "ord-1")             // state change
		tx.outbox = append(tx.outbox, "OrderPlaced:ord-1") // event, same tx
		return nil
	})
	fmt.Println("orders:", db.orders)
	fmt.Println("outbox:", db.outbox)
}
```

**Output**
```
orders: [ord-1]
outbox: [OrderPlaced:ord-1]
```

---

## 12. The outbox relay is at-least-once

The relay reads unpublished rows, publishes them, and marks them sent. Crash after publishing but
before marking, and the row re-publishes on restart — so the relay itself is at-least-once, which is
exactly why consumers must be idempotent.

```go
package main

import "fmt"

type OutboxRow struct {
	id        int
	payload   string
	published bool
}

type Broker struct{ got []string }

func relay(rows []*OutboxRow, broker *Broker, crashBeforeMark bool) {
	for _, row := range rows {
		if row.published {
			continue
		}
		broker.got = append(broker.got, row.payload) // publish
		if crashBeforeMark {
			fmt.Println("CRASH after publishing", row.payload, "before marking sent")
			return
		}
		row.published = true
	}
}

func main() {
	rows := []*OutboxRow{{id: 1, payload: "E1"}}
	broker := &Broker{}
	relay(rows, broker, true)  // crash before marking → row still unpublished
	relay(rows, broker, false) // restart: re-publishes E1
	fmt.Println("broker got:", broker.got, "(duplicate → consumers must be idempotent)")
}
```

**Output**
```
CRASH after publishing E1 before marking sent
broker got: [E1 E1] (duplicate → consumers must be idempotent)
```

---

## 13. Partition ordering

Ordering is per **partition** (per key). Messages with the same key hash to one partition and stay
ordered; across keys there's no global order.

```go
package main

import "fmt"

type Msg struct{ key, val string }

func partition(key string, numPartitions int) int {
	sum := 0
	for _, r := range key {
		sum += int(r)
	}
	return sum % numPartitions
}

func main() {
	msgs := []Msg{
		{"ord-1", "created"}, {"ord-2", "created"},
		{"ord-1", "paid"}, {"ord-1", "shipped"},
	}
	parts := make([][]string, 2)
	for _, m := range msgs {
		p := partition(m.key, 2)
		parts[p] = append(parts[p], m.key+":"+m.val)
	}
	for i, p := range parts {
		fmt.Printf("partition %d: %v\n", i, p)
	}
	// within a partition, one key's events keep their order
}
```

**Output**
```
partition 0: [ord-2:created]
partition 1: [ord-1:created ord-1:paid ord-1:shipped]
```

---

## 14. Dead-letter queue

A poison message must not block the stream forever. After `maxRetries`, route it to the **dead-letter
queue** and move on.

```go
package main

import "fmt"

type DLQ struct{ dead []string }

func consume(msg string, maxRetries int, handle func() error, dlq *DLQ) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if handle() == nil {
			fmt.Printf("%s: ok on attempt %d\n", msg, attempt)
			return
		}
		fmt.Printf("%s: attempt %d failed\n", msg, attempt)
	}
	dlq.dead = append(dlq.dead, msg)
	fmt.Printf("%s: exhausted retries → dead-letter\n", msg)
}

func main() {
	dlq := &DLQ{}
	poison := func() error { return fmt.Errorf("bad message") }
	consume("m-bad", 3, poison, dlq)
	fmt.Println("DLQ:", dlq.dead)
}
```

**Output**
```
m-bad: attempt 1 failed
m-bad: attempt 2 failed
m-bad: attempt 3 failed
m-bad: exhausted retries → dead-letter
DLQ: [m-bad]
```

---

## 15. Capstone: the full outbox flow

The write side saves an order and its event in one atomic step; a relay publishes to an in-memory
broker (with one forced duplicate); an idempotent consumer dedups and updates a read model.

```go
package main

import "fmt"

// ===== write side (outbox-backed) =====
type outboxRow struct {
	payload   string
	published bool
}

type DB struct {
	orders map[string]int64
	outbox []outboxRow
}

func newDB() *DB { return &DB{orders: map[string]int64{}} }

func (db *DB) placeOrder(id string, total int64) {
	db.orders[id] = total                                 // state change +
	db.outbox = append(db.outbox, outboxRow{payload: id}) // event, one atomic step
}

// ===== broker (in-memory stand-in) =====
type Broker struct{ msgs []string }

func relay(db *DB, broker *Broker) {
	for i := range db.outbox {
		if db.outbox[i].published {
			continue
		}
		broker.msgs = append(broker.msgs, db.outbox[i].payload)
		db.outbox[i].published = true
	}
}

// ===== idempotent read-side consumer =====
type ReadModel struct {
	seen  map[string]bool
	count int
}

func newReadModel() *ReadModel { return &ReadModel{seen: map[string]bool{}} }

func (r *ReadModel) consume(orderID string) {
	if r.seen[orderID] {
		fmt.Println("consumer: duplicate", orderID, "- skip")
		return
	}
	r.seen[orderID] = true
	r.count++
	fmt.Println("consumer: projected", orderID, "count=", r.count)
}

func main() {
	db, broker, rm := newDB(), &Broker{}, newReadModel()

	db.placeOrder("ord-1", 3250)
	db.placeOrder("ord-2", 500)

	relay(db, broker)                          // publish both
	broker.msgs = append(broker.msgs, "ord-1") // at-least-once: a duplicate

	for _, m := range broker.msgs {
		rm.consume(m)
	}
	fmt.Println("orders in read model:", rm.count)
}
```

**Output**
```
consumer: projected ord-1 count= 1
consumer: projected ord-2 count= 2
consumer: duplicate ord-1 - skip
orders in read model: 2
```

---

Back to [index](README.md) · Next lesson's examples: [35 — Sagas & Distributed Transactions](../35-sagas-distributed-transactions/README.md).
