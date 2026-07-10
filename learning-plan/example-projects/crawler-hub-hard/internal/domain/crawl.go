package domain

import (
	"errors"
	"time"
)

// errors.New makes a sentinel error value. It's package-level (var) so callers
// can compare against it with errors.Is instead of matching on a string.
var ErrNotFound = errors.New("crawl job not found")

// A struct carrying one field. Giving it an Error() method (below) makes it
// satisfy the built-in `error` interface — no inheritance, just the method.
type ValidationError struct{ Message string }

// Implementing `Error() string` is all it takes to be an error. Value receiver
// (e ValidationError): it only reads, so operating on a copy is fine.
func (e ValidationError) Error() string { return e.Message }

// PageResult is the outcome of fetching ONE page. Plain fields, no json tags —
// the handler owns a separate DTO with tags for the wire format.
type PageResult struct {
	URL        string
	Depth      int    // how many links from the seed this page was reached at
	HTTPStatus int    // 0 when the request never completed
	LinksFound int    // in-scope links discovered on this page (an out-degree)
	LatencyMS  int64  // wall-clock time for this single fetch
	Error      string // populated when the fetch failed
}

// CrawlJob is one whole crawl (a seed plus everything reachable from it) and its
// summary. Results is a slice ([]PageResult): a growable, ordered list.
type CrawlJob struct {
	ID              int64
	Seed            string
	Pages           int   // distinct pages fetched
	Edges           int   // sum of LinksFound across pages (> Pages when there are cycles)
	MaxDepthReached int   // deepest page reached from the seed
	Fetches         int   // total fetch attempts for this crawl (atomic-counted)
	DurationMS      int64 // wall-clock time for the whole concurrent crawl
	CreatedAt       time.Time
	Results         []PageResult
}

// An interface: any type with these three methods IS a CrawlRepository. The
// service depends on this, not on the concrete store — so the dependency arrow
// points inward, into domain.
type CrawlRepository interface {
	Save(job *CrawlJob) int64
	Get(id int64) (*CrawlJob, error)
	List() []*CrawlJob
}
