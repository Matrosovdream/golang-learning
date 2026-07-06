# 30 — Design Patterns III: Behavioral & Flow (the Go way)

> Part 3 of the **Design Patterns** series: [28 Creational](28-patterns-creational.md) → [29 Structural](29-patterns-structural.md) → **30 Behavioral**.
> Behavioral patterns are about *how objects collaborate and how behaviour varies*. Go's answer is **first-class functions and small interfaces**: a strategy is a function value, a command is a closure, an iterator is a function you can `range` over. This lesson also lands the modern iterator: **range-over-func** (`iter.Seq`, Go 1.23+).

## Goals
- Make behaviour pluggable with **first-class functions** and one-method interfaces: Strategy, Command, Template Method.
- Model change and notification: **Observer/pub-sub** and **State machines**.
- Iterate the modern way with **range-over-func** (`iter.Seq`/`iter.Seq2`), and know the older channel/`Next()` iterators and their trade-offs.
- Dispatch by shape: **Visitor** via type switch, **Chain of Responsibility** via a handler chain.

## Concepts

### Strategy — behaviour as a value
The single most useful behavioral idea in Go: pass the varying behaviour as a **function value** (or a tiny interface). No `StrategyFactory`, no class per strategy:
```go
type PricingStrategy func(base float64) float64

func Regular(base float64) float64 { return base }
func Member(base float64) float64  { return base * 0.90 }
func Clearance(base float64) float64 { return base * 0.50 }

type Checkout struct{ price PricingStrategy }
func (c Checkout) Total(base float64) float64 { return c.price(base) }

// c := Checkout{price: Member}; c.Total(100) → 90
```
The stdlib leans on this everywhere — `sort.Slice`'s `less` closure *is* the strategy:
```go
sort.Slice(people, func(i, j int) bool { return people[i].Age < people[j].Age })
// Go 1.21+ generic form:
slices.SortFunc(people, func(a, b Person) int { return cmp.Compare(a.Age, b.Age) })
```
Use a **one-method interface** instead of a func only when the strategy needs state or a name it can be identified by (e.g. `Compressor` with `Compress`/`Name`).

### Template Method — fixed skeleton, pluggable steps (and the embedding trap)
Template Method keeps an algorithm's outline fixed while subclasses fill in steps. In Java that's inheritance + overriding. **In Go that classic form does not work**, because embedding is not virtual dispatch — the base method calls the base's steps, never the outer type's:
```go
// ❌ Looks like Template Method — behaves wrong.
type Base struct{}
func (Base) Step() string  { return "base step" }
func (b Base) Run() string { return "run → " + b.Step() } // ALWAYS calls Base.Step

type Derived struct{ Base }
func (Derived) Step() string { return "derived step" }

// Derived{}.Run() → "run → base step"   (NOT "derived step")
```
The idiomatic fix: inject the varying steps as a **func or interface** — pass them in rather than trying to override them:
```go
// ✅ Skeleton takes the varying step as a dependency.
type Step interface{ Do() string }

func Run(s Step) string { return "run → " + s.Do() } // dispatches to whatever you pass

type derivedStep struct{}
func (derivedStep) Do() string { return "derived step" }
// Run(derivedStep{}) → "run → derived step"
```
Or, for a fixed pipeline with a couple of hooks, pass the hooks as function parameters (`func process(data []byte, validate func([]byte) error, transform func([]byte) []byte)`).

### Command — a request captured as a value
Wrap "an action plus its arguments" as something you can store, queue, log, or undo. A closure is often enough:
```go
type Command func() error

queue := []Command{
    func() error { fmt.Println("resize"); return nil },
    func() error { fmt.Println("email");  return nil },
}
for _, cmd := range queue {
    if err := cmd(); err != nil { /* handle */ }
}
```
For **undo**, make it an interface with an inverse operation:
```go
type Editor struct{ text string }

type Command interface {
    Do(*Editor)
    Undo(*Editor)
}

type appendText struct{ s string }
func (c appendText) Do(e *Editor)   { e.text += c.s }
func (c appendText) Undo(e *Editor) { e.text = e.text[:len(e.text)-len(c.s)] }

// history []Command; push on Do, pop+Undo to reverse.
```
This is how editors, transaction logs, and job queues get replay/undo: the *what* becomes data.

### Observer / Pub-Sub — notify interested parties on change
An **observer** registers a callback; the subject calls them all when something happens. The synchronous, in-process form is a slice of funcs:
```go
type Event struct{ Name string }
type Observer func(Event)

type Subject struct{ observers []Observer }
func (s *Subject) Subscribe(o Observer)  { s.observers = append(s.observers, o) }
func (s *Subject) Notify(e Event) {
    for _, o := range s.observers { o(e) } // fan out to every subscriber
}
```
When observers must be **decoupled** (different goroutines, slow consumers, backpressure), switch the callback for a **channel** per subscriber — that's the pub/sub `EventHub` from [lesson 15](15-sync-context.md). Rule of thumb: **callbacks for synchronous same-goroutine hooks; channels when you need decoupling or concurrency.**

