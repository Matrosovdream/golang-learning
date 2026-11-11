# 58 · Outbound Link Click Tracking

`🟢 easy` · *Topics:* track-then-redirect (302 via our endpoint, signed click ids, never block on a write), click ledger vs counter, the hot-row counter and the trigger it dragged along, dedupe windows, bot and fraud verdicts as revisable judgments, billing-grade counting and partner reconciliation, conversion attribution by postback · *Builds on:* [47 Web Analytics](47-web-analytics.md) (privacy-friendly counting, rollups: not repeated), [06 View & Like Counter](06-view-like-counter.md) (hot rows, batching), [01 URL Shortener](01-url-shortener.md) (redirect status codes), [17 Abuse Protection](17-abuse-protection.md) (bots, layered defence), [18 Inbound Webhook Receiver](18-inbound-webhook-receiver.md) (postbacks)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [57 Event Registration with Capacity & Waitlist](57-event-registration-waitlist.md) · Next: [59 Provider Adapters with Fake Drivers & Chaos Testing](59-provider-adapters-fakes.md)

---

> 🌱 **Seed (honeydolist-site):** a partner's product or machine can carry an external link (PLAN steps 61 and 66, migrations 0079, 0084). The **Buy / Rent** button points at the site's own address (`GET /api/v1/products/{slug}/buy?offer=…`), which records the click and answers `302` to the partner's page. A counted click is one `product_clicks` row (migration 0080: product, offer, user, `visitor`, time; rentals have `equipment_clicks`) plus `products.external_link_clicks + 1`, written in one small transaction **inside the request**; only a *failed* write is logged and ignored, and the visitor still goes on. Not counted: staff (`dashboard.access`), robots (a User-Agent list, or no User-Agent at all), prefetches (`Sec-Purpose` / `Purpose` headers) and the same visitor on the same product within 30 minutes, where `visitor` is the first 16 bytes of `sha256(pepper ‖ day|ip|user-agent)` and the pepper is random **per process**. The counter on the row also needed a special `updated_at` trigger, so a click is not mistaken for an edit. Volume there: tens of clicks a day.
> **Here, at scale:** an affiliate marketplace with 2 B outbound clicks a month (~770/s average), partners paid per click, and disputes over the numbers.

## 1. The task

> "Our pages link to partners' websites. Design the click tracking: count every outbound click, send the visitor on instantly, and give each partner a number we can defend when they say we are over- or under-counting."

At 700/s this is plumbing. The interesting part is money: once partners are paid per click, the count is an *invoice*, and an invoice needs evidence, rules you can explain, and a way to correct yesterday without rewriting history.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Write path and storage. | 2 B clicks/month ≈ 772/s average; design peak 5k/s (newsletter and promo bursts). |
| Who pays whom? | Decides how exact counting must be. | Partners pay us per valid click; a monthly invoice built from our count. |
| Where do links live? | Web, email, apps: a script on a page covers only one. | All three; so the link itself must carry the tracking. |
| What is a "valid" click? | The core business rule. | A human, not staff, not a prefetch, not a repeat within 30 minutes on the same link. |
| Latency budget? | The visitor waits for the redirect. | p99 under 50 ms on our side; the partner's page dominates the rest. |
| Do we keep visitor identity? | Privacy and the hash design. | No cookies, no raw IPs in the long-term store; a daily-rotating hash, as in [47](47-web-analytics.md). |
| How long do we keep raw clicks? | Dispute window and storage. | 90 days online (disputes and chargebacks), then archive; rollups forever. |
| Do partners report conversions? | Attribution needs a click id round-trip. | Yes: a server-to-server postback carrying our click id. |
| Disagreement rule? | A contract question, not a technical one. | Within 2% we invoice our number; beyond it, review row by row. |

## 3. Requirements

**Functional**
- A tracked link redirects to the partner with a **click id** attached, and records the click.
- Clicks are classified (valid, duplicate, bot, internal, fraud) and the verdict can be revised later.
- Partners see per-day and per-link counts, split billable and rejected with a reason, and can export click rows for a dispute.
- A partner can post conversions that reference a click id; we accept each once.

**Non-functional**
- The redirect never waits on a write, and never fails because tracking failed.
- A tracking outage loses at most seconds of clicks; every loss is counted and visible.
- Counting is repeatable: the same raw clicks and rules always produce the same billable number.

