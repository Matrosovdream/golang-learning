# Progress

> Update this file as you finish each step. Claude will read it to know where to pick up.

## Current step

**→ 11 — Interfaces** (next up)

## Status by step

| #  | Step                                  | Status      | Notes |
|----|---------------------------------------|-------------|-------|
| 01 | Introduction to Go                    | done        | covered philosophy, why the boilerplate (package main/import/func main) |
| 02 | Environment Setup                     | done        | Go 1.26.3; module `practice/` = github.com/Matrosovdream/golang-learning/practice; per-lesson subfolders |
| 03 | First Program & Basic Syntax          | done        | 4 var forms, zero value, Println vs Printf, iota; saw unused import/var errors |
| 04 | Variables, Types & Constants          | done        | basic types & zero values, no implicit conversion (T(v)), untyped vs typed consts, iota enums, named types (Celsius/Fahrenheit) |
| 05 | Control Flow                          | done        | if-init + early return, all 3 for forms + range (incl. `range 5`), tagless switch (FizzBuzz), labeled break, defer LIFO + args-evaluated-immediately gotcha |
| 06 | Functions                             | done        | multi-return (T, error), variadic + slice spread, closures w/ independent state, funcs as values, defer+recover→named err, Go 1.22+ loop-var capture (prints 0 1 2) |
| 07 | Arrays, Slices & Maps                 | done        | slice len/cap + growth (doubling), aliasing trap, copy detaches, remove-by-index, word-freq map, comma-ok, nil-map panic+recover, set via map[T]struct{}; `go vet` catches Printf verb/arg mismatch |
| 08 | Strings, Runes, Bytes & Formatting    | done        | len=bytes vs []rune=chars, range decodes UTF-8 (index jumps), strconv Atoi/Itoa, string(rune(65))="A" vs Itoa="65", strings.Fields/Join, += vs strings.Builder (O(n²) vs O(n)), %v/%+v/%#v/%q/%.2f verbs. NOTE: used "hello" not "héllo" so multibyte demo was muted |
| 09 | Structs                               | done        | base: keyed/zero/&literal, value-copy vs pointer mutation, embedding (promoted field+method), json tags+omitempty, anon struct, comparability (slice field breaks ==). Extended drills (09-structs/02-07): New() constructor+validation, nested named field, ⚠️slice range-copy gotcha (mutate via items[i]), ⚠️map values non-addressable (copy-store or map[K]*V), embedding shadowing + interface satisfaction, JSON Marshal/Unmarshal + json:"-" + nested |
| 10 | Pointers & Methods                    | done        | base: &/*, value vs pointer receiver, auto-address/deref, nil-receiver guard, Celsius Stringer, linked-list Sum. Drills: method sets T vs *T (interface satisfaction), map-elem-not-addressable vs slice-elem-addressable, new(T). Deep pointer practice (4 progs): **int/swap, return-ptr-to-local=escape analysis, range copy vs index vs []*T, map[K]*V, *[N]T, optional *bool/*int JSON fields, **Node sorted insert |
| 11 | Interfaces                            | in progress | examples library: `examples/11-interfaces/README.md` (25 graded examples easy→hard, each verified). Per-lesson tracker: `examples/11-interfaces/PROGRESS.md`. Working through them now. |
| 12 | Errors & Error Handling               | not started |       |
| 13 | Goroutines                            | not started |       |
| 14 | Channels                              | not started |       |
| 15 | Sync, Context & Patterns              | not started |       |
| 16 | Packages & Modules                    | not started |       |
| 17 | Generics                              | not started |       |
| 18 | Testing & Benchmarking                | not started |       |
| 19 | Standard Library Tour for Backend     | not started |       |
| 20 | HTTP Server Fundamentals              | not started |       |
| 21 | Building a JSON REST API              | not started |       |
| 22 | Persistence with database/sql         | not started |       |
| 23 | Config, Logging & Observability       | not started |       |
| 24 | Idiomatic Go & Effective Go           | not started |       |
| 25 | Project Layout & Clean Architecture   | not started |       |
| 26 | Capstone Project: REST API Service    | not started |       |

**Status legend:** `not started` · `in progress` · `done`

## Log

<!-- Append a short line each session, newest at the bottom. Example:
- 2026-05-28 — finished step 01, took notes on Go vs other languages
-->

- 2026-05-28 — learning plan created (26 steps, backend/REST-API focus). Start with step 01.
- 2026-05-29 — finished steps 01 & 02. Set up `practice/` Go module (per-lesson subfolders), verified `go run`/`fmt`/`vet`. Next: step 03.
- 2026-05-29 — finished steps 03 & 04 (first program, syntax, types/constants/iota). Saw unused-import/var and mismatched-types compile errors on purpose. Next: step 05.
- 2026-06-01 — finished step 05 (control flow): if-init, for forms + range, tagless switch FizzBuzz, labeled break, defer LIFO/arg-eval. Note: module root is `practice/`, so run from inside it (`cd practice && go run ./05-control-flow`). Next: step 06.
- 2026-06-01 — finished step 06 (functions): multi-return/(T,error), variadic + spread, closures, funcs as values, defer+recover→named err, loop-var capture. Next: step 07.
- 2026-06-02 — finished step 07 (slices & maps): cap growth/doubling, aliasing trap, copy, remove-by-index, word-freq map + comma-ok, nil-map panic+recover, set via map[T]struct{}. Learned `go vet` catches Printf verb/arg mismatches. Next: step 08.
- 2026-06-02 — finished step 08 (strings): bytes-vs-runes, range/UTF-8, strconv, string(rune(n)) vs Itoa, strings.Builder, fmt verbs. Moved fast — used ASCII so the multibyte len split was muted; reinforced go vet for Printf. Next: step 09.
- 2026-06-02 — finished step 09 (structs) + 6 extended drills (09-structs/02-07): constructors/validation, nested structs, slice range-copy gotcha, map non-addressability, advanced embedding (shadowing + interface), JSON round-trip. Going deeper per request. Next: step 10.
- 2026-06-03 — finished step 10 (pointers & methods): base lesson (&/*, value vs pointer receiver, nil receiver, Celsius Stringer, linked-list Sum) + drills (method sets T vs *T, map vs slice addressability, new) + extra deep pointer practice in 4 progs under 10-pointers-methods/{pointers-basics,pointers-funcs,pointers-collections,pointers-patterns}: **int/swap, return-ptr-to-local (escape analysis), range-copy vs index vs []*T, map[K]*V, *[N]T, optional *bool/*int JSON fields, **Node sorted insert. Next: step 11 (interfaces).
- 2026-06-05 — started step 11 (interfaces). Began the lesson (Shape/Stringer/type-switch/typed-nil) directly in `practice/11-interfaces`, then created a NEW examples library: `learning-plan/examples/<lesson>/README.md`. Built `11-interfaces/README.md` with 25 graded examples easy→hard (implicit satisfaction, polymorphism, (type,value) pair, Stringer, any, assertions, type switch, method sets, composition, error iface, sort.Slice/sort.Interface, io.Writer/MultiWriter, iface-to-iface assertion, strategy map, accept-iface/return-struct, typed-nil trap, uncomparable panic, optional ifaces, decorator/middleware, recursive any walker, DI test fake, capstone plugin system). Each example verified (gofmt/vet/run); workflow used to author+verify. Process: I dictate code blocks, user retypes & runs. "add more examples" → append starting at #26. Next: user works through the examples.