### State — an explicit state machine
Stop scattering `if status == …` checks; model states and transitions as data. The compact form is a transition table:
```go
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
```
When each state carries its own behaviour (not just a next-state), make **each state a type** implementing a `State` interface with a `Handle` method — the "state as object" form. The table version is best when transitions dominate; the interface version when per-state behaviour dominates.

### Iterator — the modern way: range-over-func (Go 1.23+)
Go 1.23 lets you `range` over a **function**. An iterator is a func that calls `yield` once per element; returning `false` from `yield` means the consumer did `break`. The `iter` package names the shapes:
```go
import "iter"

// iter.Seq[T]  = func(yield func(T) bool)
func Count(n int) iter.Seq[int] {
    return func(yield func(int) bool) {
        for i := 0; i < n; i++ {
            if !yield(i) {   // consumer broke out → stop producing
                return
            }
        }
    }
}

// consume it like any range:
for v := range Count(3) {
    fmt.Println(v)           // 0, 1, 2
}
```
Key/value iteration uses `iter.Seq2[K, V]`:
```go
func Enumerate[T any](s []T) iter.Seq2[int, T] {
    return func(yield func(int, T) bool) {
        for i, v := range s {
            if !yield(i, v) { return }
        }
    }
}
// for i, v := range Enumerate(names) { ... }
```
This is the pattern the stdlib now uses (`maps.Keys`, `slices.All`, `strings.Lines`, `bytes.Lines`). It composes, needs no goroutine, and **can't leak** — the loop body drives production synchronously.

**The older iterators**, still worth knowing:
```go
// Channel iterator — decoupled, but LEAKS the goroutine if the caller breaks early
// and never drains ch. Prefer range-over-func unless you truly need concurrency.
func countChan(n int) <-chan int {
    ch := make(chan int)
    go func() {
        defer close(ch)
        for i := 0; i < n; i++ { ch <- i }
    }()
    return ch
}

// "Next()" iterator — the classic stateful cursor: bufio.Scanner, sql.Rows.
// for sc.Scan() { line := sc.Text() }   ;  check sc.Err() at the end.
```

### Chain of Responsibility — pass the request along until one handles it
A request travels a chain; each link either handles it or passes it on. The HTTP middleware chain from [lesson 29](29-patterns-structural.md) is exactly this. A data-handler form:
```go
type Handler func(req string) (handled bool)

func chain(handlers ...Handler) Handler {
    return func(req string) bool {
        for _, h := range handlers {
            if h(req) {          // first handler that claims it wins
                return true
            }
        }
        return false             // nobody handled it
    }
}
```

### Visitor — double dispatch without inheritance
When you must run type-specific logic over a heterogeneous set of nodes, Go's idiom is a **type switch** over an interface — no accept/visit boilerplate needed:
```go
type Expr interface{ isExpr() }
type Num struct{ V float64 }
type Add struct{ L, R Expr }
type Mul struct{ L, R Expr }

func (Num) isExpr() {} // unexported marker method = "sealed" interface
func (Add) isExpr() {}
func (Mul) isExpr() {}

func Eval(e Expr) float64 {
    switch n := e.(type) {          // type switch = idiomatic double dispatch
    case Num:
        return n.V
    case Add:
        return Eval(n.L) + Eval(n.R)
    case Mul:
        return Eval(n.L) * Eval(n.R)
    default:
        panic(fmt.Sprintf("unknown expr %T", e))
    }
}
// Eval(Add{Num{2}, Mul{Num{3}, Num{4}}}) → 14
```
Use the *classic* visitor (an `Accept(v Visitor)` method on each node) only when you must add many new operations to a stable set of node types and want the compiler to force you to handle every node — Go's `go/ast.Walk`/`Visitor` does this. Otherwise the type switch is lighter.

### Mediator & Memento (briefly)
- **Mediator** — a hub that coordinates peers so they don't reference each other directly. In Go it's usually a struct holding the participants (a chat room owning its clients, a scheduler owning its workers). The actor/`EventHub` pattern from lesson 15 is a mediator.
- **Memento** — capture an object's state so it can be restored later (undo, snapshots). In Go: a `Snapshot()` returning an opaque value and a `Restore(snapshot)` — often just a deep `Clone()` (the Prototype link from [lesson 28](28-patterns-creational.md)).

