# System Design Track · Known issues in the lesson text

Found on 2026-11-11 by the writers who appended sections 15 (AWS sizing) and 16 (database choice) to every lesson. Each
writer re-read the lesson's sections 4 and 10, and its SQL, and reported what looked wrong **outside the section it was
writing**. Nothing here has been fixed yet: sections 15 and 16 were sized from section 4 *as written* and say so where it matters.

**How to use this file:** when a lesson is presented (the session loop in the [README](README.md)), fix its entries first, then
tick them. Most are "section 4 says X, a later section says Y". The ones marked **SQL** would fail or scan a whole table if run
as written. `[ ]` open · `[x]` fixed.

## Cross-cutting

- [ ] **SQL · partitioned tables vs primary/unique keys.** In PostgreSQL (and MySQL) a primary key or unique index on a
  partitioned table must include the partition column. Affects **14** (§4 `votes` "partition by poll creation month" has no such
  column), **15** (§8 `runs` partitioned by day with `PRIMARY KEY (run_id)`; make it `(run_id, scheduled_for)`, the real
  idempotency key is `UNIQUE (job_id, scheduled_for)`), **16** (§10 "partition the outbox by day" vs the unique `dedupe_key` index and the PK),
  **18** (§8 inbox `UNIQUE (provider, account, event_id)` vs dropping partitions to prune), **52** (§8 idempotency unique index gains
  `received_at`, so a retry with a later `now()` no longer collides: use a stable key).
- [ ] **ALB cost.** Lesson 13's original brief priced a 55k req/s ALB at $20 (base fee only); the README cheat sheet now explains
  LCUs (bytes, new connections, active connections). Any lesson that still shows a $16–20 ALB for a high-rate service understates it.
- [ ] **Savings Plans do not cover RDS, ElastiCache or MSK** (those use reserved instances/nodes). Sections 15 say "≈ 1-year plan"
  loosely and apply ~35 % to all compute lines. Fine as an order of magnitude; say so if it matters.

## Per lesson

