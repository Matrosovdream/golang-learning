package domain

import (
	"errors"
	"time"
)

// errors.New makes a sentinel error value. It's package-level (var) so callers
// can compare against it with errors.Is.
var ErrNotFound = errors.New("analysis not found")

// A struct carrying one field. Giving it an Error() method (below) makes it
// satisfy the built-in `error` interface.
type ValidationError struct{ Message string }

// Implementing `Error() string` is all it takes to be an error. Value receiver
// (e ValidationError): it only reads, so a copy is fine.
func (e ValidationError) Error() string { return e.Message }

// WordCount is one word and how many times it appeared. Plain fields, no json
// tags — the handler has its own DTO with tags for the wire format.
type WordCount struct {
	Word  string
	Count int
}

// Analysis is the summary of running one text blob through the pipeline.
// Top is a slice ([]WordCount): a growable, ordered list — here sorted by count.
type Analysis struct {
	ID         int64
	Lines      int
	Words      int   // total surviving words counted by the sink
	Unique     int   // number of distinct words (len of the count map)
	DurationMS int64 // wall-clock time for the whole pipeline run
	CreatedAt  time.Time
	Top        []WordCount // the most frequent words, highest first
}

// An interface: any type with these three methods IS an AnalysisRepository. The
// service depends on this, not on the concrete store — so the dependency arrow
// points inward, into domain.
type AnalysisRepository interface {
	Save(a *Analysis) int64
	Get(id int64) (*Analysis, error)
	List() []*Analysis
}
