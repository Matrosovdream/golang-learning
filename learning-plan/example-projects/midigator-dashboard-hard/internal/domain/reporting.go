package domain

import "context"

// TypeStageCount is one (case type, stage) -> count cell for dashboards.
type TypeStageCount struct {
	Type  string
	Stage string
	Count int
}

// ManagerStat is one row of the manager performance leaderboard.
type ManagerStat struct {
	UserID   int64
	Name     string
	Email    string
	Open     int
	Resolved int
	Score    *float64
}

// PlatformStats is the cross-tenant overview for the platform plane.
type PlatformStats struct {
	Tenants     int
	Users       int
	Chargebacks int
	Preventions int
	Orders      int
	OpenCases   int
}

// TenantOverview is a per-tenant summary (platform plane).
type TenantOverview struct {
	TenantID         int64
	Users            int
	Chargebacks      int
	Preventions      int
	OrderValidations int
	RdrCases         int
	Orders           int
	OpenCases        int
}

// ReportingRepository runs the cross-case-type aggregate queries (UNION over
// chargebacks/prevention_alerts/order_validations/rdr_cases) that power the
// dashboard, the manager plane, and the platform overview. It is read-only.
type ReportingRepository interface {
	TenantStageCounts(ctx context.Context, tenantID int64) ([]TypeStageCount, error)
	ManagerLeaderboard(ctx context.Context, tenantID int64) ([]ManagerStat, error)
	UnassignedCases(ctx context.Context, tenantID int64, limit int) ([]CaseSummary, error)
	CasesInStage(ctx context.Context, tenantID int64, stage string, limit int) ([]CaseSummary, error)
	PlatformTotals(ctx context.Context) (*PlatformStats, error)
	TenantOverview(ctx context.Context, tenantID int64) (*TenantOverview, error)
}
