# 28 — Design Patterns I: Creational & Construction (the Go way)

> Part 1 of a 3-lesson **Design Patterns** series: **28 Creational** → [29 Structural](29-patterns-structural.md) → [30 Behavioral](30-patterns-behavioral.md).
> These teach the classic Gang-of-Four (GoF) patterns **as Go actually writes them** — with functional options, first-class functions, embedding, and a useful zero value doing the work that inheritance and `new`-heavy factories do in Java/C++. Concurrency patterns (worker pool, fan-out/in, pipeline, pub/sub) live in [15 — Sync, Context & Patterns](15-sync-context.md); general style lives in [24 — Idiomatic Go](24-idiomatic-go.md).

## Goals
- Construct objects the idiomatic Go way: the **useful zero value**, constructor functions, **functional options**, and fluent **builders** — and know which fits.
- Recognise the GoF **creational** patterns (Factory / Factory Method, Abstract Factory, Singleton, Prototype, Object Pool) and their Go-native forms.
- Use Go's tools — `sync.Once`, `sync.Pool`, first-class functions, registries via `init()` — instead of porting patterns verbatim.
- Know which classic patterns Go makes **unnecessary**, and why reaching for them is usually a smell.

## Concepts

### Start here: make the zero value useful
Before any pattern, ask: *does this even need a constructor?* Go's biggest "creational pattern" is designing a type whose **zero value is ready to use**. `sync.Mutex`, `bytes.Buffer`, `strings.Builder`, and `time.Time` all work with no initialisation:
```go
var b bytes.Buffer          // zero value — usable immediately
b.WriteString("hello")

var mu sync.Mutex           // no NewMutex() needed
mu.Lock()
```
When you can arrange this, you delete a constructor and a whole class of "forgot to call `New`" bugs. Reach for the patterns below only when construction has real work: validation, defaults, dependencies, or expensive setup.

### Constructor functions (`NewT`)
Go has no constructors. The convention is a package-level `NewT` returning `*T` or `(T, error)`. Put invariant checks here so an invalid value can never exist:
```go
type Account struct {
    id      string
    balance int64 // cents; unexported → callers can't corrupt it
}

func NewAccount(id string, opening int64) (*Account, error) {
    if id == "" {
        return nil, errors.New("account: id required")
    }
    if opening < 0 {
        return nil, fmt.Errorf("account: negative opening balance %d", opening)
    }
    return &Account{id: id, balance: opening}, nil
}
```
- Name it `New` when the package name already says the type (`account.New`), else `NewAccount`.
- Return a **concrete type** (`*Account`), not an interface. *Accept interfaces, return structs.*

### Functional Options — the idiomatic "builder" for configuration
The canonical Go pattern for **optional, defaulted** settings. An option is a function that mutates the target; the constructor applies defaults first, then the options:
```go
type Server struct {
    addr    string
    timeout time.Duration
    maxConn int
    logger  *slog.Logger
}

type Option func(*Server)

func WithTimeout(d time.Duration) Option { return func(s *Server) { s.timeout = d } }
func WithMaxConn(n int) Option           { return func(s *Server) { s.maxConn = n } }
func WithLogger(l *slog.Logger) Option   { return func(s *Server) { s.logger = l } }

func NewServer(addr string, opts ...Option) *Server {
    s := &Server{                    // 1. sensible defaults
        addr:    addr,
        timeout: 30 * time.Second,
        maxConn: 100,
        logger:  slog.Default(),
    }
    for _, opt := range opts {       // 2. apply overrides in order
        opt(s)
    }
    return s
}

// call site — required arg positional, everything else named & optional:
srv := NewServer("localhost:8080",
    WithTimeout(5*time.Second),
    WithMaxConn(500),
)
```
Why this beats a config struct or telescoping constructors: **defaults live in one place**, adding an option never breaks existing callers (unlike adding a struct field they must now set), and options can be validated or composed. If an option can fail, use `type Option func(*Server) error` and return the first error from the loop.

### Builder (fluent) — and when to prefer it over options
A **builder** accumulates state through chained calls and produces the result at the end. Each step returns the receiver so calls chain:
```go
type QueryBuilder struct {
    table  string
    wheres []string
    limit  int
}

func NewQuery(table string) *QueryBuilder { return &QueryBuilder{table: table} }

func (q *QueryBuilder) Where(cond string) *QueryBuilder {
    q.wheres = append(q.wheres, cond)
    return q                          // ← return receiver → chainable
}
func (q *QueryBuilder) Limit(n int) *QueryBuilder { q.limit = n; return q }

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

// q := NewQuery("users").Where("age > 18").Where("active").Limit(10).Build()
```
**Options vs builder:** use **functional options** for a value you construct once with some optional knobs (servers, clients, loggers). Use a **fluent builder** when construction is *staged* or *incremental* (query builders, request builders, test-data factories) and reads better as a sentence. Builders can also return `(T, error)` from `Build()` to validate the assembled whole.

