# 49 · Easy (1–5) — the core kinds

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

Put each example's files in its own directory (`u01`, `u02`, …) and run `go test -v ./uNN`.

---

## 1. A unit test (`Error` vs `Fatal`)

The base kind: `func TestXxx(t *testing.T)`. Use `t.Errorf` to record a failure and keep going;
`t.Fatalf` to record it and stop *this* test (when continuing is pointless). Always put inputs, got, and
want in the message.

`u01/calc.go`
```go
package calc

import "errors"

func Add(a, b int) int { return a + b }

func Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a / b, nil
}
```

`u01/calc_test.go`
```go
package calc

import "testing"

func TestAdd(t *testing.T) {
	got := Add(2, 3)
	if got != 5 { // t.Errorf: mark failed but keep going
		t.Errorf("Add(2, 3) = %d; want 5", got)
	}
}

func TestDiv(t *testing.T) {
	got, err := Div(10, 2)
	if err != nil { // t.Fatalf: no point checking got if this failed
		t.Fatalf("Div(10, 2) returned error: %v", err)
	}
	if got != 5 {
		t.Errorf("Div(10, 2) = %d; want 5", got)
	}
}
```

Run: `go test -v ./u01`

**Output**
```
=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestDiv
--- PASS: TestDiv (0.00s)
PASS
ok  	scratch/u01	0.2s
```

`go test` finds every `TestXxx` in `*_test.go` files in the same directory. Drop `-v` for just the
`ok`/`FAIL` summary. There's no assertion library — idiomatic Go is `if got != want { t.Errorf(...) }`.

---

## 2. Table-driven tests & subtests

The dominant Go style: a slice of cases + a loop of `t.Run(name, …)` **subtests**. Each case gets its own
name in the output, fails independently, and is individually targetable with `-run`.

`u02/abs.go`
```go
package tabular

func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
```

`u02/abs_test.go`
```go
package tabular

import "testing"

func TestAbs(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"positive", 3, 3},
		{"negative", -3, 3},
		{"zero", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { // each case is its own subtest
			if got := Abs(tt.in); got != tt.want {
				t.Errorf("Abs(%d) = %d; want %d", tt.in, got, tt.want)
			}
		})
	}
}
```

Run: `go test -v ./u02`

**Output**
```
=== RUN   TestAbs
=== RUN   TestAbs/positive
=== RUN   TestAbs/negative
=== RUN   TestAbs/zero
--- PASS: TestAbs (0.00s)
    --- PASS: TestAbs/positive (0.00s)
    --- PASS: TestAbs/negative (0.00s)
    --- PASS: TestAbs/zero (0.00s)
PASS
```

Target one case with `go test -run 'TestAbs/negative' ./u02`. Adding a case is one struct literal — this
is why table-driven is the default. (The subtest names are slash-joined, so `-run` takes a regexp path.)

---

## 3. Black-box vs white-box

Two ways to place a test. **White-box** (`package foo`) sees unexported internals. **Black-box**
(`package foo_test`, same directory) imports the package and sees only its **exported** API, exactly as a
consumer would — preferred when you can, because it keeps tests refactor-proof. Both live in the same
directory and run together.

`u03/stack.go`
```go
package stack

type Stack struct {
	items []int // unexported: only same-package (white-box) tests can see it
}

func (s *Stack) Push(v int) { s.items = append(s.items, v) }

func (s *Stack) Pop() (int, bool) {
	if len(s.items) == 0 {
		return 0, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

func (s *Stack) Len() int { return len(s.items) }
```

`u03/stack_internal_test.go` — white-box
```go
package stack // white-box: same package, can touch unexported internals

import "testing"

func TestInternalGrowth(t *testing.T) {
	var s Stack
	s.Push(1)
	s.Push(2)
	if len(s.items) != 2 { // reaching into the unexported `items` field
		t.Errorf("internal items len = %d; want 2", len(s.items))
	}
}
```

`u03/stack_test.go` — black-box
```go
package stack_test // black-box: sees only the exported API, like a real consumer

import (
	"testing"

	stack "scratch/u03"
)

func TestPublicAPI(t *testing.T) {
	var s stack.Stack
	s.Push(10)
	got, ok := s.Pop()
	if !ok || got != 10 {
		t.Errorf("Pop() = %d, %v; want 10, true", got, ok)
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d; want 0", s.Len())
	}
}
```

