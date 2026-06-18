package domain

import (
	"errors"
	"time"
)

// Status is the outcome of probing a single URL.
type Status string

const (
	StatusUp    Status = "up"    // got an HTTP response with status < 400
	StatusDown  Status = "down"  // reachable, but returned 4xx/5xx
	StatusError Status = "error" // never got a response (DNS, refused, timeout)
)

// ErrNotFound is returned when a check job does not exist.
var ErrNotFound = errors.New("check job not found")

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

// CheckResult is the outcome of probing one URL.
type CheckResult struct {
	URL        string
	Status     Status
	HTTPStatus int    // 0 when the request never completed
	LatencyMS  int64  // wall-clock time for this single probe
	Error      string // populated when Status == StatusError
}

// CheckJob is one batch of URLs probed concurrently, plus its summary.
type CheckJob struct {
	ID         int64
	Requested  int
	Up         int
	Down       int
	Errored    int
	DurationMS int64 // wall-clock time for the whole concurrent batch
	CreatedAt  time.Time
	Results    []CheckResult
}

// CheckRepository stores completed jobs. The in-memory implementation is
// concurrency-safe; that is part of what this project demonstrates.
type CheckRepository interface {
	Save(job *CheckJob) int64
	Get(id int64) (*CheckJob, error)
	List() []*CheckJob
}
