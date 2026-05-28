# 23 — Config, Logging & Observability

## Goals
- Load configuration from the environment in a 12-factor way.
- Produce structured, leveled logs with `log/slog`.
- Add request logging, request IDs, and health checks.
- Understand the basics of metrics and tracing for a service.

## Concepts
- **Config from the environment (12-factor).** Read config from env vars (with sane defaults), not hardcoded constants or committed files. Parse it **once at startup** into a typed `Config` struct and pass it down — don't call `os.Getenv` scattered across the codebase.
  ```go
  type Config struct {
      Addr        string
      DatabaseURL string
      LogLevel    slog.Level
  }
  func Load() (Config, error) {
      cfg := Config{
          Addr:        getenv("ADDR", ":8080"),
          DatabaseURL: os.Getenv("DATABASE_URL"),
      }
      if cfg.DatabaseURL == "" {
          return Config{}, errors.New("DATABASE_URL is required")
      }
      return cfg, nil
  }
  ```
  - **Fail fast**: if a required value is missing/invalid, return an error from `Load()` and exit at startup — never boot a half-configured server.
  - Libraries like `kelseyhightower/envconfig` or `caarlos0/env` map env vars to structs via tags; stdlib `os.Getenv` + a helper is enough to start.
  - **Secrets** (DB passwords, API keys) come from env/secret managers, never from committed files. Keep them out of logs.
- **Structured logging with `slog`** (lesson 19 recap, now applied):
  - One logger configured at startup; `JSONHandler` in prod, `TextHandler` locally; level from config.
  - Log **key/value attributes**, not interpolated strings: `slog.Info("user created", "user_id", id)` — searchable and machine-parseable.
  - **Levels:** `Debug` (dev detail), `Info` (normal events), `Warn` (recoverable oddities), `Error` (failures needing attention). Set the threshold via config.
  - **Never log secrets or full request bodies** with credentials.
- **Request logging middleware** — log one line per request with method, path, status, duration, and a request ID (build on lesson 21's middleware). Capture the status code by wrapping `http.ResponseWriter`.
- **Request IDs / correlation** — generate a unique ID per request (e.g., `uuid`), put it in the request `context` and on a response header (`X-Request-ID`), and include it in every log line for that request so you can trace one request across many log entries. Pass a request-scoped logger via `context`.
- **Health & readiness checks** — expose endpoints for orchestrators (Kubernetes, load balancers):
  - **Liveness** (`/healthz`) — "the process is up" — cheap, no dependencies.
  - **Readiness** (`/readyz`) — "ready to serve traffic" — checks dependencies like `db.PingContext(ctx)`. Return `503` when a dependency is down so traffic is withheld.
- **Metrics (awareness)** — the standard is **Prometheus** (`prometheus/client_golang`): expose `/metrics`, track counters (requests by status), histograms (latency), and gauges (in-flight requests). You don't need it for learning, but know the shape: instrument middleware once, scrape with Prometheus, visualize in Grafana.
- **Tracing (awareness)** — **OpenTelemetry** propagates a trace across services via context, so you can see a request's full path through a distributed system. Overkill for one service; know it exists for microservices.
- **The "three pillars" of observability** — **logs** (what happened), **metrics** (how much/how fast, aggregated), **traces** (the path of one request). A production service typically has all three; start with structured logs + health checks + a couple of metrics.

## Exercises
1. Write a `Config` struct + `Load()` that reads `ADDR`, `DATABASE_URL`, and `LOG_LEVEL` from env with defaults, and returns an error if `DATABASE_URL` is missing. Wire it into your Part 6 server's `main`.
2. Set up a `slog` logger whose handler (JSON vs text) and level come from config; log an `Info` and an `Error` with attributes.
3. Write request-logging middleware that wraps `ResponseWriter` to capture the status code and logs method/path/status/duration on the way out.
4. Generate a request ID per request, store it in `r.Context()`, set it on `X-Request-ID`, and include it in the request log line.
5. Add `/healthz` (always 200) and `/readyz` (pings the DB, returns 503 if down). Stop the DB and confirm `/readyz` flips to 503 while `/healthz` stays 200.
6. (Stretch) Add `prometheus/client_golang`, expose `/metrics`, and count requests by status code via middleware. Scrape it with `curl`.

## Best Practices & Pitfalls
- **Parse config once at startup into a typed struct; pass it down explicitly.** Avoid `os.Getenv` calls deep in business logic — they hide dependencies and can't be tested.
- **Fail fast on bad/missing config.** A clear startup error beats a mysterious runtime failure later.
- **Log structured key/values, not formatted prose.** `slog.Info("done", "ms", 12)` beats `log.Printf("done in 12ms")` for searching and alerting.
- **Put a request ID on every log line** so one request's logs can be correlated; thread a request-scoped logger through `context`.
- **Separate liveness from readiness.** Liveness failing restarts the pod; readiness failing just withholds traffic — conflating them causes restart loops when a dependency blips.
- **Pitfall — logging secrets/PII.** Scrub credentials, tokens, and full bodies. A leaked token in logs is a breach.
- **Pitfall — over-instrumenting too early.** For one service, structured logs + health checks + a few metrics are plenty; add tracing when you actually have multiple services.
- **Pitfall — using `context.WithValue` for config.** Config is a startup concern passed explicitly; context values are for per-request metadata (request ID, auth), not app configuration.

## Checklist
- [ ] I load config from env into a typed struct and fail fast on missing required values.
- [ ] I have one `slog` logger configured by level/format from config.
- [ ] I log structured key/value attributes, never secrets.
- [ ] I have request-logging middleware that captures status + duration.
- [ ] I attach a request ID to context, response header, and logs.
- [ ] I expose separate liveness and readiness endpoints.
- [ ] I can explain logs vs metrics vs traces.

## Resources
- The Twelve-Factor App — Config: https://12factor.net/config
- `log/slog` package & guide: https://pkg.go.dev/log/slog · https://go.dev/blog/slog
- Prometheus Go client: https://github.com/prometheus/client_golang
- OpenTelemetry Go: https://opentelemetry.io/docs/languages/go/
