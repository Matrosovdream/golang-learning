package handler

import (
	"net/http"
	"strconv"

	"midigator/internal/service"
)

// SearchHandler serves the cross-type search endpoint, returning matching case
// summaries from every case type in one flat list.
type SearchHandler struct {
	svc *service.SearchService
}

// NewSearchHandler builds the cross-type search handler.
func NewSearchHandler(svc *service.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

// Index runs ?q= across all case types, honouring an optional ?limit=.
func (h *SearchHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	items, err := h.svc.Search(r.Context(), q, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toCaseSummaries(items)})
}
