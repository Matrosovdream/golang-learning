# Step 08 — Strings, Runes, Bytes & Formatting · 🟡 Medium

Examples **6–19**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)

---

## 6. Byte length vs rune count

`🟡 medium` · *Bytes & runes*

Multibyte UTF-8 characters make len (bytes) larger than the number of runes ([]rune length).

**Steps:**

1. é is 2 bytes, so len("héllo") is 6.
2. len([]rune(...)) counts 5 actual characters.

```go
package main

import "fmt"

func main() {
	s := "héllo"
	fmt.Println("bytes:", len(s))         // 6
	fmt.Println("runes:", len([]rune(s))) // 5
}
```

**Output:**

```
bytes: 6
runes: 5
```

---

## 7. range decodes UTF-8

`🟡 medium` · *Bytes & runes*

Ranging a string yields the starting BYTE INDEX and the decoded rune; the index skips ahead across multibyte runes.

**Steps:**

1. é occupies bytes 1-2, so the index jumps from 1 to 3.
2. The value is a rune (code point), not a byte.

```go
package main

import "fmt"

func main() {
	for i, r := range "héllo" {
		fmt.Printf("%d:%c ", i, r)
	}
	fmt.Println()
}
```

**Output:**

```
0:h 1:é 3:l 4:l 5:o 
```

---

## 8. string <-> []byte

`🟡 medium` · *Bytes & runes*

Converting between string and []byte copies the bytes; []byte is mutable, string is not.

**Steps:**

1. []byte(s) gives the raw bytes.
2. Edit/append the bytes, then string(b) to get a new string.

```go
package main

import "fmt"

func main() {
	s := "Go"
	b := []byte(s)
	fmt.Println(b) // [71 111]
	b = append(b, '!')
	fmt.Println(string(b))
}
```

**Output:**

```
[71 111]
Go!
```

---

## 9. string <-> []rune

`🟡 medium` · *Bytes & runes*

[]rune(s) decodes a string into code points, letting you index and slice by character instead of byte.

**Steps:**

1. len([]rune) is the character count.
2. Index and slice the rune slice, then convert back with string(...).

```go
package main

import "fmt"

func main() {
	r := []rune("héllo")
	fmt.Println("count:", len(r))
	fmt.Printf("r[1]=%c\n", r[1])
	fmt.Println(string(r[:2])) // "hé"
}
```

**Output:**

```
count: 5
r[1]=é
hé
```

---

## 10. Build strings with strings.Builder

`🟡 medium` · *Building strings*

Repeated + concatenation is O(n^2); strings.Builder appends efficiently and you read the result with String().

**Steps:**

1. Fprintf and WriteString append into the Builder.
2. Call b.String() once at the end.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "item%d ", i)
	}
	b.WriteString("done")
	fmt.Println(b.String())
}
```

**Output:**

```
item0 item1 item2 done
```

---

## 11. strings: Contains / HasPrefix / Index

`🟡 medium` · *strings package*

The strings package has the everyday search helpers: Contains, HasPrefix, HasSuffix, and Index (byte offset, or -1).

**Steps:**

1. Each returns a bool except Index.
2. Index gives the byte position of the match.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	s := "golang rocks"
	fmt.Println(strings.Contains(s, "lang"))
	fmt.Println(strings.HasPrefix(s, "go"))
	fmt.Println(strings.HasSuffix(s, "rocks"))
	fmt.Println(strings.Index(s, "rocks"))
}
```

**Output:**

```
true
true
true
7
```

---

## 12. strings: Split / Fields / Join

`🟡 medium` · *strings package*

Split breaks on a separator, Fields splits on runs of whitespace, and Join glues a slice back together.

**Steps:**

1. Split on a comma; Fields ignores extra spaces.
2. Join inserts a separator between elements.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.Split("a,b,c", ","))
	fmt.Println(strings.Fields("  x  y   z  "))
	fmt.Println(strings.Join([]string{"a", "b", "c"}, "-"))
}
```

**Output:**

```
[a b c]
[x y z]
a-b-c
```

---

## 13. strings: Replace / ReplaceAll / Count

`🟡 medium` · *strings package*

Replace swaps the first n matches; ReplaceAll swaps all; Count tallies non-overlapping occurrences.

**Steps:**

1. Replace with n=2 changes only the first two.
2. ReplaceAll changes every match; Count returns how many.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	s := "aaa"
	fmt.Println(strings.Replace(s, "a", "b", 2)) // bba
	fmt.Println(strings.ReplaceAll(s, "a", "b")) // bbb
	fmt.Println(strings.Count("banana", "a"))    // 3
}
```

