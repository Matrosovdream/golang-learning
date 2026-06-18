package memory

import (
	"sort"
	"sync"
	"time"

	"watchhub/internal/domain"
)

// entry is one stored monitor: its config plus the live state mutated by the
// scheduler on every check. It carries its OWN mutex — fine-grained locking, so
// recording a result for monitor A never blocks reads of monitor B.
type entry struct {
	mu      sync.RWMutex
	cfg     domain.Monitor
	status  domain.Status
	lastAt  time.Time
	lastLat int64
	total   int64
	up      int64
	fails   int
	history []domain.CheckResult // ring: newest last, capped at histSize
}

// MonitorStore is an in-memory, concurrency-safe registry of monitors.
//
// Two levels of locking:
//   - the store's mutex guards the MAP itself (add/remove/iterate);
//   - each entry's mutex guards that monitor's mutable state.
//
// A real service would persist to Postgres; here the point is the locking.
type MonitorStore struct {
	mu       sync.RWMutex
	nextID   int64
	entries  map[int64]*entry
	histSize int
}

func NewMonitorStore(histSize int) *MonitorStore {
	if histSize <= 0 {
		histSize = 20
	}
	return &MonitorStore{entries: make(map[int64]*entry), histSize: histSize}
}

func (s *MonitorStore) Add(m domain.Monitor) domain.Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	m.ID = s.nextID
	m.CreatedAt = time.Now()
	s.entries[m.ID] = &entry{cfg: m, status: domain.StatusPending}
	return m
}

func (s *MonitorStore) Remove(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return false
	}
	delete(s.entries, id)
	return true
}

func (s *MonitorStore) Get(id int64) (domain.MonitorView, error) {
	e, ok := s.lookup(id)
	if !ok {
		return domain.MonitorView{}, domain.ErrNotFound
	}
	return e.view(true), nil
}

func (s *MonitorStore) List() []domain.MonitorView {
	s.mu.RLock()
	es := make([]*entry, 0, len(s.entries))
	for _, e := range s.entries {
		es = append(es, e)
	}
	s.mu.RUnlock() // release the map lock before locking each entry

	out := make([]domain.MonitorView, 0, len(es))
	for _, e := range es {
		out = append(out, e.view(false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *MonitorStore) RecordResult(id int64, res domain.CheckResult) {
	e, ok := s.lookup(id)
	if !ok {
		return // monitor was removed between the check starting and finishing
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = res.Status
	e.lastAt = res.CheckedAt
	e.lastLat = res.LatencyMS
	e.total++
	if res.Status == domain.StatusUp {
		e.up++
		e.fails = 0
	} else {
		e.fails++
	}
	e.history = append(e.history, res)
	if len(e.history) > s.histSize { // histSize is set once and never mutated
		e.history = e.history[len(e.history)-s.histSize:]
	}
}

func (s *MonitorStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *MonitorStore) lookup(id int64) (*entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[id]
	return e, ok
}

func (e *entry) view(withHistory bool) domain.MonitorView {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v := domain.MonitorView{
		Monitor:          e.cfg,
		Status:           e.status,
		LastCheckedAt:    e.lastAt,
		LastLatencyMS:    e.lastLat,
		TotalChecks:      e.total,
		UpChecks:         e.up,
		ConsecutiveFails: e.fails,
	}
	if e.total > 0 {
		v.UptimePct = float64(e.up) / float64(e.total) * 100
	}
	if withHistory && len(e.history) > 0 {
		h := make([]domain.CheckResult, len(e.history))
		copy(h, e.history)
		for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 { // reverse: newest first
			h[i], h[j] = h[j], h[i]
		}
		v.History = h
	}
	return v
}
