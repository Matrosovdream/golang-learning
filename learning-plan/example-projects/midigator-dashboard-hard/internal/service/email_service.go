package service

import (
	"context"
	"strings"
	"time"

	"midigator/internal/domain"
)

// EmailService is a synchronous "log mailer": it stores templates and records
// every send in the email log (no SMTP or queue). Sends are tenant-scoped and
// audited; the activity log captures who sent what about which subject.
type EmailService struct {
	templates domain.EmailTemplateRepository
	logs      domain.EmailLogRepository
	activity  activityLogger
}

// NewEmailService wires the email service.
func NewEmailService(templates domain.EmailTemplateRepository, logs domain.EmailLogRepository, activity domain.ActivityRepository) *EmailService {
	return &EmailService{templates: templates, logs: logs, activity: activityLogger{repo: activity}}
}

// ListTemplates returns the tenant's templates plus global (tenant_id null) ones.
func (s *EmailService) ListTemplates(ctx context.Context) ([]domain.EmailTemplate, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.templates.ListForTenant(ctx, tenantID)
}

// CreateTemplate creates a template owned by the caller's tenant.
func (s *EmailService) CreateTemplate(ctx context.Context, in domain.EmailTemplateInput) (*domain.EmailTemplate, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	in.TenantID = &tenantID
	t, err := s.templates.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, "email_template", int64Ptr(t.ID), "email_template_created", nil)
	return t, nil
}

// UpdateTemplate edits an existing template.
func (s *EmailService) UpdateTemplate(ctx context.Context, id int64, in domain.EmailTemplateInput) (*domain.EmailTemplate, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	in.TenantID = &tenantID
	t, err := s.templates.Update(ctx, id, in)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, "email_template", int64Ptr(id), "email_template_updated", nil)
	return t, nil
}

// DeleteTemplate removes a template.
func (s *EmailService) DeleteTemplate(ctx context.Context, id int64) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	if err := s.templates.Delete(ctx, id); err != nil {
		return err
	}
	s.activity.log(ctx, tenantID, userID, "email_template", int64Ptr(id), "email_template_deleted", nil)
	return nil
}

// Send records an email about a subject and marks it sent immediately (the log
// mailer has no real transport). Returns the persisted log row.
func (s *EmailService) Send(ctx context.Context, emailableType string, emailableID int64, toEmail, subject, body string, templateID *int64) (*domain.EmailLog, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(toEmail) == "" {
		return nil, domain.Invalid("to_email", "is required")
	}
	if strings.TrimSpace(subject) == "" {
		return nil, domain.Invalid("subject", "is required")
	}
	log, err := s.logs.Create(ctx, &domain.EmailLog{
		TenantID:      tenantID,
		UserID:        userID,
		EmailableType: emailableType,
		EmailableID:   emailableID,
		ToEmail:       toEmail,
		Subject:       subject,
		Body:          body,
		TemplateID:    templateID,
		Status:        "sent",
	})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.logs.MarkSent(ctx, log.ID, now); err != nil {
		return nil, err
	}
	log.SentAt = &now
	s.activity.log(ctx, tenantID, userID, emailableType, int64Ptr(emailableID), "email_sent", map[string]any{"to_email": toEmail})
	return log, nil
}

// Logs returns a page of the tenant's sent-email records.
func (s *EmailService) Logs(ctx context.Context, limit, offset int) ([]domain.EmailLog, int, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.logs.ListByTenant(ctx, tenantID, limit, offset)
}
