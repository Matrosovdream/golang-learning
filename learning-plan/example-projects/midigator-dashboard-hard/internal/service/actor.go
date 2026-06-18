// Package service holds the business logic: it validates input, enforces RBAC's
// tenant boundary, orchestrates repositories, publishes events, and writes the
// activity log. Handlers call into it; it never imports net/http.
package service

import (
	"context"
	"encoding/json"

	"midigator/internal/domain"
	"midigator/internal/reqctx"
)

// actor returns the authenticated identity or ErrUnauthorized.
func actor(ctx context.Context) (*reqctx.Auth, error) {
	a, ok := reqctx.From(ctx)
	if !ok || a == nil {
		return nil, domain.ErrUnauthorized
	}
	return a, nil
}

// tenantActor returns the caller's user id and tenant id, requiring a tenant
// (tenant-plane endpoints are not reachable by a platform admin who isn't
// impersonating a tenant user).
func tenantActor(ctx context.Context) (userID, tenantID int64, err error) {
	a, err := actor(ctx)
	if err != nil {
		return 0, 0, err
	}
	if a.TenantID == nil {
		return 0, 0, domain.ErrForbidden
	}
	return a.UserID, *a.TenantID, nil
}

// activityLogger writes audit rows; nil-safe and best-effort.
type activityLogger struct {
	repo domain.ActivityRepository
}

func (l activityLogger) log(ctx context.Context, tenantID, userID int64, loggableType string, loggableID *int64, action string, metadata map[string]any) {
	if l.repo == nil {
		return
	}
	var raw json.RawMessage
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			raw = b
		}
	}
	_ = l.repo.Log(ctx, &domain.ActivityEntry{
		TenantID:     tenantID,
		UserID:       userID,
		LoggableType: loggableType,
		LoggableID:   loggableID,
		Action:       action,
		Metadata:     raw,
	})
}

func int64Ptr(v int64) *int64 { return &v }
