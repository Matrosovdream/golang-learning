# 49 — The Go Test Toolbox: Every Kind of Test

> Builds on [18 — Testing & Benchmarking](18-testing.md) (the mechanics) and [40 — Testing Architecture](40-testing-architecture.md) (doubles, the pyramid, determinism). This lesson is the **catalog**: every *kind* of test Go's tooling gives you — unit, table-driven, subtests, parallel, examples, benchmarks, fuzz, golden-file, HTTP, integration, property-based, end-to-end — plus the `testing` machinery (`TestMain`, helpers, `t.Cleanup`, build tags) and the run modes (`-race`, `-cover`, `-short`, `-fuzz`) that tie them together.

## Goals
- Recognise and write **each kind of test** Go supports, and know which kind fits which job.
- Drive the `testing` package's full surface: `T`, `B`, `F`, `M`, subtests, parallelism, helpers, lifecycle.
- Separate tests **by scope** (unit → integration → end-to-end) with build tags and `-short`, and **by style** (table-driven, golden-file, property-based, example).
- Run and measure tests the right way: `-run`, `-race`, `-cover`, `-count`, `-fuzz`, `-json`.

## The map

Three ways to slice "kinds of tests":
1. **By the `testing` entry point** — what function signature you write: `TestXxx(*testing.T)`, `BenchmarkXxx(*testing.B)`, `FuzzXxx(*testing.F)`, `ExampleXxx()`, and the special `TestMain(*testing.M)`.
2. **By scope** — how much of the system the test exercises: **unit** (one function/type), **integration** (real DB/HTTP/queue), **end-to-end** (the whole binary). This is the *test pyramid* ([40](40-testing-architecture.md)): many fast unit tests, fewer integration, a handful of E2E.
3. **By style** — how the test is *shaped*: table-driven, golden-file/snapshot, property-based, example-as-doc, black-box vs white-box.

A single package uses many of these at once. The rest of the lesson is the catalog.

## Concepts

### The four entry points
Everything the `go test` tool runs is one of these, discovered by name in `*_test.go` files:
```go
func TestName(t *testing.T)       { ... }   // a test:      go test
func BenchmarkName(b *testing.B)  { ... }   // a benchmark: go test -bench=.
func FuzzName(f *testing.F)       { ... }   // a fuzz test: go test -fuzz=Fuzz
func ExampleName()                { ... }   // an example:  go test  (checks // Output:)
func TestMain(m *testing.M)       { ... }   // one per package: setup/teardown wrapper
```

### Unit tests & the `*testing.T` toolbox
The base kind — exercise one unit in isolation ([18](18-testing.md)). The `T` you should have in muscle memory:
- **Failure:** `t.Error`/`t.Errorf` (mark failed, keep going), `t.Fatal`/`t.Fatalf` (mark failed, stop *this* test). Use `Fatal` when continuing is pointless (setup failed); `Error` to report several problems in one run.
- **Helpers:** `t.Helper()` marks a function as a helper so a failure points at the *caller's* line, not inside the helper.
- **Lifecycle:** `t.Cleanup(fn)` registers teardown that runs (LIFO) when the test finishes — cleaner than `defer` across helpers. `t.TempDir()` gives a per-test dir auto-removed after. `t.Setenv(k,v)` sets an env var and restores it (and forbids `t.Parallel`). `t.Context()` (Go 1.24+) is a context cancelled at test end.
- **Control:** `t.Skip`/`t.Skipf` (skip, e.g. when a dependency is absent), `t.Parallel()` (below).

### Table-driven tests & subtests
The dominant Go style ([18](18-testing.md)): a slice of cases + a loop of `t.Run(name, …)` subtests. Subtests get their own names (`go test -run TestAbs/negative`), isolate failures, and can each go parallel. Prefer this by default — adding a case is one struct literal.

### Parallel tests
`t.Parallel()` marks a test to run concurrently with other parallel tests: the function pauses at that call, and the runner resumes all siblings together once the sequential ones finish. It surfaces races (run with `-race`) and speeds up I/O-bound suites. Watch the classic trap — inside a table loop, each subtest closure must not share mutable state (in Go ≤1.21 also re-bind the loop variable; 1.22+ fixed the loop-var capture).

### Example tests (documentation that's verified)
`func ExampleFoo()` with a trailing `// Output:` comment is **both** a runnable doc snippet (shown on pkg.go.dev) **and** a test — `go test` runs it and compares stdout to the comment. Use `// Unordered output:` when order isn't guaranteed (e.g. map iteration). Name them `Example`, `ExampleType`, `ExampleType_method` to attach them to symbols in the docs. An example with no `// Output:` is compiled but not run.

