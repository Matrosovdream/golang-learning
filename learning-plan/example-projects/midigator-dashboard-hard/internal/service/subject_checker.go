package service

import (
	"context"

	"midigator/internal/domain"
)

// subjectChecker resolves whether a polymorphic subject (a case or an order)
// exists within a tenant. It is the concrete domain.SubjectChecker used by the
// comment and evidence services, bridging the case registry and the order repo.
type subjectChecker struct {
	registry *CaseRegistry
	orders   domain.OrderRepository
}

// NewSubjectChecker builds the polymorphic existence checker.
func NewSubjectChecker(registry *CaseRegistry, orders domain.OrderRepository) domain.SubjectChecker {
	return &subjectChecker{registry: registry, orders: orders}
}

func (s *subjectChecker) SubjectExists(ctx context.Context, tenantID int64, subjectType string, subjectID int64) (bool, error) {
	if subjectType == "order" {
		return s.orders.ExistsForTenant(ctx, tenantID, subjectID)
	}
	ct, ok := domain.ParseCaseType(subjectType)
	if !ok {
		return false, nil
	}
	repo, err := s.registry.RepoFor(ct)
	if err != nil {
		return false, nil
	}
	return repo.ExistsForTenant(ctx, tenantID, subjectID)
}
