package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// ReportingRepository implements domain.ReportingRepository. Every query reads
// the four case tables through a shared UNION ALL so the dashboard, manager
// plane, and platform overview stay type-agnostic.
type ReportingRepository struct {
	pool *pgxpool.Pool
}

func NewReportingRepository(pool *pgxpool.Pool) *ReportingRepository {
	return &ReportingRepository{pool: pool}
}

// caseUnion is the normalized projection across all four case types.
const caseUnion = `
	SELECT 'chargeback' AS type, id, tenant_id, chargeback_guid AS guid, COALESCE(mid,'') AS mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at FROM chargebacks
	UNION ALL
	SELECT 'prevention', id, tenant_id, prevention_guid, COALESCE(mid,''), amount, currency, stage, assigned_to, is_hidden, created_at, updated_at FROM prevention_alerts
	UNION ALL
	SELECT 'order_validation', id, tenant_id, order_validation_guid, COALESCE(mid,''), amount, currency, stage, assigned_to, is_hidden, created_at, updated_at FROM order_validations
	UNION ALL
	SELECT 'rdr', id, tenant_id, rdr_guid, '', amount, currency, stage, assigned_to, is_hidden, created_at, updated_at FROM rdr_cases`

func (r *ReportingRepository) TenantStageCounts(ctx context.Context, tenantID int64) ([]domain.TypeStageCount, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT type, stage, COUNT(*) FROM (`+caseUnion+`) u WHERE tenant_id = $1 GROUP BY type, stage ORDER BY type, stage`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("stage counts: %w", err)
	}
	defer rows.Close()
	var out []domain.TypeStageCount
	for rows.Next() {
		var c domain.TypeStageCount
		if err := rows.Scan(&c.Type, &c.Stage, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ReportingRepository) ManagerLeaderboard(ctx context.Context, tenantID int64) ([]domain.ManagerStat, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.name, u.email,
		        COUNT(c.id) FILTER (WHERE c.stage <> 'resolved') AS open,
		        COUNT(c.id) FILTER (WHERE c.stage = 'resolved') AS resolved,
		        mp.score
		 FROM users u
		 LEFT JOIN (`+caseUnion+`) c ON c.assigned_to = u.id AND c.tenant_id = $1
		 LEFT JOIN manager_profiles mp ON mp.user_id = u.id
		 WHERE u.tenant_id = $1
		 GROUP BY u.id, u.name, u.email, mp.score
		 ORDER BY resolved DESC, open DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("manager leaderboard: %w", err)
	}
	defer rows.Close()
	var out []domain.ManagerStat
	for rows.Next() {
		var m domain.ManagerStat
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Open, &m.Resolved, &m.Score); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ReportingRepository) UnassignedCases(ctx context.Context, tenantID int64, limit int) ([]domain.CaseSummary, error) {
	return r.scanSummaries(ctx,
		`SELECT type, id, tenant_id, guid, mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		 FROM (`+caseUnion+`) u WHERE tenant_id = $1 AND assigned_to IS NULL AND is_hidden = FALSE
		 ORDER BY created_at DESC LIMIT $2`, tenantID, clampLimit(limit))
}

func (r *ReportingRepository) CasesInStage(ctx context.Context, tenantID int64, stage string, limit int) ([]domain.CaseSummary, error) {
	return r.scanSummaries(ctx,
		`SELECT type, id, tenant_id, guid, mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		 FROM (`+caseUnion+`) u WHERE tenant_id = $1 AND stage = $2 AND is_hidden = FALSE
		 ORDER BY created_at DESC LIMIT $3`, tenantID, stage, clampLimit(limit))
}

func (r *ReportingRepository) scanSummaries(ctx context.Context, q string, args ...any) ([]domain.CaseSummary, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("case summaries: %w", err)
	}
	defer rows.Close()
	var out []domain.CaseSummary
	for rows.Next() {
		var s domain.CaseSummary
		var t string
		if err := rows.Scan(&t, &s.ID, &s.TenantID, &s.GUID, &s.MID, &s.Amount, &s.Currency, &s.Stage, &s.AssignedTo, &s.IsHidden, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Type = domain.CaseType(t)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ReportingRepository) PlatformTotals(ctx context.Context) (*domain.PlatformStats, error) {
	var s domain.PlatformStats
	err := r.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM tenants),
		        (SELECT COUNT(*) FROM users),
		        (SELECT COUNT(*) FROM chargebacks),
		        (SELECT COUNT(*) FROM prevention_alerts),
		        (SELECT COUNT(*) FROM orders),
		        (SELECT COUNT(*) FROM (`+caseUnion+`) u WHERE stage <> 'resolved')`).
		Scan(&s.Tenants, &s.Users, &s.Chargebacks, &s.Preventions, &s.Orders, &s.OpenCases)
	if err != nil {
		return nil, fmt.Errorf("platform totals: %w", err)
	}
	return &s, nil
}

func (r *ReportingRepository) TenantOverview(ctx context.Context, tenantID int64) (*domain.TenantOverview, error) {
	o := domain.TenantOverview{TenantID: tenantID}
	err := r.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM users WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM chargebacks WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM prevention_alerts WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM order_validations WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM rdr_cases WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM orders WHERE tenant_id = $1),
		        (SELECT COUNT(*) FROM (`+caseUnion+`) u WHERE tenant_id = $1 AND stage <> 'resolved')`, tenantID).
		Scan(&o.Users, &o.Chargebacks, &o.Preventions, &o.OrderValidations, &o.RdrCases, &o.Orders, &o.OpenCases)
	if err != nil {
		return nil, fmt.Errorf("tenant overview: %w", err)
	}
	return &o, nil
}

func clampLimit(n int) int {
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}
