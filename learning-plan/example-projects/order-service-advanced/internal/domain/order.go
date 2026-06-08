package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// OrderStatus is the order's position in its lifecycle.
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusShipped   OrderStatus = "shipped"
	StatusCancelled OrderStatus = "cancelled"
)

// allowedTransitions encodes the state machine: which statuses each status may
// move to. Terminal states map to an empty list.
var allowedTransitions = map[OrderStatus][]OrderStatus{
	StatusPending:   {StatusPaid, StatusCancelled},
	StatusPaid:      {StatusShipped, StatusCancelled},
	StatusShipped:   {},
	StatusCancelled: {},
}

// CanTransition reports whether moving from -> to is allowed.
func CanTransition(from, to OrderStatus) bool {
	for _, s := range allowedTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// ErrOrderNotFound is returned when an order does not exist.
var ErrOrderNotFound = errors.New("order not found")

// InsufficientStockError is returned when a checkout asks for more than is in
// stock. The handler maps it to HTTP 409.
type InsufficientStockError struct {
	ProductID int64
	Available int
	Requested int
}

func (e InsufficientStockError) Error() string {
	return fmt.Sprintf("insufficient stock for product %d: have %d, requested %d", e.ProductID, e.Available, e.Requested)
}

// InvalidTransitionError is returned when a status change is not allowed by the
// state machine. The handler maps it to HTTP 409.
type InvalidTransitionError struct {
	From OrderStatus
	To   OrderStatus
}

func (e InvalidTransitionError) Error() string {
	return fmt.Sprintf("cannot change order status from %s to %s", e.From, e.To)
}

// OrderLine is a requested (product, quantity) at checkout time.
type OrderLine struct {
	ProductID int64
	Quantity  int
}

// OrderItem is a line persisted on an order, with the price snapshotted at
// purchase time so later price changes don't alter past orders.
type OrderItem struct {
	ID             int64
	OrderID        int64
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// Order is a placed order and its items.
type Order struct {
	ID         int64
	Status     OrderStatus
	TotalCents int64
	Items      []OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// OrderRepository is the storage contract for orders. CreateOrder and
// SetOrderStatus are transactional (see the postgres implementation).
type OrderRepository interface {
	CreateOrder(ctx context.Context, lines []OrderLine) (*Order, error)
	GetOrder(ctx context.Context, id int64) (*Order, error)
	ListOrders(ctx context.Context) ([]Order, error)
	SetOrderStatus(ctx context.Context, id int64, to OrderStatus) (*Order, error)
}
