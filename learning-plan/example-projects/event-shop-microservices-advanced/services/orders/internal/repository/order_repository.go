package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"eventshop/services/orders/internal/domain"
)

// A raw string literal (back-ticks): spans multiple lines, no escapes processed.
const schema = `
CREATE TABLE IF NOT EXISTS orders (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    status      TEXT        NOT NULL,
    total_cents BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
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

// Holds a *pgxpool.Pool — a pointer to the shared connection pool.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// Constructor convention: return &OrderRepository{...}, a pointer to a new value.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// Pointer receiver (r *OrderRepository) on every method — all share one repo.
func (r *OrderRepository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, schema) // _ discards the first return value
	return err
}

func (r *OrderRepository) Create(ctx context.Context, o *domain.Order) error {
	const q = `INSERT INTO orders (user_id, status) VALUES ($1, $2) RETURNING id, total_cents, created_at, updated_at`
	// o is a *domain.Order; Scan(&o.ID, ...) writes through the pointer, so the
	// caller sees the populated order. &o.ID is the address of that field.
	if err := r.pool.QueryRow(ctx, q, o.UserID, o.Status).
		Scan(&o.ID, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt); err != nil {
		// %w wraps err so callers can unwrap it later with errors.Is / errors.As.
		return fmt.Errorf("insert order: %w", err)
	}
	return nil
}

func (r *OrderRepository) GetByID(ctx context.Context, id int64) (*domain.Order, error) {
	var o domain.Order // a value; &o below takes its address
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, status, total_cents, created_at, updated_at FROM orders WHERE id = $1`, id,
	).Scan(&o.ID, &o.UserID, &o.Status, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt)
	// errors.Is compares against a sentinel, unwrapping %w-wrapped errors.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT product_id, product_name, quantity, unit_price_cents FROM order_items WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, fmt.Errorf("get order items: %w", err)
	}
	defer rows.Close() // deferred: runs on every return path
	for rows.Next() {
		var it domain.OrderItem
		if err := rows.Scan(&it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPriceCents); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it) // append grows the slice, returns a new header
	}
	return &o, rows.Err() // &o: return the address of the local (escape analysis keeps it alive)
}

func (r *OrderRepository) SetReserved(ctx context.Context, orderID int64, items []domain.OrderItem, totalCents int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) // deferred rollback becomes a no-op once Commit succeeds

	tag, err := tx.Exec(ctx, `UPDATE orders SET total_cents = $1, updated_at = now() WHERE id = $2`, totalCents, orderID)
	if err != nil {
		return fmt.Errorf("update total: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, orderID); err != nil {
		return fmt.Errorf("clear items: %w", err)
	}
	for _, it := range items { // two-value range: index discarded, value is `it`
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price_cents)
			 VALUES ($1, $2, $3, $4, $5)`,
			orderID, it.ProductID, it.ProductName, it.Quantity, it.UnitPriceCents,
		); err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *OrderRepository) SetStatus(ctx context.Context, orderID int64, status string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE orders SET status = $1, updated_at = now() WHERE id = $2`, status, orderID)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
