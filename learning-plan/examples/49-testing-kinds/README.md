# Step 49 — The Go Test Toolbox: Every Kind of Test · Examples

A library of **15 runnable examples**, split into three files by difficulty. Unlike the other libraries in
this plan, **these are real `*_test.go` files you run with `go test`** (not `go run .` demos) — because the
whole point is the test machinery itself. They reinforce [49-testing-kinds.md](../../49-testing-kinds.md):
unit, table-driven, subtests, examples, helpers, parallel, benchmarks, fuzz, `httptest`, golden files,
`TestMain`, build tags, property-based, race detection, and a multi-kind capstone.

## One-time setup

```bash
mkdir -p /tmp/gotest-ex && cd /tmp/gotest-ex
go mod init scratch
```

Each example is its own package — **create a subdirectory per example** (`u01`, `u02`, …) and put its
files there, then run `go test` on that directory:

```bash
go test -v ./u01
```

> **Two examples import their own package** (#3 black-box, #4 example package): the import path is
> `scratch/<dir>`, so name those directories exactly `u03` and `u04` (or edit the import to match).

Every example was written, `gofmt`/`go vet` clean, and **run with `go test`** before being added; the
output shown is real. The trailing `ok scratch/uNN 0.2s` **wall-clock times vary by machine** — ignore
them; the `PASS`/`FAIL`/`SKIP` lines are the point. Benchmarks (#7) print machine-dependent `ns/op`
(marked *illustrative*). Standard-library only.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — the core kinds
- [1. A unit test (`Error` vs `Fatal`)](1-easy.md#1-a-unit-test-error-vs-fatal)
- [2. Table-driven tests & subtests](1-easy.md#2-table-driven-tests--subtests)
- [3. Black-box vs white-box](1-easy.md#3-black-box-vs-white-box)
- [4. Example tests (verified docs)](1-easy.md#4-example-tests-verified-docs)
- [5. Helpers & lifecycle](1-easy.md#5-helpers--lifecycle)

### 🟡 [Medium](2-medium.md) — parallel, bench, fuzz, http, golden
- [6. Parallel subtests](2-medium.md#6-parallel-subtests)
- [7. Benchmarks & sub-benchmarks](2-medium.md#7-benchmarks--sub-benchmarks)
- [8. Fuzz tests](2-medium.md#8-fuzz-tests)
- [9. HTTP tests with `httptest`](2-medium.md#9-http-tests-with-httptest)
- [10. Golden-file tests](2-medium.md#10-golden-file-tests)

### 🔴 [Hard](3-hard.md) — scope, doubles, tooling & capstone
- [11. `TestMain` and short mode](3-hard.md#11-testmain-and-short-mode)
- [12. Integration tests behind a build tag](3-hard.md#12-integration-tests-behind-a-build-tag)
- [13. Property-based tests (`testing/quick`)](3-hard.md#13-property-based-tests-testingquick)
- [14. Race detection (`-race`)](3-hard.md#14-race-detection--race)
- [15. Capstone: one package, five kinds of test](3-hard.md#15-capstone-one-package-five-kinds-of-test)
</content>
