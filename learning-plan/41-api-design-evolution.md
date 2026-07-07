# 41 — API Design & Evolution

> Part 9, Track C: [38 Caching](38-caching-patterns.md) → [39 Observability](39-observability-tracing.md) → [40 Testing Architecture](40-testing-architecture.md) → **41 API Design & Evolution**.
> [20](20-http-server.md)/[21](21-rest-api.md) built a JSON REST API; [27](27-grpc-microservices.md) mapped gRPC codes to HTTP. This lesson is about designing an API that **evolves without breaking clients** — versioning, pagination, idempotency, error contracts, and the conventions that make an API pleasant and durable.

## Goals
- Design REST resources, verbs, and status codes that behave predictably (incl. idempotency semantics).
- Evolve an API **backward-compatibly**, and version it only when you truly must.
- Paginate, filter, and sort at scale — **cursor** vs offset — and handle **idempotency keys** for safe retries.
- Return machine-readable errors (**RFC 9457 `problem+json`**) and use HTTP **caching/rate-limit** headers.

## Concepts

### Resources, verbs, status codes — done right
Model **nouns** (resources), act with HTTP **verbs**, and mean the **status codes**:
- `GET` (safe, no side effects), `POST` (create / non-idempotent action), `PUT` (full replace, **idempotent**), `PATCH` (partial update), `DELETE` (**idempotent** — deleting twice still ends deleted).
- `2xx` success (`200` ok, `201` created + `Location`, `202` accepted for async, `204` no content), `4xx` client error (`400` malformed, `401` unauthenticated, `403` unauthorised, `404` not found, `409` conflict, `422` unprocessable, `429` rate-limited), `5xx` server error.
**Idempotency of verbs is a contract**, and it's what makes safe retries ([36](36-resilience-patterns.md)) possible: a client can safely re-`PUT`/`DELETE` after a timeout, but not blindly re-`POST` (see idempotency keys below).

### Evolve backward-compatibly; version only when forced
Most changes should require **no** new version. Backward-compatible (safe) changes:
- **Add** an optional field to a response; **add** an optional request field with a default; **add** a new endpoint, a new enum value the client can ignore.
Breaking (needs a version): **removing/renaming** a field, changing a type, making an optional field required, changing status-code semantics, tightening validation. The rule mirrors proto field numbers ([27](27-grpc-microservices.md)) and event schemas ([34](34-event-driven-outbox.md)): **additive is free; destructive breaks clients.** Follow **Postel's law** — be liberal in what you accept (ignore unknown request fields), conservative in what you send.

When you must break, **version**:
- **URI versioning** (`/v1/orders`, `/v2/orders`) — most common, most visible, easy to route and cache.
- **Header/media-type versioning** (`Accept: application/vnd.acme.v2+json`) — cleaner URLs, harder to test/curl.
Run old and new **side by side**, publish a **deprecation policy** (`Deprecation` and `Sunset` headers, a timeline), and give clients time to migrate. Never silently change v1's behaviour.

### Pagination: cursor beats offset at scale
Never return an unbounded list. Two schemes:
- **Offset/limit** (`?limit=20&offset=40`) — simple and jump-to-page-able, but (a) **slow for deep pages** (`OFFSET 100000` scans and discards 100k rows) and (b) **drifts** — inserts/deletes between pages shift the window, duplicating or skipping rows.
- **Cursor / keyset** (`?limit=20&cursor=<opaque>`) — the cursor encodes the last-seen sort key (e.g. `(created_at, id)`); the next query is `WHERE (created_at, id) < (:t, :id) ORDER BY created_at DESC, id DESC LIMIT 20`. **Stable** under concurrent writes and **fast at any depth** (uses the index, no scan-and-discard). The cost: no random page access. **Prefer cursor pagination for large or live datasets.**
```go
// Opaque cursor: base64 of the last row's sort key — clients treat it as a token.
type cursor struct{ CreatedAt time.Time `json:"t"`; ID string `json:"id"` }
func encode(c cursor) string { b, _ := json.Marshal(c); return base64.URLEncoding.EncodeToString(b) }
// Response envelope carries the next cursor (empty when done):
type page[T any] struct {
    Data       []T    `json:"data"`
    NextCursor string `json:"next_cursor,omitempty"`
}
```
Always cap `limit` server-side (e.g. max 100). Support consistent **filtering** and **sorting** query params, and whitelist sortable columns (never interpolate a raw column name — SQL injection; the [22](22-database.md) rule).

