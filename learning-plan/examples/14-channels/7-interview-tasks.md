# Step 14 — Channels · 🛠️ Interview: implement it

Examples **94–112**: coding tasks as interviewers actually phrase them. Alternating printers, the LeetCode concurrency set (1114, 1116, 1117, 1195, 1226) solved with channels, the Tour of Go exercises, classic problems (prime sieve, sleeping barber, dining philosophers), and practical designs (bounded queue, scatter-gather, crawler, TTL cache, a leak-free pipeline).

**How to use this part:** read the **Task** and **Rules**, and write your own solution first (set a timer: 15 minutes for 🟢/🟡, 30 for 🔴). Open **Hint** only if you're stuck. Then compare with **Solution**, which has real output and notes on the design choices interviewers probe. If your answer differs but meets the rules, it's probably fine: most of these have several good solutions.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run -race .
```

> ← Back to the [index](README.md) · Progress tracker: [PROGRESS.md](PROGRESS.md) · Prev: [🎯 interview quiz](6-interview-quiz.md)

---

## 94. Ping-pong, N times

`🟢 easy` · *Interview task*

**Task:** start two goroutines, `ping` and `pong`. Together they print `ping`, `pong`, `ping`, `pong`… for 6 lines in total, strictly alternating, and then the program exits.

**Rules:** channels only: no mutex, no `time.Sleep`, no shared counters.

<details>
<summary><b>Hint</b></summary>

Pass a "ball" back and forth on two unbuffered channels. Only the goroutine holding the ball may print. Put the number of hits left *in* the ball, so whoever makes the last hit knows to end the game.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import "fmt"

// player hits the ball back until the rally runs out of hits.
func player(name string, in <-chan int, out chan<- int, done chan<- struct{}) {
	for hits := range in {
		fmt.Println(name)
		if hits == 1 { // that was the last hit
			close(done)
			return
		}
		out <- hits - 1
	}
}

func main() {
	const hits = 6
	toPing := make(chan int)
	toPong := make(chan int)
	done := make(chan struct{})

	go player("ping", toPing, toPong, done)
	go player("pong", toPong, toPing, done)

	toPing <- hits // serve
	<-done
}
```

**Output:**

```
ping
pong
ping
pong
ping
pong
```

Why it's strictly ordered: a player prints only after **receiving** the ball, and the other player is blocked on its own receive until the ball arrives. There's no moment when both may print. The ball carries the state (hits left), so no counter is shared. `done` lets `main` wait without sleeping.

Follow-up an interviewer might ask: "the goroutine that didn't make the last hit is still parked on `range in` when `main` exits. Is that a leak?" In a program that exits, no. In a long-running service, close both channels after `<-done` so both players return.

</details>

---

## 95. Two goroutines print 1..N in order (odd and even)

`🟢 easy` · *Interview task*

**Task:** one goroutine prints only odd numbers and another prints only even numbers. The combined output must be `1, 2, 3, … N` in order. It must work for both even and odd `N`, and exit cleanly with no deadlock.

**Rules:** each goroutine owns its own sequence (`1, 3, 5…` and `2, 4, 6…`). They coordinate only through channels.

<details>
<summary><b>Hint</b></summary>

Use two `chan struct{}` "your turn" signals, one per goroutine. Wait for your turn, print, pass the turn on. The tricky part is the **end**: the goroutine that prints `N` must **not** pass the turn, because nobody is left to take it.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	const n = 10
	oddTurn := make(chan struct{})
	evenTurn := make(chan struct{})
	var wg sync.WaitGroup

	wg.Go(func() {
		for i := 1; i <= n; i += 2 {
			<-oddTurn // wait for my turn
			fmt.Println("odd: ", i)
			if i < n {
				evenTurn <- struct{}{} // hand the turn over
			}
		}
	})
	wg.Go(func() {
		for i := 2; i <= n; i += 2 {
			<-evenTurn
			fmt.Println("even:", i)
			if i < n {
				oddTurn <- struct{}{}
			}
		}
	})

	oddTurn <- struct{}{} // odd goes first
	wg.Wait()
}
```

**Output:**

```
odd:  1
even: 2
odd:  3
even: 4
odd:  5
even: 6
odd:  7
even: 8
odd:  9
even: 10
```

Unlike example 94, no data travels through the channels, only **permission**. That's why they're `chan struct{}`. The `if i < n` guard on **both** sides is the part most people miss. Without it, the goroutine that prints `N` sends a turn nobody will receive, and the program deadlocks (for even `N` it's the even goroutine, for odd `N` the odd one). Try `n = 9`. `wg.Go` (Go 1.25+) is `Add(1)` + `go` + `defer Done()` in one call.

</details>

---

## 96. N goroutines take turns in a ring: cat dog fish × 3

`🟡 medium` · *Interview task*

**Task:** three goroutines print `cat`, `dog` and `fish` respectively. The output must be `cat dog fish` on a line, three times, always in that order. Then generalize: adding a fourth name must not require any new code.

**Rules:** one goroutine per name; channels only; the program exits when done.

<details>
<summary><b>Hint</b></summary>

Put the goroutines in a **ring**: goroutine `i` waits on `turns[i]` and, after printing, passes the token to `turns[(i+1) % N]`. Let the token carry the round number, and let the last goroutine in the ring decide when the final round is over.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import "fmt"

func main() {
	names := []string{"cat", "dog", "fish"}
	const rounds = 3
	last := len(names) - 1

	turns := make([]chan int, len(names)) // turns[i] wakes goroutine i
	for i := range turns {
		turns[i] = make(chan int)
	}
	done := make(chan struct{})

	for i, name := range names {
		go func() {
			next := turns[(i+1)%len(names)]
			for round := range turns[i] {
				if i < last {
					fmt.Print(name, " ")
					next <- round
					continue
				}
				fmt.Println(name) // the last one ends the line and the round
				if round == rounds {
					close(done)
					return
				}
				next <- round + 1
			}
		}()
	}

	turns[0] <- 1
	<-done
}
```

**Output:**

```
cat dog fish
cat dog fish
cat dog fish
```

The token ring generalizes examples 94 and 95 from two to N. At any moment exactly one goroutine holds the token, so the prints can't interleave. The last goroutine checks the round and closes `done`. The others stay parked on their `turns[i]` when `main` exits, which is fine for a program that ends. Common mistake: using one shared channel for all goroutines. A value sent on it goes to **any** waiting receiver, not "the next in line", so the order is lost.

</details>

---

## 97. Print in Order (LeetCode 1114)

`🟡 medium` · *Interview task*

**Task:** implement `Foo` with methods `First(print)`, `Second(print)` and `Third(print)`. Three goroutines call them **in any order**, and the output must always be `firstsecondthird`.

**Rules:** don't change the callers. Coordinate inside `Foo` using channels. Show that it works for several start orders.

<details>
<summary><b>Hint</b></summary>

`Second` must wait until `First` has printed, and `Third` must wait for `Second`. A channel that gets **closed** is a perfect "it has happened" flag. Receiving from it blocks until the close, and returns immediately forever after, so it doesn't matter whether the waiter arrives before or after the event.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"strings"
	"sync"
)

type Foo struct {
	firstDone  chan struct{}
	secondDone chan struct{}
}

func NewFoo() *Foo {
	return &Foo{firstDone: make(chan struct{}), secondDone: make(chan struct{})}
}

