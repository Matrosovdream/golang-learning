# 48 · Easy (1–5) — atomics & zero-copy I/O

Back to [index](README.md) · Next tier: [Medium](2-medium.md)

---

## 1. Copy-on-write config with `atomic.Pointer`

When state is read constantly and written rarely (config, routing tables, feature flags), skip the mutex:
readers `Load` a snapshot **wait-free**, and the writer `Store`s a brand-new one. The published value is
never mutated in place. (Run under `-race` to confirm.)

```go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Timeout time.Duration
	Version int
}

// Readers Load a snapshot lock-free; the writer Stores a fresh one. The pointed-to
// Config is never mutated after publishing — replaced wholesale.
var cfg atomic.Pointer[Config]

func main() {
	cfg.Store(&Config{Timeout: time.Second, Version: 1})

	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100_000; i++ {
				c := cfg.Load() // wait-free; always a whole, consistent snapshot
				_ = c.Version
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := 2; v <= 5; v++ {
			cfg.Store(&Config{Timeout: time.Second, Version: v})
			time.Sleep(time.Millisecond)
		}
	}()
	wg.Wait()

	fmt.Printf("final config version: %d\n", cfg.Load().Version)
	fmt.Println("readers loaded snapshots lock-free; each saw a whole Config, never a half-updated one")
}
```

**Output**
```
final config version: 5
readers loaded snapshots lock-free; each saw a whole Config, never a half-updated one
```

The golden rule: **never edit a published snapshot** — a reader may be looking at it. Build a new `Config`
and `Store` it. This is the safe, common face of lock-free programming.

---

## 2. A CAS retry loop (lock-free float add)

`atomic` has no float type, and some updates can't be expressed as a single `Add`. The general tool is a
**compare-and-swap loop**: read the current value, compute the new one, and swap *only if* nobody changed
it underneath — retry if they did.

```go
package main

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// Lock-free float add: atomics have no float type, so CAS on the bit pattern.
// Read the current value, compute the new one, swap only if nothing changed.
func addFloat(addr *atomic.Uint64, delta float64) {
	for {
		old := addr.Load()
		nw := math.Float64bits(math.Float64frombits(old) + delta)
		if addr.CompareAndSwap(old, nw) {
			return
		}
	}
}

func main() {
	var bits atomic.Uint64
	const goroutines, perG = 50, 10_000
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				addFloat(&bits, 0.5)
			}
		}()
	}
	wg.Wait()

	fmt.Printf("expected sum: %.1f\n", float64(goroutines*perG)*0.5)
	fmt.Printf("actual sum:   %.1f\n", math.Float64frombits(bits.Load()))
}
```

**Output**
```
expected sum: 250000.0
actual sum:   250000.0
```

