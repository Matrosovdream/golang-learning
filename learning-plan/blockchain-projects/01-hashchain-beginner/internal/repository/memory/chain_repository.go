package memory

import (
	"sync"

	"hashchain/internal/domain"
)

// ChainStore is an in-memory, concurrency-safe chain: a slice of blocks
// guarded by a sync.RWMutex. Many HTTP requests read at once (RLock); the miner
// and the tamper endpoint take the exclusive Lock to write.
type ChainStore struct {
	mu     sync.RWMutex
	blocks []domain.Block
}

func NewChainStore() *ChainStore { return &ChainStore{} }

// Append adds b only if it extends the tip exactly — right height, right
// PrevHash. The check and the append happen under one lock, so two writers
// can never both extend the same tip.
func (s *ChainStore) Append(b domain.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.blocks) == 0 {
		if b.Height != 0 {
			return domain.ErrTipChanged
		}
	} else {
		tip := s.blocks[len(s.blocks)-1]
		if b.Height != tip.Height+1 || b.PrevHash != tip.Hash {
			return domain.ErrTipChanged
		}
	}
	s.blocks = append(s.blocks, b)
	return nil
}

func (s *ChainStore) Tip() (domain.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.blocks) == 0 {
		return domain.Block{}, domain.ErrNotFound
	}
	return s.blocks[len(s.blocks)-1], nil
}

func (s *ChainStore) Get(height uint64) (domain.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if height >= uint64(len(s.blocks)) {
		return domain.Block{}, domain.ErrNotFound
	}
	return s.blocks[height], nil
}

func (s *ChainStore) Range(from uint64, limit int) []domain.Block {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if from >= uint64(len(s.blocks)) || limit <= 0 {
		return []domain.Block{}
	}
	end := min(from+uint64(limit), uint64(len(s.blocks)))
	// Copy the sub-slice: returning s.blocks[from:end] directly would hand the
	// caller a view into our backing array, readable after we unlock.
	out := make([]domain.Block, end-from)
	copy(out, s.blocks[from:end])
	return out
}

func (s *ChainStore) All() []domain.Block {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Block, len(s.blocks))
	copy(out, s.blocks)
	return out
}

func (s *ChainStore) ReplaceFrom(height uint64, blocks []domain.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if height+uint64(len(blocks)) > uint64(len(s.blocks)) {
		return domain.ErrNotFound
	}
	copy(s.blocks[height:], blocks)
	return nil
}
