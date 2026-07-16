# 60 — Building & Packaging Go Services

> Part of **Part 13 — CI/CD & Deployment**, the delivery track: **60 build & package** → [61 — CI with GitHub Actions](61-ci-github-actions.md) → [62 — Deployment & Operations](62-deployment-operations.md). Builds on [16 — Packages & Modules](16-packages-modules.md), [19 — Standard Library](19-stdlib-tour.md), and the Dockerfiles in every example-project; the container-runtime tuning (`GOMAXPROCS`/`GOMEMLIMIT`) lands in [62](62-deployment-operations.md). Thesis: **Go's killer feature for ops is a single static binary — no runtime, no interpreter, no dependencies. Learn the `go build` flags (cross-compile, `-ldflags` version stamping, `-trimpath`), `embed` your assets into that binary, then wrap it in a tiny multi-stage container, and your artifact is a few-hundred-KB image that runs anywhere.**

## Goals
- Drive `go build`: output names, **cross-compilation** (`GOOS`/`GOARCH`), **static** binaries (`CGO_ENABLED=0`), **build tags**, and shrinking (`-ldflags="-s -w"`, `-trimpath`).
- **Stamp version metadata** into the binary with `-ldflags "-X"`, and read Go's automatic **VCS build info** at runtime (`runtime/debug.ReadBuildInfo`), exposed via a `/version` endpoint.
- **Embed** files, directories, templates, and static assets into the binary with `//go:embed` — ship one artifact.
- Package with a **multi-stage Dockerfile**: a `golang` builder + a minimal (`scratch`/distroless, nonroot) runtime, with **layer caching**, a `.dockerignore`, and a build-`ARG` version.
- Round it out: **Docker Compose** for local dev, a **Makefile**, **multi-arch** images, image **security hardening**, **reproducible** builds, and **vendoring**.

## Concepts

- **`go build` produces one self-contained binary.** No shared libraries to ship, no runtime to install. `go build -o app ./cmd/app` builds a package; `go install` puts it in `$GOBIN`; `go build ./...` builds everything.
- **Cross-compilation is free.** Set `GOOS`/`GOARCH` and Go emits a binary for that target from any host (`GOOS=linux GOARCH=arm64 go build`). No cross-toolchain — as long as you don't use cgo.
- **`CGO_ENABLED=0` gives a fully static binary.** With cgo off there's no libc dependency, so the binary runs on `scratch`/distroless/Alpine unchanged. This is the default when cross-compiling (no C cross-compiler present), but set it explicitly for container builds.
- **Stamp the version at build time.** A `var version = "dev"` overridden with `-ldflags "-X main.version=1.4.2"` bakes the release into the binary — no config file, no env var. Combine with **`runtime/debug.ReadBuildInfo`**, which exposes Go's automatic **VCS stamping** (`vcs.revision`, `vcs.time`, `vcs.modified`) when you `go build` inside a repo. Expose both from a `/version` endpoint for ops.
- **Shrink and pin for production.** `-ldflags="-s -w"` strips the symbol table and DWARF (smaller binary, no debugger); **`-trimpath`** removes local filesystem paths (smaller, and **reproducible** — the same source yields the same bytes).
- **Build tags select files per environment.** A `//go:build prod` file compiles only with `-tags prod`; the `//go:build !prod` file is the default. Use it for build-time variants (mock vs real integrations, dev vs prod defaults).
- **`//go:embed` ships assets inside the binary.** `//go:embed version.txt` → a `string`/`[]byte`; `//go:embed assets` → an `embed.FS` you can `ReadFile`, `fs.WalkDir`, `template.ParseFS`, or serve with `http.FileServerFS`. Migrations, templates, and static files all live in the one artifact — nothing to mount in production.
- **Package with a multi-stage Dockerfile.** Stage 1 (`FROM golang:1.26`) compiles; stage 2 (`FROM scratch` or `gcr.io/distroless/static`) copies just the binary. The result is a few hundred KB / a few MB, with **no shell, no package manager, no OS** to attack. **Order layers for caching**: copy `go.mod`/`go.sum` and `go mod download` *before* the source, so a code change doesn't re-download dependencies. A **`.dockerignore`** keeps the build context (and image) small; a build **`ARG VERSION`** feeds the `-ldflags` stamp.
- **Harden the image.** Run as a **nonroot** user (`USER 65534`), prefer a **read-only** root filesystem, keep the image minimal (scratch/distroless), pin the base by **digest** for reproducibility, and **scan** it (`govulncheck` for Go deps, Trivy/Grype for the image).
- **The rest of the toolchain.** **Docker Compose** wires the app to Postgres with healthchecks for local dev; a **Makefile** captures the build/test/lint/image commands; **`docker buildx`** produces **multi-arch** (amd64+arm64) images; **`go mod vendor`** commits dependencies for hermetic, network-free CI builds.

