# 52 · Configurable Forms

`🟢 easy` · *Topics:* a form as versioned data, fields keyed by stable ids, closed set of field types owned by code, conditional visibility, one schema driving client and server validation, answers as JSONB tagged with the schema version, why not a table per form or EAV, evolving a live form without breaking old submissions, serving an immutable schema from a CDN, embeds and CORS · *Builds on:* [16 Public Form Intake](16-public-form-intake.md) (idempotency, retries, drafts, abuse: not repeated here), [26 Configurable Status Workflow](26-status-workflow.md) (what the code owns vs what the tenant owns), [09 Feature Flags](09-feature-flags.md) (snapshots pushed to the edge)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [51 Runtime Settings & Timed Kill Switches](51-runtime-settings.md) · Next: [53 Web Push & the Installable App (PWA)](53-web-push-pwa.md)

---

> 🌱 **Seed (honeydolist-site):** the site has five public forms (service quote, product quote, contact page, reserve a machine, checkout; PLAN steps 36–38). Their fields are **the code's** (`settingsmodel.FormSpecs`); Settings → Forms lets an Admin flip two switches per field, *Shown* and *Required*, and nothing else (no new fields, labels or order). The result is one JSON object per form in the `settings` table (`forms.quote`, `forms.rental`, … field → `{visible, required}`), **no migration**; a missing row means the shipped default. A save is versioned (a stale save is a `409`) and audited (`forms.changed`). Some fields are drawn *Always* (name, phone, a delivery's address), and a hidden field can never be required. The server enforces it (`intake.Prepare`): a field the form no longer asks is **dropped** even if a stale open page sends it, and an empty required one is a `422` on that field. Answers land in **typed columns** (`requests`, `request_quotes`, migration 0010), and the photo row on the quote form (step 83) is deliberately *not* a configurable field.
> **Here, at scale:** a form-builder product, 1M tenants × ~5 forms, tenants adding and re-ordering their own fields, ~20k submissions/s on average and 200k/s at peak, forms edited while live and embedded on other people's sites.

## 1. The task

> "Design a form builder. A tenant builds a form out of fields (text, email, choice, file…),
> says which are required and when a field shows, embeds it on their website, and gets the
> submissions to search and export. They edit the form whenever they like, including while people
> are filling it in."

The honeydolist version is a *settings page*: the fields are fixed, only visibility and
required-ness move. A real builder lets the tenant invent the fields. The interesting parts are
what that does to storage, validation and **old data** when the form changes tomorrow.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Can tenants add fields, or only toggle fixed ones? | Fixed fields can be typed columns; invented fields cannot. | They add, remove, re-order fields from a closed set of types. |
| Which field types? | The set code must validate. | text, long text, email, phone, number, date, single/multi choice, checkbox, file. No custom code per field. |
| Conditional fields? | Decides the schema language and the validation order. | "Show field B when field A equals X", on earlier fields only. No loops, no scripts. |
| Edit while live? What about submissions in flight? | Version semantics: the hardest part. | Yes. A form loaded before an edit can still be submitted. |
| Old submissions after a field is removed or changed? | Storage and export. | Kept as they were submitted, exportable with the columns they had. |
| Embeds on other sites? | CORS, caching, framing, abuse. | Script or iframe embed; submissions cross-origin. |
| Search and export? | Typed columns vs documents. | List by form and time, filter on a few fields, CSV export of everything. |
| Scale? | Shapes the write path and the schema cache. | 1M tenants × ~5 forms, ~20k submissions/s average, 200k/s peak (ticket drops, sales); 10 form views per submission. |
| Spam, retries, idempotency? | Already solved. | As in [16](16-public-form-intake.md) and [17](17-abuse-protection.md). |

## 3. Requirements

**Functional**
- Create, edit, publish, pause and archive forms; fields from a closed type set, each with a label, help text, required flag and optional `show_if`.
- Public render and submit endpoints; validation identical in the browser and on the server.
- Every submission records which **version** of the form it answered.
- Submissions list (per form, newest first), filter on a few fields, export to CSV.

