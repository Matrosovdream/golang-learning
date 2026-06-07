# Step 14 — Channels · 🟢 Easy

Examples **1–10**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Unbuffered send and receive (rendezvous)

`🟢 easy` · *Basics*

An unbuffered channel is a synchronization point: the send blocks until another goroutine is ready to receive, so the handoff is guaranteed to have happened.

**Steps:**

1. `make(chan int)` with no size is unbuffered.
2. The goroutine's send and `main`'s receive "rendezvous" — neither proceeds without the other.

```go
package main

import "fmt"

func main() {
	ch := make(chan int) // unbuffered
	go func() {
		ch <- 42 // blocks until main is ready to receive
	}()
	v := <-ch // receive — the send and receive "rendezvous"
	fmt.Println("received:", v)
}
```

**Output:**

```
received: 42
```

---

## 2. Buffered channel holds values

`🟢 easy` · *Buffered*

A buffered channel stores up to its capacity without a receiver. Sends only block when the buffer is full; receives only block when it's empty.

**Steps:**

1. `make(chan string, 2)` holds 2 values.
2. Two sends succeed with no receiver; draining brings `len` back to 0.

```go
package main

import "fmt"

func main() {
	ch := make(chan string, 2) // buffered: holds 2 without a receiver
	ch <- "a"                  // does not block
	ch <- "b"                  // buffer now full
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println("len after drain:", len(ch))
}
```

**Output:**

```
a
b
len after drain: 0
```

---

## 3. A channel of any type

`🟢 easy` · *Basics*

Channels are typed conduits — they can carry any type, including your own structs. Send a value in, receive the same value out.

**Steps:**

1. `chan msg` carries `msg` structs.
2. The received value is an independent copy you now own.

```go
package main

import "fmt"

type msg struct {
	from string
	text string
}

func main() {
	ch := make(chan msg, 1) // a channel can carry any type
	ch <- msg{from: "alice", text: "hi"}
	m := <-ch
	fmt.Printf("%s says %q\n", m.from, m.text)
}
```

**Output:**

```
alice says "hi"
```

---

## 4. Producer closes, consumer ranges

`🟢 easy` · *Close & range*

The sender calls `close` to say "no more values"; the receiver's `for v := range ch` loops until the channel is closed and drained, then ends cleanly.

**Steps:**

1. The goroutine sends 5 values, then `close(ch)`.
2. `range` receives each value and stops when the channel closes.

```go
package main

import "fmt"

func main() {
	ch := make(chan int)
	go func() {
		for i := 1; i <= 5; i++ {
			ch <- i
		}
		close(ch) // the sender closes when there is no more data
	}()
	for v := range ch { // loops until ch is closed and drained
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
1 2 3 4 5 
```

---

## 5. Comma-ok receive detects a closed channel

`🟢 easy` · *Close & range*

The two-value receive `v, ok := <-ch` tells you whether the value is real: `ok` is `true` while values remain, and `false` once the channel is closed and empty.

**Steps:**

1. Buffer one value, then `close`.
2. The first receive drains the value (`ok` true); the second reports closed (`ok` false, zero value).

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 1)
	ch <- 7
	close(ch)

	v, ok := <-ch // drains the buffered value: ok is true
	fmt.Println(v, ok)
	v, ok = <-ch // closed and empty: zero value, ok is false
	fmt.Println(v, ok)
}
```

**Output:**

```
7 true
0 false
```

---

## 6. Receiving from a closed channel returns zero

`🟢 easy` · *Close & range*

Once a channel is closed and drained, every receive returns the element type's zero value immediately — it never blocks, and you can read it as many times as you like.

**Steps:**

1. `close(ch)` on an empty channel.
2. Each `<-ch` yields `""` (the zero value of `string`) without blocking.

```go
package main

import "fmt"

func main() {
	ch := make(chan string)
	close(ch)
	// Receiving from a closed channel never blocks; it returns the
	// zero value immediately, as many times as you ask.
	fmt.Printf("%q\n", <-ch)
	fmt.Printf("%q\n", <-ch)
}
```

**Output:**

```
""
""
```

---

## 7. Channel directions in signatures

`🟢 easy` · *Directions*

Restricting a channel to send-only (`chan<- T`) or receive-only (`<-chan T`) in a function signature documents intent and lets the compiler stop misuse.

**Steps:**

1. `produce` takes a send-only channel and closes it when done.
2. `consume` takes a receive-only channel and ranges over it.

```go
package main

import "fmt"

func produce(out chan<- int) { // send-only channel
	for i := 0; i < 3; i++ {
		out <- i
	}
	close(out)
}

func consume(in <-chan int) { // receive-only channel
	for v := range in {
		fmt.Println("got", v)
	}
}

func main() {
	ch := make(chan int)
	go produce(ch)
	consume(ch)
}
```

**Output:**

```
got 0
got 1
got 2
```

---

## 8. Signal completion with a done channel

`🟢 easy` · *Signaling*

A `chan struct{}` carries no data — it's a pure signal. Sending (or closing) it tells another goroutine "this happened." `struct{}{}` is the empty, zero-size value.

**Steps:**

1. The goroutine does its work, then sends on `done`.
2. `<-done` in `main` blocks until that signal arrives.

```go
package main

import "fmt"

func main() {
	done := make(chan struct{}) // signal-only channel; carries no data
	go func() {
		fmt.Println("working...")
		done <- struct{}{} // signal "I'm finished"
	}()
	<-done // block here until the goroutine signals
	fmt.Println("done")
}
```

**Output:**

```
working...
done
```

---

## 9. Sum values received from a channel

`🟢 easy` · *Close & range*

A common shape: a producer streams numbers and closes; the consumer folds them into a single result as they arrive.

**Steps:**

1. The goroutine sends each number, then closes the channel.
2. `range` accumulates into `sum` until the channel closes.

```go
package main

import "fmt"

func main() {
	nums := make(chan int)
	go func() {
		for _, n := range []int{1, 2, 3, 4, 5} {
			nums <- n
		}
		close(nums)
	}()
	sum := 0
	for n := range nums { // accumulate until the channel closes
		sum += n
	}
	fmt.Println("sum:", sum)
}
```

**Output:**

```
sum: 15
```

---

## 10. len and cap of a buffered channel

`🟢 easy` · *Buffered*

For a buffered channel, `cap` is the buffer size and `len` is how many values are currently queued. A send blocks only when `len == cap`.

**Steps:**

1. Capacity is fixed at creation (`cap` never changes).
2. `len` rises with each send and falls with each receive.

```go
package main

import "fmt"

func main() {
	ch := make(chan int, 3) // capacity 3
	ch <- 10
	ch <- 20
	fmt.Printf("len=%d cap=%d\n", len(ch), cap(ch)) // 2 queued, room for 3
	fmt.Println(<-ch)
	fmt.Printf("len=%d cap=%d\n", len(ch), cap(ch)) // one consumed
}
```

**Output:**

```
len=2 cap=3
10
len=1 cap=3
```

---

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
