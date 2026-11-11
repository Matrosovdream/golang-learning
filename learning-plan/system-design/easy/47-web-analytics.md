# 47 · Privacy-Friendly Web Analytics

`🟢 easy` · *Topics:* beacon ingestion, cookie-less visitors with a daily-rotating salted hash, 30-minute visits, HyperLogLog uniques, additive counters that survive redelivery, raw → hourly → daily rollups, business-day time zones and DST, pull vs push, precomputed read paths · *Builds on:* [06 View & Like Counter](06-view-like-counter.md) (hot counters, batching: not repeated here), [08 Uptime Monitor](08-uptime-monitor.md) (rollups and retention), [10 Webhook Delivery](10-webhook-delivery.md) (at-least-once)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [46 Roles & Permissions (RBAC)](46-rbac.md) · Next: [48 Bulk Import with Preview & Undo](48-bulk-import.md)

---

> 🌱 **Seed (honeydolist-site):** *Dashboard → Traffic* (PLAN step 63, migration 0081). The numbers come from a **provider's** beacon (Cloudflare Web Analytics) and are **pulled** by a worker task every 30 minutes: today and yesterday again (late events), then up to 30 older days a run until 180 days are filled. They are kept in `site_traffic_pages (day, path, views, visits, PRIMARY KEY (day, path))`, so the totals are the project's for good (the provider keeps about six months), and a one-row `site_traffic_state` holds `synced_at`, `oldest_day` (how far the backfill has walked) and the last error. Days are **business days in one fixed zone**, so each is its own provider window of 23, 24 or 25 hours; a day that had views and now comes back empty is **kept as it was**; a capped answer is an error, not a short count; the page **never calls the provider while someone waits**. "No visitor is stored: no IP, no cookie, no id." Migration 0080 (`product_clicks`, steps 61/66) logs clicks on partner links through a first-party redirect, skipping staff, crawlers and prefetches and the same visitor within 30 minutes; the visitor is the first 16 bytes of a **server-salted hash of address, browser and day**, which can be neither reversed nor followed from one day to the next. As recorded, it was built and checked against a stand-in; the real-provider phase was still open. Size: one site.
> **Here, at scale:** run the analytics yourself: 10B page views a month across 1M customer sites (~3.9k/s average, ~20k/s design peak), a beacon endpoint on the edge, and a dashboard that answers in tens of milliseconds.

## 1. The task

> "Design a privacy-friendly web analytics service. Customers paste a script on their site; they see views,
> visits, visitors and top pages per day, month and total, without cookies and without personal data."

Two tasks hide inside it: a **firehose** (billions of tiny events) and a **read side** that must feel instant. Between them sit the questions that make it interesting: how to count a visitor you are forbidden to remember, how a "day" is defined, and how to count exactly when your queue delivers messages twice.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| Scale? | Sets ingestion and storage. | 10B page views/month, 1M sites; the top sites send millions a day, the median a few hundred. |
| What do we report? | Defines the counters. | **Views** (page loads), **visits** (a view that starts a visit), **uniques** (approximate), top pages, referrers, countries, outbound clicks. |
| Cookies or stored identifiers? | Defines the privacy promise. | None. Nothing that identifies a person is stored (this is a design property, not legal advice). |
| What is a "day"? | A report must match the owner's calendar. | The site's own time zone, including 23- and 25-hour DST days. |
| Exact or approximate? | Exact counters are cheap; exact uniques are not. | Views and visits exact; uniques approximate (about 1%). |
| Freshness? | Real-time needs a different path. | "Today so far" within a minute; closed days exact. |
| History? | Storage. | Daily rollups forever; hourly 30 days; raw events 7 days. |
| Bots? | Skews every number. | Filter known bots, prefetches and headless clients; keep a "filtered" counter. |
| Pull from a provider or our own beacon? | The seed pulls. | Own beacon for first-party sites; the seed's pull loop stays for third-party sources. |

## 3. Requirements

**Functional**
- A tiny script sends a beacon per page view (and per outbound click) with no cookie.
- Per site: tiles (today, yesterday, this month, last month, total), a series by day or month, top pages, referrers, countries; a live "active now".
- Reports in the site's time zone; stable for closed days.
- Exclude the owner's own visits and known bots where possible.

**Non-functional**
- Ingest ~20k events/s peak without ever slowing the customer's page: the beacon answers `204` immediately.
- Dashboard reads under 100 ms p95, independent of event volume.
- A queue redelivery or a crashed worker must not change a total.
- Nothing personal at rest: no IP, no user agent, no stable identifier.

