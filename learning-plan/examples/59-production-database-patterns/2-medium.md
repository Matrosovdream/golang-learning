# Step 59 — Production Database Patterns · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

The patterns that keep a database correct and fast: **transactions**, **migrations**, killing **N+1**, **batching**, **keyset pagination**, reading the **plan**, **upsert**, and **optimistic locking**.

---

## 9. Transactions

`🟡 medium` · *transactions*

Group statements that must all-succeed-or-all-fail in a transaction. The idiom is `Begin → defer tx.Rollback() → work → tx.Commit()`. The deferred `Rollback` is a **no-op after a successful Commit** (it returns `ErrTxDone`), so every early return is automatically safe.

**Steps:**

1. `tx, _ := db.Begin()`; `defer tx.Rollback()`.
2. Run all statements through **`tx`** (same connection), not `db`.
3. `tx.Commit()` makes it durable.

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
	db.Exec(`CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance INTEGER)`)
	db.Exec(`INSERT INTO accounts (id, balance) VALUES (1, 100), (2, 0)`)

	// The transaction pattern: Begin -> defer Rollback (a no-op after Commit) -> do
	// work -> Commit. If a step fails and we return early, the deferred Rollback undoes it.
	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}
	defer tx.Rollback() // safe to call after Commit; it just returns ErrTxDone

	tx.Exec(`UPDATE accounts SET balance = balance - 50 WHERE id = 1`)
	tx.Exec(`UPDATE accounts SET balance = balance + 50 WHERE id = 2`)
	if err := tx.Commit(); err != nil {
		panic(err)
	}

	var b1, b2 int
	db.QueryRow(`SELECT balance FROM accounts WHERE id = 1`).Scan(&b1)
	db.QueryRow(`SELECT balance FROM accounts WHERE id = 2`).Scan(&b2)
	fmt.Printf("balances: acct1=%d acct2=%d\n", b1, b2)
}
```

**Output:**

```
balances: acct1=50 acct2=50
```

---

## 10. Rollback on error

`🟡 medium` · *transactions*

The payoff of `defer tx.Rollback()`: when a business rule fails mid-transaction, you just **return the error** — the deferred rollback undoes any partial work, so the database is never left half-updated.

**Steps:**

1. Inside the tx, check the balance.
2. On insufficient funds, `return` — the deferred `Rollback` aborts everything.
3. The failed transfer leaves the balance unchanged; a valid one commits.

```go
package main

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

var errInsufficient = errors.New("insufficient funds")

func transfer(db *sql.DB, from, to, amount int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance int
	tx.QueryRow(`SELECT balance FROM accounts WHERE id = ?`, from).Scan(&balance)
	if balance < amount {
		return errInsufficient // deferred Rollback aborts the tx; nothing is committed
	}
	tx.Exec(`UPDATE accounts SET balance = balance - ? WHERE id = ?`, amount, from)
	tx.Exec(`UPDATE accounts SET balance = balance + ? WHERE id = ?`, amount, to)
	return tx.Commit()
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance INTEGER)`)
	db.Exec(`INSERT INTO accounts (id, balance) VALUES (1, 100), (2, 0)`)

	fmt.Println("transfer 150:", transfer(db, 1, 2, 150)) // fails -> rolled back
	var b1 int
	db.QueryRow(`SELECT balance FROM accounts WHERE id = 1`).Scan(&b1)
	fmt.Println("acct1 after failed transfer:", b1) // unchanged

	fmt.Println("transfer 60: ", transfer(db, 1, 2, 60)) // succeeds
	db.QueryRow(`SELECT balance FROM accounts WHERE id = 1`).Scan(&b1)
	fmt.Println("acct1 after ok transfer:    ", b1)
}
```

**Output:**

```
transfer 150: insufficient funds
acct1 after failed transfer: 100
transfer 60:  <nil>
acct1 after ok transfer:     40
```

---

## 11. Schema migrations

`🟡 medium` · *migrations*

Version your schema with ordered, immutable migrations and a `schema_migrations` table recording what's applied. On startup, apply everything above the current version — **idempotent**, so re-running does nothing. (In real projects use `golang-migrate`/`goose`; the mechanism is exactly this.)

**Steps:**

