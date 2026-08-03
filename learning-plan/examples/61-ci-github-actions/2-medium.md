# Step 61 — CI with GitHub Actions · 🟡 Medium

Examples **9–17**. Building a real pipeline: lint, vuln scan, matrices, job graphs, artifacts,
secrets, and concurrency. All workflow files are complete references (YAML-validated).

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

---

## 9. Manual caching with actions/cache

`🟡 medium` · *caching*

`setup-go` caches Go automatically, but `actions/cache` gives you manual control for anything else — a tool binary, a downloaded dataset, or a custom path. The **key** decides cache reuse (change the key → new cache); **`restore-keys`** provides fallbacks for a partial hit.

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Cache golangci-lint analysis
        uses: actions/cache@v4
        with:
          path: ~/.cache/golangci-lint
          key: golangci-${{ runner.os }}-${{ hashFiles('**/go.sum') }}
          restore-keys: golangci-${{ runner.os }}-
      - run: go test ./...
```

**Verify:** YAML-validated. The key includes `hashFiles('**/go.sum')` so the cache invalidates when dependencies change; `restore-keys` still gives a warm-ish start on a miss.

---

## 10. Linting with golangci-lint

`🟡 medium` · *lint*

`golangci-lint` runs dozens of linters in one fast pass; `golangci-lint-action` wraps it for CI with its own caching. Configure the enabled linters in a `.golangci.yml` at the repo root so local and CI agree.

```yaml
# .github/workflows/lint.yml
name: lint
on: [push, pull_request]
jobs:
  golangci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
```

```yaml
# .golangci.yml  (repo root)
linters:
  enable:
    - errcheck      # unchecked errors
    - govet
    - staticcheck   # a large, high-value analyzer set
    - ineffassign
    - unused
```

**Verify:** both YAML files validated. The action installs and runs `golangci-lint` with your `.golangci.yml`; a finding fails the job and annotates the PR diff.

---

## 11. A gofmt gate

`🟡 medium` · *gates*

Linters don't always enforce formatting, so add an explicit `gofmt` gate. The trick: `gofmt -l .` prints files that need formatting; failing when that list is non-empty blocks unformatted code from merging.

```yaml
jobs:
  fmt:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Check formatting
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "These files need gofmt:"; echo "$unformatted"; exit 1
          fi
```

**Verify:** YAML-validated; the shell logic is the run-verified pattern from easy #4. An unformatted file makes `gofmt -l` non-empty → the step exits 1 → the job fails.

---

## 12. Scan dependencies with govulncheck

`🟡 medium` · *security*

`govulncheck` reports dependencies with known CVEs **that your code actually reaches** (call-graph aware, so fewer false positives than a plain SBOM scan). Run it as its own CI job ([57](../../57-web-security.md)).

```yaml
name: vuln
on:
  push:
  schedule:
    - cron: "0 6 * * 1"   # also weekly — new CVEs land against old code
jobs:
  govulncheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

**Verify:** YAML-validated. A reachable vulnerability makes `govulncheck` exit non-zero → red build. The weekly `cron` re-scans unchanged code against newly-published advisories.

---

## 13. A build matrix

`🟡 medium` · *matrix*

A `strategy.matrix` fans one job into many combinations — build/test across the OSes and Go versions you support, in parallel. `fail-fast: false` lets every cell finish so you see *all* failures, not just the first.

```yaml
jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go: ["1.25", "1.26"]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ matrix.go }}" }   # quote expressions inside a flow mapping
      - run: go test ./...
```

**Verify:** YAML-validated. This expands to 3×2 = 6 parallel jobs (`ubuntu/macos/windows` × `1.25/1.26`). Note `go-version` (explicit) here rather than `go-version-file`, so the matrix controls the version.

---

## 14. Job dependencies with needs

`🟡 medium` · *job graph*

`needs` sequences jobs into a graph: a job waits for its dependencies to succeed. Use it to fan in — run `lint` and `test` in parallel, then a `build` job only after **both** pass.

```yaml
jobs:
  lint:
    runs-on: ubuntu-latest
    steps: [{ run: echo linting }]
  test:
    runs-on: ubuntu-latest
    steps: [{ run: echo testing }]
  build:
    needs: [lint, test]        # runs only after BOTH succeed
    runs-on: ubuntu-latest
    steps: [{ run: echo building }]
```

**Verify:** YAML-validated. `lint` and `test` run concurrently; `build` starts only when both are green. If either fails, `build` is skipped.

---

## 15. Build and upload artifacts

`🟡 medium` · *artifacts*

CI can produce downloadable **artifacts** — cross-compiled binaries, coverage reports, SBOMs. Build them, then `actions/upload-artifact` attaches them to the run (and `download-artifact` retrieves them in a later job).

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Cross-compile
        run: |
          GOOS=linux   GOARCH=amd64 go build -o dist/app-linux-amd64 ./cmd/app
          GOOS=darwin  GOARCH=arm64 go build -o dist/app-darwin-arm64 ./cmd/app
      - uses: actions/upload-artifact@v4
        with:
          name: binaries
          path: dist/
```

**Verify:** YAML-validated; the cross-compile commands are the run-verified pattern from [step 60](../60-build-package/) #3. The `dist/` binaries appear under the run's Artifacts.

---

## 16. Secrets, env, and permissions

`🟡 medium` · *security*

CI needs credentials but must handle them carefully. **`GITHUB_TOKEN`** is injected automatically; **`secrets.X`** holds your own; **`env`** sets variables; and **`permissions`** grants least privilege — default to read-only and elevate only where needed (e.g. `packages: write` to push an image).

```yaml
permissions:
  contents: read              # workflow default: least privilege
on: [push]
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write         # only this job may push to GHCR
    env:
      REGISTRY: ghcr.io
    steps:
      - uses: actions/checkout@v4
      - name: Log in to GHCR
        run: echo "${{ secrets.GITHUB_TOKEN }}" | docker login $REGISTRY -u ${{ github.actor }} --password-stdin
```

**Verify:** YAML-validated. Secrets are masked in logs and are **not** exposed to `pull_request` runs from forks. Scope `permissions` per job so a test job can't push packages.

---

## 17. Concurrency and cancel-in-progress

`🟡 medium` · *efficiency*

Push twice quickly and you'd run CI twice — wasteful. A **`concurrency`** group with `cancel-in-progress: true` cancels the older, now-superseded run when a new one starts on the same branch/PR. Group by ref so different branches don't cancel each other.

```yaml
name: CI
on: [push, pull_request]
concurrency:
  group: ci-${{ github.ref }}      # one active run per branch/PR
  cancel-in-progress: true         # new push cancels the in-flight run
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test ./...
```

**Verify:** YAML-validated. Grouping on `github.ref` keeps one active run per branch; a fresh push cancels the previous run's remaining jobs, saving minutes.

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
