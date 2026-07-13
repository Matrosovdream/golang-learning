# 43 — Authorization, RBAC & Multi-Tenancy

> Part 9, Track C (production cross-cutting): pairs with [38](38-caching-patterns.md), [39](39-observability-tracing.md), [41](41-api-design-evolution.md).
> [21](21-rest-api.md) got you a bearer token — that's **authentication** (who you are). This lesson is **authorization**: what you're allowed to do, and *whose* data you're allowed to touch. We build role-based access control (RBAC), multi-tenant row scoping, and the mass-assignment defense — the exact machinery behind the midigator capstone.

## Goals
- Separate **authentication** (identity) from **authorization** (permission) and put each in the right layer.
- Carry the caller's identity through `context.Context` so every layer can read it without re-parsing a token.
- Model **RBAC** — a rights catalog, roles, and the `role_rights`/`user_roles` pivots — and enforce a right at the route with one middleware.
- Scope every query to the caller's **tenant** so one customer can never read another's rows.
- Defend against **overposting**: trust the context for ownership/tenant, never the request body.

## Concepts

### Authentication vs. authorization — two different jobs
**Authentication** answers *"who are you?"* — verify a JWT/session, load the user. **Authorization** answers *"may you do this, to this data?"* — check a permission and a tenant boundary. They even fail differently: a bad token is **401 Unauthorized**; a valid user lacking a right is **403 Forbidden**.

Keep them in separate middleware so the ordering is explicit — authenticate first (sets identity), then authorize per route:
```go
// Authenticate verifies the token and stashes identity; RequireRight gates the handler.
mux.Handle("GET /chargebacks", auth.Authenticate(
    middleware.RequireRight("chargebacks.view", h.Index)))
```

### Carry identity in the context, not in every signature
Authentication runs once, at the edge, and stashes the *effective identity* on the request context. Everything downstream reads it — no token re-parsing, no threading a `user` argument through ten functions. Use an **unexported key type** so no other package can collide ([15](15-sync-context.md)):
```go
type Auth struct {
    UserID          int64
    TenantID        *int64              // nil for a platform admin with no tenant
    IsPlatformAdmin bool
    Rights          map[string]struct{} // a set: presence == granted
}

type ctxKey int
const authKey ctxKey = iota

func With(ctx context.Context, a *Auth) context.Context { return context.WithValue(ctx, authKey, a) }
func From(ctx context.Context) (*Auth, bool) { a, ok := ctx.Value(authKey).(*Auth); return a, ok }

// HasRight is nil-safe and gives platform admins everything.
func (a *Auth) HasRight(slug string) bool {
    if a == nil {
        return false
    }
    if a.IsPlatformAdmin {
        return true
    }
    _, ok := a.Rights[slug] // comma-ok set membership
    return ok
}
```

### The RBAC model: rights → roles → users
Don't attach permissions to users directly — that doesn't scale past a handful of people. Insert **roles** in between:

| Concept | What it is | Example |
|---|---|---|
| **Right** | one fine-grained permission slug | `chargebacks.update` |
| **Role** | a named bundle of rights (per tenant) | "Analyst" |
| **`role_rights`** | pivot: role ↔ rights (many-to-many) | Analyst → {view, update} |
| **`user_roles`** | pivot: user ↔ roles (many-to-many) | Ann → {Analyst} |

```sql
rights(id, slug UNIQUE, "group", description)   -- global catalog
roles(id, tenant_id, slug, name)                -- per-tenant bundles
role_rights(role_id, right_id, UNIQUE(role_id, right_id))
user_roles(user_id, role_id,  UNIQUE(user_id, role_id))
```
A user's **effective rights** are the distinct slugs across all their roles — one JOIN:
```sql
SELECT DISTINCT ri.slug
FROM user_roles ur
JOIN role_rights rr ON rr.role_id = ur.role_id
JOIN rights ri      ON ri.id = rr.right_id
WHERE ur.user_id = $1;
```
The payoff: adding a permission is **data, not code** — insert a right, attach it to a role. No deploy, no `if user.IsManager` scattered through handlers.

