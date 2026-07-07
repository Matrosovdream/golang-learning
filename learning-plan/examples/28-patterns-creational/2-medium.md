# 28 · Medium (7–12) — builders, factories, singleton

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 7. Fluent builder

Each step returns the receiver so calls chain; a terminal `Build()` produces the result. Use a
builder when construction is *staged* and reads better as a sentence.

```go
package main

import (
	"fmt"
	"strings"
)

type QueryBuilder struct {
	table  string
	wheres []string
	limit  int
}

func NewQuery(table string) *QueryBuilder { return &QueryBuilder{table: table} }

func (q *QueryBuilder) Where(cond string) *QueryBuilder {
	q.wheres = append(q.wheres, cond)
	return q
}

func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

func (q *QueryBuilder) Build() string {
	sql := "SELECT * FROM " + q.table
	if len(q.wheres) > 0 {
		sql += " WHERE " + strings.Join(q.wheres, " AND ")
	}
	if q.limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", q.limit)
	}
	return sql
}

func main() {
	q := NewQuery("users").Where("age > 18").Where("active").Limit(10).Build()
	fmt.Println(q)
}
```

**Output**
```
SELECT * FROM users WHERE age > 18 AND active LIMIT 10
```

> Every chained method **must** `return q`. Forget it and the next call in the chain runs on `nil`.

---

## 8. Builder that validates in Build()

`Build()` can return `(T, error)` to validate the assembled whole.

```go
package main

import (
	"errors"
	"fmt"
)

type Pizza struct {
	size     string
	toppings []string
}

type PizzaBuilder struct {
	p Pizza
}

func NewPizza() *PizzaBuilder { return &PizzaBuilder{} }

func (b *PizzaBuilder) Size(s string) *PizzaBuilder { b.p.size = s; return b }

func (b *PizzaBuilder) Add(t string) *PizzaBuilder {
	b.p.toppings = append(b.p.toppings, t)
	return b
}

func (b *PizzaBuilder) Build() (Pizza, error) {
	if b.p.size == "" {
		return Pizza{}, errors.New("pizza: size required")
	}
	return b.p, nil
}

func main() {
	p, err := NewPizza().Size("L").Add("cheese").Add("basil").Build()
	fmt.Printf("%+v err=%v\n", p, err)

	_, err = NewPizza().Add("cheese").Build()
	fmt.Println("no size:", err)
}
```

**Output**
```
{size:L toppings:[cheese basil]} err=<nil>
no size: pizza: size required
```

---

## 9. Factory returning an interface

A factory returns an **interface**, choosing the concrete type from input — the caller never learns
which implementation it got. This is one of the few places you return an interface on purpose.

```go
package main

import "fmt"

type Store interface {
	Kind() string
}

type memStore struct{}

func (memStore) Kind() string { return "in-memory" }

type fileStore struct{}

func (fileStore) Kind() string { return "file-backed" }

func NewStore(kind string) (Store, error) {
	switch kind {
	case "memory":
		return memStore{}, nil
	case "file":
		return fileStore{}, nil
	default:
		return nil, fmt.Errorf("unknown store %q", kind)
	}
}

func main() {
	for _, k := range []string{"memory", "file", "redis"} {
		s, err := NewStore(k)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Println(k, "->", s.Kind())
	}
}
```

**Output**
```
memory -> in-memory
file -> file-backed
error: unknown store "redis"
```

---

## 10. Registry: a factory open for extension

Instead of a `switch` you must edit for every new type, register constructors into a map (often from
`init()`). This is exactly how `database/sql` drivers and `image` decoders plug in.

```go
package main

import "fmt"

type Store interface {
	Kind() string
}

type memStore struct{}

func (memStore) Kind() string { return "in-memory" }

type redisStore struct{}

func (redisStore) Kind() string { return "redis" }

var registry = map[string]func() Store{}

func Register(kind string, f func() Store) { registry[kind] = f }

func New(kind string) (Store, error) {
	f, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("unknown store %q", kind)
	}
	return f(), nil
}

func init() { // a driver package would do this for its side effect
	Register("memory", func() Store { return memStore{} })
	Register("redis", func() Store { return redisStore{} })
}

func main() {
	for _, k := range []string{"memory", "redis", "file"} {
		s, err := New(k)
		if err != nil {
			fmt.Println("err:", err)
			continue
		}
		fmt.Println(k, "->", s.Kind())
	}
}
```

**Output**
```
memory -> in-memory
redis -> redis
err: unknown store "file"
```

---

## 11. Abstract factory: a matched kit

One object manufactures a *matched set* of products. Swap the kit and everything it produces stays
consistent (a dark button always pairs with a dark checkbox).

```go
package main

import "fmt"

type Button interface{ Render() string }
type Checkbox interface{ Check() string }

type Kit interface {
	NewButton() Button
	NewCheckbox() Checkbox
}

type darkButton struct{}

func (darkButton) Render() string { return "[dark button]" }

type darkCheckbox struct{}

func (darkCheckbox) Check() string { return "[dark checkbox]" }

type DarkKit struct{}

func (DarkKit) NewButton() Button     { return darkButton{} }
func (DarkKit) NewCheckbox() Checkbox { return darkCheckbox{} }

type lightButton struct{}

func (lightButton) Render() string { return "[light button]" }

type lightCheckbox struct{}

func (lightCheckbox) Check() string { return "[light checkbox]" }

type LightKit struct{}

func (LightKit) NewButton() Button     { return lightButton{} }
func (LightKit) NewCheckbox() Checkbox { return lightCheckbox{} }

func renderUI(k Kit) {
	fmt.Println(k.NewButton().Render(), k.NewCheckbox().Check())
}

func main() {
	renderUI(DarkKit{})
	renderUI(LightKit{})
}
```

**Output**
```
[dark button] [dark checkbox]
[light button] [light checkbox]
```

---

## 12. Singleton with sync.Once (concurrent)

A lazy, thread-safe singleton: the initializer runs **exactly once** even when 100 goroutines race to
call `Get()`. (In real code, prefer passing the dependency in — dependency injection — over a global.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Config struct{ v int }

var (
	once      sync.Once
	instance  *Config
	initCount atomic.Int64
)

func Get() *Config {
	once.Do(func() {
		initCount.Add(1)
		instance = &Config{v: 42}
	})
	return instance
}

func main() {
	var wg sync.WaitGroup
	got := make([]*Config, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); got[i] = Get() }(i)
	}
	wg.Wait()

	allSame := true
	for _, c := range got {
		if c != got[0] {
			allSame = false
		}
	}
	fmt.Println("init ran", initCount.Load(), "time(s)")
	fmt.Println("all goroutines got the same instance:", allSame)
	fmt.Println("value:", Get().v)
}
```

**Output**
```
init ran 1 time(s)
all goroutines got the same instance: true
value: 42
```

> Run it with `go run -race .` too — `sync.Once` makes the lazy init race-free.

---

Next tier → [Hard (13–17)](3-hard.md)
