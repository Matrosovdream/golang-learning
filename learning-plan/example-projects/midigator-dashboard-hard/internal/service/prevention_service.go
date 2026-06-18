package service

import (
	"context"

	"midigator/internal/domain"
)

// PreventionService holds the prevention-specific operations (show with
// history, edit custom fields). The generic stage/assign/hide/resolve live in
// CaseService; this is the typed half.
type PreventionService struct {
	repo        domain.PreventionRepository
	transitions domain.StageTransitionRepository
	activity    activityLogger
}

// NewPreventionService wires the prevention service.
func NewPreventionService(repo domain.PreventionRepository, transitions domain.StageTransitionRepository, activity domain.ActivityRepository) *PreventionService {
	return &PreventionService{repo: repo, transitions: transitions, activity: activityLogger{repo: activity}}
}

// PreventionDetail is a prevention alert plus its workflow history.
type PreventionDetail struct {
	Prevention *domain.PreventionAlert
	History    []domain.StageTransition
}

// Get returns a prevention alert with its stage history.
func (s *PreventionService) Get(ctx context.Context, id int64) (*PreventionDetail, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	pa, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	history, err := s.transitions.ListFor(ctx, domain.CasePrevention, id)
	if err != nil {
		return nil, err
	}
	return &PreventionDetail{Prevention: pa, History: history}, nil
}

// Update edits a prevention alert's custom fields (whitelisted in the repository).
func (s *PreventionService) Update(ctx context.Context, id int64, fields map[string]any) (*domain.PreventionAlert, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	pa, err := s.repo.Update(ctx, tenantID, id, fields)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, string(domain.CasePrevention), int64Ptr(id), "updated", nil)
	return pa, nil
}
