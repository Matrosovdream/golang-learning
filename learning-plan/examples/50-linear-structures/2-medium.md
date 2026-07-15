# Step 50 — Linear Structures · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟢 easy](1-easy.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Balanced brackets with a stack

`🟡 medium` · *Stack*

The canonical stack problem: are `()[]{}` matched and properly nested? Push every opener; on a closer, the top of the stack **must** be its matching opener. If it isn't (or the stack is empty), it's unbalanced; a non-empty stack at the end means unclosed openers.

**Steps:**

1. Map each closer to its opener.
2. Push openers; on a closer, compare with the top and pop, or fail.
3. Balanced iff the stack is empty at the end.

```go
package main

import "fmt"

// balanced reports whether every opening bracket has a matching closer in the
// right order. A stack is the natural fit: push openers, pop on closers.
func balanced(s string) bool {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	var stack []rune
	for _, r := range s {
		switch r {
		case '(', '[', '{':
			stack = append(stack, r) // push opener
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return false // nothing to match, or the wrong opener
			}
			stack = stack[:len(stack)-1] // pop the match
		}
	}
	return len(stack) == 0 // leftover openers => unbalanced
}

func main() {
	for _, s := range []string{"()", "([{}])", "(]", "((", "a(b)c[d]"} {
		fmt.Printf("%-10q %v\n", s, balanced(s))
	}
}
```

**Output:**

```
"()"       true
"([{}])"   true
"(]"       false
"(("       false
"a(b)c[d]" true
```

---

## 10. Evaluate a postfix (RPN) expression

`🟡 medium` · *Stack*

Reverse-Polish notation needs no parentheses because a stack tracks pending operands. Push numbers; on an operator, pop the top **two**, apply, and push the result. A valid expression leaves exactly one value — the answer.

**Steps:**

1. On a number token, push it.
2. On `+ - * /`, pop `b` then `a` (order matters for `-` and `/`), compute, push.
3. Return the single remaining value.

```go
package main

import (
	"fmt"
	"strconv"
)

// evalRPN evaluates a reverse-Polish (postfix) expression. Operands are pushed;
// an operator pops its two operands and pushes the result. The stack ends with
// exactly one value: the answer.
func evalRPN(tokens []string) (int, error) {
	var stack []int
	for _, tok := range tokens {
		switch tok {
		case "+", "-", "*", "/":
			if len(stack) < 2 {
				return 0, fmt.Errorf("not enough operands for %q", tok)
			}
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2] // pop both
			var r int
			switch tok {
			case "+":
				r = a + b
			case "-":
				r = a - b
			case "*":
				r = a * b
			case "/":
				r = a / b
			}
			stack = append(stack, r) // push result
		default:
			n, err := strconv.Atoi(tok)
			if err != nil {
				return 0, fmt.Errorf("bad token %q", tok)
			}
			stack = append(stack, n)
		}
	}
	if len(stack) != 1 {
		return 0, fmt.Errorf("invalid expression")
	}
	return stack[0], nil
}

func main() {
	// (3 + 4) * 2 = 14
	r, _ := evalRPN([]string{"3", "4", "+", "2", "*"})
	fmt.Println("3 4 + 2 * =", r)
	// 5 1 2 + 4 * + 3 - = 14
	r, _ = evalRPN([]string{"5", "1", "2", "+", "4", "*", "+", "3", "-"})
	fmt.Println("5 1 2 + 4 * + 3 - =", r)
}
```

**Output:**

```
3 4 + 2 * = 14
5 1 2 + 4 * + 3 - = 14
```

---

## 11. A queue that doesn't leak (head index)

`🟡 medium` · *Memory*

The naive `q = q[1:]` dequeue never frees the popped elements — the backing array keeps every one alive and grows forever in a long-running loop. The fix: track a `head` index instead of reslicing the front, and **compact** (copy the live tail to the front) once the dead prefix gets large. Memory stays bounded.

**Steps:**

1. `Enqueue` appends; `Len` is `len(buf) - head`.
2. `Dequeue` reads `buf[head]`, zeroes that slot for the GC, and advances `head`.
3. When `head` passes half the buffer, `copy` the live tail down and reset `head` — watch `backing len` shrink.

