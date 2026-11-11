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
