package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"urlshortener/internal/domain"
)

// LinkRepository implements domain.LinkRepository on top of Postgres using
// raw SQL (no ORM) through a pgx connection pool.
type LinkRepository struct {
	pool *pgxpool.Pool
}

// NewLinkRepository wires the repository to a pgx connection pool.
func NewLinkRepository(pool *pgxpool.Pool) *LinkRepository {
	return &LinkRepository{pool: pool}
}

// Create inserts a new link. A duplicate code surfaces as
// domain.ErrDuplicateCode so the service can retry with a fresh code.
func (r *LinkRepository) Create(ctx context.Context, link *domain.Link) error {
	const q = `
		INSERT INTO links (code, long_url)
		VALUES ($1, $2)
		RETURNING id, clicks, created_at`
	err := r.pool.QueryRow(ctx, q, link.Code, link.LongURL).
		Scan(&link.ID, &link.Clicks, &link.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.ErrDuplicateCode
		}
		return fmt.Errorf("insert link: %w", err)
	}
	return nil
}

// GetByCode returns the link for a code, or domain.ErrNotFound.
func (r *LinkRepository) GetByCode(ctx context.Context, code string) (*domain.Link, error) {
	const q = `
		SELECT id, code, long_url, clicks, created_at
		FROM links
		WHERE code = $1`
	var l domain.Link
	err := r.pool.QueryRow(ctx, q, code).
		Scan(&l.ID, &l.Code, &l.LongURL, &l.Clicks, &l.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get link: %w", err)
	}
	return &l, nil
}

// IncrementClicks bumps the click counter for a code.
func (r *LinkRepository) IncrementClicks(ctx context.Context, code string) error {
	const q = `UPDATE links SET clicks = clicks + 1 WHERE code = $1`
	tag, err := r.pool.Exec(ctx, q, code)
	if err != nil {
		return fmt.Errorf("increment clicks: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