**Non-functional**
- Rendering a form is a cache hit almost always: 2M views/s at peak is a CDN problem.
- A published form version never changes; an edit makes a new version.
- A submission against a version retired a minute ago is still accepted.
- Tenants cannot affect each other (hot form, huge form, hostile schema).

**Out of scope**
- Payments, e-signatures, multi-page wizards with branching, scripted logic, template marketplace.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Forms | 5M | 1M tenants × 5 |
| Schema versions | ~50M rows, ~250 GB | ~10 versions per form × ~5 KB (20 fields × ~250 B); fits one Postgres, not a scaling problem |
| Submissions, average | ~20k/s, ~1.7B/day | 20,000 × 86,400 = 1.73B |
| Submissions, peak | 200k/s | 10× average: a ticket drop or a viral link |
| Row size | ~1 KB | ~600 B JSONB answers + ids, version, timestamps, indexes |
| Write volume | ~1.7 TB/day, ~155 TB for 90 days hot | 1.73B × 1 KB × 90 |
| Shards | 32 Postgres shards | 20k/s ÷ 32 ≈ 625 inserts/s each on average; 32 × 2.5k = 80k/s sustained capacity (a few thousand writes/s per node) |
| Spike backlog | ~72M submissions ≈ 72 GB | (200k − 80k)/s × 600 s for a 10-minute spike; drains in 72M ÷ (80k − 20k) = 1,200 s = 20 min |
| Form views | ~200k/s average, ~2M/s peak | 10 views per submission |
| Schema bytes if uncached | ~1 GB/s average | 200k/s × 5 KB; hence CDN |

What the numbers tell us: **the schema is tiny, immutable per version and read 10× more than
submissions are written**, so it belongs on a CDN keyed by `(form, version)`. The volume is all
in submissions: shard by tenant, keep rows small, batch writes. Validation is pure CPU (no
database), which matters for the spike handling in §10.

## 5. Plan

1. The form schema as data: types, ids, conditions, versions.
2. One validator for both client and server.
3. Submission storage: what a row looks like and how to query it later.
4. Editing a live form: version rules and the in-flight submission.
5. Serving: schema caching, embeds, CORS, abuse.

## 6. High-level design

```
 tenant editor ─► Forms API ─► Postgres (control plane): forms, form_versions (immutable once published)
                                   │ publish = new version row + flip forms.published_version
                                   ▼
 visitor browser / embed ─► CDN ─► GET /f/{public_id}                → {version: 7}           (TTL 5 s)
                              └──► GET /f/{public_id}/v/7            → schema JSON            (immutable, 1 year)
 visitor ─► LB ─► Submit API (stateless)
                    1. load schema (form, version) from local cache     ← no DB on the hot path
                    2. validate against THAT version                    ← 422 per field, computed from the schema alone
                    3. INSERT submissions + outbox in one transaction   ← shard = hash(tenant_id) % 32
                                   ▼
                       Postgres shards (data plane) ─► worker: webhooks, email, search/CSV projection
```

Control plane (forms, versions) is small and lives in one database. Data plane (submissions) is
sharded by tenant. A submit never reads the control-plane database: the schema for
`(form, version)` is immutable, so any server may cache it forever.

## 7. API

```
GET  /f/{public_id}                    → 200 { "version": 7 }                       Cache-Control: public, max-age=5
GET  /f/{public_id}/v/{n}              → 200 schema JSON                            Cache-Control: public, max-age=31536000, immutable
POST /f/{public_id}/submissions        { "version": 7, "answers": { "f1": "Ann", … } }  Idempotency-Key (see 16)
                                       → 201 { id } | 422 { fields: { "f3": "required" } } | 409 { "reload": true } | 410 form paused/archived
POST /tenants/{t}/forms                → 201 form (draft v1)
PUT  /tenants/{t}/forms/{id}/draft     { schema }        If-Match: <draft etag>     → 200 | 412 (someone else edited)
POST /tenants/{t}/forms/{id}/publish   { "draft_version": 8 }                       → 200 { published_version: 8 } | 422 (schema invalid)
GET  /tenants/{t}/forms/{id}/submissions?cursor=…&filter[f3]=fence                 → 200 page
POST /tenants/{t}/forms/{id}/exports   → 202 { export_id }            (async CSV, streamed from object storage)
```

