package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"pinghub/internal/config"
	"pinghub/internal/domain"
	"pinghub/internal/service"
)

// CheckHandler turns HTTP requests into Checker calls and writes responses.
type CheckHandler struct {
	svc *service.Checker
	cfg config.Config
}

func NewCheckHandler(svc *service.Checker, cfg config.Config) *CheckHandler {
	return &CheckHandler{svc: svc, cfg: cfg}
}

type checkRequest struct {
	URLs      []string `json:"urls"`
	TimeoutMS int      `json:"timeout_ms"` // optional; per-URL timeout
}

type resultResponse struct {
	URL        string `json:"url"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type jobResponse struct {
	ID         int64            `json:"id"`
	Requested  int              `json:"requested"`
	Up         int              `json:"up"`
	Down       int              `json:"down"`
	Errored    int              `json:"errored"`
	DurationMS int64            `json:"duration_ms"`
	CreatedAt  string           `json:"created_at"`
	Results    []resultResponse `json:"results"`
}

// Create probes a batch of URLs concurrently and returns the job summary.
func (h *CheckHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req checkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	urls, err := h.validateURLs(req.URLs)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// r.Context() ties the batch to the request: if the client disconnects,
	// the whole fan-out is cancelled.
	job := h.svc.Run(r.Context(), urls, h.resolveTimeout(req.TimeoutMS))
	writeJSON(w, http.StatusCreated, toJobResponse(job))
}

// Get returns one stored job by ID.
func (h *CheckHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	job, err := h.svc.Get(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(job))
}

// List returns all stored jobs, newest first.
func (h *CheckHandler) List(w http.ResponseWriter, r *http.Request) {
	jobs := h.svc.List()
	resp := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		resp[i] = toJobResponse(j)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Stats exposes process-wide counters (the atomic total + number of jobs).
func (h *CheckHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int64{
		"total_checks": h.svc.TotalChecks(),
		"total_jobs":   int64(len(h.svc.List())),
	})
}

// Health is a trivial liveness probe.
func (h *CheckHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

func (h *CheckHandler) validateURLs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, domain.ValidationError{Message: "urls must not be empty"}
	}
	if len(raw) > h.cfg.MaxURLs {
		return nil, domain.ValidationError{Message: fmt.Sprintf("too many urls (max %d)", h.cfg.MaxURLs)}
	}
	out := make([]string, 0, len(raw))
	for _, u := range raw {
		parsed, err := url.Parse(u)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, domain.ValidationError{Message: "each url must be a valid http(s) url: " + u}
		}
		out = append(out, u)
	}
	return out, nil
}

func (h *CheckHandler) resolveTimeout(ms int) time.Duration {
	if ms <= 0 {
		return h.cfg.DefaultTimeout
	}
	if ms > 30000 { // clamp so one request can't pin a worker forever
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return 0, false
	}
	return id, true
}

func toJobResponse(j *domain.CheckJob) jobResponse {
	results := make([]resultResponse, len(j.Results))
	for i, res := range j.Results {
		results[i] = resultResponse{
			URL:        res.URL,
			Status:     string(res.Status),
			HTTPStatus: res.HTTPStatus,
			LatencyMS:  res.LatencyMS,
			Error:      res.Error,
		}
	}
	return jobResponse{
		ID:         j.ID,
		Requested:  j.Requested,
		Up:         j.Up,
		Down:       j.Down,
		Errored:    j.Errored,
		DurationMS: j.DurationMS,
		CreatedAt:  j.CreatedAt.Format(time.RFC3339),
		Results:    results,
	}
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "check job not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