### Idempotency keys — safe retries for non-idempotent writes
A `POST /payments` that times out leaves the client unsure whether it charged. The fix: the client sends a unique **`Idempotency-Key`** header; the server stores `key → result` and, on a repeat, returns the **stored** result instead of acting again:
```go
func (h Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
    key := r.Header.Get("Idempotency-Key")
    if key == "" { writeProblem(w, 400, "missing Idempotency-Key"); return }
    if saved, ok := h.idem.Lookup(r.Context(), key); ok {
        writeJSON(w, saved.Status, saved.Body)   // replay the original outcome
        return
    }
    // ... process once, then persist (key → status+body) in the SAME tx as the effect
}
```
This is the client-facing half of the idempotency you built for consumers ([34](34-event-driven-outbox.md)) and saga steps ([35](35-sagas-distributed-transactions.md)) — the same idea at the edge. Stripe's API is the canonical example.

### Error contract: RFC 9457 `problem+json`
Ad-hoc error shapes force every client to special-case. Standardise on **Problem Details** (RFC 9457, formerly 7807): `Content-Type: application/problem+json` with a stable structure, plus your own **machine-readable code** so clients branch on a constant, not a message string:
```json
{
  "type": "https://api.acme.com/problems/insufficient-funds",
  "title": "Insufficient funds",
  "status": 422,
  "detail": "Account 42 has balance 1000, requested 1500.",
  "instance": "/accounts/42/withdrawals",
  "code": "INSUFFICIENT_FUNDS"
}
```
Never leak internals (stack traces, SQL, panic text) — that's the [27](27-grpc-microservices.md) "don't leak errors across a boundary" rule at the public edge. Keep the `code` set stable and documented; clients depend on it.

### Consistency conventions (the details that age well)
- **Naming** — pick `snake_case` or `camelCase` and never mix; be consistent across every endpoint.
- **Timestamps** — RFC 3339 / ISO 8601 in **UTC** (`2026-07-07T12:00:00Z`), always.
- **Money** — integer **minor units + currency** (`{"amount": 1500, "currency": "USD"}`), never a float ([31](31-ddd-tactical.md)).
- **Enums** — strings (`"pending"`), not magic integers; add values additively.
- **IDs** — opaque strings (UUID/ULID); don't leak auto-increment counts.

### HTTP caching & rate-limit headers (cheap wins)
- **`ETag` + `If-None-Match`** → return `304 Not Modified` when unchanged; **`Cache-Control`** tells clients/CDNs how long to cache. This is caching ([38](38-caching-patterns.md)) at the protocol layer — requests that never reach your server.
- **Rate limiting** ([36](36-resilience-patterns.md)) → `429 Too Many Requests` with **`Retry-After`** and `RateLimit-*` headers so clients back off politely.

