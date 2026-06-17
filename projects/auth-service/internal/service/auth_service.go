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

type TokenIssuer interface {
	Generate(userID int64) (token string, expiresAt time.Time, err error)
}

type AuthService struct {
	repo   domain.UserRepository
	tokens TokenIssuer
}

func NewAuthService(repo domain.UserRepository, tokens TokenIssuer) *AuthService {
	return &AuthService{repo: repo, tokens: tokens}
}

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

func (s *AuthService) Login(ctx context.Context, email, password string) (string, time.Time, error) {

	normalised, err := normaliseEmail(email)
	if err != nil {
		return "", time.Time{}, err
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
		return domain.ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("must be at least %d characters", minPasswordLen),
		}
	}
	return nil

}
