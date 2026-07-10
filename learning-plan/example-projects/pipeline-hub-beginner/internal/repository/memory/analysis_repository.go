package memory

import (
	"sort"
	"sync"

	"pipelinehub/internal/domain"
)

// AnalysisStore is an in-memory store, and the sync.RWMutex is the whole lesson.
// The mutex is held by value (its zero value is ready to use, no init needed);
// it guards the map below so many goroutines can touch it safely. Run the app
// with -race and this stays clean.
type AnalysisStore struct {
	mu       sync.RWMutex
	nextID   int64
	analyses map[int64]*domain.Analysis
}

// A map must be made before use — writing to a nil map panics. make allocates it.
func NewAnalysisStore() *AnalysisStore {
	return &AnalysisStore{analyses: make(map[int64]*domain.Analysis)}
}

// Pointer receiver (s *AnalysisStore): the method mutates the one shared store.
func (s *AnalysisStore) Save(a *domain.Analysis) int64 {
	s.mu.Lock()         // full write lock: blocks all other readers and writers
	defer s.mu.Unlock() // defer runs Unlock on every return path
	s.nextID++
	a.ID = s.nextID
	s.analyses[a.ID] = a
	return a.ID
}

func (s *AnalysisStore) Get(id int64) (*domain.Analysis, error) {
	s.mu.RLock() // read lock: many readers may hold it at the same time
	defer s.mu.RUnlock()
	// comma-ok lookup: ok is false when the key is absent (a would be nil).
	a, ok := s.analyses[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return a, nil
}

func (s *AnalysisStore) List() []*domain.Analysis {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// len 0, capacity len(s.analyses): pre-size the slice so append never re-grows it.
	out := make([]*domain.Analysis, 0, len(s.analyses))
	for _, a := range s.analyses { // range over a map yields (key, value); _ drops the key
		out = append(out, a)
	}
	// sort.Slice takes a "less" func; using > sorts IDs descending (newest first).
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}
