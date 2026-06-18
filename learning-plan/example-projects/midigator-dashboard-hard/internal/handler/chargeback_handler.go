package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// ChargebackHandler serves the chargeback-specific endpoints: show (with stage
// history), update custom fields, and bulk hide.
type ChargebackHandler struct {
	svc *service.ChargebackService
}

func NewChargebackHandler(svc *service.ChargebackService) *ChargebackHandler {
	return &ChargebackHandler{svc: svc}
}

type chargebackResponse struct {
	ID                int64   `json:"id"`
	ChargebackGUID    string  `json:"chargeback_guid"`
	EventGUID         string  `json:"event_guid"`
	CaseNumber        string  `json:"case_number"`
	ARN               string  `json:"arn"`
	MID               string  `json:"mid"`
	Amount            int64   `json:"amount"`
	Currency          string  `json:"currency"`
	CardBrand         string  `json:"card_brand"`
	CardFirst6        string  `json:"card_first_6"`
	CardLast4         string  `json:"card_last_4"`
	ReasonCode        string  `json:"reason_code"`
	ReasonDescription string  `json:"reason_description"`
	ChargebackDate    *string `json:"chargeback_date"`
	DateReceived      *string `json:"date_received"`
	DueDate           *string `json:"due_date"`
	AuthCode          string  `json:"auth_code"`
	Result            string  `json:"result"`
	DNFReason         string  `json:"dnf_reason"`
	OrderGUID         string  `json:"order_guid"`
	OrderID           string  `json:"order_id"`
	AssignedTo        *int64  `json:"assigned_to"`
	Stage             string  `json:"stage"`
	IsHidden          bool    `json:"is_hidden"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

func toChargebackResponse(c *domain.Chargeback) chargebackResponse {
	return chargebackResponse{
		ID:                c.ID,
		ChargebackGUID:    c.ChargebackGUID,
		EventGUID:         c.EventGUID,
		CaseNumber:        c.CaseNumber,
		ARN:               c.ARN,
		MID:               c.MID,
		Amount:            c.Amount,
		Currency:          c.Currency,
		CardBrand:         c.CardBrand,
		CardFirst6:        c.CardFirst6,
		CardLast4:         c.CardLast4,
		ReasonCode:        c.ReasonCode,
		ReasonDescription: c.ReasonDescription,
		ChargebackDate:    rfc3339(c.ChargebackDate),
		DateReceived:      rfc3339(c.DateReceived),
		DueDate:           rfc3339(c.DueDate),
		AuthCode:          c.AuthCode,
		Result:            c.Result,
		DNFReason:         c.DNFReason,
		OrderGUID:         c.OrderGUID,
		OrderID:           c.OrderID,
		AssignedTo:        c.AssignedTo,
		Stage:             c.Stage,
		IsHidden:          c.IsHidden,
		CreatedAt:         c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         c.UpdatedAt.Format(time.RFC3339),
	}
}

type transitionResponse struct {
	FromStage string `json:"from_stage"`
	ToStage   string `json:"to_stage"`
	UserName  string `json:"user_name"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`
}

func toTransitions(items []domain.StageTransition) []transitionResponse {
	out := make([]transitionResponse, len(items))
	for i, t := range items {
		out[i] = transitionResponse{
			FromStage: t.FromStage,
			ToStage:   t.ToStage,
			UserName:  t.UserName,
			Notes:     t.Notes,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

// Show returns one chargeback with its workflow history.
func (h *ChargebackHandler) Show(w http.ResponseWriter, r *http.Request) {
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
		"chargeback": toChargebackResponse(d.Chargeback),
		"history":    toTransitions(d.History),
	})
}

// chargebackUpdateRequest is the editable subset (pointers => only set fields apply).
type chargebackUpdateRequest struct {
	CaseNumber        *string `json:"case_number"`
	ARN               *string `json:"arn"`
	ReasonCode        *string `json:"reason_code"`
	ReasonDescription *string `json:"reason_description"`
	Result            *string `json:"result"`
	DNFReason         *string `json:"dnf_reason"`
}

func (req chargebackUpdateRequest) fields() map[string]any {
	f := map[string]any{}
	if req.CaseNumber != nil {
		f["case_number"] = *req.CaseNumber
	}
	if req.ARN != nil {
		f["arn"] = *req.ARN
	}
	if req.ReasonCode != nil {
		f["reason_code"] = *req.ReasonCode
	}
	if req.ReasonDescription != nil {
		f["reason_description"] = *req.ReasonDescription
	}
	if req.Result != nil {
		f["result"] = *req.Result
	}
	if req.DNFReason != nil {
		f["dnf_reason"] = *req.DNFReason
	}
	return f
}

// Update edits a chargeback's custom fields.
func (h *ChargebackHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req chargebackUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	cb, err := h.svc.Update(r.Context(), id, req.fields())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toChargebackResponse(cb))
}

type bulkHideRequest struct {
	IDs    []int64 `json:"ids"`
	Hidden bool    `json:"hidden"`
}

// BulkHide hides/unhides many chargebacks at once.
func (h *ChargebackHandler) BulkHide(w http.ResponseWriter, r *http.Request) {
	var req bulkHideRequest
	if !decode(w, r, &req) {
		return
	}
	n, err := h.svc.BulkHide(r.Context(), req.IDs, req.Hidden)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n, "hidden": req.Hidden})
}
