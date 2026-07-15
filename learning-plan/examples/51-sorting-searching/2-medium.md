# Step 51 — Sorting & Searching · 🟡 Medium

Examples **9–17**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Prev tier: [🟢 easy](1-easy.md) · Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🔴 hard](3-hard.md)

---

## 9. Implement sort.Interface

`🟡 medium` · *sort.Interface*

The original, most explicit way to sort: define `Len`, `Less`, and `Swap` on a named slice type. `sort.Slice` and `slices.SortFunc` are conveniences layered over this. Implementing it once shows what the sort actually calls — and `sort.Reverse` wraps **any** `sort.Interface` to flip the order.

**Steps:**

1. Define a type over `[]string` with `Len`/`Less`/`Swap`.
2. `sort.Sort(byLength(words))` sorts by the `Less` you wrote (here, string length).
3. `sort.Sort(sort.Reverse(byLength(words)))` reverses without a second type.

```go
package main

import (
	"fmt"
	"sort"
)

// byLength implements sort.Interface (Len, Less, Swap) on a []string. This is
// the original, most explicit way to sort — sort.Slice and slices.SortFunc are
// shortcuts for it.
type byLength []string

func (s byLength) Len() int           { return len(s) }
func (s byLength) Less(i, j int) bool { return len(s[i]) < len(s[j]) }
func (s byLength) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func main() {
	words := []string{"peach", "kiwi", "fig", "banana"}
	sort.Sort(byLength(words))
	fmt.Println(words)

	// sort.Reverse wraps any sort.Interface to flip the order.
	sort.Sort(sort.Reverse(byLength(words)))
	fmt.Println(words)
}
```

**Output:**

```
[fig kiwi peach banana]
[banana peach kiwi fig]
```

---

## 10. Boundary search with sort.Search

`🟡 medium` · *Search*

`sort.Search` is binary search on a **boundary**: given a predicate that goes `false…false, true…true`, it returns the first index where it's `true`. That's a lower bound. Run it twice — `>= x` and `> x` — and the gap between the answers is the count of `x`. This "find the boundary" shape generalises to binary-searching an answer.

**Steps:**

1. `lo = Search(n, nums[i] >= x)` — first index not less than `x`.
2. `hi = Search(n, nums[i] > x)` — first index greater than `x`.
3. `[lo, hi)` is the run of `x`; `hi - lo` is how many there are.

```go
package main

import (
	"fmt"
	"sort"
)

func main() {
	// sort.Search finds the smallest index in [0,n) where the predicate is true,
	// assuming the predicate is false...false,true...true. It's binary search on
	// a boundary — great for "first element >= x" (lower bound).
	nums := []int{1, 2, 4, 4, 4, 6, 8}
	x := 4

	lo := sort.Search(len(nums), func(i int) bool { return nums[i] >= x })
	hi := sort.Search(len(nums), func(i int) bool { return nums[i] > x })
	fmt.Printf("value %d occupies indices [%d, %d) -> count %d\n", x, lo, hi, hi-lo)
}
```

**Output:**

```
value 4 occupies indices [2, 5) -> count 3
```

---

## 11. Binary-search structs by key

`🟡 medium` · *Search*

`slices.BinarySearchFunc` searches a slice that's sorted by some **key**, without needing the target to be the same type as the elements. The comparator compares an element against the target and returns the usual `-1/0/+1`. The slice must be sorted by the exact key the comparator uses.

**Steps:**

1. Keep `users` sorted by `id`.
2. Search for `id == 30` with a comparator `cmp.Compare(u.id, target)`.
3. Get back the index and a `found` flag.

```go
package main

import (
	"cmp"
	"fmt"
	"slices"
)

type user struct {
	id   int
	name string
}

func main() {
	// The slice must be sorted by the SAME key the comparator uses. Here users
	// are sorted by id, and we search for id 30 by comparing against it.
	users := []user{
		{10, "Ann"},
		{20, "Ben"},
		{30, "Cara"},
		{40, "Dan"},
	}
	i, found := slices.BinarySearchFunc(users, 30, func(u user, target int) int {
		return cmp.Compare(u.id, target)
	})
	fmt.Println("index:", i, "found:", found, "name:", users[i].name)
}
```

**Output:**

```
index: 2 found: true name: Cara
```

---

