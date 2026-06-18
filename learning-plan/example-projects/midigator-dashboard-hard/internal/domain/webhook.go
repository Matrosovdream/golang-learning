package domain

import (
	"context"
	"encoding/json"
	"time"
)

// WebhookLog records every inbound Midigator webhook for audit + replay.
type WebhookLog struct {
	ID           int64
	TenantID     int64
	EventType    string
	EventGUID    string
	Payload      json.RawMessage
	Status       string
	ErrorMessage string
	ProcessedAt  *time.Time
	CreatedAt    time.Time
}

// Webhook status values.
const (
	WebhookReceived  = "received"
	WebhookProcessed = "processed"
	WebhookFailed    = "failed"
)

// WebhookRepository stores webhook logs.
type WebhookRepository interface {
	Create(ctx context.Context, w *WebhookLog) (*WebhookLog, error)
	MarkProcessed(ctx context.Context, id int64, at time.Time) error
	MarkFailed(ctx context.Context, id int64, errMsg string) error
	GetByID(ctx context.Context, id int64) (*WebhookLog, error)
	ListByTenant(ctx context.Context, tenantID int64, limit, offset int) ([]WebhookLog, int, error)
	ListAll(ctx context.Context, limit, offset int) ([]WebhookLog, int, error)
}
