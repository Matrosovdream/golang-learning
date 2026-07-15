# Step 50 — Linear Structures · 🟢 Easy

Examples **1–8**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. A slice is a stack

`🟢 easy` · *Slice / LIFO*

A stack is last-in-first-out, and a Go slice already is one: **push** with `append` (adds at the tail), and the **top** is the last element. No new type needed — the tail is the cheap end of a slice, so both operations are amortized O(1).

**Steps:**

1. Start with an empty `[]int`.
2. Push by appending; the last element is always the top.
3. Peek the top with `stack[len(stack)-1]` — it does **not** remove anything.

```go
package main

import "fmt"

func main() {
	// A slice is a stack: push at the tail with append, the top is the last element.
	stack := []int{}
	stack = append(stack, 1) // push 1
	stack = append(stack, 2) // push 2
	stack = append(stack, 3) // push 3

	fmt.Println("stack:", stack)
	fmt.Println("top:", stack[len(stack)-1]) // peek — no removal
	fmt.Println("size:", len(stack))
}
```

**Output:**

```
stack: [1 2 3]
top: 3
size: 3
```

---

## 2. Pop off the stack

`🟢 easy` · *Slice / LIFO*

Popping removes and returns the top. Reslice with `stack[:len(stack)-1]` to drop the last element. The key discipline: **check for empty first** and report it — indexing `stack[len(stack)-1]` on an empty slice **panics**, so return an `ok` boolean instead.

**Steps:**

1. Return `ok=false` when the stack is empty.
2. Otherwise read the top, then return the resliced stack (one shorter).
3. Pop in a loop until empty, then confirm a final pop reports `false`.

```go
package main

import "fmt"

// pop removes and returns the top of the stack. It returns ok=false when the
// stack is empty, so the caller never indexes an empty slice (which panics).
func pop(stack []int) (int, []int, bool) {
	if len(stack) == 0 {
		return 0, stack, false
	}
	top := stack[len(stack)-1]
	return top, stack[:len(stack)-1], true
}

func main() {
	stack := []int{1, 2, 3}
	for len(stack) > 0 {
		var v int
		var ok bool
		v, stack, ok = pop(stack)
		fmt.Println("popped:", v, "ok:", ok)
	}
	_, _, ok := pop(stack)
	fmt.Println("pop empty ok:", ok)
}
```

**Output:**

```
popped: 3 ok: true
popped: 2 ok: true
popped: 1 ok: true
pop empty ok: false
```

---

## 3. A generic Stack[T]

`🟢 easy` · *Generics*

Wrapping the slice in a `Stack[T]` gives a reusable, typed stack that works for any element type. Idiomatic Go: make the **zero value ready** — `var s Stack[int]` is an empty, usable stack, so no `NewStack` constructor is needed.

**Steps:**

1. Put the slice in a struct with a type parameter `T any`.
2. `Pop` returns `(T, bool)`; on empty it returns the zero value and `false`.
3. Zero the popped slot (`= zero`) before reslicing so the GC can reclaim it.

```go
package main

import "fmt"

// Stack is a generic LIFO stack. Its zero value (var s Stack[int]) is an empty,
// ready-to-use stack — no constructor needed.
type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }

func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	top := s.items[len(s.items)-1]
	s.items[len(s.items)-1] = zero // let the GC reclaim the element
	s.items = s.items[:len(s.items)-1]
	return top, true
}

func (s *Stack[T]) Len() int { return len(s.items) }

func main() {
	var s Stack[string] // zero value is ready
	s.Push("a")
	s.Push("b")
	s.Push("c")

	fmt.Println("len:", s.Len())
	for s.Len() > 0 {
		v, _ := s.Pop()
		fmt.Println("pop:", v)
	}
}
```

**Output:**

```
len: 3
pop: c
pop: b
pop: a
```

---

## 4. Reverse a slice with a stack

`🟢 easy` · *LIFO*

The defining trick of a stack: push everything, pop everything, and the order flips. This is the simplest useful thing LIFO does, and it generalises with a type parameter to reverse a slice of anything.

**Steps:**

1. Push every element onto a scratch stack.
2. Pop them back out — the top comes off last-in-first-out.
3. Preallocate both slices with `make(..., 0, len(in))` to avoid regrowth.

```go
package main

import "fmt"

// reverse uses a stack (LIFO) to reverse a slice: push every element, then pop
// them back out — they come out in the opposite order.
func reverse[T any](in []T) []T {
	stack := make([]T, 0, len(in))
	for _, v := range in {
		stack = append(stack, v) // push
	}
	out := make([]T, 0, len(in))
	for len(stack) > 0 {
		out = append(out, stack[len(stack)-1]) // pop the top
		stack = stack[:len(stack)-1]
	}
	return out
}

func main() {
	fmt.Println(reverse([]int{1, 2, 3, 4, 5}))
	fmt.Println(reverse([]string{"go", "is", "fun"}))
}
```

**Output:**

```
[5 4 3 2 1]
[fun is go]
```

---

## 5. A slice is a FIFO queue

