# 49 · Hard (11–15) — scope, doubles, tooling & capstone

Back to [index](README.md) · Prev tier: [Medium](2-medium.md)

---

## 11. `TestMain` and short mode

`func TestMain(m *testing.M)` runs **once per package** — wrap `m.Run()` with setup/teardown (open a DB,
start a server). Separately, `testing.Short()` + `-short` lets you skip slow tests for a fast CI smoke run.

`u11/store.go`
```go
package store

// A tiny in-memory store standing in for something with expensive setup.
type Store struct{ data map[string]string }

func Open() *Store               { return &Store{data: map[string]string{}} }
func (s *Store) Set(k, v string) { s.data[k] = v }
func (s *Store) Get(k string) (string, bool) {
	v, ok := s.data[k]
	return v, ok
}
func (s *Store) Close() {}
```

`u11/store_test.go`
```go
package store

import (
	"fmt"
	"os"
	"testing"
)

var testStore *Store

// TestMain runs ONCE for the whole package: setup → m.Run() → teardown.
func TestMain(m *testing.M) {
	fmt.Println("setup: opening store")
	testStore = Open()

	code := m.Run() // runs every TestXxx in the package

	fmt.Println("teardown: closing store")
	testStore.Close()
	os.Exit(code)
}

func TestSetGet(t *testing.T) {
	testStore.Set("k", "v")
	if got, _ := testStore.Get("k"); got != "v" {
		t.Errorf("Get = %q; want v", got)
	}
}

func TestSlowIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	testStore.Set("big", "payload") // pretend this hits a real database
	if _, ok := testStore.Get("big"); !ok {
		t.Error("missing")
	}
}
```

Run full, then `-short`:
```bash
go test -v ./u11
go test -short -v ./u11
```

**Output** (`-short` — note the `SKIP` and that setup/teardown still bracket the run)
```
setup: opening store
=== RUN   TestSetGet
--- PASS: TestSetGet (0.00s)
=== RUN   TestSlowIntegration
--- SKIP: TestSlowIntegration (0.00s)
PASS
teardown: closing store
ok  	scratch/u11	0.1s
```

