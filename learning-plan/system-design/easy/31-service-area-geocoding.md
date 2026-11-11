# 31 · Service Area & Geocoding

`🟢 easy` · *Topics:* geocoding cache and its cost, address normalisation, service areas as postal lists vs polygons vs precomputed cells, haversine vs road distance, autocomplete without a call per keystroke, keeping a paid provider behind an interface · *Builds on:* [05 Cache](05-kv-cache-ttl.md) (TTL, hot keys), [03 Rate Limiter](03-rate-limiter.md) (protecting a paid endpoint), [12 Notification Service](12-notification-service.md) (provider adapters), [29 Price & Availability Engine](29-price-availability-engine.md) (delivery pricing consumes distance)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [30 Invoicing: Money, Tax & PDF](30-invoicing.md) · Next: [32 Block-Based CMS](32-block-cms.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN step 7f (migrations 0026 areas admin, 0027 places, 0072 request places) and step 36. *Areas* are editable towns with `postal_codes text[]`, county, state, a tax place; an address is matched to a town **by postal code, then by locality and state**; no match offers "Add this town". It has **no polygons and no PostGIS**. Addresses go through a `platform/geo` interface (`Resolve(placeID)`, `Route(from, to)`) with a `google` driver and a `fake` one, and a layering test lets only `config` import a driver. By the project's reading of its provider's terms a **place id may be stored for ever, coordinates and distances only cached for 30 days**, so `geo_places` and `geo_routes` are cache tables with `fetched_at` and a daily sweep. The API **never calls the provider in a request**: saving an address enqueues `geo.route`, a worker measures *typical* driving time from the office, the page says "working it out…". Two keys: a browser key (Places only, restricted to the site's origins) and a server key (restricted to the server's IP, in the worker only). Autocomplete exists in the dashboard only (typing without picking saves plain text flagged *not checked*); the public forms take plain text, because it "would be paid per visitor, bots included" (step 36). `cmd/geocheck` confirms the driver against the real provider, since it was written from documentation.
> **Here, at scale:** a delivery marketplace: 100M address lookups a day, 5,000 merchant service areas, and a provider that bills per call.

## 1. The task

> "Design a location service for a delivery marketplace. Given an address, tell us whether each
> merchant serves it and how far away it is. Customers type addresses with autocomplete. The
> geocoding provider charges per call."

The system design here is mostly about **not calling the provider**, and about which cheap, local
answer is good enough for each question.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What does "serves this address" mean: postal code, radius or exact boundary? | Decides the data and the algorithm. | Merchants choose: postal-code list or a drawn polygon. |
| Distance: straight line or by road? | Cost differs by orders of magnitude. | Straight line to rank and filter; road distance only for a quote's shortlist. |
| Do customers type or pick? | Autocomplete is the biggest bill. | Pick from suggestions; free text is allowed but flagged "not verified". |
| May we store provider results? | Licence terms bound the cache. | Store place ids and our own confirmed text for ever; cache coordinates 30 days. |
| Freshness of boundaries? | Merchants edit areas. | Edits live within seconds. |
| Scale? | Cache and compute sizing. | 100M lookups/day, ~6k/s peak, 5k areas. |
| Countries? | Address formats, postal systems. | Several; normalisation is per country. |
| Provider outage or quota hit? | Must not stop checkout. | Degrade to postal-code matching and cached results. |
| Quote latency? | Where road distance may sit. | Under 300 ms; road distance may arrive a moment later. |

## 3. Requirements

**Functional**
- Resolve an address (typed or a picked place) to a normalised place with coordinates and components.
- For a place, return which merchants' areas cover it; `covered: true/false` for one merchant.
- Straight-line distance to any merchant; road distance and time for a shortlist.
- Autocomplete suggestions; merchants edit areas (postal list or polygon).

**Non-functional**
- p99 under 50 ms for coverage checks on cached places; no provider call on the hot path when avoidable.
- Provider spend capped by design: quotas, per-key restrictions, a daily budget alarm.
- Provider outage degrades quality, never availability.

**Out of scope**
- Turn-by-turn routing, live traffic, map tiles, address verification for postal delivery.

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Lookups | ~1.2k/s average, **~6k/s peak** | 100M ÷ 86,400 = 1,157; peak ~5× |
| Provider calls, no cache | 100M/day | every lookup misses |
| ... with 70% cache hits | 30M/day | |
| ... with 95% cache hits | 5M/day | |
| Cost at an *assumed* $5 per 1,000 calls | **$500k / $150k / $25k per day** | 100M, 30M, 5M × $0.005 |
| Distinct addresses in 30 days | ~20M (assumption) | the cache working set |
| Cache size | ~6 GB | 20M × ~300 B (key hash, formatted text, lat/lng, components) |
| Polygon area per merchant | ~500 km² (assumption) | |
| Precomputed H3 cells at resolution 9 (cell ≈ 0.1 km²) | **~24M rows ≈ 0.6 GB** | 500 ÷ 0.105 ≈ 4,760 cells × 5,000 areas × ~24 B |
| ... at resolution 8 (≈ 0.74 km²) | ~3.4M rows | 500 ÷ 0.737 ≈ 680 × 5,000 |
| Point-in-polygon, indexed | ~6k/s × ~0.5 ms ≈ **3 cores** | assumption for simple polygons |
| Quotes needing road distance | 2M/day × 3 shortlisted merchants = 6M route calls/day before caching | assumption |

What the numbers tell us: **the provider bill, not CPU, is the cost.** Going from 70% to 95% cache hits
saves $125k a day in this model. Containment checks are cheap: ~3 cores for polygons, so start with
PostGIS on a replica and precompute cells only when removing the database from the path.

## 5. Plan

1. Model: places (cache), lookups (address → place), areas (lists and polygons).
2. Resolve an address cheaply: normalise, cache, then the provider.
3. Decide coverage: postal list, polygon, or cell set.
4. Distance in two stages: straight line to rank, road to quote.
5. Autocomplete without a bill per keystroke.
6. Keep the provider behind an interface; budgets and failover.

## 6. High-level design

```
 browser ──► LB ──► Location API ──► app-local cache ──► Redis (lookup:{addr_hash})
   autocomplete        │                                        │ miss
   (debounced,         ├──► Postgres + PostGIS (replicas): places, lookups, service_areas, area_cells
    session token)     │
                       ├──► Geo driver interface ──► provider A  (circuit breaker, quota, budget alarm)
                       │                         └──► provider B  (failover)       fake (dev, CI)
                       └──► queue ──► route worker (road distance, cached) ──► geo_routes
 sweeper: expire cached coordinates at 30 days; refresh what open orders still use
```

**An address lookup:** normalise → look in the local cache → Redis → Postgres → only then the provider
(guarded by `singleflight`), store the result, then answer coverage from our own data. **A distance:**
straight line is computed inline; road distance is queued, cached by `(origin place, destination place)`,
and shown as "working it out…" until it arrives, exactly as honeydolist does.

## 7. API

```
POST /v1/lookups   { address | place_id, merchant_id? }
                   → 200 { place: { id, formatted, lat, lng, components }, verified: true, covered_by: [area_id…] }
                   → 200 { verified: false }                         free text the provider could not resolve
GET  /v1/areas/check?merchant=42&postal_code=13118&state=NY   → 200 { covered: true, area_id }   no provider call
GET  /v1/autocomplete?q=12+Main&session=…&country=US          → 200 { suggestions: [{ place_id, text }] }   rate-limited
GET  /v1/distance?from=PLACE&to=PLACE&mode=straight|road      → 200 { km } | 202 { retry_after_s }   road is async
PUT  /admin/areas/{id}   { postal_codes?, boundary_geojson?, published }                     → 200 (cells rebuilt async)
```

`POST /v1/lookups` with a `place_id` (the customer picked a suggestion) never needs geocoding, only a
details fetch, which the cache usually has. honeydolist's `POST /admin/places/resolve` plays the same
role: it answers "which town, which tax place" *before* the form is saved.

## 8. Data model

```sql
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE geo_places (                      -- the provider's answer: a CACHE, rows age out, nothing points here
    place_id    TEXT PRIMARY KEY,
    provider    TEXT NOT NULL,
    formatted   TEXT NOT NULL,
    lat         NUMERIC(9,6) NOT NULL,
    lng         NUMERIC(9,6) NOT NULL,
    components  JSONB NOT NULL,                -- locality, county, state, postal_code, country
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX geo_places_age_idx ON geo_places (fetched_at);          -- the 30-day sweep

CREATE TABLE geo_lookups (                     -- normalised typed address -> place (also "not found")
    addr_key   BYTEA PRIMARY KEY,              -- hash(country | normalised address | provider)
    place_id   TEXT,                           -- NULL = the provider found nothing
    expires_at TIMESTAMPTZ NOT NULL            -- 30 days (found) or 1 day (not found)
);

CREATE TABLE geo_routes (
    origin_place_id TEXT NOT NULL, dest_place_id TEXT NOT NULL,
    distance_m INTEGER NOT NULL, duration_s INTEGER NOT NULL,         -- typical, not live traffic
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (origin_place_id, dest_place_id)
);

CREATE TABLE service_areas (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id  BIGINT NOT NULL,
    name         TEXT NOT NULL,
    postal_codes TEXT[],                       -- the cheap rule (what honeydolist has)
    boundary     geometry(MultiPolygon, 4326), -- the exact rule, optional
    is_published BOOLEAN NOT NULL DEFAULT false,
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX service_areas_boundary_gix ON service_areas USING gist (boundary)
    WHERE is_published AND deleted_at IS NULL;
CREATE INDEX service_areas_postal_gin ON service_areas USING gin (postal_codes)
    WHERE is_published AND deleted_at IS NULL;

CREATE TABLE area_cells (                      -- derived from boundary; rebuilt when it changes
    res SMALLINT NOT NULL, cell BIGINT NOT NULL, area_id BIGINT NOT NULL, on_edge BOOLEAN NOT NULL,
    PRIMARY KEY (res, cell, area_id)
);
```

A record that needs a place keeps `place_id` as **plain text with no foreign key**, plus the address
text a person confirmed (honeydolist's rule): the cache can expire without orphaning anything.

## 9. Deep dives

### 9.1 Geocoding cache: normalise, key, TTL, cost

| Option | Pros | Cons |
|---|---|---|
| Call the provider for every lookup | always fresh | $500k/day in this model; rate limits; outage = outage |
| **Normalised-address cache, 30-day TTL** | big hit rate on repeat addresses; compliant with the terms we assumed | a stale answer after a rename or new street |
| Own address database (open data) | no per-call fee, no TTL limits | data quality and maintenance, per country |

**Choice and why: cache, with normalisation first.** The key is `hash(country | normalised | provider)`.
Normalise: trim, case-fold, collapse whitespace, expand/contract abbreviations consistently
(`Street → ST`), standardise unit markers (`Apt 4B`, `#4B`, `Unit 4B` → one form), strip punctuation,
carry the postal code if present. Every missing rule costs hit rate, and each point of hit rate is worth
about $5k a day here. Cache **misses that found nothing** for a day (`place_id NULL`), or one bot typing
garbage repeats the paid call forever. Wrap the provider call in `singleflight` so a hot address misses
once, not 500 times. The **TTL is a licence decision, not a tuning knob**: coordinates 30 days, the place
id and the person-confirmed text for ever, exactly honeydolist's reading of its terms. A daily sweep
refreshes what open orders still reference and deletes the rest.

### 9.2 A service area as data: lists, polygons, cells

| Option | Pros | Cons |
|---|---|---|
| **Postal-code / town lists** (honeydolist) | trivial; a merchant can edit it; O(1) hash or GIN lookup; no coordinates needed | a postal code is not a delivery boundary; coarse at the edges |
| **Polygons + PostGIS GiST** | exact; merchants draw what they serve | needs coordinates; complex shapes cost more; GeoJSON validation |
| **Precomputed cells (H3/S2/geohash) → area ids** | O(1) lookup, no geometry at query time, easy to cache and shard | cells approximate the boundary; storage; rebuild when a polygon changes |

**Choice and why: lists first, polygons when merchants ask, cells only to remove the database.** The
query is one indexed statement (note **longitude first** in `ST_MakePoint`):

```sql
SELECT id FROM service_areas
WHERE is_published AND deleted_at IS NULL
  AND ST_Covers(boundary, ST_SetSRID(ST_MakePoint($1, $2), 4326));   -- $1 = lng, $2 = lat
```

~6k/s at ~0.5 ms is about 3 cores (§4), so it needs no cleverness. If it ever must run in the app's
memory, cover each polygon with H3 cells at resolution 9, store `(cell → area_ids)` (24M rows ≈ 0.6 GB),
and answer by computing the point's cell. Mark **edge cells** (`on_edge`): a hit on an edge cell falls
back to the exact `ST_Covers`, so approximation never wrongly accepts or rejects. Match order, as
honeydolist does with towns: **polygon, then postal code, then locality + state**, and when nothing
matches say so ("not in your areas") instead of guessing. Merchant edits write the area then rebuild its
cells asynchronously; the exact test stays correct meanwhile.

### 9.3 Distance: straight line to rank, road to quote

| Option | Pros | Cons |
|---|---|---|
| **Haversine** (great-circle) | microseconds, free, offline | ignores roads; roads are often ~1.2–1.4× longer (rule of thumb) |
| **Road distance/time API** | what the driver drives | billed per pair, rate-limited, traffic-dependent |
| Precompute a matrix | instant | N × M pairs explode (5k merchants × millions of addresses) |

**Choice and why: two stages.** Rank and filter with straight lines (a GiST k-nearest query
`ORDER BY merchant.location <-> point LIMIT k` on a GiST-indexed point, or a radius filter), then ask the road API for **only the
shortlist** (3 merchants), **asynchronously**, and cache by `(origin place, destination place)` for 30
days. honeydolist's rules carry over: the API never calls the provider inside a request, asks for
*typical* time not live traffic ("a job next week does not care about this afternoon's"), and shows
"working it out…" until the worker finishes; changing the origin recomputes open orders only.

```go
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0088 // mean Earth radius, km
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
```

### 9.4 Autocomplete without a call per keystroke

Typing a 25-character address can make ~20 provider calls. At 20M entries a day that is the bill.

| Technique | Effect |
|---|---|
| Minimum 3 characters, debounce 250–300 ms, cancel the in-flight request | ~20 calls per entry → ~4 |
| **Session pricing** (a session token groups suggestions plus the final pick) | the whole entry bills as ~1 unit, where your provider offers it |
| Server-side cache of popular prefixes (short TTL) per country/region | repeat prefixes cost nothing |
| Bias by the user's location | fewer, better suggestions |
| **Proxy through our API** with a rate limit and bot checks ([03](03-rate-limiter.md)) | a public autocomplete is a free paid-API for bots |
| Offer free text with "not verified" | the form never depends on the provider |

honeydolist's decisions are the cautious version of all of these: autocomplete only in the dashboard
(staff, few calls), the public forms take plain text (the dashboard flags an unpicked address *not checked*), and the two keys are
split: the **browser key** (public by nature, restricted to our origins, Places only) and the **server
key** (restricted to the server's IP, living in the worker only). Add a daily quota per API and a budget
alert, "so a bug or a leaked key cannot run up a bill".

### 9.5 Keep the provider behind an interface

honeydolist's `platform/geo` has two methods, `Resolve(placeID)` and `Route(from, to)`, a real driver and a
`fake`, and an architecture test that only `config` may import a driver. Consequences you want at scale:

- **Failover and cost routing:** a second driver; a circuit breaker ([36 Resilience](../../36-resilience-patterns.md)) flips to it, or
  to postal-code matching and cached places, when the first errors or hits its quota.
- **Dev, CI and load tests run on `fake`** and never spend money.
- **Contract check the real driver once.** honeydolist's `cmd/geocheck` runs the driver against the
  real provider with a real key, because "the driver was written from the documentation; nothing here
  could reach the provider". A fake proves your code, not your assumptions about the vendor.
- **Without any key the product still works**: plain text addresses, no map, blank distances.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** the **bill** (1B lookups/day), then the cache working set (~60 GB if
  distinct addresses scale with traffic: shard Redis by `addr_key`), then provider rate limits. Build the
  own-address-database path before the bill forces it.
- **Cache stampede on a viral address** (a stadium on match day): `singleflight` plus a short-lived
  in-process cache, jittered TTLs.
- **Provider down or quota exhausted:** serve cached places, fall back to postal-code coverage, mark new
  addresses `verified: false`, queue them for geocoding later. Checkout continues.
- **Leaked browser key:** its restrictions (origins, API list) and the daily quota bound the damage;
  rotate it. A leaked server key is why it lives in the worker's environment only.
- **Boundary edits:** write the polygon, answer exactly from `ST_Covers` immediately, rebuild cells in the
  background; never serve stale cells without the on-edge fallback.
- **Bad normalisation after a rule change:** old keys simply miss; bump a `norm_version` in the key
  instead of purging.

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| normalised-address cache, 30-day TTL | calling the provider every time | ~$100k+/day saved at high hit rates | stale answers; licence-driven TTL |
| negative caching (1 day) | caching only hits | bots cannot loop paid calls | a new address is "not found" for a day |
| postal lists first, polygons next | polygons only | simplicity, merchant-editable | coarse edges until a polygon is drawn |
| exact `ST_Covers`, cells optional | cells everywhere | exactness; 3 cores is cheap | database in the path |
| straight line to rank, road for a shortlist | road distance for all | ~10–100× fewer paid calls | approximate ranking |
| async road distance, "working it out…" | inline calls | fast quotes, no provider stalls | a second state in the UI |
| provider behind an interface | direct SDK calls | failover, `fake` in CI, cost routing | an abstraction to maintain |

## 12. Check yourself

1. Why is the provider bill, not CPU, the real cost, and what are the three levers on it?
2. What goes in the cache key, why cache "not found", and why is the TTL not a tuning knob?
3. A merchant draws a polygon. How do you answer "is this address inside" exactly, and when would you add H3 cells?
4. Why rank by haversine and quote by road distance, and why is the road call asynchronous?
5. How do you stop a public autocomplete box from becoming an expensive open API?

<details><summary>Answers</summary>

1. 100M lookups a day at an assumed $5 per 1,000 is $500k/day, while 6k/s of polygon checks is ~3 cores.
   Levers: cache hit rate (normalisation, TTL), fewer calls per entry (debounce, sessions), and calling
   the paid road API only for a shortlist.
2. `hash(country | normalised address | provider)`, with a `norm_version` if rules change. Caching
   misses stops garbage or bots repeating paid calls. The TTL follows the licence: coordinates 30 days,
   place ids and person-confirmed text for ever.
3. `ST_Covers(boundary, point)` on a GiST-indexed geometry (longitude first). Add H3 cells at
   resolution 9 only to answer in memory without the database, with an on-edge fallback to the exact test.
4. Straight lines are free and microsecond-fast, good enough to filter and sort; roads cost per pair, so
   only the top few are measured. The call is async and cached so a slow or failing provider never
   blocks the request; the UI shows "working it out…".
5. Debounce and minimum length, session pricing, cached popular prefixes, proxy it through our API with
   rate limits and bot checks, restrict keys, set daily quotas and a budget alert, and allow free text
   as a fallback (honeydolist keeps public autocomplete off entirely).

</details>

## 13. In Go

- Related lessons: [38 Caching](../../38-caching-patterns.md) (cache-aside, `singleflight`, negative caching), [36 Resilience](../../36-resilience-patterns.md) (circuit breaker, failover), [32 Hexagonal](../../32-hexagonal-ports-adapters.md) (the `Geocoder` port and its drivers), [68 Rate limiting](../../68-rate-limiting-throttling.md) (the autocomplete proxy), [44 Background jobs](../../44-background-jobs-queues.md) (the route worker), [22 database/sql](../../22-database.md) and [59 Production DB patterns](../../59-production-database-patterns.md) (GiST/GIN indexes, no-FK cache tables), [18 Testing](../../18-testing.md) (the `fake` driver, an architecture test for imports).
- The port: honeydolist's two methods plus a search by typed address:

```go
type Geocoder interface {
	Resolve(ctx context.Context, placeID string) (Place, error)
	Search(ctx context.Context, normalised string) (Place, error)
	Route(ctx context.Context, from, to Place) (distanceM, durationS int, err error)
}
```

- Packages: `pgx`, `go-redis`, `golang.org/x/sync/singleflight`, `github.com/uber/h3-go` (cells), `github.com/paulmach/orb` (GeoJSON), `net/http` for the driver.

## 14. Extensions

- **Own address database** from open data for the top countries, with the provider as the fallback only.
- **Delivery fee tiers by distance** feeding [29](29-price-availability-engine.md): rings by haversine, exact fee from the road shortlist.
- **Dynamic areas:** drive-time isochrones instead of radii or postal lists.
- **Reverse geocoding** ("drop a pin") with the same cache, keyed by a rounded coordinate cell.
- **Tax place from the address:** the area/tax-jurisdiction link honeydolist stores on towns, as input to [30](30-invoicing.md).
- **Nearest-merchant search at 100k+/s:** shard cells by region, keep them in memory, rebuild from the database.
- **Abuse analytics:** per-key spend dashboards and anomaly alerts on lookups per IP.

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4 and 9.4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| Location API (Go, stateless; also proxies autocomplete) | 6k lookups/s (§4) + ~4.6k autocomplete calls/s (§9.4: 20M entries × ~4 calls ÷ 86,400 × 5) | CPU | lookups 6k × 0.3 ms = 1.8 vCPU; autocomplete 4.6k × 0.5 ms (half answered from the prefix cache, half proxied over TLS) = 2.3; road-distance worker 0.35 (below); total ≈ 4.5 vCPU. N = ceil(4.5 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(2.1) = 3 | `c7g.xlarge` (4 vCPU / 8 GiB) on ECS/EKS | 3, one per zone | JSON, TLS and a Redis call are all CPU; memory is ~1 GiB of heap plus a ~300 MB in-process cache of hot addresses, so `c` not `m`. 37 % busy at peak, 56 % with a zone down |
| Redis (address cache + autocomplete prefixes) | ~13k ops/s (6k lookup GETs + 4.6k prefix GETs + ~2.3k prefix SETs); 20M addresses × 300 B = 6 GB (§4) | memory | 6 GB × ~1.7 overhead = 10.2 GB; prefixes ≈ 1M × 1 KB × 1.7 = 1.7 GB; total ≈ 12 GB; `maxmemory` ≤ 70 % ⇒ ≥ 17 GiB of RAM; 13k ops/s is ~13 % of one core's ~100k | `cache.r7g.xlarge` (4 vCPU / 32 GiB), ElastiCache | 1 primary + 1 replica (other zone) | memory-bound, so `r` (8 GiB per vCPU); `r7g.large` has 16 GiB, just under the 17 the data needs; CPU idles, so a second shard only when memory or ops double |
| Postgres primary (places, lookups, routes, areas) | ~290 geocode results/s + ~350 route rows/s (5M and 6M rows a day × 5 ÷ 86,400); ≈ 27 GB | neither (sized for availability) | (290 × 2 rows + 350) × ~2 IOPS ≈ 1,900 IOPS < gp3's 3,000 included; size: 6 GB places + 2 GB lookups + 11 GB routes (6M × 30 d × 60 B, no repeats) + 0.6 GB cells, +35 % indexes | `db.m7g.xlarge` (4 vCPU / 16 GiB), RDS Multi-AZ | 1 + synchronous standby | writes are tiny, so no `i` family or `io2`; 16 GiB keeps the indexes warm; the standby is for the zone loss |
| Postgres read replicas (PostGIS `ST_Covers`, place and route reads) | `ST_Covers` 6k/s × ~0.5 ms = 3 vCPU (§4, simple polygons); Redis misses ≈ 300 reads/s | CPU | survivors ≤ 80 %: one replica alone is 3 ÷ 4 = 75 %, so 2 replicas; polygons (5,000 × ~100 KB = 0.5 GB) + GiST + hot places fit in 16 GiB | `db.m7g.xlarge` (4 vCPU / 16 GiB), RDS read replicas | 2, one per zone | geometry tests are CPU, so `m` (4 GiB per vCPU), not `r`; 38 % busy with both up, 75 % with one zone down; go to `2xlarge` if polygons cost more than 0.5 ms |

The road-distance worker (6M route calls/day ÷ 86,400 × 5 ≈ 350/s × ~1 ms, mostly waiting on the provider) is a second process on the API nodes, fed by SQS (6M messages/day, batched by 10). Provider traffic leaves through NAT: ~51M calls/day (5M geocode + 40M proxied autocomplete + 6M route) × ~3 KB ≈ 150 GB/day. S3, the CDN and GPUs: nothing to size.

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| API: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Redis: 2 × `cache.r7g.xlarge` (2 × ≈ $310) | $620 |
| Postgres primary: `db.m7g.xlarge` ≈ $240 managed, Multi-AZ doubles | $480 |
| 2 read replicas: `db.m7g.xlarge` (2 × $240) | $480 |
| gp3: 4 volumes × 100 GB (primary, standby, 2 replicas), IOPS inside the free 3,000 | $30 |
| NAT: 3 gateways ($99) + 4.6 TB processed × $0.045 | $310 |
| Data out to the provider: ~1 KB per call × 51M × 30 = 1.5 TB × $0.09 | $140 |
| ALB: $16 + ~11 LCU (2.1k req/s average × 1.5 KB ≈ 11 GB/h) × $0.008 × 730 | $80 |
| SQS: 1.8M requests/day × 30 × $0.40 per million | $20 |
| Geocoding provider: 5M calls/day (95 % cache hits) × $0.005 × 30 | $750,000 |
| **Total** | **≈ $752,500** (≈ $751,800 with a 1-year plan on the ≈ $1,900 compute baseline: a rounding error) |

- **Biggest line and why:** the provider, 99.7 % of the bill. All the AWS hardware is ≈ $2.5k a month, because §4 already says it: the cost is the paid call, not CPU. Each point of cache hit rate is 1M calls × $0.005 = $5k a day ≈ $150k a month; at 70 % hits the same line is $4.5M. Normalisation, debounce, sessions and a shortlist-only road API move the bill; a bigger instance does not. Route and autocomplete calls are extra (§4 gives no price or hit rate): each 1M uncached calls a day adds ≈ $150k a month at the same assumed $5 per 1,000.
- **What changes at 10×:** 1B lookups a day, 60k/s peak. API: 18 + 23 + 3.5 = 44.5 vCPU ⇒ N = ceil(44.5 ÷ 2.13) = 21 → 21 × `c7g.xlarge`. Redis: 60 GB × 1.7 = 102 GB ÷ 0.7 ⇒ ≈ 146 GiB ⇒ 3 shards of `r7g.2xlarge` (64 GiB), each with a replica. `ST_Covers` would be 30 vCPU, so move coverage into the API's memory (24M H3 cells ≈ 0.6 GB per node, §4) and leave the replicas only the place and route reads. The provider line becomes 50M calls/day = $250k a day ≈ $7.5M a month: the own-address-database path is the real project. NAT would process ≈ 46 TB (≈ $2k a month): run the API in public subnets with no inbound rules instead.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- Two kinds of data with opposite shapes. A **cache of the provider's answers** (`geo_places`, `geo_lookups`, `geo_routes`): point reads by key, ~20M addresses ≈ 6 GB, ~290 geocode rows/s and ~350 route rows/s written, rows that must age out at 30 days (section 4, section 15). And **authored data** (`service_areas`): 5,000 merchants' postal lists and polygons, ~0.5 GB of geometry, edited by people.
- The only non-trivial query is point-in-polygon, `ST_Covers(boundary, point)`, at ~6k/s × ~0.5 ms ≈ 3 cores (section 4), plus a postal-code lookup in a `text[]` and a nearest-merchant ranking.
- Everything is small: ~27 GB in Postgres (section 15), no write rate near one primary's ceiling. The money is in the provider's calls, not in the database, but that makes the cache tables **expensive to lose**: 20M distinct addresses × $0.005 (the assumed price) = $100k to re-geocode.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL + PostGIS (RDS / Aurora) | ✅ chosen | `geometry(MultiPolygon, 4326)` with a **partial GiST** index (`WHERE is_published AND deleted_at IS NULL`) answers `ST_Covers` in one indexed statement; a partial GIN index on `postal_codes text[]` serves the cheap rule; `jsonb` holds `components`; the `<->` nearest query uses the same GiST index (9.3); one engine, so a polygon edit and its row commit together |
| MySQL (RDS / Aurora) | ✔ works, with workarounds | spatial types and R-tree `SPATIAL` indexes exist, so point-in-polygon is possible, but: no partial index (the published filter runs after the index), a spatial index needs a `NOT NULL` column (the optional `boundary` moves to its own table), no `text[]`/GIN (postal codes become a child table or a JSON multi-valued index), `ST_Contains` instead of `ST_Covers` (points exactly on the edge differ), and SRID 4326 is latitude-first by default, the opposite of the longitude-first `ST_MakePoint` bug that 9.2 warns about |
| H3 cell sets (`area_cells`) in app memory or Redis | ✔ as a derived index, never the record | ~24M rows ≈ 0.6 GB at resolution 9: a point becomes a cell, a cell becomes area ids in microseconds, no geometry at query time. That removes the database from the path at 10×. It is rebuilt from `boundary`, and on-edge cells fall back to the exact test, so a boundary store is still needed. Portable: `(res, cell, area_id)` is an ordinary primary key in either engine |
| Redis (ElastiCache) | ✔ as the front cache, ✖ as the record | the address cache (~12 GB with prefixes, section 15) answers 6k lookups/s; but a flush or failover must not turn into $100k of provider calls, so the durable copy stays in Postgres (Redis → Postgres → provider, section 6) |
| DynamoDB | ✔ for the cache tables, ✖ for areas | `geo_places` and `geo_lookups` are pure key lookups of ~300 B; but its TTL deletes lazily (hours to days), so reads still check `expires_at` (the licence rule, 9.1); there is no polygon-containment query at all, and ~290 writes/s needs no scale-out store |

**Pick:** PostgreSQL with PostGIS on RDS Multi-AZ (`db.m7g.xlarge` primary and two `db.m7g.xlarge` read replicas for `ST_Covers`, section 15), with Redis in front. Deciding properties: PostGIS (geometry type, GiST, `ST_Covers` and `<->` in one engine), the partial indexes of section 8, and `text[]` + `jsonb` on the same rows. System of record: `service_areas` (authored). Derived and rebuildable: `area_cells` (from `boundary`), the Redis cache (from Postgres), and `geo_*` (re-buyable from the provider, but at a price, so backed up).

**If it were MySQL instead:** the same tables with a separate `service_area_polygons` table (`NOT NULL` geometry plus a `SPATIAL` index), a `service_area_postal_codes (area_id, postal_code)` child table with a plain index, and a published-flag check after the spatial lookup. It works at 3 cores of load; what you pay is code that must handle the axis order and edge-case semantics, and a test of the function you use on your exact version. If the H3 path is taken, the choice stops mattering because the hot query leaves the database.

**What would flip the choice:** containment above what the replicas can carry (~6k/s on 3 cores today; the lesson's own 10× answer is 60k/s) means H3 cells in the API's memory (24M rows ≈ 0.6 GB per node), not a different database; polygons that are huge or need unions and drive-time shapes keep you on PostGIS; the cache working set growing 10× (60 GB, section 10) means Redis shards first, and DynamoDB only when "no cache servers to run" matters more than the TTL quirk.

## 17. What breaks at 100×, 1,000× and 10,000×

> A favourite interview follow-up. The baseline is the peak in section 4; every row names the *first* thing to run out, the number that proves it, and the change. Method and typical breakpoints: [Scaling ladder cheat sheet](../README.md#scaling-ladder-cheat-sheet).

| Scale | Load (baseline × N) | What breaks first, and the number | Recommendation (and what you give up) | ≈ bill |
|---|---|---|---|---|
| **100×** | 600k lookups/s peak (10B a day), 460k autocomplete calls/s; ~450 vCPU of API; if distinct addresses scale with traffic, ~2B addresses ≈ 600 GB of cache | the provider line, not a machine: 5M × 100 = 500M calls/day × $0.005 = $2.5M/day ≈ $75M/month, 99.7 % of the bill (hardware ≈ $250k). Behind it: Redis 1.3M ops/s = 13 shards by CPU, 23 by memory; 93k geocode and route rows/s is ~9× one primary | make the vendor the fallback (§10, §14): an own address database for the top countries lifts hits from 95 % to ~99.5 % (50M calls/day ≈ $7.5M/month), plus a committed-volume contract for the rest. Same design out: ~54 `c7g.4xlarge`; ~23 Redis shards by `addr_key`; coverage from H3 cells in node memory (as §15); `geo_*` cache tables to DynamoDB (as §16). Give up: own-data quality per country, a volume commitment, lazy TTL deletes | ≈ $75M/month as designed; ≈ $8M after own data (≈ $7.5M provider + ≈ $0.25M AWS) plus the own-data cluster |
| **1,000×** | 6M lookups/s peak (100B a day: ~12 per person on Earth, a thought experiment), 4.6M autocomplete calls/s; ~4,500 vCPU of API; 13M Redis ops/s | the vendor relationship and one big stack: even at 99.5 % own data, 500M calls/day ≈ $75M/month (5B a day at 95 % would be $25M a day). Separately 13M ops/s ≈ 130 Redis shards and ~530 API nodes under one control plane and one blast radius | stop being a metered customer: license address data in bulk (or run an own open-data geocoder and prefix index for autocomplete) so the provider covers only a ~0.05 % tail (50M calls/day ≈ $7.5M/month). Shape: ~25 independent cells by country group (`country` is already in the cache key), each with its own geocoder, Redis, H3 area shard and ~24 `c7g.4xlarge`, behind a `country → cell` router. Give up: cross-border queries, one global cache, per-country data licences to manage | ≈ $10M/month: AWS ≈ $2.5M (linear, traffic stays inside a cell) + $7.5M tail + the data licence (not priced); $750M/month at list price and 95 % hits |
| **10,000×** | 60M lookups/s peak (1T a day: ~125 per person on Earth, beyond the real world); ~45,000 vCPU ≈ 5,300 API nodes, so a node fails about every hour | money and meaning: at list price 50B provider calls/day = $250M/day ≈ $7.5B/month, plus ≈ $25M of AWS, to answer a question the device mostly knows already (where am I, which cell) | change the problem: do not look up per view. The client takes its own GPS fix, computes its H3 cell (res 8–9) and checks coverage against a CDN-cached, versioned tile of `cell → area ids`; only `on_edge` cells call the server for the exact `ST_Covers`. A typed address is geocoded once, when saved to the account; autocomplete needs a signed-in user and a daily quota. Give up: instant boundary edits (tile TTL of minutes), exactness on edge cells until checkout, coverage for free text while browsing | ≈ $7.5B/month if nothing changes; with 100× fewer server lookups (assumption) it returns to the 100× row, ≈ $8M |

**What stays the same at every scale:** the `Geocoder` interface with its `fake`, and the cache key `hash(country | normalised | provider)` with `norm_version`; the TTL as a licence decision, not a tuning knob; exact `ST_Covers` as the truth, with H3 cells only as a derived index that falls back on `on_edge`; and the rule that no request waits on the provider.

**Say it in an interview:** "At 100× the problem is the provider's bill, $75M a month at 5M calls a day times 100, so I make it the fallback behind an own address database and scale the rest out: Redis shards by address key, H3 cells in node memory, cache tables in DynamoDB. At 1,000× I stop being a metered customer: I license data and split the world into country cells, each with its own geocoder and cache. At 10,000× there are more lookups than addresses worth looking up, so the device computes its own cell and checks a cached tile; the server geocodes only when an address is saved and checks exactly only on edge cells."
