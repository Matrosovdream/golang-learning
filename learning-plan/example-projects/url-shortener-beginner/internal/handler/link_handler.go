package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"urlshortener/internal/config"
	"urlshortener/internal/domain"
	"urlshortener/internal/service"
)

// LinkHandler turns HTTP requests into service calls and writes responses.
// It is the only layer that knows about http.ResponseWriter / *http.Request.
type LinkHandler struct {
	svc *service.LinkService
	cfg config.Config
}

// NewLinkHandler injects the service and config the handler needs.
func NewLinkHandler(svc *service.LinkService, cfg config.Config) *LinkHandler {
	return &LinkHandler{svc: svc, cfg: cfg}
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
	LongURL  string `json:"long_url"`
}

type statsResponse struct {
	Code      string `json:"code"`
	LongURL   string `json:"long_url"`
	Clicks    int64  `json:"clicks"`
	CreatedAt string `json:"created_at"`
}

// Shorten handles POST /shorten.
func (h *LinkHandler) Shorten(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	link, err := h.svc.Shorten(r.Context(), req.URL)
	if err != nil {
		if errors.Is(err, service.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not shorten url")
		return
	}
	writeJSON(w, http.StatusCreated, shortenResponse{
		Code:     link.Code,
		ShortURL: h.cfg.BaseURL + "/" + link.Code,
		LongURL:  link.LongURL,
	})
}

// Redirect handles GET /{code} and 302-redirects to the original URL.
func (h *LinkHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := h.svc.Resolve(r.Context(), code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "short link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not resolve link")
		return
	}
	http.Redirect(w, r, link.LongURL, http.StatusFound)
}

// Stats handles GET /api/stats/{code}.
func (h *LinkHandler) Stats(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := h.svc.Stats(r.Context(), code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "short link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	writeJSON(w, http.StatusOK, statsResponse{
		Code:      link.Code,
		LongURL:   link.LongURL,
		Clicks:    link.Clicks,
		CreatedAt: link.CreatedAt.Format(time.RFC3339),
	})
}
