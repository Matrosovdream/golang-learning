# System Design Track · Progress

Method, toolbox and index: [README.md](README.md). One task at a time; tick when you've gone
through the task, questions, plan, solution, pros/cons and answered the check questions.

> ▶ **Resume here:** 🟢 **easy** — task **10. Webhook Delivery** ([easy/10-webhook-delivery.md](easy/10-webhook-delivery.md)).

### 🟢 easy — [easy/](easy/)
- [x] 01. URL Shortener
- [x] 02. Pastebin
- [x] 03. Rate Limiter
- [x] 04. Unique ID Generator
- [x] 05. In-Memory Cache with TTL
- [x] 06. View & Like Counter
- [x] 07. Leaderboard
- [x] 08. Uptime Monitor
- [x] 09. Feature Flags
- [~] 10. Webhook Delivery
- [ ] 11. Image Upload & Hosting
- [ ] 12. Notification Service
- [ ] 13. Shopping Cart
- [ ] 14. Polls & Voting
- [ ] 15. Job Scheduler
<!-- 16–60: seeded from honeydolist-site, designed for scale -->
<!-- intake & abuse -->
- [ ] 16. Public Form Intake
- [ ] 17. Abuse & Spam Protection
- [ ] 18. Inbound Webhook Receiver
- [ ] 19. Moderated Submissions
<!-- records & integrity -->
- [ ] 20. Per-Tenant Document Numbering
- [ ] 21. Presence & Soft Edit Locks
- [ ] 22. Audit Log & Revision History
- [ ] 23. Paginated Lists at a Billion Rows
<!-- lifecycle & workflow -->
- [ ] 24. Data Lifecycle: Archive, Delete, Erase
- [ ] 25. Contact Deduplication & Merge
- [ ] 26. Configurable Status Workflow
- [ ] 27. Deadlines & Escalation
<!-- booking & commerce -->
- [ ] 28. Booking Calendar for Rentable Units
- [ ] 29. Price & Availability Engine
- [ ] 30. Invoicing: Money, Tax & PDF
- [ ] 31. Service Area & Geocoding
<!-- content & SEO -->
- [ ] 32. Block-Based CMS
- [ ] 33. Sitemaps & Crawl Notifications
- [ ] 34. Crawler-Ready SPA Serving
- [ ] 35. Redirect Manager & 404 Tracker
<!-- AI & search -->
- [ ] 36. LLM Gateway
- [ ] 37. In-App Records Assistant
- [ ] 38. AI Draft Queue
- [ ] 39. Global Search with Permission Filtering
<!-- platform & operations -->
- [ ] 40. Media Reprocessing at Scale
- [ ] 41. Backup & Restore
- [ ] 42. Integration Credentials Vault
- [ ] 43. Zero-Downtime Deploys & Schema Migrations
<!-- comms, access & analytics -->
- [ ] 44. Bulk Email Campaign Sender
- [ ] 45. Live In-App Notifications
- [ ] 46. Roles & Permissions (RBAC)
- [ ] 47. Privacy-Friendly Web Analytics
<!-- tooling & settings -->
- [ ] 48. Bulk Import with Preview & Undo
- [ ] 49. Load Testing Harness
- [ ] 50. Alerting Engine
- [ ] 51. Runtime Settings & Timed Kill Switches
<!-- forms, push & landing pages -->
- [ ] 52. Configurable Forms
- [ ] 53. Web Push & the Installable App (PWA)
- [ ] 54. Programmatic Location Pages
<!-- timelines, links & events -->
- [ ] 55. Order Timeline with Photo Steps
- [ ] 56. Related Content & Interlinking
- [ ] 57. Event Registration with Capacity & Waitlist
<!-- tracking, adapters & time -->
- [ ] 58. Outbound Link Click Tracking
- [ ] 59. Provider Adapters with Fake Drivers & Chaos Testing
- [ ] 60. Business-Day Time Zones & DST in Reports

### 🟡 medium — [medium/](medium/)
_No tasks written yet. Backlog: [medium/README.md](medium/README.md)._

### 🔴 hard — [hard/](hard/)
_No tasks written yet. Backlog: [hard/README.md](hard/README.md)._