func (f *Foo) First(print func()) {
	print()
	close(f.firstDone) // wakes Second, whenever it arrives
}

func (f *Foo) Second(print func()) {
	<-f.firstDone
	print()
	close(f.secondDone)
}

func (f *Foo) Third(print func()) {
	<-f.secondDone
	print()
}

func main() {
	for _, order := range [][]int{{1, 2, 3}, {3, 2, 1}, {2, 3, 1}} {
		f := NewFoo()
		var out strings.Builder
		calls := map[int]func(){
			1: func() { f.First(func() { out.WriteString("first") }) },
			2: func() { f.Second(func() { out.WriteString("second") }) },
			3: func() { f.Third(func() { out.WriteString("third") }) },
		}

		var wg sync.WaitGroup
		for _, n := range order {
			wg.Go(calls[n]) // each call on its own goroutine, started in this order
		}
		wg.Wait()
		fmt.Printf("started %v -> %s\n", order, out.String())
	}
}
```

**Output:**

```
started [1 2 3] -> firstsecondthird
started [3 2 1] -> firstsecondthird
started [2 3 1] -> firstsecondthird
```

`close` beats sending a value here. A send needs the waiter to be there (or a buffer) and wakes exactly **one** receiver. A close is remembered: `Second` can arrive a second later and still pass straight through. `-race` stays quiet even though three goroutines write to one `strings.Builder`, because the channel operations put every write strictly after the previous one.

</details>

---

## 98. Print Zero Even Odd (LeetCode 1116)

`🟡 medium` · *Interview task*

**Task:** implement `ZeroEvenOdd` for a given `n`, with methods `Zero(print)`, `Even(print)` and `Odd(print)`, each run by its own goroutine. Together they must print `0 1 0 2 0 3 … 0 n` with no separators: `n = 5` gives `0102030405`.

**Rules:** each method only prints its own kind of number; channels only; no deadlock for even or odd `n`.

<details>
<summary><b>Hint</b></summary>

Make `Zero` the **conductor**. It prints `0`, sends a turn to `odd` or `even` depending on the next number, then waits for an acknowledgement before its next `0`. The printers wait for their turn, print, and acknowledge. Because every turn is acknowledged, the last number needs no special case.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"strings"
	"sync"
)

type ZeroEvenOdd struct {
	n               int
	zero, even, odd chan struct{}
}

func New(n int) *ZeroEvenOdd {
	return &ZeroEvenOdd{
		n:    n,
		zero: make(chan struct{}),
		even: make(chan struct{}),
		odd:  make(chan struct{}),
	}
}

// Zero prints 0 before every number and decides who goes next.
func (z *ZeroEvenOdd) Zero(printNumber func(int)) {
	for i := 1; i <= z.n; i++ {
		printNumber(0)
		if i%2 == 1 {
			z.odd <- struct{}{}
		} else {
			z.even <- struct{}{}
		}
		<-z.zero // wait until that number has been printed
	}
}

func (z *ZeroEvenOdd) Odd(printNumber func(int)) {
	for i := 1; i <= z.n; i += 2 {
		<-z.odd
		printNumber(i)
		z.zero <- struct{}{}
	}
}

func (z *ZeroEvenOdd) Even(printNumber func(int)) {
	for i := 2; i <= z.n; i += 2 {
		<-z.even
		printNumber(i)
		z.zero <- struct{}{}
	}
}

func main() {
	for _, n := range []int{5, 6} {
		z := New(n)
		var out strings.Builder
		printNumber := func(x int) { fmt.Fprint(&out, x) }

		var wg sync.WaitGroup
		wg.Go(func() { z.Odd(printNumber) })
		wg.Go(func() { z.Even(printNumber) })
		wg.Go(func() { z.Zero(printNumber) })
		wg.Wait()
		fmt.Printf("n=%d: %s\n", n, out.String())
	}
}
```

**Output:**

```
n=5: 0102030405
n=6: 010203040506
```

The acknowledgement (`<-z.zero`) is what makes this correct. Without it, `Zero` could print its next `0` before the odd or even goroutine printed its number. Compare with example 95, which needed an `if i < n` guard: here the conductor always waits for exactly as many acknowledgements as turns it gave out, so the counts always match.

</details>

---

## 99. Fizz Buzz Multithreaded (LeetCode 1195)

`🟡 medium` · *Interview task*

**Task:** implement `FizzBuzz` for `n` with four methods, each run by its own goroutine. `Fizz` prints `fizz` (divisible by 3 only), `Buzz` prints `buzz` (by 5 only), `FizzBuzz` prints `fizzbuzz` (by 15), and `Number` prints everything else. The combined output for `1..n` must be in order.

**Rules:** each goroutine prints only its own kind of output; channels only; all four goroutines must **return** at the end (no parked leftovers).

<details>
<summary><b>Hint</b></summary>

One goroutine has to know the current `i`. Make `Number` the conductor: for each `i` it either prints the number itself, or sends `i` to the right worker and waits for an acknowledgement. When the loop ends, **closing** the three worker channels makes their `for range` loops return. That covers the "all four must return" rule.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"strings"
	"sync"
)

type FizzBuzz struct {
	n                    int
	fizz, buzz, fizzbuzz chan int
	ack                  chan struct{}
}

func New(n int) *FizzBuzz {
	return &FizzBuzz{n: n, fizz: make(chan int), buzz: make(chan int),
		fizzbuzz: make(chan int), ack: make(chan struct{})}
}

func (f *FizzBuzz) Fizz(printFizz func()) {
	for range f.fizz { // ends when Number closes the channel
		printFizz()
		f.ack <- struct{}{}
	}
}

func (f *FizzBuzz) Buzz(printBuzz func()) {
	for range f.buzz {
		printBuzz()
		f.ack <- struct{}{}
	}
}

func (f *FizzBuzz) FizzBuzz(printFizzBuzz func()) {
	for range f.fizzbuzz {
		printFizzBuzz()
		f.ack <- struct{}{}
	}
}

// Number conducts: it prints plain numbers itself and hands the rest out.
func (f *FizzBuzz) Number(printNumber func(int)) {
	defer close(f.fizz)
	defer close(f.buzz)
	defer close(f.fizzbuzz)
	for i := 1; i <= f.n; i++ {
		var to chan int
		switch {
		case i%15 == 0:
			to = f.fizzbuzz
		case i%3 == 0:
			to = f.fizz
		case i%5 == 0:
			to = f.buzz
		default:
			printNumber(i)
			continue
		}
		to <- i // wake the right goroutine...
		<-f.ack // ...and wait until it has printed
	}
}

