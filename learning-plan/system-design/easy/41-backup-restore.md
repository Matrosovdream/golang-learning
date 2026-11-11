# 41 · Backup & Restore

`🟢 easy` · *Topics:* RPO/RTO per failure class, logical vs physical backups + WAL archiving (PITR), encrypted chunked archives and key custody, off-site immutable copies, restore verification and drills, per-tenant export and restore · *Builds on:* [15 Job Scheduler](15-job-scheduler.md) (one scheduled run at a time), [02 Pastebin](02-pastebin.md) (object storage), [10 Webhook Delivery](10-webhook-delivery.md) (idempotent side effects after a restore)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [40 Media Reprocessing at Scale](40-media-reprocessing.md) · Next: [42 Integration Credentials Vault](42-credentials-vault.md)

---

> 🌱 **Seed (honeydolist-site):** *Settings → Backups* (PLAN steps 44, 46, 60; migrations 0068–0070; `docs/backlog/backups.md`). The owner ticks table groups (eleven; a Go test fails if a table is in no group and not on the exclusion list), a schedule (≥ 1 h) and retention (14). A pure-Go dump (binary `COPY` in one `REPEATABLE READ` transaction; the distroless image has no `pg_dump`) is streamed through AES-256-GCM in 64 KiB chunks under an Argon2id key (t=3, 64 MiB, 2 threads) into a `.hdlbak` file whose manifest (migration number, row counts, SHA-256) is *inside* the encrypted body. It is stored in a private bucket, emailed under 15 MB, pulled to a laptop by `make backup-pull`, and copied to a write-only Backblaze B2 bucket together with the originals of photos and videos. Restore is a dry-run *Check* showing now → after row counts, a typed `RESTORE`, a safety backup first, and refusal of an archive from another migration number; one advisory lock serialises backup and restore. A drill test rebuilds a fresh server from only the bucket, a read key and the passphrase; a drill against real B2 has **not** been run. Small-scale numbers: tens of MB, at most a day of changes lost.
> **Here, at scale:** a multi-tenant SaaS with a 20 TB Postgres, 100k tenants, RPO 5 min, RTO 1 h, and a need to bring back one tenant without touching the rest.

## 1. The task

> "Design backup and restore for a multi-tenant SaaS database of 20 TB. We must survive losing the
> cluster, losing the cloud account, or a customer deleting their own data by mistake, and support
> must be able to bring back one tenant without touching the others."

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale and write rate? | Drives WAL volume, backup windows. | 20 TB, 100k tenants (avg 200 MB, heavily skewed), ~20 MB/s of WAL on average. |
| RPO 5 min and RTO 1 h: *for which failure*? | One number cannot cover a node dying and a dropped table. | Targets per failure class (9.1). |
| What can go wrong? | Each needs a different copy. | Node/AZ loss, operator error (bad migration, `DROP`), corruption or ransomware, loss or compromise of the cloud account, a tenant's own mistake. |
| Compliance? | Retention and erasure rules. | PITR for 14 days, monthly copies for a year; erasure requests must still hold after a restore. |
| How often is a single tenant restored? | Decides whether it needs a purpose-built path. | ~10 a day, mostly accidental deletes. |
| Who may restore into production? | A restore is the most destructive operation we have. | Two approvals; every restore audited. |
| Files in object storage? | Separate pipeline. | Out of scope here (versioning + replication; see §14). |

## 3. Requirements

**Functional**
- Continuous protection of the whole cluster; restore to any second in the last 14 days (PITR) and to monthly points for a year.
- Per-tenant export (portable, encrypted) and per-tenant restore from any point in the window, with a dry run first.
- Restores into a scratch instance for verification; a restore into production needs two approvals and a safety restore point.

**Non-functional**
- RPO ≤ 5 min; node/AZ loss recovered in minutes; a logical disaster in hours; a tenant in minutes.
- A backup nobody has restored does not count: restores are rehearsed on a schedule and measured.
- A compromised application or admin account cannot delete or read backups.

