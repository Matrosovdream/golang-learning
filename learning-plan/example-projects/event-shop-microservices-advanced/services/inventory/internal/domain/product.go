package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel error value, compared against with errors.Is.
var ErrProductNotFound = errors.New("product not found")

type ValidationError struct {
	Field   string
	Message string
}

// Error() string makes this type satisfy the built-in error interface (value receiver).
func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// A second custom error type — it carries data fields, not just a message.
type InsufficientStockError struct {
	ProductID int64
	Available int
	Requested int
}

// fmt.Sprintf formats and returns a string; %d is the verb for an integer.
func (e InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for product %d: have %d, requested %d", e.ProductID, e.Available, e.Requested)
}

type Product struct {
	ID         int64
	Name       string
	PriceCents int64
	Stock      int
	CreatedAt  time.Time
}

type StockLine struct {
	ProductID int64
	Quantity  int
}

type ReservedLine struct {
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// The methods return slices ([]Product, []ReservedLine) or a pointer (*Product),
// each paired with an error — the usual Go return shapes.
type Repository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id int64) (*Product, error)
	List(ctx context.Context) ([]Product, error)
	Reserve(ctx context.Context, lines []StockLine) ([]ReservedLine, error)
}
