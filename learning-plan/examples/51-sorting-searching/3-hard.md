# Step 51 — Sorting & Searching · 🔴 Hard

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

## 18. Variable window: longest unique substring

`🔴 hard` · *Sliding window*

The length of the longest substring with no repeated character — the archetypal **variable-size** window. The right edge always advances; when the incoming character was already inside the window, jump the left edge just past its previous position. A map remembers each character's last index, so each index is touched at most twice → O(n).

**Steps:**

1. `last[c]` records the most recent index of character `c`.
2. On a duplicate that's still inside the window (`idx >= lo`), set `lo = idx + 1`.
3. The window `[lo, hi]` is always duplicate-free; track its max length.

```go
package main

import "fmt"

// longestUnique returns the length of the longest substring with no repeated
// byte. A window [lo,hi] grows on the right; when a duplicate enters, lo jumps
// past the previous occurrence. Each index is visited at most twice -> O(n).
func longestUnique(s string) int {
	last := map[byte]int{} // byte -> last index seen
	best, lo := 0, 0
	for hi := 0; hi < len(s); hi++ {
		if idx, ok := last[s[hi]]; ok && idx >= lo {
			lo = idx + 1 // shrink from the left past the duplicate
		}
		last[s[hi]] = hi
		if hi-lo+1 > best {
			best = hi - lo + 1
		}
	}
	return best
}

func main() {
	for _, s := range []string{"abcabcbb", "bbbbb", "pwwkew"} {
		fmt.Printf("%-10q %d\n", s, longestUnique(s))
	}
}
```

**Output:**

```
"abcabcbb" 3
"bbbbb"    1
"pwwkew"   3
```

---

## 19. Variable window: minimum subarray length

`🔴 hard` · *Sliding window*

The shortest contiguous subarray whose sum is **≥ target** (0 if none). Grow the window on the right, adding to a running sum; whenever the sum qualifies, **shrink from the left** as far as it still qualifies, recording the length each time. Both pointers only move forward → O(n).

**Steps:**

1. Add `nums[hi]` to the running `sum`.
2. While `sum >= target`, record the window length, then drop `nums[lo]` and advance `lo`.
3. If the window never qualified, return 0.

```go
package main

import "fmt"

// minSubarrayLen returns the length of the shortest contiguous subarray whose
// sum is >= target (0 if none). Grow the window on the right; whenever the sum
// qualifies, shrink from the left as far as it still qualifies. O(n).
func minSubarrayLen(target int, nums []int) int {
	best := len(nums) + 1
	sum, lo := 0, 0
	for hi := 0; hi < len(nums); hi++ {
		sum += nums[hi]
		for sum >= target {
			if hi-lo+1 < best {
				best = hi - lo + 1
			}
			sum -= nums[lo]
			lo++
		}
	}
	if best == len(nums)+1 {
		return 0
	}
	return best
}

func main() {
	nums := []int{2, 3, 1, 2, 4, 3}
	fmt.Println("min length for sum >= 7:", minSubarrayLen(7, nums)) // [4,3] -> 2
}
```

**Output:**

```
min length for sum >= 7: 2
```

---

## 20. Two-sum with a hash map

`🔴 hard` · *Map*

The unsorted cousin of example 12. Without a sorted slice, two pointers don't apply — but a map does. As you scan, remember each value's index; for each new value, check whether its **complement** (`target - v`) has already been seen. One O(n) pass, no sorting.

**Steps:**

1. `seen[value] = index` as you go.
2. Before storing `v`, look up `target - v`; a hit gives the answer pair.
3. Return the earlier index first.

```go
package main

import "fmt"

// twoSum returns the indices of the two numbers that add to target. A map of
// value->index lets us check for the needed complement in O(1) as we scan, so
// the whole thing is one O(n) pass — no sorting needed.
func twoSum(nums []int, target int) (int, int, bool) {
	seen := map[int]int{} // value -> index
	for i, v := range nums {
		if j, ok := seen[target-v]; ok {
			return j, i, true
		}
		seen[v] = i
	}
	return 0, 0, false
}

func main() {
	nums := []int{2, 7, 11, 15}
	i, j, ok := twoSum(nums, 9)
	fmt.Println("indices:", i, j, "ok:", ok) // 0 1
	i, j, ok = twoSum(nums, 26)
	fmt.Println("indices:", i, j, "ok:", ok) // 2 3
}
```

**Output:**

```
indices: 0 1 ok: true
indices: 2 3 ok: true
```

---

## 21. Group anagrams

`🔴 hard` · *Map*

Group words that are rearrangements of each other. The trick is a **canonical key**: two words are anagrams iff their letters, sorted, are identical — so the sorted-letter string is the map key that collects each group. A classic "normalise, then bucket in a map" problem.