### Enforce at the edge with a `RequireRight` middleware
Put the check on the *route*, not sprinkled through handlers. A tiny middleware reads the identity the auth step stashed and short-circuits with 403 if the right is missing:
```go
func RequireRight(slug string, next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if a := reqctx.MustFrom(r.Context()); !a.HasRight(slug) {
            writeError(w, http.StatusForbidden, "missing right "+slug)
            return
        }
        next(w, r)
    }
}
```
Because `HasRight` returns `true` for platform admins, super-admins bypass every gate **without a special case at each route**. A parallel `RequirePlatformAdmin` guards the admin plane.

### Cache the rights — don't hit the DB every request
That effective-rights JOIN would run on *every* authenticated request. Cache it per user with a TTL and an explicit flush when a user's roles change ([38](38-caching-patterns.md)). A `sync.RWMutex` guards the map ([15](15-sync-context.md)):
```go
type Cache struct {
    mu      sync.RWMutex
    entries map[int64]entry // userID -> {rights, expires}
    load    func(context.Context, int64) ([]string, error)
    ttl     time.Duration
}

func (c *Cache) Get(ctx context.Context, userID int64) (map[string]struct{}, error) {
    c.mu.RLock()
    e, ok := c.entries[userID]
    c.mu.RUnlock() // release BEFORE the slow load below — never hold a lock across I/O
    if ok && time.Now().Before(e.expires) {
        return e.rights, nil
    }
    slugs, err := c.load(ctx, userID) // miss: run the JOIN once
    if err != nil {
        return nil, err
    }
    set := make(map[string]struct{}, len(slugs))
    for _, s := range slugs {
        set[s] = struct{}{}
    }
    c.mu.Lock()
    c.entries[userID] = entry{rights: set, expires: time.Now().Add(c.ttl)}
    c.mu.Unlock()
    return set, nil
}
```
Invalidate on change: `cache.Flush(userID)` after editing that user's roles, so a **revoked** permission takes effect immediately instead of lingering for the whole TTL.

### Multi-tenancy: scope every query to the tenant
In a multi-tenant app, one database holds many customers' rows. **The cardinal rule: every tenant-scoped query carries `WHERE tenant_id = $N`.** Resolve the tenant once (from the JWT) into the context, then pass it into every repository call:
```go
// service layer: pull tenant from the authenticated identity, never from input.
func tenantActor(ctx context.Context) (userID, tenantID int64, err error) {
    a, err := actor(ctx)
    if err != nil {
        return 0, 0, err
    }
    if a.TenantID == nil { // a platform admin has no tenant
        return 0, 0, domain.ErrForbidden
    }
    return a.UserID, *a.TenantID, nil
}

// repository: the tenant predicate is NOT optional.
const q = `SELECT ... FROM chargebacks WHERE tenant_id = $1 AND id = $2`
```
Two safety nets fall out of this discipline:
- A cross-tenant fetch simply **returns no row** (the `WHERE` excludes it) → map to **404**. An attacker can't even tell whether the id exists in another tenant.
- On writes, `RowsAffected() == 0` means "not in *your* tenant" → 404, identical to "doesn't exist."

This is the manual Go equivalent of a framework's automatic "global scope." It's easy to forget one query, so centralize the predicate in repo helpers (`existsForTenant`, a shared `updateByID`) and make "did every query carry `tenant_id`?" a code-review checklist item.

### The overposting defense: trust the context, not the body
The most common multi-tenant bug is **mass assignment / overposting**: binding a request body straight onto a row lets a caller set fields they shouldn't — `tenant_id`, `user_id`, `is_admin`, `stage`. Two rules close it.

**1. Stamp server-controlled fields from the context**, discarding whatever the client sent:
```go
func (s *OrderService) Create(ctx context.Context, o *domain.Order) (*domain.Order, error) {
    _, tenantID, err := tenantActor(ctx)
    if err != nil {
        return nil, err
    }
    o.TenantID = tenantID                         // ✅ from the trusted context...
    o.SubmissionStatus = domain.SubmissionPending // ...ignoring whatever was in the body
    return s.repo.Create(ctx, o)
}
```
**2. Whitelist the columns a partial update may touch** — never feed arbitrary request keys into `SET`:
```go
var chargebackEditable = map[string]bool{"reason_code": true, "result": true /* ... */}
// updateByID drops any field not in the whitelist before building the SQL.
```
```go
// ❌ UPDATE ... SET tenant_id = $bodyTenant   // attacker moves a row across tenants
// ✅ tenant/owner come from ctx; only whitelisted business columns come from the body
```
This is why handlers use a **request DTO** ([21](21-rest-api.md)) instead of decoding onto the domain struct: the DTO can't even carry `tenant_id`, so there's nothing to overpost.

