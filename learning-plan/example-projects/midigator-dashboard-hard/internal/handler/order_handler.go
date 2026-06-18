package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// OrderHandler serves the order endpoints: index/show plus create, update, and
// submit. Orders have no workflow stage, so this handler owns the full CRUD —
// there is no generic case handler half for them.
type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

type orderResponse struct {
	ID               int64           `json:"id"`
	OrderGUID        string          `json:"order_guid"`
	OrderID          string          `json:"order_id"`
	MID              string          `json:"mid"`
	OrderDate        *string         `json:"order_date"`
	OrderAmount      int64           `json:"order_amount"`
	Currency         string          `json:"currency"`
	CardBrand        string          `json:"card_brand"`
	CardFirst6       string          `json:"card_first_6"`
	CardLast4        string          `json:"card_last_4"`
	Email            string          `json:"email"`
	Phone            string          `json:"phone"`
	BillingFirstName string          `json:"billing_first_name"`
	BillingLastName  string          `json:"billing_last_name"`
	BillingAddress   json.RawMessage `json:"billing_address"`
	Items            json.RawMessage `json:"items"`
	MarketingSource  string          `json:"marketing_source"`
	IsHidden         bool            `json:"is_hidden"`
	SubmissionStatus string          `json:"submission_status"`
	SubmissionError  string          `json:"submission_error"`
	SubmittedAt      *string         `json:"submitted_at"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

func toOrderResponse(o *domain.Order) orderResponse {
	return orderResponse{
		ID:               o.ID,
		OrderGUID:        o.OrderGUID,
		OrderID:          o.OrderID,
		MID:              o.MID,
		OrderDate:        rfc3339(o.OrderDate),
		OrderAmount:      o.OrderAmount,
		Currency:         o.Currency,
		CardBrand:        o.CardBrand,
		CardFirst6:       o.CardFirst6,
		CardLast4:        o.CardLast4,
		Email:            o.Email,
		Phone:            o.Phone,
		BillingFirstName: o.BillingFirstName,
		BillingLastName:  o.BillingLastName,
		BillingAddress:   o.BillingAddress,
		Items:            o.Items,
		MarketingSource:  o.MarketingSource,
		IsHidden:         o.IsHidden,
		SubmissionStatus: o.SubmissionStatus,
		SubmissionError:  o.SubmissionError,
		SubmittedAt:      rfc3339(o.SubmittedAt),
		CreatedAt:        o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        o.UpdatedAt.Format(time.RFC3339),
	}
}

func toOrderResponses(items []domain.Order) []orderResponse {
	out := make([]orderResponse, len(items))
	for i := range items {
		out[i] = toOrderResponse(&items[i])
	}
	return out
}

// Index lists orders for the current tenant.
func (h *OrderHandler) Index(w http.ResponseWriter, r *http.Request) {
	f := parseCaseFilter(r)
	items, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page(toOrderResponses(items), total, f))
}

// Show returns one order.
func (h *OrderHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	o, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

// orderCreateRequest is the payload for POST /orders.
type orderCreateRequest struct {
	OrderGUID        string          `json:"order_guid"`
	OrderID          string          `json:"order_id"`
	MID              string          `json:"mid"`
	OrderDate        *time.Time      `json:"order_date"`
	OrderAmount      int64           `json:"order_amount"`
	Currency         string          `json:"currency"`
	CardBrand        string          `json:"card_brand"`
	CardFirst6       string          `json:"card_first_6"`
	CardLast4        string          `json:"card_last_4"`
	Email            string          `json:"email"`
	Phone            string          `json:"phone"`
	BillingFirstName string          `json:"billing_first_name"`
	BillingLastName  string          `json:"billing_last_name"`
	BillingAddress   json.RawMessage `json:"billing_address"`
	Items            json.RawMessage `json:"items"`
	Evidence         json.RawMessage `json:"evidence"`
}

func (req orderCreateRequest) toOrder() *domain.Order {
	return &domain.Order{
		OrderGUID:        req.OrderGUID,
		OrderID:          req.OrderID,
		MID:              req.MID,
		OrderDate:        req.OrderDate,
		OrderAmount:      req.OrderAmount,
		Currency:         req.Currency,
		CardBrand:        req.CardBrand,
		CardFirst6:       req.CardFirst6,
		CardLast4:        req.CardLast4,
		Email:            req.Email,
		Phone:            req.Phone,
		BillingFirstName: req.BillingFirstName,
		BillingLastName:  req.BillingLastName,
		BillingAddress:   req.BillingAddress,
		Items:            req.Items,
		Evidence:         req.Evidence,
	}
}

// Create records a new order.
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req orderCreateRequest
	if !decode(w, r, &req) {
		return
	}
	o, err := h.svc.Create(r.Context(), req.toOrder())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toOrderResponse(o))
}

// orderUpdateRequest is the editable subset (pointers => only set fields apply).
type orderUpdateRequest struct {
	OrderID          *string `json:"order_id"`
	MID              *string `json:"mid"`
	Email            *string `json:"email"`
	Phone            *string `json:"phone"`
	MarketingSource  *string `json:"marketing_source"`
	BillingFirstName *string `json:"billing_first_name"`
	BillingLastName  *string `json:"billing_last_name"`
}

func (req orderUpdateRequest) fields() map[string]any {
	f := map[string]any{}
	if req.OrderID != nil {
		f["order_id"] = *req.OrderID
	}
	if req.MID != nil {
		f["mid"] = *req.MID
	}
	if req.Email != nil {
		f["email"] = *req.Email
	}
	if req.Phone != nil {
		f["phone"] = *req.Phone
	}
	if req.MarketingSource != nil {
		f["marketing_source"] = *req.MarketingSource
	}
	if req.BillingFirstName != nil {
		f["billing_first_name"] = *req.BillingFirstName
	}
	if req.BillingLastName != nil {
		f["billing_last_name"] = *req.BillingLastName
	}
	return f
}

// Update edits an order's whitelisted fields.
func (h *OrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req orderUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	o, err := h.svc.Update(r.Context(), id, req.fields())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}

// Submit forwards an order to Midigator.
func (h *OrderHandler) Submit(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	o, err := h.svc.Submit(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(o))
}
