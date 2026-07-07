# 30 · Hard (12–16) — iterators, pub/sub, state objects, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

Examples 12, 13, and 16 use **range-over-func iterators** — need **Go 1.23+**.

---

## 12. Range-over-func iterator (iter.Seq)

The modern iterator: `range` over a function. `Count` yields values; returning `false` from `yield`
(the consumer's `break`) stops production. Unlike a channel iterator, it can't leak.

```go
package main

import (
	"fmt"
	"iter"
)

func Count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return // consumer broke out → stop
			}
		}
	}
}

func main() {
	fmt.Print("all: ")
	for v := range Count(5) {
		fmt.Print(v, " ")
	}
	fmt.Println()

	fmt.Print("break at 3: ")
	for v := range Count(100) {
		if v == 3 {
			break // stops the iterator too
		}
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output**
```
all: 0 1 2 3 4 
break at 3: 0 1 2 
```

---

## 13. Key/value iterator (iter.Seq2)

`iter.Seq2[K, V]` yields pairs — the shape the stdlib now uses (`maps.All`, `slices.All`, …).

```go
package main

import (
	"fmt"
	"iter"
)

func Enumerate[T any](s []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i, v := range s {
			if !yield(i, v) {
				return
			}
		}
	}
}

func main() {
	for i, name := range Enumerate([]string{"go", "rust", "zig"}) {
		fmt.Printf("%d = %s\n", i, name)
	}
}
```

**Output**
```
0 = go
1 = rust
2 = zig
```

---

## 14. Observer via channels (pub/sub)

Reach for channels over callbacks when subscribers need concurrency or backpressure. Each subscriber
gets its own channel.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

type Hub struct {
	mu   sync.Mutex
	subs []chan string
}

func (h *Hub) Subscribe() <-chan string {
	ch := make(chan string, 4)
	h.mu.Lock()
	h.subs = append(h.subs, ch)
	h.mu.Unlock()
	return ch
}

func (h *Hub) Publish(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		ch <- msg
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		close(ch)
	}
}

func main() {
	hub := &Hub{}
	a := hub.Subscribe()
	b := hub.Subscribe()
	hub.Publish("event-1")
	hub.Publish("event-2")
	hub.Close()

	var got []string
	for m := range a {
		got = append(got, "a:"+m)
	}
	for m := range b {
		got = append(got, "b:"+m)
	}
	sort.Strings(got) // deterministic output
	fmt.Println(got)
}
```

**Output**
```
[a:event-1 a:event-2 b:event-1 b:event-2]
```

---

## 15. State as objects

When per-state behaviour dominates (not just the next state), make each state a type that knows its
own transitions — the alternative to the transition table in example 5.

```go
package main

import "fmt"

type State interface {
	Name() string
	Next(event string) State
}

type Idle struct{}

func (Idle) Name() string { return "idle" }
func (Idle) Next(e string) State {
	if e == "start" {
		return Running{}
	}
	return Idle{}
}

type Running struct{}

func (Running) Name() string { return "running" }
func (Running) Next(e string) State {
	switch e {
	case "pause":
		return Paused{}
	case "stop":
		return Idle{}
	}
	return Running{}
}

type Paused struct{}

func (Paused) Name() string { return "paused" }
func (Paused) Next(e string) State {
	if e == "resume" {
		return Running{}
	}
	return Paused{}
}

func main() {
	var s State = Idle{}
	for _, e := range []string{"start", "pause", "resume", "stop"} {
		ns := s.Next(e)
		fmt.Printf("%s --%s--> %s\n", s.Name(), e, ns.Name())
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
```

---

## 16. Capstone: undoable stack calculator

Three patterns together: **Strategy** (pluggable ops in a map), **Command** (each op knows how to
undo itself), and a **range-over-func iterator** over the history to undo everything in reverse.

```go
package main

import (
	"fmt"
	"iter"
)

type BinOp func(a, b float64) float64

var ops = map[string]BinOp{
	"+": func(a, b float64) float64 { return a + b },
	"*": func(a, b float64) float64 { return a * b },
}

type Stack struct{ vals []float64 }

func (s *Stack) push(v float64) { s.vals = append(s.vals, v) }
func (s *Stack) pop() float64 {
	v := s.vals[len(s.vals)-1]
	s.vals = s.vals[:len(s.vals)-1]
	return v
}

type Command interface {
	Do(*Stack)
	Undo(*Stack)
}

type pushCmd struct{ v float64 }

func (c pushCmd) Do(s *Stack)   { s.push(c.v) }
func (c pushCmd) Undo(s *Stack) { s.pop() }

type applyCmd struct {
	op   string
	a, b float64 // operands captured at Do time so Undo can restore them
}

func (c *applyCmd) Do(s *Stack) {
	c.b, c.a = s.pop(), s.pop()
	s.push(ops[c.op](c.a, c.b))
}
func (c *applyCmd) Undo(s *Stack) {
	s.pop()
	s.push(c.a)
	s.push(c.b)
}

// reversed is a range-over-func iterator over the history, newest first.
func reversed(cmds []Command) iter.Seq[Command] {
	return func(yield func(Command) bool) {
		for i := len(cmds) - 1; i >= 0; i-- {
			if !yield(cmds[i]) {
				return
			}
		}
	}
}

func main() {
	s := &Stack{}
	program := []Command{pushCmd{2}, pushCmd{3}, &applyCmd{op: "+"}, pushCmd{4}, &applyCmd{op: "*"}}

	var history []Command
	for _, c := range program {
		c.Do(s)
		history = append(history, c)
	}
	fmt.Println("result:", s.vals) // (2+3)*4 = 20

	for c := range reversed(history) {
		c.Undo(s)
	}
	fmt.Println("after undo all:", s.vals) // empty
}
```

**Output**
```
result: [20]
after undo all: []
```

---

Back to [index](README.md) · That's the full Design Patterns example set (28 · 29 · 30). Next: work
through them, or ask for the architecture (31–41) example libraries.