1. Ensure `schema_migrations` exists; read the current max version.
2. Apply each migration with a higher version, recording it.
3. A second run applies nothing.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`},
	{2, `ALTER TABLE users ADD COLUMN email TEXT`},
	{3, `CREATE INDEX idx_users_email ON users(email)`},
}

// migrate applies every migration above the current schema version, recording progress
// in a schema_migrations table so re-running is a no-op (idempotent).
func migrate(db *sql.DB) (int, error) {
	db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL)`)
	var current int
	db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current)

	applied := 0
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return applied, err
		}
		db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version)
		applied++
	}
	return applied, nil
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)

	n, _ := migrate(db)
	fmt.Println("first run applied: ", n)
	n, _ = migrate(db) // running again does nothing
	fmt.Println("second run applied:", n)

	var v int
	db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	fmt.Println("schema version:    ", v)
}
```

**Output:**

```
first run applied:  3
second run applied: 0
schema version:     3
```

---

## 12. Fix the N+1 problem

`🟡 medium` · *performance*

The most common database performance bug: fetch a list, then run one query **per row** — N+1 round-trips. At scale that's thousands of queries. Replace it with **one** query using `IN (...)` (build the right number of placeholders) or a `JOIN`.

**Steps:**

1. The N+1 version loops and queries once per id.
2. The fix builds `?,?,?` and passes all ids to one `IN (...)` query.
3. One round-trip instead of N.

```go
package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT)`)
	db.Exec(`INSERT INTO posts (id, title) VALUES (1,'a'),(2,'b'),(3,'c')`)

	ids := []int{1, 2, 3}

	// N+1: one query PER id (here 3 round-trips; at scale, thousands).
	queries := 0
	for _, id := range ids {
		var title string
		db.QueryRow(`SELECT title FROM posts WHERE id = ?`, id).Scan(&title)
		queries++
	}
	fmt.Println("N+1 approach queries:", queries)

	// Fixed: a single query with IN (...). Build the right number of placeholders.
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, _ := db.Query(`SELECT id, title FROM posts WHERE id IN (`+ph+`)`, args...)
	defer rows.Close()
	got := 0
	for rows.Next() {
		got++
	}
	fmt.Printf("IN-clause approach queries: 1 (rows: %d)\n", got)
}
```

**Output:**

```
N+1 approach queries: 3
IN-clause approach queries: 1 (rows: 3)
```

---

## 13. Batch insert

`🟡 medium` · *performance*

Inserting rows one at a time is one round-trip each. A **multi-row `INSERT`** (`VALUES (?),(?),…`) does it in a single statement. Flatten the args in order; for very large batches, chunk them (drivers cap the number of parameters).

**Steps:**

1. Build `(?),(?),…` and a flat `args` slice.
2. One `db.Exec` inserts them all.
3. `RowsAffected` confirms the count.

```go
package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE nums (n INTEGER)`)

	// A multi-row INSERT is one round-trip instead of N. Build (?),(?),... and flatten
	// the args. (Watch the driver's arg limit — chunk very large batches.)
	values := []int{1, 2, 3, 4, 5}
	var ph []string
	var args []any
	for _, v := range values {
		ph = append(ph, "(?)")
		args = append(args, v)
	}
	q := `INSERT INTO nums (n) VALUES ` + strings.Join(ph, ",")
	res, err := db.Exec(q, args...)
	if err != nil {
		panic(err)
	}
	n, _ := res.RowsAffected()
	fmt.Println("rows inserted in one statement:", n)
}
```

**Output:**

```
rows inserted in one statement: 5
```

---

## 14. Keyset pagination

`🟡 medium` · *pagination*

`OFFSET 1000000` still scans and throws away a million rows before returning your page — it degrades linearly with depth. **Keyset pagination** (`WHERE id > :last ORDER BY id LIMIT n`) uses the index to jump straight to the page. Carry the last id forward as the **cursor**.

**Steps:**

1. `page(after, limit)` = `WHERE id > after ORDER BY id LIMIT limit`.
2. Start with `after = 0`; after each page set `after` to the last id.
3. Walk three pages of three.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Keyset pagination: WHERE id > lastSeen ORDER BY id LIMIT n. Unlike OFFSET, the DB
// jumps straight to the position via the index — fast no matter how deep you page.
func page(db *sql.DB, afterID, limit int) []int {
	rows, _ := db.Query(`SELECT id FROM items WHERE id > ? ORDER BY id LIMIT ?`, afterID, limit)
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`)
	for i := 1; i <= 10; i++ {
		db.Exec(`INSERT INTO items (id) VALUES (?)`, i)
	}

	// Walk pages by carrying the last id forward (the "cursor").
	after := 0
	for p := 1; p <= 3; p++ {
		ids := page(db, after, 3)
		if len(ids) == 0 {
			break
		}
		fmt.Printf("page %d: %v\n", p, ids)
		after = ids[len(ids)-1]
	}
	// Contrast: OFFSET 1000000 still scans + discards a million rows first.
}
```

**Output:**

```
page 1: [1 2 3]
page 2: [4 5 6]
page 3: [7 8 9]
```

---

## 15. Read the query plan

`🟡 medium` · *indexes*

Before optimizing, look at what the planner does. **`EXPLAIN QUERY PLAN`** (SQLite; Postgres uses `EXPLAIN (ANALYZE)`) tells you whether a query **SCANs** the whole table or **SEARCHes** an index. A filter on an unindexed column is a full scan; add the index and re-check.

**Steps:**

1. `EXPLAIN QUERY PLAN <query>` and read the detail column.
2. Without an index on `email`, it's a `SCAN`.
3. After `CREATE INDEX`, it's a `SEARCH … USING … INDEX`.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// plan returns the query planner's description (SQLite EXPLAIN QUERY PLAN). Postgres
// uses EXPLAIN (ANALYZE) with richer output.
func plan(db *sql.DB, query string) string {
	rows, _ := db.Query("EXPLAIN QUERY PLAN " + query)
	defer rows.Close()
	detail := ""
	for rows.Next() {
		var id, parent, notused int
		var d string
		rows.Scan(&id, &parent, &notused, &d)
		detail = d
	}
	return detail
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	for i := 1; i <= 100; i++ {
		db.Exec(`INSERT INTO users (email) VALUES (?)`, fmt.Sprintf("u%d@x.com", i))
	}

	// Without an index, filtering by email is a full table SCAN.
	fmt.Println("no index:  ", plan(db, `SELECT * FROM users WHERE email = 'u50@x.com'`))

	// Add an index -> the planner SEARCHes using it instead.
	db.Exec(`CREATE INDEX idx_email ON users(email)`)
	fmt.Println("with index:", plan(db, `SELECT * FROM users WHERE email = 'u50@x.com'`))
}
```

