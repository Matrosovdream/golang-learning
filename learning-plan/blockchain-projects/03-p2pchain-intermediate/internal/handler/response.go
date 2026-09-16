package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"p2pchain/internal/domain"
	"p2pchain/internal/mempool"
)

// writeJSON writes any payload as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// errorBody has a human message and a stable machine-readable code, so a
// wallet can react to "nonce_too_low" without parsing English.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: message, Code: code})
}

// decodeJSON reads a JSON body (max 64 KB) into dst; unknown fields are rejected.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// errorMapping ties each domain/pool error to an HTTP status and a code.
//
// 400 = the request is malformed or can never be valid (bad encoding, bad
// signature, wrong chain). 422 = well-formed, but not valid against the current
// state (nonce, balance, fee) — the same bytes might be fine later or elsewhere.
var errorMapping = []struct {
	err    error
	status int
	code   string
}{
	{domain.ErrWrongChainID, http.StatusBadRequest, "wrong_chain_id"},
	{domain.ErrInvalidSignature, http.StatusBadRequest, "invalid_signature"},
	{domain.ErrNonceTooLow, http.StatusUnprocessableEntity, "nonce_too_low"},
	{domain.ErrNonceTooHigh, http.StatusUnprocessableEntity, "nonce_too_high"},
	{mempool.ErrNonceGap, http.StatusUnprocessableEntity, "nonce_gap"},
	{domain.ErrFeeTooLow, http.StatusUnprocessableEntity, "fee_too_low"},
	{domain.ErrInsufficientFunds, http.StatusUnprocessableEntity, "insufficient_funds"},
	{domain.ErrOverflow, http.StatusUnprocessableEntity, "overflow"},
	{mempool.ErrReplacementUnderpriced, http.StatusUnprocessableEntity, "replacement_underpriced"},
	{mempool.ErrAlreadyKnown, http.StatusConflict, "already_known"},
	{mempool.ErrPoolFull, http.StatusServiceUnavailable, "mempool_full"},
	{domain.ErrNotFound, http.StatusNotFound, "not_found"},
	{context.DeadlineExceeded, http.StatusGatewayTimeout, "timeout"},
	{context.Canceled, http.StatusServiceUnavailable, "cancelled"},
}

// writeServiceError maps an error to a status + code; unknown errors are 500s.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var ie domain.InputError
	if errors.As(err, &ie) {
		writeError(w, http.StatusBadRequest, "invalid_input", ie.Error())
		return
	}
	for _, m := range errorMapping {
		if errors.Is(err, m.err) {
			writeError(w, m.status, m.code, err.Error())
			return
		}
	}
	log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}
