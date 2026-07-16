// Package repository implements domain.ProductRepository against Postgres (pgx).
// This is the real database-per-service store — catalog owns catalog-db, and no
// other service touches it.
package repository

import (
	"context"
	"errors"

	"grpcobs/services/catalog/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema is applied on startup (idempotent).
const Schema = `
CREATE TABLE IF NOT EXISTS products (
	id          BIGSERIAL PRIMARY KEY,
	name        TEXT    NOT NULL,
	price_cents BIGINT  NOT NULL,
	stock       INTEGER NOT NULL DEFAULT 0
);`

type ProductRepo struct {
	pool *pgxpool.Pool // a connection pool; pgx hands out connections per query
}

func NewProductRepo(pool *pgxpool.Pool) *ProductRepo { return &ProductRepo{pool: pool} }

func (r *ProductRepo) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	// QueryRow + RETURNING gets the generated id back in one round trip.
	err := r.pool.QueryRow(ctx,
		`INSERT INTO products (name, price_cents, stock) VALUES ($1,$2,$3) RETURNING id`,
		p.Name, p.PriceCents, p.Stock).Scan(&p.ID)
	return p, err
}

func (r *ProductRepo) Get(ctx context.Context, id int64) (domain.Product, error) {
	var p domain.Product
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, price_cents, stock FROM products WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, domain.ErrNotFound
	}
	return p, err
}

func (r *ProductRepo) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, price_cents, stock FROM products ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // always close rows to return the connection to the pool

	var out []domain.Product
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Reserve runs in a transaction: lock the row (FOR UPDATE), check stock, decrement.
// The lock serialises concurrent reservations of the same product so two orders
// can't both read stock=1 and both succeed.
func (r *ProductRepo) Reserve(ctx context.Context, id int64, qty int32) (domain.Product, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Product{}, err
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit; safety net otherwise

	var p domain.Product
	err = tx.QueryRow(ctx,
		`SELECT id, name, price_cents, stock FROM products WHERE id = $1 FOR UPDATE`, id).
		Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Product{}, err
	}
	if p.Stock < qty {
		return domain.Product{}, domain.ErrInsufficientStock
	}
	p.Stock -= qty
	if _, err := tx.Exec(ctx, `UPDATE products SET stock = $1 WHERE id = $2`, p.Stock, id); err != nil {
		return domain.Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Product{}, err
	}
	return p, nil
}
