package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"orderservice/internal/domain"
)

// writeJSON writes any payload as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError writes a uniform JSON error body: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// parseID reads a positive int64 path value or writes a 400.
func parseID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

// writeServiceError maps every domain error to the right HTTP status.
func writeServiceError(w http.ResponseWriter, err error) {
	var (
		ve    domain.ValidationError
		stock domain.InsufficientStockError
		trans domain.InvalidTransitionError
	)
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.As(err, &stock):
		writeError(w, http.StatusConflict, stock.Error())
	case errors.As(err, &trans):
		writeError(w, http.StatusConflict, trans.Error())
	case errors.Is(err, domain.ErrSKUTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrProductNotFound):
		writeError(w, http.StatusNotFound, "product not found")
	case errors.Is(err, domain.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, "order not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
