# Step 60 — Building & Packaging · 🟢 Easy

Examples **1–8**. The `go build` toolchain. Runnable examples have a real **Output**; retype the
Go file into a scratch folder and run the commands shown.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Compile and run

`🟢 easy` · *go build*

`go build` turns a package into one self-contained binary — no runtime, no interpreter, nothing to install alongside it. `-o` names the output. This single-artifact property is what makes Go so pleasant to ship.

**Steps:**

1. Put a `main` package in `./cmd/app` (or the current dir).
2. `go build -o app .` compiles it.
3. Run the binary directly.

```go
package main

import "fmt"

func main() {
	fmt.Println("hello from a compiled binary")
}
```

```bash
go build -o app .
./app
```

**Output:**

```
hello from a compiled binary
```

---

## 2. Build outputs and install

`🟢 easy` · *go build*

Three commands you'll use constantly: `go build -o` names the artifact, `go build ./...` compiles every package (a quick "does it all still build?"), and `go install` builds and drops the binary in `$GOBIN` (on your `PATH`).

**Steps:**

1. `-o` controls the output path/name.
2. `./...` matches all packages recursively.
3. `go install` installs to `$(go env GOBIN)` (or `$(go env GOPATH)/bin`).

```bash
go build -o bin/app ./cmd/app   # named output in bin/
go build ./...                  # compile everything (no output artifact)
go install ./cmd/app            # build + place in $GOBIN, onto your PATH
go env GOBIN GOPATH             # where install puts binaries
```

**Verify:** each command exits 0 on success and prints nothing (Unix convention); `go vet ./...` and `gofmt -l .` are the companion "is it clean?" checks.

---

## 3. Cross-compile for any OS/arch

`🟢 easy` · *cross-compile*

Go cross-compiles for free: set `GOOS` and `GOARCH` and it emits a binary for that target **from any host** — no cross-toolchain (as long as you don't use cgo). This is how one CI job produces Linux, macOS, Windows, and ARM builds.

**Steps:**

1. Prefix the build with `GOOS`/`GOARCH`.
2. Build the same source for three targets.
3. `file` confirms the produced architecture.

```bash
GOOS=linux   GOARCH=amd64 go build -o app-linux-amd64 .
GOOS=linux   GOARCH=arm64 go build -o app-linux-arm64 .
GOOS=windows GOARCH=amd64 go build -o app.exe .
file app-linux-amd64 app-linux-arm64 app.exe
```

**Output:** (architecture as reported by `file`)

```
linux/amd64   -> x86-64
linux/arm64   -> ARM aarch64
windows/amd64 -> PE32+
```

> See every target with `go tool dist list`.

---

## 4. A static binary

`🟢 easy` · *static*

`CGO_ENABLED=0` disables cgo, producing a **fully static** binary with no libc dependency — so it runs unchanged on `scratch`, distroless, or Alpine. This is the single most important flag for container builds. (Cross-compiling already implies cgo-off, but set it explicitly.)

**Steps:**

1. `CGO_ENABLED=0 go build`.
2. `file` shows "statically linked".
3. The binary needs no shared libraries at runtime.

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app .
file app | grep -o 'statically linked'
```

**Output:**

```
statically linked: true
```

---

## 5. Inject a version with -ldflags

`🟢 easy` · *ldflags*

Bake the release version into the binary at build time. Declare `var version = "dev"` and override it with `-ldflags "-X importpath.name=value"` — no config file, no env var, the version travels *with* the artifact.

**Steps:**

1. A package-level `var version string` with a default.
2. `-ldflags "-X main.version=1.4.2"` sets it at link time.
3. Without the flag, it stays `dev`.

```go
package main

import "fmt"

var version = "dev" // default; overridden at build time via -ldflags "-X main.version=..."

func main() {
	fmt.Println("version:", version)
}
```

```bash
go build -o app . && ./app                                  # default
go build -ldflags "-X main.version=1.4.2" -o app . && ./app # stamped
```

**Output:**

```
default:   version: dev
with -X:   version: 1.4.2
```

---

## 6. Read build metadata at runtime

`🟢 easy` · *buildinfo*

When you `go build` inside a git repo, Go automatically **stamps VCS metadata** into the binary. `runtime/debug.ReadBuildInfo()` reads it back — the commit (`vcs.revision`), build time (`vcs.time`), and whether the tree was dirty (`vcs.modified`) — plus the Go version and module dependency versions. Great for a `/version` endpoint (example 24).

**Steps:**

1. `debug.ReadBuildInfo()` returns the embedded info.
2. Scan `bi.Settings` for the `vcs.*` keys.
3. (This prints presence, not the actual commit hash, which varies.)

```go
package main

import (
	"fmt"
	"runtime/debug"
)

func main() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("no build info")
		return
	}
	find := func(key string) string {
		for _, s := range bi.Settings {
			if s.Key == key {
				return s.Value
			}
		}
		return ""
	}
	// Go stamps these VCS settings when you `go build` inside a repo (buildvcs).
	fmt.Println("go version present:  ", bi.GoVersion != "")
	fmt.Println("vcs:                 ", find("vcs"))
	fmt.Println("vcs.revision present:", find("vcs.revision") != "")
	fmt.Println("vcs.modified:        ", find("vcs.modified"))
}
```

```bash
go build -o app . && ./app   # build inside a git repo; note: `go run` doesn't stamp VCS
```

**Output:** (committed clean tree)

```
go version present:   true
vcs:                  git
vcs.revision present: true
vcs.modified:         false
```

---

## 7. Shrink the binary

`🟢 easy` · *size*

Two flags trim production binaries: `-ldflags="-s -w"` drops the symbol table and DWARF debug info (smaller, but no `gdb`/`delve`), and **`-trimpath`** removes absolute filesystem paths from the binary (smaller *and* reproducible — the same source produces identical bytes on any machine).

**Steps:**

1. Build normally, then with `-ldflags="-s -w" -trimpath`.
2. Compare the file sizes.
3. The stripped, trimmed build is smaller (and reproducible).

```bash
go build -o normal .
go build -ldflags="-s -w" -trimpath -o small .
ls -l normal small   # `small` is smaller (typically ~25-30% off a hello-world)
```

**Output:**

```
stripped+trimmed smaller than default: true
```

---

## 8. Build tags

`🟢 easy` · *build tags*

A `//go:build` constraint at the top of a file decides whether it's compiled. Pair a default file (`//go:build !prod`) with a variant (`//go:build prod`) to swap implementations at build time — dev vs prod defaults, real vs mock integrations — with no runtime branch.

**Steps:**

1. `main.go` references `buildKind`.
2. `dev.go` (`//go:build !prod`) and `prod.go` (`//go:build prod`) each define it.
3. `-tags prod` selects the prod file.

```go
// main.go
package main

import "fmt"

func main() {
	fmt.Println("build:", buildKind)
}
```

```go
// dev.go
//go:build !prod

package main

const buildKind = "development"
```

```go
// prod.go
//go:build prod

package main

const buildKind = "production"
```

```bash
go build -o app . && ./app             # default
go build -tags prod -o app . && ./app  # prod variant
```

**Output:**

```
default:    build: development
-tags prod: build: production
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
