package repository

import (
	"context"
	"sync"

	"grpcorders/services/orders/internal/domain"
)

// MemoryOrderRepo is an in-memory order store, mutex-guarded for concurrent RPCs.
type MemoryOrderRepo struct {
	mu     sync.Mutex
	seq    int64
	orders map[int64]domain.Order
}

func NewMemoryOrderRepo() *MemoryOrderRepo {
	return &MemoryOrderRepo{orders: make(map[int64]domain.Order)}
}

func (r *MemoryOrderRepo) Create(ctx context.Context, o domain.Order) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	o.ID = r.seq
	r.orders[o.ID] = o
	return o, nil
}

func (r *MemoryOrderRepo) Get(ctx context.Context, id int64) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return o, nil
}
