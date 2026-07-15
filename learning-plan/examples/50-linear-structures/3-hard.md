# Step 50 — Linear Structures · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md)

---

## 18. A min-stack (O(1) minimum)

`🔴 hard` · *Stack*

A stack that reports its smallest element in O(1). The trick: keep a **second stack** that records the running minimum at each depth. Every push stores `min(v, currentMin)`; every pop discards both, so the minimum automatically rewinds to what it was before.

**Steps:**

1. On push, append `v` to `data` and `min(v, top-of-mins)` to `mins`.
2. On pop, shrink **both** stacks by one.
3. `Min` is just the top of `mins` — watch it climb back after popping the smallest value.

```go
package main

import "fmt"

// MinStack is a stack that also reports its minimum in O(1). A parallel "mins"
// stack records the running minimum at each depth, so popping restores the
// previous minimum for free.
type MinStack struct {
	data []int
	mins []int
}

func (s *MinStack) Push(v int) {
	s.data = append(s.data, v)
	if len(s.mins) == 0 || v < s.mins[len(s.mins)-1] {
		s.mins = append(s.mins, v)
	} else {
		s.mins = append(s.mins, s.mins[len(s.mins)-1])
	}
}

func (s *MinStack) Pop() (int, bool) {
	if len(s.data) == 0 {
		return 0, false
	}
	v := s.data[len(s.data)-1]
	s.data = s.data[:len(s.data)-1]
	s.mins = s.mins[:len(s.mins)-1]
	return v, true
}

func (s *MinStack) Min() (int, bool) {
	if len(s.mins) == 0 {
		return 0, false
	}
	return s.mins[len(s.mins)-1], true
}

func main() {
	var s MinStack
	for _, v := range []int{5, 3, 7, 2, 8} {
		s.Push(v)
		m, _ := s.Min()
		fmt.Printf("push %d -> min %d\n", v, m)
	}
	s.Pop() // remove 8
	s.Pop() // remove 2 -> min should climb back to 3
	m, _ := s.Min()
	fmt.Println("after two pops, min:", m)
}
```

**Output:**

```
push 5 -> min 5
push 3 -> min 3
push 7 -> min 3
push 2 -> min 2
push 8 -> min 2
after two pops, min: 3
```

---

## 19. A queue from two stacks

`🔴 hard` · *Amortized*

You can build a FIFO queue out of two LIFO stacks. Enqueue pushes onto `in`. Dequeue pops from `out`; when `out` is empty, **pour** `in` into `out`, which reverses the order — so the oldest element ends up on top. Each element is moved at most once per stack, giving **amortized O(1)** dequeue.

**Steps:**

1. `Enqueue` pushes onto `in`.
2. `Dequeue`: if `out` is empty, drain `in` into `out` (reversing order), then pop `out`.
3. Interleave enqueues and dequeues to see FIFO order survive the transfer.

```go
package main

import "fmt"

// TwoStackQueue implements a FIFO queue using two LIFO stacks. Enqueue pushes to
// in. Dequeue pops from out; when out is empty we pour in into it, which reverses
// the order back to FIFO. Each element moves at most once per stack, so dequeue
// is amortized O(1).
type TwoStackQueue struct {
	in  []int
	out []int
}

func (q *TwoStackQueue) Enqueue(v int) { q.in = append(q.in, v) }

func (q *TwoStackQueue) Dequeue() (int, bool) {
	if len(q.out) == 0 {
		for len(q.in) > 0 { // pour in -> out, reversing order
			q.out = append(q.out, q.in[len(q.in)-1])
			q.in = q.in[:len(q.in)-1]
		}
	}
	if len(q.out) == 0 {
		return 0, false
	}
	v := q.out[len(q.out)-1]
	q.out = q.out[:len(q.out)-1]
	return v, true
}

func main() {
	var q TwoStackQueue
	q.Enqueue(1)
	q.Enqueue(2)
	d, _ := q.Dequeue() // 1
	fmt.Println("dequeue:", d)
	q.Enqueue(3)
	for {
		v, ok := q.Dequeue()
		if !ok {
			break
		}
		fmt.Println("dequeue:", v) // 2, then 3
	}
}
```