Run: `go test -v ./u03`

**Output**
```
=== RUN   TestInternalGrowth
--- PASS: TestInternalGrowth (0.00s)
=== RUN   TestPublicAPI
--- PASS: TestPublicAPI (0.00s)
PASS
```

Go uniquely allows the `foo_test` external package to sit *in the same directory* as `foo`. If a black-box
test needs one internal hook, expose it from an `export_test.go` (declared `package foo`) — the tool
compiles it only under test, so it never leaks into your real API.

---

## 4. Example tests (verified docs)

`func ExampleXxx()` with a trailing `// Output:` comment is **both** documentation (rendered on
pkg.go.dev) **and** a test — `go test` runs it and compares stdout to the comment. Use
`// Unordered output:` when order isn't guaranteed (map iteration).

`u04/greet.go`
```go
package greet

func Hello(name string) string { return "Hello, " + name + "!" }
```

`u04/example_test.go`
```go
package greet_test

import (
	"fmt"

	greet "scratch/u04"
)

// go test compares stdout to the // Output: comment — verified documentation.
func ExampleHello() {
	fmt.Println(greet.Hello("Go"))
	// Output: Hello, Go!
}

// Map iteration order is random, so assert the SET with // Unordered output:.
func Example_unorderedOutput() {
	counts := map[string]int{"apple": 1, "banana": 2, "cherry": 3}
	for name := range counts {
		fmt.Println(name)
	}
	// Unordered output:
	// apple
	// banana
	// cherry
}
```

Run: `go test -v ./u04`

**Output**
```
=== RUN   ExampleHello
--- PASS: ExampleHello (0.00s)
=== RUN   Example_unorderedOutput
--- PASS: Example_unorderedOutput (0.00s)
PASS
```

Name them `ExampleType` / `ExampleType_method` to attach the snippet to that symbol in the docs. Break the
`// Output:` and the "test" fails — which is the point: your docs can't drift. An example with **no**
`// Output:` line is compiled (so it can't rot) but not run.

---

## 5. Helpers & lifecycle

The `*testing.T` lifecycle toolbox: `t.Helper()` (failures point at the caller, not inside the helper),
`t.TempDir()` (a per-test dir auto-removed after), `t.Setenv()` (set + auto-restore an env var), and
`t.Cleanup()` (LIFO teardown that composes across helpers — cleaner than `defer`).

`u05/config.go`
```go
package config

import (
	"os"
	"path/filepath"
)

// LoadName reads $APP_NAME, or falls back to name.txt in dir.
func LoadName(dir string) (string, error) {
	if v := os.Getenv("APP_NAME"); v != "" {
		return v, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "name.txt"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

`u05/config_test.go`
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a helper: t.Helper() makes a failure point at the CALLER's line.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLoadName_fromFile(t *testing.T) {
	dir := t.TempDir() // per-test dir, auto-removed at the end
	writeFile(t, dir, "name.txt", "from-file")

	got, err := LoadName(dir)
	if err != nil {
		t.Fatalf("LoadName: %v", err)
	}
	if got != "from-file" {
		t.Errorf("LoadName = %q; want %q", got, "from-file")
	}
}

func TestLoadName_fromEnv(t *testing.T) {
	t.Setenv("APP_NAME", "from-env")                          // set + auto-restored
	t.Cleanup(func() { t.Log("cleanup ran after the test") }) // LIFO teardown

	got, err := LoadName(t.TempDir())
	if err != nil {
		t.Fatalf("LoadName: %v", err)
	}
	if got != "from-env" {
		t.Errorf("LoadName = %q; want %q", got, "from-env")
	}
}
```

Run: `go test -v ./u05`

**Output**
```
=== RUN   TestLoadName_fromFile
--- PASS: TestLoadName_fromFile (0.00s)
=== RUN   TestLoadName_fromEnv
    config_test.go:32: cleanup ran after the test
--- PASS: TestLoadName_fromEnv (0.00s)
PASS
```

`t.TempDir` and `t.Setenv` remove whole categories of flaky, order-dependent tests — no leftover files, no
env var bleeding into the next test. Note `t.Setenv` forbids `t.Parallel` in the same test (a shared
process env can't be isolated across parallel tests).

---

Next tier: [🟡 Medium (6–10)](2-medium.md) — parallel, benchmarks, fuzz, HTTP, golden files.
</content>
