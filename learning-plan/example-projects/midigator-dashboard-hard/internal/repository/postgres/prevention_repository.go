package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// PreventionRepository implements domain.PreventionRepository (and the shared
// domain.CaseRepository) with raw SQL, mirroring ChargebackRepository.
type PreventionRepository struct {
	pool *pgxpool.Pool
}

func NewPreventionRepository(pool *pgxpool.Pool) *PreventionRepository {
	return &PreventionRepository{pool: pool}
}

// preventionEditable is the whitelist of columns a PATCH may set.
var preventionEditable = map[string]bool{
	"merchant_descriptor":    true,
	"resolution_type":        true,
	"prevention_case_number": true,
}

// preventionCols is the full projection; nullable text is COALESCE'd to ” so it
// scans into plain strings, while nullable dates/fks scan into pointers.
const preventionCols = `id, tenant_id, prevention_guid, event_guid,
	COALESCE(prevention_case_number,''), prevention_type,
	COALESCE(arn,''), COALESCE(mid,''), amount, currency,
	COALESCE(card_first_6,''), COALESCE(card_last_4,''),
	COALESCE(merchant_descriptor,''),
	prevention_timestamp, transaction_timestamp,
	COALESCE(resolution_type,''), resolution_submitted_at,
	COALESCE(order_guid,''), COALESCE(order_id,''), COALESCE(crm_account_guid,''),
	assigned_to, stage, is_hidden, created_at, updated_at`

func scanPrevention(row pgx.Row) (*domain.PreventionAlert, error) {
	var p domain.PreventionAlert
	err := row.Scan(
		&p.ID, &p.TenantID, &p.PreventionGUID, &p.EventGUID,
		&p.PreventionCaseNumber, &p.PreventionType,
		&p.ARN, &p.MID, &p.Amount, &p.Currency,
		&p.CardFirst6, &p.CardLast4,
		&p.MerchantDescriptor,
		&p.PreventionTimestamp, &p.TransactionTimestamp,
		&p.ResolutionType, &p.ResolutionSubmittedAt,
		&p.OrderGUID, &p.OrderID, &p.CRMAccountGUID,
		&p.AssignedTo, &p.Stage, &p.IsHidden, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ---- domain.CaseRepository --------------------------------------------------

func (r *PreventionRepository) CaseType() domain.CaseType { return domain.CasePrevention }

func (r *PreventionRepository) AllowedStages() []string {
	return []string{"new", "in_review", "action_taken", "resolved"}
}

func (r *PreventionRepository) CanResolve() bool { return true }

func (r *PreventionRepository) ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error) {
	return existsForTenant(ctx, r.pool, "prevention_alerts", tenantID, id)
}

func (r *PreventionRepository) GetStage(ctx context.Context, tenantID, id int64) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `SELECT stage FROM prevention_alerts WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.NotFound("prevention")
	}
	return stage, err
}

// ChangeStage moves the case and records the transition in one transaction.
func (r *PreventionRepository) ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error {
	return changeStageTx(ctx, r.pool, "prevention_alerts", domain.CasePrevention, tenantID, id, toStage, userID, notes)
}

func (r *PreventionRepository) Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE prevention_alerts SET assigned_to = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		assigneeID, tenantID, id)
	if err != nil {
		return fmt.Errorf("assign prevention: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("prevention")
	}
	return nil
}

func (r *PreventionRepository) SetHidden(ctx context.Context, tenantID, id int64, hidden bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE prevention_alerts SET is_hidden = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		hidden, tenantID, id)
	if err != nil {
		return fmt.Errorf("hide prevention: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("prevention")
	}
	return nil
}

// Resolve submits a resolution_type and moves the alert to the resolved stage.
func (r *PreventionRepository) Resolve(ctx context.Context, tenantID, id int64, resolution string, userID int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE prevention_alerts SET resolution_type = $1, resolution_submitted_at = now(), stage = 'resolved', updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		resolution, tenantID, id)
	if err != nil {
		return fmt.Errorf("resolve prevention: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("prevention")
	}
	return nil
}

func (r *PreventionRepository) ListSummaries(ctx context.Context, tenantID int64, f domain.CaseFilter) ([]domain.CaseSummary, int, error) {
	f.Normalize()
	where, args := preventionFilter(tenantID, f)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM prevention_alerts `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count preventions: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT id, tenant_id, prevention_guid AS guid, COALESCE(mid,'') AS mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		FROM prevention_alerts %s %s LIMIT $%d OFFSET $%d`, where, orderClause(f), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list preventions: %w", err)
	}
	defer rows.Close()

	var out []domain.CaseSummary
	for rows.Next() {
		s := domain.CaseSummary{Type: domain.CasePrevention}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.GUID, &s.MID, &s.Amount, &s.Currency, &s.Stage, &s.AssignedTo, &s.IsHidden, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *PreventionRepository) CountByStage(ctx context.Context, tenantID int64) (map[string]int, error) {
	return countByStage(ctx, r.pool, "prevention_alerts", tenantID)
}

// ---- domain.PreventionRepository (typed) -----------------------------------

func (r *PreventionRepository) Get(ctx context.Context, tenantID, id int64) (*domain.PreventionAlert, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+preventionCols+` FROM prevention_alerts WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	p, err := scanPrevention(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("prevention")
	}
	if err != nil {
		return nil, fmt.Errorf("get prevention: %w", err)
	}
	return p, nil
}

