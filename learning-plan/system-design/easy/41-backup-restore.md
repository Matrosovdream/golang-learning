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

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). The production primary is outside section 4's numbers; assume `r7g.8xlarge` (32 vCPU / 256 GiB) with 20 TB of gp3, and price only what backup and restore *add*.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Hot standby (other zone), also the base-backup source | replay 20 MB/s of WAL; weekly full read at 500 MB/s ≈ 11 h (§4's rate) | memory + IOPS like the primary; during a backup, EBS throughput and network | 20 TB is over one gp3 volume (16 TiB max), so 2 × 10 TB striped, 16,000 IOPS. 500 MB/s = 4 Gbit/s sustained upload (the sheet: sustained multi-Gbps needs an `8xlarge`+). Compress + encrypt at ~150 MB/s a vCPU (assumed) ≈ 4 of 32 vCPU | `r7g.8xlarge` EC2 with pgBackRest or WAL-G; managed: RDS/Aurora Multi-AZ with automated backups (PITR to 5 min) + AWS Backup cross-account copy, ≈ 2× on the instances | +1 (primary exists) | `r`, same as the primary: a smaller standby cannot take over its load; the agent runs here so the primary pays nothing and no agent fleet exists |
| Delayed replica for tenant restores | ~10 restores a day, each ~200 MB (≈ 4 s) | disk (a full 20 TB copy) and WAL replay | replay 20 MB/s is well under one core's ~100 MB/s (assumed); extracts are trivial. Kept ~24 h behind (older points need a PITR clone: hours). An on-demand clone ≈ 4.4 h × ($1.70 + $2.19) ≈ $17 a restore (copy only), the replica costs $2,235 ⇒ it pays off above ≈ 130 restores a month; §2's ~10 a day is ≈ 300 | `r7g.4xlarge` (16 vCPU / 128 GiB) + 20 TB gp3 at 6,000 IOPS | 1 | RAM only for the tenants being read; "minutes" for the common case is bought with a second full copy |
| Weekly drill cluster | copy 4.4 h, replay ≤ 1 day of WAL, then verify | network/EBS copy, then single-process replay | 4.4 h + ~4.7 h (1.7 TB at an assumed 100 MB/s) + verification ≈ 24 h a week ≈ 104 h a month | `r7g.8xlarge` + 20 TB gp3, on-demand, only while it runs | 1, ephemeral | a drill the budget skips is not a backup; a Spot reclaim would waste a 24 h run |
| Catalogue DB (control plane) | a few rows a day | nothing; availability | tiny | `db.m7g.large`, RDS Multi-AZ | 1 + standby | separate from the 20 TB cluster so it survives losing it |
| Backup storage (S3, two regions, object lock) | WAL ≈ 1.2 PUT/s (20 MB/s ÷ 16 MiB segments), far under 3,500/s | nothing to size; bytes cost | 52 TB (§4) + 12 monthly sets × 10 TB = 172 TB a region × 2 = 344 TB | S3 + a gateway endpoint (no NAT) | none | not an instance question: storage and the copy are the bill |