## Exercises
1. `go build -o app ./cmd/app` and run it; cross-compile the same source for linux/amd64, linux/arm64, and windows/amd64 and `file` the outputs.
2. Build with `CGO_ENABLED=0` and confirm the binary is statically linked; stamp a version with `-ldflags "-X"` and print it.
3. Read `runtime/debug.ReadBuildInfo` (from inside a git repo) and print the VCS revision/modified flags.
4. Compare a default build to one with `-ldflags="-s -w" -trimpath`; add a `//go:build prod` variant.
5. Embed a `version.txt` string, an `embed.FS` directory, and serve embedded static files with `http.FileServerFS`.
6. Write a **multi-stage Dockerfile** (`golang` → `scratch`, nonroot) with a `.dockerignore` and a `VERSION` build-arg; build it and check the image size.
7. Write a **Compose** file (app + Postgres + healthcheck) and validate it with `docker compose config`.
8. Write a **Makefile** with `build`/`test`/`lint`/`image` targets; add a `/version` HTTP endpoint fed by the stamped version.
9. Stretch: build a **multi-arch** image with `docker buildx`; pin the base image by digest; run `govulncheck`.

## Best Practices & Pitfalls
- **Always `CGO_ENABLED=0` for container images** (unless you genuinely need cgo) — otherwise the binary needs a libc the minimal image doesn't have, and it won't start.
- **Stamp version + VCS info; expose `/version`.** "What's actually running?" should be answerable from the artifact, not a wiki.
- **Pitfall — copying source before `go mod download`.** Every code change then re-downloads all modules. Copy `go.mod`/`go.sum` first.
- **Pitfall — a fat single-stage image.** Shipping the `golang` builder image (800 MB+, with a compiler and shell) as your runtime is a huge, insecure artifact. Multi-stage → scratch/distroless.
- **Pitfall — running as root in the container.** Set a nonroot `USER`; prefer a read-only root FS.
- **Pitfall — a missing `.dockerignore`.** Without it, `.git`, local binaries, and secrets get copied into the build context (and can leak into layers).
- **Pin base images by digest** for reproducibility; `-trimpath` for reproducible binaries.
- **`scratch` has no CA certs or timezone data.** If your app makes TLS calls, `COPY` `ca-certificates` in (or use `distroless/static`, which includes them).

## Checklist
- [ ] I can `go build`, cross-compile with `GOOS`/`GOARCH`, and produce a static `CGO_ENABLED=0` binary.
- [ ] I stamp a version with `-ldflags "-X"`, read VCS info via `ReadBuildInfo`, and serve `/version`.
- [ ] I shrink with `-s -w`/`-trimpath` and use build tags for env variants.
- [ ] I embed files/dirs/templates with `//go:embed` and serve them.
- [ ] I write a multi-stage Dockerfile (scratch/distroless, nonroot) with layer caching, `.dockerignore`, and a build ARG.
- [ ] I have a Compose file (validated) and a Makefile for the common commands.
- [ ] I can build multi-arch images, pin bases by digest, and scan the image.

## Resources
- `go build`/`go install` & environment (`GOOS`/`GOARCH`/`CGO_ENABLED`): https://pkg.go.dev/cmd/go · https://go.dev/wiki/GcToolchainTricks
- `-ldflags` & version stamping: https://pkg.go.dev/cmd/link · `runtime/debug.ReadBuildInfo`: https://pkg.go.dev/runtime/debug#ReadBuildInfo
- `embed`: https://pkg.go.dev/embed
- Docker multi-stage builds: https://docs.docker.com/build/building/multi-stage/ · distroless: https://github.com/GoogleContainerTools/distroless · `docker buildx` (multi-arch): https://docs.docker.com/build/building/multi-platform/
- `govulncheck`: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck · GoReleaser: https://goreleaser.com/
- Examples: [examples/60-build-package](examples/60-build-package/).
- Related in this plan: modules in [16](16-packages-modules.md); config/logging in [23](23-config-logging.md); container runtime tuning in [62](62-deployment-operations.md); CI that runs these builds in [61](61-ci-github-actions.md).
