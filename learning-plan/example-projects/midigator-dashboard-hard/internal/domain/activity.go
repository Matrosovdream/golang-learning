package domain

import (
	"context"
	"encoding/json"
	"time"
)

// ActivityEntry is one audit-log row: who did what to which resource.
type ActivityEntry struct {
	ID           int64
	TenantID     int64
	UserID       int64
	UserName     string
	LoggableType string
	LoggableID   *int64
	Action       string
	Metadata     json.RawMessage
	CreatedAt    time.Time
}

// ActivityFilter narrows an activity-log query.
type ActivityFilter struct {
	UserID   *int64
	Action   string
	TenantID *int64 // platform plane only; nil = current tenant
	Limit    int
	Offset   int
}

// Normalize clamps paging.
func (f *ActivityFilter) Normalize() {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// ActivityRepository records and reads the activity log.
type ActivityRepository interface {
	Log(ctx context.Context, e *ActivityEntry) error
	ListByTenant(ctx context.Context, tenantID int64, f ActivityFilter) ([]ActivityEntry, int, error)
	ListAll(ctx context.Context, f ActivityFilter) ([]ActivityEntry, int, error)
}