**Output:**

```
dequeue: 1
dequeue: 2
dequeue: 3
```

---

## 20. Monotonic stack: next greater element

`🔴 hard` · *Monotonic stack*

A **monotonic stack** keeps its contents in sorted order by discarding elements that can no longer be useful. For "next greater element", keep a stack of **indices** whose values decrease. When a bigger value arrives, it is the answer for every smaller index on top — pop and record them. One O(n) pass instead of the naive O(n²).

**Steps:**

1. Default every answer to `-1`.
2. For each value, while it beats the value at the stack's top index, pop and set that index's answer to the current value.
3. Push the current index; the stack stays decreasing.

```go
package main

import "fmt"

// nextGreater returns, for each element, the next element to its right that is
// larger (or -1 if none). A monotonic (decreasing) stack of indices does it in
// one O(n) pass: when the current value beats the stack top, it is that top's
// answer.
func nextGreater(nums []int) []int {
	res := make([]int, len(nums))
	for i := range res {
		res[i] = -1
	}
	var stack []int // holds indices, values decreasing from bottom to top
	for i, v := range nums {
		for len(stack) > 0 && nums[stack[len(stack)-1]] < v {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			res[top] = v // v is the next-greater for that earlier index
		}
		stack = append(stack, i)
	}
	return res
}

func main() {
	nums := []int{2, 1, 5, 3, 4}
	fmt.Println("nums:", nums)
	fmt.Println("next greater:", nextGreater(nums))
}
```

**Output:**

```
nums: [2 1 5 3 4]
next greater: [5 5 -1 4 -1]
```

---

## 21. Sliding-window maximum

`🔴 hard` · *Monotonic deque*

The maximum of every length-`k` window, in O(n). A **monotonic deque** of indices holds candidates in decreasing value order: the front is always the current window's max. Each step drops the front if it has slid out of the window, and pops the back of any values the incoming element dominates.

**Steps:**

1. Before adding index `i`, drop the front if `dq[0] <= i-k` (out of window).
2. Pop back indices whose value is `<=` the incoming value — they can never win again.
3. Append `i`; once the first window is full (`i >= k-1`), record `nums[dq[0]]`.

```go
package main

import "fmt"

// maxSlidingWindow returns the maximum of every window of size k. A deque of
// indices kept in decreasing value order gives O(n): the front is always the
// current window's max; we drop indices that fall out of the window or that are
// smaller than the incoming value.
func maxSlidingWindow(nums []int, k int) []int {
	var dq []int // indices, nums[dq] decreasing front->back
	var res []int
	for i, v := range nums {
		// Drop the front index once it has slid out of the window on the left.
		if len(dq) > 0 && dq[0] <= i-k {
			dq = dq[1:]
		}
		// Drop smaller values at the back — they can never be the max now.
		for len(dq) > 0 && nums[dq[len(dq)-1]] <= v {
			dq = dq[:len(dq)-1]
		}
		dq = append(dq, i)
		if i >= k-1 {
			res = append(res, nums[dq[0]]) // front = window max
		}
	}
	return res
}

func main() {
	nums := []int{1, 3, -1, -3, 5, 3, 6, 7}
	fmt.Println("nums:", nums)
	fmt.Println("window max (k=3):", maxSlidingWindow(nums, 3))
}
```

**Output:**

```
nums: [1 3 -1 -3 5 3 6 7]
window max (k=3): [3 3 5 5 6 7]
```

---

## 22. An LRU cache

`🔴 hard` · *stdlib + map*

The interview classic, and a real production structure. A least-recently-used cache with O(1) `Get`/`Put` combines a **map** (key → list element, for O(1) lookup) with a **doubly linked list** (`container/list`) that keeps usage order: front = most-recently-used, back = eviction candidate. Storing the key inside each list entry lets eviction clean up the map.

**Steps:**

1. On `Get`/`Put` of an existing key, `MoveToFront` to mark it most-recently-used.
2. On a new `Put`, `PushFront` and record the element in the map.
3. If over capacity, `Remove` the `Back` element and `delete` its key from the map.

