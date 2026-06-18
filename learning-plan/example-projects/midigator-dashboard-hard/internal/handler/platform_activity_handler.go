package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
)

// PlatformActivityHandler serves the platform-plane (cross-tenant) audit log:
// who did what to which resource, across every tenant. It holds the repository
// directly: there is no business logic, so no service layer.
type PlatformActivityHandler struct {
	activity domain.ActivityRepository
}

func NewPlatformActivityHandler(activity domain.ActivityRepository) *PlatformActivityHandler {
	return &PlatformActivityHandler{activity: activity}
}

type platformActivityResponse struct {
	ID           int64           `json:"id"`
	TenantID     int64           `json:"tenant_id"`
	UserID       int64           `json:"user_id"`
	UserName     string          `json:"user_name"`
	Action       string          `json:"action"`
	LoggableType string          `json:"loggable_type"`
	LoggableID   *int64          `json:"loggable_id"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    string          `json:"created_at"`
}

func toPlatformActivityResponse(e domain.ActivityEntry) platformActivityResponse {
	return platformActivityResponse{
		ID:           e.ID,
		TenantID:     e.TenantID,
		UserID:       e.UserID,
		UserName:     e.UserName,
		Action:       e.Action,
		LoggableType: e.LoggableType,
		LoggableID:   e.LoggableID,
		Metadata:     e.Metadata,
		CreatedAt:    e.CreatedAt.Format(time.RFC3339),
	}
}

func toPlatformActivityResponses(items []domain.ActivityEntry) []platformActivityResponse {
	out := make([]platformActivityResponse, len(items))
	for i, e := range items {
		out[i] = toPlatformActivityResponse(e)
	}
	return out
}

// Index lists activity-log entries across all tenants, with optional filters
// (tenant_id, user_id) and paging.
func (h *PlatformActivityHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.ActivityFilter{
		Action: q.Get("action"),
		Limit:  50,
		Offset: 0,
	}
	if v := q.Get("tenant_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.TenantID = &n
		}
	}
	if v := q.Get("user_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.UserID = &n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}
	filter.Normalize()
	items, total, err := h.activity.ListAll(r.Context(), filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": toPlatformActivityResponses(items),
		"meta": map[string]any{"total": total, "limit": filter.Limit, "offset": filter.Offset},
	})
}
