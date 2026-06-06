# Step 13 — Goroutines · 🟢 Easy

Examples **1–5**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Start a goroutine and wait for it

`🟢 easy` · *Basics*

Prefix a call with go to run it concurrently; main must explicitly wait, here with a sync.WaitGroup, or it could exit first.

**Steps:**

1. wg.Add(1) before launching, defer wg.Done() inside.
2. wg.Wait() blocks until the goroutine finishes, so the order is deterministic.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("goroutine: working")
	}()
	wg.Wait() // block until Done is called
	fmt.Println("main: goroutine finished")
}
```

**Output:**

```
goroutine: working
main: goroutine finished
```

---

## 2. Why main must wait

`🟢 easy` · *Basics*

When main returns the program exits immediately, killing running goroutines — so `go fmt.Println(...)` alone is unreliable. Wait explicitly (never with time.Sleep).

**Steps:**

1. The commented bare `go` call might never run.
2. A WaitGroup guarantees the goroutine completes first.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	// Unreliable — main may exit before this runs:
	//   go fmt.Println("hi")
	// Fix: wait explicitly with a WaitGroup.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("hi (reliable because main waits)")
	}()
	wg.Wait()
}
```

**Output:**

```
hi (reliable because main waits)
```

---

## 3. Many goroutines, results in an indexed slice

`🟢 easy` · *WaitGroup*

When each goroutine writes to its OWN index of a slice, there's no data race and no need for locks — and the result is deterministic.

**Steps:**

1. Launch n goroutines; goroutine i writes results[i].
2. wg.Wait(), then print the filled slice.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const n = 5
	results := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = i * i // distinct index per goroutine: no race
		}()
	}
	wg.Wait()
	fmt.Println(results) // [0 1 4 9 16]
}
```

**Output:**

```
[0 1 4 9 16]
```

---

## 4. Loop-variable capture (Go 1.22+)

`🟢 easy` · *WaitGroup*

Since Go 1.22 each loop iteration has its own copy of the loop variable, so a goroutine capturing i sees its own value (older Go needed it passed as an argument).

**Steps:**

1. Each goroutine appends its own i to a shared slice (guarded by a mutex).
2. Sort the result because completion order is nondeterministic.

```go
package main

import (
	"fmt"
	"sort"
	"sync"
)

func main() {
	var mu sync.Mutex
	var got []int
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			got = append(got, i) // each goroutine's own i (Go 1.22+)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Ints(got) // order varies, so sort for stable output
	fmt.Println(got)
}
```

**Output:**

```
[0 1 2]
```

---

## 5. Anonymous goroutine with an argument

`🟢 easy` · *Basics*

You can launch an anonymous function as a goroutine and pass it arguments at the call.

**Steps:**

1. go func(msg string){...}("...") starts immediately.
2. Passing the value as an argument captures it explicitly.

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func(msg string) {
		defer wg.Done()
		fmt.Println(msg)
	}("hello from a goroutine")
	wg.Wait()
}
```

**Output:**

```
hello from a goroutine
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
