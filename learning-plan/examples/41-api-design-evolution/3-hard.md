# 41 · Hard (11–15) — headers, conventions, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. ETag / 304 Not Modified

`ETag` + `If-None-Match` → `304 Not Modified` when unchanged, so the client reuses its cached copy and
the body isn't re-sent (caching at the protocol layer).

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func handler(w http.ResponseWriter, r *http.Request) {
	const etag = `"v1"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified) // 304, no body
		return
	}
	fmt.Fprint(w, "full body")
}

func main() {
	rec1 := httptest.NewRecorder()
	handler(rec1, httptest.NewRequest("GET", "/", nil))
	fmt.Printf("1st: %d etag=%s body=%q\n", rec1.Code, rec1.Header().Get("ETag"), rec1.Body.String())

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("If-None-Match", `"v1"`)
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)
	fmt.Printf("2nd: %d body=%q\n", rec2.Code, rec2.Body.String())
}
```

**Output**
```
1st: 200 etag="v1" body="full body"
2nd: 304 body=""
```

---

## 12. Rate-limit headers

On rate limit, return `429` with `Retry-After` and `RateLimit-*` headers so clients back off politely.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func handle(allowed bool) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	if !allowed {
		rec.Header().Set("Retry-After", "30")
		rec.Header().Set("RateLimit-Remaining", "0")
		rec.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(rec, "slow down")
		return rec
	}
	rec.Header().Set("RateLimit-Remaining", "9")
	rec.WriteHeader(http.StatusOK)
	fmt.Fprint(rec, "ok")
	return rec
}

func main() {
	ok := handle(true)
	fmt.Printf("allowed: %d remaining=%s\n", ok.Code, ok.Header().Get("RateLimit-Remaining"))
	limited := handle(false)
	fmt.Printf("limited: %d retry-after=%s\n", limited.Code, limited.Header().Get("Retry-After"))
}
```

**Output**
```
allowed: 200 remaining=9
limited: 429 retry-after=30
```

---

## 13. Consistency conventions

Conventions that age well: money as integer minor units + currency, RFC3339 UTC timestamps, string
enums, opaque ids.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Money struct {
	Amount   int64  `json:"amount"` // minor units (cents), never a float
	Currency string `json:"currency"`
}

type Order struct {
	ID        string `json:"id"`     // opaque, not an auto-increment int
	Status    string `json:"status"` // string enum, not a magic number
	Total     Money  `json:"total"`
	CreatedAt string `json:"created_at"` // RFC3339 UTC
}

func main() {
	o := Order{
		ID:        "ord_01H",
		Status:    "pending",
		Total:     Money{Amount: 3250, Currency: "USD"},
		CreatedAt: "2026-07-07T12:00:00Z",
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	fmt.Println(string(b))
}
```

**Output**
```
{
  "id": "ord_01H",
  "status": "pending",
  "total": {
    "amount": 3250,
    "currency": "USD"
  },
  "created_at": "2026-07-07T12:00:00Z"
}
```

---

## 14. Deprecation & Sunset

When you must retire an old version, signal it with `Deprecation` and `Sunset` headers and a successor
link — never silently change behaviour.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func v1(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "Wed, 31 Dec 2026 23:59:59 GMT")
	w.Header().Set("Link", `</v2/user>; rel="successor-version"`)
	fmt.Fprint(w, `{"name":"Alice"}`)
}

func main() {
	rec := httptest.NewRecorder()
	v1(rec, httptest.NewRequest("GET", "/v1/user", nil))
	fmt.Println("Deprecation:", rec.Header().Get("Deprecation"))
	fmt.Println("Sunset:", rec.Header().Get("Sunset"))
	fmt.Println("Link:", rec.Header().Get("Link"))
}
```

**Output**
```
Deprecation: true
Sunset: Wed, 31 Dec 2026 23:59:59 GMT
Link: </v2/user>; rel="successor-version"
```

---

## 15. Capstone: a versioned resource

One versioned resource wired together — GET with cursor pagination, POST with an idempotency key, and
a `problem+json` error on the missing key. Driven end-to-end with `httptest`.

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

type Problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

type page struct {
	Data       []int  `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func encodeCursor(lastID int) string {
	b, _ := json.Marshal(map[string]int{"last_id": lastID})
	return base64.URLEncoding.EncodeToString(b)
}

func main() {
	data := []int{10, 20, 30, 40}
	seen := map[string]bool{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/items", func(w http.ResponseWriter, r *http.Request) {
		limit := 2
		var out []int
		for _, id := range data {
			out = append(out, id)
			if len(out) == limit {
				break
			}
		}
		_ = json.NewEncoder(w).Encode(page{Data: out, NextCursor: encodeCursor(out[len(out)-1])})
	})

	mux.HandleFunc("POST /v1/items", func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Problem{Title: "missing Idempotency-Key", Status: 400, Code: "MISSING_IDEMPOTENCY_KEY"})
			return
		}
		if seen[key] {
			fmt.Fprint(w, "replayed")
			return
		}
		seen[key] = true
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "created")
	})

	do := func(method, path, key string) {
		req := httptest.NewRequest(method, path, nil)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		fmt.Printf("%s %s → %d %s\n", method, path, rec.Code, strings.TrimRight(rec.Body.String(), "\n"))
	}

	do("GET", "/v1/items", "")
	do("POST", "/v1/items", "")   // 400 problem+json (missing key)
	do("POST", "/v1/items", "k1") // 201 created
	do("POST", "/v1/items", "k1") // replayed
}
```

**Output**
```
GET /v1/items → 200 {"data":[10,20],"next_cursor":"eyJsYXN0X2lkIjoyMH0="}
POST /v1/items → 400 {"title":"missing Idempotency-Key","status":400,"code":"MISSING_IDEMPOTENCY_KEY"}
POST /v1/items → 201 created
POST /v1/items → 200 replayed
```

---

Back to [index](README.md) · That's **Track C** (production cross-cutting: 38 · 39 · 40 · 41) — and the
whole **Part 9 architecture example set (31–41)** — complete. 🎉
