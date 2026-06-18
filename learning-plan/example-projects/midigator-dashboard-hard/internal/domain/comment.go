package domain

import (
	"context"
	"time"
)

// Comment is a polymorphic note attached to a case or order.
type Comment struct {
	ID              int64
	CommentableType string
	CommentableID   int64
	UserID          int64
	UserName        string
	Body            string
	IsInternal      bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CommentRepository stores polymorphic comments.
type CommentRepository interface {
	Create(ctx context.Context, c *Comment) (*Comment, error)
	ListFor(ctx context.Context, subjectType string, subjectID int64) ([]Comment, error)
	GetByID(ctx context.Context, id int64) (*Comment, error)
	Delete(ctx context.Context, id int64) error
}

// SubjectChecker verifies a polymorphic subject (case or order) exists within a
// tenant. Implemented in the service layer over the case registry + order repo.
type SubjectChecker interface {
	SubjectExists(ctx context.Context, tenantID int64, subjectType string, subjectID int64) (bool, error)
}

// CommentSubjectTypes are the subjects that accept comments.
func CommentSubjectTypes() []string {
	return []string{"chargeback", "prevention", "order", "order_validation", "rdr"}
}

// EvidenceSubjectTypes are the subjects that accept evidence files.
func EvidenceSubjectTypes() []string {
	return []string{"chargeback", "prevention", "rdr", "order"}
}