### Factory & Factory Method — choose the concrete type at runtime
A **factory** is just a function returning an **interface**, deciding the implementation from input:
```go
type Store interface {
    Get(ctx context.Context, key string) (string, error)
}

func NewStore(kind string) (Store, error) {   // returns the interface on purpose
    switch kind {
    case "memory":
        return newMemStore(), nil
    case "redis":
        return newRedisStore(), nil
    default:
        return nil, fmt.Errorf("unknown store %q", kind)
    }
}
```
This is one of the few places you *do* return an interface: the caller genuinely shouldn't know which concrete type it got.

### Registry (open Factory) — extension without editing the switch
When new implementations should be pluggable (drivers, codecs), register constructors into a map, often from `init()`:
```go
var storeFactories = map[string]func() Store{}

func Register(kind string, f func() Store) { storeFactories[kind] = f }

func New(kind string) (Store, error) {
    f, ok := storeFactories[kind]
    if !ok {
        return nil, fmt.Errorf("unknown store %q", kind)
    }
    return f(), nil
}

// in a driver package:
func init() { Register("redis", func() Store { return newRedisStore() }) }
```
This is exactly how `database/sql` (`sql.Register`) and `image` decoders (`image.RegisterFormat`) work — import a driver for its side-effect `init()` and it appears in the registry. The core package never changes.

