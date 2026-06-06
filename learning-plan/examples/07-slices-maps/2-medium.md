# Step 07 — Arrays, Slices & Maps · 🟡 Medium

Examples **8–16**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 8. Slicing a slice

`🟡 medium` · *Slices*

s[low:high] creates a new slice header viewing elements low..high-1; omit either bound for the start or end.

**Steps:**

1. s[1:4], s[:3], s[3:] select sub-ranges.
2. The result shares the same backing array (see the aliasing example).

```go
package main

import "fmt"

func main() {
	s := []int{0, 1, 2, 3, 4, 5}
	fmt.Println(s[1:4]) // [1 2 3]
	fmt.Println(s[:3])  // [0 1 2]
	fmt.Println(s[3:])  // [3 4 5]
}
```

**Output:**

```
[1 2 3]
[0 1 2]
[3 4 5]
```

---

## 9. append growth (capacity doubling)

`🟡 medium` · *Slices*

When append exceeds capacity, Go allocates a bigger backing array (roughly doubling for small slices) and copies the elements.

**Steps:**

1. Append in a loop and print only when cap changes.
2. Watch capacity jump 1, 2, 4, 8, 16...

```go
package main

import "fmt"

func main() {
	s := []int{}
	prev := cap(s)
	for i := 0; i < 10; i++ {
		s = append(s, i)
		if cap(s) != prev {
			fmt.Printf("len=%d cap grew to %d\n", len(s), cap(s))
			prev = cap(s)
		}
	}
}
```

**Output:**

```
len=1 cap grew to 4
len=5 cap grew to 8
len=9 cap grew to 16
```

---

## 10. Slice aliasing trap

`🟡 medium` · *Slices*

A re-slice shares the SAME backing array, so writing through one view changes the other — a common source of bugs.

**Steps:**

1. t := s[1:3] points into s.
2. t[0] = 99 also changes s[1].

```go
package main

import "fmt"

func main() {
	s := []int{1, 2, 3, 4}
	t := s[1:3] // shares backing array with s
	t[0] = 99   // also mutates s[1]
	fmt.Println("s:", s)
	fmt.Println("t:", t)
}
```

**Output:**

```
s: [1 99 3 4]
t: [99 3]
```

---

## 11. copy detaches a slice

`🟡 medium` · *Slices*

copy(dst, src) duplicates elements into a separate backing array, so later edits don't bleed across.

**Steps:**

1. Allocate dst with make, then copy.
2. Editing dst leaves src unchanged.

```go
package main

import "fmt"

func main() {
	src := []int{1, 2, 3}
	dst := make([]int, len(src))
	n := copy(dst, src) // independent copy
	dst[0] = 99
	fmt.Println("copied:", n)
	fmt.Println("src:", src) // unchanged
	fmt.Println("dst:", dst)
}
```

**Output:**

```
copied: 3
src: [1 2 3]
dst: [99 2 3]
```

---

## 12. nil vs empty slice

`🟡 medium` · *Slices*

A nil slice and an empty non-nil slice both have length 0; they differ in == nil, but append works on either.

**Steps:**

1. var a []int is nil; b := []int{} is empty but not nil.
2. Both have len 0, and append happily extends a nil slice.

```go
package main

import "fmt"

func main() {
	var a []int                   // nil
	b := []int{}                  // empty, non-nil
	fmt.Println(a == nil, len(a)) // true 0
	fmt.Println(b == nil, len(b)) // false 0
	a = append(a, 1)
	fmt.Println(a)
}
```

**Output:**

```
true 0
false 0
[1]
```

---

## 13. delete from a map

`🟡 medium` · *Maps*

delete(m, key) removes an entry; deleting a missing key is a safe no-op.

**Steps:**

1. Remove key "b" with delete.
2. comma-ok confirms it's gone and len drops.

```go
package main

import "fmt"

func main() {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	delete(m, "b")
	_, ok := m["b"]
	fmt.Println("b present?", ok)
	fmt.Println("len:", len(m))
}
```

**Output:**

```
b present? false
len: 2
```

---

## 14. Map iteration is unordered

`🟡 medium` · *Maps*

Go randomizes map iteration order on purpose; to print deterministically, collect the keys and sort them.

**Steps:**

1. Gather keys into a slice.
2. sort.Strings then iterate the sorted keys.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	m := map[string]int{"banana": 3, "apple": 5, "cherry": 2}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%d\n", k, m[k])
	}
}
```

**Output:**

```
apple=5
banana=3
cherry=2
```

---

## 15. Frequency counting with a map

`🟡 medium` · *Maps*

Incrementing m[key]++ works even on absent keys (they start at 0), making maps perfect for counting.

**Steps:**

1. Split text into words with strings.Fields.
2. freq[w]++ counts each; sort keys for stable output.

```go
package main

import (
	"fmt"
	"sort"
	"strings"
)

func main() {
	text := "the cat the dog the bird"
	freq := map[string]int{}
	for _, w := range strings.Fields(text) {
		freq[w]++
	}
	words := make([]string, 0, len(freq))
	for w := range freq {
		words = append(words, w)
	}
	sort.Strings(words)
	for _, w := range words {
		fmt.Printf("%s: %d\n", w, freq[w])
	}
}
```

**Output:**

```
bird: 1
cat: 1
dog: 1
the: 3
```

---

## 16. Arrays are comparable and usable as map keys

`🟡 medium` · *Arrays*

Arrays support == (element-by-element) and can be map keys; slices can do neither.

**Steps:**

1. Compare two arrays with ==.
2. Use an array as a map key — a slice would not compile here.

```go
package main

import "fmt"

func main() {
	a := [2]int{1, 2}
	b := [2]int{1, 2}
	c := [2]int{1, 3}
	fmt.Println("a == b:", a == b) // true
	fmt.Println("a == c:", a == c) // false

	seen := map[[2]int]bool{}
	seen[a] = true
	fmt.Println("seen b?", seen[b]) // true: same value as a
}
```

**Output:**

```
a == b: true
a == c: false
seen b? true
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
