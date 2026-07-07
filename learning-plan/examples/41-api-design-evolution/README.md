# Step 41 — API Design & Evolution · Examples

A library of **15 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[41-api-design-evolution.md](../../41-api-design-evolution.md): status codes, backward-compatible
change & versioning, cursor pagination, idempotency keys, RFC 9457 `problem+json`, and caching /
rate-limit headers.

## One-time setup

```bash
mkdir -p /tmp/api-ex && cd /tmp/api-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** is real stdout.
Standard-library only — the HTTP examples use `net/http` + `net/http/httptest`, so nothing leaves the
process. (Examples 5 and 15 use `mux.HandleFunc("GET /path", …)` method routing → **Go 1.22+**.)

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–10 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 11–15 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — verbs, codes, versioning
- [1. Status codes for outcomes](1-easy.md#1-status-codes-for-outcomes)
- [2. Verb idempotency](1-easy.md#2-verb-idempotency)
- [3. A backward-compatible change](1-easy.md#3-a-backward-compatible-change)
- [4. Detecting a breaking change](1-easy.md#4-detecting-a-breaking-change)
- [5. URI versioning, side by side](1-easy.md#5-uri-versioning-side-by-side)

### 🟡 [Medium](2-medium.md) — pagination, idempotency, errors
- [6. Offset pagination drifts](2-medium.md#6-offset-pagination-drifts)
- [7. Cursor pagination](2-medium.md#7-cursor-pagination)
- [8. Cap limit & whitelist sort](2-medium.md#8-cap-limit--whitelist-sort)
- [9. Idempotency-key middleware](2-medium.md#9-idempotency-key-middleware)
- [10. problem+json (RFC 9457)](2-medium.md#10-problemjson-rfc-9457)

### 🔴 [Hard](3-hard.md) — headers, conventions, capstone
- [11. ETag / 304 Not Modified](3-hard.md#11-etag--304-not-modified)
- [12. Rate-limit headers](3-hard.md#12-rate-limit-headers)
- [13. Consistency conventions](3-hard.md#13-consistency-conventions)
- [14. Deprecation & Sunset](3-hard.md#14-deprecation--sunset)
- [15. Capstone: a versioned resource](3-hard.md#15-capstone-a-versioned-resource)