**01** · §9.1: base62 over a variable-length `VARCHAR` key is not strictly append-ordered (loose wording).
**02** · §15 bill: CloudFront 0.53 TB × $0.085 ≈ $45, shown as $40; total ≈ $835, not $830.
**03** · §4/§9: "one Redis handles ~100k/s" holds for simple commands; the check is a Lua script, so at the 50k/s peak one primary is too hot (§15 uses 2 shards; align the text).
**05** · §4 vs §8: §4 counts 8 GB as accounted bytes (6M × 1.3 KB); §8 sets the limit at ~75 % of the real budget (≈ 10.7 GB real).
**06** · §4: "one node handles 100k INCR/s" is a whole core at the cheat sheet's rate, and §4 gives no read rate, counter count or HLL count (§15 assumed them). §15 peak row writes ≈ 20k/s exceed the ~10k/s rule of thumb; it holds only because flushes are batched.
**08** · §6: never says how checkers get a monitor's URL and config (assumed: rides in the queue message; otherwise 50k lookups/s on Postgres).
**10** · §4: no event payload size (§15 assumed ~2 KB); §15 wording "1,150 fan-out commits" only matches if fan-out is one transaction per event.
**11** · ✅ fixed 2026-11-11 (all four items below, plus found while presenting: a presigned PUT cannot enforce `content-length-range`; private files must not share the content-addressed space; blob upsert/delete race) · §10 "CDN hit rate is the lever" for egress cost: CloudFront bills bytes sent to viewers hit or miss, so hit rate lowers origin load, not the egress bill. §4 "≈ 3 cores busy" is 2.3 (55.6 CPU-h ÷ 24); "0.7 PB/year" is originals only (≈ 0.8 PB with variants). No API/status-poll load given.
**13** · no response sizes given (egress in §15 is assumed).
**14** · §10 "100k votes/s is fine for Postgres with partitioning": 100k commits/s is ~100k IOPS, past one primary, and partitioning adds no write capacity on one node. §9.2: the 5 s reconcile over ≤ 1M votes needs `PRIMARY KEY … INCLUDE (option_id)` or it fetches a heap row per vote.
**15** · §8 see cross-cutting (partition key in the PK).
**16** · §4/§15: random `public_id` and random-UUID `submission_key` indexes take random inserts across ≥ 15 GB/year; §15's ~5 GB hot set on 16 GiB looks optimistic.
**17** · §4: write endpoints ≈ 1,000/s ≈ 86M requests/day vs a funnel of 1M submissions/day (≈ 12/s); quiet botnet 16.7k/s ≈ 1.4B/day vs 400k bots/day; "200 nodes" does not follow (≈ 3 nodes); §10 says the limiter breaks first at 10×, but scoring CPU does.
**18** · §2 keeps a 90-day audit envelope but §4 sizes only the 3-day inbox (130 GB; 90 days ≈ 39B rows ≈ 3.9 TB); 100 B per row for heap + 3 indexes is low (250–350 B); §4 "50k inserts/s is beyond one Postgres" is arguable with 500-row batches; pruning 432M rows/day (~5k deletes/s) is not in §15's IOPS math.
**20** · §8: the partial unique index `WHERE number IS NOT NULL` is redundant for uniqueness (NULLs never collide); it only keeps drafts out of the index. Not an error.
**21** · §4: "~2 viewers per active record" implies ~1M record hashes but the Redis memory row uses ~1.5M. §9.2: "TTL 10 s" appears twice in the node-lease sentence.
**22** · §4, §9.2, §15 count "two indexes" (+23 %, 3 random page touches per insert, 39 GB insert-hot set) but §8 defines three B-trees (PK + `audit_entity_idx` + `audit_actor_idx`), so index overhead, IOPS and the hot set are understated. §9.2 "every extra index is 23 % more write cost": 23 % is both indexes together; one is ≈ 11 %.
**23** · §4 "~10 indexes × ~40 GB = 400 GB" vs §8/§10: four list indexes + PK (≈ 5, ≈ 200 GB); §15 builds on the 400 GB (12.5 GB of indexes per shard). §15 notes the 64 DB nodes come from the 32-shard layout, not from list load (≈ 30 vCPU, 1.4 TB).
**25** · **SQL** §8/§9.4: `contact_emails` has only a partial index on `contact_id` (`WHERE is_primary`), but the merge `UPDATE contact_emails … WHERE contact_id = $loser` needs a full index; `contacts.merged_into` (chain flatten) has no index.
**26** · **SQL** §8: composite FK `tickets (workflow_id, status)` has no index starting with `workflow_id` (`tickets_stuck` starts with `tenant_id`), so a status-in-use check scans 50M rows (rare, minor).
**27** · §9.1 says "a few claims a second" at 500 fires/s; §4 gives 30 claim batches a minute (≈ 0.5/s).
**28** · §4 sizes peak holds as 3,000/s × 900 s (15 min) for every attempt; §2/§9.3 say a 10-minute hold extended once, and only ~1 in 10 attempts succeeds, so 2.7M is a loose upper bound.
**31** · §9.3 `ORDER BY merchant.location <-> point` uses a `location` column that is not in the §8 data model.
**32** · §4 "revision bytes ~6 GB/day ≈ 2.2 TB/yr" counts every save (300k/day) but §9.3 says autosave overwrites the draft (real growth ≈ 3× smaller); §2 says 200k tenants but only 50k editors.
**34** · §4 never says how much of the ~30k/s HTML peak the CDN absorbs (§15 assumed 70 %, ~9k/s at origin, plus a 20k/s spike).
**35** · §4 vs §9.1: ~1k/s Bloom false positives reaching the table (every request probes) vs ~30/s with the live-first order. **Design:** §8/§9.1 nodes poll `redirect_changes WHERE seq > $last`; identity values can commit out of order, so a lower `seq` committing after a higher one is skipped forever. "Never see a change without its log entry" does not cover it: needs an overlap window or a periodic full reload.
**36** · §4 meter 4,000 ops/s (2 per call) vs §9.3 which also runs the reserve script for the month key (≈ 8,000; §15 used 8,000 + 2,000 GETs). §2 "≤ 5 ms ≈ 0.1 % of a 2 s call" is 0.25 %. §9.3 per-field TTL on Redis hashes needs Redis 7.4 / recent Valkey: check the ElastiCache engine version.
**39** · §4: a 28 h reindex at 50k docs/s needs ~100 vCPU of indexing and a second 6.25 TB copy; the steady cluster has 72 vCPU and §9.3 does not say so (burst cost is in §15).
**41** · §4 "backup storage ≈ 52 TB" omits the year of monthly sets (12 × 10 TB = 120 TB) and the second-region copy (§6): full footprint ≈ 344 TB (priced in §15); keeps 4 weekly fulls where 14-day PITR needs 2–3.
**42** · §2 calls 20k reads/s a peak but §4 multiplies by 86,400 (1.7B reads/day, 260 GB/day of audit) as if constant. §9.4/§6: routing jobs by tenant is only needed with many caches (3 brokers each caching all 100k KEKs = 3.2 MB already gives 83 unwraps/s).
**43** · §9.4/§15: uses `pg_create_restore_point` and a self-run WAL archive but also picks RDS; RDS restores to a timestamp from its automated backups, not to a named restore point. Either say "self-managed Postgres" or "record the deploy timestamp".
**45** · §4 sizes heartbeats at 30 s (5M ÷ 30 = 170k/s) but §6, §7 and §9.1 say every 25 s (200k/s, ≈ 6.7k/s per node).
**46** · §9.2: a 40-byte bitset is ~54 base64 characters (56 padded), not "~52".
**47** · §8/§9.2: `site_day.uniques_sketch` is a 12 KB value rewritten on every flush (heavy WAL for busy sites; §15 says keep live HLLs in Redis and write hourly). `flush_log` gets a row per partition per flush with no retention stated.
**48** · §4 "10M-row commit at a fair 1k rows/s takes 2.8 h" vs §9.4: 1 chunk of 5,000 rows in flight (~5k rows/s), 4 chunks for large plans (~20k rows/s, 10M in 8 min), above the ~8k rows/s whole-shard budget in §4.
**49** · **SQL** §8: `load_statements` has `PRIMARY KEY (test_id, query)` with `query text`; a btree entry caps at ~2.7 KB, so a long statement fails the insert: key by `queryid` or a hash.
**52** · §6 diagram has the submit API inserting straight into the shards, but §4 (72 GB spike backlog) and §10 ("durable log in front") need a log at 200k/s; §4's 2.5k writes/s per shard counts only the submission row (the outbox row and up to 3 filter-projection rows are extra). §8 see cross-cutting.
**53** · §10 "10× (50M devices)" does not match 10× of §4's 20M (200M): only fits as a 50M-device campaign audience. §15: the per-send `last_success_at` update (~8.3k random-key updates/s in a campaign) is not in the IOPS/CPU math: batch per task or write only when older than a day.
**54** · **SQL** §8/§9.4: wave rollback and cohort stats filter on `wave_id`, but `location_pages` has no index on it (each scans 12M rows).
**56** · §15 ANN row: sizes int8 vectors (38 GB) "self-run HNSW or pgvector", but pgvector's compact types are `halfvec` (2 B per dimension, ≈ 77 GB for 50M × 768) and `bit`; with `halfvec` the two-copy rebuild flip (~90 GB) no longer fits one 128 GiB node.
**57** · §10 says "~100k attempts/s" at 10×, but §4 × 10 gives ~42k/s in the first minute (section 17 uses §4 × N).
**58** · §4 gives no storage for `click_verdicts` (≈ 100 B per click ≈ 6.7 GB/day ≈ 600 GB/90 days, added in §15); "~40 B of one index" per ledger row but §8 defines two indexes (≈ +30 B per row).
**60** · §9.3 says zone conversion belongs in Go and SQL should only compare instants, but the re-bucket query still does `bucket_start AT TIME ZONE $1`, which reintroduces the two-tzdata dependency.

## Notes, not errors

- 08 picks self-managed ClickHouse (operational burden; §11 says so). 09's Multi-AZ Postgres is 55 % of its bill for ~50 writes a day (§15 says so).
- 23: one Aurora cluster with a few readers would serve the list load; the 32 × 2 nodes come from platform tenant sharding.
- 52: at 32 hand-managed shards (320 at 10×), sharded MySQL with Vitess or DynamoDB is the real alternative; §16 says so, the lesson does not weigh it.
- 51: MySQL polling at 5 s would sit right at the 5 s p99 requirement (§16 recommends 1–2 s).
