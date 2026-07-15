# 55 — Data Pipelines: Parsing & Shaping JSON, CSV & DB Rows

> Part of **Part 10 — Data Structures & Algorithms**, the practical **data-wrangling pair** with [54 — Collection Operations](54-collection-operations.md). Where 54 teaches the *operations* (filter/map/reduce/group/set) on in-memory slices, this lesson **applies them to real external data**: parsing JSON, CSV, and database rows into Go structs, then shaping them into report DTOs. Builds on [08 — Strings](08-strings.md), [09 — Structs](09-structs.md), [19 — Standard Library Tour](19-stdlib-tour.md), [22 — Persistence with database/sql](22-database.md), and the whole toolkit from [54](54-collection-operations.md). Thesis: **the hard part of "data work" is at the edges — decode messy input into clean structs, then it's just a `[]T` and the [54](54-collection-operations.md) toolkit does the rest; encode the result back out.**

## Goals
- Parse the three data sources every backend touches: **JSON** (`encoding/json` — `Unmarshal`, struct tags, `Decoder` streaming), **CSV** (`encoding/csv` — header-indexed rows → structs), and **database rows** (`rows.Next()`/`Scan()` → structs).
- Handle the realities of external data: **type conversion** from strings (`strconv`, `time.Parse`), **optional / NULL** values (`omitempty`, pointers, `sql.NullString`), and **unknown schemas** (`map[string]any`, `json.RawMessage`, custom `Marshaler`).
- Apply the [54](54-collection-operations.md) toolkit to parsed data: **filter → map to DTO → group → aggregate → sort**, plus **join** (hash-join two datasets), **pivot** (crosstab), and **paginate**.
- Keep memory flat on large inputs with **streaming**: `json.Decoder` over an array, NDJSON line-by-line, and a **lazy `iter.Seq` record reader**.
- Assemble an end-to-end **ETL**: extract (parse) → transform (validate/filter/group/aggregate) → load (emit JSON).

## Concepts

- **JSON in and out is `Unmarshal`/`Marshal` + struct tags.** A JSON array decodes straight into a `[]Struct`; field tags (`json:"name"`) map keys. `MarshalIndent` pretty-prints. This is the workhorse — most APIs are a decode, some slice work, and an encode.
  ```go
  var users []User
  json.Unmarshal(data, &users)     // []struct in
  out, _ := json.Marshal(dtos)     // []DTO out
  ```
- **Struct tags encode the optional-field rules:** `,omitempty` drops zero values from the output; `json:"-"` never serializes a field (keep secrets out of responses); a **pointer** field (`*int`) distinguishes "absent/null" from "present and zero". These three cover almost every "optional field" question.
- **CSV is `[][]string` until you give it meaning.** `csv.NewReader(...).ReadAll()` returns rows of strings. Build a **header → column-index map** so you address columns by name, not by fragile position, then convert each cell with `strconv`.
- **Everything from the outside is a string.** `strconv.Atoi`/`ParseFloat`/`ParseBool` and `time.Parse` (reference layout `2006-01-02 15:04:05`) turn text into typed values — and they return an **error**, not a panic, so a bad cell is recoverable.
- **NULL is not the zero value.** From a DB, a nullable column is `sql.NullString{String, Valid}` (check `Valid`); in JSON, model nullable fields as pointers so `nil` ⇄ `null`. Treating a missing value as `""`/`0` silently corrupts reports.
- **DB rows are a slice once scanned.** The universal idiom is `for rows.Next() { rows.Scan(&f1, &f2, …) }` then `rows.Err()` — after that loop you hold a `[]Struct` and everything is [54](54-collection-operations.md) again. `Scan` order must match the `SELECT` column order.
- **Shaping is the 54 toolkit on parsed data.** Filter records, map to a **DTO** (rename/derive/drop internal fields), **group** by a field, **aggregate** (a per-group accumulator struct carries count/sum/min/max at once), **sort**, **paginate** (`s[offset:offset+limit]`), and **join** by indexing one side into a `map[K]T` (a hash join).
- **Stream when the input is large.** `json.Decoder` reads an array element-by-element (`Token()` past `[`, `Decode` each, `Token()` past `]`); **NDJSON** (one JSON object per line) reads via `Decoder.More()` or a `bufio.Scanner`; a **lazy `iter.Seq[Record]`** yields parsed rows one at a time so you never hold the whole file. Constant memory regardless of input size.
- **When the schema is unknown, decode loosely.** `map[string]any` (numbers become `float64`, arrays `[]any`, objects `map[string]any` — type-assert as you go), or `json.RawMessage` to defer decoding a sub-object until you know its type (tagged unions). A **custom `MarshalJSON`/`UnmarshalJSON`** handles nonstandard formats like money-as-string or a bespoke date.