### Contract-first with OpenAPI
Treat the **OpenAPI** spec as the source of truth: design it, review it, then **generate** server stubs/clients (`oapi-codegen`) and validate requests against it. Lint it (`spectral`) and run a **breaking-change check** in CI so an incompatible edit fails the build ([40](40-testing-architecture.md)'s contract idea for REST). Spec-first keeps docs, code, and clients from drifting.

## Exercises
1. Take a `GET /orders` that returns all rows and add server-side pagination: first offset/limit, then **cursor** (opaque base64 of `(created_at, id)`). Insert a row mid-pagination and show offset drifts/duplicates while the cursor stays stable.
2. Cap `limit` at 100 server-side and whitelist `sort` columns; prove an unknown/injected sort param is rejected, not interpolated.
3. Implement an `Idempotency-Key` middleware: store `key → (status, body)`, replay stored results on repeat, require the header on `POST`. Simulate a client retry after a timeout and confirm exactly one side effect.
4. Write a `problem+json` error writer (RFC 9457 fields + a stable `code`) and convert your handlers' errors to it; confirm the `Content-Type` and that no internal detail leaks.
5. Make a backward-compatible change (add an optional field) and prove an old client still works. Then make a breaking change and introduce `/v2`, running `/v1` and `/v2` side by side with a `Deprecation`/`Sunset` header on v1.
6. Add `ETag`/`If-None-Match` to a `GET` and return `304` when unchanged; add `429` + `Retry-After` to a rate-limited endpoint.
7. Audit an endpoint for the consistency conventions (naming, UTC timestamps, money as minor units, string enums, opaque ids) and fix every violation.
8. Write a minimal OpenAPI spec for one resource; generate a client or server stub from it and note what stays in sync automatically.

## Best Practices & Pitfalls
- **Additive changes don't need a version; destructive ones do.** Add optional fields freely; never remove/rename/retype in place. Be liberal in what you accept, conservative in what you send.
- **Version visibly and run versions side by side.** URI versioning is the pragmatic default; publish a deprecation timeline with `Deprecation`/`Sunset` — never silently change old behaviour.
- **Cursor-paginate large/live lists.** Offset is fine for small admin tables; it drifts and slows on scale. Always cap `limit` and whitelist sort fields.
- **Support idempotency keys for non-idempotent writes.** It's what makes client and gateway retries safe; store key→result and replay.
- **Standardise errors on `problem+json` with stable codes.** Clients branch on the `code`, not the human message; never leak internals.
- **Be relentlessly consistent** — one casing, UTC RFC3339 times, money as integer minor units, string enums, opaque ids. Inconsistency is a thousand small client bugs.
- **Pitfall — leaking DB internals into the API.** Auto-increment ids reveal volume and enable enumeration; raw SQL errors leak schema. Use opaque ids and mapped errors.
- **Pitfall — unbounded responses.** A list endpoint with no limit is a latency/OOM incident waiting to happen. Paginate everything.
- **Pitfall — breaking changes disguised as "small".** Tightening validation, changing a default, or renaming a field breaks clients as surely as deleting one. Treat the response shape as a contract.
- **Pitfall — idempotency keys without expiry/scoping.** Store them with a TTL and scope to the endpoint/user; an unbounded key table grows forever and cross-endpoint collisions replay the wrong result.

## Checklist
- [ ] I can design resources/verbs/status codes and explain which verbs are idempotent and why it matters for retries.
- [ ] I can classify a change as backward-compatible or breaking and version only when forced, side by side with a deprecation policy.
- [ ] I can implement cursor pagination (opaque token) and explain why it beats offset at scale.
- [ ] I can implement idempotency keys for safe `POST` retries.
- [ ] I can return `problem+json` errors with stable machine-readable codes and no leaked internals.
- [ ] I apply consistent conventions (casing, UTC timestamps, money, enums, opaque ids) and use `ETag`/`429` headers.
- [ ] I can treat an OpenAPI spec as the source of truth with codegen + CI breaking-change checks.

## Resources
- RFC 9457 — Problem Details for HTTP APIs: https://www.rfc-editor.org/rfc/rfc9457.html
- Google API Improvement Proposals (AIPs) — pagination, errors, versioning: https://google.aip.dev/
- Microsoft REST API Guidelines: https://github.com/microsoft/api-guidelines
- Stripe on idempotency keys: https://docs.stripe.com/api/idempotent_requests
- OpenAPI + Go codegen (`oapi-codegen`): https://github.com/oapi-codegen/oapi-codegen · lint with `spectral`
- Builds on [21 — REST API](21-rest-api.md), [27 — code→HTTP mapping](27-grpc-microservices.md). Series end — back to [README](README.md).