```go
package main

import (
	"container/list"
	"fmt"
)

// entry is stored in the list; keeping the key lets us delete from the map when
// we evict the least-recently-used element from the list's back.
type entry struct {
	key, value int
}

// LRU is a fixed-capacity cache with O(1) Get/Put. A map gives O(1) lookup to a
// list element; the doubly linked list keeps usage order (front = most recent).
type LRU struct {
	cap   int
	ll    *list.List
	items map[int]*list.Element
}

func NewLRU(capacity int) *LRU {
	return &LRU{cap: capacity, ll: list.New(), items: make(map[int]*list.Element)}
}

func (c *LRU) Get(key int) (int, bool) {
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el) // mark most-recently-used
		return el.Value.(*entry).value, true
	}
	return 0, false
}

func (c *LRU) Put(key, value int) {
	if el, ok := c.items[key]; ok {
		el.Value.(*entry).value = value
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&entry{key, value})
	c.items[key] = el
	if c.ll.Len() > c.cap { // evict least-recently-used (the back)
		oldest := c.ll.Back()
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*entry).key)
	}
}

func main() {
	c := NewLRU(2)
	c.Put(1, 100)
	c.Put(2, 200)
	fmt.Println(c.Get(1)) // 100 true — now 1 is most recent
	c.Put(3, 300)         // capacity 2 -> evicts key 2 (least recent)
	fmt.Println(c.Get(2)) // 0 false — evicted
	fmt.Println(c.Get(3)) // 300 true
}
```

**Output:**

```
100 true
0 false
300 true
```

---

## 23. A generic doubly linked list

`🔴 hard` · *Generics*

Building your own doubly linked list — the shape behind `container/list`, but type-safe. Each `dnode` links both ways; the list keeps **head and tail** pointers so push/pop at either end is O(1) with no walk. The fiddly part is the edge cases: fixing up the neighbour's pointer, and resetting `tail`/`head` when the list becomes empty.

**Steps:**

1. `PushBack`/`PushFront` link a new node and update the far pointer (or set both when empty).
2. `PopFront` unlinks the head, clears the new head's `prev`, and nils `tail` if the list emptied.
3. `Forward` walks `head → nil` to snapshot the values.

```go
package main

import "fmt"

// dnode is a node of a doubly linked list: links to both neighbours.
type dnode[T any] struct {
	val        T
	prev, next *dnode[T]
}

// DList keeps head and tail pointers, so push/pop at BOTH ends is O(1) — no walk
// to the end like a singly linked list needs.
type DList[T any] struct {
	head, tail *dnode[T]
	size       int
}

func (l *DList[T]) Len() int { return l.size }

func (l *DList[T]) PushBack(v T) {
	n := &dnode[T]{val: v, prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.size++
}

func (l *DList[T]) PushFront(v T) {
	n := &dnode[T]{val: v, next: l.head}
	if l.head != nil {
		l.head.prev = n
	} else {
		l.tail = n
	}
	l.head = n
	l.size++
}

func (l *DList[T]) PopFront() (T, bool) {
	var zero T
	if l.head == nil {
		return zero, false
	}
	n := l.head
	l.head = n.next
	if l.head != nil {
		l.head.prev = nil
	} else {
		l.tail = nil
	}
	l.size--
	return n.val, true
}

func (l *DList[T]) Forward() []T {
	out := make([]T, 0, l.size)
	for n := l.head; n != nil; n = n.next {
		out = append(out, n.val)
	}
	return out
}

func main() {
	var l DList[int]
	l.PushBack(2)
	l.PushBack(3)
	l.PushFront(1) // 1 2 3
	fmt.Println("forward:", l.Forward(), "len:", l.Len())
	v, _ := l.PopFront()
	fmt.Println("popfront:", v, "->", l.Forward())
}
```

**Output:**

```
forward: [1 2 3] len: 3
popfront: 1 -> [2 3]
```

---

## 24. Detect a cycle (Floyd's)

`🔴 hard` · *Two pointers*

Does a linked list loop back on itself? Floyd's **tortoise and hare**: a slow pointer steps once, a fast pointer twice. If there's a cycle the fast one eventually laps the slow one and they meet; if the list ends (`nil`), there's no cycle. O(n) time, O(1) space — no extra `seen` set needed.

