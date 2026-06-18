package domain

import (
	"context"
	"time"
)

// User is a person who signs in. tenant_id is null for platform admins.
type User struct {
	ID              int64
	TenantID        *int64
	Email           string
	PasswordHash    string
	Name            string
	Avatar          string
	PINHash         string
	IsActive        bool
	IsPlatformAdmin bool
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UserInput creates or updates a user.
type UserInput struct {
	TenantID        *int64
	Email           string
	Name            string
	PasswordHash    string
	PINHash         string
	IsActive        bool
	IsPlatformAdmin bool
}

// UserRepository is the storage contract for users.
type UserRepository interface {
	Create(ctx context.Context, in UserInput) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	// ListByTenant returns users in a tenant (for the users.view list).
	ListByTenant(ctx context.Context, tenantID int64) ([]User, error)
	// ListAll returns every user (platform plane).
	ListAll(ctx context.Context) ([]User, error)
	SetActive(ctx context.Context, id int64, active bool) (*User, error)
	SetPlatformAdmin(ctx context.Context, id int64, isAdmin bool) (*User, error)
	TouchLastLogin(ctx context.Context, id int64) error
}
