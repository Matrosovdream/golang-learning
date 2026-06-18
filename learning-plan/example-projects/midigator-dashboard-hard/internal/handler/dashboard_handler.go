package handler

import (
	"net/http"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// DashboardHandler serves the tenant dashboard: a cross-case-type summary, the
// manager performance leaderboard, and the recent-activity feed.
type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// Summary returns the stage-count summary across all case types.
func (h *DashboardHandler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Summary(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

type managerStatResponse struct {
	UserID   int64    `json:"user_id"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Open     int      `json:"open"`
	Resolved int      `json:"resolved"`
	Score    *float64 `json:"score"`
}

func toManagerStats(items []domain.ManagerStat) []managerStatResponse {
	out := make([]managerStatResponse, len(items))
	for i, m := range items {
		out[i] = managerStatResponse{
			UserID:   m.UserID,
			Name:     m.Name,
			Email:    m.Email,
			Open:     m.Open,
			Resolved: m.Resolved,
			Score:    m.Score,
		}
	}
	return out
}

// ManagerPerformance returns the manager leaderboard.
func (h *DashboardHandler) ManagerPerformance(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.ManagerPerformance(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toManagerStats(stats))
}

// RecentActivity returns the recent audit-log feed for the tenant.
func (h *DashboardHandler) RecentActivity(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.RecentActivity(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toActivityResponses(entries))
}