**Out of scope**
- Funnels, session replay, A/B testing, per-user paths, ad attribution.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Average ingest | ~3.9k/s | 10B ÷ (30 × 86,400 = 2.592M s) = 3,858/s |
| Design peak | 20k/s | ~5× average: evening traffic and a few viral sites |
| Events per day | 333M | 10B ÷ 30 |
| Raw storage | ~67 GB/day, ~470 GB for 7 days | 333M × ~200 B |
| Ingest nodes | 3 behind a load balancer | ~7k/s each at peak; parsing, bot test, hash and an in-memory batch are microseconds per event |
| Daily rollup rows | ~15M/day, ~0.7 GB/day, ~260 GB/year | assumption: 15 distinct paths per site per day × 1M sites × ~48 B |
| Hourly rollup | ~45M rows/day, ~65 GB for 30 days | ~3× the daily rows (not every hour is active) × ~48 B |
| Visit-dedupe keys | ≤ 36M, ~2.2 GB | worst case 20k/s × 1,800 s window × ~60 B in Redis |
| Uniques sketch | 12 KB dense, error ≈ 0.8% | HyperLogLog, 2¹⁴ registers × 6 bits; small sites use a sparse form of a few hundred bytes |
| Pulling this from a provider instead | ~555 calls/s | 1M sites × 48 pulls a day (every 30 min) = 48M/day |

What the numbers tell us: ingestion is a modest, **embarrassingly batchable** stream; the real design pressure is **correctness of counters under redelivery** and **keeping the read side tiny** (rollups are a few hundred GB a year, raw events are disposable). The last row is why the seed's pull model does not survive a platform of 1M sites.

## 5. Plan

1. Ingestion: beacon, validation, bot filtering, batching, a queue, and exact counters.
2. Counting people without remembering them: the salted daily hash, visits, uniques.
3. Rollups, retention and time zones (DST included).
4. Pull vs push: when the seed's design is right.
5. The read path: tiles, series and top pages without touching raw data.

## 6. High-level design

```
 browser ──POST /e (sendBeacon, no cookie)──► edge / ingest nodes ──► 204 at once
                                                 │ validate site key (in-memory map of 1M sites)
                                                 │ drop bots, prefetch, staff; strip query string; cap paths per site
                                                 │ visitor = HMAC(today's secret, site, ip, ua);  visit? = Redis SET NX EX 1800
                                                 │ aggregate in memory per (site, path, minute); flush every few seconds
                                                 ▼
                                      queue (JetStream / Kafka) ── partition by site_id
                                                 ▼
                                     Aggregators: consume batch → ONE transaction:
                                       flush_log(partition, offsets)  +  additive upserts into page_hour / page_day / site_day
                                                 ▼
 dashboard ◄── API ◄── Redis cache (60 s) ◄── Postgres rollups (partitioned by day)       raw events ► columnar/blob store (7 d TTL)
              └─ "today so far" from Redis live counters
 provider pulls (third-party sources) ─ worker ─► same rollup tables, idempotent whole-day writes (the seed's loop)
```

One page view: the script sends a beacon; an ingest node answers `204`, validates the site, classifies bots, hashes the visitor with today's secret, checks whether it starts a visit (one Redis `SET NX`), and adds it to an in-memory counter map. A few seconds later the batch goes to the queue; an aggregator applies it in one transaction together with its offsets.

## 7. API

```
POST /e                      Content-Type: text/plain           (no preflight; always 204, even for rejects)
     { "s": "<site key>", "t": "pageview" | "click", "p": "/pricing", "r": "https://referrer.example/x", "u": "https://partner.example/y" }
GET  /go/{token}             → 302 (outbound link: record the click, then redirect; the seed's Buy button works this way)
POST /v1/sites               { domain, tz }                     → 201 { id, site_key }
GET  /v1/sites/{id}/stats?range=30d|90d|12m|all&by=day|month
     → { tiles: { today, yesterday, this_month, last_month, total },        views, visits, uniques (approx)
         series: [{ at, views, visits }], pages: [{ path, views, visits, share }],
         referrers: [...], countries: [...], updated_at }
GET  /v1/sites/{id}/realtime → { active_now, per_minute: [...] }           (Redis only)
```

