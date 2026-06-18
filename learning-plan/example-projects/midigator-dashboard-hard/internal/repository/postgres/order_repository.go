package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// OrderRepository implements domain.OrderRepository with raw SQL. Orders have no
// workflow stage; they carry a submission lifecycle instead. The file mirrors
// the ChargebackRepository reference pattern.
type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// orderEditable is the whitelist of columns a PATCH may set.
var orderEditable = map[string]bool{
	"order_id":           true,
	"mid":                true,
	"email":              true,
	"phone":              true,
	"marketing_source":   true,
	"billing_first_name": true,
	"billing_last_name":  true,
}

// orderCols is the full projection; nullable text is COALESCE'd to empty so it
// scans into plain strings, while nullable dates/fks/ints scan into pointers and
// JSONB columns scan into json.RawMessage.
const orderCols = `id, tenant_id, order_guid, COALESCE(order_id,''), COALESCE(mid,''),
	order_date, order_amount, currency,
	COALESCE(card_brand,''), COALESCE(card_first_6,''), COALESCE(card_last_4,''),
	COALESCE(card_exp_month,''), COALESCE(card_exp_year,''),
	COALESCE(avs,''), COALESCE(cvv,''),
	COALESCE(processor_auth_code,''), COALESCE(processor_transaction_id,''),
	COALESCE(email,''), COALESCE(phone,''), COALESCE(ip_address,''),
	COALESCE(billing_first_name,''), COALESCE(billing_last_name,''), billing_address,
	refunded, refunded_amount, subscription_cycle, COALESCE(subscription_parent_id,''),
	COALESCE(marketing_source,''), sub_marketing_sources, items, evidence,
	is_hidden, submission_status, COALESCE(submission_error,''), submitted_at,
	created_at, updated_at`

func scanOrder(row pgx.Row) (*domain.Order, error) {
	var o domain.Order
	err := row.Scan(
		&o.ID, &o.TenantID, &o.OrderGUID, &o.OrderID, &o.MID,
		&o.OrderDate, &o.OrderAmount, &o.Currency,
		&o.CardBrand, &o.CardFirst6, &o.CardLast4,
		&o.CardExpMonth, &o.CardExpYear,
		&o.AVS, &o.CVV,
		&o.ProcessorAuthCode, &o.ProcessorTransactionID,
		&o.Email, &o.Phone, &o.IPAddress,
		&o.BillingFirstName, &o.BillingLastName, &o.BillingAddress,
		&o.Refunded, &o.RefundedAmount, &o.SubscriptionCycle, &o.SubscriptionParentID,
		&o.MarketingSource, &o.SubMarketingSources, &o.Items, &o.Evidence,
		&o.IsHidden, &o.SubmissionStatus, &o.SubmissionError, &o.SubmittedAt,
		&o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Create inserts a new order and returns it with the generated id/timestamps.
func (r *OrderRepository) Create(ctx context.Context, o *domain.Order) (*domain.Order, error) {
	const q = `INSERT INTO orders
		(tenant_id, order_guid, order_id, mid, order_date, order_amount, currency,
		 card_brand, card_first_6, card_last_4, email, phone,
		 billing_first_name, billing_last_name, billing_address, items, evidence, submission_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id, created_at, updated_at`
	currency := o.Currency
	if currency == "" {
		currency = "USD"
	}
	status := o.SubmissionStatus
	if status == "" {
		status = domain.SubmissionPending
	}
	err := r.pool.QueryRow(ctx, q,
		o.TenantID, o.OrderGUID, nullable(o.OrderID), nullable(o.MID), o.OrderDate, o.OrderAmount, currency,
		nullable(o.CardBrand), nullable(o.CardFirst6), nullable(o.CardLast4), nullable(o.Email), nullable(o.Phone),
		nullable(o.BillingFirstName), nullable(o.BillingLastName), o.BillingAddress, o.Items, o.Evidence, status,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", mapDBError(err))
	}
	o.Currency, o.SubmissionStatus = currency, status
	return o, nil
}

// Get returns one tenant-scoped order with its full projection.
func (r *OrderRepository) Get(ctx context.Context, tenantID, id int64) (*domain.Order, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	o, err := scanOrder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("order")
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}
	return o, nil
}

// Update edits an order's whitelisted custom fields.
func (r *OrderRepository) Update(ctx context.Context, tenantID, id int64, fields map[string]any) (*domain.Order, error) {
	ok, err := updateByID(ctx, r.pool, "orders", tenantID, id, fields, orderEditable)
	if err != nil {
		return nil, err
	}
	if !ok {
		if exists, _ := r.ExistsForTenant(ctx, tenantID, id); !exists {
			return nil, domain.NotFound("order")
		}
	}
	return r.Get(ctx, tenantID, id)
}

// List paginates a tenant's orders, honoring search/MID/hidden filters.
func (r *OrderRepository) List(ctx context.Context, tenantID int64, f domain.CaseFilter) ([]domain.Order, int, error) {
	f.Normalize()
	where, args := orderFilter(tenantID, f)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	q := fmt.Sprintf(`SELECT `+orderCols+` FROM orders %s %s LIMIT $%d OFFSET $%d`,
		where, orderOrderClause(f), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *o)
	}
	return out, total, rows.Err()
}

// SetSubmission records the result of a Midigator submission attempt.
func (r *OrderRepository) SetSubmission(ctx context.Context, tenantID, id int64, status, errMsg string, submittedAt *time.Time) (*domain.Order, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE orders SET submission_status = $1, submission_error = $2, submitted_at = $3, updated_at = now()
		 WHERE tenant_id = $4 AND id = $5`,
		status, nullable(errMsg), submittedAt, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("set submission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.NotFound("order")
	}
	return r.Get(ctx, tenantID, id)
}

func (r *OrderRepository) ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error) {
	return existsForTenant(ctx, r.pool, "orders", tenantID, id)
}

// orderFilter builds the WHERE clause shared by list + count.
func orderFilter(tenantID int64, f domain.CaseFilter) (string, []any) {
	conds := []string{"tenant_id = $1"}
	args := []any{tenantID}
	if !f.IncludeHidden {
		conds = append(conds, "is_hidden = FALSE")
	}
	if f.MID != "" {
		args = append(args, f.MID)
		conds = append(conds, fmt.Sprintf("mid = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		conds = append(conds, fmt.Sprintf("(order_guid ILIKE $%d OR order_id ILIKE $%d OR mid ILIKE $%d OR email ILIKE $%d)", len(args), len(args), len(args), len(args)))
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// orderOrderClause is the order-specific ORDER BY: the shared filter's "amount"
// sort maps to this table's order_amount column.
func orderOrderClause(f domain.CaseFilter) string {
	col := "created_at"
	switch f.SortBy {
	case "updated_at":
		col = "updated_at"
	case "amount":
		col = "order_amount"
	}
	dir := "DESC"
	if f.SortDir == "asc" {
		dir = "ASC"
	}
	return "ORDER BY " + col + " " + dir
}

// compile-time guard.
var _ domain.OrderRepository = (*OrderRepository)(nil)
