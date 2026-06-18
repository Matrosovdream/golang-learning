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

// RdrRepository implements domain.RdrRepository (and the shared
// domain.CaseRepository) with raw SQL. It mirrors ChargebackRepository; the
// rdr_cases table has no `mid` column, so ListSummaries selects ” for mid and
// the MID filter is skipped.
type RdrRepository struct {
	pool *pgxpool.Pool
}

func NewRdrRepository(pool *pgxpool.Pool) *RdrRepository {
	return &RdrRepository{pool: pool}
}

// rdrEditable is the whitelist of columns a PATCH may set.
var rdrEditable = map[string]bool{
	"merchant_descriptor": true,
	"rdr_case_number":     true,
}

// rdrCols is the full projection; nullable text is COALESCE'd to ” so it scans
// into plain strings, while nullable dates/fks scan into pointers.
const rdrCols = `id, tenant_id, rdr_guid, event_guid,
	COALESCE(rdr_case_number,''), rdr_date, COALESCE(rdr_resolution,''),
	COALESCE(arn,''), COALESCE(auth_code,''), amount, currency,
	COALESCE(card_first_6,''), COALESCE(card_last_4,''),
	COALESCE(merchant_descriptor,''), COALESCE(prevention_type,''), transaction_date,
	COALESCE(order_guid,''), COALESCE(order_id,''), COALESCE(crm_account_guid,''),
	assigned_to, stage, is_hidden, created_at, updated_at`

func scanRdr(row pgx.Row) (*domain.RdrCase, error) {
	var c domain.RdrCase
	err := row.Scan(
		&c.ID, &c.TenantID, &c.RdrGUID, &c.EventGUID,
		&c.RdrCaseNumber, &c.RdrDate, &c.RdrResolution,
		&c.ARN, &c.AuthCode, &c.Amount, &c.Currency,
		&c.CardFirst6, &c.CardLast4,
		&c.MerchantDescriptor, &c.PreventionType, &c.TransactionDate,
		&c.OrderGUID, &c.OrderID, &c.CRMAccountGUID,
		&c.AssignedTo, &c.Stage, &c.IsHidden, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ---- domain.CaseRepository --------------------------------------------------

func (r *RdrRepository) CaseType() domain.CaseType { return domain.CaseRdr }

func (r *RdrRepository) AllowedStages() []string {
	return []string{"new", "in_review", "resolved"}
}

func (r *RdrRepository) CanResolve() bool { return true }

func (r *RdrRepository) ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error) {
	return existsForTenant(ctx, r.pool, "rdr_cases", tenantID, id)
}

func (r *RdrRepository) GetStage(ctx context.Context, tenantID, id int64) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `SELECT stage FROM rdr_cases WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.NotFound("rdr")
	}
	return stage, err
}

// ChangeStage moves the case and records the transition in one transaction.
func (r *RdrRepository) ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error {
	return changeStageTx(ctx, r.pool, "rdr_cases", domain.CaseRdr, tenantID, id, toStage, userID, notes)
}

func (r *RdrRepository) Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rdr_cases SET assigned_to = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		assigneeID, tenantID, id)
	if err != nil {
		return fmt.Errorf("assign rdr: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("rdr")
	}
	return nil
}

func (r *RdrRepository) SetHidden(ctx context.Context, tenantID, id int64, hidden bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rdr_cases SET is_hidden = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		hidden, tenantID, id)
	if err != nil {
		return fmt.Errorf("hide rdr: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("rdr")
	}
	return nil
}

// Resolve submits an RDR resolution ('accepted' or 'declined') and moves the
// case to 'resolved'.
func (r *RdrRepository) Resolve(ctx context.Context, tenantID, id int64, resolution string, userID int64) error {
	if resolution != "accepted" && resolution != "declined" {
		return domain.Invalid("resolution", "must be accepted or declined")
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE rdr_cases SET rdr_resolution = $1, stage = 'resolved', updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		resolution, tenantID, id)
	if err != nil {
		return fmt.Errorf("resolve rdr: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("rdr")
	}
	return nil
}

func (r *RdrRepository) ListSummaries(ctx context.Context, tenantID int64, f domain.CaseFilter) ([]domain.CaseSummary, int, error) {
	f.Normalize()
	where, args := rdrFilter(tenantID, f)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rdr_cases `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rdr: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT id, tenant_id, rdr_guid AS guid, '' AS mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		FROM rdr_cases %s %s LIMIT $%d OFFSET $%d`, where, orderClause(f), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rdr: %w", err)
	}
	defer rows.Close()

	var out []domain.CaseSummary
	for rows.Next() {
		s := domain.CaseSummary{Type: domain.CaseRdr}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.GUID, &s.MID, &s.Amount, &s.Currency, &s.Stage, &s.AssignedTo, &s.IsHidden, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *RdrRepository) CountByStage(ctx context.Context, tenantID int64) (map[string]int, error) {
	return countByStage(ctx, r.pool, "rdr_cases", tenantID)
}

// ---- domain.RdrRepository (typed) ------------------------------------------

func (r *RdrRepository) Get(ctx context.Context, tenantID, id int64) (*domain.RdrCase, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+rdrCols+` FROM rdr_cases WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	c, err := scanRdr(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("rdr")
	}
	if err != nil {
		return nil, fmt.Errorf("get rdr: %w", err)
	}
	return c, nil
}

