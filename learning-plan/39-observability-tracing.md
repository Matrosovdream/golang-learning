# 39 — Observability: Distributed Tracing

> Part 9, Track C: [38 Caching](38-caching-patterns.md) → **39 Observability: Tracing** → [40 Testing Architecture](40-testing-architecture.md) → [41 API Design & Evolution](41-api-design-evolution.md).
> [27 — gRPC & Microservices](27-grpc-microservices.md) gave you structured **logs** and Prometheus **metrics**. This lesson adds the third pillar — **distributed traces** — and, crucially, ties all three together with **OpenTelemetry** so you can follow one request across every service.

## Goals
- Understand the **three pillars** (logs, metrics, traces) and what each answers.
- Instrument a Go service with **OpenTelemetry**: tracer provider, exporter, spans, attributes.
- **Propagate** trace context across HTTP/gRPC hops so a request is one connected trace.
- **Correlate** logs ↔ metrics ↔ traces via a shared trace id, and control cost with **sampling**.

## Concepts

### The three pillars — and what each is *for*
- **Logs** — discrete, timestamped events with context. Answer: *what exactly happened at this moment?* (structured, `slog`, [27](27-grpc-microservices.md)).
- **Metrics** — cheap numeric aggregates over time (counters, gauges, histograms). Answer: *how much / how many / how fast, in aggregate?* Great for dashboards and alerts; can't explain a single request ([27](27-grpc-microservices.md)).
- **Traces** — the causal path of **one** request across services, as a tree of timed spans. Answer: *where did this specific slow/failed request spend its time, and which hop broke?*
You need all three: metrics tell you *something* is slow, a trace tells you *where*, logs tell you *why*. Two useful lenses: **RED** (Rate, Errors, Duration — per endpoint) and **USE** (Utilisation, Saturation, Errors — per resource).

### Anatomy of a trace
- A **trace** = a tree of **spans** sharing one **trace id**.
- A **span** = one unit of work (an HTTP handler, a DB query, an RPC) with a start/end time, a **span id**, a **parent span id**, a status (ok/error), and **attributes** (key/values) and **events**.
- The parent/child links form the tree; **span links** connect related traces (e.g. a batch job to the messages that triggered it).
```
trace 4bf92... 
└─ span: POST /checkout        (gateway)          120ms
   ├─ span: rpc catalog.Get    (→ catalog svc)     15ms
   ├─ span: rpc orders.Create  (→ orders svc)      80ms   ← the slow hop
   │  └─ span: db INSERT orders                    72ms   ← ...here
   └─ span: publish OrderPlaced                     5ms
```

### OpenTelemetry — the vendor-neutral standard
**OpenTelemetry (OTel)** is the CNCF standard API + SDK for all three signals; you instrument once and export to any backend (Jaeger, Tempo, Datadog, …) via the **OTLP** protocol, usually through an **OTel Collector**. Setup in Go — a `TracerProvider` with an exporter:
```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/resource"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func initTracer(ctx context.Context) (*trace.TracerProvider, error) {
    exp, err := otlptracegrpc.New(ctx) // ships spans to the collector (OTLP/gRPC)
    if err != nil { return nil, err }
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exp),                              // batch spans → fewer exports
        trace.WithResource(resource.NewSchemaless(
            semconv.ServiceName("orders"))),                 // who am I
        trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(0.1))), // sample 10%
    )
    otel.SetTracerProvider(tp)
    return tp, nil // defer tp.Shutdown(ctx) in main to flush on exit
}
```

### Creating spans
Auto-instrumentation covers the edges; you add **manual spans** around meaningful business work. A span always threads through `context` — the first-arg `ctx` you've carried since [15](15-sync-context.md) is how parent/child gets wired:
```go
var tracer = otel.Tracer("orders")

func (s Service) Checkout(ctx context.Context, cart Cart) error {
    ctx, span := tracer.Start(ctx, "Checkout")   // child of whatever span is in ctx
    defer span.End()
    span.SetAttributes(
        attribute.String("customer.id", cart.CustomerID),
        attribute.Int("cart.items", len(cart.Items)),   // LOW cardinality, no PII
    )
    if err := s.charge(ctx, cart); err != nil {          // pass ctx → nested span links up
        span.RecordError(err)
        span.SetStatus(codes.Error, "charge failed")
        return err
    }
    return nil
}
```

