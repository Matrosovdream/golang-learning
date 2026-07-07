# 38 · Medium (6–10) — write policies, LRU, stampede

Back to [index](README.md) · Prev: [Easy](1-easy.md) · Next: [Hard](3-hard.md)

---

## 6. Write-through

Writes go to the cache **and** the DB synchronously, so the cache is never stale for a written key.
Cost: every write pays the cache write.

```go
package main

import "fmt"

type DB struct{ data map[string]string }

type Cache struct {
	db    *DB
	store map[string]string
}

func (c *Cache) Set(key, val string) {
	c.store[key] = val
	c.db.data[key] = val
}

func main() {
	db := &DB{data: map[string]string{}}
	c := &Cache{db: db, store: map[string]string{}}
	c.Set("x", "1")
	fmt.Println("cache:", c.store["x"])
	fmt.Println("db:   ", db.data["x"])
}
```

**Output**
```
cache: 1
db:    1
```

---

## 7. Write-behind

Writes hit the cache and are flushed to the DB asynchronously. Fast, but a crash before the flush
**loses data** — so only for loss-tolerant data (counters, metrics).

```go
package main

import "fmt"

type DB struct{ data map[string]int }

type Cache struct {
	db      *DB
	store   map[string]int
	pending map[string]int // not yet flushed
}

func (c *Cache) Incr(key string) {
	c.store[key]++
	c.pending[key] = c.store[key]
}

func (c *Cache) Flush() {
	for k, v := range c.pending {
		c.db.data[k] = v
	}
	c.pending = map[string]int{}
}

func main() {
	db := &DB{data: map[string]int{}}
	c := &Cache{db: db, store: map[string]int{}, pending: map[string]int{}}
	c.Incr("hits")
	c.Incr("hits")
	fmt.Println("cache:", c.store["hits"], "db before flush:", db.data["hits"]) // lost on crash
	c.Flush()
	fmt.Println("db after flush:", db.data["hits"])
}
```

**Output**
```
cache: 2 db before flush: 0
db after flush: 2
```

---

## 8. LRU eviction

A bounded LRU cache using `container/list`: on overflow, evict the least-recently-used entry (the back
of the list).

```go
package main

import (
	"container/list"
	"fmt"
)

type kv struct{ key, val string }

type LRU struct {
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

func newLRU(capacity int) *LRU {
	return &LRU{cap: capacity, ll: list.New(), items: map[string]*list.Element{}}
}

func (c *LRU) Get(key string) (string, bool) {
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(kv).val, true
	}
	return "", false
}

func (c *LRU) Put(key, val string) {
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value = kv{key, val}
		return
	}
	c.items[key] = c.ll.PushFront(kv{key, val})
	if c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(kv).key)
	}
}

func main() {
	c := newLRU(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Get("a")      // touch a → b is now least-recently-used
	c.Put("c", "3") // evicts b
	_, aok := c.Get("a")
	_, bok := c.Get("b")
	_, cok := c.Get("c")
	fmt.Printf("a present=%v b present=%v c present=%v\n", aok, bok, cok)
}
```

**Output**
```
a present=true b present=false c present=true
```

---

## 9. Cache stampede

When a hot key is missing, N concurrent requests all miss and hit the DB at once — the thundering
herd.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type DB struct{ calls atomic.Int64 }

func (d *DB) load(key string) string { d.calls.Add(1); return "v" }

func main() {
	db := &DB{}
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = db.load("hot") // no coordination → every goroutine loads
		}()
	}
	wg.Wait()
	fmt.Println("db loads (stampede):", db.calls.Load())
}
```

**Output**
```
db loads (stampede): 100
```

---

## 10. singleflight collapses the herd

`singleflight` collapses concurrent identical loads into **one** call; the rest wait and share the
result. (A simplified version of `golang.org/x/sync/singleflight`. The `arrived` WaitGroup only makes
the demo deterministic — it lets `main` confirm all callers registered before releasing the loader.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type call struct {
	wg  sync.WaitGroup
	val string
}

type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() string, arrived *sync.WaitGroup) string {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		arrived.Done()
		c.wg.Wait() // an in-flight call exists → wait and share its result
		return c.val
	}
	c := &call{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	arrived.Done()
	c.val = fn() // only THIS goroutine (the leader) runs fn
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	return c.val
}

func main() {
	g := &Group{m: map[string]*call{}}
	var db atomic.Int64
	release := make(chan struct{})
	loader := func() string { db.Add(1); <-release; return "v" } // blocks until released

	var arrived, done sync.WaitGroup
	arrived.Add(100)
	done.Add(100)
	for range 100 {
		go func() {
			defer done.Done()
			_ = g.Do("hot", loader, &arrived)
		}()
	}
	arrived.Wait() // all 100 have registered as leader or follower
	close(release) // let the single in-flight loader finish
	done.Wait()
	fmt.Println("db loads (singleflight):", db.Load())
}
```

**Output**
```
db loads (singleflight): 1
```

---

Next tier → [Hard (11–15)](3-hard.md)
