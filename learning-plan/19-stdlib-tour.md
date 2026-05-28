# 19 — Standard Library Tour for Backend

## Goals
- Know the standard-library packages you'll use constantly in backend work.
- Understand the `io.Reader`/`io.Writer` abstraction that ties I/O together.
- Marshal and unmarshal JSON with struct tags.
- Log with structured logging (`log/slog`) and read flags/env.

## Concepts
- **`io.Reader` / `io.Writer`** — the two interfaces the whole I/O ecosystem is built on:
  ```go
  type Reader interface { Read(p []byte) (n int, err error) }
  type Writer interface { Write(p []byte) (n int, err error) }
  ```
  Files, network connections, HTTP bodies, buffers, and stdout/stdin all implement them, so the same code works across all sources/sinks. Helpers: `io.Copy(dst, src)`, `io.ReadAll(r)`, `bytes.Buffer`, `strings.NewReader`.
- **`os`** — process and filesystem: `os.Args` (CLI args), `os.Getenv`/`os.LookupEnv` (env vars), `os.Open`/`os.Create`/`os.ReadFile`/`os.WriteFile`, `os.Stdin`/`Stdout`/`Stderr`, `os.Exit(code)`.
- **`encoding/json`** — the backbone of REST APIs:
  ```go
  type User struct {
      ID    int    `json:"id"`
      Name  string `json:"name"`
      Email string `json:"email,omitempty"`
  }
  b, err := json.Marshal(u)          // struct → JSON bytes
  err = json.Unmarshal(b, &u)        // JSON bytes → struct (pass a pointer!)
  // streaming (preferred for HTTP):
  json.NewEncoder(w).Encode(u)       // write JSON to an io.Writer
  json.NewDecoder(r).Decode(&u)      // read JSON from an io.Reader
  ```
  - Struct tags control field names; `omitempty` drops zero-valued fields; `json:"-"` hides a field.
  - Only **exported** fields are marshaled.
  - Unknown JSON fields are ignored by default; use `Decoder.DisallowUnknownFields()` to reject them.
- **`time`** — `time.Now()`, `time.Duration` (`2*time.Second`), `time.Since(start)`, `t.Format(layout)` with Go's reference-time layout `2006-01-02 15:04:05`, `time.Parse`, `time.Sleep`. Use `time.Time` for timestamps and store/transmit in UTC.
- **`log/slog`** — structured, leveled logging (the modern default, Go 1.21+):
  ```go
  logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
  logger.Info("request handled", "method", "GET", "path", "/users", "ms", 12)
  ```
  Key/value attributes make logs queryable; choose `JSONHandler` for production, `TextHandler` for local dev. Prefer `slog` over the old `log` package for services.
- **`net/http` client** — make outbound requests:
  ```go
  resp, err := http.Get("https://api.example.com/x")
  defer resp.Body.Close()           // always close the body
  body, err := io.ReadAll(resp.Body)
  ```
  For real use, create a `*http.Client` with a timeout rather than the default (which has none).
- **`flag`** — simple CLI flags: `port := flag.Int("port", 8080, "listen port")`, then `flag.Parse()` and use `*port`. Good enough for most tools (Cobra comes later if you need subcommands).
- **`bufio`** — buffered I/O wrappers (`bufio.NewScanner` to read input line by line, `bufio.NewWriter` to batch writes).
- **Other backend staples to know exist:** `context` (lesson 15), `database/sql` (lesson 22), `sort`, `strconv` (lesson 08), `regexp`, `crypto/*`, `encoding/base64`.

## Exercises
1. Use `io.Copy` to stream `strings.NewReader("hello")` to `os.Stdout`; then read it all with `io.ReadAll`.
2. Define a `User` struct with `json` tags (one `omitempty`, one `json:"-"`). `Marshal` it, print the JSON, then `Unmarshal` back into a new struct.
3. Use `json.NewEncoder(os.Stdout).Encode(u)` and compare to `json.Marshal` — note the streaming style you'll use in HTTP handlers.
4. Decode a JSON string with an unknown field both with and without `DisallowUnknownFields()`; observe the difference.
5. Format the current time as `2006-01-02 15:04:05` and parse a date string back into a `time.Time`.
6. Create a `slog` JSON logger and log an `Info` and an `Error` with structured attributes.
7. Make an `http.Get` to a public JSON API, read and close the body, and unmarshal a couple of fields. Then switch to an `http.Client{Timeout: 5*time.Second}`.
8. Write a tiny program that reads a `-name` flag and an env var with `os.LookupEnv`.

## Best Practices & Pitfalls
- **Program to `io.Reader`/`io.Writer`, not concrete types.** Accepting an `io.Reader` makes functions testable with `strings.NewReader` and reusable across files/network/buffers.
- **Always `defer resp.Body.Close()`** after a successful HTTP request, and read the body to completion — otherwise you leak connections.
- **Never use `http.Get`/`http.DefaultClient` in production without a timeout.** The default client waits forever; build an `http.Client{Timeout: ...}`.
- **Pass a pointer to `json.Unmarshal`/`Decode`** (`&u`); passing a value silently fills nothing.
- **Pitfall — unexported fields don't marshal.** If JSON output is missing a field, check that it's capitalized and tagged.
- **Pitfall — Go's time layout** uses the specific reference date `Mon Jan 2 15:04:05 MST 2006` (i.e., 01/02 03:04:05PM '06 -0700), not `YYYY-MM-DD`. Memorize `2006-01-02 15:04:05`.
- **Prefer `slog` over `log`/`fmt.Println` in services** — structured logs are searchable and level-aware.
- **Store/transmit times in UTC**; convert to local only for display.

## Checklist
- [ ] I understand `io.Reader`/`io.Writer` and can use `io.Copy`/`io.ReadAll`.
- [ ] I can marshal/unmarshal JSON with struct tags and the streaming Encoder/Decoder.
- [ ] I can format and parse time with Go's reference layout.
- [ ] I can set up a `slog` structured logger.
- [ ] I can make an HTTP request with a client timeout and close the body.
- [ ] I can read flags and env vars.

## Resources
- `io` package: https://pkg.go.dev/io
- Blog — JSON and Go: https://go.dev/blog/json
- `log/slog` package: https://pkg.go.dev/log/slog
- `time` package (layout reference): https://pkg.go.dev/time#pkg-constants
- `net/http` package: https://pkg.go.dev/net/http
