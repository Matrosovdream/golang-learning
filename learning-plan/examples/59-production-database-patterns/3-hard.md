# Step 59 — Production Database Patterns · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

Isolation & retries, the locking choice, JSON columns, soft deletes, the repository boundary, read/write splitting, bulk loading, and an end-to-end orders repo. Examples 19–20 are **Postgres reference** snippets (printed, not run).

---

## 18. Isolation levels and retry

`🔴 hard` · *transactions*

Isolation levels decide what concurrent transactions can see. Higher levels (**Serializable**) prevent anomalies but can **abort** a transaction that would break consistency (Postgres SQLSTATE `40001`; SQLite `SQLITE_BUSY`) — and the correct response is to **retry** it. Wrap transactional work in a small retry loop that re-runs on a transient error.

**Steps:**

1. `BeginTx(ctx, &sql.TxOptions{Isolation: level})`.
2. On a transient error, roll back and loop; on a real error, return.
3. The simulated tx conflicts once, then commits on attempt 2.

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

var errTransient = errors.New("serialization failure (retryable)")

// withRetry runs fn in a transaction, retrying on a transient serialization/busy error
// (Postgres SQLSTATE 40001; SQLite SQLITE_BUSY). Serializable isolation can ABORT a tx
// that would break consistency — the correct response is to retry it.
func withRetry(db *sql.DB, level sql.IsolationLevel, fn func(*sql.Tx) error) (int, error) {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		var tx *sql.Tx
		tx, err = db.BeginTx(context.Background(), &sql.TxOptions{Isolation: level})
		if err != nil {
			return attempt, err
		}
		if err = fn(tx); err != nil {
			tx.Rollback()
			if errors.Is(err, errTransient) {
				continue // transient -> try again
			}
			return attempt, err
		}
		if err = tx.Commit(); err == nil {
			return attempt, nil
		}
	}
	return 3, err
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE t (n INTEGER)`)

	// Simulate a tx that conflicts once, then succeeds on retry.
	attempts := 0
	final, err := withRetry(db, sql.LevelSerializable, func(tx *sql.Tx) error {
		attempts++
		if attempts == 1 {
			return errTransient
		}
		_, e := tx.Exec(`INSERT INTO t (n) VALUES (?)`, attempts)
		return e
	})
	fmt.Println("committed on attempt:", final, "err:", err)

	var n int
	db.QueryRow(`SELECT count(*) FROM t`).Scan(&n)
	fmt.Println("rows:", n)
}
```

**Output:**

```
committed on attempt: 2 err: <nil>
rows: 1
```

---

## 19. Pessimistic locking

`🔴 hard` · *locking · Postgres*

The other concurrency-control strategy: **lock the row up front** with `SELECT … FOR UPDATE` so no one else can change it until you commit; other writers **wait**. Best when contention is **high** (optimistic retries would just thrash). This is Postgres syntax — SQLite has no row locks (its `BEGIN IMMEDIATE` takes a database-level write lock). *This example prints the reference pattern.*

**Steps:**

1. `SELECT … FOR UPDATE` inside a transaction takes the lock.
2. Compare with optimistic (step 17): optimistic for low contention, pessimistic for high.
3. Note SQLite's coarser `BEGIN IMMEDIATE`.

```go
package main

import "fmt"

func main() {
	// PESSIMISTIC locking (Postgres): lock the row up front so no one else can change it
	// until you commit. Best when contention is HIGH (retrying would just thrash).
	fmt.Println("-- Postgres: pessimistic row lock --")
	fmt.Println("BEGIN;")
	fmt.Println("  SELECT balance FROM accounts WHERE id = 1 FOR UPDATE; -- locks the row")
	fmt.Println("  UPDATE accounts SET balance = balance - 50 WHERE id = 1;")
	fmt.Println("COMMIT;")
	fmt.Println()
	fmt.Println("optimistic (step 17): assume no conflict, detect via a version column, retry.")
	fmt.Println("                      Best when contention is LOW.")
	fmt.Println("pessimistic (FOR UPDATE): take the lock first; other writers WAIT.")
	fmt.Println("                      Best when contention is HIGH.")
	fmt.Println("(SQLite has no row locks; BEGIN IMMEDIATE takes a database-level write lock.)")
}
```

**Output:**

```
-- Postgres: pessimistic row lock --
BEGIN;
  SELECT balance FROM accounts WHERE id = 1 FOR UPDATE; -- locks the row
  UPDATE accounts SET balance = balance - 50 WHERE id = 1;
COMMIT;

optimistic (step 17): assume no conflict, detect via a version column, retry.
                      Best when contention is LOW.
pessimistic (FOR UPDATE): take the lock first; other writers WAIT.
                      Best when contention is HIGH.
(SQLite has no row locks; BEGIN IMMEDIATE takes a database-level write lock.)
```

