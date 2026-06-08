package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"orderservice/internal/domain"
	"orderservice/internal/service"
)

// OrderHandler turns HTTP requests into order-service calls.
type OrderHandler struct {
	svc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

type checkoutItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type checkoutRequest struct {
	Items []checkoutItem `json:"items"`
}

type orderItemResponse struct {
	ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type orderResponse struct {
	ID         int64               `json:"id"`
	Status     string              `json:"status"`
	TotalCents int64               `json:"total_cents"`
	Items      []orderItemResponse `json:"items"`
	CreatedAt  string              `json:"created_at"`
	UpdatedAt  string              `json:"updated_at"`
}

type orderSummary struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	TotalCents int64  `json:"total_cents"`
	CreatedAt  string `json:"created_at"`
}

func (h *OrderHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	lines := make([]domain.OrderLine, len(req.Items))
	for i, it := range req.Items {
		lines[i] = domain.OrderLine{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	order, err := h.svc.Checkout(r.Context(), lines)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toOrderResponse(order))
}

func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	orders, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resp := make([]orderSummary, len(orders))
	for i, o := range orders {
		resp[i] = orderSummary{
			ID:         o.ID,
			Status:     string(o.Status),
			TotalCents: o.TotalCents,
			CreatedAt:  o.CreatedAt.Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	order, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

func (h *OrderHandler) Pay(w http.ResponseWriter, r *http.Request)    { h.transition(w, r, h.svc.Pay) }
func (h *OrderHandler) Ship(w http.ResponseWriter, r *http.Request)   { h.transition(w, r, h.svc.Ship) }
func (h *OrderHandler) Cancel(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.svc.Cancel) }

// transition is the shared body for pay/ship/cancel.
func (h *OrderHandler) transition(w http.ResponseWriter, r *http.Request, action func(context.Context, int64) (*domain.Order, error)) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	order, err := action(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

func toOrderResponse(o *domain.Order) orderResponse {
	items := make([]orderItemResponse, len(o.Items))
	for i, it := range o.Items {
		items[i] = orderItemResponse{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		}
	}
	return orderResponse{
		ID:         o.ID,
		Status:     string(o.Status),
		TotalCents: o.TotalCents,
		Items:      items,
		CreatedAt:  o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  o.UpdatedAt.Format(time.RFC3339),
	}
}
