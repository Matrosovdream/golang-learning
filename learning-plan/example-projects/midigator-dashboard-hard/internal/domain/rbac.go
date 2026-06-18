package domain

import (
	"context"
	"time"
)

// Right is a single permission slug in the global catalog (e.g. "chargebacks.view").
type Right struct {
	ID          int64
	Name        string
	Slug        string
	Group       string
	Description string
	CreatedAt   time.Time
}

// Role is a named bundle of rights, scoped to a tenant (tenant_id null = global).
type Role struct {
	ID          int64
	TenantID    *int64
	Name        string
	Slug        string
	Description string
	IsSystem    bool
	Rights      []string // right slugs, populated on read
	CreatedAt   time.Time
}

// RoleInput creates or updates a custom role.
type RoleInput struct {
	TenantID    *int64
	Name        string
	Slug        string
	Description string
	RightSlugs  []string
}

// RightRepository reads the global rights catalog.
type RightRepository interface {
	List(ctx context.Context) ([]Right, error)
	IDsForSlugs(ctx context.Context, slugs []string) ([]int64, error)
	Upsert(ctx context.Context, slug, name, group, description string) error
}

// RoleRepository manages roles, their rights, and user assignments.
type RoleRepository interface {
	Create(ctx context.Context, in RoleInput) (*Role, error)
	Update(ctx context.Context, id int64, in RoleInput) (*Role, error)
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*Role, error)
	GetBySlug(ctx context.Context, tenantID *int64, slug string) (*Role, error)
	ListByTenant(ctx context.Context, tenantID int64) ([]Role, error)
	// SyncRights replaces a role's right set.
	SyncRights(ctx context.Context, roleID int64, rightSlugs []string) error
	// RightsForUser returns the distinct right slugs a user holds across roles.
	RightsForUser(ctx context.Context, userID int64) ([]string, error)
	// RolesForUser returns the user's assigned roles.
	RolesForUser(ctx context.Context, userID int64) ([]Role, error)
	// AssignUser grants a role to a user.
	AssignUser(ctx context.Context, userID, roleID int64, grantedBy *int64) error
	// SyncUserRoles replaces a user's role set.
	SyncUserRoles(ctx context.Context, userID int64, roleIDs []int64) error
}