Exactly one `TestMain` per package; it **must** call `m.Run()` and `os.Exit` with its result. Use it for
package-wide fixtures — but prefer per-test `t.Cleanup`/`t.TempDir` (#5) for anything that can be scoped
smaller.

---

## 12. Integration tests behind a build tag

Put tests that need real infra behind a `//go:build` tag so they compile only when you ask — keeping a
Docker/DB test out of the default `go test ./...`.

`u12/intg.go`
```go
package intg

func Ping() string { return "pong" }
```

`u12/unit_test.go` — always runs
```go
package intg

import "testing"

// Always compiled — part of the default unit run.
func TestPingUnit(t *testing.T) {
	if Ping() != "pong" {
		t.Error("unit failed")
	}
}
```

`u12/integration_test.go` — gated
```go
//go:build integration

// This file compiles ONLY under `go test -tags=integration`, so a test that
// needs real infra stays out of the default unit run.
package intg

import "testing"

func TestPingIntegration(t *testing.T) {
	if got := Ping(); got != "pong" {
		t.Errorf("Ping() = %q; want pong", got)
	}
}
```

Run default, then with the tag:
```bash
go test -v ./u12                    # only TestPingUnit
go test -tags=integration -v ./u12  # both
```

**Output** (default — the integration test isn't even compiled in)
```
=== RUN   TestPingUnit
--- PASS: TestPingUnit (0.00s)
PASS
```

The `//go:build integration` line (with a blank line after it, before `package`) is the constraint. This
pairs with `testcontainers-go` ([40](../../40-testing-architecture.md)) to spin up a throwaway
Postgres/Redis only under `-tags=integration`, so the default run stays fast and hermetic.

---

## 13. Property-based tests (`testing/quick`)

Instead of specific cases, assert an **invariant** over many random inputs. `testing/quick.Check` generates
values and fails if your property returns false — great for "matches a reference implementation" and
"round-trips" properties.

`u13/sortx.go`
```go
package sortx

// Sort returns a sorted copy using our own insertion sort (something to test).
func Sort(in []int) []int {
	out := append([]int(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
```

`u13/sortx_test.go`
```go
package sortx

import (
	"sort"
	"testing"
	"testing/quick"
)

// Property: our Sort agrees with the stdlib sort for EVERY generated input.
func TestSortMatchesStdlib(t *testing.T) {
	f := func(in []int) bool {
		got := Sort(in)
		want := append([]int(nil), in...)
		sort.Ints(want)
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil { // generates 100 random cases
		t.Error(err)
	}
}

// Property: Sort is idempotent — sorting an already-sorted slice changes nothing.
func TestSortIdempotent(t *testing.T) {
	f := func(in []int) bool {
		once := Sort(in)
		twice := Sort(once)
		for i := range once {
			if once[i] != twice[i] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
```

Run: `go test -v ./u13`

**Output**
```
=== RUN   TestSortMatchesStdlib
--- PASS: TestSortMatchesStdlib (0.00s)
=== RUN   TestSortIdempotent
--- PASS: TestSortIdempotent (0.00s)
PASS
```

`quick` generates 100 random inputs by default and, on failure, prints the exact input that broke the
property. It's the zero-dependency stdlib option for pure invariants; **fuzzing** (#8) is the modern,
coverage-guided cousin that also mutates toward new code paths.

---

## 14. Race detection (`-race`)

The race detector is a test *mode*, not a kind — but it catches bugs no assertion can. A plain `go test`
passes even when a test has a data race; `go test -race` instruments memory access and fails.

`u14/counter.go`
```go
package counter

import "sync"

// SafeCounter guards its state with a mutex — race-free.
type SafeCounter struct {
	mu sync.Mutex
	n  int
}

func (c *SafeCounter) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}
func (c *SafeCounter) Value() int { return c.n }

// RacyCounter has NO synchronization — a data race under concurrent Inc.
type RacyCounter struct{ n int }

func (c *RacyCounter) Inc()       { c.n++ }
func (c *RacyCounter) Value() int { return c.n }
```

`u14/counter_test.go`
```go
package counter

import (
	"sync"
	"testing"
)

func TestSafeCounter(t *testing.T) {
	var c SafeCounter
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc() }()
	}
	wg.Wait()
	if c.Value() != 100 {
		t.Errorf("Value = %d; want 100", c.Value())
	}
}

// Passes under plain `go test` (the bug hides); `go test -race` catches it.
func TestRacyCounter(t *testing.T) {
	var c RacyCounter
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc() }()
	}
	wg.Wait()
	_ = c.Value()
}
```

Plain run hides the bug: `go test ./u14` → `ok`. Now with the detector:
```bash
go test -race -run TestRacyCounter ./u14
```

**Output** (trimmed — the detector points at the exact line)
```
==================
WARNING: DATA RACE
Read at 0x00c00009c208 by goroutine 9:
      .../u14/counter.go:21 +0x6c
Previous write at 0x00c00009c208 by goroutine 8:
      .../u14/counter.go:21 +0x7c
==================
--- FAIL: TestRacyCounter (0.00s)
FAIL
```

Line 21 is `c.n++` in `RacyCounter.Inc`. The plain test **passed** — races are timing-dependent and often
invisible until production. Run `go test -race ./...` in CI for anything concurrent ([15](../../15-sync-context.md)).

---

## 15. Capstone: one package, five kinds of test

A real package is covered by several kinds at once. This `money` package is tested with a **table-driven
unit test**, an **example**, a **benchmark**, a **fuzz** round-trip, and an **HTTP** test — and we read its
**coverage**.

`u15/money.go`
```go
package money

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Money is an amount in minor units (cents).
type Money int64

func (m Money) String() string {
	sign, n := "", int64(m)
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, n/100, n%100)
}

// Parse turns "12.34" (or "-0.05") into Money.
func Parse(s string) (Money, error) {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	whole, frac, found := strings.Cut(s, ".")
	if !found {
		frac = "00"
	}
	if len(frac) != 2 {
		return 0, fmt.Errorf("bad fractional part: %q", s)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	c, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || c < 0 {
		return 0, fmt.Errorf("bad cents: %q", frac)
	}
	total := w*100 + c
	if neg {
		total = -total
	}
	return Money(total), nil
}

// ParseHandler exposes Parse over HTTP: GET /parse?amount=12.34 → 1234.
func ParseHandler(w http.ResponseWriter, r *http.Request) {
	m, err := Parse(r.URL.Query().Get("amount"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "%d", int64(m))
}
```

`u15/money_test.go`
```go
package money

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 1) Table-driven UNIT test.
func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Money
	}{
		{"12.34", 1234},
		{"0.05", 5},
		{"-0.05", -5},
		{"100", 10000},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %d; want %d", tt.in, got, tt.want)
			}
		})
	}
}

// 2) EXAMPLE test (verified documentation).
func ExampleMoney_String() {
	fmt.Println(Money(1999))
	// Output: 19.99
}

// 3) BENCHMARK.
var sink Money

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink, _ = Parse("1234.56")
	}
}

// 4) FUZZ: format→parse must round-trip for every representable amount.
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range []int64{0, 5, 1234, -5, -1999} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, n int64) {
		if n < -1_000_000 || n > 1_000_000 {
			t.Skip() // keep w*100 well inside int64
		}
		m := Money(n)
		got, err := Parse(m.String())
		if err != nil {
			t.Fatalf("Parse(%q): %v", m.String(), err)
		}
		if got != m {
			t.Errorf("round-trip: Parse(%q) = %d; want %d", m.String(), got, m)
		}
	})
}

// 5) HTTP test.
func TestParseHandler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(ParseHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/parse?amount=12.34")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "1234" {
		t.Errorf("body = %q; want 1234", body)
	}
}
```

Run with coverage: `go test -v -cover ./u15`

**Output** (abridged)
```
=== RUN   TestParse
    --- PASS: TestParse/12.34 (0.00s)
    --- PASS: TestParse/0.05 (0.00s)
    --- PASS: TestParse/-0.05 (0.00s)
    --- PASS: TestParse/100 (0.00s)
--- PASS: TestParse (0.00s)
=== RUN   TestParseHandler
--- PASS: TestParseHandler (0.00s)
=== RUN   FuzzRoundTrip
--- PASS: FuzzRoundTrip (0.00s)
=== RUN   ExampleMoney_String
--- PASS: ExampleMoney_String (0.00s)
PASS
coverage: 81.5% of statements
ok  	scratch/u15	0.4s	coverage: 81.5% of statements
```

Five kinds of test on one small package, 81.5% coverage in one run — and the fuzz round-trip actively
*generates* amounts you didn't hand-write. For a line-by-line view: `go test -coverprofile=c.out ./u15 &&
go tool cover -html=c.out`. Remember coverage shows what *ran*, not what's *correct* — the fuzz and
property tests are what push toward correctness. Test doubles (fakes/spies/mocks) for the dependencies a
real service has are the subject of [40 — Testing Architecture](../../40-testing-architecture.md).

---

That's the whole library — every kind of Go test in one place. Track your progress in
[PROGRESS.md](PROGRESS.md); ask for more and I'll append starting at #16.
</content>
