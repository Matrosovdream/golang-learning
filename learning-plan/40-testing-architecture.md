# 40 — Testing Architecture

> Part 9, Track C: [38 Caching](38-caching-patterns.md) → [39 Observability](39-observability-tracing.md) → **40 Testing Architecture** → [41 API Design & Evolution](41-api-design-evolution.md).
> [18 — Testing](18-testing.md) taught the *mechanics* (table tests, `httptest`, benchmarks). This lesson is about **strategy**: how to structure tests for a real service so they're fast, trustworthy, and not a maintenance sinkhole — leaning on the hexagonal core ([32](32-hexagonal-ports-adapters.md)) and `testcontainers` for real infra.

## Goals
- Apply the **test pyramid** to a service: many fast unit tests, fewer integration tests, a thin end-to-end layer.
- Choose the right **test double** — prefer hand-written **fakes** over mock frameworks, and know when a mock earns its keep.
- Run **integration tests against real dependencies** with `testcontainers-go` (a throwaway Postgres/Redis per suite).
- Keep tests **deterministic** (injected clocks/ids, `-race`, `t.Parallel`) and guard contracts between services.

## Concepts

### The test pyramid (and the ice-cream cone to avoid)
Shape your suite like a pyramid:
- **Unit (many, milliseconds)** — the domain and use cases ([31](31-ddd-tactical.md)/[32](32-hexagonal-ports-adapters.md)) with in-memory adapters. No network, no DB. This is where most of your coverage and confidence live.
- **Integration (fewer, seconds)** — one adapter against the *real* technology: the Postgres repo against a real Postgres, the cache against a real Redis. Verifies the SQL/queries/mapping that a fake can't.
- **End-to-end (few, slow, flaky)** — the whole system through its public API. Reserve for a handful of critical happy paths.
The **anti-pattern** is the inverted "ice-cream cone": mostly slow E2E tests, few units. It's slow, flaky, and hard to localise failures. Hexagonal architecture is what *lets* the pyramid stand — a core that depends only on ports is trivially unit-testable.

### Test doubles: fakes > mocks (mostly)
Terminology (Fowler/Meszaros):
- **Dummy** — passed but unused (fills a parameter).
- **Stub** — returns canned answers (`Get` always returns this user).
- **Fake** — a *working* lightweight implementation (an in-memory repo with a map). **Your default.**
- **Spy** — a stub that also records how it was called.
- **Mock** — a double with **pre-programmed expectations** that fails the test if calls don't match.

Prefer **hand-written fakes** for your ports: an in-memory `OrderRepository` backed by a map is reusable across dozens of tests, exercises real behaviour (it actually stores and returns), and doesn't couple tests to the *sequence* of internal calls:
```go
type fakeOrders struct{ m map[OrderID]*Order }
func newFakeOrders() *fakeOrders { return &fakeOrders{m: map[OrderID]*Order{}} }
func (f *fakeOrders) Save(_ context.Context, o *Order) error            { f.m[o.ID()] = o; return nil }
func (f *fakeOrders) Get(_ context.Context, id OrderID) (*Order, error) {
    o, ok := f.m[id]
    if !ok { return nil, ErrNotFound }
    return o, nil
}
```
Reach for a **mock** (hand-written or generated via `go.uber.org/mock`/`mockgen`) only when the *interaction itself* is the behaviour under test — "the service must call `payment.Charge` exactly once with this amount", "it must **not** send the email on failure". Mocks that assert on internal call order make tests brittle: they break on harmless refactors. Fakes assert on **outcome**; mocks assert on **interaction** — choose by what actually matters.

### Integration tests with `testcontainers-go`
Testing the Postgres adapter against SQLite or a fake lies to you (different SQL, different constraints). `testcontainers-go` spins up a **real, throwaway** dependency in Docker for the test, then tears it down — deterministic, isolated, no shared staging DB:
```go
func TestOrderRepo_Integration(t *testing.T) {
    if testing.Short() { t.Skip("skipping integration test") } // `go test -short` skips
    ctx := context.Background()

    pg, err := postgres.Run(ctx, "postgres:16",
        postgres.WithDatabase("app"), postgres.WithUsername("t"), postgres.WithPassword("t"),
        testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = pg.Terminate(ctx) })   // t.Cleanup > defer for teardown

    dsn, _ := pg.ConnectionString(ctx, "sslmode=disable")
    db := mustMigrate(t, dsn)                       // run real migrations
    repo := NewOrderRepo(db)

    // ...exercise real SQL: Save then Get, assert the round-trip and constraints
}
```
Guard integration tests behind `testing.Short()` (or a build tag) so `go test -short ./...` runs the fast pyramid base in seconds, while CI runs the full set. Share one container across a package's tests via `TestMain` when startup cost matters.

### Contract testing — keep services in sync
Unit + integration tests verify **one** service. They don't catch "the provider changed a field the consumer relied on." **Consumer-driven contract testing** (e.g. Pact) has each consumer publish the shape it expects; the provider's CI verifies it still satisfies every consumer's contract — catching breaking changes *before* deploy, without a full E2E environment. For gRPC/proto and event schemas ([27](27-grpc-microservices.md)/[34](34-event-driven-outbox.md)), a **schema-compatibility check in CI** (buf breaking, schema registry) is the lighter-weight equivalent. Contracts are how you get microservice confidence without an army of E2E tests.