## 12. Two pointers: pair sum

`🟡 medium` · *Two pointers*

On a **sorted** slice, find two values that add to a target without the O(n²) double loop. Put one pointer at each end: if the sum is too big, the biggest element is too big — move the right pointer left; if too small, move the left pointer right. O(n), O(1) memory.

**Steps:**

1. `lo` at the start, `hi` at the end.
2. Compare `nums[lo] + nums[hi]` to the target and move the pointer that helps.
3. Stop when they meet (no pair) or the sum matches.

```go
package main

import "fmt"

// twoSumSorted finds two values in a SORTED slice that add to target. One pointer
// starts at each end: if the sum is too big, move the right pointer left; too
// small, move the left pointer right. O(n), no extra memory.
func twoSumSorted(nums []int, target int) (int, int, bool) {
	lo, hi := 0, len(nums)-1
	for lo < hi {
		sum := nums[lo] + nums[hi]
		switch {
		case sum == target:
			return nums[lo], nums[hi], true
		case sum < target:
			lo++
		default:
			hi--
		}
	}
	return 0, 0, false
}

func main() {
	nums := []int{1, 3, 4, 5, 7, 11}
	a, b, ok := twoSumSorted(nums, 9)
	fmt.Println(a, b, ok) // 4 5
	_, _, ok = twoSumSorted(nums, 100)
	fmt.Println("found 100?", ok)
}
```

**Output:**

```
4 5 true
found 100? false
```

---

## 13. Two pointers: remove duplicates

`🟡 medium` · *Two pointers*

Remove duplicates from a **sorted** slice **in place**, with no extra allocation — the fast/slow two-pointer pattern. `slow` marks the end of the unique prefix; `fast` scans ahead, and each time it finds a new value it's copied just past `slow`. Return the prefix.

**Steps:**

1. `slow = 0`; `fast` runs from 1 to the end.
2. When `nums[fast] != nums[slow]`, advance `slow` and copy the new value there.
3. Return `nums[:slow+1]` — the compacted unique prefix.

```go
package main

import "fmt"

// dedup removes consecutive duplicates from a SORTED slice in place. A slow
// pointer marks the end of the unique prefix; a fast pointer scans ahead and
// copies each new value forward. Returns the deduped prefix.
func dedup(nums []int) []int {
	if len(nums) == 0 {
		return nums
	}
	slow := 0
	for fast := 1; fast < len(nums); fast++ {
		if nums[fast] != nums[slow] {
			slow++
			nums[slow] = nums[fast]
		}
	}
	return nums[:slow+1]
}

func main() {
	nums := []int{1, 1, 2, 2, 2, 3, 4, 4}
	fmt.Println("unique:", dedup(nums))
}
```

**Output:**

```
unique: [1 2 3 4]
```

---

## 14. Two pointers: palindrome

`🟡 medium` · *Two pointers*

Check a phrase reads the same forwards and backwards, ignoring case and punctuation. Two pointers walk **inward** from both ends; inner loops skip any non-alphanumeric rune before each comparison. Working on `[]rune` handles multi-byte characters correctly.

**Steps:**

1. Lowercase and convert to `[]rune`.
2. From each end, skip runes that aren't letters/digits, then compare.
3. Any mismatch → not a palindrome; pointers crossing → it is.

```go
package main

import (
	"fmt"
	"strings"
	"unicode"
)

// isPalindrome checks a string ignoring case and non-letter/digit runes, using
// two pointers walking inward from both ends.
func isPalindrome(s string) bool {
	r := []rune(strings.ToLower(s))
	lo, hi := 0, len(r)-1
	for lo < hi {
		for lo < hi && !unicode.IsLetter(r[lo]) && !unicode.IsDigit(r[lo]) {
			lo++
		}
		for lo < hi && !unicode.IsLetter(r[hi]) && !unicode.IsDigit(r[hi]) {
			hi--
		}
		if r[lo] != r[hi] {
			return false
		}
		lo++
		hi--
	}
	return true
}

func main() {
	for _, s := range []string{"A man, a plan, a canal: Panama", "race a car", "No 'x' in Nixon"} {
		fmt.Printf("%-32q %v\n", s, isPalindrome(s))
	}
}
```

**Output:**

```
"A man, a plan, a canal: Panama" true
"race a car"                     false
"No 'x' in Nixon"                true
```

