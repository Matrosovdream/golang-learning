package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// SettingsRepository implements domain.SettingsRepository with raw SQL. The
// "group" column is a reserved word and is always quoted. tenant_id is nullable
// (null = a global default), so List branches on a nil tenant; value and
// description are nullable text COALESCE'd to ” so they scan into plain strings.
type SettingsRepository struct {
	pool *pgxpool.Pool
}

func NewSettingsRepository(pool *pgxpool.Pool) *SettingsRepository {
	return &SettingsRepository{pool: pool}
}

// settingCols is the full projection. "group" is quoted; value/description are
// nullable text COALESCE'd to ”.
const settingCols = `id, tenant_id, key, COALESCE(value,''), type, "group",
	COALESCE(description,''), created_at, updated_at`

func scanSetting(row pgx.Row) (*domain.SiteSetting, error) {
	var s domain.SiteSetting
	err := row.Scan(
		&s.ID, &s.TenantID, &s.Key, &s.Value, &s.Type, &s.Group,
		&s.Description, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns a tenant's settings, or the global defaults when tenantID is nil.
func (r *SettingsRepository) List(ctx context.Context, tenantID *int64) ([]domain.SiteSetting, error) {
	var rows pgx.Rows
	var err error
	if tenantID == nil {
		rows, err = r.pool.Query(ctx,
			`SELECT `+settingCols+` FROM site_settings WHERE tenant_id IS NULL ORDER BY "group" ASC, key ASC`)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT `+settingCols+` FROM site_settings WHERE tenant_id = $1 ORDER BY "group" ASC, key ASC`,
			*tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer rows.Close()

	var out []domain.SiteSetting
	for rows.Next() {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Upsert inserts or updates a setting keyed by (tenant_id, key).
func (r *SettingsRepository) Upsert(ctx context.Context, in domain.SiteSettingInput) (*domain.SiteSetting, error) {
	typ := in.Type
	if typ == "" {
		typ = "string"
	}
	group := in.Group
	if group == "" {
		group = "general"
	}
	const q = `INSERT INTO site_settings (tenant_id, key, value, type, "group", description)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value, type = EXCLUDED.type, "group" = EXCLUDED."group",
		    description = EXCLUDED.description, updated_at = now()
		RETURNING ` + settingCols
	row := r.pool.QueryRow(ctx, q, in.TenantID, in.Key, nullable(in.Value), typ, group, nullable(in.Description))
	s, err := scanSetting(row)
	if err != nil {
		return nil, fmt.Errorf("upsert setting: %w", mapDBError(err))
	}
	return s, nil
}

// Delete removes a setting by key within the given tenant (or the global scope
// when tenantID is nil).
func (r *SettingsRepository) Delete(ctx context.Context, tenantID *int64, key string) error {
	var tag pgconn.CommandTag
	var err error
	if tenantID == nil {
		tag, err = r.pool.Exec(ctx, `DELETE FROM site_settings WHERE tenant_id IS NULL AND key = $1`, key)
	} else {
		tag, err = r.pool.Exec(ctx, `DELETE FROM site_settings WHERE tenant_id = $1 AND key = $2`, *tenantID, key)
	}
	if err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("setting")
	}
	return nil
}
