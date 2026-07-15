# Step 55 — Data Pipelines · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

Shaping parsed data with the [step 54](../54-collection-operations/) toolkit: **filter, map-to-DTO, group, aggregate, join, paginate**, plus scanning DB rows and streaming.

---

## 9. Filter parsed records

`🟡 medium` · *filter*

Once JSON is decoded into a `[]Struct`, it's an ordinary slice — filter it like any other. This is the whole point of parsing at the edge: your business logic never touches raw bytes or `map[string]any`.

**Steps:**

1. `json.Unmarshal` into `[]User`.
2. Loop and keep the records where `Active` is true.
3. Everything downstream works on plain structs.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func main() {
	data := []byte(`[
		{"name":"Alice","active":true},
		{"name":"Bob","active":false},
		{"name":"Carol","active":true}
	]`)
	var users []User
	json.Unmarshal(data, &users)

	// Once parsed, it's just a []struct — filter like any other slice.
	var active []User
	for _, u := range users {
		if u.Active {
			active = append(active, u)
		}
	}
	fmt.Println("active users:")
	for _, u := range active {
		fmt.Println(" ", u.Name)
	}
}
```

**Output:**

```
active users:
  Alice
  Carol
```

---

## 10. Map records to DTOs

`🟡 medium` · *dto*

A DTO (data transfer object) is the shape you expose, separate from your internal model. Mapping model → DTO is where you rename fields, compute derived values (full name), normalize (lowercase email), and — critically — **drop internal fields** so secrets never reach the wire.

**Steps:**

1. Internal `user` has an `Internal` field that must not leak.
2. `toDTO` builds the public `UserDTO` with only safe, shaped fields.
3. Marshal the `[]UserDTO` — the internal field is simply absent.

```go
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Internal model (from the DB/source).
type user struct {
	FirstName string
	LastName  string
	Email     string
	Internal  string // must NOT leak to the API
}

// Public DTO (what the API returns).
type UserDTO struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

func toDTO(u user) UserDTO {
	return UserDTO{
		FullName: strings.TrimSpace(u.FirstName + " " + u.LastName),
		Email:    strings.ToLower(u.Email),
	}
}

func main() {
	users := []user{
		{"Alice", "Adams", "Alice@X.com", "ssn-111"},
		{"Bob", "Brown", "BOB@X.com", "ssn-222"},
	}
	dtos := make([]UserDTO, len(users))
	for i, u := range users {
		dtos[i] = toDTO(u)
	}
	b, _ := json.MarshalIndent(dtos, "", "  ")
	fmt.Println(string(b))
}
```

**Output:**

```
[
  {
    "full_name": "Alice Adams",
    "email": "alice@x.com"
  },
  {
    "full_name": "Bob Brown",
    "email": "bob@x.com"
  }
]
```

---

## 11. Group parsed records by a field

`🟡 medium` · *group-by*

Group-by (step 54) applied to parsed data: bucket transactions into `map[string][]Txn` by category. `slices.Sorted(maps.Keys(m))` gives the deterministic key order every map-derived output needs.

**Steps:**

1. Decode the JSON array of transactions.
2. `byCat[t.Category] = append(byCat[t.Category], t)`.
3. Iterate sorted keys and report each bucket's size.

```go
package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

type Txn struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

func main() {
	data := []byte(`[
		{"category":"food","amount":12.5},
		{"category":"toys","amount":30},
		{"category":"food","amount":8.25},
		{"category":"food","amount":5}
	]`)
	var txns []Txn
	json.Unmarshal(data, &txns)

	byCat := map[string][]Txn{}
	for _, t := range txns {
		byCat[t.Category] = append(byCat[t.Category], t)
	}
	for _, cat := range slices.Sorted(maps.Keys(byCat)) {
		fmt.Printf("%s: %d txns\n", cat, len(byCat[cat]))
	}
}
```

**Output:**

```
food: 3 txns
toys: 1 txns
```

---

## 12. Aggregate per group

`🟡 medium` · *aggregate*

Group + reduce in one pass: keep parallel `sum` and `count` maps keyed by category, then derive the average. This is the shape of nearly every "totals by X" report.

**Steps:**

1. `sum[cat] += amount` and `count[cat]++` together.
2. Sort the keys for stable output.
3. Average is `sum / count` per group.

```go
package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

