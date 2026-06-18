# Build checklist — midigator-dashboard-hard

A 9-phase build. Each phase compiles; the reference slice and every later phase
were runtime-verified against Postgres + Redis before moving on.

## P0 — Scaffold
- [x] `go.mod` (pgx, golang-jwt, asynq, x/crypto), `.env(.example)`, `.dockerignore`
- [x] `docker-compose.yml` (postgres + redis + seed + api + worker) + multi-binary `Dockerfile`
- [x] `migrations/001_init.sql` — all 22 tables, indexes, CHECK enums, FKs
- [x] `internal/config` — env-driven config

## P1 — Domain (the core)
- [x] Entities + repository/service interfaces for every aggregate
- [x] Unified `CaseType` / `CaseSummary` / `CaseFilter` / `CaseRepository` abstraction
- [x] Typed errors (`ValidationError`, `NotFoundError`, `ConflictError`, `ErrUnauthorized/Forbidden`)

## P2 — Shared foundation
- [x] `db` pool (connect-with-retry), `reqctx` (request identity, no imports)
- [x] `auth` (JWT issue/verify + bcrypt), `rights` cache, `events` bus
- [x] `middleware` — request-id / logger / recover / JWT auth / RequireRight / platform-admin

## P3 — Reference slice (chargebacks) — runtime-verified
- [x] Chargeback repo (CaseRepository + typed Get/Update/Insert/BulkHide), service, handler
- [x] Auth (password + PIN), RBAC enforcement, tenant scoping, stage transitions, activity log
- [x] Router + api main + seed; **verified**: PIN login, 401/403/400 paths, stage audit rows

## P4 — Case types + polymorphism — runtime-verified
- [x] prevention / order_validation / rdr (repo + service + handler), orders (+submit)
- [x] Polymorphic comments + evidence (subject validation, existence checks, tenant scope)
- [x] Notifications (repo + service + handler), ancillary repos (email/webhook/settings/manager-profile)
- [x] **Verified**: all case types list/show/update/stage/assign/hide/resolve, order create/submit

## P5 — Analytics + tenant features — runtime-verified
- [x] `ReportingRepository` (UNION across the 4 case tables): stage counts, leaderboard, unassigned, in-stage, platform/tenant totals
- [x] dashboard, manager plane, emails (log mailer), CSV export, search, activity-log
- [x] **Verified**: dashboard summary, manager assignments, export CSV, email send+log

## P6 — Webhook → worker → events — runtime-verified
- [x] Webhook service (per-tenant Basic auth) + handler (`/rest/v1/webhooks/midigator`)
- [x] asynq enqueuer + processor (ingest into case type, idempotent on replay) + worker binary
- [x] Event subscribers: `CaseReceived` → notify tenant users, `CaseAssigned` → notify assignee
- [x] **Verified**: webhook → 202 → worker inserts case → log `processed` → notifications created

## P7 — Platform plane — runtime-verified
- [x] tenants CRUD + overview/users/activity/webhooks/toggle-active/test-connection
- [x] users (toggle active / platform-admin), rights, settings, overview, integration health, webhook logs
- [x] Impersonation (start/stop via stateless JWT `imp` claim)
- [x] **Verified**: platform overview, RBAC 403 for non-admin, impersonate→act-as→stop

## P8 — Integrate, document, verify
- [x] Final router (all ~100 routes, both planes) + api main wiring
- [x] `go build ./...` + `go vet ./...` clean; `gofmt` normalized
- [x] README + this checklist
- [x] `docker compose up --build` full-stack smoke test
