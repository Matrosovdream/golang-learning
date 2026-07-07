# 41 · Medium (6–10) — pagination, idempotency, errors

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Offset pagination drifts

Offset/limit is simple but **drifts**: an insert between page reads shifts the window, so a row can be
duplicated or skipped.

```go
package main

import "fmt"

func page(data []string, offset, limit int) []string {
	if offset > len(data) {
		return nil
	}
	end := offset + limit
	if end > len(data) {
		end = len(data)
	}
	return data[offset:end]
}

func main() {
	data := []string{"a", "b", "c", "d"}
	fmt.Println("page 1 (offset 0):", page(data, 0, 2)) // [a b]

	data = append([]string{"A"}, data...)               // a row is inserted at the front
	fmt.Println("page 2 (offset 2):", page(data, 2, 2)) // [b c]
	fmt.Println("→ 'b' appears on both pages (drift from the insert)")
}
```

**Output**
```
page 1 (offset 0): [a b]
page 2 (offset 2): [b c]
→ 'b' appears on both pages (drift from the insert)
```

---

## 7. Cursor pagination

The cursor encodes the last-seen sort key as an opaque token. Stable under inserts and fast at any
depth (no scan-and-discard).

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type cursor struct {
	LastID int `json:"last_id"`
}

func encode(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decode(s string) cursor {
	var c cursor
	b, _ := base64.URLEncoding.DecodeString(s)
	_ = json.Unmarshal(b, &c)
	return c
}

func pageAfter(data []int, lastID, limit int) ([]int, string) {
	var out []int
	for _, id := range data {
		if id > lastID {
			out = append(out, id)
			if len(out) == limit {
				break
			}
		}
	}
	next := ""
	if len(out) == limit {
		next = encode(cursor{LastID: out[len(out)-1]})
	}
	return out, next
}

func main() {
	data := []int{10, 20, 30, 40, 50}
	p1, next := pageAfter(data, 0, 2)
	fmt.Println("page 1:", p1, "next cursor:", next)
	p2, _ := pageAfter(data, decode(next).LastID, 2)
	fmt.Println("page 2:", p2)
}
```

**Output**
```
page 1: [10 20] next cursor: eyJsYXN0X2lkIjoyMH0=
page 2: [30 40]
```

---

## 8. Cap limit & whitelist sort

Always cap `limit` server-side and **whitelist** sortable columns — never interpolate a raw column
name (that's SQL injection).

```go
package main

import "fmt"

var sortable = map[string]bool{"created_at": true, "name": true}

func clampLimit(req, max int) int {
	switch {
	case req > max:
		return max
	case req < 1:
		return 1
	default:
		return req
	}
}

func validSort(col string) (string, bool) {
	if sortable[col] {
		return col, true
	}
	return "created_at", false // fall back to a safe default
}

func main() {
	fmt.Println("limit 1000 →", clampLimit(1000, 100))
	fmt.Println("limit 20 →", clampLimit(20, 100))

	col, ok := validSort("name")
	fmt.Printf("sort by name: %s ok=%v\n", col, ok)
	col, ok = validSort("password; DROP TABLE users")
	fmt.Printf("sort by injection: %s ok=%v\n", col, ok)
}
```

**Output**
```
limit 1000 → 100
limit 20 → 20
sort by name: name ok=true
sort by injection: created_at ok=false
```

---

## 9. Idempotency-key middleware

An `Idempotency-Key` lets a client safely retry a POST: the server stores key→result and replays it
instead of acting twice.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

type idemStore struct {
	seen    map[string]string
	charges int
}

func (s *idemStore) create(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if body, ok := s.seen[key]; ok {
		fmt.Fprintf(w, "%s (replayed)", body)
		return
	}
	s.charges++
	body := fmt.Sprintf("charge-%d", s.charges)
	s.seen[key] = body
	fmt.Fprint(w, body)
}

func main() {
	s := &idemStore{seen: map[string]string{}}
	post := func() string {
		req := httptest.NewRequest("POST", "/charge", nil)
		req.Header.Set("Idempotency-Key", "abc")
		rec := httptest.NewRecorder()
		s.create(rec, req)
		return rec.Body.String()
	}
	fmt.Println("1st:  ", post())
	fmt.Println("retry:", post())
	fmt.Println("actual charges:", s.charges) // 1
}
```

**Output**
```
1st:   charge-1
retry: charge-1 (replayed)
actual charges: 1
```

---

## 10. problem+json (RFC 9457)

Standardize errors on RFC 9457 `problem+json`, with a stable machine-readable `code` clients branch on
(not the human message).

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
)

type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func main() {
	rec := httptest.NewRecorder()
	writeProblem(rec, Problem{
		Type:   "https://api.acme.com/problems/insufficient-funds",
		Title:  "Insufficient funds",
		Status: 422,
		Detail: "balance 1000, requested 1500",
		Code:   "INSUFFICIENT_FUNDS",
	})
	fmt.Println("Content-Type:", rec.Header().Get("Content-Type"))
	fmt.Println("Status:", rec.Code)
	fmt.Print(rec.Body.String())
}
```

**Output**
```
Content-Type: application/problem+json
Status: 422
{"type":"https://api.acme.com/problems/insufficient-funds","title":"Insufficient funds","status":422,"detail":"balance 1000, requested 1500","code":"INSUFFICIENT_FUNDS"}
```

---

Next tier → [Hard (11–15)](3-hard.md)