**Steps:**

1. Start both pointers at the head.
2. Advance `slow` by 1 and `fast` by 2; if they ever coincide, there's a cycle.
3. If `fast` (or `fast.next`) reaches `nil`, the list is acyclic.

```go
package main

import "fmt"

type node struct {
	val  int
	next *node
}

// hasCycle uses Floyd's algorithm: a slow pointer moves one step, a fast pointer
// two. If there is a loop the fast one laps the slow one and they meet; if the
// list ends (nil) there is no cycle. O(n) time, O(1) space.
func hasCycle(head *node) bool {
	slow, fast := head, head
	for fast != nil && fast.next != nil {
		slow = slow.next
		fast = fast.next.next
		if slow == fast {
			return true
		}
	}
	return false
}

func main() {
	// Straight list 1->2->3->4 (no cycle).
	n4 := &node{val: 4}
	n3 := &node{val: 3, next: n4}
	n2 := &node{val: 2, next: n3}
	n1 := &node{val: 1, next: n2}
	fmt.Println("straight list has cycle:", hasCycle(n1))

	// Make it loop: 4 -> back to 2.
	n4.next = n2
	fmt.Println("looped list has cycle:", hasCycle(n1))
}
```

**Output:**

```
straight list has cycle: false
looped list has cycle: true
```

---

## 25. Merge two sorted lists

`🔴 hard` · *Pointers*

Splice two already-sorted linked lists into one sorted list — the merge step of merge sort. A **dummy head** node removes the "is this the first element?" special case: always append to `tail.next`, advance `tail`, and at the end attach whatever list still has nodes. O(n+m), no new nodes allocated.

**Steps:**

1. Keep a `dummy` node; `tail` starts at it.
2. While both lists are non-empty, link the smaller head and advance that list.
3. Attach the non-empty remainder; return `dummy.next`.

```go
package main

import "fmt"

type node struct {
	val  int
	next *node
}

// merge splices two sorted lists into one sorted list. A dummy head avoids a
// special case for the first node; tail always points at the last merged node.
func merge(a, b *node) *node {
	dummy := &node{}
	tail := dummy
	for a != nil && b != nil {
		if a.val <= b.val {
			tail.next = a
			a = a.next
		} else {
			tail.next = b
			b = b.next
		}
		tail = tail.next
	}
	if a != nil { // attach whatever remains
		tail.next = a
	} else {
		tail.next = b
	}
	return dummy.next
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
	a := build(1, 3, 5, 7)
	b := build(2, 4, 6)
	show(merge(a, b))
}
```

**Output:**

```
1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7
```

---

## 26. A buffered channel as a queue

`🔴 hard` · *Concurrency*

The most Go-idiomatic linear structure of all: a **buffered channel is a concurrent FIFO queue**. `ch <- v` enqueues, `<-ch` dequeues, and the runtime supplies the locking and backpressure. For goroutine-to-goroutine hand-off you almost never build a mutex-guarded queue — you use a channel. Here one producer feeds a queue that three workers drain concurrently.

**Steps:**

1. `make(chan int, 100)` is the queue; enqueue jobs, then `close` to signal "done".
2. Each worker `range`s the channel — the loop ends when the channel is closed **and** drained.
3. Sum is order-independent, so the result is deterministic (run it with `-race` to prove the sync is correct).

```go
package main

import (
	"fmt"
	"sync"
)

// A buffered channel IS Go's built-in concurrent FIFO queue: ch <- v enqueues,
// <-ch dequeues, and the runtime does the locking and backpressure. You rarely
// hand-roll a mutex-guarded queue for goroutine-to-goroutine work.
func main() {
	jobs := make(chan int, 100) // the queue (capacity 100)
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0

	// Producer: enqueue 1..10, then close to signal "no more work".
	for i := 1; i <= 10; i++ {
		jobs <- i
	}
	close(jobs)

	// Three workers dequeue concurrently; range ends when the channel drains.
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs { // dequeue until closed & empty
				mu.Lock()
				total += j
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Println("processed sum:", total) // 1+..+10 = 55
}
```

**Output:**

```
processed sum: 55
```

---

> Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
