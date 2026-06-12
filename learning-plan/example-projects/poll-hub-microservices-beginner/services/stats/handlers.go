package main

import (
	"errors"
	"net/http"
	"strconv"

	"pollhub/internal/httpx"
)

type API struct{ store *Store }

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /polls/{id}/results", a.pollResults)
	mux.HandleFunc("GET /top", a.topPolls)
	mux.HandleFunc("GET /healthz", healthz)
	return httpx.Log(mux)
}

func (a *API) pollResults(w http.ResponseWriter, r *http.Request) {
	pollID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid poll id")
		return
	}

	res, err := a.store.PollResults(r.Context(), pollID)
	switch {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, errNotFound.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}

func (a *API) topPolls(w http.ResponseWriter, r *http.Request) {
	// ?limit=N, default 5, clamped to 1..50
	limit := 5
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			httpx.Error(w, http.StatusBadRequest, "limit must be a number between 1 and 50")
			return
		}
		limit = n
	}

	top, err := a.store.TopPolls(r.Context(), limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, top)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