type Txn struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

func main() {
	data := []byte(`[
		{"category":"food","amount":12.5},
		{"category":"toys","amount":30},
		{"category":"food","amount":8.25},
		{"category":"food","amount":5}
	]`)
	var txns []Txn
	json.Unmarshal(data, &txns)

	// Two parallel maps: running sum and running count per category.
	sum := map[string]float64{}
	count := map[string]int{}
	for _, t := range txns {
		sum[t.Category] += t.Amount
		count[t.Category]++
	}
	fmt.Printf("%-6s %5s %8s %8s\n", "CAT", "N", "SUM", "AVG")
	for _, cat := range slices.Sorted(maps.Keys(sum)) {
		fmt.Printf("%-6s %5d %8.2f %8.2f\n", cat, count[cat], sum[cat], sum[cat]/float64(count[cat]))
	}
}
```

**Output:**

```
CAT        N      SUM      AVG
food       3    25.75     8.58
toys       1    30.00    30.00
```

---

## 13. Join two datasets on a key

`🟡 medium` · *join*

Combining two datasets (the SQL `JOIN` you do in code) is a **hash join**: index the lookup side into a `map[K]T` once, then enrich each row of the other side in O(1). Beats a nested loop (O(n·m)).

**Steps:**

1. Build `byID` from customers, keyed by `ID`.
2. For each order, look up `byID[o.CustomerID]`.
3. Missing keys return the zero value — guard if that matters.

```go
package main

import "fmt"

type Order struct {
	ID         int
	CustomerID int
	Amount     int
}

type Customer struct {
	ID   int
	Name string
}

func main() {
	customers := []Customer{{1, "Alice"}, {2, "Bob"}}
	orders := []Order{
		{101, 1, 500},
		{102, 2, 300},
		{103, 1, 200},
	}
	// A hash join: index the lookup side by key, then enrich each order in O(1).
	byID := make(map[int]Customer, len(customers))
	for _, c := range customers {
		byID[c.ID] = c
	}
	for _, o := range orders {
		name := byID[o.CustomerID].Name
		fmt.Printf("order %d: %s spent %d\n", o.ID, name, o.Amount)
	}
}
```

**Output:**

```
order 101: Alice spent 500
order 102: Bob spent 300
order 103: Alice spent 200
```

---

## 14. Sort and paginate

`🟡 medium` · *pagination*

Serving results in pages is sort + slice. A generic `page` helper returns the window `s[offset:offset+limit]`, clamped so the last (short) page and past-the-end offsets don't panic.

**Steps:**

1. Sort by the field you page on (price DESC here).
2. `page(s, offset, limit)` returns `s[offset:min(offset+limit, len)]`, or `nil` past the end.
3. Loop pages until you get an empty one.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type Item struct {
	Name  string
	Price int
}

// page returns the slice window [offset, offset+limit).
func page[T any](s []T, offset, limit int) []T {
	if offset >= len(s) {
		return nil
	}
	return s[offset:min(offset+limit, len(s))]
}

func main() {
	items := []Item{
		{"pen", 150}, {"notebook", 450}, {"eraser", 75},
		{"stapler", 900}, {"marker", 200},
	}
	// Sort by price DESC, then serve fixed-size pages.
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(b.Price, a.Price) })
	for pageNum := 0; ; pageNum++ {
		p := page(items, pageNum*2, 2)
		if len(p) == 0 {
			break
		}
		fmt.Printf("page %d: %v\n", pageNum, p)
	}
}
```

**Output:**

```
page 0: [{stapler 900} {notebook 450}]
page 1: [{marker 200} {pen 150}]
page 2: [{eraser 75}]
```

---

