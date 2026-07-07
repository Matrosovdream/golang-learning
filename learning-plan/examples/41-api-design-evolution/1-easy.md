# 41 · Easy (1–5) — verbs, codes, versioning

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Status codes for outcomes

Mean the status codes: map each outcome to the right one.

```go
package main

import (
	"fmt"
	"net/http"
)

func main() {
	outcomes := []struct {
		op   string
		code int
	}{
		{"created a resource", http.StatusCreated},
		{"validation failed", http.StatusBadRequest},
		{"not authenticated", http.StatusUnauthorized},
		{"resource not found", http.StatusNotFound},
		{"duplicate / conflict", http.StatusConflict},
		{"rate limited", http.StatusTooManyRequests},
	}
	for _, o := range outcomes {
		fmt.Printf("%-22s → %d %s\n", o.op, o.code, http.StatusText(o.code))
	}
}
```

**Output**
```
created a resource     → 201 Created
validation failed      → 400 Bad Request
not authenticated      → 401 Unauthorized
resource not found     → 404 Not Found
duplicate / conflict   → 409 Conflict
rate limited           → 429 Too Many Requests
```

---

## 2. Verb idempotency

Verb idempotency is a contract, and it's what makes safe retries possible: a client can re-PUT/DELETE
after a timeout, but not blindly re-POST.

```go
package main

import "fmt"

func main() {
	verbs := []struct {
		verb       string
		idempotent bool
		note       string
	}{
		{"GET", true, "safe, no side effects"},
		{"PUT", true, "full replace — re-PUT is safe"},
		{"DELETE", true, "deleting twice still ends deleted"},
		{"POST", false, "create/action — needs an idempotency key to retry safely"},
	}
	for _, v := range verbs {
		fmt.Printf("%-7s idempotent=%-5v %s\n", v.verb, v.idempotent, v.note)
	}
}
```

**Output**
```
GET     idempotent=true  safe, no side effects
PUT     idempotent=true  full replace — re-PUT is safe
DELETE  idempotent=true  deleting twice still ends deleted
POST    idempotent=false create/action — needs an idempotency key to retry safely
```

---

## 3. A backward-compatible change

Adding an optional field is backward-compatible: an old client decodes into a struct without the new
field and simply ignores it.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type OldClient struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	// server v2 response has a new "email" field:
	v2 := `{"id":"u1","name":"Alice","email":"alice@x.com"}`
	var old OldClient
	_ = json.Unmarshal([]byte(v2), &old)
	fmt.Printf("old client still works: %+v\n", old)
}
```

**Output**
```
old client still works: {ID:u1 Name:Alice}
```

---

## 4. Detecting a breaking change

Removing or renaming a field is breaking. Detect it by diffing the field sets.

```go
package main

import "fmt"

func removed(oldFields, newFields []string) []string {
	has := map[string]bool{}
	for _, f := range newFields {
		has[f] = true
	}
	var gone []string
	for _, f := range oldFields {
		if !has[f] {
			gone = append(gone, f)
		}
	}
	return gone
}

func main() {
	v1 := []string{"id", "name", "total"}
	v2 := []string{"id", "name", "amount"} // renamed total → amount

	fmt.Println("removed/renamed fields (BREAKING):", removed(v1, v2))
	fmt.Println("→ needs a new version, not an in-place change")
}
```

**Output**
```
removed/renamed fields (BREAKING): [total]
→ needs a new version, not an in-place change
```

---

## 5. URI versioning, side by side

When you must break, version — and run the versions side by side under different URI prefixes.
(Go 1.22+ method+path routing.)

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Alice"}`)
	})
	mux.HandleFunc("GET /v2/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"first_name":"Alice","last_name":"Smith"}`)
	})

	for _, path := range []string{"/v1/user", "/v2/user"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		fmt.Printf("%s → %s\n", path, rec.Body.String())
	}
}
```

**Output**
```
/v1/user → {"name":"Alice"}
/v2/user → {"first_name":"Alice","last_name":"Smith"}
```

---

Next tier → [Medium (6–10)](2-medium.md)