`🟢 easy` · *Slice / FIFO*

A queue is first-in-first-out. A slice does this too: **enqueue** at the tail with `append`, **dequeue** from the head with `queue[1:]`. This is fine for short-lived queues — example 11 covers the memory caveat when a queue lives a long time.

**Steps:**

1. Enqueue three items at the tail.
2. Serve from the head: read `queue[0]`, then advance with `queue = queue[1:]`.
3. Items come out in the same order they went in.

```go
package main

import "fmt"

func main() {
	// A slice is a FIFO queue: enqueue at the tail, dequeue from the head.
	queue := []string{}
	queue = append(queue, "first")  // enqueue
	queue = append(queue, "second") // enqueue
	queue = append(queue, "third")  // enqueue

	for len(queue) > 0 {
		front := queue[0] // peek the head
		queue = queue[1:] // dequeue — see example 11 for the memory caveat
		fmt.Println("served:", front)
	}
	fmt.Println("empty:", len(queue) == 0)
}
```

**Output:**

```
served: first
served: second
served: third
empty: true
```

---

## 6. A generic Queue[T]

`🟢 easy` · *Generics*

The queue equivalent of example 3: a typed, reusable FIFO. Same shape as the stack, but `Dequeue` takes from the **front** (`items[0]`) instead of the back.

**Steps:**

1. `Enqueue` appends at the tail.
2. `Dequeue` returns `(T, bool)` from the head and reslices with `items[1:]`.
3. Drain it and watch values emerge in insertion order.

```go
package main

import "fmt"

// Queue is a generic FIFO queue backed by a slice. Simple and fine for
// short-lived queues; example 11 covers the long-running memory caveat.
type Queue[T any] struct {
	items []T
}

func (q *Queue[T]) Enqueue(v T) { q.items = append(q.items, v) }

func (q *Queue[T]) Dequeue() (T, bool) {
	var zero T
	if len(q.items) == 0 {
		return zero, false
	}
	front := q.items[0]
	q.items = q.items[1:]
	return front, true
}

func (q *Queue[T]) Len() int { return len(q.items) }

func main() {
	var q Queue[int]
	for i := 1; i <= 3; i++ {
		q.Enqueue(i * 10)
	}
	for q.Len() > 0 {
		v, _ := q.Dequeue()
		fmt.Println("dequeue:", v)
	}
}
```

**Output:**

```
dequeue: 10
dequeue: 20
dequeue: 30
```

---

## 7. Peek, Len & IsEmpty

`🟢 easy` · *API design*

A well-behaved container tells you about itself without forcing a panic. `Len` and `IsEmpty` let callers check before acting; `Peek` returns `(T, bool)` so "look at the top of an empty stack" is a safe, testable `false` rather than an out-of-range crash.

**Steps:**

1. Add `Len`, `IsEmpty`, and a `Peek` that returns `(T, bool)`.
2. Call `Peek` on the empty stack — it returns `false`, no panic.
3. Push two, then `Peek` sees the top without changing the length.

```go
package main

import "fmt"

type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T)      { s.items = append(s.items, v) }
func (s *Stack[T]) Len() int      { return len(s.items) }
func (s *Stack[T]) IsEmpty() bool { return len(s.items) == 0 }

// Peek returns the top without removing it. ok=false on an empty stack means the
// caller never has to risk an out-of-range panic.
func (s *Stack[T]) Peek() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

func main() {
	var s Stack[int]
	fmt.Println("empty?", s.IsEmpty())

	_, ok := s.Peek()
	fmt.Println("peek empty ok:", ok)

	s.Push(7)
	s.Push(8)
	v, ok := s.Peek()
	fmt.Println("peek:", v, "ok:", ok, "len:", s.Len())
	fmt.Println("empty?", s.IsEmpty())
}
```

**Output:**

```
empty? true
peek empty ok: false
peek: 8 ok: true len: 2
empty? false
```

---

## 8. A singly linked list node

`🟢 easy` · *Pointers*

A linked list is a pointer struct — the same shape as a tree node from [42](../42-trees/), minus one child. Each `node` holds a value and a `next` pointer; a `nil` next means "end of the list". You build it by hand and traverse by following `next` until `nil`.

**Steps:**

1. Define `node` with `val` and `next *node`.
2. Link three nodes tail-first so each points at the one after it.
3. Traverse with `for n := head; n != nil; n = n.next`.

```go
package main

import "fmt"

// node is one link of a singly linked list: a value plus a pointer to the next
// node. A nil next means "end of list" — the same nil-as-empty idea as a tree.
type node struct {
	val  int
	next *node
}

func main() {
	// Build 1 -> 2 -> 3 by hand, from the tail back to the head.
	third := &node{val: 3}
	second := &node{val: 2, next: third}
	head := &node{val: 1, next: second}

	// Traverse: walk next pointers until nil.
	for n := head; n != nil; n = n.next {
		fmt.Print(n.val)
		if n.next != nil {
			fmt.Print(" -> ")
		}
	}
	fmt.Println()
}
```

**Output:**

```
1 -> 2 -> 3
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
