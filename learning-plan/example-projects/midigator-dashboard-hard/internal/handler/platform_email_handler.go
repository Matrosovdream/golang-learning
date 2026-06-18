package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
)

// PlatformEmailHandler serves the platform-plane (cross-tenant) email admin
// endpoints: manage email templates (global or per-tenant) and read the
// sent-email log. It holds the repositories directly: there is no per-template
// business logic, so no service layer.
type PlatformEmailHandler struct {
	templates domain.EmailTemplateRepository
	logs      domain.EmailLogRepository
}

func NewPlatformEmailHandler(templates domain.EmailTemplateRepository, logs domain.EmailLogRepository) *PlatformEmailHandler {
	return &PlatformEmailHandler{templates: templates, logs: logs}
}

type platformEmailTemplateResponse struct {
	ID        int64           `json:"id"`
	TenantID  *int64          `json:"tenant_id"`
	Name      string          `json:"name"`
	Subject   string          `json:"subject"`
	Body      string          `json:"body"`
	Variables json.RawMessage `json:"variables"`
	IsActive  bool            `json:"is_active"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func toPlatformEmailTemplateResponse(t *domain.EmailTemplate) platformEmailTemplateResponse {
	return platformEmailTemplateResponse{
		ID:        t.ID,
		TenantID:  t.TenantID,
		Name:      t.Name,
		Subject:   t.Subject,
		Body:      t.Body,
		Variables: t.Variables,
		IsActive:  t.IsActive,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
}

type platformEmailLogResponse struct {
	ID            int64   `json:"id"`
	TenantID      int64   `json:"tenant_id"`
	UserID        int64   `json:"user_id"`
	EmailableType string  `json:"emailable_type"`
	EmailableID   int64   `json:"emailable_id"`
	ToEmail       string  `json:"to_email"`
	Subject       string  `json:"subject"`
	Body          string  `json:"body"`
	TemplateID    *int64  `json:"template_id"`
	Status        string  `json:"status"`
	SentAt        *string `json:"sent_at"`
	CreatedAt     string  `json:"created_at"`
}

func toPlatformEmailLogResponse(e *domain.EmailLog) platformEmailLogResponse {
	return platformEmailLogResponse{
		ID:            e.ID,
		TenantID:      e.TenantID,
		UserID:        e.UserID,
		EmailableType: e.EmailableType,
		EmailableID:   e.EmailableID,
		ToEmail:       e.ToEmail,
		Subject:       e.Subject,
		Body:          e.Body,
		TemplateID:    e.TemplateID,
		Status:        e.Status,
		SentAt:        rfc3339(e.SentAt),
		CreatedAt:     e.CreatedAt.Format(time.RFC3339),
	}
}

// TemplatesIndex lists every email template across all tenants (and globals).
func (h *PlatformEmailHandler) TemplatesIndex(w http.ResponseWriter, r *http.Request) {
	items, err := h.templates.ListAll(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]platformEmailTemplateResponse, len(items))
	for i := range items {
		out[i] = toPlatformEmailTemplateResponse(&items[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// TemplateShow returns one email template by id.
func (h *PlatformEmailHandler) TemplateShow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.templates.GetByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlatformEmailTemplateResponse(t))
}

// platformEmailTemplateRequest is the create/update payload. TenantID nil = global.
type platformEmailTemplateRequest struct {
	TenantID  *int64          `json:"tenant_id"`
	Name      string          `json:"name"`
	Subject   string          `json:"subject"`
	Body      string          `json:"body"`
	Variables json.RawMessage `json:"variables"`
	IsActive  bool            `json:"is_active"`
}

func (req platformEmailTemplateRequest) toInput() domain.EmailTemplateInput {
	return domain.EmailTemplateInput{
		TenantID:  req.TenantID,
		Name:      req.Name,
		Subject:   req.Subject,
		Body:      req.Body,
		Variables: req.Variables,
		IsActive:  req.IsActive,
	}
}

// TemplateCreate creates an email template (global when tenant_id is null).
func (h *PlatformEmailHandler) TemplateCreate(w http.ResponseWriter, r *http.Request) {
	var req platformEmailTemplateRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.templates.Create(r.Context(), req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPlatformEmailTemplateResponse(t))
}

// TemplateUpdate edits an existing email template.
func (h *PlatformEmailHandler) TemplateUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req platformEmailTemplateRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.templates.Update(r.Context(), id, req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlatformEmailTemplateResponse(t))
}

// TemplateDestroy deletes an email template.
func (h *PlatformEmailHandler) TemplateDestroy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.templates.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LogsIndex lists sent-email records across all tenants, with paging.
func (h *PlatformEmailHandler) LogsIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := 50, 0
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
	items, total, err := h.logs.ListAll(r.Context(), limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]platformEmailLogResponse, len(items))
	for i := range items {
		out[i] = toPlatformEmailLogResponse(&items[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}

// LogShow returns one sent-email record by id.
func (h *PlatformEmailHandler) LogShow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	e, err := h.logs.GetByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlatformEmailLogResponse(e))
}
