# 60 · Business-Day Time Zones & DST in Reports

`🟢 easy` · *Topics:* the kinds of time (instant, calendar date, wall-clock time, duration, zone) and what each is stored as, future events vs tzdata changes, defining "a day" once as a half-open window (23-, 24-, 25-hour days, :30 and :45 offsets), `AT TIME ZONE` semantics, rolling up on a 15-minute UTC grid, late data and closed days, a tenant changing zone, display and CSV export, DST golden tests and the two-tzdata problem · *Builds on:* [27 Deadlines & Escalation](27-deadlines-escalation.md) (DST in reminders: not repeated), [47 Web Analytics](47-web-analytics.md) (rollups and the business-day cut), [15 Job Scheduler](15-job-scheduler.md) (cron and DST), [28 Booking Calendar](28-booking-calendar.md) (dates as calendar days)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [59 Provider Adapters with Fake Drivers & Chaos Testing](59-provider-adapters-fakes.md) · Next: *(end of the easy set; medium backlog in [../medium/README.md](../medium/README.md))*

---

> 🌱 **Seed (honeydolist-site):** one business, one zone, decided once. `platform/dates` reads every date and time in `America/New_York` and embeds the zone data (`import _ "time/tzdata"`, because the distroless image has no zoneinfo). API dates are plain `YYYY-MM-DD` strings; `AddDays` works on UTC midnights so a clock change cannot shift one; `Start(date)` is the business's midnight "whatever the daylight saving"; a zone-less `2026-11-05T09:00` is read as the business clock (scheduled publishing, step 76). Histories show the exact time in that zone, with seconds and `EDT` on hover (step 51). The Traffic page (step 63, migration 0081; at the last record its real Cloudflare pull was still open) cuts **New York days, each its own 23-, 24- or 25-hour window**, re-pulls today and yesterday for late events, and keeps a day that comes back empty as it was. SQL uses `(created_at AT TIME ZONE 'America/New_York')::date` and `date_trunc('month', now() AT TIME ZONE …) AT TIME ZONE …`. Campaigns send in `window_from`/`window_to` wall-clock hours in the IANA zone of a US state (migration 0055); bookings are inclusive `date` ranges. Volume: one tenant.
> **Here, at scale:** a reporting product with 100k tenants across several hundred zones and 5 B events a day, where every daily total must match the tenant's own calendar day and every export must be unambiguous.

## 1. The task

> "Our customers see charts per day, week and month, and download CSVs. Each customer is in their own time zone. Make the numbers match their calendar, including the days with 23 or 25 hours, and don't let a daylight-saving change or a zone edit corrupt history."

The seed avoids the whole problem by having one zone. The lesson is what that choice was hiding: **a "day" is not 24 hours, and "the same time" is not one thing.**

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| One zone per tenant, or per user? | Who owns the day boundary. | Per tenant for reports ("the business day"); per user only for display of personal timestamps. |
| Does a day start at midnight? | A restaurant's day may end at 04:00. | Yes; a configurable day-start offset is an extension (§14). |
| Scale? | Grid size and storage. | 100k tenants, 5 B events/day ≈ 58k/s average, ~175k/s peak. |
| Late events? | When a day is "done". | Up to 24 h late is normal; a day is closed after a 1 h grace plus a recompute window. |
| Can a tenant change zone? | History semantics. | Yes, rarely (a move, a mistake); analytics follow the new zone, financial statements do not. |
| How far back can a report re-slice? | Retention of the fine grid. | 90 days of 15-minute buckets; older days keep the zone they were closed in. |
| Future dated things? | Where tzdata changes bite. | Yes: scheduled sends and appointments months ahead. |
| Exports? | Ambiguity at the fall-back hour. | CSV with local time, UTC time and zone, in ISO 8601. |

## 3. Requirements

**Functional**
- Daily, hourly and monthly report series per tenant in the tenant's zone; a day's length shown (`minutes`).
- Viewer-zone display of individual timestamps; tenant-zone display of aggregates, with the zone named in the UI.
- CSV and API exports that are unambiguous even across a DST change.
- A tenant can change zone; reports and statements behave as described in §9.3.
- Future wall-clock events (a send at 09:00 next March) fire at 09:00 local, whatever the rules become.

**Non-functional**
- The same raw events always produce the same daily totals; recomputing is safe.
- Daily and monthly sums reconcile exactly with the fine grid.
- The day boundary is computed in one place.