**Out of scope:** object-store files, multi-region active-active, application-level undo for users.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| WAL | ~20 MB/s ≈ 1.7 TB/day | 20 MB/s × 86,400 s (assumption) |
| Logical dump of everything | ≈ 11 h | 20×10⁶ MB ÷ 500 MB/s aggregate = 40,000 s |
| Logical restore | ~1–2 days | 2–5× the dump: indexes are rebuilt, constraints re-checked |
| Physical copy back, 10 Gbit/s | ≈ 4.4 h | 20×10⁶ MB ÷ 1,250 MB/s = 16,000 s |
| Copy back in 1 h | ≈ 5.6 GB/s ≈ 44 Gbit/s sustained | 20×10⁶ MB ÷ 3,600 s |
| Backup storage | ≈ 52 TB ≈ 2.6× the DB | 4 weekly fulls × 10 TB (2:1 compression, assumption) + 14 d WAL × 0.85 TB |
| One tenant export | 200 MB ≈ 4 s; a 20 GB tenant ≈ 7 min | at ~50 MB/s |
| Nightly export of every tenant | ≈ 28 h on one stream | 20×10⁶ MB ÷ 200 MB/s = 100,000 s |

What the numbers tell us: **no cold restore of 20 TB fits in one hour**; even a pure copy is 4+ hours before
WAL replay (which must be measured, not guessed). So RTO is a target *per failure class*: a hot standby for
node loss, a PITR clone (hours) for logical disasters, and a tenant-scoped path (minutes) for the common case.
Nightly per-tenant exports for everyone are a second full backup that does not fit in a night.

## 5. Plan

1. Failure classes → RPO/RTO each, and which mechanism answers which.
2. The backbone: hot standby + physical base backups + continuous WAL archive; tenant exports as a portable layer.
3. What is protected, what is rebuildable, and how we know nothing escaped classification.
4. Encryption and key custody (and what "key lost" means).
5. Off-site, immutable, independent copies.
6. Restore as the product: drills, verification, per-tenant restore, side effects, erasure.

## 6. High-level design

```
              ┌──────── primary (20 TB) ────────┐ streaming ┌── hot standby, other AZ ──┐
   app ──────►│ Postgres                        │──────────►│ promote in minutes (HA)   │
              └───┬───────────────┬─────────────┘           └─────────────┬─────────────┘
   WAL segments   │               │                 base backup taken from the standby:
   (archive, ≤60 s lag)           │                 weekly full + daily incremental
                  ▼               ▼
        backup agent: compress → encrypt (data key wrapped by KMS + offline escrow key) → upload
                  ▼
   ┌── SEPARATE backup account: object lock (WORM), write-only role for the agent ───────────┐
   │ bucket A (region 1): base/ wal/ manifests/   ◄── replicated ──►  bucket B (region 2)    │
   └──────────────┬──────────────────────────────────────────────────────────────────────────┘
                  ▼
   weekly full drill: base + WAL → scratch cluster → checks → timings → report
   tenant tools:  PITR clone on demand ──► extract tenant rows ──► merge into production (one tenant scope)
                  per-tenant logical export (on demand / paid tier)
```

**Backup:** WAL segments are archived as they fill (and every 60 s at most, so RPO is bounded by the timeout,
not by traffic); a base backup is taken from the standby so the primary does not pay for it; both are
encrypted, uploaded, copied to the second region, and recorded in a catalogue.
**Recover (logical disaster):** pick a target time, build a clone from the newest base before it, replay WAL to
the target, verify, then either promote it or *extract what is needed* from it and leave production running.

## 7. API

```
GET  /ops/backups                       → 200 [ { id, kind: "base"|"incremental", started_at, bytes, lsn_start, lsn_stop, verified_at, copies } ]
GET  /ops/recovery-window               → 200 { earliest, latest }              the PITR range we can honour right now
POST /ops/restores   { target_time, mode: "scratch"|"replace", reason }   → 202 { restore_id }     "replace" needs 2 approvals
POST /ops/restores/{id}/approve         → 200
GET  /ops/restores/{id}                 → 200 { state, step, copy_seconds, replay_seconds, checks }
POST /tenants/{tid}/exports             → 202 { export_id }
GET  /tenants/{tid}/exports/{eid}       → 200 { state, download_url (short-lived), sha256 }
POST /ops/tenants/{tid}/restore { from_restore_id, tables: [...], conflict: "keep_newer"|"overwrite", dry_run: true }
                                        → 202 { job_id }    dry run reports now → after row counts per table
```

## 8. Data model

The catalogue lives in a **separate small control-plane database** (and as manifest files next to the objects),
never inside the 20 TB cluster: it must survive the loss of the thing it describes.

