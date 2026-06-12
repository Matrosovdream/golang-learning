package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"pollhub/internal/httpx"
)

type API struct {
	store *Store
	polls *PollClient
}

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /votes", a.castVote)
	mux.HandleFunc("GET /polls/{id}/votes", a.listVotes)
	mux.HandleFunc("GET /healthz", healthz)
	return httpx.Log(mux)
}

func (a *API) castVote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PollID   int64  `json:"poll_id"`
		OptionID int64  `json:"option_id"`
		Voter    string `json:"voter"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	in.Voter = strings.TrimSpace(in.Voter)
	if in.PollID <= 0 || in.OptionID <= 0 || in.Voter == "" {
		httpx.Error(w, http.StatusBadRequest, "poll_id, option_id and voter are required")
		return
	}

	// Ask poll-service whether this vote is allowed right now.
	poll, err := a.polls.Get(r.Context(), in.PollID)
	switch {
	case errors.Is(err, errPollNotFound):
		httpx.Error(w, http.StatusNotFound, errPollNotFound.Error())
		return
	case err != nil:
		httpx.Error(w, http.StatusBadGateway, err.Error())
		return
	}
	if !poll.IsOpen() {
		httpx.Error(w, http.StatusConflict, "poll is closed")
		return
	}
	if !poll.HasOption(in.OptionID) {
		httpx.Error(w, http.StatusBadRequest, "option does not belong to this poll")
		return
	}

	vote, err := a.store.CastVote(r.Context(), in.PollID, in.OptionID, in.Voter)
	switch {
	case errors.Is(err, errAlreadyVoted):
		httpx.Error(w, http.StatusConflict, errAlreadyVoted.Error())
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.JSON(w, http.StatusCreated, vote)
	}
}

func (a *API) listVotes(w http.ResponseWriter, r *http.Request) {
	pollID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid poll id")
		return
	}

	votes, err := a.store.ListVotes(r.Context(), pollID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, votes)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
