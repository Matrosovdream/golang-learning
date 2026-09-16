package service

import (
	"sync"

	"p2pchain/internal/domain"
)

// orphanPool holds blocks whose parent we haven't seen yet.
//
// Gossip has no ordering guarantee: block 11 can arrive before block 10, and a
// node that simply dropped block 11 would have to wait for someone to re-send
// it. Instead we park it, fetch the missing ancestors, and then connect it.
// The pool is capped: an attacker could otherwise fill our memory with blocks
// whose parents will never exist.
type orphanPool struct {
	mu       sync.Mutex
	byParent map[domain.Hash][]domain.Block
	count    int
	max      int
}

func newOrphanPool(max int) *orphanPool {
	return &orphanPool{byParent: make(map[domain.Hash][]domain.Block), max: max}
}

// Add parks a block under its parent hash. It returns false when the pool is full.
func (o *orphanPool) Add(b domain.Block) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.count >= o.max {
		return false
	}
	for _, existing := range o.byParent[b.PrevHash] {
		if existing.Hash == b.Hash {
			return true // already parked
		}
	}
	o.byParent[b.PrevHash] = append(o.byParent[b.PrevHash], b)
	o.count++
	return true
}

// Take removes and returns the blocks waiting for this parent.
func (o *orphanPool) Take(parent domain.Hash) []domain.Block {
	o.mu.Lock()
	defer o.mu.Unlock()
	children := o.byParent[parent]
	delete(o.byParent, parent)
	o.count -= len(children)
	return children
}

func (o *orphanPool) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.count
}
