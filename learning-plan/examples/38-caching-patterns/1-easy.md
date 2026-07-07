# 38 · Easy (1–5) — cache-aside, TTL, invalidation

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Cache-aside (lazy loading)

Check the cache; on a miss, load from the source and populate. The most common caching pattern —
resilient (cache down → just slower).

```go
package main

import "fmt"

type DB struct{ calls int }

func (d *DB) get(key string) string { d.calls++; return "value-of-" + key }

type Cache struct {
	db    *DB
	store map[string]string
}

func newCache(db *DB) *Cache { return &Cache{db: db, store: map[string]string{}} }

func (c *Cache) Get(key string) string {
	if v, ok := c.store[key]; ok {
		return v // 1. cache hit
	}
	v := c.db.get(key) // 2. miss → load the source of truth
	c.store[key] = v   // 3. populate
	return v
}

func main() {
	db := &DB{}
	c := newCache(db)
	fmt.Println(c.Get("x")) // miss
	fmt.Println(c.Get("x")) // hit
	fmt.Println(c.Get("y")) // miss
	fmt.Println("db calls:", db.calls)
}
```

**Output**
```
value-of-x
value-of-x
value-of-y
db calls: 2
```

---

## 2. Hits vs misses

The first read of a key is a **miss** (loads); subsequent reads are **hits** (served from the cache).

```go
package main

import "fmt"

type DB struct{ calls int }

func (d *DB) get(key string) string { d.calls++; return "V" + key }

type Cache struct {
	db    *DB
	store map[string]string
}

func newCache(db *DB) *Cache { return &Cache{db: db, store: map[string]string{}} }

func (c *Cache) Get(key string) (string, string) {
	if v, ok := c.store[key]; ok {
		return v, "HIT"
	}
	v := c.db.get(key)
	c.store[key] = v
	return v, "MISS"
}

func main() {
	c := newCache(&DB{})
	for _, k := range []string{"a", "a", "b", "a"} {
		v, status := c.Get(k)
		fmt.Printf("get %s → %s (%s)\n", k, v, status)
	}
}
```

**Output**
```
get a → Va (MISS)
get a → Va (HIT)
get b → Vb (MISS)
get a → Va (HIT)
```

---

## 3. Invalidate on write

On an update, delete the key (simplest, safest) so the next read repopulates from the source.

```go
package main

import "fmt"

type DB struct {
	data  map[string]string
	calls int
}

func (d *DB) get(key string) string { d.calls++; return d.data[key] }
func (d *DB) set(key, val string)   { d.data[key] = val }

type Cache struct {
	db    *DB
	store map[string]string
}

func (c *Cache) Get(key string) string {
	if v, ok := c.store[key]; ok {
		return v
	}
	v := c.db.get(key)
	c.store[key] = v
	return v
}

func (c *Cache) Update(key, val string) {
	c.db.set(key, val)
	delete(c.store, key) // invalidate — next read repopulates
}

func main() {
	db := &DB{data: map[string]string{"x": "old"}}
	c := &Cache{db: db, store: map[string]string{}}
	fmt.Println("read:", c.Get("x")) // old (miss → cached)
	c.Update("x", "new")             // write + invalidate
	fmt.Println("read:", c.Get("x")) // new (reloaded)
	fmt.Println("db reads:", db.calls)
}
```

**Output**
```
read: old
read: new
db reads: 2
```

---

## 4. A TTL cache

A TTL cache with an **injectable clock** (so the demo is deterministic). An expired entry is treated
as a miss — always set a TTL as a safety net against invalidation you missed.

```go
package main

import "fmt"

type entry struct {
	val string
	exp int64 // expiry "time"
}

type TTLCache struct {
	now   int64
	store map[string]entry
}

func (c *TTLCache) Set(key, val string, ttl int64) {
	c.store[key] = entry{val: val, exp: c.now + ttl}
}

func (c *TTLCache) Get(key string) (string, bool) {
	e, ok := c.store[key]
	if !ok || c.now >= e.exp {
		return "", false // missing or expired
	}
	return e.val, true
}

func main() {
	c := &TTLCache{now: 0, store: map[string]entry{}}
	c.Set("x", "hello", 10) // expires at t=10
	v, ok := c.Get("x")
	fmt.Printf("t=0:  %q ok=%v\n", v, ok)
	c.now = 15 // clock advances past expiry
	v, ok = c.Get("x")
	fmt.Printf("t=15: %q ok=%v\n", v, ok)
}
```

**Output**
```
t=0:  "hello" ok=true
t=15: "" ok=false
```

---

## 5. Negative caching

Cache a "not found" (short-lived) so repeated lookups of a missing key don't hammer the DB every time
(cache penetration).

```go
package main

import "fmt"

type DB struct{ calls int }

func (d *DB) get(key string) (string, bool) {
	d.calls++
	return "", false // always missing in this demo
}

type Cache struct {
	db      *DB
	present map[string]string
	missing map[string]bool
}

func newCache(db *DB) *Cache {
	return &Cache{db: db, present: map[string]string{}, missing: map[string]bool{}}
}

func (c *Cache) Get(key string) (string, bool) {
	if v, ok := c.present[key]; ok {
		return v, true
	}
	if c.missing[key] {
		return "", false // negative-cache hit — no DB call
	}
	v, ok := c.db.get(key)
	if !ok {
		c.missing[key] = true
		return "", false
	}
	c.present[key] = v
	return v, true
}

func main() {
	db := &DB{}
	c := newCache(db)
	c.Get("ghost")
	c.Get("ghost")
	c.Get("ghost")
	fmt.Println("db calls for a missing key:", db.calls) // 1, not 3
}
```

**Output**
```
db calls for a missing key: 1
```

---

Next tier → [Medium (6–10)](2-medium.md)