**Out of scope**
- Pricing and billing documents ([30](30-invoicing.md)), ad serving and auctions, multi-touch attribution models, mobile install attribution.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Average rate | ~772/s | 2 B ÷ (30 × 86,400 = 2.592 M s) |
| Design peak | 5k/s | ~6.5× average: a 5 M-subscriber newsletter with 10% clicking in 10 minutes adds 500k ÷ 600 s ≈ 830/s on top |
| Clicks per day | 66.7 M | 2 B ÷ 30 |
| Ledger row | ~160 B | ~120 B of columns and row header + ~40 B of one index |
| Ledger growth | ~10.7 GB/day, ~320 GB per 30 days | 66.7 M × 160 B |
| 90 days online | ~960 GB | 3 × 320 GB; partitioned by day, old partitions dropped whole |
| Rollup rows | ~5 M/day, ~240 MB/day, ~88 GB/year | assumption: 5 M (link, day, verdict) combinations active a day × ~48 B |
| Hot counter | top partner ≈ 10% of traffic: ~77/s average, ~500/s at peak | a single `UPDATE … SET n = n + 1` row: serial lock, one dead tuple per click |
| Money at stake | 1% of 2 B = 20 M clicks | × an assumed $0.01 per click = **$200k a month** per point of discrepancy |

