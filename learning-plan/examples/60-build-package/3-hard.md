# Step 60 — Building & Packaging · 🔴 Hard

Examples **18–26**. Caching, Compose, a Makefile, security, multi-arch, and a production capstone.
Config files carry a **Verify** note; the runnable `/version` example has an **Output**.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

---

## 18. Layer caching for fast rebuilds

`🔴 hard` · *docker*

Docker caches each layer and rebuilds from the first change downward. So **copy `go.mod`/`go.sum` and `go mod download` before the source**: a code edit then reuses the cached dependency layer instead of re-downloading every module. Order layers from least- to most-frequently-changed.

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src

# 1. Dependency layer — changes only when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# 2. Source layer — changes on every code edit, but deps stay cached above.
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /app ./cmd/app
```

**Verify:** edit a `.go` file and rebuild — the `go mod download` layer shows `CACHED` and the rebuild skips straight to compilation. Swap the two `COPY`s and every rebuild re-downloads modules.

---

## 19. Docker Compose for local dev

`🔴 hard` · *compose*

Compose wires your app to its dependencies for local development: the app (built from the Dockerfile) plus Postgres, on one network, with one `docker compose up`. Pass the build `ARG`, set env, and depend on the database being **healthy** (example 20).

```yaml
services:
  app:
    build:
      context: .
      args: { VERSION: "1.4.2" }
    ports: ["8080:8080"]
    environment:
      DATABASE_URL: postgres://app:secret@db:5432/app?sslmode=disable
    depends_on:
      db: { condition: service_healthy }
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: app
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
      timeout: 3s
      retries: 5
```

**Verify:** validated with `docker compose config` (parses and normalizes the file — output was `OK`). `docker compose up --build` starts both services; the app waits for Postgres to be healthy.

---

## 20. Compose healthchecks and dependencies

`🔴 hard` · *compose*

`depends_on` alone only waits for a container to **start**, not to be **ready** — so the app can race ahead of a database that's still initializing. A **`healthcheck`** plus `condition: service_healthy` makes the dependency wait for actual readiness.

```yaml
services:
  app:
    build: .
    depends_on:
      db: { condition: service_healthy }   # wait for READY, not just "started"
  db:
    image: postgres:17-alpine
    environment: { POSTGRES_USER: app, POSTGRES_PASSWORD: secret, POSTGRES_DB: app }
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]  # Postgres' own readiness probe
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 10s   # grace period before failures count
```

**Verify:** `docker compose config` accepts it; on `up`, Compose polls `pg_isready` and only starts `app` once the DB reports healthy. (Your app should *still* retry its DB connection on startup — a healthcheck reduces but doesn't eliminate the race.)

---

## 21. A Makefile

`🔴 hard` · *tooling*

A `Makefile` captures the project's commands so nobody has to remember the flags — and CI calls the same targets you do locally. Note the **tab** indentation (Make requires real tabs) and the `VERSION` derived from git.

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint image

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/app ./cmd/app

test:
	go test -race -cover ./...

lint:
	go vet ./...
	gofmt -l .

image:
	docker build --build-arg VERSION=$(VERSION) -t app:$(VERSION) .
```

**Verify:** `make build` compiles with the git-derived version; `make test`/`make lint`/`make image` run the same commands your CI ([step 61](../61-ci-github-actions.md)) will. (Indent recipe lines with tabs, not spaces.)

---

## 22. Image security hardening

`🔴 hard` · *security*

A container image is an attack surface — shrink it and drop privileges. Run as a **nonroot** user, use a **read-only** root filesystem, keep the base minimal, and **scan** both Go deps and the image. ([Step 57](../57-web-security/) covers app-level security; this is the artifact.)

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /app ./cmd/app

