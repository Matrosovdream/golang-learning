package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"pipelinehub/internal/domain"
)

// stopwords is a SET: map[string]struct{} uses the zero-width struct{} as its
// value type, so each entry stores only the key and no payload. Membership is
// tested with the comma-ok form (see words below).
var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {},
	"of": {}, "to": {}, "in": {}, "on": {}, "at": {}, "for": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "it": {},
	"this": {}, "that": {}, "with": {}, "as": {}, "by": {}, "from": {},
}

// Pipeline runs a text through a chain of concurrent stages connected by channels.
type Pipeline struct {
	repo        domain.AnalysisRepository // interface field: any store implementation fits
	workerCount int                       // tokenizer fan-out width

	// atomic.Int64 is a counter you can increment from many goroutines safely.
	// A plain int written by multiple goroutines is a data race (-race flags it);
	// .Add / .Load are the lock-free fix for a single shared number. It spans
	// EVERY request, so it's process-wide.
	totalWords atomic.Int64
}

// Returns *Pipeline (a pointer) so every caller shares the one instance.
func NewPipeline(repo domain.AnalysisRepository, workerCount int) *Pipeline {
	return &Pipeline{repo: repo, workerCount: workerCount}
}

// Analyze wires the pipeline together and runs a text through it end to end.
//
//	text ─►[source]─lines─►[tokenize ×N]─words─►[count sink]─► top-N
//	       (goroutine)      (fan-out/fan-in)     (this goroutine)
//
// Each call below just STARTS goroutines and hands back a channel — no data has
// moved yet. The pipeline only actually runs once the sink (count) starts
// pulling words through it; then closing cascades DOWNSTREAM stage by stage.
func (p *Pipeline) Analyze(ctx context.Context, text string, topN, minLen int) *domain.Analysis {
	start := time.Now()

	lines := p.source(ctx, text)                           // stage 1: text → lines
	words := p.tokenize(ctx, lines, p.workerCount, minLen) // stage 2: lines → words (fan-out)
	counts, total := p.count(ctx, words)                   // stage 3: words → tally (blocks here)

	a := &domain.Analysis{
		Lines:      strings.Count(text, "\n") + 1, // equals the number of lines source emitted
		Words:      total,
		Unique:     len(counts),
		DurationMS: time.Since(start).Milliseconds(),
		CreatedAt:  start,
		Top:        topWords(counts, topN),
	}
	p.repo.Save(a)
	return a
}

// source is the FIRST stage. It splits the text into lines and streams them out,
// one per send. The return type is <-chan string — a RECEIVE-ONLY channel: the
// caller can only <-read from it, never send or close. Directional channel types
// like this document a channel's role and let the compiler enforce it.
func (p *Pipeline) source(ctx context.Context, text string) <-chan string {
	out := make(chan string) // bidirectional in here; narrowed to <-chan on return
	go func() {              // run the producer on its own goroutine
		defer close(out) // close when this goroutine ends: signals "no more lines"
		for _, line := range strings.Split(text, "\n") {
			select { // proceed with whichever case is ready first
			case out <- line: // hand this line to the next stage
			case <-ctx.Done(): // caller cancelled: stop early
				return // the deferred close(out) still runs on the way out
			}
		}
	}()
	return out // a bidirectional chan converts implicitly to a <-chan string
}

// tokenize is the MIDDLE stage and the FAN-OUT: `workers` goroutines all read
// the same `in` channel, so the lines get shared out among them. `in` is
// <-chan string (receive-only, given to us); the private send side of `out` is
// kept inside, and only its <-chan face is returned. Each worker normalizes a
// line into words and sends the survivors downstream.
func (p *Pipeline) tokenize(ctx context.Context, in <-chan string, workers, minLen int) <-chan string {
	out := make(chan string)
	var wg sync.WaitGroup // counts the worker goroutines so we know when all finish
	for i := 0; i < workers; i++ {
		wg.Add(1)   // register one goroutine BEFORE launching it
		go func() { // `go` runs this function literal as a new goroutine
			defer wg.Done()        // signal "this worker finished" on return
			for line := range in { // pull lines until `in` is closed upstream
				for _, word := range p.words(line, minLen) {
					select {
					case out <- word: // forward one surviving word to the sink
					case <-ctx.Done(): // cancelled: abandon the rest
						return
					}
				}
			}
		}()
	}
	// Closer goroutine: wg.Wait() blocks until EVERY worker has returned, then
	// close(out). This is the fan-IN — many workers, one output channel. "Wait
	// then close" is the only safe way to close a channel written by multiple
	// goroutines: closing while a worker might still send would panic.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// count is the SINK — the LAST stage. It runs on the CALLING goroutine (no `go`),
// ranging over words until `in` closes. That happens only after tokenize's closer
// fires, which happens only after source closes `lines`: closing CASCADES down
// the pipeline, and this range ending is what finally unblocks Analyze.
//
// ctx isn't read here on purpose: cancellation reaches the sink INDIRECTLY —
// upstream stages observe ctx.Done(), stop, and close their channels, which ends
// this range. Keeping ctx in the signature keeps every stage uniform.
func (p *Pipeline) count(ctx context.Context, in <-chan string) (map[string]int, int) {
	counts := make(map[string]int)
	total := 0
	for word := range in { // fan-in point: words from every worker arrive here
		counts[word]++ // map write is safe: only THIS goroutine touches counts
		total++
		p.totalWords.Add(1) // atomic: lock-free process-wide counter across requests
	}
	return counts, total
}

// words normalizes one line into filtered words: lowercase, split on any
// non-letter/digit, then drop stopwords and anything shorter than minLen.
func (p *Pipeline) words(line string, minLen int) []string {
	// strings.FieldsFunc splits wherever the predicate is true — here on any rune
	// that isn't a letter or digit, which strips punctuation for free.
	fields := strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(fields))
	for _, w := range fields {
		if len(w) < minLen {
			continue
		}
		if _, stop := stopwords[w]; stop { // comma-ok: membership test on the set
			continue
		}
		out = append(out, w)
	}
	return out
}

// These one-line methods just delegate to the repository.
func (p *Pipeline) Get(id int64) (*domain.Analysis, error) { return p.repo.Get(id) }

// List returns all stored analyses, newest first.
func (p *Pipeline) List() []*domain.Analysis { return p.repo.List() }

// TotalWords atomically reads the process-wide counter's current value.
func (p *Pipeline) TotalWords() int64 { return p.totalWords.Load() }

// topWords returns the n most frequent words, ties broken alphabetically.
func topWords(counts map[string]int, n int) []domain.WordCount {
	// Flatten the map into a slice so it can be sorted (maps have no order).
	wcs := make([]domain.WordCount, 0, len(counts))
	for word, c := range counts {
		wcs = append(wcs, domain.WordCount{Word: word, Count: c})
	}
	// sort.Slice with a two-key "less": higher count first, then A→Z on ties.
	sort.Slice(wcs, func(i, j int) bool {
		if wcs[i].Count != wcs[j].Count {
			return wcs[i].Count > wcs[j].Count
		}
		return wcs[i].Word < wcs[j].Word
	})
	if n < len(wcs) {
		wcs = wcs[:n] // slice expression: keep only the first n elements
	}
	return wcs
}
