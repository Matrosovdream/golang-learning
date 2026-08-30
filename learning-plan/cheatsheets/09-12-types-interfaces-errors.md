# Structs, Methods, Interfaces & Errors Cheatsheet

**Lessons:** [09 — Structs](../09-structs.md) · [10 — Pointers & Methods](../10-pointers-methods.md) · [11 — Interfaces](../11-interfaces.md) · [12 — Errors](../12-errors.md)
**Examples:** [09](../examples/09-structs/) · [10](../examples/10-pointers-methods/) · [11](../examples/11-interfaces/) · [12](../examples/12-errors/)
**Covers:** struct literals & tags, pointers, method sets, embedding, interface satisfaction, the `errors` package
**Legend:** `[*]` = real Go API that the lessons have not covered yet

## STRUCTS

```text
type User struct { ID int; Name string }    the declaration
u := User{ID: 1, Name: "Ann"}   keyed literal — always use this form
u := User{1, "Ann"}          positional: breaks when fields are added
var u User                   every field at its zero value
u.Name                       field access (works through pointers too)
p := &User{Name: "Ann"}      pointer to a fresh struct — the constructor shape
new(User)                    *User with zero fields (rare; &User{} reads better)
struct{}{}                   the empty struct: zero bytes, used as a signal
u1 == u2                     comparable IF every field is comparable
anon := struct{ A int }{1} [*] an anonymous struct — handy in tests
(a struct is a VALUE: assigning or passing it copies every field)
```

## STRUCT TAGS

```text
Name string `json:"name"`    rename the field in JSON
Email string `json:"email,omitempty"`   drop it when empty
Secret string `json:"-"`     never marshal this field
ID int `json:"id" db:"id"`   several tags, space-separated
Age int `json:",string"` [*] encode the number as a JSON string
`validate:"required,email"` [*] third-party validators read tags the same way
(tags are just strings read by reflection — a typo fails silently)
```

## EMBEDDING (composition, not inheritance)

```text
type Admin struct { User; Level int }    embed by TYPE, no field name
a.Name                       promoted from the embedded User
a.User.Name                  the explicit path (always available)
a := Admin{User: User{...}}  the literal must name the embedded type
type S struct { *sync.Mutex }  [*] embedding a pointer works too
type Repo struct { DB }      embedded interface: satisfy it by delegation
outer method wins            an Admin.String() shadows User.String()
(embedding promotes METHODS too — that's how interfaces get satisfied)
```

## POINTERS

```text
&x                           the address of x -> *T
*p                           the value at p (dereference)
p.Name                       auto-dereference: (*p).Name is never needed
var p *User                  nil pointer; dereferencing it panics
new(T)                       allocate a zero T, return *T
p == nil                     always check before dereferencing
func f(u *User)              the callee can MUTATE the caller's struct
func f(u User)               the callee gets a copy
&slice[0]                [*] a pointer into a slice's backing array
(no pointer arithmetic; escape analysis decides stack vs heap, not you)
```

## METHODS & RECEIVERS

```text
func (c Counter) Value() int    VALUE receiver: operates on a copy
func (c *Counter) Inc()         POINTER receiver: mutates the original
c.Inc()                      Go auto-takes &c when c is addressable
p.Value()                    Go auto-dereferences p
methods on named types only  not on int, and not on other packages' types
one receiver name per type   pick a short one and use it everywhere
func (c *Counter) String() string   satisfying fmt.Stringer
```

## THE METHOD SET RULE (memorize)

```text
value T holds                methods with VALUE receivers
pointer *T holds             methods with value AND pointer receivers
so: *T satisfies more interfaces than T does
var s Shape = Circle{}       OK if Area() has a value receiver
var s Shape = Circle{}       COMPILE ERROR if Area() has a pointer receiver
var s Shape = &Circle{}      that one works
map/slice elements           not addressable: m["k"].Inc() won't compile
rule of thumb                if ANY method needs a pointer, give them ALL pointers
```

## METHOD VALUES & EXPRESSIONS

```text
f := u.Name                  a METHOD VALUE: the receiver is bound NOW, and copied
                             if the receiver is a value type
f()                          calls it later, on that captured receiver
g := (*User).SetName         a METHOD EXPRESSION: an ordinary func whose FIRST
                             parameter is the receiver
g(&u, "Ann")                 so it can be passed where a func(*User, string) is wanted
http.HandlerFunc(h.ServeIt)  the everyday use: a bound method as a callback
(a method value on a value receiver snapshots the receiver — later mutations are
 invisible to it; use a pointer receiver if that matters)
```

## POINTER TRAPS WORTH MEMORIZING

