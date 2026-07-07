# 34 · Easy (1–5) — bus, commands vs events

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. An in-memory event bus

A tiny in-memory bus standing in for a real broker (Kafka/NATS): publishers send to a topic,
subscribers receive.

```go
package main

import "fmt"

type Handler func(payload string)

type Bus struct{ subs map[string][]Handler }

func NewBus() *Bus { return &Bus{subs: map[string][]Handler{}} }

func (b *Bus) Subscribe(topic string, h Handler) { b.subs[topic] = append(b.subs[topic], h) }

func (b *Bus) Publish(topic, payload string) {
	for _, h := range b.subs[topic] {
		h(payload)
	}
}

func main() {
	bus := NewBus()
	bus.Subscribe("orders", func(p string) { fmt.Println("handler A got:", p) })
	bus.Publish("orders", "OrderPlaced:ord-1")
}
```

**Output**
```
handler A got: OrderPlaced:ord-1
```

---

## 2. Commands vs events

A **command** targets one handler and expects it to act (often returning a result). An **event** is a
fact broadcast to zero-or-many subscribers.

```go
package main

import "fmt"

type CommandBus struct {
	handlers map[string]func(string) string
}

func (b *CommandBus) Handle(cmd, arg string) string { return b.handlers[cmd](arg) }

type EventBus struct{ subs []func(string) }

func (b *EventBus) Publish(e string) {
	for _, s := range b.subs {
		s(e)
	}
}

func main() {
	cb := &CommandBus{handlers: map[string]func(string) string{
		"ChargeCard": func(arg string) string { return "charged " + arg },
	}}
	fmt.Println("command:", cb.Handle("ChargeCard", "$10")) // one recipient, returns a result

	eb := &EventBus{}
	eb.subs = append(eb.subs, func(e string) { fmt.Println("audit:", e) })
	eb.subs = append(eb.subs, func(e string) { fmt.Println("email:", e) })
	eb.Publish("PaymentCaptured") // fire-and-forget to many
}
```

**Output**
```
command: charged $10
audit: PaymentCaptured
email: PaymentCaptured
```

---

## 3. Fan-out to many subscribers

One event, many independent reactions. Adding a consumer is just another subscription — the producer
doesn't change.

```go
package main

import "fmt"

type Bus struct{ subs []func(string) }

func (b *Bus) Subscribe(h func(string)) { b.subs = append(b.subs, h) }

func (b *Bus) Publish(e string) {
	for _, h := range b.subs {
		h(e)
	}
}

func main() {
	bus := &Bus{}
	bus.Subscribe(func(e string) { fmt.Println("inventory reacts to", e) })
	bus.Subscribe(func(e string) { fmt.Println("shipping reacts to", e) })
	bus.Subscribe(func(e string) { fmt.Println("analytics reacts to", e) })
	bus.Publish("OrderPlaced")
}
```

**Output**
```
inventory reacts to OrderPlaced
shipping reacts to OrderPlaced
analytics reacts to OrderPlaced
```

---

## 4. Typed events + dispatch

Events are past-tense facts with typed payloads. A consumer dispatches on the event type; a loud
`default` catches new ones.

```go
package main

import "fmt"

type Event any

type OrderPlaced struct{ ID string }
type OrderShipped struct{ ID, Tracking string }

func handle(e Event) {
	switch ev := e.(type) {
	case OrderPlaced:
		fmt.Println("placed:", ev.ID)
	case OrderShipped:
		fmt.Printf("shipped: %s tracking=%s\n", ev.ID, ev.Tracking)
	default:
		fmt.Printf("unknown event %T\n", e)
	}
}

func main() {
	for _, e := range []Event{OrderPlaced{"ord-1"}, OrderShipped{"ord-1", "TRK9"}} {
		handle(e)
	}
}
```

**Output**
```
placed: ord-1
shipped: ord-1 tracking=TRK9
```

---

## 5. Notification vs state transfer

**Notification** is thin ("it happened, go look") — the consumer calls back for details.
**State transfer** carries what consumers need (no callback), but the schema becomes a shared
contract.

```go
package main

import "fmt"

type OrderPlacedThin struct{ ID string }

type OrderPlacedFat struct {
	ID    string
	Total int64
	Items []string
}

func main() {
	thin := OrderPlacedThin{ID: "ord-1"}
	fmt.Printf("notification:   %+v (consumer must fetch details)\n", thin)

	fat := OrderPlacedFat{ID: "ord-1", Total: 3250, Items: []string{"book", "pen"}}
	fmt.Printf("state transfer: %+v (self-contained)\n", fat)
}
```

**Output**
```
notification:   {ID:ord-1} (consumer must fetch details)
state transfer: {ID:ord-1 Total:3250 Items:[book pen]} (self-contained)
```

---

Next tier → [Medium (6–10)](2-medium.md)
