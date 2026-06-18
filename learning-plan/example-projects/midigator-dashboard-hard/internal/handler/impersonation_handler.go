package handler

import (
	"net/http"
	"time"

	"midigator/internal/service"
)

// ImpersonationHandler serves the platform-plane impersonation endpoints: start
// acting as a tenant user, and stop (restore the original platform admin). The
// router guards Start with platform-admin middleware.
type ImpersonationHandler struct {
	svc *service.ImpersonationService
}

func NewImpersonationHandler(svc *service.ImpersonationService) *ImpersonationHandler {
	return &ImpersonationHandler{svc: svc}
}

// Start begins impersonating the target user and returns a fresh token.
func (h *ImpersonationHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "userId")
	if !ok {
		return
	}
	token, exp, user, err := h.svc.Start(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
		"user":       toUserResponse(user),
	})
}

// Stop ends the current impersonation session and restores the original admin.
func (h *ImpersonationHandler) Stop(w http.ResponseWriter, r *http.Request) {
	token, exp, err := h.svc.Stop(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp.Format(time.RFC3339),
	})
}
