# 38 · AI Draft Queue

`🟢 easy` · *Topics:* a backfill as a job queue, estimate-then-run-then-stop-cleanly, drafts as rows that never overwrite human text, approve through the editor with a version check, review capacity as the real bottleneck, risk tiers and sampling, prompt versions and input hashes · *Builds on:* [36 LLM Gateway](36-llm-gateway.md) (budgets, switches, pacing), [15 Job Scheduler](15-job-scheduler.md) (`SKIP LOCKED` dispatch, leases), [10 Webhook Delivery](10-webhook-delivery.md) (retries, dead letters), [11 Image Upload & Hosting](11-image-upload-hosting.md) (the images being described)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [37 In-App Records Assistant](37-records-assistant.md) · Next: [39 Global Search with Permission Filtering](39-global-search.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 75 adds a "Draft with AI" button to every text field plus a backlog run ("Draft all empty") whose results wait in a review queue. **Nothing is ever written to a record by the system.** Drafts live in `ai_drafts` (migration 0092) with the text's `sha256`, so a field whose saved value equals a stored draft is flagged "AI text nobody has edited". `ai_draft_runs` (0093) is one row per run: kinds, a cap, `queued → running → done | stopped | cancelled`, an estimate shown before the click, and a *partial unique index* allowing one active run. The worker takes jobs from an outbox lane of its own with one or two slots, so a hundred model calls never delay a lead alert. A guard replaces any price, licence or insurance claim, guarantee, superlative, lead time or review in a draft with `[CONFIRM: …]`. Step 86 adds a tab per kind, a before→after table, Approve (written *through the record's own editor*) and bulk Approve/Dismiss over a whole tab in batches of 500; steps 87–88 the *only missing or short (< 3 words)* and *redraft* modes; steps 89–91 build photo descriptions from the page's category, its text and what the other photos on the page already say (338 of 589 images shared an alt with a sibling before). Step 74's *Search health* is the "what is thin" detector. Cost: a photo ≈ a third of a cent; 500 photos ≈ 700,000 tokens.
> **Here, at scale:** a CMS with 100M pages and images, a 5M-call weekend backfill, and a review team that approves 2,000 drafts an hour.

## 1. The task

> "We have millions of images and pages with no description, or a bad one. Use an LLM to draft them. People must review before anything goes live. Design the backfill, the review queue and the safeguards."

The model call is the easy part. The design is in *what you ask for next*, how you stop, how a draft can never clobber a person's edit, and the arithmetic of reviewing millions of drafts.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Can a draft ever publish itself? | Defines the trust model and the review load. | Default: never. Optional narrow auto-apply (9.3) is an explicit policy. |
| What counts as "needs a draft"? | The selection query and its cost. | Empty, or under 3 words (short), or an explicit redraft of a scope. |
| Scale? | Queue and gateway sizing. | 100M records; 5M calls in a 48 h weekend backfill. |
| Review capacity? | The bottleneck. | 2,000 drafts an hour across the review team. |
| Order of work? | Whole-library runs are rarely right. | By value: traffic × severity. |
| What if the record changes after drafting? | Stale drafts and lost edits. | The draft is checked against the record's version on approve. |
| Same draft twice? | Cost and duplicates. | Idempotent per (record, field, inputs, prompt version). |
| Who may start a run? | Money. | An admin permission; one active run per tenant. |
| Languages? | Prompt per language. | One language per run; out of scope otherwise. |

## 3. Requirements

**Functional**
- Choose scope (kinds, categories, mode: *empty*, *only short*, *redraft*), see an **estimate** (items, tokens, cost), start, pause, resume, cancel.
- Workers draft one item at a time, store each draft separately, and report progress.
- Review queue: tabs per kind, before → after, edit in place, Approve and Dismiss one, many, or a whole tab.
- Approve writes through the record's editor (its limits, reindex, cache purge, history row); a draft with a `[CONFIRM: …]` marker cannot be approved.
- Guard: drafts never contain prices, credentials, guarantees, superlatives, lead times or reviews.