func main() {
	fb := New(15)
	var words []string // every append is sequenced by a channel handoff
	var wg sync.WaitGroup
	wg.Go(func() { fb.Fizz(func() { words = append(words, "fizz") }) })
	wg.Go(func() { fb.Buzz(func() { words = append(words, "buzz") }) })
	wg.Go(func() { fb.FizzBuzz(func() { words = append(words, "fizzbuzz") }) })
	wg.Go(func() { fb.Number(func(i int) { words = append(words, fmt.Sprint(i)) }) })
	wg.Wait()
	fmt.Println(strings.Join(words, " "))
}
```

**Output:**

```
1 2 fizz 4 buzz fizz 7 8 fizz buzz 11 fizz 13 14 fizzbuzz
```

This extends the conductor-plus-acknowledgement idea of example 98 with one addition: **shutdown by close**. The worker loops are `for range ch`, so the conductor's deferred `close` calls end all three cleanly, and `wg.Wait()` returns only because every goroutine really finished. That's the answer to the follow-up "how do the other threads know it's over?"

</details>

---

## 100. Building H2O (LeetCode 1117)

`🟡 medium` · *Interview task*

**Task:** hydrogen and oxygen atoms arrive as goroutines in any order. Each calls `Hydrogen(release)` or `Oxygen(release)`. Atoms must be released in groups: in the output, **every 3 consecutive** releases are exactly 2 H and 1 O. An atom that can't join a molecule yet has to wait.

**Rules:** channels only; any arrival order must work; molecules must not interleave.

<details>
<summary><b>Hint</b></summary>

Use a small **builder** goroutine that admits atoms: an atom sends on `hSeat` or `oSeat` and is let in when the builder receives. The builder takes exactly 2 hydrogens and 1 oxygen, then waits for all three to report "bonded" before admitting anyone for the next molecule. That last wait is what keeps molecules from interleaving.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"slices"
	"sync"
)

type H2O struct {
	hSeat, oSeat chan struct{} // a send is admitted only while the molecule has room
	bonded       chan struct{} // each atom reports after it has printed
}

// NewH2O builds `molecules` molecules, one at a time.
func NewH2O(molecules int) *H2O {
	w := &H2O{hSeat: make(chan struct{}), oSeat: make(chan struct{}), bonded: make(chan struct{})}
	go func() {
		for range molecules {
			<-w.hSeat // admit exactly two hydrogens...
			<-w.hSeat
			<-w.oSeat // ...and one oxygen
			for range 3 {
				<-w.bonded // wait until all three have printed
			}
		}
	}()
	return w
}

func (w *H2O) Hydrogen(releaseHydrogen func()) {
	w.hSeat <- struct{}{} // wait for a seat in the current molecule
	releaseHydrogen()
	w.bonded <- struct{}{}
}

func (w *H2O) Oxygen(releaseOxygen func()) {
	w.oSeat <- struct{}{}
	releaseOxygen()
	w.bonded <- struct{}{}
}

func main() {
	arrivals := "OOHHHHOHH" // 6 H and 3 O, in an awkward order
	w := NewH2O(len(arrivals) / 3)
	out := make(chan byte, len(arrivals))

	var wg sync.WaitGroup
	for _, atom := range arrivals {
		if atom == 'H' {
			wg.Go(func() { w.Hydrogen(func() { out <- 'H' }) })
		} else {
			wg.Go(func() { w.Oxygen(func() { out <- 'O' }) })
		}
	}
	wg.Wait()
	close(out)

	var printed []byte
	for b := range out {
		printed = append(printed, b)
	}
	ok := true
	var molecules []string
	for group := range slices.Chunk(printed, 3) {
		slices.Sort(group) // H/O order inside a molecule may vary
		molecules = append(molecules, string(group))
		ok = ok && string(group) == "HHO"
	}
	fmt.Println("molecules:", molecules)
	fmt.Println("every group of 3 is 2 H + 1 O:", ok)
}
```

**Output:**

```
molecules: [HHO HHO HHO]
every group of 3 is 2 H + 1 O: true
```

The builder is a gate: an unbuffered send is admitted only when the builder is ready to receive it, so it controls exactly who's inside the current molecule. A common wrong answer uses two semaphores, `make(chan struct{}, 2)` for H and `make(chan struct{}, 1)` for O. Those limit **how many** atoms print at once, but without the "wait until all three bonded" step, atoms from the next molecule can print before the current one is complete.

</details>

---

## 101. Concurrent prime sieve (a chain of filter goroutines)

`🟡 medium` · *Interview task*

**Task:** print the first 10 primes using only goroutines and channels, with no division checks against a list. A generator emits `2, 3, 4, …`. Every time a prime is found, a new **filter goroutine** is added to the end of a pipeline and removes that prime's multiples.

**Rules:** each filter knows only its own prime; the pipeline grows as primes are discovered.

<details>
<summary><b>Hint</b></summary>

Keep a variable `ch` that always points at the **end** of the chain. Whatever arrives there has passed every filter so far, so it's prime. Start `filter(ch, next, prime)`, then set `ch = next`.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"runtime"
)

// generate sends 2, 3, 4, ... forever.
func generate(out chan<- int) {
	for i := 2; ; i++ {
		out <- i
	}
}

// filter passes on only the numbers not divisible by prime.
func filter(in <-chan int, out chan<- int, prime int) {
	for {
		if n := <-in; n%prime != 0 {
			out <- n
		}
	}
}

func main() {
	ch := make(chan int)
	go generate(ch)

	var primes []int
	for range 10 {
		prime := <-ch // the first number to survive every filter is prime
		primes = append(primes, prime)
		next := make(chan int)
		go filter(ch, next, prime) // a new filter joins the end of the chain
		ch = next
	}
	fmt.Println("first 10 primes:", primes)
	fmt.Println("goroutines in the chain (main + generator + filters):", runtime.NumGoroutine())
}
```

**Output:**

```
first 10 primes: [2 3 5 7 11 13 17 19 23 29]
goroutines in the chain (main + generator + filters): 12
```

This is the famous sieve from the Go documentation (`go.dev/doc/play/sieve.go`), a daisy chain where each goroutine is a tiny stage. It's elegant, not efficient: every number crosses up to one channel per prime so far. Good follow-ups: "how would you stop all 11 goroutines?" (pass a `done` channel into `generate` and every `filter`, example 31), and "why is `ch = next` safe?" (only `main` touches `ch`; the filters got their channels as arguments).

</details>

---

## 102. Equivalent Binary Trees (Tour of Go)

`🟡 medium` · *Interview task*

**Task:** two binary search trees can have different shapes but hold the same values. Write `Walk(t, ch)`, which sends a tree's values to a channel in sorted order, and `Same(t1, t2)`, which walks both trees **at the same time** and reports whether they hold the same sequence.

**Rules:** don't collect either tree into a slice first; compare value by value. Improve on the Tour's version: when `Same` finds a mismatch early, both walker goroutines must still exit.

<details>
<summary><b>Hint</b></summary>

An in-order traversal (left, node, right) of a BST yields sorted values. Run one `Walk` per tree, receive from both channels in lockstep, and compare `(value, ok)` pairs. For early exit, give `Walk` a `done` channel and `select` every send against it. `defer close(done)` in `Same` covers every return.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"runtime"
	"time"
)

type Tree struct {
	Left  *Tree
	Value int
	Right *Tree
}

func insert(t *Tree, v int) *Tree {
	if t == nil {
		return &Tree{Value: v}
	}
	if v < t.Value {
		t.Left = insert(t.Left, v)
	} else {
		t.Right = insert(t.Right, v)
	}
	return t
}

// build makes a BST holding k, 2k, ..., 10k; the order sets the shape.
func build(k int, order []int) *Tree {
	var t *Tree
	for _, i := range order {
		t = insert(t, i*k)
	}
	return t
}

// Walk sends the tree's values in sorted order, then closes ch.
// It stops early (and still closes ch) once done is closed.
func Walk(t *Tree, ch chan<- int, done <-chan struct{}) {
	defer close(ch)
	var walk func(t *Tree) bool
	walk = func(t *Tree) bool {
		if t == nil {
			return true
		}
		if !walk(t.Left) {
			return false
		}
		select {
		case ch <- t.Value:
		case <-done:
			return false
		}
		return walk(t.Right)
	}
	walk(t)
}

// Same reports whether two trees hold the same values.
func Same(t1, t2 *Tree) bool {
	done := make(chan struct{})
	defer close(done) // a mismatch returns early: stop both walkers
	c1, c2 := make(chan int), make(chan int)
	go Walk(t1, c1, done)
	go Walk(t2, c2, done)
	for {
		v1, ok1 := <-c1
		v2, ok2 := <-c2
		if ok1 != ok2 || v1 != v2 {
			return false
		}
		if !ok1 {
			return true
		}
	}
}

func main() {
	bushy := build(1, []int{7, 3, 9, 1, 5, 8, 10, 2, 4, 6})
	chain := build(1, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) // every node has only a right child
	doubled := build(2, []int{5, 2, 8, 1, 3, 7, 9, 4, 6, 10})

	ch := make(chan int)
	go Walk(bushy, ch, nil) // a nil done never fires: walk everything
	var values []int
	for v := range ch {
		values = append(values, v)
	}
	fmt.Println("walk:", values)
	fmt.Println("Same(bushy, chain):  ", Same(bushy, chain))
	fmt.Println("Same(bushy, doubled):", Same(bushy, doubled))

	time.Sleep(10 * time.Millisecond)
	fmt.Println("goroutines left after an early mismatch:", runtime.NumGoroutine())
}
```

