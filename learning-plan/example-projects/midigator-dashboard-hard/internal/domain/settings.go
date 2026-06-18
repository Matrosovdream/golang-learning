package domain

import (
	"context"
	"time"
)

// SiteSetting is a key/value config row, scoped to a tenant (tenant_id null =
// global default).
type SiteSetting struct {
	ID          int64
	TenantID    *int64
	Key         string
	Value       string
	Type        string
	Group       string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SiteSettingInput upserts a setting.
type SiteSettingInput struct {
	TenantID    *int64
	Key         string
	Value       string
	Type        string
	Group       string
	Description string
}

// SettingsRepository stores site settings.
type SettingsRepository interface {
	List(ctx context.Context, tenantID *int64) ([]SiteSetting, error)
	Upsert(ctx context.Context, in SiteSettingInput) (*SiteSetting, error)
	Delete(ctx context.Context, tenantID *int64, key string) error
}