### Benchmarks
`func BenchmarkXxx(b *testing.B)` measures speed and (with `b.ReportAllocs()`) allocations ([18](18-testing.md), [46](46-low-latency-measuring.md)). Loop `for i := 0; i < b.N; i++` (or `for b.Loop()` in Go 1.24+, which also stops the compiler optimizing the body away). `b.ResetTimer()`/`b.StopTimer()`/`b.StartTimer()` exclude setup; `b.Run` makes sub-benchmarks; `b.RunParallel` measures under contention. Run `go test -bench=. -benchmem`; compare runs with `benchstat`.

### Fuzz tests
`func FuzzXxx(f *testing.F)` feeds *generated* inputs to find edge cases you didn't think of. Seed with `f.Add(...)`, then `f.Fuzz(func(t *testing.T, in string){ … })` asserts a property that must hold for *all* inputs (e.g. round-trips, no panics). Two modes: plain `go test` runs the **seed corpus** (a fast regression check); `go test -fuzz=FuzzXxx` *generates* new inputs until it finds a failure (which it saves to `testdata/fuzz/` as a permanent regression case).

### Golden-file (snapshot) tests
For big or structured output (rendered templates, formatted reports, serialized trees), assert against a **golden file** in `testdata/` rather than a giant string literal. The pattern: a `-update` flag rewrites the golden files when behaviour legitimately changes, and normal runs compare against them. `testdata/` is ignored by the Go build tool.

### HTTP tests
`net/http/httptest` tests handlers with no real network ([18](18-testing.md)): `httptest.NewRecorder()` + `httptest.NewRequest()` drive a handler directly and inspect the `ResponseRecorder`; `httptest.NewServer(handler)` spins up a real local server (a real `Client` round-trip) for integration-style tests.

### Scope: unit vs integration vs end-to-end
The *same* `TestXxx` signature; what differs is what it touches and how you gate it:
- **`testing.Short()` + `-short`** — guard slow tests with `if testing.Short() { t.Skip("skipping in -short") }`, so `go test -short` runs only the fast ones (CI smoke) and the full `go test` runs everything.
- **Build tags** — put integration tests behind `//go:build integration` so they compile only with `go test -tags=integration`. Keeps a real-DB test out of the default unit run.
- **Real infra** — spin up dependencies with `testcontainers-go` (a throwaway Postgres/Redis in Docker) so integration tests are hermetic and reproducible ([40](40-testing-architecture.md)).
- **End-to-end** — build the binary and drive it from outside (its HTTP API, its CLI). Fewest of these; they're slow and broad but catch wiring bugs unit tests can't.

### Property-based tests
Instead of specific cases, assert an **invariant** over many random inputs. `testing/quick.Check` generates values and fails if your property returns false (e.g. `reverse(reverse(x)) == x`, `Decode(Encode(x)) == x`). Fuzzing is the modern, coverage-guided cousin; `quick` is the zero-dependency stdlib version for pure invariants.

### Black-box vs white-box
- **White-box:** `package foo` — the test sees unexported internals. Good for testing tricky internal logic directly.
- **Black-box:** `package foo_test` (same directory, `_test` suffix) — the test imports `foo` and sees only its **exported** API, exactly as a real consumer does. Preferred when you can: it keeps tests honest and refactor-proof. Need one internal hook from a black-box test? Expose it from an `export_test.go` (in `package foo`), which the tool compiles only under test.

### Test doubles (in a test)
Replace real dependencies with **fakes** (working in-memory impl), **stubs** (canned answers), **mocks** (assert on interactions), or **spies** (record calls) — the full taxonomy and the "prefer fakes over mocks" guidance live in [40](40-testing-architecture.md). Here, just know each is something you wire into a `_test.go` via an interface the code under test accepts.

### Fixtures, assertions, and running
- **Fixtures:** static inputs go in `testdata/` (build-ignored). Generated helpers go in `_test.go`.
- **Assertions:** idiomatic Go uses plain `if got != want { t.Errorf(...) }`. Teams often add `testify` (`assert`/`require`) or `google/go-cmp` (`cmp.Diff` for structs/slices — far better failure output than `reflect.DeepEqual`). Stdlib style is preferred for learning.
- **Run modes:** `-run`/`-bench`/`-fuzz` (select), `-v` (verbose), `-race` (data-race detector — run for anything concurrent), `-cover`/`-coverprofile` + `go tool cover` (coverage), `-count=1` (defeat the test cache), `-short`, `-timeout`, `-json` (machine-readable for CI).

