# Step 61 — CI with GitHub Actions · 🟢 Easy

Examples **1–8**. The commands CI runs, then your first workflow. Runnable examples have an
**Output**/**Verify**; workflow files are complete references (YAML-validated).

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Run the test suite

`🟢 easy` · *go test*

The baseline CI gate is your test suite ([18](../../18-testing.md)/[49](../../49-testing-kinds.md)). `go test ./...` runs every package's tests; a non-zero exit fails the build. `-v` shows each test; plain output is one line per package.

**Steps:**

1. `go test ./...` runs all packages.
2. `ok` = passed; `FAIL` = failed (exit non-zero → CI red).
3. `(cached)` appears when nothing changed since the last run.

```bash
go test ./...
# ok  	ci-demo/mymath	0.4s        <- passed (elapsed time & path vary)
go test -v ./...   # verbose: --- PASS: TestAdd (0.00s) per test
```

**Verify:** run-verified on the `ci-demo` module — `go test ./...` prints `ok  ci-demo/mymath` and exits 0. Elapsed time and package paths vary per run/machine, which is why there's no fixed Output here.

---

## 2. The race detector

`🟢 easy` · *-race*

`go test -race` instruments the binary to detect **data races** — concurrent unsynchronized access to memory. These bugs pass normal tests and only bite under load, so **run `-race` in CI**. It's slower, but it's the gate that catches the hardest class of Go bug.

**Steps:**

1. A test writes a shared variable from many goroutines without a lock.
2. `go test -race` reports `WARNING: DATA RACE` and fails.
3. Guarding the write (mutex/atomic/channel) makes it pass.

```go
package racedemo

import (
	"sync"
	"testing"
)

func TestRace(t *testing.T) {
	var wg sync.WaitGroup
	n := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n++ }() // DATA RACE: unguarded shared write
	}
	wg.Wait()
	_ = n
}
```

```bash
go test -race ./racedemo/
```

**Output:** (addresses/goroutine ids elided — they vary)

```
WARNING: DATA RACE
...
--- FAIL: TestRace (0.00s)
FAIL
FAIL	ci-demo/racedemo	0.3s
FAIL
```

---

## 3. Coverage

`🟢 easy` · *-cover*

`go test -cover` reports the percentage of statements exercised. `-coverprofile` writes a profile you can break down per-function with `go tool cover -func` (or render as HTML). CI can display it, upload it, or **gate** on a threshold (example 24).

**Steps:**

1. `go test -cover ./...` prints a coverage percentage per package.
2. `-coverprofile=cover.out` saves the detail.
3. `go tool cover -func=cover.out` shows per-function coverage + a total.

```bash
go test -cover ./mymath/
# ok  ci-demo/mymath  0.4s  coverage: 100.0% of statements

go test -coverprofile=cover.out ./mymath/
go tool cover -func=cover.out
```

**Output:** (from `go tool cover -func`)

```
ci-demo/mymath/mymath.go:3:	Add		100.0%
ci-demo/mymath/mymath.go:4:	Abs		100.0%
total:				(statements)	100.0%
```

---

## 4. Format and vet gates

`🟢 easy` · *gates*

Two cheap, fast gates every Go CI should have. **`go vet`** catches suspicious constructs (bad `Printf` verbs, unreachable code). **`gofmt -l .`** lists files that aren't gofmt-formatted — in CI you fail if that list is non-empty, so unformatted code can't merge.

**Steps:**

1. `go vet ./...` — non-zero exit on a problem.
2. `gofmt -l .` prints unformatted file paths (empty = all good).
3. `test -z "$(gofmt -l .)"` turns "any unformatted files?" into a pass/fail.

```bash
go vet ./...
test -z "$(gofmt -l .)" || { echo "run gofmt:"; gofmt -l .; exit 1; }
```

**Verify:** run-verified on `ci-demo` — `go vet ./...` exits 0 and `gofmt -l .` prints nothing (all formatted). Introduce a stray indent and `gofmt -l` lists the file, failing the gate.

---

## 5. A first workflow

`🟢 easy` · *workflow*

A GitHub Actions workflow lives at `.github/workflows/ci.yml`. The minimum useful one: on every push and pull request, check out the code, install Go (pinned to your `go.mod`), and run the tests. `setup-go` caches modules automatically.

```yaml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test -race -cover ./...
```

**Verify:** YAML-validated (parses cleanly). Commit it to `.github/workflows/ci.yml` and every push/PR runs `go test -race -cover`. `go-version-file: go.mod` keeps CI on the same Go version as the repo.

---

## 6. Workflow anatomy

`🟢 easy` · *workflow*

Every workflow is the same handful of keys: **`name`** (shown in the UI), **`on`** (what triggers it), and **`jobs`** — each with a runner (`runs-on`) and ordered **`steps`**. A step either **`uses`** a published action or **`run`**s shell. This annotated skeleton is the mental model for everything that follows.

```yaml
name: CI                      # label in the Actions tab
on: [push]                    # trigger(s)

jobs:
  build:                      # job id
    runs-on: ubuntu-latest    # the VM/runner
    steps:
      - name: Check out code
        uses: actions/checkout@v4          # a step that USES an action
      - name: Set up Go
        uses: actions/setup-go@v5
        with: { go-version-file: go.mod }  # inputs to the action
      - name: Build
        run: go build ./...                # a step that RUNs shell
      - name: Test
        run: go test ./...
```

**Verify:** YAML-validated. `name`/`on`/`jobs`→`steps` is the shape of every workflow; `uses` pulls a reusable action, `run` executes a command on the runner.

---

## 7. Triggers

`🟢 easy` · *triggers*

The **`on`** key decides *when* a workflow runs. The common ones: `push` (optionally filtered by branch/path), `pull_request` (validate before merge), `workflow_dispatch` (a manual "Run workflow" button), and `push: tags` (for releases). Filters keep CI from running on irrelevant changes.

```yaml
on:
  push:
    branches: [main]          # only pushes to main
    paths-ignore: ["**.md"]   # skip docs-only changes
  pull_request:               # every PR
  workflow_dispatch:          # manual button in the UI
  push:
    tags: ["v*"]              # version tags -> release workflow
```

**Verify:** YAML-validated. Combine triggers per workflow: a CI workflow on `push`/`pull_request`, a separate release workflow on `tags: ["v*"]` (example 21).

---

## 8. setup-go and caching

`🟢 easy` · *caching*

`actions/setup-go` installs the toolchain **and** caches the Go module + build caches keyed on `go.sum` — on by default (`cache: true`). That turns a cold "download and compile everything" into a warm run that reuses both. Pin the version to `go.mod` so CI and local match.

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod   # single source of truth for the Go version
          cache: true               # module + build cache (default); keyed on go.sum
      - run: go test ./...
```

**Verify:** YAML-validated. With caching on, subsequent runs restore `$GOMODCACHE` and the build cache — often cutting minutes off each run. `cache-dependency-path` targets a non-root `go.sum` if needed.

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
