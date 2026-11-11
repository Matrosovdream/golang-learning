# 42 · Integration Credentials Vault

`🟢 easy` · *Topics:* envelope encryption with a per-tenant key, AES-GCM with bound context (AAD), key versions and rotation without downtime, write-only APIs, where plaintext may exist, a secrets broker, test-connection without leaks, audit at 20k reads a second, crypto-shredding · *Builds on:* [05 In-Memory Cache with TTL](05-kv-cache-ttl.md) (caching unwrapped keys), [10 Webhook Delivery](10-webhook-delivery.md) (SSRF), [41 Backup & Restore](41-backup-restore.md) (backups hold only ciphertext)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [41 Backup & Restore](41-backup-restore.md) · Next: [43 Zero-Downtime Deploys & Schema Migrations](43-zero-downtime-deploys.md)

---

> 🌱 **Seed (honeydolist-site):** *Settings → Integrations* (PLAN step 7i, ADR 0009, migration 0030). `integrations` holds the seeded providers (one active per category); `integration_settings` holds each setting as a plain `value` **or** a sealed `secret` (a `CHECK` allows only one). Secrets are sealed with AES-256-GCM under `SECRETS_KEY` (32 random bytes per environment): `version(1) | key fingerprint(8) | nonce(12) | ciphertext+tag`, with the integration slug and key as additional data, so a value copied to another row does not open. The fingerprint says *which key sealed it* without opening anything, so a production dump restored into staging reads *"cannot read — enter it again"* and counts as not set. The API is write-only (*set · ends in 7Kq2*, hint only for secrets of 16+ characters; Replace, Clear); the audit log records which keys changed, never a value; secrets never go into `settings`, because its saves snapshot into `content_revisions` and `pg_dump` would carry them off-site in clear. The environment wins over the database. *Test connection* is an outbox job: the worker opens the secret at that moment and keeps it nowhere; a provider saying no is a result, not a retry (step 7i.2; steps 23.1 and 45 let the email password and the invoicing token be typed in the dashboard). The ADR records its own costs: losing `SECRETS_KEY` means re-entering every secret, rotation is the same, and the API holds the key, so a compromised API process can open secrets. Rejected there: `pgcrypto` with the key in every query, and a secrets manager ("the right answer at more services and more people"). Small-scale numbers: ~7 integrations, a handful of secrets.
> **Here, at scale:** a platform where 100k tenants paste API keys for up to 40 integrations: ~1M sealed values, 20k secret reads/s from workers, and auditors asking who could ever read them.

## 1. The task

> "Design the service that stores the third-party credentials our customers enter (API keys, SMTP
> passwords, OAuth refresh tokens) and hands them to background jobs. Nobody (not support, not a
> database admin, not a compromised web server) should be able to read them casually."

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Reads decide the architecture; storage is tiny. | 100k tenants × ~6 connected integrations; ~1M secret values; 20k reads/s peak, 5 writes/s. |
| Who may see a secret after it is saved? | Defines the API. | Nobody: set, replace, clear, test. No reveal. |
| Who needs plaintext? | Narrows where it may exist. | Only workers and a broker, at the moment of use. |
| Are secrets platform-wide or per tenant? | Per-tenant keys limit blast radius. | Per tenant; the platform's own provider keys live in a cloud secret manager, not here. |
| Compliance? | Shapes audit and rotation. | Annual audit; rotation of our keys without tenant action; erasure of a tenant's secrets on request. |
| Is a KMS available? | Decides the root of trust. | Yes: a cloud KMS or HSM for one root key. |
| What if a tenant's third-party key leaks? | We cannot rotate their key for them. | Provide *disable* and *mark compromised*; they rotate at the provider. |

## 3. Requirements

**Functional**
- Set, replace, clear a secret field of an integration; non-secret settings are stored apart in clear.
- Show per field: *set*, *unset* or *unreadable*, a short hint, who changed it and when.
- Test connection without ever returning the secret; show the last 5 results.
- Workers obtain plaintext for `(tenant, integration, field)` for the duration of one job.
- Rotate our keys with no tenant action and no downtime; destroy a tenant's keys on offboarding.

