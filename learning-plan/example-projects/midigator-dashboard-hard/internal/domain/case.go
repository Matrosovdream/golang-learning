package domain

import (
	"context"
	"slices"
	"time"
)

// CaseType identifies one of the four workable case kinds. It doubles as the
// polymorphic subject string stored on comments / evidence / stage_transitions.
type CaseType string

const (
	CaseChargeback      CaseType = "chargeback"
	CasePrevention      CaseType = "prevention"
	CaseOrderValidation CaseType = "order_validation"
	CaseRdr             CaseType = "rdr"
)

// AllCaseTypes is the registry's full set, in display order.
func AllCaseTypes() []CaseType {
	return []CaseType{CaseChargeback, CasePrevention, CaseOrderValidation, CaseRdr}
}

// ParseCaseType validates a raw string against the known case types.
func ParseCaseType(s string) (CaseType, bool) {
	ct := CaseType(s)
	return ct, slices.Contains(AllCaseTypes(), ct)
}

// CaseSummary is the common projection across all four case types. The generic
// case endpoints (list / stage / assign / hide), the assignment board, search,
// and dashboards all work in terms of this shape, so they never need to know
// the concrete type.
type CaseSummary struct {
	Type       CaseType
	ID         int64
	TenantID   int64
	GUID       string
	MID        string
	Amount     int64
	Currency   string
	Stage      string
	AssignedTo *int64
	IsHidden   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CaseFilter holds list/search parameters shared by every case type.
type CaseFilter struct {
	Stage         string
	AssignedTo    *int64
	MID           string
	Search        string
	IncludeHidden bool
	Limit         int
	Offset        int
	SortBy        string // created_at | updated_at | amount
	SortDir       string // asc | desc
}

// Normalize clamps paging and applies defaults.
func (f *CaseFilter) Normalize() {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	switch f.SortBy {
	case "created_at", "updated_at", "amount":
	default:
		f.SortBy = "created_at"
	}
	if f.SortDir != "asc" {
		f.SortDir = "desc"
	}
}

// CaseRepository is the cross-cutting contract every case repository satisfies,
// in addition to its own typed Get/Update. The CaseRegistry resolves a CaseType
// to one of these so generic logic stays type-agnostic.
type CaseRepository interface {
	CaseType() CaseType
	// AllowedStages lists the valid workflow stages for this case type.
	AllowedStages() []string
	// CanResolve reports whether this case type exposes a /resolve action.
	CanResolve() bool

	// ExistsForTenant reports whether the row exists within the tenant.
	ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error)
	// GetStage returns the current workflow stage.
	GetStage(ctx context.Context, tenantID, id int64) (string, error)
	// ChangeStage moves the case and records a stage_transition atomically.
	ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error
	// Assign sets (or clears, when assigneeID is nil) the owner.
	Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error
	// SetHidden hides/unhides the case from managers.
	SetHidden(ctx context.Context, tenantID, id int64, hidden bool) error
	// Resolve submits a resolution (preventions / rdr); no-op repos return a ConflictError.
	Resolve(ctx context.Context, tenantID, id int64, resolution string, userID int64) error

	// ListSummaries powers cross-type listing/search/assignment.
	ListSummaries(ctx context.Context, tenantID int64, f CaseFilter) ([]CaseSummary, int, error)
	// CountByStage returns stage -> count for dashboards.
	CountByStage(ctx context.Context, tenantID int64) (map[string]int, error)
}

// StageTransition is one audit row in a case's workflow history.
type StageTransition struct {
	ID            int64
	TrackableType CaseType
	TrackableID   int64
	UserID        int64
	UserName      string
	FromStage     string
	ToStage       string
	Notes         string
	CreatedAt     time.Time
}

// StageTransitionRepository reads a case's workflow history (rows are written
// transactionally inside each case repository's ChangeStage).
type StageTransitionRepository interface {
	ListFor(ctx context.Context, trackableType CaseType, trackableID int64) ([]StageTransition, error)
}

// ValidStage reports whether stage is allowed for the given repository.
func ValidStage(repo CaseRepository, stage string) bool {
	return slices.Contains(repo.AllowedStages(), stage)
}
