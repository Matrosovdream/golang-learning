package service

import (
	"context"

	"midigator/internal/domain"
)

// ManagerService powers the manager plane: a tenant-scoped home overview, the
// team roster (with performance scores), the unassigned-cases assignment board,
// the approvals queue (cases awaiting sign-off), the activity feed, plus the
// assign and scoring actions. It reshapes the read-only ReportingRepository and
// delegates the actual case mutation to the generic CaseService.
type ManagerService struct {
	reporting domain.ReportingRepository
	profiles  domain.ManagerProfileRepository
	cases     *CaseService
	activity  domain.ActivityRepository
	logger    activityLogger
}

// NewManagerService wires the manager service.
func NewManagerService(reporting domain.ReportingRepository, profiles domain.ManagerProfileRepository, cases *CaseService, activity domain.ActivityRepository) *ManagerService {
	return &ManagerService{reporting: reporting, profiles: profiles, cases: cases, activity: activity, logger: activityLogger{repo: activity}}
}

// ManagerHome is the manager landing overview: per-(type, stage) counts plus the
// performance leaderboard.
type ManagerHome struct {
	StageCounts []domain.TypeStageCount
	Leaderboard []domain.ManagerStat
}

// Home returns the manager landing overview for the current tenant.
func (s *ManagerService) Home(ctx context.Context) (*ManagerHome, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.reporting.TenantStageCounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	leaderboard, err := s.reporting.ManagerLeaderboard(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &ManagerHome{StageCounts: counts, Leaderboard: leaderboard}, nil
}

// Team returns every manager profile in the current tenant (joined to users).
func (s *ManagerService) Team(ctx context.Context) ([]domain.ManagerProfile, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.profiles.ListByTenant(ctx, tenantID)
}

// Assignments returns the unassigned-cases queue for the assignment board.
func (s *ManagerService) Assignments(ctx context.Context) ([]domain.CaseSummary, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.reporting.UnassignedCases(ctx, tenantID, 100)
}

// Approvals returns the cases awaiting sign-off (action_taken stage).
func (s *ManagerService) Approvals(ctx context.Context) ([]domain.CaseSummary, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	return s.reporting.CasesInStage(ctx, tenantID, "action_taken", 100)
}

// Activity returns the tenant's recent activity feed.
func (s *ManagerService) Activity(ctx context.Context) ([]domain.ActivityEntry, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	entries, _, err := s.activity.ListByTenant(ctx, tenantID, domain.ActivityFilter{Limit: 50})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// managerKinds maps the manager-plane route segment to its case type.
var managerKinds = map[string]domain.CaseType{
	"chargebacks": domain.CaseChargeback,
	"preventions": domain.CasePrevention,
	"rdr":         domain.CaseRdr,
}

// Assign sets or clears (assigneeID nil) the owner of a case, addressed by the
// manager-plane kind segment.
func (s *ManagerService) Assign(ctx context.Context, kind string, id int64, assigneeID *int64) error {
	caseType, ok := managerKinds[kind]
	if !ok {
		return domain.Invalid("kind", "must be one of chargebacks, preventions, rdr")
	}
	return s.cases.Assign(ctx, caseType, id, assigneeID)
}

// SetScore upserts a team member's performance score and notes.
func (s *ManagerService) SetScore(ctx context.Context, userID int64, score *float64, notes string) (*domain.ManagerProfile, error) {
	actorID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := s.profiles.SetScore(ctx, userID, score, notes)
	if err != nil {
		return nil, err
	}
	s.logger.log(ctx, tenantID, actorID, "manager_profile", int64Ptr(userID), "manager_scored", map[string]any{"score": score})
	return profile, nil
}
