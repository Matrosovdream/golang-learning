# 16 — Packages & Modules

## Goals
- Organize code into packages with clear, minimal public APIs.
- Understand modules, `go.mod`/`go.sum`, and dependency management end to end.
- Use `internal/` to enforce boundaries.
- Understand semantic versioning and how Go resolves dependency versions.

## Concepts
- **Package = directory.** All `.go` files in one directory belong to the same package (declared by `package name` at the top). The package name is usually the last path element. One package per directory.
- **The public API is capitalization.** Exported (Capitalized) identifiers are importable from other packages; lowercase ones are package-private. A package's exported surface *is* its API — keep it small and intentional.
- **Importing:** `import "example.com/go-project/store"`, then use `store.New(...)`. The import path is `<module path>/<dir path>`, not the filesystem path.
- **`init()` functions** — an optional `func init()` runs automatically when the package is first loaded, before `main`. Use rarely (e.g., registering a driver). Avoid heavy logic and side effects in `init`; explicit wiring is clearer.
- **Modules** — a module is a collection of packages versioned together, defined by a `go.mod` at its root. The module path (e.g., `github.com/you/proj`) is the import prefix for all its packages.
- **`go.mod` anatomy:**
  ```
  module github.com/you/proj
  go 1.22
  require (
      github.com/jackc/pgx/v5 v5.5.0
      github.com/stretchr/testify v1.9.0 // indirect
  )
  ```
  - `module` — the import path prefix.
  - `go` — the language version (affects available features & behavior).
  - `require` — direct (and `// indirect`) dependencies with versions.
- **`go.sum`** — checksums of every dependency (and its `go.mod`) for verifiable, tamper-proof builds. Commit it; never edit by hand.
- **Dependency commands:**
  - `go get example.com/pkg@v1.2.3` — add/upgrade a dependency.
  - `go get example.com/pkg@latest` — get the latest release.
  - `go mod tidy` — add missing + remove unused dependencies (run after editing imports).
  - `go mod download` — fetch into the module cache.
  - `go list -m all` — list the full dependency graph.
- **Semantic Import Versioning** — Go encodes the **major** version in the import path for v2+: `github.com/foo/bar/v2`. This lets v1 and v2 of a library coexist. Versions are `vMAJOR.MINOR.PATCH`; **MVS** (Minimal Version Selection) picks the *minimum* version that satisfies all requirements — reproducible by default.
- **`internal/`** — a special directory: packages under `.../internal/` can only be imported by code rooted at the parent of `internal/`. It's the language-enforced way to keep implementation details private to your module (heavily used in Part 7's project layout).
- **`replace` and vendoring:**
  - `replace example.com/foo => ../foo` in `go.mod` — point a dependency at a local path or fork (great for local dev across modules).
  - `go mod vendor` — copy dependencies into a `vendor/` dir for hermetic, offline builds (optional; many teams skip it).
- **Package naming conventions** — short, lowercase, no underscores, no plurals (`store`, not `stores` or `store_pkg`). The name is repeated at every call site, so keep it crisp. Avoid stutter: in package `user`, name the type `User` (used as `user.User`) — but prefer `user.Service` over `user.UserService`.

## Exercises
1. In `go-project/`, create a second package `mathx/` with an exported `Sum(nums ...int) int` and an unexported helper. Import and call it from `main`. Try to call the unexported helper from `main` and read the error.
2. Add a real dependency: `go get github.com/google/uuid`, use `uuid.New()`, then run `go mod tidy` and inspect how `go.mod`/`go.sum` changed.
3. Create an `internal/secret/` package and try to import it from a sibling module path you don't own (or simulate the boundary); confirm the `internal` rule.
4. Add an `init()` to a package that prints a line; observe when it runs relative to `main`.
5. Run `go list -m all` and `go mod graph` to see the dependency tree of your module.
6. Practice the naming rule: refactor a `user.UserService` into `user.Service` and discuss why the stutter is discouraged.

## Best Practices & Pitfalls
- **Design packages around responsibilities, not layers-of-one-type.** A package should do one thing and expose a small API. Avoid a junk-drawer `utils`/`common` package.
- **Keep the exported surface minimal.** Every exported name is a promise you maintain. Unexport anything callers don't need.
- **Use `internal/` to hide implementation** so external code can't depend on details you want to change freely.
- **Run `go mod tidy` before committing.** It keeps `go.mod`/`go.sum` honest and removes stray deps.
- **Pitfall — import cycles.** Go forbids circular imports between packages. They usually signal a design problem; break the cycle by extracting a shared package or inverting a dependency with an interface.
- **Pitfall — `init()` overuse.** Hidden initialization order and side effects make programs hard to reason about; prefer explicit constructors called from `main`.
- **Pitfall — choosing a throwaway module path.** If you'll publish, use the real repo path from day one to avoid rewriting imports.
- **Pin and review dependency upgrades.** Read `go.sum` diffs in PRs; supply-chain matters.

## Checklist
- [ ] I understand package = directory and that capitalization defines the API.
- [ ] I can read every field of a `go.mod` and explain `go.sum`.
- [ ] I can add, upgrade, and tidy dependencies.
- [ ] I know how `internal/` restricts imports.
- [ ] I understand semantic import versioning and MVS at a high level.
- [ ] I can name packages idiomatically and avoid stutter & import cycles.

## Resources
- Blog — Using Go Modules: https://go.dev/blog/using-go-modules
- Reference — Go Modules: https://go.dev/ref/mod
- Blog — Package names: https://go.dev/blog/package-names
- Effective Go — package comments & names: https://go.dev/doc/effective_go#package-names
