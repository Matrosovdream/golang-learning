package domain

import (
	"context"
	"time"
)

// Payment is a (simulated) charge against an order.
type Payment struct {
	ID          int64
	OrderID     int64
	AmountCents int64
	Status      string
	CreatedAt   time.Time
}

// Repository is the storage contract for payments.
type Repository interface {
	Create(ctx context.Context, p *Payment) error
}
