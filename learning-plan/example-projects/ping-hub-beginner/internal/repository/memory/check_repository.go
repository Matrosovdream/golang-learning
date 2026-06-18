package memory

import (
	"sort"
	"sync"

	"pinghub/internal/domain"
)

// CheckStore is an in-memory, concurrency-safe store for check jobs.
//
// A real service would use Postgres; here the point is the sync.RWMutex.
// Many HTTP requests can read the store at the same time (RLock), but a
// write (Save) takes the exclusive lock so the map is never touched
// concurrently — run the app with `-race` and you'll see this stays clean.
type CheckStore struct {
	mu     sync.RWMutex
	nextID int64
	jobs   map[int64]*domain.CheckJob
}

func NewCheckStore() *CheckStore {
	return &CheckStore{jobs: make(map[int64]*domain.CheckJob)}
}

// Save assigns an ID and stores the job. Write lock: exclusive access.
func (s *CheckStore) Save(job *domain.CheckJob) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	job.ID = s.nextID
	s.jobs[job.ID] = job
	return job.ID
}

// Get returns a job by ID. Read lock: many readers may hold it at once.
func (s *CheckStore) Get(id int64) (*domain.CheckJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return job, nil
}

// List returns all jobs, newest first.
func (s *CheckStore) List() []*domain.CheckJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.CheckJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}
