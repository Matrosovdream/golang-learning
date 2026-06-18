package service

import (
	"context"

	"midigator/internal/domain"
)

// SearchService runs a single free-text query across every case type. It fans
// the term out to each registered case repository via the shared CaseSummary
// projection, merges the results, and caps the merged list — so callers never
// branch on the concrete case type.
type SearchService struct {
	registry *CaseRegistry
}

// NewSearchService wires the cross-type search service.
func NewSearchService(registry *CaseRegistry) *SearchService {
	return &SearchService{registry: registry}
}

// searchDefaultLimit caps the merged result set when no (or an invalid) limit
// is supplied.
const searchDefaultLimit = 50

// Search returns up to limit case summaries (across all types) matching q. An
// empty query yields no results; per-type totals are ignored.
func (s *SearchService) Search(ctx context.Context, q string, limit int) ([]domain.CaseSummary, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if q == "" {
		return []domain.CaseSummary{}, nil
	}
	if limit <= 0 {
		limit = searchDefaultLimit
	}
	f := domain.CaseFilter{Search: q, Limit: limit}
	out := make([]domain.CaseSummary, 0, limit)
	for _, repo := range s.registry.All() {
		summaries, _, err := repo.ListSummaries(ctx, tenantID, f)
		if err != nil {
			return nil, err
		}
		out = append(out, summaries...)
		if len(out) >= limit {
			return out[:limit], nil
		}
	}
	return out, nil
}