---

## 20. Advisory locks and LISTEN/NOTIFY

`🔴 hard` · *Postgres*

Two Postgres features with no SQLite equivalent but heavy production use. **Advisory locks** (`pg_advisory_lock`) are an app-level mutex living in the database — perfect for "only one instance runs this job at a time" ([step 44](../44-background-jobs-queues.md)). **`LISTEN/NOTIFY`** is a lightweight pub/sub bus — a natural real-time **backplane** ([step 58](../58-realtime-websockets-sse.md)). *This example prints the reference patterns.*

**Steps:**

1. `pg_advisory_lock(key)` / `pg_advisory_unlock(key)` around single-runner work.
2. `LISTEN channel` (subscriber) + `NOTIFY channel, payload` (publisher).
3. Note the ties to background jobs and the WebSocket backplane.

```go
package main

import "fmt"

func main() {
	// Two Postgres features with no SQLite equivalent, but essential at scale:
	fmt.Println("-- Advisory locks: an app-level mutex living in Postgres --")
	fmt.Println("SELECT pg_advisory_lock(42);   -- blocks until the lock on key 42 is held")
	fmt.Println("-- ... work only one instance should do at a time (e.g. a scheduled job) ...")
	fmt.Println("SELECT pg_advisory_unlock(42);")
	fmt.Println()
	fmt.Println("-- LISTEN/NOTIFY: Postgres as a lightweight pub/sub bus --")
	fmt.Println("LISTEN new_orders;                 -- a worker subscribes")
	fmt.Println("NOTIFY new_orders, '{\"id\":123}';   -- a writer publishes (often from a trigger)")
	fmt.Println()
	fmt.Println("Use advisory locks for single-runner jobs across many instances;")
	fmt.Println("use LISTEN/NOTIFY to push DB changes to app instances (a backplane -> step 58).")
}
```

**Output:**

```
-- Advisory locks: an app-level mutex living in Postgres --
SELECT pg_advisory_lock(42);   -- blocks until the lock on key 42 is held
-- ... work only one instance should do at a time (e.g. a scheduled job) ...
SELECT pg_advisory_unlock(42);

-- LISTEN/NOTIFY: Postgres as a lightweight pub/sub bus --
LISTEN new_orders;                 -- a worker subscribes
NOTIFY new_orders, '{"id":123}';   -- a writer publishes (often from a trigger)

Use advisory locks for single-runner jobs across many instances;
use LISTEN/NOTIFY to push DB changes to app instances (a backplane -> step 58).
```

---

## 21. JSON columns

`🔴 hard` · *json*

Sometimes a column holds a JSON document. SQLite's **`json_extract`** (the json1 extension) queries *into* the JSON with a path; Postgres uses `jsonb` with `->`/`->>` and can index it with GIN. Store the JSON as text and pull out fields — or filter on them.

**Steps:**

1. Store a JSON string per row.
2. `json_extract(col, '$.path')` reads nested fields.
3. Filter rows by a JSON field.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY, payload TEXT)`) // JSON stored as TEXT

	db.Exec(`INSERT INTO events (payload) VALUES ('{"type":"click","user":{"id":7}}')`)
	db.Exec(`INSERT INTO events (payload) VALUES ('{"type":"view","user":{"id":9}}')`)

	// SQLite's json_extract (json1) queries INTO the JSON. Postgres uses jsonb + -> / ->>.
	rows, _ := db.Query(`SELECT id, json_extract(payload,'$.type'),
	                            json_extract(payload,'$.user.id')
	                     FROM events ORDER BY id`)
	defer rows.Close()
	for rows.Next() {
		var id, uid int
		var typ string
		rows.Scan(&id, &typ, &uid)
		fmt.Printf("event %d: type=%s user=%d\n", id, typ, uid)
	}

	// Filter by a JSON field.
	var n int
	db.QueryRow(`SELECT count(*) FROM events WHERE json_extract(payload,'$.type') = 'click'`).Scan(&n)
	fmt.Println("clicks:", n)
}
```

**Output:**

```
event 1: type=click user=7
event 2: type=view user=9
clicks: 1
```

---

## 22. Soft deletes

`🔴 hard` · *modeling*

Many systems never truly delete — they set a `deleted_at` timestamp (keeping history and audit trails). Every "live" query must then filter `WHERE deleted_at IS NULL`. The subtlety: a plain `UNIQUE(name)` would block re-creating a deleted record, so use a **partial unique index** that applies only to live rows.

**Steps:**

