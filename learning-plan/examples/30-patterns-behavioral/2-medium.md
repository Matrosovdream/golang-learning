# 30 · Medium (7–11) — interface strategy, undo, template method, visitor

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 7. Strategy as an interface

Reach for a one-method interface over a plain func when the strategy needs state or a name it can be
identified by.

```go
package main

import "fmt"

type Compressor interface {
	Name() string
	Compress(data string) string
}

type gzipC struct{}

func (gzipC) Name() string             { return "gzip" }
func (gzipC) Compress(d string) string { return "gzip(" + d + ")" }

type noneC struct{}

func (noneC) Name() string             { return "none" }
func (noneC) Compress(d string) string { return d }

type Uploader struct{ c Compressor }

func (u Uploader) Upload(d string) { fmt.Printf("[%s] %s\n", u.c.Name(), u.c.Compress(d)) }

func main() {
	Uploader{c: gzipC{}}.Upload("payload")
	Uploader{c: noneC{}}.Upload("payload")
}
```

**Output**
```
[gzip] gzip(payload)
[none] payload
```

---

## 8. Command with undo + history

Make the command an interface with an inverse operation and keep a history stack — how editors and
transaction logs get undo/replay.

```go
package main

import "fmt"

type Editor struct{ text string }

type Command interface {
	Do(*Editor)
	Undo(*Editor)
}

type appendText struct{ s string }

func (c appendText) Do(e *Editor)   { e.text += c.s }
func (c appendText) Undo(e *Editor) { e.text = e.text[:len(e.text)-len(c.s)] }

func main() {
	e := &Editor{}
	var history []Command
	apply := func(c Command) { c.Do(e); history = append(history, c) }

	apply(appendText{"Hello"})
	apply(appendText{", "})
	apply(appendText{"world"})
	fmt.Printf("after do:   %q\n", e.text)

	for i := 0; i < 2; i++ { // undo the last two commands
		last := history[len(history)-1]
		history = history[:len(history)-1]
		last.Undo(e)
	}
	fmt.Printf("after undo: %q\n", e.text)
}
```

**Output**
```
after do:   "Hello, world"
after undo: "Hello"
```

---

## 9. The Template-Method embedding trap

The #1 surprise coming from OO languages: **embedding is not virtual dispatch**. A base method calls
the base's own step, never the "override" on the outer type.

```go
package main

import "fmt"

type Base struct{}

func (Base) Step() string  { return "base step" }
func (b Base) Run() string { return "run → " + b.Step() } // ALWAYS calls Base.Step

type Derived struct{ Base }

func (Derived) Step() string { return "derived step" }

func main() {
	fmt.Println("Derived.Run(): ", Derived{}.Run())  // run → base step  (NOT derived)
	fmt.Println("Derived.Step():", Derived{}.Step()) // derived step
}
```

**Output**
```
Derived.Run():  run → base step
Derived.Step(): derived step
```

> `Derived` "overrides" `Step`, but `Base.Run` still calls `Base.Step`. Classic Java-style Template
> Method **does not work** via embedding. The fix is next.

---

## 10. Template Method, fixed by injection

Inject the varying step as an interface (or func) instead of trying to override it via embedding.

```go
package main

import "fmt"

type Step interface{ Do() string }

func Run(s Step) string { return "run → " + s.Do() } // dispatches to whatever you pass

type baseStep struct{}

func (baseStep) Do() string { return "base step" }

type derivedStep struct{}

func (derivedStep) Do() string { return "derived step" }

func main() {
	fmt.Println(Run(baseStep{}))
	fmt.Println(Run(derivedStep{}))
}
```

**Output**
```
run → base step
run → derived step
```

---

## 11. Visitor via a type switch

Idiomatic double dispatch with no accept/visit boilerplate. The unexported marker method "seals" the
`Expr` interface so only types in this package can implement it.

```go
package main

import "fmt"

type Expr interface{ isExpr() }
type Num struct{ V float64 }
type Add struct{ L, R Expr }
type Mul struct{ L, R Expr }

func (Num) isExpr() {}
func (Add) isExpr() {}
func (Mul) isExpr() {}

func Eval(e Expr) float64 {
	switch n := e.(type) {
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

func main() {
	// 2 + 3 * 4
	expr := Add{L: Num{2}, R: Mul{L: Num{3}, R: Num{4}}}
	fmt.Println("2 + 3*4 =", Eval(expr))
}
```

**Output**
```
2 + 3*4 = 14
```

---

Next tier → [Hard (12–16)](3-hard.md)
