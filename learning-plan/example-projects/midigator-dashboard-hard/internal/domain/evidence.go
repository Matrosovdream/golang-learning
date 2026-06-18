package domain

import (
	"context"
	"time"
)

// EvidenceFile is a polymorphic uploaded file attached to a case or order.
type EvidenceFile struct {
	ID               int64
	TenantID         int64
	EvidenceableType string
	EvidenceableID   int64
	UploadedBy       int64
	UploaderName     string
	Name             string
	Path             string
	MimeType         string
	SizeBytes        int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// EvidenceRepository stores polymorphic evidence files.
type EvidenceRepository interface {
	Create(ctx context.Context, e *EvidenceFile) (*EvidenceFile, error)
	ListFor(ctx context.Context, tenantID int64, subjectType string, subjectID int64) ([]EvidenceFile, error)
	GetByID(ctx context.Context, tenantID, id int64) (*EvidenceFile, error)
	Delete(ctx context.Context, tenantID, id int64) error
}
