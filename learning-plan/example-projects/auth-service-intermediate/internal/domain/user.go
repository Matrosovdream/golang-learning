package domain

import (
	"context"
	"errors"
	"time"
)

// Domain errors. Service/repository translate failures into these so the
// handler never sees database- or crypto-specific details.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// User is the core entity. PasswordHash never leaves the service layer in a
// response — the handler's DTO omits it.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// UserRepository is the storage contract for users.
type UserRepository interface {
	CreateUser(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
}
