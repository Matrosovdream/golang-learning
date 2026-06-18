package service

import "midigator/internal/domain"

// CaseRegistry resolves a CaseType to its repository, the Go analogue of the
// Laravel CaseRegistry. Generic case logic (stage/assign/hide/resolve/list),
// search, and the assignment board all go through it so they never branch on
// the concrete case type.
type CaseRegistry struct {
	repos map[domain.CaseType]domain.CaseRepository
}

// NewCaseRegistry builds an empty registry; call Register for each case repo.
func NewCaseRegistry() *CaseRegistry {
	return &CaseRegistry{repos: make(map[domain.CaseType]domain.CaseRepository)}
}

// Register adds a case repository under its own CaseType.
func (r *CaseRegistry) Register(repo domain.CaseRepository) {
	r.repos[repo.CaseType()] = repo
}

// RepoFor returns the repository for a case type.
func (r *CaseRegistry) RepoFor(t domain.CaseType) (domain.CaseRepository, error) {
	repo, ok := r.repos[t]
	if !ok {
		return nil, domain.NotFound("case type")
	}
	return repo, nil
}

// All returns every case repository, in display order.
func (r *CaseRegistry) All() []domain.CaseRepository {
	out := make([]domain.CaseRepository, 0, len(r.repos))
	for _, t := range domain.AllCaseTypes() {
		if repo, ok := r.repos[t]; ok {
			out = append(out, repo)
		}
	}
	return out
}
