# Step 08 — Strings, Runes, Bytes & Formatting · 🟢 Easy

Examples **1–5**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

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

> ← Back to the [index](README.md) · Next tier: [🟡 medium](2-medium.md)
