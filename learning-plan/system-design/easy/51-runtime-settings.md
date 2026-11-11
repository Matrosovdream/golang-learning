# 51 · Runtime Settings & Timed Kill Switches

`🟢 easy` · *Topics:* a typed settings registry, in-process snapshots with NOTIFY + change-feed polling, read-your-writes without sticky sessions, switches that revert themselves (`off_until`), precedence (environment > tenant > plan > default) with the source shown, risk classes, audit and optimistic concurrency · *Builds on:* [09 Feature Flags](09-feature-flags.md) (do not re-learn evaluation and rollout: this is operator config, not user targeting), [05 In-Memory Cache](05-kv-cache-ttl.md), [42 Integration Credentials Vault](42-credentials-vault.md) (secrets are *not* settings), [22 Audit Log](22-audit-log-revisions.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [50 Alerting Engine](50-alerting-engine.md) · Next: [52 Configurable Forms](52-configurable-forms.md)

---

> 🌱 **Seed (honeydolist-site):** one `settings` table (`key text PRIMARY KEY, value jsonb, updated_by, updated_at`, tables.md §3). **A key with no row has its
> default**, so a new setting needs no migration; a **public allowlist in code** (`settingsmodel.PublicKeys`) decides which keys may reach the page head; private groups are saved on
> their own path with **a version of their own**, so saving one group never makes another form stale, and they are not snapshotted into revisions. Operator switches live there. **The site open or
> Under construction** (`site.closed`, migration 0049, step 23.2): every page answers `503` + `Retry-After` + `noindex`, forms and checkout refuse with `503`, signed-in staff still see the real site, and
> *a status that cannot be read leaves the site open*. **The cache** (`cache.off_until`, migration 0052, step 25.3): absent = on, an RFC 3339 time = off until then, `forever` = off until someone turns it on; "Turn off for 15 min, 1 hour,
> until I turn it on"; a *Cache off* pill in the top bar; flushing needs `cache.flush`, turning it off needs the stricter `system.write`. **Rate limits** (`limits.*`, migration 0050, step 24): read on every request from memory,
> refreshed within 5 s, a save replaces it at once; `API_RATE_LIMIT` in the environment still wins and the page locks the field; a real change is an audit row with before and after, an unchanged save is not recorded. ADR 0009: *the
> environment first, then the database*, with the page showing where each value comes from. `config.Live` keeps an in-process snapshot with a generation counter, Postgres `NOTIFY`, a 1-minute fallback TTL, stale-while-revalidate, last-known-good when the database fails, and a 10 s retry.
> **Here, at scale:** 5,000 tenants × 300 keys read on every request by 200 pods (about 1M reads/s), edits visible in under 5 s, risky switches that revert themselves, every change audited.

## 1. The task

> "Design runtime settings for a multi-tenant product. Operators and tenant admins change limits, texts and switches without a
> deploy; every request reads them; a change takes effect within seconds; risky switches turn themselves back; every change is audited."

It looks like a key-value table. The design questions are the ones around it: what a missing row means, how 200 processes learn of a change,
what happens when the database is gone, who may flip what, and how a "temporary" switch avoids becoming permanent.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Setting, flag, or secret? | The boundary decides the design. | A setting is a value or switch the same for everyone in a tenant. Per-user targeting and rollouts are flags ([09](09-feature-flags.md)); credentials are never settings ([42](42-credentials-vault.md)). |
| Read path? | Dominates the design. | Every request reads several keys; must not touch the network. |
| How fast must a change land? | Push vs poll. | p99 under 5 s everywhere; a kill switch as fast as the channel allows. |
| Layers? | Precedence rules. | Environment pin > tenant value > plan default > code default. |
| Who may change what? | Blast radius differs by key. | Risk classes with different permissions. |
| Temporary switches? | The `off_until` design. | Yes: "off for 15 min / 1 h / until I turn it on". |
| If the settings database is unreachable? | Fail-open or closed, per key. | Serve the last known good; boot from a baked-in default snapshot; per-key fail direction. |
| Audit and rollback? | Compliance and incident response. | Every change: who, when, before, after, reason; revert is a new write. |
| Scale? | Memory and refresh design. | 5,000 tenants × 300 keys, 200 pods, 1M reads/s, ~10k edits/day. |

## 3. Requirements

**Functional**
- Read effective values by `(tenant, key)`, with the **source** of each (environment, tenant, plan, default).
- Typed, validated writes; unknown keys and out-of-range values are rejected.
- Timed switches that expire on their own; "until I turn it on" is an explicit choice.
- A public allowlist of keys that may be inlined into pages; everything else stays private.
- History of every change; revert to an earlier value.

**Non-functional**
- Reads: in-process, no I/O, ~100 ns. Propagation p99 under 5 s. Stay up with the database down.
- No change is lost; concurrent edits to one group are detected ("someone else changed this").
- A bad value must be cheap to find and revert.

**Out of scope**
- Per-user targeting and gradual rollout ([09](09-feature-flags.md)); secrets ([42](42-credentials-vault.md)); a settings UI.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Possible keys / stored rows | 1.5M / ~150k | 5,000 × 300; ~10% overridden (assumption), since *no row means default* |
| Table size | ~30 MB | 150k rows × ~200 B including the index |
| Effective config per tenant | ~30 KB | 300 keys × ~100 B |
| All tenants resident in one pod | 150 MB | 5,000 × 30 KB. At 500k tenants it would be 15 GB, so load active tenants lazily (LRU) |
| Reads | 1M/s total, 5k/s per pod | 1,000,000 ÷ 200; a pointer load and map lookup ≈ 100 ns, so 5,000 × 100 ns = 0.5 ms of CPU per second: 0.05% of a core |
| Writes | ~0.12/s average | 5,000 tenants × 2 edits/day = 10k ÷ 86,400. A bulk platform change touches 5,000 rows at once |
| Fallback poll | 40 queries/s | 200 pods ÷ 5 s, each reading a small change feed |
| `NOTIFY` listeners | 200 connections | one session-level connection per pod |
| Audit | ~5 MB/day | 10k changes × ~500 B |

What the numbers tell us: reads are the whole load and must be local memory, since 1M reads/s would need ten Redis shards or an impossible Postgres. *Refresh* is the
cheap part (40 small queries a second) **unless a bulk change makes 200 pods reload thousands of tenants at once**; that is the case to design for (9.2).

## 5. Plan

1. A registry that defines every key (type, default, range, public?, permission, risk) and a table that stores only overrides.
2. Propagation: immutable snapshots, a generation counter, `NOTIFY`, a change-feed poll as the net, last-known-good.
3. Timed switches as *data* (a timestamp), not as scheduled jobs.
4. Precedence and the effective-value-with-source resolution.
5. Permissions by risk class, audit, optimistic concurrency.

## 6. High-level design

```
 admin UI/API ──PUT──► Settings service ──one txn──► Postgres: setting_values · settings_changes · settings_audit
                                                        │ pg_notify('settings', tenant_id)            ▲
                                                        ▼                                             │ poll fallback every 5 s
                  ┌─────────────────── 200 API pods ────────────────────────────────────────────────┘
                  │  Live: atomic pointer → immutable snapshot { version, tenants[ key → value ] }
                  │  Get(tenant, key) → (value, source)         ← every request, no I/O
                  └─ layers: ENV pin  >  tenant value  >  plan default  >  code default   (registry compiled into the binary)
 page head / bootstrap ◄── public allowlist only        top bar pill + alert (task 50) ◄── "a switch is not at its default"
```

A save is one transaction (value, change-feed row, audit row), with `NOTIFY` delivered on commit. Pods swap whole snapshots; readers never see a half-updated config.

## 7. API

```
GET  /v1/tenants/{t}/settings/{group}   → 200 { version, values: { key: { value, source: "env|tenant|plan|default", locked, risk } } }
PUT  /v1/tenants/{t}/settings/{group}   If-Match: "<version>"  { changes: { key: value | null } }
                                        → 200 { version, values } | 409 stale (what changed) | 422 per-key errors | 403 key not yours to change
PUT  /v1/tenants/{t}/switches/{key}     { off_until: "2026-11-11T15:00:00Z" | "forever" | null }     idempotent, absolute time
GET  /v1/tenants/{t}/settings/{group}/history?after=…  → 200 page: who, when, key, before, after, reason
GET  /v1/public-settings                → 200 allowlisted keys only (inlined into the page head by the server)
GET  /internal/settings/changes?after_seq=N&limit=1000   → pods' fallback poll (tenant ids, not values)
```

A switch is set to an **absolute time**, never "toggle" or "add 15 minutes": a retried request must give the same result (see [16](16-public-form-intake.md) for the same rule on forms).

## 8. Data model

```sql
CREATE TABLE setting_values (                    -- ONLY overrides. A key with no row means "the default": no migration for a new key.
  tenant_id  bigint NOT NULL,
  key        text   NOT NULL,                    -- must exist in the registry (code); unknown keys are rejected by the service
  value      jsonb  NOT NULL,
  updated_by bigint, updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, key)
);
CREATE TABLE settings_changes (                  -- the change feed pods poll; short retention (7 days)
  seq bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id bigint NOT NULL, key text NOT NULL, changed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE settings_audit (                    -- partition by month; keep 2 years
  id bigserial, tenant_id bigint NOT NULL, key text NOT NULL,
  before jsonb, after jsonb, actor_id bigint NOT NULL, reason text, request_id text,
  at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (at, id)
) PARTITION BY RANGE (at);

BEGIN;
INSERT INTO setting_values (tenant_id, key, value, updated_by) VALUES ($1, $2, $3, $4)
  ON CONFLICT (tenant_id, key) DO UPDATE
     SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
   WHERE setting_values.value IS DISTINCT FROM EXCLUDED.value;     -- an unchanged save writes nothing: no history row, no version bump
-- only when a row was written (the service checks the row count): audit + change feed + notify, in the same transaction
INSERT INTO settings_audit (tenant_id, key, before, after, actor_id, reason) VALUES ($1, $2, $5, $3, $4, $6);
INSERT INTO settings_changes (tenant_id, key) VALUES ($1, $2);
SELECT pg_notify('settings', $1::text);                            -- delivered when the transaction commits
COMMIT;
```

The **registry** (types, defaults, ranges, permissions) is code, compiled into every binary, so a pod can run with *no* database; the table holds only what someone changed.

## 9. Deep dives

### 9.1 Storage and typing: a code registry over key/value rows

| Option | Pros | Cons |
|---|---|---|
| One JSON blob per tenant | one read, trivial snapshot | whole-blob write conflicts; no per-key audit or permission; no per-key `updated_at` |
| **Key/value rows (`jsonb`) + a registry in code** | per-key audit, permission and timestamps; new key needs no migration; absent = default | validation lives in code; queries across tenants by key need the index |
| Typed columns per setting | strong types in SQL | a migration per new setting; 300 columns |
| Config in git (GitOps) only | reviewed, versioned | not self-service; minutes, not seconds; tenants cannot edit |

**Choice and why.** Rows plus a registry, as in the seed. A registry entry is the contract:

| Key | Type / range | Default | Public? | Write permission | Risk |
|---|---|---|---|---|---|
| `site.closed` | bool | false | no (read by the shell on every page) | `site.close` | high |
| `cache.off_until` | RFC 3339 time \| `"forever"` | absent = on | no | `system.write` | high |
| `limits.visitor_per_minute` | int 10-100000 | 120 | no | `system.write` | medium |
| `forms.quote` | JSON of field → {visible, required}, validated against what the form allows | all visible | no | `settings.write` | low |
| `search.llms_about` | Markdown ≤ 6,000 chars | none | no (the *file* it feeds is public) | `settings.write` | low |

Rules: **unknown keys are rejected** (a typo that silently does nothing is the worst bug); every value is validated against its type and range **on write** and again, defensively, **on read** (a corrupt value falls back to the default and is logged);
**public is an allowlist in code, not a denylist** (a new key must not leak by default; the seed's `PublicKeys`); and **secrets are never settings**: a setting's value is visible to ops, backups, audit, logs and the page head, which is exactly why the seed
keeps credentials in a separate sealed store ([42](42-credentials-vault.md)). Storing only overrides makes the table small, but it has a consequence: **changing a default changes behaviour for every tenant without a row**. Treat a default change like a deploy; a tenant that must stay put gets an explicit row.

### 9.2 Propagation: snapshots, NOTIFY, a feed, and the thundering herd

[09](09-feature-flags.md) covers poll-with-ETag and SSE for flag snapshots. Here the question is the *in-process* machinery and its failure modes.

| Option | Pros | Cons |
|---|---|---|
| TTL only (re-read every 5 s) | trivial | every pod × every tenant × every 5 s; 5 s is the *best* case |
| `NOTIFY` only | ~tens of ms | notifications are lost when a listener is disconnected; needs a session connection (not a transaction-pooling proxy) |
| **`NOTIFY` + change-feed poll + TTL** | fast path and a safety net | three mechanisms (each tiny) |
| Redis pub/sub | works through poolers | another dependency; also lossy |

**Choice and why.** `NOTIFY` is the fast path; every 5 s each pod asks the feed "what changed since I last looked" (40 queries/s in total); a 1-minute TTL covers everything else. The in-process snapshot is
immutable and swapped whole. The seed's `Live` pattern, simplified:

```go
type Live struct {
	cur     atomic.Pointer[Snapshot] // immutable snapshots, swapped whole
	gen     atomic.Int64             // bumped by NOTIFY or by our own save
	seen    atomic.Int64             // the gen of the snapshot in hand
	retryAt atomic.Int64             // unix nanos; back off after a failed read
	busy    atomic.Bool
}

func (l *Live) Get() *Snapshot {
	s := l.cur.Load()
	stale := s == nil || l.seen.Load() != l.gen.Load() || time.Since(s.LoadedAt) > time.Minute
	if stale && time.Now().UnixNano() > l.retryAt.Load() && l.busy.CompareAndSwap(false, true) {
		go func() {
			defer l.busy.Store(false)
			g := l.gen.Load() // a change landing during the read bumps gen again
			next, err := readAll(context.Background())
			if err != nil {
				l.retryAt.Store(time.Now().Add(10 * time.Second).UnixNano())
				return // keep last-known-good
			}
			l.cur.Store(next)
			l.seen.Store(g)
		}()
	}
	return s // nil only before the first load: boot waits for it, or starts from the baked-in defaults
}
```

Four properties matter: **readers never block** (stale-while-revalidate: a refresh runs in the background); **the generation counter** makes a change that arrives *during* a read count (the next `Get` sees `gen` ahead of `seen`); **last-known-good**
on database errors, with a retry delay so 5,000 reads a second per pod do not hammer a failing database; and a **baked-in default snapshot** (the registry) for a pod that boots while the database is down.

Two traps in the change feed. (1) **`seq` is assigned at insert, but transactions commit in any order**: a poller that has seen 105 can later see 103 appear. Fix: poll with an overlap (`seq > last - 1000`); invalidation is idempotent, so re-reading is harmless. (2) **A bulk change is the herd**:
changing a platform default for 5,000 tenants is 5,000 feed rows; 200 pods each reloading tenant by tenant is 200 × 5,000 = 1,000,000 queries in seconds. Fix: coalesce (collect the invalidated tenant ids for 100 ms), reload in batches (`WHERE tenant_id = ANY($1)`, 500 at a time = 10 queries per pod, 2,000 in total), add jitter, and
model a platform-wide default as **one platform-layer row**, not 5,000 copies.

**Read-your-writes without sticky sessions.** The pod that handled the save invalidates its own snapshot at once (the seed: "a save replaces it at once"), but the user's next request may hit another pod that is up to 5 s behind. The `PUT` response carries the new `settings_version`; the client echoes it as `X-Settings-Min-Version`; a pod whose snapshot is older refreshes *once, synchronously, with a 1 s cap* before answering.

### 9.3 Timed switches that revert themselves

The seed stores `cache.off_until`: no row = on, a time = off until then, `forever` = off until someone turns it on. The effective state is a **pure function of the clock**:

```
 off(now) = (value == "forever") || (value is a time && now < value)     // evaluated on every read, never cached as a boolean
```

| Option | Pros | Cons |
|---|---|---|
| Boolean + a scheduled job that flips it back | familiar | the job can fail, be delayed, be lost in a deploy; then "temporary" is permanent |
| Boolean + a human remembers | none | this is how switches stay off for a month |
| **A timestamp, evaluated at read time** | nothing to schedule or fail; every pod reverts within its clock skew; idempotent to set | readers need a clock (NTP: tens of ms; irrelevant at 15-minute scale) |

**Choice and why.** Data, not a job. Details that make it safe:

- **Absolute times, idempotent writes.** `PUT off_until=2026-11-11T15:00:00Z` is safe to retry; "toggle" flips twice, "extend by 15 minutes" extends twice. Extending means writing a new absolute time.
- **`forever` is a deliberate, narrower permission** and is capped for risky keys (a tenant admin may choose up to 24 h; `forever` needs the platform role).
- **Visible:** a top-bar pill ("Cache off", "Site closed") for everyone who can see the dashboard, a history row, and an **alert** ([50](50-alerting-engine.md)) when a switch has been away from its default for longer than its expected duration, plus a weekly "switches not at their defaults" report. A switch left off is a slow site or a closed shop that nobody notices.
- **Side effects at the edge, not at expiry:** turning the cache off or on purges it (the seed does); an expiry causes no purge, so make "cache back on" safe to happen unattended.
- **Fail direction is per key class:**

| Key class | If the value is unreadable | Why |
|---|---|---|
| Site open/closed | stay **open** (the seed's decision) | a monitoring glitch must not take the shop down |
| Disable a feature (e.g. payments) | last known good, else the registry default | a stale "off" is better than flapping |
| Security (block signups, require MFA) | fail **closed** / keep the stricter value | the safe side is the restrictive one |

- **A CDN extends propagation beyond the pods:** a page cached for 300 s keeps serving after the switch flips. The seed answers *both* the closed and the open page with `no-store` while closed.

### 9.4 Precedence, and showing the effective value with its source

| Layer (highest wins) | Set by | Why it ranks here |
|---|---|---|
| **Environment pin** | deploy / ops | works when the database is wrong or down; the seed's `API_RATE_LIMIT` fixes both per-minute limits and the page locks those fields |
| **Tenant value** | tenant admin, via the registry's rules | the point of the product |
| **Plan default** | product (per plan) | "Pro gets 10× the limit" without 5,000 rows |
| **Code default** | the registry | a pod can boot with no database |

Resolution returns `(value, source, locked)`, never just a value, so the UI can say *"Set on the server (`API_RATE_LIMIT`): change it there, not here"* (the seed's wording). Two more rules: **plan ceilings are enforced at write** (reject 5,000 when the plan allows 1,000 with a clear error) **and clamped at read** (a plan downgrade
must not leave a tenant above its new ceiling); the UI then shows *configured 5,000, effective 1,000 (plan limit)*. The environment layer has a second job. ADR 0009 makes the point for secrets (a production dump restored into staging cannot point staging at the real books); the same holds for settings: a **restored dump carries its settings rows too** (a `site.closed` or a limit), so pinning in the environment is how a staging copy keeps behaving like staging.

### 9.5 Permissions, audit and concurrency

Not every setting is the same kind of dangerous, and the seed's permissions follow that:

| Risk class | Example | Who | Extra |
|---|---|---|---|
| Content | `search.llms_about`, form fields | tenant editors | history only |
| Operational | close the site (`site.close`), flush the cache (`cache.flush`) | Admin and above; *flush* is harmless because "the next visitor rebuilds it"; closing is "the owner's call, not the day's", so not the Manager | confirmation dialog, reason |
| Protective | rate limits, turning the cache off (`system.write`) | the platform role only: "how the server protects itself is not the business's to change" | reason required; optional two-person approval |

- **Audit every real change**, with before and after, actor, reason, request id; an unchanged save writes nothing. Render history in words ("Visitors 120 → 200 a minute"). **Never record secret values**, only that a key changed.
- **Optimistic concurrency per group:** the group's version is its newest `updated_at`; `PUT … If-Match` gets `409` with *what* changed if someone saved first, and the seed notes the reason for per-group versions: saving one group must not make another form stale.
- **Revert** is a new write of the old value taken from the audit row; history is never edited.
- **Validate ranges so a typo cannot break the product:** a visitor limit of `0` throttles everyone, so the registry's minimum is 10. Correlate error-rate alerts ([50](50-alerting-engine.md)) with the audit log: "something changed 3 minutes ago" is the first thing an on-call wants to see.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** memory per pod (50,000 tenants × 30 KB = 1.5 GB), so keep only active tenants (LRU, load on first use, invalidate by id). Then the feed poll (2,000 pods ÷ 5 s = 400 queries/s: still fine) and `NOTIFY` fan-out: put a small regional "settings edge" between pods and Postgres.
- **Postgres down:** reads continue from the snapshot; writes fail loudly; a pod that restarts boots from the registry defaults (and a local disk copy of the last snapshot, if you keep one). Alert on "snapshot age".
- **A pod misses a `NOTIFY`:** the 5 s poll (or the 1-minute TTL) catches it. This is why the fast path is never the only path.
- **Bulk platform change:** coalesced, batched, jittered reloads (9.2); never per-tenant fan-out.
- **A bad value is saved:** range validation first, audit second; one-click revert from history; a post-change error-rate check.
- **Clock skew:** shifts a timed switch's edge by milliseconds between pods: fine for minutes-scale switches; for exact cut-overs use a version, not a time.
- **A switch left off:** pill, alert, weekly report, and the cap on `forever`.
- **Single points of failure:** none on the read path; the write path depends on Postgres, and `LISTEN` needs a session-level connection, not a transaction-pooling proxy.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Key/value overrides + code registry | JSON blob or typed columns | per-key audit and permissions; no migration per key | validation in code; defaults are a behaviour change |
| In-memory immutable snapshots | per-request cache or DB reads | ~100 ns reads; survives DB outage | up to 5 s staleness; memory per pod |
| `NOTIFY` + feed poll + TTL | TTL alone | tens-of-ms typical, 5 s worst case, no lost changes | three small mechanisms |
| `off_until` timestamps | a boolean + revert job | nothing to schedule or fail; idempotent | clock dependence (ms) |
| Layered resolution returning the source | a single value | honest UI; env pins; plan defaults without row explosion | resolution code and clamping rules |
| Risk classes with their own permissions | one `settings.write` | the dangerous switches have fewer hands | more roles to explain |
| Absolute-time, idempotent writes | toggles | safe retries | the client computes the time |

## 12. Check yourself

1. 200 pods serve 1M requests/s, each reading several settings. Why must reads be in-process, and what does a read cost per pod?
2. A tenant admin saves a limit and the very next request hits another pod. How do you avoid showing the old value, without sticky sessions?
3. Why is `cache.off_until` a timestamp rather than a boolean plus a job that turns it back on? What does the seed do if the value cannot be read, and why is that right for the *site* switch but not for every switch?
4. A limit is set both in the environment and in the database. Which wins, why, and what does the settings page show?
5. A poller asks for `seq > last` and misses a change. How, and what two fixes make the feed safe, including for a 5,000-tenant bulk change?

<details><summary>Answers</summary>

1. 1M reads/s would need ten Redis shards (100k ops/s each) or is impossible for Postgres. In-process, each pod does 5,000 reads/s at ~100 ns = 0.5 ms of CPU per second (0.05% of a core). Only *refresh* touches the network: about 40 small queries/s across the fleet.
2. The saving pod invalidates its own snapshot immediately. The `PUT` response returns the new `settings_version`; the client sends it back as `X-Settings-Min-Version`; a pod behind that version refreshes once, synchronously, with a short cap (about 1 s), then answers. Other users see it within the propagation time (usually tens of ms by `NOTIFY`, at most 5 s by the poll).
3. A timestamp is a pure function of the clock, so there is nothing to schedule, lose in a deploy, or forget: every pod reverts within its clock skew, and setting an absolute time is idempotent. A boolean needs a job that can fail and leaves a "temporary" switch permanent. The seed treats an unreadable status as *open*, so a monitoring or database glitch never closes the shop. A security switch must fail the other way (keep the restrictive value), so fail direction is declared per key class.
4. The environment wins (the seed's ADR 0009 rule): it works when the database is wrong or down, lets ops pin a value during an incident, and keeps a restored dump from changing a staging environment's behaviour. The page shows the effective value, its source ("Set on the server") and the field locked, so nobody edits a value that has no effect.
5. `seq` is assigned at insert but transactions commit in any order, so a poller that has seen 105 can later find 103 committed. Fix 1: poll with an overlap (`seq > last - 1000`) because invalidation is idempotent. Fix 2 for bulk changes: coalesce invalidations (about 100 ms), reload in batches of 500 tenants (10 queries per pod, 2,000 in total instead of 200 × 5,000 = 1,000,000), add jitter, and store platform-wide defaults as one platform-layer row.

</details>

## 13. In Go

- Related lessons: [15 Sync & context](../../15-sync-context.md) (`sync/atomic`, contexts for background refresh), [38 Caching patterns](../../38-caching-patterns.md) (stale-while-revalidate, `singleflight`), [23 Config & logging](../../23-config-logging.md) (environment precedence), [59 Production DB patterns](../../59-production-database-patterns.md) (`LISTEN/NOTIFY`, optimistic versions), [43 Authorization & multi-tenancy](../../43-authorization-rbac-multitenancy.md) (permission per key class), [36 Resilience](../../36-resilience-patterns.md) (last known good, backoff).
- Immutable snapshots: build a new `map` and publish it with `atomic.Pointer[Snapshot]`; never mutate a published snapshot. Compile the registry into the binary (a `map[string]Def` or generated code) so a pod boots without a database.
- `pgx`: a dedicated `*pgx.Conn` (not from a transaction-pooling proxy) running `LISTEN settings` and `conn.WaitForNotification(ctx)` in a loop with reconnect and a full reload after every reconnect.
- Packages: `sync/atomic`, `pgx/v5`, `golang.org/x/sync/singleflight` (collapse concurrent refreshes), `encoding/json`, `github.com/google/go-cmp/cmp` (before/after diffs for history text).

## 14. Extensions

- **Scheduled changes:** "Under construction from 02:00 to 04:00 on Sunday" as `starts_at`/`ends_at` windows evaluated like `off_until`.
- **Two-person approval** for platform keys, with the audit row recording both people.
- **Settings as code:** export/import a tenant's settings with preview, apply and undo ([48](48-bulk-import.md)), and a drift report against a reviewed baseline.
- **Cohort rollout of a setting** (5% of tenants) by delegating to the flag service ([09](09-feature-flags.md)); decide where the line between setting and flag sits.
- **Dead-setting cleanup:** record the last time each key was read, as flag services do, and retire keys nobody reads.
- **Multi-region:** read-your-writes across regions (version tokens carried between regions), and where the source of truth for a travelling tenant lives.
- **Generated typed SDK** from the registry so application code cannot read an unknown key or the wrong type.
