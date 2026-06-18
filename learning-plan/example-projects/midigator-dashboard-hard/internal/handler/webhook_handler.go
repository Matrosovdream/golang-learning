package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"midigator/internal/service"
)

// WebhookHandler receives inbound Midigator webhooks at /rest/v1/webhooks/midigator.
// Auth is per-tenant HTTP Basic; the body is recorded and queued, then a 202 is
// returned immediately (the worker does the real ingestion).
type WebhookHandler struct {
	svc *service.WebhookService
}

func NewWebhookHandler(svc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

type webhookBody struct {
	EventType string          `json:"event_type"`
	EventGUID string          `json:"event_guid"`
	Data      json.RawMessage `json:"data"`
}

// Handle authenticates, records, and enqueues a webhook.
func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="midigator-webhook"`)
		writeError(w, http.StatusUnauthorized, "basic auth required")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}
	var body webhookBody
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	tenant, err := h.svc.Authenticate(r.Context(), username, password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := h.svc.Ingest(r.Context(), tenant, body.EventType, body.EventGUID, body.Data, raw); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "event_guid": body.EventGUID})
}
