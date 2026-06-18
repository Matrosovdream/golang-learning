package service

import (
	"context"

	"midigator/internal/domain"
	"midigator/internal/reqctx"
)

// ActivityService reads the tenant's audit log. By default a caller sees only
// their own activity; holding the "activity_log.view_all" right widens the view
// to the whole tenant.
type ActivityService struct {
	repo domain.ActivityRepository
}

// NewActivityService wires the activity-log service.
func NewActivityService(repo domain.ActivityRepository) *ActivityService {
	return &ActivityService{repo: repo}
}

// List returns a page of activity-log entries for the caller's tenant, scoped to
// the caller's own actions unless they may view all.
func (s *ActivityService) List(ctx context.Context, limit, offset int) ([]domain.ActivityEntry, int, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, 0, err
	}
	a, _ := reqctx.From(ctx)
	f := domain.ActivityFilter{Limit: limit, Offset: offset}
	if !a.HasRight("activity_log.view_all") {
		f.UserID = &userID
	}
	return s.repo.ListByTenant(ctx, tenantID, f)
}
