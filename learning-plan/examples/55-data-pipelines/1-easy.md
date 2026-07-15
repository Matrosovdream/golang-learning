# Step 55 — Data Pipelines · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

Getting external data **in and out**: JSON and CSV parsing, struct tags, string conversions, and NULLs.

---

## 1. Decode a JSON array

`🟢 easy` · *json*

The most common decode: a JSON array of objects straight into a `[]Struct`. Field tags (`json:"id"`) map JSON keys to Go fields, and unexported/unmatched keys are ignored. Pass a **pointer** to `Unmarshal` so it can fill your slice.

**Steps:**

1. Define the struct with `json:"..."` tags.
2. `json.Unmarshal(data, &users)` — note the `&`.
3. Always check the returned error.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	data := []byte(`[
		{"id": 1, "name": "Alice", "email": "alice@example.com"},
		{"id": 2, "name": "Bob", "email": "bob@example.com"}
	]`)
	// Unmarshal a JSON array straight into a slice of structs; tags map fields.
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		panic(err)
	}
	fmt.Println("count:", len(users))
	for _, u := range users {
		fmt.Printf("%d %s <%s>\n", u.ID, u.Name, u.Email)
	}
}
```

**Output:**

```
count: 2
1 Alice <alice@example.com>
2 Bob <bob@example.com>
```

---

## 2. Encode structs to JSON

`🟢 easy` · *json*

The reverse: `json.Marshal` produces compact JSON for the wire; `json.MarshalIndent` produces indented JSON for humans and logs. Note that a Go `float64` of `1.50` marshals as `1.5` — JSON has one number type.

**Steps:**

1. `json.Marshal(p)` → compact bytes.
2. `json.MarshalIndent(p, "", "  ")` → two-space-indented bytes.
3. Wrap with `string(...)` to print.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Product struct {
	Name  string   `json:"name"`
	Price float64  `json:"price"`
	Tags  []string `json:"tags"`
}

func main() {
	p := Product{Name: "Pen", Price: 1.50, Tags: []string{"office", "stationery"}}

	// Compact JSON (for the wire).
	b, _ := json.Marshal(p)
	fmt.Println(string(b))

	// Pretty (indented) JSON (for humans/logs).
	pretty, _ := json.MarshalIndent(p, "", "  ")
	fmt.Println(string(pretty))
}
```

**Output:**

```
{"name":"Pen","price":1.5,"tags":["office","stationery"]}
{
  "name": "Pen",
  "price": 1.5,
  "tags": [
    "office",
    "stationery"
  ]
}
```

---

## 3. Struct tags for optional fields

`🟢 easy` · *json*

Three tag features cover almost every "optional field" case: `,omitempty` drops a field when it holds its zero value; `json:"-"` never serializes it (keep secrets out of responses); and a **pointer** field lets you tell "absent" (`nil` → omitted or `null`) apart from "present and zero".

**Steps:**

1. `Nickname` is empty → `,omitempty` drops it.
2. `Password` has `json:"-"` → never appears.
3. `Balance` is a `*int` pointing at `0` → appears as `0` (a plain `int` with `omitempty` couldn't).

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Account struct {
	ID       int    `json:"id"`
	Nickname string `json:"nickname,omitempty"` // dropped when empty
	Password string `json:"-"`                  // never serialized
	Balance  *int   `json:"balance"`            // pointer: tells 0 apart from absent
}

func main() {
	bal := 0
	a := Account{ID: 7, Password: "secret", Balance: &bal}
	b, _ := json.MarshalIndent(a, "", "  ")
	fmt.Println(string(b))
}
```

**Output:**

```
{
  "id": 7,
  "balance": 0
}
```

---

## 4. Read a CSV file

`🟢 easy` · *csv*

`encoding/csv` reads from any `io.Reader` (a file, a string, an HTTP body). `ReadAll` returns `[][]string` — every cell is a string. The first row is usually a header you handle separately.

**Steps:**

1. `csv.NewReader(strings.NewReader(raw))`.
2. `ReadAll()` → rows of strings; check the error.
3. `records[0]` is the header; `records[1:]` are the data rows.

```go
package main

import (
	"encoding/csv"
	"fmt"
	"strings"
)

func main() {
	const raw = `id,name,age
1,Alice,30
2,Bob,25`
	// csv.NewReader reads from any io.Reader; ReadAll returns [][]string.
	r := csv.NewReader(strings.NewReader(raw))
	records, err := r.ReadAll()
	if err != nil {
		panic(err)
	}
	fmt.Println("header:", records[0])
	for _, row := range records[1:] { // skip the header row
		fmt.Println("row:", row)
	}
}
```

**Output:**

```
header: [id name age]
row: [1 Alice 30]
row: [2 Bob 25]
```

---

## 5. CSV rows to structs

`🟢 easy` · *csv*

CSV cells are positional strings; to get typed structs, build a **header → column-index map** so you address columns by name (robust to reordering), then convert each cell with `strconv`.

**Steps:**

1. Build `col := map[string]int{}` from the header row.
2. For each data row, look up cells by name: `row[col["name"]]`.
3. Convert numeric cells with `strconv.Atoi`.

```go
package main

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

type Person struct {
	ID   int
	Name string
	Age  int
}

func main() {
	const raw = `id,name,age
1,Alice,30
2,Bob,25`
	r := csv.NewReader(strings.NewReader(raw))
	rows, _ := r.ReadAll()

	// Build a header -> column index map so the file's column order doesn't matter.
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}

	var people []Person
	for _, row := range rows[1:] {
		id, _ := strconv.Atoi(row[col["id"]])
		age, _ := strconv.Atoi(row[col["age"]])
		people = append(people, Person{ID: id, Name: row[col["name"]], Age: age})
	}
	for _, p := range people {
		fmt.Printf("%+v\n", p)
	}
}
```

**Output:**

```
{ID:1 Name:Alice Age:30}
{ID:2 Name:Bob Age:25}
```

---

## 6. Parse values from strings

`🟢 easy` · *strconv*

Data from CSV, query params, env vars, and forms all arrive as strings. `strconv.Atoi`/`ParseFloat`/`ParseBool` and `time.Parse` convert them — and each returns an **error** (not a panic) so a bad value is recoverable. `time`'s layout is the reference date `2006-01-02 15:04:05`.

**Steps:**

1. `Atoi` / `ParseFloat` / `ParseBool` for numbers and booleans.
2. `time.Parse("2006-01-02", s)` for a date, then reformat with `Format`.
3. A bad input returns a descriptive error — check it.

```go
package main