**Output:**

```
walk: [1 2 3 4 5 6 7 8 9 10]
Same(bushy, chain):   true
Same(bushy, doubled): false
goroutines left after an early mismatch: 1
```

This is the Tour of Go's concurrency exercise (it uses `golang.org/x/tour/tree`; here a tiny local BST keeps it stdlib-only). Passing `nil` as `done` for a full walk is a neat use of example 41's table: a nil channel is never ready, so that `select` case simply never fires. Without `done`, the first mismatch would leave both walkers parked mid-tree forever (example 91).

</details>

---

## 103. Merge two sorted channels into one sorted stream

`🟡 medium` · *Interview task*

**Task:** write `mergeSorted(a, b <-chan int) <-chan int`. Both inputs deliver values in ascending order and may have different lengths, or be empty. The output must deliver every value, in ascending order, and close when both inputs are exhausted.

**Rules:** stream it: hold at most one pending value per input, never a whole slice.

<details>
<summary><b>Hint</b></summary>

It's the merge step of merge sort, over channels. Keep the **next** value of each input in a variable, along with its `ok`. Send the smaller one and refill only that side. When one side is closed (`ok == false`), keep draining the other.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import "fmt"

func gen(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for _, n := range nums {
			out <- n
		}
	}()
	return out
}

// mergeSorted merges two sorted streams into one sorted stream.
func mergeSorted(a, b <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		va, okA := <-a // always hold the next unsent value of each side
		vb, okB := <-b
		for okA || okB {
			if okA && (!okB || va <= vb) { // <= keeps equal values stable: a first
				out <- va
				va, okA = <-a
			} else {
				out <- vb
				vb, okB = <-b
			}
		}
	}()
	return out
}

func collect(ch <-chan int) []int {
	var got []int
	for v := range ch {
		got = append(got, v)
	}
	return got
}

func main() {
	fmt.Println(collect(mergeSorted(gen(1, 4, 7, 10), gen(2, 3, 8, 9, 11, 12))))
	fmt.Println(collect(mergeSorted(gen(5, 5, 6), gen(5))))
	fmt.Println(collect(mergeSorted(gen(), gen(1, 2))))
}
```

**Output:**

```
[1 2 3 4 7 8 9 10 11 12]
[5 5 5 6]
[1 2]
```

The subtle part is **not** selecting on both inputs. `select` would take whichever value arrives first, which isn't the smaller one. You have to wait for **both** heads before choosing. `va <= vb` makes the merge stable (ties come from `a` first). Follow-up: "merge **K** sorted channels" keeps one head per input in a min-heap (lesson 52), taking O(log K) per value.

</details>

---

## 104. Sleeping Barber: the waiting room is a buffered channel

`🟡 medium` · *Interview task*

**Task:** model the classic *sleeping barber*. The shop has one barber and 3 waiting chairs. An arriving customer takes a free chair, or **leaves** if all chairs are taken. The barber cuts hair one customer at a time, in arrival order, and **sleeps** (without busy-waiting) when nobody is waiting. At closing time no new customers enter, the barber finishes everyone already seated, and goes home.

**Rules:** no mutex, no counters of free chairs, no polling loop.

<details>
<summary><b>Hint</b></summary>

Each rule maps onto one channel feature. The chairs are a buffered channel with `cap` 3. "Leave if full" is a `select` with `default`. "Sleep when empty" is a plain receive, which blocks. "Closing time, but finish the room" is `close` plus `range`, because `range` still drains buffered values after a close.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"time"
)

func main() {
	waiting := make(chan int, 3) // the waiting room: 3 chairs
	served := make(chan int, 10)
	barberHome := make(chan struct{})
	earlierCutDone := make(chan struct{}) // the barber is busy when the shop opens

	go func() { // the barber
		defer close(barberHome)
		<-earlierCutDone
		for c := range waiting { // an empty room means the barber sleeps here
			time.Sleep(5 * time.Millisecond) // cutting hair
			served <- c
		}
	}()

	arrive := func(c int) {
		select {
		case waiting <- c:
			fmt.Println("customer", c, "sits down")
		default:
			fmt.Println("customer", c, "finds no free chair and leaves")
		}
	}

	for c := 1; c <= 5; c++ {
		arrive(c)
	}
	close(earlierCutDone)
	time.Sleep(50 * time.Millisecond) // the barber empties the room and falls asleep
	arrive(6)

	close(waiting) // closing time: nobody new comes in
	<-barberHome   // the barber finishes the room, then goes home
	close(served)
	var cuts []int
	for c := range served {
		cuts = append(cuts, c)
	}
	fmt.Println("haircuts, in order:", cuts)
}
```

**Output:**

```
customer 1 sits down
customer 2 sits down
customer 3 sits down
customer 4 finds no free chair and leaves
customer 5 finds no free chair and leaves
customer 6 sits down
haircuts, in order: [1 2 3 6]
```

The whole concurrency problem fits in one channel's semantics, which is why interviewers like it: the classic solution in other languages needs a mutex, a counter and two semaphores. The `earlierCutDone` gate only makes the demo deterministic (the barber is busy while the first five arrive). Follow-ups: "several barbers?" (start several `range waiting` goroutines), and "how many customers were turned away?" (count in the `default` branch, owned by the arriving side).

</details>

---

## 105. A WaitGroup built only from channels

`🟡 medium` · *Interview task*

**Task:** implement your own `WaitGroup` with `Add(n)`, `Done()` and `Wait()`, **without** the `sync` package. `Wait` may be called from several goroutines at once, and all of them must wake when the count reaches zero. `Wait` with a count of 0 returns immediately, and a negative count panics.

**Rules:** channels only; no goroutine that lives inside the `WaitGroup`, so nothing can leak.

<details>
<summary><b>Hint</b></summary>

You need two things. First, a **lock** around the counter: a 1-slot channel holding the state works (receive to take it, send to put it back). Second, a **broadcast** when the count reaches zero: a channel that gets **closed**, because a close wakes every receiver at once. Create a fresh one each time the count leaves zero.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

