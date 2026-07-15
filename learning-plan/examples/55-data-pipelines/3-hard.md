# Step 55 — Data Pipelines · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

Full pipelines: reports, multi-metric aggregation, pivots, cross-source joins, dynamic/custom JSON, lazy record iterators, and an end-to-end ETL.

---

## 18. Report: filter, group by month, sum

`🔴 hard` · *report*

A real report chains several stages: decode orders, **filter** to paid, **derive** a group key (the `YYYY-MM` month from a parsed date), **group + sum**, and emit in key order. Deriving the group key from a parsed `time.Time` is the new move here.

**Steps:**

1. Filter `Paid`.
2. `time.Parse` the date, key by `t.Format("2006-01")`.
3. Sum into `byMonth`, print sorted keys.

```go
package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"
)

type Order struct {
	Date   string  `json:"date"`
	Paid   bool    `json:"paid"`
	Amount float64 `json:"amount"`
}

func main() {
	data := []byte(`[
		{"date":"2026-01-15","paid":true,"amount":100},
		{"date":"2026-01-20","paid":false,"amount":50},
		{"date":"2026-02-03","paid":true,"amount":200},
		{"date":"2026-01-28","paid":true,"amount":75},
		{"date":"2026-02-14","paid":true,"amount":25}
	]`)
	var orders []Order
	json.Unmarshal(data, &orders)

	// Filter paid -> group by "YYYY-MM" (derived from the parsed date) -> sum.
	byMonth := map[string]float64{}
	for _, o := range orders {
		if !o.Paid {
			continue
		}
		t, err := time.Parse("2006-01-02", o.Date)
		if err != nil {
			continue
		}
		byMonth[t.Format("2006-01")] += o.Amount
	}
	for _, m := range slices.Sorted(maps.Keys(byMonth)) {
		fmt.Printf("%s: %.2f\n", m, byMonth[m])
	}
}
```

**Output:**

```
2026-01: 175.00
2026-02: 225.00
```

---

## 19. Multi-metric aggregate per group

`🔴 hard` · *aggregate*

When a group needs several metrics at once (count, sum, min, max, avg), give each group a **single accumulator struct** instead of a map per metric. Seed `Min`/`Max` with `+Inf`/`-Inf` so the first value always wins.

**Steps:**

1. `map[string]*Stats`; create-and-seed on first sight of a key.
2. Update `Count`/`Sum`/`Min`/`Max` per reading with `math.Min`/`math.Max`.
3. Derive avg = `Sum/Count` at print time; sort keys.

```go
package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

type Reading struct {
	Sensor string
	Value  float64
}

// One accumulator struct per group carries every metric at once.
type Stats struct {
	Count    int
	Sum      float64
	Min, Max float64
}

func main() {
	readings := []Reading{
		{"a", 10}, {"b", 5}, {"a", 20}, {"a", 15}, {"b", 25},
	}
	agg := map[string]*Stats{}
	for _, r := range readings {
		s := agg[r.Sensor]
		if s == nil {
			s = &Stats{Min: math.Inf(1), Max: math.Inf(-1)}
			agg[r.Sensor] = s
		}
		s.Count++
		s.Sum += r.Value
		s.Min = math.Min(s.Min, r.Value)
		s.Max = math.Max(s.Max, r.Value)
	}
	fmt.Printf("%-7s %3s %6s %5s %5s %6s\n", "SENSOR", "N", "SUM", "MIN", "MAX", "AVG")
	for _, k := range slices.Sorted(maps.Keys(agg)) {
		s := agg[k]
		fmt.Printf("%-7s %3d %6.1f %5.1f %5.1f %6.1f\n",
			k, s.Count, s.Sum, s.Min, s.Max, s.Sum/float64(s.Count))
	}
}
```

**Output:**

```
SENSOR    N    SUM   MIN   MAX    AVG
a         3   45.0  10.0  20.0   15.0
b         2   30.0   5.0  25.0   15.0
```

