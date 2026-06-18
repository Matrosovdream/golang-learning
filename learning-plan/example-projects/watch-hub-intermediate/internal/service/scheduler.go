package service

import (
	"context"
	"runtime"
	"sync"
	"time"

	"watchhub/internal/domain"
)

// Stats is the global runtime snapshot exposed at GET /stats.
type Stats struct {
	Monitors    int   `json:"monitors"`
	TotalChecks int64 `json:"total_checks"`
	InFlight    int64 `json:"in_flight"`
	Goroutines  int   `json:"goroutines"`
}

// AddMonitorInput carries validated values for a new monitor.
type AddMonitorInput struct {
	URL      string
	Interval time.Duration
	Timeout  time.Duration
}

// Scheduler owns the lifecycle of every monitor's background goroutine.
//
// One goroutine per monitor runs a ticker loop, independent of any HTTP
// request. Each goroutine has its own context derived from a single root
// context, so:
//   - DELETE /monitors/{id} cancels just that goroutine, and
//   - Stop() cancels the root and waits (WaitGroup) for them all to drain.
type Scheduler struct {
	repo    domain.MonitorRepository
	checker *Checker
	events  *EventHub

	mu        sync.Mutex
	cancels   map[int64]context.CancelFunc
	wg        sync.WaitGroup
	rootCtx   context.Context
	cancelAll context.CancelFunc
}

func NewScheduler(repo domain.MonitorRepository, checker *Checker, events *EventHub) *Scheduler {
	return &Scheduler{
		repo:    repo,
		checker: checker,
		events:  events,
		cancels: make(map[int64]context.CancelFunc),
	}
}

// Start initialises the root context. Call once before serving.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rootCtx, s.cancelAll = context.WithCancel(context.Background())
}

// AddMonitor registers a monitor and launches its background loop.
func (s *Scheduler) AddMonitor(in AddMonitorInput) domain.MonitorView {
	m := s.repo.Add(domain.Monitor{URL: in.URL, Interval: in.Interval, Timeout: in.Timeout})

	s.mu.Lock()
	ctx, cancel := context.WithCancel(s.rootCtx)
	s.cancels[m.ID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(ctx, m)

	v, _ := s.repo.Get(m.ID)
	return v
}

// RemoveMonitor cancels the monitor's goroutine and deletes it.
func (s *Scheduler) RemoveMonitor(id int64) error {
	s.mu.Lock()
	cancel, ok := s.cancels[id]
	if ok {
		delete(s.cancels, id)
	}
	s.mu.Unlock()
	if !ok {
		return domain.ErrNotFound
	}
	cancel() // the run loop sees ctx.Done() and returns
	s.repo.Remove(id)
	return nil
}

// run is the per-monitor loop: check immediately, then on every tick, until
// the monitor's context is cancelled.
func (s *Scheduler) run(ctx context.Context, m domain.Monitor) {
	defer s.wg.Done()

	s.checkOnce(ctx, m)

	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.checkOnce(ctx, m)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) checkOnce(ctx context.Context, m domain.Monitor) {
	prev, _ := s.repo.Get(m.ID) // remember the old status to detect a change

	res := s.checker.Check(ctx, m.URL, m.Timeout)
	if ctx.Err() != nil {
		return // monitor was removed mid-check; drop the stale result
	}
	s.repo.RecordResult(m.ID, res)

	// Only broadcast when the status actually changes (up<->down<->error), so
	// SSE clients see transitions, not a firehose of identical "still up".
	if res.Status != prev.Status {
		s.events.Publish(Event{
			MonitorID:  m.ID,
			URL:        m.URL,
			Status:     string(res.Status),
			HTTPStatus: res.HTTPStatus,
			LatencyMS:  res.LatencyMS,
			At:         res.CheckedAt.Format(time.RFC3339),
		})
	}
}

// CheckNow runs one out-of-band check immediately and stores it.
func (s *Scheduler) CheckNow(id int64) (domain.CheckResult, error) {
	v, err := s.repo.Get(id)
	if err != nil {
		return domain.CheckResult{}, err
	}
	res := s.checker.Check(s.rootCtx, v.URL, v.Timeout)
	s.repo.RecordResult(id, res)
	return res, nil
}

func (s *Scheduler) GetMonitor(id int64) (domain.MonitorView, error) { return s.repo.Get(id) }
func (s *Scheduler) ListMonitors() []domain.MonitorView              { return s.repo.List() }
func (s *Scheduler) Count() int                                      { return s.repo.Count() }

// Stats reports global counters, including the live goroutine count — handy for
// seeing that each monitor really is its own goroutine.
func (s *Scheduler) Stats() Stats {
	return Stats{
		Monitors:    s.repo.Count(),
		TotalChecks: s.checker.TotalChecks(),
		InFlight:    s.checker.InFlight(),
		Goroutines:  runtime.NumGoroutine(),
	}
}

// Stop cancels every monitor goroutine and blocks until they have all returned.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancelAll != nil {
		s.cancelAll()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
