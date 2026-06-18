package service

import (
	"context"

	"midigator/internal/domain"
)

// RdrService holds the RDR-specific operations (show with history, edit custom
// fields). The generic stage/assign/hide/resolve live in CaseService; this is
// the typed half.
type RdrService struct {
	repo        domain.RdrRepository
	transitions domain.StageTransitionRepository
	activity    activityLogger
}

// NewRdrService wires the RDR service.
func NewRdrService(repo domain.RdrRepository, transitions domain.StageTransitionRepository, activity domain.ActivityRepository) *RdrService {
	return &RdrService{repo: repo, transitions: transitions, activity: activityLogger{repo: activity}}
}

// RdrDetail is an RDR case plus its workflow history.
type RdrDetail struct {
	Rdr     *domain.RdrCase
	History []domain.StageTransition
}

// Get returns an RDR case with its stage history.
func (s *RdrService) Get(ctx context.Context, id int64) (*RdrDetail, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	rc, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	history, err := s.transitions.ListFor(ctx, domain.CaseRdr, id)
	if err != nil {
		return nil, err
	}
	return &RdrDetail{Rdr: rc, History: history}, nil
}

// Update edits an RDR case's custom fields (whitelisted in the repository).
func (s *RdrService) Update(ctx context.Context, id int64, fields map[string]any) (*domain.RdrCase, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	rc, err := s.repo.Update(ctx, tenantID, id, fields)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, string(domain.CaseRdr), int64Ptr(id), "updated", nil)
	return rc, nil
}
