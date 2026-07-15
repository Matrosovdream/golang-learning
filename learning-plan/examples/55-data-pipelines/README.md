# Step 55 — Data Pipelines · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. Stdlib-only (`encoding/json`, `encoding/csv`, `strconv`, `time`, `database/sql`, `iter`); needs **Go 1.23+** for the iterator examples.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Decode a JSON array](1-easy.md#1-decode-a-json-array)
- [2. Encode structs to JSON](1-easy.md#2-encode-structs-to-json)
- [3. Struct tags for optional fields](1-easy.md#3-struct-tags-for-optional-fields)
- [4. Read a CSV file](1-easy.md#4-read-a-csv-file)
- [5. CSV rows to structs](1-easy.md#5-csv-rows-to-structs)
- [6. Parse values from strings](1-easy.md#6-parse-values-from-strings)
- [7. Handle NULL and optional values](1-easy.md#7-handle-null-and-optional-values)
- [8. Write CSV from structs](1-easy.md#8-write-csv-from-structs)

### 🟡 [Medium](2-medium.md)

- [9. Filter parsed records](2-medium.md#9-filter-parsed-records)
- [10. Map records to DTOs](2-medium.md#10-map-records-to-dtos)
- [11. Group parsed records by a field](2-medium.md#11-group-parsed-records-by-a-field)
- [12. Aggregate per group](2-medium.md#12-aggregate-per-group)
- [13. Join two datasets on a key](2-medium.md#13-join-two-datasets-on-a-key)
- [14. Sort and paginate](2-medium.md#14-sort-and-paginate)
- [15. Scan database rows into structs](2-medium.md#15-scan-database-rows-into-structs)
- [16. Read NDJSON (JSON Lines)](2-medium.md#16-read-ndjson-json-lines)
- [17. Stream a large JSON array](2-medium.md#17-stream-a-large-json-array)

### 🔴 [Hard](3-hard.md)

- [18. Report: filter, group by month, sum](3-hard.md#18-report-filter-group-by-month-sum)
- [19. Multi-metric aggregate per group](3-hard.md#19-multi-metric-aggregate-per-group)
- [20. Pivot a table (crosstab)](3-hard.md#20-pivot-a-table-crosstab)
- [21. Enrich JSON with a CSV lookup](3-hard.md#21-enrich-json-with-a-csv-lookup)
- [22. Decode dynamic JSON](3-hard.md#22-decode-dynamic-json)
- [23. Custom JSON marshaling](3-hard.md#23-custom-json-marshaling)
- [24. A lazy record iterator](3-hard.md#24-a-lazy-record-iterator)
- [25. Streaming NDJSON transform](3-hard.md#25-streaming-ndjson-transform)
- [26. Capstone: a CSV to JSON ETL](3-hard.md#26-capstone-a-csv-to-json-etl)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
