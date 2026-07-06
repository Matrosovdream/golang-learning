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

// svc is a *service.Service pointer — the handler shares one service instance.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// Returns http.Handler, an interface; *http.ServeMux satisfies it.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.place) // h.place is a method value bound to h
	mux.HandleFunc("GET /orders/{id}", h.get)
	return mux
}

type placeRequest struct {
	UserID int64                  `json:"user_id"`
	Items  []events.RequestedItem `json:"items"`
}

// http.ResponseWriter is an interface; *http.Request is a pointer (a large
// struct you never copy).
func (h *Handler) place(w http.ResponseWriter, r *http.Request) {
	var req placeRequest
	// &req: pass the address so the decoder fills the struct in place.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	order, err := h.svc.PlaceOrder(r.Context(), req.UserID, req.Items)
	if err != nil {
		var ve domain.ValidationError
		// errors.As checks whether err (or anything it wraps) is a
		// ValidationError, and if so copies it into ve via the &ve pointer.
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not place order")
		return
	}
	writeJSON(w, http.StatusAccepted, toResponse(order))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	// ParseInt returns (int64, error); 10 = base, 64 = bit size.
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	order, err := h.svc.GetOrder(r.Context(), id)
	if err != nil {
		// errors.Is compares against a sentinel value, through the wrap chain.
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

// Pointer in (*domain.Order), value out (orderResponse).
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

// payload any: `any` is an alias for interface{} — it accepts a value of any type.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// map[string]string{...} is a map literal; json.Marshal turns it into a JSON object.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
