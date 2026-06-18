// Package reqctx carries the authenticated caller through the request context.
// It depends on nothing else so every layer can read the current identity
// without an import cycle.
package reqctx

import "context"

// Auth is the effective identity for a request, built by the auth middleware
// after verifying the JWT and loading the user's rights.
type Auth struct {
	UserID          int64
	TenantID        *int64 // nil for a platform admin not impersonating a tenant user
	IsPlatformAdmin bool
	Rights          map[string]struct{}
	// ImpersonatorID is set when a platform admin is acting as this tenant user;
	// it records who to hand control back to on "stop impersonating".
	ImpersonatorID *int64
}

// HasRight reports whether the caller holds a right. Platform admins hold all.
func (a *Auth) HasRight(slug string) bool {
	if a == nil {
		return false
	}
	if a.IsPlatformAdmin {
		return true
	}
	_, ok := a.Rights[slug]
	return ok
}

type ctxKey int

const authKey ctxKey = iota

// With returns a context carrying the auth identity.
func With(ctx context.Context, a *Auth) context.Context {
	return context.WithValue(ctx, authKey, a)
}

// From returns the auth identity, if present.
func From(ctx context.Context) (*Auth, bool) {
	a, ok := ctx.Value(authKey).(*Auth)
	return a, ok
}

// MustFrom returns the auth identity, which is guaranteed present after the
// auth middleware has run.
func MustFrom(ctx context.Context) *Auth {
	a, _ := From(ctx)
	return a
}