## Exercises
1. Write a table-driven `TestXxx` with named `t.Run` subtests; make one fail, read the output, then run just that subtest with `-run Test/case`.
2. Write a helper using `t.Helper()` and confirm failures report the caller's line. Add `t.Cleanup` and `t.TempDir` and observe the cleanup order.
3. Write an `ExampleXxx` with `// Output:` and one with `// Unordered output:` for map iteration; break the output and watch `go test` fail.
4. Write a `BenchmarkXxx` with `b.ReportAllocs()` and a sub-benchmark via `b.Run`; run `-bench=. -benchmem`.
5. Write a `FuzzXxx` for a round-trip property (e.g. `parse(format(x)) == x`); run the seed corpus with `go test`, then `-fuzz` briefly and inspect any saved failing input.
6. Write a golden-file test with a `-update` flag: generate `testdata/*.golden`, then assert against it; change the output and re-`-update`.
7. Test an HTTP handler two ways: `httptest.NewRecorder` (direct) and `httptest.NewServer` (real round-trip).
8. Gate a slow test with `testing.Short()`; run `-short` vs full. Then move a "needs a database" test behind `//go:build integration` and run it only with `-tags=integration`.
9. Write a property-based test with `testing/quick.Check`.
10. Add a `TestMain` that does package-level setup/teardown around `m.Run()`. Generate a coverage report and run the suite under `-race`.

## Best Practices & Pitfalls
- **Default to table-driven unit tests with subtests**; reach for the other kinds when the job calls for them (fuzz for parsers, golden for rendered output, integration for real infra).
- **Match the kind to the scope.** Most tests are unit (fast, many); gate integration/E2E with `-short` and build tags so the default run stays fast.
- **Prefer black-box (`package foo_test`)** to test behaviour through the public API; drop to white-box only for genuinely internal logic.
- **Use `t.Cleanup`/`t.TempDir`/`t.Setenv`** instead of hand-rolled teardown; they compose across helpers and can't leak.
- **Fuzz your parsers and encoders**, and commit the failing inputs it finds — they become permanent regression tests.
- **Run `-race` on anything concurrent** and `-count=1` when you suspect the test cache is hiding a change.
- **Pitfall — flaky tests** from real clocks, sleeps, map order, or network. Inject time, use deterministic inputs, and `httptest` over live servers ([40](40-testing-architecture.md)).
- **Pitfall — shared state in parallel subtests.** `t.Parallel` siblings run together; don't share mutable fixtures (and pre-1.22, re-bind the loop variable).
- **Pitfall — chasing 100% coverage.** Coverage shows what ran, not what's correct. Write meaningful cases, not a number.
- **Pitfall — an `Example` with a wrong `// Output:`** silently becomes a failing test; keep them honest or drop the `// Output:` line to make it compile-only.
- **Pitfall — putting integration tests in the default run.** A test that needs Docker/DB in the unit path makes `go test ./...` flaky and slow for everyone.

## Checklist
- [ ] I can name the four entry points (`T`, `B`, `F`, `Example`) plus `TestMain`, and say what each is for.
- [ ] I write table-driven tests with subtests and know when to go `t.Parallel`.
- [ ] I use `t.Helper`, `t.Cleanup`, `t.TempDir`, `t.Setenv` correctly.
- [ ] I can write an example test, a benchmark, and a fuzz test (seed corpus + `-fuzz`).
- [ ] I can write a golden-file test with an `-update` flag and an HTTP test with `httptest`.
- [ ] I separate unit/integration/E2E with `testing.Short()` and `//go:build` tags.
- [ ] I can write a property-based test and reach for `go-cmp` on structured comparisons.
- [ ] I run `-race`, `-cover`, and `-count=1` when they matter, and know black-box vs white-box.

## Resources
- `testing` package (T, B, F, M, all methods): https://pkg.go.dev/testing
- Go blog — "Using Subtests and Sub-benchmarks": https://go.dev/blog/subtests
- Go — Fuzzing tutorial & docs: https://go.dev/doc/security/fuzz/
- `net/http/httptest`: https://pkg.go.dev/net/http/httptest · `testing/quick`: https://pkg.go.dev/testing/quick
- `google/go-cmp` (better diffs than `reflect.DeepEqual`): https://pkg.go.dev/github.com/google/go-cmp/cmp
- Build constraints (`//go:build`): https://pkg.go.dev/cmd/go#hdr-Build_constraints · `testcontainers-go`: https://golang.testcontainers.org/
- Examples: [examples/49-testing-kinds](examples/49-testing-kinds/).
- Prior art in this plan: mechanics in [18 — Testing & Benchmarking](18-testing.md); doubles, the pyramid & determinism in [40 — Testing Architecture](40-testing-architecture.md); benchmarking for latency in [46 — Low-Latency I](46-low-latency-measuring.md).
</content>
