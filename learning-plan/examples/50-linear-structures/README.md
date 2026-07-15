# Step 50 — Linear Structures · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout (example 26 is also `-race`-clean).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. A slice is a stack](1-easy.md#1-a-slice-is-a-stack)
- [2. Pop off the stack](1-easy.md#2-pop-off-the-stack)
- [3. A generic Stack[T]](1-easy.md#3-a-generic-stackt)
- [4. Reverse a slice with a stack](1-easy.md#4-reverse-a-slice-with-a-stack)
- [5. A slice is a FIFO queue](1-easy.md#5-a-slice-is-a-fifo-queue)
- [6. A generic Queue[T]](1-easy.md#6-a-generic-queuet)
- [7. Peek, Len & IsEmpty](1-easy.md#7-peek-len--isempty)
- [8. A singly linked list node](1-easy.md#8-a-singly-linked-list-node)

### 🟡 [Medium](2-medium.md)

- [9. Balanced brackets with a stack](2-medium.md#9-balanced-brackets-with-a-stack)
- [10. Evaluate a postfix (RPN) expression](2-medium.md#10-evaluate-a-postfix-rpn-expression)
- [11. A queue that doesn't leak (head index)](2-medium.md#11-a-queue-that-doesnt-leak-head-index)
- [12. A ring-buffer queue](2-medium.md#12-a-ring-buffer-queue)
- [13. A double-ended queue (deque)](2-medium.md#13-a-double-ended-queue-deque)
- [14. A singly linked list](2-medium.md#14-a-singly-linked-list)
- [15. Reverse a linked list](2-medium.md#15-reverse-a-linked-list)
- [16. container/list as a queue/deque](2-medium.md#16-containerlist-as-a-queuedeque)
- [17. container/ring for a circular buffer](2-medium.md#17-containerring-for-a-circular-buffer)

### 🔴 [Hard](3-hard.md)

- [18. A min-stack (O(1) minimum)](3-hard.md#18-a-min-stack-o1-minimum)
- [19. A queue from two stacks](3-hard.md#19-a-queue-from-two-stacks)
- [20. Monotonic stack: next greater element](3-hard.md#20-monotonic-stack-next-greater-element)
- [21. Sliding-window maximum](3-hard.md#21-sliding-window-maximum)
- [22. An LRU cache](3-hard.md#22-an-lru-cache)
- [23. A generic doubly linked list](3-hard.md#23-a-generic-doubly-linked-list)
- [24. Detect a cycle (Floyd's)](3-hard.md#24-detect-a-cycle-floyds)
- [25. Merge two sorted lists](3-hard.md#25-merge-two-sorted-lists)
- [26. A buffered channel as a queue](3-hard.md#26-a-buffered-channel-as-a-queue)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
