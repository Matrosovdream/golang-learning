# Config, Logging & Observability Cheatsheet

**Lessons:** [23 — Config, Logging & Observability](../23-config-logging.md) · [39 — Distributed Tracing](../39-observability-tracing.md)
**Examples:** [39](../examples/39-observability-tracing/)
**Covers:** env config, `log/slog`, request logging, health checks, metrics, OpenTelemetry spans
**Legend:** `[*]` = real API that the lessons have not covered yet

## CONFIG: the 12-factor shape

```text
type Config struct { Port int; DatabaseURL string; LogLevel string }
func Load() (*Config, error)      one function, called once in main
os.Getenv("PORT")            "" when unset
os.LookupEnv("PORT")         -> (value, ok): distinguishes unset from empty
strconv.Atoi(os.Getenv(...))     parse, and FAIL if it's malformed
fail fast at startup         a missing DATABASE_URL should stop the process
defaults for local dev       sensible values, never for secrets
never commit secrets         env vars or a secret manager, not the repo
flag for one-off overrides   env wins in containers; flags win on a laptop
caarlos0/env             [*] struct-tag based env parsing
kelseyhightower/envconfig [*] the older equivalent
(config is read ONCE and passed down — never call os.Getenv deep in a handler)
```

## slog: the API

```text
slog.Info("user created", "user_id", id, "tenant", t)   alternating key/value
slog.Debug / slog.Info / slog.Warn / slog.Error         the four levels
slog.String("k", v) / Int / Int64 / Bool / Float64 / Duration / Time / Any
                             typed attrs: faster, and a typo won't compile
slog.New(slog.NewJSONHandler(os.Stdout, opts))     production: JSON to stdout
slog.New(slog.NewTextHandler(os.Stderr, opts))     local: key=value
slog.SetDefault(logger)      the package-level slog.Info now uses it
logger.With("request_id", id)     a child logger that carries fixed attrs
logger.InfoContext(ctx, "msg", ...)   pass ctx so handlers can enrich
slog.Group("user", "id", 1, "role", "admin")  [*] nested attributes
&slog.HandlerOptions{
  Level: slog.LevelDebug,        the minimum level to emit
  AddSource: true,           [*] file:line of the call site
  ReplaceAttr: func(...)     [*] rewrite/redact attributes centrally
}
slog.LevelVar             [*] change the level at runtime, atomically
(log to STDOUT and let the platform collect it — never to a file you rotate)
```

## LOGGING RULES

```text
one event, one line          structured, machine-parsable
message is a constant        "user created", not "user 42 created"
the variables are ATTRS      so you can filter and aggregate on them
levels mean something        Error = a human must look; Warn = degraded
log at the boundary          not in every layer of the call stack
log OR return the error      never both — that's how one failure becomes five lines
never log secrets            passwords, tokens, full request bodies, card numbers
redact centrally             ReplaceAttr, so nobody has to remember
request_id on every line     the thread that ties a request together
(if a log line can't be filtered on in production, it is decoration)
```

## REQUEST LOGGING MIDDLEWARE

```text
generate a request id        uuid.NewString() or the incoming X-Request-ID
put it in the context        ctx = context.WithValue(ctx, reqIDKey{}, id)
echo it back                 w.Header().Set("X-Request-ID", id)
wrap the ResponseWriter      to capture the status code and bytes written
log once, after next()       method, path, status, duration, bytes, request_id
skip /healthz                or your logs are 90% health checks
propagate it downstream      send X-Request-ID on every outbound call
```

## HEALTH CHECKS

```text
GET /healthz                 LIVENESS: is the process alive? return 200, no I/O
GET /readyz                  READINESS: can it serve? check the DB, return 503 if not
db.PingContext(ctx)          with a short timeout — 1s, not the default
flip /readyz to 503 first    then start draining, so the LB stops sending traffic
/metrics                     Prometheus scrape endpoint
never auth /healthz          the platform can't log in
liveness must not check deps a slow DB should not restart your process
```

## METRICS (Prometheus shape) [*]

```text
Counter                      only goes up: requests_total, errors_total
Gauge                        goes up and down: queue_depth, connections_in_use
Histogram                    distribution: request_duration_seconds
Summary                      client-side quantiles (prefer Histogram)
labels                       method, route, status — LOW cardinality only
never label with user_id     one time series per user will kill the store
prometheus/client_golang     the library; promhttp.Handler() for /metrics
the four golden signals      latency, traffic, errors, saturation
RED for services             Rate, Errors, Duration
USE for resources            Utilization, Saturation, Errors
```

## TRACING: the model

```text
trace                        one request's whole journey, across services
span                         one unit of work inside it, with a start and end
trace id                     the same for every span in the trace
span id / parent span id     the tree structure
attributes                   key/value on a span (http.method, db.statement)
events                       a timestamped log line attached to a span
context propagation          the trace id travels in HTTP headers (traceparent)
W3C Trace Context            the standard header format
sampling                     record a percentage; head-based is the common choice
(logs tell you WHAT happened, metrics tell you HOW OFTEN, traces tell you WHERE)
```

## OpenTelemetry API [*]

```text
otel.Tracer("service-name")            get a tracer
ctx, span := tracer.Start(ctx, "GetUser")   start a span; ctx now carries it
defer span.End()                       always, or the span never closes
span.SetAttributes(attribute.String("user.id", id))
span.RecordError(err)                  attach the error
span.SetStatus(codes.Error, "failed")  mark the span as failed
trace.SpanFromContext(ctx)             the current span, anywhere down the stack
otelhttp.NewHandler(h, "server")       auto-instrument an HTTP server
otelhttp.NewTransport(rt)              auto-instrument an HTTP client
otel.SetTracerProvider(tp)             wire it up once in main
tp.Shutdown(ctx)                       flush buffered spans before exit
propagation.TraceContext{}             inject/extract the traceparent header
(pass ctx everywhere or the trace breaks at the first function that drops it)
```

## CORRELATING THE THREE PILLARS

```text
put trace_id in every log line     jump from a log to the trace
exemplars on histograms        [*] jump from a metric spike to a trace
same request_id across services    one id, propagated
structured logs + trace id     the cheapest observability that actually works
(start with logs + request id; add metrics; add tracing when you have >2 services)
```

## TRAPS & MEMORIZE

```text
os.Getenv deep in the code    untestable and undiscoverable; load config once
config read per request       silently slow, and impossible to validate
logging to a file             containers are ephemeral; log to stdout
fmt.Println for logs          no level, no fields, no timestamp
log.Fatal in a library        it calls os.Exit — never do this outside main
logging and returning err     the same failure appears at every layer
high-cardinality labels       user_id/request_id in metrics blows up the TSDB
liveness that checks the DB   a DB blip restarts every replica at once
readiness that checks nothing traffic arrives before the pool is warm
span.End() without defer      an early return leaks the span
dropping ctx                  the trace ends there, and so does cancellation
sampling at 100%              fine in dev, expensive and noisy in production
```
