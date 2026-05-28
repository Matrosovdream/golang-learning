# 02 — Environment Setup

## Goals
- Install Go and verify the toolchain works.
- Understand the `go` command's most important subcommands.
- Understand modules (the modern dependency system) and why GOPATH no longer matters.
- Set up an editor with Go language support.

## Concepts
- **Installing Go** — download from https://go.dev/dl/ (or `brew install go` on macOS). Verify with `go version`. We assume **Go 1.22+** (this course uses 1.22's HTTP routing later).
- **The `go` command** is your entire build system. The ones you'll use constantly:
  - `go run .` — compile and run the current package in one step (great while learning; no binary left behind).
  - `go build` — compile to a binary in the current dir.
  - `go fmt ./...` — format all code (wraps `gofmt`). Run it always.
  - `go vet ./...` — static analysis that catches likely bugs (e.g., wrong `Printf` verbs).
  - `go test ./...` — run all tests.
  - `go mod init <module-path>` / `go mod tidy` — manage dependencies.
  - `go doc <pkg>` — read docs in the terminal (e.g., `go doc strings.Builder`).
  - `go env` — show environment (`GOROOT`, `GOPATH`, `GOMODCACHE`, etc.).
  - `./...` means "this directory and all subdirectories" — a common pattern.
- **Modules vs the old GOPATH world** — Go used to require all code to live under a single `$GOPATH/src` tree. **You can ignore that now.** Since Go 1.16, **modules are the default**: a project is any directory with a `go.mod` file, and it can live anywhere.
- **`go mod init`** — creates `go.mod` with your **module path** (a unique import prefix, conventionally a repo URL like `github.com/you/project`, but for local learning `example.com/go-project` is fine). Example:
  ```
  mkdir go-project && cd go-project
  go mod init example.com/go-project
  ```
- **`go.mod` and `go.sum`** — `go.mod` lists your module path, the Go version, and direct dependencies. `go.sum` records cryptographic checksums of every dependency for reproducible, verifiable builds. Both are committed to git.
- **`GOPATH` today** — still exists, but only as a cache/install location (`$GOPATH/pkg/mod` holds downloaded modules, `$GOPATH/bin` holds installed tools). You don't put your project there.
- **`gopls`** — the official Go **language server**. It powers autocomplete, go-to-definition, inline errors, and refactoring in editors. Editor extensions install it for you.
- **Editor setup** — VS Code: install the official **Go** extension (`golang.go`); it will prompt to install `gopls` and tools. GoLand (JetBrains) works out of the box. On save, enable "format on save" so `gofmt` runs automatically.

## Exercises
1. Run `go version` and `go env GOROOT GOPATH` and read what they print. Ask Claude what each path is for.
2. Create your course module:
   ```
   mkdir go-project && cd go-project
   go mod init example.com/go-project
   ```
   Open `go.mod` and read its contents.
3. Make a throwaway `hello.go` with a `main` function that prints something, then run it with `go run .`. Delete the file after.
4. Run `go fmt ./...` and `go vet ./...` in the module (even with no issues) so you know what "clean" output looks like.
5. Install your editor's Go extension and confirm autocomplete + go-to-definition work on `fmt.Println`.

## Best Practices & Pitfalls
- **Always work inside a module.** If you see errors like *"go.mod file not found"*, you're running `go` outside a module directory — `cd` into one or `go mod init`.
- **Run `go mod tidy` after changing imports.** It adds missing dependencies and removes unused ones, keeping `go.mod`/`go.sum` accurate.
- **Commit both `go.mod` and `go.sum`.** Never gitignore `go.sum` — it's what makes builds reproducible and secure.
- **Use `go run .` (with the dot), not `go run main.go`,** once you have multiple files in a package — the dot builds the whole package, a single filename does not.
- **Pitfall:** picking a module path you'll regret. If you'll ever publish to GitHub, init with the real repo path (`github.com/you/go-project`) from the start to avoid renaming imports later.

## Checklist
- [ ] `go version` prints 1.22 or newer.
- [ ] I created `go-project/` with a `go.mod`.
- [ ] I ran a program with `go run .`.
- [ ] My editor shows autocomplete and errors via `gopls`.
- [ ] I know what `go fmt`, `go vet`, `go test`, and `go mod tidy` do.

## Resources
- Download & install: https://go.dev/doc/install
- Tutorial: Create a module: https://go.dev/doc/tutorial/create-module
- Managing dependencies: https://go.dev/doc/modules/managing-dependencies
- Editor/IDE setup: https://go.dev/doc/editors
- `go` command reference: https://pkg.go.dev/cmd/go
