# Step 60 — Building & Packaging · 🟡 Medium

Examples **9–17**. Embedding assets into the binary, then packaging it in a container. Runnable
examples have an **Output**; Dockerfiles are complete reference files with a **Verify** note.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

---

## 9. Embed a file

`🟡 medium` · *embed*

`//go:embed` copies a file's contents into a package variable at compile time — so config, a version string, or a license ships *inside* the binary with nothing to read from disk at runtime. A `//go:embed file` directive on a `string`/`[]byte` var loads that file.

**Steps:**

1. Import `embed` (blank import if you only use the directive).
2. `//go:embed version.txt` above a `string` var.
3. The contents are available immediately at startup.

```go
package main

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed version.txt
var version string

func main() {
	fmt.Println("embedded version:", strings.TrimSpace(version))
}
```

> With `version.txt` containing `1.4.2`.

**Output:**

```
embedded version: 1.4.2
```

---

## 10. Embed a directory

`🟡 medium` · *embed*

`//go:embed dir` on an `embed.FS` embeds a whole tree — templates, static assets, SQL migrations — as a read-only filesystem you can `ReadFile`, `fs.WalkDir`, or hand to `template.ParseFS`. One artifact, no mounted volumes.

**Steps:**

1. `//go:embed assets` on a `var assets embed.FS`.
2. `fs.WalkDir` lists the tree; `assets.ReadFile` reads a file.
3. Paths are relative to the module (include the `assets/` prefix).

```go
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed assets
var assets embed.FS

func main() {
	var names []string
	fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	fmt.Println("embedded files:", names)
	data, _ := assets.ReadFile("assets/a.txt")
	fmt.Printf("assets/a.txt = %q\n", string(data))
}
```

> With `assets/a.txt` = `alpha` and `assets/b.txt` = `beta`.

**Output:**

```
embedded files: [assets/a.txt assets/b.txt]
assets/a.txt = "alpha\n"
```

---

## 11. Serve embedded files over HTTP

`🟡 medium` · *embed*

An `embed.FS` plugs straight into the HTTP server: `http.FileServerFS` (Go 1.22+) serves the embedded tree as a static file handler. Your SPA/assets ship in the binary — deploy is a single file, and there's no `static/` directory to forget to copy.

**Steps:**

1. Embed `static` as an `embed.FS`.
2. `http.FileServerFS(static)` serves it.
3. Request an embedded path and read it back.

```go
package main

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

//go:embed static
var static embed.FS

func main() {
	// Serve embedded files straight from the binary — no files on disk in production.
	srv := httptest.NewServer(http.FileServerFS(static))
	defer srv.Close()

	resp, _ := http.Get(srv.URL + "/static/index.html")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("status=%d body=%q\n", resp.StatusCode, string(body))
}
```

> With `static/index.html` = `<h1>hi</h1>`.

**Output:**

```
status=200 body="<h1>hi</h1>"
```

---

## 12. A naive Dockerfile (anti-pattern)

`🟡 medium` · *docker*

The tempting first Dockerfile is also the worst: it ships the entire `golang` build image — ~800 MB with a compiler, shell, and package manager — as your **runtime**. That's a huge artifact and a large attack surface. Shown here as the thing to *avoid* (fixed in example 13).

```dockerfile
# ANTI-PATTERN — do not ship this.
FROM golang:1.26
WORKDIR /src
COPY . .
RUN go build -o /app ./cmd/app
CMD ["/app"]
# Result: ~800 MB image containing a full Go toolchain + shell in production.
```

**Verify:** builds fine, but `docker images` shows a ~800 MB image — compare with the ~650 KB of example 13/26. The problem isn't correctness, it's size and attack surface.

---

## 13. A multi-stage Dockerfile

`🟡 medium` · *docker*

The fix: a **build stage** that compiles, and a **runtime stage** that copies *only* the binary. The `golang` toolchain never ships. `CGO_ENABLED=0` makes the binary static so the tiny runtime image can run it.

```dockerfile
# --- build stage ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /app ./cmd/app

# --- runtime stage: only the binary travels here ---
FROM alpine:latest
COPY --from=build /app /app
ENTRYPOINT ["/app"]
```

**Verify:** `docker build -t app .` — the build stage compiles with the toolchain, the runtime stage is just Alpine + the binary (a few MB). Example 26 takes this to a ~650 KB `scratch` image (verified: `docker build` → run prints the stamped version).

---

## 14. A scratch / distroless runtime

`🟡 medium` · *docker*

For the smallest, hardest-to-attack image, base the runtime on **`scratch`** (literally empty) or **distroless** (a minimal base with CA certs + tzdata but no shell/package manager). A static Go binary needs nothing else. `scratch` has **no CA certificates** — add them if you make TLS calls.

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o /app ./cmd/app

# scratch = empty base. Add certs only if you make outbound TLS calls.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app /app
ENTRYPOINT ["/app"]
# Alternative runtime with certs+tzdata included:
#   FROM gcr.io/distroless/static:nonroot
```

**Verify:** `docker build` succeeds; the `scratch` image is a few hundred KB. (No shell means no `docker exec … sh` — that's the point; debug with an ephemeral/sidecar container.)

---

## 15. .dockerignore

`🟡 medium` · *docker*

The build context is everything Docker uploads to the daemon. A `.dockerignore` keeps `.git`, local binaries, and secrets **out** of it — faster builds, and no chance of a stray file leaking into an image layer. It reads like `.gitignore`.

```
.git/
bin/
*.md
Dockerfile
.dockerignore
.env
**/*_test.go
```

**Verify:** with this file present, `docker build` uploads a smaller context (watch the "transferring context" size); the `.git` directory and local `bin/` never enter the image.

---

## 16. Version the image with a build ARG

`🟡 medium` · *docker*

Feed the release version into the image build with a Docker **`ARG`**, then pass it to the `-ldflags "-X"` stamp (example 5). Now the image and the binary inside it agree on their version, set once at `docker build` time.

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o /app ./cmd/app

FROM scratch
COPY --from=build /app /app
ENTRYPOINT ["/app"]
```

```bash
docker build --build-arg VERSION=1.4.2 -t app:1.4.2 .
docker run --rm app:1.4.2   # prints: version: 1.4.2
```

**Verify:** built and run locally — `docker run` prints `version: 1.4.2` (the same `-ldflags "-X"` mechanism from example 5, driven by the build ARG).

---

## 17. Reproducible builds

`🟡 medium` · *reproducibility*

A reproducible build yields **byte-identical** artifacts from the same source, anywhere — essential for auditing "is this the binary the code produced?". Three levers: `-trimpath` (strip local paths), pin the toolchain, and **pin the base image by digest** (not a moving tag).

```dockerfile
# Pin the base by DIGEST, not a mutable tag, so the build can't drift.
FROM golang:1.26@sha256:<digest> AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# -trimpath removes machine-specific paths; the toolchain version is pinned above.
RUN CGO_ENABLED=0 GOFLAGS=-trimpath go build -ldflags="-s -w" -o /app ./cmd/app

FROM gcr.io/distroless/static@sha256:<digest>
COPY --from=build /app /app
ENTRYPOINT ["/app"]
```

**Verify:** building the same commit twice with `-trimpath` + pinned bases produces identical binaries (`sha256sum` matches). Get a real digest with `docker buildx imagetools inspect golang:1.26`.

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
