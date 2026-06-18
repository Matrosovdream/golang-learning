package service

import (
	"context"

	"midigator/internal/domain"
)

// PlatformSettingService is the platform-plane (cross-tenant) admin surface over
// site settings: list, upsert, and delete key/value config rows, either global
// (tenant_id nil) or scoped to a tenant. The router guards it with platform-admin
// middleware, so it does not re-check rights.
type PlatformSettingService struct {
	settings domain.SettingsRepository
}

// NewPlatformSettingService wires the platform setting service.
func NewPlatformSettingService(settings domain.SettingsRepository) *PlatformSettingService {
	return &PlatformSettingService{settings: settings}
}

// List returns the settings for a tenant, or the global defaults when tenantID
// is nil.
func (s *PlatformSettingService) List(ctx context.Context, tenantID *int64) ([]domain.SiteSetting, error) {
	return s.settings.List(ctx, tenantID)
}

// Upsert creates or updates a setting.
func (s *PlatformSettingService) Upsert(ctx context.Context, in domain.SiteSettingInput) (*domain.SiteSetting, error) {
	if in.Key == "" {
		return nil, domain.Invalid("key", "is required")
	}
	return s.settings.Upsert(ctx, in)
}

// Delete removes a setting by tenant scope and key.
func (s *PlatformSettingService) Delete(ctx context.Context, tenantID *int64, key string) error {
	if key == "" {
		return domain.Invalid("key", "is required")
	}
	return s.settings.Delete(ctx, tenantID, key)
}
