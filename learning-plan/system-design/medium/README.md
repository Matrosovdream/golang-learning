# 🟡 Medium — backlog

Not written yet. Candidates, roughly in the order they build on the easy tasks:

| # | Task | Why medium |
|---|---|---|
| 01 | Distributed cache (consistent hashing, replication, hot keys) | extends easy 05 to many nodes |
| 02 | Typeahead / autocomplete | trie + top-K + offline build + sharding by prefix |
| 03 | Chat (1:1 and group) | WebSocket fan-out, presence, message ordering, offline delivery |
| 04 | News feed | fan-out on write vs read, celebrity problem, ranking, pagination |
| 05 | Web crawler | frontier, politeness, dedup (bloom filters), distributed workers |
| 06 | Ticket booking / seat reservation | contention, reservations with TTL, double-booking, payments |
| 07 | Flash sale / inventory service | extends easy 13: hot SKU, queues as admission control, overselling |
| 08 | Search for a product catalogue | inverted index, Elasticsearch, indexing pipeline, relevance |
| 09 | Logging & metrics pipeline | agents → Kafka → storage, cardinality, retention, alerting |
| 10 | Payment service | idempotency, ledger, reconciliation, PSP webhooks (see blockchain track P7) |
| 11 | Ride matching / nearby search | geohash / quadtrees, moving objects, matching |
| 12 | Video upload & streaming | extends easy 11: transcoding pipeline, adaptive bitrate, CDN |
| 13 | Distributed task queue | extends easy 15: partitions, visibility timeouts, exactly-once-ish |
| 14 | Multi-tenant SaaS data layer | isolation models, noisy neighbours, per-tenant limits |
| 15 | Email service (inbox) | storage, search, spam pipeline, IMAP-ish access |

Pick one and it gets written as `medium/NN-slug.md` following [`../_template.md`](../_template.md).