```go
package main

import "fmt"

// HeadQueue is a FIFO queue that avoids the q = q[1:] memory leak. Dequeue moves
// a head index instead of reslicing; when the dead prefix grows large we compact
// by copying the live tail to the front so the backing array stays bounded.
type HeadQueue[T any] struct {
	buf  []T
	head int
}

func (q *HeadQueue[T]) Enqueue(v T) { q.buf = append(q.buf, v) }

func (q *HeadQueue[T]) Len() int { return len(q.buf) - q.head }

func (q *HeadQueue[T]) Dequeue() (T, bool) {
	var zero T
	if q.head >= len(q.buf) {
		return zero, false
	}
	v := q.buf[q.head]
	q.buf[q.head] = zero // release the element for the GC
	q.head++
	// Compact once the dead prefix is at least half the buffer.
	if q.head*2 >= len(q.buf) {
		n := copy(q.buf, q.buf[q.head:]) // slide live items to the front
		q.buf = q.buf[:n]
		q.head = 0
	}
	return v, true
}

func main() {
	var q HeadQueue[int]
	for i := 1; i <= 6; i++ {
		q.Enqueue(i)
	}
	for q.Len() > 0 {
		v, _ := q.Dequeue()
		fmt.Printf("served %d, remaining %d, backing len %d\n", v, q.Len(), len(q.buf))
	}
}
```

**Output:**

```
served 1, remaining 5, backing len 6
served 2, remaining 4, backing len 6
served 3, remaining 3, backing len 3
served 4, remaining 2, backing len 3
served 5, remaining 1, backing len 1
served 6, remaining 0, backing len 0
```

---

## 12. A ring-buffer queue

`🟡 medium` · *Ring buffer*

A ring (circular) buffer is a fixed slice used as a queue: `head` marks the front, a running `count` gives the size, and the physical tail slot is `(head+count) % cap`. Indices wrap around, so enqueue/dequeue are O(1) with **zero allocation** and memory that never grows past the capacity — the structure behind bounded work queues.

**Steps:**

1. Allocate `make([]T, capacity)` once; track `head` and `count`.
2. Enqueue at `(head+count) % len(buf)`; refuse when `Full`.
3. Dequeue reads `buf[head]`, zeroes it, advances `head` with `% len(buf)`, and drops `count`.

```go
package main

import "fmt"

// Ring is a fixed-capacity FIFO queue. head/tail indices wrap around with % cap,
// so enqueue and dequeue are O(1) with zero per-operation allocation and the
// memory never grows past the capacity.
type Ring[T any] struct {
	buf         []T
	head, count int
}

func NewRing[T any](capacity int) *Ring[T] {
	return &Ring[T]{buf: make([]T, capacity)}
}

func (r *Ring[T]) Full() bool { return r.count == len(r.buf) }
func (r *Ring[T]) Len() int   { return r.count }

func (r *Ring[T]) Enqueue(v T) bool {
	if r.Full() {
		return false
	}
	tail := (r.head + r.count) % len(r.buf) // wrap to the physical slot
	r.buf[tail] = v
	r.count++
	return true
}

func (r *Ring[T]) Dequeue() (T, bool) {
	var zero T
	if r.count == 0 {
		return zero, false
	}
	v := r.buf[r.head]
	r.buf[r.head] = zero
	r.head = (r.head + 1) % len(r.buf)
	r.count--
	return v, true
}

func main() {
	r := NewRing[int](3)
	fmt.Println("enqueue 1,2,3:", r.Enqueue(1), r.Enqueue(2), r.Enqueue(3))
	fmt.Println("enqueue 4 (full):", r.Enqueue(4))

	v, _ := r.Dequeue()
	fmt.Println("dequeue:", v, "-> now enqueue 4:", r.Enqueue(4)) // reuses the freed slot

	for r.Len() > 0 {
		v, _ := r.Dequeue()
		fmt.Print(v, " ")
	}
	fmt.Println()
}
```

**Output:**

```
enqueue 1,2,3: true true true
enqueue 4 (full): false
dequeue: 1 -> now enqueue 4: true
2 3 4 
```

---

## 13. A double-ended queue (deque)

`🟡 medium` · *Deque*

