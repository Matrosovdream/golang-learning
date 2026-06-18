package handler

import (
	"net/http"

	"midigator/internal/domain"
)

// PlatformOverviewHandler serves the platform-plane (cross-tenant) overview: a
// single totals card spanning every tenant. The router guards it with
// platform-admin middleware, so it does not re-check rights or tenant scope.
type PlatformOverviewHandler struct {
	reporting domain.ReportingRepository
}

func NewPlatformOverviewHandler(reporting domain.ReportingRepository) *PlatformOverviewHandler {
	return &PlatformOverviewHandler{reporting: reporting}
}

type platformStatsResponse struct {
	Tenants     int `json:"tenants"`
	Users       int `json:"users"`
	Chargebacks int `json:"chargebacks"`
	Preventions int `json:"preventions"`
	Orders      int `json:"orders"`
	OpenCases   int `json:"open_cases"`
}

func toPlatformStatsResponse(s *domain.PlatformStats) platformStatsResponse {
	return platformStatsResponse{
		Tenants:     s.Tenants,
		Users:       s.Users,
		Chargebacks: s.Chargebacks,
		Preventions: s.Preventions,
		Orders:      s.Orders,
		OpenCases:   s.OpenCases,
	}
}

// Summary returns the cross-tenant totals.
func (h *PlatformOverviewHandler) Summary(w http.ResponseWriter, r *http.Request) {
	stats, err := h.reporting.PlatformTotals(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPlatformStatsResponse(stats))
}
