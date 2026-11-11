# 46 · Roles & Permissions (RBAC)

`🟢 easy` · *Topics:* permissions as the unit and roles as bundles, tenant-owned custom roles, "grant only what you hold" (privilege-escalation guards), per-request lookup vs cached permission sets, revocation latency, permission granularity, deny-by-default enforcement, auditing role changes · *Builds on:* [09 Feature Flags](09-feature-flags.md) (snapshots and invalidation), [05 In-Memory Cache](05-kv-cache-ttl.md) (TTL and versioned keys), [22 Audit Log & Revision History](22-audit-log-revisions.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [45 Live In-App Notifications](45-live-notifications.md) · Next: [47 Privacy-Friendly Web Analytics](47-web-analytics.md)

---

> 🌱 **Seed (honeydolist-site):** ADR 0004: every check is by **permission slug**, never by role (`RequirePermission("equipment.write")`; "no handler knows the word *admin*"). Roles are bundles of permissions in `roles` / `role_permissions` / `user_roles` (migration 0016; about 60 distinct slugs across the migrations). `dashboard.access` (0017) gates even *serving* the admin bundle. The **Superadmin holds every permission by rule**: a trigger on `INSERT INTO permissions` grants each new one (0046); the Admin and Manager (0029, step 7h) get a new permission only when the migration says so, and tests pin their lists. A one-line **guard** (7h) stops escalation: nobody gives a role, edits a role or changes an account holding anything they do not hold themselves (`grantable`, `reachable` → `409 above-you`), nobody changes their own role (`409 own-role`, step 26.2), nobody deactivates themselves, and the last active Superadmin / last active user-manager cannot be removed. Dangerous verbs are separate permissions: `users.remove` (Superadmin only, 0083), `users.alerts` (0065), `system.write`, `integrations.manage`, `campaigns.send` apart from `campaigns.write`, `feedback.read` apart from the rest. Permissions are read by **one SQL query per request** (a session cookie, stored as a SHA-256, resolves to the user with `array_agg` of their roles and permissions), so a Superadmin's edit "takes effect on the Admin's next request". Roles are global today; the schema notes say they will be per business later (0054).
> **Here, at scale:** a B2B SaaS with 100k tenants, 5M users, 50k authorisation checks/s, and tenants that define their own roles.

## 1. The task

> "Design the roles-and-permissions system for a multi-tenant SaaS. Each customer company has its own users
> and roles; admins can create custom roles; every API call must be authorised in a few milliseconds."

Authorisation is the most rewritten part of every product. The easy answer is a column `users.role`. The real task: stay correct when roles become customer data, stay fast at 50k checks a second, and make sure nobody can promote themselves.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Decides cache vs per-request query. | 100k tenants, 5M users, 50k checks/s peak, heavy-tailed tenants (a few with 100k users). |
| Custom roles? | Roles become tenant data, not code. | Yes: three built-in roles per tenant plus custom ones. |
| One user in many tenants? | Membership, not user, carries the role. | Yes (consultants, agencies). |
| Multiple roles per user? | Union semantics. | Yes; effective permissions are the union. |
| Scopes? | "Edit my own tickets" vs "all tickets". | Two: `all` and `own` (created by / assigned to me). |
| Per-resource sharing (this one document)? | That is ReBAC, a different system. | Out of scope; extension. |
| How fast must a revocation bite? | Drives the cache design. | Suspension: immediately. Role edit: within ~5 s. |
| New permissions on deploy? | Existing custom roles must not silently gain power. | Owner role gets everything; built-ins get what the release says; custom roles get nothing. |
| Who may delegate? | The escalation rule. | Nobody can grant what they do not hold. |

## 3. Requirements

**Functional**
- A catalogue of permissions defined by the product; tenants compose roles from it.
- Assign roles to members; a member can hold several; suspend or remove members.
- Authorise each request by permission (and scope); show the UI only what is allowed (the server still decides).
- Guards: no self-promotion, no editing your own role, no granting what you lack, never zero owners.
- Every change to roles and memberships is audited and attributable.

**Non-functional**
- Check overhead under 1 ms p99 in the API process; a decision never blocks on a cross-region call.
- Revocation: suspension immediate; role/grant edits visible within ~5 s everywhere.
- Deny by default: a route with no declared permission must not be reachable.
- Tenant isolation: one tenant's roles can never grant anything in another tenant.

**Out of scope**
- SSO/SCIM provisioning, per-document sharing, attribute policies (time, IP, amount limits).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Users / tenants | 5M / 100k | given; ~50 users per tenant on average |
| Authorisation checks | 50k/s peak | ~20k requests/s × ~2.5 checks (route permission + data filters) |
| Seed's approach at this rate | ~20k resolve queries/s | one session+permissions query per request; at ~0.3 ms each ≈ 6 cores of database time, and database latency on every request |
| Catalogue | ~300 permissions | the seed has ~60; a SaaS grows a few per month |
| Roles | 600k | 100k tenants × ~6 (3 built-in + custom) |
| Grants | 24M rows, ~1.5 GB with indexes | 600k roles × ~40 permissions |
| Memberships | ~6.5M | 5M users × ~1.3 roles |
| Effective set per member | 40 B as a bitset | 300 bits → five `uint64`; all 5M members ≈ 200 MB |
| Policy edits | far below 100/s platform-wide | people clicking in settings |

What the numbers tell us: **the data is tiny** (a few GB, all of it fits in memory). The cost is the **lookup on every request**, and the interesting requirement is **revocation latency**. A bitset for the effective permission set makes both the cache and the "can this person grant that role?" test a handful of word operations.

## 5. Plan

1. Model: permission catalogue, tenant roles, memberships, scopes.
2. The escalation guards (who may grant what).
3. Where the check runs and how revocation reaches it.
4. What counts as a permission (granularity and naming).
5. Enforcement points and deny-by-default.
6. Audit and dangerous operations.

## 6. High-level design

```
 request ─► LB ─► API ── session middleware ──► Access resolver
                    │       cookie → session (hash) → (user, tenant)          ┌──────────── in-process LRU (30 s) ───────────┐
                    │                                                         ▼                                              │
                    │                      effective set = bitset {all, own} ◄── Redis  perm:{tenant}:{user}:{policy_version}  │
                    │                                  ▲ miss                                                                │
                    │                                  └── Postgres: memberships → roles → role_permissions  (one query) ◄───┘
                    ├── Require("tickets.write") middleware: bit test, 403 if absent
                    └── handler → data layer: WHERE tenant_id = $t  [AND assigned_to = $me when scope is 'own']
 settings API ──► roles / grants / memberships (Postgres)  ── same tx: bump tenants.policy_version, write audit row
```

One request: the session cookie resolves to a member of a tenant; the access resolver returns that member's permission bitset, from process memory or Redis or (on a miss) one SQL query; the route's `Require` tests one bit; the handler's queries always carry the tenant id and, for `own` scope, an owner filter. One role edit: written in a transaction that bumps the tenant's `policy_version` and writes the audit row; every cached set keyed with the old version simply stops being used.

## 7. API

```
GET  /v1/me/access?tenant=<id>          → { roles[], permissions: { "tickets.write": "all", "tickets.close": "own" }, policy_version }
GET  /v1/permissions                    → catalogue: slug, description, group, dangerous
GET  /v1/roles                          POST /v1/roles { name, permissions: [{slug, scope}] }          → 201 | 403 (grants you do not hold)
PUT  /v1/roles/{id}   If-Match: <version> { permissions: [...] }   → 200 | 403 above-you | 409 own-role | 409 system-role | 412
DELETE /v1/roles/{id}                   → 204 | 409 (members still hold it, or built-in)
PUT  /v1/members/{user}/roles { role_ids } → 200 | 403 above-you | 409 self | 409 last-owner
POST /v1/members/bulk { action: "suspend" | "restore" | "remove", ids | { all: true, filter } }
                                        → 200 { done: [..], refused: [{ id, reason }] }   (one refusal never fails the batch)
GET  /v1/audit?entity=role|member       → who changed what, when (task 22)
```

Bulk calls answer per person, as the seed's `POST /admin/users/bulk` does, and share one use case with the single-item call so the guards are written once.

## 8. Data model

```sql
CREATE TABLE permissions (
    id          SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- the bit position: append-only, never reused
    slug        TEXT NOT NULL UNIQUE,                               -- 'tickets.write'
    description TEXT NOT NULL,
    dangerous   BOOLEAN NOT NULL DEFAULT false                      -- drives audit alerts and step-up prompts
);

CREATE TABLE tenants (id BIGINT PRIMARY KEY, policy_version BIGINT NOT NULL DEFAULT 0);

CREATE TABLE roles (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    slug      TEXT NOT NULL,
    name      TEXT NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT false,       -- built-ins: never deletable
    holds_all BOOLEAN NOT NULL DEFAULT false,       -- the Owner rule: every permission, resolved, not stored
    version   INTEGER NOT NULL DEFAULT 0,           -- optimistic concurrency for the editor
    UNIQUE (tenant_id, slug)
);

CREATE TABLE role_permissions (
    role_id       BIGINT   NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id SMALLINT NOT NULL REFERENCES permissions(id),
    scope         TEXT NOT NULL DEFAULT 'all' CHECK (scope IN ('all','own')),
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE memberships (
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    user_id   BIGINT NOT NULL,
    status    TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('invited','active','suspended','removed')),
    PRIMARY KEY (tenant_id, user_id)
);
CREATE TABLE membership_roles (
    tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
    role_id   BIGINT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,   -- a role in use is not deleted
    PRIMARY KEY (tenant_id, user_id, role_id),
    FOREIGN KEY (tenant_id, user_id) REFERENCES memberships
);
CREATE INDEX membership_roles_by_role ON membership_roles (role_id);
```

The resolve query is one round trip: `SELECT r.holds_all, rp.permission_id, rp.scope FROM membership_roles mr JOIN roles r ON r.id = mr.role_id LEFT JOIN role_permissions rp ON rp.role_id = r.id JOIN memberships m ON m.tenant_id = mr.tenant_id AND m.user_id = mr.user_id WHERE mr.tenant_id = $1 AND mr.user_id = $2 AND m.status = 'active'`. Row-level security on tenant-owned tables (`USING (tenant_id = current_setting('app.tenant_id')::bigint)`) is a second line of defence under the application's own filters.

## 9. Deep dives

### 9.1 The model and the escalation guards

| Option | Pros | Cons |
|---|---|---|
| `users.role` string | trivial | no custom roles; every new rule is a code change |
| **Permissions as the unit, roles as tenant-owned bundles** (RBAC tables) | custom roles are data; code never names a role; small and fast | a catalogue to curate; no per-resource sharing |
| Policy engine (ABAC: OPA, Cedar) | conditions on attributes (amount, time, IP) | heavier to operate and explain; overkill when the question is "may this role do this verb" |
| Relationship-based (ReBAC, Zanzibar-style) | "Alice may view *this* document" | a graph store and a different mental model (extension) |

**Choice and why: RBAC with permissions as the unit**, as in the seed, with a two-value scope (`all`, `own`) because "edit only my own" is the one conditional nearly every product needs (the seed's alert preferences have the same `all | mine` idea). Everything else conditional goes to an engine later, behind the same `Require` call.

**The guards** are the part to get exactly right, and they are all one subset test:

```go
type Set [5]uint64 // 320 permissions: bit positions come from permissions.id and are never reused

func (s Set) Has(p int) bool { return s[p>>6]&(1<<(p&63)) != 0 }

// Covers reports whether s holds everything o holds.
func (s Set) Covers(o Set) bool {
	for i := range s {
		if o[i]&^s[i] != 0 {
			return false
		}
	}
	return true
}
```

- **Grant** (`grantable`): you may put a permission in a role only if `actor.Covers(thatPermission)`; you may assign a role only if `actor.Covers(role)`.
- **Reach** (`reachable`, `403 above-you`): you may touch a role or a member only if `actor.Covers(theirEffectiveSet)`. An Admin therefore never edits an Owner, never creates one, never hands out a permission it lacks.
- **Own role** (`409 own-role`): never edit the role you hold (otherwise an Admin can tick every box on the Admin role). Never change your own roles or suspend yourself.
- **Last owner**: refuse suspending, removing or demoting the last active holder of `holds_all`, and the last active member who can manage members. In a bulk call, count as the batch proceeds so two removals in one request cannot empty the set (the seed counts as it goes).
- **Owner by rule, not by rows:** the seed grants every new permission to the Superadmin with a trigger. With 100k tenants that would insert 100k rows per new permission, so the Owner role carries `holds_all` and resolves to the full catalogue. Built-in non-owner roles get a new permission only if the release says so (a test pins each list); **custom roles never gain anything automatically**: deny by default.

### 9.2 Where the check runs, and how revocation arrives

| Option | Pros | Cons |
|---|---|---|
| **Query per request** (the seed) | revocation is instant; trivially correct; nothing to invalidate | at 20k requests/s it is the most repeated query in the system, a latency tax on every call, and a coupling to the database's p99 |
| Self-contained token (JWT with the permission bitset, ~52 base64 characters) | no lookup at all | revocation takes the token's lifetime (minutes) unless you add a deny-list or a version check, which brings back a lookup |
| **Server-side session + cached effective set keyed by a tenant `policy_version`** | microsecond checks; revocation within seconds; suspension immediate by deleting the session | a cache and a version to keep honest |

**Choice and why: server-side sessions, an effective-set cache keyed by `(tenant, user, policy_version)`.** Every write that changes a role, a grant or a membership runs `UPDATE tenants SET policy_version = policy_version + 1` in the same transaction. The resolver reads the tenant's current version from Redis (cached in-process for 1 s), and the permission set from an in-process LRU (30 s) or Redis keyed with that version, falling back to the single resolve query on a miss. Why a **tenant-level** version and not a version per role: a role held by 100k members would otherwise need 100k invalidations; one counter bump makes all of them re-resolve lazily on their next request. That over-invalidates (a small edit refreshes a whole tenant) but policy edits are rare, and a refresh is one query.

Revocation targets: a **role edit** is visible everywhere within about the version-cache TTL (1–5 s); a **suspension or removal** deletes the member's sessions for that tenant, so the very next request fails, as the seed's "turn off sign-in ends their sessions at once". Authentication itself (passwords, two-step sign-in, session lists) is lesson 56's job; this task only consumes "who, in which tenant".

### 9.3 What counts as a permission

A permission is a **verb on a noun that some role must be able to hold without another**. The seed's slugs show the habits worth copying:

- **Split reading, writing and the dangerous verb.** `campaigns.write` (draft) is separate from `campaigns.send` (press the button); a Manager drafts but does not send. `users.manage` is separate from `users.remove` (Superadmin only) and from `users.alerts` (choose what other people are told of).
- **Privacy gets its own slug.** `feedback.read` is separate so that staff do not see it by default.
- **Server-protection verbs are separate**: `system.read`, `system.write`, `integrations.manage` (where the credentials are).
- **A permission for an unusual power**, not a role: `records.take_over` (take a record from someone editing it).
- **No per-field and no per-row permissions** in the catalogue; that is scope or an attribute engine.

Rules for keeping ~300 slugs usable: `resource.action` naming, stable forever (grants point at the id; never rename, deprecate and hide), a one-line description shown in the role editor (grouped by area), and a `dangerous` flag that triggers an audit alert and step-up authentication. A product team adds ~5 a month, and each addition forces the question "who gets it by default?", answered in the release and pinned by tests.

### 9.4 Enforcement points and deny-by-default

```go
func Require(perm int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acc := AccessFrom(r.Context()) // zero value holds nothing
			if !acc.All.Has(perm) && !acc.Own.Has(perm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

| Layer | What it does |
|---|---|
| Route middleware | `Require(slug)`: a bit test. **A test walks the router and fails the build** if any non-public route has no declared permission (deny by default). |
| Data layer | Every query carries `tenant_id`; scope `own` adds `AND assigned_to = $me`. A `Require` for an `own`-only holder must be followed by the owner filter or it is a bug. |
| Asset layer | The seed does not even *serve* the admin bundle to a session without `dashboard.access`; a user who cannot use the app never downloads it. |
| UI | Hides what the person cannot do. A convenience, never a control: the seed's flows hit hidden endpoints by hand and expect `403`. |
| Database | Row-level security on tenant tables as a backstop against a missing `WHERE`. |

**Choice and why: check in middleware by slug, filter in the data layer, tests that enumerate routes.** The most common real-world authorisation bug is a forgotten check on a new endpoint, and an enumerating test is the only defence that does not rely on memory.

### 9.5 Audit and dangerous operations

Every role create/edit/delete, grant change, membership change and bulk action writes an audit row in the same transaction: actor, target, **before and after as permission-slug lists** (not ids, so history survives renames), time and request id ([22](22-audit-log-revisions.md)). Others' changes show on the target's own history ("Last changed by Anna, 2 h ago"). A change that adds a `dangerous` permission to any role notifies the tenant's owners. Removing a person goes through a recoverable state (the seed: 30 days in a Trash, the email freed at once by a unique index on live rows, restored with sign-in off), and history keeps the name and email written into the audit row. Optional hardening: step-up re-authentication within the last 10 minutes for `dangerous` actions.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the resolve query on cold caches after a deploy (a thundering herd). Pre-warm the hot tenants, coalesce concurrent misses for the same key with singleflight, and keep the Redis hit rate observable.
- **Huge tenant:** a 100k-member tenant editing a role makes 100k re-resolves in the next minute. That is ~1.7k queries/s, and spread by request arrival it is tolerable; if not, rate-limit role edits per tenant ([03](03-rate-limiter.md)).
- **Redis down:** fall back to the in-process LRU, then the database. Never fail open; a denied request is a cost, a wrongly allowed one is an incident.
- **Version skew:** a node with a stale tenant version serves a stale set for at most the version-cache TTL (1–5 s). Say so in the SLA instead of pretending to be instant.
- **Catalogue deploys:** bit positions are append-only so old cached bitsets stay valid; a cache key includes a catalogue version to be safe.
- **Cross-tenant bugs:** the highest-severity class. Row-level security plus tests that run every endpoint as a user from tenant B against tenant A's ids.
- **Policy as data corruption:** a custom role edit must be validated server-side (only known slugs, only grantable ones) before commit.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Permissions as the unit | role checks in code | custom roles are data; no code change per role | a catalogue to curate |
| Bitset effective sets | lists of slugs | 40 B per member; O(1) checks; one-pass subset test | stable bit positions to maintain |
| Session + versioned cache | query per request | microsecond checks | revocation in seconds, not instantly (except suspension) |
| Tenant-level `policy_version` | per-role versions | one write invalidates a 100k-member tenant | over-invalidation (cheap) |
| `holds_all` on Owner | a trigger granting rows to every tenant | no 100k-row inserts per new permission | a special case in the resolver |
| Custom roles get no new permissions | auto-grant | deny by default | admins must opt in |
| Separate `dangerous` permissions | one `manage` per area | delegable without handing over the keys | a longer catalogue |

## 12. Check yourself

1. Why does application code check permissions and never role names?
2. A custom role holds 30 permissions; its holder tries to assign a role with 35 to someone. What decides, and what does it cost to compute?
3. A tenant admin edits a role held by 100,000 members. How do all of them lose the permission without 100,000 writes?
4. Why does the Owner role carry a `holds_all` flag instead of a trigger granting rows on every new permission?
5. What stops a forgotten check on a brand-new endpoint?

<details><summary>Answers</summary>

1. Roles are tenant data that change at runtime (custom roles, renames, new built-ins). Code that names a role must be redeployed for each new role and silently breaks when a role is edited; code that names a permission stays valid for any role that holds it, and the role editor can show exactly what each role can do.
2. The subset rule: you may assign a role only if your own effective set covers it. With bitsets that is `role &^ actor == 0` over five 64-bit words: a few nanoseconds. 35 permissions against 30 held (assuming they are not all within the actor's set) fails with `403 above-you`.
3. The edit runs in one transaction that updates the role and bumps the tenant's `policy_version`. Cached sets are keyed by that version, so every member's next request misses, re-resolves with one query and gets the new set. Nothing is written per member; revocation lands within the version-cache TTL (1–5 s).
4. A trigger would insert one row per tenant owner for every new permission (100k rows per release) and has to be kept consistent when tenants are created. A flag resolved at read time costs nothing per release and cannot drift.
5. Deny by default: the route table is enumerated by a test (or at startup) and any non-public route without a declared permission fails the build; the data layer always filters by tenant, and row-level security catches the rest.

</details>

## 13. In Go

- Related lessons: [43 Authorization, RBAC & multitenancy](../../43-authorization-rbac-multitenancy.md) (the model, tenant scoping), [56 Authentication & sessions](../../56-authentication-sessions.md) (opaque sessions, cookies, two-step sign-in), [38 Caching](../../38-caching-patterns.md) (versioned keys, singleflight), [59 Production DB patterns](../../59-production-database-patterns.md) (optimistic `version`, `FOR UPDATE`, row-level security), [57 Web security](../../57-web-security.md) (IDOR and tenant-isolation bugs), [20 HTTP server](../../20-http-server.md) (middleware chains).
- Context plumbing: the session middleware puts an `Access` value (`All`, `Own` sets, tenant, user) into `context.Context` with an unexported key; `AccessFrom` returns the zero value (holds nothing) if absent, so a forgotten middleware denies instead of allowing.
- Route-walk test: use your router's walk function (`chi.Walk`) and assert every non-public route's handler chain contains `Require`.
- `golang.org/x/sync/singleflight` for coalescing cold misses; `math/bits` if you want popcounts for diffs between old and new sets in the audit row.
- Packages: `pgx`, `go-redis`, `chi`, `singleflight`.

## 14. Extensions

- **ReBAC (Zanzibar-style):** "viewer of this folder" relations, group membership, inheritance; where does it plug in next to the permission check?
- **Attribute conditions** (amount under 1,000, office hours, IP range) through a policy engine such as OPA or Cedar behind the same `Require`.
- **SSO / SCIM:** map identity-provider groups to roles; who wins when the group and a manual assignment disagree?
- **Temporary elevation and break-glass:** a role with an expiry, an approval, and loud auditing.
- **Service accounts and API keys** with scoped permissions that can never exceed their creator's.
- **Step-up authentication** for `dangerous` permissions (recent re-login or a one-time code).