```sql
CREATE TABLE backup_sets (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text   NOT NULL CHECK (kind IN ('base', 'incremental')),
    status      text   NOT NULL CHECK (status IN ('running', 'complete', 'failed', 'expired')),
    started_at  timestamptz NOT NULL, finished_at timestamptz,
    lsn_start   pg_lsn NOT NULL, lsn_stop pg_lsn,       -- WAL needed from lsn_start to be consistent
    bytes       bigint, sha256 bytea,
    key_id      text   NOT NULL                          -- id of the wrapped data key; never the key
);
CREATE TABLE backup_copies (
    backup_id   bigint NOT NULL REFERENCES backup_sets(id),
    location    text   NOT NULL,                         -- 'region-1', 'region-2'
    copied_at   timestamptz NOT NULL, verified_at timestamptz,
    PRIMARY KEY (backup_id, location)
);
CREATE TABLE restore_drills (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    backup_id    bigint NOT NULL REFERENCES backup_sets(id),
    target_time  timestamptz NOT NULL,
    copy_seconds integer, replay_seconds integer,        -- measured RTO ingredients
    checks       jsonb, ok boolean, finished_at timestamptz
);
CREATE TABLE erasure_ledger (                            -- survives every restore; re-applied after each one
    tenant_id   bigint NOT NULL, subject_ref text NOT NULL, erased_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, subject_ref)
);
```

## 9. Deep dives

### 9.1 Logical vs physical, and RTO per failure class

| Option | Pros | Cons |
|---|---|---|
| **Logical dump** (what the repo does: `COPY` per table in one snapshot) | portable across versions, selective by table, easy to read | 11 h to dump 20 TB; restore rebuilds every index (~days); a snapshot held 11 h on the primary pins dead rows (bloat), so run it on a standby |
| **Physical base backup + WAL archive** (PITR) | restore to any second; copy speed = network speed; no index rebuild | same major version, whole cluster only, needs WAL continuity |
| **Storage snapshots** | seconds to take | consistency and portability depend on the platform; not off-site by themselves |
| **A replica** | seconds to fail over | **not a backup**: `DROP TABLE` replicates in milliseconds |

| Failure | RPO | RTO | Answered by |
|---|---|---|---|
| Node or AZ loss | ~0 | minutes | hot standby promotion |
| Bad migration, dropped table | seconds (any point) | hours | PITR clone, then extract or promote |
| Corruption, ransomware | up to 5 min | hours | PITR clone from immutable copies in the other account |
| Account lost or compromised | up to 5 min | a day | second-account, second-region copies + escrow key |
| One tenant's mistake | any point | minutes | PITR clone kept warm / per-tenant extract |

**Choice and why.** Standby + physical base + WAL as the backbone; logical exports only per tenant and on
demand. The logical layer is what the repo ships, and it is the right one at 50 MB; at 20 TB it cannot meet any
RTO. RPO 5 min is met by archiving WAL at most every 60 s; a synchronous standby makes it zero for node loss.

### 9.2 What is protected, and how nothing escapes classification

The repo's best idea scales unchanged: *every table is in a backup group or on a written exclusion list, and a
test fails the build when a new migration adds a table in neither* (`TestEveryTableIsInABackupGroupOrExcluded`).
At scale the classes change meaning, because a physical backup takes the whole cluster:

- **Source of truth** (tenants, orders, content): in the cluster, backed up by default.
- **Rebuildable** (sessions, caches, search indexes, job queues): keep them *out* of the primary (Redis, a search
  cluster) or in unlogged tables, so the 20 TB backup does not carry them.
- **Compliance records** (the audit log): the repo excludes it as "logs"; at scale it is evidence, so it is
  backed up and shipped to its own write-once store.
- **Consistency:** a physical backup is consistent through WAL; a logical one needs a single snapshot (the repo
  uses one `REPEATABLE READ` transaction), which is why it must run on a standby or clone at this size.

### 9.3 Encryption and key custody

| Option | Pros | Cons |
|---|---|---|
| **Human passphrase → Argon2id → AES-GCM** (the repo) | no key infrastructure; *the passphrase alone* restores; a stolen file is closed | guessable offline if short (the repo's code accepts **4** characters, the owner's choice); one person's memory is the key |
| **Envelope: random data key per backup set, wrapped by a KMS key** | no human secret; rotate the wrapper without re-encrypting 52 TB; every unwrap is audited | KMS down or account lost = cannot read |
| **Client-side, offline key pair** (public key on the agent) | a hacked server cannot decrypt; survives loss of the cloud account | private key custody is a process, not a feature |
| Provider server-side encryption only | free | the provider's role can read everything |

