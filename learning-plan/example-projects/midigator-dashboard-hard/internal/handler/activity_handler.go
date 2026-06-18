package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// ActivityLogHandler serves the tenant's audit log: who did what to which
// resource, scoped to the caller (or the whole tenant if they may view all).
type ActivityLogHandler struct {
	svc *service.ActivityService
}

func NewActivityLogHandler(svc *service.ActivityService) *ActivityLogHandler {
	return &ActivityLogHandler{svc: svc}
}

type activityResponse struct {
	ID           int64           `json:"id"`
	UserID       int64           `json:"user_id"`
	UserName     string          `json:"user_name"`
	Action       string          `json:"action"`
	LoggableType string          `json:"loggable_type"`
	LoggableID   *int64          `json:"loggable_id"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    string          `json:"created_at"`
}

func toActivityResponse(e domain.ActivityEntry) activityResponse {
	return activityResponse{
		ID:           e.ID,
		UserID:       e.UserID,
		UserName:     e.UserName,
		Action:       e.Action,
		LoggableType: e.LoggableType,
		LoggableID:   e.LoggableID,
		Metadata:     e.Metadata,
		CreatedAt:    e.CreatedAt.Format(time.RFC3339),
	}
}

func toActivityResponses(items []domain.ActivityEntry) []activityResponse {
	out := make([]activityResponse, len(items))
	for i, e := range items {
		out[i] = toActivityResponse(e)
	}
	return out
}

// Index lists activity-log entries for the caller's tenant, with paging.
func (h *ActivityLogHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := 0, 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	items, total, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": toActivityResponses(items),
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}