## Exercises
1. Split a single `RequireAuth` into two middlewares — `Authenticate` (verify token → load user → put a `*Auth` in the context) and `RequireRight(slug, next)` — and wire them on one route. Confirm a missing token is **401** and a missing right is **403**.
2. Create the four RBAC tables (`rights`, `roles`, `role_rights`, `user_roles`) and write the effective-rights JOIN. Seed two roles (Analyst, Manager) with different rights and verify each user resolves the right set.
3. Add a per-user rights **cache** with a TTL behind a `sync.RWMutex`; log DB hits and show the second request for a user hits the cache. Then `Flush` a user after changing their roles and show the change is immediate.
4. Add a `tenant_id` column to a table and enforce `WHERE tenant_id = $N` in every query. Write a test that logs in as tenant A and confirms fetching tenant B's id returns **404**, not the row.
5. Stage an overposting attack: POST a record with `"tenant_id": <other tenant>` in the body and prove `Create` ignores it (stamps from context). Then PATCH `stage` directly and show the column whitelist rejects it.
6. Add a `RequirePlatformAdmin` gate for an admin-only route and confirm `HasRight` short-circuits to `true` for a platform admin without listing every right.

## Best Practices & Pitfalls
- **Authenticate once at the edge; authorize per route.** Identity goes into the context; `RequireRight` reads it. Don't re-parse tokens or re-query the user deep in the stack.
- **Rights → roles → users, never rights → users.** Roles are the unit humans reason about; direct per-user permissions rot fast.
- **Every tenant-scoped query gets `WHERE tenant_id = $N`.** Treat a missing predicate as a security bug, not a style nit.
- **A cross-tenant miss is a 404, not a 403.** 403 confirms the id exists in another tenant; 404 leaks nothing.
- **Stamp trusted fields from the context.** `tenant_id`, `owner_id`, roles, status — set them server-side; ignore the body.
- **Whitelist updatable columns.** Map request keys through an allow-list before they reach `SET`; never build `UPDATE` from arbitrary JSON keys.
- **Cache rights with an explicit flush.** TTL bounds staleness; flush-on-change makes revocation immediate. Guard the map with `RWMutex` and release the lock before the DB load.
- **Pitfall — the forgotten predicate.** One repository method without `tenant_id` is a full cross-tenant leak. Centralize the predicate in a helper and review for it.
- **Pitfall — overposting via struct binding.** Decoding a body straight onto a domain struct and saving it lets callers set anything. Use a request DTO + server-side stamping + column whitelist.
- **Pitfall — platform-admin as a special case everywhere.** Bake the bypass into `HasRight` once, not into every handler.

## Checklist
- [ ] I can explain authentication (401) vs authorization (403) and which middleware owns each.
- [ ] I can put the caller's identity on the context with an unexported key and read it downstream.
- [ ] I can model RBAC with a rights catalog, roles, and two pivot tables, and write the effective-rights query.
- [ ] I can enforce a right at the route with `RequireRight` and bypass it for platform admins in one place.
- [ ] I can cache per-user rights with a TTL + flush behind a `RWMutex`.
- [ ] I can scope every query by `tenant_id` and return 404 (not 403) for cross-tenant access.
- [ ] I can defend against overposting by stamping trusted fields from context and whitelisting updatable columns.

## Resources
- OWASP — Mass Assignment / overposting cheat sheet: https://cheatsheetseries.owasp.org/cheatsheets/Mass_Assignment_Cheat_Sheet.html
- OWASP — Authorization cheat sheet: https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html
- RBAC (NIST model) overview: https://csrc.nist.gov/projects/role-based-access-control
- Worked example — the midigator capstone: `example-projects/midigator-dashboard-hard/` (`internal/reqctx`, `internal/middleware/auth.go`, `internal/rights`).
- Ties back to: [15 — Sync & Context](15-sync-context.md), [21 — REST API](21-rest-api.md), [25 — Architecture](25-architecture.md), [38 — Caching](38-caching-patterns.md).
- Next (Track C): [39 — Observability: Distributed Tracing](39-observability-tracing.md).
