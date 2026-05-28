# 21 — Building a JSON REST API

## Goals
- Build a clean JSON REST API with decode → validate → respond flow.
- Write reusable JSON helpers and consistent error responses.
- Compose middleware for logging, recovery, and auth.
- Shut the server down gracefully.

## Concepts
- **REST shape** — map HTTP methods to actions on resources:
  | Method | Path           | Action            |
  |--------|----------------|-------------------|
  | GET    | `/tasks`       | list              |
  | GET    | `/tasks/{id}`  | get one           |
  | POST   | `/tasks`       | create (201)      |
  | PUT    | `/tasks/{id}`  | replace/update    |
  | DELETE | `/tasks/{id}`  | delete (204)      |
- **JSON request decoding** — read and validate the body:
  ```go
  var in CreateTaskRequest
  dec := json.NewDecoder(r.Body)
  dec.DisallowUnknownFields()                 // reject typos / extra fields
  if err := dec.Decode(&in); err != nil {
      writeError(w, http.StatusBadRequest, "invalid JSON")
      return
  }
  ```
  Limit body size with `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` to prevent abuse.
- **JSON response helpers** — write small helpers once and reuse them everywhere:
  ```go
  func writeJSON(w http.ResponseWriter, status int, v any) {
      w.Header().Set("Content-Type", "application/json")
      w.WriteHeader(status)
      json.NewEncoder(w).Encode(v)
  }
  func writeError(w http.ResponseWriter, status int, msg string) {
      writeJSON(w, status, map[string]string{"error": msg})
  }
  ```
- **Validation** — validate decoded input *before* acting on it (required fields, ranges, formats). Return `400` with a clear message. Keep validation in the handler or a request type's `Validate()` method; for complex rules, libraries like `go-playground/validator` exist (stdlib-first here).
- **Middleware** — a function that wraps a `Handler` to add cross-cutting behavior, returning a new `Handler`:
  ```go
  func Logging(next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          start := time.Now()
          next.ServeHTTP(w, r)
          slog.Info("request", "method", r.Method, "path", r.URL.Path,
              "dur", time.Since(start))
      })
  }
  ```
  - **Chaining** — wrap repeatedly: `handler = Logging(Recover(Auth(mux)))`. Order matters (outermost runs first on the way in).
  - Common middleware: request logging, panic **recovery** (turn a handler panic into a 500 instead of crashing the process), auth, CORS, request ID, rate limiting.
- **Panic recovery middleware** — wrap handlers so one panicking request returns `500` rather than taking down the server:
  ```go
  defer func() {
      if rec := recover(); rec != nil {
          slog.Error("panic", "err", rec)
          writeError(w, http.StatusInternalServerError, "internal error")
      }
  }()
  ```
- **Context & timeouts** — pass `r.Context()` into downstream calls; wrap handlers with `http.TimeoutHandler` or use per-request `context.WithTimeout` so slow work is bounded.
- **Graceful shutdown** — on `SIGINT`/`SIGTERM`, stop accepting new connections and let in-flight requests finish:
  ```go
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  defer stop()
  go srv.ListenAndServe()
  <-ctx.Done()                                   // wait for signal
  shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  srv.Shutdown(shutdownCtx)                       // drain in-flight requests
  ```

## Exercises
1. Build an in-memory `Task` REST API (a mutex-guarded `map[int]Task`) with all five endpoints from the table. Use 1.22 routing.
2. Write `writeJSON`/`writeError` helpers and use them everywhere; make every error response shaped like `{"error": "..."}`.
3. Add `DisallowUnknownFields()` and `http.MaxBytesReader`; send a malformed and an oversized body and confirm clean `400`s.
4. Validate `POST /tasks` (e.g., title required, non-empty) and return `400` with a helpful message; return `201` + the created task on success.
5. Write `Logging` and `Recover` middleware and chain them around the mux. Add a handler that panics and confirm it returns `500` and the server stays up.
6. Add a simple `Auth` middleware checking a static `Authorization: Bearer <token>` header; return `401` when missing/wrong.
7. Implement graceful shutdown with `signal.NotifyContext` + `srv.Shutdown`; start a slow request, send Ctrl-C, and confirm it finishes before exit.

## Best Practices & Pitfalls
- **Separate request/response DTOs from your domain types.** Decode into a `CreateTaskRequest`, validate, then build the domain `Task`. This decouples your API shape from internal models (and previews the layering in lesson 25).
- **Return consistent error JSON** with appropriate status codes; never leak internal error strings/stack traces to clients (log the detail, return a generic message).
- **Always set status before encoding** and use `writeJSON` to keep header/status/body ordering correct.
- **Put panic recovery at the top of the chain** so *any* handler panic becomes a 500 — one bad request must never crash the server.
- **Pitfall — encoding after `WriteHeader(200)` then hitting an error:** you can't change the status mid-write. Validate and decide the status *before* writing the body.
- **Pitfall — unbounded request bodies** invite memory-exhaustion DoS; always cap with `MaxBytesReader`.
- **Pitfall — forgetting graceful shutdown** drops in-flight requests on deploy. Wire `Shutdown` with a timeout.
- **Pitfall — middleware order bugs:** recovery and request-ID should be outermost; auth before business logic. Reason about the chain explicitly.

## Checklist
- [ ] I can build full CRUD endpoints with 1.22 routing and proper status codes.
- [ ] I have reusable `writeJSON`/`writeError` helpers and consistent error shapes.
- [ ] I decode + validate input and reject bad/oversized bodies.
- [ ] I can write and chain logging, recovery, and auth middleware in the right order.
- [ ] I separate request DTOs from domain types.
- [ ] I implement graceful shutdown.

## Resources
- Go 1.22 routing: https://go.dev/blog/routing-enhancements
- `http.Server.Shutdown`: https://pkg.go.dev/net/http#Server.Shutdown
- `signal.NotifyContext`: https://pkg.go.dev/os/signal#NotifyContext
- Article — How I write HTTP services in Go (Mat Ryer): https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/