func (r *PreventionRepository) Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*domain.PreventionAlert, error) {
	ok, err := updateByID(ctx, r.pool, "prevention_alerts", tenantID, id, fields, preventionEditable)
	if err != nil {
		return nil, err
	}
	if !ok {
		if exists, _ := r.ExistsForTenant(ctx, tenantID, id); !exists {
			return nil, domain.NotFound("prevention")
		}
	}
	return r.Get(ctx, tenantID, id)
}

func (r *PreventionRepository) Insert(ctx context.Context, p *domain.PreventionAlert) (*domain.PreventionAlert, error) {
	const q = `INSERT INTO prevention_alerts
		(tenant_id, prevention_guid, event_guid, prevention_case_number, prevention_type, arn, mid, amount, currency,
		 card_first_6, card_last_4, merchant_descriptor,
		 prevention_timestamp, transaction_timestamp, resolution_type, resolution_submitted_at,
		 order_guid, order_id, crm_account_guid, assigned_to, stage, is_hidden)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING id, created_at, updated_at`
	stage := p.Stage
	if stage == "" {
		stage = "new"
	}
	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}
	err := r.pool.QueryRow(ctx, q,
		p.TenantID, p.PreventionGUID, p.EventGUID, nullable(p.PreventionCaseNumber), p.PreventionType, nullable(p.ARN), nullable(p.MID), p.Amount, currency,
		nullable(p.CardFirst6), nullable(p.CardLast4), nullable(p.MerchantDescriptor),
		p.PreventionTimestamp, p.TransactionTimestamp, nullable(p.ResolutionType), p.ResolutionSubmittedAt,
		nullable(p.OrderGUID), nullable(p.OrderID), nullable(p.CRMAccountGUID), p.AssignedTo, stage, p.IsHidden,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert prevention: %w", mapDBError(err))
	}
	p.Stage, p.Currency = stage, currency
	return p, nil
}

// preventionFilter builds the WHERE clause shared by list + count.
func preventionFilter(tenantID int64, f domain.CaseFilter) (string, []any) {
	conds := []string{"tenant_id = $1"}
	args := []any{tenantID}
	if !f.IncludeHidden {
		conds = append(conds, "is_hidden = FALSE")
	}
	if f.Stage != "" {
		args = append(args, f.Stage)
		conds = append(conds, fmt.Sprintf("stage = $%d", len(args)))
	}
	if f.MID != "" {
		args = append(args, f.MID)
		conds = append(conds, fmt.Sprintf("mid = $%d", len(args)))
	}
	if f.AssignedTo != nil {
		args = append(args, *f.AssignedTo)
		conds = append(conds, fmt.Sprintf("assigned_to = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		conds = append(conds, fmt.Sprintf("(prevention_guid ILIKE $%d OR order_id ILIKE $%d OR arn ILIKE $%d)", len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}
