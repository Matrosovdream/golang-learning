# 59 — Production Database Patterns

> Part of **Part 12 — Production Web App Concerns**, the closing lesson. The deep sequel to [22 — Persistence with database/sql](22-database.md): where 22 teaches the mechanics, this teaches the patterns that keep a database fast and correct under real load. Builds on [12 — Errors](12-errors.md), [15 — Context](15-sync-context.md), [25 — Architecture](25-architecture.md)/[31 — DDD](31-ddd-tactical.md) (the repository), [55 — Data Pipelines](55-data-pipelines.md) (row scanning), [57 — Web Security](57-web-security.md) (parameterization), and touches [44 — Background Jobs](44-background-jobs-queues.md) (advisory locks) and [58 — Real-Time](58-realtime-websockets-sse.md) (`LISTEN/NOTIFY`). Thesis: **`database/sql` gives you a connection pool and a scanning API; production readiness is everything around it — pool sizing, transactions done right, migrations, killing N+1, keyset pagination, indexes you can read from `EXPLAIN`, upserts, and the concurrency-control choice between optimistic and pessimistic locking.**

> The runnable examples use **`modernc.org/sqlite`** (pure Go, no cgo, no external database) so you can type and run them anywhere. Everything here is standard `database/sql`, so the same code works against Postgres by swapping the driver and DSN; the few **Postgres-only** features (`FOR UPDATE`, advisory locks, `LISTEN/NOTIFY`, `COPY`, jsonb) are shown as clearly-marked reference snippets.

## Goals
- Treat `*sql.DB` as a **pool**: `Ping`, and tune `SetMaxOpenConns`/`SetMaxIdleConns`/`SetConnMaxLifetime`/`SetConnMaxIdleTime`.
- Read/write correctly: `QueryRow`/`Query` + the `Next`/`Scan`/`Err`/`Close` idiom, `sql.ErrNoRows`, **NULLs** (`sql.Null*` / `COALESCE`), **prepared statements**, **named args**, and **`*Context` everywhere**.
- Run **transactions** with the `Begin → defer Rollback → Commit` pattern, understand **isolation levels**, and **retry** transient serialization failures.
- Apply the performance patterns: **schema migrations**, killing the **N+1** query, **batch insert**, **keyset (cursor) pagination**, reading **`EXPLAIN`** for index use, and **upsert**.
- Choose concurrency control: **optimistic** (version column) vs **pessimistic** (`FOR UPDATE`); plus **soft deletes**, the **repository** boundary, **read/write splitting**, **bulk loading**, and Postgres extras (**advisory locks**, **`LISTEN/NOTIFY`**, **jsonb**).

## Concepts

