package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"crawlerhub/internal/config"
	"crawlerhub/internal/domain"
	"crawlerhub/internal/service"
)

// Sane upper bounds a client request is clamped to, whatever it asks for. They
// keep one POST from launching an unbounded crawl. (Concurrency is clamped to
// the config's MaxConcurrency instead — that's the semaphore ceiling.)
const (
	maxDepthCap = 10
	maxPagesCap = 1000
	rateCap     = 1000
)

// CrawlHandler turns HTTP requests into Crawler calls and writes responses.
// It holds a *service.Crawler (shared pointer) and the config by value.
type CrawlHandler struct {
	svc *service.Crawler
	cfg config.Config
}

func NewCrawlHandler(svc *service.Crawler, cfg config.Config) *CrawlHandler {
	return &CrawlHandler{svc: svc, cfg: cfg}
}

// Request/response DTOs. The `json:"..."` struct tags map Go fields to JSON keys;
// omitempty drops the field from the output when it's the zero value.
type crawlRequest struct {
	SeedURL     string `json:"seed_url"`
	MaxDepth    int    `json:"max_depth"`
	MaxPages    int    `json:"max_pages"`
	Concurrency int    `json:"concurrency"`
	RatePerSec  int    `json:"rate_per_sec"`
}

type pageResponse struct {
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status,omitempty"`
	LinksFound int    `json:"links_found"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type jobResponse struct {
	ID              int64          `json:"id"`
	Seed            string         `json:"seed"`
	Pages           int            `json:"pages"`
	Edges           int            `json:"edges"`
	MaxDepthReached int            `json:"max_depth_reached"`
	Fetches         int            `json:"fetches"`
	DurationMS      int64          `json:"duration_ms"`
	CreatedAt       string         `json:"created_at"`
	Results         []pageResponse `json:"results"`
}

// Create launches a crawl and returns the job summary.
// Method signature (w, r) is what http.HandlerFunc requires.
func (h *CrawlHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req crawlRequest
	// Decode the body if there is one; an empty body means "use all defaults".
	// io.EOF is the empty-body case and is not an error here.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	params, err := h.buildParams(r, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// r.Context() ties the crawl to the request: if the client disconnects, that
	// ctx is cancelled and every in-flight fetch and new spawn stops.
	job, err := h.svc.Run(r.Context(), params)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toJobResponse(job))
}

// Get returns one stored crawl by ID.
func (h *CrawlHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok { // parseID already wrote the 400; just stop here
		return
	}
	job, err := h.svc.Get(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(job))
}

// List returns all stored crawls, newest first.
func (h *CrawlHandler) List(w http.ResponseWriter, r *http.Request) {
	jobs := h.svc.List()
	// Pre-size a slice to len(jobs), then fill by index.
	resp := make([]jobResponse, len(jobs))
	for i, j := range jobs { // range gives (index, value)
		resp[i] = toJobResponse(j)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Stats exposes process-wide counters (both atomics under the hood).
func (h *CrawlHandler) Stats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int64{
		"total_fetches": h.svc.TotalFetches(),
		"total_crawls":  h.svc.TotalCrawls(),
	})
}

// Health is a trivial liveness probe.
func (h *CrawlHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

// buildParams validates the seed and clamps every knob to a sane range, filling
// in config defaults for anything the client omitted (a 0 field).
func (h *CrawlHandler) buildParams(r *http.Request, req crawlRequest) (service.CrawlParams, error) {
	seed := req.SeedURL
	if seed == "" {
		// Default seed = the built-in mini-site on THIS server. Building it from
		// r.Host (whatever the client dialed) makes it work on localhost AND in
		// Docker without hard-coding a hostname.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		seed = scheme + "://" + r.Host + "/site/0"
	} else {
		// url.Parse returns (*url.URL, error); reject anything not http(s).
		u, err := url.Parse(seed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return service.CrawlParams{}, domain.ValidationError{Message: "seed_url must be a valid http(s) url"}
		}
	}

	return service.CrawlParams{
		Seed:         seed,
		MaxDepth:     clamp(req.MaxDepth, h.cfg.DefaultMaxDepth, maxDepthCap),
		MaxPages:     clamp(req.MaxPages, h.cfg.DefaultMaxPages, maxPagesCap),
		Concurrency:  clamp(req.Concurrency, h.cfg.MaxConcurrency, h.cfg.MaxConcurrency),
		RatePerSec:   clamp(req.RatePerSec, h.cfg.DefaultRatePerSec, rateCap),
		FetchTimeout: h.cfg.FetchTimeout,
	}, nil
}

// clamp returns def when v<=0 (the client omitted the field), otherwise v bounded
// to [1, max].
func clamp(v, def, max int) int {
	if v <= 0 {
		v = def
	}
	if v < 1 {
		v = 1
	}
	if v > max {
		v = max
	}
	return v
}

// parseID reads the {id} path segment; returns (id, false) and writes a 400 on
// bad input so callers can just `if !ok { return }`.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid crawl id")
		return 0, false
	}
	return id, true
}

// toJobResponse maps a domain.CrawlJob onto the wire DTO (decoupling the two).
func toJobResponse(j *domain.CrawlJob) jobResponse {
	results := make([]pageResponse, len(j.Results))
	for i, res := range j.Results {
		results[i] = pageResponse{
			URL:        res.URL,
			Depth:      res.Depth,
			HTTPStatus: res.HTTPStatus,
			LinksFound: res.LinksFound,
			LatencyMS:  res.LatencyMS,
			Error:      res.Error,
		}
	}
	return jobResponse{
		ID:              j.ID,
		Seed:            j.Seed,
		Pages:           j.Pages,
		Edges:           j.Edges,
		MaxDepthReached: j.MaxDepthReached,
		Fetches:         j.Fetches,
		DurationMS:      j.DurationMS,
		CreatedAt:       j.CreatedAt.Format(time.RFC3339), // format a time.Time as a string
		Results:         results,
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
		writeError(w, http.StatusNotFound, "crawl job not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