1. Soft-delete Bob by setting `deleted_at`.
2. Count live (`deleted_at IS NULL`) vs total.
3. Note the partial unique index for re-creating deleted names.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, deleted_at TEXT)`)
	db.Exec(`INSERT INTO users (name) VALUES ('Alice'),('Bob'),('Carol')`)

	// Soft delete = set deleted_at instead of removing the row (keeps history/audit).
	// Every "live" query must then filter WHERE deleted_at IS NULL.
	db.Exec(`UPDATE users SET deleted_at = '2026-07-16' WHERE name = 'Bob'`)

	var live, all int
	db.QueryRow(`SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&live)
	db.QueryRow(`SELECT count(*) FROM users`).Scan(&all)
	fmt.Printf("live=%d total=%d\n", live, all)

	// Caveat: a plain UNIQUE(name) would block re-creating "Bob". Use a PARTIAL unique
	// index so uniqueness applies only to live rows:
	//   CREATE UNIQUE INDEX u_name ON users(name) WHERE deleted_at IS NULL;
	fmt.Println("re-create Bob: allowed with a partial unique index on live rows")
}
```

**Output:**

```
live=2 total=3
re-create Bob: allowed with a partial unique index on live rows
```

---

## 23. The repository pattern

`🔴 hard` · *architecture*

Keep SQL behind a **domain-facing interface** (the repository — [step 25](../25-architecture.md)/[31](../31-ddd-tactical.md)). Callers depend on `UserRepository`, not `database/sql`, so the storage is swappable and testable. A key job of the boundary: **translate driver errors into domain errors** (`sql.ErrNoRows` → `ErrNotFound`) so the rest of the app never imports `database/sql`.

**Steps:**

1. Define `UserRepository` with `Create`/`Get`.
2. The SQL implementation maps `sql.ErrNoRows` to a domain `ErrNotFound`.
3. Callers use the interface and the domain error.

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

type User struct {
	ID   int
	Name string
}

var ErrNotFound = errors.New("user not found")

// UserRepository is the domain-facing interface; the SQL lives behind it.
type UserRepository interface {
	Create(ctx context.Context, name string) (int, error)
	Get(ctx context.Context, id int) (User, error)
}

type sqlUserRepo struct{ db *sql.DB }

func (r *sqlUserRepo) Create(ctx context.Context, name string) (int, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO users (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (r *sqlUserRepo) Get(ctx context.Context, id int) (User, error) {
	var u User
	err := r.db.QueryRowContext(ctx, `SELECT id, name FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound // translate the driver error into a DOMAIN error
	}
	return u, err
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)

	var repo UserRepository = &sqlUserRepo{db: db}
	ctx := context.Background()
	id, _ := repo.Create(ctx, "Alice")
	u, _ := repo.Get(ctx, id)
	fmt.Printf("created+fetched: %+v\n", u)

	_, err := repo.Get(ctx, 999)
	fmt.Println("missing maps to domain error:", errors.Is(err, ErrNotFound))
}
```

**Output:**

```
created+fetched: {ID:1 Name:Alice}
missing maps to domain error: true
```

---

## 24. Read/write splitting

`🔴 hard` · *scaling*

At read-heavy scale you run a **primary** for writes and one or more **read replicas**. Route by intent: writes and read-your-writes go to the primary; heavy read traffic goes to a replica. The catch is **replication lag** — a replica may not yet have your just-committed write, so it's eventually consistent.

**Steps:**

1. Wrap two handles; `Writer()` → primary, `Reader()` → replica.
2. Write to the primary, read from the replica.
3. Remember replicas lag — don't read-your-own-writes from one.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// A DB with a primary (writes) and one or more read replicas (reads). Route by intent:
// writes + read-your-writes go to primary; heavy read traffic goes to a replica.
type DB struct {
	primary *sql.DB
	replica *sql.DB
}

func (d *DB) Writer() *sql.DB { return d.primary }
func (d *DB) Reader() *sql.DB { return d.replica } // replicas can lag -> eventual consistency

func main() {
	// Here one SQLite handle stands in for both; in production these are separate DSNs.
	primary, _ := sql.Open("sqlite", ":memory:")
	primary.SetMaxOpenConns(1)
	defer primary.Close()
	db := &DB{primary: primary, replica: primary}

	db.Writer().Exec(`CREATE TABLE t (n INTEGER)`)
	db.Writer().Exec(`INSERT INTO t (n) VALUES (42)`) // write -> primary

	var n int
	db.Reader().QueryRow(`SELECT n FROM t`).Scan(&n) // read -> replica
	fmt.Println("wrote to primary, read from replica:", n)
	fmt.Println("note: replicas lag; don't read-your-own-writes from a replica")
}
```

**Output:**

```
wrote to primary, read from replica: 42
note: replicas lag; don't read-your-own-writes from a replica
```

---

## 25. Bulk loading

