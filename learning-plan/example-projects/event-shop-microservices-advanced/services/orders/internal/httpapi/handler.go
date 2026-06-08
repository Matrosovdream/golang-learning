package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"eventshop/pkg/events"
	"eventshop/services/orders/internal/domain"
	"eventshop/services/orders/internal/service"
)

// Handler is the orders REST surface.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.place)
	mux.HandleFunc("GET /orders/{id}", h.get)
	return mux
}

type placeRequest struct {
	UserID int64                  `json:"user_id"`
	Items  []events.RequestedItem `json:"items"`
}

func (h *Handler) place(w http.ResponseWriter, r *http.Request) {
	var req placeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	order, err := h.svc.PlaceOrder(r.Context(), req.UserID, req.Items)
	if err != nil {
		var ve domain.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not place order")
		return
	}
	// 202 Accepted: the order is recorded as pending; fulfilment happens async.
	writeJSON(w, http.StatusAccepted, toResponse(order))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "order not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get order")
		return
	}
	writeJSON(w, http.StatusOK, toResponse(order))
}

type itemResponse struct {
	ProductID      int64  `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type orderResponse struct {
	ID         int64          `json:"id"`
	UserID     int64          `json:"user_id"`
	Status     string         `json:"status"`
	TotalCents int64          `json:"total_cents"`
	Items      []itemResponse `json:"items"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

func toResponse(o *domain.Order) orderResponse {
	items := make([]itemResponse, len(o.Items))
	for i, it := range o.Items {
		items[i] = itemResponse{
			ProductID:      it.ProductID,
			ProductName:    it.ProductName,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		}
	}
	return orderResponse{
		ID:         o.ID,
		UserID:     o.UserID,
		Status:     o.Status,
		TotalCents: o.TotalCents,
		Items:      items,
		CreatedAt:  o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  o.UpdatedAt.Format(time.RFC3339),
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
