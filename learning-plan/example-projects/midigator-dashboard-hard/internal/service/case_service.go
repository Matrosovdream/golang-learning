package service

import (
	"context"

	"midigator/internal/domain"
	"midigator/internal/events"
)

// CaseService implements the generic, type-agnostic case operations shared by
// chargebacks, preventions, order-validations, and RDR cases: list, change
// stage, assign, hide, resolve. It resolves the concrete repository from the
// registry, publishes domain events, and writes the activity log.
type CaseService struct {
	registry *CaseRegistry
	bus      *events.Bus
	activity activityLogger
}

// NewCaseService wires the generic case service.
func NewCaseService(registry *CaseRegistry, bus *events.Bus, activity domain.ActivityRepository) *CaseService {
	return &CaseService{registry: registry, bus: bus, activity: activityLogger{repo: activity}}
}

// List returns a tenant-scoped page of case summaries for one case type.
func (s *CaseService) List(ctx context.Context, t domain.CaseType, f domain.CaseFilter) ([]domain.CaseSummary, int, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, 0, err
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return nil, 0, err
	}
	return repo.ListSummaries(ctx, tenantID, f)
}

// ChangeStage moves a case to a new workflow stage (recording a transition).
func (s *CaseService) ChangeStage(ctx context.Context, t domain.CaseType, id int64, toStage, notes string) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return err
	}
	if !domain.ValidStage(repo, toStage) {
		return domain.Invalid("stage", "is not a valid stage for "+string(t))
	}
	from, err := repo.GetStage(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if err := repo.ChangeStage(ctx, tenantID, id, toStage, userID, notes); err != nil {
		return err
	}
	s.bus.Publish(ctx, events.StageChanged{TenantID: tenantID, CaseType: string(t), CaseID: id, FromStage: from, ToStage: toStage, UserID: userID})
	s.activity.log(ctx, tenantID, userID, string(t), int64Ptr(id), "stage_changed", map[string]any{"from": from, "to": toStage})
	return nil
}

// Assign sets or clears (assigneeID nil) a case's owner.
func (s *CaseService) Assign(ctx context.Context, t domain.CaseType, id int64, assigneeID *int64) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return err
	}
	if err := repo.Assign(ctx, tenantID, id, assigneeID); err != nil {
		return err
	}
	if assigneeID != nil {
		s.bus.Publish(ctx, events.CaseAssigned{TenantID: tenantID, CaseType: string(t), CaseID: id, AssignedTo: *assigneeID, ByUserID: userID})
	}
	s.activity.log(ctx, tenantID, userID, string(t), int64Ptr(id), "assigned", map[string]any{"assigned_to": assigneeID})
	return nil
}

// Hide hides or unhides a case from managers.
func (s *CaseService) Hide(ctx context.Context, t domain.CaseType, id int64, hidden bool) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return err
	}
	if err := repo.SetHidden(ctx, tenantID, id, hidden); err != nil {
		return err
	}
	action := "unhidden"
	if hidden {
		action = "hidden"
	}
	s.activity.log(ctx, tenantID, userID, string(t), int64Ptr(id), action, nil)
	return nil
}

// Resolve submits a resolution for a case type that supports it.
func (s *CaseService) Resolve(ctx context.Context, t domain.CaseType, id int64, resolution string) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	repo, err := s.registry.RepoFor(t)
	if err != nil {
		return err
	}
	if !repo.CanResolve() {
		return domain.ConflictError{Message: string(t) + " does not support resolution"}
	}
	if err := repo.Resolve(ctx, tenantID, id, resolution, userID); err != nil {
		return err
	}
	s.activity.log(ctx, tenantID, userID, string(t), int64Ptr(id), "resolved", map[string]any{"resolution": resolution})
	return nil
}