### Which patterns Go folds into the language
- **Strategy / Command** → a function value; a `[]func() error` is a command queue.
- **Iterator** → range-over-func; you rarely hand-roll a cursor anymore.
- **Observer** → a slice of callbacks, or channels when decoupling matters.
- **Chain of Responsibility** → a slice of handlers / middleware chain.
- **Template Method** → inject steps as funcs/interfaces (embedding won't override).

## Exercises
1. Implement `Checkout` with a `PricingStrategy` func field; swap `Regular`/`Member`/`Clearance` at the call site. Then re-sort a slice two ways with `slices.SortFunc` — the comparator is your strategy.
2. Reproduce the **embedding trap**: a `Base.Run` that calls `Base.Step`, a `Derived` that "overrides" `Step`, and show `Derived{}.Run()` still calls the base. Then fix it by injecting a `Step` interface into a `Run(s Step)` function.
3. Build an undoable `Editor`: a `Command` interface with `Do`/`Undo`, an `appendText` command, a history stack, and prove `Undo` reverses the last change.
4. Write a `Subject` with `Subscribe(Observer)`/`Notify(Event)` (callback slice). Then rewrite it channel-based and describe when you'd choose each.
5. Model a tiny state machine (`idle`/`running`/`paused`) as a transition table; feed it a sequence of events and reject an invalid transition with an error.
6. Write a range-over-func iterator `Count(n) iter.Seq[int]` and consume it with `for v := range`. Add early `break` and confirm production stops. Then write `Enumerate[T]([]T) iter.Seq2[int, T]`. (Needs Go 1.23+.)
7. Write the leaky **channel iterator**, `break` out of its range early, and (with `runtime.NumGoroutine` or `-race` + a blocking send) observe the goroutine that never finishes — then contrast with the range-over-func version that can't leak.
8. Evaluate an arithmetic expression tree (`Num`/`Add`/`Mul`) with a **type-switch visitor**; add a `Sub` node and let the `default` panic remind you to handle it.

## Best Practices & Pitfalls
- **A function value is the default strategy/command in Go.** Introduce an interface only when the behaviour needs state, a name, or multiple methods.
- **Embedding does not override.** Any pattern that relies on a base method calling a subclass override (Template Method, some Visitor forms) must inject the varying behaviour instead. This is the #1 pattern surprise coming from OO languages.
- **Prefer range-over-func iterators** (Go 1.23+): composable, allocation-light, and leak-free. Reach for a channel iterator only when you genuinely need concurrent production.
- **Pitfall — leaking a channel iterator.** If the consumer `break`s and never drains, the producer goroutine blocks forever on send. Always provide cancellation (a `done`/`ctx`) or use range-over-func.
- **Pitfall — observer callbacks that block or panic.** A synchronous `Notify` runs every observer on the caller's goroutine; one slow or panicking observer stalls or crashes the subject. Recover per-observer, or decouple with channels.
- **Pitfall — implicit state machines.** Scattered boolean flags (`isRunning`, `isPaused`) encode a state machine badly and allow impossible states. Make states and transitions explicit.
- **Pitfall — non-exhaustive type switch.** Adding a node type and forgetting a `case` fails silently unless your `default` panics or you use a sealed interface + a linter. Make the `default` loud.
- **Pitfall — over-abstraction.** If a "strategy" has one implementation forever, it's just a function call. Don't build the pattern until variation actually exists.

## Checklist
- [ ] I use a function value (or one-method interface) for Strategy and Command, and can build a `[]Command` queue.
- [ ] I can explain and fix the embedding/Template-Method trap by injecting steps.
- [ ] I can implement undo with a `Do`/`Undo` command and a history stack.
- [ ] I can build an observer both as a callback slice and channel-based, and choose between them.
- [ ] I can model an explicit state machine with a transition table (or state-as-interface).
- [ ] I can write a range-over-func iterator (`iter.Seq`/`iter.Seq2`) and know why it can't leak.
- [ ] I can implement Chain of Responsibility as a handler chain, and Visitor as a type switch.

## Resources
- Range-over-func & iterators: https://go.dev/blog/range-functions · `iter` package: https://pkg.go.dev/iter
- `slices` / `maps` / `cmp` (strategy comparators, iterator producers): https://pkg.go.dev/slices · https://pkg.go.dev/cmp
- `go/ast` `Visitor`/`Walk` (classic visitor in the stdlib): https://pkg.go.dev/go/ast#Walk
- Rob Pike — "Lexical Scanning in Go" (state functions as a behavioral pattern): https://go.dev/talks/2011/lex.slide
- Go Patterns (behavioral): https://github.com/tmrts/go-patterns#behavioral-patterns
- Previous: [29 — Structural](29-patterns-structural.md) · Series start: [28 — Creational](28-patterns-creational.md).
