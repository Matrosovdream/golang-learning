# Step 59 — Production Database Patterns · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex
go mod init scratch
go get modernc.org/sqlite   # pure-Go SQLite driver — no cgo, no external database
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. The runnable examples use **`modernc.org/sqlite`** (the only dependency); the code is plain **`database/sql`**, so it ports to Postgres by swapping the driver + DSN. The few **Postgres-only** features (`FOR UPDATE`, advisory locks, `LISTEN/NOTIFY`, `COPY`, jsonb) are shown as clearly-marked reference snippets (examples 19, 20, and notes in 25). Examples keep `SetMaxOpenConns(1)` so the in-memory (`:memory:`) database lives on a single connection.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Open a connection pool](1-easy.md#1-open-a-connection-pool)
- [2. Tune the pool](1-easy.md#2-tune-the-pool)
- [3. Query a single row](1-easy.md#3-query-a-single-row)
- [4. Query many rows](1-easy.md#4-query-many-rows)
- [5. Always pass a context](1-easy.md#5-always-pass-a-context)
- [6. Handle NULLs](1-easy.md#6-handle-nulls)
- [7. Prepared statements](1-easy.md#7-prepared-statements)
- [8. Named parameters](1-easy.md#8-named-parameters)

### 🟡 [Medium](2-medium.md)

- [9. Transactions](2-medium.md#9-transactions)
- [10. Rollback on error](2-medium.md#10-rollback-on-error)
- [11. Schema migrations](2-medium.md#11-schema-migrations)
- [12. Fix the N+1 problem](2-medium.md#12-fix-the-n1-problem)
- [13. Batch insert](2-medium.md#13-batch-insert)
- [14. Keyset pagination](2-medium.md#14-keyset-pagination)
- [15. Read the query plan](2-medium.md#15-read-the-query-plan)
- [16. Upsert](2-medium.md#16-upsert)
- [17. Optimistic concurrency](2-medium.md#17-optimistic-concurrency)

### 🔴 [Hard](3-hard.md)

- [18. Isolation levels and retry](3-hard.md#18-isolation-levels-and-retry)
- [19. Pessimistic locking](3-hard.md#19-pessimistic-locking)
- [20. Advisory locks and LISTEN/NOTIFY](3-hard.md#20-advisory-locks-and-listennotify)
- [21. JSON columns](3-hard.md#21-json-columns)
- [22. Soft deletes](3-hard.md#22-soft-deletes)
- [23. The repository pattern](3-hard.md#23-the-repository-pattern)
- [24. Read/write splitting](3-hard.md#24-readwrite-splitting)
- [25. Bulk loading](3-hard.md#25-bulk-loading)
- [26. Capstone: an orders repository](3-hard.md#26-capstone-an-orders-repository)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