A deque supports push and pop at **both** ends. A slice handles the tail cheaply; the front operations here are O(n) because inserting/removing at index 0 shifts the backing array. That's fine for light use — reach for a ring buffer or `container/list` (example 16) when front traffic is heavy.

**Steps:**

1. `PushBack`/`PopBack` work at the slice tail.
2. `PushFront` prepends (`append([]T{v}, items...)`); `PopFront` reslices from the head.
3. Push both ends, then pop both ends and confirm the middle remains.

```go
package main

import "fmt"

// Deque is a double-ended queue. PushBack/PopBack use the cheap tail of the
// slice; PushFront/PopFront work at the head. (Front ops are O(n) here — for
// heavy front-and-back traffic use a ring buffer or container/list.)
type Deque[T any] struct {
	items []T
}

func (d *Deque[T]) PushBack(v T)  { d.items = append(d.items, v) }
func (d *Deque[T]) PushFront(v T) { d.items = append([]T{v}, d.items...) }
func (d *Deque[T]) Len() int      { return len(d.items) }

func (d *Deque[T]) PopBack() (T, bool) {
	var zero T
	if len(d.items) == 0 {
		return zero, false
	}
	v := d.items[len(d.items)-1]
	d.items = d.items[:len(d.items)-1]
	return v, true
}

func (d *Deque[T]) PopFront() (T, bool) {
	var zero T
	if len(d.items) == 0 {
		return zero, false
	}
	v := d.items[0]
	d.items = d.items[1:]
	return v, true
}

func main() {
	var d Deque[int]
	d.PushBack(2)
	d.PushBack(3)
	d.PushFront(1) // 1, 2, 3
	fmt.Println("len:", d.Len())

	front, _ := d.PopFront()
	back, _ := d.PopBack()
	fmt.Println("front:", front, "back:", back) // 1, 3
	mid, _ := d.PopFront()
	fmt.Println("remaining:", mid) // 2
}
```

**Output:**

```
len: 3
front: 1 back: 3
remaining: 2
```

---

## 14. A singly linked list

`🟡 medium` · *Linked list*

A real linked list wraps the `node` in a `List` type with a `head` pointer. **Prepend** is O(1) (the new node points at the old head); **Append** is O(n) (walk to the last node first). Indexing is O(n) — linked lists trade random access for O(1) splicing.

**Steps:**

1. `Prepend` makes a new head; `Append` walks to the tail then links.
2. `Length` and `Find` both traverse from `head` until `nil`.
3. A `String` method (satisfying `fmt.Stringer`) prints the chain.

```go
package main

import "fmt"

type node struct {
	val  int
	next *node
}

// List is a singly linked list with a head pointer. The zero value (var l List)
// is an empty list.
type List struct {
	head *node
}

// Prepend adds to the front in O(1): the new node points at the old head.
func (l *List) Prepend(v int) {
	l.head = &node{val: v, next: l.head}
}

// Append adds to the end in O(n): walk to the last node, then link.
func (l *List) Append(v int) {
	n := &node{val: v}
	if l.head == nil {
		l.head = n
		return
	}
	cur := l.head
	for cur.next != nil {
		cur = cur.next
	}
	cur.next = n
}

func (l *List) Length() int {
	count := 0
	for n := l.head; n != nil; n = n.next {
		count++
	}
	return count
}

func (l *List) Find(v int) bool {
	for n := l.head; n != nil; n = n.next {
		if n.val == v {
			return true
		}
	}
	return false
}

func (l *List) String() string {
	s := ""
	for n := l.head; n != nil; n = n.next {
		if s != "" {
			s += " -> "
		}
		s += fmt.Sprint(n.val)
	}
	return s
}

func main() {
	var l List
	l.Append(2)
	l.Append(3)
	l.Prepend(1) // 1 -> 2 -> 3
	fmt.Println("list:", l.String())
	fmt.Println("length:", l.Length())
	fmt.Println("find 2:", l.Find(2), "| find 9:", l.Find(9))
}
```

**Output:**

```
list: 1 -> 2 -> 3
length: 3
find 2: true | find 9: false
```

---

## 15. Reverse a linked list

`🟡 medium` · *Pointers*

The classic pointer-rewiring exercise. Walk the list with three pointers — `prev`, `cur`, and a saved `next` — flipping each `next` to point **backward**, in place. O(n) time, O(1) extra space. Miss the saved `next` and you lose the rest of the list; this is the drill that makes pointers click.

