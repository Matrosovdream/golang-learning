package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
)

// PlatformWebhookHandler serves the platform-plane (cross-tenant) webhook log:
// list every inbound webhook and show one by id. The router guards it with
// platform-admin middleware.
type PlatformWebhookHandler struct {
	webhooks domain.WebhookRepository
}

func NewPlatformWebhookHandler(webhooks domain.WebhookRepository) *PlatformWebhookHandler {
	return &PlatformWebhookHandler{webhooks: webhooks}
}

type webhookLogResponse struct {
	ID           int64           `json:"id"`
	TenantID     int64           `json:"tenant_id"`
	EventType    string          `json:"event_type"`
	EventGUID    string          `json:"event_guid"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message"`
	ProcessedAt  *string         `json:"processed_at"`
	CreatedAt    string          `json:"created_at"`
}

func toWebhookLogResponse(wl *domain.WebhookLog) webhookLogResponse {
	return webhookLogResponse{
		ID:           wl.ID,
		TenantID:     wl.TenantID,
		EventType:    wl.EventType,
		EventGUID:    wl.EventGUID,
		Payload:      wl.Payload,
		Status:       wl.Status,
		ErrorMessage: wl.ErrorMessage,
		ProcessedAt:  rfc3339(wl.ProcessedAt),
		CreatedAt:    wl.CreatedAt.Format(time.RFC3339),
	}
}

// Index lists every inbound webhook across all tenants (paged).
func (h *PlatformWebhookHandler) Index(w http.ResponseWriter, r *http.Request) {
	limit, offset := 50, 0
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	logs, total, err := h.webhooks.ListAll(r.Context(), limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]webhookLogResponse, len(logs))
	for i := range logs {
		out[i] = toWebhookLogResponse(&logs[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// Show returns one webhook log by id.
func (h *PlatformWebhookHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	wl, err := h.webhooks.GetByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toWebhookLogResponse(wl))
}
