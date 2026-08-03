# Step 61 — CI with GitHub Actions · 🔴 Hard

Examples **18–26**. Machine-readable test output, integration tests, image publishing, releases,
reusable workflows, and a full pipeline capstone. Config files are complete references (YAML-validated).

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

---

## 18. Test output as JSON

`🔴 hard` · *go test*

`go test -json` emits one JSON **event** per line — `Action: "start"/"run"/"output"/"pass"/"fail"`, tagged with the package and test. It's what test-summary actions, annotations, and dashboards consume. You can also summarize it in a script (count passes/failures) for a CI gate.

**Steps:**

1. `go test -json ./...` streams events instead of human text.
2. Each event has an `Action` field; filter/aggregate on it.
3. Here: the histogram of actions for a two-test package.

```bash
go test -json ./mymath/                              # raw event stream
go test -json ./mymath/ | grep -o '"Action":"[a-z]*"' | sort | uniq -c
```

**Output:** (the action histogram; a single event looks like `{"Action":"pass","Package":"ci-demo/mymath","Test":"TestAdd",...}` with a volatile `Time`)

```
   6 "Action":"output"
   3 "Action":"pass"
   2 "Action":"run"
   1 "Action":"start"
```

> 3 `pass` events = `TestAdd`, `TestAbs`, and the package. In a workflow, pipe `-json` to a summary action (e.g. `test-summary/action`) to annotate the run.

---

## 19. Integration tests with a service container

`🔴 hard` · *integration*

Integration tests ([40](../../40-testing-architecture.md)/[49](../../49-testing-kinds.md)) need real infrastructure. GitHub **service containers** run a dependency (Postgres, Redis…) alongside the job. A **health option** makes the runner wait until it's ready before your tests connect — otherwise the first test races the database's startup.

```yaml
jobs:
  integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_USER: app
          POSTGRES_PASSWORD: secret
          POSTGRES_DB: app_test
        ports: ["5432:5432"]
        options: >-
          --health-cmd "pg_isready -U app"
          --health-interval 5s --health-timeout 3s --health-retries 5
    env:
      DATABASE_URL: postgres://app:secret@localhost:5432/app_test?sslmode=disable
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test -tags=integration ./...
```

**Verify:** YAML-validated. The `--health-cmd` gate makes the job wait for `pg_isready` before running tests; the build-tag (`-tags=integration`, [step 49](../../49-testing-kinds.md)) keeps these tests out of the fast unit run.

---

## 20. Build and push a Docker image

`🔴 hard` · *docker*

CI builds the image ([60](../60-build-package/)) and pushes it to a registry. **GHCR** (`ghcr.io`) needs no extra secret — the built-in `GITHUB_TOKEN` authenticates with `packages: write`. `docker/build-push-action` handles buildx + push.

```yaml
name: image
on:
  push:
    branches: [main]
permissions:
  contents: read
  packages: write
jobs:
  build-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
          build-args: VERSION=${{ github.sha }}
```

**Verify:** YAML-validated. On a push to `main` it logs in to GHCR with `GITHUB_TOKEN`, builds the Dockerfile from [step 60](../60-build-package/), and pushes an image tagged with the commit SHA (also stamped into the binary via `build-args`).

---

## 21. Release with GoReleaser

`🔴 hard` · *release*

A version **tag** should produce a **release**: cross-compiled binaries, archives, checksums, and a changelog. **GoReleaser** does all of it from one config, driven by a workflow that triggers on tags.

```yaml
# .goreleaser.yaml
version: 2
builds:
  - env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags: ["-s -w -X main.version={{.Version}}"]
archives:
  - formats: [tar.gz]
changelog:
  use: github
```

```yaml
# .github/workflows/release.yml
name: release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write            # create the GitHub Release
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }   # full history for the changelog
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: goreleaser/goreleaser-action@v6
        with: { args: release --clean }
        env: { GITHUB_TOKEN: "${{ secrets.GITHUB_TOKEN }}" }
```

**Verify:** both YAML files validated. `git tag v1.4.2 && git push --tags` triggers the workflow; GoReleaser builds every `goos`×`goarch`, stamps the version via `ldflags`, and publishes a Release with archives + checksums + changelog. `fetch-depth: 0` is required for the changelog.

---

