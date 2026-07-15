# 47 · Medium (6–10) — memory limits, pooling, contention

Back to [index](README.md) · Prev tier: [Easy](1-easy.md) · Next tier: [Hard](3-hard.md)

---

## 6. `GOMEMLIMIT`: a soft cap that triggers GC

`GOMEMLIMIT` (Go 1.19+) is a **soft memory limit**: the GC runs harder as the heap approaches it. To prove
it's the limit doing the work, turn the normal `GOGC` trigger *off* with `SetGCPercent(-1)` — now the only
thing that can start a collection is the memory limit.

```go
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var live [][]byte

func main() {
	// Turn OFF the normal GOGC trigger: now only the memory limit can start a GC.
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(64 << 20) // 64 MiB soft cap

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	// Allocate ~128 MiB total in 1 MiB chunks, keeping only the last 32 MiB live.
	for i := 0; i < 128; i++ {
		live = append(live, make([]byte, 1<<20))
		if len(live) > 32 {
			live = live[len(live)-32:] // drop older chunks → they become garbage
		}
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(live)

	fmt.Printf("GC cycles with GOGC off but a 64MiB limit: %d\n", after.NumGC-before.NumGC)
	fmt.Println("(with SetGCPercent(-1) and NO limit this would be 0 — the memory limit is what forces GC)")
}
```

**Output** *(count varies; the point is it is > 0)*
```
GC cycles with GOGC off but a 64MiB limit: 23
```

With `GOGC` disabled and no limit, this would collect **zero** times and grow to 128 MiB. The limit reins
it in. In production, set `GOMEMLIMIT` a little below your container's memory cap so the GC keeps you off
the OOM-killer while `GOGC` stays high for throughput.

---

## 7. `sync.Pool`: the reset footgun

Pooling recycles objects — but a recycled object carries **stale state**. Forgetting to `Reset` is the
classic bug: the previous user's data bleeds into yours.

```go
package main

import (
	"bytes"
	"fmt"
	"sync"
)

var pool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// BUGGY: forgets to Reset, so it writes on top of whatever the previous user left.
func renderBuggy(name string) string {
	b := pool.Get().(*bytes.Buffer)
	b.WriteString("hello ")
	b.WriteString(name)
	s := b.String()
	pool.Put(b) // handed back still dirty
	return s
}

// FIXED: Reset clears the stale contents before reuse.
func renderFixed(name string) string {
	b := pool.Get().(*bytes.Buffer)
	b.Reset()
	b.WriteString("hello ")
	b.WriteString(name)
	s := b.String()
	pool.Put(b)
	return s
}

func main() {
	fmt.Println("Buggy (no Reset) — stale bytes bleed between calls:")
	fmt.Printf("  %q\n", renderBuggy("alice"))
	fmt.Printf("  %q\n", renderBuggy("bob"))
	fmt.Printf("  %q\n", renderBuggy("carol"))

	fmt.Println("Fixed (Reset on get):")
	fmt.Printf("  %q\n", renderFixed("dave"))
	fmt.Printf("  %q\n", renderFixed("erin"))
}
```

**Output**
```
Buggy (no Reset) — stale bytes bleed between calls:
  "hello alice"
  "hello alicehello bob"
  "hello alicehello bobhello carol"
Fixed (Reset on get):
  "hello dave"
  "hello erin"
```

The buggy version accumulates every previous name. The other two footguns: **never keep a reference to an
object after `Put`** (another goroutine may take it — a data race), and remember the **GC empties the
pool**, so it's for transient objects, not a long-lived cache.

---

## 8. Atomic vs mutex counter

For a single shared counter, a `sync/atomic` type gives the same correct result as a mutex with **no lock
at all** — every goroutine makes progress instead of serialising. (Run with `-race` to confirm both are
race-free.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	const goroutines, perG = 100, 10_000
	var wg sync.WaitGroup

	// Mutex-guarded counter: correct, but every increment serialises through the lock.
	var mu sync.Mutex
	muCount := 0
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				mu.Lock()
				muCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Atomic counter: same result, no lock at all.
	var atCount atomic.Int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				atCount.Add(1)
			}
		}()
	}
	wg.Wait()

	fmt.Printf("expected:       %d\n", goroutines*perG)
	fmt.Printf("mutex counter:  %d\n", muCount)
	fmt.Printf("atomic counter: %d\n", atCount.Load())
}
```

**Output**
```
expected:       1000000
mutex counter:  1000000
atomic counter: 1000000
```

Both are correct; #12 benchmarks the *speed* difference under contention. Reach for atomics for simple
counters and flags; use a mutex when you must update several fields together as one invariant.

---

## 9. Sharded (striped) counter

When even an atomic counter is too contended (all cores hammering one cache line), **shard** it: each
goroutine bumps its own cell, and you sum the cells at the end. Contention drops by roughly the shard
count.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const shards = 16

// A striped counter: each goroutine hits its own shard, so increments rarely
// collide. Each cell is padded to a full cache line to avoid false sharing (#13).
type Sharded struct {
	cells [shards]struct {
		n atomic.Int64
		_ [56]byte
	}
}

func (s *Sharded) Add(id int) { s.cells[id%shards].n.Add(1) }

func (s *Sharded) Sum() int64 {
	var total int64
	for i := range s.cells {
		total += s.cells[i].n.Load()
	}
	return total
}

func main() {
	const goroutines, perG = 64, 100_000
	var c Sharded
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				c.Add(id)
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("expected: %d\n", goroutines*perG)
	fmt.Printf("sum:      %d\n", c.Sum())
}
```

**Output**
```
expected: 6400000
sum:      6400000
```

The trade-off: reads (`Sum`) now cost O(shards), so sharding fits **write-heavy, read-rarely** counters
(metrics, request tallies). The `[56]byte` padding is essential — without it the shards share cache lines
and you're back to contention (that's #13).

---

## 10. `RWMutex` for read-mostly data

When reads vastly outnumber writes, a `sync.RWMutex` lets any number of readers hold `RLock` **in
parallel**; only the rare writer takes the exclusive `Lock`.

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// A cache read by many goroutines, written rarely. RWMutex lets readers run in
// parallel; only the occasional writer is exclusive.
type Cache struct {
	mu sync.RWMutex
	m  map[string]int
}

func (c *Cache) Get(k string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[k]
	return v, ok
}

func (c *Cache) Set(k string, v int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
}

func main() {
	c := &Cache{m: map[string]int{"x": 0}}
	const readers, reads = 50, 100_000

	var wg sync.WaitGroup
	var hits atomic.Int64
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := 0
			for j := 0; j < reads; j++ {
				if _, ok := c.Get("x"); ok {
					local++
				}
			}
			hits.Add(int64(local))
		}()
	}
	// One writer bumping the value concurrently with all the readers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := 0; v < 1000; v++ {
			c.Set("x", v)
		}
	}()
	wg.Wait()

	fmt.Printf("reads served: %d (expected %d)\n", hits.Load(), readers*reads)
	fmt.Println("many RLock readers ran in parallel; the writer held Lock exclusively")
}
```

**Output**
```
reads served: 5000000 (expected 5000000)
```
```
many RLock readers ran in parallel; the writer held Lock exclusively
```

`RWMutex` shines only when reads dominate *and* the critical section is non-trivial — the read/write
bookkeeping makes it slower than a plain `Mutex` for very short or write-heavy sections. Measure before
switching.

---

Next tier: [🔴 Hard (11–15)](3-hard.md) — benchmarks, profiling, and a capstone.
</content>
