// Package repository implements domain.OrderRepository against Postgres. An order
// and its items are written in one transaction so a half-written order can't exist.
package repository

import (
	"context"
	"errors"

	"grpcobs/services/orders/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Schema = `
CREATE TABLE IF NOT EXISTS orders (
	id          BIGSERIAL PRIMARY KEY,
	status      TEXT   NOT NULL,
	total_cents BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS order_items (
	id          BIGSERIAL PRIMARY KEY,
	order_id    BIGINT  NOT NULL REFERENCES orders(id),
	product_id  BIGINT  NOT NULL,
	quantity    INTEGER NOT NULL,
	name        TEXT    NOT NULL,
	price_cents BIGINT  NOT NULL
);`

type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo { return &OrderRepo{pool: pool} }

func (r *OrderRepo) Create(ctx context.Context, o domain.Order) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, err
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx,
		`INSERT INTO orders (status, total_cents) VALUES ($1,$2) RETURNING id`,
		o.Status, o.TotalCents).Scan(&o.ID); err != nil {
		return domain.Order{}, err
	}
	for _, it := range o.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, name, price_cents)
			 VALUES ($1,$2,$3,$4,$5)`,
			o.ID, it.ProductID, it.Quantity, it.Name, it.PriceCents); err != nil {
			return domain.Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

func (r *OrderRepo) Get(ctx context.Context, id int64) (domain.Order, error) {
	var o domain.Order
	err := r.pool.QueryRow(ctx,
		`SELECT id, status, total_cents FROM orders WHERE id = $1`, id).
		Scan(&o.ID, &o.Status, &o.TotalCents)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT product_id, quantity, name, price_cents FROM order_items WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return domain.Order{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var it domain.OrderItem
		if err := rows.Scan(&it.ProductID, &it.Quantity, &it.Name, &it.PriceCents); err != nil {
			return domain.Order{}, err
		}
		o.Items = append(o.Items, it)
	}
	return o, rows.Err()
}
