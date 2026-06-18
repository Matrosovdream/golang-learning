package service

import (
	"context"
	"time"

	"midigator/internal/auth"
	"midigator/internal/domain"
	"midigator/internal/reqctx"
)

// ImpersonationService lets a platform admin act as a tenant user (and back
// out again). It mints a fresh access token carrying the impersonator id so the
// caller can later "stop impersonating" and be restored. The router guards
// Start with platform-admin middleware; this service still re-checks the auth
// invariants defensively.
type ImpersonationService struct {
	tokens *auth.TokenManager
	users  domain.UserRepository
}

// NewImpersonationService wires the impersonation service.
func NewImpersonationService(tokens *auth.TokenManager, users domain.UserRepository) *ImpersonationService {
	return &ImpersonationService{tokens: tokens, users: users}
}

// Start begins impersonating the target user: it issues a token for that user
// with the current platform admin recorded as the impersonator.
func (s *ImpersonationService) Start(ctx context.Context, targetUserID int64) (token string, exp time.Time, user *domain.User, err error) {
	a := reqctx.MustFrom(ctx)
	if a == nil || !a.IsPlatformAdmin {
		return "", time.Time{}, nil, domain.ErrForbidden
	}
	target, err := s.users.GetByID(ctx, targetUserID)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	imp := a.UserID
	token, exp, err = s.tokens.Issue(targetUserID, &imp, time.Now())
	if err != nil {
		return "", time.Time{}, nil, err
	}
	return token, exp, target, nil
}

// Stop ends an impersonation session: it issues a token restoring the original
// impersonator, with no impersonator set.
func (s *ImpersonationService) Stop(ctx context.Context) (token string, exp time.Time, err error) {
	a := reqctx.MustFrom(ctx)
	if a == nil || a.ImpersonatorID == nil {
		return "", time.Time{}, domain.ErrForbidden
	}
	token, exp, err = s.tokens.Issue(*a.ImpersonatorID, nil, time.Now())
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}