### Abstract Factory — a "kit" of related constructors
When you need a *family* of objects that must match (a UI theme, a cloud provider's clients), group their constructors on one struct:
```go
type Kit interface {
    NewButton() Button
    NewCheckbox() Checkbox
}
type DarkKit struct{}
func (DarkKit) NewButton() Button     { return darkButton{} }
func (DarkKit) NewCheckbox() Checkbox { return darkCheckbox{} }
// swap DarkKit → LightKit and everything it produces is consistent.
```
In practice Go often replaces this with a struct of function fields or plain dependency injection — but the "one object that manufactures a matched set" idea is the same.

### Singleton — and why Go usually says *no*
A lazy, thread-safe singleton is `sync.Once`:
```go
var (
    once     sync.Once
    instance *Config
)

func Config() *Config {                 // eager readers all get the same instance
    once.Do(func() { instance = load() }) // runs exactly once, safe under concurrency
    return instance
}
```
But **prefer dependency injection**: pass the `*Config`, `*sql.DB`, or `*slog.Logger` into the constructors that need it. Singletons look convenient and then wreck you: they hide dependencies (you can't tell what a function needs from its signature), make tests share global state, and can't be swapped for a fake. Reach for `sync.Once` for genuinely process-wide, immutable setup (parsed config, a metrics registry) — not as a shortcut to avoid threading a dependency through.

### Object Pool — reuse expensive allocations with `sync.Pool`
`sync.Pool` recycles short-lived objects to cut GC pressure on hot paths (e.g. reusing buffers per request):
```go
var bufPool = sync.Pool{
    New: func() any { return new(bytes.Buffer) },
}

func render(name string) string {
    buf := bufPool.Get().(*bytes.Buffer)
    buf.Reset()                     // pooled objects are DIRTY — always reset
    defer bufPool.Put(buf)
    buf.WriteString("hello ")
    buf.WriteString(name)
    return buf.String()
}
```
Caveats: a `sync.Pool` may drop its contents at any GC, so it's a cache, **not** a guaranteed-size resource pool — for connections use a real pool (`database/sql`'s pool, or a bounded channel of resources). Never keep a reference to an object after `Put`.

### Prototype — clone instead of rebuild
When constructing a fresh object is expensive but copying a template is cheap, give the type a `Clone()`. Watch shallow vs deep copy — Go's plain assignment copies the struct but **shares** slices/maps/pointers:
```go
type Graph struct {
    Nodes map[string][]string
}

func (g *Graph) Clone() *Graph {
    cp := &Graph{Nodes: make(map[string][]string, len(g.Nodes))}
    for k, edges := range g.Nodes {
        cp.Nodes[k] = append([]string(nil), edges...) // copy the slice too → deep
    }
    return cp
}
```

### Patterns Go makes unnecessary
- **Telescoping constructors / Java-style Builder for immutability** → functional options.
- **Singleton as a convenience** → dependency injection; keep `sync.Once` for real process-wide setup only.
- **A factory for everything** → return concrete structs; only introduce a factory + interface when the caller must stay ignorant of the concrete type.
- **`Cloneable` interfaces & copy-constructors** → a plain `Clone()` method, or just let assignment copy value types.

## Exercises
1. Write a `NewAccount(id, opening)` returning `(*Account, error)` that rejects an empty id and a negative balance. Prove an invalid `Account` can't be constructed.
2. Convert a config **struct** into a **functional-options** constructor: `NewServer(addr, ...Option)` with `WithTimeout`, `WithMaxConn`, `WithLogger`, and defaults. Add a `WithTimeout` that returns an *error* for a negative duration (`Option func(*Server) error`).
3. Build a fluent `QueryBuilder` (`Where`/`Limit`/`Build`). Then rewrite the same thing with functional options and write one sentence on which reads better and why.
4. Write a `NewStore(kind)` factory returning a `Store` interface (`memory`/`redis`), then refactor it into a **registry** where a driver registers itself from `init()`. Import the driver for side effects only (`import _ "…/redisdriver"`).
5. Implement a lazy `Config()` singleton with `sync.Once`; then rewrite the calling code to take `*Config` as a parameter instead, and note what got easier to test.
6. Use `sync.Pool` to reuse a `*bytes.Buffer` in a hot function; run it under `-race` and confirm you always `Reset()` before use and never touch the buffer after `Put`.
7. Give a type with a `map` and a `slice` field a correct deep `Clone()`; mutate the clone and assert the original is untouched.

## Best Practices & Pitfalls
- **Design for a useful zero value first.** The best constructor is the one you didn't need to write.
- **Return structs, accept interfaces.** Only a *factory* returns an interface — because hiding the concrete type is its whole job.
- **Defaults belong in the constructor**, applied *before* options. Options express intent ("with TLS"), not "remember to also set these five fields."
- **Adding an option is backward-compatible; adding a required struct field is not.** That's the core reason functional options win for public APIs.
- **Pitfall — builder that forgets to `return q`.** Then `NewQuery("t").Where(...)` mutates but the chain returns `nil` on the next call. Every chained method must `return` the receiver.
- **Pitfall — singleton creep.** A `sync.Once` global that other code reaches for directly becomes an untestable dependency magnet. If two tests interfere through it, that's the smell.
- **Pitfall — `sync.Pool` misuse.** Objects come out dirty (must `Reset`), can vanish on GC (not a fixed-size pool), and must never be used after `Put`. It's a GC optimisation, not a connection pool.
- **Pitfall — shallow `Clone`.** Copying a struct with slice/map/pointer fields shares that inner state; a "clone" that mutates the original is a nasty bug. Deep-copy the reference-typed fields explicitly.
- **Pitfall — over-patterning.** Most Go construction is a `New` func and maybe options. If you're writing an `AbstractFactoryFactory`, stop and thread a dependency instead.

## Checklist
- [ ] I make the zero value useful when I can, and only add a constructor when construction does real work.
- [ ] I can write a `NewT` that validates invariants and returns `(*T, error)`.
- [ ] I can implement functional options with defaults, including an option that can fail.
- [ ] I can write a fluent builder and say when I'd use it over options.
- [ ] I can write a factory that returns an interface, and a registry that's open for extension via `init()`.
- [ ] I can write a `sync.Once` singleton — and I can justify choosing dependency injection instead.
- [ ] I can use `sync.Pool` correctly (reset on get, no use after put) and know its limits.
- [ ] I can write a correct deep `Clone()` for a type with reference-typed fields.

## Resources
- Rob Pike & Dave Cheney on functional options: https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html · https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
- Effective Go — allocation & constructors: https://go.dev/doc/effective_go#allocation_new
- `sync.Once`, `sync.Pool`: https://pkg.go.dev/sync#Once · https://pkg.go.dev/sync#Pool
- Registry pattern in the stdlib: `sql.Register` https://pkg.go.dev/database/sql#Register · `image.RegisterFormat` https://pkg.go.dev/image#RegisterFormat
- Go Patterns (community catalog): https://github.com/tmrts/go-patterns
- Next: [29 — Structural & Composition](29-patterns-structural.md).
