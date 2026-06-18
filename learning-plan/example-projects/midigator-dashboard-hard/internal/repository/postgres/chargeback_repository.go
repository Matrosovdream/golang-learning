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

// ChargebackRepository implements domain.ChargebackRepository (and the shared
// domain.CaseRepository) with raw SQL. This file is the reference pattern the
// other three case repositories (prevention / order_validation / rdr) mirror.
type ChargebackRepository struct {
	pool *pgxpool.Pool
}

func NewChargebackRepository(pool *pgxpool.Pool) *ChargebackRepository {
	return &ChargebackRepository{pool: pool}
}

// chargebackEditable is the whitelist of columns a PATCH may set.
var chargebackEditable = map[string]bool{
	"case_number":        true,
	"arn":                true,
	"reason_code":        true,
	"reason_description": true,
	"result":             true,
	"dnf_reason":         true,
}

// chargebackCols is the full projection; nullable text is COALESCE'd to ” so it
// scans into plain strings, while nullable dates/fks scan into pointers.
const chargebackCols = `id, tenant_id, chargeback_guid, event_guid,
	COALESCE(case_number,''), COALESCE(arn,''), mid, amount, currency,
	COALESCE(card_brand,''), COALESCE(card_first_6,''), COALESCE(card_last_4,''),
	COALESCE(reason_code,''), COALESCE(reason_description,''),
	chargeback_date, date_received, due_date,
	COALESCE(processor_transaction_id,''), processor_transaction_date,
	COALESCE(auth_code,''), COALESCE(alternate_transaction_id,''),
	result, COALESCE(dnf_reason,''), dnf_timestamp,
	COALESCE(order_guid,''), COALESCE(order_id,''), COALESCE(crm_account_guid,''),
	assigned_to, stage, is_hidden, created_at, updated_at`

func scanChargeback(row pgx.Row) (*domain.Chargeback, error) {
	var c domain.Chargeback
	err := row.Scan(
		&c.ID, &c.TenantID, &c.ChargebackGUID, &c.EventGUID,
		&c.CaseNumber, &c.ARN, &c.MID, &c.Amount, &c.Currency,
		&c.CardBrand, &c.CardFirst6, &c.CardLast4,
		&c.ReasonCode, &c.ReasonDescription,
		&c.ChargebackDate, &c.DateReceived, &c.DueDate,
		&c.ProcessorTransactionID, &c.ProcessorTransactionDate,
		&c.AuthCode, &c.AlternateTransactionID,
		&c.Result, &c.DNFReason, &c.DNFTimestamp,
		&c.OrderGUID, &c.OrderID, &c.CRMAccountGUID,
		&c.AssignedTo, &c.Stage, &c.IsHidden, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ---- domain.CaseRepository --------------------------------------------------

func (r *ChargebackRepository) CaseType() domain.CaseType { return domain.CaseChargeback }

func (r *ChargebackRepository) AllowedStages() []string {
	return []string{"new", "in_review", "action_taken", "responded", "resolved"}
}

func (r *ChargebackRepository) CanResolve() bool { return false }

func (r *ChargebackRepository) ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error) {
	return existsForTenant(ctx, r.pool, "chargebacks", tenantID, id)
}

func (r *ChargebackRepository) GetStage(ctx context.Context, tenantID, id int64) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `SELECT stage FROM chargebacks WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.NotFound("chargeback")
	}
	return stage, err
}

// ChangeStage moves the case and records the transition in one transaction.
func (r *ChargebackRepository) ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error {
	return changeStageTx(ctx, r.pool, "chargebacks", domain.CaseChargeback, tenantID, id, toStage, userID, notes)
}

func (r *ChargebackRepository) Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE chargebacks SET assigned_to = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		assigneeID, tenantID, id)
	if err != nil {
		return fmt.Errorf("assign chargeback: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("chargeback")
	}
	return nil
}

func (r *ChargebackRepository) SetHidden(ctx context.Context, tenantID, id int64, hidden bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE chargebacks SET is_hidden = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		hidden, tenantID, id)
	if err != nil {
		return fmt.Errorf("hide chargeback: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("chargeback")
	}
	return nil
}

// Resolve is not supported for chargebacks (no /resolve route).
func (r *ChargebackRepository) Resolve(ctx context.Context, tenantID, id int64, resolution string, userID int64) error {
	return domain.ConflictError{Message: "chargebacks cannot be resolved directly"}
}

func (r *ChargebackRepository) ListSummaries(ctx context.Context, tenantID int64, f domain.CaseFilter) ([]domain.CaseSummary, int, error) {
	f.Normalize()
	where, args := chargebackFilter(tenantID, f)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chargebacks `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chargebacks: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT id, tenant_id, chargeback_guid, mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		FROM chargebacks %s %s LIMIT $%d OFFSET $%d`, where, orderClause(f), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list chargebacks: %w", err)
	}
	defer rows.Close()

	var out []domain.CaseSummary
	for rows.Next() {
		s := domain.CaseSummary{Type: domain.CaseChargeback}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.GUID, &s.MID, &s.Amount, &s.Currency, &s.Stage, &s.AssignedTo, &s.IsHidden, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *ChargebackRepository) CountByStage(ctx context.Context, tenantID int64) (map[string]int, error) {
	return countByStage(ctx, r.pool, "chargebacks", tenantID)
}

// ---- domain.ChargebackRepository (typed) -----------------------------------

func (r *ChargebackRepository) Get(ctx context.Context, tenantID, id int64) (*domain.Chargeback, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+chargebackCols+` FROM chargebacks WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	c, err := scanChargeback(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("chargeback")
	}
	if err != nil {
		return nil, fmt.Errorf("get chargeback: %w", err)
	}
	return c, nil
}

