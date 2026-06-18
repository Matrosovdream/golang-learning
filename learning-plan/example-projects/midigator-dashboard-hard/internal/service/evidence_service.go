package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"

	"midigator/internal/domain"
)

// EvidenceService manages polymorphic evidence files. Like comments it validates
// the subject against the registry-backed SubjectChecker, but evidence rows are
// tenant-scoped. This is a metadata-only upload: the caller supplies a name,
// mime, and size, and the service stores a synthetic path (no real file IO).
type EvidenceService struct {
	repo     domain.EvidenceRepository
	subjects domain.SubjectChecker
	activity activityLogger
}

// NewEvidenceService wires the evidence service.
func NewEvidenceService(repo domain.EvidenceRepository, subjects domain.SubjectChecker, activity domain.ActivityRepository) *EvidenceService {
	return &EvidenceService{repo: repo, subjects: subjects, activity: activityLogger{repo: activity}}
}

// List returns a subject's evidence files for the current tenant.
func (s *EvidenceService) List(ctx context.Context, subjectType string, subjectID int64) ([]domain.EvidenceFile, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.validateSubject(ctx, tenantID, subjectType, subjectID); err != nil {
		return nil, err
	}
	return s.repo.ListFor(ctx, tenantID, subjectType, subjectID)
}

// Create records evidence metadata for a subject, attributing it to the caller.
func (s *EvidenceService) Create(ctx context.Context, subjectType string, subjectID int64, name, mimeType string, sizeBytes int64) (*domain.EvidenceFile, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, domain.Invalid("name", "is required")
	}
	if sizeBytes < 0 {
		return nil, domain.Invalid("size_bytes", "must not be negative")
	}
	if err := s.validateSubject(ctx, tenantID, subjectType, subjectID); err != nil {
		return nil, err
	}
	e := &domain.EvidenceFile{
		TenantID:         tenantID,
		EvidenceableType: subjectType,
		EvidenceableID:   subjectID,
		UploadedBy:       userID,
		Name:             name,
		Path:             syntheticPath(tenantID),
		MimeType:         mimeType,
		SizeBytes:        sizeBytes,
	}
	created, err := s.repo.Create(ctx, e)
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, subjectType, int64Ptr(subjectID), "evidence_uploaded", map[string]any{"evidence_id": created.ID, "name": name})
	return created, nil
}

// Delete removes a tenant's evidence file.
func (s *EvidenceService) Delete(ctx context.Context, id int64) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	e, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.activity.log(ctx, tenantID, userID, e.EvidenceableType, int64Ptr(e.EvidenceableID), "evidence_deleted", map[string]any{"evidence_id": id})
	return nil
}

// validateSubject rejects unknown subject types and missing subjects.
func (s *EvidenceService) validateSubject(ctx context.Context, tenantID int64, subjectType string, subjectID int64) error {
	if !slices.Contains(domain.EvidenceSubjectTypes(), subjectType) {
		return domain.Invalid("type", "is not a valid evidence subject")
	}
	ok, err := s.subjects.SubjectExists(ctx, tenantID, subjectType, subjectID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound(subjectType)
	}
	return nil
}

// syntheticPath fabricates a storage key for the metadata-only upload.
func syntheticPath(tenantID int64) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("evidence/%d/%s.bin", tenantID, hex.EncodeToString(b))
}