**Non-functional**
- A run stops cleanly when the feature is switched off or the budget is reached, and loses nothing.
- Backfill traffic never starves interactive AI or ordinary jobs.
- Re-running a run is cheap: unchanged inputs are not redrafted.

**Out of scope**
- The model/gateway (36), the search index (39), image upload (11), translation.

## 4. Back-of-the-envelope

Prices are **illustrative assumptions**: ~$3 per million input tokens, ~$15 per million output (large model), ~$0.25 / ~$1.25 (small).

| Quantity | Estimate | How |
|---|---|---|
| Generation rate | ~29 items/s | 5,000,000 ÷ (48 h × 3,600 = 172,800 s) = 28.9 |
| Tokens | 6B in + 1B out | 5M × ~1,200 in (image ≈ 900 + facts) and ~200 out |
| Tokens/min while running | ~2.4M | 28.9/s × 1,400 × 60 |
| Cost, large model | ≈ $33k ≈ 0.66¢ an item | 6,000 M × $3 + 1,000 M × $15 = $18k + $15k |
| Cost, small vision model | ≈ $2.75k ≈ 0.055¢ an item | 6,000 M × $0.25 + 1,000 M × $1.25 |
| Review rate | ~0.56/s | 2,000 ÷ 3,600 |
| Generation ÷ review | **≈ 52×** | 28.9 ÷ 0.556 |
| Still pending after the weekend | ≈ 4.9M | 5M − 2,000 × 48 h |
| Time to review all 5M | 2,500 reviewer-hours ≈ 312 working days | 5M ÷ 2,000; ÷ 8 h |
| Draft rows | ~7.5 GB per backfill | 5M × ~1.5 KB (text, hashes, usage, warnings, indexes) |
| QA sample at 0.2 % | 10,000 reviews ≈ 5 h | 5M × 0.002 ÷ 2,000 |

What the numbers tell us: the model can write 52× faster than people can read. A queue that grows by 5M a weekend is a failure, not progress. So the design problem is **backpressure from the review end**: generate in priority order, only as much as can be reviewed (or auto-applied under a stated policy), and make review itself fast (bulk, diff, sampling). Money is secondary: $33k is a rounding error next to 2,500 hours of people.

## 5. Plan

1. Find candidates cheaply and rank them.
2. The run: estimate, enqueue, workers, pacing, pause/cancel, stop reasons.
3. Draft storage and approval that cannot overwrite human work.
4. The review bottleneck: tiers, bulk actions, sampling, optional auto-apply.
5. Prompt/version management and what to redraft.

## 6. High-level design

```
 content health (candidates, score)  ──►  Run controller  ──►  ai_draft_jobs (SKIP LOCKED)
   partial index: words < 3                 estimate · cap · one active run/tenant        │
   maintained from change events                                                            ▼
                                                                        N workers: claim → re-check "still needs it?"
                                                                           → build inputs → hash → skip if seen
                                                                           → LLM Gateway (36, lane=batch, paced)
                                                                           → guard (CONFIRM markers) → warnings
                                                                           → INSERT ai_drafts (pending)
 reviewers ◄─ review queue (tabs, paging in SQL) ◄─ ai_drafts ─► Approve ─► Applier ─► the record's editor
                                                                            (version check, limits, reindex, history)
 metrics: pending age, approval rate, edit rate, warnings by kind, cost per approved draft
```

One item: a worker claims a job, reloads the record, drops it if nothing is missing any more, assembles the model's inputs (the record's own text, its categories, the pages that show it, what sibling images already say), hashes them, and skips if an identical draft exists. It calls the gateway on the batch lane, runs the guard on the reply, and inserts a *pending* draft. A person later approves; the Applier saves through the same editor a human would use.

## 7. API