func (r *ChargebackRepository) Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*domain.Chargeback, error) {
	ok, err := updateByID(ctx, r.pool, "chargebacks", tenantID, id, fields, chargebackEditable)
	if err != nil {
		return nil, err
	}
	if !ok {
		if exists, _ := r.ExistsForTenant(ctx, tenantID, id); !exists {
			return nil, domain.NotFound("chargeback")
		}
	}
	return r.Get(ctx, tenantID, id)
}

func (r *ChargebackRepository) BulkSetHidden(ctx context.Context, tenantID int64, ids []int64, hidden bool) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE chargebacks SET is_hidden = $1, updated_at = now() WHERE tenant_id = $2 AND id = ANY($3)`,
		hidden, tenantID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulk hide chargebacks: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *ChargebackRepository) Insert(ctx context.Context, c *domain.Chargeback) (*domain.Chargeback, error) {
	const q = `INSERT INTO chargebacks
		(tenant_id, chargeback_guid, event_guid, case_number, arn, mid, amount, currency,
		 card_brand, card_first_6, card_last_4, reason_code, reason_description,
		 chargeback_date, date_received, due_date, processor_transaction_id, processor_transaction_date,
		 auth_code, alternate_transaction_id, result, dnf_reason, dnf_timestamp,
		 order_guid, order_id, crm_account_guid, assigned_to, stage, is_hidden)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
		RETURNING id, created_at, updated_at`
	stage := c.Stage
	if stage == "" {
		stage = "new"
	}
	result := c.Result
	if result == "" {
		result = "pending"
	}
	currency := c.Currency
	if currency == "" {
		currency = "USD"
	}
	err := r.pool.QueryRow(ctx, q,
		c.TenantID, c.ChargebackGUID, c.EventGUID, nullable(c.CaseNumber), nullable(c.ARN), c.MID, c.Amount, currency,
		nullable(c.CardBrand), nullable(c.CardFirst6), nullable(c.CardLast4), nullable(c.ReasonCode), nullable(c.ReasonDescription),
		c.ChargebackDate, c.DateReceived, c.DueDate, nullable(c.ProcessorTransactionID), c.ProcessorTransactionDate,
		nullable(c.AuthCode), nullable(c.AlternateTransactionID), result, nullable(c.DNFReason), c.DNFTimestamp,
		nullable(c.OrderGUID), nullable(c.OrderID), nullable(c.CRMAccountGUID), c.AssignedTo, stage, c.IsHidden,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert chargeback: %w", mapDBError(err))
	}
	c.Stage, c.Result, c.Currency = stage, result, currency
	return c, nil
}

// chargebackFilter builds the WHERE clause shared by list + count.
func chargebackFilter(tenantID int64, f domain.CaseFilter) (string, []any) {
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
		conds = append(conds, fmt.Sprintf("(chargeback_guid ILIKE $%d OR order_id ILIKE $%d OR arn ILIKE $%d OR case_number ILIKE $%d)", len(args), len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}