**Choice: envelope with two wrappers.** A random 256-bit data key per backup set is wrapped by the KMS key (daily
use, drills) **and** by an offline escrow public key (Shamir-split among custodians, for the day the account is
gone); both wrapped copies sit in the archive header. **Key lost = data lost**, so the escrow path is tested once a
year by opening a real backup with nothing else. The stream format is the repo's, because it is right: a header
(magic, KDF or key id, chunk size, nonce prefix), then `length | ciphertext+tag` chunks of 64 KiB with a *final*
flag on the last one, so **truncation, reordering or a swapped chunk fail authentication**. That matters when a
10 TB upload can die at 99 %. Per-tenant exports use a per-tenant key: deleting it is *crypto-shredding*
(task [24](24-data-lifecycle.md)).

### 9.4 Off-site, immutable, independent

Threats: ransomware, a compromised admin or CI token, a bug that deletes backups, loss of the account, loss of a region.

| Option | Pros | Cons |
|---|---|---|
| Same bucket and account (the repo's first copy: *dies with the account*) | simple | one credential reaches data and backups |
| Second region, same account | survives a region | the same attacker deletes both |
| **Separate account + object lock (WORM) + write-only agent role + second region** | an attacker with the app's keys can add, not delete or read; deletion only by lock expiry | operational cost; restores need a human role with MFA |

**Choice: the third.** The repo's pattern is the seed: a B2 key that can only *add* objects, and a **read key typed by
the operator at restore time and never stored on the server**, so a stolen server holds only a key that can add files.
At scale: the agent role has `PutObject` and nothing else; the restore role is read-only, assumed with MFA and two
approvals; the object-lock retention equals the PITR window (14 days) and monthly sets lock for a year. 3-2-1 is
the minimum: 3 copies (primary + two regions), 2 kinds (standby stream, archived objects), 1 in another account.
Alert on **freshness**, not on "job succeeded": the repo's *last copy outside the server*, red after a day, becomes
"newest archived WAL is older than 5 minutes" (and `pg_stat_archiver` failures, because a stuck archive makes WAL pile up on the primary until its disk fills).

### 9.5 Restore is the product

| Check | How |
|---|---|
| It can be fetched | copy time measured and stored (`copy_seconds`) |
| It starts and reaches the target | recovery completes to `target_time` in a scratch cluster; `replay_seconds` measured |
| It is internally sound | `pg_verifybackup` on base sets, `pg_amcheck` (index/heap), per-table row counts against the catalogue, a sample of tenants queried through the app |
| It is fast enough | `copy_seconds + replay_seconds + verification` against the RTO of that class |

Drills: **weekly full restore** to a scratch cluster (hours: the copy alone is ~4.4 h; automated and timed every run), plus continuous per-segment checksums.
The repo says it plainly: its drill ran against a stub, *not* a real B2 account; the part no stub proves is the
provider's real behaviour. At scale a real-provider drill is a calendar item, not a to-do.

**Per-tenant restore** cannot reuse the repo's whole-table restore (it drops foreign keys, reloads the table,
resets sequences), because in a shared schema that would replace *every* tenant's rows. Instead: PITR clone →
`COPY (SELECT … WHERE tenant_id = $1)` per table → merge into production in one transaction with a conflict
policy (`keep_newer` or `overwrite`), a dry run first showing now → after counts as the repo's *Check* does.

**Side effects of going back in time** (each one a real incident somewhere):
- **Secrets** sealed under a key that is not in the backup are unreadable without it, by design (task [42](42-credentials-vault.md)).
- **Messages already sent**: a restored `outbox` row marked pending re-sends old emails. The repo leaves the
  queue out of backups and rebuilds provider state by *pulling* from the provider ([18](18-inbound-webhook-receiver.md));
  rule: every external side effect is idempotent ([10](10-webhook-delivery.md), [12](12-notification-service.md)), and a reconciliation runs after any PITR.
- **Erasures:** re-apply `erasure_ledger` after every restore, or deleted people come back.
- **Sessions and tokens:** invalidate all. **Sequences and counters:** set above the restored maximum (the repo calls `setval`).
- **Safety:** before a `replace` restore, `pg_create_restore_point('before_restore_<id>')`, so the restore itself can be undone.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (200 TB):** the weekly full no longer finishes (a 2-day job): move to incremental or
  block-level backups, and a cold restore becomes 40+ hours, so shard the database so each shard is backed up, drilled and restored on its own.
