# 38 · Hard (11–15) — two-tier, races, capstone

Back to [index](README.md) · Prev: [Medium](2-medium.md)

---

## 11. Two-tier cache (L1 + L2)

Check L1 (local), then L2 (shared "redis"), then the DB. Populate both on the way back so the hottest
keys stay closest.

```go
package main

import "fmt"

type DB struct{ calls int }

func (d *DB) get(key string) string { d.calls++; return "V" }

type TwoTier struct {
	l1 map[string]string
	l2 map[string]string
	db *DB
}

func (c *TwoTier) Get(key string) (string, string) {
	if v, ok := c.l1[key]; ok {
		return v, "L1"
	}
	if v, ok := c.l2[key]; ok {
		c.l1[key] = v
		return v, "L2"
	}
	v := c.db.get(key)
	c.l2[key], c.l1[key] = v, v
	return v, "DB"
}

func main() {
	db := &DB{}
	c := &TwoTier{l1: map[string]string{}, l2: map[string]string{}, db: db}
	_, s1 := c.Get("x") // DB
	_, s2 := c.Get("x") // L1
	delete(c.l1, "x")   // L1 evicted, still in L2
	_, s3 := c.Get("x") // L2 (repopulates L1)
	fmt.Printf("%s → %s → %s, db calls=%d\n", s1, s2, s3, db.calls)
}
```

**Output**
```
DB → L1 → L2, db calls=1
```

---

## 12. Jittered TTLs

Jitter TTLs so many keys don't expire at the same instant (which would cause a synchronized
stampede). Seeded for a reproducible demo.

```go
package main

import (
	"fmt"
	"math/rand"
)

func main() {
	r := rand.New(rand.NewSource(1))
	baseTTL := 60
	for i := 1; i <= 5; i++ {
		jitter := r.Intn(20) - 10 // ±10 seconds
		fmt.Printf("key%d: ttl=%d\n", i, baseTTL+jitter)
	}
}
```

**Output**
```
key1: ttl=51
key2: ttl=57
key3: ttl=57
key4: ttl=69
key5: ttl=51
```

---

## 13. The stale-set race

The cache-aside stale-set race, step by step: a slow reader loads the old value, a writer updates the
DB and invalidates the key, then the slow reader writes back the stale value it already fetched.

```go
package main

import "fmt"

func main() {
	db := "v1"
	cache := map[string]string{}

	readerSaw := db // 1. slow reader loads old value (hasn't populated cache yet)

	db = "v2"          // 2. writer updates the DB
	delete(cache, "x") //    and invalidates the key

	cache["x"] = readerSaw // 3. slow reader now populates with its STALE value

	fmt.Println("db:", db, "cache:", cache["x"], "→ STALE")
	fmt.Println("mitigation: short TTL, versioned keys, or write-through for hot keys")
}
```

**Output**
```
db: v2 cache: v1 → STALE
mitigation: short TTL, versioned keys, or write-through for hot keys
```

---

## 14. Namespace keys by tenant

Namespace cache keys by tenant, or one tenant's data leaks to another under a shared key.

```go
package main

import "fmt"

func key(tenant, resource string) string { return tenant + ":" + resource }

func main() {
	cache := map[string]string{}
	cache[key("acme", "user:1")] = "Alice@acme"
	cache[key("globex", "user:1")] = "Bob@globex"

	fmt.Println("acme   user:1 →", cache[key("acme", "user:1")])
	fmt.Println("globex user:1 →", cache[key("globex", "user:1")])
	// a bare shared key "user:1" would have collided and leaked across tenants
}
```

**Output**
```
acme   user:1 → Alice@acme
globex user:1 → Bob@globex
```

---

## 15. Capstone: cache-aside + TTL + metrics

Cache-aside with a TTL and hit/miss/load metrics, over an injectable clock for determinism.

```go
package main

import "fmt"

type DB struct{ loads int }

func (d *DB) load(key string) string { d.loads++; return "V" + key }

type entry struct {
	val string
	exp int64
}

type Cache struct {
	now    int64
	ttl    int64
	db     *DB
	store  map[string]entry
	hits   int
	misses int
}

func newCache(db *DB, ttl int64) *Cache {
	return &Cache{ttl: ttl, db: db, store: map[string]entry{}}
}

func (c *Cache) Get(key string) string {
	if e, ok := c.store[key]; ok && c.now < e.exp {
		c.hits++
		return e.val
	}
	c.misses++
	v := c.db.load(key)
	c.store[key] = entry{val: v, exp: c.now + c.ttl}
	return v
}

func main() {
	db := &DB{}
	c := newCache(db, 10)
	c.Get("x") // miss + load
	c.Get("x") // hit
	c.now = 20 // x expired
	c.Get("x") // miss + reload
	c.Get("y") // miss + load
	fmt.Printf("hits=%d misses=%d db_loads=%d\n", c.hits, c.misses, db.loads)
}
```

**Output**
```
hits=1 misses=3 db_loads=3
```

---

Back to [index](README.md) · Next lesson's examples: [39 — Observability: Distributed Tracing](../39-observability-tracing/README.md).