## 15. Scan database rows into structs

`🟡 medium` · *database/sql*

The universal DB idiom: `for rows.Next() { rows.Scan(&f1, &f2…) }`, then check `rows.Err()`. `Scan` fills by **position**, matching your `SELECT` column order. After the loop you hold a `[]Struct` — everything downstream is plain slice work. (Here `mockRows` mirrors `*sql.Rows` so the loop is real; in production the rows come from `db.QueryContext`, and you'd `defer rows.Close()`.)

**Steps:**

1. `rows.Next()` advances; returns false when done.
2. `rows.Scan(&u.ID, &u.Name)` fills fields **in SELECT order**.
3. Check `rows.Err()` after the loop — a mid-iteration error surfaces there.

```go
package main

import "fmt"

// mockRows mirrors the shape of *sql.Rows so the scan LOOP below is exactly what
// you'd write against a real database (rows come from db.QueryContext there).
type mockRows struct {
	data [][]any
	i    int
}

func (r *mockRows) Next() bool { r.i++; return r.i <= len(r.data) }
func (r *mockRows) Scan(dest ...any) error {
	row := r.data[r.i-1]
	for k, d := range dest {
		switch p := d.(type) {
		case *int:
			*p = row[k].(int)
		case *string:
			*p = row[k].(string)
		}
	}
	return nil
}
func (r *mockRows) Err() error { return nil }

type User struct {
	ID   int
	Name string
}

func main() {
	rows := &mockRows{data: [][]any{
		{1, "Alice"},
		{2, "Bob"},
	}}
	// The canonical scan idiom: Next -> Scan into fields -> check Err at the end.
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			panic(err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
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

## 16. Read NDJSON (JSON Lines)

`🟡 medium` · *streaming*

NDJSON (a.k.a. JSON Lines) is one JSON object per line — the format of logs, exports, and streaming APIs. A single `json.Decoder` reads them in sequence from any reader; `dec.More()` reports whether another value follows.

**Steps:**

1. `json.NewDecoder` over the reader.
2. Loop while `dec.More()`, `Decode` into a fresh value each time.
3. No outer array brackets — each line is an independent object.

```go
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Log struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

func main() {
	// NDJSON / JSON Lines: one JSON object per line. A json.Decoder reads them in
	// sequence from any io.Reader; dec.More() reports whether another value follows.
	const raw = `{"level":"info","msg":"started"}
{"level":"error","msg":"boom"}
{"level":"info","msg":"done"}`
	dec := json.NewDecoder(strings.NewReader(raw))
	for dec.More() {
		var l Log
		if err := dec.Decode(&l); err != nil {
			panic(err)
		}
		fmt.Printf("[%s] %s\n", l.Level, l.Msg)
	}
}
```

**Output:**

```
[info] started
[error] boom
[info] done
```

---

## 17. Stream a large JSON array

`🟡 medium` · *streaming*

To process a huge JSON **array** without loading it all into memory, decode element-by-element: read the opening `[` token, `Decode` each element as `More()` reports one, then read the closing `]`. Memory stays flat at one element regardless of array size.

**Steps:**

1. `dec.Token()` consumes the opening `[`.
2. Loop `dec.More()`, `Decode` each element.
3. `dec.Token()` consumes the closing `]`.

```go
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Event struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}

func main() {
	const raw = `[
		{"id":1,"type":"click"},
		{"id":2,"type":"view"},
		{"id":3,"type":"click"}
	]`
	// Stream a large JSON ARRAY element-by-element instead of loading it all:
	// read the opening '[' token, Decode each element, then the closing ']'.
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.Token() // consume '['
	count := 0
	for dec.More() {
		var e Event
		if err := dec.Decode(&e); err != nil {
			panic(err)
		}
		count++
		fmt.Printf("event %d: %s\n", e.ID, e.Type)
	}
	dec.Token() // consume ']'
	fmt.Println("total:", count)
}
```

**Output:**

```
event 1: click
event 2: view
event 3: click
total: 3
```

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
