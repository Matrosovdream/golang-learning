package service

import (
	"context"

	"midigator/internal/domain"
)

// OrderValidationService holds the order-validation-specific operations (show
// with history, edit custom fields). The generic stage/assign/hide live in
// CaseService; this is the typed half.
type OrderValidationService struct {
	repo        domain.OrderValidationRepository
	transitions domain.StageTransitionRepository
	activity    activityLogger
}

// NewOrderValidationService wires the order-validation service.
func NewOrderValidationService(repo domain.OrderValidationRepository, transitions domain.StageTransitionRepository, activity domain.ActivityRepository) *OrderValidationService {
	return &OrderValidationService{repo: repo, transitions: transitions, activity: activityLogger{repo: activity}}
}

// OrderValidationDetail is an order validation plus its workflow history.
type OrderValidationDetail struct {
	OrderValidation *domain.OrderValidation
	History         []domain.StageTransition
}

// Get returns an order validation with its stage history.
func (s *OrderValidationService) Get(ctx context.Context, id int64) (*OrderValidationDetail, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	history, err := s.transitions.ListFor(ctx, domain.CaseOrderValidation, id)
	if err != nil {
		return nil, err
	}
	return &OrderValidationDetail{OrderValidation: v, History: history}, nil
}

// Update edits an order validation's custom fields (whitelisted in the repository).
func (s *OrderValidationService) Update(ctx context.Context, id int64, fields map[string]any) (*domain.OrderValidation, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Update(ctx, tenantID, id, fields)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, string(domain.CaseOrderValidation), int64Ptr(id), "updated", nil)
	return v, nil
}
