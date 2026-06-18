package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// PlatformTenantHandler serves the cross-tenant management plane: list/show/
// create/update/destroy tenants plus per-tenant overview, users, activity,
// webhook logs, an active toggle, and a connection test. The router fronts
// these routes with platform-admin middleware.
type PlatformTenantHandler struct {
	svc *service.PlatformTenantService
}

func NewPlatformTenantHandler(svc *service.PlatformTenantService) *PlatformTenantHandler {
	return &PlatformTenantHandler{svc: svc}
}

// tenantRequest is the create/update payload. Secrets are write-only — they are
// accepted here but never echoed back in a tenantResponse.
type tenantRequest struct {
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Domain          string `json:"domain"`
	APISecret       string `json:"midigator_api_secret"`
	SandboxMode     bool   `json:"sandbox_mode"`
	WebhookUsername string `json:"webhook_username"`
	WebhookPassword string `json:"webhook_password"`
	IsActive        bool   `json:"is_active"`
}

func (req tenantRequest) toInput() domain.TenantInput {
	return domain.TenantInput{
		Name:                 req.Name,
		Slug:                 req.Slug,
		Domain:               req.Domain,
		MidigatorAPISecret:   req.APISecret,
		MidigatorSandboxMode: req.SandboxMode,
		WebhookUsername:      req.WebhookUsername,
		WebhookPassword:      req.WebhookPassword,
		IsActive:             req.IsActive,
	}
}

// tenantResponse is the safe public view of a tenant. It deliberately omits
// midigator_api_secret and webhook_password.
type tenantResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Domain      string `json:"domain"`
	IsActive    bool   `json:"is_active"`
	SandboxMode bool   `json:"sandbox_mode"`
	CreatedAt   string `json:"created_at"`
}

func toTenantResponse(t *domain.Tenant) tenantResponse {
	return tenantResponse{
		ID:          t.ID,
		Name:        t.Name,
		Slug:        t.Slug,
		Domain:      t.Domain,
		IsActive:    t.IsActive,
		SandboxMode: t.MidigatorSandboxMode,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
	}
}

func toTenantResponses(items []domain.Tenant) []tenantResponse {
	out := make([]tenantResponse, len(items))
	for i := range items {
		out[i] = toTenantResponse(&items[i])
	}
	return out
}

// Index lists every tenant.
func (h *PlatformTenantHandler) Index(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toTenantResponses(items)})
}

// Show returns one tenant.
func (h *PlatformTenantHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTenantResponse(t))
}

// Create provisions a new tenant.
func (h *PlatformTenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req tenantRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.Create(r.Context(), req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTenantResponse(t))
}

// Update edits a tenant.
func (h *PlatformTenantHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req tenantRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.Update(r.Context(), id, req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTenantResponse(t))
}

// Destroy removes a tenant.
func (h *PlatformTenantHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// Overview returns a tenant's cross-case summary.
func (h *PlatformTenantHandler) Overview(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	ov, err := h.svc.Overview(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id":         ov.TenantID,
		"users":             ov.Users,
		"chargebacks":       ov.Chargebacks,
		"preventions":       ov.Preventions,
		"order_validations": ov.OrderValidations,
		"rdr_cases":         ov.RdrCases,
		"orders":            ov.Orders,
		"open_cases":        ov.OpenCases,
	})
}

// Users lists the users in a tenant.
func (h *PlatformTenantHandler) Users(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	users, err := h.svc.Users(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, len(users))
	for i := range users {
		u := users[i]
		out[i] = map[string]any{
			"id":                u.ID,
			"email":             u.Email,
			"name":              u.Name,
			"is_active":         u.IsActive,
			"is_platform_admin": u.IsPlatformAdmin,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Activity returns a tenant's latest activity-log rows.
func (h *PlatformTenantHandler) Activity(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	items, total, err := h.svc.Activity(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, len(items))
	for i := range items {
		e := items[i]
		out[i] = map[string]any{
			"id":            e.ID,
			"user_id":       e.UserID,
			"user_name":     e.UserName,
			"loggable_type": e.LoggableType,
			"loggable_id":   e.LoggableID,
			"action":        e.Action,
			"metadata":      e.Metadata,
			"created_at":    e.CreatedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": total}})
}

// Webhooks returns a tenant's latest inbound webhook logs.
func (h *PlatformTenantHandler) Webhooks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	items, total, err := h.svc.Webhooks(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]map[string]any, len(items))
	for i := range items {
		l := items[i]
		out[i] = map[string]any{
			"id":            l.ID,
			"event_type":    l.EventType,
			"event_guid":    l.EventGUID,
			"status":        l.Status,
			"error_message": l.ErrorMessage,
			"processed_at":  rfc3339(l.ProcessedAt),
			"created_at":    l.CreatedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": total}})
}

// ToggleActive flips a tenant between active and suspended.
func (h *PlatformTenantHandler) ToggleActive(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.ToggleActive(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTenantResponse(t))
}

// TestConnection probes a tenant's Midigator settings (stub).
func (h *PlatformTenantHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req tenantRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.TestConnection(r.Context(), req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
