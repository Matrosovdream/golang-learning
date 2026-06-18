package handler

import (
	"net/http"

	"midigator/internal/domain"
)

// PlatformRightHandler serves the global rights catalog (read-only). It holds
// the repository directly: there is no business logic, so no service layer.
type PlatformRightHandler struct {
	rights domain.RightRepository
}

func NewPlatformRightHandler(rights domain.RightRepository) *PlatformRightHandler {
	return &PlatformRightHandler{rights: rights}
}

type rightResponse struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Index returns the global rights catalog as a JSON array.
func (h *PlatformRightHandler) Index(w http.ResponseWriter, r *http.Request) {
	rights, err := h.rights.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]rightResponse, len(rights))
	for i, rt := range rights {
		out[i] = rightResponse{
			ID:          rt.ID,
			Slug:        rt.Slug,
			Name:        rt.Name,
			Group:       rt.Group,
			Description: rt.Description,
		}
	}
	writeJSON(w, http.StatusOK, out)
}
