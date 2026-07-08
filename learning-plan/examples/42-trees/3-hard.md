# Step 42 — Trees · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

---

## 18. Delete a node from a BST

`🔴 hard` · *BST*

Deletion is the trickiest BST operation because of three cases when you find the node: **no child** (return `nil`), **one child** (return that child), **two children** (copy the in-order successor's value here, then delete the successor — which necessarily has at most one child). Named `remove` so as not to shadow the built-in `delete`.

**Steps:**

1. Recurse left/right until `v == n.Val`.
2. Zero or one child: return the other side directly.
3. Two children: `succ = minNode(n.Right)`, copy its value up, then `remove` the successor from the right subtree.

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

func inorderSlice(n *Node, out []int) []int {
	if n == nil {
		return out
	}
	out = inorderSlice(n.Left, out)
	out = append(out, n.Val)
	out = inorderSlice(n.Right, out)
	return out
}

func minNode(n *Node) *Node {
	for n.Left != nil {
		n = n.Left
	}
	return n
}

// remove deletes v and returns the new subtree root. Three cases when the node
// is found: no child (return nil), one child (return that child), two children
// (copy the in-order successor's value here, then delete the successor, which
// necessarily has at most one child). Named "remove" so as not to shadow the
// built-in delete.
func remove(n *Node, v int) *Node {
	if n == nil {
		return nil
	}
	switch {
	case v < n.Val:
		n.Left = remove(n.Left, v)
	case v > n.Val:
		n.Right = remove(n.Right, v)
	default:
		if n.Left == nil {
			return n.Right
		}
		if n.Right == nil {
			return n.Left
		}
		succ := minNode(n.Right)
		n.Val = succ.Val
		n.Right = remove(n.Right, succ.Val)
	}
	return n
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	fmt.Println("before:", inorderSlice(root, nil))
	// 3 has two children (1 and 4); its successor is 4.
	root = remove(root, 3)
	fmt.Println("after remove 3:", inorderSlice(root, nil))
}
```

**Output:**

```
before: [1 3 4 5 7 8 9]
after remove 3: [1 4 5 7 8 9]
```

---

## 19. Lowest common ancestor in a BST

`🔴 hard` · *BST*

The lowest common ancestor of `a` and `b` is the deepest node that has both in its subtree. In a **BST** you can find it iteratively: if both are smaller, go left; if both larger, go right; otherwise this node is the split point where their paths diverge.

**Steps:**

1. If both `a` and `b < n.Val`, descend left.
2. If both `> n.Val`, descend right.
3. Otherwise (they split here, or one equals `n`), this node is the LCA.

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

// lca finds the lowest common ancestor of a and b in a BST. The trick: the LCA
// is the first node where a and b fall on different sides (or one equals the
// node). If both are smaller, go left; if both larger, go right; otherwise this
// node is the split point.
func lca(n *Node, a, b int) *Node {
	for n != nil {
		switch {
		case a < n.Val && b < n.Val:
			n = n.Left
		case a > n.Val && b > n.Val:
			n = n.Right
		default:
			return n
		}
	}
	return nil
}

func main() {
	var root *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		root = insert(root, v)
	}
	fmt.Println("lca(1,4):", lca(root, 1, 4).Val)
	fmt.Println("lca(7,9):", lca(root, 7, 9).Val)
	fmt.Println("lca(1,9):", lca(root, 1, 9).Val)
}
```

**Output:**

```
lca(1,4): 3
lca(7,9): 8
lca(1,9): 5
```

---

## 20. Check a tree is height-balanced

`🔴 hard` · *Recursion*

A tree is height-balanced if, at every node, the two subtree heights differ by at most 1. The trick is to compute height and detect imbalance in **one bottom-up pass**: return a real height, or `-1` as a sentinel meaning "already unbalanced" that propagates straight up. Checking height separately at every node would be O(n log n); this is O(n).

**Steps:**

1. `nil` → height `0`.
2. If a subtree returned `-1`, short-circuit and return `-1`.
3. If the two heights differ by more than 1, return `-1`; else return `max+1`.

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

// checkHeight returns the height of the subtree, or -1 as a sentinel meaning
// "already unbalanced". Computing height and detecting imbalance in one bottom-up
// pass is O(n); checking height separately at every node would be O(n log n).
func checkHeight(n *Node) int {
	if n == nil {
		return 0
	}
	l := checkHeight(n.Left)
	if l == -1 {
		return -1
	}
	r := checkHeight(n.Right)
	if r == -1 {
		return -1
	}
	if l-r < -1 || l-r > 1 {
		return -1
	}
	if l > r {
		return l + 1
	}
	return r + 1
}

func isBalanced(n *Node) bool {
	return checkHeight(n) != -1
}

func main() {
	var balanced *Node
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9} {
		balanced = insert(balanced, v)
	}
	// A right-leaning chain 1 -> 2 -> 3 is maximally unbalanced.
	skewed := &Node{Val: 1, Right: &Node{Val: 2, Right: &Node{Val: 3}}}

	fmt.Println("balanced:", isBalanced(balanced))
	fmt.Println("skewed:", isBalanced(skewed))
}
```

**Output:**

```
balanced: true
skewed: false
```

---

## 21. Root-to-leaf path sum

`🔴 hard` · *Backtracking*

Collect every root-to-leaf path whose values add up to a target. This is depth-first with a running prefix — and it exposes a classic **slice-aliasing trap**: `append` can reuse the same backing array across sibling calls, so you must **copy** the path before storing it, or later appends will corrupt what you saved.

**Steps:**

1. Append the node to `path` and subtract from `remaining`.
2. At a leaf with `remaining == 0`, copy `path` into a fresh slice and store it.
3. Recurse into both children with the updated `path` and `remaining`.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// pathSum collects every root-to-leaf path whose values add up to the target.
// `path` is the running prefix; because append may reuse the same backing
// array across sibling calls, we COPY it before storing a completed path —
// forgetting that copy is the classic aliasing bug here.
func pathSum(n *Node, remaining int, path []int, out *[][]int) {
	if n == nil {
		return
	}
	path = append(path, n.Val)
	remaining -= n.Val
	if n.Left == nil && n.Right == nil && remaining == 0 {
		cp := make([]int, len(path))
		copy(cp, path)
		*out = append(*out, cp)
	}
	pathSum(n.Left, remaining, path, out)
	pathSum(n.Right, remaining, path, out)
}

func main() {
	//        5
	//       / \
	//      4   8
	//     /   / \
	//    11  13  4
	//   / \     / \
	//  7   2   5   1
	root := &Node{Val: 5,
		Left: &Node{Val: 4,
			Left: &Node{Val: 11,
				Left:  &Node{Val: 7},
				Right: &Node{Val: 2},
			},
		},
		Right: &Node{Val: 8,
			Left: &Node{Val: 13},
			Right: &Node{Val: 4,
				Left:  &Node{Val: 5},
				Right: &Node{Val: 1},
			},
		},
	}

	var out [][]int
	pathSum(root, 22, nil, &out)
	fmt.Println("paths summing to 22:")
	for _, p := range out {
		fmt.Println(p)
	}
}
```

