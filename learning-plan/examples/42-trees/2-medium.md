# Step 42 — Trees · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Insert into a BST

`🟡 medium` · *BST*

A binary search tree keeps values ordered: smaller goes left, larger goes right. `insert` returns the (possibly new) subtree root so the parent can reattach it — the idiomatic `n.Left = insert(n.Left, v)` style handles the "subtree was empty" case with no special-casing.

**Steps:**

1. `nil` → return a fresh leaf `&Node{Val: v}`.
2. Go left if smaller, right if larger, ignore duplicates.
3. In-order over the result comes out sorted, which proves the shape.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// insert adds v in binary-search-tree order: smaller values go left, larger go
// right, duplicates are ignored. It returns the (possibly new) subtree root so
// the caller can reattach it — the classic recursive "functional" insert.
func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

func inorder(n *Node) {
	if n == nil {
		return
	}
	inorder(n.Left)
	fmt.Print(n.Val, " ")
	inorder(n.Right)
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	// In-order over a BST comes out sorted, proving the shape is correct.
	inorder(root)
	fmt.Println()
}
```

**Output:**

```
1 3 4 5 7 8 9 
```

---

## 10. Search a BST

`🟡 medium` · *BST*

Because a BST is ordered, `contains` follows a single path from root to leaf: go left when the target is smaller, right when larger. That's **O(height)**, not a full scan.

**Steps:**

1. `nil` → not found.
2. Equal → found; smaller → search left; larger → search right.
3. Each step discards half the remaining tree.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// contains searches a BST. At each node it goes left or right based on the
// comparison, so it visits at most one path from root to leaf — O(height),
// not O(n).
func contains(n *Node, v int) bool {
	if n == nil {
		return false
	}
	if v == n.Val {
		return true
	}
	if v < n.Val {
		return contains(n.Left, v)
	}
	return contains(n.Right, v)
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	fmt.Println("contains 4:", contains(root, 4))
	fmt.Println("contains 6:", contains(root, 6))
}
```

**Output:**

```
contains 4: true
contains 6: false
```

---

## 11. Min and max in a BST

`🟡 medium` · *BST*

In a BST the smallest value is the **leftmost** node and the largest is the **rightmost** — no recursion needed, just walk in one direction until you can't. (Named `minNode`/`maxNode` so as not to shadow the built-in `min`/`max`.)

**Steps:**

1. For min, follow `Left` until it's `nil`.
2. For max, follow `Right` until it's `nil`.
3. Return the node you land on.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// minNode walks left as far as possible: in a BST the smallest value is the
// leftmost node. maxNode is the mirror image.
func minNode(n *Node) *Node {
	for n != nil && n.Left != nil {
		n = n.Left
	}
	return n
}

func maxNode(n *Node) *Node {
	for n != nil && n.Right != nil {
		n = n.Right
	}
	return n
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	fmt.Println("min:", minNode(root).Val)
	fmt.Println("max:", maxNode(root).Val)
}
```

**Output:**

```
min: 1
max: 9
```

---

## 12. Collect an in-order slice

`🟡 medium` · *Traversal*

Instead of printing during traversal, thread a slice through the recursion and `append` to it. Passing `out` in and returning it back is the idiomatic way to accumulate across recursive calls without a shared global.

**Steps:**

1. `nil` → return `out` unchanged.
2. Recurse left (capturing the returned slice), append the node, recurse right.
3. Call it with `nil` to start from an empty slice.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// inorderSlice threads a slice through the recursion, appending each value in
// sorted order. Passing `out` in and returning it back is the idiomatic way to
// accumulate across recursive calls without a shared global.
func inorderSlice(n *Node, out []int) []int {
	if n == nil {
		return out
	}
	out = inorderSlice(n.Left, out)
	out = append(out, n.Val)
	out = inorderSlice(n.Right, out)
	return out
}

func main() {
	var root *Node
	for _, v := range []int{8, 3, 10, 1, 6, 14} {
		root = insert(root, v)
	}
	sorted := inorderSlice(root, nil)
	fmt.Println("sorted:", sorted)
}
```

**Output:**

```
sorted: [1 3 6 8 10 14]
```

---

## 13. Level-order traversal (BFS)

`🟡 medium` · *BFS*

Breadth-first traversal visits the tree **level by level**, and it uses a queue, not recursion. A slice makes a fine FIFO queue: dequeue the front, print it, enqueue its children.

**Steps:**

