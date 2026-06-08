package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"authservice/internal/domain"
)

const minPasswordLen = 8

// TokenIssuer is satisfied by auth.JWTManager. Declaring the dependency as an
// interface here keeps the service free of any JWT import.
type TokenIssuer interface {
	Generate(userID int64) (token string, expiresAt time.Time, err error)
}

// AuthService holds the business rules: validation, hashing, token issuing.
type AuthService struct {
	repo   domain.UserRepository
	tokens TokenIssuer
}

func NewAuthService(repo domain.UserRepository, tokens TokenIssuer) *AuthService {
	return &AuthService{repo: repo, tokens: tokens}
}

// Register validates the input, hashes the password, and stores the user.
func (s *AuthService) Register(ctx context.Context, email, password string) (*domain.User, error) {
	email, err := normaliseEmail(email)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	user := &domain.User{Email: email, PasswordHash: string(hash)}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login verifies the credentials and returns a signed token.
// Any failure returns ErrInvalidCredentials so the response never reveals
// whether it was the email or the password that was wrong.
func (s *AuthService) Login(ctx context.Context, email, password string) (string, time.Time, error) {
	normalised, err := normaliseEmail(email)
	if err != nil {
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	user, err := s.repo.GetByEmail(ctx, normalised)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", time.Time{}, domain.ErrInvalidCredentials
		}
		return "", time.Time{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	return s.tokens.Generate(user.ID)
}

// GetUser loads the user behind an authenticated request.
func (s *AuthService) GetUser(ctx context.Context, id int64) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

func normaliseEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", domain.ValidationError{Field: "email", Message: "is invalid"}
	}
	return strings.ToLower(addr.Address), nil
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLen {
		return domain.ValidationError{Field: "password", Message: fmt.Sprintf("must be at least %d characters", minPasswordLen)}
	}
	return nil
}
