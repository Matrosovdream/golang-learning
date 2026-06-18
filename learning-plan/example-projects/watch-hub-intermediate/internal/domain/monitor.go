package domain

import (
	"errors"
	"time"
)

// Status is the latest known state of a monitored endpoint.
type Status string

const (
	StatusPending Status = "pending" // registered, not checked yet
	StatusUp      Status = "up"      // last check returned HTTP < 400
	StatusDown    Status = "down"    // last check returned 4xx/5xx
	StatusError   Status = "error"   // last check never got a response
)

// ErrNotFound is returned when a monitor id does not exist.
var ErrNotFound = errors.New("monitor not found")

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

// CheckResult is the outcome of a single probe.
type CheckResult struct {
	CheckedAt  time.Time
	Status     Status
	HTTPStatus int
	LatencyMS  int64
	Attempts   int    // how many tries it took (retry-with-backoff)
	Error      string // populated when Status == StatusError
}

// Monitor is the configuration of one watched endpoint.
type Monitor struct {
	ID        int64
	URL       string
	Interval  time.Duration // how often the scheduler re-checks it
	Timeout   time.Duration // per-attempt request timeout
	CreatedAt time.Time
}

// MonitorView is a read snapshot: config + live state + recent history.
type MonitorView struct {
	Monitor
	Status           Status
	LastCheckedAt    time.Time
	LastLatencyMS    int64
	TotalChecks      int64
	UpChecks         int64
	UptimePct        float64
	ConsecutiveFails int
	History          []CheckResult // newest first; only filled on single-monitor reads
}

// MonitorRepository stores monitor config plus live state. Every method is
// safe to call from many goroutines at once.
type MonitorRepository interface {
	Add(m Monitor) Monitor
	Remove(id int64) bool
	Get(id int64) (MonitorView, error)
	List() []MonitorView
	RecordResult(id int64, res CheckResult)
	Count() int
}
