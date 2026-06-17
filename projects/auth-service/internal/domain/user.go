package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

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

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type UserRepository interface {
	CreateUser(ctx context.Context, u *User) error
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
}