type wgState struct {
	count int
	zero  chan struct{} // closed when count drops back to 0; nil while count is 0
}

// WaitGroup uses a 1-slot channel as its lock: holding the state = holding the lock.
type WaitGroup struct {
	state chan *wgState
}

func NewWaitGroup() *WaitGroup {
	w := &WaitGroup{state: make(chan *wgState, 1)}
	w.state <- &wgState{}
	return w
}

func (w *WaitGroup) Add(n int) {
	s := <-w.state
	defer func() { w.state <- s }() // always put the state back, even when panicking
	if s.count == 0 && n > 0 {
		s.zero = make(chan struct{}) // a new round of waiting starts
	}
	s.count += n
	if s.count < 0 {
		panic("negative WaitGroup counter")
	}
	if s.count == 0 && s.zero != nil {
		close(s.zero) // wake every Wait at once
		s.zero = nil
	}
}

func (w *WaitGroup) Done() { w.Add(-1) }

func (w *WaitGroup) Wait() {
	s := <-w.state
	zero := s.zero
	w.state <- s
	if zero != nil {
		<-zero
	}
}

func main() {
	wg := NewWaitGroup()
	var finished atomic.Int32
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i+1) * 5 * time.Millisecond)
			finished.Add(1)
		}()
	}

	released := make(chan int32, 2)
	for range 2 { // two independent waiters
		go func() {
			wg.Wait()
			released <- finished.Load()
		}()
	}
	fmt.Println("waiter 1 released, tasks finished:", <-released)
	fmt.Println("waiter 2 released, tasks finished:", <-released)

	wg.Wait()
	fmt.Println("Wait at count 0 returns immediately")

	defer func() { fmt.Println("recovered:", recover()) }()
	wg.Done()
}
```

**Output:**

```
waiter 1 released, tasks finished: 5
waiter 2 released, tasks finished: 5
Wait at count 0 returns immediately
recovered: negative WaitGroup counter
```

Two classic channel ideas in one type: a **1-slot channel as a mutex** (example 61), and **close as a broadcast** (example 15). Details interviewers look for: `Wait` copies `zero` while holding the state and waits **after** giving the state back (waiting while holding it would block every `Done`, a deadlock), and `Add` puts the state back in a `defer` so the panic doesn't leave the lock held forever. The real `sync.WaitGroup` uses atomics and a runtime semaphore instead, which is much faster.

</details>

---

## 106. Map-reduce word count

`🟡 medium` · *Interview task*

**Task:** count word frequencies across several documents in parallel and print the top 5 words (ties broken alphabetically). Use a fixed number of **mapper** goroutines and a single **reducer**.

**Rules:** no mutex and no `sync.Map`: the mappers must never write to a shared map. Output must be deterministic.

<details>
<summary><b>Hint</b></summary>

Each mapper builds its **own** small map for one document and **sends the whole map** to the reducer. After the send, the map belongs to the reducer. The reducer is the only goroutine that touches the total. Close the partials channel with the usual `wg.Wait()` helper, and sort at the end for deterministic output.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// mapper counts words per document and hands each finished map over.
func mapper(docs <-chan string, out chan<- map[string]int) {
	for doc := range docs {
		counts := map[string]int{}
		for _, w := range strings.Fields(doc) {
			counts[w]++
		}
		out <- counts // ownership moves to the reducer: never touched again here
	}
}

func main() {
	documents := []string{
		"the quick brown fox jumps over the lazy dog",
		"the dog barks and the fox runs",
		"a quick brown dog",
		"the end",
	}

	docs := make(chan string, len(documents))
	for _, d := range documents {
		docs <- d
	}
	close(docs)

	partials := make(chan map[string]int)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { mapper(docs, partials) })
	}
	go func() {
		wg.Wait()
		close(partials)
	}()

	total := map[string]int{} // reduce: only this goroutine touches total
	for part := range partials {
		for w, n := range part {
			total[w] += n
		}
	}

	words := slices.SortedFunc(maps.Keys(total), func(a, b string) int {
		return cmp.Or(cmp.Compare(total[b], total[a]), cmp.Compare(a, b))
	})
	for _, w := range words[:5] {
		fmt.Printf("%-6s %d\n", w, total[w])
	}
}
```

**Output:**

```
the    5
dog    3
brown  2
fox    2
quick  2
```

"Share memory by communicating" in its plainest form: instead of locking one shared map, each mapper owns a private map and **hands it over** through a channel, like the ownership transfer in example 43. The reducer is single-threaded, so it needs no lock at all. The sort uses `cmp.Or` to chain "count descending, then word ascending" (more on these helpers in lesson 54).

</details>

---

## 107. A bounded blocking queue whose Close never makes callers panic

`🔴 hard` · *Interview task*

**Task:** implement `Queue` with capacity `N`:

- `Put(v) error` blocks while the queue is full.
- `Get() (v, ok)` blocks while it's empty.
- `Close()` is idempotent. After it, `Put` returns `ErrClosed`, **including Puts that were already blocked**. `Get` still drains the remaining items and then returns `ok == false`.

**Rules:** no call may ever panic or block forever after `Close`, and no mutex. A plain `chan int` is **not** enough: closing it would make blocked senders panic (example 47).

<details>
<summary><b>Hint</b></summary>

Put **one owner goroutine** in charge of a slice, and turn every method into a request to it (example 60). The owner decides what to accept by switching channels off with `nil`: while full, it doesn't receive `puts`; while empty, it doesn't receive `gets`. Once closed **and** drained it exits and closes `dead`, and every method also `select`s on `<-dead`, so late callers get an answer instead of blocking.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"errors"
	"fmt"
	"time"
)

var ErrClosed = errors.New("queue closed")

type putReq struct {
	v     int
	reply chan error
}

type getResp struct {
	v  int
	ok bool
}

// Queue is a bounded blocking FIFO. One owner goroutine holds the items;
// every call is a request to it.
type Queue struct {
	puts   chan putReq
	gets   chan chan getResp
	closes chan struct{}
	dead   chan struct{} // closed when the owner exits (closed and drained)
}

func NewQueue(capacity int) *Queue {
	q := &Queue{puts: make(chan putReq), gets: make(chan chan getResp),
		closes: make(chan struct{}), dead: make(chan struct{})}
	go q.run(capacity)
	return q
}

func (q *Queue) run(capacity int) {
	defer close(q.dead)
	var items []int
	closed := false
	for {
		puts, gets := q.puts, q.gets
		if !closed && len(items) == capacity {
			puts = nil // full: Put callers wait in their send
		}
		if len(items) == 0 {
			if closed {
				return // closed and drained: shut down
			}
			gets = nil // empty: Get callers wait
		}
		select {
		case req := <-puts:
			if closed {
				req.reply <- ErrClosed
				continue
			}
			items = append(items, req.v)
			req.reply <- nil
		case reply := <-gets:
			reply <- getResp{items[0], true}
			items = items[1:]
		case <-q.closes:
			closed = true
		}
	}
}

func (q *Queue) Put(v int) error {
	reply := make(chan error, 1)
	select {
	case q.puts <- putReq{v, reply}:
		return <-reply
	case <-q.dead:
		return ErrClosed
	}
}

