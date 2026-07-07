# 33 · Hard (11–15) — lifecycle, segregation, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Lifecycle hooks (fx-style)

The idea **uber/fx** adds on top of DI: register `OnStart`/`OnStop` hooks, start in registration
order, stop in **reverse** (the server drains before the db closes). Hand-written here so it runs.

```go
package main

import "fmt"

type Hook struct {
	name    string
	onStart func()
	onStop  func()
}

type App struct{ hooks []Hook }

func (a *App) Append(h Hook) { a.hooks = append(a.hooks, h) }

func (a *App) Start() {
	for _, h := range a.hooks {
		h.onStart()
	}
}

func (a *App) Stop() {
	for i := len(a.hooks) - 1; i >= 0; i-- {
		a.hooks[i].onStop()
	}
}

func main() {
	app := &App{}
	for _, name := range []string{"db", "cache", "server"} {
		n := name
		app.Append(Hook{
			name:    n,
			onStart: func() { fmt.Println("start", n) },
			onStop:  func() { fmt.Println("stop ", n) },
		})
	}
	app.Start()
	app.Stop()
}
```

**Output**
```
start db
start cache
start server
stop  server
stop  cache
stop  db
```

---

## 12. Read config once at the root

Read config **once** at the root and pass values down — never `os.Getenv` deep in the graph, which
hides config and breaks tests.

```go
package main

import "fmt"

type Config struct {
	Addr    string
	Workers int
}

type Pool struct{ size int }

func newPool(workers int) *Pool { return &Pool{size: workers} }

type Server struct {
	addr string
	pool *Pool
}

func newServer(addr string, p *Pool) *Server { return &Server{addr: addr, pool: p} }

func main() {
	cfg := Config{Addr: ":9090", Workers: 4} // read once, at the root
	pool := newPool(cfg.Workers)             // pass values down
	srv := newServer(cfg.Addr, pool)
	fmt.Printf("server %s with %d workers\n", srv.addr, srv.pool.size)
}
```

**Output**
```
server :9090 with 4 workers
```

---

## 13. Interface segregation for testability

A wide client, but the core depends on a **narrow** interface (just what it uses), so a test needs
only a tiny fake.

```go
package main

import "fmt"

type S3Client struct{}

func (S3Client) Upload(k string) string   { return "uploaded " + k }
func (S3Client) Download(k string) string { return "downloaded " + k }
func (S3Client) Delete(k string) string   { return "deleted " + k }
func (S3Client) ListBuckets() []string    { return []string{"a", "b"} }

// The core needs ONLY upload:
type Uploader interface{ Upload(k string) string }

type Backup struct{ up Uploader }

func (b Backup) Run(key string) string { return b.up.Upload(key) }

type fakeUploader struct{}

func (fakeUploader) Upload(k string) string { return "fake-upload " + k }

func main() {
	fmt.Println(Backup{up: S3Client{}}.Run("data.tar"))
	fmt.Println(Backup{up: fakeUploader{}}.Run("data.tar"))
}
```

**Output**
```
uploaded data.tar
fake-upload data.tar
```

---

## 14. Decorator wiring at the root

Wiring decisions live at the root: it can wrap a dependency with a decorator (a logging `Repo`) before
injecting it — the `Service` never knows.

```go
package main

import "fmt"

type Repo interface{ Get(id string) string }

type memRepo struct{}

func (memRepo) Get(id string) string { return "row:" + id }

type loggingRepo struct {
	next Repo
	log  *[]string
}

func (r loggingRepo) Get(id string) string {
	*r.log = append(*r.log, "get "+id)
	return r.next.Get(id)
}

type Service struct{ repo Repo }

func main() {
	var log []string
	var repo Repo = loggingRepo{next: memRepo{}, log: &log} // root decides to decorate
	svc := Service{repo: repo}
	fmt.Println(svc.repo.Get("7"))
	fmt.Println("log:", log)
}
```

**Output**
```
row:7
log: [get 7]
```

---

## 15. Capstone: a full composition root

A full manual composition root — config → db → repo → service → server, built leaf-to-root, with
graceful shutdown that tears down in reverse.

```go
package main

import "fmt"

// Capstone: a full manual composition root — config → db → repo → service →
// server, built leaf-to-root, with graceful shutdown that tears down in reverse.

type Config struct{ Addr string }

type DB struct{ open bool }

func newDB() *DB       { return &DB{} }
func (d *DB) Connect() { d.open = true; fmt.Println("db: connected") }
func (d *DB) Close()   { d.open = false; fmt.Println("db: closed") }

type Repo struct{ db *DB }

func newRepo(db *DB) *Repo { return &Repo{db: db} }
func (r *Repo) Count() int { return 2 }

type Service struct{ repo *Repo }

func newService(r *Repo) *Service { return &Service{repo: r} }
func (s *Service) Report() string { return fmt.Sprintf("report: %d rows", s.repo.Count()) }

type Server struct {
	cfg Config
	svc *Service
}

func newServer(cfg Config, s *Service) *Server { return &Server{cfg: cfg, svc: s} }
func (srv *Server) Serve() {
	fmt.Println("server: listening on", srv.cfg.Addr)
	fmt.Println(srv.svc.Report())
}
func (srv *Server) Shutdown() { fmt.Println("server: drained") }

func main() {
	cfg := Config{Addr: ":8080"} // 1. config once
	db := newDB()                // 2. leaves first
	db.Connect()
	repo := newRepo(db) // 3. up the graph
	svc := newService(repo)
	srv := newServer(cfg, svc) // 4. root

	srv.Serve()

	// graceful shutdown: reverse order
	srv.Shutdown()
	db.Close()
}
```

**Output**
```
db: connected
server: listening on :8080
report: 2 rows
server: drained
db: closed
```

---

Back to [index](README.md) · That's **Track A** (structuring one service: 31 · 32 · 33) complete.
Next: [34 — Event-Driven & the Outbox](../34-event-driven-outbox/README.md) (Track B).