**Output:**

```
paths summing to 22:
[5 4 11 2]
[5 8 4 5]
```

---

## 22. A generic BST with a compare function

`🔴 hard` · *Generics*

A `BST[T any]` decouples the data structure from the ordering: since `T` can be anything, the tree is told how to order values via a compare function returning `<0, 0, >0` — the same contract as `cmp.Compare`. Pass `cmp.Compare[int]` or `cmp.Compare[string]` and type inference fixes `T`.

**Steps:**

1. Store the `cmp` function on the tree; `bnode[T]` holds the value and children.
2. `Insert`/`InOrder` mirror the concrete versions but call `t.cmp` instead of `<`.
3. `NewBST(cmp.Compare[int])` builds an int tree; the same code builds a string tree.

```go
package main

import (
	"cmp"
	"fmt"
)

// BST is a binary search tree over any type T. Since T can be anything, the
// tree is told how to order values via a compare function that returns <0, 0,
// or >0 — the same contract as cmp.Compare. This decouples the data structure
// from the ordering.
type BST[T any] struct {
	root *bnode[T]
	cmp  func(a, b T) int
}

type bnode[T any] struct {
	val   T
	left  *bnode[T]
	right *bnode[T]
}

func NewBST[T any](compare func(a, b T) int) *BST[T] {
	return &BST[T]{cmp: compare}
}

func (t *BST[T]) Insert(v T) {
	t.root = t.insert(t.root, v)
}

func (t *BST[T]) insert(n *bnode[T], v T) *bnode[T] {
	if n == nil {
		return &bnode[T]{val: v}
	}
	switch {
	case t.cmp(v, n.val) < 0:
		n.left = t.insert(n.left, v)
	case t.cmp(v, n.val) > 0:
		n.right = t.insert(n.right, v)
	}
	return n
}

func (t *BST[T]) InOrder() []T {
	var out []T
	var walk func(*bnode[T])
	walk = func(n *bnode[T]) {
		if n == nil {
			return
		}
		walk(n.left)
		out = append(out, n.val)
		walk(n.right)
	}
	walk(t.root)
	return out
}

func main() {
	// cmp.Compare[int] is a ready-made func(int, int) int; type inference then
	// fixes T = int for the whole tree.
	ints := NewBST(cmp.Compare[int])
	for _, v := range []int{5, 2, 8, 1, 9, 3} {
		ints.Insert(v)
	}
	fmt.Println("ints:", ints.InOrder())

	strs := NewBST(cmp.Compare[string])
	for _, s := range []string{"pear", "apple", "kiwi", "fig"} {
		strs.Insert(s)
	}
	fmt.Println("strings:", strs.InOrder())
}
```

