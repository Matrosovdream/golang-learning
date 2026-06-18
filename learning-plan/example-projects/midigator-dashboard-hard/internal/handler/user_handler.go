package handler

import (
	"net/http"

	"midigator/internal/domain"
	"midigator/internal/reqctx"
)

// TenantUserHandler lists the users in the caller's tenant (the directory used
// by the assignment UI). It reads the tenant from the request identity.
type TenantUserHandler struct {
	users domain.UserRepository
}

func NewTenantUserHandler(users domain.UserRepository) *TenantUserHandler {
	return &TenantUserHandler{users: users}
}

// Index returns the tenant's users.
func (h *TenantUserHandler) Index(w http.ResponseWriter, r *http.Request) {
	a := reqctx.MustFrom(r.Context())
	if a == nil || a.TenantID == nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	users, err := h.users.ListByTenant(r.Context(), *a.TenantID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]userResponse, len(users))
	for i := range users {
		out[i] = toUserResponse(&users[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
