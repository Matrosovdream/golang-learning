package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pipelinehub/internal/config"
	"pipelinehub/internal/domain"
	"pipelinehub/internal/service"
)

// AnalysisHandler turns HTTP requests into Pipeline calls and writes responses.
// It holds a *service.Pipeline (shared pointer) and the config by value.
type AnalysisHandler struct {
	svc *service.Pipeline
	cfg config.Config
}

func NewAnalysisHandler(svc *service.Pipeline, cfg config.Config) *AnalysisHandler {
	return &AnalysisHandler{svc: svc, cfg: cfg}
}

// Request/response DTOs. The `json:"..."` struct tags map Go fields to JSON
// keys; omitempty drops the field from the output when it's the zero value.
type analyzeRequest struct {
	Text   string `json:"text"`
	TopN   int    `json:"top_n"`   // optional; how many top words to return
	MinLen int    `json:"min_len"` // optional; minimum word length to keep
}

type wordCountResponse struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type analysisResponse struct {
	ID         int64               `json:"id"`
	Lines      int                 `json:"lines"`
	Words      int                 `json:"words"`
	Unique     int                 `json:"unique"`
	DurationMS int64               `json:"duration_ms"`
	CreatedAt  string              `json:"created_at"`
	Top        []wordCountResponse `json:"top"`
}

// Create runs a text blob through the pipeline and returns the analysis summary.
// Method signature (w, r) is what http.HandlerFunc requires.
func (h *AnalysisHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req analyzeRequest // zero-valued struct to decode into
	// Decode reads the request body into &req (a pointer, so it can fill it).
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.validate(req.Text); err != nil {
		writeServiceError(w, err)
		return
	}

	// r.Context() ties the pipeline to the request: if the client disconnects,
	// that ctx is cancelled and every stage tears down via its ctx.Done() case.
	a := h.svc.Analyze(r.Context(), req.Text, h.resolveTopN(req.TopN), h.resolveMinLen(req.MinLen))
	writeJSON(w, http.StatusCreated, toAnalysisResponse(a))
}

// Get returns one stored analysis by ID.
func (h *AnalysisHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok { // parseID already wrote the 400; just stop here
		return
	}
	a, err := h.svc.Get(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAnalysisResponse(a))
}

// List returns all stored analyses, newest first.
func (h *AnalysisHandler) List(w http.ResponseWriter, r *http.Request) {
	items := h.svc.List()
	// Pre-size a slice to len(items), then fill by index.
	resp := make([]analysisResponse, len(items))
	for i, a := range items { // range gives (index, value)
		resp[i] = toAnalysisResponse(a)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Stats exposes process-wide counters (the atomic total + number of analyses).
func (h *AnalysisHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int64{
		"total_words":    h.svc.TotalWords(),
		"total_analyses": int64(len(h.svc.List())), // int64(...) is a type conversion
	})
}

// Health is a trivial liveness probe.
func (h *AnalysisHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

// validate rejects empty or oversized text. Returning the error type lets the
// caller map it to the right HTTP status.
func (h *AnalysisHandler) validate(text string) error {
	if strings.TrimSpace(text) == "" {
		return domain.ValidationError{Message: "text must not be empty"}
	}
	if len(text) > h.cfg.MaxBytes { // len on a string counts BYTES, not runes
		// fmt.Sprintf builds a string from a format + args.
		return domain.ValidationError{Message: fmt.Sprintf("text too large (max %d bytes)", h.cfg.MaxBytes)}
	}
	return nil
}

// resolveTopN falls back to the config default and clamps the upper bound.
func (h *AnalysisHandler) resolveTopN(n int) int {
	if n <= 0 {
		return h.cfg.DefaultTopN
	}
	if n > 100 {
		n = 100
	}
	return n
}

// resolveMinLen falls back to the config default and clamps the upper bound.
func (h *AnalysisHandler) resolveMinLen(n int) int {
	if n <= 0 {
		return h.cfg.MinWordLen
	}
	if n > 20 {
		n = 20
	}
	return n
}

// parseID reads the {id} path segment; returns (id, false) and writes a 400 on
// bad input so callers can just `if !ok { return }`.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	// PathValue reads a wildcard captured by the route pattern ("/analyze/{id}").
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid analysis id")
		return 0, false
	}
	return id, true
}

// toAnalysisResponse maps a domain.Analysis onto the wire DTO (decoupling the two).
func toAnalysisResponse(a *domain.Analysis) analysisResponse {
	top := make([]wordCountResponse, len(a.Top))
	for i, wc := range a.Top {
		top[i] = wordCountResponse{Word: wc.Word, Count: wc.Count}
	}
	return analysisResponse{
		ID:         a.ID,
		Lines:      a.Lines,
		Words:      a.Words,
		Unique:     a.Unique,
		DurationMS: a.DurationMS,
		CreatedAt:  a.CreatedAt.Format(time.RFC3339), // format a time.Time as a string
		Top:        top,
	}
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	// errors.As checks if err is (or wraps) a ValidationError, filling ve if so.
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	// errors.Is compares against a sentinel error value.
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "analysis not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
