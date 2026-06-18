package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Tenant is a merchant org. Every tenant-scoped row hangs off a tenant id.
type Tenant struct {
	ID                   int64
	Name                 string
	Slug                 string
	Domain               string
	MidigatorAPISecret   string
	MidigatorSandboxMode bool
	WebhookUsername      string
	WebhookPassword      string
	IsActive             bool
	Settings             json.RawMessage
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// TenantInput is the create/update payload for the platform plane.
type TenantInput struct {
	Name                 string
	Slug                 string
	Domain               string
	MidigatorAPISecret   string
	MidigatorSandboxMode bool
	WebhookUsername      string
	WebhookPassword      string
	IsActive             bool
}

// TenantRepository is the storage contract for tenants.
type TenantRepository interface {
	Create(ctx context.Context, in TenantInput) (*Tenant, error)
	Update(ctx context.Context, id int64, in TenantInput) (*Tenant, error)
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	List(ctx context.Context) ([]Tenant, error)
	SetActive(ctx context.Context, id int64, active bool) (*Tenant, error)
}
