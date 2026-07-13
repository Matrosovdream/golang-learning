# 22 — Persistence with database/sql

## Goals
- Connect to a SQL database using `database/sql` and a driver.
- Query, scan rows, and execute writes safely (no SQL injection).
- Use transactions and context timeouts.
- Understand the connection pool and where migrations fit.

## Concepts
- **`database/sql` is a generic interface, not a driver.** You import the stdlib package plus a **driver** for your database. For Postgres the modern choice is `pgx` (used via its `stdlib` adapter or directly); for SQLite, `modernc.org/sqlite`; for MySQL, `go-sql-driver/mysql`.
  ```go
  import (
      "database/sql"
      _ "github.com/jackc/pgx/v5/stdlib"   // blank import registers the driver
  )
  db, err := sql.Open("pgx", dsn)           // does NOT connect yet
  if err != nil { ... }
  defer db.Close()
  if err := db.Ping(); err != nil { ... }   // verify connectivity
  ```
  - The `_ "..."` **blank import** runs the driver's `init()` to register itself; you never call the driver directly.
  - `sql.Open` is lazy — it validates args but opens connections on demand. `Ping` forces a real connection.
- **`*sql.DB` is a connection pool, not a single connection.** It's safe for concurrent use by many goroutines — create **one** and share it. Tune it with `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`.
- **Querying multiple rows:**
  ```go
  rows, err := db.QueryContext(ctx, "SELECT id, name FROM users WHERE active = $1", true)
  if err != nil { ... }
  defer rows.Close()                         // always close rows
  for rows.Next() {
      var u User
      if err := rows.Scan(&u.ID, &u.Name); err != nil { ... }
      users = append(users, u)
  }
  if err := rows.Err(); err != nil { ... }   // check for iteration errors
  ```
- **Querying one row:**
  ```go
  var u User
  err := db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE id = $1", id).
      Scan(&u.ID, &u.Name)
  if errors.Is(err, sql.ErrNoRows) { /* not found */ }
  ```
  `sql.ErrNoRows` is the sentinel for "no row matched" — check it with `errors.Is` (lesson 12).
- **Writing data:**
  ```go
  res, err := db.ExecContext(ctx, "INSERT INTO users(name) VALUES($1)", name)
  id, _ := res.LastInsertId()      // driver-dependent (Postgres: use RETURNING instead)
  n, _ := res.RowsAffected()
  ```
- **Upsert with `INSERT ... ON CONFLICT`.** Create-or-update in a single statement — insert the row, and if it collides with an existing one, update instead of erroring.
  ```go
  const q = `INSERT INTO settings (user_id, channel) VALUES ($1, $2)
      ON CONFLICT (user_id) DO UPDATE SET channel = EXCLUDED.channel, updated_at = now()
      RETURNING user_id, channel`
  err := db.QueryRowContext(ctx, q, userID, channel).Scan(&s.UserID, &s.Channel)
  ```
  `ON CONFLICT (user_id)` names the unique/PK column whose collision triggers the update; `EXCLUDED` refers to the row you *tried* to insert (so `EXCLUDED.channel` is the new value); `RETURNING` reads the final row back after either branch. Use `ON CONFLICT (col) DO NOTHING` when you just want idempotent inserts that silently ignore duplicates.
- **Placeholders prevent SQL injection.** Use `$1, $2` (Postgres) or `?` (MySQL/SQLite) and pass args separately — **never** build SQL with `fmt.Sprintf` from user input. The driver parameterizes safely.
- **`NULL` handling** — a NULL column won't scan into a plain `string`/`int`; use `sql.NullString`, `sql.NullInt64`, or pointer types (`*string`) for nullable columns.
- **`COALESCE` to avoid nullable scans.** Rather than scan a nullable text column into a `*string`/`sql.NullString` and thread the pointer through every caller, push the default into SQL: `COALESCE(col, '')` returns `''` when the column is NULL, so it scans straight into a plain `string`.
  ```go
  err := db.QueryRowContext(ctx, "SELECT id, COALESCE(email,'') FROM users WHERE id = $1", id).
      Scan(&u.ID, &u.Email)   // u.Email is a plain string; NULL comes back as ""
  ```
  The trade-off: you lose the NULL-vs-empty distinction. Use it for text where empty *means* absent, but keep pointers/`sql.Null*` for dates, foreign keys, and any column where NULL is semantically different from the zero value.
- **Transactions** — group statements atomically:
  ```go
  tx, err := db.BeginTx(ctx, nil)
  if err != nil { ... }
  defer tx.Rollback()                         // no-op if already committed
  if _, err := tx.ExecContext(ctx, q1, ...); err != nil { return err }
  if _, err := tx.ExecContext(ctx, q2, ...); err != nil { return err }
  return tx.Commit()
  ```
  The `defer tx.Rollback()` + return-on-error + final `Commit()` pattern guarantees you never leave a transaction dangling.
