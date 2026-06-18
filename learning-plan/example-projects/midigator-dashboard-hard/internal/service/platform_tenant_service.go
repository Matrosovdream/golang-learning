package service

import (
	"context"
	"strings"

	"midigator/internal/domain"
)

// PlatformTenantService is the cross-tenant management plane for tenant orgs:
// list/create/update/delete tenants and drill into one tenant's overview,
// users, activity, and webhook logs. It carries no tenant scope of its own —
// the router fronts every route with platform-admin middleware, so this layer
// trusts the caller and addresses tenants by id.
type PlatformTenantService struct {
	tenants   domain.TenantRepository
	users     domain.UserRepository
	activity  domain.ActivityRepository
	webhooks  domain.WebhookRepository
	reporting domain.ReportingRepository
}

// NewPlatformTenantService wires the platform tenant service.
func NewPlatformTenantService(tenants domain.TenantRepository, users domain.UserRepository, activity domain.ActivityRepository, webhooks domain.WebhookRepository, reporting domain.ReportingRepository) *PlatformTenantService {
	return &PlatformTenantService{
		tenants:   tenants,
		users:     users,
		activity:  activity,
		webhooks:  webhooks,
		reporting: reporting,
	}
}

// List returns every tenant org.
func (s *PlatformTenantService) List(ctx context.Context) ([]domain.Tenant, error) {
	return s.tenants.List(ctx)
}

// Get returns one tenant by id.
func (s *PlatformTenantService) Get(ctx context.Context, id int64) (*domain.Tenant, error) {
	return s.tenants.GetByID(ctx, id)
}

// Create provisions a new tenant org.
func (s *PlatformTenantService) Create(ctx context.Context, in domain.TenantInput) (*domain.Tenant, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, domain.Invalid("name", "is required")
	}
	if strings.TrimSpace(in.Slug) == "" {
		return nil, domain.Invalid("slug", "is required")
	}
	return s.tenants.Create(ctx, in)
}

// Update edits a tenant org.
func (s *PlatformTenantService) Update(ctx context.Context, id int64, in domain.TenantInput) (*domain.Tenant, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, domain.Invalid("name", "is required")
	}
	if strings.TrimSpace(in.Slug) == "" {
		return nil, domain.Invalid("slug", "is required")
	}
	return s.tenants.Update(ctx, id, in)
}

// Delete removes a tenant org.
func (s *PlatformTenantService) Delete(ctx context.Context, id int64) error {
	return s.tenants.Delete(ctx, id)
}

// Overview returns a per-tenant summary (counts across case types and orders).
func (s *PlatformTenantService) Overview(ctx context.Context, id int64) (*domain.TenantOverview, error) {
	return s.reporting.TenantOverview(ctx, id)
}

// Users lists the users in a tenant.
func (s *PlatformTenantService) Users(ctx context.Context, id int64) ([]domain.User, error) {
	return s.users.ListByTenant(ctx, id)
}

// Activity returns the latest activity-log rows for a tenant.
func (s *PlatformTenantService) Activity(ctx context.Context, id int64) ([]domain.ActivityEntry, int, error) {
	return s.activity.ListAll(ctx, domain.ActivityFilter{TenantID: &id, Limit: 50})
}

// Webhooks returns the latest inbound webhook logs for a tenant.
func (s *PlatformTenantService) Webhooks(ctx context.Context, id int64) ([]domain.WebhookLog, int, error) {
	return s.webhooks.ListByTenant(ctx, id, 50, 0)
}

// ToggleActive flips a tenant between active and suspended.
func (s *PlatformTenantService) ToggleActive(ctx context.Context, id int64) (*domain.Tenant, error) {
	current, err := s.tenants.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.tenants.SetActive(ctx, id, !current.IsActive)
}

// TestConnection is a stub that reports whether a tenant's Midigator settings
// would connect. Real credential probing is out of scope for this build.
func (s *PlatformTenantService) TestConnection(ctx context.Context, in domain.TenantInput) (map[string]any, error) {
	return map[string]any{"ok": true, "sandbox": in.MidigatorSandboxMode}, nil
}
