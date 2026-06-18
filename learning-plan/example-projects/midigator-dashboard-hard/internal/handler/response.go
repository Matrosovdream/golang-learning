// Package handler is the HTTP transport layer: it decodes requests, calls the
// services, maps domain errors to status codes, and encodes JSON responses.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
)

// writeJSON encodes any payload with the given status.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError writes a uniform {"error": "..."} body.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decode reads a JSON request body into dst, writing a 400 on failure.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
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

// writeServiceError maps each domain error to the right HTTP status.
func writeServiceError(w http.ResponseWriter, err error) {
	var (
		ve domain.ValidationError
		nf domain.NotFoundError
		cf domain.ConflictError
	)
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.As(err, &nf):
		writeError(w, http.StatusNotFound, nf.Error())
	case errors.As(err, &cf):
		writeError(w, http.StatusConflict, cf.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// ---- shared query + formatting helpers -------------------------------------

// parseCaseFilter reads the list/search query params common to every case type.
func parseCaseFilter(r *http.Request) domain.CaseFilter {
	q := r.URL.Query()
	f := domain.CaseFilter{
		Stage:         q.Get("stage"),
		MID:           q.Get("mid"),
		Search:        q.Get("search"),
		IncludeHidden: q.Get("include_hidden") == "true",
		SortBy:        q.Get("sort_by"),
		SortDir:       q.Get("sort_dir"),
	}
	if v := q.Get("assigned_to"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.AssignedTo = &n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	f.Normalize()
	return f
}

// rfc3339 formats a time pointer as a string pointer (nil-safe), for JSON.
func rfc3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

// page wraps a list payload with pagination metadata.
func page(data any, total int, f domain.CaseFilter) map[string]any {
	return map[string]any{
		"data": data,
		"meta": map[string]any{"total": total, "limit": f.Limit, "offset": f.Offset},
	}
}
