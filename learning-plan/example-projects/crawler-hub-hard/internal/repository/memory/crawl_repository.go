package memory

import (
	"sort"
	"sync"

	"crawlerhub/internal/domain"
)

// CrawlStore is an in-memory store, and the sync.RWMutex is the whole lesson.
// The mutex is held by value (its zero value is ready to use, no init needed);
// it guards the map below so many goroutines can touch it safely. Run the app
// with -race and this stays clean.
type CrawlStore struct {
	mu     sync.RWMutex
	nextID int64
	jobs   map[int64]*domain.CrawlJob
}

// A map must be made before use — writing to a nil map panics. make allocates it.
func NewCrawlStore() *CrawlStore {
	return &CrawlStore{jobs: make(map[int64]*domain.CrawlJob)}
}

// Pointer receiver (s *CrawlStore): the method mutates the one shared store.
func (s *CrawlStore) Save(job *domain.CrawlJob) int64 {
	s.mu.Lock()         // full write lock: blocks all other readers and writers
	defer s.mu.Unlock() // defer runs Unlock on every return path
	s.nextID++
	job.ID = s.nextID
	s.jobs[job.ID] = job
	return job.ID
}

func (s *CrawlStore) Get(id int64) (*domain.CrawlJob, error) {
	s.mu.RLock() // read lock: many readers may hold it at the same time
	defer s.mu.RUnlock()
	// comma-ok lookup: ok is false when the key is absent (job would be nil).
	job, ok := s.jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return job, nil
}

func (s *CrawlStore) List() []*domain.CrawlJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// len 0, capacity len(s.jobs): pre-size the slice so append never re-grows it.
	out := make([]*domain.CrawlJob, 0, len(s.jobs))
	for _, j := range s.jobs { // range over a map yields (key, value); _ drops the key
		out = append(out, j)
	}
	// sort.Slice takes a "less" func; using > sorts IDs descending (newest first).
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}
