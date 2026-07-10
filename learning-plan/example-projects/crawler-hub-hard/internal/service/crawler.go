package service

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crawlerhub/internal/domain"
)

// maxBodyBytes caps how much of a page we read before scanning for links. A
// bit-shift constant: 1 << 20 == 1 MiB. Keeps one giant page from eating memory.
const maxBodyBytes = 1 << 20

// Crawler runs one bounded-parallelism crawl per request. It holds shared,
// long-lived state (the store, the HTTP client, a process-wide counter); the
// per-crawl mutable state lives in a crawlRun (below).
type Crawler struct {
	repo   domain.CrawlRepository // interface field: any store implementation fits
	client *http.Client           // pointer: one shared client, safe for concurrent use

	maxConcurrency int // the hard ceiling the handler clamps requests to

	// atomic.Int64 is a counter you can increment from many goroutines safely.
	// A plain int written by multiple goroutines is a data race (-race flags it);
	// .Add / .Load are the lock-free fix for a single shared number.
	totalFetches atomic.Int64
	totalCrawls  atomic.Int64
}

// Returns *Crawler (a pointer) so every caller shares the one instance.
func NewCrawler(repo domain.CrawlRepository, maxConcurrency int) *Crawler {
	return &Crawler{
		repo: repo,
		// &http.Client{} is the address of a new struct literal. No Timeout set
		// here — each request carries its own deadline via context (see fetch).
		client:         &http.Client{},
		maxConcurrency: maxConcurrency,
	}
}

// CrawlParams bundles the per-crawl knobs. Passing a struct beats a long
// positional argument list — each field is named at the call site.
type CrawlParams struct {
	Seed         string
	MaxDepth     int
	MaxPages     int
	Concurrency  int
	RatePerSec   int
	FetchTimeout time.Duration
}

// crawlRun holds every piece of mutable state for ONE crawl. Bundling it in a
// struct lets the recursive crawl method reach the shared semaphore, WaitGroup,
// visited-set and results channel without threading them through every call.
type crawlRun struct {
	c            *Crawler
	base         *url.URL // seed URL: we only follow links to this same host
	maxDepth     int
	maxPages     int
	fetchTimeout time.Duration

	sem     chan struct{}          // buffered chan used as a COUNTING SEMAPHORE
	limiter *Limiter               // ticker token-bucket rate limiter
	results chan domain.PageResult // fan-in: every fetched page is sent here

	wg sync.WaitGroup // counts in-flight crawl goroutines (dynamic set)

	mu      sync.Mutex          // guards visited + pages (the admit check-and-set)
	visited map[string]struct{} // set of URLs already admitted (dedup)
	pages   int                 // pages admitted so far (compared against maxPages)

	fetches atomic.Int64 // fetch attempts in THIS crawl
}

