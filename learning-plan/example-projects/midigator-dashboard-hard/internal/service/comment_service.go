package service

import (
	"context"
	"slices"
	"strings"

	"midigator/internal/domain"
)

// CommentService handles polymorphic comments attached to a case or order. It
// validates the subject type, verifies the subject exists within the caller's
// tenant (comments themselves carry no tenant_id), and writes the activity log.
type CommentService struct {
	repo     domain.CommentRepository
	subjects domain.SubjectChecker
	activity activityLogger
}

// NewCommentService wires the comment service.
func NewCommentService(repo domain.CommentRepository, subjects domain.SubjectChecker, activity domain.ActivityRepository) *CommentService {
	return &CommentService{repo: repo, subjects: subjects, activity: activityLogger{repo: activity}}
}

// List returns the comments for a subject after validating the subject type and
// confirming the subject exists within the caller's tenant.
func (s *CommentService) List(ctx context.Context, subjectType string, subjectID int64) ([]domain.Comment, error) {
	_, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(domain.CommentSubjectTypes(), subjectType) {
		return nil, domain.Invalid("type", "is not a commentable subject")
	}
	ok, err := s.subjects.SubjectExists(ctx, tenantID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound(subjectType)
	}
	return s.repo.ListFor(ctx, subjectType, subjectID)
}

// Create adds a comment to a subject after validating input and tenant scope.
func (s *CommentService) Create(ctx context.Context, subjectType string, subjectID int64, body string, isInternal bool) (*domain.Comment, error) {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(domain.CommentSubjectTypes(), subjectType) {
		return nil, domain.Invalid("type", "is not a commentable subject")
	}
	if strings.TrimSpace(body) == "" {
		return nil, domain.Invalid("body", "is required")
	}
	ok, err := s.subjects.SubjectExists(ctx, tenantID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound(subjectType)
	}
	c, err := s.repo.Create(ctx, &domain.Comment{
		CommentableType: subjectType,
		CommentableID:   subjectID,
		UserID:          userID,
		Body:            body,
		IsInternal:      isInternal,
	})
	if err != nil {
		return nil, err
	}
	s.activity.log(ctx, tenantID, userID, subjectType, int64Ptr(subjectID), "commented", nil)
	return c, nil
}

// Delete removes a comment after confirming it exists.
func (s *CommentService) Delete(ctx context.Context, id int64) error {
	userID, tenantID, err := tenantActor(ctx)
	if err != nil {
		return err
	}
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.activity.log(ctx, tenantID, userID, c.CommentableType, int64Ptr(c.CommentableID), "comment_deleted", nil)
	return nil
}
