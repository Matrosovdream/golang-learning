package service

import (
	"context"
	"slices"

	"midigator/internal/domain"
)

// ChargebackService holds the chargeback-specific operations (show with
// history, edit custom fields, bulk hide). The generic stage/assign/hide live
// in CaseService; this is the typed half.
type ChargebackService struct {
	repo        domain.ChargebackRepository
	transitions domain.StageTransitionRepository
	activity    activityLogger
}

// NewChargebackService wires the chargeback service.
func NewChargebackService(repo domain.ChargebackRepository, transitions domain.StageTransitionRepository, activity domain.ActivityRepository) *ChargebackService {
	return &ChargebackService{repo: repo, transitions: transitions, activity: activityLogger{repo: activity}}
}

// ChargebackDetail is a chargeback plus its workflow history.
type ChargebackDetail struct {
	Chargeback *domain.Chargeback
	History    []domain.StageTransition
}

// Get returns a chargeback with its stage history.
func (s *ChargebackService) Get(ctx context.Context, id int64) (*ChargebackDetail, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	cb, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	history, err := s.transitions.ListFor(ctx, domain.CaseChargeback, id)
	if err != nil {
		return nil, err
	}
	return &ChargebackDetail{Chargeback: cb, History: history}, nil
}

var chargebackResults = []string{"pending", "won", "lost", "pre_arbitration"}

// Update edits a chargeback's custom fields (whitelisted in the repository).
func (s *ChargebackService) Update(ctx context.Context, id int64, fields map[string]any) (*domain.Chargeback, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if v, ok := fields["result"]; ok {
		if str, _ := v.(string); !slices.Contains(chargebackResults, str) {
			return nil, domain.Invalid("result", "must be one of pending, won, lost, pre_arbitration")
		}
	}
	cb, err := s.repo.Update(ctx, tenantID, id, fields)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, string(domain.CaseChargeback), int64Ptr(id), "updated", nil)
	return cb, nil
}

// BulkHide hides or unhides many chargebacks at once.
func (s *ChargebackService) BulkHide(ctx context.Context, ids []int64, hidden bool) (int, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return 0, err
	}
	n, err := s.repo.BulkSetHidden(ctx, tenantID, ids, hidden)
	if err != nil {
		return 0, err
	}
	s.activity.log(ctx, tenantID, userID, string(domain.CaseChargeback), nil, "bulk_hidden", map[string]any{"count": n, "hidden": hidden})
	return n, nil
}
