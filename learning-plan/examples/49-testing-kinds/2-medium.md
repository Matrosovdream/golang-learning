# 49 · Medium (6–10) — parallel, bench, fuzz, http, golden

Back to [index](README.md) · Prev tier: [Easy](1-easy.md) · Next tier: [Hard](3-hard.md)

---

## 6. Parallel subtests

`t.Parallel()` marks a test to run concurrently with its parallel siblings: the function *pauses* at that
call, and the runner resumes them together. It speeds up I/O-bound suites and, under `-race`, surfaces
data races.

`u06/work.go`
```go
package slow

import "time"

// Work simulates independent I/O-bound work (a network/DB call).
func Work(d time.Duration) time.Duration {
	time.Sleep(d)
	return d
}
```

`u06/work_test.go`
```go
package slow

import (
	"testing"
	"time"
)

func TestWork(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
	}{
		{"a", 20 * time.Millisecond},
		{"b", 20 * time.Millisecond},
		{"c", 20 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // pauses here, then runs concurrently with its siblings
			if got := Work(tc.d); got != tc.d {
				t.Errorf("Work(%v) = %v", tc.d, got)
			}
		})
	}
}
```

Run: `go test -v ./u06`

**Output** *(the `PAUSE`/`CONT` interleaving order varies run to run)*
```
=== RUN   TestWork
=== RUN   TestWork/a
=== PAUSE TestWork/a
=== RUN   TestWork/b
=== PAUSE TestWork/b
=== RUN   TestWork/c
=== PAUSE TestWork/c
=== CONT  TestWork/a
=== CONT  TestWork/c
=== CONT  TestWork/b
--- PASS: TestWork (0.00s)
    --- PASS: TestWork/b (0.02s)
    --- PASS: TestWork/c (0.02s)
    --- PASS: TestWork/a (0.02s)
PASS
```

The three subtests `PAUSE`, then `CONT` together and finish in ~20ms total instead of 60ms. **The trap:**
parallel siblings run at the same time, so they must not share mutable state (and in Go ≤1.21 you also had
to re-bind the loop variable — 1.22+ fixed that). Run concurrent tests with `-race`.

---

## 7. Benchmarks & sub-benchmarks

`func BenchmarkXxx(b *testing.B)` loops `b.N` times; `b.ReportAllocs()` adds `B/op` and `allocs/op`; `b.Run`
makes sub-benchmarks so you can compare variants side by side.

`u07/join.go`
```go
package bench

import "strings"

func JoinLoop(parts []string) string {
	var s string
	for _, p := range parts {
		s += p
	}
	return s
}

func JoinBuilder(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p)
	}
	return b.String()
}
```

`u07/join_test.go`
```go
package bench

import (
	"strings"
	"testing"
)

var parts = strings.Split(strings.Repeat("go,", 500), ",")
var sink string

// b.Run makes sub-benchmarks; b.ReportAllocs adds B/op and allocs/op.
func BenchmarkJoin(b *testing.B) {
	b.Run("loop", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink = JoinLoop(parts)
		}
	})
	b.Run("builder", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink = JoinBuilder(parts)
		}
	})
}
```

Run: `go test -bench=. -benchmem ./u07`

**Output** *(illustrative — `ns/op` is machine-dependent; the ratio and allocs are the point)*
```
BenchmarkJoin/loop-10         	   47889	     24133 ns/op	  265139 B/op	     499 allocs/op
BenchmarkJoin/builder-10      	  706200	      1697 ns/op	    3320 B/op	       9 allocs/op
```

Benchmarks don't run under a plain `go test` — you opt in with `-bench`. The `sink` package var stops the
compiler deleting the result as dead code. In Go 1.24+, `for b.Loop()` replaces the `for i := 0; i < b.N`
loop and also prevents that dead-code elimination. Compare two runs rigorously with `benchstat`
(see [46](../../46-low-latency-measuring.md)).

---

## 8. Fuzz tests

`func FuzzXxx(f *testing.F)` feeds *generated* inputs to a property that must hold for all of them. Seed
with `f.Add`; assert inside `f.Fuzz`. Plain `go test` runs just the **seed corpus** (a fast regression
check); `go test -fuzz=Fuzz` generates new inputs until something breaks.

`u08/reverse.go`
```go
package fuzzy

// Reverse reverses a UTF-8 string by runes.
func Reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
```