- **`*sql.DB` is a pool, not a connection.** `sql.Open` validates arguments but doesn't connect; the first query (or `Ping`) does. **Size the pool deliberately**: `SetMaxOpenConns` caps concurrency (your DB has a hard connection limit — a web app fanning out unbounded goroutines will exhaust it), `SetMaxIdleConns` keeps warm connections (set it equal to MaxOpen to avoid churn), `SetConnMaxLifetime` recycles connections (so a load-balancer failover or a killed server-side connection is eventually replaced), and `SetConnMaxIdleTime` reaps idle ones.
- **The read idioms:** `QueryRow(...).Scan(...)` for one row (a miss is **`sql.ErrNoRows`** — check it with `errors.Is`); `Query` + `for rows.Next() { rows.Scan(...) }` + **`rows.Err()`** + **`rows.Close()`** for many. A **NULL** can't scan into a plain `string`/`int` — use `sql.NullString`/`NullInt64`, or `COALESCE(col, default)` in SQL. Note that **aggregates over zero rows return NULL** (`SUM` of an empty set), a classic scan panic.
- **Prepared statements and named args.** `db.Prepare` parses/plans once and executes many times (and is inherently parameterized — see [57](57-web-security.md)). `sql.Named` gives readable placeholders when a query has many arguments. **Always use `*Context` variants** (`ExecContext`/`QueryContext`/`QueryRowContext`) so a request's timeout/cancellation propagates to the query.
- **Transactions: `Begin → defer tx.Rollback() → … → tx.Commit()`.** The deferred `Rollback` is a no-op after a successful `Commit` (it returns `ErrTxDone`), so this one pattern makes every early return safe. All statements in a tx run on the **same connection**, so run them through `tx`, not `db`.
- **Isolation levels decide what concurrent transactions can see.** `BeginTx` takes a `sql.TxOptions{Isolation}`. Higher levels (**Serializable**) prevent anomalies but can **abort** a transaction that would violate consistency — and the correct response is to **retry** it (Postgres raises SQLSTATE `40001`; SQLite raises `SQLITE_BUSY`). Wrap transactional work in a small retry loop.
- **Migrations version your schema.** Keep ordered, immutable migrations and a `schema_migrations` table recording what's applied; on startup, apply everything above the current version — **idempotent**, so re-running is a no-op. In real projects use `golang-migrate` or `goose`; the mechanism is the same.
- **Kill the N+1.** Fetching a list and then querying once *per row* is N+1 round-trips. Replace it with **one query** using `IN (...)` (build the right number of placeholders) or a `JOIN`. This is the single most common database performance bug.
- **Paginate by keyset, not `OFFSET`.** `WHERE id > :last ORDER BY id LIMIT n` uses the index to jump straight to the page — fast at any depth. `OFFSET 1000000` still scans and discards a million rows first. Carry the last id forward as the **cursor**.
- **Read the plan; add the index.** `EXPLAIN QUERY PLAN` (SQLite) / `EXPLAIN (ANALYZE)` (Postgres) shows whether a query **SCANs** a table or **SEARCHes** an index. A filter/sort/join on an unindexed column is a full scan; add an index and re-check. **Batch inserts** (multi-row `INSERT`, or one transaction around many) turn N fsyncs into one.
- **Upsert** (`INSERT … ON CONFLICT(key) DO UPDATE SET … = excluded.…`) is one atomic statement for "insert or update" — same syntax in SQLite and Postgres, and it beats a read-then-write race.
- **Concurrency control — pick one:** **optimistic** (carry a `version` column; `UPDATE … WHERE version = :expected`; zero rows affected ⇒ someone else won ⇒ you detected the lost update) is best when conflicts are **rare**. **Pessimistic** (`SELECT … FOR UPDATE` locks the row until commit; others wait) is best when contention is **high**.
- **The rest of the toolkit:** **soft deletes** (`deleted_at` + `WHERE deleted_at IS NULL`, plus a **partial unique index** so uniqueness applies to live rows only); the **repository** pattern (SQL behind a domain interface, translating `sql.ErrNoRows` into a domain error); **read/write splitting** (writes → primary, heavy reads → replica, mindful that replicas **lag**); **bulk loading** (Postgres `COPY` via `pgx.CopyFrom` is far faster than `INSERT`); and Postgres extras — **advisory locks** (`pg_advisory_lock`, an app-level mutex for single-runner jobs across instances → [44](44-background-jobs-queues.md)) and **`LISTEN/NOTIFY`** (a lightweight pub/sub bus, a natural real-time backplane → [58](58-realtime-websockets-sse.md)).

## Exercises
1. Open a pool, `Ping`, and read `db.Stats()`; then set the four pool knobs and inspect them.
2. `QueryRow` + `Scan` and handle `sql.ErrNoRows`; run the full multi-row `Next`/`Scan`/`Err`/`Close` loop.
3. Pass a `context` with a timeout to `ExecContext`/`QueryContext`, and show a cancelled context failing fast.
4. Scan a NULL with `sql.NullString`; use `COALESCE`; scan a NULL `SUM` into `sql.NullInt64`.
5. Prepare a statement and reuse it; use `sql.Named` placeholders.
6. Do a transfer inside a transaction (`Begin`/defer `Rollback`/`Commit`); make it roll back on insufficient funds.
7. Write a `schema_migrations`-backed migrator; run it twice and confirm the second run is a no-op.
8. Reproduce N+1, then fix it with one `IN (...)` query; do a multi-row batch insert.
9. Keyset-paginate a table by carrying the last id; read `EXPLAIN QUERY PLAN` before/after adding an index.
10. Write an upsert with `ON CONFLICT … DO UPDATE`; implement optimistic locking with a version column.
11. Wrap a transaction in a retry loop that retries on a transient serialization error (with `Serializable` isolation).
12. Store and query JSON with `json_extract`; implement soft deletes; put SQL behind a repository interface that returns a domain "not found".
13. Sketch read/write splitting and bulk loading; note the Postgres-only `FOR UPDATE` / advisory-lock / `LISTEN/NOTIFY` / `COPY` patterns.
14. Capstone: an orders repo — migrate, seed in a tx with upsert, keyset-paginate the paid orders, and produce an aggregate report.

