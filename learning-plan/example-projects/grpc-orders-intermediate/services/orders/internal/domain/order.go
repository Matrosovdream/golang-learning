// Package domain is the orders service core. Note there is no product entity here
// and no foreign key to catalog: each order snapshots the name/price it saw at
// checkout. That's database-per-service — services don't reach into each other's data.
package domain

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("order not found")

type OrderItem struct {
	ProductID  int64
	Quantity   int32
	Name       string // snapshot from catalog
	PriceCents int64  // snapshot from catalog
}

type Order struct {
	ID         int64
	Status     string
	TotalCents int64
	Items      []OrderItem
}

type OrderRepository interface {
	Create(ctx context.Context, o Order) (Order, error)
	Get(ctx context.Context, id int64) (Order, error)
}
