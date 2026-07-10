package handler

import (
	"encoding/json"
	"net/http"
)

// writeJSON encodes any payload as JSON with the given status.
// `any` is the empty interface (alias for interface{}): it accepts any type.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status) // must set the status BEFORE writing the body
	// NewEncoder(w).Encode streams JSON straight to the response writer.
	_ = json.NewEncoder(w).Encode(payload) // _ = deliberately ignores the error
}

// writeError writes a uniform JSON error body: {"error": "..."}.
// A map[string]string marshals to a small JSON object.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