**Output:**

```
bba
bbb
3
```

---

## 14. strings: case and trimming

`🟡 medium` · *strings package*

ToUpper/ToLower change case; TrimSpace removes surrounding whitespace; Trim removes any leading/trailing cutset runes.

**Steps:**

1. Upper/lower-case whole strings.
2. TrimSpace vs Trim with a custom cutset.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.ToUpper("go"))
	fmt.Println(strings.ToLower("GO"))
	fmt.Printf("%q\n", strings.TrimSpace("  hi  "))
	fmt.Println(strings.Trim("xxhixx", "x"))
}
```

**Output:**

```
GO
go
"hi"
hi
```

---

## 15. strings: Repeat / EqualFold / Cut

`🟡 medium` · *strings package*

Repeat duplicates a string; EqualFold compares case-insensitively; Cut splits once around a separator and reports if it was found.

**Steps:**

1. Repeat "ab" three times.
2. Cut returns before, after, and a found bool.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.Repeat("ab", 3))
	fmt.Println(strings.EqualFold("Go", "GO")) // true
	before, after, found := strings.Cut("key=value", "=")
	fmt.Println(before, after, found)
}
```

**Output:**

```
ababab
true
key value true
```

---

## 16. strconv: Atoi / Itoa

`🟡 medium` · *strconv*

Atoi parses a string to int (returning an error on bad input); Itoa formats an int to its decimal string.

**Steps:**

1. Atoi("42") returns 42 and a nil error.
2. Atoi on non-numeric text returns a descriptive error.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	n, err := strconv.Atoi("42")
	fmt.Println(n, err)
	fmt.Printf("%q\n", strconv.Itoa(100))
	_, err = strconv.Atoi("oops")
	fmt.Println(err)
}
```

**Output:**

```
42 <nil>
"100"
strconv.Atoi: parsing "oops": invalid syntax
```

---

## 17. strconv: ParseFloat / ParseBool / FormatFloat

`🟡 medium` · *strconv*

strconv parses and formats other types too: floats, bools, and controllable float formatting.

**Steps:**

1. ParseFloat and ParseBool read values from text.
2. FormatFloat with 'f', prec=2 renders 2 decimals.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	f, _ := strconv.ParseFloat("3.14", 64)
	b, _ := strconv.ParseBool("true")
	fmt.Println(f, b)
	fmt.Println(strconv.FormatFloat(3.14159, 'f', 2, 64)) // "3.14"
}
```

**Output:**

```
3.14 true
3.14
```

---

## 18. strconv: Quote / Unquote

`🟡 medium` · *strconv*

Quote turns a string into a double-quoted Go literal (escaping specials); Unquote reverses it.

**Steps:**

1. Quote escapes quotes and newlines into a literal.
2. Unquote parses that literal back to the original string.

```go
package main

import (
	"fmt"
	"strconv"
)

func main() {
	q := strconv.Quote("he said \"hi\"\n")
	fmt.Println(q)
	u, _ := strconv.Unquote(q)
	fmt.Printf("%q\n", u)
}
```

**Output:**

```
"he said \"hi\"\n"
"he said \"hi\"\n"
```

---

## 19. unicode: classifying runes

`🟡 medium` · *unicode*

The unicode package classifies and transforms runes: IsLetter, IsDigit, IsSpace, ToUpper, and more.

**Steps:**

1. Test a rune's category with the Is* functions.
2. ToUpper/ToLower transform a single rune.

```go
package main

import (
	"fmt"
	"unicode"
)

func main() {
	fmt.Println(unicode.IsLetter('A'), unicode.IsDigit('7'), unicode.IsSpace(' '))
	fmt.Printf("%c\n", unicode.ToUpper('a'))
}
```

**Output:**

```
true true true
A
```

---

> ← Back to the [index](README.md) · Prev tier: [🟢 easy](1-easy.md) · Next tier: [🔴 hard](3-hard.md)
