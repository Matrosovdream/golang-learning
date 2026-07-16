# Step 59 — Production Database Patterns · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex
go mod init scratch && go get modernc.org/sqlite
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

The pool, the read/write idioms, contexts, NULLs, and prepared/named statements — the foundations you'll use in every query.

---

## 1. Open a connection pool

`🟢 easy` · *pool*

`sql.Open` doesn't connect — it builds a **connection pool**. The first real connection happens on use, so call `Ping` to force one and fail fast on a bad config. (`:memory:` + `SetMaxOpenConns(1)` keeps the in-memory database alive on a single connection for the demo.)

**Steps:**

1. `sql.Open("sqlite", ":memory:")` — no connection yet.
2. `db.Ping()` opens and checks one.
3. `db.Stats()` reports the pool state.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	// sql.Open does NOT connect — it prepares a POOL. The first real connection happens
	// on use; Ping forces one so you fail fast on a bad config.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // keep the in-memory DB alive on a single connection

	if err := db.Ping(); err != nil {
		panic(err)
	}
	fmt.Println("connected:", db.Stats().OpenConnections, "open connection")
}
```

**Output:**

```
connected: 1 open connection
```

---

## 2. Tune the pool

`🟢 easy` · *pool*

Production databases have a **hard connection limit**; a web app spawning unbounded goroutines will exhaust it. The four knobs: `SetMaxOpenConns` (cap concurrency), `SetMaxIdleConns` (keep warm — match MaxOpen to avoid churn), `SetConnMaxLifetime` (recycle so failovers heal), `SetConnMaxIdleTime` (reap idle).

**Steps:**

1. Set the four knobs.
2. `Ping` opens one connection.
3. `db.Stats()` shows the configured cap and current state.

```go
package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	// Pool tuning — the four knobs that matter in production:
	db.SetMaxOpenConns(25)                 // hard cap on concurrent connections
	db.SetMaxIdleConns(25)                 // keep this many warm (match MaxOpen to avoid churn)
	db.SetConnMaxLifetime(5 * time.Minute) // recycle conns (helps failover / stale conns)
	db.SetConnMaxIdleTime(1 * time.Minute) // close conns idle longer than this

	db.Ping()
	s := db.Stats()
	fmt.Println("MaxOpenConnections:", s.MaxOpenConnections)
	fmt.Println("OpenConnections:   ", s.OpenConnections)
	fmt.Println("Idle:              ", s.Idle)
}
```

**Output:**

```
MaxOpenConnections: 25
OpenConnections:    1
Idle:               1
```

---

## 3. Query a single row

`🟢 easy` · *read*

`QueryRow(...).Scan(...)` fetches exactly one row. When no row matches, `Scan` returns **`sql.ErrNoRows`** — a normal, expected condition you check with `errors.Is`, not a crash. Placeholders (`?`) keep the query parameterized (see [step 57](../57-web-security/)).

**Steps:**

1. `QueryRow(query, args...).Scan(&dest)`.
2. A present row scans with a `nil` error.
3. A missing row is `sql.ErrNoRows`.

```go
package main

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)

	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO users (name) VALUES ('Alice')`)

	// QueryRow + Scan for a single row. A missing row is sql.ErrNoRows, not a crash.
	var name string
	err := db.QueryRow(`SELECT name FROM users WHERE id = ?`, 1).Scan(&name)
	fmt.Println("found:", name, err)

	err = db.QueryRow(`SELECT name FROM users WHERE id = ?`, 99).Scan(&name)
	fmt.Println("missing is ErrNoRows:", errors.Is(err, sql.ErrNoRows))
}
```

**Output:**

```
found: Alice <nil>
missing is ErrNoRows: true
```

---

## 4. Query many rows

`🟢 easy` · *read*

The multi-row idiom has four parts you must get right every time: `Query`, **`defer rows.Close()`** (a leaked `*sql.Rows` holds a connection), the `for rows.Next() { rows.Scan(...) }` loop, and **`rows.Err()`** afterward (an error can surface *after* the loop ends).

**Steps:**

1. `rows, err := db.Query(...)`; `defer rows.Close()`.
2. Loop `rows.Next()`, `rows.Scan(...)` into a struct.
3. Check `rows.Err()` after the loop.