### Propagation — the "distributed" in distributed tracing
A trace stays connected across process boundaries only if the trace context travels with the request. OTel **propagators** inject the context into outgoing headers and extract it on the way in — the **W3C `traceparent`** header for HTTP, gRPC **metadata** for gRPC ([27](27-grpc-microservices.md)'s request-id, done properly). The instrumentation libraries do this for you:
```go
otel.SetTextMapPropagator(propagation.TraceContext{})     // W3C traceparent
// inbound: wrap the handler so it extracts context + starts a server span
handler := otelhttp.NewHandler(mux, "http.server")
// outbound: a transport that injects context into requests
client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
// gRPC: grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler())) and the client handler
```
With propagation set up, the gateway's span and the orders service's span share one trace id — you see the whole request as one tree.

### Correlate the pillars: put the trace id in your logs
The superpower is jumping from a log line to its trace and back. Pull the trace id from the span context and attach it to every structured log line ([27](27-grpc-microservices.md)):
```go
func logWithTrace(ctx context.Context, log *slog.Logger) *slog.Logger {
    sc := trace.SpanContextFromContext(ctx)
    if sc.HasTraceID() {
        return log.With("trace_id", sc.TraceID().String()) // now logs ↔ traces are linked
    }
    return log
}
```
Now one `trace_id` connects the trace tree, every service's logs for that request, and (via metric **exemplars**) the latency histogram bucket — one id to pivot across all three pillars.

### Sampling — you can't afford 100%
Tracing every request is expensive (storage, throughput). **Sampling** keeps a representative subset:
- **Head sampling** — decide at the start (e.g. keep 10%, `TraceIDRatioBased`). Simple, cheap, but may miss the rare error you care about. `ParentBased` ensures a whole trace is sampled together.
- **Tail sampling** — decide *after* the trace completes (in the Collector): keep all errors and slow traces, sample the boring fast ones. More useful, needs the Collector to buffer.
Keep attributes **low-cardinality** (status, route template `/orders/{id}` not `/orders/123`) and **never put secrets/PII in spans** — spans are exported to third-party backends.

## Exercises
1. Write one sentence each on what logs, metrics, and traces uniquely answer, and map a real "checkout is slow" investigation to the order you'd use them (metric → trace → log).
2. Stand up a local trace backend (Jaeger all-in-one via Docker) and an OTel `TracerProvider` exporting OTLP to it; export one manual span and view it in the Jaeger UI.
3. Add `otelhttp.NewHandler` to a two-service app (gateway → service). Fire one request and confirm **both** spans appear under **one** trace id in the UI.
4. Add a manual child span in the downstream service's business logic with two low-cardinality attributes; make it error and confirm `RecordError`/`SetStatus` show as an error span.
5. Wire `trace_id` into your `slog` lines; grep the id across both services' logs and cross-check it against the trace tree.
6. Switch the sampler from always-on to `ParentBased(TraceIDRatioBased(0.1))`; generate traffic and confirm ~10% of traces are kept and each kept trace is whole (parent + children together).
7. Find and fix a **high-cardinality** attribute (e.g. a raw URL with an id, or an email) — replace it with a route template / id-free value and say why it matters for cost and privacy.

## Best Practices & Pitfalls
- **Thread `context` everywhere.** Trace parent/child is carried in `ctx`; a function that drops it (or takes no `ctx`) breaks the trace. Pass `ctx` as the first arg, always.
- **Instrument the edges automatically, the business logic manually.** `otelhttp`/`otelgrpc` at the boundaries; hand-add spans around the operations you'll actually investigate.
- **One trace id in every log line.** Correlation across pillars is the whole payoff. Inject the trace id into `slog` at the request boundary.
- **Sample in production** (and `ParentBased` so traces stay whole). 100% tracing is a cost/throughput problem; tail-sampling errors+slow traces gives the best signal.
- **Keep span attributes low-cardinality and PII-free.** Route templates, status codes, sizes — not raw ids, emails, tokens, or full URLs. Spans leave your trust boundary.
- **Flush on shutdown.** `defer tp.Shutdown(ctx)`; a batched exporter drops buffered spans if the process exits without flushing.
- **Pitfall — a broken trace tree.** A missing propagator, or a background goroutine started from `context.Background()` instead of the request ctx, orphans spans into separate traces. Derive child contexts from the request.
- **Pitfall — over-spanning.** A span per trivial function floods the backend and hides signal. Span meaningful units of work.
- **Pitfall — metrics with unbounded labels** (from [27](27-grpc-microservices.md)) — same cardinality rule applies to span attributes and log fields.

## Checklist
- [ ] I can explain the three pillars and what question each answers (and RED/USE).
- [ ] I can set up an OTel `TracerProvider` + OTLP exporter and create manual spans with attributes.
- [ ] I can propagate context across HTTP and gRPC so one request is one trace.
- [ ] I can inject `trace_id` into structured logs and pivot between logs and traces.
- [ ] I can configure head vs tail sampling and explain the trade-off.
- [ ] I keep attributes low-cardinality and free of PII/secrets, and flush on shutdown.

## Resources
- OpenTelemetry Go — getting started & docs: https://opentelemetry.io/docs/languages/go/
- Instrumentation libs: `otelhttp` https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp · `otelgrpc`
- W3C Trace Context (the `traceparent` header): https://www.w3.org/TR/trace-context/
- Google SRE Book — Monitoring Distributed Systems (the four golden signals): https://sre.google/sre-book/monitoring-distributed-systems/
- Jaeger (local tracing backend): https://www.jaegertracing.io/docs/getting-started/
- Builds on [27 — logging & metrics](27-grpc-microservices.md). Next: [40 — Testing Architecture](40-testing-architecture.md).