The beacon never returns data and never sets a cookie. A day with no traffic is a zero, not a gap (as in the seed's API).

## 8. Data model

```sql
CREATE TABLE sites (
    id BIGINT PRIMARY KEY, domain TEXT NOT NULL UNIQUE,
    tz TEXT NOT NULL DEFAULT 'UTC',              -- the business-day zone: an IANA name, never an offset
    site_key TEXT NOT NULL UNIQUE
);
CREATE TABLE paths (                              -- dictionary: a path is stored once, rollup rows carry the id
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    site_id BIGINT NOT NULL, path TEXT NOT NULL, UNIQUE (site_id, path)
);
CREATE TABLE page_day (                           -- the seed's site_traffic_pages, with a site and a path id
    site_id BIGINT NOT NULL, day DATE NOT NULL, path_id INTEGER NOT NULL,
    views  INTEGER NOT NULL CHECK (views  >= 0),
    visits INTEGER NOT NULL CHECK (visits >= 0),
    PRIMARY KEY (site_id, day, path_id)
) PARTITION BY RANGE (day);                       -- the day is the site's LOCAL date, assigned at ingestion
CREATE TABLE page_hour (                          -- 30-day retention; hour in UTC
    site_id BIGINT NOT NULL, hour TIMESTAMPTZ NOT NULL, path_id INTEGER NOT NULL,
    views INTEGER NOT NULL, visits INTEGER NOT NULL,
    PRIMARY KEY (site_id, hour, path_id)
) PARTITION BY RANGE (hour);
CREATE TABLE site_day (                           -- per-site totals and the uniques sketch
    site_id BIGINT NOT NULL, day DATE NOT NULL,
    views BIGINT NOT NULL, visits BIGINT NOT NULL, uniques_sketch BYTEA,   -- HLL; or an extension type
    PRIMARY KEY (site_id, day)
);
CREATE TABLE flush_log (                          -- makes additive counters idempotent
    partition INTEGER NOT NULL, from_offset BIGINT NOT NULL, to_offset BIGINT NOT NULL,
    PRIMARY KEY (partition, from_offset)
);
CREATE TABLE daily_secrets (                      -- random per local date; deleted after ~48 h: this is the privacy promise
    day_key DATE PRIMARY KEY, secret BYTEA NOT NULL
);
CREATE TABLE pull_state (                         -- the seed's site_traffic_state, per source and site
    source TEXT NOT NULL, site_id BIGINT NOT NULL,
    synced_at TIMESTAMPTZ, oldest_day DATE, error TEXT, error_at TIMESTAMPTZ,
    PRIMARY KEY (source, site_id)
);
```

Raw events (when drill-down is a product feature) go to an append-only columnar or object store with a 7-day TTL: `(site_id, ts, path_id, referrer_host, country, device, visitor_hash)`. The visitor hash may be kept in raw rows only for as long as that day's secret exists.

## 9. Deep dives

### 9.1 Ingestion: batch, filter, and count exactly

Order of work per beacon: validate the site key (an in-memory map of 1M entries, ~50–100 MB, refreshed from the database); reject on `Origin`/`Referer` mismatch with the site's domain; drop bots and prefetches (`Sec-Purpose: prefetch`, known crawler user agents, datacenter ranges, headless signatures; the seed skips crawlers, prefetches and staff for clicks the same way); strip the query string (the seed does); then classify the referrer host, country (a local GeoIP database, no external call) and device class from the user agent, and discard the IP and the user agent.

**Cardinality cap.** A path like `/item?x=<random>` or `/user/<id>` creates millions of distinct paths and blows up the rollup. Cap distinct paths per site per day (say 1,000 plus an `(other)` bucket), and let customers declare path patterns (`/user/*`).

**Hot counters.** A site with 1M views an hour is 278 upserts a second on one row. Do not write per event: each ingest node keeps a `map[(site,path,minute)]counts` and flushes every few seconds ([06](06-view-like-counter.md)); after the queue and aggregator it is one additive upsert per (site, path, hour or day) per batch.

**Exactness under redelivery.** The seed's pull job is idempotent by construction: it **writes each day whole** (a re-run replaces, never adds). Our push counters are *additive*, and a queue delivers at least once, so a crash after the commit but before the offset is stored would double-count a batch. Fix it by committing the **offsets in the same transaction as the counts**:

```sql
BEGIN;
INSERT INTO flush_log (partition, from_offset, to_offset) VALUES ($1, $2, $3);   -- a replayed batch fails here: skip it
INSERT INTO page_day (site_id, day, path_id, views, visits)
SELECT * FROM unnest($4::bigint[], $5::date[], $6::int[], $7::int[], $8::int[])
ON CONFLICT (site_id, day, path_id)
DO UPDATE SET views = page_day.views + EXCLUDED.views, visits = page_day.visits + EXCLUDED.visits;
COMMIT;
```

| Option | Pros | Cons |
|---|---|---|
| Write every event to the database | simplest | 20k inserts/s plus hot rows; the database is the bottleneck |
| Count in Redis only | very fast | a Redis loss loses history; not a record |
| **Batch at the edge, queue, additive upsert with offsets in the same transaction** | exact; replay-safe; ~20 statements a second | a queue to run; seconds of delay |
| Sample events and scale up | cheap at huge scale | totals are estimates; only acceptable for *raw drill-down*, never for the headline counters |

**Choice and why: edge batching + queue + transactional offsets.** Exact counters are cheap to keep once the writes are batched; sampling is reserved for raw rows of the very largest sites.

### 9.2 Counting people you must not remember

A **visitor id** is derived, not stored: `HMAC(secret(date_key), site_id ‖ ip ‖ user_agent)` truncated to 16 bytes, where `secret(date_key)` is a random secret for the site's local date, **deleted about 48 hours later**. After deletion nothing can reproduce or link yesterday's ids, which is the privacy property: the same person is a different id tomorrow, and a different one on every site (including `site_id` prevents cross-site tracking). The seed does the same for its click log (a salted hash of address, browser and day, 16 bytes).

```go
func visitorID(secret []byte, siteID int64, ip netip.Addr, ua string) [16]byte {
	h := hmac.New(sha256.New, secret) // today's random secret for this site's local date
	_ = binary.Write(h, binary.BigEndian, siteID)
	h.Write(ip.AsSlice())
	h.Write([]byte{0})
	h.Write([]byte(ua))
	var id [16]byte
	copy(id[:], h.Sum(nil))
	return id
}
```

- **Visits.** A view starts a visit if this visitor id has no view on this site in the last 30 minutes: a single atomic `SET visit:{site}:{id} 1 NX EX 1800`. If it returns OK, `visits += 1`. Worst-case memory is the 36M keys (~2.2 GB) of the estimate table.
- **Uniques per day.** Add the id to a HyperLogLog per `(site, day)`: 12 KB dense, ~0.8% error, mergeable (a month's uniques is a union of the days' sketches, not a sum of daily counts). Do not keep sketches per path (too many); keep them per site and for the top paths only.
- **Known limits, to say out loud:** two people behind one office address and the same browser look like one; one person on two devices looks like two; a day boundary splits a night owl. These errors are acceptable for analytics and are the price of not storing anything.

| Option | Pros | Cons |
|---|---|---|
| Cookie or local-storage id | accurate across days | needs consent banners in many places; it *is* a stored identifier |
| **Daily rotating salted hash** | no stored identifier; no cross-day or cross-site link | no multi-day uniques; shared IPs undercount |
| Fingerprinting | cross-day accuracy | exactly what privacy tools exist to prevent |
| No uniques at all | simplest | customers expect the number |

**Choice and why: the daily salted hash + `SET NX` visits + HLL uniques.** It reproduces what the seed promises ("no IP, no cookie, no id") while still giving customers a unique-visitor figure.

### 9.3 Rollups, retention and time zones

Three tiers, each smaller than the last: **raw** (7 days, disposable), **hourly** per (site, path) for 30 days (live charts and "today"), **daily** per (site, path) and per site, forever (the seed's `site_traffic_pages` rows, now with a site and a path id). Daily rows are about 260 GB a year; partitioned by day they stay easy to back up, and old partitions can move to cheap storage.

| Storage | Pros | Cons |
|---|---|---|
| **Postgres, partitioned, for rollups** | queries are by site and date range: tiny indexed reads; one system | not meant for scanning billions of raw rows |
| Columnar OLAP store (ClickHouse-style) for raw events | cheap scans for drill-down | another system; skip it until drill-down is a feature |
| Everything in the OLAP store | one query language | operational weight for what is mostly range reads by key |

**Choice and why: Postgres rollups, a disposable raw store only if drill-down is a product feature.**

**The business day.** Assign each event's *local date* at ingestion (`date(now() AT TIME ZONE site.tz)`) and write the **daily row directly** from that; do not derive days from UTC hours. Hours are UTC and do not align with a local midnight in zones such as +5:30 (India) or +5:45 (Nepal), and a day is 23 hours on a spring-forward date and 25 on a fall-back one (the seed has this exact rule: each business day is its own window of 23, 24 or 25 hours). Reports use the site's zone for "today", "this month" and the series, and the zone is part of the cache key. If a customer changes their zone, old days keep their old boundaries (say so); recomputing history needs raw data.

### 9.4 Pull vs push

| | Pull from a provider (the seed) | Push: our own beacon |
|---|---|---|
| Infrastructure | none beyond a worker and a table | ingest nodes, queue, aggregators |
| Visitor data held | none, by construction | none, by the salted-hash design |
| Fidelity | the provider's definitions (their "visit"), their caps and delays | ours, to the minute |
| History | the provider's retention (~6 months); we keep what we pulled | forever |
| Cost at 1M sites | ~555 API calls a second against a third party's limits | our own compute |
| Failure mode | the provider is down or rate-limits: the page shows a stale "updated" time and the reason | ingest down: events lost unless the script retries |

**Choice and why: push for first-party sites, and keep the seed's pull loop for third-party sources** (a search console, an ad platform, a payment provider's reports). The pull machinery is worth copying exactly: a state row per source with `synced_at`, `oldest_day` and the last error; today and yesterday re-read each run (late events); a backfill that walks backwards a bounded number of days a run; **whole-day idempotent writes**; keep a day that had data if the provider now returns none; treat a capped answer as an error, not a short count; and never call the provider from a page request.

### 9.5 The read path

The dashboard never touches events. Closed days come from `page_day` and `site_day`; **"today so far"** comes from the live counters in Redis (flushed by the aggregators every few seconds) merged on read; **total** is a sum over at most ~3,650 daily rows per site (ten years), or a `site_totals` row kept by the aggregator. **Top pages** is `GROUP BY path_id ORDER BY sum(views) DESC LIMIT 20` over a site's daily rows for the range: ~1,350 rows for 90 days at 15 paths a day, trivial; for a site with 100k distinct paths a day, keep a pre-aggregated monthly top-N or a heavy-hitters sketch (Space-Saving). Cache `(site, range, tz)` for 60 seconds. Show **"Updated 12 minutes ago"** and, when the last source failed, the reason in words, as the seed does.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the aggregator's transaction rate on the largest sites and the cardinality of rollups. Partition the queue by `site_id` so one site's counters have one writer; shard Postgres by site across clusters; move hourly rollups to a columnar store.
- **Ingest node dies:** beacons in flight are lost (acceptable: analytics tolerate small loss); the load balancer drains it. The script uses `sendBeacon` and does not retry, so the customer's page is never slowed.
- **Queue or aggregator behind:** the queue buffers; "today so far" lags; closed-day numbers are unaffected. Alert on consumer lag.
- **Redis lost:** visits are briefly over-counted (every visitor looks new once) and "active now" resets; the rollups are untouched.
- **Replay and crash:** `flush_log` makes a replayed batch a no-op.
- **Abuse:** fake beacons for a site you do not own (check `Origin`), floods (a per-IP and per-site rate limit, [03](03-rate-limiter.md)), and path-cardinality attacks (the cap).
- **Secret hygiene:** deleting `daily_secrets` on schedule is a privacy control: monitor it and alert if a secret older than 48 hours exists.
- **Backups:** back up rollups and `sites`, never the secrets (the seed's backup likewise carries its traffic tables because the provider forgets after six months).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Own beacon (push) | pulling a provider for 1M sites | fidelity, history, no vendor limits | ingestion infrastructure |
| Salted daily hash | cookies / fingerprints | no stored identifier, no consent banner | multi-day uniques; shared-IP undercount |
| Offsets in the counting transaction | at-least-once additive counters | exact totals under replay | a `flush_log` row per batch |
| Edge batching | an insert per event | ~20 statements/s instead of 20k | seconds of delay |
| Daily rows written at ingestion | days derived from UTC hours | correct local days incl. DST and +5:45 zones | changing a site's zone does not rewrite history |
| HLL for uniques | exact distinct counts | 12 KB per site-day, mergeable | ~0.8% error |
| Postgres rollups, disposable raw | everything in an OLAP store | simple operation | no ad-hoc raw queries beyond 7 days |

## 12. Check yourself

1. How do you count unique visitors without a cookie and without storing the IP, and what privacy property does deleting yesterday's secret give?
2. Our aggregator crashes after committing counts but before acknowledging the queue. How do totals stay exact?
3. Why is the seed's pull job idempotent while our push counters are not, and what makes ours safe?
4. Why assign the local date at ingestion instead of summing UTC hours into days?
5. Why does the seed's pull design stop working at 1M sites, and what of it is still worth keeping?

<details><summary>Answers</summary>

1. Derive an id as `HMAC(secret(date), site, ip, user_agent)` truncated to 16 bytes, keep only the id for the day's counting (a set or a HyperLogLog), and discard the IP and user agent. The secret is random per date and deleted after about 48 hours, so nobody (including us) can recompute or link a visitor's id across days; including the site id also prevents linking across sites.
2. The aggregator writes `(partition, from_offset, to_offset)` to `flush_log` in the same transaction as the additive upserts. After a crash the same batch is delivered again, its `flush_log` insert violates the primary key, and the batch is skipped. Counts and offsets commit or fail together.
3. The pull job re-reads a full day from the provider and writes the day whole, so running it twice gives the same row. Push counters are increments, so applying one batch twice adds it twice; recording the batch's offsets in the same transaction converts "at least once" into "once in effect".
4. A local day is 23, 24 or 25 hours, and in zones like +5:30 or +5:45 its boundaries fall inside a UTC hour, so hourly UTC buckets cannot be summed into correct days. Writing the daily row from the site's local date at the moment of the event is correct by construction.
5. 1M sites × 48 pulls a day is ~555 calls a second against a third party's rate limits, and the history is limited to what they keep. The pattern is still right for third-party sources: a state row (`synced_at`, `oldest_day`, error), re-reading today and yesterday, a bounded backwards backfill, whole-day idempotent writes, treating a capped answer as an error, and never calling the provider on a page request.

</details>

## 13. In Go

- Related lessons: [55 Data pipelines](../../55-data-pipelines.md) (batching, stages), [34 Event-driven & outbox](../../34-event-driven-outbox.md) (at-least-once and idempotent consumers), [38 Caching](../../38-caching-patterns.md) (read-through, 60 s TTL), [13 Goroutines](../../13-goroutines.md) and [15 Sync & context](../../15-sync-context.md) (per-node aggregation maps, flush tickers, graceful shutdown), [68 Rate limiting](../../68-rate-limiting-throttling.md), [39 Observability](../../39-observability-tracing.md) (consumer lag, per-source freshness).
- Ingest node: an `http.Handler` that parses with `io.LimitReader` (cap the body at 2 KB), writes `204`, and hands the event to a `chan` consumed by one aggregator goroutine owning `map[key]*counts`; flush on a `time.Ticker` or when the map reaches a size; on shutdown, flush before exit.
- Time: `time.LoadLocation(site.TZ)` once per site (cache it), `t.In(loc).Format("2006-01-02")` for the local date; test the DST dates and a +5:45 zone (`Asia/Kathmandu`).
- Visit dedupe: `SET key 1 NX EX 1800` through `go-redis` (`SetNX` returns whether it was set).
- Packages: `net/netip`, `crypto/hmac`, `github.com/redis/go-redis/v9`, `github.com/nats-io/nats.go` (JetStream) or a Kafka client, `pgx` (`CopyFrom`, `unnest` upserts).

## 14. Extensions

- **Funnels and paths:** needs sessions beyond 30 minutes and a stable (but still private) per-day id; where does the sequence live?
- **Heavy-hitters:** top pages and referrers with Space-Saving or count-min sketches for sites with huge cardinality.
- **Real-time map and "active now":** Redis sorted sets by last-seen time, a 5-minute window.
- **Consent and Do-Not-Track:** honour a customer's setting at the script and the edge; what changes in the numbers?
- **Cross-day uniques with privacy:** why even a weekly salt changes the privacy story, and what to tell customers.
- **Outbound click tracking** at scale (the seed's `/go/` redirect): dedupe, bot filtering, partner reports ("how many did you send us this month").

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Ingest nodes (Go, `204` at once) | 20k events/s peak, 3.9k average (§4); ~7k/s per node over 3 nodes | CPU (small) | 20k × 0.06 ms (JSON ≤ 2 KB, HMAC, GeoIP, bot test, Redis `SET NX` client, map update; TLS ends at the ALB) = 1.2 vCPU; on `c7g.large`: N = ceil(1.2 ÷ (0.8 × 2 × ⅔ = 1.07)) = ceil(1.12) = 2 → 3 | `c7g.large` (2 vCPU / 4 GiB) | 3, one per zone | CPU-bound and tiny: 20 % busy normally, 30 % with a zone down. Memory is the 1M-site key map (~100 MB) plus the aggregation map, so `c`, not `m` |
| Redis (visit dedupe, live counters, uniques, 60 s dashboard cache) | 20k `SET NX`/s plus counter and sketch updates ≈ 25k ops/s (assumption) | memory, then single-core ops | dedupe 36M keys × 60 B = 2.2 GB × 1.7 = 3.7 GB (§4); live counters and HLLs ≈ 3 GB and the dashboard cache ≈ 1.5 GB (assumptions) ⇒ ≈ 8 GB; `maxmemory` ≤ 70 % of 16 GiB = 11.2 GiB; 25k ops/s is under one core's ~100k | `cache.r7g.large` (2 vCPU / 16 GiB, ≈ $156) | 1 primary + 1 replica | memory-bound, so `r`: 8 GB of data behind a core doing 25k ops/s; `m7g.large` (8 GiB, 5.6 usable) cannot hold it. Losing Redis only over-counts visits briefly (§10) |
| Queue (raw events and batches) | 20k events/s × 200 B = 4 MB/s if raw events ride it (§4) | disk and network, both far from their limits | 4 MB/s × 86,400 s = 345 GB/day × 3 replicas ≈ 1 TB for 24 h of retention; brokers see 4 MB/s in, 8 MB/s of replication, well under one partition's tens of MB/s | MSK `kafka.m7g.large` (2 vCPU / 8 GiB); self-run JetStream on `m7g.large` is about half the price | 3, one per zone | availability, not load: the stream is a tenth of what one broker moves. Partition by `site_id` (say 12 partitions) so one site's counters have one writer |
| Aggregators + dashboard API + raw writer (Go) | 20k events/s in; ~20 statements/s out (§9.1); dashboards ≈ 2 views/s average, 20 peak (assumption: ~150k a day) | CPU (small) | aggregate 20k × 20 µs = 0.4 vCPU; Parquet raw writer 20k × 20 µs = 0.4; API 20 × 5 ms = 0.1 ⇒ 0.9 vCPU; N = ceil(0.9 ÷ 1.07) = 1 → 3 | `c7g.large` | 3, one per zone | stateless; three for availability, the queue's partitions spread over them |
| Postgres (page_day, page_hour, site_day, paths) | 20 statements/s ≈ 25k row upserts/s (assumption: 5 s flush ⇒ ~50k distinct keys per flush on each of page_day and page_hour, plus site_day); 260 GB a year daily, 65 GB hourly (§4) | memory, then IOPS | hot set = today's page_day (0.7 GB) + the last day's page_hour (2.2 GB) + paths, site_day, indexes ≈ 12 GB < 32 GiB. IOPS: ~3 GB of dirty pages per 5-min checkpoint ≈ 10 MB/s ≈ 1.3k, plus WAL ≈ 5 MB/s ⇒ ~3k ⇒ `gp3` 6,000. CPU 25k × 25 µs ≈ 0.6 vCPU of 4. Disk ≈ 0.5 TB in year 1 ⇒ 1,000 GB | `db.r7g.xlarge` (4 vCPU / 32 GiB), RDS Multi-AZ | 1 primary + synchronous standby | the rollups are cached, so memory then IOPS, never CPU: `r`. `m7g.xlarge` (16 GiB) would hold the 12 GB hot set with no slack. Counters are additive batches, so ~20 commits/s, not 25k |

Raw events on S3 (7-day expiry): 470 GB × $0.023 ≈ $11, nothing to size. Beacons arrive free; the only egress is ~300 B per `204` (10B × 300 B = 3 TB a month). One note on the sketch: `site_day.uniques_sketch` is a 12 KB value, so rewriting it on every flush is large WAL for busy sites; keep live HLLs in Redis and write them to Postgres hourly.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| Ingest: 3 × `c7g.large` (3 × $53) | $160 |
| Aggregators, API, raw writer: 3 × `c7g.large` | $160 |
| Redis: 2 × `cache.r7g.large` (2 × $156) | $310 |
| Queue: 3 × MSK `kafka.m7g.large` (3 × $120) + ~1 TB storage × $0.08 | $440 |
| Postgres: `db.r7g.xlarge` ($155 × 2 managed = $310), Multi-AZ doubles | $620 |
| gp3 1,000 GB + 3,000 extra IOPS (= $95, ×2 for the standby) | $190 |
| S3 raw events, 470 GB | $10 |
| ALB: ~960 new TLS connections/s (a quarter of the 3.9k/s average, assumption) ÷ 25 = 39 LCU × $0.008 × 730, + $16 | $240 |
| Egress: 10B `204`s × 300 B = 3 TB × $0.09 | $270 |
| **Total** | **≈ $2,400** (≈ $1,840 with a 1-year Savings Plan on compute) |

- **Biggest line and why:** Postgres with its disk (≈ 34 %), then the queue (18 %). All in, that is about $0.24 per million page views: this is a cheap workload because it is batched and the read side is a few hundred GB of rollups. The database is the one piece that cannot shrink: the rollups are the permanent record (the provider forgets after six months), so it stays Multi-AZ.
- **What changes at 10×:** 200k events/s ⇒ 12 vCPU ⇒ N = ceil(12 ÷ 1.07) = 12 × `c7g.large` for ingest. The dedupe set is 360M keys ≈ 37 GB and 200k `SET NX`/s is two cores, so Redis goes to cluster mode (~4 shards of `cache.r7g.xlarge`). The queue moves 40 MB/s, so `m7g.xlarge` brokers and more partitions. Postgres runs out first (as §10 says): 250k upserts/s is past one primary, so shard by site across clusters and move hourly rollups to a columnar store.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- A firehose in and a tiny read side out: 10B events a month, ~3.9k/s on average and 20k/s at peak, 333M events a day (~67 GB raw, ~470 GB for 7 days). The rollups are small: ~15M daily rows a day (~260 GB a year) and ~45M hourly rows a day (~65 GB for 30 days) (section 4).
- Writes are additive counters that must stay exact under redelivery: ~20 batched statements/s (~25k row upserts/s with a 5 s flush) that commit their queue offsets in `flush_log` in the same transaction (9.1).
- Reads are range scans by key: a site's days, `GROUP BY path_id … LIMIT 20` over ~1,350 rows, cached for 60 s. "Today so far", visit dedupe (`SET NX EX 1800`), live counters and the HyperLogLogs live in Redis (9.2, 9.5).

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for the rollups | `INSERT … ON CONFLICT DO UPDATE SET views = page_day.views + EXCLUDED.views` over `unnest` arrays plus a `flush_log` insert, in one transaction, gives exact counts under replay (a duplicate offset fails the batch); `RANGE` partitions by day give cheap retention and per-day backups; ~20 commits/s is nothing for one primary |
| MySQL (RDS / Aurora) | ✔ works, nothing material | `INSERT … ON DUPLICATE KEY UPDATE views = views + …` and a duplicate-key error on `flush_log` do the same job. The clustered key `(site_id, day, path_id)` makes a site's 90-day top pages one contiguous range, and in-place updates mean no vacuum behind counters that are rewritten every 5 s |
| ClickHouse | ✔ for raw events and drill-down, ✖ now for the rollups | the textbook store for 333M events a day: scanning 7 days (~2.3B rows) for drill-down is cheap and compression is high. But there is no transaction to hold `flush_log` and the counters together, and the rollups are a few hundred GB of key reads that do not justify a second system (9.3) |
| Redis (ElastiCache) | ✅ for live counters, visit dedupe and HLL, ✖ as the record | derived and rebuildable: losing it only over-counts visits for a moment (section 10); the rollups in Postgres are the permanent record |
| DynamoDB | ✖ poor fit | atomic `ADD` counters are easy, but the exact-once batch (~50k keys per flush on each of two tables) cannot be one transaction (limit 100 items), so replay safety is lost |

**Pick:** PostgreSQL on RDS Multi-AZ (`db.r7g.xlarge`, `gp3` 1,000 GB, section 15) is the system of record for `sites`, `paths`, `page_day`, `page_hour`, `site_day` and `flush_log`. Redis (`cache.r7g.large`, primary and replica) holds the derived state (visit keys, live counters, sketches, the dashboard cache); raw events go to S3 with a 7-day expiry and are disposable; MSK is a transport, not a store. Postgres wins on two properties: the transaction that commits offsets with counts, so a 50k-key batch is never half-applied and a replay is skipped; and day partitions, dropped for retention and backed up one day at a time. The read side is a few hundred GB of range reads, which an OLTP database serves cheaply. One tuning note (a rule of thumb): the counters are rewritten every flush, so keep those updates HOT (no indexed column changes, a lower `fillfactor` on the active partitions) to keep autovacuum ahead.

**If it were MySQL instead:** nothing material. The `unnest` upsert becomes a multi-row `INSERT … ON DUPLICATE KEY UPDATE` sent in chunks of a few thousand rows inside the same transaction; `BYTEA` becomes `BLOB`; the `RANGE` partitions keep working because `day` and `hour` are already in each primary key. In-place updates remove the HOT and vacuum tuning above. With the cheat sheet's rule, the choice is team skill and tooling.

**What would flip the choice:** a product requirement for drill-down over raw events (filter by referrer, country or device across 7 days, ~470 GB) points to ClickHouse, or S3 with Athena, for raw and hourly data. At 10× (200k events/s, ~250k upserts/s, section 10) one primary is not enough: shard by site, which is already the queue's partition key so each site keeps one writer, and move the hourly rollups to a columnar store while Postgres keeps the daily rollups and `sites`. At a tenth of the volume one small primary and Redis do it all, and raw events could stay in partitions.
