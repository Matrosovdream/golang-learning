package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// ManagerHandler serves the manager plane: a tenant home overview, the team
// roster, the unassigned-cases assignment board (with assign action), the
// approvals queue, the activity feed, and per-user performance scoring.
type ManagerHandler struct {
	svc *service.ManagerService
}

func NewManagerHandler(svc *service.ManagerService) *ManagerHandler {
	return &ManagerHandler{svc: svc}
}

type stageCountResponse struct {
	Type  string `json:"type"`
	Stage string `json:"stage"`
	Count int    `json:"count"`
}

func toStageCounts(items []domain.TypeStageCount) []stageCountResponse {
	out := make([]stageCountResponse, len(items))
	for i, c := range items {
		out[i] = stageCountResponse{Type: c.Type, Stage: c.Stage, Count: c.Count}
	}
	return out
}

// Home returns the manager landing overview for the current tenant.
func (h *ManagerHandler) Home(w http.ResponseWriter, r *http.Request) {
	home, err := h.svc.Home(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"stage_counts": toStageCounts(home.StageCounts),
		"leaderboard":  toManagerStats(home.Leaderboard),
	})
}

type managerProfileResponse struct {
	UserID       int64           `json:"user_id"`
	UserName     string          `json:"user_name"`
	UserEmail    string          `json:"user_email"`
	Score        *float64        `json:"score"`
	Notes        string          `json:"notes"`
	AssignedMids json.RawMessage `json:"assigned_mids"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

func toManagerProfile(p domain.ManagerProfile) managerProfileResponse {
	return managerProfileResponse{
		UserID:       p.UserID,
		UserName:     p.UserName,
		UserEmail:    p.UserEmail,
		Score:        p.Score,
		Notes:        p.Notes,
		AssignedMids: p.AssignedMids,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
	}
}

func toManagerProfiles(items []domain.ManagerProfile) []managerProfileResponse {
	out := make([]managerProfileResponse, len(items))
	for i, p := range items {
		out[i] = toManagerProfile(p)
	}
	return out
}

// Team returns every manager profile in the current tenant.
func (h *ManagerHandler) Team(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.svc.Team(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toManagerProfiles(profiles)})
}

// Assignments returns the unassigned-cases queue for the assignment board.
func (h *ManagerHandler) Assignments(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Assignments(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toCaseSummaries(items)})
}

// Assign sets or clears the owner of a case addressed by kind + id.
func (h *ManagerHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req assignRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.Assign(r.Context(), r.PathValue("kind"), id, req.AssignedTo); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "assigned_to": req.AssignedTo})
}

// Approvals returns the cases awaiting sign-off.
func (h *ManagerHandler) Approvals(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Approvals(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toCaseSummaries(items)})
}

// Activity returns the tenant's recent activity feed.
func (h *ManagerHandler) Activity(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.Activity(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toActivityResponses(entries)})
}

type managerScoreRequest struct {
	Score *float64 `json:"score"`
	Notes string   `json:"notes"`
}

// UpdateScore upserts a team member's performance score and notes.
func (h *ManagerHandler) UpdateScore(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseID(w, r, "userId")
	if !ok {
		return
	}
	var req managerScoreRequest
	if !decode(w, r, &req) {
		return
	}
	profile, err := h.svc.SetScore(r.Context(), userID, req.Score, req.Notes)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toManagerProfile(*profile))
}