## 22. Cache Docker build layers

`🔴 hard` · *docker*

Rebuilding an image in CI is slow if every layer is cold. `docker/build-push-action` can cache layers to the **GitHub Actions cache** (`type=gha`), so unchanged layers (like the `go mod download` layer from [step 60](../60-build-package/) #18) are restored instead of rebuilt.

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: false
          tags: app:ci
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Verify:** YAML-validated. `cache-from`/`cache-to: type=gha` persists buildx layers across runs; combined with the layer ordering from [step 60](../60-build-package/), a code-only change reuses the dependency layer and rebuilds only the final stages.

---

## 23. Reusable workflows and composite actions

`🔴 hard` · *DRY*

Copy-pasting the same CI across repos rots fast. A **reusable workflow** (`on: workflow_call`) is a whole pipeline another workflow invokes with `uses:`. A **composite action** bundles repeated *steps* (checkout + setup-go + cache) into one `uses:`. Both centralize the definition.

```yaml
# .github/workflows/reusable-test.yml  (the callee)
on:
  workflow_call:
    inputs:
      go-version: { type: string, default: "1.26" }
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ inputs.go-version }}" }
      - run: go test -race ./...
```

```yaml
# .github/workflows/ci.yml  (the caller)
name: CI
on: [push, pull_request]
jobs:
  call-test:
    uses: ./.github/workflows/reusable-test.yml
    with: { go-version: "1.26" }
```

**Verify:** both YAML files validated. The caller invokes the reusable workflow with `uses: ./…` and passes `inputs`; many repos (or many workflows in one repo) share the single definition.

---

## 24. A coverage gate

`🔴 hard` · *quality gate*

Turn coverage from a number into a **gate**: compute the total, compare against a threshold, and fail the build if it drops below. (Alternatively upload the profile to Codecov and let it comment on the PR.)

```yaml
jobs:
  coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: Enforce coverage >= 80%
        run: |
          go test -coverprofile=cover.out ./...
          total=$(go tool cover -func=cover.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')
          echo "total coverage: ${total}%"
          awk "BEGIN { exit !($total >= 80) }" || { echo "coverage below 80%"; exit 1; }
      # - uses: codecov/codecov-action@v4   # alternative: upload instead of gate
      #   with: { files: cover.out }
```

**Verify:** YAML-validated; the `go tool cover -func` extraction is the run-verified command from easy #3 (which reported `total: 100.0%`). The `awk` compares the total against the 80% threshold and exits 1 if it's lower.

---

## 25. Automated dependency updates

`🔴 hard` · *maintenance*

**Dependabot** opens PRs to bump your Go modules and — just as important — your **GitHub Actions versions** (unpinned actions are a supply-chain risk). It lives at `.github/dependabot.yml`, not in `workflows/`.

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: gomod
    directory: "/"
    schedule: { interval: weekly }
    groups:
      go-deps: { patterns: ["*"] }   # one grouped PR instead of many
  - package-ecosystem: github-actions
    directory: "/"
    schedule: { interval: weekly }
```

**Verify:** YAML-validated. Dependabot raises weekly PRs for `gomod` and `github-actions`; grouping collapses many bumps into one PR your CI can validate together.

---

## 26. Capstone: a complete CI pipeline

`🔴 hard` · *capstone*

Everything assembled into one `ci.yml`: least-privilege permissions, cancel-in-progress concurrency, parallel **lint** + **vuln** + **test(matrix, race, cover)** jobs, and a **build-and-push** job gated on `needs` that only runs on `main`.

```yaml
name: CI
on: [push, pull_request]
permissions:
  contents: read
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest }

  vuln:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...

  test:
    strategy:
      fail-fast: false
      matrix: { go: ["1.25", "1.26"] }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ matrix.go }}" }
      - run: go test -race -coverprofile=cover.out ./...

  image:
    needs: [lint, vuln, test]
    if: github.ref == 'refs/heads/main'   # publish only from main
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Verify:** YAML-validated. `lint`, `vuln`, and the `test` matrix run in parallel on every push/PR; `image` waits for all three (`needs`) and — guarded by `if: github.ref == 'refs/heads/main'` — builds and pushes to GHCR only from `main`, with buildx layer caching. This is the pipeline that feeds [step 62](../62-deployment-operations.md).

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