## Best Practices & Pitfalls
- **Size the pool to your database, not to infinity.** An unbounded `MaxOpenConns` under load exhausts the DB's connection limit; set `ConnMaxLifetime` so failovers heal.
- **Always `rows.Close()` and check `rows.Err()`.** A leaked `*sql.Rows` holds a connection; an error can surface only after the loop.
- **Pitfall — NULL and empty-set aggregates.** `Scan` into `sql.Null*` (or `COALESCE`) or you'll get "converting NULL to string is unsupported".
- **Pitfall — running tx statements on `db`.** They'd use a different pooled connection and escape the transaction. Use the `tx` handle for everything inside.
- **Pitfall — `OFFSET` pagination.** It degrades linearly with depth and can skip/duplicate rows when data shifts. Prefer keyset.
- **Pitfall — N+1.** Watch for a query inside a `range` over query results; collapse to one `IN`/`JOIN`.
- **Pitfall — ignoring serialization failures.** Under `Serializable`, commits can fail transiently; retry them, don't crash.
- **Parameterize everything** ([57](57-web-security.md)); never build SQL with string concatenation.
- **In real code use a migration tool** (`golang-migrate`/`goose`) and a driver like **`pgx`** for Postgres; the hand-rolled versions here are for understanding.

## Checklist
- [ ] I treat `*sql.DB` as a pool and tune its four knobs; I `Ping` to fail fast.
- [ ] I use the `QueryRow`/`Query` idioms correctly, handle `ErrNoRows` and NULLs, and always pass a context.
- [ ] I run transactions with `Begin`/defer `Rollback`/`Commit`, understand isolation, and retry serialization failures.
- [ ] I version my schema with migrations and can kill an N+1 with `IN`/`JOIN`.
- [ ] I paginate by keyset, read `EXPLAIN` to confirm index use, and batch inserts.
- [ ] I can upsert, and I can choose optimistic vs pessimistic locking for the contention level.
- [ ] I use soft deletes, a repository boundary, read/write splitting, and know the Postgres-only tools (`FOR UPDATE`, advisory locks, `LISTEN/NOTIFY`, `COPY`).

## Resources
- `database/sql`: https://pkg.go.dev/database/sql · Go's official tutorial: https://go.dev/doc/database/ · "Common pitfalls" (go-database-sql.org): http://go-database-sql.org/
- Drivers: `pgx` (Postgres) https://pkg.go.dev/github.com/jackc/pgx/v5 · `modernc.org/sqlite` (pure-Go SQLite) https://pkg.go.dev/modernc.org/sqlite
- Migrations: `golang-migrate` https://github.com/golang-migrate/migrate · `goose` https://github.com/pressly/goose · Codegen: `sqlc` https://sqlc.dev/
- Postgres docs — `EXPLAIN`, isolation, advisory locks, `LISTEN`/`NOTIFY`, `COPY`: https://www.postgresql.org/docs/current/
- Examples: [examples/59-production-database-patterns](examples/59-production-database-patterns/).
- Related in this plan: the basics in [22](22-database.md); the repository/architecture in [25](25-architecture.md) & [31](31-ddd-tactical.md); parameterization in [57](57-web-security.md); advisory locks & single-runner jobs in [44](44-background-jobs-queues.md); `LISTEN/NOTIFY` as a backplane in [58](58-realtime-websockets-sse.md).