```
POST /v1/ai/runs/estimate   { kinds, categories?, mode: "empty|only_short|redraft", cap, model_class }
  → 200 { items, est_tokens, est_cost_micros, daily_budget_left, warning? }
POST /v1/ai/runs            same body + estimate_token  → 201 { run_id } | 409 { code: "run_active" }
GET  /v1/ai/runs/{id}       → { status, done, failed, skipped, tokens, cost_micros, stopped_reason }
POST /v1/ai/runs/{id}/pause | resume | cancel     → 202

GET  /v1/ai/queue?entity=&page=&per_page=&risk=   → { tabs[{entity,count}], total, items[{record, fields[{key,before,after,max,warnings[]}]}] }
POST /v1/ai/queue/approve   { items:[{id, text?}] | ids:[…] | all:true, except:[…] }
  → 200 { approved, failed:[{id, reason}], skipped, remaining }       (batches of 500; refused ones stay pending with a reason)
POST /v1/ai/queue/dismiss   { ids | all, except }                     → 200 { remaining }
```

## 8. Data model

```sql
CREATE TABLE ai_draft_runs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id bigint NOT NULL, started_by bigint,
  kinds text[] NOT NULL CHECK (cardinality(kinds) BETWEEN 1 AND 20),
  mode text NOT NULL CHECK (mode IN ('empty','only_short','redraft')),
  scope jsonb NOT NULL DEFAULT '{}',                 -- categories, priority cut-off
  cap integer NOT NULL CHECK (cap BETWEEN 1 AND 5000000),
  status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','running','paused','done','stopped','cancelled')),
  stopped_reason text,                               -- "switched off", "budget reached": stopped is not failed
  prompt_version text NOT NULL, model_class text NOT NULL,
  est_items bigint, est_cost_micros bigint,
  created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz
);
-- the seed has ((true)): one active run in the whole database. At scale: one per tenant.
CREATE UNIQUE INDEX ai_draft_runs_one_active ON ai_draft_runs (tenant_id) WHERE status IN ('queued','running','paused');

CREATE TABLE ai_draft_jobs (
  run_id bigint NOT NULL REFERENCES ai_draft_runs(id) ON DELETE CASCADE,
  entity text NOT NULL, entity_id text NOT NULL,
  priority real NOT NULL DEFAULT 0,                  -- traffic × severity
  state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','leased','done','skipped','failed')),
  lease_until timestamptz, attempts smallint NOT NULL DEFAULT 0, last_error text,
  PRIMARY KEY (run_id, entity, entity_id)
);
CREATE INDEX ai_draft_jobs_ready ON ai_draft_jobs (run_id, priority DESC) WHERE state = 'queued';

CREATE TABLE ai_drafts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  tenant_id bigint NOT NULL, run_id bigint REFERENCES ai_draft_runs(id) ON DELETE SET NULL,
  entity text NOT NULL, entity_id text NOT NULL, field text NOT NULL,
  text text NOT NULL,
  sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),   -- "AI text nobody has edited" = saved value hashes to this
  input_hash bytea NOT NULL,                          -- everything the model was told, hashed
  source_version bigint NOT NULL,                     -- the record's version when drafted
  prompt_version text NOT NULL, model text NOT NULL, provider text NOT NULL,
  usage jsonb NOT NULL DEFAULT '{}', warnings jsonb NOT NULL DEFAULT '[]',
  risk text NOT NULL DEFAULT 'review' CHECK (risk IN ('review','low')),
  state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','used','dismissed','stale')),
  state_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ai_drafts_one_pending ON ai_drafts (tenant_id, entity, entity_id, field) WHERE state = 'pending';
CREATE UNIQUE INDEX ai_drafts_idem ON ai_drafts (tenant_id, entity, entity_id, field, input_hash, prompt_version);
CREATE INDEX ai_drafts_queue ON ai_drafts (tenant_id, entity, created_at DESC) WHERE state = 'pending';

CREATE TABLE content_health (                         -- candidates, kept current by change events (as search_dirty in 39)
  tenant_id bigint NOT NULL, entity text NOT NULL, entity_id text NOT NULL, field text NOT NULL,
  words smallint NOT NULL, severity smallint NOT NULL, traffic_30d bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, entity, entity_id, field)
);
CREATE INDEX content_health_thin ON content_health (tenant_id, entity, (severity::bigint * traffic_30d) DESC) WHERE words < 3;
```