```text
append can move the array    p := &s[0]; s = append(s, x)  ->  p may now point at
                             the OLD array. Never hold a pointer across an append.
range gives you a COPY       for _, v := range s { v.N++ } changes nothing;
                             use s[i].N++ , or range over the index
map values aren't addressable  m[k].Field = v won't compile
                             use map[K]*V, or read-modify-write the whole value
&loopVar                     safe since Go 1.22 (per-iteration variables);
                             before that, every pointer aliased the same variable
pointer as a map key         map[*T]bool keys on IDENTITY, not contents —
                             exactly what you want for a set of live objects
*T vs T in a slice           []*T lets you mutate in place; []T is denser and
                             cheaper for the GC (see the low-latency sheet)
weak.Pointer[T]          [*] Go 1.24+: reference a value without keeping it alive —
                             for caches that must not leak
unsafe.Pointer           [*] the escape hatch: no type safety, no GC guarantees
```

## INTERFACES

```text
type Shape interface { Area() float64 }   a set of method signatures
implicit satisfaction        no `implements` keyword — matching methods is enough
var s Shape = Circle{}       assignment is the check
any                          alias for interface{} — satisfied by everything
var _ Shape = (*Circle)(nil) the compile-time assertion idiom
accept interfaces, return structs        the API design rule
keep interfaces small        1–3 methods; define them where they're USED
type Reader interface { Read(p []byte) (int, error) }   the stdlib's shape
optional interface           feature detection: take the basic interface, then ask
                             for more at runtime
  if f, ok := w.(http.Flusher); ok { f.Flush() }
  if rf, ok := dst.(io.ReaderFrom); ok { return rf.ReadFrom(src) }
                             — how io.Copy uses a fast path when one exists,
                             and how a decorator can pass capabilities through
```

## TYPE ASSERTIONS & SWITCHES

```text
v := x.(Circle)              assert; PANICS if x isn't a Circle
v, ok := x.(Circle)          the comma-ok form: never panics
v, ok := x.(Shape)           assert to another interface
switch v := x.(type) {       type switch: v has the branch's type
case Circle: ...             one concrete type
case Shape: ...              an interface type also matches
case nil: ...                x holds no value at all
default: ...                 everything else
}
```

## THE NIL INTERFACE TRAP

```text
an interface is 2 words      (type, value)
iface == nil                 only when BOTH are nil
var p *MyErr = nil           a typed nil pointer
var e error = p              e != nil — type is set, value is nil
return p                     the classic bug: a nil *MyErr becomes a non-nil error
fix                          return nil explicitly, or keep the concrete type out
```

## THE STDLIB INTERFACES WORTH KNOWING

```text
error                        Error() string
fmt.Stringer                 String() string — used by %v and Println
io.Reader / io.Writer        Read(p) (n, err) / Write(p) (n, err)
io.Closer / io.ReadWriter    Close() error / both of the above
sort.Interface           [*] Len, Less, Swap
json.Marshaler           [*] MarshalJSON() ([]byte, error)
http.Handler                 ServeHTTP(w, r)
driver.Valuer / sql.Scanner  [*] custom DB column types
```

## ERRORS: creating

```text
errors.New("not found")      a simple sentinel value
fmt.Errorf("open %s: %w", name, err)     WRAP: keeps the cause in the chain
fmt.Errorf("bad input: %v", err)         formats but BREAKS the chain
var ErrNotFound = errors.New("not found")   package-level sentinel
type NotFoundError struct { ID int }     a custom error type ...
func (e *NotFoundError) Error() string   ... needs this one method
errors.Join(err1, err2)  [*] Go 1.20+: several errors as one
func (e *E) Unwrap() error [*] make a custom type wrappable
```

## ERRORS: inspecting

```text
if err != nil { return err } handle or return — never both, never neither
errors.Is(err, ErrNotFound)  is this sentinel anywhere in the chain?
var e *NotFoundError; errors.As(err, &e)    pull a typed error out of the chain
errors.Unwrap(err)       [*] one step down the chain
err == ErrNotFound           WRONG once anything wraps it — use errors.Is
_ = f()                      explicitly ignoring an error (say why in a comment)
errors.Is(err, context.Canceled)   the shape for cancellation checks
```

## ERROR STYLE

```text
lowercase, no punctuation    "open config: permission denied"
add context going up         each layer prepends what IT was doing
wrap with %w at boundaries   so callers can still use Is/As
don't log AND return         the caller will log it once
sentinel for expected cases  ErrNotFound, ErrAlreadyExists
custom type when you need data   status codes, field names, IDs
panic only for programmer bugs   not for a bad request body
(the error message is read by a human at 3am — make it say what failed)
```

## TRAPS & MEMORIZE

```text
positional struct literals   break silently when a field is inserted
copying a struct with a mutex  go vet copylocks; embed *sync.Mutex or don't copy
value receiver mutation      changes the copy and is silently lost
mixing receiver kinds        confusing method sets; pick one per type
map[K]Struct field write     m["k"].F = v doesn't compile — not addressable
interface holding a typed nil  != nil; the #1 error-handling bug
big interfaces               harder to fake in tests; keep them at 1–3 methods
defining interfaces early    define them in the CONSUMER, when you need them
%v instead of %w             loses errors.Is/As for every caller above you
comparing wrapped errors     use errors.Is, not ==
struct tag typos             fail silently — there is no compiler check
```