import (
	"fmt"
	"strconv"
	"time"
)

func main() {
	// External data arrives as strings — convert with strconv and time.Parse.
	n, err := strconv.Atoi("42")
	fmt.Println("int:", n, err)

	f, _ := strconv.ParseFloat("3.14", 64)
	fmt.Println("float:", f)

	b, _ := strconv.ParseBool("true")
	fmt.Println("bool:", b)

	// The reference date is always Mon Jan 2 15:04:05 MST 2006.
	t, _ := time.Parse("2006-01-02", "2026-07-15")
	fmt.Println("date:", t.Format("Jan 2, 2006"))

	// A bad conversion returns an error, not a panic — always check it.
	_, err = strconv.Atoi("notanumber")
	fmt.Println("err:", err)
}
```

**Output:**

```
int: 42 <nil>
float: 3.14
bool: true
date: Jul 15, 2026
err: strconv.Atoi: parsing "notanumber": invalid syntax
```

---

## 7. Handle NULL and optional values

`🟢 easy` · *nulls*

A missing value is **not** the zero value — treating a NULL email as `""` corrupts your data. From a database, a nullable column is `sql.NullString{String, Valid}`; check `Valid`. In JSON, model an optional field as a **pointer** so `nil` marshals to `null` and a present value marshals normally.

**Steps:**

1. `sql.NullString` carries a value plus a `Valid` flag; branch on `Valid`.
2. For JSON, a `*string` field marshals `nil` → `null`.
3. Compare the two "no email" representations.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type Row struct {
	Name  string
	Email sql.NullString // a nullable DB column
}

func main() {
	rows := []Row{
		{Name: "Alice", Email: sql.NullString{String: "a@x.com", Valid: true}},
		{Name: "Bob", Email: sql.NullString{Valid: false}}, // NULL
	}
	for _, r := range rows {
		if r.Email.Valid {
			fmt.Printf("%s: %s\n", r.Name, r.Email.String)
		} else {
			fmt.Printf("%s: <no email>\n", r.Name)
		}
	}

	// In JSON a nullable field is usually a pointer: nil marshals to null.
	type DTO struct {
		Name  string  `json:"name"`
		Email *string `json:"email"`
	}
	email := "a@x.com"
	out := []DTO{{"Alice", &email}, {"Bob", nil}}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}
```

**Output:**

```
Alice: a@x.com
Bob: <no email>
[{"name":"Alice","email":"a@x.com"},{"name":"Bob","email":null}]
```

---

## 8. Write CSV from structs

`🟢 easy` · *csv*

`csv.Writer` writes `[]string` rows to any `io.Writer`. Convert each field to a string (`strconv.Itoa`, `FormatFloat`, …) and write row by row. It **buffers**, so you must `Flush` (or `defer w.Flush()`) or the output is empty.

**Steps:**

1. `csv.NewWriter(os.Stdout)`.
2. `w.Write(header)` then a `w.Write(...)` per record.
3. `w.Flush()` — the easiest CSV bug to hit is forgetting this.

```go
package main

import (
	"encoding/csv"
	"os"
	"strconv"
)

type Sale struct {
	Product string
	Qty     int
}

func main() {
	sales := []Sale{{"pen", 3}, {"notebook", 1}}
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"product", "qty"}) // header
	for _, s := range sales {
		w.Write([]string{s.Product, strconv.Itoa(s.Qty)})
	}
	w.Flush() // don't forget — the writer buffers
}
```

**Output:**

```
product,qty
pen,3
notebook,1
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
