// Package domain is the catalog service core: the Product entity, the repository
// interface it needs, and its sentinel errors. It imports nothing outward — no
// gRPC, no storage driver.
package domain

import (
	"context"
	"errors"
)

// Sentinel errors the server layer maps to gRPC status codes.
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

// ProductRepository is the storage contract. The consumer (this package) defines
// it; the repository package implements it. Swap the implementation freely.
type ProductRepository interface {
	Create(ctx context.Context, p Product) (Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	List(ctx context.Context) ([]Product, error)
	// Reserve decrements stock by qty and returns the updated product, or
	// ErrInsufficientStock / ErrNotFound.
	Reserve(ctx context.Context, id int64, qty int32) (Product, error)
}
