package domain

import (
	"context"
	"time"
)

type Payment struct {
	ID          int64
	OrderID     int64
	AmountCents int64
	Status      string
	CreatedAt   time.Time
}

// A one-method interface. The *Payment param is a pointer, so Create can write
// the generated ID back into the caller's value.
type Repository interface {
	Create(ctx context.Context, p *Payment) error
}
