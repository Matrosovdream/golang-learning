# 12 — Errors & Error Handling

## Goals
- Understand Go's "errors are values" philosophy.
- Create, wrap, and inspect errors idiomatically.
- Use `errors.Is` / `errors.As` and the `%w` verb.
- Know when to use sentinel errors, custom error types, or panic.

## Concepts
- **`error` is just an interface:**
  ```go
  type error interface {
      Error() string
  }
  ```
  Anything with an `Error() string` method is an error. No exceptions, no try/catch — errors are ordinary values you return and check.
- **The universal pattern** — return `(result, error)` and check immediately:
  ```go
  f, err := os.Open("config.json")
  if err != nil {
      return fmt.Errorf("opening config: %w", err)
  }
  defer f.Close()
  ```
- **Creating errors:**
  - `errors.New("message")` — a simple static error.
  - `fmt.Errorf("got %d: %w", n, err)` — a formatted error that can **wrap** another error with `%w`.
- **Wrapping with `%w`** — `fmt.Errorf("...: %w", err)` creates a new error that *contains* `err`, preserving the chain. This lets callers add context ("loading user: opening db: connection refused") while still being able to inspect the original cause. Use `%w` to wrap, `%v` to merely format (no unwrapping).
- **Inspecting wrapped errors:**
  - **`errors.Is(err, target)`** — is `target` anywhere in the chain? Used with **sentinel errors**:
    ```go
    if errors.Is(err, sql.ErrNoRows) { /* not found */ }
    ```
  - **`errors.As(err, &target)`** — find an error of a specific *type* in the chain and extract it:
    ```go
    var perr *fs.PathError
    if errors.As(err, &perr) { fmt.Println(perr.Path) }
    ```
- **Sentinel errors** — predeclared error values for known conditions, compared with `errors.Is`:
  ```go
  var ErrNotFound = errors.New("not found")
  // ... later
  return ErrNotFound
  // caller: if errors.Is(err, ErrNotFound) { ... }
  ```
- **Custom error types** — a struct implementing `error`, for when callers need structured data:
  ```go
  type ValidationError struct {
      Field string
      Msg   string
  }
  func (e *ValidationError) Error() string {
      return fmt.Sprintf("%s: %s", e.Field, e.Msg)
  }
  // caller: var ve *ValidationError; if errors.As(err, &ve) { use ve.Field }
  ```
- **`panic` vs `error`** — `error` is for *expected* failure (file missing, bad input, network down). `panic` is for *impossible* states / programmer bugs (nil map write, index out of range, "this should never happen"). A library should almost never panic across its public API.
- **Don't ignore errors.** `_ = f()` silently swallows failures. Either handle, wrap-and-return, or log. The linter `errcheck` exists precisely to catch ignored errors.
- **Error message style** — lowercase, no trailing punctuation, no "failed to" noise; they get composed: `fmt.Errorf("open %s: %w", name, err)` reads well when nested.

## Exercises
1. Write `findUser(id int) (User, error)` that returns a sentinel `ErrNotFound` when missing; in the caller, branch with `errors.Is`.
2. Wrap that error with context using `fmt.Errorf("getUser %d: %w", id, err)` at a higher layer and print the full chain; confirm `errors.Is(err, ErrNotFound)` still matches through the wrap.
3. Define a `ValidationError` struct implementing `error`; return it from a validate function and recover the `Field` via `errors.As`.
4. Compare `%w` vs `%v`: wrap once with each, then test whether `errors.Is` can still find the cause. Explain the difference.
5. Write a function that `panic`s on a clearly impossible state, and discuss with Claude why a returned error would be wrong there.
6. Take a function that currently ignores an error (`_ = doThing()`) and rewrite it to handle the error properly.

## Best Practices & Pitfalls
- **Wrap with `%w` as the error travels up**, adding a short context phrase at each layer. This builds a readable trace without stack-trace machinery.
- **Compare errors with `errors.Is`/`errors.As`, never with `==` on wrapped errors** (`==` misses wrapped causes) and never by string-matching `err.Error()`.
- **Expose sentinel errors (`ErrNotFound`) from your packages** so callers can branch on them — this is part of your package's API.
- **Pitfall — over-wrapping:** don't add `"error: "` or restate the same context at every level. One concise phrase per layer.
- **Pitfall — losing the cause with `%v`:** if you format with `%v`, callers can't `errors.Is` the original. Use `%w` when you want the chain preserved.
- **Pitfall — panicking in libraries.** Convert recoverable problems to errors; reserve `panic` for genuinely unrecoverable bugs, and `recover` only at process boundaries (e.g., an HTTP middleware that turns a handler panic into a 500).
- **Handle each error exactly once** — either wrap-and-return *or* log, not both (double logging creates noise).

## Checklist
- [ ] I know `error` is an interface with one method.
- [ ] I use the `if err != nil` pattern and return early.
- [ ] I can wrap with `%w` and inspect with `errors.Is` / `errors.As`.
- [ ] I can define sentinel errors and custom error types.
- [ ] I can explain when to `panic` vs return an `error`.
- [ ] I never silently ignore errors.

## Resources
- Blog — Error handling and Go: https://go.dev/blog/error-handling-and-go
- Blog — Working with Errors in Go 1.13 (`%w`, Is/As): https://go.dev/blog/go1.13-errors
- `errors` package: https://pkg.go.dev/errors
- Effective Go — errors: https://go.dev/doc/effective_go#errors
