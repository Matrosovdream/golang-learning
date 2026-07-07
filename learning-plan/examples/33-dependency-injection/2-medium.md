# 33 · Medium (6–10) — providers, wire, anti-patterns

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Provider functions

Extract sub-graphs into small `newX` provider functions so the composition root stays readable as it
grows. Manual DI scales surprisingly far this way.

```go
package main

import "fmt"

type DB struct{ dsn string }
type Repo struct{ db *DB }
type Service struct{ repo *Repo }

func newDB(dsn string) *DB        { return &DB{dsn: dsn} }
func newRepo(db *DB) *Repo        { return &Repo{db: db} }
func newService(r *Repo) *Service { return &Service{repo: r} }

func main() {
	db := newDB("postgres://...")
	repo := newRepo(db)
	svc := newService(repo)
	fmt.Println("wired service with db:", svc.repo.db.dsn)
}
```

**Output**
```
wired service with db: postgres://...
```

---

## 7. A provider set

Group related constructors so the root stays tidy — the idea `google/wire`'s `wire.NewSet` expresses.

```go
package main

import "fmt"

type Config struct{ Addr string }
type Repo struct{}
type Server struct {
	cfg  Config
	repo *Repo
}

type providers struct {
	config func() Config
	repo   func() *Repo
}

func storageProviders() providers {
	return providers{
		config: func() Config { return Config{Addr: ":8080"} },
		repo:   func() *Repo { return &Repo{} },
	}
}

func main() {
	p := storageProviders()
	srv := &Server{cfg: p.config(), repo: p.repo()}
	fmt.Println("server addr:", srv.cfg.Addr)
}
```

**Output**
```
server addr: :8080
```

---

## 8. A hand-written injector (wire)

This ordered constructor function is exactly what a tool like **google/wire** *generates* from your
providers — compile-checked, no runtime reflection. Here it's written by hand so it runs with no deps.

```go
package main

import "fmt"

type Config struct{ DSN string }
type DB struct{ dsn string }
type Repo struct{ db *DB }
type Service struct{ repo *Repo }

func newDB(c Config) *DB          { return &DB{dsn: c.DSN} }
func newRepo(db *DB) *Repo        { return &Repo{db: db} }
func newService(r *Repo) *Service { return &Service{repo: r} }

func InitService(c Config) *Service { // the "generated" injector
	db := newDB(c)
	repo := newRepo(db)
	return newService(repo)
}

func main() {
	svc := InitService(Config{DSN: "db://x"})
	fmt.Println("injected, dsn:", svc.repo.db.dsn)
}
```

**Output**
```
injected, dsn: db://x
```

---

## 9. Anti-pattern: the service locator

A **service locator** (a global bag components reach into) hides what a component needs — the
signature lies. Explicit injection puts the dependency in view.

```go
package main

import "fmt"

var registry = map[string]any{}

func provide(name string, v any) { registry[name] = v }
func locate(name string) any     { return registry[name] }

type BadService struct{}

func (BadService) Run() string {
	greeter := locate("greeter").(func() string) // invisible dependency
	return greeter()
}

type GoodService struct{ greet func() string }

func (s GoodService) Run() string { return s.greet() } // dependency is explicit

func main() {
	provide("greeter", func() string { return "hi (via locator)" })
	fmt.Println(BadService{}.Run())
	fmt.Println(GoodService{greet: func() string { return "hi (injected)" }}.Run())
}
```

**Output**
```
hi (via locator)
hi (injected)
```

---

## 10. Anti-pattern: package globals

A package **global** is shared state — two "tests" pollute one counter. An injected instance is
isolated.

```go
package main

import "fmt"

var globalCount int

func addGlobal() { globalCount++ }

type Counter struct{ n int }

func (c *Counter) Add() { c.n++ }

func main() {
	addGlobal() // "test A"
	addGlobal()
	addGlobal() // "test B" — sees test A's leftovers
	fmt.Println("global (shared):", globalCount)

	a := &Counter{}
	a.Add()
	a.Add()
	b := &Counter{}
	b.Add()
	fmt.Println("injected A:", a.n, "B:", b.n) // isolated
}
```

**Output**
```
global (shared): 3
injected A: 2 B: 1
```

---

Next tier → [Hard (11–15)](3-hard.md)
