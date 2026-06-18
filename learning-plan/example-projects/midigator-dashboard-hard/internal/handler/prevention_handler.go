package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// PreventionHandler serves the prevention-specific endpoints: show (with stage
// history) and update custom fields.
type PreventionHandler struct {
	svc *service.PreventionService
}

func NewPreventionHandler(svc *service.PreventionService) *PreventionHandler {
	return &PreventionHandler{svc: svc}
}

type preventionResponse struct {
	ID                    int64   `json:"id"`
	PreventionGUID        string  `json:"prevention_guid"`
	EventGUID             string  `json:"event_guid"`
	PreventionCaseNumber  string  `json:"prevention_case_number"`
	PreventionType        string  `json:"prevention_type"`
	ARN                   string  `json:"arn"`
	MID                   string  `json:"mid"`
	Amount                int64   `json:"amount"`
	Currency              string  `json:"currency"`
	CardFirst6            string  `json:"card_first_6"`
	CardLast4             string  `json:"card_last_4"`
	MerchantDescriptor    string  `json:"merchant_descriptor"`
	PreventionTimestamp   *string `json:"prevention_timestamp"`
	TransactionTimestamp  *string `json:"transaction_timestamp"`
	ResolutionType        string  `json:"resolution_type"`
	ResolutionSubmittedAt *string `json:"resolution_submitted_at"`
	OrderGUID             string  `json:"order_guid"`
	OrderID               string  `json:"order_id"`
	AssignedTo            *int64  `json:"assigned_to"`
	Stage                 string  `json:"stage"`
	IsHidden              bool    `json:"is_hidden"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

func toPreventionResponse(p *domain.PreventionAlert) preventionResponse {
	return preventionResponse{
		ID:                    p.ID,
		PreventionGUID:        p.PreventionGUID,
		EventGUID:             p.EventGUID,
		PreventionCaseNumber:  p.PreventionCaseNumber,
		PreventionType:        p.PreventionType,
		ARN:                   p.ARN,
		MID:                   p.MID,
		Amount:                p.Amount,
		Currency:              p.Currency,
		CardFirst6:            p.CardFirst6,
		CardLast4:             p.CardLast4,
		MerchantDescriptor:    p.MerchantDescriptor,
		PreventionTimestamp:   rfc3339(p.PreventionTimestamp),
		TransactionTimestamp:  rfc3339(p.TransactionTimestamp),
		ResolutionType:        p.ResolutionType,
		ResolutionSubmittedAt: rfc3339(p.ResolutionSubmittedAt),
		OrderGUID:             p.OrderGUID,
		OrderID:               p.OrderID,
		AssignedTo:            p.AssignedTo,
		Stage:                 p.Stage,
		IsHidden:              p.IsHidden,
		CreatedAt:             p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             p.UpdatedAt.Format(time.RFC3339),
	}
}

// Show returns one prevention alert with its workflow history.
func (h *PreventionHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"prevention": toPreventionResponse(d.Prevention),
		"history":    toTransitions(d.History),
	})
}

// preventionUpdateRequest is the editable subset (pointers => only set fields apply).
type preventionUpdateRequest struct {
	MerchantDescriptor   *string `json:"merchant_descriptor"`
	ResolutionType       *string `json:"resolution_type"`
	PreventionCaseNumber *string `json:"prevention_case_number"`
}

func (req preventionUpdateRequest) fields() map[string]any {
	f := map[string]any{}
	if req.MerchantDescriptor != nil {
		f["merchant_descriptor"] = *req.MerchantDescriptor
	}
	if req.ResolutionType != nil {
		f["resolution_type"] = *req.ResolutionType
	}
	if req.PreventionCaseNumber != nil {
		f["prevention_case_number"] = *req.PreventionCaseNumber
	}
	return f
}

// Update edits a prevention alert's custom fields.
func (h *PreventionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req preventionUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	pa, err := h.svc.Update(r.Context(), id, req.fields())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPreventionResponse(pa))
}
