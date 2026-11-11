# 25 · Contact Deduplication & Merge

`🟢 easy` · *Topics:* what a unique constraint can and cannot see, normalisation and match keys (email, E.164 phone), find-or-create races, blocking instead of O(n²) comparison, merging with tombstones, redirects and an undo window, auto-merge vs suggest-merge · *Builds on:* [04 Unique ID Generator](04-unique-id-generator.md) (ids that outlive a merge), [10 Webhook Delivery](10-webhook-delivery.md) (outbox for the matcher), [24 Data Lifecycle](24-data-lifecycle.md) (tombstones, purge of undo data)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [24 Data Lifecycle: Archive, Delete, Erase](24-data-lifecycle.md) · Next: [26 Configurable Status Workflow](26-status-workflow.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 13 made `clients.email` unique ("an email is a person", `citext`), gave a client any number of phones (`client_phones`, one primary, a number *not* unique across clients: a household landline), and put the login on `users.client_id`. Every place that creates a client looks for the email first; two checkouts racing with one new email are settled by the unique index (the loser re-matches). Merge covers "what a constraint cannot see" (one person, two emails): it moves orders, invoices, phones, logins and the provider's records, fills but never overwrites fields, then deletes the other client. Measured at 200,000 clients: the match as a union of two indexed lookups took 0.13 ms; written with `OR` it took 360 ms. Step 64 then made every request create a client by default, so the table fills with enquiries (and spam).
> **Here, at scale:** a multi-tenant CRM with 500M contacts, 5k find-or-create calls a second at peak, imports of 10M rows, and about 8 % duplicates (40M) to find and merge safely.

## 1. The task

> "Design the contact store of a CRM. Contacts arrive from forms, imports and integrations, often
> the same person many times. Keep one contact per person, find duplicates we could not prevent,
> and let people merge them without losing history."

Two problems that look like one: **preventing** duplicates at write time (cheap, exact) and
**repairing** those that slipped through (fuzzy, risky, needs undo).

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Unique by what? | Decides the constraint. | Email is the person (per tenant); phones may be shared; names are never keys. |
| Several emails and phones per contact? | Changes the schema. | Yes: both are child rows; one primary each. |
| Auto-merge or human review? | Cost of a wrong merge. | Only very high confidence is automatic; the rest is a review queue. |
| Can a merge be undone? | Storage and design of the merge. | Yes, for 30 days. |
| Do old ids and links keep working? | Bookmarks, emails, API clients. | Yes: a merged id redirects to the survivor. |
| Scale? | Algorithms. | 500M contacts, 5k find-or-create/s peak (1.5k avg), 10M-row imports. |

## 3. Requirements

**Functional**
- `find-or-create` by email or phone: idempotent, race-safe, returns whether it matched.
- Detect likely duplicates (fuzzy) and show them with reasons and a score.
- Merge two contacts: survivor chosen, fields combined, every child record repointed, undoable.
- Bulk import with a dedupe preview (new, matched, conflicts) before anything is written.

**Non-functional**
- find-or-create p99 under 20 ms; no duplicate from concurrent callers with the same email.
- A merge is atomic: no half-moved contact. Old ids and emails keep resolving.
- A wrong merge is the expensive mistake (it mixes two people's private data): prefer a missed duplicate.

**Out of scope**
- Company/household graphs, third-party enrichment, matching across tenants.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Contacts | 500M | given |
| Phones | ~650M | 1.3 per contact (seed: 260k numbers for 200k clients) |
| Storage | contacts ~0.5 TB; phones ~145 GB | 1 KB per contact; ~223 B per phone incl. indexes (seed: 19 MB + 39 MB ÷ 260k) |
| find-or-create | 5k/s peak, ~1.5k/s avg | given; peak ≈ 3.3× |
| New contacts | ~3/s avg ≈ 95M/yr; ~10/s at peak | only 0.2 % of calls create; the rest touch known contacts |
| Import | 10M rows ≈ 33 min | 5k rows/s into staging, then one set-based match |
| Expected duplicates | ~40M | 8 % of 500M (assumption) |
| All-pairs comparison | 1.25 × 10¹⁷ | n²/2 for n = 500M: impossible |
| Blocked pairs | ~25B | 5M blocks of ~100 contacts × ~5,000 pairs each |
| Blocked scan time | ~13 min | 25B × 1 µs ÷ 32 cores |
| Incremental match | ~100–300 comparisons per new contact | only its own blocks |
| Merge cost | ~20 ms | ~20 child rows moved; 40M merges ≈ 9 days of one worker, ~1 day on 10 |

What the numbers tell us: the write path is small (tens of creates per second, indexed probes).
The expensive idea is the *full scan*; do it incrementally per new or changed contact and keep
the full blocked scan for backfills and imports. Merge cost is per-contact and parallel.

## 5. Plan

1. Normalise identifiers into match keys; decide which keys are unique and which are only hints.
2. Make find-or-create race-safe with the unique key as the arbiter.
3. Find fuzzy duplicates with blocking keys and scoring, incrementally.
4. Design the merge: survivor, fields, children, tombstone, aliases, redirect, undo.
5. Choose the auto-merge policy and the review queue.

## 6. High-level design

```
 forms / API / syncs ──► Contact service: find-or-create
 CSV import ─► staging ─►   1 normalise (email lower-case, phone → E.164)
                            2 probe contact_emails (tenant_id, email)  ← the unique arbiter
                            3 phone-only: advisory lock on (tenant, e164), probe, insert
                            ▼
                      Postgres: contacts · contact_emails · contact_phones · child tables (deals, notes…)
                            │ outbox: contact.created / contact.changed
                            ▼
              matcher worker ── blocking keys ─► candidates ─► score ─► duplicate_candidates
                                       ≥ 0.97 and two independent signals ──► auto-merge
              merge worker ── lock both (id order) ─► repoint children ─► tombstone ─► contact_merges (30-day undo)
 UI: review queue · "not a duplicate" (suppresses the pair) · GET /contacts/{loser} → 308 to survivor
```

## 7. API

```
POST /contacts:upsert  { email?, phone?, name, source }   Idempotency-Key → 200 | 201 { contact, matched_on: "email"|"phone"|null }
GET  /contacts/{id}                                → 200 | 308 Location: /contacts/{survivor}   (a merged contact redirects)
GET  /contacts/{id}/duplicates                     → 200 [{ other_id, score, reasons: ["same email domain", "name ~ 0.94"] }]
POST /contacts/{survivor}/merge  { loser_id, pick?: { field: "survivor"|"loser" } }  → 200 { merge_id, undo_until } | 409 { reason }
POST /merges/{id}/undo                             → 200 | 409 { reason: "survivor changed the moved rows" | "window closed" }
POST /duplicates/{pair_id}/dismiss                 → 204    ("not a duplicate": never suggested again)
```

## 8. Data model

```sql
CREATE TABLE contacts (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id   bigint NOT NULL,
  public_id   text   NOT NULL UNIQUE,                 -- what URLs and APIs show
  name        text,
  source      text   NOT NULL,                        -- form | import | api | sync
  merged_into bigint REFERENCES contacts(id),         -- tombstone: NULL = a live contact
  merged_at   timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (merged_into IS NULL OR merged_into <> id)
);

-- "an email is a person": unique per tenant. Lives in a child table so a merged contact's emails
-- keep pointing at the survivor and the next form from that address does NOT recreate the duplicate.
CREATE TABLE contact_emails (
  tenant_id  bigint NOT NULL,
  email      citext NOT NULL,
  contact_id bigint NOT NULL REFERENCES contacts(id),
  is_primary boolean NOT NULL DEFAULT false,
  PRIMARY KEY (tenant_id, email)
);
CREATE UNIQUE INDEX contact_emails_primary ON contact_emails (contact_id) WHERE is_primary;

CREATE TABLE contact_phones (                          -- NOT unique across contacts: a household or an office line
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id  bigint NOT NULL,
  contact_id bigint NOT NULL REFERENCES contacts(id),
  e164       text   NOT NULL,                         -- +13152243877
  label      text,
  is_primary boolean NOT NULL DEFAULT false,
  UNIQUE (contact_id, e164)
);
CREATE INDEX contact_phones_lookup ON contact_phones (tenant_id, e164);

CREATE TABLE duplicate_candidates (
  tenant_id bigint NOT NULL, a_id bigint NOT NULL, b_id bigint NOT NULL CHECK (a_id < b_id),
  score real NOT NULL, reasons jsonb NOT NULL, status text NOT NULL DEFAULT 'open',  -- open | merged | dismissed
  PRIMARY KEY (tenant_id, a_id, b_id)
);
CREATE INDEX duplicate_candidates_queue ON duplicate_candidates (tenant_id, score DESC) WHERE status = 'open';

CREATE TABLE contact_merges (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL,
  survivor_id bigint NOT NULL, loser_id bigint NOT NULL, merged_by bigint,
  merged_at timestamptz NOT NULL DEFAULT now(), undo_until timestamptz NOT NULL,
  loser_snapshot jsonb NOT NULL,                       -- every field, for undo and the audit
  moved jsonb NOT NULL                                 -- { "deals": [ids…] }: dropped after the window
);
```

## 9. Deep dives

### 9.1 What a unique constraint can and cannot see

`UNIQUE (tenant_id, email)` stops one email being two contacts. It cannot see that
`dana@example.com` and `dana.fields@acme.example` are one person, nor that a phone-only contact is the
person who later wrote from an email. So decide, per identifier, whether it is a **key** (enforced)
or a **hint** (used for suggestions):

| Identifier | Normalisation | Unique? | Auto-merge on it alone? |
|---|---|---|---|
| Email | trim, lower-case (`citext`) | **yes**, per tenant | n/a: the constraint already makes it one contact |
| Email "match key" (strip `+tag`, Gmail dots) | derived, stored separately | no | no: `john+work@` is sometimes deliberately separate |
| Phone | **E.164** with the tenant's default region | no (shared lines) | no |
| Name | case-fold, strip punctuation, phonetic key | no | never |

Phones need real normalisation: the seed compares digits only (`regexp_replace(phone, '\D', '', 'g')`),
fine for one country, wrong once `315 224 3877` and `+1 315 224 3877` and `+44…` meet. Parse with a
libphonenumber port and store E.164. And a phone **is not a person**: two family members share one;
"the email decides" is the seed's rule, and a phone match alone only creates a *suggestion*.

### 9.2 Finding fuzzy duplicates without comparing everyone to everyone

| Option | Pros | Cons |
|---|---|---|
| All pairs | simplest, most complete | 1.25 × 10¹⁷ comparisons for 500M: impossible |
| Exact keys only (hash join on email/phone) | cheap, precise | misses typos, name variants, different emails |
| **Blocking + scoring** | only compares contacts that share a cheap key; ~25B pairs for a full scan, ~100–300 for one new contact | needs good blocking keys; a pair that shares none is never seen |
| Embeddings / LSH | catches fuzzy similarity | model, index and tuning; hard to explain a score to a human |

**Choice and why.** Blocking + scoring, **incrementally**. On `contact.created/changed` the matcher
computes blocking keys (`metaphone(last_name) + postal_prefix`, `email domain + 4-char local-part
prefix`, `last 7 phone digits`) and does an indexed lookup per key; a block over ~1,000 is **ignored
as non-discriminating**. Candidates are scored (name similarity, domain, address, phone) and written
to `duplicate_candidates` with human-readable `reasons`. The full scan runs only for backfills and
imports. Seed lesson: match in SQL with **a union of equality lookups, never `OR` across tables**
(0.13 ms against 360 ms at 200k rows).

### 9.3 Concurrent creators: let the unique key decide

`SELECT … if none INSERT` races: two forms with a new email both see "none". Make the unique key the
arbiter and treat a lost race as a match (the seed: the second insert "matches again and takes the first"):

```sql
-- 1. find
SELECT contact_id FROM contact_emails WHERE tenant_id = $1 AND email = $2;
-- 2. none: ONE transaction
INSERT INTO contacts (tenant_id, public_id, name, source) VALUES ($1, $3, $4, $5) RETURNING id;           -- c
INSERT INTO contact_emails (tenant_id, email, contact_id, is_primary) VALUES ($1, $2, c, true)
  ON CONFLICT (tenant_id, email) DO NOTHING RETURNING contact_id;
-- 0 rows: someone else won. ROLLBACK (the orphan contact vanishes) and run step 1 again.
```

| Option | Pros | Cons |
|---|---|---|
| Check, then insert | simple | the race above |
| **Unique index + rollback/re-find** (emails) | correct, no extra locks | one wasted insert on a lost race |
| Advisory lock `pg_advisory_xact_lock(hashtextextended(tenant \|\| ':' \|\| e164, 0))` (phone-only) | serialises creators of one phone without a unique key | a hash collision only serialises two unrelated keys: harmless |

**Choice and why.** The unique index for email, an advisory lock for phone-only creators, and an
`Idempotency-Key` ([16](16-public-form-intake.md)) so a retried form re-finds its own contact. Since
the seed makes *every request* create a contact, this path is also an abuse surface: stop bots before
they create rows ([17](17-abuse-protection.md)), and let a spam verdict remove the contact it made.

### 9.4 The merge itself

| Option | Pros | Cons |
|---|---|---|
| Hard-delete the loser (the seed) | simplest; no leftovers | irreversible; old ids and emails dangle |
| **Tombstone + aliases + redirect + undo window** | old ids redirect; emails resolve to the survivor; undo possible | more tables; a retention job |
| Never merge: link both into a "same person" cluster, union at read time | non-destructive | every query pays; two records still to maintain |

**Choice and why.** Tombstone. In one transaction:

```sql
BEGIN;
SELECT id FROM contacts WHERE id IN ($survivor, $loser) ORDER BY id FOR UPDATE;  -- same order everywhere: no deadlocks
-- both live, same tenant, else ROLLBACK + 409
UPDATE contact_emails SET contact_id = $survivor, is_primary = false WHERE contact_id = $loser;  -- aliases keep resolving
-- phones: move the ones the survivor lacks (UNIQUE (contact_id, e164)), drop the rest
UPDATE deals SET contact_id = $survivor WHERE contact_id = $loser;       -- one statement per registered child table
UPDATE contacts SET merged_into = $survivor, merged_at = now() WHERE id = $loser;
UPDATE contacts SET merged_into = $survivor WHERE merged_into = $loser;  -- flatten chains A→B→C
INSERT INTO contact_merges (...);                                        -- snapshot + moved ids, undo_until = now() + 30 days
COMMIT;
```

Fields are filled, **never overwritten**; conflicts are shown to the person merging. **Children are a
registry**: a test lists every foreign key to `contacts` from `information_schema` and fails if a table
is unregistered, so no table silently keeps pointing at a tombstone (the seed's backup test does the
same). Undo works while the moved rows are unchanged; `moved` ids are deleted after 30 days
([24](24-data-lifecycle.md)). A contact with a million child rows needs a different path: see §10.

### 9.5 Auto-merge or suggest

Costs are asymmetric: a **missed** duplicate means two emails to one person; a **wrong** merge shows
one person's history to another, a privacy incident that undo only partly repairs.

| Policy | Pros | Cons |
|---|---|---|
| Always manual | safest | 40M duplicates never get reviewed |
| Always automatic above a score | clears the backlog | one false positive is an incident |
| **Tiered** | safe and scalable | thresholds need tuning and metrics |

**Choice and why.** Tier 1 is not a merge: same email = same contact by constraint. Tier 2: score
≥ 0.97 **and at least two independent signals** (e.g. same name + same verified phone), auto-merged
with the 30-day undo and a notification. Tier 3: 0.80–0.97 goes to the review queue, best score
first, with reasons. Below that: ignore. A phone match is only used when the existing contact has
no email (the seed's checkout rule). "Not a duplicate" is stored so a pair is never suggested twice.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the matcher backlog after a big import, not the write path.
  Throttle with the outbox, scan imports in blocked batches off-peak, and shard the matcher by
  `tenant_id`. The ~10k probes/s of today fit one primary plus replicas; 10× is a sharding conversation.
- **Giant contacts:** a million child rows in one transaction holds locks for a minute. Mark the
  contact `merging`, repoint in batches of 5,000, resolve `merged_into` at read time until done.
- **Merge chains and cycles:** flatten on every merge; the `CHECK` stops self-reference; a nightly query catches loops.
- **Duplicate storms:** a bot or bad integration creating contacts at 5k/s: rate-limit per tenant and source ([03](03-rate-limiter.md)); clean up by `source`.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Email unique, phone not | uniqueness on both | shared lines work; the constraint is cheap and exact | phone-only duplicates need the matcher |
| Emails in a child table | one `contacts.email` | merged emails keep resolving | a join on every lookup |
| Blocking + scoring, incremental | all-pairs scans | ~100–300 comparisons per change | pairs sharing no block are missed |
| Tombstone + undo window | hard delete | redirects, undo, audit | extra tables, a retention job |
| Tiered auto-merge | all manual / all auto | safe at 40M duplicates | threshold tuning |

## 12. Check yourself

1. Why can `UNIQUE (email)` not prevent duplicate contacts? Give three cases it misses.
2. After merging B into A, B's email must keep resolving to A. Why, and how does the schema make it so?
3. Why not compare every pair of contacts? How does blocking change the numbers, and what runs per new contact?
4. Two forms submit the same new email at the same instant. What happens, step by step? And two phone-only submissions?
5. Which merges do you automate, and why is the policy asymmetric?

<details><summary>Answers</summary>

1. Different emails for one person (`dana@home` / `dana@work`); a phone-only contact who later gives
   an email; spelling or plus-address variants (`dana+shop@…`) that are different strings. A
   constraint compares exact values only.
2. Otherwise the next form from B's address finds no contact and creates a new duplicate, undoing the
   merge. With emails in `contact_emails (tenant_id, email)`, the merge just repoints B's rows to A, so
   the same unique key now resolves to A.
3. n²/2 = 1.25 × 10¹⁷ pairs. Blocking gives ~5M blocks of ~100 → ~25B pairs for a *full* scan
   (~13 min on 32 cores). Normally it is incremental: a new contact meets only its blocks, ~100–300 comparisons.
4. Both look up, find nothing, both insert; the primary key on `contact_emails` admits one, the other
   gets zero rows from `DO NOTHING`, rolls back and re-finds the winner. Phone-only has no unique
   key, so an advisory lock on (tenant, e164) serialises lookup-then-insert.
5. Automate only a score ≥ 0.97 with two independent signals (undo and a notice); queue the middle
   band for humans; ignore the rest. A missed duplicate costs a second email; a wrong merge exposes
   one person's data to another.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md), [59 Production DB patterns](../../59-production-database-patterns.md) (`ON CONFLICT`, advisory locks, `FOR UPDATE` ordering, `CopyFrom` for imports), [53 Graphs](../../53-graphs-algorithms.md) (union-find: transitive duplicate clusters A≈B, B≈C), [34 Outbox](../../34-event-driven-outbox.md) (feeding the matcher), [55 Data pipelines](../../55-data-pipelines.md) (the import → preview → apply flow), [18 Testing](../../18-testing.md) (the foreign-key registry test).
- Match keys, parsed once at the edge:

```go
func MatchKeys(email, phone, region string) (emailKey, e164 string, err error) {
	emailKey = strings.ToLower(strings.TrimSpace(email))
	if phone == "" {
		return emailKey, "", nil
	}
	num, perr := phonenumbers.Parse(phone, region) // region = the tenant's default, e.g. "US"
	if perr != nil {
		return "", "", perr
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", "", errors.New("invalid phone number")
	}
	return emailKey, phonenumbers.Format(num, phonenumbers.E164), nil
}
```

- Packages: `pgx` (`CopyFrom`, `pg_advisory_xact_lock`), `github.com/nyaruka/phonenumbers`, a Jaro-Winkler implementation (`github.com/xrash/smetrics`), `golang.org/x/sync/errgroup` for the blocked scan.

## 14. Extensions

- **Households and companies:** link contacts to an account instead of merging people.
- **Probabilistic matching:** learn the score weights from the review queue's accept/dismiss decisions.
- **Cross-system ids:** the seed re-points the invoicing provider's customer record on merge so new invoices still reach the right customer; do the same for every integration.
