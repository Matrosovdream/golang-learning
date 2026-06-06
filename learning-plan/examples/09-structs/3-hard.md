# Step 09 — Structs · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 18. Embedding: field promotion

`🔴 hard` · *Embedding*

An embedded (anonymous) field's fields are promoted, so you can access them directly on the outer struct.

**Steps:**

1. Dog embeds Animal.
2. d.Name reaches the promoted Animal.Name.

```go
package main

import "fmt"

type Animal struct {
	Name string
}

type Dog struct {
	Animal // embedded
	Breed  string
}

func main() {
	d := Dog{Animal: Animal{Name: "Rex"}, Breed: "Lab"}
	fmt.Println(d.Name, d.Breed) // Name promoted from Animal
}
```

**Output:**

```
Rex Lab
```

---

## 19. Embedding: method promotion

`🔴 hard` · *Embedding*

Methods of an embedded type are promoted too, so the outer struct can call them as if they were its own.

**Steps:**

1. Animal has Speak().
2. Dog embeds Animal and can call d.Speak() directly.

```go
package main

import "fmt"

type Animal struct {
	Name string
}

func (a Animal) Speak() string {
	return a.Name + " makes a sound"
}

type Dog struct {
	Animal
}

func main() {
	d := Dog{Animal{Name: "Rex"}}
	fmt.Println(d.Speak()) // promoted from Animal
}
```

**Output:**

```
Rex makes a sound
```

---

## 20. Embedding: shadowing

`🔴 hard` · *Embedding*

A field or method on the outer struct shadows the embedded one with the same name; the embedded one is still reachable via the type name.

**Steps:**

1. Derived.Name and Derived.Describe shadow Base's.
2. d.Base.Name still reaches the inner field.

```go
package main

import "fmt"

type Base struct {
	Name string
}

func (b Base) Describe() string { return "base:" + b.Name }

type Derived struct {
	Base
	Name string // shadows Base.Name
}

func (d Derived) Describe() string { return "derived:" + d.Name } // shadows Base.Describe

func main() {
	d := Derived{Base: Base{Name: "B"}, Name: "D"}
	fmt.Println(d.Name)       // D (outer)
	fmt.Println(d.Base.Name)  // B (inner, still reachable)
	fmt.Println(d.Describe()) // derived:D
}
```

**Output:**

```
D
B
derived:D
```

---

## 21. Embedding to satisfy an interface

`🔴 hard` · *Embedding*

Because embedded methods are promoted, embedding a type that already implements an interface makes the outer struct implement it too.

**Steps:**

1. Animal implements Speaker.
2. Dog embeds Animal, so a Dog is also a Speaker.

```go
package main

import "fmt"

type Speaker interface {
	Speak() string
}

type Animal struct{ Name string }

func (a Animal) Speak() string { return a.Name + " speaks" }

type Dog struct {
	Animal // Dog gets Speak() via embedding
}

func main() {
	var s Speaker = Dog{Animal{Name: "Rex"}}
	fmt.Println(s.Speak())
}
```

**Output:**

```
Rex speaks
```

---

## 22. JSON: Marshal with struct tags

`🔴 hard` · *JSON*

encoding/json maps struct fields to JSON; `json:"name"` tags rename keys. Only exported (capitalized) fields are marshaled.

**Steps:**

1. Tag each field with its JSON key.
2. json.Marshal returns the bytes; convert to string to print.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func main() {
	u := User{Name: "Alice", Email: "a@x.com", Age: 30}
	b, _ := json.Marshal(u)
	fmt.Println(string(b))
}
```

**Output:**

```
{"name":"Alice","email":"a@x.com","age":30}
```

---

## 23. JSON: Unmarshal into a struct

`🔴 hard` · *JSON*

json.Unmarshal fills a struct from JSON bytes; pass a pointer so it can write into your value.

**Steps:**

1. Provide a *User to Unmarshal.
2. Matching tags populate the fields; check the error.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func main() {
	data := `{"name":"Bob","age":25}`
	var u User
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%+v\n", u)
}
```

**Output:**

```
{Name:Bob Age:25}
```

---

## 24. JSON: omitempty and json:"-"

`🔴 hard` · *JSON*

The omitempty option drops zero-valued fields from the output; the tag json:"-" excludes a field entirely (e.g. secrets).

**Steps:**

1. Nickname is empty, so omitempty drops it.
2. Password has json:"-" and is never marshaled.

```go
package main

import (
	"encoding/json"
	"fmt"
)

type Account struct {
	User     string `json:"user"`
	Nickname string `json:"nickname,omitempty"`
	Password string `json:"-"`
}

func main() {
	a := Account{User: "alice", Password: "secret"}
	b, _ := json.Marshal(a)
	fmt.Println(string(b))
}
```

**Output:**

```
{"user":"alice"}
```

---

## 25. Constructor functions (NewX)

`🔴 hard` · *Constructors*

Go has no constructors; the idiom is a NewX function that validates inputs and returns (*T, error).

**Steps:**

1. NewAccount checks its arguments.
2. It returns a pointer on success, or an error.

```go
package main

import (
	"errors"
	"fmt"
)

type Account struct {
	Owner   string
	Balance int
}

func NewAccount(owner string, initial int) (*Account, error) {
	if owner == "" {
		return nil, errors.New("owner required")
	}
	if initial < 0 {
		return nil, errors.New("initial balance cannot be negative")
	}
	return &Account{Owner: owner, Balance: initial}, nil
}

func main() {
	a, err := NewAccount("Alice", 100)
	fmt.Println(a, err)
	_, err = NewAccount("", 0)
	fmt.Println("error:", err)
}
```

**Output:**

```
&{Alice 100} <nil>
error: owner required
```

---

## 26. The empty struct struct{}

`🔴 hard` · *Empty struct*

struct{} has zero size, making it the idiomatic value for sets (map[T]struct{}) and pure signaling.

**Steps:**

1. unsafe.Sizeof(struct{}{}) is 0.
2. Use it as a map value to build a memory-free set.

```go
package main

import (
	"fmt"
	"sort"
	"unsafe"
)

func main() {
	fmt.Println("size of struct{}:", unsafe.Sizeof(struct{}{}))

	set := map[string]struct{}{}
	for _, w := range []string{"a", "b", "a"} {
		set[w] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("unique:", keys)
}
```

**Output:**

```
size of struct{}: 0
unique: [a b]
```

---

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
