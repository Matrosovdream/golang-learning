# 61 — Continuous Integration with GitHub Actions

> Part of **Part 13 — CI/CD & Deployment**: [60 — Build & Package](60-build-package.md) → **61 CI** → [62 — Deployment & Operations](62-deployment-operations.md). Builds on [18 — Testing](18-testing.md) & [49 — The Go Test Toolbox](49-testing-kinds.md) (what CI runs), [24 — Idiomatic Go](24-idiomatic-go.md) (vet/lint), [57 — Web Security](57-web-security.md) (`govulncheck`), and [60](60-build-package.md) (the build/image it produces). Thesis: **CI is your test suite plus the quality gates run automatically on every push — `go test -race -cover`, `vet`, a linter, and `govulncheck`, cached for speed and fanned out over a matrix. Get those green on every PR and you can ship continuously; add a release job and a tag becomes a published artifact.**

## Goals
- Run the **CI command set** — `go test ./...`, **`-race`**, **`-cover`**, `go vet`, `gofmt`/lint, `govulncheck` — and understand what each gate catches.
- Write a **GitHub Actions workflow**: triggers (`on`), jobs, steps, `actions/checkout` + **`actions/setup-go`**, and **caching** for fast runs.
- Scale the pipeline: a **build/test matrix** (OS × Go version), **job dependencies** (`needs`), **artifacts**, **secrets/permissions**, and **concurrency** (cancel superseded runs).
- Add the production gates: **integration tests** with a **service container** (Postgres), **build & push a Docker image** (to GHCR), a **release** via **GoReleaser**, **Docker layer caching**, **reusable workflows**, a **coverage gate**, and **automated dependency updates** (Dependabot).

## Concepts

- **CI = your quality gates, automated.** The core set for a Go repo:
  - **`go test ./...`** — the suite ([18](18-testing.md)/[49](49-testing-kinds.md)); the baseline gate.
  - **`go test -race ./...`** — the **race detector** catches concurrency bugs that pass without it. Run it in CI even though it's slower.
  - **`go test -coverprofile=cover.out ./...`** + `go tool cover` — measure coverage; optionally **gate** on a threshold or upload to Codecov.
  - **`go vet ./...`** and **`gofmt -l .`** (fails if any file needs formatting) — cheap correctness/style gates.
  - **`govulncheck ./...`** — flags dependencies with known CVEs that your code actually reaches ([57](57-web-security.md)).
- **A workflow is YAML in `.github/workflows/`.** `name`, **`on`** (the triggers: `push`, `pull_request`, `workflow_dispatch`, `tags`), and **`jobs`** — each with `runs-on` and ordered **`steps`**. A step either **`uses`** a published action (`actions/checkout@v4`) or **`run`**s shell. The canonical first two steps are `actions/checkout` (get the code) and **`actions/setup-go`** (install the toolchain, pinned to your `go.mod`).
- **Cache to keep runs fast.** `actions/setup-go` caches the **module** and **build** caches keyed on `go.sum` automatically (`cache: true`, the default). Without caching, every run re-downloads and re-compiles the world. `actions/cache` gives you manual control for other paths (Docker layers, tool binaries).
- **Matrix builds test the combinations that matter.** A `strategy.matrix` over `os` (`ubuntu`/`macos`/`windows`) and `go-version` fans one job into N — verifying your code on every platform and Go version you support, in parallel.
- **Compose jobs with `needs`, gate with `permissions`.** `needs: [lint, test]` makes a job wait for others (fan-in before a deploy). **`GITHUB_TOKEN`** is injected automatically; **`secrets.X`** holds credentials; set least-privilege **`permissions`** per workflow/job (e.g. `contents: read`, `packages: write` only where needed). **`concurrency`** with `cancel-in-progress` kills an in-flight run when you push again — no wasted minutes.
- **Real pipelines do more than test.** **Integration tests** get a real dependency via a **service container** (`services: postgres:` — GitHub runs it alongside the job). CI **builds and pushes the image** ([60](60-build-package.md)) to **GHCR** with `docker/build-push-action` + `docker/login-action`, cached with **buildx**. A **tag** triggers a **release**: **GoReleaser** cross-compiles, packages, and publishes binaries + a changelog in one step. **Reusable workflows** / **composite actions** keep it DRY across repos, and **Dependabot** opens PRs to bump modules and actions.
- **`go test -json`** emits a machine-readable event stream (`Action: "run"/"pass"/"fail"`, per test and package) — what test summaries, annotations, and dashboards consume.