---

## 15. A set from a map

`🟡 medium` · *Set*

Go has no set type — you build one from a map. `map[T]struct{}` is the idiom: the `struct{}` value takes **zero bytes**, so the map stores only keys. Wrap it in a generic `Set[T comparable]` with `Add`/`Has`/`Len`, and set operations (union, intersection) are simple loops.

**Steps:**

1. `type Set[T comparable] map[T]struct{}` with `Add`/`Has`/`Len` methods.
2. Adding a duplicate is a no-op, so the set naturally dedups.
3. Intersection: keep the elements of one set that are present in the other (sort the result, since map order is random).

```go
package main

import (
	"fmt"
	"slices"
)

// Set is the idiomatic Go set: a map to empty structs (struct{} uses zero bytes).
type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(v T)      { s[v] = struct{}{} }
func (s Set[T]) Has(v T) bool { _, ok := s[v]; return ok }
func (s Set[T]) Len() int     { return len(s) }

func main() {
	a := Set[int]{}
	for _, v := range []int{1, 2, 3, 3, 2} {
		a.Add(v)
	}
	fmt.Println("len:", a.Len(), "has 3:", a.Has(3), "has 9:", a.Has(9))

	b := Set[int]{}
	b.Add(2)
	b.Add(4)

	// Intersection: keep elements of a that are also in b.
	var inter []int
	for v := range a {
		if b.Has(v) {
			inter = append(inter, v)
		}
	}
	slices.Sort(inter) // map order is random; sort for stable output
	fmt.Println("intersection:", inter)
}
```

**Output:**

```
len: 3 has 3: true has 9: false
intersection: [2]
```

---

## 16. Frequency counting

`🟡 medium` · *Map*

Counting occurrences is the other everyday map trick: `freq[key]++`. A missing key reads as the zero value `0`, so the increment just works — no "does this key exist yet" check. To emit results in a stable order, pull the keys into a slice and sort them (map iteration order is deliberately random).

**Steps:**

1. `freq[w]++` for each word.
2. Copy the keys into a slice.
3. Sort by count descending, breaking ties alphabetically, then print.

```go
package main

import (
	"fmt"
	"sort"
	"strings"
)

func main() {
	text := "the quick brown fox the lazy dog the fox"
	freq := map[string]int{}
	for _, w := range strings.Fields(text) {
		freq[w]++ // a missing key reads as 0, so ++ just works
	}

	// To print by frequency, pull keys into a slice and sort (map order is random).
	words := make([]string, 0, len(freq))
	for w := range freq {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if freq[words[i]] != freq[words[j]] {
			return freq[words[i]] > freq[words[j]] // by count desc
		}
		return words[i] < words[j] // ties: alphabetical
	})
	for _, w := range words {
		fmt.Printf("%s: %d\n", w, freq[w])
	}
}
```

**Output:**

```
the: 3
fox: 2
brown: 1
dog: 1
lazy: 1
quick: 1
```

---

## 17. Fixed sliding window: max sum

`🟡 medium` · *Sliding window*

The largest sum of any `k` consecutive elements. The naive way recomputes each window in O(n·k). The sliding window keeps a **running sum**: when it moves one step right, add the entering element and subtract the leaving one — O(n) total.

**Steps:**

1. Sum the first `k` elements as the initial window.
2. Slide: `sum += nums[i] - nums[i-k]` for each new right edge.
3. Track the best sum seen.

```go
package main

import "fmt"

// maxSumK returns the largest sum of any k consecutive elements. Instead of
// recomputing each window (O(n*k)), slide it: add the entering element and
// subtract the leaving one — O(n).
func maxSumK(nums []int, k int) int {
	if len(nums) < k {
		return 0
	}
	sum := 0
	for i := 0; i < k; i++ {
		sum += nums[i] // first window
	}
	best := sum
	for i := k; i < len(nums); i++ {
		sum += nums[i] - nums[i-k] // slide: +new -old
		if sum > best {
			best = sum
		}
	}
	return best
}

func main() {
	nums := []int{2, 1, 5, 1, 3, 2}
	fmt.Println("max sum of 3:", maxSumK(nums, 3)) // 5+1+3 = 9
}
```

**Output:**

```
max sum of 3: 9
```

---

> Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md) · Back to the [index](README.md)
