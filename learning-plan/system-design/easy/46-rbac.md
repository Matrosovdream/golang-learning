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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API nodes (Go; the access resolver runs inside them) | 20k requests/s ⇒ 50k checks/s (§4) | CPU | the authorise path: 50k bit tests × ~0.2 µs + 20k × ~10 µs (in-process LRU, Redis client on a miss) ≈ 0.2 vCPU, negligible. The tier is sized by the product's own work: assume 1.5 ms per request ⇒ 30 vCPU; N = ceil(30 ÷ (0.8 × 8 × ⅔ = 4.27)) = ceil(7.03) = 8 → 9 | `c7g.2xlarge` (8 vCPU / 16 GiB) on ECS/EKS | 9, three per zone | CPU-bound; the LRU holds ~200k members × ~200 B = 40 MB per node (30 s TTL, assumption), so memory is irrelevant and `c` beats `m`. These nodes are the product's: RBAC adds about 1 % to them |
| Redis (permission sets, tenant versions, sessions) | ~12k ops/s (assumption: 10 % LRU misses of 20k req/s = 2k, plus ~10k tenant-version reads with a 1 s in-process cache) | memory (small), then single-core ops | 5M bitsets × 40 B = 200 MB (§4); with key and per-key overhead ≈ 250 B ⇒ 1.25 GB; stale versions until TTL and sessions bring it to ~3 GB (assumption); `maxmemory` ≤ 70 % ⇒ ≥ 4.3 GiB; 12k ops/s is far under one core's ~100k | `cache.m7g.large` (2 vCPU / 8 GiB, ≈ $120) | 1 primary + 1 replica | the smallest node that holds it: `r` would add 8 GiB nobody uses. If Redis is down, nodes fall back to the LRU, then Postgres, and never fail open (§10) |
| Postgres (roles, grants, memberships, audit) | 24M grants ≈ 1.5 GB, 6.5M memberships, 600k roles (§4); edits < 100/s; resolve queries after misses ≈ 200/s (1 % of requests, assumption) | memory (small); IOPS trivial | all RBAC data ≈ 3 GB (grants 1.5 GB, memberships ~0.7 GB, indexes, audit): fits in RAM several times over. 200/s × 0.3 ms = 0.06 vCPU; ≤ 100 edit commits/s ≈ 300 IOPS, inside the 3,000 included. Worst case (Redis down and every LRU cold): 20k × 0.3 ms = 6 vCPU, far more than 2 | `db.m7g.large` (2 vCPU / 8 GiB, ≈ $120), RDS Multi-AZ | 1 primary + synchronous standby | the smallest node; in practice these tables sit in the product's main database and add ~3 GB. The cold-cache worst case is solved with singleflight and a warm Redis, not a bigger primary |

"A few small instances, for availability not load": the authorisation data is a few GB and the check is a bit test. The only large number is the API tier it rides on.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Redis: 2 × `cache.m7g.large` (2 × $120) | $240 |
| Postgres: `db.m7g.large` ($60 × 2 managed), Multi-AZ doubles | $240 |
| gp3 100 GB (= $8, ×2 for the standby) | $20 |
| *Context, not RBAC's:* API tier, 9 × `c7g.2xlarge` (9 × $212) | $1,910 |
| **Total** | **≈ $2,410, of which RBAC's own pieces are ≈ $500** (≈ $1,570 with a 1-year Savings Plan on compute, ≈ $330 for RBAC's own) |

