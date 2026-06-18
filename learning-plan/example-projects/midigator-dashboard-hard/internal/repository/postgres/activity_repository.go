package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// ActivityRepository implements domain.ActivityRepository (the audit log).
type ActivityRepository struct {
	pool *pgxpool.Pool
}

func NewActivityRepository(pool *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{pool: pool}
}

func (r *ActivityRepository) Log(ctx context.Context, e *domain.ActivityEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO activity_log (tenant_id, user_id, loggable_type, loggable_id, action, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		e.TenantID, e.UserID, e.LoggableType, e.LoggableID, e.Action, e.Metadata)
	return err
}

const activityCols = `a.id, a.tenant_id, a.user_id, COALESCE(u.name,''), a.loggable_type, a.loggable_id,
	a.action, a.metadata, a.created_at`

func (r *ActivityRepository) ListByTenant(ctx context.Context, tenantID int64, f domain.ActivityFilter) ([]domain.ActivityEntry, int, error) {
	f.Normalize()
	conds := []string{"a.tenant_id = $1"}
	args := []any{tenantID}
	if f.UserID != nil {
		args = append(args, *f.UserID)
		conds = append(conds, fmt.Sprintf("a.user_id = $%d", len(args)))
	}
	if f.Action != "" {
		args = append(args, f.Action)
		conds = append(conds, fmt.Sprintf("a.action = $%d", len(args)))
	}
	return r.query(ctx, strings.Join(conds, " AND "), args, f.Limit, f.Offset)
}

func (r *ActivityRepository) ListAll(ctx context.Context, f domain.ActivityFilter) ([]domain.ActivityEntry, int, error) {
	f.Normalize()
	conds := []string{"1 = 1"}
	var args []any
	if f.TenantID != nil {
		args = append(args, *f.TenantID)
		conds = append(conds, fmt.Sprintf("a.tenant_id = $%d", len(args)))
	}
	if f.UserID != nil {
		args = append(args, *f.UserID)
		conds = append(conds, fmt.Sprintf("a.user_id = $%d", len(args)))
	}
	if f.Action != "" {
		args = append(args, f.Action)
		conds = append(conds, fmt.Sprintf("a.action = $%d", len(args)))
	}
	return r.query(ctx, strings.Join(conds, " AND "), args, f.Limit, f.Offset)
}

func (r *ActivityRepository) query(ctx context.Context, where string, args []any, limit, offset int) ([]domain.ActivityEntry, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_log a WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity: %w", err)
	}
	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT %s FROM activity_log a LEFT JOIN users u ON u.id = a.user_id
		WHERE %s ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d`, activityCols, where, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list activity: %w", err)
	}
	defer rows.Close()
	var out []domain.ActivityEntry
	for rows.Next() {
		var e domain.ActivityEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.UserName, &e.LoggableType, &e.LoggableID, &e.Action, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
