package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shop/services/orders/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS orders (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    status      TEXT        NOT NULL,
    total_cents BIGINT      NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS order_items (
    id               BIGSERIAL PRIMARY KEY,
    order_id         BIGINT    NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id       BIGINT    NOT NULL,
    product_name     TEXT      NOT NULL,
    quantity         INTEGER   NOT NULL,
    unit_price_cents BIGINT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);`

// OrderRepository implements domain.Repository with raw SQL via pgx.
type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, schema)
	return err
}

func (r *OrderRepository) Create(ctx context.Context, o *domain.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, status, total_cents) VALUES ($1, $2, $3) RETURNING id, created_at`,
		o.UserID, o.Status, o.TotalCents,
	).Scan(&o.ID, &o.CreatedAt); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	for _, it := range o.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price_cents)
			 VALUES ($1, $2, $3, $4, $5)`,
			o.ID, it.ProductID, it.ProductName, it.Quantity, it.UnitPriceCents,
		); err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *OrderRepository) GetByID(ctx context.Context, id int64) (*domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, total_cents, created_at FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.TotalCents, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT product_id, product_name, quantity, unit_price_cents FROM order_items WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("get order items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.OrderItem
		if err := rows.Scan(&it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPriceCents); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it)
	}
	return &o, rows.Err()
}
