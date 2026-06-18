package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// TenantRepository implements domain.TenantRepository.
type TenantRepository struct {
	pool *pgxpool.Pool
}

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{pool: pool}
}

const tenantCols = `id, name, slug, COALESCE(domain,''), COALESCE(midigator_api_secret,''),
	midigator_sandbox_mode, COALESCE(midigator_webhook_username,''), COALESCE(midigator_webhook_password,''),
	is_active, settings, created_at, updated_at`

func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	var t domain.Tenant
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Domain, &t.MidigatorAPISecret,
		&t.MidigatorSandboxMode, &t.WebhookUsername, &t.WebhookPassword,
		&t.IsActive, &t.Settings, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) Create(ctx context.Context, in domain.TenantInput) (*domain.Tenant, error) {
	const q = `INSERT INTO tenants (name, slug, domain, midigator_api_secret, midigator_sandbox_mode,
		midigator_webhook_username, midigator_webhook_password, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING ` + tenantCols
	row := r.pool.QueryRow(ctx, q, in.Name, in.Slug, nullable(in.Domain), nullable(in.MidigatorAPISecret),
		in.MidigatorSandboxMode, nullable(in.WebhookUsername), nullable(in.WebhookPassword), in.IsActive)
	t, err := scanTenant(row)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", mapDBError(err))
	}
	return t, nil
}

func (r *TenantRepository) Update(ctx context.Context, id int64, in domain.TenantInput) (*domain.Tenant, error) {
	const q = `UPDATE tenants SET name = $1, slug = $2, domain = $3, midigator_api_secret = $4,
		midigator_sandbox_mode = $5, midigator_webhook_username = $6, midigator_webhook_password = $7,
		is_active = $8, updated_at = now() WHERE id = $9 RETURNING ` + tenantCols
	row := r.pool.QueryRow(ctx, q, in.Name, in.Slug, nullable(in.Domain), nullable(in.MidigatorAPISecret),
		in.MidigatorSandboxMode, nullable(in.WebhookUsername), nullable(in.WebhookPassword), in.IsActive, id)
	t, err := scanTenant(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("tenant")
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", mapDBError(err))
	}
	return t, nil
}

func (r *TenantRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("tenant")
	}
	return nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id int64) (*domain.Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("tenant")
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return t, nil
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE slug = $1`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("tenant")
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant by slug: %w", err)
	}
	return t, nil
}

func (r *TenantRepository) List(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+tenantCols+` FROM tenants ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *TenantRepository) SetActive(ctx context.Context, id int64, active bool) (*domain.Tenant, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE tenants SET is_active = $1, updated_at = now() WHERE id = $2`, active, id)
	if err != nil {
		return nil, fmt.Errorf("set tenant active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.NotFound("tenant")
	}
	return r.GetByID(ctx, id)
}
