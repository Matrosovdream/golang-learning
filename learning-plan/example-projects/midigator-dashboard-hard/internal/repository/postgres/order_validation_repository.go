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

// OrderValidationRepository implements domain.OrderValidationRepository (and the
// shared domain.CaseRepository) with raw SQL, mirroring ChargebackRepository.
type OrderValidationRepository struct {
	pool *pgxpool.Pool
}

func NewOrderValidationRepository(pool *pgxpool.Pool) *OrderValidationRepository {
	return &OrderValidationRepository{pool: pool}
}

// order_validationEditable is the whitelist of columns a PATCH may set.
var order_validationEditable = map[string]bool{
	"order_validation_type": true,
}

// order_validationCols is the full projection; nullable text is COALESCE'd to ”
// so it scans into plain strings, while nullable dates/fks scan into pointers.
const order_validationCols = `id, tenant_id, order_validation_guid, event_guid,
	COALESCE(order_validation_type,''), order_validation_timestamp, transaction_timestamp,
	amount, currency,
	COALESCE(card_first_6,''), COALESCE(card_last_4,''),
	COALESCE(arn,''), COALESCE(auth_code,''), COALESCE(card_brand,''),
	COALESCE(order_guid,''), COALESCE(order_id,''), COALESCE(crm_account_guid,''),
	COALESCE(mid,''), assigned_to, stage, is_hidden, created_at, updated_at`

func scanOrderValidation(row pgx.Row) (*domain.OrderValidation, error) {
	var v domain.OrderValidation
	err := row.Scan(
		&v.ID, &v.TenantID, &v.OrderValidationGUID, &v.EventGUID,
		&v.OrderValidationType, &v.OrderValidationTimestamp, &v.TransactionTimestamp,
		&v.Amount, &v.Currency,
		&v.CardFirst6, &v.CardLast4,
		&v.ARN, &v.AuthCode, &v.CardBrand,
		&v.OrderGUID, &v.OrderID, &v.CRMAccountGUID,
		&v.MID, &v.AssignedTo, &v.Stage, &v.IsHidden, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ---- domain.CaseRepository --------------------------------------------------

func (r *OrderValidationRepository) CaseType() domain.CaseType { return domain.CaseOrderValidation }

func (r *OrderValidationRepository) AllowedStages() []string {
	return []string{"new", "in_review", "action_taken", "resolved"}
}

func (r *OrderValidationRepository) CanResolve() bool { return false }

func (r *OrderValidationRepository) ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error) {
	return existsForTenant(ctx, r.pool, "order_validations", tenantID, id)
}

func (r *OrderValidationRepository) GetStage(ctx context.Context, tenantID, id int64) (string, error) {
	var stage string
	err := r.pool.QueryRow(ctx, `SELECT stage FROM order_validations WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.NotFound("order_validation")
	}
	return stage, err
}

// ChangeStage moves the case and records the transition in one transaction.
func (r *OrderValidationRepository) ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error {
	return changeStageTx(ctx, r.pool, "order_validations", domain.CaseOrderValidation, tenantID, id, toStage, userID, notes)
}

func (r *OrderValidationRepository) Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE order_validations SET assigned_to = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		assigneeID, tenantID, id)
	if err != nil {
		return fmt.Errorf("assign order_validation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("order_validation")
	}
	return nil
}

func (r *OrderValidationRepository) SetHidden(ctx context.Context, tenantID, id int64, hidden bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE order_validations SET is_hidden = $1, updated_at = now() WHERE tenant_id = $2 AND id = $3`,
		hidden, tenantID, id)
	if err != nil {
		return fmt.Errorf("hide order_validation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("order_validation")
	}
	return nil
}

// Resolve is not supported for order validations (no /resolve route).
func (r *OrderValidationRepository) Resolve(ctx context.Context, tenantID, id int64, resolution string, userID int64) error {
	return domain.ConflictError{Message: "order validations cannot be resolved"}
}

func (r *OrderValidationRepository) ListSummaries(ctx context.Context, tenantID int64, f domain.CaseFilter) ([]domain.CaseSummary, int, error) {
	f.Normalize()
	where, args := orderValidationFilter(tenantID, f)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM order_validations `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count order_validations: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT id, tenant_id, order_validation_guid AS guid, COALESCE(mid,'') AS mid, amount, currency, stage, assigned_to, is_hidden, created_at, updated_at
		FROM order_validations %s %s LIMIT $%d OFFSET $%d`, where, orderClause(f), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list order_validations: %w", err)
	}
	defer rows.Close()

	var out []domain.CaseSummary
	for rows.Next() {
		s := domain.CaseSummary{Type: domain.CaseOrderValidation}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.GUID, &s.MID, &s.Amount, &s.Currency, &s.Stage, &s.AssignedTo, &s.IsHidden, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *OrderValidationRepository) CountByStage(ctx context.Context, tenantID int64) (map[string]int, error) {
	return countByStage(ctx, r.pool, "order_validations", tenantID)
}

// ---- domain.OrderValidationRepository (typed) ------------------------------

func (r *OrderValidationRepository) Get(ctx context.Context, tenantID, id int64) (*domain.OrderValidation, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+order_validationCols+` FROM order_validations WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	v, err := scanOrderValidation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("order_validation")
	}
	if err != nil {
		return nil, fmt.Errorf("get order_validation: %w", err)
	}
	return v, nil
}

func (r *OrderValidationRepository) Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*domain.OrderValidation, error) {
	ok, err := updateByID(ctx, r.pool, "order_validations", tenantID, id, fields, order_validationEditable)
	if err != nil {
		return nil, err
	}
	if !ok {
		if exists, _ := r.ExistsForTenant(ctx, tenantID, id); !exists {
			return nil, domain.NotFound("order_validation")
		}
	}
	return r.Get(ctx, tenantID, id)
}

func (r *OrderValidationRepository) Insert(ctx context.Context, v *domain.OrderValidation) (*domain.OrderValidation, error) {
	const q = `INSERT INTO order_validations
		(tenant_id, order_validation_guid, event_guid, order_validation_type, order_validation_timestamp, transaction_timestamp,
		 amount, currency, card_first_6, card_last_4, arn, auth_code, card_brand,
		 order_guid, order_id, crm_account_guid, mid, assigned_to, stage, is_hidden)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id, created_at, updated_at`
	stage := v.Stage
	if stage == "" {
		stage = "new"
	}
	currency := v.Currency
	if currency == "" {
		currency = "USD"
	}
	err := r.pool.QueryRow(ctx, q,
		v.TenantID, v.OrderValidationGUID, v.EventGUID, nullable(v.OrderValidationType), v.OrderValidationTimestamp, v.TransactionTimestamp,
		v.Amount, currency, nullable(v.CardFirst6), nullable(v.CardLast4), nullable(v.ARN), nullable(v.AuthCode), nullable(v.CardBrand),
		nullable(v.OrderGUID), nullable(v.OrderID), nullable(v.CRMAccountGUID), nullable(v.MID), v.AssignedTo, stage, v.IsHidden,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert order_validation: %w", mapDBError(err))
	}
	v.Stage, v.Currency = stage, currency
	return v, nil
}

// orderValidationFilter builds the WHERE clause shared by list + count.
func orderValidationFilter(tenantID int64, f domain.CaseFilter) (string, []any) {
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
		conds = append(conds, fmt.Sprintf("(order_validation_guid ILIKE $%d OR order_id ILIKE $%d OR arn ILIKE $%d)", len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}