What the numbers tell us: the write volume is modest and batchable. Two things are not: the **single hot counter row** (the seed's `external_link_clicks`) and the **cost of being wrong**: a 1% disagreement is real money, so evidence and repeatable rules matter more than raw speed.

## 5. Plan

1. The redirect path: how the click is observed, and why it never blocks.
2. The record: an immutable ledger of facts, in batches, with a click id.
3. The judgment: verdicts from rules, kept apart from the facts.
4. The count: rollups for reading, and what a dispute needs.
5. The money loop: reconcile with the partner; attribute conversions by postback.

## 6. High-level design

```
 page / email / app:  <a href="https://go.example.com/c/Ab3xK" rel="sponsored nofollow">
          │
          ▼
  LB ──► Redirector (stateless Go)
          1. link := cache.Get("Ab3xK")           in-memory, refreshed ~60 s; unknown/paused → fallback page
          2. id := uuidv7(); clk := id ‖ mac8     signed click id (§9.1)
          3. events <- Click{...}                 non-blocking send; a full buffer is counted, never waited on
          4. 302 Location: partner URL + clk       Cache-Control: no-store, X-Robots-Tag: noindex
                │ batcher (flush 200 ms or 1,000 rows)
                ▼
        Postgres  clicks            (immutable, partitioned by day, COPY / multi-row INSERT)
                ▼ classifier workers: dedupe (Redis SET NX EX 1800), bot rules, velocity
        Postgres  click_verdicts    (append-only; rules_version; reason)
                ▼ rollup job (idempotent recompute per day/partner)
        Postgres  click_rollups     ──► partner dashboard, monthly invoice
 partner ──► POST /postbacks/{partner} {clk, event_id, ...}  (signed; accept once)  ──► conversions
```

One click: the visitor taps Buy, our edge answers `302` in a few milliseconds, and the partner's page loads. Meanwhile the event reaches the batcher, then the ledger, then gets a verdict within seconds. Nothing the visitor waits for depends on any of it.

## 7. API

```
GET  /c/{link}?s={source}&t={signed render time}  → 302 Location: <partner URL with clk=…>
       Cache-Control: no-store · X-Robots-Tag: noindex, nofollow · never reads a destination from the query
       unknown, paused or deleted link → 302 to a fallback page (the product's own page), not a 404
POST /postbacks/{partner}   signed (HMAC, timestamp)  { "clk": "…", "event_id": "p-7781", "event": "purchase", "amount_cents": 4999, "at": "…" }
       → 202 accepted | 200 duplicate (same partner + event_id) | 401 bad signature | 422 forged or unknown clk
GET  /partners/{id}/clicks?from=2026-10-01&to=2026-10-31&group=day|link   → rollup rows: valid, duplicate, bot, internal, fraud
GET  /partners/{id}/clicks/export?month=2026-10   → 202 { url }  CSV of click_id, time, link, verdict, reason (for disputes)
```

The destination comes only from our `links` table, never from a query parameter: `/c?url=…` is an open redirect, and phishers love an open redirect on a trusted domain ([57 Web security](../../57-web-security.md)).

## 8. Data model

```sql
CREATE TABLE links (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id    TEXT   NOT NULL UNIQUE,          -- 'Ab3xK', random base62, never a counter
    partner_id   BIGINT NOT NULL,
    destination  TEXT   NOT NULL CHECK (destination LIKE 'https://%'),
    status       TEXT   NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused','deleted')),
    fallback_url TEXT                              -- where a paused link sends people (the seed's product page)
);

CREATE TABLE clicks (                              -- facts: written once, never updated
    click_id    BYTEA       NOT NULL,              -- 16 B UUIDv7: its first 48 bits are the time
    clicked_at  TIMESTAMPTZ NOT NULL,              -- minted by the redirector; same value on every redelivery
    link_id     BIGINT      NOT NULL,
    partner_id  BIGINT      NOT NULL,
    visitor     BYTEA,                             -- 16 B salted daily hash of ip + user agent; never a raw address
    user_id     BIGINT,                            -- a signed-in customer; staff are flagged, not stored as customers
    ua_class    SMALLINT    NOT NULL,              -- browser, known bot, headless, library, empty, prefetch
    country     CHAR(2),
    PRIMARY KEY (click_id, clicked_at)             -- a partitioned PK must contain the partition key
) PARTITION BY RANGE (clicked_at);
CREATE INDEX clicks_link_time_idx ON clicks (link_id, clicked_at);

CREATE TABLE click_verdicts (                      -- judgments: appended, never edited; newest rules_version wins
    click_id      BYTEA       NOT NULL,
    clicked_at    TIMESTAMPTZ NOT NULL,
    rules_version INTEGER     NOT NULL,
    verdict       SMALLINT    NOT NULL,            -- 1 valid, 2 duplicate, 3 bot, 4 internal, 5 fraud
    reason        SMALLINT    NOT NULL,
    decided_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (click_id, clicked_at, rules_version)
) PARTITION BY RANGE (clicked_at);

CREATE TABLE click_rollups (                       -- derived; recomputable from the two tables above
    day         DATE     NOT NULL,                 -- the partner's reporting day ([60](60-business-day-time-zones.md))
    partner_id  BIGINT   NOT NULL,
    link_id     BIGINT   NOT NULL,
    verdict     SMALLINT NOT NULL,
    n           BIGINT   NOT NULL,
    PRIMARY KEY (day, partner_id, link_id, verdict)
);

CREATE TABLE conversions (
    partner_id  BIGINT      NOT NULL,
    event_id    TEXT        NOT NULL,              -- the partner's id: idempotency
    click_id    BYTEA       NOT NULL,
    clicked_at  TIMESTAMPTZ NOT NULL,              -- read from the UUIDv7 inside click_id, so lookups hit one partition
    event       TEXT        NOT NULL,
    amount_cents BIGINT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (partner_id, event_id)
);
```

Facts and judgments live in separate tables on purpose: a new fraud rule changes `click_verdicts` and the rollups, and the ledger stays exactly as observed. That is what lets you answer "why was click X not billed?" a month later.

## 9. Deep dives

### 9.1 Track-then-redirect

How do we see the click at all?

| Option | Pros | Cons |
|---|---|---|
| **302 through our own endpoint** (the seed) | works in web, email and apps; first-party URL; we mint the click id and attach it to the destination; the visitor cannot skip it | one extra hop (an RTT to our edge); our uptime becomes the link's uptime |
| Client beacon (`sendBeacon`) and a direct link | no extra hop; fine for on-page analytics | blocked or lost on unload often enough to matter for billing; useless in email; no click id for the partner |
| `<a ping>` attribute | declarative | not reliably supported (disabled by default in some browsers); same blind spots as a beacon |
| Count at the partner only (postback) | the partner's number is the truth they pay on | we cannot audit, and clicks without conversions are invisible |

**Choice and why: the first-party 302, with a beacon at most as a cross-check.** Rules that make it safe:

- **`302` (or `307`), never `301`.** A permanent redirect is cached by browsers and clicks disappear (the same trap as [01](01-url-shortener.md)). Add `Cache-Control: no-store`.
- **Never block on the write.** The seed does its small transaction before answering and only swallows a *failure*; at 5k/s we do not even attempt it in the request: the handler does a non-blocking channel send and moves on.
- **Failing open.** Unknown, paused or deleted link: redirect to the fallback page (the seed's product page), not an error.
- **Prefetch and previews** (browser `Sec-Purpose: prefetch`, chat apps unfurling a link) must not count; they still get the redirect.
- **Hygiene:** `rel="sponsored nofollow"` on the links, `X-Robots-Tag: noindex, nofollow` on the endpoint, and `Disallow: /c/` in `robots.txt`.

A sketch (the helper types are not shown):

```go
func (s *Server) click(w http.ResponseWriter, r *http.Request) {
	link, ok := s.links.Get(r.PathValue("id")) // in-memory copy, refreshed about once a minute
	if !ok || link.Status != "active" {
		http.Redirect(w, r, s.fallback(link), http.StatusFound)
		return
	}
	id, _ := uuid.NewV7() // 16 bytes, time-ordered
	clk := s.signer.Sign(id[:], link.ID) // id ‖ first 8 bytes of HMAC(id, link)
	ev := Click{ID: id, At: time.Now(), LinkID: link.ID, PartnerID: link.PartnerID, IP: clientIP(r), UA: r.UserAgent()}
	select {
	case s.events <- ev: // one goroutine batches these into the ledger
	default:
		s.dropped.Inc() // buffer full: count it, never wait
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link.WithClick(clk), http.StatusFound)
}
```

The click id is a **UUIDv7 plus a short MAC** ([04](04-unique-id-generator.md)): time-ordered (good index locality, and the timestamp inside lets a lookup by id hit a single day-partition), and signed, so a forged id in a postback is rejected without touching the database.

### 9.2 Ledger vs counter

| Option | Pros | Cons |
|---|---|---|
| **Counter only** (a number on the product row) | trivial reads | no evidence, no re-classification; a hot row |
| The seed: **log row + counter on the row**, one transaction | the card reads one number; the log has what a report needs | two writes per click; the counter row is serial: ~500/s at one busy partner, each update leaves a dead tuple; and it needed a trigger exception so a click does not bump `updated_at`, the version an open editor saves against |
| **Ledger + rollup table**, counters derived | writes are inserts only (batched); reads hit small rollups; recomputable | rollups lag by seconds; a rollup job to run |
| Ledger only, aggregate on read | simplest | 66.7 M rows a day: not a read path |

**Choice and why: an append-only ledger and derived rollups.** The batcher writes with `COPY` or multi-row `INSERT … ON CONFLICT (click_id, clicked_at) DO NOTHING` (a redelivered event has the same id and time, so it conflicts and vanishes: idempotent). The rollup job sums *per key per batch*, then upserts once per key, so the "hot partner" is one row update every few seconds instead of 500 a second ([06](06-view-like-counter.md) calls this local batching). Retention is a **partition drop** (day by day after 90 days), never a `DELETE` of billions of rows; before dropping, archive to object storage so a late dispute still has evidence.

Crash maths: 20 redirector pods each hold ≤ 200 ms of buffered clicks at ~40/s: a pod crash loses ~8 clicks, about 0.0000004% of a month's 2 B (8 ÷ 2,000,000,000). That is far below any reconciliation tolerance, so a durable queue in front is optional here. If a contract demands zero loss, put the log first ([18](18-inbound-webhook-receiver.md)'s durable-accept pattern) and let the writer ack after commit.

### 9.3 Bot and fraud filtering, and billing-grade counting

The seed's rules are the first three layers of a longer list:

| Layer | Rule | Cost | Note |
|---|---|---|---|
| 1 | Prefetch headers, known crawlers and link-preview agents (User-Agent marks, empty UA); verified crawler ranges by reverse DNS ([34](34-crawler-ready-spa.md)) | microseconds | the seed does the UA part |
| 2 | Internal traffic: staff accounts, office and partner-owned ranges | microseconds | the seed excludes `dashboard.access` |
| 3 | **Dedupe window**: the same visitor on the same link within 30 minutes counts once | one Redis `SET key NX EX 1800` | the seed does it with a `SELECT` in the request |
| 4 | Velocity: clicks per visitor, per /24, per ASN, per link (token bucket, [03](03-rate-limiter.md)) | Redis | catches simple scripts |
| 5 | Behaviour: time from page render to click (the link carries a signed render time `t`); a click 80 ms after render is not a thumb ([17](17-abuse-protection.md)'s timing trap) | microseconds | needs `t` signed so it cannot be forged |
| 6 | Statistics: datacenter ASNs, click-through rate by source far above the median, one User-Agent string for 40% of a source | batch job | produces verdicts after the fact |

Two details the seed gets right for one process and breaks for many:

- `visitor` there is salted with a pepper that lives **only in one process**; with 20 pods the same person hashes differently on each, and the dedupe silently weakens. At scale the classifier computes the hash centrally with a **shared daily key** ([47](47-web-analytics.md) §9.2); the redirector ships the raw address only to the pipeline, which hashes and drops it (keeping, if fraud review needs it, a truncated /24 for 30 days).
- Redis `NX EX` has exactly the seed's meaning: the key is set by the first *counted* click and expires 30 minutes later; duplicates do not extend it.

**Verdicts are judgments, not facts.** Each rule set has a `rules_version`; the classifier appends a verdict row. A fraud finding two weeks later is a new version for the affected clicks, and the rollup for those days is **recomputed** (replace, not add), so running it twice is harmless. The invoice for a closed month is a frozen snapshot; a later reversal appears as a credit on the next one.

**Reconciliation.** Monthly, compare our valid count per link with the partner's own report. Within the agreed tolerance (assume 2%) we invoice ours. Beyond it, diff by day and source, then sample click rows (the export in §7 exists for this). Common causes are time-zone day boundaries ([60](60-business-day-time-zones.md)), the dedupe window, and the partner counting prefetches.

### 9.4 Attribution: from click to conversion

We cannot see a purchase on the partner's site; they tell us. The click id is the thread:

1. The redirect appends `clk=<click id ‖ mac>` to the partner URL.
2. The partner stores `clk` with the visitor's session and, on a conversion, calls `POST /postbacks/{partner}` server-to-server (cookies in the browser are fragile; a server call is not).
3. We verify the signature on the body ([18](18-inbound-webhook-receiver.md): HMAC, timestamp window, per-partner secret), then verify the **MAC on `clk`**, which rejects forged ids without a lookup, and read `clicked_at` out of the UUIDv7 to find the one partition.
4. Insert into `conversions` keyed by `(partner_id, event_id)`: a retried postback answers `200 duplicate`.
5. Apply the contract: an attribution window (say 30 days from click), last click wins, reversals (a refund postback) subtract.

| Option | Pros | Cons |
|---|---|---|
| **Click id + server postback** | robust, auditable, per-conversion | the partner must integrate |
| Cookie / pixel on the partner's page | no partner code | third-party cookies are going away; blockers |
| Partner's periodic report only | zero integration | no per-click evidence; monthly lag |

**Choice and why: the click id with a signed postback**, with the partner's report kept as the reconciliation source.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (20 B clicks, ~7.7k/s average, ~50k/s peak):** not the redirect (it is a map lookup and a channel send), but (1) the **ledger's index and disk** (107 GB/day: partitions on fast storage, one index only, `COPY` in 1,000-row batches), (2) **verdict lookups** (shard the classifier by `hash(link_id, visitor)` so one worker sees one key's order), and (3) analytics reads from Postgres: move raw-click exploration to a columnar store, keep rollups in Postgres.
- **Hot link (a viral promo, 10k/s on one link):** the cached link row and the sharded dedupe keys handle it; the old per-row counter would not.
- **Database down:** the batcher buffers in memory, then spills to a local append-only file, replays on recovery; the redirect keeps answering. Clicks lost are the buffer's overflow, counted in `dropped`.
- **Redis down:** dedupe fails *open* (counts may be briefly high); the classifier re-runs the window rule over the ledger afterwards, because the ledger kept everything.
- **Redirector deploys and crashes:** lose ≤ one flush interval of buffered clicks; flush on graceful shutdown.
- **Link cache stale:** a paused link keeps redirecting for up to a minute; accept it, or push invalidations.
- **Single points of failure:** the redirector fleet is the link: run it in several regions behind anycast or geo-DNS; the partner's own downtime is not ours to hide.
- **Abuse of our endpoint:** `t` and `s` parameters are signed; unknown links cost one map miss; rate limit per IP ([03](03-rate-limiter.md)) protects the cache refresh path.
- **Privacy:** only salted daily hashes and a coarse country in the ledger; click rows reference no person unless the visitor is signed in.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| first-party `302` with a signed click id | beacon or `ping` | works in email and apps; evidence; the id travels to the partner | one extra hop; our uptime is the link's |
| non-blocking enqueue in the request | the seed's transaction before the redirect | a flat, tiny latency; no coupling to the database | up to one flush interval of clicks lost on a crash |
| append-only ledger + derived rollups | a counter column on the row | no hot row, no `updated_at` trigger workaround, recomputable counts | a rollup job and a few seconds of lag |
| verdicts in their own table, versioned | flags edited in place | repeatable counts, "why was this not billed" | more rows and a join |
| shared daily-rotating visitor key | a per-process pepper | dedupe that works across pods, still no cross-day tracking | a key to distribute and rotate |
| postback with click id | cookies or pixels | robust, per-conversion, auditable | partners must build the call |

## 12. Check yourself

1. Why a `302` and not a `301`, and what must the endpoint never accept as input?
2. The seed keeps a counter column and a log. What goes wrong with the counter at 500 clicks/s on one partner, and what replaces it?
3. How do you make a redelivered click event harmless?
4. A fraud rule added today finds bots among last week's clicks. What changes, and what must stay unchanged?
5. A partner's postback arrives twice and once with a forged `clk`. What do you answer each time?

<details><summary>Answers</summary>

1. A `301` is cacheable: the browser skips us next time and the click is never seen. Use `302`/`307` with `no-store`. The endpoint must never take the destination from the request (`?url=…`): that is an open redirect. It looks the link up by id in our own table.
2. Every click updates the same row: the row lock serialises them (each transaction holds it for its whole commit, so the ceiling is some hundreds of updates a second, and 500/s is at or past it), and each update leaves a dead tuple to vacuum. The seed also had to change the table's `updated_at` trigger so counting is not treated as an edit. Replace it with append-only ledger rows and a rollup updated once per key per batch.
3. The redirector assigns `click_id` and `clicked_at` once; the ledger's primary key is `(click_id, clicked_at)` and the insert is `ON CONFLICT DO NOTHING`. The same event redelivered conflicts and is dropped.
4. Append `click_verdicts` rows under a new `rules_version` and recompute the affected days' rollups (replace, not add). The raw `clicks` rows never change, and an already invoiced month is not edited; a correction becomes a credit on the next invoice.
5. The duplicate: `200 duplicate` (same `partner_id` and `event_id`), nothing inserted. The forged id: the MAC on `clk` fails before any database lookup; answer `422` and count it per partner (a rising rate is a signal).

</details>

## 13. In Go

- Related lessons: [20 HTTP server](../../20-http-server.md) (handlers, `http.Redirect`, graceful shutdown), [13 Goroutines](../../13-goroutines.md), [14 Channels](../../14-channels.md) and [15 Sync & context](../../15-sync-context.md) (the buffered `events` channel, a single batcher goroutine, flush on a ticker and on shutdown), [38 Caching](../../38-caching-patterns.md) (the in-memory link cache), [55 Data pipelines](../../55-data-pipelines.md), [57 Web security](../../57-web-security.md) (open redirects, HMAC), [59 Production DB patterns](../../59-production-database-patterns.md) (partitions, `ON CONFLICT`, `COPY`), [68 Rate limiting](../../68-rate-limiting-throttling.md).
- The redirector holds no per-request state: `links` is a `sync.Map` or an `atomic.Pointer[map[string]Link]` swapped by a refresher; the batcher owns a slice and flushes at 1,000 rows or 200 ms; on `SIGTERM` stop accepting, drain the channel, flush, then exit.
- Click ids: `github.com/google/uuid` (`NewV7`), `crypto/hmac` with `crypto/sha256`, `hmac.Equal` for the check.
- Batching to Postgres: `pgx` `CopyFrom` into a staging table, then `INSERT … SELECT … ON CONFLICT DO NOTHING`; or multi-row inserts through `pgx.Batch`.
- Packages: `net/http`, `pgx`, `go-redis` (`SetNX`), a Kafka or NATS client only if you add the durable log.

## 14. Extensions

- **Budgets and caps:** pause a link when the partner's monthly budget is spent; the cap check must read the rollup, not the ledger, and tolerate the lag.
- **Rotating destinations:** one tracked link, several partners by geography, device or weight; the click id still records which one served.
- **Real-time fraud scoring** on the stream (per-source anomaly detection) instead of a batch job; what does a pre-redirect "serve a clean page instead" decision cost in latency?
- **Server-to-server clicks** for apps that cannot follow redirects (a `POST` that returns the URL), and how that changes dedupe.
- **Privacy rules:** a consent-aware mode that omits `clk` for visitors who declined tracking, and what that does to attribution.
- **Chargebacks:** partners disputing a month 60 days later; how long raw clicks must live, and what an archived-to-object-storage lookup costs.
- **Multi-touch attribution:** several clicks before one conversion; a credit-split table instead of last-click.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption). Small in hardware terms: 772 clicks/s on average and 5k/s at peak is about half a vCPU of redirecting. The money sits in 90 days of evidence (≈ 1.65 TB on a Multi-AZ disk pair) and in the load balancer's per-connection charge.

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Redirector (Go, stateless: map lookup, UUIDv7, HMAC, `302`; TLS ends at the ALB) | 5k clicks/s at peak (§4) | CPU, barely | 5k × 0.1 ms = 0.5 vCPU. N = ceil(0.5 ÷ (0.8 × 2 × ⅔ = 1.07)) = 1 → 3 | `c7g.large` (2 vCPU / 4 GiB) on ECS/EKS | 3, one per zone | three for the zones, not the load: ~8 % busy at peak. Each pod holds the whole link cache (~200 B per link; the link count is not in section 4), so memory, not CPU, caps pods per node: the 20 pods of §9.2 are a blast-radius choice (a crash loses ~8 clicks) |
| Classifier + rollup + archiver (Go workers) | 5k verdicts/s at peak; rollup upserts; weekly partition archive | CPU, barely | classifier 5k × ~0.1 ms (Redis `SET NX`, rules, batched verdict insert) = 0.5 vCPU; rollups, the statistics batch and archiving ~0.5; total ≈ 1 vCPU. N = ceil(1 ÷ 1.07) = 1 → 3 | `c7g.large` | 3 (sharded by `hash(link_id, visitor)`, §10) | async, so a lagging worker only delays verdicts; one shard per zone |
| Redis (dedupe `SET NX EX 1800`, velocity counters) | 5k × (1 dedupe + ~4 velocity ops) ≈ 25k ops/s; keys 5k/s × 1,800 s = 9M (assumes the peak lasts 30 min, conservative) | memory (small), then one core | 9M × ~100 B = 0.9 GB × 2 (fork) = 1.8 GB ÷ 0.7 ⇒ ≥ 2.6 GiB; ops 25k × 10 µs = 0.25 core | `cache.m7g.large` (8 GiB), ElastiCache | 1 primary + 1 replica | 2.6 GiB is far under 8, so `m`, not `r`; the core has room (25 %). It fails open (§10), so the replica is only to avoid 30 minutes of weakened dedupe after a node loss |
| Postgres ledger (`clicks`, `click_verdicts`, `click_rollups`) | 5k click rows/s + 5k verdict rows/s at peak (772 + 772 average), in ~100 batch commits/s; 90 days online: 960 GB (§4) + verdicts ≈ 600 GB (assumed ~100 B per click with index; §4 omits them) + rollups 88 GB/year ≈ 1.65 TB | disk size, then write IOPS | 10k rows/s × ~0.5 IOPS (random index pages, coalesced by checkpoints; assumption) = 5k IOPS at peak, ~0.8k average ⇒ `gp3` at 6,000. Storage 1.65 TB ⇒ 2 TB. Hot set = today's index tail ≈ 66.7M × 40 B = 2.7 GB + heap tails ≈ 6–8 GB; CPU 10k × 25 µs = 0.25 vCPU + rollups ≈ 1 of 4 | `db.m7g.xlarge` (4 vCPU / 16 GiB), RDS Multi-AZ, 2 TB `gp3` at 6,000 IOPS | 1 primary + synchronous standby | the data is big but the hot part is only today, so `m`, not `r`: the bill is disks, not the instance (storage is cheap, IOPS are not). Old partitions drop whole. A dispute export scans cold partitions at `gp3`'s 125 MiB/s (320 GB ≈ 40 min), so export from the S3 archive |

Front door and bytes. The ALB costs by new TLS connections, not by servers: 772/s ÷ 25 per LCU = 31 LCU × $0.008 × 730 = $180, plus $16. The `302` is ~400 B out: 2 B × 400 B = 800 GB a month. S3 archive: dropped partitions are ≈ 520 GB a month (320 GB ledger + 200 GB verdicts), ÷ 4 compressed (assumed), × 12 months kept ≈ 1.6 TB.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Redirectors: 3 × `c7g.large` (3 × $53) | $160 |
| Workers: 3 × `c7g.large` | $160 |
| ALB: $16 + 31 LCU × $0.008 × 730 | $200 |
| Postgres: `db.m7g.xlarge` ≈ $240 managed, Multi-AZ doubles | $480 |
| gp3 2 TB ($160) + 3,000 extra IOPS ($15), × 2 for the standby | $350 |
| Redis: 2 × `cache.m7g.large` (2 × ≈ $120) | $240 |
| S3 archive: 1.6 TB × $0.023 | $40 |
| Data out: 800 GB × $0.09 | $70 |
| **Total** | **≈ $1,700** (≈ $1,340 with a 1-year plan on the $1,040 of steady compute) |

- **Biggest line and why:** Postgres plus its disks (≈ $830, 49 %), for keeping 90 days of evidence on a Multi-AZ pair, not for load; the 2 TB of disk alone is $350. The whole bill is ≈ 0.85 % of one point of partner discrepancy ($200k a month, §4), so this is money well spent. Levers: a shorter online window if the contract allows it, and one index only.
- **What changes at 10×:** 20 B clicks, ~50k/s peak (§10). Redirector 5 vCPU ⇒ N = ceil(5 ÷ 2.13) = 3 × `c7g.xlarge`, still small. The ALB's new-connection LCUs (≈ 308 ⇒ ≈ $1,800 a month) overtake all the servers: move TLS onto the redirectors behind an NLB. Redis needs ~250k ops/s (÷ 70k per core ≈ 4 shards) and ~90M keys (9 GB) ⇒ 4 shards of `cache.r7g.large`. The ledger is 107 GB a day ⇒ 9.6 TB + ≈ 6 TB of verdicts: past a comfortable single Postgres, so keep ~14 days in Postgres and move raw exploration to a columnar store (§10).

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- An append-only ledger: 2B clicks a month = 66.7M a day (~772/s average, 5k/s at peak), ~160 B a row, ~10.7 GB a day, ~960 GB for the 90 days kept online (section 4). Facts and verdicts are inserted in batches (~100 commits/s) and never updated.
- Reads are aggregates over small rollups (~5M rows a day) plus a few point lookups (a click by id for a postback) and dispute exports; nothing on the hot path joins.
- Retention is by time: after 90 days a whole day leaves, which should be a partition drop, never a `DELETE` of billions of rows.
- It is money: the count must be repeatable (recompute a day from facts plus rules), inserts must be idempotent (`(click_id, clicked_at)`), and a postback must be accepted once (`(partner_id, event_id)`).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen | declarative `PARTITION BY RANGE (clicked_at)` by day: retention is detach, archive to S3, drop; batched `COPY` plus `ON CONFLICT DO NOTHING` makes a redelivered click vanish; ledger, verdicts, rollups and conversions in one engine make "why was click X not billed?" a join, and `PRIMARY KEY (partner_id, event_id)` makes postbacks once-only; the ledger is insert-only, so vacuum and bloat (the hot counter's problem in the seed) do not apply |
| MySQL (RDS / Aurora) | ✔ works, interchangeable here | day partitions and `DROP PARTITION` exist too, with the same rule that every unique key contains the partition column; the PK starts with a time-ordered UUIDv7, so InnoDB's clustered index appends at the tail instead of splitting pages like a random UUID (task 04); nothing used here is Postgres-only |
| ClickHouse (self-run, ClickHouse Cloud) | ✔ works, not needed yet: the 10× home of the raw ledger | columnar and compressed, fast for dispute exports and re-classification over billions of rows; but no unique constraint (the idempotent insert becomes a ReplacingMergeTree, which de-duplicates only at merge time), weak single-row work, and it is not an AWS-managed service (Redshift or Athena over Parquet are the AWS-native analytics); section 10 already puts raw exploration there at 10× |
| DynamoDB | ✖ poor fit | keyed by `click_id` it spreads writes and TTL expires old items, but "all clicks of link X in October" is then a scan; keyed by link, one hot link at 10k/s passes the ~1,000 writes/s per partition limit; rollups and exports have no ad-hoc queries |
| Redis (ElastiCache) | ✔ as the derived dedupe and velocity store, ✖ as the ledger | `SET NX EX 1800` (~9M live keys, section 15) fails open, and the classifier re-runs the window rule over the ledger afterwards |

**Pick:** PostgreSQL on RDS Multi-AZ, the `db.m7g.xlarge` with 2 TB `gp3` at 6,000 IOPS from section 15, because day partitions turn 90-day retention into a metadata operation, `COPY` with `ON CONFLICT DO NOTHING` gives idempotent batch ingest, and facts, verdicts and conversions live in one engine. Record: `clicks`, `click_verdicts` (facts and judgments) and `conversions`. Derived and rebuildable: `click_rollups` and the Redis dedupe keys. Cold archive: dropped partitions go to S3, and disputes are served from there (section 15).

**If it were MySQL instead:** nothing material. `COPY` becomes `LOAD DATA` or multi-row `INSERT`; `ON CONFLICT DO NOTHING` becomes `INSERT IGNORE` (which also turns other errors into warnings) or `ON DUPLICATE KEY UPDATE`; `BYTEA` becomes `BINARY(16)`; the instant columns become `DATETIME` kept in UTC (a daily `RANGE COLUMNS` partition works on it, see 60 for the time types). The secondary index `(link_id, clicked_at)` carries the 16-byte primary key in InnoDB, which is why the key stays small. The choice is then team skill and tooling.

**What would flip the choice:** at 10× (~107 GB a day, ~9.6 TB online plus ~6 TB of verdicts) the raw ledger is too big for one comfortable Postgres: keep ~14 days there and move raw clicks to ClickHouse (or Parquet in S3 queried with Athena), while rollups, links and conversions stay relational (section 10). A contract that demands zero click loss calls for a durable log (Kafka/MSK) in front, not another database. Aurora instead of RDS if you would rather not provision and double a 2 TB disk: its storage grows on its own, and replicas can serve the partner dashboards, but storage and I/O are billed separately.
