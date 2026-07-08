# Step 42 — Trees · 🟢 Easy

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

## 1. A binary tree node

`🟢 easy` · *Struct*

A binary tree node is just a struct that holds a value and two pointers to more nodes. A `nil` child pointer means "no child", so a node with two `nil` children is a **leaf**. There is no separate tree type — a `*Node` *is* the tree.

**Steps:**

1. Declare `Node` with `Val`, `Left`, and `Right *Node`.
2. Build a single leaf with `&Node{Val: 42}` — its children default to the zero value, `nil`.
3. Confirm both children are `nil`.

```go
package main

import "fmt"

// Node is one node of a binary tree: a value plus two child pointers.
// A nil child pointer means "no child" — that is how leaves are represented.
type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func main() {
	// A single node with no children is a leaf.
	leaf := &Node{Val: 42}
	fmt.Println("value:", leaf.Val)
	fmt.Println("left is nil:", leaf.Left == nil)
	fmt.Println("right is nil:", leaf.Right == nil)
}
```

**Output:**

```
value: 42
left is nil: true
right is nil: true
```

---

## 2. Build a tree with pointers

`🟢 easy` · *Pointers*

You assemble a tree by nesting composite literals. Each `&Node{...}` is a pointer, so children can themselves have children. Here we hand-build a small tree and walk into it by chaining field accesses.

**Steps:**

1. Nest `&Node{...}` literals to describe the shape.
2. `root.Left.Left` reaches two levels down.
3. Reading a `.Val` through several pointers is how you navigate a tree.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

func main() {
	// Build this tree by hand:
	//        1
	//       / \
	//      2   3
	//     /
	//    4
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2, Left: &Node{Val: 4}},
		Right: &Node{Val: 3},
	}

	fmt.Println("root:", root.Val)
	fmt.Println("root.Left:", root.Left.Val)
	fmt.Println("root.Right:", root.Right.Val)
	fmt.Println("root.Left.Left:", root.Left.Left.Val)
}
```

**Output:**

```
root: 1
root.Left: 2
root.Right: 3
root.Left.Left: 4
```

---

## 3. Count nodes recursively

`🟢 easy` · *Recursion*

Almost every tree algorithm is a recursion whose base case is the `nil` pointer. To count nodes: an empty tree has 0, otherwise it's `1` (this node) plus the counts of both subtrees.

**Steps:**

1. Return `0` when `n == nil` — the base case.
2. Otherwise return `1 + count(Left) + count(Right)`.
3. Each recursive call shrinks the tree toward `nil`, so it always terminates.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// count returns the number of nodes in the tree rooted at n. The base case is
// the nil pointer: an empty (sub)tree has 0 nodes. Every recursive call shrinks
// the problem toward that base case, so the recursion always terminates.
func count(n *Node) int {
	if n == nil {
		return 0
	}
	return 1 + count(n.Left) + count(n.Right)
}

func main() {
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2, Left: &Node{Val: 4}},
		Right: &Node{Val: 3},
	}
	fmt.Println("node count:", count(root))
	fmt.Println("empty count:", count(nil))
}
```

**Output:**

```
node count: 4
empty count: 0
```

---

## 4. Sum every value

`🟢 easy` · *Recursion*

Summing values is the same recursion as counting, with a different combine step. `nil` returns the identity for addition (`0`); every node adds its own value to the two subtree sums.

**Steps:**

1. Base case `nil` → `0`.
2. Combine `n.Val + sum(Left) + sum(Right)`.
3. Notice how swapping the combine turns "count" into "sum".

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// sum adds up every value in the tree. Same shape as count: nil returns the
// identity for addition (0), and each node combines its own value with the
// results from both subtrees.
func sum(n *Node) int {
	if n == nil {
		return 0
	}
	return n.Val + sum(n.Left) + sum(n.Right)
}