### Determinism: the enemy is hidden state
Flaky tests erode trust until people ignore red builds. The usual culprits and fixes:
- **Time** — never call `time.Now()` deep in code under test. Inject a **clock** (`type Clock interface{ Now() time.Time }`), use a fake in tests. Same for `rand` and UUIDs — inject an id generator.
- **Sleeps** — `time.Sleep` to "wait for" async work is the #1 flake source. Poll a condition with a timeout, use channels/`sync.WaitGroup`, or a synchronous test seam instead.
- **Ordering / parallelism** — run with `-race` (from [15](15-sync-context.md)) in CI; use `t.Parallel()` for independent tests but ensure they don't share mutable state.
- **Shared external state** — a shared dev DB makes tests order-dependent. testcontainers gives each run a clean one.

### Other tools in the belt
- **`httptest`** ([20](20-http-server.md)/[21](21-rest-api.md)) — drive handlers in-process without a socket.
- **Golden files** — assert a serialised output (JSON response, generated SQL) against a checked-in `testdata/*.golden`, updatable with a `-update` flag. Great for large stable outputs.
- **Test data builders** — the builder pattern ([28](28-patterns-creational.md)) for fixtures: `newOrder().withItems(2).placed()` reads clearly and defaults the boring fields.
- **Black-box tests** — put tests in `package foo_test` to test only the exported API, keeping you honest about the public surface.
- **`t.Cleanup`, `t.Helper`, `t.TempDir`** — lean on the testing toolkit for teardown, better failure lines, and isolated temp dirs.

## Exercises
1. Draw your service's test pyramid: list which tests are unit (core + fakes), which are integration (one real dependency), and which (few) are E2E. Flag any inversion.
2. Write a reusable in-memory **fake** for a repository port and use it in three different use-case unit tests. Note that no test knows the fake's internals.
3. Convert one test to a **mock** where the *interaction* is the point (e.g. "sends exactly one payment"); then break it by a harmless refactor and observe the brittleness — decide fake vs mock for that case.
4. Write a `testcontainers-go` Postgres integration test for a repository: run migrations, `Save`→`Get` round-trip, and assert a unique-constraint violation surfaces as your domain error. Guard it with `testing.Short()`.
5. Share the container across a package via `TestMain`; measure the wall-clock difference vs per-test containers.
6. Inject a `Clock` and an id generator into code that currently calls `time.Now()`/`uuid.New()`; make a time-dependent test deterministic and remove a `time.Sleep`.
7. Add a golden-file test for an HTTP JSON response with a `-update` flag; change the response and regenerate the golden.
8. (Concept) Sketch how a consumer-driven contract (or a `buf breaking` check) would have caught a field rename between two of your services before deploy.

## Best Practices & Pitfalls
- **Most tests are fast unit tests against ports.** If they're not, your architecture — not your testing — is the problem. A hard-to-unit-test core means a dependency isn't behind an interface yet.
- **Prefer fakes; use mocks for interaction contracts only.** Fakes assert outcomes and survive refactors; mocks assert call sequences and break easily. Reach for a mock when "did it call X?" *is* the requirement.
- **Integration-test against the real thing** via testcontainers, not against SQLite/fakes that behave differently. Test the SQL and constraints that actually ship.
- **Keep the fast base runnable in seconds** (`-short`), full suite in CI. Developers must be able to run units constantly.
- **Make tests deterministic:** inject time/rand/ids, poll instead of sleep, run `-race`, isolate state. A flaky test is worse than no test — it trains people to ignore failures.
- **Guard cross-service contracts** with consumer-driven contracts or schema-compatibility checks, so you don't need dozens of E2E tests for confidence.
- **Pitfall — the ice-cream cone.** Mostly-E2E suites are slow, flaky, and don't localise failures. Push logic and its tests down to the unit level.
- **Pitfall — over-mocking.** Tests that mock every collaborator verify that your code calls the methods you told it to — a tautology that breaks on refactor and proves little. Test behaviour through fakes.
- **Pitfall — `time.Sleep` in tests.** Almost always a race waiting to flake. Synchronise explicitly.
- **Pitfall — asserting on unstable output.** Golden-file a JSON blob only after normalising timestamps/ids, or it fails every run.

## Checklist
- [ ] I can place a service's tests on the pyramid and justify each layer's size.
- [ ] I can write reusable in-memory fakes for ports and choose fake vs mock deliberately.
- [ ] I can write a `testcontainers-go` integration test with real migrations, guarded by `-short`.
- [ ] I can make a test deterministic by injecting time/ids and removing sleeps, and run `-race`.
- [ ] I can use golden files and test-data builders where they fit.
- [ ] I can explain how contract/schema-compatibility testing guards service boundaries without heavy E2E.

## Resources
- Martin Fowler — "The Practical Test Pyramid": https://martinfowler.com/articles/practical-test-pyramid.html · "Test Double" taxonomy: https://martinfowler.com/bliki/TestDouble.html
- `testcontainers-go`: https://golang.testcontainers.org/
- `go.uber.org/mock` (maintained gomock) & `mockgen`: https://github.com/uber-go/mock
- Pact (consumer-driven contract testing) & `buf breaking` (proto compat): https://docs.pact.io/ · https://buf.build/docs/breaking/overview
- Builds on [18 — Testing & Benchmarking](18-testing.md); enabled by [32 — Hexagonal](32-hexagonal-ports-adapters.md).
- Next: [41 — API Design & Evolution](41-api-design-evolution.md).
