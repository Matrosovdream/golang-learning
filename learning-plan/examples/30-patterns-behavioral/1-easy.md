# 30 · Easy (1–6) — strategy, command, observer, state

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Strategy as a function value

The most useful behavioral idea in Go: pass the varying behaviour as a function value. No
`StrategyFactory`, no class per strategy.

```go
package main

import "fmt"

type PricingStrategy func(base float64) float64

func regular(base float64) float64   { return base }
func member(base float64) float64    { return base * 0.90 }
func clearance(base float64) float64 { return base * 0.50 }

type Checkout struct{ price PricingStrategy }

func (c Checkout) Total(base float64) float64 { return c.price(base) }

func main() {
	base := 100.0
	fmt.Printf("regular:   %.2f\n", Checkout{price: regular}.Total(base))
	fmt.Printf("member:    %.2f\n", Checkout{price: member}.Total(base))
	fmt.Printf("clearance: %.2f\n", Checkout{price: clearance}.Total(base))
}
```

**Output**
```
regular:   100.00
member:    90.00
clearance: 50.00
```

---

## 2. Strategy via slices.SortFunc

The comparator you pass to `slices.SortFunc` *is* the strategy — swap it to sort the same data
different ways.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type Person struct {
	Name string
	Age  int
}

func main() {
	people := []Person{{"carol", 20}, {"alice", 30}, {"bob", 25}}

	slices.SortFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println("by age: ", people)

	slices.SortFunc(people, func(a, b Person) int { return cmp.Compare(a.Name, b.Name) })
	fmt.Println("by name:", people)
}
```

**Output**
```
by age:  [{carol 20} {bob 25} {alice 30}]
by name: [{alice 30} {bob 25} {carol 20}]
```

---

## 3. Command as a closure queue

Wrap "an action plus its arguments" as a value you can store and run later. A `[]Command` is a job
queue.

```go
package main

import "fmt"

type Command func() error

func main() {
	queue := []Command{
		func() error { fmt.Println("resize image"); return nil },
		func() error { fmt.Println("send email"); return nil },
		func() error { fmt.Println("update index"); return nil },
	}
	for _, cmd := range queue {
		if err := cmd(); err != nil {
			fmt.Println("error:", err)
		}
	}
}
```

**Output**
```
resize image
send email
update index
```

---

## 4. Observer: a callback slice

The subject holds a list of observers and calls them all when something happens. Synchronous,
same-goroutine hooks.

```go
package main

import "fmt"

type Event struct{ Name string }
type Observer func(Event)

type Subject struct{ observers []Observer }

func (s *Subject) Subscribe(o Observer) { s.observers = append(s.observers, o) }

func (s *Subject) Notify(e Event) {
	for _, o := range s.observers {
		o(e)
	}
}

func main() {
	var s Subject
	s.Subscribe(func(e Event) { fmt.Println("logger saw:", e.Name) })
	s.Subscribe(func(e Event) { fmt.Println("mailer saw:", e.Name) })
	s.Notify(Event{Name: "UserSignedUp"})
}
```

**Output**
```
logger saw: UserSignedUp
mailer saw: UserSignedUp
```

---

## 5. State machine: a transition table

Model states and transitions as data instead of scattered boolean flags; invalid transitions are
rejected.

```go
package main

import "fmt"

type State string
type Event string

var transitions = map[State]map[Event]State{
	"idle":    {"start": "running"},
	"running": {"pause": "paused", "stop": "idle"},
	"paused":  {"resume": "running", "stop": "idle"},
}

func next(s State, e Event) (State, error) {
	if ns, ok := transitions[s][e]; ok {
		return ns, nil
	}
	return s, fmt.Errorf("invalid event %q in state %q", e, s)
}

func main() {
	s := State("idle")
	for _, e := range []Event{"start", "pause", "resume", "stop", "pause"} {
		ns, err := next(s, e)
		if err != nil {
			fmt.Println("rejected:", err)
			continue
		}
		fmt.Printf("%s --%s--> %s\n", s, e, ns)
		s = ns
	}
}
```

**Output**
```
idle --start--> running
running --pause--> paused
paused --resume--> running
running --stop--> idle
rejected: invalid event "pause" in state "idle"
```

---

## 6. Chain of responsibility

A request travels a chain; each link either handles it (stopping the chain) or passes it on.

```go
package main

import "fmt"

type Handler func(req string) bool

func chain(handlers ...Handler) Handler {
	return func(req string) bool {
		for _, h := range handlers {
			if h(req) {
				return true // first handler that claims it wins
			}
		}
		return false
	}
}

func main() {
	auth := func(req string) bool {
		if req == "auth" {
			fmt.Println("auth handled")
			return true
		}
		return false
	}
	cache := func(req string) bool {
		if req == "cache" {
			fmt.Println("cache handled")
			return true
		}
		return false
	}
	pipeline := chain(auth, cache)
	for _, req := range []string{"cache", "auth", "unknown"} {
		if !pipeline(req) {
			fmt.Println("unhandled:", req)
		}
	}
}
```

**Output**
```
cache handled
auth handled
unhandled: unknown
```

---

Next tier → [Medium (7–11)](2-medium.md)
