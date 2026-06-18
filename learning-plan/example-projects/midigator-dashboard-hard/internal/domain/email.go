package domain

import (
	"context"
	"encoding/json"
	"time"
)

// EmailTemplate is a reusable email body with variable placeholders. tenant_id
// null means a global (platform-provided) template.
type EmailTemplate struct {
	ID        int64
	TenantID  *int64
	Name      string
	Subject   string
	Body      string
	Variables json.RawMessage
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmailTemplateInput creates or updates a template.
type EmailTemplateInput struct {
	TenantID  *int64
	Name      string
	Subject   string
	Body      string
	Variables json.RawMessage
	IsActive  bool
}

// EmailLog is a record of an email sent (or queued) about a case/order.
type EmailLog struct {
	ID            int64
	TenantID      int64
	UserID        int64
	EmailableType string
	EmailableID   int64
	ToEmail       string
	Subject       string
	Body          string
	TemplateID    *int64
	Status        string
	SentAt        *time.Time
	CreatedAt     time.Time
}

// EmailTemplateRepository stores email templates.
type EmailTemplateRepository interface {
	Create(ctx context.Context, in EmailTemplateInput) (*EmailTemplate, error)
	Update(ctx context.Context, id int64, in EmailTemplateInput) (*EmailTemplate, error)
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*EmailTemplate, error)
	// ListForTenant returns the tenant's templates plus global (tenant_id null) ones.
	ListForTenant(ctx context.Context, tenantID int64) ([]EmailTemplate, error)
	ListAll(ctx context.Context) ([]EmailTemplate, error)
}

// EmailLogRepository stores sent-email records.
type EmailLogRepository interface {
	Create(ctx context.Context, e *EmailLog) (*EmailLog, error)
	MarkSent(ctx context.Context, id int64, at time.Time) error
	MarkStatus(ctx context.Context, id int64, status string) error
	GetByID(ctx context.Context, id int64) (*EmailLog, error)
	ListByTenant(ctx context.Context, tenantID int64, limit, offset int) ([]EmailLog, int, error)
	ListAll(ctx context.Context, limit, offset int) ([]EmailLog, int, error)
}
