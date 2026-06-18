# midigator-dashboard-hard — multi-tenant chargeback dashboard (Go)

The **capstone** of the example-projects track: a faithful Go port of a real
PHP/Laravel SaaS (`midigator-dashboard-app`) for **payment-dispute / chargeback
management**. It folds in every pattern the earlier projects taught and adds the
hard ones — multi-tenancy, RBAC, polymorphic relations, a unified case
abstraction, an inbound-webhook → background-worker pipeline, domain events, and
a two-plane (tenant vs platform) API.

Each **tenant** (merchant) connects to a payment-dispute provider; the platform
ingests **chargebacks, prevention alerts, order validations, and RDR cases** via
signed webhooks, and a team works those cases through a **workflow**
(assign → stage transitions → resolve), attaching **comments / evidence**,
sending **emails**, and watching **dashboards**. On top sits a **platform-admin
plane** (super-admin over all tenants) with **impersonation**.

> Built on the same Go clean-architecture skeleton as the rest of the track:
> `handler → service → domain ← repository`, raw SQL via **pgx**, stdlib
> `net/http` routing, Docker Compose + Postgres.

---

## What makes it "hard" — the patterns it teaches

1. **Multi-tenant global scoping.** The tenant is resolved from the JWT in
   middleware and carried in the request context; every repository query carries
   an explicit `WHERE tenant_id = $N` (the Go analogue of Laravel's global
   `TenantScope` + `BelongsToTenant`). See [internal/reqctx/reqctx.go](internal/reqctx/reqctx.go)
   and any repo, e.g. [chargeback_repository.go](internal/repository/postgres/chargeback_repository.go).
2. **RBAC.** A global **rights** catalog, per-tenant **roles**, `role_rights` +
   `user_roles` pivots, a `RequireRight("chargebacks.view")` route middleware, a
   platform-admin bypass, and a per-user rights cache.
   See [internal/middleware/auth.go](internal/middleware/auth.go) and
   [internal/rights/cache.go](internal/rights/cache.go).
3. **Polymorphic relations.** Comments, evidence, stage-transitions, email-logs
   and the activity-log attach to any case/order via a `(type, id)` pair.
4. **Unified case abstraction.** One `CaseRepository` interface + a `CaseRegistry`
   let the generic case endpoints (list / stage / assign / hide / resolve),
   search, the assignment board and the dashboards work without ever branching on
   the concrete case type. See [internal/domain/case.go](internal/domain/case.go)
   and [internal/service/registry.go](internal/service/registry.go).
5. **Workflow state machine.** Every stage change writes a `stage_transitions`
   audit row inside a transaction. See [common.go `changeStageTx`](internal/repository/postgres/common.go).
6. **Webhook → outbox → worker.** A signed inbound webhook is recorded, enqueued
   on **asynq/Redis**, and processed by a separate worker binary into the right
   case type. See [internal/worker/](internal/worker/).
7. **Domain events / observers.** A tiny in-process event bus turns "a case
   arrived" / "a case was assigned" into notifications, without the publisher
   knowing who listens. See [internal/events/events.go](internal/events/events.go)
   and [internal/service/subscribers.go](internal/service/subscribers.go).
8. **Two API planes.** A tenant plane (`/api/v1/...`) and a platform super-admin
   plane (`/api/v1/platform/...`) with **impersonation**.

---

## Tech stack

- Go 1.26, stdlib `net/http` routing (Go 1.22 method+pattern mux)
- PostgreSQL 16 — **pgx/v5** with raw SQL
- Redis 7 + **asynq** — background job queue (webhook processing)
- **JWT** access tokens (golang-jwt/v5) + **bcrypt** (password & PIN login)
- Docker Compose (api + worker + seed + postgres + redis)

---

## Project layout

```
cmd/
  api/      HTTP server
  worker/   asynq worker (drains webhook:process)
  seed/     one-shot demo-data loader
internal/
  config/         env-driven config
  domain/         entities + repository/service interfaces + typed errors (the core)
  reqctx/         request-scoped identity (no imports → no cycles)
  auth/           JWT issue/verify + bcrypt
  rights/         per-user rights cache
  events/         in-process pub/sub bus + event types
  middleware/     request-id, logging, recover, JWT auth, RequireRight, platform-admin
  repository/postgres/  pgx implementations (tenant-scoped raw SQL)
  service/        business logic: validation, RBAC tenant boundary, orchestration, events
  handler/        HTTP transport: decode, call service, map domain errors → status, encode
  router/         every route → handler, with the middleware chain
  worker/         asynq enqueuer + processor + server
  seed/           rights catalog, demo tenant/roles/users, synthetic cases
migrations/        001_init.sql (mounted into postgres initdb)
```

---

## Data model (22 tables)

**Identity & tenancy:** `tenants`, `users` (nullable `tenant_id`; platform admins
have none), `site_settings`.
**RBAC:** `rights` (global catalog), `roles` (per-tenant), `role_rights`,
`user_roles`, `manager_profiles`.
**Case types** (all tenant-scoped; `assigned_to`, `stage`, `is_hidden`):
`chargebacks`, `prevention_alerts`, `order_validations`, `rdr_cases`, plus
`orders` (no stage — a submission lifecycle).
**Polymorphic / cross-cutting:** `comments`, `evidence_files`,
`stage_transitions`, `activity_log`.
**Comms & ops:** `email_templates`, `email_logs`, `webhook_logs`,
`notifications` (uuid pk), `notification_settings`.

