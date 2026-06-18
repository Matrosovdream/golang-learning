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

// EmailLogRepository implements domain.EmailLogRepository with raw SQL. Listings
// are tenant-scoped; template_id and sent_at are nullable.
type EmailLogRepository struct {
	pool *pgxpool.Pool
}

func NewEmailLogRepository(pool *pgxpool.Pool) *EmailLogRepository {
	return &EmailLogRepository{pool: pool}
}

// emailLogCols is the full projection. template_id is a nullable bigint scanned
// into *int64; sent_at is a nullable timestamp scanned into *time.Time.
const emailLogCols = `id, tenant_id, user_id, emailable_type, emailable_id,
	to_email, subject, body, template_id, status, sent_at, created_at`

func scanEmailLog(row pgx.Row) (*domain.EmailLog, error) {
	var e domain.EmailLog
	err := row.Scan(
		&e.ID, &e.TenantID, &e.UserID, &e.EmailableType, &e.EmailableID,
		&e.ToEmail, &e.Subject, &e.Body, &e.TemplateID, &e.Status, &e.SentAt, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmailLogRepository) Create(ctx context.Context, e *domain.EmailLog) (*domain.EmailLog, error) {
	const q = `INSERT INTO email_logs
		(tenant_id, user_id, emailable_type, emailable_id, to_email, subject, body, template_id, status, sent_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at`
	status := e.Status
	if status == "" {
		status = "queued"
	}
	err := r.pool.QueryRow(ctx, q,
		e.TenantID, e.UserID, e.EmailableType, e.EmailableID, e.ToEmail,
		e.Subject, e.Body, e.TemplateID, status, e.SentAt,
	).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create email log: %w", mapDBError(err))
	}
	e.Status = status
	return e, nil
}

func (r *EmailLogRepository) MarkSent(ctx context.Context, id int64, at time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE email_logs SET status = 'sent', sent_at = $1 WHERE id = $2`, at, id)
	if err != nil {
		return fmt.Errorf("mark email sent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("email log")
	}
	return nil
}

func (r *EmailLogRepository) MarkStatus(ctx context.Context, id int64, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE email_logs SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("mark email status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("email log")
	}
	return nil
}

func (r *EmailLogRepository) GetByID(ctx context.Context, id int64) (*domain.EmailLog, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+emailLogCols+` FROM email_logs WHERE id = $1`, id)
	e, err := scanEmailLog(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("email log")
	}
	if err != nil {
		return nil, fmt.Errorf("get email log: %w", err)
	}
	return e, nil
}

func (r *EmailLogRepository) ListByTenant(ctx context.Context, tenantID int64, limit, offset int) ([]domain.EmailLog, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_logs WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count email logs: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+emailLogCols+` FROM email_logs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list email logs: %w", err)
	}
	defer rows.Close()
	out, err := scanEmailLogs(rows)
	return out, total, err
}

func (r *EmailLogRepository) ListAll(ctx context.Context, limit, offset int) ([]domain.EmailLog, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_logs`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count all email logs: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+emailLogCols+` FROM email_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list all email logs: %w", err)
	}
	defer rows.Close()
	out, err := scanEmailLogs(rows)
	return out, total, err
}

func scanEmailLogs(rows pgx.Rows) ([]domain.EmailLog, error) {
	var out []domain.EmailLog
	for rows.Next() {
		e, err := scanEmailLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}