Not tiny hardware: two more 20 TB copies run all month because no cold restore of 20 TB meets an hour (§4: 4.4 h copy at 10 Gbit/s, ≈ 44 Gbit/s sustained for 1 h, more than one instance's EBS bandwidth is likely to sustain: check its limit).

**Rough monthly bill (≈, on-demand); the primary (≈ $2,900) is not counted**

| Line | ≈ $/month |
|---|---|
| Hot standby: `r7g.8xlarge` ($1,240) | $1,240 |
| Standby disk: 20 TB × $0.08 = $1,600 + 13,000 IOPS above 3,000 × $0.005 = $65 | $1,665 |
| Delayed replica: `r7g.4xlarge` ($620) + 20 TB gp3 ($1,600) + 3,000 IOPS ($15) | $2,235 |
| Weekly drill: 104 h of `r7g.8xlarge` ($177) + 20 TB gp3 for those hours ($228) | $405 |
| Catalogue DB: `db.m7g.large` $120, Multi-AZ doubles | $240 |
| S3 backups: 344 TB × 1,000 × $0.023 | $7,912 |
| Cross-region copy: ~66 TB a month (4 × 10 TB fulls + 26 TB WAL) × $0.02 | $1,320 |
| **Total** | **≈ $15,000** (sum $15,017; ≈ $14,300 with a 1-year commitment on the $2.1k of steady compute) |

- **Biggest line and why:** S3 backup storage, 53 %, because the backups are 17× the database: two regions, a year of monthly full sets (120 TB a region) and 4 weekly fulls. Levers: monthly sets are write-once and rarely read, so an archive storage class costs a fraction (the sheet lists no price; check it), and incrementals instead of 10 TB monthly fulls. The two standing 20 TB replicas (standby and delayed) are the next 34 %.
- **What changes at 10×:** 200 TB and 200 MB/s of WAL, which is above the assumed ~100 MB/s of replay, so the standby and delayed replica fall behind; a full backup takes 110 h. One cluster is no longer recoverable: shard into ~10 databases of 20 TB, each with its own lineage, drill and replicas (≈ $5.5k each ≈ $55k), plus S3 ≈ $79k and cross-region ≈ $13k, roughly $150k a month.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:** here the data is the database itself, so the choice is which engine's backup tools meet the targets of 9.1, plus where the small catalogue lives.
- A 20 TB cluster holding 100k tenants in a shared schema, ~20 MB/s of WAL (~1.7 TB/day); a logical dump takes ~11 h and a logical restore 1–2 days, while a physical copy back is ~4.4 h at 10 Gbit/s plus WAL replay (section 4).
- Targets per failure class: RPO ~0 and minutes for node loss, 5 min and hours for a logical disaster, minutes for one tenant.
- Two small stores beside it: the backup catalogue (a few rows a day, kept apart from the 20 TB cluster) and the backup bytes (~52 TB a region of base sets and WAL, immutable).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (EC2 as in section 15, or RDS / Aurora) | ✅ chosen: the engine under backup and the catalogue's engine | a physical base backup (`pg_basebackup`, pgBackRest or WAL-G) plus continuous WAL archiving gives restore to any second (`recovery_target_time`) at network speed with no index rebuild; `archive_timeout = 60 s` bounds the RPO; `pg_create_restore_point`, `pg_verifybackup` and `pg_amcheck` are the drill's checks; DDL is transactional, so a migration that fails halfway leaves nothing behind, which shrinks the "bad migration" row of 9.1 |
| MySQL (self-run or RDS / Aurora) | ✔ works, same shape with different tools | a physical backup (Percona XtraBackup, or an RDS/Aurora snapshot) plus archived binary logs replayed with `mysqlbinlog` to a time or GTID; logical dumps with `mysqldump --single-transaction` or MySQL Shell (the analogue of the lesson's single `REPEATABLE READ` snapshot). DDL commits implicitly, so a bad migration cannot be rolled back and leans on PITR more; the catalogue's `pg_lsn` columns become a binlog file and position or a GTID set |
| RDS / Aurora automated backups (either engine) | ✔ the managed alternative | snapshots and PITR (to 5 min, section 15) with a cross-account copy through AWS Backup, and no agent or WAL archive to run. You give up the 60 s archive timeout, the KMS-plus-offline-escrow envelope of 9.3 and control over where each object lands. A restore always creates a new instance or cluster, so a tenant restore is still extract-and-merge. Aurora MySQL alone also offers Backtrack, an in-place rewind |
| S3 with Object Lock (second account, two regions) | ✅ chosen for the backup bytes | write-once retention equal to the PITR window, a write-only agent role and cross-region replication are what ransomware and a compromised admin demand (9.4); no database offers that |
| DynamoDB | ✔ for the catalogue only, ✖ for the 20 TB | a few rows a day by id with built-in PITR, but the manifests beside the objects already rebuild the catalogue and it would be a third technology to run; as the 20 TB store it changes nothing, since its PITR also restores into a new table |

**Pick:** PostgreSQL as the lesson states it: the 20 TB primary and its hot standby on EC2 `r7g.8xlarge` (section 15), pgBackRest or WAL-G running on the standby so the primary pays nothing, WAL archived at most every 60 s, base sets written to S3 Object Lock in a second account and region; the catalogue in a separate RDS PostgreSQL Multi-AZ `db.m7g.large`. What decides it: WAL archiving with point-in-time recovery, the verification tools of 9.5, and owning the archive, the keys and the destination account. The cluster is the system of record; the archives are copies; the catalogue is metadata the manifests beside the objects can rebuild.

**If it were MySQL instead:** the same architecture: XtraBackup base sets taken from a replica, binlogs archived continuously (for example `mysqlbinlog --read-from-remote-server`) to the same locked bucket, replay to a target time in the drill, measured as `replay_seconds`. What changes: other tools and catalogue columns, one more reliance on PITR because DDL is not transactional, and the tenant extract is a `SELECT` from the PITR clone merged back with `INSERT … ON DUPLICATE KEY UPDATE` instead of `COPY (SELECT …)`. About the same effort.

**What would flip the choice:** size and RTO, not features. At 10× (200 TB, 200 MB/s of WAL) one cluster is no longer recoverable (section 15), so shard into ~10 databases of 20 TB, each with its own lineage and drill, before considering another engine. Choose RDS or Aurora when the team will not run pgBackRest or WAL-G and can live with a fixed archive schedule; Aurora's continuous backup sits in its storage layer (which grows to 128 TiB, cheat sheet), so there is no agent or WAL archive to run, though 200 TB exceeds that too.
