# Step 07 — Arrays, Slices & Maps · 🔴 Hard

Examples **17–22**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Prev tier: [🟡 medium](2-medium.md)
