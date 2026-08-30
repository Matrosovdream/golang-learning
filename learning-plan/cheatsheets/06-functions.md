# Functions Cheatsheet

**Lessons:** [06 — Functions](../06-functions.md)
**Examples:** [06](../examples/06-functions/)
**Covers:** declarations, multiple returns, variadic, closures, function values, `defer`/`panic`/`recover`
**Legend:** `[*]` = real Go feature that the lesson has not covered yet

## DECLARING

```text
func f() {}                  no params, no return
func f(a int, b string) {}   typed params
func f(a, b int) {}          shared type, written once
func f() int { return 0 }    one result
func f() (int, error)        several results — the Go signature
func f() (n int, err error)  NAMED results: declared and zero-valued
func f(...) (err error) {}   named result + bare `return` returns its value
func (r T) m() {}            a method — see the interfaces sheet
func f[T any](v T) T     [*] generic function — see the generics sheet
(functions are package-level or literals; no nested func declarations)
```

## RETURNING MULTIPLE VALUES

```text
return a, b                  results in declaration order
v, err := f()                the caller must handle both
_, err := f()                discard a result with the blank identifier
v, err := f(); if err != nil the canonical shape
return                       bare return with named results ("naked return")
(error is ALWAYS the last result; results are values, not exceptions)
```

## VARIADIC

```text
func sum(nums ...int) int    nums is an []int inside the function
sum(1, 2, 3)                 pass any number of args
sum()                        legal: nums is nil, len 0
sum(slice...)                spread an existing slice
func f(a string, rest ...int)  variadic must be the LAST parameter
fmt.Printf(format, args...)  the stdlib's most-used variadic shape
(the callee can modify the backing array when you spread a slice)
```

## CLOSURES

```text
func() { fmt.Println(x) }    a literal capturing x from the enclosing scope
counter := func() func() int  a function returning a function
  { n := 0; return func() int { n++; return n } }
                             each call to the outer func gets its own n
go func(){ ... }()           the goroutine form — captures by reference
defer func(){ ... }()        the defer form — sees final values
callbacks, middleware, options — all built on closures
(captured variables are shared, not copied; per-iteration since Go 1.22)
```

## FUNCTIONS AS VALUES

```text
var f func(int) int          a function-typed variable; zero value is nil
f = double                   assign a named function
f(3)                         call through the variable
type Handler func(w, r)      a named function type
func (f Handler) ServeHTTP() adapter trick: give a func type a method
func apply(nums []int, fn func(int) int)   take a function as a parameter
sort.Slice(s, func(i, j int) bool { ... })  the classic callback
(calling a nil function value panics)
```

## defer

```text
defer f()                    runs when the FUNCTION returns, however it returns
defer mu.Unlock()            immediately after Lock — the pairing idiom
defer file.Close()           but check the error on writers
defer resp.Body.Close()      after checking err, not before
LIFO order                   last registered, first run
args evaluated at defer time  the CALL happens later
defer func(){ ... }()        closure form: reads final variable state
named result + defer         a defer can CHANGE the returned value
(defers still run during a panic — that's the whole point)
```

## panic & recover

```text
panic("message")             unwind the stack, running defers as it goes
panic(err)                   any value works; an error is most useful
recover()                    inside a deferred func: stop the panic, get the value
if r := recover(); r != nil  the guard shape
runtime errors panic         nil deref, index out of range, /0 on ints
os.Exit(1)                   NOT a panic: no defers, no recover
(recover only works when called DIRECTLY by a deferred function)
```

## PATTERNS (shape, not API)

```text
Guard clause       if err != nil { return ..., err } — keep the happy path flat
Error wrapping     return fmt.Errorf("load user: %w", err)
Constructor        func New(...) (*T, error) — return a ready-to-use value
Functional option  func WithTimeout(d) Option — see the patterns sheet
Middleware         func(next Handler) Handler — a closure over next
Callback           pass behaviour in as a func parameter
Cleanup            defer at the top, right after acquiring the resource
Recover boundary   recover at a goroutine/handler edge, then log and continue
```

## TRAPS & MEMORIZE

```text
defer in a loop              accumulates to the end of the FUNCTION
defer f(x)                   x is captured now; use a closure for the latest value
naked returns                fine in 3 lines, unreadable in 30
recover outside a defer      returns nil and does nothing
recover in a nested func     doesn't stop the panic — must be the deferred one
a panic in a goroutine       crashes the whole program; recover per goroutine
returning a pointer to local  perfectly safe — Go moves it to the heap
nil func value               calling it panics
ignoring the error result    the one habit that hides real bugs
variadic slice spread        sum(s...) can be mutated by the callee
```