func (q *Queue) Get() (int, bool) {
	reply := make(chan getResp, 1)
	select {
	case q.gets <- reply:
		r := <-reply
		return r.v, r.ok
	case <-q.dead:
		return 0, false
	}
}

func (q *Queue) Close() {
	select {
	case q.closes <- struct{}{}:
	case <-q.dead:
	}
}

func main() {
	q := NewQueue(2)
	fmt.Println("Put 1, Put 2:", q.Put(1), q.Put(2))

	blocked := make(chan error)
	go func() { blocked <- q.Put(3) }() // the queue is full: this Put waits
	time.Sleep(20 * time.Millisecond)

	q.Close()
	fmt.Println("the blocked Put returns:", <-blocked)
	fmt.Println("Put after Close:", q.Put(4))
	q.Close() // idempotent

	for {
		v, ok := q.Get()
		if !ok {
			break
		}
		fmt.Println("Get drained:", v)
	}
	v, ok := q.Get()
	fmt.Println("Get after drain:", v, ok)
}
```

**Output:**

```
Put 1, Put 2: <nil> <nil>
the blocked Put returns: queue closed
Put after Close: queue closed
Get drained: 1
Get drained: 2
Get after drain: 0 false
```

Why this design holds up: every state change happens in one goroutine, so "closed" and "has items" can't race each other. Blocked callers wait **in their channel send**, which the owner simply isn't receiving yet. After close the owner receives those sends again and answers `ErrClosed`. The owner exits once there's nothing left to deliver, and `dead` answers every caller after that, so no goroutine is left behind. Replies go on 1-slot channels, so the owner never blocks on a caller.

</details>

---

## 108. Scatter-gather: one required call plus optional calls

`🔴 hard` · *Interview task*

**Task:** a product page needs data from three services, called **concurrently**:

- **user**: required. If it fails, or isn't back within the **100ms** page budget, the page fails.
- **recommendations** and **reviews**: optional. Whatever arrives within **30ms** is included; anything later is skipped, and the page renders without it.

Return the assembled page or an error, as soon as the outcome is known.

**Rules:** no goroutine may leak, however late a service answers; no mutex.

<details>
<summary><b>Hint</b></summary>

Use one `select` loop over the three result channels plus two timers: the page deadline and the optional budget. When a result arrives, set that channel to `nil` so its case switches off. When the optional budget fires, set **both** optional channels to `nil`, which means "stop waiting for them". Loop while the user is missing or an optional result is still awaited. Make every result channel buffered (capacity 1) so late services can still deliver and exit.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"errors"
	"fmt"
	"runtime"
	"time"
)

const ms = time.Millisecond

type Page struct {
	User            string
	Recommendations []string // nil when skipped
	Reviews         []string // nil when skipped
}

type userResult struct {
	name string
	err  error
}

type delays struct {
	user, recs, reviews time.Duration
	userFails           bool
}

func callUser(d delays, out chan<- userResult) {
	time.Sleep(d.user)
	if d.userFails {
		out <- userResult{err: errors.New("user service down")}
		return
	}
	out <- userResult{name: "alice"}
}

func callList(delay time.Duration, items []string, out chan<- []string) {
	time.Sleep(delay)
	out <- items
}

// buildPage needs the user (within 100ms total); recommendations and
// reviews are optional and only get 30ms.
func buildPage(d delays) (Page, error) {
	users := make(chan userResult, 1) // buffered: late calls exit instead of leaking
	recs := make(chan []string, 1)
	reviews := make(chan []string, 1)

	// Channels go in as arguments: the loop below sets its own variables to
	// nil, and the callers must keep sending on the real channels.
	go callUser(d, users)
	go callList(d.recs, []string{"gopher mug", "go book"}, recs)
	go callList(d.reviews, []string{"5 stars"}, reviews)

	deadline := time.After(100 * ms)
	optionalBudget := time.After(30 * ms)
	var page Page
	gotUser := false
	for !gotUser || recs != nil || reviews != nil {
		select {
		case u := <-users:
			if u.err != nil {
				return Page{}, u.err
			}
			page.User, gotUser, users = u.name, true, nil
		case r := <-recs:
			page.Recommendations, recs = r, nil
		case r := <-reviews:
			page.Reviews, reviews = r, nil
		case <-optionalBudget:
			recs, reviews, optionalBudget = nil, nil, nil // stop waiting for extras
		case <-deadline:
			return Page{}, errors.New("timed out waiting for user")
		}
	}
	return page, nil
}

func show(items []string) string {
	if items == nil {
		return "(skipped)"
	}
	return fmt.Sprint(items)
}

func main() {
	for _, c := range []struct {
		name string
		d    delays
	}{
		{"everything fast", delays{user: 10 * ms, recs: 10 * ms, reviews: 15 * ms}},
		{"slow recommendations", delays{user: 10 * ms, recs: 80 * ms, reviews: 15 * ms}},
		{"user fails", delays{user: 5 * ms, recs: 10 * ms, reviews: 10 * ms, userFails: true}},
		{"user too slow", delays{user: 150 * ms, recs: 10 * ms, reviews: 10 * ms}},
	} {
		page, err := buildPage(c.d)
		if err != nil {
			fmt.Printf("%-21s error: %v\n", c.name+":", err)
			continue
		}
		fmt.Printf("%-21s user=%s recs=%s reviews=%s\n",
			c.name+":", page.User, show(page.Recommendations), show(page.Reviews))
	}
	time.Sleep(200 * ms) // every late call has finished by now
	fmt.Println("goroutines left:", runtime.NumGoroutine())
}
```

**Output:**

```
everything fast:      user=alice recs=[gopher mug go book] reviews=[5 stars]
slow recommendations: user=alice recs=(skipped) reviews=[5 stars]
user fails:           error: user service down
user too slow:        error: timed out waiting for user
goroutines left: 1
```

**The trap this solution avoids:** the service goroutines receive their channels **as arguments**. The first draft of this example used closures (`go func() { recs <- ... }()`), which capture the *variable* `recs`. When the loop later set `recs = nil`, `-race` reported a data race, and a late service that read the variable after that point was sending on a **nil channel**, blocked forever: a leak. When you use the nil-channel trick, the goroutines on the other end must hold their own copies of the channel.

The nil-channel trick (examples 23 and 37) does all the bookkeeping. Each piece of data, and the optional budget itself, is a case that switches off once it's done. The loop condition reads like the requirements: "wait while we lack the user, or an extra is still wanted". Buffered result channels are what make `goroutines left: 1` true: a late recommendation still has somewhere to put its value (example 88). With `context` (lesson 15) you would also **cancel** the slow calls instead of letting them finish.

</details>

---

## 109. Web Crawler (Tour of Go): parallel, no duplicate visits, knows when it's done

`🔴 hard` · *Interview task*

**Task:** the Tour of Go's final concurrency exercise. Starting from a URL, fetch pages up to a depth limit, following links. Fetch **in parallel**, **never fetch the same URL twice**, limit concurrent fetches to 3, and return as soon as the crawl is complete.

**Rules:** no `time.Sleep` to wait for the crawl to finish; no mutex (the Tour suggests one; do it with channels instead); no data races.

<details>
<summary><b>Hint</b></summary>

Keep **all** the bookkeeping in one coordinator goroutine: the `seen` map and a `pending` counter of fetches in flight. Fetch goroutines only fetch and send their result back. The coordinator adds new links it hasn't seen, increments `pending` for each fetch it starts, and decrements it for each result it receives. When `pending` reaches 0, nothing is running and nothing can start: you're done.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"slices"
	"time"
)

