# 29 — Design Patterns II: Structural & Composition (the Go way)

> Part 2 of the **Design Patterns** series: [28 Creational](28-patterns-creational.md) → **29 Structural** → [30 Behavioral](30-patterns-behavioral.md).
> Structural patterns are about *how types are wired together*. Go's answer to almost all of them is **composition via embedding and small interfaces** rather than class inheritance. The two you'll use daily — **decorator/middleware** and **adapter** — are here.

## Goals
- Compose behaviour with **struct/interface embedding** instead of inheritance, and know exactly what embedding does (and doesn't) do.
- Recognise the GoF **structural** patterns (Adapter, Decorator, Facade, Proxy, Composite, Bridge, Flyweight) and their Go forms.
- Master **middleware/decorator** chains over both interfaces and functions.
- Keep interfaces **small and consumer-defined** so wrapping and swapping stay cheap.

## Concepts

### Embedding: composition, not inheritance
Go has no classes and no inheritance. You **embed** a type to get its fields and methods *promoted* onto the outer type:
```go
type Logger struct{ prefix string }
func (l Logger) Log(msg string) { fmt.Println(l.prefix, msg) }

type Service struct {
    Logger              // embedded → Service now has a promoted Log method
    name string
}

s := Service{Logger: Logger{prefix: "[svc]"}, name: "orders"}
s.Log("started")        // calls the promoted Logger.Log
```
Promotion is *delegation*, not overriding: `Service` **has-a** `Logger` and forwards to it. Crucially, a method on the embedded type calls **its own** methods, not the outer type's — Go embedding is **not virtual dispatch**. (That single fact reshapes several GoF patterns; it bites hardest in Template Method — see [lesson 30](30-patterns-behavioral.md).)

### Embedding an *interface* to decorate selectively
Embed an **interface value** and you inherit every method through it — override just the ones you care about, delegate the rest:
```go
type Store interface {
    Get(key string) (string, error)
    Set(key, val string) error
}

type auditStore struct {
    Store                            // embedded interface value (the wrapped store)
    log *slog.Logger
}
func (a auditStore) Set(key, val string) error {
    a.log.Info("set", "key", key)
    return a.Store.Set(key, val)     // delegate to the wrapped implementation
}
// Get is promoted from the embedded Store unchanged — no boilerplate.

func WithAudit(s Store, l *slog.Logger) Store { return auditStore{Store: s, log: l} }
```
This is the workhorse decorator idiom in Go: embed the interface, override one method, forward the rest for free.

### Small interfaces, defined by the consumer
Structural patterns only stay cheap if interfaces are small. `io.Reader`/`io.Writer` are one method each; `io.ReadWriter` **embeds** both:
```go
type Reader interface{ Read(p []byte) (int, error) }
type Writer interface{ Write(p []byte) (int, error) }
type ReadWriter interface {          // interface embedding = set union of methods
    Reader
    Writer
}
```
Define the interface **where it's used** (the consumer), listing only the methods that call site needs. A 1–3 method interface is trivial to adapt, decorate, proxy, and fake in tests; a 20-method interface is none of those.

### Adapter — make X satisfy interface Y
An **adapter** wraps a type so it fits an interface it wasn't written for. Go's most elegant example is `http.HandlerFunc`, which adapts a plain function to the `http.Handler` interface — *that's the entire pattern*:
```go
type Handler interface{ ServeHTTP(w http.ResponseWriter, r *http.Request) }

type HandlerFunc func(http.ResponseWriter, *http.Request)

// the adapter: a func type given the interface method, which just calls itself
func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) { f(w, r) }
```
Adapting a *struct* is just as common — wrap a third-party client so your code depends on your own small interface:
```go
type Cache interface{ Get(key string) ([]byte, bool) }

type redisAdapter struct{ c *redis.Client }             // adaptee
func (a redisAdapter) Get(key string) ([]byte, bool) {  // fit it to *our* Cache
    b, err := a.c.Get(context.Background(), key).Bytes()
    return b, err == nil
}
```
Now the rest of the code knows only `Cache`, and Redis is swappable.

### Decorator / Middleware — wrap to add behaviour, same interface out
A **decorator** wraps a value and returns *the same interface*, adding behaviour around it. Because the wrapper satisfies the interface too, you can stack decorators. The function form is HTTP middleware:
```go
type Middleware func(http.Handler) http.Handler

func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)                     // ← call the wrapped handler
        slog.Info("req", "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
    })
}

func Recover(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if v := recover(); v != nil {
                http.Error(w, "internal error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// Chain applies middleware so the FIRST listed runs OUTERMOST (first in, last out):
func Chain(h http.Handler, mws ...Middleware) http.Handler {
    for i := len(mws) - 1; i >= 0; i-- {
        h = mws[i](h)
    }
    return h
}

// handler := Chain(mux, Recover, Logging)   // Recover wraps Logging wraps mux
```
The interface form decorates a service instead of a handler — same idea, wrapping `Store.Get` to add logging, metrics, retries, each as an independent layer (see also *Proxy* below, which is a decorator whose job is access control/caching).

### Facade — one friendly door over a messy subsystem
A **facade** is a single type exposing a simple method over several collaborators, so callers don't wire the subsystem themselves:
```go
type SignupService struct {         // the facade
    users  UserRepo
    mailer Mailer
    events EventBus
}

func (s SignupService) Register(ctx context.Context, email, pass string) (*User, error) {
    u, err := s.users.Create(ctx, email, hash(pass)) // subsystem call 1
    if err != nil {
        return nil, err
    }
    _ = s.mailer.SendWelcome(ctx, u.Email)           // subsystem call 2
    s.events.Publish(ctx, UserRegistered{ID: u.ID})  // subsystem call 3
    return u, nil
}
```
Your HTTP handler calls `svc.Register(...)` and knows nothing about repos, mail, or events. In clean-architecture terms the **service layer is a facade** over repositories and clients ([lesson 25](25-architecture.md)).

### Proxy — same interface, controlled access
A **proxy** has the *same interface* as the real object but interposes control: lazy creation, caching, access checks, remote calls. A caching proxy over `Store`:
```go
type cachingStore struct {
    next  Store
    mu    sync.Mutex
    cache map[string]string
}

func (s *cachingStore) Get(ctx context.Context, key string) (string, error) {
    s.mu.Lock()
    if v, ok := s.cache[key]; ok {
        s.mu.Unlock()
        return v, nil                 // served from cache, real store untouched
    }
    s.mu.Unlock()

    v, err := s.next.Get(ctx, key)    // miss → delegate
    if err != nil {
        return "", err
    }
    s.mu.Lock()
    s.cache[key] = v
    s.mu.Unlock()
    return v, nil
}
```
Decorator vs proxy: mechanically identical (wrap + delegate); the *intent* differs — a decorator **adds behaviour**, a proxy **controls access** to the real thing. `http.RoundTripper` wrappers (add auth headers, retry, cache) are proxies over the transport.

### Composite — trees via one shared interface
A **composite** lets a client treat a single object and a group of objects uniformly, by making both satisfy one interface. The recursive case (a directory) just loops over children of that same interface:
```go
type Node interface {
    Name() string
    Size() int64
}

type File struct {
    name string
    size int64
}
func (f File) Name() string { return f.name }
func (f File) Size() int64  { return f.size }

type Dir struct {
    name     string
    children []Node          // Files AND Dirs — both are Nodes
}
func (d Dir) Name() string { return d.name }
func (d Dir) Size() int64 {
    var total int64
    for _, c := range d.children {
        total += c.Size()    // uniform call — leaf or subtree, we don't care
    }
    return total
}
```
The stdlib's `io/fs.FS`, `html/template`'s node tree, and expression trees are all composites. The pattern is: *one interface, leaves and containers both implement it, containers recurse.*

### Bridge — vary abstraction and implementation independently
A **bridge** splits a thing into an **abstraction** (the API callers use) and an **implementation** (the mechanism), connected by an interface, so each can change without the other. In Go it's dependency injection with a role name:
```go
type Sender interface{ Send(to, msg string) error } // implementation side

type Notifier struct{ send Sender }                  // abstraction side
func (n Notifier) Alert(to string)   error { return n.send.Send(to, "ALERT: act now") }
func (n Notifier) Welcome(to string) error { return n.send.Send(to, "welcome!") }

type EmailSender struct{ /* ... */ }
func (EmailSender) Send(to, msg string) error { /* ... */ return nil }
type SMSSender struct{ /* ... */ }
func (SMSSender) Send(to, msg string) error { /* ... */ return nil }

// Notifier's methods never change; swap the mechanism freely:
n := Notifier{send: SMSSender{}}
```
Add a `SlackSender` without touching `Notifier`; add a `n.PasswordReset()` without touching any sender. That independence is the whole point.

### Flyweight — share immutable data instead of copying it
A **flyweight** shares one immutable instance across many owners to save memory. In Go the everyday form is **interning** (share one copy of a repeated value) or handing out a shared pointer to read-only config:
```go
var intern = map[string]string{}
func Intern(s string) string {
    if v, ok := intern[s]; ok {
        return v            // reuse the existing backing string
    }
    intern[s] = s
    return s
}
```
Because the shared object is **immutable**, sharing is safe even across goroutines. (If it's mutable, you're back to needing a lock — and it's probably not a flyweight.)

### Which patterns Go folds into the language
- **Adapter** is a function type with one method, or a thin wrapper struct — no `AdapterFactory`.
- **Decorator/Proxy** are the same "embed-or-wrap the interface, delegate the rest" move; `slog`, `http`, `database/sql` are built from it.
- **Facade** is just your service layer.
- **Bridge / Abstract Factory** collapse into plain dependency injection most of the time.

## Exercises
1. Build a `Service` that embeds a `Logger` and gets a promoted `Log`. Then prove embedding isn't inheritance: give `Logger` a method that calls another `Logger` method, override that second method on `Service`, and show the base still calls the base version.
2. Write `WithAudit(Store, *slog.Logger) Store` by **embedding the `Store` interface** and overriding only `Set`. Confirm `Get` is promoted with no code.
3. Implement `HandlerFunc` yourself (the func type + the `ServeHTTP` method) and use it to serve a route — the adapter in ~3 lines.
4. Write two middlewares (`Logging`, `Recover`) and a `Chain` helper; verify the *first* listed runs outermost by logging entry/exit order.
5. Wrap a `Store` in a `cachingStore` **proxy**; hit the same key twice and prove the second call never reaches the underlying store (increment a counter in the real one).
6. Model a filesystem as a **composite** (`File`/`Dir` both `Node`) and compute total `Size()` of a nested tree recursively.
7. Build a `Notifier` **bridge** over a `Sender` interface; add an `SMSSender` and a new `Notifier.PasswordReset()` method and note that neither change touches the other side.

## Best Practices & Pitfalls
- **Prefer composition (embedding) over deep type hierarchies** — Go gives you no inheritance on purpose; you rarely miss it.
- **Embedding is delegation, not overriding.** A promoted method calls the embedded type's own methods. If you need the outer type's behaviour, inject an interface instead of embedding (see Template Method in [lesson 30](30-patterns-behavioral.md)).
- **Keep interfaces tiny and define them at the consumer.** Everything structural — adapt, decorate, proxy, fake — gets cheaper as the interface shrinks.
- **Decorators must return the same interface** so they stack. If a wrapper changes the return type, it breaks the chain.
- **Mind middleware order.** `Chain(h, Recover, Logging)` should mean Recover is outermost. Pick a convention (first-listed = outermost) and keep it everywhere; getting Recover *inside* Logging means a panic escapes your recovery.
- **Pitfall — ambiguous embedding.** Embedding two types with the same method name is a compile error at the call site unless you disambiguate. Don't embed types with overlapping method sets casually.
- **Pitfall — leaky facade.** A facade that returns the subsystem's types (a raw `*sql.Rows`, a vendor error) re-exposes what it was meant to hide. Translate at the boundary.
- **Pitfall — flyweight that's actually mutable.** Sharing is only safe if the shared object is immutable; a "shared" mutable object across goroutines is a data race.

## Checklist
- [ ] I can explain the difference between embedding (delegation) and inheritance (virtual dispatch), and why Go has only the former.
- [ ] I can decorate a `Store` by embedding its interface and overriding one method.
- [ ] I can write the `HandlerFunc` adapter and a struct adapter over a third-party client.
- [ ] I can build a middleware chain and reason about wrap order.
- [ ] I can write a caching/access **proxy** and say how it differs in *intent* from a decorator.
- [ ] I can model a tree with the **composite** pattern and recurse over one interface.
- [ ] I can wire an abstraction to a swappable implementation with the **bridge** pattern (a.k.a. DI).

## Resources
- Effective Go — embedding: https://go.dev/doc/effective_go#embedding
- `net/http` `Handler`/`HandlerFunc` (the canonical adapter): https://pkg.go.dev/net/http#HandlerFunc
- `io` interfaces & composition: https://pkg.go.dev/io · `io/fs.FS` (composite tree): https://pkg.go.dev/io/fs#FS
- Dave Cheney — SOLID Go & interface design: https://dave.cheney.net/2016/08/20/solid-go-design
- Go Patterns (structural): https://github.com/tmrts/go-patterns#structural-patterns
- Previous: [28 — Creational](28-patterns-creational.md) · Next: [30 — Behavioral](30-patterns-behavioral.md).