func (r *RdrRepository) Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*domain.RdrCase, error) {
	ok, err := updateByID(ctx, r.pool, "rdr_cases", tenantID, id, fields, rdrEditable)
	if err != nil {
		return nil, err
	}
	if !ok {
		if exists, _ := r.ExistsForTenant(ctx, tenantID, id); !exists {
			return nil, domain.NotFound("rdr")
		}
	}
	return r.Get(ctx, tenantID, id)
}

func (r *RdrRepository) Insert(ctx context.Context, c *domain.RdrCase) (*domain.RdrCase, error) {
	const q = `INSERT INTO rdr_cases
		(tenant_id, rdr_guid, event_guid, rdr_case_number, rdr_date, rdr_resolution,
		 arn, auth_code, amount, currency, card_first_6, card_last_4,
		 merchant_descriptor, prevention_type, transaction_date,
		 order_guid, order_id, crm_account_guid, assigned_to, stage, is_hidden)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING id, created_at, updated_at`
	stage := c.Stage
	if stage == "" {
		stage = "new"
	}
	currency := c.Currency
	if currency == "" {
		currency = "USD"
	}
	err := r.pool.QueryRow(ctx, q,
		c.TenantID, c.RdrGUID, c.EventGUID, nullable(c.RdrCaseNumber), c.RdrDate, nullable(c.RdrResolution),
		nullable(c.ARN), nullable(c.AuthCode), c.Amount, currency, nullable(c.CardFirst6), nullable(c.CardLast4),
		nullable(c.MerchantDescriptor), nullable(c.PreventionType), c.TransactionDate,
		nullable(c.OrderGUID), nullable(c.OrderID), nullable(c.CRMAccountGUID), c.AssignedTo, stage, c.IsHidden,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert rdr: %w", mapDBError(err))
	}
	c.Stage, c.Currency = stage, currency
	return c, nil
}

// rdrFilter builds the WHERE clause shared by list + count. rdr_cases has no
// `mid` column, so the MID filter is skipped.
func rdrFilter(tenantID int64, f domain.CaseFilter) (string, []any) {
	conds := []string{"tenant_id = $1"}
	args := []any{tenantID}
	if !f.IncludeHidden {
		conds = append(conds, "is_hidden = FALSE")
	}
	if f.Stage != "" {
		args = append(args, f.Stage)
		conds = append(conds, fmt.Sprintf("stage = $%d", len(args)))
	}
	if f.AssignedTo != nil {
		args = append(args, *f.AssignedTo)
		conds = append(conds, fmt.Sprintf("assigned_to = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		conds = append(conds, fmt.Sprintf("(rdr_guid ILIKE $%d OR order_id ILIKE $%d OR arn ILIKE $%d OR rdr_case_number ILIKE $%d)", len(args), len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}
