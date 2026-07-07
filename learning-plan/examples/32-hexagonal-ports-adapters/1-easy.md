# 32 · Easy (1–5) — ports, adapters, the composition root

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. A driven port + an in-memory adapter

A **driven port** is an interface the core owns and calls out through; an **adapter** implements it.
The core here needs persistence but imports no storage.

```go
package main

import (
	"errors"
	"fmt"
)

type Order struct {
	ID    string
	Total int64
}

type OrderRepository interface { // driven port, owned by the core
	Save(o Order) error
	Get(id string) (Order, error)
}

var ErrNotFound = errors.New("not found")

// --- adapter: in-memory implementation of the driven port ---
type memRepo struct{ m map[string]Order }

func newMemRepo() *memRepo { return &memRepo{m: map[string]Order{}} }

func (r *memRepo) Save(o Order) error { r.m[o.ID] = o; return nil }
func (r *memRepo) Get(id string) (Order, error) {
	o, ok := r.m[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	return o, nil
}

func main() {
	var repo OrderRepository = newMemRepo()
	_ = repo.Save(Order{ID: "ord-1", Total: 500})
	o, _ := repo.Get("ord-1")
	fmt.Printf("got: %+v\n", o)
}
```

**Output**
```
got: {ID:ord-1 Total:500}
```

---

## 2. Swap the adapter without touching the core

Because the core depends on the **port**, swapping the adapter changes behaviour without changing a
line of core logic.

```go
package main

import "fmt"

type Greeter interface{ Greet(name string) string } // core port

func welcome(g Greeter, name string) string { return "[welcome] " + g.Greet(name) }

type english struct{}

func (english) Greet(n string) string { return "Hello, " + n }

type french struct{}

func (french) Greet(n string) string { return "Bonjour, " + n }

func main() {
	fmt.Println(welcome(english{}, "Alice"))
	fmt.Println(welcome(french{}, "Alice")) // swap the adapter, core unchanged
}
```

**Output**
```
[welcome] Hello, Alice
[welcome] Bonjour, Alice
```

---

## 3. Two adapters for one port

One driven port, two adapters: a real one (prints) and a collecting one (perfect for tests). The core
calls the port and doesn't care which it got.

```go
package main

import "fmt"

type Notifier interface{ Notify(msg string) }

type printNotifier struct{}

func (printNotifier) Notify(msg string) { fmt.Println("print:", msg) }

type collectNotifier struct{ msgs []string }

func (c *collectNotifier) Notify(msg string) { c.msgs = append(c.msgs, msg) }

func alertAll(n Notifier) { // core logic, depends only on the port
	n.Notify("disk full")
	n.Notify("cpu high")
}

func main() {
	alertAll(printNotifier{})

	c := &collectNotifier{}
	alertAll(c)
	fmt.Println("collected:", c.msgs)
}
```

**Output**
```
print: disk full
print: cpu high
collected: [disk full cpu high]
```

---

## 4. A driving port the core implements

A **driving port** is the core's use-case interface — what the application can *do*. The core
implements it; driving adapters (HTTP, CLI…) call it.

```go
package main

import "fmt"

type PlaceOrder interface {
	Place(customer string, total int64) (string, error)
}

type orderService struct{ seq int } // the core implements the driving port

func (s *orderService) Place(customer string, total int64) (string, error) {
	s.seq++
	id := fmt.Sprintf("ord-%d", s.seq)
	fmt.Printf("placed %s for %s total=%d\n", id, customer, total)
	return id, nil
}

func main() {
	var uc PlaceOrder = &orderService{}
	id, _ := uc.Place("alice", 500)
	fmt.Println("returned id:", id)
}
```

**Output**
```
placed ord-1 for alice total=500
returned id: ord-1
```

---

## 5. The composition root

`main` is the one place that builds adapters and injects them into the core. Nothing inside the core
constructs an adapter.

```go
package main

import "fmt"

type Repo interface{ Save(s string) }

type Service struct{ repo Repo }

func (s Service) Do(x string) { s.repo.Save("done:" + x) }

type memRepo struct{ saved []string }

func (r *memRepo) Save(s string) { r.saved = append(r.saved, s) }

func main() {
	repo := &memRepo{}         // 1. build the leaf adapter
	svc := Service{repo: repo} // 2. inject it into the core
	svc.Do("a")
	svc.Do("b")
	fmt.Println("saved:", repo.saved)
}
```

**Output**
```
saved: [done:a done:b]
```

---

Next tier → [Medium (6–10)](2-medium.md)
