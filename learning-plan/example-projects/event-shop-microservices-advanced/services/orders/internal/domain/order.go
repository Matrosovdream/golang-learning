package domain

import (
	"context"
	"errors"
	"time"
)

// A package-level var. errors.New builds a sentinel error value; callers compare
// against it with errors.Is.
var ErrNotFound = errors.New("order not found")

type ValidationError struct {
	Field   string
	Message string
}

// Defining Error() string makes ValidationError satisfy the built-in `error`
// interface — implicitly, with no "implements" keyword. The value receiver (e)
// means both ValidationError and *ValidationError satisfy error.
func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

type OrderItem struct {
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// time.Time is a struct value (not a pointer). []OrderItem is a slice field
// whose zero value is nil until something is appended to it.
type Order struct {
	ID         int64
	UserID     int64
	Status     string
	TotalCents int64
	Items      []OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// An interface lists method signatures; any type with those methods satisfies
// it automatically. The *Order params/returns pass orders by pointer (no copy).
type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id int64) (*Order, error)
	SetReserved(ctx context.Context, orderID int64, items []OrderItem, totalCents int64) error
	SetStatus(ctx context.Context, orderID int64, status string) error
}
