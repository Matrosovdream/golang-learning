# 08 — Strings, Runes, Bytes & Formatting

## Goals
- Understand that Go strings are immutable UTF-8 byte sequences.
- Know the difference between a byte, a rune, and a string index.
- Use the `strings` and `strconv` packages for common tasks.
- Format values precisely with `fmt` verbs and build strings efficiently.

## Concepts
- **A string is immutable bytes.** Internally a `string` is a read-only slice of bytes that *usually* holds UTF-8-encoded text. You can't modify a string in place (`s[0] = 'x'` won't compile); you build a new one.
- **`len(s)` counts bytes, not characters.** `len("héllo")` is 6, not 5, because `é` is 2 bytes in UTF-8. This trips up everyone once.
- **Indexing returns a byte.** `s[0]` is a `byte` (`uint8`), not a character. For ASCII that's the same thing; for multibyte runes it's just one byte of several.
- **Runes** — a `rune` is an `int32` holding a Unicode **code point** (a "character"). To iterate by character, `range` decodes UTF-8 into runes automatically:
  ```go
  for i, r := range "héllo" {
      // i = byte index (jumps 0,1,3,4,5), r = rune
  }
  ```
- **Three views of text, three conversions:**
  - `[]byte(s)` — the raw bytes (copy). Used for I/O, hashing, network.
  - `[]rune(s)` — the code points (copy). Used when you need to index/count *characters*.
  - `string(b)` / `string(r)` — convert bytes or runes back to a string.
  - **Gotcha:** `string(65)` is `"A"` (code point → char), not `"65"`. To turn a number into its digits, use `strconv.Itoa`.
- **`strconv`** — string ⇄ number conversions (with errors, because parsing can fail):
  ```go
  n, err := strconv.Atoi("42")        // string → int
  s := strconv.Itoa(42)               // int → string
  f, err := strconv.ParseFloat("3.14", 64)
  b, err := strconv.ParseBool("true")
  ```
- **`strings` package** — the everyday text toolkit: `Contains`, `HasPrefix`/`HasSuffix`, `Split`, `Join`, `Replace`/`ReplaceAll`, `ToLower`/`ToUpper`, `TrimSpace`/`Trim`, `Fields` (split on whitespace), `Index`, `Count`.
- **`strings.Builder`** — efficient string concatenation. Building strings with `+=` in a loop reallocates every time (O(n²)); `Builder` writes into a growing buffer:
  ```go
  var b strings.Builder
  for _, w := range words {
      b.WriteString(w)
      b.WriteByte(' ')
  }
  result := b.String()
  ```
- **`fmt` formatting verbs** — the essentials:
  - `%v` — default format of any value; `%+v` adds struct field names; `%#v` Go-syntax representation.
  - `%s` string, `%q` quoted string, `%d` integer, `%f`/`%.2f` float (with precision), `%t` bool, `%T` the type, `%p` pointer.
  - `%w` — wrap an error (lesson 12).
  - `Sprintf` returns a string; `Printf` writes to stdout; `Fprintf` writes to any `io.Writer`.
- **Raw string literals** — backticks `` `...` `` keep text verbatim (no escapes, multi-line) — handy for regexes, JSON, SQL, paths.

## Exercises
1. Print `len("héllo")` and explain why it isn't 5. Then print `len([]rune("héllo"))` to get the character count.
2. `range` over a multibyte string printing the byte index and the rune; note the index jumps.
3. Convert `"42"` to an int with `strconv.Atoi` (handle the error), add 8, convert back with `Itoa`.
4. Show the `string(65)` vs `strconv.Itoa(65)` difference and explain it.
5. Use `strings` functions to: lowercase a sentence, split it into words with `Fields`, and rejoin with `Join`.
6. Build a comma-separated list two ways — with `+=` and with `strings.Builder` — and discuss why `Builder` is preferred for large inputs.
7. Use `fmt.Sprintf` with `%v`, `%+v`, `%#v`, `%q`, and `%.2f` on a small struct and a float; compare the outputs.

## Best Practices & Pitfalls
- **Think in bytes for storage/I/O, runes for human characters.** Choose the right view before indexing.
- **Pitfall — indexing a string gives a byte.** Don't assume `s[i]` is a character unless the text is ASCII.
- **Pitfall — `string(intValue)`** produces the Unicode character for that code point, not the number's text. Use `strconv` for number→text.
- **Use `strings.Builder` (or `bytes.Buffer`) for loops** that assemble strings; avoid `+=` accumulation in hot paths.
- **Always handle `strconv` errors.** Parsing user/network input is exactly where bad data shows up.
- **Use `%q` when logging strings** — it quotes and escapes, making whitespace and empty values visible.
- **Prefer `%w` (not `%v`) when an error contains another error** so callers can unwrap it (lesson 12).

## Checklist
- [ ] I can explain why `len` of a string is bytes, not characters.
- [ ] I know the difference between `byte`, `rune`, and a string index.
- [ ] I can convert between `string`, `[]byte`, and `[]rune`.
- [ ] I use `strconv` for number⇄string with error handling.
- [ ] I can pick the right `fmt` verb for a given value.
- [ ] I use `strings.Builder` for efficient concatenation.

## Resources
- Blog — Strings, bytes, runes and characters: https://go.dev/blog/strings
- `strings` package: https://pkg.go.dev/strings
- `strconv` package: https://pkg.go.dev/strconv
- `fmt` package (all verbs): https://pkg.go.dev/fmt