```go
package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type User struct {
	ID   int
	Name string
}

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO users (name) VALUES ('Alice'), ('Bob')`)

	// The full multi-row idiom: Query -> defer Close -> Next/Scan loop -> check Err.
	rows, err := db.Query(`SELECT id, name FROM users ORDER BY id`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			panic(err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil { // an error can surface AFTER the loop
		panic(err)
	}
	fmt.Printf("%+v\n", users)
}
```

**Output:**

```
[{ID:1 Name:Alice} {ID:2 Name:Bob}]
```

---

## 5. Always pass a context

`🟢 easy` · *context*

Every query should take a `context.Context` via the `*Context` methods. It enforces a **per-query timeout** and propagates **cancellation** — if the HTTP client hangs up, the in-flight query is cancelled instead of running to completion and wasting the connection.

**Steps:**

1. `context.WithTimeout` for a deadline; `ExecContext`/`QueryRowContext`.
2. A cancelled context makes the query fail immediately.
3. Prefer the `*Context` variants everywhere.

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE t (n INTEGER)`)

	// ALWAYS pass a context: it enforces a per-query timeout and propagates cancellation
	// (e.g. the client hung up). Use ExecContext / QueryContext / QueryRowContext.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `INSERT INTO t (n) VALUES (1)`)
	fmt.Println("insert err:", err)

	var n int
	db.QueryRowContext(ctx, `SELECT count(*) FROM t`).Scan(&n)
	fmt.Println("count:", n)

	// A cancelled context fails fast instead of running.
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	_, err = db.ExecContext(cctx, `INSERT INTO t (n) VALUES (2)`)
	fmt.Println("cancelled err:", err)
}
```

**Output:**

```
insert err: <nil>
count: 1
cancelled err: context canceled
```

---

## 6. Handle NULLs

`🟢 easy` · *null*

A NULL column **can't** scan into a plain `string`/`int` ("converting NULL to string is unsupported"). Use `sql.NullString`/`NullInt64` and check `.Valid`, or give NULL a default in SQL with `COALESCE`. Beware: **aggregates over zero rows return NULL too** — `SUM` of an empty set is NULL.

**Steps:**

1. Scan a nullable column into `sql.NullString`; branch on `.Valid`.
2. `COALESCE(col, '')` lets a plain string scan.
3. Scan an empty-set `SUM` into `sql.NullInt64` — it's not valid.

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
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	db.Exec(`INSERT INTO users (email) VALUES ('a@x.com'), (NULL)`)

	// A NULL column can't scan into a plain string -> use sql.NullString.
	rows, _ := db.Query(`SELECT id, email FROM users ORDER BY id`)
	defer rows.Close()
	for rows.Next() {
		var id int
		var email sql.NullString
		rows.Scan(&id, &email)
		if email.Valid {
			fmt.Printf("user %d: %s\n", id, email.String)
		} else {
			fmt.Printf("user %d: <no email>\n", id)
		}
	}

	// COALESCE gives NULL a default IN SQL, so a plain string scans fine.
	var email string
	db.QueryRow(`SELECT COALESCE(email, '') FROM users WHERE id = 2`).Scan(&email)
	fmt.Printf("coalesced: %q\n", email)

	// Aggregates over no rows return NULL too: SUM of an empty set is NULL.
	var total sql.NullInt64
	db.QueryRow(`SELECT SUM(id) FROM users WHERE id > 100`).Scan(&total)
	fmt.Println("sum of empty set valid:", total.Valid)
}
```

**Output:**

```
user 1: a@x.com
user 2: <no email>
coalesced: ""
sum of empty set valid: false
```

---

## 7. Prepared statements

`🟢 easy` · *prepare*

`db.Prepare` parses and plans a statement **once**, then executes it many times — cheaper for a hot query in a loop, and inherently parameterized (injection-safe). Close it when done (a prepared statement holds resources).

**Steps:**

1. `stmt, err := db.Prepare(...)`; `defer stmt.Close()`.
2. `stmt.Exec(args)` in a loop reuses the plan.
3. Verify the inserted rows.

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

	// Prepare once, execute many times — the DB parses/plans the statement a single
	// time. Prepared statements are also inherently parameterized (injection-safe).
	stmt, err := db.Prepare(`INSERT INTO nums (n) VALUES (?)`)
	if err != nil {
		panic(err)
	}
	defer stmt.Close()
	for i := 1; i <= 3; i++ {
		stmt.Exec(i * 10)
	}

	var count, sum int
	db.QueryRow(`SELECT count(*), SUM(n) FROM nums`).Scan(&count, &sum)
	fmt.Printf("rows=%d sum=%d\n", count, sum)
}
```

**Output:**

```
rows=3 sum=60
```

---

## 8. Named parameters

`🟢 easy` · *args*

Positional `?` placeholders get hard to track when a query has many arguments. `sql.Named` lets you use **named** placeholders (`:name`), which are self-documenting and can be reused in several spots.

**Steps:**

1. Pass `sql.Named("name", value)` args.
2. Reference them as `:name` in the SQL.
3. Read the row back with a named filter.

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
	db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)`)

	// sql.Named lets you use NAMED placeholders — clearer than counting ?s when a query
	// has many args (and you can reuse one arg in several spots).
	_, err := db.Exec(
		`INSERT INTO users (name, age) VALUES (:name, :age)`,
		sql.Named("name", "Alice"), sql.Named("age", 30),
	)
	if err != nil {
		panic(err)
	}
	var name string
	var age int
	db.QueryRow(`SELECT name, age FROM users WHERE name = :n`, sql.Named("n", "Alice")).Scan(&name, &age)
	fmt.Printf("%s is %d\n", name, age)
}
```

**Output:**

```
Alice is 30
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
