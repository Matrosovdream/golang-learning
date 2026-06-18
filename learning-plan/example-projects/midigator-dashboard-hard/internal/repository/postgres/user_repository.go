package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// UserRepository implements domain.UserRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

const userCols = `id, tenant_id, email, password, name, COALESCE(avatar,''), COALESCE(pin,''),
	is_active, is_platform_admin, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Name, &u.Avatar, &u.PINHash,
		&u.IsActive, &u.IsPlatformAdmin, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) Create(ctx context.Context, in domain.UserInput) (*domain.User, error) {
	const q = `INSERT INTO users (tenant_id, email, password, name, pin, is_active, is_platform_admin)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING ` + userCols
	row := r.pool.QueryRow(ctx, q, in.TenantID, in.Email, in.PasswordHash, in.Name, nullable(in.PINHash), in.IsActive, in.IsPlatformAdmin)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", mapDBError(err))
	}
	return u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("user")
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("user")
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *UserRepository) ListByTenant(ctx context.Context, tenantID int64) ([]domain.User, error) {
	return r.queryUsers(ctx, `SELECT `+userCols+` FROM users WHERE tenant_id = $1 ORDER BY name`, tenantID)
}

func (r *UserRepository) ListAll(ctx context.Context) ([]domain.User, error) {
	return r.queryUsers(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
}

func (r *UserRepository) queryUsers(ctx context.Context, q string, args ...any) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (r *UserRepository) SetActive(ctx context.Context, id int64, active bool) (*domain.User, error) {
	return r.setBool(ctx, id, "is_active", active)
}

func (r *UserRepository) SetPlatformAdmin(ctx context.Context, id int64, isAdmin bool) (*domain.User, error) {
	return r.setBool(ctx, id, "is_platform_admin", isAdmin)
}

func (r *UserRepository) setBool(ctx context.Context, id int64, col string, val bool) (*domain.User, error) {
	tag, err := r.pool.Exec(ctx, fmt.Sprintf(`UPDATE users SET %s = $1, updated_at = now() WHERE id = $2`, col), val, id)
	if err != nil {
		return nil, fmt.Errorf("update user %s: %w", col, err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.NotFound("user")
	}
	return r.GetByID(ctx, id)
}

func (r *UserRepository) TouchLastLogin(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}