**Out of scope**
- Business-day calendars with holidays, fiscal calendars, locale-specific week starts (extensions).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Ingest rate | ~58k/s average, ~175k/s peak | 5 B ÷ 86,400 = 57,870/s; peak ≈ 3× |
| Raw events | ~500 GB/day | 5 B × ~100 B (assumption); short retention, not the report path |
| 15-minute rows | ~192 M/day, ~7.7 GB/day | 100k tenants × 96 buckets × ~20 active series per bucket (assumption) × ~40 B |
| Compression of the grid | ~26 events per row | 5 B ÷ 192 M |
| Hourly rows (for comparison) | ~48 M/day, ~1.9 GB/day | 100k × 24 × 20 × 40 B |
| Daily rows | ~2 M/day, ~80 MB/day, ~29 GB/year | 100k × 20 × 40 B; × 365 = 29.2 GB |
| 90 days of the 15-minute grid | ~690 GB | 7.7 GB × 90; partitioned by day |
| A 30-day chart | ~600 daily rows vs ~57,600 grid rows | 30 × 20 vs 30 × 96 × 20 |
| Day-close jobs | ~10k/hour at the busiest | assume 40% of tenants in 4 US zones: 10k each, one zone's midnight per hour; each reads 96 × 20 rows: trivial |

