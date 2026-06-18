package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// RdrHandler serves the RDR-specific endpoints: show (with stage history) and
// update custom fields. The generic stage/assign/hide/resolve live in
// CaseHandler.
type RdrHandler struct {
	svc *service.RdrService
}

func NewRdrHandler(svc *service.RdrService) *RdrHandler {
	return &RdrHandler{svc: svc}
}

type rdrResponse struct {
	ID                 int64   `json:"id"`
	RdrGUID            string  `json:"rdr_guid"`
	EventGUID          string  `json:"event_guid"`
	RdrCaseNumber      string  `json:"rdr_case_number"`
	RdrDate            *string `json:"rdr_date"`
	RdrResolution      string  `json:"rdr_resolution"`
	ARN                string  `json:"arn"`
	AuthCode           string  `json:"auth_code"`
	Amount             int64   `json:"amount"`
	Currency           string  `json:"currency"`
	CardFirst6         string  `json:"card_first_6"`
	CardLast4          string  `json:"card_last_4"`
	MerchantDescriptor string  `json:"merchant_descriptor"`
	PreventionType     string  `json:"prevention_type"`
	TransactionDate    *string `json:"transaction_date"`
	OrderGUID          string  `json:"order_guid"`
	OrderID            string  `json:"order_id"`
	AssignedTo         *int64  `json:"assigned_to"`
	Stage              string  `json:"stage"`
	IsHidden           bool    `json:"is_hidden"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

func toRdrResponse(c *domain.RdrCase) rdrResponse {
	return rdrResponse{
		ID:                 c.ID,
		RdrGUID:            c.RdrGUID,
		EventGUID:          c.EventGUID,
		RdrCaseNumber:      c.RdrCaseNumber,
		RdrDate:            rfc3339(c.RdrDate),
		RdrResolution:      c.RdrResolution,
		ARN:                c.ARN,
		AuthCode:           c.AuthCode,
		Amount:             c.Amount,
		Currency:           c.Currency,
		CardFirst6:         c.CardFirst6,
		CardLast4:          c.CardLast4,
		MerchantDescriptor: c.MerchantDescriptor,
		PreventionType:     c.PreventionType,
		TransactionDate:    rfc3339(c.TransactionDate),
		OrderGUID:          c.OrderGUID,
		OrderID:            c.OrderID,
		AssignedTo:         c.AssignedTo,
		Stage:              c.Stage,
		IsHidden:           c.IsHidden,
		CreatedAt:          c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          c.UpdatedAt.Format(time.RFC3339),
	}
}

// Show returns one RDR case with its workflow history.
func (h *RdrHandler) Show(w http.ResponseWriter, r *http.Request) {
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
		"rdr":     toRdrResponse(d.Rdr),
		"history": toTransitions(d.History),
	})
}

// rdrUpdateRequest is the editable subset (pointers => only set fields apply).
type rdrUpdateRequest struct {
	MerchantDescriptor *string `json:"merchant_descriptor"`
	RdrCaseNumber      *string `json:"rdr_case_number"`
}

func (req rdrUpdateRequest) fields() map[string]any {
	f := map[string]any{}
	if req.MerchantDescriptor != nil {
		f["merchant_descriptor"] = *req.MerchantDescriptor
	}
	if req.RdrCaseNumber != nil {
		f["rdr_case_number"] = *req.RdrCaseNumber
	}
	return f
}

// Update edits an RDR case's custom fields.
func (h *RdrHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req rdrUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	rc, err := h.svc.Update(r.Context(), id, req.fields())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRdrResponse(rc))
}
