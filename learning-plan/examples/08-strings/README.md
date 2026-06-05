# Step 08 — Strings, Runes, Bytes & Formatting · Examples

A library of **28 runnable examples**. Each is a complete `package main` program:
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

- [1. len is bytes; indexing gives a byte](#1-len-is-bytes-indexing-gives-a-byte)
- [2. Strings are immutable](#2-strings-are-immutable)
- [3. Concatenation and comparison](#3-concatenation-and-comparison)
- [4. Raw vs interpreted string literals](#4-raw-vs-interpreted-string-literals)
- [5. Iterating bytes by index](#5-iterating-bytes-by-index)

**Medium**

- [6. Byte length vs rune count](#6-byte-length-vs-rune-count)
- [7. range decodes UTF-8](#7-range-decodes-utf-8)
- [8. string <-> []byte](#8-string---byte)
- [9. string <-> []rune](#9-string---rune)
- [10. Build strings with strings.Builder](#10-build-strings-with-stringsbuilder)
- [11. strings: Contains / HasPrefix / Index](#11-strings-contains--hasprefix--index)
- [12. strings: Split / Fields / Join](#12-strings-split--fields--join)
- [13. strings: Replace / ReplaceAll / Count](#13-strings-replace--replaceall--count)
- [14. strings: case and trimming](#14-strings-case-and-trimming)
- [15. strings: Repeat / EqualFold / Cut](#15-strings-repeat--equalfold--cut)
- [16. strconv: Atoi / Itoa](#16-strconv-atoi--itoa)
- [17. strconv: ParseFloat / ParseBool / FormatFloat](#17-strconv-parsefloat--parsebool--formatfloat)
- [18. strconv: Quote / Unquote](#18-strconv-quote--unquote)
- [19. unicode: classifying runes](#19-unicode-classifying-runes)

**Hard**

- [20. fmt: %v, %+v, %#v, %T](#20-fmt-v-v-v-t)
- [21. fmt: integer verbs](#21-fmt-integer-verbs)
- [22. fmt: float verbs](#22-fmt-float-verbs)
- [23. fmt: %s vs %q, and slices](#23-fmt-s-vs-q-and-slices)
- [24. fmt: width, precision, flags](#24-fmt-width-precision-flags)
- [25. fmt: explicit argument indexes](#25-fmt-explicit-argument-indexes)
- [26. fmt: Sprintf and Fprintf](#26-fmt-sprintf-and-fprintf)
- [27. Counting runes with unicode/utf8](#27-counting-runes-with-unicodeutf8)
- [28. strings.Map and NewReplacer](#28-stringsmap-and-newreplacer)

---

## 1. len is bytes; indexing gives a byte

`🟢 easy` · *String basics*

A string is a read-only sequence of bytes: len counts bytes and s[i] is a byte (uint8), not a character.

**Steps:**

1. len("hello") is 5 bytes.
2. s[0] is the byte 104; print it as a character with %c.

```go
package main

import "fmt"

func main() {
	s := "hello"
	fmt.Println("len:", len(s)) // bytes
	fmt.Println("s[0]:", s[0])  // a byte (uint8) = 104
	fmt.Printf("s[0] as char: %c\n", s[0])
}
```

**Output:**

```
len: 5
s[0]: 104
s[0] as char: h
```

---

## 2. Strings are immutable

`🟢 easy` · *String basics*

You cannot assign to s[i]; to change a string, convert to []byte, edit, and convert back.

**Steps:**

1. The commented line shows s[0]='H' is a compile error.
2. Round-trip through []byte to produce a modified string.

```go
package main

import "fmt"

func main() {
	s := "hello"
	// s[0] = 'H' // compile error: cannot assign to s[0]
	b := []byte(s)
	b[0] = 'H'
	fmt.Println(s, "->", string(b))
}
```

**Output:**

```
hello -> Hello
```

---

## 3. Concatenation and comparison

`🟢 easy` · *String basics*

+ joins strings, and the comparison operators order them lexicographically by bytes.

**Steps:**

1. a + b concatenates.
2. < compares lexicographically; == checks equality.

```go
package main

import "fmt"

func main() {
	a := "go"
	b := "lang"
	fmt.Println(a + b)
	fmt.Println("apple" < "banana") // lexicographic
	fmt.Println("Go" == "Go")
}
```

**Output:**

```
golang
true
true
```

---

## 4. Raw vs interpreted string literals

`🟢 easy` · *String basics*

Double quotes interpret escapes like \n; backtick `raw` strings take the text literally and can span lines.

**Steps:**

1. The interpreted string turns \n into a newline.
2. The raw string keeps \n as two characters.

```go
package main

import "fmt"

func main() {
	interpreted := "line1\nline2"
	raw := `line1\nline2` // backticks: no escapes
	fmt.Println(interpreted)
	fmt.Println(raw)
}
```

**Output:**

```
line1
line2
line1\nline2
```

---

## 5. Iterating bytes by index

`🟢 easy` · *Bytes & runes*

Indexing from 0 to len-1 walks the bytes; for pure ASCII that's also the characters.

**Steps:**

1. Loop i from 0 to len(s)-1.
2. s[i] is each byte; %c shows it.

```go
package main

import "fmt"

func main() {
	s := "abc"
	for i := 0; i < len(s); i++ {
		fmt.Printf("%d=%c ", i, s[i])
	}
	fmt.Println()
}
```

**Output:**

```
0=a 1=b 2=c 
```

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

## 20. fmt: %v, %+v, %#v, %T

`🔴 hard` · *fmt verbs*

The core fmt verbs: %v (value), %+v (with field names), %#v (Go syntax), and %T (the type).

**Steps:**

1. Print a struct four ways.
2. Note how each verb adds more detail.

```go
package main

import "fmt"

type Point struct{ X, Y int }

func main() {
	p := Point{1, 2}
	fmt.Printf("%v\n", p)  // {1 2}
	fmt.Printf("%+v\n", p) // {X:1 Y:2}
	fmt.Printf("%#v\n", p) // main.Point{X:1, Y:2}
	fmt.Printf("%T\n", p)  // main.Point
}
```

**Output:**

```
{1 2}
{X:1 Y:2}
main.Point{X:1, Y:2}
main.Point
```

---

## 21. fmt: integer verbs

`🔴 hard` · *fmt verbs*

Integers can print in many bases and as characters: %d %b %o %x %X %c %U.

**Steps:**

1. Print 65 in decimal, binary, octal, hex.
2. %c is the character, %U the Unicode code point.

```go
package main

import "fmt"

func main() {
	n := 65
	fmt.Printf("dec=%d bin=%b oct=%o hex=%x HEX=%X\n", n, n, n, n, n)
	fmt.Printf("char=%c unicode=%U\n", n, n)
}
```

**Output:**

```
dec=65 bin=1000001 oct=101 hex=41 HEX=41
char=A unicode=U+0041
```

---

## 22. fmt: float verbs

`🔴 hard` · *fmt verbs*

Floats: %f (decimal), %.2f (fixed precision), %e (scientific), %g (compact).

**Steps:**

1. Same number, four formats.
2. %.2f rounds to two decimals.

```go
package main

import "fmt"

func main() {
	f := 1234.5678
	fmt.Printf("%f\n", f)   // 1234.567800
	fmt.Printf("%.2f\n", f) // 1234.57
	fmt.Printf("%e\n", f)   // 1.234568e+03
	fmt.Printf("%g\n", f)   // 1234.5678
}
```

**Output:**

```
1234.567800
1234.57
1.234568e+03
1234.5678
```

---

## 23. fmt: %s vs %q, and slices

`🔴 hard` · *fmt verbs*

%s prints a string raw; %q wraps it as a quoted Go literal showing escapes; %v works for composite values like slices.

**Steps:**

1. %s expands a tab; %q shows it as \t inside quotes.
2. %v prints a slice cleanly. (%p prints a pointer address, which varies per run.)

```go
package main

import "fmt"

func main() {
	s := "hi\tthere"
	fmt.Printf("%s\n", s) // raw (tab expands)
	fmt.Printf("%q\n", s) // quoted literal with escapes
	fmt.Printf("%v\n", []int{1, 2, 3})
}
```

**Output:**

```
hi	there
"hi\tthere"
[1 2 3]
```

---

## 24. fmt: width, precision, flags

`🔴 hard` · *fmt verbs*

Modifiers control layout: %-10s left-justifies, %10s right-justifies, %08d zero-pads, %+d shows the sign, %6.2f sets width+precision.

**Steps:**

1. Pipes show the field boundaries.
2. Combine width and precision like %6.2f.

```go
package main

import "fmt"

func main() {
	fmt.Printf("|%-10s|\n", "left")
	fmt.Printf("|%10s|\n", "right")
	fmt.Printf("|%08d|\n", 42)
	fmt.Printf("|%+d|\n", 42)
	fmt.Printf("|%6.2f|\n", 3.14159)
}
```

**Output:**

```
|left      |
|     right|
|00000042|
|+42|
|  3.14|
```

---

## 25. fmt: explicit argument indexes

`🔴 hard` · *fmt verbs*

%[n] selects which argument a verb consumes, letting you reuse or reorder arguments.

**Steps:**

1. %[1]d ... %[1]b reuses the same argument twice.
2. %[2]s %[1]s reorders the arguments.

```go
package main

import "fmt"

func main() {
	fmt.Printf("%[1]d in binary is %[1]b\n", 5)
	fmt.Printf("%[2]s %[1]s\n", "world", "hello")
}
```

**Output:**

```
5 in binary is 101
hello world
```

---

## 26. fmt: Sprintf and Fprintf

`🔴 hard` · *fmt verbs*

Sprintf returns a formatted string instead of printing; Fprintf writes formatted output to any io.Writer.

**Steps:**

1. Sprintf builds a string you can store.
2. Fprintf(os.Stdout, ...) writes to a chosen destination.

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	s := fmt.Sprintf("%s is %d", "age", 30)
	fmt.Println(s)
	fmt.Fprintf(os.Stdout, "written to stdout: %d\n", 7)
}
```

**Output:**

```
age is 30
written to stdout: 7
```

---

## 27. Counting runes with unicode/utf8

`🔴 hard` · *UTF-8*

The unicode/utf8 package works at the byte level: RuneCountInString counts characters and ValidString checks encoding.

**Steps:**

1. Compare len (bytes) with utf8.RuneCountInString (runes).
2. ValidString reports whether the bytes are valid UTF-8.

```go
package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	s := "héllo, 世界"
	fmt.Println("bytes:", len(s))
	fmt.Println("runes:", utf8.RuneCountInString(s))
	fmt.Println("valid UTF-8?", utf8.ValidString(s))
}
```

**Output:**

```
bytes: 14
runes: 9
valid UTF-8? true
```

---

## 28. strings.Map and NewReplacer

`🔴 hard` · *strings package*

Map rewrites each rune through a function; NewReplacer does many literal substring replacements in one efficient pass.

**Steps:**

1. Map shifts each lowercase letter by one (a Caesar-style transform).
2. NewReplacer swaps several substrings at once.

```go
package main

import (
	"fmt"
	"strings"
)

func main() {
	rot := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return 'a' + (r-'a'+1)%26
		}
		return r
	}, "abz")
	fmt.Println(rot) // bca

	rep := strings.NewReplacer("cat", "dog", "fast", "slow")
	fmt.Println(rep.Replace("a fast cat"))
}
```

**Output:**

```
bca
a slow dog
```

---