---

## 20. Pivot a table (crosstab)

`🔴 hard` · *pivot*

A pivot turns rows into a grid: `map[rowKey]map[colKey]value`. Track the set of columns seen so the header is complete and sorted; a missing cell reads as the zero value. This is the "spreadsheet pivot table" in ~20 lines.

**Steps:**

1. `pivot[region][product] += units`, initializing the inner map on first use.
2. Collect the column set separately and sort it.
3. Print a header row, then one row per (sorted) region filling each column.

```go
package main

import (
	"fmt"
	"maps"
	"slices"
)

type Sale struct {
	Region  string
	Product string
	Units   int
}

func main() {
	sales := []Sale{
		{"west", "pen", 3}, {"east", "pen", 5}, {"west", "ink", 2},
		{"east", "ink", 1}, {"west", "pen", 4},
	}
	// Pivot / crosstab: rows=region, cols=product, cell=sum(units).
	pivot := map[string]map[string]int{}
	products := map[string]struct{}{}
	for _, s := range sales {
		if pivot[s.Region] == nil {
			pivot[s.Region] = map[string]int{}
		}
		pivot[s.Region][s.Product] += s.Units
		products[s.Product] = struct{}{}
	}

	cols := slices.Sorted(maps.Keys(products))
	fmt.Printf("%-6s", "REGION")
	for _, c := range cols {
		fmt.Printf(" %5s", c)
	}
	fmt.Println()
	for _, region := range slices.Sorted(maps.Keys(pivot)) {
		fmt.Printf("%-6s", region)
		for _, c := range cols {
			fmt.Printf(" %5d", pivot[region][c])
		}
		fmt.Println()
	}
}
```

**Output:**

```
REGION   ink   pen
east       1     5
west       2     7
```

---

## 21. Enrich JSON with a CSV lookup

`🔴 hard` · *join*

Real pipelines combine sources: a CSV price list and JSON orders. Load the CSV into a `map[sku]price` lookup, then enrich each parsed order — a hash join across two *formats*. `fmt.Sscanf` is a quick way to pull an int out of a CSV cell.

**Steps:**

1. Parse the CSV into a `prices` lookup map.
2. Decode the JSON orders.
3. For each order, multiply the looked-up price by qty for a line total.

```go
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
)

type Order struct {
	ID  int    `json:"id"`
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

func main() {
	// Source 1: a CSV price list (the lookup table).
	const priceCSV = `sku,price
A-1,150
B-2,900`
	prices := map[string]int{}
	r := csv.NewReader(strings.NewReader(priceCSV))
	rows, _ := r.ReadAll()
	for _, row := range rows[1:] {
		var p int
		fmt.Sscanf(row[1], "%d", &p)
		prices[row[0]] = p
	}

	// Source 2: JSON orders. Enrich each with the CSV price and a line total.
	const ordersJSON = `[
		{"id":1,"sku":"A-1","qty":2},
		{"id":2,"sku":"B-2","qty":1}
	]`
	var orders []Order
	json.Unmarshal([]byte(ordersJSON), &orders)
	for _, o := range orders {
		total := prices[o.SKU] * o.Qty
		fmt.Printf("order %d: %d x %s @ %d = %d\n", o.ID, o.Qty, o.SKU, prices[o.SKU], total)
	}
}
```

**Output:**

```
order 1: 2 x A-1 @ 150 = 300
order 2: 1 x B-2 @ 900 = 900
```

---

## 22. Decode dynamic JSON

`🔴 hard` · *dynamic*

When you don't control the schema, decode loosely. `map[string]any` gives you the tree — but remember every JSON number is a `float64`, arrays are `[]any`, objects are `map[string]any`; type-assert as you descend. `json.RawMessage` defers decoding a sub-object until you've read a discriminator (tagged unions).

**Steps:**

