package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"

	"midigator/internal/domain"
	"midigator/internal/worker"
)

// WebhookService authenticates inbound Midigator webhooks (per-tenant HTTP
// Basic credentials), records each one, and enqueues it for the worker to
// process. It never touches the case tables directly — that is the worker's job.
type WebhookService struct {
	tenants  domain.TenantRepository
	webhooks domain.WebhookRepository
	enqueuer *worker.Enqueuer
}

// NewWebhookService wires the webhook ingestion service.
func NewWebhookService(tenants domain.TenantRepository, webhooks domain.WebhookRepository, enqueuer *worker.Enqueuer) *WebhookService {
	return &WebhookService{tenants: tenants, webhooks: webhooks, enqueuer: enqueuer}
}

// Authenticate resolves the tenant from the webhook's Basic-auth username and
// constant-time-compares the password. Unknown user / bad password both return
// ErrUnauthorized (no enumeration).
func (s *WebhookService) Authenticate(ctx context.Context, username, password string) (*domain.Tenant, error) {
	if username == "" {
		return nil, domain.ErrUnauthorized
	}
	tenants, err := s.tenants.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tenants {
		t := tenants[i]
		if t.WebhookUsername == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(t.WebhookUsername), []byte(username)) == 1 &&
			subtle.ConstantTimeCompare([]byte(t.WebhookPassword), []byte(password)) == 1 {
			return &t, nil
		}
	}
	return nil, domain.ErrUnauthorized
}

// Ingest records the webhook and enqueues it for processing.
func (s *WebhookService) Ingest(ctx context.Context, tenant *domain.Tenant, eventType, eventGUID string, data, rawBody json.RawMessage) error {
	if eventType == "" {
		return domain.Invalid("event_type", "is required")
	}
	log, err := s.webhooks.Create(ctx, &domain.WebhookLog{
		TenantID:  tenant.ID,
		EventType: eventType,
		EventGUID: eventGUID,
		Payload:   rawBody,
		Status:    domain.WebhookReceived,
	})
	if err != nil {
		return err
	}
	return s.enqueuer.EnqueueWebhook(ctx, worker.WebhookProcessPayload{
		WebhookLogID: log.ID,
		TenantID:     tenant.ID,
		EventType:    eventType,
		EventGUID:    eventGUID,
		Data:         data,
	})
}
