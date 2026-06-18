package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// EmailTemplateRepository implements domain.EmailTemplateRepository with raw SQL.
// email_templates.tenant_id is nullable (null = a global, platform-provided
// template), so it scans into *int64.
type EmailTemplateRepository struct {
	pool *pgxpool.Pool
}

func NewEmailTemplateRepository(pool *pgxpool.Pool) *EmailTemplateRepository {
	return &EmailTemplateRepository{pool: pool}
}

// emailTemplateCols is the full projection. variables is JSONB scanned into
// json.RawMessage; tenant_id is a nullable bigint scanned into *int64.
const emailTemplateCols = `id, tenant_id, name, subject, body, variables, is_active, created_at, updated_at`

func scanEmailTemplate(row pgx.Row) (*domain.EmailTemplate, error) {
	var t domain.EmailTemplate
	err := row.Scan(
		&t.ID, &t.TenantID, &t.Name, &t.Subject, &t.Body,
		&t.Variables, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *EmailTemplateRepository) Create(ctx context.Context, in domain.EmailTemplateInput) (*domain.EmailTemplate, error) {
	const q = `INSERT INTO email_templates (tenant_id, name, subject, body, variables, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + emailTemplateCols
	row := r.pool.QueryRow(ctx, q, in.TenantID, in.Name, in.Subject, in.Body, in.Variables, in.IsActive)
	t, err := scanEmailTemplate(row)
	if err != nil {
		return nil, fmt.Errorf("create email template: %w", mapDBError(err))
	}
	return t, nil
}

func (r *EmailTemplateRepository) Update(ctx context.Context, id int64, in domain.EmailTemplateInput) (*domain.EmailTemplate, error) {
	const q = `UPDATE email_templates
		SET name = $2, subject = $3, body = $4, variables = $5, is_active = $6, updated_at = now()
		WHERE id = $1
		RETURNING ` + emailTemplateCols
	row := r.pool.QueryRow(ctx, q, id, in.Name, in.Subject, in.Body, in.Variables, in.IsActive)
	t, err := scanEmailTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("email template")
	}
	if err != nil {
		return nil, fmt.Errorf("update email template: %w", mapDBError(err))
	}
	return t, nil
}

func (r *EmailTemplateRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM email_templates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete email template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("email template")
	}
	return nil
}

func (r *EmailTemplateRepository) GetByID(ctx context.Context, id int64) (*domain.EmailTemplate, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+emailTemplateCols+` FROM email_templates WHERE id = $1`, id)
	t, err := scanEmailTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("email template")
	}
	if err != nil {
		return nil, fmt.Errorf("get email template: %w", err)
	}
	return t, nil
}

// ListForTenant returns the tenant's templates plus global (tenant_id null) ones.
func (r *EmailTemplateRepository) ListForTenant(ctx context.Context, tenantID int64) ([]domain.EmailTemplate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+emailTemplateCols+` FROM email_templates WHERE tenant_id = $1 OR tenant_id IS NULL ORDER BY name ASC`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("list email templates: %w", err)
	}
	defer rows.Close()
	return scanEmailTemplates(rows)
}

func (r *EmailTemplateRepository) ListAll(ctx context.Context) ([]domain.EmailTemplate, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+emailTemplateCols+` FROM email_templates ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all email templates: %w", err)
	}
	defer rows.Close()
	return scanEmailTemplates(rows)
}

func scanEmailTemplates(rows pgx.Rows) ([]domain.EmailTemplate, error) {
	var out []domain.EmailTemplate
	for rows.Next() {
		t, err := scanEmailTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}
