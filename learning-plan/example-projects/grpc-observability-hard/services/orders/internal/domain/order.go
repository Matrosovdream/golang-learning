// Package domain is the orders core. Orders snapshot the name/price they saw at
// checkout — no foreign key into catalog's database (database-per-service).
package domain

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("order not found")

type OrderItem struct {
	ProductID  int64
	Quantity   int32
	Name       string
	PriceCents int64
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
