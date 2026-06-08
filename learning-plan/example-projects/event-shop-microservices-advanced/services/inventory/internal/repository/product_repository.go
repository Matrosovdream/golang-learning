package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"eventshop/services/inventory/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS products (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL,
    price_cents BIGINT      NOT NULL CHECK (price_cents >= 0),
    stock       INTEGER     NOT NULL CHECK (stock >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// ProductRepository implements domain.Repository with raw SQL via pgx.
type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

func (r *ProductRepository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, schema)
	return err
}

func (r *ProductRepository) Create(ctx context.Context, p *domain.Product) error {
	const q = `INSERT INTO products (name, price_cents, stock) VALUES ($1, $2, $3) RETURNING id, created_at`
	if err := r.pool.QueryRow(ctx, q, p.Name, p.PriceCents, p.Stock).Scan(&p.ID, &p.CreatedAt); err != nil {
		return fmt.Errorf("create product: %w", err)
	}
	return nil
}

func (r *ProductRepository) GetByID(ctx context.Context, id int64) (*domain.Product, error) {
	const q = `SELECT id, name, price_cents, stock, created_at FROM products WHERE id = $1`
	var p domain.Product
	err := r.pool.QueryRow(ctx, q, id).Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return &p, nil
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, price_cents, stock, created_at FROM products ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	var out []domain.Product
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Reserve atomically decrements stock for every line (transaction + row locks)
// and returns the priced lines, or an error if any line is short or missing.
func (r *ProductRepository) Reserve(ctx context.Context, lines []domain.StockLine) ([]domain.ReservedLine, error) {
	sort.Slice(lines, func(i, j int) bool { return lines[i].ProductID < lines[j].ProductID })

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	out := make([]domain.ReservedLine, 0, len(lines))
	for _, l := range lines {
		var (
			id    int64
			name  string
			price int64
			stock int
		)
		err := tx.QueryRow(ctx,
			`SELECT id, name, price_cents, stock FROM products WHERE id = $1 FOR UPDATE`,
			l.ProductID,
		).Scan(&id, &name, &price, &stock)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("lock product %d: %w", l.ProductID, err)
		}
		if stock < l.Quantity {
			return nil, domain.InsufficientStockError{ProductID: id, Available: stock, Requested: l.Quantity}
		}
		if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock - $1 WHERE id = $2`, l.Quantity, id); err != nil {
			return nil, fmt.Errorf("decrement stock: %w", err)
		}
		out = append(out, domain.ReservedLine{
			ProductID:      id,
			ProductName:    name,
			Quantity:       l.Quantity,
			UnitPriceCents: price,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}
