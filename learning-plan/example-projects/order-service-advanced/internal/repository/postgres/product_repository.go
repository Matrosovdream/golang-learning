package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"orderservice/internal/domain"
)

// ProductRepository implements domain.ProductRepository with raw SQL via pgx.
type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

const productColumns = `id, sku, name, price_cents, stock, created_at, updated_at`

func (r *ProductRepository) CreateProduct(ctx context.Context, p *domain.Product) error {
	const q = `
		INSERT INTO products (sku, name, price_cents, stock)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`
	err := r.pool.QueryRow(ctx, q, p.SKU, p.Name, p.PriceCents, p.Stock).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.ErrSKUTaken
		}
		return fmt.Errorf("create product: %w", err)
	}
	return nil
}

func (r *ProductRepository) GetProduct(ctx context.Context, id int64) (*domain.Product, error) {
	q := `SELECT ` + productColumns + ` FROM products WHERE id = $1`
	p, err := scanProduct(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}
	return p, nil
}

func (r *ProductRepository) ListProducts(ctx context.Context) ([]domain.Product, error) {
	q := `SELECT ` + productColumns + ` FROM products ORDER BY id`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var out []domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *ProductRepository) Restock(ctx context.Context, id int64, qty int) (*domain.Product, error) {
	q := `UPDATE products SET stock = stock + $1, updated_at = now() WHERE id = $2 RETURNING ` + productColumns
	p, err := scanProduct(r.pool.QueryRow(ctx, q, qty, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("restock: %w", err)
	}
	return p, nil
}

// row is satisfied by both pgx.Row and pgx.Rows so one scan helper serves both.
type row interface {
	Scan(dest ...any) error
}

func scanProduct(r row) (*domain.Product, error) {
	var p domain.Product
	if err := r.Scan(&p.ID, &p.SKU, &p.Name, &p.PriceCents, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
