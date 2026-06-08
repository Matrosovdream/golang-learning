package domain

import (
	"context"
	"errors"
	"time"
)

// ErrOrderNotFound is returned when an order does not exist.
var ErrOrderNotFound = errors.New("order not found")

// OrderItem is a line on an order. Note it stores a product_id and a *snapshot*
// of the name/price — it cannot foreign-key to the catalog's products table,
// because that table lives in a different service's database.
type OrderItem struct {
	ProductID      int64
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// Order is a placed order owned by this service.
type Order struct {
	ID         int64
	UserID     int64
	Status     string
	TotalCents int64
	Items      []OrderItem
	CreatedAt  time.Time
}

// Repository is the storage contract for orders.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id int64) (*Order, error)
}
