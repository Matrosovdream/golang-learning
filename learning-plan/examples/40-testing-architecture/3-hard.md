# 40 · Hard (11–15) — determinism, contracts, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. The test-double taxonomy

The test-double taxonomy (Meszaros / Fowler), simplest → most demanding.

```go
package main

import "fmt"

func main() {
	doubles := []struct{ kind, does string }{
		{"dummy", "passed but unused (fills a parameter)"},
		{"stub", "returns canned answers"},
		{"fake", "a working lightweight impl (in-memory) — the default"},
		{"spy", "a stub that records how it was called"},
		{"mock", "pre-programmed expectations; fails if calls don't match"},
	}
	for _, d := range doubles {
		fmt.Printf("%-6s %s\n", d.kind, d.does)
	}
}
```

**Output**
```
dummy  passed but unused (fills a parameter)
stub   returns canned answers
fake   a working lightweight impl (in-memory) — the default
spy    a stub that records how it was called
mock   pre-programmed expectations; fails if calls don't match
```

---

## 12. Determinism: no sleeps

Never `time.Sleep` to "wait for" async work — it's the #1 flake source. Synchronize explicitly (a
channel / WaitGroup) so the test is deterministic.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	result := make(chan int, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		result <- 42 // async work completes
	}()

	wg.Wait() // wait for completion — NOT time.Sleep
	fmt.Println("result:", <-result)
}
```

**Output**
```
result: 42
```

---

## 13. A contract check

A contract check catches a breaking change between services **before** deploy: a consumer requires
certain fields; the provider must still supply them.

```go
package main

import "fmt"

func breaks(providerFields, consumerRequires []string) []string {
	has := map[string]bool{}
	for _, f := range providerFields {
		has[f] = true
	}
	var missing []string
	for _, r := range consumerRequires {
		if !has[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

func main() {
	consumerRequires := []string{"id", "total", "currency"}
	providerV2 := []string{"id", "amount", "currency"} // renamed total → amount: breaking!

	missing := breaks(providerV2, consumerRequires)
	if len(missing) == 0 {
		fmt.Println("contract OK")
	} else {
		fmt.Println("BREAKING: provider missing fields the consumer needs:", missing)
	}
}
```

**Output**
```
BREAKING: provider missing fields the consumer needs: [total]
```

---

## 14. A reusable fake

One hand-written fake, reused across many tests — each test gets a fresh, isolated instance, so tests
never share state.

```go
package main

import "fmt"

type Store struct{ m map[string]int }

func newStore() *Store { return &Store{m: map[string]int{}} }

func (s *Store) Set(k string, v int) { s.m[k] = v }
func (s *Store) Get(k string) int    { return s.m[k] }

func testA(s *Store) string { s.Set("x", 1); return fmt.Sprintf("A: x=%d", s.Get("x")) }
func testB(s *Store) string { s.Set("y", 2); return fmt.Sprintf("B: y=%d", s.Get("y")) }

func main() {
	fmt.Println(testA(newStore())) // fresh fake
	fmt.Println(testB(newStore())) // fresh fake — no leftover state from A
}
```

**Output**
```
A: x=1
B: y=2
```

---

## 15. Capstone: unit-test a use case

Unit-test a "register user" use case with a fake repo, a spy mailer, and an injected clock — fully
deterministic, no infrastructure. This is the payoff of hexagonal ports ([lesson 32](../32-hexagonal-ports-adapters/README.md)): the core is trivially testable.

```go
package main

import (
	"errors"
	"fmt"
)

type User struct {
	ID        string
	Email     string
	CreatedAt int64
}

type Repo interface{ Save(User) error }
type Mailer interface{ SendWelcome(email string) }
type Clock interface{ Now() int64 }

type Service struct {
	repo   Repo
	mailer Mailer
	clock  Clock
}

func (s Service) Register(email string) (User, error) {
	if email == "" {
		return User{}, errors.New("email required")
	}
	u := User{ID: "u1", Email: email, CreatedAt: s.clock.Now()}
	if err := s.repo.Save(u); err != nil {
		return User{}, err
	}
	s.mailer.SendWelcome(email)
	return u, nil
}

// --- test doubles ---
type fakeRepo struct{ saved []User }

func (r *fakeRepo) Save(u User) error { r.saved = append(r.saved, u); return nil }

type spyMailer struct{ sent []string }

func (m *spyMailer) SendWelcome(email string) { m.sent = append(m.sent, email) }

type fixedClock struct{}

func (fixedClock) Now() int64 { return 1000 }

func main() {
	repo := &fakeRepo{}
	mailer := &spyMailer{}
	svc := Service{repo: repo, mailer: mailer, clock: fixedClock{}}

	u, err := svc.Register("alice@x.com")
	fmt.Printf("returned: %+v err=%v\n", u, err)
	fmt.Println("repo saved:", len(repo.saved), "user(s), CreatedAt =", repo.saved[0].CreatedAt)
	fmt.Println("welcome emails:", mailer.sent)
}
```

**Output**
```
returned: {ID:u1 Email:alice@x.com CreatedAt:1000} err=<nil>
repo saved: 1 user(s), CreatedAt = 1000
welcome emails: [alice@x.com]
```

---

Back to [index](README.md) · Next lesson's examples: [41 — API Design & Evolution](../41-api-design-evolution/README.md).