**Status legend:** `[ ]` not started · `[~]` in progress · `[x]` done

## Log

- 2026-10-13 — track created: README (method, toolbox, estimation cheat sheet), `_template.md`, and the first **15 easy tasks** (01–15). Medium/hard folders hold backlogs only.
- 2026-10-23 — easy **01 URL Shortener** done (side question: why not UUIDv7 as the short code — 128 bits = 22 base62 chars; truncating collides). Started 02.
- 2026-10-26 — easy **02 Pastebin** done (side questions: streaming vs presigned S3 URL, gzip at rest, `ON CONFLICT (id) DO NOTHING` collision retry). Open gap found: blob key `pastes/<id>` is written before the row, so an ID collision would overwrite another paste's blob — fix (unique blob key / `If-None-Match: *`) not yet written into 02. Started 03.
- 2026-11-06 — easy **03 Rate Limiter** done (side questions: token-bucket refill code in Go, verified by running; end-to-end walk of one request through limiter + Redis Lua). Started 04.
- 2026-11-08 — easy **04 Unique ID Generator** done. Started 05.
- 2026-11-08 — easy **05 In-Memory Cache with TTL** done (long side track: Big-O, p99/SLO/nines vocabulary, ns timing in Go vs Rust measured, jitter & deadline guarantees, cache misses / page faults / CPU clock / pinning / busy-spin / RTOS, hints for learning Go/Rust memory layout → lessons 46–48). Started 06.
- 2026-11-09 — easy **06 View & Like Counter** done (fixed the like-rows volume estimate in 06: 5k/s ≈ 158B rows/yr, not 1B; side question: what a delta is). Started 07.
- 2026-11-10 — easy **07 Leaderboard** done (fixed 07 §9.5: tiebreak time part must be minutes since board start to fit 20 bits, and composite scores need an atomic Lua update instead of `ZINCRBY`; side question: what "one node" means — data fits one Redis, clients are served by many API servers). Started 08 (fixed its storage math: 1-minute rollups don't shrink data at a 60 s interval — now raw 7 d + 5-min rollups 90 d + 1-hour rollups 2 y).
- 2026-11-10 — easy **08 Uptime Monitor** done (side questions: worker pool of goroutines, backpressure, the due-queue as a Redis sorted set, end-to-end flow, why not serve everything from Redis → cache only the hot subset). Started 09 (fixed its CDN TTL: `max-age=30` + 30 s poll = ~60 s worst case, now `max-age=5`).
- 2026-11-11 — easy **09 Feature Flags** done. Started 10 (fixed its retry-load math: 5% × 6 retries = +30%, not +50%; completed the SSRF private-range list: `fc00::/7`, `fe80::/10`, `0/8`, `100.64/10`, IPv4-mapped IPv6).
- 2026-11-11 — **36 new easy tasks (16–51)** mined from the `honeydolist-site` project and written at big scale, each with a 🌱 Seed line (what the project really does, with PLAN step / migration / ADR) and a *Here, at scale* line: intake & abuse (16–19), records & integrity (20–23), lifecycle & workflow (24–27), booking & commerce (28–31), content & SEO (32–35), AI & search (36–39), platform & operations (40–43), comms, access & analytics (44–47), tooling & settings (48–51). Nine ideas that didn't get a file are listed in the README's *Easy backlog*. Resume pointer unchanged (still task 10). All 14 sections, links and the 15→16 chain checked by script; numbers and SQL were re-read by the writers but not run — expect arithmetic fixes as each task is read, as with 06–10.
- 2026-11-11 — **9 more easy tasks (52–60)** finished from the *Easy backlog* (configurable forms, web push/PWA, programmatic location pages, order timeline, related content & interlinking, event registration & waitlist, outbound click tracking, provider adapters & fakes, business-day time zones). The honeydolist-site seed is now fully mined: easy has **60 tasks**, the backlog is empty. Same checks as 16–51 (sections, links, Prev/Next chain); the Go in 60 was actually run, the SQL anywhere was not. Resume pointer unchanged (task 10).

---
*Global progress: [../PROGRESS.md](../PROGRESS.md).*