**Output:**

```
ints: [1 2 3 5 8 9]
strings: [apple fig kiwi pear]
```

---

## 23. Serialize and deserialize a tree

`🔴 hard` · *Serialization*

To store or transmit a tree, flatten it to a string and rebuild it later. A **pre-order** walk that writes `#` for each `nil` child produces an unambiguous encoding — recording the nils is what lets you reconstruct the exact shape. Deserialize consumes the tokens in the same pre-order, advancing a shared cursor.

**Steps:**

1. `serialize` writes the value then recurses; `nil` writes `#`.
2. `deserialize` pops the front token: `#` → `nil`, else build a node and recurse for both children.
3. Re-serialize the rebuilt tree to prove the round-trip is lossless.

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// serialize writes the tree as a pre-order sequence, using "#" for a nil child.
// Recording the nils is what makes the string unambiguous — you can rebuild the
// exact shape from it.
func serialize(n *Node, sb *strings.Builder) {
	if n == nil {
		sb.WriteString("# ")
		return
	}
	sb.WriteString(strconv.Itoa(n.Val))
	sb.WriteByte(' ')
	serialize(n.Left, sb)
	serialize(n.Right, sb)
}

// deserialize consumes the token stream in the same pre-order it was written.
// The *[]string lets each call advance the shared cursor as it pops a token.
func deserialize(tokens *[]string) *Node {
	tok := (*tokens)[0]
	*tokens = (*tokens)[1:]
	if tok == "#" {
		return nil
	}
	v, _ := strconv.Atoi(tok)
	n := &Node{Val: v}
	n.Left = deserialize(tokens)
	n.Right = deserialize(tokens)
	return n
}

func main() {
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2},
		Right: &Node{Val: 3, Left: &Node{Val: 4}, Right: &Node{Val: 5}},
	}

	var sb strings.Builder
	serialize(root, &sb)
	s := strings.TrimSpace(sb.String())
	fmt.Println("serialized:", s)

	tokens := strings.Fields(s)
	restored := deserialize(&tokens)

	// Re-serialize the rebuilt tree to prove the round-trip is lossless.
	var sb2 strings.Builder
	serialize(restored, &sb2)
	fmt.Println("round-trip:", strings.TrimSpace(sb2.String()))
}
```

**Output:**

```
serialized: 1 2 # # 3 4 # # 5 # #
round-trip: 1 2 # # 3 4 # # 5 # #
```

---

## 24. A trie (prefix tree)

`🔴 hard` · *Trie*

A trie stores strings by branching on each **character** instead of comparing whole values. Each node holds a `map[rune]*Trie` of children plus an `end` flag marking a complete word. Shared prefixes share nodes, which makes prefix queries (`StartsWith`) cheap — the basis of autocomplete.

**Steps:**

1. `Insert` walks/creates a child per rune, then marks the last node `end`.
2. `find` follows a string and returns the node it lands on (or `nil`).
3. `Search` = landed on a node *and* it's a word-end; `StartsWith` = landed anywhere.

```go
package main

import "fmt"

// Trie (prefix tree) stores strings by branching on each character. Each node
// holds a map from the next rune to a child, plus a flag marking the end of a
// complete word. Shared prefixes share nodes, which is what makes prefix
// queries cheap.
type Trie struct {
	children map[rune]*Trie
	end      bool
}

func NewTrie() *Trie {
	return &Trie{children: map[rune]*Trie{}}
}

func (t *Trie) Insert(word string) {
	cur := t
	for _, r := range word {
		next, ok := cur.children[r]
		if !ok {
			next = NewTrie()
			cur.children[r] = next
		}
		cur = next
	}
	cur.end = true
}