## 9. Deep dives

### 9.1 A backfill is a queue, not a loop

| Option | Pros | Cons |
|---|---|---|
| One goroutine loops over records | trivial | dies with the process; no pause, no progress, no pacing |
| Enqueue millions of outbox/queue messages up front | simple workers | huge queue, cancel means draining it, priorities are awkward |
| **Jobs table per run + `SKIP LOCKED` workers + a run row** | cancel/pause is one `UPDATE`; priority order; progress is a `COUNT`; survives restarts (leases, see [15](15-job-scheduler.md)) | the table is big during a run (a few GB) |

**Choice and why.** A jobs table and a run row. Details that matter:

- **Estimate first, from a sample.** Draft 200 random candidates (or use average lengths), multiply, show tokens and cost *and the day's remaining budget*. The seed shows an estimate before the click, but its step 86 notes the preview does not mention the daily limit: a second 500-photo redraft the same day (≈ 700,000 tokens each) hits the 1M-token cap and the run stops with "run out of tokens". Putting the remaining budget in the estimate closes that gap.
- **Re-check at the item's turn.** The candidate list is a snapshot; by the time a job runs, a person may have fixed the field. Reload the record, and mark the job `skipped` if nothing is missing any more (the seed counts these).
- **Stop cleanly, never fail.** Before *each* claim the worker checks: run cancelled or paused? feature switched off? budget reached? Any "yes" ends the run as `stopped` with a reason, leaving untouched jobs `queued` so *resume* just continues ([36](36-llm-gateway.md) 9.2). Five consecutive provider failures also end the run (breaker thinking) instead of burning through 5M failures.
- **Pacing.** Send to the gateway's *batch lane* with a token-per-minute ceiling (assume ≤ 30 % of the provider limit) so interactive features keep headroom; the seed gives its lane one or two worker slots for the same reason. Rate limiting: [03](03-rate-limiter.md).
- **Progress without a hot row.** 29 updates/s to one counter row is fine; 3,000/s is not. Each worker accumulates and flushes every ~50 items or 2 s, or the UI just `COUNT(*) GROUP BY state` every few seconds.
- **One active run per tenant** (the partial unique index is the second click's refusal); global fairness across tenants comes from claiming round-robin by tenant.
- **Retries:** jittered, `attempts` capped, then `failed` with `last_error` (dead-letter, see [10](10-webhook-delivery.md)).

### 9.2 Drafts never overwrite human text

The rule that makes the whole feature trustworthy: **a draft is a row in its own table, and the record changes only when a person approves.**

- **Identity of a draft:** `(tenant, entity, entity_id, field)` for the *pending* one: a new draft retires the older pending draft of the same field (the seed dismisses it) in the same transaction (`ai_drafts_one_pending`).
- **Approve = apply through the editor, with a version check.** The Applier calls the record's normal save path, so every rule of that editor holds (length limits, word minimum, reindex, cache purge, a history row "AI draft approved") and it saves only the field(s) approved. The write is `UPDATE … WHERE id = $1 AND version = $2`; zero rows means the record changed after drafting: the draft becomes `stale`, stays visible with the reason, and is never forced in. Approve never publishes anything.
- **Markers block approval.** `[CONFIRM: …]` means the model hit a claim it could not ground. A draft with one is refused whoever edited it, until a human replaces the marker. The seed refuses a *single row's* Approve until markers are gone from all its fields, while a whole-tab Approve writes only the marker-free ones and leaves the rest pending.
- **The guard runs on the reply, not the prompt.** Regexes for prices, "licensed/insured/bonded", "guarantee/warranty", superlatives, lead times ("within 2 days", "same-day"), and reviews/ratings: matches are replaced by `[CONFIRM: price]` etc. and listed in `warnings`. A model that ignores instructions still cannot get a price into a page. Cheap, deterministic, testable.
- **Provenance:** `sha256` of the text, `model`, `provider`, `prompt_version`, `usage`, `run_id` on every row. "AI text nobody has edited" is a query: the saved field hashes equal to a stored draft. The seed's *Search health* ranks it (40 points) so unreviewed AI text is findable forever.
- **Inputs are untrusted data.** The record's text, a customer-supplied caption, page text: fence each value as data, one line each, angle brackets neutralised (as in [37](37-records-assistant.md) 9.3).

### 9.3 Review is the bottleneck: tiers, bulk, sampling, and an honest policy

With a 52× gap (§4) the options are limited.

| Option | Pros | Cons |
|---|---|---|
| Draft everything, review everything | zero risk of unreviewed text | 312 working days of review; the queue only grows |
| **Draft by priority, only up to review capacity** | queue stays short; effort goes where traffic is | the long tail waits (that is acceptable) |
| Auto-apply low-risk fields, sample for QA | clears the tail | AI text goes live unreviewed; needs revert and monitoring |
| Skip the queue, show drafts as live suggestions | no backlog | quality varies per visitor; SEO and trust risk |

**Choice and why.** Default: *priority-bounded generation with human review.* Rank by `severity × traffic_30d` (the seed's *Fix next* weights: a service with no text outranks a project with no date) and let the controller start the next slice only when pending drafts fall below a threshold, say one day of review capacity (48k). That is **backpressure from the review end**. Review itself must be fast:

- **Paging and counts in SQL**, by record; only the page's records are read (a photo through a light read, never its pixels).
- **Before → after with the text editable**, a popup for long texts, keyboard approve/dismiss.
- **Bulk over the whole tab, not the page:** select the page, then "select all N", untick exceptions; the client repeats `approve({all, except})` in batches of 500 until `remaining = 0`, adding each refused id to `except` (the seed's loop).
- **Risk tiers:** drafts with warnings (`keyword_stuffing`: a word repeated 3+ times; `copied_text`: 4+ consecutive words from the page's own text; any marker) always go to humans, first.
- **Optional, explicit auto-apply** for one narrow, low-harm field (image alt text) *only* when there are no warnings and the guard is clean; the previous value is stored, a one-click revert exists, and **0.2 % is sampled for human QA** (10,000 reviews ≈ 5 hours at 2,000/h) with the approval/edit rate tracked per prompt version; if the edit rate rises, auto-apply switches itself off. The seed deliberately does *not* do this ("Not doing: auto-publish AI text"); at 100M records that stance becomes a *policy choice with a price*, and this lesson wants it named, not hidden.

### 9.4 What to draft first, and how to find it without scanning 100M rows

- **A table scan is minutes** (it reads everything), so "WHERE alt is short" over the content tables is not a plan. Maintain `content_health` from change events (the dirty-queue/outbox pattern in [39](39-global-search.md)), with a partial index on `words < 3`, and select with keyset paging in priority order.
- **Short is a rule, not a guess:** under 3 words counts as missing (the seed's `ShortText`), computed when the row is written and **re-checked when the job runs**, because rules change and so do records.
- **Modes** (the seed's three, set per run): *empty*; *only missing or short* (draft just those fields); *redraft* (every field in scope, replacing pending drafts, never saved text). Redraft is a deliberate, expensive scope (a category, a kind).
- **Value ordering:** traffic is heavy-tailed (assumption: check yours); drafting the top 1–5 % of pages first usually captures most of the benefit. Severity adds judgement (a page with no body beats an image with a weak alt).
- **Detectors double as acceptance tests:** the same *Search health* rules that find thin text (no answer, body under 150 words, typed `$` amount, `[CONFIRM` left in) re-run after approval and the item leaves the list.

### 9.5 Prompts, versions, siblings and cost

- **Version everything that shapes output:** `prompt_version`, `model`, rules text. **`input_hash`** covers every input the model saw (record fields, categories, page text, sibling texts). The idempotency key is `(record, field, input_hash, prompt_version)`; a retried job or a second run inserts nothing (`ON CONFLICT DO NOTHING`), and bumping the prompt version is the explicit way to redraft.
- **Context makes drafts distinct, and creates order dependence.** The seed tells the model what the page's *other* photos already say (up to eight, deduplicated, clipped) and to say what is *different*; before that, 338 of 589 content images carried an alt identical to a sibling's. But drafting A changes B's inputs. Options: process a page's images sequentially in one group-job (each sees the previous drafts); or one call per page returning N distinct alts (fewer calls, distinct by construction, bigger blast radius on failure). **Choose group-jobs** per page, siblings drafted in order, so any single failure costs one image.
- **Post-checks warn, they do not rewrite:** `keyword_stuffing`, `copied_text` (repeats ≥ 4 consecutive words of the page text), length caps (a title ≤ 60 characters, an alt ≤ 125). A draft over the editor's limit is cut at a word for SEO fields, only *reported* elsewhere.
- **Cost levers in order:** a small vision model for alt text (§4: 12× cheaper); shared rules as a cached prefix; send the smaller image rendition; skip unchanged inputs; provider batch endpoints for work nobody waits on (check current terms and turnaround). The seed's whole library of 2,000 photos is "a one-click job" at a third of a cent each; at 5M the same click needs the estimate, the budget check and pacing.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (50M-call weekend):** the provider's token-per-minute limit, then `ai_draft_jobs` churn (batch-delete finished jobs by run), then drafts storage (75 GB). Review capacity broke long before all of these.
- **Single points of failure:** the gateway/provider (run pauses itself), Postgres (jobs are leases, resumable), the Applier path (the editor), the review UI (queue just waits).
- **Provider outage mid-run:** failures count toward the 5-in-a-row stop; the run ends `stopped: provider failing`; resume later.
- **Budget reached:** `stopped: daily limit`; resumes after the reset. The estimate warned about it.
- **Workers crash:** `lease_until` expires, the job is claimed again; the idempotency index makes the redo free.
- **Record edited while a draft is pending:** version mismatch → `stale`, never applied.
- **Guard bypass attempt (model writes "$1,200"):** marker inserted, approval blocked.
- **Prompt change regresses quality:** edit rate and dismiss rate per `prompt_version` show it within hours; pause auto-apply, roll the prompt back.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| jobs table + run row | millions of queue messages | pause/cancel/priority/progress in SQL | a large transient table |
| drafts in their own table | writing AI text into the record | no human edit is ever lost; provenance | an approve step |
| version check on approve | last write wins | stale drafts never clobber edits | some drafts go `stale` and need a redraft |
| priority-bounded generation | draft everything | a queue people can finish | the tail stays undrafted for a while |
| guard on the reply | trusting the prompt | deterministic safety net | regex false positives (a `[CONFIRM]` to resolve) |
| group-job per page | one call per image, in parallel | distinct sibling alts | less parallelism within a page |

## 12. Check yourself

1. The model writes 52× faster than people review. What does that do to the design?
2. How can a draft never overwrite a person's edit, even if the person edits the record while the draft waits?
3. How does a run stop cleanly, and what state is left behind?
4. What makes a retried job or a second identical run cost nothing?
5. Why is a table scan the wrong way to find "short" descriptions at 100M rows, and what replaces it?

<details><summary>Answers</summary>

1. Generation must be bounded by review capacity: rank candidates by value, start the next slice only when pending drafts drop below about a day of review, make review fast (bulk, diff, tiers) and decide explicitly whether any low-risk field may be auto-applied, with sampling and a revert. Otherwise the queue grows by millions a weekend.
2. Drafts are separate rows; approving applies through the record's editor with `WHERE version = $source_version`. If the record changed, zero rows update, the draft is marked `stale`, and nothing is overwritten; the person can redraft.
3. The worker checks cancel/pause/switch/budget before each claim and ends the run as `stopped` with a reason. Unclaimed jobs stay `queued`, finished drafts stay pending, in-flight leases expire and are reclaimed on resume. Nothing is lost and no failure alert fires.
4. A unique key over (record, field, `input_hash`, `prompt_version`) with `ON CONFLICT DO NOTHING`: same inputs and same prompt produce no new row, and the worker can skip before calling the model by checking the hash first. Changing the prompt version is the deliberate redraft.
5. It reads the entire table (minutes), per run, repeatedly. A maintained `content_health` table with a partial index on `words < 3`, ordered by severity × traffic and read with keyset paging, answers in milliseconds, and each job re-checks the live record anyway.

</details>

## 13. In Go

- Related lessons: [44 Background jobs & queues](../../44-background-jobs-queues.md) (leases, retries, lanes), [59 Production DB patterns](../../59-production-database-patterns.md) (`FOR UPDATE SKIP LOCKED`, optimistic version checks, partial unique indexes), [15 Sync & context](../../15-sync-context.md) (worker pool, cancellation), [36 Resilience](../../36-resilience-patterns.md) (backoff, breaker), [12 Errors](../../12-errors.md) (typed conflict/invalid errors from the Applier), [32 Hexagonal ports & adapters](../../32-hexagonal-ports-adapters.md) (the `Applier` interface so the draft engine never reads an entity), [55 Data pipelines](../../55-data-pipelines.md).
- The worker's shape (one item):

```go
func (w *Worker) once(ctx context.Context) (worked bool, err error) {
    if reason := w.gate(ctx); reason != "" { return false, w.stopRun(ctx, reason) } // cancelled, switched off, budget
    job, ok, err := w.claim(ctx) // UPDATE … SET state='leased', lease_until=now()+'2 min' WHERE … FOR UPDATE SKIP LOCKED
    if err != nil || !ok { return false, err }
    rec, err := w.records.Load(ctx, job.Entity, job.ID)
    if err != nil || !needsDraft(rec, job.Mode) { return true, w.finish(ctx, job, StateSkipped) }
    in := buildInputs(rec)                           // fenced values, siblings, page text
    h := inputHash(in, w.promptVersion)
    if w.drafts.Exists(ctx, job.Key(), h) { return true, w.finish(ctx, job, StateSkipped) }
    out, err := w.gateway.Complete(ctx, in.Request(), gateway.LaneBatch)
    if err != nil { return true, w.retryOrFail(ctx, job, err) }
    return true, w.store(ctx, job, guard(out), h)    // INSERT … ON CONFLICT DO NOTHING; retire older pending
}
```

- Packages: `pgx`, `golang.org/x/sync/errgroup`, `regexp` for the guard (compile once), `crypto/sha256`, `golang.org/x/time/rate` for pacing.

## 14. Extensions

- **Per-field acceptance learning:** which prompt versions get edited least; promote them.
- **Human edits as training signal:** store before/after of edited approvals for few-shot examples (privacy and licensing review first).
- **Scheduled health sweeps:** a weekly run that drafts only newly thin records, bounded by a monthly review budget.
- **Multilingual drafts** with language per run and a reviewer pool per language.
- **A/B test descriptions** by traffic/CTR before choosing between two approved candidates.
- **Reviewer analytics:** throughput, approve/edit/dismiss rates per reviewer, disagreement on a shared sample to calibrate quality.
- **Auto-draft on create** as an accept/dismiss suggestion: the seed skipped it on purpose, since an unasked call for every new record is spend nobody pressed a button for. Revisit with budgets from [36](36-llm-gateway.md).