1. Decode into `map[string]any`; assert `.(string)`, `.(float64)`, `.([]any)`.
2. For a tagged payload, keep it as `json.RawMessage`.
3. Read the `type`, then decode the raw payload into the right concrete struct.

```go
package main

import (
	"encoding/json"
	"fmt"
)

func main() {
	// When the schema is unknown, decode into map[string]any and type-assert.
	raw := []byte(`{"name":"widget","price":9.99,"tags":["a","b"],"active":true}`)
	var m map[string]any
	json.Unmarshal(raw, &m)

	// JSON numbers decode to float64; arrays to []any; objects to map[string]any.
	name := m["name"].(string)
	price := m["price"].(float64)
	tags := m["tags"].([]any)
	fmt.Printf("%s costs %.2f with %d tags\n", name, price, len(tags))

	// json.RawMessage defers decoding a sub-part until you know its concrete type.
	var env struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	json.Unmarshal([]byte(`{"type":"point","payload":{"x":1,"y":2}}`), &env)
	if env.Type == "point" {
		var pt struct{ X, Y int }
		json.Unmarshal(env.Payload, &pt)
		fmt.Printf("point: %+v\n", pt)
	}
}
```

**Output:**

```
widget costs 9.99 with 2 tags
point: {X:1 Y:2}
```

---

## 23. Custom JSON marshaling

`🔴 hard` · *marshaler*

When a field's JSON form differs from its Go form — money stored as integer cents but sent as a decimal string `"12.50"` — implement `MarshalJSON`/`UnmarshalJSON` on the type. `json` calls them automatically wherever the type appears, so the rest of your structs stay clean.

**Steps:**

1. `MarshalJSON` on `Money` (value receiver): cents → quoted `"%.2f"`.
2. `UnmarshalJSON` on `*Money` (pointer receiver, so it can assign): string → cents.
3. Round-trip `"12.50"` ⇄ `1250` through an `Invoice`.

```go
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Money is stored as integer cents but serialized as a decimal string "12.50".
type Money int

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(fmt.Sprintf("%.2f", float64(m)/100))), nil
}

func (m *Money) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*m = Money(f*100 + 0.5) // round to the nearest cent
	return nil
}

type Invoice struct {
	ID    int   `json:"id"`
	Total Money `json:"total"`
}

func main() {
	// Decode "12.50" -> 1250 cents.
	var inv Invoice
	json.Unmarshal([]byte(`{"id":1,"total":"12.50"}`), &inv)
	fmt.Println("cents:", int(inv.Total))

	// Encode 1250 cents -> "12.50".
	b, _ := json.Marshal(inv)
	fmt.Println(string(b))
}
```

**Output:**

```
cents: 1250
{"id":1,"total":"12.50"}
```

---

## 24. A lazy record iterator

`🔴 hard` · *iterators*

Wrap a `csv.Reader` in an `iter.Seq[Row]` (Go 1.23) that reads and parses **one row at a time** — the caller ranges over records without the whole file ever being in memory. This is the streaming counterpart to step 54's lazy slice pipelines, applied to I/O.

**Steps:**

1. `records` returns `iter.Seq[Row]`; inside, skip the header, then `Read` in a loop.
2. `io.EOF` (or any error) ends the sequence; `yield` returning false stops it early.
3. The consumer ranges the sequence, filtering as it goes.

```go
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
)

type Row struct {
	Name string
	Age  int
}

// records lazily yields parsed rows from a CSV reader — one Read at a time, never
// loading the whole file into memory. Reaching EOF (or an error) ends the sequence.
func records(r *csv.Reader) iter.Seq[Row] {
	return func(yield func(Row) bool) {
		r.Read() // skip header
		for {
			rec, err := r.Read()
			if err == io.EOF || err != nil {
				return
			}
			var age int
			fmt.Sscanf(rec[1], "%d", &age)
			if !yield(Row{Name: rec[0], Age: age}) {
				return
			}
		}
	}
}

func main() {
	const raw = `name,age