func main() {
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2, Left: &Node{Val: 4}},
		Right: &Node{Val: 3},
	}
	fmt.Println("sum:", sum(root))
}
```

**Output:**

```
sum: 10
```

---

## 5. Height of a tree

`🟢 easy` · *Recursion*

Height is the number of nodes on the longest root-to-leaf path. An empty tree has height 0; any node's height is `1` plus the taller of its two subtrees.

**Steps:**

1. Base case `nil` → `0`.
2. Recurse on both sides, keep the larger.
3. Add `1` for the current node.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// height returns the number of nodes on the longest root-to-leaf path.
// An empty tree has height 0; a single leaf has height 1.
func height(n *Node) int {
	if n == nil {
		return 0
	}
	left := height(n.Left)
	right := height(n.Right)
	if left > right {
		return left + 1
	}
	return right + 1
}

func main() {
	// 1 -> 2 -> 4 is the longest path (3 nodes deep).
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2, Left: &Node{Val: 4}},
		Right: &Node{Val: 3},
	}
	fmt.Println("height:", height(root))
}
```

**Output:**

```
height: 3
```

---

## 6. In-order traversal

`🟢 easy` · *Traversal*

A depth-first traversal visits Left, the node, then Right. When the tree is a binary search tree, in-order prints the values in **sorted** order — the defining party trick of in-order.

**Steps:**

1. Recurse left, then print the node, then recurse right.
2. `nil` just returns (prints nothing).
3. The balanced BST below comes out `1 2 3 4 5 6 7`.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// inorder visits Left, then the node itself, then Right. For a binary search
// tree this prints the values in sorted order.
func inorder(n *Node) {
	if n == nil {
		return
	}
	inorder(n.Left)
	fmt.Print(n.Val, " ")
	inorder(n.Right)
}

func main() {
	//        4
	//       / \
	//      2   6
	//     / \ / \
	//    1  3 5  7
	root := &Node{
		Val:   4,
		Left:  &Node{Val: 2, Left: &Node{Val: 1}, Right: &Node{Val: 3}},
		Right: &Node{Val: 6, Left: &Node{Val: 5}, Right: &Node{Val: 7}},
	}
	inorder(root)
	fmt.Println()
}
```

**Output:**

```
1 2 3 4 5 6 7 
```

---

## 7. Pre-order traversal

`🟢 easy` · *Traversal*

Pre-order visits the node **first**, then Left, then Right. It's the order you'd use to copy a tree or print it as a nested structure — the root always comes out before its subtrees.

**Steps:**

1. Print the node *before* recursing.
2. Then recurse left, then right.
3. Same tree as example 6, different visit order.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// preorder visits the node first, then Left, then Right. This is the order you
// would use to copy a tree or print it as a nested structure.
func preorder(n *Node) {
	if n == nil {
		return
	}
	fmt.Print(n.Val, " ")
	preorder(n.Left)
	preorder(n.Right)
}

func main() {
	root := &Node{
		Val:   4,
		Left:  &Node{Val: 2, Left: &Node{Val: 1}, Right: &Node{Val: 3}},
		Right: &Node{Val: 6, Left: &Node{Val: 5}, Right: &Node{Val: 7}},
	}
	preorder(root)
	fmt.Println()
}
```

**Output:**

```
4 2 1 3 6 5 7 
```

---

## 8. Post-order traversal

`🟢 easy` · *Traversal*

Post-order visits Left, Right, then the node **last** — children are always processed before their parent. It's the order for freeing a tree or evaluating an expression tree bottom-up.

**Steps:**

1. Recurse left, recurse right, *then* print.
2. The root prints last of all.
3. Same tree again: leaves surface before their parents.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// postorder visits Left, then Right, then the node last. Children are always
// processed before their parent — the order you would use to free a tree or
// evaluate an expression tree bottom-up.
func postorder(n *Node) {
	if n == nil {
		return
	}
	postorder(n.Left)
	postorder(n.Right)
	fmt.Print(n.Val, " ")
}

func main() {
	root := &Node{
		Val:   4,
		Left:  &Node{Val: 2, Left: &Node{Val: 1}, Right: &Node{Val: 3}},
		Right: &Node{Val: 6, Left: &Node{Val: 5}, Right: &Node{Val: 7}},
	}
	postorder(root)
	fmt.Println()
}
```

**Output:**

```
1 3 2 5 7 6 4 
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
