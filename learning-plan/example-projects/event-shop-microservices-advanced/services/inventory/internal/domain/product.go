package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrProductNotFound is returned when a product does not exist.
var ErrProductNotFound = errors.New("product not found")

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

// InsufficientStockError is returned when a reservation exceeds stock.
type InsufficientStockError struct {
	ProductID int64
	Available int
	Requested int
}

func (e InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for product %d: have %d, requested %d", e.ProductID, e.Available, e.Requested)
}

// Product is the core entity. Money is integer cents.
type Product struct {
	ID         int64
	Name       string
	PriceCents int64
	Stock      int
	CreatedAt  time.Time
}

// StockLine is a requested (product, quantity) to reserve.
type StockLine struct {
	ProductID int64
	Quantity  int
}

// ReservedLine is a reserved line with the price snapshotted at reservation.
type ReservedLine struct {
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// Repository is the storage contract for the inventory.
type Repository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id int64) (*Product, error)
	List(ctx context.Context) ([]Product, error)
	Reserve(ctx context.Context, lines []StockLine) ([]ReservedLine, error)
}