## 8. Data model

```sql
CREATE TABLE forms (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id          BIGINT NOT NULL,
    public_id          TEXT   NOT NULL UNIQUE,              -- 'f_' + 12 base62: what embeds use, never the integer id
    name               TEXT   NOT NULL,
    state              TEXT   NOT NULL DEFAULT 'live' CHECK (state IN ('live','paused','archived')),
    draft_version      INT    NOT NULL DEFAULT 1,
    published_version  INT,                                  -- NULL until first publish
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX forms_tenant_idx ON forms (tenant_id, id);

CREATE TABLE form_versions (
    form_id      BIGINT NOT NULL REFERENCES forms(id),
    version      INT    NOT NULL,
    status       TEXT   NOT NULL CHECK (status IN ('draft','published','retired')),
    schema       JSONB  NOT NULL,                            -- fields, types, show_if, limits
    retired_at   TIMESTAMPTZ,                                -- when it stopped being the current version
    PRIMARY KEY (form_id, version)
);
-- a published version never changes: only its status may move (published → retired)
CREATE FUNCTION form_versions_frozen() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'draft' AND NEW.schema IS DISTINCT FROM OLD.schema THEN
        RAISE EXCEPTION 'form version % is published and immutable', OLD.version;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER form_versions_frozen BEFORE UPDATE ON form_versions
    FOR EACH ROW EXECUTE FUNCTION form_versions_frozen();

-- on each data shard
CREATE TABLE submissions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       BIGINT NOT NULL,
    form_id         BIGINT NOT NULL,
    form_version    INT    NOT NULL,                         -- which schema these answers belong to
    submission_key  UUID,                                    -- idempotency, as in 16
    answers         JSONB  NOT NULL,                         -- keyed by FIELD ID, never by label: {"f1":"Ann","f3":"fence"}
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX submissions_form_idx ON submissions (form_id, id DESC);
CREATE UNIQUE INDEX submissions_key_idx ON submissions (form_id, submission_key) WHERE submission_key IS NOT NULL;
```

At this volume `submissions` is partitioned by month; a unique key on a partitioned table must
include the partition key, so the primary key and the idempotency index gain `received_at`, and
the id comes from an explicit sequence instead of an identity column.

## 9. Deep dives

### 9.1 The schema as data

Tenants own **fields**; code owns **field types**: the same split as statuses and stages in
[26](26-status-workflow.md). A type is a small closed set with a validator the platform maintains;
a tenant cannot ship code.

```json
{ "fields": [
  { "id": "f1", "type": "text",   "label": "Name",   "required": true, "max": 100 },
  { "id": "f2", "type": "email",  "label": "Email" },
  { "id": "f3", "type": "choice", "label": "Service", "required": true,
    "options": [{"v":"deck","l":"Deck"},{"v":"fence","l":"Fence"}] },
  { "id": "f4", "type": "number", "label": "Fence height (cm)", "required": true,
    "show_if": { "field": "f3", "eq": "fence" } } ] }
```

Rules that keep it safe and simple:

- **Stable ids, free labels.** Answers are keyed by `f4`, never by "Fence height". Renaming a label changes nothing stored; **never reuse an id** for a different meaning (a changed type is a new id).
- **`show_if` may only look at earlier fields.** Evaluation is one pass, in order, no cycles to detect.
- **Hidden means dropped and never required**, honeydolist's rule (`a hidden field is never required`; a field no longer asked is dropped). A required field behind a false condition would make the form unsubmittable.
- Limits on the schema itself (≤ 100 fields, ≤ 200 options per choice, ≤ 100 KB) so a hostile tenant cannot publish a 50 MB form to every visitor.

**One validator, two ends.** The server is the authority; the browser runs the same rules for
instant feedback. They must not drift:

| Option | Pros | Cons |
|---|---|---|
| **Server validates; client has hand-written checks** | simplest | the two disagree within a month; users see "valid" then a 422 |
| **Generate JSON Schema, use a library on each side** | standard | `show_if`, "hidden ⇒ dropped" and per-type rules are not JSON Schema; escape hatches creep in |
| **A tiny interpreter of *our* schema in Go and in TypeScript, with a shared test-vector file run in CI by both** | exact semantics, small (a few hundred lines each), drift is a failing test | two implementations to maintain |
| Server renders the form and validates round-trip only | one implementation | slow, no instant feedback, no embeds on static sites |

**Choice and why.** The interpreter on each side plus **shared golden vectors** (`schema +
answers → expected clean answers and errors`): the server stays the authority (a `422` always
wins), and a divergence is a red build, not a support ticket.

```go
func Validate(fields []Field, in map[string]string) (clean, errs map[string]string) {
    clean, errs = map[string]string{}, map[string]string{}
    for _, f := range fields { // in order: show_if only looks backwards
        if f.ShowIf != nil && clean[f.ShowIf.Field] != f.ShowIf.Eq {
            continue // hidden: dropped, never required
        }
        v := strings.TrimSpace(in[f.ID])
        if v == "" {
            if f.Required {
                errs[f.ID] = "required"
            }
            continue
        }
        if msg := checkType(f, v); msg != "" { // a switch on f.Type: email, number range, option membership…
            errs[f.ID] = msg
            continue
        }
        clean[f.ID] = v
    }
    return clean, errs
}
```

### 9.2 Where the answers live

| Option | Pros | Cons |
|---|---|---|
| **One wide table per form** (DDL on every edit) | real columns, SQL-friendly | 5M tables; every field edit is an `ALTER`; the catalogue itself becomes the bottleneck |
| **EAV: one row per answer** (`submission_id, field_id, value`) | filter any field with one index | ~8× the rows and index entries (8 answers → 8 rows): 20k submissions/s becomes 160k row-writes/s; reassembling a submission is a join |
| **JSONB `answers` keyed by field id + `form_version`** | one row per submission, no DDL, old rows keep their shape | no per-field typed indexes; querying a field needs help |
| Document store (wide-column, key = `(form_id, id)`) | write scaling is native | ad-hoc filters and CSV need a second system anyway |

**Choice and why: JSONB + version.** honeydolist's typed columns work because *its* fields are
the code's; invented fields cannot have columns. One row per submission keeps the write path at
~1 KB and one insert.

Querying and exporting later, without per-form indexes (you cannot afford 5M × N indexes):

- **Listing** is `(form_id, id DESC)` with a keyset cursor: no field filter needed ([23](23-paginated-lists.md)).
- **Filtering on a field** is a *projection*: the worker (fed by the outbox) writes `(form_id, field_id, value_hash_or_text, submission_id)` rows **only for fields the tenant marked "filterable"** (at most 3 per form), or feeds a search index ([39](39-global-search.md)). Unmarked fields are still exported, just not filterable.
- **CSV export** is an async job: scan `(form_id, id range)`, resolve each row's `form_version` to its schema for the header, **union the columns across versions** (a removed field keeps its column, empty for new rows), stream to object storage.
- **A hot form** (one viral form at 200k/s) cannot sit on one shard at ~2.5k writes/s: such a form gets `k` sub-buckets (`hash(submission_key) % k`) across shards, and listing merges them. Flag it from the rate, not at creation.

### 9.3 Editing a live form

Versions are immutable once published: **an edit is a new version, publishing is a pointer
flip** (`forms.published_version`), the same shape as [32](32-block-cms.md)'s pointer flip. What each change means to data already collected:

| Edit | Old submissions | New submissions |
|---|---|---|
| Rename a label | unchanged (same id) | new label shown |
| Add an optional field | have no value; export shows empty | may carry it |
| Add a **required** field | were valid then; **not re-validated** | required from the new version |
| Make required → optional | unchanged | relaxed |
| Make optional → required | unchanged | required from the new version only |
| Remove a field | keep their value, export keeps the column | cannot carry it |
| Change a field's type | keep old values under the old id | a **new id**, never a reinterpretation |

