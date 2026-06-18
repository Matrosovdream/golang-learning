package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// WebhookRepository implements domain.WebhookRepository with raw SQL. Listings
// are tenant-scoped; payload is JSONB, error_message is nullable text COALESCE'd
// to ” so it scans into a plain string, processed_at is nullable.
type WebhookRepository struct {
	pool *pgxpool.Pool
}

func NewWebhookRepository(pool *pgxpool.Pool) *WebhookRepository {
	return &WebhookRepository{pool: pool}
}

// webhookCols is the full projection.
const webhookCols = `id, tenant_id, event_type, event_guid, payload, status,
	COALESCE(error_message,''), processed_at, created_at`

func scanWebhookLog(row pgx.Row) (*domain.WebhookLog, error) {
	var w domain.WebhookLog
	err := row.Scan(
		&w.ID, &w.TenantID, &w.EventType, &w.EventGUID, &w.Payload,
		&w.Status, &w.ErrorMessage, &w.ProcessedAt, &w.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WebhookRepository) Create(ctx context.Context, w *domain.WebhookLog) (*domain.WebhookLog, error) {
	const q = `INSERT INTO webhook_logs
		(tenant_id, event_type, event_guid, payload, status, error_message, processed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at`
	status := w.Status
	if status == "" {
		status = domain.WebhookReceived
	}
	err := r.pool.QueryRow(ctx, q,
		w.TenantID, w.EventType, w.EventGUID, w.Payload, status, nullable(w.ErrorMessage), w.ProcessedAt,
	).Scan(&w.ID, &w.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook log: %w", mapDBError(err))
	}
	w.Status = status
	return w, nil
}

func (r *WebhookRepository) MarkProcessed(ctx context.Context, id int64, at time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE webhook_logs SET status = $1, processed_at = $2 WHERE id = $3`,
		domain.WebhookProcessed, at, id)
	if err != nil {
		return fmt.Errorf("mark webhook processed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("webhook log")
	}
	return nil
}

func (r *WebhookRepository) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE webhook_logs SET status = $1, error_message = $2 WHERE id = $3`,
		domain.WebhookFailed, nullable(errMsg), id)
	if err != nil {
		return fmt.Errorf("mark webhook failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("webhook log")
	}
	return nil
}

func (r *WebhookRepository) GetByID(ctx context.Context, id int64) (*domain.WebhookLog, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+webhookCols+` FROM webhook_logs WHERE id = $1`, id)
	w, err := scanWebhookLog(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("webhook log")
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook log: %w", err)
	}
	return w, nil
}

func (r *WebhookRepository) ListByTenant(ctx context.Context, tenantID int64, limit, offset int) ([]domain.WebhookLog, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_logs WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count webhook logs: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+webhookCols+` FROM webhook_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list webhook logs: %w", err)
	}
	defer rows.Close()

	var out []domain.WebhookLog
	for rows.Next() {
		w, err := scanWebhookLog(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *w)
	}
	return out, total, rows.Err()
}

// ListAll returns webhook logs across every tenant (platform plane).
func (r *WebhookRepository) ListAll(ctx context.Context, limit, offset int) ([]domain.WebhookLog, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_logs`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count webhook logs: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+webhookCols+` FROM webhook_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list all webhook logs: %w", err)
	}
	defer rows.Close()

	var out []domain.WebhookLog
	for rows.Next() {
		w, err := scanWebhookLog(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *w)
	}
	return out, total, rows.Err()
}
