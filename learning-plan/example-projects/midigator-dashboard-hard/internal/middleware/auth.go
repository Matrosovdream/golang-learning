package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"midigator/internal/auth"
	"midigator/internal/domain"
	"midigator/internal/reqctx"
	"midigator/internal/rights"
)

// Authenticator verifies bearer tokens and builds the request's identity.
type Authenticator struct {
	tokens *auth.TokenManager
	users  domain.UserRepository
	rights *rights.Cache
}

// NewAuthenticator wires the dependencies needed to authenticate a request.
func NewAuthenticator(tokens *auth.TokenManager, users domain.UserRepository, cache *rights.Cache) *Authenticator {
	return &Authenticator{tokens: tokens, users: users, rights: cache}
}

// Authenticate requires a valid bearer token, loads the user, and attaches the
// effective identity (rights resolved via the cache) to the request context.
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r)
		if raw == "" {
			unauthorized(w, "missing bearer token")
			return
		}
		claims, err := a.tokens.Parse(raw)
		if err != nil {
			unauthorized(w, "invalid token")
			return
		}
		user, err := a.users.GetByID(r.Context(), claims.UserID)
		if err != nil || user == nil || !user.IsActive {
			unauthorized(w, "user not found or inactive")
			return
		}

		ident := &reqctx.Auth{
			UserID:          user.ID,
			TenantID:        user.TenantID,
			IsPlatformAdmin: user.IsPlatformAdmin,
			ImpersonatorID:  claims.Imp,
		}
		if !user.IsPlatformAdmin {
			set, err := a.rights.Get(r.Context(), user.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load permissions"})
				return
			}
			ident.Rights = set
		}

		next.ServeHTTP(w, r.WithContext(reqctx.With(r.Context(), ident)))
	})
}

// RequireRight wraps a handler so it only runs when the caller holds the right.
func RequireRight(slug string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a := reqctx.MustFrom(r.Context()); !a.HasRight(slug) {
			forbidden(w, "missing right "+slug)
			return
		}
		h(w, r)
	}
}

// RequirePlatformAdmin wraps a handler so it only runs for platform admins.
func RequirePlatformAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := reqctx.MustFrom(r.Context())
		if a == nil || !a.IsPlatformAdmin {
			forbidden(w, "platform admin access required")
			return
		}
		h(w, r)
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func unauthorized(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg})
}

func forbidden(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
