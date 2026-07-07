# 32 · Medium (6–10) — driving adapters & testing

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. HTTP driving adapter (httptest)

A **driving adapter** translates an HTTP request into a call on the driving port and the result back.
It depends on the port, not the concrete service. `httptest` drives it in-process.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

type PlaceOrder interface {
	Place(customer string) (string, error)
}

type svc struct{ seq int }

func (s *svc) Place(customer string) (string, error) {
	s.seq++
	return fmt.Sprintf("ord-%d", s.seq), nil
}

type handler struct{ uc PlaceOrder }

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Customer string `json:"customer"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id, _ := h.uc.Place(body.Customer)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func main() {
	h := &handler{uc: &svc{}}
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{"customer":"alice"}`))
	rec := httptest.NewRecorder()
	h.create(rec, req)
	fmt.Println("status:", rec.Code)
	fmt.Print("body: ", rec.Body.String())
}
```

**Output**
```
status: 201
body: {"id":"ord-1"}
```

---

## 7. A second driving adapter (CLI) for the same port

The same driving port, driven by a CLI instead of HTTP — zero changes to the core. HTTP, gRPC, a
queue consumer: all just adapters.

```go
package main

import "fmt"

type PlaceOrder interface {
	Place(customer string) (string, error)
}

type svc struct{ seq int }

func (s *svc) Place(customer string) (string, error) {
	s.seq++
	return fmt.Sprintf("ord-%d", s.seq), nil
}

func cli(uc PlaceOrder, args []string) {
	for _, customer := range args {
		id, _ := uc.Place(customer)
		fmt.Printf("cli: placed %s for %s\n", id, customer)
	}
}

func main() {
	uc := &svc{}
	cli(uc, []string{"alice", "bob"})
}
```

**Output**
```
cli: placed ord-1 for alice
cli: placed ord-2 for bob
```

---

## 8. Test the core with an in-memory fake

The payoff of ports: test the core with an in-memory fake and zero infrastructure. A fake `Clock`
makes output deterministic.

```go
package main

import "fmt"

type Clock interface{ Now() string } // driven port

type Stamper struct{ clock Clock }

func (s Stamper) Stamp(msg string) string { return s.clock.Now() + " " + msg }

type fakeClock struct{ t string }

func (f fakeClock) Now() string { return f.t }

func main() {
	s := Stamper{clock: fakeClock{t: "2026-01-01T00:00:00Z"}}
	fmt.Println(s.Stamp("event"))
}
```

**Output**
```
2026-01-01T00:00:00Z event
```

---

## 9. Small, consumer-defined ports

Keep ports small and defined at the consumer. The core declares only the one method it needs; a big
infrastructure type satisfies it implicitly.

```go
package main

import "fmt"

type BigDB struct{}

func (BigDB) Connect()              {}
func (BigDB) Migrate()              {}
func (BigDB) Query(q string) string { return "rows-for:" + q }
func (BigDB) Close()                {}

// The core's port — one method, the only thing findUser actually needs:
type UserFinder interface{ Query(q string) string }

func findUser(f UserFinder, id string) string { return f.Query("user " + id) }

func main() {
	db := BigDB{}
	fmt.Println(findUser(db, "42")) // BigDB satisfies the tiny UserFinder implicitly
}
```

**Output**
```
rows-for:user 42
```

---

## 10. Dependency inversion: arrows point inward

The core owns the interface; the adapter depends on the core, never the reverse. That inward arrow is
the whole idea.

```go
package main

import "fmt"

// ===== core (its own package in real life; imports nothing below) =====
type EmailPort interface{ Send(to, body string) error } // core owns this

type Welcomer struct{ email EmailPort }

func (w Welcomer) Welcome(user string) error {
	return w.email.Send(user+"@example.com", "welcome "+user)
}

// ===== adapter (its own package; imports the core) =====
type smtpAdapter struct{ sent []string }

func (a *smtpAdapter) Send(to, body string) error {
	a.sent = append(a.sent, to+": "+body)
	return nil
}

func main() {
	a := &smtpAdapter{}
	w := Welcomer{email: a} // adapter depends on the core's interface; core knows nothing of the adapter
	_ = w.Welcome("alice")
	fmt.Println("sent:", a.sent)
}
```

**Output**
```
sent: [alice@example.com: welcome alice]
```

---

Next tier → [Hard (11–15)](3-hard.md)