## Exercises
1. Run the CI command set locally: `go test ./...`, `-race`, `-coverprofile` + `go tool cover -func`, `go vet`, `gofmt -l .`.
2. Write a first `.github/workflows/ci.yml` — checkout + setup-go + `go test ./...` on push and PR.
3. Add caching (via setup-go), a `gofmt`/`vet` gate, a **golangci-lint** job, and a **govulncheck** job.
4. Add a **matrix** over OS and Go version; wire a `deploy`/`build` job with `needs: [lint, test]`.
5. Upload a build **artifact**; add **concurrency** with `cancel-in-progress`; set least-privilege **permissions**.
6. Add **integration tests** against a Postgres **service container**.
7. Build and **push a Docker image** to GHCR (buildx + login + build-push-action), with layer caching.
8. Add a **GoReleaser** config + a release workflow triggered on a version tag.
9. Add a **coverage gate** (or Codecov upload) and a **Dependabot** config.
10. Capstone: assemble a complete pipeline — lint + test(+race+cover) + vuln + matrix build, image push on `main`, release on tag.

## Best Practices & Pitfalls
- **Run `-race` in CI.** Locally it's easy to skip; CI is where the race detector earns its keep. Slower, but it catches the bugs that only appear under load.
- **Cache modules and the build cache.** It's on by default with `setup-go`; leaving it off makes every run minutes slower.
- **Fail the build on `gofmt -l`.** `gofmt -l .` lists unformatted files; a non-empty list should fail the job (`test -z "$(gofmt -l .)"`).
- **Set least-privilege `permissions`.** Default the workflow to `contents: read` and grant `packages: write`/`contents: write` only to the jobs that push images or releases.
- **Pin actions and set `concurrency`.** Pin `uses:` to a version (or SHA) for reproducibility; `cancel-in-progress` avoids piling up runs on rapid pushes.
- **Pitfall — flaky integration tests.** A service container needs a **health/readiness wait** before tests connect (poll `pg_isready`), or the first test races the database's startup.
- **Pitfall — secrets in logs / forks.** `secrets` aren't available to `pull_request` runs from forks (by design). Don't `echo` secrets; use them only in the steps that need them.
- **Keep CI and local in sync.** Have CI call the same **Makefile** targets ([60](60-build-package.md)) you run locally, so "works on my machine" and "passes CI" mean the same thing.

## Checklist
- [ ] I run `go test -race -cover`, `vet`, `gofmt`/lint, and `govulncheck` as CI gates.
- [ ] I can write a workflow with triggers, checkout, setup-go, and caching.
- [ ] I use a matrix (OS × Go version), `needs` for job order, and `concurrency` with cancel-in-progress.
- [ ] I set least-privilege `permissions` and use `secrets`/`GITHUB_TOKEN` safely.
- [ ] I can run integration tests against a service container and wait for it to be healthy.
- [ ] I build & push an image to GHCR and cut a release with GoReleaser on a tag.
- [ ] I have a coverage gate and Dependabot keeping deps current.

## Resources
- GitHub Actions docs: https://docs.github.com/actions · workflow syntax: https://docs.github.com/actions/using-workflows/workflow-syntax-for-github-actions
- `actions/setup-go`: https://github.com/actions/setup-go · `actions/checkout`: https://github.com/actions/checkout · `actions/cache`: https://github.com/actions/cache
- `golangci-lint-action`: https://github.com/golangci/golangci-lint-action · `govulncheck`: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
- `docker/build-push-action`: https://github.com/docker/build-push-action · GHCR: https://docs.github.com/packages · GoReleaser: https://goreleaser.com/ · Dependabot: https://docs.github.com/code-security/dependabot
- Examples: [examples/61-ci-github-actions](examples/61-ci-github-actions/).
- Related in this plan: what CI runs — [18](18-testing.md) & [49](49-testing-kinds.md); the image it builds — [60](60-build-package.md); the deploy it feeds — [62](62-deployment-operations.md); `govulncheck` — [57](57-web-security.md).