1. Start with the root in the queue.
2. Pop `queue[0]`, re-slice off the front, visit it.
3. Append its non-nil children to the back; repeat until empty.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// levelOrder is breadth-first traversal: visit the tree level by level using a
// FIFO queue. A slice works as a queue — append to the back, re-slice off the
// front. Dequeue a node, print it, then enqueue its children.
func levelOrder(root *Node) {
	if root == nil {
		return
	}
	queue := []*Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		fmt.Print(n.Val, " ")
		if n.Left != nil {
			queue = append(queue, n.Left)
		}
		if n.Right != nil {
			queue = append(queue, n.Right)
		}
	}
	fmt.Println()
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	levelOrder(root)
}
```

**Output:**

```
5 3 8 1 4 7 9 
```

---

## 14. Iterative in-order with a stack

`🟡 medium` · *Traversal*

Any recursion can be rewritten as an explicit stack loop. In-order iteratively: push the whole left spine, pop to visit, then switch to the right child and dive left again. Useful when you want to avoid deep call stacks or pause mid-traversal.

**Steps:**

1. Push every node down the left edge onto the stack.
2. Pop one, visit it.
3. Move to its right child and repeat until both the stack and `cur` are empty.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// inorderIterative does in-order traversal without recursion, using an explicit
// stack. Push every node down the left spine, then pop: each popped node is
// visited, and we switch to its right child before diving left again.
func inorderIterative(root *Node) {
	var stack []*Node
	cur := root
	for cur != nil || len(stack) > 0 {
		for cur != nil {
			stack = append(stack, cur)
			cur = cur.Left
		}
		cur = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fmt.Print(cur.Val, " ")
		cur = cur.Right
	}
	fmt.Println()
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	inorderIterative(root)
}
```

**Output:**

```
1 3 4 5 7 8 9 
```

---

## 15. Count the leaves

`🟡 medium` · *Recursion*

A leaf is a node with no children. This adds a **second base case** to the count recursion: `nil` contributes 0, a node whose children are both `nil` contributes 1, everything else recurses.

**Steps:**

1. `nil` → `0`.
2. Both children `nil` → `1` (it's a leaf).
3. Otherwise sum the leaves of both subtrees.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

// countLeaves counts nodes with no children. A leaf is the second base case:
// nil contributes 0, a node with both children nil contributes 1.
func countLeaves(n *Node) int {
	if n == nil {
		return 0
	}
	if n.Left == nil && n.Right == nil {
		return 1
	}
	return countLeaves(n.Left) + countLeaves(n.Right)
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	// Leaves are 1, 4, 7, 9.
	fmt.Println("leaves:", countLeaves(root))
}
```

**Output:**

```
leaves: 4
```

---

## 16. Invert a binary tree

`🟡 medium` · *Transform*

Inverting (mirroring) a tree swaps the two subtrees at every node. Go's tuple assignment does the swap *and* the recursion in one line, no temporary needed. Level-order before and after makes the mirror visible.

**Steps:**

1. `nil` → `nil`.
2. `n.Left, n.Right = invert(n.Right), invert(n.Left)` swaps and recurses at once.
3. Print level-order before and after to see the mirror.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func insert(n *Node, v int) *Node {
	if n == nil {
		return &Node{Val: v}
	}
	if v < n.Val {
		n.Left = insert(n.Left, v)
	} else if v > n.Val {
		n.Right = insert(n.Right, v)
	}
	return n
}

func levelOrder(root *Node) []int {
	var out []int
	if root == nil {
		return out
	}
	queue := []*Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, n.Val)
		if n.Left != nil {
			queue = append(queue, n.Left)
		}
		if n.Right != nil {
			queue = append(queue, n.Right)
		}
	}
	return out
}

// invert mirrors the tree: at every node it swaps the two subtrees. The
// tuple assignment swaps and recurses in one line — no temporary needed.
func invert(n *Node) *Node {
	if n == nil {
		return nil
	}
	n.Left, n.Right = invert(n.Right), invert(n.Left)
	return n
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	fmt.Println("before:", levelOrder(root))
	invert(root)
	fmt.Println("after: ", levelOrder(root))
}
```

**Output:**

```
before: [5 3 8 1 4 7 9]
after:  [5 8 3 9 7 4 1]
```

---

## 17. Validate a BST

`🟡 medium` · *BST*

Checking the search-tree invariant needs a **range**, not a local compare. It is not enough to check each node against its own children: every value in a left subtree must be below *every* ancestor it turned left from. Carry an allowed `(lo, hi)` interval down and tighten it as you descend.

**Steps:**

1. `nil` → valid.
2. Fail if `n.Val` is outside the open interval `(lo, hi)`.
3. Recurse left with `hi = n.Val`, right with `lo = n.Val`; start with `math.MinInt`/`math.MaxInt`.

```go
package main

import (
	"fmt"
	"math"
)

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// isBST checks the search-tree invariant by carrying the allowed (lo, hi) range
// down the tree. It is NOT enough to compare a node only with its own children:
// every value in the left subtree must be below every ancestor it turns left
// from. The range tightens as we descend.
func isBST(n *Node, lo, hi int) bool {
	if n == nil {
		return true
	}
	if n.Val <= lo || n.Val >= hi {
		return false
	}
	return isBST(n.Left, lo, n.Val) && isBST(n.Right, n.Val, hi)
}

func main() {
	valid := &Node{Val: 5,
		Left:  &Node{Val: 3},
		Right: &Node{Val: 8},
	}
	// 4 is on the right of 5 but is smaller than 5 — breaks the invariant.
	invalid := &Node{Val: 5,
		Left:  &Node{Val: 3},
		Right: &Node{Val: 4},
	}
	fmt.Println("valid:", isBST(valid, math.MinInt, math.MaxInt))
	fmt.Println("invalid:", isBST(invalid, math.MinInt, math.MaxInt))
}
```

**Output:**

```
valid: true
invalid: false
```

---

> Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md) · Back to the [index](README.md)