// Run performs one whole crawl and returns the stored job summary.
//
// The engine is BOUNDED PARALLELISM over DYNAMIC work: fetching a page discovers
// more pages to fetch. Three primitives make that terminate cleanly and race-free:
//   - a buffered-channel SEMAPHORE bounds how many fetches run at once,
//   - a WaitGroup + closer goroutine detects "all work (and work it spawned) done",
//   - one mutex-guarded admit() dedups URLs and enforces the page budget.
func (c *Crawler) Run(parent context.Context, p CrawlParams) (*domain.CrawlJob, error) {
	start := time.Now()
	c.totalCrawls.Add(1)

	// Parse the seed once: its host is the boundary we refuse to crawl past.
	base, err := url.Parse(p.Seed)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, domain.ValidationError{Message: "seed must be a valid http(s) url"}
	}

	// A whole-crawl deadline bounds total wall-clock. context.WithTimeout returns
	// a CHILD ctx that auto-cancels; cancel() (via defer) also stops every
	// in-flight fetch because they all select on ctx.Done(). Cancelling `parent`
	// (e.g. the HTTP client disconnecting) tears the whole crawl down too.
	ctx, cancel := context.WithTimeout(parent, p.FetchTimeout*time.Duration(p.MaxPages))
	defer cancel()

	// The limiter runs its own ticker goroutine; Stop() ends it. defer guarantees
	// we don't leak that goroutine or the underlying timer.
	limiter := NewLimiter(p.RatePerSec)
	defer limiter.Stop()

	run := &crawlRun{
		c:            c,
		base:         base,
		maxDepth:     p.MaxDepth,
		maxPages:     p.MaxPages,
		fetchTimeout: p.FetchTimeout,
		sem:          make(chan struct{}, p.Concurrency), // buffer size == max simultaneous fetches
		limiter:      limiter,
		results:      make(chan domain.PageResult),
		visited:      make(map[string]struct{}),
	}

	// Admit the seed ONCE before launching it, so it counts against the budget and
	// a back-link to it can never re-queue it. admit returns false only if the
	// budget is 0 (defensive — the handler clamps maxPages to >= 1).
	seed := canonical(base)
	if run.admit(seed) {
		run.wg.Add(1) // register the goroutine BEFORE `go` (see the idiom in crawl)
		go run.crawl(ctx, seed, 0)
	}

	// Closer goroutine: wg.Wait() blocks until every crawl goroutine AND every
	// child it spawned has returned, THEN close(results). Closing is what ends the
	// collector's range loop below. This wait-then-close is exactly what makes
	// "work that spawns more work" terminate — see the README's termination note.
	go func() {
		run.wg.Wait()
		close(run.results)
	}()

	// Collector: runs on THIS goroutine and tallies every page as it arrives,
	// until results is closed. Because only this goroutine touches `job`, the
	// summary needs no lock of its own.
	job := &domain.CrawlJob{Seed: p.Seed, CreatedAt: start}
	for r := range run.results {
		job.Results = append(job.Results, r)
		job.Pages++
		job.Edges += r.LinksFound
		if r.Depth > job.MaxDepthReached {
			job.MaxDepthReached = r.Depth
		}
	}

	// Safe to read the atomic now: the range loop only ended after results closed,
	// which only happened after wg.Wait() — so no fetch goroutine is still running.
	job.Fetches = int(run.fetches.Load())
	job.DurationMS = time.Since(start).Milliseconds()
	c.repo.Save(job)
	return job, nil
}

// crawl fetches ONE page and spawns a child crawl for each NEW in-scope link it
// finds. It runs as its own goroutine, so it must obey the WaitGroup idiom:
//
//	wg.Add(1) is called by the PARENT before `go crawl(...)`  (add-before-launch)
//	defer wg.Done()                                           (done-on-return)
//	a separate goroutine does wg.Wait() then close(results)   (wait-then-close)
//
// Adding before the launch (never inside the new goroutine) is what guarantees
// the closer can't observe a count of zero while a child is about to be spawned.
func (r *crawlRun) crawl(ctx context.Context, pageURL string, depth int) {
	defer r.wg.Done()

	// Acquire a semaphore slot. sem is a buffered chan struct{}: a send succeeds
	// while there's room in the buffer and BLOCKS once it's full — so at most
	// cap(sem) crawl goroutines fetch at the same time. We also watch ctx.Done()
	// so a cancelled crawl doesn't park here forever.
	select {
	case r.sem <- struct{}{}: // take a token
	case <-ctx.Done():
		return
	}
	defer func() { <-r.sem }() // release the token on every return path

	// Rate limit: block until the token bucket hands out a token (or ctx dies).
	if err := r.limiter.Wait(ctx); err != nil {
		return
	}

	res, links := r.fetch(ctx, pageURL, depth)

	// Send the result to the collector, but keep watching ctx.Done() so a
	// cancelled crawl never blocks on a collector that has stopped receiving.
	select {
	case r.results <- res:
	case <-ctx.Done():
		return
	}

	// Depth limit: we still REPORTED this page, we just don't descend past it.
	if depth >= r.maxDepth {
		return
	}

	// For each discovered link, try to admit it. admit is the single check-and-set
	// under one mutex: it returns true only for a URL not seen before and only
	// while the budget has room. We spawn a child ONLY for admitted links — that
	// is what stops two goroutines crawling the same URL and caps the total pages.
	for _, link := range links {
		if r.admit(link) {
			r.wg.Add(1) // add BEFORE the launch, so the closer can't Wait->close early
			go r.crawl(ctx, link, depth+1)
		}
	}
}

