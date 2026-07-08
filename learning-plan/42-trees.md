# 42 — Trees

## Goals
- Model a binary tree in Go with a recursive pointer struct and the `nil`-as-empty convention.
- Write the core recursions: count, sum, height, and the three depth-first traversals (in/pre/post-order).
- Build and query a **binary search tree** (BST): insert, search, min/max, delete, validate.
- Traverse breadth-first (level-order) with a queue, and turn a recursion into an explicit stack loop.
- Reach for trees in real problems: a generic BST, serialize/deserialize, a trie, an expression tree.

## Concepts
- **A tree node is a struct that points to itself.** The whole data structure is one recursive type; a `nil` child pointer means "no subtree", which doubles as the base case for every recursion.
  ```go
  type Node struct {
      Val   int
      Left  *Node
      Right *Node
  }
  ```
  There is no separate "tree" type — a `*Node` (possibly `nil`) *is* the tree. An empty tree is the nil pointer.
- **Almost every tree algorithm is a recursion with `nil` as the base case.** Handle `nil`, then combine the node's own value with the results from the two subtrees:
  ```go
  func count(n *Node) int {
      if n == nil {
          return 0            // base case: empty tree
      }
      return 1 + count(n.Left) + count(n.Right)
  }
  ```
  `sum`, `height`, and `countLeaves` are all this same shape with a different combine step.
- **Depth-first traversals differ only in *when* you visit the node** relative to its children:
  - **in-order** — Left, **node**, Right → for a BST this yields **sorted** order.
  - **pre-order** — **node**, Left, Right → good for copying/serializing.
  - **post-order** — Left, Right, **node** → children before parent; good for freeing or evaluating.
- **A binary search tree keeps values ordered:** everything in the left subtree is smaller, everything in the right is larger. That invariant makes `insert`/`search` **O(height)** — you follow one path instead of scanning everything.
  ```go
  func insert(n *Node, v int) *Node {
      if n == nil {
          return &Node{Val: v}     // grow a new leaf
      }
      if v < n.Val {
          n.Left = insert(n.Left, v)
      } else if v > n.Val {
          n.Right = insert(n.Right, v)
      }
      return n                     // return the (maybe new) subtree root
  }
  ```
  The **return-the-subtree** style (`n.Left = insert(n.Left, v)`) is the idiomatic Go pattern — it handles "the subtree was empty" without special-casing the parent.
- **Validating a BST needs a range, not a local check.** Comparing each node only with its children is wrong; carry an allowed `(lo, hi)` interval down the tree and tighten it as you descend.
- **Breadth-first (level-order) traversal uses a queue, not recursion.** A slice is a fine FIFO queue: dequeue the front, enqueue the children. This visits the tree level by level.
- **Any recursion can become an explicit stack loop.** In-order iterative traversal pushes the left spine onto a stack, pops to visit, then turns right — useful when you want to pause/resume or avoid deep call stacks.
- **`height` vs `depth`.** Height counts nodes (or edges) on the longest downward path; a balanced tree has height ≈ log₂(n), a degenerate "linked-list" tree has height n. Balance is what keeps BST operations fast.

## Exercises
1. Define the `Node` struct and build the tree `1(2(4), 3)` by hand; print each node's value to confirm the shape.
2. Write `count`, `sum`, and `height` as three recursions sharing the `nil`-base-case pattern.
3. Write all three depth-first traversals (`inorder`, `preorder`, `postorder`) and run them on the same tree; note how the output order changes.
4. Write `insert` and `contains` for a BST; insert `{5,3,8,1,4,7,9}` and confirm `inorder` prints them sorted.
5. Write `minNode`/`maxNode` by walking left/right, and `countLeaves` (a node with two `nil` children).
6. Implement level-order traversal with a slice-as-queue; then write the iterative in-order traversal with an explicit stack.
7. Write `isBST(n, lo, hi)` using a range; test it on a valid tree and on one where a right child is smaller than an ancestor.
8. Implement BST `remove` handling the three cases (0, 1, 2 children) using the in-order successor; delete a two-child node and re-check the in-order output.
9. Stretch: pick one of a generic `BST[T]` (compare-func), serialize/deserialize (pre-order with `#` for nil), or a `Trie`, and build it.

## Best Practices & Pitfalls
- **Let `nil` be your base case and your empty tree.** Don't invent an `IsEmpty` flag — checking `n == nil` first is the whole pattern, and it keeps functions total.
- **Return the subtree from recursive mutators** (`n.Left = insert(n.Left, v)`). It's cleaner than checking "is the child nil?" in the parent and is how you'll write `insert`/`remove`.
- **Validate a BST with a descending range, never a local parent/child compare** — the local check passes on trees that aren't actually BSTs.
- **Pitfall — path collection and slice aliasing.** When you accumulate a root-to-leaf `path` with `append`, the backing array is shared across sibling calls; **copy** the slice before storing a completed path or later appends will corrupt it.
- **Pitfall — shadowing builtins.** `min`, `max`, and `delete` are predeclared; name your tree helpers `minNode`/`remove` rather than shadowing them.
- **Pitfall — unbalanced trees.** A plain BST built from already-sorted input degrades to a linked list (O(n) operations). Real systems use self-balancing trees (red-black, AVL, B-trees); Go's standard library uses B-trees/red-black internally (e.g., in the runtime and `container` isn't a BST — reach for a library when you need guaranteed balance).
- **Reach for the standard library first.** For an ordered set/map, a sorted slice + `slices.BinarySearch`, or a `map` when you don't need order, is usually simpler than hand-rolling a BST. Write the tree to *understand* it; use it in production only when you genuinely need ordered, mutable, logarithmic operations.

## Checklist
- [ ] I can define a recursive `Node` and build a tree by hand.
- [ ] I can write count/sum/height and explain why `nil` is the base case.
- [ ] I can write in-, pre-, and post-order traversals and predict their output.
- [ ] I can insert into, search, and delete from a BST, and validate the invariant with a range.
- [ ] I can do a level-order (BFS) traversal with a queue and an iterative in-order with a stack.
- [ ] I understand why balance matters and when to use the stdlib instead of a hand-rolled tree.

## Resources
- Go by Example — Recursion: https://gobyexample.com/recursion
- `container/heap` (a tree-shaped stdlib structure): https://pkg.go.dev/container/heap
- `slices.BinarySearch` (ordered lookups without a tree): https://pkg.go.dev/slices#BinarySearch
- Wikipedia — Binary search tree: https://en.wikipedia.org/wiki/Binary_search_tree
- Wikipedia — Tree traversal: https://en.wikipedia.org/wiki/Tree_traversal
