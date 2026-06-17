package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"authservice/internal/domain"

	"github.com/jackc/pgx/pgconn"
	"github.com/jmoiron/sqlx"
)

type userRow struct {
	ID           int64     `db:"id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	CreatedAt    time.Time `db:"created_at"`
}

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, u *domain.User) error {

	const q = `
		INSERT INTO users (email, password_hash)
		VALUE ($1, $2)
		RETURNING id, created_at`
	err := r.db.QueryRowxContext(ctx, q, u.Email, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrEmailTaken
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil

}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {

	const q = `SELECT id, email, password_hash, created_at FROM users WHERE email=$1`
	var row userRow

	if err := r.db.GetContext(ctx, &row, q, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	u := toDomainUser(row)
	return &u, nil

}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {

	const q = `SELECT id, email, password_hash, created_at FROM users WHERE id = $1`
	var row userRow

	if err := r.db.GetContext(ctx, &row, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	u := toDomainUser(row)
	return &u, nil

}

func toDomainUser(row userRow) domain.User {

	return domain.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
	}

}