**Non-functional**
- 20k reads/s at p99 < 5 ms (the job's budget), with no KMS call on the hot path.
- A database dump, a backup or a stolen replica reveals no usable secret.
- A compromised web process cannot open secrets; a compromised worker can read only what it processes.

**Out of scope:** OAuth flows, human password storage ([56](../../56-authentication-sessions.md)), the platform's own secrets.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Secret values | ~1M | 100k tenants × 6 integrations × ~1.7 secret fields |
| Table size | ~500 MB | ≤ 300 B sealed value (≤ 256 B secret + 33 B header, nonce, tag) + metadata ≈ 0.5 KB per row |
| Reads | 20k/s ≈ 1.7B/day | given; 20,000 × 86,400 |
| AES-GCM per read | microseconds | hardware AES; the CPU cost is negligible |
| A KMS call per read | 20k/s | not feasible or affordable: the root key is used only to unwrap tenant keys |
| Tenant keys in memory | 100k × 32 B ≈ 3.2 MB | every process can cache every KEK it touches |
| Unwraps, random job routing | ≈ 17k/s | 200 workers × ~26k distinct tenants per 5-min TTL ÷ 300 s (a worker makes 100 reads/s) |
| Unwraps, tenant affinity (3 workers per tenant) | ≈ 1k/s at 5 min, ≈ 83/s at 1 h | 100k × 3 ÷ 300 s; ÷ 3,600 s |
| Audit rows if one per read | ~260 GB/day | 1.7B × ~150 B: not a table, a firehose |
| Re-seal sweep after a KEK rotation | ≈ 17 min | 1M values at 1k/s |

What the numbers tell us: storage is trivial, a single Postgres holds it all. The design is about **who may
decrypt** and **how often the root key is touched**. Caching tenant keys is what makes 20k reads/s possible,
and *how jobs are routed* decides whether the KMS sees 17k or 83 calls a second.

## 5. Plan

1. Envelope encryption: the key hierarchy, the sealed-value format, what the AAD binds.
2. Rotation: root key, tenant key, and the compromise case.
3. The API: write-only semantics, states, hints, modes.
4. Where plaintext may exist: the API cannot open, a broker or the worker can.
5. Test connection, blast radius, and an audit that scales.

## 6. High-level design

```
 dashboard ─► API (seal-only) ──► broker.Seal ─► tenant KEK (cached) ─► AES-GCM ─► Postgres: integration_secrets
                │ no Open permission                         ▲
                ▼                                            │ unwrap on cache miss
        integrations / checks / audit (Postgres)        KMS: root key (never leaves)
                                                             ▲
 job (sync, send) ─► worker ──mTLS, job-scoped──► broker.Open ┘   tenant KEK cache (TTL 1 h, routed by tenant)
                         │ plaintext in memory only, for this call
                         ▼
                   provider (HTTPS)       test connection = a job like any other (result stored, secret not)
```

**Set:** the API validates the field's shape, asks the broker to seal under the tenant's *active* KEK
(bound to tenant, integration, field and key version), stores the blob and an audit row. **Use:** a worker
calls `Open` with its service identity and a job id; the broker loads the blob (a cache of *ciphertext* is safe),
gets the KEK from memory or unwraps it with KMS, opens, returns plaintext over mTLS, and the worker drops it after the call.

## 7. API

```
GET    /tenants/{t}/integrations                       → 200 [ { slug, active, mode, status, fields: [ { name, secret, state: "set"|"unset"|"unreadable", hint, updated_by, updated_at } ] } ]
PUT    /tenants/{t}/integrations/{slug}/fields/{f}     { "value": "…" }  → 204     write-only; the value is never echoed, not even in errors
DELETE /tenants/{t}/integrations/{slug}/fields/{f}     → 204
POST   /tenants/{t}/integrations/{slug}/test           → 202 { check_id }   one at a time per (tenant, slug); rate-limited
GET    /tenants/{t}/integrations/{slug}/checks         → 200 last 5 [ { ok, note, at, by } ]
POST   /tenants/{t}/integrations/{slug}/disable        → 204                  the "mark compromised" switch

-- internal, mTLS, by service identity (workers only):
POST   /internal/secrets/open { tenant_id, slug, field, job_id }  → 200 { value }   never logged; Cache-Control: no-store
```

`Cache-Control: no-store` on every response, secrets only in bodies (never in URLs or headers), and request-body
logging is redacted by field name.

## 8. Data model

Non-secret settings live in `integration_settings (tenant_id, slug, key, value jsonb)`, separate from secrets: a
value is in exactly one of the two places, so a dump of settings never contains a credential.

```sql
CREATE TABLE tenant_keys (                  -- one KEK per tenant per version, wrapped by the root key in the KMS
    tenant_id   bigint  NOT NULL,
    version     integer NOT NULL,
    wrapped_kek bytea   NOT NULL,
    root_key_id text    NOT NULL,           -- which root key version wrapped it
    state       text    NOT NULL CHECK (state IN ('active', 'decrypt_only', 'destroyed')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, version)
);
CREATE UNIQUE INDEX tenant_keys_one_active ON tenant_keys (tenant_id) WHERE state = 'active';

CREATE TABLE tenant_integrations (
    tenant_id bigint NOT NULL, slug text NOT NULL, mode text NOT NULL DEFAULT 'tenant' CHECK (mode IN ('platform', 'tenant')),
    is_active boolean NOT NULL DEFAULT false, disabled_reason text,
    PRIMARY KEY (tenant_id, slug)
);
CREATE TABLE integration_secrets (
    tenant_id   bigint  NOT NULL, slug text NOT NULL, field text NOT NULL,
    sealed      bytea   NOT NULL,           -- 0x01 | kek_version(4) | nonce(12) | ciphertext+tag
    kek_version integer NOT NULL,           -- same number as in the header, so a sweep can use an index
    hint        text,                       -- last 4 characters, only for secrets of 16+ characters
    updated_by  bigint, updated_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,               -- written at most once an hour per secret (≤ ~280 writes/s overall)
    PRIMARY KEY (tenant_id, slug, field),
    FOREIGN KEY (tenant_id, slug) REFERENCES tenant_integrations (tenant_id, slug) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, kek_version) REFERENCES tenant_keys (tenant_id, version)
);
CREATE INDEX integration_secrets_kek_idx ON integration_secrets (kek_version, tenant_id);

CREATE TABLE secret_audit (                 -- human and API actions only: keys, never values
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id bigint NOT NULL, actor text NOT NULL,
    action text NOT NULL CHECK (action IN ('set', 'clear', 'test', 'disable', 'open_denied')),
    slug text NOT NULL, field text, at timestamptz NOT NULL DEFAULT now()
);
```

## 9. Deep dives

### 9.1 Envelope encryption: the key hierarchy

| Option | Pros | Cons |
|---|---|---|
| **One master key seals everything** (the repo's `SECRETS_KEY`) | tiny, no infrastructure | one leak opens every tenant; rotation = every tenant re-enters their secrets |
| **A data key per secret, wrapped by a master key** (the textbook form) | per-secret rotation | 1M wrapped keys, a KMS unwrap per read unless cached; no tenant-level shredding |
| **Root key in KMS → a KEK per tenant → values sealed with the KEK** | blast radius = one tenant; rotate the root by re-wrapping 100k small keys; destroying a tenant's KEK shreds all their secrets; KMS touched only on cache miss | a tenant-key table, a cache, a broker |
| Database-side `pgcrypto` with the key in each query | no app code | the key travels in every statement, shows up in logs and `pg_stat_statements` (the repo's reason to reject it) |

**Choice: root → tenant KEK → values.** A tenant has ≤ a few hundred values, so sealing them directly with
the KEK under random 96-bit nonces stays far below GCM's limits; a per-secret data key adds KMS traffic and
no safety at this size (it earns its place for large objects such as files). The sealed value keeps the repo's
shape, with the key *version* instead of a fingerprint:

```go
// sealed = 0x01 | kekVersion(4) | nonce(12) | ciphertext+tag
func seal(kek []byte, ver uint32, aad, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 random bytes
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := binary.BigEndian.AppendUint32([]byte{1}, ver)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, aad), nil // ciphertext is appended after the header
}

func open(kekFor func(ver uint32) ([]byte, error), aad, sealed []byte) ([]byte, error) {
	if len(sealed) < 1+4+12+16 || sealed[0] != 1 {
		return nil, ErrUnreadable
	}
	kek, err := kekFor(binary.BigEndian.Uint32(sealed[1:5])) // ErrKeyGone if the KEK was destroyed
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(kek)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, sealed[5:17], sealed[17:], aad)
	if err != nil {
		return nil, ErrUnreadable // wrong key, a moved row, or tampering
	}
	return pt, nil
}
```

`newGCM` is `aes.NewCipher(kek)` (32 bytes = AES-256) then `cipher.NewGCM`. The **AAD** is
`tenant_id ‖ 0x00 ‖ slug ‖ 0x00 ‖ field ‖ 0x00 ‖ kek_version`: immutable identifiers only (never `updated_at`),
so a blob copied to another tenant, integration or field refuses to open, exactly the repo's slug-and-key binding
extended with the tenant. The **version in the header** lets `open` pick the right KEK without trial decryption.

### 9.2 Rotation without downtime (and without asking 100k customers)

The repo's answer, *set a new `SECRETS_KEY` and re-enter every secret*, cannot work when the secrets are customers' and there are a million.

| Rotation | Procedure | Cost |
|---|---|---|
| **Root key** | KMS creates root v2; a job unwraps each KEK with v1 and wraps it with v2 (`UPDATE tenant_keys … WHERE root_key_id = 'v1'`); secrets are untouched | 100k small operations, minutes; cached KEKs stay valid |
| **Tenant KEK** | insert version 2 as `active` (v1 becomes `decrypt_only`); new writes use v2; a sweep re-seals v1 rows (`WHERE kek_version < $cur ORDER BY kek_version, tenant_id LIMIT 500 FOR UPDATE SKIP LOCKED`, update guarded by `AND kek_version = $old`); when none remain v1 becomes `destroyed` | 1M values at 1k/s ≈ 17 min, dual-read the whole time |
| **A leaked provider key** | we cannot rotate it; `disable` stops all use at once, the tenant pastes a new one | seconds, tenant action |

A *destroyed* KEK is how offboarding erases a tenant's secrets everywhere, backups included
([41](41-backup-restore.md): backups hold only ciphertext, and the key is gone), the *crypto-shredding* of
[24](24-data-lifecycle.md). A KEK cache means revocation takes effect after the cache TTL: choose it (1 h) knowing that.

### 9.3 The write-only API

| Choice | Why |
|---|---|
| No endpoint returns plaintext, **to anyone**, including our staff | a "reveal" feature is an exfiltration feature; support can *replace* or *clear*, not read |
| `state` is `set` / `unset` / `unreadable` | *unreadable* (KEK destroyed, or sealed under another environment's keys) is the repo's *"cannot read — enter it again"*; it counts as *not set*, so a restored dump never silently uses stale credentials |
| `hint` only for secrets ≥ 16 characters (the repo's rule) | a short password never gives away a quarter of itself; skip hints entirely for low-entropy fields |
| Shape check on write (`sk_live_…` prefixes, length) | catches a pasted wrong value before it fails at 3 a.m. in a job |
| `mode: platform \| tenant` per integration | the repo's *environment first, then database* rule, made explicit: a platform-managed provider needs no tenant secret, a bring-your-own one does, and the UI shows which applies |
| Separate permissions `integrations.read` / `.manage` | seeing *that* a key is set is not changing it (the repo reserves `integrations.manage` for the Superadmin) |

### 9.4 Where plaintext may exist

| Option | Pros | Cons |
|---|---|---|
| **The API opens secrets** (the repo's accepted cost: the API holds the key) | simplest | a compromised web process reads every secret it can reach |
| **Every worker may call KMS** | no extra service | blast radius = every worker; 200 processes × cache misses (≈ 17k unwraps/s with random routing) |
| **A secrets broker**: only it may unwrap KEKs; `Seal` for the API, `Open` for workers only | the API cannot open; one place to rate-limit, audit and cache; routing by tenant cuts KMS traffic to ~1k/s or less | a hot-path service; mTLS and identities to run; on outage jobs retry with backoff |
| Per-tenant public keys so the API seals without any key to open | strongest API isolation (the repo names this as the later change) | key distribution and a second sealing path |

**Choice: the broker.** A stateless Go service, horizontally scaled, the only identity with `Decrypt` on the root
key. The API has `Seal` and no `Open`; workers have `Open` for the job they hold, checked against the job id. Route
jobs by tenant (consistent hashing) so each worker sees a stable set of tenants and the KEK cache hit rate is high.
The broker may cache **ciphertext** rows freely; caching *plaintext* secrets is a separate, short-TTL decision.
Plaintext may exist **only** in the memory of the broker and of the worker during the call, and **never** in:
logs or error strings (provider errors often echo the token: scrub before storing), job payloads or outbox rows
(carry `tenant_id, slug, field`, never values, as the repo's outbox carries ids), `pprof` heap endpoints and
core dumps, child-process environments, revisions, backups, or a support screenshot. In Go, give secrets a type
that cannot be printed:

```go
type Secret string

func (Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }
func (Secret) String() string       { return "[REDACTED]" }
```

### 9.5 Test connection, blast radius, and an audit that scales

The repo's flow is the right shape: `POST …/test` enqueues a job (the API never calls a provider in a request),
one at a time per integration, the worker opens the secret at that moment, and a provider's *no* is a stored
result, not a retry. At scale add:

- **SSRF:** a host or URL field (SMTP host, a webhook base URL) lets a tenant aim our worker at `169.254.169.254` or an internal service. Resolve, check against a deny-list of private ranges, connect to the checked address ([10](10-webhook-delivery.md)).
- **Scrub results:** `last_check_note` is the provider's own words; remove any substring equal to a secret of that tenant before storing it.
- **Rate-limit tests** per tenant ([03](03-rate-limiter.md)), or the test button becomes a free proxy.
- **Blast radius by design:** a KEK opens one tenant; a worker sees one job's tenant at a time; tell tenants the minimum scopes to grant (an integration's `schema` lists them).
- **Audit:** 1.7B reads/day cannot be rows. Audit what is rare and meaningful: **KEK unwraps** (the KMS's own log), `secret_audit` for set/clear/test/disable/denied, and `last_used_at` throttled to one write an hour. "Who could read secrets?" has a one-line answer for auditors: *the broker role, and workers through it*.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (10M values, 200k reads/s):** the broker fleet and its cache, not Postgres (5 GB still fits one primary). Route by tenant, grow the TTL, and add a ciphertext row cache; plaintext stays uncached.
- **KMS down:** cached KEKs keep working until their TTL; a first read for an uncached tenant fails and the job retries with backoff. New tenants cannot get a KEK created until it returns.
- **Broker down:** jobs back off and retry (queues absorb it, as in [15](15-job-scheduler.md)); saving a new secret fails with a clear "try again" (`Seal` needs the broker too), so the dashboard stays up but cannot change credentials until it returns.
- **Restore into another environment or a destroyed KEK:** `open` returns `ErrKeyGone`/`ErrUnreadable`; the field shows *unreadable* and the integration is treated as not set, never as a crash.
- **Corrupt ciphertext:** same path, plus an alert; **never** fall back to a plaintext copy.
- **Compromised worker:** reads only secrets of jobs it holds, rate-limited by the broker; alert when a worker's open rate leaves its normal band.
- **GCM limits:** random 96-bit nonces are safe for a few thousand values per key; if a KEK ever seals billions, rotate sooner.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Root in KMS → per-tenant KEK | one app-wide key | blast radius of one tenant, cheap rotation, shredding | a key table, a cache, TTL-delayed revocation |
| Seal values directly with the KEK | a data key per secret | fewer keys and KMS calls | per-object rotation granularity (fine at this size) |
| A broker holds the only `Decrypt` | the API (or all workers) opening | a compromised web tier reads nothing | a hot-path service to run |
| Version in the header + AAD | trial decryption, unbound blobs | right key first time; blobs cannot be moved between rows | immutable identifiers in the AAD forever |
| No reveal, ever | an admin "show secret" | nothing to exfiltrate | support must ask tenants to re-enter |
| Audit unwraps and actions, not reads | a row per read | an audit trail that fits on disk | no per-read history (counters instead) |

## 12. Check yourself

1. Why a per-tenant KEK wrapped by a root key instead of one key for everything, as the repo does?
2. What does binding the AAD to tenant, integration and field buy, and what must never go into it?
3. How do you rotate the root key for 100k tenants without touching a single secret? And a tenant KEK without downtime?
4. Why must the API be unable to open secrets, and how can it still save them?
5. Why can't every read call the KMS, and why not write an audit row per read?

<details><summary>Answers</summary>

1. One key means one leak opens every tenant and every rotation needs every customer to re-enter their keys.
   A per-tenant KEK limits a breach to one tenant, lets us rotate the root by re-wrapping 100k small keys, and
   lets us destroy a tenant's KEK to erase their secrets everywhere, backups included.
2. A blob copied to another tenant's row, another integration or another field will not authenticate, so a
   database writer cannot swap secrets around. The AAD must hold only immutable identifiers (never `updated_at` or anything that changes), or valid blobs stop opening.
3. Root: create root v2, unwrap each tenant KEK with v1 and wrap it with v2; secrets are not re-encrypted.
   KEK: insert v2 as active, new writes use v2, a background sweep re-seals v1 rows with a guarded update,
   `open` reads the version from the header and dual-reads meanwhile, and v1 is destroyed when none remain.
4. A compromised web process would otherwise read every secret. The API has only `Seal` on the broker; saving is
   a seal call and never needs an open permission. Only workers (and the broker) can open, per job.
5. 20k KMS calls/s is not feasible or affordable and puts the KMS on the hot path; we unwrap each tenant KEK once
   per cache TTL, so routing jobs by tenant matters. An audit row per read is ~260 GB/day; audit the rare, meaningful events instead (unwraps, sets, denials).

</details>

## 13. In Go

- Related lessons: [57 Web security](../../57-web-security.md) (secrets, headers, SSRF), [43 Authorization, RBAC & multi-tenancy](../../43-authorization-rbac-multitenancy.md) (tenant scoping, the separate read/manage permissions), [23 Config & logging](../../23-config-logging.md) (redaction with `slog.LogValuer`), [38 Caching patterns](../../38-caching-patterns.md) (a TTL cache of unwrapped keys, singleflight on a miss), [12 Errors](../../12-errors.md) (sentinel errors such as `ErrUnreadable`, `ErrKeyGone`), [44 Background jobs](../../44-background-jobs-queues.md) (the test-connection job).
- Packages: `crypto/aes`, `crypto/cipher`, `crypto/rand`, `encoding/binary`, `golang.org/x/sync/singleflight` (one KMS unwrap per tenant at a time), `log/slog`; for the root key a cloud KMS SDK or Vault's transit engine behind a small `KeyWrapper` interface (so tests use a fake).
- Test like the repo does: a fake `KeyWrapper`, a *round-trip*, a *moved-row refuses to open*, a *wrong-key says unreadable*, and a test that greps logs and error strings for a planted secret.

## 14. Extensions

- **Customer-managed keys (BYOK):** the tenant's own KMS key wraps their KEK; revoking it makes all their secrets unreadable instantly.
- **Short-lived credentials:** broker-minted, scoped tokens (OAuth refresh → access token cached for minutes) so workers rarely hold long-lived secrets.
- **Anomaly detection:** per-tenant and per-worker open rates, geographic or time-of-day outliers, auto-disable on a burst.
- **Secret scanning:** reject pasted values that match a known leaked-key pattern; scan logs for sealed-then-leaked values.
- **Dual control:** two approvals to change the root key's policy; a break-glass role with an audited, time-boxed grant.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Secrets broker (Go, stateless, mTLS) | 20k `Open`/s (§2 says peak; §4 treats it as constant) + 5 `Seal`/s | CPU (TLS + AES-GCM) | 0.2 ms CPU an `Open` (kept-alive mTLS, hardware AES, job-id check, JSON) × 20,000 = 4 vCPU. N = ceil(4 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.88) = 2 → 3. Memory: ciphertext cache 1M × ~0.5 KB = 500 MB, KEK cache 3.2 MB, of 8 GiB. Traffic ≈ 20k × 0.5 KB = 10 MB/s | `c7g.xlarge` (4 vCPU / 8 GiB), one per zone; workers balance client-side (an ALB would add ≈ $230: ~36 GB/h ≈ 36 LCU) | 3 | `c`: crypto and TLS, no state worth RAM; 33 % busy normally, 50 % with a zone down. Each worker calls the broker in its own zone, so there is no cross-zone bill (≈ $350 if routed across zones) |
| KMS (the root key) | KEK unwraps, not reads | request price and quota, not hardware | unwraps = brokers × tenants ÷ TTL = 3 × 100k ÷ 3,600 s = 83/s ≈ 219M a month. With 200 workers each caching for 5 min it is 17k/s (§4), and the quota (regional, roughly 5k-50k requests/s; check) | **AWS KMS**, one symmetric key (managed already; CloudHSM or a Vault cluster means owning HSMs or 3+ nodes plus unseal) | 1 key | few, fat brokers keep the cache count at 3, so each broker holds every tenant's KEK (3.2 MB). Every unwrap is a CloudTrail event (≈ 219M a month, ~0.2 TB): the audit trail the lesson wants |
| Postgres (`integration_secrets`, keys, audit) | misses ≈ 1M secrets ÷ 3,600 s = 278 reads/s; `last_used_at` ≤ 278 writes/s; 5 sets/s | nothing; availability | ≈ 560 ops/s ≈ 600 IOPS vs 3,000 included; table 500 MB, ~1 GB with indexes, all in RAM | `db.m7g.large` (2 vCPU / 8 GiB), RDS Multi-AZ | 1 + standby | a separate instance from the product DB, so a dump or admin of one never holds the other's secrets; `m`, because the whole DB is 1 GB. Reads of ciphertext hit the broker's cache (1 h TTL, invalidated on write), so 20k/s never reaches Postgres |

No GPU, no big instance: this is a crypto-and-cache service whose bill is one managed API's request count.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Brokers: 3 × `c7g.xlarge` (3 × $105) | $315 |
| Postgres: `db.m7g.large` $120, Multi-AZ doubles | $240 |
| gp3 20 GB × 2 | $3 |
| KMS: 83/s × 2,628,000 s = 219M requests × $0.03 per 10,000 + $1 for the key | $658 |
| **Total** | **≈ $1,200** (≈ $1,000 with a 1-year commitment on the $555 of compute; KMS is not discounted) |

- **Biggest line and why:** KMS requests, 54 %, and the only line a design choice moves by 200×: per-worker caches with random routing mean 17k unwraps/s ≈ 45.5B a month ≈ **$137,000**. The lever is the cache count (unwraps = caches × tenants ÷ TTL: few, large brokers, a longer TTL, with revocation delayed by that TTL, 9.2).
- **What changes at 10×:** 200k opens/s ⇒ 40 vCPU ⇒ N = ceil(40 ÷ (0.8 × 16 × ⅔ = 8.53)) = 5 → 6 × `c7g.4xlarge` (≈ $2,520), still 6 caches: ~167 unwraps/s ≈ $1,300 if tenants stay at 100k, ≈ $13,000 if they also grow 10×. Postgres sees 10M ÷ 3,600 = 2.8k reads and 2.8k `last_used_at` writes a second on 5 GB: one `db.m7g.xlarge` is enough. The broker CPU and the KMS quota run out first, not the database.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Tiny: ~1M sealed values of ≤ 300 B (~500 MB, ~1 GB with indexes) and 100k wrapped tenant keys. The database never sees the 20k reads/s: the broker caches ciphertext and keys, so ~278 reads/s, at most ~278 `last_used_at` writes/s and 5 sets/s arrive (sections 4, 15).
- Access is by primary key `(tenant_id, slug, field)`, one re-seal sweep (`WHERE kek_version < $cur … FOR UPDATE SKIP LOCKED`) and two composite foreign keys; the values are opaque `bytea` that no query looks inside.
- What matters is guarantees, not query power: one active key per tenant, a secret that can never point at a missing key, a dump that holds nothing usable (the root key is in KMS), and an instance separate from the product database.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | `bytea` for the sealed blob; composite foreign keys `(tenant_id, kek_version)` stop a secret pointing at a missing key row; the partial unique index `tenant_keys_one_active … WHERE state = 'active'` makes "one active KEK per tenant" a database rule; `SKIP LOCKED` batches the sweep. Only that partial index is Postgres-only |
| MySQL (RDS / Aurora) | ✔ works, equally good | `VARBINARY` or `BLOB` for the blob, foreign keys, `SKIP LOCKED` (8.0), `ON DUPLICATE KEY UPDATE` for a replace; the one-active rule is a generated column `IF(state = 'active', tenant_id, NULL)` plus a unique index (a unique index allows many NULLs). The clustered primary key keeps a tenant's secrets adjacent |
| DynamoDB | ✔ works, no gain | one item per secret (≤ 300 B against 400 KB) and key reads are its strength, but there are no foreign keys (the key-exists check moves into code), "one active KEK" needs a conditional write, and the sweep by `kek_version` needs a GSI whose single key per version takes the sweep's ~1k writes/s, right at the ~1,000 writes/s-per-partition figure |
| AWS KMS, and a secrets manager | ✅ KMS holds the root key (not a table), ✖ a managed secret per value | the root never leaves KMS and every unwrap is a logged event (section 15); 1M customer values as 1M managed secrets would turn a table lookup plus local AES-GCM into an API call per read. A secrets manager fits the platform's own few secrets, which §2 puts out of scope |

**Pick:** PostgreSQL on its own RDS Multi-AZ instance (`db.m7g.large`, section 15) for `tenant_keys`, `integration_secrets` and `secret_audit`, plus AWS KMS for the single root key. This is a lesson where any relational database works; the tie-breakers are the team's skill, one engine to be on call for (so the product's), and one line of difference in how "one active key" is enforced. It is safe in an ordinary database because everything stored is a `wrapped_kek` (useless without KMS) or ciphertext bound by AAD to its row, so a dump, a backup ([41](41-backup-restore.md)) or a replica reveals nothing usable. System of record: the two tables of ciphertext and wrapped keys, and the root key in KMS (irreplaceable: lose it and every tenant's secrets are gone); derived: the broker's ciphertext and key caches (plaintext is never cached).

**If it were MySQL instead:** nothing material: the crypto, the AAD and the broker are Go code. Differences are a generated column plus a unique index for the one-active rule, `ON DUPLICATE KEY UPDATE` instead of `ON CONFLICT`, and a sweep that selects with `FOR UPDATE SKIP LOCKED` then updates (no `RETURNING`). Keep it on its own instance either way.

**What would flip the choice:** size never does: 10× is 10M values and 5 GB, one `db.m7g.xlarge` (section 15). Requirements would: owning the root key's hardware (CloudHSM or a Vault cluster behind the `KeyWrapper` interface), customer-managed keys (BYOK), which change KMS usage and not the table, or a no-servers, multi-region key lookup, where DynamoDB's per-key reads become attractive. If reads ever bypass the broker's cache, the KMS quota runs out before the database.
