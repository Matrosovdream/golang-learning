# 24 — Idiomatic Go & Effective Go

## Goals
- Internalize the conventions that make code "look like Go."
- Apply the most important idioms: naming, zero values, error style, interfaces.
- Set up `gofmt`, `go vet`, and `golangci-lint` as a quality gate.
- Write good doc comments.

## Concepts
- **Idiomatic Go is a real, enforced thing.** The community values consistency highly, codified in *Effective Go*, *Code Review Comments*, and *Google's Go Style Guide*. Writing idiomatic code makes your work readable to every other Go dev.
- **Naming:**
  - `MixedCaps`/`mixedCaps`, **never** snake_case. Capitalization = visibility (lesson 03).
  - **Short names for short scopes** (`i`, `r`, `buf`, `ctx`, `err`); longer, descriptive names for package-level identifiers.
  - **Avoid stutter:** in package `user`, expose `user.Service`, not `user.UserService`; `bytes.Buffer`, not `bytes.BytesBuffer`.
  - **Getters drop "Get":** `u.Name()`, not `u.GetName()`. Setters keep `Set`.
  - **Interfaces with one method are named `MethodName + er`:** `Reader`, `Writer`, `Stringer`, `Handler`.
  - **Acronyms stay uppercase:** `userID`, `ServeHTTP`, `parseURL` — not `userId`/`ServeHttp`.
- **The zero value should be useful.** Design types so the zero value works without initialization: `var b bytes.Buffer` is ready to use; `sync.Mutex{}` is unlocked and ready. Avoid requiring an `Init()` call when the zero value could just work.
- **"Accept interfaces, return concrete types"** (lesson 11) — flexible inputs, concrete useful outputs. Define interfaces at the consumer, keep them small.
- **Error handling style** (lesson 12) — check errors immediately, return early, wrap with `%w` and a short context phrase, don't `panic` for expected failures, don't ignore errors.
- **Return early; keep the happy path un-indented.** Handle errors/edge cases and `return`, so the main logic isn't buried in nested `if`s. This "line of sight" style is core Go aesthetics.
- **A little copying is better than a little dependency.** Don't add a dependency (or a heavy abstraction) to save a few lines. Prefer simple, duplicated code over a premature shared abstraction.
- **Make the zero value and the simple path obvious.** Constructors are plain functions named `New`/`NewThing` returning `(*Thing, error)` — there are no constructors as a language feature.
- **Comments & docs:**
  - Doc comments are regular comments **immediately above** a declaration, starting with the **name**: `// User represents an account holder.`
  - Every exported identifier should have a doc comment. Package docs go above `package x` in one file (often `doc.go`).
  - `go doc ./...` and `pkg.go.dev` render these. Comments explain **why**, not what.
- **The tooling that enforces quality:**
  - **`gofmt` / `go fmt`** — formatting; non-negotiable, run on save.
  - **`go vet`** — catches suspicious constructs (bad `Printf` verbs, lock copies, unreachable code).
  - **`golangci-lint`** — a fast meta-linter bundling `staticcheck`, `errcheck`, `govet`, `ineffassign`, etc. The de-facto standard for CI. Configure via `.golangci.yml`.
  - **`staticcheck`** — excellent standalone analyzer (included in golangci-lint).
- **Don't over-engineer.** Go culture favors simple, boring, explicit code over clever abstractions, deep inheritance-like embedding, or framework magic. Solve today's problem.

## Exercises
1. Audit a file you wrote earlier for naming: fix any `GetX` getters, `userId`-style acronyms, stutter (`user.UserService`), or snake_case.
2. Refactor a deeply nested function to the early-return style and compare readability.
3. Install `golangci-lint`, add a minimal `.golangci.yml`, and run it on your `go-project/`. Fix (or justify) each finding.
4. Run `go vet ./...` and intentionally introduce a `Printf` verb mismatch (`%d` with a string) to see vet catch it.
5. Add doc comments to every exported identifier in one package (starting with the name) and view them with `go doc`.
6. Find a place where you added a small dependency or abstraction you didn't need; remove it and inline the simple version.
7. Design a type whose zero value is immediately usable (no `Init()` needed); contrast with one that requires construction.

## Best Practices & Pitfalls
- **Run `gofmt` + `go vet` + `golangci-lint` in CI** and locally before committing. Treat lint failures like build failures.
- **Name for the reader at the call site.** `ctx`, `err`, `r`, `w` are idiomatic and instantly recognizable; don't rename them.
- **Doc-comment every exported symbol, starting with its name.** Missing/awkward docs are the most common lint finding.
- **Prefer the simplest thing that works.** Resist adding interfaces "for flexibility" until a second implementation actually exists (interfaces are cheap to add later because satisfaction is implicit).
- **Pitfall — premature abstraction / over-generic code.** YAGNI applies hard in Go; the language rewards directness.
- **Pitfall — fighting `gofmt` or inventing house styles.** There's one Go format; adopt it and move on.
- **Pitfall — `Get` prefixes, `Id`/`Url`/`Http` casing, plural package names.** These read as "not written by a Go dev" — small things that signal fluency.
- **Pitfall — ignoring `go vet`/linter output.** These tools catch real bugs (printf mismatches, copied locks, shadowed errors), not just style.

## Checklist
- [ ] I can name packages, types, variables, getters, and interfaces idiomatically.
- [ ] I design types whose zero value is useful.
- [ ] I write early-return, low-indentation code.
- [ ] I have `gofmt`, `go vet`, and `golangci-lint` running and clean.
- [ ] I doc-comment exported identifiers starting with their name.
- [ ] I resist premature abstraction and unnecessary dependencies.

## Resources
- Effective Go: https://go.dev/doc/effective_go
- Code Review Comments: https://go.dev/wiki/CodeReviewComments
- Google Go Style Guide: https://google.github.io/styleguide/go/
- golangci-lint: https://golangci-lint.run/ · staticcheck: https://staticcheck.dev/