`🔴 hard` · *performance*

Loading many rows one INSERT at a time is dominated by per-row commits/fsyncs. Wrapping them in **one transaction** collapses that to a single commit — orders of magnitude faster. On Postgres the fastest path is **`COPY`** (via `pgx.CopyFrom`), which streams rows and skips per-row statement overhead entirely.

**Steps:**

1. `Begin`, prepare once, `Exec` in a loop, `Commit`.
2. 1000 rows load under a single commit.
3. Note Postgres `COPY` for the fastest bulk path.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE nums (n INTEGER)`)

	// Bulk load: wrap many inserts in ONE transaction so there's a single commit/fsync,
	// not one per row (orders of magnitude faster).
	tx, _ := db.Begin()
	stmt, _ := tx.Prepare(`INSERT INTO nums (n) VALUES (?)`)
	for i := 0; i < 1000; i++ {
		stmt.Exec(i)
	}
	stmt.Close()
	tx.Commit()

	var count int
	db.QueryRow(`SELECT count(*) FROM nums`).Scan(&count)
	fmt.Println("bulk-inserted rows:", count)

	fmt.Println()
	fmt.Println("-- Postgres: the fastest bulk path is COPY, not INSERT --")
	fmt.Println("COPY nums (n) FROM STDIN;   -- via pgx CopyFrom in Go")
}
```

**Output:**

```
bulk-inserted rows: 1000

-- Postgres: the fastest bulk path is COPY, not INSERT --
COPY nums (n) FROM STDIN;   -- via pgx CopyFrom in Go
```

---

## 26. Capstone: an orders repository

`🔴 hard` · *capstone*

The lesson assembled: **migrate** a schema (with a composite index), **seed** in a transaction using **upsert**, **keyset-paginate** the paid orders, and produce an **aggregate report** by customer. This is the shape of a real reporting endpoint over a real database.

**Steps:**

1. `migrate` creates `orders` + an index on `(status, id)`.
2. Seed five orders in a tx with `ON CONFLICT DO UPDATE`.
3. Keyset-paginate paid orders (pages of 2), then `GROUP BY` for revenue by customer.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func migrate(db *sql.DB) {
	db.Exec(`CREATE TABLE orders (
		id INTEGER PRIMARY KEY,
		customer TEXT NOT NULL,
		amount INTEGER NOT NULL,
		status TEXT NOT NULL
	)`)
	db.Exec(`CREATE INDEX idx_orders_status_id ON orders(status, id)`)
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	migrate(db)

	// 1. Seed orders in a transaction, upserting on id.
	tx, _ := db.Begin()
	up := `INSERT INTO orders (id, customer, amount, status) VALUES (?,?,?,?)
	       ON CONFLICT(id) DO UPDATE SET amount = excluded.amount`
	seed := []struct {
		id     int
		cust   string
		amt    int
		status string
	}{
		{1, "acme", 1200, "paid"}, {2, "globex", 4500, "paid"},
		{3, "acme", 800, "refunded"}, {4, "acme", 3300, "paid"},
		{5, "initech", 500, "paid"},
	}
	for _, o := range seed {
		tx.Exec(up, o.id, o.cust, o.amt, o.status)
	}
	tx.Commit()

	// 2. Keyset-paginate the paid orders (pages of 2).
	fmt.Println("paid orders (keyset pages of 2):")
	after := 0
	for {
		rows, _ := db.Query(
			`SELECT id, customer, amount FROM orders
			 WHERE status = 'paid' AND id > ? ORDER BY id LIMIT 2`, after)
		got := 0
		for rows.Next() {
			var id, amt int
			var cust string
			rows.Scan(&id, &cust, &amt)
			fmt.Printf("  #%d %-8s $%.2f\n", id, cust, float64(amt)/100)
			after = id
			got++
		}
		rows.Close()
		if got == 0 {
			break
		}
	}

	// 3. Aggregate report by customer (paid only).
	fmt.Println("revenue by customer:")
	rows, _ := db.Query(
		`SELECT customer, count(*), SUM(amount) FROM orders
		 WHERE status = 'paid' GROUP BY customer ORDER BY SUM(amount) DESC, customer`)
	defer rows.Close()
	for rows.Next() {
		var cust string
		var n, total int
		rows.Scan(&cust, &n, &total)
		fmt.Printf("  %-8s %d orders  $%.2f\n", cust, n, float64(total)/100)
	}
}
```

**Output:**

```
paid orders (keyset pages of 2):
  #1 acme     $12.00
  #2 globex   $45.00
  #4 acme     $33.00
  #5 initech  $5.00
revenue by customer:
  acme     2 orders  $45.00
  globex   1 orders  $45.00
  initech  1 orders  $5.00
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
