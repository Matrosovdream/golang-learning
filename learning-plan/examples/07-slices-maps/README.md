# Step 07 — Arrays, Slices & Maps · Examples

A library of **22 runnable examples**. Each is a complete `package main` program:
read the concept and steps, then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** is real stdout.

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them.

## Index


**Easy**

- [1. Arrays have a fixed length](#1-arrays-have-a-fixed-length)
- [2. Arrays are copied by value](#2-arrays-are-copied-by-value)
- [3. Slice literal and indexing](#3-slice-literal-and-indexing)
- [4. len vs cap](#4-len-vs-cap)
- [5. append to a slice](#5-append-to-a-slice)
- [6. Map literal and lookup](#6-map-literal-and-lookup)
- [7. Map comma-ok lookup](#7-map-comma-ok-lookup)

**Medium**

- [8. Slicing a slice](#8-slicing-a-slice)
- [9. append growth (capacity doubling)](#9-append-growth-capacity-doubling)
- [10. Slice aliasing trap](#10-slice-aliasing-trap)
- [11. copy detaches a slice](#11-copy-detaches-a-slice)
- [12. nil vs empty slice](#12-nil-vs-empty-slice)
- [13. delete from a map](#13-delete-from-a-map)
- [14. Map iteration is unordered](#14-map-iteration-is-unordered)
- [15. Frequency counting with a map](#15-frequency-counting-with-a-map)
- [16. Arrays are comparable and usable as map keys](#16-arrays-are-comparable-and-usable-as-map-keys)

**Hard**

- [17. nil map: read ok, write panics](#17-nil-map-read-ok-write-panics)
- [18. Three-index slice controls capacity](#18-three-index-slice-controls-capacity)
- [19. Remove an element by index](#19-remove-an-element-by-index)
- [20. 2D / jagged slices](#20-2d--jagged-slices)
- [21. A set via map[T]struct{}](#21-a-set-via-maptstruct)
- [22. Sorting slices](#22-sorting-slices)

---

## 1. Arrays have a fixed length

`🟢 easy` · *Arrays*

An array's length is part of its type ([3]int), it is zero-valued on declaration, and len gives its size.

**Steps:**

1. var a [3]int starts as three zeros.
2. Assign by index; len(a) is the fixed size.

```go
package main

import "fmt"

func main() {
	var a [3]int // fixed length 3, zero-valued
	a[0] = 10
	a[2] = 30
	fmt.Println(a, "len:", len(a))

	b := [3]string{"x", "y", "z"}
	fmt.Println(b)
}
```

**Output:**

```
[10 0 30] len: 3
[x y z]
```

---

## 2. Arrays are copied by value

`🟢 easy` · *Arrays*

Assigning or passing an array copies the whole thing — the copy is independent of the original.

**Steps:**

1. b := a duplicates the array.
2. Changing b leaves a untouched.

```go
package main

import "fmt"

func main() {
	a := [3]int{1, 2, 3}
	b := a // full copy
	b[0] = 99
	fmt.Println("a:", a) // unchanged
	fmt.Println("b:", b)
}
```

**Output:**

```
a: [1 2 3]
b: [99 2 3]
```

---

## 3. Slice literal and indexing

`🟢 easy` · *Slices*

A slice is a flexible view over elements; create one with a literal and read/write by index.

**Steps:**

1. []int{...} (no length) is a slice, not an array.
2. Index to read and to assign.

```go
package main

import "fmt"

func main() {
	s := []int{10, 20, 30}
	fmt.Println(s[0], s[2])
	s[1] = 99
	fmt.Println(s)
}
```

**Output:**

```
10 30
[10 99 30]
```

---

## 4. len vs cap

`🟢 easy` · *Slices*

A slice has a length (elements in use) and a capacity (room in its backing array); make([]T, len, cap) sets both.

**Steps:**

1. make([]int, 2, 5) gives len 2, cap 5.
2. Appending fills the spare capacity before the slice has to grow.

```go
package main

import "fmt"

func main() {
	s := make([]int, 2, 5) // len 2, cap 5
	fmt.Println("len:", len(s), "cap:", cap(s))
	s = append(s, 1, 2, 3)
	fmt.Println("after append -> len:", len(s), "cap:", cap(s))
}
```

**Output:**

```
len: 2 cap: 5
after append -> len: 5 cap: 5
```

---

## 5. append to a slice

`🟢 easy` · *Slices*

append adds elements and returns the (possibly new) slice; it even works on a nil slice.

**Steps:**

1. Start from a nil slice.
2. append one or several values at a time; reassign the result.

```go
package main

import "fmt"

func main() {
	var s []int // nil slice
	s = append(s, 1)
	s = append(s, 2, 3)
	fmt.Println(s, "len:", len(s))
}
```

**Output:**

```
[1 2 3] len: 3
```

---

## 6. Map literal and lookup

`🟢 easy` · *Maps*

A map associates keys with values; a missing key returns the value type's zero value.

**Steps:**

1. Create with a literal, then read/write by key.
2. Reading an absent key gives 0 (for int), not an error.

```go
package main

import "fmt"

func main() {
	ages := map[string]int{"alice": 30, "bob": 25}
	fmt.Println(ages["alice"])
	ages["carol"] = 35
	fmt.Println(ages["carol"])
	fmt.Println("missing:", ages["dave"]) // zero value 0
}
```

**Output:**

```
30
35
missing: 0
```

---

## 7. Map comma-ok lookup

`🟢 easy` · *Maps*

The two-value form v, ok := m[k] tells you whether the key was actually present, distinguishing 'missing' from 'zero value'.

**Steps:**

1. ok is true only if the key exists.
2. Use it to tell a stored 0 apart from an absent key.

```go
package main

import "fmt"

func main() {
	m := map[string]int{"x": 1}
	v, ok := m["x"]
	fmt.Println("x:", v, ok)
	v, ok = m["y"]
	fmt.Println("y:", v, ok) // 0 false
}
```

**Output:**

```
x: 1 true
y: 0 false
```

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

## 17. nil map: read ok, write panics

`🔴 hard` · *Maps*

Reading from a nil map returns the zero value, but writing to one panics — you must make/initialize a map before adding keys.

**Steps:**

1. Reading m["x"] from a nil map gives 0.
2. Writing m["x"]=1 panics; a deferred recover catches it.

```go
package main

import "fmt"

func main() {
	var m map[string]int                  // nil map
	fmt.Println("read from nil:", m["x"]) // ok -> 0

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered:", r)
		}
	}()
	m["x"] = 1 // panics: assignment to entry in nil map
	fmt.Println("never reached")
}
```

**Output:**

```
read from nil: 0
recovered: assignment to entry in nil map
```

---

## 18. Three-index slice controls capacity

`🔴 hard` · *Slices*

s[low:high:max] sets length to high-low and capacity to max-low, so a later append can't reach into the original's tail.

**Steps:**

1. t := s[1:3:4] gives len 2, cap 3.
2. Capping the capacity protects the rest of s from append.

```go
package main

import "fmt"

func main() {
	s := []int{0, 1, 2, 3, 4, 5}
	t := s[1:3:4] // len = 3-1 = 2, cap = 4-1 = 3
	fmt.Printf("t=%v len=%d cap=%d\n", t, len(t), cap(t))
}
```

**Output:**

```
t=[1 2] len=2 cap=3
```

---

## 19. Remove an element by index

`🔴 hard` · *Slices*

There's no built-in remove; the idiom is append(s[:i], s[i+1:]...) to splice out index i.

**Steps:**

1. Drop index 2 by joining the parts before and after it.
2. The spread ... feeds the tail back into append.

```go
package main

import "fmt"

func main() {
	s := []int{10, 20, 30, 40, 50}
	i := 2 // remove the element at index 2 (value 30)
	s = append(s[:i], s[i+1:]...)
	fmt.Println(s) // [10 20 40 50]
}
```

**Output:**

```
[10 20 40 50]
```

---

## 20. 2D / jagged slices

`🔴 hard` · *Slices*

A slice of slices can have rows of different lengths (jagged), since each row is its own slice.

**Steps:**

1. [][]int holds rows of varying length.
2. Range the outer slice to get each row.

```go
package main

import "fmt"

func main() {
	grid := [][]int{
		{1, 2, 3},
		{4, 5},
		{6},
	}
	for _, row := range grid {
		fmt.Println(row, "len:", len(row))
	}
}
```

**Output:**

```
[1 2 3] len: 3
[4 5] len: 2
[6] len: 1
```

---

## 21. A set via map[T]struct{}

`🔴 hard` · *Sets*

Go has no set type; the idiom is map[T]struct{} — struct{} uses zero bytes, so it's a memory-free presence marker.

**Steps:**

1. Insert with set[x] = struct{}{}.
2. Membership is the comma-ok test; sort keys to print.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	set := map[string]struct{}{}
	for _, w := range []string{"a", "b", "a", "c", "b"} {
		set[w] = struct{}{} // zero-byte value
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("unique:", keys)
	_, ok := set["a"]
	fmt.Println("contains a?", ok)
}
```

**Output:**

```
unique: [a b c]
contains a? true
```

---

## 22. Sorting slices

`🔴 hard` · *Sorting*

sort.Slice sorts in place with a custom less function; the newer slices package adds Sort, Contains, Max and more for common cases.

**Steps:**

1. sort.Slice with a less func orders any slice.
2. slices.Sort/Contains/Max are simpler for ordered element types.

```go
package main

import (
	"fmt"
	"slices"
	"sort"
)

func main() {
	nums := []int{5, 2, 8, 1, 9}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	fmt.Println("sorted:", nums)

	fmt.Println("contains 8?", slices.Contains(nums, 8))
	slices.Sort(nums)
	fmt.Println("slices.Sort:", nums)
	fmt.Println("max:", slices.Max(nums))
}
```

**Output:**

```
sorted: [1 2 5 8 9]
contains 8? true
slices.Sort: [1 2 5 8 9]
max: 9
```

---

