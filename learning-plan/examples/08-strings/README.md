# Step 08 — Strings, Runes, Bytes & Formatting · Examples

A library of **28 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–5 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 6–19 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 20–28 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. len is bytes; indexing gives a byte](1-easy.md#1-len-is-bytes-indexing-gives-a-byte)
- [2. Strings are immutable](1-easy.md#2-strings-are-immutable)
- [3. Concatenation and comparison](1-easy.md#3-concatenation-and-comparison)
- [4. Raw vs interpreted string literals](1-easy.md#4-raw-vs-interpreted-string-literals)
- [5. Iterating bytes by index](1-easy.md#5-iterating-bytes-by-index)

### 🟡 [Medium](2-medium.md)

- [6. Byte length vs rune count](2-medium.md#6-byte-length-vs-rune-count)
- [7. range decodes UTF-8](2-medium.md#7-range-decodes-utf-8)
- [8. string <-> []byte](2-medium.md#8-string---byte)
- [9. string <-> []rune](2-medium.md#9-string---rune)
- [10. Build strings with strings.Builder](2-medium.md#10-build-strings-with-stringsbuilder)
- [11. strings: Contains / HasPrefix / Index](2-medium.md#11-strings-contains--hasprefix--index)
- [12. strings: Split / Fields / Join](2-medium.md#12-strings-split--fields--join)
- [13. strings: Replace / ReplaceAll / Count](2-medium.md#13-strings-replace--replaceall--count)
- [14. strings: case and trimming](2-medium.md#14-strings-case-and-trimming)
- [15. strings: Repeat / EqualFold / Cut](2-medium.md#15-strings-repeat--equalfold--cut)
- [16. strconv: Atoi / Itoa](2-medium.md#16-strconv-atoi--itoa)
- [17. strconv: ParseFloat / ParseBool / FormatFloat](2-medium.md#17-strconv-parsefloat--parsebool--formatfloat)
- [18. strconv: Quote / Unquote](2-medium.md#18-strconv-quote--unquote)
- [19. unicode: classifying runes](2-medium.md#19-unicode-classifying-runes)

### 🔴 [Hard](3-hard.md)

- [20. fmt: %v, %+v, %#v, %T](3-hard.md#20-fmt-v-v-v-t)
- [21. fmt: integer verbs](3-hard.md#21-fmt-integer-verbs)
- [22. fmt: float verbs](3-hard.md#22-fmt-float-verbs)
- [23. fmt: %s vs %q, and slices](3-hard.md#23-fmt-s-vs-q-and-slices)
- [24. fmt: width, precision, flags](3-hard.md#24-fmt-width-precision-flags)
- [25. fmt: explicit argument indexes](3-hard.md#25-fmt-explicit-argument-indexes)
- [26. fmt: Sprintf and Fprintf](3-hard.md#26-fmt-sprintf-and-fprintf)
- [27. Counting runes with unicode/utf8](3-hard.md#27-counting-runes-with-unicodeutf8)
- [28. strings.Map and NewReplacer](3-hard.md#28-stringsmap-and-newreplacer)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