type page struct {
	body  string
	links []string
}

// fetcher is a fake web, like the one in the Tour of Go.
var fetcher = map[string]page{
	"https://golang.org/": {"The Go Programming Language",
		[]string{"https://golang.org/pkg/", "https://golang.org/cmd/"}},
	"https://golang.org/pkg/": {"Packages",
		[]string{"https://golang.org/", "https://golang.org/cmd/", "https://golang.org/pkg/fmt/", "https://golang.org/pkg/os/"}},
	"https://golang.org/pkg/fmt/": {"Package fmt",
		[]string{"https://golang.org/", "https://golang.org/pkg/"}},
	"https://golang.org/pkg/os/": {"Package os",
		[]string{"https://golang.org/", "https://golang.org/pkg/"}},
}

func fetch(url string) (string, []string, error) {
	time.Sleep(5 * time.Millisecond) // network latency
	if p, ok := fetcher[url]; ok {
		return p.body, p.links, nil
	}
	return "", nil, fmt.Errorf("not found: %s", url)
}

type fetched struct {
	url, body string
	links     []string
	err       error
	depth     int
}

// crawl fetches in parallel (at most maxParallel at once), never fetches a URL
// twice, and returns as soon as the last fetch has reported back.
func crawl(start string, depth, maxParallel int) []string {
	results := make(chan fetched)
	sem := make(chan struct{}, maxParallel)
	seen := map[string]bool{start: true} // only this goroutine touches it

	launch := func(url string, depth int) {
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			body, links, err := fetch(url)
			results <- fetched{url, body, links, err, depth}
		}()
	}

	var report []string
	launch(start, depth)
	for pending := 1; pending > 0; pending-- { // one fewer in flight per result...
		r := <-results
		if r.err != nil {
			report = append(report, "error: "+r.err.Error())
			continue
		}
		report = append(report, fmt.Sprintf("found: %s %q", r.url, r.body))
		if r.depth <= 1 {
			continue
		}
		for _, u := range r.links {
			if !seen[u] {
				seen[u] = true
				pending++ // ...one more per new fetch
				launch(u, r.depth-1)
			}
		}
	}
	slices.Sort(report) // fetches finish in any order
	return report
}

func main() {
	for _, line := range crawl("https://golang.org/", 4, 3) {
		fmt.Println(line)
	}
}
```

**Output:**

```
error: not found: https://golang.org/cmd/
found: https://golang.org/ "The Go Programming Language"
found: https://golang.org/pkg/ "Packages"
found: https://golang.org/pkg/fmt/ "Package fmt"
found: https://golang.org/pkg/os/ "Package os"
```

Why counting beats the alternatives: a `WaitGroup` would need `Add` calls from inside fetch goroutines while `main` waits, which is easy to get wrong. Closing `results` has no safe moment, since any in-flight fetch might still discover links. The coordinator already knows the exact number of results it's waiting for, so the crawl ends precisely when the last one arrives. Duplicate suppression is race-free because only the coordinator reads and writes `seen`. The `sem` channel bounds parallelism **inside** the fetch goroutines, so the coordinator never blocks on it (it would deadlock example 54-style otherwise).

</details>

---

## 110. Dining Philosophers (LeetCode 1226) without deadlock

`🔴 hard` · *Interview task*

**Task:** five philosophers sit around a table with one fork between each pair. To eat, a philosopher needs **both** neighbouring forks. Each must eat 3 times, all running concurrently, and the program must never deadlock, whatever the timing.

**Rules:** forks are channels; no mutex; first show why the naive "left fork, then right fork" approach fails.

<details>
<summary><b>Hint</b></summary>

A fork is a 1-slot channel: receive to pick it up, send to put it back. The naive version deadlocks when everyone holds their left fork at the same moment, which is a **cycle** of waiting. Break the cycle with a global order: always pick up the **lower-numbered** fork first. Philosopher 4 then reaches for fork 0 before fork 4, so no cycle can form.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// newForks puts n forks on the table: a token in a 1-slot channel means "free".
func newForks(n int) []chan struct{} {
	forks := make([]chan struct{}, n)
	for i := range forks {
		forks[i] = make(chan struct{}, 1)
		forks[i] <- struct{}{}
	}
	return forks
}

// naive: everyone takes the left fork, then the right one.
func naive(n int) int {
	forks := newForks(n)
	var allHaveLeft sync.WaitGroup
	allHaveLeft.Add(n)
	stuck := make(chan bool, n)
	for p := range n {
		go func() {
			<-forks[p] // left fork
			allHaveLeft.Done()
			allHaveLeft.Wait() // (force the unlucky timing for the demo)
			select {
			case <-forks[(p+1)%n]: // right fork: held by the neighbour
				stuck <- false
			case <-time.After(50 * time.Millisecond): // stands in for "forever"
				stuck <- true
			}
		}()
	}
	count := 0
	for range n {
		if <-stuck {
			count++
		}
	}
	return count
}

// ordered: everyone takes the LOWER-numbered fork first. No cycle, no deadlock.
func ordered(n, meals int) []int {
	forks := newForks(n)
	eaten := make([]int, n) // each philosopher writes only its own slot
	var wg sync.WaitGroup
	for p := range n {
		wg.Go(func() {
			left, right := p, (p+1)%n
			first, second := min(left, right), max(left, right)
			for range meals {
				<-forks[first]
				<-forks[second]
				eaten[p]++
				time.Sleep(time.Millisecond) // eating
				forks[second] <- struct{}{}
				forks[first] <- struct{}{}
			}
		})
	}
	wg.Wait()
	return eaten
}

func main() {
	fmt.Println("naive:   philosophers stuck holding one fork:", naive(5), "of 5")
	fmt.Println("ordered: meals eaten per philosopher:", ordered(5, 3))
}
```

**Output:**

```
naive:   philosophers stuck holding one fork: 5 of 5
ordered: meals eaten per philosopher: [3 3 3 3 3]
```

This is the same fix as for mutex deadlocks: **acquire resources in a consistent global order**. With circular waiting impossible, deadlock is impossible. Other accepted answers: a "waiter" that lets at most 4 philosophers try at once (a `make(chan struct{}, 4)` semaphore), or picking up the second fork with a timeout and backing off. The naive demo forces the fatal timing with a barrier and uses a timeout in place of "forever", so it can report the deadlock instead of hanging.

</details>

---

## 111. TTL cache with no mutex: one goroutine owns the data

`🔴 hard` · *Interview task*

**Task:** build a concurrency-safe in-memory cache with `Set(key, value, ttl)`, `Get(key) (value, ok)`, `Len()` and `Close()`. Expired entries must never be returned, and they must be **removed in the background**, even if nobody asks for them again. `Close` stops the background work, and calls after `Close` must not block.

**Rules:** no `sync.Mutex`, no `sync.Map`. The interviewer wants the channel-based design (`sync.Once` for `Close` is fine).

<details>
<summary><b>Hint</b></summary>

One goroutine owns the map, and every method sends it a request (with a reply channel where an answer is needed). Its `select` loop also has a `ticker.C` case, so cleanup is just another event, handled between requests and never racing them. For "after Close, don't block", have every method also `select` on a `stopped` channel that the owner closes when it exits.

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type entry struct {
	value   string
	expires time.Time
}