// find walks the trie following word and returns the node it lands on, or nil
// if the path breaks. Both Search and StartsWith are thin wrappers over it.
func (t *Trie) find(word string) *Trie {
	cur := t
	for _, r := range word {
		next, ok := cur.children[r]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func (t *Trie) Search(word string) bool {
	n := t.find(word)
	return n != nil && n.end
}

func (t *Trie) StartsWith(prefix string) bool {
	return t.find(prefix) != nil
}

func main() {
	t := NewTrie()
	for _, w := range []string{"go", "gopher", "rust"} {
		t.Insert(w)
	}
	fmt.Println("search 'go':", t.Search("go"))
	fmt.Println("search 'gop':", t.Search("gop"))
	fmt.Println("startsWith 'gop':", t.StartsWith("gop"))
	fmt.Println("startsWith 'ru':", t.StartsWith("ru"))
	fmt.Println("search 'java':", t.Search("java"))
}
```

**Output:**

```
search 'go': true
search 'gop': false
startsWith 'gop': true
startsWith 'ru': true
search 'java': false
```

---

## 25. Diameter of a binary tree

`🔴 hard` · *Recursion*

The diameter is the longest path (in edges) between any two nodes — it need not pass through the root. The insight: the longest path *through* a node is `leftHeight + rightHeight`, so compute height as usual and, as a side effect via a pointer, track the best `l+r` seen at any node.

**Steps:**

1. Recurse to get left and right heights.
2. Update `*best` with `l + r` (the path through this node).
3. Return the node's own height (`max(l,r)+1`) to its parent.

```go
package main

import "fmt"

type Node struct {
	Val   int
	Left  *Node
	Right *Node
}

// diameter returns the height (in edges) of the subtree and, as a side effect
// through *best, tracks the longest path between any two nodes. The key insight:
// the longest path THROUGH a node is leftHeight + rightHeight, so we update best
// at every node while still returning a normal height to the parent.
func diameter(n *Node, best *int) int {
	if n == nil {
		return 0
	}
	l := diameter(n.Left, best)
	r := diameter(n.Right, best)
	if l+r > *best {
		*best = l + r
	}
	if l > r {
		return l + 1
	}
	return r + 1
}

func main() {
	//        1
	//       / \
	//      2   3
	//     / \
	//    4   5
	// Longest path is 4 -> 2 -> 1 -> 3 (3 edges).
	root := &Node{
		Val:   1,
		Left:  &Node{Val: 2, Left: &Node{Val: 4}, Right: &Node{Val: 5}},
		Right: &Node{Val: 3},
	}
	best := 0
	diameter(root, &best)
	fmt.Println("diameter (edges):", best)
}
```

**Output:**

```
diameter (edges): 3
```

---

## 26. Evaluate an expression tree

`🔴 hard` · *Application*

An expression tree represents arithmetic: leaves are numbers, internal nodes are operators with two operand subtrees. Evaluating is just a **post-order** traversal — compute both children, then combine with the node's operator. This is the shape a parser produces and an interpreter walks.

**Steps:**

1. A leaf (`Op == 0`) evaluates to its `Val`.
2. Otherwise evaluate `Left` and `Right`, then apply `Op` via a `switch`.
3. The tree for `(3 + 4) * 2` evaluates to `14`.

```go
package main

import "fmt"

// ExprNode is a node in an arithmetic expression tree. Leaves carry a number
// (Op == 0). Internal nodes carry an operator byte and two operand subtrees.
// Evaluating is just a post-order traversal: children first, then combine.
type ExprNode struct {
	Op    byte // '+', '-', '*', '/' — or 0 for a leaf value
	Val   float64
	Left  *ExprNode
	Right *ExprNode
}

func eval(n *ExprNode) float64 {
	if n.Op == 0 {
		return n.Val
	}
	l := eval(n.Left)
	r := eval(n.Right)
	switch n.Op {
	case '+':
		return l + r
	case '-':
		return l - r
	case '*':
		return l * r
	case '/':
		return l / r
	}
	panic("unknown operator")
}

func main() {
	// (3 + 4) * 2
	//        (*)
	//       /   \
	//      (+)   2
	//     /   \
	//    3     4
	expr := &ExprNode{
		Op: '*',
		Left: &ExprNode{
			Op:    '+',
			Left:  &ExprNode{Val: 3},
			Right: &ExprNode{Val: 4},
		},
		Right: &ExprNode{Val: 2},
	}
	fmt.Println("(3 + 4) * 2 =", eval(expr))
}
```

**Output:**

```
(3 + 4) * 2 = 14
```

---

> Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
