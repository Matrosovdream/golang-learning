package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// CaseHandler serves the generic, type-agnostic case endpoints (index, change
// stage, assign, hide, resolve). One instance is bound to each case type; the
// router wraps each route with the type-specific RBAC right.
type CaseHandler struct {
	svc *service.CaseService
	typ domain.CaseType
}

// NewCaseHandler builds a handler for one case type.
func NewCaseHandler(svc *service.CaseService, typ domain.CaseType) *CaseHandler {
	return &CaseHandler{svc: svc, typ: typ}
}

// caseSummaryResponse is the shared JSON shape for a case in a list.
type caseSummaryResponse struct {
	Type       string `json:"type"`
	ID         int64  `json:"id"`
	GUID       string `json:"guid"`
	MID        string `json:"mid"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Stage      string `json:"stage"`
	AssignedTo *int64 `json:"assigned_to"`
	IsHidden   bool   `json:"is_hidden"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func toCaseSummary(s domain.CaseSummary) caseSummaryResponse {
	return caseSummaryResponse{
		Type:       string(s.Type),
		ID:         s.ID,
		GUID:       s.GUID,
		MID:        s.MID,
		Amount:     s.Amount,
		Currency:   s.Currency,
		Stage:      s.Stage,
		AssignedTo: s.AssignedTo,
		IsHidden:   s.IsHidden,
		CreatedAt:  s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  s.UpdatedAt.Format(time.RFC3339),
	}
}

func toCaseSummaries(items []domain.CaseSummary) []caseSummaryResponse {
	out := make([]caseSummaryResponse, len(items))
	for i, s := range items {
		out[i] = toCaseSummary(s)
	}
	return out
}

// Index lists cases of this type for the current tenant.
func (h *CaseHandler) Index(w http.ResponseWriter, r *http.Request) {
	f := parseCaseFilter(r)
	items, total, err := h.svc.List(r.Context(), h.typ, f)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page(toCaseSummaries(items), total, f))
}

type stageRequest struct {
	Stage string `json:"stage"`
	Notes string `json:"notes"`
}

// ChangeStage moves the case to a new workflow stage.
func (h *CaseHandler) ChangeStage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req stageRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.ChangeStage(r.Context(), h.typ, id, req.Stage, req.Notes); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "stage": req.Stage})
}

type assignRequest struct {
	AssignedTo *int64 `json:"assigned_to"`
}

// Assign sets or clears the case owner.
func (h *CaseHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req assignRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.Assign(r.Context(), h.typ, id, req.AssignedTo); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "assigned_to": req.AssignedTo})
}

type hideRequest struct {
	Hidden bool `json:"hidden"`
}

// Hide hides or unhides the case.
func (h *CaseHandler) Hide(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req hideRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.Hide(r.Context(), h.typ, id, req.Hidden); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "is_hidden": req.Hidden})
}

type resolveRequest struct {
	Resolution string `json:"resolution"`
}

// Resolve submits a resolution (preventions / rdr).
func (h *CaseHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req resolveRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.Resolve(r.Context(), h.typ, id, req.Resolution); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "resolution": req.Resolution})
}