FROM gcr.io/distroless/static:nonroot   # minimal base, already nonroot
COPY --from=build /app /app
USER 65532:65532                         # nonroot uid (belt-and-suspenders)
ENTRYPOINT ["/app"]
```

```bash
govulncheck ./...                        # scan Go dependencies for known CVEs
docker run --read-only --cap-drop ALL app  # read-only FS, no Linux capabilities
# trivy image app:1.4.2                  # scan the built image
```

**Verify:** `docker build` produces a nonroot, distroless image; `docker run --read-only` starts (a stateless Go service needs no writable root). `govulncheck ./...` reports known-vulnerable dependencies.

---

## 23. Multi-arch images

`🔴 hard` · *multi-arch*

Apple Silicon laptops are arm64; most cloud is amd64. `docker buildx` builds a **multi-arch** image (one tag, both architectures) in a single push — the registry serves each host the right one. Go's free cross-compilation makes this cheap.

```bash
# One-time: a builder that supports multiple platforms.
docker buildx create --use

# Build + push both architectures under one tag.
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=1.4.2 -t registry/app:1.4.2 --push .

# Inspect the resulting manifest list.
docker buildx imagetools inspect registry/app:1.4.2
```

**Verify:** `imagetools inspect` shows a manifest list with both `linux/amd64` and `linux/arm64` entries; each host pulls its native image automatically. (For pure-Go builds, buildx cross-compiles per platform — fast, no emulation.)

---

## 24. A /version endpoint

`🔴 hard` · *ops*

Expose the stamped version + Go build info over HTTP so ops can ask a running service "what are you?" without a shell. It combines the `-ldflags` version (example 5) with `ReadBuildInfo` (example 6).

**Steps:**

1. A handler returns the `version` var + `bi.GoVersion` as JSON.
2. In production the version comes from the build ARG → `-ldflags "-X"`.
3. `curl /version` (or your load balancer's health page) reports it.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
)

var version = "dev" // set at build time via -ldflags "-X main.version=..."

func versionHandler(w http.ResponseWriter, r *http.Request) {
	info := map[string]string{"version": version}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info["go"] = bi.GoVersion
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func main() {
	srv := httptest.NewServer(http.HandlerFunc(versionHandler))
	defer srv.Close()
	resp, _ := http.Get(srv.URL)
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	fmt.Println("version field: ", got["version"])
	fmt.Println("has go version:", got["go"] != "")
}
```

**Output:**

```
version field:  dev
has go version: true
```

---

## 25. Vendoring for reproducible CI

`🔴 hard` · *modules*

`go mod vendor` copies every dependency into a `vendor/` directory committed to the repo. Builds then use those exact files with **no network** — hermetic, reproducible, and immune to a deleted upstream module or a registry outage. Go uses `vendor/` automatically when it's present.

```bash
go mod vendor          # copy all deps into ./vendor (commit it)
go build -mod=vendor ./...   # build from vendor/, no network needed
go mod verify          # check module cache matches go.sum (integrity)
```

**Verify:** with a populated `vendor/`, `go build` works offline (`GOFLAGS=-mod=vendor` or automatic when `vendor/` exists). Trade-off: a larger repo and vendor churn in diffs — many teams skip it and rely on the module proxy + `go.sum` instead.

---

## 26. Capstone: a production Dockerfile

`🔴 hard` · *capstone*

Everything from this lesson in one artifact: multi-stage build, cached dependency layer, **static** binary, **stamped** version via build ARG, **stripped + trimmed**, a **`scratch`** runtime with CA certs, and a **nonroot** user. The result is a self-contained image measured in **hundreds of kilobytes**.

```dockerfile
# ---------- build ----------
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download                      # cached dependency layer
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -trimpath -o /app ./cmd/app          # static, stamped, stripped, reproducible

# ---------- runtime ----------
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app /app
USER 65534:65534                         # nonroot
ENTRYPOINT ["/app"]
```

```bash
docker build --build-arg VERSION=1.4.2 -t app:1.4.2 .
docker run --rm app:1.4.2   # prints the stamped version
docker image inspect app:1.4.2 --format '{{.Size}}'
```

**Verify:** built and run with a local Docker daemon — `docker run` prints `version: 1.4.2`, and `docker image inspect` reports a **~665 KB** image (`scratch` + a static Go binary). This is your deployable artifact for [step 62](../62-deployment-operations.md).

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
