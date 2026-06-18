package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// EmailHandler serves the email-template CRUD endpoints, the synchronous send
// endpoint, and the per-tenant email log listing.
type EmailHandler struct {
	svc *service.EmailService
}

func NewEmailHandler(svc *service.EmailService) *EmailHandler {
	return &EmailHandler{svc: svc}
}

type emailTemplateResponse struct {
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

func toEmailTemplateResponse(t *domain.EmailTemplate) emailTemplateResponse {
	return emailTemplateResponse{
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

func toEmailTemplateResponses(items []domain.EmailTemplate) []emailTemplateResponse {
	out := make([]emailTemplateResponse, len(items))
	for i := range items {
		out[i] = toEmailTemplateResponse(&items[i])
	}
	return out
}

type emailLogResponse struct {
	ID            int64   `json:"id"`
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

func toEmailLogResponse(e domain.EmailLog) emailLogResponse {
	return emailLogResponse{
		ID:            e.ID,
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

func toEmailLogResponses(items []domain.EmailLog) []emailLogResponse {
	out := make([]emailLogResponse, len(items))
	for i, e := range items {
		out[i] = toEmailLogResponse(e)
	}
	return out
}

// emailTemplateRequest is the create/update body for an email template.
type emailTemplateRequest struct {
	Name      string          `json:"name"`
	Subject   string          `json:"subject"`
	Body      string          `json:"body"`
	Variables json.RawMessage `json:"variables"`
	IsActive  bool            `json:"is_active"`
}

func (req emailTemplateRequest) input() domain.EmailTemplateInput {
	return domain.EmailTemplateInput{
		Name:      req.Name,
		Subject:   req.Subject,
		Body:      req.Body,
		Variables: req.Variables,
		IsActive:  req.IsActive,
	}
}

// emailSendRequest is the body for sending (logging) an email.
type emailSendRequest struct {
	EmailableType string `json:"emailable_type"`
	EmailableID   int64  `json:"emailable_id"`
	ToEmail       string `json:"to_email"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	TemplateID    *int64 `json:"template_id"`
}

// ListTemplates lists the tenant's templates plus global ones.
func (h *EmailHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListTemplates(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toEmailTemplateResponses(items)})
}

// CreateTemplate creates a new email template.
func (h *EmailHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req emailTemplateRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.CreateTemplate(r.Context(), req.input())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEmailTemplateResponse(t))
}

// UpdateTemplate edits an existing email template.
func (h *EmailHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req emailTemplateRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.UpdateTemplate(r.Context(), id, req.input())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEmailTemplateResponse(t))
}

// DeleteTemplate removes an email template.
func (h *EmailHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// Send records (and immediately marks sent) an email about a subject.
func (h *EmailHandler) Send(w http.ResponseWriter, r *http.Request) {
	var req emailSendRequest
	if !decode(w, r, &req) {
		return
	}
	log, err := h.svc.Send(r.Context(), req.EmailableType, req.EmailableID, req.ToEmail, req.Subject, req.Body, req.TemplateID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEmailLogResponse(*log))
}

// Logs lists a page of the tenant's sent-email records.
func (h *EmailHandler) Logs(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.svc.Logs(r.Context(), limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": toEmailLogResponses(items),
		"meta": map[string]any{"total": total, "limit": limit, "offset": offset},
	})
}