**Output:**

```
no index:   SCAN users
with index: SEARCH users USING COVERING INDEX idx_email (email=?)
```

---

## 16. Upsert

`🟡 medium` · *upsert*

"Insert, or update if it already exists" is one atomic statement: `INSERT … ON CONFLICT(key) DO UPDATE SET …`. `excluded` refers to the row that *would* have been inserted. Same syntax in SQLite and Postgres, and it beats a read-then-write race.

**Steps:**

1. `INSERT … ON CONFLICT(name) DO UPDATE SET count = count + 1`.
2. Repeated upserts of `hits` increment; the first `misses` inserts.
3. One statement, no race.

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
	db.Exec(`CREATE TABLE counters (name TEXT PRIMARY KEY, count INTEGER)`)

	// Upsert: insert, or if the key already exists, update instead. "excluded" refers to
	// the row that would have been inserted. (Same syntax in SQLite and Postgres.)
	upsert := `INSERT INTO counters (name, count) VALUES (?, 1)
	           ON CONFLICT(name) DO UPDATE SET count = count + 1`
	for i := 0; i < 3; i++ {
		db.Exec(upsert, "hits")
	}
	db.Exec(upsert, "misses")

	rows, _ := db.Query(`SELECT name, count FROM counters ORDER BY name`)
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int
		rows.Scan(&name, &count)
		fmt.Printf("%s = %d\n", name, count)
	}
}
```

**Output:**

```
hits = 3
misses = 1
```

---

## 17. Optimistic concurrency

`🟡 medium` · *locking*

When two clients read a row and both write it back, the second silently clobbers the first (a lost update). **Optimistic locking** carries a `version` column: the update only applies `WHERE version = :expected` and bumps it. If another writer already bumped the version, **zero rows are affected** — so you *detect* the conflict instead of losing data. Best when conflicts are rare.

**Steps:**

1. `UPDATE … SET version = version + 1 WHERE id = ? AND version = ?`.
2. Two clients both hold version 1; the first wins.
3. The second affects zero rows → rejected.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// updateWithVersion only succeeds if the row's version still matches what we read
// (optimistic locking). A concurrent writer that bumped the version makes this a no-op,
// so we DETECT the lost update instead of silently clobbering it.
func updateWithVersion(db *sql.DB, id, expectedVersion int, name string) bool {
	res, _ := db.Exec(
		`UPDATE docs SET name = ?, version = version + 1 WHERE id = ? AND version = ?`,
		name, id, expectedVersion)
	n, _ := res.RowsAffected()
	return n == 1
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE docs (id INTEGER PRIMARY KEY, name TEXT, version INTEGER)`)
	db.Exec(`INSERT INTO docs (id, name, version) VALUES (1, 'orig', 1)`)

	// Two clients both read version 1.
	fmt.Println("client A (v1) update:", updateWithVersion(db, 1, 1, "from A")) // wins
	fmt.Println("client B (v1) update:", updateWithVersion(db, 1, 1, "from B")) // stale -> rejected

	var name string
	var version int
	db.QueryRow(`SELECT name, version FROM docs WHERE id = 1`).Scan(&name, &version)
	fmt.Printf("final: name=%q version=%d\n", name, version)
}
```

**Output:**

```
client A (v1) update: true
client B (v1) update: false
final: name="from A" version=2
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
