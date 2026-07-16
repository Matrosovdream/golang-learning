// Package domain is the catalog core: entity, repository interface, sentinel errors.
package domain

import (
	"context"
	"errors"
)

var (
	ErrNotFound          = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Product struct {
	ID         int64
	Name       string
	PriceCents int64
	Stock      int32
}

type ProductRepository interface {
	Create(ctx context.Context, p Product) (Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	List(ctx context.Context) ([]Product, error)
	// Reserve decrements stock inside a transaction (SELECT ... FOR UPDATE) so
	// concurrent checkouts can't oversell. Returns the reserved product.
	Reserve(ctx context.Context, id int64, qty int32) (Product, error)
}
