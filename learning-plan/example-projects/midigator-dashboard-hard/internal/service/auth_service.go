package service

import (
	"context"
	"time"

	"midigator/internal/auth"
	"midigator/internal/domain"
)

// AuthService handles password + PIN login and the "me" lookup. Login failures
// are uniform (no user enumeration): a missing user and a bad credential return
// the same ErrUnauthorized.
type AuthService struct {
	users  domain.UserRepository
	roles  domain.RoleRepository
	tokens *auth.TokenManager
	now    func() time.Time
}

// NewAuthService wires the auth service.
func NewAuthService(users domain.UserRepository, roles domain.RoleRepository, tokens *auth.TokenManager) *AuthService {
	return &AuthService{users: users, roles: roles, tokens: tokens, now: time.Now}
}

// Session is the result of a successful login.
type Session struct {
	Token     string
	ExpiresAt time.Time
	User      *domain.User
}

// Login authenticates by email + password.
func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
	return s.authenticate(ctx, email, password, func(u *domain.User, secret string) bool {
		return auth.Check(u.PasswordHash, secret)
	})
}

// LoginPIN authenticates by email + PIN.
func (s *AuthService) LoginPIN(ctx context.Context, email, pin string) (*Session, error) {
	return s.authenticate(ctx, email, pin, func(u *domain.User, secret string) bool {
		return u.PINHash != "" && auth.Check(u.PINHash, secret)
	})
}

func (s *AuthService) authenticate(ctx context.Context, email, secret string, check func(*domain.User, string) bool) (*Session, error) {
	if email == "" || secret == "" {
		return nil, domain.ErrUnauthorized
	}
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil || user == nil || !user.IsActive || !check(user, secret) {
		return nil, domain.ErrUnauthorized
	}
	token, exp, err := s.tokens.Issue(user.ID, nil, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.users.TouchLastLogin(ctx, user.ID)
	return &Session{Token: token, ExpiresAt: exp, User: user}, nil
}

// Profile bundles the current user with their roles and effective rights.
type Profile struct {
	User   *domain.User
	Roles  []domain.Role
	Rights []string
}

// Me returns the authenticated caller's profile.
func (s *AuthService) Me(ctx context.Context) (*Profile, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	p := &Profile{User: user}
	if a.IsPlatformAdmin {
		p.Rights = []string{"*"}
		return p, nil
	}
	if p.Roles, err = s.roles.RolesForUser(ctx, a.UserID); err != nil {
		return nil, err
	}
	if p.Rights, err = s.roles.RightsForUser(ctx, a.UserID); err != nil {
		return nil, err
	}
	return p, nil
}
