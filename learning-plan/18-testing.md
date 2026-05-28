# 18 — Testing & Benchmarking

## Goals
- Write tests with the standard `testing` package.
- Use table-driven tests and subtests — the dominant Go style.
- Test HTTP handlers with `httptest`.
- Write benchmarks, fuzz tests, and measure coverage.

## Concepts
- **Tests live next to code.** A file `foo.go` is tested by `foo_test.go` in the same package. Test functions are `func TestXxx(t *testing.T)`. Run with `go test ./...`.
  ```go
  func TestAdd(t *testing.T) {
      got := Add(2, 3)
      if got != 5 {
          t.Errorf("Add(2,3) = %d; want 5", got)
      }
  }
  ```
- **`*testing.T` methods:** `t.Errorf`/`t.Error` (fail but continue), `t.Fatalf`/`t.Fatal` (fail and stop this test), `t.Logf` (log when `-v` or on failure), `t.Helper()` (mark a function as a test helper so failures point at the caller), `t.Cleanup(fn)` (deferred cleanup), `t.Parallel()` (run in parallel), `t.Skip()`.
- **No assertion library needed** — idiomatic Go uses plain `if got != want { t.Errorf(...) }`. (Many teams add `testify` for `assert`/`require`; the stdlib style is fine and preferred for learning.)
- **Table-driven tests** — the canonical pattern: a slice of cases + a loop with subtests:
  ```go
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
          t.Run(tt.name, func(t *testing.T) {
              if got := Abs(tt.in); got != tt.want {
                  t.Errorf("Abs(%d) = %d; want %d", tt.in, got, tt.want)
              }
          })
      }
  }
  ```
- **Subtests (`t.Run`)** — give each case a name, run them independently, and let you target one with `go test -run TestAbs/negative`.
- **External test package** — `package foo_test` (note the `_test` suffix) tests only the *exported* API as a real consumer would; same-package tests can reach unexported internals. Use whichever fits.
- **Testing HTTP with `httptest`** — exercise handlers without a real network:
  ```go
  func TestHandler(t *testing.T) {
      req := httptest.NewRequest("GET", "/health", nil)
      rec := httptest.NewRecorder()
      HealthHandler(rec, req)
      if rec.Code != http.StatusOK {
          t.Errorf("status = %d; want 200", rec.Code)
      }
  }
  ```
  `httptest.NewServer` spins up a real local server when you need an actual client round-trip.
- **Benchmarks** — `func BenchmarkXxx(b *testing.B)` with a `for i := 0; i < b.N; i++` loop; run with `go test -bench=. -benchmem`:
  ```go
  func BenchmarkSum(b *testing.B) {
      for i := 0; i < b.N; i++ { Sum(data) }
  }
  ```
- **Fuzzing** — `func FuzzXxx(f *testing.F)` feeds randomized inputs to find edge-case crashes; seed with `f.Add(...)` and run `go test -fuzz=Fuzz`.
- **Coverage** — `go test -cover ./...` for a summary; `go test -coverprofile=c.out && go tool cover -html=c.out` for a line-by-line HTML report.
- **Examples as tests** — `func ExampleAdd()` with an `// Output:` comment doubles as documentation *and* a test that verifies the output.
- **Test fixtures** — put static test data in a `testdata/` directory (the Go tool ignores it during builds).

## Exercises
1. Write `Add`/`Abs` and a table-driven test with named subtests covering positive, negative, and zero cases.
2. Make one case fail on purpose; read the failure output; then run a single subtest with `-run`.
3. Write a test helper that calls `t.Helper()` and confirm failures report the caller's line, not the helper's.
4. Add a `t.Parallel()` to independent subtests and run with `-v` to see them interleave.
5. Test a simple HTTP handler with `httptest.NewRequest` + `httptest.NewRecorder`; assert status and body.
6. Write a `BenchmarkSum` and run `go test -bench=. -benchmem`; read ns/op and allocs/op.
7. Write a `FuzzReverse`-style fuzz test (reverse-of-reverse equals original) and run it briefly with `-fuzz`.
8. Generate an HTML coverage report and find an untested branch.

## Best Practices & Pitfalls
- **Default to table-driven tests with subtests.** They scale: adding a case is one struct literal, and `t.Run` names make failures pinpointable.
- **Test behavior through the public API** (`package foo_test`) when you can; only reach into internals when there's no other way.
- **Write clear failure messages:** include inputs, got, and want (`"f(%v) = %v; want %v"`). A failing test should explain itself without a debugger.
- **Use `t.Fatal` when continuing is pointless** (e.g., setup failed), `t.Error` when you want to report multiple problems in one run.
- **Use `httptest`, not a live server**, for handler tests — fast, deterministic, no port conflicts.
- **Run `go test -race ./...`** for anything concurrent (lesson 15).
- **Pitfall — flaky tests from time/order/randomness:** don't depend on map order, real clocks, or sleeps. Inject time and use deterministic inputs.
- **Pitfall — chasing 100% coverage.** Coverage shows what's *executed*, not what's *correct*. Aim for meaningful tests, not a number.
- **Pitfall — shared state across parallel subtests:** capture loop variables and avoid shared mutable fixtures when using `t.Parallel()`.

## Checklist
- [ ] I can write a `TestXxx` with clear failure messages.
- [ ] I write table-driven tests with `t.Run` subtests by default.
- [ ] I can test an HTTP handler with `httptest`.
- [ ] I can write a benchmark and read `-benchmem` output.
- [ ] I can write a fuzz test and a coverage report.
- [ ] I know when to use `t.Error` vs `t.Fatal` and `t.Helper`.

## Resources
- `testing` package: https://pkg.go.dev/testing
- Tutorial — Add a test: https://go.dev/doc/tutorial/add-a-test
- Blog — Table-driven tests: https://go.dev/wiki/TableDrivenTests
- Blog — Fuzzing: https://go.dev/doc/security/fuzz/
- `net/http/httptest`: https://pkg.go.dev/net/http/httptest