**The in-flight submission.** A visitor loaded v7; the tenant publishes v8; the visitor submits
`version: 7`. Options:

| Option | Pros | Cons |
|---|---|---|
| Reject with 409 "form changed, reload" | simple | the visitor loses what they typed; terrible for a lead form |
| **Validate against the version they loaded**, accept within a grace window (24 h after v7 retired) | no lost leads, exact semantics | must keep old versions available; after the window a 409 |
| Validate against the current version and drop unknown fields | one code path | silently loses answers to fields removed meanwhile |

**Choice and why: validate against the submitted version within a grace window.** honeydolist
drops fields a form no longer asks because its page and its API ship together and a stale page is
rare; a builder with embeds on other people's pages has stale pages constantly. After the window,
`409 {"reload": true}` and the client re-renders keeping the answers it can match by field id.
Retired versions are retained as long as any submission references them (cheap: 5 KB each).

Concurrent *editors*: the draft carries an ETag and `PUT` needs `If-Match` (`412` if someone saved
first). It is the same optimistic check honeydolist runs on its settings saves, which answer a
stale save with `409`; `412` is just the HTTP-native status for a failed `If-Match`.

### 9.4 Serving the schema, embeds and abuse

- **Two-step fetch.** `GET /f/{id}` returns just `{version}` with a 5 s TTL; `GET /f/{id}/v/7` is immutable for a year. A publish is visible within ~5 s everywhere and the big object is never revalidated. An embed script can inline the version number to skip the first hop.
- **The submit path never touches the control plane:** servers keep an LRU of `(form, version) → compiled schema`; a miss is one primary-key read. 5M forms × 5 KB = 25 GB is the whole current set, so even a cold fleet warms quickly.
- **Embeds and CORS.** The schema `GET` is public and cacheable (`Access-Control-Allow-Origin: *`). The submit `POST` is cross-origin from arbitrary sites, so CORS is *not* the security control (a non-browser client ignores it): the controls are the rate limits, bot traps and score from [17](17-abuse-protection.md), plus an optional per-form allow-list of origins checked against `Origin` as a courtesy and an analytics signal.
- **CSRF is not the issue** (no session cookie on a public form); **clickjacking** is: iframe embeds are allowed on purpose, but the *tenant's dashboard* sends `frame-ancestors 'none'`.
- **Captcha** is a per-form switch (token verified server-side before insert), off by default; do not make every tenant pay the friction.
- **File fields** never travel in the JSON: they use presigned uploads ([11](11-image-upload-hosting.md)) and the answer holds the object key, sent after the lead as honeydolist's quote photos are ([16](16-public-form-intake.md) §9.5).

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the submission write path on a hot tenant or hot form, not the schema. Put a durable log in front for spikes (valid because validation needs only the cached schema: the one case [16](16-public-form-intake.md)'s "no broker for forms" excludes, since there the answers depended on the database), and spread hot forms over sub-buckets. Then the CSV export scans.
- **CDN down or cold:** servers fall back to the origin schema endpoint; it serves 25 GB of small immutable objects from a cache, so even an origin-only day is survivable.
- **Control-plane database down:** forms already cached keep rendering and accepting; **editing and publishing stop**. This split is why submit must not read it.
- **Bad publish:** a tenant publishes a broken form (a required field that can never show). Publish runs a validator over the schema *and* a satisfiability check ("is there at least one assignment that passes?") and `422`s; one-click **revert** is flipping the pointer back.
- **Schema bomb:** limits on field count, option count, total size and `show_if` depth at publish time; the interpreter has no loops by construction.
- **Retention:** per-tenant retention windows; erasure of one person's submissions ([24](24-data-lifecycle.md)) must also hit the projection rows and exports.
- **Single points of failure:** the control-plane primary (read-only degradation above), the log (replicated), a shard (replicas; tenants on it fail over together).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Tenants own fields, code owns types | Fully custom field code | safe, validatable, exportable | exotic fields need a platform change |
| One interpreter per language + golden vectors | Hand-written client checks | no client/server drift | two small implementations |
| JSONB answers + `form_version` | Table per form / EAV | one row per submission, no DDL | no per-field indexes; projections for filters |
| Immutable versions, pointer flip | Mutable form | old data stays interpretable; instant revert | version retention and a grace-window rule |
| Validate against the submitted version | Always the current one | no lost leads on embeds | old versions must stay loadable |
| Two-step schema fetch (pointer + immutable) | Short-TTL schema | CDN hit rate, 5 s publish visibility | an extra tiny request |

## 12. Check yourself

1. Why are answers stored under field ids and not labels, and what must never happen to an id?
2. Why does `show_if` only look at earlier fields, and what happens to a hidden required field?
3. Compare JSONB, one table per form and EAV for 20k submissions/s. Why does the choice not break honeydolist's typed columns?
4. A visitor loaded v7, the tenant published v8, and the visitor submits. What happens, and why not simply a 409?
5. Why can the submit path avoid the control-plane database, and what does that buy during an outage?

<details><summary>Answers</summary>

1. Labels are display text and change freely; ids are identity. A stored answer keyed by id stays
   meaningful through renames. An id must never be **reused for a different meaning** (a new type or
   semantics gets a new id), otherwise old rows would be reinterpreted wrongly.
2. One ordered pass, no cycles or fixpoints to resolve. A hidden field is dropped and not required:
   a required field behind a false condition would make the form impossible to submit.
3. One table per form means 5M tables and an `ALTER` per edit; EAV turns ~8 answers into ~8 rows
   (160k row-writes/s) plus a join to reassemble; JSONB is one ~1 KB row per submission with the
   version tag. honeydolist's fields are fixed by code, so typed columns work there: invented fields
   cannot have columns.
4. Validate against v7 (the version the submission names) and accept inside a grace window (say
   24 h after v7 retired), then `409 reload` and the client keeps matching answers by field id. A
   plain 409 loses what the visitor typed, which for a lead form is lost revenue.
5. A published `(form, version)` schema is immutable, so it can be cached forever on every server.
   Submits then need no control-plane read: with that database down, rendering and submitting
   continue and only editing and publishing stop.

</details>

## 13. In Go

- Related lessons: [22 database/sql](../../22-database.md) and [59 Production DB patterns](../../59-production-database-patterns.md) (JSONB, partial indexes, triggers, keyset), [41 API design](../../41-api-design-evolution.md) (`ETag`/`If-Match`, `422` problem details, `Cache-Control: immutable`), [38 Caching](../../38-caching-patterns.md) (the compiled-schema LRU), [57 Web security](../../57-web-security.md) (CORS, framing, what a captcha is for), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (filter projections), [18 Testing](../../18-testing.md) (table-driven golden vectors).
- Put the validator in one package (`formschema`) with no I/O; load the shared vector file in a table-driven test, and run the same file from the TypeScript package in CI.
- Decode answers with `map[string]string` or `json.RawMessage` per field type; **never trust the client's types**, coerce in `checkType`.
- Packages: `pgx/v5`, `encoding/json`, `hashicorp/golang-lru/v2` (schema cache), `net/http` (with `http.MaxBytesReader` on the submit body).

## 14. Extensions

- **Multi-page wizards with branching:** the schema becomes a graph; what replaces "earlier fields only" to stay cycle-free?
- **Computed fields and scoring** (quizzes): an expression language without loops; where does evaluation live, and how do you test it against both ends?
- **Prefill and partial saves:** server-side drafts keyed by a resume token, and what a draft means across a version change (task 16's drafts are client-side).
- **Per-tenant retention and legal holds** over sharded data, including exports already handed out.
- **Webhooks per form** with their own signing secret and delivery log ([10](10-webhook-delivery.md)).
- **Template marketplace:** copy-on-use versions, and what happens to a tenant's form when the template author changes theirs.
