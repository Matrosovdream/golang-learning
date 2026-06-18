package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// AuthHandler serves login (password + PIN), me, and logout.
type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginPINRequest struct {
	Email string `json:"email"`
	PIN   string `json:"pin"`
}

type sessionResponse struct {
	Token     string       `json:"token"`
	ExpiresAt string       `json:"expires_at"`
	User      userResponse `json:"user"`
}

type userResponse struct {
	ID              int64  `json:"id"`
	TenantID        *int64 `json:"tenant_id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	IsPlatformAdmin bool   `json:"is_platform_admin"`
	IsActive        bool   `json:"is_active"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:              u.ID,
		TenantID:        u.TenantID,
		Email:           u.Email,
		Name:            u.Name,
		IsPlatformAdmin: u.IsPlatformAdmin,
		IsActive:        u.IsActive,
	}
}

func writeSession(w http.ResponseWriter, s *service.Session) {
	writeJSON(w, http.StatusOK, sessionResponse{
		Token:     s.Token,
		ExpiresAt: s.ExpiresAt.Format(time.RFC3339),
		User:      toUserResponse(s.User),
	})
}

// Login authenticates with email + password.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	s, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSession(w, s)
}

// LoginPIN authenticates with email + PIN.
func (h *AuthHandler) LoginPIN(w http.ResponseWriter, r *http.Request) {
	var req loginPINRequest
	if !decode(w, r, &req) {
		return
	}
	s, err := h.svc.LoginPIN(r.Context(), req.Email, req.PIN)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSession(w, s)
}

// Me returns the authenticated caller's profile, roles, and rights.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Me(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	roles := make([]string, len(p.Roles))
	for i, role := range p.Roles {
		roles[i] = role.Slug
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":   toUserResponse(p.User),
		"roles":  roles,
		"rights": p.Rights,
	})
}

// Logout is a no-op for stateless JWT (the client discards the token).
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