Full DDL: [migrations/001_init.sql](migrations/001_init.sql).

---

## RBAC — roles × rights

~50 rights grouped by domain (`chargebacks.*`, `preventions.*`, `orders.*`,
`order_validations.*`, `rdr.*`, `comments.*`, `emails.*`, `users.*`, `roles.*`,
`managers.*`, `dashboard.*`, `settings.*`, `activity_log.*`, `evidence.*`,
`export.run`). Four system roles are seeded per tenant, plus a platform admin.

### Demo logins (PIN login screen)

| Role           | Email                      | PIN  |
|----------------|----------------------------|------|
| Tenant Admin   | admin@midigator.test       | 1234 |
| Manager        | manager@midigator.test     | 5678 |
| Analyst        | analyst@midigator.test     | 4321 |
| Viewer         | viewer@midigator.test      | 8765 |
| Platform Admin | platform@midigator.test    | 9999 |

All demo users also share the password `password`.

---

## API surface (~100 routes)

All tenant routes are JWT-authenticated and gated by a per-route right.

**Auth** — `POST /api/v1/auth/login`, `POST /api/v1/auth/login-pin`,
`GET /api/v1/auth/me`, `POST /api/v1/auth/logout`.

**Cases** (`chargebacks`, `preventions`, `order-validations`, `rdr-cases`):
`GET /` · `GET /{id}` · `PATCH /{id}` · `POST /{id}/stage` · `/assign` · `/hide`
(+ `/resolve` on preventions & rdr; `POST /chargebacks/hide/bulk`).

**Orders** — `GET /orders`, `POST /orders`, `GET/PATCH /orders/{id}`,
`POST /orders/{id}/submit`.

**Polymorphic** — `GET|POST /api/v1/comments/{type}/{id}`,
`DELETE /api/v1/comments/{id}`; same shape under `/api/v1/evidence/...`
(`{type}` ∈ chargeback, prevention, order, order_validation, rdr).

**Dashboards & manager** — `/dashboard/summary`, `/dashboard/manager-performance`,
`/dashboard/recent-activity`; `/manager/home|team|assignments|approvals|activity`,
`POST /manager/assignments/{kind}/{id}`, `PATCH /users/{userId}/score`.

**Comms & data** — `email-templates` CRUD, `POST /emails/send`, `GET /email-logs`,
`GET /notifications` (+ read / read-all / settings), `GET /export/{type}`,
`GET /search?q=`, `GET /activity-log`, `GET /users`.

**Platform plane** (`/api/v1/platform/...`, platform-admin only) — `tenants`
full CRUD + `overview|users|activity|webhooks|toggle-active|test-connection`;
`users` (toggle-active, toggle-platform-admin); `rights`; `settings`;
`overview/summary`; `integrations/health`; `webhooks`; `emails/templates` +
`emails/logs`; `activity`; **impersonation** `POST /platform/impersonate/{userId}`
+ `POST /platform/impersonate/stop`.

**Webhook ingress** — `POST /rest/v1/webhooks/midigator` (per-tenant HTTP Basic).

---

## Run it

```bash
cd midigator-dashboard-hard
cp .env.example .env          # or just use the committed .env
docker compose up -d --build  # postgres + redis + seed (one-shot) + api + worker
```

The API is on `http://localhost:8080`. The `seed` service loads the rights
catalog, the demo tenant, the five demo users, and a few hundred synthetic cases
on first boot (it is idempotent).

### Try it

```bash
# 1. Log in (PIN) and grab the token
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login-pin \
  -H 'Content-Type: application/json' \
  -d '{"email":"manager@midigator.test","pin":"5678"}' | jq -r .token)

# 2. List chargebacks (tenant-scoped, paginated)
curl -s localhost:8080/api/v1/chargebacks -H "Authorization: Bearer $TOKEN"

# 3. Move one through the workflow (writes a stage_transitions audit row)
curl -s -X POST localhost:8080/api/v1/chargebacks/1/stage \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"stage":"in_review","notes":"working it"}'

# 4. Dashboard summary (counts across all four case types)
curl -s localhost:8080/api/v1/dashboard/summary -H "Authorization: Bearer $TOKEN"
```

### Webhook → worker → notification

```bash
curl -s -u midigator:webhook-secret -X POST \
  localhost:8080/rest/v1/webhooks/midigator -H 'Content-Type: application/json' \
  -d '{"event_type":"chargeback.created","event_guid":"evt-1",
       "data":{"guid":"cb-001","mid":"MID-1001","amount":7777,"currency":"USD",
               "card_last_4":"4242","reason_code":"10.4","order_id":"ORD-1"}}'
# → 202 accepted. The worker inserts the chargeback, marks the webhook log
#   processed, and the CaseReceived event creates a notification for each
#   active tenant user.
```

---

## What was simplified vs the original Laravel app

Faithful in **data model, RBAC, routes, and behavior**; deliberately trimmed to
keep the Go project focused and dependency-light:

- **Search** uses Postgres `ILIKE` across case types instead of Meilisearch.
- **Notifications** are in-app rows (created via the event bus); the original's
  Reverb websockets are out of scope.
- **Email** uses a synchronous "log mailer" (records an `email_logs` row marked
  `sent`) rather than real SMTP.
- The **Midigator API client** (order submission, prevention resolution to the
  upstream) is stubbed — the dashboard records the local state transition, which
  is what the workflow and the UI care about.
- The frontend SPA is not included; this is the JSON API + worker.
