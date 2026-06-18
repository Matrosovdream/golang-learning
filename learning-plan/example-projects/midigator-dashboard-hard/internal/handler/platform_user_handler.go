package handler

import (
	"net/http"

	"midigator/internal/service"
)

// PlatformUserHandler serves the platform-plane (cross-tenant) user admin
// endpoints: list every user, toggle active, toggle platform-admin.
type PlatformUserHandler struct {
	svc *service.PlatformUserService
}

func NewPlatformUserHandler(svc *service.PlatformUserService) *PlatformUserHandler {
	return &PlatformUserHandler{svc: svc}
}

// Index lists every user across all tenants.
func (h *PlatformUserHandler) Index(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]userResponse, len(users))
	for i := range users {
		out[i] = toUserResponse(&users[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// ToggleActive flips a user's active flag.
func (h *PlatformUserHandler) ToggleActive(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	u, err := h.svc.ToggleActive(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(u))
}

// TogglePlatformAdmin flips a user's platform-admin flag.
func (h *PlatformUserHandler) TogglePlatformAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	u, err := h.svc.TogglePlatformAdmin(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(u))
}
