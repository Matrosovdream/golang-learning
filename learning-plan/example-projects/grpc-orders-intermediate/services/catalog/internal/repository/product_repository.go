// Package repository implements domain.ProductRepository with an in-memory store.
// It's deliberately not Postgres — this project keeps the focus on gRPC + service
// composition + observability. The hard project (grpc-observability-hard) swaps in
// a real database-per-service.
package repository

import (
	"context"
	"sort"
	"sync"

	"grpcorders/services/catalog/internal/domain"
)

// MemoryProductRepo is safe for concurrent gRPC handlers: one mutex guards the map.
type MemoryProductRepo struct {
	mu    sync.Mutex // gRPC serves each call on its own goroutine, so guard shared state
	seq   int64
	items map[int64]domain.Product
}

func NewMemoryProductRepo() *MemoryProductRepo {
	return &MemoryProductRepo{items: make(map[int64]domain.Product)}
}

func (r *MemoryProductRepo) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	p.ID = r.seq
	r.items[p.ID] = p
	return p, nil
}

func (r *MemoryProductRepo) Get(ctx context.Context, id int64) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return domain.Product{}, domain.ErrNotFound
	}
	return p, nil
}

func (r *MemoryProductRepo) List(ctx context.Context) ([]domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Product, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	// maps have no order; sort so the API is deterministic
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryProductRepo) Reserve(ctx context.Context, id int64, qty int32) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return domain.Product{}, domain.ErrNotFound
	}
	if p.Stock < qty {
		return domain.Product{}, domain.ErrInsufficientStock
	}
	p.Stock -= qty
	r.items[id] = p // map values aren't addressable — write the whole struct back
	return p, nil
}
