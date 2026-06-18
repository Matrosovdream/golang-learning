package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// OrderValidationHandler serves the order-validation-specific endpoints: show
// (with stage history) and update custom fields.
type OrderValidationHandler struct {
	svc *service.OrderValidationService
}

func NewOrderValidationHandler(svc *service.OrderValidationService) *OrderValidationHandler {
	return &OrderValidationHandler{svc: svc}
}

type order_validationResponse struct {
	ID                       int64   `json:"id"`
	OrderValidationGUID      string  `json:"order_validation_guid"`
	EventGUID                string  `json:"event_guid"`
	OrderValidationType      string  `json:"order_validation_type"`
	OrderValidationTimestamp *string `json:"order_validation_timestamp"`
	TransactionTimestamp     *string `json:"transaction_timestamp"`
	Amount                   int64   `json:"amount"`
	Currency                 string  `json:"currency"`
	CardFirst6               string  `json:"card_first_6"`
	CardLast4                string  `json:"card_last_4"`
	ARN                      string  `json:"arn"`
	AuthCode                 string  `json:"auth_code"`
	CardBrand                string  `json:"card_brand"`
	OrderGUID                string  `json:"order_guid"`
	OrderID                  string  `json:"order_id"`
	MID                      string  `json:"mid"`
	AssignedTo               *int64  `json:"assigned_to"`
	Stage                    string  `json:"stage"`
	IsHidden                 bool    `json:"is_hidden"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}

func toOrderValidationResponse(v *domain.OrderValidation) order_validationResponse {
	return order_validationResponse{
		ID:                       v.ID,
		OrderValidationGUID:      v.OrderValidationGUID,
		EventGUID:                v.EventGUID,
		OrderValidationType:      v.OrderValidationType,
		OrderValidationTimestamp: rfc3339(v.OrderValidationTimestamp),
		TransactionTimestamp:     rfc3339(v.TransactionTimestamp),
		Amount:                   v.Amount,
		Currency:                 v.Currency,
		CardFirst6:               v.CardFirst6,
		CardLast4:                v.CardLast4,
		ARN:                      v.ARN,
		AuthCode:                 v.AuthCode,
		CardBrand:                v.CardBrand,
		OrderGUID:                v.OrderGUID,
		OrderID:                  v.OrderID,
		MID:                      v.MID,
		AssignedTo:               v.AssignedTo,
		Stage:                    v.Stage,
		IsHidden:                 v.IsHidden,
		CreatedAt:                v.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                v.UpdatedAt.Format(time.RFC3339),
	}
}

// Show returns one order validation with its workflow history.
func (h *OrderValidationHandler) Show(w http.ResponseWriter, r *http.Request) {
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
		"order_validation": toOrderValidationResponse(d.OrderValidation),
		"history":          toTransitions(d.History),
	})
}

// order_validationUpdateRequest is the editable subset (pointers => only set fields apply).
type order_validationUpdateRequest struct {
	OrderValidationType *string `json:"order_validation_type"`
}

func (req order_validationUpdateRequest) fields() map[string]any {
	f := map[string]any{}
	if req.OrderValidationType != nil {
		f["order_validation_type"] = *req.OrderValidationType
	}
	return f
}

// Update edits an order validation's custom fields.
func (h *OrderValidationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req order_validationUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.Update(r.Context(), id, req.fields())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderValidationResponse(v))
}