Alice,30
Bob,25
Carol,40`
	seq := records(csv.NewReader(strings.NewReader(raw)))

	// Consume the lazy sequence, filtering as we go.
	var names []string
	for row := range seq {
		if row.Age >= 30 {
			names = append(names, row.Name)
		}
	}
	slices.Sort(names)
	fmt.Println("30+:", names)
}
```

**Output:**

```
30+: [Alice Carol]
```

---

## 25. Streaming NDJSON transform

`🔴 hard` · *streaming*

An ETL that never holds more than one record: read NDJSON with a `bufio.Scanner` (line by line), transform each object, and write the result with a `json.Encoder` (which appends a newline per value). Input can be gigabytes; memory stays constant.

**Steps:**

1. `bufio.NewScanner` over the input; `sc.Scan()` per line.
2. Unmarshal the line into `In`, build the transformed `Out`.
3. `enc.Encode(out)` streams each result line straight to stdout.

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type In struct {
	Name  string `json:"name"`
	Cents int    `json:"cents"`
}
type Out struct {
	Name    string  `json:"name"`
	Dollars float64 `json:"dollars"`
}

func main() {
	// Read NDJSON line-by-line and write transformed NDJSON — constant memory,
	// no matter how large the input stream.
	const raw = `{"name":"a","cents":150}
{"name":"b","cents":2999}
{"name":"c","cents":75}`
	sc := bufio.NewScanner(strings.NewReader(raw))
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var in In
		if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
			continue
		}
		enc.Encode(Out{Name: in.Name, Dollars: float64(in.Cents) / 100})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
```

**Output:**

```
{"name":"a","dollars":1.5}
{"name":"b","dollars":29.99}
{"name":"c","dollars":0.75}
```

---

## 26. Capstone: a CSV to JSON ETL

`🔴 hard` · *capstone*

The whole lesson in one program — **Extract** (parse CSV, validate/skip bad rows), **Transform** (filter settled, group by category, aggregate count + total), **Load** (emit a sorted JSON summary). This is the shape of a nightly report job or an import endpoint.

**Steps:**

1. Parse each CSV row; `ParseFloat` failure → skip (validate).
2. Keep `settled` rows; fold into `map[category]*CategorySummary`.
3. Emit the summaries in sorted category order as indented JSON.

```go
package main

import (
	"encoding/csv"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Txn struct {
	Category string
	Amount   float64
	Status   string
}

type CategorySummary struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Total    float64 `json:"total"`
}

func main() {
	const raw = `category,amount,status
food,12.50,settled
toys,30.00,settled
food,8.25,refunded
food,5.00,settled
books,20.00,settled
toys,15.00,settled`

	// 1. EXTRACT + parse each row (skipping malformed ones).
	r := csv.NewReader(strings.NewReader(raw))
	rows, err := r.ReadAll()
	if err != nil {
		panic(err)
	}
	var txns []Txn
	for _, row := range rows[1:] {
		amt, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue // validate: drop rows with a bad amount
		}
		txns = append(txns, Txn{Category: row[0], Amount: amt, Status: row[2]})
	}

	// 2. TRANSFORM: keep settled, group by category, aggregate count+total.
	agg := map[string]*CategorySummary{}
	for _, t := range txns {
		if t.Status != "settled" {
			continue
		}
		s := agg[t.Category]
		if s == nil {
			s = &CategorySummary{Category: t.Category}
			agg[t.Category] = s
		}
		s.Count++
		s.Total += t.Amount
	}

	// 3. LOAD: emit a sorted JSON summary report.
	out := make([]CategorySummary, 0, len(agg))
	for _, cat := range slices.Sorted(maps.Keys(agg)) {
		out = append(out, *agg[cat])
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
```

**Output:**

```
[
  {
    "category": "books",
    "count": 1,
    "total": 20
  },
  {
    "category": "food",
    "count": 2,
    "total": 17.5
  },
  {
    "category": "toys",
    "count": 2,
    "total": 45
  }
]
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