**Steps:**

1. Start `prev = nil`, `cur = head`.
2. Each step: save `next`, point `cur.next` at `prev`, then advance `prev` and `cur`.
3. When `cur` hits `nil`, `prev` is the new head.

```go
package main

import "fmt"

type node struct {
	val  int
	next *node
}

// reverse rewires the next pointers in place. Walk the list carrying prev; each
// step points the current node back at prev, then advances. O(n) time, O(1) space.
func reverse(head *node) *node {
	var prev *node
	cur := head
	for cur != nil {
		next := cur.next // remember the rest before we overwrite
		cur.next = prev  // flip the link
		prev = cur       // advance prev
		cur = next       // advance cur
	}
	return prev // new head
}

func build(vals ...int) *node {
	var head *node
	for i := len(vals) - 1; i >= 0; i-- {
		head = &node{val: vals[i], next: head}
	}
	return head
}

func show(head *node) {
	for n := head; n != nil; n = n.next {
		fmt.Print(n.val)
		if n.next != nil {
			fmt.Print(" -> ")
		}
	}
	fmt.Println()
}

func main() {
	head := build(1, 2, 3, 4, 5)
	fmt.Print("before: ")
	show(head)
	head = reverse(head)
	fmt.Print("after:  ")
	show(head)
}
```

**Output:**

```
before: 1 -> 2 -> 3 -> 4 -> 5
after:  5 -> 4 -> 3 -> 2 -> 1
```

---

## 16. container/list as a queue/deque

`🟡 medium` · *stdlib*

You rarely hand-roll a doubly linked list — the standard library ships one in `container/list`. It gives O(1) push/pop at both ends and O(1) removal of any element you hold, so it works as a queue or a deque. The catch: values are stored as `any`, so you **type-assert** on the way out.

**Steps:**

1. `list.New()`; `PushBack` to enqueue, `PushFront` for deque-style front insertion.
2. Read `Front().Value` and type-assert it (`.(string)`).
3. `Remove(front)` to dequeue; loop while `Len() > 0`.

```go
package main

import (
	"container/list"
	"fmt"
)

func main() {
	// container/list is a doubly linked list: O(1) push/pop at BOTH ends, so it
	// works as a queue or a deque. Values are stored as any, so we type-assert.
	l := list.New()
	l.PushBack("a") // enqueue at the tail
	l.PushBack("b")
	l.PushFront("z") // or push at the front (deque)

	// FIFO drain: repeatedly remove the front element.
	for l.Len() > 0 {
		front := l.Front()
		fmt.Println("serve:", front.Value.(string))
		l.Remove(front)
	}
}
```

**Output:**

```
serve: z
serve: a
serve: b
```

---

## 17. container/ring for a circular buffer

`🟡 medium` · *stdlib*

`container/ring` is a fixed-size **circular** doubly linked list — the last element links back to the first. It's the natural fit for round-robin scheduling or a fixed sliding window. `Next`/`Prev` step around it, `Move(n)` jumps, and `Do` visits every element once.

**Steps:**

1. `ring.New(5)` makes a 5-element ring; fill it by writing `Value` and stepping `Next`.
2. `Do` walks the whole ring — sum the values.
3. `Move(2)` advances the current pointer; read three in a row, wrapping around.

```go
package main

import (
	"container/ring"
	"fmt"
)

func main() {
	// container/ring is a fixed-size circular list: the last element links back
	// to the first. Handy for round-robin and fixed sliding windows.
	r := ring.New(5)
	for i := 0; i < r.Len(); i++ {
		r.Value = i + 1 // 1..5 around the ring
		r = r.Next()
	}

	// Do calls f once per element, walking the whole ring back to the start.
	sum := 0
	r.Do(func(v any) { sum += v.(int) })
	fmt.Println("sum around ring:", sum)

	// Move the "current" pointer forward 2 and read three in a row (wrapping).
	r = r.Move(2)
	for i := 0; i < 3; i++ {
		fmt.Print(r.Value.(int), " ")
		r = r.Next()
	}
	fmt.Println()
}
```

**Output:**

```
sum around ring: 15
3 4 5 
```

---

> Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md) · Back to the [index](README.md)
