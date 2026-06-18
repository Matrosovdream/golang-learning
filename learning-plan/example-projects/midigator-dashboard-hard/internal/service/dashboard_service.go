package service

import (
	"context"

	"midigator/internal/domain"
)

// DashboardService powers the tenant home screen: a stage-count summary across
// all case types, the manager performance leaderboard, and recent activity. It
// is read-only and reshapes the reporting aggregates for the handler.
type DashboardService struct {
	reporting domain.ReportingRepository
	activity  domain.ActivityRepository
}

// NewDashboardService wires the dashboard service.
func NewDashboardService(reporting domain.ReportingRepository, activity domain.ActivityRepository) *DashboardService {
	return &DashboardService{reporting: reporting, activity: activity}
}

// Summary reshapes the per-(type,stage) counts into a nested structure plus
// per-type totals and an open total (every case not in the 'resolved' stage).
func (s *DashboardService) Summary(ctx context.Context) (map[string]any, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.reporting.TenantStageCounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byType := map[string]map[string]int{}
	totals := map[string]int{}
	openTotal := 0
	for _, c := range counts {
		stages := byType[c.Type]
		if stages == nil {
			stages = map[string]int{}
			byType[c.Type] = stages
		}
		stages[c.Stage] += c.Count
		totals[c.Type] += c.Count
		if c.Stage != "resolved" {
			openTotal += c.Count
		}
	}
	return map[string]any{
		"by_type":    byType,
		"totals":     totals,
		"open_total": openTotal,
	}, nil
}

// ManagerPerformance returns the manager leaderboard for the current tenant.
func (s *DashboardService) ManagerPerformance(ctx context.Context) ([]domain.ManagerStat, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.reporting.ManagerLeaderboard(ctx, tenantID)
}

// RecentActivity returns the most recent audit-log entries for the tenant.
func (s *DashboardService) RecentActivity(ctx context.Context) ([]domain.ActivityEntry, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	entries, _, err := s.activity.ListByTenant(ctx, tenantID, domain.ActivityFilter{Limit: 20})
	if err != nil {
		return nil, err
	}
	return entries, nil
}