- **Biggest line and why:** the API tier (79 %), which is the product's, not RBAC's. What the design buys is visible in the rest: the seed's query per request would be 20k resolves/s × 0.3 ms = 6 vCPU, and at ≤ 60 % busy that needs ~10 vCPU, a `db.r7g.4xlarge` Multi-AZ at ≈ $2,480 plus a database round trip on every request; the cache design does it with Redis and a small Postgres for ≈ $500.
- **What changes at 10×:** 200k req/s ⇒ 300 vCPU ⇒ N = ceil(300 ÷ 4.27) = 71 → 72 × `c7g.2xlarge` (≈ $15k). Redis at ~120k ops/s is past one core and ~30 GB of data needs ~43 GiB at the 70 % rule, so cluster mode with 3 shards of `cache.r7g.xlarge` (3 × 22 GiB usable), or a longer in-process TTL. Postgres still holds ~30 GB and takes ~2k resolves/s: unchanged; what hurts first is the cold-cache herd after a deploy, so pre-warm hot tenants and coalesce misses.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Tiny and relational: 600k roles, 24M grants (~1.5 GB with indexes), ~6.5M memberships; ~3 GB all told, all of it in RAM (sections 4, 15). Writes are people clicking in settings, far below 100/s.
- The invariants are relational: a role in use cannot be deleted (`ON DELETE RESTRICT`), grants die with their role (`CASCADE`), `UNIQUE (tenant_id, slug)`, a composite foreign key from `membership_roles` to `memberships`, and a role edit, the `policy_version` bump and the audit row commit together (sections 8, 9.2, 9.5).
- The read that matters, 50k checks/s, is not a database read: bit tests in process, with Redis behind them. The database sees ~200 resolve queries/s (1 % of requests, section 15's assumption), a three-table join on indexed keys.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | foreign keys (including the composite one), uniqueness, `CHECK (scope IN …)` and a multi-table transaction keep "no orphan grants, no deleting a role in use" inside the database; row-level security (`USING (tenant_id = current_setting('app.tenant_id')::bigint)`) is a second wall under a forgotten `WHERE` (9.4, section 10); `SELECT … FOR UPDATE` serialises the last-owner check |
| MySQL (RDS / Aurora) | ✔ works, one loss | everything in section 8 is portable (composite foreign keys, `CHECK` since 8.0.16, joins, the optimistic `version`), and InnoDB's clustered keys `(role_id, permission_id)` and `(tenant_id, user_id, role_id)` keep a role's grants and a member's roles contiguous, which suits the resolve query. The loss: no row-level security, so the backstop under the application's filters is gone |
| DynamoDB | ✔ as a derived store for effective sets, ✖ as the record | the cached bitset keyed by `(tenant, user, policy_version)` is a pure key lookup (what Redis does now). The record needs rules that span items: role in use, unique slug, last owner, and one edit touching grants, `policy_version` and audit together. Transactions stop at 100 items, there are no foreign keys, and "who holds this role" needs a GSI |
| Redis (ElastiCache) | ✔ as the cache (section 15), ✖ as the record | `perm:{tenant}:{user}:{policy_version}` entries are derived and rebuilt by one query; with Redis down, nodes fall back to the in-process LRU, then Postgres, and never fail open (section 10) |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.m7g.large`, section 15), plus Redis (`cache.m7g.large`, primary and replica) as the derived cache. Relational is the obvious fit because authorisation is a web of small tables with integrity rules (who holds what, who may grant what), the data is 3 GB and the write rate trivial: nothing in the load asks for another kind of store, and everything in the correctness asks for constraints and transactions. Postgres is the system of record; the bitset sets in Redis and the LRU are derived. In practice these tables live in the product's main database and add ~3 GB; the standalone node is the sizing. What decides Postgres over MySQL is the row-level-security backstop and transactional guards, not speed.

**If it were MySQL instead:** one material change, row-level security, which has no counterpart: the backstop becomes the data layer's `tenant_id` filters, per-tenant views at best, and the cross-tenant tests of section 10 (run every endpoint as a user of tenant B against tenant A's ids), which are then the only safety net. Smaller changes: `SMALLINT GENERATED ALWAYS AS IDENTITY` becomes `SMALLINT UNSIGNED AUTO_INCREMENT` (bit positions stay append-only; gaps are fine), and the last-owner `SELECT … FOR UPDATE` takes next-key locks under the default `REPEATABLE READ`, harmless at under 100 edits/s (use `READ COMMITTED` if deadlocks appear). The two engines are otherwise interchangeable here.

**What would flip the choice:** not scale: at 10× section 15 still has ~30 GB and ~2k resolves/s on the same single node, and what hurts first is the cold-cache herd after a deploy. Per-resource sharing (ReBAC, "Alice may view this document", section 14) is the real trigger, because relation tuples grow per object and not per role; at hundreds of millions of tuples (a rule of thumb), or for "list everything Alice can see", use a dedicated relationship store. A multi-region active-active requirement with ~5 s revocation everywhere would put the derived layer on DynamoDB global tables while the record stays relational in one region.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 10M tenants, 500M users, 5M checks/s on 2M req/s; 2.4B grants (~150 GB), 650M memberships, effective sets 20 GB (≈ 300 GB in Redis with keys, versions and sessions); edits below 10k/s; ~705 `c7g.2xlarge` of API | the lookup tier, not the check: bit tests and the LRU cost only ~21 vCPU, but Redis would see ~1.2M ops/s (12× one core's ~100k) and 300 GB (3.3× a 128 GiB node's 70 %). Behind it the cold-cache herd: a deploy or failover sends up to 2M resolves/s × 0.3 ms = 600 vCPU at Postgres, ~37 primaries' worth | Redis Cluster of ~14 shards (`cache.r7g.xlarge`), keys hash-tagged by `{tenant}` so a tenant's sets and its version share a shard; consistent-hash requests by `tenant_id` so each node's 30 s LRU stays warm (random routing over ~700 nodes would make hits rare); Postgres stays one primary with ~4 read replicas for resolves; singleflight and stale-while-revalidate so a restart never sends the herd to the database. Give up: a role edit can serve the old set a few seconds longer (suspension stays immediate: delete the session), and one huge tenant becomes a hot node | ≈ $165k/month bottom-up (API ≈ $150k, RBAC's own ≈ $15k); linear would say $240k, which overstates because the baseline sits on minimum-size nodes |
| **1,000×** | 100M tenants, 5B users (every internet user holds a seat: a thought experiment), 50M checks/s on 20M req/s; 24B grants (~1.5 TB), 6.5B memberships, ≈ 3 TB of cached sets; edits up to 100k/s; ~7,000 API nodes | one policy store in one region: edits up to 100k/s are 10× a primary's ~10k ceiling, 1.5 TB of grants plus audit sits at "a few TB comfortable", one Redis cluster would be ~140 shards, and with users on every continent a miss costs 100–150 ms across an ocean, which breaks "a decision never blocks on a cross-region call" | **cells**: ~100, each ≈ the 10× design (1M tenants, 72 API nodes, 3 Redis shards, a Postgres primary with replicas) in the region nearest its tenants; a tenant's roles, sessions and cached sets live only in its home cell. A role edit publishes (tenant, version) on a log and nodes refresh only that tenant on its next request; each node keeps a last-known snapshot, so the settings API and policy database can be down and checks still run (static stability; an unknown tenant is denied). A small eventually consistent `user → tenants` directory feeds the tenant switcher. Give up: cross-tenant queries, the ~5 s revocation promise outside the home cell, 100 cells to operate | ≈ $2.4M/month (linear) |
| **10,000×** | 1B tenants, 50B users (more than six per person alive: a thought experiment), 500M checks/s on 200M req/s; 240B grants (~15 TB), ≈ 30 TB of cached sets; ~70,000 API nodes | the lookup costs more than the answer: the 500M bit tests are ~100 vCPU, but fetching a set per request means ~120M Redis ops/s (≈ 1,200 cores) over ~1,400 shards, and the bill is ≈ $24M a month, ~80 % of it the product's own API tier | change the problem: make the check need no lookup. A short-lived signed token (≤ 60 s) carries the member's 40 B bitset and the tenant's `policy_version`; nodes and edges verify it against a cached tenant-version map, so lookups fall from every request to one per active session a minute. Suspensions go on a pushed deny-list holding only users suspended in the last minute. Roles are stored content-addressed (identical bitsets once; most custom roles are copies of built-ins, an assumption) and custom roles per tenant are capped (a product limit). Give up: role-edit revocation of ~5 s becomes up to 60 s, immediate suspension depends on the deny-list push, unlimited role sprawl | ≈ $24M/month (linear; the API tier is ~80 %) |

**What stays the same at every scale:** permissions, not roles, are the unit, with append-only bit positions (a 40 B bitset per member); a tenant-level `policy_version` that invalidates lazily; the guards as one subset test (`Covers`); and deny by default (no declared permission, no loaded policy or no token means no access, never fail open).

**Say it in an interview:** "At 100× the check itself is still free (about 21 vCPU), but Redis needs ~14 shards and a deploy could send 2M resolves a second to Postgres, so I cluster Redis by tenant, route by tenant so the LRU stays warm, and add replicas and singleflight. At 1,000× one region's policy store breaks (100k edits a second, ~140 Redis shards, cross-ocean misses), so I split into ~100 cells by tenant that keep checking from a snapshot when the control plane is down. At 10,000× the lookup tier costs more than the answer, so the check moves into a short-lived signed token and revocation becomes eventual."
