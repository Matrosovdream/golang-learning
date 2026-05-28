# 20 — HTTP Server Fundamentals

## Goals
- Stand up an HTTP server with only the standard library.
- Understand handlers, `ServeMux`, and the request/response lifecycle.
- Use Go 1.22+ method- and path-pattern routing.
- Read requests (path values, query, body) and write proper responses.

## Concepts
- **The smallest server:**
  ```go
  func main() {
      mux := http.NewServeMux()
      mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
          w.Write([]byte("ok"))
      })
      log.Fatal(http.ListenAndServe(":8080", mux))
  }
  ```
- **`http.Handler` and `http.HandlerFunc`:**
  - `Handler` is an interface with one method: `ServeHTTP(w http.ResponseWriter, r *http.Request)`.
  - `HandlerFunc` adapts a plain function `func(w, r)` into a `Handler`. This is why `HandleFunc` accepts a function.
  - Everything in `net/http` — routers, middleware, the server — speaks `Handler`.
- **`http.ResponseWriter`** — you *write* the response through it:
  - `w.Header().Set("Content-Type", "application/json")` — **set headers before** writing the body or status.
  - `w.WriteHeader(http.StatusCreated)` — set the status code (optional; defaults to 200 on first `Write`).
  - `w.Write(b)` — write body bytes. After the first `Write`/`WriteHeader`, headers are locked.
- **`*http.Request`** — the incoming request:
  - `r.Method`, `r.URL.Path`, `r.URL.Query().Get("q")` (query params).
  - `r.PathValue("id")` — path wildcards (Go 1.22+, see routing).
  - `r.Body` — an `io.ReadCloser`; decode JSON from it, then it's consumed. The server closes it, but closing yourself is fine.
  - `r.Context()` — the request's context, cancelled when the client disconnects (use for DB calls, timeouts).
- **`ServeMux` routing (Go 1.22+)** — the stdlib router gained **method + path pattern** matching, so you often don't need a third-party router:
  ```go
  mux.HandleFunc("GET /users/{id}", getUser)     // method + wildcard
  mux.HandleFunc("POST /users", createUser)
  mux.HandleFunc("GET /files/{path...}", serve)   // {path...} matches the rest
  id := r.PathValue("id")                          // read the wildcard
  ```
  - Patterns can require a method (`GET `, `POST `, …), match a host, and capture `{name}` / `{name...}` wildcards.
  - More specific patterns win over less specific ones.
- **The request lifecycle** — the server accepts a connection, parses the request, the mux matches a pattern and calls the handler **in its own goroutine** (so handlers run concurrently — mind shared state), the handler writes to `w`, and the response is flushed when the handler returns.
- **Status codes** — use the `http.Status*` constants (`http.StatusOK`, `StatusCreated`, `StatusBadRequest`, `StatusNotFound`, `StatusInternalServerError`). `http.Error(w, msg, code)` is a shortcut for an error response.
- **`http.Server` for real use** — `http.ListenAndServe` is fine for demos, but for production create an `&http.Server{Addr, Handler, ReadTimeout, WriteTimeout, IdleTimeout}` so slow clients can't hang connections (timeouts come up again with graceful shutdown in lesson 21).
- **Serving static files** — `http.FileServer(http.Dir("public"))` and `http.ServeFile` (mentioned for completeness; APIs rarely need them).

## Exercises
1. Build a server with `GET /health` returning `"ok"` and run it; hit it with `curl localhost:8080/health`.
2. Add `GET /users/{id}` that reads `r.PathValue("id")` and echoes it. Add `GET /search?q=...` reading the query.
3. Add `POST /echo` that reads the raw request body with `io.ReadAll(r.Body)` and writes it back.
4. Return different status codes: a `404` via `http.Error` for an unknown id, `201` for a created resource.
5. Set `Content-Type: application/json` and write a hand-built JSON string; verify the header in `curl -i`.
6. Add a deliberate slow handler (`time.Sleep`) and observe (by hitting two endpoints at once) that handlers run concurrently.
7. Replace `ListenAndServe` with an explicit `&http.Server{...}` that sets `ReadTimeout`/`WriteTimeout`.

## Best Practices & Pitfalls
- **Use the stdlib `ServeMux` (1.22+) before reaching for a framework.** It now covers method+path routing for most APIs; add `chi`/`gin` only when you need more.
- **Set headers before `WriteHeader`/`Write`.** Once you've written the status or body, header changes are silently ignored.
- **Call `WriteHeader` exactly once.** A second call logs a "superfluous WriteHeader" warning.
- **Always read and close `r.Body`** if you intend to reuse the connection; decode it once (it's a stream, not re-readable).
- **Handlers run concurrently — guard shared state.** Each request is a goroutine; protect shared maps/counters with a mutex (lesson 15) or avoid shared mutability.
- **Pitfall — no server timeouts.** `http.ListenAndServe` has none, so a slow/malicious client can tie up resources. Set `ReadTimeout`/`WriteTimeout`/`IdleTimeout` on an `http.Server`.
- **Pitfall — ignoring `r.Context()`.** Pass it into DB/HTTP calls so work stops when the client disconnects or the request times out.
- **Return after `http.Error`.** Writing an error and then continuing to write more produces a corrupt response.

## Checklist
- [ ] I can start a server with `http.NewServeMux` and `ListenAndServe`.
- [ ] I understand `Handler` vs `HandlerFunc` and `ServeHTTP`.
- [ ] I can route by method + path and read `r.PathValue` and query params.
- [ ] I can read the request body and write a status + headers + body in the right order.
- [ ] I know handlers run concurrently and why timeouts matter.
- [ ] I can use `r.Context()` for cancellation.

## Resources
- `net/http` package: https://pkg.go.dev/net/http
- Tutorial — Web service with Go: https://go.dev/doc/tutorial/web-service-gin (note: stdlib equivalents)
- Go 1.22 routing enhancements: https://go.dev/blog/routing-enhancements
- Blog — Writing Web Applications: https://go.dev/doc/articles/wiki/
