package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"watchhub/internal/config"
	"watchhub/internal/domain"
	"watchhub/internal/service"
)

// MonitorHandler serves the monitor CRUD routes plus the ad-hoc batch check.
type MonitorHandler struct {
	sched   *service.Scheduler
	checker *service.Checker
	cfg     config.Config
}

func NewMonitorHandler(sched *service.Scheduler, checker *service.Checker, cfg config.Config) *MonitorHandler {
	return &MonitorHandler{sched: sched, checker: checker, cfg: cfg}
}

// --- DTOs ---

type createMonitorRequest struct {
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutMS       int    `json:"timeout_ms"`
}

type resultResponse struct {
	CheckedAt  string `json:"checked_at"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	Attempts   int    `json:"attempts,omitempty"`
	Error      string `json:"error,omitempty"`
}

type monitorResponse struct {
	ID               int64            `json:"id"`
	URL              string           `json:"url"`
	IntervalSeconds  float64          `json:"interval_seconds"`
	TimeoutMS        int64            `json:"timeout_ms"`
	Status           string           `json:"status"`
	LastCheckedAt    string           `json:"last_checked_at,omitempty"`
	LastLatencyMS    int64            `json:"last_latency_ms"`
	TotalChecks      int64            `json:"total_checks"`
	UpChecks         int64            `json:"up_checks"`
	UptimePct        float64          `json:"uptime_pct"`
	ConsecutiveFails int              `json:"consecutive_fails"`
	CreatedAt        string           `json:"created_at"`
	History          []resultResponse `json:"history,omitempty"`
}

// Create registers a monitor and starts watching it in the background.
func (h *MonitorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	in, err := h.validate(req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if h.sched.Count() >= h.cfg.MaxMonitors {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("monitor limit reached (max %d)", h.cfg.MaxMonitors))
		return
	}
	view := h.sched.AddMonitor(in)
	writeJSON(w, http.StatusCreated, toMonitorResponse(view))
}

// List returns all monitors with their latest status (no history).
func (h *MonitorHandler) List(w http.ResponseWriter, r *http.Request) {
	views := h.sched.ListMonitors()
	resp := make([]monitorResponse, len(views))
	for i, v := range views {
		resp[i] = toMonitorResponse(v)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get returns one monitor including its recent history.
func (h *MonitorHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	view, err := h.sched.GetMonitor(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMonitorResponse(view))
}

// Delete stops watching and removes a monitor.
func (h *MonitorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.sched.RemoveMonitor(id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CheckNow triggers an immediate out-of-band check.
func (h *MonitorHandler) CheckNow(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	res, err := h.sched.CheckNow(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResultResponse(res))
}

type batchRequest struct {
	URLs      []string `json:"urls"`
	TimeoutMS int      `json:"timeout_ms"`
}

type batchResponse struct {
	Requested  int              `json:"requested"`
	Up         int              `json:"up"`
	Down       int              `json:"down"`
	Errored    int              `json:"errored"`
	DurationMS int64            `json:"duration_ms"`
	Results    []resultResponse `json:"results"`
}

// Batch probes a list of URLs concurrently using errgroup with a concurrency
// limit — a one-shot check that does NOT create monitors. Contrast with the
// scheduler's hand-rolled goroutine/channel pool: errgroup.SetLimit gives you a
// bounded pool plus a single Wait() in a few lines.
func (h *MonitorHandler) Batch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	urls, err := h.validateURLs(req.URLs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	timeout := h.resolveTimeout(req.TimeoutMS)

	start := time.Now()
	results := make([]domain.CheckResult, len(urls)) // index i is owned by goroutine i — no shared writes

	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(h.cfg.MaxConcurrent) // bound THIS batch (the checker also has a global gate)
	for i, u := range urls {
		g.Go(func() error {
			results[i] = h.checker.Check(ctx, u, timeout)
			return nil
		})
	}
	_ = g.Wait() // we never return an error from a goroutine, so this is always nil

	resp := batchResponse{Requested: len(urls), DurationMS: time.Since(start).Milliseconds()}
	for _, res := range results {
		switch res.Status {
		case domain.StatusUp:
			resp.Up++
		case domain.StatusDown:
			resp.Down++
		default:
			resp.Errored++
		}
		resp.Results = append(resp.Results, toResultResponse(res))
	}
	writeJSON(w, http.StatusOK, resp)
}

// Stats exposes global counters (monitors, checks, in-flight, goroutines).
func (h *MonitorHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.sched.Stats())
}

// Health is a trivial liveness probe.
func (h *MonitorHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

func (h *MonitorHandler) validate(req createMonitorRequest) (service.AddMonitorInput, error) {
	u, err := parseHTTPURL(req.URL)
	if err != nil {
		return service.AddMonitorInput{}, err
	}
	interval := req.IntervalSeconds
	if interval == 0 {
		interval = 30
	}
	if interval < 1 || interval > 3600 {
		return service.AddMonitorInput{}, domain.ValidationError{Message: "interval_seconds must be between 1 and 3600"}
	}
	timeout := h.resolveTimeout(req.TimeoutMS)
	if timeout > time.Duration(interval)*time.Second {
		return service.AddMonitorInput{}, domain.ValidationError{Message: "timeout_ms must not exceed the interval"}
	}
	return service.AddMonitorInput{
		URL:      u,
		Interval: time.Duration(interval) * time.Second,
		Timeout:  timeout,
	}, nil
}

func (h *MonitorHandler) validateURLs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, domain.ValidationError{Message: "urls must not be empty"}
	}
	if len(raw) > h.cfg.MaxMonitors {
		return nil, domain.ValidationError{Message: fmt.Sprintf("too many urls (max %d)", h.cfg.MaxMonitors)}
	}
	out := make([]string, 0, len(raw))
	for _, u := range raw {
		valid, err := parseHTTPURL(u)
		if err != nil {
			return nil, err
		}
		out = append(out, valid)
	}
	return out, nil
}

func (h *MonitorHandler) resolveTimeout(ms int) time.Duration {
	if ms <= 0 {
		return h.cfg.DefaultTimeout
	}
	if ms > 30000 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

func parseHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", domain.ValidationError{Message: "must be a valid http(s) url: " + raw}
	}
	return raw, nil
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid monitor id")
		return 0, false
	}
	return id, true
}

func toResultResponse(res domain.CheckResult) resultResponse {
	out := resultResponse{
		Status:     string(res.Status),
		HTTPStatus: res.HTTPStatus,
		LatencyMS:  res.LatencyMS,
		Attempts:   res.Attempts,
		Error:      res.Error,
	}
	if !res.CheckedAt.IsZero() {
		out.CheckedAt = res.CheckedAt.Format(time.RFC3339)
	}
	return out
}

func toMonitorResponse(v domain.MonitorView) monitorResponse {
	out := monitorResponse{
		ID:               v.ID,
		URL:              v.URL,
		IntervalSeconds:  v.Interval.Seconds(),
		TimeoutMS:        v.Timeout.Milliseconds(),
		Status:           string(v.Status),
		LastLatencyMS:    v.LastLatencyMS,
		TotalChecks:      v.TotalChecks,
		UpChecks:         v.UpChecks,
		UptimePct:        v.UptimePct,
		ConsecutiveFails: v.ConsecutiveFails,
		CreatedAt:        v.CreatedAt.Format(time.RFC3339),
	}
	if !v.LastCheckedAt.IsZero() {
		out.LastCheckedAt = v.LastCheckedAt.Format(time.RFC3339)
	}
	for _, res := range v.History {
		out.History = append(out.History, toResultResponse(res))
	}
	return out
}

func writeServiceError(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "monitor not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
