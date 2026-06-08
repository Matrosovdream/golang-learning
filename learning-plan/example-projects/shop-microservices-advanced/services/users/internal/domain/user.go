package domain

import (
	"context"
	"errors"
	"time"
)

// Domain errors.
var (
	ErrEmailTaken   = errors.New("email already registered")
	ErrUserNotFound = errors.New("user not found")
)

// User is the core entity.
type User struct {
	ID        int64
	Email     string
	Name      string
	CreatedAt time.Time
}

// Repository is the storage contract for users.
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
}
