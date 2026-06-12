package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"pollhub/internal/httpx"
)

type API struct{ store *Store }

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /polls", a.createPoll)
	mux.HandleFunc("GET /polls", a.listPolls)
	mux.HandleFunc("GET /polls/{id}", a.getPoll)
	mux.HandleFunc("POST /polls/{id}/close", a.closePoll)
	mux.HandleFunc("DELETE /polls/{id}", a.deletePoll)
	mux.HandleFunc("GET /healthz", healthz)
	return httpx.Log(mux)
}

func (a *API) createPoll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	in.Question = strings.TrimSpace(in.Question)
	labels := make([]string, 0, len(in.Options))
	for _, l := range in.Options {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	if in.Question == "" || len(labels) < 2 {
		httpx.Error(w, http.StatusBadRequest, "question and at least two non-empty options are required")
		return
	}

	poll, err := a.store.CreatePoll(r.Context(), in.Question, labels)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, poll)
}

func (a *API) listPolls(w http.ResponseWriter, r *http.Request) {
	polls, err := a.store.ListPolls(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, polls)
}

func (a *API) getPoll(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid poll id")
		return
	}

	poll, err := a.store.GetPoll(r.Context(), id)
	switch {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, errNotFound.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.JSON(w, http.StatusOK, poll)
	}
}

func (a *API) closePoll(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid poll id")
		return
	}

	switch err := a.store.ClosePoll(r.Context(), id); {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, errNotFound.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "closed"})
	}
}

func (a *API) deletePoll(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid poll id")
		return
	}

	switch err := a.store.DeletePoll(r.Context(), id); {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, errNotFound.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// pathID parses the {id} segment of the matched route.
func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
