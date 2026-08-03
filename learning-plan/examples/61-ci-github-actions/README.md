# Step 61 — CI with GitHub Actions · Examples

A library of **26 examples**, split into three files by difficulty.

Two shapes (as in [step 60](../60-build-package/)):

- **Runnable** (the Go commands CI runs): a real **Output** or a **Verify** note — run before being added.
- **Reference config** (`.github/workflows/*.yml`, `.golangci.yml`, `.goreleaser.yaml`, `dependabot.yml`): a complete, copy-pasteable file with a **Verify** note. Every YAML file here was **syntax-validated** (parsed with a YAML loader). No `actionlint` was available in this environment, so the workflows are additionally **reviewed against the GitHub Actions schema** — treat them as production-ready references, but run them in your repo to confirm action versions.

The Go command outputs were captured on a tiny module (`ci-demo`) with a `mymath` package at 100% coverage and a deliberately racy test.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — the CI commands & a first workflow

- [1. Run the test suite](1-easy.md#1-run-the-test-suite)
- [2. The race detector](1-easy.md#2-the-race-detector)
- [3. Coverage](1-easy.md#3-coverage)
- [4. Format and vet gates](1-easy.md#4-format-and-vet-gates)
- [5. A first workflow](1-easy.md#5-a-first-workflow)
- [6. Workflow anatomy](1-easy.md#6-workflow-anatomy)
- [7. Triggers](1-easy.md#7-triggers)
- [8. setup-go and caching](1-easy.md#8-setup-go-and-caching)

### 🟡 [Medium](2-medium.md) — a real pipeline

- [9. Manual caching with actions/cache](2-medium.md#9-manual-caching-with-actionscache)
- [10. Linting with golangci-lint](2-medium.md#10-linting-with-golangci-lint)
- [11. A gofmt gate](2-medium.md#11-a-gofmt-gate)
- [12. Scan dependencies with govulncheck](2-medium.md#12-scan-dependencies-with-govulncheck)
- [13. A build matrix](2-medium.md#13-a-build-matrix)
- [14. Job dependencies with needs](2-medium.md#14-job-dependencies-with-needs)
- [15. Build and upload artifacts](2-medium.md#15-build-and-upload-artifacts)
- [16. Secrets, env, and permissions](2-medium.md#16-secrets-env-and-permissions)
- [17. Concurrency and cancel-in-progress](2-medium.md#17-concurrency-and-cancel-in-progress)

### 🔴 [Hard](3-hard.md) — quality gates, releases, integration

- [18. Test output as JSON](3-hard.md#18-test-output-as-json)
- [19. Integration tests with a service container](3-hard.md#19-integration-tests-with-a-service-container)
- [20. Build and push a Docker image](3-hard.md#20-build-and-push-a-docker-image)
- [21. Release with GoReleaser](3-hard.md#21-release-with-goreleaser)
- [22. Cache Docker build layers](3-hard.md#22-cache-docker-build-layers)
- [23. Reusable workflows and composite actions](3-hard.md#23-reusable-workflows-and-composite-actions)
- [24. A coverage gate](3-hard.md#24-a-coverage-gate)
- [25. Automated dependency updates](3-hard.md#25-automated-dependency-updates)
- [26. Capstone: a complete CI pipeline](3-hard.md#26-capstone-a-complete-ci-pipeline)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
