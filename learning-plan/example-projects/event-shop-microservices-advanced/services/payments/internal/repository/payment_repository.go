package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"eventshop/services/payments/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS payments (
    id           BIGSERIAL   PRIMARY KEY,
    order_id     BIGINT      NOT NULL,
    amount_cents BIGINT      NOT NULL,
    status       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// PaymentRepository implements domain.Repository with raw SQL via pgx.
type PaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

func (r *PaymentRepository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, schema)
	return err
}

func (r *PaymentRepository) Create(ctx context.Context, p *domain.Payment) error {
	const q = `INSERT INTO payments (order_id, amount_cents, status) VALUES ($1, $2, $3) RETURNING id, created_at`
	if err := r.pool.QueryRow(ctx, q, p.OrderID, p.AmountCents, p.Status).Scan(&p.ID, &p.CreatedAt); err != nil {
		return fmt.Errorf("create payment: %w", err)
	}
	return nil
}
