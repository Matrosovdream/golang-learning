package domain

import (
	"context"
	"errors"
	"time"
)

// Product errors.
var (
	ErrProductNotFound = errors.New("product not found")
	ErrSKUTaken        = errors.New("sku already exists")
)

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// Product is a sellable item. Money is stored as integer cents to avoid any
// floating-point rounding.
type Product struct {
	ID         int64
	SKU        string
	Name       string
	PriceCents int64
	Stock      int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ProductRepository is the storage contract for products.
type ProductRepository interface {
	CreateProduct(ctx context.Context, p *Product) error
	GetProduct(ctx context.Context, id int64) (*Product, error)
	ListProducts(ctx context.Context) ([]Product, error)
	Restock(ctx context.Context, id int64, qty int) (*Product, error)
}