`u08/reverse_test.go`
```go
package fuzzy

import (
	"testing"
	"unicode/utf8"
)

func FuzzReverse(f *testing.F) {
	for _, seed := range []string{"", "a", "abc", "Hello, 世界"} {
		f.Add(seed) // seed corpus: plain `go test` runs exactly these
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			t.Skip() // only assert the property on valid UTF-8
		}
		rev := Reverse(s)
		// Property 1: reversing twice returns the original.
		if got := Reverse(rev); got != s {
			t.Errorf("Reverse(Reverse(%q)) = %q; want %q", s, got, s)
		}
		// Property 2: reversing preserves the rune count.
		if a, b := utf8.RuneCountInString(s), utf8.RuneCountInString(rev); a != b {
			t.Errorf("rune count changed: %d != %d", a, b)
		}
	})
}
```

Run the seed corpus (part of the normal suite): `go test -v ./u08`

**Output**
```
=== RUN   FuzzReverse
=== RUN   FuzzReverse/seed#0
=== RUN   FuzzReverse/seed#1
=== RUN   FuzzReverse/seed#2
=== RUN   FuzzReverse/seed#3
--- PASS: FuzzReverse (0.00s)
    ... (one PASS per seed)
PASS
```

Now actually *fuzz* it: `go test -fuzz=FuzzReverse -fuzztime=3s ./u08`

**Output** *(illustrative)*
```
fuzz: elapsed: 0s, gathering baseline coverage: 4/4 completed, now fuzzing with 10 workers
fuzz: elapsed: 3s, execs: 250808 (83597/sec), new interesting: 38 (total: 42)
PASS
```

A quarter-million inputs, no crash — because this `Reverse` is rune-correct. (Fuzz the classic *byte*-based
reverse and it finds an invalid-UTF-8 input in milliseconds — and saves it to `testdata/fuzz/` as a
permanent regression test.) Fuzz your parsers, decoders, and anything that takes untrusted bytes.

---

## 9. HTTP tests with `httptest`

`net/http/httptest` tests handlers with no real network. `NewRecorder` drives a handler directly;
`NewServer` spins up a real local server for a genuine client round-trip.

`u09/web.go`
```go
package web

import (
	"encoding/json"
	"net/http"
)

func GreetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"greeting": "Hello, " + name})
}
```

`u09/web_test.go`
```go
package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Direct: drive the handler with a recorder — no network, no port.
func TestGreetHandler_recorder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/greet?name=Go", nil)
	rec := httptest.NewRecorder()

	GreetHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"greeting":"Hello, Go"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestGreetHandler_missingName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/greet", nil)
	rec := httptest.NewRecorder()
	GreetHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
}

// Real round-trip: a local server exercised by a real http.Client.
func TestGreetHandler_server(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(GreetHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/greet?name=World")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello, World") {
		t.Errorf("body = %s", body)
	}
}
```

Run: `go test -v ./u09`

**Output**
```
=== RUN   TestGreetHandler_recorder
--- PASS: TestGreetHandler_recorder (0.00s)
=== RUN   TestGreetHandler_missingName
--- PASS: TestGreetHandler_missingName (0.00s)
=== RUN   TestGreetHandler_server
--- PASS: TestGreetHandler_server (0.00s)
PASS
```

Prefer `NewRecorder` for fast, focused handler tests; reach for `NewServer` when you need a real
client/transport (redirects, timeouts, TLS). Both avoid port conflicts and flakiness.

---

## 10. Golden-file tests

For large or structured output, assert against a **golden file** in `testdata/` instead of a giant string
literal. A `-update` flag regenerates the golden files when the output legitimately changes.

`u10/report.go`
```go
package report

import (
	"fmt"
	"sort"
	"strings"
)

func Render(title string, items map[string]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s: %d\n", k, items[k])
	}
	return b.String()
}
```

`u10/report_test.go`
```go
package report

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// A -update flag rewrites the golden files when behaviour legitimately changes.
var update = flag.Bool("update", false, "update golden files")

func TestRender(t *testing.T) {
	got := Render("Sales", map[string]int{"apples": 3, "pears": 7})

	golden := filepath.Join("testdata", "report.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", golden)
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run once with -update first): %v", err)
	}
	if got != string(want) {
		t.Errorf("output mismatch:\n got: %q\nwant: %q", got, string(want))
	}
}
```

First generate the golden file (custom flags go **after** the package path), then run normally:
```bash
go test ./u10 -run TestRender -update   # writes testdata/report.golden
go test -v ./u10                         # compares against it
```

**Output** (second command)
```
=== RUN   TestRender
--- PASS: TestRender (0.00s)
PASS
```

`testdata/report.golden` now holds:
```
# Sales

- apples: 3
- pears: 7
```

Commit the golden files. When output changes on purpose, re-run with `-update` and **review the diff** in
code review — that diff is exactly what your change did to the output. (Snapshot/approval testing is the
same idea with library sugar.)

---

Next tier: [🔴 Hard (11–15)](3-hard.md) — scope, race detection, and a multi-kind capstone.
</content>
