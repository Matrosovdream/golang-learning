# 40 · Easy (1–5) — the pyramid & test doubles

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. The test pyramid

Shape the suite like a pyramid: many fast unit tests, fewer integration, a thin end-to-end layer. The
inverted "ice-cream cone" (mostly e2e) is the anti-pattern — slow and flaky.

```go
package main

import "fmt"

func main() {
	suite := []struct {
		layer string
		count int
	}{
		{"unit (fake deps, ms)", 120},
		{"integration (real DB via containers, s)", 20},
		{"end-to-end (whole system, slow)", 3},
	}
	for _, s := range suite {
		fmt.Printf("%-40s %d\n", s.layer, s.count)
	}
}
```

**Output**
```
unit (fake deps, ms)                     120
integration (real DB via containers, s)  20
end-to-end (whole system, slow)          3
```

---

## 2. A hand-written fake

A **fake** is a working in-memory implementation of a port — your default test double, because it
actually stores and returns.

```go
package main

import (
	"errors"
	"fmt"
)

type User struct{ ID, Name string }

type UserRepo interface {
	Save(User) error
	Get(id string) (User, error)
}

var ErrNotFound = errors.New("not found")

type fakeRepo struct{ m map[string]User }

func newFakeRepo() *fakeRepo { return &fakeRepo{m: map[string]User{}} }

func (r *fakeRepo) Save(u User) error { r.m[u.ID] = u; return nil }
func (r *fakeRepo) Get(id string) (User, error) {
	u, ok := r.m[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func main() {
	var repo UserRepo = newFakeRepo()
	_ = repo.Save(User{ID: "u1", Name: "Alice"})
	u, _ := repo.Get("u1")
	fmt.Printf("got %+v\n", u)
	_, err := repo.Get("u2")
	fmt.Println("missing:", err)
}
```

**Output**
```
got {ID:u1 Name:Alice}
missing: not found
```

---

## 3. A stub

A **stub** returns canned answers, ignoring input — just enough to drive a code path.

```go
package main

import "fmt"

type PricingStub struct{ price int }

func (s PricingStub) PriceOf(sku string) int { return s.price } // same answer regardless of sku

func total(p interface{ PriceOf(string) int }, skus []string) int {
	sum := 0
	for _, s := range skus {
		sum += p.PriceOf(s)
	}
	return sum
}

func main() {
	stub := PricingStub{price: 100}
	fmt.Println("total:", total(stub, []string{"a", "b", "c"})) // 3 * 100
}
```

**Output**
```
total: 300
```

---

## 4. A spy

A **spy** is a stub that also records how it was called, so a test can inspect the calls afterward.

```go
package main

import "fmt"

type MailerSpy struct{ sent []string }

func (m *MailerSpy) Send(to string) { m.sent = append(m.sent, to) }

func welcome(mailer interface{ Send(string) }, users []string) {
	for _, u := range users {
		mailer.Send(u)
	}
}

func main() {
	spy := &MailerSpy{}
	welcome(spy, []string{"a@x.com", "b@x.com"})
	fmt.Println("emails sent:", spy.sent)
	fmt.Println("count:", len(spy.sent))
}
```

**Output**
```
emails sent: [a@x.com b@x.com]
count: 2
```

---

## 5. A mock

A **mock** has pre-programmed expectations and fails if the calls don't match. Use it when the
interaction itself is the requirement.

```go
package main

import "fmt"

type PaymentMock struct {
	expectedAmount int
	called         bool
	ok             bool
}

func (m *PaymentMock) Charge(amount int) {
	m.called = true
	m.ok = amount == m.expectedAmount
}

func (m *PaymentMock) Verify() string {
	switch {
	case !m.called:
		return "FAIL: Charge was never called"
	case !m.ok:
		return "FAIL: Charge called with wrong amount"
	default:
		return "PASS"
	}
}

func checkout(p interface{ Charge(int) }, amount int) { p.Charge(amount) }

func main() {
	m := &PaymentMock{expectedAmount: 500}
	checkout(m, 500)
	fmt.Println(m.Verify())

	m2 := &PaymentMock{expectedAmount: 500}
	checkout(m2, 999)
	fmt.Println(m2.Verify())
}
```

**Output**
```
PASS
FAIL: Charge called with wrong amount
```

---

Next tier → [Medium (6–10)](2-medium.md)