// admit is the ONE check-and-set that makes the crawl correct. Holding a single
// mutex it: (1) rejects a URL already visited — so cycles and back-links can't
// re-crawl a page; and (2) rejects everything once maxPages is reached — the
// budget. Because the check and the mark happen atomically under the lock, two
// goroutines racing on the same new URL can't both win: exactly one gets true.
func (r *crawlRun) admit(u string) bool {
	r.mu.Lock()         // sync.Mutex: only one goroutine past this line at a time
	defer r.mu.Unlock() // release on return, whatever branch we take
	if _, seen := r.visited[u]; seen {
		return false
	}
	if r.pages >= r.maxPages {
		return false
	}
	r.visited[u] = struct{}{} // an empty struct is 0 bytes: a set, not a map-of-values
	r.pages++
	return true
}

// fetch performs one GET under a per-request timeout and extracts its links. It
// returns the domain result plus the in-scope links (kept out of the domain type
// so PageResult stays a flat summary).
func (r *crawlRun) fetch(ctx context.Context, pageURL string, depth int) (domain.PageResult, []string) {
	r.fetches.Add(1)        // per-crawl counter (atomic: many goroutines call fetch)
	r.c.totalFetches.Add(1) // process-wide counter for /stats

	// A child context that auto-cancels after fetchTimeout, and also cancels if
	// the parent ctx does. Always cancel() (via defer) to release its resources.
	reqCtx, cancel := context.WithTimeout(ctx, r.fetchTimeout)
	defer cancel()

	start := time.Now()
	res := domain.PageResult{URL: pageURL, Depth: depth}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, pageURL, nil)
	if err != nil {
		res.Error = shortErr(err)
		return res, nil
	}
	resp, err := r.c.client.Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		// timeouts, DNS failures, connection refused all land here
		res.Error = shortErr(err)
		return res, nil
	}
	defer resp.Body.Close()
	res.HTTPStatus = resp.StatusCode

	// Read at most maxBodyBytes so a huge page can't blow up memory, then drain
	// the rest so the TCP connection can be reused by the pool.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	_, _ = io.Copy(io.Discard, resp.Body)

	links := r.extractLinks(pageURL, string(body))
	res.LinksFound = len(links)
	return res, links
}

// hrefRe pulls the target out of href="..." or href='...'. A production crawler
// would parse the HTML with golang.org/x/net/html; a regexp keeps this project at
// ZERO external dependencies, which is the whole point of the exercise.
var hrefRe = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)

// extractLinks finds every href, resolves it to an absolute URL against the page
// it was found on, and keeps only same-host http(s) links (so the crawl stays on
// the seed's site instead of wandering the whole internet).
func (r *crawlRun) extractLinks(pageURL, body string) []string {
	page, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var out []string
	// FindAllStringSubmatch returns [][]string: one entry per match, [1] is the
	// first capture group (the href value).
	for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
		ref, err := url.Parse(m[1])
		if err != nil {
			continue
		}
		// ResolveReference turns a relative href ("/site/8") into an absolute URL
		// against the page it appeared on (the standard URL-resolution algorithm).
		abs := page.ResolveReference(ref)
		if abs.Scheme != "http" && abs.Scheme != "https" {
			continue // skip mailto:, javascript:, tel:, etc.
		}
		if abs.Host != r.base.Host {
			continue // stay on the seed's host
		}
		out = append(out, canonical(abs))
	}
	return out
}

// These one-line methods just delegate to the repository / atomics.
func (c *Crawler) Get(id int64) (*domain.CrawlJob, error) { return c.repo.Get(id) }
func (c *Crawler) List() []*domain.CrawlJob               { return c.repo.List() }
func (c *Crawler) TotalFetches() int64                    { return c.totalFetches.Load() }
func (c *Crawler) TotalCrawls() int64                     { return c.totalCrawls.Load() }

// canonical returns a stable string form of a URL for the visited-set. Dropping
// the #fragment means "/site/3" and "/site/3#top" dedup to the same page.
func canonical(u *url.URL) string {
	c := *u // copy the struct (value semantics) so we don't mutate the caller's URL
	c.Fragment = ""
	return c.String()
}

// shortErr trims net/http's wrapping (`Get "url": <reason>`) down to the reason.
func shortErr(err error) string {
	msg := err.Error()
	// LastIndex returns -1 when ": " isn't found; otherwise slice past it.
	if i := strings.LastIndex(msg, ": "); i != -1 {
		return msg[i+2:]
	}
	return msg
}