type setReq struct {
	key, value string
	ttl        time.Duration
}

type getReq struct {
	key   string
	reply chan entry // zero entry = miss
}

// Cache is owned by one goroutine; methods are requests to it.
type Cache struct {
	sets      chan setReq
	gets      chan getReq
	lens      chan chan int
	quit      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

func NewCache(cleanupEvery time.Duration) *Cache {
	c := &Cache{sets: make(chan setReq), gets: make(chan getReq), lens: make(chan chan int),
		quit: make(chan struct{}), stopped: make(chan struct{})}
	go c.run(cleanupEvery)
	return c
}

func (c *Cache) run(every time.Duration) {
	defer close(c.stopped)
	data := map[string]entry{} // only this goroutine touches data
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case r := <-c.sets:
			data[r.key] = entry{r.value, time.Now().Add(r.ttl)}
		case r := <-c.gets:
			e, ok := data[r.key]
			if ok && time.Now().After(e.expires) {
				delete(data, r.key) // expired, the cleanup just hasn't run yet
				e = entry{}
			}
			r.reply <- e
		case r := <-c.lens:
			r <- len(data)
		case now := <-ticker.C: // cleanup: drop everything expired
			for k, e := range data {
				if now.After(e.expires) {
					delete(data, k)
				}
			}
		case <-c.quit:
			return
		}
	}
}

func (c *Cache) Set(key, value string, ttl time.Duration) {
	select {
	case c.sets <- setReq{key, value, ttl}:
	case <-c.stopped:
	}
}

func (c *Cache) Get(key string) (string, bool) {
	reply := make(chan entry, 1)
	select {
	case c.gets <- getReq{key, reply}:
		e := <-reply
		return e.value, !e.expires.IsZero()
	case <-c.stopped:
		return "", false
	}
}

func (c *Cache) Len() int {
	reply := make(chan int, 1)
	select {
	case c.lens <- reply:
		return <-reply
	case <-c.stopped:
		return 0
	}
}

func (c *Cache) Close() {
	c.closeOnce.Do(func() { close(c.quit) })
	<-c.stopped
}

func main() {
	c := NewCache(10 * time.Millisecond)
	c.Set("session", "alice", 30*time.Millisecond)
	c.Set("config", "v1", time.Hour)

	v, ok := c.Get("session")
	fmt.Printf("session now:        %q %v\n", v, ok)

	time.Sleep(60 * time.Millisecond)
	fmt.Printf("entries after 60ms: %d (cleanup removed the session)\n", c.Len())
	v, ok = c.Get("session")
	fmt.Printf("session after 60ms: %q %v\n", v, ok)
	v, ok = c.Get("config")
	fmt.Printf("config after 60ms:  %q %v\n", v, ok)

	c.Close()
	c.Close() // idempotent
	v, ok = c.Get("config")
	fmt.Printf("Get after Close:    %q %v\n", v, ok)
}
```

**Output:**

```
session now:        "alice" true
entries after 60ms: 1 (cleanup removed the session)
session after 60ms: "" false
config after 60ms:  "v1" true
Get after Close:    "" false
```

The design question interviewers are probing is "mutex or owner goroutine?". The owner wins here because **time-based work** (the ticker) and requests are serialized by the same `select`, with no lock ordering to think about. The costs: every call is a channel round-trip, slower than an `RWMutex` read, and one slow request stalls all the others. `Get` still checks expiry itself, because the cleanup runs only every 10ms. Lifecycle is example 59's stop-and-wait handshake; lesson 38 covers caching strategy itself.

</details>

---

## 112. Capstone: ordered parallel pipeline that stops on the first error

`🔴 hard` · *Interview task*

**Task:** implement `ProcessAll(inputs []string, workers int) ([]int, error)`. It parses each input as an integer and squares it, using at most `workers` goroutines at once, and returns the results **in input order**. If any input fails, it returns that error **promptly** and stops all remaining work. After it returns, successfully or not, **no goroutine it started may still be running**.

**Rules:** use `done` channels, not `context` or `errgroup` (those come in lesson 15, and interviewers often ask for the manual version first). Prove there are no leaks.

<details>
<summary><b>Hint</b></summary>

You need four pieces:

1. A feeder that selects every send against `done` and closes `jobs`.
2. Workers that `range jobs` and select their result send against `done`.
3. A helper that closes `results` after `wg.Wait()`.
4. In the caller, `defer close(done)`, so returning early (on the first error) shuts everything down.

Tag each job with its index to restore the order (example 39).

</details>

<details>
<summary><b>Solution</b></summary>

```go
package main

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var started atomic.Int32 // for the demo: how many jobs began work

type job struct {
	idx int
	s   string
}

type result struct {
	idx, val int
	err      error
}

// ProcessAll parses and squares every input with `workers` goroutines.
// Results come back in input order; the first error stops all work.
func ProcessAll(inputs []string, workers int) ([]int, error) {
	done := make(chan struct{})
	defer close(done) // every return path stops the feeder and the workers

	jobs := make(chan job)
	go func() {
		defer close(jobs)
		for i, s := range inputs {
			select {
			case jobs <- job{i, s}:
			case <-done:
				return
			}
		}
	}()

	results := make(chan result)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for j := range jobs {
				started.Add(1)
				time.Sleep(5 * time.Millisecond) // pretend work
				n, err := strconv.Atoi(j.s)
				if err != nil {
					err = fmt.Errorf("input %d (%q): %w", j.idx, j.s, err)
				}
				select {
				case results <- result{j.idx, n * n, err}:
				case <-done:
					return
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]int, len(inputs))
	for r := range results {
		if r.err != nil {
			return nil, r.err
		}
		out[r.idx] = r.val // put each result at its input position
	}
	return out, nil
}

func main() {
	before := runtime.NumGoroutine()

	vals, err := ProcessAll([]string{"1", "2", "3", "4", "5", "6", "7", "8"}, 3)
	fmt.Println("all valid:  ", vals, err)

	started.Store(0)
	inputs := []string{"1", "2", "x", "4", "5", "6", "7", "8", "9", "10", "11", "12"}
	vals, err = ProcessAll(inputs, 3)
	fmt.Println("one invalid:", vals, err)
	fmt.Println("stopped early, jobs started < 12:", started.Load() < 12)

	time.Sleep(30 * time.Millisecond) // give stopped goroutines a moment to exit
	fmt.Println("goroutines leaked:", runtime.NumGoroutine()-before)
}
```

**Output:**

```
all valid:   [1 4 9 16 25 36 49 64] <nil>
one invalid: [] input 2 ("x"): strconv.Atoi: parsing "x": invalid syntax
stopped early, jobs started < 12: true
goroutines leaked: 0
```

Every goroutine has a guaranteed exit:

- **Feeder:** it's blocked only on `jobs <- …`, which also watches `done`.
- **Workers:** they block on `range jobs`, which ends when the feeder closes `jobs`, or on the result send, which watches `done`.
- **Closer:** it finishes once all workers do.

A single `defer close(done)` triggers all of that on **any** return. This one function combines the lessons of examples 31, 39, 67, 88, 91 and 93, and it's exactly what `errgroup.WithContext` with `SetLimit` gives you in lesson 15.

</details>

---

> ← Back to the [index](README.md) · Prev: [🎯 interview quiz](6-interview-quiz.md)