## Exercises
1. Decode a JSON array into `[]User`; then `Marshal` and `MarshalIndent` a struct back out.
2. Use `,omitempty`, `json:"-"`, and a `*int` field; confirm which fields appear for a zero-valued struct.
3. Read a CSV with `ReadAll`, build a header→index map, and convert each row into a struct.
4. Parse an `int`, `float`, `bool`, and a date from strings; handle a bad conversion without panicking.
5. Model a nullable value with `sql.NullString` and, separately, a `*string` that marshals to JSON `null`.
6. Write structs back out as CSV with `csv.Writer` (remember `Flush`).
7. Over parsed records: filter (active only), map to a DTO (lowercased email, full name, no secrets), group by a field, and aggregate sum/avg per group with sorted output.
8. Hash-join two datasets on a key (index one side into a map); sort + paginate a result set.
9. Scan "DB rows" into structs with the `Next`/`Scan`/`Err` idiom.
10. Stream a JSON array with `Decoder.Token()`/`Decode`; read and transform NDJSON line-by-line with constant memory.
11. Decode unknown JSON into `map[string]any`; use `json.RawMessage` for a tagged payload; write a custom `Money` (Un)Marshaler.
12. Capstone: CSV transactions → parse/validate → filter settled → group by category → aggregate (count/total) → emit a sorted JSON summary.

## Best Practices & Pitfalls
- **Decode at the edge, then work with structs.** Don't thread `map[string]any` through your logic — parse into typed structs once, so the rest of the code is type-safe and the [54](54-collection-operations.md) toolkit applies.
- **Always check the decode/convert error.** `json.Unmarshal`, `csv.Read`, `strconv.Atoi`, `time.Parse`, and `rows.Scan` all return errors; a malformed record should be skipped or reported, never silently coerced to a zero value.
- **Pitfall — `json.Unmarshal` numbers are `float64`.** In a `map[string]any`, every number (even an "int") comes back as `float64`; assert `.(float64)` and convert. For big integers that lose precision, use `Decoder.UseNumber()`.
- **Pitfall — the zero value hides missing data.** An absent JSON field or a NULL column leaves the Go zero value (`0`, `""`, `false`), which is indistinguishable from a real zero. Use pointers or `sql.Null*` when the distinction matters.
- **Pitfall — CSV column order.** Addressing cells by literal index (`row[2]`) breaks the day someone reorders columns. Index by header name.
- **Pitfall — forgetting `csv.Writer.Flush` / not `Close`ing `rows`.** The CSV writer buffers, so output is empty without `Flush`; `*sql.Rows` must be `Close`d (usually `defer rows.Close()`) or you leak a connection.
- **Pitfall — `rows.Scan` argument order.** `Scan` fills by position, matching the `SELECT` list, not struct field names. Keep them in lockstep.
- **Sort before emitting anything built from a map.** Every group/aggregate/pivot result iterates a map — collect and sort keys first, or your JSON/CSV output (and tests) are non-deterministic.
- **Stream when input size is unbounded.** `ReadAll`/`Unmarshal` load everything into memory; a `Decoder`, a `bufio.Scanner`, or a lazy `iter.Seq` keep it flat.

## Checklist
- [ ] I can Unmarshal/Marshal JSON arrays and control fields with `omitempty` / `-` / pointers.
- [ ] I can read CSV, index columns by header, and convert cells with `strconv` / `time.Parse` (checking errors).
- [ ] I can represent NULL/optional with `sql.Null*` and JSON-null pointers, and I know why the zero value isn't enough.
- [ ] I can scan DB rows into structs with `Next`/`Scan`/`Err` and `defer rows.Close()`.
- [ ] I can filter → map-to-DTO → group → aggregate → sort → paginate parsed records, and hash-join two datasets.
- [ ] I can stream a JSON array, read/write NDJSON, and build a lazy `iter.Seq` record reader.
- [ ] I can decode unknown JSON (`map[string]any`, `RawMessage`) and write a custom (Un)Marshaler.
- [ ] I can assemble a CSV→JSON ETL end-to-end.

## Resources
- `encoding/json`: https://pkg.go.dev/encoding/json · Go blog "JSON and Go": https://go.dev/blog/json
- `encoding/csv`: https://pkg.go.dev/encoding/csv
- `strconv`: https://pkg.go.dev/strconv · `time` layouts: https://pkg.go.dev/time#pkg-constants
- `database/sql` (`Rows`, `Null*`): https://pkg.go.dev/database/sql · the tutorial: https://go.dev/doc/database/querying
- `iter` (record iterators): https://pkg.go.dev/iter
- Examples: [examples/55-data-pipelines](examples/55-data-pipelines/).
- Related in this plan: the operations these pipelines run in [54 — Collection Operations](54-collection-operations.md); real DB access in [22 — Persistence with database/sql](22-database.md); DTOs & API shaping in [21 — Building a JSON REST API](21-rest-api.md).