What the numbers tell us: the grid is cheap enough to keep as the **source of truth** (7.7 GB a day), daily rows are tiny, and the day-close herd is a non-problem (that is [27](27-deadlines-escalation.md)'s concern, not ours). The hard part is **correctness of definitions**, not volume.

## 5. Plan

1. Name the kinds of time and decide what each is stored as.
2. Define "a day" once, as a half-open window, and use that definition everywhere.
3. Roll up on a UTC grid every zone aligns to; derive daily rows from it.
4. Decide what late data, closed days and a zone change do.
5. Display and export without ambiguity.
6. Test the dates that break things, with an injected clock.

## 6. High-level design

```
 events (instants, UTC) ─► ingest ─► events_15m   UTC grid, zone-free facts        (source of truth)
                                         │ close job at the tenant's local midnight + grace (idempotent)
                                         ▼
 tenants.tz ──────────────────────► daily_rollups(local_day, tz, minutes)           (derived, recomputable)
 tenant_zone_history ─────────────────────┘
 report API:  closed days ← daily_rollups   ·   today ← events_15m   ·   the zone is resolved once and echoed back
 statements:  frozen (tz, starts_at, ends_at) at close — never re-sliced
 scheduled_items: local_start + tz (truth) ─► starts_at (a cache rebuilt when tzdata changes)
 window boundaries: computed by ONE component (the app's tz library); SQL receives instants
```

One request: "March for tenant 7": resolve the tenant's zone, compute the 31 local-day windows (the 8th is 23 hours), read closed days from `daily_rollups`, and today's partial day from the grid. The response carries `tz` and each day's `minutes`.

## 7. API

```
GET /v1/reports/events?from=2026-03-01&to=2026-03-31&group=day            (tenant zone; &tz=… to view another)
 → { "tz": "America/New_York",
     "buckets": [ …, { "day": "2026-03-08", "starts_at": "2026-03-08T05:00:00Z", "minutes": 1380, "n": 4120 }, … ] }
GET /v1/reports/events?from=2026-03-01&to=2026-03-02&group=hour&tz=Asia/Kathmandu
 → hours begin at :15 past the UTC hour here (+05:45), so only a 15-minute grid serves them exactly
GET /v1/exports/events?from=…&to=…        → 202 { url }   CSV: local_time, utc_time, tz, utc_offset, …
PUT /v1/tenant/timezone   { "tz": "Europe/Berlin" }   → 200 { "effective_from": "…", "reports_rebucketed": true }
```

Dates in the API are plain `YYYY-MM-DD` with no time and no zone; instants are RFC 3339 with `Z`; the zone is always a separate IANA id, never inferred from an offset.

## 8. Data model

```sql
CREATE TABLE zones (name TEXT PRIMARY KEY);              -- IANA ids, loaded from the tzdata the app ships

CREATE TABLE tenants (
    id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tz       TEXT NOT NULL REFERENCES zones(name),       -- never 'EST', never '-05:00'
    tz_since TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE tenant_zone_history (
    tenant_id      BIGINT NOT NULL,
    tz             TEXT   NOT NULL REFERENCES zones(name),
    effective_from TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, effective_from)
);

CREATE TABLE events_15m (                                 -- zone-free facts on a UTC grid
    tenant_id    BIGINT      NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,                    -- always :00, :15, :30 or :45
    series_id    INTEGER     NOT NULL,
    n            BIGINT      NOT NULL,
    PRIMARY KEY (tenant_id, bucket_start, series_id)
) PARTITION BY RANGE (bucket_start);

CREATE TABLE daily_rollups (                              -- derived for the tenant's zone; recomputable
    tenant_id   BIGINT   NOT NULL,
    local_day   DATE     NOT NULL,
    series_id   INTEGER  NOT NULL,
    tz          TEXT     NOT NULL,                        -- the zone this row was cut in
    n           BIGINT   NOT NULL,
    day_minutes SMALLINT NOT NULL,                        -- 1380 / 1440 / 1500 (1410 / 1470 at Lord Howe)
    closed_at   TIMESTAMPTZ,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, local_day, series_id)
);

CREATE TABLE scheduled_items (                            -- a future wall-clock event
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   BIGINT    NOT NULL,
    local_start TIMESTAMP NOT NULL,                       -- 2027-03-28 09:00, deliberately no zone
    tz          TEXT      NOT NULL REFERENCES zones(name),
    starts_at   TIMESTAMPTZ NOT NULL                      -- cache: derived from the two columns above
);

CREATE TABLE statements (                                 -- money documents freeze their zone
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    BIGINT NOT NULL,
    period_start DATE NOT NULL, period_end DATE NOT NULL,
    tz           TEXT NOT NULL, starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL,
    closed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## 9. Deep dives

### 9.1 Five kinds of time

| Kind | Example | Store as | Pitfall |
|---|---|---|---|
| **Instant** | a payment captured, an event received | `timestamptz` (UTC) | showing it in the server's zone |
| **Calendar date** | a booking's first day, an invoice date | `date`, a plain `YYYY-MM-DD` | converting it: midnight in one zone is the previous day in another |
| **Wall-clock time** | the shop opens at 09:00 | `time` + a zone id | storing it as a UTC time of day |
| **Future wall-clock event** | send at 09:00 on 2027-03-28 in Berlin | local `timestamp` + IANA id; the instant is derived | storing the instant computed today |
| **Duration** | "24 hours" vs "1 day" | an interval, with the unit that matters | `Add(24h)` where you meant "tomorrow at the same time" |

Rules that follow:

- **The zone is an IANA id** (`America/New_York`). Not an abbreviation (`CST` is US Central, China Standard and Cuba Standard), and not a bare offset (`-05:00` has lost the daylight-saving rules). Store an offset beside an instant only as a record of what applied then.
- **Future events keep local time + zone as the truth** and the UTC instant as a cache. Governments change the rules, sometimes on weeks' notice (Mexico abolished DST in 2022). An instant computed today is wrong the day the rules change; one derived from `local_start` + `tz` after a tzdata update is right. [27](27-deadlines-escalation.md) says the same as "recompute `due_at` after tzdata updates"; here it is the model.
- **Dates are not instants.** A `date` stays a `date` end to end. The classic bug is on the client: JavaScript parses a date-only ISO string (`new Date("2026-03-14")`) as **UTC midnight**, which displays as March 13 in the Americas, while a date-time string without an offset parses as *local*. The seed sidesteps it: `formatDate` builds `new Date(y, m-1, d)` from the parts, and its `addDays` works on `Date.UTC`, so a clock change cannot move the date (the Go side's `AddDays` does the same on UTC midnights).

```go
loc, _ := time.LoadLocation("America/New_York")
t := time.Date(2026, 3, 7, 12, 0, 0, 0, loc) // Saturday noon, EST

t.Add(24 * time.Hour) // 2026-03-08 13:00 EDT: exactly 24 real hours later
t.AddDate(0, 0, 1)    // 2026-03-08 12:00 EDT: tomorrow at the same wall clock (23 real hours)
```

### 9.2 What is "a day"?

A day is the window **[start of the local date, start of the next local date)**, half-open so consecutive days tile with no gap and no overlap. Its length is *derived*, never assumed:

| Zone | Date | Window starts (UTC) | Length |
|---|---|---|---|
| America/New_York | 2026-03-08 (spring forward) | 05:00 | **23 h** |
| America/New_York | 2026-11-01 (fall back) | 04:00 | **25 h** |
| Asia/Kolkata | any | 18:30 the evening before | 24 h (offset +5:30) |
| Asia/Kathmandu | any | 18:15 the evening before | 24 h (offset +5:45) |
| Australia/Lord_Howe | 2026-04-05 and 2026-10-04 | 13:00 / 13:30 the evening before | **24.5 h** and **23.5 h** (its DST shift is 30 minutes) |
| Europe/Berlin | 2026-03-29 | 23:00 the evening before | 23 h |

These come from running the snippet below, not from memory. Oddities worth knowing exist so that you use a library and never hand-roll: some zones have switched at midnight (Brazil did until 2019), so 00:00 does not exist on that date; Go documents that for a time in a gap or overlap `time.Date` returns a time correct in one of the two zones involved without guaranteeing which; Samoa skipped 30 December 2011 entirely.

```go
func dayWindow(loc *time.Location, y int, m time.Month, d int) (start, end time.Time) {
	start = time.Date(y, m, d, 0, 0, 0, 0, loc)
	end = time.Date(y, m, d+1, 0, 0, 0, 0, loc) // Date normalises d+1 across month ends
	return
}
// end.Sub(start): 23h0m0s on 2026-03-08 in New York, 25h0m0s on 2026-11-01, 24h30m0s at Lord Howe on 2026-04-05
```

**In SQL, do the same with the zone made explicit.** `timestamptz AT TIME ZONE z` gives the local wall-clock `timestamp`; `timestamp AT TIME ZONE z` gives the instant. So the instant a local day starts is a *local midnight timestamp* converted with the zone:

```sql
SELECT d::date                                              AS local_day,
       d AT TIME ZONE 'America/New_York'                     AS starts_at,
       (d + interval '1 day') AT TIME ZONE 'America/New_York' AS ends_at     -- d is a timestamp: +1 calendar day
FROM generate_series(timestamp '2026-03-01', timestamp '2026-03-31', interval '1 day') AS d;
-- the row for 2026-03-08 spans 05:00Z to 04:00Z the next day: 23 hours
```

Two traps: `timestamptz + interval '1 day'` adds a day **in the session's `TimeZone` setting**, which is a hidden input; and `date_trunc('day', now())` truncates in that same setting. The seed's idiom (`date_trunc('month', now() AT TIME ZONE z) AT TIME ZONE z`) is correct because both conversions name the zone.

**Choice and why:** one function in the application computes `[start, next_start)` instants for a tenant and date range; SQL receives instants and does plain `>=`/`<` comparisons. One definition of "a day" cannot disagree with itself.

### 9.3 Rollups: a grid every zone aligns to

| Option | Pros | Cons |
|---|---|---|
| **A. Local-day rows written at ingest** (what [47](47-web-analytics.md) does) | tiny; direct reads | needs the zone on the hot path; a zone change or a late event means rewriting; cannot re-slice by local hour or into another zone |
| **B. Fine UTC buckets as the source of truth, daily rows derived** | any zone, any day boundary, a viewer's zone, a zone change: all just re-aggregation | 4× the rows of hourly; a close job |
| C. Scan raw events on read | exact | 500 GB a day: not a read path |

**Choice and why: B, on a 15-minute grid.** Every UTC offset in use today is a multiple of 15 minutes (Kolkata +5:30, Kathmandu +5:45, the Chatham Islands +12:45, Lord Howe's 30-minute DST), so a 15-minute bucket never straddles a local hour or day boundary in any zone. Hourly UTC buckets would straddle them in every zone whose offset is not a whole number of hours. Re-bucketing a day is then a plain sum:

```sql
SELECT (bucket_start AT TIME ZONE $1)::date AS local_day, series_id, sum(n)
FROM events_15m
WHERE tenant_id = $2 AND bucket_start >= $3 AND bucket_start < $4        -- the window's instants
GROUP BY 1, 2;
```

- **Late data.** Buckets are keyed by *event* time. Closing a day (writing `daily_rollups`) happens after a grace period and is a **replace**, not an increment, so re-running it is harmless; late events within the recompute window re-trigger it. The seed does this by hand: its pull re-reads today and yesterday every run, and it keeps a day that had views and now comes back empty rather than writing a zero over it.
- **Day-close herd.** Tenants' midnights spread over the globe; the busiest hour closes ~10k days, each reading a few thousand rows: trivial. Jitter the starts ([27](27-deadlines-escalation.md) §9.5).
- **Zone change in June.** Two different things:
  - **Analytics** (`daily_rollups`): re-slice from the grid under the new zone, record the change in `tenant_zone_history`, tell the user. Days older than the 90-day grid keep the zone in their `tz` column; say so.
  - **Money documents** (`statements`): never re-sliced. They stored `tz`, `starts_at` and `ends_at` when closed; March's statement still means the March in the old zone.

### 9.4 Display and export

| Surface | Zone | Why |
|---|---|---|
| Aggregates and charts | the tenant's zone, **named in the UI** ("Eastern Time (New York)") | everyone in the company sees the same "March" |
| An individual timestamp in a feed | the viewer's zone, with the exact instant and zone abbreviation on hover | "when did *I* do this" |
| API | instants as RFC 3339 `…Z`; dates as `YYYY-MM-DD`; the zone as a separate IANA field | machines should not parse "EDT" |
| CSV | `local_time`, `utc_time`, `tz`, `utc_offset` as separate columns | the hour 01:30 happens twice on the fall-back night: a local time alone is ambiguous |

The seed labels its zone next to time fields (`ZoneLabel`) and shows `3:42:07 PM EDT` on hover (`exactTimeFull`). One inconsistency to avoid at scale: it has two helpers for a zone-less date-time field: the dashboard's `fromLocalInput`, which reads it in the **browser's** zone, and the backend's `ParseLocal`, which reads it as the **business** zone; they agree only while the editor's browser is in the business zone. Pick **one authority per field** and label it. CSV rules: ISO 8601, never a locale format or an Excel serial; remember spreadsheets have no zone concept, so give them text columns.

### 9.5 Testing, and the two tzdatas

- **Inject the clock** (`now func() time.Time`, as the seed's fakes do): no test calls `time.Now()`.
- **Golden dates, several zones:** New York 2026-03-08 (23 h) and 2026-11-01 (25 h); Berlin 2026-03-29 and 2026-10-25; a southern-hemisphere zone (DST reversed); Lord Howe (30 minutes); Kathmandu (:45); a zone whose switch is at midnight. Check the **offset gap**: for three weeks in March, New York and London differ by 4 hours, not 5 (US DST starts March 8, EU's March 29).
- **A property test:** for every zone and every day of two years, `end(d) == start(d+1)` (the windows tile), the lengths sum to the elapsed real time, and `Σ daily == Σ grid buckets` for random events.
- **Ship the zone data.** In Go, `import _ "time/tzdata"` embeds it (about 450 KB), so a minimal image works and behaviour no longer depends on the host: the seed does this. The catch: the embedded copy updates when you rebuild with a newer Go.
- **Two databases can disagree.** Go's tzdata and PostgreSQL's (built in, or the OS's) are separate copies that update separately; for the days around a rule change they give different answers and your SQL buckets stop matching your Go windows. That is the argument for computing windows in **one** place and passing instants to SQL, plus a monitored check that both agree on a handful of canary dates.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10× (50 B events/day):** the grid's write path before anything zone-related: ~580k events/s on average and ~1.7 M/s at peak, which pre-aggregation in memory per node turns into ~22k grid rows/s on average (1.92 B rows a day, 10 × 192 M) ([47](47-web-analytics.md) §9.1); partitions by day make retention a drop.
- **A tzdata update mid-quarter:** recompute `starts_at` for future `scheduled_items` and re-close open days in affected zones; alert if Go and Postgres canary dates disagree.
- **Late events after a day is closed:** within the window they trigger a replace; after it, they land in an "adjustments" counter shown on the report ("12 late events not in the closed days"), never silently altering a statement.
- **A tenant's zone change:** re-slicing 90 days × 96 × 20 rows is ~170k rows: instant. Do it as a job, keep the old rows until the new ones commit, then swap.
- **Hot tenants:** one tenant with 5% of events is a single partition-key range; sub-partition by `series_id` hash if its day-close slows.
- **Invalid zone ids:** the FK to `zones` rejects them; unknown ids from imports map through an alias table (`US/Eastern` → `America/New_York`), loaded from tzdata's links.
- **Single points of failure:** none specific; the grid is the replay source for every derived row.
- **Silent data errors are the real failure mode:** a day that is wrong by an hour looks plausible. The tiling property test, `day_minutes` in the response and the reconcile job (`Σ daily == Σ grid`) are how you notice.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| IANA zone ids; local time + zone for future events | offsets or stored instants | correct across rule changes | deriving the instant (and recomputing it) |
| half-open `[start, next)` windows from one function | "24 hours" or SQL-side day math | tiling days, one definition | SQL does no day arithmetic of its own |
| 15-minute UTC grid as the source | local-day rows at ingest | any zone, zone changes, hourly local charts | ~4× the rows of hourly, a close job |
| replace-style day close | increments | idempotent recompute, late-data friendly | a recompute window to manage |
| statements freeze their zone | re-slicing everything | money documents never change | two behaviours to explain |
| UTC instants + IANA id in APIs and CSV | local strings | unambiguous across DST | clients format for display |

## 12. Check yourself

1. Why is a booking's start a `date`, not a `timestamptz` at midnight?
2. How long is a New York day on 2026-03-08, and how do you get its two boundary instants in SQL?
3. Why a 15-minute grid and not hourly UTC buckets?
4. A tenant changes zone in June. What happens to March's chart, and to March's statement?
5. A meeting is created in February for 09:00 on 2027-03-28 in Berlin. What do you store, and why not the UTC instant?

<details><summary>Answers</summary>

1. A booking starts on a calendar day wherever it is viewed. Midnight in one zone is the previous evening in another, so a `timestamptz` shifts the date for some viewers; a `date` is the same everywhere. (JavaScript's date-only parsing as UTC is the same trap on the client.)
2. 23 hours: clocks jump 02:00 → 03:00. Build the local-midnight `timestamp`s and convert with the zone: `(date '2026-03-08')::timestamp AT TIME ZONE 'America/New_York'` is 05:00Z and `((date '2026-03-08') + 1)::timestamp AT TIME ZONE …` is 04:00Z the next day; use `>= start AND < next_start`.
3. Every UTC offset in use is a multiple of 15 minutes, so a 15-minute bucket never straddles a local hour or day boundary. Hourly UTC buckets straddle them for +5:30, +5:45 and similar zones, so those tenants' hours and days could not be rebuilt from them.
4. The chart can be re-sliced from the 15-minute grid under the new zone (with a notice; days older than the grid keep their old `tz`). The statement is a money document that stored its `tz`, `starts_at` and `ends_at` at close, so it is not re-cut.
5. `local_start = 2027-03-28 09:00` and `tz = Europe/Berlin`; derive `starts_at` (07:00Z, since DST starts that day) as a cache. A stored instant is only as right as the rules known when it was computed; if the rules change, the local fields still give the right answer after a tzdata update.

</details>

## 13. In Go

- Related lessons: [19 Stdlib tour](../../19-stdlib-tour.md) (`time`: `LoadLocation`, `Date`, `In`, `AddDate`), [18 Testing](../../18-testing.md) and [40 Testing architecture](../../40-testing-architecture.md) (injected clocks, table tests, property tests), [55 Data pipelines](../../55-data-pipelines.md) (bucketing and replace-style rollups), [59 Production DB patterns](../../59-production-database-patterns.md) (partitions, upserts), [62 Deployment & operations](../../62-deployment-operations.md) (tzdata in minimal images).
- `import _ "time/tzdata"` in the `main` package of anything that runs in a scratch or distroless image; `time.LoadLocation` once per zone and cached (a `sync.Map` keyed by name).
- Keep the date type small: a `Date` struct or a `YYYY-MM-DD` string with `AddDays` on UTC midnights, as the seed's `platform/dates` does; do not pass `time.Time` where a calendar date is meant.
- Tests: a table of `{zone, date, wantStartUTC, wantMinutes}` for the golden days; `for` over every day of two years asserting `end == nextStart`; run them with `-race` and with `TZ` set to something odd to prove nothing reads the host zone.
- Packages: `time`, `time/tzdata`, `pgx`, and optionally `github.com/rickar/cal` or a similar library for holiday calendars (extension).

## 14. Extensions

- **Day-start offset:** a tenant whose "day" runs 04:00 to 04:00; the window function takes an offset and the DST rules still apply to the *shifted* boundary.
- **Business days and holidays:** per-tenant calendars; SLA clocks that pause outside working hours ([27](27-deadlines-escalation.md)).
- **Weeks, ISO weeks and fiscal months:** week starts by locale; a fiscal calendar table instead of date math.
- **Viewer-zone drill-down:** a report in a zone other than the tenant's, served from the grid; the cost of the 15-minute rows vs a cache of the top zones.
- **Live tzdata updates:** deploy new zone data without a rebuild, and a shadow comparison of Go vs Postgres windows on canary dates.
- **Multi-zone tenants:** a chain of stores in five zones that wants "yesterday, per store, in the store's zone" and one company total; what is the company's day?
- **Historical zone correctness:** tenants who migrated from another system with local-time-only timestamps; reconstructing instants from ambiguous local times (the fall-back hour).