- **Context everywhere** — use the `...Context` variants (`QueryContext`, `ExecContext`, `BeginTx`) and pass the request's `ctx` so queries are cancelled when the request times out or the client disconnects.
- **Migrations** — schema changes are managed *outside* `database/sql` by a migration tool: `golang-migrate`, `goose`, or `atlas`. Migrations are versioned SQL files checked into the repo and applied in order. (You won't hand-roll schema changes in app code.)
- **Higher-level options (know they exist):** `sqlc` generates type-safe Go from your SQL; `pgx` (native, not via `database/sql`) offers better Postgres features/perf; ORMs like `gorm`/`ent` exist but the community leans toward explicit SQL.

## Exercises
1. Spin up Postgres (Docker: `docker run -e POSTGRES_PASSWORD=pw -p 5432:5432 postgres`) or use SQLite for zero setup. `sql.Open` + `Ping`.
2. Create a `users` table (via a `.sql` file or a quick `Exec`) and insert a few rows with parameterized `ExecContext`.
3. Query all users with `QueryContext`, scan into a `[]User`, and remember `defer rows.Close()` + `rows.Err()`.
4. Query one user by id with `QueryRowContext`; handle the `sql.ErrNoRows` case explicitly with `errors.Is`.
5. Add a nullable column and scan it with `sql.NullString` (or `*string`).
6. Wrap two inserts in a transaction using the `BeginTx` / `defer Rollback` / `Commit` pattern; force the second to fail and confirm the first rolled back.
7. Demonstrate injection safety: try to pass `'; DROP TABLE users; --` as a parameterized value and confirm it's treated as data, not SQL.
8. Add `golang-migrate` (or `goose`) and write one up/down migration; discuss with Claude why migrations belong in their own tool.
9. Scan a nullable text column two ways: once with `sql.NullString` (inspecting `.Valid`/`.String`), then once with `COALESCE(col,'')` into a plain `string`. Compare the two call sites and note what information you gave up.
10. Write an upsert with `INSERT ... ON CONFLICT (col) DO UPDATE SET ... = EXCLUDED. ...`. Run it twice with the same key and prove the second run *updated* the row (same count, changed value) rather than inserting a duplicate.

## Best Practices & Pitfalls
- **Create one `*sql.DB` for the whole app and share it.** It's a pool; opening one per request destroys performance.
- **Always use parameterized queries.** Never interpolate user input into SQL strings — this is the #1 web vulnerability.
- **Always `defer rows.Close()` and check `rows.Err()`** after the loop; forgetting `Close` leaks connections from the pool.
- **Use the `...Context` methods and pass the request context** so queries respect timeouts/cancellation.
- **Use the `defer tx.Rollback()` + `Commit()` pattern** for every transaction; the deferred rollback is a no-op after a successful commit and saves you from leaks on error paths.
- **Set pool limits** (`SetMaxOpenConns`, `SetConnMaxLifetime`) to match your DB's capacity, especially behind a connection limit / pgbouncer.
- **Reach for `COALESCE` for nullable text, pointers/`sql.Null*` for everything where NULL ≠ zero value.** Collapsing NULL to `''` keeps scans simple, but only where empty and absent mean the same thing; preserve the distinction for dates, foreign keys, and flags.
- **Pitfall — `sql.ErrNoRows` is not a failure for `QueryRow`;** it's the normal "not found" signal. Branch on it; don't treat it as a 500.
- **Pitfall — scanning NULL into non-nullable types panics/errors.** Use `sql.Null*` or pointers.
- **Pitfall — schema changes in app code.** Keep migrations in a dedicated tool, versioned and reviewed.
- **Pitfall — `ON CONFLICT` needs a real unique constraint / PK on the conflict target;** without a unique index on the named column(s), Postgres has nothing to detect the collision and the upsert errors out.

## Checklist
- [ ] I can open a DB with a driver (blank import) and `Ping` it.
- [ ] I understand `*sql.DB` is a shared pool.
- [ ] I can query many rows (with `rows.Close`/`rows.Err`) and a single row (handling `ErrNoRows`).
- [ ] I always use parameterized queries.
- [ ] I can run a transaction with the Rollback/Commit pattern.
- [ ] I pass `ctx` into every query and know where migrations live.
- [ ] I can write an upsert with ON CONFLICT ... DO UPDATE and know COALESCE's trade-off.

## Resources
- `database/sql` package: https://pkg.go.dev/database/sql
- Tutorial — Accessing a relational database: https://go.dev/doc/tutorial/database-access
- Guide — `database/sql` (the canonical walkthrough): https://go.dev/doc/database/
- pgx driver: https://github.com/jackc/pgx · golang-migrate: https://github.com/golang-migrate/migrate
