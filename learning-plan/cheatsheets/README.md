# Cheatsheets

Dense, printable reference sheets — one per group of related lessons. They are for
**repetition**: everything a lesson taught, compressed to the API surface, the shapes,
and the traps, with no prose to wade through.

Each sheet is a `.md` file here plus a rendered `.pdf` in [pdf/](pdf/).
`[*]` marks a real Go API that the lessons have not covered yet — extra surface worth
knowing, kept visually separate from what you have already studied.

## The sheets

| Sheet | Lessons | What it holds |
|---|---|---|
| [01-03 Toolchain & Basics](01-03-toolchain.md) | 1–3 | `go` command, modules, env vars, program skeleton, declarations |
| [04-05 Types & Control Flow](04-05-types-control-flow.md) | 4–5 | every basic type, zero values, conversions, `iota`, if/for/switch/defer |
| [06 Functions](06-functions.md) | 6 | signatures, multiple returns, variadic, closures, panic/recover |
| [07-08 Slices, Maps & Strings](07-08-slices-maps-strings.md) | 7–8 | slice internals, `slices`/`maps`, `strings`/`strconv`, `fmt` verbs |
| [09-12 Types, Interfaces & Errors](09-12-types-interfaces-errors.md) | 9–12 | structs, method sets, embedding, type switches, the `errors` API |
| [13-15 Concurrency](13-15-concurrency.md) | 13–15 | goroutines, channels, `sync`, `sync/atomic`, `context`, `errgroup`, ~45 patterns |
| [16-17 Modules & Generics](16-17-modules-generics.md) | 16–17 | package design, versioning, type parameters, constraints |
| [18-40-49 Testing](18-40-49-testing.md) | 18, 40, 49 | the `testing` API, every kind of test, fakes, flags |
| [19 Standard Library](19-stdlib.md) | 19 | `io`, `os`, `time`, `encoding/json`, `log/slog`, `flag` |
| [20-21 HTTP & REST](20-21-http-rest.md) | 20–21 | `net/http` types, routing patterns, status codes, middleware |
| [22-59 Database](22-59-database.md) | 22, 59 | `database/sql`, pool knobs, transactions, pagination, upsert |
| [23-39 Config, Logging & Tracing](23-39-config-logging-tracing.md) | 23, 39 | `log/slog`, env config, health checks, OpenTelemetry spans |
| [24-26 Idiomatic Go & Architecture](24-26-idiomatic-architecture.md) | 24–26 | naming, style rules, project layout, the review checklist |
| [27 gRPC](27-grpc.md) | 27 | protobuf, server/client, streaming, interceptors |
| [28-30 Design Patterns](28-30-design-patterns.md) | 28–30 | creational, structural, behavioral — the Go form of each |
| [31-33-45 DDD, Hexagonal & DI](31-33-45-ddd-hexagonal-di.md) | 31–33, 45 | aggregates, ports/adapters, wiring, type registries |
| [34-35-44 Events, Outbox & Queues](34-35-44-events-outbox-queues.md) | 34–35, 44 | async events, the outbox, sagas, background jobs |
| [36-68 Resilience & Rate Limiting](36-68-resilience-rate-limiting.md) | 36, 68 | timeout/retry/breaker/bulkhead, the five limiter algorithms |
| [37-38 CQRS & Caching](37-38-cqrs-caching.md) | 37–38 | read/write split, event sourcing, cache-aside, `singleflight` |
| [41-43 API Design & Authorization](41-43-api-authz.md) | 41, 43 | versioning, pagination, `problem+json`, RBAC, tenant scoping |
| [42-50 Trees & Linear Structures](42-50-trees-linear.md) | 42, 50 | recursion shapes, stacks/queues/deques, linked lists, LRU |
| [51-53 Sorting, Heaps & Graphs](51-53-sorting-heaps-graphs.md) | 51–53 | `slices`/`sort`/`cmp`, `container/heap`, BFS/DFS/Dijkstra |
| [54-55 Collections & Data Pipelines](54-55-collections-pipelines.md) | 54–55 | filter/map/reduce/group/set, iterators, JSON/CSV/DB rows |
| [46-48 Low-Latency Go](46-48-low-latency.md) | 46–48 | benchmarks, escape analysis, GC knobs, pprof, lock-free |
| [56-57 Auth & Web Security](56-57-auth-security.md) | 56–57 | hashing, JWT, cookies, CSRF, OAuth2, the OWASP checklist |
| [58-67 Real-Time & Multi-User State](58-67-realtime-multiuser.md) | 58, 67 | SSE, WebSocket frames, the hub, request/user/process state |
| [60-62 Build, CI & Deploy](60-62-build-ci-deploy.md) | 60–62 | build flags, Docker, GitHub Actions, Kubernetes, signals |
| [63-66 Networking & Serving](63-66-networking-serving.md) | 63–66 | TCP/sockets, the HTTP wire format, timeouts, listener catalog |

## Regenerating the PDFs

```sh
./render.sh                       # every sheet
./render.sh 06-functions.md       # just one
```

`render.sh` builds [tools/render](tools/render/) — a small dependency-free Markdown-to-HTML
converter — and prints the result with headless Chrome. Override the browser path with
`CHROME=/path/to/chrome ./render.sh`.

## Checking a sheet

```sh
python3 tools/check.py 19-stdlib.md
```

Verifies that code fences are balanced, that no code line is too wide for the PDF,
and that every stdlib symbol named in the sheet actually exists (via `go doc`).