- **WAL archive stuck:** the primary keeps WAL until its disk is full. Alert on archive failures and lag long before that.
- **KMS or the key account down:** running backups use the data key already unwrapped for the run; restores fall back to the escrow key.
- **Backup agent compromised:** write-only role + object lock means it can poison *new* objects but not erase old ones; verify drills catch a poisoned set.
- **Catalogue lost:** manifests beside the objects rebuild it. **Clock skew:** PITR targets are in the database's time.
- **Restore itself fails halfway:** the restore point and the untouched backup remain; a restore never consumes its source.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Standby + physical base + WAL | logical dumps as the backbone | any-second PITR, network-speed restore | one major version, whole-cluster granularity |
| RTO per failure class | one RTO | honest, testable targets | explaining three numbers |
| Envelope keys + offline escrow | one human passphrase | rotation, audit, survives account loss | KMS and custody process |
| Separate account, object lock, write-only agent | same-account bucket | an attacker cannot delete or read backups | restore needs MFA + approvals |
| Tenant restore by extract and merge | whole-table replace | one tenant back, others untouched | a conflict policy to decide |
| Weekly full drill | trusting "job succeeded" | a measured RTO | scratch capacity every week |

## 12. Check yourself

1. Why can a 20 TB system not promise "RTO 1 h" with a cold backup, and what do we promise instead?
2. Why is a replica not a backup?
3. What does the *final* flag on the last encrypted chunk protect against?
4. Why must the backup agent's credential be write-only, and where does the read key live?
5. After a point-in-time restore, name three things that can go wrong outside the database.

<details><summary>Answers</summary>

1. Copying 20 TB back takes 4.4 h at 10 Gbit/s before WAL replay and verification; 1 h would need ~44 Gbit/s
   sustained. We promise RTO per failure class: minutes for node loss (standby), hours for logical disasters
   (PITR clone), minutes for a single tenant (extract and merge).
2. A replica applies every change, including a `DROP TABLE` or a bad `UPDATE`, within milliseconds. A backup
   is a point in the past that later mistakes cannot reach.
3. Truncation: a stream that stops early, or chunks reordered or swapped, decrypts piece by piece but never
   reaches a verified final chunk, so it fails instead of silently restoring a partial database.
4. So that whoever steals the app's or the server's credentials can add objects but not delete or read backups.
   The read key is held by a human role (MFA, two approvals) or typed at restore time; the repo never stores it on the server.
5. Messages restored as pending are sent again; secrets are unreadable if the sealing key is missing; erased
   people reappear unless the erasure ledger is re-applied; sessions are valid for a world that no longer exists;
   counters and sequences may be below the maximum already handed out.

</details>

## 13. In Go

- Related lessons: [62 Deployment & operations](../../62-deployment-operations.md) (running agents, health, runbooks), [59 Production DB patterns](../../59-production-database-patterns.md) (advisory locks so a restore and a backup never overlap, `REPEATABLE READ` snapshots), [44 Background jobs](../../44-background-jobs-queues.md) (scheduled runs, one at a time), [57 Web security](../../57-web-security.md) (secrets and key handling), [34 Outbox](../../34-event-driven-outbox.md) (why the queue is not restored).
- The Go parts: a streaming `io.Pipe` pipeline `COPY → compress → AEAD chunks → upload` that never holds more than a chunk in memory; `pgx`'s `CopyTo`; `golang.org/x/crypto/argon2` for passphrase keys and `crypto/cipher` (`NewGCM`) for chunks (nonce = random prefix + chunk counter); `github.com/minio/minio-go/v7` for S3-compatible stores.
- The backbone is operations software, not application code: WAL archiving and base backups run through tools such as pgBackRest or WAL-G (WAL-G is itself written in Go); your service orchestrates, catalogues and verifies them.

## 14. Extensions

- **Object-store files:** versioning plus soft delete plus cross-region replication, and a manifest that ties rows to objects (the repo copies new and changed objects only and never deletes in the bucket).
- **Tenant-owned keys (BYOK):** a customer revokes their key and their backups become unreadable.
- **Per-tenant PITR from a change log:** logical decoding into an immutable log so one tenant can be replayed to any second without a full clone.
- **Game day:** lose the primary account on purpose; time the rebuild; update the runbook (what must live in a password manager, in what order to rebuild: the repo's *Total loss* runbook).
- **Cost:** tiering old base sets to cold storage, dedup between incrementals, egress-aware restore planning.