**Steps:**

1. For each word, sort its bytes to form the canonical key.
2. Append the original word to `groups[key]`.
3. Collect the buckets and sort them for deterministic output.

```go
package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// groupAnagrams buckets words that are anagrams of each other. The key insight:
// two words are anagrams iff their sorted letters match, so the sorted string is
// a canonical map key.
func groupAnagrams(words []string) [][]string {
	groups := map[string][]string{}
	for _, w := range words {
		letters := []byte(w)
		sort.Slice(letters, func(i, j int) bool { return letters[i] < letters[j] })
		key := string(letters)
		groups[key] = append(groups[key], w)
	}
	// Collect and sort for deterministic output.
	out := make([][]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b []string) int {
		return strings.Compare(a[0], b[0])
	})
	return out
}

func main() {
	words := []string{"eat", "tea", "tan", "ate", "nat", "bat"}
	for _, g := range groupAnagrams(words) {
		fmt.Println(g)
	}
}
```

**Output:**

```
[bat]
[eat tea ate]
[tan nat]
```

---

## 22. Dutch national flag

`🔴 hard` · *Partition*

Sort an array of only `0`s, `1`s, and `2`s in a **single pass** with three pointers (Dijkstra's Dutch national flag). `lo` is the boundary below which everything is `0`, `hi` the boundary above which everything is `2`, and `i` scans. The subtlety: after swapping a `2` in from the right, **don't advance `i`** — the value you pulled in is still unexamined.

**Steps:**

1. `nums[i] == 0` → swap into the `lo` region, advance both `lo` and `i`.
2. `nums[i] == 2` → swap into the `hi` region, decrement `hi`, but **hold `i`**.
3. `nums[i] == 1` → just advance `i`.

```go
package main

import "fmt"

// sortColors sorts an array of 0s, 1s, and 2s in a single pass using three
// pointers (Dijkstra's Dutch national flag). lo tracks the 0/1 boundary, hi the
// 1/2 boundary, i scans. Everything below lo is 0, above hi is 2.
func sortColors(nums []int) {
	lo, i, hi := 0, 0, len(nums)-1
	for i <= hi {
		switch nums[i] {
		case 0:
			nums[lo], nums[i] = nums[i], nums[lo]
			lo++
			i++
		case 2:
			nums[hi], nums[i] = nums[i], nums[hi]
			hi-- // don't advance i: the swapped-in value is unexamined
		default:
			i++
		}
	}
}

func main() {
	nums := []int{2, 0, 2, 1, 1, 0, 1, 2, 0}
	sortColors(nums)
	fmt.Println(nums)
}
```

**Output:**

```
[0 0 0 1 1 1 2 2 2]
```

---

## 23. Quickselect: the k-th smallest

`🔴 hard` · *Selection*

Find the k-th smallest element **without fully sorting** — O(n) average. Quickselect partitions like quicksort (Lomuto scheme: pick a pivot, move smaller elements left), but then recurses into only the **one** side that contains index `k`. Half the work is thrown away each step.

**Steps:**

1. `partition` places the pivot at its final sorted index `p`.
2. If `p == k`, done; if `p < k`, search the right side; else the left.
3. Loop until the pivot lands exactly on `k`.

```go
package main

import "fmt"

// quickselect returns the k-th smallest element (0-indexed) in O(n) average time
// by partitioning like quicksort but recursing into only ONE side. It reorders
// nums in place.
func quickselect(nums []int, k int) int {
	lo, hi := 0, len(nums)-1
	for lo < hi {
		p := partition(nums, lo, hi)
		switch {
		case p == k:
			return nums[p]
		case p < k:
			lo = p + 1
		default:
			hi = p - 1
		}
	}
	return nums[lo]
}

// partition uses the last element as pivot (Lomuto scheme) and returns its final
// resting index; everything left is <= pivot, everything right is > pivot.
func partition(nums []int, lo, hi int) int {
	pivot := nums[hi]
	i := lo
	for j := lo; j < hi; j++ {
		if nums[j] <= pivot {
			nums[i], nums[j] = nums[j], nums[i]
			i++
		}
	}
	nums[i], nums[hi] = nums[hi], nums[i]
	return i
}

func main() {
	nums := []int{7, 2, 9, 4, 1, 8, 3}
	// 3rd smallest (k=2, 0-indexed) of {1,2,3,4,7,8,9} is 3.
	fmt.Println("3rd smallest:", quickselect(nums, 2))
}
```

**Output:**

```
3rd smallest: 3
```

---

## 24. Merge sort from scratch

`🔴 hard` · *Divide & conquer*

The classic O(n log n) **stable** sort, worth writing once to feel divide-and-conquer: split the slice in half, sort each half recursively, then **merge** the two sorted halves by repeatedly taking the smaller head. In real code you'd call `slices.Sort` — but the `merge` step here is the same one behind merging lists, intervals, and external sorts.

**Steps:**

1. Base case: a slice of length ≤ 1 is already sorted.
2. Recurse on the left and right halves.
3. `merge` walks both sorted halves, appending the smaller head each step.

```go
package main

import "fmt"

// mergeSort is the classic divide-and-conquer O(n log n) STABLE sort: split in
// half, sort each half, then merge. It returns a new sorted slice.
func mergeSort(nums []int) []int {
	if len(nums) <= 1 {
		return nums
	}
	mid := len(nums) / 2
	left := mergeSort(nums[:mid])
	right := mergeSort(nums[mid:])
	return merge(left, right)
}

// merge combines two sorted slices into one, taking the smaller head each step.
func merge(a, b []int) []int {
	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] <= b[j] {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	out = append(out, a[i:]...) // one of these is empty
	out = append(out, b[j:]...)
	return out
}

func main() {
	nums := []int{5, 2, 8, 1, 9, 3, 7, 4}
	fmt.Println(mergeSort(nums))
}
```

**Output:**

```
[1 2 3 4 5 7 8 9]
```

---

## 25. Merge overlapping intervals

`🔴 hard` · *Sort + sweep*

A hugely common real pattern: **sort, then sweep once.** Given intervals like meeting times, collapse the overlapping ones. Sort by start, then walk through: each interval either overlaps the last merged one (extend its end) or doesn't (start a new merged interval). The sort makes the single linear sweep possible.

**Steps:**

1. `slices.SortFunc` by `start`.
2. If the current interval starts at or before the last merged one's end, extend that end.
3. Otherwise append it as a new merged interval.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type interval struct{ start, end int }

// mergeIntervals collapses overlapping intervals. The key move: SORT by start,
// then sweep once — each interval either extends the current merged one (overlap)
// or starts a new one. Sort + linear scan is a hugely common pattern.
func mergeIntervals(ivs []interval) []interval {
	slices.SortFunc(ivs, func(a, b interval) int { return cmp.Compare(a.start, b.start) })
	var out []interval
	for _, iv := range ivs {
		n := len(out)
		if n > 0 && iv.start <= out[n-1].end {
			if iv.end > out[n-1].end {
				out[n-1].end = iv.end // extend the last merged interval
			}
		} else {
			out = append(out, iv) // no overlap: start a new one
		}
	}
	return out
}

func main() {
	ivs := []interval{{1, 3}, {2, 6}, {8, 10}, {15, 18}, {9, 12}}
	for _, iv := range mergeIntervals(ivs) {
		fmt.Printf("[%d,%d] ", iv.start, iv.end)
	}
	fmt.Println()
}
```

**Output:**

```
[1,6] [8,12] [15,18] 
```

---

## 26. Top-K frequent elements

`🔴 hard` · *Map + sort*

The capstone: combine **frequency counting** with a **custom sort**. Count occurrences into a map, then sort the distinct values by count (descending, tie-broken by value) and take the first `k`. For very large inputs with small `k`, a heap does this in O(n log k) — see [52 — Heaps & Priority Queues](../52-heaps-priority-queues/) — but sorting is the clearest baseline.

**Steps:**

1. Count with `freq[v]++`.
2. Sort the distinct values with `cmp.Or(count desc, value asc)`.
3. Return the first `k` (clamped to how many distinct values exist).

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

// topK returns the k most frequent values, most frequent first. Count with a
// map, then sort the distinct values by count. (For large n with small k, a heap
// gives O(n log k) — see lesson 52; here sorting is clearest.)
func topK(nums []int, k int) []int {
	freq := map[int]int{}
	for _, v := range nums {
		freq[v]++
	}
	distinct := make([]int, 0, len(freq))
	for v := range freq {
		distinct = append(distinct, v)
	}
	slices.SortFunc(distinct, func(a, b int) int {
		return cmp.Or(
			cmp.Compare(freq[b], freq[a]), // higher count first
			cmp.Compare(a, b),             // ties: smaller value first
		)
	})
	if k > len(distinct) {
		k = len(distinct)
	}
	return distinct[:k]
}

func main() {
	nums := []int{1, 1, 1, 2, 2, 3, 4, 4, 4, 4}
	fmt.Println("top 2:", topK(nums, 2)) // 4 (x4), 1 (x3)
}
```

**Output:**

```
top 2: [4 1]
```

---

> Prev tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
