package domain

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when an order does not exist.
var ErrNotFound = errors.New("order not found")

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

// Order status lifecycle (driven by events):
//
//	pending ──stock.rejected──► cancelled
//	   │
//	   └──stock.reserved (fills items+total) ──payment.settled──► confirmed
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

// OrderItem is a priced line, populated once inventory reserves stock.
type OrderItem struct {
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// Order is owned by this service. Its items/total/status fill in over time as
// events arrive — the order is eventually consistent.
type Order struct {
	ID         int64
	UserID     int64
	Status     string
	TotalCents int64
	Items      []OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Repository is the storage contract for orders.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id int64) (*Order, error)
	SetReserved(ctx context.Context, orderID int64, items []OrderItem, totalCents int64) error
	SetStatus(ctx context.Context, orderID int64, status string) error
}
