package handler

import (
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// PlatformSettingHandler serves the platform-plane (cross-tenant) site-settings
// endpoints: list (optionally scoped to a tenant), upsert, and delete.
type PlatformSettingHandler struct {
	svc *service.PlatformSettingService
}

func NewPlatformSettingHandler(svc *service.PlatformSettingService) *PlatformSettingHandler {
	return &PlatformSettingHandler{svc: svc}
}

type settingResponse struct {
	ID          int64  `json:"id"`
	TenantID    *int64 `json:"tenant_id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Group       string `json:"group"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toSettingResponse(s *domain.SiteSetting) settingResponse {
	return settingResponse{
		ID:          s.ID,
		TenantID:    s.TenantID,
		Key:         s.Key,
		Value:       s.Value,
		Type:        s.Type,
		Group:       s.Group,
		Description: s.Description,
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Format(time.RFC3339),
	}
}

// queryTenantID reads an optional ?tenant_id= as a *int64 (nil = global). It
// returns ok=false only when the value is present but unparseable.
func queryTenantID(r *http.Request) (*int64, bool) {
	v := r.URL.Query().Get("tenant_id")
	if v == "" {
		return nil, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, false
	}
	return &n, true
}

// Index lists the settings for a tenant (?tenant_id=) or the global defaults.
func (h *PlatformSettingHandler) Index(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := queryTenantID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	settings, err := h.svc.List(r.Context(), tenantID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]settingResponse, len(settings))
	for i := range settings {
		out[i] = toSettingResponse(&settings[i])
	}
	writeJSON(w, http.StatusOK, out)
}

type settingUpsertRequest struct {
	TenantID    *int64 `json:"tenant_id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Upsert creates or updates a setting from the request body.
func (h *PlatformSettingHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req settingUpsertRequest
	if !decode(w, r, &req) {
		return
	}
	s, err := h.svc.Upsert(r.Context(), domain.SiteSettingInput{
		TenantID:    req.TenantID,
		Key:         req.Key,
		Value:       req.Value,
		Type:        req.Type,
		Group:       req.Group,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSettingResponse(s))
}

type settingDeleteRequest struct {
	TenantID *int64 `json:"tenant_id"`
	Key      string `json:"key"`
}

// Destroy removes a setting identified by {tenant_id?, key} from the body, or
// from the query string when no body is supplied.
func (h *PlatformSettingHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	var req settingDeleteRequest
	if r.Body != nil && r.ContentLength != 0 {
		if !decode(w, r, &req) {
			return
		}
	}
	if req.Key == "" {
		req.Key = r.URL.Query().Get("key")
		tenantID, ok := queryTenantID(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		req.TenantID = tenantID
	}
	if err := h.svc.Delete(r.Context(), req.TenantID, req.Key); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
