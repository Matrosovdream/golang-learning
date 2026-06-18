package service

import (
	"context"

	"midigator/internal/domain"
)

// PlatformUserService is the platform-plane (cross-tenant) admin surface over
// users: it lists every user and toggles their active / platform-admin flags.
// The router guards it with platform-admin middleware, so it does not re-check
// rights or scope to a tenant.
type PlatformUserService struct {
	users    domain.UserRepository
	activity activityLogger
}

// NewPlatformUserService wires the platform user service.
func NewPlatformUserService(users domain.UserRepository, activity domain.ActivityRepository) *PlatformUserService {
	return &PlatformUserService{users: users, activity: activityLogger{repo: activity}}
}

// List returns every user across all tenants.
func (s *PlatformUserService) List(ctx context.Context) ([]domain.User, error) {
	return s.users.ListAll(ctx)
}

// ToggleActive flips a user's active flag.
func (s *PlatformUserService) ToggleActive(ctx context.Context, id int64) (*domain.User, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	updated, err := s.users.SetActive(ctx, id, !u.IsActive)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, userTenant(updated), a.UserID, "user", int64Ptr(id), "active_toggled", map[string]any{"is_active": updated.IsActive})
	return updated, nil
}

// TogglePlatformAdmin flips a user's platform-admin flag.
func (s *PlatformUserService) TogglePlatformAdmin(ctx context.Context, id int64) (*domain.User, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	updated, err := s.users.SetPlatformAdmin(ctx, id, !u.IsPlatformAdmin)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, userTenant(updated), a.UserID, "user", int64Ptr(id), "platform_admin_toggled", map[string]any{"is_platform_admin": updated.IsPlatformAdmin})
	return updated, nil
}

// userTenant returns the user's tenant id for the activity log, or 0 for a
// platform admin (tenant_id null).
func userTenant(u *domain.User) int64 {
	if u != nil && u.TenantID != nil {
		return *u.TenantID
	}
	return 0
}