Exact, with no lock. But CAS loops are subtle — under heavy contention they *spin* (wasted retries), and
they invite the ABA problem (#11). For a plain counter, `atomic.Int64.Add` is simpler; for anything more,
weigh a `sync.Mutex` first and only go lock-free with a benchmark to justify it.

---

## 3. `unsafe.String`: zero-copy `[]byte`→`string`

`string(b)` copies so the immutable string can't be changed through the slice. When you can *guarantee*
the bytes won't change for the string's lifetime, `unsafe.String` reinterprets the same memory with **no
copy** (Go 1.20+).

```go
package main

import (
	"fmt"
	"testing"
	"unsafe"
)

// Zero-copy view of bytes as a string. SAFE ONLY IF b is never mutated afterwards.
func asString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

var sink string

func main() {
	b := []byte("low-latency")

	fmt.Printf("string(b)        : %.0f allocs/op\n", testing.AllocsPerRun(100, func() { sink = string(b) }))
	fmt.Printf("unsafe.String(b) : %.0f allocs/op\n", testing.AllocsPerRun(100, func() { sink = asString(b) }))
	fmt.Printf("view: %q\n", asString(b))
	fmt.Println("(unsafe.String shares b's memory — mutating b afterwards would corrupt the string)")
}
```

**Output**
```
string(b)        : 1 allocs/op
unsafe.String(b) : 0 allocs/op
view: "low-latency"
(unsafe.String shares b's memory — mutating b afterwards would corrupt the string)
```

This is a **sharp knife**. If `b` is later written to (say it came from a `sync.Pool` buffer you reuse),
the string silently changes — a heisenbug that breaks Go's guarantee that strings are immutable. Confine
it to one audited helper over guaranteed-immutable bytes; everywhere else, use the safe conversion (the
compiler already elides the copy at the blessed sites — see [46 #8](../46-low-latency-measuring/2-medium.md#8-bytestring-conversion--the-map-lookup-elision)).

---

## 4. Stream with `io.Copy`, don't buffer with `io.ReadAll`

`io.ReadAll` pulls the *entire* stream into a growing slice — memory proportional to the payload.
`io.Copy` streams through a small fixed buffer (or a `ReaderFrom`/`WriterTo` fast path), so memory stays
flat regardless of size.

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
)

func allocMiB(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) >> 20
}

func main() {
	const size = 64 << 20 // 64 MiB
	data := make([]byte, size)

	// ReadAll buffers the ENTIRE stream into memory (a slice that grows to 64 MiB).
	readAll := allocMiB(func() {
		buf, _ := io.ReadAll(bytes.NewReader(data))
		_ = buf
	})

	// io.Copy streams: it uses ReaderFrom/WriterTo fast paths or a small fixed
	// buffer, so memory stays flat no matter how big the payload is.
	ioCopy := allocMiB(func() {
		io.Copy(io.Discard, bytes.NewReader(data))
	})

	fmt.Printf("io.ReadAll then use: %d MiB allocated\n", readAll)
	fmt.Printf("io.Copy(Discard):    %d MiB allocated\n", ioCopy)
}
```

**Output** *(the ReadAll MiB varies with the slice's growth schedule; the contrast is the point)*
```
io.ReadAll then use: 157 MiB allocated
io.Copy(Discard):    0 MiB allocated
```

`ReadAll` allocated **157 MiB** to move 64 MiB (the slice doubled several times as it grew); `io.Copy`
allocated essentially nothing. For proxies, file servers, and copies, prefer `io.Copy` — and if your `dst`
implements `ReaderFrom` or `src` implements `WriterTo`, it may copy zero times in user space (e.g.
`sendfile`).

---

## 5. `bufio` batches the syscalls

Every `Write` is a syscall in real I/O. Writing a byte at a time is thousands of them; `bufio` fills one
buffer and flushes it in a handful. We count the underlying `Write` calls with a tiny writer.

```go
package main

import (
	"bufio"
	"fmt"
)

// countWriter records how many times Write is called — a stand-in for syscalls.
type countWriter struct{ writes int }

func (w *countWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func main() {
	const n = 10_000

	// Unbuffered: every byte is its own Write call (a syscall in real I/O).
	var raw countWriter
	for i := 0; i < n; i++ {
		raw.Write([]byte{'x'})
	}

	// Buffered: bufio batches into 4096-byte chunks → far fewer underlying writes.
	var backing countWriter
	bw := bufio.NewWriter(&backing) // default 4096-byte buffer
	for i := 0; i < n; i++ {
		bw.WriteByte('x')
	}
	bw.Flush()

	fmt.Printf("unbuffered: %d underlying Write calls\n", raw.writes)
	fmt.Printf("bufio:      %d underlying Write calls\n", backing.writes)
}
```

**Output**
```
unbuffered: 10000 underlying Write calls
bufio:      3 underlying Write calls
```

10000 → 3. Wrap network and file writers in `bufio.Writer` (and readers in `bufio.Reader`) whenever you do
many small operations — just remember to `Flush` before you close. This is *batching* applied to syscalls;
#7 batches at the application level.

---

Next tier: [🟡 Medium (6–10)](2-medium.md) — amortising: batch, coalesce, reuse.
</content>
